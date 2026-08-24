package proc

import (
	"math"
	"os"
	"testing"
	"time"
)

const (
	fixtureRoot    = "testdata/proc"
	sysFixtureRoot = "testdata/sys"
)

func TestCollect(t *testing.T) {
	c := newCollector(fixtureRoot, sysFixtureRoot)
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
	// The traffic counters are cumulative like the cpu ones, so the first
	// snapshot has no rate to report either.
	if snap.Disk != (Throughput{}) || snap.Net != (Throughput{}) {
		t.Errorf("first snapshot disk = %+v net = %+v, want no traffic yet", snap.Disk, snap.Net)
	}
	if want := "Intel Core i7-8700K"; snap.CPU.Model != want {
		t.Errorf("CPU.Model = %q, want %q", snap.CPU.Model, want)
	}
	if snap.CPU.Temp != 52 {
		t.Errorf("CPU.Temp = %v, want 52", snap.CPU.Temp)
	}
}

// TestCollectThroughput feeds the collector a previous reading by hand,
// because the fixture files cannot change between two calls.
func TestCollectThroughput(t *testing.T) {
	c := newCollector(fixtureRoot, sysFixtureRoot)
	if _, err := c.Collect(); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	// Half of what the fixture holds, two seconds ago, so every rate is a
	// quarter of the fixture's own totals.
	c.prevDisk = ioCounters{in: 15000 * sectorSize, out: 23000 * sectorSize}
	c.prevNet = ioCounters{in: 3250, out: 1500}
	c.prevProc[1234] = sample{busy: 200, io: ioCounters{in: 4096, out: 2048}, ioOK: true}
	c.prevAt = time.Now().Add(-2 * time.Second)

	snap, err := c.Collect()
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if want := 7500.0 * sectorSize; !closeTo(snap.Disk.In, want) {
		t.Errorf("Disk.In = %v, want %v bytes a second", snap.Disk.In, want)
	}
	if want := 1625.0; !closeTo(snap.Net.In, want) {
		t.Errorf("Net.In = %v, want %v bytes a second", snap.Net.In, want)
	}

	byPID := map[int]Process{}
	for _, p := range snap.Processes {
		byPID[p.PID] = p
	}
	if got := byPID[1234]; !got.DiskKnown || !closeTo(got.Disk.In, 2048) || !closeTo(got.Disk.Out, 1024) {
		t.Errorf("pid 1234 disk = %+v known = %v, want 2048 read and 1024 written a second",
			got.Disk, got.DiskKnown)
	}
	// The kernel thread has no io file, which is not the same as no
	// traffic: the table has to be able to say so.
	if got := byPID[7]; got.DiskKnown {
		t.Errorf("pid 7 disk reported as known, want it marked unreadable")
	}
}

// closeTo compares two rates. The interval a rate is divided by is a real
// clock reading, so the result is never exactly the round number the
// counters were chosen to produce.
func closeTo(got, want float64) bool { return math.Abs(got-want) < want/1000 }

func TestCollectProcessFields(t *testing.T) {
	c := newCollector(fixtureRoot, sysFixtureRoot)
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
	// They hang off kthreadd, which is how they are told apart from
	// anything a user started.
	if !byPID[7].Kernel {
		t.Errorf("pid 7 is a child of kthreadd, want it marked as a kernel thread")
	}
	if byPID[1234].Kernel {
		t.Errorf("pid 1234 has a command line of its own, want it not marked as a kernel thread")
	}
}

// TestCollectCPUDelta feeds the collector two readings by hand, because the
// fixture files cannot change between calls.
func TestCollectCPUDelta(t *testing.T) {
	c := newCollector(fixtureRoot, sysFixtureRoot)
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
