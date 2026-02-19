package migration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// Config contains configuration for the migration process
type Config struct {
	LocalPath  string
	SSHHost    string
	SSHPort    int
	SSHUser    string
	RemotePath string
	Resume     bool
}

// Result contains the results of a migration operation
type Result struct {
	FilesTransferred int
	FilesSkipped     int
	BytesTransferred int64
}

// Migrator handles the file migration process using rsync
type Migrator struct {
	config Config
}

// NewMigrator creates a new migrator with the given configuration
func NewMigrator(cfg Config) *Migrator {
	return &Migrator{config: cfg}
}

// Run executes the migration, calling onProgress periodically with bytes transferred
func (m *Migrator) Run(ctx context.Context, files *ScanResult, onProgress func(int64)) (*Result, error) {
	// Use rsync for efficient transfer
	args := []string{
		"-avz",
		"--progress",
		"--partial",
	}

	if m.config.Resume {
		args = append(args, "--append-verify")
	}

	// SSH options
	sshCmd := fmt.Sprintf("ssh -p %d -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null",
		m.config.SSHPort)
	args = append(args, "-e", sshCmd)

	// Source (with trailing slash to copy contents)
	args = append(args, m.config.LocalPath+"/")

	// Destination
	dest := fmt.Sprintf("%s@%s:%s/",
		m.config.SSHUser, m.config.SSHHost, m.config.RemotePath)
	args = append(args, dest)

	cmd := exec.CommandContext(ctx, "rsync", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rsync failed: %w", err)
	}

	return &Result{
		FilesTransferred: files.Count,
		BytesTransferred: files.TotalSize,
	}, nil
}
