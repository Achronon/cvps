package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

type Config struct {
	// Authentication
	APIKey      string `yaml:"api_key" mapstructure:"api_key"`
	AccessToken string `yaml:"access_token,omitempty" mapstructure:"access_token"`

	// TokenCommand is an optional shell command whose stdout is the API
	// token (e.g. "op read op://vault/cvps/token"). It is resolved at
	// client-build time so the token itself never touches disk. Also
	// settable via the CVPS_TOKEN_COMMAND environment variable.
	TokenCommand string `yaml:"token_command,omitempty" mapstructure:"token_command"`

	// API settings
	APIBaseURL string `yaml:"api_base_url" mapstructure:"api_base_url"`

	// Default sandbox settings
	Defaults SandboxDefaults `yaml:"defaults" mapstructure:"defaults"`

	// Sync settings
	Sync SyncConfig `yaml:"sync" mapstructure:"sync"`

	// envToken holds a token sourced from the environment
	// (CVPS_API_TOKEN / CVPS_TOKEN / CVPS_API_KEY). Unexported on purpose:
	// it must never be persisted by Save (yaml.Marshal skips unexported
	// fields), so env-injected credentials cannot leak into config.yaml.
	envToken string

	// envTokenIsAPIKey records how envToken must be sent: CVPS_API_KEY has
	// always meant X-API-Key (regardless of prefix, for back-compat),
	// while CVPS_API_TOKEN/CVPS_TOKEN are classified by the cvps_ prefix.
	envTokenIsAPIKey bool

	// envTokenCommand holds CVPS_TOKEN_COMMAND. Kept separate from the
	// persisted TokenCommand field for the same reason as envToken: a
	// process-scoped env override must never be written to disk by a
	// later Save (login, logout, config set).
	envTokenCommand string
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

	// No config file: start from defaults. Environment overrides below
	// still apply, so a fresh shell with only CVPS_API_TOKEN set is fully
	// authenticated without ever running 'cvps login'.
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := DefaultConfig()
		applyEnvOverrides(cfg)
		return cfg, nil
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// NOTE: no viper.AutomaticEnv() here on purpose. Environment handling
	// is explicit in applyEnvOverrides so env-injected credentials land in
	// the unexported envToken field and can never be persisted by Save.

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	applyEnvOverrides(&cfg)
	return &cfg, nil
}

// applyEnvOverrides layers environment variables over the stored config.
// Token material goes into the unexported envToken field so it can never
// be persisted back to disk by Save.
func applyEnvOverrides(cfg *Config) {
	// Highest-precedence first: CVPS_API_TOKEN (HLM-375) > CVPS_TOKEN
	// (provisioner-injected credential, HLM-372) > CVPS_API_KEY (legacy
	// alias). CVPS_API_KEY has always been sent as X-API-Key, so it stays
	// an API key regardless of prefix; the new variables are classified
	// by the cvps_ prefix.
	if token := os.Getenv("CVPS_API_TOKEN"); token != "" {
		cfg.envToken = token
		cfg.envTokenIsAPIKey = strings.HasPrefix(token, "cvps_")
	} else if token := os.Getenv("CVPS_TOKEN"); token != "" {
		cfg.envToken = token
		cfg.envTokenIsAPIKey = strings.HasPrefix(token, "cvps_")
	} else if token := os.Getenv("CVPS_API_KEY"); token != "" {
		cfg.envToken = token
		cfg.envTokenIsAPIKey = true
	}
	if tokenCmd := os.Getenv("CVPS_TOKEN_COMMAND"); tokenCmd != "" {
		cfg.envTokenCommand = tokenCmd
	}
	if apiURL := os.Getenv("CVPS_API_URL"); apiURL != "" {
		cfg.APIBaseURL = apiURL
	}
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
	return c.envToken != "" || c.envTokenCommand != "" || c.TokenCommand != "" ||
		c.APIKey != "" || c.AccessToken != ""
}

// Credential is a resolved API credential.
type Credential struct {
	Token string
	// IsAPIKey selects the X-API-Key header; otherwise the credential is
	// sent as an Authorization: Bearer token. The backend accepts
	// cvps_-prefixed API keys via either header.
	IsAPIKey bool
}

// ResolveCredential resolves the effective credential at client-build time.
// Precedence: environment token (CVPS_API_TOKEN > CVPS_TOKEN >
// CVPS_API_KEY) > token command (CVPS_TOKEN_COMMAND env, then the stored
// token_command) > stored access_token > stored api_key. Returns
// (nil, nil) when no credential is configured.
func (c *Config) ResolveCredential() (*Credential, error) {
	if c.envToken != "" {
		return &Credential{Token: c.envToken, IsAPIKey: c.envTokenIsAPIKey}, nil
	}

	tokenCommand := c.envTokenCommand
	if tokenCommand == "" {
		tokenCommand = c.TokenCommand
	}
	if tokenCommand != "" {
		token, err := runTokenCommand(tokenCommand)
		if err != nil {
			return nil, err
		}
		return credentialFromToken(token), nil
	}

	if c.AccessToken != "" {
		return &Credential{Token: c.AccessToken}, nil
	}
	if c.APIKey != "" {
		return &Credential{Token: c.APIKey, IsAPIKey: true}, nil
	}

	return nil, nil
}

// credentialFromToken classifies a dynamically sourced token: cvps_-prefixed
// values are API keys (X-API-Key, matching the legacy CVPS_API_KEY
// behavior); anything else is treated as a bearer token (e.g. a JWT).
func credentialFromToken(token string) *Credential {
	return &Credential{Token: token, IsAPIKey: strings.HasPrefix(token, "cvps_")}
}

// runTokenCommand executes token_command via the platform shell and
// returns its trimmed stdout. The token only ever exists in memory.
func runTokenCommand(command string) (string, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/C", command)
	} else {
		cmd = exec.Command("/bin/sh", "-c", command)
	}
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("token_command failed: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("token_command produced no output")
	}
	return token, nil
}
