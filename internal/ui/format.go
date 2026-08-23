package ui

import (
	"fmt"
	"strings"
	"time"
)

// formatBytes renders a byte count in the shortest readable form. Large
// units keep one decimal, because the difference between 15.7G and 16G of
// RAM is worth seeing.
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	value := float64(b)
	for i, suffix := range []string{"K", "M", "G", "T", "P"} {
		value /= unit
		if value >= unit {
			continue
		}
		if value < 10 || i >= 2 { // G and up keep a decimal
			return fmt.Sprintf("%.1f%s", value, suffix)
		}
		return fmt.Sprintf("%.0f%s", value, suffix)
	}
	return fmt.Sprintf("%.0fP", value)
}

// formatCPUTime renders accumulated CPU time as hh:mm:ss, dropping the hours
// while they are zero.
func formatCPUTime(d time.Duration) string {
	total := int(d.Seconds())
	h, m, s := total/3600, total/60%60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// formatUptime renders how long the machine has been up, coarsest unit first.
func formatUptime(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

// pad fits s into exactly w cells, truncating with an ellipsis when it does
// not fit. Alignment is left unless right is set.
func pad(s string, w int, right bool) string {
	if w <= 0 {
		return ""
	}
	runes := []rune(s)
	switch {
	case len(runes) > w:
		if w == 1 {
			return "…"
		}
		return string(runes[:w-1]) + "…"
	case right:
		return strings.Repeat(" ", w-len(runes)) + s
	default:
		return s + strings.Repeat(" ", w-len(runes))
	}
}
