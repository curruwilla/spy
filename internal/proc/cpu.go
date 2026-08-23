package proc

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// cpuTimes is one "cpu" line of /proc/stat, reduced to the two numbers a
// usage percentage needs.
type cpuTimes struct {
	total uint64 // every tick the CPU accounted for
	busy  uint64 // total minus idle and iowait
}

// readCPUTimes parses the aggregate line and the per-core lines of
// /proc/stat, in file order: index 0 is the machine, 1..n are the cores.
func readCPUTimes(path string) ([]cpuTimes, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var times []cpuTimes
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			// The cpu lines come first, so the first miss ends them.
			if len(times) > 0 {
				break
			}
			continue
		}
		t, err := parseCPUTimes(fields[1:])
		if err != nil {
			return nil, fmt.Errorf("parse %q: %w", fields[0], err)
		}
		times = append(times, t)
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	if len(times) == 0 {
		return nil, fmt.Errorf("no cpu line in %s", path)
	}
	return times, nil
}

// parseCPUTimes sums the tick counters. Fields are, in order: user, nice,
// system, idle, iowait, irq, softirq, steal, guest, guest_nice.
func parseCPUTimes(fields []string) (cpuTimes, error) {
	var t cpuTimes
	for i, f := range fields {
		v, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return cpuTimes{}, err
		}
		// guest and guest_nice are already counted inside user and nice.
		if i >= 8 {
			continue
		}
		t.total += v
		if i != 3 && i != 4 { // idle, iowait
			t.busy += v
		}
	}
	return t, nil
}

// cpuUsage turns two readings into percentages. The first reading has no
// predecessor and reports zero.
func cpuUsage(prev, cur []cpuTimes) CPU {
	usage := make([]float64, len(cur))
	for i := range cur {
		if i < len(prev) {
			usage[i] = percentBusy(prev[i], cur[i])
		}
	}
	return CPU{Total: usage[0], Cores: usage[1:]}
}

func percentBusy(prev, cur cpuTimes) float64 {
	total := cur.total - prev.total
	if cur.total < prev.total || total == 0 {
		return 0
	}
	return float64(cur.busy-prev.busy) / float64(total) * 100
}
