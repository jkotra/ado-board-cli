package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ado-board-cli/pkg/ado"
)

func TestDatabaseCRUD(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "adotest_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	database, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// 1. Save and Get Tracked Root
	err = database.SaveTrackedRoot(656842, "my-org", "my-project", "https://dev.azure.com/my-org/my-project/_workitems/edit/656842")
	if err != nil {
		t.Fatalf("failed to save tracked root: %v", err)
	}

	roots, err := database.GetTrackedRoots()
	if err != nil {
		t.Fatalf("failed to get tracked roots: %v", err)
	}
	if len(roots) != 1 || roots[0].ID != 656842 {
		t.Fatalf("expected 1 root with ID 656842, got %v", roots)
	}

	// 2. Save and Get Work Item
	mockItem := &ado.WorkItem{
		ID:  656842,
		URL: "https://dev.azure.com/my-org/my-project/_apis/wit/workitems/656842",
		Fields: map[string]interface{}{
			"System.WorkItemType":                       "Feature",
			"System.Title":                              "Test GKE Feature",
			"System.State":                              "In Progress",
			"System.AssignedTo":                         map[string]interface{}{"displayName": "John Doe", "uniqueName": "john.doe@example.com"},
			"System.CreatedBy":                          map[string]interface{}{"displayName": "John Doe", "uniqueName": "john.doe@example.com"},
			"System.CreatedDate":                        "2026-08-20T10:00:00Z",
			"System.ChangedDate":                        "2026-08-20T12:00:00Z",
			"System.Description":                        "<p>Test description with <b>HTML</b></p>",
			"Custom.OriginalEstimate":                   40.0,
			"Custom.CompletedWork":                      10.0,
			"Microsoft.VSTS.Scheduling.RemainingWork":   30.0,
			"System.AreaPath":                           "my-project\\GKE",
			"System.IterationPath":                      "my-project\\Sprint 1",
			"System.Tags":                               "GKE; Cloud",
		},
	}

	err = database.SaveWorkItem("my-org", "my-project", mockItem)
	if err != nil {
		t.Fatalf("failed to save work item: %v", err)
	}

	// 3. Query Work Item
	itemRec, err := database.GetWorkItem(656842)
	if err != nil {
		t.Fatalf("failed to get work item: %v", err)
	}
	if itemRec.Title != "Test GKE Feature" {
		t.Errorf("expected title 'Test GKE Feature', got '%s'", itemRec.Title)
	}
	if itemRec.OriginalEstimate != 40 {
		t.Errorf("expected OriginalEstimate 40, got %f", itemRec.OriginalEstimate)
	}
	if itemRec.AssignedToEmail != "john.doe@example.com" {
		t.Errorf("expected email 'john.doe@example.com', got '%s'", itemRec.AssignedToEmail)
	}

	// 4. Save and Get Comments (Verify latest on top)
	mockComment1 := &ado.CommentItem{
		ID: 1001,
		CreatedBy: &ado.IdentityRef{
			DisplayName: "John Doe",
			UniqueName:  "john.doe@example.com",
		},
		CreatedDate: time.Now().Add(-2 * time.Hour),
		Text:        "Older comment content",
	}
	mockComment2 := &ado.CommentItem{
		ID: 1002,
		CreatedBy: &ado.IdentityRef{
			DisplayName: "John Doe",
			UniqueName:  "john.doe@example.com",
		},
		CreatedDate: time.Now().Add(-1 * time.Hour),
		Text:        "Newer latest comment content",
	}

	err = database.SaveComments(656842, []*ado.CommentItem{mockComment1, mockComment2})
	if err != nil {
		t.Fatalf("failed to save comments: %v", err)
	}

	comments, err := database.GetComments(656842)
	if err != nil {
		t.Fatalf("failed to get comments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].Text != "Newer latest comment content" {
		t.Fatalf("expected latest comment on top, got %q", comments[0].Text)
	}
	if comments[1].Text != "Older comment content" {
		t.Fatalf("expected older comment second, got %q", comments[1].Text)
	}

	// 5. Distinct queries
	states, err := database.GetDistinctStates()
	if err != nil || len(states) != 1 || states[0] != "In Progress" {
		t.Errorf("expected distinct states ['In Progress'], got %v (err: %v)", states, err)
	}

	types, err := database.GetDistinctWorkItemTypes()
	if err != nil || len(types) != 1 || types[0] != "Feature" {
		t.Errorf("expected distinct types ['Feature'], got %v (err: %v)", types, err)
	}

	assignees, err := database.GetDistinctAssignees()
	if err != nil || len(assignees) == 0 {
		t.Errorf("expected at least 1 assignee, got %v (err: %v)", assignees, err)
	}

	// 6. Delete Work Item
	err = database.DeleteWorkItem(656842)
	if err != nil {
		t.Fatalf("failed to delete work item: %v", err)
	}

	_, err = database.GetWorkItem(656842)
	if err == nil {
		t.Fatalf("expected error when getting deleted work item, got nil")
	}

	// 7. Delete Tracked Root
	err = database.DeleteTrackedRoot(656842)
	if err != nil {
		t.Fatalf("failed to delete tracked root: %v", err)
	}
	rootsAfter, _ := database.GetTrackedRoots()
	if len(rootsAfter) != 0 {
		t.Fatalf("expected 0 roots after deletion, got %d", len(rootsAfter))
	}
}

func TestFilterOptionsLogic(t *testing.T) {
	rec1 := &WorkItemRecord{
		ID:               101,
		Title:            "Fix memory leak in ingress",
		WorkItemType:     "Bug",
		State:            "Active",
		AssignedToName:   "John Doe",
		AssignedToEmail:  "john@example.com",
		Tags:             "backend; gke",
	}

	rec2 := &WorkItemRecord{
		ID:               102,
		Title:            "Build CI pipeline",
		WorkItemType:     "Task",
		State:            "To Do",
		AssignedToName:   "Dev Team",
		AssignedToEmail:  "dev@example.com",
		Tags:             "ci; devops",
	}

	// 1. Inactive filter matches all
	emptyFilter := FilterOptions{}
	if !emptyFilter.MatchesItem(rec1) || !emptyFilter.MatchesItem(rec2) {
		t.Error("expected empty filter to match all items")
	}

	// 2. AND logic (Status=Active AND Type=Bug)
	andFilter := FilterOptions{
		Status:   "Active",
		Op1:      "AND",
		ItemType: "Bug",
	}
	if !andFilter.MatchesItem(rec1) {
		t.Error("expected andFilter to match rec1")
	}
	if andFilter.MatchesItem(rec2) {
		t.Error("expected andFilter to NOT match rec2")
	}

	// 3. OR logic (Status=Active OR Type=Task)
	orFilter := FilterOptions{
		Status:   "Active",
		Op1:      "OR",
		ItemType: "Task",
	}
	if !orFilter.MatchesItem(rec1) {
		t.Error("expected orFilter to match rec1 (Status Active)")
	}
	if !orFilter.MatchesItem(rec2) {
		t.Error("expected orFilter to match rec2 (Type Task)")
	}

	// 4. Search term matching
	searchFilter := FilterOptions{
		SearchTerm: "ingress",
	}
	if !searchFilter.MatchesItem(rec1) {
		t.Error("expected searchFilter to match rec1 (Title contains ingress)")
	}
	if searchFilter.MatchesItem(rec2) {
		t.Error("expected searchFilter to NOT match rec2")
	}

	// 5. Multi-select statuses and types
	multiFilter := FilterOptions{
		Statuses:  []string{"Active", "To Do"},
		ItemTypes: []string{"Task", "Bug"},
	}
	if !multiFilter.MatchesItem(rec1) {
		t.Error("expected multiFilter to match rec1 (Active, Bug)")
	}
	if !multiFilter.MatchesItem(rec2) {
		t.Error("expected multiFilter to match rec2 (To Do, Task)")
	}

	nonMatchingMulti := FilterOptions{
		Statuses:  []string{"Closed", "Resolved"},
		ItemTypes: []string{"Task", "Bug"},
	}
	if nonMatchingMulti.MatchesItem(rec1) || nonMatchingMulti.MatchesItem(rec2) {
		t.Error("expected nonMatchingMulti to NOT match active/to do items")
	}
}

func TestFilterOptionsMultiSelectAndAssignees(t *testing.T) {
	rec1 := &WorkItemRecord{
		ID:              101,
		Title:           "Frontend auth issue",
		WorkItemType:    "Bug",
		State:           "Active",
		AssignedToName:  "John Doe",
		AssignedToEmail: "john@example.com",
	}
	rec2 := &WorkItemRecord{
		ID:              102,
		Title:           "Database migration script",
		WorkItemType:    "Task",
		State:           "New",
		AssignedToName:  "Miller, Erin Grace",
		AssignedToEmail: "erin@example.com",
	}
	rec3 := &WorkItemRecord{
		ID:              103,
		Title:           "Unassigned backlog item",
		WorkItemType:    "Product Backlog Item",
		State:           "New",
		AssignedToName:  "",
		AssignedToEmail: "",
	}

	// 1. Test GetEffective* methods
	f1 := FilterOptions{
		Statuses:  []string{"[All]", "Active", " "},
		Assignees: []string{"Unassigned", "all"},
		ItemTypes: []string{"Bug", "Task"},
	}
	if effStatus := f1.GetEffectiveStatuses(); len(effStatus) != 1 || effStatus[0] != "Active" {
		t.Fatalf("expected [Active], got %+v", effStatus)
	}
	if effAssignees := f1.GetEffectiveAssignees(); len(effAssignees) != 1 || effAssignees[0] != "Unassigned" {
		t.Fatalf("expected [Unassigned], got %+v", effAssignees)
	}
	if effTypes := f1.GetEffectiveItemTypes(); len(effTypes) != 2 {
		t.Fatalf("expected [Bug, Task], got %+v", effTypes)
	}

	// 2. Test Multi-Assignee Matching
	assigneeFilter := FilterOptions{
		Assignees: []string{"John Doe <john@example.com>", "Unassigned"},
	}
	if !assigneeFilter.MatchesItem(rec1) {
		t.Error("expected assigneeFilter to match rec1 (John Doe)")
	}
	if assigneeFilter.MatchesItem(rec2) {
		t.Error("expected assigneeFilter to NOT match rec2 (Erin)")
	}
	if !assigneeFilter.MatchesItem(rec3) {
		t.Error("expected assigneeFilter to match rec3 (Unassigned)")
	}

	// 3. Test SummaryString()
	summarySingle := FilterOptions{
		Status:   "Active",
		Assignee: "John",
		ItemType: "Bug",
		Op1:      "AND",
		Op2:      "AND",
	}.SummaryString()
	if !strings.Contains(summarySingle, "Status: Active") || !strings.Contains(summarySingle, "Assignee: John") || !strings.Contains(summarySingle, "Type: Bug") {
		t.Fatalf("unexpected single summary string: %q", summarySingle)
	}

	summaryMulti := FilterOptions{
		Statuses:  []string{"Active", "New"},
		Assignees: []string{"John Doe <john@example.com>", "Erin Miller"},
		ItemTypes: []string{"Bug", "Task"},
		Op1:       "AND",
		Op2:       "OR",
	}.SummaryString()
	if !strings.Contains(summaryMulti, "Status: [Active, New]") || !strings.Contains(summaryMulti, "Assignees: [John Doe, Erin Miller]") || !strings.Contains(summaryMulti, "Types: [Bug, Task]") {
		t.Fatalf("unexpected multi summary string: %q", summaryMulti)
	}

	// 4. Test MatchesNodeOrDescendant with Multi-Criteria
	childNode := &TreeNode{
		Record:   rec1, // Active Bug assigned to John
		Children: nil,
	}
	parentNode := &TreeNode{
		Record: &WorkItemRecord{ID: 100, Title: "Feature parent", WorkItemType: "Feature", State: "Active"},
		Children: []*TreeNode{
			childNode,
		},
	}

	bugFilter := FilterOptions{
		ItemTypes: []string{"Bug"},
	}
	if !bugFilter.MatchesNodeOrDescendant(parentNode) {
		t.Error("expected parentNode to match descendant bug filter")
	}

	epicFilter := FilterOptions{
		ItemTypes: []string{"Epic"},
	}
	if epicFilter.MatchesNodeOrDescendant(parentNode) {
		t.Error("expected parentNode to NOT match epic filter")
	}
}

func TestResolveAssignee(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "adotest_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	database, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Seed identities
	identities := []*ado.IdentityRef{
		{ID: "guid-1", DisplayName: "Doe, John", UniqueName: "john.doe@example.com"},
		{ID: "guid-2", DisplayName: "Taylor, Henry", UniqueName: "henry.taylor@example.com"},
		{ID: "guid-3", DisplayName: "Wilson, Frank Lee", UniqueName: "frank.wilson@example.com"},
	}
	if err := database.SaveIdentities(identities); err != nil {
		t.Fatalf("failed to save identities: %v", err)
	}

	// Test 1: Empty or unassigned
	if res, unassign := database.ResolveAssignee(""); !unassign || res != "" {
		t.Errorf("expected empty string to be unassign, got (%q, %v)", res, unassign)
	}
	if res, unassign := database.ResolveAssignee("unassigned"); !unassign || res != "" {
		t.Errorf("expected 'unassigned' to be unassign, got (%q, %v)", res, unassign)
	}
	if res, unassign := database.ResolveAssignee("none"); !unassign || res != "" {
		t.Errorf("expected 'none' to be unassign, got (%q, %v)", res, unassign)
	}

	// Test 2: Angle bracket extraction
	if res, unassign := database.ResolveAssignee("Doe, John <john.doe@example.com>"); unassign || res != "john.doe@example.com" {
		t.Errorf("expected 'john.doe@example.com', got (%q, %v)", res, unassign)
	}

	// Test 3: Raw email
	if res, unassign := database.ResolveAssignee("henry.taylor@example.com"); unassign || res != "henry.taylor@example.com" {
		t.Errorf("expected 'henry.taylor@example.com', got (%q, %v)", res, unassign)
	}

	// Test 4: Display name resolution to email
	if res, unassign := database.ResolveAssignee("Taylor, Henry"); unassign || res != "henry.taylor@example.com" {
		t.Errorf("expected 'henry.taylor@example.com', got (%q, %v)", res, unassign)
	}

// Test 5: Partial name resolution
	if res, unassign := database.ResolveAssignee("Frank"); unassign || res != "frank.wilson@example.com" {
		t.Errorf("expected 'frank.wilson@example.com', got (%q, %v)", res, unassign)
	}
}

func TestDefaultSettingsSeeding(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "adotest_settings_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	database, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Verify open_in_browser_cmd is seeded as empty by default
	browserCmd, err := database.GetSetting("open_in_browser_cmd")
	if err != nil {
		t.Fatalf("expected open_in_browser_cmd to be found in settings, got error: %v", err)
	}
	if browserCmd != "" {
		t.Fatalf("expected open_in_browser_cmd to be empty string by default, got: %q", browserCmd)
	}

	// Verify pat_token is NOT seeded with a default token
	patToken, err := database.GetSetting("pat_token")
	if err == nil && patToken != "" {
		t.Fatalf("expected pat_token to NOT have a default value in db, got: %q", patToken)
	}

	// Verify gemini_api_key is NOT seeded with a default token
	geminiKey, err := database.GetSetting("gemini_api_key")
	if err == nil && geminiKey != "" {
		t.Fatalf("expected gemini_api_key to NOT have a default value in db, got: %q", geminiKey)
	}
}

func TestUIStatePersistence(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "adotest_uistate_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	database, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// 1. Initial UI state defaults
	initState, err := database.GetUIState()
	if err != nil {
		t.Fatalf("failed to get initial UI state: %v", err)
	}
	if initState.GeminiDockHeight != 8 {
		t.Errorf("expected default dock height 8, got %d", initState.GeminiDockHeight)
	}

	// 2. Save and retrieve modified UI state
	saveState := &UIState{
		ActiveTabRootID:  656842,
		ActiveTabIdx:     2,
		SelectedItemID:   123456,
		CollapsedNodeIDs: []int{656842, 656843},
		GeminiDockHeight: 14,
		Filter: FilterOptions{
			Status:     "Active",
			Assignee:   "user@example.com",
			ItemType:   "Bug",
			SearchTerm: "GKE",
		},
	}
	if err := database.SaveUIState(saveState); err != nil {
		t.Fatalf("failed to save UI state: %v", err)
	}

	loadedState, err := database.GetUIState()
	if err != nil {
		t.Fatalf("failed to get UI state: %v", err)
	}
	if loadedState.ActiveTabRootID != 656842 {
		t.Errorf("expected ActiveTabRootID 656842, got %d", loadedState.ActiveTabRootID)
	}
	if loadedState.ActiveTabIdx != 2 {
		t.Errorf("expected ActiveTabIdx 2, got %d", loadedState.ActiveTabIdx)
	}
	if loadedState.SelectedItemID != 123456 {
		t.Errorf("expected SelectedItemID 123456, got %d", loadedState.SelectedItemID)
	}
	if loadedState.GeminiDockHeight != 14 {
		t.Errorf("expected GeminiDockHeight 14, got %d", loadedState.GeminiDockHeight)
	}
	if len(loadedState.CollapsedNodeIDs) != 2 || loadedState.CollapsedNodeIDs[0] != 656842 || loadedState.CollapsedNodeIDs[1] != 656843 {
		t.Errorf("expected CollapsedNodeIDs [656842, 656843], got %v", loadedState.CollapsedNodeIDs)
	}
	if loadedState.Filter.Status != "Active" || loadedState.Filter.Assignee != "user@example.com" || loadedState.Filter.ItemType != "Bug" || loadedState.Filter.SearchTerm != "GKE" {
		t.Errorf("expected filter options preserved, got %+v", loadedState.Filter)
	}
}

func TestGetNearestParentIteration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_parent_iter_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	database, err := Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// 1. Root Epic with Iteration
	epic := &ado.WorkItem{
		ID: 100,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Epic",
			"System.Title":         "Root Epic",
			"System.IterationPath": "Project\\Sprint 2026.Q3",
		},
	}
	_ = database.SaveWorkItem("org", "proj", epic)

	// 2. Feature with no explicit iteration, parent is 100
	feature := &ado.WorkItem{
		ID: 101,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Feature",
			"System.Title":         "Middle Feature",
			"System.IterationPath": "",
		},
		Relations: []ado.WorkItemRelation{
			{Rel: "System.LinkTypes.Hierarchy-Reverse", URL: "https://dev.azure.com/org/proj/_apis/wit/workItems/100"},
		},
	}
	_ = database.SaveWorkItem("org", "proj", feature)

	// 3. Story with specific iteration, parent is 101
	story := &ado.WorkItem{
		ID: 102,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "User Story",
			"System.Title":         "Child Story",
			"System.IterationPath": "Project\\Sprint 2026.Q3.Sprint 2",
		},
		Relations: []ado.WorkItemRelation{
			{Rel: "System.LinkTypes.Hierarchy-Reverse", URL: "https://dev.azure.com/org/proj/_apis/wit/workItems/101"},
		},
	}
	_ = database.SaveWorkItem("org", "proj", story)

	// Direct parent with iteration
	iter102 := database.GetNearestParentIteration(102)
	if iter102 != "Project\\Sprint 2026.Q3.Sprint 2" {
		t.Fatalf("expected nearest iteration for 102 to be %q, got %q", "Project\\Sprint 2026.Q3.Sprint 2", iter102)
	}

	// 101 has empty iteration, does not go past its nearest parent
	iter101 := database.GetNearestParentIteration(101)
	if iter101 != "" {
		t.Fatalf("expected empty iteration for 101, got %q", iter101)
	}

	// Root/non-existent parent
	iter0 := database.GetNearestParentIteration(0)
	if iter0 != "" {
		t.Fatalf("expected empty iteration for parent 0, got %q", iter0)
	}

	iter999 := database.GetNearestParentIteration(999)
	if iter999 != "" {
		t.Fatalf("expected empty iteration for nonexistent parent 999, got %q", iter999)
	}

	// Test Area Path inheritance
	epicWithArea := &ado.WorkItem{
		ID: 200,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Epic",
			"System.Title":         "Area Epic",
			"System.AreaPath":      "Project\\Backend\\Core",
			"System.IterationPath": "Project\\Sprint 1",
		},
	}
	_ = database.SaveWorkItem("org", "proj", epicWithArea)

	featureChild := &ado.WorkItem{
		ID: 201,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Feature",
			"System.Title":         "Area Feature",
			"System.AreaPath":      "",
			"System.IterationPath": "",
		},
		Relations: []ado.WorkItemRelation{
			{Rel: "System.LinkTypes.Hierarchy-Reverse", URL: "https://dev.azure.com/org/proj/_apis/wit/workItems/200"},
		},
	}
	_ = database.SaveWorkItem("org", "proj", featureChild)

	area200 := database.GetNearestParentArea(200)
	if area200 != "Project\\Backend\\Core" {
		t.Fatalf("expected nearest area for 200 to be %q, got %q", "Project\\Backend\\Core", area200)
	}

	// 201 has empty area, does not go past its nearest parent
	area201 := database.GetNearestParentArea(201)
	if area201 != "" {
		t.Fatalf("expected empty area for 201, got %q", area201)
	}

	area, iter := database.GetNearestParentAreaAndIteration(200)
	if area != "Project\\Backend\\Core" || iter != "Project\\Sprint 1" {
		t.Fatalf("expected area %q and iter %q, got area %q and iter %q", "Project\\Backend\\Core", "Project\\Sprint 1", area, iter)
	}
}

func TestTrackedQueriesCRUD(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "adotest_query_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	database, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// 1. Save Tracked Query
	queryID := "7509d605-466a-4d8c-847f-c14e1b46a647"
	err = database.SaveTrackedQuery(queryID, "Sprint 2026 Backlog", "Shared Queries/Sprint", "my-org", "my-project", "https://dev.azure.com/...", "tree")
	if err != nil {
		t.Fatalf("failed to save tracked query: %v", err)
	}

	// 2. Save Tracked Roots linked to Query
	err = database.SaveTrackedRootWithQuery(100, "my-org", "my-project", "", queryID)
	if err != nil {
		t.Fatalf("failed to save tracked root: %v", err)
	}
	err = database.SaveTrackedRootWithQuery(200, "my-org", "my-project", "", queryID)
	if err != nil {
		t.Fatalf("failed to save tracked root: %v", err)
	}

	// 3. Get Tracked Queries and verify count
	queries, err := database.GetTrackedQueries()
	if err != nil {
		t.Fatalf("failed to get tracked queries: %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("expected 1 tracked query, got %d", len(queries))
	}
	if queries[0].ID != queryID || queries[0].Name != "Sprint 2026 Backlog" || queries[0].ItemCount != 2 {
		t.Errorf("unexpected query details: %+v", queries[0])
	}

	// 4. Delete Tracked Query (should also clean up linked roots)
	err = database.DeleteTrackedQuery(queryID)
	if err != nil {
		t.Fatalf("failed to delete tracked query: %v", err)
	}

	queriesAfter, _ := database.GetTrackedQueries()
	if len(queriesAfter) != 0 {
		t.Errorf("expected 0 tracked queries after delete, got %d", len(queriesAfter))
	}

	rootsAfter, _ := database.GetTrackedRoots()
	if len(rootsAfter) != 0 {
		t.Errorf("expected 0 tracked roots after delete, got %d", len(rootsAfter))
	}
}

func TestDistinctIterationsAreasAndTags(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "distinct_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	database, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Insert test work items with iterations, areas, and tags
	item1 := &ado.WorkItem{
		ID: 1,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Task",
			"System.Title":         "Task 1",
			"System.State":         "Active",
			"System.IterationPath": "DigitalApp\\Sprint 2",
			"System.AreaPath":      "DigitalApp\\Backend",
			"System.Tags":          "Golang; Backend; Critical",
		},
	}
	item2 := &ado.WorkItem{
		ID: 2,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Bug",
			"System.Title":         "Bug 2",
			"System.State":         "New",
			"System.IterationPath": "DigitalApp\\Sprint 1",
			"System.AreaPath":      "DigitalApp\\Frontend",
			"System.Tags":          "Frontend, UI, Critical",
		},
	}
	_ = database.SaveWorkItem("myorg", "myproject", item1)
	_ = database.SaveWorkItem("myorg", "myproject", item2)

	// 1. Distinct Iterations
	iters, err := database.GetDistinctIterationPaths()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(iters) != 2 || iters[0] != "DigitalApp\\Sprint 1" || iters[1] != "DigitalApp\\Sprint 2" {
		t.Errorf("unexpected iterations: %v", iters)
	}

	// 2. Distinct Areas
	areas, err := database.GetDistinctAreaPaths()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(areas) != 2 || areas[0] != "DigitalApp\\Backend" || areas[1] != "DigitalApp\\Frontend" {
		t.Errorf("unexpected areas: %v", areas)
	}

	// 3. Distinct Tags
	tags, err := database.GetDistinctTags()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedTags := []string{"Backend", "Critical", "Frontend", "Golang", "UI"}
	if len(tags) != len(expectedTags) {
		t.Fatalf("expected %d tags, got %v", len(expectedTags), tags)
	}
	for i, exp := range expectedTags {
		if tags[i] != exp {
			t.Errorf("tag[%d] expected %s, got %s", i, exp, tags[i])
		}
	}
}

func TestWorkItemTypeStates(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_wit_states.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	// 1. Initially empty
	states, err := database.GetWorkItemTypeStates("myorg", "myproj", "Bug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 0 {
		t.Fatalf("expected 0 states, got %d", len(states))
	}

	// 2. Save Bug states
	bugStates := []ado.WorkItemState{
		{Name: "New", Category: "Proposed"},
		{Name: "Approved", Category: "Proposed"},
		{Name: "Committed", Category: "InProgress"},
		{Name: "Done", Category: "Completed"},
		{Name: "Removed", Category: "Removed"},
	}
	if err := database.SaveWorkItemTypeStates("myorg", "myproj", "Bug", bugStates); err != nil {
		t.Fatalf("failed to save Bug states: %v", err)
	}

	saved, err := database.GetWorkItemTypeStates("myorg", "myproj", "Bug")
	if err != nil {
		t.Fatalf("failed to get Bug states: %v", err)
	}
	if len(saved) != 5 || saved[0] != "New" || saved[2] != "Committed" || saved[3] != "Done" {
		t.Fatalf("unexpected Bug states: %v", saved)
	}

	// 3. Test DistinctStatesForType from work items
	wi := &ado.WorkItem{
		ID: 100,
		Fields: map[string]interface{}{
			"System.Title":        "Task 1",
			"System.WorkItemType": "Task",
			"System.State":        "In Progress",
		},
	}
	_ = database.SaveWorkItem("myorg", "myproj", wi)

	taskStates, err := database.GetDistinctStatesForType("Task")
	if err != nil {
		t.Fatalf("failed to get distinct states for Task: %v", err)
	}
	if len(taskStates) != 1 || taskStates[0] != "In Progress" {
		t.Fatalf("expected ['In Progress'], got %v", taskStates)
	}
}

func TestDynamicStateCategoryAndFields(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	org := "test-org"
	project := "test-proj"
	itemType := "ProductBacklogItem"

	// 1. Save states with various standard and custom categories
	states := []ado.WorkItemState{
		{Name: "New", Category: "Proposed", Color: "b2b2b2"},
		{Name: "Approved", Category: "Proposed", Color: "b2b2b2"},
		{Name: "Committed", Category: "InProgress", Color: "007acc"},
		{Name: "In Review", Category: "Resolved", Color: "ff9d00"},
		{Name: "Shipped", Category: "Completed", Color: "339933"},
		{Name: "Abandoned", Category: "Removed", Color: "e60017"},
	}

	err = database.SaveWorkItemTypeStates(org, project, itemType, states)
	if err != nil {
		t.Fatalf("failed to save states: %v", err)
	}

	// 2. Test GetWorkItemStatesDetailed
	detailed, err := database.GetWorkItemStatesDetailed(org, project, itemType)
	if err != nil {
		t.Fatalf("failed to get detailed states: %v", err)
	}
	if len(detailed) != 6 {
		t.Fatalf("expected 6 states, got %d", len(detailed))
	}
	if detailed[4].Name != "Shipped" || detailed[4].Category != "Completed" {
		t.Errorf("expected Shipped to have Completed category, got %v", detailed[4])
	}

	// 3. Test GetStateCategory
	cat, ok := database.GetStateCategory(org, project, itemType, "Shipped")
	if !ok || cat != "Completed" {
		t.Errorf("expected category 'Completed' for 'Shipped', got %q (ok=%v)", cat, ok)
	}

	cat2, ok := database.GetStateCategory(org, project, itemType, "In Review")
	if !ok || cat2 != "Resolved" {
		t.Errorf("expected category 'Resolved' for 'In Review', got %q (ok=%v)", cat2, ok)
	}

	// 4. Test IsStateCompleted
	if !database.IsStateCompleted(org, project, itemType, "Shipped") {
		t.Errorf("expected 'Shipped' to be recognized as completed")
	}
	if !database.IsStateCompleted(org, project, itemType, "In Review") {
		t.Errorf("expected 'In Review' (Resolved category) to be recognized as completed")
	}
	if !database.IsStateCompleted(org, project, itemType, "Abandoned") {
		t.Errorf("expected 'Abandoned' (Removed category) to be recognized as completed")
	}
	if database.IsStateCompleted(org, project, itemType, "Committed") {
		t.Errorf("expected 'Committed' (InProgress) NOT to be recognized as completed")
	}
	if database.IsStateCompleted(org, project, itemType, "Approved") {
		t.Errorf("expected 'Approved' (Proposed) NOT to be recognized as completed")
	}

	// Fallback check on non-cached item type
	if !database.IsStateCompleted(org, project, "UncachedType", "Done") {
		t.Errorf("expected 'Done' to be completed via fallback for uncached type")
	}

	// 5. Save and Test Work Item Type Fields
	fields := []ado.WorkItemTypeField{
		{ReferenceName: "System.Title", Name: "Title"},
		{ReferenceName: "Custom.OriginalEstimate", Name: "Original Estimate"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.Effort", Name: "Effort"},
	}
	err = database.SaveWorkItemTypeFields(org, project, itemType, fields)
	if err != nil {
		t.Fatalf("failed to save fields: %v", err)
	}

	if !database.TypeHasField(org, project, itemType, "Custom.OriginalEstimate") {
		t.Errorf("expected TypeHasField to return true for Custom.OriginalEstimate")
	}
	if database.TypeHasField(org, project, itemType, "NonExistent.Field") {
		t.Errorf("expected TypeHasField to return false for NonExistent.Field")
	}
}

func TestGetDescendantAndAncestorWorkItems(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "adotest_hierarchy_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	database, err := Open(tmpPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Seed hierarchy:
	// Epic (100) -> Feature (200) -> Story (300) -> Task 1 (401), Task 2 (402)
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 100,
		Fields: map[string]any{
			"System.WorkItemType": "Epic",
			"System.Title":        "Enterprise Cloud Migration",
			"System.State":        "Active",
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 200,
		Fields: map[string]any{
			"System.WorkItemType": "Feature",
			"System.Title":        "Kubernetes Infrastructure",
			"System.State":        "Active",
			"System.Parent":       100,
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 300,
		Fields: map[string]any{
			"System.WorkItemType": "User Story",
			"System.Title":        "Setup Cluster Ingress",
			"System.State":        "Active",
			"System.Parent":       200,
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 401,
		Fields: map[string]any{
			"System.WorkItemType":                       "Task",
			"System.Title":                              "Configure Traefik",
			"System.State":                              "Active",
			"System.Parent":                             300,
			"Microsoft.VSTS.Scheduling.OriginalEstimate": 8.0,
			"Microsoft.VSTS.Scheduling.RemainingWork":    4.0,
			"Microsoft.VSTS.Scheduling.CompletedWork":    4.0,
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 402,
		Fields: map[string]any{
			"System.WorkItemType":                       "Task",
			"System.Title":                              "Setup TLS Certs",
			"System.State":                              "Done",
			"System.Parent":                             300,
			"Microsoft.VSTS.Scheduling.OriginalEstimate": 4.0,
			"Microsoft.VSTS.Scheduling.RemainingWork":    0.0,
			"Microsoft.VSTS.Scheduling.CompletedWork":    4.0,
		},
	})

	// 1. Descendants of Feature (200) should be Story 300, Task 401, Task 402
	desc200, err := database.GetDescendantWorkItems(200)
	if err != nil {
		t.Fatalf("failed to get descendants: %v", err)
	}
	if len(desc200) != 3 {
		t.Fatalf("expected 3 descendants under Feature 200, got %d", len(desc200))
	}

	// 2. Ancestors of Task 401 should be Epic 100 -> Feature 200 -> Story 300
	anc401, err := database.GetAncestorWorkItems(401)
	if err != nil {
		t.Fatalf("failed to get ancestors: %v", err)
	}
	if len(anc401) != 3 {
		t.Fatalf("expected 3 ancestors for Task 401, got %d", len(anc401))
	}
	if anc401[0].ID != 100 || anc401[1].ID != 200 || anc401[2].ID != 300 {
		t.Fatalf("expected ancestors [100, 200, 300], got [%d, %d, %d]", anc401[0].ID, anc401[1].ID, anc401[2].ID)
	}

	// 3. Subtree & HierarchyNode verification
	treeNode, flatDesc, err := database.GetWorkItemSubtree(200)
	if err != nil {
		t.Fatalf("failed to get subtree: %v", err)
	}
	if treeNode.ID != 200 {
		t.Errorf("expected root ID 200, got %d", treeNode.ID)
	}
	if len(treeNode.Children) != 1 || treeNode.Children[0].ID != 300 {
		t.Fatalf("expected Story 300 as direct child of Feature 200")
	}
	if len(treeNode.Children[0].Children) != 2 {
		t.Fatalf("expected 2 tasks under Story 300, got %d", len(treeNode.Children[0].Children))
	}
	if treeNode.RollupOriginalEstimate != 12.0 {
		t.Errorf("expected rollup original estimate 12.0, got %f", treeNode.RollupOriginalEstimate)
	}
	if treeNode.RollupRemainingWork != 4.0 {
		t.Errorf("expected rollup remaining work 4.0, got %f", treeNode.RollupRemainingWork)
	}
	if treeNode.RollupCompletedWork != 8.0 {
		t.Errorf("expected rollup completed work 8.0, got %f", treeNode.RollupCompletedWork)
	}
	if len(flatDesc) != 3 {
		t.Errorf("expected 3 flat descendants, got %d", len(flatDesc))
	}
}




