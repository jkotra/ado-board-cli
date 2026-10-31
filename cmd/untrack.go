package cmd

import (
	"fmt"
	"strconv"

	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/spf13/cobra"
)

var untrackCmd = &cobra.Command{
	Use:   "untrack <id>",
	Short: "Untrack an Azure DevOps work item tree root",
	Long:  `Untrack a previously tracked root work item from the local board.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid work item ID: %s", args[0])
		}

		cfg, err := config.Load(flagDbPath, flagToken)
		if err != nil {
			return err
		}

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()

		if err := database.DeleteTrackedRoot(id); err != nil {
			return fmt.Errorf("failed to untrack item #%d: %w", id, err)
		}

		fmt.Printf("✅ Successfully untracked work item #%d\n", id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(untrackCmd)
}
