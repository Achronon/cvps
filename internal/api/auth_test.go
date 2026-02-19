package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInitiateDeviceAuth(t *testing.T) {
	expectedResponse := &DeviceAuthResponse{
		DeviceCode:              "test-device-code",
		UserCode:                "ABC-123",
		VerificationURI:         "https://example.com/device",
		VerificationURIComplete: "https://example.com/device?code=ABC-123",
		ExpiresIn:               300,
		Interval:                5,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/device" {
			t.Errorf("Expected path /auth/device, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedResponse)
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	response, err := client.InitiateDeviceAuth(context.Background())
	if err != nil {
		t.Fatalf("InitiateDeviceAuth failed: %v", err)
	}

	if response.DeviceCode != expectedResponse.DeviceCode {
		t.Errorf("Expected device code %s, got %s", expectedResponse.DeviceCode, response.DeviceCode)
	}
	if response.UserCode != expectedResponse.UserCode {
		t.Errorf("Expected user code %s, got %s", expectedResponse.UserCode, response.UserCode)
	}
}

func TestGetCurrentUser(t *testing.T) {
	expectedUser := &User{
		ID:    "user-123",
		Email: "test@example.com",
		Name:  "Test User",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/me" {
			t.Errorf("Expected path /users/me, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected GET method, got %s", r.Method)
		}

		// Check for authentication header
		apiKey := r.Header.Get("X-API-Key")
		token := r.Header.Get("Authorization")
		if apiKey == "" && token == "" {
			t.Error("Expected authentication header")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedUser)
	}))
	defer server.Close()

	t.Run("with API key", func(t *testing.T) {
		client := NewClient(server.URL, "test-api-key")
		user, err := client.GetCurrentUser(context.Background())
		if err != nil {
			t.Fatalf("GetCurrentUser failed: %v", err)
		}

		if user.ID != expectedUser.ID {
			t.Errorf("Expected user ID %s, got %s", expectedUser.ID, user.ID)
		}
		if user.Email != expectedUser.Email {
			t.Errorf("Expected email %s, got %s", expectedUser.Email, user.Email)
		}
	})

	t.Run("with token", func(t *testing.T) {
		client := NewClientWithToken(server.URL, "test-token")
		user, err := client.GetCurrentUser(context.Background())
		if err != nil {
			t.Fatalf("GetCurrentUser failed: %v", err)
		}

		if user.Name != expectedUser.Name {
			t.Errorf("Expected name %s, got %s", expectedUser.Name, user.Name)
		}
	})
}

func TestCheckDeviceAuth(t *testing.T) {
	t.Run("authorization pending", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "authorization_pending",
			})
		}))
		defer server.Close()

		client := NewClient(server.URL, "")
		_, err := client.checkDeviceAuth(context.Background(), "device-code")
		if err == nil {
			t.Error("Expected error for authorization pending")
		}
		if err.Error() != "authorization_pending" {
			t.Errorf("Expected 'authorization_pending' error, got %v", err)
		}
	})

	t.Run("successful token", func(t *testing.T) {
		expectedToken := &TokenResponse{
			AccessToken: "test-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(expectedToken)
		}))
		defer server.Close()

		client := NewClient(server.URL, "")
		token, err := client.checkDeviceAuth(context.Background(), "device-code")
		if err != nil {
			t.Fatalf("checkDeviceAuth failed: %v", err)
		}

		if token.AccessToken != expectedToken.AccessToken {
			t.Errorf("Expected access token %s, got %s", expectedToken.AccessToken, token.AccessToken)
		}
	})
}

func TestPollDeviceAuth_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "authorization_pending",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.PollDeviceAuth(ctx, "device-code", 50*time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}
