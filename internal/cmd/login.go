package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

var (
	loginAPIKey string
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with ClaudeVPS",
	Long: `Authenticate with the ClaudeVPS API.

By default, opens a browser for OAuth authentication.
Use --api-key to authenticate with an API key instead.`,
	RunE: runLogin,
}

func init() {
	rootCmd.AddCommand(loginCmd)
	loginCmd.Flags().StringVar(&loginAPIKey, "api-key", "", "authenticate with API key")
}

func runLogin(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// API key authentication
	if loginAPIKey != "" {
		return loginWithAPIKey(cfg, loginAPIKey)
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
		return loginWithAPIKey(cfg, apiKey)
	}

	return loginWithOAuth(cfg)
}

func loginWithAPIKey(cfg *config.Config, apiKey string) error {
	client := api.NewClient(cfg.APIBaseURL, apiKey)

	// Validate the API key
	user, err := client.GetCurrentUser(context.Background())
	if err != nil {
		return fmt.Errorf("invalid API key: %w", err)
	}

	cfg.APIKey = apiKey
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
