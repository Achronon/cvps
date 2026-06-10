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

var restartCmd = &cobra.Command{
	Use:   "restart [sandbox-id]",
	Short: "Restart a running sandbox",
	Long: `Restart a running sandbox (stop + re-provision).

Service sandboxes scale to zero and re-apply their workload; the persistent
workspace volume is preserved.

Without a sandbox ID, uses the current context (.cvps.yaml).`,
	Example: `  # Restart the current sandbox
  cvps restart

  # Restart a specific sandbox
  cvps restart sbx-abc123`,
	RunE: runRestart,
}

func init() {
	rootCmd.AddCommand(restartCmd)
}

func runRestart(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' first")
	}

	sandboxID := ""
	if len(args) > 0 {
		sandboxID = args[0]
	} else {
		id, err := getCurrentSandboxID()
		if err != nil {
			return err
		}
		sandboxID = id
	}

	client := api.NewClientFromConfig(cfg)
	fmt.Printf("Restarting sandbox %s...\n", sandboxID)

	sandbox, err := client.RestartSandbox(context.Background(), sandboxID)
	if err != nil {
		if api.IsNotFound(err) {
			return fmt.Errorf("sandbox not found: %s", sandboxID)
		}
		// Only the backend's not-running validation earns the bootstrap hint —
		// auth/5xx/transport failures must not send users down that path.
		var apiErr *api.APIError
		if errors.As(err, &apiErr) &&
			apiErr.StatusCode == 400 &&
			strings.Contains(apiErr.Message, "Cannot restart sandbox with status") {
			return fmt.Errorf("failed to restart sandbox: %w\n\nOnly RUNNING sandboxes can be restarted. For a service that never became\nready, finish its bootstrap first ('cvps connect' then set up model auth),\nor use stop/start from the dashboard", err)
		}
		return fmt.Errorf("failed to restart sandbox: %w", err)
	}

	fmt.Printf("Sandbox %s is %s. Use 'cvps status --watch' to follow it.\n", sandbox.ID, sandbox.Status)
	return nil
}
