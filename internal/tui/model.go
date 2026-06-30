package tui

import (
	idf "espworkbench/internal/espworkbench"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// ── State machine ─────────────────────────────────────────────────────────────

type AppState int

const (
	StateIdle AppState = iota
	StateBuilding
	StateFlashing
	StateMonitoring
	StateErasing
)

func (state AppState) String() string {
	switch state {
	case StateBuilding:
		return "building"
	case StateFlashing:
		return "flashing"
	case StateMonitoring:
		return "monitoring"
	case StateErasing:
		return "erasing"
	default:
		return "idle"
	}
}

// ── Panels ────────────────────────────────────────────────────────────────────

type Panel int

const (
	PanelDevices Panel = iota
	PanelCommands
	PanelLogs
)

// ── Messages ──────────────────────────────────────────────────────────────────

type (
	DevicesScannedMsg  = idf.DevicesScannedMsg
	LogMsg             = idf.LogMsg
	OperationDoneMsg   = idf.OperationDoneMsg
	TickMsg            = idf.TickMsg
	BinFilesScannedMsg = idf.BinFilesScannedMsg
)

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	width        int
	height       int
	devices      []idf.Device
	selectedDev  int
	state        AppState
	focusedPanel Panel
	spinner      spinner.Model
	logViewport  viewport.Model
	logs         []idf.LogLine
	logChannel   chan idf.LogLine
	idfPort      string
	lastErr      error
	project      idf.ProjectContext

	// bin flash picker
	binSelectMode bool
	binOptions    []idf.BinFlashOption
	selectedBin   int
}

func InitialModel(projectPath string) Model {
	spinnerModel := spinner.New()
	spinnerModel.Spinner = spinner.MiniDot
	spinnerModel.Style = accentStyle

	project := idf.LoadProjectContext(projectPath)

	return Model{
		state:       StateIdle,
		spinner:     spinnerModel,
		logViewport: viewport.New(80, 20),
		logChannel:  make(chan idf.LogLine, 128),
		logs:        make([]idf.LogLine, 0, 256),
		project:     project,
	}
}

func (model Model) Init() tea.Cmd {
	return tea.Batch(
		idf.ScanDevicesCmd(),
		idf.TickCmd(),
		model.spinner.Tick,
		idf.WaitForLog(model.logChannel),
	)
}
