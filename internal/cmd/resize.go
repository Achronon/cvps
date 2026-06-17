package cmd

import (
	"context"
	"fmt"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/spf13/cobra"
)

var resizeStorage int

var resizeCmd = &cobra.Command{
	Use:   "resize [sandbox]",
	Short: "Grow a sandbox's workspace storage",
	Long: `Grow a sandbox's persistent workspace storage.

Storage is grow-only: the new --storage value must be larger than the current
size. Longhorn expands the volume online (no pod restart) and the workspace
filesystem is resized automatically.

The sandbox may be given by ID or name. Without one, the current context
(.cvps.yaml) is used.`,
	Example: `  # Grow the current sandbox to 50 GB
  cvps resize --storage 50

  # Grow a specific sandbox by name or ID
  cvps resize my-project --storage 100
  cvps resize sbx-abc123 --storage 100`,
	Args: cobra.MaximumNArgs(1),
	RunE: runResize,
}

func init() {
	resizeCmd.Flags().IntVar(&resizeStorage, "storage", 0, "new total workspace storage in GB (grow only)")
	rootCmd.AddCommand(resizeCmd)
}

func runResize(cmd *cobra.Command, args []string) error {
	if resizeStorage <= 0 {
		return fmt.Errorf("--storage is required and must be a positive number of GB")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' or set CVPS_API_TOKEN")
	}

	client, err := api.NewClientFromConfig(cfg)
	if err != nil {
		return err
	}

	ctx := context.Background()

	sandboxID := ""
	if len(args) > 0 {
		id, err := resolveSandboxRef(ctx, client, args[0])
		if err != nil {
			return err
		}
		sandboxID = id
	} else {
		id, err := getCurrentSandboxID()
		if err != nil {
			return err
		}
		sandboxID = id
	}

	fmt.Printf("Resizing sandbox %s to %dGB...\n", sandboxID, resizeStorage)

	sandbox, err := client.ResizeSandbox(ctx, sandboxID, resizeStorage)
	if err != nil {
		if api.IsNotFound(err) {
			return fmt.Errorf("sandbox not found: %s", sandboxID)
		}
		return fmt.Errorf("failed to resize sandbox: %w", err)
	}

	fmt.Printf("Sandbox %s storage is now %dGB.\n", sandbox.ID, sandbox.StorageGB)
	return nil
}
