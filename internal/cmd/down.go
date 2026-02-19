package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	downForce bool
	downAll   bool
)

var downCmd = &cobra.Command{
	Use:   "down [sandbox-id]",
	Short: "Terminate a sandbox",
	Long: `Terminate (delete) a sandbox.

Without arguments, terminates the current context sandbox
(determined by .cvps.yaml in the current directory).

Warning: This action is irreversible. All data in the sandbox will be lost.`,
	Example: `  # Terminate current sandbox
  cvps down

  # Terminate specific sandbox
  cvps down sbx-abc123

  # Force terminate without confirmation
  cvps down --force

  # Terminate all sandboxes
  cvps down --all`,
	RunE: runDown,
}

func init() {
	rootCmd.AddCommand(downCmd)

	downCmd.Flags().BoolVarP(&downForce, "force", "f", false, "skip confirmation prompt")
	downCmd.Flags().BoolVar(&downAll, "all", false, "terminate all sandboxes")
}

func runDown(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' first")
	}

	client := api.NewClientFromConfig(cfg)
	ctx := context.Background()

	// Terminate all sandboxes
	if downAll {
		return terminateAllSandboxes(ctx, client)
	}

	// Get sandbox ID from args or context
	sandboxID := ""
	if len(args) > 0 {
		sandboxID = args[0]
	} else {
		id, err := getCurrentSandboxID()
		if err != nil {
			return fmt.Errorf("no sandbox specified and no context found: %w", err)
		}
		sandboxID = id
	}

	return terminateSandbox(ctx, client, sandboxID)
}

func terminateSandbox(ctx context.Context, client *api.Client, sandboxID string) error {
	// Get sandbox info for confirmation
	sandbox, err := client.GetSandbox(ctx, sandboxID)
	if err != nil {
		if api.IsNotFound(err) {
			fmt.Printf("Sandbox %s not found (may already be deleted)\n", sandboxID)
			cleanupLocalContext(sandboxID)
			return nil
		}
		return fmt.Errorf("failed to get sandbox: %w", err)
	}

	// Confirm deletion
	if !downForce {
		warning := color.New(color.FgYellow, color.Bold)
		warning.Printf("⚠ Warning: This will permanently delete sandbox '%s' (%s)\n", sandbox.Name, sandboxID)
		fmt.Println("All data in the sandbox will be lost.")
		fmt.Print("\nType the sandbox name to confirm: ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input != sandbox.Name {
			return fmt.Errorf("confirmation failed: expected '%s', got '%s'", sandbox.Name, input)
		}
	}

	// Delete sandbox
	fmt.Printf("Terminating sandbox %s...\n", sandboxID)

	if err := client.DeleteSandbox(ctx, sandboxID); err != nil {
		return fmt.Errorf("failed to terminate sandbox: %w", err)
	}

	// Wait for termination
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Waiting for termination..."
	s.Start()

	timeout := 2 * time.Minute
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		_, err := client.GetSandbox(ctx, sandboxID)
		if err != nil {
			if api.IsNotFound(err) {
				s.Stop()
				fmt.Println("✓ Sandbox terminated successfully")
				cleanupLocalContext(sandboxID)
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}

	s.Stop()
	fmt.Println("✓ Sandbox termination initiated (may take a few more seconds)")
	cleanupLocalContext(sandboxID)
	return nil
}

func terminateAllSandboxes(ctx context.Context, client *api.Client) error {
	list, err := client.ListSandboxes(ctx, 1, 100)
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	if len(list.Data) == 0 {
		fmt.Println("No sandboxes to terminate.")
		return nil
	}

	// Confirm
	if !downForce {
		warning := color.New(color.FgRed, color.Bold)
		warning.Printf("⚠ DANGER: This will permanently delete ALL %d sandboxes!\n\n", len(list.Data))

		for _, s := range list.Data {
			fmt.Printf("  - %s (%s)\n", s.Name, s.ID)
		}

		fmt.Print("\nType 'delete all' to confirm: ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input != "delete all" {
			return fmt.Errorf("confirmation failed")
		}
	}

	// Delete all
	fmt.Println()
	for _, s := range list.Data {
		fmt.Printf("Terminating %s (%s)... ", s.Name, s.ID)
		if err := client.DeleteSandbox(ctx, s.ID); err != nil {
			fmt.Printf("failed: %s\n", err)
		} else {
			fmt.Println("done")
		}
	}

	// Cleanup local context
	os.Remove(".cvps.yaml")

	fmt.Printf("\n✓ Terminated %d sandboxes\n", len(list.Data))
	return nil
}

func cleanupLocalContext(sandboxID string) {
	localCtx, err := loadLocalContext()
	if err != nil || localCtx == nil {
		return
	}

	if localCtx.SandboxID == sandboxID {
		os.Remove(".cvps.yaml")
	}
}
