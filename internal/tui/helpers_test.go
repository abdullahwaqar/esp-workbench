package tui

import (
	"fmt"
	"strings"
	"testing"

	idf "espworkbench/internal/espworkbench"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func validProject() idf.ProjectContext {
	return idf.ProjectContext{
		Path:           "/tmp/blink",
		Name:           "blink",
		IsValid:        true,
		HasCMake:       true,
		HasMain:        true,
		Target:         "esp32-s3",
		HasSDKConfig:   true,
		HasComponents:  true,
		ComponentCount: 3,
		PartitionTable: "partitions.csv",
		ProjectVersion: "1.4.2",
	}
}

// A model sized to w×h with no devices and no pending scan.
func newTestModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := newModel(validProject())
	m.scanning = false
	return resize(m, w, h)
}

func resize(m Model, w, h int) Model {
	return update(m, tea.WindowSizeMsg{Width: w, Height: h})
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func updateCmd(m Model, msg tea.Msg) (Model, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		m = update(m, keyMsg(k))
	}
	return m
}

func pressCmd(m Model, k string) (Model, tea.Cmd) {
	return updateCmd(m, keyMsg(k))
}

func keyMsg(k string) tea.KeyMsg {
	switch k {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "pgup":
		return tea.KeyMsg{Type: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyMsg{Type: tea.KeyPgDown}
	case "home":
		return tea.KeyMsg{Type: tea.KeyHome}
	case "end":
		return tea.KeyMsg{Type: tea.KeyEnd}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func devices(n int) []idf.Device {
	out := make([]idf.Device, n)
	for i := range out {
		out[i] = idf.Device{
			Port:     fmt.Sprintf("/dev/ttyUSB%d", i),
			ChipType: "ESP32-S3",
			MAC:      fmt.Sprintf("aa:bb:cc:dd:ee:%02x", i),
		}
	}
	return out
}

// Model with one device detected and auto-selected.
func withDevice(m Model) Model {
	return update(m, idf.DevicesScannedMsg(devices(1)))
}

func withLogs(m Model, n int) Model {
	for i := range n {
		m = m.appendLog(idf.LogLine{Text: fmt.Sprintf("line %03d of build output", i), Level: idf.LogLevel(i % 5)})
	}
	return m
}

// Asserts the frame is exactly the terminal's height and no line is wider
// than the terminal. Bubble Tea clips anything wider and drops rows from
// the top of anything taller, so either one hides content.
func assertFits(t *testing.T, name string, m Model) {
	t.Helper()
	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Errorf("%s at %dx%d: rendered %d rows, want exactly %d", name, m.width, m.height, len(lines), m.height)
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("%s at %dx%d: row %d is %d cells wide: %q", name, m.width, m.height, i, w, stripANSI(line))
			break
		}
	}
}

func stripANSI(s string) string {
	return csiSequence.ReplaceAllString(s, "")
}

func plainView(m Model) string {
	return stripANSI(m.View())
}

// Runs cmd and reports whether it (or a batch it returns) quits. Only call
// this on commands known not to start real processes.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.QuitMsg:
		return true
	case tea.BatchMsg:
		for _, c := range msg {
			if isQuit(c) {
				return true
			}
		}
	}
	return false
}

// Number of commands in a batch, without running any of them.
func batchLen(cmd tea.Cmd) int {
	if cmd == nil {
		return 0
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		return len(batch)
	}
	return 1
}
