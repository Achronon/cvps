package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/spf13/cobra"
)

// maxFilePutSize caps uploads: the files API tunnels content through a
// shell heredoc on the pod, which is not built for large payloads.
const maxFilePutSize = 5 * 1024 * 1024

var fileCmd = &cobra.Command{
	Use:   "file",
	Short: "Manage files inside a sandbox",
	Long:  `Manage files inside a sandbox via the sandbox files API (no TTY needed).`,
}

var filePutCmd = &cobra.Command{
	Use:   "put <sandbox> <local-path> <remote-path>",
	Short: "Upload a local file into a sandbox",
	Long: `Upload a local file into a sandbox via the files API. Works headlessly
(CVPS_API_TOKEN, no TTY) and also while a service sandbox is still
PROVISIONING, so bootstrap files can land before first readiness.

Parent directories are created automatically.

Text files only: the backend writes through a shell heredoc, so NUL
bytes are unsupported and a missing trailing newline is appended.`,
	Example: `  # Unattended cortex bootstrap (BYO codex auth.json, no device-auth):
  cvps up --profile cortex --name cortex-brain --secret TELEGRAM_BOT_TOKEN
  cvps file put cortex-brain ~/.codex/auth.json /workspace/.codex/auth.json
  cvps status   # until RUNNING (readiness gates on auth.json)`,
	Args: cobra.ExactArgs(3),
	RunE: runFilePut,
}

func init() {
	rootCmd.AddCommand(fileCmd)
	fileCmd.AddCommand(filePutCmd)
}

// validateRemotePath mirrors the backend's path rules so we fail fast:
// absolute, no path traversal.
func validateRemotePath(remotePath string) (string, error) {
	if !strings.HasPrefix(remotePath, "/") {
		return "", fmt.Errorf("remote path must be absolute (got %q)", remotePath)
	}
	if strings.Contains(remotePath, "..") {
		return "", fmt.Errorf("remote path must not contain '..' (got %q)", remotePath)
	}
	cleaned := path.Clean(remotePath)
	if cleaned == "/" {
		return "", fmt.Errorf("remote path must name a file, not the filesystem root")
	}
	return cleaned, nil
}

// guardFileContent rejects payloads the backend's heredoc-based write
// would silently corrupt.
func guardFileContent(content []byte) error {
	if len(content) > maxFilePutSize {
		return fmt.Errorf("file is %d bytes; cvps file put caps uploads at %d bytes", len(content), maxFilePutSize)
	}
	if bytes.IndexByte(content, 0) >= 0 {
		return fmt.Errorf("file contains NUL bytes; the sandbox files API only supports text files")
	}
	for _, line := range bytes.Split(content, []byte("\n")) {
		if string(bytes.TrimSuffix(line, []byte("\r"))) == "FILECONTENT" {
			return fmt.Errorf("file contains a line exactly matching the backend's heredoc delimiter (FILECONTENT) and would be corrupted in transit; adjust the file or upload it differently (e.g. base64-wrap it yourself and decode in the sandbox with 'cvps exec')")
		}
	}
	return nil
}

func runFilePut(cmd *cobra.Command, args []string) error {
	sandboxRef, localPath, remoteArg := args[0], args[1], args[2]

	remotePath, err := validateRemotePath(remoteArg)
	if err != nil {
		return err
	}

	content, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", localPath, err)
	}
	if err := guardFileContent(content); err != nil {
		return err
	}

	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	sandboxID := sandboxRef
	if !looksLikeSandboxID(sandboxRef) {
		sandboxID, err = resolveSandboxIDByName(ctx, client, sandboxRef)
		if err != nil {
			return err
		}
	}

	// No running-status gate on purpose: the files API rides pods/exec,
	// which works while a service sandbox is still PROVISIONING (the
	// whole point for bootstrap files like codex auth.json).

	// The backend write does NOT create parent directories.
	if parent := path.Dir(remotePath); parent != "/" {
		if _, err := client.CreateDirectory(ctx, sandboxID, parent); err != nil {
			return fmt.Errorf("failed to create remote directory %s: %w", parent, err)
		}
	}

	resp, err := client.WriteFile(ctx, sandboxID, remotePath, content)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", remotePath, err)
	}

	fmt.Printf("✓ Wrote %d bytes to %s:%s\n", len(content), sandboxRef, resp.Path)
	return nil
}
