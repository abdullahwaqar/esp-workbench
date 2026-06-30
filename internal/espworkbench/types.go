package espworkbench

import "time"

// Device represents a connected ESP32 device
type Device struct {
	Port     string
	ChipType string
	MAC      string
}

// LogLevel classifies severity of log output
type LogLevel int

const (
	LogLevelInfo LogLevel = iota
	LogLevelWarn
	LogLevelError
	LogLevelSuccess
	LogLevelSystem
)

// LogLine is a single timestamped log entry
type LogLine struct {
	Text  string
	Level LogLevel
}

// ── Message types (exported for TUI layer) ────────────────────────────────

// DevicesScannedMsg is returned when device scan completes
type DevicesScannedMsg []Device

// LogMsg carries a single log line from a running command
type LogMsg LogLine

// OperationDoneMsg signals that a subprocess operation finished
type OperationDoneMsg struct {
	Err error
}

// TickMsg is a periodic refresh timer
type TickMsg struct {
	Timestamp time.Time
}
