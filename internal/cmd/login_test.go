package cmd

import (
	"strings"
	"testing"
)

func TestReadTokenFromStdin(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "token with trailing newline", input: "cvps_token_abc\n", want: "cvps_token_abc"},
		{name: "token without newline", input: "cvps_token_abc", want: "cvps_token_abc"},
		{name: "token with surrounding whitespace", input: "  cvps_token_abc  \n", want: "cvps_token_abc"},
		{name: "only first line is read", input: "cvps_token_abc\nleftover\n", want: "cvps_token_abc"},
		{name: "empty stdin", input: "", wantErr: true},
		{name: "blank line", input: "\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readTokenFromStdin(strings.NewReader(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
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

func TestReadOtpCodeFromStdin(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "code with trailing newline", input: "ABCD-2345\n", want: "ABCD-2345"},
		{name: "code without newline", input: "ABCD-2345", want: "ABCD-2345"},
		{name: "code with surrounding whitespace", input: "  ABCD-2345  \n", want: "ABCD-2345"},
		{name: "only first line is read", input: "ABCD-2345\nleftover\n", want: "ABCD-2345"},
		{name: "empty stdin", input: "", wantErr: true},
		{name: "blank line", input: "\n", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readOtpCodeFromStdin(strings.NewReader(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
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
