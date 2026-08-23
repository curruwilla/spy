// Package proc reads system and process metrics from the Linux /proc
// filesystem. Counters in /proc are cumulative, so a Collector keeps the
// previous reading and turns the difference into percentages.
package proc

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"time"
)

// defaultRoot is where procfs is mounted. Tests point the collector at a
// fixture directory instead.
const defaultRoot = "/proc"

// userHZ is the clock tick rate the kernel uses for the CPU counters in
// /proc (getconf CLK_TCK). It is 100 on every mainstream Linux build.
const userHZ = 100

// Snapshot is one complete reading of the machine.
type Snapshot struct {
	At        time.Time
	Uptime    time.Duration
	CPU       CPU
	Memory    Memory
	Load      Load
	Processes []Process
}

// CPU holds usage percentages for the whole machine and for each core.
type CPU struct {
	Total float64
	Cores []float64
}

// Collector produces snapshots. It is not safe for concurrent use: call
// Collect from a single goroutine.
type Collector struct {
	root     string
	pageSize uint64

	prevCPU  []cpuTimes     // index 0 is the machine, 1..n the cores
	prevProc map[int]uint64 // pid -> busy ticks at the previous collect
	prevAt   time.Time

	users map[uint64]string // uid -> name, cached because lookups hit NSS
}

// NewCollector returns a collector reading the live /proc filesystem.
func NewCollector() *Collector { return newCollector(defaultRoot) }

func newCollector(root string) *Collector {
	return &Collector{
		root:     root,
		pageSize: uint64(os.Getpagesize()),
		prevProc: make(map[int]uint64),
		users:    make(map[uint64]string),
	}
}

// Collect reads the whole system state once. Percentages are measured
// against the previous call, so the first snapshot reports zero CPU usage.
func (c *Collector) Collect() (Snapshot, error) {
	now := time.Now()
	elapsed := now.Sub(c.prevAt).Seconds()
	if c.prevAt.IsZero() {
		elapsed = 0
	}

	cpuTimes, err := readCPUTimes(c.path("stat"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read cpu times: %w", err)
	}
	mem, err := readMemory(c.path("meminfo"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read memory: %w", err)
	}
	load, err := readLoad(c.path("loadavg"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read loadavg: %w", err)
	}
	uptime, err := readUptime(c.path("uptime"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("read uptime: %w", err)
	}
	procs, err := c.readProcesses(mem.Total, uptime, elapsed)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read processes: %w", err)
	}

	snap := Snapshot{
		At:        now,
		Uptime:    uptime,
		CPU:       cpuUsage(c.prevCPU, cpuTimes),
		Memory:    mem,
		Load:      load,
		Processes: procs,
	}
	c.prevCPU = cpuTimes
	c.prevAt = now
	return snap, nil
}

func (c *Collector) path(elem ...string) string {
	p := c.root
	for _, e := range elem {
		p += "/" + e
	}
	return p
}

// userName resolves a uid to a login name, falling back to the numeric id
// for uids with no passwd entry (containers, deleted users).
func (c *Collector) userName(uid uint64) string {
	if name, ok := c.users[uid]; ok {
		return name
	}
	name := strconv.FormatUint(uid, 10)
	if u, err := user.LookupId(name); err == nil {
		name = u.Username
	}
	c.users[uid] = name
	return name
}
