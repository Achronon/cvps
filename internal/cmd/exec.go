package cmd

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"

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
// marker (only the printf OUTPUT does). The user command runs in a
// SUBSHELL with stdin from /dev/null: the subshell isolates shell
// builtins that would otherwise kill the wrapper before the rc marker
// prints (exit 7, exec false), and the /dev/null stdin prevents
// stdin-reading commands (cat, grep with no files, prompts) from
// hanging forever since cvps exec sends no further input. set +e guards
// against a remote shell with errexit enabled (e.g. from .bashrc):
// without it a non-zero subshell would kill the wrapper before the rc
// marker prints.
func buildRemoteCommand(m *execMarkers, userCmd string) string {
	return fmt.Sprintf(
		"set +e 2>/dev/null; stty -echo 2>/dev/null; printf '%%s%%s\\n' '__CVPS_EXEC_BEGIN_' '%s__'; ( %s ) </dev/null; __cvps_rc=$?; printf '\\n%%s%%s%%d__\\n' '__CVPS_EXEC_RC_' '%s_' \"$__cvps_rc\"; exit \"$__cvps_rc\"\n",
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
			// Marker started but the exit-code digits have not fully
			// arrived: flush everything before the (synthetic) newline
			// preceding the marker, keep the marker bytes buffered.
			keepFrom := idx
			if keepFrom > 0 && f.buf[keepFrom-1] == '\n' {
				keepFrom--
			}
			return f.flushUpTo(p, keepFrom)
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

	// No marker yet: stream everything except the longest buffer suffix
	// that could still grow into "\n"+rcPfx. Anything else (e.g. an
	// interactive prompt tail without a trailing newline) is flushed
	// immediately - cvps exec advertises streamed output and a
	// long-waiting command's last line must not stay hidden.
	return f.flushUpTo(p, len(f.buf)-f.markerOverlap())
}

// markerOverlap returns the length of the longest suffix of the buffer
// that is a prefix of "\n"+rcPfx (i.e. bytes that may still turn out to
// be a split rc marker and must stay buffered).
func (f *markerFilter) markerOverlap() int {
	pattern := append([]byte("\n"), f.rcPfx...)
	max := len(pattern) - 1
	if max > len(f.buf) {
		max = len(f.buf)
	}
	for k := max; k > 0; k-- {
		if bytes.Equal(f.buf[len(f.buf)-k:], pattern[:k]) {
			return k
		}
	}
	return 0
}

// flushUpTo emits buf[:n] and keeps the rest buffered.
func (f *markerFilter) flushUpTo(p []byte, n int) (int, error) {
	if n <= 0 {
		return len(p), nil
	}
	if _, err := f.out.Write(f.buf[:n]); err != nil {
		return len(p), err
	}
	f.buf = append(f.buf[:0:0], f.buf[n:]...)
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
	if err != nil && api.IsNotFound(err) && sandboxID == sandboxRef {
		// The ref LOOKED like a sandbox id but none exists - it may be a
		// sandbox NAME that merely matches the id shape (e.g. sbx-prod).
		if nameID, nameErr := resolveSandboxIDByName(ctx, client, sandboxRef); nameErr == nil {
			sandboxID = nameID
			sandbox, err = client.GetSandbox(ctx, sandboxID)
		}
	}
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
		if errors.Is(err, syscall.EPIPE) {
			// The consumer of our piped stdout exited early (e.g.
			// `cvps exec sbx -- yes | head -n1`). Mirror shell SIGPIPE
			// semantics: stop the session, exit 141.
			_ = term.Close()
			os.Exit(141)
		}
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
