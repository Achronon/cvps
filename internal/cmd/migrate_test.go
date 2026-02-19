package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"zero bytes", 0, "0 B"},
		{"bytes", 500, "500 B"},
		{"kilobytes", 1024, "1.0 KB"},
		{"megabytes", 1024 * 1024, "1.0 MB"},
		{"gigabytes", 1024 * 1024 * 1024, "1.0 GB"},
		{"terabytes", 1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{"mixed", 1536, "1.5 KB"},
		{"large", 1572864, "1.5 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %s; want %s", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestMigrateCmd_Help(t *testing.T) {
	// Test that the command is properly registered
	if migrateCmd == nil {
		t.Fatal("migrateCmd is nil")
	}

	if migrateCmd.Use != "migrate <local-path>" {
		t.Errorf("expected Use to be 'migrate <local-path>', got %s", migrateCmd.Use)
	}

	if migrateCmd.Short != "Migrate local workspace to sandbox" {
		t.Errorf("expected Short to be 'Migrate local workspace to sandbox', got %s", migrateCmd.Short)
	}
}

func TestMigrateCmd_Flags(t *testing.T) {
	// Check that flags are properly defined
	excludeFlag := migrateCmd.Flags().Lookup("exclude")
	if excludeFlag == nil {
		t.Error("exclude flag not found")
	}

	dryRunFlag := migrateCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("dry-run flag not found")
	}

	resumeFlag := migrateCmd.Flags().Lookup("resume")
	if resumeFlag == nil {
		t.Error("resume flag not found")
	}
}

func TestMigrateCmd_NoArgs(t *testing.T) {
	// Test that command requires exactly one argument
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"migrate"})

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when no arguments provided")
	}

	output := buf.String()
	if !strings.Contains(output, "requires") && !strings.Contains(output, "arg") {
		t.Errorf("expected error message about arguments, got: %s", output)
	}
}

func TestMigrateCmd_NotAuthenticated(t *testing.T) {
	// Create a temporary config directory
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"migrate", "."})

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when not authenticated")
	}

	if !strings.Contains(err.Error(), "not logged in") {
		t.Errorf("expected 'not logged in' error, got: %s", err.Error())
	}
}
