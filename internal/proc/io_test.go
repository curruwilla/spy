package proc

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadDiskStats(t *testing.T) {
	got, err := readDiskStats(fixtureRoot + "/diskstats")
	if err != nil {
		t.Fatalf("readDiskStats: %v", err)
	}
	// Only sda and nvme0n1 count: their partitions are the same bytes
	// again, and loop0 and dm-0 are not disks.
	want := ioCounters{in: (20000 + 10000) * sectorSize, out: (40000 + 6000) * sectorSize}
	if got != want {
		t.Errorf("readDiskStats = %+v, want %+v", got, want)
	}
}

func TestReadDiskStatsMissingFile(t *testing.T) {
	if _, err := readDiskStats(fixtureRoot + "/no-such-file"); err == nil {
		t.Error("want an error for a file that is not there")
	}
}

func TestIsPartition(t *testing.T) {
	devices := map[string]ioCounters{
		"sda": {}, "sda1": {}, "sda12": {},
		"nvme0n1": {}, "nvme0n1p3": {},
		"mmcblk0": {}, "mmcblk0p1": {},
		"vdb": {}, "sdb3": {},
	}
	cases := map[string]bool{
		"sda":       false,
		"sda1":      true,
		"sda12":     true,
		"nvme0n1":   false, // the trailing 1 is part of the name, not a partition
		"nvme0n1p3": true,
		"mmcblk0":   false,
		"mmcblk0p1": true,
		"vdb":       false,
		"sdb3":      false, // its disk is not in the file, so nothing counts it twice
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isPartition(name, devices); got != want {
				t.Errorf("isPartition(%q) = %v, want %v", name, got, want)
			}
		})
	}
}

// TestReadDiskStatsSkipsWhatItCannotRead covers the lines a kernel or a
// container may put there that this parser has no use for: they are
// stepped over rather than taken for zeroes.
func TestReadDiskStatsSkipsWhatItCannotRead(t *testing.T) {
	path := writeTemp(t, "diskstats", strings.Join([]string{
		"   8       0 sda 1000 0 20000 500 2000 0 40000 900 0 1000 1400",
		"   8       1", // truncated mid-line
		"   8       2 sdb x 0 nonsense 500 2000 0 40000 900 0 1000 1400",
		"",
	}, "\n"))

	got, err := readDiskStats(path)
	if err != nil {
		t.Fatalf("readDiskStats: %v", err)
	}
	if want := (ioCounters{in: 20000 * sectorSize, out: 40000 * sectorSize}); got != want {
		t.Errorf("readDiskStats = %+v, want only the line it could read (%+v)", got, want)
	}
}

func TestReadNetDevSkipsWhatItCannotRead(t *testing.T) {
	path := writeTemp(t, "dev", strings.Join([]string{
		"Inter-|   Receive                    |  Transmit",
		" face |bytes packets errs drop fifo frame compressed multicast|bytes packets",
		"  eth0: 5000 50 0 0 0 0 0 0 2500 25 0 0 0 0 0 0",
		"  eth1: 1 2 3", // too few fields to hold a transmit count
		"  eth2: x 50 0 0 0 0 0 0 y 25 0 0 0 0 0 0",
		"",
	}, "\n"))

	got, err := readNetDev(path)
	if err != nil {
		t.Fatalf("readNetDev: %v", err)
	}
	if want := (ioCounters{in: 5000, out: 2500}); got != want {
		t.Errorf("readNetDev = %+v, want only the interface it could read (%+v)", got, want)
	}
}

func TestReadNetDev(t *testing.T) {
	got, err := readNetDev(fixtureRoot + "/net/dev")
	if err != nil {
		t.Fatalf("readNetDev: %v", err)
	}
	// eth0 and wlan0 only: the loopback talks to itself, and the bridge
	// and its veth carry what eth0 already carried.
	want := ioCounters{in: 5000 + 1500, out: 2500 + 500}
	if got != want {
		t.Errorf("readNetDev = %+v, want %+v", got, want)
	}
}

func TestReadProcIO(t *testing.T) {
	got, ok := readProcIO(fixtureRoot + "/1234/io")
	if !ok {
		t.Fatal("readProcIO refused a readable file")
	}
	if want := (ioCounters{in: 8192, out: 4096}); got != want {
		t.Errorf("readProcIO = %+v, want %+v", got, want)
	}

	// A process of somebody else's refuses the file, which is not the same
	// answer as no traffic.
	if _, ok := readProcIO(fixtureRoot + "/7/io"); ok {
		t.Error("readProcIO reported an unreadable file as known")
	}
}

// TestReadProcIOWithoutTheFields covers a kernel built without the io
// accounting: the file is there and readable, and says nothing.
func TestReadProcIOWithoutTheFields(t *testing.T) {
	path := writeTemp(t, "io", "rchar: 100\nwchar: not a number\n")
	got, ok := readProcIO(path)
	if !ok {
		t.Fatal("a readable file should be reported as known")
	}
	if got != (ioCounters{}) {
		t.Errorf("readProcIO = %+v, want zeroes", got)
	}
}

// writeTemp puts one made-up /proc file on disk, for the shapes the
// fixture tree has no room to hold.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestThroughputRate(t *testing.T) {
	cases := []struct {
		name    string
		prev    ioCounters
		cur     ioCounters
		elapsed float64
		want    Throughput
	}{
		{
			name:    "bytes over seconds",
			prev:    ioCounters{in: 1000, out: 500},
			cur:     ioCounters{in: 3000, out: 1500},
			elapsed: 2,
			want:    Throughput{In: 1000, Out: 500},
		},
		{
			name:    "no previous reading",
			cur:     ioCounters{in: 3000, out: 1500},
			elapsed: 0,
			want:    Throughput{},
		},
		{
			name:    "counter reset",
			prev:    ioCounters{in: 9000, out: 9000},
			cur:     ioCounters{in: 10, out: 20},
			elapsed: 1,
			want:    Throughput{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.cur.rate(c.prev, c.elapsed)
			if math.Abs(got.In-c.want.In) > 1e-9 || math.Abs(got.Out-c.want.Out) > 1e-9 {
				t.Errorf("rate = %+v, want %+v", got, c.want)
			}
			if want := c.want.In + c.want.Out; math.Abs(got.Total()-want) > 1e-9 {
				t.Errorf("Total = %v, want %v", got.Total(), want)
			}
		})
	}
}
