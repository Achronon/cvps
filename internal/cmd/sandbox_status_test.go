package cmd

import "testing"

func TestIsRunningStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{name: "lowercase running", status: "running", want: true},
		{name: "uppercase running", status: "RUNNING", want: true},
		{name: "mixed case running", status: "Running", want: true},
		{name: "trimmed running", status: " running ", want: true},
		{name: "stopped", status: "stopped", want: false},
		{name: "empty", status: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRunningStatus(tt.status)
			if got != tt.want {
				t.Fatalf("isRunningStatus(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
