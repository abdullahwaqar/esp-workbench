package espworkbench

import (
	"time"
)

type Device struct {
	Port     string
	ChipType string
	MAC      string
}

type LogLevel int

const (
	LogLevelInfo LogLevel = iota
	LogLevelWarn
	LogLevelError
	LogLevelSuccess
	LogLevelSystem
)

type LogLine struct {
	Text  string
	Level LogLevel
}

type DevicesScannedMsg []Device

type LogMsg LogLine

type OperationDoneMsg struct {
	Err error
}

type TickMsg struct {
	Timestamp time.Time
}
