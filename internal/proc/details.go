package proc

import (
	"os"
	"strconv"
	"strings"
)

// Details is what the detail panel adds to a Process: the facts that cost
// a file of their own, and so are only worth reading for the one process
// the panel is open on rather than for every row of the table.
type Details struct {
	CWD        string // the directory the process runs in
	Exe        string // the binary behind the command line
	Cgroup     string // the container it runs in, or the systemd unit that started it
	Files      int    // open file descriptors, -1 when the reader may not count them
	OOMScore   int    // how likely the kernel is to pick it when memory runs out
	Swap       uint64 // resident memory pushed out to swap
	Switches   uint64 // context switches since it started, both kinds together
	Restricted bool   // some of it needed privileges the reader does not have
}

// Details reads the extras for one process. Everything here is best
// effort: most of it belongs to the process owner, so a process of
// somebody else's comes back mostly empty with Restricted set, which is
// itself worth showing. Only immutable collector state is touched, so this
// is safe to call while a Collect is in flight.
func (c *Collector) Details(pid int) Details {
	dir := c.path(strconv.Itoa(pid))
	d := Details{Files: -1}

	var denied bool
	d.CWD, denied = readLink(dir + "/cwd")
	d.Restricted = d.Restricted || denied
	d.Exe, denied = readLink(dir + "/exe")
	d.Restricted = d.Restricted || denied
	d.Cgroup = readCgroup(dir + "/cgroup")
	d.OOMScore, _ = strconv.Atoi(readTrimmed(dir + "/oom_score"))

	if entries, err := os.ReadDir(dir + "/fd"); err == nil {
		d.Files = len(entries)
	} else if os.IsPermission(err) {
		d.Restricted = true
	}

	d.Swap, d.Switches = readStatus(dir + "/status")
	return d
}

// statusSwap and the switch counters are the /proc/<pid>/status lines the
// panel shows that /proc/<pid>/stat has no field for. The sizes there are
// in kB.
const (
	statusSwap         = "VmSwap"
	statusSwitches     = "voluntary_ctxt_switches"
	statusForcedSwitch = "nonvoluntary_ctxt_switches"
)

// readStatus picks the swapped-out size and the context switch count out
// of /proc/<pid>/status. A process with nothing in swap has no VmSwap line
// at all, and a kernel thread has none of them.
func readStatus(path string) (swap, switches uint64) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(value)
		if len(fields) == 0 {
			continue
		}
		n, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		switch key {
		case statusSwap:
			swap = n * 1024
		case statusSwitches, statusForcedSwitch:
			switches += n
		}
	}
	return swap, switches
}

// readLink resolves one of the symlinks in a process directory, and says
// whether it came back empty because the reader is not allowed to look.
func readLink(path string) (target string, denied bool) {
	target, err := os.Readlink(path)
	if err != nil {
		return "", os.IsPermission(err)
	}
	return target, false
}

// readCgroup names the group a process was put in: the container it runs
// in, or the unit systemd started it as. Both cgroup layouts put the path
// last on the line — v2 writes a single "0::/path", v1 one line per
// controller — and the systemd hierarchy is the one that carries a name a
// reader recognises.
func readCgroup(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var best string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 || parts[2] == "" || parts[2] == "/" {
			continue
		}
		if best == "" || parts[1] == "" || parts[1] == "name=systemd" {
			best = parts[2]
		}
	}
	return shortCgroup(best)
}

// containerRuntimes are the prefixes the runtimes put in front of a
// container id in the cgroup path.
var containerRuntimes = []string{"docker-", "libpod-", "crio-", "cri-containerd-", "containerd-"}

// shortCgroup keeps the last element of a cgroup path and strips what is
// wrapped around it: the scope or slice suffix systemd adds, and the
// runtime's prefix in front of a 64 character container id, of which the
// first twelve are what the runtimes themselves print.
func shortCgroup(path string) string {
	name := path[strings.LastIndexByte(path, '/')+1:]
	for _, suffix := range []string{".scope", ".service", ".slice"} {
		name = strings.TrimSuffix(name, suffix)
	}
	for _, prefix := range containerRuntimes {
		id, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		if len(id) > 12 {
			id = id[:12]
		}
		return strings.TrimSuffix(prefix, "-") + " " + id
	}
	return name
}
