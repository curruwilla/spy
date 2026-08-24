# spy

Terminal system monitor: CPU, memory, disk and network throughput and processes on one
screen. Go + Bubble Tea, reading Linux `/proc` directly with no third-party metrics
library.

## Layout

- `cmd/spy` — flag parsing only, hands over to `ui.Run`.
- `internal/proc` — everything that touches `/proc`, plus `/sys` for the temperature.
  Counters there are cumulative, so `Collector` keeps the previous reading and turns
  the difference into percentages and byte rates. It is not safe for concurrent use:
  exactly one `Collect` runs at a time, scheduled by the previous snapshot. `Details`
  is the exception — it reads a handful of files for the one process the panel is open
  on, touches no collector state that changes, and so runs on the update loop.
- `internal/ui` — Bubble Tea model, key handling and rendering. `buildRows` is the one
  place that applies filter, sort and tree mode.

## Conventions

- Tests use the standard library only, table-driven, with `/proc` fixtures in
  `internal/proc/testdata`. No live-system assumptions except in `run_test.go`.
- The screen is a fixed header plus a 1-line footer; the table gets the rest
  (`tableHeight`). The header is 11 lines, or 7 on a terminal shorter than
  `compactHeight`, where it drops the top margin, the spacers and the `hist` line.
  Keep `headerLines` and `headerLinesCompact` in sync when adding header lines.
- Every detail on the right of the header starts at one column: `gaugeCells` sizes the
  bars from the widest of them, and `sparkCells` gives the spark lines exactly the room
  a bar takes up. No meter spells its percentage out — how full the bar is says it —
  so a bar and a spark line are the same length. The `core` line spends that room on the
  clearest form that fits: `coreBars` numbers every core while the labels fit, and
  past a dozen or so cores the row falls back to `sparkline`, with `sparkCell`
  splitting the room between the cells. The `hist` line keeps one column per reading,
  because it grows a reading at a time. Measure with `lipgloss.Width`, never `len`: the
  glyphs and the separators are multi-byte.
- The table is `columns`, one descriptor per column. `visibleColumns` drops the disk
  pair on a terminal too narrow for them, so nothing may assume a cell's position —
  `cellValue` and `cellStyle` switch on the `columnID`.
- Everything is laid out in `inner()`, the terminal less a `gutter` on each side,
  and `View` indents the finished lines into it. Use `m.inner()`, never `m.width`,
  when measuring a line.
- A bar carries the reading behind its percentage inside it, along the right end
  where the empty cells are: `gauge` takes the text, gives the scale what is left and
  drops the text on a bar too short to keep both. `Mem` and `Swp` say it there and
  have no detail on their right at all, so they are not counted in `gaugeCells`.
- Errors from a failed refresh are shown in the footer, never fatal: the last good
  snapshot stays on screen.
- `viewRow` marks how relevant a process is: kernel threads (`proc.Process.Kernel`)
  are dimmed whole, the reader's own account is colored apart from the others, the
  state letter carries the anomalies and an `active` process gets a bold command.
- The title, the column titles and the footer are filled bars. Every style used
  inside one carries its background (`styleBar*`, `styleColumns*`) and the line is
  padded to `inner()` with `fill`, otherwise the color stops mid-line.

## Go development

Before any Go coding, review, debugging, troubleshooting, or setup task, load the `samber/cc-skills-golang@golang-how-to` skill first — it routes to whichever other Go skills the task needs.
