package tui

import (
	"regexp"
	"slices"
	"strings"

	idf "espworkbench/internal/espworkbench"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const tabWidth = 8

var (
	// CSI (colors, cursor moves), OSC (titles, hyperlinks) and any other
	// two-byte escape. Tool output is restyled by level, so all are dropped.
	csiSequence   = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	oscSequence   = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	otherEscape   = regexp.MustCompile(`\x1b[@-Z\\-_]`)
	progressShape = regexp.MustCompile(`\(\s*\d{1,3}\s*%\)\s*$`)
)

// Adds a line to the history and refreshes the viewport. Consecutive
// progress redraws ("Writing at 0x... (42 %)") replace each other so a
// flash shows one live progress line rather than a hundred.
func (model Model) appendLog(logLine idf.LogLine) Model {
	// copy on write: Model is passed by value, and two copies must never
	// share (and silently edit) one backing array
	logs := slices.Clone(model.logs)
	if n := len(logs); n > 0 && isProgressLine(logLine.Text) && isProgressLine(logs[n-1].Text) {
		logs[n-1] = logLine
	} else {
		logs = append(logs, logLine)
	}
	if len(logs) > maxLogLines {
		logs = logs[len(logs)-maxLogLines:]
	}
	model.logs = logs
	return model.refreshLogContent()
}

func isProgressLine(text string) bool {
	return progressShape.MatchString(sanitizeLogText(text))
}

// Re-renders the log content, keeping the reader's place: the view follows
// new output only while it is already scrolled to the bottom.
func (model Model) refreshLogContent() Model {
	follow := model.logViewport.AtBottom() || model.logViewport.TotalLineCount() <= model.logViewport.Height
	offset := model.logViewport.YOffset
	model.logViewport.SetContent(renderLogs(model.logs, model.logViewport.Width))
	if follow {
		model.logViewport.GotoBottom()
	} else {
		model.logViewport.SetYOffset(offset)
	}
	return model
}

// Rebuilds the viewport when the log panel's size changes (window resize,
// entering or leaving the browser), keeping the scroll position.
func (model Model) syncLogViewport() Model {
	width, height := model.logViewDimensions()
	if width == model.logViewport.Width && height == model.logViewport.Height {
		return model
	}
	follow := model.logViewport.AtBottom() || model.logViewport.TotalLineCount() <= model.logViewport.Height
	offset := model.logViewport.YOffset

	model.logViewport = newLogViewport(width, height)
	model.logViewport.SetContent(renderLogs(model.logs, width))
	if follow {
		model.logViewport.GotoBottom()
	} else {
		model.logViewport.SetYOffset(offset)
	}
	return model
}

func renderLogs(logs []idf.LogLine, width int) string {
	var sb strings.Builder
	for _, logLine := range logs {
		style := logStyleFor(logLine.Level)
		for _, physicalLine := range wrapLine(sanitizeLogText(logLine.Text), width) {
			sb.WriteString(style.Render(physicalLine))
			sb.WriteString("\n")
		}
	}
	return strings.TrimSuffix(sb.String(), "\n")
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

// Splits text on embedded newlines and wraps each piece to width display
// cells (wide characters count as two), breaking at spaces where possible
// and mid-word only for words longer than the line.
func wrapLine(text string, width int) []string {
	var lines []string
	for _, piece := range strings.Split(text, "\n") {
		if width <= 0 || piece == "" {
			lines = append(lines, piece)
			continue
		}
		lines = append(lines, strings.Split(ansi.Wrap(piece, width, ""), "\n")...)
	}
	return lines
}

// Makes one line of tool output safe to draw inside a panel: strips escape
// sequences and control characters, keeps only what follows the last bare
// carriage return (the final state of a redrawn line) and expands tabs, all
// of which would otherwise break the panel borders.
func sanitizeLogText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimRight(text, "\r\n")

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if idx := strings.LastIndex(line, "\r"); idx != -1 {
			line = line[idx+1:]
		}
		line = oscSequence.ReplaceAllString(line, "")
		line = csiSequence.ReplaceAllString(line, "")
		line = otherEscape.ReplaceAllString(line, "")
		lines[i] = expandTabs(stripControl(line))
	}
	return strings.Join(lines, "\n")
}

func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return -1
		}
		return r
	}, s)
}

func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			spaces := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", spaces))
			col += spaces
			continue
		}
		b.WriteRune(r)
		col += ansi.StringWidth(string(r))
	}
	return b.String()
}
