package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	tea "github.com/charmbracelet/bubbletea"
)

func TestIncompleteTypeApplicable(t *testing.T) {
	cfg := &config.Config{
		IncompleteTypeTasks: true,
		IncompleteTypeBugs:  true,
		IncompleteTypePBI:   true,
	}

	if !IsTypeApplicable("Task", cfg) {
		t.Errorf("expected Task to be applicable")
	}
	if !IsTypeApplicable("Bug", cfg) {
		t.Errorf("expected Bug to be applicable")
	}
	if !IsTypeApplicable("Defect", cfg) {
		t.Errorf("expected Defect to be applicable")
	}
	if !IsTypeApplicable("Product Backlog Item", cfg) {
		t.Errorf("expected Product Backlog Item to be applicable")
	}
	if !IsTypeApplicable("User Story", cfg) {
		t.Errorf("expected User Story to be applicable")
	}
	if !IsTypeApplicable("Requirement", cfg) {
		t.Errorf("expected Requirement to be applicable")
	}
	if IsTypeApplicable("Feature", cfg) {
		t.Errorf("expected Feature not to be applicable")
	}
	if IsTypeApplicable("Epic", cfg) {
		t.Errorf("expected Epic not to be applicable")
	}

	// Disable Tasks
	cfg.IncompleteTypeTasks = false
	if IsTypeApplicable("Task", cfg) {
		t.Errorf("expected Task to not be applicable when IncompleteTypeTasks is false")
	}
}

func TestIncompleteDateOverdue(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	// Empty dates are not overdue (empty is handled as missing date if criteria is checked)
	overdue, _ := IsDateOverdue("", now)
	if overdue {
		t.Errorf("expected empty date not to be overdue")
	}

	// Future date
	futureDate := "2026-08-25T12:00:00Z"
	overdue, _ = IsDateOverdue(futureDate, now)
	if overdue {
		t.Errorf("expected future date not to be overdue")
	}
	if text := FormatOverdueSmallText(futureDate, now); text != "" {
		t.Errorf("expected empty string for future date, got %q", text)
	}

	// Past date (2 days overdue)
	pastDate := "2026-08-20T12:00:00Z"
	overdue, dur := IsDateOverdue(pastDate, now)
	if !overdue {
		t.Errorf("expected past date to be overdue")
	}
	if dur < 47*time.Hour || dur > 49*time.Hour {
		t.Errorf("expected ~48h duration, got %v", dur)
	}
	text := FormatOverdueSmallText(pastDate, now)
	if !strings.Contains(text, "Overdue") || !strings.Contains(text, "2d") {
		t.Errorf("expected overdue text with 2d, got %q", text)
	}
}

func TestIncompleteCriteriaEvaluation(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	cfg := &config.Config{
		IncompleteCriteriaEffort:     true,
		IncompleteCriteriaEstimate:   true,
		IncompleteCriteriaStartDate:  true,
		IncompleteCriteriaTargetDate: true,
		IncompleteCriteriaFinishDate: true,
		IncompleteTypeTasks:          true,
		IncompleteTypeBugs:           true,
		IncompleteTypePBI:            true,
	}

	// Complete Task item with StartDate in the past (normal for in-progress items)
	completeTask := &db.WorkItemRecord{
		ID:               101,
		Title:            "Complete Task",
		WorkItemType:     "Task",
		OriginalEstimate: 4.0,
		Effort:           4.0,
		StartDate:        "2026-08-15T09:00:00Z", // Past start date is normal, not overdue
		TargetDate:       "2026-08-26T17:00:00Z",
		FinishDate:       "2026-08-27T17:00:00Z",
	}
	if IsItemIncomplete(completeTask, cfg, now) {
		t.Errorf("expected completeTask not to be incomplete")
	}
	if reasons := GetIncompleteReasons(completeTask, cfg, now); len(reasons) != 0 {
		t.Errorf("expected 0 reasons, got %v", reasons)
	}

	// Incomplete Task item: missing estimate, target date overdue, missing start date
	incompleteTask := &db.WorkItemRecord{
		ID:               102,
		Title:            "Incomplete Task",
		WorkItemType:     "Task",
		OriginalEstimate: 0.0,                    // Missing estimate
		Effort:           0.0,
		StartDate:        "",                     // Missing start date
		TargetDate:       "2026-08-19T09:00:00Z", // Target date overdue
		FinishDate:       "2026-08-30T17:00:00Z",
	}
	if !IsItemIncomplete(incompleteTask, cfg, now) {
		t.Errorf("expected incompleteTask to be incomplete")
	}
	reasons := GetIncompleteReasons(incompleteTask, cfg, now)
	reasonsStr := strings.Join(reasons, ", ")
	if !strings.Contains(reasonsStr, "Missing Estimate") {
		t.Errorf("expected reasons to contain 'Missing Estimate', got %v", reasons)
	}
	if strings.Contains(reasonsStr, "Missing Effort") {
		t.Errorf("expected reasons NOT to contain 'Missing Effort' for Task, got %v", reasons)
	}
	if !strings.Contains(reasonsStr, "Missing Start Date") {
		t.Errorf("expected reasons to contain 'Missing Start Date', got %v", reasons)
	}
	if strings.Contains(reasonsStr, "Start Date Overdue") {
		t.Errorf("expected reasons NOT to contain 'Start Date Overdue', got %v", reasons)
	}
	if !strings.Contains(reasonsStr, "Target Date Overdue") {
		t.Errorf("expected reasons to contain 'Target Date Overdue', got %v", reasons)
	}

	// Bug with Original Estimate & Completed Work but Effort 0 (Bug #663964 scenario)
	completedBug := &db.WorkItemRecord{
		ID:               663964,
		Title:            "Cannot Deploy GKE Cluster",
		WorkItemType:     "Bug",
		State:            "Done",
		OriginalEstimate: 4.0,
		CompletedWork:    4.0,
		RemainingWork:    0.0,
		Effort:           0.0,
		StartDate:        "2026-08-20T03:30:00Z",
		TargetDate:       "2026-08-22T03:30:00Z",
		FinishDate:       "2026-08-22T03:30:00Z",
	}
	if IsItemIncomplete(completedBug, cfg, now) {
		t.Errorf("expected completedBug not to be incomplete, reasons: %v", GetIncompleteReasons(completedBug, cfg, now))
	}

	// Type not applicable (e.g. Feature)
	feature := &db.WorkItemRecord{
		ID:           103,
		Title:        "Feature Item",
		WorkItemType: "Feature",
	}
	if IsItemIncomplete(feature, cfg, now) {
		t.Errorf("expected Feature not to be flagged incomplete since type is not enabled")
	}
}

func TestTreeIncompleteViewToggleAndRendering(t *testing.T) {
	cfg := &config.Config{
		IncompleteCriteriaEffort:     true,
		IncompleteCriteriaTargetDate: true,
		IncompleteTypeTasks:          true,
	}

	tree := NewScreenTree()
	tree.Config = cfg

	rec1 := &db.WorkItemRecord{
		ID:           101,
		Title:        "Valid Item",
		WorkItemType: "Task",
		Effort:       2.0,
		TargetDate:   "2026-08-30T17:00:00Z",
	}
	rec2 := &db.WorkItemRecord{
		ID:           102,
		Title:        "Missing Target Date Item",
		WorkItemType: "Task",
		Effort:       2.0,
		TargetDate:   "",
	}

	node1 := &db.TreeNode{Record: rec1}
	node2 := &db.TreeNode{Record: rec2}
	tree.SetData([]*db.TreeNode{node1, node2})

	// When IncompleteView is false (default)
	viewOff := tree.View(120, 30, "", false)
	if strings.Contains(viewOff, "INCOMPLETE VIEW") {
		t.Errorf("expected no INCOMPLETE VIEW badge when IncompleteView is false")
	}
	if strings.Contains(viewOff, "⚠️") {
		t.Errorf("expected no ⚠️ emoji when IncompleteView is false")
	}

	// Toggle IncompleteView ON
	tree.IncompleteView = true
	viewOn := tree.View(120, 30, "", false)
	if !strings.Contains(viewOn, "INCOMPLETE VIEW") {
		t.Errorf("expected INCOMPLETE VIEW badge at right corner when IncompleteView is true")
	}
	if !strings.Contains(viewOn, "⚠️") {
		t.Errorf("expected ⚠️ warning emoji for incomplete item when IncompleteView is true")
	}

	// Verify notice is visible across all root tabs in tree view and height is exactly 30 lines
	linesTab1 := strings.Split(viewOn, "\n")
	if len(linesTab1) != 30 {
		t.Errorf("expected tab 1 view height to be exactly 30 lines, got %d", len(linesTab1))
	}

	// Add Tab 2 with Tech Enablers and nested items
	root2 := &db.TreeNode{
		Record: &db.WorkItemRecord{
			ID:           656842,
			Title:        "GKE & Spot Instances",
			WorkItemType: "Feature",
			State:        "Active",
		},
		Children: []*db.TreeNode{
			{
				Record: &db.WorkItemRecord{
					ID:           659219,
					Title:        "GKE Architecture Analysis",
					WorkItemType: "Tech Enabler",
					State:        "In Progress",
				},
				Children: []*db.TreeNode{
					{
						Record: &db.WorkItemRecord{
							ID:           662396,
							Title:        "Analysis",
							WorkItemType: "Task",
							State:        "To Do",
						},
					},
				},
			},
		},
	}
	tree.SetData([]*db.TreeNode{node1, root2})
	tree.SetActiveTab(1)

	viewTab2 := tree.View(120, 30, "", false)
	if !strings.Contains(viewTab2, "INCOMPLETE VIEW") {
		t.Errorf("expected INCOMPLETE VIEW badge to be visible on Tab 2 as well")
	}

	linesTab2 := strings.Split(viewTab2, "\n")
	if len(linesTab2) != 30 {
		t.Errorf("expected tab 2 view height to be exactly 30 lines, got %d", len(linesTab2))
	}
}

func TestScreenItemOverdueAndIncompleteNotice(t *testing.T) {
	cfg := &config.Config{
		IncompleteCriteriaEstimate:   true,
		IncompleteCriteriaTargetDate: true,
		IncompleteTypeTasks:          true,
	}

	itemScreen := NewScreenItem()
	itemScreen.Config = cfg

	pastDate := "2026-08-20T12:00:00Z"
	rec := &db.WorkItemRecord{
		ID:               102,
		Title:            "Overdue Task",
		WorkItemType:     "Task",
		OriginalEstimate: 0.0,      // missing estimate
		Effort:           0.0,
		StartDate:        pastDate, // past start date should not trigger overdue
		TargetDate:       pastDate, // past target date triggers overdue
	}

	itemScreen.SetData(rec, nil, nil, nil)
	viewOff := itemScreen.View(120, 40, "", false)

	// Verify Target overdue small text under date fields is rendered, but not Start
	if !strings.Contains(viewOff, "Overdue") {
		t.Errorf("expected view to contain Target overdue note under date fields")
	}
	if strings.Contains(viewOff, "Start: ⚠️ Overdue") || strings.Contains(viewOff, "Start Date Overdue") {
		t.Errorf("expected view not to contain Start Date overdue warning")
	}

	// When IncompleteView is false, incomplete notice should NOT be shown
	if strings.Contains(viewOff, "Incomplete:") || strings.Contains(viewOff, "Missing Estimate") {
		t.Errorf("expected view without IncompleteView mode not to show Incomplete notice")
	}

	// When IncompleteView is true, bold red incomplete notice IS rendered
	itemScreen.IncompleteView = true
	viewOn := itemScreen.View(120, 40, "", false)
	if !strings.Contains(viewOn, "Incomplete:") || !strings.Contains(viewOn, "Missing Estimate") {
		t.Errorf("expected view with IncompleteView mode to contain Incomplete notice with missing fields")
	}
}

func TestScreenItemOverdueWarningAlignment(t *testing.T) {
	itemScreen := NewScreenItem()
	now := time.Now()
	pastDate := now.Add(-48 * time.Hour) // 2 days ago

	rec := &db.WorkItemRecord{
		ID:           666641,
		Title:        "Bug with Overdue Target Date",
		WorkItemType: "Bug",
		StartDate:    pastDate.Format(time.RFC3339),
		TargetDate:   pastDate.Format(time.RFC3339),
		FinishDate:   "",
	}

	itemScreen.SetData(rec, nil, nil, nil)
	view := itemScreen.View(120, 40, "", false)

	// Extract the lines
	lines := strings.Split(view, "\n")
	var dateLineIdx = -1
	var overdueLineIdx = -1
	for i, l := range lines {
		if strings.Contains(l, "Start Date:") && strings.Contains(l, "Target Date:") {
			dateLineIdx = i
		}
		if strings.Contains(l, "Overdue") {
			overdueLineIdx = i
		}
	}

	if dateLineIdx == -1 {
		t.Fatalf("could not find dateLine in view:\n%s", view)
	}
	if overdueLineIdx == -1 || overdueLineIdx != dateLineIdx+1 {
		t.Fatalf("expected overdueLine immediately after dateLine, got dateLineIdx=%d, overdueLineIdx=%d", dateLineIdx, overdueLineIdx)
	}

	dateLineRaw := StripEscapes(lines[dateLineIdx])
	overdueLineRaw := StripEscapes(lines[overdueLineIdx])

	targetDateColPos := -1
	if byteIdx := strings.Index(dateLineRaw, "Target Date:"); byteIdx != -1 {
		targetDateColPos = utf8.RuneCountInString(dateLineRaw[:byteIdx])
	}

	overdueIndicatorPos := -1
	if byteIdx := strings.Index(overdueLineRaw, "↳"); byteIdx != -1 {
		overdueIndicatorPos = utf8.RuneCountInString(overdueLineRaw[:byteIdx])
	}

	if targetDateColPos == -1 {
		t.Fatalf("could not find 'Target Date:' in raw line: %q", dateLineRaw)
	}
	if overdueIndicatorPos == -1 {
		t.Fatalf("could not find '↳' in raw overdue line: %q", overdueLineRaw)
	}

	if overdueIndicatorPos != targetDateColPos {
		t.Errorf("expected overdue indicator '↳' to align at character column %d (under 'Target Date:'), but got column %d\nDate line:    %s\nOverdue line: %s",
			targetDateColPos, overdueIndicatorPos, dateLineRaw, overdueLineRaw)
	}
}

func TestScreenItemOverdueNotShownOnCompletedOrFinishDate(t *testing.T) {
	itemScreen := NewScreenItem()
	now := time.Now()
	pastDate := now.Add(-48 * time.Hour).Format(time.RFC3339)

	// Work item in "Done" state with past TargetDate & FinishDate (like work item #663044)
	doneItem := &db.WorkItemRecord{
		ID:           663044,
		Title:        "Raw Manager implementation",
		WorkItemType: "Task",
		State:        "Done",
		StartDate:    pastDate,
		TargetDate:   "",
		FinishDate:   pastDate,
	}

	itemScreen.SetData(doneItem, nil, nil, nil)
	view := itemScreen.View(120, 40, "", false)

	if strings.Contains(view, "Overdue") {
		t.Errorf("expected Done item with past Finish Date not to show overdue warning, got view:\n%s", view)
	}

	// Active work item with past FinishDate (which should never trigger overdue anyway)
	activeItemWithPastFinish := &db.WorkItemRecord{
		ID:           663045,
		Title:        "Active item",
		WorkItemType: "Task",
		State:        "Active",
		StartDate:    pastDate,
		TargetDate:   "",
		FinishDate:   pastDate,
	}

	itemScreen.SetData(activeItemWithPastFinish, nil, nil, nil)
	view2 := itemScreen.View(120, 40, "", false)

	if strings.Contains(view2, "Overdue") {
		t.Errorf("expected item with past Finish Date not to show overdue warning, got view:\n%s", view2)
	}
}

func TestAppIncompleteShortcutAndSettingsPersistence(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "incomplete_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DbPath:   dbPath,
		PatToken: "test-pat",
	}

	app := NewAppModel(cfg, database)

	// Toggle IncompleteView with "I" key
	if app.ScreenTree.IncompleteView {
		t.Fatalf("expected IncompleteView default false")
	}
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	app = updated.(*AppModel)
	if !app.ScreenTree.IncompleteView {
		t.Fatalf("expected IncompleteView to toggle to true on 'I'")
	}
	if app.ToastMessage != "Incomplete View enabled" {
		t.Fatalf("expected toast 'Incomplete View enabled', got %q", app.ToastMessage)
	}

	// Toggle IncompleteView off with "shift+i"
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'I'}})
	app = updated.(*AppModel)
	if app.ScreenTree.IncompleteView {
		t.Fatalf("expected IncompleteView to toggle to false on 'shift+i'")
	}
	if app.ToastMessage != "Incomplete View disabled" {
		t.Fatalf("expected toast 'Incomplete View disabled', got %q", app.ToastMessage)
	}

	// Test saving settings with all criteria checked
	saveCmd := app.saveSettingsCmd("pat123", "gemini123", "gemini-3.7-flash", true, true, true, 120,
		true, true, true, true, true, false, false, "", []string{"Task", "Bug"}, []string{"[All]"}, []string{"[All]"})
	msg := saveCmd()
	updated, _ = app.Update(msg)
	app = updated.(*AppModel)

	// Check DB persistence
	val, err := database.GetSetting("incomplete_criteria_effort")
	if err != nil || val != "true" {
		t.Fatalf("expected incomplete_criteria_effort to be 'true' in db, got %q, err %v", val, err)
	}
	val, err = database.GetSetting("incomplete_types")
	if err != nil || !strings.Contains(val, "Task") {
		t.Fatalf("expected incomplete_types to contain 'Task' in db, got %q, err %v", val, err)
	}
	val, err = database.GetSetting("incomplete_statuses")
	if err != nil || !strings.Contains(val, "[All]") {
		t.Fatalf("expected incomplete_statuses to contain '[All]' in db, got %q, err %v", val, err)
	}
	val, err = database.GetSetting("incomplete_date_statuses")
	if err != nil || !strings.Contains(val, "[All]") {
		t.Fatalf("expected incomplete_date_statuses to contain '[All]' in db, got %q, err %v", val, err)
	}

	// Reload config from DB
	newCfg, err := config.Load(dbPath, "")
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}
	if !newCfg.IncompleteCriteriaEffort || !newCfg.IncompleteCriteriaStartDate || len(newCfg.IncompleteTypes) == 0 {
		t.Fatalf("expected reloaded config to have incomplete criteria enabled")
	}
	if len(newCfg.IncompleteStatuses) == 0 || newCfg.IncompleteStatuses[0] != "[All]" {
		t.Fatalf("expected reloaded config to have incomplete statuses enabled, got %v", newCfg.IncompleteStatuses)
	}
	if len(newCfg.IncompleteDateStatuses) == 0 || newCfg.IncompleteDateStatuses[0] != "[All]" {
		t.Fatalf("expected reloaded config to have incomplete date statuses enabled, got %v", newCfg.IncompleteDateStatuses)
	}
}

func TestIncompleteStatusApplicable(t *testing.T) {
	// 1. Default config: Removed is not in the list, so it is excluded; standard statuses are applicable
	defaultCfg := &config.Config{
		IncompleteStatuses: []string{"New", "Active", "To Do", "In Progress", "Committed"},
	}
	if IsStatusApplicable("Removed", defaultCfg) {
		t.Errorf("expected Removed to be excluded by default")
	}
	if IsStatusApplicable("removed", defaultCfg) {
		t.Errorf("expected removed (lowercase) to be excluded by default")
	}
	if !IsStatusApplicable("Active", defaultCfg) {
		t.Errorf("expected Active to be applicable by default")
	}
	if !IsStatusApplicable("New", defaultCfg) {
		t.Errorf("expected New to be applicable by default")
	}
	if !IsStatusApplicable("To Do", defaultCfg) {
		t.Errorf("expected To Do to be applicable by default")
	}
	if !IsStatusApplicable("In Progress", defaultCfg) {
		t.Errorf("expected In Progress to be applicable by default")
	}

	// 2. Config with literal [All]: matches all statuses including Removed
	allCfg := &config.Config{
		IncompleteStatuses: []string{"[All]"},
	}
	if !IsStatusApplicable("Removed", allCfg) {
		t.Errorf("expected Removed to be included under literal [All]")
	}
	if !IsStatusApplicable("Active", allCfg) {
		t.Errorf("expected Active to be applicable under [All]")
	}
	if !IsStatusApplicable("Done", allCfg) {
		t.Errorf("expected Done to be applicable under [All]")
	}

	// 3. Config with explicit statuses (e.g. Active, In Progress)
	customCfg := &config.Config{
		IncompleteStatuses: []string{"Active", "In Progress"},
	}
	if !IsStatusApplicable("Active", customCfg) {
		t.Errorf("expected Active to be applicable under custom statuses")
	}
	if !IsStatusApplicable("In Progress", customCfg) {
		t.Errorf("expected In Progress to be applicable under custom statuses")
	}
	if IsStatusApplicable("New", customCfg) {
		t.Errorf("expected New to be excluded when not in custom statuses")
	}
	if IsStatusApplicable("Removed", customCfg) {
		t.Errorf("expected Removed to be excluded when not in custom statuses")
	}

	// 4. Config with explicit Removed status
	explicitRemovedCfg := &config.Config{
		IncompleteStatuses: []string{"Removed", "Active"},
	}
	if !IsStatusApplicable("Removed", explicitRemovedCfg) {
		t.Errorf("expected Removed to be applicable when explicitly listed")
	}
}

func TestRemovedStatusExcludedFromIncompleteWarnings(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	cfg := &config.Config{
		IncompleteCriteriaEffort:     true,
		IncompleteCriteriaEstimate:   true,
		IncompleteCriteriaStartDate:  true,
		IncompleteCriteriaTargetDate: true,
		IncompleteCriteriaFinishDate: true,
		IncompleteTypes:              []string{"Task", "Bug"},
		IncompleteStatuses:           []string{"New", "Active", "To Do", "In Progress", "Committed"},
		IncompleteDateStatuses:       []string{"In Progress", "Active", "Committed"},
	}

	// Task 662397 in "Removed" state with missing dates and missing estimates
	removedTask := &db.WorkItemRecord{
		ID:               662397,
		Title:            "Implementation",
		WorkItemType:     "Task",
		State:            "Removed",
		OriginalEstimate: 0,
		Effort:           0,
		StartDate:        "",
		TargetDate:       "",
		FinishDate:       "",
	}

	if IsItemIncomplete(removedTask, cfg, now) {
		t.Errorf("expected Removed item NOT to be flagged incomplete")
	}
	reasons := GetIncompleteReasons(removedTask, cfg, now)
	if len(reasons) != 0 {
		t.Errorf("expected 0 incomplete reasons for Removed item, got %v", reasons)
	}

	// Verify ScreenItem view for Removed item does NOT contain Incomplete warning
	itemScreen := NewScreenItem()
	itemScreen.Config = cfg
	itemScreen.IncompleteView = true
	itemScreen.SetData(removedTask, nil, nil, nil)
	view := itemScreen.View(120, 40, "", false)

	if strings.Contains(view, "Incomplete:") || strings.Contains(view, "Missing Start Date") || strings.Contains(view, "Missing Target Date") {
		t.Errorf("expected ScreenItem view for Removed item NOT to show Incomplete warning banner, got:\n%s", view)
	}

	// Verify ScreenTree view for Removed item does NOT show ⚠️ emoji for the node
	treeScreen := NewScreenTree()
	treeScreen.Config = cfg
	treeScreen.IncompleteView = true
	node := &db.TreeNode{Record: removedTask}
	treeScreen.SetData([]*db.TreeNode{node})
	treeView := treeScreen.View(120, 30, "", false)

	// Incomplete view header badge contains "INCOMPLETE VIEW", but the item node line itself must not have warning glyph
	if strings.Contains(treeView, "⚠️ #662397") || strings.Contains(treeView, "⚠️  #662397") {
		t.Errorf("expected ScreenTree view for Removed item NOT to show ⚠️ emoji for work item #662397, got:\n%s", treeView)
	}
}

func TestIncompleteDateStatusesApplicable(t *testing.T) {
	// Sane default configuration: ["In Progress", "Active", "Committed"]
	cfgDefault := &config.Config{
		IncompleteDateStatuses: []string{"In Progress", "Active", "Committed"},
	}
	backlogStates := []string{"To Do", "todo", "TO DO", "New", "new", "Proposed", "Design", "Planning", "Removed"}
	for _, st := range backlogStates {
		if IsDateStatusApplicable(st, cfgDefault) {
			t.Errorf("expected state %q to NOT be date applicable by default", st)
		}
	}

	execStates := []string{"Active", "active", "In Progress", "in progress", "Committed"}
	for _, st := range execStates {
		if !IsDateStatusApplicable(st, cfgDefault) {
			t.Errorf("expected state %q to be date applicable by default", st)
		}
	}

	// [All] configuration: literally all states are applicable for date checks
	cfgAll := &config.Config{
		IncompleteDateStatuses: []string{"[All]"},
	}
	for _, st := range backlogStates {
		if !IsDateStatusApplicable(st, cfgAll) {
			t.Errorf("expected state %q to be date applicable under literal [All]", st)
		}
	}
	for _, st := range execStates {
		if !IsDateStatusApplicable(st, cfgAll) {
			t.Errorf("expected state %q to be date applicable under literal [All]", st)
		}
	}

	// Custom explicit statuses: if user explicitly configures "To Do" and "Active"
	cfgCustom := &config.Config{
		IncompleteDateStatuses: []string{"To Do", "Active"},
	}
	if !IsDateStatusApplicable("To Do", cfgCustom) {
		t.Errorf("expected 'To Do' to be applicable when explicitly selected")
	}
	if !IsDateStatusApplicable("Active", cfgCustom) {
		t.Errorf("expected 'Active' to be applicable when explicitly selected")
	}
	if IsDateStatusApplicable("In Progress", cfgCustom) {
		t.Errorf("expected 'In Progress' to NOT be applicable when not in custom list")
	}
	if IsDateStatusApplicable("New", cfgCustom) {
		t.Errorf("expected 'New' to NOT be applicable when not in custom list")
	}
}

func TestToDoStatusExcludedFromDateWarnings(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	cfg := &config.Config{
		IncompleteCriteriaEffort:     true,
		IncompleteCriteriaEstimate:   true,
		IncompleteCriteriaStartDate:  true,
		IncompleteCriteriaTargetDate: true,
		IncompleteCriteriaFinishDate: false,
		IncompleteTypes:              []string{"Task", "Bug"},
		IncompleteStatuses:           []string{"New", "Active", "To Do", "In Progress", "Committed"},
		IncompleteDateStatuses:       []string{"In Progress", "Active", "Committed"},
	}

	// Work item #665650: Task in "To Do" state, has effort/estimate, missing start and target dates
	toDoTask := &db.WorkItemRecord{
		ID:               665650,
		Title:            "External Demo (Dev)",
		WorkItemType:     "Task",
		State:            "To Do",
		OriginalEstimate: 1.0,
		RemainingWork:    1.0,
		Effort:           1.0,
		StartDate:        "",
		TargetDate:       "",
		FinishDate:       "",
	}

	// Item should NOT be considered incomplete because dates are not required for "To Do"
	if IsItemIncomplete(toDoTask, cfg, now) {
		t.Errorf("expected Task #665650 with State='To Do' NOT to be flagged incomplete")
	}
	reasons := GetIncompleteReasons(toDoTask, cfg, now)
	if len(reasons) != 0 {
		t.Errorf("expected 0 incomplete reasons for To Do item, got %v", reasons)
	}

	// Verify ScreenItem view for To Do item does NOT contain date warnings
	itemScreen := NewScreenItem()
	itemScreen.Config = cfg
	itemScreen.IncompleteView = true
	itemScreen.SetData(toDoTask, nil, nil, nil)
	view := itemScreen.View(120, 40, "", false)

	if strings.Contains(view, "Missing Start Date") || strings.Contains(view, "Missing Target Date") {
		t.Errorf("expected ScreenItem view for To Do item NOT to show Missing Start/Target Date warnings, got:\n%s", view)
	}

	// If the To Do item is missing Effort/Estimate, those SHOULD still be flagged!
	toDoTaskMissingEffort := &db.WorkItemRecord{
		ID:               665650,
		Title:            "External Demo (Dev)",
		WorkItemType:     "Task",
		State:            "To Do",
		OriginalEstimate: 0,
		RemainingWork:    0,
		Effort:           0,
		StartDate:        "",
		TargetDate:       "",
		FinishDate:       "",
	}

	if !IsItemIncomplete(toDoTaskMissingEffort, cfg, now) {
		t.Errorf("expected To Do item with missing estimate to be incomplete")
	}
	effortReasons := GetIncompleteReasons(toDoTaskMissingEffort, cfg, now)
	for _, r := range effortReasons {
		if r == "Missing Start Date" || r == "Missing Target Date" {
			t.Errorf("unexpected date reason %q on To Do item", r)
		}
	}
	hasEstimate := false
	for _, r := range effortReasons {
		if r == "Missing Estimate" {
			hasEstimate = true
		}
	}
	if !hasEstimate {
		t.Errorf("expected 'Missing Estimate' in reasons, got %v", effortReasons)
	}
}

func TestIncompleteCriteriaSaneDefaults(t *testing.T) {
	// 1. Fresh config defaults
	tmpDb := filepath.Join(t.TempDir(), "sane_defaults.db")
	database, err := db.Open(tmpDb)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	cfg, err := config.Load(tmpDb, "")
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if !cfg.IncompleteCriteriaEffort {
		t.Errorf("expected IncompleteCriteriaEffort default true")
	}
	if !cfg.IncompleteCriteriaEstimate {
		t.Errorf("expected IncompleteCriteriaEstimate default true")
	}
	if !cfg.IncompleteCriteriaStartDate {
		t.Errorf("expected IncompleteCriteriaStartDate default true")
	}
	if !cfg.IncompleteCriteriaTargetDate {
		t.Errorf("expected IncompleteCriteriaTargetDate default true")
	}
	if cfg.IncompleteCriteriaFinishDate {
		t.Errorf("expected IncompleteCriteriaFinishDate default false")
	}
	if len(cfg.IncompleteTypes) != 2 || cfg.IncompleteTypes[0] != "Task" || cfg.IncompleteTypes[1] != "Bug" {
		t.Errorf("expected IncompleteTypes default ['Task', 'Bug'], got %v", cfg.IncompleteTypes)
	}
	if len(cfg.IncompleteStatuses) != 8 || cfg.IncompleteStatuses[0] != "New" || cfg.IncompleteStatuses[5] != "Resolved" {
		t.Errorf("expected IncompleteStatuses default ['New', 'Active', 'To Do', 'In Progress', 'Committed', 'Resolved', 'Done', 'Closed'], got %v", cfg.IncompleteStatuses)
	}
	if len(cfg.IncompleteDateStatuses) != 6 || cfg.IncompleteDateStatuses[0] != "In Progress" || cfg.IncompleteDateStatuses[3] != "Resolved" {
		t.Errorf("expected IncompleteDateStatuses default ['In Progress', 'Active', 'Committed', 'Resolved', 'Done', 'Closed'], got %v", cfg.IncompleteDateStatuses)
	}

	// 2. Settings screen reset reflects sane defaults
	settings := NewScreenSettings()
	settings.Reset(cfg, database)

	_, _, _, _, _, _, _, critEffort, critEstimate, critStart, critTarget, critFinish, _, critActivity, _, incTypes, incStatuses, incDateStatuses := settings.Values()
	if !critEffort || !critEstimate || !critStart || !critTarget || critFinish || critActivity {
		t.Errorf("expected settings values to reflect criteria defaults")
	}
	if len(incTypes) != 2 || incTypes[0] != "Task" || incTypes[1] != "Bug" {
		t.Errorf("expected settings incTypes ['Task', 'Bug'], got %v", incTypes)
	}
	if len(incStatuses) != 8 {
		t.Errorf("expected settings incStatuses 8 items, got %v", incStatuses)
	}
	if len(incDateStatuses) != 6 {
		t.Errorf("expected settings incDateStatuses 6 items, got %v", incDateStatuses)
	}

	// 3. Settings View renders badges
	view := settings.View(100, 80, "", false)
	if !strings.Contains(view, "Applicable Item Types:") || !strings.Contains(view, "Task") || !strings.Contains(view, "Bug") {
		t.Errorf("expected view to render Applicable Item Types badges, got:\n%s", view)
	}
	if !strings.Contains(view, "Applicable Item Statuses:") || !strings.Contains(view, "New") || !strings.Contains(view, "Active") {
		t.Errorf("expected view to render Applicable Item Statuses badges, got:\n%s", view)
	}
	if !strings.Contains(view, "Applicable Date Statuses:") || !strings.Contains(view, "In Progress") {
		t.Errorf("expected view to render Applicable Date Statuses badges, got:\n%s", view)
	}

	// 4. Incomplete evaluation under defaults
	// A. "Removed" is not in IncompleteStatuses -> Not applicable
	if IsStatusApplicable("Removed", cfg) {
		t.Errorf("expected 'Removed' to NOT be applicable under default statuses")
	}
	// B. "To Do" is in IncompleteStatuses -> Applicable for item criteria
	if !IsStatusApplicable("To Do", cfg) {
		t.Errorf("expected 'To Do' to be applicable under default statuses")
	}
	// C. "To Do" is NOT in IncompleteDateStatuses -> Not applicable for date criteria
	if IsDateStatusApplicable("To Do", cfg) {
		t.Errorf("expected 'To Do' to NOT require dates under default date statuses")
	}
	// D. "In Progress", "Resolved", "Done", "Closed" are in IncompleteDateStatuses -> Require dates
	for _, st := range []string{"In Progress", "Active", "Committed", "Resolved", "Done", "Closed"} {
		if !IsStatusApplicable(st, cfg) {
			t.Errorf("expected %q to be in IncompleteStatuses", st)
		}
		if !IsDateStatusApplicable(st, cfg) {
			t.Errorf("expected %q to require dates under default date statuses", st)
		}
	}

	// 5. If [All] is explicitly configured, literal [All] includes everything including Removed
	allCfg := &config.Config{
		IncompleteStatuses:     []string{"[All]"},
		IncompleteDateStatuses: []string{"[All]"},
	}
	if !IsStatusApplicable("Removed", allCfg) {
		t.Errorf("expected literal [All] in IncompleteStatuses to match 'Removed'")
	}
	if !IsDateStatusApplicable("To Do", allCfg) {
		t.Errorf("expected literal [All] in IncompleteDateStatuses to match 'To Do'")
	}
}

func TestDynamicWorkItemTypeFieldsIncompleteCheck(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ado-incomplete-dynamic-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	org := "testorg"
	project := "testproj"

	// Define fields for "CustomTask" -> only has OriginalEstimate, no Effort
	err = database.SaveWorkItemTypeFields(org, project, "CustomTask", []ado.WorkItemTypeField{
		{ReferenceName: "System.Title", Name: "Title"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.OriginalEstimate", Name: "Original Estimate"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.StartDate", Name: "Start Date"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.TargetDate", Name: "Target Date"},
	})
	if err != nil {
		t.Fatalf("failed to save CustomTask fields: %v", err)
	}

	// Define fields for "CustomStory" -> only has Effort, no OriginalEstimate
	err = database.SaveWorkItemTypeFields(org, project, "CustomStory", []ado.WorkItemTypeField{
		{ReferenceName: "System.Title", Name: "Title"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.Effort", Name: "Effort"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.StartDate", Name: "Start Date"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.TargetDate", Name: "Target Date"},
	})
	if err != nil {
		t.Fatalf("failed to save CustomStory fields: %v", err)
	}

	cfg := &config.Config{
		IncompleteCriteriaEffort:     true,
		IncompleteCriteriaEstimate:   true,
		IncompleteCriteriaStartDate:  true,
		IncompleteCriteriaTargetDate: true,
		IncompleteStatuses:           []string{"[All]"},
		IncompleteDateStatuses:       []string{"[All]"},
		IncompleteTypes:              []string{"[All]"},
	}
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	// 1. CustomTask missing estimate
	taskRec := &db.WorkItemRecord{
		Org:              org,
		Project:          project,
		ID:               1001,
		Title:            "Task Missing Estimate",
		WorkItemType:     "CustomTask",
		State:            "Active",
		OriginalEstimate: 0,
		Effort:           0,
		StartDate:        "2026-08-20T10:00:00Z",
		TargetDate:       "2026-08-30T10:00:00Z",
	}

	reasons := GetIncompleteReasonsWithDB(taskRec, cfg, database, now)
	reasonsStr := strings.Join(reasons, ", ")
	if !strings.Contains(reasonsStr, "Missing Estimate") {
		t.Errorf("expected CustomTask to flag Missing Estimate, got: %v", reasons)
	}
	if strings.Contains(reasonsStr, "Missing Effort") {
		t.Errorf("expected CustomTask NOT to flag Missing Effort dynamically, got: %v", reasons)
	}

	// 2. CustomStory missing effort
	storyRec := &db.WorkItemRecord{
		Org:              org,
		Project:          project,
		ID:               1002,
		Title:            "Story Missing Effort",
		WorkItemType:     "CustomStory",
		State:            "Active",
		OriginalEstimate: 0,
		Effort:           0,
		StartDate:        "2026-08-20T10:00:00Z",
		TargetDate:       "2026-08-30T10:00:00Z",
	}

	storyReasons := GetIncompleteReasonsWithDB(storyRec, cfg, database, now)
	storyReasonsStr := strings.Join(storyReasons, ", ")
	if !strings.Contains(storyReasonsStr, "Missing Effort") {
		t.Errorf("expected CustomStory to flag Missing Effort, got: %v", storyReasons)
	}
	if strings.Contains(storyReasonsStr, "Missing Estimate") {
		t.Errorf("expected CustomStory NOT to flag Missing Estimate dynamically, got: %v", storyReasons)
	}

	// 3. CustomStory with Effort satisfied
	storyRec.Effort = 5.0
	if IsItemIncompleteWithDB(storyRec, cfg, database, now) {
		t.Errorf("expected complete CustomStory to not be incomplete, got reasons: %v", GetIncompleteReasonsWithDB(storyRec, cfg, database, now))
	}
}

func TestScreenTreeIncompleteCacheAndShortCircuit(t *testing.T) {
	cfg := &config.Config{
		IncompleteCriteriaEffort:   true,
		IncompleteCriteriaEstimate: true,
		IncompleteTypeTasks:        true,
		IncompleteTypePBI:          true,
	}

	st := NewScreenTree()
	st.Config = cfg
	st.IncompleteView = true

	rec := &db.WorkItemRecord{
		ID:           101,
		Title:        "Test Incomplete Item",
		WorkItemType: "Task",
		State:        "Active",
	}

	// Verify short-circuiting returns true without error
	now := time.Now()
	if !IsItemIncompleteWithDB(rec, cfg, nil, now) {
		t.Errorf("expected rec to be incomplete")
	}

	// Render view row to populate cache
	node := &db.TreeNode{
		Record:   rec,
		Children: nil,
	}
	st.SetData([]*db.TreeNode{node})
	st.Width = 120
	st.Height = 30

	_ = st.View(120, 30, "", false)

	if len(st.incompleteCache) == 0 {
		t.Errorf("expected incompleteCache to be populated after View()")
	}
	if !st.incompleteCache[101] {
		t.Errorf("expected cached value for item 101 to be true")
	}

	// Clear cache and verify
	st.ClearIncompleteCache()
	if len(st.incompleteCache) != 0 {
		t.Errorf("expected incompleteCache to be empty after ClearIncompleteCache()")
	}
}

func TestIncompleteTagRegexCriteria(t *testing.T) {
	cfg := &config.Config{
		IncompleteCriteriaEffort:   false,
		IncompleteCriteriaEstimate: false,
		IncompleteCriteriaTagRegex: true,
		TagRegex:                   `^(BE|FE)-`,
		IncompleteTypes:            []string{"[All]"},
		IncompleteStatuses:         []string{"[All]"},
	}

	now := time.Now()

	// Item with matching tag should not be incomplete
	recWithTag := &db.WorkItemRecord{
		ID:           101,
		Title:        "Frontend Task",
		WorkItemType: "Task",
		State:        "Active",
		Tags:         "FE-123; ui",
	}
	if IsItemIncompleteWithDB(recWithTag, cfg, nil, now) {
		t.Errorf("expected item with matching tag not to be incomplete")
	}

	// Item with non-matching tag should be incomplete
	recWithoutTag := &db.WorkItemRecord{
		ID:           102,
		Title:        "Misc Task",
		WorkItemType: "Task",
		State:        "Active",
		Tags:         "general; design",
	}
	if !IsItemIncompleteWithDB(recWithoutTag, cfg, nil, now) {
		t.Errorf("expected item without matching tag to be incomplete")
	}

	reasons := GetIncompleteReasonsWithDB(recWithoutTag, cfg, nil, now)
	foundReason := false
	for _, r := range reasons {
		if strings.Contains(r, "Missing Tag (Regex)") {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Errorf("expected incomplete reasons to contain 'Missing Tag (Regex)', got %v", reasons)
	}

	// Test multi-rule comma/semicolon-separated regex rules
	cfgMulti := &config.Config{
		IncompleteCriteriaTagRegex: true,
		TagRegex:                   `^team-(alpha|beta), ^release-[0-9]+; ^infra-.*`,
		IncompleteTypes:            []string{"[All]"},
		IncompleteStatuses:         []string{"[All]"},
	}

	recMulti1 := &db.WorkItemRecord{ID: 103, Title: "Alpha Task", WorkItemType: "Task", State: "Active", Tags: "team-alpha; dev"}
	recMulti2 := &db.WorkItemRecord{ID: 104, Title: "Release Task", WorkItemType: "Task", State: "Active", Tags: "release-42"}
	recMulti3 := &db.WorkItemRecord{ID: 105, Title: "Infra Task", WorkItemType: "Task", State: "Active", Tags: "infra-k8s"}
	recMulti4 := &db.WorkItemRecord{ID: 106, Title: "Unmatched Task", WorkItemType: "Task", State: "Active", Tags: "other; random"}

	if IsItemIncompleteWithDB(recMulti1, cfgMulti, nil, now) {
		t.Errorf("expected recMulti1 matching first rule not to be incomplete")
	}
	if IsItemIncompleteWithDB(recMulti2, cfgMulti, nil, now) {
		t.Errorf("expected recMulti2 matching second rule not to be incomplete")
	}
	if IsItemIncompleteWithDB(recMulti3, cfgMulti, nil, now) {
		t.Errorf("expected recMulti3 matching third rule not to be incomplete")
	}
	if !IsItemIncompleteWithDB(recMulti4, cfgMulti, nil, now) {
		t.Errorf("expected recMulti4 with no matching tags to be incomplete")
	}
}

func TestIncompleteActivityCriteria(t *testing.T) {
	cfg := &config.Config{
		IncompleteCriteriaEffort:   false,
		IncompleteCriteriaEstimate: false,
		IncompleteCriteriaActivity: true,
		IncompleteTypes:            []string{"[All]"},
		IncompleteStatuses:         []string{"[All]"},
	}

	now := time.Now()

	// Item with Activity should not be incomplete
	recWithActivity := &db.WorkItemRecord{
		ID:           201,
		Title:        "Dev Task",
		WorkItemType: "Task",
		State:        "Active",
		Activity:     "Development",
	}
	if IsItemIncompleteWithDB(recWithActivity, cfg, nil, now) {
		t.Errorf("expected item with Activity not to be incomplete")
	}

	// Item without Activity should be incomplete
	recWithoutActivity := &db.WorkItemRecord{
		ID:           202,
		Title:        "Unassigned Activity Task",
		WorkItemType: "Task",
		State:        "Active",
		Activity:     "",
	}
	if !IsItemIncompleteWithDB(recWithoutActivity, cfg, nil, now) {
		t.Errorf("expected item without Activity to be incomplete")
	}

	reasons := GetIncompleteReasonsWithDB(recWithoutActivity, cfg, nil, now)
	foundReason := false
	for _, r := range reasons {
		if strings.Contains(r, "Missing Activity") {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Errorf("expected incomplete reasons to contain 'Missing Activity', got %v", reasons)
	}
}
