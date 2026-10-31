//go:build !windows

package tui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

// GetTerminalSize is a no-op on non-Windows (POSIX) systems where Bubble Tea natively receives SIGWINCH.
func GetTerminalSize() (int, int, error) {
	return 0, 0, errors.New("terminal sizing managed natively by POSIX SIGWINCH")
}

// checkTerminalResize is a no-op on non-Windows (POSIX) systems with zero syscall/polling overhead.
func (m *AppModel) checkTerminalResize() tea.Cmd {
	return nil
}
