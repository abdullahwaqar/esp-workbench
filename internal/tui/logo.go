package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// logoHeight is the row count of the pixel-font glyphs below, before any
// scaling. Kept as a constant so chromeHeight() and renderLogo() can't
// drift out of sync with each other.
const logoHeight = 5

// 5-row pixel-font glyphs used to spell "ESP" in the header. Each row is
// 5 characters wide; doubleWidth() stretches them horizontally for visual
// weight without adding to the vertical footprint.
var logoGlyphs = map[rune][5]string{
	'E': {
		"█████",
		"█    ",
		"████ ",
		"█    ",
		"█████",
	},
	'S': {
		"█████",
		"█    ",
		"█████",
		"    █",
		"█████",
	},
	'P': {
		"█████",
		"█   █",
		"█████",
		"█    ",
		"█    ",
	},
}

// Renders the "ESP" wordmark as blocky ASCII art with "workbench" set
// beside it, meant to be centered across the full header width.
func renderLogo() string {
	letters := []rune{'E', 'S', 'P'}
	rows := make([]string, logoHeight)
	for i, letter := range letters {
		glyph := logoGlyphs[letter]
		for row := 0; row < logoHeight; row++ {
			if i > 0 {
				rows[row] += "  "
			}
			rows[row] += doubleWidth(glyph[row])
		}
	}

	art := make([]string, logoHeight)
	for i, row := range rows {
		art[i] = accentStyle.Render(row)
	}

	wordmark := strings.Join([]string{
		"",
		panelTitleStyle.Render("workbench"),
		"",
		mutedStyle.Render("esp-idf toolkit"),
		"",
	}, "\n")

	return lipgloss.JoinHorizontal(lipgloss.Top,
		strings.Join(art, "\n"),
		"   ",
		wordmark,
	)
}

// Stretches a row of glyph characters to twice its width so the logo
// reads clearly without needing extra vertical space.
func doubleWidth(row string) string {
	var b strings.Builder
	for _, r := range row {
		b.WriteRune(r)
		b.WriteRune(r)
	}
	return b.String()
}
