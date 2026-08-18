package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
)

func TestParseEnvOverrides(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got, err := parseEnvOverrides(nil)
		if err != nil || got != nil {
			t.Errorf("expected nil/nil, got %v/%v", got, err)
		}
	})

	t.Run("valid pairs", func(t *testing.T) {
		got, err := parseEnvOverrides([]string{"CAPTURE_MODE=auto", "ASKS_PER_DAY=5", "WITH_EQUALS=a=b=c"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["CAPTURE_MODE"] != "auto" || got["ASKS_PER_DAY"] != "5" {
			t.Errorf("unexpected map: %v", got)
		}
		// Only the FIRST '=' splits key from value.
		if got["WITH_EQUALS"] != "a=b=c" {
			t.Errorf("expected value a=b=c, got %q", got["WITH_EQUALS"])
		}
	})

	invalid := []struct {
		name string
		pair string
	}{
		{"missing equals", "JUSTAKEY"},
		{"lowercase key", "lower=v"},
		{"empty value", "KEY="},
		{"empty key", "=v"},
		{"dash in key", "BAD-KEY=v"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseEnvOverrides([]string{tc.pair}); err == nil {
				t.Errorf("expected error for %q", tc.pair)
			}
		})
	}

	t.Run("duplicate key", func(t *testing.T) {
		if _, err := parseEnvOverrides([]string{"K=1", "K=2"}); err == nil {
			t.Error("expected error for duplicate key")
		}
	})
}

func TestValidateSecretKeys(t *testing.T) {
	t.Run("no keys", func(t *testing.T) {
		keys, err := validateSecretKeys(nil)
		if err != nil || keys != nil {
			t.Errorf("expected nil/nil, got %v/%v", keys, err)
		}
	})

	t.Run("dedupes valid keys", func(t *testing.T) {
		keys, err := validateSecretKeys([]string{"TELEGRAM_BOT_TOKEN", "ANTHROPIC_API_KEY", "TELEGRAM_BOT_TOKEN"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(keys) != 2 || keys[0] != "TELEGRAM_BOT_TOKEN" || keys[1] != "ANTHROPIC_API_KEY" {
			t.Errorf("unexpected keys: %v", keys)
		}
	})

	t.Run("invalid key shape fails before any API call", func(t *testing.T) {
		_, err := validateSecretKeys([]string{"not-a-key"})
		if err == nil || !strings.Contains(err.Error(), "invalid --secret key") {
			t.Errorf("expected client-side validation error, got: %v", err)
		}
	})
}

func TestRunUp_WithSecretsAndEnv(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	var gotReq api.CreateSandboxRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes":
			json.NewDecoder(r.Body).Decode(&gotReq)
			json.NewEncoder(w).Encode(api.Sandbox{ID: "sbx-1", Name: gotReq.Name, Status: "provisioning"})
		case "/sandboxes/sbx-1/status":
			json.NewEncoder(w).Encode(map[string]string{"status": "RUNNING"})
		case "/sandboxes/sbx-1":
			json.NewEncoder(w).Encode(api.Sandbox{
				ID: "sbx-1", Name: "with-secrets", Status: "RUNNING",
				Secrets: []api.SandboxSecret{{Key: "TELEGRAM_BOT_TOKEN"}},
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	if err := config.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	upName = "with-secrets"
	upCPU, upMemory, upStorage = 0, 0, 0
	upDetach = true
	upDedicatedIP, upAcceptAup = false, false
	upProfile = ""
	upSecrets = []string{"TELEGRAM_BOT_TOKEN"}
	upEnv = []string{"CAPTURE_MODE=auto"}
	t.Cleanup(func() {
		upSecrets, upEnv = nil, nil
		upDetach = false
		upName = ""
	})

	if err := runUp(nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(gotReq.SecretKeys) != 1 || gotReq.SecretKeys[0] != "TELEGRAM_BOT_TOKEN" {
		t.Errorf("expected secretKeys [TELEGRAM_BOT_TOKEN], got %v", gotReq.SecretKeys)
	}
	if len(gotReq.SecretIDs) != 0 {
		t.Errorf("expected no secret IDs, got %v", gotReq.SecretIDs)
	}
	if gotReq.EnvOverrides["CAPTURE_MODE"] != "auto" {
		t.Errorf("expected envOverrides CAPTURE_MODE=auto, got %v", gotReq.EnvOverrides)
	}
}

func TestVerifySecretAttachmentsRejectsMissingKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.Sandbox{
			ID: "sbx-verify", Secrets: []api.SandboxSecret{{Key: "PRESENT_KEY"}},
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-key")
	err := verifySecretAttachments(
		context.Background(),
		client,
		"sbx-verify",
		[]string{"PRESENT_KEY", "MISSING_KEY"},
	)
	if err == nil || !strings.Contains(err.Error(), "MISSING_KEY") {
		t.Fatalf("expected missing-key verification error, got %v", err)
	}
}

func TestVerifySecretAttachmentsReportsReadFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 500,
			"error":      "internal_error",
			"message":    "temporary failure",
		})
	}))
	defer server.Close()

	client := api.NewClient(server.URL, "test-key")
	err := verifySecretAttachments(
		context.Background(), client, "sbx-verify", []string{"PRESENT_KEY"},
	)
	if err == nil || !strings.Contains(err.Error(), "could not be verified") {
		t.Fatalf("expected verification read error, got %v", err)
	}
}

func TestRunUp_SendsUnknownSecretKeyToBackend(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	createCalled := false
	var gotReq api.CreateSandboxRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandboxes":
			createCalled = true
			json.NewDecoder(r.Body).Decode(&gotReq)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{
				"statusCode": 404,
				"error":      "secret_key_not_found",
				"message":    "Secrets not found by key: MISSING_KEY",
			})
		case "/secrets":
			t.Errorf("cvps up must not enumerate /secrets")
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "test-key"
	cfg.APIBaseURL = server.URL
	if err := config.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	upName = "fail-fast"
	upCPU, upMemory, upStorage = 0, 0, 0
	upDetach = true
	upDedicatedIP, upAcceptAup = false, false
	upProfile = ""
	upSecrets = []string{"MISSING_KEY"}
	upEnv = nil
	t.Cleanup(func() {
		upSecrets = nil
		upDetach = false
		upName = ""
	})

	err := runUp(nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown secret key")
	}
	if !createCalled {
		t.Error("sandbox create should receive the key so the backend can resolve it")
	}
	if len(gotReq.SecretKeys) != 1 || gotReq.SecretKeys[0] != "MISSING_KEY" {
		t.Errorf("expected backend to receive MISSING_KEY, got %v", gotReq.SecretKeys)
	}
	if !strings.Contains(err.Error(), "cvps secret create <KEY>") {
		t.Errorf("expected actionable secret-create hint, got: %v", err)
	}
}
