package cmd

import (
	"bytes"
	"strings"
	"testing"

	"ado-board-cli/pkg/version"
)

func TestVersionCommand(t *testing.T) {
	buf := new(bytes.Buffer)
	versionCmd.SetOut(buf)
	versionCmd.Run(versionCmd, []string{})

	if !strings.Contains(version.Version, "0.05") {
		t.Errorf("expected version to be 0.05, got %q", version.Version)
	}
}

func TestRootCmdHelpIncludesVersion(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)

	err := rootCmd.Help()
	if err != nil {
		t.Fatalf("unexpected error getting help: %v", err)
	}

	helpText := buf.String()
	if !strings.Contains(helpText, version.Version) {
		t.Errorf("expected help output to contain version %q, got:\n%s", version.Version, helpText)
	}
	if !strings.Contains(helpText, "Version:") {
		t.Errorf("expected help output to contain 'Version:', got:\n%s", helpText)
	}
}
