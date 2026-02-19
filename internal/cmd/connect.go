package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/achronon/cvps/internal/terminal"
	"github.com/spf13/cobra"
)

var (
	connectMethod string
	connectName   string
)

var (
	connectLoadConfig = config.Load
	connectLoginOAuth = loginWithOAuth
)

var (
	connectCUIDLikePattern = regexp.MustCompile(`^c[a-z0-9]{20,}$`)
)

var connectCmd = &cobra.Command{
	Use:   "connect [sandbox-id]",
	Short: "Open terminal to sandbox",
	Long: `Open an interactive terminal session to a sandbox.

By default, uses SSH.

Use either a sandbox ID argument or --name to select a sandbox.`,
	Example: `  # Connect to current sandbox
  cvps connect

  # Connect to specific sandbox
  cvps connect sbx-abc123

  # Connect by exact sandbox name
  cvps connect --name openclaw

  # Force SSH connection
  cvps connect --method ssh`,
	RunE: runConnect,
}

func init() {
	rootCmd.AddCommand(connectCmd)

	connectCmd.Flags().StringVarP(&connectMethod, "method", "m", "", "connection method (ssh|websocket)")
	connectCmd.Flags().StringVar(&connectName, "name", "", "sandbox name (exact match, alternative to sandbox ID argument)")
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

	sandboxID, err := resolveSandboxIDForConnect(ctx, client, args, connectName)
	if err != nil {
		return err
	}

	// Get sandbox info
	sandbox, err := client.GetSandbox(ctx, sandboxID)
	if err != nil {
		if api.IsNotFound(err) {
			if len(args) > 0 {
				message := fmt.Sprintf("sandbox not found: %s", args[0])
				if !looksLikeSandboxID(args[0]) {
					message += fmt.Sprintf(". If you meant a name, use 'cvps connect --name %s'.", args[0])
				}
				return fmt.Errorf(message)
			}

			if connectName != "" {
				return fmt.Errorf("sandbox named %q no longer exists. Run 'cvps status --all' and try again", connectName)
			}

			return fmt.Errorf("sandbox not found: %s", sandboxID)
		}

		return fmt.Errorf("failed to get sandbox: %w", err)
	}

	if !isRunningStatus(sandbox.Status) {
		return fmt.Errorf("sandbox is not running (status: %s)", sandbox.Status)
	}

	method, err := resolveConnectMethod(connectMethod, sandbox)
	if err != nil {
		return err
	}

	fmt.Printf("Connecting to sandbox %s via %s...\n", sandbox.Name, method)

	switch method {
	case "ssh":
		return connectSSH(sandbox)
	case "websocket":
		return connectWebSocket(ctx, client, sandbox)
	default:
		return fmt.Errorf("unknown connection method: %s", method)
	}
}

func resolveSandboxIDForConnect(ctx context.Context, client *api.Client, args []string, byName string) (string, error) {
	if len(args) > 0 && byName != "" {
		return "", fmt.Errorf("provide either a sandbox ID argument or --name, not both")
	}

	if len(args) > 0 {
		return args[0], nil
	}

	if byName != "" {
		return resolveSandboxIDByName(ctx, client, byName)
	}

	id, err := getCurrentSandboxID()
	if err != nil {
		return "", fmt.Errorf("no sandbox specified: %w", err)
	}

	return id, nil
}

func resolveSandboxIDByName(ctx context.Context, client *api.Client, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("sandbox name cannot be empty")
	}

	sandboxes, err := listAllSandboxesForConnect(ctx, client)
	if err != nil {
		return "", fmt.Errorf("failed to list sandboxes: %w", err)
	}

	matches := make([]api.Sandbox, 0, 2)
	for _, sandbox := range sandboxes {
		if strings.EqualFold(strings.TrimSpace(sandbox.Name), name) {
			matches = append(matches, sandbox)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("sandbox named %q not found. Run 'cvps status --all' to view available sandboxes", name)
	case 1:
		return matches[0].ID, nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "sandbox name %q is ambiguous. Use a sandbox ID:\n", name)
		for _, sandbox := range matches {
			fmt.Fprintf(&b, "  - %s (%s)\n", sandbox.ID, sandbox.Name)
		}
		return "", fmt.Errorf(strings.TrimRight(b.String(), "\n"))
	}
}

func listAllSandboxesForConnect(ctx context.Context, client *api.Client) ([]api.Sandbox, error) {
	const pageSize = 100
	const maxPages = 20

	all := make([]api.Sandbox, 0, pageSize)
	for page := 1; page <= maxPages; page++ {
		list, err := client.ListSandboxes(ctx, page, pageSize)
		if err != nil {
			return nil, err
		}

		all = append(all, list.Data...)
		if len(list.Data) < pageSize || len(all) >= list.Total {
			break
		}
	}

	return all, nil
}

func resolveConnectMethod(requested string, sandbox *api.Sandbox) (string, error) {
	method := strings.ToLower(strings.TrimSpace(requested))

	switch method {
	case "":
		if sandbox.SSHHost != "" && isSSHAvailable() {
			return "ssh", nil
		}
		return "websocket", nil
	case "ssh":
		if sandbox.SSHHost == "" {
			return "", fmt.Errorf("SSH connection is not available for this sandbox")
		}
		return "ssh", nil
	case "websocket":
		return "websocket", nil
	default:
		return "", fmt.Errorf("unknown connection method: %s", requested)
	}
}

func looksLikeSandboxID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	return strings.HasPrefix(value, "sbx-") || connectCUIDLikePattern.MatchString(value)
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

func connectWebSocket(ctx context.Context, client *api.Client, sandbox *api.Sandbox) error {
	// Get terminal websocket info from API
	wsInfo, err := client.GetTerminalWebSocket(ctx, sandbox.ID)
	if err != nil {
		return fmt.Errorf("failed to get terminal connection: %w", err)
	}

	// Create Socket.IO terminal connection
	term, err := terminal.NewSocketIOTerminal(wsInfo.URL, wsInfo.Token, sandbox.ID)
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
				_ = term.Resize(cols, rows)
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
		_ = term.Resize(cols, rows)
	}

	// Start I/O forwarding
	return term.Run(os.Stdin, os.Stdout)
}
