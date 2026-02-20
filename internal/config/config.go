package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	// Authentication
	APIKey      string `yaml:"api_key" mapstructure:"api_key"`
	AccessToken string `yaml:"access_token,omitempty" mapstructure:"access_token"`

	// API settings
	APIBaseURL string `yaml:"api_base_url" mapstructure:"api_base_url"`

	// Default sandbox settings
	Defaults SandboxDefaults `yaml:"defaults" mapstructure:"defaults"`

	// Sync settings
	Sync SyncConfig `yaml:"sync" mapstructure:"sync"`
}

type SandboxDefaults struct {
	CPUCores  int    `yaml:"cpu_cores" mapstructure:"cpu_cores"`
	MemoryGB  int    `yaml:"memory_gb" mapstructure:"memory_gb"`
	StorageGB int    `yaml:"storage_gb" mapstructure:"storage_gb"`
	Image     string `yaml:"image" mapstructure:"image"`
}

type SyncConfig struct {
	IgnorePatterns []string `yaml:"ignore_patterns" mapstructure:"ignore_patterns"`
	Mode           string   `yaml:"mode" mapstructure:"mode"` // "mutagen" or "rsync"
}

func DefaultConfig() *Config {
	return &Config{
		APIBaseURL: "https://api.claudevps.com",
		Defaults: SandboxDefaults{
			CPUCores:  1,
			MemoryGB:  2,
			StorageGB: 5,
			Image:     "ghcr.io/claudevps/claude-sandbox:latest",
		},
		Sync: SyncConfig{
			IgnorePatterns: []string{
				"node_modules/",
				".git/",
				"vendor/",
				"__pycache__/",
				".next/",
				"dist/",
				"build/",
			},
			Mode: "mutagen",
		},
	}
}

func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".cvps"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func Load() (*Config, error) {
	configPath, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	// Return defaults if config doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// Environment variable overrides
	viper.SetEnvPrefix("CVPS")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply env var overrides
	if apiKey := os.Getenv("CVPS_API_KEY"); apiKey != "" {
		cfg.APIKey = apiKey
	}
	if apiURL := os.Getenv("CVPS_API_URL"); apiURL != "" {
		cfg.APIBaseURL = apiURL
	}

	return &cfg, nil
}

func Save(cfg *Config) error {
	configDir, err := ConfigDir()
	if err != nil {
		return err
	}

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write with restricted permissions (user-only)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (c *Config) Validate() error {
	if c.APIBaseURL == "" {
		return fmt.Errorf("api_base_url is required")
	}
	return nil
}

func (c *Config) IsAuthenticated() bool {
	return c.APIKey != "" || c.AccessToken != ""
}
