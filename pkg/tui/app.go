package tui

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/clipboard"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
	"ado-board-cli/pkg/gemini"
	"ado-board-cli/pkg/ipc"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ScreenType int

const (
	ScreenTypeTree ScreenType = iota
	ScreenTypeItem
	ScreenTypeCreate
	ScreenTypeSettings
)

func (s ScreenType) String() string {
	switch s {
	case ScreenTypeTree:
		return "Tree"
	case ScreenTypeItem:
		return "Item"
	case ScreenTypeCreate:
		return "Create"
	case ScreenTypeSettings:
		return "Settings"
	default:
		return "Unknown"
	}
}

const (
	MinTerminalWidth  = 115
	MinTerminalHeight = 16
)

type ModalType int

const (
	ModalTypeNone ModalType = iota
	ModalTypeStatus
	ModalTypeAssign
	ModalTypeEstimate
	ModalTypeComment
	ModalTypeTrack
	ModalTypeConfirm
	ModalTypeEdit
	ModalTypeFilter
	ModalTypeSchedule
	ModalTypeTab
	ModalTypeActivity
)

func (m ModalType) String() string {
	switch m {
	case ModalTypeNone:
		return "None"
	case ModalTypeStatus:
		return "Status"
	case ModalTypeAssign:
		return "Assign"
	case ModalTypeEstimate:
		return "Estimate"
	case ModalTypeComment:
		return "Comment"
	case ModalTypeTrack:
		return "Track"
	case ModalTypeConfirm:
		return "Confirm"
	case ModalTypeEdit:
		return "Edit"
	case ModalTypeFilter:
		return "Filter"
	case ModalTypeSchedule:
		return "Schedule"
	case ModalTypeTab:
		return "Tab"
	case ModalTypeActivity:
		return "Activity"
	default:
		return "Unknown"
	}
}

// AppModel is the root Bubble Tea application model
type AppModel struct {
	Config   *config.Config
	DB       *db.DB
	Ado      *ado.Client
	AdoOrg   string
	AdoProj  string
	AdoToken string

	CurrentScreen ScreenType
	ActiveModal   ModalType

	ScreenTree     *ScreenTree
	ScreenItem     *ScreenItem
	ScreenCreate   *ScreenCreate
	ScreenSettings *ScreenSettings

	ModalStatus   *ModalStatus
	ModalAssign   *ModalAssign
	ModalEstimate *ModalEstimate
	ModalComment  *ModalComment
	ModalTrack    *ModalTrack
	ModalConfirm  *ModalConfirm
	ModalEdit     *ModalEdit
	ModalFilter   *ModalFilter
	ModalSchedule *ModalSchedule
	ModalTab      *ModalTab
	ModalActivity *ModalActivity

	DockGemini *DockGemini

	GeminiConfirmChan        chan MsgGeminiConfirmRequest
	PendingGeminiConfirmChan chan bool
	GeminiProgressChan       chan MsgGeminiProgress
	GeminiCancelFunc         context.CancelFunc

	Width         int
	Height        int
	ToastMessage  string
	ToastIsError  bool
	Loading       bool
	Syncing       bool
	Spinner       spinner.Model
	OpenURLFunc         func(url string)
	LastOpenedURL       string

	TypeStatesCache map[string][]string
	IPCServer       *ipc.Server
	IPCReqChan      chan MsgIPCRequest
}

// Messages
type MsgIPCRequest struct {
	Req      ipc.Request
	RespChan chan ipc.Response
}

type MsgGeminiConfirmRequest struct {
	Prompt       gemini.ConfirmPrompt
	ResponseChan chan bool
}

type MsgAutoSyncTick struct{}

type MsgSyncDone struct {
	Roots      []*db.TreeNode
	Count      int
	Err        error
	IsAutoSync bool
}

type MsgItemUpdated struct {
	Item *db.WorkItemRecord
	Err  error
}

type MsgItemCreated struct {
	Item *db.WorkItemRecord
	Err  error
}

type MsgItemDeleted struct {
	ID  int
	Err error
}

type MsgCommentsLoaded struct {
	Comments []db.CommentRecord
	Err      error
}

type MsgCommentAdded struct {
	Comment *db.CommentRecord
	Err     error
}

type MsgTrackDone struct {
	RootID    int
	QueryName string
	Count     int
	Err       error
}

type MsgTypeMetadataLoaded struct {
	Org          string
	Project      string
	WorkItemType string
	Fields       []ado.WorkItemTypeField
	States       []ado.WorkItemState
}

type MsgClearToast struct{}

func NewAppModel(cfg *config.Config, database *db.DB) *AppModel {
	m := &AppModel{
		Config:            cfg,
		DB:                database,
		AdoOrg:            cfg.DefaultOrg,
		AdoProj:           cfg.DefaultProject,
		AdoToken:          cfg.PatToken,
		CurrentScreen:     ScreenTypeTree,
		ActiveModal:       ModalTypeNone,
		ScreenTree:        NewScreenTree(),
		ScreenItem:        NewScreenItem(),
		ScreenCreate:      NewScreenCreate(),
		ScreenSettings:    NewScreenSettings(),
		ModalStatus:       NewModalStatus(),
		ModalAssign:       NewModalAssign(),
		ModalEstimate:     NewModalEstimate(),
		ModalComment:      NewModalComment(),
		ModalTrack:        NewModalTrack(),
		ModalConfirm:      NewModalConfirm(),
		ModalEdit:         NewModalEdit(),
		ModalFilter:       NewModalFilter(),
		ModalSchedule:     NewModalSchedule(),
		ModalTab:          NewModalTab(),
		ModalActivity:     NewModalActivity(),
		DockGemini:        NewDockGemini(cfg.GeminiModel),
		GeminiConfirmChan: make(chan MsgGeminiConfirmRequest, 5),
		GeminiProgressChan: make(chan MsgGeminiProgress, 20),
		TypeStatesCache:   make(map[string][]string),
	}
	m.Ado = m.newADOClient(cfg.DefaultOrg, cfg.DefaultProject)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorSecondary)
	m.Spinner = sp

	m.ModalEstimate.AutoCalcRemaining = cfg.AutoCalcRemaining
	m.ModalEstimate.EffortSameAsEstimate = cfg.EffortSameAsEstimate
	m.ScreenTree.Config = cfg
	m.ScreenTree.DB = database
	m.ScreenItem.Config = cfg
	m.ScreenItem.DB = database

	// Update Gemini model from DB setting if configured
	if dbModel, err := database.GetSetting("gemini_model"); err == nil && dbModel != "" {
		m.DockGemini.ModelName = dbModel
	}

	// Restore persisted UI state (dock height)
	if !cfg.NoResume {
		if uiState, err := database.GetUIState(); err == nil && uiState != nil {
			if uiState.GeminiDockHeight > 0 {
				m.DockGemini.PaneHeight = uiState.GeminiDockHeight
			}
		}
	}
	m.DockGemini.Visible = false

	if idMap, err := database.GetIdentitiesMap(); err == nil && len(idMap) > 0 {
		SetGlobalIdentities(idMap)
	}

	if w, h, err := GetTerminalSize(); err == nil && w > 0 && h > 0 {
		m.Width = w
		m.Height = h
		m.ScreenTree.Width = w
		m.ScreenTree.Height = m.availableScreenHeight()
	}

	m.IPCReqChan = make(chan MsgIPCRequest, 10)
	if srv, err := ipc.StartServer(func(req ipc.Request) ipc.Response {
		respChan := make(chan ipc.Response, 1)
		m.IPCReqChan <- MsgIPCRequest{Req: req, RespChan: respChan}
		select {
		case resp := <-respChan:
			return resp
		case <-time.After(3 * time.Second):
			return ipc.Response{Success: false, Message: "TUI application timed out processing request"}
		}
	}); err == nil {
		m.IPCServer = srv
	}

	return m
}

func (m *AppModel) Close() {
	if m.IPCServer != nil {
		_ = m.IPCServer.Close()
	}
}

func (m *AppModel) waitForIPCCmd() tea.Cmd {
	return func() tea.Msg {
		if m.IPCReqChan == nil {
			return nil
		}
		reqMsg, ok := <-m.IPCReqChan
		if !ok {
			return nil
		}
		return reqMsg
	}
}

func (m *AppModel) Init() tea.Cmd {
	pat := m.Config.PatToken
	if pat == "" && m.DB != nil {
		if dbPat, err := m.DB.GetSetting("pat_token"); err == nil && dbPat != "" {
			pat = dbPat
		}
	}
	geminiKey := m.Config.GeminiApiKey
	if geminiKey == "" && m.DB != nil {
		if dbKey, err := m.DB.GetSetting("gemini_api_key"); err == nil && dbKey != "" {
			geminiKey = dbKey
		}
	}
	geminiModel := m.Config.GeminiModel
	if geminiModel == "" && m.DB != nil {
		if dbModel, err := m.DB.GetSetting("gemini_model"); err == nil && dbModel != "" {
			geminiModel = dbModel
		}
	}

	if m.ScreenSettings != nil {
		if cleanSingleLineInput(pat) != "" {
			m.ScreenSettings.PatStatus = ValidationStatusChecking
		} else {
			m.ScreenSettings.PatStatus = ValidationStatusNotConfigured
		}
		if cleanSingleLineInput(geminiKey) != "" {
			m.ScreenSettings.GeminiStatus = ValidationStatusChecking
		} else {
			m.ScreenSettings.GeminiStatus = ValidationStatusNotConfigured
		}
	}

	return tea.Batch(
		m.reloadTreeCmd(),
		m.syncTeamMembersCmd(),
		m.validateCredentialsCmd(pat, geminiKey, geminiModel, m.AdoOrg, m.AdoProj),
		m.waitForGeminiConfirmCmd(),
		m.waitForGeminiProgressCmd(),
		m.waitForIPCCmd(),
		m.marqueeTickCmd(),
		m.autoSyncTickCmd(),
	)
}

func (m *AppModel) autoSyncTickCmd() tea.Cmd {
	interval := 120
	if m.Config != nil {
		interval = m.Config.AutoSyncIntervalSeconds
	}
	if interval <= 0 {
		return nil
	}
	return tea.Tick(time.Duration(interval)*time.Second, func(t time.Time) tea.Msg {
		return MsgAutoSyncTick{}
	})
}

// FormatNetworkOrSyncError formats network and sync errors into clear, explicit user-facing notifications.
func FormatNetworkOrSyncError(err error, isAuto bool) string {
	if err == nil {
		return ""
	}

	prefix := "⚠️ Sync failed: "
	if isAuto {
		prefix = "⚠️ Auto-sync failed: "
	}

	errStr := err.Error()
	errLower := strings.ToLower(errStr)

	// DNS / Host resolution failure
	if strings.Contains(errLower, "no such host") || strings.Contains(errLower, "lookup") {
		return fmt.Sprintf("%sNetwork issue (cannot resolve Azure DevOps host). Check internet connection.", prefix)
	}

	// Connection refused / unreachable
	if strings.Contains(errLower, "connection refused") || strings.Contains(errLower, "network is unreachable") || strings.Contains(errLower, "no route to host") || strings.Contains(errLower, "host is down") {
		return fmt.Sprintf("%sNetwork issue (cannot connect to Azure DevOps). Check internet/VPN connection.", prefix)
	}

	// Timeout
	if strings.Contains(errLower, "timeout") || strings.Contains(errLower, "deadline exceeded") {
		return fmt.Sprintf("%sNetwork issue (connection timed out). Azure DevOps is taking too long to respond.", prefix)
	}

	// Connection reset / broken pipe / TLS error
	if strings.Contains(errLower, "connection reset") || strings.Contains(errLower, "broken pipe") || strings.Contains(errLower, "tls connection") || strings.Contains(errLower, "handshake") {
		return fmt.Sprintf("%sNetwork connection interrupted while contacting Azure DevOps.", prefix)
	}

	// General network error prefix from resty/ado client
	if strings.Contains(errLower, "network error") {
		return fmt.Sprintf("%sNetwork issue: %s", prefix, errStr)
	}

	// Authentication failure (401)
	if strings.Contains(errLower, "401") || strings.Contains(errLower, "unauthorized") || strings.Contains(errLower, "invalid or expired pat") {
		return fmt.Sprintf("%sAzure DevOps authentication failed (HTTP 401). Please check or renew your PAT token in Settings (Press S).", prefix)
	}

	// Permission denied (403)
	if strings.Contains(errLower, "403") || strings.Contains(errLower, "forbidden") {
		return fmt.Sprintf("%sAzure DevOps permission denied (HTTP 403). Check PAT access permissions.", prefix)
	}

	// Server-side / ADO Service outage (500, 502, 503, 504)
	if strings.Contains(errLower, "503") || strings.Contains(errLower, "service unavailable") {
		return fmt.Sprintf("%sAzure DevOps service temporarily unavailable (HTTP 503). Retrying shortly...", prefix)
	}
	if strings.Contains(errLower, "502") || strings.Contains(errLower, "bad gateway") {
		return fmt.Sprintf("%sAzure DevOps Bad Gateway (HTTP 502). Retrying shortly...", prefix)
	}
	if strings.Contains(errLower, "504") || strings.Contains(errLower, "gateway timeout") {
		return fmt.Sprintf("%sAzure DevOps Gateway Timeout (HTTP 504). Retrying shortly...", prefix)
	}
	if strings.Contains(errLower, "500") || strings.Contains(errLower, "internal server error") {
		return fmt.Sprintf("%sAzure DevOps server error (HTTP 500).", prefix)
	}

	// Fallback to raw error string
	return fmt.Sprintf("%s%s", prefix, errStr)
}

func (m *AppModel) marqueeTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return MarqueeTickMsg(t)
	})
}

func (m *AppModel) waitForGeminiConfirmCmd() tea.Cmd {
	return func() tea.Msg {
		req, ok := <-m.GeminiConfirmChan
		if !ok {
			return nil
		}
		return req
	}
}

func (m *AppModel) waitForGeminiProgressCmd() tea.Cmd {
	return func() tea.Msg {
		if m.GeminiProgressChan == nil {
			return nil
		}
		p, ok := <-m.GeminiProgressChan
		if !ok {
			return nil
		}
		return p
	}
}

func (m *AppModel) syncTeamMembersCmd() tea.Cmd {
	return func() tea.Msg {
		org := m.Config.DefaultOrg
		proj := m.Config.DefaultProject
		if org != "" && proj != "" && m.Config.PatToken != "" {
			client := ado.NewClient(org, proj, m.Config.PatToken)
			if members, err := client.FetchProjectTeamMembers(); err == nil && len(members) > 0 {
				_ = m.DB.SaveIdentities(members)
				if idMap, err := m.DB.GetIdentitiesMap(); err == nil && len(idMap) > 0 {
					SetGlobalIdentities(idMap)
				}
			}
		}
		return nil
	}
}

func (m *AppModel) reloadTreeCmd() tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		if idMap, err := m.DB.GetIdentitiesMap(); err == nil && len(idMap) > 0 {
			SetGlobalIdentities(idMap)
		}
		roots, err := m.DB.GetTrackedRoots()
		if err != nil {
			slog.Error("reloadTree failed to get tracked roots", slog.Any("error", err))
			return MsgSyncDone{Err: err}
		}
		queries, err := m.DB.GetTrackedQueries()
		if err != nil {
			slog.Error("reloadTree failed to get tracked queries", slog.Any("error", err))
			return MsgSyncDone{Err: err}
		}
		if len(roots) == 0 && len(queries) == 0 {
			return MsgSyncDone{Roots: nil, Count: 0}
		}
		items, err := m.DB.GetAllWorkItems()
		if err != nil {
			slog.Error("reloadTree failed to get work items", slog.Any("error", err))
			return MsgSyncDone{Err: err}
		}
		tree := db.BuildTree(items, roots, queries)
		slog.Debug("reloadTree completed",
			slog.Int("items_count", len(items)),
			slog.Int("roots_count", len(tree)),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
		return MsgSyncDone{Roots: tree, Count: len(items)}
	}
}

func (m *AppModel) loadDataCmd() tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		roots, err := m.DB.GetTrackedRoots()
		if err != nil {
			slog.Error("loadData failed to get tracked roots", slog.Any("error", err))
			return MsgSyncDone{Err: err}
		}
		queries, err := m.DB.GetTrackedQueries()
		if err != nil {
			slog.Error("loadData failed to get tracked queries", slog.Any("error", err))
			return MsgSyncDone{Err: err}
		}
		if len(roots) == 0 && len(queries) == 0 {
			return MsgSyncDone{Roots: nil, Count: 0}
		}
		items, err := m.DB.GetAllWorkItems()
		if err != nil {
			slog.Error("loadData failed to get work items", slog.Any("error", err))
			return MsgSyncDone{Err: err}
		}
		tree := db.BuildTree(items, roots, queries)
		slog.Debug("loadData completed",
			slog.Int("items_count", len(items)),
			slog.Int("roots_count", len(tree)),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
		return MsgSyncDone{Roots: tree, Count: len(items)}
	}
}

func (m *AppModel) syncAllFromAdoCmd(isAutoSync bool) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		slog.Info("Syncing all items from ADO", slog.Bool("is_auto_sync", isAutoSync))
		// Sync team members in background
		org := m.Config.DefaultOrg
		proj := m.Config.DefaultProject
		if org != "" && proj != "" && m.Config.PatToken != "" {
			client := ado.NewClient(org, proj, m.Config.PatToken)
			if members, err := client.FetchProjectTeamMembers(); err == nil && len(members) > 0 {
				_ = m.DB.SaveIdentities(members)
				if idMap, err := m.DB.GetIdentitiesMap(); err == nil && len(idMap) > 0 {
					SetGlobalIdentities(idMap)
				}
			}
		}

		if m.Config.PatToken == "" {
			return MsgSyncDone{
				Err:        fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN or configure in Settings"),
				IsAutoSync: isAutoSync,
			}
		}

		totalSynced := 0

		// 1. Sync all tracked queries first
		trackedQueries, err := m.DB.GetTrackedQueries()
		if err != nil {
			slog.Error("syncAllFromAdo failed to get tracked queries", slog.Any("error", err))
			return MsgSyncDone{Err: err, IsAutoSync: isAutoSync}
		}

		for _, tq := range trackedQueries {
			client := ado.NewClient(tq.Org, tq.Project, m.Config.PatToken)
			qInfo, rootIDs, qItems, err := client.FetchQueryHierarchy(tq.ID)
			if err != nil {
				slog.Error("syncAllFromAdo failed for query", slog.String("query_id", tq.ID), slog.Any("error", err))
				return MsgSyncDone{Err: fmt.Errorf("sync failed for query %q: %w", tq.Name, err), IsAutoSync: isAutoSync}
			}
			if len(qItems) > 0 {
				_ = m.DB.SaveTrackedQuery(qInfo.ID, qInfo.Name, qInfo.Path, tq.Org, tq.Project, tq.URL, qInfo.QueryType)
				for _, rID := range rootIDs {
					_ = m.DB.SaveTrackedRootWithQuery(rID, tq.Org, tq.Project, "", tq.ID)
				}
				_ = m.DB.SaveWorkItems(tq.Org, tq.Project, qItems)
				totalSynced += len(qItems)
			}
		}

		// 2. Sync standalone tracked roots (where query_id == "")
		roots, err := m.DB.GetTrackedRoots()
		if err != nil {
			slog.Error("syncAllFromAdo failed to get tracked roots", slog.Any("error", err))
			return MsgSyncDone{Err: err, IsAutoSync: isAutoSync}
		}

		for _, r := range roots {
			if r.QueryID != "" {
				continue // Already synced as part of query
			}
			client := ado.NewClient(r.Org, r.Project, m.Config.PatToken)
			items, err := client.FetchHierarchy(r.ID)
			if err != nil {
				slog.Error("syncAllFromAdo failed for root", slog.Int("root_id", r.ID), slog.Any("error", err))
				return MsgSyncDone{Err: fmt.Errorf("sync failed for #%d: %w", r.ID, err), IsAutoSync: isAutoSync}
			}
			if err := m.DB.SaveWorkItems(r.Org, r.Project, items); err != nil {
				slog.Error("syncAllFromAdo failed to save items to db", slog.Int("root_id", r.ID), slog.Any("error", err))
				return MsgSyncDone{Err: fmt.Errorf("db save failed: %w", err), IsAutoSync: isAutoSync}
			}
			totalSynced += len(items)
		}

		// Background PR refresh and work item types/states cache
		go func() {
			visitedProjects := make(map[string]bool)
			for _, r := range roots {
				if r.Org != "" && r.Project != "" {
					key := fmt.Sprintf("%s/%s", strings.ToLower(r.Org), strings.ToLower(r.Project))
					if visitedProjects[key] {
						continue
					}
					visitedProjects[key] = true
					c := ado.NewClient(r.Org, r.Project, m.Config.PatToken)
					if prs, err := c.GetPullRequestsForProject(30); err == nil {
						for _, pr := range prs {
							_ = m.DB.SavePullRequest(pr, r.Org, r.Project)
						}
					}
					if types, err := c.GetWorkItemTypes(); err == nil {
						for _, t := range types {
							if states, err := c.GetWorkItemStates(t); err == nil && len(states) > 0 {
								_ = m.DB.SaveWorkItemTypeStates(r.Org, r.Project, t, states)
							}
							if fields, err := c.GetWorkItemTypeFields(t); err == nil && len(fields) > 0 {
								_ = m.DB.SaveWorkItemTypeFields(r.Org, r.Project, t, fields)
							}
						}
					}
				}
			}
			for _, q := range trackedQueries {
				if q.Org != "" && q.Project != "" {
					key := fmt.Sprintf("%s/%s", strings.ToLower(q.Org), strings.ToLower(q.Project))
					if visitedProjects[key] {
						continue
					}
					visitedProjects[key] = true
					c := ado.NewClient(q.Org, q.Project, m.Config.PatToken)
					if types, err := c.GetWorkItemTypes(); err == nil {
						for _, t := range types {
							if states, err := c.GetWorkItemStates(t); err == nil && len(states) > 0 {
								_ = m.DB.SaveWorkItemTypeStates(q.Org, q.Project, t, states)
							}
							if fields, err := c.GetWorkItemTypeFields(t); err == nil && len(fields) > 0 {
								_ = m.DB.SaveWorkItemTypeFields(q.Org, q.Project, t, fields)
							}
						}
					}
				}
			}
		}()

		if len(roots) == 0 && len(trackedQueries) == 0 {
			return MsgSyncDone{Roots: nil, Count: 0, IsAutoSync: isAutoSync}
		}

		allItems, err := m.DB.GetAllWorkItems()
		if err != nil {
			slog.Error("syncAllFromAdo failed to get all work items", slog.Any("error", err))
			return MsgSyncDone{Err: err, IsAutoSync: isAutoSync}
		}
		tree := db.BuildTree(allItems, roots, trackedQueries)
		slog.Info("syncAllFromAdo finished",
			slog.Int("total_synced", totalSynced),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
		return MsgSyncDone{Roots: tree, Count: totalSynced, IsAutoSync: isAutoSync}
	}
}

func (m *AppModel) trackCmd(input string) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		slog.Info("Tracking target started", slog.String("input", input))
		if m.Config.PatToken == "" {
			return MsgTrackDone{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		target, err := ado.ParseTrackTarget(input, m.Config.DefaultOrg, m.Config.DefaultProject)
		if err != nil {
			slog.Error("Failed to parse track target", slog.String("input", input), slog.Any("error", err))
			return MsgTrackDone{Err: err}
		}

		client := ado.NewClient(target.Org, target.Project, m.Config.PatToken)

		if target.Type == ado.TrackTargetQuery {
			qInfo, rootIDs, items, err := client.FetchQueryHierarchy(target.QueryID)
			if err != nil {
				slog.Error("Failed to fetch query hierarchy from ADO", slog.String("query_id", target.QueryID), slog.Any("error", err))
				return MsgTrackDone{Err: fmt.Errorf("failed to fetch query from ADO: %w", err)}
			}
			if len(items) == 0 {
				name := target.QueryID
				if qInfo != nil && qInfo.Name != "" {
					name = qInfo.Name
				}
				slog.Warn("Query returned 0 items", slog.String("query_id", target.QueryID), slog.String("name", name))
				return MsgTrackDone{Err: fmt.Errorf("query %q returned 0 work items", name)}
			}

			project := target.Project
			if project == "" && len(items) > 0 {
				project = items[0].GetStringField("System.TeamProject")
			}

			// Save tracked query
			if err := m.DB.SaveTrackedQuery(qInfo.ID, qInfo.Name, qInfo.Path, target.Org, project, target.RawURL, qInfo.QueryType); err != nil {
				slog.Error("Failed to save tracked query to db", slog.String("query_id", qInfo.ID), slog.Any("error", err))
				return MsgTrackDone{Err: fmt.Errorf("failed to save tracked query: %w", err)}
			}

			// Save tracked roots for the query
			for _, rID := range rootIDs {
				_ = m.DB.SaveTrackedRootWithQuery(rID, target.Org, project, "", qInfo.ID)
			}

			// Save all items
			if err := m.DB.SaveWorkItems(target.Org, project, items); err != nil {
				slog.Error("Failed to save query items to db", slog.String("query_id", qInfo.ID), slog.Any("error", err))
				return MsgTrackDone{Err: fmt.Errorf("failed to save items to db: %w", err)}
			}

			// Link PRs asynchronously in background so track returns immediately
			go func() {
				for _, item := range items {
					for _, lpr := range item.LinkedPullRequests() {
						if lpr.PullRequestID > 0 {
							_ = m.DB.LinkWorkItemPullRequest(item.ID, lpr.PullRequestID)
							if pr, err := client.GetPullRequest(lpr.RepositoryID, lpr.PullRequestID); err == nil && pr != nil {
								_ = m.DB.SavePullRequest(pr, target.Org, project)
							}
						}
					}
				}
			}()

			// Update default org/proj in settings if empty
			if m.Config.DefaultOrg == "" {
				m.Config.DefaultOrg = target.Org
				m.Config.DefaultProject = project
				_ = m.DB.SetSetting("default_org", target.Org)
				_ = m.DB.SetSetting("default_project", project)
			}

			firstRootID := 0
			if len(rootIDs) > 0 {
				firstRootID = rootIDs[0]
			}

			slog.Info("Tracked query successfully",
				slog.String("query_id", qInfo.ID),
				slog.String("query_name", qInfo.Name),
				slog.Int("items_count", len(items)),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)

			return MsgTrackDone{
				RootID:    firstRootID,
				QueryName: qInfo.Name,
				Count:     len(items),
			}
		}

		// Work item hierarchy tracking
		items, err := client.FetchHierarchy(target.ItemID)
		if err != nil {
			slog.Error("Failed to fetch work item hierarchy from ADO", slog.Int("item_id", target.ItemID), slog.Any("error", err))
			return MsgTrackDone{Err: fmt.Errorf("failed to fetch from ADO: %w", err)}
		}

		if len(items) == 0 {
			slog.Warn("No work items found for ID", slog.Int("item_id", target.ItemID))
			return MsgTrackDone{Err: fmt.Errorf("no work items found for ID %d", target.ItemID)}
		}

		// Save tracked root
		project := target.Project
		if project == "" && len(items) > 0 {
			project = items[0].GetStringField("System.TeamProject")
		}
		if err := m.DB.SaveTrackedRoot(target.ItemID, target.Org, project, target.RawURL); err != nil {
			slog.Error("Failed to save tracked root to db", slog.Int("item_id", target.ItemID), slog.Any("error", err))
			return MsgTrackDone{Err: fmt.Errorf("failed to save tracked root: %w", err)}
		}

		// Save all items
		if err := m.DB.SaveWorkItems(target.Org, project, items); err != nil {
			slog.Error("Failed to save work items to db", slog.Int("item_id", target.ItemID), slog.Any("error", err))
			return MsgTrackDone{Err: fmt.Errorf("failed to save items to db: %w", err)}
		}

		// Link PRs asynchronously in background
		go func() {
			for _, item := range items {
				for _, lpr := range item.LinkedPullRequests() {
					if lpr.PullRequestID > 0 {
						_ = m.DB.LinkWorkItemPullRequest(item.ID, lpr.PullRequestID)
						if pr, err := client.GetPullRequest(lpr.RepositoryID, lpr.PullRequestID); err == nil && pr != nil {
							_ = m.DB.SavePullRequest(pr, target.Org, project)
						}
					}
				}
			}
		}()

		// Update default org/proj in settings if empty
		if m.Config.DefaultOrg == "" {
			m.Config.DefaultOrg = target.Org
			m.Config.DefaultProject = project
			_ = m.DB.SetSetting("default_org", target.Org)
			_ = m.DB.SetSetting("default_project", project)
		}

		slog.Info("Tracked item successfully",
			slog.Int("item_id", target.ItemID),
			slog.Int("items_count", len(items)),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)

		return MsgTrackDone{
			RootID: target.ItemID,
			Count:  len(items),
		}
	}
}

func (m *AppModel) loadCommentsCmd(id int) tea.Cmd {
	return func() tea.Msg {
		item, err := m.DB.GetWorkItem(id)
		if err != nil {
			return MsgCommentsLoaded{Err: err}
		}

		// Fetch from ADO and update db if PAT is available
		if m.Config.PatToken != "" {
			client := ado.NewClient(item.Org, item.Project, m.Config.PatToken)
			comments, err := client.GetComments(id)
			if err == nil {
				_ = m.DB.SaveComments(id, comments)
				if idMap, err := m.DB.GetIdentitiesMap(); err == nil && len(idMap) > 0 {
					SetGlobalIdentities(idMap)
				}
			}

			// Also refresh work item relations and linked PRs
			if wi, err := client.GetWorkItem(id); err == nil && wi != nil {
				_ = m.DB.SaveWorkItem(item.Org, item.Project, wi)
				_ = m.DB.SaveRelations(id, wi.Relations)
				for _, lpr := range wi.LinkedPullRequests() {
					if lpr.PullRequestID > 0 {
						_ = m.DB.LinkWorkItemPullRequest(id, lpr.PullRequestID)
						if pr, err := client.GetPullRequest(lpr.RepositoryID, lpr.PullRequestID); err == nil && pr != nil {
							_ = m.DB.SavePullRequest(pr, item.Org, item.Project)
						}
					}
				}
			}
		}

		localComments, err := m.DB.GetComments(id)
		return MsgCommentsLoaded{Comments: localComments, Err: err}
	}
}

// newADOClient creates an ADO REST API client with identity resolver wired from SQLite DB
func (m *AppModel) newADOClient(org, project string) *ado.Client {
	token := m.Config.PatToken
	if m.AdoToken != "" {
		token = m.AdoToken
	}
	client := ado.NewClient(org, project, token)
	if m.DB != nil {
		client.SetIdentityResolver(func(q string) *ado.IdentityRef {
			if id, err := m.DB.FindIdentity(q); err == nil && id != nil {
				return &ado.IdentityRef{
					ID:          id.ID,
					DisplayName: id.DisplayName,
					UniqueName:  id.Email,
				}
			}
			return nil
		})
		if ids, err := m.DB.GetAllIdentities(); err == nil && len(ids) > 0 {
			refs := make([]*ado.IdentityRef, 0, len(ids))
			for _, id := range ids {
				refs = append(refs, &ado.IdentityRef{
					ID:          id.ID,
					DisplayName: id.DisplayName,
					UniqueName:  id.Email,
				})
			}
			client.RegisterIdentities(refs...)
		}
	}
	return client
}

func (m *AppModel) addCommentCmd(id int, text string) tea.Cmd {
	return func() tea.Msg {
		if m.Config.PatToken == "" {
			return MsgCommentAdded{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		item, err := m.DB.GetWorkItem(id)
		if err != nil {
			return MsgCommentAdded{Err: err}
		}

		client := m.newADOClient(item.Org, item.Project)
		comment, err := client.AddComment(id, text)
		if err != nil {
			return MsgCommentAdded{Err: fmt.Errorf("failed to add comment to ADO: %w", err)}
		}

		_ = m.DB.SaveComments(id, []*ado.CommentItem{comment})
		return MsgCommentAdded{
			Comment: &db.CommentRecord{
				WorkItemID:  id,
				CommentID:   comment.ID,
				AuthorName:  comment.CreatedBy.DisplayName,
				AuthorEmail: comment.CreatedBy.UniqueName,
				Text:        comment.Text,
				CreatedDate: comment.CreatedDate,
			},
		}
	}
}

// getStatesForItemType retrieves the valid states dynamically for a specific work item type
func (m *AppModel) getStatesForItemType(itemType string) []string {
	if itemType == "" {
		return []string{"To Do", "In Progress", "Committed", "Done", "Removed"}
	}

	// 1. Check in-memory cache
	if m.TypeStatesCache != nil {
		if states, ok := m.TypeStatesCache[itemType]; ok && len(states) > 0 {
			return states
		}
	} else {
		m.TypeStatesCache = make(map[string][]string)
	}

	// 2. Check SQLite database
	org := m.AdoOrg
	proj := m.AdoProj
	if org == "" && m.Config != nil {
		org = m.Config.DefaultOrg
	}
	if proj == "" && m.Config != nil {
		proj = m.Config.DefaultProject
	}

	if m.DB != nil && org != "" && proj != "" {
		if dbStates, err := m.DB.GetWorkItemTypeStates(org, proj, itemType); err == nil && len(dbStates) > 0 {
			m.TypeStatesCache[itemType] = dbStates
			return dbStates
		}
	}

	// 3. Query ADO Client dynamically if credentials available
	var client *ado.Client
	if m.Ado != nil {
		client = m.Ado
	} else if org != "" && proj != "" && m.Config != nil && m.Config.PatToken != "" {
		client = ado.NewClient(org, proj, m.Config.PatToken)
	}

	if client != nil {
		if adoStates, err := client.GetWorkItemStates(itemType); err == nil && len(adoStates) > 0 {
			var stateNames []string
			for _, st := range adoStates {
				stateNames = append(stateNames, st.Name)
			}
			if len(stateNames) > 0 {
				m.TypeStatesCache[itemType] = stateNames
				if m.DB != nil && org != "" && proj != "" {
					_ = m.DB.SaveWorkItemTypeStates(org, proj, itemType, adoStates)
				}
				return stateNames
			}
		}
	}

	// 4. Fallback: Check distinct states stored for this type in SQLite
	if m.DB != nil {
		if distinctStates, err := m.DB.GetDistinctStatesForType(itemType); err == nil && len(distinctStates) > 0 {
			m.TypeStatesCache[itemType] = distinctStates
			return distinctStates
		}
	}

	// 5. Fallback defaults based on standard ADO WIT templates
	switch strings.ToLower(itemType) {
	case "bug", "production issue", "prod issue", "issue":
		return []string{"New", "Approved", "Committed", "Done", "Removed"}
	case "user story", "product backlog item", "pbi":
		return []string{"New", "Approved", "Committed", "Done", "Removed"}
	case "feature", "epic", "initiative":
		return []string{"New", "In Progress", "Done", "Removed"}
	default:
		return []string{"To Do", "In Progress", "Done", "Removed"}
	}
}

func (m *AppModel) ensureWorkItemTypeMetadataCmd(org, project, itemType string) tea.Cmd {
	return func() tea.Msg {
		if m.DB == nil || org == "" || project == "" || itemType == "" {
			return nil
		}
		hasFields, _ := m.DB.HasWorkItemTypeFields(org, project, itemType)
		hasStates, _ := m.DB.HasWorkItemTypeStates(org, project, itemType)
		if hasFields && hasStates {
			return nil
		}

		pat := ""
		if m.Config != nil && m.Config.PatToken != "" {
			pat = m.Config.PatToken
		} else if m.AdoToken != "" {
			pat = m.AdoToken
		}
		if pat == "" {
			return nil
		}

		c := ado.NewClient(org, project, pat)
		var loadedFields []ado.WorkItemTypeField
		var loadedStates []ado.WorkItemState

		if !hasFields {
			if f, err := c.GetWorkItemTypeFields(itemType); err == nil && len(f) > 0 {
				_ = m.DB.SaveWorkItemTypeFields(org, project, itemType, f)
				loadedFields = f
			}
		}
		if !hasStates {
			if s, err := c.GetWorkItemStates(itemType); err == nil && len(s) > 0 {
				_ = m.DB.SaveWorkItemTypeStates(org, project, itemType, s)
				loadedStates = s
			}
		}

		return MsgTypeMetadataLoaded{
			Org:          org,
			Project:      project,
			WorkItemType: itemType,
			Fields:       loadedFields,
			States:       loadedStates,
		}
	}
}

func (m *AppModel) updateStatusCmd(id int, newState string) tea.Cmd {
	return func() tea.Msg {
		if m.Config.PatToken == "" {
			return MsgItemUpdated{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		item, err := m.DB.GetWorkItem(id)
		if err != nil {
			return MsgItemUpdated{Err: err}
		}

		fields := map[string]interface{}{
			"System.State": newState,
		}

		// Automatically set finish date on transition to Done/completed states if enabled
		autoFinish := m.Config != nil && m.Config.AutoSetFinishDate
		if m.DB != nil {
			if val, err := m.DB.GetSetting("auto_set_finish_date"); err == nil && val != "" {
				autoFinish = (val == "true" || val == "1")
			}
		}
		if autoFinish {
			isDone := false
			if m.DB != nil {
				isDone = m.DB.IsStateCompleted(item.Org, item.Project, item.WorkItemType, newState)
			} else {
				isDone = ado.IsStateDoneOrClosed(newState)
			}
			if isDone {
				finishField := "Microsoft.VSTS.Scheduling.FinishDate"
				if m.DB != nil {
					if ref, ok := m.DB.TypeGetFinishDateField(item.Org, item.Project, item.WorkItemType); ok && ref != "" {
						finishField = ref
					}
				}
				fields[finishField] = ado.FormatAdoDateTime(time.Now())
			}
		}

		client := ado.NewClient(item.Org, item.Project, m.Config.PatToken)
		updated, err := client.UpdateWorkItemFields(id, fields)
		if err != nil {
			return MsgItemUpdated{Err: fmt.Errorf("failed to update status in ADO: %w", err)}
		}

		_ = m.DB.SaveWorkItem(item.Org, item.Project, updated)
		rec, _ := m.DB.GetWorkItem(id)
		return MsgItemUpdated{Item: rec}
	}
}

func (m *AppModel) updateAssigneeCmd(id int, rawAssignee string) tea.Cmd {
	return func() tea.Msg {
		if m.Config.PatToken == "" {
			return MsgItemUpdated{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		item, err := m.DB.GetWorkItem(id)
		if err != nil {
			return MsgItemUpdated{Err: err}
		}

		resolvedAssignee, isUnassign := m.DB.ResolveAssignee(rawAssignee)
		if isUnassign {
			resolvedAssignee = ""
		}

		client := ado.NewClient(item.Org, item.Project, m.Config.PatToken)
		updated, err := client.AssignWorkItem(id, resolvedAssignee)
		if err != nil {
			return MsgItemUpdated{Err: fmt.Errorf("failed to reassign in ADO: %w", err)}
		}

		_ = m.DB.SaveWorkItem(item.Org, item.Project, updated)
		rec, _ := m.DB.GetWorkItem(id)
		return MsgItemUpdated{Item: rec}
	}
}

func (m *AppModel) updateEstimatesFieldsCmd(id int, fields map[string]interface{}) tea.Cmd {
	return func() tea.Msg {
		if m.Config.PatToken == "" {
			return MsgItemUpdated{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		item, err := m.DB.GetWorkItem(id)
		if err != nil {
			return MsgItemUpdated{Err: err}
		}

		if len(fields) == 0 {
			return MsgItemUpdated{Item: item}
		}

		client := m.newADOClient(item.Org, item.Project)
		updated, err := client.UpdateWorkItemFields(id, fields)
		if err != nil {
			return MsgItemUpdated{Err: fmt.Errorf("failed to update estimates in ADO: %w", err)}
		}

		_ = m.DB.SaveWorkItem(item.Org, item.Project, updated)
		rec, _ := m.DB.GetWorkItem(id)
		return MsgItemUpdated{Item: rec}
	}
}

func (m *AppModel) updateEstimatesCmd(id int, orig, rem, comp, effort float64) tea.Cmd {
	item, err := m.DB.GetWorkItem(id)
	if err != nil {
		return func() tea.Msg { return MsgItemUpdated{Err: err} }
	}

	fields := map[string]interface{}{}
	effField := "Microsoft.VSTS.Scheduling.Effort"
	origField := "Microsoft.VSTS.Scheduling.OriginalEstimate"
	remField := "Microsoft.VSTS.Scheduling.RemainingWork"
	compField := "Microsoft.VSTS.Scheduling.CompletedWork"

	if m.DB != nil && item != nil {
		if ref, ok := m.DB.TypeGetEffortField(item.Org, item.Project, item.WorkItemType); ok {
			effField = ref
		}
		if ref, ok := m.DB.TypeGetEstimateField(item.Org, item.Project, item.WorkItemType); ok {
			origField = ref
		}
		if ref, ok := m.DB.TypeGetRemainingWorkField(item.Org, item.Project, item.WorkItemType); ok {
			remField = ref
		}
		if ref, ok := m.DB.TypeGetCompletedWorkField(item.Org, item.Project, item.WorkItemType); ok {
			compField = ref
		}
	}

	isDone := false
	if m.DB != nil && item != nil {
		isDone = m.DB.IsStateCompleted(item.Org, item.Project, item.WorkItemType, item.State)
	} else if item != nil {
		isDone = IsStateDoneOrClosed(item.State)
	}

	if orig >= 0 {
		fields[origField] = orig
	}
	if rem >= 0 && !isDone {
		fields[remField] = rem
	}
	if comp >= 0 {
		fields[compField] = comp
	}
	if effort >= 0 {
		fields[effField] = effort
	}
	return m.updateEstimatesFieldsCmd(id, fields)
}

func (m *AppModel) updateDetailsCmd(id int, title, desc, tags, iteration, area string) tea.Cmd {
	return func() tea.Msg {
		if m.Config.PatToken == "" {
			return MsgItemUpdated{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		item, err := m.DB.GetWorkItem(id)
		if err != nil {
			return MsgItemUpdated{Err: err}
		}

		fields := map[string]interface{}{
			"System.Title":         title,
			"System.Description":   desc,
			"System.Tags":          tags,
			"System.IterationPath": iteration,
			"System.AreaPath":      area,
		}

		client := m.newADOClient(item.Org, item.Project)
		updated, err := client.UpdateWorkItemFields(id, fields)
		if err != nil {
			return MsgItemUpdated{Err: fmt.Errorf("failed to update details in ADO: %w", err)}
		}

		_ = m.DB.SaveWorkItem(item.Org, item.Project, updated)
		rec, _ := m.DB.GetWorkItem(id)
		return MsgItemUpdated{Item: rec}
	}
}

func (m *AppModel) updateActivityCmd(id int, activity string) tea.Cmd {
	return func() tea.Msg {
		if m.Config.PatToken == "" {
			return MsgItemUpdated{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		item, err := m.DB.GetWorkItem(id)
		if err != nil {
			return MsgItemUpdated{Err: err}
		}

		fields := map[string]interface{}{}
		if strings.TrimSpace(activity) != "" {
			fields["Microsoft.VSTS.Common.Activity"] = activity
		} else {
			fields["Microsoft.VSTS.Common.Activity"] = ""
		}

		client := m.newADOClient(item.Org, item.Project)
		updated, err := client.UpdateWorkItemFields(id, fields)
		if err != nil {
			return MsgItemUpdated{Err: fmt.Errorf("failed to update activity in ADO: %w", err)}
		}

		_ = m.DB.SaveWorkItem(item.Org, item.Project, updated)
		rec, _ := m.DB.GetWorkItem(id)
		return MsgItemUpdated{Item: rec}
	}
}

func (m *AppModel) updateScheduleCmd(id int, startDate, targetDate, finishDate string) tea.Cmd {
	return func() tea.Msg {
		if m.Config.PatToken == "" {
			return MsgItemUpdated{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		item, err := m.DB.GetWorkItem(id)
		if err != nil {
			return MsgItemUpdated{Err: err}
		}

		fields := map[string]interface{}{}
		if startDate != item.StartDate {
			if startDate != "" {
				fields["Microsoft.VSTS.Scheduling.StartDate"] = startDate
			} else if item.StartDate != "" {
				fields["Microsoft.VSTS.Scheduling.StartDate"] = nil
			}
		}

		if targetDate != item.TargetDate {
			if targetDate != "" {
				fields["Microsoft.VSTS.Scheduling.TargetDate"] = targetDate
			} else if item.TargetDate != "" {
				fields["Microsoft.VSTS.Scheduling.TargetDate"] = nil
			}
		}

		if finishDate != item.FinishDate {
			if finishDate != "" {
				fields["Microsoft.VSTS.Scheduling.FinishDate"] = finishDate
			} else if item.FinishDate != "" {
				fields["Microsoft.VSTS.Scheduling.FinishDate"] = nil
			}
		}

		if len(fields) == 0 {
			return MsgItemUpdated{Item: item}
		}

		client := ado.NewClient(item.Org, item.Project, m.Config.PatToken)
		updated, err := client.UpdateWorkItemFields(id, fields)
		if err != nil {
			return MsgItemUpdated{Err: fmt.Errorf("failed to update dates in ADO: %w", err)}
		}

		_ = m.DB.SaveWorkItem(item.Org, item.Project, updated)
		rec, _ := m.DB.GetWorkItem(id)
		return MsgItemUpdated{Item: rec}
	}
}

func (m *AppModel) createItemCmd(itemType string, title, desc, assigned, tags, iteration, area string, orig, rem float64, parentID int, startDate, targetDate, finishDate string) tea.Cmd {
	return func() tea.Msg {
		if m.Config.PatToken == "" {
			return MsgItemCreated{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		org := m.Config.DefaultOrg
		project := m.Config.DefaultProject

		// If parent is specified, borrow its org/project and nearest parent iteration
		if parentID > 0 {
			if parent, err := m.DB.GetWorkItem(parentID); err == nil {
				org = parent.Org
				project = parent.Project
			}
		}

		if org == "" || project == "" {
			return MsgItemCreated{Err: fmt.Errorf("no organization or project configured")}
		}

		fields := map[string]interface{}{
			"System.Title": title,
		}
		if tags != "" {
			fields["System.Tags"] = tags
		}
		if iteration != "" {
			fields["System.IterationPath"] = iteration
		} else if parentID > 0 {
			if iter := m.DB.GetNearestParentIteration(parentID); iter != "" {
				fields["System.IterationPath"] = iter
			}
		}
		if area != "" {
			fields["System.AreaPath"] = area
		} else if parentID > 0 {
			if a := m.DB.GetNearestParentArea(parentID); a != "" {
				fields["System.AreaPath"] = a
			}
		}
		if desc != "" {
			fields["System.Description"] = desc
		}
		if assigned != "" {
			fields["System.AssignedTo"] = assigned
		}
		if orig > 0 {
			fields["Microsoft.VSTS.Scheduling.OriginalEstimate"] = orig
			if m.Config != nil && m.Config.EffortSameAsEstimate {
				fields["Microsoft.VSTS.Scheduling.Effort"] = orig
			}
		}
		if rem > 0 {
			fields["Microsoft.VSTS.Scheduling.RemainingWork"] = rem
		}
		if startDate != "" {
			fields["Microsoft.VSTS.Scheduling.StartDate"] = startDate
		}
		if targetDate != "" {
			fields["Microsoft.VSTS.Scheduling.TargetDate"] = targetDate
		}
		if finishDate != "" {
			fields["Microsoft.VSTS.Scheduling.FinishDate"] = finishDate
		}

		client := m.newADOClient(org, project)
		created, err := client.CreateWorkItem(itemType, fields, parentID)
		if err != nil {
			return MsgItemCreated{Err: fmt.Errorf("failed to create work item in ADO: %w", err)}
		}

		_ = m.DB.SaveWorkItem(org, project, created)
		rec, _ := m.DB.GetWorkItem(created.ID)
		return MsgItemCreated{Item: rec}
	}
}

type MsgSettingsSaved struct {
	Pat         string
	GeminiKey   string
	GeminiModel string
	Err         error
}

type MsgCredentialsValidated struct {
	PatChecked    bool
	PatValid      bool
	PatError      string
	GeminiChecked bool
	GeminiValid   bool
	GeminiError   string
}

func (m *AppModel) validateCredentialsCmd(pat, geminiKey, geminiModel, org, proj string) tea.Cmd {
	return func() tea.Msg {
		msg := MsgCredentialsValidated{}

		// 1. Validate PAT
		trimmedPat := cleanSingleLineInput(pat)
		if trimmedPat == "" {
			msg.PatChecked = true
			msg.PatValid = false
			msg.PatError = ""
		} else {
			targetOrg := org
			if targetOrg == "" {
				targetOrg = m.AdoOrg
			}
			if targetOrg == "" && m.Config != nil {
				targetOrg = m.Config.DefaultOrg
			}
			if targetOrg == "" && m.DB != nil {
				if roots, _ := m.DB.GetTrackedRoots(); len(roots) > 0 {
					targetOrg = roots[0].Org
				}
			}
			client := ado.NewClient(targetOrg, proj, trimmedPat)
			valid, err := client.ValidatePAT()
			msg.PatChecked = true
			msg.PatValid = valid
			if err != nil {
				msg.PatError = err.Error()
			}
		}

		// 2. Validate Gemini API Key
		trimmedKey := cleanSingleLineInput(geminiKey)
		if trimmedKey == "" {
			msg.GeminiChecked = true
			msg.GeminiValid = false
			msg.GeminiError = ""
		} else {
			targetModel := geminiModel
			if targetModel == "" && m.Config != nil {
				targetModel = m.Config.GeminiModel
			}
			if targetModel == "" {
				targetModel = config.DefaultGeminiModel
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			valid, err := gemini.ValidateAPIKey(ctx, trimmedKey, targetModel)
			msg.GeminiChecked = true
			msg.GeminiValid = valid
			if err != nil {
				msg.GeminiError = err.Error()
			}
		}

		return msg
	}
}

func (m *AppModel) saveSettingsCmd(
	pat, geminiKey, geminiModel string,
	autoCalc, effortSame, autoSetFinishDate bool,
	autoSyncInterval int,
	critEffort, critEstimate, critStartDate, critTargetDate, critFinishDate, critTagRegex, critActivity bool,
	tagRegex string,
	incompleteTypes, incompleteStatuses, incompleteDateStatuses []string,
) tea.Cmd {
	return func() tea.Msg {
		if m.DB != nil {
			_ = m.DB.SetSetting("pat_token", pat)
			_ = m.DB.SetSetting("gemini_api_key", geminiKey)
			_ = m.DB.SetSetting("gemini_model", geminiModel)
			_ = m.DB.SetSetting("auto_calc_remaining", fmt.Sprintf("%t", autoCalc))
			_ = m.DB.SetSetting("effort_same_as_estimate", fmt.Sprintf("%t", effortSame))
			_ = m.DB.SetSetting("auto_set_finish_date", fmt.Sprintf("%t", autoSetFinishDate))
			_ = m.DB.SetSetting("auto_sync_interval_seconds", fmt.Sprintf("%d", autoSyncInterval))
			_ = m.DB.SetSetting("incomplete_criteria_effort", fmt.Sprintf("%t", critEffort))
			_ = m.DB.SetSetting("incomplete_criteria_estimate", fmt.Sprintf("%t", critEstimate))
			_ = m.DB.SetSetting("incomplete_criteria_start_date", fmt.Sprintf("%t", critStartDate))
			_ = m.DB.SetSetting("incomplete_criteria_target_date", fmt.Sprintf("%t", critTargetDate))
			_ = m.DB.SetSetting("incomplete_criteria_finish_date", fmt.Sprintf("%t", critFinishDate))
			_ = m.DB.SetSetting("incomplete_criteria_tag_regex", fmt.Sprintf("%t", critTagRegex))
			_ = m.DB.SetSetting("incomplete_criteria_activity", fmt.Sprintf("%t", critActivity))
			_ = m.DB.SetSetting("tag_regex", tagRegex)
			_ = m.DB.SetSetting("incomplete_types", strings.Join(incompleteTypes, ","))
			_ = m.DB.SetSetting("incomplete_statuses", strings.Join(incompleteStatuses, ","))
			_ = m.DB.SetSetting("incomplete_date_statuses", strings.Join(incompleteDateStatuses, ","))
		}
		if m.Config != nil {
			m.Config.PatToken = pat
			m.Config.GeminiApiKey = geminiKey
			m.Config.GeminiModel = geminiModel
			m.Config.AutoCalcRemaining = autoCalc
			m.Config.EffortSameAsEstimate = effortSame
			m.Config.AutoSetFinishDate = autoSetFinishDate
			m.Config.AutoSyncIntervalSeconds = autoSyncInterval
			m.Config.IncompleteCriteriaEffort = critEffort
			m.Config.IncompleteCriteriaEstimate = critEstimate
			m.Config.IncompleteCriteriaStartDate = critStartDate
			m.Config.IncompleteCriteriaTargetDate = critTargetDate
			m.Config.IncompleteCriteriaFinishDate = critFinishDate
			m.Config.IncompleteCriteriaTagRegex = critTagRegex
			m.Config.IncompleteCriteriaActivity = critActivity
			m.Config.TagRegex = tagRegex
			m.Config.TagRegexes = ado.ParseRegexRules(tagRegex)
			m.Config.IncompleteTypes = incompleteTypes
			m.Config.IncompleteStatuses = incompleteStatuses
			m.Config.IncompleteDateStatuses = incompleteDateStatuses
		}
		m.AdoToken = pat
		if m.ScreenTree != nil {
			m.ScreenTree.Config = m.Config
		}
		if m.ScreenItem != nil {
			m.ScreenItem.Config = m.Config
		}
		if m.ModalEstimate != nil {
			m.ModalEstimate.AutoCalcRemaining = autoCalc
			m.ModalEstimate.EffortSameAsEstimate = effortSame
		}
		if m.DockGemini != nil {
			m.DockGemini.ModelName = geminiModel
		}
		if m.ScreenSettings != nil {
			if cleanSingleLineInput(pat) != "" {
				m.ScreenSettings.PatStatus = ValidationStatusChecking
			} else {
				m.ScreenSettings.PatStatus = ValidationStatusNotConfigured
			}
			if cleanSingleLineInput(geminiKey) != "" {
				m.ScreenSettings.GeminiStatus = ValidationStatusChecking
			} else {
				m.ScreenSettings.GeminiStatus = ValidationStatusNotConfigured
			}
		}
		return MsgSettingsSaved{
			Pat:         pat,
			GeminiKey:   geminiKey,
			GeminiModel: geminiModel,
		}
	}
}

func (m *AppModel) deleteItemCmd(id int) tea.Cmd {
	return func() tea.Msg {
		if m.Config.PatToken == "" {
			return MsgItemDeleted{Err: fmt.Errorf("Azure DevOps PAT is required. Please set PAT_TOKEN environment variable")}
		}

		item, err := m.DB.GetWorkItem(id)
		if err != nil {
			return MsgItemDeleted{Err: err}
		}

		client := ado.NewClient(item.Org, item.Project, m.Config.PatToken)
		if err := client.DeleteWorkItem(id, false); err != nil {
			return MsgItemDeleted{Err: fmt.Errorf("failed to delete from ADO: %w", err)}
		}

		_ = m.DB.DeleteWorkItem(id)
		return MsgItemDeleted{ID: id}
	}
}

func (m *AppModel) showToast(msg string, isError bool) tea.Cmd {
	m.ToastMessage = msg
	m.ToastIsError = isError
	if m.Loading && !isError {
		return m.Spinner.Tick
	}
	return tea.Tick(4*time.Second, func(t time.Time) tea.Msg {
		return MsgClearToast{}
	})
}

func (m *AppModel) saveUIState() {
	if m.DB == nil || m.ScreenTree == nil {
		return
	}
	state := &db.UIState{
		ActiveTabRootID:  m.ScreenTree.ActiveRootID(),
		ActiveTabIdx:     m.ScreenTree.ActiveTab,
		SelectedItemID:   m.ScreenTree.SelectedItemID(),
		CollapsedNodeIDs: m.ScreenTree.CollapsedNodeIDs(),
		Filter:           m.ScreenTree.GlobalFilterOpts,
		TabFilters:       m.ScreenTree.TabFilterOpts,
	}
	if m.DockGemini != nil {
		state.GeminiDockHeight = m.DockGemini.GetPaneHeight()
	}
	_ = m.DB.SaveUIState(state)
}

func (m *AppModel) updateScreenItemData(item *db.WorkItemRecord) {
	if item == nil {
		return
	}
	m.ScreenItem.Config = m.Config
	m.ScreenItem.IncompleteView = m.ScreenTree.IncompleteView
	comments, _ := m.DB.GetComments(item.ID)
	relItems, _ := m.DB.GetRelatedWorkItems(item.ID)
	prs, _ := m.DB.GetPullRequestsForWorkItem(item.ID)
	m.ScreenItem.SetData(item, comments, relItems, prs)
}

func isRunningInTest() bool {
	if flag.Lookup("test.v") != nil {
		return true
	}
	base := filepath.Base(os.Args[0])
	if strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe") || strings.Contains(base, "__debug_bin") {
		return true
	}
	if os.Getenv("GO_TESTING") != "" || os.Getenv("ADO_TESTING") != "" {
		return true
	}
	return false
}

func (m *AppModel) openURL(url string) {
	if url == "" {
		return
	}
	m.LastOpenedURL = url
	if m.OpenURLFunc != nil {
		m.OpenURLFunc(url)
		return
	}

	// Never spawn external browsers or OS browser processes during automated test runs
	if isRunningInTest() {
		return
	}

	var cmdTemplate string
	if m.DB != nil {
		cmdTemplate, _ = m.DB.GetSetting("open_in_browser_cmd")
	}
	if cmdTemplate == "" && m.Config != nil {
		cmdTemplate = m.Config.OpenInBrowserCmd
	}
	cmdTemplate = strings.TrimSpace(cmdTemplate)

	if cmdTemplate != "" {
		var fullCmd string
		if strings.Contains(cmdTemplate, "{url}") {
			fullCmd = strings.ReplaceAll(cmdTemplate, "{url}", url)
		} else if strings.Contains(cmdTemplate, "{URL}") {
			fullCmd = strings.ReplaceAll(cmdTemplate, "{URL}", url)
		} else if strings.Contains(cmdTemplate, "%s") {
			fullCmd = fmt.Sprintf(cmdTemplate, url)
		} else {
			fullCmd = fmt.Sprintf("%s %q", cmdTemplate, url)
		}

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/c", fullCmd)
		} else {
			cmd = exec.Command("sh", "-c", fullCmd)
		}
		if err := cmd.Start(); err == nil {
			return
		}
	}

	// Fallback to system default browser
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func (m *AppModel) openInBrowser(id int) {
	item, err := m.DB.GetWorkItem(id)
	if err != nil {
		return
	}
	org := item.Org
	if org == "" {
		org = m.Config.DefaultOrg
	}
	project := item.Project
	if project == "" {
		project = m.Config.DefaultProject
	}
	url := ado.BuildWebURL(org, project, item.ID)
	m.openURL(url)
}

func (m *AppModel) copyWorkItemToClipboard(id int) (string, error) {
	item, err := m.DB.GetWorkItem(id)
	if err != nil {
		return "", err
	}
	org := item.Org
	if org == "" {
		org = m.Config.DefaultOrg
	}
	project := item.Project
	if project == "" {
		project = m.Config.DefaultProject
	}
	webURL := ado.BuildWebURL(org, project, item.ID)
	return clipboard.CopyWorkItem(item.WorkItemType, item.ID, item.Title, webURL)
}

func (m *AppModel) compileGeminiContext() *gemini.ContextPayload {
	now := time.Now()
	payload := &gemini.ContextPayload{
		CurrentTime:  now.Format("2006-01-02 15:04:05"),
		UserTimezone: now.Format("MST (-07:00)"),
		Org:          m.AdoOrg,
		Project:      m.AdoProj,
	}

	if m.CurrentScreen == ScreenTypeItem && m.ScreenItem.Item != nil {
		payload.ActiveScreen = "item_details"
		payload.SelectedItem = gemini.RecordToSummary(m.ScreenItem.Item)
		if m.ScreenItem.Item.ParentID > 0 {
			if parent, err := m.DB.GetWorkItem(m.ScreenItem.Item.ParentID); err == nil && parent != nil {
				payload.ParentItem = gemini.RecordToSummary(parent)
			}
		}
		// Direct children
		if children, err := m.DB.GetChildWorkItems(m.ScreenItem.Item.ID); err == nil {
			for _, ch := range children {
				payload.ChildItems = append(payload.ChildItems, gemini.RecordToSummary(ch))
			}
		}
		// Siblings
		if m.ScreenItem.Item.ParentID > 0 {
			if siblings, err := m.DB.GetChildWorkItems(m.ScreenItem.Item.ParentID); err == nil {
				for _, sib := range siblings {
					if sib.ID != m.ScreenItem.Item.ID {
						payload.SiblingItems = append(payload.SiblingItems, gemini.RecordToSummary(sib))
					}
				}
			}
		}
	} else {
		payload.ActiveScreen = "tree_view"
		node := m.ScreenTree.SelectedNode()
		if node != nil && node.Record != nil {
			payload.SelectedItem = gemini.RecordToSummary(node.Record)
			if node.Record.ParentID > 0 {
				if parent, err := m.DB.GetWorkItem(node.Record.ParentID); err == nil && parent != nil {
					payload.ParentItem = gemini.RecordToSummary(parent)
				}
			}
			for _, ch := range node.Children {
				if ch != nil && ch.Record != nil {
					payload.ChildItems = append(payload.ChildItems, gemini.RecordToSummary(ch.Record))
				}
			}
		}
	}

	// Cached identities
	if idMap, err := m.DB.GetIdentitiesMap(); err == nil {
		seen := make(map[string]bool)
		for key, name := range idMap {
			if strings.Contains(key, "@") && !seen[key] {
				seen[key] = true
				payload.CachedIdentities = append(payload.CachedIdentities, gemini.IdentitySummary{
					DisplayName: name,
					Email:       key,
				})
			}
		}
	}

	// Current application configuration
	if m.Config != nil {
		payload.CurrentSettings = &gemini.SettingsSummary{
			AutoSyncIntervalSeconds:      m.Config.AutoSyncIntervalSeconds,
			AutoCalcRemaining:            m.Config.AutoCalcRemaining,
			EffortSameAsEstimate:         m.Config.EffortSameAsEstimate,
			AutoSetFinishDate:            m.Config.AutoSetFinishDate,
			IncompleteCriteriaEffort:     m.Config.IncompleteCriteriaEffort,
			IncompleteCriteriaEstimate:   m.Config.IncompleteCriteriaEstimate,
			IncompleteCriteriaStartDate:  m.Config.IncompleteCriteriaStartDate,
			IncompleteCriteriaTargetDate: m.Config.IncompleteCriteriaTargetDate,
			IncompleteCriteriaFinishDate: m.Config.IncompleteCriteriaFinishDate,
			IncompleteCriteriaTagRegex:   m.Config.IncompleteCriteriaTagRegex,
			IncompleteCriteriaActivity:   m.Config.IncompleteCriteriaActivity,
			TagRegex:                     m.Config.TagRegex,
			TagRegexes:                   m.Config.TagRegexes,
			IncompleteTypes:              m.Config.IncompleteTypes,
			IncompleteStatuses:           m.Config.IncompleteStatuses,
			IncompleteDateStatuses:       m.Config.IncompleteDateStatuses,
			GeminiModel:                  m.Config.GeminiModel,
		}
	}

	return payload
}

func (m *AppModel) recordToIpcSummary(r *db.WorkItemRecord) *ipc.WorkItemSummary {
	if r == nil {
		return nil
	}
	url := ""
	if m.AdoOrg != "" && r.ID > 0 {
		if m.AdoProj != "" {
			url = fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d", m.AdoOrg, m.AdoProj, r.ID)
		} else {
			url = fmt.Sprintf("https://dev.azure.com/%s/_workitems/edit/%d", m.AdoOrg, r.ID)
		}
	}
	return &ipc.WorkItemSummary{
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
		URL:              url,
	}
}

func (m *AppModel) compileAppState() *ipc.AppStateSummary {
	state := &ipc.AppStateSummary{
		IsRunning: true,
	}

	switch m.CurrentScreen {
	case ScreenTypeItem:
		state.ActiveScreen = "item_details"
	case ScreenTypeSettings:
		state.ActiveScreen = "settings"
	case ScreenTypeCreate:
		state.ActiveScreen = "create"
	default:
		state.ActiveScreen = "tree"
	}

	switch m.ActiveModal {
	case ModalTypeFilter:
		state.ActiveModal = "filter"
	case ModalTypeStatus:
		state.ActiveModal = "status"
	case ModalTypeTab:
		state.ActiveModal = "tab_menu"
	case ModalTypeEstimate:
		state.ActiveModal = "estimate"
	case ModalTypeSchedule:
		state.ActiveModal = "schedule"
	default:
		state.ActiveModal = "none"
	}

	// Open tabs
	if m.ScreenTree != nil {
		for i, root := range m.ScreenTree.TreeRoots {
			if root == nil || root.Record == nil {
				continue
			}
			name := root.Record.Title
			tab := ipc.TabSummary{
				Index:    i,
				Name:     name,
				RootID:   root.Record.ID,
				IsQuery:  root.IsQuery || root.Record.WorkItemType == "Query",
				QueryID:  root.QueryID,
				IsActive: i == m.ScreenTree.ActiveTab,
			}
			state.OpenTabs = append(state.OpenTabs, tab)
			if tab.IsActive {
				state.ActiveTab = tab
			}
		}

		state.TotalVisibleItems = len(m.ScreenTree.Flattened)
		state.CursorIndex = m.ScreenTree.Cursor

		// Filters
		state.GlobalFilter = ipc.FilterSummary{
			Statuses:   m.ScreenTree.GlobalFilterOpts.Statuses,
			Assignees:  m.ScreenTree.GlobalFilterOpts.Assignees,
			ItemTypes:  m.ScreenTree.GlobalFilterOpts.ItemTypes,
			SearchTerm: m.ScreenTree.GlobalFilterOpts.SearchTerm,
		}
		tabOpts := m.ScreenTree.GetTabFilter(m.ScreenTree.ActiveTab)
		state.TabFilter = ipc.FilterSummary{
			Statuses:   tabOpts.Statuses,
			Assignees:  tabOpts.Assignees,
			ItemTypes:  tabOpts.ItemTypes,
			SearchTerm: tabOpts.SearchTerm,
		}
	}

	// Selected Item, Parent, Children
	if m.CurrentScreen == ScreenTypeItem && m.ScreenItem != nil && m.ScreenItem.Item != nil {
		state.SelectedItem = m.recordToIpcSummary(m.ScreenItem.Item)
		if m.ScreenItem.Item.ParentID > 0 && m.DB != nil {
			if parent, err := m.DB.GetWorkItem(m.ScreenItem.Item.ParentID); err == nil && parent != nil {
				state.ParentItem = m.recordToIpcSummary(parent)
			}
		}
		if m.DB != nil {
			if children, err := m.DB.GetChildWorkItems(m.ScreenItem.Item.ID); err == nil {
				for _, ch := range children {
					state.ChildItems = append(state.ChildItems, m.recordToIpcSummary(ch))
				}
			}
			if m.ScreenItem.Item.ParentID > 0 {
				if siblings, err := m.DB.GetChildWorkItems(m.ScreenItem.Item.ParentID); err == nil {
					for _, sib := range siblings {
						if sib.ID != m.ScreenItem.Item.ID {
							state.SiblingItems = append(state.SiblingItems, m.recordToIpcSummary(sib))
						}
					}
				}
			}
		}
	} else if m.ScreenTree != nil {
		node := m.ScreenTree.SelectedNode()
		if node != nil && node.Record != nil {
			state.SelectedItem = m.recordToIpcSummary(node.Record)
			if node.Record.ParentID > 0 && m.DB != nil {
				if parent, err := m.DB.GetWorkItem(node.Record.ParentID); err == nil && parent != nil {
					state.ParentItem = m.recordToIpcSummary(parent)
				}
			}
			for _, ch := range node.Children {
				if ch != nil && ch.Record != nil {
					state.ChildItems = append(state.ChildItems, m.recordToIpcSummary(ch.Record))
				}
			}
		}
	}

	return state
}

func (m *AppModel) cancelGemini() {
	if m.GeminiCancelFunc != nil {
		m.GeminiCancelFunc()
		m.GeminiCancelFunc = nil
	}
	if m.PendingGeminiConfirmChan != nil {
		select {
		case m.PendingGeminiConfirmChan <- false:
		default:
		}
		m.PendingGeminiConfirmChan = nil
	}
	if m.ActiveModal == ModalTypeConfirm && m.ModalConfirm != nil && m.ModalConfirm.ActionTag == "gemini_confirm" {
		m.ModalConfirm.Close()
		m.ActiveModal = ModalTypeNone
	}
	if m.DockGemini != nil {
		m.DockGemini.Loading = false
		m.DockGemini.StatusMsg = "Cancelled"
	}
}

func (m *AppModel) askGeminiCmd(query string) tea.Cmd {
	// Cancel any active AI operation before starting new one
	if m.GeminiCancelFunc != nil {
		m.GeminiCancelFunc()
		m.GeminiCancelFunc = nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	m.GeminiCancelFunc = cancel

	return func() tea.Msg {
		defer cancel()

		agent, err := gemini.NewAgent(ctx, m.Config, m.DB, m.Ado)
		if err != nil {
			return MsgGeminiDone{Err: err}
		}

		ctxPayload := m.compileGeminiContext()
		progressCb := func(step string) {
			if m.GeminiProgressChan != nil {
				select {
				case <-ctx.Done():
				case m.GeminiProgressChan <- MsgGeminiProgress{Step: step}:
				default:
				}
			}
		}
		confirmCb := func(prompt gemini.ConfirmPrompt) bool {
			resCh := make(chan bool, 1)
			select {
			case <-ctx.Done():
				return false
			case m.GeminiConfirmChan <- MsgGeminiConfirmRequest{
				Prompt:       prompt,
				ResponseChan: resCh,
			}:
			}

			select {
			case <-ctx.Done():
				return false
			case res, ok := <-resCh:
				if !ok {
					return false
				}
				return res
			}
		}

		result, err := agent.Run(ctx, query, ctxPayload, progressCb, confirmCb)
		return MsgGeminiDone{Result: result, Err: err}
	}
}

func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch t := msg.(type) {
	case spinner.TickMsg, MarqueeTickMsg:
		// skip high-frequency animation ticks
	case tea.KeyMsg:
		slog.Debug("TUI Key Press",
			slog.String("key", t.String()),
			slog.String("screen", m.CurrentScreen.String()),
			slog.String("modal", m.ActiveModal.String()),
		)
	case tea.MouseMsg:
		slog.Debug("TUI Mouse Event",
			slog.String("action", t.String()),
			slog.Int("x", t.X),
			slog.Int("y", t.Y),
			slog.String("screen", m.CurrentScreen.String()),
			slog.String("modal", m.ActiveModal.String()),
		)
	default:
		slog.Debug("TUI Message",
			slog.String("type", fmt.Sprintf("%T", msg)),
			slog.String("screen", m.CurrentScreen.String()),
			slog.String("modal", m.ActiveModal.String()),
		)
	}

	var cmds []tea.Cmd

	if m.Width == 0 || m.Height == 0 {
		if w, h, err := GetTerminalSize(); err == nil && w > 0 && h > 0 {
			m.Width = w
			m.Height = h
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		if m.DockGemini != nil {
			m.DockGemini.SetSize(msg.Width, msg.Height)
		}
		availH := m.availableScreenHeight()
		m.ScreenTree.Width = msg.Width
		m.ScreenTree.Height = availH
		m.ScreenItem.InitViewport(msg.Width, availH)
		m.ScreenCreate.InitViewport(msg.Width, availH)
		m.ScreenSettings.InitViewport(msg.Width, availH)
		return m, tea.ClearScreen

	case MsgClearToast:
		m.ToastMessage = ""
		m.ToastIsError = false
		return m, nil

	case MsgPaste:
		if m.DockGemini != nil && m.DockGemini.Visible {
			m.DockGemini.InsertText(msg.Content)
			availH := m.availableScreenHeight()
			m.ScreenTree.Height = availH
			m.ScreenTree.adjustScroll()
			m.ScreenItem.Height = availH
			m.ScreenItem.SetSize(m.Width, availH)
			return m, nil
		}
		if m.ActiveModal == ModalTypeComment && m.ModalComment != nil {
			m.ModalComment.Textarea.InsertString(msg.Content)
			return m, nil
		}
		if m.ActiveModal == ModalTypeEdit && m.ModalEdit != nil {
			if m.ModalEdit.FocusedIndex == fieldEditDesc {
				m.ModalEdit.TextareaDesc.InsertString(msg.Content)
				return m, nil
			}
		}
		if m.CurrentScreen == ScreenTypeCreate && m.ScreenCreate != nil {
			if m.ScreenCreate.FocusedField == fieldCreateDesc {
				m.ScreenCreate.TextareaDesc.InsertString(msg.Content)
				return m, nil
			}
		}
		return m, nil

	case MsgIPCRequest:
		var resp ipc.Response
		switch msg.Req.Command {
		case ipc.CmdPing:
			resp = ipc.Response{Success: true, Message: "pong"}
		case ipc.CmdSwitchTab:
			switched := false
			if msg.Req.TabIndex >= 0 && msg.Req.TabIndex < len(m.ScreenTree.TreeRoots) {
				m.ScreenTree.SetActiveTab(msg.Req.TabIndex)
				switched = true
			} else if msg.Req.TabName != "" || msg.Req.RootID > 0 {
				switched = m.ScreenTree.SelectTabByNameOrRootID(msg.Req.TabName, msg.Req.RootID)
			}
			if switched {
				m.CurrentScreen = ScreenTypeTree
				m.ActiveModal = ModalTypeNone
				m.showToast("Tab switched by AI", false)
				resp = ipc.Response{Success: true, Message: fmt.Sprintf("Switched to tab %d", m.ScreenTree.ActiveTab)}
			} else {
				resp = ipc.Response{Success: false, Message: "Tab not found"}
			}
		case ipc.CmdNavigateToItem:
			found := m.ScreenTree.NavigateToItem(msg.Req.WorkItemID)
			if found {
				if msg.Req.OpenDetails {
					if rec, err := m.DB.GetWorkItem(msg.Req.WorkItemID); err == nil && rec != nil {
						m.updateScreenItemData(rec)
						m.CurrentScreen = ScreenTypeItem
						m.ActiveModal = ModalTypeNone
						m.showToast(fmt.Sprintf("Navigated to #%d", msg.Req.WorkItemID), false)
						resp = ipc.Response{Success: true, Message: fmt.Sprintf("Navigated to item #%d", msg.Req.WorkItemID)}
						msg.RespChan <- resp
						return m, tea.Batch(m.loadCommentsCmd(rec.ID), m.waitForIPCCmd())
					}
				} else {
					m.CurrentScreen = ScreenTypeTree
					m.ActiveModal = ModalTypeNone
				}
				m.showToast(fmt.Sprintf("Navigated to #%d", msg.Req.WorkItemID), false)
				resp = ipc.Response{Success: true, Message: fmt.Sprintf("Navigated to item #%d", msg.Req.WorkItemID)}
			} else {
				resp = ipc.Response{Success: false, Message: fmt.Sprintf("Item #%d not found in active tree", msg.Req.WorkItemID)}
			}
		case ipc.CmdRefresh:
			m.showToast("Refreshing board...", false)
			resp = ipc.Response{Success: true, Message: "Board refresh triggered"}
			msg.RespChan <- resp
			return m, tea.Batch(m.syncAllFromAdoCmd(false), m.waitForIPCCmd())
		case ipc.CmdSetFilter:
			if msg.Req.ClearAll {
				if msg.Req.FilterScope == "tab" {
					m.ScreenTree.ClearTabFilter(m.ScreenTree.ActiveTab)
				} else if msg.Req.FilterScope == "global" {
					m.ScreenTree.ClearGlobalFilter()
				} else {
					m.ScreenTree.ClearAllFilters()
				}
				m.showToast("Filters cleared by AI", false)
				resp = ipc.Response{Success: true, Message: "Filters cleared successfully"}
			} else {
				opts := db.FilterOptions{
					Statuses:   msg.Req.Statuses,
					Assignees:  msg.Req.Assignees,
					ItemTypes:  msg.Req.ItemTypes,
					SearchTerm: msg.Req.Search,
				}
				if msg.Req.FilterScope == "tab" {
					m.ScreenTree.SetTabFilter(m.ScreenTree.ActiveTab, opts)
				} else {
					m.ScreenTree.SetGlobalFilter(opts)
				}
				m.showToast("Filter updated by AI", false)
				resp = ipc.Response{Success: true, Message: "Filter applied successfully"}
			}
		case ipc.CmdClearFilter:
			scope := strings.ToLower(strings.TrimSpace(msg.Req.FilterScope))
			if scope == "tab" {
				m.ScreenTree.ClearTabFilter(m.ScreenTree.ActiveTab)
				m.showToast("Tab filter cleared by AI", false)
				resp = ipc.Response{Success: true, Message: "Active tab filter cleared"}
			} else if scope == "global" {
				m.ScreenTree.ClearGlobalFilter()
				m.showToast("Global filter cleared by AI", false)
				resp = ipc.Response{Success: true, Message: "Global filter cleared"}
			} else {
				m.ScreenTree.ClearAllFilters()
				m.showToast("All filters cleared by AI", false)
				resp = ipc.Response{Success: true, Message: "All filters cleared"}
			}
		case ipc.CmdRenameTab:
			newName := strings.TrimSpace(msg.Req.NewName)
			if newName == "" {
				resp = ipc.Response{Success: false, Message: "new tab name cannot be empty"}
			} else {
				targetIdx := msg.Req.TabIndex
				rootID := msg.Req.RootID
				if targetIdx < 0 || targetIdx >= len(m.ScreenTree.TreeRoots) {
					// Search by root ID or tab name
					if rootID > 0 || msg.Req.TabName != "" {
						for i, r := range m.ScreenTree.TreeRoots {
							if r != nil && r.Record != nil {
								if (rootID > 0 && r.Record.ID == rootID) || (msg.Req.TabName != "" && strings.EqualFold(r.Record.Title, msg.Req.TabName)) {
									targetIdx = i
									rootID = r.Record.ID
									break
								}
							}
						}
					} else {
						targetIdx = m.ScreenTree.ActiveTab
						if targetIdx >= 0 && targetIdx < len(m.ScreenTree.TreeRoots) && m.ScreenTree.TreeRoots[targetIdx] != nil && m.ScreenTree.TreeRoots[targetIdx].Record != nil {
							rootID = m.ScreenTree.TreeRoots[targetIdx].Record.ID
						}
					}
				}
				if targetIdx >= 0 && targetIdx < len(m.ScreenTree.TreeRoots) {
					m.renameTab(targetIdx, rootID, newName)
					m.showToast(fmt.Sprintf("Tab renamed to %q", newName), false)
					resp = ipc.Response{Success: true, Message: fmt.Sprintf("Tab #%d renamed to %q", targetIdx+1, newName)}
				} else {
					resp = ipc.Response{Success: false, Message: "Target tab not found"}
				}
			}
		case ipc.CmdShowToast:
			m.showToast(msg.Req.ToastMessage, msg.Req.IsError)
			resp = ipc.Response{Success: true, Message: "Toast displayed"}
		case ipc.CmdSetScreen:
			switch strings.ToLower(msg.Req.Screen) {
			case "tree":
				m.CurrentScreen = ScreenTypeTree
				m.ActiveModal = ModalTypeNone
			case "item":
				if sel := m.ScreenTree.SelectedItem(); sel != nil {
					m.updateScreenItemData(sel)
					m.CurrentScreen = ScreenTypeItem
					m.ActiveModal = ModalTypeNone
					msg.RespChan <- ipc.Response{Success: true, Message: "Screen set to item"}
					return m, tea.Batch(m.loadCommentsCmd(sel.ID), m.waitForIPCCmd())
				}
			case "settings":
				m.CurrentScreen = ScreenTypeSettings
				m.ActiveModal = ModalTypeNone
			case "create":
				var parentItem *db.WorkItemRecord
				if sel := m.ScreenTree.SelectedItem(); sel != nil {
					parentItem = sel
				}
				m.ScreenCreate.Reset(parentItem, "", m.DB)
				m.ScreenCreate.InitViewport(m.Width, m.availableScreenHeight())
				m.CurrentScreen = ScreenTypeCreate
				m.ActiveModal = ModalTypeNone
			}
			resp = ipc.Response{Success: true, Message: fmt.Sprintf("Screen set to %s", msg.Req.Screen)}
		case ipc.CmdGetState:
			state := m.compileAppState()
			resp = ipc.Response{
				Success: true,
				Message: "TUI state retrieved",
				Data:    state,
			}
		default:
			resp = ipc.Response{Success: false, Message: fmt.Sprintf("Unknown IPC command: %s", msg.Req.Command)}
		}
		msg.RespChan <- resp
		return m, m.waitForIPCCmd()

	case spinner.TickMsg:
		var cmds []tea.Cmd
		if m.Loading {
			var cmd tea.Cmd
			m.Spinner, cmd = m.Spinner.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if m.DockGemini != nil && m.DockGemini.Loading {
			var cmd tea.Cmd
			m.DockGemini.Spinner, cmd = m.DockGemini.Spinner.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}
		return m, nil

	case MsgGeminiProgress:
		if m.DockGemini != nil {
			m.DockGemini.StatusMsg = msg.Step
		}
		return m, m.waitForGeminiProgressCmd()

	case MsgGeminiConfirmRequest:
		m.PendingGeminiConfirmChan = msg.ResponseChan
		itemTitle := msg.Prompt.Title
		if itemTitle == "" {
			itemTitle = fmt.Sprintf("#%d", msg.Prompt.WorkItemID)
		} else {
			itemTitle = fmt.Sprintf("#%d (%s)", msg.Prompt.WorkItemID, itemTitle)
		}
		m.ModalConfirm.Open(
			"Confirm Destructive AI Action",
			fmt.Sprintf("Gemini wants to delete work item:\n%s\n\nAre you sure you want to proceed?", itemTitle),
			"gemini_confirm",
			msg.Prompt.WorkItemID,
			true,
		)
		m.ActiveModal = ModalTypeConfirm
		return m, m.waitForGeminiConfirmCmd()

	case MsgGeminiDone:
		m.GeminiCancelFunc = nil
		if m.DockGemini != nil {
			m.DockGemini.Loading = false
			if msg.Err != nil {
				if errors.Is(msg.Err, context.Canceled) {
					m.DockGemini.StatusMsg = "Cancelled"
					m.DockGemini.ErrorMsg = ""
					cmds = append(cmds, m.showToast("AI query cancelled", false))
				} else if errors.Is(msg.Err, context.DeadlineExceeded) {
					m.DockGemini.StatusMsg = "Timed out"
					m.DockGemini.ErrorMsg = "Request timed out after 60s"
					cmds = append(cmds, m.showToast("AI query timed out", true))
				} else {
					m.DockGemini.ErrorMsg = msg.Err.Error()
					cmds = append(cmds, m.showToast(msg.Err.Error(), true))
				}
			} else if msg.Result != nil {
				m.DockGemini.ErrorMsg = ""
				m.DockGemini.ResponseMsg = msg.Result.Response
				m.DockGemini.StatusMsg = "✅ Done"
				if msg.Result.SettingsUpdated {
					if m.DB != nil && m.Config != nil {
						if reloadedCfg, err := config.Load(m.Config.DbPath, m.Config.PatToken); err == nil && reloadedCfg != nil {
							m.Config = reloadedCfg
						}
						m.ScreenSettings.Reset(m.Config, m.DB)
						if m.Config.GeminiModel != "" && m.DockGemini != nil {
							m.DockGemini.ModelName = m.Config.GeminiModel
						}
					}
					cmds = append(cmds, m.showToast("Settings updated by AI", false))
				}
				if len(msg.Result.AffectedIDs) > 0 || len(msg.Result.ToolExecLogs) > 0 || msg.Result.SettingsUpdated {
					cmds = append(cmds,
						m.reloadTreeCmd(),
					)
					if m.CurrentScreen == ScreenTypeItem && m.ScreenItem.Item != nil {
						for _, id := range msg.Result.AffectedIDs {
							if id == m.ScreenItem.Item.ID {
								if updated, err := m.DB.GetWorkItem(id); err == nil && updated != nil {
									m.updateScreenItemData(updated)
								}
								cmds = append(cmds, m.loadCommentsCmd(id))
							}
						}
					}
				}
			}
		}
		return m, tea.Batch(cmds...)

	case MsgAutoSyncTick:
		if m.Config != nil && m.Config.AutoSyncIntervalSeconds <= 0 {
			return m, nil
		}
		if m.Syncing {
			// A sync is already in progress; try next cycle
			return m, m.autoSyncTickCmd()
		}
		m.Syncing = true
		return m, m.syncAllFromAdoCmd(true)

	case MsgSyncDone:
		m.Loading = false
		m.Syncing = false
		if msg.Err != nil {
			cmds = append(cmds, m.showToast(FormatNetworkOrSyncError(msg.Err, msg.IsAutoSync), true))
		} else {
			curItemID := m.ScreenTree.SelectedItemID()
			curRootID := m.ScreenTree.ActiveRootID()
			curTabIdx := m.ScreenTree.ActiveTab
			collapsedIDs := m.ScreenTree.CollapsedNodeIDs()
			filterOpts := m.ScreenTree.GlobalFilterOpts

			// If current tree was empty (e.g. initial launch), fetch persisted state from DB unless NoResume is set
			if curRootID == 0 && curItemID == 0 && !m.Config.NoResume {
				if uiState, err := m.DB.GetUIState(); err == nil && uiState != nil {
					curRootID = uiState.ActiveTabRootID
					curTabIdx = uiState.ActiveTabIdx
					curItemID = uiState.SelectedItemID
					if len(uiState.CollapsedNodeIDs) > 0 {
						collapsedIDs = uiState.CollapsedNodeIDs
					}
					filterOpts = uiState.Filter
					if len(uiState.TabFilters) > 0 {
						m.ScreenTree.TabFilterOpts = uiState.TabFilters
					}
				}
			}

			m.ScreenTree.SetData(msg.Roots)
			m.ScreenTree.RestoreState(curRootID, curTabIdx, curItemID, collapsedIDs, filterOpts)
			if m.CurrentScreen == ScreenTypeItem && m.ScreenItem.Item != nil {
				if rec, err := m.DB.GetWorkItem(m.ScreenItem.Item.ID); err == nil && rec != nil {
					m.updateScreenItemData(rec)
				}
			}

			if msg.IsAutoSync {
				cmds = append(cmds, m.showToast(fmt.Sprintf("Auto-synced %d work items", msg.Count), false))
			} else {
				cmds = append(cmds, m.showToast(fmt.Sprintf("Synced %d work items", msg.Count), false))
			}
		}

		if m.Config != nil && m.Config.AutoSyncIntervalSeconds > 0 {
			cmds = append(cmds, m.autoSyncTickCmd())
		}
		return m, tea.Batch(cmds...)

	case MsgTrackDone:
		m.Loading = false
		if msg.Err != nil {
			cmds = append(cmds, m.showToast(msg.Err.Error(), true))
		} else {
			toastMsg := fmt.Sprintf("Tracked item #%d and %d descendants", msg.RootID, msg.Count)
			if msg.QueryName != "" {
				toastMsg = fmt.Sprintf("Tracked query %q (%d items)", msg.QueryName, msg.Count)
			}
			cmds = append(cmds,
				m.showToast(toastMsg, false),
				m.reloadTreeCmd(),
			)
		}
		return m, tea.Batch(cmds...)

	case MsgSettingsSaved:
		m.Loading = false
		m.CurrentScreen = ScreenTypeTree
		if msg.Err != nil {
			return m, m.showToast(fmt.Sprintf("Failed to save settings: %v", msg.Err), true)
		}
		cmds = append(cmds,
			m.showToast("Settings saved successfully", false),
			m.validateCredentialsCmd(msg.Pat, msg.GeminiKey, msg.GeminiModel, m.AdoOrg, m.AdoProj),
		)
		if m.Config != nil && m.Config.AutoSyncIntervalSeconds > 0 {
			cmds = append(cmds, m.autoSyncTickCmd())
		}
		return m, tea.Batch(cmds...)

	case MsgCredentialsValidated:
		if m.ScreenSettings != nil {
			if msg.PatChecked {
				if cleanSingleLineInput(m.ScreenSettings.InputPat.Value()) == "" && !msg.PatValid && msg.PatError == "" {
					m.ScreenSettings.PatStatus = ValidationStatusNotConfigured
					m.ScreenSettings.PatError = ""
				} else if msg.PatValid {
					m.ScreenSettings.PatStatus = ValidationStatusValid
					m.ScreenSettings.PatError = ""
				} else {
					m.ScreenSettings.PatStatus = ValidationStatusInvalid
					m.ScreenSettings.PatError = msg.PatError
				}
			}
			if msg.GeminiChecked {
				if cleanSingleLineInput(m.ScreenSettings.InputGeminiKey.Value()) == "" && !msg.GeminiValid && msg.GeminiError == "" {
					m.ScreenSettings.GeminiStatus = ValidationStatusNotConfigured
					m.ScreenSettings.GeminiError = ""
				} else if msg.GeminiValid {
					m.ScreenSettings.GeminiStatus = ValidationStatusValid
					m.ScreenSettings.GeminiError = ""
				} else {
					m.ScreenSettings.GeminiStatus = ValidationStatusInvalid
					m.ScreenSettings.GeminiError = msg.GeminiError
				}
			}
		}
		return m, nil

	case MsgItemUpdated:
		m.Loading = false
		m.Syncing = false
		if msg.Err != nil {
			cmds = append(cmds, m.showToast(FormatNetworkOrSyncError(msg.Err, false), true))
		} else {
			cmds = append(cmds,
				m.showToast(fmt.Sprintf("Updated item #%d", msg.Item.ID), false),
				m.reloadTreeCmd(),
			)
			if m.CurrentScreen == ScreenTypeItem {
				m.updateScreenItemData(msg.Item)
			}
			if msg.Item != nil && m.DB != nil {
				if _, isCached := m.DB.ResolveItemFieldPermissions(msg.Item.Org, msg.Item.Project, msg.Item.WorkItemType, msg.Item.State); !isCached {
					cmds = append(cmds, m.ensureWorkItemTypeMetadataCmd(msg.Item.Org, msg.Item.Project, msg.Item.WorkItemType))
				}
			}
		}
		return m, tea.Batch(cmds...)

	case MsgItemCreated:
		m.Loading = false
		if msg.Err != nil {
			cmds = append(cmds, m.showToast(msg.Err.Error(), true))
		} else {
			cmds = append(cmds,
				m.showToast(fmt.Sprintf("Created item #%d: %s", msg.Item.ID, msg.Item.Title), false),
				m.reloadTreeCmd(),
			)
			// Switch to Screen 2 to see the newly created item
			m.updateScreenItemData(msg.Item)
			m.CurrentScreen = ScreenTypeItem
			if msg.Item != nil && m.DB != nil {
				if _, isCached := m.DB.ResolveItemFieldPermissions(msg.Item.Org, msg.Item.Project, msg.Item.WorkItemType, msg.Item.State); !isCached {
					cmds = append(cmds, m.ensureWorkItemTypeMetadataCmd(msg.Item.Org, msg.Item.Project, msg.Item.WorkItemType))
				}
			}
		}
		return m, tea.Batch(cmds...)

	case MsgTypeMetadataLoaded:
		if msg.Org != "" && msg.Project != "" && msg.WorkItemType != "" {
			if m.CurrentScreen == ScreenTypeItem && m.ScreenItem.Item != nil {
				if strings.EqualFold(m.ScreenItem.Item.Org, msg.Org) &&
					strings.EqualFold(m.ScreenItem.Item.Project, msg.Project) &&
					strings.EqualFold(m.ScreenItem.Item.WorkItemType, msg.WorkItemType) {
					m.updateScreenItemData(m.ScreenItem.Item)
				}
			}
			if m.ActiveModal == ModalTypeEstimate && m.ModalEstimate.Active {
				if strings.EqualFold(m.ModalEstimate.Org, msg.Org) &&
					strings.EqualFold(m.ModalEstimate.Project, msg.Project) &&
					strings.EqualFold(m.ModalEstimate.ItemType, msg.WorkItemType) {
					m.ModalEstimate.OpenWithDB(
						m.ModalEstimate.WorkItemID,
						m.ModalEstimate.ItemTitle,
						m.ModalEstimate.ItemType,
						m.ModalEstimate.ItemState,
						m.ModalEstimate.Org,
						m.ModalEstimate.Project,
						m.ModalEstimate.InitialOriginal,
						m.ModalEstimate.InitialRemaining,
						m.ModalEstimate.InitialCompleted,
						m.ModalEstimate.InitialEffort,
						m.DB,
					)
				}
			}
		}
		return m, nil

	case MsgItemDeleted:
		m.Loading = false
		if msg.Err != nil {
			cmds = append(cmds, m.showToast(msg.Err.Error(), true))
		} else {
			cmds = append(cmds,
				m.showToast(fmt.Sprintf("Deleted item #%d", msg.ID), false),
				m.reloadTreeCmd(),
			)
			if m.CurrentScreen == ScreenTypeItem {
				m.CurrentScreen = ScreenTypeTree
			}
		}
		return m, tea.Batch(cmds...)

	case MsgCommentsLoaded:
		if msg.Err == nil && m.ScreenItem.Item != nil {
			m.updateScreenItemData(m.ScreenItem.Item)
		}
		return m, nil

	case MsgCommentAdded:
		m.Loading = false
		if msg.Err != nil {
			cmds = append(cmds, m.showToast(msg.Err.Error(), true))
		} else {
			cmds = append(cmds,
				m.showToast("Comment added", false),
				m.loadCommentsCmd(msg.Comment.WorkItemID),
			)
		}
		return m, tea.Batch(cmds...)

	case MarqueeTickMsg:
		if m.ActiveModal == ModalTypeFilter {
			m.ModalFilter, _, _ = m.ModalFilter.Update(msg)
		} else if m.ActiveModal == ModalTypeNone && m.CurrentScreen == ScreenTypeTree {
			m.ScreenTree.MarqueeTick++
		}
		if resizeCmd := m.checkTerminalResize(); resizeCmd != nil {
			return m, tea.Batch(m.marqueeTickCmd(), resizeCmd)
		}
		return m, m.marqueeTickCmd()

	case tea.MouseMsg:
		isWheelUp := msg.Button == tea.MouseButtonWheelUp || msg.Type == tea.MouseWheelUp
		isWheelDown := msg.Button == tea.MouseButtonWheelDown || msg.Type == tea.MouseWheelDown
		isClick := msg.Action == tea.MouseActionPress || msg.Type == tea.MouseLeft

		dockVisible := m.DockGemini != nil && m.DockGemini.Visible
		dockStartY := m.Height
		if dockVisible {
			dockHeight := lipgloss.Height(m.DockGemini.View())
			dockStartY = m.Height - dockHeight
		}

		// 1. Mouse event inside Dock Gemini
		if dockVisible && msg.Y >= dockStartY {
			if isWheelUp {
				m.DockGemini.ScrollUp(2)
				return m, nil
			} else if isWheelDown {
				m.DockGemini.ScrollDown(2)
				return m, nil
			} else if msg.Ctrl && isClick {
				relY := msg.Y - dockStartY
				if targetURL := m.DockGemini.GetURLAt(relY, msg.X); targetURL != "" {
					m.openURL(targetURL)
					return m, m.showToast(fmt.Sprintf("Opened %s in browser", targetURL), false)
				}
			}
			return m, nil
		}

		// 2. Mouse event inside Main Screen or Modal
		if m.ActiveModal == ModalTypeTab {
			if isWheelUp {
				m.ModalTab.CursorUp()
				return m, nil
			} else if isWheelDown {
				m.ModalTab.CursorDown()
				return m, nil
			}
		}

		if m.ActiveModal == ModalTypeNone {
			switch m.CurrentScreen {
			case ScreenTypeTree:
				m.ScreenTree.Height = m.availableScreenHeight()
				if isWheelUp {
					m.ScreenTree.CursorUp()
					return m, nil
				} else if isWheelDown {
					m.ScreenTree.CursorDown()
					return m, nil
				} else if isClick {
					headerHeight := 4
					tabBarHeight := 0
					if len(m.ScreenTree.TreeRoots) > 0 {
						tabBarHeight = 1
					}
					visibleHeight := m.ScreenTree.Height - headerHeight - 1 - tabBarHeight
					if msg.Y >= headerHeight && msg.Y < headerHeight+visibleHeight {
						rowIdx := (msg.Y - headerHeight) + m.ScreenTree.ScrollOffset
						if rowIdx >= 0 && rowIdx < len(m.ScreenTree.Flattened) {
							m.ScreenTree.Cursor = rowIdx
							m.ScreenTree.MarqueeTick = 0
							m.ScreenTree.adjustScroll()
						}
					}
					if msg.Ctrl {
						if sel := m.ScreenTree.SelectedItem(); sel != nil {
							m.openInBrowser(sel.ID)
							return m, m.showToast(fmt.Sprintf("Opened #%d in browser", sel.ID), false)
						}
					}
					return m, nil
				}
			case ScreenTypeItem:
				if isWheelUp {
					m.ScreenItem.Viewport.LineUp(3)
					return m, nil
				} else if isWheelDown {
					m.ScreenItem.Viewport.LineDown(3)
					return m, nil
				} else if msg.Ctrl && isClick {
					if sel := m.ScreenItem.SelectedLink(); sel != nil && sel.URL != "" {
						m.openURL(sel.URL)
						return m, m.showToast(fmt.Sprintf("Opened %s in browser", sel.Title), false)
					} else if m.ScreenItem.Item != nil {
						m.openInBrowser(m.ScreenItem.Item.ID)
						return m, m.showToast(fmt.Sprintf("Opened #%d in browser", m.ScreenItem.Item.ID), false)
					}
				}
			case ScreenTypeCreate:
				if isWheelUp {
					m.ScreenCreate.Viewport.LineUp(3)
					return m, nil
				} else if isWheelDown {
					m.ScreenCreate.Viewport.LineDown(3)
					return m, nil
				} else if isClick {
					headerHeight := 2
					contentY := (msg.Y - headerHeight) + m.ScreenCreate.Viewport.YOffset
					if contentY >= 24 || msg.Y >= m.Height-4 {
						m.ScreenCreate.FocusedField = fieldCreateSubmit
						m.ScreenCreate.updateFocus()
						title := m.ScreenCreate.Title()
						if title == "" {
							return m, m.showToast("Title is required to create a work item", true)
						}
						itemType := m.ScreenCreate.SelectedType()
						desc := m.ScreenCreate.Description()
						assigned := m.ScreenCreate.AssignedTo()
						tags := m.ScreenCreate.Tags()
						iteration := m.ScreenCreate.Iteration()
						area := m.ScreenCreate.Area()
						parentID := m.ScreenCreate.ParentID()

						m.Loading = true
						return m, tea.Batch(
							m.showToast(fmt.Sprintf("Creating %s '%s'...", itemType, title), false),
							m.createItemCmd(itemType, title, desc, assigned, tags, iteration, area, 0, 0, parentID, "", "", ""),
						)
					}
					var cmd tea.Cmd
					m.ScreenCreate, cmd = m.ScreenCreate.Update(msg)
					return m, cmd
				}
			case ScreenTypeSettings:
				if isWheelUp {
					m.ScreenSettings.Viewport.LineUp(3)
					return m, nil
				} else if isWheelDown {
					m.ScreenSettings.Viewport.LineDown(3)
					return m, nil
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		keyStr := msg.String()
		// Filter out leaked mouse tracking escape sequences (e.g. [<64;79;20M, <0;..., etc.)
		if strings.HasPrefix(keyStr, "[<") || strings.HasPrefix(keyStr, "<") || (strings.Contains(keyStr, ";") && strings.HasSuffix(keyStr, "M")) {
			return m, nil
		}

		// Handle active modal input first
		if m.ActiveModal != ModalTypeNone {
			switch m.ActiveModal {
			case ModalTypeStatus:
				if msg.String() == "esc" {
					m.ModalStatus.Close()
					m.ActiveModal = ModalTypeNone
					return m, nil
				}
				if msg.String() == "enter" {
					selected := m.ModalStatus.SelectedState()
					id := m.ModalStatus.WorkItemID
					m.ModalStatus.Close()
					m.ActiveModal = ModalTypeNone
					m.Loading = true
					return m, tea.Batch(m.showToast("Updating status...", false), m.updateStatusCmd(id, selected))
				}
				var cmd tea.Cmd
				m.ModalStatus, cmd = m.ModalStatus.Update(msg)
				return m, cmd

			case ModalTypeAssign:
				if msg.String() == "esc" {
					m.ModalAssign.Close()
					m.ActiveModal = ModalTypeNone
					return m, nil
				}
				if msg.String() == "enter" {
					assignee := m.ModalAssign.Value()
					if m.ModalAssign.Autocomplete.Active {
						if sel := m.ModalAssign.Autocomplete.Selected(); sel != nil {
							assignee = sel.InsertValue
						}
					}
					id := m.ModalAssign.WorkItemID
					m.ModalAssign.Close()
					m.ActiveModal = ModalTypeNone
					m.Loading = true
					return m, tea.Batch(m.showToast("Assigning item...", false), m.updateAssigneeCmd(id, assignee))
				}
				var cmd tea.Cmd
				m.ModalAssign, cmd = m.ModalAssign.Update(msg)
				return m, cmd

			case ModalTypeEstimate:
				if msg.String() == "esc" {
					m.ModalEstimate.Close()
					m.ActiveModal = ModalTypeNone
					return m, nil
				}
				if msg.String() == "enter" {
					id := m.ModalEstimate.WorkItemID
					fields := m.ModalEstimate.FieldsToUpdate()
					m.ModalEstimate.Close()
					m.ActiveModal = ModalTypeNone
					m.Loading = true
					return m, tea.Batch(m.showToast("Updating estimates & effort...", false), m.updateEstimatesFieldsCmd(id, fields))
				}
				var cmd tea.Cmd
				m.ModalEstimate, cmd = m.ModalEstimate.Update(msg)
				return m, cmd

			case ModalTypeComment:
				if msg.String() == "esc" {
					m.ModalComment.Close()
					m.ActiveModal = ModalTypeNone
					return m, nil
				}
				if msg.String() == "ctrl+s" {
					text := m.ModalComment.Value()
					id := m.ModalComment.WorkItemID
					m.ModalComment.Close()
					m.ActiveModal = ModalTypeNone
					if text != "" {
						m.Loading = true
						return m, tea.Batch(m.showToast("Posting comment...", false), m.addCommentCmd(id, text))
					}
					return m, nil
				}
				var cmd tea.Cmd
				m.ModalComment, cmd = m.ModalComment.Update(msg)
				return m, cmd

			case ModalTypeTrack:
				if !m.ModalTrack.InputMode {
					// List mode
					switch msg.String() {
					case "esc":
						m.ModalTrack.Close()
						m.ActiveModal = ModalTypeNone
						return m, nil

					case "enter":
						if sel := m.ModalTrack.SelectedItem(); sel != nil {
							m.ModalTrack.Close()
							m.ActiveModal = ModalTypeNone
							m.ScreenTree.SelectItemID(sel.ID)
							return m, nil
						}
						m.ModalTrack.Close()
						m.ActiveModal = ModalTypeNone
						return m, nil

					case "d", "x", "delete", "backspace":
						if q := m.ModalTrack.SelectedQuery(); q != nil {
							queryID := q.ID
							queryName := q.Name
							_ = m.DB.DeleteTrackedQuery(queryID)
							m.ModalTrack.RefreshList()
							if m.ModalTrack.TotalCount() == 0 {
								m.ModalTrack.InputMode = true
								m.ModalTrack.Input.Reset()
								m.ModalTrack.Input.Focus()
							}
							return m, tea.Batch(
								m.showToast(fmt.Sprintf("Untracked query %q", queryName), false),
								m.reloadTreeCmd(),
							)
						} else if sel := m.ModalTrack.SelectedItem(); sel != nil {
							untrackedID := sel.ID
							_ = m.DB.DeleteTrackedRoot(untrackedID)
							m.ModalTrack.RefreshList()
							if m.ModalTrack.TotalCount() == 0 {
								m.ModalTrack.InputMode = true
								m.ModalTrack.Input.Reset()
								m.ModalTrack.Input.Focus()
							}
							return m, tea.Batch(
								m.showToast(fmt.Sprintf("Untracked #%d", untrackedID), false),
								m.reloadTreeCmd(),
							)
						}
						return m, nil
					}
				} else {
					// Input mode
					if msg.String() == "enter" {
						val := strings.TrimSpace(m.ModalTrack.Value())
						if val != "" {
							m.ModalTrack.Close()
							m.ActiveModal = ModalTypeNone
							m.Loading = true
							return m, tea.Batch(m.showToast(fmt.Sprintf("Fetching %s from ADO...", val), false), m.trackCmd(val))
						}
						return m, nil
					}
					if msg.String() == "esc" {
						if m.ModalTrack.TotalCount() > 0 {
							m.ModalTrack.InputMode = false
							m.ModalTrack.Input.Blur()
							return m, nil
						}
						m.ModalTrack.Close()
						m.ActiveModal = ModalTypeNone
						return m, nil
					}
				}
				var cmd tea.Cmd
				m.ModalTrack, cmd = m.ModalTrack.Update(msg)
				return m, cmd

			case ModalTypeConfirm:
				var cmd tea.Cmd
				m.ModalConfirm, cmd = m.ModalConfirm.Update(msg)
				if !m.ModalConfirm.Active {
					if m.ModalConfirm.ActionTag == "gemini_confirm" {
						if m.PendingGeminiConfirmChan != nil {
							m.PendingGeminiConfirmChan <- m.ModalConfirm.Confirmed
							m.PendingGeminiConfirmChan = nil
						}
						m.ActiveModal = ModalTypeNone
						return m, cmd
					}
					if m.ModalConfirm.Confirmed && m.ModalConfirm.ActionTag == "delete" {
						id := m.ModalConfirm.TargetID
						m.ActiveModal = ModalTypeNone
						m.Loading = true
						return m, tea.Batch(m.showToast(fmt.Sprintf("Deleting item #%d...", id), false), m.deleteItemCmd(id))
					}
					m.ActiveModal = ModalTypeNone
				}
				return m, cmd

			case ModalTypeEdit:
				if msg.String() == "esc" {
					if m.ModalEdit.Picker != nil && m.ModalEdit.Picker.Active {
						m.ModalEdit.Picker.Close()
						return m, nil
					}
					m.ModalEdit.Close()
					m.ActiveModal = ModalTypeNone
					return m, nil
				}
				if msg.String() == "ctrl+s" {
					title, desc, tags, iter, area := m.ModalEdit.Values()
					id := m.ModalEdit.WorkItemID
					m.ModalEdit.Close()
					m.ActiveModal = ModalTypeNone
					m.Loading = true
					return m, tea.Batch(m.showToast("Saving details...", false), m.updateDetailsCmd(id, title, desc, tags, iter, area))
				}
				var cmd tea.Cmd
				m.ModalEdit, cmd = m.ModalEdit.Update(msg)
				return m, cmd

			case ModalTypeFilter:
				var action ModalFilterAction
				var cmd tea.Cmd
				m.ModalFilter, cmd, action = m.ModalFilter.Update(msg)
				switch action {
				case FilterActionApplyGlobal:
					opts := m.ModalFilter.GetFilterOptions()
					m.ScreenTree.SetGlobalFilter(opts)
					m.ActiveModal = ModalTypeNone
					summary := opts.SummaryString()
					if summary == "" {
						summary = "All items shown"
					}
					return m, m.showToast("🌐 Global filter applied: "+summary, false)

				case FilterActionApplyTab:
					opts := m.ModalFilter.GetFilterOptions()
					activeTab := m.ScreenTree.ActiveTab
					m.ScreenTree.SetTabFilter(activeTab, opts)
					m.ActiveModal = ModalTypeNone
					summary := opts.SummaryString()
					if summary == "" {
						summary = "All items shown"
					}
					return m, m.showToast(fmt.Sprintf("📑 Tab %d filter applied: %s", activeTab+1, summary), false)

				case FilterActionClearTab:
					activeTab := m.ScreenTree.ActiveTab
					m.ScreenTree.ClearTabFilter(activeTab)
					m.ActiveModal = ModalTypeNone
					tabTitle := fmt.Sprintf("Tab %d", activeTab+1)
					if activeTab >= 0 && activeTab < len(m.ScreenTree.TreeRoots) {
						r := m.ScreenTree.TreeRoots[activeTab]
						if r != nil && r.Record != nil && r.Record.Title != "" {
							tabTitle = fmt.Sprintf("Tab %d (%s)", activeTab+1, truncateString(r.Record.Title, 20))
						}
					}
					return m, m.showToast(fmt.Sprintf("📑 %s filter cleared (global filter retained)", tabTitle), false)

				case FilterActionClearAll:
					m.ScreenTree.ClearAllFilters()
					m.ActiveModal = ModalTypeNone
					return m, m.showToast("All filters cleared", false)

				case FilterActionCancel:
					m.ActiveModal = ModalTypeNone
					return m, nil
				}
				return m, cmd

			case ModalTypeSchedule:
				switch msg.String() {
				case "esc":
					m.ModalSchedule.Close()
					m.ActiveModal = ModalTypeNone
					return m, nil
				case "enter":
					if m.ModalSchedule.Focus == ScheduleFocusButtons && m.ModalSchedule.ButtonIndex == 1 {
						m.ModalSchedule.Close()
						m.ActiveModal = ModalTypeNone
						return m, nil
					}
					start, target, finish := m.ModalSchedule.Values()
					id := m.ModalSchedule.WorkItemID
					m.ModalSchedule.Close()
					m.ActiveModal = ModalTypeNone
					m.Loading = true
					return m, tea.Batch(
						m.showToast(fmt.Sprintf("Updating schedule for #%d...", id), false),
						m.updateScheduleCmd(id, start, target, finish),
					)
				}
				var cmd tea.Cmd
				m.ModalSchedule, cmd = m.ModalSchedule.Update(msg)
				return m, cmd

			case ModalTypeTab:
				var action ModalTabAction
				var cmd tea.Cmd
				m.ModalTab, cmd, action = m.ModalTab.Update(msg)
				switch action {
				case TabActionSelect:
					sel := m.ModalTab.SelectedTab()
					m.ModalTab.Close()
					m.ActiveModal = ModalTypeNone
					if sel != nil {
						m.ScreenTree.SetActiveTab(sel.Index)
						return m, m.showToast(fmt.Sprintf("Switched to Tab %d: %s", sel.Index+1, sel.Title), false)
					}
					return m, nil

				case TabActionRename:
					sel := m.ModalTab.SelectedTab()
					newName := strings.TrimSpace(m.ModalTab.RenameInput.Value())
					if sel != nil && newName != "" {
						m.renameTab(sel.Index, sel.RootID, newName)
						return m, m.showToast(fmt.Sprintf("Renamed tab to %q", newName), false)
					}
					return m, nil

				case TabActionClose:
					m.ModalTab.Close()
					m.ActiveModal = ModalTypeNone
					return m, nil
				}
				return m, cmd

			case ModalTypeActivity:
				if msg.String() == "esc" {
					m.ModalActivity.Close()
					m.ActiveModal = ModalTypeNone
					return m, nil
				}
				if msg.String() == "enter" {
					selected := m.ModalActivity.SelectedActivity()
					id := m.ModalActivity.WorkItemID
					m.ModalActivity.Close()
					m.ActiveModal = ModalTypeNone
					m.Loading = true
					return m, tea.Batch(m.showToast("Updating activity...", false), m.updateActivityCmd(id, selected))
				}
				var cmd tea.Cmd
				m.ModalActivity, cmd = m.ModalActivity.Update(msg)
				return m, cmd
			}
			return m, nil
		}

		// Global shortcuts (when no modal is open)
		if msg.String() == "ctrl+c" {
			if m.GeminiCancelFunc != nil || (m.DockGemini != nil && m.DockGemini.Loading) {
				m.cancelGemini()
				return m, m.showToast("AI operation cancelled", false)
			}
			if m.DockGemini != nil && m.DockGemini.Visible {
				m.DockGemini.Hide()
				return m, nil
			}
			return m, m.showToast("Press 'q' from Home screen to quit", false)
		}

		// Handle Gemini prompt dock input when open
		if m.DockGemini != nil && m.DockGemini.Visible {
			availH := m.availableScreenHeight()
			m.ScreenTree.Height = availH
			m.ScreenItem.Height = availH
			switch msg.String() {
			case "esc":
				if m.DockGemini.Loading {
					m.cancelGemini()
					return m, m.showToast("AI operation cancelled", false)
				}
				m.DockGemini.Hide()
				m.ScreenTree.Height = m.Height
				m.ScreenTree.adjustScroll()
				m.ScreenItem.Height = m.Height
				m.ScreenItem.SetSize(m.Width, m.Height)
				return m, nil
			case "delete", "del", "ctrl+delete", "ctrl+del", "shift+delete", "ctrl+l":
				if m.DockGemini.Input.Value() == "" || msg.String() == "ctrl+delete" || msg.String() == "ctrl+del" || msg.String() == "ctrl+l" {
					m.DockGemini.Clear()
					return m, m.showToast("Gemini history cleared", false)
				}
			case "ctrl+up":
				m.DockGemini.IncreaseHeight(2)
				newAvailH := m.availableScreenHeight()
				m.ScreenTree.Height = newAvailH
				m.ScreenTree.adjustScroll()
				m.ScreenItem.Height = newAvailH
				m.ScreenItem.SetSize(m.Width, newAvailH)
				return m, nil
			case "ctrl+down":
				m.DockGemini.DecreaseHeight(2)
				newAvailH := m.availableScreenHeight()
				m.ScreenTree.Height = newAvailH
				m.ScreenTree.adjustScroll()
				m.ScreenItem.Height = newAvailH
				m.ScreenItem.SetSize(m.Width, newAvailH)
				return m, nil
			case "shift+up":
				m.ScreenTree.CursorUp()
				return m, nil
			case "shift+down":
				m.ScreenTree.CursorDown()
				return m, nil
			case "alt+[", "alt+{", "alt+shift+tab":
				m.ScreenTree.PrevTab()
				return m, nil
			case "alt+]", "alt+}":
				m.ScreenTree.NextTab()
				return m, nil
			case "alt+space", "alt+ ", "shift+alt+space", "shift+alt+ ":
				m.ScreenTree.ToggleExpand()
				return m, nil
			case "ctrl+v":
				clipText, err := clipboard.Read()
				if err == nil && clipText != "" {
					m.DockGemini.InsertText(clipText)
					availH := m.availableScreenHeight()
					m.ScreenTree.Height = availH
					m.ScreenTree.adjustScroll()
					m.ScreenItem.Height = availH
					m.ScreenItem.SetSize(m.Width, availH)
					return m, nil
				}
				return m, nil
			case "shift+enter", "alt+enter", "alt+ctrl+j", "ctrl+enter", "ctrl+j", "\n", "\r\n":
				m.DockGemini.Input.InsertString("\n")
				m.DockGemini.updateInputHeight()
				availH := m.availableScreenHeight()
				m.ScreenTree.Height = availH
				m.ScreenTree.adjustScroll()
				m.ScreenItem.Height = availH
				m.ScreenItem.SetSize(m.Width, availH)
				return m, nil
			case "enter":
				query := strings.TrimSpace(m.DockGemini.Input.Value())
				if query != "" {
					m.DockGemini.History = append(m.DockGemini.History, query)
					m.DockGemini.HistoryIdx = -1
					m.DockGemini.Loading = true
					m.DockGemini.StatusMsg = "Analyzing context & executing actions..."
					m.DockGemini.ErrorMsg = ""
					m.DockGemini.ResponseMsg = ""
					m.DockGemini.ScrollOffset = 0
					m.DockGemini.Input.Reset()
					m.DockGemini.updateInputHeight()
					return m, tea.Batch(
						m.askGeminiCmd(query),
						m.DockGemini.Spinner.Tick,
					)
				}
				return m, nil
			}
			var cmd tea.Cmd
			m.DockGemini, cmd = m.DockGemini.Update(msg)
			return m, cmd
		}

		// Handle active screen
		switch m.CurrentScreen {
		case ScreenTypeTree:
			if !m.ScreenTree.Filtering {
				switch msg.String() {
				case " ", "space":
					m.ScreenTree.ToggleExpand()
					return m, nil
				case "[", "{", "shift+tab":
					m.ScreenTree.PrevTab()
					return m, nil
				case "]", "}", "tab":
					m.ScreenTree.NextTab()
					return m, nil
				case "1", "2", "3", "4", "5", "6", "7", "8", "9":
					idx := int(msg.String()[0] - '1')
					m.ScreenTree.SetActiveTab(idx)
					return m, nil
				case "q":
					m.saveUIState()
					return m, tea.Quit
				case "esc":
					if m.GeminiCancelFunc != nil || (m.DockGemini != nil && m.DockGemini.Loading) {
						m.cancelGemini()
						return m, m.showToast("AI operation cancelled", false)
					}
					return m, nil
				case "f":
					activeTabIdx := m.ScreenTree.ActiveTab
					tabTitle := ""
					if activeTabIdx >= 0 && activeTabIdx < len(m.ScreenTree.TreeRoots) {
						root := m.ScreenTree.TreeRoots[activeTabIdx]
						if root != nil && root.Record != nil {
							tabTitle = root.Record.Title
						}
					}
					tabFilter := m.ScreenTree.GetTabFilter(activeTabIdx)
					m.ModalFilter.OpenWithTab(m.ScreenTree.GlobalFilterOpts, tabFilter, tabTitle, m.DB)
					m.ActiveModal = ModalTypeFilter
					return m, m.ModalFilter.TickCmd()
				case "F":
					m.ScreenTree.ClearAllFilters()
					return m, m.showToast("Filters cleared", false)
				case "g":
					if m.DockGemini != nil {
						m.DockGemini.Show()
						m.DockGemini.SetSize(m.Width, m.Height)
					}
					return m, nil
				case "enter":
					item := m.ScreenTree.SelectedItem()
					if item != nil {
						if item.WorkItemType == "Query" || item.ID == 0 {
							m.ScreenTree.ToggleExpand()
							return m, nil
						}
						m.updateScreenItemData(item)
						m.CurrentScreen = ScreenTypeItem
						return m, m.loadCommentsCmd(item.ID)
					}
				case "o":
					if item := m.ScreenTree.SelectedItem(); item != nil {
						if item.WorkItemType == "Query" || item.ID == 0 {
							if item.URL != "" {
								m.openURL(item.URL)
								return m, m.showToast("Opening query in browser...", false)
							}
							return m, nil
						}
						m.openInBrowser(item.ID)
						return m, m.showToast(fmt.Sprintf("Opening #%d in browser...", item.ID), false)
					}
				case "C", "shift+c":
					if item := m.ScreenTree.SelectedItem(); item != nil {
						plainText, err := m.copyWorkItemToClipboard(item.ID)
						if err != nil {
							return m, m.showToast(fmt.Sprintf("Failed to copy: %v", err), true)
						}
						return m, m.showToast(fmt.Sprintf("Copied \"%s\" to clipboard", plainText), false)
					}
				case "I", "shift+i":
					m.ScreenTree.IncompleteView = !m.ScreenTree.IncompleteView
					m.ScreenItem.IncompleteView = m.ScreenTree.IncompleteView
					m.ScreenTree.ClearIncompleteCache()
					if m.ScreenTree.IncompleteView {
						return m, m.showToast("Incomplete View enabled", false)
					}
					return m, m.showToast("Incomplete View disabled", false)
				case "c":
					var parentItem *db.WorkItemRecord
					if item := m.ScreenTree.SelectedItem(); item != nil {
						parentItem = item
					}
					m.ScreenCreate.Reset(parentItem, "", m.DB)
					m.ScreenCreate.InitViewport(m.Width, m.availableScreenHeight())
					m.CurrentScreen = ScreenTypeCreate
					return m, nil
				case "t":
					m.ModalTrack.Open(m.DB)
					m.ActiveModal = ModalTypeTrack
					return m, nil
				case "T", "shift+t":
					if len(m.ScreenTree.TreeRoots) > 0 {
						m.ModalTab.Open(m.ScreenTree.TreeRoots, m.ScreenTree.ActiveTab)
						m.ActiveModal = ModalTypeTab
						return m, nil
					}
					return m, m.showToast("No tracked root tabs available", false)
				case "s":
					if item := m.ScreenTree.SelectedItem(); item != nil {
						availableStates := m.getStatesForItemType(item.WorkItemType)
						m.ModalStatus.Open(item.ID, item.Title, item.WorkItemType, item.State, availableStates)
						m.ActiveModal = ModalTypeStatus
						return m, nil
					}
				case "v", "V", "A", "shift+a":
					if item := m.ScreenTree.SelectedItem(); item != nil {
						var dbActs []string
						if m.DB != nil {
							dbActs, _ = m.DB.GetDistinctActivities()
						}
						m.ModalActivity.Open(item.ID, item.Title, item.WorkItemType, item.Activity, dbActs)
						m.ActiveModal = ModalTypeActivity
						return m, nil
					}
				case "S", "shift+s":
					m.ScreenSettings.Reset(m.Config, m.DB)
					m.CurrentScreen = ScreenTypeSettings
					return m, nil
				case "d":
					if item := m.ScreenTree.SelectedItem(); item != nil {
						m.ModalSchedule.Open(item.ID, item.Title, item.WorkItemType, item.StartDate, item.TargetDate, item.FinishDate)
						m.ActiveModal = ModalTypeSchedule
						return m, nil
					}
				case "a":
					if item := m.ScreenTree.SelectedItem(); item != nil {
						m.ModalAssign.Open(item.ID, item.Title, item.AssignedToEmail, m.DB)
						m.ActiveModal = ModalTypeAssign
						return m, nil
					}
				case "e", "w":
					if item := m.ScreenTree.SelectedItem(); item != nil {
						m.ModalEstimate.OpenWithDB(item.ID, item.Title, item.WorkItemType, item.State, item.Org, item.Project, item.OriginalEstimate, item.RemainingWork, item.CompletedWork, item.Effort, m.DB)
						m.ActiveModal = ModalTypeEstimate
						var cmd tea.Cmd
						if m.DB != nil {
							if _, isCached := m.DB.ResolveItemFieldPermissions(item.Org, item.Project, item.WorkItemType, item.State); !isCached {
								cmd = m.ensureWorkItemTypeMetadataCmd(item.Org, item.Project, item.WorkItemType)
							}
						}
						return m, cmd
					}
				case "D", "shift+d":
					if item := m.ScreenTree.SelectedItem(); item != nil {
						m.ModalConfirm.Open("Delete Work Item", fmt.Sprintf("Are you sure you want to delete #%d (%s)?", item.ID, item.Title), "delete", item.ID, true)
						m.ActiveModal = ModalTypeConfirm
						return m, nil
					}
				case "r":
					if m.Syncing {
						return m, m.showToast("Sync already in progress...", false)
					}
					m.Loading = true
					m.Syncing = true
					return m, tea.Batch(m.showToast("Syncing all items from ADO...", false), m.syncAllFromAdoCmd(false))
				}
			}
			var cmd tea.Cmd
			m.ScreenTree, cmd = m.ScreenTree.Update(msg)
			return m, cmd

		case ScreenTypeItem:
			switch msg.String() {
			case "esc", "b", "backspace":
				if m.GeminiCancelFunc != nil || (m.DockGemini != nil && m.DockGemini.Loading) {
					m.cancelGemini()
					return m, m.showToast("AI operation cancelled", false)
				}
				m.CurrentScreen = ScreenTypeTree
				return m, m.reloadTreeCmd()
			case "[", "shift+tab":
				m.ScreenItem.PrevLink()
				return m, nil
			case "]", "tab":
				m.ScreenItem.NextLink()
				return m, nil
			case "enter", "ctrl+enter", "ctrl+m", "ctrl+j":
				if sel := m.ScreenItem.SelectedLink(); sel != nil {
					if sel.IsPR && sel.URL != "" {
						m.openURL(sel.URL)
						return m, m.showToast(fmt.Sprintf("Opened %s in browser", sel.Title), false)
					}
					if !sel.IsPR {
						if targetItem, err := m.DB.GetWorkItem(sel.ID); err == nil && targetItem != nil {
							m.updateScreenItemData(targetItem)
							return m, m.loadCommentsCmd(targetItem.ID)
						} else if sel.URL != "" {
							m.openURL(sel.URL)
							return m, m.showToast(fmt.Sprintf("Opened %s in browser", sel.Title), false)
						}
					}
					if sel.URL != "" {
						m.openURL(sel.URL)
						return m, m.showToast(fmt.Sprintf("Opened %s in browser", sel.Title), false)
					}
				} else if len(m.ScreenItem.RelatedLinks) > 0 {
					firstLink := m.ScreenItem.RelatedLinks[0]
					if firstLink.URL != "" {
						m.openURL(firstLink.URL)
						return m, m.showToast(fmt.Sprintf("Opened %s in browser", firstLink.Title), false)
					}
				} else if m.ScreenItem.Item != nil {
					m.openInBrowser(m.ScreenItem.Item.ID)
					return m, m.showToast(fmt.Sprintf("Opened #%d in browser", m.ScreenItem.Item.ID), false)
				}
			case "g":
				if m.DockGemini != nil {
					m.DockGemini.Show()
					m.DockGemini.SetSize(m.Width, m.Height)
				}
				return m, nil
			case "e":
				if m.ScreenItem.Item != nil {
					m.ModalEdit.Open(
						m.ScreenItem.Item.ID,
						m.ScreenItem.Item.WorkItemType,
						m.ScreenItem.Item.Title,
						m.ScreenItem.Item.Description,
						m.ScreenItem.Item.Tags,
						m.ScreenItem.Item.IterationPath,
						m.ScreenItem.Item.AreaPath,
						m.DB,
					)
					m.ActiveModal = ModalTypeEdit
					return m, nil
				}
			case "c":
				if m.ScreenItem.Item != nil {
					m.ModalComment.Open(m.ScreenItem.Item.ID, m.ScreenItem.Item.Title, m.DB)
					m.ActiveModal = ModalTypeComment
					return m, nil
				}
			case "C", "shift+c":
				if m.ScreenItem.Item != nil {
					plainText, err := m.copyWorkItemToClipboard(m.ScreenItem.Item.ID)
					if err != nil {
						return m, m.showToast(fmt.Sprintf("Failed to copy: %v", err), true)
					}
					return m, m.showToast(fmt.Sprintf("Copied \"%s\" to clipboard", plainText), false)
				}
			case "w":
				if m.ScreenItem.Item != nil {
					m.ModalEstimate.OpenWithDB(m.ScreenItem.Item.ID, m.ScreenItem.Item.Title, m.ScreenItem.Item.WorkItemType, m.ScreenItem.Item.State, m.ScreenItem.Item.Org, m.ScreenItem.Item.Project, m.ScreenItem.Item.OriginalEstimate, m.ScreenItem.Item.RemainingWork, m.ScreenItem.Item.CompletedWork, m.ScreenItem.Item.Effort, m.DB)
					m.ActiveModal = ModalTypeEstimate
					var cmd tea.Cmd
					if m.DB != nil {
						if _, isCached := m.DB.ResolveItemFieldPermissions(m.ScreenItem.Item.Org, m.ScreenItem.Item.Project, m.ScreenItem.Item.WorkItemType, m.ScreenItem.Item.State); !isCached {
							cmd = m.ensureWorkItemTypeMetadataCmd(m.ScreenItem.Item.Org, m.ScreenItem.Item.Project, m.ScreenItem.Item.WorkItemType)
						}
					}
					return m, cmd
				}
			case "s":
				if m.ScreenItem.Item != nil {
					availableStates := m.getStatesForItemType(m.ScreenItem.Item.WorkItemType)
					m.ModalStatus.Open(m.ScreenItem.Item.ID, m.ScreenItem.Item.Title, m.ScreenItem.Item.WorkItemType, m.ScreenItem.Item.State, availableStates)
					m.ActiveModal = ModalTypeStatus
					return m, nil
				}
			case "v", "V", "A", "shift+a":
				if m.ScreenItem.Item != nil {
					var dbActs []string
					if m.DB != nil {
						dbActs, _ = m.DB.GetDistinctActivities()
					}
					m.ModalActivity.Open(m.ScreenItem.Item.ID, m.ScreenItem.Item.Title, m.ScreenItem.Item.WorkItemType, m.ScreenItem.Item.Activity, dbActs)
					m.ActiveModal = ModalTypeActivity
					return m, nil
				}
			case "d":
				if m.ScreenItem.Item != nil {
					m.ModalSchedule.Open(m.ScreenItem.Item.ID, m.ScreenItem.Item.Title, m.ScreenItem.Item.WorkItemType, m.ScreenItem.Item.StartDate, m.ScreenItem.Item.TargetDate, m.ScreenItem.Item.FinishDate)
					m.ActiveModal = ModalTypeSchedule
					return m, nil
				}
			case "a":
				if m.ScreenItem.Item != nil {
					m.ModalAssign.Open(m.ScreenItem.Item.ID, m.ScreenItem.Item.Title, m.ScreenItem.Item.AssignedToEmail, m.DB)
					m.ActiveModal = ModalTypeAssign
					return m, nil
				}
			case "D", "shift+d":
				if m.ScreenItem.Item != nil {
					m.ModalConfirm.Open("Delete Work Item", fmt.Sprintf("Are you sure you want to delete #%d (%s)?", m.ScreenItem.Item.ID, m.ScreenItem.Item.Title), "delete", m.ScreenItem.Item.ID, true)
					m.ActiveModal = ModalTypeConfirm
					return m, nil
				}
			case "o", "O":
				if m.ScreenItem.Item != nil {
					m.openInBrowser(m.ScreenItem.Item.ID)
					return m, m.showToast(fmt.Sprintf("Opened current item #%d in browser", m.ScreenItem.Item.ID), false)
				}
			case "p", "P":
				if m.ScreenItem.Item != nil {
					if sel := m.ScreenItem.SelectedLink(); sel != nil && sel.URL != "" {
						m.openURL(sel.URL)
						return m, m.showToast(fmt.Sprintf("Opened %s in browser", sel.Title), false)
					} else if len(m.ScreenItem.RelatedLinks) > 0 {
						firstLink := m.ScreenItem.RelatedLinks[0]
						if firstLink.URL != "" {
							m.openURL(firstLink.URL)
							return m, m.showToast(fmt.Sprintf("Opened %s in browser", firstLink.Title), false)
						}
					} else {
						return m, m.showToast("No linked PR or resource to open", true)
					}
				}
			case "T", "shift+t":
				if len(m.ScreenTree.TreeRoots) > 0 {
					m.ModalTab.Open(m.ScreenTree.TreeRoots, m.ScreenTree.ActiveTab)
					m.ActiveModal = ModalTypeTab
					return m, nil
				}
				return m, m.showToast("No tracked root tabs available", false)
			case "r":
				if m.ScreenItem.Item != nil {
					if m.Syncing {
						return m, m.showToast("Sync already in progress...", false)
					}
					m.Loading = true
					m.Syncing = true
					id := m.ScreenItem.Item.ID
					return m, tea.Batch(
						m.showToast("Refreshing item from ADO...", false),
						func() tea.Msg {
							item, err := m.DB.GetWorkItem(id)
							if err != nil {
								return MsgItemUpdated{Err: err}
							}
							client := ado.NewClient(item.Org, item.Project, m.Config.PatToken)
							fetched, err := client.GetWorkItem(id)
							if err != nil {
								return MsgItemUpdated{Err: err}
							}
							_ = m.DB.SaveWorkItem(item.Org, item.Project, fetched)
							rec, _ := m.DB.GetWorkItem(id)
							return MsgItemUpdated{Item: rec}
						},
						m.loadCommentsCmd(id),
					)
				}
			}
			var cmd tea.Cmd
			m.ScreenItem, cmd = m.ScreenItem.Update(msg)
			return m, cmd

		case ScreenTypeCreate:
			if msg.String() == "esc" {
				m.CurrentScreen = ScreenTypeTree
				return m, nil
			}
			switch msg.String() {
			case "ctrl+s", "ctrl+enter":
				title := m.ScreenCreate.Title()
				if title == "" {
					return m, m.showToast("Title is required to create a work item", true)
				}
				itemType := m.ScreenCreate.SelectedType()
				desc := m.ScreenCreate.Description()
				assigned := m.ScreenCreate.AssignedTo()
				tags := m.ScreenCreate.Tags()
				iteration := m.ScreenCreate.Iteration()
				area := m.ScreenCreate.Area()
				parentID := m.ScreenCreate.ParentID()

				m.Loading = true
				return m, tea.Batch(
					m.showToast(fmt.Sprintf("Creating %s '%s'...", itemType, title), false),
					m.createItemCmd(itemType, title, desc, assigned, tags, iteration, area, 0, 0, parentID, "", "", ""),
				)
			case "enter", " ":
				if m.ScreenCreate.FocusedField == fieldCreateSubmit {
					title := m.ScreenCreate.Title()
					if title == "" {
						return m, m.showToast("Title is required to create a work item", true)
					}
					itemType := m.ScreenCreate.SelectedType()
					desc := m.ScreenCreate.Description()
					assigned := m.ScreenCreate.AssignedTo()
					tags := m.ScreenCreate.Tags()
					iteration := m.ScreenCreate.Iteration()
					area := m.ScreenCreate.Area()
					parentID := m.ScreenCreate.ParentID()

					m.Loading = true
					return m, tea.Batch(
						m.showToast(fmt.Sprintf("Creating %s '%s'...", itemType, title), false),
						m.createItemCmd(itemType, title, desc, assigned, tags, iteration, area, 0, 0, parentID, "", "", ""),
					)
				}
			}
			var cmd tea.Cmd
			m.ScreenCreate, cmd = m.ScreenCreate.Update(msg)
			return m, cmd

		case ScreenTypeSettings:
			action, cmd := m.ScreenSettings.Update(msg)
			if action == SettingsActionCancel {
				m.CurrentScreen = ScreenTypeTree
				return m, nil
			}
			if action == SettingsActionSave {
				pat, geminiKey, geminiModel, autoCalc, effortSame, autoSetFinishDate, autoSyncInterval,
					critEffort, critEstimate, critStartDate, critTargetDate, critFinishDate, critTagRegex, critActivity,
					tagRegex, incompleteTypes, incompleteStatuses, incompleteDateStatuses := m.ScreenSettings.Values()
				m.Loading = true
				return m, tea.Batch(
					m.showToast("Saving settings...", false),
					m.saveSettingsCmd(
						pat, geminiKey, geminiModel, autoCalc, effortSame, autoSetFinishDate, autoSyncInterval,
						critEffort, critEstimate, critStartDate, critTargetDate, critFinishDate, critTagRegex, critActivity,
						tagRegex, incompleteTypes, incompleteStatuses, incompleteDateStatuses,
					),
				)
			}
			return m, cmd
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *AppModel) availableScreenHeight() int {
	screenHeight := m.Height
	if m.DockGemini != nil && m.DockGemini.Visible {
		dockView := m.DockGemini.View()
		dockHeight := lipgloss.Height(dockView)
		screenHeight = m.Height - dockHeight
		if screenHeight < 1 {
			screenHeight = 1
		}
	}
	return screenHeight
}

func (m *AppModel) View() string {
	if m.Width > 0 && m.Height > 0 && (m.Width < MinTerminalWidth || m.Height < MinTerminalHeight) {
		return m.renderTerminalTooSmallView()
	}

	screenHeight := m.availableScreenHeight()
	var dockView string
	if m.DockGemini != nil && m.DockGemini.Visible {
		dockView = m.DockGemini.View()
	}

	effectiveToast := m.ToastMessage
	if m.Loading && effectiveToast != "" && !m.ToastIsError {
		spView := m.Spinner.View()
		effectiveToast = fmt.Sprintf("%s %s", spView, effectiveToast)
	}

	var baseView string
	switch m.CurrentScreen {
	case ScreenTypeTree:
		m.ScreenTree.Width = m.Width
		m.ScreenTree.Height = screenHeight
		baseView = m.ScreenTree.View(m.Width, screenHeight, effectiveToast, m.ToastIsError)
	case ScreenTypeItem:
		m.ScreenItem.Width = m.Width
		m.ScreenItem.Height = screenHeight
		baseView = m.ScreenItem.View(m.Width, screenHeight, effectiveToast, m.ToastIsError)
	case ScreenTypeCreate:
		baseView = m.ScreenCreate.View(m.Width, screenHeight, effectiveToast, m.ToastIsError)
	case ScreenTypeSettings:
		baseView = m.ScreenSettings.View(m.Width, screenHeight, effectiveToast, m.ToastIsError)
	}

	// Render modal overlay if active
	if m.ActiveModal != ModalTypeNone {
		var modalView string
		switch m.ActiveModal {
		case ModalTypeStatus:
			modalView = m.ModalStatus.View(m.Width, m.Height)
		case ModalTypeAssign:
			modalView = m.ModalAssign.View(m.Width, m.Height)
		case ModalTypeEstimate:
			modalView = m.ModalEstimate.View(m.Width, m.Height)
		case ModalTypeComment:
			modalView = m.ModalComment.View(m.Width, m.Height)
		case ModalTypeTrack:
			modalView = m.ModalTrack.View(m.Width, m.Height)
		case ModalTypeConfirm:
			modalView = m.ModalConfirm.View(m.Width, m.Height)
		case ModalTypeEdit:
			modalView = m.ModalEdit.View(m.Width, m.Height)
		case ModalTypeFilter:
			modalView = m.ModalFilter.View(m.Width, m.Height)
		case ModalTypeSchedule:
			modalView = m.ModalSchedule.View(m.Width, m.Height)
		case ModalTypeTab:
			modalView = m.ModalTab.View(m.Width, m.Height)
		case ModalTypeActivity:
			modalView = m.ModalActivity.View(m.Width, m.Height)
		}
		if modalView != "" {
			return modalView
		}
	}

	if m.DockGemini != nil && m.DockGemini.Visible && dockView != "" {
		return lipgloss.JoinVertical(lipgloss.Left, baseView, dockView)
	}

	return baseView
}

func (m *AppModel) renderTerminalTooSmallView() string {
	icon := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("⚠️  TERMINAL WINDOW TOO SMALL")

	msg := lipgloss.NewStyle().Foreground(ColorFg).Render("Azure DevOps Board requires a wider terminal window\nto display work item hierarchy, columns, and metrics accurately.")

	curSize := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("Current window size: %s", lipgloss.NewStyle().Foreground(ColorRed).Bold(true).Render(fmt.Sprintf("%d × %d", m.Width, m.Height))))
	minSize := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("Minimum required size: %s", lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render(fmt.Sprintf("%d × %d (or maximize window)", MinTerminalWidth, MinTerminalHeight))))

	hint := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("Please enlarge or maximize your terminal window.")
	quitHint := lipgloss.NewStyle().Foreground(ColorMuted).Render("Press [q] to quit")

	cardContent := lipgloss.JoinVertical(
		lipgloss.Center,
		icon,
		"",
		msg,
		"",
		curSize,
		minSize,
		"",
		hint,
		"",
		quitHint,
	)

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorYellow).
		Padding(1, 3).
		Render(cardContent)

	return lipgloss.Place(m.Width, m.Height, lipgloss.Center, lipgloss.Center, card)
}

func (m *AppModel) renameTab(tabIdx, rootID int, newName string) {
	if m.DB == nil {
		return
	}
	if tabIdx >= 0 && tabIdx < len(m.ScreenTree.TreeRoots) {
		root := m.ScreenTree.TreeRoots[tabIdx]
		if root != nil {
			root.CustomTitle = newName
			if root.IsQuery || (root.Record != nil && root.Record.WorkItemType == "Query") {
				oldTitle := ""
				if root.Record != nil {
					oldTitle = root.Record.Title
					root.Record.Title = newName
				}
				if root.QueryID != "" {
					_ = m.DB.UpdateTrackedQueryName(root.QueryID, newName)
				} else if queries, err := m.DB.GetTrackedQueries(); err == nil {
					for _, q := range queries {
						if q.Name == oldTitle || q.Name == newName {
							_ = m.DB.UpdateTrackedQueryName(q.ID, newName)
							break
						}
					}
				}
			} else if rootID > 0 {
				_ = m.DB.UpdateTrackedRootCustomTitle(rootID, newName)
			}
		}
	}
	m.ModalTab.UpdateTabTitle(tabIdx, rootID, newName)
	m.ScreenTree.rebuildFlattened()
}
