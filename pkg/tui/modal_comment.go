package tui

import (
	"fmt"

	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModalComment represents the add comment dialog
type ModalComment struct {
	WorkItemID   int
	ItemTitle    string
	Textarea     textarea.Model
	Active       bool
	DB           *db.DB
	Autocomplete *AutocompleteState
}

func NewModalComment() *ModalComment {
	ta := textarea.New()
	ta.Placeholder = "Write comment... Use @ for users, # for items, ! for PRs"
	ta.CharLimit = 4000
	ta.SetWidth(56)
	ta.SetHeight(5)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(ColorFg)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(ColorMuted)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false
	return &ModalComment{
		Textarea:     ta,
		Autocomplete: NewAutocompleteState(),
	}
}

func (m *ModalComment) Open(id int, title string, database *db.DB) {
	m.WorkItemID = id
	m.ItemTitle = title
	m.DB = database
	m.Textarea.Reset()
	m.Textarea.Focus()
	m.Active = true
	m.Autocomplete.Reset()
}

func (m *ModalComment) Close() {
	m.Active = false
	m.Textarea.Blur()
	m.Autocomplete.Reset()
}

func (m *ModalComment) Value() string {
	return m.Textarea.Value()
}

func (m *ModalComment) Update(msg tea.Msg) (*ModalComment, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.Autocomplete.Active {
			switch msg.String() {
			case "up", "ctrl+p":
				m.Autocomplete.Prev()
				return m, nil
			case "down", "ctrl+n":
				m.Autocomplete.Next()
				return m, nil
			case "tab", "enter":
				if sel := m.Autocomplete.Selected(); sel != nil {
					curr := m.Textarea.Value()
					newText := ReplaceToken(curr, m.Autocomplete.StartIndex, len(m.Autocomplete.Query), sel.InsertValue)
					m.Textarea.SetValue(newText)
					m.Autocomplete.Reset()
					return m, nil
				}
			case "esc":
				m.Autocomplete.Reset()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	prevVal := m.Textarea.Value()
	m.Textarea, cmd = m.Textarea.Update(msg)

	currVal := m.Textarea.Value()
	if currVal != prevVal && m.DB != nil {
		if tr, q, startIdx, found := DetectInlineTrigger(currVal, len(currVal)); found {
			m.Autocomplete.UpdateSuggestions(m.DB, tr, q, startIdx, len(currVal))
		} else {
			m.Autocomplete.Reset()
		}
	}

	return m, cmd
}

func (m *ModalComment) View(width, height int) string {
	if !m.Active {
		return ""
	}

	modalWidth := 64
	if width > 0 && width < 68 {
		modalWidth = width - 4
		m.Textarea.SetWidth(modalWidth - 8)
	}

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(fmt.Sprintf("Add Comment to #%d", m.WorkItemID))

	itemTitle := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(m.ItemTitle)

	var autoView string
	if m.Autocomplete != nil && m.Autocomplete.Active {
		autoView = "\n" + m.Autocomplete.View(modalWidth-6)
	}

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1).
		Render("[Tab] Complete  [@ # !] Mentions  [Ctrl+S] Submit  [Esc] Cancel")

	taBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSecondary).
		Padding(0, 1).
		Width(modalWidth - 6).
		Render(m.Textarea.View())

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleText,
		itemTitle,
		"",
		taBox,
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
