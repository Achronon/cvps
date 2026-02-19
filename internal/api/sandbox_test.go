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
