package tui

import (
	"fmt"

	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModalAssign represents the assign to user/email modal
type ModalAssign struct {
	WorkItemID   int
	ItemTitle    string
	Input        textinput.Model
	Active       bool
	DB           *db.DB
	Autocomplete *AutocompleteState
}

func NewModalAssign() *ModalAssign {
	ti := textinput.New()
	ti.Placeholder = "e.g. user@domain.com or First Last"
	ti.CharLimit = 150
	ti.Width = 46
	ti.Prompt = "❯ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	return &ModalAssign{
		Input:        ti,
		Autocomplete: NewAutocompleteState(),
	}
}

func (m *ModalAssign) Open(id int, title, currentAssignee string, database *db.DB) {
	m.WorkItemID = id
	m.ItemTitle = title
	m.DB = database
	m.Input.SetValue(currentAssignee)
	m.Input.Focus()
	m.Active = true
	m.Autocomplete.Reset()
	if database != nil {
		m.Autocomplete.UpdateIdentitySuggestions(database, currentAssignee)
	}
}

func (m *ModalAssign) Close() {
	m.Active = false
	m.Input.Blur()
	m.Autocomplete.Reset()
}

func (m *ModalAssign) Value() string {
	return m.Input.Value()
}

func (m *ModalAssign) Update(msg tea.Msg) (*ModalAssign, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "ctrl+p":
			if m.Autocomplete.Active {
				m.Autocomplete.Prev()
				return m, nil
			}
		case "down", "ctrl+n":
			if m.Autocomplete.Active {
				m.Autocomplete.Next()
				return m, nil
			}
		case "tab":
			if m.Autocomplete.Active {
				if sel := m.Autocomplete.Selected(); sel != nil {
					m.Input.SetValue(sel.InsertValue)
					m.Autocomplete.Reset()
					return m, nil
				}
			}
		case "esc":
			if m.Autocomplete.Active {
				m.Autocomplete.Reset()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	prevVal := m.Input.Value()
	m.Input, cmd = m.Input.Update(msg)
	if m.Input.Value() != prevVal && m.DB != nil {
		m.Autocomplete.UpdateIdentitySuggestions(m.DB, m.Input.Value())
	}
	return m, cmd
}

func (m *ModalAssign) View(width, height int) string {
	if !m.Active {
		return ""
	}

	modalWidth := 60
	if width > 0 && width < 64 {
		modalWidth = width - 4
		m.Input.Width = modalWidth - 12
	}

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(fmt.Sprintf("Assign Work Item #%d", m.WorkItemID))

	itemTitle := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(m.ItemTitle)

	inputLabel := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		Render("Assignee (Email or Name):")

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSecondary).
		Padding(0, 1).
		Width(modalWidth - 6).
		Render(m.Input.View())

	var autoView string
	if m.Autocomplete != nil && m.Autocomplete.Active {
		autoView = "\n" + m.Autocomplete.View(modalWidth-6)
	}

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1).
		Render("[Tab] Insert Suggestion  •  [Enter] Assign  •  [Esc] Cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleText,
		itemTitle,
		"",
		inputLabel,
		inputBox,
		autoView,
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
