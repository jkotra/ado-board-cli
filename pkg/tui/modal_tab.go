package tui

import (
	"fmt"
	"strings"

	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// ModalTabAction describes the result of interacting with the tab modal
type ModalTabAction int

const (
	TabActionNone ModalTabAction = iota
	TabActionSelect
	TabActionRename
	TabActionClose
)

// TabItem represents an item in the Tab Selection modal
type TabItem struct {
	Index        int
	RootID       int
	Title        string
	WorkItemType string
	State        string
	Assignee     string
	Active       bool
}

// ModalTab represents the modal dialog to select, switch between, and rename root tabs
type ModalTab struct {
	Items        []TabItem
	Filtered     []TabItem
	Cursor       int
	ScrollOffset int
	SearchInput  textinput.Model
	RenameInput  textinput.Model
	Renaming     bool
	Active       bool
}

// NewModalTab creates a new ModalTab instance
func NewModalTab() *ModalTab {
	ti := textinput.New()
	ti.Placeholder = "Search tabs by title, ID (#123), type, or status..."
	ti.CharLimit = 100
	ti.Width = 52
	ti.Prompt = "🔍 "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	ri := textinput.New()
	ri.Placeholder = "Enter new tab name..."
	ri.CharLimit = 80
	ri.Width = 52
	ri.Prompt = "✏️  "
	ri.PromptStyle = lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)
	ri.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ri.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	return &ModalTab{
		SearchInput: ti,
		RenameInput: ri,
	}
}

// Open initializes the modal with current roots and focuses on the active tab
func (m *ModalTab) Open(roots []*db.TreeNode, activeTabIdx int) {
	m.Renaming = false
	m.Items = make([]TabItem, 0, len(roots))
	for i, root := range roots {
		if root == nil || root.Record == nil {
			continue
		}
		m.Items = append(m.Items, TabItem{
			Index:        i,
			RootID:       root.Record.ID,
			Title:        root.TabTitle(),
			WorkItemType: root.Record.WorkItemType,
			State:        root.Record.State,
			Assignee:     root.Record.AssignedToName,
			Active:       (i == activeTabIdx),
		})
	}

	m.SearchInput.Reset()
	m.SearchInput.Focus()
	m.rebuildFiltered()

	// Position cursor on current active tab
	m.Cursor = 0
	for i, item := range m.Filtered {
		if item.Active {
			m.Cursor = i
			break
		}
	}
	m.ScrollOffset = 0
	m.adjustScroll(7)
	m.Active = true
}

// Close hides the modal and blurs input
func (m *ModalTab) Close() {
	m.Active = false
	m.Renaming = false
	m.SearchInput.Blur()
	m.RenameInput.Blur()
}

// UpdateTabTitle updates the title of a tab across Items and Filtered lists
func (m *ModalTab) UpdateTabTitle(index int, rootID int, newTitle string) {
	for i := range m.Items {
		if m.Items[i].Index == index || (rootID > 0 && m.Items[i].RootID == rootID) {
			m.Items[i].Title = newTitle
		}
	}
	for i := range m.Filtered {
		if m.Filtered[i].Index == index || (rootID > 0 && m.Filtered[i].RootID == rootID) {
			m.Filtered[i].Title = newTitle
		}
	}
}

// SelectedTab returns the currently highlighted tab item, or nil if none
func (m *ModalTab) SelectedTab() *TabItem {
	if len(m.Filtered) > 0 && m.Cursor >= 0 && m.Cursor < len(m.Filtered) {
		return &m.Filtered[m.Cursor]
	}
	return nil
}

// CursorUp moves the selection cursor up
func (m *ModalTab) CursorUp() {
	if m.Cursor > 0 {
		m.Cursor--
		m.adjustScroll(7)
	}
}

// CursorDown moves the selection cursor down
func (m *ModalTab) CursorDown() {
	if m.Cursor < len(m.Filtered)-1 {
		m.Cursor++
		m.adjustScroll(7)
	}
}

// PageUp moves up by page
func (m *ModalTab) PageUp() {
	m.Cursor -= 5
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	m.adjustScroll(7)
}

// PageDown moves down by page
func (m *ModalTab) PageDown() {
	m.Cursor += 5
	if m.Cursor >= len(m.Filtered) {
		m.Cursor = len(m.Filtered) - 1
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	m.adjustScroll(7)
}

func (m *ModalTab) adjustScroll(visibleRows int) {
	if visibleRows <= 0 {
		visibleRows = 7
	}
	if m.Cursor < m.ScrollOffset {
		m.ScrollOffset = m.Cursor
	} else if m.Cursor >= m.ScrollOffset+visibleRows {
		m.ScrollOffset = m.Cursor - visibleRows + 1
	}

	maxScroll := len(m.Filtered) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.ScrollOffset > maxScroll {
		m.ScrollOffset = maxScroll
	}
	if m.ScrollOffset < 0 {
		m.ScrollOffset = 0
	}
}

func (m *ModalTab) rebuildFiltered() {
	query := strings.ToLower(strings.TrimSpace(m.SearchInput.Value()))
	if query == "" {
		m.Filtered = make([]TabItem, len(m.Items))
		copy(m.Filtered, m.Items)
	} else {
		m.Filtered = nil
		for _, item := range m.Items {
			idStr := fmt.Sprintf("%d", item.RootID)
			idxStr := fmt.Sprintf("%d", item.Index+1)
			if strings.Contains(strings.ToLower(item.Title), query) ||
				strings.Contains(idStr, query) ||
				strings.Contains(idxStr, query) ||
				strings.Contains(strings.ToLower(item.WorkItemType), query) ||
				strings.Contains(strings.ToLower(item.State), query) ||
				strings.Contains(strings.ToLower(item.Assignee), query) {
				m.Filtered = append(m.Filtered, item)
			}
		}
	}

	if m.Cursor >= len(m.Filtered) {
		m.Cursor = len(m.Filtered) - 1
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
}

// Update handles input messages for the Tab modal
func (m *ModalTab) Update(msg tea.Msg) (*ModalTab, tea.Cmd, ModalTabAction) {
	if !m.Active {
		return m, nil, TabActionNone
	}

	if m.Renaming {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.Renaming = false
				m.SearchInput.Focus()
				return m, nil, TabActionNone
			case "enter":
				m.Renaming = false
				m.SearchInput.Focus()
				return m, nil, TabActionRename
			}
		}
		var cmd tea.Cmd
		m.RenameInput, cmd = m.RenameInput.Update(msg)
		return m, cmd, TabActionNone
	}

	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if msg.Button == tea.MouseButtonWheelUp || msg.Type == tea.MouseWheelUp {
			m.CursorUp()
			return m, nil, TabActionNone
		} else if msg.Button == tea.MouseButtonWheelDown || msg.Type == tea.MouseWheelDown {
			m.CursorDown()
			return m, nil, TabActionNone
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.Close()
			return m, nil, TabActionClose

		case "enter":
			return m, nil, TabActionSelect

		case "f2", "ctrl+r", "ctrl+e":
			if sel := m.SelectedTab(); sel != nil {
				m.Renaming = true
				m.RenameInput.SetValue(sel.Title)
				m.RenameInput.Focus()
				m.SearchInput.Blur()
				return m, nil, TabActionNone
			}

		case "r", "e":
			if m.SearchInput.Value() == "" {
				if sel := m.SelectedTab(); sel != nil {
					m.Renaming = true
					m.RenameInput.SetValue(sel.Title)
					m.RenameInput.Focus()
					m.SearchInput.Blur()
					return m, nil, TabActionNone
				}
			}

		case "up", "ctrl+p":
			m.CursorUp()
			return m, nil, TabActionNone

		case "down", "ctrl+n":
			m.CursorDown()
			return m, nil, TabActionNone

		case "pgup":
			m.PageUp()
			return m, nil, TabActionNone

		case "pgdown":
			m.PageDown()
			return m, nil, TabActionNone
		}
	}

	oldVal := m.SearchInput.Value()
	m.SearchInput, cmd = m.SearchInput.Update(msg)
	if m.SearchInput.Value() != oldVal {
		m.rebuildFiltered()
		m.adjustScroll(7)
	}

	return m, cmd, TabActionNone
}

// View renders the Tab Picker modal
func (m *ModalTab) View(width, height int) string {
	if !m.Active {
		return ""
	}

	modalWidth := 74
	if width > 0 && width < 78 {
		modalWidth = width - 4
	}
	m.SearchInput.Width = modalWidth - 10

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render("📑 Select Root Tab")

	desc := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("Choose a tracked work item tree tab to display on screen:")

	if m.Renaming {
		sel := m.SelectedTab()
		tabInfo := ""
		if sel != nil {
			if sel.WorkItemType == "Query" {
				tabInfo = "Query Tab"
			} else {
				tabInfo = fmt.Sprintf("Tab %d: #%d (%s)", sel.Index+1, sel.RootID, sel.WorkItemType)
			}
		}

		renameHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorYellow).
			Render("✏️  Rename Tab: " + tabInfo)

		renameDesc := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Render("Enter a new display name for this tab:")

		m.RenameInput.Width = modalWidth - 10
		renameBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorYellow).
			Padding(0, 1).
			Width(modalWidth - 6).
			Render(m.RenameInput.View())

		renameHints := lipgloss.NewStyle().
			Foreground(ColorMuted).
			MarginTop(1).
			Render("[Enter] Save  •  [Esc] Cancel")

		var renameSections []string
		renameSections = append(renameSections, titleText, desc, "", renameHeader, renameDesc, renameBox, renameHints)

		content := strings.Join(renameSections, "\n")
		modalBox := lipgloss.NewStyle().
			Width(modalWidth).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2).
			Render(content)

		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modalBox)
	}

	headerLine := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSecondary).
		Render(fmt.Sprintf("Tracked Root Tabs (%d):", len(m.Filtered)))

	// Search box
	searchBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSecondary).
		Padding(0, 1).
		Width(modalWidth - 6).
		Render(m.SearchInput.View())

	visibleRows := 7
	if height > 30 {
		visibleRows = 10
	} else if height < 20 {
		visibleRows = 5
	}
	m.adjustScroll(visibleRows)

	var listRows []string
	if len(m.Filtered) == 0 {
		listRows = append(listRows, lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true).
			Render("  (No matching root tabs found)"))
	} else {
		endIdx := m.ScrollOffset + visibleRows
		if endIdx > len(m.Filtered) {
			endIdx = len(m.Filtered)
		}

		for i := m.ScrollOffset; i < endIdx; i++ {
			item := m.Filtered[i]
			isSelected := (i == m.Cursor)
			typeColor := GetTypeColor(item.WorkItemType)

			title := item.Title
			if title == "" {
				title = "(Tree Root)"
			}

			maxTitle := modalWidth - 38
			if maxTitle < 12 {
				maxTitle = 12
			}
			title = truncateString(title, maxTitle)

			numStr := fmt.Sprintf("%d:", item.Index+1)
			var idStr string
			if item.WorkItemType != "Query" {
				idStr = fmt.Sprintf("#%d", item.RootID)
			}
			shortType := FormatItemType(item.WorkItemType)

			var typeBadge string
			if item.WorkItemType != "" {
				typeBadge = lipgloss.NewStyle().Foreground(typeColor).Bold(true).Render("[" + shortType + "]")
			}

			var activeBadge string
			if item.Active {
				activeBadge = " " + lipgloss.NewStyle().Bold(true).Foreground(ColorGreen).Render("[Active]")
			}

			var stateBadge string
			if item.State != "" && item.WorkItemType != "Query" {
				stateBadge = " " + RenderStateBadge(item.State)
			}

			var rowContent string
			if isSelected {
				pointer := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("❯ ")
				numPart := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(numStr)
				titlePart := lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render(title)

				if idStr != "" {
					idPart := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(idStr)
					rowContent = fmt.Sprintf("%s%s %s %s %s%s%s", pointer, numPart, idPart, typeBadge, titlePart, stateBadge, activeBadge)
				} else {
					rowContent = fmt.Sprintf("%s%s %s %s%s%s", pointer, numPart, typeBadge, titlePart, stateBadge, activeBadge)
				}
			} else {
				pointer := "  "
				numPart := lipgloss.NewStyle().Foreground(ColorRibbonKey).Render(numStr)
				titlePart := lipgloss.NewStyle().Foreground(ColorMuted).Render(title)

				if idStr != "" {
					idPart := lipgloss.NewStyle().Foreground(ColorMuted).Render(idStr)
					rowContent = fmt.Sprintf("%s%s %s %s %s%s%s", pointer, numPart, idPart, typeBadge, titlePart, stateBadge, activeBadge)
				} else {
					rowContent = fmt.Sprintf("%s%s %s %s%s%s", pointer, numPart, typeBadge, titlePart, stateBadge, activeBadge)
				}
			}

			listRows = append(listRows, rowContent)
		}
	}

	// Scroll indicator hints
	var scrollHintTop string
	var scrollHintBottom string
	if m.ScrollOffset > 0 {
		scrollHintTop = lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render(fmt.Sprintf("  ▲ %d more tabs above...", m.ScrollOffset))
	}
	if m.ScrollOffset+visibleRows < len(m.Filtered) {
		remaining := len(m.Filtered) - (m.ScrollOffset + visibleRows)
		scrollHintBottom = lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render(fmt.Sprintf("  ▼ %d more tabs below...", remaining))
	}

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1).
		Render("[↑/↓/Mouse] Select  •  [Enter] Switch  •  [F2/Ctrl+R/r] Rename  •  [Esc] Cancel")

	var sections []string
	sections = append(sections, titleText, desc, "", searchBox, "", headerLine)
	if scrollHintTop != "" {
		sections = append(sections, scrollHintTop)
	}
	sections = append(sections, strings.Join(listRows, "\n"))
	if scrollHintBottom != "" {
		sections = append(sections, scrollHintBottom)
	}
	sections = append(sections, instructions)

	content := strings.Join(sections, "\n")

	modalBox := lipgloss.NewStyle().
		Width(modalWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modalBox)
}

// Ensure unused import safety
var _ = runewidth.StringWidth
