package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestResolveSandboxIDForConnect_MutuallyExclusiveSelectors(t *testing.T) {
	_, err := resolveSandboxIDForConnect(context.Background(), nil, []string{"sbx-123"}, "openclaw")
	if err == nil {
		t.Fatal("resolveSandboxIDForConnect() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "either a sandbox ID argument or --name") {
		t.Fatalf("resolveSandboxIDForConnect() error = %q, expected selector conflict message", err.Error())
	}
}

func TestResolveSandboxIDByName_SingleMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sandboxes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "cmlt6ghp0000101dyq5j3d5xu", "name": "openclaw", "status": "RUNNING"},
			},
			"total": 1,
			"page":  1,
			"limit": 100,
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "cvps_test")
	sandboxID, err := resolveSandboxIDByName(context.Background(), client, "OpenClaw")
	if err != nil {
		t.Fatalf("resolveSandboxIDByName() error = %v, want nil", err)
	}
	if sandboxID != "cmlt6ghp0000101dyq5j3d5xu" {
		t.Fatalf("resolveSandboxIDByName() = %q, want %q", sandboxID, "cmlt6ghp0000101dyq5j3d5xu")
	}
}

func TestResolveSandboxIDByName_NoMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  []map[string]any{},
			"total": 0,
			"page":  1,
			"limit": 100,
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "cvps_test")
	_, err := resolveSandboxIDByName(context.Background(), client, "openclaw")
	if err == nil {
		t.Fatal("resolveSandboxIDByName() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "Run 'cvps status --all'") {
		t.Fatalf("resolveSandboxIDByName() error = %q, expected status hint", err.Error())
	}
}

func TestResolveSandboxIDByName_Ambiguous(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "cmlt6ghp0000101dyq5j3d5xu", "name": "openclaw", "status": "RUNNING"},
				{"id": "cmlt6ghp0000101dyq5j3d5xv", "name": "OpenClaw", "status": "RUNNING"},
			},
			"total": 2,
			"page":  1,
			"limit": 100,
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "cvps_test")
	_, err := resolveSandboxIDByName(context.Background(), client, "openclaw")
	if err == nil {
		t.Fatal("resolveSandboxIDByName() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("resolveSandboxIDByName() error = %q, expected ambiguity message", err.Error())
	}
	if !strings.Contains(err.Error(), "cmlt6ghp0000101dyq5j3d5xu") {
		t.Fatalf("resolveSandboxIDByName() error = %q, expected first candidate", err.Error())
	}
	if !strings.Contains(err.Error(), "cmlt6ghp0000101dyq5j3d5xv") {
		t.Fatalf("resolveSandboxIDByName() error = %q, expected second candidate", err.Error())
	}
}

func TestResolveConnectMethod(t *testing.T) {
	tests := []struct {
		name     string
		request  string
		sandbox  *api.Sandbox
		want     string
		wantErr  bool
		errMatch string
	}{
		{
			name:     "auto without ssh host returns actionable error",
			request:  "",
			sandbox:  &api.Sandbox{ID: "sbx-abc123", SSHHost: ""},
			wantErr:  true,
			errMatch: "SSH connection is not available",
		},
		{
			name:     "websocket is unsupported",
			request:  "websocket",
			sandbox:  &api.Sandbox{ID: "sbx-abc123", SSHHost: "sbx.example.com"},
			wantErr:  true,
			errMatch: "unsupported",
		},
		{
			name:     "ssh without host errors",
			request:  "ssh",
			sandbox:  &api.Sandbox{ID: "sbx-abc123", SSHHost: ""},
			wantErr:  true,
			errMatch: "SSH connection is not available",
		},
		{
			name:     "unknown method errors",
			request:  "telnet",
			sandbox:  &api.Sandbox{ID: "sbx-abc123", SSHHost: "sbx.example.com"},
			wantErr:  true,
			errMatch: "unknown connection method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveConnectMethod(tt.request, tt.sandbox)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveConnectMethod() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if !strings.Contains(err.Error(), tt.errMatch) {
					t.Fatalf("resolveConnectMethod() error = %q, expected %q", err.Error(), tt.errMatch)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("resolveConnectMethod() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLooksLikeSandboxID(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "sbx-abc123", want: true},
		{value: "cmlt6ghp0000101dyq5j3d5xu", want: true},
		{value: "openclaw", want: false},
		{value: "   ", want: false},
	}

	for _, tt := range tests {
		if got := looksLikeSandboxID(tt.value); got != tt.want {
			t.Fatalf("looksLikeSandboxID(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}
