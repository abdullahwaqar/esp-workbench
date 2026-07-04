package tui

import (
	"path/filepath"
	"regexp"
	"strings"

	idf "espworkbench/internal/espworkbench"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var commands []tea.Cmd

	switch messageTyped := message.(type) {

	case tea.WindowSizeMsg:
		model.width = messageTyped.Width
		model.height = messageTyped.Height
		logWidth, logHeight := model.logViewDimensions()
		model.logViewport = viewport.New(logWidth, logHeight)
		model.logViewport.SetContent(renderLogs(model.logs, logWidth))
		model.logViewport.GotoBottom()

	case tea.KeyMsg:
		if messageTyped.String() == "ctrl+c" {
			return model, tea.Quit
		}

		if model.partitionMode {
			switch messageTyped.String() {
			case "esc", "q":
				model.partitionMode = false
				model = model.resizeLogViewport()
			case "r":
				if model.idfPort != "" {
					model.partitionMode = false
					model.state = StateReadingPartitions
					model = model.resizeLogViewport()
					commands = append(commands, idf.ReadPartitionTableCmd(model.idfPort))
				}
			}
			return model, tea.Batch(commands...)
		}

		if model.browserMode {
			// confirmation sub-state: require a second, deliberate enter before
			// anything actually gets written to the device
			if model.browserConfirm != nil {
				switch messageTyped.String() {
				case "enter", "y":
					opt := *model.browserConfirm
					model.state = StateFlashing
					model.browserMode = false
					model.browserConfirm = nil
					model = model.resizeLogViewport()
					commands = append(commands,
						idf.RunFlashBin(model.idfPort, opt, model.logChannel),
						idf.WaitForLog(model.logChannel),
					)
				case "esc", "n":
					// back to the listing, nothing flashed
					model.browserConfirm = nil
				}
				return model, tea.Batch(commands...)
			}

			switch messageTyped.String() {

			case "esc", "q":
				model.browserMode = false
				model.browserItems = nil
				model.browserCursor = 0
				model.browserConfirm = nil
				model = model.resizeLogViewport()

			case "up", "k":
				if model.browserCursor > 0 {
					model.browserCursor--
				}

			case "down", "j":
				if model.browserCursor < len(model.browserItems)-1 {
					model.browserCursor++
				}

			// backspace or left arrow: go up one directory
			case "backspace", "left", "h":
				parent := filepath.Dir(model.browserPath)
				if parent != model.browserPath {
					commands = append(commands, idf.BrowseDirCmd(parent))
				}

			case "enter", "right", "l":
				if len(model.browserItems) == 0 {
					break
				}
				item := model.browserItems[model.browserCursor]

				switch {
				case item.Name == "..":
					parent := filepath.Dir(model.browserPath)
					if parent != model.browserPath {
						commands = append(commands, idf.BrowseDirCmd(parent))
					}

				case item.IsDir:
					commands = append(commands, idf.BrowseDirCmd(
						filepath.Join(model.browserPath, item.Name),
					))

				case item.IsFullFlash:
					if model.idfPort == "" {
						model = model.appendLog(idf.LogLine{Text: "select a device first", Level: idf.LogLevelWarn})
						break
					}
					opt, err := idf.BuildFullFlashOption(model.browserPath)
					if err != nil {
						model = model.appendLog(idf.LogLine{
							Text:  "failed to read flasher_args.json: " + err.Error(),
							Level: idf.LogLevelError,
						})
						break
					}
					// stage for confirmation — does not flash yet
					model.browserConfirm = &opt

				default:
					// .bin file
					if model.idfPort == "" {
						model = model.appendLog(idf.LogLine{Text: "select a device first", Level: idf.LogLevelWarn})
						break
					}
					absPath := filepath.Join(model.browserPath, item.Name)
					opt := idf.BinFlashOption{
						Label:     item.Name,
						BinPath:   absPath,
						FlashAddr: idf.GuessFlashAddr(item.Name, item.Size),
					}
					// stage for confirmation — does not flash yet
					model.browserConfirm = &opt
				}
			}
			return model, tea.Batch(commands...)
		}

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
				model.monitorDone = make(chan struct{})
				commands = append(commands, idf.StartMonitorCmd(
					model.project.Path, model.idfPort, model.logChannel, model.monitorDone,
				))
			} else if model.state == StateMonitoring {
				commands = append(commands, idf.StopMonitorCmd(model.monitorPty))
			}

		case "e":
			if model.state == StateIdle && model.idfPort != "" {
				model.state = StateErasing
				commands = append(commands, idf.RunIDFCommand(model.project.Path, []string{"-p", model.idfPort, "erase-flash"}, model.logChannel))
			}

		case "x":
			if model.state == StateIdle {
				commands = append(commands, idf.StartBrowserCmd(model.project.Path))
			}

		case "p":
			if model.state == StateIdle {
				if model.idfPort == "" {
					model = model.appendLog(idf.LogLine{Text: "select a device first", Level: idf.LogLevelWarn})
				} else {
					model.state = StateReadingPartitions
					commands = append(commands, idf.ReadPartitionTableCmd(model.idfPort))
				}
			}

		case "r":
			model = model.appendLog(idf.LogLine{Text: "→ scanning devices...", Level: idf.LogLevelSystem})
			commands = append(commands, idf.ScanDevicesCmd())

		case "l":
			model.logs = model.logs[:0]
			model.logViewport.SetContent("")
		}

	case idf.DevicesScannedMsg:
		model.devices = []idf.Device(messageTyped)
		if model.selectedDev >= len(model.devices) {
			model.selectedDev = max(0, len(model.devices)-1)
		}
		if model.idfPort == "" && len(model.devices) > 0 {
			model.idfPort = model.devices[0].Port
		}
		commands = append(commands, idf.TickCmd())

	case idf.DirLoadedMsg:
		if messageTyped.Err != nil {
			model = model.appendLog(idf.LogLine{
				Text:  "cannot read directory: " + messageTyped.Err.Error(),
				Level: idf.LogLevelError,
			})
		} else {
			model.browserPath = messageTyped.Path
			model.browserItems = messageTyped.Items
			model.browserCursor = 0
			model.browserMode = true
			model.browserConfirm = nil
			model = model.resizeLogViewport()
		}

	case idf.PartitionsReadMsg:
		model.state = StateIdle
		if messageTyped.Err != nil {
			model.partitionErr = messageTyped.Err
			model = model.appendLog(idf.LogLine{
				Text:  "partition read failed: " + messageTyped.Err.Error(),
				Level: idf.LogLevelError,
			})
		} else {
			model.partitionTable = messageTyped.Table
			model.partitionErr = nil
			model.partitionMode = true
			model = model.resizeLogViewport()
		}

	case idf.LogMsg:
		logLine := idf.LogLine(messageTyped)
		model = model.appendLog(logLine)
		model.logViewport.SetContent(renderLogs(model.logs, model.logViewport.Width))
		model.logViewport.GotoBottom()
		commands = append(commands, idf.WaitForLog(model.logChannel))

	case idf.MonitorStartedMsg:
		if messageTyped.Err != nil {
			model.state = StateIdle
			model = model.appendLog(idf.LogLine{Text: messageTyped.Err.Error(), Level: idf.LogLevelError})
		} else {
			model.monitorPty = messageTyped.Pty
			commands = append(commands, idf.WaitForMonitorDone(model.monitorDone))
		}

	case idf.MonitorDoneMsg:
		model.state = StateIdle
		model.monitorPty = nil
		model.monitorDone = nil

	case idf.OperationDoneMsg:
		model.lastErr = messageTyped.Err
		model.state = StateIdle

	case idf.TickMsg:
		if model.state == StateIdle {
			commands = append(commands, idf.ScanDevicesCmd())
		} else {
			commands = append(commands, idf.TickCmd())
		}

	case spinner.TickMsg:
		var command tea.Cmd
		model.spinner, command = model.spinner.Update(messageTyped)
		commands = append(commands, command)
	}

	if model.focusedPanel == PanelLogs {
		var command tea.Cmd
		model.logViewport, command = model.logViewport.Update(message)
		commands = append(commands, command)
	}

	return model, tea.Batch(commands...)
}

func (model Model) resizeLogViewport() Model {
	logWidth, logHeight := model.logViewDimensions()
	model.logViewport = viewport.New(logWidth, logHeight)
	model.logViewport.SetContent(renderLogs(model.logs, logWidth))
	model.logViewport.GotoBottom()
	return model
}

func (model Model) appendLog(logLine idf.LogLine) Model {
	model.logs = append(model.logs, logLine)
	if len(model.logs) > 500 {
		model.logs = model.logs[len(model.logs)-500:]
	}
	return model
}

var ansiEscapeRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func renderLogs(logs []idf.LogLine, width int) string {
	var sb strings.Builder
	for _, logLine := range logs {
		text := sanitizeLogText(logLine.Text)
		style := logStyleFor(logLine.Level)

		for _, physicalLine := range wrapLine(text, width) {
			sb.WriteString(style.Render(physicalLine))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

func logStyleFor(level idf.LogLevel) lipgloss.Style {
	switch level {
	case idf.LogLevelError:
		return logErrorStyle
	case idf.LogLevelWarn:
		return logWarnStyle
	case idf.LogLevelSuccess:
		return logSuccessStyle
	case idf.LogLevelSystem:
		return logSystemStyle
	default:
		return logInfoStyle
	}
}

func wrapLine(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	runes := []rune(s)
	if len(runes) == 0 {
		return []string{""}
	}
	var lines []string
	for len(runes) > width {
		lines = append(lines, string(runes[:width]))
		runes = runes[width:]
	}
	lines = append(lines, string(runes))
	return lines
}

func sanitizeLogText(line string) string {
	line = strings.TrimRight(line, "\r\n")
	if idx := strings.LastIndex(line, "\r"); idx != -1 {
		line = line[idx+1:]
	}
	return ansiEscapeRegex.ReplaceAllString(line, "")
}

// Returns a wider panel width when an interactive
// visualization (browser, confirm, or partitions) needs the extra room.
// Used by both viewBody and logViewDimensions so layout never drifts apart.
func commandPanelWidth(model Model) int {
	if model.browserMode || model.partitionMode {
		return 52
	}
	return 34
}

func (model Model) logViewDimensions() (width int, height int) {
	if model.width == 0 {
		return 60, 20
	}
	deviceWidth := 28
	commandWidth := commandPanelWidth(model)
	logWidth := model.width - deviceWidth - commandWidth
	if logWidth < 20 {
		logWidth = 20
	}
	width = logWidth - 4 - 2
	bodyHeight := model.height - model.chromeHeight()
	height = bodyHeight - 5
	if height < 3 {
		height = 3
	}
	return
}
