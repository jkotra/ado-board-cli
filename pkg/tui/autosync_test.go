package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFormatNetworkOrSyncError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		isAuto      bool
		wantPrefix  string
		mustContain []string
	}{
		{
			name:        "DNS lookup failure during auto-sync",
			err:         errors.New("Get \"https://dev.azure.com/myorg/_apis/wit/workitems\": dial tcp: lookup dev.azure.com on 127.0.0.53:53: no such host"),
			isAuto:      true,
			wantPrefix:  "⚠️ Auto-sync failed: ",
			mustContain: []string{"Network issue", "cannot resolve Azure DevOps host", "Check internet connection"},
		},
		{
			name:        "DNS lookup failure during manual sync",
			err:         errors.New("dial tcp: lookup dev.azure.com: no such host"),
			isAuto:      false,
			wantPrefix:  "⚠️ Sync failed: ",
			mustContain: []string{"Network issue", "cannot resolve Azure DevOps host", "Check internet connection"},
		},
		{
			name:        "Connection refused",
			err:         errors.New("dial tcp 13.107.42.18:443: connect: connection refused"),
			isAuto:      true,
			wantPrefix:  "⚠️ Auto-sync failed: ",
			mustContain: []string{"Network issue", "cannot connect to Azure DevOps", "Check internet/VPN connection"},
		},
		{
			name:        "Network unreachable",
			err:         errors.New("connect: network is unreachable"),
			isAuto:      false,
			wantPrefix:  "⚠️ Sync failed: ",
			mustContain: []string{"Network issue", "cannot connect to Azure DevOps", "Check internet/VPN connection"},
		},
		{
			name:        "Connection timeout",
			err:         errors.New("net/http: request canceled while waiting for connection (Client.Timeout exceeded while awaiting headers)"),
			isAuto:      true,
			wantPrefix:  "⚠️ Auto-sync failed: ",
			mustContain: []string{"Network issue", "connection timed out", "Azure DevOps is taking too long to respond"},
		},
		{
			name:        "Deadline exceeded",
			err:         errors.New("context deadline exceeded"),
			isAuto:      false,
			wantPrefix:  "⚠️ Sync failed: ",
			mustContain: []string{"Network issue", "connection timed out"},
		},
		{
			name:        "Connection reset by peer",
			err:         errors.New("read tcp 192.168.1.5:54321->13.107.42.18:443: read: connection reset by peer"),
			isAuto:      true,
			wantPrefix:  "⚠️ Auto-sync failed: Network connection interrupted while contacting Azure DevOps.",
			mustContain: []string{"interrupted"},
		},
		{
			name:        "Broken pipe",
			err:         errors.New("write: broken pipe"),
			isAuto:      false,
			wantPrefix:  "⚠️ Sync failed: Network connection interrupted while contacting Azure DevOps.",
			mustContain: []string{"interrupted"},
		},
		{
			name:        "HTTP 401 Unauthorized / Expired PAT",
			err:         errors.New("Azure DevOps REST API returned HTTP 401 Unauthorized: invalid or expired PAT"),
			isAuto:      true,
			wantPrefix:  "⚠️ Auto-sync failed: ",
			mustContain: []string{"authentication failed (HTTP 401)", "Settings (Press S)"},
		},
		{
			name:        "HTTP 403 Forbidden",
			err:         errors.New("Azure DevOps API returned 403 Forbidden: user does not have permission"),
			isAuto:      false,
			wantPrefix:  "⚠️ Sync failed: ",
			mustContain: []string{"permission denied (HTTP 403)", "Check PAT access permissions"},
		},
		{
			name:        "HTTP 503 Service Unavailable",
			err:         errors.New("HTTP 503 Service Unavailable"),
			isAuto:      true,
			wantPrefix:  "⚠️ Auto-sync failed: ",
			mustContain: []string{"service temporarily unavailable (HTTP 503)", "Retrying shortly"},
		},
		{
			name:        "HTTP 502 Bad Gateway",
			err:         errors.New("HTTP 502 Bad Gateway"),
			isAuto:      false,
			wantPrefix:  "⚠️ Sync failed: ",
			mustContain: []string{"Bad Gateway (HTTP 502)", "Retrying shortly"},
		},
		{
			name:        "HTTP 500 Internal Server Error",
			err:         errors.New("HTTP 500 Internal Server Error"),
			isAuto:      true,
			wantPrefix:  "⚠️ Auto-sync failed: ",
			mustContain: []string{"server error (HTTP 500)"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatNetworkOrSyncError(tc.err, tc.isAuto)
			if !strings.HasPrefix(got, tc.wantPrefix) {
				t.Errorf("expected prefix %q, got %q", tc.wantPrefix, got)
			}
			for _, substr := range tc.mustContain {
				if !strings.Contains(got, substr) {
					t.Errorf("expected error message to contain %q, got %q", substr, got)
				}
			}
		})
	}

	// Nil error returns empty string
	if got := FormatNetworkOrSyncError(nil, false); got != "" {
		t.Errorf("expected empty string for nil error, got %q", got)
	}
}

func TestAutoSyncTickCmdScheduling(t *testing.T) {
	app := &AppModel{
		Config: &config.Config{
			AutoSyncIntervalSeconds: 120,
		},
	}

	// Interval > 0 returns a ticker Cmd
	cmd := app.autoSyncTickCmd()
	if cmd == nil {
		t.Fatalf("expected non-nil tea.Cmd when AutoSyncIntervalSeconds=120")
	}

	// Interval = 0 returns nil (disabled)
	app.Config.AutoSyncIntervalSeconds = 0
	if cmd := app.autoSyncTickCmd(); cmd != nil {
		t.Fatalf("expected nil tea.Cmd when AutoSyncIntervalSeconds=0 (disabled)")
	}

	// Interval < 0 returns nil (disabled)
	app.Config.AutoSyncIntervalSeconds = -1
	if cmd := app.autoSyncTickCmd(); cmd != nil {
		t.Fatalf("expected nil tea.Cmd when AutoSyncIntervalSeconds=-1 (disabled)")
	}
}

func TestAppModelAutoSyncLifecycleAndConcurrency(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_autosync_app.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer database.Close()

	// Seed work items
	_ = database.SaveWorkItem("myorg", "myproj", &ado.WorkItem{
		ID: 101,
		Fields: map[string]interface{}{
			"System.Title":        "AutoSync Task 1",
			"System.WorkItemType": "Task",
			"System.State":        "Active",
		},
	})
	_ = database.SaveTrackedRoot(101, "myorg", "myproj", "")

	cfg := &config.Config{
		AutoSyncIntervalSeconds: 120,
		PatToken:                "test-pat-token",
		DefaultOrg:              "myorg",
		DefaultProject:          "myproj",
	}

	app := NewAppModel(cfg, database)

	// 1. Receive MsgAutoSyncTick when not syncing -> sets Syncing = true and returns a Cmd
	if app.Syncing {
		t.Fatalf("expected Syncing to initially be false")
	}

	updated, cmd := app.Update(MsgAutoSyncTick{})
	app = updated.(*AppModel)
	if !app.Syncing {
		t.Fatalf("expected Syncing to be true after MsgAutoSyncTick")
	}
	if cmd == nil {
		t.Fatalf("expected syncAllFromAdoCmd to be returned")
	}

	// 2. Receive another MsgAutoSyncTick while already Syncing -> does NOT launch another sync
	updated, cmd = app.Update(MsgAutoSyncTick{})
	app = updated.(*AppModel)
	if !app.Syncing {
		t.Fatalf("expected Syncing to remain true")
	}

	// 3. Receive MsgSyncDone with success and IsAutoSync: true
	syncedRoots, _ := database.GetAllWorkItems()
	tree := db.BuildTree(syncedRoots, []db.TrackedRoot{{ID: 101, Org: "myorg", Project: "myproj"}}, nil)

	syncDoneMsg := MsgSyncDone{
		Roots:      tree,
		Count:      1,
		Err:        nil,
		IsAutoSync: true,
	}

	updated, cmd = app.Update(syncDoneMsg)
	app = updated.(*AppModel)

	if app.Syncing {
		t.Fatalf("expected Syncing to be reset to false after MsgSyncDone")
	}
	if app.ToastMessage != "Auto-synced 1 work items" {
		t.Fatalf("expected toast 'Auto-synced 1 work items', got %q", app.ToastMessage)
	}
	if app.ToastIsError {
		t.Fatalf("expected ToastIsError to be false")
	}
	if cmd == nil {
		t.Fatalf("expected cmd to contain next autoSyncTickCmd")
	}

	// 4. Receive MsgSyncDone with a network error and IsAutoSync: true
	netErr := errors.New("dial tcp: lookup dev.azure.com: no such host")
	syncDoneErrMsg := MsgSyncDone{
		Roots:      nil,
		Count:      0,
		Err:        netErr,
		IsAutoSync: true,
	}

	updated, _ = app.Update(syncDoneErrMsg)
	app = updated.(*AppModel)

	if app.Syncing {
		t.Fatalf("expected Syncing to be false after error MsgSyncDone")
	}
	if !app.ToastIsError {
		t.Fatalf("expected ToastIsError to be true on error")
	}
	if !strings.Contains(app.ToastMessage, "⚠️ Auto-sync failed: Network issue (cannot resolve Azure DevOps host)") {
		t.Fatalf("expected explicit network error toast, got %q", app.ToastMessage)
	}

	// 5. Manual sync "r" key while syncing is in progress -> rejected with toast
	app.Syncing = true
	updated, cmd = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	app = updated.(*AppModel)
	if app.ToastMessage != "Sync already in progress..." {
		t.Fatalf("expected toast 'Sync already in progress...', got %q", app.ToastMessage)
	}

	// Reset syncing
	app.Syncing = false
}

func TestScreenSettingsAutoSyncIntervalConfiguration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_settings_autosync_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDb := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpDb)

	database, err := db.Open(tmpDb)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		AutoSyncIntervalSeconds: 90,
		PatToken:                "pat123",
	}

	settings := NewScreenSettings()
	settings.Reset(cfg, database)

	// Check initial input value
	if settings.InputAutoSyncInterval.Value() != "90" {
		t.Fatalf("expected InputAutoSyncInterval to be '90', got %q", settings.InputAutoSyncInterval.Value())
	}

	// Check Values() returns 90
	_, _, _, _, _, _, autoSyncVal, _, _, _, _, _, _, _, _, _, _, _ := settings.Values()
	if autoSyncVal != 90 {
		t.Fatalf("expected Values() to return autoSyncVal 90, got %d", autoSyncVal)
	}

	// Change input value to 300
	settings.InputAutoSyncInterval.SetValue("300")
	_, _, _, _, _, _, autoSyncVal, _, _, _, _, _, _, _, _, _, _, _ = settings.Values()
	if autoSyncVal != 300 {
		t.Fatalf("expected Values() to return autoSyncVal 300, got %d", autoSyncVal)
	}

	// Change input value to "0 (disabled)"
	settings.InputAutoSyncInterval.SetValue("0")
	_, _, _, _, _, _, autoSyncVal, _, _, _, _, _, _, _, _, _, _, _ = settings.Values()
	if autoSyncVal != 0 {
		t.Fatalf("expected Values() to return autoSyncVal 0, got %d", autoSyncVal)
	}

	// Non-numeric or invalid string defaults to 120
	settings.InputAutoSyncInterval.SetValue("invalid")
	_, _, _, _, _, _, autoSyncVal, _, _, _, _, _, _, _, _, _, _, _ = settings.Values()
	if autoSyncVal != 120 {
		t.Fatalf("expected Values() to default to 120 on invalid input, got %d", autoSyncVal)
	}

	// Test saving through saveSettingsCmd persists to DB and Config
	app := NewAppModel(cfg, database)
	saveCmd := app.saveSettingsCmd("pat123", "", "gemini-3.7-flash", true, false, true, 45,
		false, false, false, false, false, false, false, "", []string{"[All]"}, []string{"[All]"}, []string{"[All]"})
	msg := saveCmd()
	updated, _ := app.Update(msg)
	app = updated.(*AppModel)

	if app.Config.AutoSyncIntervalSeconds != 45 {
		t.Fatalf("expected Config.AutoSyncIntervalSeconds to be 45, got %d", app.Config.AutoSyncIntervalSeconds)
	}

	val, err := database.GetSetting("auto_sync_interval_seconds")
	if err != nil || val != "45" {
		t.Fatalf("expected auto_sync_interval_seconds in DB to be '45', got %q, err %v", val, err)
	}
}
