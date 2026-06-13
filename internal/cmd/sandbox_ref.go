package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/achronon/cvps/internal/api"
)

var sandboxCUIDLikePattern = regexp.MustCompile(`^c[a-z0-9]{20,}$`)

func resolveSandboxIDByName(ctx context.Context, client *api.Client, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("sandbox name cannot be empty")
	}

	sandboxes, err := listAllSandboxesForConnect(ctx, client)
	if err != nil {
		return "", fmt.Errorf("failed to list sandboxes: %w", err)
	}

	matches := make([]api.Sandbox, 0, 2)
	for _, sandbox := range sandboxes {
		if strings.EqualFold(strings.TrimSpace(sandbox.Name), name) {
			matches = append(matches, sandbox)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("sandbox named %q not found. Run 'cvps status --all' to view available sandboxes", name)
	case 1:
		return matches[0].ID, nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "sandbox name %q is ambiguous. Use a sandbox ID:\n", name)
		for _, sandbox := range matches {
			fmt.Fprintf(&b, "  - %s (%s)\n", sandbox.ID, sandbox.Name)
		}
		return "", fmt.Errorf(strings.TrimRight(b.String(), "\n"))
	}
}

func listAllSandboxesForConnect(ctx context.Context, client *api.Client) ([]api.Sandbox, error) {
	const pageSize = 100
	const maxPages = 20

	all := make([]api.Sandbox, 0, pageSize)
	for page := 1; page <= maxPages; page++ {
		list, err := client.ListSandboxes(ctx, page, pageSize)
		if err != nil {
			return nil, err
		}

		all = append(all, list.Data...)
		if len(list.Data) < pageSize || len(all) >= list.Total {
			break
		}
	}

	return all, nil
}

func looksLikeSandboxID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	return strings.HasPrefix(value, "sbx-") || sandboxCUIDLikePattern.MatchString(value)
}
