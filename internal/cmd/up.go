package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	upName        string
	upCPU         int
	upMemory      int
	upStorage     int
	upDetach      bool
	upDedicatedIP bool
	upAcceptAup   bool
	upProfile     string
	upSecrets     []string
	upEnv         []string
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
  cvps up --detach

  # Create with a dedicated egress IP (requires accepting the AUP)
  cvps up --dedicated-ip --accept-aup

  # Provision from a specific runtime profile (e.g. an agent service)
  cvps up --profile cortex

  # Attach existing tenant secrets and set env overrides at create time
  cvps up --profile cortex --name cortex-brain --secret TELEGRAM_BOT_TOKEN --env CAPTURE_MODE=auto`,
	RunE: runUp,
}

func init() {
	rootCmd.AddCommand(upCmd)

	upCmd.Flags().StringVarP(&upName, "name", "n", "", "sandbox name")
	upCmd.Flags().IntVar(&upCPU, "cpu", 0, "CPU cores (default from config)")
	upCmd.Flags().IntVar(&upMemory, "memory", 0, "memory in GB (default from config)")
	upCmd.Flags().IntVar(&upStorage, "storage", 0, "storage in GB (default from config)")
	upCmd.Flags().BoolVarP(&upDetach, "detach", "d", false, "return immediately without waiting")
	upCmd.Flags().BoolVar(&upDedicatedIP, "dedicated-ip", false, "request a dedicated egress IP (requires --accept-aup)")
	upCmd.Flags().BoolVar(&upAcceptAup, "accept-aup", false, "accept the dedicated-IP / outbound-email Acceptable Use Policy")
	upCmd.Flags().StringVar(&upProfile, "profile", "", "runtime profile slug (e.g. cortex); default is the server-side default profile")
	upCmd.Flags().StringArrayVar(&upSecrets, "secret", nil, "attach an existing tenant secret by key (repeatable; see 'cvps secret list')")
	upCmd.Flags().StringArrayVar(&upEnv, "env", nil, "set a non-secret env override as KEY=VALUE (repeatable; keys must be allowlisted by the runtime profile)")
}

func runUp(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' first")
	}

	if upDedicatedIP && !upAcceptAup {
		return fmt.Errorf("--dedicated-ip requires --accept-aup\n\n" +
			"Dedicated IPs are governed by the Acceptable Use Policy for dedicated\n" +
			"IPs and sandbox-originated email (no unsolicited bulk mail, no raw SMTP\n" +
			"egress, no abuse tooling). Review it in the dashboard's Create Sandbox\n" +
			"dialog, then re-run with --accept-aup to confirm acceptance")
	}

	client := api.NewClientFromConfig(cfg)
	ctx := context.Background()

	// Resolve --profile slug to the full profile up-front: we need its id for the
	// create request and its mode to pick the right post-create UX.
	var profile *api.RuntimeProfile
	if upProfile != "" {
		var err error
		profile, err = client.GetRuntimeProfileBySlug(ctx, upProfile)
		if err != nil {
			if api.IsNotFound(err) {
				return fmt.Errorf("runtime profile not found: %s", upProfile)
			}
			return fmt.Errorf("failed to resolve runtime profile %q: %w", upProfile, err)
		}
	}

	// Parse --env overrides before any creation side effects.
	envOverrides, err := parseEnvOverrides(upEnv)
	if err != nil {
		return err
	}

	// Resolve --secret keys to TenantSecret ids up-front: attach is
	// create-time only, so an unknown key must fail fast with no sandbox
	// half-created.
	secretIDs, err := resolveSecretKeys(ctx, client, upSecrets)
	if err != nil {
		return err
	}

	// Build create request
	req := &api.CreateSandboxRequest{
		Name:           upName,
		CPUCores:       upCPU,
		MemoryGB:       upMemory,
		StorageGB:      upStorage,
		UseDedicatedIp: upDedicatedIP,
		AcceptedAup:    upAcceptAup,
		SecretIDs:      secretIDs,
		EnvOverrides:   envOverrides,
	}
	if profile != nil {
		req.RuntimeProfileID = profile.ID
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

	sandbox, err := client.CreateSandbox(ctx, req)
	if err != nil {
		if hint := createErrorHint(err); hint != "" {
			return fmt.Errorf("failed to create sandbox: %w\n\n%s", err, hint)
		}
		return fmt.Errorf("failed to create sandbox: %w", err)
	}

	fmt.Printf("Sandbox created: %s\n", sandbox.ID)
	if ip := dedicatedIPOf(sandbox); ip != "" {
		fmt.Printf("Dedicated IP: %s\n", ip)
	}

	// Service profiles gate readiness on in-sandbox model auth (/readyz is 503 until
	// e.g. codex auth.json exists), so polling for RUNNING would sit red for no
	// reason. Save context first — the id-less next steps depend on it.
	if isServiceProfile(profile, sandbox) {
		saveLocalContext(sandbox.ID, sandbox.Name)
		printServiceBootstrapSteps(sandbox.ID)
		return nil
	}

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

		// The backend reports Prisma enum casing (RUNNING/ERROR);
		// normalize so we never spin past a ready sandbox.
		switch strings.ToLower(strings.TrimSpace(status.Status)) {
		case "running":
			s.Stop()
			// The /status endpoint only carries {status,details}; fetch
			// the full sandbox so resources/SSH/dedicated IP are real.
			full, fetchErr := client.GetSandbox(ctx, sandbox.ID)
			if fetchErr != nil {
				full = sandbox
			}
			printSandboxReady(full)
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

// parseEnvOverrides turns repeatable --env KEY=VALUE flags into the
// envOverrides map. Keys are validated client-side against the env-var name
// shape; the runtime profile's tenantEnvKeys allowlist is enforced by the
// backend (its 400 lists the allowed keys).
func parseEnvOverrides(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	overrides := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			return nil, fmt.Errorf("invalid --env %q: expected KEY=VALUE", pair)
		}
		if !secretKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid --env key %q: must be a valid environment variable name (uppercase letters, numbers, underscores)", key)
		}
		if value == "" {
			return nil, fmt.Errorf("invalid --env %q: value must not be empty", pair)
		}
		if _, dup := overrides[key]; dup {
			return nil, fmt.Errorf("duplicate --env key %q", key)
		}
		overrides[key] = value
	}
	return overrides, nil
}

// resolveSecretKeys resolves repeatable --secret <KEY> flags to TenantSecret
// ids, failing fast (before sandbox creation) on any unknown key.
func resolveSecretKeys(ctx context.Context, client *api.Client, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	for _, key := range keys {
		if !secretKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid --secret key %q: must be a valid environment variable name (uppercase letters, numbers, underscores)", key)
		}
	}

	// One list call resolves all keys (and dedupes repeats).
	secrets, err := client.ListAllSecrets(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	byKey := make(map[string]string, len(secrets))
	for _, s := range secrets {
		byKey[s.Key] = s.ID
	}

	var ids []string
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		id, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("no secret found with key %s.\n\nCreate it first:\n  cvps secret create %s\nor list available keys with 'cvps secret list'", key, key)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// createErrorHint maps backend error codes from POST /sandboxes to
// actionable guidance. Empty string when no hint applies.
func createErrorHint(err error) string {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		return ""
	}
	switch apiErr.ErrCode() {
	case "subscription_required":
		return "An active subscription is required. Subscribe in the dashboard under\nSettings → Billing, then retry."
	case "aup_acceptance_required":
		return "This request needs the Acceptable Use Policy accepted. Re-run with\n--accept-aup (see the dashboard's Create Sandbox dialog for the policy text)."
	case "dedicated_ip_not_entitled":
		return "Your subscription has no dedicated-IP entitlement right now — the plan\nincludes none or the subscription is not active. Check Settings → Billing\n(plan status, or the dedicated-IP add-on)."
	case "dedicated_ip_capacity":
		return "Every dedicated IP your plan is entitled to is already attached to a\nsandbox, or the shared pool is exhausted. Destroy a sandbox that holds an\nIP, add another IP under Settings → Billing, or create the sandbox\nwithout --dedicated-ip."
	case "phone_verification_required":
		return "A verified phone number is required for dedicated-IP sandboxes. Verify\nyour phone in the dashboard under Settings, then retry."
	case "mfa_required":
		return "Multi-factor authentication is required for dedicated-IP sandboxes.\nEnable MFA in the dashboard under Settings, then retry."
	case "unsupported_runtime_for_create":
		return "This runtime profile isn't enabled for self-serve creation. Gated\nprofiles (e.g. agent services like cortex) need the operator to enable\nthem (SELF_SERVE_EXTRA_RUNTIME_SLUGS on the backend)."
	}
	return ""
}

// isServiceProfile reports whether the sandbox runs a SERVICE-mode profile, using
// the resolved profile when --profile was given and the create response otherwise.
func isServiceProfile(profile *api.RuntimeProfile, sandbox *api.Sandbox) bool {
	if sandbox != nil && sandbox.ServiceMode {
		return true
	}
	if profile != nil && strings.EqualFold(profile.Mode, "SERVICE") {
		return true
	}
	return false
}

func printServiceBootstrapSteps(sandboxID string) {
	fmt.Println("\nService sandbox is provisioning. It will stay NOT READY until its model")
	fmt.Println("auth is set up inside the sandbox (readiness gates on it).")
	fmt.Println("\nBootstrap:")
	fmt.Printf("  cvps connect %s        - open a terminal (works before ready)\n", sandboxID)
	fmt.Println("    then inside: codex login --device-auth")
	fmt.Println("\nThen:")
	fmt.Println("  cvps status            - watch it turn RUNNING once auth lands")
	fmt.Println("  cvps logs              - recent service logs")
	fmt.Println("  cvps restart           - bounce the workload (when RUNNING)")
}

func dedicatedIPOf(sandbox *api.Sandbox) string {
	if sandbox == nil || sandbox.DedicatedIp == nil {
		return ""
	}
	return sandbox.DedicatedIp.IPAddress
}

func printSandboxReady(sandbox *api.Sandbox) {
	fmt.Println("\n✓ Sandbox is ready!")

	fmt.Println("Resources:")
	fmt.Printf("  CPU:     %d cores\n", sandbox.CPUCores)
	fmt.Printf("  Memory:  %d GB\n", sandbox.MemoryGB)
	fmt.Printf("  Storage: %d GB\n", sandbox.StorageGB)

	if ip := dedicatedIPOf(sandbox); ip != "" {
		fmt.Printf("\nDedicated IP: %s\n", ip)
	}

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
