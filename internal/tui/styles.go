package tui

import "github.com/charmbracelet/lipgloss"

var (
	darkStep1  = lipgloss.Color("#0a0a0a")
	darkStep2  = lipgloss.Color("#141414")
	darkStep3  = lipgloss.Color("#1e1e1e")
	darkStep4  = lipgloss.Color("#282828")
	darkStep6  = lipgloss.Color("#3c3c3c")
	darkStep11 = lipgloss.Color("#808080")
	darkStep12 = lipgloss.Color("#eeeeee")

	opencodePrimary   = lipgloss.Color("#fab283") // warm orange
	opencodeSecondary = lipgloss.Color("#5c9cf5") // blue
	opencodeAccent    = lipgloss.Color("#9d7cd8") // purple
	opencodeRed       = lipgloss.Color("#e06c75")
	opencodeGreen     = lipgloss.Color("#7fd88f")
	opencodeYellow    = lipgloss.Color("#e5c07b")
)

var (
	colorAccent   = opencodePrimary // focus highlight, spinner
	colorPurple   = opencodeAccent  // titles
	colorSuccess  = opencodeGreen
	colorError    = opencodeRed
	colorWarn     = opencodeYellow
	colorMuted    = darkStep11        // secondary text
	colorSelected = darkStep4         // selection background
	colorBorder   = darkStep4         // dim border
	colorBorderHi = opencodePrimary   // focused border highlight
	colorText     = darkStep12        // off-white text
	colorDim      = darkStep6         // barely visible text
	colorInfo     = opencodeSecondary // reserved for secondary/info accents

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	focusedPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorderHi).
				Padding(0, 1)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPurple)

	separatorStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	accentStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	deviceNormalStyle = lipgloss.NewStyle().
				Foreground(colorText)

	deviceSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent).
				Background(colorSelected)

	deviceChipStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	portActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorInfo)

	idleStatusStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	activeStatusStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	keyBadgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(darkStep1).
			Background(colorAccent).
			Padding(0, 1)

	keyBadgeDisabledStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	cmdLabelStyle = lipgloss.NewStyle().
			Foreground(colorText)

	cmdLabelDisabledStyle = lipgloss.NewStyle().
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
