package cmd

import (
	"testing"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/fatih/color"
)

func TestColorStatus(t *testing.T) {
	prevNoColor := color.NoColor
	color.NoColor = false
	t.Cleanup(func() {
		color.NoColor = prevNoColor
	})

	tests := []struct {
		name   string
		status string
		want   string
	}{
		{
			name:   "running status should be green",
			status: "running",
			want:   "\x1b[32mrunning\x1b[0m",
		},
		{
			name:   "provisioning status should be yellow",
			status: "provisioning",
			want:   "\x1b[33mprovisioning\x1b[0m",
		},
		{
			name:   "starting status should be yellow",
			status: "starting",
			want:   "\x1b[33mstarting\x1b[0m",
		},
		{
			name:   "stopped status should be gray",
			status: "stopped",
			want:   "\x1b[90mstopped\x1b[0m",
		},
		{
			name:   "failed status should be red",
			status: "failed",
			want:   "\x1b[31mfailed\x1b[0m",
		},
		{
			name:   "error status should be red",
			status: "error",
			want:   "\x1b[31merror\x1b[0m",
		},
		{
			name:   "unknown status should remain unchanged",
			status: "unknown",
			want:   "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := colorStatus(tt.status)
			if got != tt.want {
				t.Errorf("colorStatus(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "valid RFC3339 timestamp",
			input: "2024-01-15T10:30:00Z",
			want:  "2024-01-15 10:30:00",
		},
		{
			name:  "invalid timestamp returns original",
			input: "invalid-date",
			want:  "invalid-date",
		},
		{
			name:  "empty string returns empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTime(tt.input)
			// For valid timestamps, we need to account for timezone conversion
			if tt.input == "2024-01-15T10:30:00Z" {
				parsed, _ := time.Parse(time.RFC3339, tt.input)
				expected := parsed.Local().Format("2006-01-02 15:04:05")
				if got != expected {
					t.Errorf("formatTime(%q) = %q, want %q", tt.input, got, expected)
				}
			} else {
				if got != tt.want {
					t.Errorf("formatTime(%q) = %q, want %q", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestPrintSandboxDetails(t *testing.T) {
	tests := []struct {
		name    string
		sandbox *api.Sandbox
	}{
		{
			name: "sandbox with all fields",
			sandbox: &api.Sandbox{
				ID:         "sbx-abc123",
				Name:       "my-project",
				Status:     "running",
				CPUCores:   2,
				MemoryGB:   4,
				StorageGB:  20,
				CreatedAt:  "2024-01-15T10:00:00Z",
				LastActive: "2024-01-15T11:30:00Z",
				SSHHost:    "sbx-abc123.example.com",
				SSHPort:    22,
				SSHUser:    "sandbox",
			},
		},
		{
			name: "sandbox without connection info",
			sandbox: &api.Sandbox{
				ID:        "sbx-def456",
				Name:      "test-env",
				Status:    "stopped",
				CPUCores:  1,
				MemoryGB:  2,
				StorageGB: 10,
				CreatedAt: "2024-01-15T08:00:00Z",
			},
		},
		{
			name: "sandbox provisioning",
			sandbox: &api.Sandbox{
				ID:        "sbx-ghi789",
				Name:      "new-sandbox",
				Status:    "provisioning",
				CPUCores:  2,
				MemoryGB:  4,
				StorageGB: 20,
				CreatedAt: "2024-01-15T12:00:00Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This test just ensures the function doesn't panic
			// We can't easily test the output without mocking stdout
			printSandboxDetails(tt.sandbox)
		})
	}
}
