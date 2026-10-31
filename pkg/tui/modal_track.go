package tui

import (
	"fmt"
	"strings"

	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModalTrack represents the Track Menu for adding, viewing, and untracking queries & root work items
type ModalTrack struct {
	DB        *db.DB
	Queries   []db.TrackedQuery
	Items     []db.TrackedRootDetail
	Cursor    int
	Input     textinput.Model
	InputMode bool
	Active    bool
}

// NewModalTrack creates a new ModalTrack
func NewModalTrack() *ModalTrack {
	ti := textinput.New()
	ti.Placeholder = "Paste ADO Query URL or Work Item URL/ID..."
	ti.CharLimit = 300
	ti.Width = 60
	ti.Prompt = "🔗 "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	return &ModalTrack{
		Input: ti,
	}
}

// Open initializes and displays the Track Menu
func (m *ModalTrack) Open(database *db.DB) {
	m.DB = database
	m.Active = true
	m.RefreshList()

	if m.TotalCount() == 0 {
		m.InputMode = true
		m.Input.Reset()
		m.Input.Focus()
	} else {
		m.InputMode = false
		m.Cursor = 0
		m.Input.Reset()
		m.Input.Blur()
	}
}

// Close hides the modal and resets focus
func (m *ModalTrack) Close() {
	m.Active = false
	m.Input.Blur()
}

// TotalCount returns the total number of selectable queries and items
func (m *ModalTrack) TotalCount() int {
	return len(m.Queries) + len(m.Items)
}

// RefreshList reloads all tracked queries and roots from the database
func (m *ModalTrack) RefreshList() {
	if m.DB == nil {
		m.Queries = nil
		m.Items = nil
		return
	}
	queries, err := m.DB.GetTrackedQueries()
	if err == nil {
		m.Queries = queries
	} else {
		m.Queries = nil
	}

	items, err := m.DB.GetStandaloneTrackedRootsWithDetails()
	if err == nil {
		m.Items = items
	} else {
		m.Items = nil
	}

	total := m.TotalCount()
	if m.Cursor >= total {
		m.Cursor = total - 1
	}
	if m.Cursor < 0 {
		m.Cursor = 0
	}
}

// SelectedQuery returns the highlighted tracked query if selected, or nil
func (m *ModalTrack) SelectedQuery() *db.TrackedQuery {
	if m.Cursor >= 0 && m.Cursor < len(m.Queries) {
		return &m.Queries[m.Cursor]
	}
	return nil
}

// SelectedItem returns the highlighted tracked root item if selected, or nil
func (m *ModalTrack) SelectedItem() *db.TrackedRootDetail {
	itemIdx := m.Cursor - len(m.Queries)
	if itemIdx >= 0 && itemIdx < len(m.Items) {
		return &m.Items[itemIdx]
	}
	return nil
}

// Value returns the add input value
func (m *ModalTrack) Value() string {
	return m.Input.Value()
}

// Update handles tea messages for the track menu
func (m *ModalTrack) Update(msg tea.Msg) (*ModalTrack, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.InputMode {
			switch msg.String() {
			case "esc", "tab":
				if m.TotalCount() > 0 {
					m.InputMode = false
					m.Input.Blur()
					return m, nil
				}
				m.Close()
				return m, nil

			case "up":
				if m.Input.Value() == "" && m.TotalCount() > 0 {
					m.InputMode = false
					m.Input.Blur()
					m.Cursor = m.TotalCount() - 1
					return m, nil
				}
			}
		} else {
			// List Mode navigation
			switch msg.String() {
			case "esc":
				m.Close()
				return m, nil

			case "up", "k", "ctrl+p":
				if m.Cursor > 0 {
					m.Cursor--
				}
				return m, nil

			case "down", "j", "ctrl+n":
				if m.Cursor < m.TotalCount()-1 {
					m.Cursor++
				}
				return m, nil

			case "tab", "a", "i", "+", "/":
				m.InputMode = true
				m.Input.Reset()
				m.Input.Focus()
				return m, nil

			case "d", "x", "delete", "backspace":
				if q := m.SelectedQuery(); q != nil && m.DB != nil {
					_ = m.DB.DeleteTrackedQuery(q.ID)
					m.RefreshList()
					if m.TotalCount() == 0 {
						m.InputMode = true
						m.Input.Reset()
						m.Input.Focus()
					}
				} else if sel := m.SelectedItem(); sel != nil && m.DB != nil {
					_ = m.DB.DeleteTrackedRoot(sel.ID)
					m.RefreshList()
					if m.TotalCount() == 0 {
						m.InputMode = true
						m.Input.Reset()
						m.Input.Focus()
					}
				}
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	if m.InputMode {
		m.Input, cmd = m.Input.Update(msg)
	}
	return m, cmd
}

// View renders the Track Menu
func (m *ModalTrack) View(width, height int) string {
	if !m.Active {
		return ""
	}

	modalWidth := 74
	if width > 0 && width < 78 {
		modalWidth = width - 4
	}
	m.Input.Width = modalWidth - 10

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render("Tracked Queries & Items Management")

	desc := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("Manage tracked queries, work item trees, or add new Azure DevOps items:")

	var sections []string
	sections = append(sections, titleText, desc, "")

	// 1. Render Tracked Queries (if any)
	if len(m.Queries) > 0 {
		queryHeader := lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary).
			Render(fmt.Sprintf("Tracked Queries (%d):", len(m.Queries)))
		sections = append(sections, queryHeader)

		for i, q := range m.Queries {
			isSelected := (!m.InputMode && i == m.Cursor)
			name := q.Name
			if name == "" {
				name = "Saved Query"
			}

			maxNameLen := modalWidth - 30
			if maxNameLen < 15 {
				maxNameLen = 15
			}
			if len(name) > maxNameLen {
				name = name[:maxNameLen-3] + "..."
			}

			countStr := fmt.Sprintf("(%d items)", q.ItemCount)
			projStr := ""
			if q.Project != "" {
				projStr = fmt.Sprintf("(%s)", q.Project)
			}

			var rowContent string
			if isSelected {
				pointer := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("❯ ")
				icon := lipgloss.NewStyle().Bold(true).Foreground(ColorCyan).Render("🔍")
				nameStr := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(name)
				countBadge := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render(countStr)
				pStr := lipgloss.NewStyle().Foreground(ColorMuted).Render(projStr)
				rowContent = fmt.Sprintf("%s%s %s %s %s", pointer, icon, nameStr, countBadge, pStr)
			} else {
				pointer := "  "
				icon := lipgloss.NewStyle().Foreground(ColorCyan).Render("🔍")
				nameStr := lipgloss.NewStyle().Foreground(ColorFg).Render(name)
				countBadge := lipgloss.NewStyle().Foreground(ColorMuted).Render(countStr)
				pStr := lipgloss.NewStyle().Foreground(ColorSubtle).Render(projStr)
				rowContent = fmt.Sprintf("%s%s %s %s %s", pointer, icon, nameStr, countBadge, pStr)
			}
			sections = append(sections, rowContent)
		}
		sections = append(sections, "")
	}

	// 2. Render Tracked Items List
	listHeader := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSecondary).
		Render(fmt.Sprintf("Tracked Root Items (%d):", len(m.Items)))
	sections = append(sections, listHeader)

	if len(m.Items) == 0 && len(m.Queries) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true).
			Render("  (No queries or work items currently tracked. Add a URL or ID below to begin!)")
		sections = append(sections, emptyMsg)
	} else if len(m.Items) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true).
			Render("  (No standalone work items tracked)")
		sections = append(sections, emptyMsg)
	} else {
		for i, item := range m.Items {
			globalIdx := len(m.Queries) + i
			isSelected := (!m.InputMode && globalIdx == m.Cursor)
			typeColor := GetTypeColor(item.WorkItemType)

			title := item.Title
			if title == "" {
				title = "(Tree Root)"
			}

			maxTitleLen := modalWidth - 35
			if maxTitleLen < 15 {
				maxTitleLen = 15
			}
			if len(title) > maxTitleLen {
				title = title[:maxTitleLen-3] + "..."
			}

			var rowContent string
			if isSelected {
				pointer := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("❯ ")
				idStr := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render(fmt.Sprintf("#%d", item.ID))
				var typeBadge string
				if item.WorkItemType != "" {
					typeBadge = lipgloss.NewStyle().Foreground(typeColor).Bold(true).Render("[" + item.WorkItemType + "]")
				}
				titleStr := lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render(title)
				projStr := ""
				if item.Project != "" {
					projStr = lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("(%s)", item.Project))
				}
				rowContent = fmt.Sprintf("%s%s %s %s %s", pointer, idStr, typeBadge, titleStr, projStr)
			} else {
				pointer := "  "
				idStr := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("#%d", item.ID))
				var typeBadge string
				if item.WorkItemType != "" {
					typeBadge = lipgloss.NewStyle().Foreground(typeColor).Render("[" + item.WorkItemType + "]")
				}
				titleStr := lipgloss.NewStyle().Foreground(ColorMuted).Render(title)
				projStr := ""
				if item.Project != "" {
					projStr = lipgloss.NewStyle().Foreground(ColorSubtle).Render(fmt.Sprintf("(%s)", item.Project))
				}
				rowContent = fmt.Sprintf("%s%s %s %s %s", pointer, idStr, typeBadge, titleStr, projStr)
			}

			sections = append(sections, rowContent)
		}
	}

	sections = append(sections, "")

	// 3. Render Add Section
	addHeaderStyle := lipgloss.NewStyle().Bold(true)
	if m.InputMode {
		addHeaderStyle = addHeaderStyle.Foreground(ColorYellow)
	} else {
		addHeaderStyle = addHeaderStyle.Foreground(ColorMuted)
	}
	sections = append(sections, addHeaderStyle.Render("➕ Add Query or Item to Track (URL, Query ID, or Item ID):"))

	inputBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(modalWidth - 6)

	if m.InputMode {
		inputBoxStyle = inputBoxStyle.BorderForeground(ColorYellow)
	} else {
		inputBoxStyle = inputBoxStyle.BorderForeground(ColorSubtle)
	}

	inputBox := inputBoxStyle.Render(m.Input.View())
	sections = append(sections, inputBox)

	// 4. Instructions Footer
	var instructions string
	if m.InputMode {
		instructions = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Render("[Enter] Fetch & Track  [Tab / Esc] Back to List")
	} else {
		instructions = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Render("[↑/↓] Select  [d/x] Untrack  [a/Tab] Add New  [Enter] Jump to  [Esc] Close")
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
