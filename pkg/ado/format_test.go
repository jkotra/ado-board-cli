package ado

import (
	"strings"
	"testing"
)

func TestFormatMarkdownToAdoHTML(t *testing.T) {
	mockUsers := []*IdentityRef{
		{
			ID:          "141b714f-561b-4171-8933-cb7ecf2ea3fe",
			DisplayName: "Johnson, Bob Henry",
			UniqueName:  "bob.johnson@example.com",
		},
		{
			ID:          "22222222-3333-4444-5555-666666666666",
			DisplayName: "Brown, Carol",
			UniqueName:  "carol.brown@example.com",
		},
		{
			ID:          "33333333-4444-5555-6666-777777777777",
			DisplayName: "Doe, John",
			UniqueName:  "john.doe@example.com",
		},
	}

	mockResolver := func(q string) *IdentityRef {
		q = strings.ToLower(strings.TrimSpace(q))
		for _, u := range mockUsers {
			if strings.ToLower(u.ID) == q ||
				strings.ToLower(u.UniqueName) == q ||
				strings.ToLower(u.DisplayName) == q ||
				strings.ToLower(strings.Split(u.DisplayName, ",")[0]) == q {
				return u
			}
		}
		return nil
	}

	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:  "User screenshot comment with backticks and two mentions",
			input: "changes pushed to `task-667425-orch-ng-changes-for-version`. cc: @Johnson, Bob Henry @Brown, Carol",
			contains: []string{
				"<code>task-667425-orch-ng-changes-for-version</code>",
				`<a href="#" data-vss-mention="version:2.0,141b714f-561b-4171-8933-cb7ecf2ea3fe">@Johnson, Bob Henry</a>`,
				`<a href="#" data-vss-mention="version:2.0,22222222-3333-4444-5555-666666666666">@Brown, Carol</a>`,
			},
		},
		{
			name:  "Autocomplete format with name and email in angle brackets",
			input: "Hey @Doe, John <john.doe@example.com> please review this",
			contains: []string{
				`<a href="#" data-vss-mention="version:2.0,33333333-4444-5555-6666-777777777777">@Doe, John</a>`,
				"please review this",
			},
		},
		{
			name:  "GUID mention token",
			input: "Review by @<141b714f-561b-4171-8933-cb7ecf2ea3fe>",
			contains: []string{
				`<a href="#" data-vss-mention="version:2.0,141b714f-561b-4171-8933-cb7ecf2ea3fe">@Johnson, Bob Henry</a>`,
			},
		},
		{
			name:  "Angle bracket email mention",
			input: "ping @<carol.brown@example.com> about the status",
			contains: []string{
				`<a href="#" data-vss-mention="version:2.0,22222222-3333-4444-5555-666666666666">@Brown, Carol</a>`,
			},
		},
		{
			name:  "Markdown headers, bullet lists, bold and inline code",
			input: "## Implementation\n- Added `feature_flag`\n- **Important**: Test before deployment\n- cc: @Doe, John",
			contains: []string{
				"<h2>Implementation</h2>",
				"<ul>",
				"<li>Added <code>feature_flag</code></li>",
				"<li><strong>Important</strong>: Test before deployment</li>",
				`<a href="#" data-vss-mention="version:2.0,33333333-4444-5555-6666-777777777777">@Doe, John</a>`,
			},
		},
		{
			name:  "Fenced code block preserves @ inside code",
			input: "Here is code:\n```go\n// @Johnson should not be converted\nvar @foo = 1\n```",
			contains: []string{
				"<pre><code",
				"// @Johnson should not be converted",
			},
		},
		{
			name:  "Inline backtick code preserves @ inside code",
			input: "Check variable `@Johnson` in config",
			contains: []string{
				"<code>@Johnson</code>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := FormatMarkdownToAdoHTML(tt.input, mockResolver, mockUsers)
			for _, c := range tt.contains {
				if !strings.Contains(res, c) {
					t.Errorf("expected result to contain %q\nGot:\n%s", c, res)
				}
			}
		})
	}
}

func TestIsHTMLField(t *testing.T) {
	if !isHTMLField("System.Description") {
		t.Errorf("expected System.Description to be HTML field")
	}
	if !isHTMLField("Microsoft.VSTS.Common.AcceptanceCriteria") {
		t.Errorf("expected AcceptanceCriteria to be HTML field")
	}
	if !isHTMLField("Microsoft.VSTS.TCM.ReproSteps") {
		t.Errorf("expected ReproSteps to be HTML field")
	}
	if !isHTMLField("System.History") {
		t.Errorf("expected History to be HTML field")
	}
	if isHTMLField("System.Title") {
		t.Errorf("System.Title should NOT be an HTML field")
	}
	if isHTMLField("System.AssignedTo") {
		t.Errorf("System.AssignedTo should NOT be an HTML field")
	}
}

func TestTagRegexAndActivity(t *testing.T) {
	// Test WorkItem.Activity()
	wi := &WorkItem{
		Fields: map[string]interface{}{
			"Microsoft.VSTS.Common.Activity": "Development",
		},
	}
	if wi.Activity() != "Development" {
		t.Errorf("expected wi.Activity() to be 'Development', got %q", wi.Activity())
	}

	wiCustom := &WorkItem{
		Fields: map[string]interface{}{
			"Custom.Activity": "Testing",
		},
	}
	if wiCustom.Activity() != "Testing" {
		t.Errorf("expected wiCustom.Activity() to be 'Testing', got %q", wiCustom.Activity())
	}

	// Test HasTagMatchingRegex
	if !HasTagMatchingRegex("frontend; backend; release-v1", `^backend$`) {
		t.Errorf("expected true for exact match")
	}
	if !HasTagMatchingRegex("FE-123; BE-456", `^(FE|BE)-`) {
		t.Errorf("expected true for prefix regex")
	}
	if !HasTagMatchingRegex("team-alpha; release-1.0; other", `^team-(alpha|beta), ^release-[0-9.]+`) {
		t.Errorf("expected true for multi-rule comma-separated regex")
	}
	if !HasTagMatchingRegex("release-2.0", `^team-alpha; ^release-.*`) {
		t.Errorf("expected true for multi-rule semicolon-separated regex")
	}
	if HasTagMatchingRegex("other; misc", `^(FE|BE)-`) {
		t.Errorf("expected false when no tags match")
	}
	if !HasTagMatchingRegex("tag1; tag2", `[invalid(regex`) {
		t.Errorf("expected true on invalid regex (safe fallback) without panic")
	}
	if !HasTagMatchingRegex("tag1; tag2", "") {
		t.Errorf("expected true on empty regex (criteria disabled)")
	}

	// Test ParseRegexRules
	rules := ParseRegexRules(`^team-(alpha,beta), ^release-[0-9]{1,3}; ^backend-.*`)
	if len(rules) != 3 || rules[0] != `^team-(alpha,beta)` || rules[1] != `^release-[0-9]{1,3}` || rules[2] != `^backend-.*` {
		t.Errorf("unexpected ParseRegexRules result: %v", rules)
	}

	// Test FilterInheritedTags with single and multiple rules
	inherited := FilterInheritedTags("FE-123; unrelated; BE-456", `^(FE|BE)-`)
	if inherited != "FE-123; BE-456" {
		t.Errorf("expected 'FE-123; BE-456', got %q", inherited)
	}

	inheritedMulti := FilterInheritedTags("FE-100; team-core; unrelated; release-v2", `^FE-.*, ^team-.*, ^release-.*`)
	if inheritedMulti != "FE-100; team-core; release-v2" {
		t.Errorf("expected 'FE-100; team-core; release-v2', got %q", inheritedMulti)
	}

	// When tagRegex is empty, no tags are inherited
	emptyInherited := FilterInheritedTags("tag1; tag2; tag3", "")
	if emptyInherited != "" {
		t.Errorf("expected empty string when tagRegex empty, got %q", emptyInherited)
	}
}

