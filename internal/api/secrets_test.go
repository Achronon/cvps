package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestCreateSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/secrets" {
			t.Errorf("Expected path /secrets, got %s", r.URL.Path)
		}

		var req CreateSecretRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}
		if req.Key != "TELEGRAM_BOT_TOKEN" {
			t.Errorf("Expected key TELEGRAM_BOT_TOKEN, got %s", req.Key)
		}
		if req.Value != "tok-123" {
			t.Errorf("Expected value tok-123, got %s", req.Value)
		}
		if req.Name != "Telegram token" {
			t.Errorf("Expected name 'Telegram token', got %s", req.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Secret{
			ID:   "sec-1",
			Name: req.Name,
			Key:  req.Key,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	secret, err := client.CreateSecret(context.Background(), &CreateSecretRequest{
		Name:  "Telegram token",
		Key:   "TELEGRAM_BOT_TOKEN",
		Value: "tok-123",
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if secret.ID != "sec-1" {
		t.Errorf("Expected ID sec-1, got %s", secret.ID)
	}
	if secret.Key != "TELEGRAM_BOT_TOKEN" {
		t.Errorf("Expected key TELEGRAM_BOT_TOKEN, got %s", secret.Key)
	}
}

func TestCreateSecretConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Secret with this key already exists",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	_, err := client.CreateSecret(context.Background(), &CreateSecretRequest{
		Name: "x", Key: "X", Value: "v",
	})
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusConflict {
		t.Errorf("Expected status 409, got %d", apiErr.StatusCode)
	}
}

func TestListSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/secrets" {
			t.Errorf("Expected path /secrets, got %s", r.URL.Path)
		}
		if page := r.URL.Query().Get("page"); page != "2" {
			t.Errorf("Expected page 2, got %s", page)
		}
		if limit := r.URL.Query().Get("limit"); limit != "50" {
			t.Errorf("Expected limit 50, got %s", limit)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SecretList{
			Data: []Secret{
				{ID: "sec-1", Key: "A_KEY", Name: "A"},
			},
			Total: 51,
			Page:  2,
			Limit: 50,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	list, err := client.ListSecrets(context.Background(), 2, 50)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(list.Data) != 1 || list.Data[0].Key != "A_KEY" {
		t.Errorf("Unexpected list data: %+v", list.Data)
	}
	if list.Total != 51 {
		t.Errorf("Expected total 51, got %d", list.Total)
	}
}

func TestDeleteSecret(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/secrets/sec-1" {
			t.Errorf("Expected path /secrets/sec-1, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	if err := client.DeleteSecret(context.Background(), "sec-1"); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !called {
		t.Error("Expected DELETE to be called")
	}
}

// newPaginatedSecretsServer serves `total` secrets named KEY_0..KEY_{n-1},
// clamping the requested page size to `serverPageSize` (simulating a backend
// that enforces its own max limit).
func newPaginatedSecretsServer(t *testing.T, total, serverPageSize int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}

		start := (page - 1) * serverPageSize
		var data []Secret
		for i := start; i < start+serverPageSize && i < total; i++ {
			data = append(data, Secret{
				ID:  fmt.Sprintf("sec-%d", i),
				Key: fmt.Sprintf("KEY_%d", i),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SecretList{
			Data:  data,
			Total: total,
			Page:  page,
			Limit: serverPageSize,
		})
	}))
}

func TestListAllSecretsPaginatesWithClampedPageSize(t *testing.T) {
	// 120 secrets, server clamps every page to 50 even though the client
	// asks for 100 — the client must keep paging until the total is reached.
	server := newPaginatedSecretsServer(t, 120, 50)
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	secrets, err := client.ListAllSecrets(context.Background())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(secrets) != 120 {
		t.Fatalf("Expected 120 secrets, got %d", len(secrets))
	}
	if secrets[119].Key != "KEY_119" {
		t.Errorf("Expected last key KEY_119, got %s", secrets[119].Key)
	}
}

func TestListAllSecretsEmpty(t *testing.T) {
	server := newPaginatedSecretsServer(t, 0, 50)
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	secrets, err := client.ListAllSecrets(context.Background())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("Expected no secrets, got %d", len(secrets))
	}
}

func TestFindSecretByKey(t *testing.T) {
	server := newPaginatedSecretsServer(t, 120, 50)
	defer server.Close()

	client := NewClient(server.URL, "test-key")

	// Hit on a later page.
	secret, err := client.FindSecretByKey(context.Background(), "KEY_110")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if secret == nil {
		t.Fatal("Expected secret, got nil")
	}
	if secret.ID != "sec-110" {
		t.Errorf("Expected ID sec-110, got %s", secret.ID)
	}

	// Miss.
	secret, err = client.FindSecretByKey(context.Background(), "NOPE")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if secret != nil {
		t.Errorf("Expected nil for unknown key, got %+v", secret)
	}
}
