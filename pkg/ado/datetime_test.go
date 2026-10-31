package ado

import (
	"testing"
	"time"
)

func TestParseDateOrDateTime(t *testing.T) {
	refTime := time.Date(2026, 8, 22, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		input    string
		expected string
	}{
		{"8/18/2026 12:00 AM", "2026-08-18T00:00:00Z"},
		{"8/19/2026 8:01 AM", "2026-08-19T08:01:00Z"},
		{"8/21/2026 3:57 PM", "2026-08-21T15:57:00Z"},
		{"2026-08-18", "2026-08-18T00:00:00Z"},
		{"2026-08-18 15:30", "2026-08-18T15:30:00Z"},
		{"2026-08-18T12:00:00Z", "2026-08-18T12:00:00Z"},
		{"today", "2026-08-22T00:00:00Z"},
		{"tomorrow", "2026-08-23T00:00:00Z"},
		{"yesterday", "2026-08-21T00:00:00Z"},
		{"+3d", "2026-08-25T00:00:00Z"},
		{"+1w", "2026-08-29T00:00:00Z"},
		{"tomorrow 5pm", "2026-08-23T17:00:00Z"},
		{"today at 9:00 AM", "2026-08-22T09:00:00Z"},
	}

	for _, tt := range tests {
		parsed, ok := ParseDateOrDateTime(tt.input, refTime)
		if !ok {
			t.Fatalf("failed to parse valid input %q", tt.input)
		}
		formatted := FormatAdoDateTime(parsed)
		if formatted != tt.expected {
			t.Errorf("input %q: expected %s, got %s", tt.input, tt.expected, formatted)
		}
	}
}

func TestFormatDisplayDateTime(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2026-08-18T00:00:00Z", "8/18/2026 12:00 AM"},
		{"2026-08-19T08:01:00Z", "8/19/2026 8:01 AM"},
		{"2026-08-21T15:57:00Z", "8/21/2026 3:57 PM"},
		{"", "—"},
		{"invalid-date", "invalid-date"},
	}

	for _, tt := range tests {
		res := FormatDisplayDateTimeInLocation(tt.input, time.UTC)
		if res != tt.expected {
			t.Errorf("input %q: expected %s, got %s", tt.input, tt.expected, res)
		}
	}

	// Test IST (UTC+5:30) timezone conversion
	istLoc := time.FixedZone("IST", 5*3600+30*60)
	istRes := FormatDisplayDateTimeInLocation("2026-09-02T18:17:00Z", istLoc)
	if istRes != "9/2/2026 11:47 PM" {
		t.Errorf("expected '9/2/2026 11:47 PM' for 18:17 UTC in IST, got %q", istRes)
	}
}
