package ado

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientWithRestyV3(t *testing.T) {
	// Create a mock ADO HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Verify Basic Auth header is present
		auth := r.Header.Get("Authorization")
		if auth == "" {
			t.Errorf("expected Authorization header, got empty")
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/test-org/test-proj/_apis/wit/workitems/100":
			wi := WorkItem{
				ID:  100,
				Rev: 1,
				Fields: map[string]interface{}{
					"System.Title":        "Test Item 100",
					"System.WorkItemType": "Task",
					"System.State":        "In Progress",
				},
			}
			json.NewEncoder(w).Encode(wi)

		case r.Method == http.MethodPost && r.URL.Path == "/test-org/test-proj/_apis/wit/workitems/$Task":
			wi := WorkItem{
				ID:  200,
				Rev: 1,
				Fields: map[string]interface{}{
					"System.Title":        "Created Task",
					"System.WorkItemType": "Task",
					"System.State":        "To Do",
				},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(wi)

		case r.Method == http.MethodPatch && r.URL.Path == "/test-org/test-proj/_apis/wit/workitems/100":
			wi := WorkItem{
				ID:  100,
				Rev: 2,
				Fields: map[string]interface{}{
					"System.Title": "Updated Task",
					"System.State": "Done",
				},
			}
			json.NewEncoder(w).Encode(wi)

		case r.Method == http.MethodDelete && r.URL.Path == "/test-org/test-proj/_apis/wit/workitems/100":
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodGet && r.URL.Path == "/test-org/test-proj/_apis/wit/workitems/100/comments":
			comments := CommentList{
				TotalCount: 1,
				Comments: []*CommentItem{
					{
						ID:   1,
						Text: "Test comment on #100",
					},
				},
			}
			json.NewEncoder(w).Encode(comments)

		case r.Method == http.MethodPost && r.URL.Path == "/test-org/test-proj/_apis/wit/workitems/100/comments":
			comment := CommentItem{
				ID:   2,
				Text: "New comment added",
			}
			json.NewEncoder(w).Encode(comment)

		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"message": "not found"}`))
		}
	}))
	defer server.Close()

	// Instantiate client
	client := NewClient("test-org", "test-proj", "dummy-pat-token")

	// Verify Client instantiation and Resty settings
	if client.R == nil {
		t.Fatal("expected resty client to be initialized")
	}
	if client.Token != "dummy-pat-token" {
		t.Errorf("expected token dummy-pat-token, got %s", client.Token)
	}
}

func TestExtractMissingField(t *testing.T) {
	tests := []struct {
		err      string
		expected string
	}{
		{"TF51535: Cannot find field Custom.RemainingWork.", "Custom.RemainingWork"},
		{"Field 'Microsoft.VSTS.Scheduling.OriginalEstimate' does not exist.", "Microsoft.VSTS.Scheduling.OriginalEstimate"},
		{"Cannot find field Custom.Effortss.", "Custom.Effortss"},
		{"Field Custom.Effort not found", "Custom.Effort"},
		{"Generic network error", ""},
	}

	for _, tt := range tests {
		got := extractMissingField(tt.err)
		if got != tt.expected {
			t.Errorf("extractMissingField(%q) = %q, expected %q", tt.err, got, tt.expected)
		}
	}
}

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestUpdateWorkItemFieldsFallback(t *testing.T) {
	attempt := 0
	client := NewClient("test-org", "test-proj", "token")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		attempt++
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodGet {
			wi := WorkItem{
				ID: 100,
				Fields: map[string]interface{}{
					"System.Title":                            "Updated Task",
					"Microsoft.VSTS.Scheduling.RemainingWork": 16.0,
				},
			}
			data, _ := json.Marshal(wi)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}

		var patches []JsonPatchOperation
		if req.Body != nil {
			_ = json.NewDecoder(req.Body).Decode(&patches)
		}

		hasCustomRemaining := false
		hasStandardRemaining := false
		for _, p := range patches {
			if p.Path == "/fields/Custom.RemainingWork" {
				hasCustomRemaining = true
			}
			if p.Path == "/fields/Microsoft.VSTS.Scheduling.RemainingWork" {
				hasStandardRemaining = true
			}
		}

		if hasCustomRemaining {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(`{"message": "TF51535: Cannot find field Custom.RemainingWork."}`)),
			}
		}

		if hasStandardRemaining {
			wi := WorkItem{
				ID: 100,
				Fields: map[string]interface{}{
					"System.Title":                            "Updated Task",
					"Microsoft.VSTS.Scheduling.RemainingWork": 16.0,
				},
			}
			data, _ := json.Marshal(wi)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}

		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     hdr,
			Body:       io.NopCloser(strings.NewReader(`{"message": "unexpected patch payload"}`)),
		}
	}))

	// Attempt update with Custom.RemainingWork which should automatically fall back to Microsoft.VSTS.Scheduling.RemainingWork
	updated, err := client.UpdateWorkItemFields(100, map[string]interface{}{
		"Custom.RemainingWork": 16.0,
	})
	if err != nil {
		t.Fatalf("expected fallback to succeed, got err: %v", err)
	}
	if updated == nil || updated.RemainingWork() != 16.0 {
		t.Fatalf("expected remaining work 16.0, got %v", updated)
	}
	if attempt < 2 {
		t.Fatalf("expected at least 2 attempts (initial failure + retry), got %d", attempt)
	}
}

func TestUpdateWorkItemFieldsDateRemovalAndFallback(t *testing.T) {
	attempt := 0
	client := NewClient("myorg", "myproj", "token")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		attempt++
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodGet {
			wi := WorkItem{
				ID: 200,
				Fields: map[string]interface{}{
					"System.Title": "Task with Date Cleared",
				},
			}
			data, _ := json.Marshal(wi)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}

		var patches []JsonPatchOperation
		if req.Body != nil {
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &patches)
		}

		// Check if remove operation is generated for empty date field
		for _, p := range patches {
			if p.Path == "/fields/Microsoft.VSTS.Scheduling.StartDate" && p.Op == "remove" {
				wi := WorkItem{
					ID: 200,
					Fields: map[string]interface{}{
						"System.Title": "Task with Date Cleared",
					},
				}
				data, _ := json.Marshal(wi)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     hdr,
					Body:       io.NopCloser(bytes.NewReader(data)),
				}
			}
		}

		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     hdr,
			Body:       io.NopCloser(strings.NewReader(`{"message": "unexpected patch"}`)),
		}
	}))

	updated, err := client.UpdateWorkItemFields(200, map[string]interface{}{
		"Microsoft.VSTS.Scheduling.StartDate": "",
	})
	if err != nil {
		t.Fatalf("expected date removal to succeed, got err: %v", err)
	}
	if updated == nil || updated.ID != 200 {
		t.Fatalf("expected updated item 200, got %v", updated)
	}
}

func TestCreateWorkItemInheritsNearestParentIteration(t *testing.T) {
	client := NewClient("myorg", "myproj", "token")
	var receivedPatches []JsonPatchOperation

	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		// Mock GET parent work item #100
		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "workitems/100") {
			wi := WorkItem{
				ID: 100,
				Fields: map[string]interface{}{
					"System.Title":         "Parent Story",
					"System.AreaPath":      "myproj\\Core\\Backend",
					"System.IterationPath": "myproj\\Sprint 2026.Q3",
				},
			}
			data, _ := json.Marshal(wi)
			hdr := make(http.Header)
			hdr.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}

		// Mock POST create child item
		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "workitems/$Task") {
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &receivedPatches)

			wi := WorkItem{
				ID: 201,
				Fields: map[string]interface{}{
					"System.Title":         "Child Task",
					"System.WorkItemType":  "Task",
					"System.AreaPath":      "myproj\\Core\\Backend",
					"System.IterationPath": "myproj\\Sprint 2026.Q3",
				},
			}
			data, _ := json.Marshal(wi)
			hdr := make(http.Header)
			hdr.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}

		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     hdr,
			Body:       io.NopCloser(strings.NewReader(`{"message": "not found"}`)),
		}
	}))

	created, err := client.CreateWorkItem("Task", map[string]interface{}{
		"System.Title": "Child Task",
	}, 100)

	if err != nil {
		t.Fatalf("expected CreateWorkItem to succeed, got %v", err)
	}
	if created == nil || created.ID != 201 {
		t.Fatalf("expected created work item 201, got %v", created)
	}

	// Verify that System.IterationPath and System.AreaPath were inherited from parent 100 in the patches
	hasIterationPatch := false
	hasAreaPatch := false
	for _, p := range receivedPatches {
		if p.Path == "/fields/System.IterationPath" && p.Value == "myproj\\Sprint 2026.Q3" {
			hasIterationPatch = true
		}
		if p.Path == "/fields/System.AreaPath" && p.Value == "myproj\\Core\\Backend" {
			hasAreaPatch = true
		}
	}
	if !hasIterationPatch {
		t.Fatalf("expected patch '/fields/System.IterationPath' with value 'myproj\\Sprint 2026.Q3', got %+v", receivedPatches)
	}
	if !hasAreaPatch {
		t.Fatalf("expected patch '/fields/System.AreaPath' with value 'myproj\\Core\\Backend', got %+v", receivedPatches)
	}
}

func TestValidatePAT(t *testing.T) {
	// 1. Empty token
	emptyClient := NewClient("myorg", "myproj", "")
	valid, err := emptyClient.ValidatePAT()
	if valid || err == nil {
		t.Errorf("expected false for empty token, got %v, %v", valid, err)
	}

	// 2. Valid token with connectionData
	validClient := NewClient("myorg", "myproj", "valid-pat")
	validClient.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Path, "_apis/connectionData") {
			hdr := make(http.Header)
			hdr.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(`{"authenticatedUser":{"displayName":"Test User"}}`)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))
	valid, err = validClient.ValidatePAT()
	if !valid || err != nil {
		t.Errorf("expected valid PAT, got valid=%v, err=%v", valid, err)
	}

	// 3. Invalid token (HTTP 401)
	invalidClient := NewClient("myorg", "myproj", "invalid-pat")
	invalidClient.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"message": "Unauthorized"}`)),
		}
	}))
	valid, err = invalidClient.ValidatePAT()
	if valid || err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 error, got valid=%v, err=%v", valid, err)
	}

	// 4. Redirect to login (HTTP 203)
	redirectClient := NewClient("myorg", "myproj", "redirect-pat")
	redirectClient.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusNonAuthoritativeInfo,
			Body:       io.NopCloser(strings.NewReader(`<html>Signin</html>`)),
		}
	}))
	valid, err = redirectClient.ValidatePAT()
	if valid || err == nil || !strings.Contains(err.Error(), "203") {
		t.Errorf("expected 203 error, got valid=%v, err=%v", valid, err)
	}
}

func TestFetchQueryHierarchy(t *testing.T) {
	client := NewClient("myorg", "myproj", "valid-pat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if strings.Contains(req.URL.Path, "_apis/wit/queries/7509d605-466a-4d8c-847f-c14e1b46a647") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": "7509d605-466a-4d8c-847f-c14e1b46a647",
					"name": "Sprint 2026 Backlog",
					"path": "Shared Queries/Sprint 2026 Backlog",
					"queryType": "tree",
					"queryResultType": "workItemHierarchy"
				}`)),
			}
		}

		if strings.Contains(req.URL.Path, "_apis/wit/wiql/7509d605-466a-4d8c-847f-c14e1b46a647") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"queryType": "tree",
					"queryResultType": "workItemHierarchy",
					"workItemRelations": [
						{"rel": null, "source": null, "target": {"id": 100}},
						{"rel": "System.LinkTypes.Hierarchy-Forward", "source": {"id": 100}, "target": {"id": 101}}
					]
				}`)),
			}
		}

		if strings.Contains(req.URL.Path, "_apis/wit/workitems") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"count": 2,
					"value": [
						{
							"id": 100,
							"fields": {
								"System.WorkItemType": "Feature",
								"System.Title": "GKE Root Feature",
								"System.State": "Active"
							}
						},
						{
							"id": 101,
							"fields": {
								"System.WorkItemType": "Task",
								"System.Title": "Deploy Pods",
								"System.State": "To Do"
							},
							"relations": [
								{"rel": "System.LinkTypes.Hierarchy-Reverse", "url": "https://dev.azure.com/myorg/myproj/_apis/wit/workItems/100"}
							]
						}
					]
				}`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	qInfo, rootIDs, items, err := client.FetchQueryHierarchy("7509d605-466a-4d8c-847f-c14e1b46a647")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if qInfo.Name != "Sprint 2026 Backlog" {
		t.Errorf("expected query name 'Sprint 2026 Backlog', got %q", qInfo.Name)
	}

	if len(rootIDs) != 1 || rootIDs[0] != 100 {
		t.Errorf("expected rootIDs [100], got %v", rootIDs)
	}

	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestExtractFailingFields(t *testing.T) {
	ruleErr := `{"$id":"1","customProperties":{"FieldReferenceName":null},"RuleValidationErrors":[{"fieldReferenceName":"Microsoft.VSTS.Scheduling.RemainingWork","fieldStatusFlags":"invalidNotEmpty","errorMessage":"TF401320: Rule Error for field Remaining Work. Error code: InvalidNotEmpty."},{"fieldReferenceName":"Microsoft.VSTS.Scheduling.Effort","fieldStatusFlags":"readOnly, invalidNotOldValue","errorMessage":"TF401320: Rule Error for field Effort. Error code: ReadOnly, InvalidNotOldValue."}],"message":"TF401320: Rule Error for field Remaining Work. Error code: InvalidNotEmpty."}`
	fields := extractFailingFields(ruleErr)
	hasRem := false
	hasEff := false
	for _, f := range fields {
		if f == "Microsoft.VSTS.Scheduling.RemainingWork" {
			hasRem = true
		}
		if f == "Microsoft.VSTS.Scheduling.Effort" {
			hasEff = true
		}
	}
	if !hasRem || !hasEff {
		t.Errorf("extractFailingFields failed to find RemainingWork or Effort, got: %v", fields)
	}
}

func TestUpdateWorkItemFieldsRuleErrors(t *testing.T) {
	attempt := 0
	client := NewClient("test-org", "test-proj", "token")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		attempt++
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodGet {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
				"id": 100,
				"fields": {
					"System.Title": "Test Bug",
					"Microsoft.VSTS.Scheduling.OriginalEstimate": 4.0
				}
			}`)),
			}
		}

		var patches []JsonPatchOperation
		if req.Body != nil {
			_ = json.NewDecoder(req.Body).Decode(&patches)
		}

		hasRemaining := false
		hasEffort := false
		hasOriginal := false
		for _, p := range patches {
			if strings.Contains(p.Path, "RemainingWork") {
				hasRemaining = true
			}
			if strings.Contains(p.Path, "Effort") {
				hasEffort = true
			}
			if strings.Contains(p.Path, "OriginalEstimate") {
				hasOriginal = true
			}
		}

		if attempt == 1 {
			if !hasRemaining || !hasEffort || !hasOriginal {
				t.Fatalf("attempt 1 should contain all requested fields")
			}
			ruleErr := `{"RuleValidationErrors":[{"fieldReferenceName":"Microsoft.VSTS.Scheduling.RemainingWork"},{"fieldReferenceName":"Microsoft.VSTS.Scheduling.Effort"}],"message":"TF401320: Rule Error for field Remaining Work."}`
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(ruleErr)),
			}
		}

		// Attempt 2: Remaining and Effort should be stripped, only OriginalEstimate remaining
		if hasRemaining || hasEffort {
			t.Fatalf("attempt 2 should have stripped failing fields, patches: %+v", patches)
		}
		if !hasOriginal {
			t.Fatalf("attempt 2 should still contain OriginalEstimate")
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     hdr,
			Body: io.NopCloser(strings.NewReader(`{
				"id": 100,
				"fields": {
					"System.Title": "Test Bug",
					"Microsoft.VSTS.Scheduling.OriginalEstimate": 4.0
				}
			}`)),
		}
	}))

	updated, err := client.UpdateWorkItemFields(100, map[string]interface{}{
		"Microsoft.VSTS.Scheduling.OriginalEstimate": 4.0,
		"Microsoft.VSTS.Scheduling.RemainingWork":    0.0,
		"Microsoft.VSTS.Scheduling.Effort":           0.0,
	})
	if err != nil {
		t.Fatalf("expected successful recovery from rule errors, got: %v", err)
	}
	if updated.ID != 100 {
		t.Errorf("expected ID 100, got %d", updated.ID)
	}
}

func TestUpdateWorkItemFieldsSegregatesEffortAndOriginalEstimate(t *testing.T) {
	// Verify that fieldFallbacks for OriginalEstimate does NOT contain Effort or StoryPoints
	fallbacks, ok := fieldFallbacks["Microsoft.VSTS.Scheduling.OriginalEstimate"]
	if !ok {
		t.Fatalf("expected fieldFallbacks to contain Microsoft.VSTS.Scheduling.OriginalEstimate")
	}
	for _, fb := range fallbacks {
		if strings.Contains(strings.ToLower(fb), "effort") || strings.Contains(strings.ToLower(fb), "storypoint") {
			t.Errorf("expected OriginalEstimate fallback NOT to contain effort or story points, got: %s", fb)
		}
	}

	// Verify that fieldFallbacks for Effort does NOT contain OriginalEstimate
	effortFallbacks, ok := fieldFallbacks["Microsoft.VSTS.Scheduling.Effort"]
	if !ok {
		t.Fatalf("expected fieldFallbacks to contain Microsoft.VSTS.Scheduling.Effort")
	}
	for _, fb := range effortFallbacks {
		if strings.Contains(strings.ToLower(fb), "originalestimate") {
			t.Errorf("expected Effort fallback NOT to contain OriginalEstimate, got: %s", fb)
		}
	}
}

func TestAddCommentAndFieldsWithMarkdownAndMentions(t *testing.T) {
	var commentPayload map[string]string
	var createPatches []JsonPatchOperation
	var updatePatches []JsonPatchOperation

	client := NewClient("test-org", "test-proj", "dummy-pat-token")
	client.R.SetTransport(roundTripFunc(func(r *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/comments") {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &commentPayload)
			comment := CommentItem{
				ID:   501,
				Text: commentPayload["text"],
				CreatedBy: &IdentityRef{
					DisplayName: "Doe, John",
				},
			}
			data, _ := json.Marshal(comment)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}

		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/workitems/$Task") {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &createPatches)
			wi := WorkItem{
				ID: 601,
				Fields: map[string]interface{}{
					"System.Title": "New Task",
				},
			}
			data, _ := json.Marshal(wi)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}

		if (r.Method == http.MethodPatch || r.Method == http.MethodGet) && strings.Contains(r.URL.Path, "/workitems/601") {
			if r.Method == http.MethodPatch && r.Body != nil {
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &updatePatches)
			}
			wi := WorkItem{
				ID: 601,
				Fields: map[string]interface{}{
					"System.Title": "Updated Task",
				},
			}
			data, _ := json.Marshal(wi)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}

		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     hdr,
			Body:       io.NopCloser(strings.NewReader(`{"message": "not found"}`)),
		}
	}))

	client.RegisterIdentities(
		&IdentityRef{
			ID:          "141b714f-561b-4171-8933-cb7ecf2ea3fe",
			DisplayName: "Johnson, Bob Henry",
			UniqueName:  "bob.johnson@example.com",
		},
		&IdentityRef{
			ID:          "22222222-3333-4444-5555-666666666666",
			DisplayName: "Brown, Carol",
			UniqueName:  "carol.brown@example.com",
		},
	)

	// 1. Test AddComment converts markdown and @mentions
	commentText := "changes pushed to `task-667425-orch-ng-changes-for-version`. cc: @Johnson, Bob Henry @Brown, Carol"
	comment, err := client.AddComment(601, commentText)
	if err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}
	if comment.ID != 501 {
		t.Errorf("expected comment ID 501, got %d", comment.ID)
	}

	sentCommentText := commentPayload["text"]
	if !strings.Contains(sentCommentText, "<code>task-667425-orch-ng-changes-for-version</code>") {
		t.Errorf("comment text missing code tag, got: %s", sentCommentText)
	}
	if !strings.Contains(sentCommentText, `<a href="#" data-vss-mention="version:2.0,141b714f-561b-4171-8933-cb7ecf2ea3fe">@Johnson, Bob Henry</a>`) {
		t.Errorf("comment text missing Johnson mention, got: %s", sentCommentText)
	}
	if !strings.Contains(sentCommentText, `<a href="#" data-vss-mention="version:2.0,22222222-3333-4444-5555-666666666666">@Brown, Carol</a>`) {
		t.Errorf("comment text missing Carol mention, got: %s", sentCommentText)
	}

	// 2. Test CreateWorkItem converts description markdown and @mentions
	_, err = client.CreateWorkItem("Task", map[string]interface{}{
		"System.Title":       "Test Markdown Task",
		"System.Description": "## Steps\n- Run `make test`\n- Review with @Johnson, Bob Henry",
	}, 0)
	if err != nil {
		t.Fatalf("CreateWorkItem failed: %v", err)
	}

	var descValue string
	for _, p := range createPatches {
		if p.Path == "/fields/System.Description" {
			descValue = fmt.Sprint(p.Value)
		}
	}
	if !strings.Contains(descValue, "<h2>Steps</h2>") || !strings.Contains(descValue, "<code>make test</code>") {
		t.Errorf("create work item description missing HTML tags, got: %s", descValue)
	}
	if !strings.Contains(descValue, `data-vss-mention="version:2.0,141b714f-561b-4171-8933-cb7ecf2ea3fe"`) {
		t.Errorf("create work item description missing mention tag, got: %s", descValue)
	}

	// 3. Test UpdateWorkItemFields converts description and acceptance criteria markdown
	_, err = client.UpdateWorkItemFields(601, map[string]interface{}{
		"System.Description": "Updated `code` cc: @Brown, Carol",
	})
	if err != nil {
		t.Fatalf("UpdateWorkItemFields failed: %v", err)
	}

	var updateDescValue string
	for _, p := range updatePatches {
		if p.Path == "/fields/System.Description" {
			updateDescValue = fmt.Sprint(p.Value)
		}
	}
	if !strings.Contains(updateDescValue, "<code>code</code>") {
		t.Errorf("update work item description missing code tag, got: %s", updateDescValue)
	}
	if !strings.Contains(updateDescValue, `data-vss-mention="version:2.0,22222222-3333-4444-5555-666666666666"`) {
		t.Errorf("update work item description missing mention tag, got: %s", updateDescValue)
	}
}

func TestExtractFailingFieldsReadOnlyAndLocked(t *testing.T) {
	err1 := "TF401320: Rule Error for field Remaining Work. Error details: The field 'Remaining Work' cannot be modified."
	fields1 := extractFailingFields(err1)
	hasRem := false
	for _, f := range fields1 {
		if f == "Microsoft.VSTS.Scheduling.RemainingWork" || f == "Remaining Work" {
			hasRem = true
			break
		}
	}
	if !hasRem {
		t.Errorf("expected extractFailingFields to detect RemainingWork from rule error, got: %v", fields1)
	}

	err2 := "The field 'Remaining Work' is read-only."
	fields2 := extractFailingFields(err2)
	hasRem2 := false
	for _, f := range fields2 {
		if f == "Microsoft.VSTS.Scheduling.RemainingWork" || f == "Remaining Work" {
			hasRem2 = true
			break
		}
	}
	if !hasRem2 {
		t.Errorf("expected extractFailingFields to detect RemainingWork from read-only error, got: %v", fields2)
	}
}

func TestUpdateWorkItemFieldsLockedRemainingWorkRetry(t *testing.T) {
	attempt := 0
	client := NewClient("myorg", "myproj", "token")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		attempt++
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodGet {
			wi := WorkItem{
				ID: 663674,
				Fields: map[string]interface{}{
					"System.Title":                                "Implementation",
					"System.State":                                "Done",
					"Microsoft.VSTS.Scheduling.OriginalEstimate": 12.0,
					"Microsoft.VSTS.Scheduling.CompletedWork":    12.0,
					"Microsoft.VSTS.Scheduling.RemainingWork":    0.0,
				},
			}
			data, _ := json.Marshal(wi)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(bytes.NewReader(data)),
			}
		}

		var patches []JsonPatchOperation
		if req.Body != nil {
			body, _ := io.ReadAll(req.Body)
			_ = json.Unmarshal(body, &patches)
		}

		// On attempt 1, if it contains RemainingWork, simulate ADO error
		hasRem := false
		for _, p := range patches {
			if strings.Contains(p.Path, "RemainingWork") {
				hasRem = true
				break
			}
		}

		if hasRem {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(`{"message": "The field 'Remaining Work' cannot be modified."}`)),
			}
		}

		// When RemainingWork is dropped, patch succeeds and returns updated item
		wi := WorkItem{
			ID: 663674,
			Fields: map[string]interface{}{
				"System.Title":                                "Implementation",
				"System.State":                                "Done",
				"Microsoft.VSTS.Scheduling.OriginalEstimate": 12.0,
				"Microsoft.VSTS.Scheduling.CompletedWork":    12.0,
				"Microsoft.VSTS.Scheduling.RemainingWork":    0.0,
			},
		}
		data, _ := json.Marshal(wi)
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     hdr,
			Body:       io.NopCloser(bytes.NewReader(data)),
		}
	}))

	updated, err := client.UpdateWorkItemFields(663674, map[string]interface{}{
		"Microsoft.VSTS.Scheduling.OriginalEstimate": 12.0,
		"Microsoft.VSTS.Scheduling.CompletedWork":    12.0,
		"Microsoft.VSTS.Scheduling.RemainingWork":    0.0,
	})

	if err != nil {
		t.Fatalf("expected update to recover and succeed after dropping locked field, got err: %v", err)
	}
	if updated == nil || updated.CompletedWork() != 12.0 {
		t.Fatalf("expected completed work 12.0, got %v", updated)
	}
}

func TestUploadPullRequestAttachment(t *testing.T) {
	client := NewClient("myorg", "myproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "attachments/screenshot.png") {
			bodyBytes, _ := io.ReadAll(req.Body)
			if string(bodyBytes) != "fake image bytes" {
				t.Errorf("expected 'fake image bytes', got %s", string(bodyBytes))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": 42,
					"url": "https://dev.azure.com/myorg/myproj/_apis/git/repositories/myrepo/pullRequests/101/attachments/screenshot.png"
				}`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	att, err := client.UploadPullRequestAttachment("myrepo", 101, "screenshot.png", []byte("fake image bytes"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if att.ID != 42 || !strings.Contains(att.URL, "screenshot.png") {
		t.Errorf("unexpected attachment result: %+v", att)
	}
}

func TestUpdatePullRequest(t *testing.T) {
	client := NewClient("myorg", "myproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "pullrequests/101") {
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["description"] != "Updated description text" {
				t.Errorf("unexpected description: %v", body["description"])
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 101,
					"title": "My PR Title",
					"description": "Updated description text"
				}`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	desc := "Updated description text"
	updated, err := client.UpdatePullRequest("myrepo", 101, UpdatePullRequestParams{
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Description != "Updated description text" {
		t.Errorf("unexpected PR description: %s", updated.Description)
	}
}

func TestLinkPullRequestToWorkItem(t *testing.T) {
	client := NewClient("myorg", "myproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		// 1. Get PR
		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "pullrequests/101") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 101,
					"title": "Feature PR",
					"repository": {
						"id": "repo-guid-123",
						"name": "myrepo",
						"project": {
							"id": "proj-guid-456",
							"name": "myproj"
						}
					}
				}`)),
			}
		}

		// 2. Patch work item
		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "workitems/555") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": 555,
					"fields": {
						"System.Title": "Target Work Item"
					}
				}`)),
			}
		}

		// 3. PR workitems post
		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "pullRequests/101/workitems") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(`[{"id": "555"}]`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	item, pr, err := client.LinkPullRequestToWorkItem("myrepo", 101, 555, "Linked PR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID != 555 {
		t.Errorf("expected work item ID 555, got %d", item.ID)
	}
	if pr.PullRequestID != 101 {
		t.Errorf("expected PR ID 101, got %d", pr.PullRequestID)
	}
}

func TestCreatePullRequest(t *testing.T) {
	client := NewClient("myorg", "myproj", "token")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "pullrequests") {
			var body CreatePullRequestParams
			_ = json.NewDecoder(req.Body).Decode(&body)

			if !strings.HasPrefix(body.SourceRefName, "refs/heads/") {
				t.Errorf("expected sourceRefName with refs/heads/, got %s", body.SourceRefName)
			}
			if body.TargetRefName != "refs/heads/main" {
				t.Errorf("expected targetRefName refs/heads/main, got %s", body.TargetRefName)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 303,
					"title": "New PR",
					"sourceRefName": "refs/heads/feature/auth",
					"targetRefName": "refs/heads/main",
					"status": "active"
				}`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	pr, err := client.CreatePullRequest("myrepo", CreatePullRequestParams{
		SourceRefName: "feature/auth",
		Title:         "New PR",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.PullRequestID != 303 {
		t.Errorf("expected PR ID 303, got %d", pr.PullRequestID)
	}
	if pr.SourceBranch() != "feature/auth" {
		t.Errorf("expected SourceBranch feature/auth, got %s", pr.SourceBranch())
	}
}

func TestAddPullRequestReviewer(t *testing.T) {
	client := NewClient("myorg", "myproj", "token")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodPut && strings.Contains(req.URL.Path, "reviewers/reviewer-123") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"id": "reviewer-123",
					"displayName": "Jane Doe",
					"isRequired": true
				}`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	reviewer, err := client.AddPullRequestReviewer("myrepo", 101, "reviewer-123", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reviewer.ID != "reviewer-123" || !reviewer.IsRequired {
		t.Errorf("unexpected reviewer data: %+v", reviewer)
	}
}

func TestGetPullRequestDetails(t *testing.T) {
	client := NewClient("myorg", "myproj", "token")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		hdr := make(http.Header)
		hdr.Set("Content-Type", "application/json")

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "attachments") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"count": 1,
					"value": [
						{"id": 1, "url": "https://attachment-url", "author": {"displayName": "Alice"}}
					]
				}`)),
			}
		}

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "workitems") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"count": 1,
					"value": [{"id": "12345"}]
				}`)),
			}
		}

		if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "pullrequests/101") {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body: io.NopCloser(strings.NewReader(`{
					"pullRequestId": 101,
					"title": "Detailed PR",
					"description": "PR description text",
					"status": "active",
					"sourceRefName": "refs/heads/feature/x",
					"targetRefName": "refs/heads/main",
					"repository": {
						"id": "repo-guid",
						"name": "myrepo"
					},
					"reviewers": [
						{"id": "rev1", "displayName": "Bob", "isRequired": true, "vote": 10}
					]
				}`)),
			}
		}

		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	details, err := client.GetPullRequestDetails("myrepo", 101)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if details.PullRequest.PullRequestID != 101 {
		t.Errorf("expected PR ID 101, got %d", details.PullRequest.PullRequestID)
	}
	if len(details.Attachments) != 1 {
		t.Errorf("expected 1 attachment, got %d", len(details.Attachments))
	}
	if len(details.WorkItems) != 1 || details.WorkItems[0].ID != "12345" {
		t.Errorf("expected 1 work item ref 12345, got %+v", details.WorkItems)
	}
	if len(details.PullRequest.Reviewers) != 1 {
		t.Errorf("expected 1 reviewer, got %d", len(details.PullRequest.Reviewers))
	}
}

func TestGetPullRequestThreads(t *testing.T) {
	client := NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		if strings.Contains(req.URL.Path, "/threads") {
			body := `{
				"count": 1,
				"value": [
					{
						"id": 42,
						"status": "active",
						"threadContext": {
							"filePath": "/src/main.go",
							"rightFileStart": {"line": 15, "offset": 1}
						},
						"comments": [
							{
								"id": 1,
								"content": "Please check this error handling",
								"commentType": "text",
								"author": {"displayName": "Alice", "uniqueName": "alice@example.com"}
							},
							{
								"id": 2,
								"parentCommentId": 1,
								"content": "Fixed in latest commit",
								"commentType": "text",
								"author": {"displayName": "Bob", "uniqueName": "bob@example.com"}
							}
						]
					}
				]
			}`
			hdr := make(http.Header)
			hdr.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	threads, err := client.GetPullRequestThreads("myrepo", 101)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(threads))
	}
	if threads[0].ID != 42 || threads[0].Status != "active" {
		t.Errorf("unexpected thread id or status: %+v", threads[0])
	}
	if threads[0].ThreadContext == nil || threads[0].ThreadContext.FilePath != "/src/main.go" {
		t.Errorf("unexpected threadContext: %+v", threads[0].ThreadContext)
	}
	if len(threads[0].Comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(threads[0].Comments))
	}
	if threads[0].Comments[0].Author.DisplayName != "Alice" {
		t.Errorf("expected Alice, got %s", threads[0].Comments[0].Author.DisplayName)
	}
}

func TestAddPullRequestThreadComment(t *testing.T) {
	client := NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/threads/42/comments") {
			body := `{
				"id": 3,
				"parentCommentId": 1,
				"content": "Addressed feedback in commit 123",
				"commentType": "text",
				"author": {"displayName": "Bob", "uniqueName": "bob@example.com"}
			}`
			hdr := make(http.Header)
			hdr.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	comment, err := client.AddPullRequestThreadComment("myrepo", 101, 42, "Addressed feedback in commit 123", 1)
	if err != nil {
		t.Fatalf("unexpected error adding PR comment: %v", err)
	}
	if comment.ID != 3 || comment.ParentCommentID != 1 {
		t.Errorf("unexpected comment response: %+v", comment)
	}
	if comment.Content != "Addressed feedback in commit 123" {
		t.Errorf("unexpected content: %s", comment.Content)
	}
}

func TestUpdatePullRequestThread(t *testing.T) {
	client := NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		if req.Method == http.MethodPatch && strings.Contains(req.URL.Path, "/threads/42") {
			body := `{
				"id": 42,
				"status": "fixed"
			}`
			hdr := make(http.Header)
			hdr.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	thread, err := client.UpdatePullRequestThread("myrepo", 101, 42, "fixed")
	if err != nil {
		t.Fatalf("unexpected error updating PR thread: %v", err)
	}
	if thread.ID != 42 || thread.Status != "fixed" {
		t.Errorf("unexpected thread response: %+v", thread)
	}
}

func TestCreatePullRequestThread(t *testing.T) {
	client := NewClient("testorg", "testproj", "fakepat")
	client.R.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/threads") {
			body := `{
				"id": 50,
				"status": "active",
				"threadContext": {
					"filePath": "/pkg/auth.go",
					"rightFileStart": {"line": 15, "offset": 1},
					"rightFileEnd": {"line": 20, "offset": 1}
				},
				"comments": [
					{
						"id": 501,
						"content": "Please add unit test for this handler",
						"commentType": "text",
						"author": {"displayName": "Reviewer"}
					}
				]
			}`
			hdr := make(http.Header)
			hdr.Set("Content-Type", "application/json")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     hdr,
				Body:       io.NopCloser(strings.NewReader(body)),
			}
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(""))}
	}))

	ctx := &PullRequestThreadContext{
		FilePath: "/pkg/auth.go",
		RightFileStart: &struct {
			Line   int `json:"line"`
			Offset int `json:"offset"`
		}{Line: 15, Offset: 1},
		RightFileEnd: &struct {
			Line   int `json:"line"`
			Offset int `json:"offset"`
		}{Line: 20, Offset: 1},
	}

	thread, err := client.CreatePullRequestThread("myrepo", 101, "Please add unit test for this handler", ctx, "active")
	if err != nil {
		t.Fatalf("unexpected error creating PR thread: %v", err)
	}
	if thread.ID != 50 || thread.Status != "active" {
		t.Errorf("unexpected thread: %+v", thread)
	}
	if thread.ThreadContext == nil || thread.ThreadContext.FilePath != "/pkg/auth.go" {
		t.Errorf("unexpected thread context: %+v", thread.ThreadContext)
	}
	if len(thread.Comments) != 1 || thread.Comments[0].Content != "Please add unit test for this handler" {
		t.Errorf("unexpected thread comments: %+v", thread.Comments)
	}
}



