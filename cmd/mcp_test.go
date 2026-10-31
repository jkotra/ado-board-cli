package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPSkillAndConfigCommands(t *testing.T) {
	// 1. Test 'skill' subcommand output
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"mcp", "skill"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("mcp skill failed: %v", err)
	}

	// 2. Test 'config' subcommand output
	buf.Reset()
	rootCmd.SetArgs([]string{"mcp", "config"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("mcp config failed: %v", err)
	}
}

func TestMCPSetupCommandExecution(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)

	// 1. Test --global setup
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"mcp", "setup", "--agents", "--deepseek", "--global"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("mcp setup --agents --deepseek --global failed: %v", err)
	}

	skillFile := filepath.Join(tempHome, ".gemini", "config", "skills", "ado-board", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("expected SKILL.md to be created at %s: %v", skillFile, err)
	}

	mcpFile := filepath.Join(tempHome, ".gemini", "config", "mcp_config.json")
	if _, err := os.Stat(mcpFile); err != nil {
		t.Fatalf("expected mcp_config.json to be created at %s: %v", mcpFile, err)
	}

	data, _ := os.ReadFile(mcpFile)
	if !strings.Contains(string(data), "ado-board") {
		t.Fatalf("expected ado-board in mcp_config.json: %s", string(data))
	}

	dshSkillFile := filepath.Join(tempHome, ".dsh", "skills", "ado-board", "SKILL.md")
	if _, err := os.Stat(dshSkillFile); err != nil {
		t.Fatalf("expected DeepSeek SKILL.md to be created at %s: %v", dshSkillFile, err)
	}

	dshPatchFile := filepath.Join(tempHome, ".dsh", "cordis.patch.yml")
	if _, err := os.Stat(dshPatchFile); err != nil {
		t.Fatalf("expected cordis.patch.yml at %s: %v", dshPatchFile, err)
	}
	dshData, _ := os.ReadFile(dshPatchFile)
	if !strings.Contains(string(dshData), "@deepseek-ai/dsh-mcp-client") {
		t.Fatalf("expected @deepseek-ai/dsh-mcp-client in cordis.patch.yml: %s", string(dshData))
	}

	// 2. Test mandatory target integration validation (error on missing target)
	localDir := t.TempDir()
	origWd, _ := os.Getwd()
	_ = os.Chdir(localDir)
	defer func() { _ = os.Chdir(origWd) }()

	// Reset global and platform flags
	flagSetupGlobal = false
	flagSetupAgents = false
	flagSetupDeepseek = false
	flagSetupClaude = false

	buf.Reset()
	rootCmd.SetArgs([]string{"mcp", "setup"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error when running mcp setup without target integration, but got nil")
	}

	// 3. Test rejection of 'all' target
	buf.Reset()
	rootCmd.SetArgs([]string{"mcp", "setup", "all"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error when running mcp setup all, but got nil")
	}

	// 4. Test positional argument setup (e.g. 'mcp setup claude', 'mcp setup agents', 'mcp setup deepseek')
	buf.Reset()
	rootCmd.SetArgs([]string{"mcp", "setup", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("mcp setup claude failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localDir, ".mcp.json")); err != nil {
		t.Fatalf("expected local .mcp.json: %v", err)
	}

	buf.Reset()
	rootCmd.SetArgs([]string{"mcp", "setup", "agents"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("mcp setup agents failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localDir, ".gemini", "skills", "ado-board", "SKILL.md")); err != nil {
		t.Fatalf("expected local SKILL.md: %v", err)
	}

	buf.Reset()
	rootCmd.SetArgs([]string{"mcp", "setup", "deepseek"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("mcp setup deepseek failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localDir, ".dsh", "skills", "ado-board", "SKILL.md")); err != nil {
		t.Fatalf("expected local .dsh SKILL.md: %v", err)
	}

	buf.Reset()
	rootCmd.SetArgs([]string{"mcp", "setup", "opencode"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("mcp setup opencode failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(localDir, ".opencode", "skills", "ado-board", "SKILL.md")); err != nil {
		t.Fatalf("expected local opencode SKILL.md: %v", err)
	}
	ocData, err := os.ReadFile(filepath.Join(localDir, "opencode.json"))
	if err != nil {
		t.Fatalf("expected local opencode.json: %v", err)
	}
	if !strings.Contains(string(ocData), `"type": "local"`) || !strings.Contains(string(ocData), `"ado-board"`) {
		t.Fatalf("unexpected opencode.json: %s", string(ocData))
	}
}
