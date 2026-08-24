package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/curruwilla/spy/internal/proc"
)

func TestGaugeSpacesItsCells(t *testing.T) {
	cases := []struct {
		name  string
		pct   float64
		cells int
		want  string
	}{
		{"empty", 0, 4, wantGauge(0, 4)},
		{"half", 50, 4, wantGauge(2, 2)},
		{"full", 100, 4, wantGauge(4, 0)},
		{"below zero", -10, 4, wantGauge(0, 4)},
		{"above a hundred", 150, 4, wantGauge(4, 0)},
		{"no room", 50, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := gauge(c.pct, c.cells)
			if got != c.want {
				t.Errorf("gauge(%v, %d) = %q, want %q", c.pct, c.cells, got, c.want)
			}
			if w := lipgloss.Width(got); w != gaugeWidth(c.cells) {
				t.Errorf("gauge is %d columns wide, want gaugeWidth = %d", w, gaugeWidth(c.cells))
			}
		})
	}
}

func TestSparklineFitsTheWidth(t *testing.T) {
	cases := []struct {
		name   string
		cores  int
		width  int
		want   string
		reason string
	}{
		{name: "grouped in fours", cores: 8, width: 40, want: "▅▅▅▅ ▅▅▅▅"},
		{name: "gaps dropped to fit", cores: 8, width: 8, want: "▅▅▅▅▅▅▅▅"},
		{name: "trimmed when even that is too wide", cores: 10, width: 6, want: "▅▅▅▅▅…"},
		{name: "no room at all", cores: 8, width: 0, want: ""},
		{name: "nothing to draw", cores: 0, width: 20, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sparkline(halfLoaded(c.cores), c.width)
			if got != c.want {
				t.Errorf("sparkline(%d cores, width %d) = %q, want %q", c.cores, c.width, got, c.want)
			}
			if w := lipgloss.Width(got); w > c.width {
				t.Errorf("sparkline is %d cells wide, want at most %d", w, c.width)
			}
		})
	}
}

// TestHeaderNeverWrapsTheScreen covers the header giving up detail rather
// than a second line: it has a fixed height, and a line that wraps pushes
// the footer off the bottom of the screen.
func TestHeaderNeverWrapsTheScreen(t *testing.T) {
	for _, width := range []int{120, 100, 80, 60, 40, 30, 20} {
		for _, cores := range []int{1, 4, 16, 64, 128} {
			t.Run(fmt.Sprintf("%d wide, %d cores", width, cores), func(t *testing.T) {
				m := testModel(t, sample())
				m.width = width
				m.snap.CPU.Cores = halfLoaded(cores)

				lines := m.viewHeader()
				for i, line := range lines {
					if w := lipgloss.Width(line); w > width {
						t.Errorf("header line %d is %d cells wide, want at most %d:\n%s",
							i, w, width, strings.Join(lines, "\n"))
					}
				}
			})
		}
	}
}

// TestGaugesGrowWithTheScreenAndKeepTheirDetail covers how the bars are
// sized: they take what is left of the line once the widest detail has had
// its room, so a wider terminal gets a longer bar and none of them pushes
// the load or the memory figures off the screen.
func TestGaugesGrowWithTheScreenAndKeepTheirDetail(t *testing.T) {
	previous := 0
	for _, width := range []int{60, 80, 100, 140, 200} {
		m := testModel(t, sample())
		m.width = width
		m.snap.CPU.Cores = halfLoaded(8)

		cells := m.gaugeCells(detail("load", "1.00  1.00  1.00"))
		if cells < previous {
			t.Errorf("at %d columns the bar is %d cells, narrower than the %d it had on a smaller screen",
				width, cells, previous)
		}
		previous = cells

		var cpu string
		for _, line := range m.viewHeader() {
			if strings.HasPrefix(line, "CPU") {
				cpu = line
			}
		}
		if !strings.Contains(cpu, "load") {
			t.Errorf("at %d columns the cpu line lost its detail: %q", width, cpu)
		}
		if w := lipgloss.Width(cpu); w > m.inner() {
			t.Errorf("the cpu line is %d columns wide, want at most %d: %q", w, m.inner(), cpu)
		}
	}
}

// loaded is a machine with something to say on every header line: cores
// under load, traffic on both the disk and the network, and a processor
// warm enough to have a temperature worth showing.
func loaded(t *testing.T) Model {
	t.Helper()
	m := testModel(t, sample())
	m.width, m.height = 120, 30
	m.snap.CPU = proc.CPU{
		Total: 42, Cores: halfLoaded(8),
		Model: "Intel Core i7-8700K", Temp: 52,
	}
	m.snap.Disk = proc.Throughput{In: 12 << 20, Out: 4 << 20}
	m.snap.Net = proc.Throughput{In: 1 << 20, Out: 512 << 10}
	m.history = halfLoaded(40)
	m.clampView()
	return m
}

// headerLine is the header line starting with the given label.
func headerLine(t *testing.T, m Model, label string) string {
	t.Helper()
	for _, line := range m.viewHeader() {
		if strings.HasPrefix(line, label) {
			return line
		}
	}
	t.Fatalf("no header line labelled %q:\n%s", label, strings.Join(m.viewHeader(), "\n"))
	return ""
}

// TestHeaderShowsTheTrafficAndTheTemperature covers the readings that have
// no gauge of their own: they ride on the right of the lines that do.
func TestHeaderShowsTheTrafficAndTheTemperature(t *testing.T) {
	m := loaded(t)
	for _, c := range []struct{ label, want string }{
		{"core", "disk read 12M/s  write 4.0M/s"},
		{"hist", "net rx 1.0M/s  tx 512K/s"},
		{"CPU", "temp 52°C"},
	} {
		if line := headerLine(t, m, c.label); !strings.Contains(line, c.want) {
			t.Errorf("the %s line is missing %q: %q", c.label, c.want, line)
		}
	}
}

// TestHeaderDetailsShareAColumn covers the shape of the header: every
// detail on the right starts at the same place, whether the line it is on
// carries a bar or a spark line.
func TestHeaderDetailsShareAColumn(t *testing.T) {
	for _, width := range []int{160, 120, 100, 90} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := loaded(t)
			m.width = width

			at := map[string]int{
				"core": detailColumn(headerLine(t, m, "core"), "disk"),
				"hist": detailColumn(headerLine(t, m, "hist"), "net"),
				"CPU":  detailColumn(headerLine(t, m, "CPU"), "load"),
				"MEM":  detailColumn(headerLine(t, m, "MEM"), "used"),
			}
			for label, column := range at {
				if column < 0 {
					t.Fatalf("the %s line lost its detail at %d columns", label, width)
				}
				if column != at["CPU"] {
					t.Errorf("the %s detail starts at column %d, want %d like the rest:\n%s",
						label, column, at["CPU"], strings.Join(m.viewHeader(), "\n"))
				}
			}
		})
	}
}

// detailColumn is the column a header line's detail starts at. The bars
// and the spark cells are multi-byte glyphs, so where the label sits in
// the string is not where it sits on the screen.
func detailColumn(line, label string) int {
	i := strings.Index(line, label)
	if i < 0 {
		return -1
	}
	return lipgloss.Width(line[:i])
}

// TestHeaderWithoutASensorSaysNothing covers a machine that exposes no
// temperature: an empty label would only raise the question.
func TestHeaderWithoutASensorSaysNothing(t *testing.T) {
	m := loaded(t)
	m.snap.CPU.Temp = 0
	if line := headerLine(t, m, "CPU"); strings.Contains(line, "temp") {
		t.Errorf("cpu line = %q, want no temperature at all", line)
	}
}

// TestTitleNamesTheProcessorWhileItFits covers the one detail with two
// forms: the model is worth the room when there is room, and the count of
// cores is what is left when there is not.
func TestTitleNamesTheProcessorWhileItFits(t *testing.T) {
	cases := []struct {
		width int
		want  string
	}{
		{160, "8 × Intel Core i7-8700K"},
		{120, "8 × Intel Core i7-8700K"},
		{48, "8 cores"},
	}
	for _, c := range cases {
		t.Run(fmt.Sprint(c.width), func(t *testing.T) {
			m := loaded(t)
			m.width = c.width
			title := m.viewTitle()
			if !strings.Contains(title, c.want) {
				t.Errorf("title = %q, want it to hold %q", title, c.want)
			}
			if w := lipgloss.Width(title); w != m.inner() {
				t.Errorf("title is %d columns wide, want the full %d", w, m.inner())
			}
		})
	}
}

// TestTitleWithoutAProcessorName covers the machines that do not say what
// they are: the core count is all there is to show.
func TestTitleWithoutAProcessorName(t *testing.T) {
	m := loaded(t)
	m.snap.CPU.Model = ""
	if title := m.viewTitle(); !strings.Contains(title, "8 cores") {
		t.Errorf("title = %q, want the core count on its own", title)
	}
}

// TestTailValuesKeepsTheNewest covers the trend line: what does not fit is
// dropped from the old end, because a trend is read from the right.
func TestTailValuesKeepsTheNewest(t *testing.T) {
	values := make([]float64, 100)
	for i := range values {
		values[i] = float64(i)
	}
	cases := []struct {
		name   string
		values []float64
		width  int
		want   []float64
	}{
		{name: "all of it", values: values[:4], width: 40, want: values[:4]},
		{name: "the newest end", values: values, width: 10, want: values[92:]},
		{name: "nothing to draw", values: nil, width: 10},
		{name: "no room at all", values: values, width: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := tailValues(c.values, c.width)
			if fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("tailValues(%d values, %d) = %v, want %v", len(c.values), c.width, got, c.want)
			}
			if w := lipgloss.Width(sparkline(got, c.width)); w > c.width {
				t.Errorf("the line it draws is %d columns wide, want at most %d", w, c.width)
			}
		})
	}
}

// TestSparkWidthCountsTheGaps keeps the two halves of the spark line in
// step: what tailValues measures with and what sparkline then draws.
func TestSparkWidthCountsTheGaps(t *testing.T) {
	for n := 1; n <= 40; n++ {
		if got, want := sparkWidth(n), lipgloss.Width(sparkline(halfLoaded(n), sparkWidth(n))); got != want {
			t.Errorf("sparkWidth(%d) = %d, but the line it draws is %d wide", n, got, want)
		}
	}
	if got := sparkWidth(0); got != 0 {
		t.Errorf("sparkWidth(0) = %d, want 0", got)
	}
}

// TestActiveMarksWhatIsDoingSomething covers which rows are emphasised:
// what holds a core, a slice of one, or a serious amount of memory.
func TestActiveMarksWhatIsDoingSomething(t *testing.T) {
	cases := []struct {
		name string
		proc proc.Process
		want bool
	}{
		{"on a core", proc.Process{State: "R"}, true},
		{"burning cpu", proc.Process{State: "S", CPU: 12.5}, true},
		{"holding memory", proc.Process{State: "S", Mem: 9}, true},
		{"barely awake", proc.Process{State: "S", CPU: 0.4, Mem: 0.2}, false},
		{"asleep", proc.Process{State: "S"}, false},
		{"idle kernel thread", proc.Process{State: "I", Kernel: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := active(c.proc); got != c.want {
				t.Errorf("active(%+v) = %v, want %v", c.proc, got, c.want)
			}
		})
	}
}

// TestStateStyleMarksTrouble covers the state letter: the states that mean
// something is wrong, or about to be, are colored apart from the sleeping
// majority of the table.
func TestStateStyleMarksTrouble(t *testing.T) {
	asleep := stateStyle("S").GetForeground()
	for _, state := range []string{"R", "D", "Z", "T", "t"} {
		if stateStyle(state).GetForeground() == asleep {
			t.Errorf("state %q is drawn like a sleeping process, want it to stand out", state)
		}
	}
	for _, state := range []string{"I", "X", "?"} {
		if stateStyle(state).GetForeground() != asleep {
			t.Errorf("state %q stands out, want it drawn like a sleeping process", state)
		}
	}
}

// TestOwnProcessesAreMarked covers whose row is whose: the account the
// monitor runs under is what the reader is usually looking for.
func TestOwnProcessesAreMarked(t *testing.T) {
	m := testModel(t, sample())
	m.me = "will"
	if m.userStyle("will").GetForeground() == m.userStyle("root").GetForeground() {
		t.Error("the reader's own processes are drawn like everyone else's")
	}
	if m.userStyle("postgres").GetForeground() != m.userStyle("root").GetForeground() {
		t.Error("two accounts that are not the reader's are drawn differently")
	}
}

// wantGauge spells out the bar the gauge constants draw, so the cells a
// test expects follow whatever glyphs they are set to.
func wantGauge(filled, empty int) string {
	cells := make([]string, 0, filled+empty)
	for range filled {
		cells = append(cells, gaugeFilled)
	}
	for range empty {
		cells = append(cells, gaugeEmpty)
	}
	return gaugeOpen + strings.Join(cells, " ") + gaugeClose
}

// halfLoaded is n cores at the same middling usage, so the cells they draw
// are predictable.
func halfLoaded(n int) []float64 {
	cores := make([]float64, n)
	for i := range cores {
		cores[i] = 50
	}
	return cores
}
