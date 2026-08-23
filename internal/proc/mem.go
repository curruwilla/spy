package proc

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// Memory holds RAM and swap figures in bytes.
type Memory struct {
	Total     uint64
	Available uint64
	Free      uint64
	Buffers   uint64
	Cached    uint64
	SwapTotal uint64
	SwapFree  uint64
}

// Used is what applications actually hold: everything the kernel could not
// hand back on demand.
func (m Memory) Used() uint64 { return m.Total - m.Available }

// UsedPercent is Used over Total, 0 when the total is unknown.
func (m Memory) UsedPercent() float64 { return percent(m.Used(), m.Total) }

func (m Memory) SwapUsed() uint64 { return m.SwapTotal - m.SwapFree }

func (m Memory) SwapPercent() float64 { return percent(m.SwapUsed(), m.SwapTotal) }

func percent(part, whole uint64) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole) * 100
}

// readMemory parses the handful of /proc/meminfo keys the display needs.
// Values there are in kB.
func readMemory(path string) (Memory, error) {
	f, err := os.Open(path)
	if err != nil {
		return Memory{}, err
	}
	defer f.Close()

	fields := map[string]*uint64{}
	var m Memory
	fields["MemTotal"] = &m.Total
	fields["MemAvailable"] = &m.Available
	fields["MemFree"] = &m.Free
	fields["Buffers"] = &m.Buffers
	fields["Cached"] = &m.Cached
	fields["SwapTotal"] = &m.SwapTotal
	fields["SwapFree"] = &m.SwapFree

	scan := bufio.NewScanner(f)
	for scan.Scan() {
		key, value, ok := strings.Cut(scan.Text(), ":")
		if !ok {
			continue
		}
		dst, wanted := fields[key]
		if !wanted {
			continue
		}
		kb, err := strconv.ParseUint(strings.Fields(value)[0], 10, 64)
		if err != nil {
			continue
		}
		*dst = kb * 1024
	}
	if err := scan.Err(); err != nil {
		return Memory{}, err
	}
	// Kernels older than 3.14 have no MemAvailable; approximate it.
	if m.Available == 0 {
		m.Available = m.Free + m.Buffers + m.Cached
	}
	return m, nil
}
