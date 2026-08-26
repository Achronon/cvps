package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/spf13/cobra"
)

var deployImage string

// Service image deploys are synchronous because the API health-gates the new
// revision and rolls back before returning. Keep this above the backend's
// documented 120s gate (and allow for image pulls and network variance), while
// leaving the ordinary client timeout unchanged for other commands.
const deployHTTPTimeout = 5 * time.Minute

var deployCmd = &cobra.Command{
	Use:   "deploy [sandbox-id|name]",
	Short: "Deploy an image to a service sandbox",
	Long: `Deploy an OCI image to a SERVICE-profile sandbox.

The backend validates image ownership and architecture, health-gates the new
revision, and automatically rolls back if the new workload does not become
healthy. The command waits up to five minutes for that synchronous gate. An
errored service can be repaired in place without deleting its workspace.

Without a sandbox ID or name, uses the current context (.cvps.yaml).`,
	Example: `  # Deploy to the current service sandbox
  cvps deploy --image ghcr.io/achronon/my-service:v2@sha256:<digest>

  # Repair a named service sandbox
  cvps deploy trading-bot --image ghcr.io/achronon/trading-bot:v2@sha256:<digest>`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDeploy,
}

func init() {
	rootCmd.AddCommand(deployCmd)
	deployCmd.Flags().StringVar(&deployImage, "image", "", "OCI image reference to deploy")
}

func runDeploy(cmd *cobra.Command, args []string) error {
	image := strings.TrimSpace(deployImage)
	if image == "" {
		return fmt.Errorf("--image is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' or set CVPS_API_TOKEN")
	}

	client, err := api.NewClientFromConfig(cfg, api.WithTimeout(deployHTTPTimeout))
	if err != nil {
		return err
	}
	ctx := context.Background()

	sandboxID, err := resolveLifecycleSandboxID(ctx, client, args)
	if err != nil {
		return err
	}

	fmt.Printf("Deploying %s to sandbox %s...\n", image, sandboxID)
	sandbox, err := client.DeploySandbox(ctx, sandboxID, &api.DeploySandboxRequest{Image: image})
	if err != nil {
		if api.IsNotFound(err) {
			return fmt.Errorf("sandbox not found: %s", sandboxID)
		}
		return fmt.Errorf("failed to deploy image to sandbox: %w", err)
	}

	fmt.Printf("Sandbox %s is %s. Use 'cvps status --watch' to follow it.\n", sandbox.ID, sandbox.Status)
	return nil
}
