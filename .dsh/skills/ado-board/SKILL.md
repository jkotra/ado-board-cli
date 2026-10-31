---
name: ado-board
description: >-
  Comprehensive guide and tool suite to inspect, track, update, create, and manage Azure DevOps work items, hierarchies, and control the live ado-board-cli TUI application.
---

# Azure DevOps Board & Work Item Management (ado-board-cli)

This skill equips you with tools to inspect, manage, and modify Azure DevOps boards, work item hierarchies, and orchestrate the live interactive `ado-board-cli` terminal application.

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
- `get_work_item_details(work_item_id)`: Full metadata, child relations, comments, and links.
- `update_work_item(work_item_id, ...)`: Update state, estimates (original, remaining, completed, effort), assignee, dates, tags, iteration, area, title, description.
- `create_work_item(work_item_type, title, parent_id, ...)`: Create a single new item (Task, Bug, PBI, Feature) with optional tags, iteration, area, effort, estimates.
- `batch_create_tasks(parent_id, tasks)`: Create multiple tasks under a parent in a single atomic call with estimates, dates, tags, and assignees.
- `batch_update_items(updates)`: Bulk update estimates, statuses, assignees, dates, tags, iterations, and titles.
- `delete_work_item(work_item_id)`: Permanently delete a work item.
- `add_comment(work_item_id, comment)`: Post a discussion note to a work item.
- `get_activity(work_item_id, time_range, include_children)`: Chronological event timeline (status changes, PRs, comments, estimates).
- `get_pull_requests(work_item_id)`: Linked Pull Requests with branch & author status.
- `get_board_summary()`: Complete overview of tracked tabs, queries, item counts, and rollup totals.
- `get_users(query, limit)`: Lists or searches project team members, users, and assignees with their display names and email addresses.
- `open_in_browser(work_item_id, url)`: Opens a work item or query URL in the default web browser.
- `track_work_item(work_item_id, custom_title)`: Add and track a work item as a new root tab on the board, importing its hierarchy.
- `untrack_work_item(work_item_id)`: Untrack and remove a root work item tab from the board.
- `track_query(query_id_or_url, name)`: Track an ADO query URL/ID as a board tab, importing all matching root items.
- `untrack_query(query_id)`: Untrack and remove a query tab from the board.
- `get_settings()` / `update_settings(...)`: Configure board incomplete criteria and auto-sync intervals.

### Live App Control Tools (Interactive TUI)
- `app_get_state()`: Returns the complete live context and state of the running `ado-board-cli` TUI: currently selected/cursor work item (with parent and children), active screen, active tab, open tabs list, and active filters.
- `get_selected_item()`: Returns the work item currently selected or under the cursor in the running TUI screen, along with its parent and child items.
- `app_switch_tab(tab_index, tab_name, root_id)`: Switches active tab on the running TUI screen.
- `app_rename_tab(new_name, tab_index, root_id, tab_name)`: Renames a board tab in the running TUI and database.
- `app_navigate_to_item(work_item_id, open_details)`: Locates, expands parent tree nodes, highlights item, and optionally opens detail view.
- `app_refresh()`: Triggers immediate background sync and screen re-render across active TUI windows.
- `app_set_filter(scope, clear_all, statuses, assignees, item_types, search)`: Applies global or tab-specific filters on the live screen.
- `app_clear_filter(scope)`: Clears active filters (scope: 'all', 'global', or 'tab') on the running TUI screen.
- `app_show_toast(message, is_error)`: Displays an instant banner toast notification to the user in the running TUI.
- `app_set_screen(screen)`: Switches screen ('tree', 'item', 'settings', 'create').

## 3. Best Practices & Workflow Patterns

- **Context Awareness & State Tracking**:
  - When the user asks contextual questions like *"what item am I inspecting?"*, *"what item is under my cursor?"*, *"do this to the selected item"*, or refers to *"this item / current story / active tab"*, **ALWAYS call `app_get_state` or `get_selected_item` first** to inspect the user's active cursor position, selected work item, open tab, child hierarchy, and running TUI state.
  - If `app_get_state` returns a `selected_item`, you can immediately read its ID, title, state, and child items to fulfill the user's prompt without asking them for an ID.
- **Task Breakdown**: When breaking down a user story or feature:
  1. Call `get_work_item_details` to understand context and acceptance criteria.
  2. Call `batch_create_tasks` with realistic hourly estimates.
  3. Call `app_refresh` and `app_show_toast` to notify the user.
- **Context Highlighting**: When discussing a specific work item, call `app_navigate_to_item(work_item_id, open_details=false)` so the user's terminal jumps straight to that item.
- **Incomplete Items**: Look for items missing estimates or story points and suggest or apply fixes with `update_work_item`.
