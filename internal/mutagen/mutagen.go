package mutagen

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// SessionConfig contains configuration for creating a sync session
type SessionConfig struct {
	Name       string
	LocalPath  string
	RemoteHost string
	RemotePort int
	RemotePath string
	Ignores    []string
	OneWay     string // "local-to-remote", "remote-to-local", or ""
	Verbose    bool
}

// Session represents an active Mutagen sync session
type Session struct {
	ID     string
	Name   string
	config SessionConfig
}

// SessionStatus contains the status of a sync session
type SessionStatus struct {
	Status     string
	LocalPath  string
	RemotePath string
	Conflicts  int
}

// IsInstalled checks if Mutagen is available in PATH
func IsInstalled() bool {
	_, err := exec.LookPath("mutagen")
	return err == nil
}

// CreateSession creates a new Mutagen sync session
func CreateSession(cfg SessionConfig) (*Session, error) {
	// Build mutagen sync create command
	args := []string{
		"sync", "create",
		"--name", cfg.Name,
	}

	// Add ignore patterns
	for _, ignore := range cfg.Ignores {
		args = append(args, "--ignore", ignore)
	}

	// Add one-way mode
	switch cfg.OneWay {
	case "local-to-remote":
		args = append(args, "--sync-mode", "one-way-safe")
	case "remote-to-local":
		args = append(args, "--sync-mode", "one-way-replica")
	}

	// Source and destination
	args = append(args, cfg.LocalPath)

	// Build remote URL - Mutagen expects format: user@host:port:path
	remoteURL := fmt.Sprintf("%s:%s", cfg.RemoteHost, cfg.RemotePath)
	if cfg.RemotePort != 0 && cfg.RemotePort != 22 {
		// For non-standard ports, we need to use SSH config or pass via SSH options
		args = append(args, "--ssh-args", fmt.Sprintf("-p %d", cfg.RemotePort))
	}
	args = append(args, remoteURL)

	cmd := exec.Command("mutagen", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("mutagen create failed: %s", string(output))
	}

	// Parse session ID from output (Mutagen typically prints "Created session <id>")
	sessionID := parseSessionIDFromOutput(string(output))
	if sessionID == "" {
		sessionID = cfg.Name // Fallback to name
	}

	return &Session{
		ID:     sessionID,
		Name:   cfg.Name,
		config: cfg,
	}, nil
}

// parseSessionIDFromOutput extracts session ID from Mutagen output
func parseSessionIDFromOutput(output string) string {
	// Look for "Created session <id>" pattern
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Created session") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				return parts[2]
			}
		}
	}
	return ""
}

// Monitor starts monitoring the sync session and streams output
func (s *Session) Monitor(out io.Writer) error {
	cmd := exec.Command("mutagen", "sync", "monitor", s.Name)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// Terminate stops the sync session
func (s *Session) Terminate() error {
	return TerminateSession(s.Name)
}

// GetSessionStatus retrieves the current status of a sync session
func GetSessionStatus(name string) (*SessionStatus, error) {
	cmd := exec.Command("mutagen", "sync", "list", "--template", "{{json .}}", name)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get session status: %w", err)
	}

	// Parse JSON output
	var sessions []struct {
		Status struct {
			Description string `json:"description"`
		} `json:"status"`
		Alpha struct {
			Path string `json:"path"`
		} `json:"alpha"`
		Beta struct {
			Path string `json:"path"`
		} `json:"beta"`
		Conflicts []interface{} `json:"conflicts"`
	}

	if err := json.Unmarshal(output, &sessions); err != nil {
		return nil, fmt.Errorf("failed to parse session status: %w", err)
	}

	if len(sessions) == 0 {
		return nil, fmt.Errorf("session not found: %s", name)
	}

	s := sessions[0]
	return &SessionStatus{
		Status:     s.Status.Description,
		LocalPath:  s.Alpha.Path,
		RemotePath: s.Beta.Path,
		Conflicts:  len(s.Conflicts),
	}, nil
}

// TerminateSession terminates a sync session by name
func TerminateSession(name string) error {
	cmd := exec.Command("mutagen", "sync", "terminate", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to terminate session: %s", string(output))
	}
	return nil
}

// ListSessions lists all CVPS sync sessions
func ListSessions() ([]string, error) {
	cmd := exec.Command("mutagen", "sync", "list", "--template", "{{range .}}{{.Name}}\n{{end}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	var names []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(name, "cvps-") {
			names = append(names, name)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse session list: %w", err)
	}

	return names, nil
}
