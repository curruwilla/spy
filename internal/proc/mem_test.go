package proc

import "testing"

func TestReadMemory(t *testing.T) {
	m, err := readMemory(fixtureRoot + "/meminfo")
	if err != nil {
		t.Fatalf("readMemory: %v", err)
	}
	if want := uint64(32744648) * 1024; m.Total != want {
		t.Errorf("Total = %d, want %d", m.Total, want)
	}
	if want := uint64(16000000) * 1024; m.Available != want {
		t.Errorf("Available = %d, want %d", m.Available, want)
	}
	if want := (32744648 - 16000000) * uint64(1024); m.Used() != want {
		t.Errorf("Used = %d, want %d", m.Used(), want)
	}
	if got := m.UsedPercent(); got < 51 || got > 52 {
		t.Errorf("UsedPercent = %v, want about 51", got)
	}
	if want := (8388604 - 8000000) * uint64(1024); m.SwapUsed() != want {
		t.Errorf("SwapUsed = %d, want %d", m.SwapUsed(), want)
	}
}

func TestMemoryZeroTotals(t *testing.T) {
	var m Memory
	if got := m.UsedPercent(); got != 0 {
		t.Errorf("UsedPercent on empty Memory = %v, want 0", got)
	}
	if got := m.SwapPercent(); got != 0 {
		t.Errorf("SwapPercent with no swap = %v, want 0", got)
	}
}
