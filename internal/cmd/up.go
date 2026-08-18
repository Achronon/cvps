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
	upCmd.Flags().StringArrayVar(&upSecrets, "secret", nil, "attach an existing tenant secret by key (repeatable; resolved server-side)")
	upCmd.Flags().StringArrayVar(&upEnv, "env", nil, "set a non-secret env override as KEY=VALUE (repeatable; keys must be allowlisted by the runtime profile)")
}

func runUp(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if !cfg.IsAuthenticated() {
		return fmt.Errorf("not logged in. Run 'cvps login' or set CVPS_API_TOKEN")
	}

	if upDedicatedIP && !upAcceptAup {
		return fmt.Errorf("--dedicated-ip requires --accept-aup\n\n" +
			"Dedicated IPs are governed by the Acceptable Use Policy for dedicated\n" +
			"IPs and sandbox-originated email (no unsolicited bulk mail, no raw SMTP\n" +
			"egress, no abuse tooling). Review it in the dashboard's Create Sandbox\n" +
			"dialog, then re-run with --accept-aup to confirm acceptance")
	}

	client, err := api.NewClientFromConfig(cfg)
	if err != nil {
		return err
	}
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

	// Validate --secret keys locally, then let the backend resolve them under
	// secrets:attach. Enumerating /secrets here would require secrets:read and
	// would defeat the least-privilege Cortex control profile.
	secretKeys, err := validateSecretKeys(upSecrets)
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
		SecretKeys:     secretKeys,
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
		recoveryHint := createFailureRecoveryHint(err, len(secretKeys) > 0)
		if hint := createErrorHint(err); hint != "" {
			if recoveryHint != "" {
				return fmt.Errorf("failed to create sandbox with requested secret keys: %w\n\n%s\n\n%s", err, hint, recoveryHint)
			}
			return fmt.Errorf("failed to create sandbox: %w\n\n%s", err, hint)
		}
		if recoveryHint != "" {
			return fmt.Errorf("failed to create sandbox with requested secret keys: %w\n\n%s", err, recoveryHint)
		}
		return fmt.Errorf("failed to create sandbox: %w", err)
	}

	fmt.Printf("Sandbox created: %s\n", sandbox.ID)
	if len(secretKeys) > 0 {
		if err := verifySecretAttachments(ctx, client, sandbox.ID, secretKeys); err != nil {
			if contextErr := saveLocalContext(sandbox.ID, sandbox.Name); contextErr != nil {
				return fmt.Errorf("%w; failed to save sandbox context: %v", err, contextErr)
			}
			return fmt.Errorf("%w; context saved — use 'cvps status %s' to inspect it, then 'cvps secret attach <KEY> --sandbox %s' or 'cvps down %s' as appropriate", err, sandbox.ID, sandbox.ID, sandbox.ID)
		}
	}
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
		status, err := pollSandboxForUp(ctx, client, sandbox)
		if err != nil {
			s.Stop()
			return fmt.Errorf("failed to get status: %w", err)
		}

		// The backend reports Prisma enum casing (RUNNING/ERROR);
		// normalize so we never spin past a ready sandbox.
		switch strings.ToLower(strings.TrimSpace(status.Status)) {
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

func pollSandboxForUp(ctx context.Context, client *api.Client, created *api.Sandbox) (*api.Sandbox, error) {
	status, err := client.GetSandboxStatus(ctx, created.ID)
	if err != nil {
		return nil, err
	}

	// The /status endpoint can lag the full sandbox resource. Fetch the full
	// record for RUNNING details and whenever /status is still non-terminal,
	// allowing DB status to break a stale PROVISIONING loop.
	normalized := strings.ToLower(strings.TrimSpace(status.Status))
	if normalized == "running" || normalized == "provisioning" || normalized == "starting" {
		full, fetchErr := client.GetSandbox(ctx, created.ID)
		if fetchErr == nil && strings.TrimSpace(full.Status) != "" {
			fullStatus := strings.ToLower(strings.TrimSpace(full.Status))
			if normalized == "running" || fullStatus != normalized {
				return full, nil
			}
		}
		if normalized == "running" {
			created.Status = status.Status
			return created, nil
		}
	}

	return status, nil
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

// validateSecretKeys validates repeatable --secret <KEY> flags without
// enumerating tenant secrets. The backend performs the tenant-scoped lookup
// during create, before any sandbox is persisted.
func validateSecretKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	for _, key := range keys {
		if !secretKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid --secret key %q: must be a valid environment variable name (uppercase letters, numbers, underscores)", key)
		}
	}

	var validated []string
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		validated = append(validated, key)
	}
	return validated, nil
}

// verifySecretAttachments confirms that the backend understood the key-based
// create contract and attached every requested key. The detail response only
// contains secret metadata (keys/names), never secret values, and this is a
// scoped sandbox read rather than a tenant-wide secret enumeration.
func verifySecretAttachments(ctx context.Context, client *api.Client, sandboxID string, requestedKeys []string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		sandbox, err := client.GetSandbox(ctx, sandboxID)
		if err != nil {
			lastErr = fmt.Errorf("sandbox %s was created, but secret attachment could not be verified: %w", sandboxID, err)
		} else {
			attached := make(map[string]struct{}, len(sandbox.Secrets))
			for _, secret := range sandbox.Secrets {
				attached[secret.Key] = struct{}{}
			}

			missing := make([]string, 0, len(requestedKeys))
			for _, key := range requestedKeys {
				if _, ok := attached[key]; !ok {
					missing = append(missing, key)
				}
			}
			if len(missing) == 0 {
				return nil
			}
			lastErr = fmt.Errorf(
				"sandbox %s was created, but requested secret keys were not attached: %s",
				sandboxID,
				strings.Join(missing, ", "),
			)
		}
		if attempt < 2 {
			time.Sleep(250 * time.Millisecond)
		}
	}
	return fmt.Errorf("%w; check the backend/CLI versions before using this sandbox", lastErr)
}

func createFailureRecoveryHint(err error, requestedSecrets bool) string {
	if !requestedSecrets {
		return ""
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode > 0 && apiErr.StatusCode < 500 {
		return ""
	}
	return "If the backend created a sandbox before reporting the error, check 'cvps status --all' for a stray sandbox before retrying."
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
	case "secret_key_not_found":
		return "One or more requested secret keys do not exist. Create each missing secret first with\n  cvps secret create <KEY>\nthen retry the sandbox create."
	case "capacity_exhausted":
		return "The CVPS cluster does not currently have enough free capacity for this\nsandbox shape. Retry with a smaller --cpu, --memory, or --storage value, or\nask an operator to inspect admin/billing/capacity/sandbox before changing\ninfra capacity."
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
