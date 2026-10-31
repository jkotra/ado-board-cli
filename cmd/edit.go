package cmd

import (
	"fmt"
	"strconv"
	"time"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/spf13/cobra"
)

var (
	editTitle          string
	editDesc           string
	editTags           string
	editIteration      string
	editArea           string
	editEstimate       float64
	editRemaining      float64
	editCompleted      float64
	editEffort         float64
	editStartDate      string
	editTargetDate     string
	editFinishDate     string
	editActivity       string
	clearStartDate     bool
	clearTargetDate    bool
	clearFinishDate    bool
	clearActivity      bool
)

var editCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit description, estimates, completed work, effort, dates, tags, iteration, area, activity, or title of a work item",
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

		fields := map[string]interface{}{}
		if editTitle != "" {
			fields["System.Title"] = editTitle
		}
		if editDesc != "" {
			fields["System.Description"] = editDesc
		}
		if cmd.Flags().Changed("tags") {
			fields["System.Tags"] = editTags
		}
		if cmd.Flags().Changed("iteration") {
			fields["System.IterationPath"] = editIteration
		}
		if cmd.Flags().Changed("area") {
			fields["System.AreaPath"] = editArea
		}
		effField := "Microsoft.VSTS.Scheduling.Effort"
		origField := "Microsoft.VSTS.Scheduling.OriginalEstimate"
		remField := "Microsoft.VSTS.Scheduling.RemainingWork"
		compField := "Microsoft.VSTS.Scheduling.CompletedWork"
		startField := "Microsoft.VSTS.Scheduling.StartDate"
		targetField := "Microsoft.VSTS.Scheduling.TargetDate"
		finishField := "Microsoft.VSTS.Scheduling.FinishDate"

		if database != nil && item != nil {
			if ref, ok := database.TypeGetEffortField(item.Org, item.Project, item.WorkItemType); ok {
				effField = ref
			}
			if ref, ok := database.TypeGetEstimateField(item.Org, item.Project, item.WorkItemType); ok {
				origField = ref
			}
			if ref, ok := database.TypeGetRemainingWorkField(item.Org, item.Project, item.WorkItemType); ok {
				remField = ref
			}
			if ref, ok := database.TypeGetCompletedWorkField(item.Org, item.Project, item.WorkItemType); ok {
				compField = ref
			}
			if ref, ok := database.TypeGetStartDateField(item.Org, item.Project, item.WorkItemType); ok {
				startField = ref
			}
			if ref, ok := database.TypeGetTargetDateField(item.Org, item.Project, item.WorkItemType); ok {
				targetField = ref
			}
			if ref, ok := database.TypeGetFinishDateField(item.Org, item.Project, item.WorkItemType); ok {
				finishField = ref
			}
		}

		if cmd.Flags().Changed("estimate") {
			fields[origField] = editEstimate
		}
		if cmd.Flags().Changed("remaining") {
			fields[remField] = editRemaining
		}
		if cmd.Flags().Changed("completed") {
			fields[compField] = editCompleted
		}
		if cmd.Flags().Changed("effort") {
			fields[effField] = editEffort
		}
		if clearStartDate {
			fields[startField] = ""
		} else if cmd.Flags().Changed("start-date") {
			if parsed, ok := ado.ParseDateOrDateTime(editStartDate, time.Now()); ok {
				fields[startField] = ado.FormatAdoDateTime(parsed)
			} else {
				fields[startField] = editStartDate
			}
		}
		if clearTargetDate {
			fields[targetField] = ""
		} else if cmd.Flags().Changed("target-date") {
			if parsed, ok := ado.ParseDateOrDateTime(editTargetDate, time.Now()); ok {
				fields[targetField] = ado.FormatAdoDateTime(parsed)
			} else {
				fields[targetField] = editTargetDate
			}
		}
		if clearFinishDate {
			fields[finishField] = ""
		} else if cmd.Flags().Changed("finish-date") {
			if parsed, ok := ado.ParseDateOrDateTime(editFinishDate, time.Now()); ok {
				fields[finishField] = ado.FormatAdoDateTime(parsed)
			} else {
				fields[finishField] = editFinishDate
			}
		}
		if clearActivity {
			fields["Microsoft.VSTS.Common.Activity"] = ""
		} else if cmd.Flags().Changed("activity") {
			fields["Microsoft.VSTS.Common.Activity"] = editActivity
		}

		if len(fields) == 0 {
			return fmt.Errorf("no fields specified to edit (use --title, --desc, --tags, --iteration, --area, --estimate, --remaining, --completed, --effort, --start-date, --target-date, --finish-date, or --activity)")
		}

		client := ado.NewClient(item.Org, item.Project, cfg.PatToken)
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
		updated, err := client.UpdateWorkItemFields(id, fields)
		if err != nil {
			return fmt.Errorf("failed to update in ADO: %w", err)
		}

		if err := database.SaveWorkItem(item.Org, item.Project, updated); err != nil {
			return err
		}

		fmt.Printf("✅ Updated work item #%d successfully.\n", id)
		return nil
	},
}

func init() {
	editCmd.Flags().StringVar(&editTitle, "title", "", "Updated title")
	editCmd.Flags().StringVar(&editDesc, "desc", "", "Updated description")
	editCmd.Flags().StringVar(&editTags, "tags", "", "Updated tags (semicolon or comma separated)")
	editCmd.Flags().StringVar(&editIteration, "iteration", "", "Updated iteration path")
	editCmd.Flags().StringVar(&editArea, "area", "", "Updated area path")
	editCmd.Flags().Float64Var(&editEstimate, "estimate", 0, "Original estimate in hours")
	editCmd.Flags().Float64Var(&editRemaining, "remaining", 0, "Remaining work in hours")
	editCmd.Flags().Float64Var(&editCompleted, "completed", 0, "Completed work in hours")
	editCmd.Flags().Float64Var(&editEffort, "effort", 0, "Effort / Story points")
	editCmd.Flags().StringVar(&editStartDate, "start-date", "", "Start date (e.g. '8/18/2026 12:00 AM', '2026-08-18')")
	editCmd.Flags().StringVar(&editTargetDate, "target-date", "", "Target date (e.g. '8/19/2026 8:01 AM', '2026-08-19')")
	editCmd.Flags().StringVar(&editFinishDate, "finish-date", "", "Finish date (e.g. '8/21/2026 3:57 PM', '2026-08-21')")
	editCmd.Flags().StringVar(&editActivity, "activity", "", "Activity (e.g. Development, Testing, Design, Documentation, Requirements, Deployment)")
	editCmd.Flags().BoolVar(&clearStartDate, "clear-start-date", false, "Clear the start date")
	editCmd.Flags().BoolVar(&clearTargetDate, "clear-target-date", false, "Clear the target date")
	editCmd.Flags().BoolVar(&clearFinishDate, "clear-finish-date", false, "Clear the finish date")
	editCmd.Flags().BoolVar(&clearActivity, "clear-activity", false, "Clear the activity field")

	rootCmd.AddCommand(editCmd)
}
