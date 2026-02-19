package cmd

import "strings"

func isRunningStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "running")
}
