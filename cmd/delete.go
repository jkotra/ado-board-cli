package cmd

import (
	"fmt"
	"strconv"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/spf13/cobra"
)

var destroyFlag bool

var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a work item from Azure DevOps and local database",
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

		fmt.Printf("🗑️ Deleting work item #%d (%s)...\n", id, item.Title)
		client := ado.NewClient(item.Org, item.Project, cfg.PatToken)
		if err := client.DeleteWorkItem(id, destroyFlag); err != nil {
			return fmt.Errorf("failed to delete from ADO: %w", err)
		}

		if err := database.DeleteWorkItem(id); err != nil {
			return err
		}

		fmt.Printf("✅ Deleted work item #%d\n", id)
		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolVar(&destroyFlag, "destroy", false, "Permanently destroy work item instead of soft deleting")
	rootCmd.AddCommand(deleteCmd)
}
