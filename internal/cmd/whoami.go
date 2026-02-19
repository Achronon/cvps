package cmd

import (
	"context"
	"fmt"

	"github.com/achronon/cvps/internal/api"
	"github.com/achronon/cvps/internal/config"
	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Show current user",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if !cfg.IsAuthenticated() {
			return fmt.Errorf("not logged in. Run 'cvps login' first")
		}

		client := api.NewClientFromConfig(cfg)
		user, err := client.GetCurrentUser(context.Background())
		if err != nil {
			return fmt.Errorf("failed to get user info: %w", err)
		}

		fmt.Printf("Logged in as: %s (%s)\n", user.Name, user.Email)
		fmt.Printf("User ID: %s\n", user.ID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(whoamiCmd)
}
