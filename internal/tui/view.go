package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	idf "espworkbench/internal/espworkbench"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (model Model) View() string {
	if model.width == 0 {
		return "\n loading...\n"
	}
	if model.tooSmall() {
		return model.viewTooSmall()
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		model.viewHeader(),
		model.viewBody(),
		model.viewFooter(),
	)
}

func (model Model) viewTooSmall() string {
	lines := []string{
		accentStyle.Render("terminal too small"),
		"",
		textStyle.Render(fmt.Sprintf("%d×%d, needs at least %d×%d", model.width, model.height, minTermWidth, minTermHeight)),
		mutedStyle.Render("make the window bigger, or press q to quit"),
	}
	if model.isBusy() {
		lines = append(lines, "", activeStatusStyle.Render(model.spinner.View()+" "+model.state.String()))
	}
	block := lipgloss.JoinVertical(lipgloss.Center, lines...)
	// clip in case even the message is wider than the terminal
	out := lipgloss.Place(model.width, model.height, lipgloss.Center, lipgloss.Center, block)
	rows := strings.Split(out, "\n")
	if len(rows) > model.height {
		rows = rows[:model.height]
	}
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, model.width, "")
	}
	return strings.Join(rows, "\n")
}

func (model Model) viewHeader() string {
	innerW := max(model.width-2, 1) // headerStyle pads one column each side

	badge := model.viewStateBadge()

	left := ""
	if !model.showLogo() {
		// the ASCII logo already spells out "esp"
		left = lipgloss.NewStyle().Bold(true).Foreground(colorAccent).Render("esp workbench")
	}
	if model.project.Name != "" {
		name := mutedStyle.Render(model.project.Name)
		if left != "" {
			left += "  "
		}
		left += name
	}
	maxLeft := max(innerW-lipgloss.Width(badge)-2, 1)
	left = ansi.Truncate(left, maxLeft, ellipsis)
	gap := strings.Repeat(" ", max(innerW-lipgloss.Width(left)-lipgloss.Width(badge), 1))
	line1 := left + gap + badge
	line2 := model.viewProjectInfo(innerW)

	lines := make([]string, 0, 4)
	if model.showLogo() {
		logo := lipgloss.NewStyle().Width(innerW).Align(lipgloss.Center).Render(renderLogo())
		lines = append(lines, logo, "")
	}
	lines = append(lines, line1, line2)

	return headerStyle.Width(model.width).Render(strings.Join(lines, "\n"))
}

func (model Model) viewStateBadge() string {
	switch {
	case model.isBusy():
		return activeStatusStyle.Render(model.spinner.View() + " " + model.state.String())
	case model.eraseConfirm:
		return logWarnStyle.Render("confirm erase")
	case model.browserConfirm != nil:
		return logWarnStyle.Render("confirm flash")
	case model.browserMode:
		return accentStyle.Render("browse")
	case model.partitionMode:
		return accentStyle.Render("partitions")
	default:
		return mutedStyle.Render("idle")
	}
}

// Second header line. Parts are listed most important first and the ones
// that don't fit are dropped whole, never cut mid-word.
func (model Model) viewProjectInfo(width int) string {
	p := model.project
	if !p.IsValid {
		return logWarnStyle.Render(ansi.Truncate(p.ValidityLabel()+" (not an esp-idf project)", width, ellipsis))
	}

	parts := []string{accentStyle.Render(p.Target)}

	if p.HasSDKConfig {
		parts = append(parts, logSuccessStyle.Render("configured"))
	} else {
		label := "not configured"
		if p.HasSDKDefaults {
			label = "not configured (sdkconfig.defaults present)"
		}
		parts = append(parts, logWarnStyle.Render(label))
	}

	if p.HasComponents {
		label := fmt.Sprintf("%d components", p.ComponentCount)
		if p.ComponentCount == 1 {
			label = "1 component"
		}
		parts = append(parts, mutedStyle.Render(label))
	}
	if p.ProjectVersion != "" {
		parts = append(parts, mutedStyle.Render("v"+p.ProjectVersion))
	}
	if p.PartitionTable != "" {
		parts = append(parts, mutedStyle.Render("partitions: "+p.PartitionTable))
	}

	return fitParts(parts, width)
}

func (model Model) viewBody() string {
	l := model.layout()
	var panels []string
	if l.deviceW > 0 {
		panels = append(panels, model.viewDevicePanel(l.deviceW, l.bodyH))
	}
	panels = append(panels,
		model.viewCommandPanel(l.commandW, l.bodyH),
		model.viewLogPanel(l.logW, l.bodyH),
	)
	return lipgloss.JoinHorizontal(lipgloss.Top, panels...)
}

func (model Model) viewDevicePanel(width int, height int) string {
	innerW, innerH := innerSize(width, height)
	focused := model.focusedPanel == PanelDevices && !model.wideMode() && !model.eraseConfirm

	meta := ""
	if model.scanning && !model.isBusy() {
		meta = mutedStyle.Render("scanning")
	}
	lines := panelHeader("devices", meta, innerW, focused)

	var footer []string
	if model.idfPort != "" {
		footer = []string{separatorLine(innerW), portActiveStyle.Render("● ") + mutedStyle.Render("active ") + portActiveStyle.Render(model.idfPort)}
	}

	if len(model.devices) == 0 {
		lines = append(lines, "",
			mutedStyle.Render("no devices found"),
			mutedStyle.Render("plug in a board, then"),
			mutedStyle.Render("press ")+accentStyle.Render("r")+mutedStyle.Render(" to rescan"),
		)
		return renderPanel(padTo(lines, footer, innerH), width, height, focused)
	}

	// one block per device: port, chip, optional MAC, blank spacer
	blocks := make([][]string, len(model.devices))
	for i, device := range model.devices {
		isSelected := i == model.selectedDev
		isActive := device.Port == model.idfPort

		marker := "  "
		if isSelected {
			marker = "› "
		}
		portText := device.Port
		if isActive {
			portText += " ●"
		}
		portLine := marker + portText
		if isSelected {
			portLine = selectedRowStyle.Width(innerW).Render(ansi.Truncate(portLine, innerW, ellipsis))
		} else {
			portLine = textStyle.Render(portLine)
		}
		block := []string{portLine, mutedStyle.Render("  " + device.ChipType)}
		if device.MAC != "" {
			block = append(block, mutedStyle.Render("  "+device.MAC))
		}
		blocks[i] = append(block, "")
	}

	avail := innerH - len(lines) - len(footer)
	start, end := blockWindow(blocks, model.selectedDev, avail)
	if start > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("↑ %d more", start)))
	}
	for _, block := range blocks[start:end] {
		lines = append(lines, block...)
	}
	if end < len(blocks) {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("↓ %d more", len(blocks)-end)))
	}
	return renderPanel(padTo(lines, footer, innerH), width, height, focused)
}

// Picks the range of variable-height blocks to show in avail lines, keeping
// the selected one visible and reserving a line for each "more" cue.
func blockWindow(blocks [][]string, selected int, avail int) (start, end int) {
	total := 0
	for _, b := range blocks {
		total += len(b)
	}
	if total <= avail {
		return 0, len(blocks)
	}
	fits := func(s, e int) bool {
		need := 0
		for _, b := range blocks[s:e] {
			need += len(b)
		}
		if s > 0 {
			need++
		}
		if e < len(blocks) {
			need++
		}
		return need <= avail
	}
	selected = min(max(selected, 0), len(blocks)-1)
	start, end = selected, selected+1
	for start < selected && !fits(start, end) {
		start++
	}
	// grow downward first, then upward, while it still fits
	for end < len(blocks) && fits(start, end+1) {
		end++
	}
	for start > 0 && fits(start-1, end) {
		start--
	}
	return start, end
}

// Appends footer lines so they sit on the panel's last rows.
func padTo(lines []string, footer []string, innerH int) []string {
	if len(footer) == 0 {
		return lines
	}
	room := innerH - len(footer)
	if len(lines) > room {
		lines = lines[:max(room, 0)]
	}
	for len(lines) < room {
		lines = append(lines, "")
	}
	return append(lines, footer...)
}

type commandEntry struct {
	key       string
	desc      string
	needsPort bool
}

var primaryCommands = []commandEntry{
	{"b", "build", false},
	{"f", "flash", true},
	{"a", "build + flash", true},
	{"m", "monitor", true},
	{"e", "erase flash", true},
	{"x", "flash a binary", false},
	{"p", "view partitions", true},
}

var miscCommands = []commandEntry{
	{"r", "rescan devices", false},
	{"l", "clear logs", false},
	{"tab", "switch panel", false},
	{"q", "quit", false},
}

func (model Model) viewCommandPanel(width int, height int) string {
	switch {
	case model.eraseConfirm:
		return model.viewEraseConfirmPanel(width, height)
	case model.browserConfirm != nil:
		return model.viewConfirmPanel(width, height)
	case model.browserMode:
		return model.viewBrowserPanel(width, height)
	case model.partitionMode:
		return model.viewPartitionPanel(width, height)
	}

	innerW, innerH := innerSize(width, height)
	header := panelHeader("commands", "", innerW, false)

	primary := make([]string, 0, len(primaryCommands))
	for _, c := range primaryCommands {
		disabled := model.isBusy() || (c.needsPort && model.idfPort == "") || (c.key == "b" && !model.project.IsValid)
		primary = append(primary, renderKeyCommand(c.key, c.desc, disabled))
	}
	misc := make([]string, 0, len(miscCommands))
	for _, c := range miscCommands {
		disabled := c.key == "r" && model.isBusy()
		misc = append(misc, renderKeyCommand(c.key, c.desc, disabled))
	}
	state := model.viewStateInfo(innerW)

	// Roomy layout spaces commands apart; tighter terminals pack them, then
	// drop the misc group (its keys are in the footer hints too).
	spaced := interleave(primary, "")
	candidates := [][]string{
		concat(header, []string{""}, spaced, []string{"", separatorLine(innerW), ""}, misc, []string{"", separatorLine(innerW)}, state),
		concat(header, []string{""}, primary, []string{"", separatorLine(innerW)}, misc, []string{separatorLine(innerW)}, state),
		concat(header, primary, []string{separatorLine(innerW)}, misc, []string{separatorLine(innerW)}, state),
		concat(header, primary, []string{separatorLine(innerW)}, state),
	}
	for _, lines := range candidates {
		if len(lines) <= innerH {
			return renderPanel(lines, width, height, false)
		}
	}
	return renderPanel(candidates[len(candidates)-1], width, height, false)
}

func interleave(items []string, sep string) []string {
	out := make([]string, 0, len(items)*2)
	for i, item := range items {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, item)
	}
	return out
}

func concat(groups ...[]string) []string {
	var out []string
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

func (model Model) viewEraseConfirmPanel(width int, height int) string {
	innerW, innerH := innerSize(width, height)
	lines := panelHeader("erase flash?", "", innerW, true)
	lines = append(lines, "")
	lines = append(lines, wrapText("this erases all flash on", innerW, "", textStyle)...)
	lines = append(lines, portActiveStyle.Render(ansi.Truncate(model.idfPort, innerW, ellipsis)))
	lines = append(lines, "")
	lines = append(lines, wrapText("app, bootloader and nvs data are all wiped. this cannot be undone.", innerW, "", logWarnStyle)...)

	keys := []string{
		separatorLine(innerW),
		keyHint("enter", "erase flash"),
		keyHint("esc", "cancel"),
	}
	return renderPanel(padTo(lines, keys, innerH), width, height, true)
}

func (model Model) viewBrowserPanel(width int, height int) string {
	innerW, innerH := innerSize(width, height)

	meta := ""
	if n := len(model.browserItems); n > 0 {
		meta = mutedStyle.Render(fmt.Sprintf("%d/%d", model.browserCursor+1, n))
	}
	lines := panelHeader("flash a binary", meta, innerW, true)
	lines = append(lines,
		mutedStyle.Render(truncateBrowserPath(model.browserPath, innerW)),
		separatorLine(innerW),
	)

	portStr := mutedStyle.Render("target ") + portActiveStyle.Render(model.idfPort)
	if model.idfPort == "" {
		portStr = logWarnStyle.Render("no device selected")
	}
	footer := []string{separatorLine(innerW), portStr, model.viewBrowserHint()}

	avail := max(innerH-len(lines)-len(footer), 1)
	if len(model.browserItems) == 0 {
		lines = append(lines, mutedStyle.Render("no folders or .bin files here"))
		return renderPanel(padTo(lines, footer, innerH), width, height, true)
	}

	start, end := listWindow(len(model.browserItems), model.browserCursor, avail)
	if start > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("↑ %d more", start)))
	}
	for i := start; i < end; i++ {
		line := renderBrowserEntry(model.browserItems[i], innerW)
		if i == model.browserCursor {
			plain := "› " + strings.TrimPrefix(ansi.Strip(line), "  ")
			line = selectedRowStyle.Width(innerW).Render(ansi.Truncate(plain, innerW, ellipsis))
		}
		lines = append(lines, line)
	}
	if end < len(model.browserItems) {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("↓ %d more", len(model.browserItems)-end)))
	}
	return renderPanel(padTo(lines, footer, innerH), width, height, true)
}

// Picks a window of avail rows over n single-line items that keeps cursor
// visible, reserving a row for each "more" cue it needs.
func listWindow(n, cursor, avail int) (start, end int) {
	if n <= avail {
		return 0, n
	}
	rows := func(s, e int) int {
		r := e - s
		if s > 0 {
			r++
		}
		if e < n {
			r++
		}
		return r
	}
	cursor = min(max(cursor, 0), n-1)
	start, end = cursor, cursor+1
	for end < n && rows(start, end+1) <= avail {
		end++
	}
	for start > 0 && rows(start-1, end) <= avail {
		start--
	}
	return start, end
}

// Shows a deliberate one-step confirmation before any bytes are written to
// the device, so a stray or buffered keypress can't trigger a flash.
func (model Model) viewConfirmPanel(width int, height int) string {
	innerW, innerH := innerSize(width, height)
	opt := model.browserConfirm

	lines := panelHeader("confirm flash", "", innerW, true)
	lines = append(lines, "")

	if opt.IsFullFlash {
		lines = append(lines, accentStyle.Render("full flash"))
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("%d files from flasher_args.json", opt.FileCount)))
	} else {
		// wrapped, not truncated: the full file name must be readable here
		lines = append(lines, wrapText(opt.Label, innerW, "", accentStyle)...)
		lines = append(lines, mutedStyle.Render("address ")+textStyle.Render(opt.FlashAddr))
	}
	lines = append(lines, "",
		mutedStyle.Render("target device"),
		portActiveStyle.Render(ansi.Truncate(model.idfPort, innerW, ellipsis)),
		"",
	)
	lines = append(lines, wrapText("this overwrites flash on the device above", innerW, "", logWarnStyle)...)

	keys := []string{
		separatorLine(innerW),
		keyHint("enter", "flash it"),
		keyHint("esc", "back to the file list"),
	}
	return renderPanel(padTo(lines, keys, innerH), width, height, true)
}

func (model Model) viewPartitionPanel(width int, height int) string {
	innerW, innerH := innerSize(width, height)
	table := model.partitionTable

	summary := fmt.Sprintf("%d partitions", len(table.Entries))
	if table.FlashSize > 0 {
		summary = humanBytes(table.FlashSize) + " flash · " + summary
	} else {
		summary = "flash size unknown · " + summary
	}
	lines := panelHeader("partitions", mutedStyle.Render(summary), innerW, true)
	lines = append(lines,
		"",
		buildUsageBar(table, innerW),
		mutedStyle.Render("▓ bootloader + table   ░ free"),
		"",
	)

	keys := []string{separatorLine(innerW), keyHint("r", "re-read") + mutedStyle.Render(hintSeparator) + keyHint("esc", "back")}

	totalRef := referenceTotal(table)
	rows := make([]string, 0, len(table.Entries))
	for _, entry := range table.Entries {
		rows = append(rows, partitionRow(entry, totalRef, innerW))
	}
	avail := innerH - len(lines) - len(keys)
	if len(rows) > avail {
		keep := max(avail-1, 0)
		hidden := len(rows) - keep
		rows = append(rows[:keep], mutedStyle.Render(fmt.Sprintf("+ %d more. make the terminal taller to see them", hidden)))
	}
	lines = append(lines, rows...)
	return renderPanel(padTo(lines, keys, innerH), width, height, true)
}

// One legend row: swatch, name, subtype, offset, size, share of flash.
// Columns are dropped from the end when the panel is narrow.
func partitionRow(entry idf.PartitionEntry, totalRef uint64, innerW int) string {
	swatch := partitionStyle(entry).Render("■")
	name := fmt.Sprintf("%-12s", ansi.Truncate(entry.Name, 12, ""))

	pct := ""
	if totalRef > 0 {
		pct = fmt.Sprintf("%5.1f%%", float64(entry.Size)/float64(totalRef)*100)
	}
	full := fmt.Sprintf(" %-8s 0x%06x %8s %6s", entry.SubLabel, entry.Offset, humanBytes(uint64(entry.Size)), pct)
	short := fmt.Sprintf(" %8s %s", humanBytes(uint64(entry.Size)), pct)

	detail := full
	if 2+ansi.StringWidth(name)+ansi.StringWidth(full) > innerW {
		detail = short
	}
	return swatch + " " + textStyle.Render(name) + mutedStyle.Render(detail)
}

// Picks the denominator for percentage and bar-width math:
// real flash size when known, otherwise the table's own extent.
func referenceTotal(table idf.PartitionTable) uint64 {
	var maxEnd uint64
	for _, e := range table.Entries {
		maxEnd = max(maxEnd, uint64(e.Offset)+uint64(e.Size))
	}
	// a flash size smaller than the table itself means detection was wrong
	return max(table.FlashSize, maxEnd)
}

type barSegment struct {
	size      uint64
	char      string
	style     lipgloss.Style
	partition bool
}

// Renders one line of block characters representing the whole flash: a
// muted "▓" for the bootloader and partition table region, a colored "█"
// segment per partition proportional to its size, and a "░" segment for
// unallocated space. Every partition gets at least one cell so small ones
// like nvs or phy_init still appear in the bar.
func buildUsageBar(table idf.PartitionTable, barWidth int) string {
	totalRef := referenceTotal(table)
	if totalRef == 0 || len(table.Entries) == 0 || barWidth <= 0 {
		return mutedStyle.Render(strings.Repeat("░", max(barWidth, 0)))
	}

	freeStyle := lipgloss.NewStyle().Foreground(colorDim)
	var segments []barSegment
	if first := uint64(table.Entries[0].Offset); first > 0 {
		// counted like a partition so it always gets a visible cell
		segments = append(segments, barSegment{size: first, char: "▓", style: mutedStyle, partition: true})
	}
	for i, entry := range table.Entries {
		segments = append(segments, barSegment{size: uint64(entry.Size), char: "█", style: partitionStyle(entry), partition: true})

		end := uint64(entry.Offset) + uint64(entry.Size)
		nextStart := totalRef
		if i+1 < len(table.Entries) {
			nextStart = uint64(table.Entries[i+1].Offset)
		}
		if nextStart > end {
			segments = append(segments, barSegment{size: nextStart - end, char: "░", style: freeStyle})
		}
	}

	widths := allocateWidths(segments, totalRef, barWidth)

	var bar strings.Builder
	for i, seg := range segments {
		if widths[i] > 0 {
			bar.WriteString(seg.style.Render(strings.Repeat(seg.char, widths[i])))
		}
	}
	return bar.String()
}

// Largest-remainder allocation so segment widths sum exactly to barWidth,
// then a minimum of one cell per partition (taken from the widest
// segment) when there is room for it.
func allocateWidths(segments []barSegment, totalRef uint64, barWidth int) []int {
	type weighted struct {
		index     int
		floor     int
		remainder float64
	}
	weights := make([]weighted, len(segments))
	floorSum := 0
	for i, seg := range segments {
		raw := float64(seg.size) / float64(totalRef) * float64(barWidth)
		floor := int(raw)
		weights[i] = weighted{index: i, floor: floor, remainder: raw - float64(floor)}
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

	partitions := 0
	for _, seg := range segments {
		if seg.partition {
			partitions++
		}
	}
	if partitions > barWidth/2 {
		return widths
	}
	for i, seg := range segments {
		if !seg.partition || widths[i] > 0 {
			continue
		}
		widest := 0
		for j := range widths {
			if widths[j] > widths[widest] {
				widest = j
			}
		}
		if widths[widest] <= 1 {
			break
		}
		widths[widest]--
		widths[i] = 1
	}
	return widths
}

// Picks a categorical color per partition kind so the bar and the rows read
// together. Status hues (red, green, yellow) are left for status only; the
// row text carries the type as well, so color is never the only cue.
func partitionStyle(entry idf.PartitionEntry) lipgloss.Style {
	switch {
	case entry.TypeLabel == "app":
		return lipgloss.NewStyle().Foreground(colorAccent)
	case entry.SubLabel == "nvs":
		return lipgloss.NewStyle().Foreground(colorInfo)
	case entry.SubLabel == "ota_data":
		return lipgloss.NewStyle().Foreground(colorPurple)
	case entry.SubLabel == "phy":
		return lipgloss.NewStyle().Foreground(colorPartCyan)
	case entry.SubLabel == "spiffs" || entry.SubLabel == "fat" || entry.SubLabel == "littlefs":
		return lipgloss.NewStyle().Foreground(colorPartPink)
	default:
		return lipgloss.NewStyle().Foreground(colorText)
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

func renderBrowserEntry(item idf.FileBrowserEntry, innerW int) string {
	switch {
	case item.Name == "..":
		return mutedStyle.Render("  ../  (up one folder)")

	case item.IsFullFlash:
		return accentStyle.Render("  [full flash]") + mutedStyle.Render("  everything idf.py built")

	case item.IsDir:
		return mutedStyle.Render(ansi.Truncate("  "+item.Name+"/", innerW, ellipsis))

	default:
		// .bin file, size right-aligned; the name is truncated in the list
		// and shown in full on the confirm screen
		size := item.SizeLabel
		nameW := max(innerW-2-len(size)-1, 1)
		label := ansi.Truncate(item.Name, nameW, ellipsis)
		gap := max(innerW-2-ansi.StringWidth(label)-len(size), 1)
		return textStyle.Render("  "+label) + mutedStyle.Render(strings.Repeat(" ", gap)+size)
	}
}

func (model Model) viewBrowserHint() string {
	if model.browserCursor < 0 || model.browserCursor >= len(model.browserItems) {
		return keyHint("bksp", "up") + mutedStyle.Render(hintSeparator) + keyHint("esc", "close")
	}
	item := model.browserItems[model.browserCursor]
	action := "select"
	if item.Name == ".." || item.IsDir {
		action = "open"
	}
	return keyHint("enter", action) + mutedStyle.Render(hintSeparator) +
		keyHint("bksp", "up") + mutedStyle.Render(hintSeparator) +
		keyHint("esc", "close")
}

// Shortens a long path to fit in maxWidth by keeping the tail components
// and replacing the prefix with "…".
func truncateBrowserPath(path string, maxWidth int) string {
	if ansi.StringWidth(path) <= maxWidth {
		return path
	}
	sep := string(filepath.Separator)
	parts := strings.Split(path, sep)
	for len(parts) > 1 {
		parts = parts[1:]
		result := ellipsis + sep + strings.Join(parts, sep)
		if ansi.StringWidth(result) <= maxWidth {
			return result
		}
	}
	// a single path component wider than the panel: keep its end
	return ansi.TruncateLeft(path, ansi.StringWidth(path)-maxWidth+1, ellipsis)
}

func renderKeyCommand(keyName string, description string, isDisabled bool) string {
	if isDisabled {
		// brackets as well as the dim color, so "unavailable" isn't color-only
		return disabledStyle.Render("[" + keyName + "] " + description)
	}
	return keyBadgeStyle.Render(keyName) + textStyle.Render(" "+description)
}

func (model Model) viewStateInfo(innerW int) []string {
	switch {
	case model.isBusy():
		return []string{activeStatusStyle.Render(model.spinner.View() + " " + model.state.String())}
	case model.lastErr != nil:
		return []string{
			logErrorStyle.Render(ansi.Truncate("failed: "+firstLine(model.lastErr), innerW, ellipsis)),
			mutedStyle.Render("details are in the logs"),
		}
	case model.idfPort == "":
		return []string{logWarnStyle.Render("no device selected")}
	default:
		return []string{logSuccessStyle.Render("ready")}
	}
}

func (model Model) viewLogPanel(width int, height int) string {
	innerW, innerH := innerSize(width, height)
	focused := model.focusedPanel == PanelLogs && !model.wideMode() && !model.eraseConfirm

	var meta string
	vp := model.logViewport
	scrolledUp := vp.TotalLineCount() > vp.Height && !vp.AtBottom()
	switch {
	case scrolledUp:
		below := vp.TotalLineCount() - vp.YOffset - vp.Height
		meta = logWarnStyle.Render(fmt.Sprintf("↓ %d more", below)) + mutedStyle.Render(" · end to follow")
	case len(model.logs) == 1:
		meta = mutedStyle.Render("1 line")
	default:
		meta = mutedStyle.Render(fmt.Sprintf("%d lines", len(model.logs)))
	}

	lines := panelHeader("logs", meta, innerW, focused)
	if len(model.logs) == 0 {
		lines = append(lines, wrapText("output from build, flash and monitor shows up here", innerW, "", mutedStyle)...)
	} else {
		lines = append(lines, strings.Split(vp.View(), "\n")...)
	}
	return renderPanel(lines[:min(len(lines), innerH)], width, height, focused)
}

func (model Model) viewFooter() string {
	width := max(model.width-2, 1)

	if model.notice != nil {
		// notices carry the way out of a dead end, so they wrap rather
		// than lose their second half
		lines := strings.Split(ansi.Wrap(model.notice.Text, width, ""), "\n")
		if len(lines) > maxNoticeLines {
			lines = lines[:maxNoticeLines]
			lines[maxNoticeLines-1] = ansi.Truncate(lines[maxNoticeLines-1]+" "+ellipsis, width, ellipsis)
		}
		style := logStyleFor(model.notice.Level)
		for i, line := range lines {
			lines[i] = footerStyle.Render(style.Render(line))
		}
		return strings.Join(lines, "\n")
	}

	var parts []string
	switch {
	case model.eraseConfirm:
		parts = []string{keyHint("enter", "erase flash"), keyHint("esc", "cancel")}
	case model.browserConfirm != nil:
		parts = []string{keyHint("enter", "flash it"), keyHint("esc", "back")}
	case model.browserMode:
		parts = []string{keyHint("↑↓", "move"), keyHint("enter", "open / select"), keyHint("bksp", "up"), keyHint("esc", "close")}
	case model.partitionMode:
		parts = []string{keyHint("r", "re-read"), keyHint("esc", "back")}
	case model.focusedPanel == PanelLogs:
		parts = []string{keyHint("↑↓", "scroll"), keyHint("pgup/pgdn", "page"), keyHint("end", "follow"), keyHint("tab", "devices"), keyHint("q", "quit")}
	default:
		parts = []string{keyHint("↑↓", "choose"), keyHint("enter", "use device"), keyHint("tab", "logs"), keyHint("r", "rescan"), keyHint("q", "quit")}
	}
	return footerStyle.Render(fitParts(parts, width))
}
