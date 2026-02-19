package mutagen

import (
	"os/exec"
	"testing"
)

func TestIsInstalled(t *testing.T) {
	// This test will pass or fail depending on whether mutagen is actually installed
	// We're just testing that the function doesn't panic
	result := IsInstalled()

	// Verify by checking directly
	_, err := exec.LookPath("mutagen")
	expected := err == nil

	if result != expected {
		t.Errorf("IsInstalled() = %v, expected %v", result, expected)
	}
}

func TestParseSessionIDFromOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name:     "valid output with session ID",
			output:   "Created session session_abc123xyz\n",
			expected: "session_abc123xyz",
		},
		{
			name:     "output with multiple lines",
			output:   "Initializing...\nCreated session session_xyz789\nSession started\n",
			expected: "session_xyz789",
		},
		{
			name:     "no session ID in output",
			output:   "Some other output\n",
			expected: "",
		},
		{
			name:     "empty output",
			output:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSessionIDFromOutput(tt.output)
			if result != tt.expected {
				t.Errorf("parseSessionIDFromOutput() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestSessionConfig(t *testing.T) {
	cfg := SessionConfig{
		Name:       "test-session",
		LocalPath:  "/local/path",
		RemoteHost: "user@example.com",
		RemotePort: 2222,
		RemotePath: "/remote/path",
		Ignores:    []string{"node_modules/", ".git/"},
		OneWay:     "local-to-remote",
		Verbose:    true,
	}

	if cfg.Name != "test-session" {
		t.Errorf("Expected Name test-session, got %s", cfg.Name)
	}
	if cfg.RemotePort != 2222 {
		t.Errorf("Expected RemotePort 2222, got %d", cfg.RemotePort)
	}
	if len(cfg.Ignores) != 2 {
		t.Errorf("Expected 2 ignore patterns, got %d", len(cfg.Ignores))
	}
}

func TestSession(t *testing.T) {
	session := &Session{
		ID:   "session_123",
		Name: "cvps-test",
		config: SessionConfig{
			Name:      "cvps-test",
			LocalPath: "/test",
		},
	}

	if session.ID != "session_123" {
		t.Errorf("Expected ID session_123, got %s", session.ID)
	}
	if session.Name != "cvps-test" {
		t.Errorf("Expected Name cvps-test, got %s", session.Name)
	}
}

func TestSessionStatus(t *testing.T) {
	status := &SessionStatus{
		Status:     "watching",
		LocalPath:  "/local",
		RemotePath: "/remote",
		Conflicts:  0,
	}

	if status.Status != "watching" {
		t.Errorf("Expected Status watching, got %s", status.Status)
	}
	if status.Conflicts != 0 {
		t.Errorf("Expected Conflicts 0, got %d", status.Conflicts)
	}
}

// Note: CreateSession, GetSessionStatus, TerminateSession, and ListSessions
// are integration tests that require Mutagen to be installed and would need
// a real or mocked Mutagen setup. These would be better suited for integration
// tests rather than unit tests.
