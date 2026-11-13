package tui

import (
	"errors"
	"strings"
	"testing"

	idf "espworkbench/internal/espworkbench"

	"github.com/charmbracelet/bubbles/spinner"
)

// Before: Init started a scan *and* a tick, and every DevicesScannedMsg
// queued another tick, so two scan loops ran from startup and each r press
// added one more. Now the tick loop is the only thing that schedules ticks.
func TestScanDoesNotScheduleAnotherTick(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m.scanning = true
	m, cmd := updateCmd(m, idf.DevicesScannedMsg(devices(1)))
	if cmd != nil {
		t.Error("DevicesScannedMsg returned a command; scans must not reschedule ticks")
	}
	if m.scanning {
		t.Error("scan result should clear the scanning flag")
	}
}

func TestTickSkipsScanWhileScanningOrBusy(t *testing.T) {
	m := newTestModel(t, 120, 40)

	m.scanning = true
	m = update(m, idf.TickMsg{})
	if !m.scanning {
		t.Fatal("tick cleared the scanning flag")
	}

	m.scanning = false
	m.state = StateMonitoring
	m = update(m, idf.TickMsg{})
	if m.scanning {
		t.Error("tick started a scan while monitoring; it would fight the monitor for the port")
	}

	m.state = StateIdle
	m = update(m, idf.TickMsg{})
	if !m.scanning {
		t.Error("tick should start a scan when idle")
	}
}

func TestManualRescan(t *testing.T) {
	m := newTestModel(t, 120, 40)

	m = press(m, "r")
	if !m.scanning {
		t.Fatal("r should start a scan")
	}
	m = press(m, "r")
	if m.notice == nil || !strings.Contains(m.notice.Text, "already scanning") {
		t.Errorf("second r during a scan should explain itself, got %+v", m.notice)
	}

	busy := withDevice(newTestModel(t, 120, 40))
	busy.state = StateMonitoring
	busy = press(busy, "r")
	if busy.scanning {
		t.Error("r must not probe ports while the monitor owns one")
	}
	if busy.notice == nil || !strings.Contains(busy.notice.Text, "press m to stop the monitor") {
		t.Errorf("expected a notice naming the way out, got %+v", busy.notice)
	}
}

// Before: the bubbles viewport defaults bound b, f, d, u, l and space, so
// with the log focused, b started a build and paged the log at once.
func TestCommandKeysDoNotScrollLog(t *testing.T) {
	m := withLogs(withDevice(newTestModel(t, 120, 40)), 300)
	m = press(m, "tab")
	if m.focusedPanel != PanelLogs {
		t.Fatal("tab should focus the log")
	}
	m = press(m, "pgup")
	offset := m.logViewport.YOffset

	m = press(m, "b")
	if m.state != StateBuilding {
		t.Errorf("b should build, state=%v", m.state)
	}
	if m.logViewport.YOffset != offset {
		t.Errorf("b also scrolled the log from %d to %d", offset, m.logViewport.YOffset)
	}
	m.state = StateIdle
	for _, k := range []string{"d", "u", "h", " "} {
		m = press(m, k)
		if m.logViewport.YOffset != offset {
			t.Errorf("%q scrolled the log", k)
		}
	}
	m = press(m, "l")
	if len(m.logs) != 0 {
		t.Error("l should clear logs")
	}
}

func TestLogScrollKeysAndFollow(t *testing.T) {
	m := withLogs(withDevice(newTestModel(t, 120, 40)), 300)
	m = press(m, "tab")
	if !m.logViewport.AtBottom() {
		t.Fatal("log should start at the bottom")
	}
	m = press(m, "up")
	if m.logViewport.AtBottom() {
		t.Fatal("up should scroll the focused log")
	}
	offset := m.logViewport.YOffset

	// new output while scrolled up must not yank the reader to the bottom
	m = update(m, idf.LogMsg{Text: "new line", Level: idf.LogLevelInfo})
	if m.logViewport.YOffset != offset {
		t.Errorf("new output moved a scrolled-up log from %d to %d", offset, m.logViewport.YOffset)
	}
	if !strings.Contains(plainView(m), "end to follow") {
		t.Error("scrolled-up log needs a cue that more output is below")
	}

	m = press(m, "end")
	if !m.logViewport.AtBottom() {
		t.Error("end should jump to the newest output")
	}
	m = update(m, idf.LogMsg{Text: "another", Level: idf.LogLevelInfo})
	if !m.logViewport.AtBottom() {
		t.Error("log at the bottom should keep following new output")
	}
	m = press(m, "home")
	if m.logViewport.YOffset != 0 {
		t.Error("home should jump to the oldest output")
	}
}

// Before: lines added by key handlers ("→ selected ...") weren't drawn until
// some later LogMsg happened to refresh the viewport.
func TestKeyHandlerLogsAppearImmediately(t *testing.T) {
	m := update(newTestModel(t, 120, 40), idf.DevicesScannedMsg(devices(3)))
	m = press(m, "down", "enter")
	if m.idfPort != "/dev/ttyUSB1" {
		t.Fatalf("enter should select the highlighted device, got %q", m.idfPort)
	}
	if !strings.Contains(m.logViewport.View(), "selected /dev/ttyUSB1") {
		t.Error("selection log line is not visible in the viewport")
	}
}

func TestTabTogglesBetweenFocusablePanels(t *testing.T) {
	m := newTestModel(t, 120, 40)
	seen := []Panel{m.focusedPanel}
	for range 4 {
		m = press(m, "tab")
		seen = append(seen, m.focusedPanel)
	}
	for _, p := range seen {
		if p == PanelCommands {
			t.Error("tab stopped on the command panel, which has nothing to operate")
		}
	}
	m = press(m, "shift+tab")
	if m.focusedPanel != PanelLogs {
		t.Errorf("shift+tab should move focus too, got %v", m.focusedPanel)
	}
}

func TestEraseNeedsConfirmation(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))

	m = press(m, "e")
	if m.state != StateIdle || !m.eraseConfirm {
		t.Fatalf("e should ask first, state=%v confirm=%v", m.state, m.eraseConfirm)
	}
	m = press(m, "b") // unrelated keys are ignored while confirming
	if m.state != StateIdle || !m.eraseConfirm {
		t.Fatal("other keys must not act or dismiss the erase prompt")
	}
	m = press(m, "esc")
	if m.eraseConfirm || m.state != StateIdle {
		t.Fatal("esc should cancel the erase")
	}
	if m.notice == nil || !strings.Contains(m.notice.Text, "nothing was changed") {
		t.Error("cancel should confirm nothing happened")
	}

	m = press(m, "e", "enter")
	if m.state != StateErasing {
		t.Errorf("enter should erase, state=%v", m.state)
	}
}

func TestEraseConfirmDropsIfDeviceVanishes(t *testing.T) {
	m := press(withDevice(newTestModel(t, 120, 40)), "e")
	m = update(m, idf.DevicesScannedMsg(nil))
	m = press(m, "enter")
	if m.state == StateErasing {
		t.Fatal("erased a device that was unplugged")
	}
}

func TestActionsWithoutDeviceExplainWhy(t *testing.T) {
	for _, k := range []string{"f", "a", "m", "e", "p"} {
		m := press(newTestModel(t, 120, 40), k)
		if m.state != StateIdle || m.eraseConfirm {
			t.Errorf("%s acted with no device", k)
		}
		if m.notice == nil || !strings.Contains(m.notice.Text, "no device selected") || !strings.Contains(m.notice.Text, "press r") {
			t.Errorf("%s: notice should say why and how to fix it, got %+v", k, m.notice)
		}
	}
}

func TestBusyKeysExplainWhy(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m.state = StateBuilding
	for _, k := range []string{"b", "f", "a", "m", "e", "x", "p"} {
		next := press(m, k)
		if next.state != StateBuilding || next.eraseConfirm {
			t.Errorf("%s acted while building", k)
		}
		if next.notice == nil || !strings.Contains(next.notice.Text, "wait for it to finish") {
			t.Errorf("%s: expected busy notice, got %+v", k, next.notice)
		}
	}
}

func TestNoticeClearsOnNextKey(t *testing.T) {
	m := press(newTestModel(t, 120, 40), "f")
	if m.notice == nil {
		t.Fatal("expected a notice")
	}
	m = press(m, "down")
	if m.notice != nil {
		t.Error("notice should clear on the next key")
	}
}

func TestBuildBlockedOutsideProject(t *testing.T) {
	m := newModel(idf.ProjectContext{Path: "/tmp", Name: "tmp", Target: "esp32"})
	m.scanning = false
	m = press(resize(m, 120, 40), "b")
	if m.state != StateIdle {
		t.Fatal("built outside an esp-idf project")
	}
	if m.notice == nil || !strings.Contains(m.notice.Text, "missing CMakeLists.txt") {
		t.Errorf("notice should name what's missing, got %+v", m.notice)
	}
}

func TestQuitWhileBusyNeedsSecondPress(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	if _, cmd := pressCmd(m, "q"); !isQuit(cmd) {
		t.Fatal("q should quit when idle")
	}

	m.state = StateFlashing
	m, cmd := pressCmd(m, "q")
	if isQuit(cmd) {
		t.Fatal("a single q quit in the middle of a flash")
	}
	if !m.quitArmed {
		t.Fatal("first q should arm quit")
	}
	if _, cmd := pressCmd(m, "q"); !isQuit(cmd) {
		t.Error("second q should quit")
	}

	disarmed := press(m, "down")
	if disarmed.quitArmed {
		t.Error("any other key should disarm quit")
	}
	if _, cmd := pressCmd(m, "ctrl+c"); !isQuit(cmd) {
		t.Error("ctrl+c always quits")
	}
}

func TestOperationLifecycle(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m = press(m, "f")
	if m.state != StateFlashing {
		t.Fatalf("f should flash, state=%v", m.state)
	}
	m = update(m, idf.OperationDoneMsg{Err: errors.New("exit status 2")})
	if m.state != StateIdle || m.lastErr == nil {
		t.Fatal("failed operation should return to idle with the error kept")
	}
	m = press(m, "b")
	if m.lastErr != nil {
		t.Error("starting a new operation should clear the old error")
	}
	m = update(m, idf.OperationDoneMsg{})
	if m.state != StateIdle || m.lastErr != nil {
		t.Error("successful operation should leave no error")
	}
}

func TestSpinnerOnlyTicksWhileBusy(t *testing.T) {
	m := newTestModel(t, 120, 40)
	if _, cmd := updateCmd(m, spinner.TickMsg{}); cmd != nil {
		if msg := cmd(); msg != nil {
			t.Errorf("idle spinner kept ticking (%T); it redraws the screen ten times a second for nothing", msg)
		}
	}
	m.state = StateBuilding
	if _, cmd := updateCmd(m, m.spinner.Tick()); cmd == nil {
		t.Error("busy spinner should keep ticking")
	}
}

func TestMonitorLifecycle(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m = press(m, "m")
	if m.state != StateMonitoring || m.monitorDone == nil {
		t.Fatal("m should start the monitor")
	}

	m = press(m, "m")
	if m.notice == nil || !strings.Contains(m.notice.Text, "still starting") {
		t.Error("m before the pty exists should say the monitor is starting")
	}

	failed := update(m, idf.MonitorStartedMsg{Err: errors.New("failed to start monitor: no pty")})
	if failed.state != StateIdle || failed.monitorDone != nil {
		t.Error("failed monitor start should return to idle")
	}

	m = update(m, idf.MonitorDoneMsg{})
	if m.state != StateIdle || m.monitorPty != nil {
		t.Error("monitor done should return to idle")
	}
}

func TestDisconnectedActivePortIsCleared(t *testing.T) {
	m := update(newTestModel(t, 120, 40), idf.DevicesScannedMsg(devices(3)))
	m = press(m, "down", "down", "enter")
	if m.idfPort != "/dev/ttyUSB2" {
		t.Fatalf("setup: idfPort=%q", m.idfPort)
	}
	m = update(m, idf.DevicesScannedMsg(devices(2)))
	if m.idfPort != "" {
		t.Errorf("unplugged port %q is still the flash target", m.idfPort)
	}
	if m.selectedDev != 1 {
		t.Errorf("cursor should clamp to the last device, got %d", m.selectedDev)
	}
	if !strings.Contains(renderLogs(m.logs, 200), "disconnected") {
		t.Error("disconnect should be logged")
	}
}

func TestAutoSelectOnlyWhenThereIsNoChoice(t *testing.T) {
	m := update(newTestModel(t, 120, 40), idf.DevicesScannedMsg(devices(2)))
	if m.idfPort != "" {
		t.Errorf("auto-selected %q among two boards", m.idfPort)
	}
	m = update(newTestModel(t, 120, 40), idf.DevicesScannedMsg(devices(1)))
	if m.idfPort != "/dev/ttyUSB0" {
		t.Errorf("single board should be selected automatically, got %q", m.idfPort)
	}
	// a rescan must not replace the user's choice
	m = update(m, idf.DevicesScannedMsg(devices(2)))
	if m.idfPort != "/dev/ttyUSB0" {
		t.Errorf("rescan changed the active port to %q", m.idfPort)
	}
}

func TestBrowserFlow(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m = update(m, idf.DirLoadedMsg{Path: "/p/build", Items: []idf.FileBrowserEntry{
		{Name: "..", IsDir: true},
		{Name: "bootloader", IsDir: true},
		{Name: "app.bin", Size: 900_000, SizeLabel: "878 KB"},
	}})
	if !m.browserMode {
		t.Fatal("dir load should open the browser")
	}

	m = press(m, "end", "enter")
	if m.browserConfirm == nil {
		t.Fatal("enter on a .bin should stage a confirmation")
	}
	if m.state != StateIdle {
		t.Fatal("staging must not flash")
	}
	// app.bin isn't a merged image, so it goes to the app offset, not 0x0
	if m.browserConfirm.FlashAddr != "0x10000" {
		t.Errorf("app.bin address = %s, want 0x10000", m.browserConfirm.FlashAddr)
	}

	m = press(m, "esc")
	if m.browserConfirm != nil || !m.browserMode {
		t.Fatal("esc on the confirmation should return to the list")
	}

	m = press(m, "enter", "y")
	if m.state != StateFlashing || m.browserMode {
		t.Errorf("y should flash and close the browser, state=%v browser=%v", m.state, m.browserMode)
	}
}

// Before: the confirm screen added a second WaitForLog reader, so log lines
// from every later operation could arrive out of order.
func TestFlashConfirmDoesNotAddLogReader(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m.browserMode = true
	m.browserConfirm = &idf.BinFlashOption{Label: "a.bin", BinPath: "/x/a.bin", FlashAddr: "0x10000"}
	_, cmd := pressCmd(m, "enter")
	if cmd == nil {
		t.Fatal("expected flash command")
	}
	// the batch holds the flash command and the spinner tick; a WaitForLog
	// would block forever on the empty channel, so run nothing here and
	// just count the commands
	if n := batchLen(cmd); n != 2 {
		t.Errorf("confirm returned %d commands, want 2 (flash + spinner)", n)
	}
}

func TestBrowserIgnoredIfOperationStarted(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m = press(m, "x", "b") // directory load still in flight when b is pressed
	m = update(m, idf.DirLoadedMsg{Path: "/p/build", Items: []idf.FileBrowserEntry{{Name: "app.bin"}}})
	if m.browserMode {
		t.Error("browser opened mid-build; its confirm would start a second concurrent write")
	}
}

func TestFlashConfirmRefusedWhileBusy(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m.browserMode = true
	m.browserConfirm = &idf.BinFlashOption{Label: "a.bin"}
	m.state = StateBuilding
	m = press(m, "enter")
	if m.state != StateBuilding {
		t.Error("confirm flashed during a build")
	}
}

func TestBrowserBinWithoutDevice(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = update(m, idf.DirLoadedMsg{Path: "/p", Items: []idf.FileBrowserEntry{{Name: "a.bin"}, {Name: "[full flash]", IsFullFlash: true}}})
	m = press(m, "enter")
	if m.browserConfirm != nil || m.notice == nil {
		t.Error("selecting a bin with no device should explain, not stage")
	}
	m = press(m, "down", "enter")
	if m.browserConfirm != nil || m.notice == nil {
		t.Error("full flash with no device should explain, not stage")
	}
}

func TestBrowserGoingUpKeepsPlace(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m = update(m, idf.DirLoadedMsg{Path: "/p/build/bootloader", Items: []idf.FileBrowserEntry{{Name: "..", IsDir: true}}})
	m, cmd := pressCmd(m, "backspace")
	if cmd == nil || m.browserReturnTo != "bootloader" {
		t.Fatalf("backspace should load the parent and remember %q, got %q", "bootloader", m.browserReturnTo)
	}
	m = update(m, idf.DirLoadedMsg{Path: "/p/build", Items: []idf.FileBrowserEntry{
		{Name: "..", IsDir: true}, {Name: "a", IsDir: true}, {Name: "bootloader", IsDir: true}, {Name: "x.bin"},
	}})
	if m.browserCursor != 2 {
		t.Errorf("cursor should land on the folder we left, got %d", m.browserCursor)
	}
}

func TestBrowserCursorBounds(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m = update(m, idf.DirLoadedMsg{Path: "/", Items: []idf.FileBrowserEntry{{Name: "a", IsDir: true}, {Name: "b.bin"}}})
	m = press(m, "up", "up")
	if m.browserCursor != 0 {
		t.Error("cursor went above the list")
	}
	m = press(m, "down", "down", "down")
	if m.browserCursor != 1 {
		t.Error("cursor went past the list")
	}
	if _, cmd := pressCmd(m, "backspace"); cmd != nil {
		t.Error("going up from / should do nothing")
	}
	m = press(m, "esc")
	if m.browserMode || m.browserItems != nil {
		t.Error("esc should close and reset the browser")
	}
}

func TestEmptyBrowserEnterIsSafe(t *testing.T) {
	m := update(withDevice(newTestModel(t, 120, 40)), idf.DirLoadedMsg{Path: "/tmp/empty"})
	m = press(m, "enter", "down", "up", "end")
	if m.browserConfirm != nil {
		t.Error("staged a flash from an empty folder")
	}
}

func TestDirLoadErrorIsLogged(t *testing.T) {
	m := update(newTestModel(t, 120, 40), idf.DirLoadedMsg{Path: "/root/secret", Err: errors.New("permission denied")})
	if m.browserMode {
		t.Error("browser opened on a failed load")
	}
	if !strings.Contains(renderLogs(m.logs, 200), "can't open /root/secret") {
		t.Error("load error should name the folder")
	}
}

func TestPartitionFlow(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m = press(m, "p")
	if m.state != StateReadingPartitions {
		t.Fatal("p should read partitions")
	}
	m = update(m, idf.PartitionsReadMsg{Table: partitionTable(3, 4<<20)})
	if !m.partitionMode || m.state != StateIdle {
		t.Fatal("successful read should open the partition view")
	}
	m = press(m, "r")
	if m.partitionMode || m.state != StateReadingPartitions {
		t.Error("r should re-read")
	}
	m = update(m, idf.PartitionsReadMsg{Err: errors.New("no partition table found")})
	if m.partitionMode || m.partitionErr == nil {
		t.Error("failed read should stay on the main view with the error kept")
	}

	m = update(m, idf.PartitionsReadMsg{Table: partitionTable(3, 0)})
	m.idfPort = ""
	m = press(m, "r")
	if m.notice == nil || m.state != StateIdle {
		t.Error("r without a device should explain instead of reading")
	}
	m = press(m, "q")
	if m.partitionMode {
		t.Error("q should leave the partition view")
	}
}

func TestProgressRedrawsCollapseToOneLine(t *testing.T) {
	m := newTestModel(t, 120, 40)
	m = m.appendLog(idf.LogLine{Text: "Compressed 912384 bytes"})
	for _, pct := range []string{"5", "50", "100"} {
		m = m.appendLog(idf.LogLine{Text: "Writing at 0x00010000... (" + pct + " %)"})
	}
	m = m.appendLog(idf.LogLine{Text: "Wrote 912384 bytes"})
	if len(m.logs) != 3 {
		t.Fatalf("got %d log lines, want 3: %+v", len(m.logs), m.logs)
	}
	if !strings.Contains(m.logs[1].Text, "(100 %)") {
		t.Errorf("progress line should hold the latest value, got %q", m.logs[1].Text)
	}
}

func TestLogHistoryIsCapped(t *testing.T) {
	m := withLogs(newTestModel(t, 120, 40), maxLogLines+50)
	if len(m.logs) != maxLogLines {
		t.Fatalf("kept %d lines, want %d", len(m.logs), maxLogLines)
	}
	if !strings.Contains(m.logs[0].Text, "line 050") {
		t.Errorf("oldest lines should be dropped first, first=%q", m.logs[0].Text)
	}
}

func TestModelCopiesDoNotShareLogs(t *testing.T) {
	a := withLogs(newTestModel(t, 120, 40), 3)
	b := a.appendLog(idf.LogLine{Text: "only in b"})
	_ = a.appendLog(idf.LogLine{Text: "only in a"})
	if strings.Contains(renderLogs(b.logs, 200), "only in a") {
		t.Error("appending to one model copy changed another")
	}
}

func TestResizeKeepsLogScrollPosition(t *testing.T) {
	m := withLogs(withDevice(newTestModel(t, 120, 40)), 300)
	m = press(m, "tab", "pgup")
	offset := m.logViewport.YOffset
	m = resize(m, 121, 40)
	if m.logViewport.AtBottom() || m.logViewport.YOffset == 0 {
		t.Errorf("resize lost the scroll position (was %d, now %d)", offset, m.logViewport.YOffset)
	}
}

func TestFirstLine(t *testing.T) {
	err := &idf.PermissionError{ErrorMessage: "\n──────\npermission denied on serial device\n──────"}
	if got := firstLine(err); got != "permission denied on serial device" {
		t.Errorf("firstLine = %q", got)
	}
	if got := firstLine(errors.New("\n\n")); got != "unknown error" {
		t.Errorf("firstLine of blank = %q", got)
	}
}

func TestInitialModelIsUsable(t *testing.T) {
	m := InitialModel(t.TempDir())
	if m.Init() == nil {
		t.Fatal("Init should start the scan, tick and log loops")
	}
	m = resize(m, 100, 30)
	assertFits(t, "initial model", m)
	if m.project.IsValid {
		t.Error("an empty temp dir is not an esp-idf project")
	}
}
