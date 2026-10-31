package tui

import (
	"fmt"
	"strings"

	"ado-board-cli/pkg/clipboard"
	"ado-board-cli/pkg/gemini"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MsgGeminiProgress carries a live progress update from tool execution
type MsgGeminiProgress struct {
	Step string
}

// MsgGeminiDone carries the final response and modified item IDs
type MsgGeminiDone struct {
	Result *gemini.ExecutionResult
	Err    error
}

// DockGemini represents the interactive Gemini prompt dock below the ribbon
type DockGemini struct {
	Visible       bool
	Input         textarea.Model
	Spinner       spinner.Model
	Loading       bool
	StatusMsg     string
	ResponseMsg   string
	ErrorMsg      string
	History       []string
	HistoryIdx    int
	Width         int
	Height        int
	ModelName     string
	ScrollOffset  int
	PaneHeight    int
	renderedCache string
	renderedLines []string
}

// NewDockGemini creates a new Gemini AI prompt dock
func NewDockGemini(modelName string) *DockGemini {
	ta := textarea.New()
	ta.Placeholder = "Ask Gemini (e.g., 'update estimate to 8h', 'move to user@example.com', 'divide 12h into 2 research tasks')..."
	ta.Focus()
	ta.CharLimit = 10000
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.SetPromptFunc(4, func(lineIdx int) string {
		if lineIdx == 0 {
			return "✦ > "
		}
		return "  · "
	})
	ta.ShowLineNumbers = false
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	ta.FocusedStyle.Text = lipgloss.NewStyle().Foreground(ColorFg)
	ta.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(ColorMuted)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)

	if modelName == "" {
		modelName = "gemini-3.8-flash"
	}

	return &DockGemini{
		Visible:    false,
		Input:      ta,
		Spinner:    sp,
		Loading:    false,
		History:    make([]string, 0),
		HistoryIdx: -1,
		ModelName:  modelName,
		PaneHeight: 8,
	}
}

// InsertText inserts text into the prompt textarea, normalizing newlines and updating dimensions
func (d *DockGemini) InsertText(content string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if content == "" {
		return
	}
	d.Input.InsertString(content)

	// If pasting multi-line text and dock is small, auto-expand pane height
	numLines := strings.Count(content, "\n") + 1
	if numLines > 2 && d.PaneHeight < 12 {
		targetH := d.PaneHeight + (numLines - 1)
		if targetH > 16 {
			targetH = 16
		}
		d.PaneHeight = targetH
	}
	d.updateInputHeight()
}

// Toggle toggles the dock visibility
func (d *DockGemini) Toggle() {
	d.Visible = !d.Visible
	if d.Visible {
		d.Input.Focus()
	} else {
		d.Input.Blur()
	}
}

// Show opens the dock and focuses input
func (d *DockGemini) Show() {
	d.Visible = true
	d.Input.Focus()
}

// Hide closes the dock and blurs input
func (d *DockGemini) Hide() {
	d.Visible = false
	d.Input.Blur()
}

// Clear resets the response, error, status message, and scroll position
func (d *DockGemini) Clear() {
	d.ResponseMsg = ""
	d.ErrorMsg = ""
	d.StatusMsg = ""
	d.ScrollOffset = 0
	d.renderedLines = nil
	d.renderedCache = ""
	d.Input.Reset()
	d.updateInputHeight()
}

func (d *DockGemini) updateInputHeight() {
	lines := strings.Split(d.Input.Value(), "\n")
	numLines := len(lines)
	if numLines < 1 {
		numLines = 1
	}
	innerH := d.GetPaneHeight() - 2
	var maxInputH int
	if d.ResponseMsg != "" || d.Loading || d.ErrorMsg != "" {
		// When response or loading state is active, balance input and response
		maxInputH = innerH / 2
	} else {
		// When typing or reviewing prompt without a response yet, allow prompt to expand
		maxInputH = innerH - 1
	}
	if maxInputH < 1 {
		maxInputH = 1
	}
	if numLines > maxInputH {
		numLines = maxInputH
	}
	d.Input.SetHeight(numLines)
}

// GetPaneHeight returns the clamped total outer height of the dock
func (d *DockGemini) GetPaneHeight() int {
	h := d.PaneHeight
	if h <= 0 {
		h = 8
	}
	minH := 6
	maxH := 35
	if d.Height > 0 {
		maxH = d.Height - 7
	}
	if maxH < minH {
		maxH = minH
	}
	if h < minH {
		h = minH
	}
	if h > maxH {
		h = maxH
	}
	return h
}

// IncreaseHeight increases the visible dock pane height
func (d *DockGemini) IncreaseHeight(delta int) {
	d.PaneHeight = d.GetPaneHeight() + delta
	d.updateInputHeight()
}

// DecreaseHeight shrinks the visible dock pane height
func (d *DockGemini) DecreaseHeight(delta int) {
	d.PaneHeight = d.GetPaneHeight() - delta
	d.updateInputHeight()
}

func (d *DockGemini) availableResponseHeight() int {
	innerH := d.GetPaneHeight() - 2
	inputH := lipgloss.Height(d.Input.View())
	availH := innerH - 1 - inputH
	if availH < 1 {
		availH = 1
	}
	return availH
}

// ScrollUp scrolls up by n lines
func (d *DockGemini) ScrollUp(n int) {
	d.ScrollOffset -= n
	if d.ScrollOffset < 0 {
		d.ScrollOffset = 0
	}
}

// ScrollDown scrolls down by n lines
func (d *DockGemini) ScrollDown(n int) {
	d.ScrollOffset += n
	maxOffset := d.maxScrollOffset()
	if d.ScrollOffset > maxOffset {
		d.ScrollOffset = maxOffset
	}
}

func (d *DockGemini) getRenderedLines() []string {
	if d.ResponseMsg == "" {
		d.renderedLines = nil
		d.renderedCache = ""
		return nil
	}
	dockWidth := d.Width - 4
	if dockWidth < 40 {
		dockWidth = 40
	}
	cacheKey := fmt.Sprintf("%d:%s", dockWidth, d.ResponseMsg)
	if d.renderedCache == cacheKey && d.renderedLines != nil {
		return d.renderedLines
	}

	respRendered := RenderMarkdown(d.ResponseMsg, dockWidth-6)
	lines := strings.Split(strings.TrimRight(respRendered, "\n"), "\n")
	d.renderedLines = lines
	d.renderedCache = cacheKey
	return lines
}

// maxScrollOffset returns the maximum allowed scroll offset
func (d *DockGemini) maxScrollOffset() int {
	lines := d.getRenderedLines()
	totalLines := len(lines)
	if totalLines == 0 {
		return 0
	}
	availH := d.availableResponseHeight()
	if totalLines <= availH {
		return 0
	}
	return totalLines - availH
}

// GetURLAt finds a URL corresponding to relative Y coordinate in the dock or visible response
func (d *DockGemini) GetURLAt(relY, relX int) string {
	lines := d.getRenderedLines()
	if len(lines) == 0 {
		return ""
	}

	inputH := lipgloss.Height(d.Input.View())
	headerRows := 2 + inputH
	respLineIdx := relY - headerRows + d.ScrollOffset

	// 1. Check exact line if within bounds
	if respLineIdx >= 0 && respLineIdx < len(lines) {
		line := lines[respLineIdx]
		cleanLine := StripEscapes(line)
		matches := urlRegex.FindAllString(cleanLine, -1)
		if len(matches) > 0 {
			return cleanURL(matches[0])
		}
	}

	// 2. Check nearby lines (± 1 or 2 rows for click tolerance)
	for delta := 1; delta <= 2; delta++ {
		for _, idx := range []int{respLineIdx - delta, respLineIdx + delta} {
			if idx >= 0 && idx < len(lines) {
				cleanLine := StripEscapes(lines[idx])
				if m := urlRegex.FindString(cleanLine); m != "" {
					return cleanURL(m)
				}
			}
		}
	}

	// 3. Fallback: check all visible rendered lines
	availH := d.availableResponseHeight()
	start := d.ScrollOffset
	end := start + availH
	if end > len(lines) {
		end = len(lines)
	}
	if start >= 0 && start < len(lines) {
		for i := start; i < end; i++ {
			cleanLine := StripEscapes(lines[i])
			if m := urlRegex.FindString(cleanLine); m != "" {
				return cleanURL(m)
			}
		}
	}

	// 4. Fallback: check raw ResponseMsg
	if m := urlRegex.FindString(d.ResponseMsg); m != "" {
		return cleanURL(m)
	}

	return ""
}

func cleanURL(u string) string {
	u = strings.TrimSpace(u)
	for len(u) > 0 {
		last := u[len(u)-1]
		if last == '.' || last == ',' || last == ')' || last == ']' || last == ';' || last == '>' || last == '\'' || last == '"' {
			u = u[:len(u)-1]
		} else {
			break
		}
	}
	return u
}

// SetSize updates layout dimensions
func (d *DockGemini) SetSize(w, h int) {
	d.Width = w
	d.Height = h
	inputWidth := w - 10
	if inputWidth < 20 {
		inputWidth = 20
	}
	d.Input.SetWidth(inputWidth)
	d.updateInputHeight()
}

// Update handles tea messages for the dock
func (d *DockGemini) Update(msg tea.Msg) (*DockGemini, tea.Cmd) {
	if !d.Visible {
		return d, nil
	}

	switch msg := msg.(type) {
	case spinner.TickMsg:
		if d.Loading {
			var cmd tea.Cmd
			d.Spinner, cmd = d.Spinner.Update(msg)
			return d, cmd
		}
		return d, nil

	case MsgGeminiProgress:
		d.StatusMsg = msg.Step
		return d, nil

	case MsgGeminiDone:
		d.Loading = false
		d.ScrollOffset = 0
		d.renderedLines = nil
		d.renderedCache = ""
		if msg.Err != nil {
			d.ErrorMsg = msg.Err.Error()
			d.ResponseMsg = ""
		} else if msg.Result != nil {
			d.ErrorMsg = ""
			d.ResponseMsg = msg.Result.Response
			d.StatusMsg = "✅ Done"
		}
		return d, nil

	case MsgPaste:
		d.InsertText(msg.Content)
		return d, nil

	case tea.KeyMsg:
		str := msg.String()
		// Filter out leaked mouse tracking escape sequences (e.g. [<64;79;20M, <0;..., etc.)
		if strings.HasPrefix(str, "[<") || strings.HasPrefix(str, "<") || (strings.Contains(str, ";") && strings.HasSuffix(str, "M")) {
			return d, nil
		}

		switch str {
		case "esc":
			d.Hide()
			return d, nil

		case "ctrl+v":
			clipText, err := clipboard.Read()
			if err == nil && clipText != "" {
				d.InsertText(clipText)
				return d, nil
			}

		case "shift+enter", "alt+enter", "alt+ctrl+j", "ctrl+enter", "ctrl+j", "\n", "\r\n":
			d.Input.InsertString("\n")
			d.updateInputHeight()
			return d, nil

		case "ctrl+up":
			d.IncreaseHeight(2)
			return d, nil

		case "ctrl+down":
			d.DecreaseHeight(2)
			return d, nil

		case "delete", "del", "ctrl+delete", "ctrl+del", "shift+delete", "shift+c", "ctrl+l":
			k := msg.String()
			if d.Input.Value() == "" || k == "ctrl+delete" || k == "ctrl+del" || k == "ctrl+l" {
				d.Clear()
				return d, nil
			}

		case "pageup", "pgup":
			d.ScrollUp(d.availableResponseHeight() - 1)
			return d, nil

		case "pagedown", "pgdown":
			d.ScrollDown(d.availableResponseHeight() - 1)
			return d, nil

		case "ctrl+u":
			d.ScrollUp(4)
			return d, nil

		case "ctrl+d":
			d.ScrollDown(4)
			return d, nil

		case "home":
			if d.Input.Value() == "" {
				d.ScrollOffset = 0
				return d, nil
			}

		case "end":
			if d.Input.Value() == "" {
				d.ScrollOffset = d.maxScrollOffset()
				return d, nil
			}

		case "up":
			if len(d.History) > 0 && d.Input.Line() == 0 && (d.Input.Value() == "" || d.HistoryIdx != -1 || d.Input.LineCount() <= 1) {
				if d.HistoryIdx == -1 {
					d.HistoryIdx = len(d.History) - 1
				} else if d.HistoryIdx > 0 {
					d.HistoryIdx--
				}
				d.Input.SetValue(d.History[d.HistoryIdx])
				d.updateInputHeight()
				return d, nil
			}

		case "down":
			if len(d.History) > 0 && d.HistoryIdx != -1 && d.Input.Line() >= d.Input.LineCount()-1 {
				if d.HistoryIdx < len(d.History)-1 {
					d.HistoryIdx++
					d.Input.SetValue(d.History[d.HistoryIdx])
				} else {
					d.HistoryIdx = -1
					d.Input.SetValue("")
				}
				d.updateInputHeight()
				return d, nil
			}
		}
	}

	var cmd tea.Cmd
	d.Input, cmd = d.Input.Update(msg)
	d.updateInputHeight()
	return d, cmd
}

// View renders the Gemini prompt dock
func (d *DockGemini) View() string {
	if !d.Visible {
		return ""
	}

	dockWidth := d.Width - 4
	if dockWidth < 40 {
		dockWidth = 40
	}

	totalH := d.GetPaneHeight()
	innerH := totalH - 2
	if innerH < 4 {
		innerH = 4
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorCyan)

	modelTag := lipgloss.NewStyle().
		Foreground(ColorYellow).
		Render("[" + d.ModelName + "]")

	closeHint := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("[Ctrl+↑/↓: Resize • Shift+↑/↓: Tree • Del: Clear • Enter: Run • Esc: Cancel/Hide]")

	topBar := fmt.Sprintf("✦ Gemini AI Assistant %s %s", modelTag, closeHint)

	var body []string
	body = append(body, headerStyle.Render(topBar))
	body = append(body, d.Input.View())

	availResponseH := innerH - 1 - lipgloss.Height(d.Input.View())
	if availResponseH < 1 {
		availResponseH = 1
	}

	if d.Loading {
		spView := d.Spinner.View()
		statusText := d.StatusMsg
		if statusText == "" {
			statusText = "Analyzing context & executing actions..."
		}
		statusText = strings.TrimPrefix(statusText, "✦ ")
		statusText = strings.TrimPrefix(statusText, "⏳ ")
		statusText = strings.TrimSpace(statusText)

		spinnerStyle := lipgloss.NewStyle().Foreground(ColorYellow).Bold(true)
		body = append(body, fmt.Sprintf("%s %s", spView, spinnerStyle.Render(statusText)))
	} else if d.ErrorMsg != "" {
		errStyle := lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
		body = append(body, errStyle.Render(fmt.Sprintf("❌ Error: %s", d.ErrorMsg)))
	} else if d.ResponseMsg != "" {
		lines := d.getRenderedLines()
		totalLines := len(lines)

		var visibleLines []string
		var scrollInfo string

		if totalLines <= availResponseH {
			visibleLines = lines
			d.ScrollOffset = 0
		} else {
			displayLines := availResponseH - 1
			if displayLines < 1 {
				displayLines = 1
			}
			maxOffset := totalLines - displayLines
			if d.ScrollOffset > maxOffset {
				d.ScrollOffset = maxOffset
			}
			if d.ScrollOffset < 0 {
				d.ScrollOffset = 0
			}

			endIdx := d.ScrollOffset + displayLines
			if endIdx > totalLines {
				endIdx = totalLines
			}
			visibleLines = lines[d.ScrollOffset:endIdx]

			scrollPct := 0
			if maxOffset > 0 {
				scrollPct = (d.ScrollOffset * 100) / maxOffset
			}
			scrollInfo = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Italic(true).
				Render(fmt.Sprintf("  [PgUp/PgDn to scroll • %d%% (%d-%d/%d)]", scrollPct, d.ScrollOffset+1, endIdx, totalLines))
		}

		respBox := lipgloss.NewStyle().
			Foreground(ColorFg).
			Render(strings.Join(visibleLines, "\n"))
		body = append(body, respBox)
		if scrollInfo != "" {
			body = append(body, scrollInfo)
		}
	} else {
		if availResponseH > 1 {
			body = append(body, lipgloss.NewStyle().Foreground(ColorMuted).Italic(true).Render("✦ Ready. Ask anything about your work items, estimates, or assignees."))
		}
	}

	content := strings.Join(body, "\n")

	dockBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorCyan).
		Padding(0, 1).
		Width(dockWidth).
		Height(innerH).
		Render(content)

	return dockBox
}
