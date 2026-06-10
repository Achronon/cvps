package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	secretCreateName        string
	secretCreateDescription string
	secretCreateCategory    string
	secretCreateValueFile   string
	secretRmForce           bool
)

// secretKeyPattern mirrors the backend CreateSecretDto key validation so we
// can fail fast before any network call.
var secretKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// secretCategories mirrors the backend SecretCategory enum.
var secretCategories = []string{"API_KEY", "ACCESS_TOKEN", "DATABASE_CREDENTIAL", "OTHER"}

var secretCmd = &cobra.Command{
	Use:   "secret",
	Short: "Manage tenant secrets",
	Long: `Manage tenant secrets (encrypted environment variables).

Secrets are attached to sandboxes at create time and injected as environment
variables. Values are encrypted at rest and never returned by the API after
creation.`,
}

var secretCreateCmd = &cobra.Command{
	Use:   "create <KEY>",
	Short: "Create a secret",
	Long: `Create a tenant secret.

The secret value is read from --value-file, piped stdin, or a hidden
interactive prompt - never from a command-line argument, so it cannot leak
into shell history or process listings. A single trailing newline is trimmed
from file/stdin input.`,
	Example: `  # From a pipe (e.g. a secrets manager)
  op read "op://vault/telegram/token" | cvps secret create TELEGRAM_BOT_TOKEN

  # From a file (e.g. BYO codex auth.json)
  cvps secret create CODEX_AUTH --value-file ~/.codex/auth.json

  # Interactively (hidden prompt)
  cvps secret create ANTHROPIC_API_KEY`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretCreate,
}

var secretListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List secrets",
	Long:    `List the tenant's secrets. Secret values are never shown.`,
	Args:    cobra.NoArgs,
	RunE:    runSecretList,
}

var secretRmCmd = &cobra.Command{
	Use:     "rm <KEY-or-ID>",
	Aliases: []string{"delete", "remove"},
	Short:   "Delete a secret",
	Long: `Permanently delete a secret by key (e.g. TELEGRAM_BOT_TOKEN) or by id.

Deleting a secret also detaches it from any sandboxes that use it.`,
	Args: cobra.ExactArgs(1),
	RunE: runSecretRm,
}

func init() {
	rootCmd.AddCommand(secretCmd)
	secretCmd.AddCommand(secretCreateCmd)
	secretCmd.AddCommand(secretListCmd)
	secretCmd.AddCommand(secretRmCmd)

	secretCreateCmd.Flags().StringVar(&secretCreateName, "name", "", "human-readable name (defaults to the key)")
	secretCreateCmd.Flags().StringVar(&secretCreateDescription, "description", "", "optional description")
	secretCreateCmd.Flags().StringVar(&secretCreateCategory, "category", "", fmt.Sprintf("secret category (%s)", strings.Join(secretCategories, ", ")))
	secretCreateCmd.Flags().StringVar(&secretCreateValueFile, "value-file", "", "read the secret value from this file ('-' for stdin)")

	secretRmCmd.Flags().BoolVarP(&secretRmForce, "force", "f", false, "skip confirmation prompt")
}

func newAuthenticatedClient() (*api.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if !cfg.IsAuthenticated() {
		return nil, fmt.Errorf("not logged in. Run 'cvps login' first")
	}
	return api.NewClientFromConfig(cfg), nil
}

func runSecretCreate(cmd *cobra.Command, args []string) error {
	key := args[0]
	if !secretKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid secret key %q: must be a valid environment variable name (uppercase letters, numbers, underscores; e.g. TELEGRAM_BOT_TOKEN)", key)
	}

	category := strings.ToUpper(strings.TrimSpace(secretCreateCategory))
	if category != "" && !isValidSecretCategory(category) {
		return fmt.Errorf("invalid --category %q: must be one of %s", secretCreateCategory, strings.Join(secretCategories, ", "))
	}

	value, err := readSecretValue(secretCreateValueFile, os.Stdin, key)
	if err != nil {
		return err
	}
	if value == "" {
		return fmt.Errorf("secret value is empty")
	}

	name := secretCreateName
	if name == "" {
		name = key
	}

	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}

	secret, err := client.CreateSecret(context.Background(), &api.CreateSecretRequest{
		Name:        name,
		Key:         key,
		Value:       value,
		Description: secretCreateDescription,
		Category:    category,
	})
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 409 {
			return fmt.Errorf("a secret with key %s already exists.\n\nRemove it first with 'cvps secret rm %s' (this detaches it from any\nsandboxes), then re-create it", key, key)
		}
		return fmt.Errorf("failed to create secret: %w", err)
	}

	fmt.Printf("✓ Secret created: %s (%s)\n", secret.Key, secret.ID)
	return nil
}

// readSecretValue resolves the secret value without ever touching argv.
// Precedence: --value-file ('-' meaning stdin) > piped stdin > hidden
// interactive prompt. A single trailing newline is trimmed from file/stdin
// input (shell pipelines and editors almost always append one).
func readSecretValue(valueFile string, stdin *os.File, key string) (string, error) {
	if valueFile != "" && valueFile != "-" {
		data, err := os.ReadFile(valueFile)
		if err != nil {
			return "", fmt.Errorf("failed to read --value-file: %w", err)
		}
		return trimSingleTrailingNewline(string(data)), nil
	}

	if valueFile == "-" || !term.IsTerminal(int(stdin.Fd())) {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read secret value from stdin: %w", err)
		}
		return trimSingleTrailingNewline(string(data)), nil
	}

	// Interactive: hidden prompt, no echo.
	fmt.Fprintf(os.Stderr, "Enter value for %s (input hidden): ", key)
	value, err := term.ReadPassword(int(stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("failed to read secret value: %w", err)
	}
	return string(value), nil
}

func trimSingleTrailingNewline(s string) string {
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
}

func isValidSecretCategory(category string) bool {
	for _, c := range secretCategories {
		if c == category {
			return true
		}
	}
	return false
}

func runSecretList(cmd *cobra.Command, args []string) error {
	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}

	secrets, err := client.ListAllSecrets(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list secrets: %w", err)
	}

	if len(secrets) == 0 {
		fmt.Println("No secrets found. Create one with 'cvps secret create <KEY>'.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tNAME\tCATEGORY\tID\tCREATED")
	for _, s := range secrets {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Key, s.Name, s.Category, s.ID, formatSecretDate(s.CreatedAt))
	}
	return w.Flush()
}

// formatSecretDate trims an RFC3339 timestamp down to its date part for
// table display; unknown formats pass through untouched.
func formatSecretDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

func runSecretRm(cmd *cobra.Command, args []string) error {
	ref := args[0]

	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	// Resolve key -> secret. Anything that isn't a valid env key shape is
	// treated as an id directly.
	id := ref
	display := ref
	if secretKeyPattern.MatchString(ref) {
		secret, err := client.FindSecretByKey(ctx, ref)
		if err != nil {
			return fmt.Errorf("failed to resolve secret %s: %w", ref, err)
		}
		if secret == nil {
			return fmt.Errorf("no secret found with key %s. Run 'cvps secret list' to see available secrets", ref)
		}
		id = secret.ID
		display = fmt.Sprintf("%s (%s)", secret.Key, secret.ID)
	}

	if !secretRmForce {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("refusing to delete %s without confirmation on non-interactive stdin; re-run with --force", display)
		}
		fmt.Printf("Permanently delete secret %s? This detaches it from any sandboxes. [y/N]: ", display)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.ToLower(strings.TrimSpace(input))
		if input != "y" && input != "yes" {
			return fmt.Errorf("aborted")
		}
	}

	if err := client.DeleteSecret(ctx, id); err != nil {
		if api.IsNotFound(err) {
			return fmt.Errorf("secret %s not found (may already be deleted)", display)
		}
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	fmt.Printf("✓ Secret deleted: %s\n", display)
	return nil
}
