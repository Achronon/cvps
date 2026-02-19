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

		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Println("✓ Logged out successfully")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
