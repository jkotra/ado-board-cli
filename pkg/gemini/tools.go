package gemini

import "google.golang.org/genai"

// GetAdoTools returns the GenAI tool specifications for Azure DevOps management
func GetAdoTools() []*genai.Tool {
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "get_work_item_details",
					Description: "Retrieves full metadata, comments, and relationships for a specific Azure DevOps work item by ID.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the work item (e.g., 666236)",
							},
						},
						Required: []string{"work_item_id"},
					},
				},
				{
					Name:        "get_children",
					Description: "Retrieves all children or recursive descendants under a work item (e.g. Feature, User Story, Epic, Bug) with hierarchy trees, rollup estimates, and status breakdowns.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the parent/root work item (e.g. 656842)",
							},
							"recursive": {
								Type:        genai.TypeBoolean,
								Description: "If true (default), returns all recursive descendants (subtrees, tasks under PBIs). If false, returns only direct immediate children.",
							},
							"format": {
								Type:        genai.TypeString,
								Description: "Format of output: 'tree' (nested tree with rollups), 'flat' (list of work items), or 'both'. Default is 'tree'.",
							},
							"refresh": {
								Type:        genai.TypeBoolean,
								Description: "If true, fetches fresh data directly from Azure DevOps before building hierarchy.",
							},
						},
						Required: []string{"work_item_id"},
					},
				},
				{
					Name:        "get_hierarchy",
					Description: "Retrieves the complete ancestor chain and descendant subtree hierarchy for a work item, including rollup estimates and status/type breakdowns.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the work item (e.g. 656842)",
							},
							"include_ancestors": {
								Type:        genai.TypeBoolean,
								Description: "If true (default), includes the ancestor chain up to the top root item.",
							},
							"refresh": {
								Type:        genai.TypeBoolean,
								Description: "If true, fetches fresh data directly from Azure DevOps before building hierarchy.",
							},
						},
						Required: []string{"work_item_id"},
					},
				},
				{
					Name:        "update_work_item",
					Description: "Updates one or more fields on an existing Azure DevOps work item (state/status, estimates, assignee, title, description).",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the work item to update",
							},
							"state": {
								Type:        genai.TypeString,
								Description: "New state (e.g. 'New', 'Active', 'In Progress', 'Done', 'Closed', 'Resolved', 'Approved', 'Committed', 'Removed')",
							},
							"title": {
								Type:        genai.TypeString,
								Description: "Updated title for the work item",
							},
							"assigned_to": {
								Type:        genai.TypeString,
								Description: "Email address or full display name of the person to assign to",
							},
							"original_estimate": {
								Type:        genai.TypeNumber,
								Description: "Original estimate in hours",
							},
							"remaining_work": {
								Type:        genai.TypeNumber,
								Description: "Remaining work in hours",
							},
							"completed_work": {
								Type:        genai.TypeNumber,
								Description: "Completed work in hours",
							},
							"effort": {
								Type:        genai.TypeNumber,
								Description: "Story points or effort value",
							},
							"start_date": {
								Type:        genai.TypeString,
								Description: "Start date and time (e.g. '8/18/2026 12:00 AM', '2026-08-18', 'today', 'tomorrow 9am'). Empty string to clear.",
							},
							"target_date": {
								Type:        genai.TypeString,
								Description: "Target / Due date and time (e.g. '8/19/2026 8:01 AM', '2026-08-19', '+3d'). Empty string to clear.",
							},
							"finish_date": {
								Type:        genai.TypeString,
								Description: "Finish date and time (e.g. '8/21/2026 3:57 PM', '2026-08-21', '+1w'). Empty string to clear.",
							},
							"description": {
								Type:        genai.TypeString,
								Description: "Updated description text or HTML",
							},
							"activity": {
								Type:        genai.TypeString,
								Description: "Task activity type (e.g. 'Development', 'Testing', 'Design', 'Documentation', 'Requirements', 'Deployment'). Empty string to clear.",
							},
							"tags": {
								Type:        genai.TypeString,
								Description: "Tags associated with work item (semicolon or comma separated, e.g. 'backend; api; v2'). Empty string to clear.",
							},
							"iteration_path": {
								Type:        genai.TypeString,
								Description: "ADO iteration sprint path (e.g. 'Project\\Sprint 1')",
							},
							"area_path": {
								Type:        genai.TypeString,
								Description: "ADO area team path (e.g. 'Project\\Backend Team')",
							},
						},
						Required: []string{"work_item_id"},
					},
				},
				{
					Name:        "add_comment",
					Description: "Adds a discussion comment or note to an Azure DevOps work item.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the work item to comment on",
							},
							"comment": {
								Type:        genai.TypeString,
								Description: "The comment text or markdown to add",
							},
						},
						Required: []string{"work_item_id", "comment"},
					},
				},
				{
					Name:        "create_work_item",
					Description: "Creates a single new work item (e.g. Task, Bug, Product Backlog Item, Feature) linked to an optional parent item.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"parent_id": {
								Type:        genai.TypeInteger,
								Description: "Parent work item ID to link under (e.g. parent PBI or Feature ID). 0 if root level.",
							},
							"work_item_type": {
								Type:        genai.TypeString,
								Description: "Work item type, such as 'Task', 'Bug', 'Product Backlog Item', 'Feature', or 'User Story'",
							},
							"title": {
								Type:        genai.TypeString,
								Description: "Title of the new work item",
							},
							"description": {
								Type:        genai.TypeString,
								Description: "Description or repro steps for the work item",
							},
							"assigned_to": {
								Type:        genai.TypeString,
								Description: "Email address or full display name of assignee",
							},
							"original_estimate": {
								Type:        genai.TypeNumber,
								Description: "Original estimate in hours",
							},
							"remaining_work": {
								Type:        genai.TypeNumber,
								Description: "Remaining work in hours",
							},
							"start_date": {
								Type:        genai.TypeString,
								Description: "Start date and time (e.g. '8/18/2026 12:00 AM', '2026-08-18', 'today', 'tomorrow 9am')",
							},
							"target_date": {
								Type:        genai.TypeString,
								Description: "Target / Due date and time (e.g. '8/19/2026 8:01 AM', '2026-08-19', '+3d')",
							},
							"finish_date": {
								Type:        genai.TypeString,
								Description: "Finish date and time (e.g. '8/21/2026 3:57 PM', '2026-08-21', '+1w')",
							},
							"effort": {
								Type:        genai.TypeNumber,
								Description: "Story points or effort value",
							},
							"tags": {
								Type:        genai.TypeString,
								Description: "Tags associated with work item (semicolon or comma separated, e.g. 'backend; api; v2')",
							},
							"activity": {
								Type:        genai.TypeString,
								Description: "Task activity type (e.g. 'Development', 'Testing', 'Design', 'Documentation', 'Requirements', 'Deployment')",
							},
							"iteration_path": {
								Type:        genai.TypeString,
								Description: "ADO iteration sprint path (e.g. 'Project\\Sprint 1')",
							},
							"area_path": {
								Type:        genai.TypeString,
								Description: "ADO area team path (e.g. 'Project\\Backend Team')",
							},
						},
						Required: []string{"work_item_type", "title"},
					},
				},
				{
					Name:        "batch_create_tasks",
					Description: "Creates multiple tasks under a parent work item in one operation (e.g. breaking down work, dividing effort across research and implementation).",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"parent_id": {
								Type:        genai.TypeInteger,
								Description: "The parent work item ID under which tasks should be created",
							},
							"tasks": {
								Type:        genai.TypeArray,
								Description: "List of tasks to create",
								Items: &genai.Schema{
									Type: genai.TypeObject,
									Properties: map[string]*genai.Schema{
										"title": {
											Type:        genai.TypeString,
											Description: "Title of the task",
										},
										"description": {
											Type:        genai.TypeString,
											Description: "Description of the task",
										},
										"assigned_to": {
											Type:        genai.TypeString,
											Description: "Email or name of assignee",
										},
										"activity": {
											Type:        genai.TypeString,
											Description: "Task activity type (e.g. 'Development', 'Testing', 'Design', 'Documentation', 'Requirements', 'Deployment')",
										},
										"original_estimate": {
											Type:        genai.TypeNumber,
											Description: "Original estimate in hours",
										},
										"remaining_work": {
											Type:        genai.TypeNumber,
											Description: "Remaining work in hours",
										},
										"effort": {
											Type:        genai.TypeNumber,
											Description: "Story points or effort value",
										},
										"tags": {
											Type:        genai.TypeString,
											Description: "Tags associated with task",
										},
										"iteration_path": {
											Type:        genai.TypeString,
											Description: "ADO iteration sprint path",
										},
										"area_path": {
											Type:        genai.TypeString,
											Description: "ADO area team path",
										},
										"start_date": {
											Type:        genai.TypeString,
											Description: "Start date and time (e.g. '8/18/2026 12:00 AM', '2026-08-18', 'today')",
										},
										"target_date": {
											Type:        genai.TypeString,
											Description: "Target / Due date and time (e.g. '8/19/2026 8:01 AM', '+3d')",
										},
										"finish_date": {
											Type:        genai.TypeString,
											Description: "Finish date and time (e.g. '8/21/2026 3:57 PM', '+1w')",
										},
									},
									Required: []string{"title"},
								},
							},
						},
						Required: []string{"parent_id", "tasks"},
					},
				},
				{
					Name:        "batch_update_items",
					Description: "Updates multiple work items simultaneously (e.g., updating a parent PBI's estimate to the sum of child task estimates, or updating multiple items at once).",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"updates": {
								Type:        genai.TypeArray,
								Description: "List of item updates to apply",
								Items: &genai.Schema{
									Type: genai.TypeObject,
									Properties: map[string]*genai.Schema{
										"work_item_id": {
											Type:        genai.TypeInteger,
											Description: "ID of the work item to update",
										},
										"state": {
											Type:        genai.TypeString,
											Description: "New state/status",
										},
										"title": {
											Type:        genai.TypeString,
											Description: "New title",
										},
										"description": {
											Type:        genai.TypeString,
											Description: "New description",
										},
										"assigned_to": {
											Type:        genai.TypeString,
											Description: "Assignee email or name",
										},
										"activity": {
											Type:        genai.TypeString,
											Description: "Task activity type (e.g. 'Development', 'Testing', 'Design', 'Documentation', 'Requirements', 'Deployment'). Empty string to clear.",
										},
										"original_estimate": {
											Type:        genai.TypeNumber,
											Description: "Original estimate in hours",
										},
										"remaining_work": {
											Type:        genai.TypeNumber,
											Description: "Remaining work in hours",
										},
										"completed_work": {
											Type:        genai.TypeNumber,
											Description: "Completed work in hours",
										},
										"effort": {
											Type:        genai.TypeNumber,
											Description: "Effort / Story points",
										},
										"tags": {
											Type:        genai.TypeString,
											Description: "Tags (semicolon or comma separated). Empty string to clear.",
										},
										"iteration_path": {
											Type:        genai.TypeString,
											Description: "ADO iteration sprint path",
										},
										"area_path": {
											Type:        genai.TypeString,
											Description: "ADO area team path",
										},
										"start_date": {
											Type:        genai.TypeString,
											Description: "Start date and time (e.g. '8/18/2026 12:00 AM', '2026-08-18', 'today')",
										},
										"target_date": {
											Type:        genai.TypeString,
											Description: "Target / Due date and time (e.g. '8/19/2026 8:01 AM', '+3d')",
										},
										"finish_date": {
											Type:        genai.TypeString,
											Description: "Finish date and time (e.g. '8/21/2026 3:57 PM', '+1w')",
										},
									},
									Required: []string{"work_item_id"},
								},
							},
						},
						Required: []string{"updates"},
					},
				},
				{
					Name:        "delete_work_item",
					Description: "Deletes a work item permanently from Azure DevOps and the local board.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the work item to delete",
							},
						},
						Required: []string{"work_item_id"},
					},
				},
				{
					Name:        "get_activity",
					Description: "Retrieves the full chronological activity history (comments made, status/state changes, PRs raised/linked, estimate updates, and field changes) for a work item, its hierarchy, or the board within a specific time range ('today', 'yesterday', 'past 3 days', 'past 7 days', 'past 30 days', or 'all').",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the work item (e.g. 656842). 0 or omitted uses the currently selected item in context.",
							},
							"time_range": {
								Type:        genai.TypeString,
								Description: "Time range filter: 'today', 'yesterday', 'past 2 days', 'past 3 days', 'past 7 days', 'past 30 days', 'all'.",
							},
							"since_days": {
								Type:        genai.TypeInteger,
								Description: "Optional number of past days to query (e.g. 1 for past 24h, 3 for past 3 days). Overrides time_range if specified.",
							},
							"include_children": {
								Type:        genai.TypeBoolean,
								Description: "If true, fetches and merges chronological activity across all child tasks, bugs, and PBIs under this parent item.",
							},
						},
					},
				},
				{
					Name:        "get_pull_requests",
					Description: "Retrieves details about Pull Requests linked to a work item or across the project, including status (active, completed), author, source/target branches, and timestamps.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "Optional work item ID to get linked PRs for. 0 or omitted gets PRs for the currently selected item.",
							},
						},
					},
				},
				{
					Name:        "upload_pull_request_attachment",
					Description: "Uploads a file or image attachment to an Azure DevOps Git Pull Request and returns direct attachment URL and markdown embed snippet.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"pull_request_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the Pull Request (e.g. 294732).",
							},
							"file_path": {
								Type:        genai.TypeString,
								Description: "Local file path of the image or document to upload (e.g. '/path/to/screenshot.png' or 'diagram.svg').",
							},
							"file_name": {
								Type:        genai.TypeString,
								Description: "Optional custom name for the uploaded file in Azure DevOps (defaults to the basename of file_path).",
							},
							"repository_id": {
								Type:        genai.TypeString,
								Description: "Optional Git repository name or GUID (if omitted, automatically discovered from PR).",
							},
						},
						Required: []string{"pull_request_id", "file_path"},
					},
				},
				{
					Name:        "update_pull_request",
					Description: "Updates an Azure DevOps Git Pull Request's description, title, or draft status. Supports appending to description.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"pull_request_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the Pull Request to update (e.g. 294732).",
							},
							"description": {
								Type:        genai.TypeString,
								Description: "Complete updated markdown description for the PR (overwrites existing description unless append_description is used).",
							},
							"append_description": {
								Type:        genai.TypeString,
								Description: "Markdown text to append to the existing description (e.g. newly uploaded attachment links or notes) without replacing current text.",
							},
							"title": {
								Type:        genai.TypeString,
								Description: "Updated title for the Pull Request.",
							},
							"repository_id": {
								Type:        genai.TypeString,
								Description: "Optional Git repository name or GUID (if omitted, automatically discovered).",
							},
						},
						Required: []string{"pull_request_id"},
					},
				},
				{
					Name:        "link_pull_request_work_item",
					Description: "Links an Azure DevOps Git Pull Request to a Work Item bidirectionally (adds ArtifactLink relation to the work item and associates PR).",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"pull_request_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the Pull Request (e.g. 294732).",
							},
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the Work Item to link to (e.g. 663674).",
							},
							"repository_id": {
								Type:        genai.TypeString,
								Description: "Optional Git repository name or GUID (if omitted, automatically discovered).",
							},
							"comment": {
								Type:        genai.TypeString,
								Description: "Optional comment for the link relation (defaults to 'Pull Request').",
							},
						},
						Required: []string{"pull_request_id", "work_item_id"},
					},
				},
				{
					Name:        "create_pull_request",
					Description: "Creates a new Git Pull Request in Azure DevOps from a source branch to a target branch with optional title, description, draft status, reviewers, and work item links.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"source_branch": {
								Type:        genai.TypeString,
								Description: "The source branch name (e.g. 'feature/user-auth' or 'refs/heads/feature/user-auth').",
							},
							"title": {
								Type:        genai.TypeString,
								Description: "The title of the pull request.",
							},
							"target_branch": {
								Type:        genai.TypeString,
								Description: "The target branch name (defaults to 'main' or repository default branch if omitted).",
							},
							"description": {
								Type:        genai.TypeString,
								Description: "The markdown description for the pull request.",
							},
							"is_draft": {
								Type:        genai.TypeBoolean,
								Description: "Whether to create the pull request as a draft (defaults to false).",
							},
							"repository_id": {
								Type:        genai.TypeString,
								Description: "Optional Git repository name or GUID (if omitted, automatically discovered).",
							},
							"reviewers": {
								Type:        genai.TypeArray,
								Description: "Optional list of reviewer display names, email addresses, or identity GUIDs to add as optional reviewers.",
								Items: &genai.Schema{
									Type: genai.TypeString,
								},
							},
							"required_reviewers": {
								Type:        genai.TypeArray,
								Description: "Optional list of reviewer display names, email addresses, or identity GUIDs to add as REQUIRED reviewers.",
								Items: &genai.Schema{
									Type: genai.TypeString,
								},
							},
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "Optional Work Item ID to link to this Pull Request upon creation.",
							},
						},
						Required: []string{"source_branch", "title"},
					},
				},
				{
					Name:        "add_pull_request_reviewers",
					Description: "Adds optional and/or required reviewers to an existing Azure DevOps Git Pull Request using team member display names, emails, or identity GUIDs.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"pull_request_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the Pull Request (e.g. 294732).",
							},
							"reviewers": {
								Type:        genai.TypeArray,
								Description: "List of reviewer display names, email addresses, or GUIDs to add as optional reviewers.",
								Items: &genai.Schema{
									Type: genai.TypeString,
								},
							},
							"required_reviewers": {
								Type:        genai.TypeArray,
								Description: "List of reviewer display names, email addresses, or GUIDs to add as REQUIRED reviewers.",
								Items: &genai.Schema{
									Type: genai.TypeString,
								},
							},
							"repository_id": {
								Type:        genai.TypeString,
								Description: "Optional Git repository name or GUID (if omitted, automatically discovered).",
							},
						},
						Required: []string{"pull_request_id"},
					},
				},
				{
					Name:        "get_pull_request_details",
					Description: "Retrieves complete metadata and details of an Azure DevOps Pull Request, including description, status, author, merge status, reviewers with votes and required status, uploaded attachments, and linked work items.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"pull_request_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the Pull Request (e.g. 294732).",
							},
							"repository_id": {
								Type:        genai.TypeString,
								Description: "Optional Git repository name or GUID (if omitted, automatically discovered).",
							},
						},
						Required: []string{"pull_request_id"},
					},
				},
				{
					Name:        "get_pull_request_comments",
					Description: "Retrieves discussion comments and review threads for an Azure DevOps Git Pull Request, including inline code comments, author details, and thread status.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"pull_request_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the Pull Request (e.g. 294732).",
							},
							"repository_id": {
								Type:        genai.TypeString,
								Description: "Optional Git repository name or GUID (if omitted, automatically discovered).",
							},
							"include_system": {
								Type:        genai.TypeBoolean,
								Description: "Whether to include automated system comments and events (defaults to false to focus on human comments).",
							},
						},
						Required: []string{"pull_request_id"},
					},
				},
				{
					Name:        "add_pull_request_comment",
					Description: "Adds a review comment or thread to an Azure DevOps Git Pull Request, supporting both overall PR comments and inline review comments on specific files and code line selections.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"pull_request_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the Pull Request (e.g. 294732).",
							},
							"content": {
								Type:        genai.TypeString,
								Description: "The review comment text or markdown description to post.",
							},
							"file_path": {
								Type:        genai.TypeString,
								Description: "Optional path to the file in repository for inline code review comments (e.g. '/pkg/auth/login.go' or 'src/index.ts'). If omitted, creates a general PR-level comment.",
							},
							"start_line": {
								Type:        genai.TypeInteger,
								Description: "Optional starting line number for inline code selection (1-indexed).",
							},
							"end_line": {
								Type:        genai.TypeInteger,
								Description: "Optional ending line number for inline code selection (1-indexed, defaults to start_line).",
							},
							"start_offset": {
								Type:        genai.TypeInteger,
								Description: "Optional starting character column/offset on start_line (1-indexed, defaults to 1).",
							},
							"end_offset": {
								Type:        genai.TypeInteger,
								Description: "Optional ending character column/offset on end_line (1-indexed, defaults to 1).",
							},
							"side": {
								Type:        genai.TypeString,
								Description: "Which side of the diff the comment belongs to: 'right' (modified/new code, default) or 'left' (original/base code).",
							},
							"status": {
								Type:        genai.TypeString,
								Description: "Initial status of the review comment thread: 'active' (default, unresolved), 'fixed' (resolved), 'wontFix', 'closed', 'byDesign', 'pending'.",
							},
							"repository_id": {
								Type:        genai.TypeString,
								Description: "Optional Git repository name or GUID (if omitted, automatically discovered).",
							},
						},
						Required: []string{"pull_request_id", "content"},
					},
				},
				{
					Name:        "reply_pull_request_comment",
					Description: "Replies to an Azure DevOps Git Pull Request review comment or thread and optionally updates the thread resolution status (e.g. mark resolved or active/unresolved).",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"pull_request_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the Pull Request (e.g. 294732).",
							},
							"thread_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the discussion thread to reply to or update (optional if comment_id is provided).",
							},
							"comment_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the specific comment being replied to (if thread_id is omitted, it will automatically find the corresponding thread).",
							},
							"content": {
								Type:        genai.TypeString,
								Description: "The reply text or markdown comment to post to the thread.",
							},
							"status": {
								Type:        genai.TypeString,
								Description: "Optional thread status update: 'fixed' (resolved), 'active' (unresolved), 'wontFix', 'closed', 'byDesign', 'pending'.",
							},
							"resolved": {
								Type:        genai.TypeBoolean,
								Description: "Optional shorthand boolean to resolve (true -> 'fixed') or unresolve (false -> 'active') the thread.",
							},
							"repository_id": {
								Type:        genai.TypeString,
								Description: "Optional Git repository name or GUID (if omitted, automatically discovered).",
							},
							"parent_comment_id": {
								Type:        genai.TypeInteger,
								Description: "Optional explicit parent comment ID within the thread.",
							},
						},
						Required: []string{"pull_request_id"},
					},
				},
				{
					Name:        "get_comments",
					Description: "Retrieves all discussion comments for a work item with author, timestamp, text, and mentions in chronological order.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The work item ID to fetch discussion comments for.",
							},
						},
						Required: []string{"work_item_id"},
					},
				},
				{
					Name:        "get_settings",
					Description: "Retrieves all application configuration settings and board preferences, including Incomplete View criteria, applicable item types, applicable item statuses, applicable date statuses, auto-sync interval, effort calculation preferences, and Gemini AI configuration.",
					Parameters: &genai.Schema{
						Type:       genai.TypeObject,
						Properties: map[string]*genai.Schema{},
					},
				},
				{
					Name:        "update_settings",
					Description: "Updates one or more application settings and board preferences (Incomplete View criteria, applicable item types, applicable item statuses, applicable date statuses, auto-sync interval, effort/estimate calculation modes, Gemini model, etc.). All changes are immediately persisted to the SQLite database and active application config.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"auto_sync_interval_seconds": {
								Type:        genai.TypeInteger,
								Description: "Interval in seconds for automatic background syncing (e.g. 120, 60). Set to 0 to disable auto-sync.",
							},
							"auto_calc_remaining": {
								Type:        genai.TypeBoolean,
								Description: "Automatically calculate Remaining Work as (Original Estimate - Completed Work).",
							},
							"effort_same_as_estimate": {
								Type:        genai.TypeBoolean,
								Description: "Keep Effort (Story Points) identical to Original Estimate.",
							},
							"auto_set_finish_date": {
								Type:        genai.TypeBoolean,
								Description: "Automatically set Finish Date to today/now when changing status to Done / Completed.",
							},
							"incomplete_criteria_effort": {
								Type:        genai.TypeBoolean,
								Description: "Incomplete View: Require Effort / Story Points on applicable items.",
							},
							"incomplete_criteria_estimate": {
								Type:        genai.TypeBoolean,
								Description: "Incomplete View: Require Original Estimate on applicable items.",
							},
							"incomplete_criteria_start_date": {
								Type:        genai.TypeBoolean,
								Description: "Incomplete View: Require Start Date on applicable items.",
							},
							"incomplete_criteria_target_date": {
								Type:        genai.TypeBoolean,
								Description: "Incomplete View: Require Target Date and detect overdue items.",
							},
							"incomplete_criteria_finish_date": {
								Type:        genai.TypeBoolean,
								Description: "Incomplete View: Require Finish Date.",
							},
							"incomplete_types": {
								Type:        genai.TypeArray,
								Description: "Incomplete View: List of work item types to check (e.g. ['Task', 'Bug'] or ['[All]']).",
								Items: &genai.Schema{
									Type: genai.TypeString,
								},
							},
							"incomplete_statuses": {
								Type:        genai.TypeArray,
								Description: "Incomplete View: List of work item statuses/states to check (e.g. ['[All]'] or ['In Progress', 'Active']).",
								Items: &genai.Schema{
									Type: genai.TypeString,
								},
							},
							"incomplete_date_statuses": {
								Type:        genai.TypeArray,
								Description: "Incomplete View: List of statuses where Start & Target Dates are required (e.g. ['[All]'] or ['In Progress', 'Active']).",
								Items: &genai.Schema{
									Type: genai.TypeString,
								},
							},
							"tag_regex": {
								Type:        genai.TypeString,
								Description: "Comma or semicolon-separated regex patterns for child tag inheritance & incomplete validation (e.g. '^BE-.*, ^FE-.*').",
							},
							"tag_regexes": {
								Type:        genai.TypeArray,
								Description: "List of regex rules for child tag inheritance & incomplete validation (e.g. ['^BE-.*', '^FE-.*', '^release-.*']).",
								Items: &genai.Schema{
									Type: genai.TypeString,
								},
							},
							"incomplete_criteria_tag_regex": {
								Type:        genai.TypeBoolean,
								Description: "Incomplete View: Require at least one tag matching configured tag regex rules.",
							},
							"incomplete_criteria_activity": {
								Type:        genai.TypeBoolean,
								Description: "Incomplete View: Require Activity on applicable items.",
							},
							"gemini_model": {
								Type:        genai.TypeString,
								Description: "Gemini AI model name (e.g. 'gemini-3.8-flash', 'gemini-1.5-pro').",
							},
							"open_in_browser_cmd": {
								Type:        genai.TypeString,
								Description: "Custom shell command to open links in browser (empty string for system default).",
							},
						},
					},
				},
				{
					Name:        "track_work_item",
					Description: "Tracks an Azure DevOps work item as a new root tab on the board, fetching its complete child hierarchy and saving it locally.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the work item to track as a root (e.g., 666236)",
							},
							"custom_title": {
								Type:        genai.TypeString,
								Description: "Optional custom title/alias for the tracked root tab",
							},
						},
						Required: []string{"work_item_id"},
					},
				},
				{
					Name:        "untrack_work_item",
					Description: "Untracks and removes a root work item tab from the local board.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the tracked root work item to remove",
							},
						},
						Required: []string{"work_item_id"},
					},
				},
				{
					Name:        "track_query",
					Description: "Tracks an Azure DevOps query (by GUID, URL, or Path) as a query tab on the board, executing it and importing all root work items.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"query_id_or_url": {
								Type:        genai.TypeString,
								Description: "The ADO Query ID (GUID) or full query web URL",
							},
							"name": {
								Type:        genai.TypeString,
								Description: "Optional custom name for the tracked query tab",
							},
						},
						Required: []string{"query_id_or_url"},
					},
				},
				{
					Name:        "untrack_query",
					Description: "Untracks and removes a query tab and its associated roots from the local board.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"query_id": {
								Type:        genai.TypeString,
								Description: "The ID/GUID of the tracked query to remove",
							},
						},
						Required: []string{"query_id"},
					},
				},
				{
					Name:        "get_board_summary",
					Description: "Retrieves a summary of the current board, including all tracked query tabs, root item tabs, item counts, and rollup estimate totals.",
					Parameters: &genai.Schema{
						Type:       genai.TypeObject,
						Properties: map[string]*genai.Schema{},
					},
				},
				{
					Name:        "app_get_state",
					Description: "Retrieves the live context and state of the interactive ado-board-cli TUI: currently selected/cursor work item, active screen, active tab, open tabs list, and filters.",
					Parameters: &genai.Schema{
						Type:       genai.TypeObject,
						Properties: map[string]*genai.Schema{},
					},
				},
				{
					Name:        "get_selected_item",
					Description: "Retrieves the work item currently selected or under the cursor in the interactive ado-board-cli TUI screen.",
					Parameters: &genai.Schema{
						Type:       genai.TypeObject,
						Properties: map[string]*genai.Schema{},
					},
				},
				{
					Name:        "get_users",
					Description: "Lists all known team members / users / identities from the project, with their display names and email addresses. Supports optional search query.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"query": {
								Type:        genai.TypeString,
								Description: "Optional search filter matching user display name or email address",
							},
							"limit": {
								Type:        genai.TypeInteger,
								Description: "Maximum number of users to return (default: 50)",
							},
						},
					},
				},
				{
					Name:        "rename_tab",
					Description: "Renames an existing board tab (tracked root or query tab) in the interactive TUI and database.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"new_name": {
								Type:        genai.TypeString,
								Description: "The new display name for the tab",
							},
							"tab_index": {
								Type:        genai.TypeInteger,
								Description: "0-based index of the tab to rename (optional if root_id or tab_name is provided, or active tab is used)",
							},
							"root_id": {
								Type:        genai.TypeInteger,
								Description: "The work item ID of the tracked root tab to rename",
							},
							"tab_name": {
								Type:        genai.TypeString,
								Description: "Existing name of the tab to rename",
							},
						},
						Required: []string{"new_name"},
					},
				},
				{
					Name:        "app_clear_filter",
					Description: "Clears active filters in the running ado-board-cli TUI application.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"scope": {
								Type:        genai.TypeString,
								Description: "Filter scope to clear: 'all' (clears both global and tab filters), 'global' (clears global filter only), or 'tab' (clears active tab filter only). Defaults to 'all'.",
							},
						},
					},
				},
				{
					Name:        "open_in_browser",
					Description: "Opens a work item or custom ADO URL in the default web browser.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"work_item_id": {
								Type:        genai.TypeInteger,
								Description: "The ID of the work item to open in the browser",
							},
							"url": {
								Type:        genai.TypeString,
								Description: "Direct URL to open in browser (optional if work_item_id is provided)",
							},
						},
					},
				},
			},
		},
	}
}
