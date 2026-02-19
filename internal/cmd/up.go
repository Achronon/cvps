package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	upName    string
	upCPU     int
	upMemory  int
	upStorage int
	upDetach  bool
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Provision a remote sandbox",
	Long: `Provision a new remote sandbox instance on claudevps.com.

The sandbox will be created with the specified resources and become
available for connections once provisioning completes.`,
	Example: `  # Create sandbox with defaults
  cvps up

  # Create named sandbox with custom resources
  cvps up --name my-project --cpu 4 --memory 8 --storage 50

  # Create and return immediately without waiting
  cvps up --detach`,
	RunE: runUp,
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().StringVarP(&upName, "name", "n", "", "sandbox name")
	upCmd.Flags().IntVar(&upCPU, "cpu", 0, "CPU cores (default from config)")
	upCmd.Flags().IntVar(&upMemory, "memory", 0, "memory in GB (default from config)")
	upCmd.Flags().IntVar(&upStorage, "storage", 0, "storage in GB (default from config)")
	upCmd.Flags().BoolVarP(&upDetach, "detach", "d", false, "return immediately without waiting")
}

func runUp(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' first")
	}

	client := api.NewClientFromConfig(cfg)

	// Build create request
	req := &api.CreateSandboxRequest{
		Name:      upName,
		CPUCores:  upCPU,
		MemoryGB:  upMemory,
		StorageGB: upStorage,
	}

	// Apply defaults
	if req.CPUCores == 0 {
		req.CPUCores = cfg.Defaults.CPUCores
	}
	if req.MemoryGB == 0 {
		req.MemoryGB = cfg.Defaults.MemoryGB
	}
	if req.StorageGB == 0 {
		req.StorageGB = cfg.Defaults.StorageGB
	}
	if req.Name == "" {
		req.Name = fmt.Sprintf("sandbox-%d", time.Now().Unix())
	}

	// Create sandbox
	fmt.Printf("Creating sandbox '%s'...\n", req.Name)

	ctx := context.Background()
	sandbox, err := client.CreateSandbox(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to create sandbox: %w", err)
	}

	fmt.Printf("Sandbox created: %s\n", sandbox.ID)

	if upDetach {
		fmt.Println("\nSandbox is provisioning. Use 'cvps status' to check progress.")
		saveLocalContext(sandbox.ID, sandbox.Name)
		return nil
	}

	// Wait for sandbox to be ready
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Provisioning sandbox..."
	s.Start()

	timeout := 5 * time.Minute
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := client.GetSandboxStatus(ctx, sandbox.ID)
		if err != nil {
			s.Stop()
			return fmt.Errorf("failed to get status: %w", err)
		}

		switch status.Status {
		case "running":
			s.Stop()
			printSandboxReady(status)
			saveLocalContext(sandbox.ID, sandbox.Name)
			return nil

		case "failed", "error":
			s.Stop()
			return fmt.Errorf("sandbox provisioning failed: %s", status.Status)

		default:
			s.Suffix = fmt.Sprintf(" %s...", status.Status)
		}

		time.Sleep(2 * time.Second)
	}

	s.Stop()
	return fmt.Errorf("timeout waiting for sandbox to be ready (waited %s)", timeout)
}

func printSandboxReady(sandbox *api.Sandbox) {
	fmt.Println("\n✓ Sandbox is ready!")

	fmt.Println("Resources:")
	fmt.Printf("  CPU:     %d cores\n", sandbox.CPUCores)
	fmt.Printf("  Memory:  %d GB\n", sandbox.MemoryGB)
	fmt.Printf("  Storage: %d GB\n", sandbox.StorageGB)

	if sandbox.SSHHost != "" {
		fmt.Println("\nConnection:")
		fmt.Printf("  SSH:  ssh %s@%s -p %d\n", sandbox.SSHUser, sandbox.SSHHost, sandbox.SSHPort)
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  cvps connect     - Open terminal to sandbox")
	fmt.Println("  cvps sync        - Start file synchronization")
	fmt.Println("  cvps status      - Check sandbox status")
	fmt.Println("  cvps down        - Terminate sandbox")
}

// LocalContext stores current sandbox context in working directory
type LocalContext struct {
	SandboxID string `yaml:"sandbox_id"`
	Name      string `yaml:"name,omitempty"`
	CreatedAt string `yaml:"created_at"`
}

func saveLocalContext(sandboxID, name string) error {
	ctx := LocalContext{
		SandboxID: sandboxID,
		Name:      name,
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	data, err := yaml.Marshal(ctx)
	if err != nil {
		return err
	}

	return os.WriteFile(".cvps.yaml", data, 0644)
}

func loadLocalContext() (*LocalContext, error) {
	data, err := os.ReadFile(".cvps.yaml")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ctx LocalContext
	if err := yaml.Unmarshal(data, &ctx); err != nil {
		return nil, err
	}

	return &ctx, nil
}

func getCurrentSandboxID() (string, error) {
	ctx, err := loadLocalContext()
	if err != nil {
		return "", err
	}
	if ctx == nil {
		return "", fmt.Errorf("no sandbox context. Run 'cvps up' first or pass a sandbox ID as the first argument")
	}
	return ctx.SandboxID, nil
}
