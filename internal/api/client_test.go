package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/achronon/cvps/internal/config"
)

func TestNewClient(t *testing.T) {
	client := NewClient("https://api.example.com", "test-api-key")
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if client.baseURL != "https://api.example.com" {
		t.Errorf("Expected baseURL https://api.example.com, got %s", client.baseURL)
	}
	if client.apiKey != "test-api-key" {
		t.Errorf("Expected apiKey test-api-key, got %s", client.apiKey)
	}
}

func TestNewClientWithToken(t *testing.T) {
	client := NewClientWithToken("https://api.example.com", "test-token")
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if client.token != "test-token" {
		t.Errorf("Expected token test-token, got %s", client.token)
	}
}

func TestNewClientFromConfig(t *testing.T) {
	t.Run("with access token", func(t *testing.T) {
		cfg := &config.Config{
			APIBaseURL:  "https://api.example.com",
			AccessToken: "test-token",
			APIKey:      "test-api-key",
		}
		client := NewClientFromConfig(cfg)
		if client.token != "test-token" {
			t.Errorf("Expected token test-token, got %s", client.token)
		}
		// Should prefer token over API key
		if client.apiKey != "" {
			t.Error("Expected empty apiKey when token is present")
		}
	})

	t.Run("with API key only", func(t *testing.T) {
		cfg := &config.Config{
			APIBaseURL: "https://api.example.com",
			APIKey:     "test-api-key",
		}
		client := NewClientFromConfig(cfg)
		if client.apiKey != "test-api-key" {
			t.Errorf("Expected apiKey test-api-key, got %s", client.apiKey)
		}
		if client.token != "" {
			t.Error("Expected empty token")
		}
	})
}

func TestClientOptions(t *testing.T) {
	t.Run("WithVerbose", func(t *testing.T) {
		client := NewClient("https://api.example.com", "key", WithVerbose(true))
		if !client.verbose {
			t.Error("Expected verbose to be true")
		}
	})

	t.Run("WithTimeout", func(t *testing.T) {
		timeout := 10 * time.Second
		client := NewClient("https://api.example.com", "key", WithTimeout(timeout))
		if client.httpClient.Timeout != timeout {
			t.Errorf("Expected timeout %v, got %v", timeout, client.httpClient.Timeout)
		}
	})
}

func TestClientGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/test" {
			t.Errorf("Expected path /test, got %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Error("Expected X-API-Key header")
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("Expected User-Agent header")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	var result map[string]string
	err := client.Get(context.Background(), "/test", &result)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("Expected status ok, got %s", result["status"])
	}
}

func TestClientPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("Expected Authorization header with Bearer token")
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "test" {
			t.Errorf("Expected name test, got %s", body["name"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "123", "name": "test"})
	}))
	defer server.Close()

	client := NewClientWithToken(server.URL, "test-token")
	var result map[string]string
	err := client.Post(context.Background(), "/test", map[string]string{"name": "test"}, &result)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result["id"] != "123" {
		t.Errorf("Expected id 123, got %s", result["id"])
	}
}

func TestClientDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.Delete(context.Background(), "/test/123")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestClientPatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" {
			t.Errorf("Expected PATCH request, got %s", r.Method)
		}

		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["name"] != "updated" {
			t.Errorf("Expected name updated, got %s", body["name"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id": "123", "name": "updated"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	var result map[string]string
	err := client.Patch(context.Background(), "/test/123", map[string]string{"name": "updated"}, &result)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result["name"] != "updated" {
		t.Errorf("Expected name updated, got %s", result["name"])
	}
}

func TestClientErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIError{
			Message: "Resource not found",
			Code:    "NOT_FOUND",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.Get(context.Background(), "/test", nil)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("Expected APIError, got %T", err)
	}

	if apiErr.StatusCode != 404 {
		t.Errorf("Expected status code 404, got %d", apiErr.StatusCode)
	}

	if apiErr.Message != "Resource not found" {
		t.Errorf("Expected message 'Resource not found', got %s", apiErr.Message)
	}

	if !IsNotFound(err) {
		t.Error("Expected IsNotFound to return true")
	}
}

func TestClientUnauthorizedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(APIError{
			Message: "Invalid credentials",
			Code:    "UNAUTHORIZED",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "invalid-key")
	err := client.Get(context.Background(), "/test", nil)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !IsUnauthorized(err) {
		t.Error("Expected IsUnauthorized to return true")
	}
}

func TestClientForbiddenError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(APIError{
			Message: "Access denied",
			Code:    "FORBIDDEN",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.Get(context.Background(), "/test", nil)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if !IsForbidden(err) {
		t.Error("Expected IsForbidden to return true")
	}
}
