//go:build windows

package tui

import (
	"errors"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

// GetTerminalSize returns the current terminal dimensions (width, height) on Windows.
func GetTerminalSize() (int, int, error) {
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 && h > 0 {
		return w, h, nil
	}
	if w, h, err := term.GetSize(int(os.Stderr.Fd())); err == nil && w > 0 && h > 0 {
		return w, h, nil
	}
	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil && w > 0 && h > 0 {
		return w, h, nil
	}
	return 0, 0, errors.New("unable to determine terminal size")
}

// checkTerminalResize polls for terminal window dimension changes on Windows.
// Returns a tea.Cmd dispatching tea.WindowSizeMsg if dimensions changed, or nil if unchanged.
func (m *AppModel) checkTerminalResize() tea.Cmd {
	if w, h, err := GetTerminalSize(); err == nil && w > 0 && h > 0 {
		if w != m.Width || h != m.Height {
			return func() tea.Msg {
				return tea.WindowSizeMsg{Width: w, Height: h}
			}
		}
	}
	return nil
}
