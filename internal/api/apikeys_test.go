package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAPIKeyCapabilityProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api-keys" {
			t.Errorf("Expected path /api-keys, got %s", r.URL.Path)
		}

		var req CreateAPIKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}
		if req.CapabilityProfile != "cortex-control" {
			t.Fatalf("Expected cortex-control capability profile, got %q", req.CapabilityProfile)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(APIKey{
			ID:                "key-1",
			Name:              req.Name,
			Key:               "cvps_created_key",
			KeyPrefix:         "cvps_created",
			Scopes:            []string{"sandboxes:read", "sandboxes:write", "secrets:attach"},
			CapabilityProfile: req.CapabilityProfile,
			CreatedAt:         "2026-06-20T10:00:00Z",
			ExpiresAt:         "2026-06-20T11:00:00Z",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	key, err := client.CreateAPIKey(context.Background(), &CreateAPIKeyRequest{
		Name:              "cortex",
		CapabilityProfile: "cortex-control",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if key.CapabilityProfile != "cortex-control" {
		t.Fatalf("Expected cortex-control response profile, got %q", key.CapabilityProfile)
	}
}
