package ui

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/curruwilla/spy/internal/proc"
)

// sortKey is the column the process list is ordered by.
type sortKey int

const (
	sortCPU sortKey = iota
	sortMem
	sortPID
	sortName
	sortTime
)

// sortKeys lists every column in the order the tab key cycles them.
var sortKeys = []sortKey{sortCPU, sortMem, sortPID, sortName, sortTime}

func (k sortKey) String() string {
	switch k {
	case sortCPU:
		return "cpu"
	case sortMem:
		return "mem"
	case sortPID:
		return "pid"
	case sortName:
		return "name"
	default:
		return "time"
	}
}

// compare orders two processes by this column, largest first for the
// numeric ones because that is what a monitor is usually asked for.
func (k sortKey) compare(a, b proc.Process) int {
	switch k {
	case sortCPU:
		return cmp.Compare(b.CPU, a.CPU)
	case sortMem:
		return cmp.Compare(b.RSS, a.RSS)
	case sortPID:
		return cmp.Compare(a.PID, b.PID)
	case sortName:
		return strings.Compare(strings.ToLower(a.Command), strings.ToLower(b.Command))
	default:
		return cmp.Compare(b.CPUTime, a.CPUTime)
	}
}

// next returns the column after k, wrapping around.
func (k sortKey) next(step int) sortKey {
	i := (int(k) + step + len(sortKeys)) % len(sortKeys)
	return sortKeys[i]
}

// parseSortKey maps a command-line name to a column.
func parseSortKey(name string) (sortKey, error) {
	for _, k := range sortKeys {
		if k.String() == strings.ToLower(name) {
			return k, nil
		}
	}
	return sortCPU, fmt.Errorf("unknown sort column %q (want cpu, mem, pid, name or time)", name)
}

// descendingFirst reports whether the column starts on its most useful
// direction: biggest first for the measurements, smallest first for the
// identifiers.
func (k sortKey) descendingFirst() bool {
	return k == sortCPU || k == sortMem || k == sortTime
}
