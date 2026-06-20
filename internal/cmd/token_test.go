package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
)

func TestParseExpiresIn(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "minutes", value: "30m", want: "2026-06-10T12:30:00Z"},
		{name: "hours", value: "12h", want: "2026-06-11T00:00:00Z"},
		{name: "days", value: "7d", want: "2026-06-17T12:00:00Z"},
		{name: "weeks", value: "4w", want: "2026-07-08T12:00:00Z"},
		{name: "surrounding whitespace", value: " 7d ", want: "2026-06-17T12:00:00Z"},
		{name: "zero is rejected", value: "0d", wantErr: true},
		{name: "missing unit", value: "30", wantErr: true},
		{name: "unknown unit", value: "30s", wantErr: true},
		{name: "negative", value: "-7d", wantErr: true},
		{name: "go duration syntax rejected", value: "1h30m", wantErr: true},
		{name: "empty", value: "", wantErr: true},
		{name: "garbage", value: "soon", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExpiresIn(tt.value, now)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTokenRef(t *testing.T) {
	keys := []api.APIKey{
		{ID: "key-id-1", Name: "agent-x", KeyPrefix: "cvps_abc1234"},
		{ID: "key-id-2", Name: "agent-y", KeyPrefix: "cvps_def5678"},
		{ID: "key-id-3", Name: "agent-z", KeyPrefix: "cvps_abc9999"},
	}

	t.Run("exact id match", func(t *testing.T) {
		got, err := resolveTokenRef(keys, "key-id-2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "agent-y" {
			t.Errorf("got %s, want agent-y", got.Name)
		}
	})

	t.Run("unique prefix match", func(t *testing.T) {
		got, err := resolveTokenRef(keys, "cvps_def")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "agent-y" {
			t.Errorf("got %s, want agent-y", got.Name)
		}
	})

	t.Run("full pasted key resolves via its prefix", func(t *testing.T) {
		got, err := resolveTokenRef(keys, "cvps_def5678fullkeymaterialhere")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Name != "agent-y" {
			t.Errorf("got %s, want agent-y", got.Name)
		}
	})

	t.Run("ambiguous prefix is an error", func(t *testing.T) {
		_, err := resolveTokenRef(keys, "cvps_abc")
		if err == nil {
			t.Fatal("expected ambiguity error")
		}
		if !strings.Contains(err.Error(), "ambiguous") {
			t.Errorf("expected ambiguity message, got: %v", err)
		}
	})

	t.Run("no match", func(t *testing.T) {
		_, err := resolveTokenRef(keys, "cvps_zzz")
		if err == nil {
			t.Fatal("expected not-found error")
		}
		if !strings.Contains(err.Error(), "no token found") {
			t.Errorf("expected not-found message, got: %v", err)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		_, err := resolveTokenRef(nil, "anything")
		if err == nil {
			t.Fatal("expected not-found error")
		}
	})
}

func TestRunTokenCreateCortexControl(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	var seen api.CreateAPIKeyRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api-keys" || r.Method != "POST" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.APIKey{
			ID:                "key-1",
			Name:              seen.Name,
			Key:               "cvps_created_key",
			KeyPrefix:         "cvps_created",
			Scopes:            []string{"sandboxes:read", "sandboxes:write", "secrets:attach"},
			CapabilityProfile: "cortex-control",
			CreatedAt:         "2026-06-20T10:00:00Z",
			ExpiresAt:         "2026-06-20T11:00:00Z",
		})
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.APIKey = "session-token-for-tests"
	cfg.APIBaseURL = server.URL
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	oldStdout, oldStderr := os.Stdout, os.Stderr
	stdoutR, stdoutW, _ := os.Pipe()
	stderrR, stderrW, _ := os.Pipe()
	os.Stdout = stdoutW
	os.Stderr = stderrW
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		tokenCreateName = ""
		tokenCreateScopes = nil
		tokenCreateExpiresIn = ""
		tokenCreateExpiresAt = ""
		tokenCreateCortex = false
	}()

	tokenCreateName = "cortex"
	tokenCreateScopes = nil
	tokenCreateExpiresIn = ""
	tokenCreateExpiresAt = ""
	tokenCreateCortex = true

	err := runTokenCreate(nil, nil)
	stdoutW.Close()
	stderrW.Close()
	stdoutBytes, _ := io.ReadAll(stdoutR)
	stderrBytes, _ := io.ReadAll(stderrR)
	if err != nil {
		t.Fatalf("runTokenCreate: %v", err)
	}

	if seen.CapabilityProfile != "cortex-control" {
		t.Fatalf("Expected cortex-control payload profile, got %q", seen.CapabilityProfile)
	}
	if strings.TrimSpace(string(stdoutBytes)) != "cvps_created_key" {
		t.Fatalf("Expected key on stdout only, got %q", string(stdoutBytes))
	}
	if strings.Contains(string(stderrBytes), "cvps_created_key") {
		t.Fatal("stderr should not contain token material")
	}
	if !strings.Contains(string(stderrBytes), "Profile: cortex-control") {
		t.Fatalf("stderr should mention profile, got %q", string(stderrBytes))
	}
}
