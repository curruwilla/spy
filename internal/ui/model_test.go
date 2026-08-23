package ui

import (
	"fmt"
	"strings"
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
	if got := back.(Model).sort; got != sortTime {
		t.Errorf("shift+tab from cpu = %v, want time (wraps around)", got)
	}
}

// TestCursorFollowsProcessAcrossRefresh is the reason the model tracks a
// pid instead of an index: a refresh must not move the highlight to another
// process.
func TestCursorFollowsProcessAcrossRefresh(t *testing.T) {
	m := testModel(t, sample())
	m = press(t, m, "j", "j") // third row by cpu: pid 20
	if got, _ := m.selected(); got.PID != 20 {
		t.Fatalf("cursor on pid %d, want 20", got.PID)
	}

	// The next snapshot has pid 20 busiest, so it moves to the top.
	refreshed := sample()
	refreshed[3].CPU = 99
	next, _ := m.Update(snapshotMsg{snap: proc.Snapshot{Processes: refreshed}})
	m = next.(Model)

	if got, _ := m.selected(); got.PID != 20 {
		t.Errorf("after a refresh the cursor moved to pid %d, want 20", got.PID)
	}
	if m.cursor != 0 {
		t.Errorf("cursor index = %d, want 0 (pid 20 is now first)", m.cursor)
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
	if got, _ := m.selected(); got.PID != m.rows[0].proc.PID || m.selectedPID != got.PID {
		t.Errorf("selection = %d, want the new first row %d", m.selectedPID, m.rows[0].proc.PID)
	}

	// The next refresh must not drag the view back to the old process.
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
