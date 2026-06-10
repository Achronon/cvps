package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.APIBaseURL != "https://api.claudevps.com" {
		t.Errorf("expected APIBaseURL to be https://api.claudevps.com, got %s", cfg.APIBaseURL)
	}

	if cfg.Defaults.CPUCores != 1 {
		t.Errorf("expected CPUCores to be 1, got %d", cfg.Defaults.CPUCores)
	}

	if cfg.Defaults.MemoryGB != 2 {
		t.Errorf("expected MemoryGB to be 2, got %d", cfg.Defaults.MemoryGB)
	}

	if cfg.Defaults.StorageGB != 5 {
		t.Errorf("expected StorageGB to be 5, got %d", cfg.Defaults.StorageGB)
	}

	if cfg.Defaults.Image != "ghcr.io/claudevps/claude-sandbox:latest" {
		t.Errorf("expected Image to be ghcr.io/claudevps/claude-sandbox:latest, got %s", cfg.Defaults.Image)
	}

	if cfg.Sync.Mode != "mutagen" {
		t.Errorf("expected Sync.Mode to be mutagen, got %s", cfg.Sync.Mode)
	}

	if len(cfg.Sync.IgnorePatterns) == 0 {
		t.Error("expected Sync.IgnorePatterns to have default values")
	}
}

func TestConfigDir(t *testing.T) {
	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() failed: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() failed: %v", err)
	}

	expected := filepath.Join(home, ".cvps")
	if dir != expected {
		t.Errorf("expected ConfigDir to be %s, got %s", expected, dir)
	}
}

func TestConfigPath(t *testing.T) {
	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() failed: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() failed: %v", err)
	}

	expected := filepath.Join(home, ".cvps", "config.yaml")
	if path != expected {
		t.Errorf("expected ConfigPath to be %s, got %s", expected, path)
	}
}

func TestLoadNonExistentConfig(t *testing.T) {
	// Load config when no file exists - should return defaults
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.APIBaseURL != "https://api.claudevps.com" {
		t.Errorf("expected default APIBaseURL, got %s", cfg.APIBaseURL)
	}
}

func TestSaveAndLoad(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override ConfigDir to use temp directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create a test config
	cfg := DefaultConfig()
	cfg.APIKey = "test-api-key"
	cfg.AccessToken = "test-access-token"

	// Save the config
	err := Save(cfg)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file permissions
	configPath := filepath.Join(tmpDir, ".cvps", "config.yaml")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("os.Stat() failed: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected file permissions 0600, got %o", perm)
	}

	// Load the config back
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.APIKey != "test-api-key" {
		t.Errorf("expected APIKey to be test-api-key, got %s", loaded.APIKey)
	}

	if loaded.AccessToken != "test-access-token" {
		t.Errorf("expected AccessToken to be test-access-token, got %s", loaded.AccessToken)
	}
}

func TestEnvironmentVariableOverrides(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override ConfigDir to use temp directory
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create and save a config
	cfg := DefaultConfig()
	cfg.APIKey = "config-file-key"
	cfg.APIBaseURL = "https://config-file.com"

	err := Save(cfg)
	if err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Set environment variables
	t.Setenv("CVPS_API_KEY", "cvps_env_override_key")
	t.Setenv("CVPS_API_URL", "https://env-override.com")

	// Load config - env credential must win over the stored one
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	cred, err := loaded.ResolveCredential()
	if err != nil {
		t.Fatalf("ResolveCredential() failed: %v", err)
	}
	if cred == nil || cred.Token != "cvps_env_override_key" {
		t.Errorf("expected env credential cvps_env_override_key, got %+v", cred)
	}

	// The stored field must stay untouched so Save can never persist the
	// env-injected credential.
	if loaded.APIKey != "config-file-key" {
		t.Errorf("expected stored APIKey to remain config-file-key, got %s", loaded.APIKey)
	}

	if loaded.APIBaseURL != "https://env-override.com" {
		t.Errorf("expected APIBaseURL from env to be https://env-override.com, got %s", loaded.APIBaseURL)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     DefaultConfig(),
			wantErr: false,
		},
		{
			name: "missing api_base_url",
			cfg: &Config{
				APIBaseURL: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsAuthenticated(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *Config
		expect bool
	}{
		{
			name: "has api key",
			cfg: &Config{
				APIKey: "test-key",
			},
			expect: true,
		},
		{
			name: "has access token",
			cfg: &Config{
				AccessToken: "test-token",
			},
			expect: true,
		},
		{
			name: "has both",
			cfg: &Config{
				APIKey:      "test-key",
				AccessToken: "test-token",
			},
			expect: true,
		},
		{
			name:   "has neither",
			cfg:    &Config{},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cfg.IsAuthenticated()
			if result != tt.expect {
				t.Errorf("IsAuthenticated() = %v, expected %v", result, tt.expect)
			}
		})
	}
}

// --- HLM-382: headless auth ---

func TestLoadFreshShellEnvTokenOnly(t *testing.T) {
	// No config file at all: env-only auth must still work (the HLM-375
	// acceptance scenario; previously Load early-returned before env
	// overrides were applied).
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CVPS_API_TOKEN", "cvps_fresh_shell_token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if !cfg.IsAuthenticated() {
		t.Fatal("expected IsAuthenticated() with only CVPS_API_TOKEN set")
	}

	cred, err := cfg.ResolveCredential()
	if err != nil {
		t.Fatalf("ResolveCredential() failed: %v", err)
	}
	if cred == nil || cred.Token != "cvps_fresh_shell_token" {
		t.Fatalf("expected env token, got %+v", cred)
	}
	if !cred.IsAPIKey {
		t.Error("cvps_-prefixed token should resolve as an API key")
	}
}

func TestEnvTokenPrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "CVPS_API_TOKEN wins over CVPS_TOKEN and CVPS_API_KEY",
			env: map[string]string{
				"CVPS_API_TOKEN": "tok-api-token",
				"CVPS_TOKEN":     "tok-token",
				"CVPS_API_KEY":   "tok-api-key",
			},
			want: "tok-api-token",
		},
		{
			name: "CVPS_TOKEN wins over CVPS_API_KEY",
			env: map[string]string{
				"CVPS_TOKEN":   "tok-token",
				"CVPS_API_KEY": "tok-api-key",
			},
			want: "tok-token",
		},
		{
			name: "CVPS_API_KEY alone still works",
			env:  map[string]string{"CVPS_API_KEY": "tok-api-key"},
			want: "tok-api-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, name := range []string{"CVPS_API_TOKEN", "CVPS_TOKEN", "CVPS_API_KEY"} {
				t.Setenv(name, "")
				os.Unsetenv(name)
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() failed: %v", err)
			}
			cred, err := cfg.ResolveCredential()
			if err != nil {
				t.Fatalf("ResolveCredential() failed: %v", err)
			}
			if cred == nil || cred.Token != tt.want {
				t.Errorf("expected token %q, got %+v", tt.want, cred)
			}
		})
	}
}

func TestTokenCommand(t *testing.T) {
	t.Run("resolves trimmed stdout", func(t *testing.T) {
		cfg := &Config{TokenCommand: "echo '  cvps_from_command  '"}
		cred, err := cfg.ResolveCredential()
		if err != nil {
			t.Fatalf("ResolveCredential() failed: %v", err)
		}
		if cred == nil || cred.Token != "cvps_from_command" {
			t.Fatalf("expected trimmed token, got %+v", cred)
		}
		if !cred.IsAPIKey {
			t.Error("cvps_-prefixed token should resolve as an API key")
		}
	})

	t.Run("non-cvps token resolves as bearer", func(t *testing.T) {
		cfg := &Config{TokenCommand: "echo some.jwt.token"}
		cred, err := cfg.ResolveCredential()
		if err != nil {
			t.Fatalf("ResolveCredential() failed: %v", err)
		}
		if cred == nil || cred.IsAPIKey {
			t.Errorf("expected bearer credential, got %+v", cred)
		}
	})

	t.Run("empty output is an error", func(t *testing.T) {
		cfg := &Config{TokenCommand: "true"}
		if _, err := cfg.ResolveCredential(); err == nil {
			t.Fatal("expected error for empty token_command output")
		}
	})

	t.Run("command failure is an error", func(t *testing.T) {
		cfg := &Config{TokenCommand: "exit 3"}
		if _, err := cfg.ResolveCredential(); err == nil {
			t.Fatal("expected error for failing token_command")
		}
	})

	t.Run("env token beats token_command", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CVPS_API_TOKEN", "tok-env")
		t.Setenv("CVPS_TOKEN_COMMAND", "echo tok-command")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}
		cred, err := cfg.ResolveCredential()
		if err != nil {
			t.Fatalf("ResolveCredential() failed: %v", err)
		}
		if cred == nil || cred.Token != "tok-env" {
			t.Errorf("expected env token to win, got %+v", cred)
		}
	})

	t.Run("CVPS_TOKEN_COMMAND env sets TokenCommand", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("CVPS_TOKEN_COMMAND", "echo tok-from-env-command")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() failed: %v", err)
		}
		if !cfg.IsAuthenticated() {
			t.Fatal("expected IsAuthenticated() with CVPS_TOKEN_COMMAND set")
		}
		cred, err := cfg.ResolveCredential()
		if err != nil {
			t.Fatalf("ResolveCredential() failed: %v", err)
		}
		if cred == nil || cred.Token != "tok-from-env-command" {
			t.Errorf("expected token from command, got %+v", cred)
		}
		// The env override must stay out of the persistable field so a
		// later Save cannot write it to disk.
		if cfg.TokenCommand != "" {
			t.Errorf("CVPS_TOKEN_COMMAND must not populate the persisted TokenCommand field, got %q", cfg.TokenCommand)
		}
		if err := Save(cfg); err != nil {
			t.Fatalf("Save() failed: %v", err)
		}
		path, err := ConfigPath()
		if err != nil {
			t.Fatalf("ConfigPath() failed: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() failed: %v", err)
		}
		if strings.Contains(string(data), "tok-from-env-command") || strings.Contains(string(data), "token_command") {
			t.Fatal("CVPS_TOKEN_COMMAND leaked into the persisted config file")
		}
	})

	t.Run("env CVPS_TOKEN_COMMAND beats stored token_command", func(t *testing.T) {
		t.Setenv("CVPS_TOKEN_COMMAND", "echo tok-env-cmd")
		cfg := &Config{TokenCommand: "echo tok-stored-cmd"}
		applyEnvOverrides(cfg)
		cred, err := cfg.ResolveCredential()
		if err != nil {
			t.Fatalf("ResolveCredential() failed: %v", err)
		}
		if cred == nil || cred.Token != "tok-env-cmd" {
			t.Errorf("expected env token command to win, got %+v", cred)
		}
	})

	t.Run("token_command beats stored config", func(t *testing.T) {
		cfg := &Config{
			TokenCommand: "echo tok-command",
			AccessToken:  "stored-access-token",
			APIKey:       "stored-api-key",
		}
		cred, err := cfg.ResolveCredential()
		if err != nil {
			t.Fatalf("ResolveCredential() failed: %v", err)
		}
		if cred == nil || cred.Token != "tok-command" {
			t.Errorf("expected token_command to win over stored config, got %+v", cred)
		}
	})
}

func TestResolveCredentialStoredConfig(t *testing.T) {
	t.Run("access token preferred over api key", func(t *testing.T) {
		cfg := &Config{AccessToken: "stored-token", APIKey: "stored-key"}
		cred, err := cfg.ResolveCredential()
		if err != nil {
			t.Fatalf("ResolveCredential() failed: %v", err)
		}
		if cred == nil || cred.Token != "stored-token" || cred.IsAPIKey {
			t.Errorf("expected stored access token as bearer, got %+v", cred)
		}
	})

	t.Run("api key fallback uses X-API-Key", func(t *testing.T) {
		cfg := &Config{APIKey: "stored-key"}
		cred, err := cfg.ResolveCredential()
		if err != nil {
			t.Fatalf("ResolveCredential() failed: %v", err)
		}
		if cred == nil || cred.Token != "stored-key" || !cred.IsAPIKey {
			t.Errorf("expected stored API key credential, got %+v", cred)
		}
	})

	t.Run("no credential", func(t *testing.T) {
		cfg := &Config{}
		cred, err := cfg.ResolveCredential()
		if err != nil {
			t.Fatalf("ResolveCredential() failed: %v", err)
		}
		if cred != nil {
			t.Errorf("expected nil credential, got %+v", cred)
		}
	})
}

func TestSaveNeverPersistsEnvToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CVPS_API_TOKEN", "cvps_env_secret_token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}
	if strings.Contains(string(data), "cvps_env_secret_token") {
		t.Fatal("env-injected token leaked into the persisted config file")
	}
}
