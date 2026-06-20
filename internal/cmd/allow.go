package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	allowCreateTo string
	allowRevokeTo string
)

var allowCmd = &cobra.Command{
	Use:   "allow <source-sandbox>",
	Short: "Allow sandbox-to-sandbox reachability",
	Long: `Allow one sandbox to reach another same-tenant sandbox on a TCP port.

The backend persists the allow rule and applies the source sandbox's Cilium
egress policy. The destination is identified as <sandbox>:<port>.`,
	Example: `  cvps allow cortex-brain --to cortex-worker:8788
  cvps allow cmqa1source000001cpq3n7r8b9 --to cmqa1dest000001cpq3n7r8b9:8080
  cvps allow list cortex-brain
  cvps allow revoke cortex-brain --to cortex-worker:8788`,
	Args: cobra.ExactArgs(1),
	RunE: runAllowCreate,
}

var allowListCmd = &cobra.Command{
	Use:     "list <source-sandbox>",
	Aliases: []string{"ls"},
	Short:   "List sandbox reachability allow rules",
	Args:    cobra.ExactArgs(1),
	RunE:    runAllowList,
}

var allowRevokeCmd = &cobra.Command{
	Use:   "revoke <source-sandbox>",
	Short: "Revoke sandbox-to-sandbox reachability",
	Args:  cobra.ExactArgs(1),
	RunE:  runAllowRevoke,
}

func init() {
	rootCmd.AddCommand(allowCmd)
	allowCmd.AddCommand(allowListCmd)
	allowCmd.AddCommand(allowRevokeCmd)

	allowCmd.Flags().StringVar(&allowCreateTo, "to", "", "destination as <sandbox-id-or-name>:<port>")
	allowRevokeCmd.Flags().StringVar(&allowRevokeTo, "to", "", "destination as <sandbox-id-or-name>:<port>")
}

func runAllowCreate(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(allowCreateTo) == "" {
		return fmt.Errorf("--to is required")
	}
	return runAllowMutation(args[0], allowCreateTo, false)
}

func runAllowRevoke(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(allowRevokeTo) == "" {
		return fmt.Errorf("--to is required")
	}
	return runAllowMutation(args[0], allowRevokeTo, true)
}

func runAllowMutation(sourceRef, to string, revoke bool) error {
	targetRef, port, err := parseAllowTarget(to)
	if err != nil {
		return err
	}

	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	sourceID, err := resolveSandboxRef(ctx, client, sourceRef)
	if err != nil {
		return fmt.Errorf("failed to resolve source sandbox: %w", err)
	}
	targetID, err := resolveSandboxRef(ctx, client, targetRef)
	if err != nil {
		return fmt.Errorf("failed to resolve target sandbox: %w", err)
	}

	if revoke {
		fmt.Printf("Revoking allow rule %s -> %s:%d...\n", sourceID, targetID, port)
		rule, err := client.RevokeSandboxReachability(ctx, sourceID, targetID, port)
		if err != nil {
			return fmt.Errorf("failed to revoke allow rule: %w", err)
		}
		if rule.Changed != nil && !*rule.Changed {
			fmt.Printf("✓ Allow rule already absent: %s -> %s:%d/%s\n", sourceID, targetID, port, rule.Protocol)
			return nil
		}
		fmt.Printf("✓ Allow rule revoked: %s -> %s:%d/%s\n", sourceID, targetID, port, rule.Protocol)
		return nil
	}

	fmt.Printf("Allowing %s to reach %s:%d...\n", sourceID, targetID, port)
	rule, err := client.AllowSandboxReachability(ctx, sourceID, targetID, port)
	if err != nil {
		return fmt.Errorf("failed to create allow rule: %w", err)
	}
	if rule.Changed != nil && !*rule.Changed {
		fmt.Printf("✓ Allow rule already exists: %s -> %s:%d/%s\n", sourceID, targetID, port, rule.Protocol)
		return nil
	}
	fmt.Printf("✓ Allow rule created: %s -> %s:%d/%s\n", sourceID, targetID, port, rule.Protocol)
	return nil
}

func runAllowList(cmd *cobra.Command, args []string) error {
	client, err := newAuthenticatedClient()
	if err != nil {
		return err
	}
	ctx := context.Background()

	sourceID, err := resolveSandboxRef(ctx, client, args[0])
	if err != nil {
		return fmt.Errorf("failed to resolve source sandbox: %w", err)
	}

	rules, err := client.ListSandboxAllowRules(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("failed to list allow rules: %w", err)
	}
	if len(rules) == 0 {
		fmt.Printf("No allow rules for sandbox %s.\n", sourceID)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TARGET\tPORT\tPROTOCOL\tID\tCREATED")
	for _, rule := range rules {
		target := rule.TargetSandboxID
		if rule.TargetSandboxName != "" {
			target = fmt.Sprintf("%s (%s)", rule.TargetSandboxName, rule.TargetSandboxID)
		}
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n",
			target,
			rule.Port,
			rule.Protocol,
			rule.ID,
			formatSecretDate(rule.CreatedAt),
		)
	}
	return w.Flush()
}

func parseAllowTarget(value string) (string, int, error) {
	value = strings.TrimSpace(value)
	separator := strings.LastIndex(value, ":")
	if separator <= 0 || separator == len(value)-1 {
		return "", 0, fmt.Errorf("--to must be in the form <sandbox-id-or-name>:<port>")
	}

	target := strings.TrimSpace(value[:separator])
	portText := strings.TrimSpace(value[separator+1:])
	if target == "" {
		return "", 0, fmt.Errorf("--to target sandbox cannot be empty")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("--to port must be an integer between 1 and 65535")
	}
	return target, port, nil
}
