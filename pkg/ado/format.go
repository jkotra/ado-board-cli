package ado

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var (
	// Regexes to extract code blocks and inline code so they are protected from mention replacement
	fencedCodeRegex = regexp.MustCompile("(?s)```[^\n]*\n.*?```")
	inlineCodeRegex = regexp.MustCompile("`[^`\n]+`")

	// Regex to check if input is already rich HTML
	richHTMLRegex = regexp.MustCompile(`(?i)<(?:p|div|br|span|a|table|ul|ol|li|h[1-6]|strong|em|pre|code|blockquote)[^>]*>`)

	// Mention patterns
	// 1. @DisplayName <email@domain.com>
	mentionWithEmailRegex = regexp.MustCompile(`(?i)(?:^|[\s\(\[\{:,;])@([^\n<@]+?)\s*<([a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,})>`)

	// 2. @<GUID> e.g. @<141B714F-561B-4171-8933-CB7ECF2EA3FE>
	mentionGuidRegex = regexp.MustCompile(`(?i)(?:^|[\s\(\[\{:,;])@<([0-9a-fA-F\-]{36})>`)

	// 3. @<email@domain.com>
	mentionAngleEmailRegex = regexp.MustCompile(`(?i)(?:^|[\s\(\[\{:,;])@<([a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,})>`)

	// Check if already an HTML mention tag
	vssMentionTagRegex = regexp.MustCompile(`(?i)<a[^>]*data-vss-mention[^>]*>.*?</a>`)

	// Shared Goldmark converter instance
	adoGoldmark = goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
			extension.Strikethrough,
			extension.Linkify,
			extension.TaskList,
		),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),   // Allow raw HTML tags (e.g., data-vss-mention links)
			html.WithHardWraps(), // Convert newlines inside paragraphs to <br />
			html.WithXHTML(),
		),
	)
)

// FormatMarkdownToAdoHTML converts a markdown or rich text string into ADO-compatible HTML,
// converting @mentions into native <a href="#" data-vss-mention="version:2.0,{id}">@DisplayName</a> tags
// and markdown elements (backticks, lists, headers, bold, italics, tables) into clean HTML.
func FormatMarkdownToAdoHTML(input string, resolver func(string) *IdentityRef, knownUsers []*IdentityRef) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}

	// 1. Protect code blocks and inline code from mention replacement
	codePlaceholders := make(map[string]string)
	codeCount := 0

	protected := fencedCodeRegex.ReplaceAllStringFunc(trimmed, func(m string) string {
		key := fmt.Sprintf("\x00CODEBLOCK_%d\x00", codeCount)
		codeCount++
		codePlaceholders[key] = m
		return key
	})

	protected = inlineCodeRegex.ReplaceAllStringFunc(protected, func(m string) string {
		key := fmt.Sprintf("\x00INLINECODE_%d\x00", codeCount)
		codeCount++
		codePlaceholders[key] = m
		return key
	})

	// 2. Protect existing <a data-vss-mention...> tags
	vssPlaceholders := make(map[string]string)
	vssCount := 0
	protected = vssMentionTagRegex.ReplaceAllStringFunc(protected, func(m string) string {
		key := fmt.Sprintf("\x00VSSMENTION_%d\x00", vssCount)
		vssCount++
		vssPlaceholders[key] = m
		return key
	})

	// 3. Resolve explicit mentions: @DisplayName <email>
	protected = mentionWithEmailRegex.ReplaceAllStringFunc(protected, func(match string) string {
		sub := mentionWithEmailRegex.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		rawPrefix := match[:strings.Index(match, "@")]
		name := strings.TrimSpace(sub[1])
		email := strings.TrimSpace(sub[2])

		var targetID string
		displayName := name
		if resolver != nil {
			if ref := resolver(email); ref != nil {
				targetID = ref.ID
				if ref.DisplayName != "" {
					displayName = ref.DisplayName
				}
			} else if ref := resolver(name); ref != nil {
				targetID = ref.ID
				if ref.DisplayName != "" {
					displayName = ref.DisplayName
				}
			}
		}
		if targetID == "" {
			targetID = email
		}

		return rawPrefix + fmt.Sprintf(`<a href="#" data-vss-mention="version:2.0,%s">@%s</a>`, targetID, displayName)
	})

	// 4. Resolve GUID mentions: @<141B714F-561B-4171-8933-CB7ECF2EA3FE>
	protected = mentionGuidRegex.ReplaceAllStringFunc(protected, func(match string) string {
		sub := mentionGuidRegex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		rawPrefix := match[:strings.Index(match, "@")]
		guid := strings.TrimSpace(sub[1])
		displayName := "User"
		if resolver != nil {
			if ref := resolver(guid); ref != nil && ref.DisplayName != "" {
				displayName = ref.DisplayName
			}
		}
		return rawPrefix + fmt.Sprintf(`<a href="#" data-vss-mention="version:2.0,%s">@%s</a>`, guid, displayName)
	})

	// 5. Resolve angle-bracket email mentions: @<bob.johnson@example.com>
	protected = mentionAngleEmailRegex.ReplaceAllStringFunc(protected, func(match string) string {
		sub := mentionAngleEmailRegex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		rawPrefix := match[:strings.Index(match, "@")]
		email := strings.TrimSpace(sub[1])
		targetID := email
		displayName := email
		if resolver != nil {
			if ref := resolver(email); ref != nil {
				if ref.ID != "" {
					targetID = ref.ID
				}
				if ref.DisplayName != "" {
					displayName = ref.DisplayName
				}
			}
		}
		return rawPrefix + fmt.Sprintf(`<a href="#" data-vss-mention="version:2.0,%s">@%s</a>`, targetID, displayName)
	})

	// 6. Match known identities: @DisplayName (e.g. @Johnson, Bob Henry or @Brown, Carol)
	if len(knownUsers) > 0 {
		// Sort known identities by display name length descending to avoid prefix collision
		sortedUsers := make([]*IdentityRef, 0, len(knownUsers))
		for _, u := range knownUsers {
			if u != nil && strings.TrimSpace(u.DisplayName) != "" {
				sortedUsers = append(sortedUsers, u)
			}
		}
		sort.SliceStable(sortedUsers, func(i, j int) bool {
			return len(sortedUsers[i].DisplayName) > len(sortedUsers[j].DisplayName)
		})

		for _, u := range sortedUsers {
			name := strings.TrimSpace(u.DisplayName)
			if name == "" {
				continue
			}
			targetID := u.ID
			if targetID == "" {
				targetID = u.UniqueName
			}
			replacement := fmt.Sprintf(`<a href="#" data-vss-mention="version:2.0,%s">@%s</a>`, targetID, name)
			protected = replaceMention(protected, name, replacement)
		}
	}

	// 7. Dynamic resolver for remaining single-token mentions like @bob or @jdoe
	if resolver != nil {
		dynamicMentionRegex := regexp.MustCompile(`(?i)(?:^|[\s\(\[\{:,;])@([a-zA-Z0-9_\-\.]+)`)
		protected = dynamicMentionRegex.ReplaceAllStringFunc(protected, func(match string) string {
			atIdx := strings.Index(match, "@")
			if atIdx < 0 {
				return match
			}
			prefix := match[:atIdx]
			token := match[atIdx+1:]
			if strings.HasPrefix(token, "\x00") {
				return match
			}
			if ref := resolver(token); ref != nil {
				targetID := ref.ID
				if targetID == "" {
					targetID = ref.UniqueName
				}
				name := ref.DisplayName
				if name == "" {
					name = token
				}
				return prefix + fmt.Sprintf(`<a href="#" data-vss-mention="version:2.0,%s">@%s</a>`, targetID, name)
			}
			return match
		})
	}

	// 8. Restore VSS Mention placeholders
	for k, v := range vssPlaceholders {
		protected = strings.ReplaceAll(protected, k, v)
	}

	// 9. Restore code placeholders
	for k, v := range codePlaceholders {
		protected = strings.ReplaceAll(protected, k, v)
	}

	// 10. If the text is already full HTML, return as is (with mentions expanded)
	if isFullHTML(protected) {
		return strings.TrimSpace(protected)
	}

	// 11. Convert Markdown to HTML via Goldmark
	var buf bytes.Buffer
	if err := adoGoldmark.Convert([]byte(protected), &buf); err != nil {
		// Fallback to simple HTML paragraph if goldmark fails
		return "<p>" + strings.ReplaceAll(protected, "\n", "<br/>") + "</p>"
	}

	htmlResult := strings.TrimSpace(buf.String())
	return htmlResult
}

// isFullHTML checks if a string is already structured HTML
func isFullHTML(s string) bool {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "<p>") ||
		strings.HasPrefix(trimmed, "<div>") ||
		strings.HasPrefix(trimmed, "<html>") ||
		strings.HasPrefix(trimmed, "<table>") ||
		strings.HasPrefix(trimmed, "<ul>") ||
		strings.HasPrefix(trimmed, "<ol>") ||
		strings.HasPrefix(trimmed, "<h1>") ||
		strings.HasPrefix(trimmed, "<h2>") ||
		strings.HasPrefix(trimmed, "<h3>") {
		return true
	}
	return false
}

// isHTMLField returns true if the specified field is an ADO rich-text HTML field
func isHTMLField(fieldName string) bool {
	switch strings.ToLower(strings.TrimSpace(fieldName)) {
	case "system.description", "microsoft.vsts.common.acceptancecriteria",
		"microsoft.vsts.tcm.reprosteps", "system.history",
		"description", "acceptancecriteria", "reprosteps", "history":
		return true
	default:
		lower := strings.ToLower(fieldName)
		return strings.HasSuffix(lower, ".description") ||
			strings.HasSuffix(lower, ".acceptancecriteria") ||
			strings.HasSuffix(lower, ".reprosteps") ||
			strings.HasSuffix(lower, ".history")
	}
}

// replaceMention matches @Name case-insensitively with word/punctuation boundary checks and replaces with replacement
func replaceMention(input, name, replacement string) string {
	if name == "" || input == "" {
		return input
	}
	nameLen := len(name)
	nameLower := strings.ToLower(name)
	inputLower := strings.ToLower(input)

	var sb strings.Builder
	lastIdx := 0

	for {
		idx := strings.Index(inputLower[lastIdx:], "@")
		if idx == -1 {
			sb.WriteString(input[lastIdx:])
			break
		}
		atPos := lastIdx + idx

		// Check prefix boundary (start of string or whitespace / bracket / punctuation)
		prefixValid := (atPos == 0)
		if !prefixValid {
			prevChar := input[atPos-1]
			if strings.ContainsRune(" \t\n\r([{:;,", rune(prevChar)) {
				prefixValid = true
			}
		}

		// Check if name matches
		matchEnd := atPos + 1 + nameLen
		if prefixValid && matchEnd <= len(input) && inputLower[atPos+1:matchEnd] == nameLower {
			// Check suffix boundary (end of string or whitespace / bracket / punctuation)
			suffixValid := (matchEnd == len(input))
			if !suffixValid {
				nextChar := input[matchEnd]
				if strings.ContainsRune(" \t\n\r)]},;:!?.<>", rune(nextChar)) {
					suffixValid = true
				}
			}

			if suffixValid {
				sb.WriteString(input[lastIdx:atPos])
				sb.WriteString(replacement)
				lastIdx = matchEnd
				continue
			}
		}

		sb.WriteString(input[lastIdx : atPos+1])
		lastIdx = atPos + 1
	}

	return sb.String()
}
