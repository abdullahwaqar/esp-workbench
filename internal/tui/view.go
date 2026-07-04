package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	idf "espworkbench/internal/espworkbench"

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

// Reports whether the terminal has room to show the ASCII logo above the
// header without crowding the panels below it. Narrow or short terminals
// fall back to the plain text title instead.
func (model Model) showLogo() bool {
	return model.width >= 60 && model.height >= 30
}

// Height reserved for header and footer chrome, used by viewBody and
// logViewDimensions so the body panels never overlap them. Grows when
// the ASCII logo is shown so both stay in sync automatically.
func (model Model) chromeHeight() int {
	base := 6 // header (2 lines) + footer (1 line) + safety margin
	if model.showLogo() {
		base += logoHeight + 1 // logo rows + one spacer line
	}
	return base
}

func (model Model) viewHeader() string {
	title := ""
	if !model.showLogo() {
		// the ASCII logo already spells out "esp", so the plain text
		// title is only needed when the logo isn't shown
		title = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("esp workbench")
	}

	projectName := ""
	if model.project.Name != "" {
		projectName = mutedStyle.Render(model.project.Name)
	}

	badge := model.viewStateBadge()

	titleWithProject := title
	if projectName != "" {
		if titleWithProject != "" {
			titleWithProject += "  "
		}
		titleWithProject += projectName
	}

	gapWidth := max(model.width-lipgloss.Width(titleWithProject)-lipgloss.Width(badge)-2, 1)
	gap := lipgloss.NewStyle().Width(gapWidth).Render("")

	line1 := lipgloss.JoinHorizontal(lipgloss.Top, titleWithProject, gap, badge)
	line2 := model.viewProjectInfo()

	lines := make([]string, 0, 4)
	if model.showLogo() {
		logo := lipgloss.NewStyle().Width(model.width).Align(lipgloss.Center).Render(renderLogo())
		lines = append(lines, logo, "")
	}
	lines = append(lines, line1, line2)

	return headerStyle.Width(model.width).Render(
		lipgloss.JoinVertical(lipgloss.Left, lines...),
	)
}

func (model Model) viewStateBadge() string {
	if model.browserConfirm != nil {
		return logWarnStyle.Render("confirm")
	}
	if model.browserMode {
		return accentStyle.Render("browse")
	}
	if model.partitionMode {
		return accentStyle.Render("partitions")
	}
	if model.state == StateIdle {
		return idleStatusStyle.Render("idle")
	}
	return activeStatusStyle.Render(model.spinner.View() + " " + model.state.String())
}

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

func (model Model) viewBody() string {
	deviceWidth := 28
	commandWidth := commandPanelWidth(model)
	logWidth := max(model.width-deviceWidth-commandWidth, 20)
	bodyHeight := max(model.height-model.chromeHeight(), 10)

	return lipgloss.JoinHorizontal(lipgloss.Top,
		model.viewDevicePanel(deviceWidth, bodyHeight),
		model.viewCommandPanel(commandWidth, bodyHeight),
		model.viewLogPanel(logWidth, bodyHeight),
	)
}

func (model Model) viewDevicePanel(width int, height int) string {
	innerWidth := width - 4
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))

	var b strings.Builder
	b.WriteString(panelTitleStyle.Render("devices") + "\n")
	b.WriteString(separator + "\n\n")

	if len(model.devices) == 0 {
		b.WriteString(mutedStyle.Render("  no devices found\n"))
		b.WriteString(mutedStyle.Render("  press r to scan\n"))
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
				b.WriteString(deviceSelectedStyle.Width(innerWidth).Render(portLine) + "\n")
			} else {
				b.WriteString(deviceNormalStyle.Render(portLine) + "\n")
			}
			b.WriteString(deviceChipStyle.Render("   "+device.ChipType) + "\n")
			if device.MAC != "" {
				b.WriteString(deviceChipStyle.Render("   "+device.MAC) + "\n")
			}
			b.WriteString("\n")
		}
	}

	if model.idfPort != "" {
		b.WriteString(separator + "\n")
		b.WriteString(portActiveStyle.Render("  active  "+model.idfPort) + "\n")
	}

	style := panelStyle
	if model.focusedPanel == PanelDevices {
		style = focusedPanelStyle
	}
	return style.Width(width).Height(height - 2).Render(b.String())
}

func (model Model) viewCommandPanel(width int, height int) string {
	if model.browserConfirm != nil {
		return model.viewConfirmPanel(width, height)
	}
	if model.browserMode {
		return model.viewBrowserPanel(width, height)
	}
	if model.partitionMode {
		return model.viewPartitionPanel(width, height)
	}

	innerWidth := width - 4
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))
	isRunning := model.state != StateIdle

	var b strings.Builder
	b.WriteString(panelTitleStyle.Render("commands") + "\n")
	b.WriteString(separator + "\n\n")

	type cmd struct {
		key       string
		desc      string
		needsPort bool
	}
	primary := []cmd{
		{"b", "build", false},
		{"f", "flash", true},
		{"a", "build + flash", true},
		{"m", "monitor", true},
		{"e", "erase flash", true},
		{"x", "flash binary", false},
		{"p", "view partitions", true},
	}
	for _, c := range primary {
		disabled := isRunning || (c.needsPort && model.idfPort == "")
		b.WriteString(renderKeyCommand(c.key, c.desc, disabled) + "\n\n")
	}

	b.WriteString(separator + "\n\n")

	misc := []cmd{
		{"r", "refresh devices", false},
		{"l", "clear logs", false},
		{"tab", "switch panel", false},
		{"q", "quit", false},
	}
	for _, c := range misc {
		b.WriteString(renderKeyCommand(c.key, c.desc, false) + "\n")
	}

	b.WriteString("\n" + separator + "\n")
	b.WriteString(model.viewStateInfo())

	style := panelStyle
	if model.focusedPanel == PanelCommands {
		style = focusedPanelStyle
	}
	return style.Width(width).Height(height - 2).Render(b.String())
}

func (model Model) viewBrowserPanel(width int, height int) string {
	innerWidth := width - 4
	panelInnerHeight := height - 4 // subtract border (2) + padding (2)
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))

	// header: title + sep + path + sep = 4 lines
	// footer: sep + port + hint        = 3 lines
	// items area
	headerLines := 4
	footerLines := 3
	visibleItems := max(panelInnerHeight-headerLines-footerLines, 2)

	// scroll offset: keep cursor visible
	scrollOff := 0
	if model.browserCursor >= visibleItems {
		scrollOff = model.browserCursor - visibleItems + 1
	}

	var b strings.Builder

	// title
	b.WriteString(panelTitleStyle.Render("flash binary") + "\n")
	b.WriteString(separator + "\n")

	// current path (truncated to fit inner width)
	b.WriteString(mutedStyle.Render(truncateBrowserPath(model.browserPath, innerWidth)) + "\n")
	b.WriteString(separator + "\n")

	// item list
	if len(model.browserItems) == 0 {
		b.WriteString(mutedStyle.Render("  empty directory\n"))
	} else {
		end := min(scrollOff+visibleItems, len(model.browserItems))
		for i := scrollOff; i < end; i++ {
			item := model.browserItems[i]
			isSelected := i == model.browserCursor
			line := renderBrowserEntry(item, innerWidth)
			if isSelected {
				b.WriteString(deviceSelectedStyle.Width(innerWidth).Render(line) + "\n")
			} else {
				b.WriteString(line + "\n")
			}
		}

		// scroll position indicator when list is longer than the visible area
		if len(model.browserItems) > visibleItems {
			indicator := fmt.Sprintf("  %d / %d", model.browserCursor+1, len(model.browserItems))
			b.WriteString(mutedStyle.Render(indicator) + "\n")
		}
	}

	// footer
	b.WriteString(separator + "\n")

	portStr := model.idfPort
	if portStr == "" {
		portStr = "no device selected"
	}
	b.WriteString(mutedStyle.Render("  "+portStr) + "\n")
	b.WriteString("  " + model.viewBrowserHint() + "\n")

	return focusedPanelStyle.Width(width).Height(height - 2).Render(b.String())
}

// Shows a deliberate one-step confirmation before any bytes
// are written to the device. This exists specifically to prevent a stray or
// buffered keypress from triggering a flash without the user intending it.
func (model Model) viewConfirmPanel(width int, height int) string {
	innerWidth := width - 4
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))
	opt := model.browserConfirm

	var b strings.Builder
	b.WriteString(panelTitleStyle.Render("confirm flash") + "\n")
	b.WriteString(separator + "\n\n")

	if opt.IsFullFlash {
		b.WriteString(accentStyle.Render("  full flash") + "\n")
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  %d files  ·  flasher_args.json", opt.FileCount)) + "\n\n")
	} else {
		label := opt.Label
		if len(label)+2 > innerWidth {
			label = label[:innerWidth-4] + ".."
		}
		b.WriteString(accentStyle.Render("  "+label) + "\n")
		b.WriteString(mutedStyle.Render("  address  "+opt.FlashAddr) + "\n\n")
	}

	b.WriteString(mutedStyle.Render("  target device") + "\n")
	b.WriteString(deviceNormalStyle.Render("  "+model.idfPort) + "\n\n")

	b.WriteString(logWarnStyle.Render("  this will overwrite flash on the device above") + "\n")

	b.WriteString("\n" + separator + "\n")
	b.WriteString("  " + accentStyle.Render("enter") + mutedStyle.Render(" confirm and flash") + "\n")
	b.WriteString("  " + accentStyle.Render("esc") + mutedStyle.Render(" cancel, back to browser") + "\n")

	return focusedPanelStyle.Width(width).Height(height - 2).Render(b.String())
}

func (model Model) viewPartitionPanel(width int, height int) string {
	innerWidth := width - 4
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))
	table := model.partitionTable

	var b strings.Builder
	b.WriteString(panelTitleStyle.Render("partitions") + "\n")

	if table.FlashSize > 0 {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  flash: %s  ·  %d partitions", humanBytes(table.FlashSize), len(table.Entries))) + "\n")
	} else {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  flash size unknown  ·  %d partitions", len(table.Entries))) + "\n")
	}
	b.WriteString(separator + "\n\n")

	barWidth := innerWidth
	if barWidth > 0 {
		b.WriteString(buildUsageBar(table, barWidth) + "\n\n")
	}

	totalRef := referenceTotal(table)
	for _, entry := range table.Entries {
		swatch := partitionStyle(entry).Render("■")
		name := entry.Name
		if len(name) > 12 {
			name = name[:12]
		}
		nameField := fmt.Sprintf("%-12s", name)

		pct := ""
		if totalRef > 0 {
			pct = fmt.Sprintf("%4.1f%%", float64(entry.Size)/float64(totalRef)*100)
		}

		line := swatch + " " + deviceNormalStyle.Render(nameField) +
			mutedStyle.Render(fmt.Sprintf(" %-9s 0x%06x  %8s  %s",
				entry.SubLabel, entry.Offset, humanBytes(uint64(entry.Size)), pct))

		if len(line) > innerWidth+10 { // ansi codes inflate len(); generous slack is fine here
			// fall back to a shorter line on very narrow panels
			line = swatch + " " + deviceNormalStyle.Render(nameField) +
				mutedStyle.Render(fmt.Sprintf(" %s", humanBytes(uint64(entry.Size))))
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n" + separator + "\n")
	b.WriteString("  " + accentStyle.Render("r") + mutedStyle.Render(" re-read") +
		mutedStyle.Render("  ·  ") + accentStyle.Render("esc") + mutedStyle.Render(" back"))

	return focusedPanelStyle.Width(width).Height(height - 2).Render(b.String())
}

// Picks the denominator for percentage and bar-width math:
// real flash size when known, otherwise the table's own extent.
func referenceTotal(table idf.PartitionTable) uint64 {
	if table.FlashSize > 0 {
		return table.FlashSize
	}
	var maxEnd uint64
	for _, e := range table.Entries {
		end := uint64(e.Offset) + uint64(e.Size)
		if end > maxEnd {
			maxEnd = end
		}
	}
	return maxEnd
}

// Renders one line of block characters representing the
// entire flash layout: a muted segment for the reserved bootloader and
// partition-table region, a colored segment per partition proportional to
// its size, and (when the real flash size is known) a dim "free" segment
// for anything left unallocated.
func buildUsageBar(table idf.PartitionTable, barWidth int) string {
	totalRef := referenceTotal(table)
	if totalRef == 0 || len(table.Entries) == 0 {
		return mutedStyle.Render(strings.Repeat("░", barWidth))
	}

	type segment struct {
		size  uint64
		style lipgloss.Style
	}
	var segments []segment

	firstOffset := uint64(table.Entries[0].Offset)
	if firstOffset > 0 {
		segments = append(segments, segment{size: firstOffset, style: mutedStyle})
	}

	for i, entry := range table.Entries {
		segments = append(segments, segment{size: uint64(entry.Size), style: partitionStyle(entry)})

		// gap between this partition's end and the next one's start (alignment padding)
		end := uint64(entry.Offset) + uint64(entry.Size)
		var nextStart uint64
		if i+1 < len(table.Entries) {
			nextStart = uint64(table.Entries[i+1].Offset)
		} else {
			nextStart = totalRef
		}
		if nextStart > end {
			segments = append(segments, segment{size: nextStart - end, style: lipgloss.NewStyle().Foreground(colorDim)})
		}
	}

	// largest-remainder allocation so segment widths sum exactly to barWidth
	type weighted struct {
		index     int
		raw       float64
		floor     int
		remainder float64
	}
	weights := make([]weighted, len(segments))
	floorSum := 0
	for i, seg := range segments {
		raw := float64(seg.size) / float64(totalRef) * float64(barWidth)
		floor := int(raw)
		weights[i] = weighted{index: i, raw: raw, floor: floor, remainder: raw - float64(floor)}
		floorSum += floor
	}
	remaining := barWidth - floorSum
	sort.SliceStable(weights, func(i, j int) bool { return weights[i].remainder > weights[j].remainder })
	widths := make([]int, len(segments))
	for i, w := range weights {
		widths[w.index] = w.floor
		if i < remaining {
			widths[w.index]++
		}
	}

	var bar strings.Builder
	for i, seg := range segments {
		w := widths[i]
		if w <= 0 {
			continue
		}
		bar.WriteString(seg.style.Render(strings.Repeat("█", w)))
	}
	return bar.String()
}

// Picks a color per partition by type/subtype so the bar and
// the detail rows read consistently at a glance.
func partitionStyle(entry idf.PartitionEntry) lipgloss.Style {
	switch {
	case entry.TypeLabel == "app":
		return lipgloss.NewStyle().Foreground(colorAccent)
	case entry.SubLabel == "nvs":
		return lipgloss.NewStyle().Foreground(colorSuccess)
	case entry.SubLabel == "ota_data":
		return lipgloss.NewStyle().Foreground(colorPurple)
	case entry.SubLabel == "phy":
		return lipgloss.NewStyle().Foreground(colorWarn)
	case entry.SubLabel == "coredump":
		return lipgloss.NewStyle().Foreground(colorError)
	case entry.SubLabel == "spiffs" || entry.SubLabel == "fat":
		return lipgloss.NewStyle().Foreground(colorPurple)
	default:
		return mutedStyle
	}
}

// Formats a byte count the way a person would expect to read it.
func humanBytes(size uint64) string {
	switch {
	case size >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
	case size >= 1024:
		return fmt.Sprintf("%d KB", size/1024)
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func renderBrowserEntry(item idf.FileBrowserEntry, innerWidth int) string {
	switch {
	case item.Name == "..":
		return mutedStyle.Render("  ..")

	case item.IsFullFlash:
		return accentStyle.Render("  [full flash]")

	case item.IsDir:
		name := item.Name + "/"
		if len(name)+2 > innerWidth {
			name = name[:innerWidth-4] + "../"
		}
		return mutedStyle.Render("  " + name)

	default:
		// .bin file
		label := item.Name
		size := item.SizeLabel
		// right-align size if it fits
		gap := innerWidth - 2 - len(label) - len(size)
		if gap >= 1 {
			padding := strings.Repeat(" ", gap)
			return deviceNormalStyle.Render("  "+label) +
				mutedStyle.Render(padding+size)
		}
		// truncate filename if line is too long
		if len(label)+2 > innerWidth {
			label = label[:innerWidth-4] + ".."
		}
		return deviceNormalStyle.Render("  " + label)
	}
}

func (model Model) viewBrowserHint() string {
	if model.browserCursor < 0 || model.browserCursor >= len(model.browserItems) {
		return mutedStyle.Render("no items")
	}
	item := model.browserItems[model.browserCursor]

	hint := func(k, desc string) string {
		return accentStyle.Render(k) + mutedStyle.Render(" "+desc)
	}

	if item.Name == ".." || item.IsDir {
		return hint("enter", "open") + mutedStyle.Render("  ·  ") +
			hint("bksp", "up") + mutedStyle.Render("  ·  ") +
			hint("esc", "cancel")
	}
	return hint("enter", "select") + mutedStyle.Render("  ·  ") +
		hint("bksp", "up") + mutedStyle.Render("  ·  ") +
		hint("esc", "cancel")
}

// Shortens a long path to fit in maxWidth by keeping the
// tail components and replacing the prefix with "...".
func truncateBrowserPath(path string, maxWidth int) string {
	if len(path) <= maxWidth {
		return path
	}
	sep := string(filepath.Separator)
	parts := strings.Split(path, sep)
	result := path
	for len(result) > maxWidth && len(parts) > 2 {
		parts = parts[1:]
		result = "..." + sep + strings.Join(parts, sep)
	}
	return result
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

func (model Model) viewLogPanel(width int, height int) string {
	innerWidth := width - 4
	separator := separatorStyle.Render(strings.Repeat("─", innerWidth))

	title := panelTitleStyle.Render("logs") +
		mutedStyle.Render(fmt.Sprintf("  %d lines", len(model.logs)))

	var b strings.Builder
	b.WriteString(title + "\n")
	b.WriteString(separator + "\n")
	b.WriteString(model.logViewport.View())

	style := panelStyle
	if model.focusedPanel == PanelLogs {
		style = focusedPanelStyle
	}
	return style.Width(width).Height(height - 2).Render(b.String())
}

func (model Model) viewFooter() string {
	hint := func(k, desc string) string {
		return accentStyle.Render(k) + mutedStyle.Render(" "+desc)
	}

	if model.browserConfirm != nil {
		parts := []string{
			hint("enter", "confirm and flash"),
			hint("esc", "cancel"),
		}
		return footerStyle.Render(strings.Join(parts, "  ·  "))
	}

	if model.browserMode {
		parts := []string{
			hint("↑↓", "navigate"),
			hint("enter", "open / select"),
			hint("bksp", "up"),
			hint("esc", "cancel"),
		}
		return footerStyle.Render(strings.Join(parts, "  ·  "))
	}

	if model.partitionMode {
		parts := []string{
			hint("r", "re-read"),
			hint("esc", "back"),
		}
		return footerStyle.Render(strings.Join(parts, "  ·  "))
	}

	parts := []string{
		mutedStyle.Render("↑↓ select"),
		hint("enter", "use device"),
		hint("b", "build"),
		hint("f", "flash"),
		hint("a", "build+flash"),
		hint("m", "monitor"),
		hint("e", "erase"),
		hint("x", "flash binary"),
		hint("p", "partitions"),
		hint("r", "refresh"),
		hint("tab", "panel"),
		hint("q", "quit"),
	}
	return footerStyle.Render(strings.Join(parts, "  ·  "))
}
