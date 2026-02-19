package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/achronon/cvps/internal/mutagen"
	"github.com/spf13/cobra"
)

var (
	syncIgnore  []string
	syncOneWay  string
	syncVerbose bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [local-path]",
	Short: "Sync files with sandbox",
	Long: `Start bidirectional file synchronization between local directory and sandbox.

Uses Mutagen for efficient, real-time file sync. Changes in either location
are automatically propagated to the other.`,
	Example: `  # Sync current directory
  cvps sync

  # Sync specific directory
  cvps sync ./my-project

  # One-way sync (local to remote only)
  cvps sync --one-way=local-to-remote

  # Custom ignore patterns
  cvps sync --ignore="*.log" --ignore="tmp/"`,
	RunE: runSync,
}

var syncStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	RunE:  runSyncStatus,
}

var syncStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop sync session",
	RunE:  runSyncStop,
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.AddCommand(syncStatusCmd)
	syncCmd.AddCommand(syncStopCmd)

	syncCmd.Flags().StringSliceVar(&syncIgnore, "ignore", nil, "patterns to ignore")
	syncCmd.Flags().StringVar(&syncOneWay, "one-way", "", "one-way sync (local-to-remote|remote-to-local)")
	syncCmd.Flags().BoolVarP(&syncVerbose, "verbose", "v", false, "verbose output")
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' first")
	}

	// Check Mutagen is installed
	if !mutagen.IsInstalled() {
		return fmt.Errorf("mutagen is not installed. Install it with: brew install mutagen-io/mutagen/mutagen")
	}

	client := api.NewClientFromConfig(cfg)
	ctx := context.Background()

	// Get sandbox ID
	sandboxID, err := getCurrentSandboxID()
	if err != nil {
		return fmt.Errorf("no sandbox specified: %w", err)
	}

	// Get sandbox info
	sandbox, err := client.GetSandbox(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("failed to get sandbox: %w", err)
	}

	if !isRunningStatus(sandbox.Status) {
		return fmt.Errorf("sandbox is not running (status: %s)", sandbox.Status)
	}

	// Determine local path
	localPath := "."
	if len(args) > 0 {
		localPath = args[0]
	}

	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Verify path exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", absPath)
	}

	// Build ignore patterns
	ignores := append(cfg.Sync.IgnorePatterns, syncIgnore...)

	// Validate one-way flag
	if syncOneWay != "" && syncOneWay != "local-to-remote" && syncOneWay != "remote-to-local" {
		return fmt.Errorf("invalid --one-way value: %s (must be 'local-to-remote' or 'remote-to-local')", syncOneWay)
	}

	// Create sync session
	fmt.Printf("Starting sync: %s ↔ sandbox:%s:/workspace\n", absPath, sandbox.ID)

	session, err := mutagen.CreateSession(mutagen.SessionConfig{
		Name:       fmt.Sprintf("cvps-%s", sandboxID),
		LocalPath:  absPath,
		RemoteHost: fmt.Sprintf("%s@%s", sandbox.SSHUser, sandbox.SSHHost),
		RemotePort: sandbox.SSHPort,
		RemotePath: "/workspace",
		Ignores:    ignores,
		OneWay:     syncOneWay,
		Verbose:    syncVerbose,
	})
	if err != nil {
		return fmt.Errorf("failed to create sync session: %w", err)
	}

	fmt.Printf("✓ Sync session created: %s\n", session.ID)
	fmt.Println("\nSync is running. Press Ctrl+C to stop.")
	fmt.Println("Use 'cvps sync status' to check progress.")

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if syncVerbose {
		// Monitor sync status in background
		go func() {
			if err := session.Monitor(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Monitor error: %v\n", err)
			}
		}()
	}

	<-sigChan

	fmt.Println("\nStopping sync...")
	if err := session.Terminate(); err != nil {
		fmt.Printf("Warning: failed to terminate session: %s\n", err)
	}

	return nil
}

func runSyncStatus(cmd *cobra.Command, args []string) error {
	if !mutagen.IsInstalled() {
		return fmt.Errorf("mutagen is not installed")
	}

	sandboxID, err := getCurrentSandboxID()
	if err != nil {
		return fmt.Errorf("no sandbox context found")
	}

	sessionName := fmt.Sprintf("cvps-%s", sandboxID)
	status, err := mutagen.GetSessionStatus(sessionName)
	if err != nil {
		return fmt.Errorf("no active sync session: %w", err)
	}

	fmt.Printf("Session: %s\n", sessionName)
	fmt.Printf("Status:  %s\n", status.Status)
	fmt.Printf("Local:   %s\n", status.LocalPath)
	fmt.Printf("Remote:  %s\n", status.RemotePath)

	if status.Conflicts > 0 {
		fmt.Printf("\n⚠ Conflicts: %d\n", status.Conflicts)
		fmt.Println("Run 'mutagen sync list' to view details")
	}

	return nil
}

func runSyncStop(cmd *cobra.Command, args []string) error {
	if !mutagen.IsInstalled() {
		return fmt.Errorf("mutagen is not installed")
	}

	sandboxID, err := getCurrentSandboxID()
	if err != nil {
		return fmt.Errorf("no sandbox context found")
	}

	sessionName := fmt.Sprintf("cvps-%s", sandboxID)
	if err := mutagen.TerminateSession(sessionName); err != nil {
		return fmt.Errorf("failed to stop sync: %w", err)
	}

	fmt.Println("✓ Sync session stopped")
	return nil
}
