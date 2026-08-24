package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/curruwilla/spy/internal/proc"
)

// busyDisk is one process moving bytes and one that will not say whether
// it is: the two things a disk column has to be able to draw.
func busyDisk() []proc.Process {
	return []proc.Process{
		{
			PID: 10, User: "root", State: "R", Command: "postgres", CPU: 5,
			Disk: proc.Throughput{In: 12 << 20, Out: 4 << 20}, DiskKnown: true,
		},
		{PID: 11, User: "www", State: "S", Command: "nginx", CPU: 1},
	}
}

// TestDiskColumnsFitOrGoAway covers the table giving up its least
// important pair of columns rather than the command that names the row.
func TestDiskColumnsFitOrGoAway(t *testing.T) {
	cases := []struct {
		name  string
		width int
		want  bool
	}{
		{"wide enough for everything", 120, true},
		{"just wide enough", 86, true},
		{"one column short", 85, false},
		{"an ordinary terminal", 80, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := testModel(t, busyDisk())
			m.width = c.width
			m.clampView()

			titles := m.viewColumns()
			if got := strings.Contains(titles, "RD/s"); got != c.want {
				t.Errorf("disk columns shown = %v, want %v at %d columns:\n%s", got, c.want, c.width, titles)
			}
			// Whatever is dropped, the command keeps a readable share.
			cols := m.visibleColumns()
			if got := m.columnWidth(cols[len(cols)-1], cols); got < minCommandWidth {
				t.Errorf("the command column is %d cells wide, want at least %d", got, minCommandWidth)
			}
		})
	}
}

func TestDiskCellsSayWhatTheyKnow(t *testing.T) {
	m := testModel(t, busyDisk())
	m.width = 120
	m.clampView()

	rows := strings.Join(m.viewRows(), "\n")
	if !strings.Contains(rows, "12M") || !strings.Contains(rows, "4.0M") {
		t.Errorf("the rates are not in the table:\n%s", rows)
	}
	// The process whose counters are closed to the reader is not an idle
	// one, and the table has to tell the two apart.
	nginx := m.viewRow(row{proc: busyDisk()[1]}, false)
	if !strings.Contains(nginx, "-") {
		t.Errorf("row = %q, want a dash where the rates cannot be read", nginx)
	}
}

// TestColumnTitlesMarkTheSortedOne covers the arrow: it belongs on the
// column the table is ordered by, including the disk pair, which is
// sorted by both halves at once and so marks the first of them.
func TestColumnTitlesMarkTheSortedOne(t *testing.T) {
	cases := []struct {
		key   sortKey
		title string
	}{
		{sortCPU, "CPU%"},
		{sortMem, "MEM%"},
		{sortPID, "PID"},
		{sortName, "COMMAND"},
		{sortTime, "TIME"},
		{sortIO, "RD/s"},
	}
	for _, c := range cases {
		t.Run(c.key.String(), func(t *testing.T) {
			m := testModel(t, busyDisk())
			m.width = 120
			m.sort = c.key
			m.clampView()

			// A column starts on the direction it is most often read in:
			// biggest first for a measurement, smallest first for a name
			// or a number that identifies rather than measures.
			arrow := "▲"
			if c.key.descendingFirst() {
				arrow = "▼"
			}
			titles := m.viewColumns()
			if !strings.Contains(titles, c.title+arrow) {
				t.Errorf("the %s arrow is not on %s:\n%s", arrow, c.title, titles)
			}
			if strings.Count(titles, "▼")+strings.Count(titles, "▲") != 1 {
				t.Errorf("more than one column is marked as sorted:\n%s", titles)
			}
		})
	}
}

// TestInfoPanelShowsTheDetails covers the fields the panel reads a file of
// its own for: they are the reason it is worth opening on a process the
// table already has a row for.
func TestInfoPanelShowsTheDetails(t *testing.T) {
	procs := detailed()
	procs[0].Disk, procs[0].DiskKnown = proc.Throughput{In: 3 << 20, Out: 1 << 20}, true

	m := testModel(t, procs)
	m.width, m.height = 100, 40
	m.clampView()
	m = press(t, m, "i")
	m.details = proc.Details{
		CWD: "/var/www", Exe: "/usr/sbin/nginx", Cgroup: "docker 3f2a1b4c5d6e",
		Files: 42, OOMScore: 667, Swap: 8 << 20, Switches: 1620,
	}

	view := m.View()
	for _, want := range []string{
		"cgroup", "docker 3f2a1b4c5d6e",
		"exe", "/usr/sbin/nginx",
		"cwd", "/var/www",
		"files", "42",
		"ctx sw", "1620",
		"oom", "667",
		"swap", "8.0M",
		"disk r", "3.0M",
		"disk w", "1.0M",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the panel is missing %q:\n%s", want, view)
		}
	}
}

// TestInfoPanelSaysWhatItWasNotAllowedToSee covers a process of somebody
// else's: most of the panel is empty, and the reason is worth a line.
func TestInfoPanelSaysWhatItWasNotAllowedToSee(t *testing.T) {
	m := testModel(t, detailed())
	m.width, m.height = 100, 40
	m.clampView()
	m = press(t, m, "i")
	m.details = proc.Details{Files: -1, Restricted: true}

	view := m.View()
	if !strings.Contains(view, "another account") {
		t.Errorf("the panel does not say why it is empty:\n%s", view)
	}
	if !strings.Contains(view, "files") || !strings.Contains(view, "-") {
		t.Error("a count that could not be taken should read as unknown, not as none")
	}
}

// TestSignalPanelKeepsTheCursorOnScreen covers a terminal too short for
// the whole list: it scrolls to whatever is picked instead of cutting the
// bottom off and leaving the cursor outside the box.
func TestSignalPanelKeepsTheCursorOnScreen(t *testing.T) {
	for _, height := range []int{40, 24, 18, 14} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			m := testModel(t, sample())
			m.width, m.height = 100, height
			m.clampView()
			m = press(t, m, "x")
			m = press(t, m, "G") // the last signal on the list

			panel := strings.Join(m.viewSignal(), "\n")
			if !strings.Contains(panel, signals[len(signals)-1].name) {
				t.Errorf("the picked signal is not on screen:\n%s", panel)
			}
			if got := len(strings.Split(m.View(), "\n")); got != height {
				t.Errorf("view has %d lines, want exactly %d", got, height)
			}
			for i, line := range m.viewSignal() {
				if w := lipgloss.Width(line); w > m.inner() {
					t.Errorf("panel line %d is %d cells wide, want at most %d", i, w, m.inner())
				}
			}
		})
	}
}

// TestSignalPanelNamesItsProcess covers what the box is asking about: a
// signal is sent to one process, and the panel says which.
func TestSignalPanelNamesItsProcess(t *testing.T) {
	m := testModel(t, detailed())
	m.width, m.height = 100, 30
	m.clampView()
	m = press(t, m, "x")

	panel := strings.Join(m.viewSignal(), "\n")
	if !strings.Contains(panel, "signal to 1234") || !strings.Contains(panel, "/usr/sbin/nginx") {
		t.Errorf("the panel does not say what it is about:\n%s", panel)
	}
	if !strings.Contains(panel, "15  SIGTERM") {
		t.Errorf("the panel should show the number next to the name:\n%s", panel)
	}
}

// TestFooterNamesWhatIsAboutToHappen covers the two prompts that act on a
// process: both say what they will do before they do it.
func TestFooterNamesWhatIsAboutToHappen(t *testing.T) {
	m := testModel(t, detailed())
	m = press(t, m, "x")
	if got := m.viewFooter(); !strings.Contains(got, "enter to send") {
		t.Errorf("footer = %q, want the keys the list takes", got)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := next.(Model).viewFooter(); !strings.Contains(got, "SIGTERM") || !strings.Contains(got, "1234") {
		t.Errorf("footer = %q, want the signal and the process it is for", got)
	}
}
