package mcp

import (
	"context"
	"fmt"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
	"ado-board-cli/pkg/gemini"
	"ado-board-cli/pkg/ipc"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterAdoTools registers all Azure DevOps data and workflow tools to the MCP server.
func RegisterAdoTools(s *server.MCPServer, adoClient *ado.Client, database *db.DB, cfg *config.Config) {
	// Helper to create an executor instance for each call
	getExecutor := func() *gemini.Executor {
		ctxPayload := &gemini.ContextPayload{}
		if database != nil {
			if identities, err := database.SearchIdentities("", 500); err == nil {
				for _, id := range identities {
					ctxPayload.CachedIdentities = append(ctxPayload.CachedIdentities, gemini.IdentitySummary{
						DisplayName: id.DisplayName,
						Email:       id.Email,
					})
				}
			}
		}
		// In automated MCP mode, confirmation callback is nil
		return gemini.NewExecutor(adoClient, database, ctxPayload, nil)
	}

	// 1. get_work_item_details
	s.AddTool(
		mcp.NewTool(
			"get_work_item_details",
			mcp.WithDescription("Retrieves full metadata, hierarchy relations, discussion comments, and links for a specific Azure DevOps work item by ID."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the work item (e.g. 666236)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_work_item_details", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// get_children
	s.AddTool(
		mcp.NewTool(
			"get_children",
			mcp.WithDescription("Retrieves all children or recursive descendants under a work item (e.g. Feature, User Story, Epic, Bug) with hierarchy trees, rollup estimates, and status breakdowns."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the parent/root work item (e.g. 656842)")),
			mcp.WithBoolean("recursive", mcp.Description("If true (default), returns all recursive descendants (subtrees, tasks under PBIs). If false, returns only immediate direct children.")),
			mcp.WithString("format", mcp.Description("Format of output: 'tree' (nested hierarchy with rollups), 'flat' (list of work items), or 'both'. Default is 'tree'.")),
			mcp.WithBoolean("refresh", mcp.Description("If true, fetches fresh data directly from Azure DevOps before building hierarchy.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_children", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// get_hierarchy
	s.AddTool(
		mcp.NewTool(
			"get_hierarchy",
			mcp.WithDescription("Retrieves the complete ancestor chain and descendant subtree hierarchy for a work item, including rollup estimates and status/type breakdowns."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the work item (e.g. 656842)")),
			mcp.WithBoolean("include_ancestors", mcp.Description("If true (default), includes the ancestor chain up to the top root item.")),
			mcp.WithBoolean("refresh", mcp.Description("If true, fetches fresh data directly from Azure DevOps before building hierarchy.")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_hierarchy", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 2. update_work_item
	s.AddTool(
		mcp.NewTool(
			"update_work_item",
			mcp.WithDescription("Updates one or more fields on an existing Azure DevOps work item (state/status, estimates, assignee, dates, title, description)."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the work item to update")),
			mcp.WithString("state", mcp.Description("New state (e.g. 'New', 'Active', 'In Progress', 'Done', 'Closed', 'Resolved', 'Approved', 'Committed', 'Removed')")),
			mcp.WithString("title", mcp.Description("Updated title for the work item")),
			mcp.WithString("assigned_to", mcp.Description("Email address or display name of the person to assign to")),
			mcp.WithNumber("original_estimate", mcp.Description("Original estimate in hours")),
			mcp.WithNumber("remaining_work", mcp.Description("Remaining work in hours")),
			mcp.WithNumber("completed_work", mcp.Description("Completed work in hours")),
			mcp.WithNumber("effort", mcp.Description("Story points or effort value")),
			mcp.WithString("start_date", mcp.Description("Start date and time (e.g. '8/18/2026 12:00 AM', '2026-08-18', 'today', 'tomorrow 9am'). Empty string to clear.")),
			mcp.WithString("target_date", mcp.Description("Target / Due date and time (e.g. '8/19/2026 8:01 AM', '2026-08-19', '+3d'). Empty string to clear.")),
			mcp.WithString("finish_date", mcp.Description("Finish date and time (e.g. '8/21/2026 3:57 PM', '2026-08-21', '+1w'). Empty string to clear.")),
			mcp.WithString("description", mcp.Description("Updated description text or HTML")),
			mcp.WithString("tags", mcp.Description("Tags associated with work item (semicolon or comma separated, e.g. 'backend; api; v2'). Empty string to clear.")),
			mcp.WithString("activity", mcp.Description("Activity category (e.g. 'Development', 'Testing', 'Design', 'Deployment', 'Documentation', 'Requirements'). Empty string to clear.")),
			mcp.WithString("iteration_path", mcp.Description("ADO iteration sprint path (e.g. 'Project\\Sprint 1')")),
			mcp.WithString("area_path", mcp.Description("ADO area team path (e.g. 'Project\\Backend Team')")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("update_work_item", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 3. create_work_item
	s.AddTool(
		mcp.NewTool(
			"create_work_item",
			mcp.WithDescription("Creates a single new work item (Task, Bug, Product Backlog Item, User Story, Feature) linked to an optional parent item."),
			mcp.WithString("work_item_type", mcp.Required(), mcp.Description("Work item type, such as 'Task', 'Bug', 'Product Backlog Item', 'Feature', or 'User Story'")),
			mcp.WithString("title", mcp.Required(), mcp.Description("Title of the new work item")),
			mcp.WithNumber("parent_id", mcp.Description("Parent work item ID to link under (e.g. parent PBI or Feature ID). 0 for root level.")),
			mcp.WithString("description", mcp.Description("Description or repro steps for the work item")),
			mcp.WithString("assigned_to", mcp.Description("Email address or display name of assignee")),
			mcp.WithString("activity", mcp.Description("Activity category (e.g. 'Development', 'Testing', 'Design', 'Deployment', 'Documentation', 'Requirements')")),
			mcp.WithNumber("original_estimate", mcp.Description("Original estimate in hours")),
			mcp.WithNumber("remaining_work", mcp.Description("Remaining work in hours")),
			mcp.WithNumber("effort", mcp.Description("Story points or effort value")),
			mcp.WithString("tags", mcp.Description("Tags (semicolon or comma separated)")),
			mcp.WithString("iteration_path", mcp.Description("ADO iteration sprint path (e.g. 'Project\\Sprint 1')")),
			mcp.WithString("area_path", mcp.Description("ADO area team path (e.g. 'Project\\Backend Team')")),
			mcp.WithString("start_date", mcp.Description("Start date and time (e.g. '8/18/2026 12:00 AM', '2026-08-18', 'today')")),
			mcp.WithString("target_date", mcp.Description("Target / Due date and time (e.g. '8/19/2026 8:01 AM', '2026-08-19', '+3d')")),
			mcp.WithString("finish_date", mcp.Description("Finish date and time (e.g. '8/21/2026 3:57 PM', '2026-08-21', '+1w')")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("create_work_item", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 4. batch_create_tasks
	s.AddTool(
		mcp.NewTool(
			"batch_create_tasks",
			mcp.WithDescription("Creates multiple tasks under a parent work item in one operation (e.g. breaking down work, dividing effort across research and implementation)."),
			mcp.WithNumber("parent_id", mcp.Required(), mcp.Description("The parent work item ID under which tasks should be created")),
			mcp.WithArray("tasks", mcp.Required(), mcp.Description("List of tasks to create with title, description, assigned_to, original_estimate, remaining_work, start_date, target_date, finish_date")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("batch_create_tasks", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 5. batch_update_items
	s.AddTool(
		mcp.NewTool(
			"batch_update_items",
			mcp.WithDescription("Updates multiple work items simultaneously (e.g. updating estimates, states, assignees across child items)."),
			mcp.WithArray("updates", mcp.Required(), mcp.Description("List of item updates with work_item_id and fields to change")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("batch_update_items", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 6. delete_work_item
	s.AddTool(
		mcp.NewTool(
			"delete_work_item",
			mcp.WithDescription("Deletes a work item permanently from Azure DevOps and the local board."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the work item to delete")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("delete_work_item", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 7. add_comment
	s.AddTool(
		mcp.NewTool(
			"add_comment",
			mcp.WithDescription("Adds a discussion comment or note to an Azure DevOps work item."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the work item to comment on")),
			mcp.WithString("comment", mcp.Required(), mcp.Description("The comment text or markdown to add")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("add_comment", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 8. get_activity
	s.AddTool(
		mcp.NewTool(
			"get_activity",
			mcp.WithDescription("Retrieves the full chronological activity history (comments made, status/state changes, PRs raised/linked, estimate updates, and field changes) for a work item or hierarchy within a time range."),
			mcp.WithNumber("work_item_id", mcp.Description("The ID of the work item (e.g. 656842)")),
			mcp.WithString("time_range", mcp.Description("Time range filter: 'today', 'yesterday', 'past 2 days', 'past 3 days', 'past 7 days', 'past 30 days', 'all'")),
			mcp.WithNumber("since_days", mcp.Description("Optional number of past days to query")),
			mcp.WithBoolean("include_children", mcp.Description("If true, fetches and merges chronological activity across all child tasks, bugs, and PBIs")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_activity", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9. get_pull_requests
	s.AddTool(
		mcp.NewTool(
			"get_pull_requests",
			mcp.WithDescription("Retrieves details about Pull Requests linked to a work item, including status (active, completed), author, source/target branches, and timestamps."),
			mcp.WithNumber("work_item_id", mcp.Description("Work item ID to get linked PRs for")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_pull_requests", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9a. upload_pull_request_attachment
	s.AddTool(
		mcp.NewTool(
			"upload_pull_request_attachment",
			mcp.WithDescription("Uploads a file or image attachment to an Azure DevOps Git Pull Request and returns direct attachment URL and markdown embed snippet."),
			mcp.WithNumber("pull_request_id", mcp.Required(), mcp.Description("The ID of the Pull Request (e.g. 294732)")),
			mcp.WithString("file_path", mcp.Required(), mcp.Description("Local path of the file or image to upload (e.g. '/path/to/diagram.png')")),
			mcp.WithString("file_name", mcp.Description("Optional custom file name for the attachment in ADO (defaults to basename of file_path)")),
			mcp.WithString("repository_id", mcp.Description("Optional repository name or GUID (automatically discovered if omitted)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("upload_pull_request_attachment", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9b. update_pull_request
	s.AddTool(
		mcp.NewTool(
			"update_pull_request",
			mcp.WithDescription("Updates an Azure DevOps Git Pull Request's description, title, or draft status. Supports appending to existing description."),
			mcp.WithNumber("pull_request_id", mcp.Required(), mcp.Description("The ID of the Pull Request to update (e.g. 294732)")),
			mcp.WithString("description", mcp.Description("Complete updated markdown description for the PR (overwrites existing description unless append_description is used)")),
			mcp.WithString("append_description", mcp.Description("Markdown text to append to the existing description (e.g. newly uploaded attachment links or notes)")),
			mcp.WithString("title", mcp.Description("Updated title for the Pull Request")),
			mcp.WithString("repository_id", mcp.Description("Optional repository name or GUID (automatically discovered if omitted)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("update_pull_request", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9c. link_pull_request_work_item
	s.AddTool(
		mcp.NewTool(
			"link_pull_request_work_item",
			mcp.WithDescription("Links an Azure DevOps Git Pull Request to a Work Item bidirectionally (adds ArtifactLink relation to work item and associates PR)."),
			mcp.WithNumber("pull_request_id", mcp.Required(), mcp.Description("The ID of the Pull Request (e.g. 294732)")),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the Work Item to link to (e.g. 663674)")),
			mcp.WithString("repository_id", mcp.Description("Optional repository name or GUID (automatically discovered if omitted)")),
			mcp.WithString("comment", mcp.Description("Optional comment for the link relation (defaults to 'Pull Request')")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("link_pull_request_work_item", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9d. create_pull_request
	s.AddTool(
		mcp.NewTool(
			"create_pull_request",
			mcp.WithDescription("Creates a new Git Pull Request in Azure DevOps from a source branch to a target branch with optional title, description, draft status, reviewers, and work item links."),
			mcp.WithString("source_branch", mcp.Required(), mcp.Description("The source branch name (e.g. 'feature/login' or 'refs/heads/feature/login')")),
			mcp.WithString("title", mcp.Required(), mcp.Description("The title of the pull request")),
			mcp.WithString("target_branch", mcp.Description("Target branch name (defaults to 'main' or repository default branch if omitted)")),
			mcp.WithString("description", mcp.Description("The markdown description for the pull request")),
			mcp.WithBoolean("is_draft", mcp.Description("Whether to create the pull request as a draft (defaults to false)")),
			mcp.WithString("repository_id", mcp.Description("Optional Git repository name or GUID (automatically discovered if omitted)")),
			mcp.WithArray("reviewers", mcp.Description("Optional list of reviewer display names, emails, or GUIDs to add as optional reviewers")),
			mcp.WithArray("required_reviewers", mcp.Description("Optional list of reviewer display names, emails, or GUIDs to add as REQUIRED reviewers")),
			mcp.WithNumber("work_item_id", mcp.Description("Optional Work Item ID to link to this Pull Request upon creation")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("create_pull_request", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9e. add_pull_request_reviewers
	s.AddTool(
		mcp.NewTool(
			"add_pull_request_reviewers",
			mcp.WithDescription("Adds optional and/or required reviewers to an existing Azure DevOps Git Pull Request using team member display names, emails, or identity GUIDs."),
			mcp.WithNumber("pull_request_id", mcp.Required(), mcp.Description("The ID of the Pull Request (e.g. 294732)")),
			mcp.WithArray("reviewers", mcp.Description("List of reviewer display names, emails, or GUIDs to add as optional reviewers")),
			mcp.WithArray("required_reviewers", mcp.Description("List of reviewer display names, emails, or GUIDs to add as REQUIRED reviewers")),
			mcp.WithString("repository_id", mcp.Description("Optional Git repository name or GUID (automatically discovered if omitted)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("add_pull_request_reviewers", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9f. get_pull_request_details
	s.AddTool(
		mcp.NewTool(
			"get_pull_request_details",
			mcp.WithDescription("Retrieves complete metadata and details of an Azure DevOps Pull Request, including description, status, author, merge status, reviewers with votes and required status, uploaded attachments, and linked work items."),
			mcp.WithNumber("pull_request_id", mcp.Required(), mcp.Description("The ID of the Pull Request (e.g. 294732)")),
			mcp.WithString("repository_id", mcp.Description("Optional Git repository name or GUID (automatically discovered if omitted)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_pull_request_details", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9g. get_pull_request_comments
	s.AddTool(
		mcp.NewTool(
			"get_pull_request_comments",
			mcp.WithDescription("Retrieves discussion comments and review threads for an Azure DevOps Git Pull Request, including inline code comments, author details, and thread status."),
			mcp.WithNumber("pull_request_id", mcp.Required(), mcp.Description("The ID of the Pull Request (e.g. 294732)")),
			mcp.WithString("repository_id", mcp.Description("Optional Git repository name or GUID (automatically discovered if omitted)")),
			mcp.WithBoolean("include_system", mcp.Description("Whether to include automated system comments and events (defaults to false)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_pull_request_comments", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9h. add_pull_request_comment
	s.AddTool(
		mcp.NewTool(
			"add_pull_request_comment",
			mcp.WithDescription("Adds a review comment or thread to an Azure DevOps Git Pull Request, supporting both overall PR comments and inline review comments on specific files and code line selections."),
			mcp.WithNumber("pull_request_id", mcp.Required(), mcp.Description("The ID of the Pull Request (e.g. 294732)")),
			mcp.WithString("content", mcp.Required(), mcp.Description("The review comment text or markdown description to post")),
			mcp.WithString("file_path", mcp.Description("Optional path to the file in repository for inline code review comments (e.g. '/pkg/auth/login.go' or 'src/index.ts')")),
			mcp.WithNumber("start_line", mcp.Description("Optional starting line number for inline code selection (1-indexed)")),
			mcp.WithNumber("end_line", mcp.Description("Optional ending line number for inline code selection (1-indexed, defaults to start_line)")),
			mcp.WithNumber("start_offset", mcp.Description("Optional starting character column/offset on start_line (1-indexed, defaults to 1)")),
			mcp.WithNumber("end_offset", mcp.Description("Optional ending character column/offset on end_line (1-indexed, defaults to 1)")),
			mcp.WithString("side", mcp.Description("Which side of the diff the comment belongs to: 'right' (modified/new code, default) or 'left' (original/base code)")),
			mcp.WithString("status", mcp.Description("Initial status of the review comment thread: 'active' (default, unresolved), 'fixed' (resolved), 'wontFix', 'closed', 'byDesign', 'pending'")),
			mcp.WithString("repository_id", mcp.Description("Optional Git repository name or GUID (automatically discovered if omitted)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("add_pull_request_comment", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9h-alias. create_pull_request_comment
	s.AddTool(
		mcp.NewTool(
			"create_pull_request_comment",
			mcp.WithDescription("Alias for add_pull_request_comment. Adds a review comment or thread to an Azure DevOps Git Pull Request with optional file and code selection."),
			mcp.WithNumber("pull_request_id", mcp.Required(), mcp.Description("The ID of the Pull Request (e.g. 294732)")),
			mcp.WithString("content", mcp.Required(), mcp.Description("The review comment text or markdown description to post")),
			mcp.WithString("file_path", mcp.Description("Optional path to the file in repository for inline code review comments (e.g. '/pkg/auth/login.go' or 'src/index.ts')")),
			mcp.WithNumber("start_line", mcp.Description("Optional starting line number for inline code selection (1-indexed)")),
			mcp.WithNumber("end_line", mcp.Description("Optional ending line number for inline code selection (1-indexed, defaults to start_line)")),
			mcp.WithNumber("start_offset", mcp.Description("Optional starting character column/offset on start_line (1-indexed, defaults to 1)")),
			mcp.WithNumber("end_offset", mcp.Description("Optional ending character column/offset on end_line (1-indexed, defaults to 1)")),
			mcp.WithString("side", mcp.Description("Which side of the diff the comment belongs to: 'right' (modified/new code, default) or 'left' (original/base code)")),
			mcp.WithString("status", mcp.Description("Initial status of the review comment thread: 'active' (default, unresolved), 'fixed' (resolved), 'wontFix', 'closed', 'byDesign', 'pending'")),
			mcp.WithString("repository_id", mcp.Description("Optional Git repository name or GUID (automatically discovered if omitted)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("add_pull_request_comment", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 9i. reply_pull_request_comment
	s.AddTool(
		mcp.NewTool(
			"reply_pull_request_comment",
			mcp.WithDescription("Replies to an Azure DevOps Git Pull Request review comment or thread and optionally updates the thread resolution status (e.g. mark resolved or active/unresolved)."),
			mcp.WithNumber("pull_request_id", mcp.Required(), mcp.Description("The ID of the Pull Request (e.g. 294732)")),
			mcp.WithNumber("thread_id", mcp.Description("The ID of the discussion thread to reply to or update (optional if comment_id is provided)")),
			mcp.WithNumber("comment_id", mcp.Description("The ID of the specific comment being replied to (if thread_id is omitted, it will automatically find the corresponding thread)")),
			mcp.WithString("content", mcp.Description("The reply text or markdown comment to post to the thread")),
			mcp.WithString("status", mcp.Description("Optional thread status update: 'fixed' (resolved), 'active' (unresolved), 'wontFix', 'closed', 'byDesign', 'pending'")),
			mcp.WithBoolean("resolved", mcp.Description("Optional shorthand boolean to resolve (true -> 'fixed') or unresolve (false -> 'active') the thread")),
			mcp.WithString("repository_id", mcp.Description("Optional Git repository name or GUID (automatically discovered if omitted)")),
			mcp.WithNumber("parent_comment_id", mcp.Description("Optional explicit parent comment ID within the thread")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("reply_pull_request_comment", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 10. get_comments
	s.AddTool(
		mcp.NewTool(
			"get_comments",
			mcp.WithDescription("Retrieves all discussion comments for a work item with author, timestamp, text, and mentions in chronological order."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The work item ID to fetch discussion comments for")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_comments", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 11. get_board_summary
	s.AddTool(
		mcp.NewTool(
			"get_board_summary",
			mcp.WithDescription("Retrieves a complete summary of all tracked root items, queries, active tab hierarchy, total estimates, and rollup statistics."),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if database == nil {
				return mcp.NewToolResultError("database is not connected"), nil
			}

			roots, err := database.GetTrackedRoots()
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to get tracked roots: %v", err)), nil
			}

			queries, _ := database.GetTrackedQueries()
			items, _ := database.GetAllWorkItems()

			tree := db.BuildTree(items, roots)
			flattened := db.FlattenTree(tree, "")

			var totalOrig, totalRem, totalComp, totalEffort float64
			for _, rec := range items {
				totalOrig += rec.OriginalEstimate
				totalRem += rec.RemainingWork
				totalComp += rec.CompletedWork
				totalEffort += rec.Effort
			}

			summary := map[string]any{
				"total_tracked_roots":   len(roots),
				"total_tracked_queries": len(queries),
				"total_work_items":      len(items),
				"total_tree_rows":       len(flattened),
				"total_estimates": map[string]float64{
					"original_estimate_hours": totalOrig,
					"remaining_work_hours":    totalRem,
					"completed_work_hours":    totalComp,
					"effort_points":           totalEffort,
				},
				"tracked_roots":   roots,
				"tracked_queries": queries,
			}
			return mcp.NewToolResultText(toJSONString(summary)), nil
		},
	)

	// 12. get_settings
	s.AddTool(
		mcp.NewTool(
			"get_settings",
			mcp.WithDescription("Retrieves application settings and board preferences (Incomplete criteria, applicable item types, sync interval, calculation preferences)."),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_settings", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 13. update_settings
	s.AddTool(
		mcp.NewTool(
			"update_settings",
			mcp.WithDescription("Updates application settings and board preferences (Incomplete View criteria, applicable item types/statuses, auto-sync interval, effort/estimate calculation modes)."),
			mcp.WithNumber("auto_sync_interval_seconds", mcp.Description("Interval in seconds for automatic background syncing. 0 to disable.")),
			mcp.WithBoolean("auto_calc_remaining", mcp.Description("Automatically calculate Remaining Work as (Original Estimate - Completed Work)")),
			mcp.WithBoolean("effort_same_as_estimate", mcp.Description("Keep Effort (Story Points) identical to Original Estimate")),
			mcp.WithBoolean("auto_set_finish_date", mcp.Description("Automatically set Finish Date to today/now when changing status to Done / Completed")),
			mcp.WithBoolean("incomplete_criteria_effort", mcp.Description("Require Effort / Story Points on applicable items")),
			mcp.WithBoolean("incomplete_criteria_estimate", mcp.Description("Require Original Estimate on applicable items")),
			mcp.WithBoolean("incomplete_criteria_start_date", mcp.Description("Require Start Date on applicable items")),
			mcp.WithBoolean("incomplete_criteria_target_date", mcp.Description("Require Target Date and detect overdue items")),
			mcp.WithBoolean("incomplete_criteria_finish_date", mcp.Description("Require Finish Date")),
			mcp.WithBoolean("incomplete_criteria_tag_regex", mcp.Description("Require item to have at least one tag matching configured tag regex rules")),
			mcp.WithBoolean("incomplete_criteria_activity", mcp.Description("Require Activity on applicable items")),
			mcp.WithString("tag_regex", mcp.Description("Comma or semicolon-separated regex patterns for child tag inheritance and incomplete tag validation (e.g. '^BE-.*, ^FE-.*')")),
			mcp.WithArray("tag_regexes", mcp.Description("List of regex rules for child tag inheritance and incomplete tag validation (e.g. ['^BE-.*', '^FE-.*', '^release-.*'])")),
			mcp.WithArray("incomplete_types", mcp.Description("List of work item types to check (e.g. ['Task', 'Bug'])")),
			mcp.WithArray("incomplete_statuses", mcp.Description("List of statuses to check (e.g. ['Active', 'In Progress'])")),
			mcp.WithArray("incomplete_date_statuses", mcp.Description("List of statuses where dates are required")),
			mcp.WithString("gemini_model", mcp.Description("Gemini AI model name")),
			mcp.WithString("open_in_browser_cmd", mcp.Description("Custom shell command to open links in browser")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("update_settings", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 14. track_work_item
	s.AddTool(
		mcp.NewTool(
			"track_work_item",
			mcp.WithDescription("Tracks an Azure DevOps work item as a new root tab on the board, fetching its complete child hierarchy and saving it locally."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the work item to track as a root (e.g. 666236)")),
			mcp.WithString("custom_title", mcp.Description("Optional custom title / alias for the tracked root tab")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("track_work_item", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			// Trigger live UI refresh
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 15. untrack_work_item
	s.AddTool(
		mcp.NewTool(
			"untrack_work_item",
			mcp.WithDescription("Untracks and removes a root work item tab from the local board."),
			mcp.WithNumber("work_item_id", mcp.Required(), mcp.Description("The ID of the tracked root work item to remove")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("untrack_work_item", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			// Trigger live UI refresh
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 16. track_query
	s.AddTool(
		mcp.NewTool(
			"track_query",
			mcp.WithDescription("Tracks an Azure DevOps query (by GUID, URL, or Path) as a query tab on the board, executing it and importing all root work items."),
			mcp.WithString("query_id_or_url", mcp.Required(), mcp.Description("The ADO Query ID (GUID) or full query web URL")),
			mcp.WithString("name", mcp.Description("Optional custom display name for the tracked query tab")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("track_query", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			// Trigger live UI refresh
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 17. untrack_query
	s.AddTool(
		mcp.NewTool(
			"untrack_query",
			mcp.WithDescription("Untracks and removes a query tab and its associated roots from the local board."),
			mcp.WithString("query_id", mcp.Required(), mcp.Description("The ID/GUID of the tracked query to remove")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("untrack_query", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			// Trigger live UI refresh
			_, _ = ipc.SendCommand(ctx, ipc.Request{Command: ipc.CmdRefresh})
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 18. get_users
	s.AddTool(
		mcp.NewTool(
			"get_users",
			mcp.WithDescription("Lists or searches project team members, users, and assignees with their display names and email addresses."),
			mcp.WithString("query", mcp.Description("Optional search query to filter users by name or email (e.g. 'alice', 'john')")),
			mcp.WithNumber("limit", mcp.Description("Maximum number of users to return (default: 50)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("get_users", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)

	// 19. open_in_browser
	s.AddTool(
		mcp.NewTool(
			"open_in_browser",
			mcp.WithDescription("Opens a work item or custom ADO URL in the default web browser."),
			mcp.WithNumber("work_item_id", mcp.Description("The ID of the work item to open in the browser")),
			mcp.WithString("url", mcp.Description("Direct URL to open in browser (optional if work_item_id is provided)")),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			exec := getExecutor()
			res, err := exec.Execute("open_in_browser", request.GetArguments())
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(toJSONString(res)), nil
		},
	)
}
