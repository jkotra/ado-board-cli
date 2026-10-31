package ado

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// IdentityRef represents a user identity in ADO
type IdentityRef struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	UniqueName  string `json:"uniqueName,omitempty"` // Email in Azure AD
	ImageUrl    string `json:"imageUrl,omitempty"`
	Descriptor  string `json:"descriptor,omitempty"`
}

// WorkItemRelation represents links between work items (Parent, Child, Related, etc.)
type WorkItemRelation struct {
	Rel        string                 `json:"rel"`
	URL        string                 `json:"url"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// WorkItem represents an Azure DevOps Work Item
type WorkItem struct {
	ID        int                    `json:"id"`
	Rev       int                    `json:"rev,omitempty"`
	Fields    map[string]interface{} `json:"fields"`
	Relations []WorkItemRelation     `json:"relations,omitempty"`
	URL       string                 `json:"url,omitempty"`
	Links     map[string]interface{} `json:"_links,omitempty"`
}

// JsonPatchOperation represents an RFC 6902 JSON Patch operation
type JsonPatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	From  string      `json:"from,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

// WorkItemBatchResponse represents batch response for work items
type WorkItemBatchResponse struct {
	Count int         `json:"count"`
	Value []*WorkItem `json:"value"`
}

// CommentList represents the list of comments from ADO REST API
type CommentList struct {
	TotalCount int            `json:"totalCount"`
	Count      int            `json:"count"`
	Comments   []*CommentItem `json:"comments"`
}

// CommentMention represents a user mention in an ADO comment
type CommentMention struct {
	ArtifactID   string       `json:"artifactId"`
	ArtifactType string       `json:"artifactType"`
	CommentID    int          `json:"commentId"`
	TargetID     string       `json:"targetId"`
	ResolvedUser *IdentityRef `json:"resolvedUser"`
	Person       *IdentityRef `json:"person"`
}

// CommentItem represents a single comment
type CommentItem struct {
	ID           int               `json:"id"`
	WorkItemID   int               `json:"workItemId"`
	Version      int               `json:"version"`
	Text         string            `json:"text"`
	RenderedText string            `json:"renderedText"`
	Mentions     []*CommentMention `json:"mentions"`
	CreatedBy    *IdentityRef      `json:"createdBy"`
	CreatedDate  time.Time         `json:"createdDate"`
	ModifiedBy   *IdentityRef      `json:"modifiedBy"`
	ModifiedDate time.Time         `json:"modifiedDate"`
}

// WorkItemFieldUpdate represents a single field change in a work item update
type WorkItemFieldUpdate struct {
	OldValue interface{} `json:"oldValue,omitempty"`
	NewValue interface{} `json:"newValue,omitempty"`
}

// WorkItemRelationsUpdate represents relations added, removed, or updated in a work item update
type WorkItemRelationsUpdate struct {
	Added   []WorkItemRelation `json:"added,omitempty"`
	Removed []WorkItemRelation `json:"removed,omitempty"`
	Updated []WorkItemRelation `json:"updated,omitempty"`
}

// WorkItemUpdate represents a revision/update record for a work item in ADO
type WorkItemUpdate struct {
	ID          int                             `json:"id"`
	WorkItemID  int                             `json:"workItemId"`
	Rev         int                             `json:"rev"`
	RevisedBy   *IdentityRef                    `json:"revisedBy,omitempty"`
	RevisedDate time.Time                       `json:"revisedDate"`
	Fields      map[string]*WorkItemFieldUpdate `json:"fields,omitempty"`
	Relations   *WorkItemRelationsUpdate        `json:"relations,omitempty"`
	URL         string                          `json:"url,omitempty"`
}

// WorkItemUpdateList represents the batch response for work item updates
type WorkItemUpdateList struct {
	Count int               `json:"count"`
	Value []*WorkItemUpdate `json:"value"`
}

// WorkItemTypeInfo represents metadata about a work item type
type WorkItemTypeInfo struct {
	Name          string `json:"name"`
	ReferenceName string `json:"referenceName"`
	Description   string `json:"description"`
	Color         string `json:"color"`
	Icon          struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"icon"`
}

// WorkItemState represents a state of a work item type
type WorkItemState struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Color    string `json:"color"`
}

// WorkItemTypeField represents metadata about a field on a work item type
type WorkItemTypeField struct {
	Name           string `json:"name"`
	ReferenceName  string `json:"referenceName"`
	AlwaysRequired bool   `json:"alwaysRequired"`
	DefaultValue   any    `json:"defaultValue"`
	HelpText       string `json:"helpText"`
}

// GitRepository represents an Azure DevOps Git repository
type GitRepository struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Project struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
}

// GitRepositoryList represents a list of git repositories
type GitRepositoryList struct {
	Count int              `json:"count"`
	Value []*GitRepository `json:"value"`
}

// PullRequestReviewer represents a reviewer on an Azure DevOps Git Pull Request
type PullRequestReviewer struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	UniqueName  string `json:"uniqueName,omitempty"`
	IsRequired  bool   `json:"isRequired,omitempty"`
	Vote        int    `json:"vote,omitempty"` // 10: approved, 5: approved with suggestions, 0: no vote, -5: waiting for author, -10: rejected
	HasDeclined bool   `json:"hasDeclined,omitempty"`
	ReviewerURL string `json:"reviewerUrl,omitempty"`
}

// PRWorkItemRef represents a work item reference associated with a pull request
type PRWorkItemRef struct {
	ID  string `json:"id"`
	URL string `json:"url,omitempty"`
}

// PRWorkItemList represents response for listing PR work items
type PRWorkItemList struct {
	Count int              `json:"count"`
	Value []*PRWorkItemRef `json:"value"`
}

// ReviewerInput specifies a reviewer to add to a pull request
type ReviewerInput struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	UniqueName  string `json:"uniqueName,omitempty"`
	IsRequired  bool   `json:"isRequired,omitempty"`
}

// CreatePullRequestParams specifies parameters for creating a new pull request
type CreatePullRequestParams struct {
	SourceRefName string          `json:"sourceRefName"` // e.g. "refs/heads/feature/login"
	TargetRefName string          `json:"targetRefName"` // e.g. "refs/heads/main"
	Title         string          `json:"title"`
	Description   string          `json:"description,omitempty"`
	IsDraft       bool            `json:"isDraft,omitempty"`
	Reviewers     []ReviewerInput `json:"reviewers,omitempty"`
	WorkItemRefs  []PRWorkItemRef `json:"workItemRefs,omitempty"`
}

// GitPullRequest represents an Azure DevOps Pull Request
type GitPullRequest struct {
	PullRequestID int                    `json:"pullRequestId"`
	CodeReviewID  int                    `json:"codeReviewId,omitempty"`
	Status        string                 `json:"status"` // "active", "completed", "abandoned"
	CreatedBy     *IdentityRef           `json:"createdBy,omitempty"`
	CreationDate  time.Time              `json:"creationDate"`
	ClosedDate    time.Time              `json:"closedDate,omitempty"`
	Title         string                 `json:"title"`
	Description   string                 `json:"description,omitempty"`
	SourceRefName string                 `json:"sourceRefName"` // e.g. "refs/heads/feature/login"
	TargetRefName string                 `json:"targetRefName"` // e.g. "refs/heads/main"
	MergeStatus   string                 `json:"mergeStatus,omitempty"`
	IsDraft       bool                   `json:"isDraft,omitempty"`
	Repository    *GitRepository         `json:"repository,omitempty"`
	Reviewers     []*PullRequestReviewer `json:"reviewers,omitempty"`
	WorkItemRefs  []*PRWorkItemRef       `json:"workItemRefs,omitempty"`
	URL           string                 `json:"url,omitempty"`
}

func (pr *GitPullRequest) SourceBranch() string {
	return strings.TrimPrefix(pr.SourceRefName, "refs/heads/")
}

func (pr *GitPullRequest) TargetBranch() string {
	return strings.TrimPrefix(pr.TargetRefName, "refs/heads/")
}

// GitPullRequestList represents batch response for pull requests
type GitPullRequestList struct {
	Count int               `json:"count"`
	Value []*GitPullRequest `json:"value"`
}

// PRAttachment represents an attachment uploaded to an Azure DevOps Git Pull Request
type PRAttachment struct {
	ID          int          `json:"id"`
	URL         string       `json:"url"`
	CreatedDate time.Time    `json:"createdDate,omitempty"`
	Author      *IdentityRef `json:"author,omitempty"`
}

// PRAttachmentList represents response for listing PR attachments
type PRAttachmentList struct {
	Count int             `json:"count"`
	Value []*PRAttachment `json:"value"`
}

// PullRequestDetailed represents comprehensive pull request details with attachments and work items
type PullRequestDetailed struct {
	PullRequest *GitPullRequest  `json:"pullRequest"`
	Attachments []*PRAttachment  `json:"attachments,omitempty"`
	WorkItems   []*PRWorkItemRef `json:"workItems,omitempty"`
	WebURL      string           `json:"webUrl"`
}

// PullRequestComment represents an individual discussion comment inside a pull request thread
type PullRequestComment struct {
	ID              int          `json:"id"`
	ParentCommentID int          `json:"parentCommentId,omitempty"`
	Author          *IdentityRef `json:"author,omitempty"`
	Content         string       `json:"content"`
	PublishedDate   time.Time    `json:"publishedDate"`
	LastUpdatedDate time.Time    `json:"lastUpdatedDate,omitempty"`
	CommentType     string       `json:"commentType,omitempty"` // "text", "system", "codeChange"
	IsDeleted       bool         `json:"isDeleted,omitempty"`
}

// PullRequestThreadContext represents file and line coordinates for an inline PR review comment
type PullRequestThreadContext struct {
	FilePath       string `json:"filePath,omitempty"`
	RightFileStart *struct {
		Line   int `json:"line"`
		Offset int `json:"offset"`
	} `json:"rightFileStart,omitempty"`
	RightFileEnd *struct {
		Line   int `json:"line"`
		Offset int `json:"offset"`
	} `json:"rightFileEnd,omitempty"`
	LeftFileStart *struct {
		Line   int `json:"line"`
		Offset int `json:"offset"`
	} `json:"leftFileStart,omitempty"`
	LeftFileEnd *struct {
		Line   int `json:"line"`
		Offset int `json:"offset"`
	} `json:"leftFileEnd,omitempty"`
}

// PullRequestThread represents a discussion thread on an Azure DevOps Git Pull Request
type PullRequestThread struct {
	ID              int                       `json:"id"`
	PublishedDate   time.Time                 `json:"publishedDate"`
	LastUpdatedDate time.Time                 `json:"lastUpdatedDate,omitempty"`
	Comments        []*PullRequestComment     `json:"comments,omitempty"`
	Status          string                    `json:"status,omitempty"` // "active", "fixed", "wontFix", "closed", "byDesign", "pending"
	ThreadContext   *PullRequestThreadContext `json:"threadContext,omitempty"`
	IsDeleted       bool                      `json:"isDeleted,omitempty"`
}

// PullRequestThreadList represents response for listing PR discussion threads
type PullRequestThreadList struct {
	Count int                  `json:"count"`
	Value []*PullRequestThread `json:"value"`
}

// LinkedPullRequest represents an extracted pull request reference from an artifact relation
type LinkedPullRequest struct {
	ProjectID     string `json:"project_id"`
	RepositoryID  string `json:"repository_id"`
	PullRequestID int    `json:"pull_request_id"`
	URL           string `json:"url"`
}

// LinkedWorkItem represents an extracted linked work item
type LinkedWorkItem struct {
	ID      int    `json:"id"`
	RelType string `json:"rel_type"` // "Parent", "Child", "Related", "Duplicate", "Tested By", etc.
	RawRel  string `json:"raw_rel"`
	URL     string `json:"url"`
}

// ProjectTeam represents a team within an Azure DevOps project
type ProjectTeam struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// ProjectTeamList represents response for listing project teams
type ProjectTeamList struct {
	Count int           `json:"count"`
	Value []ProjectTeam `json:"value"`
}

// TeamMemberList represents response for listing members of a team
type TeamMemberList struct {
	Count int `json:"count"`
	Value []struct {
		Identity IdentityRef `json:"identity"`
		IsTeamAdmin bool     `json:"isTeamAdmin,omitempty"`
	} `json:"value"`
}


// Helper methods on WorkItem to extract common fields safely

func (w *WorkItem) GetStringField(keys ...string) string {
	if w == nil || w.Fields == nil {
		return ""
	}
	for _, k := range keys {
		if val, ok := w.Fields[k]; ok && val != nil {
			switch v := val.(type) {
			case string:
				return v
			case fmt.Stringer:
				return v.String()
			default:
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return ""
}

func (w *WorkItem) GetFloatField(keys ...string) float64 {
	if w == nil || w.Fields == nil {
		return 0
	}
	for _, k := range keys {
		if val, ok := w.Fields[k]; ok && val != nil {
			switch v := val.(type) {
			case float64:
				return v
			case float32:
				return float64(v)
			case int:
				return float64(v)
			case int64:
				return float64(v)
			case json.Number:
				if f, err := v.Float64(); err == nil {
					return f
				}
			case string:
				if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					return f
				}
			}
		}
	}
	return 0
}

func (w *WorkItem) GetIntField(keys ...string) int {
	return int(w.GetFloatField(keys...))
}

func (w *WorkItem) GetIdentityField(key string) *IdentityRef {
	if w == nil || w.Fields == nil {
		return nil
	}
	val, ok := w.Fields[key]
	if !ok || val == nil {
		return nil
	}

	if identity, ok := val.(*IdentityRef); ok {
		return identity
	}

	// Try unmarshalling if map
	if m, ok := val.(map[string]interface{}); ok {
		ref := &IdentityRef{}
		if name, ok := m["displayName"].(string); ok {
			ref.DisplayName = name
		}
		if email, ok := m["uniqueName"].(string); ok {
			ref.UniqueName = email
		}
		if id, ok := m["id"].(string); ok {
			ref.ID = id
		}
		if img, ok := m["imageUrl"].(string); ok {
			ref.ImageUrl = img
		}
		return ref
	}

	if s, ok := val.(string); ok {
		return &IdentityRef{DisplayName: s, UniqueName: s}
	}

	return nil
}

// Convenient getters for standard work item properties

func (w *WorkItem) Title() string {
	return w.GetStringField("System.Title")
}

func (w *WorkItem) Type() string {
	return w.GetStringField("System.WorkItemType")
}

func (w *WorkItem) State() string {
	return w.GetStringField("System.State")
}

// IsStateDoneOrClosed returns true if the state string represents a completed or closed state in ADO
func IsStateDoneOrClosed(state string) bool {
	st := strings.ToLower(strings.TrimSpace(state))
	switch st {
	case "done", "closed", "resolved", "removed", "completed", "inactive":
		return true
	}
	return false
}

func (w *WorkItem) Reason() string {
	return w.GetStringField("System.Reason")
}

func (w *WorkItem) AssignedTo() *IdentityRef {
	return w.GetIdentityField("System.AssignedTo")
}

func (w *WorkItem) CreatedBy() *IdentityRef {
	return w.GetIdentityField("System.CreatedBy")
}

func (w *WorkItem) ChangedBy() *IdentityRef {
	return w.GetIdentityField("System.ChangedBy")
}

func (w *WorkItem) AuthorizedAs() *IdentityRef {
	return w.GetIdentityField("System.AuthorizedAs")
}

func (w *WorkItem) AuthorizedBy() *IdentityRef {
	return w.GetIdentityField("System.AuthorizedBy")
}

// AllIdentities returns all unique IdentityRefs associated with this work item
func (w *WorkItem) AllIdentities() []*IdentityRef {
	var list []*IdentityRef
	seen := make(map[string]bool)

	for _, k := range []string{"System.AssignedTo", "System.CreatedBy", "System.ChangedBy", "System.AuthorizedAs", "System.AuthorizedBy"} {
		if id := w.GetIdentityField(k); id != nil && id.DisplayName != "" {
			key := id.ID
			if key == "" {
				key = id.UniqueName
			}
			if key != "" && !seen[strings.ToLower(key)] {
				seen[strings.ToLower(key)] = true
				list = append(list, id)
			}
		}
	}
	return list
}

func (w *WorkItem) CreatedDate() string {
	return w.GetStringField("System.CreatedDate")
}

func (w *WorkItem) ChangedDate() string {
	return w.GetStringField("System.ChangedDate")
}

func (w *WorkItem) StartDate() string {
	return w.GetStringField("Microsoft.VSTS.Scheduling.StartDate", "Custom.StartDate")
}

func (w *WorkItem) TargetDate() string {
	return w.GetStringField("Microsoft.VSTS.Scheduling.TargetDate", "Custom.TargetDate", "Microsoft.VSTS.Scheduling.DueDate", "Custom.DueDate")
}

func (w *WorkItem) FinishDate() string {
	return w.GetStringField("Microsoft.VSTS.Scheduling.FinishDate", "Custom.FinishDate")
}

func (w *WorkItem) Description() string {
	// Task/Feature uses System.Description, Bug often uses Microsoft.VSTS.TCM.ReproSteps
	desc := w.GetStringField("System.Description", "Microsoft.VSTS.TCM.ReproSteps")
	return desc
}

func (w *WorkItem) AcceptanceCriteria() string {
	return w.GetStringField("Microsoft.VSTS.Common.AcceptanceCriteria")
}

func (w *WorkItem) ReproSteps() string {
	return w.GetStringField("Microsoft.VSTS.TCM.ReproSteps")
}

func (w *WorkItem) OriginalEstimate() float64 {
	return w.GetFloatField("Microsoft.VSTS.Scheduling.OriginalEstimate", "Custom.OriginalEstimate")
}

func (w *WorkItem) RemainingWork() float64 {
	return w.GetFloatField("Microsoft.VSTS.Scheduling.RemainingWork", "Custom.RemainingWork")
}

func (w *WorkItem) CompletedWork() float64 {
	return w.GetFloatField("Microsoft.VSTS.Scheduling.CompletedWork", "Custom.CompletedWork")
}

func (w *WorkItem) Effort() float64 {
	return w.GetFloatField("Microsoft.VSTS.Scheduling.Effort", "Microsoft.VSTS.Scheduling.StoryPoints", "Custom.Effortss", "Custom.Effort")
}

func (w *WorkItem) StoryPoints() float64 {
	return w.GetFloatField("Microsoft.VSTS.Scheduling.StoryPoints")
}

func (w *WorkItem) Priority() int {
	return w.GetIntField("Microsoft.VSTS.Common.Priority")
}

func (w *WorkItem) AreaPath() string {
	return w.GetStringField("System.AreaPath")
}

func (w *WorkItem) IterationPath() string {
	return w.GetStringField("System.IterationPath")
}

func (w *WorkItem) Tags() string {
	return w.GetStringField("System.Tags")
}

func (w *WorkItem) Activity() string {
	return w.GetStringField("Microsoft.VSTS.Common.Activity", "Custom.Activity")
}

func (w *WorkItem) ParentID() int {
	if p := w.GetIntField("System.Parent"); p > 0 {
		return p
	}
	// Check relations for Parent link
	for _, rel := range w.Relations {
		if rel.Rel == "System.LinkTypes.Hierarchy-Reverse" {
			// URL ends with /workItems/{id}
			parts := strings.Split(strings.TrimRight(rel.URL, "/"), "/")
			if len(parts) > 0 {
				if id, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
					return id
				}
			}
		}
	}
	return 0
}

// ChildIDs returns all child work item IDs linked via Hierarchy-Forward relations
func (w *WorkItem) ChildIDs() []int {
	var childIDs []int
	for _, rel := range w.Relations {
		if rel.Rel == "System.LinkTypes.Hierarchy-Forward" {
			parts := strings.Split(strings.TrimRight(rel.URL, "/"), "/")
			if len(parts) > 0 {
				if id, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
					childIDs = append(childIDs, id)
				}
			}
		}
	}
	return childIDs
}

// ParsePullRequest checks if a relation is an artifact link to a Git Pull Request
func (r *WorkItemRelation) ParsePullRequest() (*LinkedPullRequest, bool) {
	if r == nil || r.URL == "" {
		return nil, false
	}
	u := r.URL
	if !strings.Contains(u, "PullRequestId") {
		return nil, false
	}

	unescaped, err := url.PathUnescape(u)
	if err != nil {
		unescaped = u
	}
	unescaped = strings.ReplaceAll(unescaped, "%2F", "/")
	unescaped = strings.ReplaceAll(unescaped, "%2f", "/")

	idx := strings.Index(unescaped, "PullRequestId/")
	if idx == -1 {
		return nil, false
	}
	sub := unescaped[idx+len("PullRequestId/"):]
	parts := strings.Split(strings.Trim(sub, "/"), "/")
	if len(parts) == 0 {
		return nil, false
	}

	prID, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return nil, false
	}

	linked := &LinkedPullRequest{
		PullRequestID: prID,
		URL:           u,
	}
	if len(parts) == 3 {
		linked.ProjectID = parts[0]
		linked.RepositoryID = parts[1]
	} else if len(parts) == 2 {
		linked.RepositoryID = parts[0]
	}
	return linked, true
}

// LinkedPullRequests extracts all linked Pull Requests from the work item's relations
func (w *WorkItem) LinkedPullRequests() []*LinkedPullRequest {
	if w == nil {
		return nil
	}
	var prs []*LinkedPullRequest
	for _, rel := range w.Relations {
		if pr, ok := rel.ParsePullRequest(); ok {
			prs = append(prs, pr)
		}
	}
	return prs
}

// FormatRelType returns a clean, human-readable name for relation types
func FormatRelType(rel string) string {
	switch rel {
	case "System.LinkTypes.Hierarchy-Reverse":
		return "Parent"
	case "System.LinkTypes.Hierarchy-Forward":
		return "Child"
	case "System.LinkTypes.Related":
		return "Related"
	case "System.LinkTypes.Duplicate-Forward":
		return "Duplicate"
	case "System.LinkTypes.Duplicate-Reverse":
		return "Duplicate Of"
	case "Microsoft.VSTS.Common.TestedBy-Forward":
		return "Tested By"
	case "Microsoft.VSTS.Common.TestedBy-Reverse":
		return "Tests"
	case "Microsoft.VSTS.Common.Affects-Forward":
		return "Affects"
	case "Microsoft.VSTS.Common.Affects-Reverse":
		return "Affected By"
	default:
		parts := strings.Split(rel, ".")
		return parts[len(parts)-1]
	}
}

// LinkedWorkItems extracts all linked work items (Parent, Child, Related, etc.) from relations
func (w *WorkItem) LinkedWorkItems() []*LinkedWorkItem {
	if w == nil {
		return nil
	}
	var links []*LinkedWorkItem
	seen := make(map[int]bool)
	for _, rel := range w.Relations {
		if strings.HasPrefix(rel.Rel, "System.LinkTypes.") || strings.HasPrefix(rel.Rel, "Microsoft.VSTS.Common.") {
			parts := strings.Split(strings.TrimRight(rel.URL, "/"), "/")
			if len(parts) > 0 {
				if id, err := strconv.Atoi(parts[len(parts)-1]); err == nil && id != w.ID && !seen[id] {
					seen[id] = true
					links = append(links, &LinkedWorkItem{
						ID:      id,
						RelType: FormatRelType(rel.Rel),
						RawRel:  rel.Rel,
						URL:     rel.URL,
					})
				}
			}
		}
	}
	return links
}

// ParseRegexRules parses a comma, semicolon, or newline separated string into individual regex rules,
// respecting parenthesis, brackets, and braces nesting.
func ParseRegexRules(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var rules []string
	var current strings.Builder
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	escaped := false

	for _, r := range raw {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			current.WriteRune(r)
			escaped = true
			continue
		}
		if r == '(' {
			parenDepth++
			current.WriteRune(r)
			continue
		} else if r == ')' {
			if parenDepth > 0 {
				parenDepth--
			}
			current.WriteRune(r)
			continue
		} else if r == '[' {
			bracketDepth++
			current.WriteRune(r)
			continue
		} else if r == ']' {
			if bracketDepth > 0 {
				bracketDepth--
			}
			current.WriteRune(r)
			continue
		} else if r == '{' {
			braceDepth++
			current.WriteRune(r)
			continue
		} else if r == '}' {
			if braceDepth > 0 {
				braceDepth--
			}
			current.WriteRune(r)
			continue
		}

		// Split on comma, semicolon, newline when at top level
		if (r == ',' || r == ';' || r == '\n' || r == '\r') && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
			rule := strings.TrimSpace(current.String())
			if rule != "" {
				rules = append(rules, rule)
			}
			current.Reset()
			continue
		}

		current.WriteRune(r)
	}

	last := strings.TrimSpace(current.String())
	if last != "" {
		rules = append(rules, last)
	}

	return rules
}

// CompileRegexRules compiles all valid regex rules from a delimited string into compiled regexp objects.
func CompileRegexRules(raw string) []*regexp.Regexp {
	rules := ParseRegexRules(raw)
	var compiled []*regexp.Regexp
	for _, r := range rules {
		if re, err := regexp.Compile(r); err == nil {
			compiled = append(compiled, re)
		}
	}
	return compiled
}

// HasTagMatchingRegex checks if any tag in the semicolon-separated tags list matches ANY of the configured regex rules in tagRegex.
func HasTagMatchingRegex(tags, tagRegex string) bool {
	if strings.TrimSpace(tagRegex) == "" {
		return true
	}
	compiled := CompileRegexRules(tagRegex)
	if len(compiled) == 0 {
		return true // invalid/empty regex shouldn't flag everything as incomplete
	}
	if strings.TrimSpace(tags) == "" {
		return false
	}
	for _, tag := range strings.Split(tags, ";") {
		clean := strings.TrimSpace(tag)
		if clean == "" {
			continue
		}
		for _, re := range compiled {
			if re.MatchString(clean) {
				return true
			}
		}
	}
	return false
}

// FilterInheritedTags filters parent tags to only those matching at least one of the regex rules in tagRegex.
func FilterInheritedTags(parentTags, tagRegex string) string {
	if strings.TrimSpace(parentTags) == "" || strings.TrimSpace(tagRegex) == "" {
		return ""
	}
	compiled := CompileRegexRules(tagRegex)
	if len(compiled) == 0 {
		return ""
	}
	var matched []string
	seen := make(map[string]bool)
	for _, tag := range strings.Split(parentTags, ";") {
		clean := strings.TrimSpace(tag)
		if clean == "" || seen[clean] {
			continue
		}
		for _, re := range compiled {
			if re.MatchString(clean) {
				seen[clean] = true
				matched = append(matched, clean)
				break
			}
		}
	}
	return strings.Join(matched, "; ")
}


