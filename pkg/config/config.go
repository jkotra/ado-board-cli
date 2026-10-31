package config

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ado-board-cli/pkg/ado"
	_ "modernc.org/sqlite"
)

// Config holds the application configuration
type Config struct {
	PatToken             string `json:"pat_token"`
	DefaultOrg           string `json:"default_org"`
	DefaultProject       string `json:"default_project"`
	DbPath               string `json:"db_path"`
	GeminiApiKey         string `json:"gemini_api_key"`
	GeminiModel          string `json:"gemini_model"`
	OpenInBrowserCmd     string `json:"open_in_browser_cmd"`
	NoResume                     bool   `json:"no_resume"`
	AutoCalcRemaining    bool   `json:"auto_calc_remaining"`
	EffortSameAsEstimate bool   `json:"effort_same_as_estimate"`
	AutoSetFinishDate    bool   `json:"auto_set_finish_date"`
	AutoSyncIntervalSeconds int `json:"auto_sync_interval_seconds"`
	// Incomplete View Criteria settings (none enabled by default)
	IncompleteCriteriaEffort     bool `json:"incomplete_criteria_effort"`
	IncompleteCriteriaEstimate   bool `json:"incomplete_criteria_estimate"`
	IncompleteCriteriaStartDate  bool `json:"incomplete_criteria_start_date"`
	IncompleteCriteriaTargetDate bool `json:"incomplete_criteria_target_date"`
	IncompleteCriteriaFinishDate bool     `json:"incomplete_criteria_finish_date"`
	IncompleteTypes              []string `json:"incomplete_types"`
	IncompleteStatuses           []string `json:"incomplete_statuses"`
	IncompleteDateStatuses       []string `json:"incomplete_date_statuses"`
	IncompleteTypeTasks          bool     `json:"incomplete_type_tasks"`
	IncompleteTypeBugs           bool     `json:"incomplete_type_bugs"`
	IncompleteTypePBI            bool     `json:"incomplete_type_pbi"`
	TagRegex                     string   `json:"tag_regex"`
	TagRegexes                   []string `json:"tag_regexes,omitempty"`
	IncompleteCriteriaTagRegex   bool     `json:"incomplete_criteria_tag_regex"`
	IncompleteCriteriaActivity   bool     `json:"incomplete_criteria_activity"`
}

var globalConfig *Config

// Default constants
const (
	DefaultGeminiModel = "gemini-3.8-flash"
	DefaultOrg         = "my-org"
	DefaultProject     = "my-project"
	DefaultBrowserCmd  = ""
)

// GetAppDir returns the directory path for storing application data
func GetAppDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	appDir := filepath.Join(home, ".ado-board-cli")
	_ = os.MkdirAll(appDir, 0755)
	return appDir
}

// GetDefaultDbPath returns the default SQLite database path
func GetDefaultDbPath() string {
	return filepath.Join(GetAppDir(), "ado.db")
}

// Load loads configuration from SQLite app_settings and environment variables
func Load(dbPathOverride, tokenOverride string) (*Config, error) {
	cfg := &Config{
		DbPath:                  GetDefaultDbPath(),
		DefaultOrg:              DefaultOrg,
		DefaultProject:          DefaultProject,
		PatToken:                "",
		GeminiApiKey:            "",
		GeminiModel:             DefaultGeminiModel,
		OpenInBrowserCmd:        DefaultBrowserCmd,
		AutoCalcRemaining:            true,
		EffortSameAsEstimate:         false,
		AutoSetFinishDate:            true,
		AutoSyncIntervalSeconds:      120,
		IncompleteCriteriaEffort:     true,
		IncompleteCriteriaEstimate:   true,
		IncompleteCriteriaStartDate:  true,
		IncompleteCriteriaTargetDate: true,
		IncompleteCriteriaFinishDate: false,
		IncompleteTypeTasks:          true,
		IncompleteTypeBugs:           true,
		IncompleteTypePBI:            false,
		IncompleteTypes:              []string{"Task", "Bug"},
		IncompleteStatuses:           []string{"New", "Active", "To Do", "In Progress", "Committed", "Resolved", "Done", "Closed"},
		IncompleteDateStatuses:       []string{"In Progress", "Active", "Committed", "Resolved", "Done", "Closed"},
	}

	if dbPathOverride != "" {
		cfg.DbPath = dbPathOverride
	}

	autoSyncSetFromDB := false

	// Read settings from SQLite if database file exists
	if _, err := os.Stat(cfg.DbPath); err == nil {
		dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", filepath.ToSlash(cfg.DbPath))
		if conn, err := sql.Open("sqlite", dsn); err == nil {
			rows, err := conn.Query("SELECT key, value FROM app_settings")
			if err == nil {
				for rows.Next() {
					var k, v string
					if err := rows.Scan(&k, &v); err == nil && v != "" {
						cleanV := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(v, "\r", ""), "\n", ""))
						normKey := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(k)), "-", "_")
						switch normKey {
						case "pat_token":
							cfg.PatToken = cleanV
						case "default_org":
							cfg.DefaultOrg = cleanV
						case "default_project":
							cfg.DefaultProject = cleanV
						case "gemini_api_key":
							cfg.GeminiApiKey = cleanV
						case "gemini_model":
							cfg.GeminiModel = cleanV
						case "open_in_browser_cmd":
							cfg.OpenInBrowserCmd = cleanV
						case "auto_calc_remaining":
							cfg.AutoCalcRemaining = (cleanV == "true" || cleanV == "1")
						case "effort_same_as_estimate":
							cfg.EffortSameAsEstimate = (cleanV == "true" || cleanV == "1")
						case "auto_set_finish_date":
							cfg.AutoSetFinishDate = (cleanV == "true" || cleanV == "1")
						case "auto_sync_interval_seconds", "auto_sync_interval":
							if iv, err := strconv.Atoi(cleanV); err == nil && iv >= 0 {
								cfg.AutoSyncIntervalSeconds = iv
								autoSyncSetFromDB = true
							}
						case "incomplete_criteria_effort":
							cfg.IncompleteCriteriaEffort = (cleanV == "true" || cleanV == "1")
						case "incomplete_criteria_estimate":
							cfg.IncompleteCriteriaEstimate = (cleanV == "true" || cleanV == "1")
						case "incomplete_criteria_start_date":
							cfg.IncompleteCriteriaStartDate = (cleanV == "true" || cleanV == "1")
						case "incomplete_criteria_target_date":
							cfg.IncompleteCriteriaTargetDate = (cleanV == "true" || cleanV == "1")
						case "incomplete_criteria_finish_date":
							cfg.IncompleteCriteriaFinishDate = (cleanV == "true" || cleanV == "1")
						case "incomplete_types":
							parts := strings.Split(cleanV, ",")
							var parsedTypes []string
							for _, p := range parts {
								p = strings.TrimSpace(p)
								if p != "" {
									parsedTypes = append(parsedTypes, p)
								}
							}
							cfg.IncompleteTypes = parsedTypes
						case "incomplete_statuses":
							parts := strings.Split(cleanV, ",")
							var parsedStatuses []string
							for _, p := range parts {
								p = strings.TrimSpace(p)
								if p != "" {
									parsedStatuses = append(parsedStatuses, p)
								}
							}
							cfg.IncompleteStatuses = parsedStatuses
						case "incomplete_date_statuses":
							parts := strings.Split(cleanV, ",")
							var parsedDateStatuses []string
							for _, p := range parts {
								p = strings.TrimSpace(p)
								if p != "" {
									parsedDateStatuses = append(parsedDateStatuses, p)
								}
							}
							cfg.IncompleteDateStatuses = parsedDateStatuses
						case "incomplete_type_tasks":
							cfg.IncompleteTypeTasks = (cleanV == "true" || cleanV == "1")
						case "incomplete_type_bugs":
							cfg.IncompleteTypeBugs = (cleanV == "true" || cleanV == "1")
						case "incomplete_type_pbi":
							cfg.IncompleteTypePBI = (cleanV == "true" || cleanV == "1")
						case "tag_regex":
							cfg.TagRegex = cleanV
							cfg.TagRegexes = ado.ParseRegexRules(cleanV)
						case "incomplete_criteria_tag_regex":
							cfg.IncompleteCriteriaTagRegex = (cleanV == "true" || cleanV == "1")
						case "incomplete_criteria_activity":
							cfg.IncompleteCriteriaActivity = (cleanV == "true" || cleanV == "1")
						}
					}
				}
				rows.Close()
			}
			conn.Close()
		}
	}

	// Environment variable fallback if not configured in DB
	if cfg.PatToken == "" {
		if token := os.Getenv("PAT_TOKEN"); token != "" {
			cfg.PatToken = strings.TrimSpace(token)
		} else if token := os.Getenv("AZURE_DEVOPS_EXT_PAT"); token != "" {
			cfg.PatToken = strings.TrimSpace(token)
		} else if token := os.Getenv("ADO_PAT"); token != "" {
			cfg.PatToken = strings.TrimSpace(token)
		}
	}

	if cfg.DefaultOrg == DefaultOrg || cfg.DefaultOrg == "" {
		if org := os.Getenv("ADO_ORG"); org != "" {
			cfg.DefaultOrg = strings.TrimSpace(org)
		}
	}

	if cfg.DefaultProject == DefaultProject || cfg.DefaultProject == "" {
		if proj := os.Getenv("ADO_PROJECT"); proj != "" {
			cfg.DefaultProject = strings.TrimSpace(proj)
		}
	}

	if cfg.GeminiApiKey == "" {
		if gKey := os.Getenv("GEMINI_API_KEY"); gKey != "" {
			cfg.GeminiApiKey = strings.TrimSpace(gKey)
		}
	}
	if gModel := os.Getenv("GEMINI_MODEL"); gModel != "" {
		cfg.GeminiModel = strings.TrimSpace(gModel)
	}
	if cfg.OpenInBrowserCmd == "" {
		if bCmd := os.Getenv("OPEN_IN_BROWSER_CMD"); bCmd != "" {
			cfg.OpenInBrowserCmd = strings.TrimSpace(bCmd)
		}
	}
	if cfg.TagRegex == "" {
		if tRegex := os.Getenv("TAG_REGEX"); tRegex != "" {
			cfg.TagRegex = strings.TrimSpace(tRegex)
		}
	}
	if !autoSyncSetFromDB {
		if syncEnv := os.Getenv("AUTO_SYNC_INTERVAL"); syncEnv != "" {
			if iv, err := strconv.Atoi(strings.TrimSpace(syncEnv)); err == nil && iv >= 0 {
				cfg.AutoSyncIntervalSeconds = iv
			}
		} else if syncEnv := os.Getenv("AUTO_SYNC_INTERVAL_SECONDS"); syncEnv != "" {
			if iv, err := strconv.Atoi(strings.TrimSpace(syncEnv)); err == nil && iv >= 0 {
				cfg.AutoSyncIntervalSeconds = iv
			}
		}
	}

	// CLI flags override
	if tokenOverride != "" {
		cfg.PatToken = strings.TrimSpace(tokenOverride)
	}

	globalConfig = cfg
	return cfg, nil
}

// Get returns the loaded configuration
func Get() *Config {
	if globalConfig == nil {
		cfg, _ := Load("", "")
		return cfg
	}
	return globalConfig
}
