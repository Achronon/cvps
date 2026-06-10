package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/terminal"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <sandbox> -- <command> [args...]",
	Short: "Run a command in a sandbox non-interactively",
	Long: `Run a command in a sandbox without an interactive shell: output is
streamed back and the remote command's exit code becomes cvps's exit
code. The automation replacement for 'cvps connect' in bootstrap
scripts - works headlessly under CVPS_API_TOKEN with no TTY.

The sandbox may be referenced by id (sbx-...) or by name.

The command rides the existing websocket terminal (a bash PTY), so
output is text-oriented and local stdin is NOT forwarded to the remote
command. For quoting-heavy one-liners use: cvps exec <sandbox> -- sh -c '<script>'.`,
	Example: `  # List the workspace
  cvps exec cortex-brain -- ls /workspace

  # Exit code propagates
  cvps exec cortex-brain -- test -f /workspace/.codex/auth.json && echo present

  # Shell one-liner
  cvps exec cortex-brain -- sh -c 'echo hello > /workspace/hello.txt'`,
	Args: cobra.MinimumNArgs(2),
	RunE: runExec,
}

func init() {
	rootCmd.AddCommand(execCmd)
}

// splitExecArgs splits cobra args into (sandbox ref, remote argv) honoring
// an explicit "--" separator position (cobra's ArgsLenAtDash). Without
// "--", the first arg is the sandbox and the rest is the command.
func splitExecArgs(args []string, argsLenAtDash int) (string, []string, error) {
	if argsLenAtDash >= 0 {
		// Everything before the dash must be exactly the sandbox ref.
		if argsLenAtDash != 1 {
			return "", nil, fmt.Errorf("usage: cvps exec <sandbox> -- <command> [args...]")
		}
		if len(args) < 2 {
			return "", nil, fmt.Errorf("no command provided after --")
		}
		return args[0], args[1:], nil
	}

	if len(args) < 2 {
		return "", nil, fmt.Errorf("usage: cvps exec <sandbox> -- <command> [args...]")
	}
	return args[0], args[1:], nil
}

// shellQuote renders argv as a single bash command string, single-quoting
// every argument (with the standard '\'' escape for embedded quotes).
func shellQuote(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}
	return strings.Join(quoted, " ")
}

// execMarkers are the per-invocation sentinels used to slice command
// output out of the PTY stream.
type execMarkers struct {
	nonce string
	begin string // appears alone on a line right before command output
	rc    string // followed by the decimal exit code and "__"
}

func newExecMarkers() (*execMarkers, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("failed to generate exec nonce: %w", err)
	}
	nonce := hex.EncodeToString(raw)
	return &execMarkers{
		nonce: nonce,
		begin: "__CVPS_EXEC_BEGIN_" + nonce + "__",
		rc:    "__CVPS_EXEC_RC_" + nonce + "_",
	}, nil
}

// buildRemoteCommand wraps the user command for marker scraping. The
// marker literals are SPLIT across two printf arguments in the typed
// line, so the PTY echo of this command line can never contain a full
// marker (only the printf OUTPUT does).
func buildRemoteCommand(m *execMarkers, userCmd string) string {
	return fmt.Sprintf(
		"stty -echo 2>/dev/null; printf '%%s%%s\\n' '__CVPS_EXEC_BEGIN_' '%s__'; %s; __cvps_rc=$?; printf '\\n%%s%%s%%d__\\n' '__CVPS_EXEC_RC_' '%s_' \"$__cvps_rc\"; exit \"$__cvps_rc\"\n",
		m.nonce, userCmd, m.nonce,
	)
}

// onceThenBlockReader yields its payload once, then blocks forever (the
// remote shell exits on its own; we never want to signal local EOF).
type onceThenBlockReader struct {
	payload []byte
	offset  int
	done    chan struct{}
	once    sync.Once
}

func newOnceThenBlockReader(payload string) *onceThenBlockReader {
	return &onceThenBlockReader{payload: []byte(payload), done: make(chan struct{})}
}

func (r *onceThenBlockReader) Read(p []byte) (int, error) {
	if r.offset < len(r.payload) {
		n := copy(p, r.payload[r.offset:])
		r.offset += n
		return n, nil
	}
	<-r.done // block until released (never, in practice)
	return 0, io.EOF
}

func (r *onceThenBlockReader) release() {
	r.once.Do(func() { close(r.done) })
}

// markerFilter is an io.Writer that passes through only the bytes between
// the begin marker and the rc marker, normalizing CRLF to LF. It tolerates
// markers AND CRLF pairs split across arbitrary Write chunk boundaries by
// buffering.
type markerFilter struct {
	out       io.Writer
	begin     []byte
	rcPfx     []byte
	started   bool
	done      bool
	buf       []byte
	rc        int
	rcSeen    bool
	pendingCR bool
}

func newMarkerFilter(out io.Writer, m *execMarkers) *markerFilter {
	return &markerFilter{
		out:   out,
		begin: []byte(m.begin + "\n"),
		rcPfx: []byte(m.rc),
	}
}

// normalize converts CRLF to LF across Write chunk boundaries: a chunk
// ending in a bare CR is held back until the next chunk reveals whether an
// LF follows.
func (f *markerFilter) normalize(p []byte) []byte {
	data := p
	if f.pendingCR {
		data = make([]byte, 0, len(p)+1)
		data = append(data, '\r')
		data = append(data, p...)
		f.pendingCR = false
	}
	if len(data) > 0 && data[len(data)-1] == '\r' {
		f.pendingCR = true
		data = data[:len(data)-1]
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
}

func (f *markerFilter) Write(p []byte) (int, error) {
	if f.done {
		return len(p), nil
	}

	f.buf = append(f.buf, f.normalize(p)...)

	if !f.started {
		idx := bytes.Index(f.buf, f.begin)
		if idx < 0 {
			// Keep only a tail that could still hold a split marker.
			if keep := len(f.begin) - 1; len(f.buf) > keep {
				f.buf = append(f.buf[:0:0], f.buf[len(f.buf)-keep:]...)
			}
			return len(p), nil
		}
		f.buf = append(f.buf[:0:0], f.buf[idx+len(f.begin):]...)
		f.started = true
	}

	// Scan for the rc marker. The remote wrapper prints "\n" + marker, so
	// the newline immediately preceding the marker is synthetic and must
	// not be emitted.
	if idx := bytes.Index(f.buf, f.rcPfx); idx >= 0 {
		rest := f.buf[idx+len(f.rcPfx):]
		end := bytes.Index(rest, []byte("__"))
		if end < 0 {
			// Exit code digits not fully arrived yet; flush what we
			// safely can and keep the rest buffered.
			return f.flushKeepingTail(p)
		}
		if rc, err := strconv.Atoi(string(rest[:end])); err == nil {
			f.rc = rc
			f.rcSeen = true
		}
		payload := f.buf[:idx]
		payload = bytes.TrimSuffix(payload, []byte("\n")) // synthetic newline
		if len(payload) > 0 {
			if _, err := f.out.Write(payload); err != nil {
				return len(p), err
			}
		}
		f.done = true
		f.buf = nil
		return len(p), nil
	}

	return f.flushKeepingTail(p)
}

// flushKeepingTail emits buffered output except a tail large enough to
// hold a split rc marker (plus its synthetic preceding newline).
func (f *markerFilter) flushKeepingTail(p []byte) (int, error) {
	keep := len(f.rcPfx) + 24 // marker prefix + room for "\n", digits, "__"
	if len(f.buf) <= keep {
		return len(p), nil
	}
	emit := f.buf[:len(f.buf)-keep]
	if _, err := f.out.Write(emit); err != nil {
		return len(p), err
	}
	f.buf = append(f.buf[:0:0], f.buf[len(f.buf)-keep:]...)
	return len(p), nil
}

// ExitCode returns the remote exit code (ok=false when the rc marker was
// never observed, e.g. the session died mid-command).
func (f *markerFilter) ExitCode() (int, bool) {
	return f.rc, f.rcSeen
}

func runExec(cmd *cobra.Command, args []string) error {
	sandboxRef, remoteArgv, err := splitExecArgs(args, cmd.ArgsLenAtDash())
	if err != nil {
		return err
	}

	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	sandboxID := sandboxRef
	if !looksLikeSandboxID(sandboxRef) {
		sandboxID, err = resolveSandboxIDByName(ctx, client, sandboxRef)
		if err != nil {
			return err
		}
	}

	sandbox, err := client.GetSandbox(ctx, sandboxID)
	if err != nil {
		if api.IsNotFound(err) {
			return fmt.Errorf("sandbox not found: %s", sandboxRef)
		}
		return fmt.Errorf("failed to get sandbox: %w", err)
	}

	// Same posture as connect: a service sandbox that is still
	// PROVISIONING is exec-able for bootstrap (the websocket terminal
	// rides pods/exec, which doesn't need a Ready pod).
	if !isRunningStatus(sandbox.Status) {
		bootstrapOK := sandbox.ServiceMode &&
			isProvisioningStatus(sandbox.Status) &&
			sandbox.Connectivity.WebsocketTerminal
		if !bootstrapOK {
			return fmt.Errorf("sandbox is not running (status: %s)", sandbox.Status)
		}
	}

	wsInfo, err := client.GetTerminalWebSocket(ctx, sandbox.ID)
	if err != nil {
		return fmt.Errorf("failed to get terminal connection: %w", err)
	}

	term, err := terminal.NewSocketIOTerminal(wsInfo.URL, wsInfo.Token, sandbox.ID)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer term.Close()

	markers, err := newExecMarkers()
	if err != nil {
		return err
	}

	remote := buildRemoteCommand(markers, shellQuote(remoteArgv))
	stdin := newOnceThenBlockReader(remote)
	defer stdin.release()
	filter := newMarkerFilter(os.Stdout, markers)

	if err := term.Run(stdin, filter); err != nil {
		return fmt.Errorf("exec failed: %w", err)
	}

	rc, ok := filter.ExitCode()
	if !ok {
		return fmt.Errorf("session ended before the command reported an exit code")
	}
	if rc != 0 {
		_ = term.Close()
		os.Exit(rc)
	}
	return nil
}
