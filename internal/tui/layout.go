package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Smallest terminal the three-panel layout fits in. 80×24 is the classic
// default and must work; below this the app shows a resize message instead
// of a clipped, unusable screen.
const (
	minTermWidth  = 80
	minTermHeight = 16

	logMinWidth = 24

	// a footer notice wraps onto at most this many lines
	maxNoticeLines = 2

	hintSeparator = "  ·  "
	ellipsis      = "…"
)

// Outer sizes, borders included, of the three body panels. deviceW is 0
// when the device panel is hidden to make room for the browser or the
// partition view on narrower terminals.
type panelLayout struct {
	deviceW  int
	commandW int
	logW     int
	bodyH    int
	tooSmall bool
}

// A "wide mode" swaps the command list for the file browser, the flash
// confirmation or the partition view, which need more columns.
func (model Model) wideMode() bool {
	return model.browserMode || model.partitionMode || model.browserConfirm != nil
}

func (model Model) tooSmall() bool {
	return model.width < minTermWidth || model.height < minTermHeight
}

// Single source of truth for panel geometry, used by both the view and the
// log viewport sizing so the two can never drift apart.
func (model Model) layout() panelLayout {
	if model.tooSmall() {
		return panelLayout{tooSmall: true}
	}

	l := panelLayout{bodyH: model.height - model.headerHeight() - lipgloss.Height(model.viewFooter())}

	if model.wideMode() {
		l.deviceW, l.commandW = 28, 52
		if model.width-l.deviceW-l.commandW < logMinWidth {
			l.deviceW = 0
		}
		if model.width-l.commandW < logMinWidth {
			l.commandW = model.width - logMinWidth
		}
	} else {
		l.deviceW, l.commandW = 28, 34
		if model.width < 100 {
			l.deviceW, l.commandW = 26, 30
		}
	}
	l.logW = model.width - l.deviceW - l.commandW
	return l
}

func (model Model) showLogo() bool {
	return model.width >= minTermWidth && model.height >= 34
}

func (model Model) headerHeight() int {
	return lipgloss.Height(model.viewHeader())
}

// Content area of a panel: outer size minus border (1 each side) and
// horizontal padding (1 each side).
func innerSize(width, height int) (int, int) {
	return max(width-4, 1), max(height-2, 1)
}

func (model Model) logViewDimensions() (width int, height int) {
	l := model.layout()
	if l.tooSmall {
		return 60, 20
	}
	innerW, innerH := innerSize(l.logW, l.bodyH)
	// title + separator take two lines
	return innerW, max(innerH-2, 1)
}

// Draws a bordered panel of exactly width×height cells. Each line is cut to
// the inner width and extra lines are dropped; callers are expected to fit
// their content (with scroll cues) first, this is only the safety net that
// keeps one panel from pushing the whole screen out of shape.
func renderPanel(lines []string, width, height int, focused bool) string {
	innerW, innerH := innerSize(width, height)
	if len(lines) > innerH {
		lines = lines[:innerH]
	}
	fitted := make([]string, len(lines))
	for i, line := range lines {
		fitted[i] = ansi.Truncate(line, innerW, ellipsis)
	}
	style := panelStyle
	if focused {
		style = focusedPanelStyle
	}
	return style.Width(width - 2).Height(innerH).Render(strings.Join(fitted, "\n"))
}

// Title line plus separator. A focused panel is marked by a "▸" glyph and
// the accent color, not by border color alone.
func panelHeader(title string, meta string, innerW int, focused bool) []string {
	var head string
	if focused {
		head = focusedTitleStyle.Render("▸ " + title)
	} else {
		head = panelTitleStyle.Render("  " + title)
	}
	if meta != "" {
		gap := innerW - lipgloss.Width(head) - lipgloss.Width(meta)
		if gap >= 2 {
			head += strings.Repeat(" ", gap) + meta
		} else {
			head += "  " + meta
		}
	}
	return []string{head, separatorLine(innerW)}
}

func separatorLine(width int) string {
	return separatorStyle.Render(strings.Repeat("─", max(width, 0)))
}

// Joins parts with the hint separator, dropping trailing parts that don't
// fit in width. Parts are given in priority order, most important first.
func fitParts(parts []string, width int) string {
	var out string
	for _, part := range parts {
		candidate := part
		if out != "" {
			candidate = out + mutedStyle.Render(hintSeparator) + part
		}
		if lipgloss.Width(candidate) > width {
			break
		}
		out = candidate
	}
	return out
}

// Word-wraps plain text to width and returns the lines, each indented by
// indent and rendered with style. Used where the full value must stay
// readable rather than being cut off.
func wrapText(text string, width int, indent string, style lipgloss.Style) []string {
	avail := max(width-lipgloss.Width(indent), 1)
	wrapped := ansi.Wrap(text, avail, "/-_.")
	var lines []string
	for _, line := range strings.Split(wrapped, "\n") {
		lines = append(lines, indent+style.Render(line))
	}
	return lines
}

func keyHint(key, desc string) string {
	return accentStyle.Render(key) + mutedStyle.Render(" "+desc)
}
