# spy

Terminal system monitor: CPU, memory and processes on one screen. Go + Bubble Tea,
reading Linux `/proc` directly with no third-party metrics library.

## Layout

- `cmd/spy` — flag parsing only, hands over to `ui.Run`.
- `internal/proc` — everything that touches `/proc`. Counters there are cumulative,
  so `Collector` keeps the previous reading and turns the difference into percentages.
  It is not safe for concurrent use: exactly one `Collect` runs at a time, scheduled by
  the previous snapshot.
- `internal/ui` — Bubble Tea model, key handling and rendering. `buildRows` is the one
  place that applies filter, sort and tree mode.

## Conventions

- Tests use the standard library only, table-driven, with `/proc` fixtures in
  `internal/proc/testdata`. No live-system assumptions except in `run_test.go`.
- The screen is a fixed header plus a 1-line footer; the table gets the rest
  (`tableHeight`). The header is 10 lines, or 7 on a terminal shorter than
  `compactHeight`, where it drops the top margin and the spacers. Keep
  `headerLines` and `headerLinesCompact` in sync when adding header lines.
- Everything is laid out in `inner()`, the terminal less a `gutter` on each side,
  and `View` indents the finished lines into it. Use `m.inner()`, never `m.width`,
  when measuring a line.
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
