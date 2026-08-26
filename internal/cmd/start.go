package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [sandbox-id|name]",
	Short: "Start or retry a sandbox",
	Long: `Start a stopped sandbox or retry an errored sandbox.

Starting an errored sandbox retries its normal provisioning path while
preserving the sandbox identity and persistent workspace volume.

Without a sandbox ID or name, uses the current context (.cvps.yaml).`,
	Example: `  # Start the current sandbox
  cvps start

  # Retry an errored service sandbox
  cvps start cortex-brain

  # Start a specific sandbox by ID
  cvps start sbx-abc123`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
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

	fmt.Printf("Starting sandbox %s...\n", sandboxID)
	sandbox, err := client.StartSandbox(ctx, sandboxID)
	if err != nil {
		if api.IsNotFound(err) {
			return fmt.Errorf("sandbox not found: %s", sandboxID)
		}
		var apiErr *api.APIError
		if errors.As(err, &apiErr) &&
			apiErr.StatusCode == 400 &&
			strings.Contains(apiErr.Message, "Cannot start sandbox with status") {
			return fmt.Errorf("failed to start sandbox: %w\n\nOnly STOPPED or ERROR sandboxes can be started. RUNNING sandboxes should use 'cvps restart'.", err)
		}
		return fmt.Errorf("failed to start sandbox: %w", err)
	}

	fmt.Printf("Sandbox %s is %s. Use 'cvps status --watch' to follow it.\n", sandbox.ID, sandbox.Status)
	return nil
}

func resolveLifecycleSandboxID(ctx context.Context, client *api.Client, args []string) (string, error) {
	if len(args) > 0 {
		return resolveSandboxRef(ctx, client, args[0])
	}

	id, err := getCurrentSandboxID()
	if err != nil {
		return "", fmt.Errorf("no sandbox specified and no context found: %w", err)
	}
	return id, nil
}
