package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/curruwilla/spy/internal/proc"
)

func TestParseThresholds(t *testing.T) {
	cases := []struct {
		in   string
		want thresholds
	}{
		{"", thresholds{}},
		{"cpu>5", thresholds{CPU: 5}},
		{"cpu 5", thresholds{CPU: 5}},
		{"cpu>=5", thresholds{CPU: 5}},
		{"cpu=5", thresholds{CPU: 5}},
		{"CPU>5.5%", thresholds{CPU: 5.5}},
		{"mem>2", thresholds{Mem: 2}},
		{"ram>2%", thresholds{Mem: 2}},
		{"mem>500M", thresholds{RSS: 500 << 20}},
		{"mem>1.5g", thresholds{RSS: 1536 << 20}},
		{"mem>4096B", thresholds{RSS: 4096}},
		{"time>90", thresholds{Time: 90 * time.Second}},
		{"time>1m30s", thresholds{Time: 90 * time.Second}},
		{"time>2h", thresholds{Time: 2 * time.Hour}},
		{"cpu>5 mem>500M time>1m", thresholds{CPU: 5, RSS: 500 << 20, Time: time.Minute}},
		{"  cpu > 5   mem : 10  ", thresholds{CPU: 5, Mem: 10}},
		// The two ways of saying "memory" are alternatives: the last wins.
		{"mem>10 mem>500M", thresholds{RSS: 500 << 20}},
		{"mem>500M mem>10", thresholds{Mem: 10}},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := parseThresholds(c.in)
			if err != nil {
				t.Fatalf("parseThresholds(%q): %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("parseThresholds(%q) = %+v, want %+v", c.in, got, c.want)
			}
		})
	}
}

func TestParseThresholdsRejectsBadInput(t *testing.T) {
	cases := []string{
		"disk>5",     // not a measurement
		"cpu",        // no value
		"cpu>abc",    // not a number
		"cpu>-5",     // below zero
		"mem>500X",   // not a unit
		"mem>M",      // unit without a number
		"time>later", // not a duration
		"time>-1m",   // below zero
	}
	for _, in := range cases {
		t.Run(in, func(t *testing.T) {
			if got, err := parseThresholds(in); err == nil {
				t.Errorf("parseThresholds(%q) = %+v, want an error", in, got)
			}
		})
	}
}

// TestThresholdsStringRoundTrips is what lets the prompt be prefilled with
// the active thresholds: what it prints has to parse back the same.
func TestThresholdsStringRoundTrips(t *testing.T) {
	cases := []thresholds{
		{},
		{CPU: 5},
		{Mem: 2.5},
		{RSS: 500 << 20},
		{Time: 90 * time.Second},
		{CPU: 5, RSS: 1 << 30, Time: time.Hour},
	}
	for _, want := range cases {
		t.Run(want.String(), func(t *testing.T) {
			got, err := parseThresholds(want.String())
			if err != nil {
				t.Fatalf("reparsing %q: %v", want.String(), err)
			}
			if got != want {
				t.Errorf("%q parsed back as %+v, want %+v", want.String(), got, want)
			}
		})
	}
}

// busy is a family whose measurements span every threshold, so one fixture
// covers cpu, mem and time.
//
//	1 init   0.5%  0.1%    4K   1s
//	├─ 10 nginx    5%  2.0%  500M  1m
//	│  └─ 11 worker  20%  8.0%    2G  1h
//	└─ 20 editor     1%  0.5%   64M  30s
func busy() []proc.Process {
	return []proc.Process{
		{PID: 1, PPID: 0, Command: "init", CPU: 0.5, Mem: 0.1, RSS: 4 << 10, CPUTime: time.Second},
		{PID: 10, PPID: 1, Command: "nginx", CPU: 5, Mem: 2, RSS: 500 << 20, CPUTime: time.Minute},
		{PID: 11, PPID: 10, Command: "worker", CPU: 20, Mem: 8, RSS: 2 << 30, CPUTime: time.Hour},
		{PID: 20, PPID: 1, Command: "editor", CPU: 1, Mem: 0.5, RSS: 64 << 20, CPUTime: 30 * time.Second},
	}
}

func TestThresholdsSelectRows(t *testing.T) {
	cases := []struct {
		min  string
		want []int
	}{
		{"", []int{11, 10, 20, 1}},
		{"cpu>5", []int{11, 10}}, // at least, so 10 with exactly 5% stays
		{"cpu>5.1", []int{11}},
		{"mem>2", []int{11, 10}},
		{"mem>100M", []int{11, 10}},
		{"mem>3G", nil},
		{"time>30s", []int{11, 10, 20}},
		{"time>1m30s", []int{11}},
		{"cpu>1 time>1m", []int{11, 10}},
		{"cpu>100", nil},
	}
	for _, c := range cases {
		t.Run(c.min, func(t *testing.T) {
			min, err := parseThresholds(c.min)
			if err != nil {
				t.Fatal(err)
			}
			got := pids(buildRows(busy(), sortCPU, false, filter{min: min}, false))
			if !equal(got, c.want) {
				t.Errorf("min %q gave %v, want %v", c.min, got, c.want)
			}
		})
	}
}

// TestThresholdCombinesWithText makes sure the two halves of the filter
// narrow the list together instead of one replacing the other.
func TestThresholdCombinesWithText(t *testing.T) {
	min, err := parseThresholds("cpu>2")
	if err != nil {
		t.Fatal(err)
	}
	got := pids(buildRows(busy(), sortCPU, false, filter{text: "nginx", min: min}, false))
	if want := []int{10}; !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestThresholdTreeKeepsAncestors mirrors the text filter: a busy process
// deep in the tree still shows the chain that leads to it.
func TestThresholdTreeKeepsAncestors(t *testing.T) {
	min, err := parseThresholds("cpu>10")
	if err != nil {
		t.Fatal(err)
	}
	rows := buildRows(busy(), sortCPU, false, filter{min: min}, true)
	if got, want := pids(rows), []int{1, 10, 11}; !equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestNewRejectsBadThreshold(t *testing.T) {
	if _, err := New(nil, Options{Sort: "cpu", Min: "disk>5"}); err == nil {
		t.Error("want an error for an unknown threshold")
	}
}

func TestOptionsApplyThresholds(t *testing.T) {
	m, err := New(nil, Options{Sort: "cpu", Min: "cpu>5"})
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 140, 20
	m.snap = proc.Snapshot{Processes: busy()}
	m.rebuild()
	if got, want := pids(m.rows), []int{11, 10}; !equal(got, want) {
		t.Errorf("rows = %v, want %v", got, want)
	}
	if !strings.Contains(m.View(), "cpu>5") {
		t.Error("the footer should show the active threshold")
	}
}
