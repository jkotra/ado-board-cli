package config

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestConfigPatTokenResolution(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "cfg_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDb := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpDb)

	// Create SQLite schema
	conn, err := sql.Open("sqlite", tmpDb)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	conn.Close()

	// Scenario 1: No DB setting, no env var -> PatToken is empty
	_ = os.Unsetenv("PAT_TOKEN")
	_ = os.Unsetenv("AZURE_DEVOPS_EXT_PAT")
	_ = os.Unsetenv("ADO_PAT")

	cfg, err := Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.PatToken != "" {
		t.Fatalf("expected empty PatToken, got %q", cfg.PatToken)
	}

	// Scenario 2: No DB setting, PAT_TOKEN env var set -> falls back to env var
	_ = os.Setenv("PAT_TOKEN", "env-secret-pat-12345")
	cfg, err = Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.PatToken != "env-secret-pat-12345" {
		t.Fatalf("expected fallback to env var, got %q", cfg.PatToken)
	}

	// Scenario 3: DB setting stored -> takes precedence over env var
	conn, err = sql.Open("sqlite", tmpDb)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, err = conn.Exec(`INSERT OR REPLACE INTO app_settings (key, value, updated_at) VALUES ('pat_token', 'db-stored-pat-67890', CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatalf("failed to insert setting: %v", err)
	}
	conn.Close()

	cfg, err = Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.PatToken != "db-stored-pat-67890" {
		t.Fatalf("expected DB setting to take precedence, got %q", cfg.PatToken)
	}

	// Scenario 4: CLI flag passed -> takes highest precedence
	cfg, err = Load(tmpDb, "cli-flag-pat-99999")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.PatToken != "cli-flag-pat-99999" {
		t.Fatalf("expected CLI flag override, got %q", cfg.PatToken)
	}

	// Clean up env
	_ = os.Unsetenv("PAT_TOKEN")
}

func TestConfigGeminiApiKeyResolution(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "cfg_gemini_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDb := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpDb)

	// Create SQLite schema
	conn, err := sql.Open("sqlite", tmpDb)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	conn.Close()

	// Scenario 1: Unset in DB and env -> empty string
	_ = os.Unsetenv("GEMINI_API_KEY")
	cfg, err := Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.GeminiApiKey != "" {
		t.Fatalf("expected empty GeminiApiKey by default, got %q", cfg.GeminiApiKey)
	}

	// Scenario 2: Set via GEMINI_API_KEY env var -> falls back to env var
	_ = os.Setenv("GEMINI_API_KEY", "env-gemini-key-12345")
	cfg, err = Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.GeminiApiKey != "env-gemini-key-12345" {
		t.Fatalf("expected env var GeminiApiKey, got %q", cfg.GeminiApiKey)
	}

	// Scenario 3: Stored in DB -> DB takes precedence
	conn, err = sql.Open("sqlite", tmpDb)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, err = conn.Exec(`INSERT OR REPLACE INTO app_settings (key, value, updated_at) VALUES ('gemini_api_key', 'db-gemini-key-67890', CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatalf("failed to insert setting: %v", err)
	}
	conn.Close()

	cfg, err = Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.GeminiApiKey != "db-gemini-key-67890" {
		t.Fatalf("expected DB setting to take precedence, got %q", cfg.GeminiApiKey)
	}

	_ = os.Unsetenv("GEMINI_API_KEY")
}

func TestConfigOpenInBrowserCmdResolution(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "cfg_browser_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDb := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpDb)

	// Create SQLite schema
	conn, err := sql.Open("sqlite", tmpDb)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	conn.Close()

	// Scenario 1: Unset in DB and env -> empty string (defaults to system default browser)
	_ = os.Unsetenv("OPEN_IN_BROWSER_CMD")
	cfg, err := Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.OpenInBrowserCmd != "" {
		t.Fatalf("expected empty OpenInBrowserCmd by default, got %q", cfg.OpenInBrowserCmd)
	}

	// Scenario 2: Set via OPEN_IN_BROWSER_CMD env var -> falls back to env var
	_ = os.Setenv("OPEN_IN_BROWSER_CMD", "google-chrome {url}")
	cfg, err = Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.OpenInBrowserCmd != "google-chrome {url}" {
		t.Fatalf("expected env var OpenInBrowserCmd, got %q", cfg.OpenInBrowserCmd)
	}

	// Scenario 3: Stored in DB -> DB takes precedence
	conn, err = sql.Open("sqlite", tmpDb)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, err = conn.Exec(`INSERT OR REPLACE INTO app_settings (key, value, updated_at) VALUES ('open_in_browser_cmd', 'firefox {url}', CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatalf("failed to insert setting: %v", err)
	}
	conn.Close()

	cfg, err = Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.OpenInBrowserCmd != "firefox {url}" {
		t.Fatalf("expected DB setting to take precedence, got %q", cfg.OpenInBrowserCmd)
	}

	_ = os.Unsetenv("OPEN_IN_BROWSER_CMD")
}

func TestConfigAutoSyncIntervalResolution(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "cfg_autosync_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	tmpDb := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpDb)

	// Create SQLite schema
	conn, err := sql.Open("sqlite", tmpDb)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, err = conn.Exec(`
		CREATE TABLE app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	conn.Close()

	// Scenario 1: Unset in DB and env -> defaults to 120
	_ = os.Unsetenv("AUTO_SYNC_INTERVAL")
	_ = os.Unsetenv("AUTO_SYNC_INTERVAL_SECONDS")
	cfg, err := Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.AutoSyncIntervalSeconds != 120 {
		t.Fatalf("expected default AutoSyncIntervalSeconds 120, got %d", cfg.AutoSyncIntervalSeconds)
	}

	// Scenario 2: Set via AUTO_SYNC_INTERVAL env var -> falls back to env var
	_ = os.Setenv("AUTO_SYNC_INTERVAL", "60")
	cfg, err = Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.AutoSyncIntervalSeconds != 60 {
		t.Fatalf("expected env var AutoSyncIntervalSeconds 60, got %d", cfg.AutoSyncIntervalSeconds)
	}

	// Scenario 3: Stored in DB -> DB takes precedence (can be 0 to disable)
	conn, err = sql.Open("sqlite", tmpDb)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	_, err = conn.Exec(`INSERT OR REPLACE INTO app_settings (key, value, updated_at) VALUES ('auto_sync_interval_seconds', '0', CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatalf("failed to insert setting: %v", err)
	}
	conn.Close()

	cfg, err = Load(tmpDb, "")
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if cfg.AutoSyncIntervalSeconds != 0 {
		t.Fatalf("expected DB setting 0 (disabled) to take precedence, got %d", cfg.AutoSyncIntervalSeconds)
	}

	_ = os.Unsetenv("AUTO_SYNC_INTERVAL")
}
