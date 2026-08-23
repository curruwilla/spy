package ui

import (
	"testing"
	"time"
)

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		bytes uint64
		want  string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0K"},
		{524288, "512K"},
		{5 * 1024 * 1024, "5.0M"},
		{698 * 1024 * 1024, "698M"},
		{16_853_247_296, "15.7G"},
	}
	for _, c := range cases {
		if got := formatBytes(c.bytes); got != c.want {
			t.Errorf("formatBytes(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}

func TestFormatCPUTime(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"},
		{45 * time.Second, "0:45"},
		{9*time.Minute + 37*time.Second, "9:37"},
		{time.Hour + 39*time.Minute + 18*time.Second, "1:39:18"},
	}
	for _, c := range cases {
		if got := formatCPUTime(c.d); got != c.want {
			t.Errorf("formatCPUTime(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "0m"},
		{90 * time.Minute, "1h 30m"},
		{39*time.Hour + 20*time.Minute, "1d 15h"},
	}
	for _, c := range cases {
		if got := formatUptime(c.d); got != c.want {
			t.Errorf("formatUptime(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestPad(t *testing.T) {
	cases := []struct {
		in    string
		width int
		right bool
		want  string
	}{
		{"abc", 5, false, "abc  "},
		{"abc", 5, true, "  abc"},
		{"abcdef", 4, false, "abc…"},
		{"abc", 1, false, "…"},
		{"abc", 0, false, ""},
		{"abc", 3, false, "abc"},
	}
	for _, c := range cases {
		if got := pad(c.in, c.width, c.right); got != c.want {
			t.Errorf("pad(%q, %d, %v) = %q, want %q", c.in, c.width, c.right, got, c.want)
		}
	}
}

func TestWrapText(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		width int
		want  []string
	}{
		{"fits on one line", "spy -tree", 20, []string{"spy -tree"}},
		{"breaks at spaces", "one two three four", 9, []string{"one two", "three", "four"}},
		{"a word longer than the line is cut", "aaaaaaaa b", 4, []string{"aaaa", "aaaa", "b"}},
		{"a long word flushes what came before", "ab cdefghij", 4, []string{"ab", "cdef", "ghij"}},
		{"runs of spaces collapse", "one   two", 20, []string{"one two"}},
		{"empty", "", 10, nil},
		{"no width", "one two", 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := wrapText(c.in, c.width)
			if len(got) != len(c.want) {
				t.Fatalf("wrapText(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("line %d = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestFormatState(t *testing.T) {
	cases := map[string]string{"S": "S sleeping", "R": "R running", "Z": "Z zombie", "?": "?"}
	for in, want := range cases {
		if got := formatState(in); got != want {
			t.Errorf("formatState(%q) = %q, want %q", in, got, want)
		}
	}
}
