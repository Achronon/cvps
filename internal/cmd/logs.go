package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/spf13/cobra"
)

var logsTail int

var logsCmd = &cobra.Command{
	Use:   "logs [sandbox-id]",
	Short: "Show recent sandbox logs",
	Long: `Show recent logs from the sandbox's main container.

For service sandboxes (e.g. agent services like cortex) this is the primary
observability surface — the workload runs headless, with no terminal attached.

Without a sandbox ID, uses the current context (.cvps.yaml).`,
	Example: `  # Logs of the current sandbox
  cvps logs

  # Logs of a specific sandbox, last 500 lines
  cvps logs sbx-abc123 --tail 500`,
	RunE: runLogs,
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().IntVar(&logsTail, "tail", 200, "number of recent log lines to fetch (max 2000)")
}

func runLogs(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' or set CVPS_API_TOKEN")
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

	client, err := api.NewClientFromConfig(cfg)
	if err != nil {
		return err
	}
	logs, err := client.GetSandboxLogs(context.Background(), sandboxID, logsTail)
	if err != nil {
		if api.IsNotFound(err) {
			return fmt.Errorf("sandbox not found: %s", sandboxID)
		}
		return fmt.Errorf("failed to fetch logs: %w", err)
	}

	fmt.Fprint(os.Stdout, logs.Logs)
	if logs.Logs != "" && logs.Logs[len(logs.Logs)-1] != '\n' {
		fmt.Fprintln(os.Stdout)
	}
	return nil
}
