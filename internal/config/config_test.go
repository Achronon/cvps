package config

import (
	"os"
	"path/filepath"
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
	os.Setenv("CVPS_API_KEY", "env-override-key")
	os.Setenv("CVPS_API_URL", "https://env-override.com")
	defer os.Unsetenv("CVPS_API_KEY")
	defer os.Unsetenv("CVPS_API_URL")

	// Load config - should use env vars
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.APIKey != "env-override-key" {
		t.Errorf("expected APIKey from env to be env-override-key, got %s", loaded.APIKey)
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
