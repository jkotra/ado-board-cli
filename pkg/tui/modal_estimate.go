package tui

import (
	"fmt"
	"strconv"
	"strings"

	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ModalEstimate represents the estimate and work editor modal
type ModalEstimate struct {
	WorkItemID           int
	ItemTitle            string
	ItemType             string
	ItemState            string
	Org                  string
	Project              string
	ShowEffort           bool
	ShowOriginal         bool
	ShowRemaining        bool
	ShowCompleted        bool
	FieldEffort          string
	FieldOriginal        string
	FieldRemaining       string
	FieldCompleted       string
	VisibleFields        []string // "effort", "original", "remaining", "completed"
	InputEffort          textinput.Model
	InputOriginal        textinput.Model
	InputRemaining       textinput.Model
	InputCompleted       textinput.Model
	FocusedIndex         int // index into VisibleFields
	Active               bool
	AutoCalcRemaining    bool
	EffortSameAsEstimate bool
	effortCustomized     bool
	effortModified       bool
	originalModified     bool
	remainingModified    bool
	completedModified    bool
	InitialOriginal      float64
	InitialRemaining     float64
	InitialCompleted     float64
	InitialEffort        float64
	DB                   *db.DB
}

func setupEstimateInput(placeholder string) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = 10
	ti.Width = 30
	ti.Prompt = "❯ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	return ti
}

func NewModalEstimate() *ModalEstimate {
	return &ModalEstimate{
		InputEffort:          setupEstimateInput("Effort / Story Points (e.g. 5)"),
		InputOriginal:        setupEstimateInput("Original Estimate in hours (e.g. 8)"),
		InputRemaining:       setupEstimateInput("Remaining Work in hours (e.g. 4)"),
		InputCompleted:       setupEstimateInput("Completed Work in hours (e.g. 4)"),
		AutoCalcRemaining:    true,
		EffortSameAsEstimate: true,
		ShowEffort:           true,
		ShowOriginal:         true,
		ShowRemaining:        true,
		ShowCompleted:        true,
	}
}

func formatEstimateValue(val float64) string {
	if val <= 0 {
		return ""
	}
	if val == float64(int(val)) {
		return fmt.Sprintf("%.0f", val)
	}
	return fmt.Sprintf("%.1f", val)
}

func (m *ModalEstimate) buildVisibleFields() {
	m.VisibleFields = nil
	if m.ShowEffort {
		m.VisibleFields = append(m.VisibleFields, "effort")
	}
	if m.ShowOriginal {
		m.VisibleFields = append(m.VisibleFields, "original")
	}
	if m.ShowRemaining {
		m.VisibleFields = append(m.VisibleFields, "remaining")
	}
	if m.ShowCompleted {
		m.VisibleFields = append(m.VisibleFields, "completed")
	}
	// Fallback if none matched
	if len(m.VisibleFields) == 0 {
		m.VisibleFields = []string{"effort", "original", "remaining", "completed"}
		m.ShowEffort = true
		m.ShowOriginal = true
		m.ShowRemaining = true
		m.ShowCompleted = true
	}
}

func (m *ModalEstimate) OpenWithDB(id int, title, itemType, state, org, project string, original, remaining, completed, effort float64, database *db.DB) {
	m.WorkItemID = id
	m.ItemTitle = title
	m.ItemType = itemType
	m.ItemState = state
	m.Org = org
	m.Project = project
	m.InitialOriginal = original
	m.InitialRemaining = remaining
	m.InitialCompleted = completed
	m.InitialEffort = effort
	m.effortCustomized = (effort > 0 && effort != original)
	m.effortModified = false
	m.originalModified = false
	m.remainingModified = false
	m.completedModified = false
	m.DB = database

	// Dynamically resolve field permissions and reference names from ADO schema
	if database != nil {
		perms, _ := database.ResolveItemFieldPermissions(org, project, itemType, state)
		m.ShowEffort = perms.ShowEffort
		m.ShowOriginal = perms.ShowOriginal
		m.ShowRemaining = perms.ShowRemaining
		m.ShowCompleted = perms.ShowCompleted
		m.FieldEffort = perms.FieldEffort
		m.FieldOriginal = perms.FieldOriginal
		m.FieldRemaining = perms.FieldRemaining
		m.FieldCompleted = perms.FieldCompleted
	} else {
		isDone := IsStateDoneOrClosed(state)
		m.ShowEffort = true
		m.ShowOriginal = true
		m.ShowCompleted = true
		m.ShowRemaining = !isDone
		m.FieldEffort = "Microsoft.VSTS.Scheduling.Effort"
		m.FieldOriginal = "Microsoft.VSTS.Scheduling.OriginalEstimate"
		m.FieldRemaining = "Microsoft.VSTS.Scheduling.RemainingWork"
		m.FieldCompleted = "Microsoft.VSTS.Scheduling.CompletedWork"
	}

	m.buildVisibleFields()

	if m.AutoCalcRemaining && remaining == 0 && original > 0 && completed == 0 && m.ShowOriginal && m.ShowRemaining {
		remaining = original
	}
	if m.EffortSameAsEstimate && effort == 0 && original > 0 && m.ShowEffort && m.ShowOriginal {
		effort = original
	}

	m.InputEffort.SetValue(formatEstimateValue(effort))
	m.InputOriginal.SetValue(formatEstimateValue(original))
	m.InputRemaining.SetValue(formatEstimateValue(remaining))
	m.InputCompleted.SetValue(formatEstimateValue(completed))

	m.FocusedIndex = 0
	m.updateFocus()
	m.Active = true
}

func (m *ModalEstimate) Open(id int, title, itemType string, original, remaining, completed, effort float64) {
	m.OpenWithDB(id, title, itemType, "", "", "", original, remaining, completed, effort, nil)
}

func (m *ModalEstimate) Close() {
	m.Active = false
	m.InputEffort.Blur()
	m.InputOriginal.Blur()
	m.InputRemaining.Blur()
	m.InputCompleted.Blur()
}

func (m *ModalEstimate) autoCalcRemaining() {
	if !m.AutoCalcRemaining || !m.ShowOriginal || !m.ShowRemaining {
		return
	}
	origStr := strings.TrimSpace(m.InputOriginal.Value())
	compStr := strings.TrimSpace(m.InputCompleted.Value())

	orig, errOrig := strconv.ParseFloat(origStr, 64)
	comp, _ := strconv.ParseFloat(compStr, 64)

	if errOrig == nil {
		rem := orig - comp
		if rem < 0 {
			rem = 0
		}
		m.InputRemaining.SetValue(formatEstimateValue(rem))
	} else if origStr == "" {
		m.InputRemaining.SetValue("")
	}
}

func (m *ModalEstimate) Values() (original, remaining, completed, effort float64) {
	effStr := strings.TrimSpace(m.InputEffort.Value())
	origStr := strings.TrimSpace(m.InputOriginal.Value())
	remStr := strings.TrimSpace(m.InputRemaining.Value())
	compStr := strings.TrimSpace(m.InputCompleted.Value())

	if v, err := strconv.ParseFloat(effStr, 64); err == nil {
		effort = v
	}
	if v, err := strconv.ParseFloat(origStr, 64); err == nil {
		original = v
	}
	if v, err := strconv.ParseFloat(remStr, 64); err == nil {
		remaining = v
	} else if origStr != "" && original > 0 && m.AutoCalcRemaining {
		// Default to original - completed if remaining wasn't explicitly parsed
		if c, errC := strconv.ParseFloat(compStr, 64); errC == nil {
			remaining = original - c
		} else {
			remaining = original
		}
		if remaining < 0 {
			remaining = 0
		}
	}
	if v, err := strconv.ParseFloat(compStr, 64); err == nil {
		completed = v
	}
	return original, remaining, completed, effort
}

// FieldsToUpdate produces a map of field names to values, submitting only fields with user-provided input or explicit changes
func (m *ModalEstimate) FieldsToUpdate() map[string]interface{} {
	fields := make(map[string]interface{})
	effStr := strings.TrimSpace(m.InputEffort.Value())
	origStr := strings.TrimSpace(m.InputOriginal.Value())
	remStr := strings.TrimSpace(m.InputRemaining.Value())
	compStr := strings.TrimSpace(m.InputCompleted.Value())

	effField := m.FieldEffort
	if effField == "" {
		effField = "Microsoft.VSTS.Scheduling.Effort"
	}
	origField := m.FieldOriginal
	if origField == "" {
		origField = "Microsoft.VSTS.Scheduling.OriginalEstimate"
	}
	compField := m.FieldCompleted
	if compField == "" {
		compField = "Microsoft.VSTS.Scheduling.CompletedWork"
	}
	remField := m.FieldRemaining
	if remField == "" {
		remField = "Microsoft.VSTS.Scheduling.RemainingWork"
	}

	if m.ShowEffort {
		if effStr != "" {
			if v, err := strconv.ParseFloat(effStr, 64); err == nil {
				if v != m.InitialEffort || m.effortModified {
					fields[effField] = v
				}
			}
		} else if m.InitialEffort > 0 {
			fields[effField] = ""
		}
	}

	if m.ShowOriginal {
		if origStr != "" {
			if v, err := strconv.ParseFloat(origStr, 64); err == nil {
				if v != m.InitialOriginal || m.originalModified {
					fields[origField] = v
				}
			}
		} else if m.InitialOriginal > 0 {
			fields[origField] = ""
		}
	}

	if m.ShowCompleted {
		if compStr != "" {
			if v, err := strconv.ParseFloat(compStr, 64); err == nil {
				if v != m.InitialCompleted || m.completedModified {
					fields[compField] = v
				}
			}
		} else if m.InitialCompleted > 0 {
			fields[compField] = ""
		}
	}

	isDone := false
	if m.DB != nil {
		isDone = m.DB.IsStateCompleted(m.Org, m.Project, m.ItemType, m.ItemState)
	} else {
		isDone = IsStateDoneOrClosed(m.ItemState)
	}

	if m.ShowRemaining && !isDone {
		if remStr != "" {
			if v, err := strconv.ParseFloat(remStr, 64); err == nil {
				if v != m.InitialRemaining || m.remainingModified {
					fields[remField] = v
				}
			}
		} else if m.InitialRemaining > 0 {
			fields[remField] = ""
		}
	}

	return fields
}

func (m *ModalEstimate) Update(msg tea.Msg) (*ModalEstimate, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	numFields := len(m.VisibleFields)
	if numFields == 0 {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.FocusedIndex = (m.FocusedIndex + 1) % numFields
			m.updateFocus()
			return m, nil
		case "shift+tab", "up":
			m.FocusedIndex = (m.FocusedIndex - 1 + numFields) % numFields
			m.updateFocus()
			return m, nil
		}
	}

	var cmd tea.Cmd
	if m.FocusedIndex >= 0 && m.FocusedIndex < numFields {
		curr := m.VisibleFields[m.FocusedIndex]
		switch curr {
		case "effort":
			prevVal := m.InputEffort.Value()
			m.InputEffort, cmd = m.InputEffort.Update(msg)
			if m.InputEffort.Value() != prevVal {
				m.effortCustomized = true
				m.effortModified = true
			}
		case "original":
			prevVal := m.InputOriginal.Value()
			m.InputOriginal, cmd = m.InputOriginal.Update(msg)
			if m.InputOriginal.Value() != prevVal {
				m.originalModified = true
				if m.AutoCalcRemaining {
					m.autoCalcRemaining()
				}
				if m.EffortSameAsEstimate && !m.effortCustomized && m.ShowEffort {
					m.InputEffort.SetValue(m.InputOriginal.Value())
				}
			}
		case "remaining":
			prevVal := m.InputRemaining.Value()
			m.InputRemaining, cmd = m.InputRemaining.Update(msg)
			if m.InputRemaining.Value() != prevVal {
				m.remainingModified = true
			}
		case "completed":
			prevVal := m.InputCompleted.Value()
			m.InputCompleted, cmd = m.InputCompleted.Update(msg)
			if m.InputCompleted.Value() != prevVal {
				m.completedModified = true
				if m.AutoCalcRemaining {
					m.autoCalcRemaining()
				}
			}
		}
	}
	return m, cmd
}

func (m *ModalEstimate) updateFocus() {
	m.InputEffort.Blur()
	m.InputOriginal.Blur()
	m.InputRemaining.Blur()
	m.InputCompleted.Blur()

	if len(m.VisibleFields) == 0 {
		return
	}
	if m.FocusedIndex < 0 || m.FocusedIndex >= len(m.VisibleFields) {
		m.FocusedIndex = 0
	}

	switch m.VisibleFields[m.FocusedIndex] {
	case "effort":
		m.InputEffort.Focus()
	case "original":
		m.InputOriginal.Focus()
	case "remaining":
		m.InputRemaining.Focus()
	case "completed":
		m.InputCompleted.Focus()
	}
}

func (m *ModalEstimate) View(width, height int) string {
	if !m.Active {
		return ""
	}

	modalWidth := 58
	if width > 0 && width < 62 {
		modalWidth = width - 4
	}

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(fmt.Sprintf("Edit Estimates & Work: #%d", m.WorkItemID))

	itemTitle := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render(m.ItemTitle)

	labelStyle := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)

	isFieldFocused := func(fieldName string) bool {
		if m.FocusedIndex >= 0 && m.FocusedIndex < len(m.VisibleFields) {
			return m.VisibleFields[m.FocusedIndex] == fieldName
		}
		return false
	}

	borderColor := func(fieldName string) lipgloss.Color {
		if isFieldFocused(fieldName) {
			return ColorSecondary
		}
		return ColorSubtle
	}

	renderInputBox := func(label string, fieldName string, ti textinput.Model) string {
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor(fieldName)).
			Padding(0, 1).
			Width(modalWidth - 6).
			Render(ti.View())
		return lipgloss.JoinVertical(lipgloss.Left, labelStyle.Render(label), box)
	}

	var elements []string
	elements = append(elements, titleText, itemTitle)

	for _, f := range m.VisibleFields {
		elements = append(elements, "")
		switch f {
		case "effort":
			elements = append(elements, renderInputBox("Effort:", "effort", m.InputEffort))
		case "original":
			elements = append(elements, renderInputBox("Original Estimate:", "original", m.InputOriginal))
		case "remaining":
			elements = append(elements, renderInputBox("Remaining Work:", "remaining", m.InputRemaining))
		case "completed":
			elements = append(elements, renderInputBox("Completed Work:", "completed", m.InputCompleted))
		}
	}

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1).
		Render("[Tab] Next  [Shift+Tab] Prev  [Enter] Save  [Esc] Cancel")

	elements = append(elements, "", instructions)

	content := lipgloss.JoinVertical(lipgloss.Left, elements...)

	modalBox := lipgloss.NewStyle().
		Width(modalWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modalBox)
}

