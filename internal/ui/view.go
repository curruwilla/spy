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
	headerLinesCompact = 7  // the same without the margin and the spacers
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

// Column widths. The command column takes all remaining space.
const (
	wPID   = 7
	wUser  = 9
	wState = 1
	wCPU   = 5
	wMem   = 5
	wRSS   = 6
	wTime  = 8

	// Every column is separated by a single space.
	fixedWidth = wPID + wUser + wState + wCPU + wMem + wRSS + wTime + 7
)

func (m Model) View() string {
	lines := make([]string, 0, m.height)
	lines = append(lines, m.viewHeader()...)
	if m.mode == modeInfo {
		lines = append(lines, m.viewInfo()...)
	} else {
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
	memInfo := detail("used", fmt.Sprintf("%s of %s", formatBytes(mem.Used()), formatBytes(mem.Total)))
	swapInfo := detail("used", fmt.Sprintf("%s of %s", formatBytes(mem.SwapUsed()), formatBytes(mem.SwapTotal)))
	if mem.SwapTotal == 0 {
		swapInfo = detail("used", "swap disabled")
	}
	bar := m.gaugeCells(load, memInfo, swapInfo)

	var lines []string
	if !m.compact() {
		lines = append(lines, "") // the margin above the title
	}
	lines = append(lines, m.viewTitle())
	if !m.compact() {
		lines = append(lines, "")
	}
	lines = append(lines,
		// The cores come first: they are what the total below them is made
		// of, and the bar reads as their summary.
		m.sparkRow("core", cpu.Cores),
		m.meter("CPU", cpu.Total, bar, load),
		"",
		m.meter("MEM", mem.UsedPercent(), bar, memInfo),
		m.meter("SWP", mem.SwapPercent(), bar, swapInfo),
	)
	if !m.compact() {
		lines = append(lines, "")
	}
	return lines
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

// sparkRow is the labelled, bracketed line of per-core cells, laid out like
// a meter so the two line up.
func (m Model) sparkRow(label string, values []float64) string {
	width := m.inner() - meterLabelWidth - len(meterGap) - len(gaugeOpen) - len(gaugeClose)
	return styleLabel.Render(pad(label, meterLabelWidth, false)) + meterGap +
		bracket(sparkline(values, width))
}

// Spacing inside the header. The gauges keep a wide, constant gap on each
// side so the label, the bar and the numbers read as separate things
// instead of one block of characters.
const (
	meterLabelWidth = 4
	meterGap        = "  "
	detailGap       = "     "
	titleGap        = "   "
	titleSep        = "   ·   "

	// What a meter line spends on everything but the bar and the detail:
	// the label, the gaps around the bar, the brackets and the percentage.
	meterFixed    = meterLabelWidth + 2*len(meterGap) + len(gaugeOpen) + len(gaugeClose) + len("100%")
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

	var shown []string
	for _, part := range []string{
		"up " + formatUptime(snap.Uptime),
		fmt.Sprintf("%d cores", len(snap.CPU.Cores)),
		fmt.Sprintf("%d procs, %d running", len(snap.Processes), snap.Load.Running),
	} {
		next := len(part)
		if len(shown) > 0 {
			next += len(titleSep)
		}
		if next > budget {
			break
		}
		shown = append(shown, part)
		budget -= next
	}

	title := styleBarStrong.Render("spy")
	if len(shown) > 0 {
		title += styleBar.Render(titleGap + strings.Join(shown, titleSep))
	}
	return m.spread(title, styleBar.Render(clock), styleBar)
}

// meter is one labelled gauge line: label, bar, percentage, then whatever
// detail belongs on the right of it.
func (m Model) meter(label string, pct float64, cells int, right string) string {
	bar := styleLabel.Render(pad(label, meterLabelWidth, false)) + meterGap +
		gauge(pct, cells) + meterGap +
		heat(pct).Render(fmt.Sprintf("%3.0f%%", pct))
	// The gauge itself is what the line is for, so a screen with no room
	// for the detail on its right goes without it instead of wrapping.
	if lipgloss.Width(bar)+len(detailGap)+lipgloss.Width(right) > m.inner() {
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
	// Cell positions of the sortable columns, matching the order in cells.
	sorted := map[sortKey]int{sortPID: 0, sortCPU: 3, sortMem: 4, sortTime: 6, sortName: 7}

	titles := []string{"PID", "USER", "S", "CPU%", "MEM%", "RSS", "TIME", "COMMAND"}
	titles[sorted[m.sort]] += arrow

	cells := m.cells(titles[0], titles[1], titles[2], titles[3], titles[4], titles[5], titles[6], titles[7])
	for i, cell := range cells {
		style := styleColumns
		if i == sorted[m.sort] {
			style = styleColumnsSort
		}
		cells[i] = style.Render(cell)
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
	cells := m.cells(
		fmt.Sprint(p.PID),
		p.User,
		p.State,
		fmt.Sprintf("%.1f", p.CPU),
		fmt.Sprintf("%.1f", p.Mem),
		formatBytes(p.RSS),
		formatCPUTime(p.CPUTime),
		r.indent+p.Command,
	)
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

	cells[1] = m.userStyle(p.User).Render(cells[1])
	cells[2] = stateStyle(p.State).Render(cells[2])
	cells[3] = heat(p.CPU).Render(cells[3])
	cells[5] = styleDim.Render(cells[5])
	cells[6] = styleDim.Render(cells[6])
	if active(p) {
		cells[7] = styleActive.Render(cells[7])
	}
	return strings.Join(cells, " ")
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

// cells lays out one table line, padded but unstyled so callers can color
// individual columns without breaking the alignment.
func (m Model) cells(pid, user, state, cpu, mem, rss, cputime, command string) []string {
	return []string{
		pad(pid, wPID, true),
		pad(user, wUser, false),
		pad(state, wState, false),
		pad(cpu, wCPU, true),
		pad(mem, wMem, true),
		pad(rss, wRSS, true),
		pad(cputime, wTime, true),
		pad(command, max(10, m.inner()-fixedWidth), false),
	}
}

// The detail panel is a label column wide enough for the longest label,
// followed by the value, and two of those pairs to a line. It is kept
// narrow so it reads as a panel over the table rather than another table.
const (
	infoLabelWidth = 9
	infoMinWidth   = 30
	infoMaxWidth   = 64
)

// viewInfo draws the detail panel in place of the table, column titles
// included: the panel is about one process, and a list nobody is reading
// should not compete with it. It returns exactly as many lines as the two
// of them together would have.
func (m Model) viewInfo() []string {
	height := m.tableHeight() + 1
	panel := strings.Split(m.infoPanel(m.info, height), "\n")
	if len(panel) > height {
		panel = panel[:height] // a terminal too short even for the trimmed box
	}
	lines := blankLines((height - len(panel)) / 2)
	lines = append(lines, panel...)
	return append(lines, blankLines(height-len(lines))...)
}

// infoPanel renders the bordered box for one process, centred across the
// width of the screen and no taller than height.
func (m Model) infoPanel(p proc.Process, height int) string {
	// The border and its padding cost four cells, which a narrow terminal
	// has to come out of the panel rather than off the side of the screen.
	width := min(clamp(m.inner()-8, infoMinWidth, infoMaxWidth), m.inner()-4)
	available := height - 2 // the lines inside the border
	fields := m.infoFields(p, width)
	// The command takes what the field rows and its own label leave behind,
	// counted as if the spacers were already gone, because they are the
	// first thing given up. It never gets less than one line: showing the
	// command is most of what the panel is for.
	rows := len(slices.DeleteFunc(slices.Clone(fields), isSpacer))
	budget := max(1, available-rows-2)

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

	box := styleInfoBox.Render(strings.Join(body, "\n"))
	indent := strings.Repeat(" ", max(0, (m.inner()-lipgloss.Width(box))/2))
	lines := strings.Split(box, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
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
	return []string{
		styleTitle.Render(pad(fmt.Sprintf("process %d", p.PID), width, false)),
		"",
		pair("user", p.User, "state", formatState(p.State)),
		pair("parent", strconv.Itoa(p.PPID), "threads", strconv.Itoa(p.Threads)),
		pair("nice", strconv.Itoa(p.Nice), "running", formatUptime(p.Uptime)),
		"",
		pair("cpu", fmt.Sprintf("%.1f%%", p.CPU), "cpu time", formatCPUTime(p.CPUTime)),
		pair("memory", fmt.Sprintf("%.1f%%", p.Mem), "rss", formatBytes(p.RSS)),
		pair("virtual", formatBytes(p.VSize), "started", m.snap.At.Add(-p.Uptime).Format("02 Jan 15:04")),
	}
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
	case m.mode == modeConfirm:
		return styleBarAlert.Render(pad(fmt.Sprintf("send SIGTERM to %d %s ? [y/N]",
			m.confirm.PID, firstWord(m.confirm.Command)), m.inner(), false))
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
	return m.spread(m.viewHelp(), styleBar.Render(state), styleBar)
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
// sort columns, l only opens a prompt the footer already echoes — are last.
var helpHints = []struct{ key, label string }{
	{"↑↓", "move"},
	{"c/m/p/n", "sort"},
	{"/", "filter"},
	{"t", "tree"},
	{"i", "info"},
	{"x", "kill"},
	{"q", "quit"},
	{"tab", "column"},
	{"l", "min"},
}

// viewHelp joins as many hints as the width allows, dropping the rest from
// the right: a footer that wraps costs the table a line and pushes the
// bottom of the screen out of view.
func (m Model) viewHelp() string {
	const sep = " · "

	var shown []string
	used := 0
	for _, h := range helpHints {
		next := lipgloss.Width(h.key) + 1 + len(h.label)
		if len(shown) > 0 {
			next += len(sep)
		}
		if used+next > m.inner() {
			break
		}
		shown = append(shown, hint(h.key, h.label))
		used += next
	}
	return strings.Join(shown, styleBar.Render(sep))
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
