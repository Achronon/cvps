package cmd

import (
	"strings"
	"testing"
)

func TestValidateRemotePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "simple absolute path", input: "/workspace/.codex/auth.json", want: "/workspace/.codex/auth.json"},
		{name: "duplicate slashes cleaned", input: "/workspace//file.txt", want: "/workspace/file.txt"},
		{name: "relative path rejected", input: "workspace/file.txt", wantErr: true},
		{name: "traversal rejected", input: "/workspace/../etc/passwd", wantErr: true},
		{name: "bare root rejected", input: "/", wantErr: true},
		{name: "empty rejected", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateRemotePath(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGuardFileContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "normal json", content: `{"token": "abc"}`},
		{name: "multi-line text", content: "line1\nline2\n"},
		{name: "nul byte rejected", content: "abc\x00def", wantErr: "NUL"},
		{name: "heredoc delimiter line rejected", content: "before\nFILECONTENT\nafter", wantErr: "FILECONTENT"},
		{name: "heredoc delimiter with CR rejected", content: "before\nFILECONTENT\r\nafter", wantErr: "FILECONTENT"},
		{name: "delimiter as substring is fine", content: "xFILECONTENTx\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := guardFileContent([]byte(tt.content))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestGuardFileContentSizeCap(t *testing.T) {
	atCap := make([]byte, maxFilePutSize)
	for i := range atCap {
		atCap[i] = 'a'
	}
	if err := guardFileContent(atCap); err != nil {
		t.Errorf("content at the cap should pass, got %v", err)
	}
	if err := guardFileContent(append(atCap, 'a')); err == nil {
		t.Error("content over the cap should be rejected")
	}
}
