package logger

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	logFile *os.File
	mu      sync.Mutex
)

// DefaultLogPath returns the canonical path for the debug log file.
func DefaultLogPath() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ado-board-cli", "debug.log")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".ado-board-cli", "debug.log")
	}
	return filepath.Join(".", "ado-board-debug.log")
}

// InitLogger initializes the global slog default logger and standard log output.
// If debug is true, logs are written with LevelDebug; otherwise with LevelInfo.
// Target path defaults to DefaultLogPath() (~/.ado-board-cli/debug.log).
func InitLogger(debug bool, logFilePath string) (*os.File, error) {
	mu.Lock()
	defer mu.Unlock()

	// Close any previously opened file handle
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}

	targetPath := logFilePath
	if targetPath == "" {
		targetPath = DefaultLogPath()
	}

	// Ensure parent directory exists
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	logFile = f

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	// Structured text handler
	handler := slog.NewTextHandler(f, &slog.HandlerOptions{
		Level: level,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Redirect standard library logger to the same file handle
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	slog.Info("Logger initialized", "path", targetPath, "debug", debug)

	return f, nil
}

// Close closes the underlying log file if open.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
}
