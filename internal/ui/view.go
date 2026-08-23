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
// is left of the terminal.
const (
	headerHeight = 6 // five summary lines plus the column titles
	footerHeight = 1
)

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
	return strings.Join(lines, "\n")
}

func (m Model) viewHeader() []string {
	bar := clamp(m.width/3, 10, 30)
	snap := m.snap
	mem, cpu := snap.Memory, snap.CPU

	title := styleTitle.Render("spy") + styleDim.Render(fmt.Sprintf("  up %s  ·  %d cores  ·  %d procs, %d running",
		formatUptime(snap.Uptime), len(cpu.Cores), len(snap.Processes), snap.Load.Running))
	clock := styleDim.Render(snap.At.Format("15:04:05"))

	load := styleDim.Render(fmt.Sprintf("load %.2f %.2f %.2f", snap.Load.One, snap.Load.Five, snap.Load.Fifteen))
	memInfo := styleDim.Render(fmt.Sprintf("%s / %s", formatBytes(mem.Used()), formatBytes(mem.Total)))
	swapInfo := styleDim.Render(fmt.Sprintf("%s / %s", formatBytes(mem.SwapUsed()), formatBytes(mem.SwapTotal)))
	if mem.SwapTotal == 0 {
		swapInfo = styleDim.Render("disabled")
	}

	return []string{
		m.spread(title, clock),
		m.meter("CPU", cpu.Total, bar, load),
		styleLabel.Render("core") + " " + sparkline(cpu.Cores),
		m.meter("MEM", mem.UsedPercent(), bar, memInfo),
		m.meter("SWP", mem.SwapPercent(), bar, swapInfo),
	}
}

// meter is one labelled gauge line: label, bar, percentage, then whatever
// detail belongs on the right of it.
func (m Model) meter(label string, pct float64, width int, detail string) string {
	return fmt.Sprintf("%s %s %s  %s",
		styleLabel.Render(pad(label, 4, false)),
		gauge(pct, width),
		heat(pct).Render(fmt.Sprintf("%3.0f%%", pct)),
		detail)
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
		style := styleHeader
		if i == sorted[m.sort] {
			style = styleSortCol
		}
		cells[i] = style.Render(cell)
	}
	return strings.Join(cells, " ")
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
		return styleSelected.Render(pad(strings.Join(cells, " "), m.width, false))
	}
	cells[3] = heat(p.CPU).Render(cells[3])
	cells[5] = styleDim.Render(cells[5])
	cells[6] = styleDim.Render(cells[6])
	return strings.Join(cells, " ")
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
		pad(command, max(10, m.width-fixedWidth), false),
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
	p, ok := m.selected()
	if !ok {
		return blankLines(height)
	}

	panel := strings.Split(m.infoPanel(p, height), "\n")
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
	width := min(clamp(m.width-8, infoMinWidth, infoMaxWidth), m.width-4)
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
	if len(wrapText(p.Command, width)) > budget && width < m.width-4 {
		width = m.width - 4
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
	indent := strings.Repeat(" ", max(0, (m.width-lipgloss.Width(box))/2))
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
		return styleDim.Render("i or esc to close")
	case m.mode == modeConfirm:
		return styleAlert.Render(pad(fmt.Sprintf("send SIGTERM to %d %s ? [y/N]",
			m.confirm.PID, firstWord(m.confirm.Command)), m.width, false))
	case m.mode == modeFilter:
		return styleLabel.Render("filter: ") + m.filter.text + styleSelected.Render(" ") +
			styleDim.Render("   enter to keep it · esc to clear")
	case m.mode == modeThreshold:
		prompt := styleLabel.Render("min: ") + m.input + styleSelected.Render(" ")
		if m.status != "" {
			return prompt + "  " + styleAlert.Render(m.status)
		}
		return prompt + styleDim.Render("   cpu>5 mem>500M time>1m · enter to apply · esc to clear")
	case m.err != nil:
		return styleAlert.Render(pad("error: "+m.err.Error(), m.width, false))
	case m.status != "":
		return styleWarn.Render(m.status)
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
	return m.spread(m.viewHelp(), styleDim.Render(state))
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
		if used+next > m.width {
			break
		}
		shown = append(shown, hint(h.key, h.label))
		used += next
	}
	return strings.Join(shown, styleDim.Render(sep))
}

func hint(key, label string) string {
	return styleKey.Render(key) + styleDim.Render(" "+label)
}

// spread puts left and right on the same line, pushed to opposite edges.
func (m Model) spread(left, right string) string {
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func firstWord(s string) string {
	if i := strings.IndexByte(s, ' '); i > 0 {
		return s[:i]
	}
	return s
}
