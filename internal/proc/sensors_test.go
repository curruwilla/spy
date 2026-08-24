package proc

import "testing"

func TestReadTemperature(t *testing.T) {
	// The fixture has three sensors: the package temperature wins over
	// coretemp and over acpitz, which is the last resort.
	if got := readTemperature(sysFixtureRoot); got != 52 {
		t.Errorf("readTemperature = %v, want 52", got)
	}
}

func TestReadTemperatureWithoutSensors(t *testing.T) {
	if got := readTemperature("testdata/nowhere"); got != 0 {
		t.Errorf("readTemperature = %v, want 0 when the machine exposes nothing", got)
	}
}

func TestReadCPUModel(t *testing.T) {
	if got, want := readCPUModel(fixtureRoot+"/cpuinfo"), "Intel Core i7-8700K"; got != want {
		t.Errorf("readCPUModel = %q, want %q", got, want)
	}
	if got := readCPUModel("testdata/nowhere/cpuinfo"); got != "" {
		t.Errorf("readCPUModel = %q, want empty for a file that is not there", got)
	}
}

func TestTidyModel(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"intel", "Intel(R) Core(TM) i7-8700K CPU @ 3.70GHz", "Intel Core i7-8700K"},
		{"amd", "AMD Ryzen 9 5900X 12-Core Processor", "AMD Ryzen 9 5900X"},
		{"epyc", "AMD EPYC 7742 64-Core Processor", "AMD EPYC 7742"},
		{"arm board", "Raspberry Pi 4 Model B Rev 1.4", "Raspberry Pi 4 Model B Rev 1.4"},
		{"nothing at all", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tidyModel(c.in); got != c.want {
				t.Errorf("tidyModel(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
