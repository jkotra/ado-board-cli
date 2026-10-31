package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
)

func TestContextPayloadAndSystemPrompt(t *testing.T) {
	ctxPayload := &ContextPayload{
		ActiveScreen: "tree_view",
		SelectedItem: &ItemSummary{
			ID:            666236,
			Type:          "Feature",
			Title:         "ADO API test",
			State:         "New",
			RemainingWork: 4,
		},
		ChildItems: []*ItemSummary{
			{
				ID:            666237,
				Type:          "Task",
				Title:         "Subtask 1",
				RemainingWork: 2,
			},
		},
		CachedIdentities: []IdentitySummary{
			{
				DisplayName: "Anderson, Alice",
				Email:       "alice.anderson@example.com",
			},
		},
	}

	prompt := BuildSystemPrompt(ctxPayload)
	if !strings.Contains(prompt, "666236") {
		t.Errorf("expected system prompt to contain selected item ID 666236")
	}
	if !strings.Contains(prompt, "Anderson, Alice") {
		t.Errorf("expected system prompt to contain identity Anderson, Alice")
	}
}

func TestExecutorAssigneeResolution(t *testing.T) {
	ctxPayload := &ContextPayload{
		CachedIdentities: []IdentitySummary{
			{DisplayName: "Anderson, Alice", Email: "alice.anderson@example.com"},
			{DisplayName: "Doe, John", Email: "john.doe@example.com"},
		},
	}
	executor := NewExecutor(nil, nil, ctxPayload, nil)

	res := executor.resolveAssignee("alice")
	if res != "alice.anderson@example.com" {
		t.Errorf("expected alice.anderson@example.com, got %s", res)
	}

	res2 := executor.resolveAssignee("Doe, John")
	if res2 != "john.doe@example.com" {
		t.Errorf("expected john.doe@example.com, got %s", res2)
	}
}

func TestDeleteWorkItemConfirmation(t *testing.T) {
	ctxPayload := &ContextPayload{
		SelectedItem: &ItemSummary{
			ID:    12345,
			Title: "Test Task",
		},
	}

	// 1. Rejected confirmation
	promptSeen := false
	rejectConfirm := func(prompt ConfirmPrompt) bool {
		promptSeen = true
		if prompt.WorkItemID != 12345 {
			t.Errorf("expected prompt item ID 12345, got %d", prompt.WorkItemID)
		}
		return false // User clicks [No]
	}

	executor := NewExecutor(nil, nil, ctxPayload, rejectConfirm)
	res, err := executor.Execute("delete_work_item", map[string]any{"work_item_id": 12345})
	if err != nil {
		t.Fatalf("unexpected error on rejected delete: %v", err)
	}
	if !promptSeen {
		t.Errorf("expected confirmation prompt to be called")
	}
	if res["status"] != "cancelled" {
		t.Errorf("expected status 'cancelled', got %v", res["status"])
	}
}

func TestAgentInitialization(t *testing.T) {
	cfg := &config.Config{
		GeminiApiKey: "test-fake-gemini-key",
		GeminiModel:  "gemini-3.8-flash",
	}
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer database.Close()

	agent, err := NewAgent(context.Background(), cfg, database, ado.NewClient("org", "proj", "pat"))
	if err != nil {
		t.Fatalf("failed to initialize agent: %v", err)
	}
	if agent.ModelName != "gemini-3.8-flash" {
		t.Errorf("expected model gemini-3.8-flash, got %s", agent.ModelName)
	}

	// Test missing API key returns error
	emptyCfg := &config.Config{}
	_, err = NewAgent(context.Background(), emptyCfg, database, ado.NewClient("org", "proj", "pat"))
	if err == nil {
		t.Errorf("expected error when Gemini API key is missing, got nil")
	}
}

func TestExecutorEstimateCalculation(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer database.Close()

	ctxPayload := &ContextPayload{
		SelectedItem: &ItemSummary{
			ID:               500,
			Title:            "Parent Feature",
			OriginalEstimate: 10,
			RemainingWork:    10,
		},
	}

	executor := NewExecutor(ado.NewClient("testorg", "testproj", "fakepat"), database, ctxPayload, nil)

	// Verify BuildSystemPrompt includes estimate rule
	prompt := BuildSystemPrompt(ctxPayload)
	if !strings.Contains(prompt, "ESTIMATE RULE") {
		t.Errorf("expected system prompt to include estimate rule, got:\n%s", prompt)
	}
	_ = executor
}

func TestExecutorActivityAndPRTools(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer database.Close()

	// Seed work item
	wi := &ado.WorkItem{
		ID: 100,
		Fields: map[string]interface{}{
			"System.Title":        "Test Work Item 100",
			"System.WorkItemType": "Feature",
			"System.State":        "Active",
		},
	}
	_ = database.SaveWorkItem("testorg", "testproj", wi)

	// Seed comments
	now := time.Now()
	_ = database.SaveComments(100, []*ado.CommentItem{
		{
			ID:          1,
			Text:        "First comment on task",
			CreatedDate: now.Add(-2 * time.Hour),
			CreatedBy: &ado.IdentityRef{
				DisplayName: "Dev Tester",
				UniqueName:  "dev@test.com",
			},
		},
	})

	// Seed PR
	_ = database.SavePullRequest(&ado.GitPullRequest{
		PullRequestID: 555,
		Title:         "Feature branch PR",
		Status:        "active",
		CreatedBy: &ado.IdentityRef{
			DisplayName: "Dev Tester",
			UniqueName:  "dev@test.com",
		},
	}, "testorg", "testproj")
	_ = database.LinkWorkItemPullRequest(100, 555)

	ctxPayload := &ContextPayload{
		SelectedItem: &ItemSummary{
			ID:    100,
			Title: "Test Work Item 100",
			Type:  "Feature",
		},
	}

	executor := NewExecutor(nil, database, ctxPayload, nil)

	// 1. Test get_comments
	resComments, err := executor.Execute("get_comments", map[string]any{"work_item_id": 100})
	if err != nil {
		t.Fatalf("get_comments failed: %v", err)
	}
	if resComments["count"] != 1 {
		t.Errorf("expected 1 comment, got %v", resComments["count"])
	}

	// 2. Test get_pull_requests
	resPRs, err := executor.Execute("get_pull_requests", map[string]any{"work_item_id": 100})
	if err != nil {
		t.Fatalf("get_pull_requests failed: %v", err)
	}
	if resPRs["total_count"] != 1 {
		t.Errorf("expected 1 PR, got %v", resPRs["total_count"])
	}

	// 3. Test get_activity
	resActivity, err := executor.Execute("get_activity", map[string]any{
		"work_item_id": 100,
		"time_range":   "today",
	})
	if err != nil {
		t.Fatalf("get_activity failed: %v", err)
	}
	if resActivity["status"] != "success" {
		t.Errorf("expected status 'success', got %v", resActivity["status"])
	}
	events, ok := resActivity["events"].([]*ado.ActivityEvent)
	if !ok || len(events) == 0 {
		t.Fatalf("expected at least 1 activity event, got %v", resActivity["events"])
	}
}

func TestParseFlexibleDateField(t *testing.T) {
	// Nil input
	res, ok := parseFlexibleDateField(nil)
	if ok || res != "" {
		t.Errorf("expected false, empty for nil; got %v, %q", ok, res)
	}

	// Empty string (clear)
	res, ok = parseFlexibleDateField("")
	if !ok || res != "" {
		t.Errorf("expected true, empty for empty string; got %v, %q", ok, res)
	}

	// Specific date
	res, ok = parseFlexibleDateField("8/18/2026 12:00 AM")
	if !ok || res == "" {
		t.Errorf("expected non-empty date, got %v, %q", ok, res)
	}
	expectedTime, _ := ado.ParseDateOrDateTime("8/18/2026 12:00 AM", time.Now())
	if res != ado.FormatAdoDateTime(expectedTime) {
		t.Errorf("expected %q, got %q", ado.FormatAdoDateTime(expectedTime), res)
	}

	// Relative date "today"
	res, ok = parseFlexibleDateField("today")
	if !ok || res == "" {
		t.Errorf("expected valid date for 'today', got %v, %q", ok, res)
	}
}

type geminiRoundTripFunc func(req *http.Request) *http.Response

func (f geminiRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestExecutorCreateWorkItemNearestParentIteration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_gemini_create_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	database, err := db.Open(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	// Seed parent Feature with Iteration and Area
	parent := &ado.WorkItem{
		ID: 500,
		Fields: map[string]interface{}{
			"System.WorkItemType":  "Feature",
			"System.Title":         "Parent Feature",
			"System.AreaPath":      "Org\\Engineering\\Frontend",
			"System.IterationPath": "Org\\Sprint 42",
		},
	}
	_ = database.SaveWorkItem("myorg", "myproj", parent)

	var capturedPatches []ado.JsonPatchOperation
	adoClient := ado.NewClient("myorg", "myproj", "token")
	adoClient.R.SetTransport(geminiRoundTripFunc(func(req *http.Request) *http.Response {
		if req.Method == http.MethodPost {
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &capturedPatches)
			wi := ado.WorkItem{
				ID: 600,
				Fields: map[string]interface{}{
					"System.Title":         "New Task",
					"System.WorkItemType":  "Task",
					"System.AreaPath":      "Org\\Engineering\\Frontend",
					"System.IterationPath": "Org\\Sprint 42",
				},
			}
			data, _ := json.Marshal(wi)
			hdr := make(http.Header)
			hdr.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("")),
		}
	}))

	ctxPayload := &ContextPayload{}
	executor := NewExecutor(adoClient, database, ctxPayload, nil)

	res, err := executor.Execute("create_work_item", map[string]any{
		"parent_id":      500,
		"work_item_type": "Task",
		"title":          "New Task",
	})
	if err != nil {
		t.Fatalf("execute create_work_item failed: %v", err)
	}
	if res["status"] != "success" {
		t.Fatalf("expected status 'success', got %v", res["status"])
	}

	hasIter := false
	hasArea := false
	for _, p := range capturedPatches {
		if p.Path == "/fields/System.IterationPath" && p.Value == "Org\\Sprint 42" {
			hasIter = true
		}
		if p.Path == "/fields/System.AreaPath" && p.Value == "Org\\Engineering\\Frontend" {
			hasArea = true
		}
	}
	if !hasIter {
		t.Fatalf("expected System.IterationPath 'Org\\Sprint 42' in patches, got %+v", capturedPatches)
	}
	if !hasArea {
		t.Fatalf("expected System.AreaPath 'Org\\Engineering\\Frontend' in patches, got %+v", capturedPatches)
	}
}

func TestValidateAPIKey(t *testing.T) {
	// 1. Empty key
	valid, err := ValidateAPIKey(context.Background(), "", "gemini-3.8-flash")
	if valid || err == nil {
		t.Fatalf("expected error for empty key, got valid=%v, err=%v", valid, err)
	}

	// 2. Custom mock validation
	defer func() { ValidateKeyFunc = nil }()

	ValidateKeyFunc = func(ctx context.Context, apiKey, modelName string) (bool, error) {
		if apiKey == "valid-gemini-key" {
			return true, nil
		}
		return false, fmt.Errorf("invalid API key")
	}

	valid, err = ValidateAPIKey(context.Background(), "valid-gemini-key", "gemini-3.8-flash")
	if !valid || err != nil {
		t.Fatalf("expected valid key to succeed, got valid=%v, err=%v", valid, err)
	}

	valid, err = ValidateAPIKey(context.Background(), "bad-gemini-key", "gemini-3.8-flash")
	if valid || err == nil || !strings.Contains(err.Error(), "invalid API key") {
		t.Fatalf("expected invalid API key error, got valid=%v, err=%v", valid, err)
	}
}

func TestExecutorGetAndUpdateSettings(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	ctxPayload := &ContextPayload{
		CurrentSettings: &SettingsSummary{
			AutoSyncIntervalSeconds:      120,
			AutoCalcRemaining:            true,
			EffortSameAsEstimate:         false,
			AutoSetFinishDate:            true,
			IncompleteCriteriaEffort:     true,
			IncompleteCriteriaEstimate:   true,
			IncompleteCriteriaStartDate:  true,
			IncompleteCriteriaTargetDate: true,
			IncompleteCriteriaFinishDate: false,
			IncompleteTypes:              []string{"Task", "Bug"},
			IncompleteStatuses:           []string{"[All]"},
			IncompleteDateStatuses:       []string{"[All]"},
			GeminiModel:                  "gemini-3.8-flash",
		},
	}

	executor := NewExecutor(nil, database, ctxPayload, nil)

	// 1. Test get_settings
	resGet, err := executor.Execute("get_settings", map[string]any{})
	if err != nil {
		t.Fatalf("execute get_settings failed: %v", err)
	}
	if resGet["auto_sync_interval_seconds"] != 120 {
		t.Errorf("expected auto_sync_interval_seconds 120, got %v", resGet["auto_sync_interval_seconds"])
	}
	if resGet["auto_set_finish_date"] != true {
		t.Errorf("expected auto_set_finish_date true, got %v", resGet["auto_set_finish_date"])
	}
	if resGet["incomplete_statuses"].([]string)[0] != "[All]" {
		t.Errorf("expected incomplete_statuses ['[All]'], got %v", resGet["incomplete_statuses"])
	}

	// 2. Test update_settings
	resUpdate, err := executor.Execute("update_settings", map[string]any{
		"auto_sync_interval_seconds":      60,
		"auto_set_finish_date":            false,
		"incomplete_statuses":             []any{"In Progress", "Active"},
		"incomplete_criteria_finish_date": true,
		"incomplete_criteria_tag_regex":   true,
		"tag_regex":                       `^(BE|FE)-`,
		"gemini_model":                    "gemini-1.5-pro",
	})
	if err != nil {
		t.Fatalf("execute update_settings failed: %v", err)
	}
	if resUpdate["success"] != true {
		t.Fatalf("expected success true, got %v", resUpdate["success"])
	}
	if !executor.SettingsUpdated {
		t.Errorf("expected executor.SettingsUpdated to be true")
	}

	// 3. Verify settings saved in database
	val, err := database.GetSetting("auto_sync_interval_seconds")
	if err != nil || val != "60" {
		t.Errorf("expected db auto_sync_interval_seconds '60', got %q (err: %v)", val, err)
	}
	val, err = database.GetSetting("auto_set_finish_date")
	if err != nil || val != "false" {
		t.Errorf("expected db auto_set_finish_date 'false', got %q (err: %v)", val, err)
	}
	val, err = database.GetSetting("incomplete_statuses")
	if err != nil || val != "In Progress,Active" {
		t.Errorf("expected db incomplete_statuses 'In Progress,Active', got %q (err: %v)", val, err)
	}
	val, err = database.GetSetting("incomplete_criteria_finish_date")
	if err != nil || val != "true" {
		t.Errorf("expected db incomplete_criteria_finish_date 'true', got %q (err: %v)", val, err)
	}
	val, err = database.GetSetting("incomplete_criteria_tag_regex")
	if err != nil || val != "true" {
		t.Errorf("expected db incomplete_criteria_tag_regex 'true', got %q (err: %v)", val, err)
	}
	val, err = database.GetSetting("tag_regex")
	if err != nil || val != `^(BE|FE)-` {
		t.Errorf("expected db tag_regex '^(BE|FE)-', got %q (err: %v)", val, err)
	}
	val, err = database.GetSetting("gemini_model")
	if err != nil || val != "gemini-1.5-pro" {
		t.Errorf("expected db gemini_model 'gemini-1.5-pro', got %q (err: %v)", val, err)
	}

	// Test update_settings with tag_regexes array
	resUpdateArray, err := executor.Execute("update_settings", map[string]any{
		"tag_regexes": []any{"^team-(alpha|beta)", "^release-[0-9]+", "^infra-.*"},
	})
	if err != nil || resUpdateArray["success"] != true {
		t.Fatalf("execute update_settings with tag_regexes failed: %v", err)
	}
	val, err = database.GetSetting("tag_regex")
	if err != nil || val != "^team-(alpha|beta), ^release-[0-9]+, ^infra-.*" {
		t.Errorf("expected db tag_regex array joined, got %q (err: %v)", val, err)
	}
	resGet2, err := executor.Execute("get_settings", nil)
	if err != nil {
		t.Fatalf("get_settings failed: %v", err)
	}
	tagRegexesList, ok := resGet2["tag_regexes"].([]string)
	if !ok || len(tagRegexesList) != 3 {
		t.Errorf("expected tag_regexes list of length 3, got %v", resGet2["tag_regexes"])
	}

	// 4. Test GetAdoTools returns get_settings and update_settings
	tools := GetAdoTools()
	hasGet := false
	hasUpdate := false
	for _, tool := range tools {
		for _, decl := range tool.FunctionDeclarations {
			if decl.Name == "get_settings" {
				hasGet = true
			}
			if decl.Name == "update_settings" {
				hasUpdate = true
			}
		}
	}
	if !hasGet || !hasUpdate {
		t.Errorf("expected GetAdoTools to include get_settings and update_settings")
	}
}

func TestExecutorTrackingAndBoardSummary(t *testing.T) {
	tempDB := t.TempDir() + "/test_tracking.db"
	database, err := db.Open(tempDB)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	adoClient := ado.NewClient("myorg", "myproj", "fakepat")
	adoClient.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")
		if strings.Contains(req.URL.Path, "workitems") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"count": 1,
					"value": [
						{
							"id": 100,
							"fields": {
								"System.Title": "Root Feature",
								"System.WorkItemType": "Feature",
								"System.State": "Active",
								"System.TeamProject": "myproj"
							}
						}
					]
				}`)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	executor := NewExecutor(adoClient, database, &ContextPayload{}, nil)

	// 2. Test track_work_item
	resTrack, err := executor.Execute("track_work_item", map[string]any{
		"work_item_id": 100,
		"custom_title": "Custom Epic",
	})
	if err != nil {
		t.Fatalf("track_work_item failed: %v", err)
	}
	if resTrack["success"] != true {
		t.Fatalf("expected success true, got %v", resTrack)
	}

	// Verify in DB
	roots, err := database.GetTrackedRoots()
	if err != nil || len(roots) != 1 || roots[0].ID != 100 {
		t.Fatalf("expected 1 tracked root #100, got %v", roots)
	}
	if roots[0].CustomTitle != "Custom Epic" {
		t.Errorf("expected custom title 'Custom Epic', got %q", roots[0].CustomTitle)
	}

	// 3. Test get_board_summary
	resSummary, err := executor.Execute("get_board_summary", map[string]any{})
	if err != nil {
		t.Fatalf("get_board_summary failed: %v", err)
	}
	if resSummary["total_tracked_roots"].(int) != 1 {
		t.Errorf("expected total_tracked_roots = 1, got %v", resSummary["total_tracked_roots"])
	}

	// 4. Test untrack_work_item
	resUntrack, err := executor.Execute("untrack_work_item", map[string]any{
		"work_item_id": 100,
	})
	if err != nil {
		t.Fatalf("untrack_work_item failed: %v", err)
	}
	if resUntrack["success"] != true {
		t.Fatalf("expected success true on untrack, got %v", resUntrack)
	}

	rootsAfter, err := database.GetTrackedRoots()
	if err != nil || len(rootsAfter) != 0 {
		t.Fatalf("expected 0 tracked roots after untrack, got %v", rootsAfter)
	}
}

func TestExecutorGetAppStateAndSelectedItem(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tempDir)
	dbPath := filepath.Join(tempDir, "test_executor_state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	ctxPayload := &ContextPayload{
		ActiveScreen: "tree_view",
		SelectedItem: &ItemSummary{
			ID:            777,
			Type:          "Bug",
			Title:         "Crash on startup",
			State:         "Active",
			RemainingWork: 2.0,
		},
		ParentItem: &ItemSummary{
			ID:    700,
			Type:  "Feature",
			Title: "Core Engine",
		},
		ChildItems: []*ItemSummary{
			{
				ID:    778,
				Type:  "Task",
				Title: "Reproduce crash",
			},
		},
	}

	executor := NewExecutor(nil, database, ctxPayload, nil)

	// 1. Test app_get_state
	stateRes, err := executor.Execute("app_get_state", map[string]any{})
	if err != nil {
		t.Fatalf("app_get_state failed: %v", err)
	}
	if stateRes["active_screen"] != "tree_view" {
		t.Errorf("expected active_screen tree_view, got %v", stateRes["active_screen"])
	}

	// 2. Test get_selected_item
	selRes, err := executor.Execute("get_selected_item", map[string]any{})
	if err != nil {
		t.Fatalf("get_selected_item failed: %v", err)
	}
	selItem, ok := selRes["selected_item"].(*ItemSummary)
	if !ok || selItem.ID != 777 || selItem.Title != "Crash on startup" {
		t.Fatalf("unexpected selected_item: %v", selRes["selected_item"])
	}
}

func TestExecutorGetUsers(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_executor_users.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	_ = database.SaveIdentities([]*ado.IdentityRef{
		{
			ID:          "guid-1",
			DisplayName: "Doe, John",
			UniqueName:  "john.doe@example.com",
		},
		{
			ID:          "guid-2",
			DisplayName: "Alice Taylor",
			UniqueName:  "henry.taylor@example.com",
		},
	})

	executor := NewExecutor(nil, database, nil, nil)

	// 1. Get all users
	resAll, err := executor.Execute("get_users", map[string]any{})
	if err != nil {
		t.Fatalf("get_users failed: %v", err)
	}
	if resAll["total_users"].(int) != 2 {
		t.Fatalf("expected 2 users, got %v", resAll["total_users"])
	}

	// 2. Search users by query
	resQuery, err := executor.Execute("get_users", map[string]any{"query": "alice"})
	if err != nil {
		t.Fatalf("get_users with query failed: %v", err)
	}
	if resQuery["total_users"].(int) != 1 {
		t.Fatalf("expected 1 user matching alice, got %v", resQuery["total_users"])
	}

	// 3. Test resolveAssignee integration
	resolved := executor.resolveAssignee("alice")
	if resolved != "henry.taylor@example.com" {
		t.Fatalf("expected henry.taylor@example.com, got %v", resolved)
	}
}

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestExecutorUploadPRAttachment(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "architecture.png")
	if err := os.WriteFile(filePath, []byte("fake image data"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	client := ado.NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "attachments/architecture.png") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": 99,
					"url": "https://dev.azure.com/testorg/testproj/_apis/git/repositories/myrepo/pullRequests/777/attachments/architecture.png"
				}`)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	executor := NewExecutor(client, nil, nil, nil)
	res, err := executor.Execute("upload_pull_request_attachment", map[string]any{
		"pull_request_id": 777,
		"file_path":       filePath,
		"repository_id":   "myrepo",
	})
	if err != nil {
		t.Fatalf("upload_pull_request_attachment failed: %v", err)
	}
	if res["status"] != "success" {
		t.Errorf("expected success, got %v", res["status"])
	}
	if res["attachment_id"] != 99 {
		t.Errorf("expected attachment_id 99, got %v", res["attachment_id"])
	}
	if !strings.Contains(res["markdown"].(string), "![architecture.png]") {
		t.Errorf("expected markdown image embed, got %v", res["markdown"])
	}
}

func TestExecutorUpdatePullRequest(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_executor_pr.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	client := ado.NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "pullrequests/777") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 777,
					"title": "Initial Title",
					"description": "Initial description",
					"repository": {"name": "myrepo"}
				}`)),
			}
		}

		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "pullrequests/777") {
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			respData, _ := json.Marshal(map[string]interface{}{
				"pullRequestId": 777,
				"title":         "Initial Title",
				"description":   body["description"],
				"repository":    map[string]string{"name": "myrepo"},
			})
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(respData)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	executor := NewExecutor(client, database, nil, nil)
	res, err := executor.Execute("update_pull_request", map[string]any{
		"pull_request_id":    777,
		"append_description": "Added more details",
		"repository_id":      "myrepo",
	})
	if err != nil {
		t.Fatalf("update_pull_request failed: %v", err)
	}
	if res["status"] != "success" {
		t.Errorf("expected success, got %v", res["status"])
	}
	if !strings.Contains(res["description"].(string), "Added more details") {
		t.Errorf("expected description to contain appended text, got %v", res["description"])
	}
}

func TestExecutorLinkPullRequestWorkItem(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_executor_link_pr.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	client := ado.NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "pullrequests/777") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 777,
					"title": "PR Title",
					"repository": {"id": "repo-id", "name": "myrepo", "project": {"name": "testproj"}}
				}`)),
			}
		}

		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "workitems/888") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": 888,
					"fields": {"System.Title": "Linked Task"}
				}`)),
			}
		}

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "pullRequests/777/workitems") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(`[{"id": "888"}]`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	executor := NewExecutor(client, database, nil, nil)
	res, err := executor.Execute("link_pull_request_work_item", map[string]any{
		"pull_request_id": 777,
		"work_item_id":    888,
		"repository_id":   "myrepo",
	})
	if err != nil {
		t.Fatalf("link_pull_request_work_item failed: %v", err)
	}
	if res["status"] != "success" {
		t.Errorf("expected success, got %v", res["status"])
	}
	if !executor.AffectedIDs[888] {
		t.Errorf("expected AffectedIDs[888] to be true")
	}
}

func TestGetAdoToolsPRDeclarations(t *testing.T) {
	tools := GetAdoTools()
	if len(tools) == 0 {
		t.Fatalf("expected non-empty tools list")
	}

	foundUpload := false
	foundUpdate := false
	foundLink := false
	foundCreate := false
	foundAddReviewers := false
	foundGetDetails := false
	foundGetComments := false
	foundAddComment := false
	foundReplyComment := false

	for _, decl := range tools[0].FunctionDeclarations {
		switch decl.Name {
		case "upload_pull_request_attachment":
			foundUpload = true
		case "update_pull_request":
			foundUpdate = true
		case "link_pull_request_work_item":
			foundLink = true
		case "create_pull_request":
			foundCreate = true
		case "add_pull_request_reviewers":
			foundAddReviewers = true
		case "get_pull_request_details":
			foundGetDetails = true
		case "get_pull_request_comments":
			foundGetComments = true
		case "add_pull_request_comment":
			foundAddComment = true
		case "reply_pull_request_comment":
			foundReplyComment = true
		}
	}

	if !foundUpload {
		t.Errorf("upload_pull_request_attachment not found in GetAdoTools()")
	}
	if !foundUpdate {
		t.Errorf("update_pull_request not found in GetAdoTools()")
	}
	if !foundLink {
		t.Errorf("link_pull_request_work_item not found in GetAdoTools()")
	}
	if !foundCreate {
		t.Errorf("create_pull_request not found in GetAdoTools()")
	}
	if !foundAddReviewers {
		t.Errorf("add_pull_request_reviewers not found in GetAdoTools()")
	}
	if !foundGetDetails {
		t.Errorf("get_pull_request_details not found in GetAdoTools()")
	}
	if !foundGetComments {
		t.Errorf("get_pull_request_comments not found in GetAdoTools()")
	}
	if !foundAddComment {
		t.Errorf("add_pull_request_comment not found in GetAdoTools()")
	}
	if !foundReplyComment {
		t.Errorf("reply_pull_request_comment not found in GetAdoTools()")
	}
}

func TestExecutorCreatePullRequestAndReviewers(t *testing.T) {
	tempDB := t.TempDir() + "/test_pr.db"
	database, err := db.Open(tempDB)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer database.Close()

	// Insert identity in DB to test reviewer resolution
	_ = database.SaveIdentities([]*ado.IdentityRef{
		{
			ID:          "rev-guid-111",
			DisplayName: "Alice Smith",
			UniqueName:  "alice.smith@example.com",
		},
		{
			ID:          "rev-guid-222",
			DisplayName: "Bob Jones",
			UniqueName:  "bob.jones@example.com",
		},
	})

	client := ado.NewClient("myorg", "myproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "pullrequests") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 404,
					"title": "Feature Auth",
					"sourceRefName": "refs/heads/feature/auth",
					"targetRefName": "refs/heads/main",
					"status": "active"
				}`)),
			}
		}

		if req.Method == http.MethodPut && strings.Contains(req.URL.Path, "reviewers/rev-guid-111") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": "rev-guid-111",
					"displayName": "Alice Smith",
					"uniqueName": "alice.smith@example.com",
					"isRequired": true
				}`)),
			}
		}

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "attachments") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(`{"count": 0, "value": []}`)),
			}
		}

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "workitems") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(`{"count": 0, "value": []}`)),
			}
		}

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "pullrequests/404") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 404,
					"title": "Feature Auth",
					"status": "active",
					"sourceRefName": "refs/heads/feature/auth",
					"targetRefName": "refs/heads/main"
				}`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	executor := NewExecutor(client, database, nil, nil)

	// 1. Test create_pull_request
	resCreate, err := executor.Execute("create_pull_request", map[string]any{
		"source_branch":      "feature/auth",
		"title":              "Feature Auth",
		"target_branch":      "main",
		"repository_id":      "myrepo",
		"required_reviewers": []string{"Alice Smith"},
	})
	if err != nil {
		t.Fatalf("create_pull_request failed: %v", err)
	}
	if resCreate["pull_request_id"] != 404 {
		t.Errorf("expected pull_request_id 404, got %v", resCreate["pull_request_id"])
	}

	// 2. Test add_pull_request_reviewers
	resAddRev, err := executor.Execute("add_pull_request_reviewers", map[string]any{
		"pull_request_id":    404,
		"repository_id":      "myrepo",
		"required_reviewers": []string{"Alice Smith"},
	})
	if err != nil {
		t.Fatalf("add_pull_request_reviewers failed: %v", err)
	}
	if resAddRev["status"] != "success" {
		t.Errorf("expected success, got %v", resAddRev["status"])
	}

	// 3. Test get_pull_request_details
	resDetails, err := executor.Execute("get_pull_request_details", map[string]any{
		"pull_request_id": 404,
		"repository_id":   "myrepo",
	})
	if err != nil {
		t.Fatalf("get_pull_request_details failed: %v", err)
	}
	if resDetails["pull_request_id"] != 404 || resDetails["title"] != "Feature Auth" {
		t.Errorf("unexpected details: %v", resDetails)
	}
}

func TestExecuteGetPullRequestComments(t *testing.T) {
	client := ado.NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")
		if strings.Contains(req.URL.Path, "/threads") {
			body := `{
				"count": 1,
				"value": [
					{
						"id": 10,
						"status": "active",
						"threadContext": {
							"filePath": "/pkg/auth.go",
							"rightFileStart": {
								"line": 42
							}
						},
						"comments": [
							{
								"id": 1,
								"content": "Consider caching the token here",
								"commentType": "text",
								"author": {
									"displayName": "Reviewer One",
									"uniqueName": "rev1@test.com"
								},
								"publishedDate": "2026-09-04T10:00:00Z"
							},
							{
								"id": 2,
								"content": "Updated branch with commit xyz",
								"commentType": "system",
								"author": {
									"displayName": "System"
								},
								"publishedDate": "2026-09-04T10:05:00Z"
							}
						]
					}
				]
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	executor := NewExecutor(client, database, nil, nil)

	// 1. Without include_system (system comment should be filtered out)
	res, err := executor.Execute("get_pull_request_comments", map[string]any{
		"pull_request_id": 404,
		"repository_id":   "myrepo",
	})
	if err != nil {
		t.Fatalf("get_pull_request_comments failed: %v", err)
	}
	if res["status"] != "success" {
		t.Errorf("expected status success, got %v", res["status"])
	}
	if res["comment_count"] != 1 {
		t.Errorf("expected 1 human comment, got %v", res["comment_count"])
	}
	comments, ok := res["comments"].([]map[string]any)
	if !ok || len(comments) != 1 {
		t.Fatalf("unexpected comments slice: %+v", res["comments"])
	}
	if comments[0]["content"] != "Consider caching the token here" {
		t.Errorf("unexpected content: %v", comments[0]["content"])
	}
	if comments[0]["file_path"] != "/pkg/auth.go" || comments[0]["line"] != 42 {
		t.Errorf("unexpected file_path or line: %+v", comments[0])
	}

	// 2. With include_system: true
	resSys, err := executor.Execute("get_pull_request_comments", map[string]any{
		"pull_request_id": 404,
		"repository_id":   "myrepo",
		"include_system":  true,
	})
	if err != nil {
		t.Fatalf("get_pull_request_comments with include_system failed: %v", err)
	}
	if resSys["comment_count"] != 2 {
		t.Errorf("expected 2 comments with system comments included, got %v", resSys["comment_count"])
	}
}

func TestExecuteReplyPullRequestComment(t *testing.T) {
	client := ado.NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		// GET threads for thread discovery by comment_id
		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/threads") {
			body := `{
				"count": 1,
				"value": [
					{
						"id": 10,
						"status": "active",
						"comments": [
							{
								"id": 101,
								"content": "Need review here",
								"commentType": "text"
							}
						]
					}
				]
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
		}

		// POST comment reply to thread
		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/threads/10/comments") {
			body := `{
				"id": 102,
				"parentCommentId": 101,
				"content": "Fixed in latest commit",
				"commentType": "text",
				"author": {
					"displayName": "Developer One"
				}
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
		}

		// PATCH thread status
		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "/threads/10") {
			body := `{
				"id": 10,
				"status": "fixed"
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	database, err := db.Open(filepath.Join(t.TempDir(), "test_reply.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	executor := NewExecutor(client, database, nil, nil)

	// 1. Reply to thread with content and resolved: true
	res, err := executor.Execute("reply_pull_request_comment", map[string]any{
		"pull_request_id": 404,
		"repository_id":   "myrepo",
		"thread_id":       10,
		"content":         "Fixed in latest commit",
		"resolved":        true,
	})
	if err != nil {
		t.Fatalf("reply_pull_request_comment failed: %v", err)
	}
	if res["status"] != "success" {
		t.Errorf("expected success status, got %v", res["status"])
	}
	if res["comment_id"] != 102 {
		t.Errorf("expected comment_id 102, got %v", res["comment_id"])
	}
	if res["thread_status"] != "fixed" {
		t.Errorf("expected thread_status fixed, got %v", res["thread_status"])
	}
	if res["is_resolved"] != true {
		t.Errorf("expected is_resolved true, got %v", res["is_resolved"])
	}

	// 2. Reply via comment_id lookup
	resLookup, err := executor.Execute("reply_pull_request_comment", map[string]any{
		"pull_request_id": 404,
		"repository_id":   "myrepo",
		"comment_id":      101,
		"content":         "Fixed in latest commit",
		"status":          "resolved",
	})
	if err != nil {
		t.Fatalf("reply_pull_request_comment with comment_id lookup failed: %v", err)
	}
	if resLookup["thread_id"] != 10 || resLookup["parent_comment_id"] != 101 {
		t.Errorf("unexpected thread_id or parent_comment_id: %+v", resLookup)
	}

	// 3. Error case: Missing pull_request_id
	_, errMissingPR := executor.Execute("reply_pull_request_comment", map[string]any{
		"thread_id": 10,
		"content":   "test",
	})
	if errMissingPR == nil {
		t.Errorf("expected error for missing pull_request_id")
	}

	// 4. Error case: Missing content and status
	_, errMissingAction := executor.Execute("reply_pull_request_comment", map[string]any{
		"pull_request_id": 404,
		"thread_id":       10,
	})
	if errMissingAction == nil {
		t.Errorf("expected error for missing content/status")
	}

	// 5. Error case: Non-existent comment ID
	_, errNotFound := executor.Execute("reply_pull_request_comment", map[string]any{
		"pull_request_id": 404,
		"repository_id":   "myrepo",
		"comment_id":      9999,
		"resolved":        true,
	})
	if errNotFound == nil {
		t.Errorf("expected error for non-existent comment ID")
	}
}

func TestExecuteAddPullRequestComment(t *testing.T) {
	client := ado.NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/threads") {
			body := `{
				"id": 88,
				"status": "active",
				"threadContext": {
					"filePath": "/src/auth.go",
					"rightFileStart": {"line": 10, "offset": 1},
					"rightFileEnd": {"line": 12, "offset": 5}
				},
				"comments": [
					{
						"id": 8801,
						"content": "Consider extracting this helper",
						"commentType": "text",
						"author": {"displayName": "Reviewer Bot"}
					}
				]
			}`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	executor := NewExecutor(client, nil, nil, nil)

	// 1. Success: Inline comment with code selection
	res, err := executor.Execute("add_pull_request_comment", map[string]any{
		"pull_request_id": 101,
		"repository_id":   "myrepo",
		"content":         "Consider extracting this helper",
		"file_path":       "src/auth.go",
		"start_line":      10,
		"end_line":        12,
		"status":          "active",
	})
	if err != nil {
		t.Fatalf("unexpected error adding PR comment: %v", err)
	}
	if res["thread_id"] != 88 || res["comment_id"] != 8801 {
		t.Errorf("unexpected add PR comment result: %+v", res)
	}
	if res["code_selection"] == nil {
		t.Errorf("expected code_selection in result, got nil")
	}

	// 2. Alias invocation: create_pull_request_comment
	aliasRes, err := executor.Execute("create_pull_request_comment", map[string]any{
		"pull_request_id": 101,
		"repository_id":   "myrepo",
		"description":     "General feedback",
	})
	if err != nil {
		t.Fatalf("unexpected error via alias: %v", err)
	}
	if aliasRes["thread_id"] != 88 {
		t.Errorf("unexpected result: %+v", aliasRes)
	}

	// 3. Error case: Missing PR ID
	_, errMissingPR := executor.Execute("add_pull_request_comment", map[string]any{
		"content": "Some comment",
	})
	if errMissingPR == nil {
		t.Errorf("expected error for missing pull_request_id")
	}

	// 4. Error case: Missing content
	_, errMissingContent := executor.Execute("add_pull_request_comment", map[string]any{
		"pull_request_id": 101,
	})
	if errMissingContent == nil {
		t.Errorf("expected error for missing content")
	}
}

func TestExecutorGetChildrenAndHierarchy(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "test_gemini_hierarchy.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	// Seed work items:
	// Epic 100 -> Feature 200 -> Story 300 -> Task 401, Task 402
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 100,
		Fields: map[string]any{
			"System.WorkItemType": "Epic",
			"System.Title":        "Enterprise Platform",
			"System.State":        "Active",
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 200,
		Fields: map[string]any{
			"System.WorkItemType": "Feature",
			"System.Title":        "Service Mesh Setup",
			"System.State":        "Active",
			"System.Parent":       100,
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 300,
		Fields: map[string]any{
			"System.WorkItemType": "User Story",
			"System.Title":        "Istio Ingress Gateway",
			"System.State":        "Active",
			"System.Parent":       200,
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 401,
		Fields: map[string]any{
			"System.WorkItemType":                       "Task",
			"System.Title":                              "Deploy Gateway Helm Chart",
			"System.State":                              "Done",
			"System.Parent":                             300,
			"Microsoft.VSTS.Scheduling.OriginalEstimate": 6.0,
			"Microsoft.VSTS.Scheduling.RemainingWork":    0.0,
			"Microsoft.VSTS.Scheduling.CompletedWork":    6.0,
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 402,
		Fields: map[string]any{
			"System.WorkItemType":                       "Task",
			"System.Title":                              "Configure VirtualService",
			"System.State":                              "In Progress",
			"System.Parent":                             300,
			"Microsoft.VSTS.Scheduling.OriginalEstimate": 4.0,
			"Microsoft.VSTS.Scheduling.RemainingWork":    2.0,
			"Microsoft.VSTS.Scheduling.CompletedWork":    2.0,
		},
	})

	executor := NewExecutor(nil, database, nil, nil)

	// 1. Test get_children with recursive: true
	resChildrenRec, err := executor.Execute("get_children", map[string]any{
		"work_item_id": 200,
		"recursive":    true,
	})
	if err != nil {
		t.Fatalf("get_children recursive failed: %v", err)
	}
	if resChildrenRec["total_children"] != 3 {
		t.Errorf("expected 3 total recursive children under 200, got %v", resChildrenRec["total_children"])
	}
	rollups, ok := resChildrenRec["rollup_estimates"].(map[string]float64)
	if !ok {
		t.Fatalf("rollup_estimates missing or wrong type")
	}
	if rollups["original_estimate_hours"] != 10.0 {
		t.Errorf("expected original estimate rollup 10.0, got %v", rollups["original_estimate_hours"])
	}
	if rollups["remaining_work_hours"] != 2.0 {
		t.Errorf("expected remaining work rollup 2.0, got %v", rollups["remaining_work_hours"])
	}

	// 2. Test get_children with recursive: false
	resChildrenDirect, err := executor.Execute("get_children", map[string]any{
		"work_item_id": 200,
		"recursive":    false,
	})
	if err != nil {
		t.Fatalf("get_children direct failed: %v", err)
	}
	if resChildrenDirect["total_children"] != 1 {
		t.Errorf("expected 1 direct child under 200, got %v", resChildrenDirect["total_children"])
	}

	// 3. Test get_hierarchy with ancestors
	resHierarchy, err := executor.Execute("get_hierarchy", map[string]any{
		"work_item_id":      300,
		"include_ancestors": true,
	})
	if err != nil {
		t.Fatalf("get_hierarchy failed: %v", err)
	}
	ancestors, ok := resHierarchy["ancestors"].([]*db.WorkItemRecord)
	if !ok || len(ancestors) != 2 {
		t.Fatalf("expected 2 ancestors for 300, got %v", resHierarchy["ancestors"])
	}
	if ancestors[0].ID != 100 || ancestors[1].ID != 200 {
		t.Errorf("unexpected ancestors: %v", ancestors)
	}
	if resHierarchy["total_descendants"] != 2 {
		t.Errorf("expected 2 descendants under 300, got %v", resHierarchy["total_descendants"])
	}
}





