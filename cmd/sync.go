package cmd

import (
	"fmt"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync all tracked work items and their child hierarchies from Azure DevOps",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(flagDbPath, flagToken)
		if err != nil {
			return err
		}

		if cfg.PatToken == "" {
			return fmt.Errorf("PAT_TOKEN environment variable or --token flag is required")
		}

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()

		roots, err := database.GetTrackedRoots()
		if err != nil {
			return err
		}

		if len(roots) == 0 {
			fmt.Println("No tracked items found. Use 'ado-board-cli track <link>' first.")
			return nil
		}

		totalSynced := 0
		for _, r := range roots {
			fmt.Printf("🔄 Syncing #%d from %s/%s...\n", r.ID, r.Org, r.Project)
			client := ado.NewClient(r.Org, r.Project, cfg.PatToken)
			items, err := client.FetchHierarchy(r.ID)
			if err != nil {
				fmt.Printf("⚠️ Failed to sync #%d: %v\n", r.ID, err)
				continue
			}
			if err := database.SaveWorkItems(r.Org, r.Project, items); err != nil {
				return fmt.Errorf("db save error: %w", err)
			}
			totalSynced += len(items)
		}

		fmt.Printf("✅ Sync complete! Refreshed %d work items.\n", totalSynced)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
