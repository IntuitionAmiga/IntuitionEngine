//go:build windows

package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// TerminalHost reads raw stdin and feeds bytes into a TerminalMMIO device.
// Only instantiated in main.go for interactive use - never in tests.
type TerminalHost struct {
	mmio         *TerminalMMIO
	stopCh       chan struct{}
	done         chan struct{}
	stopped      sync.Once
	fd           int
	oldTermState *term.State
}

// NewTerminalHost creates a host adapter that reads stdin into the given MMIO device.
func NewTerminalHost(mmio *TerminalMMIO) *TerminalHost {
	return &TerminalHost{
		mmio:   mmio,
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start sets stdin to raw mode and begins reading in a goroutine.
// Each byte is routed by mode to TERM_IN (line mode) or TERM_KEY_IN (char mode).
// Call Stop() to restore stdin.
func (h *TerminalHost) Start() {
	h.fd = int(os.Stdin.Fd())

	// Put terminal in raw mode to disable OS-level echo and line buffering.
	// The MMIO device handles echo itself via echoEnabled.
	oldState, err := term.MakeRaw(h.fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "terminal_host: failed to set raw mode: %v\n", err)
		close(h.done)
		return
	}
	h.oldTermState = oldState

	go func() {
		defer close(h.done)
		buf := make([]byte, 1)

		for {
			select {
			case <-h.stopCh:
				return
			default:
			}

			n, err := os.Stdin.Read(buf)
			if n > 0 {
				b := buf[0]
				// Raw mode sends CR for Enter; translate to LF for the MMIO device.
				if b == '\r' {
					b = '\n'
				}
				// Modern terminals send 0x7F (DEL) for Backspace; translate to 0x08 (BS).
				if b == 0x7F {
					b = 0x08
				}
				h.mmio.RouteHostKey(b)
			}
			if err != nil {
				return
			}
			if n == 0 {
				time.Sleep(5 * time.Millisecond)
			}
		}
	}()
}

// Stop terminates the stdin reading goroutine and restores terminal state.
func (h *TerminalHost) Stop() {
	h.stopped.Do(func() {
		close(h.stopCh)
	})
	<-h.done
	if h.oldTermState != nil {
		_ = term.Restore(h.fd, h.oldTermState)
		h.oldTermState = nil
	}
}

// PrintOutput drains the MMIO output buffer and prints it to stdout.
// Call this periodically from the main loop for interactive mode.
func (h *TerminalHost) PrintOutput() {
	out := h.mmio.DrainOutput()
	if len(out) > 0 {
		fmt.Print(out)
	}
}
