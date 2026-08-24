package proc

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Throughput is a pair of byte rates measured over the interval between
// two snapshots. In is what came in — read from a disk, received from the
// network — and Out is what went the other way.
type Throughput struct {
	In  float64
	Out float64
}

// Total is both directions at once, which is what "how busy is this"
// usually means.
func (t Throughput) Total() float64 { return t.In + t.Out }

// ioCounters is the cumulative pair a Throughput is the difference of.
type ioCounters struct {
	in  uint64
	out uint64
}

// rate turns the movement since the previous reading into bytes per
// second. Without a previous reading, or without time between the two,
// there is no rate to report yet.
func (c ioCounters) rate(prev ioCounters, elapsed float64) Throughput {
	if elapsed <= 0 {
		return Throughput{}
	}
	return Throughput{
		In:  moved(prev.in, c.in) / elapsed,
		Out: moved(prev.out, c.out) / elapsed,
	}
}

// moved is how far a cumulative counter travelled. One that went backwards
// was reset — a device unplugged, a pid reused by another process — and
// counts as no movement rather than as a number the size of the counter.
func moved(prev, cur uint64) float64 {
	if cur < prev {
		return 0
	}
	return float64(cur - prev)
}

// sectorSize is the unit /proc/diskstats counts in, whatever block size
// the device itself uses. See Documentation/admin-guide/iostats.rst.
const sectorSize = 512

// Fields of a /proc/diskstats line: major, minor, name, then the read and
// write counters.
const (
	statsName           = 2
	statsSectorsRead    = 5
	statsSectorsWritten = 9
)

// virtualDisks are the /proc/diskstats entries that are not a disk, or are
// another view of one that is already counted: a partition's parent, the
// members under a software raid, the disk under a device mapper.
var virtualDisks = []string{"loop", "ram", "zram", "dm-", "md", "sr", "fd"}

// readDiskStats sums the traffic of every whole disk on the machine.
// Partitions and the layers stacked on top of a disk are left out, because
// their counters are the same bytes over again.
func readDiskStats(path string) (ioCounters, error) {
	f, err := os.Open(path)
	if err != nil {
		return ioCounters{}, err
	}
	defer f.Close()

	// Sectors for now: whether a device is a partition of another one can
	// only be told once every name in the file has been seen.
	devices := map[string]ioCounters{}
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) <= statsSectorsWritten {
			continue
		}
		name := fields[statsName]
		if hasAnyPrefix(name, virtualDisks) {
			continue
		}
		read, err1 := strconv.ParseUint(fields[statsSectorsRead], 10, 64)
		written, err2 := strconv.ParseUint(fields[statsSectorsWritten], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		devices[name] = ioCounters{in: read, out: written}
	}
	if err := scan.Err(); err != nil {
		return ioCounters{}, err
	}

	var total ioCounters
	for name, sectors := range devices {
		if isPartition(name, devices) {
			continue
		}
		total.in += sectors.in * sectorSize
		total.out += sectors.out * sectorSize
	}
	return total, nil
}

// isPartition reports whether name is a slice of another device in the
// same file, whose counters already include it: sda1 of sda, and the
// nvme and mmc spelling, nvme0n1p3 of nvme0n1.
func isPartition(name string, devices map[string]ioCounters) bool {
	base := strings.TrimRight(name, "0123456789")
	if base == name {
		return false // nothing trailing to be a partition number
	}
	if _, ok := devices[base]; ok {
		return true
	}
	if stem, cut := strings.CutSuffix(base, "p"); cut {
		_, ok := devices[stem]
		return ok
	}
	return false
}

// Fields of a /proc/net/dev line, counted from the colon after the
// interface name.
const (
	netReceiveBytes  = 0
	netTransmitBytes = 8
)

// virtualLinks are the interfaces whose bytes would be counted twice: the
// loopback talks only to itself, and the container and virtual machine
// ends of a bridge carry what a real interface has already carried.
var virtualLinks = []string{"lo", "veth", "docker", "br-", "virbr", "tap", "bond"}

// readNetDev sums the traffic of every real interface on the machine.
func readNetDev(path string) (ioCounters, error) {
	f, err := os.Open(path)
	if err != nil {
		return ioCounters{}, err
	}
	defer f.Close()

	var total ioCounters
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		// The two header lines carry no colon, so they drop out here.
		name, values, ok := strings.Cut(scan.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" || hasAnyPrefix(name, virtualLinks) {
			continue
		}
		fields := strings.Fields(values)
		if len(fields) <= netTransmitBytes {
			continue
		}
		received, err1 := strconv.ParseUint(fields[netReceiveBytes], 10, 64)
		sent, err2 := strconv.ParseUint(fields[netTransmitBytes], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		total.in += received
		total.out += sent
	}
	if err := scan.Err(); err != nil {
		return ioCounters{}, err
	}
	return total, nil
}

// Fields of /proc/<pid>/io. read_bytes and write_bytes are what actually
// reached the block layer, unlike rchar and wchar, which count everything
// that went through a read or a write syscall, cache hits included.
const (
	ioReadBytes  = "read_bytes"
	ioWriteBytes = "write_bytes"
)

// readProcIO reads one process's disk counters. They need privileges the
// reader only has over its own processes, so a refusal is reported as
// "unknown" rather than as zero traffic.
func readProcIO(path string) (ioCounters, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ioCounters{}, false
	}
	var c ioCounters
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case ioReadBytes:
			c.in = n
		case ioWriteBytes:
			c.out = n
		}
	}
	return c, true
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
