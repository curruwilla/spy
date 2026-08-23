package ui

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/curruwilla/spy/internal/proc"
)

// TestRunDrawsAndQuits drives the real program loop against the live /proc
// with a fake terminal: it must render a frame and then exit on "q".
func TestRunDrawsAndQuits(t *testing.T) {
	input, keys := io.Pipe()
	var output syncBuffer

	done := make(chan error, 1)
	go func() {
		done <- Run(proc.NewCollector(), Options{Interval: 100 * time.Millisecond, Sort: "cpu"},
			tea.WithInput(input), tea.WithOutput(&output))
	}()

	// Give the first snapshot time to arrive, then quit.
	time.Sleep(300 * time.Millisecond)
	if _, err := keys.Write([]byte("q")); err != nil {
		t.Fatalf("write key: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the program did not quit on q")
	}

	frame := output.String()
	for _, want := range []string{"spy", "CPU", "MEM", "PID", "COMMAND"} {
		if !strings.Contains(frame, want) {
			t.Errorf("rendered frame is missing %q", want)
		}
	}
}

func TestRunRejectsBadOptions(t *testing.T) {
	if err := Run(nil, Options{Sort: "swap"}); err == nil {
		t.Error("want an error for an unknown sort column")
	}
}

// syncBuffer collects the program output, which is written from the render
// goroutine while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
