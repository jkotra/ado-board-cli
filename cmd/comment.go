package cmd

import (
	"fmt"
	"strconv"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/spf13/cobra"
)

var commentCmd = &cobra.Command{
	Use:   "comment <id> <comment-text>",
	Short: "Add a comment to an Azure DevOps work item",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid work item ID: %s", args[0])
		}
		text := args[1]

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
		comment, err := client.AddComment(id, text)
		if err != nil {
			return fmt.Errorf("failed to add comment in ADO: %w", err)
		}

		_ = database.SaveComments(id, []*ado.CommentItem{comment})

		fmt.Printf("✅ Comment added to #%d by %s\n", id, comment.CreatedBy.DisplayName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(commentCmd)
}
