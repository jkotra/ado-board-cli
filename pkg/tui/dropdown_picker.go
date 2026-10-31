package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DropdownPicker provides a single-select searchable popup dialog for selecting options
type DropdownPicker struct {
	Active        bool
	Title         string
	AllOptions    []string
	Filtered      []string
	SelectedValue string
	SearchInput   textinput.Model
	Cursor        int
	ScrollOffset  int
	MaxVisible    int
}

func NewDropdownPicker() *DropdownPicker {
	ti := textinput.New()
	ti.Placeholder = "Type to search or enter custom..."
	ti.CharLimit = 150
	ti.Width = 46
	ti.Prompt = "🔍 "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(ColorFg)
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	return &DropdownPicker{
		SearchInput: ti,
		MaxVisible:  7,
	}
}

func (p *DropdownPicker) Open(title string, options []string, currentSelected string) {
	p.Active = true
	p.Title = title
	p.SelectedValue = currentSelected
	p.SearchInput.Reset()
	p.SearchInput.Focus()

	// Ensure unique options, preserving original options order
	seen := make(map[string]bool)
	var cleanOpts []string
	for _, opt := range options {
		trimmed := strings.TrimSpace(opt)
		if trimmed != "" && !seen[trimmed] {
			cleanOpts = append(cleanOpts, trimmed)
			seen[trimmed] = true
		}
	}
	if currentSelected != "" && !seen[currentSelected] {
		cleanOpts = append([]string{currentSelected}, cleanOpts...)
	}
	p.AllOptions = cleanOpts
	p.filter("")

	// Position cursor on current selected if found
	p.Cursor = 0
	p.ScrollOffset = 0
	for i, opt := range p.Filtered {
		if strings.EqualFold(opt, currentSelected) {
			p.Cursor = i
			if p.Cursor >= p.MaxVisible {
				p.ScrollOffset = p.Cursor - p.MaxVisible + 1
			}
			break
		}
	}
}

func (p *DropdownPicker) Close() {
	p.Active = false
	p.SearchInput.Blur()
}

func (p *DropdownPicker) filter(query string) {
	q := strings.ToLower(strings.TrimSpace(query))
	var filtered []string

	for _, opt := range p.AllOptions {
		if q == "" || strings.Contains(strings.ToLower(opt), q) {
			filtered = append(filtered, opt)
		}
	}

	// If query is typed and no existing options matched, allow using typed custom value
	if q != "" && len(filtered) == 0 {
		filtered = append(filtered, strings.TrimSpace(query))
	}

	p.Filtered = filtered
	if p.Cursor >= len(p.Filtered) {
		p.Cursor = 0
		p.ScrollOffset = 0
	}
}

func (p *DropdownPicker) Update(msg tea.Msg) (selectedVal string, chosen bool, closed bool, cmd tea.Cmd) {
	if !p.Active {
		return "", false, false, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			p.Close()
			return "", false, true, nil

		case "enter":
			var val string
			if len(p.Filtered) > 0 && p.Cursor >= 0 && p.Cursor < len(p.Filtered) {
				val = p.Filtered[p.Cursor]
			} else {
				val = strings.TrimSpace(p.SearchInput.Value())
			}
			p.SelectedValue = val
			p.Close()
			return val, true, true, nil

		case "up", "ctrl+p":
			if p.Cursor > 0 {
				p.Cursor--
				if p.Cursor < p.ScrollOffset {
					p.ScrollOffset = p.Cursor
				}
			}
			return "", false, false, nil

		case "down", "ctrl+n":
			if p.Cursor < len(p.Filtered)-1 {
				p.Cursor++
				if p.Cursor >= p.ScrollOffset+p.MaxVisible {
					p.ScrollOffset = p.Cursor - p.MaxVisible + 1
				}
			}
			return "", false, false, nil

		case "pgup":
			p.Cursor -= p.MaxVisible
			if p.Cursor < 0 {
				p.Cursor = 0
			}
			p.ScrollOffset = p.Cursor
			return "", false, false, nil

		case "pgdown":
			p.Cursor += p.MaxVisible
			if p.Cursor >= len(p.Filtered) {
				p.Cursor = len(p.Filtered) - 1
			}
			if p.Cursor < 0 {
				p.Cursor = 0
			}
			if p.Cursor >= p.ScrollOffset+p.MaxVisible {
				p.ScrollOffset = p.Cursor - p.MaxVisible + 1
			}
			return "", false, false, nil
		}
	}

	prevVal := p.SearchInput.Value()
	var searchCmd tea.Cmd
	p.SearchInput, searchCmd = p.SearchInput.Update(msg)
	if p.SearchInput.Value() != prevVal {
		p.filter(p.SearchInput.Value())
	}

	return "", false, false, searchCmd
}

func (p *DropdownPicker) View(parentWidth, parentHeight int) string {
	if !p.Active {
		return ""
	}

	boxWidth := 56
	if parentWidth > 0 && parentWidth < 60 {
		boxWidth = parentWidth - 4
	}

	titleText := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Render(p.Title)

	searchBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSecondary).
		Padding(0, 1).
		Width(boxWidth - 6).
		Render(p.SearchInput.View())

	var listItems []string
	endIdx := p.ScrollOffset + p.MaxVisible
	if endIdx > len(p.Filtered) {
		endIdx = len(p.Filtered)
	}

	if len(p.Filtered) == 0 {
		listItems = append(listItems, lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render("  (no matches - press Enter to use custom)"))
	} else {
		for i := p.ScrollOffset; i < endIdx; i++ {
			opt := p.Filtered[i]
			isCurrent := (opt == p.SelectedValue)
			isCursor := (i == p.Cursor)

			var prefix string
			if isCursor {
				prefix = lipgloss.NewStyle().Foreground(ColorYellow).Bold(true).Render("❯ ")
			} else {
				prefix = "  "
			}

			var itemText string
			if isCurrent {
				itemText = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true).Render(opt + " ✓")
			} else if isCursor {
				itemText = lipgloss.NewStyle().Foreground(ColorFg).Bold(true).Render(opt)
			} else {
				itemText = lipgloss.NewStyle().Foreground(ColorMuted).Render(opt)
			}

			listItems = append(listItems, prefix+itemText)
		}
	}

	scrollInfo := ""
	if p.ScrollOffset > 0 || endIdx < len(p.Filtered) {
		scrollInfo = lipgloss.NewStyle().Foreground(ColorSubtle).Render(fmt.Sprintf(" [%d/%d]", p.Cursor+1, len(p.Filtered)))
	}

	instructions := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("[↑/↓] Navigate  [Enter] Select  [Esc] Cancel" + scrollInfo)

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleText,
		"",
		searchBox,
		"",
		strings.Join(listItems, "\n"),
		"",
		instructions,
	)

	modalBox := lipgloss.NewStyle().
		Width(boxWidth).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSecondary).
		Padding(1, 2).
		Render(content)

	return lipgloss.Place(parentWidth, parentHeight, lipgloss.Center, lipgloss.Center, modalBox)
}
