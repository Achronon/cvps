package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretKeyPattern(t *testing.T) {
	valid := []string{"A", "TELEGRAM_BOT_TOKEN", "API_KEY_2", "X_1_Y"}
	for _, key := range valid {
		if !secretKeyPattern.MatchString(key) {
			t.Errorf("Expected %q to be a valid key", key)
		}
	}

	invalid := []string{"", "lower", "1STARTS_WITH_DIGIT", "_UNDERSCORE_FIRST", "HAS-DASH", "HAS SPACE", "has_lower"}
	for _, key := range invalid {
		if secretKeyPattern.MatchString(key) {
			t.Errorf("Expected %q to be an invalid key", key)
		}
	}
}

func TestIsValidSecretCategory(t *testing.T) {
	for _, c := range []string{"API_KEY", "ACCESS_TOKEN", "DATABASE_CREDENTIAL", "OTHER"} {
		if !isValidSecretCategory(c) {
			t.Errorf("Expected %q to be valid", c)
		}
	}
	for _, c := range []string{"", "api_key", "TOKEN", "PASSWORD"} {
		if isValidSecretCategory(c) {
			t.Errorf("Expected %q to be invalid", c)
		}
	}
}

func TestTrimSingleTrailingNewline(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"value\n", "value"},
		{"value\r\n", "value"},
		{"value", "value"},
		{"value\n\n", "value\n"}, // only ONE trailing newline is trimmed
		{"multi\nline\n", "multi\nline"},
		{"", ""},
		{"\n", ""},
	}
	for _, c := range cases {
		if got := trimSingleTrailingNewline(c.in); got != c.want {
			t.Errorf("trimSingleTrailingNewline(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestReadSecretValueFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "value.txt")
	if err := os.WriteFile(path, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	value, err := readSecretValue(path, os.Stdin, "KEY")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if value != "file-secret" {
		t.Errorf("Expected 'file-secret', got %q", value)
	}
}

func TestReadSecretValueFromFileMissing(t *testing.T) {
	_, err := readSecretValue(filepath.Join(t.TempDir(), "missing"), os.Stdin, "KEY")
	if err == nil {
		t.Fatal("Expected error for missing file")
	}
}

func TestReadSecretValueFromPipedStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	go func() {
		w.WriteString("piped-secret\n")
		w.Close()
	}()

	value, err := readSecretValue("", r, "KEY")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if value != "piped-secret" {
		t.Errorf("Expected 'piped-secret', got %q", value)
	}
}

func TestReadSecretValueExplicitStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	go func() {
		w.WriteString("dash-secret")
		w.Close()
	}()

	value, err := readSecretValue("-", r, "KEY")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if value != "dash-secret" {
		t.Errorf("Expected 'dash-secret', got %q", value)
	}
}

func TestSecretCommandRegistered(t *testing.T) {
	names := map[string]bool{}
	for _, c := range secretCmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"create", "list", "rm"} {
		if !names[want] {
			t.Errorf("Expected 'cvps secret %s' to be registered", want)
		}
	}
}
