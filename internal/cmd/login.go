package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	loginAPIKey      string
	loginWithToken   bool
	loginEmail       string
	loginRequestOnly bool
	loginWithCode    bool
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with ClaudeVPS",
	Long: `Authenticate with the ClaudeVPS API.

By default, opens a browser for OAuth authentication.

BOOTSTRAP WITHOUT A BROWSER OR EXISTING TOKEN (agents): use --email to
sign in with a single-use code sent to the account's registered
address. One-shot (sends the code, then reads it from stdin):

  cvps login --email agent@example.com

Or split into two steps so an agent can read its own mailbox in
between (the code is read from STDIN, never from argv):

  cvps login --email agent@example.com --request-only
  printf '%s' "$CODE" | cvps login --email agent@example.com --with-code

The resulting session lasts 24h; mint a durable scoped token from it
with 'cvps token create'.

For headless/agent use with an EXISTING token, pass it on stdin with
--with-token so it never appears on the command line:

  op read "op://vault/cvps/token" | cvps login --with-token

Alternatively skip 'login' entirely: set CVPS_API_TOKEN in the
environment, or set token_command in ~/.cvps/config.yaml (e.g.
"op read op://vault/cvps/token") to resolve the token on demand without
storing it on disk.

The --api-key flag remains for back-compat, but prefer --with-token or
CVPS_API_TOKEN: argv values can leak into shell history and process
listings.`,
	RunE: runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
	loginCmd.Flags().StringVar(&loginAPIKey, "api-key", "", "authenticate with API key (prefer --with-token: argv can leak)")
	loginCmd.Flags().BoolVar(&loginWithToken, "with-token", false, "read the token from stdin and persist it after validation")
	loginCmd.Flags().StringVar(&loginEmail, "email", "", "sign in with a single-use code emailed to this address (agent bootstrap)")
	loginCmd.Flags().BoolVar(&loginRequestOnly, "request-only", false, "with --email: send the code and exit (verify later with --with-code)")
	loginCmd.Flags().BoolVar(&loginWithCode, "with-code", false, "with --email: skip sending, read the code from stdin and verify")
}

func runLogin(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Email-OTP bootstrap (HLM-413/HLM-415): the only login mode that
	// needs neither a browser nor a pre-existing token.
	if loginEmail != "" {
		if loginWithToken || loginAPIKey != "" {
			return fmt.Errorf("--email cannot be combined with --with-token or --api-key")
		}
		if loginRequestOnly && loginWithCode {
			return fmt.Errorf("--request-only and --with-code are mutually exclusive")
		}
		return loginWithEmailOtp(cfg)
	}
	if loginRequestOnly || loginWithCode {
		return fmt.Errorf("--request-only and --with-code require --email <address>")
	}

	// Token from stdin (headless; mirrors 'gh auth login --with-token').
	if loginWithToken {
		// Reading a TTY would block waiting for typed input - the exact
		// hang --no-interactive exists to prevent.
		if noInteractive && term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("--with-token under --no-interactive requires the token on piped stdin (e.g. 'op read op://vault/cvps/token | cvps login --with-token --no-interactive')")
		}
		token, err := readTokenFromStdin(os.Stdin)
		if err != nil {
			return err
		}
		return loginWithCredential(cfg, token, false)
	}

	// API key authentication: --api-key explicitly means X-API-Key auth
	// for back-compat, regardless of key prefix.
	if loginAPIKey != "" {
		return loginWithCredential(cfg, loginAPIKey, true)
	}

	if noInteractive {
		return fmt.Errorf("login requires interaction; under --no-interactive use --with-token (token on stdin), --email <address> (code on stdin), or skip login and set CVPS_API_TOKEN / token_command")
	}

	// Interactive API key entry if --api-key flag is empty but user wants API key auth
	fmt.Print("Choose authentication method:\n")
	fmt.Print("  1. Browser (OAuth) [default]\n")
	fmt.Print("  2. API Key\n")
	fmt.Print("Enter choice (1/2): ")

	reader := bufio.NewReader(os.Stdin)
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	if choice == "2" {
		fmt.Print("Enter API key: ")
		apiKey, _ := reader.ReadString('\n')
		apiKey = strings.TrimSpace(apiKey)
		return loginWithCredential(cfg, apiKey, true)
	}

	return loginWithOAuth(cfg)
}

// loginWithEmailOtp implements the agent-completable bootstrap login
// (HLM-413): request a single-use code emailed to the account's
// registered address, then exchange it for a session token. The code is
// always read from stdin (TTY prompt for humans, piped line for
// agents) — never from argv.
func loginWithEmailOtp(cfg *config.Config) error {
	client := api.NewClient(cfg.APIBaseURL, "")
	ctx := context.Background()

	// Fail fast BEFORE requesting a code: the request consumes the
	// per-account resend cooldown/quota and sends a real email, so a
	// combined flow that can never read the code (--no-interactive with
	// a TTY stdin) must not trigger it. --request-only never reads the
	// code, so it is exempt.
	stdinIsTTY := term.IsTerminal(int(os.Stdin.Fd()))
	willReadCode := !loginRequestOnly
	if willReadCode && noInteractive && stdinIsTTY {
		return fmt.Errorf("reading the code under --no-interactive requires piped stdin; use --request-only first, then 'printf '%%s' \"$CODE\" | cvps login --email %s --with-code'", loginEmail)
	}

	if !loginWithCode {
		ack, err := client.RequestEmailOtp(ctx, loginEmail)
		if err != nil {
			return fmt.Errorf("failed to request sign-in code: %w", err)
		}
		fmt.Fprintln(os.Stderr, ack.Message)
		if loginRequestOnly {
			fmt.Fprintf(os.Stderr, "Verify with: printf '%%s' \"$CODE\" | cvps login --email %s --with-code\n", loginEmail)
			return nil
		}
	}

	if stdinIsTTY {
		fmt.Fprint(os.Stderr, "Enter code: ")
	}
	code, err := readOtpCodeFromStdin(os.Stdin)
	if err != nil {
		return err
	}

	token, err := client.VerifyEmailOtp(ctx, loginEmail, code)
	if err != nil {
		return fmt.Errorf("sign-in code verification failed: %w", err)
	}

	cfg.AccessToken = token.AccessToken
	cfg.APIKey = ""
	clearTokenCommandOnLogin(cfg)
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	// Fetch user info (best-effort, mirrors the OAuth flow)
	authed := api.NewClientWithToken(cfg.APIBaseURL, token.AccessToken)
	user, err := authed.GetCurrentUser(ctx)
	if err != nil {
		fmt.Println("✓ Logged in successfully")
		return nil
	}

	fmt.Printf("✓ Logged in as %s (%s)\n", user.Name, user.Email)
	fmt.Fprintln(os.Stderr, "Session lasts 24h - mint a durable scoped token with 'cvps token create --name <n> --scope <s> --expires-in 30d'.")
	return nil
}

// readOtpCodeFromStdin reads a single sign-in code from stdin (first
// line, whitespace-trimmed) so the code never appears on argv.
func readOtpCodeFromStdin(stdin io.Reader) (string, error) {
	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read code from stdin: %w", err)
	}
	code := strings.TrimSpace(line)
	if code == "" {
		return "", fmt.Errorf("no code provided on stdin (pipe it in, e.g. 'printf '%%s' \"$CODE\" | cvps login --email <address> --with-code')")
	}
	return code, nil
}

// readTokenFromStdin reads a single token from stdin (first line,
// whitespace-trimmed) so token material never appears on argv.
func readTokenFromStdin(stdin io.Reader) (string, error) {
	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read token from stdin: %w", err)
	}
	token := strings.TrimSpace(line)
	if token == "" {
		return "", fmt.Errorf("no token provided on stdin (pipe it in, e.g. 'op read op://vault/cvps/token | cvps login --with-token')")
	}
	return token, nil
}

func loginWithCredential(cfg *config.Config, token string, forceAPIKey bool) error {
	if token == "" {
		return fmt.Errorf("empty token")
	}

	// cvps_-prefixed tokens are API keys (X-API-Key); anything else is
	// sent as a bearer token. The backend accepts cvps_ keys via either
	// header, so both paths validate against /users/me. forceAPIKey
	// preserves the legacy --api-key contract for non-prefixed keys.
	isAPIKey := forceAPIKey || strings.HasPrefix(token, "cvps_")

	var client *api.Client
	if isAPIKey {
		client = api.NewClient(cfg.APIBaseURL, token)
	} else {
		client = api.NewClientWithToken(cfg.APIBaseURL, token)
	}

	user, err := client.GetCurrentUser(context.Background())
	if err != nil {
		return fmt.Errorf("invalid token: %w", err)
	}

	// Clear the counterpart credential: ResolveCredential prefers
	// access_token over api_key, so a stale one would keep winning even
	// though this login just validated and reported success.
	if isAPIKey {
		cfg.APIKey = token
		cfg.AccessToken = ""
	} else {
		cfg.AccessToken = token
		cfg.APIKey = ""
	}
	clearTokenCommandOnLogin(cfg)
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Logged in as %s (%s)\n", user.Name, user.Email)
	return nil
}

// clearTokenCommandOnLogin removes a persisted token_command when the user
// explicitly logs in with a new credential: token_command outranks stored
// tokens at resolution time, so leaving it in place would make the login
// appear to succeed while the old command stays the effective credential.
func clearTokenCommandOnLogin(cfg *config.Config) {
	if cfg.TokenCommand == "" {
		return
	}
	cfg.TokenCommand = ""
	fmt.Fprintln(os.Stderr, "Note: removed the configured token_command so the new login takes effect (it would otherwise keep precedence). Re-set it with 'cvps config set token_command ...' if that was unintended.")
}

func loginWithOAuth(cfg *config.Config) error {
	client := api.NewClient(cfg.APIBaseURL, "")

	// Initiate device authorization flow
	deviceAuth, err := client.InitiateDeviceAuth(context.Background())
	if err != nil {
		return fmt.Errorf("failed to initiate login: %w", err)
	}

	fmt.Printf("\n")
	fmt.Printf("To authenticate, visit:\n")
	fmt.Printf("  %s\n\n", deviceAuth.VerificationURI)
	fmt.Printf("And enter code: %s\n\n", deviceAuth.UserCode)

	// Try to open browser automatically
	if err := browser.OpenURL(deviceAuth.VerificationURIComplete); err != nil {
		fmt.Println("(Could not open browser automatically)")
	}

	fmt.Println("Waiting for authentication...")

	// Poll for completion
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(deviceAuth.ExpiresIn)*time.Second)
	defer cancel()

	token, err := client.PollDeviceAuth(ctx, deviceAuth.DeviceCode, time.Duration(deviceAuth.Interval)*time.Second)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	cfg.AccessToken = token.AccessToken
	cfg.APIKey = ""
	clearTokenCommandOnLogin(cfg)
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	// Fetch user info
	client = api.NewClientWithToken(cfg.APIBaseURL, token.AccessToken)
	user, err := client.GetCurrentUser(context.Background())
	if err != nil {
		fmt.Println("✓ Logged in successfully")
		return nil
	}

	fmt.Printf("✓ Logged in as %s (%s)\n", user.Name, user.Email)
	return nil
}
