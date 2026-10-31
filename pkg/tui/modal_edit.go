package tui

import (
	"fmt"
	"strings"

	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	fieldEditTitle = iota
	fieldEditTags
	fieldEditIteration
	fieldEditArea
	fieldEditDesc
	fieldEditTotal
)

// ModalEdit represents the edit title, tags, iteration, area, and description modal
type ModalEdit struct {
	WorkItemID          int
	WorkItemType        string
	InputTitle          textinput.Model
	InputTags           textinput.Model
	IterationPath       string
	AreaPath            string
	AvailableIterations []string
	AvailableAreas      []string
	TextareaDesc        textarea.Model
	FocusedIndex        int
	Active              bool
	DB                  *db.DB
	Autocomplete        *AutocompleteState
	Picker              *DropdownPicker
	PickerTarget        int
}

func NewModalEdit() *ModalEdit {
	tiTitle := textinput.New()
	tiTitle.Placeholder = "Work item title..."
	tiTitle.CharLimit = 250
	tiTitle.Width = 60
	tiTitle.Prompt = "❯ "
	tiTitle.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	tiTitle.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	tiTitle.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	tiTags := textinput.New()
	tiTags.Placeholder = "e.g. backend; gke; release-2.0 (semicolon-separated)"
	tiTags.CharLimit = 250
	tiTags.Width = 60
	tiTags.Prompt = "❯ "
	tiTags.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	tiTags.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	tiTags.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	ta := textarea.New()
	ta.Placeholder = "Description... Use @ for users, # for items, ! for PRs"
	ta.CharLimit = 10000
	ta.SetWidth(60)
	ta.SetHeight(4)
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(ColorFg)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(ColorMuted)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false

	return &ModalEdit{
		InputTitle:   tiTitle,
		InputTags:    tiTags,
		TextareaDesc: ta,
		Autocomplete: NewAutocompleteState(),
		Picker:       NewDropdownPicker(),
	}
}

func (m *ModalEdit) Open(id int, itemType, title, description, tags, iterationPath, areaPath string, database *db.DB) {
	m.WorkItemID = id
	m.WorkItemType = itemType
	m.DB = database
	m.InputTitle.SetValue(title)
	m.InputTags.SetValue(tags)
	m.IterationPath = iterationPath
	m.AreaPath = areaPath
	m.TextareaDesc.SetValue(StripAndFormatHTML(description))

	// Fetch distinct iterations & areas from DB
	m.AvailableIterations = nil
	m.AvailableAreas = nil
	if database != nil {
		if iters, err := database.GetDistinctIterationPaths(); err == nil {
			m.AvailableIterations = iters
		}
		if areas, err := database.GetDistinctAreaPaths(); err == nil {
			m.AvailableAreas = areas
		}
	}

	m.FocusedIndex = fieldEditTitle
	m.updateFocus()
	m.Active = true
	m.Autocomplete.Reset()
	if m.Picker != nil {
		m.Picker.Close()
	}
}

func (m *ModalEdit) Close() {
	m.Active = false
	m.InputTitle.Blur()
	m.InputTags.Blur()
	m.TextareaDesc.Blur()
	m.Autocomplete.Reset()
	if m.Picker != nil {
		m.Picker.Close()
	}
}

func (m *ModalEdit) Values() (title, description, tags, iterationPath, areaPath string) {
	return strings.TrimSpace(m.InputTitle.Value()),
		m.TextareaDesc.Value(),
		strings.TrimSpace(m.InputTags.Value()),
		strings.TrimSpace(m.IterationPath),
		strings.TrimSpace(m.AreaPath)
}

func (m *ModalEdit) Update(msg tea.Msg) (*ModalEdit, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	// Handle DropdownPicker if active
	if m.Picker != nil && m.Picker.Active {
		val, chosen, closed, cmd := m.Picker.Update(msg)
		if chosen {
			if m.PickerTarget == fieldEditIteration {
				m.IterationPath = val
			} else if m.PickerTarget == fieldEditArea {
				m.AreaPath = val
			}
		}
		if closed {
			m.Picker.Close()
		}
		return m, cmd
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
					if m.FocusedIndex == fieldEditDesc {
						curr := m.TextareaDesc.Value()
						newText := ReplaceToken(curr, m.Autocomplete.StartIndex, len(m.Autocomplete.Query), sel.InsertValue)
						m.TextareaDesc.SetValue(newText)
					}
					m.Autocomplete.Reset()
					return m, nil
				}
			case "esc":
				m.Autocomplete.Reset()
				return m, nil
			}
		}

		switch msg.String() {
		case "tab":
			m.FocusedIndex = (m.FocusedIndex + 1) % fieldEditTotal
			m.updateFocus()
			m.Autocomplete.Reset()
			return m, nil
		case "shift+tab":
			m.FocusedIndex = (m.FocusedIndex - 1 + fieldEditTotal) % fieldEditTotal
			m.updateFocus()
			m.Autocomplete.Reset()
			return m, nil
		}

		// Field-specific interactions
		switch m.FocusedIndex {
		case fieldEditIteration:
			switch msg.String() {
			case "enter", " ", "down", "o":
				m.PickerTarget = fieldEditIteration
				m.Picker.Open("⚡ Select Iteration Path", m.AvailableIterations, m.IterationPath)
				return m, nil
			case "left", "h":
				m.IterationPath = cycleOption(m.IterationPath, m.AvailableIterations, -1)
				return m, nil
			case "right", "l":
				m.IterationPath = cycleOption(m.IterationPath, m.AvailableIterations, 1)
				return m, nil
			}

		case fieldEditArea:
			switch msg.String() {
			case "enter", " ", "down", "o":
				m.PickerTarget = fieldEditArea
				m.Picker.Open("⚡ Select Area Path", m.AvailableAreas, m.AreaPath)
				return m, nil
			case "left", "h":
				m.AreaPath = cycleOption(m.AreaPath, m.AvailableAreas, -1)
				return m, nil
			case "right", "l":
				m.AreaPath = cycleOption(m.AreaPath, m.AvailableAreas, 1)
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	switch m.FocusedIndex {
	case fieldEditTitle:
		m.InputTitle, cmd = m.InputTitle.Update(msg)
	case fieldEditTags:
		m.InputTags, cmd = m.InputTags.Update(msg)
	case fieldEditDesc:
		prevVal := m.TextareaDesc.Value()
		m.TextareaDesc, cmd = m.TextareaDesc.Update(msg)
		currVal := m.TextareaDesc.Value()
		if currVal != prevVal && m.DB != nil {
			if tr, q, startIdx, found := DetectInlineTrigger(currVal, len(currVal)); found {
				m.Autocomplete.UpdateSuggestions(m.DB, tr, q, startIdx, len(currVal))
			} else {
				m.Autocomplete.Reset()
			}
		}
	}
	return m, cmd
}

func cycleOption(current string, options []string, delta int) string {
	if len(options) == 0 {
		return current
	}
	curIdx := -1
	for i, opt := range options {
		if strings.EqualFold(opt, current) {
			curIdx = i
			break
		}
	}
	if curIdx == -1 {
		if delta > 0 {
			return options[0]
		}
		return options[len(options)-1]
	}
	nextIdx := (curIdx + delta + len(options)) % len(options)
	return options[nextIdx]
}

func (m *ModalEdit) updateFocus() {
	m.InputTitle.Blur()
	m.InputTags.Blur()
	m.TextareaDesc.Blur()

	switch m.FocusedIndex {
	case fieldEditTitle:
		m.InputTitle.Focus()
	case fieldEditTags:
		m.InputTags.Focus()
	case fieldEditDesc:
		m.TextareaDesc.Focus()
	}
}

func (m *ModalEdit) View(width, height int) string {
	if !m.Active {
		return ""
	}

	if m.Picker != nil && m.Picker.Active {
		return m.Picker.View(width, height)
	}

	modalWidth := 72
	if width > 0 && width < 76 {
		modalWidth = width - 4
		m.InputTitle.Width = modalWidth - 10
		m.InputTags.Width = modalWidth - 10
		m.TextareaDesc.SetWidth(modalWidth - 10)
	}

	headerText := fmt.Sprintf("Edit Details: #%d", m.WorkItemID)
	if m.WorkItemType != "" {
		headerText += "  " + RenderTypeBadge(m.WorkItemType)
	}

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(headerText)

	labelStyle := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)

	borderTitle := ColorSubtle
	borderTags := ColorSubtle
	borderIter := ColorSubtle
	borderArea := ColorSubtle
	borderDesc := ColorSubtle

	switch m.FocusedIndex {
	case fieldEditTitle:
		borderTitle = ColorSecondary
	case fieldEditTags:
		borderTags = ColorSecondary
	case fieldEditIteration:
		borderIter = ColorSecondary
	case fieldEditArea:
		borderArea = ColorSecondary
	case fieldEditDesc:
		borderDesc = ColorSecondary
	}

	boxTitle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderTitle).
		Padding(0, 1).
		Width(modalWidth - 6).
		Render(m.InputTitle.View())

	boxTags := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderTags).
		Padding(0, 1).
		Width(modalWidth - 6).
		Render(m.InputTags.View())

	// Iteration box
	iterDisplay := m.IterationPath
	if iterDisplay == "" {
		iterDisplay = "— (none / inherit)"
	}
	iterRight := lipgloss.NewStyle().Foreground(ColorMuted).Render("[Enter ▾]")
	iterPrompt := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("❯ ")
	iterValStyle := lipgloss.NewStyle().Foreground(ColorFg)
	if m.IterationPath == "" {
		iterValStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	}
	iterLeft := iterPrompt + iterValStyle.Render(iterDisplay)
	availWidth := modalWidth - 10
	padIter := availWidth - lipgloss.Width(iterLeft) - lipgloss.Width(iterRight)
	if padIter < 1 {
		padIter = 1
	}
	iterContent := iterLeft + strings.Repeat(" ", padIter) + iterRight

	boxIter := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderIter).
		Padding(0, 1).
		Width(modalWidth - 6).
		Render(iterContent)

	// Area box
	areaDisplay := m.AreaPath
	if areaDisplay == "" {
		areaDisplay = "— (none / inherit)"
	}
	areaRight := lipgloss.NewStyle().Foreground(ColorMuted).Render("[Enter ▾]")
	areaPrompt := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true).Render("❯ ")
	areaValStyle := lipgloss.NewStyle().Foreground(ColorFg)
	if m.AreaPath == "" {
		areaValStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	}
	areaLeft := areaPrompt + areaValStyle.Render(areaDisplay)
	padArea := availWidth - lipgloss.Width(areaLeft) - lipgloss.Width(areaRight)
	if padArea < 1 {
		padArea = 1
	}
	areaContent := areaLeft + strings.Repeat(" ", padArea) + areaRight

	boxArea := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderArea).
		Padding(0, 1).
		Width(modalWidth - 6).
		Render(areaContent)

	boxDesc := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderDesc).
		Padding(0, 1).
		Width(modalWidth - 6).
		Render(m.TextareaDesc.View())

	var autoView string
	if m.Autocomplete != nil && m.Autocomplete.Active {
		autoView = "\n" + m.Autocomplete.View(modalWidth-6)
	}

	fieldTitle := lipgloss.JoinVertical(lipgloss.Left, labelStyle.Render("Title:"), boxTitle)
	fieldTags := lipgloss.JoinVertical(lipgloss.Left, labelStyle.Render("Tags:"), boxTags)
	fieldIter := lipgloss.JoinVertical(lipgloss.Left, labelStyle.Render("Iteration:"), boxIter)
	fieldArea := lipgloss.JoinVertical(lipgloss.Left, labelStyle.Render("Area Path:"), boxArea)
	fieldDesc := lipgloss.JoinVertical(lipgloss.Left, labelStyle.Render("Description:"), boxDesc)

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1).
		Render("[Tab] Next Field  [Enter] Dropdown  [←/→] Cycle  [Ctrl+S] Save  [Esc] Cancel")

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleText,
		"",
		fieldTitle,
		fieldTags,
		fieldIter,
		fieldArea,
		fieldDesc,
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
