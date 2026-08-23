package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/curruwilla/spy/internal/proc"
)

// testModel returns a model already holding the sample snapshot, without a
// collector: nothing in these tests reads /proc.
func testModel(t *testing.T, procs []proc.Process) Model {
	t.Helper()
	m, err := New(nil, Options{Interval: time.Second, Sort: "cpu"})
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 20
	m.snap = proc.Snapshot{At: time.Now(), Processes: procs}
	m.rebuild()
	return m
}

// press feeds printable keys to the model, one at a time.
func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		m = next.(Model)
	}
	return m
}

func TestNewRejectsUnknownSortColumn(t *testing.T) {
	if _, err := New(nil, Options{Sort: "disk"}); err == nil {
		t.Error("want an error for an unknown sort column")
	}
}

func TestSortKeySwitchesAndToggles(t *testing.T) {
	m := testModel(t, sample())

	m = press(t, m, "m")
	if m.sort != sortMem || m.reverse {
		t.Errorf("after m: sort=%v reverse=%v, want mem descending", m.sort, m.reverse)
	}
	m = press(t, m, "m")
	if m.sort != sortMem || !m.reverse {
		t.Errorf("pressing the active column again should reverse it, got reverse=%v", m.reverse)
	}
	m = press(t, m, "p")
	if m.sort != sortPID || m.reverse {
		t.Errorf("switching column should reset the direction, got %v reverse=%v", m.sort, m.reverse)
	}
}

func TestTabCyclesColumns(t *testing.T) {
	m := testModel(t, sample())
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := next.(Model).sort; got != sortMem {
		t.Errorf("tab from cpu = %v, want mem", got)
	}
	back, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if got := back.(Model).sort; got != sortTime {
		t.Errorf("shift+tab from cpu = %v, want time (wraps around)", got)
	}
}

// TestCursorFollowsProcess is the reason the model tracks a pid instead of
// an index: a refresh must not move the highlight to another process.
func TestCursorFollowsProcess(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "j", "j") // third row by cpu: pid 20
	selected, _ := m.selected()
	if selected.PID != 20 {
		t.Fatalf("cursor on pid %d, want 20", selected.PID)
	}

	m = press(t, m, "p") // reorder by pid: 1, 10, 11, 20
	if got, _ := m.selected(); got.PID != 20 {
		t.Errorf("after re-sorting the cursor moved to pid %d, want 20", got.PID)
	}
	if m.cursor != 3 {
		t.Errorf("cursor index = %d, want 3", m.cursor)
	}
}

func TestCursorStaysInsideList(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "k", "k") // already at the top
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	m = press(t, m, "G")
	if m.cursor != len(m.rows)-1 {
		t.Errorf("cursor = %d, want the last row %d", m.cursor, len(m.rows)-1)
	}
}

func TestScrollingKeepsCursorVisible(t *testing.T) {
	many := make([]proc.Process, 100)
	for i := range many {
		many[i] = proc.Process{PID: i + 1, Command: "proc", CPU: float64(100 - i)}
	}
	m := testModel(t, many)
	m = press(t, m, "G")

	height := m.tableHeight()
	if m.cursor < m.offset || m.cursor >= m.offset+height {
		t.Errorf("cursor %d outside the window [%d, %d)", m.cursor, m.offset, m.offset+height)
	}
	if want := len(m.rows) - height; m.offset != want {
		t.Errorf("offset = %d, want %d", m.offset, want)
	}
}

func TestFilterTypingIsLive(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "/")
	if m.mode != modeFilter {
		t.Fatal("/ should open the filter")
	}

	m = press(t, m, "n", "g")
	if m.filter != "ng" || len(m.rows) != 2 {
		t.Errorf("filter %q matched %d rows, want 2", m.filter, len(m.rows))
	}

	back, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = back.(Model)
	if m.filter != "n" {
		t.Errorf("filter after backspace = %q, want %q", m.filter, "n")
	}

	done, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = done.(Model); m.mode != modeNormal || m.filter != "n" {
		t.Errorf("enter should keep the filter and leave input mode, got mode=%v filter=%q", m.mode, m.filter)
	}

	cleared, _ := press(t, m, "/").Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m = cleared.(Model); m.filter != "" || len(m.rows) != len(sample()) {
		t.Errorf("esc should clear the filter, got %q with %d rows", m.filter, len(m.rows))
	}
}

func TestKillAsksForConfirmation(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "x")
	if m.mode != modeConfirm {
		t.Fatal("x should ask before sending a signal")
	}
	if m.confirm.PID != m.rows[0].proc.PID {
		t.Errorf("confirming pid %d, want the selected %d", m.confirm.PID, m.rows[0].proc.PID)
	}

	m = press(t, m, "n")
	if m.mode != modeNormal || !strings.Contains(m.status, "cancelled") {
		t.Errorf("anything but y cancels, got mode=%v status=%q", m.mode, m.status)
	}
}

func TestQuitKeys(t *testing.T) {
	for _, key := range []string{"q", "esc", "ctrl+c"} {
		t.Run(key, func(t *testing.T) {
			m := testModel(t, sample())
			var msg tea.KeyMsg
			switch key {
			case "q":
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}
			case "esc":
				msg = tea.KeyMsg{Type: tea.KeyEsc}
			default:
				msg = tea.KeyMsg{Type: tea.KeyCtrlC}
			}
			if _, cmd := m.Update(msg); cmd == nil {
				t.Errorf("%s should quit", key)
			}
		})
	}
}

func TestSnapshotErrorIsKept(t *testing.T) {
	m := testModel(t, sample())
	next, _ := m.Update(snapshotMsg{err: errFake})
	m = next.(Model)
	if m.err == nil {
		t.Fatal("a failed collect should be reported")
	}
	if len(m.rows) != len(sample()) {
		t.Error("a failed collect should keep the last good snapshot on screen")
	}
	if !strings.Contains(m.View(), "error") {
		t.Error("the footer should show the error")
	}
}

func TestViewFillsTheTerminal(t *testing.T) {
	m := testModel(t, sample())
	lines := strings.Split(m.View(), "\n")
	if len(lines) != m.height {
		t.Errorf("view has %d lines, want exactly %d", len(lines), m.height)
	}
	if !strings.Contains(lines[0], "spy") {
		t.Errorf("first line = %q, want the title", lines[0])
	}
}

func TestViewWithoutMatchesExplainsItself(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "/", "z", "z", "z")
	if !strings.Contains(m.View(), "no process matches") {
		t.Error("an empty list should say why it is empty")
	}
}

var errFake = fakeError("meminfo: permission denied")

type fakeError string

func (e fakeError) Error() string { return string(e) }
