package ado

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// TrackTargetType indicates whether a link/input is a work item or an ADO query
type TrackTargetType int

const (
	TrackTargetWorkItem TrackTargetType = iota
	TrackTargetQuery
)

// ParsedTrackTarget represents the parsed components of an Azure DevOps work item or query reference
type ParsedTrackTarget struct {
	Type     TrackTargetType
	Org      string
	Project  string
	ItemID   int    // For work items
	QueryID  string // For queries (UUID)
	RawURL   string
}

// ParsedLink represents the parsed components of an Azure DevOps work item reference
type ParsedLink struct {
	Org     string
	Project string
	ID      int
	RawURL  string
}

var (
	// Regex for dev.azure.com/{org}/{project}/_workitems/edit/{id}
	// or dev.azure.com/{org}/_workitems/edit/{id}
	devAzureRegex = regexp.MustCompile(`(?i)https?://dev\.azure\.com/([^/]+)(?:/([^/]+))?/_workitems/edit/(\d+)`)

	// Regex for {org}.visualstudio.com/{project}/_workitems/edit/{id}
	// or {org}.visualstudio.com/DefaultCollection/{project}/_workitems/edit/{id}
	vsRegex = regexp.MustCompile(`(?i)https?://([^.]+)\.visualstudio\.com(?:/DefaultCollection)?(?:/([^/]+))?/_workitems/edit/(\d+)`)

	// Regex for dev.azure.com/{org}/{project}/_apis/wit/workitems/{id}
	devAzureApiRegex = regexp.MustCompile(`(?i)https?://dev\.azure\.com/([^/]+)(?:/([^/]+))?/_apis/wit/work[iI]tems/(\d+)`)

	// Regex for dev.azure.com/{org}/{project}/_queries/query/{queryId} or _queries/query-edit/{queryId}
	devAzureQueryRegex = regexp.MustCompile(`(?i)https?://dev\.azure\.com/([^/]+)(?:/([^/]+))?/_queries/query(?:-edit)?/([0-9a-fA-F-]{36})`)

	// Regex for dev.azure.com/{org}/{project}/_apis/wit/queries/{queryId}
	devAzureQueryApiRegex = regexp.MustCompile(`(?i)https?://dev\.azure\.com/([^/]+)(?:/([^/]+))?/_apis/wit/queries/([0-9a-fA-F-]{36})`)

	// Regex for {org}.visualstudio.com/{project}/_queries/query/{queryId}
	vsQueryRegex = regexp.MustCompile(`(?i)https?://([^.]+)\.visualstudio\.com(?:/DefaultCollection)?(?:/([^/]+))?/_queries/query(?:-edit)?/([0-9a-fA-F-]{36})`)

	// UUID regex
	uuidRegex = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

	// Numeric only ID
	numericRegex = regexp.MustCompile(`^\d+$`)
)

// ParseTrackTarget parses either a work item URL/ID or an Azure DevOps query URL/UUID
func ParseTrackTarget(input string, defaultOrg, defaultProject string) (*ParsedTrackTarget, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty work item or query link")
	}

	// Case 1: Standard dev.azure.com Query URL
	if matches := devAzureQueryRegex.FindStringSubmatch(input); len(matches) >= 4 {
		org := matches[1]
		project := matches[2]
		queryID := strings.ToLower(matches[3])
		if project == "" {
			project = defaultProject
		}
		if unescaped, err := url.PathUnescape(project); err == nil {
			project = unescaped
		}
		return &ParsedTrackTarget{
			Type:    TrackTargetQuery,
			Org:     org,
			Project: project,
			QueryID: queryID,
			RawURL:  input,
		}, nil
	}

	// Case 2: Query API URL
	if matches := devAzureQueryApiRegex.FindStringSubmatch(input); len(matches) >= 4 {
		org := matches[1]
		project := matches[2]
		queryID := strings.ToLower(matches[3])
		if project == "" {
			project = defaultProject
		}
		if unescaped, err := url.PathUnescape(project); err == nil {
			project = unescaped
		}
		return &ParsedTrackTarget{
			Type:    TrackTargetQuery,
			Org:     org,
			Project: project,
			QueryID: queryID,
			RawURL:  input,
		}, nil
	}

	// Case 3: visualstudio.com Query URL
	if matches := vsQueryRegex.FindStringSubmatch(input); len(matches) >= 4 {
		org := matches[1]
		project := matches[2]
		queryID := strings.ToLower(matches[3])
		if project == "" {
			project = defaultProject
		}
		if unescaped, err := url.PathUnescape(project); err == nil {
			project = unescaped
		}
		return &ParsedTrackTarget{
			Type:    TrackTargetQuery,
			Org:     org,
			Project: project,
			QueryID: queryID,
			RawURL:  input,
		}, nil
	}

	// Case 4: Standalone Query UUID
	cleanedUUID := strings.Trim(input, "{}")
	if uuidRegex.MatchString(cleanedUUID) {
		return &ParsedTrackTarget{
			Type:    TrackTargetQuery,
			Org:     defaultOrg,
			Project: defaultProject,
			QueryID: strings.ToLower(cleanedUUID),
			RawURL:  input,
		}, nil
	}

	// Work item link parsing
	wiLink, err := ParseWorkItemLink(input, defaultOrg, defaultProject)
	if err != nil {
		return nil, err
	}

	return &ParsedTrackTarget{
		Type:    TrackTargetWorkItem,
		Org:     wiLink.Org,
		Project: wiLink.Project,
		ItemID:  wiLink.ID,
		RawURL:  wiLink.RawURL,
	}, nil
}

// ParseWorkItemLink parses a work item URL or ID into ParsedLink
func ParseWorkItemLink(input string, defaultOrg, defaultProject string) (*ParsedLink, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("empty work item link or ID")
	}

	// Case 1: Pure numeric ID (with optional # or id: prefix)
	cleaned := strings.TrimLeft(input, "#")
	cleaned = strings.TrimSpace(cleaned)
	if numericRegex.MatchString(cleaned) {
		id, err := strconv.Atoi(cleaned)
		if err != nil {
			return nil, fmt.Errorf("invalid work item ID: %s", cleaned)
		}
		return &ParsedLink{
			Org:     defaultOrg,
			Project: defaultProject,
			ID:      id,
			RawURL:  input,
		}, nil
	}

	// Case 2: Standard dev.azure.com URL
	if matches := devAzureRegex.FindStringSubmatch(input); len(matches) >= 4 {
		org := matches[1]
		project := matches[2]
		idStr := matches[3]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid ID in URL: %s", idStr)
		}
		if project == "" {
			project = defaultProject
		}
		// Unescape project name if needed (e.g. %20 -> space)
		if unescaped, err := url.PathUnescape(project); err == nil {
			project = unescaped
		}
		return &ParsedLink{
			Org:     org,
			Project: project,
			ID:      id,
			RawURL:  input,
		}, nil
	}

	// Case 3: dev.azure.com API URL
	if matches := devAzureApiRegex.FindStringSubmatch(input); len(matches) >= 4 {
		org := matches[1]
		project := matches[2]
		idStr := matches[3]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid ID in API URL: %s", idStr)
		}
		if project == "" {
			project = defaultProject
		}
		if unescaped, err := url.PathUnescape(project); err == nil {
			project = unescaped
		}
		return &ParsedLink{
			Org:     org,
			Project: project,
			ID:      id,
			RawURL:  input,
		}, nil
	}

	// Case 4: {org}.visualstudio.com URL
	if matches := vsRegex.FindStringSubmatch(input); len(matches) >= 4 {
		org := matches[1]
		project := matches[2]
		idStr := matches[3]
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid ID in visualstudio URL: %s", idStr)
		}
		if project == "" {
			project = defaultProject
		}
		if unescaped, err := url.PathUnescape(project); err == nil {
			project = unescaped
		}
		return &ParsedLink{
			Org:     org,
			Project: project,
			ID:      id,
			RawURL:  input,
		}, nil
	}

	// Case 5: Generic URL with an ID at the end or in path
	u, err := url.Parse(input)
	if err == nil && u.Host != "" {
		// Try to extract numbers from path
		segments := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i := len(segments) - 1; i >= 0; i-- {
			if id, err := strconv.Atoi(segments[i]); err == nil && id > 0 {
				org := defaultOrg
				project := defaultProject
				if strings.Contains(u.Host, "dev.azure.com") && len(segments) > 0 {
					org = segments[0]
					if len(segments) > 1 && !strings.HasPrefix(segments[1], "_") {
						project = segments[1]
					}
				}
				return &ParsedLink{
					Org:     org,
					Project: project,
					ID:      id,
					RawURL:  input,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("unable to parse Azure DevOps work item link or ID from: %s", input)
}

// BuildWebURL constructs the web browser URL for a work item
func BuildWebURL(org, project string, id int) string {
	if project != "" {
		return fmt.Sprintf("https://dev.azure.com/%s/%s/_workitems/edit/%d", url.PathEscape(org), url.PathEscape(project), id)
	}
	return fmt.Sprintf("https://dev.azure.com/%s/_workitems/edit/%d", url.PathEscape(org), id)
}

// BuildPRWebURL constructs the web browser URL for a pull request
func BuildPRWebURL(org, project, repo string, prID int) string {
	if repo == "" {
		repo = project
	}
	if project != "" {
		return fmt.Sprintf("https://dev.azure.com/%s/%s/_git/%s/pullrequest/%d",
			url.PathEscape(org), url.PathEscape(project), url.PathEscape(repo), prID)
	}
	return fmt.Sprintf("https://dev.azure.com/%s/_git/%s/pullrequest/%d",
		url.PathEscape(org), url.PathEscape(repo), prID)
}
