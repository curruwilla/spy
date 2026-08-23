package ui

import (
	"fmt"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/curruwilla/spy/internal/proc"
)

// mode is what the screen is currently doing: showing the table, reading a
// filter or a set of thresholds, asking to confirm a kill, or holding the
// detail panel open over the table.
type mode int

const (
	modeNormal mode = iota
	modeFilter
	modeThreshold
	modeConfirm
	modeInfo
)

// Options configures the monitor from the command line.
type Options struct {
	Interval time.Duration
	Sort     string
	Tree     bool
	Filter   string
	Min      string // threshold clauses, e.g. "cpu>5 mem>500M time>1m"
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

	info    proc.Process // process the detail panel describes, fixed when it opened
	confirm proc.Process // process awaiting a kill confirmation
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
			m.rebuild()
		}
		return m, tick(m.interval)

	case tickMsg:
		return m, m.collect()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeFilter:
		return m.handleFilterKey(msg)
	case modeThreshold:
		return m.handleThresholdKey(msg)
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
	case "tab":
		m.sortBy(m.sort.next(1))
	case "shift+tab":
		m.sortBy(m.sort.next(-1))

	case "t":
		m.tree = !m.tree
		m.rebuild()
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
		}
	case "x":
		if p, ok := m.selected(); ok {
			m.confirm, m.mode = p, modeConfirm
		}
	}
	return m, nil
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

// handleConfirmKey answers the kill prompt: only an explicit y signs off.
func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.mode = modeNormal
	if msg.String() != "y" && msg.String() != "Y" {
		m.status = "kill cancelled"
		return m, nil
	}
	if err := syscall.Kill(m.confirm.PID, syscall.SIGTERM); err != nil {
		m.status = fmt.Sprintf("kill %d: %v", m.confirm.PID, err)
		return m, nil
	}
	m.status = fmt.Sprintf("SIGTERM sent to %d", m.confirm.PID)
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

func (m *Model) moveCursor(delta int) {
	m.cursor += delta
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
	if m.mode == modeInfo {
		m.refreshInfo()
	}
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
			return
		}
	}
	m.mode = modeNormal
	m.status = fmt.Sprintf("%d exited", m.info.PID)
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
	return max(1, m.height-headerHeight-footerHeight)
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
