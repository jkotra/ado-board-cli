package tui

import (
	"fmt"
	"strings"
	"time"

	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModalFilterAction describes the result of interacting with the filter modal
type ModalFilterAction int

const (
	FilterActionNone ModalFilterAction = iota
	FilterActionApplyGlobal
	FilterActionApplyTab
	FilterActionClearTab
	FilterActionClearAll
	FilterActionCancel
)

const (
	FilterActionApply = FilterActionApplyGlobal
	FilterActionClear = FilterActionClearAll
)

const (
	filterFieldStatus = iota
	filterFieldOp1
	filterFieldAssignee
	filterFieldOp2
	filterFieldType
	filterFieldOp3
	filterFieldSearch
	filterFieldButtons
	filterFieldCount
)

// MarqueeTickMsg is fired on a recurring timer to animate horizontal scrolling for clipped text
type MarqueeTickMsg time.Time

// ScrollClippedText smoothly scrolls text horizontally if its length exceeds maxLen.
// It pauses at the start and end of the text for readability.
func ScrollClippedText(text string, maxLen int, tick int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}

	overflow := len(runes) - maxLen
	totalTicks := overflow + 6 // 3 ticks pause at start, 3 ticks pause at end
	phase := tick % totalTicks

	var startOffset int
	if phase < 3 {
		startOffset = 0
	} else if phase < 3+overflow {
		startOffset = phase - 3
	} else {
		startOffset = overflow
	}

	endOffset := startOffset + maxLen
	if endOffset > len(runes) {
		endOffset = len(runes)
	}

	return string(runes[startOffset:endOffset])
}

// ModalFilter provides an interactive filter configuration dialog with multi-select checkboxes
type ModalFilter struct {
	StatusOptions     []string
	SelectedStatuses  []string
	Op1               string // "AND" or "OR"
	AssigneeOptions   []string
	SelectedAssignees []string
	Op2               string // "AND" or "OR"
	TypeOptions       []string
	SelectedTypes     []string
	Op3               string // "AND" or "OR"
	SearchInput       textinput.Model

	ActiveTabTitle  string
	TabFilterActive bool

	Picker          *FilterPicker
	FocusIndex      int // 0 to filterFieldCount-1
	ActionButtonIdx int // 0: Apply Global, 1: Apply Tab, 2: Clear Tab, 3: Clear All, 4: Cancel
	MarqueeTick     int
	Width           int
	Height          int
}

// FilterPicker provides a multi-select search-enabled popup dialog with checkboxes
type FilterPicker struct {
	Active          bool
	Title           string
	Field           int
	AllOptions      []string
	Filtered        []string
	SelectedOptions map[string]bool
	OrigSelected    map[string]bool
	Modified        bool
	SearchInput     textinput.Model
	Cursor          int
	ScrollOffset    int
	MaxVisible      int
	BadgeColor      lipgloss.Color
	MarqueeTick     int
}

func NewFilterPicker() *FilterPicker {
	ti := textinput.New()
	ti.Placeholder = "Type to search options..."
	ti.CharLimit = 50
	ti.Width = 46
	ti.Prompt = "🔍 "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	return &FilterPicker{
		SearchInput:     ti,
		MaxVisible:      7,
		SelectedOptions: make(map[string]bool),
		OrigSelected:    make(map[string]bool),
	}
}

func (p *FilterPicker) Open(title string, field int, options []string, currentSelected []string, badgeColor lipgloss.Color) {
	p.Active = true
	p.Title = title
	p.Field = field
	p.AllOptions = options
	p.BadgeColor = badgeColor
	p.MarqueeTick = 0
	p.Modified = false
	p.SearchInput.Reset()
	p.SearchInput.Focus()

	p.SelectedOptions = make(map[string]bool)
	p.OrigSelected = make(map[string]bool)

	if len(currentSelected) == 0 {
		p.SelectedOptions["[All]"] = true
		p.OrigSelected["[All]"] = true
	} else {
		hasAll := false
		for _, s := range currentSelected {
			if strings.EqualFold(s, "[all]") || strings.EqualFold(s, "all") {
				hasAll = true
				break
			}
			p.SelectedOptions[s] = true
			p.OrigSelected[s] = true
		}
		if hasAll || len(p.SelectedOptions) == 0 {
			p.SelectedOptions = map[string]bool{"[All]": true}
			p.OrigSelected = map[string]bool{"[All]": true}
		}
	}

	p.filter("")
	p.Cursor = 0
	p.ScrollOffset = 0
}

func (p *FilterPicker) Close() {
	p.Active = false
	p.SearchInput.Blur()
}

func (p *FilterPicker) filter(query string) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		p.Filtered = p.AllOptions
	} else {
		var filtered []string
		for _, opt := range p.AllOptions {
			if strings.Contains(strings.ToLower(opt), q) || opt == "[All]" {
				filtered = append(filtered, opt)
			}
		}
		p.Filtered = filtered
	}

	if p.Cursor >= len(p.Filtered) {
		p.Cursor = 0
		p.ScrollOffset = 0
	}
}

func (p *FilterPicker) ToggleOption(opt string) {
	p.Modified = true
	if opt == "[All]" {
		if p.SelectedOptions["[All]"] {
			// Uncheck [All] -> clear selection
			p.SelectedOptions = make(map[string]bool)
		} else {
			// Check [All] -> select all (clear individual selections)
			p.SelectedOptions = map[string]bool{"[All]": true}
		}
		return
	}

	// Specific option toggled
	if p.SelectedOptions["[All]"] {
		// When unchecking something while [All] was active, select this single option
		delete(p.SelectedOptions, "[All]")
		p.SelectedOptions = map[string]bool{opt: true}
		return
	}

	if p.SelectedOptions[opt] {
		delete(p.SelectedOptions, opt)
		if len(p.SelectedOptions) == 0 {
			p.SelectedOptions["[All]"] = true
		}
	} else {
		p.SelectedOptions[opt] = true
	}
}

func (p *FilterPicker) GetSelectedList() []string {
	if p.SelectedOptions["[All]"] {
		return nil
	}

	var res []string
	for _, opt := range p.AllOptions {
		if opt != "[All]" && p.SelectedOptions[opt] {
			res = append(res, opt)
		}
	}
	return res
}

func (p *FilterPicker) SelectedCount() int {
	if p.SelectedOptions["[All]"] {
		return 0
	}
	count := 0
	for _, opt := range p.AllOptions {
		if opt != "[All]" && p.SelectedOptions[opt] {
			count++
		}
	}
	return count
}

func (p *FilterPicker) Update(msg tea.Msg) (selectedVals []string, selected bool, closed bool, cmd tea.Cmd) {
	if !p.Active {
		return nil, false, false, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Restore original selection
			p.SelectedOptions = make(map[string]bool)
			for k, v := range p.OrigSelected {
				p.SelectedOptions[k] = v
			}
			p.Close()
			return nil, false, true, nil

		case "enter":
			if !p.Modified && len(p.Filtered) > 0 && p.Cursor >= 0 && p.Cursor < len(p.Filtered) {
				opt := p.Filtered[p.Cursor]
				if opt == "[All]" {
					p.SelectedOptions = map[string]bool{"[All]": true}
				} else {
					p.SelectedOptions = map[string]bool{opt: true}
				}
			}
			selectedList := p.GetSelectedList()
			p.Close()
			return selectedList, true, true, nil

		case " ":
			// Toggle checkbox for current item
			if len(p.Filtered) > 0 && p.Cursor >= 0 && p.Cursor < len(p.Filtered) {
				opt := p.Filtered[p.Cursor]
				p.ToggleOption(opt)
			}
			return nil, false, false, nil

		case "up", "ctrl+p":
			if len(p.Filtered) > 0 {
				p.MarqueeTick = 0
				p.Cursor--
				if p.Cursor < 0 {
					p.Cursor = len(p.Filtered) - 1
				}
				if p.Cursor < p.ScrollOffset {
					p.ScrollOffset = p.Cursor
				}
				if p.Cursor >= p.ScrollOffset+p.MaxVisible {
					p.ScrollOffset = p.Cursor - p.MaxVisible + 1
				}
			}
			return nil, false, false, nil

		case "down", "ctrl+n":
			if len(p.Filtered) > 0 {
				p.MarqueeTick = 0
				p.Cursor++
				if p.Cursor >= len(p.Filtered) {
					p.Cursor = 0
					p.ScrollOffset = 0
				}
				if p.Cursor >= p.ScrollOffset+p.MaxVisible {
					p.ScrollOffset = p.Cursor - p.MaxVisible + 1
				}
				if p.Cursor < p.ScrollOffset {
					p.ScrollOffset = p.Cursor
				}
			}
			return nil, false, false, nil
		}

		prevVal := p.SearchInput.Value()
		p.SearchInput, cmd = p.SearchInput.Update(msg)
		if p.SearchInput.Value() != prevVal {
			p.MarqueeTick = 0
			p.filter(p.SearchInput.Value())
		}
		return nil, false, false, cmd
	}

	return nil, false, false, nil
}

func (p *FilterPicker) View(parentWidth, parentHeight int) string {
	if !p.Active {
		return ""
	}

	pickerWidth := 58
	if parentWidth > 0 && parentWidth < 64 {
		pickerWidth = parentWidth - 4
	}
	if pickerWidth < 46 {
		pickerWidth = 46
	}

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(p.Title)

	selCount := p.SelectedCount()
	var countStr string
	if p.SelectedOptions["[All]"] || selCount == 0 {
		countStr = "All selected"
	} else {
		countStr = fmt.Sprintf("%d selected", selCount)
	}

	subHeader := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(fmt.Sprintf("Showing %d of %d options (%s)", len(p.Filtered), len(p.AllOptions), countStr))

	searchBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSecondary).
		Padding(0, 1).
		Width(pickerWidth - 6).
		Render(p.SearchInput.View())

	maxOptLen := pickerWidth - 14
	if maxOptLen < 15 {
		maxOptLen = 15
	}

	var listRows []string
	if len(p.Filtered) == 0 {
		listRows = append(listRows, lipgloss.NewStyle().Foreground(ColorMuted).Render("  No matching options found"))
	} else {
		endIdx := p.ScrollOffset + p.MaxVisible
		if endIdx > len(p.Filtered) {
			endIdx = len(p.Filtered)
		}

		for i := p.ScrollOffset; i < endIdx; i++ {
			opt := p.Filtered[i]
			isHighlighted := i == p.Cursor

			// Determine checked state
			isChecked := false
			if p.SelectedOptions["[All]"] {
				isChecked = (opt == "[All]")
			} else {
				isChecked = p.SelectedOptions[opt]
			}

			// Checkbox icon
			var checkIcon string
			if isChecked {
				checkIcon = lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("[✓] ")
			} else {
				checkIcon = lipgloss.NewStyle().Foreground(ColorMuted).Render("[ ] ")
			}

			// Cursor
			var cursor string
			if isHighlighted {
				cursor = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("▶ ")
			} else {
				cursor = "  "
			}

			var labelRendered string
			if opt == "[All]" {
				if isHighlighted {
					labelRendered = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("[All] (Show All)")
				} else if isChecked {
					labelRendered = lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render("[All] (Show All)")
				} else {
					labelRendered = lipgloss.NewStyle().Foreground(ColorMuted).Render("[All] (Show All)")
				}
			} else if p.Field == filterFieldStatus {
				labelRendered = RenderStateBadge(opt)
			} else if p.Field == filterFieldType {
				labelRendered = RenderTypeBadge(opt)
			} else { // Assignee or generic
				if isHighlighted {
					dispOpt := ScrollClippedText(opt, maxOptLen, p.MarqueeTick)
					labelRendered = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(dispOpt)
				} else {
					dispOpt := truncateString(opt, maxOptLen)
					if isChecked {
						labelRendered = lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render(dispOpt)
					} else {
						labelRendered = lipgloss.NewStyle().Foreground(ColorFg).Render(dispOpt)
					}
				}
			}

			listRows = append(listRows, cursor+checkIcon+labelRendered)
		}
	}

	var scrollHint string
	if len(p.Filtered) > p.MaxVisible {
		scrollHint = lipgloss.NewStyle().Foreground(ColorMuted).Render(
			fmt.Sprintf("▲▼ [%d/%d]", p.Cursor+1, len(p.Filtered)),
		)
	}

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("[Space] Toggle  [Enter] Confirm  [Esc] Cancel  [↑/↓] Navigate")

	var contentElements []string
	contentElements = append(contentElements, titleText, subHeader, "", searchBox, "")
	contentElements = append(contentElements, strings.Join(listRows, "\n"))
	if scrollHint != "" {
		contentElements = append(contentElements, "", scrollHint)
	}
	contentElements = append(contentElements, "", instructions)

	content := lipgloss.JoinVertical(lipgloss.Left, contentElements...)

	modalBox := lipgloss.NewStyle().
		Width(pickerWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(parentWidth, parentHeight, lipgloss.Center, lipgloss.Center, modalBox)
}

func NewModalFilter() *ModalFilter {
	ti := textinput.New()
	ti.Placeholder = "Search text, #ID, tags, description..."
	ti.CharLimit = 60
	ti.Width = 44
	ti.Prompt = "🔍 "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	return &ModalFilter{
		Op1:             "AND",
		Op2:             "AND",
		Op3:             "AND",
		SearchInput:     ti,
		Picker:          NewFilterPicker(),
		StatusOptions:   []string{"[All]"},
		AssigneeOptions: []string{"[All]"},
		TypeOptions:     []string{"[All]"},
	}
}

// TickCmd returns a timer command for animating marquee scrolling (smooth 250ms interval)
func (m *ModalFilter) TickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return MarqueeTickMsg(t)
	})
}

// Open initializes the filter modal with database options and existing filter state
func (m *ModalFilter) Open(currentFilter db.FilterOptions, database *db.DB) {
	m.OpenWithTab(currentFilter, db.FilterOptions{}, "", database)
}

// OpenWithTab initializes the filter modal with tab context and existing global/tab filter state
func (m *ModalFilter) OpenWithTab(globalFilter, tabFilter db.FilterOptions, tabTitle string, database *db.DB) {
	m.FocusIndex = filterFieldStatus
	m.ActionButtonIdx = 0
	m.MarqueeTick = 0
	m.Picker.Close()
	m.ActiveTabTitle = tabTitle

	initialFilter := globalFilter
	if tabFilter.IsActive() {
		m.TabFilterActive = true
		initialFilter = tabFilter
	} else {
		m.TabFilterActive = false
	}

	m.Op1 = "AND"
	if initialFilter.Op1 != "" {
		m.Op1 = strings.ToUpper(initialFilter.Op1)
	}
	m.Op2 = "AND"
	if initialFilter.Op2 != "" {
		m.Op2 = strings.ToUpper(initialFilter.Op2)
	}
	m.Op3 = "AND"
	if initialFilter.Op3 != "" {
		m.Op3 = strings.ToUpper(initialFilter.Op3)
	}

	// 1. Status Options
	m.StatusOptions = []string{"[All]"}
	if database != nil {
		if states, err := database.GetDistinctStates(); err == nil {
			m.StatusOptions = append(m.StatusOptions, states...)
		}
	}
	m.SelectedStatuses = initialFilter.GetEffectiveStatuses()

	// 2. Assignee Options (Search all known identities + distinct assignees)
	m.AssigneeOptions = []string{"[All]", "Unassigned"}
	if database != nil {
		seen := make(map[string]bool)
		seen["[all]"] = true
		seen["unassigned"] = true

		if identities, err := database.SearchIdentities("", 1000); err == nil && len(identities) > 0 {
			for _, id := range identities {
				label := id.DisplayName
				if id.Email != "" && !strings.Contains(label, "<") {
					label = fmt.Sprintf("%s <%s>", id.DisplayName, id.Email)
				}
				key := strings.ToLower(strings.TrimSpace(label))
				if !seen[key] {
					seen[key] = true
					m.AssigneeOptions = append(m.AssigneeOptions, label)
				}
			}
		}
		if assignees, err := database.GetDistinctAssignees(); err == nil {
			for _, a := range assignees {
				label := a.DisplayName
				if a.Email != "" && !strings.Contains(label, "<") {
					label = fmt.Sprintf("%s <%s>", a.DisplayName, a.Email)
				}
				key := strings.ToLower(strings.TrimSpace(label))
				if !seen[key] {
					seen[key] = true
					m.AssigneeOptions = append(m.AssigneeOptions, label)
				}
			}
		}
	}
	m.SelectedAssignees = initialFilter.GetEffectiveAssignees()

	// 3. Type Options
	m.TypeOptions = []string{"[All]"}
	if database != nil {
		if types, err := database.GetDistinctWorkItemTypes(); err == nil {
			m.TypeOptions = append(m.TypeOptions, types...)
		}
	}
	m.SelectedTypes = initialFilter.GetEffectiveItemTypes()

	// 4. Search input
	m.SearchInput.Reset()
	m.SearchInput.SetValue(initialFilter.SearchTerm)
	m.SearchInput.Blur()
}

// GetFilterOptions builds a db.FilterOptions from current modal state
func (m *ModalFilter) GetFilterOptions() db.FilterOptions {
	var singleStatus string
	if len(m.SelectedStatuses) == 1 {
		singleStatus = m.SelectedStatuses[0]
	}

	var singleAssignee string
	if len(m.SelectedAssignees) == 1 {
		raw := m.SelectedAssignees[0]
		if strings.Contains(raw, "<") {
			parts := strings.Split(raw, "<")
			singleAssignee = strings.TrimSpace(parts[0])
		} else {
			singleAssignee = raw
		}
	}

	var singleType string
	if len(m.SelectedTypes) == 1 {
		singleType = m.SelectedTypes[0]
	}

	return db.FilterOptions{
		Status:     singleStatus,
		Statuses:   m.SelectedStatuses,
		Op1:        m.Op1,
		Assignee:   singleAssignee,
		Assignees:  m.SelectedAssignees,
		Op2:        m.Op2,
		ItemType:   singleType,
		ItemTypes:  m.SelectedTypes,
		Op3:        m.Op3,
		SearchTerm: strings.TrimSpace(m.SearchInput.Value()),
	}
}

// Clear resets all filters
func (m *ModalFilter) Clear() {
	m.SelectedStatuses = nil
	m.SelectedAssignees = nil
	m.SelectedTypes = nil
	m.Op1 = "AND"
	m.Op2 = "AND"
	m.Op3 = "AND"
	m.MarqueeTick = 0
	m.SearchInput.Reset()
	m.Picker.Close()
}

// Update processes key events and tick messages in the filter modal
func (m *ModalFilter) Update(msg tea.Msg) (*ModalFilter, tea.Cmd, ModalFilterAction) {
	// ALWAYS process MarqueeTickMsg and schedule next tick
	if _, ok := msg.(MarqueeTickMsg); ok {
		m.MarqueeTick++
		if m.Picker.Active {
			m.Picker.MarqueeTick++
		}
		return m, m.TickCmd(), FilterActionNone
	}

	// If the selection picker sub-modal is open, delegate events to it
	if m.Picker.Active {
		selectedList, selected, closed, cmd := m.Picker.Update(msg)
		if selected {
			switch m.Picker.Field {
			case filterFieldStatus:
				m.SelectedStatuses = selectedList
			case filterFieldAssignee:
				m.SelectedAssignees = selectedList
			case filterFieldType:
				m.SelectedTypes = selectedList
			}
		}
		if closed {
			return m, nil, FilterActionNone
		}
		return m, cmd, FilterActionNone
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global modal shortcuts
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, nil, FilterActionCancel
		case "ctrl+s":
			return m, nil, FilterActionApplyGlobal
		case "ctrl+t":
			return m, nil, FilterActionApplyTab
		case "ctrl+r":
			m.Clear()
			return m, nil, FilterActionClearAll
		}

		if m.FocusIndex != filterFieldSearch {
			switch msg.String() {
			case "t", "T":
				return m, nil, FilterActionApplyTab
			case "c", "C":
				return m, nil, FilterActionClearTab
			}
		}

		// Navigation between rows
		switch msg.String() {
		case "tab", "down":
			m.MarqueeTick = 0
			m.FocusIndex = (m.FocusIndex + 1) % filterFieldCount
			if m.FocusIndex == filterFieldSearch {
				m.SearchInput.Focus()
			} else {
				m.SearchInput.Blur()
			}
			return m, nil, FilterActionNone

		case "shift+tab", "up":
			m.MarqueeTick = 0
			m.FocusIndex = (m.FocusIndex - 1 + filterFieldCount) % filterFieldCount
			if m.FocusIndex == filterFieldSearch {
				m.SearchInput.Focus()
			} else {
				m.SearchInput.Blur()
			}
			return m, nil, FilterActionNone
		}

		// Field-specific interactions
		switch m.FocusIndex {
		case filterFieldStatus:
			switch msg.String() {
			case "enter", "o", " ":
				m.Picker.Open("Select Statuses", filterFieldStatus, m.StatusOptions, m.SelectedStatuses, ColorSecondary)
				return m, nil, FilterActionNone
			}

		case filterFieldOp1:
			switch msg.String() {
			case "left", "right", " ", "enter", "h", "l":
				if m.Op1 == "AND" {
					m.Op1 = "OR"
				} else {
					m.Op1 = "AND"
				}
			}

		case filterFieldAssignee:
			switch msg.String() {
			case "enter", "o", " ":
				m.Picker.Open("Select Assignees", filterFieldAssignee, m.AssigneeOptions, m.SelectedAssignees, ColorSecondary)
				return m, nil, FilterActionNone
			}

		case filterFieldOp2:
			switch msg.String() {
			case "left", "right", " ", "enter", "h", "l":
				if m.Op2 == "AND" {
					m.Op2 = "OR"
				} else {
					m.Op2 = "AND"
				}
			}

		case filterFieldType:
			switch msg.String() {
			case "enter", "o", " ":
				m.Picker.Open("Select Work Item Types", filterFieldType, m.TypeOptions, m.SelectedTypes, ColorPurple)
				return m, nil, FilterActionNone
			}

		case filterFieldOp3:
			switch msg.String() {
			case "left", "right", " ", "enter", "h", "l":
				if m.Op3 == "AND" {
					m.Op3 = "OR"
				} else {
					m.Op3 = "AND"
				}
			}

		case filterFieldSearch:
			if msg.String() == "enter" {
				return m, nil, FilterActionApplyGlobal
			}
			var cmd tea.Cmd
			m.SearchInput, cmd = m.SearchInput.Update(msg)
			return m, cmd, FilterActionNone

		case filterFieldButtons:
			switch msg.String() {
			case "left", "h":
				m.ActionButtonIdx = (m.ActionButtonIdx - 1 + 5) % 5
			case "right", "l":
				m.ActionButtonIdx = (m.ActionButtonIdx + 1) % 5
			case "enter", " ":
				switch m.ActionButtonIdx {
				case 0:
					return m, nil, FilterActionApplyGlobal
				case 1:
					return m, nil, FilterActionApplyTab
				case 2:
					return m, nil, FilterActionClearTab
				case 3:
					m.Clear()
					return m, nil, FilterActionClearAll
				case 4:
					return m, nil, FilterActionCancel
				}
			}
		}
	}

	return m, nil, FilterActionNone
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// View renders the filter modal dialog
func (m *ModalFilter) View(width, height int) string {
	if m.Picker.Active {
		return m.Picker.View(width, height)
	}

	modalWidth := 74
	if width > 0 && width < 78 {
		modalWidth = width - 4
	}
	if modalWidth < 46 {
		modalWidth = 46
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	lblStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorSecondary)
	dimLblStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	titleLeft := titleStyle.Render("Filter Work Items")
	closeHint := lipgloss.NewStyle().Foreground(ColorSubtle).Render("[Esc] Close")
	headerGap := modalWidth - lipgloss.Width(titleLeft) - lipgloss.Width(closeHint) - 4
	if headerGap < 1 {
		headerGap = 1
	}
	header := lipgloss.JoinHorizontal(lipgloss.Center, titleLeft, strings.Repeat(" ", headerGap), closeHint)

	subHeaderTxt := "Configure criteria filters, multi-select values, and AND/OR logic"
	if m.ActiveTabTitle != "" {
		subHeaderTxt = fmt.Sprintf("Current Tab: %s  •  Apply Globally or Tab-Only", truncateString(m.ActiveTabTitle, 35))
	}
	subHeader := lipgloss.NewStyle().Foreground(ColorMuted).Render(subHeaderTxt)

	maxValLen := modalWidth - 32
	if maxValLen < 20 {
		maxValLen = 20
	}

	// Render multi-select row
	renderMultiSelectRow := func(label string, isFocused bool, selected []string, fieldType int) string {
		var valRendered string
		if len(selected) == 0 {
			if isFocused {
				valRendered = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("[All]")
			} else {
				valRendered = lipgloss.NewStyle().Foreground(ColorMuted).Render("[All]")
			}
		} else if fieldType == filterFieldStatus {
			if len(selected) <= 3 {
				var badges []string
				for _, s := range selected {
					badges = append(badges, RenderStateBadge(s))
				}
				valRendered = strings.Join(badges, " ")
			} else {
				var badges []string
				for i := 0; i < 2; i++ {
					badges = append(badges, RenderStateBadge(selected[i]))
				}
				moreCount := len(selected) - 2
				moreBadge := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("+%d more (%d)", moreCount, len(selected)))
				valRendered = strings.Join(badges, " ") + " " + moreBadge
			}
		} else if fieldType == filterFieldType {
			if len(selected) <= 3 {
				var badges []string
				for _, t := range selected {
					badges = append(badges, RenderTypeBadge(t))
				}
				valRendered = strings.Join(badges, " ")
			} else {
				var badges []string
				for i := 0; i < 2; i++ {
					badges = append(badges, RenderTypeBadge(selected[i]))
				}
				moreCount := len(selected) - 2
				moreBadge := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("+%d more (%d)", moreCount, len(selected)))
				valRendered = strings.Join(badges, " ") + " " + moreBadge
			}
		} else {
			// Assignees
			var names []string
			for _, a := range selected {
				if strings.Contains(a, "<") {
					parts := strings.Split(a, "<")
					names = append(names, strings.TrimSpace(parts[0]))
				} else {
					names = append(names, a)
				}
			}
			combined := strings.Join(names, ", ")
			if len(names) > 2 {
				combined = fmt.Sprintf("%s, %s +%d more (%d)", names[0], names[1], len(names)-2, len(names))
			}

			if isFocused {
				disp := ScrollClippedText(combined, maxValLen, m.MarqueeTick)
				valRendered = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(disp)
			} else {
				disp := truncateString(combined, maxValLen)
				valRendered = lipgloss.NewStyle().Foreground(ColorFg).Render(disp)
			}
		}

		if isFocused {
			hint := lipgloss.NewStyle().Foreground(ColorMuted).Render("  [Enter: Choose Checkboxes]")
			valRendered += hint
		}

		var labelRender string
		if isFocused {
			labelRender = lblStyle.Render("▶ " + label)
		} else {
			labelRender = dimLblStyle.Render("  " + label)
		}

		return fmt.Sprintf("%-18s %s", labelRender, valRendered)
	}

	// Render uniform AND/OR toggle
	renderToggle := func(isFocused bool, op string) string {
		var andBtn, orBtn string
		if op == "AND" {
			andBtn = lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("[AND]")
			orBtn = lipgloss.NewStyle().Foreground(ColorMuted).Render("OR")
		} else {
			andBtn = lipgloss.NewStyle().Foreground(ColorMuted).Render("AND")
			orBtn = lipgloss.NewStyle().Bold(true).Foreground(ColorPurple).Render("[OR]")
		}

		toggleContent := fmt.Sprintf("%s   %s", andBtn, orBtn)
		if isFocused {
			prefix := lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render("▶ Toggle:")
			hint := lipgloss.NewStyle().Foreground(ColorMuted).Render("  (Press Space)")
			return fmt.Sprintf("     %s  %s%s", prefix, toggleContent, hint)
		}
		return fmt.Sprintf("       Toggle:   %s", toggleContent)
	}

	// 1. Status Row
	statusRow := renderMultiSelectRow("Status:", m.FocusIndex == filterFieldStatus, m.SelectedStatuses, filterFieldStatus)

	// 2. Op1 Toggle
	op1Row := renderToggle(m.FocusIndex == filterFieldOp1, m.Op1)

	// 3. Assignee Row
	assigneeRow := renderMultiSelectRow("Assigned To:", m.FocusIndex == filterFieldAssignee, m.SelectedAssignees, filterFieldAssignee)

	// 4. Op2 Toggle
	op2Row := renderToggle(m.FocusIndex == filterFieldOp2, m.Op2)

	// 5. Type Row
	typeRow := renderMultiSelectRow("Item Type:", m.FocusIndex == filterFieldType, m.SelectedTypes, filterFieldType)

	// 6. Op3 Toggle
	op3Row := renderToggle(m.FocusIndex == filterFieldOp3, m.Op3)

	// 7. Search Term Row
	searchLabel := dimLblStyle.Render("  Search Term:")
	if m.FocusIndex == filterFieldSearch {
		searchLabel = lblStyle.Render("▶ Search Term:")
	}
	searchBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSubtle)
	if m.FocusIndex == filterFieldSearch {
		searchBox = searchBox.BorderForeground(ColorPrimary)
	}
	searchRendered := searchBox.Width(modalWidth - 6).Render(m.SearchInput.View())
	searchRow := searchLabel + "\n" + searchRendered

	// 8. Buttons Row with modern, polished styling
	isButtonsFocused := (m.FocusIndex == filterFieldButtons)

	var applyGlobalBtnStr, applyTabBtnStr, clearTabBtnStr, clearAllBtnStr, cancelBtnStr string

	if isButtonsFocused && m.ActionButtonIdx == 0 {
		applyGlobalBtnStr = lipgloss.NewStyle().
			Background(ColorGreen).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1).
			Render("🌐 Global (Ctrl+S)")
	} else {
		applyGlobalBtnStr = lipgloss.NewStyle().
			Background(ColorSubtle).
			Foreground(ColorGreen).
			Padding(0, 1).
			Render("🌐 Global")
	}

	if isButtonsFocused && m.ActionButtonIdx == 1 {
		applyTabBtnStr = lipgloss.NewStyle().
			Background(ColorCyan).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1).
			Render("📑 Tab (Ctrl+T)")
	} else {
		applyTabBtnStr = lipgloss.NewStyle().
			Background(ColorSubtle).
			Foreground(ColorCyan).
			Padding(0, 1).
			Render("📑 Tab")
	}

	if isButtonsFocused && m.ActionButtonIdx == 2 {
		clearTabBtnStr = lipgloss.NewStyle().
			Background(ColorYellow).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1).
			Render("⌫ Clear Tab (c)")
	} else {
		clearTabBtnStr = lipgloss.NewStyle().
			Background(ColorSubtle).
			Foreground(ColorYellow).
			Padding(0, 1).
			Render("⌫ Clear Tab")
	}

	if isButtonsFocused && m.ActionButtonIdx == 3 {
		clearAllBtnStr = lipgloss.NewStyle().
			Background(ColorOrange).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1).
			Render("↺ Clear All (Ctrl+R)")
	} else {
		clearAllBtnStr = lipgloss.NewStyle().
			Background(ColorSubtle).
			Foreground(ColorOrange).
			Padding(0, 1).
			Render("↺ Clear All")
	}

	if isButtonsFocused && m.ActionButtonIdx == 4 {
		cancelBtnStr = lipgloss.NewStyle().
			Background(ColorRed).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true).
			Padding(0, 1).
			Render("✖ Cancel (Esc)")
	} else {
		cancelBtnStr = lipgloss.NewStyle().
			Background(ColorSubtle).
			Foreground(ColorMuted).
			Padding(0, 1).
			Render("Cancel")
	}

	buttonsRow := lipgloss.JoinHorizontal(lipgloss.Center, applyGlobalBtnStr, " ", applyTabBtnStr, " ", clearTabBtnStr, " ", clearAllBtnStr, " ", cancelBtnStr)

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("[Tab/Shift-Tab] Move • [Ctrl+S] Global 🌐 • [Ctrl+T] Tab 📑 • [c] Clear Tab 📑 • [Ctrl+R] Clear All • [Esc] Close")

	divider := lipgloss.NewStyle().Foreground(ColorSubtle).Render(strings.Repeat("─", modalWidth-4))

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		subHeader,
		"",
		statusRow,
		"",
		op1Row,
		"",
		assigneeRow,
		"",
		op2Row,
		"",
		typeRow,
		"",
		op3Row,
		"",
		searchRow,
		"",
		divider,
		"",
		buttonsRow,
		"",
		instructions,
	)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Width(modalWidth).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
