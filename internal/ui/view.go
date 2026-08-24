package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/curruwilla/spy/internal/proc"
)

// The header and the footer have a fixed height, so the table gets whatever
// is left of the terminal. The header separates its three groups — the
// title, the CPU block and the memory block — with blank lines, which a
// terminal too short to spare them gives back to the table.
const (
	headerLines        = 10 // top margin, cpu and memory blocks, spacers, column titles
	headerLinesCompact = 6  // the same without the margin, the spacers and the trend line
	compactHeight      = 22 // terminal height below which the header sheds them
	footerHeight       = 1
)

// The screen keeps a margin so it does not sit against the edges of the
// terminal: a blank line above the title, and a few columns on each side
// that every line is indented into.
const gutter = 2

// inner is the width the content is laid out in, the terminal less both
// gutters.
func (m Model) inner() int { return max(1, m.width-2*gutter) }

// compact reports whether the screen is too short to spend lines on
// spacing.
func (m Model) compact() bool { return m.height < compactHeight }

func (m Model) headerHeight() int {
	if m.compact() {
		return headerLinesCompact
	}
	return headerLines
}

// columnID names a table column, so a row can be styled and measured by
// what a cell holds rather than by where it happens to sit.
type columnID int

const (
	colPID columnID = iota
	colUser
	colState
	colCPU
	colMem
	colRSS
	colTime
	colRead
	colWrite
	colCommand
)

// column is one column of the table: how wide it is drawn, which way its
// values are aligned, and the sort it puts its arrow on.
type column struct {
	id       columnID
	title    string
	width    int // 0 for the command, which takes whatever the others leave
	right    bool
	key      sortKey
	sortable bool
}

// columns is the table, in the order it is drawn. Memory sorts by the
// bytes rather than by the percentage, so its arrow sits on MEM% while
// RSS carries none; the two disk columns are one measurement split in
// two, so the arrow goes on the first of them.
var columns = []column{
	{id: colPID, title: "PID", width: 7, right: true, key: sortPID, sortable: true},
	{id: colUser, title: "USER", width: 9},
	{id: colState, title: "S", width: 1},
	{id: colCPU, title: "CPU%", width: 5, right: true, key: sortCPU, sortable: true},
	{id: colMem, title: "MEM%", width: 5, right: true, key: sortMem, sortable: true},
	{id: colRSS, title: "RSS", width: 6, right: true},
	{id: colTime, title: "TIME", width: 8, right: true, key: sortTime, sortable: true},
	{id: colRead, title: "RD/s", width: 6, right: true, key: sortIO, sortable: true},
	{id: colWrite, title: "WR/s", width: 6, right: true},
	{id: colCommand, title: "COMMAND", key: sortName, sortable: true},
}

// minCommandWidth is the least the command column is worth drawing in.
const minCommandWidth = 20

// visibleColumns is the table the width can afford. The disk pair is the
// first thing a narrow terminal gives up: it is the least of what the
// table says and the command is the most, and neither of them is worth
// having if the command is down to a handful of characters.
func (m Model) visibleColumns() []column {
	if m.inner()-fixedWidth(columns) >= minCommandWidth {
		return columns
	}
	narrow := make([]column, 0, len(columns))
	for _, c := range columns {
		if c.id != colRead && c.id != colWrite {
			narrow = append(narrow, c)
		}
	}
	return narrow
}

// fixedWidth is what a set of columns costs before the command gets its
// share, the single space between each pair included.
func fixedWidth(cols []column) int {
	total := len(cols) - 1
	for _, c := range cols {
		total += c.width
	}
	return total
}

// width is how wide one column is drawn: what it asked for, or, for the
// command, everything the others left.
func (m Model) columnWidth(c column, cols []column) int {
	if c.width > 0 {
		return c.width
	}
	return max(10, m.inner()-fixedWidth(cols))
}

func (m Model) View() string {
	lines := make([]string, 0, m.height)
	lines = append(lines, m.viewHeader()...)
	switch {
	case m.mode == modeInfo:
		lines = append(lines, m.viewInfo()...)
	case m.mode == modeSignal:
		lines = append(lines, m.viewSignal()...)
	default:
		lines = append(lines, m.viewColumns())
		lines = append(lines, m.viewRows()...)
	}
	lines = append(lines, m.viewFooter())

	// Every line is laid out in the inner width, so the gutter is the same
	// on both sides once they are indented. Empty lines stay empty rather
	// than carrying trailing spaces.
	indent := strings.Repeat(" ", gutter)
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewHeader() []string {
	snap := m.snap
	mem, cpu := snap.Memory, snap.CPU

	load := detail("load", fmt.Sprintf("%.2f  %.2f  %.2f", snap.Load.One, snap.Load.Five, snap.Load.Fifteen))
	if cpu.Temp > 0 {
		load += detailSep + detail("temp", fmt.Sprintf("%.0f°C", cpu.Temp))
	}
	// What each bar is a picture of, written inside it: the percentage on
	// its right says how full it is, and these say how full of what.
	memInfo := used(mem.Used(), mem.Total)
	swapInfo := used(mem.SwapUsed(), mem.SwapTotal)
	if mem.SwapTotal == 0 {
		swapInfo = "swap disabled"
	}
	// The processor has no total to count against but the cores themselves,
	// so its bar carries how many of them the load adds up to.
	cpuInfo := ""
	if n := len(cpu.Cores); n > 0 {
		cpuInfo = fmt.Sprintf("%.1f/%d cores", cpu.Total/100*float64(n), n)
	}
	disk := traffic("disk", "read", "write", snap.Disk)
	net := traffic("net", "rx", "tx", snap.Net)

	// Every detail on the right of the header starts at the same column,
	// so the bars are all one length and the spark lines fill what a bar
	// and its percentage take up together. The memory lines have no detail
	// out there any more — it moved inside the bar — so they have no say in
	// how long the bars are.
	details := []string{load, disk}
	if !m.compact() {
		details = append(details, net)
	}
	bar := m.gaugeCells(details...)
	cells := sparkCells(bar)

	var lines []string
	if !m.compact() {
		lines = append(lines, "") // the margin above the title
	}
	lines = append(lines, m.viewTitle())
	if !m.compact() {
		lines = append(lines, "")
	}
	// The trend comes before the bar it is a history of, and it is the
	// first header line a short terminal gives up, along with the network
	// figures that ride on its right, because it is the only line that
	// says something the one below it already says, only earlier.
	if !m.compact() {
		lines = append(lines, m.headerRow("hist", sparkBody(tailValues(m.history, cells), cells), cells, net))
	}
	lines = append(lines,
		m.meter("cpu", cpu.Total, bar, cpuInfo, load),
		"",
		m.meter("mem", mem.UsedPercent(), bar, memInfo, disk),
		m.meter("swp", mem.SwapPercent(), bar, swapInfo, ""),
	)
	if !m.compact() {
		lines = append(lines, "")
	}
	return lines
}

// used is a reading and the total it counts against, the shape the bars
// carry it in: short enough to sit inside one and still leave a scale.
func used(part, total uint64) string {
	return formatBytes(part) + "/" + formatBytes(total)
}

// traffic is one throughput as the header shows it: both directions, named
// so that neither is mistaken for the other.
func traffic(label, in, out string, t proc.Throughput) string {
	return detail(label, fmt.Sprintf("%s %s  %s %s",
		in, formatPerSecond(t.In), out, formatPerSecond(t.Out)))
}

// gaugeCells is how many cells the bars get: as many as the widest of the
// details on their right leaves them, so all three bars are the same length
// and none of them pushes its detail off the screen. A screen too narrow
// for both keeps a short bar and lets meter drop the detail.
func (m Model) gaugeCells(details ...string) int {
	widest := 0
	for _, d := range details {
		widest = max(widest, lipgloss.Width(d))
	}
	room := m.inner() - meterFixed - len(detailGap) - widest
	cells := clamp(room/2, minGaugeCells, maxGaugeCells)
	// Whatever the detail leaves, the bar still has to fit on the line.
	return max(1, min(cells, (m.inner()-meterFixed-1)/2))
}

// sparkBody is a spark line bracketed and padded out to the whole field,
// so that a trend with less history than room does not drag the detail on
// its right along with it.
func sparkBody(values []float64, cells int) string {
	return bracket(padStyled(sparkline(values, cells), cells))
}

// sparkField is how many columns a spark line takes up in full, its
// brackets counted in. It is what every spark row is padded out to, and so
// what the numbered bars have to fit inside.
func sparkField(cells int) int {
	return cells + len(gaugeOpen) + len(gaugeClose)
}

// headerRow is a labelled header line laid out like a meter so that the
// two line up, with whatever detail belongs on its right. The body is
// padded out to the whole field even when it is shorter, so the detail
// does not slide along the line with it.
func (m Model) headerRow(label, body string, cells int, right string) string {
	line := styleLabel.Render(pad(label, meterLabelWidth, false)) + meterGap +
		padStyled(body, sparkField(cells))
	if right == "" || lipgloss.Width(line)+len(detailGap)+lipgloss.Width(right) > m.inner() {
		return line
	}
	return line + detailGap + right
}

// sparkCells is how many cells a spark line gets for a given bar: exactly
// the room the bar takes up, less its own brackets, so that the two kinds
// of line are the same length and the details on their right start at the
// same column.
func sparkCells(bar int) int {
	return gaugeWidth(bar) - len(gaugeOpen) - len(gaugeClose)
}

// padStyled pads content that has already been coloured out to w columns.
// pad counts runes, which stops being the same thing once the escape codes
// are in the string.
func padStyled(s string, w int) string {
	if gap := w - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// tailValues is the newest end of a history, as much of it as fits in
// width columns. A trend line is read from the right, so what is dropped
// when it does not fit is the oldest of it, not the newest.
func tailValues(values []float64, width int) []float64 {
	n := len(values)
	for n > 0 && sparkWidth(n) > width {
		n--
	}
	return values[len(values)-n:]
}

// Spacing inside the header. The gauges keep a wide, constant gap on each
// side so the label, the bar and the detail read as separate things
// instead of one block of characters.
const (
	meterLabelWidth = 4
	meterGap        = "  "
	detailGap       = "     "
	detailSep       = "   "
	titleGap        = "   "
	titleSep        = "   ·   "

	// What a meter line spends on everything but the bar and the detail:
	// the label, the gap before the bar and the brackets around it.
	meterFixed    = meterLabelWidth + len(meterGap) + len(gaugeOpen) + len(gaugeClose)
	minGaugeCells = 4
	maxGaugeCells = 32
)

// detail is one label and value on the right of a gauge, the label colored
// like the ones on the left so the eye can find it.
func detail(label, value string) string {
	return styleLabel.Render(label) + styleDim.Render(" "+value)
}

// viewTitle is the name of the program, what the machine is doing and the
// clock. Like the footer hints, it drops its details from the right when
// the screen is too narrow for them rather than wrapping onto a second
// line, which the fixed header has no room for.
func (m Model) viewTitle() string {
	snap := m.snap
	clock := snap.At.Format("15:04:05")
	// The gap before the clock is what the details have to fit inside.
	budget := m.inner() - len("spy") - len(titleGap) - lipgloss.Width(clock) - 1

	// What the processor is has a short form and a long one; the rest of
	// the details have only themselves. Each is written in the first form
	// the line still has room for, and the ones after a detail that does
	// not fit at all are dropped with it.
	cores := []string{fmt.Sprintf("%d cores", len(snap.CPU.Cores))}
	if snap.CPU.Model != "" {
		cores = append([]string{fmt.Sprintf("%d × %s", len(snap.CPU.Cores), snap.CPU.Model)}, cores...)
	}

	var shown []string
	for _, forms := range [][]string{
		{"up " + formatUptime(snap.Uptime)},
		cores,
		{fmt.Sprintf("%d procs, %d running", len(snap.Processes), snap.Load.Running)},
	} {
		sep := 0
		if len(shown) > 0 {
			sep = lipgloss.Width(titleSep)
		}
		part := ""
		for _, form := range forms {
			if sep+lipgloss.Width(form) <= budget {
				part = form
				break
			}
		}
		if part == "" {
			break
		}
		shown = append(shown, part)
		budget -= sep + lipgloss.Width(part)
	}

	title := styleBarStrong.Render("spy")
	if len(shown) > 0 {
		title += styleBar.Render(titleGap + strings.Join(shown, titleSep))
	}
	return m.spread(title, styleBar.Render(clock), styleBar)
}

// meter is one labelled gauge line: label, bar with the reading written
// inside it, then whatever detail belongs on the right of it. How full the
// bar is says the percentage, and the reading inside it says what of, so
// there is no number after it to line the spark rows up against.
func (m Model) meter(label string, pct float64, cells int, inside, right string) string {
	bar := styleLabel.Render(pad(label, meterLabelWidth, false)) + meterGap +
		gauge(pct, cells, inside)
	// The gauge itself is what the line is for, so a screen with no room
	// for the detail on its right goes without it instead of wrapping —
	// and so does a bar that carries its whole reading inside it.
	if right == "" || lipgloss.Width(bar)+len(detailGap)+lipgloss.Width(right) > m.inner() {
		return bar
	}
	return bar + detailGap + right
}

// viewColumns draws the table titles, highlighting the sorted one with the
// direction it is sorted in.
func (m Model) viewColumns() string {
	arrow := "▲"
	if m.sort.descendingFirst() != m.reverse {
		arrow = "▼"
	}

	cols := m.visibleColumns()
	cells := make([]string, len(cols))
	for i, c := range cols {
		title, style := c.title, styleColumns
		if c.sortable && c.key == m.sort {
			title, style = title+arrow, styleColumnsSort
		}
		cells[i] = style.Render(pad(title, m.columnWidth(c, cols), c.right))
	}
	// The gaps between the cells are part of the bar and are colored with
	// it, and so is whatever is left of a line the columns do not fill.
	return m.fill(styleColumns, strings.Join(cells, styleColumns.Render(" ")))
}

// fill pads a bar to the full width in its own background, so the color
// runs to the edge of the screen instead of stopping at the text.
func (m Model) fill(style lipgloss.Style, content string) string {
	if gap := m.inner() - lipgloss.Width(content); gap > 0 {
		return content + style.Render(strings.Repeat(" ", gap))
	}
	return content
}

func (m Model) viewRows() []string {
	height := m.tableHeight()
	lines := make([]string, 0, height)

	if len(m.rows) == 0 {
		lines = append(lines, styleDim.Render("  no process matches the filter"))
	}
	for i := m.offset; i < len(m.rows) && len(lines) < height; i++ {
		lines = append(lines, m.viewRow(m.rows[i], i == m.cursor))
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	return lines
}

func (m Model) viewRow(r row, selected bool) string {
	p := r.proc
	cols := m.visibleColumns()
	cells := make([]string, len(cols))
	for i, c := range cols {
		cells[i] = pad(m.cellValue(c, r), m.columnWidth(c, cols), c.right)
	}
	if selected {
		// The highlight has to span the whole line, so it is styled as one
		// plain string instead of per cell.
		return styleSelected.Render(pad(strings.Join(cells, " "), m.inner(), false))
	}
	// A kernel thread is the machine talking to itself. There are dozens of
	// them and none is ever what the reader came looking for, so the whole
	// row is dimmed and the eye skips it.
	if p.Kernel {
		return styleDim.Render(strings.Join(cells, " "))
	}

	for i, c := range cols {
		cells[i] = m.cellStyle(c, p).Render(cells[i])
	}
	return strings.Join(cells, " ")
}

// cellValue is what one column holds for one row.
func (m Model) cellValue(c column, r row) string {
	p := r.proc
	switch c.id {
	case colPID:
		return strconv.Itoa(p.PID)
	case colUser:
		return p.User
	case colState:
		return p.State
	case colCPU:
		return fmt.Sprintf("%.1f", p.CPU)
	case colMem:
		return fmt.Sprintf("%.1f", p.Mem)
	case colRSS:
		return formatBytes(p.RSS)
	case colTime:
		return formatCPUTime(p.CPUTime)
	case colRead:
		return formatRate(p.Disk.In, p.DiskKnown)
	case colWrite:
		return formatRate(p.Disk.Out, p.DiskKnown)
	default:
		return r.indent + p.Command
	}
}

// cellStyle is how much of the reader's attention one cell is worth: the
// figures that carry the anomalies are colored, the ones that are only
// context recede, and the plain ones are left alone.
func (m Model) cellStyle(c column, p proc.Process) lipgloss.Style {
	switch c.id {
	case colUser:
		return m.userStyle(p.User)
	case colState:
		return stateStyle(p.State)
	case colCPU:
		return heat(p.CPU)
	case colRSS, colTime, colRead, colWrite:
		return styleDim
	case colCommand:
		if active(p) {
			return styleActive
		}
	}
	return lipgloss.NewStyle()
}

// A process is worth the reader's attention when it is doing something
// now, or holding enough memory that it is why the machine feels full.
const (
	activeCPU = 1.0
	activeMem = 5.0
)

func active(p proc.Process) bool {
	return p.State == "R" || p.CPU >= activeCPU || p.Mem >= activeMem
}

// userStyle marks whose process a row is: the reader's own read as theirs,
// everything else as background.
func (m Model) userStyle(name string) lipgloss.Style {
	if name == m.me {
		return styleMine
	}
	return styleOther
}

// The detail panel is a label column wide enough for the longest label,
// followed by the value, and two of those pairs to a line. It is kept
// narrow so it reads as a panel over the table rather than another table.
const (
	infoLabelWidth = 9
	infoMinWidth   = 30
	infoMaxWidth   = 64

	// How much of the box the command is worth before the field rows are.
	minCommandLines = 4
)

// viewInfo draws the detail panel in place of the table, column titles
// included: the panel is about one process, and a list nobody is reading
// should not compete with it. It returns exactly as many lines as the two
// of them together would have.
func (m Model) viewInfo() []string {
	height := m.tableHeight() + 1
	return m.panelLines(m.infoPanel(m.info, height), height)
}

// viewSignal draws the signal list in the same place, for the same reason:
// picking what to send to one process is not something to do while reading
// a table of all the others.
func (m Model) viewSignal() []string {
	height := m.tableHeight() + 1
	return m.panelLines(m.signalPanel(height), height)
}

// panelLines centres a rendered box over the lines the table would have
// taken, so opening one moves nothing else on the screen.
func (m Model) panelLines(box string, height int) []string {
	panel := strings.Split(box, "\n")
	if len(panel) > height {
		panel = panel[:height] // a terminal too short even for the trimmed box
	}
	lines := blankLines((height - len(panel)) / 2)
	lines = append(lines, panel...)
	return append(lines, blankLines(height-len(lines))...)
}

// signalPanel is the choice of what to send, the way htop asks it: the
// process at the top, the signals under it, and the cursor on the one that
// will be sent. A terminal too short for the whole list scrolls it, so
// whatever is picked is on screen.
func (m Model) signalPanel(height int) string {
	width := m.panelWidth()
	body := []string{
		styleTitle.Render(pad(fmt.Sprintf("signal to %d %s",
			m.confirm.PID, firstWord(m.confirm.Command)), width, false)),
		"",
	}

	room := max(1, height-2-len(body))
	first := clamp(m.signal-room/2, 0, max(0, len(signals)-room))
	for i := first; i < len(signals) && i < first+room; i++ {
		line := pad(fmt.Sprintf("%2d  %s", int(signals[i].number), signals[i].name), width, false)
		if i == m.signal {
			line = styleSelected.Render(line)
		}
		body = append(body, line)
	}
	return m.centre(styleInfoBox.Render(strings.Join(body, "\n")))
}

// panelWidth is how wide a panel is drawn: narrow enough to read as a box
// over the table rather than as another table, and never wider than the
// screen it has to leave a border on.
func (m Model) panelWidth() int {
	// The border and its padding cost four cells, which a narrow terminal
	// has to come out of the panel rather than off the side of the screen.
	return min(clamp(m.inner()-8, infoMinWidth, infoMaxWidth), m.inner()-4)
}

// centre indents a rendered box into the middle of the screen.
func (m Model) centre(box string) string {
	indent := strings.Repeat(" ", max(0, (m.inner()-lipgloss.Width(box))/2))
	lines := strings.Split(box, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// infoPanel renders the bordered box for one process, centred across the
// width of the screen and no taller than height.
func (m Model) infoPanel(p proc.Process, height int) string {
	width := m.panelWidth()
	available := height - 2 // the lines inside the border
	fields := m.infoFields(p, width)
	// The command takes what the field rows and its own label leave behind,
	// counted as if the spacers were already gone, because they are the
	// first thing given up. It never gets less than one line: showing the
	// command is most of what the panel is for.
	rows := len(slices.DeleteFunc(slices.Clone(fields), isSpacer))
	// The command keeps a few lines of its own even when the fields would
	// have taken them: the trimming below gives the fields at the bottom of
	// the box away first, and what those hold is the least of what the
	// panel came to say.
	budget := max(1, min(available-2, minCommandLines))
	if room := available - rows - 2; room > budget {
		budget = room
	}

	// The command is the one field with no length worth speaking of. When
	// it does not fit, the panel gives up being narrow before it gives up
	// showing the command whole.
	if len(wrapText(p.Command, width)) > budget && width < m.inner()-4 {
		width = m.inner() - 4
		fields = m.infoFields(p, width)
	}
	command := infoCommand(p.Command, width, budget)

	body := append(append(fields[:len(fields):len(fields)], ""), command...)
	// A terminal too short for all of that gives up the spacers first,
	// because they carry nothing, and then the field rows from the bottom.
	// The title and the command stay.
	if len(body) > available {
		fields = slices.DeleteFunc(fields, isSpacer)
		for len(fields) > 1 && len(fields)+len(command) > available {
			fields = fields[:len(fields)-1]
		}
		body = append(fields, command...)
	}

	return m.centre(styleInfoBox.Render(strings.Join(body, "\n")))
}

// infoFields is everything about a process that fits on one line, two
// label/value pairs at a time. It is laid out no wider than a comfortable
// panel even when the box itself is stretched for a long command.
func (m Model) infoFields(p proc.Process, width int) []string {
	// The left column takes half, the right one whatever is left, so an odd
	// width still adds up.
	width = min(width, infoMaxWidth)
	left := width / 2
	pair := func(l1, v1, l2, v2 string) string {
		return infoCell(l1, v1, left) + infoCell(l2, v2, width-left)
	}
	full := func(label, value string) string { return infoCell(label, value, width) }

	d := m.details
	fields := []string{
		styleTitle.Render(pad(fmt.Sprintf("process %d", p.PID), width, false)),
		"",
		pair("user", p.User, "state", formatState(p.State)),
		pair("parent", strconv.Itoa(p.PPID), "threads", strconv.Itoa(p.Threads)),
		pair("nice", strconv.Itoa(p.Nice), "running", formatUptime(p.Uptime)),
		"",
		pair("cpu", fmt.Sprintf("%.1f%%", p.CPU), "cpu time", formatCPUTime(p.CPUTime)),
		pair("memory", fmt.Sprintf("%.1f%%", p.Mem), "rss", formatBytes(p.RSS)),
		pair("virtual", formatBytes(p.VSize), "swap", formatBytes(d.Swap)),
		pair("disk r", formatRate(p.Disk.In, p.DiskKnown), "disk w", formatRate(p.Disk.Out, p.DiskKnown)),
		pair("files", formatCount(d.Files), "ctx sw", strconv.FormatUint(d.Switches, 10)),
		pair("started", m.snap.At.Add(-p.Uptime).Format("02 Jan 15:04"), "oom", strconv.Itoa(d.OOMScore)),
	}
	// The paths are the fields with nothing to pair them with: a cgroup or
	// a working directory is worth a whole line or nothing at all, and a
	// process that has none of them says so once, at the end.
	for _, f := range []struct{ label, value string }{
		{"cgroup", d.Cgroup},
		{"exe", d.Exe},
		{"cwd", d.CWD},
	} {
		if f.value != "" {
			fields = append(fields, full(f.label, f.value))
		}
	}
	if d.Restricted {
		fields = append(fields, full("", styleDim.Render("some of it belongs to another account")))
	}
	return fields
}

// formatCount renders a count the reader may not have been allowed to
// take, which reads as unknown rather than as none.
func formatCount(n int) string {
	if n < 0 {
		return "-"
	}
	return strconv.Itoa(n)
}

// infoCommand is the command line wrapped over the lines the panel has
// left. It gets a block of its own across the full width, because that is
// the only way a command long enough to be interesting is readable at all.
// A command with more of it than the screen can hold says so.
func infoCommand(command string, width, budget int) []string {
	lines := wrapText(command, width)
	if len(lines) > budget {
		lines = lines[:budget]
		lines[budget-1] += " …"
	}

	block := []string{styleLabel.Render(pad("command", width, false))}
	for _, line := range lines {
		block = append(block, pad(line, width, false))
	}
	return block
}

// infoCell is one label and its value, padded to exactly w cells. Both
// halves are padded before they are styled, so the escape codes never enter
// the width arithmetic.
func infoCell(label, value string, w int) string {
	labelW := min(infoLabelWidth, w) // a panel too narrow even for the labels
	return styleLabel.Render(pad(label, labelW, false)) + pad(value, w-labelW, false)
}

func blankLines(n int) []string {
	return make([]string, max(0, n))
}

func isSpacer(line string) bool { return line == "" }

func (m Model) viewFooter() string {
	// A prompt owns the footer while it is open: the refresh error is
	// still there when it closes, but a prompt with no echo looks like a
	// key that did nothing.
	switch {
	case m.mode == modeInfo:
		return m.fill(styleBar, styleBar.Render("i or esc to close"))
	case m.mode == modeSignal:
		return m.fill(styleBar, styleBar.Render("↑↓ to pick · enter to send · esc to cancel"))
	case m.mode == modeConfirm:
		return styleBarAlert.Render(pad(fmt.Sprintf("send %s to %d %s ? [y/N]",
			signals[m.signal].name, m.confirm.PID, firstWord(m.confirm.Command)), m.inner(), false))
	case m.mode == modeFilter:
		return m.prompt("filter: ", m.filter.text, "   enter to keep it · esc to clear")
	case m.mode == modeThreshold:
		return m.prompt("min: ", m.input, "   cpu>5 mem>500M time>1m · enter to apply · esc to clear")
	case m.err != nil:
		return styleBarAlert.Render(pad("error: "+m.err.Error(), m.inner(), false))
	case m.status != "":
		return styleBarWarn.Render(pad(m.status, m.inner(), false))
	}

	state := fmt.Sprintf("sort %s", m.sort)
	if m.filter.text != "" {
		state += fmt.Sprintf(" · filter %q", m.filter.text)
	}
	if min := m.filter.min.String(); min != "" {
		state += " · " + min
	}
	if m.tree {
		state += " · tree"
	}
	if m.follow != 0 {
		state += fmt.Sprintf(" · following %d", m.follow)
	}

	right := styleBar.Render(state)
	// A paused screen is no longer showing what the machine is doing,
	// which is the one piece of state that has to be said out loud.
	if m.paused {
		right = styleBarWarn.Render("paused") + styleBar.Render(" · "+state)
	}
	// What the monitor is doing comes before the reminder of how to tell it
	// to do something else, so the hints are what gives up the room.
	return m.spread(m.viewHelp(m.inner()-lipgloss.Width(right)-lipgloss.Width(hintSep)), right, styleBar)
}

// prompt is the footer while something is being typed into it: the label,
// what has been typed with a cursor after it, and either the reminder of
// what the prompt takes or why what is there was refused.
func (m Model) prompt(label, text, hint string) string {
	line := styleBarKey.Render(label) + styleBarStrong.Render(text) + styleBarCursor.Render(" ")
	if m.status != "" {
		return m.fill(styleBar, line+styleBar.Render("  ")+styleBarAlert.Render(" "+m.status+" "))
	}
	return m.fill(styleBar, line+styleBar.Render(hint))
}

// helpHints are the footer key reminders. A narrow terminal gives them up
// from the right, so the two that have another way in — tab only cycles the
// sort columns, l only opens a prompt the footer already echoes — are last,
// behind the keys that are the only way to what they do.
var helpHints = []struct{ key, label string }{
	{"↑↓", "move"},
	{"c/m/p/n", "sort"},
	{"/", "filter"},
	{"t", "tree"},
	{"i", "info"},
	{"x", "kill"},
	{"q", "quit"},
	{"space", "pause"},
	{"f", "follow"},
	{"[ ]", "nice"},
	{"tab", "column"},
	{"l", "min"},
}

// hintSep separates two key reminders in the footer.
const hintSep = " · "

// viewHelp joins as many hints as the budget allows, dropping the rest
// from the right: a footer that wraps costs the table a line and pushes
// the bottom of the screen out of view.
func (m Model) viewHelp(budget int) string {
	var shown []string
	used := 0
	for _, h := range helpHints {
		next := lipgloss.Width(h.key) + 1 + len(h.label)
		if len(shown) > 0 {
			next += lipgloss.Width(hintSep)
		}
		if used+next > budget {
			break
		}
		shown = append(shown, hint(h.key, h.label))
		used += next
	}
	return strings.Join(shown, styleBar.Render(hintSep))
}

func hint(key, label string) string {
	return styleBarKey.Render(key) + styleBar.Render(" "+label)
}

// spread puts left and right on the same line, pushed to opposite edges,
// with the space between them drawn in the bar's own background.
func (m Model) spread(left, right string, fill lipgloss.Style) string {
	gap := m.inner() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return m.fill(fill, left)
	}
	return left + fill.Render(strings.Repeat(" ", gap)) + right
}

func firstWord(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return s[:i]
	}
	return s
}
