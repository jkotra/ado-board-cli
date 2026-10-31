package cmd

import (
	"fmt"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
	"ado-board-cli/pkg/tui"

	"github.com/spf13/cobra"
)

var trackCmd = &cobra.Command{
	Use:   "track <link-or-id>",
	Short: "Track an Azure DevOps work item link or query and discover all items",
	Long: `Track an Azure DevOps work item by URL/ID or saved query by URL/UUID.
This fetches the work items and their child hierarchies recursively,
stores the data in SQLite, and builds the tree.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		link := args[0]
		cfg, err := config.Load(flagDbPath, flagToken)
		if err != nil {
			return err
		}

		if cfg.PatToken == "" {
			return fmt.Errorf("PAT_TOKEN environment variable or --token flag is required")
		}

		target, err := ado.ParseTrackTarget(link, cfg.DefaultOrg, cfg.DefaultProject)
		if err != nil {
			return err
		}

		fmt.Printf("🔍 Connecting to Azure DevOps (%s)...\n", target.Org)
		client := ado.NewClient(target.Org, target.Project, cfg.PatToken)

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()

		if target.Type == ado.TrackTargetQuery {
			fmt.Printf("📥 Executing query %s and discovering item hierarchy...\n", target.QueryID)
			qInfo, rootIDs, items, err := client.FetchQueryHierarchy(target.QueryID)
			if err != nil {
				return fmt.Errorf("failed to fetch query hierarchy from ADO: %w", err)
			}

			if len(items) == 0 {
				return fmt.Errorf("query returned 0 work items")
			}

			project := target.Project
			if project == "" && len(items) > 0 {
				project = items[0].GetStringField("System.TeamProject")
			}

			// Save tracked query
			if err := database.SaveTrackedQuery(qInfo.ID, qInfo.Name, qInfo.Path, target.Org, project, target.RawURL, qInfo.QueryType); err != nil {
				return fmt.Errorf("failed to save tracked query: %w", err)
			}

			// Save roots linked to query
			for _, rID := range rootIDs {
				_ = database.SaveTrackedRootWithQuery(rID, target.Org, project, "", qInfo.ID)
			}

			// Save all items
			if err := database.SaveWorkItems(target.Org, project, items); err != nil {
				return fmt.Errorf("failed to save work items: %w", err)
			}

			// Update default org/project in settings
			if cfg.DefaultOrg == "" {
				cfg.DefaultOrg = target.Org
				cfg.DefaultProject = project
				_ = database.SetSetting("default_org", target.Org)
				_ = database.SetSetting("default_project", project)
			}

			fmt.Printf("✅ Successfully tracked query %q with %d items across %d root trees!\n\n", qInfo.Name, len(items), len(rootIDs))
		} else {
			fmt.Printf("📥 Discovering work item #%d and child hierarchy...\n", target.ItemID)
			items, err := client.FetchHierarchy(target.ItemID)
			if err != nil {
				return fmt.Errorf("failed to fetch hierarchy from ADO: %w", err)
			}

			if len(items) == 0 {
				return fmt.Errorf("no work items found for ID %d", target.ItemID)
			}

			project := target.Project
			if project == "" && len(items) > 0 {
				project = items[0].GetStringField("System.TeamProject")
			}

			// Save tracked root
			if err := database.SaveTrackedRoot(target.ItemID, target.Org, project, target.RawURL); err != nil {
				return fmt.Errorf("failed to save tracked root: %w", err)
			}

			// Save all items
			if err := database.SaveWorkItems(target.Org, project, items); err != nil {
				return fmt.Errorf("failed to save work items: %w", err)
			}

			// Update default org/project in settings
			if cfg.DefaultOrg == "" {
				cfg.DefaultOrg = target.Org
				cfg.DefaultProject = project
				_ = database.SetSetting("default_org", target.Org)
				_ = database.SetSetting("default_project", project)
			}

			fmt.Printf("✅ Successfully tracked #%d (%s) with %d items in tree!\n\n", target.ItemID, items[0].Title(), len(items))
		}

		// Print tree preview
		allWorkItems, _ := database.GetAllWorkItems()
		roots, _ := database.GetTrackedRoots()
		queries, _ := database.GetTrackedQueries()
		tree := db.BuildTree(allWorkItems, roots, queries)
		flattened := db.FlattenTree(tree, "")

		fmt.Println("Hierarchy Tree:")
		for _, row := range flattened {
			origEst := row.Node.EstimateDisplayString()
			prog := row.Node.RollupProgressString()
			if row.Node.Record.WorkItemType == "Query" || row.Node.IsQuery {
				fmt.Printf("  %s[%s] [%s] Est: %s | Prog: %s - %s\n",
					row.Prefix,
					"Query",
					row.Node.Record.State,
					origEst,
					prog,
					row.Node.Record.Title,
				)
			} else {
				fmt.Printf("  %s#%d [%s] [%s] Est: %s | Prog: %s - %s\n",
					row.Prefix,
					row.Node.Record.ID,
					tui.FormatItemType(row.Node.Record.WorkItemType),
					row.Node.Record.State,
					origEst,
					prog,
					row.Node.Record.Title,
				)
			}
		}

		fmt.Println("\nRun 'ado-board-cli' to view and manage in interactive TUI.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(trackCmd)
}
