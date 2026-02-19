package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/achronon/cvps/internal/migration"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

var (
	migrateExclude []string
	migrateDryRun  bool
	migrateResume  bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate <local-path>",
	Short: "Migrate local workspace to sandbox",
	Long: `Upload a local workspace directory to your sandbox.

This is typically used once when moving from local development to ClaudeVPS.
For ongoing synchronization, use 'cvps sync' instead.`,
	Example: `  # Migrate current directory
  cvps migrate .

  # Migrate specific directory
  cvps migrate ~/projects/my-app

  # Exclude patterns
  cvps migrate . --exclude="node_modules" --exclude="*.log"

  # Preview without uploading
  cvps migrate . --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runMigrate,
}

func init() {
	rootCmd.AddCommand(migrateCmd)

	migrateCmd.Flags().StringSliceVar(&migrateExclude, "exclude", nil, "patterns to exclude")
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "preview migration without uploading")
	migrateCmd.Flags().BoolVar(&migrateResume, "resume", false, "resume interrupted migration")
}

func runMigrate(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' first")
	}

	client := api.NewClientFromConfig(cfg)
	ctx := context.Background()

	// Get sandbox ID
	sandboxID, err := getCurrentSandboxID()
	if err != nil {
		return fmt.Errorf("no sandbox specified: %w", err)
	}

	// Verify sandbox is running
	sandbox, err := client.GetSandbox(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("failed to get sandbox: %w", err)
	}

	if sandbox.Status != "running" {
		return fmt.Errorf("sandbox is not running (status: %s). Start it with 'cvps up'", sandbox.Status)
	}

	// Resolve local path
	localPath := args[0]
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Verify path exists
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", absPath)
	}
	if !info.IsDir() {
		return fmt.Errorf("path must be a directory: %s", absPath)
	}

	// Build exclusion patterns
	excludes := append(cfg.Sync.IgnorePatterns, migrateExclude...)

	// Scan directory
	fmt.Println("Scanning files...")
	scanner := migration.NewScanner(absPath, excludes)
	files, err := scanner.Scan()
	if err != nil {
		return fmt.Errorf("failed to scan directory: %w", err)
	}

	// Print summary
	fmt.Printf("\nMigration Summary:\n")
	fmt.Printf("  Files:  %d\n", files.Count)
	fmt.Printf("  Size:   %s\n", formatBytes(files.TotalSize))
	fmt.Printf("  From:   %s\n", absPath)
	fmt.Printf("  To:     %s:/workspace\n", sandbox.Name)
	fmt.Println()

	if migrateDryRun {
		fmt.Println("Dry run - no files uploaded")
		fmt.Println("\nTop 10 largest files:")
		for i, f := range files.LargestFiles(10) {
			fmt.Printf("  %d. %s (%s)\n", i+1, f.RelPath, formatBytes(f.Size))
		}
		return nil
	}

	// Confirm
	fmt.Print("Continue with migration? (y/N): ")
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "y" && confirm != "Y" {
		return fmt.Errorf("migration cancelled")
	}

	// Create migrator
	migrator := migration.NewMigrator(migration.Config{
		LocalPath:  absPath,
		SSHHost:    sandbox.SSHHost,
		SSHPort:    sandbox.SSHPort,
		SSHUser:    sandbox.SSHUser,
		RemotePath: "/workspace",
		Resume:     migrateResume,
	})

	// Progress bar
	bar := progressbar.NewOptions64(
		files.TotalSize,
		progressbar.OptionSetDescription("Migrating"),
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionFullWidth(),
	)

	// Run migration
	startTime := time.Now()
	result, err := migrator.Run(ctx, files, func(bytesTransferred int64) {
		bar.Set64(bytesTransferred)
	})
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	bar.Finish()
	fmt.Println()

	// Print results
	elapsed := time.Since(startTime)
	fmt.Printf("✓ Migration complete!\n")
	fmt.Printf("  Files transferred: %d\n", result.FilesTransferred)
	fmt.Printf("  Data transferred:  %s\n", formatBytes(result.BytesTransferred))
	fmt.Printf("  Time elapsed:      %s\n", elapsed.Round(time.Second))
	fmt.Printf("  Average speed:     %s/s\n", formatBytes(int64(float64(result.BytesTransferred)/elapsed.Seconds())))

	if result.FilesSkipped > 0 {
		fmt.Printf("  Files skipped:     %d\n", result.FilesSkipped)
	}

	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
