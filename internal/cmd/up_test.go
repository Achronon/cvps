package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
)

func TestRunUp_NotAuthenticated(t *testing.T) {
	// Create temp dir for config
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	// Create empty config (no auth)
	cfg := config.DefaultConfig()
	cfg.APIKey = ""
	cfg.AccessToken = ""
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Run command
	err := runUp(nil, nil)
	if err == nil {
		t.Fatal("Expected error for unauthenticated request")
	}
	if err.Error() != "not logged in. Run 'cvps login' or set CVPS_API_TOKEN" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestRunUp_WithDefaults(t *testing.T) {
	// Create temp dir for config and context
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	// Change to tmpDir for .cvps.yaml
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Mock API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes":
			if r.Method != "POST" {
				t.Errorf("Expected POST, got %s", r.Method)
			}

			var req api.CreateSandboxRequest
			json.NewDecoder(r.Body).Decode(&req)

			// Verify defaults are applied
			if req.CPUCores != 1 {
				t.Errorf("Expected CPU 1, got %d", req.CPUCores)
			}
			if req.MemoryGB != 2 {
				t.Errorf("Expected Memory 2, got %d", req.MemoryGB)
			}
			if req.StorageGB != 5 {
				t.Errorf("Expected Storage 5, got %d", req.StorageGB)
			}

			resp := api.Sandbox{
				ID:        "sbx-test-123",
				Name:      req.Name,
				Status:    "provisioning",
				CPUCores:  req.CPUCores,
				MemoryGB:  req.MemoryGB,
				StorageGB: req.StorageGB,
			}
			json.NewEncoder(w).Encode(resp)

		case "/sandboxes/sbx-test-123/status":
			// Real backend shape: {status} only, Prisma enum casing.
			json.NewEncoder(w).Encode(map[string]string{"status": "RUNNING"})

		case "/sandboxes/sbx-test-123":
			resp := api.Sandbox{
				ID:        "sbx-test-123",
				Name:      "sandbox-test",
				Status:    "RUNNING",
				CPUCores:  1,
				MemoryGB:  2,
				StorageGB: 5,
				SSHHost:   "test.claudevps.com",
				SSHPort:   22,
				SSHUser:   "sandbox",
			}
			json.NewEncoder(w).Encode(resp)

		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Create config with auth
	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Set flags
	upName = "sandbox-test"
	upCPU = 0 // Use defaults
	upMemory = 0
	upStorage = 0
	upDetach = false
	upDedicatedIP = false
	upAcceptAup = false

	// Run command
	err := runUp(nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify .cvps.yaml was created
	if _, err := os.Stat(".cvps.yaml"); os.IsNotExist(err) {
		t.Fatal("Expected .cvps.yaml to be created")
	}

	// Verify context content
	ctx, err := loadLocalContext()
	if err != nil {
		t.Fatalf("Failed to load context: %v", err)
	}
	if ctx.SandboxID != "sbx-test-123" {
		t.Errorf("Expected sandbox ID sbx-test-123, got %s", ctx.SandboxID)
	}
	if ctx.Name != "sandbox-test" {
		t.Errorf("Expected name sandbox-test, got %s", ctx.Name)
	}
}

func TestRunUp_WithCustomResources(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Mock API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes":
			var req api.CreateSandboxRequest
			json.NewDecoder(r.Body).Decode(&req)

			// Verify custom values
			if req.CPUCores != 4 {
				t.Errorf("Expected CPU 4, got %d", req.CPUCores)
			}
			if req.MemoryGB != 8 {
				t.Errorf("Expected Memory 8, got %d", req.MemoryGB)
			}
			if req.StorageGB != 50 {
				t.Errorf("Expected Storage 50, got %d", req.StorageGB)
			}
			if req.Name != "my-project" {
				t.Errorf("Expected name my-project, got %s", req.Name)
			}

			resp := api.Sandbox{
				ID:        "sbx-custom-456",
				Name:      req.Name,
				Status:    "provisioning",
				CPUCores:  req.CPUCores,
				MemoryGB:  req.MemoryGB,
				StorageGB: req.StorageGB,
			}
			json.NewEncoder(w).Encode(resp)

		case "/sandboxes/sbx-custom-456/status":
			json.NewEncoder(w).Encode(map[string]string{"status": "running"})

		case "/sandboxes/sbx-custom-456":
			resp := api.Sandbox{
				ID:        "sbx-custom-456",
				Name:      "my-project",
				Status:    "running",
				CPUCores:  4,
				MemoryGB:  8,
				StorageGB: 50,
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	// Set custom flags
	upName = "my-project"
	upCPU = 4
	upMemory = 8
	upStorage = 50
	upDetach = false
	upDedicatedIP = false
	upAcceptAup = false

	err := runUp(nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestRunUp_Detach(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Mock API server - should NOT call status endpoint
	statusCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes":
			resp := api.Sandbox{
				ID:     "sbx-detach-789",
				Name:   "detach-test",
				Status: "provisioning",
			}
			json.NewEncoder(w).Encode(resp)

		case "/sandboxes/sbx-detach-789/status":
			statusCalled = true
			t.Error("Status endpoint should not be called with --detach")
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	upName = "detach-test"
	upDetach = true
	upDedicatedIP = false
	upAcceptAup = false

	err := runUp(nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if statusCalled {
		t.Error("Status should not be checked with --detach flag")
	}

	// Verify context was still saved
	ctx, err := loadLocalContext()
	if err != nil {
		t.Fatalf("Failed to load context: %v", err)
	}
	if ctx.SandboxID != "sbx-detach-789" {
		t.Errorf("Expected sandbox ID sbx-detach-789, got %s", ctx.SandboxID)
	}
}

func TestRunUp_ProvisioningFailed(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Mock API server that returns failed status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes":
			resp := api.Sandbox{
				ID:     "sbx-fail-999",
				Name:   "fail-test",
				Status: "provisioning",
			}
			json.NewEncoder(w).Encode(resp)

		case "/sandboxes/sbx-fail-999/status":
			resp := api.Sandbox{
				ID:     "sbx-fail-999",
				Status: "failed",
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	upName = "fail-test"
	upDetach = false
	upDedicatedIP = false
	upAcceptAup = false

	err := runUp(nil, nil)
	if err == nil {
		t.Fatal("Expected error for failed provisioning")
	}
	if err.Error() != "sandbox provisioning failed: failed" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestRunUp_UsesFullSandboxWhenStatusEndpointIsStale(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	fullSandboxFetched := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes":
			json.NewEncoder(w).Encode(api.Sandbox{
				ID:        "sbx-stale-123",
				Name:      "stale-status",
				Status:    "PROVISIONING",
				CPUCores:  2,
				MemoryGB:  4,
				StorageGB: 5,
			})
		case "/sandboxes/sbx-stale-123/status":
			json.NewEncoder(w).Encode(map[string]string{"status": "PROVISIONING"})
		case "/sandboxes/sbx-stale-123":
			fullSandboxFetched = true
			json.NewEncoder(w).Encode(api.Sandbox{
				ID:        "sbx-stale-123",
				Name:      "stale-status",
				Status:    "RUNNING",
				CPUCores:  2,
				MemoryGB:  4,
				StorageGB: 5,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	upName = "stale-status"
	upCPU = 2
	upMemory = 4
	upStorage = 5
	upDetach = false
	upDedicatedIP = false
	upAcceptAup = false

	if err := runUp(nil, nil); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !fullSandboxFetched {
		t.Fatal("expected full sandbox fetch to break stale status loop")
	}
}

func TestSaveLoadLocalContext(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Save context
	err := saveLocalContext("sbx-123", "test-sandbox")
	if err != nil {
		t.Fatalf("Failed to save context: %v", err)
	}

	// Load context
	ctx, err := loadLocalContext()
	if err != nil {
		t.Fatalf("Failed to load context: %v", err)
	}

	if ctx.SandboxID != "sbx-123" {
		t.Errorf("Expected sandbox ID sbx-123, got %s", ctx.SandboxID)
	}
	if ctx.Name != "test-sandbox" {
		t.Errorf("Expected name test-sandbox, got %s", ctx.Name)
	}

	// Verify timestamp format
	_, err = time.Parse(time.RFC3339, ctx.CreatedAt)
	if err != nil {
		t.Errorf("Invalid timestamp format: %v", err)
	}
}

func TestGetCurrentSandboxID(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Test with no context
	_, err := getCurrentSandboxID()
	if err == nil {
		t.Fatal("Expected error when no context exists")
	}

	// Save context
	saveLocalContext("sbx-456", "test")

	// Test with context
	id, err := getCurrentSandboxID()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if id != "sbx-456" {
		t.Errorf("Expected sbx-456, got %s", id)
	}
}

func TestRunUp_DedicatedIPRequiresAup(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	config.Save(cfg)

	upName = "ip-test"
	upDetach = false
	upDedicatedIP = true
	upAcceptAup = false
	defer func() {
		upDedicatedIP = false
		upAcceptAup = false
	}()

	err := runUp(nil, nil)
	if err == nil {
		t.Fatal("Expected error when --dedicated-ip is set without --accept-aup")
	}
	if !strings.Contains(err.Error(), "--accept-aup") {
		t.Errorf("Error should point at --accept-aup, got: %v", err)
	}
}

func TestRunUp_DedicatedIP_FullFlow(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes":
			var req api.CreateSandboxRequest
			json.NewDecoder(r.Body).Decode(&req)
			if !req.UseDedicatedIp {
				t.Error("Expected useDedicatedIp=true in create request")
			}
			if !req.AcceptedAup {
				t.Error("Expected acceptedAup=true in create request")
			}
			resp := api.Sandbox{
				ID:     "sbx-ip-111",
				Name:   req.Name,
				Status: "PROVISIONING",
				DedicatedIp: &api.SandboxDedicatedIp{
					ID:        "dip-1",
					IPAddress: "85.232.180.42",
				},
			}
			json.NewEncoder(w).Encode(resp)

		case "/sandboxes/sbx-ip-111/status":
			// Pin the real backend casing: Prisma enum RUNNING.
			json.NewEncoder(w).Encode(map[string]string{"status": "RUNNING"})

		case "/sandboxes/sbx-ip-111":
			resp := api.Sandbox{
				ID:        "sbx-ip-111",
				Name:      "ip-test",
				Status:    "RUNNING",
				CPUCores:  1,
				MemoryGB:  2,
				StorageGB: 5,
				DedicatedIp: &api.SandboxDedicatedIp{
					ID:        "dip-1",
					IPAddress: "85.232.180.42",
				},
			}
			json.NewEncoder(w).Encode(resp)

		default:
			t.Errorf("Unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	upName = "ip-test"
	upCPU = 0
	upMemory = 0
	upStorage = 0
	upDetach = false
	upDedicatedIP = true
	upAcceptAup = true
	defer func() {
		upDedicatedIP = false
		upAcceptAup = false
	}()

	if err := runUp(nil, nil); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestCreateErrorHint(t *testing.T) {
	cases := []struct {
		code     string
		wantHint bool
	}{
		{"subscription_required", true},
		{"aup_acceptance_required", true},
		{"dedicated_ip_not_entitled", true},
		{"dedicated_ip_capacity", true},
		{"phone_verification_required", true},
		{"mfa_required", true},
		{"capacity_exhausted", true},
		{"plan_limit_exceeded", false},
		{"", false},
	}
	for _, tc := range cases {
		err := fmt.Errorf("wrap: %w", &api.APIError{StatusCode: 402, Message: "m", ErrorCode: tc.code})
		hint := createErrorHint(err)
		if tc.wantHint && hint == "" {
			t.Errorf("expected hint for %q, got none", tc.code)
		}
		if !tc.wantHint && hint != "" {
			t.Errorf("expected no hint for %q, got %q", tc.code, hint)
		}
	}

	if createErrorHint(fmt.Errorf("plain error")) != "" {
		t.Error("expected no hint for non-API error")
	}
}
