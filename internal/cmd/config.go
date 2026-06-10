package cmd

import (
	"fmt"

	"github.com/achronon/cvps/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View or modify configuration",
	Long:  `View or modify cvps configuration settings.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		// Build a display struct field-by-field instead of copying the
		// Config: credential values never enter it, only constant
		// placeholders (CodeQL go/clear-text-logging tracks taint through
		// whole-struct copies).
		display := struct {
			APIKey       string                 `yaml:"api_key,omitempty"`
			AccessToken  string                 `yaml:"access_token,omitempty"`
			TokenCommand string                 `yaml:"token_command,omitempty"`
			APIBaseURL   string                 `yaml:"api_base_url"`
			Defaults     config.SandboxDefaults `yaml:"defaults"`
			Sync         config.SyncConfig      `yaml:"sync"`
		}{
			APIKey:       redactedPlaceholder(cfg.APIKey != ""),
			AccessToken:  redactedPlaceholder(cfg.AccessToken != ""),
			TokenCommand: cfg.TokenCommand,
			APIBaseURL:   cfg.APIBaseURL,
			Defaults:     cfg.Defaults,
			Sync:         cfg.Sync,
		}

		data, err := yaml.Marshal(display)
		if err != nil {
			return err
		}

		fmt.Println(string(data))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set KEY VALUE",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]

		cfg, err := config.Load()
		if err != nil {
			return err
		}

		switch key {
		case "api_key":
			cfg.APIKey = value
		case "api_base_url":
			cfg.APIBaseURL = value
		case "token_command":
			cfg.TokenCommand = value
		default:
			return fmt.Errorf("unknown config key: %s", key)
		}

		if err := config.Save(cfg); err != nil {
			return err
		}

		fmt.Printf("Set %s successfully\n", key)
		return nil
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show config file path",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.ConfigPath()
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configPathCmd)
}

// redactedPlaceholder renders presence of a credential without ever
// touching the credential value itself.
func redactedPlaceholder(present bool) string {
	if present {
		return "***"
	}
	return ""
}
