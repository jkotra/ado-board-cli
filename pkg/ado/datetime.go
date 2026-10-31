package ado

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Common date/time formats
var standardDateTimeLayouts = []string{
	// 12-hour formats with AM/PM (like ADO Web UI: "8/18/2026 12:00 AM", "8/21/2026 3:57 PM")
	"1/2/2006 3:04 PM",
	"1/2/2006 3:04:05 PM",
	"1/2/2006 03:04 PM",
	"1/2/2006 03:04:05 PM",
	"01/02/2006 3:04 PM",
	"01/02/2006 3:04:05 PM",
	"01/02/2006 03:04 PM",
	"01/02/2006 03:04:05 PM",
	"2006-01-02 3:04 PM",
	"2006-01-02 03:04 PM",
	"2006/01/02 3:04 PM",
	"2006/01/02 03:04 PM",

	// 24-hour formats
	"1/2/2006 15:04",
	"1/2/2006 15:04:05",
	"01/02/2006 15:04",
	"01/02/2006 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02 15:04:05",
	"2006/01/02 15:04",
	"2006/01/02 15:04:05",

	// Date only formats (default to 00:00:00 local)
	"1/2/2006",
	"01/02/2006",
	"2006-01-02",
	"2006/01/02",
	"Jan 2, 2006",
	"Jan 02, 2006",
	"January 2, 2006",
	"January 02, 2006",
	"2 Jan 2006",
	"02 Jan 2006",
	"2 January 2006",
	"02 January 2006",

	// ISO 8601 & RFC3339 formats
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05.999999999Z07:00",
}

var (
	relDateRe    = regexp.MustCompile(`^([+-]?\d+)\s*([dwmy])$`)
	timeSuffixRe = regexp.MustCompile(`^(today|tomorrow|yesterday)\s+(?:at\s+)?(.+)$`)
)

// ParseDateOrDateTime resiliently parses a date or date+time string into a time.Time
func ParseDateOrDateTime(input string, now time.Time) (time.Time, bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return time.Time{}, false
	}
	if now.IsZero() {
		now = time.Now()
	}

	lower := strings.ToLower(s)

	// Relative keywords
	if lower == "today" {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), true
	}
	if lower == "tomorrow" {
		t := now.AddDate(0, 0, 1)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()), true
	}
	if lower == "yesterday" {
		t := now.AddDate(0, 0, -1)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()), true
	}

	// Relative offsets like "+1d", "+3d", "+1w", "-2d"
	if m := relDateRe.FindStringSubmatch(lower); len(m) == 3 {
		num, err := strconv.Atoi(m[1])
		if err == nil {
			unit := m[2]
			switch unit {
			case "d":
				t := now.AddDate(0, 0, num)
				return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()), true
			case "w":
				t := now.AddDate(0, 0, num*7)
				return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()), true
			case "m":
				t := now.AddDate(0, num, 0)
				return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()), true
			case "y":
				t := now.AddDate(num, 0, 0)
				return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location()), true
			}
		}
	}

	// Expressions like "today 5pm", "tomorrow at 9:00 AM", "tomorrow 17:00"
	if m := timeSuffixRe.FindStringSubmatch(lower); len(m) == 3 {
		baseDay := now
		if m[1] == "tomorrow" {
			baseDay = now.AddDate(0, 0, 1)
		} else if m[1] == "yesterday" {
			baseDay = now.AddDate(0, 0, -1)
		}
		timeStr := strings.ToUpper(strings.TrimSpace(m[2]))
		for _, tLayout := range []string{"3:04 PM", "3:04PM", "3 PM", "3PM", "03:04 PM", "03:04PM", "15:04", "15:04:05"} {
			if tParsed, err := time.ParseInLocation(tLayout, timeStr, now.Location()); err == nil {
				return time.Date(baseDay.Year(), baseDay.Month(), baseDay.Day(), tParsed.Hour(), tParsed.Minute(), tParsed.Second(), 0, now.Location()), true
			}
		}
	}

	// Try standard layouts
	for _, layout := range standardDateTimeLayouts {
		if t, err := time.ParseInLocation(layout, s, now.Location()); err == nil {
			return t.In(now.Location()), true
		}
		if strings.HasSuffix(layout, "Z") {
			if t, err := time.Parse(layout, s); err == nil {
				return t.In(now.Location()), true
			}
		}
	}

	return time.Time{}, false
}

// FormatAdoDateTime formats a time.Time to Azure DevOps REST API ISO-8601 UTC format
func FormatAdoDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// FormatDisplayDateTime formats an ISO-8601 / RFC3339 date string into clean ADO Web UI format in local time (e.g. "8/18/2026 12:00 AM")
func FormatDisplayDateTime(dateStr string) string {
	return FormatDisplayDateTimeInLocation(dateStr, time.Local)
}

// FormatDisplayDateTimeInLocation formats an ISO-8601 / RFC3339 date string into clean ADO Web UI format in a specified location
func FormatDisplayDateTimeInLocation(dateStr string, loc *time.Location) string {
	dateStr = strings.TrimSpace(dateStr)
	if dateStr == "" {
		return "—"
	}
	if loc == nil {
		loc = time.Local
	}

	ref := time.Now().In(loc)
	t, ok := ParseDateOrDateTime(dateStr, ref)
	if !ok {
		return dateStr
	}

	return FormatInputDateTime(t.In(loc))
}

// FormatInputDateTime formats a time.Time as "M/D/YYYY h:mm A" (e.g. "8/18/2026 12:00 AM")
func FormatInputDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	hour := t.Hour()
	minute := t.Minute()
	amPm := "AM"
	displayHour := hour
	if hour >= 12 {
		amPm = "PM"
		if hour > 12 {
			displayHour = hour - 12
		}
	} else if hour == 0 {
		displayHour = 12
	}

	return fmt.Sprintf("%d/%d/%d %d:%02d %s", int(t.Month()), t.Day(), t.Year(), displayHour, minute, amPm)
}

// FormatDateOnly formats a time.Time as "M/D/YYYY"
func FormatDateOnly(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return fmt.Sprintf("%d/%d/%d", int(t.Month()), t.Day(), t.Year())
}
