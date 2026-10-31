package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StandardActivityOptions lists the default standard Azure DevOps activity options
var StandardActivityOptions = []string{
	"Development",
	"Testing",
	"Design",
	"Deployment",
	"Documentation",
	"Requirements",
}

// ModalActivity represents the Activity selection modal
type ModalActivity struct {
	WorkItemID      int
	ItemTitle       string
	ItemType        string
	CurrentActivity string
	Activities      []string
	Cursor          int
	Active          bool
}

func NewModalActivity() *ModalActivity {
	return &ModalActivity{
		Activities: append([]string{"(Clear / None)"}, StandardActivityOptions...),
	}
}

func (m *ModalActivity) Open(id int, title, itemType, currentActivity string, dbActivities []string) {
	m.WorkItemID = id
	m.ItemTitle = title
	m.ItemType = itemType
	m.CurrentActivity = strings.TrimSpace(currentActivity)

	// Build options starting with Clear / None, then standard options
	m.Activities = []string{"(Clear / None)"}
	seen := make(map[string]bool)
	seen["(clear / none)"] = true
	seen[""] = true

	for _, opt := range StandardActivityOptions {
		m.Activities = append(m.Activities, opt)
		seen[strings.ToLower(opt)] = true
	}

	// Add any distinct custom activities passed from DB
	for _, act := range dbActivities {
		clean := strings.TrimSpace(act)
		if clean != "" && !seen[strings.ToLower(clean)] {
			m.Activities = append(m.Activities, clean)
			seen[strings.ToLower(clean)] = true
		}
	}

	// Ensure currentActivity is present in list if non-empty
	if m.CurrentActivity != "" && !seen[strings.ToLower(m.CurrentActivity)] {
		m.Activities = append(m.Activities, m.CurrentActivity)
	}

	m.Cursor = 0
	if m.CurrentActivity != "" {
		for i, act := range m.Activities {
			if strings.EqualFold(act, m.CurrentActivity) {
				m.Cursor = i
				break
			}
		}
	}
	m.Active = true
}

func (m *ModalActivity) Close() {
	m.Active = false
}

func (m *ModalActivity) SelectedActivity() string {
	if m.Cursor <= 0 || m.Cursor >= len(m.Activities) {
		return ""
	}
	val := m.Activities[m.Cursor]
	if strings.HasPrefix(val, "(") {
		return ""
	}
	return val
}

func (m *ModalActivity) Update(msg tea.Msg) (*ModalActivity, tea.Cmd) {
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
			if m.Cursor < len(m.Activities)-1 {
				m.Cursor++
			}
		}
	}
	return m, nil
}

func (m *ModalActivity) View(width, height int) string {
	if !m.Active {
		return ""
	}

	modalWidth := 52
	if width > 0 && width < 56 {
		modalWidth = width - 4
	}

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(fmt.Sprintf("Select Activity: #%d", m.WorkItemID))

	itemTitle := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(m.ItemTitle)

	var rows []string
	for i, act := range m.Activities {
		var badge string
		if i == 0 {
			badge = lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render(act)
		} else {
			style := lipgloss.NewStyle().Bold(true)
			switch strings.ToLower(act) {
			case "development":
				style = style.Foreground(ColorCyan)
			case "testing":
				style = style.Foreground(ColorGreen)
			case "design":
				style = style.Foreground(ColorPurple)
			case "deployment":
				style = style.Foreground(ColorYellow)
			case "documentation":
				style = style.Foreground(ColorSecondary)
			case "requirements":
				style = style.Foreground(ColorOrange)
			default:
				style = style.Foreground(ColorFg)
			}
			badge = style.Render(act)
		}

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
		strings.Join(rows, "\n"),
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
