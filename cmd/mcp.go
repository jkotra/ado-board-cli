package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
	"ado-board-cli/pkg/mcp"

	"github.com/spf13/cobra"
)

var (
	flagSetupClaude   bool
	flagSetupAgents   bool
	flagSetupDeepseek bool
	flagSetupOpencode bool
	flagSetupGlobal   bool
	flagSetupLocal    bool
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "🤖 Run or configure Model Context Protocol (MCP) server for Claude, DeepSeek Harness & AI Agents",
	Long: `Run the ado-board-cli MCP server over stdio to enable Claude Desktop,
Claude Code CLI, DeepSeek Harness, Antigravity, and AI Agents to manage Azure DevOps work items
and interact with the live TUI application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(flagDbPath, flagToken)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		if flagOrg != "" {
			cfg.DefaultOrg = flagOrg
		}
		if flagProject != "" {
			cfg.DefaultProject = flagProject
		}

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()

		// Get PAT token from config or database settings
		pat := cfg.PatToken
		if pat == "" {
			if dbPat, err := database.GetSetting("pat_token"); err == nil && dbPat != "" {
				pat = dbPat
			}
		}

		org := cfg.DefaultOrg
		if org == "" {
			if dbOrg, err := database.GetSetting("default_org"); err == nil && dbOrg != "" {
				org = dbOrg
			}
		}

		proj := cfg.DefaultProject
		if proj == "" {
			if dbProj, err := database.GetSetting("default_project"); err == nil && dbProj != "" {
				proj = dbProj
			}
		}

		var client *ado.Client
		if pat != "" && org != "" {
			client = ado.NewClient(org, proj, pat)
		}

		return mcp.RunStdioServer(client, database, cfg)
	},
}

var mcpSetupCmd = &cobra.Command{
	Use:   "setup [claude|agents|deepseek|opencode]",
	Short: "⚙️ Automatically configure Claude, DeepSeek Harness, opencode, or AI Agents to use ado-board-cli",
	Long: `Automatically registers ado-board-cli in the specified AI integration:
  • claude:   Claude (.mcp.json / .claude.json)
  • agents:   Antigravity & AI Agents (.gemini/ & .agent/)
  • deepseek: DeepSeek Harness (cordis.patch.yml & SKILL.md)
  • opencode: opencode (opencode.json & .opencode/skills/)

Local workspace configuration is written by default.
Use --global (-g) to configure user-wide environment (~/.dsh, ~/.gemini, ~/.claude.json, ~/.config/opencode) instead.

Examples:
  ado-board-cli mcp setup claude --global
  ado-board-cli mcp setup deepseek
  ado-board-cli mcp setup opencode
  ado-board-cli mcp setup agents`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var runClaude, runAgents, runDeepseek, runOpencode bool

		for _, arg := range args {
			switch strings.ToLower(arg) {
			case "claude":
				runClaude = true
			case "agents", "agent", "antigravity", "gemini":
				runAgents = true
			case "deepseek", "dsh":
				runDeepseek = true
			case "opencode":
				runOpencode = true
			default:
				return fmt.Errorf("unknown integration target %q. Supported targets: claude, agents, deepseek, opencode", arg)
			}
		}

		if flagSetupClaude {
			runClaude = true
		}
		if flagSetupAgents {
			runAgents = true
		}
		if flagSetupDeepseek {
			runDeepseek = true
		}

		if flagSetupOpencode {
			runOpencode = true
		}

		if !runClaude && !runAgents && !runDeepseek && !runOpencode {
			return fmt.Errorf("please specify a target integration to configure: 'claude', 'agents', 'deepseek', or 'opencode'\n\nExamples:\n  ado-board-cli mcp setup claude --global\n  ado-board-cli mcp setup deepseek\n  ado-board-cli mcp setup opencode\n  ado-board-cli mcp setup agents")
		}

		binPath, err := os.Executable()
		if err != nil {
			binPath = "ado-board-cli"
		}

		// Local is default unless --global is explicitly requested
		isLocal := true
		if flagSetupGlobal {
			isLocal = false
		}

		scopeText := "Local Workspace"
		if !isLocal {
			scopeText = "Global User Environment"
		}
		fmt.Printf("🚀 Configuring ado-board-cli MCP Integration (%s)...\n\n", scopeText)

		if runClaude {
			fmt.Println("📦 Configuring Claude...")
			claudeRes, err := mcp.SetupClaude(binPath, isLocal)
			if err != nil {
				fmt.Printf("❌ Failed to configure Claude: %v\n\n", err)
			} else {
				fmt.Printf("%s\n\n", claudeRes)
			}
		}

		if runAgents {
			fmt.Println("🤖 Configuring AI Agents & Antigravity...")
			agentsRes, err := mcp.SetupAgents(binPath, isLocal)
			if err != nil {
				fmt.Printf("❌ Failed to configure Agents: %v\n\n", err)
			} else {
				fmt.Printf("%s\n\n", agentsRes)
			}
		}

		if runDeepseek {
			fmt.Println("🐋 Configuring DeepSeek Harness...")
			deepseekRes, err := mcp.SetupDeepseek(binPath, isLocal)
			if err != nil {
				fmt.Printf("❌ Failed to configure DeepSeek Harness: %v\n\n", err)
			} else {
				fmt.Printf("%s\n\n", deepseekRes)
			}
		}

		if runOpencode {
			fmt.Println("🧩 Configuring opencode...")
			opencodeRes, err := mcp.SetupOpencode(binPath, isLocal)
			if err != nil {
				fmt.Printf("❌ Failed to configure opencode: %v\n\n", err)
			} else {
				fmt.Printf("%s\n\n", opencodeRes)
			}
		}

		fmt.Println("🎉 Setup complete! You can now ask your AI assistant to manage ADO work items.")
		return nil
	},
}

var mcpSkillCmd = &cobra.Command{
	Use:   "skill",
	Short: "📖 Display the embedded SKILL.md documentation",
	Long:  "Prints the complete embedded SKILL.md guide for AI agents and documentation.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(mcp.EmbeddedSkillMarkdown)
	},
}

var mcpConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "📋 Print MCP configuration snippets for manual setup",
	Long:  "Outputs copy-pasteable JSON configuration for Claude Desktop / Antigravity and YAML patch configuration for DeepSeek Harness.",
	RunE: func(cmd *cobra.Command, args []string) error {
		binPath, err := os.Executable()
		if err != nil {
			binPath = "ado-board-cli"
		}

		cfgObj := map[string]any{
			"mcpServers": map[string]any{
				"ado-board": map[string]any{
					"command": binPath,
					"args":    []string{"mcp"},
				},
			},
		}

		out, err := json.MarshalIndent(cfgObj, "", "  ")
		if err != nil {
			return err
		}

		yamlSnippet := fmt.Sprintf(`- insert:
    - id: mcp-ado-board
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        transport: stdio
        serverName: ado-board
        command: %s
        args:
          - mcp`, binPath)

		fmt.Printf("// JSON Configuration (.mcp.json, claude_desktop_config.json, ~/.gemini/config/mcp_config.json):\n%s\n\n", string(out))
		fmt.Printf("# DeepSeek Harness Configuration (cordis.patch.yml / ~/.dsh/cordis.patch.yml):\n%s\n", yamlSnippet)
		return nil
	},
}

func init() {
	mcpSetupCmd.Flags().BoolVar(&flagSetupClaude, "claude", false, "Configure Claude Desktop & Claude Code")
	mcpSetupCmd.Flags().BoolVar(&flagSetupAgents, "agents", false, "Install embedded SKILL.md and register MCP for Agents")
	mcpSetupCmd.Flags().BoolVar(&flagSetupDeepseek, "deepseek", false, "Configure DeepSeek Harness (cordis.patch.yml & SKILL.md)")
	mcpSetupCmd.Flags().BoolVar(&flagSetupDeepseek, "dsh", false, "Alias for --deepseek")
	mcpSetupCmd.Flags().BoolVar(&flagSetupOpencode, "opencode", false, "Configure opencode (opencode.json & SKILL.md)")
	mcpSetupCmd.Flags().BoolVarP(&flagSetupGlobal, "global", "g", false, "Configure global user environment (~/.dsh, ~/.gemini, ~/.claude.json) instead of local workspace")
	mcpSetupCmd.Flags().BoolVarP(&flagSetupLocal, "local", "l", true, "Configure local workspace (.dsh/, .gemini/ & .mcp.json) [default]")

	mcpCmd.AddCommand(mcpSetupCmd)
	mcpCmd.AddCommand(mcpSkillCmd)
	mcpCmd.AddCommand(mcpConfigCmd)

	rootCmd.AddCommand(mcpCmd)
}
