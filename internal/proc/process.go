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
	VSize   uint64  // address space in bytes, almost always far above RSS
	CPUTime time.Duration
	Uptime  time.Duration // wall time since the process started
	Nice    int
	Threads int
	Command string
	Kernel  bool // a kernel thread: no command line of its own, no user code
}

// kthreaddPID is the kernel thread daemon, which every kernel thread hangs
// off. It is pid 2 on every Linux build.
const kthreaddPID = 2

// readProcesses walks /proc and builds one Process per numeric entry.
// Processes that exit while being read are skipped, not reported as errors.
// uptime is the machine's, and is what process start times are measured
// against.
func (c *Collector) readProcesses(memTotal uint64, uptime time.Duration, elapsed float64) ([]Process, error) {
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
		p, busy, err := c.readProcess(pid, memTotal, uptime)
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
func (c *Collector) readProcess(pid int, memTotal uint64, uptime time.Duration) (Process, uint64, error) {
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
		VSize:   st.vsize,
		CPUTime: time.Duration(st.busyTicks()) * time.Second / userHZ,
		Uptime:  processUptime(uptime, st.startTicks),
		Nice:    st.nice,
		Threads: st.threads,
		Command: c.command(pid, st.comm),
		Kernel:  pid == kthreaddPID || st.ppid == kthreaddPID,
	}
	return p, st.busyTicks(), nil
}

// processUptime turns a start time counted in ticks since boot into how
// long the process has been running. A process that claims to have started
// after the boot clock reads zero rather than a negative age.
func processUptime(uptime time.Duration, startTicks uint64) time.Duration {
	age := uptime - time.Duration(startTicks)*time.Second/userHZ
	if age < 0 {
		return 0
	}
	return age
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
	comm       string
	state      string
	ppid       int
	utime      uint64
	stime      uint64
	nice       int
	threads    int
	startTicks uint64
	vsize      uint64
	rssPages   uint64
}

func (s stat) busyTicks() uint64 { return s.utime + s.stime }

// Field offsets counted from the state field, which is the first one after
// the command name. See proc(5) for the full list.
const (
	statPPID       = 1
	statUtime      = 11
	statStime      = 12
	statNice       = 16
	statThreads    = 17
	statStartTicks = 19
	statVSize      = 20
	statRSSPages   = 21
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
	s.nice, _ = strconv.Atoi(fields[statNice])
	s.threads, _ = strconv.Atoi(fields[statThreads])
	s.utime, _ = strconv.ParseUint(fields[statUtime], 10, 64)
	s.stime, _ = strconv.ParseUint(fields[statStime], 10, 64)
	s.startTicks, _ = strconv.ParseUint(fields[statStartTicks], 10, 64)
	s.vsize, _ = strconv.ParseUint(fields[statVSize], 10, 64)
	s.rssPages, _ = strconv.ParseUint(fields[statRSSPages], 10, 64)
	return s, nil
}
