package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
)

var (
	rendererCache = make(map[int]*glamour.TermRenderer)
	rendererMu    sync.RWMutex
)

func strPtr(s string) *string { return &s }
func bPtr(b bool) *bool       { return &b }
func uPtr(u uint) *uint       { return &u }

func getAdoMarkdownStyle() ansi.StyleConfig {
	style := styles.DarkStyleConfig

	// 1. Zero margin for precise full-width terminal layout
	zero := uint(0)
	style.Document.Margin = &zero

	// 2. BlockQuote: styled border and distinct italic styling
	quoteColor := "#94A3B8" // Crisp readable quote text
	quoteBar := "│ "
	style.BlockQuote = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color:  strPtr(quoteColor),
			Italic: bPtr(true),
		},
		Indent:      uPtr(1),
		IndentToken: strPtr(quoteBar),
	}

	// 3. Headings: clean colored headings without arbitrary prefix bars
	h1Color := "#38BDF8"
	h2Color := "#818CF8"
	h3Color := "#60A5FA"
	bold := true

	style.Heading = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Bold: &bold,
		},
	}
	style.H1 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "",
			Color:  strPtr(h1Color),
			Bold:   &bold,
		},
	}
	style.H2 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "",
			Color:  strPtr(h2Color),
			Bold:   &bold,
		},
	}
	style.H3 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "",
			Color:  strPtr(h3Color),
			Bold:   &bold,
		},
	}
	style.H4 = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "",
			Color:  strPtr(h3Color),
			Bold:   &bold,
		},
	}

	// 4. Code & Codeblocks
	codeFg := "#FBBF24"
	codeBg := "#1E293B"
	style.Code = ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix:          " ",
			Suffix:          " ",
			Color:           strPtr(codeFg),
			BackgroundColor: strPtr(codeBg),
		},
	}
	style.CodeBlock.Margin = &zero
	style.Paragraph.Margin = &zero

	// 5. Links
	linkFg := "#38BDF8"
	style.Link = ansi.StylePrimitive{
		Color:     strPtr(linkFg),
		Underline: bPtr(true),
	}
	style.LinkText = ansi.StylePrimitive{
		Color:     strPtr(linkFg),
		Underline: bPtr(true),
	}

	// 6. Horizontal Rule
	hrColor := "#334155"
	style.HorizontalRule = ansi.StylePrimitive{
		Color:  strPtr(hrColor),
		Format: "\n────────────────────────────\n",
	}

	return style
}

func getOrCreateRenderer() (*glamour.TermRenderer, error) {
	rendererMu.RLock()
	if r, ok := rendererCache[0]; ok {
		rendererMu.RUnlock()
		return r, nil
	}
	rendererMu.RUnlock()

	rendererMu.Lock()
	defer rendererMu.Unlock()

	if r, ok := rendererCache[0]; ok {
		return r, nil
	}

	style := getAdoMarkdownStyle()
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		return nil, err
	}

	rendererCache[0] = r
	return r, nil
}

// RenderMarkdown formats markdown string for high-quality terminal display using cached Glamour renderer
func RenderMarkdown(input string, wrapWidth int) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}

	sanitized := SanitizeMarkdown(trimmed)
	if sanitized == "" {
		return ""
	}

	r, err := getOrCreateRenderer()
	if err != nil {
		return Linkify(WrapWords(sanitized, wrapWidth))
	}

	out, err := r.Render(sanitized)
	if err != nil {
		return Linkify(WrapWords(sanitized, wrapWidth))
	}

	out = Linkify(out)
	if wrapWidth > 0 {
		out = WrapWords(out, wrapWidth)
	}

	return strings.TrimSpace(out)
}
