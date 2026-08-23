package proc

import (
	"os"
	"testing"
	"time"
)

const fixtureRoot = "testdata/proc"

func TestCollect(t *testing.T) {
	c := newCollector(fixtureRoot)
	snap, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// The first snapshot has nothing to diff against, so CPU reads zero.
	if snap.CPU.Total != 0 {
		t.Errorf("first snapshot CPU.Total = %v, want 0", snap.CPU.Total)
	}
	if len(snap.CPU.Cores) != 2 {
		t.Errorf("cores = %d, want 2", len(snap.CPU.Cores))
	}
	if snap.Uptime != time.Duration(141785.42*float64(time.Second)) {
		t.Errorf("uptime = %s", snap.Uptime)
	}
	if snap.Load.One != 1.80 || snap.Load.Running != 3 || snap.Load.Total != 1234 {
		t.Errorf("load = %+v", snap.Load)
	}
	if len(snap.Processes) != 2 {
		t.Fatalf("processes = %d, want 2", len(snap.Processes))
	}
}

func TestCollectProcessFields(t *testing.T) {
	c := newCollector(fixtureRoot)
	snap, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	byPID := map[int]Process{}
	for _, p := range snap.Processes {
		byPID[p.PID] = p
	}

	user := byPID[1234]
	if user.PPID != 7 || user.State != "S" || user.Threads != 4 || user.Nice != 0 {
		t.Errorf("pid 1234 = %+v", user)
	}
	if user.VSize != 123456789 {
		t.Errorf("pid 1234 VSize = %d, want 123456789", user.VSize)
	}
	// Started 900 ticks after boot, so it is that much younger than the
	// machine.
	if want := snap.Uptime - 9*time.Second; user.Uptime != want {
		t.Errorf("pid 1234 Uptime = %s, want %s", user.Uptime, want)
	}
	if want := 4 * time.Second; user.CPUTime != want {
		t.Errorf("pid 1234 CPUTime = %s, want %s", user.CPUTime, want)
	}
	if want := "/usr/bin/myprog --flag value"; user.Command != want {
		t.Errorf("pid 1234 Command = %q, want %q", user.Command, want)
	}
	if user.RSS != 2048*uint64(os.Getpagesize()) {
		t.Errorf("pid 1234 RSS = %d", user.RSS)
	}

	// Kernel threads have an empty cmdline and fall back to comm.
	if want := "[kworker/0:1]"; byPID[7].Command != want {
		t.Errorf("pid 7 Command = %q, want %q", byPID[7].Command, want)
	}
}

// TestCollectCPUDelta feeds the collector two readings by hand, because the
// fixture files cannot change between calls.
func TestCollectCPUDelta(t *testing.T) {
	c := newCollector(fixtureRoot)
	if _, err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// Pretend the previous reading was half as far along: 500 total ticks,
	// 85 of them busy, so the second reading is also 17% busy.
	c.prevCPU = []cpuTimes{{total: 500, busy: 85}, {total: 250, busy: 42}, {total: 250, busy: 43}}
	c.prevAt = time.Now().Add(-time.Second)

	snap, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if got, want := snap.CPU.Total, 17.0; got != want {
		t.Errorf("CPU.Total = %v, want %v", got, want)
	}
}
