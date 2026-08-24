package proc

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// defaultSysRoot is where sysfs is mounted. It is where the temperature
// comes from: /proc has no sensors of its own.
const defaultSysRoot = "/sys"

// cpuSensors are the sensor names that belong to the processor, best
// first. A machine exposes several — the battery, the wireless card, the
// chipset — and acpitz is last because on many boards it is not the CPU
// at all, only the closest thing to it the firmware admits to.
var cpuSensors = []string{
	"x86_pkg_temp", "coretemp", "k10temp", "zenpower",
	"cpu_thermal", "cpu-thermal", "soc_thermal", "acpitz",
}

// sensor is one temperature reading and the kind of sensor it came from.
type sensor struct {
	name    string
	celsius float64
}

// readTemperature is the hottest thing that can honestly be called the
// CPU, in degrees Celsius, or 0 when the machine exposes no sensor this
// code recognises: a wrong number here is worse than no number.
func readTemperature(sysRoot string) float64 {
	var celsius float64
	rank := len(cpuSensors)
	for _, s := range append(thermalSensors(sysRoot), hwmonSensors(sysRoot)...) {
		i := slices.Index(cpuSensors, s.name)
		if i < 0 || i > rank {
			continue
		}
		// Several sensors of the same kind — one per core — so the
		// warmest of them is the one worth reporting.
		if i < rank || s.celsius > celsius {
			celsius, rank = s.celsius, i
		}
	}
	return celsius
}

// thermalSensors reads /sys/class/thermal, where each zone holds the kind
// of sensor it is in type and its reading, in thousandths of a degree, in
// temp.
func thermalSensors(sysRoot string) []sensor {
	dirs, _ := filepath.Glob(filepath.Join(sysRoot, "class", "thermal", "thermal_zone*"))
	sensors := make([]sensor, 0, len(dirs))
	for _, dir := range dirs {
		name := readTrimmed(filepath.Join(dir, "type"))
		milli, err := strconv.ParseFloat(readTrimmed(filepath.Join(dir, "temp")), 64)
		if name == "" || err != nil {
			continue
		}
		sensors = append(sensors, sensor{name: name, celsius: milli / 1000})
	}
	return sensors
}

// hwmonSensors reads /sys/class/hwmon, which is where the drivers with no
// thermal zone of their own report: the driver's name next to its first
// temperature input.
func hwmonSensors(sysRoot string) []sensor {
	dirs, _ := filepath.Glob(filepath.Join(sysRoot, "class", "hwmon", "hwmon*"))
	sensors := make([]sensor, 0, len(dirs))
	for _, dir := range dirs {
		name := readTrimmed(filepath.Join(dir, "name"))
		milli, err := strconv.ParseFloat(readTrimmed(filepath.Join(dir, "temp1_input")), 64)
		if name == "" || err != nil {
			continue
		}
		sensors = append(sensors, sensor{name: name, celsius: milli / 1000})
	}
	return sensors
}

// modelKeys are the /proc/cpuinfo lines that name the processor, in the
// order they are looked for: x86 spells it out in model name, while an arm
// board says what the board is instead.
var modelKeys = []string{"model name", "Hardware", "cpu model", "Model"}

// readCPUModel is what the processor calls itself, tidied up. It is read
// once and kept: it cannot change while the machine is running.
func readCPUModel(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	found := map[string]string{}
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		key, value, ok := strings.Cut(scan.Text(), ":")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		// Every core repeats the line; the first one is enough.
		if _, seen := found[key]; !seen && value != "" {
			found[key] = value
		}
	}
	for _, key := range modelKeys {
		if name, ok := found[key]; ok {
			return tidyModel(name)
		}
	}
	return ""
}

// tidyModel drops what a processor name carries for the marketing rather
// than for the reader: the trademark signs, the clock it will not hold
// under load anyway, and the core count the header already shows.
func tidyModel(name string) string {
	name, _, _ = strings.Cut(name, " @ ")
	for _, noise := range []string{"(R)", "(r)", "(TM)", "(tm)"} {
		name = strings.ReplaceAll(name, noise, "")
	}
	kept := make([]string, 0, len(strings.Fields(name)))
	for _, word := range strings.Fields(name) {
		if word == "CPU" || word == "Processor" || strings.HasSuffix(word, "-Core") {
			continue
		}
		kept = append(kept, word)
	}
	return strings.Join(kept, " ")
}

// readTrimmed is one short sysfs file, without the newline it ends with.
// A file that is not there reads as empty: sensors come and go with the
// drivers that publish them.
func readTrimmed(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
