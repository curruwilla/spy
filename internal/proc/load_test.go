package proc

import (
	"testing"
	"time"
)

func TestReadLoad(t *testing.T) {
	l, err := readLoad(fixtureRoot + "/loadavg")
	if err != nil {
		t.Fatalf("readLoad: %v", err)
	}
	want := Load{One: 1.80, Five: 1.41, Fifteen: 1.07, Running: 3, Total: 1234}
	if l != want {
		t.Errorf("got %+v, want %+v", l, want)
	}
}

func TestReadUptime(t *testing.T) {
	d, err := readUptime(fixtureRoot + "/uptime")
	if err != nil {
		t.Fatalf("readUptime: %v", err)
	}
	if want := 141785*time.Second + 420*time.Millisecond; d != want {
		t.Errorf("got %s, want %s", d, want)
	}
}

func TestReadLoadMissingFile(t *testing.T) {
	if _, err := readLoad(fixtureRoot + "/nope"); err == nil {
		t.Error("want an error for a missing loadavg")
	}
}
