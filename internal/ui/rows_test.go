package ui

import (
	"strings"
	"testing"

	"github.com/curruwilla/spy/internal/proc"
)

// sample is a small process family:
//
//	1 init
//	├─ 10 nginx      (root)
//	│  └─ 11 nginx worker
//	└─ 20 editor     (will)
func sample() []proc.Process {
	return []proc.Process{
		{PID: 1, PPID: 0, User: "root", Command: "init", CPU: 0.5, RSS: 1000},
		{PID: 10, PPID: 1, User: "root", Command: "nginx", CPU: 5, RSS: 3000},
		{PID: 11, PPID: 10, User: "www", Command: "nginx worker", CPU: 20, RSS: 2000},
		{PID: 20, PPID: 1, User: "will", Command: "editor", CPU: 1, RSS: 9000},
	}
}

func pids(rows []row) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r.proc.PID
	}
	return out
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFlatSorting(t *testing.T) {
	cases := []struct {
		name    string
		key     sortKey
		reverse bool
		want    []int
	}{
		{"cpu, busiest first", sortCPU, false, []int{11, 10, 20, 1}},
		{"cpu reversed", sortCPU, true, []int{1, 20, 10, 11}},
		{"memory, largest first", sortMem, false, []int{20, 10, 11, 1}},
		{"pid ascending", sortPID, false, []int{1, 10, 11, 20}},
		{"name alphabetical", sortName, false, []int{20, 1, 10, 11}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pids(buildRows(sample(), c.key, c.reverse, filter{}, false))
			if !equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestFilterMatchesCommandUserAndPID(t *testing.T) {
	cases := []struct {
		filter string
		want   []int
	}{
		{"nginx", []int{11, 10}},
		{"NGINX", []int{11, 10}},
		{"will", []int{20}},
		{"11", []int{11}},
		{"nothing here", nil},
	}
	for _, c := range cases {
		t.Run(c.filter, func(t *testing.T) {
			got := pids(buildRows(sample(), sortCPU, false, filter{text: c.filter}, false))
			if !equal(got, c.want) {
				t.Errorf("filter %q gave %v, want %v", c.filter, got, c.want)
			}
		})
	}
}

func TestTreeNesting(t *testing.T) {
	rows := buildRows(sample(), sortCPU, false, filter{}, true)
	if got, want := pids(rows), []int{1, 10, 11, 20}; !equal(got, want) {
		t.Fatalf("order = %v, want %v (parents before children)", got, want)
	}

	indents := map[int]string{1: "", 10: "├─ ", 11: "│  └─ ", 20: "└─ "}
	for _, r := range rows {
		if want := indents[r.proc.PID]; r.indent != want {
			t.Errorf("pid %d indent = %q, want %q", r.proc.PID, r.indent, want)
		}
	}
}

// TestTreeFilterKeepsAncestors makes sure a match deep in the tree still
// shows the chain that leads to it.
func TestTreeFilterKeepsAncestors(t *testing.T) {
	rows := buildRows(sample(), sortCPU, false, filter{text: "worker"}, true)
	if got, want := pids(rows), []int{1, 10, 11}; !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestTreeOrphanBecomesRoot covers a process whose parent already exited,
// which happens constantly on a live machine.
func TestTreeOrphanBecomesRoot(t *testing.T) {
	procs := append(sample(), proc.Process{PID: 99, PPID: 4242, Command: "orphan", CPU: 100})
	rows := buildRows(procs, sortCPU, false, filter{}, true)
	if rows[0].proc.PID != 99 || rows[0].indent != "" {
		t.Errorf("orphan = %+v, want a root row", rows[0])
	}
	if len(rows) != 5 {
		t.Errorf("rows = %d, want 5", len(rows))
	}
}

func TestTreeSelfParentDoesNotLoop(t *testing.T) {
	procs := []proc.Process{{PID: 3, PPID: 3, Command: "weird"}}
	rows := buildRows(procs, sortCPU, false, filter{}, true)
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if strings.Contains(rows[0].indent, "└") {
		t.Errorf("indent = %q, want a root row", rows[0].indent)
	}
}
