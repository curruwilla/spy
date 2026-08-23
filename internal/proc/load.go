package proc

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Load is /proc/loadavg: the three run-queue averages plus the process
// counts that follow them.
type Load struct {
	One     float64
	Five    float64
	Fifteen float64
	Running int
	Total   int
}

func readLoad(path string) (Load, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Load{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) < 4 {
		return Load{}, fmt.Errorf("malformed loadavg: %q", data)
	}

	var l Load
	for i, dst := range []*float64{&l.One, &l.Five, &l.Fifteen} {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return Load{}, err
		}
		*dst = v
	}
	// The fourth field is "running/total".
	running, total, _ := strings.Cut(fields[3], "/")
	l.Running, _ = strconv.Atoi(running)
	l.Total, _ = strconv.Atoi(total)
	return l, nil
}

// readUptime parses the first field of /proc/uptime, seconds since boot.
func readUptime(path string) (time.Duration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("malformed uptime: %q", data)
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, err
	}
	return time.Duration(secs * float64(time.Second)), nil
}
