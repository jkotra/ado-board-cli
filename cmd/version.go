package cmd

import (
	"fmt"

	"ado-board-cli/pkg/version"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the semantic version of ado-board-cli",
	Long:  "Display the current version, git commit, and build information for ado-board-cli.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ado-board-cli version %s\n", version.String())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
