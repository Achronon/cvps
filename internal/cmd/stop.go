package cmd

import (
	"context"
	"fmt"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [sandbox-id|name]",
	Short: "Stop a sandbox without deleting it",
	Long: `Stop a running or provisioning sandbox.

The persistent workspace volume and sandbox identity are preserved. Use
'cvps start' to bring it back, including after a provisioning error.

Without a sandbox ID or name, uses the current context (.cvps.yaml).`,
	Example: `  # Stop the current sandbox
  cvps stop

  # Stop a specific sandbox by ID or name
  cvps stop sbx-abc123`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
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

	sandboxID, err := resolveLifecycleSandboxID(ctx, client, args)
	if err != nil {
		return err
	}

	fmt.Printf("Stopping sandbox %s...\n", sandboxID)
	sandbox, err := client.StopSandbox(ctx, sandboxID)
	if err != nil {
		if api.IsNotFound(err) {
			return fmt.Errorf("sandbox not found: %s", sandboxID)
		}
		return fmt.Errorf("failed to stop sandbox: %w", err)
	}

	fmt.Printf("Sandbox %s is %s. Use 'cvps start' to bring it back.\n", sandbox.ID, sandbox.Status)
	return nil
}
