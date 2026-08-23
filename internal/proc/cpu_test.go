package proc

import "testing"

func TestReadCPUTimes(t *testing.T) {
	times, err := readCPUTimes(fixtureRoot + "/stat")
	if err != nil {
		t.Fatalf("readCPUTimes: %v", err)
	}
	if len(times) != 3 {
		t.Fatalf("got %d cpu lines, want 3 (machine + 2 cores)", len(times))
	}
	// user+nice+system+idle+iowait = 1000, of which idle and iowait are not busy.
	if want := (cpuTimes{total: 1000, busy: 170}); times[0] != want {
		t.Errorf("aggregate = %+v, want %+v", times[0], want)
	}
	if want := (cpuTimes{total: 500, busy: 85}); times[1] != want {
		t.Errorf("core 0 = %+v, want %+v", times[1], want)
	}
}

func TestParseCPUTimesIgnoresGuest(t *testing.T) {
	// guest and guest_nice are already included in user and nice, so
	// counting them again would push usage over 100%.
	fields := []string{"10", "0", "5", "80", "5", "0", "0", "0", "999", "999"}
	got, err := parseCPUTimes(fields)
	if err != nil {
		t.Fatalf("parseCPUTimes: %v", err)
	}
	if want := (cpuTimes{total: 100, busy: 15}); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestPercentBusy(t *testing.T) {
	cases := []struct {
		name      string
		prev, cur cpuTimes
		want      float64
	}{
		{"half busy", cpuTimes{total: 100, busy: 10}, cpuTimes{total: 200, busy: 60}, 50},
		{"idle", cpuTimes{total: 100, busy: 10}, cpuTimes{total: 200, busy: 10}, 0},
		{"no elapsed ticks", cpuTimes{total: 100, busy: 10}, cpuTimes{total: 100, busy: 10}, 0},
		{"counter reset", cpuTimes{total: 500, busy: 100}, cpuTimes{total: 10, busy: 2}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := percentBusy(c.prev, c.cur); got != c.want {
				t.Errorf("percentBusy = %v, want %v", got, c.want)
			}
		})
	}
}

func TestCPUUsageWithoutPrevious(t *testing.T) {
	cur := []cpuTimes{{total: 100, busy: 50}, {total: 50, busy: 25}}
	got := cpuUsage(nil, cur)
	if got.Total != 0 || len(got.Cores) != 1 || got.Cores[0] != 0 {
		t.Errorf("cpuUsage(nil, cur) = %+v, want all zeros with 1 core", got)
	}
}
