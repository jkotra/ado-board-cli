package logger

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitLoggerProductionMode(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "prod.log")

	f, err := InitLogger(false, logPath)
	if err != nil {
		t.Fatalf("expected no error in production mode, got %v", err)
	}
	if f == nil {
		t.Fatalf("expected non-nil file handle in production mode")
	}
	defer Close()

	// Logging in production mode should write info but not debug
	slog.Info("test production info log")
	slog.Debug("test production debug log should be suppressed")
	log.Println("standard lib test")

	_ = f.Sync()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "test production info log") {
		t.Errorf("expected info log in file, got:\n%s", content)
	}
	if strings.Contains(content, "test production debug log should be suppressed") {
		t.Errorf("expected debug log to be filtered out in production mode, got:\n%s", content)
	}
}

func TestInitLoggerDebugMode(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "test_debug.log")

	f, err := InitLogger(true, logPath)
	if err != nil {
		t.Fatalf("failed to init debug logger: %v", err)
	}
	if f == nil {
		t.Fatalf("expected non-nil file handle")
	}
	defer Close()

	slog.Debug("test debug event", slog.String("component", "tui"), slog.Int("item_id", 12345))
	slog.Info("test info event", slog.String("status", "ok"))
	log.Println("standard lib log line")

	// Flush/sync
	_ = f.Sync()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "test debug event") {
		t.Errorf("expected log file to contain 'test debug event', got:\n%s", content)
	}
	if !strings.Contains(content, "component=tui") {
		t.Errorf("expected log file to contain 'component=tui', got:\n%s", content)
	}
	if !strings.Contains(content, "item_id=12345") {
		t.Errorf("expected log file to contain 'item_id=12345', got:\n%s", content)
	}
	if !strings.Contains(content, "standard lib log line") {
		t.Errorf("expected log file to contain standard lib log line, got:\n%s", content)
	}
}

func TestDefaultLogPath(t *testing.T) {
	path := DefaultLogPath()
	if path == "" {
		t.Fatalf("expected non-empty default log path")
	}
	if !strings.HasSuffix(path, "debug.log") {
		t.Errorf("expected log path to end with 'debug.log', got %q", path)
	}
}
