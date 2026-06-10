package cmd

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestSplitExecArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		lenAtDash   int
		wantSandbox string
		wantCmd     []string
		wantErr     bool
	}{
		{
			name:        "with dash separator",
			args:        []string{"cortex-brain", "ls", "/workspace"},
			lenAtDash:   1,
			wantSandbox: "cortex-brain",
			wantCmd:     []string{"ls", "/workspace"},
		},
		{
			name:        "without dash separator",
			args:        []string{"cortex-brain", "ls", "/workspace"},
			lenAtDash:   -1,
			wantSandbox: "cortex-brain",
			wantCmd:     []string{"ls", "/workspace"},
		},
		{
			name:      "two args before dash",
			args:      []string{"a", "b", "ls"},
			lenAtDash: 2,
			wantErr:   true,
		},
		{
			name:      "nothing after dash",
			args:      []string{"cortex-brain"},
			lenAtDash: 1,
			wantErr:   true,
		},
		{
			name:      "dash first (no sandbox)",
			args:      []string{"ls", "/workspace"},
			lenAtDash: 0,
			wantErr:   true,
		},
		{
			name:      "single arg without dash",
			args:      []string{"cortex-brain"},
			lenAtDash: -1,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sandbox, cmd, err := splitExecArgs(tt.args, tt.lenAtDash)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sandbox != tt.wantSandbox {
				t.Errorf("sandbox: got %q, want %q", sandbox, tt.wantSandbox)
			}
			if strings.Join(cmd, "\x00") != strings.Join(tt.wantCmd, "\x00") {
				t.Errorf("cmd: got %v, want %v", cmd, tt.wantCmd)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{name: "simple", argv: []string{"ls", "/workspace"}, want: "'ls' '/workspace'"},
		{name: "spaces", argv: []string{"echo", "hello world"}, want: "'echo' 'hello world'"},
		{name: "single quotes", argv: []string{"echo", "it's"}, want: `'echo' 'it'\''s'`},
		{name: "shell metacharacters stay inert", argv: []string{"echo", "$HOME; rm -rf /"}, want: `'echo' '$HOME; rm -rf /'`},
		{name: "empty arg survives", argv: []string{"printf", ""}, want: "'printf' ''"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellQuote(tt.argv); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestBuildRemoteCommandNeverContainsMarkerLiterals(t *testing.T) {
	m, err := newExecMarkers()
	if err != nil {
		t.Fatalf("newExecMarkers: %v", err)
	}

	line := buildRemoteCommand(m, shellQuote([]string{"ls", "/workspace"}))

	// The typed line is echoed verbatim by the PTY before stty -echo takes
	// effect; if it contained a full marker the filter would mis-slice.
	if strings.Contains(line, m.begin) {
		t.Error("remote command contains the literal begin marker")
	}
	if strings.Contains(line, m.rc) {
		t.Error("remote command contains the literal rc marker prefix")
	}
	if !strings.HasSuffix(line, "\n") {
		t.Error("remote command must end with a newline to execute")
	}
	if !strings.Contains(line, "</dev/null") {
		t.Error("user command stdin must be redirected from /dev/null so stdin-reading commands cannot hang")
	}
	if !strings.Contains(line, "( "+shellQuote([]string{"ls", "/workspace"})+" )") {
		t.Error("user command must run in a subshell so exit/exec builtins cannot kill the wrapper before the rc marker prints")
	}
	if !strings.HasPrefix(line, "set +e ") {
		t.Error("wrapper must disable errexit first so a non-zero subshell cannot kill it before the rc marker prints")
	}
}

// feedFilter writes the stream to the filter in the given chunk size.
func feedFilter(t *testing.T, f *markerFilter, stream string, chunk int) {
	t.Helper()
	for i := 0; i < len(stream); i += chunk {
		end := i + chunk
		if end > len(stream) {
			end = len(stream)
		}
		if _, err := f.Write([]byte(stream[i:end])); err != nil {
			t.Fatalf("filter write failed: %v", err)
		}
	}
}

func TestMarkerFilter(t *testing.T) {
	m := &execMarkers{
		nonce: "deadbeef",
		begin: "__CVPS_EXEC_BEGIN_deadbeef__",
		rc:    "__CVPS_EXEC_RC_deadbeef_",
	}

	// Simulated PTY stream: prompt + echoed command line (split-quoted, so
	// no full marker), then marker output with CRLF endings.
	echoed := "bash-5.1$ stty -echo 2>/dev/null; printf '%s%s\\n' '__CVPS_EXEC_BEGIN_' 'deadbeef__'; 'ls' '/workspace'; ...\r\n"
	stream := echoed +
		"__CVPS_EXEC_BEGIN_deadbeef__\r\n" +
		"file-a\r\nfile-b\r\n" +
		"\r\n__CVPS_EXEC_RC_deadbeef_0__\r\n" +
		"leftover prompt noise\r\n"

	for _, chunk := range []int{1, 3, 7, 1024} {
		t.Run(fmt.Sprintf("chunk-%d", chunk), func(t *testing.T) {
			var out bytes.Buffer
			f := newMarkerFilter(&out, m)
			feedFilter(t, f, stream, chunk)

			if got, want := out.String(), "file-a\nfile-b\n"; got != want {
				t.Errorf("output: got %q, want %q", got, want)
			}
			rc, ok := f.ExitCode()
			if !ok || rc != 0 {
				t.Errorf("exit code: got (%d, %v), want (0, true)", rc, ok)
			}
		})
	}
}

func TestMarkerFilterNonZeroExit(t *testing.T) {
	m := &execMarkers{
		nonce: "deadbeef",
		begin: "__CVPS_EXEC_BEGIN_deadbeef__",
		rc:    "__CVPS_EXEC_RC_deadbeef_",
	}

	stream := "__CVPS_EXEC_BEGIN_deadbeef__\r\n" +
		"ls: cannot access '/nope': No such file or directory\r\n" +
		"\r\n__CVPS_EXEC_RC_deadbeef_2__\r\n"

	var out bytes.Buffer
	f := newMarkerFilter(&out, m)
	feedFilter(t, f, stream, 5)

	rc, ok := f.ExitCode()
	if !ok || rc != 2 {
		t.Errorf("exit code: got (%d, %v), want (2, true)", rc, ok)
	}
	if !strings.Contains(out.String(), "No such file or directory") {
		t.Errorf("expected error output to pass through, got %q", out.String())
	}
}

func TestMarkerFilterOutputWithoutTrailingNewline(t *testing.T) {
	m := &execMarkers{
		nonce: "deadbeef",
		begin: "__CVPS_EXEC_BEGIN_deadbeef__",
		rc:    "__CVPS_EXEC_RC_deadbeef_",
	}

	// printf without newline: the wrapper's synthetic "\n" before the rc
	// marker must be swallowed, leaving the output without one.
	stream := "__CVPS_EXEC_BEGIN_deadbeef__\r\n" +
		"no-newline" +
		"\r\n__CVPS_EXEC_RC_deadbeef_0__\r\n"

	var out bytes.Buffer
	f := newMarkerFilter(&out, m)
	feedFilter(t, f, stream, 4)

	if got := out.String(); got != "no-newline" {
		t.Errorf("output: got %q, want %q", got, "no-newline")
	}
}

func TestMarkerFilterMissingRC(t *testing.T) {
	m := &execMarkers{
		nonce: "deadbeef",
		begin: "__CVPS_EXEC_BEGIN_deadbeef__",
		rc:    "__CVPS_EXEC_RC_deadbeef_",
	}

	var out bytes.Buffer
	f := newMarkerFilter(&out, m)
	feedFilter(t, f, "__CVPS_EXEC_BEGIN_deadbeef__\r\npartial output\r\n", 8)

	if _, ok := f.ExitCode(); ok {
		t.Error("exit code should not be reported when the rc marker never arrived")
	}
}

func TestMarkerFilterIgnoresEverythingBeforeBegin(t *testing.T) {
	m := &execMarkers{
		nonce: "deadbeef",
		begin: "__CVPS_EXEC_BEGIN_deadbeef__",
		rc:    "__CVPS_EXEC_RC_deadbeef_",
	}

	// Banner noise that includes marker-LIKE fragments (but not the full
	// begin marker followed by newline).
	stream := "welcome\r\n__CVPS_EXEC_BEGIN_othernonce__\r\n" +
		"__CVPS_EXEC_BEGIN_deadbeef__\r\n" +
		"real\r\n" +
		"\r\n__CVPS_EXEC_RC_deadbeef_0__\r\n"

	var out bytes.Buffer
	f := newMarkerFilter(&out, m)
	feedFilter(t, f, stream, 6)

	if got := out.String(); got != "real\n" {
		t.Errorf("output: got %q, want %q", got, "real\n")
	}
}

func TestMarkerFilterStreamsPromptTailsImmediately(t *testing.T) {
	m := &execMarkers{
		nonce: "deadbeef",
		begin: "__CVPS_EXEC_BEGIN_deadbeef__",
		rc:    "__CVPS_EXEC_RC_deadbeef_",
	}

	var out bytes.Buffer
	f := newMarkerFilter(&out, m)

	// A long-running command prints a prompt WITHOUT trailing newline and
	// then waits (e.g. device-auth printing a code). The prompt must be
	// fully visible immediately, not held back waiting for the rc marker.
	feedFilter(t, f, "__CVPS_EXEC_BEGIN_deadbeef__\r\nEnter code ABCD-1234 at https://example.com: ", 9)
	if got := out.String(); got != "Enter code ABCD-1234 at https://example.com: " {
		t.Errorf("prompt tail must be streamed immediately, got %q", got)
	}

	// A partial rc marker (with its synthetic newline) must stay buffered...
	feedFilter(t, f, "\r\n__CVPS_EXEC_RC_dead", 5)
	if got := out.String(); got != "Enter code ABCD-1234 at https://example.com: " {
		t.Errorf("partial rc marker must stay buffered, got %q", got)
	}

	// ...and complete into the exit code without emitting marker bytes.
	feedFilter(t, f, "beef_0__\r\n", 3)
	rc, ok := f.ExitCode()
	if !ok || rc != 0 {
		t.Errorf("exit code: got (%d, %v), want (0, true)", rc, ok)
	}
	if got := out.String(); got != "Enter code ABCD-1234 at https://example.com: " {
		t.Errorf("marker bytes leaked into output: %q", got)
	}
}

func TestMarkerFilterFalseMarkerPrefixFlushesWhenBroken(t *testing.T) {
	m := &execMarkers{
		nonce: "deadbeef",
		begin: "__CVPS_EXEC_BEGIN_deadbeef__",
		rc:    "__CVPS_EXEC_RC_deadbeef_",
	}

	var out bytes.Buffer
	f := newMarkerFilter(&out, m)

	// Output that starts like the rc marker but diverges must be flushed
	// once the divergence is visible.
	feedFilter(t, f, "__CVPS_EXEC_BEGIN_deadbeef__\r\nx\n__CVPS_EXEC_RX", 7)
	feedFilter(t, f, " not a marker\n", 100)
	feedFilter(t, f, "\n__CVPS_EXEC_RC_deadbeef_3__\n", 100)

	rc, ok := f.ExitCode()
	if !ok || rc != 3 {
		t.Errorf("exit code: got (%d, %v), want (3, true)", rc, ok)
	}
	if got, want := out.String(), "x\n__CVPS_EXEC_RX not a marker\n"; got != want {
		t.Errorf("output: got %q, want %q", got, want)
	}
}
