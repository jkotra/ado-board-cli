package clipboard

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"html"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	atotto "github.com/atotto/clipboard"
)

// CustomWriter allows overriding clipboard operations for testing or custom handlers
var CustomWriter func(plainText, htmlText string) error

// CustomReader allows overriding clipboard read operations for testing or custom handlers
var CustomReader func() (string, error)

// LastCopiedPlain and LastCopiedHTML store the most recent copied payload (useful in tests)
var (
	LastCopiedPlain string
	LastCopiedHTML  string
)

func isRunningInTest() bool {
	if flag.Lookup("test.v") != nil {
		return true
	}
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-test.") {
			return true
		}
	}
	if len(os.Args) > 0 && (strings.HasSuffix(os.Args[0], ".test") || strings.Contains(os.Args[0], "/_test/")) {
		return true
	}
	return os.Getenv("GO_TEST") == "1"
}

// CopyWorkItem formats a work item as both rich HTML and plain text and copies it to the clipboard
func CopyWorkItem(itemType string, id int, title string, webURL string) (string, error) {
	if itemType == "" {
		itemType = "WorkItem"
	}
	plainText := fmt.Sprintf("%s %d: %s", itemType, id, title)
	escapedTitle := html.EscapeString(title)
	htmlText := fmt.Sprintf(`<a href="%s">%s %d</a>: %s`, webURL, itemType, id, escapedTitle)

	err := Write(plainText, htmlText)
	return plainText, err
}

// Write writes both plain text and HTML to the clipboard using the best available platform tool
func Write(plainText, htmlText string) error {
	LastCopiedPlain = plainText
	LastCopiedHTML = htmlText

	if CustomWriter != nil {
		return CustomWriter(plainText, htmlText)
	}

	if isRunningInTest() {
		return nil
	}

	var err error
	switch runtime.GOOS {
	case "linux":
		err = writeLinux(plainText, htmlText)
	case "darwin":
		err = writeDarwin(plainText, htmlText)
	case "windows":
		err = writeWindows(plainText, htmlText)
	default:
		err = atotto.WriteAll(plainText)
	}

	if err != nil {
		// Fallback to basic plain-text clipboard library
		return atotto.WriteAll(plainText)
	}
	return nil
}

func writeLinux(plainText, htmlText string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Try wl-copy (Wayland) first if available
	if _, err := exec.LookPath("wl-copy"); err == nil {
		cmd := exec.CommandContext(ctx, "wl-copy", "-t", "text/html")
		cmd.Stdin = strings.NewReader(htmlText)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try xclip (X11) if available
	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-t", "text/html")
		cmd.Stdin = strings.NewReader(htmlText)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	// Try xsel (X11) if available
	if _, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.CommandContext(ctx, "xsel", "--clipboard", "--input")
		cmd.Stdin = strings.NewReader(plainText)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return atotto.WriteAll(plainText)
}

func writeDarwin(plainText, htmlText string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Try AppleScript with rich HTML format
	script := fmt.Sprintf(`set the clipboard to {«class HTML»: "%s", unicode text: "%s"}`,
		strings.ReplaceAll(htmlText, `"`, `\"`),
		strings.ReplaceAll(plainText, `"`, `\"`),
	)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Fallback to pbcopy with plain text
	if _, err := exec.LookPath("pbcopy"); err == nil {
		pbCmd := exec.CommandContext(ctx, "pbcopy")
		pbCmd.Stdin = strings.NewReader(plainText)
		if err := pbCmd.Run(); err == nil {
			return nil
		}
	}

	return atotto.WriteAll(plainText)
}

func writeWindows(plainText, htmlText string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Try PowerShell to set both HTML and PlainText clipboard data
	psScript := fmt.Sprintf(`
Add-Type -AssemblyName System.Windows.Forms
$obj = New-Object System.Windows.Forms.DataObject
$obj.SetData([System.Windows.Forms.DataFormats]::Html, @"
%s
"@)
$obj.SetData([System.Windows.Forms.DataFormats]::UnicodeText, @"
%s
"@)
[System.Windows.Forms.Clipboard]::SetDataObject($obj, $true)
`, htmlText, plainText)

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		return nil
	}

	return atotto.WriteAll(plainText)
}

// Read reads plain text from the clipboard using the best available platform tool
func Read() (string, error) {
	var text string
	var err error

	if CustomReader != nil {
		text, err = CustomReader()
	} else if isRunningInTest() {
		text, err = LastCopiedPlain, nil
	} else {
		switch runtime.GOOS {
		case "linux":
			text, err = readLinux()
		case "darwin":
			text, err = readDarwin()
		case "windows":
			text, err = readWindows()
		default:
			text, err = atotto.ReadAll()
		}

		if err != nil {
			text, err = atotto.ReadAll()
		}
	}

	if err != nil {
		return "", err
	}

	// Normalize line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text, nil
}

func readLinux() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if _, err := exec.LookPath("wl-paste"); err == nil {
		cmd := exec.CommandContext(ctx, "wl-paste", "-t", "text/plain")
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
		cmd2 := exec.CommandContext(ctx, "wl-paste")
		if out, err = cmd2.Output(); err == nil {
			return string(out), nil
		}
	}

	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.CommandContext(ctx, "xclip", "-selection", "clipboard", "-o")
		if out, err := cmd.Output(); err == nil {
			return string(out), nil
		}
	}

	if _, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.CommandContext(ctx, "xsel", "--clipboard", "--output")
		if out, err := cmd.Output(); err == nil {
			return string(out), nil
		}
	}

	return atotto.ReadAll()
}

func readDarwin() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if _, err := exec.LookPath("pbpaste"); err == nil {
		cmd := exec.CommandContext(ctx, "pbpaste")
		if out, err := cmd.Output(); err == nil {
			return string(out), nil
		}
	}

	return atotto.ReadAll()
}

func readWindows() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", "Get-Clipboard")
	if out, err := cmd.Output(); err == nil {
		return string(out), nil
	}

	return atotto.ReadAll()
}
