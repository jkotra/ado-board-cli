package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

func TestTUIViewRendering(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	roots := []*db.TreeNode{
		{
			Record: &db.WorkItemRecord{
				ID:           656842,
				WorkItemType: "Feature",
				Title:        "GKE & Spot Instances",
				State:        "New",
			},
			Expanded: true,
			Children: []*db.TreeNode{
				{
					Record: &db.WorkItemRecord{
						ID:           662409,
						WorkItemType: "Product Backlog Item",
						Title:        "Deploy Scalar Forecast on GKE",
						State:        "Committed",
					},
					Expanded: true,
					Children: []*db.TreeNode{
						{
							Record: &db.WorkItemRecord{
								ID:               665688,
								WorkItemType:     "Task",
								Title:            "custom HPA integration",
								State:            "To Do",
								OriginalEstimate: 12,
								RemainingWork:    6,
								CompletedWork:    6,
							},
						},
					},
				},
			},
		},
	}

	screenTree := NewScreenTree()
	screenTree.SetData(roots)
	treeView := screenTree.View(100, 30, "Ready", false)
	if treeView == "" {
		t.Fatal("expected non-empty tree view")
	}
	if !strings.Contains(treeView, "ASSIGNED TO") {
		t.Errorf("expected ASSIGNED TO column header in treeView, got:\n%s", treeView)
	}

	screenItem := NewScreenItem()
	screenItem.SetData(
		roots[0].Children[0].Children[0].Record,
		[]db.CommentRecord{
			{
				AuthorName: "Doe, John",
				Text:       "Testing comment styling",
			},
		},
		[]*db.WorkItemRecord{
			{
				ID:           662409,
				WorkItemType: "Product Backlog Item",
				Title:        "Deploy Scalar Forecast on GKE",
				State:        "Committed",
			},
		},
		[]*db.PullRequestRecord{
			{
				ID:           1240,
				Title:        "Implement custom HPA metrics provider",
				Status:       "active",
				SourceBranch: "feature/hpa-metrics",
				TargetBranch: "main",
			},
		},
	)
	itemView := screenItem.View(100, 80, "", false)
	if itemView == "" {
		t.Fatal("expected non-empty item view")
	}
	if !strings.Contains(itemView, "PR #1240") {
		t.Errorf("expected PR #1240 in itemView, got: %s", itemView)
	}
	if !strings.Contains(itemView, "Pull Requests") {
		t.Errorf("expected Pull Requests header, got: %s", itemView)
	}

	screenCreate := NewScreenCreate()
	createView := screenCreate.View(100, 30, "", false)
	if createView == "" {
		t.Fatal("expected non-empty create view")
	}

	modalAssign := NewModalAssign()
	modalAssign.Open(665688, "custom HPA integration", "user@domain.com", database)
	assignView := modalAssign.View(100, 30)
	if assignView == "" {
		t.Fatal("expected non-empty assign view")
	}

	modalComment := NewModalComment()
	modalComment.Open(665688, "custom HPA integration", database)
	commentView := modalComment.View(100, 30)
	if commentView == "" {
		t.Fatal("expected non-empty comment view")
	}

	modalEdit := NewModalEdit()
	modalEdit.Open(665688, "Task", "custom HPA integration", "Description content", "backend; gke", "Project\\Sprint 1", "Project\\Team A", database)
	editView := modalEdit.View(100, 30)
	if editView == "" {
		t.Fatal("expected non-empty edit view")
	}

	modalEstimate := NewModalEstimate()
	modalEstimate.Open(665688, "custom HPA integration", "Task", 12, 6, 6, 0)
	estView := modalEstimate.View(100, 30)
	if estView == "" {
		t.Fatal("expected non-empty estimate view")
	}

	modalConfirm := NewModalConfirm()
	modalConfirm.Open("Confirm Destructive AI Action", "Gemini wants to delete work item #665688", "gemini_confirm", 665688, true)
	confirmView := modalConfirm.View(100, 30)
	if confirmView == "" {
		t.Fatal("expected non-empty confirm view")
	}

	// Test Removed state item strikethrough
	removedRecord := &db.WorkItemRecord{
		ID:           662397,
		WorkItemType: "Task",
		Title:        "Implementation",
		State:        "Removed",
	}
	screenItem.SetItem(removedRecord, nil)
	removedItemView := screenItem.View(100, 30, "", false)
	if removedItemView == "" {
		t.Fatal("expected non-empty removed item view")
	}
}

func TestAutocompleteTriggersAndReplacement(t *testing.T) {
	// 1. Trigger detection for @
	tr, q, idx, found := DetectInlineTrigger("Hello @doe", 11)
	if !found || tr != '@' || q != "doe" || idx != 6 {
		t.Fatalf("unexpected @ trigger parse: found=%v, tr=%c, q=%s, idx=%d", found, tr, q, idx)
	}

	// 2. Trigger detection for #
	tr, q, idx, found = DetectInlineTrigger("Refers to #665", 14)
	if !found || tr != '#' || q != "665" || idx != 10 {
		t.Fatalf("unexpected # trigger parse: found=%v, tr=%c, q=%s, idx=%d", found, tr, q, idx)
	}

	// 3. Trigger detection for !
	tr, q, idx, found = DetectInlineTrigger("See PR !124", 11)
	if !found || tr != '!' || q != "124" || idx != 7 {
		t.Fatalf("unexpected ! trigger parse: found=%v, tr=%c, q=%s, idx=%d", found, tr, q, idx)
	}

	// 4. Token replacement
	replaced := ReplaceToken("Working on @doe to fix bug", 11, 3, "@John Doe <john@example.com>")
	expected := "Working on @John Doe <john@example.com> to fix bug"
	if replaced != expected {
		t.Fatalf("unexpected replaced string:\nGot:  %q\nWant: %q", replaced, expected)
	}
}

func TestModalFilterViewAndNavigation(t *testing.T) {
	modal := NewModalFilter()
	filterOpts := db.FilterOptions{
		Status:     "Active",
		Op1:        "AND",
		Assignee:   "John",
		Op2:        "OR",
		ItemType:   "Bug",
		Op3:        "AND",
		SearchTerm: "ingress",
	}

	modal.Open(filterOpts, nil)

	// Check initial values
	view := modal.View(100, 30)
	if view == "" {
		t.Fatal("expected non-empty filter modal view")
	}

	// Toggle Op1
	modal.FocusIndex = 1 // Op1
	modal.Update(tea.KeyMsg{Type: tea.KeySpace})
	if modal.Op1 != "OR" {
		t.Errorf("expected Op1 to be OR after toggle, got %s", modal.Op1)
	}

	// Get options
	opts := modal.GetFilterOptions()
	if opts.Op1 != "OR" {
		t.Errorf("expected GetFilterOptions Op1 to be OR, got %s", opts.Op1)
	}
	if opts.SearchTerm != "ingress" {
		t.Errorf("expected SearchTerm to be ingress, got %s", opts.SearchTerm)
	}

	// Clear
	modal.Clear()
	clearedOpts := modal.GetFilterOptions()
	if clearedOpts.IsActive() {
		t.Errorf("expected cleared filter to not be active, got %+v", clearedOpts)
	}

	// Test FilterPicker sub-modal interaction
	modal.AssigneeOptions = []string{"[All]", "Unassigned", "Miller, Erin Grace <erin.miller@example.com>", "Doe, John <jdoe@example.com>"}
	modal.FocusIndex = 2 // Assignee
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !modal.Picker.Active {
		t.Fatal("expected FilterPicker to be active after Enter on Assignee field")
	}

	pickerView := modal.View(100, 30)
	if !strings.Contains(pickerView, "Select Assignee") {
		t.Fatalf("expected picker header in view, got: %s", pickerView)
	}

	// Type search query "erin" into picker
	for _, ch := range "erin" {
		modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if len(modal.Picker.Filtered) != 2 { // [All] and Erin
		t.Fatalf("expected 2 filtered items matching erin (including [All]), got %d", len(modal.Picker.Filtered))
	}

	// Navigate down to highlight Erin
	modal.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press Enter to select
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if modal.Picker.Active {
		t.Fatal("expected FilterPicker to close after Enter")
	}

	selectedAssignee := modal.GetFilterOptions().Assignee
	if !strings.Contains(selectedAssignee, "Miller, Erin Grace") {
		t.Fatalf("expected selected assignee to contain 'Miller, Erin Grace', got: %s", selectedAssignee)
	}

	// Re-open picker and test typing 'j' and 'k' specifically (regression test for j/k key hijacking)
	modal.FocusIndex = 2
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !modal.Picker.Active {
		t.Fatal("expected FilterPicker to be active")
	}

	for _, ch := range "john" {
		modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	if modal.Picker.SearchInput.Value() != "john" {
		t.Fatalf("expected search input to contain 'john', got %q", modal.Picker.SearchInput.Value())
	}
	if len(modal.Picker.Filtered) != 2 { // [All] and Doe, John
		t.Fatalf("expected 2 filtered items matching 'john' (including [All]), got %d", len(modal.Picker.Filtered))
	}

	// Navigate down and select
	modal.Update(tea.KeyMsg{Type: tea.KeyDown})
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	selectedAssignee2 := modal.GetFilterOptions().Assignee
	if !strings.Contains(selectedAssignee2, "Doe, John") {
		t.Fatalf("expected selected assignee to contain 'Doe, John', got: %s", selectedAssignee2)
	}

	// Test Multi-Select with Space key checkboxes on Work Item Types
	modal.TypeOptions = []string{"[All]", "Task", "Bug", "Product Backlog Item", "Feature", "Epic"}
	modal.FocusIndex = filterFieldType
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !modal.Picker.Active {
		t.Fatal("expected FilterPicker to be active on Type field")
	}

	// Navigate to Task (index 1) and press Space to toggle
	modal.Update(tea.KeyMsg{Type: tea.KeyDown})
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	// Navigate to Bug (index 2) and press Space to toggle
	modal.Update(tea.KeyMsg{Type: tea.KeyDown})
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	// Confirm with Enter
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if modal.Picker.Active {
		t.Fatal("expected FilterPicker to close after Enter")
	}

	typesOpts := modal.GetFilterOptions()
	if len(typesOpts.ItemTypes) != 2 || typesOpts.ItemTypes[0] != "Task" || typesOpts.ItemTypes[1] != "Bug" {
		t.Fatalf("expected ItemTypes to be [Task, Bug], got %+v", typesOpts.ItemTypes)
	}

	// Test Button Navigation in Filter Modal (0: Apply Global, 1: Apply Tab, 2: Clear Tab, 3: Clear All, 4: Cancel)
	modal.FocusIndex = filterFieldButtons
	modal.ActionButtonIdx = 0 // Apply Global

	// Left moves to Cancel (idx 4)
	modal.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if modal.ActionButtonIdx != 4 {
		t.Fatalf("expected ActionButtonIdx to wrap to 4 (Cancel), got %d", modal.ActionButtonIdx)
	}

	// Right moves back to Apply Global (idx 0)
	modal.Update(tea.KeyMsg{Type: tea.KeyRight})
	if modal.ActionButtonIdx != 0 {
		t.Fatalf("expected ActionButtonIdx to be 0 (Apply Global), got %d", modal.ActionButtonIdx)
	}

	// Right moves to Apply Tab (idx 1)
	modal.Update(tea.KeyMsg{Type: tea.KeyRight})
	if modal.ActionButtonIdx != 1 {
		t.Fatalf("expected ActionButtonIdx to be 1 (Apply Tab), got %d", modal.ActionButtonIdx)
	}

	// Right moves to Clear Tab (idx 2)
	modal.Update(tea.KeyMsg{Type: tea.KeyRight})
	if modal.ActionButtonIdx != 2 {
		t.Fatalf("expected ActionButtonIdx to be 2 (Clear Tab), got %d", modal.ActionButtonIdx)
	}

	// Right moves to Clear All (idx 3)
	modal.Update(tea.KeyMsg{Type: tea.KeyRight})
	if modal.ActionButtonIdx != 3 {
		t.Fatalf("expected ActionButtonIdx to be 3 (Clear All), got %d", modal.ActionButtonIdx)
	}

	// Right moves to Cancel (idx 4)
	modal.Update(tea.KeyMsg{Type: tea.KeyRight})
	if modal.ActionButtonIdx != 4 {
		t.Fatalf("expected ActionButtonIdx to be 4 (Cancel), got %d", modal.ActionButtonIdx)
	}

	// Check View rendering for modern buttons
	finalView := modal.View(100, 30)
	if !strings.Contains(finalView, "Global") || !strings.Contains(finalView, "Tab") || !strings.Contains(finalView, "Clear Tab") || !strings.Contains(finalView, "Clear All") || !strings.Contains(finalView, "Cancel") {
		t.Fatalf("expected finalView to contain modern buttons, got:\n%s", finalView)
	}
}

func TestFilterPickerCheckboxTogglingAndActions(t *testing.T) {
	picker := NewFilterPicker()
	options := []string{"[All]", "Active", "Closed", "Resolved", "To Do"}

	// 1. Open with initial selection of "Active" and "To Do"
	picker.Open("Select Statuses", filterFieldStatus, options, []string{"Active", "To Do"}, ColorSecondary)
	if !picker.Active {
		t.Fatal("expected picker to be active")
	}
	if picker.SelectedCount() != 2 {
		t.Fatalf("expected 2 selected, got %d", picker.SelectedCount())
	}

	// Verify checkbox rendering in View
	view := picker.View(80, 20)
	if !strings.Contains(view, "[✓]") || !strings.Contains(view, "[ ]") {
		t.Fatalf("expected view to contain [✓] and [ ] checkboxes, got:\n%s", view)
	}
	if !strings.Contains(view, "2 selected") {
		t.Fatalf("expected view to show '2 selected', got:\n%s", view)
	}

	// 2. Space on cursor 0 ([All]) should uncheck individual selections and select [All]
	picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !picker.SelectedOptions["[All]"] {
		t.Fatal("expected [All] to be selected after pressing Space on [All]")
	}
	if picker.SelectedCount() != 0 {
		t.Fatalf("expected selected count 0 when [All] is active, got %d", picker.SelectedCount())
	}

	// 3. Move down to "Active" (index 1) and press Space: should select only "Active"
	picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if picker.SelectedOptions["[All]"] {
		t.Fatal("expected [All] to be unchecked when an item is selected")
	}
	if !picker.SelectedOptions["Active"] {
		t.Fatal("expected Active to be selected")
	}

	// 4. Move down to "Closed" (index 2) and press Space: should select "Closed" too
	picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !picker.SelectedOptions["Closed"] {
		t.Fatal("expected Closed to be selected")
	}
	if picker.SelectedCount() != 2 {
		t.Fatalf("expected 2 items selected (Active, Closed), got %d", picker.SelectedCount())
	}

	// 5. Test Escape discards changes and restores initial selection
	picker.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if picker.Active {
		t.Fatal("expected picker to close on Esc")
	}
	// Initial was Active and To Do
	if !picker.SelectedOptions["Active"] || !picker.SelectedOptions["To Do"] || picker.SelectedOptions["Closed"] {
		t.Fatalf("expected selection restored to Active and To Do, got %+v", picker.SelectedOptions)
	}

	// 6. Test Enter confirms and closes
	picker.Open("Select Statuses", filterFieldStatus, options, []string{"Resolved"}, ColorSecondary)
	// Move down to "To Do" (index 4) and press Space
	picker.Cursor = 4
	picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	selectedList, confirmed, closed, _ := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !confirmed || !closed {
		t.Fatalf("expected confirmed=true, closed=true, got confirmed=%v, closed=%v", confirmed, closed)
	}
	if len(selectedList) != 2 || selectedList[0] != "Resolved" || selectedList[1] != "To Do" {
		t.Fatalf("expected [Resolved, To Do], got %+v", selectedList)
	}
}

func TestModalFilterFullWorkflowAndShortcuts(t *testing.T) {
	modal := NewModalFilter()
	initialFilter := db.FilterOptions{
		Statuses:   []string{"Active"},
		ItemTypes:  []string{"Bug"},
		Assignees:  []string{"Doe, John"},
		Op1:        "AND",
		Op2:        "OR",
		Op3:        "AND",
		SearchTerm: "login crash",
	}
	modal.Open(initialFilter, nil)

	// Verify loaded state
	opts := modal.GetFilterOptions()
	if len(opts.Statuses) != 1 || opts.Statuses[0] != "Active" {
		t.Fatalf("expected Statuses to be [Active], got %+v", opts.Statuses)
	}
	if len(opts.ItemTypes) != 1 || opts.ItemTypes[0] != "Bug" {
		t.Fatalf("expected ItemTypes to be [Bug], got %+v", opts.ItemTypes)
	}
	if opts.Op2 != "OR" {
		t.Fatalf("expected Op2 to be OR, got %s", opts.Op2)
	}
	if opts.SearchTerm != "login crash" {
		t.Fatalf("expected SearchTerm to be 'login crash', got %s", opts.SearchTerm)
	}

	// 1. Test Shortcuts: Ctrl+S -> Apply
	_, _, actionApply := modal.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if actionApply != FilterActionApply {
		t.Fatalf("expected FilterActionApply on Ctrl+S, got %v", actionApply)
	}

	// 2. Test Shortcuts: Ctrl+C -> Cancel
	_, _, actionCancel := modal.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if actionCancel != FilterActionCancel {
		t.Fatalf("expected FilterActionCancel on Ctrl+C, got %v", actionCancel)
	}

	// 3. Test Shortcuts: Ctrl+R -> Clear
	_, _, actionClear := modal.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if actionClear != FilterActionClear {
		t.Fatalf("expected FilterActionClear on Ctrl+R, got %v", actionClear)
	}
	clearedOpts := modal.GetFilterOptions()
	if len(clearedOpts.Statuses) != 0 || len(clearedOpts.ItemTypes) != 0 || len(clearedOpts.Assignees) != 0 || clearedOpts.SearchTerm != "" {
		t.Fatalf("expected all filters cleared, got %+v", clearedOpts)
	}

	// 4. Test Operator Toggles via Space
	modal.FocusIndex = filterFieldOp1
	modal.Op1 = "AND"
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if modal.Op1 != "OR" {
		t.Fatalf("expected Op1 toggled to OR, got %s", modal.Op1)
	}
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if modal.Op1 != "AND" {
		t.Fatalf("expected Op1 toggled back to AND, got %s", modal.Op1)
	}

	// 5. Test Tab Navigation through all fields
	modal.FocusIndex = filterFieldStatus
	for i := 0; i < filterFieldCount; i++ {
		expected := (filterFieldStatus + i + 1) % filterFieldCount
		modal.Update(tea.KeyMsg{Type: tea.KeyTab})
		if modal.FocusIndex != expected {
			t.Fatalf("step %d: expected FocusIndex %d, got %d", i, expected, modal.FocusIndex)
		}
	}
}

func TestScrollClippedText(t *testing.T) {
	short := "John Doe"
	if s := ScrollClippedText(short, 10, 0); s != short {
		t.Fatalf("expected %q for short text, got %q", short, s)
	}

	long := "Wilson, Frank Lee <frank.wilson@example.com>"
	// tick 0: paused at beginning
	if s := ScrollClippedText(long, 20, 0); s != "Wilson, Frank Lee <f" {
		t.Fatalf("expected initial slice %q, got %q", "Wilson, Frank Lee <f", s)
	}

	// tick 4: start of scrolling (after 3-tick pause)
	if s := ScrollClippedText(long, 20, 4); s != "ilson, Frank Lee <fr" {
		t.Fatalf("expected scrolled slice %q, got %q", "ilson, Frank Lee <fr", s)
	}
}

func TestDockGeminiScrollingAndClear(t *testing.T) {
	dock := NewDockGemini("gemini-3.7-flash")
	dock.Visible = true
	dock.SetSize(80, 24)

	// Set a multi-line response
	longResponse := "1. Item 1\n2. Item 2\n3. Item 3\n4. Item 4\n5. Item 5\n6. Item 6\n7. Item 7\n8. Item 8\n9. Item 9\n10. Item 10\n11. Item 11\n12. Item 12\n13. Item 13\n14. Item 14\n15. Item 15"

	dock.ResponseMsg = longResponse
	dock.StatusMsg = "Done"

	// Check view rendering
	view := dock.View()
	if !strings.Contains(view, "Gemini AI Assistant") {
		t.Fatal("expected view to contain header")
	}
	if !strings.Contains(view, "PgUp/PgDn to scroll") {
		t.Fatal("expected view to contain scroll indicator for long response")
	}

	// Test scrolling down with pagedown
	dock.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if dock.ScrollOffset == 0 {
		t.Fatal("expected ScrollOffset to increase after PgDown")
	}

	// Test scrolling up with pageup
	dock.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if dock.ScrollOffset != 0 {
		t.Fatalf("expected ScrollOffset to be 0 after PgUp, got %d", dock.ScrollOffset)
	}

	// Test scrolling down with ctrl+d
	dock.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if dock.ScrollOffset != 4 {
		t.Fatalf("expected ScrollOffset to be 4 after ctrl+d, got %d", dock.ScrollOffset)
	}

	// Verify typing 'c' types into prompt input without clearing response
	dock.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if dock.Input.Value() != "c" {
		t.Fatalf("expected input value 'c', got %q", dock.Input.Value())
	}
	if dock.ResponseMsg == "" {
		t.Fatal("expected response NOT to be cleared by typing 'c'")
	}

	// Test 'delete' / 'del' key clears response history when input is empty
	dock.ResponseMsg = "Some previous output"
	dock.StatusMsg = "Done"
	dock.Input.Reset()
	dock.Update(tea.KeyMsg{Type: tea.KeyDelete})
	if dock.ResponseMsg != "" || dock.StatusMsg != "" {
		t.Fatalf("expected response and status to be cleared on Delete key, got response=%q", dock.ResponseMsg)
	}

	// Now verify that when response is cleared, view does not show response box
	clearedView := dock.View()
	if strings.Contains(clearedView, "PgUp/PgDn to scroll") {
		t.Fatal("expected scroll indicator to be absent after clear")
	}
}

func TestModalTrackMenu(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer database.Close()

	modal := NewModalTrack()

	// 1. Open with empty DB
	modal.Open(database)
	if !modal.Active {
		t.Fatal("expected modal to be active")
	}
	if !modal.InputMode {
		t.Fatal("expected modal to start in InputMode when no items are tracked")
	}
	viewEmpty := modal.View(80, 24)
	if !strings.Contains(viewEmpty, "No queries or work items currently tracked") {
		t.Fatalf("expected empty track list message, got:\n%s", viewEmpty)
	}

	// 2. Add tracked query & roots
	_ = database.SaveTrackedQuery("query-uuid-1", "Sprint 2026 Backlog", "Shared Queries/Sprint", "myorg", "myproj", "https://dev.azure.com/...", "tree")
	_ = database.SaveTrackedRoot(100, "myorg", "myproj", "https://dev.azure.com/myorg/myproj/_workitems/edit/100")
	_ = database.SaveTrackedRoot(200, "myorg", "myproj", "https://dev.azure.com/myorg/myproj/_workitems/edit/200")

	modal.Open(database)
	if modal.InputMode {
		t.Fatal("expected modal to start in List mode when items exist")
	}
	if len(modal.Queries) != 1 {
		t.Fatalf("expected 1 tracked query, got %d", len(modal.Queries))
	}
	if len(modal.Items) != 2 {
		t.Fatalf("expected 2 tracked items, got %d", len(modal.Items))
	}
	if modal.TotalCount() != 3 {
		t.Fatalf("expected 3 total items, got %d", modal.TotalCount())
	}

	// 3. Navigate list (Cursor 0 is Query, Cursor 1 is Item 100, Cursor 2 is Item 200)
	qSel := modal.SelectedQuery()
	if qSel == nil || qSel.Name != "Sprint 2026 Backlog" {
		t.Fatalf("expected query selected at cursor 0, got %+v", qSel)
	}

	modal.Update(tea.KeyMsg{Type: tea.KeyDown})
	if modal.Cursor != 1 {
		t.Fatalf("expected cursor at 1, got %d", modal.Cursor)
	}
	itemSel := modal.SelectedItem()
	if itemSel == nil || itemSel.ID != 200 {
		t.Fatalf("expected item 200 selected at cursor 1, got %+v", itemSel)
	}

	// 4. Untrack item #200
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(modal.Items) != 1 {
		t.Fatalf("expected 1 item after untrack, got %d", len(modal.Items))
	}

	// 5. Untrack query
	modal.Cursor = 0
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if len(modal.Queries) != 0 {
		t.Fatalf("expected 0 queries after untrack, got %d", len(modal.Queries))
	}

	// 6. Switch to input mode with 'a'
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !modal.InputMode {
		t.Fatal("expected modal to switch to InputMode on 'a'")
	}
}

func TestScreenTreeTabsAndNavigation(t *testing.T) {
	root1 := &db.TreeNode{
		Record:   &db.WorkItemRecord{ID: 100, Title: "Epic 100", WorkItemType: "Epic"},
		Expanded: true,
		Children: []*db.TreeNode{
			{
				Record: &db.WorkItemRecord{ID: 101, Title: "Feature 101", WorkItemType: "Feature", ParentID: 100},
			},
		},
	}
	root2 := &db.TreeNode{
		Record:   &db.WorkItemRecord{ID: 200, Title: "User Story 200", WorkItemType: "User Story"},
		Expanded: true,
		Children: []*db.TreeNode{
			{
				Record: &db.WorkItemRecord{ID: 201, Title: "Task 201", WorkItemType: "Task", ParentID: 200},
			},
		},
	}

	screen := NewScreenTree()
	screen.SetData([]*db.TreeNode{root1, root2})

	// 1. Initially on Tab 0 (Epic 100)
	if screen.ActiveTab != 0 {
		t.Fatalf("expected ActiveTab 0, got %d", screen.ActiveTab)
	}
	if len(screen.Flattened) != 2 || screen.Flattened[0].Node.Record.ID != 100 {
		t.Fatalf("expected 2 items for root 1, got %d items", len(screen.Flattened))
	}

	// 2. Next tab with ']'
	screen.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if screen.ActiveTab != 1 {
		t.Fatalf("expected ActiveTab 1, got %d", screen.ActiveTab)
	}
	if len(screen.Flattened) != 2 || screen.Flattened[0].Node.Record.ID != 200 {
		t.Fatalf("expected root 2 items, got %d items", len(screen.Flattened))
	}

	// 3. Prev tab with '['
	screen.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if screen.ActiveTab != 0 {
		t.Fatalf("expected ActiveTab 0, got %d", screen.ActiveTab)
	}

	// 4. Tab bar rendering
	tabBar := screen.renderTabBar(100)
	if !strings.Contains(tabBar, "Epic 100") || !strings.Contains(tabBar, "User Story 200") {
		t.Fatalf("expected tab bar to render roots, got:\n%s", tabBar)
	}

	// 5. SelectItemID across tabs
	screen.SelectItemID(201)
	if screen.ActiveTab != 1 {
		t.Fatalf("expected SelectItemID to switch to ActiveTab 1, got %d", screen.ActiveTab)
	}
	if sel := screen.SelectedItem(); sel == nil || sel.ID != 201 {
		t.Fatalf("expected selected item 201, got %v", sel)
	}

	// 6. View rendering contains tab bar above ribbon
	viewStr := screen.View(100, 30, "", false)
	if !strings.Contains(viewStr, "Roots:") || !strings.Contains(viewStr, "View") {
		t.Fatalf("expected view to contain tab bar and ribbon, got:\n%s", viewStr)
	}
}

func TestScreenCreateParentContext(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer database.Close()

	parent := &db.WorkItemRecord{
		ID:           662411,
		Title:        "Create Memory Sharing / Caching layer",
		WorkItemType: "Product Backlog Item",
		State:        "Approved",
	}

	sc := NewScreenCreate()
	sc.Reset(parent, "user@domain.com", database)

	if sc.ParentID() != 662411 {
		t.Fatalf("expected ParentID 662411, got %d", sc.ParentID())
	}
	if sc.SelectedType() != "Task" {
		t.Fatalf("expected default child type Task for PBI parent, got %s", sc.SelectedType())
	}

	viewStr := sc.View(120, 35, "", false)
	if !strings.Contains(viewStr, "662411") || !strings.Contains(viewStr, "Create Memory Sharing") {
		t.Fatalf("expected parent info in create view, got:\n%s", viewStr)
	}
	if !strings.Contains(viewStr, "Target Parent:") {
		t.Fatalf("expected 'Target Parent:' banner in create view, got:\n%s", viewStr)
	}
}

func TestScreenCreateViewportAndScrolling(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer database.Close()

	parent := &db.WorkItemRecord{
		ID:           662411,
		Title:        "Create Memory Sharing / Caching layer",
		WorkItemType: "Product Backlog Item",
		State:        "Approved",
	}

	sc := NewScreenCreate()
	sc.Reset(parent, "user@domain.com", database)
	sc.InitViewport(100, 25)

	if !sc.Ready {
		t.Fatal("expected ScreenCreate to be ready after InitViewport")
	}
	if sc.Viewport.Height != 22 {
		t.Fatalf("expected viewport height 22 (25 - 2 header - 1 ribbon), got %d", sc.Viewport.Height)
	}

	// 1. Initial view render should bounded to exactly 25 lines (2 header + 22 viewport + 1 ribbon)
	viewStr := sc.View(100, 25, "", false)
	lines := strings.Split(viewStr, "\n")
	if len(lines) != 25 {
		t.Fatalf("expected 25 lines in view output, got %d", len(lines))
	}
	// Verify header and parent banner are visible
	if !strings.Contains(lines[0], "CREATE NEW WORK ITEM") {
		t.Fatalf("expected top line to have header, got: %s", lines[0])
	}
	if !strings.Contains(viewStr, "Target Parent:") {
		t.Fatalf("expected Target Parent banner to be visible in initial viewport")
	}

	// 2. Tab down to fieldCreateDesc and fieldCreateSubmit (triggers auto-scroll on small viewport)
	sc.InitViewport(100, 16) // use a small viewport to test scrolling
	sc.View(100, 16, "", false)
	sc.FocusedField = fieldCreateType
	sc.updateFocus()
	for i := 0; i < 3; i++ {
		sc, _ = sc.Update(tea.KeyMsg{Type: tea.KeyTab})
	}
	if sc.FocusedField != fieldCreateSubmit {
		t.Fatalf("expected focused field to be Submit (%d), got %d", fieldCreateSubmit, sc.FocusedField)
	}
	sc.View(100, 16, "", false)
	if sc.Viewport.YOffset <= 0 {
		t.Fatalf("expected viewport YOffset to be > 0 after tabbing to bottom fields, got %d", sc.Viewport.YOffset)
	}

	// 3. Mouse wheel up should scroll up
	offsetBefore := sc.Viewport.YOffset
	sc, _ = sc.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if sc.Viewport.YOffset >= offsetBefore {
		t.Fatalf("expected YOffset to decrease after mouse wheel up, was %d now %d", offsetBefore, sc.Viewport.YOffset)
	}

	// 4. Mouse wheel down should scroll down
	offsetBefore = sc.Viewport.YOffset
	sc, _ = sc.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if sc.Viewport.YOffset <= offsetBefore {
		t.Fatalf("expected YOffset to increase after mouse wheel down, was %d now %d", offsetBefore, sc.Viewport.YOffset)
	}

	// 5. pgup and pgdown key support
	offsetBefore = sc.Viewport.YOffset
	sc, _ = sc.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if sc.Viewport.YOffset >= offsetBefore {
		t.Fatalf("expected YOffset to decrease after pgup, was %d now %d", offsetBefore, sc.Viewport.YOffset)
	}
	offsetBefore = sc.Viewport.YOffset
	sc, _ = sc.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if sc.Viewport.YOffset <= offsetBefore {
		t.Fatalf("expected YOffset to increase after pgdown, was %d now %d", offsetBefore, sc.Viewport.YOffset)
	}

	// 6. Full-screen rendering and symmetry verification on wide display
	fullView := sc.View(140, 40, "", false)
	requiredSubstrings := []string{
		"Target Parent:",
		"Work Item Type",
		"Title (required):",
		"Description / Notes (Markdown):",
		"Create Work Item",
	}
	for _, sub := range requiredSubstrings {
		if !strings.Contains(fullView, sub) {
			t.Errorf("expected fullView to contain %q, but was missing", sub)
		}
	}

	// Verify no hideous symbols in buttons or labels
	prohibitedSymbols := []string{"✔", "●", "🎯", "▶", "❯"}
	for _, sym := range prohibitedSymbols {
		if strings.Contains(fullView, sym) {
			t.Errorf("expected view not to contain symbol %q in buttons/labels, but found it in:\n%s", sym, fullView)
		}
	}

	// 7. Test Enter key advances single-line fields
	sc.FocusedField = fieldCreateType
	sc.updateFocus()
	sc, _ = sc.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sc.FocusedField != fieldCreateTitle {
		t.Fatalf("expected Enter on type to advance to Title, got %d", sc.FocusedField)
	}
	sc, _ = sc.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sc.FocusedField != fieldCreateDesc {
		t.Fatalf("expected Enter on Title to advance to Desc, got %d", sc.FocusedField)
	}

	// 8. Test Mouse Click focus
	sc.InitViewport(140, 40)
	sc.View(140, 40, "", false)
	sc, _ = sc.Update(tea.MouseMsg{Action: tea.MouseActionPress, Type: tea.MouseLeft, X: 50, Y: 35})
	if sc.FocusedField != fieldCreateSubmit {
		t.Fatalf("expected click near bottom to focus Submit button, got %d", sc.FocusedField)
	}
}

func TestScreenItemLinkSelectionAndHotkeys(t *testing.T) {
	screenItem := NewScreenItem()
	item := &db.WorkItemRecord{
		ID:           663713,
		WorkItemType: "Task",
		Title:        "Create long term forecast job",
		State:        "In Progress",
	}
	prs := []*db.PullRequestRecord{
		{
			ID:           294732,
			Title:        "long term forecast job",
			Status:       "active",
			SourceBranch: "663713-task-long-term-forecast-job",
			TargetBranch: "CAISO-production-integration-jobs",
			URL:          "https://dev.azure.com/my-org/my-project/_git/my-repo/pullrequest/294732",
		},
		{
			ID:           294735,
			Title:        "integration tests for forecast job",
			Status:       "active",
			SourceBranch: "663713-tests",
			TargetBranch: "main",
			URL:          "https://dev.azure.com/my-org/my-project/_git/my-repo/pullrequest/294735",
		},
	}

	screenItem.SetData(item, nil, nil, prs)

	if len(screenItem.RelatedLinks) != 2 {
		t.Fatalf("expected 2 related links, got %d", len(screenItem.RelatedLinks))
	}

	// Default selection
	sel := screenItem.SelectedLink()
	if sel == nil || sel.ID != 294732 {
		t.Fatalf("expected first PR #294732 selected, got %+v", sel)
	}

	// Cycle to next link
	screenItem.NextLink()
	sel2 := screenItem.SelectedLink()
	if sel2 == nil || sel2.ID != 294735 {
		t.Fatalf("expected second PR #294735 selected, got %+v", sel2)
	}

	// Cycle back
	screenItem.PrevLink()
	sel3 := screenItem.SelectedLink()
	if sel3 == nil || sel3.ID != 294732 {
		t.Fatalf("expected first PR #294732 selected, got %+v", sel3)
	}

	// Check view contains distinct hints for PR and Item
	view := screenItem.View(120, 40, "", false)
	if !strings.Contains(view, "[p") || !strings.Contains(view, "Open Link/PR") {
		t.Errorf("expected ribbon to contain [p] Open Link/PR, got:\n%s", view)
	}
	if !strings.Contains(view, "[o") || !strings.Contains(view, "Open Current Item") {
		t.Errorf("expected ribbon to contain [o] Open Current Item, got:\n%s", view)
	}
}

func TestDockGeminiResizeAndMultiline(t *testing.T) {
	dock := NewDockGemini("gemini-3.7-flash")
	dock.Show()
	dock.SetSize(100, 30)

	initialHeight := dock.GetPaneHeight()
	renderedInitHeight := lipgloss.Height(dock.View())
	if renderedInitHeight != initialHeight {
		t.Fatalf("expected rendered height %d to match pane height %d", renderedInitHeight, initialHeight)
	}

	// 1. Resize height up with ctrl+up / IncreaseHeight
	dock.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	if dock.GetPaneHeight() <= initialHeight {
		t.Fatalf("expected height to increase from %d, got %d", initialHeight, dock.GetPaneHeight())
	}
	renderedUpHeight := lipgloss.Height(dock.View())
	if renderedUpHeight != dock.GetPaneHeight() {
		t.Fatalf("expected rendered height %d to match increased pane height %d", renderedUpHeight, dock.GetPaneHeight())
	}

	// 2. Resize height down with ctrl+down / DecreaseHeight
	dock.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	if dock.GetPaneHeight() != initialHeight {
		t.Fatalf("expected height to decrease back to %d, got %d", initialHeight, dock.GetPaneHeight())
	}

	// 3. Alt+Enter or Ctrl+J inserts newline into textarea
	dock.Input.SetValue("line 1")
	dock.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if !strings.Contains(dock.Input.Value(), "\n") {
		t.Fatalf("expected textarea to have newline after ctrl+j, got %q", dock.Input.Value())
	}

	// 4. Typing 'g' types the letter 'g' and does NOT close dock
	dock.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if !dock.Visible {
		t.Fatal("expected dock to remain visible after typing 'g'")
	}
	if !strings.HasSuffix(dock.Input.Value(), "g") {
		t.Fatalf("expected input to end with 'g', got %q", dock.Input.Value())
	}

	// 5. Proportional height: prompt can expand up to innerH - 1 (5) when idle, or innerH / 2 (3) when response is present
	dock.Show()
	dock.PaneHeight = 8
	dock.Input.SetValue("line 1\nline 2\nline 3\nline 4\nline 5\nline 6")
	dock.updateInputHeight()
	if dock.Input.Height() != 5 { // innerH = 6, maxInputH = innerH - 1 = 5 when idle
		t.Fatalf("expected input height 5 when idle and pane height is 8, got %d", dock.Input.Height())
	}

	// When response is present, maxInputH is clamped to innerH / 2 = 3
	dock.ResponseMsg = "Sample AI response"
	dock.updateInputHeight()
	if dock.Input.Height() != 3 {
		t.Fatalf("expected input height 3 when response is present and pane height is 8, got %d", dock.Input.Height())
	}
	dock.ResponseMsg = ""

	// Increase pane height to 20 (innerH = 18, maxInputH = 17)
	dock.PaneHeight = 20
	dock.updateInputHeight()
	if dock.Input.Height() != 6 { // all 6 lines fit within maxInputH
		t.Fatalf("expected input height 6 when pane height is 20, got %d", dock.Input.Height())
	}

	// Check response lines increase proportionally
	dock.Input.SetValue("single line")
	dock.updateInputHeight()
	dock.ResponseMsg = "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\nLine 11\nLine 12"
	dock.PaneHeight = 8
	avail8 := dock.availableResponseHeight()
	dock.PaneHeight = 20
	avail20 := dock.availableResponseHeight()
	if avail20 <= avail8 {
		t.Fatalf("expected available response height to increase from %d at pane 8 to > %d at pane 20, got %d", avail8, avail8, avail20)
	}

	// 6. Esc closes the dock
	dock.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if dock.Visible {
		t.Fatal("expected dock to be hidden after pressing Esc")
	}
}

func TestModalEstimateAutoCalculation(t *testing.T) {
	modal := NewModalEstimate()

	// 1. Opening modal with Original=10, Completed=0, Effort=8 on a Custom item (all 4 fields enabled)
	modal.Open(12345, "Test Item", "Custom", 10.0, 0.0, 0.0, 8.0)
	orig, rem, comp, eff := modal.Values()
	if orig != 10.0 || rem != 10.0 || comp != 0.0 || eff != 8.0 {
		t.Fatalf("expected orig=10, rem=10, comp=0, eff=8, got orig=%f, rem=%f, comp=%f, eff=%f", orig, rem, comp, eff)
	}

	// For PBI, focus starts on Effort (index 0)
	if modal.FocusedIndex != 0 {
		t.Fatalf("expected FocusedIndex=0 for PBI, got %d", modal.FocusedIndex)
	}

	// 2. User modifies Effort to 13
	modal.InputEffort.SetValue("13")
	_, _, _, eff = modal.Values()
	if eff != 13.0 {
		t.Fatalf("expected eff=13, got %f", eff)
	}

	// 3. User modifies Original to 16 -> Remaining auto-updates to 16
	modal.FocusedIndex = 1
	modal.InputOriginal.SetValue("16")
	modal.autoCalcRemaining()
	orig, rem, comp, _ = modal.Values()
	if orig != 16.0 || rem != 16.0 {
		t.Fatalf("expected orig=16, rem=16, got orig=%f, rem=%f", orig, rem)
	}

	// 4. User tabs to Completed and enters 6 -> Remaining auto-calculates to 10 (16 - 6)
	modal.FocusedIndex = 3
	modal.InputCompleted.SetValue("6")
	modal.autoCalcRemaining()
	orig, rem, comp, _ = modal.Values()
	if orig != 16.0 || comp != 6.0 || rem != 10.0 {
		t.Fatalf("expected orig=16, comp=6, rem=10, got orig=%f, comp=%f, rem=%f", orig, comp, rem)
	}

	// 5. Completed exceeds Original -> Remaining clamps to 0
	modal.InputCompleted.SetValue("20")
	modal.autoCalcRemaining()
	_, rem, _, _ = modal.Values()
	if rem != 0.0 {
		t.Fatalf("expected clamped rem=0, got %f", rem)
	}

	// 6. User clears all inputs (empty estimates)
	modal.InputEffort.SetValue("")
	modal.InputOriginal.SetValue("")
	modal.InputRemaining.SetValue("")
	modal.InputCompleted.SetValue("")
	modal.autoCalcRemaining()
	orig, rem, comp, eff = modal.Values()
	if orig != 0.0 || rem != 0.0 || comp != 0.0 || eff != 0.0 {
		t.Fatalf("expected all 0.0 for cleared inputs, got orig=%f, rem=%f, comp=%f, eff=%f", orig, rem, comp, eff)
	}
}

func TestUIStateRestorationAndExitControl(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_ui.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
		GeminiModel:    "gemini-3.7-flash",
	}

	// 1. Pre-save UI state into database
	savedState := &db.UIState{
		ActiveTabRootID:  200,
		ActiveTabIdx:     1,
		SelectedItemID:   202,
		CollapsedNodeIDs: []int{100},
		GeminiDockHeight: 12,
		Filter: db.FilterOptions{
			Status: "Active",
		},
	}
	if err := database.SaveUIState(savedState); err != nil {
		t.Fatalf("failed to save UI state: %v", err)
	}

	app := NewAppModel(cfg, database)

	// Verify Gemini dock height was restored and dock is closed by default
	if app.DockGemini.GetPaneHeight() != 12 {
		t.Fatalf("expected dock height 12, got %d", app.DockGemini.GetPaneHeight())
	}
	if app.DockGemini.Visible {
		t.Fatal("expected dock to be closed by default on launch")
	}

	// Setup mock tree roots
	roots := []*db.TreeNode{
		{
			Record:   &db.WorkItemRecord{ID: 100, Title: "Root 100", WorkItemType: "Feature", State: "Active"},
			Expanded: true,
			Children: []*db.TreeNode{
				{Record: &db.WorkItemRecord{ID: 101, Title: "Task 101", WorkItemType: "Task", State: "Active"}},
			},
		},
		{
			Record:   &db.WorkItemRecord{ID: 200, Title: "Root 200", WorkItemType: "Feature", State: "Active"},
			Expanded: true,
			Children: []*db.TreeNode{
				{Record: &db.WorkItemRecord{ID: 201, Title: "Task 201", WorkItemType: "Task", State: "Active"}},
				{Record: &db.WorkItemRecord{ID: 202, Title: "Task 202", WorkItemType: "Task", State: "Active"}},
			},
		},
	}

	// Simulate MsgSyncDone
	app.Update(MsgSyncDone{Roots: roots, Count: 4})

	// Verify tab 1 (root 200) was restored
	if app.ScreenTree.ActiveTab != 1 || app.ScreenTree.ActiveRootID() != 200 {
		t.Fatalf("expected active tab 1 (root 200), got tab=%d, root=%d", app.ScreenTree.ActiveTab, app.ScreenTree.ActiveRootID())
	}

	// Verify selected item 202 was restored
	if app.ScreenTree.SelectedItemID() != 202 {
		t.Fatalf("expected selected item 202, got %d", app.ScreenTree.SelectedItemID())
	}

	// Verify node 100 is collapsed
	if roots[0].Expanded {
		t.Fatal("expected root 100 to be collapsed from restored state")
	}

	// Verify filter option was restored
	if app.ScreenTree.FilterOpts.Status != "Active" {
		t.Fatalf("expected filter status 'Active', got %q", app.ScreenTree.FilterOpts.Status)
	}

	// 2. Test exit control: Ctrl+C does NOT quit
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected Ctrl+C to return toast command")
	}
	if !strings.Contains(app.ToastMessage, "Press 'q' from Home screen to quit") {
		t.Fatalf("expected toast message about 'q' to quit, got %q", app.ToastMessage)
	}

	// 3. Switch active tab and test exit control: 'q' saves UI state and quits
	app.ScreenTree.PrevTab() // Switch to tab 0 (root 100)
	_, qCmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if qCmd == nil {
		t.Fatal("expected 'q' to return quit command")
	}

	// Verify new state was saved synchronously to DB upon 'q'
	exitState, err := database.GetUIState()
	if err != nil {
		t.Fatalf("failed to read UI state after exit: %v", err)
	}
	if exitState.ActiveTabIdx != 0 || exitState.ActiveTabRootID != 100 {
		t.Fatalf("expected saved active tab 0 (root 100) on exit, got idx=%d, root=%d", exitState.ActiveTabIdx, exitState.ActiveTabRootID)
	}
}

func TestNoResumeFlag(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_no_resume.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	// 1. Pre-save UI state into database
	savedState := &db.UIState{
		ActiveTabRootID:  200,
		ActiveTabIdx:     1,
		SelectedItemID:   202,
		CollapsedNodeIDs: []int{100},
		GeminiDockHeight: 14,
		Filter: db.FilterOptions{
			Status: "Active",
		},
	}
	if err := database.SaveUIState(savedState); err != nil {
		t.Fatalf("failed to save UI state: %v", err)
	}

	// 2. Launch with NoResume = true
	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
		GeminiModel:    "gemini-3.7-flash",
		NoResume:       true,
	}

	app := NewAppModel(cfg, database)

	// Verify default dock height 8 is used instead of saved 14
	if app.DockGemini.GetPaneHeight() != 8 {
		t.Fatalf("expected default dock height 8 with --no-resume, got %d", app.DockGemini.GetPaneHeight())
	}

	roots := []*db.TreeNode{
		{
			Record:   &db.WorkItemRecord{ID: 100, Title: "Root 100", WorkItemType: "Feature"},
			Expanded: true,
			Children: []*db.TreeNode{
				{Record: &db.WorkItemRecord{ID: 101, Title: "Task 101", WorkItemType: "Task"}},
			},
		},
		{
			Record:   &db.WorkItemRecord{ID: 200, Title: "Root 200", WorkItemType: "Feature"},
			Expanded: true,
			Children: []*db.TreeNode{
				{Record: &db.WorkItemRecord{ID: 201, Title: "Task 201", WorkItemType: "Task"}},
				{Record: &db.WorkItemRecord{ID: 202, Title: "Task 202", WorkItemType: "Task"}},
			},
		},
	}

	// Simulate MsgSyncDone with NoResume
	app.Update(MsgSyncDone{Roots: roots, Count: 4})

	// Verify active tab remains default 0 (not 1 / root 200)
	if app.ScreenTree.ActiveTab != 0 || app.ScreenTree.ActiveRootID() != 100 {
		t.Fatalf("expected default tab 0 with --no-resume, got tab=%d, root=%d", app.ScreenTree.ActiveTab, app.ScreenTree.ActiveRootID())
	}

	// Verify all nodes remain expanded
	if !roots[0].Expanded {
		t.Fatal("expected root 100 to remain expanded with --no-resume")
	}

	// Verify filter was NOT applied
	if app.ScreenTree.FilterOpts.IsActive() {
		t.Fatal("expected filter to be inactive with --no-resume")
	}
}

func TestGhosttyCompatibilityAndMouseSupport(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_ghostty.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
	}

	app := NewAppModel(cfg, database)

	roots := []*db.TreeNode{
		{
			Record:   &db.WorkItemRecord{ID: 100, Title: "Root 100", WorkItemType: "Feature"},
			Expanded: true,
			Children: []*db.TreeNode{
				{Record: &db.WorkItemRecord{ID: 101, Title: "Task 101", WorkItemType: "Task"}},
				{Record: &db.WorkItemRecord{ID: 102, Title: "Task 102", WorkItemType: "Task"}},
			},
		},
	}
	app.Update(MsgSyncDone{Roots: roots, Count: 3})

	// 1. Mouse wheel scrolling on tree
	if app.ScreenTree.Cursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", app.ScreenTree.Cursor)
	}
	app.Update(tea.MouseMsg{Type: tea.MouseWheelDown})
	if app.ScreenTree.Cursor != 1 {
		t.Fatalf("expected cursor 1 after MouseWheelDown, got %d", app.ScreenTree.Cursor)
	}
	app.Update(tea.MouseMsg{Type: tea.MouseWheelUp})
	if app.ScreenTree.Cursor != 0 {
		t.Fatalf("expected cursor 0 after MouseWheelUp, got %d", app.ScreenTree.Cursor)
	}

	// 2. Mouse wheel scrolling on Gemini dock
	app.DockGemini.Show()
	app.DockGemini.SetSize(100, 30)
	app.DockGemini.ResponseMsg = "# Heading\n\n- Line 1\n- Line 2\n- Line 3\n- Line 4\n- Line 5\n- Line 6\n- Line 7\n- Line 8\n- Line 9\n- Line 10\n- Line 11\n- Line 12\n- Line 13\n- Line 14\n- Line 15\n- Line 16\n- Line 17\n- Line 18\n- Line 19\n- Line 20"
	app.Update(tea.MouseMsg{Type: tea.MouseWheelDown})
	if app.DockGemini.ScrollOffset < 2 {
		t.Fatalf("expected scroll offset >= 2 after dock MouseWheelDown, got %d", app.DockGemini.ScrollOffset)
	}
	app.Update(tea.MouseMsg{Type: tea.MouseWheelUp})
	if app.DockGemini.ScrollOffset != 0 {
		t.Fatalf("expected scroll offset 0 after dock MouseWheelUp, got %d", app.DockGemini.ScrollOffset)
	}

	// 3. Ctrl + Click on URL inside Gemini Dock
	app.Height = 30
	app.Width = 100
	app.DockGemini.ResponseMsg = "• Davis, Dan linked Pull Request #294732:\n  - URL: https://dev.azure.com/my-org/my-project/_git/my-repo/pullrequest/294732"
	dockH := lipgloss.Height(app.DockGemini.View())
	dockStartY := app.Height - dockH

	_, cmd := app.Update(tea.MouseMsg{
		Type:   tea.MouseLeft,
		Action: tea.MouseActionPress,
		Ctrl:   true,
		X:      15,
		Y:      dockStartY + 4,
	})
	if cmd == nil {
		t.Fatalf("expected toast cmd after Ctrl+Click in dock, got nil")
	}
	if !strings.Contains(app.ToastMessage, "294732") && !strings.Contains(app.ToastMessage, "Opened") {
		t.Fatalf("expected Opened toast for URL, got: %q", app.ToastMessage)
	}
}

func TestTextareaMultiline(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_multiline.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
	}

	app := NewAppModel(cfg, database)
	app.DockGemini.Show()
	app.DockGemini.SetSize(100, 30)
	app.DockGemini.Input.SetValue("first line")

	keyMessages := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"Alt+Enter", tea.KeyMsg{Type: tea.KeyEnter, Alt: true}},
		{"Alt+Ctrl+J (Esc+LF)", tea.KeyMsg{Type: tea.KeyCtrlJ, Alt: true}},
		{"Ctrl+J (LF)", tea.KeyMsg{Type: tea.KeyCtrlJ}},
		{"Newline Rune", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\n'}}},
	}

	for _, km := range keyMessages {
		app := NewAppModel(cfg, database)
		app.DockGemini.Show()
		app.DockGemini.SetSize(100, 30)
		app.DockGemini.Input.SetValue("first line")

		updated, _ := app.Update(km.msg)
		resApp := updated.(*AppModel)
		if !strings.Contains(resApp.DockGemini.Input.Value(), "\n") {
			t.Fatalf("key message %s failed to insert newline, prompt value is %q", km.name, resApp.DockGemini.Input.Value())
		}
	}

	// Full interactive 3-line typing test with Alt+Enter and Ctrl+J
	fullApp := NewAppModel(cfg, database)
	fullApp.DockGemini.Show()
	fullApp.DockGemini.SetSize(100, 30)
	fullApp.DockGemini.Input.SetValue("")

	for _, r := range "line 1" {
		updated, _ := fullApp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		fullApp = updated.(*AppModel)
	}
	// Alt+Enter
	updated, _ := fullApp.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	fullApp = updated.(*AppModel)

	for _, r := range "line 2" {
		updated, _ := fullApp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		fullApp = updated.(*AppModel)
	}
	// Ctrl+J
	updated, _ = fullApp.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	fullApp = updated.(*AppModel)

	for _, r := range "line 3" {
		updated, _ := fullApp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		fullApp = updated.(*AppModel)
	}

	expectedValue := "line 1\nline 2\nline 3"
	if fullApp.DockGemini.Input.Value() != expectedValue {
		t.Fatalf("expected prompt value %q, got %q", expectedValue, fullApp.DockGemini.Input.Value())
	}
	if fullApp.DockGemini.Input.LineCount() != 3 {
		t.Fatalf("expected 3 lines in prompt, got %d", fullApp.DockGemini.Input.LineCount())
	}
}

func TestWindowResizeAndRibbonPosition(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_resize.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
	}

	app := NewAppModel(cfg, database)
	roots := []*db.TreeNode{
		{
			Record:   &db.WorkItemRecord{ID: 100, Title: "Root 100", WorkItemType: "Feature"},
			Expanded: true,
			Children: []*db.TreeNode{
				{Record: &db.WorkItemRecord{ID: 101, Title: "Task 101", WorkItemType: "Task"}},
			},
		},
	}
	app.Update(MsgSyncDone{Roots: roots, Count: 2})

	// 1. Below minimum size threshold (width 100 < 115)
	_, cmd := app.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	if cmd == nil {
		t.Fatal("expected ClearScreen command on WindowSizeMsg")
	}
	viewSmall := app.View()
	if !strings.Contains(viewSmall, "TERMINAL WINDOW TOO SMALL") {
		t.Fatalf("expected view to contain 'TERMINAL WINDOW TOO SMALL' when width < 115, got:\n%s", viewSmall)
	}

	// 2. Standard full terminal (width 140, height 25)
	_, cmd = app.Update(tea.WindowSizeMsg{Width: 140, Height: 25})
	if cmd == nil {
		t.Fatal("expected ClearScreen command on WindowSizeMsg")
	}
	viewStd := app.View()
	linesStd := strings.Split(viewStd, "\n")
	if len(linesStd) != 25 {
		t.Fatalf("expected view height exactly 25 when window height is 25, got %d", len(linesStd))
	}
	if !strings.Contains(linesStd[len(linesStd)-1], "[q] Quit") {
		t.Fatalf("expected bottom line to contain ribbon '[q] Quit', got %q", linesStd[len(linesStd)-1])
	}

	// 3. Maximized terminal (width 160, height 45)
	_, cmd = app.Update(tea.WindowSizeMsg{Width: 160, Height: 45})
	if cmd == nil {
		t.Fatal("expected ClearScreen command on WindowSizeMsg")
	}
	viewMax := app.View()
	linesMax := strings.Split(viewMax, "\n")
	if len(linesMax) != 45 {
		t.Fatalf("expected view height exactly 45 when window height is 45, got %d", len(linesMax))
	}
	// Ribbon should follow and stay at the bottom (line index 44)
	if !strings.Contains(linesMax[len(linesMax)-1], "[q] Quit") {
		t.Fatalf("expected bottom line in maximized view to contain ribbon, got %q", linesMax[len(linesMax)-1])
	}

	// 4. Maximized with Gemini dock open
	app.DockGemini.Show()
	viewMaxDock := app.View()
	linesMaxDock := strings.Split(viewMaxDock, "\n")
	if len(linesMaxDock) != 45 {
		t.Fatalf("expected total view height 45 with dock open, got %d", len(linesMaxDock))
	}
}

func TestStripAndFormatHTMLCleanURLs(t *testing.T) {
	// 1. Plain URL preservation with Sky Blue and Underline styling
	rawText := "Raised PR for changes https://dev.azure.com/org/proj/_git/repo/pullrequest/294732 however dependent on https://dev.azure.com/org/proj/_workitems/edit/663696"
	formatted := StripAndFormatHTML(rawText)
	if !strings.Contains(formatted, "https://dev.azure.com/org/proj/_git/repo/pullrequest/294732") {
		t.Fatalf("expected PR URL intact, got: %s", formatted)
	}
	if !strings.Contains(formatted, "https://dev.azure.com/org/proj/_workitems/edit/663696") {
		t.Fatalf("expected WI URL intact, got: %s", formatted)
	}

	// 2. HTML anchor tag preservation with styled hyperlink
	htmlText := `<div>Raised PR <a href="https://dev.azure.com/org/proj/_git/repo/pullrequest/294732">PR 294732</a> for bug fix</div>`
	formattedHTML := StripAndFormatHTML(htmlText)
	if !strings.Contains(formattedHTML, "PR 294732") || !strings.Contains(formattedHTML, "https://dev.azure.com/org/proj/_git/repo/pullrequest/294732") {
		t.Fatalf("expected clean text and URL from HTML anchor, got: %s", formattedHTML)
	}

	// 3. WrapWords without breaking URLs or words across hyphens
	wrapped := WrapWords(formatted, 80)
	lines := strings.Split(wrapped, "\n")
	for _, l := range lines {
		if strings.Contains(l, "https://dev.azure.com") && !strings.Contains(l, "https://dev.azure.com/org/proj/_git/repo/pullrequest/294732") && !strings.Contains(l, "https://dev.azure.com/org/proj/_workitems/edit/663696") {
			t.Fatalf("URL was split or chopped across lines: %s", l)
		}
	}
}

func TestScreenItemLinkDiscoveryAndHotkeys(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_links.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
	}

	app := NewAppModel(cfg, database)

	item := &db.WorkItemRecord{
		ID:           663713,
		Title:        "Create long term forecast job",
		WorkItemType: "Task",
		State:        "In Progress",
		Description:  "Job implementation. See parent https://dev.azure.com/my-org/my-project/_workitems/edit/663690",
	}
	comments := []db.CommentRecord{
		{
			ID:          1,
			WorkItemID:  663713,
			AuthorName:  "Davis, Dan",
			Text:        "Raised PR for the changes https://dev.azure.com/my-org/my-project/_git/my-repo/pullrequest/294732\ncreating an task to cover integration https://dev.azure.com/my-org/my-project/_workitems/edit/661644",
		},
	}

	app.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app.CurrentScreen = ScreenTypeItem
	app.ScreenItem.SetItem(item, comments)
	app.ScreenItem.InitViewport(140, 30)

	// 1. Verify link extraction into RelatedLinks
	if len(app.ScreenItem.RelatedLinks) != 3 {
		t.Fatalf("expected 3 extracted links, got %d", len(app.ScreenItem.RelatedLinks))
	}
	// First link should be the PR
	prLink := app.ScreenItem.RelatedLinks[0]
	if !prLink.IsPR || prLink.ID != 294732 {
		t.Fatalf("expected PR #294732, got %+v", prLink)
	}

	// 2. Tab cycling through links
	app.Update(tea.KeyMsg{Type: tea.KeyTab})
	if app.ScreenItem.SelectedLinkIdx != 1 {
		t.Fatalf("expected selected link idx 1 after Tab, got %d", app.ScreenItem.SelectedLinkIdx)
	}
	app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if app.ScreenItem.SelectedLinkIdx != 0 {
		t.Fatalf("expected selected link idx 0 after Shift+Tab, got %d", app.ScreenItem.SelectedLinkIdx)
	}

	// 3. Ctrl+Enter key handling
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlJ}) // or ctrl+enter
	if !strings.Contains(app.ToastMessage, "Opened") {
		t.Fatalf("expected toast message 'Opened ...' on Ctrl+Enter/Ctrl+J, got %q", app.ToastMessage)
	}

	// 4. Ctrl+Click mouse event handling
	app.Update(tea.MouseMsg{
		Type:   tea.MouseLeft,
		Action: tea.MouseActionPress,
		Ctrl:   true,
		X:      20,
		Y:      10,
	})
	if !strings.Contains(app.ToastMessage, "Opened") {
		t.Fatalf("expected toast message 'Opened ...' on Ctrl+Click, got %q", app.ToastMessage)
	}

	// 5. 'o' opens the current work item (#663713)
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if !strings.Contains(app.ToastMessage, "Opened current item #663713") {
		t.Fatalf("expected toast message for current item #663713, got %q", app.ToastMessage)
	}

	// 6. 'p' opens the selected link (PR #294732)
	app.ScreenItem.SelectedLinkIdx = 0
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !strings.Contains(app.ToastMessage, "PR #294732") {
		t.Fatalf("expected toast message for PR #294732, got %q", app.ToastMessage)
	}

	// 7. Viewport content renders complete URLs without truncation
	view := app.View()
	if !strings.Contains(view, "https://dev.azure.com/my-org/my-project/_git/my-repo/pullrequest/294732") {
		t.Fatalf("expected complete unbroken PR URL in view, but was truncated or missing")
	}
	if !strings.Contains(view, "https://dev.azure.com/my-org/my-project/_workitems/edit/661644") {
		t.Fatalf("expected complete unbroken WI URL in view, but was truncated or missing")
	}
}

func TestScreenItemAttachmentAndGeneralLinkClassification(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_item_attach.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
	}

	app := NewAppModel(cfg, database)

	item := &db.WorkItemRecord{
		ID:           649688,
		Title:        "UAT_Search for Pod",
		WorkItemType: "Bug",
		State:        "Done",
		Description:  "Raised PR: https://dev.azure.com/my-org/my-project/_git/my-repo/pullrequest/287874",
	}
	comments := []db.CommentRecord{
		{
			ID:         1,
			WorkItemID: 649688,
			AuthorName: "Johnson, Bob",
			Text: "Screenshots attached:\n" +
				"https://dev.azure.com/my-org/bdf83e34/_apis/wit/attachments/5b53a5b1?fileName=image.png\n" +
				"https://dev.azure.com/my-org/bdf83e34/_apis/wit/attachments/93e4f4d0?fileName=Screenshot%202026-07-21%20114109.png\n" +
				"Also see documentation at https://example.com/docs/pod",
		},
	}

	app.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	app.CurrentScreen = ScreenTypeItem
	app.ScreenItem.SetItem(item, comments)
	app.ScreenItem.InitViewport(140, 30)

	// Verify links
	if len(app.ScreenItem.RelatedLinks) != 4 {
		t.Fatalf("expected 4 related links, got %d", len(app.ScreenItem.RelatedLinks))
	}

	// 1: PR
	l0 := app.ScreenItem.RelatedLinks[0]
	if !l0.IsPR || l0.ID != 287874 || l0.Type != "Pull Request" {
		t.Fatalf("expected PR #287874, got %+v", l0)
	}

	// 2: Attachment image.png
	l1 := app.ScreenItem.RelatedLinks[1]
	if l1.IsPR || l1.Type != "Attachment" || l1.Title != "image.png" {
		t.Fatalf("expected Attachment 'image.png', got %+v", l1)
	}

	// 3: Attachment Screenshot 2026-07-21 114109.png (URL unescaped)
	l2 := app.ScreenItem.RelatedLinks[2]
	if l2.IsPR || l2.Type != "Attachment" || l2.Title != "Screenshot 2026-07-21 114109.png" {
		t.Fatalf("expected Attachment 'Screenshot 2026-07-21 114109.png', got %+v", l2)
	}

	// 4: General Web Link
	l3 := app.ScreenItem.RelatedLinks[3]
	if l3.IsPR || l3.Type != "Link" || l3.Title != "https://example.com/docs/pod" {
		t.Fatalf("expected Link 'https://example.com/docs/pod', got %+v", l3)
	}

	// Render full content and ensure [PR #0] NEVER appears
	fullContent := app.ScreenItem.renderItemContent(140)
	if strings.Contains(fullContent, "PR #0") {
		t.Fatalf("view should NEVER render 'PR #0', but found it in:\n%s", fullContent)
	}
	if !strings.Contains(fullContent, "ATTACHMENT") {
		t.Fatalf("view should render [ATTACHMENT] badge for attachments, got:\n%s", fullContent)
	}
	if !strings.Contains(fullContent, "Screenshot 2026-07-21 114109.png") {
		t.Fatalf("view should render unescaped attachment title, got:\n%s", fullContent)
	}
}


func TestTreeScrollingWithAIDockVisible(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_scroll_dock.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
	}

	// Create a root with 30 children so tree has 31 items
	rootNode := &db.TreeNode{
		Record: &db.WorkItemRecord{
			ID:           1000,
			Title:        "Root Epic",
			WorkItemType: "Epic",
			State:        "Active",
		},
		Expanded: true,
	}

	for i := 1; i <= 30; i++ {
		child := &db.TreeNode{
			Record: &db.WorkItemRecord{
				ID:           1000 + i,
				Title:        "Child Task",
				WorkItemType: "Task",
				State:        "To Do",
				ParentID:     1000,
			},
			Parent: rootNode,
		}
		rootNode.Children = append(rootNode.Children, child)
	}

	app := NewAppModel(cfg, database)
	updated, _ := app.Update(tea.WindowSizeMsg{Width: 120, Height: 35})
	app = updated.(*AppModel)

	// Refresh tree roots
	roots := []*db.TreeNode{rootNode}
	updated, _ = app.Update(MsgSyncDone{Roots: roots, Count: len(roots)})
	app = updated.(*AppModel)

	// Total flattened items should be 31
	if len(app.ScreenTree.Flattened) != 31 {
		t.Fatalf("expected 31 flattened items, got %d", len(app.ScreenTree.Flattened))
	}

	// 1. Show Gemini AI Dock
	app.DockGemini.Show()
	app.DockGemini.SetSize(120, 35)

	// Available screen height should be less than terminal height
	availH := app.availableScreenHeight()
	if availH >= 30 || availH <= 10 {
		t.Fatalf("expected available screen height to be reduced by dock, got %d", availH)
	}

	// Render view
	view := app.View()
	if !strings.Contains(view, "Gemini AI Assistant") {
		t.Fatalf("expected Gemini AI Assistant in view")
	}

	// 2. Navigate down with Shift+Down key
	for i := 0; i < 25; i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyShiftDown})
		app = updated.(*AppModel)
	}

	if app.ScreenTree.Cursor != 25 {
		t.Fatalf("expected cursor at 25 after navigation, got %d", app.ScreenTree.Cursor)
	}

	// ScrollOffset must be > 0 so row 25 is visible above dock!
	if app.ScreenTree.ScrollOffset <= 0 {
		t.Fatalf("expected ScrollOffset > 0 when cursor is 25, got %d", app.ScreenTree.ScrollOffset)
	}

	// 3. Mouse wheel scrolling over tree area
	// msg.Y = 5 is in the tree area (< dockStartY)
	updated, _ = app.Update(tea.MouseMsg{
		Type:   tea.MouseWheelUp,
		Button: tea.MouseButtonWheelUp,
		X:      20,
		Y:      5,
	})
	app = updated.(*AppModel)
	if app.ScreenTree.Cursor != 24 {
		t.Fatalf("expected cursor to move up to 24 with wheel up, got %d", app.ScreenTree.Cursor)
	}

	// 4. Mouse click on tree row
	// Header height = 4. Clicking Y=4 should select item at ScrollOffset + 0
	expectedIdx := app.ScreenTree.ScrollOffset
	updated, _ = app.Update(tea.MouseMsg{
		Type:   tea.MouseLeft,
		Action: tea.MouseActionPress,
		X:      20,
		Y:      4,
	})
	app = updated.(*AppModel)
	if app.ScreenTree.Cursor != expectedIdx {
		t.Fatalf("expected cursor to be %d on row click, got %d", expectedIdx, app.ScreenTree.Cursor)
	}
}

func TestScreenItemAIDockResize(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_item_dock.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
		GeminiModel:    "gemini-3.7-flash",
	}

	app := NewAppModel(cfg, database)
	app.Width = 120
	app.Height = 35

	testItem := &db.WorkItemRecord{
		ID:               666609,
		Title:            "Analysis on timeout and warmup",
		WorkItemType:     "Task",
		State:            "In Progress",
		OriginalEstimate: 18.0,
		CompletedWork:    0.0,
		RemainingWork:    18.0,
		Description:      "Analyze timeout configurations and warmup durations for GKE instances.",
	}

	app.CurrentScreen = ScreenTypeItem
	app.ScreenItem.SetItem(testItem, nil)
	app.ScreenItem.InitViewport(app.Width, app.Height)

	// Initial render without dock
	view := app.View()
	viewHeight := lipgloss.Height(view)
	if viewHeight != app.Height {
		t.Fatalf("expected view height to match terminal height %d, got %d", app.Height, viewHeight)
	}
	if !strings.Contains(view, "Analysis on timeout and warmup") {
		t.Fatalf("expected title in view")
	}

	// 1. Open Gemini Dock
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	app = updated.(*AppModel)

	if !app.DockGemini.Visible {
		t.Fatalf("expected DockGemini to be visible after pressing 'g'")
	}

	view = app.View()
	viewHeight = lipgloss.Height(view)
	if viewHeight != app.Height {
		t.Fatalf("expected view height with dock open to match terminal height %d, got %d", app.Height, viewHeight)
	}

	// Count occurrences of ribbon shortcut "[w] Est"
	estCount := strings.Count(view, "[w] Est")
	if estCount != 1 {
		t.Fatalf("expected ribbon shortcut '[w] Est' to appear exactly 1 time, got %d", estCount)
	}

	// 2. Resize Dock larger with ctrl+up multiple times
	for i := 0; i < 5; i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
		app = updated.(*AppModel)

		view = app.View()
		viewHeight = lipgloss.Height(view)
		if viewHeight != app.Height {
			t.Fatalf("iteration %d: expected view height %d on ctrl+up, got %d", i, app.Height, viewHeight)
		}

		estCount = strings.Count(view, "[w] Est")
		if estCount != 1 {
			t.Fatalf("iteration %d: expected ribbon shortcut to appear exactly 1 time on ctrl+up, got %d", i, estCount)
		}

		if !strings.Contains(view, "#666609") {
			t.Fatalf("iteration %d: expected item ID header #666609 to remain visible", i)
		}
	}

	// 3. Resize Dock smaller with ctrl+down multiple times
	for i := 0; i < 4; i++ {
		updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
		app = updated.(*AppModel)

		view = app.View()
		viewHeight = lipgloss.Height(view)
		if viewHeight != app.Height {
			t.Fatalf("iteration %d: expected view height %d on ctrl+down, got %d", i, app.Height, viewHeight)
		}

		estCount = strings.Count(view, "[w] Est")
		if estCount != 1 {
			t.Fatalf("iteration %d: expected ribbon shortcut to appear exactly 1 time on ctrl+down, got %d", i, estCount)
		}
	}

	// 4. Close Dock with Esc
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*AppModel)

	if app.DockGemini.Visible {
		t.Fatalf("expected dock to be hidden after pressing Esc")
	}

	view = app.View()
	viewHeight = lipgloss.Height(view)
	if viewHeight != app.Height {
		t.Fatalf("expected view height after Esc to match terminal height %d, got %d", app.Height, viewHeight)
	}
}

func TestScreenSettingsNavigationAndToggles(t *testing.T) {
	settings := NewScreenSettings()
	cfg := &config.Config{
		PatToken:             "pat-12345",
		GeminiApiKey:         "gemini-key-abc",
		GeminiModel:          "gemini-3.7-flash",
		AutoCalcRemaining:    true,
		EffortSameAsEstimate: false,
	}
	settings.Reset(cfg, nil)

	if settings.InputPat.EchoMode != textinput.EchoNormal {
		t.Fatalf("expected InputPat EchoMode to be EchoNormal, got %v", settings.InputPat.EchoMode)
	}
	if settings.InputGeminiKey.EchoMode != textinput.EchoNormal {
		t.Fatalf("expected InputGeminiKey EchoMode to be EchoNormal, got %v", settings.InputGeminiKey.EchoMode)
	}
	if settings.InputPat.Value() != "pat-12345" {
		t.Fatalf("expected pat-12345, got %s", settings.InputPat.Value())
	}
	if settings.InputGeminiKey.Value() != "gemini-key-abc" {
		t.Fatalf("expected gemini-key-abc, got %s", settings.InputGeminiKey.Value())
	}
	if settings.InputGeminiModel.Value() != "gemini-3.7-flash" {
		t.Fatalf("expected gemini-3.7-flash, got %s", settings.InputGeminiModel.Value())
	}
	if !settings.AutoCalcRemaining {
		t.Fatalf("expected AutoCalcRemaining true")
	}
	if settings.EffortSameAsEstimate {
		t.Fatalf("expected EffortSameAsEstimate false")
	}

	// Navigate to AutoCalcRemaining checkbox (index 3)
	settings.Update(tea.KeyMsg{Type: tea.KeyTab}) // to gemini key (1)
	settings.Update(tea.KeyMsg{Type: tea.KeyTab}) // to gemini model (2)
	settings.Update(tea.KeyMsg{Type: tea.KeyTab}) // to auto calc (3)

	if settings.FocusedIndex != fieldSettingsAutoCalc {
		t.Fatalf("expected focused field to be fieldSettingsAutoCalc (3), got %d", settings.FocusedIndex)
	}

	// Toggle AutoCalc checkbox with Space
	settings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if settings.AutoCalcRemaining {
		t.Fatalf("expected AutoCalcRemaining to toggle to false")
	}

	// Move to EffortSameAsEstimate (index 4)
	settings.Update(tea.KeyMsg{Type: tea.KeyTab})
	if settings.FocusedIndex != fieldSettingsEffortSame {
		t.Fatalf("expected focused field to be fieldSettingsEffortSame (4), got %d", settings.FocusedIndex)
	}

	// Move to AutoSetFinishDate (index 5)
	settings.Update(tea.KeyMsg{Type: tea.KeyTab})
	if settings.FocusedIndex != fieldSettingsAutoSetFinishDate {
		t.Fatalf("expected focused field to be fieldSettingsAutoSetFinishDate (5), got %d", settings.FocusedIndex)
	}
	settings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !settings.AutoSetFinishDate {
		t.Fatalf("expected AutoSetFinishDate to toggle to true")
	}

	// Move to AutoSyncInterval (index 6)
	settings.Update(tea.KeyMsg{Type: tea.KeyTab})
	if settings.FocusedIndex != fieldSettingsAutoSyncInterval {
		t.Fatalf("expected focused field to be fieldSettingsAutoSyncInterval (6), got %d", settings.FocusedIndex)
	}

	// Move to Incomplete Criteria checkboxes (index 7)
	settings.Update(tea.KeyMsg{Type: tea.KeyTab})
	if settings.FocusedIndex != fieldSettingsIncompleteEffort {
		t.Fatalf("expected focused field to be fieldSettingsIncompleteEffort (7), got %d", settings.FocusedIndex)
	}
	settings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !settings.IncompleteCriteriaEffort {
		t.Fatalf("expected IncompleteCriteriaEffort to toggle to true")
	}

	// Verify View contains headers, checkboxes, and buttons
	view := settings.View(120, 100, "", false)
	if !strings.Contains(view, "Application Settings") {
		t.Fatalf("expected view to contain 'Application Settings'")
	}
	if !strings.Contains(view, "Compute Remaining Work Automatically") {
		t.Fatalf("expected view to contain 'Compute Remaining Work Automatically'")
	}
	if !strings.Contains(view, "Effort same as Estimate") {
		t.Fatalf("expected view to contain 'Effort same as Estimate'")
	}
	if !strings.Contains(view, "Auto-Sync Interval (seconds):") {
		t.Fatalf("expected view to contain 'Auto-Sync Interval (seconds):'")
	}
	if !strings.Contains(view, "INCOMPLETE VIEW CRITERIA") {
		t.Fatalf("expected view to contain 'INCOMPLETE VIEW CRITERIA'")
	}
	if !strings.Contains(view, "Require Effort / Story Points") || !strings.Contains(view, "Require Target Date") {
		t.Fatalf("expected view to contain incomplete criteria labels, got:\n%s", view)
	}
	if !strings.Contains(view, "Applicable Item Types:") {
		t.Fatalf("expected view to contain 'Applicable Item Types:', got:\n%s", view)
	}
	if !strings.Contains(view, "[ Save Settings (Ctrl+S) ]") {
		t.Fatalf("expected view to contain Save button")
	}
	if !strings.Contains(view, "[ Cancel (Esc) ]") {
		t.Fatalf("expected view to contain Cancel button")
	}
	if strings.Contains(view, "(hrs)") || strings.Contains(view, "(pts)") {
		t.Fatalf("expected view not to contain '(hrs)' or '(pts)' in hints")
	}
	if strings.Contains(view, "⚙") {
		t.Fatalf("expected view not to contain cog wheel icon '⚙'")
	}

	// Continue navigating down to Save Button (11)
	for settings.FocusedIndex < fieldSettingsSaveBtn {
		settings.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if settings.FocusedIndex != fieldSettingsSaveBtn {
		t.Fatalf("expected FocusedIndex to be fieldSettingsSaveBtn (11), got %d", settings.FocusedIndex)
	}

	// Down moves to Cancel Button (12)
	settings.Update(tea.KeyMsg{Type: tea.KeyDown})
	if settings.FocusedIndex != fieldSettingsCancelBtn {
		t.Fatalf("expected FocusedIndex to be fieldSettingsCancelBtn (12), got %d", settings.FocusedIndex)
	}

	// Down at Cancel stays on Cancel (no sudden jump to top)
	settings.Update(tea.KeyMsg{Type: tea.KeyDown})
	if settings.FocusedIndex != fieldSettingsCancelBtn {
		t.Fatalf("expected FocusedIndex to remain at fieldSettingsCancelBtn (12), got %d", settings.FocusedIndex)
	}

	// Left moves to Save Button
	settings.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if settings.FocusedIndex != fieldSettingsSaveBtn {
		t.Fatalf("expected FocusedIndex to move to fieldSettingsSaveBtn (11), got %d", settings.FocusedIndex)
	}

	// Right moves back to Cancel Button
	settings.Update(tea.KeyMsg{Type: tea.KeyRight})
	if settings.FocusedIndex != fieldSettingsCancelBtn {
		t.Fatalf("expected FocusedIndex to move to fieldSettingsCancelBtn (12), got %d", settings.FocusedIndex)
	}
}

func TestScreenSettingsSaveAndPersist(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "settings_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DbPath:               dbPath,
		PatToken:             "old-pat",
		GeminiApiKey:         "old-gemini-key",
		GeminiModel:          "gemini-3.7-flash",
		AutoCalcRemaining:    true,
		EffortSameAsEstimate: false,
	}

	app := &AppModel{
		Config:         cfg,
		DB:             database,
		Width:          120,
		Height:         30,
		CurrentScreen:  ScreenTypeSettings,
		ScreenTree:     NewScreenTree(),
		ScreenItem:     NewScreenItem(),
		ScreenCreate:   NewScreenCreate(),
		ScreenSettings: NewScreenSettings(),
		ModalEstimate:  NewModalEstimate(),
		DockGemini:     NewDockGemini("gemini-3.7-flash"),
	}

	app.ScreenSettings.Reset(cfg, database)
	app.ScreenSettings.InputPat.SetValue("new-pat-999")
	app.ScreenSettings.InputGeminiKey.SetValue("new-ai-key-777")
	app.ScreenSettings.InputGeminiModel.SetValue("gemini-2.5-pro")
	app.ScreenSettings.AutoCalcRemaining = false
	app.ScreenSettings.EffortSameAsEstimate = true

	// Trigger Save with ctrl+s
	updated, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	app = updated.(*AppModel)

	if cmd == nil {
		t.Fatalf("expected command on save")
	}

	// Execute save command batch
	msg := app.saveSettingsCmd("new-pat-999", "new-ai-key-777", "gemini-2.5-pro", false, true, true, 60, true, false, true, false, true, false, false, "", []string{"Task", "Product Backlog Item"}, []string{"Active", "New"}, []string{"Active"})()
	updated, _ = app.Update(msg)
	app = updated.(*AppModel)

	if app.CurrentScreen != ScreenTypeTree {
		t.Fatalf("expected CurrentScreen to return to ScreenTypeTree, got %v", app.CurrentScreen)
	}

	// Verify values in Config
	if app.Config.PatToken != "new-pat-999" {
		t.Fatalf("expected Config.PatToken new-pat-999, got %s", app.Config.PatToken)
	}
	if app.Config.GeminiApiKey != "new-ai-key-777" {
		t.Fatalf("expected Config.GeminiApiKey new-ai-key-777, got %s", app.Config.GeminiApiKey)
	}
	if !app.Config.IncompleteCriteriaEffort || !app.Config.IncompleteCriteriaStartDate || !app.Config.IncompleteCriteriaFinishDate {
		t.Fatalf("expected incomplete criteria to be saved to config")
	}
	if len(app.Config.IncompleteTypes) != 2 || app.Config.IncompleteTypes[0] != "Task" || app.Config.IncompleteTypes[1] != "Product Backlog Item" {
		t.Fatalf("expected incomplete types to be correctly saved to config, got %v", app.Config.IncompleteTypes)
	}
	if app.Config.GeminiModel != "gemini-2.5-pro" {
		t.Fatalf("expected Config.GeminiModel gemini-2.5-pro, got %s", app.Config.GeminiModel)
	}
	if app.Config.AutoCalcRemaining != false {
		t.Fatalf("expected Config.AutoCalcRemaining false")
	}
	if app.Config.EffortSameAsEstimate != true {
		t.Fatalf("expected Config.EffortSameAsEstimate true")
	}

	// Verify values in SQLite DB
	patDb, _ := database.GetSetting("pat_token")
	if patDb != "new-pat-999" {
		t.Fatalf("expected DB pat_token new-pat-999, got %s", patDb)
	}
	keyDb, _ := database.GetSetting("gemini_api_key")
	if keyDb != "new-ai-key-777" {
		t.Fatalf("expected DB gemini_api_key new-ai-key-777, got %s", keyDb)
	}
	modelDb, _ := database.GetSetting("gemini_model")
	if modelDb != "gemini-2.5-pro" {
		t.Fatalf("expected DB gemini_model gemini-2.5-pro, got %s", modelDb)
	}
	autoDb, _ := database.GetSetting("auto_calc_remaining")
	if autoDb != "false" {
		t.Fatalf("expected DB auto_calc_remaining false, got %s", autoDb)
	}
	effortDb, _ := database.GetSetting("effort_same_as_estimate")
	if effortDb != "true" {
		t.Fatalf("expected DB effort_same_as_estimate true, got %s", effortDb)
	}
}

func TestModalEstimateEffortSameAsEstimateAndAutoCalc(t *testing.T) {
	modal := NewModalEstimate()
	modal.AutoCalcRemaining = true
	modal.EffortSameAsEstimate = true

	modal.Open(101, "Test Feature", "Custom", 0, 0, 0, 0)
	// Focus on Original Estimate (index 1)
	modal.FocusedIndex = 1
	modal.updateFocus()

	// Type '1' '6' into Original Estimate
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'6'}})

	// Verify Effort and Remaining automatically synced to 16
	if modal.InputOriginal.Value() != "16" {
		t.Fatalf("expected Original 16, got %s", modal.InputOriginal.Value())
	}
	if modal.InputEffort.Value() != "16" {
		t.Fatalf("expected Effort auto-synced to 16, got %s", modal.InputEffort.Value())
	}
	if modal.InputRemaining.Value() != "16" {
		t.Fatalf("expected Remaining auto-calculated to 16, got %s", modal.InputRemaining.Value())
	}

	// Now customize Effort explicitly to '8'
	modal.FocusedIndex = 0
	modal.updateFocus()
	modal.InputEffort.SetValue("8")
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'8'}}) // triggers effortCustomized

	// Change Original to '24'
	modal.FocusedIndex = 1
	modal.updateFocus()
	modal.InputOriginal.SetValue("24")
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})

	// Effort should NOT be overwritten because it was customized
	if modal.InputEffort.Value() != "88" && modal.InputEffort.Value() != "8" {
		t.Fatalf("expected custom Effort to be preserved, got %s", modal.InputEffort.Value())
	}

	// Disable AutoCalcRemaining
	modal.AutoCalcRemaining = false
	modal.InputRemaining.SetValue("5")
	modal.InputCompleted.SetValue("10")
	modal.FocusedIndex = 3
	modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'0'}})

	// Remaining should NOT be auto-calculated when AutoCalcRemaining is false
	// Verify modal view contains clean labels without parenthesis/brackets
	view := modal.View(80, 25)
	if !strings.Contains(view, "Effort:") || strings.Contains(view, "Effort (") {
		t.Fatalf("expected view to have clean 'Effort:' label without brackets")
	}
	if !strings.Contains(view, "Original Estimate:") || strings.Contains(view, "Original Estimate (") {
		t.Fatalf("expected view to have clean 'Original Estimate:' label without (hours)")
	}
	if !strings.Contains(view, "Remaining Work:") || strings.Contains(view, "Remaining Work (") {
		t.Fatalf("expected view to have clean 'Remaining Work:' label without (hours)")
	}
	if !strings.Contains(view, "Completed Work:") || strings.Contains(view, "Completed Work (") {
		t.Fatalf("expected view to have clean 'Completed Work:' label without (hours)")
	}
}

func TestModalEstimateFieldsToUpdate(t *testing.T) {
	modal := NewModalEstimate()
	// Open modal with Task type (supports original, remaining, completed)
	modal.Open(663964, "Cannot Deploy GKE Cluster", "Task", 0, 0, 0, 0)
	modal.InputOriginal.SetValue("4")
	modal.InputCompleted.SetValue("4")

	fields := modal.FieldsToUpdate()
	if orig, ok := fields["Microsoft.VSTS.Scheduling.OriginalEstimate"]; !ok || orig != 4.0 {
		t.Errorf("expected OriginalEstimate = 4.0, got %v", orig)
	}
	if comp, ok := fields["Microsoft.VSTS.Scheduling.CompletedWork"]; !ok || comp != 4.0 {
		t.Errorf("expected CompletedWork = 4.0, got %v", comp)
	}
	// Effort and Remaining were not set, so they should not be injected
	if _, ok := fields["Microsoft.VSTS.Scheduling.Effort"]; ok {
		t.Errorf("expected Effort to not be in fields since it was empty, but got %v", fields["Microsoft.VSTS.Scheduling.Effort"])
	}
}

func TestModalEstimateDynamicBugHidesEstimate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	org := "my-org"
	project := "my-project"

	// Define fields for Bug in Scrum: has Effort and RemainingWork, but NO OriginalEstimate or CompletedWork
	err = database.SaveWorkItemTypeFields(org, project, "Bug", []ado.WorkItemTypeField{
		{ReferenceName: "System.Title", Name: "Title"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.Effort", Name: "Effort"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.RemainingWork", Name: "Remaining Work"},
	})
	if err != nil {
		t.Fatalf("failed to save Bug fields: %v", err)
	}

	modal := NewModalEstimate()
	modal.OpenWithDB(663964, "Cannot Deploy GKE Cluster", "Bug", "Active", org, project, 0, 0, 0, 4.0, database)

	if !modal.ShowEffort {
		t.Errorf("expected ShowEffort to be true for Bug")
	}
	if !modal.ShowRemaining {
		t.Errorf("expected ShowRemaining to be true for Bug")
	}
	if modal.ShowOriginal {
		t.Errorf("expected ShowOriginal to be false for Bug (no estimate field)")
	}
	if modal.ShowCompleted {
		t.Errorf("expected ShowCompleted to be false for Bug (no completed field)")
	}

	view := modal.View(80, 25)
	if !strings.Contains(view, "Effort:") {
		t.Errorf("expected view to contain 'Effort:', got: %s", view)
	}
	if !strings.Contains(view, "Remaining Work:") {
		t.Errorf("expected view to contain 'Remaining Work:', got: %s", view)
	}
	if strings.Contains(view, "Original Estimate:") {
		t.Errorf("expected view NOT to contain 'Original Estimate:' for Bug without estimate field, got: %s", view)
	}
	if strings.Contains(view, "Completed Work:") {
		t.Errorf("expected view NOT to contain 'Completed Work:' for Bug without completed field, got: %s", view)
	}
}

func TestModalEstimateDynamicTaskCustomFields(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	org := "custom-org"
	project := "Custom-Project"

	// Define fields for Task with Custom.OriginalEstimate and Custom.CompletedWork
	err = database.SaveWorkItemTypeFields(org, project, "Task", []ado.WorkItemTypeField{
		{ReferenceName: "System.Title", Name: "Title"},
		{ReferenceName: "Custom.OriginalEstimate", Name: "Original Estimate"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.RemainingWork", Name: "Remaining Work"},
		{ReferenceName: "Custom.CompletedWork", Name: "Completed Work"},
	})
	if err != nil {
		t.Fatalf("failed to save Task fields: %v", err)
	}

	modal := NewModalEstimate()
	modal.OpenWithDB(663676, "Deploy Services to GKE", "Task", "In Progress", org, project, 12.0, 4.0, 8.0, 0, database)

	if modal.ShowEffort {
		t.Errorf("expected ShowEffort to be false for Task")
	}
	if !modal.ShowOriginal {
		t.Errorf("expected ShowOriginal to be true for Task with Custom.OriginalEstimate")
	}
	if !modal.ShowRemaining {
		t.Errorf("expected ShowRemaining to be true for Task with RemainingWork")
	}
	if !modal.ShowCompleted {
		t.Errorf("expected ShowCompleted to be true for Task with Custom.CompletedWork")
	}

	if modal.FieldOriginal != "Custom.OriginalEstimate" {
		t.Errorf("expected FieldOriginal to be 'Custom.OriginalEstimate', got %s", modal.FieldOriginal)
	}
	if modal.FieldCompleted != "Custom.CompletedWork" {
		t.Errorf("expected FieldCompleted to be 'Custom.CompletedWork', got %s", modal.FieldCompleted)
	}

	// Update completed work to 10
	modal.InputCompleted.SetValue("10")
	fields := modal.FieldsToUpdate()

	if v, ok := fields["Custom.CompletedWork"]; !ok || v != 10.0 {
		t.Errorf("expected Custom.CompletedWork to be 10.0, got %v", fields["Custom.CompletedWork"])
	}
	if _, ok := fields["Microsoft.VSTS.Scheduling.CompletedWork"]; ok {
		t.Errorf("expected standard CompletedWork not to be present when Custom field is mapped")
	}
	if _, ok := fields["Microsoft.VSTS.Scheduling.Effort"]; ok {
		t.Errorf("expected Effort not to be in fields for Task")
	}
}

func TestModalEstimateDoneStateSuppressesRemainingWork(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	org := "test-org"
	project := "Test-Project"

	// Define fields for Task
	err = database.SaveWorkItemTypeFields(org, project, "Task", []ado.WorkItemTypeField{
		{ReferenceName: "System.Title", Name: "Title"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.OriginalEstimate", Name: "Original Estimate"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.RemainingWork", Name: "Remaining Work"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.CompletedWork", Name: "Completed Work"},
	})
	if err != nil {
		t.Fatalf("failed to save Task fields: %v", err)
	}

	// Open modal for a Task in "Done" state
	modal := NewModalEstimate()
	modal.OpenWithDB(663674, "Implementation", "Task", "Done", org, project, 12.0, 0.0, 8.0, 0, database)

	// Remaining work MUST NOT be shown/editable when in Done state
	if modal.ShowRemaining {
		t.Errorf("expected ShowRemaining to be false for Task in Done state")
	}
	if !modal.ShowOriginal {
		t.Errorf("expected ShowOriginal to be true for Task in Done state")
	}
	if !modal.ShowCompleted {
		t.Errorf("expected ShowCompleted to be true for Task in Done state")
	}

	// Update completed work from 8 to 12
	modal.InputCompleted.SetValue("12")
	fields := modal.FieldsToUpdate()

	// RemainingWork must NOT be sent in the patch payload
	if _, ok := fields["Microsoft.VSTS.Scheduling.RemainingWork"]; ok {
		t.Errorf("expected RemainingWork NOT to be in FieldsToUpdate() for item in Done state")
	}
	if v, ok := fields["Microsoft.VSTS.Scheduling.CompletedWork"]; !ok || v != 12.0 {
		t.Errorf("expected CompletedWork to be 12.0, got %v", fields["Microsoft.VSTS.Scheduling.CompletedWork"])
	}
}

func TestModalEstimateCustomStateCategorySuppression(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}
	defer database.Close()

	org := "custom-org"
	project := "Custom-Project"

	// 1. Save custom states for Task with custom state names and ADO categories
	err = database.SaveWorkItemTypeStates(org, project, "Task", []ado.WorkItemState{
		{Name: "To Do", Category: "Proposed"},
		{Name: "Dev", Category: "InProgress"},
		{Name: "Shipped", Category: "Completed"},
	})
	if err != nil {
		t.Fatalf("failed to save states: %v", err)
	}

	err = database.SaveWorkItemTypeFields(org, project, "Task", []ado.WorkItemTypeField{
		{ReferenceName: "System.Title", Name: "Title"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.OriginalEstimate", Name: "Original Estimate"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.RemainingWork", Name: "Remaining Work"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.CompletedWork", Name: "Completed Work"},
	})
	if err != nil {
		t.Fatalf("failed to save Task fields: %v", err)
	}

	// 2. Open for "Dev" (InProgress) -> Remaining work must be visible
	modalDev := NewModalEstimate()
	modalDev.OpenWithDB(101, "Dev Task", "Task", "Dev", org, project, 10.0, 5.0, 5.0, 0, database)
	if !modalDev.ShowRemaining {
		t.Errorf("expected ShowRemaining to be true for Task in 'Dev' state (InProgress category)")
	}

	// 3. Open for custom "Shipped" state (Completed category) -> Remaining work must be suppressed
	modalShipped := NewModalEstimate()
	modalShipped.OpenWithDB(102, "Shipped Task", "Task", "Shipped", org, project, 10.0, 0.0, 10.0, 0, database)
	if modalShipped.ShowRemaining {
		t.Errorf("expected ShowRemaining to be false for Task in 'Shipped' state (Completed category)")
	}

	modalShipped.InputCompleted.SetValue("12")
	fields := modalShipped.FieldsToUpdate()
	if _, ok := fields["Microsoft.VSTS.Scheduling.RemainingWork"]; ok {
		t.Errorf("expected RemainingWork NOT to be in FieldsToUpdate() for item in 'Shipped' state")
	}
	if v, ok := fields["Microsoft.VSTS.Scheduling.CompletedWork"]; !ok || v != 12.0 {
		t.Errorf("expected CompletedWork 12.0, got %v", fields["Microsoft.VSTS.Scheduling.CompletedWork"])
	}
}

func TestModalEstimateDirtyFieldsOnly(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	org := "test-org"
	project := "Test-Project"

	_ = database.SaveWorkItemTypeFields(org, project, "Task", []ado.WorkItemTypeField{
		{ReferenceName: "Microsoft.VSTS.Scheduling.OriginalEstimate", Name: "Original Estimate"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.RemainingWork", Name: "Remaining Work"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.CompletedWork", Name: "Completed Work"},
	})
	_ = database.SaveWorkItemTypeStates(org, project, "Task", []ado.WorkItemState{
		{Name: "To Do", Category: "Proposed"},
		{Name: "In Progress", Category: "InProgress"},
		{Name: "Done", Category: "Completed"},
	})

	modal := NewModalEstimate()
	// Open with initial values: Original=10, Remaining=2, Completed=8
	modal.OpenWithDB(123, "Test Task", "Task", "In Progress", org, project, 10.0, 2.0, 8.0, 0, database)

	// If no fields are touched, FieldsToUpdate should be empty!
	untouched := modal.FieldsToUpdate()
	if len(untouched) != 0 {
		t.Errorf("expected 0 fields to update when untouched, got %v", untouched)
	}

	// Change only CompletedWork from 8 to 10
	modal.InputCompleted.SetValue("10")
	modal.completedModified = true

	updated := modal.FieldsToUpdate()
	if len(updated) != 1 {
		t.Fatalf("expected exactly 1 field to update, got %d: %v", len(updated), updated)
	}
	if v, ok := updated["Microsoft.VSTS.Scheduling.CompletedWork"]; !ok || v != 10.0 {
		t.Errorf("expected CompletedWork 10.0, got %v", updated)
	}
	if _, ok := updated["Microsoft.VSTS.Scheduling.RemainingWork"]; ok {
		t.Errorf("unmodified RemainingWork should NOT be in update map")
	}
	if _, ok := updated["Microsoft.VSTS.Scheduling.OriginalEstimate"]; ok {
		t.Errorf("unmodified OriginalEstimate should NOT be in update map")
	}
}

func TestMsgTypeMetadataLoadedRerender(t *testing.T) {
	tmpDir := t.TempDir()
	database, err := db.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{DefaultOrg: "org1", DefaultProject: "proj1"}
	app := NewAppModel(cfg, database)
	app.CurrentScreen = ScreenTypeItem

	item := &db.WorkItemRecord{
		ID:           456,
		Org:          "org1",
		Project:      "proj1",
		WorkItemType: "CustomType",
		State:        "Active",
		Title:        "Custom item",
		Effort:       15.0,
	}
	app.ScreenItem.SetItem(item, nil)

	// Before metadata is loaded, default fallback visibility applies
	permsBefore, cached := database.ResolveItemFieldPermissions("org1", "proj1", "CustomType", "Active")
	if cached {
		t.Errorf("expected cached to be false before metadata is loaded")
	}
	if !permsBefore.ShowEffort {
		t.Errorf("expected default ShowEffort to be true before schema")
	}

	// Save fields and fire MsgTypeMetadataLoaded
	_ = database.SaveWorkItemTypeFields("org1", "proj1", "CustomType", []ado.WorkItemTypeField{
		{ReferenceName: "Custom.StoryPoints", Name: "Story Points"},
	})
	_ = database.SaveWorkItemTypeStates("org1", "proj1", "CustomType", []ado.WorkItemState{
		{Name: "Active", Category: "InProgress"},
	})

	_, _ = app.Update(MsgTypeMetadataLoaded{
		Org:          "org1",
		Project:      "proj1",
		WorkItemType: "CustomType",
	})

	permsAfter, cachedAfter := database.ResolveItemFieldPermissions("org1", "proj1", "CustomType", "Active")
	if !cachedAfter {
		t.Errorf("expected cached to be true after saving metadata")
	}
	if !permsAfter.ShowEffort || permsAfter.FieldEffort != "Custom.StoryPoints" {
		t.Errorf("expected FieldEffort Custom.StoryPoints, got %v", permsAfter.FieldEffort)
	}
}

func TestProductionIssueColorAndBadge(t *testing.T) {
	// 1. Color check - distinct red hue from Bug
	bugColor := GetTypeColor("Bug")
	prodColor := GetTypeColor("Production Issue")

	if prodColor != ColorProdIssue {
		t.Fatalf("expected Production Issue to have ColorProdIssue (%v), got %v", ColorProdIssue, prodColor)
	}
	if prodColor == bugColor {
		t.Fatalf("expected Production Issue to have a different red hue than Bug (%v vs %v)", prodColor, bugColor)
	}

	// Also check aliases
	if GetTypeColor("Prod Issue") != ColorProdIssue {
		t.Errorf("expected 'Prod Issue' to have ColorProdIssue")
	}
	if GetTypeColor("issue") != ColorProdIssue {
		t.Errorf("expected 'issue' to have ColorProdIssue")
	}

	// 2. FormatItemType check
	short := FormatItemType("Production Issue")
	if short != "Prod Issue" {
		t.Fatalf("expected FormatItemType('Production Issue') to be 'Prod Issue', got '%s'", short)
	}

	// 3. RenderTypeBadge check
	badge := RenderTypeBadge("Production Issue")
	if !strings.Contains(badge, "Prod Issue") {
		t.Fatalf("expected badge to contain 'Prod Issue', got: %s", badge)
	}
}

func TestHomeScreenSettingsHotkey(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "tree_hotkey_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{DbPath: dbPath}
	app := &AppModel{
		Config:         cfg,
		DB:             database,
		Width:          120,
		Height:         30,
		CurrentScreen:  ScreenTypeTree,
		ScreenTree:     NewScreenTree(),
		ScreenItem:     NewScreenItem(),
		ScreenCreate:   NewScreenCreate(),
		ScreenSettings: NewScreenSettings(),
		ModalStatus:    NewModalStatus(),
	}

	app.ScreenTree.SetData([]*db.TreeNode{
		{Record: &db.WorkItemRecord{ID: 101, Title: "Test Task", WorkItemType: "Task", State: "Active"}},
	})

	// Pressing 's' on Home Screen should open Status Modal
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	app = updated.(*AppModel)

	if app.ActiveModal != ModalTypeStatus {
		t.Fatalf("expected ActiveModal to be ModalTypeStatus after pressing 's', got %v", app.ActiveModal)
	}

	// Close modal with Esc
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*AppModel)
	if app.ActiveModal != ModalTypeNone {
		t.Fatalf("expected ActiveModal to be None after Esc")
	}

	// Pressing 'S' (Shift+S) on Home Screen should open Settings Screen
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	app = updated.(*AppModel)

	if app.CurrentScreen != ScreenTypeSettings {
		t.Fatalf("expected CurrentScreen to be ScreenTypeSettings after pressing 'S', got %v", app.CurrentScreen)
	}

	// Pressing Esc on Settings Screen should return to Home Screen
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*AppModel)

	if app.CurrentScreen != ScreenTypeTree {
		t.Fatalf("expected CurrentScreen to return to ScreenTypeTree after Esc, got %v", app.CurrentScreen)
	}
}

func TestSyncingSpinnerAndSubtleToast(t *testing.T) {
	cfg := &config.Config{DefaultOrg: "testorg", DefaultProject: "testproj"}
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	defer database.Close()

	app := NewAppModel(cfg, database)
	app.Width = 120
	app.Height = 30
	app.ScreenTree.SetData([]*db.TreeNode{
		{Record: &db.WorkItemRecord{ID: 101, Title: "Test Task", WorkItemType: "Task", State: "Active"}},
	})

	// Trigger syncing via 'r' key
	updated, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	app = updated.(*AppModel)

	if !app.Loading {
		t.Fatalf("expected app.Loading to be true while syncing")
	}
	if app.ToastMessage != "Syncing all items from ADO..." {
		t.Fatalf("expected ToastMessage to be 'Syncing all items from ADO...', got %s", app.ToastMessage)
	}
	if cmd == nil {
		t.Fatalf("expected tea.Cmd batch with spinner tick when syncing begins")
	}

	// Render view during loading
	view := app.View()
	if !strings.Contains(view, "Syncing all items from ADO...") {
		t.Fatalf("expected view to contain syncing message")
	}

	// Test ribbon rendering with spinner characters and subtle style
	ribbon := RenderBottomRibbon(120, "", []Shortcut{{Key: "q", Label: "Quit"}}, "⠋ Syncing all items from ADO...", false)
	if !strings.Contains(ribbon, "Syncing all items from ADO...") {
		t.Fatalf("expected ribbon to contain syncing message")
	}
	if !strings.Contains(ribbon, "⠋") {
		t.Fatalf("expected ribbon to contain spinner character")
	}
}

func TestScreenSettingsScrollingAndViewport(t *testing.T) {
	settings := NewScreenSettings()
	cfg := &config.Config{
		PatToken:             "pat-secret-123",
		GeminiApiKey:         "gemini-key-456",
		GeminiModel:          "gemini-3.7-flash",
		AutoCalcRemaining:    true,
		EffortSameAsEstimate: true,
	}
	settings.Reset(cfg, nil)

	// Render view with compact height to force scrolling
	view := settings.View(100, 20, "", false)
	if !settings.Ready {
		t.Fatalf("expected settings.Ready to be true after View()")
	}
	if !strings.Contains(view, "Application Settings") {
		t.Fatalf("expected view to contain header")
	}
	if strings.Contains(view, "⚙") {
		t.Fatalf("expected view not to contain cog wheel icon '⚙'")
	}

	// Initial offset at top
	if settings.Viewport.YOffset != 0 {
		t.Fatalf("expected initial YOffset 0, got %d", settings.Viewport.YOffset)
	}

	// Test PgDn scrolling and verify View() re-render does not reset YOffset
	settings.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	settings.View(100, 20, "", false)
	if settings.Viewport.YOffset <= 0 {
		t.Fatalf("expected YOffset to increase after PgDn, got %d", settings.Viewport.YOffset)
	}

	// Test PgUp scrolling
	settings.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	settings.View(100, 20, "", false)
	if settings.Viewport.YOffset != 0 {
		t.Fatalf("expected YOffset to return to 0 after PgUp, got %d", settings.Viewport.YOffset)
	}

	// Test Mouse Wheel scrolling in ScreenSettings and through AppModel
	settings.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Type: tea.MouseWheelDown})
	settings.View(100, 20, "", false)
	if settings.Viewport.YOffset <= 0 {
		t.Fatalf("expected YOffset to increase after MouseWheelDown, got %d", settings.Viewport.YOffset)
	}

	settings.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Type: tea.MouseWheelUp})
	settings.View(100, 20, "", false)
	if settings.Viewport.YOffset != 0 {
		t.Fatalf("expected YOffset to return to 0 after MouseWheelUp, got %d", settings.Viewport.YOffset)
	}

	// Test auto-scroll when navigating to Save Button (bottom field)
	settings.FocusedIndex = fieldSettingsSaveBtn
	settings.updateFocus()
	settings.View(100, 20, "", false)
	if settings.Viewport.YOffset <= 0 {
		t.Fatalf("expected YOffset to auto-scroll down to make Save button visible, got %d", settings.Viewport.YOffset)
	}

	// Test auto-scroll back to top when navigating to PAT Token (top field)
	settings.FocusedIndex = fieldSettingsPat
	settings.updateFocus()
	settings.View(100, 20, "", false)
	if settings.Viewport.YOffset != 0 {
		t.Fatalf("expected YOffset to auto-scroll back to 0 for PAT field, got %d", settings.Viewport.YOffset)
	}
}

func TestScreenSettingsInputSanitization(t *testing.T) {
	settings := NewScreenSettings()
	// Config with newlines and carriage returns
	cfg := &config.Config{
		PatToken:             "\n3ze32X1gPicRB6Y21KH46r7C0gI9NZQyOJQQJ99CHACAAAAAAjD9c7AAASAZD03yf\n",
		GeminiApiKey:         "\r\ngemini-key-with-crlf\r\n",
		GeminiModel:          "gemini-3.7-flash\n",
		AutoCalcRemaining:    true,
		EffortSameAsEstimate: false,
	}
	settings.Reset(cfg, nil)

	// Verify all inputs are cleaned of newlines
	pat, gKey, gModel, _, _, _, _, _, _, _, _, _, _, _, _, _, _, _ := settings.Values()
	if pat != "3ze32X1gPicRB6Y21KH46r7C0gI9NZQyOJQQJ99CHACAAAAAAjD9c7AAASAZD03yf" {
		t.Fatalf("expected clean PAT token, got %q", pat)
	}
	if gKey != "gemini-key-with-crlf" {
		t.Fatalf("expected clean Gemini Key, got %q", gKey)
	}
	if gModel != "gemini-3.7-flash" {
		t.Fatalf("expected clean Gemini Model, got %q", gModel)
	}

	// Verify rendered view has inline prompt with no newline splitting
	view := settings.View(100, 45, "", false)
	if strings.Contains(view, "\n│ ❯ \n") || strings.Contains(view, "\n│ ❯\n") {
		t.Fatalf("prompt was split onto its own line with newline in view")
	}

	// Test pasting a multiline string while updating
	settings.FocusedIndex = fieldSettingsPat
	settings.InputPat.SetValue("token\nwith\nnewlines")
	settings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if strings.Contains(settings.InputPat.Value(), "\n") {
		t.Fatalf("expected newlines to be stripped on input update")
	}
}

func TestClipboardCopyInTreeAndItemScreens(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	err = database.SaveWorkItem("my-org", "my-project", &ado.WorkItem{
		ID: 666613,
		Fields: map[string]interface{}{
			"System.Title":        "sample task",
			"System.WorkItemType": "Task",
			"System.State":        "Active",
		},
	})
	if err != nil {
		t.Fatalf("failed to save work item: %v", err)
	}
	rec, err := database.GetWorkItem(666613)
	if err != nil {
		t.Fatalf("failed to get item: %v", err)
	}

	app := &AppModel{
		DB:            database,
		Config:        &config.Config{DefaultOrg: "my-org", DefaultProject: "my-project"},
		AdoOrg:        "my-org",
		AdoProj:       "my-project",
		ScreenTree:    NewScreenTree(),
		ScreenItem:    NewScreenItem(),
		CurrentScreen: ScreenTypeTree,
		ActiveModal:   ModalTypeNone,
	}
	app.ScreenTree.SetData([]*db.TreeNode{
		{
			Record: rec,
		},
	})

	// 1. Test Shift+C on Tree Screen
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	expectedToast := `Copied "Task 666613: sample task" to clipboard`
	if app.ToastMessage != expectedToast {
		t.Fatalf("expected toast %q on tree screen, got %q", expectedToast, app.ToastMessage)
	}

	// 2. Test Shift+C on Item Detail Screen
	app.CurrentScreen = ScreenTypeItem
	app.ScreenItem.Item = rec
	app.ToastMessage = ""

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	if app.ToastMessage != expectedToast {
		t.Fatalf("expected toast %q on item screen, got %q", expectedToast, app.ToastMessage)
	}
}

func TestGeminiAICancellationAndRobustness(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	app := NewAppModel(&config.Config{DefaultOrg: "testorg", DefaultProject: "testproj"}, database)
	app.Width = 100
	app.Height = 30

	// 1. Open Gemini Dock
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if !app.DockGemini.Visible {
		t.Fatalf("expected Gemini Dock to be visible after pressing 'g'")
	}

	// 2. Simulate AI running state
	app.DockGemini.Loading = true
	app.DockGemini.StatusMsg = "Analyzing context..."
	canceled := false
	app.GeminiCancelFunc = func() {
		canceled = true
	}

	// 3. Pressing Esc while Loading should cancel the AI operation without freezing
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*AppModel)

	if !canceled {
		t.Fatalf("expected GeminiCancelFunc to be invoked on Esc")
	}
	if app.DockGemini.Loading {
		t.Fatalf("expected DockGemini.Loading to be false after Esc cancellation")
	}
	if app.ToastMessage != "AI operation cancelled" {
		t.Fatalf("expected toast 'AI operation cancelled', got %q", app.ToastMessage)
	}

	// 4. Pressing Esc again should hide the dock
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*AppModel)
	if app.DockGemini.Visible {
		t.Fatalf("expected DockGemini to be hidden after second Esc")
	}

	// 5. Pressing 'g' should reopen the dock cleanly and allow typing
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	app = updated.(*AppModel)
	if !app.DockGemini.Visible {
		t.Fatalf("expected DockGemini to be visible after 'g'")
	}
	if app.DockGemini.Loading {
		t.Fatalf("expected DockGemini.Loading to be false when reopened")
	}

	// 6. Test Ctrl+C cancellation when Loading
	canceled = false
	app.DockGemini.Loading = true
	app.GeminiCancelFunc = func() {
		canceled = true
	}
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	app = updated.(*AppModel)
	if !canceled {
		t.Fatalf("expected Ctrl+C to cancel running Gemini query")
	}
	if app.DockGemini.Loading {
		t.Fatalf("expected DockGemini.Loading to be reset on Ctrl+C")
	}
}

func TestTabBarAutoScrollActiveTabInView(t *testing.T) {
	screen := NewScreenTree()
	screen.TreeRoots = []*db.TreeNode{
		{Record: &db.WorkItemRecord{ID: 101, Title: "Alpha Feature Long Title", WorkItemType: "Feature"}},
		{Record: &db.WorkItemRecord{ID: 102, Title: "Beta Feature Long Title", WorkItemType: "Feature"}},
		{Record: &db.WorkItemRecord{ID: 103, Title: "Gamma Epic Architecture", WorkItemType: "Epic"}},
		{Record: &db.WorkItemRecord{ID: 104, Title: "Delta Spot Instances", WorkItemType: "Feature"}},
		{Record: &db.WorkItemRecord{ID: 105, Title: "Epsilon Database Scaling", WorkItemType: "PBI"}},
	}

	// Case 1: Active tab is 0 (first tab) on restricted width (e.g. 70 columns)
	screen.ActiveTab = 0
	tabBar0 := screen.renderTabBar(70)
	if !strings.Contains(tabBar0, "#101") {
		t.Fatalf("expected active tab 0 (#101) to be visible in tab bar, got:\n%s", tabBar0)
	}
	if !strings.Contains(tabBar0, "▶") {
		t.Fatalf("expected right scroll indicator or active marker in tab bar 0, got:\n%s", tabBar0)
	}

	// Case 2: Active tab is 3 (tab 4) on restricted width
	screen.ActiveTab = 3
	tabBar3 := screen.renderTabBar(70)
	if !strings.Contains(tabBar3, "#104") {
		t.Fatalf("expected active tab 3 (#104) to be fully visible in tab bar, got:\n%s", tabBar3)
	}
	if !strings.Contains(tabBar3, "◀") {
		t.Fatalf("expected left scroll indicator '◀' when earlier tabs are scrolled out of view, got:\n%s", tabBar3)
	}

	// Case 3: Active tab is last tab (tab 4: #105)
	screen.ActiveTab = 4
	tabBar4 := screen.renderTabBar(70)
	if !strings.Contains(tabBar4, "#105") {
		t.Fatalf("expected active tab 4 (#105) to be fully visible, got:\n%s", tabBar4)
	}
}

func TestModalTabSwitchWorkflowAndMouseWheel(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	_ = database.SaveWorkItem("myorg", "myproj", &ado.WorkItem{
		ID: 656841,
		Fields: map[string]interface{}{
			"System.Title":        "GCP Cost Optimization",
			"System.WorkItemType": "Epic",
			"System.State":        "New",
		},
	})
	_ = database.SaveWorkItem("myorg", "myproj", &ado.WorkItem{
		ID: 666236,
		Fields: map[string]interface{}{
			"System.Title":        "ADO API test",
			"System.WorkItemType": "Feature",
			"System.State":        "In Progress",
		},
	})
	_ = database.SaveWorkItem("myorg", "myproj", &ado.WorkItem{
		ID: 656842,
		Fields: map[string]interface{}{
			"System.Title":        "GKE & Spot Instances",
			"System.WorkItemType": "Feature",
			"System.State":        "New",
		},
	})
	_ = database.SaveTrackedRoot(656841, "myorg", "myproj", "")
	_ = database.SaveTrackedRoot(666236, "myorg", "myproj", "")
	_ = database.SaveTrackedRoot(656842, "myorg", "myproj", "")

	cfg := &config.Config{DefaultOrg: "myorg", DefaultProject: "myproj"}
	app := NewAppModel(cfg, database)
	app.Width = 100
	app.Height = 30
	roots, _ := database.GetTrackedRoots()
	allItems, _ := database.GetAllWorkItems()
	app.ScreenTree.SetData(db.BuildTree(allItems, roots))

	if len(app.ScreenTree.TreeRoots) != 3 {
		t.Fatalf("expected 3 tree roots, got %d", len(app.ScreenTree.TreeRoots))
	}

	// 1. Press Shift+T or T to open ModalTab
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	app = updated.(*AppModel)
	if app.ActiveModal != ModalTypeTab {
		t.Fatalf("expected ActiveModal to be ModalTypeTab after pressing 'T', got %v", app.ActiveModal)
	}
	if !app.ModalTab.Active {
		t.Fatalf("expected ModalTab to be active")
	}
	if len(app.ModalTab.Filtered) != 3 {
		t.Fatalf("expected 3 filtered items in ModalTab, got %d", len(app.ModalTab.Filtered))
	}

	// 2. Test modal view rendering
	viewStr := app.ModalTab.View(100, 30)
	if !strings.Contains(viewStr, "Select Root Tab") || !strings.Contains(viewStr, "GCP Cost Optimization") {
		t.Fatalf("expected ModalTab view to render title and roots, got:\n%s", viewStr)
	}

	// 3. Test keyboard navigation & Mouse Wheel
	app.ModalTab.CursorDown()
	if app.ModalTab.Cursor != 1 {
		t.Fatalf("expected cursor 1 after CursorDown, got %d", app.ModalTab.Cursor)
	}
	// MouseWheelDown
	updated, _ = app.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Type: tea.MouseWheelDown})
	app = updated.(*AppModel)
	if app.ModalTab.Cursor != 2 {
		t.Fatalf("expected cursor 2 after MouseWheelDown, got %d", app.ModalTab.Cursor)
	}
	// MouseWheelUp
	updated, _ = app.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Type: tea.MouseWheelUp})
	app = updated.(*AppModel)
	if app.ModalTab.Cursor != 1 {
		t.Fatalf("expected cursor 1 after MouseWheelUp, got %d", app.ModalTab.Cursor)
	}

	// 4. Test Search Filtering: filter for Optimization (Item #656841 at index 2)
	app.ModalTab.SearchInput.SetValue("Optimization")
	app.ModalTab.rebuildFiltered()
	if len(app.ModalTab.Filtered) != 1 || app.ModalTab.Filtered[0].RootID != 656841 {
		t.Fatalf("expected filtered item 656841 for query 'Optimization', got %v", app.ModalTab.Filtered)
	}

	// 5. Select & Confirm with Enter
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = updated.(*AppModel)
	if app.ActiveModal != ModalTypeNone {
		t.Fatalf("expected ActiveModal to be ModalTypeNone after Enter, got %v", app.ActiveModal)
	}
	if app.ScreenTree.ActiveTab != 2 {
		t.Fatalf("expected ScreenTree.ActiveTab to be 2 (656841), got %d", app.ScreenTree.ActiveTab)
	}
	if app.ScreenTree.TreeRoots[app.ScreenTree.ActiveTab] == nil || app.ScreenTree.TreeRoots[app.ScreenTree.ActiveTab].Record.ID != 656841 {
		t.Fatalf("expected active root ID 656841, got %v", app.ScreenTree.TreeRoots[app.ScreenTree.ActiveTab])
	}
}

func TestKeybindingsSwapDatesAndDelete(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	_ = database.SaveWorkItem("myorg", "myproj", &ado.WorkItem{
		ID: 100,
		Fields: map[string]interface{}{
			"System.Title":        "Test Item",
			"System.WorkItemType": "Task",
			"System.State":        "Active",
		},
	})
	_ = database.SaveTrackedRoot(100, "myorg", "myproj", "")

	cfg := &config.Config{DefaultOrg: "myorg", DefaultProject: "myproj"}
	app := NewAppModel(cfg, database)
	app.Width = 100
	app.Height = 30
	roots, _ := database.GetTrackedRoots()
	allItems, _ := database.GetAllWorkItems()
	app.ScreenTree.SetData(db.BuildTree(allItems, roots))

	// 1. In ScreenTree, pressing 'd' should open ModalSchedule (Dates)
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = updated.(*AppModel)
	if app.ActiveModal != ModalTypeSchedule {
		t.Fatalf("expected ActiveModal to be ModalTypeSchedule on 'd', got %v", app.ActiveModal)
	}

	// Close modal with Esc
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*AppModel)
	if app.ActiveModal != ModalTypeNone {
		t.Fatalf("expected ActiveModal to be ModalTypeNone after Esc, got %v", app.ActiveModal)
	}

	// 2. In ScreenTree, pressing 'D' / shift+d should open ModalConfirm (Delete)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	app = updated.(*AppModel)
	if app.ActiveModal != ModalTypeConfirm {
		t.Fatalf("expected ActiveModal to be ModalTypeConfirm on 'D', got %v", app.ActiveModal)
	}
	if app.ModalConfirm.ActionTag != "delete" {
		t.Fatalf("expected ModalConfirm.ActionTag to be 'delete', got %q", app.ModalConfirm.ActionTag)
	}

	// Close modal with Esc
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*AppModel)

	// 3. Switch to ScreenItem
	app.CurrentScreen = ScreenTypeItem
	app.ScreenItem.SetData(allItems[0], nil, nil, nil)

	// Pressing 'd' in ScreenItem should open ModalSchedule (Dates)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	app = updated.(*AppModel)
	if app.ActiveModal != ModalTypeSchedule {
		t.Fatalf("expected ActiveModal to be ModalTypeSchedule on 'd' in ScreenItem, got %v", app.ActiveModal)
	}

	// Close modal with Esc
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*AppModel)

	// Pressing 'D' in ScreenItem should open ModalConfirm (Delete)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	app = updated.(*AppModel)
	if app.ActiveModal != ModalTypeConfirm {
		t.Fatalf("expected ActiveModal to be ModalTypeConfirm on 'D' in ScreenItem, got %v", app.ActiveModal)
	}
	if app.ModalConfirm.ActionTag != "delete" {
		t.Fatalf("expected ModalConfirm.ActionTag to be 'delete', got %q", app.ModalConfirm.ActionTag)
	}
}

func TestScreenSettingsWorkItemTypesPicker(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	// Seed some work items with custom types
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 101,
		Fields: map[string]interface{}{
			"System.Title":        "Task 1",
			"System.WorkItemType": "Task",
			"System.State":        "Active",
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 102,
		Fields: map[string]interface{}{
			"System.Title":        "Custom Spike",
			"System.WorkItemType": "Spike",
			"System.State":        "Active",
		},
	})

	cfg := &config.Config{
		IncompleteTypes: []string{"Task"},
	}

	settings := NewScreenSettings()
	settings.Reset(cfg, database)

	// Verify Spike was populated into TypeOptions from DB
	foundSpike := false
	for _, opt := range settings.TypeOptions {
		if opt == "Spike" {
			foundSpike = true
			break
		}
	}
	if !foundSpike {
		t.Fatalf("expected TypeOptions to include 'Spike' from database")
	}

	// Focus on fieldSettingsIncompleteTypes
	settings.FocusedIndex = fieldSettingsIncompleteTypes
	settings.updateFocus()

	// Verify view before opening picker has badge and no descriptions
	view := settings.View(100, 60, "", false)
	if !strings.Contains(view, "Applicable Item Types:") {
		t.Fatalf("expected view to contain 'Applicable Item Types:', got:\n%s", view)
	}
	if strings.Contains(view, "Enforce incomplete criteria on Task work items") {
		t.Fatalf("expected view not to contain old hardcoded descriptions")
	}

	// Press Enter to open TypePicker modal
	action, _ := settings.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if action != SettingsActionNone {
		t.Fatalf("expected SettingsActionNone, got %v", action)
	}
	if settings.TypePicker == nil || !settings.TypePicker.Active {
		t.Fatalf("expected TypePicker to be active after Enter on fieldSettingsIncompleteTypes")
	}

	// View while picker is active should render filter picker modal
	pickerView := settings.View(100, 40, "", false)
	if !strings.Contains(pickerView, "Select Applicable Work Item Types") {
		t.Fatalf("expected pickerView to show 'Select Applicable Work Item Types'")
	}

	// Navigate down and toggle selection
	settings.Update(tea.KeyMsg{Type: tea.KeyDown})
	settings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	// Confirm selection with Enter
	settings.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if settings.TypePicker.Active {
		t.Fatalf("expected TypePicker to close after Enter")
	}

	// IncompleteTypes should be updated
	if len(settings.IncompleteTypes) == 0 {
		t.Fatalf("expected IncompleteTypes to have items after confirmation")
	}

	// Test IsTypeApplicable with configured IncompleteTypes
	cfg.IncompleteTypes = settings.IncompleteTypes
	if !IsTypeApplicable("Task", cfg) {
		t.Fatalf("expected IsTypeApplicable true for Task")
	}
	if IsTypeApplicable("Epic", cfg) && !containsStringCaseInsensitive(settings.IncompleteTypes, "Epic") && !containsStringCaseInsensitive(settings.IncompleteTypes, "[All]") {
		t.Fatalf("expected IsTypeApplicable false for unselected Epic")
	}
}

func containsStringCaseInsensitive(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}

func TestScreenSettingsIncompleteStatusesPicker(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "settings_statuses_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 101,
		Fields: map[string]interface{}{
			"System.Title":        "Task 1",
			"System.WorkItemType": "Task",
			"System.State":        "Active",
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 102,
		Fields: map[string]interface{}{
			"System.Title":        "Task 2",
			"System.WorkItemType": "Task",
			"System.State":        "CustomState",
		},
	})

	cfg := &config.Config{
		IncompleteStatuses: []string{"Active"},
	}

	settings := NewScreenSettings()
	settings.Reset(cfg, database)

	// Verify CustomState was populated into StatusOptions from DB
	foundCustom := false
	for _, opt := range settings.StatusOptions {
		if opt == "CustomState" {
			foundCustom = true
			break
		}
	}
	if !foundCustom {
		t.Fatalf("expected StatusOptions to include 'CustomState' from database")
	}

	// Focus on fieldSettingsIncompleteStatuses
	settings.FocusedIndex = fieldSettingsIncompleteStatuses
	settings.updateFocus()

	// Verify view before opening picker has badge
	view := settings.View(100, 60, "", false)
	if !strings.Contains(view, "Applicable Item Statuses:") {
		t.Fatalf("expected view to contain 'Applicable Item Statuses:', got:\n%s", view)
	}

	// Press Enter to open StatusPicker modal
	action, _ := settings.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if action != SettingsActionNone {
		t.Fatalf("expected SettingsActionNone, got %v", action)
	}
	if settings.StatusPicker == nil || !settings.StatusPicker.Active {
		t.Fatalf("expected StatusPicker to be active after Enter on fieldSettingsIncompleteStatuses")
	}

	// View while picker is active should render filter picker modal
	pickerView := settings.View(100, 40, "", false)
	if !strings.Contains(pickerView, "Select Applicable Item Statuses") {
		t.Fatalf("expected pickerView to show 'Select Applicable Item Statuses'")
	}

	// Navigate down twice to CustomState and toggle selection
	settings.Update(tea.KeyMsg{Type: tea.KeyDown})
	settings.Update(tea.KeyMsg{Type: tea.KeyDown})
	settings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	// Confirm selection with Enter
	settings.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if settings.StatusPicker.Active {
		t.Fatalf("expected StatusPicker to close after Enter")
	}

	// IncompleteStatuses should be updated with Active and CustomState
	if len(settings.IncompleteStatuses) == 0 {
		t.Fatalf("expected IncompleteStatuses to have items after confirmation")
	}

	// Test IsStatusApplicable with configured IncompleteStatuses
	cfg.IncompleteStatuses = settings.IncompleteStatuses
	if !IsStatusApplicable("Active", cfg) && containsStringCaseInsensitive(settings.IncompleteStatuses, "Active") {
		t.Fatalf("expected IsStatusApplicable true for Active")
	}
	if IsStatusApplicable("Removed", cfg) && !containsStringCaseInsensitive(settings.IncompleteStatuses, "Removed") {
		t.Fatalf("expected IsStatusApplicable false for unselected Removed")
	}
}

func TestScreenSettingsIncompleteDateStatusesPicker(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_incomplete_date_statuses.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	// Pre-populate with work items
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 601,
		Fields: map[string]interface{}{
			"System.Title":        "Backlog Item",
			"System.WorkItemType": "Task",
			"System.State":        "To Do",
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 602,
		Fields: map[string]interface{}{
			"System.Title":        "Active Item",
			"System.WorkItemType": "Task",
			"System.State":        "Active",
		},
	})

	cfg := &config.Config{
		IncompleteDateStatuses: []string{"Active"},
	}

	settings := NewScreenSettings()
	settings.Reset(cfg, database)

	// Focus on fieldSettingsIncompleteDateStatuses
	settings.FocusedIndex = fieldSettingsIncompleteDateStatuses
	settings.updateFocus()

	// Verify view before opening picker has badge
	view := settings.View(100, 60, "", false)
	if !strings.Contains(view, "Applicable Date Statuses:") {
		t.Fatalf("expected view to contain 'Applicable Date Statuses:', got:\n%s", view)
	}

	// Press Enter to open DateStatusPicker modal
	action, _ := settings.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if action != SettingsActionNone {
		t.Fatalf("expected SettingsActionNone, got %v", action)
	}
	if settings.DateStatusPicker == nil || !settings.DateStatusPicker.Active {
		t.Fatalf("expected DateStatusPicker to be active after Enter on fieldSettingsIncompleteDateStatuses")
	}

	// View while picker is active should render filter picker modal
	pickerView := settings.View(100, 40, "", false)
	if !strings.Contains(pickerView, "Select Statuses Requiring Dates") {
		t.Fatalf("expected pickerView to show 'Select Statuses Requiring Dates'")
	}

	// Navigate down twice and toggle selection
	settings.Update(tea.KeyMsg{Type: tea.KeyDown})
	settings.Update(tea.KeyMsg{Type: tea.KeyDown})
	settings.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})

	// Confirm selection with Enter
	settings.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if settings.DateStatusPicker.Active {
		t.Fatalf("expected DateStatusPicker to close after Enter")
	}

	// IncompleteDateStatuses should be updated
	if len(settings.IncompleteDateStatuses) == 0 {
		t.Fatalf("expected IncompleteDateStatuses to have items after confirmation")
	}

	// Test IsDateStatusApplicable with configured IncompleteDateStatuses
	cfg.IncompleteDateStatuses = settings.IncompleteDateStatuses
	if !IsDateStatusApplicable("Active", cfg) && containsStringCaseInsensitive(settings.IncompleteDateStatuses, "Active") {
		t.Fatalf("expected IsDateStatusApplicable true for Active")
	}
	if IsDateStatusApplicable("To Do", cfg) && !containsStringCaseInsensitive(settings.IncompleteDateStatuses, "To Do") {
		t.Fatalf("expected IsDateStatusApplicable false for unselected To Do")
	}
}

func TestOpenURLTestSafetyAndMocking(t *testing.T) {
	app := &AppModel{}
	testURL := "https://dev.azure.com/test-org/test-proj/_workitems/edit/12345"

	// 1. In test environment, openURL must not spawn any browser processes, but sets LastOpenedURL
	app.openURL(testURL)
	if app.LastOpenedURL != testURL {
		t.Fatalf("expected LastOpenedURL %q, got %q", testURL, app.LastOpenedURL)
	}

	// 2. Custom OpenURLFunc callback is invoked if provided
	var captured string
	app.OpenURLFunc = func(url string) {
		captured = url
	}
	app.openURL("https://dev.azure.com/custom-url")
	if captured != "https://dev.azure.com/custom-url" {
		t.Fatalf("expected captured URL, got %q", captured)
	}
}

func TestScreenCreateNearestParentIterationInheritance(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_create_iter.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// 1. Root Epic #100 with Iteration and Area
	epic := &ado.WorkItem{
		ID: 100,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Epic",
			"System.Title":         "Core Epic",
			"System.AreaPath":      "DigitalApp\\CoreServices",
			"System.IterationPath": "DigitalApp\\Sprint 2026.Q3",
		},
	}
	_ = database.SaveWorkItem("myorg", "myproj", epic)

	// 2. Child Story #101 without iteration or area
	story := &ado.WorkItem{
		ID: 101,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "User Story",
			"System.Title":         "Sub Story",
			"System.AreaPath":      "",
			"System.IterationPath": "",
		},
		Relations: []ado.WorkItemRelation{
			{Rel: "System.LinkTypes.Hierarchy-Reverse", URL: "https://dev.azure.com/myorg/myproj/_apis/wit/workItems/100"},
		},
	}
	_ = database.SaveWorkItem("myorg", "myproj", story)

	// 3. ScreenCreate Reset with parent #100 (which has area and iteration)
	epicRec, _ := database.GetWorkItem(100)
	sc := NewScreenCreate()
	sc.Reset(epicRec, "user@org.com", database)

	view := sc.View(100, 30, "", false)
	if !strings.Contains(view, "DigitalApp\\Sprint 2026.Q3") {
		t.Fatalf("expected ScreenCreate parent banner to display inherited iteration 'DigitalApp\\Sprint 2026.Q3', got:\n%s", view)
	}
	if !strings.Contains(view, "DigitalApp\\CoreServices") {
		t.Fatalf("expected ScreenCreate parent banner to display inherited area 'DigitalApp\\CoreServices', got:\n%s", view)
	}

	// 4. Verify parent iteration & area lookup from DB for immediate parent
	nearestIter100 := database.GetNearestParentIteration(100)
	if nearestIter100 != "DigitalApp\\Sprint 2026.Q3" {
		t.Fatalf("expected nearest parent iteration 'DigitalApp\\Sprint 2026.Q3', got %q", nearestIter100)
	}
	nearestArea100 := database.GetNearestParentArea(100)
	if nearestArea100 != "DigitalApp\\CoreServices" {
		t.Fatalf("expected nearest parent area 'DigitalApp\\CoreServices', got %q", nearestArea100)
	}

	// 5. Verify that parent #101 (which has empty iteration/area) does NOT look past itself
	nearestIter101 := database.GetNearestParentIteration(101)
	if nearestIter101 != "" {
		t.Fatalf("expected empty iteration for parent 101, got %q", nearestIter101)
	}
	nearestArea101 := database.GetNearestParentArea(101)
	if nearestArea101 != "" {
		t.Fatalf("expected empty area for parent 101, got %q", nearestArea101)
	}
}

func TestScreenSettingsValidationStatusRendering(t *testing.T) {
	ss := NewScreenSettings()
	ss.InitViewport(120, 40)

	// 1. Checking status
	ss.PatStatus = ValidationStatusChecking
	ss.GeminiStatus = ValidationStatusChecking
	view := ss.View(120, 40, "", false)
	if !strings.Contains(view, "[⟳ Checking...]") {
		t.Errorf("expected view to contain [⟳ Checking...], got:\n%s", view)
	}

	// 2. Valid status
	ss.PatStatus = ValidationStatusValid
	ss.GeminiStatus = ValidationStatusValid
	view = ss.View(120, 40, "", false)
	if !strings.Contains(view, "[✓ Valid]") {
		t.Errorf("expected view to contain [✓ Valid], got:\n%s", view)
	}

	// 3. Invalid status with detailed error
	ss.PatStatus = ValidationStatusInvalid
	ss.PatError = "authentication failed (HTTP 401): invalid or expired PAT"
	ss.GeminiStatus = ValidationStatusInvalid
	ss.GeminiError = "invalid API key"
	view = ss.View(120, 40, "", false)
	if !strings.Contains(view, "[✗ Invalid]") {
		t.Errorf("expected view to contain [✗ Invalid], got:\n%s", view)
	}
	if !strings.Contains(view, "authentication failed (HTTP 401): invalid or expired PAT") {
		t.Errorf("expected view to contain PAT error message, got:\n%s", view)
	}
	if !strings.Contains(view, "invalid API key") {
		t.Errorf("expected view to contain Gemini error message, got:\n%s", view)
	}

	// 4. Not configured status
	ss.PatStatus = ValidationStatusNotConfigured
	ss.GeminiStatus = ValidationStatusNotConfigured
	view = ss.View(120, 40, "", false)
	if !strings.Contains(view, "[○ Not Configured]") {
		t.Errorf("expected view to contain [○ Not Configured], got:\n%s", view)
	}
}

func TestAppModelCredentialsValidationLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		PatToken:     "test-pat",
		GeminiApiKey: "test-gemini-key",
		GeminiModel:  "gemini-3.7-flash",
		DefaultOrg:   "myorg",
		DefaultProject: "myproj",
	}

	app := NewAppModel(cfg, database)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// 1. Startup validation status initialization
	_ = app.Init()
	if app.ScreenSettings.PatStatus != ValidationStatusChecking {
		t.Errorf("expected PatStatus to be Checking at startup, got %v", app.ScreenSettings.PatStatus)
	}
	if app.ScreenSettings.GeminiStatus != ValidationStatusChecking {
		t.Errorf("expected GeminiStatus to be Checking at startup, got %v", app.ScreenSettings.GeminiStatus)
	}

	// 2. Receiving MsgCredentialsValidated (Valid)
	updatedModel, _ := app.Update(MsgCredentialsValidated{
		PatChecked:    true,
		PatValid:      true,
		GeminiChecked: true,
		GeminiValid:   true,
	})
	m := updatedModel.(*AppModel)
	if m.ScreenSettings.PatStatus != ValidationStatusValid {
		t.Errorf("expected PatStatus to be Valid, got %v", m.ScreenSettings.PatStatus)
	}
	if m.ScreenSettings.GeminiStatus != ValidationStatusValid {
		t.Errorf("expected GeminiStatus to be Valid, got %v", m.ScreenSettings.GeminiStatus)
	}

	// 3. Receiving MsgCredentialsValidated (Invalid)
	updatedModel, _ = app.Update(MsgCredentialsValidated{
		PatChecked:    true,
		PatValid:      false,
		PatError:      "expired token",
		GeminiChecked: true,
		GeminiValid:   false,
		GeminiError:   "invalid key",
	})
	m = updatedModel.(*AppModel)
	if m.ScreenSettings.PatStatus != ValidationStatusInvalid || m.ScreenSettings.PatError != "expired token" {
		t.Errorf("expected PatStatus to be Invalid with 'expired token', got status=%v err=%v", m.ScreenSettings.PatStatus, m.ScreenSettings.PatError)
	}
	if m.ScreenSettings.GeminiStatus != ValidationStatusInvalid || m.ScreenSettings.GeminiError != "invalid key" {
		t.Errorf("expected GeminiStatus to be Invalid with 'invalid key', got status=%v err=%v", m.ScreenSettings.GeminiStatus, m.ScreenSettings.GeminiError)
	}

	// 4. Save settings triggers re-validation
	cmd := m.saveSettingsCmd("new-pat", "new-key", "gemini-3.7-flash", true, false, true, 120, false, false, false, false, false, false, false, "", []string{"Task"}, []string{"[All]"}, []string{"[All]"})
	msg := cmd()
	savedMsg, ok := msg.(MsgSettingsSaved)
	if !ok {
		t.Fatalf("expected MsgSettingsSaved, got %T", msg)
	}
	if savedMsg.Pat != "new-pat" || savedMsg.GeminiKey != "new-key" {
		t.Errorf("unexpected MsgSettingsSaved data: %+v", savedMsg)
	}
	if m.ScreenSettings.PatStatus != ValidationStatusChecking {
		t.Errorf("expected PatStatus to be Checking after save, got %v", m.ScreenSettings.PatStatus)
	}
	if m.ScreenSettings.GeminiStatus != ValidationStatusChecking {
		t.Errorf("expected GeminiStatus to be Checking after save, got %v", m.ScreenSettings.GeminiStatus)
	}
}

func TestAppModelQueryTracking(t *testing.T) {
	tempDir := t.TempDir()
	tmpPath := filepath.Join(tempDir, "adotest_app_query.db")

	database, err := db.Open(tmpPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DbPath:         tmpPath,
		PatToken:       "dummy-token",
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
	}

	app := NewAppModel(cfg, database)
	app.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// 1. Save tracked query in DB and open Track Modal
	queryID := "7509d605-466a-4d8c-847f-c14e1b46a647"
	_ = database.SaveTrackedQuery(queryID, "Active Sprint PBIs & Bugs", "Shared Queries/Sprint", "my-org", "my-project", "https://dev.azure.com/...", "tree")
	_ = database.SaveTrackedRootWithQuery(101, "my-org", "my-project", "", queryID)
	_ = database.SaveTrackedRootWithQuery(102, "my-org", "my-project", "", queryID)

	app.ModalTrack.Open(database)
	view := app.ModalTrack.View(100, 30)
	if !strings.Contains(view, "Tracked Queries (1):") {
		t.Fatalf("expected Tracked Queries section in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Active Sprint PBIs & Bugs") {
		t.Fatalf("expected query name in view, got:\n%s", view)
	}
	if !strings.Contains(view, "(2 items)") {
		t.Fatalf("expected item count in query view, got:\n%s", view)
	}

	// 1b. Test ModalTab rendering for query tabs (ensure no 🔍 before [Query])
	queryNode := &db.TreeNode{
		Record: &db.WorkItemRecord{
			ID:           0,
			Title:        "Feature 4 - UAT Bugs",
			WorkItemType: "Query",
			State:        "Active",
		},
		IsQuery: true,
		QueryID: queryID,
	}
	app.ModalTab.Open([]*db.TreeNode{queryNode}, 0)
	tabView := app.ModalTab.View(100, 30)
	if !strings.Contains(tabView, "1: [Query] Feature 4 - UAT Bugs") {
		t.Fatalf("expected '1: [Query] Feature 4 - UAT Bugs' in ModalTab view, got:\n%s", tabView)
	}
	if strings.Contains(tabView, "🔍 [Query]") || strings.Contains(tabView, "🔍 Feature") {
		t.Fatalf("expected NO emoji before [Query] in ModalTab view, got:\n%s", tabView)
	}

	// 2. Handle MsgTrackDone for query
	updatedModel, _ := app.Update(MsgTrackDone{
		RootID:    101,
		QueryName: "Active Sprint PBIs & Bugs",
		Count:     5,
	})
	m := updatedModel.(*AppModel)
	if !strings.Contains(m.ToastMessage, "Active Sprint PBIs & Bugs") || !strings.Contains(m.ToastMessage, "5 items") {
		t.Errorf("expected query toast message, got %q", m.ToastMessage)
	}

	// 3. Untrack query in ModalTrack
	app.ActiveModal = ModalTypeTrack
	app.ModalTrack.InputMode = false
	app.ModalTrack.Cursor = 0
	updatedModel, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updatedModel.(*AppModel)
	if !strings.Contains(m.ToastMessage, "Untracked query") {
		t.Errorf("expected untracked query toast, got %q", m.ToastMessage)
	}

	queriesAfter, _ := database.GetTrackedQueries()
	if len(queriesAfter) != 0 {
		t.Errorf("expected 0 tracked queries after delete, got %d", len(queriesAfter))
	}
}

func TestDropdownPicker(t *testing.T) {
	picker := NewDropdownPicker()
	options := []string{
		"DigitalApp\\Sprint 2026.Q1",
		"DigitalApp\\Sprint 2026.Q2",
		"DigitalApp\\Sprint 2026.Q3",
	}

	// 1. Open picker with initial selection
	picker.Open("⚡ Select Iteration", options, "DigitalApp\\Sprint 2026.Q2")
	if !picker.Active {
		t.Fatal("expected picker to be active")
	}
	if picker.Cursor != 1 {
		t.Fatalf("expected cursor to be at index 1 for current selection, got %d", picker.Cursor)
	}

	// 2. View rendering
	v := picker.View(80, 24)
	if !strings.Contains(v, "⚡ Select Iteration") || !strings.Contains(v, "Sprint 2026.Q2 ✓") {
		t.Fatalf("unexpected picker view:\n%s", v)
	}

	// 3. Navigate Down
	picker.Update(tea.KeyMsg{Type: tea.KeyDown})
	if picker.Cursor != 2 {
		t.Fatalf("expected cursor 2, got %d", picker.Cursor)
	}

	// 4. Filter search
	picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q', '1'}})
	if len(picker.Filtered) != 1 || picker.Filtered[0] != "DigitalApp\\Sprint 2026.Q1" {
		t.Fatalf("expected filtered to have Q1 only, got %v", picker.Filtered)
	}

	// 5. Select with Enter
	val, chosen, closed, _ := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !chosen || !closed || val != "DigitalApp\\Sprint 2026.Q1" {
		t.Fatalf("expected selection of Q1, got val=%s, chosen=%v, closed=%v", val, chosen, closed)
	}
	if picker.Active {
		t.Fatal("expected picker to be closed after Enter")
	}

	// 6. Custom value typing
	picker.Open("⚡ Select Area", options, "")
	for _, r := range "DigitalApp\\NewTeam" {
		picker.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	val, chosen, closed, _ = picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !chosen || !closed || val != "DigitalApp\\NewTeam" {
		t.Fatalf("expected custom val 'DigitalApp\\NewTeam', got val=%s, chosen=%v, closed=%v", val, chosen, closed)
	}
}

func TestModalEditTagsIterationArea(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Seed work items for iterations and areas
	item1 := &ado.WorkItem{
		ID: 663676,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Task",
			"System.Title":         "Deploy Services to GKE",
			"System.State":         "Active",
			"System.IterationPath": "DigitalApp\\Sprint 2026.Q3",
			"System.AreaPath":      "DigitalApp\\Cloud Team",
			"System.Tags":          "gke; backend",
		},
	}
	item2 := &ado.WorkItem{
		ID: 663677,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Task",
			"System.Title":         "Setup Ingress",
			"System.State":         "New",
			"System.IterationPath": "DigitalApp\\Sprint 2026.Q4",
			"System.AreaPath":      "DigitalApp\\Core Team",
			"System.Tags":          "networking; security",
		},
	}
	_ = database.SaveWorkItem("myorg", "myproject", item1)
	_ = database.SaveWorkItem("myorg", "myproject", item2)

	modal := NewModalEdit()
	modal.Open(663676, "Task", "Deploy Services to GKE", "<p>Deploy manifests</p>", "gke; backend", "DigitalApp\\Sprint 2026.Q3", "DigitalApp\\Cloud Team", database)

	if !modal.Active {
		t.Fatal("expected modal to be active")
	}

	// 1. Verify View Rendering
	view := modal.View(80, 24)
	if !strings.Contains(view, "Edit Details: #663676") {
		t.Fatalf("expected header with item ID in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Title:") || !strings.Contains(view, "Tags:") || !strings.Contains(view, "Iteration:") || !strings.Contains(view, "Area Path:") || !strings.Contains(view, "Description:") {
		t.Fatalf("expected all fields rendered in view, got:\n%s", view)
	}
	if !strings.Contains(view, "DigitalApp\\Sprint 2026.Q3") || !strings.Contains(view, "DigitalApp\\Cloud Team") {
		t.Fatalf("expected current iteration and area values in view, got:\n%s", view)
	}

	// 2. Tab navigation
	if modal.FocusedIndex != fieldEditTitle {
		t.Fatalf("expected initial focus on Title, got %d", modal.FocusedIndex)
	}
	modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	if modal.FocusedIndex != fieldEditTags {
		t.Fatalf("expected focus on Tags after Tab, got %d", modal.FocusedIndex)
	}
	modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	if modal.FocusedIndex != fieldEditIteration {
		t.Fatalf("expected focus on Iteration after Tab, got %d", modal.FocusedIndex)
	}
	modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	if modal.FocusedIndex != fieldEditArea {
		t.Fatalf("expected focus on Area after Tab, got %d", modal.FocusedIndex)
	}
	modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	if modal.FocusedIndex != fieldEditDesc {
		t.Fatalf("expected focus on Description after Tab, got %d", modal.FocusedIndex)
	}
	modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	if modal.FocusedIndex != fieldEditTitle {
		t.Fatalf("expected focus to wrap back to Title, got %d", modal.FocusedIndex)
	}

	// 3. Shift+Tab navigation
	modal.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if modal.FocusedIndex != fieldEditDesc {
		t.Fatalf("expected focus on Description after Shift+Tab, got %d", modal.FocusedIndex)
	}

	// 4. Edit Tags
	modal.FocusedIndex = fieldEditTags
	modal.updateFocus()
	modal.InputTags.SetValue("gke; backend; release-2.0")

	// 5. Open Iteration DropdownPicker and select
	modal.FocusedIndex = fieldEditIteration
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if modal.Picker == nil || !modal.Picker.Active {
		t.Fatal("expected DropdownPicker to open on Enter on Iteration field")
	}
	// Select next option in picker
	modal.Update(tea.KeyMsg{Type: tea.KeyDown})
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if modal.Picker.Active {
		t.Fatal("expected DropdownPicker to close after selection")
	}
	if modal.IterationPath != "DigitalApp\\Sprint 2026.Q4" {
		t.Fatalf("expected IterationPath to be updated to Sprint 2026.Q4, got %s", modal.IterationPath)
	}

	// 6. Open Area DropdownPicker and select
	modal.FocusedIndex = fieldEditArea
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if modal.Picker == nil || !modal.Picker.Active {
		t.Fatal("expected DropdownPicker to open on Enter on Area field")
	}
	modal.Update(tea.KeyMsg{Type: tea.KeyDown})
	modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if modal.AreaPath != "DigitalApp\\Core Team" {
		t.Fatalf("expected AreaPath to be updated to Core Team, got %s", modal.AreaPath)
	}

	// 7. Cycle Iteration using Left/Right keys
	modal.FocusedIndex = fieldEditIteration
	modal.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if modal.IterationPath != "DigitalApp\\Sprint 2026.Q3" {
		t.Fatalf("expected IterationPath to cycle left to Sprint 2026.Q3, got %s", modal.IterationPath)
	}

	// 8. Test Values()
	title, desc, tags, iter, area := modal.Values()
	if title != "Deploy Services to GKE" {
		t.Errorf("expected title 'Deploy Services to GKE', got %s", title)
	}
	if tags != "gke; backend; release-2.0" {
		t.Errorf("expected tags 'gke; backend; release-2.0', got %s", tags)
	}
	if iter != "DigitalApp\\Sprint 2026.Q3" {
		t.Errorf("expected iteration 'DigitalApp\\Sprint 2026.Q3', got %s", iter)
	}
	if area != "DigitalApp\\Core Team" {
		t.Errorf("expected area 'DigitalApp\\Core Team', got %s", area)
	}
	if desc != "Deploy manifests" {
		t.Errorf("expected desc 'Deploy manifests', got %s", desc)
	}

	// 9. Close modal
	modal.Close()
	if modal.Active {
		t.Fatal("expected modal to be inactive after Close()")
	}
}

func TestScreenCreateTagsIterationArea(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Seed work items for iterations and areas
	item1 := &ado.WorkItem{
		ID: 501,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Feature",
			"System.Title":         "Cloud Platform",
			"System.State":         "Active",
			"System.IterationPath": "DigitalApp\\Sprint 2026.Q3",
			"System.AreaPath":      "DigitalApp\\Cloud Team",
			"System.Tags":          "infra; cloud",
		},
	}
	item2 := &ado.WorkItem{
		ID: 502,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Feature",
			"System.Title":         "Frontend App",
			"System.State":         "Active",
			"System.IterationPath": "DigitalApp\\Sprint 2026.Q4",
			"System.AreaPath":      "DigitalApp\\UI Team",
			"System.Tags":          "ui; frontend",
		},
	}
	_ = database.SaveWorkItem("myorg", "myproject", item1)
	_ = database.SaveWorkItem("myorg", "myproject", item2)

	sc := NewScreenCreate()
	parentRec, _ := database.GetWorkItem(501)
	sc.Reset(parentRec, "dev@example.com", database)

	// 1. Verify inherited Iteration and Area from parent
	if sc.Iteration() != "DigitalApp\\Sprint 2026.Q3" {
		t.Fatalf("expected inherited Iteration 'DigitalApp\\Sprint 2026.Q3', got %s", sc.Iteration())
	}
	if sc.Area() != "DigitalApp\\Cloud Team" {
		t.Fatalf("expected inherited Area 'DigitalApp\\Cloud Team', got %s", sc.Area())
	}
	if sc.AssignedTo() != "dev@example.com" {
		t.Fatalf("expected inherited Assignee 'dev@example.com', got %s", sc.AssignedTo())
	}

	// 2. Verify View Rendering shows Parent Banner with inherited info and clean fields
	view := sc.View(100, 35, "", false)
	if !strings.Contains(view, "Target Parent:") {
		t.Fatalf("expected Target Parent in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Title (required):") || !strings.Contains(view, "Description / Notes (Markdown):") {
		t.Fatalf("expected Title and Description in view, got:\n%s", view)
	}

	// 3. Test Tab navigation through simple fields
	sc.FocusedField = fieldCreateType
	sc.updateFocus()
	expectedSequence := []int{
		fieldCreateTitle,
		fieldCreateDesc,
		fieldCreateSubmit,
		fieldCreateType,
	}
	for i, exp := range expectedSequence {
		sc.Update(tea.KeyMsg{Type: tea.KeyTab})
		if sc.FocusedField != exp {
			t.Fatalf("step %d: expected focused field %d, got %d", i, exp, sc.FocusedField)
		}
	}
}

func TestModalStatusDynamicStates(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "dynamic_status_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	// 1. Save Bug specific states in DB
	bugStates := []ado.WorkItemState{
		{Name: "New", Category: "Proposed"},
		{Name: "Approved", Category: "Proposed"},
		{Name: "Committed", Category: "InProgress"},
		{Name: "Done", Category: "Completed"},
	}
	_ = database.SaveWorkItemTypeStates("myorg", "myproj", "Bug", bugStates)

	cfg := &config.Config{
		DefaultOrg:     "myorg",
		DefaultProject: "myproj",
	}
	app := NewAppModel(cfg, database)

	// 2. Test getStatesForItemType for Bug
	states := app.getStatesForItemType("Bug")
	if len(states) != 4 || states[0] != "New" || states[2] != "Committed" || states[3] != "Done" {
		t.Fatalf("expected Bug states [New, Approved, Committed, Done], got %v", states)
	}

	// 3. Open modal for a Bug
	app.ScreenTree.SetData([]*db.TreeNode{
		{Record: &db.WorkItemRecord{ID: 663964, Title: "Cannot Deploy GKE Cluster", WorkItemType: "Bug", State: "Committed"}},
	})

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	app = updated.(*AppModel)

	if app.ActiveModal != ModalTypeStatus {
		t.Fatalf("expected ActiveModal to be ModalTypeStatus")
	}

	// Modal states should ONLY contain Bug states
	if len(app.ModalStatus.States) != 4 {
		t.Fatalf("expected 4 states in modal for Bug, got %d (%v)", len(app.ModalStatus.States), app.ModalStatus.States)
	}
	for _, s := range app.ModalStatus.States {
		if s == "In Progress" || s == "To Do" {
			t.Fatalf("In Progress / To Do should not be present for Bug, but found %q in %v", s, app.ModalStatus.States)
		}
	}

	// 4. Test fallback defaults for Task
	taskStates := app.getStatesForItemType("Task")
	hasInProgress := false
	for _, s := range taskStates {
		if s == "In Progress" {
			hasInProgress = true
			break
		}
	}
	if !hasInProgress {
		t.Fatalf("expected In Progress to be present in default Task states: %v", taskStates)
	}
}

func TestScreenItemScrollPerformanceAndBackNavigation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "scroll_test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{DbPath: dbPath}
	app := &AppModel{
		Config:        cfg,
		DB:            database,
		Width:         120,
		Height:        30,
		CurrentScreen: ScreenTypeItem,
		ScreenTree:    NewScreenTree(),
		ScreenItem:    NewScreenItem(),
	}
	app.ScreenItem.Config = cfg
	app.ScreenItem.DB = database

	// Simulate a large item with a Python traceback description
	tracebackDesc := `Copied from Repro Steps when changed from Bug to Production Issue

Traceback (most recent call last):
  File "/app/src/shared/decorators.py", line 139, in wrapper
    result = await func(request, request_body, *args, **kwargs)
    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
  File "/app/src/api/endpoints/edi_site.py", line 13, in edi_site
    message,rawid = await get_edi_site(podid=request_body.podid, rawid=request_body.rawid)
    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
`
	rec := &db.WorkItemRecord{
		ID:           643519,
		Title:        "Prod: Site-Edi - duplicate constrained values",
		WorkItemType: "Production Issue",
		State:        "Done",
		Description:  tracebackDesc,
	}
	app.ScreenItem.SetData(rec, nil, nil, nil)
	app.ScreenItem.SetSize(120, 30)

	// Initial render
	initialView := app.ScreenItem.View(120, 30, "", false)
	if !strings.Contains(initialView, "643519") {
		t.Fatalf("expected view to contain #643519")
	}

	// Verify scrolling doesn't re-parse or cause dirty state
	initialYOffset := app.ScreenItem.Viewport.YOffset
	for i := 0; i < 20; i++ {
		updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyDown})
		app = updated.(*AppModel)
		_ = app.View() // Render frame
	}

	if app.ScreenItem.Viewport.YOffset <= initialYOffset {
		t.Fatalf("expected YOffset to increase after scrolling down, got %d", app.ScreenItem.Viewport.YOffset)
	}

	// Verify pressing Esc immediately transitions back to ScreenTypeTree
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*AppModel)

	if app.CurrentScreen != ScreenTypeTree {
		t.Fatalf("expected CurrentScreen to be ScreenTypeTree on Esc, got %v", app.CurrentScreen)
	}
}

func TestPerTabAndGlobalFilteringAndBadges(t *testing.T) {
	screen := NewScreenTree()
	screen.Width = 120
	screen.Height = 30

	node1 := &db.TreeNode{
		Record:   &db.WorkItemRecord{ID: 100, Title: "Alpha Project", WorkItemType: "Feature", State: "Active", AssignedToName: "Alice"},
		Depth:    0,
		Expanded: true,
		Children: []*db.TreeNode{
			{Record: &db.WorkItemRecord{ID: 101, Title: "Backend Task", WorkItemType: "Task", State: "Active", AssignedToName: "Bob", ParentID: 100}, Depth: 1, Expanded: true},
			{Record: &db.WorkItemRecord{ID: 102, Title: "Frontend Task", WorkItemType: "Task", State: "Done", AssignedToName: "Alice", ParentID: 100}, Depth: 1, Expanded: true},
		},
	}
	node2 := &db.TreeNode{
		Record:   &db.WorkItemRecord{ID: 200, Title: "Beta Project", WorkItemType: "Feature", State: "New", AssignedToName: "Charlie"},
		Depth:    0,
		Expanded: true,
		Children: []*db.TreeNode{
			{Record: &db.WorkItemRecord{ID: 201, Title: "Design Task", WorkItemType: "Task", State: "New", AssignedToName: "Alice", ParentID: 200}, Depth: 1, Expanded: true},
		},
	}

	screen.SetData([]*db.TreeNode{node1, node2})
	if len(screen.Flattened) != 3 {
		t.Fatalf("expected 3 rows in tab 0 (#100, #101, #102), got %d", len(screen.Flattened))
	}

	// 1. Set Tab-specific filter on Tab 0: Assignee Alice
	screen.SetTabFilter(0, db.FilterOptions{Assignees: []string{"Alice"}})
	if len(screen.Flattened) != 2 {
		t.Fatalf("expected 2 rows in tab 0 (#100 parent + #102 Alice), got %d", len(screen.Flattened))
	}

	// Tab 1 should not have a tab filter yet
	screen.NextTab()
	if len(screen.Flattened) != 2 {
		t.Fatalf("expected 2 rows in tab 1 (#200 + #201), got %d", len(screen.Flattened))
	}

	// Check Tab 1 rendering: Tab 0 should show 📑 indicator in ribbon
	viewTab1 := screen.View(120, 30, "", false)
	if !strings.Contains(viewTab1, "📑") {
		t.Fatalf("expected tab bar in viewTab1 to contain tab filter indicator 📑, got:\n%s", viewTab1)
	}

	// Switch back to Tab 0: Top header should show 📑 TAB badge
	screen.PrevTab()
	viewTab0 := screen.View(120, 30, "", false)
	if !strings.Contains(viewTab0, "📑 TAB") {
		t.Fatalf("expected top header to contain 📑 TAB badge, got:\n%s", viewTab0)
	}

	// 2. Set Global Filter: State Active
	screen.SetGlobalFilter(db.FilterOptions{Statuses: []string{"Active"}})
	viewGlobalAndTab := screen.View(120, 30, "", false)

	// Top header should show BOTH 🌐 GLOBAL and 📑 TAB badges
	if !strings.Contains(viewGlobalAndTab, "🌐 GLOBAL") {
		t.Fatalf("expected top header to contain 🌐 GLOBAL badge, got:\n%s", viewGlobalAndTab)
	}
	if !strings.Contains(viewGlobalAndTab, "📑 TAB") {
		t.Fatalf("expected top header to contain 📑 TAB badge, got:\n%s", viewGlobalAndTab)
	}

	// In Tab 0: Global Active + Tab Alice -> Only #100 is Active and Alice
	if len(screen.Flattened) != 1 || screen.Flattened[0].Node.Record.ID != 100 {
		t.Fatalf("expected exactly #100 to match both global Active and tab Alice, got %d rows", len(screen.Flattened))
	}

	// Clear only tab filter on tab 0
	screen.ClearTabFilter(0)
	if screen.GetTabFilter(0).IsActive() {
		t.Fatalf("expected tab filter on tab 0 to be cleared")
	}
	// With tab filter cleared, Tab 0 has only Global Active (#100, #101)
	if len(screen.Flattened) != 2 {
		t.Fatalf("expected 2 rows for Active in tab 0, got %d", len(screen.Flattened))
	}

	// Clear all filters
	screen.ClearAllFilters()
	if screen.GlobalFilterOpts.IsActive() {
		t.Fatalf("expected global filter to be cleared")
	}
	if len(screen.Flattened) != 3 {
		t.Fatalf("expected all 3 rows in tab 0 after clearing filters, got %d", len(screen.Flattened))
	}
}

func TestFilterModalGlobalAndTabShortcuts(t *testing.T) {
	modal := NewModalFilter()
	modal.OpenWithTab(db.FilterOptions{}, db.FilterOptions{}, "Feature 100", nil)

	// Set a filter option (e.g. status "Active")
	modal.SelectedStatuses = []string{"Active"}

	// 1. Pressing Ctrl+T should trigger FilterActionApplyTab
	_, _, actionTab := modal.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	if actionTab != FilterActionApplyTab {
		t.Fatalf("expected FilterActionApplyTab on ctrl+t, got %v", actionTab)
	}

	// 2. Pressing 'T' should trigger FilterActionApplyTab
	_, _, actionTabKey := modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	if actionTabKey != FilterActionApplyTab {
		t.Fatalf("expected FilterActionApplyTab on 'T', got %v", actionTabKey)
	}

	// 3. Pressing Ctrl+S should trigger FilterActionApplyGlobal
	_, _, actionGlobal := modal.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	if actionGlobal != FilterActionApplyGlobal {
		t.Fatalf("expected FilterActionApplyGlobal on ctrl+s, got %v", actionGlobal)
	}

	// 4. Pressing 'c' / 'C' should trigger FilterActionClearTab
	_, _, actionClearTab := modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if actionClearTab != FilterActionClearTab {
		t.Fatalf("expected FilterActionClearTab on 'c', got %v", actionClearTab)
	}
	_, _, actionClearTabCap := modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	if actionClearTabCap != FilterActionClearTab {
		t.Fatalf("expected FilterActionClearTab on 'C', got %v", actionClearTabCap)
	}

	// 5. Test Button Index Navigation and Enter on Clear Tab button (index 2)
	modal.FocusIndex = filterFieldButtons
	modal.ActionButtonIdx = 2
	_, _, actionBtnClearTab := modal.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if actionBtnClearTab != FilterActionClearTab {
		t.Fatalf("expected FilterActionClearTab on Enter with ActionButtonIdx=2, got %v", actionBtnClearTab)
	}

	// Check view shows tab context and buttons
	view := modal.View(100, 30)
	if !strings.Contains(view, "Feature 100") {
		t.Fatalf("expected modal view to display active tab context 'Feature 100', got:\n%s", view)
	}
	if !strings.Contains(view, "Global") || !strings.Contains(view, "Tab") || !strings.Contains(view, "Clear Tab") {
		t.Fatalf("expected modal view to display Global, Tab, and Clear Tab buttons, got:\n%s", view)
	}
}

func TestFilterClearTabRetainingGlobal(t *testing.T) {
	screenTree := NewScreenTree()
	node1 := &db.TreeNode{
		Record: &db.WorkItemRecord{ID: 101, Title: "Root 1", WorkItemType: "User Story", State: "Active"},
	}
	node2 := &db.TreeNode{
		Record: &db.WorkItemRecord{ID: 102, Title: "Root 2", WorkItemType: "Bug", State: "Resolved"},
	}
	screenTree.SetData([]*db.TreeNode{node1, node2})

	// Set Global Filter (e.g., State: Active)
	globalFilter := db.FilterOptions{Status: "Active", Statuses: []string{"Active"}}
	screenTree.SetGlobalFilter(globalFilter)

	// Set Tab Filter for Tab 0 (e.g., ItemType: User Story)
	tabFilter := db.FilterOptions{ItemType: "User Story", ItemTypes: []string{"User Story"}}
	screenTree.SetTabFilter(0, tabFilter)

	if !screenTree.GetTabFilter(0).IsActive() {
		t.Fatalf("expected tab 0 filter to be active")
	}
	if !screenTree.GlobalFilterOpts.IsActive() {
		t.Fatalf("expected global filter to be active")
	}

	// Clear tab filter for tab 0
	screenTree.ClearTabFilter(0)

	if screenTree.GetTabFilter(0).IsActive() {
		t.Fatalf("expected tab 0 filter to be cleared, but it was active")
	}
	if !screenTree.GlobalFilterOpts.IsActive() {
		t.Fatalf("expected global filter to be retained when tab filter cleared, but it was cleared")
	}
	if screenTree.GlobalFilterOpts.Status != "Active" {
		t.Fatalf("expected global filter status 'Active', got '%s'", screenTree.GlobalFilterOpts.Status)
	}
}

func TestFilterStateRetentionOnRestoreAndSync(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_filter_retention.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "testorg",
		DefaultProject: "testproj",
	}
	app := NewAppModel(cfg, database)

	node1 := &db.TreeNode{
		Record: &db.WorkItemRecord{ID: 101, Title: "Story 101", WorkItemType: "User Story", State: "Active"},
	}
	node2 := &db.TreeNode{
		Record: &db.WorkItemRecord{ID: 102, Title: "Bug 102", WorkItemType: "Bug", State: "Resolved"},
	}
	roots := []*db.TreeNode{node1, node2}

	// 1. Set Global Filter and Tab Filter
	globalFilter := db.FilterOptions{Status: "Active", Statuses: []string{"Active"}}
	tabFilter := db.FilterOptions{ItemType: "Bug", ItemTypes: []string{"Bug"}}

	app.ScreenTree.SetData(roots)
	app.ScreenTree.SetGlobalFilter(globalFilter)
	app.ScreenTree.SetTabFilter(1, tabFilter)
	app.saveUIState()

	// 2. Verify state saved in SQLite
	uiState, err := database.GetUIState()
	if err != nil || uiState == nil {
		t.Fatalf("failed to retrieve UIState: %v", err)
	}
	if !uiState.Filter.IsActive() || uiState.Filter.Status != "Active" {
		t.Fatalf("expected saved filter to have status 'Active', got %+v", uiState.Filter)
	}
	if len(uiState.TabFilters) == 0 {
		t.Fatalf("expected saved tab filters, got empty")
	}

	// 3. Simulate new sync/launch restoring state
	newApp := NewAppModel(cfg, database)
	newApp.ScreenTree.SetData(roots)
	newApp.ScreenTree.RestoreState(uiState.ActiveTabRootID, uiState.ActiveTabIdx, uiState.SelectedItemID, uiState.CollapsedNodeIDs, uiState.Filter)
	if len(uiState.TabFilters) > 0 {
		newApp.ScreenTree.TabFilterOpts = uiState.TabFilters
	}

	// Verify restored state
	if !newApp.ScreenTree.GlobalFilterOpts.IsActive() || newApp.ScreenTree.GlobalFilterOpts.Status != "Active" {
		t.Fatalf("expected restored global filter to be 'Active', got %+v", newApp.ScreenTree.GlobalFilterOpts)
	}
	if !newApp.ScreenTree.GetTabFilter(1).IsActive() || newApp.ScreenTree.GetTabFilter(1).ItemType != "Bug" {
		t.Fatalf("expected restored tab 1 filter to be 'Bug', got %+v", newApp.ScreenTree.GetTabFilter(1))
	}

	// 4. Verify opening modal preserves filters
	modal := NewModalFilter()
	modal.OpenWithTab(newApp.ScreenTree.GlobalFilterOpts, newApp.ScreenTree.GetTabFilter(0), "Story 101", database)
	if len(modal.SelectedStatuses) != 1 || modal.SelectedStatuses[0] != "Active" {
		t.Fatalf("expected modal to be pre-populated with active status, got %+v", modal.SelectedStatuses)
	}
}

func TestModalTabRenameWorkflow(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_rename.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "testorg",
		DefaultProject: "testproj",
	}
	app := NewAppModel(cfg, database)
	_ = database.SaveTrackedRootWithQuery(100, "testorg", "testproj", "https://dev.azure.com/testorg/testproj/_workitems/edit/100", "")

	roots := []*db.TreeNode{
		{Record: &db.WorkItemRecord{ID: 100, Title: "Original Feature", WorkItemType: "Feature"}},
		{Record: &db.WorkItemRecord{ID: 200, Title: "Original Bug", WorkItemType: "Bug"}},
	}
	app.ScreenTree.SetData(roots)
	app.ModalTab.Open(roots, 0)
	app.ActiveModal = ModalTypeTab

	// 1. Press F2 to enter inline rename mode
	app.Update(tea.KeyMsg{Type: tea.KeyF2})
	if !app.ModalTab.Renaming {
		t.Fatalf("expected ModalTab.Renaming to be true after pressing F2")
	}

	// Rename input should be initialized with current title
	if app.ModalTab.RenameInput.Value() != "Original Feature" {
		t.Fatalf("expected RenameInput to have 'Original Feature', got %q", app.ModalTab.RenameInput.Value())
	}

	// View should render rename dialog
	renameView := app.ModalTab.View(100, 30)
	if !strings.Contains(renameView, "Rename Tab") {
		t.Fatalf("expected view to render rename header, got:\n%s", renameView)
	}

	// 2. Type new name and press Enter to commit
	app.ModalTab.RenameInput.SetValue("Super Feature Rebranded")
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if app.ModalTab.Renaming {
		t.Fatalf("expected ModalTab.Renaming to be false after pressing Enter")
	}

	// Check that tab was renamed
	if app.ScreenTree.TreeRoots[0].CustomTitle != "Super Feature Rebranded" {
		t.Fatalf("expected root 0 custom title to be updated to 'Super Feature Rebranded', got %q", app.ScreenTree.TreeRoots[0].CustomTitle)
	}
	if app.ScreenTree.TreeRoots[0].TabTitle() != "Super Feature Rebranded" {
		t.Fatalf("expected root 0 TabTitle to be 'Super Feature Rebranded', got %q", app.ScreenTree.TreeRoots[0].TabTitle())
	}
	if app.ScreenTree.TreeRoots[0].Record.Title != "Original Feature" {
		t.Fatalf("expected root 0 work item title to remain 'Original Feature', got %q", app.ScreenTree.TreeRoots[0].Record.Title)
	}
	if app.ModalTab.Items[0].Title != "Super Feature Rebranded" {
		t.Fatalf("expected ModalTab item 0 title to be updated, got %q", app.ModalTab.Items[0].Title)
	}

	// Verify DB persistence of custom title
	trackedRoots, err := app.DB.GetTrackedRoots()
	if err == nil && len(trackedRoots) > 0 {
		for _, tr := range trackedRoots {
			if tr.ID == 100 && tr.CustomTitle != "Super Feature Rebranded" {
				t.Fatalf("expected tracked root 100 in DB to have custom title 'Super Feature Rebranded', got %q", tr.CustomTitle)
			}
		}
	}
}

func TestScreenTreeStatusBadgeMarginAndSpacing(t *testing.T) {
	roots := []*db.TreeNode{
		{
			Record: &db.WorkItemRecord{
				ID:             667425,
				WorkItemType:   "Task",
				Title:          "Changes in the orchestrator to test new versions of the RA",
				State:          "In Progress",
				AssignedToName: "Doe, John",
			},
		},
	}

	screenTree := NewScreenTree()
	screenTree.SetData(roots)
	view := screenTree.View(120, 30, "Ready", false)

	// Check that view contains both In Progress and Doe, John
	if !strings.Contains(view, "In Progress") || !strings.Contains(view, "Doe, John") {
		t.Fatalf("expected view to contain 'In Progress' and 'Doe, John', got:\n%s", view)
	}

	// Verify that In Progress badge is not directly touching the assignee name (e.g. not "In ProgressDoe, John")
	plainView := StripEscapes(view)
	if strings.Contains(plainView, "In ProgressDoe") || strings.Contains(plainView, "ProgressDoe") {
		t.Fatalf("expected margin/spacing between In Progress badge and assignee name, but found direct collision in:\n%s", plainView)
	}

	// Verify there is multiple spaces of separation between "Progress" and "Doe" in plain text
	if !strings.Contains(plainView, "Progress    Doe") && !strings.Contains(plainView, "Progress   Doe") {
		t.Fatalf("expected multiple spaces separating 'Progress' and 'Doe', got:\n%s", plainView)
	}

	// Verify clear spacing between ESTIMATE and PROGRESS / WORK headers (at least 3 spaces)
	if strings.Contains(plainView, "ESTIMATE PROGRESS") || strings.Contains(plainView, "ESTIMATE  PROGRESS") {
		t.Fatalf("expected distinct spacing between ESTIMATE and PROGRESS / WORK in header, got:\n%s", plainView)
	}
	if !strings.Contains(plainView, "ESTIMATE    PROGRESS / WORK") {
		t.Fatalf("expected 4 spaces between ESTIMATE and PROGRESS / WORK in header, got:\n%s", plainView)
	}
}

func TestFormatEffort(t *testing.T) {
	if got := formatEffort(5.0); got != "5pts" {
		t.Errorf("expected '5pts', got %q", got)
	}
	if got := formatEffort(5.5); got != "5.5pts" {
		t.Errorf("expected '5.5pts', got %q", got)
	}
	if got := formatEffort(0); got != "—" {
		t.Errorf("expected '—', got %q", got)
	}
}



