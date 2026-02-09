package main

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"
)

// TerminalHost reads raw stdin and feeds bytes into a TerminalMMIO device.
// Only instantiated in main.go for interactive use — never in tests.
type TerminalHost struct {
	mmio        *TerminalMMIO
	stopCh      chan struct{}
	done        chan struct{}
	stopped     sync.Once
	fd          int
	nonblockSet bool
}

// NewTerminalHost creates a host adapter that reads stdin into the given MMIO device.
func NewTerminalHost(mmio *TerminalMMIO) *TerminalHost {
	return &TerminalHost{
		mmio:   mmio,
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
}

// Start sets stdin to non-blocking mode and begins reading in a goroutine.
// Each byte is fed to mmio.EnqueueByte(). Call Stop() to restore stdin.
func (h *TerminalHost) Start() {
	h.fd = int(os.Stdin.Fd())
	if err := syscall.SetNonblock(h.fd, true); err != nil {
		fmt.Fprintf(os.Stderr, "terminal_host: failed to set nonblocking stdin: %v\n", err)
		close(h.done)
		return
	}
	h.nonblockSet = true

	go func() {
		defer close(h.done)
		buf := make([]byte, 1)

		for {
			select {
			case <-h.stopCh:
				return
			default:
			}

			n, err := syscall.Read(h.fd, buf)
			if n > 0 {
				h.mmio.EnqueueByte(buf[0])
			}
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				time.Sleep(5 * time.Millisecond)
				continue
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

// Stop terminates the stdin reading goroutine and restores stdin to blocking mode.
func (h *TerminalHost) Stop() {
	h.stopped.Do(func() {
		close(h.stopCh)
	})
	<-h.done
	if h.nonblockSet {
		_ = syscall.SetNonblock(h.fd, false)
		h.nonblockSet = false
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
