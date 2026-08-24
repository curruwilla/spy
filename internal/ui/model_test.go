package ui

import (
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	if got := back.(Model).sort; got != sortIO {
		t.Errorf("shift+tab from cpu = %v, want io (wraps around)", got)
	}
}

// TestCursorHoldsPositionAcrossRefresh is the rule for the highlight: it
// stays on the line the user left it on, whatever process ends up there
// after the list is reordered by a refresh.
func TestCursorHoldsPositionAcrossRefresh(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "j", "j") // third row by cpu: pid 20
	if got, _ := m.selected(); got.PID != 20 {
		t.Fatalf("cursor on pid %d, want 20", got.PID)
	}

	// The next snapshot has pid 20 busiest, so it moves to the top and
	// another process takes the third row.
	refreshed := sample()
	refreshed[3].CPU = 99
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{Processes: refreshed}})
	m = next.(Model)

	if m.cursor != 2 {
		t.Errorf("cursor index = %d, want 2 (the line the user picked)", m.cursor)
	}
	if got, _ := m.selected(); got.PID != m.rows[2].proc.PID || got.PID == 20 {
		t.Errorf("cursor followed pid %d instead of staying on the third row", got.PID)
	}
}

// TestCursorClampsWhenTheListShrinks covers the one case that does move the
// cursor: the line it sits on stops existing.
func TestCursorClampsWhenTheListShrinks(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "G")
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("cursor = %d, want the last row", m.cursor)
	}

	shorter := sample()[:2]
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{Processes: shorter}})
	if m = next.(Model); m.cursor != len(m.rows)-1 {
		t.Errorf("cursor = %d, want the new last row %d", m.cursor, len(m.rows)-1)
	}
}

// TestSortJumpsToTop covers the rule that re-sorting shows the new head of
// the list: the cursor goes to the first row even though that selects a
// different process.
func TestSortJumpsToTop(t *testing.T) {
	many := make([]proc.Process, 100)
	for i := range many {
		many[i] = proc.Process{PID: i + 1, Command: "proc", CPU: float64(100 - i), RSS: uint64(i)}
	}
	m := testModel(t, many)
	m = press(t, m, "G") // scroll to the bottom
	if m.offset == 0 {
		t.Fatal("the list did not scroll")
	}

	m = press(t, m, "m")
	if m.cursor != 0 || m.offset != 0 {
		t.Errorf("after sorting: cursor=%d offset=%d, want 0 and 0", m.cursor, m.offset)
	}
	if got, _ := m.selected(); got.PID != m.rows[0].proc.PID {
		t.Errorf("selection = %d, want the new first row %d", got.PID, m.rows[0].proc.PID)
	}

	// The next refresh must leave the cursor on that first line.
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{Processes: many}})
	if m = next.(Model); m.offset != 0 || m.cursor != 0 {
		t.Errorf("after the next refresh: cursor=%d offset=%d, want 0 and 0", m.cursor, m.offset)
	}
}

// TestReverseAlsoJumpsToTop covers pressing the active column again.
func TestReverseAlsoJumpsToTop(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "G", "c")
	if m.cursor != 0 || m.offset != 0 {
		t.Errorf("reversing left cursor=%d offset=%d, want 0 and 0", m.cursor, m.offset)
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
	if m.filter.text != "ng" || len(m.rows) != 2 {
		t.Errorf("filter %q matched %d rows, want 2", m.filter.text, len(m.rows))
	}

	back, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = back.(Model)
	if m.filter.text != "n" {
		t.Errorf("filter after backspace = %q, want %q", m.filter.text, "n")
	}

	done, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = done.(Model); m.mode != modeNormal || m.filter.text != "n" {
		t.Errorf("enter should keep the filter and leave input mode, got mode=%v filter=%q", m.mode, m.filter.text)
	}

	cleared, _ := press(t, m, "/").Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m = cleared.(Model); m.filter.text != "" || len(m.rows) != len(sample()) {
		t.Errorf("esc should clear the filter, got %q with %d rows", m.filter.text, len(m.rows))
	}
}

// TestThresholdPromptAppliesOnEnter covers the > prompt: it is buffered,
// not live, because a half-typed clause does not parse.
func TestThresholdPromptAppliesOnEnter(t *testing.T) {
	m := testModel(t, busy())
	m = press(t, m, "l")
	if m.mode != modeThreshold {
		t.Fatal("l should open the threshold prompt")
	}

	m = press(t, m, "c", "p", "u", ">", "5")
	if m.input != "cpu>5" {
		t.Fatalf("input = %q, want %q", m.input, "cpu>5")
	}
	if len(m.rows) != len(busy()) {
		t.Errorf("typing filtered %d rows already, it should wait for enter", len(m.rows))
	}

	done, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = done.(Model)
	if m.mode != modeNormal {
		t.Errorf("mode = %v, want normal after enter", m.mode)
	}
	if got, want := pids(m.rows), []int{11, 10}; !equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}

	// Reopening prefills the prompt with what is active, so it can be
	// edited instead of retyped.
	m = press(t, m, "l")
	if m.input != "cpu>5%" {
		t.Errorf("input = %q, want the active threshold", m.input)
	}

	cleared, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m = cleared.(Model); m.mode != modeNormal || len(m.rows) != len(busy()) {
		t.Errorf("esc should clear the thresholds, got mode=%v with %d rows", m.mode, len(m.rows))
	}
}

// TestThresholdPromptKeepsBadInput leaves a clause that does not parse on
// screen with the reason, instead of silently dropping what was typed.
func TestThresholdPromptKeepsBadInput(t *testing.T) {
	m := testModel(t, busy())
	m = press(t, m, "l", "d", "i", "s", "k", ">", "5")
	bad, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = bad.(Model)

	if m.mode != modeThreshold || m.input != "disk>5" {
		t.Fatalf("mode=%v input=%q, want the prompt still open with the text", m.mode, m.input)
	}
	if !strings.Contains(m.View(), "unknown threshold") {
		t.Error("the prompt should say why the clause was rejected")
	}
	if len(m.rows) != len(busy()) {
		t.Error("a rejected clause should not change the list")
	}

	// Fixing it clears the complaint.
	back, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = back.(Model)
	if m.status != "" || m.input != "disk>" {
		t.Errorf("status=%q input=%q, want the error dropped on the next keystroke", m.status, m.input)
	}
}

// TestPromptsSurviveARefreshError covers a prompt opening while the footer
// is showing a failed refresh: without the echo the key looks dead, and
// everything typed after it disappears into a mode nothing announced.
func TestPromptsSurviveARefreshError(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"l", "min:"},
		{"/", "filter:"},
		{"x", "SIGTERM"},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			m := testModel(t, busy())
			m.err = errFake
			m = press(t, m, c.key)
			if !strings.Contains(m.View(), c.want) {
				t.Errorf("%q opened mode %v but the footer still shows the error", c.key, m.mode)
			}
		})
	}
}

func TestKillAsksForConfirmation(t *testing.T) {
	m := press(t, testModel(t, sample()), "x")
	if m.mode != modeSignal {
		t.Fatal("x should offer the signals before sending one")
	}
	if m.confirm.PID != m.rows[0].proc.PID {
		t.Errorf("confirming pid %d, want the selected %d", m.confirm.PID, m.rows[0].proc.PID)
	}
	if signals[m.signal].number != syscall.SIGTERM {
		t.Errorf("the list opened on %s, want SIGTERM", signals[m.signal].name)
	}

	sent, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = sent.(Model); m.mode != modeConfirm {
		t.Fatalf("enter left mode %v, want the confirmation", m.mode)
	}
	if !strings.Contains(m.View(), "send SIGTERM to") {
		t.Error("the confirmation should name the signal it is about to send")
	}

	m = press(t, m, "n")
	if m.mode != modeNormal || !strings.Contains(m.status, "cancelled") {
		t.Errorf("anything but y cancels, got mode=%v status=%q", m.mode, m.status)
	}
}

// TestSignalPickerSendsWhatWasPicked is the point of the list: the signal
// that reaches the process is the one the cursor was left on.
func TestSignalPickerSendsWhatWasPicked(t *testing.T) {
	var gotPID int
	var gotSignal syscall.Signal
	defer stubSignals(t, func(pid int, sig syscall.Signal) error {
		gotPID, gotSignal = pid, sig
		return nil
	})()

	m := press(t, testModel(t, sample()), "x")
	// SIGKILL is three above SIGTERM in the list.
	m = press(t, m, "k", "k", "k")
	if signals[m.signal].number != syscall.SIGKILL {
		t.Fatalf("cursor on %s, want SIGKILL", signals[m.signal].name)
	}
	if !strings.Contains(m.View(), "SIGKILL") {
		t.Error("the panel should show the signals to pick from")
	}

	chosen, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, chosen.(Model), "y")

	if gotPID != m.confirm.PID || gotSignal != syscall.SIGKILL {
		t.Errorf("sent %v to %d, want SIGKILL to %d", gotSignal, gotPID, m.confirm.PID)
	}
	if !strings.Contains(m.status, "SIGKILL sent") {
		t.Errorf("status = %q, want it to say what was sent", m.status)
	}
}

func TestSignalPickerCancels(t *testing.T) {
	defer stubSignals(t, func(int, syscall.Signal) error {
		t.Error("a cancelled prompt should send nothing")
		return nil
	})()

	for _, key := range []string{"esc", "q", "x"} {
		t.Run(key, func(t *testing.T) {
			m := press(t, testModel(t, sample()), "x")
			var msg tea.KeyMsg
			if key == "esc" {
				msg = tea.KeyMsg{Type: tea.KeyEsc}
			} else {
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
			}
			next, _ := m.Update(msg)
			if m = next.(Model); m.mode != modeNormal || !strings.Contains(m.status, "cancelled") {
				t.Errorf("%q left mode %v status %q, want it cancelled", key, m.mode, m.status)
			}
		})
	}
}

// TestSignalReportsARefusal covers the kill that does not go through:
// somebody else's process, or one that left between the prompt and the y.
func TestSignalReportsARefusal(t *testing.T) {
	defer stubSignals(t, func(int, syscall.Signal) error { return syscall.EPERM })()

	m := press(t, testModel(t, sample()), "x")
	sent, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, sent.(Model), "y")

	if !strings.Contains(m.status, "SIGTERM") || !strings.Contains(m.status, "not permitted") {
		t.Errorf("status = %q, want the refusal and the signal it was about", m.status)
	}
}

// stubSignals swaps the kill out for the duration of a test and returns
// the call that puts the real one back.
func stubSignals(t *testing.T, fn func(int, syscall.Signal) error) func() {
	t.Helper()
	previous := sendSignal
	sendSignal = fn
	return func() { sendSignal = previous }
}

// detailed is one process with every field the panel shows filled in.
func detailed() []proc.Process {
	return []proc.Process{{
		PID: 1234, PPID: 7, User: "root", State: "S",
		CPU: 12.5, Mem: 2.1, RSS: 48 << 20, VSize: 1180 << 20,
		CPUTime: 83 * time.Second, Uptime: 3 * time.Hour, Nice: 19, Threads: 4,
		Command: "/usr/sbin/nginx -g daemon off;",
	}}
}

// TestInfoPanelToggles covers the one key doing both jobs: i opens the
// panel and the same i closes it again.
func TestInfoPanelToggles(t *testing.T) {
	for _, close := range []string{"i", "esc", "q"} {
		t.Run(close, func(t *testing.T) {
			m := press(t, testModel(t, sample()), "i")
			if m.mode != modeInfo {
				t.Fatalf("i left mode %v, want the info panel", m.mode)
			}

			var msg tea.KeyMsg
			if close == "esc" {
				msg = tea.KeyMsg{Type: tea.KeyEsc}
			} else {
				msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(close)}
			}
			next, cmd := m.Update(msg)
			if m = next.(Model); m.mode != modeNormal {
				t.Errorf("%q left mode %v, want normal", close, m.mode)
			}
			if cmd != nil {
				t.Errorf("%q should close the panel, not quit", close)
			}
		})
	}
}

// TestInfoPanelIgnoresOtherKeys keeps the panel from acting on a table it
// is covering: only the closing keys do anything.
func TestInfoPanelIgnoresOtherKeys(t *testing.T) {
	m := press(t, testModel(t, sample()), "i")
	before := m.cursor
	m = press(t, m, "j", "m", "/", "x")
	if m.mode != modeInfo || m.cursor != before {
		t.Errorf("mode=%v cursor=%d, want the panel untouched at %d", m.mode, m.cursor, before)
	}
}

// TestInfoPanelStaysOnItsProcess pins the panel to the pid that was under
// the cursor when i was pressed: a refresh that reorders the table must not
// swap the panel for another process.
func TestInfoPanelStaysOnItsProcess(t *testing.T) {
	m := press(t, testModel(t, sample()), "j") // second row by cpu: pid 10
	m = press(t, m, "i")
	if m.info.PID != 10 {
		t.Fatalf("panel opened on pid %d, want 10", m.info.PID)
	}

	// pid 20 becomes the busiest, so the second row is now somebody else.
	refreshed := sample()
	refreshed[3].CPU = 99
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{Processes: refreshed}})
	m = next.(Model)

	if m.mode != modeInfo || m.info.PID != 10 {
		t.Errorf("after a refresh the panel shows pid %d in mode %v, want pid 10 still open", m.info.PID, m.mode)
	}
	if !strings.Contains(m.View(), "process 10") {
		t.Error("the panel should still be drawing pid 10")
	}
}

// TestInfoPanelKeepsItsNumbersFresh is the other half of pinning: the pid
// is fixed, its values are not.
func TestInfoPanelKeepsItsNumbersFresh(t *testing.T) {
	m := press(t, testModel(t, sample()), "i") // busiest row: pid 11
	if m.info.PID != 11 {
		t.Fatalf("panel opened on pid %d, want 11", m.info.PID)
	}

	refreshed := sample()
	refreshed[2].CPU = 42.5
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{Processes: refreshed}})
	if m = next.(Model); m.info.CPU != 42.5 {
		t.Errorf("panel cpu = %v, want the refreshed 42.5", m.info.CPU)
	}
}

// TestInfoPanelClosesWhenTheProcessExits covers the pid disappearing: there
// is nothing left to describe, so the panel goes away and the footer says
// why.
func TestInfoPanelClosesWhenTheProcessExits(t *testing.T) {
	m := press(t, testModel(t, sample()), "i") // pid 11

	survivors := append([]proc.Process(nil), sample()[:2]...)
	survivors = append(survivors, sample()[3])
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{Processes: survivors}})
	m = next.(Model)

	if m.mode != modeNormal {
		t.Errorf("mode = %v, want normal after the process exited", m.mode)
	}
	if m.status != "11 exited" {
		t.Errorf("status = %q, want the reason the panel closed", m.status)
	}
}

func TestInfoPanelNeedsAProcess(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "/", "z", "z", "z") // nothing matches
	done, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = press(t, done.(Model), "i")
	if m.mode == modeInfo {
		t.Error("i opened a panel with no process under the cursor")
	}
}

// TestInfoPanelShowsWhatTheTableCannot is the point of the panel: the
// fields the table has no column for.
func TestInfoPanelShowsWhatTheTableCannot(t *testing.T) {
	m := press(t, testModel(t, detailed()), "i")
	view := m.View()

	for _, want := range []string{
		"process 1234",
		"/usr/sbin/nginx -g daemon off;", // the full command, not truncated
		"S sleeping",                     // the state letter spelled out
		"parent", "7",
		"threads", "4",
		"nice", "19",
		"virtual", "1.2G",
		"running", "3h 0m",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the panel is missing %q", want)
		}
	}
	if strings.Contains(view, "COMMAND") {
		t.Error("the panel should replace the table, not sit next to its titles")
	}
}

// TestInfoPanelShowsTheWholeCommand covers the reason the command has a
// block of its own: it is the field worth opening the panel for, and it is
// far too long for a single line.
func TestInfoPanelShowsTheWholeCommand(t *testing.T) {
	const command = "/opt/google/chrome/chrome --type=renderer --crashpad-handler-pid=14938 " +
		"--enable-crash-reporter=,stable --change-stack-guard-on-fork=enable --lang=en-US " +
		"--num-raster-threads=4 --enable-main-frame-before-activation --renderer-client-id=412"

	procs := detailed()
	procs[0].Command = command
	m := testModel(t, procs)
	m.width, m.height = 120, 30
	m.clampView()
	m = press(t, m, "i")
	view := m.View()

	for _, word := range strings.Fields(command) {
		if !strings.Contains(view, word) {
			t.Errorf("the panel dropped %q from the command", word)
		}
	}
	if strings.Contains(view, "…") {
		t.Error("the command was cut short on a screen with room for all of it")
	}
	if lines := strings.Count(view, "--"); lines == 0 {
		t.Error("the command is not on screen at all")
	}
}

// TestInfoPanelWrapsRatherThanCuts covers a command too long even for the
// widened panel: it wraps over several lines and only the tail is marked.
func TestInfoPanelWrapsRatherThanCuts(t *testing.T) {
	procs := detailed()
	procs[0].Command = strings.Repeat("/some/very/long/path/to/a/binary ", 40)
	m := testModel(t, procs)
	m.width, m.height = 90, 24
	m.clampView()
	m = press(t, m, "i")

	body := strings.Join(m.viewInfo(), "\n")
	if got := strings.Count(body, "/some/very/long/path/to/a/binary"); got < 4 {
		t.Errorf("the command occupies %d copies of the path, want it wrapped over several lines", got)
	}
	if !strings.Contains(body, "…") {
		t.Error("a command that does not fit should say that there is more of it")
	}
}

// TestInfoPanelKeepsTheLayout covers the fixed screen height: the panel
// takes the table's lines, it does not add any.
func TestInfoPanelKeepsTheLayout(t *testing.T) {
	sizes := []struct{ width, height int }{{100, 24}, {80, 20}, {62, 16}, {40, 12}, {30, 9}}
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := testModel(t, detailed())
			m.width, m.height = size.width, size.height
			m.clampView()
			m = press(t, m, "i")

			for i, line := range m.viewInfo() {
				if w := lipgloss.Width(line); w > size.width {
					t.Errorf("panel line %d is %d cells wide, want at most %d", i, w, size.width)
				}
			}
			if got := len(strings.Split(m.View(), "\n")); got != size.height {
				t.Errorf("view has %d lines, want exactly %d", got, size.height)
			}
		})
	}
}

// TestInfoPanelClosesWhenTheProcessGoes covers a refresh emptying the list
// underneath an open panel: it has nothing left to describe.
func TestInfoPanelClosesWhenTheProcessGoes(t *testing.T) {
	m := press(t, testModel(t, sample()), "i")
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{}})
	if m = next.(Model); m.mode != modeNormal {
		t.Errorf("mode = %v, want normal once there is no process left", m.mode)
	}
}

// TestPauseHoldsTheScreenStill covers what space is for: the refresh stops
// where it is, and the timer keeps beating so that letting go resumes it.
func TestPauseHoldsTheScreenStill(t *testing.T) {
	m := testModel(t, sample())
	m.interval = time.Millisecond

	m = press(t, m, " ")
	if !m.paused {
		t.Fatal("space should hold the refresh")
	}
	if !strings.Contains(m.View(), "paused") {
		t.Error("a screen that is no longer refreshing has to say so")
	}

	// The tick has to come back as another tick rather than as a reading:
	// this model has no collector to read with, and a paused one must not
	// go looking for it.
	next, cmd := m.Update(tickMsg(time.Now()))
	if cmd == nil {
		t.Fatal("a paused monitor dropped its timer, so nothing would resume it")
	}
	if _, ok := cmd().(tickMsg); !ok {
		t.Error("a paused tick should re-arm the timer, not collect")
	}

	if m = press(t, next.(Model), " "); m.paused {
		t.Error("space again should let it go")
	}
}

// TestFollowKeepsTheCursorOnAProcess covers the opposite rule to the
// default one: with f pressed the cursor leaves its line and goes wherever
// the process it is watching went.
func TestFollowKeepsTheCursorOnAProcess(t *testing.T) {
	m := press(t, testModel(t, sample()), "j") // second row by cpu: pid 10
	m = press(t, m, "f")
	if m.follow != 10 {
		t.Fatalf("following %d, want the selected 10", m.follow)
	}
	if !strings.Contains(m.View(), "following 10") {
		t.Error("the footer should say which process the cursor is locked to")
	}

	// pid 10 becomes the busiest, so it moves to the top and the cursor
	// goes with it instead of staying on the second row.
	refreshed := sample()
	refreshed[1].CPU = 99
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{Processes: refreshed}})
	m = next.(Model)
	if got, _ := m.selected(); got.PID != 10 || m.cursor != 0 {
		t.Errorf("cursor on row %d holding pid %d, want the first row and pid 10", m.cursor, got.PID)
	}

	// Moving the cursor by hand is the reader taking it back.
	m = press(t, m, "j")
	if m.follow != 0 {
		t.Errorf("still following %d after the cursor was moved", m.follow)
	}
}

// TestFollowSurvivesAFilter covers a followed process the table stops
// listing: it is still running, so there is still something to follow.
func TestFollowSurvivesAFilter(t *testing.T) {
	m := press(t, testModel(t, sample()), "j", "f") // pid 10, nginx
	m = press(t, m, "/", "e", "d", "i")             // only editor matches
	done, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m = done.(Model); m.follow != 10 {
		t.Errorf("stopped following %d because the filter hid it", 10)
	}
}

// TestFollowEndsWithTheProcess covers the one thing that does release the
// lock on its own.
func TestFollowEndsWithTheProcess(t *testing.T) {
	m := press(t, testModel(t, sample()), "j", "f") // pid 10

	survivors := append(sample()[:1], sample()[2:]...)
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{Processes: survivors}})
	m = next.(Model)

	if m.follow != 0 {
		t.Errorf("still following %d, which is gone", m.follow)
	}
	if !strings.Contains(m.status, "10 exited") {
		t.Errorf("status = %q, want the reason the cursor was let go", m.status)
	}
}

func TestRenice(t *testing.T) {
	var gotPID, gotNice int
	defer stubPriority(t, func(pid, nice int) error {
		gotPID, gotNice = pid, nice
		return nil
	})()

	m := testModel(t, sample())
	m = press(t, m, "]")
	if gotPID != m.rows[0].proc.PID || gotNice != 1 {
		t.Errorf("] reniced %d to %d, want the selected process one step up", gotPID, gotNice)
	}
	if !strings.Contains(m.status, "nice 1") {
		t.Errorf("status = %q, want what it was set to", m.status)
	}

	m = press(t, m, "[")
	if gotNice != -1 {
		t.Errorf("[ set nice %d, want one step down", gotNice)
	}
}

// TestReniceStopsAtTheEndsOfTheRange covers the two ends of what the
// scheduler takes: there is nothing beyond them to ask for.
func TestReniceStopsAtTheEndsOfTheRange(t *testing.T) {
	defer stubPriority(t, func(int, int) error {
		t.Error("want no call for a process already at the end of the range")
		return nil
	})()

	procs := sample()
	procs[2].Nice = maxNice // the busiest, so it is the row under the cursor
	m := testModel(t, procs)
	if got, _ := m.selected(); got.Nice != maxNice {
		t.Fatalf("selected pid %d with nice %d, want the one at the end of the range", got.PID, got.Nice)
	}
	if m = press(t, m, "]"); !strings.Contains(m.status, "already") {
		t.Errorf("status = %q, want it to say there is nowhere further to go", m.status)
	}
}

func TestReniceReportsARefusal(t *testing.T) {
	defer stubPriority(t, func(int, int) error { return syscall.EACCES })()

	m := press(t, testModel(t, sample()), "[")
	if !strings.Contains(m.status, "renice") || !strings.Contains(m.status, "denied") {
		t.Errorf("status = %q, want the refusal", m.status)
	}
}

// TestMousePicksAndScrolls covers the two things a mouse is for: the wheel
// moves through the table and a click lands on the row under the pointer.
func TestMousePicksAndScrolls(t *testing.T) {
	many := make([]proc.Process, 60)
	for i := range many {
		many[i] = proc.Process{PID: i + 1, Command: "proc", CPU: float64(100 - i)}
	}
	m := testModel(t, many)
	m.width, m.height = 100, 30
	m.clampView()

	clicked, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		Y:      m.headerHeight() + 3,
	})
	if m = clicked.(Model); m.cursor != 3 {
		t.Errorf("clicking the fourth row of the table left the cursor at %d, want 3", m.cursor)
	}

	scrolled, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if m = scrolled.(Model); m.cursor != 3+wheelStep {
		t.Errorf("cursor = %d, want it a wheel notch further down", m.cursor)
	}
	back, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if m = back.(Model); m.cursor != 3 {
		t.Errorf("cursor = %d, want it back where it was", m.cursor)
	}

	// A click above the table is not a click on a row.
	header, _ := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: 1})
	if got := header.(Model).cursor; got != 3 {
		t.Errorf("a click on the header moved the cursor to %d", got)
	}
}

// TestMouseIsIgnoredUnderAPanel covers the panel covering the table: there
// is no row under the pointer to pick.
func TestMouseIsIgnoredUnderAPanel(t *testing.T) {
	m := press(t, testModel(t, sample()), "i")
	next, _ := m.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		Y:      m.headerHeight() + 1,
	})
	if got := next.(Model); got.cursor != m.cursor || got.mode != modeInfo {
		t.Errorf("cursor %d mode %v, want the panel untouched", got.cursor, got.mode)
	}
}

// TestHistoryKeepsTheNewestReadings covers the trend line's buffer: it
// grows with every snapshot and never past what it is allowed to hold.
func TestHistoryKeepsTheNewestReadings(t *testing.T) {
	m := testModel(t, sample())
	for i := range historyLength + 50 {
		next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{
			CPU:       proc.CPU{Total: float64(i % 100)},
			Processes: sample(),
		}})
		m = next.(Model)
	}
	if len(m.history) != historyLength {
		t.Errorf("history holds %d readings, want it capped at %d", len(m.history), historyLength)
	}
	if want := float64((historyLength + 49) % 100); m.history[len(m.history)-1] != want {
		t.Errorf("newest reading = %v, want %v", m.history[len(m.history)-1], want)
	}
}

// stubPriority swaps the renice out for the duration of a test and returns
// the call that puts the real one back.
func stubPriority(t *testing.T, fn func(pid, nice int) error) func() {
	t.Helper()
	previous := setPriority
	setPriority = fn
	return func() { setPriority = previous }
}

// TestFooterHelpFitsTheWidth covers the footer dropping hints instead of
// wrapping, which would push the bottom line off the screen.
func TestFooterHelpFitsTheWidth(t *testing.T) {
	for _, width := range []int{120, 80, 60, 40, 20, 8} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := testModel(t, sample())
			m.width = width
			if got := lipgloss.Width(m.viewFooter()); got > width {
				t.Errorf("footer is %d cells wide, want at most %d", got, width)
			}
		})
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

// TestHeaderSeparatesItsGroups covers the blank lines in the header: the
// margin above the title, and the ones that keep the cpu block from
// reading as part of the memory one below it. A short terminal keeps the
// blank between the two blocks and gives the rest back to the table.
func TestHeaderSeparatesItsGroups(t *testing.T) {
	cases := []struct {
		name   string
		height int
		blanks []int // header lines expected to be empty
	}{
		{"tall", 30, []int{0, 2, 6, 9}},
		{"compact", compactHeight - 1, []int{3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := testModel(t, sample())
			m.height = c.height
			m.clampView()

			lines := m.viewHeader()
			// The column titles are the last header line and are drawn by
			// the table, not by the header itself.
			if len(lines) != m.headerHeight()-1 {
				t.Fatalf("header has %d lines, want %d", len(lines), m.headerHeight()-1)
			}
			var blanks []int
			for i, line := range lines {
				if strings.TrimSpace(line) == "" {
					blanks = append(blanks, i)
				}
			}
			if fmt.Sprint(blanks) != fmt.Sprint(c.blanks) {
				t.Errorf("blank lines at %v, want %v:\n%s", blanks, c.blanks, strings.Join(lines, "\n"))
			}
			// The blank both layouts keep is the one between the cpu block
			// and the memory gauges.
			separated := false
			for _, i := range blanks {
				separated = separated || i > 0 && i+1 < len(lines) &&
					strings.Contains(lines[i-1], "CPU") && strings.Contains(lines[i+1], "MEM")
			}
			if !separated {
				t.Errorf("nothing separates the cpu block from the memory one:\n%s", strings.Join(lines, "\n"))
			}
			if got := len(strings.Split(m.View(), "\n")); got != c.height {
				t.Errorf("view has %d lines, want exactly %d", got, c.height)
			}
		})
	}
}

// TestBarsFillTheWholeLine covers the three filled lines — the title, the
// column titles and the footer. Their background has to reach both edges
// of the screen: a bar that stops where its text ends looks like a stray
// highlight rather than a bar.
func TestBarsFillTheWholeLine(t *testing.T) {
	base := testModel(t, sample())
	base.width, base.height = 100, 30
	base.clampView()

	filtering := base
	filtering.mode, filtering.filter.text = modeFilter, "fire"

	refused := base
	refused.mode, refused.input, refused.status = modeThreshold, "cpu>x", "unknown threshold"

	picking := base
	picking.mode, picking.confirm, picking.signal = modeSignal, sample()[0], defaultSignal()

	confirming := picking
	confirming.mode = modeConfirm

	held := base
	held.paused = true

	locked := base
	locked.follow = 10

	failed := base
	failed.err = errFake

	noted := base
	noted.status = "SIGTERM sent to 10"

	informing := base
	informing.mode = modeInfo

	cases := []struct{ name, line string }{
		{"title", base.viewTitle()},
		{"column titles", base.viewColumns()},
		{"footer", base.viewFooter()},
		{"filter prompt", filtering.viewFooter()},
		{"refused threshold", refused.viewFooter()},
		{"signal list", picking.viewFooter()},
		{"kill confirmation", confirming.viewFooter()},
		{"paused", held.viewFooter()},
		{"following a process", locked.viewFooter()},
		{"refresh error", failed.viewFooter()},
		{"status message", noted.viewFooter()},
		{"open panel", informing.viewFooter()},
	}
	for _, c := range cases {
		if w := lipgloss.Width(c.line); w != base.inner() {
			t.Errorf("the %s bar is %d columns wide, want the full %d: %q",
				c.name, w, base.inner(), c.line)
		}
	}
}

// TestScreenKeepsItsMargins covers the gutter: nothing is drawn against
// the edges of the terminal, and a screen tall enough for it keeps a blank
// line above the title too.
func TestScreenKeepsItsMargins(t *testing.T) {
	sizes := []struct{ width, height int }{{120, 30}, {100, 24}, {80, 20}}
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := testModel(t, sample())
			m.width, m.height = size.width, size.height
			m.clampView()

			lines := strings.Split(m.View(), "\n")
			if top := m.height >= compactHeight; top != (lines[0] == "") {
				t.Errorf("first line = %q, want a blank one above the title: %v", lines[0], top)
			}
			for i, line := range lines {
				if line == "" {
					continue // an empty line carries no trailing spaces
				}
				if !strings.HasPrefix(line, strings.Repeat(" ", gutter)) {
					t.Errorf("line %d starts at the left edge: %q", i, line)
				}
				if w := lipgloss.Width(line); w > size.width-gutter {
					t.Errorf("line %d is %d cells wide, want at most %d: %q",
						i, w, size.width-gutter, line)
				}
			}
		})
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
