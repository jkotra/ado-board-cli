package tui

import (
	"fmt"
	"strings"
	"unicode"

	"ado-board-cli/pkg/db"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// AutocompleteType defines what type of items are being suggested
type AutocompleteType int

const (
	AutocompleteNone AutocompleteType = iota
	AutocompleteIdentity
	AutocompleteWorkItem
	AutocompletePullRequest
)

// SuggestionItem represents a single autocomplete option
type SuggestionItem struct {
	ID          string // Unique key (e.g. ID or email)
	Title       string // Primary text (e.g. Name, WI Title, PR Title)
	Subtitle    string // Secondary text (e.g. Email, Status, Branch)
	InsertValue string // Exact text to insert upon selection
	Badge       string // Optional badge (e.g. "PBI", "Task", "PR")
	BadgeColor  lipgloss.Color
}

// AutocompleteState manages autocomplete suggestions and selection
type AutocompleteState struct {
	Active        bool
	Type          AutocompleteType
	TriggerChar   rune
	Query         string
	StartIndex    int
	EndIndex      int
	SelectedIndex int
	Items         []SuggestionItem
	MaxItems      int
}

func NewAutocompleteState() *AutocompleteState {
	return &AutocompleteState{
		MaxItems: 5,
	}
}

// Reset clears the autocomplete state
func (a *AutocompleteState) Reset() {
	a.Active = false
	a.Type = AutocompleteNone
	a.TriggerChar = 0
	a.Query = ""
	a.StartIndex = 0
	a.EndIndex = 0
	a.SelectedIndex = 0
	a.Items = nil
}

// Next moves selection down
func (a *AutocompleteState) Next() {
	if len(a.Items) > 0 {
		a.SelectedIndex = (a.SelectedIndex + 1) % len(a.Items)
	}
}

// Prev moves selection up
func (a *AutocompleteState) Prev() {
	if len(a.Items) > 0 {
		a.SelectedIndex = (a.SelectedIndex - 1 + len(a.Items)) % len(a.Items)
	}
}

// Selected returns the currently highlighted suggestion
func (a *AutocompleteState) Selected() *SuggestionItem {
	if a.Active && len(a.Items) > 0 && a.SelectedIndex >= 0 && a.SelectedIndex < len(a.Items) {
		return &a.Items[a.SelectedIndex]
	}
	return nil
}

// DetectInlineTrigger inspects text up to cursor to detect @, #, or ! triggers
func DetectInlineTrigger(text string, cursor int) (trigger rune, query string, startIdx int, found bool) {
	if cursor < 0 || cursor > len(text) {
		cursor = len(text)
	}
	sub := text[:cursor]
	if len(sub) == 0 {
		return 0, "", 0, false
	}

	// Scan backward from cursor
	for i := len(sub) - 1; i >= 0; i-- {
		ch := rune(sub[i])
		if unicode.IsSpace(ch) {
			return 0, "", 0, false // Hit whitespace before trigger
		}
		if ch == '@' || ch == '#' || ch == '!' {
			// Trigger must be at beginning of text or preceded by whitespace, parenthesis, or punctuation
			if i == 0 || unicode.IsSpace(rune(sub[i-1])) || strings.ContainsRune(" \t\n([{\"',.:;", rune(sub[i-1])) {
				q := sub[i+1:]
				return ch, q, i, true
			}
			return 0, "", 0, false
		}
	}
	return 0, "", 0, false
}

// ReplaceToken replaces the trigger query token in text with insertVal
func ReplaceToken(text string, startIdx int, queryLen int, insertVal string) string {
	if startIdx < 0 || startIdx > len(text) {
		return text + " " + insertVal
	}
	endIdx := startIdx + 1 + queryLen // 1 for trigger char (@, #, !) + query length
	if endIdx > len(text) {
		endIdx = len(text)
	}
	after := text[endIdx:]
	if after == "" {
		insertVal += " "
	} else if !strings.HasPrefix(after, " ") && !strings.HasPrefix(after, "\n") && !strings.HasPrefix(after, "\t") {
		insertVal += " "
	}
	return text[:startIdx] + insertVal + after
}

// UpdateSuggestions updates suggestions based on trigger and query from SQLite DB
func (a *AutocompleteState) UpdateSuggestions(database *db.DB, trigger rune, query string, startIdx, endIdx int) {
	if database == nil {
		a.Reset()
		return
	}

	a.TriggerChar = trigger
	a.Query = query
	a.StartIndex = startIdx
	a.EndIndex = endIdx
	a.Items = nil

	switch trigger {
	case '@':
		a.Type = AutocompleteIdentity
		identities, err := database.SearchIdentities(query, a.MaxItems)
		if err == nil {
			for _, id := range identities {
				insert := "@" + id.DisplayName
				if id.Email != "" {
					insert = fmt.Sprintf("@%s <%s>", id.DisplayName, id.Email)
				}
				a.Items = append(a.Items, SuggestionItem{
					ID:          id.ID,
					Title:       id.DisplayName,
					Subtitle:    id.Email,
					InsertValue: insert,
					Badge:       "User",
					BadgeColor:  ColorSecondary,
				})
			}
		}

	case '#':
		a.Type = AutocompleteWorkItem
		items, err := database.SearchWorkItems(query, a.MaxItems)
		if err == nil {
			for _, item := range items {
				a.Items = append(a.Items, SuggestionItem{
					ID:          fmt.Sprintf("%d", item.ID),
					Title:       fmt.Sprintf("#%d %s", item.ID, item.Title),
					Subtitle:    fmt.Sprintf("[%s] %s", item.WorkItemType, item.State),
					InsertValue: fmt.Sprintf("#%d", item.ID),
					Badge:       FormatItemType(item.WorkItemType),
					BadgeColor:  GetTypeColor(item.WorkItemType),
				})
			}
		}

	case '!':
		a.Type = AutocompletePullRequest
		prs, err := database.SearchPullRequests(query, a.MaxItems)
		if err == nil {
			for _, pr := range prs {
				a.Items = append(a.Items, SuggestionItem{
					ID:          fmt.Sprintf("%d", pr.ID),
					Title:       fmt.Sprintf("!%d %s", pr.ID, pr.Title),
					Subtitle:    fmt.Sprintf("[%s] %s ➔ %s", strings.ToUpper(pr.Status), pr.SourceBranch, pr.TargetBranch),
					InsertValue: fmt.Sprintf("!%d", pr.ID),
					Badge:       "PR",
					BadgeColor:  ColorPurple,
				})
			}
		}
	}

	if len(a.Items) > 0 {
		a.Active = true
		if a.SelectedIndex >= len(a.Items) {
			a.SelectedIndex = 0
		}
	} else {
		a.Active = false
		a.SelectedIndex = 0
	}
}

// UpdateIdentitySuggestions directly loads identity suggestions for assign modal
func (a *AutocompleteState) UpdateIdentitySuggestions(database *db.DB, query string) {
	if database == nil {
		a.Reset()
		return
	}
	a.Type = AutocompleteIdentity
	a.TriggerChar = 0
	a.Query = query
	a.Items = nil

	identities, err := database.SearchIdentities(query, a.MaxItems)
	if err == nil {
		for _, id := range identities {
			insertVal := id.DisplayName
			if id.Email != "" {
				insertVal = id.Email
			}
			a.Items = append(a.Items, SuggestionItem{
				ID:          id.ID,
				Title:       id.DisplayName,
				Subtitle:    id.Email,
				InsertValue: insertVal,
				Badge:       "User",
				BadgeColor:  ColorSecondary,
			})
		}
	}

	if len(a.Items) > 0 {
		a.Active = true
		if a.SelectedIndex >= len(a.Items) {
			a.SelectedIndex = 0
		}
	} else {
		a.Active = false
		a.SelectedIndex = 0
	}
}

// HandleKey processes navigation keys for autocomplete
func (a *AutocompleteState) HandleKey(msg tea.KeyMsg) (handled bool, selected *SuggestionItem) {
	if !a.Active || len(a.Items) == 0 {
		return false, nil
	}

	switch msg.String() {
	case "up", "ctrl+p":
		a.Prev()
		return true, nil
	case "down", "ctrl+n":
		a.Next()
		return true, nil
	case "tab", "enter":
		return true, a.Selected()
	case "esc":
		a.Reset()
		return true, nil
	}
	return false, nil
}

// View renders the autocomplete suggestion box
func (a *AutocompleteState) View(maxWidth int) string {
	if !a.Active || len(a.Items) == 0 {
		return ""
	}

	boxWidth := maxWidth
	if boxWidth < 38 {
		boxWidth = 38
	}
	if boxWidth > 64 {
		boxWidth = 64
	}

	var rows []string
	headerLabel := "Suggestions"
	switch a.Type {
	case AutocompleteIdentity:
		headerLabel = "👤 Users (@)"
	case AutocompleteWorkItem:
		headerLabel = "📋 Work Items (#)"
	case AutocompletePullRequest:
		headerLabel = "🔀 Pull Requests (!)"
	}

	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSecondary).
		Render(fmt.Sprintf("%s (%d)", headerLabel, len(a.Items)))
	rows = append(rows, header)

	for i, item := range a.Items {
		isSelected := i == a.SelectedIndex

		prefix := "  "
		styleTitle := lipgloss.NewStyle().Foreground(ColorFg)
		styleSubtitle := lipgloss.NewStyle().Foreground(ColorMuted)

		if isSelected {
			prefix = lipgloss.NewStyle().Bold(true).Foreground(ColorYellow).Render("❯ ")
			styleTitle = styleTitle.Bold(true).Foreground(ColorYellow)
			styleSubtitle = lipgloss.NewStyle().Foreground(ColorSecondary)
		}

		badge := ""
		if item.Badge != "" {
			badge = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#000000")).
				Background(item.BadgeColor).
				Padding(0, 1).
				Bold(true).
				Render(item.Badge) + " "
		}

		maxContentLen := boxWidth - 12
		if maxContentLen < 20 {
			maxContentLen = 20
		}

		var fullText string
		if item.Subtitle != "" {
			fullText = fmt.Sprintf("%s <%s>", item.Title, item.Subtitle)
		} else {
			fullText = item.Title
		}

		truncatedText := truncateString(fullText, maxContentLen)

		var renderedText string
		if strings.Contains(truncatedText, "<") && strings.Contains(truncatedText, ">") {
			parts := strings.SplitN(truncatedText, "<", 2)
			namePart := parts[0]
			emailPart := "<" + parts[1]
			renderedText = styleTitle.Render(namePart) + styleSubtitle.Render(emailPart)
		} else {
			renderedText = styleTitle.Render(truncatedText)
		}

		rowContent := fmt.Sprintf("%s%s%s", prefix, badge, renderedText)
		rows = append(rows, rowContent)
	}

	hint := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("[↑/↓] Navigate  [Tab] Insert  [Esc] Close")
	rows = append(rows, "", hint)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSecondary).
		Padding(0, 1).
		Width(boxWidth).
		Render(strings.Join(rows, "\n"))
}
