package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetTerminalWebSocket(t *testing.T) {
	tests := []struct {
		name       string
		sandboxID  string
		serverResp TerminalWebSocketInfo
		serverCode int
		wantErr    bool
	}{
		{
			name:      "successful terminal websocket request",
			sandboxID: "sbx-abc123",
			serverResp: TerminalWebSocketInfo{
				URL:   "wss://terminal.example.com/ws/sbx-abc123",
				Token: "test-token-123",
			},
			serverCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "sandbox not found",
			sandboxID:  "sbx-notfound",
			serverResp: TerminalWebSocketInfo{},
			serverCode: http.StatusNotFound,
			wantErr:    true,
		},
		{
			name:       "server error",
			sandboxID:  "sbx-error",
			serverResp: TerminalWebSocketInfo{},
			serverCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and path
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				expectedPath := "/sandboxes/" + tt.sandboxID + "/terminal"
				if r.URL.Path != expectedPath {
					t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
				}

				// Send response
				w.WriteHeader(tt.serverCode)
				if tt.serverCode == http.StatusOK {
					json.NewEncoder(w).Encode(tt.serverResp)
				}
			}))
			defer server.Close()

			// Create client
			client := NewClient(server.URL, "test-api-key")

			// Call method
			ctx := context.Background()
			info, err := client.GetTerminalWebSocket(ctx, tt.sandboxID)

			// Check error expectation
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTerminalWebSocket() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// If no error expected, verify response
			if !tt.wantErr {
				if info.URL != tt.serverResp.URL {
					t.Errorf("GetTerminalWebSocket() URL = %v, want %v", info.URL, tt.serverResp.URL)
				}
				if info.Token != tt.serverResp.Token {
					t.Errorf("GetTerminalWebSocket() Token = %v, want %v", info.Token, tt.serverResp.Token)
				}
			}
		})
	}
}
