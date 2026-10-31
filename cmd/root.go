package cmd

import (
	"fmt"
	"os"

	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
	"ado-board-cli/pkg/logger"
	"ado-board-cli/pkg/tui"
	"ado-board-cli/pkg/version"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	flagDbPath   string
	flagToken    string
	flagOrg      string
	flagProject  string
	flagNoResume bool
	flagDebug    bool
	flagLogFile  string
)

var rootCmd = &cobra.Command{
	Use:     "ado-board-cli",
	Short:   "⚡ Azure DevOps Work Item Tree & Board CLI",
	Version: version.String(),
	Long: fmt.Sprintf(`ado-board-cli (v%s) is a powerful terminal interface and CLI tool to track, view,
create, edit, assign, and manage Azure DevOps work item hierarchies.

Version:
  %s

Author:
  Jagadeesh Kotra <jagadeesh@stdin.top>

Run 'ado-board-cli' to launch the interactive Bubble Tea TUI.
Run 'ado-board-cli track <link>' to track an item and its child tree.`, version.Version, version.String()),
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := logger.InitLogger(flagDebug, flagLogFile); err != nil {
			return fmt.Errorf("failed to initialize logger: %w", err)
		}
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		logger.Close()
	},
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
		cfg.NoResume = flagNoResume

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()

		model := tui.NewAppModel(cfg, database)
		defer model.Close()

		// Enable Bracketed Paste Mode in terminal (disabled on exit)
		os.Stdout.WriteString("\x1b[?2004h")
		defer os.Stdout.WriteString("\x1b[?2004l")

		pasteFilter := tui.NewPasteFilter()
		p := tea.NewProgram(
			model,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
			tea.WithFilter(pasteFilter.Filter),
		)

		if _, err := p.Run(); err != nil {
			return fmt.Errorf("TUI execution error: %w", err)
		}

		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Disable Cobra's default Windows Explorer mousetrap warning so double-clicking
	// launches the interactive TUI smoothly in modern Windows Terminal / consoles.
	cobra.MousetrapHelpText = ""

	rootCmd.PersistentFlags().StringVar(&flagDbPath, "db", "", "Path to SQLite database file")
	rootCmd.PersistentFlags().StringVar(&flagToken, "token", "", "Azure DevOps Personal Access Token (PAT)")
	rootCmd.PersistentFlags().StringVar(&flagOrg, "org", "", "Default Azure DevOps organization")
	rootCmd.PersistentFlags().StringVar(&flagProject, "project", "", "Default Azure DevOps project")
	rootCmd.PersistentFlags().BoolVarP(&flagDebug, "debug", "d", false, "Enable structured debug logging to file")
	rootCmd.PersistentFlags().StringVar(&flagLogFile, "log-file", "", "Custom path for debug log file (defaults to ~/.ado-board-cli/debug.log)")
	rootCmd.Flags().BoolVar(&flagNoResume, "no-resume", false, "Skip restoring previously saved UI state (tab, item selection, dock height, filters, collapsed nodes)")
}
