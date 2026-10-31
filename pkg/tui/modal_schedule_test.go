package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModalScheduleNavigationAndValues(t *testing.T) {
	modal := NewModalSchedule()
	if modal.Active {
		t.Fatalf("expected modal to be inactive initially")
	}

	// Open modal with sample dates
	start := "8/18/2026 12:00 AM"
	target := "8/19/2026 8:01 AM"
	finish := "8/21/2026 3:57 PM"

	modal.Open(666236, "Implement User Authentication", "Task", start, target, finish)
	if !modal.Active {
		t.Fatalf("expected modal to be active after Open()")
	}
	if modal.WorkItemID != 666236 {
		t.Errorf("expected WorkItemID=666236, got %d", modal.WorkItemID)
	}

	// Default focus should be ScheduleFocusCalendar
	if modal.Focus != ScheduleFocusCalendar {
		t.Errorf("expected initial focus=ScheduleFocusCalendar, got %v", modal.Focus)
	}

	// Check tabs initialized properly
	if modal.tabs[0].Hour != 12 || modal.tabs[0].IsPM {
		t.Errorf("expected tab 0 (Start Date) 12 AM, got %d (isPM=%v)", modal.tabs[0].Hour, modal.tabs[0].IsPM)
	}
	if modal.tabs[1].Hour != 8 || modal.tabs[1].Minute != 1 || modal.tabs[1].IsPM {
		t.Errorf("expected tab 1 (Target Date) 8:01 AM, got %d:%02d (isPM=%v)", modal.tabs[1].Hour, modal.tabs[1].Minute, modal.tabs[1].IsPM)
	}
	if modal.tabs[2].Hour != 3 || modal.tabs[2].Minute != 57 || !modal.tabs[2].IsPM {
		t.Errorf("expected tab 2 (Finish Date) 3:57 PM, got %d:%02d (isPM=%v)", modal.tabs[2].Hour, modal.tabs[2].Minute, modal.tabs[2].IsPM)
	}

	// Direct tab jumps with 2 and 3 keys while in Calendar focus
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if modal.ActiveTab != ScheduleTabTarget {
		t.Errorf("expected active tab=ScheduleTabTarget, got %v", modal.ActiveTab)
	}

	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if modal.ActiveTab != ScheduleTabFinish {
		t.Errorf("expected active tab=ScheduleTabFinish, got %v", modal.ActiveTab)
	}

	// Test tab focus cycling: Calendar -> Time -> Input -> Buttons -> Calendar
	modal.Focus = ScheduleFocusCalendar
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	if modal.Focus != ScheduleFocusTime {
		t.Errorf("expected focus=ScheduleFocusTime, got %v", modal.Focus)
	}

	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	if modal.Focus != ScheduleFocusInput {
		t.Errorf("expected focus=ScheduleFocusInput, got %v", modal.Focus)
	}

	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyTab})
	if modal.Focus != ScheduleFocusButtons {
		t.Errorf("expected focus=ScheduleFocusButtons, got %v", modal.Focus)
	}

	// Test Values output
	vStart, vTarget, vFinish := modal.Values()
	if vStart == "" || vTarget == "" || vFinish == "" {
		t.Errorf("expected all 3 values non-empty, got start=%q, target=%q, finish=%q", vStart, vTarget, vFinish)
	}

	// Verify View rendering fits within standard 24-line terminal
	view := modal.View(80, 24)
	if len(view) == 0 {
		t.Errorf("expected non-empty View()")
	}
	lines := strings.Split(view, "\n")
	if len(lines) > 24 {
		t.Errorf("expected view line count <= 24 for 80x24 terminal, got %d", len(lines))
	}
}

func TestModalSchedulePresetsAndTimeControls(t *testing.T) {
	modal := NewModalSchedule()
	modal.Open(101, "Test Schedule", "Task", "", "", "")

	// Preset 't' for today from Calendar focus
	modal.Focus = ScheduleFocusCalendar
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	now := time.Now()
	tab := &modal.tabs[modal.ActiveTab]
	if tab.ViewMonth.Year() != now.Year() || tab.ViewMonth.Month() != now.Month() || tab.SelectedDay != now.Day() {
		t.Errorf("expected tab date to be today %v, got %d-%02d-%02d", now, tab.ViewMonth.Year(), tab.ViewMonth.Month(), tab.SelectedDay)
	}

	// Time controls in ScheduleFocusTime
	modal.Focus = ScheduleFocusTime
	tab.Hour = 9
	tab.IsPM = false
	// Up increments hour
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyUp})
	if tab.Hour != 10 {
		t.Errorf("expected hour=10 after KeyUp, got %d", tab.Hour)
	}

	// Toggle AM/PM with 'p'
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if !tab.IsPM {
		t.Errorf("expected IsPM=true after 'p' key")
	}

	// Clear preset with 'x'
	modal, _ = modal.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if tab.InputText.Value() != "" {
		t.Errorf("expected empty input after clear, got %q", tab.InputText.Value())
	}
}
