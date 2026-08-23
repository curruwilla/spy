package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	lines = append(lines, m.viewColumns())
	lines = append(lines, m.viewRows()...)
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

func (m Model) viewFooter() string {
	// A prompt owns the footer while it is open: the refresh error is
	// still there when it closes, but a prompt with no echo looks like a
	// key that did nothing.
	switch {
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

	help := strings.Join([]string{
		hint("↑↓", "move"),
		hint("c/m/p/n", "sort"),
		hint("tab", "column"),
		hint("t", "tree"),
		hint("/", "filter"),
		hint("l", "min"),
		hint("x", "kill"),
		hint("q", "quit"),
	}, styleDim.Render(" · "))

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
	return m.spread(help, styleDim.Render(state))
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
