package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootCmdDebugFlag(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "cmd_debug.log")

	// Reset flags
	flagDebug = true
	flagLogFile = logPath

	if rootCmd.PersistentPreRunE != nil {
		err := rootCmd.PersistentPreRunE(rootCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error in PersistentPreRunE: %v", err)
		}
	}

	if rootCmd.PersistentPostRun != nil {
		rootCmd.PersistentPostRun(rootCmd, []string{})
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read created log file: %v", err)
	}

	if !strings.Contains(string(data), "Logger initialized") {
		t.Errorf("expected log file to contain 'Logger initialized', got %s", string(data))
	}
}
