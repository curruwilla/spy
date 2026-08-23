package proc

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Process is one entry of /proc/<pid>, already converted to display units.
type Process struct {
	PID     int
	PPID    int
	User    string
	State   string  // R running, S sleeping, D disk wait, Z zombie, T stopped
	CPU     float64 // percent of a single core, so it can exceed 100
	Mem     float64 // percent of total RAM
	RSS     uint64  // resident memory in bytes
	CPUTime time.Duration
	Threads int
	Command string
}

// readProcesses walks /proc and builds one Process per numeric entry.
// Processes that exit while being read are skipped, not reported as errors.
func (c *Collector) readProcesses(memTotal uint64, elapsed float64) ([]Process, error) {
	dir, err := os.Open(c.root)
	if err != nil {
		return nil, err
	}
	defer dir.Close()

	names, err := dir.Readdirnames(-1)
	if err != nil {
		return nil, err
	}

	procs := make([]Process, 0, len(names))
	busyTicks := make(map[int]uint64, len(names))
	for _, name := range names {
		pid, err := strconv.Atoi(name)
		if err != nil {
			continue // not a process directory
		}
		p, busy, err := c.readProcess(pid, memTotal)
		if err != nil {
			continue // vanished mid-read
		}
		busyTicks[pid] = busy
		if prev, ok := c.prevProc[pid]; ok && elapsed > 0 && busy >= prev {
			p.CPU = float64(busy-prev) / userHZ / elapsed * 100
		}
		procs = append(procs, p)
	}
	c.prevProc = busyTicks
	return procs, nil
}

// readProcess reads one pid. It returns the process and its cumulative busy
// ticks, which the caller diffs against the previous snapshot.
func (c *Collector) readProcess(pid int, memTotal uint64) (Process, uint64, error) {
	data, err := os.ReadFile(c.path(strconv.Itoa(pid), "stat"))
	if err != nil {
		return Process{}, 0, err
	}
	st, err := parseStat(data)
	if err != nil {
		return Process{}, 0, err
	}

	rss := st.rssPages * c.pageSize
	p := Process{
		PID:     pid,
		PPID:    st.ppid,
		User:    c.processUser(pid),
		State:   st.state,
		Mem:     percent(rss, memTotal),
		RSS:     rss,
		CPUTime: time.Duration(st.busyTicks()) * time.Second / userHZ,
		Threads: st.threads,
		Command: c.command(pid, st.comm),
	}
	return p, st.busyTicks(), nil
}

// processUser takes the owner from the directory itself, which is cheaper
// than parsing /proc/<pid>/status.
func (c *Collector) processUser(pid int) string {
	info, err := os.Stat(c.path(strconv.Itoa(pid)))
	if err != nil {
		return "?"
	}
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "?"
	}
	return c.userName(uint64(sys.Uid))
}

// command prefers the full command line; kernel threads have an empty one,
// so they fall back to the comm name in brackets, like ps does.
func (c *Collector) command(pid int, comm string) string {
	data, err := os.ReadFile(c.path(strconv.Itoa(pid), "cmdline"))
	if err != nil || len(bytes.Trim(data, "\x00")) == 0 {
		return "[" + comm + "]"
	}
	args := bytes.Split(bytes.TrimRight(data, "\x00"), []byte{0})
	return string(bytes.Join(args, []byte{' '}))
}

// stat is the subset of /proc/<pid>/stat fields the monitor uses.
type stat struct {
	comm     string
	state    string
	ppid     int
	utime    uint64
	stime    uint64
	threads  int
	rssPages uint64
}

func (s stat) busyTicks() uint64 { return s.utime + s.stime }

// Field offsets counted from the state field, which is the first one after
// the command name. See proc(5) for the full list.
const (
	statPPID     = 1
	statUtime    = 11
	statStime    = 12
	statThreads  = 17
	statRSSPages = 21
)

// parseStat splits /proc/<pid>/stat. The command name is parenthesised and
// may itself contain spaces and parens, so everything is measured from the
// last closing paren.
func parseStat(data []byte) (stat, error) {
	open := bytes.IndexByte(data, '(')
	end := bytes.LastIndexByte(data, ')')
	if open < 0 || end < open {
		return stat{}, fmt.Errorf("malformed stat line")
	}

	s := stat{comm: string(data[open+1 : end])}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) <= statRSSPages {
		return stat{}, fmt.Errorf("stat line has %d fields", len(fields))
	}

	s.state = fields[0]
	s.ppid, _ = strconv.Atoi(fields[statPPID])
	s.threads, _ = strconv.Atoi(fields[statThreads])
	s.utime, _ = strconv.ParseUint(fields[statUtime], 10, 64)
	s.stime, _ = strconv.ParseUint(fields[statStime], 10, 64)
	s.rssPages, _ = strconv.ParseUint(fields[statRSSPages], 10, 64)
	return s, nil
}
