package tui

import "github.com/charmbracelet/lipgloss"

var (
	// ── Palette ───────────────────────────────────────────────────────────────
	colorAccent   = lipgloss.Color("#00D9FF") // electric cyan
	colorPurple   = lipgloss.Color("#BD93F9") // purple for titles
	colorSuccess  = lipgloss.Color("#50FA7B") // green for success
	colorError    = lipgloss.Color("#FF5555") // red for errors
	colorWarn     = lipgloss.Color("#FFB86C") // amber for warnings
	colorMuted    = lipgloss.Color("#6272A4") // slate for secondary text
	colorSelected = lipgloss.Color("#44475A") // selection background
	colorBorder   = lipgloss.Color("#3A3A5C") // dim border
	colorBorderHi = lipgloss.Color("#00D9FF") // focused border highlight
	colorText     = lipgloss.Color("#F8F8F2") // off-white text
	colorDim      = lipgloss.Color("#3D3F55") // barely visible text
	colorBg       = lipgloss.Color("#0A0A1A") // header background

	// ── Panels ────────────────────────────────────────────────────────────────
	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	focusedPanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorderHi).
				Padding(0, 1)

	// ── Header ────────────────────────────────────────────────────────────────
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Background(colorBg).
			Padding(0, 1)

	// ── Typography ────────────────────────────────────────────────────────────
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

	// ── Device list ───────────────────────────────────────────────────────────
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
			Foreground(colorAccent)

	// ── Status ────────────────────────────────────────────────────────────────
	idleStatusStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	activeStatusStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent)

	// ── Key bindings ──────────────────────────────────────────────────────────
	keyBadgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0A0A1A")).
			Background(colorAccent).
			Padding(0, 1)

	keyBadgeDisabledStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	cmdLabelStyle = lipgloss.NewStyle().
			Foreground(colorText)

	cmdLabelDisabledStyle = lipgloss.NewStyle().
				Foreground(colorDim)

	// ── Logs ──────────────────────────────────────────────────────────────────
	logInfoStyle    = lipgloss.NewStyle().Foreground(colorText)
	logWarnStyle    = lipgloss.NewStyle().Foreground(colorWarn)
	logErrorStyle   = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	logSuccessStyle = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	logSystemStyle  = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	// ── Footer ────────────────────────────────────────────────────────────────
	footerStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)
)
