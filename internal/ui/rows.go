package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/curruwilla/spy/internal/proc"
)

// row is one printable line of the process table.
type row struct {
	proc   proc.Process
	indent string // tree connectors, empty in flat mode
}

// buildRows applies the filter, the sort column and the view mode to a
// snapshot, producing the exact lines the table will draw.
func buildRows(procs []proc.Process, key sortKey, reverse bool, filter string, tree bool) []row {
	less := func(a, b proc.Process) int {
		if reverse {
			return key.compare(b, a)
		}
		return key.compare(a, b)
	}
	if tree {
		return treeRows(procs, less, filter)
	}
	return flatRows(procs, less, filter)
}

func flatRows(procs []proc.Process, less func(a, b proc.Process) int, filter string) []row {
	kept := make([]proc.Process, 0, len(procs))
	for _, p := range procs {
		if matches(p, filter) {
			kept = append(kept, p)
		}
	}
	slices.SortStableFunc(kept, less)

	rows := make([]row, len(kept))
	for i, p := range kept {
		rows[i] = row{proc: p}
	}
	return rows
}

// treeRows nests each process under its parent. With a filter active the
// matches are kept along with their ancestors, so the hierarchy stays
// readable instead of collapsing into orphans.
func treeRows(procs []proc.Process, less func(a, b proc.Process) int, filter string) []row {
	byPID := make(map[int]proc.Process, len(procs))
	children := make(map[int][]proc.Process, len(procs))
	for _, p := range procs {
		byPID[p.PID] = p
	}

	keep := keepSet(procs, byPID, filter)
	var roots []proc.Process
	for _, p := range procs {
		if !keep[p.PID] {
			continue
		}
		if _, hasParent := byPID[p.PPID]; hasParent && p.PPID != p.PID {
			children[p.PPID] = append(children[p.PPID], p)
			continue
		}
		roots = append(roots, p)
	}
	slices.SortStableFunc(roots, less)
	for pid := range children {
		slices.SortStableFunc(children[pid], less)
	}

	rows := make([]row, 0, len(procs))
	var walk func(p proc.Process, prefix string, last bool, depth int)
	walk = func(p proc.Process, prefix string, last bool, depth int) {
		indent := prefix
		if depth > 0 {
			indent += branch(last)
		}
		rows = append(rows, row{proc: p, indent: indent})

		kids := children[p.PID]
		for i, kid := range kids {
			childPrefix := prefix
			if depth > 0 {
				childPrefix += continuation(last)
			}
			walk(kid, childPrefix, i == len(kids)-1, depth+1)
		}
	}
	for _, r := range roots {
		walk(r, "", true, 0)
	}
	return rows
}

func branch(last bool) string {
	if last {
		return "└─ "
	}
	return "├─ "
}

func continuation(last bool) string {
	if last {
		return "   "
	}
	return "│  "
}

// keepSet marks the processes a filter selects plus every ancestor needed to
// reach them. Without a filter every process is kept.
func keepSet(procs []proc.Process, byPID map[int]proc.Process, filter string) map[int]bool {
	keep := make(map[int]bool, len(procs))
	if filter == "" {
		for _, p := range procs {
			keep[p.PID] = true
		}
		return keep
	}
	for _, p := range procs {
		if !matches(p, filter) {
			continue
		}
		for cur, ok := p, true; ok && !keep[cur.PID]; cur, ok = byPID[cur.PPID] {
			keep[cur.PID] = true
		}
	}
	return keep
}

// matches reports whether a process satisfies the filter, which is compared
// against the command, the owner and the pid.
func matches(p proc.Process, filter string) bool {
	if filter == "" {
		return true
	}
	filter = strings.ToLower(filter)
	return strings.Contains(strings.ToLower(p.Command), filter) ||
		strings.Contains(strings.ToLower(p.User), filter) ||
		strings.Contains(strconv.Itoa(p.PID), filter)
}
