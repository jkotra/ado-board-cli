package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// EmbeddedSkillMarkdown contains the complete, self-contained documentation and instructions
// for AI agents (Claude, Antigravity, Gemini, Cursor) to manage Azure DevOps boards and orchestrate
// the live ado-board-cli interactive TUI.
const EmbeddedSkillMarkdown = `---
name: ado-board
description: >-
  Comprehensive guide and tool suite to inspect, track, update, create, and manage Azure DevOps work items, hierarchies, and control the live ado-board-cli TUI application.
---

# Azure DevOps Board & Work Item Management (ado-board-cli)

This skill equips you with tools to inspect, manage, and modify Azure DevOps boards, work item hierarchies, and orchestrate the live interactive ` + "`ado-board-cli`" + ` terminal application.

## 1. Hierarchy & Sizing Rules

In Azure DevOps, work items follow a hierarchical parent-child structure:
- **Epic** (top-level initiatives)
- **Feature** (major deliverables under an Epic)
- **User Story / Product Backlog Item (PBI)** (schedulable units of value under a Feature)
  - Story Sizing: Uses **Effort / Story Points** (e.g., 1, 2, 3, 5, 8, 13).
- **Task / Bug** (concrete implementation work under a Story/PBI/Feature)
  - Task Sizing: Uses hourly estimates:
    - **Original Estimate**: Initial planned hours.
    - **Remaining Work**: Hours left to complete.
    - **Completed Work**: Actual hours spent so far.

## 2. Tool Reference

### ADO Data & Workflow Tools
- ` + "`get_work_item_details(work_item_id)`" + `: Full metadata (including activity), child relations, comments, and links.
- ` + "`get_children(work_item_id, recursive, format, refresh)`" + `: Retrieves all children or recursive descendants under a work item (e.g. Feature, User Story, Epic, Bug) with hierarchy trees, rollup estimates, status/type breakdowns, and item lists.
- ` + "`get_hierarchy(work_item_id, include_ancestors, refresh)`" + `: Complete ancestor chain (from top root down) and descendant subtree hierarchy for a work item with rollup estimates.
- ` + "`update_work_item(work_item_id, ...)`" + `: Update state, estimates (original, remaining, completed, effort), activity ('Development', 'Testing', 'Design', 'Deployment', 'Documentation', 'Requirements'), assignee, dates, tags, iteration, area, title, description.
- ` + "`create_work_item(work_item_type, title, parent_id, ...)`" + `: Create a single new item (Task, Bug, PBI, Feature) with optional activity, tags, iteration, area, effort, estimates. Parent tags matching configured ` + "`tag_regex`" + ` rules are automatically inherited.
- ` + "`batch_create_tasks(parent_id, tasks)`" + `: Create multiple tasks under a parent in a single atomic call with estimates, activity, dates, tags, and assignees. Parent tags matching ` + "`tag_regex`" + ` rules are automatically inherited.
- ` + "`batch_update_items(updates)`" + `: Bulk update estimates, statuses, activity, assignees, dates, tags, iterations, and titles.
- ` + "`delete_work_item(work_item_id)`" + `: Permanently delete a work item.
- ` + "`add_comment(work_item_id, comment)`" + `: Post a discussion note to a work item.
- ` + "`get_activity(work_item_id, time_range, include_children)`" + `: Chronological event timeline (status changes, PRs, comments, estimates).
- ` + "`get_pull_requests(work_item_id)`" + `: Linked Pull Requests with branch & author status.
- ` + "`upload_pull_request_attachment(pull_request_id, file_path, file_name, repository_id)`" + `: Uploads an attachment/image to a PR and returns direct URL and markdown embed snippet.
- ` + "`update_pull_request(pull_request_id, description, append_description, title, repository_id)`" + `: Updates PR markdown description (or appends new text/attachment links) and title.
- ` + "`link_pull_request_work_item(pull_request_id, work_item_id, repository_id, comment)`" + `: Links a PR to a work item bidirectionally with ArtifactLink.
- ` + "`create_pull_request(source_branch, title, target_branch, description, is_draft, reviewers, required_reviewers, work_item_id, repository_id)`" + `: Creates a new PR from a source branch to target. Automatically detects local git repository from .git/config when repository_id is omitted.
- ` + "`add_pull_request_reviewers(pull_request_id, reviewers, required_reviewers, repository_id)`" + `: Adds optional and/or required reviewers to a PR using display names, emails, or identity GUIDs.
- ` + "`get_pull_request_details(pull_request_id, repository_id)`" + `: Retrieves complete metadata and details of a PR, including attachments, reviewers with votes/status, merge status, and linked work items.
- ` + "`get_pull_request_comments(pull_request_id, repository_id, include_system)`" + `: Retrieves discussion comments and review threads for a PR, including inline code comments, author details, and thread status.
- ` + "`add_pull_request_comment(pull_request_id, content, file_path, start_line, end_line, start_offset, end_offset, side, status, repository_id)`" + `: Adds a review comment or discussion thread to a PR with optional file path and code line selection.
- ` + "`reply_pull_request_comment(pull_request_id, thread_id, comment_id, content, status, resolved, repository_id, parent_comment_id)`" + `: Replies to a PR review comment or thread and optionally updates resolution status (e.g. resolve / unresolve).
- ` + "`get_board_summary()`" + `: Complete overview of tracked tabs, queries, item counts, and rollup totals.
- ` + "`get_users(query, limit)`" + `: Lists or searches project team members, users, and assignees with their display names and email addresses.
- ` + "`open_in_browser(work_item_id, url)`" + `: Opens a work item or query URL in the default web browser.
- ` + "`track_work_item(work_item_id, custom_title)`" + `: Add and track a work item as a new root tab on the board, importing its hierarchy.
- ` + "`untrack_work_item(work_item_id)`" + `: Untrack and remove a root work item tab from the board.
- ` + "`track_query(query_id_or_url, name)`" + `: Track an ADO query URL/ID as a board tab, importing all matching root items.
- ` + "`untrack_query(query_id)`" + `: Untrack and remove a query tab from the board.
- ` + "`get_settings()` / `update_settings(...)`" + `: Configure board incomplete criteria (including tag regex and activity requirements), ` + "`tag_regex`" + ` / ` + "`tag_regexes`" + ` rules, auto-sync intervals, and auto-setting finish dates.

### Live App Control Tools (Interactive TUI)
- ` + "`app_get_state()`" + `: Returns the complete live context and state of the running ` + "`ado-board-cli`" + ` TUI: currently selected/cursor work item (with parent and children), active screen, active tab, open tabs list, and active filters.
- ` + "`get_selected_item()`" + `: Returns the work item currently selected or under the cursor in the running TUI screen, along with its parent and child items.
- ` + "`app_switch_tab(tab_index, tab_name, root_id)`" + `: Switches active tab on the running TUI screen.
- ` + "`app_rename_tab(new_name, tab_index, root_id, tab_name)`" + `: Renames a board tab in the running TUI and database.
- ` + "`app_navigate_to_item(work_item_id, open_details)`" + `: Locates, expands parent tree nodes, highlights item, and optionally opens detail view.
- ` + "`app_refresh()`" + `: Triggers immediate background sync and screen re-render across active TUI windows.
- ` + "`app_set_filter(scope, clear_all, statuses, assignees, item_types, search)`" + `: Applies global or tab-specific filters on the live screen.
- ` + "`app_clear_filter(scope)`" + `: Clears active filters (scope: 'all', 'global', or 'tab') on the running TUI screen.
- ` + "`app_show_toast(message, is_error)`" + `: Displays an instant banner toast notification to the user in the running TUI.
- ` + "`app_set_screen(screen)`" + `: Switches screen ('tree', 'item', 'settings', 'create').

## 3. Best Practices & Workflow Patterns

- **Hierarchy & Subtree Inspection**:
  - When inspecting all children under a Feature, User Story, PBI, Epic, or Bug, **call ` + "`get_children(work_item_id)`" + ` or ` + "`get_hierarchy(work_item_id)`" + `**.
  - Do NOT query or inspect SQLite ` + "`ado.db`" + ` directly. ` + "`get_children`" + ` and ` + "`get_hierarchy`" + ` automatically return the complete nested tree, rollup estimates, status breakdowns, and descendant item lists.
- **Context Awareness & State Tracking**:
  - When the user asks contextual questions like *"what item am I inspecting?"*, *"what item is under my cursor?"*, *"do this to the selected item"*, or refers to *"this item / current story / active tab"*, **ALWAYS call ` + "`app_get_state`" + ` or ` + "`get_selected_item`" + ` first** to inspect the user's active cursor position, selected work item, open tab, child hierarchy, and running TUI state.
  - If ` + "`app_get_state`" + ` returns a ` + "`selected_item`" + `, you can immediately read its ID, title, state, and child items to fulfill the user's prompt without asking them for an ID.
- **Pull Request Management & Review Comments**:
  - When creating a pull request (` + "`create_pull_request`" + `) or inspecting PR details (` + "`get_pull_request_details`" + `), the tool automatically detects the local workspace's git repository by inspecting ` + "`.git/config`" + ` and remotes (SSH/HTTPS) and matching against Azure DevOps project repositories.
  - If calling from outside the repo workspace or targeting a specific repo in multi-repo setups, provide ` + "`repository_id`" + ` (name or GUID).
  - To link work items to the new PR, pass ` + "`work_item_id`" + ` to ` + "`create_pull_request`" + ` or call ` + "`link_pull_request_work_item`" + `.
  - Pass reviewer display names, emails, or GUIDs to ` + "`reviewers`" + ` and ` + "`required_reviewers`" + ` for automatic identity resolution.
  - To inspect review feedback and discussions on a PR, call ` + "`get_pull_request_comments`" + `.
  - To add new review comments on specific files/lines (or overall PR feedback), call ` + "`add_pull_request_comment`" + ` with ` + "`pull_request_id`" + `, ` + "`content`" + `, and optional ` + "`file_path`" + `, ` + "`start_line`" + `, ` + "`end_line`" + `, ` + "`side`" + `.
  - To address, reply to, or resolve/unresolve review comments, call ` + "`reply_pull_request_comment`" + ` with ` + "`pull_request_id`" + `, ` + "`thread_id`" + ` (or ` + "`comment_id`" + `), ` + "`content`" + `, and ` + "`status`" + ` (or ` + "`resolved=true/false`" + `).
- **Task Breakdown**: When breaking down a user story or feature:
  1. Call ` + "`get_work_item_details`" + ` or ` + "`get_children`" + ` to understand context and acceptance criteria.
  2. Call ` + "`batch_create_tasks`" + ` with realistic hourly estimates and activity (e.g. 'Development', 'Testing').
  3. Call ` + "`app_refresh`" + ` and ` + "`app_show_toast`" + ` to notify the user.
- **Context Highlighting**: When discussing a specific work item, call ` + "`app_navigate_to_item(work_item_id, open_details=false)`" + ` so the user's terminal jumps straight to that item.
- **Incomplete Items & Tag Regex Validation**: Look for items missing estimates, story points, or tags matching configured ` + "`tag_regex`" + ` rules and suggest or apply fixes with ` + "`update_work_item`" + `.
`

// GetClaudeConfigPath returns the OS-specific path to Claude Desktop configuration file.
func GetClaudeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Claude", "claude_desktop_config.json"), nil
	default:
		// Linux and others
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(home, ".config")
		}
		return filepath.Join(configDir, "Claude", "claude_desktop_config.json"), nil
	}
}

// GetClaudeCodeConfigPath returns the path to ~/.claude.json.
func GetClaudeCodeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude.json"), nil
}

// SetupClaude registers ado-board-cli in Claude Desktop / Claude Code configuration files.
// If isLocal is true, writes to local project files (.mcp.json & .claude.json in the current working directory).
func SetupClaude(binaryPath string, isLocal bool) (string, error) {
	if binaryPath == "" {
		var err error
		binaryPath, err = os.Executable()
		if err != nil {
			binaryPath = "ado-board-cli"
		}
	}

	var results []string

	if isLocal {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}

		// 1. Local .mcp.json
		localMcpPath := filepath.Join(cwd, ".mcp.json")
		if err := updateMCPConfigFile(localMcpPath, "ado-board", binaryPath, []string{"mcp"}); err != nil {
			results = append(results, fmt.Sprintf("⚠️ Failed to write local .mcp.json: %v", err))
		} else {
			results = append(results, fmt.Sprintf("✅ Local Claude MCP configured: %s", localMcpPath))
		}

		// 2. Local .claude.json
		localClaudePath := filepath.Join(cwd, ".claude.json")
		if err := updateMCPConfigFile(localClaudePath, "ado-board", binaryPath, []string{"mcp"}); err == nil {
			results = append(results, fmt.Sprintf("✅ Local Claude Code configured: %s", localClaudePath))
		}

		results = append(results, fmt.Sprintf("\n💡 Quick Claude CLI shortcut (in this project):\n   claude mcp add ado-board -- %s mcp", binaryPath))
		return strings.Join(results, "\n"), nil
	}

	// Global configuration
	// 1. Configure Claude Desktop
	desktopPath, err := GetClaudeConfigPath()
	if err == nil {
		if err := updateMCPConfigFile(desktopPath, "ado-board", binaryPath, []string{"mcp"}); err != nil {
			results = append(results, fmt.Sprintf("⚠️ Claude Desktop config update skipped (%v)", err))
		} else {
			results = append(results, fmt.Sprintf("✅ Claude Desktop configured: %s", desktopPath))
		}
	}

	// 2. Configure Global Claude Code (~/.claude.json)
	codePath, err := GetClaudeCodeConfigPath()
	if err == nil {
		if err := updateMCPConfigFile(codePath, "ado-board", binaryPath, []string{"mcp"}); err == nil {
			results = append(results, fmt.Sprintf("✅ Claude Code configured: %s", codePath))
		}
	}

	results = append(results, fmt.Sprintf("\n💡 Quick Claude CLI shortcut:\n   claude mcp add ado-board -- %s mcp", binaryPath))
	return strings.Join(results, "\n"), nil
}

// SetupAgents installs the embedded SKILL.md and registers MCP.
// If isLocal is true, installs into local workspace (./.gemini/skills/ado-board/SKILL.md & ./.gemini/mcp_config.json).
// If isLocal is false, installs globally into ~/.gemini/config/.
func SetupAgents(binaryPath string, isLocal bool) (string, error) {
	if binaryPath == "" {
		var err error
		binaryPath, err = os.Executable()
		if err != nil {
			binaryPath = "ado-board-cli"
		}
	}

	var results []string

	if isLocal {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}

		// 1. Install local workspace SKILL.md in .gemini/skills/ado-board/SKILL.md
		skillDir := filepath.Join(cwd, ".gemini", "skills", "ado-board")
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create local skill directory: %w", err)
		}

		skillFilePath := filepath.Join(skillDir, "SKILL.md")
		if err := os.WriteFile(skillFilePath, []byte(EmbeddedSkillMarkdown), 0644); err != nil {
			return "", fmt.Errorf("failed to write local SKILL.md: %w", err)
		}
		results = append(results, fmt.Sprintf("✅ Local workspace skill installed: %s", skillFilePath))

		// Also mirror to .agent/skills/ado-board/SKILL.md for compatibility
		agentSkillDir := filepath.Join(cwd, ".agent", "skills", "ado-board")
		if err := os.MkdirAll(agentSkillDir, 0755); err == nil {
			_ = os.WriteFile(filepath.Join(agentSkillDir, "SKILL.md"), []byte(EmbeddedSkillMarkdown), 0644)
		}

		// 2. Register local MCP in .gemini/mcp_config.json and .mcp.json
		localMcpConfig := filepath.Join(cwd, ".gemini", "mcp_config.json")
		if err := updateMCPConfigFile(localMcpConfig, "ado-board", binaryPath, []string{"mcp"}); err != nil {
			return "", fmt.Errorf("failed to update local mcp_config.json: %w", err)
		}
		results = append(results, fmt.Sprintf("✅ Local Agent MCP registered: %s", localMcpConfig))

		_ = updateMCPConfigFile(filepath.Join(cwd, ".mcp.json"), "ado-board", binaryPath, []string{"mcp"})

		return strings.Join(results, "\n"), nil
	}

	// Global configuration
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine user home directory: %w", err)
	}

	// 1. Install SKILL.md in ~/.gemini/config/skills/ado-board/SKILL.md
	skillDir := filepath.Join(home, ".gemini", "config", "skills", "ado-board")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create skill directory: %w", err)
	}

	skillFilePath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFilePath, []byte(EmbeddedSkillMarkdown), 0644); err != nil {
		return "", fmt.Errorf("failed to write SKILL.md: %w", err)
	}
	results = append(results, fmt.Sprintf("✅ Embedded skill installed: %s", skillFilePath))

	// 2. Register MCP in ~/.gemini/config/mcp_config.json
	mcpConfigPath := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	if err := updateMCPConfigFile(mcpConfigPath, "ado-board", binaryPath, []string{"mcp"}); err != nil {
		return "", fmt.Errorf("failed to update mcp_config.json: %w", err)
	}
	results = append(results, fmt.Sprintf("✅ Global Agent MCP registered: %s", mcpConfigPath))

	return strings.Join(results, "\n"), nil
}

// GetOpencodeHome returns the global opencode config directory ($XDG_CONFIG_HOME/opencode or ~/.config/opencode).
func GetOpencodeHome() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "opencode"), nil
}

// SetupOpencode installs the embedded SKILL.md and registers MCP for opencode.
// If isLocal is true, installs into ./opencode.json and ./.opencode/skills/ado-board/SKILL.md.
// If isLocal is false, installs globally into ~/.config/opencode/.
func SetupOpencode(binaryPath string, isLocal bool) (string, error) {
	if binaryPath == "" {
		var err error
		binaryPath, err = os.Executable()
		if err != nil {
			binaryPath = "ado-board-cli"
		}
	}

	var baseDir, configPath, scope string
	if isLocal {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		baseDir = filepath.Join(cwd, ".opencode")
		configPath = filepath.Join(cwd, "opencode.json")
		scope = "Local"
	} else {
		dir, err := GetOpencodeHome()
		if err != nil {
			return "", fmt.Errorf("could not determine opencode config directory: %w", err)
		}
		baseDir = dir
		configPath = filepath.Join(dir, "opencode.json")
		scope = "Global"
	}

	skillDir := filepath.Join(baseDir, "skills", "ado-board")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create opencode skill directory: %w", err)
	}
	skillFilePath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFilePath, []byte(EmbeddedSkillMarkdown), 0644); err != nil {
		return "", fmt.Errorf("failed to write opencode SKILL.md: %w", err)
	}

	if err := updateOpencodeConfigFile(configPath, "ado-board", binaryPath, []string{"mcp"}); err != nil {
		return "", fmt.Errorf("failed to update opencode.json: %w", err)
	}

	return strings.Join([]string{
		fmt.Sprintf("✅ %s opencode skill installed: %s", scope, skillFilePath),
		fmt.Sprintf("✅ %s opencode MCP registered: %s", scope, configPath),
	}, "\n"), nil
}

// updateOpencodeConfigFile registers a local MCP server under the "mcp" key of an opencode.json file.
func updateOpencodeConfigFile(filePath, serverName, command string, args []string) error {
	_ = os.MkdirAll(filepath.Dir(filePath), 0755)

	var configMap map[string]any
	if data, err := os.ReadFile(filePath); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &configMap)
	}
	if configMap == nil {
		configMap = make(map[string]any)
	}
	if _, ok := configMap["$schema"]; !ok {
		configMap["$schema"] = "https://opencode.ai/config.json"
	}

	mcpServers, ok := configMap["mcp"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
	}
	mcpServers[serverName] = map[string]any{
		"type":    "local",
		"command": append([]string{command}, args...),
		"enabled": true,
	}
	configMap["mcp"] = mcpServers

	outBytes, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, outBytes, 0644)
}

func updateMCPConfigFile(filePath, serverName, command string, args []string) error {
	dir := filepath.Dir(filePath)
	_ = os.MkdirAll(dir, 0755)

	var configMap map[string]any
	data, err := os.ReadFile(filePath)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &configMap)
	}

	if configMap == nil {
		configMap = make(map[string]any)
	}

	var mcpServers map[string]any
	if existing, ok := configMap["mcpServers"].(map[string]any); ok {
		mcpServers = existing
	} else {
		mcpServers = make(map[string]any)
	}

	mcpServers[serverName] = map[string]any{
		"command": command,
		"args":    args,
	}
	configMap["mcpServers"] = mcpServers

	outBytes, err := json.MarshalIndent(configMap, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, outBytes, 0644)
}

// GetDshHome returns the DeepSeek Harness home directory ($DSH_HOME or ~/.dsh)
func GetDshHome() (string, error) {
	if dshHome := os.Getenv("DSH_HOME"); dshHome != "" {
		return dshHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".dsh"), nil
}

// SetupDeepseek installs the embedded SKILL.md and registers the MCP client in DeepSeek Harness cordis.patch.yml.
// If isLocal is true, writes to local workspace (./.dsh/skills/ado-board/SKILL.md, ./.dsh/cordis.patch.yml).
// If isLocal is false, writes globally to ~/.dsh/skills/ado-board/SKILL.md and ~/.dsh/cordis.patch.yml.
func SetupDeepseek(binaryPath string, isLocal bool) (string, error) {
	if binaryPath == "" {
		var err error
		binaryPath, err = os.Executable()
		if err != nil {
			binaryPath = "ado-board-cli"
		}
	}

	var results []string

	if isLocal {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}

		// 1. Install local workspace skill in .dsh/skills/ado-board/SKILL.md
		localDshSkillDir := filepath.Join(cwd, ".dsh", "skills", "ado-board")
		if err := os.MkdirAll(localDshSkillDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create local deepseek skill directory: %w", err)
		}

		skillFilePath := filepath.Join(localDshSkillDir, "SKILL.md")
		if err := os.WriteFile(skillFilePath, []byte(EmbeddedSkillMarkdown), 0644); err != nil {
			return "", fmt.Errorf("failed to write local DeepSeek SKILL.md: %w", err)
		}
		results = append(results, fmt.Sprintf("✅ Local DeepSeek Harness skill installed: %s", skillFilePath))

		// Also mirror to .agents/skills/ado-board/SKILL.md for dsh-skill-filesystem compatibility
		localAgentsSkillDir := filepath.Join(cwd, ".agents", "skills", "ado-board")
		if err := os.MkdirAll(localAgentsSkillDir, 0755); err == nil {
			_ = os.WriteFile(filepath.Join(localAgentsSkillDir, "SKILL.md"), []byte(EmbeddedSkillMarkdown), 0644)
		}

		// 2. Configure local .dsh/cordis.patch.yml
		localPatchFile := filepath.Join(cwd, ".dsh", "cordis.patch.yml")
		if err := UpdateCordisPatchFile(localPatchFile, "ado-board", binaryPath, []string{"mcp"}); err != nil {
			results = append(results, fmt.Sprintf("⚠️ Failed to write local .dsh/cordis.patch.yml: %v", err))
		} else {
			results = append(results, fmt.Sprintf("✅ Local DeepSeek Harness patch configured: %s", localPatchFile))
		}

		// Clean any accidental duplicate entries in subprofiles if present
		if dshHome, err := GetDshHome(); err == nil {
			_ = RemoveCordisEntry(filepath.Join(dshHome, "profiles", "web", "cordis.patch.yml"), "ado-board")
			_ = RemoveCordisEntry(filepath.Join(dshHome, "profiles", "headless", "cordis.patch.yml"), "ado-board")
		}

		return strings.Join(results, "\n"), nil
	}

	// Global configuration
	dshHome, err := GetDshHome()
	if err != nil {
		return "", fmt.Errorf("could not determine DeepSeek Harness home directory: %w", err)
	}

	// 1. Install SKILL.md in ~/.dsh/skills/ado-board/SKILL.md
	skillDir := filepath.Join(dshHome, "skills", "ado-board")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create DeepSeek Harness skill directory: %w", err)
	}

	skillFilePath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFilePath, []byte(EmbeddedSkillMarkdown), 0644); err != nil {
		return "", fmt.Errorf("failed to write DeepSeek Harness SKILL.md: %w", err)
	}
	results = append(results, fmt.Sprintf("✅ Global DeepSeek Harness skill installed: %s", skillFilePath))

	// Also mirror to ~/.agents/skills/ado-board/SKILL.md
	if home, err := os.UserHomeDir(); err == nil {
		userAgentsSkillDir := filepath.Join(home, ".agents", "skills", "ado-board")
		if err := os.MkdirAll(userAgentsSkillDir, 0755); err == nil {
			_ = os.WriteFile(filepath.Join(userAgentsSkillDir, "SKILL.md"), []byte(EmbeddedSkillMarkdown), 0644)
		}
	}

	// 2. Register MCP in ~/.dsh/cordis.patch.yml (single global entrypoint for all profiles)
	globalPatchFile := filepath.Join(dshHome, "cordis.patch.yml")
	if err := UpdateCordisPatchFile(globalPatchFile, "ado-board", binaryPath, []string{"mcp"}); err != nil {
		return "", fmt.Errorf("failed to update DeepSeek Harness cordis.patch.yml: %w", err)
	}
	results = append(results, fmt.Sprintf("✅ Global DeepSeek Harness MCP registered: %s", globalPatchFile))

	// 3. Clean any duplicate entries in profile subdirectories so Cordis loader won't fail with duplicate entry IDs
	_ = RemoveCordisEntry(filepath.Join(dshHome, "profiles", "web", "cordis.patch.yml"), "ado-board")
	_ = RemoveCordisEntry(filepath.Join(dshHome, "profiles", "headless", "cordis.patch.yml"), "ado-board")

	return strings.Join(results, "\n"), nil
}

// RemoveCordisEntry removes an MCP client entry by serverName from a cordis.patch.yml file if present.
func RemoveCordisEntry(filePath, serverName string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}

	var patches []any
	if err := yaml.Unmarshal(data, &patches); err != nil {
		return nil
	}

	targetID := "mcp-" + serverName
	var cleanedPatches []any
	changed := false

	for _, rawPatch := range patches {
		patchMap, ok := rawPatch.(map[string]any)
		if !ok {
			if m, ok2 := rawPatch.(map[interface{}]interface{}); ok2 {
				patchMap = make(map[string]any)
				for k, v := range m {
					if ks, ok := k.(string); ok {
						patchMap[ks] = v
					}
				}
			} else {
				cleanedPatches = append(cleanedPatches, rawPatch)
				continue
			}
		}

		// Check direct entry
		if id, ok := patchMap["id"].(string); ok && id == targetID {
			changed = true
			continue
		}

		// Check insert array
		if insertList, ok := patchMap["insert"].([]any); ok {
			var newInsertList []any
			for _, subItem := range insertList {
				subMap, ok := subItem.(map[string]any)
				if !ok {
					if sm, ok2 := subItem.(map[interface{}]interface{}); ok2 {
						subMap = make(map[string]any)
						for k, v := range sm {
							if ks, ok := k.(string); ok {
								subMap[ks] = v
							}
						}
					}
				}
				if subMap != nil {
					if id, ok := subMap["id"].(string); ok && id == targetID {
						changed = true
						continue
					}
					if cfg, ok := subMap["config"].(map[string]any); ok {
						if sName, ok := cfg["serverName"].(string); ok && sName == serverName {
							changed = true
							continue
						}
					}
				}
				newInsertList = append(newInsertList, subItem)
			}
			if len(newInsertList) == 0 {
				changed = true
				continue
			}
			patchMap["insert"] = newInsertList
		}

		cleanedPatches = append(cleanedPatches, patchMap)
	}

	if !changed {
		return nil
	}

	if len(cleanedPatches) == 0 {
		return os.WriteFile(filePath, []byte("[]\n"), 0644)
	}

	outBytes, err := yaml.Marshal(cleanedPatches)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, outBytes, 0644)
}

// UpdateCordisPatchFile modifies or adds an MCP client entry in DeepSeek Harness cordis.patch.yml.
func UpdateCordisPatchFile(filePath, serverName, command string, args []string) error {
	dir := filepath.Dir(filePath)
	_ = os.MkdirAll(dir, 0755)

	targetID := "mcp-" + serverName
	newEntry := map[string]any{
		"id":   targetID,
		"name": "@deepseek-ai/dsh-mcp-client",
		"config": map[string]any{
			"transport":  "stdio",
			"serverName": serverName,
			"command":    command,
			"args":       args,
		},
	}

	var patches []any
	data, err := os.ReadFile(filePath)
	if err == nil && len(strings.TrimSpace(string(data))) > 0 {
		_ = yaml.Unmarshal(data, &patches)
	}

	updated := false
	for _, rawPatch := range patches {
		patchMap, ok := rawPatch.(map[string]any)
		if !ok {
			if m, ok2 := rawPatch.(map[interface{}]interface{}); ok2 {
				patchMap = make(map[string]any)
				for k, v := range m {
					if ks, ok := k.(string); ok {
						patchMap[ks] = v
					}
				}
			} else {
				continue
			}
		}

		if id, ok := patchMap["id"].(string); ok && id == targetID {
			patchMap["name"] = "@deepseek-ai/dsh-mcp-client"
			patchMap["config"] = map[string]any{
				"transport":  "stdio",
				"serverName": serverName,
				"command":    command,
				"args":       args,
			}
			updated = true
			break
		}

		if insertList, ok := patchMap["insert"].([]any); ok {
			for i, subItem := range insertList {
				subMap, ok := subItem.(map[string]any)
				if !ok {
					if sm, ok2 := subItem.(map[interface{}]interface{}); ok2 {
						subMap = make(map[string]any)
						for k, v := range sm {
							if ks, ok := k.(string); ok {
								subMap[ks] = v
							}
						}
					}
				}
				if subMap != nil {
					if id, ok := subMap["id"].(string); ok && id == targetID {
						insertList[i] = newEntry
						patchMap["insert"] = insertList
						updated = true
						break
					}
					if cfg, ok := subMap["config"].(map[string]any); ok {
						if sName, ok := cfg["serverName"].(string); ok && sName == serverName {
							insertList[i] = newEntry
							patchMap["insert"] = insertList
							updated = true
							break
						}
					}
				}
			}
			if updated {
				break
			}
		}
	}

	if !updated {
		newPatch := map[string]any{
			"insert": []any{newEntry},
		}
		patches = append(patches, newPatch)
	}

	outBytes, err := yaml.Marshal(patches)
	if err != nil {
		return fmt.Errorf("failed to marshal yaml: %w", err)
	}

	return os.WriteFile(filePath, outBytes, 0644)
}
