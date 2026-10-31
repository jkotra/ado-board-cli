package tui

import (
	"strings"
	"testing"
)

func TestResolveMentions(t *testing.T) {
	// Register test identities
	RegisterIdentity("0f433be8-555e-621f-8adb-94531c9bb685", "Anderson, Alice")
	RegisterIdentity("1aa6436d-015d-62f3-a3cb-ef086246c3b3", "Doe, John")
	RegisterIdentity("alice.anderson@example.com", "Anderson, Alice")

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "ADO GUID mention",
			input:    "@<0F433BE8-555E-621F-8ADB-94531C9BB685> please provide an update, if any. Thanks.",
			contains: "@Anderson, Alice",
		},
		{
			name:     "ADO email mention",
			input:    "@<alice.anderson@example.com> hello",
			contains: "@Anderson, Alice",
		},
		{
			name:     "ADO named mention",
			input:    "@<Doe, John> ping",
			contains: "@Doe, John",
		},
		{
			name:     "ADO HTML vss-mention",
			input:    `<a href="#" data-vss-mention="version:2.0,123">@Doe, John</a> look at this`,
			contains: "@Doe, John",
		},
		{
			name:     "StripAndFormatHTML with mention",
			input:    "<p>Hello @<0F433BE8-555E-621F-8ADB-94531C9BB685><br>Status update needed.</p>",
			contains: "@Anderson, Alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := StripAndFormatHTML(tt.input)
			if !strings.Contains(res, tt.contains) {
				t.Errorf("expected result to contain %q, got: %s", tt.contains, res)
			}
		})
	}
}
