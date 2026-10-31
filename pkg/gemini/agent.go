package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"google.golang.org/genai"
)

// Agent orchestrates multi-turn context-aware interactions with Gemini
type Agent struct {
	Client    *genai.Client
	ModelName string
	AdoClient *ado.Client
	DB        *db.DB
}

// NewAgent creates and initializes a new Gemini agent
func NewAgent(ctx context.Context, cfg *config.Config, database *db.DB, adoClient *ado.Client) (*Agent, error) {
	apiKey := cfg.GeminiApiKey
	if apiKey == "" {
		if dbKey, err := database.GetSetting("gemini_api_key"); err == nil && dbKey != "" {
			apiKey = dbKey
		}
	}

	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is not set. Please export GEMINI_API_KEY in your environment or set it via 'ado-board-cli set gemini-api-key <key>'")
	}

	modelName := cfg.GeminiModel
	if modelName == "" {
		if dbModel, err := database.GetSetting("gemini_model"); err == nil && dbModel != "" {
			modelName = dbModel
		} else {
			modelName = config.DefaultGeminiModel
		}
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &Agent{
		Client:    client,
		ModelName: modelName,
		AdoClient: adoClient,
		DB:        database,
	}, nil
}

// ValidateKeyFunc allows tests or custom environments to mock Gemini API key validation
var ValidateKeyFunc func(ctx context.Context, apiKey, modelName string) (bool, error)

// ValidateAPIKey validates the provided Gemini API key with a lightweight metadata lookup.
func ValidateAPIKey(ctx context.Context, apiKey, modelName string) (bool, error) {
	if ValidateKeyFunc != nil {
		return ValidateKeyFunc(ctx, apiKey, modelName)
	}

	trimmedKey := strings.TrimSpace(apiKey)
	if trimmedKey == "" {
		return false, fmt.Errorf("Gemini API key is not configured")
	}

	if modelName == "" {
		modelName = config.DefaultGeminiModel
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: trimmedKey,
	})
	if err != nil {
		return false, fmt.Errorf("failed to create client: %w", err)
	}

	// Models.Get is a lightweight metadata lookup that verifies the key without generating tokens
	_, err = client.Models.Get(ctx, modelName, nil)
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "API_KEY_INVALID") || strings.Contains(errMsg, "API key not valid") || strings.Contains(errMsg, "INVALID_ARGUMENT") {
			return false, fmt.Errorf("invalid API key")
		}
		if strings.Contains(errMsg, "PERMISSION_DENIED") {
			return false, fmt.Errorf("permission denied: invalid API key")
		}
		if strings.Contains(errMsg, "NOT_FOUND") {
			return false, fmt.Errorf("model %q not found or inaccessible", modelName)
		}
		return false, fmt.Errorf("Gemini error: %w", err)
	}

	return true, nil
}

// BuildSystemPrompt generates a comprehensive instruction prompt containing current board context
func BuildSystemPrompt(ctxPayload *ContextPayload) string {
	ctxJSON, _ := json.MarshalIndent(ctxPayload, "", "  ")

	currentTimeStr := ctxPayload.CurrentTime
	if currentTimeStr == "" {
		currentTimeStr = time.Now().Format("2006-01-02 15:04:05")
	}
	userTzStr := ctxPayload.UserTimezone
	if userTzStr == "" {
		userTzStr = time.Now().Format("MST (-07:00)")
	}

	return fmt.Sprintf(`You are an intelligent AI assistant integrated into the 'ado-board-cli' Azure DevOps terminal board management tool.
Your job is to understand user commands and execute actions on their Azure DevOps work items using the provided tools.

CURRENT USER INTERACTION CONTEXT:
`+"```json\n%s\n```"+`

CRITICAL CONTEXT RULES:
1. When the user says "this", "this item", "this task", or doesn't mention an ID/name:
   - Target the 'selected_item' in the context payload (ID: %d, Title: %q).
2. When the user says "these", "child tasks", "children", or "all subtasks":
   - Target the 'child_items' list under the selected item.
3. When the user says "parent", "parent PBI", "parent feature":
   - Target the 'parent_item' in the context payload.
4. When assigning to a person (e.g. "move to Alice" or "assign to user@example.com"):
   - Match against 'cached_identities' to pick the correct email address.
5. When asked to "divide effort of Y hours between X tasks":
   - Split the total hours evenly across the tasks created or updated.
6. When asked to "update parent estimate to sum of task effort":
   - Calculate the sum of the children's remaining/original work and call update_work_item or batch_update_items on parent_item.
7. ACTIVITY & TIMELINE VIEW FORMAT (DEFAULT PRESENTATION):
   - ALWAYS interpret and format all timestamps using the user's current local system time and timezone (%s, %s). Never display raw UTC times unless explicitly requested.
   - NOTE: These instructions represent the DEFAULT layout and formatting. If the user explicitly requests another format, layout, or style of output (such as a table, categorized breakdown, CSV, JSON, bulleted summary, or customized sections), prioritize and follow the user's requested output style.
   - When asked for activity (e.g. "get me activity for this today/yesterday/past 3 days", "what changed recently?"):
     * ALWAYS call 'get_activity' with the target 'work_item_id' (or 0 for selected item), 'time_range', and 'include_children: true'.
     * By default, format the output as a clean, unified chronological **Timeline View** grouped by date with clear headers (e.g. "### 📅 Today (Aug 22)", "### 📅 Yesterday (Aug 21)", etc.).
     * In the timeline, display each event chronologically (from earliest to latest):
       - **When someone comments**: Show who commented on which item, and format the entire comment body directly below as an indented markdown blockquote (prefix every line/bullet of the comment with >):
         * `+"`"+`HH:MM MST`+"`"+` — **Author Name** commented on **WorkItemType #ID** (*Title*):

           > Comment body, bulleted notes, or quoted text here...

       - **When someone links or raises a PR**: ALWAYS include the full PR URL, branches, and status:
         * `+"`"+`HH:MM MST`+"`"+` — **Author Name** linked **Pull Request #PR_ID**: *PR Title*
           - **URL**: [URL of PR from tool data, e.g. https://dev.azure.com/org/project/_git/repo/pullrequest/PR_ID]
           - **Branches**: `+"`"+`source_branch`+"`"+` → `+"`"+`target_branch`+"`"+` • Status: **Active** / **Completed**
       - **When status changes**:
         * `+"`"+`HH:MM MST`+"`"+` — **Author Name** changed status of **WorkItemType #ID** (*Title*): `+"`"+`Old State`+"`"+` → `+"`"+`New State`+"`"+`
       - **When an item is created, assigned, or estimates change**:
         * `+"`"+`HH:MM MST`+"`"+` — **Author Name** created **WorkItemType #ID** (*Title*) (Estimate: Xh, Assigned to: Person)
         * `+"`"+`HH:MM MST`+"`"+` — **Author Name** updated estimate / reassigned #ID to Person
8. ESTIMATE RULE: In this board, Remaining Work is automatically calculated as (Original Estimate - Completed Work). Whenever you update or set estimates, ensure remaining_work reflects (original_estimate - completed_work) unless explicitly instructed otherwise. When creating new tasks with an estimate, remaining_work defaults to the original estimate.
9. DATES & SCHEDULE RULE: When requested to set or update dates (e.g., 'start date Aug 18 12:00 AM', 'target date tomorrow 5pm', 'finish date Aug 21 3:57 PM', 'due next Monday', 'clear finish date'):
   - Pass 'start_date', 'target_date', or 'finish_date' in tool calls. You can pass flexible formats like '8/18/2026 12:00 AM', '2026-08-19', 'tomorrow 9am', '+3d', or empty string "" to clear.
10. TOOL INVOCATION & EFFICIENCY:
   - Always pass all known fields (such as 'parent_id', 'title', 'original_estimate', 'remaining_work', 'assigned_to', 'start_date', 'target_date', 'finish_date', 'description') in the initial 'create_work_item' or 'update_work_item' tool call to complete actions in a single turn.
   - Resiliently interpret user shorthand and typos (e.g., "out estimate of 12" or "est 12" -> original_estimate: 12, remaining_work: 12).
   - Do NOT call additional query tools ('get_work_item_details', 'get_activity') after successfully creating or modifying work items unless the user explicitly requested detailed inspection.
   - Once the requested action is performed, STOP calling tools immediately and return your final response.
11. APP SETTINGS & CONFIGURATION:
   - You have direct access to view and modify application settings and Incomplete View criteria using 'get_settings' and 'update_settings'.
   - When the user asks to view or check settings (e.g. "what are my current settings?", "show incomplete view criteria", "what is my auto sync interval?"):
     * Call 'get_settings' and summarize the configuration in clean markdown.
   - When the user asks to edit, set, or toggle settings (e.g. "set auto-sync to 60s", "disable auto sync", "set incomplete types to Task, Bug, User Story", "set incomplete statuses to [All]", "set applicable date statuses to In Progress, Active", "enable require finish date in incomplete view", "switch gemini model to gemini-1.5-pro"):
     * Call 'update_settings' with the desired fields.
     * Explain the updated settings clearly to the user.
12. HIERARCHY & CHILD INSPECTION:
   - When asked to list, show, or analyze children or subtrees under an item (e.g. "show all children of this feature", "get subtasks and stories under #656842", "what is the hierarchy under this story?", "show rollup estimates for children"):
     * Call 'get_children' with target 'work_item_id' (or 0 for selected item), 'recursive: true', and desired 'format'.
     * For full ancestor-to-descendant trees, call 'get_hierarchy'.
     * Present the breakdown with clear tree/list hierarchy, rollup totals (original estimate, remaining work, completed work, effort), and item statuses.
13. After tool execution succeeds, provide a concise, friendly summary formatted in clean markdown of what was accomplished (or formatted per user requested style).`,
		string(ctxJSON),
		func() int {
			if ctxPayload.SelectedItem != nil {
				return ctxPayload.SelectedItem.ID
			}
			return 0
		}(),
		func() string {
			if ctxPayload.SelectedItem != nil {
				return ctxPayload.SelectedItem.Title
			}
			return "None"
		}(),
		currentTimeStr,
		userTzStr,
	)
}

// Run executes a natural language query with dynamic context and tool calling
func (a *Agent) Run(ctx context.Context, query string, ctxPayload *ContextPayload, progress ProgressCallback, confirm ConfirmCallback) (*ExecutionResult, error) {
	if strings.TrimSpace(query) == "" {
		return &ExecutionResult{Response: "Please provide an instruction for Gemini."}, nil
	}

	executor := NewExecutor(a.AdoClient, a.DB, ctxPayload, confirm)
	systemInstruction := BuildSystemPrompt(ctxPayload)

	tools := GetAdoTools()
	generateConfig := &genai.GenerateContentConfig{
		Tools: tools,
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: systemInstruction},
			},
		},
	}

	if progress != nil {
		progress("✦ Thinking & analyzing context...")
	}

	// Conversation history
	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: query},
			},
		},
	}

	var toolLogs []string
	var lastAssistantText string
	maxTurns := 30

	for turn := 0; turn < maxTurns; turn++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		var res *genai.GenerateContentResponse
		var err error
		maxRetries := 3

		for attempt := 1; attempt <= maxRetries; attempt++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt > 1 {
				if progress != nil {
					progress(fmt.Sprintf("✦ Retrying with %s (attempt %d/%d)...", a.ModelName, attempt, maxRetries))
				}
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(attempt-1) * 1000 * time.Millisecond):
				}
			}

			res, err = a.Client.Models.GenerateContent(ctx, a.ModelName, contents, generateConfig)
			if err == nil {
				break
			}
		}

		if err != nil {
			return nil, fmt.Errorf("Gemini API error (%s after %d attempts): %w", a.ModelName, maxRetries, err)
		}

		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if len(res.Candidates) == 0 || res.Candidates[0].Content == nil {
			return &ExecutionResult{
				Response:        "No response received from Gemini.",
				AffectedIDs:     executor.GetAffectedIDs(),
				ToolExecLogs:    toolLogs,
				SettingsUpdated: executor.SettingsUpdated,
			}, nil
		}

		candidate := res.Candidates[0]
		contents = append(contents, candidate.Content)

		var functionCalls []*genai.FunctionCall
		var assistantTexts []string

		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				functionCalls = append(functionCalls, part.FunctionCall)
			}
			if part.Text != "" {
				assistantTexts = append(assistantTexts, part.Text)
			}
		}

		if len(assistantTexts) > 0 {
			lastAssistantText = strings.TrimSpace(strings.Join(assistantTexts, "\n"))
		}

		// If no function calls requested, we have our final text response!
		if len(functionCalls) == 0 {
			finalText := strings.TrimSpace(strings.Join(assistantTexts, "\n"))
			if finalText == "" {
				finalText = "Action completed successfully."
			}
			return &ExecutionResult{
				Response:        finalText,
				AffectedIDs:     executor.GetAffectedIDs(),
				ToolExecLogs:    toolLogs,
				SettingsUpdated: executor.SettingsUpdated,
			}, nil
		}

		// Execute function calls
		var responseParts []*genai.Part
		for _, fc := range functionCalls {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}

			logMsg := fmt.Sprintf("⚡ %s(%v)", fc.Name, formatArgs(fc.Args))
			toolLogs = append(toolLogs, logMsg)
			if progress != nil {
				progress(logMsg)
			}

			toolResult, execErr := executor.Execute(fc.Name, fc.Args)
			if execErr != nil {
				toolResult = map[string]any{"error": execErr.Error()}
			}

			responseParts = append(responseParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					Name:     fc.Name,
					Response: toolResult,
				},
			})
		}

		// Append tool results to conversation
		contents = append(contents, &genai.Content{
			Role:  "user",
			Parts: responseParts,
		})
	}

	fallbackResp := lastAssistantText
	if fallbackResp == "" {
		if len(toolLogs) > 0 {
			var createdOrUpdated []string
			for _, id := range executor.GetAffectedIDs() {
				createdOrUpdated = append(createdOrUpdated, fmt.Sprintf("#%d", id))
			}
			if len(createdOrUpdated) > 0 {
				fallbackResp = fmt.Sprintf("✓ Successfully performed requested actions on work item(s): %s.", strings.Join(createdOrUpdated, ", "))
			} else {
				fallbackResp = "✓ Completed requested actions."
			}
		} else {
			fallbackResp = "Completed operations."
		}
	}

	return &ExecutionResult{
		Response:        fallbackResp,
		AffectedIDs:     executor.GetAffectedIDs(),
		ToolExecLogs:    toolLogs,
		SettingsUpdated: executor.SettingsUpdated,
	}, nil
}

func formatArgs(args map[string]any) string {
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, ", ")
}
