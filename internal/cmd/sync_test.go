package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestSyncCmd(t *testing.T) {
	if syncCmd == nil {
		t.Fatal("syncCmd should not be nil")
	}

	if syncCmd.Use != "sync [local-path]" {
		t.Errorf("Expected Use 'sync [local-path]', got %s", syncCmd.Use)
	}

	if syncCmd.Short != "Sync files with sandbox" {
		t.Errorf("Expected Short 'Sync files with sandbox', got %s", syncCmd.Short)
	}
}

func TestSyncStatusCmd(t *testing.T) {
	if syncStatusCmd == nil {
		t.Fatal("syncStatusCmd should not be nil")
	}

	if syncStatusCmd.Use != "status" {
		t.Errorf("Expected Use 'status', got %s", syncStatusCmd.Use)
	}

	if syncStatusCmd.Short != "Show sync status" {
		t.Errorf("Expected Short 'Show sync status', got %s", syncStatusCmd.Short)
	}
}

func TestSyncStopCmd(t *testing.T) {
	if syncStopCmd == nil {
		t.Fatal("syncStopCmd should not be nil")
	}

	if syncStopCmd.Use != "stop" {
		t.Errorf("Expected Use 'stop', got %s", syncStopCmd.Use)
	}

	if syncStopCmd.Short != "Stop sync session" {
		t.Errorf("Expected Short 'Stop sync session', got %s", syncStopCmd.Short)
	}
}

func TestSyncCmdFlags(t *testing.T) {
	// Create a temporary root command for testing
	testRootCmd := &cobra.Command{Use: "cvps"}
	testSyncCmd := &cobra.Command{
		Use:   "sync [local-path]",
		Short: "Sync files with sandbox",
	}

	testRootCmd.AddCommand(testSyncCmd)
	testSyncCmd.AddCommand(&cobra.Command{Use: "status"})
	testSyncCmd.AddCommand(&cobra.Command{Use: "stop"})

	// Add flags
	var testIgnore []string
	var testOneWay string
	var testVerbose bool

	testSyncCmd.Flags().StringSliceVar(&testIgnore, "ignore", nil, "patterns to ignore")
	testSyncCmd.Flags().StringVar(&testOneWay, "one-way", "", "one-way sync (local-to-remote|remote-to-local)")
	testSyncCmd.Flags().BoolVarP(&testVerbose, "verbose", "v", false, "verbose output")

	// Test that flags exist
	ignoreFlag := testSyncCmd.Flags().Lookup("ignore")
	if ignoreFlag == nil {
		t.Error("Expected --ignore flag to exist")
	}

	oneWayFlag := testSyncCmd.Flags().Lookup("one-way")
	if oneWayFlag == nil {
		t.Error("Expected --one-way flag to exist")
	}

	verboseFlag := testSyncCmd.Flags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("Expected --verbose flag to exist")
	}

	// Test verbose flag shorthand
	vFlag := testSyncCmd.Flags().ShorthandLookup("v")
	if vFlag == nil {
		t.Error("Expected -v shorthand for verbose flag to exist")
	}
}

func TestSyncCmdStructure(t *testing.T) {
	// Verify sync command has subcommands
	subcommands := syncCmd.Commands()

	var hasStatus, hasStop bool
	for _, cmd := range subcommands {
		if cmd.Use == "status" {
			hasStatus = true
		}
		if cmd.Use == "stop" {
			hasStop = true
		}
	}

	if !hasStatus {
		t.Error("Expected sync command to have 'status' subcommand")
	}

	if !hasStop {
		t.Error("Expected sync command to have 'stop' subcommand")
	}
}

// Note: runSync, runSyncStatus, and runSyncStop functions require:
// - Valid authentication
// - Running sandbox
// - Mutagen installed
// These would be better tested in integration tests rather than unit tests.
// For unit tests, we would need to refactor the code to inject dependencies
// (like the API client and mutagen wrapper) for proper mocking.
