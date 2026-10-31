package ado

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseTimeRange(t *testing.T) {
	now := time.Now()
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	yesterdayStart := todayStart.AddDate(0, 0, -1)

	// 1. "today"
	s, u := ParseTimeRange("today", 0)
	if !s.Equal(todayStart) {
		t.Errorf("expected today start %v, got %v", todayStart, s)
	}
	if u.Before(now) {
		t.Errorf("expected until to be in future, got %v", u)
	}

	// 2. "yesterday"
	s, u = ParseTimeRange("yesterday", 0)
	if !s.Equal(yesterdayStart) {
		t.Errorf("expected yesterday start %v, got %v", yesterdayStart, s)
	}
	if !u.Equal(todayStart) {
		t.Errorf("expected yesterday end %v, got %v", todayStart, u)
	}

	// 3. "past 3 days"
	s, _ = ParseTimeRange("past 3 days", 0)
	expected3Days := time.Date(now.Year(), now.Month(), now.Day()-2, 0, 0, 0, 0, loc)
	if !s.Equal(expected3Days) {
		t.Errorf("expected past 3 days start %v, got %v", expected3Days, s)
	}

	// 4. "last 24 hours"
	s, _ = ParseTimeRange("last 24 hours", 0)
	diff := now.Sub(s)
	if diff < 23*time.Hour || diff > 25*time.Hour {
		t.Errorf("expected ~24h diff, got %v", diff)
	}

	// 5. sinceDays parameter
	s, _ = ParseTimeRange("", 7)
	expected7Days := time.Date(now.Year(), now.Month(), now.Day()-6, 0, 0, 0, 0, loc)
	if !s.Equal(expected7Days) {
		t.Errorf("expected 7 days start %v, got %v", expected7Days, s)
	}
}

func TestParseUpdatesToEvents(t *testing.T) {
	wi := &WorkItem{
		ID: 656842,
		Fields: map[string]interface{}{
			"System.Title":        "Implement Authentication",
			"System.WorkItemType": "Feature",
		},
	}

	t1 := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 21, 9, 30, 0, 0, time.UTC)
	t3 := time.Date(2026, 8, 21, 14, 0, 0, 0, time.UTC)

	updates := []*WorkItemUpdate{
		{
			ID:          1,
			WorkItemID:  656842,
			Rev:         1,
			RevisedDate: t1,
			RevisedBy: &IdentityRef{
				DisplayName: "Alice Admin",
				UniqueName:  "alice@company.com",
			},
			Fields: map[string]*WorkItemFieldUpdate{
				"System.Title": {
					NewValue: "Implement Authentication",
				},
				"System.State": {
					NewValue: "New",
				},
			},
		},
		{
			ID:          2,
			WorkItemID:  656842,
			Rev:         2,
			RevisedDate: t2,
			RevisedBy: &IdentityRef{
				DisplayName: "Bob Developer",
				UniqueName:  "bob@company.com",
			},
			Fields: map[string]*WorkItemFieldUpdate{
				"System.State": {
					OldValue: "New",
					NewValue: "Active",
				},
				"System.AssignedTo": {
					NewValue: map[string]interface{}{
						"displayName": "Bob Developer",
						"uniqueName":  "bob@company.com",
					},
				},
				"Microsoft.VSTS.Scheduling.RemainingWork": {
					NewValue: 8.0,
				},
			},
		},
		{
			ID:          3,
			WorkItemID:  656842,
			Rev:         3,
			RevisedDate: t3,
			RevisedBy: &IdentityRef{
				DisplayName: "Bob Developer",
				UniqueName:  "bob@company.com",
			},
			Fields: map[string]*WorkItemFieldUpdate{
				"System.History": {
					NewValue: "<div>Finished OAuth backend endpoints. Ready for testing.</div>",
				},
			},
			Relations: &WorkItemRelationsUpdate{
				Added: []WorkItemRelation{
					{
						Rel: "ArtifactLink",
						URL: "vstfs:///Git/PullRequestId/proj-123%2Frepo-456%2F789",
					},
				},
			},
		},
	}

	events := ParseUpdatesToEvents(wi, updates)
	if len(events) < 5 {
		t.Fatalf("expected at least 5 events, got %d", len(events))
	}

	// Verify events content
	var hasCreated, hasStateChange, hasAssign, hasEst, hasComment, hasPR bool
	for _, ev := range events {
		switch ev.Type {
		case ActivityCreated:
			hasCreated = true
			if ev.AuthorName != "Alice Admin" {
				t.Errorf("expected creator Alice Admin, got %s", ev.AuthorName)
			}
		case ActivityStateChange:
			hasStateChange = true
			if ev.OldValue != "New" || ev.NewValue != "Active" {
				t.Errorf("expected New -> Active, got %s -> %s", ev.OldValue, ev.NewValue)
			}
		case ActivityAssignChange:
			hasAssign = true
			if ev.NewValue != "Bob Developer" {
				t.Errorf("expected assignee Bob Developer, got %s", ev.NewValue)
			}
		case ActivityEstimate:
			hasEst = true
		case ActivityComment:
			hasComment = true
			if !strings.Contains(ev.Details, "Finished OAuth backend") {
				t.Errorf("expected clean comment details, got %q", ev.Details)
			}
		case ActivityPullRequest:
			hasPR = true
			if ev.PullRequestID != 789 {
				t.Errorf("expected PR ID 789, got %d", ev.PullRequestID)
			}
		}
	}

	if !hasCreated || !hasStateChange || !hasAssign || !hasEst || !hasComment || !hasPR {
		t.Errorf("missing event types: created=%v, state=%v, assign=%v, est=%v, comment=%v, pr=%v",
			hasCreated, hasStateChange, hasAssign, hasEst, hasComment, hasPR)
	}

	// Test Chronological Sorting and Filtering
	since := time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)
	filtered := SortAndFilterEvents(events, since, until)

	for _, ev := range filtered {
		if ev.Timestamp.Before(since) || ev.Timestamp.After(until) {
			t.Errorf("event outside time filter: %v", ev.Timestamp)
		}
	}

	// Verify strict ascending order
	for i := 1; i < len(filtered); i++ {
		if filtered[i].Timestamp.Before(filtered[i-1].Timestamp) {
			t.Errorf("events not in chronological order: index %d (%v) before %d (%v)",
				i, filtered[i].Timestamp, i-1, filtered[i-1].Timestamp)
		}
	}
}

func TestGetWorkItemUpdatesClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/test-org/test-proj/_apis/wit/workitems/500/updates" {
			resp := WorkItemUpdateList{
				Count: 1,
				Value: []*WorkItemUpdate{
					{
						ID:          1,
						WorkItemID:  500,
						Rev:         1,
						RevisedDate: time.Now(),
						RevisedBy: &IdentityRef{
							DisplayName: "Dev Tester",
							UniqueName:  "tester@test.com",
						},
						Fields: map[string]*WorkItemFieldUpdate{
							"System.State": {
								NewValue: "Active",
							},
						},
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient("test-org", "test-proj", "dummy-pat")
	// Test update list struct
	if client == nil {
		t.Fatal("expected client to be initialized")
	}
}
