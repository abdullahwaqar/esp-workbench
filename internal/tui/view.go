package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (model Model) View() string {
	if model.width == 0 {
		return "\n loading...\n"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		model.viewHeader(),
		model.viewBody(),
		model.viewFooter(),
	)
}

// ── Header ────────────────────────────────────────────────────────────────────

func (model Model) viewHeader() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("esp workbench")

	projectName := ""
	if model.project.Name != "" {
		projectName = mutedStyle.Render(model.project.Name)
	}

	badge := model.viewStateBadge()

	titleWithProject := title
	if projectName != "" {
		titleWithProject = title + "  " + projectName
	}

	gapWidth := model.width - lipgloss.Width(titleWithProject) - lipgloss.Width(badge) - 2
	if gapWidth < 1 {
		gapWidth = 1
	}
	gap := lipgloss.NewStyle().Width(gapWidth).Render("")

	line1 := lipgloss.JoinHorizontal(lipgloss.Top, titleWithProject, gap, badge)
	line2 := model.viewProjectInfo()

	return headerStyle.Width(model.width).Render(
		lipgloss.JoinVertical(lipgloss.Left, line1, line2),
	)
}

func (model Model) viewStateBadge() string {
	if model.state == StateIdle {
		if model.binSelectMode {
			return accentStyle.Render("select binary")
		}
		return idleStatusStyle.Render("idle")
	}
	return activeStatusStyle.Render(model.spinner.View() + " " + model.state.String())
}

// viewProjectInfo renders the second header line: project validity + metadata.
func (model Model) viewProjectInfo() string {
	p := model.project
	sep := mutedStyle.Render("  ·  ")

	if !p.IsValid {
		return mutedStyle.Render(p.ValidityLabel())
	}

	var parts []string

	parts = append(parts, accentStyle.Render(p.Target))

	if p.HasSDKConfig {
		parts = append(parts, logSuccessStyle.Render("configured"))
	} else {
		label := "not configured"
		if p.HasSDKDefaults {
			label = "not configured  (sdkconfig.defaults present)"
		}
		parts = append(parts, logWarnStyle.Render(label))
	}

	if p.HasComponents {
		label := fmt.Sprintf("%d component", p.ComponentCount)
		if p.ComponentCount != 1 {
			label = fmt.Sprintf("%d components", p.ComponentCount)
		}
		parts = append(parts, mutedStyle.Render(label))
	}

	if p.PartitionTable != "" {
		parts = append(parts, mutedStyle.Render("partitions: "+p.PartitionTable))
	}

	if p.ProjectVersion != "" {
		parts = append(parts, mutedStyle.Render("v"+p.ProjectVersion))
	}

	return strings.Join(parts, sep)
}

// ── Body ──────────────────────────────────────────────────────────────────────

func (model Model) viewBody() string {
	deviceWidth := 28
	commandWidth := 34
	logWidth := model.width - deviceWidth - commandWidth
	if logWidth < 20 {
		logWidth = 20
	}
	bodyHeight := model.height - 6
	if bodyHeight < 10 {
		bodyHeight = 10
	}

	return lipgloss.JoinHorizontal(lipgloss.Top,
		model.viewDevicePanel(deviceWidth, bodyHeight),
		model.viewCommandPanel(commandWidth, bodyHeight),
		model.viewLogPanel(logWidth, bodyHeight),
	)
}

// ── Device Panel ──────────────────────────────────────────────────────────────

func (model Model) viewDevicePanel(width int, height int) string {
	innerWidth := width - 4
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))

	var builder strings.Builder
	builder.WriteString(panelTitleStyle.Render("devices") + "\n")
	builder.WriteString(separator + "\n\n")

	if len(model.devices) == 0 {
		builder.WriteString(mutedStyle.Render("  no devices found\n"))
		builder.WriteString(mutedStyle.Render("  press r to scan\n"))
	} else {
		for index, device := range model.devices {
			isSelected := index == model.selectedDev
			isActive := device.Port == model.idfPort

			prefix := "  "
			if isActive {
				prefix = "> "
			}
			portLine := prefix + device.Port

			if isSelected {
				builder.WriteString(deviceSelectedStyle.Width(innerWidth).Render(portLine) + "\n")
			} else {
				builder.WriteString(deviceNormalStyle.Render(portLine) + "\n")
			}

			builder.WriteString(deviceChipStyle.Render("   "+device.ChipType) + "\n")

			if device.MAC != "" {
				builder.WriteString(deviceChipStyle.Render("   "+device.MAC) + "\n")
			}
			builder.WriteString("\n")
		}
	}

	if model.idfPort != "" {
		builder.WriteString(separator + "\n")
		builder.WriteString(portActiveStyle.Render("  active  "+model.idfPort) + "\n")
	}

	style := panelStyle
	if model.focusedPanel == PanelDevices {
		style = focusedPanelStyle
	}
	return style.Width(width).Height(height - 2).Render(builder.String())
}

// ── Command Panel ─────────────────────────────────────────────────────────────

func (model Model) viewCommandPanel(width int, height int) string {
	if model.binSelectMode {
		return model.viewBinPickerPanel(width, height)
	}

	innerWidth := width - 4
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))
	isRunning := model.state != StateIdle

	var builder strings.Builder
	builder.WriteString(panelTitleStyle.Render("commands") + "\n")
	builder.WriteString(separator + "\n\n")

	type commandOption struct {
		key       string
		desc      string
		needsPort bool
	}

	primaryCommands := []commandOption{
		{"b", "build", false},
		{"f", "flash", true},
		{"a", "build + flash", true},
		{"m", "monitor", true},
		{"e", "erase flash", true},
		{"x", "flash binary", false}, // no port required to open picker
	}
	for _, commandOpt := range primaryCommands {
		isDisabled := isRunning || (commandOpt.needsPort && model.idfPort == "")
		builder.WriteString(renderKeyCommand(commandOpt.key, commandOpt.desc, isDisabled) + "\n\n")
	}

	builder.WriteString(separator + "\n\n")

	miscCommands := []commandOption{
		{"r", "refresh devices", false},
		{"l", "clear logs", false},
		{"tab", "switch panel", false},
		{"q", "quit", false},
	}
	for _, commandOpt := range miscCommands {
		builder.WriteString(renderKeyCommand(commandOpt.key, commandOpt.desc, false) + "\n")
	}

	builder.WriteString("\n" + separator + "\n")
	builder.WriteString(model.viewStateInfo())

	style := panelStyle
	if model.focusedPanel == PanelCommands {
		style = focusedPanelStyle
	}
	return style.Width(width).Height(height - 2).Render(builder.String())
}

// viewBinPickerPanel replaces the commands panel when binSelectMode is active.
func (model Model) viewBinPickerPanel(width int, height int) string {
	innerWidth := width - 4
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))

	var builder strings.Builder
	builder.WriteString(panelTitleStyle.Render("flash binary") + "\n")
	builder.WriteString(separator + "\n\n")

	if len(model.binOptions) == 0 {
		builder.WriteString(mutedStyle.Render("  no binaries found\n"))
		builder.WriteString(mutedStyle.Render("  build the project first\n"))
	} else {
		for i, opt := range model.binOptions {
			isSelected := i == model.selectedBin

			label := opt.Label
			if len(label) > innerWidth-2 {
				label = label[:innerWidth-4] + ".."
			}

			line := "  " + label
			if isSelected {
				builder.WriteString(deviceSelectedStyle.Width(innerWidth).Render(line) + "\n")
			} else {
				builder.WriteString(deviceNormalStyle.Render(line) + "\n")
			}
			builder.WriteString(mutedStyle.Render("  "+opt.Description) + "\n\n")
		}
	}

	builder.WriteString(separator + "\n")

	if model.idfPort == "" {
		builder.WriteString(logWarnStyle.Render("  select a device first") + "\n")
	} else if model.selectedBin < 0 {
		// nothing selected yet — prompt navigation
		portHint := mutedStyle.Render("  " + model.idfPort + "\n")
		actionHint := accentStyle.Render("down/up") + mutedStyle.Render(" navigate  ·  ") + accentStyle.Render("esc") + mutedStyle.Render(" cancel")
		builder.WriteString(portHint)
		builder.WriteString("  " + actionHint + "\n")
	} else {
		portHint := mutedStyle.Render("  " + model.idfPort + "\n")
		actionHint := accentStyle.Render("enter") + mutedStyle.Render(" flash  ·  ") + accentStyle.Render("esc") + mutedStyle.Render(" cancel")
		builder.WriteString(portHint)
		builder.WriteString("  " + actionHint + "\n")
	}

	// always use focused style — the user is actively in this mode
	return focusedPanelStyle.Width(width).Height(height - 2).Render(builder.String())
}

func renderKeyCommand(keyName string, description string, isDisabled bool) string {
	if isDisabled {
		return keyBadgeDisabledStyle.Render("["+keyName+"]") +
			cmdLabelDisabledStyle.Render(" "+description)
	}
	return keyBadgeStyle.Render(keyName) + cmdLabelStyle.Render(" "+description)
}

func (model Model) viewStateInfo() string {
	switch {
	case model.state != StateIdle:
		return accentStyle.Render(model.spinner.View() + " " + model.state.String())
	case model.lastErr != nil:
		return logErrorStyle.Render("error: " + model.lastErr.Error())
	case model.idfPort == "":
		return mutedStyle.Render("no device selected")
	default:
		return logSuccessStyle.Render("ready")
	}
}

// ── Log Panel ─────────────────────────────────────────────────────────────────

func (model Model) viewLogPanel(width int, height int) string {
	innerWidth := width - 4
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))

	title := panelTitleStyle.Render("logs") +
		mutedStyle.Render(fmt.Sprintf("  %d lines", len(model.logs)))

	var builder strings.Builder
	builder.WriteString(title + "\n")
	builder.WriteString(separator + "\n")
	builder.WriteString(model.logViewport.View())

	style := panelStyle
	if model.focusedPanel == PanelLogs {
		style = focusedPanelStyle
	}
	return style.Width(width).Height(height - 2).Render(builder.String())
}

// ── Footer ────────────────────────────────────────────────────────────────────

func (model Model) viewFooter() string {
	hint := func(keyName string, description string) string {
		return accentStyle.Render(keyName) + mutedStyle.Render(" "+description)
	}

	if model.binSelectMode {
		parts := []string{
			hint("up/down", "navigate"),
			hint("enter", "flash"),
			hint("esc", "cancel"),
		}
		return footerStyle.Render(strings.Join(parts, "  ·  "))
	}

	parts := []string{
		mutedStyle.Render("up/down") + mutedStyle.Render(" select"),
		hint("enter", "use device"),
		hint("b", "build"),
		hint("f", "flash"),
		hint("a", "build+flash"),
		hint("m", "monitor"),
		hint("e", "erase"),
		hint("x", "flash bin"),
		hint("r", "refresh"),
		hint("tab", "panel"),
		hint("q", "quit"),
	}

	return footerStyle.Render(strings.Join(parts, "  ·  "))
}
