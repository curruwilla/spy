package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Basic ANSI colors keep the monitor readable on light and dark terminals,
// because they follow whatever palette the user already picked.
var (
	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleLabel    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("7"))
	styleSortCol  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleAlert    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("1"))
	styleKey      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("4"))

	// The detail panel sits on top of the table, so it is drawn with a
	// border to separate it from what is left of the screen.
	styleInfoBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).
			Padding(0, 1)
)

// heat colors a measurement by how alarming it is: calm below half, warm up
// to 80 percent, hot above.
func heat(pct float64) lipgloss.Style {
	switch {
	case pct >= 80:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	case pct >= 50:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	}
}

// gauge draws a horizontal bar: filled cells colored by load, the remainder
// dimmed.
func gauge(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	filled := int(pct / 100 * float64(width))
	filled = clamp(filled, 0, width)
	return heat(pct).Render(strings.Repeat("█", filled)) +
		styleDim.Render(strings.Repeat("░", width-filled))
}

// sparkline levels, from nearly idle to saturated.
var levels = []rune("▁▂▃▄▅▆▇█")

// sparkline draws one cell per value, its height and color both showing the
// load. It is how every core fits on a single line.
func sparkline(values []float64) string {
	var b strings.Builder
	for _, v := range values {
		i := clamp(int(v/100*float64(len(levels))), 0, len(levels)-1)
		b.WriteString(heat(v).Render(string(levels[i])))
	}
	return b.String()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
