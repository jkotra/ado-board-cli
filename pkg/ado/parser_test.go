package ado

import (
	"testing"
)

func TestParseWorkItemLink(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		defaultOrg  string
		defaultProj string
		expectedID  int
		expectedOrg string
		expectedPrj string
		expectErr   bool
	}{
		{
			name:        "Standard dev.azure.com work item URL",
			input:       "https://dev.azure.com/my-org/my-project/_workitems/edit/665688/",
			defaultOrg:  "",
			defaultProj: "",
			expectedID:  665688,
			expectedOrg: "my-org",
			expectedPrj: "my-project",
			expectErr:   false,
		},
		{
			name:        "Legacy visualstudio.com URL",
			input:       "https://myorg.visualstudio.com/MyProject/_workitems/edit/12345",
			defaultOrg:  "",
			defaultProj: "",
			expectedID:  12345,
			expectedOrg: "myorg",
			expectedPrj: "MyProject",
			expectErr:   false,
		},
		{
			name:        "REST API work item URL",
			input:       "https://dev.azure.com/my-org/my-project/_apis/wit/workitems/659219",
			defaultOrg:  "",
			defaultProj: "",
			expectedID:  659219,
			expectedOrg: "my-org",
			expectedPrj: "my-project",
			expectErr:   false,
		},
		{
			name:        "Plain numeric ID with default org and project",
			input:       "665688",
			defaultOrg:  "my-org",
			defaultProj: "my-project",
			expectedID:  665688,
			expectedOrg: "my-org",
			expectedPrj: "my-project",
			expectErr:   false,
		},
		{
			name:        "Plain numeric ID with hash prefix",
			input:       "#665688",
			defaultOrg:  "my-org",
			defaultProj: "my-project",
			expectedID:  665688,
			expectedOrg: "my-org",
			expectedPrj: "my-project",
			expectErr:   false,
		},
		{
			name:        "Invalid input",
			input:       "https://google.com/search?q=test",
			defaultOrg:  "",
			defaultProj: "",
			expectedID:  0,
			expectedOrg: "",
			expectedPrj: "",
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseWorkItemLink(tt.input, tt.defaultOrg, tt.defaultProj)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.ID != tt.expectedID {
				t.Errorf("expected ID %d, got %d", tt.expectedID, res.ID)
			}
			if res.Org != tt.expectedOrg {
				t.Errorf("expected Org %s, got %s", tt.expectedOrg, res.Org)
			}
			if res.Project != tt.expectedPrj {
				t.Errorf("expected Project %s, got %s", tt.expectedPrj, res.Project)
			}
		})
	}
}

func TestParseTrackTarget(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		defaultOrg  string
		defaultProj string
		expectedTyp TrackTargetType
		expectedID  int
		expectedQID string
		expectedOrg string
		expectedPrj string
		expectErr   bool
	}{
		{
			name:        "Dev Azure Query URL (User Example)",
			input:       "https://dev.azure.com/my-org/my-project/_queries/query/7509d605-466a-4d8c-847f-c14e1b46a647/",
			defaultOrg:  "",
			defaultProj: "",
			expectedTyp: TrackTargetQuery,
			expectedQID: "7509d605-466a-4d8c-847f-c14e1b46a647",
			expectedOrg: "my-org",
			expectedPrj: "my-project",
			expectErr:   false,
		},
		{
			name:        "Dev Azure Query Edit URL",
			input:       "https://dev.azure.com/my-org/my-project/_queries/query-edit/7509d605-466a-4d8c-847f-c14e1b46a647/",
			defaultOrg:  "",
			defaultProj: "",
			expectedTyp: TrackTargetQuery,
			expectedQID: "7509d605-466a-4d8c-847f-c14e1b46a647",
			expectedOrg: "my-org",
			expectedPrj: "my-project",
			expectErr:   false,
		},
		{
			name:        "Visual Studio Query URL",
			input:       "https://my-org.visualstudio.com/my-project/_queries/query/7509d605-466a-4d8c-847f-c14e1b46a647",
			defaultOrg:  "",
			defaultProj: "",
			expectedTyp: TrackTargetQuery,
			expectedQID: "7509d605-466a-4d8c-847f-c14e1b46a647",
			expectedOrg: "my-org",
			expectedPrj: "my-project",
			expectErr:   false,
		},
		{
			name:        "Query UUID alone",
			input:       "7509d605-466a-4d8c-847f-c14e1b46a647",
			defaultOrg:  "my-org",
			defaultProj: "my-project",
			expectedTyp: TrackTargetQuery,
			expectedQID: "7509d605-466a-4d8c-847f-c14e1b46a647",
			expectedOrg: "my-org",
			expectedPrj: "my-project",
			expectErr:   false,
		},
		{
			name:        "Work Item URL",
			input:       "https://dev.azure.com/my-org/my-project/_workitems/edit/665688",
			defaultOrg:  "",
			defaultProj: "",
			expectedTyp: TrackTargetWorkItem,
			expectedID:  665688,
			expectedOrg: "my-org",
			expectedPrj: "my-project",
			expectErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseTrackTarget(tt.input, tt.defaultOrg, tt.defaultProj)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Type != tt.expectedTyp {
				t.Errorf("expected Type %v, got %v", tt.expectedTyp, res.Type)
			}
			if res.QueryID != tt.expectedQID {
				t.Errorf("expected QueryID %q, got %q", tt.expectedQID, res.QueryID)
			}
			if res.ItemID != tt.expectedID {
				t.Errorf("expected ItemID %d, got %d", tt.expectedID, res.ItemID)
			}
			if res.Org != tt.expectedOrg {
				t.Errorf("expected Org %s, got %s", tt.expectedOrg, res.Org)
			}
			if res.Project != tt.expectedPrj {
				t.Errorf("expected Project %s, got %s", tt.expectedPrj, res.Project)
			}
		})
	}
}

