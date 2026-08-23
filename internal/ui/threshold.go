package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/curruwilla/spy/internal/proc"
)

// thresholds is a floor under each measurement: anything below it is not
// worth looking at and never reaches the table. A zero field means that
// measurement is not filtered, so the zero value keeps every process.
type thresholds struct {
	CPU  float64       // percent of one core
	Mem  float64       // percent of total RAM
	RSS  uint64        // resident bytes, the alternative way to say "memory"
	Time time.Duration // accumulated CPU time
}

// keeps reports whether a process reaches every active floor. The
// comparison is "at least", which is also why the zero value lets
// everything through.
func (t thresholds) keeps(p proc.Process) bool {
	return p.CPU >= t.CPU && p.Mem >= t.Mem && p.RSS >= t.RSS && p.CPUTime >= t.Time
}

// String renders the active floors in the same syntax parseThresholds
// reads, so the footer echoes something the user can retype.
func (t thresholds) String() string {
	var parts []string
	if t.CPU > 0 {
		parts = append(parts, fmt.Sprintf("cpu>%g%%", t.CPU))
	}
	if t.Mem > 0 {
		parts = append(parts, fmt.Sprintf("mem>%g%%", t.Mem))
	}
	if t.RSS > 0 {
		parts = append(parts, "mem>"+formatBytes(t.RSS))
	}
	if t.Time > 0 {
		parts = append(parts, "time>"+t.Time.String())
	}
	return strings.Join(parts, " ")
}

// parseThresholds reads a whitespace separated list of clauses, such as
// "cpu>5 mem>500M time>1m". The comparison sign is decoration: "cpu 5",
// "cpu>5" and "cpu>=5" all mean at least five. An empty string filters
// nothing.
func parseThresholds(s string) (thresholds, error) {
	var t thresholds
	tokens := splitClauses(s)
	for i := 0; i < len(tokens); i += 2 {
		if i+1 == len(tokens) {
			return thresholds{}, fmt.Errorf("threshold %q needs a value", tokens[i])
		}
		if err := t.set(tokens[i], tokens[i+1]); err != nil {
			return thresholds{}, err
		}
	}
	return t, nil
}

// splitClauses breaks "cpu>5 mem 10%" into names and values, dropping the
// separators: ["cpu", "5", "mem", "10%"].
func splitClauses(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '>' || r == '=' || r == ':' || r == ','
	})
}

// set applies one name/value clause.
func (t *thresholds) set(name, value string) error {
	switch strings.ToLower(name) {
	case "cpu":
		pct, err := parsePercent(value)
		if err != nil {
			return fmt.Errorf("cpu threshold: %w", err)
		}
		t.CPU = pct
	case "mem", "ram":
		return t.setMem(value)
	case "time":
		d, err := parseCPUTime(value)
		if err != nil {
			return fmt.Errorf("time threshold: %w", err)
		}
		t.Time = d
	default:
		return fmt.Errorf("unknown threshold %q (want cpu, mem or time)", name)
	}
	return nil
}

// setMem takes either a share of the total RAM ("10", "10%") or a resident
// size ("500M", "1.5G"). They are two ways of saying the same thing, so the
// newest one replaces the other.
func (t *thresholds) setMem(value string) error {
	if size, isSize, err := parseSize(value); isSize {
		if err != nil {
			return fmt.Errorf("mem threshold: %w", err)
		}
		t.Mem, t.RSS = 0, size
		return nil
	}
	pct, err := parsePercent(value)
	if err != nil {
		return fmt.Errorf("mem threshold: %w", err)
	}
	t.Mem, t.RSS = pct, 0
	return nil
}

// parsePercent reads a percentage, with or without its sign.
func parsePercent(value string) (float64, error) {
	pct, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
	switch {
	case err != nil:
		return 0, fmt.Errorf("%q is not a number", value)
	case pct < 0:
		return 0, fmt.Errorf("%q is negative", value)
	}
	return pct, nil
}

// parseSize reads a byte size such as 500M, 1.5G, 900k or 4096B. The second
// result reports whether the value looked like a size at all, so a plain
// number can fall through to being read as a percentage.
func parseSize(value string) (uint64, bool, error) {
	const units = "KMGTP"

	s := value
	hasB := strings.HasSuffix(s, "b") || strings.HasSuffix(s, "B")
	if hasB {
		s = s[:len(s)-1]
	}
	if s == "" {
		return 0, false, nil
	}

	scale := float64(1)
	if i := strings.IndexRune(units, unicode.ToUpper(rune(s[len(s)-1]))); i >= 0 {
		scale = float64(uint64(1) << (10 * (i + 1)))
		s = s[:len(s)-1]
	} else if !hasB {
		return 0, false, nil // no unit at all: this is a percentage
	}

	size, err := strconv.ParseFloat(s, 64)
	switch {
	case err != nil:
		return 0, true, fmt.Errorf("%q is not a size", value)
	case size < 0:
		return 0, true, fmt.Errorf("%q is negative", value)
	}
	return uint64(size * scale), true, nil
}

// parseCPUTime reads accumulated CPU time. A bare number is seconds, so
// "90" and "1m30s" are the same floor.
func parseCPUTime(value string) (time.Duration, error) {
	if secs, err := strconv.ParseFloat(value, 64); err == nil {
		if secs < 0 {
			return 0, fmt.Errorf("%q is negative", value)
		}
		return time.Duration(secs * float64(time.Second)), nil
	}
	d, err := time.ParseDuration(value)
	switch {
	case err != nil:
		return 0, fmt.Errorf("%q is not a duration (want 90, 90s, 1m30s or 2h)", value)
	case d < 0:
		return 0, fmt.Errorf("%q is negative", value)
	}
	return d, nil
}
