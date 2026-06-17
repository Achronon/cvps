package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
)

func TestRunResize_RequiresStorageFlag(t *testing.T) {
	resizeStorage = 0
	err := runResize(nil, []string{"sbx-abc"})
	if err == nil {
		t.Fatal("Expected error when --storage is missing")
	}
	if !strings.Contains(err.Error(), "--storage is required") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestRunResize_NotAuthenticated(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg := config.DefaultConfig()
	cfg.APIKey = ""
	cfg.AccessToken = ""
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	resizeStorage = 50
	err := runResize(nil, []string{"sbx-abc"})
	if err == nil {
		t.Fatal("Expected error for unauthenticated request")
	}
	if err.Error() != "not logged in. Run 'cvps login' or set CVPS_API_TOKEN" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestRunResize_GrowsStorage(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	var gotMethod, gotPath string
	var gotStorage int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sandboxes/sbx-grow" {
			gotMethod = r.Method
			gotPath = r.URL.Path
			var req api.ResizeSandboxRequest
			json.NewDecoder(r.Body).Decode(&req)
			gotStorage = req.StorageGB
			json.NewEncoder(w).Encode(api.Sandbox{
				ID:        "sbx-grow",
				Name:      "grow-test",
				Status:    "running",
				StorageGB: req.StorageGB,
			})
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	resizeStorage = 80
	if err := runResize(nil, []string{"sbx-grow"}); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if gotMethod != "PATCH" {
		t.Errorf("Expected PATCH, got %s", gotMethod)
	}
	if gotPath != "/sandboxes/sbx-grow" {
		t.Errorf("Expected path /sandboxes/sbx-grow, got %s", gotPath)
	}
	if gotStorage != 80 {
		t.Errorf("Expected storageGb 80, got %d", gotStorage)
	}
}

func TestRunResize_SandboxNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(api.APIError{StatusCode: 404, Message: "Sandbox not found"})
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	resizeStorage = 50
	err := runResize(nil, []string{"sbx-missing"})
	if err == nil {
		t.Fatal("Expected error for missing sandbox")
	}
	if !strings.Contains(err.Error(), "sandbox not found") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestRunResize_ShrinkRejectedByBackend(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(api.APIError{
			StatusCode: 400,
			Message:    "Storage can only be grown. Requested 5GB is not greater than the current 20GB.",
		})
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	config.Save(cfg)

	resizeStorage = 5
	err := runResize(nil, []string{"sbx-shrink"})
	if err == nil {
		t.Fatal("Expected error when shrinking")
	}
	if !strings.Contains(err.Error(), "can only be grown") {
		t.Errorf("Expected backend shrink-rejection message, got: %v", err)
	}
}
