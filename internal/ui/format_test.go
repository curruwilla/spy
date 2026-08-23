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
