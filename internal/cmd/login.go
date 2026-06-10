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
)

var (
	loginAPIKey    string
	loginWithToken bool
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with ClaudeVPS",
	Long: `Authenticate with the ClaudeVPS API.

By default, opens a browser for OAuth authentication.

For headless/agent use, pass the token on stdin with --with-token so it
never appears on the command line:

  cvps token create --name agent-x | cvps login --with-token

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
}

func runLogin(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Token from stdin (headless; mirrors 'gh auth login --with-token').
	if loginWithToken {
		token, err := readTokenFromStdin(os.Stdin)
		if err != nil {
			return err
		}
		return loginWithCredential(cfg, token)
	}

	// API key authentication
	if loginAPIKey != "" {
		return loginWithCredential(cfg, loginAPIKey)
	}

	if noInteractive {
		return fmt.Errorf("login requires interaction; under --no-interactive use --with-token (token on stdin), or skip login and set CVPS_API_TOKEN / token_command")
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
		return loginWithCredential(cfg, apiKey)
	}

	return loginWithOAuth(cfg)
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
		return "", fmt.Errorf("no token provided on stdin (pipe it in, e.g. 'cvps token create --name agent-x | cvps login --with-token')")
	}
	return token, nil
}

func loginWithCredential(cfg *config.Config, token string) error {
	if token == "" {
		return fmt.Errorf("empty token")
	}

	// cvps_-prefixed tokens are API keys (X-API-Key); anything else is
	// sent as a bearer token. The backend accepts cvps_ keys via either
	// header, so both paths validate against /users/me.
	isAPIKey := strings.HasPrefix(token, "cvps_")

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

	if isAPIKey {
		cfg.APIKey = token
	} else {
		cfg.AccessToken = token
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("✓ Logged in as %s (%s)\n", user.Name, user.Email)
	return nil
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
