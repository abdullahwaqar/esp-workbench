package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	idf "espworkbench/internal/espworkbench"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	var commands []tea.Cmd

	switch messageTyped := message.(type) {

	case tea.WindowSizeMsg:
		model.width = messageTyped.Width
		model.height = messageTyped.Height

	case tea.KeyMsg:
		var command tea.Cmd
		model, command = model.handleKey(messageTyped)
		commands = append(commands, command)

	case idf.DevicesScannedMsg:
		model = model.applyScan([]idf.Device(messageTyped))

	case idf.DirLoadedMsg:
		model = model.applyDirLoaded(messageTyped)

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
		}

	case idf.LogMsg:
		model = model.appendLog(idf.LogLine(messageTyped))
		commands = append(commands, idf.WaitForLog(model.logChannel))

	case idf.MonitorStartedMsg:
		if messageTyped.Err != nil {
			model.state = StateIdle
			model.monitorDone = nil
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
		// exactly one tick loop runs for the life of the program; it only
		// scans while idle so probes never contend with build/flash/monitor
		// for the serial port
		commands = append(commands, idf.TickCmd())
		if !model.isBusy() && !model.scanning {
			model.scanning = true
			commands = append(commands, idf.ScanDevicesCmd())
		}

	case spinner.TickMsg:
		// let the spinner loop die while idle; startOperation restarts it
		if model.isBusy() {
			var command tea.Cmd
			model.spinner, command = model.spinner.Update(messageTyped)
			commands = append(commands, command)
		}
	}

	if !model.isBusy() {
		model.quitArmed = false
	}
	model = model.syncLogViewport()
	return model, tea.Batch(commands...)
}

func (model Model) handleKey(keyMsg tea.KeyMsg) (Model, tea.Cmd) {
	keyName := keyMsg.String()
	if keyName == "ctrl+c" {
		return model, tea.Quit
	}

	model.notice = nil
	if keyName != "q" {
		model.quitArmed = false
	}

	switch {
	case model.tooSmall():
		if keyName == "q" {
			return model.requestQuit()
		}
		return model, nil
	case model.eraseConfirm:
		return model.handleEraseConfirmKey(keyName)
	case model.partitionMode:
		return model.handlePartitionKey(keyName)
	case model.browserConfirm != nil:
		return model.handleFlashConfirmKey(keyName)
	case model.browserMode:
		return model.handleBrowserKey(keyName)
	}
	return model.handleMainKey(keyMsg)
}

func (model Model) handleMainKey(keyMsg tea.KeyMsg) (Model, tea.Cmd) {
	switch keyMsg.String() {

	case "q":
		return model.requestQuit()

	case "tab", "shift+tab":
		if model.focusedPanel == PanelLogs {
			model.focusedPanel = PanelDevices
		} else {
			model.focusedPanel = PanelLogs
		}

	case "up", "k":
		if model.focusedPanel == PanelDevices && model.selectedDev > 0 {
			model.selectedDev--
		}

	case "down", "j":
		if model.focusedPanel == PanelDevices && model.selectedDev < len(model.devices)-1 {
			model.selectedDev++
		}

	case "home", "g":
		if model.focusedPanel == PanelLogs {
			model.logViewport.GotoTop()
		}

	case "end", "G":
		if model.focusedPanel == PanelLogs {
			model.logViewport.GotoBottom()
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
		if model, ok := model.requireIdle("build"); !ok {
			return model, nil
		}
		if !model.project.IsValid {
			return model.withNotice(idf.LogLevelWarn,
				"can't build: "+model.project.ValidityLabel()+". start esp-workbench in an esp-idf project folder"), nil
		}
		return model.startOperation(StateBuilding,
			idf.RunIDFCommand(model.project.Path, []string{"build"}, model.logChannel))

	case "f":
		if model, ok := model.requireIdleWithPort("flash"); !ok {
			return model, nil
		}
		return model.startOperation(StateFlashing,
			idf.RunIDFCommand(model.project.Path, []string{"-p", model.idfPort, "flash"}, model.logChannel))

	case "a":
		if model, ok := model.requireIdleWithPort("build and flash"); !ok {
			return model, nil
		}
		return model.startOperation(StateFlashing,
			idf.RunIDFCommand(model.project.Path, []string{"-p", model.idfPort, "build", "flash"}, model.logChannel))

	case "m":
		if model.state == StateMonitoring {
			if model.monitorPty == nil {
				return model.withNotice(idf.LogLevelSystem, "monitor is still starting. press m again in a moment to stop it"), nil
			}
			return model, idf.StopMonitorCmd(model.monitorPty)
		}
		if model, ok := model.requireIdleWithPort("monitor"); !ok {
			return model, nil
		}
		model.monitorDone = make(chan struct{})
		return model.startOperation(StateMonitoring,
			idf.StartMonitorCmd(model.project.Path, model.idfPort, model.logChannel, model.monitorDone))

	case "e":
		if model, ok := model.requireIdleWithPort("erase"); !ok {
			return model, nil
		}
		// erasing wipes the whole chip and cannot be undone, so it gets the
		// same deliberate second step as flashing a binary
		model.eraseConfirm = true

	case "x":
		if model, ok := model.requireIdle("browse binaries"); !ok {
			return model, nil
		}
		model.browserReturnTo = ""
		return model, idf.StartBrowserCmd(model.project.Path)

	case "p":
		if model, ok := model.requireIdleWithPort("read partitions"); !ok {
			return model, nil
		}
		return model.startOperation(StateReadingPartitions, idf.ReadPartitionTableCmd(model.idfPort))

	case "r":
		if model, ok := model.requireIdle("rescan"); !ok {
			return model, nil
		}
		if model.scanning {
			return model.withNotice(idf.LogLevelSystem, "already scanning for devices"), nil
		}
		model.scanning = true
		model = model.appendLog(idf.LogLine{Text: "→ scanning devices...", Level: idf.LogLevelSystem})
		return model, idf.ScanDevicesCmd()

	case "l":
		model.logs = nil
		model.logViewport.SetContent("")
		model.logViewport.GotoTop()

	default:
		if model.focusedPanel == PanelLogs {
			var command tea.Cmd
			model.logViewport, command = model.logViewport.Update(keyMsg)
			return model, command
		}
	}

	// up/down/pgup/pgdn scroll the log when it has focus
	if model.focusedPanel == PanelLogs {
		switch keyMsg.String() {
		case "up", "k", "down", "j":
			var command tea.Cmd
			model.logViewport, command = model.logViewport.Update(keyMsg)
			return model, command
		}
	}
	return model, nil
}

func (model Model) handleEraseConfirmKey(keyName string) (Model, tea.Cmd) {
	switch keyName {
	case "enter", "y":
		model.eraseConfirm = false
		if model, ok := model.requireIdleWithPort("erase"); !ok {
			return model, nil
		}
		return model.startOperation(StateErasing,
			idf.RunIDFCommand(model.project.Path, []string{"-p", model.idfPort, "erase-flash"}, model.logChannel))
	case "esc", "n", "q":
		model.eraseConfirm = false
		return model.withNotice(idf.LogLevelSystem, "erase cancelled. nothing was changed"), nil
	}
	return model, nil
}

func (model Model) handlePartitionKey(keyName string) (Model, tea.Cmd) {
	switch keyName {
	case "esc", "q":
		model.partitionMode = false
	case "r":
		if model.idfPort == "" {
			return model.withNotice(idf.LogLevelWarn, "no device selected. press esc, then choose a device"), nil
		}
		model.partitionMode = false
		return model.startOperation(StateReadingPartitions, idf.ReadPartitionTableCmd(model.idfPort))
	}
	return model, nil
}

// Confirmation sub-state: a second, deliberate enter is required before
// anything gets written to the device.
func (model Model) handleFlashConfirmKey(keyName string) (Model, tea.Cmd) {
	switch keyName {
	case "enter", "y":
		opt := *model.browserConfirm
		model.browserConfirm = nil
		if model, ok := model.requireIdleWithPort("flash"); !ok {
			return model, nil
		}
		model.browserMode = false
		// the single long-lived WaitForLog loop already drains logChannel;
		// adding a second reader here would reorder log lines
		return model.startOperation(StateFlashing, idf.RunFlashBin(model.idfPort, opt, model.logChannel))
	case "esc", "n":
		model.browserConfirm = nil
	}
	return model, nil
}

func (model Model) handleBrowserKey(keyName string) (Model, tea.Cmd) {
	switch keyName {

	case "esc", "q":
		model.browserMode = false
		model.browserItems = nil
		model.browserCursor = 0
		model.browserConfirm = nil

	case "up", "k":
		if model.browserCursor > 0 {
			model.browserCursor--
		}

	case "down", "j":
		if model.browserCursor < len(model.browserItems)-1 {
			model.browserCursor++
		}

	case "home", "g":
		model.browserCursor = 0

	case "end", "G":
		model.browserCursor = max(len(model.browserItems)-1, 0)

	case "backspace", "left", "h":
		return model.browseUp()

	case "enter", "right", "l":
		if len(model.browserItems) == 0 {
			return model, nil
		}
		item := model.browserItems[model.browserCursor]

		switch {
		case item.Name == "..":
			return model.browseUp()

		case item.IsDir:
			model.browserReturnTo = ""
			return model, idf.BrowseDirCmd(filepath.Join(model.browserPath, item.Name))

		case item.IsFullFlash:
			if model.idfPort == "" {
				return model.withNotice(idf.LogLevelWarn, noDeviceText("flash")), nil
			}
			opt, err := idf.BuildFullFlashOption(model.browserPath)
			if err != nil {
				model = model.appendLog(idf.LogLine{
					Text:  "can't read flasher_args.json: " + err.Error() + ". rebuild the project with b, then try again",
					Level: idf.LogLevelError,
				})
				return model.withNotice(idf.LogLevelError, "can't read flasher_args.json. rebuild with b, then try again"), nil
			}
			model.browserConfirm = &opt

		default:
			if model.idfPort == "" {
				return model.withNotice(idf.LogLevelWarn, noDeviceText("flash")), nil
			}
			absPath := filepath.Join(model.browserPath, item.Name)
			model.browserConfirm = &idf.BinFlashOption{
				Label:     item.Name,
				BinPath:   absPath,
				FlashAddr: idf.GuessFlashAddr(absPath, item.Name, model.project.Target),
			}
		}
	}
	return model, nil
}

func (model Model) browseUp() (Model, tea.Cmd) {
	parent := filepath.Dir(model.browserPath)
	if parent == model.browserPath {
		return model, nil
	}
	model.browserReturnTo = filepath.Base(model.browserPath)
	return model, idf.BrowseDirCmd(parent)
}

func (model Model) applyDirLoaded(msg idf.DirLoadedMsg) Model {
	// an operation may have started between pressing x and the directory
	// arriving; opening the browser now would allow a second, concurrent
	// write to the device
	if model.isBusy() {
		return model
	}
	if msg.Err != nil {
		return model.appendLog(idf.LogLine{
			Text:  "can't open " + msg.Path + ": " + msg.Err.Error(),
			Level: idf.LogLevelError,
		})
	}
	model.browserPath = msg.Path
	model.browserItems = msg.Items
	model.browserCursor = 0
	if model.browserReturnTo != "" {
		if idx := slices.IndexFunc(msg.Items, func(e idf.FileBrowserEntry) bool {
			return e.IsDir && e.Name == model.browserReturnTo
		}); idx >= 0 {
			model.browserCursor = idx
		}
		model.browserReturnTo = ""
	}
	model.browserMode = true
	model.browserConfirm = nil
	return model
}

func (model Model) applyScan(devices []idf.Device) Model {
	model.scanning = false
	model.devices = devices

	if model.selectedDev >= len(model.devices) {
		model.selectedDev = max(0, len(model.devices)-1)
	}

	// the active port was unplugged: forget it rather than pointing flash
	// and erase at a device that is gone
	if model.idfPort != "" && !slices.ContainsFunc(devices, func(d idf.Device) bool { return d.Port == model.idfPort }) {
		model = model.appendLog(idf.LogLine{
			Text:  "device " + model.idfPort + " disconnected",
			Level: idf.LogLevelWarn,
		})
		model.idfPort = ""
	}

	// pick a device automatically only when there is no choice to make;
	// with several boards attached the user chooses, since f flashes
	// without a confirmation step
	if model.idfPort == "" && len(model.devices) == 1 {
		model.idfPort = model.devices[0].Port
		model.selectedDev = 0
		model = model.appendLog(idf.LogLine{
			Text:  "→ using " + model.idfPort + "  [" + model.devices[0].ChipType + "]",
			Level: idf.LogLevelSystem,
		})
	}
	return model
}

// Moves into a busy state and (re)starts the spinner.
func (model Model) startOperation(state AppState, command tea.Cmd) (Model, tea.Cmd) {
	model.state = state
	model.lastErr = nil
	return model, tea.Batch(command, model.spinner.Tick)
}

func (model Model) requestQuit() (Model, tea.Cmd) {
	if !model.isBusy() || model.quitArmed {
		return model, tea.Quit
	}
	model.quitArmed = true
	return model.withNotice(idf.LogLevelWarn,
		fmt.Sprintf("%s is still running. press q again to quit anyway, or any other key to stay", model.state)), nil
}

// Returns ok=false, with a footer notice explaining why, when an operation
// is already running.
func (model Model) requireIdle(action string) (Model, bool) {
	if !model.isBusy() {
		return model, true
	}
	text := fmt.Sprintf("can't %s while %s. wait for it to finish", action, model.state)
	if model.state == StateMonitoring {
		text = fmt.Sprintf("can't %s while monitoring. press m to stop the monitor first", action)
	}
	return model.withNotice(idf.LogLevelWarn, text), false
}

func (model Model) requireIdleWithPort(action string) (Model, bool) {
	model, ok := model.requireIdle(action)
	if !ok {
		return model, false
	}
	if model.idfPort == "" {
		return model.withNotice(idf.LogLevelWarn, noDeviceText(action)), false
	}
	return model, true
}

func noDeviceText(action string) string {
	return "can't " + action + ": no device selected. plug in a board and press r, or pick one in devices with enter"
}

func (model Model) withNotice(level idf.LogLevel, text string) Model {
	model.notice = &idf.LogLine{Text: text, Level: level}
	return model
}

// First line of an error, for places that only have room for a summary.
func firstLine(err error) string {
	for _, line := range strings.Split(err.Error(), "\n") {
		line = strings.Trim(line, " ─")
		if line != "" {
			return line
		}
	}
	return "unknown error"
}
