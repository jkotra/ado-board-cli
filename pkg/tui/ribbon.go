package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shortcut represents a keyboard shortcut in the bottom ribbon
type Shortcut struct {
	Key   string
	Label string
}

// RenderBottomRibbon renders the bottom shortcut ribbon across the full terminal width cleanly without leftover color blocks
func RenderBottomRibbon(width int, screenName string, shortcuts []Shortcut, toastMsg string, isError bool) string {
	if width <= 0 {
		width = 80
	}

	var screenBadge string
	badgeWidth := 0
	if screenName != "" {
		screenBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorPrimary).
			Padding(0, 1).
			Render(screenName)
		badgeWidth = lipgloss.Width(screenBadge) + 2
	}

	// Toast message
	var toastStr string
	if toastMsg != "" {
		if isError {
			toastStr = lipgloss.NewStyle().Bold(true).Foreground(ColorDanger).Render(" " + toastMsg + " ")
		} else if strings.Contains(toastMsg, "...") || strings.ContainsAny(toastMsg, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") {
			toastStr = lipgloss.NewStyle().Foreground(ColorSecondary).Render(" " + toastMsg + " ")
		} else {
			toastStr = lipgloss.NewStyle().Foreground(ColorSuccess).Render(" " + toastMsg + " ")
		}
	}

	// Always reserve space for [q] Quit if it is present in the shortcuts list
	hasQuit := false
	var quitItemStr string
	quitWidth := 0
	for _, sc := range shortcuts {
		if strings.EqualFold(sc.Key, "q") {
			hasQuit = true
			quitKey := lipgloss.NewStyle().Bold(true).Foreground(ColorRibbonKey).Render("[" + sc.Key + "]")
			quitLabel := lipgloss.NewStyle().Foreground(ColorFg).Render(" " + sc.Label)
			quitItemStr = quitKey + quitLabel
			quitWidth = lipgloss.Width(quitItemStr) + 2
			break
		}
	}

	toastWidth := lipgloss.Width(toastStr)
	availableForShortcuts := width - badgeWidth - toastWidth - quitWidth - 2
	if availableForShortcuts < 0 {
		availableForShortcuts = 0
	}

	// Build shortcuts string that strictly fits in 1 single line (skipping 'q' so it stays pinned at end)
	var sb strings.Builder
	currentLen := 0

	for _, sc := range shortcuts {
		if strings.EqualFold(sc.Key, "q") {
			continue
		}
		key := lipgloss.NewStyle().Bold(true).Foreground(ColorRibbonKey).Render("[" + sc.Key + "]")
		label := lipgloss.NewStyle().Foreground(ColorFg).Render(" " + sc.Label)
		scItem := key + label
		itemWidth := lipgloss.Width(scItem)

		sepWidth := 0
		if currentLen > 0 {
			sepWidth = 2
		}

		if currentLen+sepWidth+itemWidth > availableForShortcuts {
			break
		}

		if currentLen > 0 {
			sb.WriteString("  ")
			currentLen += 2
		}
		sb.WriteString(scItem)
		currentLen += itemWidth
	}

	if hasQuit && quitItemStr != "" {
		if sb.Len() > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(quitItemStr)
	}

	shortcutsStr := sb.String()

	// Calculate spacing
	var leftContent string
	if screenBadge != "" {
		leftContent = screenBadge + "  " + shortcutsStr
	} else {
		leftContent = shortcutsStr
	}
	leftLen := lipgloss.Width(leftContent)

	if toastStr != "" {
		gap := width - leftLen - toastWidth - 2
		if gap < 1 {
			gap = 1
		}
		return leftContent + strings.Repeat(" ", gap) + toastStr
	}

	return leftContent
}

// FormatToast returns a formatted string with an icon
func FormatSuccessToast(msg string) string {
	return fmt.Sprintf("✓ %s", msg)
}

func FormatErrorToast(msg string) string {
	return fmt.Sprintf("✗ %s", msg)
}

func FormatInfoToast(msg string) string {
	return fmt.Sprintf("⏳ %s", msg)
}
