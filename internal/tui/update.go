package tui

import (
	"strings"

	idf "espworkbench/internal/espworkbench"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var commands []tea.Cmd

	switch messageTyped := message.(type) {

	// ── Window resize ─────────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		model.width = messageTyped.Width
		model.height = messageTyped.Height
		logWidth, logHeight := model.logViewDimensions()
		model.logViewport = viewport.New(logWidth, logHeight)
		model.logViewport.SetContent(renderLogs(model.logs))
		model.logViewport.GotoBottom()

	// ── Keyboard ─────────────────────────────────────────────────────────────
	case tea.KeyMsg:
		// ctrl+c always quits regardless of mode
		if messageTyped.String() == "ctrl+c" {
			return model, tea.Quit
		}

		// bin select mode intercepts all navigation keys
		if model.binSelectMode {
			switch messageTyped.String() {
			case "esc", "q":
				model.binSelectMode = false
				model.binOptions = nil
				model.selectedBin = -1
				model = model.appendLog(idf.LogLine{Text: "→ cancelled", Level: idf.LogLevelSystem})

			case "up", "k":
				if model.selectedBin > 0 {
					model.selectedBin--
				}

			case "down", "j":
				// first down from unselected state moves to index 0
				if model.selectedBin < 0 {
					model.selectedBin = 0
				} else if model.selectedBin < len(model.binOptions)-1 {
					model.selectedBin++
				}

			case "enter":
				if model.selectedBin < 0 {
					// nothing selected yet — require explicit navigation first
					break
				}
				if model.idfPort == "" {
					model = model.appendLog(idf.LogLine{Text: "select a device first", Level: idf.LogLevelWarn})
					break
				}
				opt := model.binOptions[model.selectedBin]
				model.state = StateFlashing
				model.binSelectMode = false
				model.binOptions = nil
				model.selectedBin = -1
				commands = append(commands,
					idf.RunFlashBin(model.idfPort, opt, model.logChannel),
					idf.WaitForLog(model.logChannel),
				)
			}
			return model, tea.Batch(commands...)
		}

		// ── Normal key handling ───────────────────────────────────────────────
		switch messageTyped.String() {

		case "q":
			return model, tea.Quit

		case "tab":
			model.focusedPanel = (model.focusedPanel + 1) % 3

		case "shift+tab":
			model.focusedPanel = (model.focusedPanel + 2) % 3

		case "up", "k":
			if model.focusedPanel == PanelDevices && model.selectedDev > 0 {
				model.selectedDev--
			}

		case "down", "j":
			if model.focusedPanel == PanelDevices && model.selectedDev < len(model.devices)-1 {
				model.selectedDev++
			}

		case "enter":
			if model.focusedPanel == PanelDevices && len(model.devices) > 0 {
				device := model.devices[model.selectedDev]
				model.idfPort = device.Port
				model = model.appendLog(idf.LogLine{
					Text:  "→ selected " + device.Port + "  [" + device.ChipType + "]",
					Level: idf.LogLevelSystem,
				})
			}

		// ── IDF operations ────────────────────────────────────────────────────
		case "b":
			if model.state == StateIdle {
				model.state = StateBuilding
				commands = append(commands, idf.RunIDFCommand(model.project.Path, []string{"build"}, model.logChannel))
			}

		case "f":
			if model.state == StateIdle && model.idfPort != "" {
				model.state = StateFlashing
				commands = append(commands, idf.RunIDFCommand(model.project.Path, []string{"-p", model.idfPort, "flash"}, model.logChannel))
			}

		case "a":
			if model.state == StateIdle && model.idfPort != "" {
				model.state = StateFlashing
				commands = append(commands, idf.RunIDFCommand(model.project.Path, []string{"-p", model.idfPort, "build", "flash"}, model.logChannel))
			}

		case "m":
			if model.state == StateIdle && model.idfPort != "" {
				model.state = StateMonitoring
				commands = append(commands, idf.RunIDFCommand(model.project.Path, []string{"-p", model.idfPort, "monitor"}, model.logChannel))
			}

		case "e":
			if model.state == StateIdle && model.idfPort != "" {
				model.state = StateErasing
				commands = append(commands, idf.RunIDFCommand(model.project.Path, []string{"-p", model.idfPort, "erase-flash"}, model.logChannel))
			}

		// ── Flash existing binary ─────────────────────────────────────────────
		case "x":
			if model.state == StateIdle {
				model = model.appendLog(idf.LogLine{Text: "→ scanning build/ for binaries...", Level: idf.LogLevelSystem})
				commands = append(commands, idf.ScanBinFilesCmd(model.project.Path))
			}

		case "r":
			model = model.appendLog(idf.LogLine{Text: "→ scanning devices...", Level: idf.LogLevelSystem})
			commands = append(commands, idf.ScanDevicesCmd())

		case "l":
			model.logs = model.logs[:0]
			model.logViewport.SetContent("")
		}

	// ── Devices scanned ───────────────────────────────────────────────────────
	case idf.DevicesScannedMsg:
		model.devices = []idf.Device(messageTyped)
		if model.selectedDev >= len(model.devices) {
			model.selectedDev = max(0, len(model.devices)-1)
		}
		if model.idfPort == "" && len(model.devices) > 0 {
			model.idfPort = model.devices[0].Port
		}
		commands = append(commands, idf.TickCmd())

	// ── Bin files scanned ─────────────────────────────────────────────────────
	case idf.BinFilesScannedMsg:
		options := []idf.BinFlashOption(messageTyped)
		if len(options) == 0 {
			model = model.appendLog(idf.LogLine{
				Text:  "no binary files found in build/  ·  run build first",
				Level: idf.LogLevelWarn,
			})
		} else {
			model.binOptions = options
			model.selectedBin = -1 // require explicit navigation before enter works
			model.binSelectMode = true
		}

	// ── Streaming log line ────────────────────────────────────────────────────
	case idf.LogMsg:
		logLine := idf.LogLine(messageTyped)
		model = model.appendLog(logLine)
		model.logViewport.SetContent(renderLogs(model.logs))
		model.logViewport.GotoBottom()
		commands = append(commands, idf.WaitForLog(model.logChannel))

	// ── Operation finished ────────────────────────────────────────────────────
	case idf.OperationDoneMsg:
		model.lastErr = messageTyped.Err
		model.state = StateIdle

	// ── Auto-refresh tick ─────────────────────────────────────────────────────
	case idf.TickMsg:
		commands = append(commands, idf.ScanDevicesCmd())

	// ── Spinner animation ─────────────────────────────────────────────────────
	case spinner.TickMsg:
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(messageTyped)
		commands = append(commands, command)
	}

	// Delegate scroll events to viewport when log panel is focused.
	if model.focusedPanel == PanelLogs {
		var command tea.Cmd
		model.logViewport, command = model.logViewport.Update(message)
		commands = append(commands, command)
	}

	return model, tea.Batch(commands...)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (model Model) appendLog(logLine idf.LogLine) Model {
	model.logs = append(model.logs, logLine)
	if len(model.logs) > 500 {
		model.logs = model.logs[len(model.logs)-500:]
	}
	return model
}

func renderLogs(logs []idf.LogLine) string {
	var stringBuilder strings.Builder
	for _, logLine := range logs {
		switch logLine.Level {
		case idf.LogLevelError:
			stringBuilder.WriteString(logErrorStyle.Render(logLine.Text))
		case idf.LogLevelWarn:
			stringBuilder.WriteString(logWarnStyle.Render(logLine.Text))
		case idf.LogLevelSuccess:
			stringBuilder.WriteString(logSuccessStyle.Render(logLine.Text))
		case idf.LogLevelSystem:
			stringBuilder.WriteString(logSystemStyle.Render(logLine.Text))
		default:
			stringBuilder.WriteString(logInfoStyle.Render(logLine.Text))
		}
		stringBuilder.WriteString("\n")
	}
	return stringBuilder.String()
}

// logViewDimensions calculates the usable inner size of the log viewport.
func (model Model) logViewDimensions() (width int, height int) {
	if model.width == 0 {
		return 60, 20
	}
	deviceWidth := 28
	commandWidth := 34
	logWidth := model.width - deviceWidth - commandWidth
	if logWidth < 20 {
		logWidth = 20
	}
	width = logWidth - 4 - 2
	bodyHeight := model.height - 6 // header is 2 lines
	height = bodyHeight - 5
	if height < 3 {
		height = 3
	}
	return
}

func max(firstValue int, secondValue int) int {
	if firstValue > secondValue {
		return firstValue
	}
	return secondValue
}
