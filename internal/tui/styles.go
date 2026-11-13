package tui

import "github.com/charmbracelet/lipgloss"

// Every color is adaptive: lipgloss picks Light or Dark from the terminal's
// background, so the UI stays readable on light themes too. Dark values are
// the original opencode-inspired palette; light values are darker versions
// of the same hues that keep 4.5:1 against a white background.
//
// Each color also pins its 256-color and 16-color fallbacks. Left to the
// automatic conversion, termenv maps #e06c75 (error red) and #303030
// (panel border) to palette index 232, which is almost black, so on
// terminals without truecolor (macOS Terminal.app, tmux by default) every
// error line was unreadable.
type tone struct {
	hex     string
	ansi256 string
	ansi    string
}

func adaptive(light, dark tone) lipgloss.CompleteAdaptiveColor {
	return lipgloss.CompleteAdaptiveColor{
		Light: lipgloss.CompleteColor{TrueColor: light.hex, ANSI256: light.ansi256, ANSI: light.ansi},
		Dark:  lipgloss.CompleteColor{TrueColor: dark.hex, ANSI256: dark.ansi256, ANSI: dark.ansi},
	}
}

var (
	colorAccent   = adaptive(tone{"#a8541c", "130", "3"}, tone{"#fab283", "216", "11"}) // focus, keys, spinner
	colorPurple   = adaptive(tone{"#6b46c1", "61", "5"}, tone{"#9d7cd8", "140", "13"})  // panel titles
	colorSuccess  = adaptive(tone{"#1f7a33", "29", "2"}, tone{"#7fd88f", "114", "10"})
	colorError    = adaptive(tone{"#b42335", "124", "1"}, tone{"#e06c75", "168", "9"})
	colorWarn     = adaptive(tone{"#8a5a00", "94", "3"}, tone{"#e5c07b", "180", "3"})
	colorInfo     = adaptive(tone{"#1f5fbf", "25", "4"}, tone{"#5c9cf5", "75", "12"}) // active port
	colorMuted    = adaptive(tone{"#5f5f5f", "59", "8"}, tone{"#8a8a8a", "245", "7"}) // secondary text
	colorText     = adaptive(tone{"#1c1c1c", "234", "0"}, tone{"#eeeeee", "255", "15"})
	colorDim      = adaptive(tone{"#a8a8a8", "248", "7"}, tone{"#4a4a4a", "239", "8"}) // separators, disabled
	colorBorder   = adaptive(tone{"#d0d0d0", "252", "7"}, tone{"#303030", "236", "8"})
	colorSelected = adaptive(tone{"#f0f0f0", "255", "7"}, tone{"#282828", "235", "0"})  // selected row background
	colorOnAccent = adaptive(tone{"#ffffff", "231", "15"}, tone{"#0a0a0a", "232", "0"}) // text on accent fill

	// categorical partition colors; never reuse status hues for data
	colorPartCyan = adaptive(tone{"#0f7480", "30", "6"}, tone{"#56b6c2", "73", "14"})
	colorPartPink = adaptive(tone{"#a3307e", "132", "5"}, tone{"#d67ad2", "176", "13"})

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	focusedPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPurple)

	focusedTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	separatorStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	accentStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	textStyle = lipgloss.NewStyle().
			Foreground(colorText)

	selectedRowStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent).
				Background(colorSelected)

	portActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorInfo)

	activeStatusStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	keyBadgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorOnAccent).
			Background(colorAccent).
			Padding(0, 1)

	disabledStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	logInfoStyle    = lipgloss.NewStyle().Foreground(colorText)
	logWarnStyle    = lipgloss.NewStyle().Foreground(colorWarn)
	logErrorStyle   = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	logSuccessStyle = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	logSystemStyle  = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)
)
