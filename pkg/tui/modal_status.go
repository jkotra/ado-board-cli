package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModalStatus represents the state picker modal
type ModalStatus struct {
	WorkItemID int
	ItemTitle  string
	ItemType   string
	States     []string
	Cursor     int
	Active     bool
}

func NewModalStatus() *ModalStatus {
	return &ModalStatus{
		States: []string{"To Do", "In Progress", "Committed", "Done", "Removed"},
	}
}

func (m *ModalStatus) Open(id int, title, itemType, currentState string, availableStates []string) {
	m.WorkItemID = id
	m.ItemTitle = title
	m.ItemType = itemType
	if len(availableStates) > 0 {
		m.States = make([]string, len(availableStates))
		copy(m.States, availableStates)
	} else {
		switch strings.ToLower(itemType) {
		case "bug", "production issue", "prod issue", "issue":
			m.States = []string{"New", "Approved", "Committed", "Done", "Removed"}
		case "user story", "product backlog item", "pbi":
			m.States = []string{"New", "Approved", "Committed", "Done", "Removed"}
		case "feature", "epic", "initiative":
			m.States = []string{"New", "In Progress", "Done", "Removed"}
		default:
			m.States = []string{"To Do", "In Progress", "Done", "Removed"}
		}
	}

	// Ensure currentState is present in list if non-empty
	if currentState != "" {
		found := false
		for _, s := range m.States {
			if strings.EqualFold(s, currentState) {
				found = true
				break
			}
		}
		if !found {
			m.States = append([]string{currentState}, m.States...)
		}
	}

	m.Cursor = 0
	for i, s := range m.States {
		if strings.EqualFold(s, currentState) {
			m.Cursor = i
			break
		}
	}
	m.Active = true
}

func (m *ModalStatus) Close() {
	m.Active = false
}

func (m *ModalStatus) SelectedState() string {
	if m.Cursor >= 0 && m.Cursor < len(m.States) {
		return m.States[m.Cursor]
	}
	return ""
}

func (m *ModalStatus) Update(msg tea.Msg) (*ModalStatus, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.Cursor > 0 {
				m.Cursor--
			}
		case "down", "j":
			if m.Cursor < len(m.States)-1 {
				m.Cursor++
			}
		}
	}
	return m, nil
}

func (m *ModalStatus) View(width, height int) string {
	if !m.Active {
		return ""
	}

	modalWidth := 50
	if width > 0 && width < 54 {
		modalWidth = width - 4
	}

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(fmt.Sprintf("Change Status: #%d", m.WorkItemID))

	itemTitle := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(m.ItemTitle)

	var rows []string
	for i, s := range m.States {
		badge := RenderStateBadge(s)
		if i == m.Cursor {
			cursor := lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("❯ ")
			rows = append(rows, cursor+badge)
		} else {
			rows = append(rows, "  "+badge)
		}
	}

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1).
		Render("[↑/↓] Select  [Enter] Confirm  [Esc] Cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleText,
		itemTitle,
		"",
		strings.Join(rows, "\n\n"),
		instructions,
	)

	modalBox := lipgloss.NewStyle().
		Width(modalWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modalBox)
}
