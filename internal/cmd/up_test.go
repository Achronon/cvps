package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	if err.Error() != "not logged in. Run 'cvps login' first" {
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
			// Return running status immediately
			resp := api.Sandbox{
				ID:        "sbx-test-123",
				Name:      "sandbox-test",
				Status:    "running",
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

	err := runUp(nil, nil)
	if err == nil {
		t.Fatal("Expected error for failed provisioning")
	}
	if err.Error() != "sandbox provisioning failed: failed" {
		t.Errorf("Unexpected error: %v", err)
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
