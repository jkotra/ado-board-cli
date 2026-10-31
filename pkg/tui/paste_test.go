package tui

import (
	"fmt"
	"strings"
	"testing"

	"ado-board-cli/pkg/clipboard"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	tea "github.com/charmbracelet/bubbletea"
)

type mockCSISequenceMsg []byte

func (m mockCSISequenceMsg) String() string {
	return fmt.Sprintf("?CSI%+v?", []byte(m)[2:])
}

func TestPasteFilterMultiline(t *testing.T) {
	filter := NewPasteFilter()

	// Simulate Bracketed Paste: Start sequence
	startMsg := mockCSISequenceMsg([]byte{0x1b, '[', '2', '0', '0', '~'})
	res := filter.Filter(nil, startMsg)
	if res != nil {
		t.Fatalf("expected startMsg to be consumed, got %v", res)
	}

	// Keystrokes inside paste
	msgs := []tea.Msg{
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello world")},
		tea.KeyMsg{Type: tea.KeyEnter},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("second line")},
		tea.KeyMsg{Type: tea.KeyCtrlJ},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("third line")},
	}

	for _, m := range msgs {
		r := filter.Filter(nil, m)
		if r != nil {
			t.Fatalf("expected message inside paste to be buffered, got %v", r)
		}
	}

	// End sequence
	endMsg := mockCSISequenceMsg([]byte{0x1b, '[', '2', '0', '1', '~'})
	finalMsg := filter.Filter(nil, endMsg)
	if finalMsg == nil {
		t.Fatalf("expected MsgPaste on end sequence, got nil")
	}

	pasteMsg, ok := finalMsg.(MsgPaste)
	if !ok {
		t.Fatalf("expected type MsgPaste, got %T", finalMsg)
	}

	expected := "hello world\nsecond line\nthird line"
	if pasteMsg.Content != expected {
		t.Fatalf("expected paste content %q, got %q", expected, pasteMsg.Content)
	}
}

func TestPasteFilterPassThrough(t *testing.T) {
	filter := NewPasteFilter()

	// Normal key should pass through untouched
	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	res := filter.Filter(nil, keyMsg)
	resKey, ok := res.(tea.KeyMsg)
	if !ok || resKey.Type != keyMsg.Type {
		t.Fatalf("expected keyMsg to pass through, got %v", res)
	}
}

func TestDockGeminiInsertTextAndAutoExpand(t *testing.T) {
	dock := NewDockGemini("gemini-3.7-flash")
	dock.Show()
	dock.SetSize(100, 30)
	dock.PaneHeight = 8

	// Paste 5 lines with CRLF and CR mixed
	multilineInput := "line 1\r\nline 2\rline 3\nline 4\nline 5"
	dock.InsertText(multilineInput)

	expectedNormalized := "line 1\nline 2\nline 3\nline 4\nline 5"
	if dock.Input.Value() != expectedNormalized {
		t.Fatalf("expected normalized input %q, got %q", expectedNormalized, dock.Input.Value())
	}

	// Auto-expand check: Dock height should have expanded from 8 to comfortably fit multiline text
	if dock.PaneHeight <= 8 {
		t.Fatalf("expected pane height to auto-expand above 8, got %d", dock.PaneHeight)
	}

	// Input textarea height should have expanded
	if dock.Input.Height() < 4 {
		t.Fatalf("expected input textarea height >= 4, got %d", dock.Input.Height())
	}
}

func TestDockGeminiCtrlVPaste(t *testing.T) {
	dock := NewDockGemini("gemini-3.7-flash")
	dock.Show()
	dock.SetSize(100, 30)

	// Set test clipboard contents
	clipboard.LastCopiedPlain = "pasted query\nwith second line"

	// Trigger ctrl+v
	dock.Update(tea.KeyMsg{Type: tea.KeyCtrlV})

	expected := "pasted query\nwith second line"
	if dock.Input.Value() != expected {
		t.Fatalf("expected input %q after ctrl+v, got %q", expected, dock.Input.Value())
	}
}

func TestDockGeminiHistoryNavigationMultilineSafety(t *testing.T) {
	dock := NewDockGemini("gemini-3.7-flash")
	dock.Show()
	dock.SetSize(100, 30)
	dock.History = []string{"previous query 1", "previous query 2"}

	// Insert multiline text
	dock.InsertText("first line\nsecond line\nthird line")

	// Cursor is at end (line 2). Pressing up should move cursor in textarea, NOT replace with history
	dock.Update(tea.KeyMsg{Type: tea.KeyUp})
	if dock.Input.Value() != "first line\nsecond line\nthird line" {
		t.Fatalf("expected input to remain intact after pressing up, got %q", dock.Input.Value())
	}

	// Empty input should trigger history
	dock.Input.Reset()
	dock.Update(tea.KeyMsg{Type: tea.KeyUp})
	if dock.Input.Value() != "previous query 2" {
		t.Fatalf("expected history item 'previous query 2', got %q", dock.Input.Value())
	}
}

func TestAppModelMsgPasteWorkflow(t *testing.T) {
	tempDir := t.TempDir()
	database, err := db.Open(tempDir + "/test_paste.db")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		DefaultOrg:     "my-org",
		DefaultProject: "my-project",
	}

	app := NewAppModel(cfg, database)
	app.DockGemini.Show()
	app.DockGemini.SetSize(100, 30)

	// Send MsgPaste to AppModel
	multilinePaste := "Line 1: Analyze tasks\nLine 2: Assign to user\nLine 3: Estimate 4h"
	updated, _ := app.Update(MsgPaste{Content: multilinePaste})
	resApp := updated.(*AppModel)

	if resApp.DockGemini.Input.Value() != multilinePaste {
		t.Fatalf("expected prompt value %q, got %q", multilinePaste, resApp.DockGemini.Input.Value())
	}

	// Also test ctrl+v shortcut in AppModel
	clipboard.LastCopiedPlain = "pasted via ctrl+v\nline two"
	updated, _ = resApp.Update(tea.KeyMsg{Type: tea.KeyCtrlV})
	resApp = updated.(*AppModel)

	if !strings.Contains(resApp.DockGemini.Input.Value(), "pasted via ctrl+v\nline two") {
		t.Fatalf("expected input to contain ctrl+v clipboard content, got %q", resApp.DockGemini.Input.Value())
	}
}

