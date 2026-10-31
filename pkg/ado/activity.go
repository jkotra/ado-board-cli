package ado

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ActivityType classifies the type of event in the activity timeline
type ActivityType string

const (
	ActivityComment      ActivityType = "comment"
	ActivityStateChange  ActivityType = "state_change"
	ActivityPullRequest  ActivityType = "pull_request"
	ActivityAssignChange ActivityType = "assignment_change"
	ActivityEstimate     ActivityType = "estimate_change"
	ActivityFieldChange  ActivityType = "field_change"
	ActivityCreated      ActivityType = "created"
)

// ActivityEvent represents a normalized, chronological event in work item history
type ActivityEvent struct {
	Timestamp      time.Time    `json:"timestamp"`
	DateString     string       `json:"date_string"`
	TimeOnlyString string       `json:"time_only_string"`
	TimeString     string       `json:"time_string"`
	RelativeTime   string       `json:"relative_time"`
	WorkItemID     int          `json:"work_item_id"`
	WorkItemTitle  string       `json:"work_item_title,omitempty"`
	WorkItemType   string       `json:"work_item_type,omitempty"`
	Type           ActivityType `json:"type"`
	AuthorName     string       `json:"author_name"`
	AuthorEmail    string       `json:"author_email,omitempty"`
	Summary        string       `json:"summary"`
	Details        string       `json:"details,omitempty"`
	OldValue       string       `json:"old_value,omitempty"`
	NewValue       string       `json:"new_value,omitempty"`
	PullRequestID  int          `json:"pull_request_id,omitempty"`
	PullRequestURL string       `json:"pull_request_url,omitempty"`
}

// ActivitySummary provides a top-level report structure for AI summaries
type ActivitySummary struct {
	WorkItemID        int              `json:"work_item_id"`
	WorkItemTitle     string           `json:"work_item_title"`
	WorkItemType      string           `json:"work_item_type"`
	TimeRange         string           `json:"time_range"`
	Since             time.Time        `json:"since"`
	Until             time.Time        `json:"until"`
	TotalEvents       int              `json:"total_events"`
	CommentsCount     int              `json:"comments_count"`
	StateChangesCount int              `json:"state_changes_count"`
	PRsCount          int              `json:"prs_count"`
	Events            []*ActivityEvent `json:"events"`
}

// ParseTimeRange parses human time range strings like "today", "yesterday", "past 3 days", "last 24 hours"
func ParseTimeRange(timeRangeStr string, sinceDays int) (time.Time, time.Time) {
	now := time.Now()
	loc := now.Location()

	if sinceDays > 0 {
		startDate := time.Date(now.Year(), now.Month(), now.Day()-sinceDays+1, 0, 0, 0, 0, loc)
		return startDate, now.Add(24 * time.Hour)
	}

	norm := strings.ToLower(strings.TrimSpace(timeRangeStr))
	if norm == "" || norm == "all" || norm == "everything" {
		return time.Time{}, now.Add(24 * time.Hour)
	}

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	if strings.Contains(norm, "today") {
		return todayStart, now.Add(24 * time.Hour)
	}

	if strings.Contains(norm, "yesterday") {
		yesterdayStart := todayStart.AddDate(0, 0, -1)
		return yesterdayStart, todayStart
	}

	// Regex for "past N days", "last N days", "N days"
	dayRe := regexp.MustCompile(`(?:past|last)?\s*(\d+)\s*(?:day|days)`)
	if match := dayRe.FindStringSubmatch(norm); len(match) == 2 {
		if d, err := strconv.Atoi(match[1]); err == nil && d > 0 {
			startDate := time.Date(now.Year(), now.Month(), now.Day()-d+1, 0, 0, 0, 0, loc)
			return startDate, now.Add(24 * time.Hour)
		}
	}

	// Regex for "past N hours", "last N hours", "N hours"
	hourRe := regexp.MustCompile(`(?:past|last)?\s*(\d+)\s*(?:hour|hours|hr|hrs)`)
	if match := hourRe.FindStringSubmatch(norm); len(match) == 2 {
		if h, err := strconv.Atoi(match[1]); err == nil && h > 0 {
			return now.Add(-time.Duration(h) * time.Hour), now.Add(24 * time.Hour)
		}
	}

	if strings.Contains(norm, "week") {
		return todayStart.AddDate(0, 0, -7), now.Add(24 * time.Hour)
	}

	if strings.Contains(norm, "month") {
		return todayStart.AddDate(0, 0, -30), now.Add(24 * time.Hour)
	}

	// Default fallback: past 3 days
	return todayStart.AddDate(0, 0, -3), now.Add(24 * time.Hour)
}

// FormatDateHeader formats a user-friendly date heading (e.g. "Today (Aug 21)", "Yesterday (Aug 20)", or "Jan 19, 2026")
func FormatDateHeader(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	tLoc := t.Local()
	now := time.Now()
	loc := now.Location()

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)

	if !tLoc.Before(todayStart) {
		return fmt.Sprintf("Today (%s)", tLoc.Format("Jan 02"))
	}
	if !tLoc.Before(yesterdayStart) && tLoc.Before(todayStart) {
		return fmt.Sprintf("Yesterday (%s)", tLoc.Format("Jan 02"))
	}
	return tLoc.Format("Jan 02, 2006")
}

// FormatTimeOnly returns the time portion in local system timezone (e.g. "15:04 IST")
func FormatTimeOnly(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	tLoc := t.Local()
	return tLoc.Format("15:04 MST")
}

// FormatRelativeTime creates a user-friendly timestamp representation in local system time
func FormatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	tLoc := t.Local()
	now := time.Now()
	loc := now.Location()

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)
	tz := tLoc.Format("MST")

	if !tLoc.Before(todayStart) {
		return fmt.Sprintf("Today at %s %s", tLoc.Format("15:04"), tz)
	}
	if !tLoc.Before(yesterdayStart) && tLoc.Before(todayStart) {
		return fmt.Sprintf("Yesterday at %s %s", tLoc.Format("15:04"), tz)
	}
	if tLoc.Year() == now.Year() {
		return fmt.Sprintf("%s %s", tLoc.Format("Jan 02 at 15:04"), tz)
	}
	return fmt.Sprintf("%s %s", tLoc.Format("2006-01-02 15:04"), tz)
}

// CleanHTML removes HTML tags, breaks, and unescapes entities for clean markdown presentation
func CleanHTML(input string) string {
	if input == "" {
		return ""
	}

	s := input
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")
	s = strings.ReplaceAll(s, "</div>", "\n")
	s = strings.ReplaceAll(s, "</p>", "\n\n")

	tagRe := regexp.MustCompile(`<[^>]*>`)
	s = tagRe.ReplaceAllString(s, "")

	s = html.UnescapeString(s)

	// Clean up consecutive empty lines
	lines := strings.Split(s, "\n")
	var cleaned []string
	lastEmpty := false
	for _, l := range lines {
		trim := strings.TrimSpace(l)
		if trim == "" {
			if !lastEmpty {
				cleaned = append(cleaned, "")
				lastEmpty = true
			}
		} else {
			cleaned = append(cleaned, trim)
			lastEmpty = false
		}
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// ExtractIdentityName parses identity strings or structs into a clean display name
func ExtractIdentityName(v interface{}) (name string, email string) {
	if v == nil {
		return "", ""
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val), ""
	case *IdentityRef:
		if val != nil {
			return val.DisplayName, val.UniqueName
		}
	case IdentityRef:
		return val.DisplayName, val.UniqueName
	case map[string]interface{}:
		if d, ok := val["displayName"].(string); ok {
			name = d
		}
		if u, ok := val["uniqueName"].(string); ok {
			email = u
		}
		if name == "" {
			name = email
		}
	}
	return name, email
}

// ParseUpdatesToEvents extracts normalized chronological ActivityEvents from WorkItemUpdates
func ParseUpdatesToEvents(wi *WorkItem, updates []*WorkItemUpdate) []*ActivityEvent {
	var events []*ActivityEvent
	if len(updates) == 0 {
		return events
	}

	itemID := 0
	itemTitle := ""
	itemType := ""
	if wi != nil {
		itemID = wi.ID
		itemTitle = wi.Title()
		itemType = wi.Type()
	}

	for _, u := range updates {
		if u == nil {
			continue
		}
		if u.RevisedDate.IsZero() {
			continue
		}
		t := u.RevisedDate.Local()

		authorName := "System"
		authorEmail := ""
		if u.RevisedBy != nil {
			if u.RevisedBy.DisplayName != "" {
				authorName = u.RevisedBy.DisplayName
			}
			authorEmail = u.RevisedBy.UniqueName
		}

		timeStr := t.Format("2006-01-02 15:04:05 MST")
		timeOnly := FormatTimeOnly(t)
		dateStr := FormatDateHeader(t)
		relTime := FormatRelativeTime(t)

		// 1. Check Comments (System.History)
		if fHist, ok := u.Fields["System.History"]; ok && fHist != nil && fHist.NewValue != nil {
			commentBody := CleanHTML(fmt.Sprintf("%v", fHist.NewValue))
			if commentBody != "" {
				events = append(events, &ActivityEvent{
					Timestamp:      t,
					DateString:     dateStr,
					TimeOnlyString: timeOnly,
					TimeString:     timeStr,
					RelativeTime:   relTime,
					WorkItemID:     itemID,
					WorkItemTitle:  itemTitle,
					WorkItemType:   itemType,
					Type:           ActivityComment,
					AuthorName:     authorName,
					AuthorEmail:    authorEmail,
					Summary:        fmt.Sprintf("%s commented", authorName),
					Details:        commentBody,
					NewValue:       commentBody,
				})
			}
		}

		// 2. Check State Changes (System.State)
		if fState, ok := u.Fields["System.State"]; ok && fState != nil && fState.NewValue != nil {
			oldSt := ""
			if fState.OldValue != nil {
				oldSt = fmt.Sprintf("%v", fState.OldValue)
			}
			newSt := fmt.Sprintf("%v", fState.NewValue)

			if u.Rev > 1 || oldSt != "" {
				summary := ""
				if oldSt != "" {
					summary = fmt.Sprintf("%s changed state: %s → %s", authorName, oldSt, newSt)
				} else {
					summary = fmt.Sprintf("%s set state to %s", authorName, newSt)
				}

				events = append(events, &ActivityEvent{
					Timestamp:      t,
					DateString:     dateStr,
					TimeOnlyString: timeOnly,
					TimeString:     timeStr,
					RelativeTime:   relTime,
					WorkItemID:     itemID,
					WorkItemTitle:  itemTitle,
					WorkItemType:   itemType,
					Type:           ActivityStateChange,
					AuthorName:     authorName,
					AuthorEmail:    authorEmail,
					Summary:        summary,
					OldValue:       oldSt,
					NewValue:       newSt,
				})
			}
		}

		// 3. Check Assignee Changes (System.AssignedTo)
		if fAssign, ok := u.Fields["System.AssignedTo"]; ok && fAssign != nil {
			oldName, _ := ExtractIdentityName(fAssign.OldValue)
			newName, newEmail := ExtractIdentityName(fAssign.NewValue)

			summary := ""
			if newName != "" && oldName != "" {
				summary = fmt.Sprintf("%s reassigned from %s to %s", authorName, oldName, newName)
			} else if newName != "" {
				summary = fmt.Sprintf("%s assigned to %s", authorName, newName)
			} else if oldName != "" {
				summary = fmt.Sprintf("%s unassigned %s", authorName, oldName)
			}

			if summary != "" {
				events = append(events, &ActivityEvent{
					Timestamp:      t,
					DateString:     dateStr,
					TimeOnlyString: timeOnly,
					TimeString:     timeStr,
					RelativeTime:   relTime,
					WorkItemID:     itemID,
					WorkItemTitle:  itemTitle,
					WorkItemType:   itemType,
					Type:           ActivityAssignChange,
					AuthorName:     authorName,
					AuthorEmail:    authorEmail,
					Summary:        summary,
					OldValue:       oldName,
					NewValue:       newName,
					Details:        newEmail,
				})
			}
		}

		// 4. Check Work & Estimate Changes
		estFields := []string{
			"Microsoft.VSTS.Scheduling.OriginalEstimate",
			"Microsoft.VSTS.Scheduling.RemainingWork",
			"Microsoft.VSTS.Scheduling.CompletedWork",
			"Microsoft.VSTS.Scheduling.Effort",
			"Custom.OriginalEstimate",
			"Custom.RemainingWork",
		}
		hasEst := false
		var estParts []string
		for _, ef := range estFields {
			if fEst, ok := u.Fields[ef]; ok && fEst != nil && fEst.NewValue != nil {
				hasEst = true
				shortName := ef
				if idx := strings.LastIndex(ef, "."); idx != -1 {
					shortName = ef[idx+1:]
				}
				estParts = append(estParts, fmt.Sprintf("%s: %v", shortName, fEst.NewValue))
			}
		}
		if hasEst && len(estParts) > 0 {
			events = append(events, &ActivityEvent{
				Timestamp:      t,
				DateString:     dateStr,
				TimeOnlyString: timeOnly,
				TimeString:     timeStr,
				RelativeTime:   relTime,
				WorkItemID:     itemID,
				WorkItemTitle:  itemTitle,
				WorkItemType:   itemType,
				Type:           ActivityEstimate,
				AuthorName:     authorName,
				AuthorEmail:    authorEmail,
				Summary:        fmt.Sprintf("%s updated estimates (%s)", authorName, strings.Join(estParts, ", ")),
				Details:        strings.Join(estParts, ", "),
			})
		}

		// 5. Check Pull Requests and Relations Added
		if u.Relations != nil && len(u.Relations.Added) > 0 {
			for _, rel := range u.Relations.Added {
				if pr, ok := rel.ParsePullRequest(); ok {
					events = append(events, &ActivityEvent{
						Timestamp:      t,
						DateString:     dateStr,
						TimeOnlyString: timeOnly,
						TimeString:     timeStr,
						RelativeTime:   relTime,
						WorkItemID:     itemID,
						WorkItemTitle:  itemTitle,
						WorkItemType:   itemType,
						Type:           ActivityPullRequest,
						AuthorName:     authorName,
						AuthorEmail:    authorEmail,
						Summary:        fmt.Sprintf("%s linked Pull Request #%d", authorName, pr.PullRequestID),
						PullRequestID:  pr.PullRequestID,
						PullRequestURL: pr.URL,
					})
				}
			}
		}

		// 6. Check Creation on Rev 1
		if u.Rev == 1 {
			events = append(events, &ActivityEvent{
				Timestamp:      t,
				DateString:     dateStr,
				TimeOnlyString: timeOnly,
				TimeString:     timeStr,
				RelativeTime:   relTime,
				WorkItemID:     itemID,
				WorkItemTitle:  itemTitle,
				WorkItemType:   itemType,
				Type:           ActivityCreated,
				AuthorName:     authorName,
				AuthorEmail:    authorEmail,
				Summary:        fmt.Sprintf("%s created %s #%d: %s", authorName, itemType, itemID, itemTitle),
			})
		}
	}

	return events
}

// SortAndFilterEvents filters events by time window and sorts them in ascending chronological order
func SortAndFilterEvents(events []*ActivityEvent, since, until time.Time) []*ActivityEvent {
	var filtered []*ActivityEvent

	for _, e := range events {
		if e == nil || e.Timestamp.IsZero() {
			continue
		}
		if !since.IsZero() && e.Timestamp.Before(since) {
			continue
		}
		if !until.IsZero() && e.Timestamp.After(until) {
			continue
		}
		filtered = append(filtered, e)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	return filtered
}
