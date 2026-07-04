package tui

import (
	idf "espworkbench/internal/espworkbench"
	"os"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type AppState int

const (
	StateIdle AppState = iota
	StateBuilding
	StateFlashing
	StateMonitoring
	StateErasing
	StateReadingPartitions
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
	case StateReadingPartitions:
		return "reading partitions"
	default:
		return "idle"
	}
}

type Panel int

const (
	PanelDevices Panel = iota
	PanelCommands
	PanelLogs
)

type (
	DevicesScannedMsg = idf.DevicesScannedMsg
	LogMsg            = idf.LogMsg
	OperationDoneMsg  = idf.OperationDoneMsg
	TickMsg           = idf.TickMsg
	DirLoadedMsg      = idf.DirLoadedMsg
	PartitionsReadMsg = idf.PartitionsReadMsg
	MonitorStartedMsg = idf.MonitorStartedMsg
	MonitorDoneMsg    = idf.MonitorDoneMsg
)

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

	// file browser for flashing existing binaries
	browserMode   bool
	browserPath   string
	browserItems  []idf.FileBrowserEntry
	browserCursor int

	// non-nil while awaiting a second enter to confirm
	browserConfirm *idf.BinFlashOption

	// partition table visualization
	partitionMode  bool
	partitionTable idf.PartitionTable
	partitionErr   error

	monitorPty  *os.File
	monitorDone chan struct{}
}

func InitialModel(projectPath string) Model {
	spinnerModel := spinner.New()
	spinnerModel.Spinner = spinner.MiniDot
	spinnerModel.Style = accentStyle

	project := idf.LoadProjectContext(projectPath)

	model := Model{
		state:       StateIdle,
		spinner:     spinnerModel,
		logViewport: viewport.New(80, 20),
		logChannel:  make(chan idf.LogLine, 128),
		logs:        make([]idf.LogLine, 0, 256),
		project:     project,
	}

	toolchain := idf.Toolchain()
	switch {
	case toolchain.Err != nil:
		model = model.appendLog(idf.LogLine{Text: toolchain.Err.Error(), Level: idf.LogLevelError})
	case toolchain.ExportScript != "":
		model = model.appendLog(idf.LogLine{
			Text:  "idf.py not on PATH, auto-sourcing " + toolchain.ExportScript,
			Level: idf.LogLevelWarn,
		})
	}

	return model
}

func (model Model) Init() tea.Cmd {
	return tea.Batch(
		idf.ScanDevicesCmd(),
		idf.TickCmd(),
		model.spinner.Tick,
		idf.WaitForLog(model.logChannel),
	)
}
