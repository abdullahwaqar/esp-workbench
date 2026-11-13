package tui

import (
	"os"

	idf "espworkbench/internal/espworkbench"

	"github.com/charmbracelet/bubbles/key"
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

// Tab moves focus between the panels that respond to keys: the device list
// and the log viewport. PanelCommands is kept for API compatibility but is
// not a focus stop, because nothing in it is navigable.
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

// Lines of log history kept in memory.
const maxLogLines = 500

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

	// true while a device scan is in flight, so the tick loop and the
	// manual rescan key never stack up parallel scans
	scanning bool

	// one-line feedback shown in the footer until the next key press,
	// e.g. why a key did nothing
	notice *idf.LogLine

	// set by a first q while an operation runs; a second q quits
	quitArmed bool

	// waiting for a second, deliberate key before erasing the whole chip
	eraseConfirm bool

	// file browser for flashing existing binaries
	browserMode   bool
	browserPath   string
	browserItems  []idf.FileBrowserEntry
	browserCursor int
	// name of the directory we just left, so going up lands the cursor on it
	browserReturnTo string

	// non-nil while awaiting a second enter to confirm
	browserConfirm *idf.BinFlashOption

	// partition table visualization
	partitionMode  bool
	partitionTable idf.PartitionTable
	partitionErr   error

	monitorPty  *os.File
	monitorDone chan struct{}
}

// Builds the starting model without touching the toolchain or the
// filesystem beyond loading the project, so tests can construct it.
func newModel(project idf.ProjectContext) Model {
	spinnerModel := spinner.New()
	spinnerModel.Spinner = spinner.MiniDot
	spinnerModel.Style = accentStyle

	return Model{
		state:        StateIdle,
		spinner:      spinnerModel,
		logViewport:  newLogViewport(60, 20),
		logChannel:   make(chan idf.LogLine, 128),
		logs:         make([]idf.LogLine, 0, 256),
		project:      project,
		focusedPanel: PanelDevices,
		scanning:     true, // Init starts the first scan
	}
}

func InitialModel(projectPath string) Model {
	model := newModel(idf.LoadProjectContext(projectPath))

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
		idf.WaitForLog(model.logChannel),
	)
}

// The log viewport only scrolls with keys that no app command uses. The
// bubbles defaults also bind b, f, d, u, h, l and space, which would make a
// single press both start a build/flash and scroll the log.
func newLogViewport(width, height int) viewport.Model {
	vp := viewport.New(width, height)
	vp.KeyMap = viewport.KeyMap{
		PageDown:     key.NewBinding(key.WithKeys("pgdown")),
		PageUp:       key.NewBinding(key.WithKeys("pgup")),
		HalfPageDown: key.NewBinding(key.WithKeys("ctrl+d")),
		HalfPageUp:   key.NewBinding(key.WithKeys("ctrl+u")),
		Down:         key.NewBinding(key.WithKeys("down", "j")),
		Up:           key.NewBinding(key.WithKeys("up", "k")),
		Left:         key.NewBinding(key.WithDisabled()),
		Right:        key.NewBinding(key.WithDisabled()),
	}
	return vp
}

func (model Model) isBusy() bool {
	return model.state != StateIdle
}
