package cmd

import (
	"fmt"
	"strconv"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status <id> <new-state>",
	Short: "Update the state/status of a work item (e.g. 'To Do', 'In Progress', 'Done')",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid work item ID: %s", args[0])
		}
		newState := args[1]

		cfg, err := config.Load(flagDbPath, flagToken)
		if err != nil {
			return err
		}

		if cfg.PatToken == "" {
			return fmt.Errorf("PAT_TOKEN environment variable or --token flag is required")
		}

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return err
		}
		defer database.Close()

		item, err := database.GetWorkItem(id)
		if err != nil {
			return fmt.Errorf("item #%d not found in database: %w", id, err)
		}

		client := ado.NewClient(item.Org, item.Project, cfg.PatToken)
		updated, err := client.UpdateWorkItemFields(id, map[string]interface{}{
			"System.State": newState,
		})
		if err != nil {
			return fmt.Errorf("failed to update status in ADO: %w", err)
		}

		if err := database.SaveWorkItem(item.Org, item.Project, updated); err != nil {
			return err
		}

		fmt.Printf("✅ Updated #%d state to '%s'\n", id, newState)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
