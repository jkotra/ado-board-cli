package tui

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// MsgPaste represents pasted text from terminal bracketed paste or clipboard
type MsgPaste struct {
	Content string
}

// PasteFilter intercepts terminal bracketed paste escape sequences:
// Start: \x1b[200~
// End:   \x1b[201~
//
// In Bubble Tea v0.25.0, unrecognized CSI sequences are delivered as unknownCSISequenceMsg ([]byte).
// During paste, all keystrokes and newlines are buffered and emitted as a single MsgPaste
// once the end sequence is received. This prevents multiline pastes from prematurely triggering Enter.
type PasteFilter struct {
	inPaste bool
	buf     strings.Builder
}

// NewPasteFilter creates a new bracketed paste message filter
func NewPasteFilter() *PasteFilter {
	return &PasteFilter{}
}

var (
	pasteStartBytes = []byte{0x1b, '[', '2', '0', '0', '~'}
	pasteEndBytes   = []byte{0x1b, '[', '2', '0', '1', '~'}
)

func isPasteStartMsg(msg tea.Msg) bool {
	if msg == nil {
		return false
	}
	// 1. Direct byte slice comparison (unknownCSISequenceMsg underlying type is []byte)
	val := reflect.ValueOf(msg)
	if val.IsValid() && val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8 {
		b := val.Bytes()
		if bytes.Equal(b, pasteStartBytes) {
			return true
		}
	}

	// 2. Stringer representation check ("?CSI[50 48 48 126]?" or "\x1b[200~")
	if s, ok := msg.(fmt.Stringer); ok {
		str := s.String()
		if str == "\x1b[200~" || strings.Contains(str, "50 48 48 126") || strings.Contains(str, "[200~") {
			return true
		}
	}
	return false
}

func isPasteEndMsg(msg tea.Msg) bool {
	if msg == nil {
		return false
	}
	// 1. Direct byte slice comparison
	val := reflect.ValueOf(msg)
	if val.IsValid() && val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Uint8 {
		b := val.Bytes()
		if bytes.Equal(b, pasteEndBytes) {
			return true
		}
	}

	// 2. Stringer representation check ("?CSI[50 48 49 126]?" or "\x1b[201~")
	if s, ok := msg.(fmt.Stringer); ok {
		str := s.String()
		if str == "\x1b[201~" || strings.Contains(str, "50 48 49 126") || strings.Contains(str, "[201~") {
			return true
		}
	}
	return false
}

// Filter filters incoming Bubble Tea messages, capturing bracketed paste blocks
func (pf *PasteFilter) Filter(m tea.Model, msg tea.Msg) tea.Msg {
	if isPasteStartMsg(msg) {
		pf.inPaste = true
		pf.buf.Reset()
		return nil
	}

	if isPasteEndMsg(msg) {
		if pf.inPaste {
			pf.inPaste = false
			content := pf.buf.String()
			pf.buf.Reset()
			if content != "" {
				// Normalize line endings
				content = strings.ReplaceAll(content, "\r\n", "\n")
				content = strings.ReplaceAll(content, "\r", "\n")
				return MsgPaste{Content: content}
			}
		}
		return nil
	}

	if pf.inPaste {
		switch k := msg.(type) {
		case tea.KeyMsg:
			switch k.Type {
			case tea.KeyEnter:
				pf.buf.WriteString("\n")
			case tea.KeyCtrlJ:
				pf.buf.WriteString("\n")
			case tea.KeyTab:
				pf.buf.WriteString("\t")
			case tea.KeySpace:
				pf.buf.WriteString(" ")
			case tea.KeyRunes:
				pf.buf.WriteString(string(k.Runes))
			default:
				if str := k.String(); len(str) == 1 {
					pf.buf.WriteString(str)
				}
			}
			return nil
		}
		// Drop other unknown messages (like intermediate escape sequences) while inside paste block
		return nil
	}

	return msg
}
