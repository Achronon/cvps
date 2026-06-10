package cmd

import "strings"

func isRunningStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "running")
}

func isProvisioningStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "provisioning")
}

// serviceHealth renders a service sandbox's health from its status: RUNNING means the
// readiness probe passes (for cortex-style services: model auth present).
func serviceHealth(status string) string {
	switch {
	case isRunningStatus(status):
		return "healthy (ready)"
	case isProvisioningStatus(status):
		return "starting / awaiting readiness (model auth not set up yet?)"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}
