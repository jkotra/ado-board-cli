package version

import (
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	if Version != "0.05" {
		t.Errorf("expected Version '0.05', got %q", Version)
	}

	str := String()
	if !strings.HasPrefix(str, "0.05") {
		t.Errorf("expected String() to start with '0.05', got %q", str)
	}

	// Test with GitCommit
	GitCommit = "abcdef1"
	strCommit := String()
	if strCommit != "0.05 (abcdef1)" {
		t.Errorf("expected '0.05 (abcdef1)', got %q", strCommit)
	}
	GitCommit = ""
}
