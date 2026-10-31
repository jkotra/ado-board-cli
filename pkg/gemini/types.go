package gemini

import "ado-board-cli/pkg/db"

// ItemSummary captures a simplified, clean view of a work item for AI context
type ItemSummary struct {
	ID               int     `json:"id"`
	Type             string  `json:"type"`
	Title            string  `json:"title"`
	State            string  `json:"state"`
	AssignedTo       string  `json:"assigned_to,omitempty"`
	AssignedEmail    string  `json:"assigned_email,omitempty"`
	OriginalEstimate float64 `json:"original_estimate,omitempty"`
	RemainingWork    float64 `json:"remaining_work,omitempty"`
	CompletedWork    float64 `json:"completed_work,omitempty"`
	Effort           float64 `json:"effort,omitempty"`
	ParentID         int     `json:"parent_id,omitempty"`
	AreaPath         string  `json:"area_path,omitempty"`
	IterationPath    string  `json:"iteration_path,omitempty"`
	Tags             string  `json:"tags,omitempty"`
	Activity         string  `json:"activity,omitempty"`
}

// IdentitySummary captures team member info for identity resolution
type IdentitySummary struct {
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

// SettingsSummary captures current app configuration for AI context
type SettingsSummary struct {
	AutoSyncIntervalSeconds      int      `json:"auto_sync_interval_seconds"`
	AutoCalcRemaining            bool     `json:"auto_calc_remaining"`
	EffortSameAsEstimate         bool     `json:"effort_same_as_estimate"`
	AutoSetFinishDate            bool     `json:"auto_set_finish_date"`
	IncompleteCriteriaEffort     bool     `json:"incomplete_criteria_effort"`
	IncompleteCriteriaEstimate   bool     `json:"incomplete_criteria_estimate"`
	IncompleteCriteriaStartDate  bool     `json:"incomplete_criteria_start_date"`
	IncompleteCriteriaTargetDate bool     `json:"incomplete_criteria_target_date"`
	IncompleteCriteriaFinishDate bool     `json:"incomplete_criteria_finish_date"`
	IncompleteCriteriaTagRegex   bool     `json:"incomplete_criteria_tag_regex"`
	IncompleteCriteriaActivity   bool     `json:"incomplete_criteria_activity"`
	TagRegex                     string   `json:"tag_regex,omitempty"`
	TagRegexes                   []string `json:"tag_regexes,omitempty"`
	IncompleteTypes              []string `json:"incomplete_types"`
	IncompleteStatuses           []string `json:"incomplete_statuses"`
	IncompleteDateStatuses       []string `json:"incomplete_date_statuses"`
	GeminiModel                  string   `json:"gemini_model"`
	OpenInBrowserCmd             string   `json:"open_in_browser_cmd,omitempty"`
}

// ContextPayload represents the complete UI and data context at the moment of asking
type ContextPayload struct {
	ActiveScreen     string            `json:"active_screen"` // "tree_view" or "item_details"
	CurrentTime      string            `json:"current_time,omitempty"`     // e.g. "2026-08-21 23:33:15"
	UserTimezone     string            `json:"user_timezone,omitempty"`    // e.g. "IST (UTC+05:30)"
	SelectedItem     *ItemSummary      `json:"selected_item,omitempty"`
	ParentItem       *ItemSummary      `json:"parent_item,omitempty"`
	ChildItems       []*ItemSummary    `json:"child_items,omitempty"`
	SiblingItems     []*ItemSummary    `json:"sibling_items,omitempty"`
	CachedIdentities []IdentitySummary `json:"cached_identities,omitempty"`
	Org              string            `json:"org,omitempty"`
	Project          string            `json:"project,omitempty"`
	CurrentSettings  *SettingsSummary  `json:"current_settings,omitempty"`
}

// ProgressCallback receives real-time progress strings while Gemini executes tools
type ProgressCallback func(step string)

// ConfirmPrompt contains details about a potentially destructive action requiring user confirmation
type ConfirmPrompt struct {
	Action      string // e.g. "delete_work_item"
	WorkItemID  int
	Title       string
	Description string
}

// ConfirmCallback prompts the user/TUI to approve or reject a destructive action
type ConfirmCallback func(prompt ConfirmPrompt) bool

// ExecutionResult holds the final assistant answer and list of modified item IDs
type ExecutionResult struct {
	Response        string
	AffectedIDs     []int
	ToolExecLogs    []string
	SettingsUpdated bool
	Err             error
}

// RecordToSummary converts a DB WorkItemRecord to an ItemSummary
func RecordToSummary(r *db.WorkItemRecord) *ItemSummary {
	if r == nil {
		return nil
	}
	return &ItemSummary{
		ID:               r.ID,
		Type:             r.WorkItemType,
		Title:            r.Title,
		State:            r.State,
		AssignedTo:       r.AssignedToName,
		AssignedEmail:    r.AssignedToEmail,
		OriginalEstimate: r.OriginalEstimate,
		RemainingWork:    r.RemainingWork,
		CompletedWork:    r.CompletedWork,
		Effort:           r.Effort,
		ParentID:         r.ParentID,
		AreaPath:         r.AreaPath,
		IterationPath:    r.IterationPath,
		Tags:             r.Tags,
		Activity:         r.Activity,
	}
}
