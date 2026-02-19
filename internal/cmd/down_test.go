package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
)

func TestRunDown_NotAuthenticated(t *testing.T) {
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
	err := runDown(nil, nil)
	if err == nil {
		t.Fatal("Expected error for unauthenticated request")
	}
	if err.Error() != "not logged in. Run 'cvps login' first" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestRunDown_NoContextNoArgs(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create config with auth
	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// No .cvps.yaml and no args
	err := runDown(nil, nil)
	if err == nil {
		t.Fatal("Expected error when no context and no args")
	}
}

func TestRunDown_SandboxNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Mock API server returning 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sandboxes/sbx-notfound" {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(api.APIError{
				StatusCode: 404,
				Message:    "Sandbox not found",
			})
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	// Set flags
	downForce = true // Skip confirmation

	// Run with explicit ID
	err := runDown(nil, []string{"sbx-notfound"})
	// Should not error, just print message
	if err != nil {
		t.Fatalf("Expected no error for already-deleted sandbox: %v", err)
	}
}

func TestRunDown_WithForceFlag(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	deleteCalled := false
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes/sbx-force":
			if r.Method == "GET" {
				if deleted {
					// After deletion, return 404
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(api.APIError{
						StatusCode: 404,
						Message:    "Sandbox not found",
					})
				} else {
					// First call - return sandbox
					resp := api.Sandbox{
						ID:     "sbx-force",
						Name:   "force-test",
						Status: "running",
					}
					json.NewEncoder(w).Encode(resp)
				}
			} else if r.Method == "DELETE" {
				deleteCalled = true
				deleted = true
				w.WriteHeader(http.StatusNoContent)
			}
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	// Set flags
	downForce = true
	downAll = false

	// Run with explicit ID
	err := runDown(nil, []string{"sbx-force"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !deleteCalled {
		t.Error("Expected DELETE to be called")
	}
}

func TestRunDown_FromContext(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create local context
	saveLocalContext("sbx-ctx-123", "context-sandbox")

	deleteCalled := false
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes/sbx-ctx-123":
			if r.Method == "GET" {
				if deleted {
					// After deletion, return 404
					w.WriteHeader(http.StatusNotFound)
					json.NewEncoder(w).Encode(api.APIError{
						StatusCode: 404,
						Message:    "Sandbox not found",
					})
				} else {
					resp := api.Sandbox{
						ID:     "sbx-ctx-123",
						Name:   "context-sandbox",
						Status: "running",
					}
					json.NewEncoder(w).Encode(resp)
				}
			} else if r.Method == "DELETE" {
				deleteCalled = true
				deleted = true
				w.WriteHeader(http.StatusNoContent)
			}
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	downForce = true
	downAll = false

	// Run without args - should use context
	err := runDown(nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !deleteCalled {
		t.Error("Expected DELETE to be called")
	}

	// Verify .cvps.yaml was cleaned up
	if _, err := os.Stat(".cvps.yaml"); err == nil {
		t.Error("Expected .cvps.yaml to be removed")
	}
}

func TestRunDown_AllSandboxes(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	deleteCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes":
			if r.Method == "GET" {
				resp := api.SandboxList{
					Data: []api.Sandbox{
						{ID: "sbx-1", Name: "sandbox-1", Status: "running"},
						{ID: "sbx-2", Name: "sandbox-2", Status: "running"},
						{ID: "sbx-3", Name: "sandbox-3", Status: "running"},
					},
					Total: 3,
					Page:  1,
					Limit: 100,
				}
				json.NewEncoder(w).Encode(resp)
			}
		case "/sandboxes/sbx-1", "/sandboxes/sbx-2", "/sandboxes/sbx-3":
			if r.Method == "DELETE" {
				deleteCount++
				w.WriteHeader(http.StatusNoContent)
			}
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	downForce = true
	downAll = true

	err := runDown(nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if deleteCount != 3 {
		t.Errorf("Expected 3 deletes, got %d", deleteCount)
	}
}

func TestRunDown_AllSandboxes_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	oldConfigDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldConfigDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sandboxes" && r.Method == "GET" {
			resp := api.SandboxList{
				Data:  []api.Sandbox{},
				Total: 0,
				Page:  1,
				Limit: 100,
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	downForce = true
	downAll = true

	err := runDown(nil, nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestCleanupLocalContext(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create context
	saveLocalContext("sbx-cleanup", "cleanup-test")

	// Verify file exists
	if _, err := os.Stat(".cvps.yaml"); os.IsNotExist(err) {
		t.Fatal("Expected .cvps.yaml to exist")
	}

	// Cleanup matching sandbox
	cleanupLocalContext("sbx-cleanup")

	// Verify file was removed
	if _, err := os.Stat(".cvps.yaml"); err == nil {
		t.Error("Expected .cvps.yaml to be removed")
	}
}

func TestCleanupLocalContext_DifferentSandbox(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	// Create context
	saveLocalContext("sbx-keep", "keep-test")

	// Try to cleanup different sandbox
	cleanupLocalContext("sbx-other")

	// Verify file still exists
	if _, err := os.Stat(".cvps.yaml"); os.IsNotExist(err) {
		t.Error("Expected .cvps.yaml to still exist")
	}
}
