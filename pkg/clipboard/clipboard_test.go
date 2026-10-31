package clipboard

import (
	"testing"
)

func TestCopyWorkItemFormat(t *testing.T) {
	plainText, err := CopyWorkItem("Task", 666613, "sample task & demo", "https://dev.azure.com/myorg/myproj/_workitems/edit/666613")
	// In test environment without display server, err might fall back to atotto or be nil
	_ = err

	expectedPlain := "Task 666613: sample task & demo"
	if plainText != expectedPlain {
		t.Fatalf("expected plainText %q, got %q", expectedPlain, plainText)
	}
}

func TestCopyWorkItemDefaultType(t *testing.T) {
	plainText, _ := CopyWorkItem("", 12345, "untitled item", "https://dev.azure.com/myorg/myproj/_workitems/edit/12345")
	expectedPlain := "WorkItem 12345: untitled item"
	if plainText != expectedPlain {
		t.Fatalf("expected plainText %q, got %q", expectedPlain, plainText)
	}
}

func TestClipboardReadCustomReader(t *testing.T) {
	oldReader := CustomReader
	defer func() { CustomReader = oldReader }()

	CustomReader = func() (string, error) {
		return "line 1\r\nline 2\rline 3\nline 4", nil
	}

	text, err := Read()
	if err != nil {
		t.Fatalf("unexpected error reading clipboard: %v", err)
	}

	expected := "line 1\nline 2\nline 3\nline 4"
	if text != expected {
		t.Fatalf("expected normalized newlines %q, got %q", expected, text)
	}
}

func TestClipboardReadInTest(t *testing.T) {
	oldReader := CustomReader
	CustomReader = nil
	defer func() { CustomReader = oldReader }()

	LastCopiedPlain = "multiline\npaste\ntest"
	text, err := Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "multiline\npaste\ntest" {
		t.Fatalf("expected %q, got %q", "multiline\npaste\ntest", text)
	}
}
