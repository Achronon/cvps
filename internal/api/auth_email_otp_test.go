package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestEmailOtp(t *testing.T) {
	var gotPath string
	var gotBody map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "If an account exists for that address, a sign-in code has been sent.",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	ack, err := client.RequestEmailOtp(context.Background(), "agent@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/auth/email-otp/request" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["email"] != "agent@example.com" {
		t.Errorf("email = %q", gotBody["email"])
	}
	if ack.Message == "" {
		t.Error("expected a generic message")
	}
}

func TestVerifyEmailOtp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/email-otp/verify" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["email"] != "agent@example.com" || body["code"] != "ABCD-2345" {
			t.Errorf("body = %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "session-jwt",
			"token_type":   "Bearer",
			"expires_in":   86400,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	token, err := client.VerifyEmailOtp(context.Background(), "agent@example.com", "ABCD-2345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "session-jwt" || token.TokenType != "Bearer" {
		t.Errorf("token = %+v", token)
	}
}

func TestVerifyEmailOtpInvalidCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": 400,
			"message":    "Invalid or expired code",
			"error":      "Bad Request",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	_, err := client.VerifyEmailOtp(context.Background(), "agent@example.com", "WRONG-999")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("status = %d", apiErr.StatusCode)
	}
}
