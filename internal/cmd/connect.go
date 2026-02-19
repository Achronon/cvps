package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/achronon/cvps/internal/terminal"
	"github.com/spf13/cobra"
)

var (
	connectMethod string
)

var (
	connectLoadConfig = config.Load
	connectLoginOAuth = loginWithOAuth
)

var connectCmd = &cobra.Command{
	Use:   "connect [sandbox-id]",
	Short: "Open terminal to sandbox",
	Long: `Open an interactive terminal session to a sandbox.

By default, uses SSH if available, falling back to WebSocket
for environments with restricted SSH access.`,
	Example: `  # Connect to current sandbox
  cvps connect

  # Connect to specific sandbox
  cvps connect sbx-abc123

  # Force SSH connection
  cvps connect --method ssh

  # Force WebSocket connection
  cvps connect --method websocket`,
	RunE: runConnect,
}

func init() {
	rootCmd.AddCommand(connectCmd)

	connectCmd.Flags().StringVarP(&connectMethod, "method", "m", "", "connection method (ssh|websocket)")
}

func runConnect(cmd *cobra.Command, args []string) error {
	cfg, err := connectLoadConfig()
	if err != nil {
		return err
	}

	cfg, err = ensureConnectedAuth(cfg)
	if err != nil {
		return err
	}

	client := api.NewClientFromConfig(cfg)
	ctx := context.Background()

	// Get sandbox ID
	sandboxID := ""
	if len(args) > 0 {
		sandboxID = args[0]
	} else {
		id, err := getCurrentSandboxID()
		if err != nil {
			return fmt.Errorf("no sandbox specified: %w", err)
		}
		sandboxID = id
	}

	// Get sandbox info
	sandbox, err := client.GetSandbox(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("failed to get sandbox: %w", err)
	}

	if !isRunningStatus(sandbox.Status) {
		return fmt.Errorf("sandbox is not running (status: %s)", sandbox.Status)
	}

	// Determine connection method
	method := connectMethod
	if method == "" {
		// Auto-detect: prefer SSH if available
		if sandbox.SSHHost != "" && isSSHAvailable() {
			method = "ssh"
		} else {
			method = "websocket"
		}
	}

	fmt.Printf("Connecting to sandbox %s via %s...\n", sandbox.Name, method)

	switch method {
	case "ssh":
		return connectSSH(sandbox)
	case "websocket":
		return connectWebSocket(ctx, client, sandbox, cfg)
	default:
		return fmt.Errorf("unknown connection method: %s", method)
	}
}

func ensureConnectedAuth(cfg *config.Config) (*config.Config, error) {
	if cfg.IsAuthenticated() {
		return cfg, nil
	}

	fmt.Println("Not logged in. Starting browser authentication...")
	if err := connectLoginOAuth(cfg); err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	refreshed, err := connectLoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to reload config after authentication: %w", err)
	}
	if !refreshed.IsAuthenticated() {
		return nil, fmt.Errorf("authentication did not produce credentials. Run 'cvps login' and try again")
	}

	return refreshed, nil
}

func isSSHAvailable() bool {
	_, err := exec.LookPath("ssh")
	return err == nil
}

func connectSSH(sandbox *api.Sandbox) error {
	if sandbox.SSHHost == "" {
		return fmt.Errorf("SSH not available for this sandbox")
	}

	// Build SSH command
	sshArgs := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "LogLevel=ERROR",
		"-p", fmt.Sprintf("%d", sandbox.SSHPort),
		fmt.Sprintf("%s@%s", sandbox.SSHUser, sandbox.SSHHost),
	}

	// Execute SSH
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found in PATH")
	}

	// Replace current process with SSH
	return syscall.Exec(sshPath, append([]string{"ssh"}, sshArgs...), os.Environ())
}

func connectWebSocket(ctx context.Context, client *api.Client, sandbox *api.Sandbox, cfg *config.Config) error {
	// Get WebSocket URL from API
	wsInfo, err := client.GetTerminalWebSocket(ctx, sandbox.ID)
	if err != nil {
		return fmt.Errorf("failed to get terminal connection: %w", err)
	}

	// Create terminal connection
	term, err := terminal.NewWebSocketTerminal(wsInfo.URL, wsInfo.Token)
	if err != nil {
		return fmt.Errorf("failed to create terminal: %w", err)
	}
	defer term.Close()

	// Handle terminal resize
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGWINCH)
	go func() {
		for range sigChan {
			if cols, rows, err := terminal.GetSize(); err == nil {
				term.Resize(cols, rows)
			}
		}
	}()

	// Set raw mode
	restore, err := terminal.SetRaw()
	if err != nil {
		return fmt.Errorf("failed to set terminal mode: %w", err)
	}
	defer restore()

	// Send initial resize
	if cols, rows, err := terminal.GetSize(); err == nil {
		term.Resize(cols, rows)
	}

	// Start I/O forwarding
	return term.Run(os.Stdin, os.Stdout)
}
