package proc

import "testing"

func TestParseStat(t *testing.T) {
	cases := []struct {
		name string
		line string
		want stat
	}{
		{
			name: "plain command",
			line: "42 (bash) S 7 42 42 0 -1 4194304 100 0 0 0 250 150 0 0 20 0 4 0 900 123 2048" + tail(),
			want: stat{comm: "bash", state: "S", ppid: 7, utime: 250, stime: 150, threads: 4, startTicks: 900, vsize: 123, rssPages: 2048},
		},
		{
			name: "command with spaces and parens",
			line: "42 (my prog (test)) R 1 42 42 0 -1 0 0 0 0 0 10 5 0 0 20 0 2 0 900 123 64" + tail(),
			want: stat{comm: "my prog (test)", state: "R", ppid: 1, utime: 10, stime: 5, threads: 2, startTicks: 900, vsize: 123, rssPages: 64},
		},
		{
			name: "renamed process",
			line: "42 (batch) S 1 42 42 0 -1 0 0 0 0 0 1 2 0 0 39 19 1 0 4200 8192 16" + tail(),
			want: stat{comm: "batch", state: "S", ppid: 1, utime: 1, stime: 2, nice: 19, threads: 1, startTicks: 4200, vsize: 8192, rssPages: 16},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseStat([]byte(c.line))
			if err != nil {
				t.Fatalf("parseStat: %v", err)
			}
			if got != c.want {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
			if want := c.want.utime + c.want.stime; got.busyTicks() != want {
				t.Errorf("busyTicks = %d, want %d", got.busyTicks(), want)
			}
		})
	}
}

func TestParseStatRejectsGarbage(t *testing.T) {
	cases := map[string]string{
		"no parens":  "42 bash S 7 42",
		"too short":  "42 (bash) S 7 42 42 0 -1",
		"empty line": "",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseStat([]byte(line)); err == nil {
				t.Errorf("parseStat(%q) succeeded, want an error", line)
			}
		})
	}
}

// tail pads a stat line with the fields after rss, which the parser skips.
func tail() string { return " 0 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0" }
