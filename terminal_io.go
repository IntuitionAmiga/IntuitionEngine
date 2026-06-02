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

	// Mouse state, updated by graphical backends and read via MMIO.
	mouseX            atomic.Int32
	mouseY            atomic.Int32
	mouseDX           atomic.Int32
	mouseDY           atomic.Int32
	mouseButtons      atomic.Uint32
	mouseChanged      atomic.Bool
	mouseCtrl         atomic.Uint32
	mouseOverride     atomic.Bool  // when true, backend skips mouse updates (script owns mouse)
	mouseNativeW      atomic.Int32 // native video source width (0 = use raw coordinates)
	mouseNativeH      atomic.Int32 // native video source height
	mouseNativeLocked atomic.Bool  // when true, video mode changes do not alter mouseNativeW/H

	// Scancode ring buffer for raw keyboard make/break events.
	scanBuf  [256]uint8
	scanHead int
	scanTail int
	scanLen  int
	// Bit 0=shift, 1=ctrl, 2=alt, 3=capslock
	modifiers atomic.Uint32

	// amigaScancodeMode selects Amiga rawkey encoding for the scancode ring buffer.
	// When true, the Ebiten backend enqueues Amiga rawkeys instead of PC/AT scancodes.
	amigaScancodeMode atomic.Bool

	// SentinelTriggered is set when TERM_SENTINEL receives 0xDEAD.
	SentinelTriggered atomic.Bool

	// onSentinel is called (if non-nil) when TERM_SENTINEL is triggered.
	// Typically wired to stop the CPU.
	onSentinel func()

	// onCharOutput, when set, receives TERM_OUT bytes immediately.
	// Callback is invoked outside tm.mu to avoid deadlocks/re-entrancy issues.
	onCharOutput func(byte)

	// hostKeyInterceptor, when set, can consume host-originated keystrokes before
	// they enter the guest terminal queues.
	hostKeyInterceptor func(byte) bool

	// lastStatusRead stores unix nanos of the latest TERM_STATUS read.
	lastStatusRead atomic.Int64

	// monoStart is the monotonic epoch for RTC_MONO_USEC_*.
	monoStart time.Time
}

// TerminalMMIOSetter allows video backends to receive a terminal MMIO pointer.
type TerminalMMIOSetter interface {
	SetTerminalMMIO(tm *TerminalMMIO)
}

// NewTerminalMMIO creates a new terminal MMIO device with echo enabled.
// lineInputMode defaults to true so keys typed before read_line sets TERM_CTRL
// are routed to TERM_IN (where read_line reads), not TERM_KEY_IN.
func NewTerminalMMIO() *TerminalMMIO {
	return &TerminalMMIO{
		echoEnabled:   true,
		lineInputMode: true,
		outputBuf:     make([]byte, 0, 256),
		monoStart:     time.Now(),
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

func (tm *TerminalMMIO) SetHostKeyInterceptor(fn func(byte) bool) {
	tm.mu.Lock()
	tm.hostKeyInterceptor = fn
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
	case MOUSE_X:
		return uint32(tm.mouseX.Load())
	case MOUSE_Y:
		return uint32(tm.mouseY.Load())
	case MOUSE_BUTTONS:
		return tm.mouseButtons.Load()
	case MOUSE_STATUS:
		if tm.mouseChanged.Swap(false) {
			return 1
		}
		return 0
	case MOUSE_CTRL:
		return tm.mouseCtrl.Load()
	case MOUSE_DX:
		return uint32(tm.mouseDX.Swap(0))
	case MOUSE_DY:
		return uint32(tm.mouseDY.Swap(0))
	case SCAN_CODE:
		return uint32(tm.dequeueScancodeLocked())
	case SCAN_STATUS:
		if tm.scanLen > 0 {
			return 1
		}
		return 0
	case SCAN_MODIFIERS:
		return tm.modifiers.Load()

	case RTC_EPOCH:
		return uint32(time.Now().Unix())
	case RTC_MONO_USEC_LO:
		return uint32(tm.monotonicUsecLocked())
	case RTC_MONO_USEC_HI:
		return uint32(tm.monotonicUsecLocked() >> 32)

	default:
		return 0
	}
}

func (tm *TerminalMMIO) monotonicUsecLocked() uint64 {
	if tm.monoStart.IsZero() {
		tm.monoStart = time.Now()
		return 0
	}
	return uint64(time.Since(tm.monoStart).Microseconds())
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
	case MOUSE_CTRL:
		old := tm.mouseCtrl.Swap(value & 1)
		if old != (value & 1) {
			tm.ClearMouseDeltas()
		}

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
// InputPending reports the number of queued input bytes not yet consumed by
// the guest. Used by tests to wait for the REPL to drain a submitted line
// without a fixed sleep.
func (tm *TerminalMMIO) InputPending() int {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.inputLen
}

func (tm *TerminalMMIO) EnqueueByte(b byte) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.enqueueInputByteLocked(b)
	// No echo here - echo is the application's responsibility (e.g. read_line).
	// EnqueueByte is a transport layer; echoing here causes double-echo when
	// the application (EhBASIC) also echoes characters it reads from TERM_IN.
}

func (tm *TerminalMMIO) EnqueueRawKey(b byte) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.enqueueRawKeyLocked(b)
}

func (tm *TerminalMMIO) EnqueueScancode(code uint8) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.scanLen >= len(tm.scanBuf) {
		return
	}
	tm.scanBuf[tm.scanTail] = code
	tm.scanTail = (tm.scanTail + 1) % len(tm.scanBuf)
	tm.scanLen++
}

// RouteHostKey atomically checks line mode and routes the key to exactly one queue.
func (tm *TerminalMMIO) RouteHostKey(b byte) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.hostKeyInterceptor != nil && tm.hostKeyInterceptor(b) {
		return
	}
	if tm.lineInputMode {
		tm.enqueueInputByteLocked(b)
		return
	}
	tm.enqueueRawKeyLocked(b)
}

// RouteGraphicalKey atomically checks line mode and, if in char mode, enqueues
// the key to the raw key buffer. Returns the line mode state so the caller can
// decide how to handle the key visually. This prevents the race where
// LineInputMode() and EnqueueRawKey() are separate lock acquisitions.
func (tm *TerminalMMIO) RouteGraphicalKey(b byte) (lineMode bool) {
	tm.mu.Lock()
	lineMode = tm.lineInputMode
	if !lineMode {
		tm.enqueueRawKeyLocked(b)
	}
	tm.mu.Unlock()
	return lineMode
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

// SetMouseNativeResolution sets the guest mouse coordinate space. When set
// (w > 0), backends scale display-space coordinates before writing MMIO.
func (tm *TerminalMMIO) SetMouseNativeResolution(w, h int) {
	if tm.mouseNativeLocked.Load() {
		return
	}
	tm.mouseNativeW.Store(int32(w))
	tm.mouseNativeH.Store(int32(h))
}

// LockMouseNativeResolution pins the guest mouse coordinate space until
// UnlockMouseNativeResolution is called. This is used by guests whose cursor
// contract is not the currently composited native video source.
func (tm *TerminalMMIO) LockMouseNativeResolution(w, h int) {
	tm.mouseNativeW.Store(int32(w))
	tm.mouseNativeH.Store(int32(h))
	tm.mouseNativeLocked.Store(true)
}

func (tm *TerminalMMIO) UnlockMouseNativeResolution() {
	tm.mouseNativeLocked.Store(false)
}

// MouseRelativeMode reports whether the guest requested captured relative mouse input.
func (tm *TerminalMMIO) MouseRelativeMode() bool {
	return tm.mouseCtrl.Load()&1 != 0
}

// AddMouseDelta accumulates signed relative mouse motion for MOUSE_DX/Y.
func (tm *TerminalMMIO) AddMouseDelta(dx, dy int32) {
	if dx == 0 && dy == 0 {
		return
	}
	tm.mouseDX.Add(dx)
	tm.mouseDY.Add(dy)
	tm.mouseChanged.Store(true)
}

// ClearMouseDeltas drops any pending relative motion.
func (tm *TerminalMMIO) ClearMouseDeltas() {
	tm.mouseDX.Store(0)
	tm.mouseDY.Store(0)
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

func (tm *TerminalMMIO) dequeueScancodeLocked() uint8 {
	if tm.scanLen == 0 {
		return 0
	}
	code := tm.scanBuf[tm.scanHead]
	tm.scanHead = (tm.scanHead + 1) % len(tm.scanBuf)
	tm.scanLen--
	return code
}
