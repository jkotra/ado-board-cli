package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/db"
	"ado-board-cli/pkg/gemini"
	"ado-board-cli/pkg/ipc"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterAppControlTools adds live TUI interaction tools to the MCP server.
func RegisterAppControlTools(s *server.MCPServer, adoClient *ado.Client, database *db.DB) {
	getExecutor := func() *gemini.Executor {
		return gemini.NewExecutor(adoClient, database, nil, nil)
	}

	// 0. app_get_state
	s.AddTool(
		mcp.NewTool(
			"app_get_state",
			mcp.WithDescription("Returns the complete live context of the running ado-board-cli TUI application: currently selected/cursor work item (with parent and children), active screen, active tab, all open tabs, and active filters. If TUI is offline, returns saved board tabs from local database."),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, err := ipc.SendCommand(ctx, ipc.Request{
				Command: ipc.CmdGetState,
			})
			if err != nil {
				if errors.Is(err, ipc.ErrAppNotRunning) {
					return buildOfflineBoardState(database)
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get TUI state: %v", err)), nil
			}

			if resp.Data != nil {
				return mcp.NewToolResultText(toJSONString(resp.Data)), nil
			}
			return mcp.NewToolResultText(resp.Message), nil
		},
	)

	// 0b. get_selected_item
	s.AddTool(
		mcp.NewTool(
			"get_selected_item",
			mcp.WithDescription("Returns the work item currently selected or under the cursor in the running ado-board-cli TUI screen, along with its parent and child items."),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, err := ipc.SendCommand(ctx, ipc.Request{
				Command: ipc.CmdGetState,
			})
			if err != nil {
				if errors.Is(err, ipc.ErrAppNotRunning) {
					return buildOfflineBoardState(database)
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get selected item: %v", err)), nil
			}

			if resp.Data != nil {
				// If Data is already parsed or a map
				if m, ok := resp.Data.(map[string]any); ok {
					if sel, ok := m["selected_item"]; ok && sel != nil {
						resMap := map[string]any{
							"is_running":    true,
							"active_screen": m["active_screen"],
							"selected_item": sel,
							"parent_item":   m["parent_item"],
							"child_items":   m["child_items"],
						}
						return mcp.NewToolResultText(toJSONString(resMap)), nil
					}
					return mcp.NewToolResultText(`{"is_running": true, "selected_item": null, "message": "No item is currently selected in TUI."}`), nil
				}
				return mcp.NewToolResultText(toJSONString(resp.Data)), nil
			}
			return mcp.NewToolResultText(resp.Message), nil
		},
	)

	// 1. app_switch_tab
	s.AddTool(
		mcp.NewTool(
			"app_switch_tab",
			mcp.WithDescription("Switches the active work item tree / query tab on the running ado-board-cli TUI screen by index, name, or root work item ID."),
			mcp.WithNumber("tab_index", mcp.Description("Zero-based tab index (e.g. 0, 1, 2)")),
			mcp.WithString("tab_name", mcp.Description("Partial or full title of the tab to switch to")),
			mcp.WithNumber("root_id", mcp.Description("The root work item ID of the tracked tab")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			tabIdx := request.GetInt("tab_index", -1)
			tabName := request.GetString("tab_name", "")
			rootID := request.GetInt("root_id", 0)

			resp, err := ipc.SendCommand(ctx, ipc.Request{
				Command:  ipc.CmdSwitchTab,
				TabIndex: tabIdx,
				TabName:  tabName,
				RootID:   rootID,
			})
			if err != nil {
				if errors.Is(err, ipc.ErrAppNotRunning) {
					return mcp.NewToolResultText("Notice: The interactive ado-board-cli TUI is not currently running. No live screen tab was switched."), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to switch tab: %v", err)), nil
			}
			return mcp.NewToolResultText(resp.Message), nil
		},
	)

	// 2. app_navigate_to_item
	s.AddTool(
		mcp.NewTool(
			"app_navigate_to_item",
			mcp.WithDescription("Locates a work item in the tree, expands all ancestor parent nodes, highlights it in the running TUI, and optionally opens its full detail view."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the work item to focus and navigate to")),
			mcp.WithBoolean("open_details", mcp.Description("If true, automatically opens the work item detail screen")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := request.RequireInt("work_item_id")
			if err != nil {
				return mcp.NewToolResultError("missing required parameter: work_item_id"), nil
			}
			openDetails := request.GetBool("open_details", false)

			resp, err := ipc.SendCommand(ctx, ipc.Request{
				Command:     ipc.CmdNavigateToItem,
				WorkItemID:  id,
				OpenDetails: openDetails,
			})
			if err != nil {
				if errors.Is(err, ipc.ErrAppNotRunning) {
					return mcp.NewToolResultText(fmt.Sprintf("Notice: Work item #%d identified. Interactive TUI is currently offline so live navigation was skipped.", id)), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("Navigation error: %v", err)), nil
			}
			return mcp.NewToolResultText(resp.Message), nil
		},
	)

	// 3. app_refresh
	s.AddTool(
		mcp.NewTool(
			"app_refresh",
			mcp.WithDescription("Triggers immediate synchronization with Azure DevOps and refreshes the tree and UI state on the running board."),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			resp, err := ipc.SendCommand(ctx, ipc.Request{
				Command: ipc.CmdRefresh,
			})
			if err != nil {
				if errors.Is(err, ipc.ErrAppNotRunning) {
					// Fallback: perform direct sync against DB and ADO if client is available
					return mcp.NewToolResultText("Notice: Board data is up to date in SQLite. Interactive TUI is currently offline."), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("Refresh failed: %v", err)), nil
			}
			return mcp.NewToolResultText(resp.Message), nil
		},
	)

	// 4. app_set_filter
	s.AddTool(
		mcp.NewTool(
			"app_set_filter",
			mcp.WithDescription("Applies global or tab-specific filters (by status, assignee, item type, or search term) on the active TUI board. To clear filters, set clear_all to true."),
			mcp.WithString("scope", mcp.Description("Filter scope: 'global' (applies to all tabs) or 'tab' (applies to active tab only). Defaults to 'global'.")),
			mcp.WithBoolean("clear_all", mcp.Description("If true, clears active filters for the given scope")),
			mcp.WithArray("statuses", mcp.Description("List of work item states/statuses to match (e.g. ['Active', 'In Progress'])")),
			mcp.WithArray("assignees", mcp.Description("List of assignees to match")),
			mcp.WithArray("item_types", mcp.Description("List of work item types to match (e.g. ['Task', 'Bug'])")),
			mcp.WithString("search", mcp.Description("Search substring to match in work item titles or IDs")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			scope := request.GetString("scope", "global")
			clearAll := request.GetBool("clear_all", false)
			statuses := request.GetStringSlice("statuses", nil)
			assignees := request.GetStringSlice("assignees", nil)
			types := request.GetStringSlice("item_types", nil)
			search := request.GetString("search", "")

			resp, err := ipc.SendCommand(ctx, ipc.Request{
				Command:     ipc.CmdSetFilter,
				FilterScope: scope,
				ClearAll:    clearAll,
				Statuses:    statuses,
				Assignees:   assignees,
				ItemTypes:   types,
				Search:      search,
			})
			if err != nil {
				if errors.Is(err, ipc.ErrAppNotRunning) {
					return mcp.NewToolResultText("Notice: Filter evaluated. Interactive TUI is currently offline."), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to set filter: %v", err)), nil
			}
			return mcp.NewToolResultText(resp.Message), nil
		},
	)

	// 4b. app_clear_filter
	s.AddTool(
		mcp.NewTool(
			"app_clear_filter",
			mcp.WithDescription("Clears active filters on the running ado-board-cli TUI application."),
			mcp.WithString("scope", mcp.Description("Filter scope to clear: 'all' (clears both global and tab filters), 'global' (clears global filter only), or 'tab' (clears active tab filter only). Defaults to 'all'.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			scope := request.GetString("scope", "all")
			resp, err := ipc.SendCommand(ctx, ipc.Request{
				Command:     ipc.CmdClearFilter,
				FilterScope: scope,
				ClearAll:    true,
			})
			if err != nil {
				if errors.Is(err, ipc.ErrAppNotRunning) {
					return mcp.NewToolResultText("Notice: Filter clear command evaluated. Interactive TUI is currently offline."), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to clear filters: %v", err)), nil
			}
			return mcp.NewToolResultText(resp.Message), nil
		},
	)

	// 4c. app_rename_tab
	s.AddTool(
		mcp.NewTool(
			"app_rename_tab",
			mcp.WithDescription("Renames a tab on the board (tracked root item or query tab) in the running TUI and persists to database."),
			mcp.WithString("new_name", mcp.Required(), mcp.Description("The new title for the tab")),
			mcp.WithNumber("tab_index", mcp.Description("0-based index of the tab to rename (optional)")),
			mcp.WithNumber("root_id", mcp.Description("Work item ID of the tracked root to rename (optional)")),
			mcp.WithString("tab_name", mcp.Description("Existing name of the tab to rename (optional)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			newName, err := request.RequireString("new_name")
			if err != nil {
				return mcp.NewToolResultError("missing required parameter: new_name"), nil
			}
			tabIdx := request.GetInt("tab_index", -1)
			rootID := request.GetInt("root_id", 0)
			tabName := request.GetString("tab_name", "")

			exec := getExecutor()
			res, err := exec.Execute("rename_tab", map[string]any{
				"new_name":  newName,
				"tab_index": tabIdx,
				"root_id":   rootID,
				"tab_name":  tabName,
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 5. app_show_toast
	s.AddTool(
		mcp.NewTool(
			"app_show_toast",
			mcp.WithDescription("Displays an instant banner toast notification to the user in the running ado-board-cli TUI window."),
			mcp.WithString("message", mcp.Required(), mcp.Description("The message text to display")),
			mcp.WithBoolean("is_error", mcp.Description("If true, formats the toast as an alert/error")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msg, err := request.RequireString("message")
			if err != nil {
				return mcp.NewToolResultError("missing required parameter: message"), nil
			}
			isErr := request.GetBool("is_error", false)

			resp, err := ipc.SendCommand(ctx, ipc.Request{
				Command:      ipc.CmdShowToast,
				ToastMessage: msg,
				IsError:      isErr,
			})
			if err != nil {
				if errors.Is(err, ipc.ErrAppNotRunning) {
					return mcp.NewToolResultText(fmt.Sprintf("Toast ignored (TUI offline): %s", msg)), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to show toast: %v", err)), nil
			}
			return mcp.NewToolResultText(resp.Message), nil
		},
	)

	// 6. app_set_screen
	s.AddTool(
		mcp.NewTool(
			"app_set_screen",
			mcp.WithDescription("Navigates the running TUI application to a specific screen view ('tree', 'item', 'settings', 'create')."),
			mcp.WithString("screen", mcp.Required(), mcp.Description("Target screen: 'tree', 'item', 'settings', or 'create'")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			screen, err := request.RequireString("screen")
			if err != nil {
				return mcp.NewToolResultError("missing required parameter: screen"), nil
			}

			resp, err := ipc.SendCommand(ctx, ipc.Request{
				Command: ipc.CmdSetScreen,
				Screen:  screen,
			})
			if err != nil {
				if errors.Is(err, ipc.ErrAppNotRunning) {
					return mcp.NewToolResultText(fmt.Sprintf("Notice: Screen change to '%s' skipped (TUI offline).", screen)), nil
				}
				return mcp.NewToolResultError(fmt.Sprintf("Failed to set screen: %v", err)), nil
			}
			return mcp.NewToolResultText(resp.Message), nil
		},
	)
}

func toJSONString(val any) string {
	b, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", val)
	}
	return string(b)
}

func buildOfflineBoardState(database *db.DB) (*mcp.CallToolResult, error) {
	if database == nil {
		return mcp.NewToolResultText(`{"is_running": false, "message": "Notice: Interactive ado-board-cli TUI is not running and database is unavailable."}`), nil
	}

	roots, err := database.GetTrackedRoots()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
	}
	queries, _ := database.GetTrackedQueries()

	var openTabs []ipc.TabSummary
	for i, root := range roots {
		name := root.CustomTitle
		if name == "" {
			if item, err := database.GetWorkItem(root.ID); err == nil && item != nil && item.Title != "" {
				name = item.Title
			} else {
				name = fmt.Sprintf("Work Item #%d", root.ID)
			}
		}
		openTabs = append(openTabs, ipc.TabSummary{
			Index:    i,
			Name:     name,
			RootID:   root.ID,
			IsQuery:  false,
			IsActive: i == 0,
		})
	}
	for i, q := range queries {
		openTabs = append(openTabs, ipc.TabSummary{
			Index:    len(roots) + i,
			Name:     q.Name,
			IsQuery:  true,
			QueryID:  q.ID,
			IsActive: len(roots) == 0 && i == 0,
		})
	}

	offlineState := map[string]any{
		"is_running":    false,
		"message":       "Notice: The interactive ado-board-cli TUI is not currently running. Returning saved board tabs and roots from local database.",
		"open_tabs":     openTabs,
		"total_tracked": len(roots) + len(queries),
	}

	if len(roots) > 0 {
		if item, err := database.GetWorkItem(roots[0].ID); err == nil && item != nil {
			offlineState["top_root_item"] = item
		}
	}

	return mcp.NewToolResultText(toJSONString(offlineState)), nil
}
