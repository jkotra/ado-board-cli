package tui

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Clean, modern dark palette
var (
	ColorPrimary   = lipgloss.Color("#818CF8") // Soft Indigo
	ColorSecondary = lipgloss.Color("#38BDF8") // Sky Blue
	ColorAccent    = lipgloss.Color("#F43F5E") // Rose / Coral
	ColorTeal      = lipgloss.Color("#14B8A6") // Vibrant Teal for PBI
	ColorYellow    = lipgloss.Color("#FBBF24") // Warm Yellow for Tasks
	ColorRed       = lipgloss.Color("#EF4444") // Red for Bugs
	ColorProdIssue = lipgloss.Color("#E11D48") // Deep Crimson / Rose Red for Production Issues
	ColorOrange    = lipgloss.Color("#FB923C") // Orange for Technical Enablement
	ColorPurple    = lipgloss.Color("#A855F7") // Purple for Features
	ColorPink      = lipgloss.Color("#EC4899") // Pink for Epics
	ColorCyan      = lipgloss.Color("#06B6D4") // Cyan for User Stories
	ColorGreen     = lipgloss.Color("#10B981") // Emerald Green for Done
	ColorFg        = lipgloss.Color("#F1F5F9") // Crisp off-white text
	ColorMuted     = lipgloss.Color("#94A3B8") // Slate gray for secondary text
	ColorSubtle     = lipgloss.Color("#334155") // Subtle border / line color
	ColorHighlight  = lipgloss.Color("#1E3A8A") // Deep Royal Navy highlight for active row
	ColorSelectedBg = lipgloss.Color("#1E3A8A") // Explicit row selection background
	ColorTreeLine   = lipgloss.Color("#475569") // Tree branch connectors

	// Aliases
	ColorWarning = ColorYellow
	ColorDanger  = ColorRed
	ColorSuccess = ColorGreen
	ColorInfo    = ColorSecondary
	ColorCardBg  = lipgloss.Color("#0F172A")

	// Azure DevOps Official Brand Palette (from azure-devops.svg)
	ColorAzureDark  = lipgloss.Color("#0078D4") // Deep Azure Blue
	ColorAzureMid   = lipgloss.Color("#3C91E5") // Sky Azure Blue
	ColorAzureLight = lipgloss.Color("#5EA0EF") // Soft Cyan Azure Blue

	// Ribbon and Tab Accent
	ColorRibbonKey = ColorAzureLight // Soft Cyan Azure Blue for shortcut keys
)

// RenderAzureDevOpsBrand renders the official Azure DevOps logo & brand banner
func RenderAzureDevOpsBrand() string {
	logo := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorAzureLight).
		Render("⣠⠴⠖⣤")

	brand := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorFg).
		Render("Azure DevOps")

	board := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSecondary).
		Render("Board")

	return lipgloss.JoinHorizontal(lipgloss.Center, logo, " ", brand, " ", board)
}

// Common UI Styles
var (
	StyleAppTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorAzureDark).
			Padding(0, 2).
			MarginRight(1)

	StyleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary).
			MarginBottom(1)

	StyleSelectedRow = lipgloss.NewStyle().
				Background(ColorHighlight).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	StyleNormalRow = lipgloss.NewStyle().
			Foreground(ColorFg)

	StyleMuted = lipgloss.NewStyle().
			Foreground(ColorMuted)

	StyleBold = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorFg)

	StyleCard = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSubtle).
			Padding(1, 2).
			MarginBottom(1)

	StyleModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2)

	StyleRibbon = lipgloss.NewStyle().
			Background(ColorSubtle).
			Foreground(ColorFg).
			Padding(0, 1)

	StyleRibbonKey = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorYellow).
			Background(ColorSubtle)

	StyleRibbonLabel = lipgloss.NewStyle().
				Foreground(ColorFg).
				Background(ColorSubtle)

	StyleToastSuccess = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorGreen)

	StyleToastError = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorRed)

	StyleToastInfo = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary)
)

// FormatItemType returns concise, standard short names (e.g. Product Backlog Item -> PBI)
func FormatItemType(itemType string) string {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case "product backlog item", "pbi":
		return "PBI"
	case "technical enablement", "tech enablement", "tech enabler":
		return "Tech Enabler"
	case "production issue", "prod issue", "prod_issue", "production_issue":
		return "Prod Issue"
	case "user story":
		return "Story"
	case "task":
		return "Task"
	case "bug":
		return "Bug"
	case "issue":
		return "Issue"
	case "feature":
		return "Feature"
	case "epic":
		return "Epic"
	case "query":
		return "Query"
	default:
		return itemType
	}
}

// GetTypeColor returns the semantic color for a given work item type
func GetTypeColor(itemType string) lipgloss.Color {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case "pbi", "product backlog item":
		return ColorSecondary // Sky Blue
	case "task":
		return ColorYellow // Yellow
	case "bug":
		return ColorRed // Red
	case "production issue", "prod issue", "prod_issue", "production_issue", "issue":
		return ColorProdIssue // Crimson / Deep Rose Red
	case "technical enablement", "tech enablement", "tech enabler":
		return ColorGreen // Green
	case "feature":
		return ColorPurple // Purple
	case "epic":
		return ColorPink // Pink
	case "user story", "story":
		return ColorCyan // Cyan
	case "query":
		return ColorPurple // Purple
	default:
		return ColorSecondary
	}
}

// RenderTypeBadge returns a styled badge for a work item type
func RenderTypeBadge(itemType string) string {
	shortType := FormatItemType(itemType)
	var bg, fg lipgloss.Color
	fg = lipgloss.Color("#FFFFFF")

	switch strings.ToLower(shortType) {
	case "pbi", "product backlog item":
		bg = ColorSecondary
		fg = lipgloss.Color("#000000")
	case "task":
		bg = ColorYellow
		fg = lipgloss.Color("#000000")
	case "bug":
		bg = ColorRed
	case "prod issue", "production issue", "issue":
		bg = ColorProdIssue
		fg = lipgloss.Color("#FFFFFF")
	case "tech enabler", "tech enablement", "technical enablement":
		bg = ColorGreen
		fg = lipgloss.Color("#000000")
	case "feature":
		bg = ColorPurple
	case "epic":
		bg = ColorPink
	case "story":
		bg = ColorCyan
		fg = lipgloss.Color("#000000")
	case "query":
		bg = ColorPurple
		fg = lipgloss.Color("#FFFFFF")
	default:
		bg = lipgloss.Color("#4B5563")
	}

	return lipgloss.NewStyle().
		Bold(true).
		Foreground(fg).
		Background(bg).
		Padding(0, 1).
		Render(shortType)
}

// RenderStateBadge returns a styled badge for a work item state
func RenderStateBadge(state string) string {
	var bg, fg lipgloss.Color
	fg = lipgloss.Color("#FFFFFF")

	switch strings.ToLower(strings.TrimSpace(state)) {
	case "to do", "new", "proposed":
		bg = lipgloss.Color("#334155")
		fg = lipgloss.Color("#94A3B8")
	case "in progress", "active":
		bg = lipgloss.Color("#2563EB") // Blue
	case "committed":
		bg = lipgloss.Color("#0284C7") // Sky
	case "approved":
		bg = lipgloss.Color("#7C3AED") // Purple
	case "done", "closed", "resolved", "completed":
		bg = ColorGreen // Green
		fg = lipgloss.Color("#000000")
	case "removed":
		bg = lipgloss.Color("#1F2937")
		fg = lipgloss.Color("#6B7280")
	default:
		bg = lipgloss.Color("#4B5563")
	}

	return lipgloss.NewStyle().
		Bold(true).
		Foreground(fg).
		Background(bg).
		Padding(0, 1).
		Render(state)
}

// RenderProgressBar renders a visual progress bar
func RenderProgressBar(width int, completed, total float64) string {
	if total <= 0 {
		return lipgloss.NewStyle().Foreground(ColorMuted).Render("[ — ]")
	}

	pct := completed / total
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}

	filledLen := int(pct * float64(width))
	emptyLen := width - filledLen

	filledBar := strings.Repeat("█", filledLen)
	emptyBar := strings.Repeat("░", emptyLen)

	barStyle := lipgloss.NewStyle().Foreground(ColorGreen)
	emptyStyle := lipgloss.NewStyle().Foreground(ColorSubtle)

	return fmt.Sprintf("[%s%s] %s",
		barStyle.Render(filledBar),
		emptyStyle.Render(emptyBar),
		lipgloss.NewStyle().Bold(true).Foreground(ColorFg).Render(fmt.Sprintf("%.0f%%", pct*100)),
	)
}

var (
	htmlTagRegex           = regexp.MustCompile(`<[^>]*>`)
	htmlBrRegex            = regexp.MustCompile(`(?i)<br\s*/?>`)
	htmlDivRegex           = regexp.MustCompile(`(?i)</?div\b[^>]*>`)
	htmlParaRegex          = regexp.MustCompile(`(?i)<p\b[^>]*>`)
	htmlParaCloseRegex     = regexp.MustCompile(`(?i)</p\b[^>]*>`)
	htmlBoldRegex          = regexp.MustCompile(`(?is)<(?:b|strong)\b[^>]*>(.*?)</(?:b|strong)>`)
	htmlSpanBoldRegex      = regexp.MustCompile(`(?is)<(?:span|div|p)\b[^>]*style=["'][^"']*font-weight:\s*(?:bold|[6-9]00)[^"']*["'][^>]*>(.*?)</(?:span|div|p)>`)
	htmlItalicRegex        = regexp.MustCompile(`(?is)<(?:i|em)\b[^>]*>(.*?)</(?:i|em)>`)
	htmlSpanItalicRegex    = regexp.MustCompile(`(?is)<(?:span|div|p)\b[^>]*style=["'][^"']*font-style:\s*italic[^"']*["'][^>]*>(.*?)</(?:span|div|p)>`)
	htmlStrikeRegex        = regexp.MustCompile(`(?is)<(?:s|strike|del)\b[^>]*>(.*?)</(?:s|strike|del)>`)
	htmlSpanStrikeRegex    = regexp.MustCompile(`(?is)<(?:span|div|p)\b[^>]*style=["'][^"']*text-decoration:\s*line-through[^"']*["'][^>]*>(.*?)</(?:span|div|p)>`)
	htmlPreCodeRegex       = regexp.MustCompile(`(?is)<pre\b[^>]*><code\b[^>]*>(.*?)</code></pre>`)
	htmlPreRegex           = regexp.MustCompile(`(?is)<pre\b[^>]*>(.*?)</pre>`)
	htmlCodeRegex          = regexp.MustCompile(`(?is)<code\b[^>]*>(.*?)</code>`)
	htmlHeadingRegex       = regexp.MustCompile(`(?is)<h([1-6])\b[^>]*>(.*?)</h[1-6]>`)
	htmlQuoteRegex         = regexp.MustCompile(`(?is)<blockquote\b[^>]*>(.*?)</blockquote>`)
	htmlHrRegex            = regexp.MustCompile(`(?i)<hr\b[^>]*\s*/?>`)
	htmlTableRegex         = regexp.MustCompile(`(?is)<table\b[^>]*>(.*?)</table>`)
	htmlTrRegex            = regexp.MustCompile(`(?is)<tr\b[^>]*>(.*?)</tr>`)
	htmlThTdRegex          = regexp.MustCompile(`(?is)<t[hd]\b[^>]*>(.*?)</t[hd]>`)
	htmlLiRegex            = regexp.MustCompile(`(?is)<li\b[^>]*>(.*?)</li>`)
	htmlListBoundRegex     = regexp.MustCompile(`(?i)</?(?:ul|ol)\b[^>]*>`)
	htmlOlStartRegex       = regexp.MustCompile(`(?i)\bstart=["']?(\d+)["']?`)
	numListRegex           = regexp.MustCompile(`^(\s*\d+\.\s+)`)
	bulletListRegex        = regexp.MustCompile(`^(\s*[•\-\*]\s+)`)
	htmlAnchorRegex        = regexp.MustCompile(`(?is)<a\b[^>]*href=["']?([^"'>\s]+)["']?[^>]*>(.*?)</a>`)
	htmlLeadingBrRegex     = regexp.MustCompile(`(?i)^(?:<br\s*/?>\s*)+`)
	htmlTrailingBrRegex    = regexp.MustCompile(`(?i)(?:\s*<br\s*/?>)+$`)
	htmlDoubleBrRegex      = regexp.MustCompile(`(?i)(?:<br\s*/?>\s*){2,}`)
	multipleNl             = regexp.MustCompile(`\n{3,}`)
	urlRegex               = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `\x1b]+`)
	ansiOscRegex           = regexp.MustCompile(`\x1b(\[[0-9;?]*[a-zA-Z]|\]8;[^\x1b]*\x1b\\)`)
	mdOrphanBoldLine       = regexp.MustCompile(`(?m)^\s*\*{2,}\s*$`)
	mdEmptyBold            = regexp.MustCompile(`\*{2,}\s*\*{2,}`)
	mdTrailingSpaceBold    = regexp.MustCompile(`\*\*([^\*\n]+?)\s+\*\*`)
	mdLeadingSpaceBold     = regexp.MustCompile(`\*\*\s+([^\*\n]+?)\*\*`)
)

// StripEscapes strips ANSI formatting and OSC 8 sequences from a string
func StripEscapes(s string) string {
	return ansiOscRegex.ReplaceAllString(s, "")
}

// OSCHyperlink formats a URL and visible text with standard OSC 8 terminal hyperlink sequences
func OSCHyperlink(url, text string) string {
	if url == "" {
		return text
	}
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, text)
}

// CleanURL removes trailing punctuation often attached to URLs at sentence ends
func CleanURL(raw string) (string, string) {
	clean := strings.TrimRight(raw, ".,;:!?)>]}\"'")
	trailing := raw[len(clean):]
	return clean, trailing
}

// Linkify styles any HTTP/HTTPS URLs with sky blue color and underline for clear readability and terminal clickability
func Linkify(text string) string {
	linkStyle := lipgloss.NewStyle().Foreground(ColorSecondary).Underline(true)
	return urlRegex.ReplaceAllStringFunc(text, func(u string) string {
		clean, trailing := CleanURL(u)
		if clean == "" {
			return u
		}
		return linkStyle.Render(clean) + trailing
	})
}

// WrapWords wraps lines at whitespace boundaries up to maxCols without splitting inside words, URLs, or ANSI/OSC escape codes
func WrapWords(text string, maxCols int) string {
	if maxCols <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	var wrappedLines []string

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			wrappedLines = append(wrappedLines, "")
			continue
		}

		plain := StripEscapes(line)
		trimmedLeft := strings.TrimLeft(plain, " ")

		var firstPrefix string
		var contPrefix string
		var content string

		if idx := strings.Index(line, "│ "); idx != -1 && strings.HasPrefix(trimmedLeft, "│ ") {
			bar := line[:idx+len("│ ")]
			content = line[idx+len("│ "):]
			plainInner := StripEscapes(content)
			if numMatch := numListRegex.FindStringSubmatch(plainInner); len(numMatch) > 1 {
				pLen := len(numMatch[1])
				firstPrefix = bar + content[:pLen]
				contPrefix = bar + strings.Repeat(" ", runewidth.StringWidth(numMatch[1]))
				content = content[pLen:]
			} else if bulletMatch := bulletListRegex.FindStringSubmatch(plainInner); len(bulletMatch) > 1 {
				pLen := len(bulletMatch[1])
				firstPrefix = bar + content[:pLen]
				contPrefix = bar + strings.Repeat(" ", runewidth.StringWidth(bulletMatch[1]))
				content = content[pLen:]
			} else {
				firstPrefix = bar
				contPrefix = bar
			}
		} else if idx := strings.Index(line, "▎ "); idx != -1 && strings.HasPrefix(trimmedLeft, "▎ ") {
			bar := line[:idx+len("▎ ")]
			content = line[idx+len("▎ "):]
			plainInner := StripEscapes(content)
			if numMatch := numListRegex.FindStringSubmatch(plainInner); len(numMatch) > 1 {
				pLen := len(numMatch[1])
				firstPrefix = bar + content[:pLen]
				contPrefix = bar + strings.Repeat(" ", runewidth.StringWidth(numMatch[1]))
				content = content[pLen:]
			} else if bulletMatch := bulletListRegex.FindStringSubmatch(plainInner); len(bulletMatch) > 1 {
				pLen := len(bulletMatch[1])
				firstPrefix = bar + content[:pLen]
				contPrefix = bar + strings.Repeat(" ", runewidth.StringWidth(bulletMatch[1]))
				content = content[pLen:]
			} else {
				firstPrefix = bar
				contPrefix = bar
			}
		} else if numMatch := numListRegex.FindStringSubmatch(plain); len(numMatch) > 1 {
			pLen := len(numMatch[1])
			firstPrefix = line[:pLen]
			contPrefix = strings.Repeat(" ", runewidth.StringWidth(numMatch[1]))
			content = line[pLen:]
		} else if bulletMatch := bulletListRegex.FindStringSubmatch(plain); len(bulletMatch) > 1 {
			pLen := len(bulletMatch[1])
			firstPrefix = line[:pLen]
			contPrefix = strings.Repeat(" ", runewidth.StringWidth(bulletMatch[1]))
			content = line[pLen:]
		} else {
			indentLen := len(line) - len(strings.TrimLeft(line, " "))
			if indentLen > 0 {
				firstPrefix = line[:indentLen]
				contPrefix = firstPrefix
				content = line[indentLen:]
			} else {
				firstPrefix = ""
				contPrefix = ""
				content = line
			}
		}

		words := strings.Fields(content)
		if len(words) == 0 {
			wrappedLines = append(wrappedLines, firstPrefix)
			continue
		}

		var curLine strings.Builder
		firstPrefixWidth := runewidth.StringWidth(StripEscapes(firstPrefix))
		contPrefixWidth := runewidth.StringWidth(StripEscapes(contPrefix))

		curLine.WriteString(firstPrefix)
		curWidth := firstPrefixWidth

		for i, word := range words {
			plainWord := StripEscapes(word)
			wordLen := runewidth.StringWidth(plainWord)

			if i == 0 && curWidth == firstPrefixWidth {
				curLine.WriteString(word)
				curWidth += wordLen
			} else if curWidth+1+wordLen <= maxCols {
				curLine.WriteString(" ")
				curLine.WriteString(word)
				curWidth += 1 + wordLen
			} else {
				wrappedLines = append(wrappedLines, curLine.String())
				curLine.Reset()
				curLine.WriteString(contPrefix)
				curLine.WriteString(word)
				curWidth = contPrefixWidth + wordLen
			}
		}
		if curLine.Len() > 0 {
			wrappedLines = append(wrappedLines, curLine.String())
		}
	}

	return strings.Join(wrappedLines, "\n")
}

// formatInline wraps non-empty inner content with markdown formatting (e.g. "**", "*", "~~")
// while properly leaving leading/trailing whitespace outside the delimiter to adhere to CommonMark rules.
func formatInline(inner, marker string) string {
	plain := htmlTagRegex.ReplaceAllString(inner, "")
	if strings.TrimSpace(plain) == "" {
		return inner
	}

	leadLen := 0
	for leadLen < len(inner) && (inner[leadLen] == ' ' || inner[leadLen] == '\t' || inner[leadLen] == '\n' || inner[leadLen] == '\r') {
		leadLen++
	}
	lead := inner[:leadLen]
	inner = inner[leadLen:]

	trailLen := 0
	for trailLen < len(inner) && (inner[len(inner)-1-trailLen] == ' ' || inner[len(inner)-1-trailLen] == '\t' || inner[len(inner)-1-trailLen] == '\n' || inner[len(inner)-1-trailLen] == '\r') {
		trailLen++
	}
	trail := inner[len(inner)-trailLen:]
	core := inner[:len(inner)-trailLen]

	core = htmlLeadingBrRegex.ReplaceAllString(core, "")
	core = htmlTrailingBrRegex.ReplaceAllString(core, "")

	corePlain := strings.TrimSpace(htmlTagRegex.ReplaceAllString(core, ""))
	if corePlain == "" {
		return lead + trail
	}

	if htmlDoubleBrRegex.MatchString(core) {
		parts := htmlDoubleBrRegex.Split(core, -1)
		var formattedParts []string
		for _, p := range parts {
			pClean := strings.TrimSpace(htmlTagRegex.ReplaceAllString(p, ""))
			if pClean != "" {
				formattedParts = append(formattedParts, marker+strings.TrimSpace(p)+marker)
			}
		}
		return lead + strings.Join(formattedParts, "\n\n") + trail
	}

	return lead + marker + core + marker + trail
}

// SanitizeMarkdown cleans up broken markdown asterisks, empty delimiter markers,
// and orphan delimiter lines so Glamour parses standard CommonMark properly.
func SanitizeMarkdown(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}

	s := input

	// 1. Remove standalone asterisks or markdown markers on their own line
	s = mdOrphanBoldLine.ReplaceAllString(s, "")

	// 2. Remove empty bold/italic markers (**** or ** **)
	s = mdEmptyBold.ReplaceAllString(s, "")

	// 3. Clean consecutive newlines to maximum 2 newlines and trim line endings
	lines := strings.Split(s, "\n")
	var cleanedLines []string
	prevEmpty := false
	for _, l := range lines {
		trimmedRight := strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(trimmedRight) == "" {
			if !prevEmpty {
				cleanedLines = append(cleanedLines, "")
				prevEmpty = true
			}
		} else {
			cleanedLines = append(cleanedLines, trimmedRight)
			prevEmpty = false
		}
	}

	return strings.TrimSpace(strings.Join(cleanedLines, "\n"))
}

// StripAndFormatHTML converts HTML description or comment into clean markdown representation
func StripAndFormatHTML(input string) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}

	s := input

	// 1. Normalize line endings and non-breaking spaces
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&#160;", " ")
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = strings.ReplaceAll(s, "\u200b", "")

	// 2. Resolve mentions before stripping HTML tags so @<GUID> is preserved and resolved
	s = ResolveMentions(s)

	// 3. Convert HTML tables to Markdown tables
	s = htmlTableRegex.ReplaceAllStringFunc(s, func(tableMatch string) string {
		rows := htmlTrRegex.FindAllString(tableMatch, -1)
		if len(rows) == 0 {
			return ""
		}
		var mdRows []string
		maxCols := 0
		for _, row := range rows {
			cells := htmlThTdRegex.FindAllStringSubmatch(row, -1)
			if len(cells) == 0 {
				continue
			}
			var rowCells []string
			for _, cell := range cells {
				cellContent := strings.TrimSpace(htmlTagRegex.ReplaceAllString(cell[1], ""))
				cellContent = strings.ReplaceAll(cellContent, "|", "\\|")
				cellContent = strings.ReplaceAll(cellContent, "\n", " ")
				rowCells = append(rowCells, cellContent)
			}
			if len(rowCells) > maxCols {
				maxCols = len(rowCells)
			}
			mdRows = append(mdRows, "| "+strings.Join(rowCells, " | ")+" |")
		}
		if len(mdRows) == 0 {
			return ""
		}
		var sep []string
		for i := 0; i < maxCols; i++ {
			sep = append(sep, "---")
		}
		sepRow := "| " + strings.Join(sep, " | ") + " |"
		var result []string
		result = append(result, mdRows[0], sepRow)
		if len(mdRows) > 1 {
			result = append(result, mdRows[1:]...)
		}
		return "\n\n" + strings.Join(result, "\n") + "\n\n"
	})

	// 4. Convert preformatted code blocks and inline code
	s = htmlPreCodeRegex.ReplaceAllString(s, "\n```\n$1\n```\n")
	s = htmlPreRegex.ReplaceAllString(s, "\n```\n$1\n```\n")
	s = htmlCodeRegex.ReplaceAllString(s, "`$1`")

	// 5. Convert headings <h1>-<h6>
	s = htmlHeadingRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlHeadingRegex.FindStringSubmatch(match)
		if len(sub) >= 3 {
			level := 1
			if len(sub[1]) > 0 && sub[1][0] >= '1' && sub[1][0] <= '6' {
				level = int(sub[1][0] - '0')
			}
			content := strings.TrimSpace(htmlTagRegex.ReplaceAllString(sub[2], ""))
			if content != "" {
				return "\n\n" + strings.Repeat("#", level) + " " + content + "\n\n"
			}
		}
		return ""
	})

	// 6. Convert horizontal rules and blockquotes
	s = htmlHrRegex.ReplaceAllString(s, "\n\n---\n\n")
	s = htmlQuoteRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlQuoteRegex.FindStringSubmatch(match)
		if len(sub) >= 2 {
			inner := strings.TrimSpace(sub[1])
			lines := strings.Split(inner, "\n")
			var qLines []string
			for _, l := range lines {
				qLines = append(qLines, "> "+l)
			}
			return "\n\n" + strings.Join(qLines, "\n") + "\n\n"
		}
		return match
	})

	// 7. Preserve HTML anchor links as markdown links
	s = htmlAnchorRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlAnchorRegex.FindStringSubmatch(match)
		if len(sub) >= 3 {
			href := strings.TrimSpace(sub[1])
			anchorText := strings.TrimSpace(htmlTagRegex.ReplaceAllString(sub[2], ""))
			if href != "" {
				if anchorText == "" || anchorText == href {
					return href
				}
				return fmt.Sprintf("[%s](%s)", anchorText, href)
			}
		}
		return match
	})

	// 8. Convert bold, strong, and font-weight styles
	s = htmlBoldRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlBoldRegex.FindStringSubmatch(match)
		if len(sub) >= 2 {
			return formatInline(sub[1], "**")
		}
		return match
	})
	s = htmlSpanBoldRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlSpanBoldRegex.FindStringSubmatch(match)
		if len(sub) >= 2 {
			return formatInline(sub[1], "**")
		}
		return match
	})

	// 9. Convert italic, em, and font-style styles
	s = htmlItalicRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlItalicRegex.FindStringSubmatch(match)
		if len(sub) >= 2 {
			return formatInline(sub[1], "*")
		}
		return match
	})
	s = htmlSpanItalicRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlSpanItalicRegex.FindStringSubmatch(match)
		if len(sub) >= 2 {
			return formatInline(sub[1], "*")
		}
		return match
	})

	// 10. Convert strikethrough and del styles
	s = htmlStrikeRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlStrikeRegex.FindStringSubmatch(match)
		if len(sub) >= 2 {
			return formatInline(sub[1], "~~")
		}
		return match
	})
	s = htmlSpanStrikeRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlSpanStrikeRegex.FindStringSubmatch(match)
		if len(sub) >= 2 {
			return formatInline(sub[1], "~~")
		}
		return match
	})

	// 11. Convert HTML ordered and unordered lists (<ol>, <ul>, <li>)
	s = convertHTMLLists(s)

	// 12. Replace line breaking and paragraph tags with newlines
	s = htmlBrRegex.ReplaceAllString(s, "\n")
	s = htmlParaRegex.ReplaceAllString(s, "\n\n")
	s = htmlParaCloseRegex.ReplaceAllString(s, "\n\n")
	s = htmlDivRegex.ReplaceAllString(s, "\n")

	// 13. Strip all remaining HTML tags
	s = htmlTagRegex.ReplaceAllString(s, "")

	// 14. Unescape HTML entities (&amp; -> &, &lt; -> <, &gt; -> >)
	s = html.UnescapeString(s)

	// 15. Sanitize and normalize markdown formatting
	s = SanitizeMarkdown(s)

	return strings.TrimSpace(s)
}

// convertHTMLLists transforms <ol>, <ul>, and <li> elements into standard CommonMark markdown lists
func convertHTMLLists(input string) string {
	s := input

	for iter := 0; iter < 10; iter++ {
		lower := strings.ToLower(s)
		olClose := strings.Index(lower, "</ol>")
		ulClose := strings.Index(lower, "</ul>")

		var closeIdx int
		var closeTagLen int
		var isOrdered bool

		if olClose != -1 && ulClose != -1 {
			if olClose < ulClose {
				closeIdx = olClose
				closeTagLen = len("</ol>")
				isOrdered = true
			} else {
				closeIdx = ulClose
				closeTagLen = len("</ul>")
				isOrdered = false
			}
		} else if olClose != -1 {
			closeIdx = olClose
			closeTagLen = len("</ol>")
			isOrdered = true
		} else if ulClose != -1 {
			closeIdx = ulClose
			closeTagLen = len("</ul>")
			isOrdered = false
		} else {
			break
		}

		beforeClose := lower[:closeIdx]
		lastOlOpen := strings.LastIndex(beforeClose, "<ol")
		lastUlOpen := strings.LastIndex(beforeClose, "<ul")

		var openIdx int
		if lastOlOpen != -1 && lastUlOpen != -1 {
			if lastOlOpen > lastUlOpen {
				openIdx = lastOlOpen
				isOrdered = true
			} else {
				openIdx = lastUlOpen
				isOrdered = false
			}
		} else if lastOlOpen != -1 {
			openIdx = lastOlOpen
			isOrdered = true
		} else if lastUlOpen != -1 {
			openIdx = lastUlOpen
			isOrdered = false
		} else {
			s = s[:closeIdx] + s[closeIdx+closeTagLen:]
			continue
		}

		tagCloseIdx := strings.Index(s[openIdx:], ">")
		if tagCloseIdx == -1 {
			break
		}
		openTagEnd := openIdx + tagCloseIdx + 1
		openTagHeader := s[openIdx:openTagEnd]
		listContent := s[openTagEnd:closeIdx]

		startIndex := 1
		if isOrdered {
			if startMatch := htmlOlStartRegex.FindStringSubmatch(openTagHeader); len(startMatch) > 1 {
				if val, err := strconv.Atoi(startMatch[1]); err == nil && val > 0 {
					startIndex = val
				}
			}
		}

		liMatches := htmlLiRegex.FindAllStringSubmatch(listContent, -1)
		var mdItems []string
		currIndex := startIndex

		for _, liMatch := range liMatches {
			if len(liMatch) < 2 {
				continue
			}
			itemRaw := liMatch[1]
			itemRaw = strings.ReplaceAll(itemRaw, "\r\n", "\n")
			itemRaw = strings.ReplaceAll(itemRaw, "\r", "\n")

			itemRaw = htmlBrRegex.ReplaceAllString(itemRaw, "\n")
			itemRaw = htmlDivRegex.ReplaceAllString(itemRaw, "\n")
			itemRaw = htmlParaRegex.ReplaceAllString(itemRaw, "\n")
			itemRaw = htmlParaCloseRegex.ReplaceAllString(itemRaw, "\n")

			rawLines := strings.Split(itemRaw, "\n")
			var itemLines []string
			for _, l := range rawLines {
				t := strings.TrimSpace(l)
				if t != "" {
					itemLines = append(itemLines, t)
				}
			}

			if len(itemLines) == 0 {
				continue
			}

			prefix := fmt.Sprintf("%d. ", currIndex)
			contIndent := "   "
			if !isOrdered {
				prefix = "- "
				contIndent = "  "
			}

			var formattedItem strings.Builder
			formattedItem.WriteString(prefix)
			formattedItem.WriteString(itemLines[0])
			for _, l := range itemLines[1:] {
				formattedItem.WriteString("\n")
				formattedItem.WriteString(contIndent)
				formattedItem.WriteString(l)
			}

			mdItems = append(mdItems, formattedItem.String())
			if isOrdered {
				currIndex++
			}
		}

		var replacement string
		if len(mdItems) > 0 {
			replacement = "\n\n" + strings.Join(mdItems, "\n") + "\n\n"
		} else {
			plain := strings.TrimSpace(htmlTagRegex.ReplaceAllString(listContent, ""))
			if plain != "" {
				replacement = "\n\n" + plain + "\n\n"
			}
		}

		s = s[:openIdx] + replacement + s[closeIdx+closeTagLen:]
	}

	// Clean up any remaining orphan list tags
	s = htmlListBoundRegex.ReplaceAllString(s, "\n")
	s = htmlLiRegex.ReplaceAllStringFunc(s, func(match string) string {
		sub := htmlLiRegex.FindStringSubmatch(match)
		if len(sub) >= 2 {
			plain := strings.TrimSpace(htmlTagRegex.ReplaceAllString(sub[1], ""))
			if plain == "" {
				return ""
			}
			return "\n- " + strings.TrimSpace(sub[1])
		}
		return ""
	})

	return s
}

// FormatDateTime formats an ISO date string to YYYY-MM-DD
func formatDateTime(dateStr string) string {
	if len(dateStr) >= 10 {
		return dateStr[:10]
	}
	return dateStr
}
