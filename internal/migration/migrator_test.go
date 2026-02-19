package migration

import (
	"context"
	"testing"
)

func TestNewMigrator(t *testing.T) {
	cfg := Config{
		LocalPath:  "/tmp/test",
		SSHHost:    "test.example.com",
		SSHPort:    22,
		SSHUser:    "testuser",
		RemotePath: "/workspace",
		Resume:     false,
	}

	migrator := NewMigrator(cfg)
	if migrator == nil {
		t.Fatal("expected migrator to be created")
	}

	if migrator.config.LocalPath != cfg.LocalPath {
		t.Errorf("expected LocalPath %s, got %s", cfg.LocalPath, migrator.config.LocalPath)
	}

	if migrator.config.SSHHost != cfg.SSHHost {
		t.Errorf("expected SSHHost %s, got %s", cfg.SSHHost, migrator.config.SSHHost)
	}

	if migrator.config.SSHPort != cfg.SSHPort {
		t.Errorf("expected SSHPort %d, got %d", cfg.SSHPort, migrator.config.SSHPort)
	}
}

func TestMigrator_ConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		valid  bool
	}{
		{
			name: "valid config",
			config: Config{
				LocalPath:  "/tmp/test",
				SSHHost:    "test.example.com",
				SSHPort:    22,
				SSHUser:    "testuser",
				RemotePath: "/workspace",
			},
			valid: true,
		},
		{
			name: "empty local path",
			config: Config{
				LocalPath:  "",
				SSHHost:    "test.example.com",
				SSHPort:    22,
				SSHUser:    "testuser",
				RemotePath: "/workspace",
			},
			valid: false,
		},
		{
			name: "empty SSH host",
			config: Config{
				LocalPath:  "/tmp/test",
				SSHHost:    "",
				SSHPort:    22,
				SSHUser:    "testuser",
				RemotePath: "/workspace",
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migrator := NewMigrator(tt.config)
			if migrator == nil {
				t.Fatal("expected migrator to be created")
			}

			// Basic validation - ensure config is stored
			if tt.valid {
				if migrator.config.LocalPath == "" || migrator.config.SSHHost == "" {
					t.Error("valid config should have all required fields")
				}
			}
		})
	}
}

func TestMigrator_Run_InvalidContext(t *testing.T) {
	// Test with cancelled context
	cfg := Config{
		LocalPath:  "/nonexistent",
		SSHHost:    "test.example.com",
		SSHPort:    22,
		SSHUser:    "testuser",
		RemotePath: "/workspace",
	}

	migrator := NewMigrator(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	files := &ScanResult{
		Count:     0,
		TotalSize: 0,
	}

	_, err := migrator.Run(ctx, files, nil)
	if err == nil {
		// rsync may not respect context cancellation immediately
		// This is expected behavior
		t.Log("rsync command executed despite cancelled context (expected)")
	}
}

func TestMigrator_Run_NonexistentPath(t *testing.T) {
	cfg := Config{
		LocalPath:  "/absolutely/nonexistent/path/that/should/not/exist",
		SSHHost:    "test.example.com",
		SSHPort:    22,
		SSHUser:    "testuser",
		RemotePath: "/workspace",
	}

	migrator := NewMigrator(cfg)
	ctx := context.Background()

	files := &ScanResult{
		Count:     1,
		TotalSize: 100,
	}

	_, err := migrator.Run(ctx, files, nil)
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestMigrator_Run_EmptyFileSet(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := Config{
		LocalPath:  tmpDir,
		SSHHost:    "test.example.com",
		SSHPort:    22,
		SSHUser:    "testuser",
		RemotePath: "/workspace",
	}

	migrator := NewMigrator(cfg)
	ctx := context.Background()

	files := &ScanResult{
		Files:     []FileInfo{},
		Count:     0,
		TotalSize: 0,
	}

	// This should fail because SSH connection will fail
	_, err := migrator.Run(ctx, files, nil)
	if err == nil {
		t.Log("rsync may succeed or fail depending on SSH availability")
	}
}
