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

func newSecretsListServer(t *testing.T, secrets []api.Secret) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/secrets" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode(api.SecretList{
			Data:  secrets,
			Total: len(secrets),
			Page:  1,
			Limit: 100,
		})
	}))
}

func TestResolveSecretKeys(t *testing.T) {
	server := newSecretsListServer(t, []api.Secret{
		{ID: "sec-1", Key: "TELEGRAM_BOT_TOKEN"},
		{ID: "sec-2", Key: "ANTHROPIC_API_KEY"},
	})
	defer server.Close()
	client := api.NewClient(server.URL, "test-key")
	ctx := context.Background()

	t.Run("no keys", func(t *testing.T) {
		ids, err := resolveSecretKeys(ctx, client, nil)
		if err != nil || ids != nil {
			t.Errorf("expected nil/nil, got %v/%v", ids, err)
		}
	})

	t.Run("resolves and dedupes", func(t *testing.T) {
		ids, err := resolveSecretKeys(ctx, client, []string{"TELEGRAM_BOT_TOKEN", "ANTHROPIC_API_KEY", "TELEGRAM_BOT_TOKEN"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(ids) != 2 || ids[0] != "sec-1" || ids[1] != "sec-2" {
			t.Errorf("unexpected ids: %v", ids)
		}
	})

	t.Run("unknown key fails with create hint", func(t *testing.T) {
		_, err := resolveSecretKeys(ctx, client, []string{"NOPE_KEY"})
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "cvps secret create NOPE_KEY") {
			t.Errorf("expected create hint in error, got: %v", err)
		}
	})

	t.Run("invalid key shape fails before any API call", func(t *testing.T) {
		badClient := api.NewClient("http://127.0.0.1:1", "test-key") // unreachable
		_, err := resolveSecretKeys(ctx, badClient, []string{"not-a-key"})
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
		case "/secrets":
			json.NewEncoder(w).Encode(api.SecretList{
				Data:  []api.Secret{{ID: "sec-tg", Key: "TELEGRAM_BOT_TOKEN"}},
				Total: 1, Page: 1, Limit: 100,
			})
		case "/sandboxes":
			json.NewDecoder(r.Body).Decode(&gotReq)
			json.NewEncoder(w).Encode(api.Sandbox{ID: "sbx-1", Name: gotReq.Name, Status: "provisioning"})
		case "/sandboxes/sbx-1/status":
			json.NewEncoder(w).Encode(map[string]string{"status": "RUNNING"})
		case "/sandboxes/sbx-1":
			json.NewEncoder(w).Encode(api.Sandbox{ID: "sbx-1", Name: "with-secrets", Status: "RUNNING"})
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

	if len(gotReq.SecretIDs) != 1 || gotReq.SecretIDs[0] != "sec-tg" {
		t.Errorf("expected secretIds [sec-tg], got %v", gotReq.SecretIDs)
	}
	if gotReq.EnvOverrides["CAPTURE_MODE"] != "auto" {
		t.Errorf("expected envOverrides CAPTURE_MODE=auto, got %v", gotReq.EnvOverrides)
	}
}

func TestRunUp_UnknownSecretFailsBeforeCreate(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	oldWd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldWd)

	createCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/secrets":
			json.NewEncoder(w).Encode(api.SecretList{Data: nil, Total: 0, Page: 1, Limit: 100})
		case "/sandboxes":
			createCalled = true
			http.Error(w, "should not be called", http.StatusInternalServerError)
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
	if createCalled {
		t.Error("sandbox create must not be called when secret resolution fails")
	}
}
