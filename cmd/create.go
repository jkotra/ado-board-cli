package cmd

import (
	"fmt"
	"time"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/spf13/cobra"
)

var (
	createType       string
	createTitle      string
	createDesc       string
	createTags       string
	createIteration  string
	createArea       string
	createAssigned   string
	createActivity   string
	createEstimate   float64
	createRemain     float64
	createParent     int
	createStartDate  string
	createTargetDate string
	createFinishDate string
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new Azure DevOps work item (Task, Bug, User Story, etc.)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(flagDbPath, flagToken)
		if err != nil {
			return err
		}

		if cfg.PatToken == "" {
			return fmt.Errorf("PAT_TOKEN environment variable or --token flag is required")
		}

		if createTitle == "" {
			return fmt.Errorf("--title is required")
		}
		if createType == "" {
			createType = "Task"
		}

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return err
		}
		defer database.Close()

		org := cfg.DefaultOrg
		project := cfg.DefaultProject

		if createParent > 0 {
			if parent, err := database.GetWorkItem(createParent); err == nil {
				if org == "" {
					org = parent.Org
				}
				if project == "" {
					project = parent.Project
				}
			}
		}

		if org == "" || project == "" {
			return fmt.Errorf("organization and project must be specified or configured via tracked items")
		}

		fields := map[string]interface{}{
			"System.Title": createTitle,
		}
		if createParent > 0 {
			if iter := database.GetNearestParentIteration(createParent); iter != "" {
				fields["System.IterationPath"] = iter
			}
			if area := database.GetNearestParentArea(createParent); area != "" {
				fields["System.AreaPath"] = area
			}
		}
		if createIteration != "" {
			fields["System.IterationPath"] = createIteration
		}
		if createArea != "" {
			fields["System.AreaPath"] = createArea
		}
		if createTags != "" {
			fields["System.Tags"] = createTags
		} else if createParent > 0 {
			if parent, err := database.GetWorkItem(createParent); err == nil && parent != nil && parent.Tags != "" {
				tagRegex, _ := database.GetSetting("tag_regex")
				if tagRegex == "" {
					tagRegex = cfg.TagRegex
				}
				if inherited := ado.FilterInheritedTags(parent.Tags, tagRegex); inherited != "" {
					fields["System.Tags"] = inherited
				}
			}
		}
		if createDesc != "" {
			fields["System.Description"] = createDesc
		}
		if createAssigned != "" {
			fields["System.AssignedTo"] = createAssigned
		}
		if createActivity != "" {
			fields["Microsoft.VSTS.Common.Activity"] = createActivity
		}
		if createEstimate > 0 {
			fields["Microsoft.VSTS.Scheduling.OriginalEstimate"] = createEstimate
		}
		if createRemain > 0 {
			fields["Microsoft.VSTS.Scheduling.RemainingWork"] = createRemain
		}
		if createStartDate != "" {
			if parsed, ok := ado.ParseDateOrDateTime(createStartDate, time.Now()); ok {
				fields["Microsoft.VSTS.Scheduling.StartDate"] = ado.FormatAdoDateTime(parsed)
			} else {
				fields["Microsoft.VSTS.Scheduling.StartDate"] = createStartDate
			}
		}
		if createTargetDate != "" {
			if parsed, ok := ado.ParseDateOrDateTime(createTargetDate, time.Now()); ok {
				fields["Microsoft.VSTS.Scheduling.TargetDate"] = ado.FormatAdoDateTime(parsed)
			} else {
				fields["Microsoft.VSTS.Scheduling.TargetDate"] = createTargetDate
			}
		}
		if createFinishDate != "" {
			if parsed, ok := ado.ParseDateOrDateTime(createFinishDate, time.Now()); ok {
				fields["Microsoft.VSTS.Scheduling.FinishDate"] = ado.FormatAdoDateTime(parsed)
			} else {
				fields["Microsoft.VSTS.Scheduling.FinishDate"] = createFinishDate
			}
		}

		fmt.Printf("🚀 Creating %s in %s/%s...\n", createType, org, project)
		client := ado.NewClient(org, project, cfg.PatToken)
		client.SetIdentityResolver(func(q string) *ado.IdentityRef {
			if idRec, err := database.FindIdentity(q); err == nil && idRec != nil {
				return &ado.IdentityRef{
					ID:          idRec.ID,
					DisplayName: idRec.DisplayName,
					UniqueName:  idRec.Email,
				}
			}
			return nil
		})
		if ids, err := database.GetAllIdentities(); err == nil && len(ids) > 0 {
			refs := make([]*ado.IdentityRef, 0, len(ids))
			for _, idRec := range ids {
				refs = append(refs, &ado.IdentityRef{
					ID:          idRec.ID,
					DisplayName: idRec.DisplayName,
					UniqueName:  idRec.Email,
				})
			}
			client.RegisterIdentities(refs...)
		}
		created, err := client.CreateWorkItem(createType, fields, createParent)
		if err != nil {
			return fmt.Errorf("failed to create work item in ADO: %w", err)
		}

		if err := database.SaveWorkItem(org, project, created); err != nil {
			return fmt.Errorf("failed to save created item in db: %w", err)
		}

		fmt.Printf("✅ Created %s #%d: %s\n", created.Type(), created.ID, created.Title())
		return nil
	},
}

func init() {
	createCmd.Flags().StringVarP(&createType, "type", "t", "Task", "Work item type (Task, Bug, User Story, Technical Enablement)")
	createCmd.Flags().StringVar(&createTitle, "title", "", "Work item title (required)")
	createCmd.Flags().StringVar(&createDesc, "desc", "", "Description / Details")
	createCmd.Flags().StringVar(&createTags, "tags", "", "Tags (semicolon or comma separated)")
	createCmd.Flags().StringVar(&createIteration, "iteration", "", "Iteration path")
	createCmd.Flags().StringVar(&createArea, "area", "", "Area path")
	createCmd.Flags().StringVar(&createAssigned, "assign", "", "Assigned user email or name")
	createCmd.Flags().Float64Var(&createEstimate, "estimate", 0, "Original estimate in hours")
	createCmd.Flags().Float64Var(&createRemain, "remaining", 0, "Remaining work in hours")
	createCmd.Flags().IntVarP(&createParent, "parent", "p", 0, "Parent work item ID")
	createCmd.Flags().StringVar(&createStartDate, "start-date", "", "Start date (e.g. '8/18/2026 12:00 AM', '2026-08-18', 'tomorrow')")
	createCmd.Flags().StringVar(&createTargetDate, "target-date", "", "Target date (e.g. '8/19/2026 8:01 AM', '2026-08-19')")
	createCmd.Flags().StringVar(&createFinishDate, "finish-date", "", "Finish date (e.g. '8/21/2026 3:57 PM', '2026-08-21')")
	createCmd.Flags().StringVar(&createActivity, "activity", "", "Activity (e.g. Development, Testing, Design, Documentation, Requirements, Deployment)")

	rootCmd.AddCommand(createCmd)
}
