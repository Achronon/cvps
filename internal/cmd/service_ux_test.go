package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
)

func setupTestEnv(t *testing.T, server *httptest.Server) {
	t.Helper()

	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", oldHome) })

	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(oldWd) })

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	if err := config.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
}

func resetUpFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		upName = ""
		upCPU = 0
		upMemory = 0
		upStorage = 0
		upDetach = false
		upDedicatedIP = false
		upAcceptAup = false
		upProfile = ""
	})
}

func TestRunUp_ServiceProfileSkipsWaitAndSavesContext(t *testing.T) {
	resetUpFlags(t)

	statusPolled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/runtime-profiles/slug/cortex":
			json.NewEncoder(w).Encode(api.RuntimeProfile{
				ID:   "cortex-profile-id",
				Slug: "cortex",
				Mode: "SERVICE",
			})
		case "/sandboxes":
			var req api.CreateSandboxRequest
			json.NewDecoder(r.Body).Decode(&req)
			if req.RuntimeProfileID != "cortex-profile-id" {
				t.Errorf("expected runtimeProfileId cortex-profile-id, got %q", req.RuntimeProfileID)
			}
			json.NewEncoder(w).Encode(api.Sandbox{
				ID:          "sbx-svc-1",
				Name:        req.Name,
				Status:      "PROVISIONING",
				ServiceMode: true,
			})
		case "/sandboxes/sbx-svc-1/status":
			statusPolled = true
			json.NewEncoder(w).Encode(map[string]string{"status": "PROVISIONING"})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupTestEnv(t, server)

	upName = "cortex-brain"
	upProfile = "cortex"

	if err := runUp(nil, nil); err != nil {
		t.Fatalf("runUp: %v", err)
	}

	if statusPolled {
		t.Error("service-mode up must not poll for RUNNING (readiness gates on model auth)")
	}

	// The advertised id-less next steps (cvps connect/status/logs) need local context.
	wd, _ := os.Getwd()
	data, err := os.ReadFile(filepath.Join(wd, ".cvps.yaml"))
	if err != nil {
		t.Fatalf("expected .cvps.yaml to be written before next steps: %v", err)
	}
	if !strings.Contains(string(data), "sbx-svc-1") {
		t.Errorf(".cvps.yaml missing sandbox id, got: %s", data)
	}
}

func TestRunUp_UnknownProfile(t *testing.T) {
	resetUpFlags(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	setupTestEnv(t, server)
	upProfile = "nope"

	err := runUp(nil, nil)
	if err == nil || !strings.Contains(err.Error(), "runtime profile not found: nope") {
		t.Fatalf("expected profile-not-found error, got: %v", err)
	}
}

func TestRunUp_UnsupportedRuntimeHint(t *testing.T) {
	resetUpFlags(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/runtime-profiles/slug/cortex":
			json.NewEncoder(w).Encode(api.RuntimeProfile{
				ID:   "cortex-profile-id",
				Slug: "cortex",
				Mode: "SERVICE",
			})
		case "/sandboxes":
			w.WriteHeader(http.StatusBadRequest)
			// Backend carries the machine code in `error`.
			json.NewEncoder(w).Encode(map[string]any{
				"statusCode": 400,
				"error":      "unsupported_runtime_for_create",
				"message":    "This runtime is not currently available in self-serve create flow.",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	setupTestEnv(t, server)
	upProfile = "cortex"

	err := runUp(nil, nil)
	if err == nil {
		t.Fatal("expected create to fail")
	}
	if !strings.Contains(err.Error(), "isn't enabled for self-serve creation") {
		t.Errorf("expected the gated-runtime hint, got: %v", err)
	}
}

func TestResolveConnectMethod_CapabilityAware(t *testing.T) {
	serviceSandbox := &api.Sandbox{
		ID:      "svc-1",
		SSHHost: "ssh.claudevps.com",
		SSHPort: 30022,
		SSHUser: "root",
	}
	serviceSandbox.Connectivity.SSHDirect = false
	serviceSandbox.Connectivity.WebsocketTerminal = true

	method, err := resolveConnectMethod("", serviceSandbox)
	if err != nil {
		t.Fatalf("resolveConnectMethod: %v", err)
	}
	if method != "websocket" {
		t.Errorf("expected websocket default for non-SSH runtime, got %s", method)
	}

	if _, err := resolveConnectMethod("ssh", serviceSandbox); err == nil ||
		!strings.Contains(err.Error(), "no SSH service") {
		t.Errorf("expected explicit ssh to be rejected for non-SSH runtime, got: %v", err)
	}

	sshSandbox := &api.Sandbox{ID: "sbx-1", SSHHost: "ssh.claudevps.com"}
	sshSandbox.Connectivity.SSHDirect = true
	if method, err := resolveConnectMethod("ssh", sshSandbox); err != nil || method != "ssh" {
		t.Errorf("expected ssh to remain available, got %s / %v", method, err)
	}
}

func TestRunConnect_ServiceBootstrapGate(t *testing.T) {
	cases := []struct {
		name          string
		sandbox       api.Sandbox
		wantGateError bool
	}{
		{
			name: "provisioning service sandbox passes the gate",
			sandbox: func() api.Sandbox {
				s := api.Sandbox{ID: "svc-1", Name: "brain", Status: "PROVISIONING", ServiceMode: true}
				s.Connectivity.WebsocketTerminal = true
				return s
			}(),
			wantGateError: false,
		},
		{
			name:          "provisioning interactive sandbox is rejected",
			sandbox:       api.Sandbox{ID: "sbx-1", Name: "dev", Status: "PROVISIONING"},
			wantGateError: true,
		},
		{
			name: "stopped service sandbox is rejected",
			sandbox: func() api.Sandbox {
				s := api.Sandbox{ID: "svc-1", Name: "brain", Status: "STOPPED", ServiceMode: true}
				s.Connectivity.WebsocketTerminal = true
				return s
			}(),
			wantGateError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/sandboxes/" + tc.sandbox.ID:
					json.NewEncoder(w).Encode(tc.sandbox)
				case "/sandboxes/" + tc.sandbox.ID + "/terminal":
					// Past the gate: fail the terminal-token call so the test ends
					// here without dialing a websocket.
					w.WriteHeader(http.StatusServiceUnavailable)
					json.NewEncoder(w).Encode(map[string]string{"message": "gate passed"})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			setupTestEnv(t, server)

			err := runConnect(nil, []string{tc.sandbox.ID})
			if err == nil {
				t.Fatal("expected an error in both cases (terminal call is stubbed to fail)")
			}

			gateError := strings.Contains(err.Error(), "sandbox is not running")
			if tc.wantGateError && !gateError {
				t.Errorf("expected the not-running gate error, got: %v", err)
			}
			if !tc.wantGateError && gateError {
				t.Errorf("service bootstrap should pass the gate, got: %v", err)
			}
		})
	}
}

func TestServiceHealth(t *testing.T) {
	if got := serviceHealth("RUNNING"); got != "healthy (ready)" {
		t.Errorf("RUNNING => %q", got)
	}
	if got := serviceHealth("PROVISIONING"); !strings.Contains(got, "awaiting readiness") {
		t.Errorf("PROVISIONING => %q", got)
	}
	if got := serviceHealth("ERROR"); got != "error" {
		t.Errorf("ERROR => %q", got)
	}
}

func TestRunLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sandboxes/svc-1/logs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("tailLines"); got != "500" {
			t.Errorf("expected tailLines=500, got %q", got)
		}
		json.NewEncoder(w).Encode(api.SandboxLogs{PodName: "svc-1-deploy-abc", Logs: "brain: ready\n"})
	}))
	defer server.Close()

	setupTestEnv(t, server)

	oldTail := logsTail
	logsTail = 500
	t.Cleanup(func() { logsTail = oldTail })

	if err := runLogs(nil, []string{"svc-1"}); err != nil {
		t.Fatalf("runLogs: %v", err)
	}
}

func TestRunStart(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sandboxes/sbx-svc-1/start" {
			t.Errorf("unexpected start request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(api.Sandbox{ID: "sbx-svc-1", Status: "PROVISIONING"})
	}))
	defer server.Close()

	setupTestEnv(t, server)
	if err := runStart(nil, []string{"sbx-svc-1"}); err != nil {
		t.Fatalf("runStart: %v", err)
	}
}

func TestRunStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/sandboxes/sbx-svc-1/stop" {
			t.Errorf("unexpected stop request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(api.Sandbox{ID: "sbx-svc-1", Status: "PROVISIONING"})
	}))
	defer server.Close()

	setupTestEnv(t, server)
	if err := runStop(nil, []string{"sbx-svc-1"}); err != nil {
		t.Fatalf("runStop: %v", err)
	}
}

func TestRunRestart_NotRunningHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sandboxes/svc-1/restart" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 400,
			"message":    "Cannot restart sandbox with status: PROVISIONING",
		})
	}))
	defer server.Close()

	setupTestEnv(t, server)

	err := runRestart(nil, []string{"svc-1"})
	if err == nil || !strings.Contains(err.Error(), "Only RUNNING sandboxes can be restarted") {
		t.Fatalf("expected restart hint, got: %v", err)
	}
}

func TestRunRestart_NoHintOnUnrelatedFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 500,
			"message":    "Internal server error",
		})
	}))
	defer server.Close()

	setupTestEnv(t, server)

	err := runRestart(nil, []string{"svc-1"})
	if err == nil {
		t.Fatal("expected restart to fail")
	}
	if strings.Contains(err.Error(), "Only RUNNING sandboxes can be restarted") {
		t.Errorf("bootstrap hint must not fire for unrelated failures, got: %v", err)
	}
}
