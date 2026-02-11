package main

import (
	"sync"
	"sync/atomic"
	"time"
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
	echoEnabled   bool
	forceEchoOff  bool
	lineInputMode bool

	// Raw key ring buffer for per-keystroke input (GET).
	rawKeyBuf  [256]byte
	rawKeyHead int
	rawKeyTail int
	rawKeyLen  int

	// SentinelTriggered is set when TERM_SENTINEL receives 0xDEAD.
	SentinelTriggered atomic.Bool

	// onSentinel is called (if non-nil) when TERM_SENTINEL is triggered.
	// Typically wired to stop the CPU.
	onSentinel func()

	// onCharOutput, when set, receives TERM_OUT bytes immediately.
	// Callback is invoked outside tm.mu to avoid deadlocks/re-entrancy issues.
	onCharOutput func(byte)

	// lastStatusRead stores unix nanos of the latest TERM_STATUS read.
	lastStatusRead atomic.Int64
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

// SetCharOutputCallback registers a callback for TERM_OUT writes.
// When set, TERM_OUT bytes are delivered directly to fn and not buffered in outputBuf.
func (tm *TerminalMMIO) SetCharOutputCallback(fn func(byte)) {
	tm.mu.Lock()
	tm.onCharOutput = fn
	tm.mu.Unlock()
}

// LastStatusReadTime returns the most recent TERM_STATUS read time.
func (tm *TerminalMMIO) LastStatusReadTime() time.Time {
	nanos := tm.lastStatusRead.Load()
	if nanos <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos)
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
		tm.lastStatusRead.Store(time.Now().UnixNano())
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
		b := tm.dequeueInputByteLocked()
		return uint32(b)

	case TERM_LINE_STATUS:
		var status uint32
		if tm.newlines > 0 {
			status |= 1 // bit 0: complete line available
		}
		return status

	case TERM_ECHO:
		if tm.forceEchoOff {
			return 0
		}
		if tm.echoEnabled {
			return 1
		}
		return 0
	case TERM_KEY_STATUS:
		if tm.rawKeyLen > 0 {
			return 1
		}
		return 0
	case TERM_KEY_IN:
		if tm.rawKeyLen == 0 {
			return 0
		}
		b := tm.rawKeyBuf[tm.rawKeyHead]
		tm.rawKeyHead = (tm.rawKeyHead + 1) % len(tm.rawKeyBuf)
		tm.rawKeyLen--
		return uint32(b)
	case TERM_CTRL:
		if tm.lineInputMode {
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
	var charFn func(byte)
	var charArg byte

	tm.mu.Lock()
	switch addr {
	case TERM_OUT, TERM_OUT_16BIT, TERM_OUT_SIGNEXT:
		ch := byte(value & 0xFF)
		if tm.onCharOutput != nil {
			charFn = tm.onCharOutput
			charArg = ch
		} else {
			tm.outputBuf = append(tm.outputBuf, ch)
		}

	case TERM_ECHO:
		tm.echoEnabled = (value & 1) != 0
	case TERM_CTRL:
		tm.lineInputMode = (value & 1) != 0

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
	if charFn != nil {
		charFn(charArg)
	}
}

// EnqueueByte adds a byte to the input ring buffer.
func (tm *TerminalMMIO) EnqueueByte(b byte) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.enqueueInputByteLocked(b)
	// No echo here — echo is the application's responsibility (e.g. read_line).
	// EnqueueByte is a transport layer; echoing here causes double-echo when
	// the application (EhBASIC) also echoes characters it reads from TERM_IN.
}

func (tm *TerminalMMIO) EnqueueRawKey(b byte) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.enqueueRawKeyLocked(b)
}

// RouteHostKey atomically checks line mode and routes the key to exactly one queue.
func (tm *TerminalMMIO) RouteHostKey(b byte) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.lineInputMode {
		tm.enqueueInputByteLocked(b)
		return
	}
	tm.enqueueRawKeyLocked(b)
}

func (tm *TerminalMMIO) LineInputMode() bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.lineInputMode
}

func (tm *TerminalMMIO) SetForceEchoOff(force bool) {
	tm.mu.Lock()
	tm.forceEchoOff = force
	tm.mu.Unlock()
}

// DrainOutput returns and clears the accumulated output buffer.
func (tm *TerminalMMIO) DrainOutput() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	s := string(tm.outputBuf)
	tm.outputBuf = tm.outputBuf[:0]
	return s
}

func (tm *TerminalMMIO) enqueueInputByteLocked(b byte) {
	if tm.inputLen >= len(tm.inputBuf) {
		return
	}
	tm.inputBuf[tm.inputTail] = b
	tm.inputTail = (tm.inputTail + 1) % len(tm.inputBuf)
	tm.inputLen++
	if b == '\n' {
		tm.newlines++
	}
}

func (tm *TerminalMMIO) dequeueInputByteLocked() byte {
	b := tm.inputBuf[tm.inputHead]
	tm.inputHead = (tm.inputHead + 1) % len(tm.inputBuf)
	tm.inputLen--
	if b == '\n' {
		tm.newlines--
	}
	return b
}

func (tm *TerminalMMIO) enqueueRawKeyLocked(b byte) {
	if tm.rawKeyLen >= len(tm.rawKeyBuf) {
		return
	}
	tm.rawKeyBuf[tm.rawKeyTail] = b
	tm.rawKeyTail = (tm.rawKeyTail + 1) % len(tm.rawKeyBuf)
	tm.rawKeyLen++
}
