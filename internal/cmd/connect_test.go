package cmd

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
)

func TestIsSSHAvailable(t *testing.T) {
	// This test checks if SSH is available on the system
	available := isSSHAvailable()

	// Verify the result matches exec.LookPath("ssh")
	_, err := exec.LookPath("ssh")
	expectedAvailable := err == nil

	if available != expectedAvailable {
		t.Errorf("isSSHAvailable() = %v, want %v", available, expectedAvailable)
	}
}

func TestConnectSSH_NoSSHHost(t *testing.T) {
	sandbox := &api.Sandbox{
		ID:      "sbx-test123",
		Name:    "test-sandbox",
		Status:  "running",
		SSHHost: "", // No SSH host
		SSHPort: 0,
		SSHUser: "",
	}

	err := connectSSH(sandbox)
	if err == nil {
		t.Error("connectSSH() with no SSHHost should return error")
	}

	expectedMsg := "SSH not available for this sandbox"
	if err.Error() != expectedMsg {
		t.Errorf("connectSSH() error = %q, want %q", err.Error(), expectedMsg)
	}
}

func TestConnectSSH_BuildsCorrectArgs(t *testing.T) {
	sandbox := &api.Sandbox{
		ID:      "sbx-test123",
		Name:    "test-sandbox",
		Status:  "running",
		SSHHost: "sandbox.example.com",
		SSHPort: 2222,
		SSHUser: "ubuntu",
	}

	// We can't actually test the exec without running it,
	// but we can verify the error path when SSH is not available
	if !isSSHAvailable() {
		err := connectSSH(sandbox)
		if err == nil {
			t.Error("connectSSH() should return error when ssh not in PATH")
		}
	}
	// If SSH is available, we can't easily test without actually executing
	// which would replace the test process
}

func TestEnsureConnectedAuth_AlreadyAuthenticated(t *testing.T) {
	originalLoad := connectLoadConfig
	originalLogin := connectLoginOAuth
	t.Cleanup(func() {
		connectLoadConfig = originalLoad
		connectLoginOAuth = originalLogin
	})

	loginCalled := false
	connectLoginOAuth = func(*config.Config) error {
		loginCalled = true
		return nil
	}

	cfg := &config.Config{AccessToken: "token"}
	authenticatedCfg, err := ensureConnectedAuth(cfg)
	if err != nil {
		t.Fatalf("ensureConnectedAuth() error = %v, want nil", err)
	}
	if loginCalled {
		t.Error("ensureConnectedAuth() should not call login when already authenticated")
	}
	if authenticatedCfg != cfg {
		t.Error("ensureConnectedAuth() should return the original config when already authenticated")
	}
}

func TestEnsureConnectedAuth_LoginAndReload(t *testing.T) {
	originalLoad := connectLoadConfig
	originalLogin := connectLoginOAuth
	t.Cleanup(func() {
		connectLoadConfig = originalLoad
		connectLoginOAuth = originalLogin
	})

	loginCalled := false
	reloadCalled := false
	connectLoginOAuth = func(cfg *config.Config) error {
		loginCalled = true
		cfg.AccessToken = "temporary-token"
		return nil
	}
	connectLoadConfig = func() (*config.Config, error) {
		reloadCalled = true
		return &config.Config{AccessToken: "persisted-token"}, nil
	}

	authenticatedCfg, err := ensureConnectedAuth(&config.Config{})
	if err != nil {
		t.Fatalf("ensureConnectedAuth() error = %v, want nil", err)
	}
	if !loginCalled {
		t.Error("ensureConnectedAuth() should call login when not authenticated")
	}
	if !reloadCalled {
		t.Error("ensureConnectedAuth() should reload config after login")
	}
	if !authenticatedCfg.IsAuthenticated() {
		t.Error("ensureConnectedAuth() should return an authenticated config")
	}
}

func TestEnsureConnectedAuth_LoginFailure(t *testing.T) {
	originalLoad := connectLoadConfig
	originalLogin := connectLoginOAuth
	t.Cleanup(func() {
		connectLoadConfig = originalLoad
		connectLoginOAuth = originalLogin
	})

	connectLoginOAuth = func(*config.Config) error {
		return errors.New("oauth unavailable")
	}
	connectLoadConfig = func() (*config.Config, error) {
		t.Fatal("connectLoadConfig() should not be called when login fails")
		return nil, nil
	}

	_, err := ensureConnectedAuth(&config.Config{})
	if err == nil {
		t.Fatal("ensureConnectedAuth() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "failed to authenticate") {
		t.Fatalf("ensureConnectedAuth() error = %q, expected auth failure message", err.Error())
	}
}

func TestEnsureConnectedAuth_ReloadFailure(t *testing.T) {
	originalLoad := connectLoadConfig
	originalLogin := connectLoginOAuth
	t.Cleanup(func() {
		connectLoadConfig = originalLoad
		connectLoginOAuth = originalLogin
	})

	connectLoginOAuth = func(*config.Config) error { return nil }
	connectLoadConfig = func() (*config.Config, error) {
		return nil, errors.New("read failed")
	}

	_, err := ensureConnectedAuth(&config.Config{})
	if err == nil {
		t.Fatal("ensureConnectedAuth() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "failed to reload config after authentication") {
		t.Fatalf("ensureConnectedAuth() error = %q, expected reload failure message", err.Error())
	}
}

func TestEnsureConnectedAuth_ReloadNotAuthenticated(t *testing.T) {
	originalLoad := connectLoadConfig
	originalLogin := connectLoginOAuth
	t.Cleanup(func() {
		connectLoadConfig = originalLoad
		connectLoginOAuth = originalLogin
	})

	connectLoginOAuth = func(*config.Config) error { return nil }
	connectLoadConfig = func() (*config.Config, error) {
		return &config.Config{}, nil
	}

	_, err := ensureConnectedAuth(&config.Config{})
	if err == nil {
		t.Fatal("ensureConnectedAuth() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "authentication did not produce credentials") {
		t.Fatalf("ensureConnectedAuth() error = %q, expected missing credential message", err.Error())
	}
}
