package cmd

import (
	"fmt"
	"os"
	"strings"

	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Update a configuration setting in the database",
	Long: `Update a configuration setting stored in the SQLite database.

Supported setting keys:
  pat-token              Azure DevOps Personal Access Token (stored in DB, falls back to PAT_TOKEN env var if empty)
  gemini-api-key         Google Gemini API Key
  gemini-model           Google Gemini Model (e.g. 'gemini-3.8-flash', 'gemini-2.5-flash')
  open-in-browser-cmd    Command template to open URLs (use '{url}' placeholder, or leave empty for system default)
  default-org            Default Azure DevOps organization
  default-project        Default Azure DevOps project

Examples:
  ado-board-cli set pat-token <token>
  ado-board-cli set pat-token ''            # Clear db value to fallback to PAT_TOKEN env var
  ado-board-cli set gemini-api-key AIzaSy...
  ado-board-cli set gemini-model gemini-3.8-flash
  ado-board-cli set open-in-browser-cmd 'google-chrome --profile-directory="Profile 1" "{url}"'
  ado-board-cli set open-in-browser-cmd ''  # Reset to OS default browser
  ado-board-cli set default-org my-org
  ado-board-cli set default-project my-project`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := strings.Join(args[1:], " ")

		cfg, err := config.Load(flagDbPath, flagToken)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()

		if err := database.SetSetting(key, value); err != nil {
			return fmt.Errorf("failed to save setting: %w", err)
		}

		normKey := db.NormalizeSettingKey(key)
		displayVal := fmt.Sprintf("%q", value)
		if (strings.Contains(normKey, "token") || strings.Contains(normKey, "key")) && len(value) > 8 {
			displayVal = fmt.Sprintf("%q", value[:4]+"..."+value[len(value)-4:])
		}
		fmt.Printf("✓ Successfully updated setting %s = %s\n",
			lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true).Render(normKey),
			lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render(displayVal),
		)
		return nil
	},
}

var getCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "View configuration settings stored in the database",
	Long: `View one or all configuration settings stored in the SQLite database.

Examples:
  ado-board-cli get
  ado-board-cli get pat-token
  ado-board-cli get gemini-model
  ado-board-cli get open-in-browser-cmd`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(flagDbPath, flagToken)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		database, err := db.Open(cfg.DbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer database.Close()

		if len(args) > 0 {
			key := args[0]
			normKey := db.NormalizeSettingKey(key)
			val, err := database.GetSetting(key)
			if err != nil {
				if normKey == "pat_token" {
					if envToken := os.Getenv("PAT_TOKEN"); envToken != "" {
						fmt.Println("(not configured in db, using PAT_TOKEN environment variable)")
						return nil
					}
				} else if normKey == "gemini_api_key" {
					if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
						fmt.Println("(not configured in db, using GEMINI_API_KEY environment variable)")
						return nil
					}
				}
				return fmt.Errorf("setting %q not found in database", key)
			}
			if strings.TrimSpace(val) == "" {
				if normKey == "open_in_browser_cmd" {
					fmt.Println("(not configured, using system default browser)")
				} else if normKey == "pat_token" {
					if envToken := os.Getenv("PAT_TOKEN"); envToken != "" {
						fmt.Println("(empty in db, using PAT_TOKEN environment variable)")
					} else {
						fmt.Println("(empty in db, no PAT_TOKEN environment variable set)")
					}
				} else if normKey == "gemini_api_key" {
					if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
						fmt.Println("(empty in db, using GEMINI_API_KEY environment variable)")
					} else {
						fmt.Println("(empty in db, no GEMINI_API_KEY environment variable set)")
					}
				} else {
					fmt.Println("(empty)")
				}
				return nil
			}
			if strings.Contains(normKey, "token") || strings.Contains(normKey, "key") {
				if len(val) > 8 {
					fmt.Println(val[:4] + "..." + val[len(val)-4:])
					return nil
				}
			}
			fmt.Println(val)
			return nil
		}

		settings, err := database.GetAllSettings()
		if err != nil {
			return fmt.Errorf("failed to retrieve settings: %w", err)
		}

		if len(settings) == 0 {
			fmt.Println("No settings stored in database.")
			return nil
		}

		headerKey := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("SETTING KEY")
		headerVal := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Render("VALUE")

		fmt.Printf("%-24s  %s\n", headerKey, headerVal)
		fmt.Println(strings.Repeat("─", 80))

		for k, v := range settings {
			displayVal := v
			if strings.TrimSpace(v) == "" {
				if k == "open_in_browser_cmd" {
					displayVal = "(system default browser)"
				} else if k == "pat_token" {
					if envToken := os.Getenv("PAT_TOKEN"); envToken != "" {
						displayVal = "(using PAT_TOKEN env var)"
					} else {
						displayVal = "(not set)"
					}
				} else if k == "gemini_api_key" {
					if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
						displayVal = "(using GEMINI_API_KEY env var)"
					} else {
						displayVal = "(not set)"
					}
				} else {
					displayVal = "(empty)"
				}
			} else if strings.Contains(k, "key") || strings.Contains(k, "token") {
				if len(v) > 12 {
					displayVal = v[:4] + "..." + v[len(v)-4:]
				}
			}
			keyStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true).Render(k)
			valColor := "220"
			if strings.HasPrefix(displayVal, "(") {
				valColor = "244"
			}
			valStyled := lipgloss.NewStyle().Foreground(lipgloss.Color(valColor)).Render(displayVal)
			fmt.Printf("%-34s  %s\n", keyStyled, valStyled)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(getCmd)
}
