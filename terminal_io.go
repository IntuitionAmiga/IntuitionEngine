package main

import (
	"sync"
	"sync/atomic"
)

// TerminalMMIO is a pure state-machine terminal device for MMIO register access.
// It owns an input ring buffer, output buffer, status bits, and echo flag.
// Tests inject characters via EnqueueByte(); the host adapter (TerminalHost)
// feeds stdin bytes through the same method.
type TerminalMMIO struct {
	mu sync.Mutex

	// Input ring buffer
	inputBuf  [1024]byte
	inputHead int // next read position
	inputTail int // next write position
	inputLen  int // number of bytes in buffer
	newlines  int // count of '\n' in buffer (for TERM_LINE_STATUS)

	// Output buffer (drained by tests or host adapter)
	outputBuf []byte

	// Echo flag: readable by application code via TERM_ECHO register.
	// The application (e.g. read_line) decides whether to echo based on this.
	echoEnabled bool

	// SentinelTriggered is set when TERM_SENTINEL receives 0xDEAD.
	SentinelTriggered atomic.Bool

	// onSentinel is called (if non-nil) when TERM_SENTINEL is triggered.
	// Typically wired to stop the CPU.
	onSentinel func()
}

// NewTerminalMMIO creates a new terminal MMIO device with echo enabled.
func NewTerminalMMIO() *TerminalMMIO {
	return &TerminalMMIO{
		echoEnabled: true,
		outputBuf:   make([]byte, 0, 256),
	}
}

// OnSentinel registers a callback invoked when TERM_SENTINEL receives 0xDEAD.
// Typically used to stop the CPU: tm.OnSentinel(func() { cpu.running.Store(false) })
func (tm *TerminalMMIO) OnSentinel(fn func()) {
	tm.onSentinel = fn
}

// HandleRead processes reads from terminal registers.
func (tm *TerminalMMIO) HandleRead(addr uint32) uint32 {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	switch addr {
	case TERM_OUT:
		// Write-only register; reading returns 0
		return 0

	case TERM_STATUS:
		var status uint32
		if tm.inputLen > 0 {
			status |= 1 // bit 0: input available
		}
		status |= 2 // bit 1: output ready (always)
		return status

	case TERM_IN:
		if tm.inputLen == 0 {
			return 0
		}
		b := tm.inputBuf[tm.inputHead]
		tm.inputHead = (tm.inputHead + 1) % len(tm.inputBuf)
		tm.inputLen--
		if b == '\n' {
			tm.newlines--
		}
		return uint32(b)

	case TERM_LINE_STATUS:
		var status uint32
		if tm.newlines > 0 {
			status |= 1 // bit 0: complete line available
		}
		return status

	case TERM_ECHO:
		if tm.echoEnabled {
			return 1
		}
		return 0

	default:
		return 0
	}
}

// HandleWrite processes writes to terminal registers.
func (tm *TerminalMMIO) HandleWrite(addr uint32, value uint32) {
	var sentinelFn func()

	tm.mu.Lock()
	switch addr {
	case TERM_OUT, TERM_OUT_16BIT, TERM_OUT_SIGNEXT:
		ch := byte(value & 0xFF)
		tm.outputBuf = append(tm.outputBuf, ch)

	case TERM_ECHO:
		tm.echoEnabled = (value & 1) != 0

	case TERM_SENTINEL:
		if value == 0xDEAD {
			tm.SentinelTriggered.Store(true)
			sentinelFn = tm.onSentinel
		}
	}
	tm.mu.Unlock()

	if sentinelFn != nil {
		sentinelFn()
	}
}

// EnqueueByte adds a byte to the input ring buffer.
func (tm *TerminalMMIO) EnqueueByte(b byte) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.inputLen >= len(tm.inputBuf) {
		return // buffer full — drop
	}
	tm.inputBuf[tm.inputTail] = b
	tm.inputTail = (tm.inputTail + 1) % len(tm.inputBuf)
	tm.inputLen++
	if b == '\n' {
		tm.newlines++
	}
	// No echo here — echo is the application's responsibility (e.g. read_line).
	// EnqueueByte is a transport layer; echoing here causes double-echo when
	// the application (EhBASIC) also echoes characters it reads from TERM_IN.
}

// DrainOutput returns and clears the accumulated output buffer.
func (tm *TerminalMMIO) DrainOutput() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	s := string(tm.outputBuf)
	tm.outputBuf = tm.outputBuf[:0]
	return s
}
