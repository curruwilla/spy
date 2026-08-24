package ui

import (
	"fmt"
	"os/user"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/curruwilla/spy/internal/proc"
)

// mode is what the screen is currently doing: showing the table, reading a
// filter or a set of thresholds, picking a signal, asking to confirm the
// kill, or holding the detail panel open over the table.
type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeThreshold
	modeSignal
	modeConfirm
	modeInfo
)

// The two operations that reach outside the program. They are variables so
// the tests can drive the keys that lead to them without signalling
// anything real.
var (
	sendSignal  = syscall.Kill
	setPriority = func(pid, nice int) error {
		return syscall.Setpriority(syscall.PRIO_PROCESS, pid, nice)
	}
)

// The range the scheduler takes, from the most urgent to the most patient.
// See setpriority(2).
const (
	minNice = -20
	maxNice = 19
)

// wheelStep is how far one notch of the mouse wheel moves the cursor.
const wheelStep = 3

// historyLength is how many readings the trend line keeps. It is more than
// the widest terminal has cells for, so the line is always drawn from the
// newest end of a full buffer.
const historyLength = 512

// Options configures the monitor from the command line.
type Options struct {
	Interval time.Duration
	Sort     string
	Tree     bool
	Filter   string
	Min      string // threshold clauses, e.g. "cpu>5 mem>500M time>1m"
}

// currentUser is the account the monitor runs under, read once: it is how
// a row tells the reader's own processes from everyone else's, and it
// cannot change while the program runs.
func currentUser() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}

// Model is the whole application state.
type Model struct {
	collector *proc.Collector
	interval  time.Duration

	snap proc.Snapshot
	rows []row
	err  error

	width  int
	height int

	sort    sortKey
	reverse bool
	tree    bool
	filter  filter
	mode    mode
	input   string // what is being typed at the threshold prompt

	cursor int // index into rows, kept where the user left it
	offset int // first visible row, for scrolling

	paused  bool      // the refresh is held, so the screen can be read
	follow  int       // pid the cursor is locked to, 0 when it is free to stay put
	history []float64 // total cpu of the last snapshots, oldest first

	me      string       // account the monitor runs under, to mark its own processes
	info    proc.Process // process the detail panel describes, fixed when it opened
	details proc.Details // and what it costs a file of its own to know about it
	confirm proc.Process // process awaiting a kill confirmation
	signal  int          // index into signals, the one the kill prompt will send
	status  string       // one-off message shown in the footer
}

// New builds the model. It fails on an invalid sort column or on
// thresholds it cannot parse.
func New(c *proc.Collector, opts Options) (Model, error) {
	key, err := parseSortKey(opts.Sort)
	if err != nil {
		return Model{}, err
	}
	min, err := parseThresholds(opts.Min)
	if err != nil {
		return Model{}, err
	}
	return Model{
		collector: c,
		interval:  opts.Interval,
		me:        currentUser(),
		sort:      key,
		tree:      opts.Tree,
		filter:    filter{text: opts.Filter, min: min},
		width:     80,
		height:    24,
	}, nil
}

type snapshotMsg struct {
	snap proc.Snapshot
	err  error
}

type tickMsg time.Time

// Init reads the first snapshot right away. Every later read is scheduled
// by the previous one, so only ever one collect is in flight.
func (m Model) Init() tea.Cmd {
	return m.collect()
}

// collect reads /proc off the update loop so typing stays responsive.
func (m Model) collect() tea.Cmd {
	return func() tea.Msg {
		snap, err := m.collector.Collect()
		return snapshotMsg{snap: snap, err: err}
	}
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampView()

	case snapshotMsg:
		m.err = msg.err
		if msg.err == nil {
			m.snap = msg.snap
			m.recordHistory()
			m.rebuild()
		}
		return m, tick(m.interval)

	case tickMsg:
		// A paused monitor keeps the heartbeat and skips the reading, so
		// there is still exactly one timer in flight to resume from.
		if m.paused {
			return m, tick(m.interval)
		}
		return m, m.collect()

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// recordHistory adds the new reading to the trend line, dropping the
// oldest once there is more of it than any screen can show.
func (m *Model) recordHistory() {
	m.history = append(m.history, m.snap.CPU.Total)
	if len(m.history) > historyLength {
		m.history = m.history[len(m.history)-historyLength:]
	}
}

// handleMouse gives the table the two things a mouse is good for: the
// wheel scrolls it, and a click picks the row under the pointer. Both are
// ignored while a panel covers the table, because there is no row there.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeInfo || m.mode == modeSignal {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveCursor(-wheelStep)
	case tea.MouseButtonWheelDown:
		m.moveCursor(wheelStep)
	case tea.MouseButtonLeft:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		// The rows start under the header, the column titles included.
		row := m.offset + msg.Y - m.headerHeight()
		if msg.Y >= m.headerHeight() && row < len(m.rows) {
			m.follow, m.cursor = 0, row
			m.clampView()
		}
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeFilter:
		return m.handleFilterKey(msg)
	case modeThreshold:
		return m.handleThresholdKey(msg)
	case modeSignal:
		return m.handleSignalKey(msg)
	case modeConfirm:
		return m.handleConfirmKey(msg)
	case modeInfo:
		return m.handleInfoKey(msg)
	}

	m.status = ""
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup", "ctrl+b":
		m.moveCursor(-m.tableHeight())
	case "pgdown", "ctrl+f":
		m.moveCursor(m.tableHeight())
	case "home", "g":
		m.moveCursor(-len(m.rows))
	case "end", "G":
		m.moveCursor(len(m.rows))

	case "c":
		m.sortBy(sortCPU)
	case "m":
		m.sortBy(sortMem)
	case "p":
		m.sortBy(sortPID)
	case "n":
		m.sortBy(sortName)
	case "d":
		m.sortBy(sortIO)
	case "tab":
		m.sortBy(m.sort.next(1))
	case "shift+tab":
		m.sortBy(m.sort.next(-1))

	case "t":
		m.tree = !m.tree
		m.rebuild()
	case " ":
		// Holding the screen still is the only way to read a busy table:
		// the last snapshot stays up until it is let go.
		m.paused = !m.paused
	case "f":
		m.toggleFollow()
	case "[":
		m.renice(-1)
	case "]":
		m.renice(1)
	case "/":
		m.mode = modeFilter
	case "l":
		// Prefilled with what is already active, so it can be edited
		// instead of retyped.
		m.mode, m.input = modeThreshold, m.filter.min.String()
	case "i":
		// The panel is about the process that was under the cursor at this
		// keypress, not about whatever the cursor points at later.
		if p, ok := m.selected(); ok {
			m.info, m.mode = p, modeInfo
			m.readDetails()
		}
	case "x":
		if p, ok := m.selected(); ok {
			m.confirm, m.mode, m.signal = p, modeSignal, defaultSignal()
		}
	}
	return m, nil
}

// toggleFollow locks the cursor onto the process under it, or lets it go.
// Without it the cursor holds its line and the list moves under it, which
// is what a reader watching the top of the table wants; with it the cursor
// holds one process wherever the sorting sends it.
func (m *Model) toggleFollow() {
	if m.follow != 0 {
		m.follow, m.status = 0, "following nothing"
		return
	}
	p, ok := m.selected()
	if !ok {
		return
	}
	m.follow = p.PID
	m.status = fmt.Sprintf("following %d %s", p.PID, firstWord(p.Command))
}

// renice moves the selected process along the scheduler's range: down
// towards the urgent end, which only root may do, or up towards the
// patient one, which anybody may do to their own processes. A refusal is
// reported in the footer like any other.
func (m *Model) renice(delta int) {
	p, ok := m.selected()
	if !ok {
		return
	}
	nice := clamp(p.Nice+delta, minNice, maxNice)
	if nice == p.Nice {
		m.status = fmt.Sprintf("%d is already at nice %d", p.PID, nice)
		return
	}
	if err := setPriority(p.PID, nice); err != nil {
		m.status = fmt.Sprintf("renice %d: %v", p.PID, err)
		return
	}
	m.status = fmt.Sprintf("%d now nice %d", p.PID, nice)
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		m.mode = modeNormal
	case tea.KeyEsc:
		m.mode, m.filter.text = modeNormal, ""
		m.rebuild()
	case tea.KeyBackspace:
		if runes := []rune(m.filter.text); len(runes) > 0 {
			m.filter.text = string(runes[:len(runes)-1])
			m.rebuild()
		}
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyRunes, tea.KeySpace:
		m.filter.text += string(msg.Runes)
		if msg.Type == tea.KeySpace {
			m.filter.text += " "
		}
		m.rebuild()
	}
	return m, nil
}

// handleThresholdKey reads the "cpu>5 mem>500M" prompt. Unlike the text
// filter it applies on enter, because half-typed clauses do not parse; a
// clause that never parses keeps the prompt open with the reason in it.
func (m Model) handleThresholdKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.status = ""
	switch msg.Type {
	case tea.KeyEnter:
		min, err := parseThresholds(m.input)
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.mode, m.filter.min = modeNormal, min
		m.rebuild()
	case tea.KeyEsc:
		m.mode, m.input, m.filter.min = modeNormal, "", thresholds{}
		m.rebuild()
	case tea.KeyBackspace:
		if runes := []rune(m.input); len(runes) > 0 {
			m.input = string(runes[:len(runes)-1])
		}
	case tea.KeyCtrlC:
		return m, tea.Quit
	case tea.KeyRunes, tea.KeySpace:
		m.input += string(msg.Runes)
		if msg.Type == tea.KeySpace {
			m.input += " "
		}
	}
	return m, nil
}

// handleSignalKey drives the signal list. Which signal to send is a real
// choice — reload, stop, freeze, take out — so the prompt is a list to
// move through rather than a number to remember.
func (m Model) handleSignalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.signal = clamp(m.signal-1, 0, len(signals)-1)
	case "down", "j":
		m.signal = clamp(m.signal+1, 0, len(signals)-1)
	case "home", "g":
		m.signal = 0
	case "end", "G":
		m.signal = len(signals) - 1
	case "enter":
		m.mode = modeConfirm
	case "esc", "q", "x":
		m.mode, m.status = modeNormal, "kill cancelled"
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// handleConfirmKey answers the kill prompt: only an explicit y signs off.
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	chosen := signals[m.signal]
	if msg.String() != "y" && msg.String() != "Y" {
		m.status = "kill cancelled"
		return m, nil
	}
	if err := sendSignal(m.confirm.PID, chosen.number); err != nil {
		m.status = fmt.Sprintf("%s to %d: %v", chosen.name, m.confirm.PID, err)
		return m, nil
	}
	m.status = fmt.Sprintf("%s sent to %d", chosen.name, m.confirm.PID)
	return m, nil
}

// handleInfoKey holds the detail panel open until it is closed on purpose:
// the same i that opened it, or esc. Everything else is swallowed, because
// the table it would act on is not on screen.
func (m Model) handleInfoKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "i", "esc", "q":
		m.mode = modeNormal
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// sortBy switches column, or flips the direction when the column is already
// the active one. Re-sorting always shows the new top of the list.
func (m *Model) sortBy(key sortKey) {
	if m.sort == key {
		m.reverse = !m.reverse
	} else {
		m.sort, m.reverse = key, false
	}
	m.rebuild()
	m.jumpToTop()
}

// jumpToTop parks the cursor on the first row. The point of a re-sort is to
// see what is now at the top, so the view goes back to the first line
// instead of staying wherever the list was scrolled to.
func (m *Model) jumpToTop() {
	m.cursor, m.offset = 0, 0
}

// moveCursor is the reader taking the cursor somewhere, which is also the
// answer to "stop following that process": the two ways of choosing a row
// cannot both be in charge of it.
func (m *Model) moveCursor(delta int) {
	m.cursor += delta
	m.follow = 0
	m.clampView()
}

func (m Model) selected() (proc.Process, bool) {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return proc.Process{}, false
	}
	return m.rows[m.cursor].proc, true
}

// rebuild recomputes the visible rows after any change to the data or to
// the filter, sort and view settings.
func (m *Model) rebuild() {
	m.rows = buildRows(m.snap.Processes, m.sort, m.reverse, m.filter, m.tree)
	m.clampView()
	m.followCursor()
	if m.mode == modeInfo {
		m.refreshInfo()
	}
}

// followCursor drags the cursor back onto the process it is locked to. A
// process that is only missing from the table is still there to follow —
// the filter hides it, the tree may have it collapsed — but one that has
// left the machine releases the lock, because nothing will bring it back.
func (m *Model) followCursor() {
	if m.follow == 0 {
		return
	}
	for i, r := range m.rows {
		if r.proc.PID == m.follow {
			m.cursor = i
			m.clampView()
			return
		}
	}
	for _, p := range m.snap.Processes {
		if p.PID == m.follow {
			return
		}
	}
	m.status = fmt.Sprintf("%d exited, following nothing", m.follow)
	m.follow = 0
}

// refreshInfo re-reads the process the panel is pinned to, so its numbers
// keep ticking while the panel stays on the same pid. The filter and the
// sort do not apply here: the panel is about one process, whether or not
// the table still lists it. A pid that is gone has nothing left to show,
// so the panel closes and says so.
func (m *Model) refreshInfo() {
	for _, p := range m.snap.Processes {
		if p.PID == m.info.PID {
			m.info = p
			m.readDetails()
			return
		}
	}
	m.mode = modeNormal
	m.status = fmt.Sprintf("%d exited", m.info.PID)
}

// readDetails re-reads what the panel shows beyond the table's columns. It
// is a handful of files for a single process, so it is read inline rather
// than off the update loop, and it only touches collector state that never
// changes, so a refresh already in flight is none the wiser. A model built
// without a collector — every test but one — shows the panel without them.
func (m *Model) readDetails() {
	if m.collector == nil {
		m.details = proc.Details{Files: -1}
		return
	}
	m.details = m.collector.Details(m.info.PID)
}

// clampView keeps the cursor inside the list and the scroll window around
// the cursor. The cursor holds its line: a refresh only pulls it back when
// the list got short enough that the line no longer exists.
func (m *Model) clampView() {
	if len(m.rows) == 0 {
		m.cursor, m.offset = 0, 0
		return
	}
	height := m.tableHeight()
	m.cursor = clamp(m.cursor, 0, len(m.rows)-1)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+height {
		m.offset = m.cursor - height + 1
	}
	m.offset = clamp(m.offset, 0, max(0, len(m.rows)-height))
}

// tableHeight is the terminal minus the fixed header and footer.
func (m Model) tableHeight() int {
	return max(1, m.height-m.headerHeight()-footerHeight)
}

// Run starts the monitor and blocks until the user quits. The extra program
// options exist so tests can drive it with a fake terminal.
func Run(c *proc.Collector, opts Options, programOpts ...tea.ProgramOption) error {
	m, err := New(c, opts)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, programOpts...).Run()
	return err
}
