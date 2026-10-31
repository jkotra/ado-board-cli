package tui

import (
	"fmt"
	"strings"
	"time"

	"ado-board-cli/pkg/ado"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ScheduleTab represents the date field being edited
type ScheduleTab int

const (
	ScheduleTabStart ScheduleTab = iota
	ScheduleTabTarget
	ScheduleTabFinish
)

// ScheduleFocus represents which control within the active tab has focus
type ScheduleFocus int

const (
	ScheduleFocusCalendar ScheduleFocus = iota
	ScheduleFocusTime
	ScheduleFocusInput
	ScheduleFocusButtons
)

type tabState struct {
	InputText   textinput.Model
	ParsedTime  time.Time
	HasDate     bool
	ViewMonth   time.Time // year & month displayed in calendar
	SelectedDay int       // 1-31 selected day in ViewMonth
	Hour        int       // 1-12
	Minute      int       // 0-59
	IsPM        bool      // AM = false, PM = true
	Cleared     bool
}

// ModalSchedule represents the interactive calendar & time picker modal
type ModalSchedule struct {
	WorkItemID  int
	ItemTitle   string
	ItemType    string
	ActiveTab   ScheduleTab
	Focus       ScheduleFocus
	ButtonIndex int // 0: Save, 1: Cancel
	Active      bool

	tabs [3]tabState
}

func setupScheduleInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 25
	ti.Width = 20
	ti.Prompt = ""
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	return ti
}

func NewModalSchedule() *ModalSchedule {
	m := &ModalSchedule{
		ActiveTab:   ScheduleTabStart,
		Focus:       ScheduleFocusCalendar,
		ButtonIndex: 0,
	}
	for i := 0; i < 3; i++ {
		m.tabs[i].InputText = setupScheduleInput("8/18/2026 12:00 AM")
	}
	return m
}

func (m *ModalSchedule) Open(id int, title, itemType, startDate, targetDate, finishDate string) {
	m.WorkItemID = id
	m.ItemTitle = title
	m.ItemType = itemType
	m.Active = true
	m.ActiveTab = ScheduleTabStart
	m.Focus = ScheduleFocusCalendar
	m.ButtonIndex = 0

	dates := []string{startDate, targetDate, finishDate}
	now := time.Now()

	for i := 0; i < 3; i++ {
		raw := strings.TrimSpace(dates[i])
		m.tabs[i].Cleared = false
		if raw != "" {
			if t, ok := ado.ParseDateOrDateTime(raw, now); ok {
				m.tabs[i].ParsedTime = t
				m.tabs[i].HasDate = true
				m.tabs[i].ViewMonth = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, now.Location())
				m.tabs[i].SelectedDay = t.Day()
				h := t.Hour()
				if h >= 12 {
					m.tabs[i].IsPM = true
					if h > 12 {
						m.tabs[i].Hour = h - 12
					} else {
						m.tabs[i].Hour = 12
					}
				} else {
					m.tabs[i].IsPM = false
					if h == 0 {
						m.tabs[i].Hour = 12
					} else {
						m.tabs[i].Hour = h
					}
				}
				m.tabs[i].Minute = t.Minute()
				m.tabs[i].InputText.SetValue(ado.FormatInputDateTime(t))
			} else {
				m.tabs[i].InputText.SetValue(raw)
				m.tabs[i].HasDate = false
				m.tabs[i].ViewMonth = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
				m.tabs[i].SelectedDay = now.Day()
				m.tabs[i].Hour = 9
				m.tabs[i].Minute = 0
				m.tabs[i].IsPM = false
			}
		} else {
			m.tabs[i].InputText.SetValue("")
			m.tabs[i].HasDate = false
			m.tabs[i].ViewMonth = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			m.tabs[i].SelectedDay = now.Day()
			m.tabs[i].Hour = 9
			m.tabs[i].Minute = 0
			m.tabs[i].IsPM = false
		}
	}

	m.updateFocus()
}

func (m *ModalSchedule) Close() {
	m.Active = false
	for i := 0; i < 3; i++ {
		m.tabs[i].InputText.Blur()
	}
}

func (m *ModalSchedule) updateFocus() {
	for i := 0; i < 3; i++ {
		if ScheduleTab(i) == m.ActiveTab && m.Focus == ScheduleFocusInput {
			m.tabs[i].InputText.Focus()
		} else {
			m.tabs[i].InputText.Blur()
		}
	}
}

func (m *ModalSchedule) syncFromInput(tabIdx int) {
	val := strings.TrimSpace(m.tabs[tabIdx].InputText.Value())
	if val == "" {
		m.tabs[tabIdx].HasDate = false
		m.tabs[tabIdx].Cleared = true
		return
	}
	now := time.Now()
	if t, ok := ado.ParseDateOrDateTime(val, now); ok {
		m.tabs[tabIdx].ParsedTime = t
		m.tabs[tabIdx].HasDate = true
		m.tabs[tabIdx].Cleared = false
		m.tabs[tabIdx].ViewMonth = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, now.Location())
		m.tabs[tabIdx].SelectedDay = t.Day()
		h := t.Hour()
		if h >= 12 {
			m.tabs[tabIdx].IsPM = true
			if h > 12 {
				m.tabs[tabIdx].Hour = h - 12
			} else {
				m.tabs[tabIdx].Hour = 12
			}
		} else {
			m.tabs[tabIdx].IsPM = false
			if h == 0 {
				m.tabs[tabIdx].Hour = 12
			} else {
				m.tabs[tabIdx].Hour = h
			}
		}
		m.tabs[tabIdx].Minute = t.Minute()
	}
}

func (m *ModalSchedule) syncToInput(tabIdx int) {
	tab := &m.tabs[tabIdx]
	if !tab.HasDate || tab.Cleared {
		tab.InputText.SetValue("")
		return
	}
	hour24 := tab.Hour
	if tab.IsPM {
		if hour24 < 12 {
			hour24 += 12
		}
	} else {
		if hour24 == 12 {
			hour24 = 0
		}
	}
	loc := time.Now().Location()
	t := time.Date(tab.ViewMonth.Year(), tab.ViewMonth.Month(), tab.SelectedDay, hour24, tab.Minute, 0, 0, loc)
	tab.ParsedTime = t
	tab.InputText.SetValue(ado.FormatInputDateTime(t))
	tab.Cleared = false
}

// SetPresetDate applies a relative or preset date to active tab
func (m *ModalSchedule) SetPresetDate(offsetDays int) {
	tab := &m.tabs[m.ActiveTab]
	now := time.Now()
	target := now.AddDate(0, 0, offsetDays)
	tab.HasDate = true
	tab.Cleared = false
	tab.ViewMonth = time.Date(target.Year(), target.Month(), 1, 0, 0, 0, 0, now.Location())
	tab.SelectedDay = target.Day()
	if tab.Hour == 0 {
		tab.Hour = 9
		tab.Minute = 0
		tab.IsPM = false
	}
	m.syncToInput(int(m.ActiveTab))
}

// ClearActiveDate clears the active tab's date
func (m *ModalSchedule) ClearActiveDate() {
	tab := &m.tabs[m.ActiveTab]
	tab.HasDate = false
	tab.Cleared = true
	tab.InputText.SetValue("")
}

func (m *ModalSchedule) Values() (startDate, targetDate, finishDate string) {
	vals := [3]string{}
	for i := 0; i < 3; i++ {
		tab := &m.tabs[i]
		raw := strings.TrimSpace(tab.InputText.Value())
		if raw == "" || tab.Cleared {
			vals[i] = ""
		} else if tab.HasDate {
			vals[i] = ado.FormatAdoDateTime(tab.ParsedTime)
		} else {
			if t, ok := ado.ParseDateOrDateTime(raw, time.Now()); ok {
				vals[i] = ado.FormatAdoDateTime(t)
			} else {
				vals[i] = raw
			}
		}
	}
	return vals[0], vals[1], vals[2]
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (m *ModalSchedule) prevMonth() {
	tab := &m.tabs[m.ActiveTab]
	tab.ViewMonth = tab.ViewMonth.AddDate(0, -1, 0)
	maxDays := daysInMonth(tab.ViewMonth.Year(), tab.ViewMonth.Month())
	if tab.SelectedDay > maxDays {
		tab.SelectedDay = maxDays
	}
	if tab.HasDate {
		m.syncToInput(int(m.ActiveTab))
	}
}

func (m *ModalSchedule) nextMonth() {
	tab := &m.tabs[m.ActiveTab]
	tab.ViewMonth = tab.ViewMonth.AddDate(0, 1, 0)
	maxDays := daysInMonth(tab.ViewMonth.Year(), tab.ViewMonth.Month())
	if tab.SelectedDay > maxDays {
		tab.SelectedDay = maxDays
	}
	if tab.HasDate {
		m.syncToInput(int(m.ActiveTab))
	}
}

func (m *ModalSchedule) prevYear() {
	tab := &m.tabs[m.ActiveTab]
	tab.ViewMonth = tab.ViewMonth.AddDate(-1, 0, 0)
	maxDays := daysInMonth(tab.ViewMonth.Year(), tab.ViewMonth.Month())
	if tab.SelectedDay > maxDays {
		tab.SelectedDay = maxDays
	}
	if tab.HasDate {
		m.syncToInput(int(m.ActiveTab))
	}
}

func (m *ModalSchedule) nextYear() {
	tab := &m.tabs[m.ActiveTab]
	tab.ViewMonth = tab.ViewMonth.AddDate(1, 0, 0)
	maxDays := daysInMonth(tab.ViewMonth.Year(), tab.ViewMonth.Month())
	if tab.SelectedDay > maxDays {
		tab.SelectedDay = maxDays
	}
	if tab.HasDate {
		m.syncToInput(int(m.ActiveTab))
	}
}

func (m *ModalSchedule) selectDay(day int) {
	tab := &m.tabs[m.ActiveTab]
	maxDays := daysInMonth(tab.ViewMonth.Year(), tab.ViewMonth.Month())
	if day < 1 {
		day = 1
	}
	if day > maxDays {
		day = maxDays
	}
	tab.SelectedDay = day
	tab.HasDate = true
	tab.Cleared = false
	if tab.Hour == 0 {
		tab.Hour = 9
		tab.Minute = 0
		tab.IsPM = false
	}
	m.syncToInput(int(m.ActiveTab))
}

func (m *ModalSchedule) Update(msg tea.Msg) (*ModalSchedule, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	tabIdx := int(m.ActiveTab)
	tab := &m.tabs[tabIdx]

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if m.Focus == ScheduleFocusCalendar {
				m.Focus = ScheduleFocusTime
			} else if m.Focus == ScheduleFocusTime {
				m.Focus = ScheduleFocusInput
			} else if m.Focus == ScheduleFocusInput {
				m.Focus = ScheduleFocusButtons
			} else {
				m.Focus = ScheduleFocusCalendar
				m.ActiveTab = (m.ActiveTab + 1) % 3
			}
			m.updateFocus()
			return m, nil

		case "shift+tab":
			if m.Focus == ScheduleFocusButtons {
				m.Focus = ScheduleFocusInput
			} else if m.Focus == ScheduleFocusInput {
				m.Focus = ScheduleFocusTime
			} else if m.Focus == ScheduleFocusTime {
				m.Focus = ScheduleFocusCalendar
			} else {
				m.Focus = ScheduleFocusButtons
				m.ActiveTab = (m.ActiveTab - 1 + 3) % 3
			}
			m.updateFocus()
			return m, nil

		case "alt+1", "f1":
			m.ActiveTab = ScheduleTabStart
			m.updateFocus()
			return m, nil
		case "alt+2", "f2":
			m.ActiveTab = ScheduleTabTarget
			m.updateFocus()
			return m, nil
		case "alt+3", "f3":
			m.ActiveTab = ScheduleTabFinish
			m.updateFocus()
			return m, nil
		}

		// Direct tab jumps and presets when not in text input
		if m.Focus != ScheduleFocusInput {
			switch msg.String() {
			case "1":
				m.ActiveTab = ScheduleTabStart
				m.updateFocus()
				return m, nil
			case "2":
				m.ActiveTab = ScheduleTabTarget
				m.updateFocus()
				return m, nil
			case "3":
				m.ActiveTab = ScheduleTabFinish
				m.updateFocus()
				return m, nil
			case "t", "T":
				m.SetPresetDate(0)
				return m, nil
			case "m", "M":
				m.SetPresetDate(1)
				return m, nil
			case "w", "W":
				m.SetPresetDate(7)
				return m, nil
			case "x", "X", "delete":
				m.ClearActiveDate()
				return m, nil
			}
		}

		// Handling per focused area
		switch m.Focus {
		case ScheduleFocusCalendar:
			maxDays := daysInMonth(tab.ViewMonth.Year(), tab.ViewMonth.Month())
			switch msg.String() {
			case "left", "h":
				if tab.SelectedDay > 1 {
					m.selectDay(tab.SelectedDay - 1)
				} else {
					m.prevMonth()
					m.selectDay(daysInMonth(tab.ViewMonth.Year(), tab.ViewMonth.Month()))
				}
				return m, nil
			case "right", "l":
				if tab.SelectedDay < maxDays {
					m.selectDay(tab.SelectedDay + 1)
				} else {
					m.nextMonth()
					m.selectDay(1)
				}
				return m, nil
			case "up", "k":
				if tab.SelectedDay > 7 {
					m.selectDay(tab.SelectedDay - 7)
				} else {
					m.prevMonth()
					prevMax := daysInMonth(tab.ViewMonth.Year(), tab.ViewMonth.Month())
					m.selectDay(prevMax - (7 - tab.SelectedDay))
				}
				return m, nil
			case "down", "j":
				if tab.SelectedDay+7 <= maxDays {
					m.selectDay(tab.SelectedDay + 7)
				} else {
					nextDay := (tab.SelectedDay + 7) - maxDays
					m.nextMonth()
					m.selectDay(nextDay)
				}
				return m, nil
			case "<", ",", "pgup", "ctrl+b":
				m.prevMonth()
				return m, nil
			case ">", ".", "pgdown", "ctrl+f":
				m.nextMonth()
				return m, nil
			case "y":
				m.prevYear()
				return m, nil
			case "Y":
				m.nextYear()
				return m, nil
			}

		case ScheduleFocusTime:
			switch msg.String() {
			case "up", "k":
				tab.Hour = (tab.Hour % 12) + 1
				tab.HasDate = true
				m.syncToInput(tabIdx)
				return m, nil
			case "down", "j":
				tab.Hour = tab.Hour - 1
				if tab.Hour < 1 {
					tab.Hour = 12
				}
				tab.HasDate = true
				m.syncToInput(tabIdx)
				return m, nil
			case "right", "l":
				tab.Minute = (tab.Minute + 15) % 60
				tab.HasDate = true
				m.syncToInput(tabIdx)
				return m, nil
			case "left", "h":
				tab.Minute = (tab.Minute - 15 + 60) % 60
				tab.HasDate = true
				m.syncToInput(tabIdx)
				return m, nil
			case "a", "A", "p", "P", "space":
				tab.IsPM = !tab.IsPM
				tab.HasDate = true
				m.syncToInput(tabIdx)
				return m, nil
			}

		case ScheduleFocusInput:
			if msg.String() == "esc" {
				m.Focus = ScheduleFocusCalendar
				m.updateFocus()
				return m, nil
			}
			var cmd tea.Cmd
			prevVal := tab.InputText.Value()
			tab.InputText, cmd = tab.InputText.Update(msg)
			if tab.InputText.Value() != prevVal {
				m.syncFromInput(tabIdx)
			}
			return m, cmd

		case ScheduleFocusButtons:
			switch msg.String() {
			case "left", "h", "right", "l":
				m.ButtonIndex = 1 - m.ButtonIndex
				return m, nil
			}
		}
	}

	return m, nil
}

// RenderCalendar renders a compact 21-char monthly calendar (8 lines)
func (m *ModalSchedule) RenderCalendar(tabIdx int) string {
	tab := &m.tabs[tabIdx]
	now := time.Now()

	monthYearTitle := fmt.Sprintf("◀  %s %d  ▶", tab.ViewMonth.Month().String()[:3], tab.ViewMonth.Year())
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorAzureLight)
	if m.Focus == ScheduleFocusCalendar {
		titleStyle = titleStyle.Foreground(ColorYellow)
	}
	headerLine := lipgloss.NewStyle().Width(21).Align(lipgloss.Center).Render(titleStyle.Render(monthYearTitle))

	// Weekdays header with crisp secondary/sky blue
	dayNames := []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	dayHeaderParts := make([]string, 7)
	for i, dn := range dayNames {
		dayHeaderParts[i] = lipgloss.NewStyle().Width(3).Align(lipgloss.Center).Bold(true).Foreground(ColorSecondary).Render(dn)
	}
	daysHeader := lipgloss.JoinHorizontal(lipgloss.Top, dayHeaderParts...)

	firstOfMonth := time.Date(tab.ViewMonth.Year(), tab.ViewMonth.Month(), 1, 0, 0, 0, 0, tab.ViewMonth.Location())
	startWeekday := int(firstOfMonth.Weekday()) // 0 = Sunday
	totalDays := daysInMonth(tab.ViewMonth.Year(), tab.ViewMonth.Month())

	var rows []string
	var currentCells []string

	// Leading blank days
	for i := 0; i < startWeekday; i++ {
		currentCells = append(currentCells, lipgloss.NewStyle().Width(3).Render("   "))
	}

	for day := 1; day <= totalDays; day++ {
		isToday := (now.Year() == tab.ViewMonth.Year() && now.Month() == tab.ViewMonth.Month() && now.Day() == day)
		isSelected := (tab.HasDate && tab.SelectedDay == day)

		cellStr := fmt.Sprintf("%2d", day)
		cellStyle := lipgloss.NewStyle().Width(3).Align(lipgloss.Center)

		if isSelected {
			if m.Focus == ScheduleFocusCalendar {
				// Vibrant Azure Blue badge with bold white text when active
				cellStyle = cellStyle.Bold(true).Background(ColorAzureDark).Foreground(lipgloss.Color("#FFFFFF"))
			} else {
				// Clean underline without patchy background when inactive
				cellStyle = cellStyle.Bold(true).Foreground(ColorAzureLight).Underline(true)
			}
		} else if isToday {
			cellStyle = cellStyle.Bold(true).Foreground(ColorYellow).Underline(true)
		} else {
			cellStyle = cellStyle.Foreground(ColorFg)
		}

		currentCells = append(currentCells, cellStyle.Render(cellStr))

		if len(currentCells) == 7 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, currentCells...))
			currentCells = []string{}
		}
	}

	// Trailing blank days
	if len(currentCells) > 0 {
		for len(currentCells) < 7 {
			currentCells = append(currentCells, lipgloss.NewStyle().Width(3).Render("   "))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, currentCells...))
	}

	for len(rows) < 6 {
		rows = append(rows, lipgloss.NewStyle().Width(21).Render("                     "))
	}

	calContent := lipgloss.JoinVertical(lipgloss.Left,
		headerLine,
		daysHeader,
		rows[0],
		rows[1],
		rows[2],
		rows[3],
		rows[4],
		rows[5],
	)

	return calContent
}

func renderSchedulePreset(label, key string, isDanger bool) string {
	lblStyle := lipgloss.NewStyle().Foreground(ColorFg)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow)
	if isDanger {
		keyStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(lblStyle.Render(label+" (") + keyStyle.Render(key) + lblStyle.Render(")"))
}

func truncateScheduleTitle(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// RenderTimeAndPresets renders compact time controls & presets side card (8 lines)
func (m *ModalSchedule) RenderTimeAndPresets(tabIdx int) string {
	tab := &m.tabs[tabIdx]

	amPmStr := "AM"
	if tab.IsPM {
		amPmStr = "PM"
	}

	clockDisplay := fmt.Sprintf("%02d : %02d  %s", tab.Hour, tab.Minute, amPmStr)
	clockStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorFg).
		Padding(0, 1)

	if m.Focus == ScheduleFocusTime {
		clockStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorAzureDark).
			Padding(0, 1)
	}

	timeRow := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary).Render("Time: "),
		clockStyle.Render(clockDisplay),
	)
	timeHint := lipgloss.NewStyle().Foreground(ColorMuted).Render("(↑/↓: hr, ←/→: min, p: AM/PM)")

	presetRow1 := lipgloss.JoinHorizontal(lipgloss.Top,
		renderSchedulePreset("Today", "t", false), " ",
		renderSchedulePreset("Tmrw", "m", false),
	)
	presetRow2 := lipgloss.JoinHorizontal(lipgloss.Top,
		renderSchedulePreset("+1 Wk", "w", false), " ",
		renderSchedulePreset("Clear", "x", true),
	)

	inputPrompt := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("Type Date:")
	inputStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(ColorSubtle).
		Width(26).
		Padding(0, 1)

	if m.Focus == ScheduleFocusInput {
		inputStyle = inputStyle.BorderForeground(ColorYellow)
	}
	inputBox := inputStyle.Render(tab.InputText.View())

	content := lipgloss.JoinVertical(lipgloss.Left,
		timeRow,
		timeHint,
		"",
		presetRow1,
		presetRow2,
		"",
		inputPrompt,
		inputBox,
	)

	return content
}

func (m *ModalSchedule) View(width, height int) string {
	if !m.Active {
		return ""
	}

	modalWidth := 60
	if width > 0 && width < 64 {
		modalWidth = width - 4
	}

	// 1. Header with work item badge & title
	header := lipgloss.NewStyle().Bold(true).Foreground(ColorAzureLight).Render(
		fmt.Sprintf("🗓️  Schedule & Dates: #%d (%s)", m.WorkItemID, m.ItemType),
	)
	itemTitle := lipgloss.NewStyle().Foreground(ColorMuted).Render(
		truncateScheduleTitle(m.ItemTitle, 48),
	)

	// 2. Tab Headers: Only active tab has solid pill background, inactive are transparent with bright key indicators
	tabNames := []struct {
		key  string
		name string
	}{
		{"1", "Start Date"},
		{"2", "Target Date"},
		{"3", "Finish Date"},
	}
	tabParts := make([]string, 3)
	for i, tInfo := range tabNames {
		if ScheduleTab(i) == m.ActiveTab {
			tabStyle := lipgloss.NewStyle().Bold(true).Background(ColorAzureDark).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1)
			tabParts[i] = tabStyle.Render(fmt.Sprintf("[%s] %s", tInfo.key, tInfo.name))
		} else {
			keyStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow)
			nameStyle := lipgloss.NewStyle().Foreground(ColorMuted)
			tabStyle := lipgloss.NewStyle().Padding(0, 1)
			tabParts[i] = tabStyle.Render(fmt.Sprintf("[%s] %s", keyStyle.Render(tInfo.key), nameStyle.Render(tInfo.name)))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabParts[0], " ", tabParts[1], " ", tabParts[2])

	tabIdx := int(m.ActiveTab)
	tab := &m.tabs[tabIdx]

	// 3. Current active value display line
	valStr := "None (Not Set)"
	if tab.HasDate && !tab.Cleared {
		valStr = ado.FormatInputDateTime(tab.ParsedTime)
	} else if tab.Cleared {
		valStr = "Cleared"
	}
	statusLine := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Foreground(ColorMuted).Render("Current Value: "),
		lipgloss.NewStyle().Foreground(ColorGreen).Bold(true).Render(valStr),
	)

	// 4. Side-by-side columns: Calendar (21 cols) + Spacer (3 cols) + Time/Presets (28 cols) = 52 cols
	calendarView := m.RenderCalendar(tabIdx)
	timePresetsView := m.RenderTimeAndPresets(tabIdx)
	sideBySide := lipgloss.JoinHorizontal(lipgloss.Top, calendarView, "   ", timePresetsView)

	// 5. Action Buttons: Transparent background when inactive, solid pill when focused
	btnSaveStyle := lipgloss.NewStyle().Padding(0, 2)
	btnCancelStyle := lipgloss.NewStyle().Padding(0, 2)
	if m.Focus == ScheduleFocusButtons && m.ButtonIndex == 0 {
		btnSaveStyle = btnSaveStyle.Bold(true).Background(ColorGreen).Foreground(lipgloss.Color("#0F172A"))
		btnCancelStyle = btnCancelStyle.Foreground(ColorMuted)
	} else if m.Focus == ScheduleFocusButtons && m.ButtonIndex == 1 {
		btnSaveStyle = btnSaveStyle.Bold(true).Foreground(ColorGreen)
		btnCancelStyle = btnCancelStyle.Bold(true).Background(ColorAccent).Foreground(lipgloss.Color("#FFFFFF"))
	} else {
		btnSaveStyle = btnSaveStyle.Bold(true).Foreground(ColorGreen)
		btnCancelStyle = btnCancelStyle.Foreground(ColorMuted)
	}

	saveBtn := btnSaveStyle.Render("[ Save Schedule ]")
	cancelBtn := btnCancelStyle.Render("[ Cancel ]")
	buttonRow := lipgloss.JoinHorizontal(lipgloss.Center, saveBtn, "    ", cancelBtn)

	// 6. Navigation hints footer (compact, single line)
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorRibbonKey)
	lblStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	dot := lipgloss.NewStyle().Foreground(ColorSubtle).Render(" • ")

	footerHints := lipgloss.JoinHorizontal(lipgloss.Left,
		keyStyle.Render("[Tab] ") + lblStyle.Render("Focus"), dot,
		keyStyle.Render("[1/2/3] ") + lblStyle.Render("Tab"), dot,
		keyStyle.Render("[Arrows] ") + lblStyle.Render("Move"), dot,
		keyStyle.Render("[Enter] ") + lblStyle.Render("Save"),
	)

	content := lipgloss.JoinVertical(lipgloss.Center,
		header,
		itemTitle,
		"",
		tabBar,
		statusLine,
		"",
		sideBySide,
		"",
		buttonRow,
		footerHints,
	)

	modalBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorAzureMid).
		Padding(1, 2).
		Width(modalWidth).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modalBox)
}
