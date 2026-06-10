package cmd

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/spf13/cobra"
)

var (
	tokenCreateName      string
	tokenCreateScopes    []string
	tokenCreateExpiresIn string
	tokenCreateExpiresAt string
)

const tokenScopeCaveat = `CAVEAT: scopes are stored by the backend but NOT YET ENFORCED - a minted
token currently carries the holder's full entitlement regardless of
--scope. Treat --scope as advisory metadata until backend scope
enforcement lands (HLM-384). Expiry and revocation ARE enforced at auth
time.`

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Mint and manage API tokens for agents and automation",
	Long: `Mint and manage API tokens (cvps_... keys) for agents and automation.

Tokens authenticate headlessly via the CVPS_API_TOKEN environment
variable or 'cvps login --with-token'.

` + tokenScopeCaveat,
}

var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Mint a new API token (full key printed once, on stdout only)",
	Long: `Mint a new API token.

The full key is printed ONCE, alone on stdout (everything else goes to
stderr), so it pipes cleanly into a secret store or another command. It
cannot be retrieved again afterwards - only its prefix is listed.

` + tokenScopeCaveat,
	Example: `  # Mint a 7-day token for an agent and use it immediately
  CVPS_API_TOKEN=$(cvps token create --name agent-x --expires-in 7d) cvps status

  # Store it in 1Password without it ever hitting disk
  cvps token create --name ci --expires-in 30d | op item edit cvps-ci token[password]=-`,
	Args: cobra.NoArgs,
	RunE: runTokenCreate,
}

var tokenListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List API tokens (never shows key material)",
	Args:    cobra.NoArgs,
	RunE:    runTokenList,
}

var tokenRevokeCmd = &cobra.Command{
	Use:     "revoke <id-or-prefix>",
	Aliases: []string{"rm", "delete"},
	Short:   "Revoke an API token by id or key prefix",
	Long: `Revoke an API token by its id or its key prefix (as shown by
'cvps token list'). A revoked token is rejected at auth time immediately.`,
	Args: cobra.ExactArgs(1),
	RunE: runTokenRevoke,
}

func init() {
	rootCmd.AddCommand(tokenCmd)
	tokenCmd.AddCommand(tokenCreateCmd)
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)

	tokenCreateCmd.Flags().StringVar(&tokenCreateName, "name", "", "name for the token (required)")
	tokenCreateCmd.Flags().StringArrayVar(&tokenCreateScopes, "scope", nil, "scope to grant (repeatable; ADVISORY until backend enforcement lands - see command help)")
	tokenCreateCmd.Flags().StringVar(&tokenCreateExpiresIn, "expires-in", "", "expiry as a duration from now (e.g. 30m, 12h, 7d, 4w)")
	tokenCreateCmd.Flags().StringVar(&tokenCreateExpiresAt, "expires-at", "", "expiry as an absolute RFC 3339 timestamp (e.g. 2026-07-01T00:00:00Z)")
	_ = tokenCreateCmd.MarkFlagRequired("name")
}

// expiresInPattern accepts a positive integer followed by a single unit:
// m(inutes), h(ours), d(ays), w(eeks).
var expiresInPattern = regexp.MustCompile(`^(\d+)([mhdw])$`)

// parseExpiresIn converts a human duration (30m, 12h, 7d, 4w) into an
// absolute RFC 3339 timestamp relative to now.
func parseExpiresIn(value string, now time.Time) (string, error) {
	match := expiresInPattern.FindStringSubmatch(strings.TrimSpace(value))
	if match == nil {
		return "", fmt.Errorf("invalid --expires-in %q: use <N><unit> with unit one of m, h, d, w (e.g. 30m, 12h, 7d, 4w)", value)
	}
	n, err := strconv.Atoi(match[1])
	if err != nil || n <= 0 {
		return "", fmt.Errorf("invalid --expires-in %q: duration must be a positive integer", value)
	}

	var d time.Duration
	switch match[2] {
	case "m":
		d = time.Duration(n) * time.Minute
	case "h":
		d = time.Duration(n) * time.Hour
	case "d":
		d = time.Duration(n) * 24 * time.Hour
	case "w":
		d = time.Duration(n) * 7 * 24 * time.Hour
	}
	return now.Add(d).UTC().Format(time.RFC3339), nil
}

func runTokenCreate(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(tokenCreateName) == "" {
		return fmt.Errorf("--name must not be empty")
	}
	if tokenCreateExpiresIn != "" && tokenCreateExpiresAt != "" {
		return fmt.Errorf("--expires-in and --expires-at are mutually exclusive")
	}

	expiresAt := strings.TrimSpace(tokenCreateExpiresAt)
	if expiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return fmt.Errorf("invalid --expires-at %q: must be RFC 3339 (e.g. 2026-07-01T00:00:00Z)", expiresAt)
		}
		if !parsed.After(time.Now()) {
			return fmt.Errorf("--expires-at %q is in the past", expiresAt)
		}
	}
	if tokenCreateExpiresIn != "" {
		parsed, err := parseExpiresIn(tokenCreateExpiresIn, time.Now())
		if err != nil {
			return err
		}
		expiresAt = parsed
	}

	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}

	key, err := client.CreateAPIKey(context.Background(), &api.CreateAPIKeyRequest{
		Name:      tokenCreateName,
		Scopes:    tokenCreateScopes,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return fmt.Errorf("failed to create token: %w", err)
	}
	if key.Key == "" {
		return fmt.Errorf("backend did not return the key material; the token %s may still have been created - check 'cvps token list'", key.KeyPrefix)
	}

	// The full key goes to stdout ALONE so it pipes cleanly; everything
	// human-facing goes to stderr.
	fmt.Fprintf(os.Stderr, "✓ Token created: %s (%s)\n", key.Name, key.KeyPrefix)
	if key.ExpiresAt != "" {
		fmt.Fprintf(os.Stderr, "  Expires: %s\n", key.ExpiresAt)
	} else {
		fmt.Fprintf(os.Stderr, "  Expires: never\n")
	}
	fmt.Fprintf(os.Stderr, "  Scopes:  %s (advisory until backend enforcement lands)\n", strings.Join(key.Scopes, ", "))
	fmt.Fprintf(os.Stderr, "\nThe key below is shown ONCE and cannot be retrieved again.\n")
	fmt.Println(key.Key)
	return nil
}

func runTokenList(cmd *cobra.Command, args []string) error {
	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}

	keys, err := client.ListAPIKeys(context.Background())
	if err != nil {
		return fmt.Errorf("failed to list tokens: %w", err)
	}

	if len(keys) == 0 {
		fmt.Println("No tokens found. Mint one with 'cvps token create --name <name>'.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PREFIX\tNAME\tSCOPES\tEXPIRES\tLAST USED\tID")
	for _, k := range keys {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			k.KeyPrefix,
			k.Name,
			strings.Join(k.Scopes, ","),
			formatTokenDate(k.ExpiresAt, "never"),
			formatTokenDate(k.LastUsed, "-"),
			k.ID,
		)
	}
	return w.Flush()
}

// formatTokenDate trims an RFC 3339 timestamp to its date part for table
// display, substituting fallback when empty.
func formatTokenDate(ts, fallback string) string {
	if ts == "" {
		return fallback
	}
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

func runTokenRevoke(cmd *cobra.Command, args []string) error {
	ref := strings.TrimSpace(args[0])
	if ref == "" {
		return fmt.Errorf("empty token reference")
	}

	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	// The backend DELETE silently no-ops on unknown ids, so resolve the
	// reference against the live list to give real not-found/ambiguity
	// feedback.
	keys, err := client.ListAPIKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tokens: %w", err)
	}

	target, err := resolveTokenRef(keys, ref)
	if err != nil {
		return err
	}

	if err := client.RevokeAPIKey(ctx, target.ID); err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	fmt.Printf("✓ Token revoked: %s (%s)\n", target.Name, target.KeyPrefix)
	return nil
}

// resolveTokenRef matches ref against token ids (exact) and key prefixes
// (prefix match in either direction, so both a stored 12-char prefix and a
// longer pasted fragment of the key resolve). Ambiguous prefix matches are
// an error.
func resolveTokenRef(keys []api.APIKey, ref string) (*api.APIKey, error) {
	for i := range keys {
		if keys[i].ID == ref {
			return &keys[i], nil
		}
	}

	var matches []*api.APIKey
	for i := range keys {
		if strings.HasPrefix(keys[i].KeyPrefix, ref) || strings.HasPrefix(ref, keys[i].KeyPrefix) {
			matches = append(matches, &keys[i])
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no token found matching %q. Run 'cvps token list' to see ids and prefixes", ref)
	case 1:
		return matches[0], nil
	default:
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = fmt.Sprintf("%s (%s)", m.KeyPrefix, m.ID)
		}
		return nil, fmt.Errorf("token reference %q is ambiguous: matches %s. Use the id instead", ref, strings.Join(names, ", "))
	}
}
