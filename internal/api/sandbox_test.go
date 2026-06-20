package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSandbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes" {
			t.Errorf("Expected path /sandboxes, got %s", r.URL.Path)
		}

		var req CreateSandboxRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}

		if req.Name != "test-sandbox" {
			t.Errorf("Expected name test-sandbox, got %s", req.Name)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Sandbox{
			ID:        "sb-123",
			Name:      req.Name,
			Status:    "creating",
			CPUCores:  req.CPUCores,
			MemoryGB:  req.MemoryGB,
			StorageGB: req.StorageGB,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	sandbox, err := client.CreateSandbox(context.Background(), &CreateSandboxRequest{
		Name:      "test-sandbox",
		CPUCores:  2,
		MemoryGB:  4,
		StorageGB: 20,
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if sandbox.ID != "sb-123" {
		t.Errorf("Expected ID sb-123, got %s", sandbox.ID)
	}

	if sandbox.Name != "test-sandbox" {
		t.Errorf("Expected name test-sandbox, got %s", sandbox.Name)
	}

	if sandbox.Status != "creating" {
		t.Errorf("Expected status creating, got %s", sandbox.Status)
	}
}

func TestListSandboxes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes" {
			t.Errorf("Expected path /sandboxes, got %s", r.URL.Path)
		}

		page := r.URL.Query().Get("page")
		limit := r.URL.Query().Get("limit")
		if page != "1" {
			t.Errorf("Expected page 1, got %s", page)
		}
		if limit != "10" {
			t.Errorf("Expected limit 10, got %s", limit)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SandboxList{
			Data: []Sandbox{
				{ID: "sb-1", Name: "sandbox-1", Status: "running"},
				{ID: "sb-2", Name: "sandbox-2", Status: "stopped"},
			},
			Total: 2,
			Page:  1,
			Limit: 10,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	list, err := client.ListSandboxes(context.Background(), 1, 10)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if list.Total != 2 {
		t.Errorf("Expected total 2, got %d", list.Total)
	}

	if len(list.Data) != 2 {
		t.Fatalf("Expected 2 sandboxes, got %d", len(list.Data))
	}

	if list.Data[0].ID != "sb-1" {
		t.Errorf("Expected first sandbox ID sb-1, got %s", list.Data[0].ID)
	}
}

func TestGetSandbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sb-123" {
			t.Errorf("Expected path /sandboxes/sb-123, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Sandbox{
			ID:      "sb-123",
			Name:    "test-sandbox",
			Status:  "running",
			SSHHost: "sandbox.example.com",
			SSHPort: 2222,
			SSHUser: "ubuntu",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	sandbox, err := client.GetSandbox(context.Background(), "sb-123")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if sandbox.ID != "sb-123" {
		t.Errorf("Expected ID sb-123, got %s", sandbox.ID)
	}

	if sandbox.Status != "running" {
		t.Errorf("Expected status running, got %s", sandbox.Status)
	}

	if sandbox.SSHHost != "sandbox.example.com" {
		t.Errorf("Expected SSHHost sandbox.example.com, got %s", sandbox.SSHHost)
	}
}

func TestGetSandboxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sb-123/status" {
			t.Errorf("Expected path /sandboxes/sb-123/status, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Sandbox{
			ID:     "sb-123",
			Name:   "test-sandbox",
			Status: "running",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	sandbox, err := client.GetSandboxStatus(context.Background(), "sb-123")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if sandbox.Status != "running" {
		t.Errorf("Expected status running, got %s", sandbox.Status)
	}
}

func TestDeleteSandbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sb-123" {
			t.Errorf("Expected path /sandboxes/sb-123, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	err := client.DeleteSandbox(context.Background(), "sb-123")

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestDeleteSandboxWithGrant(t *testing.T) {
	const grant = "cvps_dgrant_test"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sb-123" {
			t.Errorf("Expected path /sandboxes/sb-123, got %s", r.URL.Path)
		}
		if got := r.Header.Get(destructiveGrantHeader); got != grant {
			t.Errorf("Expected destructive grant header %q, got %q", grant, got)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	if err := client.DeleteSandboxWithGrant(context.Background(), "sb-123", grant); err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestListSandboxAllowRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-123/allow-rules" {
			t.Errorf("Expected allow-rules path, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]SandboxAllowRule{{
			ID:              "allow-1",
			SourceSandboxID: "sbx-123",
			TargetSandboxID: "sbx-456",
			Port:            8788,
			Protocol:        "TCP",
		}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	rules, err := client.ListSandboxAllowRules(context.Background(), "sbx-123")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(rules) != 1 || rules[0].TargetSandboxID != "sbx-456" || rules[0].Port != 8788 {
		t.Fatalf("unexpected allow rules: %+v", rules)
	}
}

func TestAllowSandboxReachability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-123/allow-rules" {
			t.Errorf("Expected allow-rules path, got %s", r.URL.Path)
		}

		var req CreateSandboxAllowRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}
		if req.TargetSandboxID != "sbx-456" || req.Port != 8788 {
			t.Fatalf("unexpected request: %+v", req)
		}

		changed := true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SandboxAllowRule{
			ID:              "allow-1",
			SourceSandboxID: "sbx-123",
			TargetSandboxID: "sbx-456",
			Port:            8788,
			Protocol:        "TCP",
			Changed:         &changed,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	rule, err := client.AllowSandboxReachability(context.Background(), "sbx-123", "sbx-456", 8788)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if rule.ID != "allow-1" || rule.Changed == nil || !*rule.Changed {
		t.Fatalf("unexpected allow rule: %+v", rule)
	}
}

func TestRevokeSandboxReachability(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-123/allow-rules/sbx-456/8788" {
			t.Errorf("Expected revoke path, got %s", r.URL.Path)
		}

		changed := true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(SandboxAllowRule{
			ID:              "allow-1",
			SourceSandboxID: "sbx-123",
			TargetSandboxID: "sbx-456",
			Port:            8788,
			Protocol:        "TCP",
			Changed:         &changed,
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	rule, err := client.RevokeSandboxReachability(context.Background(), "sbx-123", "sbx-456", 8788)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if rule.TargetSandboxID != "sbx-456" || rule.Port != 8788 {
		t.Fatalf("unexpected allow rule: %+v", rule)
	}
}

func TestAttachSecretToSandbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-123/secrets/TELEGRAM_BOT_TOKEN" {
			t.Errorf("Expected attach path, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Sandbox{
			ID:     "sbx-123",
			Name:   "cortex-brain",
			Status: "PROVISIONING",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	sandbox, err := client.AttachSecretToSandbox(
		context.Background(),
		"sbx-123",
		"TELEGRAM_BOT_TOKEN",
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if sandbox.ID != "sbx-123" || sandbox.Status != "PROVISIONING" {
		t.Fatalf("unexpected sandbox response: %+v", sandbox)
	}
}

func TestDetachSecretFromSandbox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/sandboxes/sbx-123/secrets/TELEGRAM_BOT_TOKEN" {
			t.Errorf("Expected detach path, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Sandbox{
			ID:     "sbx-123",
			Name:   "cortex-brain",
			Status: "PROVISIONING",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	sandbox, err := client.DetachSecretFromSandbox(
		context.Background(),
		"sbx-123",
		"TELEGRAM_BOT_TOKEN",
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if sandbox.ID != "sbx-123" || sandbox.Status != "PROVISIONING" {
		t.Fatalf("unexpected sandbox response: %+v", sandbox)
	}
}
