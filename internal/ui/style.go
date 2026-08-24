package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The bars are the one place that sets a background, so they cannot follow
// the terminal palette the way the rest of the screen does: a fixed color
// there is unreadable on half the themes out there. They use neutral greys
// picked per scheme instead, quiet enough to sit behind the text and to
// leave the gauges as the only strong color on the screen.
var (
	barBackground    = lipgloss.AdaptiveColor{Light: "253", Dark: "236"}
	columnBackground = lipgloss.AdaptiveColor{Light: "250", Dark: "239"}
	barText          = lipgloss.AdaptiveColor{Light: "240", Dark: "249"}
	barBright        = lipgloss.AdaptiveColor{Light: "233", Dark: "255"}
	barAccent        = lipgloss.AdaptiveColor{Light: "25", Dark: "117"}
	barWarn          = lipgloss.AdaptiveColor{Light: "130", Dark: "215"}

	// An alert is the one bar that is meant to be loud, so it keeps a color
	// of its own, muted enough to read the text on it.
	alertBackground = lipgloss.AdaptiveColor{Light: "224", Dark: "52"}
	alertText       = lipgloss.AdaptiveColor{Light: "88", Dark: "217"}
)

// Basic ANSI colors keep the rest of the monitor readable on light and dark
// terminals, because they follow whatever palette the user already picked.
var (
	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleLabel    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleSelected = lipgloss.NewStyle().Reverse(true)

	// How much of the reader's attention a row is worth: what is doing
	// something now stands out, what belongs to someone else recedes, and
	// kernel threads recede furthest.
	styleActive = lipgloss.NewStyle().Bold(true)
	styleMine   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleOther  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	// Three lines are filled bars: the title, the column titles and the
	// footer. Every style used inside one carries the background as well,
	// because a segment drawn without it punches a hole in the bar.
	styleBar       = lipgloss.NewStyle().Background(barBackground).Foreground(barText)
	styleBarStrong = styleBar.Foreground(barBright).Bold(true)
	styleBarKey    = styleBar.Foreground(barAccent).Bold(true)
	styleBarCursor = lipgloss.NewStyle().Background(barAccent).Foreground(barBackground)
	styleBarWarn   = styleBar.Foreground(barWarn).Bold(true)
	styleBarAlert  = lipgloss.NewStyle().Background(alertBackground).Foreground(alertText).Bold(true)

	// The column titles get a slightly stronger bar of their own, so the
	// table reads as something separate from the gauges above it.
	styleColumns     = lipgloss.NewStyle().Background(columnBackground).Foreground(barBright)
	styleColumnsSort = styleColumns.Foreground(barAccent).Bold(true)

	// The detail panel sits on top of the table, so it is drawn with a
	// border to separate it from what is left of the screen.
	styleInfoBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).
			Padding(0, 1)
)

// stateStyles color the process states that mean something is wrong or
// about to happen. Everything else is asleep, which is the normal state of
// most of the table and is left plain.
var stateStyles = map[string]lipgloss.Style{
	"R": lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true), // on a core right now
	"D": lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Bold(true), // stuck in a syscall
	"T": lipgloss.NewStyle().Foreground(lipgloss.Color("3")),            // stopped
	"t": lipgloss.NewStyle().Foreground(lipgloss.Color("3")),            // stopped by a debugger
	"Z": lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true), // gone, still in the table
}

// stateStyle is how a state letter is drawn: plain unless it is worth
// noticing.
func stateStyle(state string) lipgloss.Style {
	if style, ok := stateStyles[state]; ok {
		return style
	}
	return styleDim
}

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

// What a gauge is drawn with: a thin stroke for the filled cells and a dot
// for the rest, inside brackets that give the bar an edge to end at.
const (
	gaugeFilled = "▇"
	gaugeEmpty  = "·"
	gaugeOpen   = "["
	gaugeClose  = "]"
)

// gauge draws a bracketed bar of cells, the filled ones colored by load and
// the rest dimmed. The cells are spaced apart so the bar reads as a scale
// instead of a solid block; cells is how many of them there are, so the
// result is gaugeWidth(cells) columns wide whatever else is on it.
//
// inside is the reading the bar is a picture of — the bytes behind the
// percentage — written along the right end of the bar, where the empty
// cells are and there is nothing to cover. The cells make do with the room
// it leaves, down to the shortest scale still worth reading; a bar narrower
// than that keeps its cells and goes without the reading.
func gauge(pct float64, cells int, inside string) string {
	if cells <= 0 {
		return ""
	}
	room := gaugeWidth(cells) - len(gaugeOpen) - len(gaugeClose)
	if text := lipgloss.Width(inside); text > 0 {
		// The scale keeps a gap the width of the ones around the bar, so
		// that the last cell and the first digit do not read as one thing.
		if n := (room - text - len(meterGap) + 1) / 2; n >= minGaugeCells {
			cells = n
		} else {
			inside = ""
		}
	}
	filled := clamp(int(pct/100*float64(cells)), 0, cells)
	parts := make([]string, 0, 2)
	if filled > 0 {
		parts = append(parts, heat(pct).Render(spaced(gaugeFilled, filled)))
	}
	if filled < cells {
		parts = append(parts, styleDim.Render(spaced(gaugeEmpty, cells-filled)))
	}
	body := strings.Join(parts, " ")
	if inside != "" {
		body += strings.Repeat(" ", room-gaugeWidth(cells)+2-lipgloss.Width(inside)) + inside
	}
	return bracket(body)
}

// gaugeWidth is how many columns a bar of n cells takes: a gap between
// every pair of them, and the brackets around the lot.
func gaugeWidth(cells int) int {
	if cells <= 0 {
		return 0
	}
	return 2*cells + 1
}

// bracket puts a gauge or a sparkline between dimmed brackets.
func bracket(s string) string {
	return styleDim.Render(gaugeOpen) + s + styleDim.Render(gaugeClose)
}

// spaced repeats cell n times with a gap between each.
func spaced(cell string, n int) string {
	return strings.TrimSuffix(strings.Repeat(cell+" ", n), " ")
}

// sparkline levels, from nearly idle to saturated.
var levels = []rune("▁▂▃▄▅▆▇█")

// sparkGroup is how many readings share a group. A gap every few cells
// keeps a long line countable instead of a solid block.
const sparkGroup = 4

// sparkWidth is how many columns n cells take up once the gaps between the
// groups are in.
func sparkWidth(n int) int {
	if n <= 0 {
		return 0
	}
	return n + (n-1)/sparkGroup
}

// sparkline draws one cell per value, its height and color both showing the
// load. It is how a long trend fits on a single line, and it never draws
// more than width cells: a header line that wraps costs the table a row. A
// trend too long for the gaps loses them first, and only then the readings
// on its right, replaced by an ellipsis.
func sparkline(values []float64, width int) string {
	if width <= 0 || len(values) == 0 {
		return ""
	}
	group := sparkGroup
	if sparkWidth(len(values)) > width {
		group = 0
	}
	trimmed := len(values) > width
	if trimmed {
		values = values[:width-1]
	}

	var b strings.Builder
	for n, v := range values {
		if group > 0 && n > 0 && n%group == 0 {
			b.WriteByte(' ')
		}
		i := clamp(int(v/100*float64(len(levels))), 0, len(levels)-1)
		b.WriteString(heat(v).Render(string(levels[i])))
	}
	if trimmed {
		b.WriteString(styleDim.Render("…"))
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
