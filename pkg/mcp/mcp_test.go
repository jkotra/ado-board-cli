package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
	"ado-board-cli/pkg/gemini"
	"ado-board-cli/pkg/ipc"

	mcp_lib "github.com/mark3labs/mcp-go/mcp"
)

func TestMCPServerToolsRegistrationAndSummary(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_mcp.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	// Seed test data
	_ = database.SaveTrackedRootWithQuery(100, "org", "proj", "https://dev.azure.com/org/proj/_workitems/edit/100", "Custom Root Title")
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 100,
		Fields: map[string]any{
			"System.Title":                              "Custom Root Title",
			"System.WorkItemType":                       "Feature",
			"System.State":                              "Active",
			"Microsoft.VSTS.Scheduling.OriginalEstimate": 10.0,
			"Microsoft.VSTS.Scheduling.RemainingWork":    5.0,
			"Microsoft.VSTS.Scheduling.Effort":           8.0,
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 101,
		Fields: map[string]any{
			"System.Title":                              "Child Task",
			"System.WorkItemType":                       "Task",
			"System.State":                              "Active",
			"System.Parent":                             100,
			"Microsoft.VSTS.Scheduling.OriginalEstimate": 6.0,
			"Microsoft.VSTS.Scheduling.RemainingWork":    3.0,
		},
	})

	cfg := &config.Config{
		DefaultOrg:     "org",
		DefaultProject: "proj",
	}

	server := NewServer(nil, database, cfg)
	if server == nil {
		t.Fatalf("expected non-nil MCP server")
	}

	// Direct execution via db and tree
	roots, err := database.GetTrackedRoots()
	if err != nil || len(roots) != 1 {
		t.Fatalf("expected 1 tracked root, got %v", roots)
	}

	items, err := database.GetAllWorkItems()
	if err != nil || len(items) != 2 {
		t.Fatalf("expected 2 work items, got %v", items)
	}
}

func TestAppControlToolsWithIPCBridge(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tempDir)

	// 1. Start mock IPC server
	received := make(chan ipc.Request, 5)
	ipcServer, err := ipc.StartServer(func(req ipc.Request) ipc.Response {
		received <- req
		return ipc.Response{
			Success: true,
			Message: "OK from TUI for " + string(req.Command),
		}
	})
	if err != nil {
		t.Fatalf("failed to start mock IPC server: %v", err)
	}
	defer ipcServer.Close()

	server := NewServer(nil, nil, nil)
	if server == nil {
		t.Fatalf("expected server")
	}

	ctx := context.Background()

	// Test app_switch_tab
	switchResp, err := ipc.SendCommand(ctx, ipc.Request{
		Command:  ipc.CmdSwitchTab,
		TabIndex: 2,
		TabName:  "Feature 100",
	})
	if err != nil {
		t.Fatalf("failed to send switch_tab: %v", err)
	}
	if !switchResp.Success || switchResp.Message != "OK from TUI for switch_tab" {
		t.Fatalf("unexpected response: %+v", switchResp)
	}

	// Test app_show_toast
	toastResp, err := ipc.SendCommand(ctx, ipc.Request{
		Command:      ipc.CmdShowToast,
		ToastMessage: "Hello from AI",
		IsError:      false,
	})
	if err != nil {
		t.Fatalf("failed to send show_toast: %v", err)
	}
	if !toastResp.Success || toastResp.Message != "OK from TUI for show_toast" {
		t.Fatalf("unexpected response: %+v", toastResp)
	}
}

func TestSetupAgentsAndClaudeConfigGenerators(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	binaryPath := "/usr/local/bin/ado-board-cli"

	// 1. Test SetupAgents (Global)
	outAgents, err := SetupAgents(binaryPath, false)
	if err != nil {
		t.Fatalf("SetupAgents global failed: %v", err)
	}
	if !strings.Contains(outAgents, "SKILL.md") || !strings.Contains(outAgents, "mcp_config.json") {
		t.Fatalf("unexpected SetupAgents output: %s", outAgents)
	}

	// Check that global SKILL.md was written
	skillFile := filepath.Join(tempHome, ".gemini", "config", "skills", "ado-board", "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("expected SKILL.md at %s: %v", skillFile, err)
	}
	if !strings.Contains(string(data), "Azure DevOps Board & Work Item Management") {
		t.Fatalf("unexpected SKILL.md content")
	}

	// Check that global mcp_config.json was written
	mcpFile := filepath.Join(tempHome, ".gemini", "config", "mcp_config.json")
	mcpData, err := os.ReadFile(mcpFile)
	if err != nil {
		t.Fatalf("expected mcp_config.json at %s: %v", mcpFile, err)
	}
	var mcpParsed map[string]any
	if err := json.Unmarshal(mcpData, &mcpParsed); err != nil {
		t.Fatalf("failed to parse mcp_config.json: %v", err)
	}
	servers, ok := mcpParsed["mcpServers"].(map[string]any)
	if !ok || servers["ado-board"] == nil {
		t.Fatalf("expected ado-board in mcpServers: %+v", mcpParsed)
	}

	// 2. Test SetupClaude (Global)
	outClaude, err := SetupClaude(binaryPath, false)
	if err != nil {
		t.Fatalf("SetupClaude global failed: %v", err)
	}
	if !strings.Contains(outClaude, "Claude") {
		t.Fatalf("unexpected SetupClaude output: %s", outClaude)
	}

	// 3. Test Local Project Setup
	localDir := t.TempDir()
	origWd, _ := os.Getwd()
	_ = os.Chdir(localDir)
	defer func() { _ = os.Chdir(origWd) }()

	outLocalAgents, err := SetupAgents(binaryPath, true)
	if err != nil {
		t.Fatalf("SetupAgents local failed: %v", err)
	}
	if !strings.Contains(outLocalAgents, ".gemini") {
		t.Fatalf("unexpected local SetupAgents output: %s", outLocalAgents)
	}

	// Verify local workspace files
	if _, err := os.Stat(filepath.Join(localDir, ".gemini", "skills", "ado-board", "SKILL.md")); err != nil {
		t.Errorf("local SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localDir, ".gemini", "mcp_config.json")); err != nil {
		t.Errorf("local mcp_config.json missing: %v", err)
	}

	outLocalClaude, err := SetupClaude(binaryPath, true)
	if err != nil {
		t.Fatalf("SetupClaude local failed: %v", err)
	}
	if !strings.Contains(outLocalClaude, ".mcp.json") {
		t.Fatalf("unexpected local SetupClaude output: %s", outLocalClaude)
	}
	if _, err := os.Stat(filepath.Join(localDir, ".mcp.json")); err != nil {
		t.Errorf("local .mcp.json missing: %v", err)
	}

	// 4. Test SetupDeepseek (Global)
	outDeepseek, err := SetupDeepseek(binaryPath, false)
	if err != nil {
		t.Fatalf("SetupDeepseek global failed: %v", err)
	}
	if !strings.Contains(outDeepseek, "cordis.patch.yml") {
		t.Fatalf("unexpected SetupDeepseek output: %s", outDeepseek)
	}
	globalCordis := filepath.Join(tempHome, ".dsh", "cordis.patch.yml")
	if _, err := os.Stat(globalCordis); err != nil {
		t.Fatalf("expected global cordis.patch.yml: %v", err)
	}
	globalSkill := filepath.Join(tempHome, ".dsh", "skills", "ado-board", "SKILL.md")
	if _, err := os.Stat(globalSkill); err != nil {
		t.Fatalf("expected global deepseek SKILL.md: %v", err)
	}

	// 5. Test SetupDeepseek (Local)
	outLocalDeepseek, err := SetupDeepseek(binaryPath, true)
	if err != nil {
		t.Fatalf("SetupDeepseek local failed: %v", err)
	}
	if !strings.Contains(outLocalDeepseek, "cordis.patch.yml") {
		t.Fatalf("unexpected local SetupDeepseek output: %s", outLocalDeepseek)
	}
	if _, err := os.Stat(filepath.Join(localDir, ".dsh", "skills", "ado-board", "SKILL.md")); err != nil {
		t.Fatalf("expected local .dsh SKILL.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localDir, ".dsh", "cordis.patch.yml")); err != nil {
		t.Fatalf("expected local .dsh cordis.patch.yml: %v", err)
	}

	// 6. Test RemoveCordisEntry
	testPatchFile := filepath.Join(localDir, "test-remove-cordis.patch.yml")
	_ = UpdateCordisPatchFile(testPatchFile, "test-server", "echo", []string{"hi"})
	_ = UpdateCordisPatchFile(testPatchFile, "keep-server", "echo", []string{"ok"})
	if err := RemoveCordisEntry(testPatchFile, "test-server"); err != nil {
		t.Fatalf("RemoveCordisEntry failed: %v", err)
	}
	patchData, _ := os.ReadFile(testPatchFile)
	if strings.Contains(string(patchData), "test-server") {
		t.Fatalf("expected test-server to be removed: %s", string(patchData))
	}
	if !strings.Contains(string(patchData), "keep-server") {
		t.Fatalf("expected keep-server to be retained: %s", string(patchData))
	}
}

func TestMCPTrackingTools(t *testing.T) {
	tempDir := t.TempDir()
	database, err := db.Open(filepath.Join(tempDir, "test_track_mcp.db"))
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	client := ado.NewClient("org", "proj", "pat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
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
							"id": 200,
							"fields": {
								"System.Title": "Tracked Epic via MCP",
								"System.WorkItemType": "Epic",
								"System.State": "New",
								"System.TeamProject": "proj"
							}
						}
					]
				}`)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	cfg := &config.Config{
		DefaultOrg:     "org",
		DefaultProject: "proj",
	}

	server := NewServer(client, database, cfg)
	if server == nil {
		t.Fatalf("expected non-nil server")
	}

	// 1. Test track_work_item through executor
	exec := gemini.NewExecutor(client, database, &gemini.ContextPayload{}, nil)
	resTrack, err := exec.Execute("track_work_item", map[string]any{
		"work_item_id": 200,
		"custom_title": "Custom MCP Epic",
	})
	if err != nil {
		t.Fatalf("track_work_item execution failed: %v", err)
	}
	if resTrack["success"] != true {
		t.Fatalf("expected success on track_work_item: %v", resTrack)
	}

	roots, err := database.GetTrackedRoots()
	if err != nil || len(roots) != 1 || roots[0].ID != 200 {
		t.Fatalf("expected 1 tracked root #200, got %v", roots)
	}
	if roots[0].CustomTitle != "Custom MCP Epic" {
		t.Errorf("expected custom title 'Custom MCP Epic', got %q", roots[0].CustomTitle)
	}

	// 2. Test untrack_work_item
	resUntrack, err := exec.Execute("untrack_work_item", map[string]any{
		"work_item_id": 200,
	})
	if err != nil {
		t.Fatalf("untrack_work_item execution failed: %v", err)
	}
	if resUntrack["success"] != true {
		t.Fatalf("expected success on untrack_work_item: %v", resUntrack)
	}

	rootsAfter, err := database.GetTrackedRoots()
	if err != nil || len(rootsAfter) != 0 {
		t.Fatalf("expected 0 tracked roots after untrack, got %v", rootsAfter)
	}
}

func TestMCPAppGetStateOnlineAndOffline(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tempDir)
	dbPath := filepath.Join(tempDir, "test_mcp_state.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	_ = database.SaveTrackedRoot(500, "org", "proj", "")
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 500,
		Fields: map[string]any{
			"System.Title":        "Offline Root Item",
			"System.WorkItemType": "Feature",
			"System.State":        "Active",
		},
	})

	// 1. Test offline board state fallback
	offlineRes, err := buildOfflineBoardState(database)
	if err != nil {
		t.Fatalf("buildOfflineBoardState failed: %v", err)
	}
	txtContent, ok := offlineRes.Content[0].(mcp_lib.TextContent)
	if !ok || !strings.Contains(txtContent.Text, "Offline Root Item") {
		t.Fatalf("expected offline state to contain Offline Root Item: %+v", offlineRes.Content[0])
	}

	// 2. Test online board state with active IPC
	ipcServer, err := ipc.StartServer(func(req ipc.Request) ipc.Response {
		if req.Command == ipc.CmdGetState {
			return ipc.Response{
				Success: true,
				Data: &ipc.AppStateSummary{
					IsRunning:    true,
					ActiveScreen: "item_details",
					ActiveTab: ipc.TabSummary{
						Index:    0,
						Name:     "Feature 500",
						RootID:   500,
						IsActive: true,
					},
					SelectedItem: &ipc.WorkItemSummary{
						ID:            501,
						Type:          "Task",
						Title:         "Live Selected Item",
						State:         "In Progress",
						RemainingWork: 3.5,
					},
				},
			}
		}
		return ipc.Response{Success: false}
	})
	if err != nil {
		t.Fatalf("failed to start IPC server: %v", err)
	}
	defer ipcServer.Close()

	ctx := context.Background()
	stateResp, err := ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdGetState})
	if err != nil {
		t.Fatalf("CmdGetState failed: %v", err)
	}
	if !stateResp.Success {
		t.Fatalf("expected success on CmdGetState")
	}
	dataMap, ok := stateResp.Data.(map[string]any)
	if !ok || dataMap["active_screen"] != "item_details" {
		t.Fatalf("unexpected state data: %v", stateResp.Data)
	}
	selItem, ok := dataMap["selected_item"].(map[string]any)
	if !ok || selItem["id"] != float64(501) || selItem["title"] != "Live Selected Item" {
		t.Fatalf("unexpected selected item: %v", dataMap["selected_item"])
	}
}

func TestMCPGetUsersTool(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_mcp_users.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	_ = database.SaveIdentities([]*ado.IdentityRef{
		{
			ID:          "guid-10",
			DisplayName: "Doe, John",
			UniqueName:  "john.doe@example.com",
		},
	})

	server := NewServer(nil, database, nil)
	if server == nil {
		t.Fatalf("expected server")
	}

	exec := gemini.NewExecutor(nil, database, nil, nil)
	res, err := exec.Execute("get_users", map[string]any{})
	if err != nil {
		t.Fatalf("get_users failed: %v", err)
	}
	if res["total_users"].(int) != 1 {
		t.Fatalf("expected 1 user, got %v", res["total_users"])
	}
}

func TestMCPNewTools(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tempDir)

	dbPath := filepath.Join(tempDir, "test_mcp_new_tools.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	_ = database.SaveTrackedRootWithQuery(100, "org", "proj", "https://dev.azure.com/org/proj/_workitems/edit/100", "Old Tab Title")

	exec := gemini.NewExecutor(nil, database, nil, nil)

	// Test rename_tab
	renameRes, err := exec.Execute("rename_tab", map[string]any{
		"root_id":  100,
		"new_name": "New Tab Title",
	})
	if err != nil {
		t.Fatalf("rename_tab failed: %v", err)
	}
	if !renameRes["success"].(bool) {
		t.Fatalf("expected rename_tab success: %v", renameRes)
	}

	roots, _ := database.GetTrackedRoots()
	if len(roots) != 1 || roots[0].CustomTitle != "New Tab Title" {
		t.Fatalf("expected custom title 'New Tab Title', got %+v", roots)
	}

	// Test open_in_browser offline / no GUI error graceful handling
	browserRes, err := exec.Execute("open_in_browser", map[string]any{
		"url": "https://dev.azure.com/org/proj/_workitems/edit/100",
	})
	if err != nil {
		t.Fatalf("open_in_browser failed: %v", err)
	}
	if browserRes["url"] != "https://dev.azure.com/org/proj/_workitems/edit/100" {
		t.Fatalf("unexpected browserRes: %v", browserRes)
	}
}

func TestMCPPRTools(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "diagram.png")
	_ = os.WriteFile(filePath, []byte("fake binary data"), 0644)

	dbPath := filepath.Join(tempDir, "test_mcp_pr.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	client := ado.NewClient("org", "proj", "pat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "attachments/diagram.png") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": 123,
					"url": "https://dev.azure.com/org/proj/_apis/git/repositories/repo1/pullRequests/999/attachments/diagram.png"
				}`)),
			}
		}

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "pullrequests") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 999,
					"title": "Test PR",
					"status": "active",
					"sourceRefName": "refs/heads/feature/test",
					"targetRefName": "refs/heads/main",
					"repository": {"name": "repo1"}
				}`)),
			}
		}

		if req.Method == http.MethodPut && strings.Contains(req.URL.Path, "reviewers") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": "user-guid-a",
					"displayName": "User A",
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

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "threads") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"count": 1,
					"value": [
						{
							"id": 1,
							"status": "active",
							"comments": [
								{
									"id": 101,
									"content": "Nice PR!",
									"commentType": "text",
									"author": {"displayName": "User A"}
								}
							]
						}
					]
				}`)),
			}
		}

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "threads/1/comments") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": 102,
					"parentCommentId": 101,
					"content": "Thanks!",
					"commentType": "text",
					"author": {"displayName": "User B"}
				}`)),
			}
		}

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "threads") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": 5,
					"status": "active",
					"threadContext": {
						"filePath": "/pkg/auth.go",
						"rightFileStart": {"line": 20, "offset": 1},
						"rightFileEnd": {"line": 25, "offset": 1}
					},
					"comments": [
						{
							"id": 501,
							"content": "Add error check here",
							"commentType": "text",
							"author": {"displayName": "Reviewer"}
						}
					]
				}`)),
			}
		}

		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "threads/1") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": 1,
					"status": "fixed"
				}`)),
			}
		}

		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "pullrequests/999") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 999,
					"title": "New Title",
					"description": "New Description",
					"repository": {"name": "repo1"}
				}`)),
			}
		}

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "pullrequests/999") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 999,
					"title": "New Title",
					"repository": {"name": "repo1", "id": "repo-guid", "project": {"name": "proj"}}
				}`)),
			}
		}

		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "workitems/100") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": 100,
					"fields": {"System.Title": "Item 100"}
				}`)),
			}
		}

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "pullRequests/999/workitems") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(`[{"id": "100"}]`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	server := NewServer(client, database, nil)
	if server == nil {
		t.Fatalf("expected non-nil MCP server")
	}

	// Test upload_pull_request_attachment
	exec := gemini.NewExecutor(client, database, nil, nil)
	upRes, err := exec.Execute("upload_pull_request_attachment", map[string]any{
		"pull_request_id": 999,
		"file_path":       filePath,
		"repository_id":   "repo1",
	})
	if err != nil {
		t.Fatalf("upload_pull_request_attachment failed: %v", err)
	}
	if upRes["attachment_id"] != 123 {
		t.Errorf("expected attachment_id 123, got %v", upRes["attachment_id"])
	}

	// Test update_pull_request
	updPRRes, err := exec.Execute("update_pull_request", map[string]any{
		"pull_request_id": 999,
		"description":     "New Description",
		"title":           "New Title",
		"repository_id":   "repo1",
	})
	if err != nil {
		t.Fatalf("update_pull_request failed: %v", err)
	}
	if updPRRes["title"] != "New Title" {
		t.Errorf("expected title 'New Title', got %v", updPRRes["title"])
	}

	// Test link_pull_request_work_item
	linkRes, err := exec.Execute("link_pull_request_work_item", map[string]any{
		"pull_request_id": 999,
		"work_item_id":    100,
		"repository_id":   "repo1",
	})
	if err != nil {
		t.Fatalf("link_pull_request_work_item failed: %v", err)
	}
	if linkRes["work_item_id"] != 100 {
		t.Errorf("expected work_item_id 100, got %v", linkRes["work_item_id"])
	}

	// Test create_pull_request
	createRes, err := exec.Execute("create_pull_request", map[string]any{
		"source_branch": "feature/test",
		"title":         "Test PR",
		"repository_id": "repo1",
	})
	if err != nil {
		t.Fatalf("create_pull_request failed: %v", err)
	}
	if createRes["pull_request_id"] != 999 {
		t.Errorf("expected pull_request_id 999, got %v", createRes["pull_request_id"])
	}

	// Test add_pull_request_reviewers
	addRevRes, err := exec.Execute("add_pull_request_reviewers", map[string]any{
		"pull_request_id":    999,
		"required_reviewers": []string{"User A"},
		"repository_id":      "repo1",
	})
	if err != nil {
		t.Fatalf("add_pull_request_reviewers failed: %v", err)
	}
	if addRevRes["status"] != "success" {
		t.Errorf("expected status success, got %v", addRevRes["status"])
	}

	// Test get_pull_request_details
	detRes, err := exec.Execute("get_pull_request_details", map[string]any{
		"pull_request_id": 999,
		"repository_id":   "repo1",
	})
	if err != nil {
		t.Fatalf("get_pull_request_details failed: %v", err)
	}
	if detRes["pull_request_id"] != 999 {
		t.Errorf("expected pull_request_id 999, got %v", detRes["pull_request_id"])
	}

	// Test get_pull_request_comments
	commentsRes, err := exec.Execute("get_pull_request_comments", map[string]any{
		"pull_request_id": 999,
		"repository_id":   "repo1",
	})
	if err != nil {
		t.Fatalf("get_pull_request_comments failed: %v", err)
	}
	if commentsRes["comment_count"] != 1 {
		t.Errorf("expected comment_count 1, got %v", commentsRes["comment_count"])
	}

	// Test add_pull_request_comment
	addCommentRes, err := exec.Execute("add_pull_request_comment", map[string]any{
		"pull_request_id": 999,
		"content":         "Add error check here",
		"file_path":       "pkg/auth.go",
		"start_line":      20,
		"end_line":        25,
		"repository_id":   "repo1",
	})
	if err != nil {
		t.Fatalf("add_pull_request_comment failed: %v", err)
	}
	if addCommentRes["thread_id"] != 5 || addCommentRes["comment_id"] != 501 {
		t.Errorf("unexpected add_pull_request_comment response: %+v", addCommentRes)
	}

	// Test reply_pull_request_comment
	replyRes, err := exec.Execute("reply_pull_request_comment", map[string]any{
		"pull_request_id": 999,
		"thread_id":       1,
		"content":         "Thanks!",
		"resolved":        true,
		"repository_id":   "repo1",
	})
	if err != nil {
		t.Fatalf("reply_pull_request_comment failed: %v", err)
	}
	if replyRes["comment_id"] != 102 {
		t.Errorf("expected comment_id 102, got %v", replyRes["comment_id"])
	}
	if replyRes["thread_status"] != "fixed" || replyRes["is_resolved"] != true {
		t.Errorf("expected thread_status fixed and is_resolved true, got %+v", replyRes)
	}
}

func TestMCPGetChildrenAndHierarchy(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_mcp_hierarchy.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	// Seed work items:
	// Feature 500 -> Story 501 -> Task 502
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 500,
		Fields: map[string]any{
			"System.Title":        "Auth System",
			"System.WorkItemType": "Feature",
			"System.State":        "Active",
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 501,
		Fields: map[string]any{
			"System.Title":        "OAuth Login",
			"System.WorkItemType": "User Story",
			"System.State":        "Active",
			"System.Parent":       500,
		},
	})
	_ = database.SaveWorkItem("org", "proj", &ado.WorkItem{
		ID: 502,
		Fields: map[string]any{
			"System.Title":                              "Implement Callback",
			"System.WorkItemType":                       "Task",
			"System.State":                              "Done",
			"System.Parent":                             501,
			"Microsoft.VSTS.Scheduling.OriginalEstimate": 8.0,
			"Microsoft.VSTS.Scheduling.RemainingWork":    0.0,
			"Microsoft.VSTS.Scheduling.CompletedWork":    8.0,
		},
	})

	cfg := &config.Config{
		DefaultOrg:     "org",
		DefaultProject: "proj",
	}

	server := NewServer(nil, database, cfg)
	if server == nil {
		t.Fatalf("expected non-nil server")
	}

	exec := gemini.NewExecutor(nil, database, nil, nil)

	// Test get_children
	resChildren, err := exec.Execute("get_children", map[string]any{
		"work_item_id": 500,
		"format":       "both",
	})
	if err != nil {
		t.Fatalf("get_children failed: %v", err)
	}
	if resChildren["total_children"] != 2 {
		t.Errorf("expected 2 children, got %v", resChildren["total_children"])
	}

	// Test get_hierarchy
	resHierarchy, err := exec.Execute("get_hierarchy", map[string]any{
		"work_item_id": 501,
	})
	if err != nil {
		t.Fatalf("get_hierarchy failed: %v", err)
	}
	if resHierarchy["total_descendants"] != 1 {
		t.Errorf("expected 1 descendant, got %v", resHierarchy["total_descendants"])
	}
}

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}



