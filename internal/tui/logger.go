package tui

import (
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
)

// Global log panel instance
var (
	globalLogPanel *LogPanelModel
	logPanelMu     sync.RWMutex
)

// SetLogPanel sets the global log panel for capturing logs
func SetLogPanel(panel *LogPanelModel) {
	logPanelMu.Lock()
	defer logPanelMu.Unlock()
	globalLogPanel = panel
}

// GetLogPanel returns the global log panel
func GetLogPanel() *LogPanelModel {
	logPanelMu.RLock()
	defer logPanelMu.RUnlock()
	return globalLogPanel
}

// LogDebug logs a debug message
func LogDebug(format string, args ...interface{}) {
	logMessage(LevelDebug, format, args...)
}

// LogInfo logs an info message
func LogInfo(format string, args ...interface{}) {
	logMessage(LevelInfo, format, args...)
}

// LogWarn logs a warning message
func LogWarn(format string, args ...interface{}) {
	logMessage(LevelWarn, format, args...)
}

// LogErr logs an error message
func LogErr(format string, args ...interface{}) {
	logMessage(LevelErr, format, args...)
}

// logMessage sends a message to the global log panel
func logMessage(level LogLevel, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	// Clean up the message (remove newlines at end)
	msg = strings.TrimRight(msg, "\n\r")

	logPanelMu.RLock()
	panel := globalLogPanel
	logPanelMu.RUnlock()

	if panel != nil {
		panel.AddEntry(level, msg)
	}
}

// InitLogging sets up the logging system to discard stderr output
// and route all logs through the log panel. Call this early in main.
func InitLogging() {
	// Discard standard log output - we route through log panel instead
	log.SetOutput(io.Discard)
	log.SetFlags(0)
}

// parseLogLevel attempts to parse a log level from a message prefix
// Returns the level and the message with prefix removed
func parseLogLevel(msg string) (LogLevel, string) {
	msg = strings.TrimSpace(msg)

	// Check for [LEVEL] prefix
	if strings.HasPrefix(msg, "[DEBUG]") {
		return LevelDebug, strings.TrimSpace(msg[7:])
	}
	if strings.HasPrefix(msg, "[INFO]") {
		return LevelInfo, strings.TrimSpace(msg[6:])
	}
	if strings.HasPrefix(msg, "[WARN]") {
		return LevelWarn, strings.TrimSpace(msg[6:])
	}
	if strings.HasPrefix(msg, "[WARNING]") {
		return LevelWarn, strings.TrimSpace(msg[9:])
	}
	if strings.HasPrefix(msg, "[ERR]") {
		return LevelErr, strings.TrimSpace(msg[5:])
	}
	if strings.HasPrefix(msg, "[ERROR]") {
		return LevelErr, strings.TrimSpace(msg[7:])
	}

	// Default to info if no prefix found
	return LevelInfo, msg
}
