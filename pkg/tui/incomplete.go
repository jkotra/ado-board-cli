package tui

import (
	"fmt"
	"strings"
	"time"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
)

// IsTypeApplicable checks if a work item type matches any of the enabled work item types
func IsTypeApplicable(wit string, cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	clean := strings.TrimSpace(wit)
	if clean == "" {
		return false
	}

	if len(cfg.IncompleteTypes) > 0 {
		for _, t := range cfg.IncompleteTypes {
			tClean := strings.TrimSpace(t)
			if strings.EqualFold(tClean, "[all]") || strings.EqualFold(tClean, "all") || strings.EqualFold(tClean, clean) {
				return true
			}
		}
		return false
	}

	// Tasks
	if cfg.IncompleteTypeTasks {
		if strings.EqualFold(clean, "Task") {
			return true
		}
	}

	// Bugs
	if cfg.IncompleteTypeBugs {
		if strings.EqualFold(clean, "Bug") || strings.EqualFold(clean, "Defect") || strings.EqualFold(clean, "Issue") {
			return true
		}
	}

	// PBI (Product Backlog Item / User Story / Requirement)
	if cfg.IncompleteTypePBI {
		if strings.EqualFold(clean, "Product Backlog Item") ||
			strings.EqualFold(clean, "User Story") ||
			strings.EqualFold(clean, "Requirement") ||
			strings.EqualFold(clean, "PBI") {
			return true
		}
	}

	return false
}

// IsStatusApplicable checks if a work item state/status matches any of the enabled work item statuses.
func IsStatusApplicable(state string, cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	clean := strings.TrimSpace(state)
	if clean == "" {
		return true
	}

	if len(cfg.IncompleteStatuses) > 0 {
		for _, s := range cfg.IncompleteStatuses {
			sClean := strings.TrimSpace(s)
			if strings.EqualFold(sClean, "[all]") || strings.EqualFold(sClean, "all") || strings.EqualFold(sClean, clean) {
				return true
			}
		}
		return false
	}

	return true
}

// IsDateStatusApplicable checks if a work item state/status requires Start/Target/Finish dates.
func IsDateStatusApplicable(state string, cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	clean := strings.TrimSpace(state)
	if clean == "" {
		return true
	}

	if len(cfg.IncompleteDateStatuses) > 0 {
		for _, s := range cfg.IncompleteDateStatuses {
			sClean := strings.TrimSpace(s)
			if strings.EqualFold(sClean, "[all]") || strings.EqualFold(sClean, "all") || strings.EqualFold(sClean, clean) {
				return true
			}
		}
		return false
	}

	return true
}

// IsDateOverdue checks if a date string represents a time in the past
func IsDateOverdue(dateStr string, now time.Time) (bool, time.Duration) {
	if strings.TrimSpace(dateStr) == "" {
		return false, 0
	}
	t, ok := ado.ParseDateOrDateTime(dateStr, now)
	if !ok {
		return false, 0
	}
	if t.Before(now) {
		return true, now.Sub(t)
	}
	return false, 0
}

// FormatOverdueSmallText returns a small human-readable overdue tag
func FormatOverdueSmallText(dateStr string, now time.Time) string {
	overdue, diff := IsDateOverdue(dateStr, now)
	if !overdue {
		return ""
	}
	days := int(diff.Hours() / 24)
	if days <= 0 {
		return "⚠️ Overdue today"
	}
	if days == 1 {
		return "⚠️ Overdue by 1d"
	}
	return fmt.Sprintf("⚠️ Overdue by %dd", days)
}

// IsItemRecordDone dynamically checks if a work item record is in a completed or closed state
func IsItemRecordDone(database *db.DB, rec *db.WorkItemRecord) bool {
	if rec == nil {
		return false
	}
	if database != nil {
		return database.IsStateCompleted(rec.Org, rec.Project, rec.WorkItemType, rec.State)
	}
	return IsStateDoneOrClosed(rec.State)
}

// IsItemDoneOrClosed checks if a work item is in a completed or closed state
func IsItemDoneOrClosed(rec *db.WorkItemRecord) bool {
	if rec == nil {
		return false
	}
	return IsStateDoneOrClosed(rec.State)
}

// IsStateDoneOrClosed checks if a state string represents a completed or closed state
func IsStateDoneOrClosed(state string) bool {
	st := strings.ToLower(strings.TrimSpace(state))
	switch st {
	case "done", "closed", "resolved", "removed", "completed", "inactive":
		return true
	}
	return false
}

// IsEffortApplicable checks if the work item type uses Effort (or story points) rather than hourly estimates
func IsEffortApplicable(itemType string) bool {
	clean := strings.ToLower(strings.TrimSpace(itemType))
	switch clean {
	case "task":
		return false
	case "user story", "product backlog item", "pbi", "requirement", "feature", "epic", "initiative":
		return true
	case "bug", "defect", "issue":
		return true
	default:
		return false
	}
}

// IsEstimateApplicable checks if the work item type uses hourly Original Estimate
func IsEstimateApplicable(itemType string) bool {
	clean := strings.ToLower(strings.TrimSpace(itemType))
	switch clean {
	case "task":
		return true
	case "bug", "defect", "issue":
		return true
	case "user story", "product backlog item", "pbi", "requirement", "feature", "epic", "initiative":
		return false
	default:
		return true
	}
}

// IsEffortApplicableWithDB dynamically checks if the work item type supports an Effort / StoryPoints field
func IsEffortApplicableWithDB(rec *db.WorkItemRecord, database *db.DB) bool {
	if rec == nil {
		return false
	}
	if database != nil {
		if hasFields, _ := database.HasWorkItemTypeFields(rec.Org, rec.Project, rec.WorkItemType); hasFields {
			hasEffort, _ := database.TypeHasEffortField(rec.Org, rec.Project, rec.WorkItemType)
			return hasEffort
		}
	}
	return IsEffortApplicable(rec.WorkItemType)
}

// IsEstimateApplicableWithDB dynamically checks if the work item type supports an Original Estimate field
func IsEstimateApplicableWithDB(rec *db.WorkItemRecord, database *db.DB) bool {
	if rec == nil {
		return false
	}
	if database != nil {
		if hasFields, _ := database.HasWorkItemTypeFields(rec.Org, rec.Project, rec.WorkItemType); hasFields {
			hasEst, _ := database.TypeHasEstimateField(rec.Org, rec.Project, rec.WorkItemType)
			return hasEst
		}
	}
	return IsEstimateApplicable(rec.WorkItemType)
}

// IsStartDateApplicableWithDB dynamically checks if the work item type supports a Start Date field
func IsStartDateApplicableWithDB(rec *db.WorkItemRecord, database *db.DB) bool {
	if rec == nil {
		return false
	}
	if database != nil {
		if hasFields, _ := database.HasWorkItemTypeFields(rec.Org, rec.Project, rec.WorkItemType); hasFields {
			hasDate, _ := database.TypeHasStartDateField(rec.Org, rec.Project, rec.WorkItemType)
			return hasDate
		}
	}
	return true
}

// IsTargetDateApplicableWithDB dynamically checks if the work item type supports a Target Date field
func IsTargetDateApplicableWithDB(rec *db.WorkItemRecord, database *db.DB) bool {
	if rec == nil {
		return false
	}
	if database != nil {
		if hasFields, _ := database.HasWorkItemTypeFields(rec.Org, rec.Project, rec.WorkItemType); hasFields {
			hasDate, _ := database.TypeHasTargetDateField(rec.Org, rec.Project, rec.WorkItemType)
			return hasDate
		}
	}
	return true
}

// IsFinishDateApplicableWithDB dynamically checks if the work item type supports a Finish Date field
func IsFinishDateApplicableWithDB(rec *db.WorkItemRecord, database *db.DB) bool {
	if rec == nil {
		return false
	}
	if database != nil {
		if hasFields, _ := database.HasWorkItemTypeFields(rec.Org, rec.Project, rec.WorkItemType); hasFields {
			hasDate, _ := database.TypeHasFinishDateField(rec.Org, rec.Project, rec.WorkItemType)
			return hasDate
		}
	}
	return true
}

// GetIncompleteReasonsWithDB checks a work item against enabled incomplete criteria using dynamic schema information
func GetIncompleteReasonsWithDB(rec *db.WorkItemRecord, cfg *config.Config, database *db.DB, now time.Time) []string {
	if rec == nil || cfg == nil {
		return nil
	}

	if !IsTypeApplicable(rec.WorkItemType, cfg) {
		return nil
	}

	if !IsStatusApplicable(rec.State, cfg) {
		return nil
	}

	var reasons []string

	// Effort check: Only applicable if the item type dynamically supports Effort and doesn't already have an hourly estimate/completed work
	if cfg.IncompleteCriteriaEffort && IsEffortApplicableWithDB(rec, database) {
		if rec.Effort <= 0 && rec.OriginalEstimate <= 0 && rec.CompletedWork <= 0 {
			reasons = append(reasons, "Missing Effort")
		}
	}

	// Estimate check: Only applicable if the item type dynamically supports hourly estimates and doesn't already have effort sizing
	if cfg.IncompleteCriteriaEstimate && IsEstimateApplicableWithDB(rec, database) {
		if rec.OriginalEstimate <= 0 && rec.Effort <= 0 && rec.CompletedWork <= 0 {
			reasons = append(reasons, "Missing Estimate")
		}
	}

	dateApplicable := IsDateStatusApplicable(rec.State, cfg)

	if cfg.IncompleteCriteriaStartDate && dateApplicable && IsStartDateApplicableWithDB(rec, database) {
		if strings.TrimSpace(rec.StartDate) == "" {
			reasons = append(reasons, "Missing Start Date")
		}
	}

	if cfg.IncompleteCriteriaTargetDate && IsTargetDateApplicableWithDB(rec, database) {
		if strings.TrimSpace(rec.TargetDate) == "" {
			if dateApplicable {
				reasons = append(reasons, "Missing Target Date")
			}
		} else if overdue, _ := IsDateOverdue(rec.TargetDate, now); overdue && !IsItemRecordDone(database, rec) {
			reasons = append(reasons, "Target Date Overdue")
		}
	}

	if cfg.IncompleteCriteriaFinishDate && dateApplicable && IsFinishDateApplicableWithDB(rec, database) {
		if strings.TrimSpace(rec.FinishDate) == "" {
			reasons = append(reasons, "Missing Finish Date")
		}
	}

	// Tag Regex check: if required and pattern configured, check if item has a matching tag
	if cfg.IncompleteCriteriaTagRegex && strings.TrimSpace(cfg.TagRegex) != "" {
		if !HasTagMatchingRegex(rec.Tags, cfg.TagRegex) {
			reasons = append(reasons, "Missing Tag (Regex)")
		}
	}

	// Activity check: if required, check if item has Activity specified
	if cfg.IncompleteCriteriaActivity {
		if strings.TrimSpace(rec.Activity) == "" {
			reasons = append(reasons, "Missing Activity")
		}
	}

	return reasons
}

// GetIncompleteReasons checks a work item against enabled incomplete criteria
func GetIncompleteReasons(rec *db.WorkItemRecord, cfg *config.Config, now time.Time) []string {
	return GetIncompleteReasonsWithDB(rec, cfg, nil, now)
}

// IsItemIncompleteWithDB returns true if the work item has any incomplete criteria triggered
func IsItemIncompleteWithDB(rec *db.WorkItemRecord, cfg *config.Config, database *db.DB, now time.Time) bool {
	if rec == nil || cfg == nil {
		return false
	}

	if !IsTypeApplicable(rec.WorkItemType, cfg) {
		return false
	}

	if !IsStatusApplicable(rec.State, cfg) {
		return false
	}

	// Effort check
	if cfg.IncompleteCriteriaEffort && IsEffortApplicableWithDB(rec, database) {
		if rec.Effort <= 0 && rec.OriginalEstimate <= 0 && rec.CompletedWork <= 0 {
			return true
		}
	}

	// Estimate check
	if cfg.IncompleteCriteriaEstimate && IsEstimateApplicableWithDB(rec, database) {
		if rec.OriginalEstimate <= 0 && rec.Effort <= 0 && rec.CompletedWork <= 0 {
			return true
		}
	}

	dateApplicable := IsDateStatusApplicable(rec.State, cfg)

	if cfg.IncompleteCriteriaStartDate && dateApplicable && IsStartDateApplicableWithDB(rec, database) {
		if strings.TrimSpace(rec.StartDate) == "" {
			return true
		}
	}

	if cfg.IncompleteCriteriaTargetDate && IsTargetDateApplicableWithDB(rec, database) {
		if strings.TrimSpace(rec.TargetDate) == "" {
			if dateApplicable {
				return true
			}
		} else if overdue, _ := IsDateOverdue(rec.TargetDate, now); overdue && !IsItemRecordDone(database, rec) {
			return true
		}
	}

	if cfg.IncompleteCriteriaFinishDate && dateApplicable && IsFinishDateApplicableWithDB(rec, database) {
		if strings.TrimSpace(rec.FinishDate) == "" {
			return true
		}
	}

	// Tag Regex check
	if cfg.IncompleteCriteriaTagRegex && strings.TrimSpace(cfg.TagRegex) != "" {
		if !HasTagMatchingRegex(rec.Tags, cfg.TagRegex) {
			return true
		}
	}

	// Activity check
	if cfg.IncompleteCriteriaActivity {
		if strings.TrimSpace(rec.Activity) == "" {
			return true
		}
	}

	return false
}

// IsItemIncomplete returns true if the work item has any incomplete criteria triggered
func IsItemIncomplete(rec *db.WorkItemRecord, cfg *config.Config, now time.Time) bool {
	return IsItemIncompleteWithDB(rec, cfg, nil, now)
}

// HasTagMatchingRegex checks if any tag in the semicolon-delimited tags string matches the given regex pattern
func HasTagMatchingRegex(tags, tagRegex string) bool {
	return ado.HasTagMatchingRegex(tags, tagRegex)
}

// FilterInheritedTags filters parent tags to only those matching tagRegex
func FilterInheritedTags(parentTags, tagRegex string) string {
	return ado.FilterInheritedTags(parentTags, tagRegex)
}

