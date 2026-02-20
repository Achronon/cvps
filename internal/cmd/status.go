package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	statusAll   bool
	statusJSON  bool
	statusWatch bool
)

var statusCmd = &cobra.Command{
	Use:   "status [sandbox-id]",
	Short: "Show sandbox status",
	Long: `Show the status of sandboxes.

Without arguments, shows the status of the current context sandbox
(determined by .cvps.yaml in the current directory).
If no local context exists, falls back to listing all sandboxes.`,
	Example: `  # Show current sandbox status
  cvps status

  # Show all sandboxes
  cvps status --all

  # Show specific sandbox
  cvps status sbx-abc123

  # Watch status continuously
  cvps status --watch`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().BoolVarP(&statusAll, "all", "a", false, "list all sandboxes")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "output in JSON format")
	statusCmd.Flags().BoolVarP(&statusWatch, "watch", "w", false, "watch for changes")
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' first")
	}

	client := api.NewClientFromConfig(cfg)
	ctx := context.Background()

	// List all sandboxes
	if statusAll {
		if statusWatch {
			return watchAllSandboxes(ctx, client)
		}
		return listAllSandboxes(ctx, client)
	}

	// Get sandbox ID from args or context
	sandboxID := ""
	if len(args) > 0 {
		sandboxID = args[0]
	} else {
		id, err := getCurrentSandboxID()
		if err != nil {
			if statusWatch {
				fmt.Println("No current sandbox context found. Watching all sandboxes instead.")
				return watchAllSandboxes(ctx, client)
			}

			fmt.Println("No current sandbox context found. Showing all sandboxes:")
			return listAllSandboxes(ctx, client)
		}
		sandboxID = id
	}

	if statusWatch {
		return watchSandbox(ctx, client, sandboxID)
	}

	return showSandboxStatus(ctx, client, sandboxID)
}

func listAllSandboxes(ctx context.Context, client *api.Client) error {
	list, err := client.ListSandboxes(ctx, 1, 100)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	if statusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(list.Data)
	}

	if len(list.Data) == 0 {
		fmt.Println("No sandboxes found. Run 'cvps up' to create one.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATUS\tCPU\tMEMORY\tCREATED")

	for _, s := range list.Data {
		status := colorStatus(s.Status)
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%dGB\t%s\n",
			s.ID, s.Name, status, s.CPUCores, s.MemoryGB, formatTime(s.CreatedAt))
	}

	w.Flush()
	return nil
}

func showSandboxStatus(ctx context.Context, client *api.Client, sandboxID string) error {
	sandbox, err := client.GetSandbox(ctx, sandboxID)
	if err != nil {
		if api.IsNotFound(err) {
			return fmt.Errorf("sandbox not found: %s", sandboxID)
		}
		return fmt.Errorf("failed to get sandbox: %w", err)
	}

	if statusJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sandbox)
	}

	printSandboxDetails(sandbox)
	return nil
}

func printSandboxDetails(s *api.Sandbox) {
	fmt.Printf("Sandbox: %s\n", s.Name)
	fmt.Printf("ID:      %s\n", s.ID)
	fmt.Printf("Status:  %s\n", colorStatus(s.Status))
	fmt.Println()

	fmt.Println("Resources:")
	fmt.Printf("  CPU:     %d cores\n", s.CPUCores)
	fmt.Printf("  Memory:  %d GB\n", s.MemoryGB)
	fmt.Printf("  Storage: %d GB\n", s.StorageGB)
	fmt.Println()

	fmt.Printf("Created: %s\n", formatTime(s.CreatedAt))
	if s.LastActive != "" {
		fmt.Printf("Last Active: %s\n", formatTime(s.LastActive))
	}

	if isRunningStatus(s.Status) && s.SSHHost != "" {
		fmt.Println()
		fmt.Println("Connection:")
		fmt.Printf("  SSH: ssh %s@%s -p %d\n", s.SSHUser, s.SSHHost, s.SSHPort)
		if s.Connectivity.SSHProxyRequired {
			fmt.Println("  Note: ProxyCommand is required for this route (cloudflared).")
		}
		return
	}

	if isRunningStatus(s.Status) && s.SSHHost == "" {
		fmt.Println()
		fmt.Println("Connection:")
		fmt.Println("  SSH endpoint not ready yet.")
		fmt.Printf("  Use: cvps connect %s\n", s.ID)
	}
}

func watchSandbox(ctx context.Context, client *api.Client, sandboxID string) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastStatus := ""

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			sandbox, err := client.GetSandboxStatus(ctx, sandboxID)
			if err != nil {
				fmt.Printf("Error: %s\n", err)
				continue
			}

			if sandbox.Status != lastStatus {
				// Clear screen
				fmt.Print("\033[H\033[2J")
				printSandboxDetails(sandbox)
				lastStatus = sandbox.Status
			}
		}
	}
}

func watchAllSandboxes(ctx context.Context, client *api.Client) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Clear screen
			fmt.Print("\033[H\033[2J")
			fmt.Printf("Sandboxes (updated: %s)\n\n", time.Now().Format(time.RFC3339))
			if err := listAllSandboxes(ctx, client); err != nil {
				fmt.Printf("Error: %s\n", err)
			}
		}
	}
}

func colorStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return color.GreenString(status)
	case "provisioning", "starting":
		return color.YellowString(status)
	case "stopped":
		return color.HiBlackString(status)
	case "failed", "error":
		return color.RedString(status)
	default:
		return status
	}
}

func formatTime(t string) string {
	parsed, err := time.Parse(time.RFC3339, t)
	if err != nil {
		return t
	}
	return parsed.Local().Format("2006-01-02 15:04:05")
}
