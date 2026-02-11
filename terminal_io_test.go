package main

import (
	"testing"
	"time"
)

// =============================================================================
// Phase 0 TDD Tests — TerminalMMIO
// =============================================================================

func TestTerminalMMIO_WriteChar(t *testing.T) {
	tm := NewTerminalMMIO()
	tm.HandleWrite(TERM_OUT, 0x41) // 'A'
	out := tm.DrainOutput()
	if out != "A" {
		t.Fatalf("expected output 'A', got %q", out)
	}
}

func TestTerminalMMIO_WriteMultipleChars(t *testing.T) {
	tm := NewTerminalMMIO()
	for _, ch := range "Hello" {
		tm.HandleWrite(TERM_OUT, uint32(ch))
	}
	out := tm.DrainOutput()
	if out != "Hello" {
		t.Fatalf("expected output 'Hello', got %q", out)
	}
}

func TestTerminalMMIO_StatusEmpty(t *testing.T) {
	tm := NewTerminalMMIO()
	status := tm.HandleRead(TERM_STATUS)
	if status&1 != 0 {
		t.Fatalf("expected bit 0 = 0 (no input), got 0x%X", status)
	}
}

func TestTerminalMMIO_StatusHasInput(t *testing.T) {
	tm := NewTerminalMMIO()
	tm.EnqueueByte('A')
	status := tm.HandleRead(TERM_STATUS)
	if status&1 != 1 {
		t.Fatalf("expected bit 0 = 1 (input available), got 0x%X", status)
	}
}

func TestTerminalMMIO_ReadChar(t *testing.T) {
	tm := NewTerminalMMIO()
	tm.EnqueueByte('A')
	val := tm.HandleRead(TERM_IN)
	if val != 0x41 {
		t.Fatalf("expected 0x41 ('A'), got 0x%X", val)
	}
}

func TestTerminalMMIO_ReadClearsQueue(t *testing.T) {
	tm := NewTerminalMMIO()
	tm.EnqueueByte('A')
	_ = tm.HandleRead(TERM_IN) // consume
	status := tm.HandleRead(TERM_STATUS)
	if status&1 != 0 {
		t.Fatalf("expected bit 0 = 0 after read, got 0x%X", status)
	}
}

func TestTerminalMMIO_ReadSequence(t *testing.T) {
	tm := NewTerminalMMIO()
	input := "HELLO"
	for _, ch := range input {
		tm.EnqueueByte(byte(ch))
	}
	var result []byte
	for i := 0; i < len(input); i++ {
		result = append(result, byte(tm.HandleRead(TERM_IN)))
	}
	if string(result) != input {
		t.Fatalf("expected %q, got %q", input, string(result))
	}
}

func TestTerminalMMIO_LineStatus(t *testing.T) {
	tm := NewTerminalMMIO()
	// No complete line yet
	ls := tm.HandleRead(TERM_LINE_STATUS)
	if ls&1 != 0 {
		t.Fatalf("expected no line available, got 0x%X", ls)
	}
	// Enqueue partial line
	for _, ch := range "ABC" {
		tm.EnqueueByte(byte(ch))
	}
	ls = tm.HandleRead(TERM_LINE_STATUS)
	if ls&1 != 0 {
		t.Fatalf("expected no line available (no newline yet), got 0x%X", ls)
	}
	// Complete the line
	tm.EnqueueByte('\n')
	ls = tm.HandleRead(TERM_LINE_STATUS)
	if ls&1 != 1 {
		t.Fatalf("expected line available after \\n, got 0x%X", ls)
	}
}

func TestTerminalMMIO_EchoControl(t *testing.T) {
	tm := NewTerminalMMIO()
	// Echo is enabled by default
	if !tm.echoEnabled {
		t.Fatal("expected echo enabled by default")
	}
	// Disable echo
	tm.HandleWrite(TERM_ECHO, 0)
	if tm.echoEnabled {
		t.Fatal("expected echo disabled after writing 0")
	}
	// Re-enable
	tm.HandleWrite(TERM_ECHO, 1)
	if !tm.echoEnabled {
		t.Fatal("expected echo re-enabled after writing 1")
	}
}

func TestTerminalMMIO_EchoWritesToOutput(t *testing.T) {
	tm := NewTerminalMMIO()
	// EnqueueByte does NOT echo — echo is the application's responsibility.
	tm.EnqueueByte('X')
	out := tm.DrainOutput()
	if out != "" {
		t.Fatalf("expected no echo from EnqueueByte, got %q", out)
	}
	// Same with echo disabled
	tm.HandleWrite(TERM_ECHO, 0)
	tm.EnqueueByte('Y')
	out = tm.DrainOutput()
	if out != "" {
		t.Fatalf("expected no echo when disabled, got %q", out)
	}
}

func TestTerminalMMIO_ReadFromEmptyReturnsZero(t *testing.T) {
	tm := NewTerminalMMIO()
	val := tm.HandleRead(TERM_IN)
	if val != 0 {
		t.Fatalf("expected 0 from empty input, got 0x%X", val)
	}
}

func TestTerminalMMIO_OutputReady(t *testing.T) {
	tm := NewTerminalMMIO()
	status := tm.HandleRead(TERM_STATUS)
	// Bit 1 should always be 1 (output ready — we never block)
	if status&2 != 2 {
		t.Fatalf("expected bit 1 = 1 (output ready), got 0x%X", status)
	}
}

func TestTerminalMMIO_BusIntegration(t *testing.T) {
	bus := NewMachineBus()
	tm := NewTerminalMMIO()
	bus.MapIO(TERM_OUT, TERMINAL_REGION_END, tm.HandleRead, tm.HandleWrite)

	// Write char via bus
	bus.Write32(TERM_OUT, 0x42) // 'B'
	out := tm.DrainOutput()
	if out != "B" {
		t.Fatalf("expected 'B' via bus write, got %q", out)
	}

	// Enqueue and read via bus
	tm.EnqueueByte('Z')
	status := bus.Read32(TERM_STATUS)
	if status&1 != 1 {
		t.Fatalf("expected input available via bus, got 0x%X", status)
	}
	val := bus.Read32(TERM_IN)
	if val != 0x5A { // 'Z'
		t.Fatalf("expected 0x5A ('Z') via bus, got 0x%X", val)
	}
}

func TestTerminalMMIO_RingBufferWrap(t *testing.T) {
	tm := NewTerminalMMIO()
	// Disable echo so output doesn't fill up
	tm.HandleWrite(TERM_ECHO, 0)
	// Fill and drain the ring buffer multiple times to test wrap-around
	for round := 0; round < 3; round++ {
		for i := 0; i < 128; i++ {
			tm.EnqueueByte(byte(i + 1))
		}
		for i := 0; i < 128; i++ {
			val := tm.HandleRead(TERM_IN)
			expected := uint32(i + 1)
			if val != expected {
				t.Fatalf("round %d, byte %d: expected 0x%X, got 0x%X", round, i, expected, val)
			}
		}
		// Should be empty now
		status := tm.HandleRead(TERM_STATUS)
		if status&1 != 0 {
			t.Fatalf("round %d: expected empty after drain, got 0x%X", round, status)
		}
	}
}

func TestTerminalMMIO_SentinelTrigger(t *testing.T) {
	tm := NewTerminalMMIO()
	callbackFired := false
	tm.OnSentinel(func() { callbackFired = true })

	// Not triggered initially
	if tm.SentinelTriggered.Load() {
		t.Fatal("expected sentinel not triggered initially")
	}
	// Writing wrong value does nothing
	tm.HandleWrite(TERM_SENTINEL, 0x1234)
	if tm.SentinelTriggered.Load() {
		t.Fatal("expected sentinel not triggered for wrong value")
	}
	if callbackFired {
		t.Fatal("expected callback not fired for wrong value")
	}
	// Writing 0xDEAD triggers flag and callback
	tm.HandleWrite(TERM_SENTINEL, 0xDEAD)
	if !tm.SentinelTriggered.Load() {
		t.Fatal("expected sentinel triggered after writing 0xDEAD")
	}
	if !callbackFired {
		t.Fatal("expected callback fired after writing 0xDEAD")
	}
}

func TestTerminalMMIO_LineStatusClearsAfterConsume(t *testing.T) {
	tm := NewTerminalMMIO()
	tm.HandleWrite(TERM_ECHO, 0) // disable echo for clarity
	for _, ch := range "ABC\n" {
		tm.EnqueueByte(byte(ch))
	}
	// Line available
	ls := tm.HandleRead(TERM_LINE_STATUS)
	if ls&1 != 1 {
		t.Fatal("expected line available")
	}
	// Consume all 4 bytes including newline
	for i := 0; i < 4; i++ {
		tm.HandleRead(TERM_IN)
	}
	// No more line
	ls = tm.HandleRead(TERM_LINE_STATUS)
	if ls&1 != 0 {
		t.Fatal("expected no line after consuming all chars")
	}
}

func TestTerminalMMIO_CharOutputCallback(t *testing.T) {
	tm := NewTerminalMMIO()
	got := byte(0)
	called := false
	tm.SetCharOutputCallback(func(b byte) {
		called = true
		got = b
	})

	tm.HandleWrite(TERM_OUT, 'A')

	if !called {
		t.Fatal("expected callback to be called")
	}
	if got != 'A' {
		t.Fatalf("expected callback byte 'A', got %q", got)
	}
	if out := tm.DrainOutput(); out != "" {
		t.Fatalf("expected no buffered output with callback set, got %q", out)
	}
}

func TestTerminalMMIO_NilCallbackOutputBuf(t *testing.T) {
	tm := NewTerminalMMIO()
	tm.SetCharOutputCallback(nil)

	tm.HandleWrite(TERM_OUT, 'B')
	if out := tm.DrainOutput(); out != "B" {
		t.Fatalf("expected buffered output 'B', got %q", out)
	}
}

func TestTerminalMMIO_CallbackNoDeadlock(t *testing.T) {
	tm := NewTerminalMMIO()
	done := make(chan struct{})
	tm.SetCharOutputCallback(func(_ byte) {
		// Re-enter terminal API while callback runs.
		tm.EnqueueByte('x')
		close(done)
	})

	tm.HandleWrite(TERM_OUT, 'C')

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("callback did not complete; possible deadlock")
	}
}

func TestTerminalMMIO_StatusReadTimestamp(t *testing.T) {
	tm := NewTerminalMMIO()
	before := time.Now()
	_ = tm.HandleRead(TERM_STATUS)
	got := tm.LastStatusReadTime()
	if got.IsZero() {
		t.Fatal("expected non-zero status read timestamp")
	}
	if got.Before(before.Add(-50*time.Millisecond)) || got.After(time.Now().Add(50*time.Millisecond)) {
		t.Fatalf("expected recent status read timestamp, got %v", got)
	}
}
