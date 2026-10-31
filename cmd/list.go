package cmd

import (
	"fmt"

	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
	"ado-board-cli/pkg/tui"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list [work_item_id]",
	Short: "List all tracked work items and hierarchical trees, or a specific item hierarchy",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(flagDbPath, flagToken)
		if err != nil {
			return err
		}

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return err
		}
		defer database.Close()

		if len(args) == 1 {
			var rootID int
			if _, err := fmt.Sscanf(args[0], "%d", &rootID); err != nil || rootID <= 0 {
				return fmt.Errorf("invalid work item ID: %s", args[0])
			}

			rootRec, err := database.GetWorkItem(rootID)
			if err != nil || rootRec == nil {
				return fmt.Errorf("work item #%d not found in local database", rootID)
			}

			descendants, err := database.GetDescendantWorkItems(rootID)
			if err != nil {
				return err
			}

			rootNode := db.BuildSingleTree(rootRec, descendants)
			flattened := db.FlattenTree([]*db.TreeNode{rootNode}, "")

			fmt.Printf("Work Item #%d Hierarchy (%d total items):\n\n", rootID, len(descendants)+1)
			for _, row := range flattened {
				origEst := row.Node.EstimateDisplayString()
				prog := row.Node.RollupProgressString()
				assignee := row.Node.Record.AssignedToName
				if assignee == "" {
					assignee = "Unassigned"
				}

				fmt.Printf("  %s#%-6d [%-12s] [%-11s] Est: %-6s | Work: %-15s | %-16s | %s\n",
					row.Prefix,
					row.Node.Record.ID,
					tui.FormatItemType(row.Node.Record.WorkItemType),
					row.Node.Record.State,
					origEst,
					prog,
					assignee,
					row.Node.Record.Title,
				)
			}
			return nil
		}

		items, err := database.GetAllWorkItems()
		if err != nil {
			return err
		}

		roots, err := database.GetTrackedRoots()
		if err != nil {
			return err
		}

		queries, err := database.GetTrackedQueries()
		if err != nil {
			return err
		}

		if len(items) == 0 && len(queries) == 0 && len(roots) == 0 {
			fmt.Println("No work items or queries found in local database. Use 'ado-board-cli track <link>' to track an item or query.")
			return nil
		}

		tree := db.BuildTree(items, roots, queries)
		flattened := db.FlattenTree(tree, "")

		fmt.Printf("Work Item Hierarchy (%d items total):\n\n", len(items))
		for _, row := range flattened {
			origEst := row.Node.EstimateDisplayString()
			prog := row.Node.RollupProgressString()
			assignee := row.Node.Record.AssignedToName
			if assignee == "" {
				if row.Node.Record.WorkItemType == "Query" || row.Node.IsQuery {
					assignee = ""
				} else {
					assignee = "Unassigned"
				}
			}

			if row.Node.Record.WorkItemType == "Query" || row.Node.IsQuery {
				fmt.Printf("  %s[%-12s] [%-11s] Est: %-6s | Work: %-15s | %-16s | %s\n",
					row.Prefix,
					"Query",
					row.Node.Record.State,
					origEst,
					prog,
					assignee,
					row.Node.Record.Title,
				)
			} else {
				fmt.Printf("  %s#%-6d [%-12s] [%-11s] Est: %-6s | Work: %-15s | %-16s | %s\n",
					row.Prefix,
					row.Node.Record.ID,
					tui.FormatItemType(row.Node.Record.WorkItemType),
					row.Node.Record.State,
					origEst,
					prog,
					assignee,
					row.Node.Record.Title,
				)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
