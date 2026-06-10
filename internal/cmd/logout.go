package cmd

import (
	"fmt"

	"github.com/achronon/cvps/internal/config"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out from ClaudeVPS",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		cfg.APIKey = ""
		cfg.AccessToken = ""
		// token_command is a credential source too: leaving it set would
		// keep the CLI silently authenticated after logout.
		hadTokenCommand := cfg.TokenCommand != ""
		cfg.TokenCommand = ""

		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Println("✓ Logged out successfully")
		if hadTokenCommand {
			fmt.Println("Note: the configured token_command was removed as part of logout.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
