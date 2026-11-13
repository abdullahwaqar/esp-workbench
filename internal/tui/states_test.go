package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	idf "espworkbench/internal/espworkbench"
)

var termSizes = [][2]int{
	{80, 24},  // classic default, must work
	{80, 16},  // minimum supported
	{90, 20},  // between breakpoints
	{100, 30}, // wide layout kicks in
	{120, 40}, // logo shown
	{200, 60},
	{79, 24}, // too narrow
	{80, 15}, // too short
	{40, 10},
	{1, 1},
}

func partitionTable(n int, flashSize uint64) idf.PartitionTable {
	entries := []idf.PartitionEntry{
		{Name: "nvs", TypeLabel: "data", SubLabel: "nvs", Offset: 0x9000, Size: 0x6000},
		{Name: "phy_init", TypeLabel: "data", SubLabel: "phy", Offset: 0xf000, Size: 0x1000},
		{Name: "factory", TypeLabel: "app", SubLabel: "factory", Offset: 0x10000, Size: 0x100000},
	}
	for i := 3; i < n; i++ {
		entries = append(entries, idf.PartitionEntry{
			Name: fmt.Sprintf("storage_%d", i), TypeLabel: "data", SubLabel: "spiffs",
			Offset: uint32(0x110000 + (i-3)*0x10000), Size: 0x10000,
		})
	}
	return idf.PartitionTable{Entries: entries, FlashSize: flashSize}
}

func browserItems(n int) []idf.FileBrowserEntry {
	items := []idf.FileBrowserEntry{
		{Name: "..", IsDir: true},
		{Name: "[full flash]", IsFullFlash: true},
		{Name: "bootloader", IsDir: true},
		{Name: "a-very-long-directory-name-that-will-not-fit-in-the-panel", IsDir: true},
	}
	for i := 0; len(items) < n; i++ {
		items = append(items, idf.FileBrowserEntry{
			Name: fmt.Sprintf("firmware_build_variant_%02d_with_a_long_name.bin", i), Size: 912_384, SizeLabel: "891 KB",
		})
	}
	return items
}

// Every state the app can be in, built through Update where possible so
// the transitions themselves are exercised too.
func allStates(t *testing.T, w, h int) map[string]Model {
	t.Helper()
	base := newTestModel(t, w, h)
	dev := withDevice(base)
	states := map[string]Model{}

	states["idle, no devices"] = base

	scanning := base
	scanning.scanning = true
	states["idle, scanning"] = scanning

	states["idle, one device"] = dev

	many := update(base, idf.DevicesScannedMsg(devices(14)))
	many = press(many, "enter")
	for range 13 {
		many = press(many, "down")
	}
	states["idle, 14 devices, last selected"] = many

	longPort := update(base, idf.DevicesScannedMsg([]idf.Device{{Port: "/dev/cu.usbserial-0001A2B3C4D5E6F7", ChipType: "ESP32-C6 (QFN40)", MAC: "40:4c:ca:ff:fe:12:34:56"}}))
	states["idle, long port name"] = longPort

	invalid := newModel(idf.ProjectContext{Path: "/tmp/x", Name: "not-a-project", Target: "esp32"})
	invalid.scanning = false
	states["invalid project"] = resize(invalid, w, h)

	for _, s := range []AppState{StateBuilding, StateFlashing, StateMonitoring, StateErasing, StateReadingPartitions} {
		busy := dev
		busy.state = s
		states["busy: "+s.String()] = busy
	}

	failed := update(dev, idf.OperationDoneMsg{Err: &idf.PermissionError{
		ErrorMessage: "\n──────\npermission denied on serial device\n──────\n\ndevice  : /dev/ttyUSB0\nuser    : dev\n\nfix: add user to dialout group",
	}})
	states["last operation failed (multi-line error)"] = failed

	states["notice shown"] = press(base, "f")

	quitArmed := dev
	quitArmed.state = StateFlashing
	quitArmed = press(quitArmed, "q")
	states["busy, quit armed"] = quitArmed

	states["erase confirm"] = press(dev, "e")

	browser := update(dev, idf.DirLoadedMsg{Path: "/home/dev/projects/very/deep/nested/folder/structure/blink/build", Items: browserItems(60)})
	for range 45 {
		browser = press(browser, "down")
	}
	states["browser, long list scrolled"] = browser

	states["browser, empty folder"] = update(dev, idf.DirLoadedMsg{Path: "/tmp/empty"})

	noPortBrowser := update(base, idf.DirLoadedMsg{Path: "/tmp/b", Items: browserItems(6)})
	states["browser, no device"] = noPortBrowser

	confirm := dev
	confirm.browserMode = true
	confirm.browserPath = "/tmp/build"
	confirm.browserItems = browserItems(6)
	confirm.browserConfirm = &idf.BinFlashOption{
		Label:     "firmware_build_variant_00_with_a_really_really_long_file_name_that_wraps.bin",
		BinPath:   "/tmp/build/firmware.bin",
		FlashAddr: "0x10000",
	}
	states["confirm single binary"] = confirm

	full := confirm
	full.browserConfirm = &idf.BinFlashOption{Label: "full flash", IsFullFlash: true, FileCount: 3}
	states["confirm full flash"] = full

	states["partitions, flash size known"] = update(dev, idf.PartitionsReadMsg{Table: partitionTable(5, 4*1024*1024)})
	states["partitions, flash size unknown"] = update(dev, idf.PartitionsReadMsg{Table: partitionTable(3, 0)})
	states["partitions, many entries"] = update(dev, idf.PartitionsReadMsg{Table: partitionTable(30, 16*1024*1024)})

	logs := withLogs(dev, 300)
	logs = press(logs, "tab", "pgup", "pgup")
	states["logs focused, scrolled up"] = logs

	nasty := dev.appendLog(idf.LogLine{Text: "\x1b[0;31mE (123) wifi:\tconnect\tfailed 中文字符宽度测试 🔥🔥🔥 \x1b]0;title\x07 trailing " + strings.Repeat("x", 300)})
	nasty = nasty.appendLog(idf.LogLine{Text: "progress\rWriting at 0x00010000... (5 %)\rWriting at 0x00014000... (10 %)"})
	nasty = nasty.appendLog(idf.LogLine{Text: "line one\nline two\r\nline three"})
	states["logs with escapes, tabs, wide chars"] = nasty

	return states
}

func TestEveryStateFitsEveryTerminalSize(t *testing.T) {
	for _, size := range termSizes {
		for name, m := range allStates(t, size[0], size[1]) {
			assertFits(t, name, m)
		}
	}
}

// A resize in the middle of any state must keep the frame fitting, and
// return to the full layout after a too-small window grows again.
func TestResizeRoundTrip(t *testing.T) {
	for name, m := range allStates(t, 120, 40) {
		for _, size := range termSizes {
			m = resize(m, size[0], size[1])
			assertFits(t, name+" after resize", m)
		}
		m = resize(m, 120, 40)
		if strings.Contains(plainView(m), "terminal too small") {
			t.Errorf("%s: still showing the too-small screen after growing back", name)
		}
	}
}

func TestTooSmallScreenNamesTheFix(t *testing.T) {
	m := newTestModel(t, 70, 20)
	view := plainView(m)
	for _, want := range []string{"terminal too small", "70×20", "80×16", "press q to quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("too-small view missing %q:\n%s", want, view)
		}
	}
	// keys other than q are ignored so nothing runs blind
	m = withDevice(m)
	m = press(m, "e", "enter", "b")
	if m.state != StateIdle || m.eraseConfirm {
		t.Errorf("keys acted while the screen was too small: state=%v eraseConfirm=%v", m.state, m.eraseConfirm)
	}
}

func TestStateViewsShowExpectedContent(t *testing.T) {
	states := allStates(t, 120, 40)
	cases := map[string][]string{
		"idle, no devices":                         {"no devices found", "press r to rescan", "no device selected"},
		"idle, scanning":                           {"scanning"},
		"idle, one device":                         {"/dev/ttyUSB0 ●", "active /dev/ttyUSB0", "ready", "ESP32-S3"},
		"idle, 14 devices, last selected":          {"› /dev/ttyUSB13", "more"},
		"invalid project":                          {"not an esp-idf project", "[b] build"},
		"busy: building":                           {"building", "[f] flash"},
		"busy: monitoring":                         {"monitoring"},
		"last operation failed (multi-line error)": {"failed: permission", "details are in the logs"},
		"notice shown":                             {"can't flash: no device selected"},
		"busy, quit armed":                         {"press q again to quit anyway"},
		"erase confirm":                            {"erase flash?", "/dev/ttyUSB0", "this cannot be", "enter erase flash", "esc cancel"},
		"browser, long list scrolled":              {"› firmware_build_variant_41", "46/60", "↑", "↓", "more"},
		"browser, empty folder":                    {"no folders or .bin files here"},
		"browser, no device":                       {"no device selected"},
		"confirm single binary":                    {"confirm flash", "really_really_", "address 0x10000", "enter flash it"},
		"confirm full flash":                       {"full flash", "3 files from flasher_args.json"},
		"partitions, flash size known":             {"4.0 MB flash", "5 partitions", "factory", "▓ bootloader + table"},
		"partitions, flash size unknown":           {"flash size unknown"},
		"partitions, many entries":                 {"more. make the terminal taller"},
		"logs focused, scrolled up":                {"▸ logs", "end to follow"},
	}
	for name, wants := range cases {
		m, ok := states[name]
		if !ok {
			t.Fatalf("no state named %q", name)
		}
		view := plainView(m)
		for _, want := range wants {
			if !strings.Contains(view, want) {
				t.Errorf("%s: view missing %q", name, want)
			}
		}
	}
}

func TestLogoOnlyWhenTall(t *testing.T) {
	if !strings.Contains(plainView(newTestModel(t, 120, 40)), "workbench") || !newTestModel(t, 120, 40).showLogo() {
		t.Error("expected logo at 120x40")
	}
	if newTestModel(t, 120, 30).showLogo() {
		t.Error("logo should be hidden at 120x30")
	}
}

func TestBusyStateDisablesCommandsWithTextCue(t *testing.T) {
	m := withDevice(newTestModel(t, 120, 40))
	m.state = StateBuilding
	view := plainView(m)
	for _, k := range []string{"[b] build", "[f] flash", "[e] erase flash", "[r] rescan devices"} {
		if !strings.Contains(view, k) {
			t.Errorf("busy view should mark %q unavailable with brackets", k)
		}
	}
}

func TestPanelWidthsAddUp(t *testing.T) {
	for _, w := range []int{80, 99, 100, 160} {
		for _, wide := range []bool{false, true} {
			m := newTestModel(t, w, 30)
			m.partitionMode = wide
			l := m.layout()
			if l.deviceW+l.commandW+l.logW != w {
				t.Errorf("width %d wide=%v: panels sum to %d", w, wide, l.deviceW+l.commandW+l.logW)
			}
			if l.logW < logMinWidth {
				t.Errorf("width %d wide=%v: log panel only %d wide", w, wide, l.logW)
			}
		}
	}
}

func TestMultiLineErrorDoesNotBreakCommandPanel(t *testing.T) {
	m := withDevice(newTestModel(t, 80, 24))
	m = update(m, idf.OperationDoneMsg{Err: errors.New("line one\nline two\nline three\nline four")})
	assertFits(t, "multi-line error", m)
	if strings.Contains(plainView(m), "line two") && !strings.Contains(plainView(m), "logs") {
		t.Error("error detail should stay in the log panel")
	}
}

func TestLongNoticeWrapsInsteadOfLosingTheFix(t *testing.T) {
	m := press(newTestModel(t, 80, 24), "a")
	assertFits(t, "long notice", m)
	view := plainView(m)
	if !strings.Contains(view, "press r") || !strings.Contains(view, "enter") {
		t.Errorf("the recovery half of the notice was cut off:\n%s", view)
	}
	if h := m.layout().bodyH; h != 24-m.headerHeight()-2 {
		t.Errorf("body should shrink by one row for a two-line notice, bodyH=%d", h)
	}
}
