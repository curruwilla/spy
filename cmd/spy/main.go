// Command spy is a terminal system monitor: CPU, memory, and the process
// list with sorting, filtering and a process tree, all on one screen.
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/curruwilla/spy/internal/proc"
	"github.com/curruwilla/spy/internal/ui"
)

// version is stamped at build time: go build -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "spy:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		opts                    ui.Options
		minCPU, minMem, minTime string
		showVersion             bool
	)
	flag.DurationVar(&opts.Interval, "i", 2*time.Second, "refresh interval")
	flag.StringVar(&opts.Sort, "sort", "cpu", "initial sort column: cpu, mem, pid, name, time or io")
	flag.BoolVar(&opts.Tree, "tree", false, "start in process tree mode")
	flag.StringVar(&opts.Filter, "filter", "", "only show processes matching this text")
	flag.StringVar(&minCPU, "min-cpu", "", "only show processes using at least this much CPU, in percent of one core")
	flag.StringVar(&minMem, "min-mem", "", "only show processes using at least this much memory: a percentage (2) or a size (500M)")
	flag.StringVar(&minTime, "min-time", "", "only show processes with at least this much CPU time (90, 90s, 1m30s)")
	flag.BoolVar(&showVersion, "version", false, "print the version and exit")
	flag.Parse()

	opts.Min = minClauses(minCPU, minMem, minTime)

	if showVersion {
		fmt.Printf("spy %s (%s)\n", version, runtime.Version())
		return nil
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("spy reads /proc, which %s does not provide", runtime.GOOS)
	}
	if opts.Interval < 100*time.Millisecond {
		return fmt.Errorf("refresh interval %s is too short, use at least 100ms", opts.Interval)
	}

	// Cell motion is the quietest of the mouse modes: it reports the wheel
	// and the clicks, and only follows the pointer while a button is down.
	return ui.Run(proc.NewCollector(), opts, tea.WithAltScreen(), tea.WithMouseCellMotion())
}

// minClauses turns the -min-* flags into the same clause syntax the in-app
// prompt takes, so both ways of setting a threshold go through one parser.
func minClauses(cpu, mem, cputime string) string {
	given := []struct{ name, value string }{
		{"cpu", cpu},
		{"mem", mem},
		{"time", cputime},
	}
	var clauses []string
	for _, c := range given {
		if c.value != "" {
			clauses = append(clauses, c.name+">"+c.value)
		}
	}
	return strings.Join(clauses, " ")
}
