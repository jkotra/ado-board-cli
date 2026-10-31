package ado

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"resty.dev/v3"
)

var fieldFallbacks = map[string][]string{
	"Microsoft.VSTS.Scheduling.OriginalEstimate": {"Custom.OriginalEstimate"},
	"Custom.OriginalEstimate":                   {"Microsoft.VSTS.Scheduling.OriginalEstimate"},
	"Microsoft.VSTS.Scheduling.RemainingWork":   {"Custom.RemainingWork"},
	"Custom.RemainingWork":                      {"Microsoft.VSTS.Scheduling.RemainingWork"},
	"Microsoft.VSTS.Scheduling.CompletedWork":   {"Custom.CompletedWork"},
	"Custom.CompletedWork":                      {"Microsoft.VSTS.Scheduling.CompletedWork"},
	"Microsoft.VSTS.Scheduling.Effort":          {"Microsoft.VSTS.Scheduling.StoryPoints", "Custom.Effort", "Custom.Effortss", "Custom.StoryPoints"},
	"Custom.Effort":                             {"Microsoft.VSTS.Scheduling.Effort", "Microsoft.VSTS.Scheduling.StoryPoints", "Custom.StoryPoints"},
	"Custom.Effortss":                           {"Microsoft.VSTS.Scheduling.Effort", "Microsoft.VSTS.Scheduling.StoryPoints"},
	"Microsoft.VSTS.Scheduling.StoryPoints":     {"Microsoft.VSTS.Scheduling.Effort", "Custom.StoryPoints", "Custom.Effort"},
	"Custom.StoryPoints":                        {"Microsoft.VSTS.Scheduling.StoryPoints", "Microsoft.VSTS.Scheduling.Effort"},
	"Microsoft.VSTS.Scheduling.StartDate":       {"Custom.StartDate"},
	"Custom.StartDate":                          {"Microsoft.VSTS.Scheduling.StartDate"},
	"Microsoft.VSTS.Scheduling.TargetDate":      {"Microsoft.VSTS.Scheduling.DueDate", "Custom.TargetDate", "Custom.DueDate"},
	"Custom.TargetDate":                         {"Microsoft.VSTS.Scheduling.TargetDate", "Microsoft.VSTS.Scheduling.DueDate"},
	"Microsoft.VSTS.Scheduling.DueDate":         {"Microsoft.VSTS.Scheduling.TargetDate", "Custom.DueDate", "Custom.TargetDate"},
	"Custom.DueDate":                            {"Microsoft.VSTS.Scheduling.DueDate", "Microsoft.VSTS.Scheduling.TargetDate"},
	"Microsoft.VSTS.Scheduling.FinishDate":      {"Microsoft.VSTS.Scheduling.CompletedDate", "Custom.FinishDate", "Custom.CompletedDate"},
	"Custom.FinishDate":                         {"Microsoft.VSTS.Scheduling.FinishDate", "Microsoft.VSTS.Scheduling.CompletedDate"},
	"System.Description":                        {"Microsoft.VSTS.TCM.ReproSteps"},
	"Microsoft.VSTS.TCM.ReproSteps":             {"System.Description"},
	"Microsoft.VSTS.Common.Activity":            {"Custom.Activity"},
	"Custom.Activity":                           {"Microsoft.VSTS.Common.Activity"},
}

var friendlyFieldMap = map[string][]string{
	"remaining work":    {"Microsoft.VSTS.Scheduling.RemainingWork", "Custom.RemainingWork"},
	"original estimate": {"Microsoft.VSTS.Scheduling.OriginalEstimate", "Custom.OriginalEstimate"},
	"completed work":    {"Microsoft.VSTS.Scheduling.CompletedWork", "Custom.CompletedWork"},
	"effort":            {"Microsoft.VSTS.Scheduling.Effort", "Custom.Effort", "Custom.Effortss", "Microsoft.VSTS.Scheduling.StoryPoints"},
	"story points":      {"Microsoft.VSTS.Scheduling.StoryPoints", "Microsoft.VSTS.Scheduling.Effort", "Custom.Effort"},
	"start date":        {"Microsoft.VSTS.Scheduling.StartDate", "Custom.StartDate"},
	"target date":       {"Microsoft.VSTS.Scheduling.TargetDate", "Custom.TargetDate", "Microsoft.VSTS.Scheduling.DueDate", "Custom.DueDate"},
	"finish date":       {"Microsoft.VSTS.Scheduling.FinishDate", "Custom.FinishDate", "Microsoft.VSTS.Scheduling.CompletedDate"},
	"due date":          {"Microsoft.VSTS.Scheduling.DueDate", "Custom.DueDate", "Microsoft.VSTS.Scheduling.TargetDate", "Custom.TargetDate"},
	"description":       {"System.Description", "Microsoft.VSTS.TCM.ReproSteps"},
	"title":             {"System.Title"},
	"tags":              {"System.Tags"},
	"activity":          {"Microsoft.VSTS.Common.Activity", "Custom.Activity"},
	"iteration path":    {"System.IterationPath"},
	"area path":         {"System.AreaPath"},
	"assigned to":       {"System.AssignedTo"},
}

func isDateOrNumericField(field string) bool {
	f := strings.ToLower(field)
	return strings.Contains(f, "date") ||
		strings.Contains(f, "estimate") ||
		strings.Contains(f, "remainingwork") ||
		strings.Contains(f, "completedwork") ||
		strings.Contains(f, "effort") ||
		strings.Contains(f, "storypoints")
}

// extractFailingFields finds any field reference names or friendly field names mentioned in an ADO error
func extractFailingFields(errMsg string) []string {
	var results []string
	seen := make(map[string]bool)
	add := func(f string) {
		f = strings.TrimSpace(strings.Trim(f, ".'\""))
		if f == "" || seen[f] {
			return
		}
		seen[f] = true
		results = append(results, f)
		if refs, ok := friendlyFieldMap[strings.ToLower(f)]; ok {
			for _, ref := range refs {
				if !seen[ref] {
					seen[ref] = true
					results = append(results, ref)
				}
			}
		}
	}

	// 1. Check for JSON "fieldReferenceName": "..."
	reRef := regexp.MustCompile(`(?i)"fieldReferenceName"\s*:\s*"([^"]+)"`)
	for _, m := range reRef.FindAllStringSubmatch(errMsg, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}

	// 2. Check for "Rule Error for field <Name>."
	reRule := regexp.MustCompile(`(?i)Rule Error for field\s+([^.]+)\.`)
	for _, m := range reRule.FindAllStringSubmatch(errMsg, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}

	// 3. Check for standard "Field '...' does not exist" or "Cannot find field '...'"
	reMissing := regexp.MustCompile(`(?i)(?:Cannot find field|Field|field|does not exist:?|following field does not exist:?)\s+['"]?([a-zA-Z0-9_.]+)['"]?`)
	for _, m := range reMissing.FindAllStringSubmatch(errMsg, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}

	// 4. Check for quoted field names "field '...' "
	reQuoted := regexp.MustCompile(`(?i)(?:field|Field)\s+['"]([^'"]+)['"]`)
	for _, m := range reQuoted.FindAllStringSubmatch(errMsg, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}

	// 5. Check for read-only or locked fields e.g. "The field 'Remaining Work' cannot be modified"
	reFieldCannot := regexp.MustCompile(`(?i)(?:The field|the field)\s+['"]([^'"]+)['"]\s+(?:cannot be modified|cannot be changed|is read-only|is not allowed|has a value that is not allowed|is locked)`)
	for _, m := range reFieldCannot.FindAllStringSubmatch(errMsg, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}

	// 6. Check for general read-only phrase
	reReadOnly := regexp.MustCompile(`(?i)(?:read-only field|cannot modify field|cannot change field)\s+['"]?([^'"\n.]+)['"]?`)
	for _, m := range reReadOnly.FindAllStringSubmatch(errMsg, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}

	return results
}

// extractMissingField finds any field name mentioned in an ADO error
func extractMissingField(errMsg string) string {
	fields := extractFailingFields(errMsg)
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

// Client is the Azure DevOps REST API client powered by Resty v3
type Client struct {
	Org              string
	Project          string
	Token            string
	R                *resty.Client
	identityMu       sync.RWMutex
	identityCache    map[string]*IdentityRef
	identityResolver func(string) *IdentityRef
}

// NewClient creates a new ADO API client with automated retries and JSON handling
func NewClient(org, project, token string) *Client {
	r := resty.New().
		SetTimeout(30 * time.Second).
		SetRetryCount(3).
		SetRetryWaitTime(500 * time.Millisecond).
		SetRetryMaxWaitTime(2 * time.Second).
		SetHeader("Accept", "application/json")

	if token != "" {
		basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+token))
		r.SetHeader("Authorization", basicAuth)
	}

	r.AddRequestMiddleware(func(c *resty.Client, req *resty.Request) error {
		slog.Debug("ADO HTTP Request",
			slog.String("method", req.Method),
			slog.String("url", req.URL),
		)
		return nil
	})

	r.AddResponseMiddleware(func(c *resty.Client, resp *resty.Response) error {
		reqURL := ""
		if resp.Request != nil {
			reqURL = resp.Request.URL
		}
		if resp.CascadeError != nil {
			slog.Error("ADO HTTP Cascade Error",
				slog.String("url", reqURL),
				slog.Any("error", resp.CascadeError),
			)
			return nil
		}
		if resp.IsStatusSuccess() {
			slog.Debug("ADO HTTP Response Success",
				slog.Int("status", resp.StatusCode()),
				slog.Int64("duration_ms", resp.Duration().Milliseconds()),
				slog.String("url", reqURL),
			)
		} else {
			slog.Error("ADO HTTP Response Failed",
				slog.Int("status", resp.StatusCode()),
				slog.Int64("duration_ms", resp.Duration().Milliseconds()),
				slog.String("url", reqURL),
				slog.String("status_text", resp.Status()),
			)
		}
		return nil
	})

	return &Client{
		Org:           org,
		Project:       project,
		Token:         token,
		R:             r,
		identityCache: make(map[string]*IdentityRef),
	}
}

// RegisterIdentities adds identity references to the client's local cache
func (c *Client) RegisterIdentities(identities ...*IdentityRef) {
	if len(identities) == 0 {
		return
	}
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	if c.identityCache == nil {
		c.identityCache = make(map[string]*IdentityRef)
	}
	for _, id := range identities {
		if id == nil {
			continue
		}
		if id.ID != "" {
			c.identityCache[strings.ToLower(strings.TrimSpace(id.ID))] = id
		}
		if id.UniqueName != "" {
			c.identityCache[strings.ToLower(strings.TrimSpace(id.UniqueName))] = id
		}
		if id.DisplayName != "" {
			c.identityCache[strings.ToLower(strings.TrimSpace(id.DisplayName))] = id
		}
	}
}

// RegisterIdentity adds a single identity reference to cache
func (c *Client) RegisterIdentity(ref *IdentityRef) {
	if ref != nil {
		c.RegisterIdentities(ref)
	}
}

// SetIdentityResolver configures an external lookup callback (e.g. SQLite database)
func (c *Client) SetIdentityResolver(fn func(string) *IdentityRef) {
	c.identityMu.Lock()
	defer c.identityMu.Unlock()
	c.identityResolver = fn
}

// FindIdentity searches for an identity by GUID, email, or display name
func (c *Client) FindIdentity(query string) *IdentityRef {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}

	c.identityMu.RLock()
	if ref, ok := c.identityCache[q]; ok && ref != nil {
		c.identityMu.RUnlock()
		return ref
	}
	resolver := c.identityResolver
	c.identityMu.RUnlock()

	if resolver != nil {
		if ref := resolver(query); ref != nil {
			c.RegisterIdentity(ref)
			return ref
		}
	}
	return nil
}

// GetKnownIdentities returns all unique cached identities
func (c *Client) GetKnownIdentities() []*IdentityRef {
	c.identityMu.RLock()
	defer c.identityMu.RUnlock()
	seen := make(map[string]bool)
	var list []*IdentityRef
	for _, u := range c.identityCache {
		if u == nil {
			continue
		}
		key := strings.ToLower(u.ID)
		if key == "" {
			key = strings.ToLower(u.UniqueName)
		}
		if key == "" {
			key = strings.ToLower(u.DisplayName)
		}
		if key != "" && !seen[key] {
			seen[key] = true
			list = append(list, u)
		}
	}
	return list
}

// FormatTextForADO converts markdown and @mentions in text into ADO-compatible HTML
func (c *Client) FormatTextForADO(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return FormatMarkdownToAdoHTML(text, c.FindIdentity, c.GetKnownIdentities())
}

// ValidatePAT checks whether the configured PAT token is valid with Azure DevOps.
// It queries connectionData (or profile) and returns true if authenticated, or false with an error description.
func (c *Client) ValidatePAT() (bool, error) {
	if strings.TrimSpace(c.Token) == "" {
		return false, fmt.Errorf("PAT token is not configured")
	}

	var endpoint string
	if c.Org != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/connectionData", url.PathEscape(c.Org))
	} else {
		endpoint = "https://app.vssps.visualstudio.com/_apis/profile/profiles/me?api-version=7.0"
	}

	resp, err := c.R.R().Get(endpoint)
	if err != nil {
		return false, fmt.Errorf("connection failed: %w", err)
	}

	if resp.StatusCode() == http.StatusNonAuthoritativeInfo || resp.StatusCode() == http.StatusUnauthorized {
		return false, fmt.Errorf("authentication failed (HTTP %d): invalid or expired PAT", resp.StatusCode())
	}
	if resp.StatusCode() == http.StatusForbidden {
		return false, fmt.Errorf("access forbidden (HTTP 403): insufficient PAT permissions")
	}
	if resp.StatusCode() == http.StatusNotFound && c.Org != "" {
		return false, fmt.Errorf("organization %q not found (HTTP 404)", c.Org)
	}
	if resp.IsStatusFailure() {
		return false, fmt.Errorf("ADO error (HTTP %d): %s", resp.StatusCode(), resp.Status())
	}

	body := resp.String()
	if strings.Contains(body, "<html") || strings.Contains(body, "TF400813") || strings.Contains(body, "signin.live.com") {
		return false, fmt.Errorf("authentication failed: invalid PAT token")
	}

	return true, nil
}

// GetWorkItem fetches a single work item by ID with all fields and relations
func (c *Client) GetWorkItem(id int) (*WorkItem, error) {
	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitems/%d?$expand=all&api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), id)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/workitems/%d?$expand=all&api-version=7.0",
			url.PathEscape(c.Org), id)
	}

	var wi WorkItem
	resp, err := c.R.R().
		SetResult(&wi).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &wi, nil
}

// GetWorkItemsBatch fetches multiple work items by IDs in chunked requests
func (c *Client) GetWorkItemsBatch(ids []int) ([]*WorkItem, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// ADO allows max 200 IDs per request
	var allItems []*WorkItem
	chunkSize := 100

	for i := 0; i < len(ids); i += chunkSize {
		end := i + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[i:end]

		idStrs := make([]string, len(chunk))
		for j, id := range chunk {
			idStrs[j] = strconv.Itoa(id)
		}
		idList := strings.Join(idStrs, ",")

		var endpoint string
		if c.Project != "" {
			endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitems?ids=%s&$expand=all&api-version=7.0",
				url.PathEscape(c.Org), url.PathEscape(c.Project), idList)
		} else {
			endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/workitems?ids=%s&$expand=all&api-version=7.0",
				url.PathEscape(c.Org), idList)
		}

		var batchResp WorkItemBatchResponse
		resp, err := c.R.R().
			SetResult(&batchResp).
			Get(endpoint)
		if err != nil {
			return nil, fmt.Errorf("network error: %w", err)
		}
		if resp.IsStatusFailure() {
			return nil, fmt.Errorf("ADO API error (HTTP %d): %s", resp.StatusCode(), resp.String())
		}

		allItems = append(allItems, batchResp.Value...)
	}

	return allItems, nil
}

// FetchHierarchy recursively fetches a root item and all its descendant children
func (c *Client) FetchHierarchy(rootID int) ([]*WorkItem, error) {
	visited := make(map[int]bool)
	var allWorkItems []*WorkItem

	toFetch := []int{rootID}

	for len(toFetch) > 0 {
		var currentBatch []int
		for _, id := range toFetch {
			if !visited[id] {
				visited[id] = true
				currentBatch = append(currentBatch, id)
			}
		}
		toFetch = nil

		if len(currentBatch) == 0 {
			break
		}

		items, err := c.GetWorkItemsBatch(currentBatch)
		if err != nil {
			return nil, fmt.Errorf("error fetching batch of work items: %w", err)
		}

		for _, item := range items {
			allWorkItems = append(allWorkItems, item)
			// Check child relations
			for _, childID := range item.ChildIDs() {
				if !visited[childID] {
					toFetch = append(toFetch, childID)
				}
			}
		}
	}

	return allWorkItems, nil
}

// QueryInfo represents metadata about an Azure DevOps query
type QueryInfo struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Path            string `json:"path"`
	QueryType       string `json:"queryType"`       // "flat", "tree", "oneHop"
	QueryResultType string `json:"queryResultType"` // "workItems", "workItemHierarchy"
	WIQL            string `json:"wiql"`
}

// QueryResult represents the response from executing an ADO query via WIQL API
type QueryResult struct {
	QueryType         string                  `json:"queryType"`
	QueryResultType   string                  `json:"queryResultType"`
	AsOf              string                  `json:"asOf"`
	WorkItems         []QueryWorkItemRef      `json:"workItems"`
	WorkItemRelations []QueryWorkItemRelation `json:"workItemRelations"`
}

type QueryWorkItemRef struct {
	ID  int    `json:"id"`
	URL string `json:"url"`
}

type QueryWorkItemRelation struct {
	Rel    string            `json:"rel"`
	Source *QueryWorkItemRef `json:"source"`
	Target *QueryWorkItemRef `json:"target"`
}

// GetQuery fetches metadata and WIQL for a given query ID
func (c *Client) GetQuery(queryID string) (*QueryInfo, error) {
	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/queries/%s?$expand=all&api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(queryID))
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/queries/%s?$expand=all&api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(queryID))
	}

	var qInfo QueryInfo
	resp, err := c.R.R().
		SetResult(&qInfo).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error getting query info: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("failed to get query info (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &qInfo, nil
}

// ExecuteQuery runs a query by ID using the ADO WIQL execution endpoint
func (c *Client) ExecuteQuery(queryID string) (*QueryResult, error) {
	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/wiql/%s?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(queryID))
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/wiql/%s?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(queryID))
	}

	var result QueryResult
	resp, err := c.R.R().
		SetResult(&result).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error executing query: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("failed to execute query (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &result, nil
}

// FetchQueryHierarchy executes an ADO query, retrieves all work items, and resolves root IDs
func (c *Client) FetchQueryHierarchy(queryID string) (*QueryInfo, []int, []*WorkItem, error) {
	qInfo, err := c.GetQuery(queryID)
	if err != nil {
		return nil, nil, nil, err
	}

	qResult, err := c.ExecuteQuery(queryID)
	if err != nil {
		return nil, nil, nil, err
	}

	var allIDs []int
	var rootIDs []int
	seen := make(map[int]bool)
	seenRoots := make(map[int]bool)

	if len(qResult.WorkItemRelations) > 0 {
		// Tree or OneHop query
		for _, rel := range qResult.WorkItemRelations {
			if rel.Target != nil && rel.Target.ID > 0 {
				if !seen[rel.Target.ID] {
					seen[rel.Target.ID] = true
					allIDs = append(allIDs, rel.Target.ID)
				}
				// Roots are items without source or with null/empty relation
				if rel.Source == nil || rel.Rel == "" {
					if !seenRoots[rel.Target.ID] {
						seenRoots[rel.Target.ID] = true
						rootIDs = append(rootIDs, rel.Target.ID)
					}
				}
			}
			if rel.Source != nil && rel.Source.ID > 0 {
				if !seen[rel.Source.ID] {
					seen[rel.Source.ID] = true
					allIDs = append(allIDs, rel.Source.ID)
				}
			}
		}
	} else {
		// Flat query
		for _, wi := range qResult.WorkItems {
			if wi.ID > 0 && !seen[wi.ID] {
				seen[wi.ID] = true
				allIDs = append(allIDs, wi.ID)
			}
		}
	}

	if len(allIDs) == 0 {
		return qInfo, nil, nil, nil
	}

	// Fetch all work items in batch
	items, err := c.GetWorkItemsBatch(allIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to fetch query work items: %w", err)
	}

	itemMap := make(map[int]*WorkItem, len(items))
	for _, it := range items {
		itemMap[it.ID] = it
	}

	// Filter rootIDs so that any item whose parent is present in the query items set is NOT a root.
	var validRootIDs []int
	seenValidRoots := make(map[int]bool)
	for _, rID := range rootIDs {
		it := itemMap[rID]
		if it == nil {
			continue
		}
		pID := it.ParentID()
		if pID > 0 && itemMap[pID] != nil {
			// Parent exists in query items; this item belongs under its parent's subtree
			continue
		}
		if !seenValidRoots[rID] {
			seenValidRoots[rID] = true
			validRootIDs = append(validRootIDs, rID)
		}
	}

	// Identify roots for flat queries or if all candidate roots were filtered
	if len(validRootIDs) == 0 {
		for _, it := range items {
			pID := it.ParentID()
			if pID == 0 || itemMap[pID] == nil {
				if !seenValidRoots[it.ID] {
					seenValidRoots[it.ID] = true
					validRootIDs = append(validRootIDs, it.ID)
				}
			}
		}
		if len(validRootIDs) == 0 && len(items) > 0 {
			for _, it := range items {
				validRootIDs = append(validRootIDs, it.ID)
			}
		}
	}

	return qInfo, validRootIDs, items, nil
}

// GetNearestParentIteration returns the IterationPath of the immediate parent work item.
// It does not traverse past the immediate parent if it is empty.
func (c *Client) GetNearestParentIteration(parentID int) string {
	if parentID <= 0 {
		return ""
	}
	wi, err := c.GetWorkItem(parentID)
	if err != nil || wi == nil {
		return ""
	}
	return strings.TrimSpace(wi.IterationPath())
}

// GetNearestParentArea returns the AreaPath of the immediate parent work item.
// It does not traverse past the immediate parent if it is empty.
func (c *Client) GetNearestParentArea(parentID int) string {
	if parentID <= 0 {
		return ""
	}
	wi, err := c.GetWorkItem(parentID)
	if err != nil || wi == nil {
		return ""
	}
	return strings.TrimSpace(wi.AreaPath())
}

// CreateWorkItem creates a new work item in ADO with automated field fallback
func (c *Client) CreateWorkItem(itemType string, fields map[string]interface{}, parentID int) (*WorkItem, error) {
	project := c.Project
	if project == "" {
		return nil, fmt.Errorf("project name is required to create a work item")
	}

	currentFields := make(map[string]interface{})
	for k, v := range fields {
		if v != nil {
			currentFields[k] = v
		}
	}

	// If parent is specified and iteration is not explicitly provided, inherit nearest parent's iteration
	if parentID > 0 && currentFields["System.IterationPath"] == nil && currentFields["IterationPath"] == nil {
		if iter := c.GetNearestParentIteration(parentID); iter != "" {
			currentFields["System.IterationPath"] = iter
		}
	}
	// If parent is specified and area is not explicitly provided, inherit nearest parent's area
	if parentID > 0 && currentFields["System.AreaPath"] == nil && currentFields["AreaPath"] == nil {
		if area := c.GetNearestParentArea(parentID); area != "" {
			currentFields["System.AreaPath"] = area
		}
	}

	triedFields := make(map[string]bool)

	for attempt := 0; attempt < 5; attempt++ {
		var patches []JsonPatchOperation

		for k, v := range currentFields {
			if v == nil {
				continue
			}
			val := v
			if strVal, ok := v.(string); ok && isHTMLField(k) {
				val = c.FormatTextForADO(strVal)
			}
			patches = append(patches, JsonPatchOperation{
				Op:    "add",
				Path:  "/fields/" + k,
				Value: val,
			})
		}

		// If parent is specified, add parent relation
		if parentID > 0 {
			parentURL := fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/workItems/%d", url.PathEscape(c.Org), parentID)
			patches = append(patches, JsonPatchOperation{
				Op:   "add",
				Path: "/relations/-",
				Value: map[string]interface{}{
					"rel": "System.LinkTypes.Hierarchy-Reverse",
					"url": parentURL,
					"attributes": map[string]interface{}{
						"comment": "Parent link created via ado-board-cli",
					},
				},
			})
		}

		endpoint := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitems/$%s?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(project), url.PathEscape(itemType))

		var created WorkItem
		resp, err := c.R.R().
			SetHeader("Content-Type", "application/json-patch+json").
			SetBody(patches).
			SetResult(&created).
			Post(endpoint)
		if err != nil {
			return nil, fmt.Errorf("network error: %w", err)
		}
		if !resp.IsStatusFailure() {
			return &created, nil
		}

		errMsg := resp.String()
		missingField := extractMissingField(errMsg)
		if missingField == "" {
			return nil, fmt.Errorf("ADO API error (HTTP %d): %s", resp.StatusCode(), errMsg)
		}

		triedFields[missingField] = true
		val, hasVal := currentFields[missingField]
		delete(currentFields, missingField)

		substituted := false
		if hasVal {
			if fallbacks, ok := fieldFallbacks[missingField]; ok {
				for _, fb := range fallbacks {
					if !triedFields[fb] {
						currentFields[fb] = val
						triedFields[fb] = true
						substituted = true
						break
					}
				}
			}
		}

		if !substituted && len(currentFields) == 0 {
			return nil, fmt.Errorf("ADO API error (HTTP %d): %s", resp.StatusCode(), errMsg)
		}
	}

	return nil, fmt.Errorf("failed to create %s in ADO after retries", itemType)
}

// UpdateWorkItem updates work item fields using JSON patch
func (c *Client) UpdateWorkItem(id int, patches []JsonPatchOperation) (*WorkItem, error) {
	// Auto-format rich text fields if passed as plain markdown
	for i := range patches {
		if strings.HasPrefix(patches[i].Path, "/fields/") {
			fieldName := strings.TrimPrefix(patches[i].Path, "/fields/")
			if strVal, ok := patches[i].Value.(string); ok && isHTMLField(fieldName) {
				patches[i].Value = c.FormatTextForADO(strVal)
			}
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitems/%d?$expand=all&api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), id)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/workitems/%d?$expand=all&api-version=7.0",
			url.PathEscape(c.Org), id)
	}

	var updated WorkItem
	resp, err := c.R.R().
		SetHeader("Content-Type", "application/json-patch+json").
		SetBody(patches).
		SetResult(&updated).
		Patch(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &updated, nil
}

// AssignWorkItem assigns a work item to an email/identity or removes assignment if empty
func (c *Client) AssignWorkItem(id int, assignee string) (*WorkItem, error) {
	assignee = strings.TrimSpace(assignee)
	if assignee == "" || strings.EqualFold(assignee, "unassigned") || strings.EqualFold(assignee, "none") {
		patches := []JsonPatchOperation{
			{
				Op:   "remove",
				Path: "/fields/System.AssignedTo",
			},
		}
		item, err := c.UpdateWorkItem(id, patches)
		if err != nil {
			// If removal fails because the field wasn't present on the item, fetch and return current
			errMsg := strings.ToLower(err.Error())
			if strings.Contains(errMsg, "not present") || strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "not allowed") {
				return c.GetWorkItem(id)
			}
			return nil, err
		}
		return item, nil
	}

	return c.UpdateWorkItemFields(id, map[string]interface{}{
		"System.AssignedTo": assignee,
	})
}

// UpdateWorkItemFields is a convenience method to update specific fields with automated field fallback
func (c *Client) UpdateWorkItemFields(id int, fields map[string]interface{}) (*WorkItem, error) {
	currentFields := make(map[string]interface{})
	for k, v := range fields {
		currentFields[k] = v
	}

	triedFields := make(map[string]bool)

	for attempt := 0; attempt < 5; attempt++ {
		var patches []JsonPatchOperation
		for k, v := range currentFields {
			if (k == "System.IterationPath" || k == "System.AreaPath") && (v == nil || strings.TrimSpace(fmt.Sprint(v)) == "") {
				continue
			}
			if (k == "System.AssignedTo" && (v == nil || v == "" || strings.EqualFold(fmt.Sprint(v), "unassigned"))) || v == nil || (v == "" && isDateOrNumericField(k)) {
				patches = append(patches, JsonPatchOperation{
					Op:   "remove",
					Path: "/fields/" + k,
				})
			} else {
				val := v
				if strVal, ok := v.(string); ok && isHTMLField(k) {
					val = c.FormatTextForADO(strVal)
				}
				patches = append(patches, JsonPatchOperation{
					Op:    "add",
					Path:  "/fields/" + k,
					Value: val,
				})
			}
		}

		if len(patches) == 0 {
			return c.GetWorkItem(id)
		}

		item, err := c.UpdateWorkItem(id, patches)
		if err == nil {
			if fullItem, errFull := c.GetWorkItem(id); errFull == nil && fullItem != nil {
				return fullItem, nil
			}
			return item, nil
		}

		// Check if error is due to unknown/unsupported/rejected fields
		failingFields := extractFailingFields(err.Error())
		if len(failingFields) == 0 {
			return nil, err
		}

		modified := false
		for _, failing := range failingFields {
			triedFields[failing] = true
			val, hasVal := currentFields[failing]
			if hasVal {
				delete(currentFields, failing)
				modified = true

				// If this was a remove operation on a field that does not exist in ADO,
				// it is already absent, so we don't need to retry it.
				if val == nil || (val == "" && isDateOrNumericField(failing)) {
					continue
				}

				// Check if there is an alternative fallback field to try
				if fallbacks, ok := fieldFallbacks[failing]; ok {
					for _, fb := range fallbacks {
						if !triedFields[fb] {
							currentFields[fb] = val
							triedFields[fb] = true
							break
						}
					}
				}
			} else {
				// Also check if any key in currentFields matches case-insensitively
				for k, v := range currentFields {
					if strings.EqualFold(k, failing) {
						delete(currentFields, k)
						modified = true
						if fallbacks, ok := fieldFallbacks[k]; ok {
							for _, fb := range fallbacks {
								if !triedFields[fb] {
									currentFields[fb] = v
									triedFields[fb] = true
									break
								}
							}
						}
					}
				}
			}
		}

		if !modified || len(currentFields) == 0 {
			if len(currentFields) == 0 {
				return c.GetWorkItem(id)
			}
			return nil, err
		}
	}

	return nil, fmt.Errorf("failed to update work item #%d after retries", id)
}

// DeleteWorkItem deletes a work item (soft delete by default, or destroy)
func (c *Client) DeleteWorkItem(id int, destroy bool) error {
	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitems/%d?destroy=%t&api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), id, destroy)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/workitems/%d?destroy=%t&api-version=7.0",
			url.PathEscape(c.Org), id, destroy)
	}

	resp, err := c.R.R().Delete(endpoint)
	if err != nil {
		return fmt.Errorf("network error: %w", err)
	}
	if resp.IsStatusFailure() {
		return fmt.Errorf("ADO API error (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return nil
}

// GetComments fetches all comments for a work item
func (c *Client) GetComments(id int) ([]*CommentItem, error) {
	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitems/%d/comments?$expand=renderedText,mentions&api-version=7.0-preview",
			url.PathEscape(c.Org), url.PathEscape(c.Project), id)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/workitems/%d/comments?$expand=renderedText,mentions&api-version=7.0-preview",
			url.PathEscape(c.Org), id)
	}

	var commentList CommentList
	resp, err := c.R.R().
		SetResult(&commentList).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return commentList.Comments, nil
}

// GetWorkItemUpdates fetches the revision/update history for a work item
func (c *Client) GetWorkItemUpdates(id int) ([]*WorkItemUpdate, error) {
	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitems/%d/updates?$top=200&api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), id)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/workitems/%d/updates?$top=200&api-version=7.0",
			url.PathEscape(c.Org), id)
	}

	var updateList WorkItemUpdateList
	resp, err := c.R.R().
		SetResult(&updateList).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error fetching updates for #%d: %w", id, err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error fetching updates for #%d (HTTP %d): %s", id, resp.StatusCode(), resp.String())
	}

	return updateList.Value, nil
}

// AddComment posts a new comment on a work item
func (c *Client) AddComment(id int, text string) (*CommentItem, error) {
	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitems/%d/comments?api-version=7.0-preview",
			url.PathEscape(c.Org), url.PathEscape(c.Project), id)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/wit/workitems/%d/comments?api-version=7.0-preview",
			url.PathEscape(c.Org), id)
	}

	formattedText := c.FormatTextForADO(text)
	payload := map[string]string{
		"text": formattedText,
	}

	var comment CommentItem
	resp, err := c.R.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		SetResult(&comment).
		Post(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &comment, nil
}

// GetWorkItemTypes returns available work item types for the project
func (c *Client) GetWorkItemTypes() ([]string, error) {
	if c.Project == "" {
		return []string{"Task", "Bug", "User Story", "Technical Enablement", "Feature", "Epic"}, nil
	}

	endpoint := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitemtypes?api-version=7.0",
		url.PathEscape(c.Org), url.PathEscape(c.Project))

	var result struct {
		Value []WorkItemTypeInfo `json:"value"`
	}

	resp, err := c.R.R().
		SetResult(&result).
		Get(endpoint)
	if err != nil || resp.IsStatusFailure() {
		// Fallback to defaults
		return []string{"Task", "Bug", "User Story", "Technical Enablement", "Feature", "Epic"}, nil
	}

	var types []string
	for _, t := range result.Value {
		if strings.HasPrefix(t.Name, "Feedback") || strings.HasPrefix(t.Name, "Code Review") || strings.HasPrefix(t.Name, "Shared") {
			continue
		}
		types = append(types, t.Name)
	}
	if len(types) == 0 {
		types = []string{"Task", "Bug", "User Story", "Technical Enablement", "Feature", "Epic"}
	}
	return types, nil
}

// GetWorkItemStates returns the states for a specific work item type
func (c *Client) GetWorkItemStates(itemType string) ([]WorkItemState, error) {
	defaultStates := []WorkItemState{
		{Name: "To Do", Category: "Proposed"},
		{Name: "In Progress", Category: "InProgress"},
		{Name: "Committed", Category: "InProgress"},
		{Name: "Done", Category: "Completed"},
		{Name: "Removed", Category: "Removed"},
	}

	if c.Project == "" || itemType == "" {
		return defaultStates, nil
	}

	endpoint := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitemtypes/%s/states?api-version=7.0",
		url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(itemType))

	var result struct {
		Value []WorkItemState `json:"value"`
	}

	resp, err := c.R.R().
		SetResult(&result).
		Get(endpoint)
	if err != nil || resp.IsStatusFailure() {
		return defaultStates, nil
	}

	return result.Value, nil
}

// GetWorkItemTypeFields returns the fields supported by a given work item type
func (c *Client) GetWorkItemTypeFields(itemType string) ([]WorkItemTypeField, error) {
	if c.Project == "" || itemType == "" {
		return nil, fmt.Errorf("project and itemType are required")
	}

	endpoint := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/wit/workitemtypes/%s/fields?api-version=7.0",
		url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(itemType))

	var result struct {
		Value []WorkItemTypeField `json:"value"`
	}

	resp, err := c.R.R().
		SetResult(&result).
		Get(endpoint)
	if err != nil || resp.IsStatusFailure() {
		return nil, fmt.Errorf("failed to fetch fields for work item type %q", itemType)
	}

	return result.Value, nil
}

// GetPullRequest fetches details of a specific Pull Request
func (c *Client) GetPullRequest(repoID string, prID int) (*GitPullRequest, error) {
	var endpoint string
	if repoID != "" {
		if c.Project != "" {
			endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/%d?api-version=7.0",
				url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID)
		} else {
			endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullrequests/%d?api-version=7.0",
				url.PathEscape(c.Org), url.PathEscape(repoID), prID)
		}
	} else {
		if c.Project != "" {
			endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/pullrequests/%d?api-version=7.0",
				url.PathEscape(c.Org), url.PathEscape(c.Project), prID)
		} else {
			endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/pullrequests/%d?api-version=7.0",
				url.PathEscape(c.Org), prID)
		}
	}

	var pr GitPullRequest
	resp, err := c.R.R().
		SetResult(&pr).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error fetching PR #%d: %w", prID, err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error fetching PR #%d (HTTP %d): %s", prID, resp.StatusCode(), resp.String())
	}

	return &pr, nil
}

// GetPullRequestsForProject fetches recent pull requests across the project for autocomplete & caching
func (c *Client) GetPullRequestsForProject(top int) ([]*GitPullRequest, error) {
	if top <= 0 {
		top = 50
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/pullrequests?searchCriteria.status=all&$top=%d&api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), top)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/pullrequests?searchCriteria.status=all&$top=%d&api-version=7.0",
			url.PathEscape(c.Org), top)
	}

	var list GitPullRequestList
	resp, err := c.R.R().
		SetResult(&list).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error fetching PRs: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error fetching PRs (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return list.Value, nil
}

// UploadPullRequestAttachment uploads a binary or text attachment file to a Pull Request.
// If repoID is empty, it will be automatically discovered by fetching PR metadata.
func (c *Client) UploadPullRequestAttachment(repoID string, prID int, fileName string, content []byte) (*PRAttachment, error) {
	if repoID == "" {
		pr, err := c.GetPullRequest("", prID)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repository for PR #%d: %w", prID, err)
		}
		if pr.Repository != nil && pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else if pr.Repository != nil && pr.Repository.Name != "" {
			repoID = pr.Repository.Name
		} else {
			return nil, fmt.Errorf("could not determine repository for PR #%d", prID)
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/attachments/%s?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID, url.PathEscape(fileName))
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullRequests/%d/attachments/%s?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID), prID, url.PathEscape(fileName))
	}

	var att PRAttachment
	resp, err := c.R.R().
		SetHeader("Content-Type", "application/octet-stream").
		SetBody(content).
		SetResult(&att).
		Post(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error uploading PR attachment: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error uploading PR attachment (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &att, nil
}

// UpdatePullRequestParams specifies fields to update on an Azure DevOps Git Pull Request
type UpdatePullRequestParams struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	IsDraft     *bool   `json:"isDraft,omitempty"`
}

// UpdatePullRequest updates title, description, or draft status of a pull request.
// If repoID is empty, it is automatically resolved by querying the PR.
func (c *Client) UpdatePullRequest(repoID string, prID int, params UpdatePullRequestParams) (*GitPullRequest, error) {
	if repoID == "" {
		pr, err := c.GetPullRequest("", prID)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repository for PR #%d: %w", prID, err)
		}
		if pr.Repository != nil && pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else if pr.Repository != nil && pr.Repository.Name != "" {
			repoID = pr.Repository.Name
		} else {
			return nil, fmt.Errorf("could not determine repository for PR #%d", prID)
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/%d?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullrequests/%d?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID), prID)
	}

	var updated GitPullRequest
	resp, err := c.R.R().
		SetHeader("Content-Type", "application/json").
		SetBody(params).
		SetResult(&updated).
		Patch(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error updating PR #%d: %w", prID, err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error updating PR #%d (HTTP %d): %s", prID, resp.StatusCode(), resp.String())
	}

	return &updated, nil
}

// LinkPullRequestToWorkItem associates a Pull Request to a Work Item using ADO's ArtifactLink relation.
func (c *Client) LinkPullRequestToWorkItem(repoID string, prID, workItemID int, comment string) (*WorkItem, *GitPullRequest, error) {
	pr, err := c.GetPullRequest(repoID, prID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch PR #%d: %w", prID, err)
	}

	projectID := c.Project
	resolvedRepoID := repoID
	if pr.Repository != nil {
		if pr.Repository.Project.ID != "" {
			projectID = pr.Repository.Project.ID
		} else if pr.Repository.Project.Name != "" {
			projectID = pr.Repository.Project.Name
		}
		if pr.Repository.ID != "" {
			resolvedRepoID = pr.Repository.ID
		} else if pr.Repository.Name != "" {
			resolvedRepoID = pr.Repository.Name
		}
	}

	if projectID == "" {
		projectID = c.Project
	}
	if resolvedRepoID == "" {
		return nil, nil, fmt.Errorf("repository ID could not be determined for PR #%d", prID)
	}

	// ADO Artifact Link URI format: vstfs:///Git/PullRequestId/{project}%2F{repo}%2F{prID}
	artifactURL := fmt.Sprintf("vstfs:///Git/PullRequestId/%s%%2F%s%%2F%d",
		url.PathEscape(projectID), url.PathEscape(resolvedRepoID), prID)

	attributes := map[string]interface{}{
		"name": "Pull Request",
	}
	if comment != "" {
		attributes["comment"] = comment
	}

	patches := []JsonPatchOperation{
		{
			Op:   "add",
			Path: "/relations/-",
			Value: map[string]interface{}{
				"rel":        "ArtifactLink",
				"url":        artifactURL,
				"attributes": attributes,
			},
		},
	}

	updatedItem, err := c.UpdateWorkItem(workItemID, patches)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to link PR #%d to work item #%d: %w", prID, workItemID, err)
	}

	// Also call PR workitems API endpoint for complete bidirectional association
	prWorkItemsURL := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/workitems?api-version=7.0",
		url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(resolvedRepoID), prID)
	payload := []map[string]interface{}{
		{"id": fmt.Sprintf("%d", workItemID)},
	}
	_, _ = c.R.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(prWorkItemsURL)

	return updatedItem, pr, nil
}

// ListRepositories lists all Git repositories in the project or organization
func (c *Client) ListRepositories() ([]*GitRepository, error) {
	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project))
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories?api-version=7.0",
			url.PathEscape(c.Org))
	}

	var list GitRepositoryList
	resp, err := c.R.R().
		SetResult(&list).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error listing repositories: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error listing repositories (HTTP %d): %s", resp.StatusCode(), resp.String())
	}
	return list.Value, nil
}

// CreatePullRequest creates a new Git Pull Request
func (c *Client) CreatePullRequest(repoID string, params CreatePullRequestParams) (*GitPullRequest, error) {
	if repoID == "" {
		detectedName := DetectLocalGitRepoName("")
		repos, err := c.ListRepositories()
		if err != nil || len(repos) == 0 {
			return nil, fmt.Errorf("could not discover default repository to create PR: %v", err)
		}

		// 1. Try matching detected local git repository name against project repositories
		if detectedName != "" {
			for _, r := range repos {
				if strings.EqualFold(r.Name, detectedName) || strings.EqualFold(r.ID, detectedName) {
					repoID = r.ID
					break
				}
			}
		}

		// 2. Try matching project name if repoID is still empty
		if repoID == "" {
			for _, r := range repos {
				if strings.EqualFold(r.Name, c.Project) {
					repoID = r.ID
					break
				}
			}
		}

		// 3. If only one repository exists in the project, use it
		if repoID == "" && len(repos) == 1 {
			repoID = repos[0].ID
		}

		// 4. If still not resolved and multiple repositories exist, return a descriptive error
		if repoID == "" {
			var names []string
			for _, r := range repos {
				names = append(names, r.Name)
			}
			if detectedName != "" {
				return nil, fmt.Errorf("detected local git repository %q, but found no matching repository in project %q (available repositories: %s). Please specify 'repository_id'", detectedName, c.Project, strings.Join(names, ", "))
			}
			return nil, fmt.Errorf("multiple repositories found in project %q (%s). Please specify 'repository_id'", c.Project, strings.Join(names, ", "))
		}
	}

	if !strings.HasPrefix(params.SourceRefName, "refs/heads/") {
		params.SourceRefName = "refs/heads/" + strings.TrimPrefix(params.SourceRefName, "refs/heads/")
	}
	if params.TargetRefName == "" {
		params.TargetRefName = "refs/heads/main"
	} else if !strings.HasPrefix(params.TargetRefName, "refs/heads/") {
		params.TargetRefName = "refs/heads/" + strings.TrimPrefix(params.TargetRefName, "refs/heads/")
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID))
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullrequests?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID))
	}

	var pr GitPullRequest
	resp, err := c.R.R().
		SetHeader("Content-Type", "application/json").
		SetBody(params).
		SetResult(&pr).
		Post(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error creating pull request: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error creating pull request (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &pr, nil
}

// AddPullRequestReviewer adds or updates a reviewer on a Pull Request.
func (c *Client) AddPullRequestReviewer(repoID string, prID int, reviewerID string, isRequired bool) (*PullRequestReviewer, error) {
	if repoID == "" {
		pr, err := c.GetPullRequest("", prID)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repository for PR #%d: %w", prID, err)
		}
		if pr.Repository != nil && pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else if pr.Repository != nil && pr.Repository.Name != "" {
			repoID = pr.Repository.Name
		} else {
			return nil, fmt.Errorf("could not determine repository for PR #%d", prID)
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/reviewers/%s?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID, url.PathEscape(reviewerID))
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullRequests/%d/reviewers/%s?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID), prID, url.PathEscape(reviewerID))
	}

	body := map[string]interface{}{
		"id":         reviewerID,
		"isRequired": isRequired,
	}

	var reviewer PullRequestReviewer
	resp, err := c.R.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&reviewer).
		Put(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error adding reviewer to PR #%d: %w", prID, err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error adding reviewer to PR #%d (HTTP %d): %s", prID, resp.StatusCode(), resp.String())
	}

	return &reviewer, nil
}

// GetPullRequestAttachments fetches all attachments uploaded to a Pull Request.
func (c *Client) GetPullRequestAttachments(repoID string, prID int) ([]*PRAttachment, error) {
	if repoID == "" {
		pr, err := c.GetPullRequest("", prID)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repository for PR #%d: %w", prID, err)
		}
		if pr.Repository != nil && pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else if pr.Repository != nil && pr.Repository.Name != "" {
			repoID = pr.Repository.Name
		} else {
			return nil, fmt.Errorf("could not determine repository for PR #%d", prID)
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/attachments?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullRequests/%d/attachments?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID), prID)
	}

	var list PRAttachmentList
	resp, err := c.R.R().
		SetResult(&list).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error fetching PR attachments: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error fetching PR attachments (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return list.Value, nil
}

// GetPullRequestWorkItems fetches work item references associated with a Pull Request.
func (c *Client) GetPullRequestWorkItems(repoID string, prID int) ([]*PRWorkItemRef, error) {
	if repoID == "" {
		pr, err := c.GetPullRequest("", prID)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repository for PR #%d: %w", prID, err)
		}
		if pr.Repository != nil && pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else if pr.Repository != nil && pr.Repository.Name != "" {
			repoID = pr.Repository.Name
		} else {
			return nil, fmt.Errorf("could not determine repository for PR #%d", prID)
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/workitems?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullRequests/%d/workitems?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID), prID)
	}

	var list PRWorkItemList
	resp, err := c.R.R().
		SetResult(&list).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error fetching PR work items: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error fetching PR work items (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return list.Value, nil
}

// GetPullRequestDetails retrieves comprehensive pull request details including attachments and linked work items.
func (c *Client) GetPullRequestDetails(repoID string, prID int) (*PullRequestDetailed, error) {
	pr, err := c.GetPullRequest(repoID, prID)
	if err != nil {
		return nil, err
	}

	if repoID == "" && pr.Repository != nil {
		if pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else {
			repoID = pr.Repository.Name
		}
	}

	attachments, _ := c.GetPullRequestAttachments(repoID, prID)
	workItems, _ := c.GetPullRequestWorkItems(repoID, prID)

	webURL := pr.URL
	if pr.Repository != nil {
		proj := c.Project
		if pr.Repository.Project.Name != "" {
			proj = pr.Repository.Project.Name
		}
		webURL = fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s/pullrequest/%d",
			c.Org, proj, pr.Repository.Name, pr.PullRequestID)
	}

	return &PullRequestDetailed{
		PullRequest: pr,
		Attachments: attachments,
		WorkItems:   workItems,
		WebURL:      webURL,
	}, nil
}

// GetPullRequestThreads retrieves all discussion threads and review comments for a pull request.
// If repoID is empty, it will automatically be discovered by querying the PR metadata.
func (c *Client) GetPullRequestThreads(repoID string, prID int) ([]*PullRequestThread, error) {
	if repoID == "" {
		pr, err := c.GetPullRequest("", prID)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repository for PR #%d: %w", prID, err)
		}
		if pr.Repository != nil && pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else if pr.Repository != nil && pr.Repository.Name != "" {
			repoID = pr.Repository.Name
		} else {
			return nil, fmt.Errorf("could not determine repository for PR #%d", prID)
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullRequests/%d/threads?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID), prID)
	}

	var threadList PullRequestThreadList
	resp, err := c.R.R().
		SetResult(&threadList).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error fetching PR threads: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error fetching PR threads (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return threadList.Value, nil
}

// AddPullRequestThreadComment adds a comment reply to an existing pull request review thread.
// If repoID is empty, it will automatically be discovered by querying the PR metadata.
func (c *Client) AddPullRequestThreadComment(repoID string, prID int, threadID int, content string, parentCommentID int) (*PullRequestComment, error) {
	if repoID == "" {
		pr, err := c.GetPullRequest("", prID)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repository for PR #%d: %w", prID, err)
		}
		if pr.Repository != nil && pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else if pr.Repository != nil && pr.Repository.Name != "" {
			repoID = pr.Repository.Name
		} else {
			return nil, fmt.Errorf("could not determine repository for PR #%d", prID)
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d/comments?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID, threadID)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d/comments?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID), prID, threadID)
	}

	body := map[string]any{
		"content":     content,
		"commentType": 1, // 1 = text
	}
	if parentCommentID > 0 {
		body["parentCommentId"] = parentCommentID
	}

	var comment PullRequestComment
	resp, err := c.R.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&comment).
		Post(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error posting PR comment: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error posting PR comment (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &comment, nil
}

// UpdatePullRequestThread updates the status of an existing pull request thread.
// If repoID is empty, it will automatically be discovered by querying the PR metadata.
func (c *Client) UpdatePullRequestThread(repoID string, prID int, threadID int, status string) (*PullRequestThread, error) {
	if repoID == "" {
		pr, err := c.GetPullRequest("", prID)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repository for PR #%d: %w", prID, err)
		}
		if pr.Repository != nil && pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else if pr.Repository != nil && pr.Repository.Name != "" {
			repoID = pr.Repository.Name
		} else {
			return nil, fmt.Errorf("could not determine repository for PR #%d", prID)
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID, threadID)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullRequests/%d/threads/%d?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID), prID, threadID)
	}

	body := map[string]any{
		"status": status,
	}

	var thread PullRequestThread
	resp, err := c.R.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&thread).
		Patch(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error updating PR thread: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error updating PR thread (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &thread, nil
}

// CreatePullRequestThread creates a new review comment thread on a pull request.
// It supports general PR comments as well as inline file and code selection comments.
// If repoID is empty, it will automatically be discovered by querying the PR metadata.
func (c *Client) CreatePullRequestThread(repoID string, prID int, content string, threadContext *PullRequestThreadContext, status string) (*PullRequestThread, error) {
	if repoID == "" {
		pr, err := c.GetPullRequest("", prID)
		if err != nil {
			return nil, fmt.Errorf("failed to discover repository for PR #%d: %w", prID, err)
		}
		if pr.Repository != nil && pr.Repository.ID != "" {
			repoID = pr.Repository.ID
		} else if pr.Repository != nil && pr.Repository.Name != "" {
			repoID = pr.Repository.Name
		} else {
			return nil, fmt.Errorf("could not determine repository for PR #%d", prID)
		}
	}

	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullRequests/%d/threads?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(repoID), prID)
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/git/repositories/%s/pullRequests/%d/threads?api-version=7.0",
			url.PathEscape(c.Org), url.PathEscape(repoID), prID)
	}

	if status == "" {
		status = "active"
	}

	body := map[string]any{
		"comments": []map[string]any{
			{
				"parentCommentId": 0,
				"content":         content,
				"commentType":     1, // 1 = text
			},
		},
		"status": status,
	}

	if threadContext != nil {
		body["threadContext"] = threadContext
	}

	var thread PullRequestThread
	resp, err := c.R.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&thread).
		Post(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error creating PR review comment thread: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error creating PR review comment thread (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	return &thread, nil
}

// FetchProjectTeamMembers queries all project teams and gathers all team members across the project
func (c *Client) FetchProjectTeamMembers() ([]*IdentityRef, error) {
	var endpoint string
	if c.Project != "" {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/projects/%s/teams?api-version=7.0&$top=200",
			url.PathEscape(c.Org), url.PathEscape(c.Project))
	} else {
		endpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/teams?api-version=7.0&$top=200",
			url.PathEscape(c.Org))
	}

	var teamList ProjectTeamList
	resp, err := c.R.R().
		SetResult(&teamList).
		Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("network error fetching teams: %w", err)
	}
	if resp.IsStatusFailure() {
		return nil, fmt.Errorf("ADO API error fetching teams (HTTP %d): %s", resp.StatusCode(), resp.String())
	}

	// Concurrently fetch members from all teams
	memberMap := make(map[string]*IdentityRef)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Concurrency limiter (up to 8 parallel requests)
	sem := make(chan struct{}, 8)

	for _, team := range teamList.Value {
		teamID := team.ID
		wg.Add(1)
		go func(tID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var memEndpoint string
			if c.Project != "" {
				memEndpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/projects/%s/teams/%s/members?api-version=7.0",
					url.PathEscape(c.Org), url.PathEscape(c.Project), url.PathEscape(tID))
			} else {
				memEndpoint = fmt.Sprintf("https://dev.azure.com/%s/_apis/teams/%s/members?api-version=7.0",
					url.PathEscape(c.Org), url.PathEscape(tID))
			}

			var members TeamMemberList
			memResp, memErr := c.R.R().
				SetResult(&members).
				Get(memEndpoint)
			if memErr != nil || memResp.IsStatusFailure() {
				return
			}

			mu.Lock()
			for _, m := range members.Value {
				id := strings.ToLower(strings.TrimSpace(m.Identity.ID))
				email := strings.ToLower(strings.TrimSpace(m.Identity.UniqueName))
				key := id
				if key == "" {
					key = email
				}
				if key != "" && m.Identity.DisplayName != "" {
					itemCopy := m.Identity
					memberMap[key] = &itemCopy
				}
			}
			mu.Unlock()
		}(teamID)
	}

	wg.Wait()

	var result []*IdentityRef
	for _, m := range memberMap {
		result = append(result, m)
	}
	return result, nil
}

