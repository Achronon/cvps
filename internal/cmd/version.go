package cmd

import (
	"fmt"

	"github.com/achronon/cvps/internal/version"
	"github.com/spf13/cobra"
)

var versionFull bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Display the version and build information for the cvps CLI.`,
	Run: func(cmd *cobra.Command, args []string) {
		if versionFull {
			fmt.Println(version.Full())
		} else {
			fmt.Println(version.String())
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolVarP(&versionFull, "full", "f", false, "show full version info")
}
