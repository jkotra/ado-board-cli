package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModalConfirm represents a yes/no confirmation modal
type ModalConfirm struct {
	Title       string
	Message     string
	ActionTag   string
	TargetID    int
	IsDanger    bool
	Active      bool
	Confirmed   bool
	ChoiceIsYes bool
}

func NewModalConfirm() *ModalConfirm {
	return &ModalConfirm{}
}

func (m *ModalConfirm) Open(title, message, actionTag string, targetID int, isDanger bool) {
	m.Title = title
	m.Message = message
	m.ActionTag = actionTag
	m.TargetID = targetID
	m.IsDanger = isDanger
	m.ChoiceIsYes = false
	m.Confirmed = false
	m.Active = true
}

func (m *ModalConfirm) Close() {
	m.Active = false
}

func (m *ModalConfirm) Update(msg tea.Msg) (*ModalConfirm, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			m.ChoiceIsYes = true
			m.Confirmed = true
			m.Active = false
		case "n", "N", "esc":
			m.ChoiceIsYes = false
			m.Confirmed = false
			m.Active = false
		case "left", "h":
			m.ChoiceIsYes = true
		case "right", "l":
			m.ChoiceIsYes = false
		case "enter":
			m.Confirmed = m.ChoiceIsYes
			m.Active = false
		}
	}
	return m, nil
}

func (m *ModalConfirm) View(width, height int) string {
	if !m.Active {
		return ""
	}

	modalWidth := 52
	if width > 0 && width < 56 {
		modalWidth = width - 4
	}

	borderColor := ColorPrimary
	if m.IsDanger {
		borderColor = ColorRed
	}

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(borderColor).
		Render(m.Title)

	msgText := lipgloss.NewStyle().
		Foreground(ColorFg).
		Render(m.Message)

	btnYesStyle := lipgloss.NewStyle().Padding(0, 2)
	btnNoStyle := lipgloss.NewStyle().Padding(0, 2)

	if m.ChoiceIsYes {
		btnYesStyle = btnYesStyle.Bold(true).Background(ColorRed).Foreground(lipgloss.Color("#FFFFFF"))
		btnNoStyle = btnNoStyle.Foreground(ColorMuted)
	} else {
		btnYesStyle = btnYesStyle.Foreground(ColorMuted)
		btnNoStyle = btnNoStyle.Bold(true).Background(ColorPrimary).Foreground(lipgloss.Color("#FFFFFF"))
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center,
		btnYesStyle.Render("[ Yes ]"),
		"   ",
		btnNoStyle.Render("[ No ]"),
	)

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1).
		Render("[←/→] Select  [Enter] Confirm  [y/n] Quick")

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleText,
		"",
		msgText,
		"",
		buttons,
		instructions,
	)

	modalBox := lipgloss.NewStyle().
		Width(modalWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modalBox)
}
