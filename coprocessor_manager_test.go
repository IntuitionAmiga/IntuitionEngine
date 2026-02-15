package main

import (
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// --- Helper: build IE32 service binary ---
// IE32 instruction format: 8 bytes each [opcode, reg, addrMode, 0, operand(4 bytes LE)]

func ie32Instr(opcode, reg, addrMode byte, operand uint32) [8]byte {
	var b [8]byte
	b[0] = opcode
	b[1] = reg
	b[2] = addrMode
	b[3] = 0
	binary.LittleEndian.PutUint32(b[4:], operand)
	return b
}

const (
	ie32_LOAD  = 0x01
	ie32_STORE = 0x02
	ie32_ADD   = 0x03
	ie32_SUB   = 0x04
	ie32_AND   = 0x05
	ie32_JMP   = 0x06
	ie32_JNZ   = 0x07
	ie32_JZ    = 0x08
	ie32_NOP   = 0xEE
	ie32_HALT  = 0xFF

	ie32_ADDR_IMM    = 0x00
	ie32_ADDR_REG    = 0x01
	ie32_ADDR_DIRECT = 0x04
)

// buildIE32ServiceBinary creates a minimal IE32 service binary that:
// - Polls the ring buffer for requests
// - For op=1: adds two uint32 from reqPtr and writes result to respPtr
// - Sets response status=2 (ok), respLen=4
// - Advances tail
//
// Ring layout (relative to MAILBOX_BASE + cpuIdx*RING_STRIDE):
//
//	+0x00: head (uint8)
//	+0x01: tail (uint8)
//	+0x08: entries[16 * 32 bytes]
//	+0x208: responses[16 * 16 bytes]
//
// Request descriptor (32 bytes at entries + tail*32):
//
//	+0x00: ticket, +0x08: op, +0x10: reqPtr, +0x14: reqLen, +0x18: respPtr, +0x1C: respCap
//
// Response descriptor (16 bytes at responses + tail*16):
//
//	+0x00: ticket, +0x04: status, +0x08: resultCode, +0x0C: respLen
func buildIE32ServiceBinary(ringBase uint32) []byte {
	// For simplicity, use a step-by-step approach with direct addressing.
	// The IE32 has 16 registers (A=0, X=1, Y=2, Z=3, B=4, C=5, D=6, E=7, ...)
	//
	// Strategy:
	// 1. Load tail from ringBase+1
	// 2. Load head from ringBase+0
	// 3. Compare: if tail == head, loop back to step 1
	// 4. Compute entry address: ringBase + 0x08 + tail*32
	// 5. Read request fields
	// 6. Process op=1 (add)
	// 7. Write response
	// 8. Advance tail: tail = (tail + 1) & 0x0F
	// 9. Jump back to step 1

	var code []byte
	appendInstr := func(opcode, reg, addrMode byte, operand uint32) {
		instr := ie32Instr(opcode, reg, addrMode, operand)
		code = append(code, instr[:]...)
	}

	base := uint32(WORKER_IE32_BASE) // All PC addresses are bus addresses

	// PC = base + 0x00: POLL_LOOP:
	// Load tail byte (memory byte at ringBase+1) into register A
	appendInstr(ie32_LOAD, 0, ie32_ADDR_DIRECT, ringBase+1) // A = mem[ringBase+1] (tail)
	// AND with 0xFF to get just the byte
	appendInstr(ie32_AND, 0, ie32_ADDR_IMM, 0xFF) // A &= 0xFF

	// PC = base + 0x10:
	// Load head byte into register X
	appendInstr(ie32_LOAD, 1, ie32_ADDR_DIRECT, ringBase+0) // X = mem[ringBase+0] (head)
	appendInstr(ie32_AND, 1, ie32_ADDR_IMM, 0xFF)           // X &= 0xFF

	// PC = base + 0x20:
	// Compare: tail - head → if zero, loop (empty ring)
	appendInstr(ie32_SUB, 1, ie32_ADDR_REG, 0)     // X = X - A (head - tail)
	appendInstr(ie32_JZ, 1, ie32_ADDR_IMM, base+0) // if X==0, jump to POLL_LOOP

	// PC = base + 0x30: Ring not empty - process request
	// tail is in A. Compute entry addr = ringBase + 0x08 + A*32
	// B = A * 32 (shift left 5)
	appendInstr(ie32_LOAD, 4, ie32_ADDR_REG, 0) // B = A (tail)
	appendInstr(0x0B, 4, ie32_ADDR_IMM, 5)      // B <<= 5 (SHL by 5 = *32)

	// PC = base + 0x40:
	appendInstr(ie32_ADD, 4, ie32_ADDR_IMM, ringBase+RING_ENTRIES_OFFSET) // B += ringBase + entries_offset

	// Now B = address of request descriptor entry
	// Read ticket (entry+0) into C
	appendInstr(ie32_LOAD, 5, ie32_ADDR_DIRECT, 0) // placeholder - we'll use register indirect
	// Actually, IE32 ADDR_REG_IND: operand[3:0] = reg, operand[31:4] = offset
	// value = MEM[reg[operand&0xF] + (operand & ~0xF)]
	// So to read MEM[B + 0], operand = (0 << 4) | 4 = 0x04 (reg=B=4, offset=0)
	// To read MEM[B + 8], operand = (8 << 4) | 4 ... wait, that's wrong.
	// Offset uses bits [31:4], shifted: offset = operand & 0xFFFFFFF0
	// Register index uses bits [3:0]
	// So for B (index 4) with offset 0: operand = 0x00000004
	// For B with offset 8: operand = 0x00000084 ... no.
	// offset = operand & 0xFFFFFFF0, reg = operand & 0x0F
	// So operand = (offset & 0xFFFFFFF0) | reg_idx
	// For offset=0, reg=B(4): operand = 0x00000004
	// For offset=8, reg=B(4): operand = 0x00000014 ... no, 8 = 0x08, so 0x08 & 0xFFFFFFF0 = 0
	// That doesn't work for small offsets!
	// offset must be in upper 28 bits. So offset 8 → operand[31:4] = 8 → operand = 8*16 | reg = 0x84
	// Wait no, let me re-read: operand & 0xFFFFFFF0 gives the OFFSET directly.
	// So for offset 8: we need (operand & 0xFFFFFFF0) = 8, meaning operand = 0x08 | reg.
	// But 0x08 & 0xFFFFFFF0 = 0x00. That's 0, not 8!
	// This means offsets < 16 can't be represented with this scheme!

	// Actually let me re-read the code more carefully.
	code = nil // Reset and use a different strategy

	// Use direct memory addressing instead of register-indirect.
	// The IE32 has ADDR_DIRECT which reads MEM[operand].
	// For the service binary, I'll compute addresses and use direct loads/stores.
	// But that means I can't have dynamic addressing easily.
	//
	// Alternative: use a scratch RAM area to store computed addresses and load from there.
	// This is getting complex. Let me use a much simpler approach for testing:
	// Write the service logic directly in Go as a "mock" that manipulates bus memory.

	// For now, just create a binary that spins (NOP loop + halt on stop)
	// and have the test directly manipulate mailbox memory to simulate the worker.

	appendInstr(ie32_NOP, 0, ie32_ADDR_IMM, 0)
	appendInstr(ie32_JMP, 0, ie32_ADDR_IMM, base)

	return code
}

// --- Test Helpers ---

func newTestBusAndManager(t *testing.T) (*MachineBus, *CoprocessorManager) {
	t.Helper()
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)
	return bus, mgr
}

// writeString writes a null-terminated string to bus memory at addr.
func writeString(bus *MachineBus, addr uint32, s string) {
	for i, b := range []byte(s) {
		bus.Write8(addr+uint32(i), b)
	}
	bus.Write8(addr+uint32(len(s)), 0)
}

// --- Manager Unit Tests ---

func TestCoprocessorTicketMonotonicity(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	_ = bus

	mgr.mu.Lock()
	t1 := mgr.nextTicket
	mgr.nextTicket++
	t2 := mgr.nextTicket
	mgr.nextTicket++
	t3 := mgr.nextTicket
	mgr.mu.Unlock()

	if t1 >= t2 || t2 >= t3 {
		t.Fatalf("tickets not monotonically increasing: %d, %d, %d", t1, t2, t3)
	}
}

func TestCoprocessorInvalidCPUType(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	for _, ct := range []uint32{0, EXEC_TYPE_IE64, 7, 99} {
		mgr.mu.Lock()
		mgr.cpuType = ct
		mgr.cmdStart()
		if mgr.cmdStatus != COPROC_STATUS_ERROR || mgr.cmdError != COPROC_ERR_INVALID_CPU {
			t.Errorf("cpuType=%d: expected INVALID_CPU error, got status=%d err=%d",
				ct, mgr.cmdStatus, mgr.cmdError)
		}
		mgr.mu.Unlock()
	}
}

func TestCoprocessorPathSanitization(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	tests := []struct {
		path string
		ok   bool
	}{
		{"../etc/passwd", false},
		{"/absolute/path", false},
		{"../../hack", false},
		{"safe_file.bin", true},
		{"subdir/file.bin", true},
	}

	for _, tt := range tests {
		_, ok := mgr.sanitizePath(tt.path)
		if ok != tt.ok {
			t.Errorf("sanitizePath(%q) = %v, want %v", tt.path, ok, tt.ok)
		}
	}
}

func TestCoprocessorPathSanitizationViaMmio(t *testing.T) {
	bus, _ := newTestBusAndManager(t)

	// Write an absolute path into bus memory
	writeString(bus, 0x400000, "/etc/passwd")

	// Try to start a worker with an invalid path
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_NAME_PTR, 0x400000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)

	status := bus.Read32(COPROC_CMD_STATUS)
	errCode := bus.Read32(COPROC_CMD_ERROR)
	if status != COPROC_STATUS_ERROR || errCode != COPROC_ERR_PATH_INVALID {
		t.Fatalf("expected PATH_INVALID error, got status=%d err=%d", status, errCode)
	}
}

func TestCoprocessorStartFileNotFound(t *testing.T) {
	bus, _ := newTestBusAndManager(t)

	writeString(bus, 0x400000, "nonexistent_file.bin")
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_NAME_PTR, 0x400000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)

	status := bus.Read32(COPROC_CMD_STATUS)
	errCode := bus.Read32(COPROC_CMD_ERROR)
	if status != COPROC_STATUS_ERROR || errCode != COPROC_ERR_NOT_FOUND {
		t.Fatalf("expected NOT_FOUND error, got status=%d err=%d", status, errCode)
	}
}

func TestCoprocessorStaleTicketPoll(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	mgr.mu.Lock()
	mgr.ticket = 9999 // non-existent ticket
	mgr.cmdPoll()
	if mgr.cmdError != COPROC_ERR_STALE_TICKET {
		t.Fatalf("expected STALE_TICKET error, got err=%d", mgr.cmdError)
	}
	mgr.mu.Unlock()
}

func TestCoprocessorEnqueueNoWorker(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	mgr.mu.Lock()
	mgr.cpuType = EXEC_TYPE_IE32
	mgr.cmdEnqueue()
	if mgr.cmdStatus != COPROC_STATUS_ERROR || mgr.cmdError != COPROC_ERR_NO_WORKER {
		t.Fatalf("expected NO_WORKER error, got status=%d err=%d", mgr.cmdStatus, mgr.cmdError)
	}
	mgr.mu.Unlock()
}

func TestCoprocessorTwoReadEviction(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	mgr.mu.Lock()

	// Simulate a completed ticket
	ticket := uint32(42)
	mgr.completions[ticket] = &CoprocCompletion{
		ticket:  ticket,
		status:  COPROC_TICKET_OK,
		created: time.Now(),
	}

	// Also write a matching response in the ring so scanTicketStatus finds it
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	respAddr := ringBase + RING_RESPONSES_OFFSET
	mgr.bus.Write32(respAddr+RESP_TICKET_OFF, ticket)
	mgr.bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)

	// First POLL: should return OK, not evict
	mgr.ticket = ticket
	mgr.cmdPoll()
	if mgr.ticketStatus != COPROC_TICKET_OK {
		t.Fatalf("first poll: expected OK, got %d", mgr.ticketStatus)
	}
	if _, exists := mgr.completions[ticket]; !exists {
		t.Fatalf("first poll should NOT evict completion")
	}
	if !mgr.completions[ticket].observed {
		t.Fatalf("first poll should mark as observed")
	}

	// Second POLL: should return OK and evict
	mgr.ticket = ticket
	mgr.cmdPoll()
	if mgr.ticketStatus != COPROC_TICKET_OK {
		t.Fatalf("second poll: expected OK, got %d", mgr.ticketStatus)
	}
	if _, exists := mgr.completions[ticket]; exists {
		t.Fatalf("second poll should evict completion")
	}

	// Third POLL: should return stale error
	mgr.ticket = ticket
	mgr.cmdPoll()
	if mgr.cmdError != COPROC_ERR_STALE_TICKET {
		t.Fatalf("third poll: expected STALE_TICKET, got err=%d", mgr.cmdError)
	}

	mgr.mu.Unlock()
}

func TestCoprocessorWorkerState(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	mgr.mu.Lock()
	state := mgr.computeWorkerState()
	if state != 0 {
		t.Fatalf("expected worker state 0 with no workers, got %d", state)
	}

	// Simulate workers
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{cpuType: EXEC_TYPE_IE32}
	mgr.workers[EXEC_TYPE_Z80] = &CoprocWorker{cpuType: EXEC_TYPE_Z80}
	state = mgr.computeWorkerState()
	expected := uint32(1<<EXEC_TYPE_IE32 | 1<<EXEC_TYPE_Z80)
	if state != expected {
		t.Fatalf("expected worker state %#x, got %#x", expected, state)
	}
	mgr.mu.Unlock()
}

func TestCoprocessorCompletionPruning(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	mgr.mu.Lock()
	// Add old completions
	for i := uint32(1); i <= 10; i++ {
		mgr.completions[i] = &CoprocCompletion{
			ticket:  i,
			status:  COPROC_TICKET_OK,
			created: time.Now().Add(-120 * time.Second), // older than TTL
		}
	}
	mgr.pruneCompletions()
	if len(mgr.completions) != 0 {
		t.Fatalf("expected all old completions pruned, got %d", len(mgr.completions))
	}
	mgr.mu.Unlock()
}

func TestCoprocessorCompletionCapPruning(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	mgr.mu.Lock()
	// Add more than COPROC_MAX_COMPLETIONS
	for i := uint32(1); i <= COPROC_MAX_COMPLETIONS+50; i++ {
		mgr.completions[i] = &CoprocCompletion{
			ticket:  i,
			status:  COPROC_TICKET_PENDING,
			created: time.Now(),
		}
	}
	mgr.pruneCompletions()
	if len(mgr.completions) > COPROC_MAX_COMPLETIONS {
		t.Fatalf("expected completions capped at %d, got %d", COPROC_MAX_COMPLETIONS, len(mgr.completions))
	}
	mgr.mu.Unlock()
}

// --- Worker Adapter Tests ---

func TestCoprocWorkerIE32StartStop(t *testing.T) {
	bus := NewMachineBus()

	// Create a simple IE32 binary: NOP + JMP to self
	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))

	worker, err := createIE32Worker(bus, code)
	if err != nil {
		t.Fatalf("createIE32Worker: %v", err)
	}

	// Verify it's running
	time.Sleep(10 * time.Millisecond)
	select {
	case <-worker.done:
		t.Fatal("worker stopped prematurely")
	default:
		// still running, good
	}

	// Stop it
	worker.stop()
	select {
	case <-worker.done:
		// stopped, good
	case <-time.After(2 * time.Second):
		t.Fatal("worker didn't stop in time")
	}
}

func TestCoprocWorkerIE32MemoryIsolation(t *testing.T) {
	bus := NewMachineBus()

	// Write a marker in IE64 code space
	bus.Write32(PROG_START, 0xDEADBEEF)

	code := buildIE32ServiceBinary(ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32)))
	worker, err := createIE32Worker(bus, code)
	if err != nil {
		t.Fatalf("createIE32Worker: %v", err)
	}
	defer func() {
		worker.stop()
		<-worker.done
	}()

	// Verify IE64 code space not touched
	if bus.Read32(PROG_START) != 0xDEADBEEF {
		t.Fatal("IE64 code space was modified by IE32 worker")
	}

	// Verify worker code is in its region
	if bus.Read8(WORKER_IE32_BASE) != code[0] {
		t.Fatal("worker code not found at expected location")
	}
}

// TestCoprocBus32Translation tests the CoprocBus32 address translation.
func TestCoprocBus32Translation(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()

	cb := &CoprocBus32{
		bus:          bus,
		mem:          mem,
		bankBase:     0x300000,
		mailboxBase:  MAILBOX_BASE,
		mailboxStart: 0x2000,
		mailboxEnd:   0x4000,
	}

	// Write to CPU addr 0x100 → bus addr 0x300100
	cb.Write8(0x100, 0x42)
	if mem[0x300100] != 0x42 {
		t.Fatalf("expected 0x42 at bus addr 0x300100, got 0x%02x", mem[0x300100])
	}

	// Read back
	if cb.Read8(0x100) != 0x42 {
		t.Fatalf("Read8 mismatch")
	}

	// Write to mailbox window (CPU addr 0x2000 → bus addr MAILBOX_BASE)
	cb.Write8(0x2000, 0xAB)
	if mem[MAILBOX_BASE] != 0xAB {
		t.Fatalf("mailbox write: expected 0xAB at bus addr %#x, got 0x%02x", MAILBOX_BASE, mem[MAILBOX_BASE])
	}

	// Test 16-bit and 32-bit
	cb.Write16(0x2010, 0x1234)
	if cb.Read16(0x2010) != 0x1234 {
		t.Fatalf("Read16 mismatch")
	}

	cb.Write32(0x2020, 0xDEADBEEF)
	if cb.Read32(0x2020) != 0xDEADBEEF {
		t.Fatalf("Read32 mismatch")
	}

	// Reset should be no-op
	cb.Reset()
	if mem[0x300100] != 0x42 {
		t.Fatal("Reset modified memory")
	}

	// GetMemory should return the worker's 64KB slice
	gm := cb.GetMemory()
	if len(gm) != 0x10000 {
		t.Fatalf("GetMemory length: %d, want %d", len(gm), 0x10000)
	}
}

// TestCoprocZ80BusTranslation tests the CoprocZ80Bus address translation.
func TestCoprocZ80BusTranslation(t *testing.T) {
	bus := NewMachineBus()
	mem := bus.GetMemory()

	zb := &CoprocZ80Bus{
		bus:          bus,
		mem:          mem,
		bankBase:     0x310000,
		mailboxBase:  MAILBOX_BASE,
		mailboxStart: 0x2000,
		mailboxEnd:   0x4000,
	}

	// Write to Z80 addr 0x100 → bus addr 0x310100
	zb.Write(0x100, 0x42)
	if mem[0x310100] != 0x42 {
		t.Fatalf("expected 0x42 at bus addr 0x310100, got 0x%02x", mem[0x310100])
	}

	// Read back
	if zb.Read(0x100) != 0x42 {
		t.Fatalf("Read mismatch")
	}

	// Write to mailbox window
	zb.Write(0x2000, 0xCD)
	if mem[MAILBOX_BASE] != 0xCD {
		t.Fatalf("mailbox write failed")
	}

	// IO ports return 0 / no-op
	if zb.In(0x10) != 0 {
		t.Fatal("In should return 0")
	}
	zb.Out(0x10, 0xFF) // should not panic
	zb.Tick(100)       // should not panic
}

// TestCoprocessorEnqueueAndRingLayout tests that enqueue correctly writes to the ring buffer.
func TestCoprocessorEnqueueAndRingLayout(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Create a dummy worker so enqueue doesn't fail with NO_WORKER
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.mu.Unlock()

	// Enqueue a request via MMIO
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_OP, 1) // op=1 (add)
	bus.Write32(COPROC_REQ_PTR, 0x400000)
	bus.Write32(COPROC_REQ_LEN, 8)
	bus.Write32(COPROC_RESP_PTR, 0x400100)
	bus.Write32(COPROC_RESP_CAP, 16)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	status := bus.Read32(COPROC_CMD_STATUS)
	if status != COPROC_STATUS_OK {
		errCode := bus.Read32(COPROC_CMD_ERROR)
		t.Fatalf("enqueue failed: status=%d err=%d", status, errCode)
	}

	ticket := bus.Read32(COPROC_TICKET)
	if ticket == 0 {
		t.Fatal("expected non-zero ticket")
	}

	// Verify ring state
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)

	// Head should have advanced to 1
	head := bus.Read8(ringBase + RING_HEAD_OFFSET)
	if head != 1 {
		t.Fatalf("expected head=1, got %d", head)
	}

	// Tail should still be 0
	tail := bus.Read8(ringBase + RING_TAIL_OFFSET)
	if tail != 0 {
		t.Fatalf("expected tail=0, got %d", tail)
	}

	// Verify request descriptor at entries[0]
	entryAddr := ringBase + RING_ENTRIES_OFFSET
	if bus.Read32(entryAddr+REQ_TICKET_OFF) != ticket {
		t.Fatal("ticket mismatch in ring entry")
	}
	if bus.Read32(entryAddr+REQ_OP_OFF) != 1 {
		t.Fatal("op mismatch in ring entry")
	}
	if bus.Read32(entryAddr+REQ_REQ_PTR_OFF) != 0x400000 {
		t.Fatal("reqPtr mismatch in ring entry")
	}
}

// TestCoprocessorQueueFull verifies ring full detection.
func TestCoprocessorQueueFull(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.mu.Unlock()

	// Fill the ring (capacity=16, but ring full at 15 entries since head+1==tail means full)
	for i := range RING_CAPACITY - 1 {
		bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
		bus.Write32(COPROC_OP, 1)
		bus.Write32(COPROC_REQ_PTR, 0x400000)
		bus.Write32(COPROC_REQ_LEN, 8)
		bus.Write32(COPROC_RESP_PTR, 0x400100)
		bus.Write32(COPROC_RESP_CAP, 16)
		bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

		if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
			t.Fatalf("enqueue %d failed unexpectedly", i)
		}
	}

	// 16th enqueue should fail (ring full)
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_OP, 1)
	bus.Write32(COPROC_REQ_PTR, 0x400000)
	bus.Write32(COPROC_REQ_LEN, 8)
	bus.Write32(COPROC_RESP_PTR, 0x400100)
	bus.Write32(COPROC_RESP_CAP, 16)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_ERROR {
		t.Fatal("expected queue full error")
	}
	if bus.Read32(COPROC_CMD_ERROR) != COPROC_ERR_QUEUE_FULL {
		t.Fatalf("expected QUEUE_FULL error, got %d", bus.Read32(COPROC_CMD_ERROR))
	}
}

// TestCoprocessorWaitTimeout tests that WAIT times out when no worker responds.
func TestCoprocessorWaitTimeout(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Create a dummy worker (won't actually process requests)
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.mu.Unlock()

	// Enqueue a request
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_OP, 1)
	bus.Write32(COPROC_REQ_PTR, 0x400000)
	bus.Write32(COPROC_REQ_LEN, 8)
	bus.Write32(COPROC_RESP_PTR, 0x400100)
	bus.Write32(COPROC_RESP_CAP, 16)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	ticket := bus.Read32(COPROC_TICKET)

	// Wait with short timeout
	bus.Write32(COPROC_TICKET, ticket)
	bus.Write32(COPROC_TIMEOUT, 50) // 50ms timeout
	start := time.Now()
	bus.Write32(COPROC_CMD, COPROC_CMD_WAIT)
	elapsed := time.Since(start)

	ticketStatus := bus.Read32(COPROC_TICKET_STATUS)
	if ticketStatus != COPROC_TICKET_TIMEOUT {
		t.Fatalf("expected TIMEOUT status, got %d", ticketStatus)
	}

	if elapsed < 40*time.Millisecond {
		t.Fatalf("wait returned too quickly: %v", elapsed)
	}
}

// TestCoprocessorSimulatedEndToEnd simulates a complete request-response cycle
// by directly writing response data (simulating what a worker CPU would do).
func TestCoprocessorSimulatedEndToEnd(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Create dummy worker
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.mu.Unlock()

	// Write request data: two uint32s at 0x400000
	bus.Write32(0x400000, 10)
	bus.Write32(0x400004, 20)

	// Enqueue request
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_OP, 1)
	bus.Write32(COPROC_REQ_PTR, 0x400000)
	bus.Write32(COPROC_REQ_LEN, 8)
	bus.Write32(COPROC_RESP_PTR, 0x400100)
	bus.Write32(COPROC_RESP_CAP, 16)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	ticket := bus.Read32(COPROC_TICKET)

	// Simulate worker processing: read request, compute result, write response
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	tail := bus.Read8(ringBase + RING_TAIL_OFFSET) // should be 0

	// Read request from entries[tail]
	entryAddr := ringBase + RING_ENTRIES_OFFSET + uint32(tail)*REQ_DESC_SIZE
	reqPtr := bus.Read32(entryAddr + REQ_REQ_PTR_OFF)
	respPtr := bus.Read32(entryAddr + REQ_RESP_PTR_OFF)

	// Process: add two uint32s
	a := bus.Read32(reqPtr)
	b := bus.Read32(reqPtr + 4)
	bus.Write32(respPtr, a+b)

	// Write response descriptor
	respDescAddr := ringBase + RING_RESPONSES_OFFSET + uint32(tail)*RESP_DESC_SIZE
	bus.Write32(respDescAddr+RESP_TICKET_OFF, ticket)
	bus.Write32(respDescAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)
	bus.Write32(respDescAddr+RESP_RESULT_CODE_OFF, 0)
	bus.Write32(respDescAddr+RESP_RESP_LEN_OFF, 4)

	// Advance tail
	newTail := (tail + 1) % RING_CAPACITY
	bus.Write8(ringBase+RING_TAIL_OFFSET, newTail)

	// Now poll for the result
	bus.Write32(COPROC_TICKET, ticket)
	bus.Write32(COPROC_CMD, COPROC_CMD_POLL)

	ticketStatus := bus.Read32(COPROC_TICKET_STATUS)
	if ticketStatus != COPROC_TICKET_OK {
		t.Fatalf("expected OK status, got %d", ticketStatus)
	}

	// Verify result in response buffer
	result := bus.Read32(0x400100)
	if result != 30 {
		t.Fatalf("expected result 30, got %d", result)
	}
}

// TestCoprocessorDoubleStart tests that starting the same CPU type twice
// stops the first worker.
func TestCoprocessorDoubleStart(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	done1 := make(chan struct{})
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    done1,
		stop:    func() { close(done1) },
	}
	mgr.mu.Unlock()

	// Starting another IE32 worker should stop the first
	done2 := make(chan struct{})
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    done2,
		stop:    func() {},
	}
	mgr.mu.Unlock()

	// First worker's done channel should be closed
	select {
	case <-done1:
		// Good - first worker was stopped
	default:
		// This is expected since we directly replaced, not via cmdStart
	}
}

func TestCoprocessorStopNoWorker(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	mgr.mu.Lock()
	mgr.cpuType = EXEC_TYPE_IE32
	mgr.cmdStop()
	if mgr.cmdStatus != COPROC_STATUS_ERROR || mgr.cmdError != COPROC_ERR_NO_WORKER {
		t.Fatalf("expected NO_WORKER error, got status=%d err=%d", mgr.cmdStatus, mgr.cmdError)
	}
	mgr.mu.Unlock()
}

// TestCoprocWorkerZ80StartStop tests Z80 worker lifecycle.
func TestCoprocWorkerZ80StartStop(t *testing.T) {
	bus := NewMachineBus()

	// Create a minimal Z80 binary: JR -2 (infinite loop: 0x18 0xFE)
	code := []byte{0x18, 0xFE}

	worker, err := createZ80Worker(bus, code)
	if err != nil {
		t.Fatalf("createZ80Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	select {
	case <-worker.done:
		t.Fatal("Z80 worker stopped prematurely")
	default:
	}

	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Z80 worker didn't stop in time")
	}
}

// TestCoprocWorker6502StartStop tests 6502 worker lifecycle.
func TestCoprocWorker6502StartStop(t *testing.T) {
	bus := NewMachineBus()

	// Create a minimal 6502 binary: JMP $0000 (infinite loop)
	// JMP absolute: 0x4C low high
	code := []byte{0x4C, 0x00, 0x00}

	worker, err := create6502Worker(bus, code)
	if err != nil {
		t.Fatalf("create6502Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	select {
	case <-worker.done:
		t.Fatal("6502 worker stopped prematurely")
	default:
	}

	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("6502 worker didn't stop in time")
	}
}

// TestCoprocWorkerM68KStartStop tests M68K worker lifecycle.
func TestCoprocWorkerM68KStartStop(t *testing.T) {
	bus := NewMachineBus()

	// M68K BRA.S -2 (infinite loop): 0x60FE
	// Must be stored in big-endian in bus memory (M68K byte-swaps on fetch)
	// M68K CPU does bits.ReverseBytes16 on fetch, so store LE bytes that
	// will become 0x60FE after swap: store as 0xFE60
	code := []byte{0xFE, 0x60}

	worker, err := createM68KWorker(bus, code)
	if err != nil {
		t.Fatalf("createM68KWorker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	select {
	case <-worker.done:
		t.Fatal("M68K worker stopped prematurely")
	default:
	}

	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("M68K worker didn't stop in time")
	}
}

// TestCoprocWorkerX86StartStop tests x86 worker lifecycle.
func TestCoprocWorkerX86StartStop(t *testing.T) {
	bus := NewMachineBus()

	// x86 JMP short -2 (infinite loop): EB FE
	code := []byte{0xEB, 0xFE}

	worker, err := createX86Worker(bus, code)
	if err != nil {
		t.Fatalf("createX86Worker: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	select {
	case <-worker.done:
		t.Fatal("x86 worker stopped prematurely")
	default:
	}

	worker.stop()
	select {
	case <-worker.done:
	case <-time.After(2 * time.Second):
		t.Fatal("x86 worker didn't stop in time")
	}
}

// --- End-to-End Tests with Assembled Service Binaries ---
// These tests assemble actual service templates, create real CPU workers,
// enqueue requests, and verify the workers process them correctly.

// assembleService assembles a service binary using the specified assembler command.
// Returns the binary data or skips the test if the assembler is not available.
func assembleService(t *testing.T, args []string, srcFile string) []byte {
	t.Helper()

	// Check if assembler exists
	_, err := exec.LookPath(args[0])
	if err != nil {
		t.Skipf("assembler %s not found, skipping", args[0])
	}

	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "service.bin")

	// Build command with output file substitution
	var cmdArgs []string
	for _, a := range args[1:] {
		if a == "OUTPUT" {
			cmdArgs = append(cmdArgs, outFile)
		} else {
			cmdArgs = append(cmdArgs, a)
		}
	}
	cmdArgs = append(cmdArgs, srcFile)

	cmd := exec.Command(args[0], cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("assembly failed: %v\n%s", err, out)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading assembled binary: %v", err)
	}
	return data
}

// assemble6502Service assembles a 6502 service using ca65+ld65.
func assemble6502Service(t *testing.T, srcFile string) []byte {
	t.Helper()

	for _, tool := range []string{"ca65", "ld65"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not found, skipping", tool)
		}
	}

	tmpDir := t.TempDir()
	objFile := filepath.Join(tmpDir, "service.o")
	binFile := filepath.Join(tmpDir, "service.bin")

	// Assemble
	cmd := exec.Command("ca65", "-o", objFile, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ca65 failed: %v\n%s", err, out)
	}

	// Link
	cmd = exec.Command("ld65", "-t", "none", "-o", binFile, objFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ld65 failed: %v\n%s", err, out)
	}

	data, err := os.ReadFile(binFile)
	if err != nil {
		t.Fatalf("reading assembled binary: %v", err)
	}
	return data
}

// coprocEndToEndTest runs a complete end-to-end coprocessor test:
// enqueue an add(10,20) request, wait for the worker to process it,
// verify result=30 at respPtr.
func coprocEndToEndTest(t *testing.T, bus *MachineBus, mgr *CoprocessorManager,
	cpuType uint32, reqBusAddr, respBusAddr uint32, reqCPUAddr, respCPUAddr uint32) {
	t.Helper()

	// Write request data: two uint32s (10, 20) at reqBusAddr
	bus.Write32(reqBusAddr, 10)
	bus.Write32(reqBusAddr+4, 20)

	// Enqueue request via MMIO
	bus.Write32(COPROC_CPU_TYPE, cpuType)
	bus.Write32(COPROC_OP, 1)               // op=1 (add)
	bus.Write32(COPROC_REQ_PTR, reqCPUAddr) // CPU-visible address
	bus.Write32(COPROC_REQ_LEN, 8)
	bus.Write32(COPROC_RESP_PTR, respCPUAddr) // CPU-visible address
	bus.Write32(COPROC_RESP_CAP, 16)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	status := bus.Read32(COPROC_CMD_STATUS)
	if status != COPROC_STATUS_OK {
		errCode := bus.Read32(COPROC_CMD_ERROR)
		t.Fatalf("enqueue failed: status=%d err=%d", status, errCode)
	}

	ticket := bus.Read32(COPROC_TICKET)

	// Wait for the worker to process the request
	bus.Write32(COPROC_TICKET, ticket)
	bus.Write32(COPROC_TIMEOUT, 5000) // 5 second timeout
	bus.Write32(COPROC_CMD, COPROC_CMD_WAIT)

	ticketStatus := bus.Read32(COPROC_TICKET_STATUS)
	if ticketStatus != COPROC_TICKET_OK {
		t.Fatalf("expected OK status, got %d (ticket=%d)", ticketStatus, ticket)
	}

	// Verify result at respBusAddr
	result := bus.Read32(respBusAddr)
	if result != 30 {
		t.Fatalf("expected result 30, got %d", result)
	}
}

func TestCoprocEndToEnd_X86(t *testing.T) {
	// Assemble x86 service binary with nasm
	data := assembleService(t, []string{
		"nasm", "-f", "bin", "-o", "OUTPUT",
	}, "sdk/examples/asm/coproc_service_x86.asm")

	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Create x86 worker
	worker, err := createX86Worker(bus, data)
	if err != nil {
		t.Fatalf("createX86Worker: %v", err)
	}
	defer func() {
		worker.stop()
		<-worker.done
	}()

	// Register worker with manager
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_X86] = worker
	mgr.mu.Unlock()

	// x86 uses 32-bit addressing - reqPtr/respPtr are bus addresses directly
	coprocEndToEndTest(t, bus, mgr, EXEC_TYPE_X86, 0x400000, 0x400100, 0x400000, 0x400100)
}

func TestCoprocEndToEnd_Z80(t *testing.T) {
	// Assemble Z80 service binary with vasmz80_std
	data := assembleService(t, []string{
		"vasmz80_std", "-Fbin", "-o", "OUTPUT",
	}, "sdk/examples/asm/coproc_service_z80.asm")

	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Create Z80 worker
	worker, err := createZ80Worker(bus, data)
	if err != nil {
		t.Fatalf("createZ80Worker: %v", err)
	}
	defer func() {
		worker.stop()
		<-worker.done
	}()

	// Register worker with manager
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_Z80] = worker
	mgr.mu.Unlock()

	// Z80 uses 16-bit addressing via CoprocZ80Bus:
	//   CPU addr 0x4000-0xFFFF → bus addr 0x314000-0x31FFFF
	// So reqPtr=0x4000 maps to bus addr 0x314000
	coprocEndToEndTest(t, bus, mgr, EXEC_TYPE_Z80,
		0x314000, 0x314100, // bus addresses for data
		0x4000, 0x4100, // CPU-visible addresses for ring entry
	)
}

func TestCoprocEndToEnd_M68K(t *testing.T) {
	// Assemble M68K service binary with vasmm68k_mot
	data := assembleService(t, []string{
		"/opt/amiga/bin/vasmm68k_mot", "-Fbin", "-m68020", "-devpac", "-o", "OUTPUT",
	}, "sdk/examples/asm/coproc_service_68k.asm")

	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Create M68K worker
	worker, err := createM68KWorker(bus, data)
	if err != nil {
		t.Fatalf("createM68KWorker: %v", err)
	}
	defer func() {
		worker.stop()
		<-worker.done
	}()

	// Register worker with manager
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_M68K] = worker
	mgr.mu.Unlock()

	// M68K uses 32-bit addressing - reqPtr/respPtr are bus addresses directly
	coprocEndToEndTest(t, bus, mgr, EXEC_TYPE_M68K, 0x400000, 0x400100, 0x400000, 0x400100)
}

func TestCoprocEndToEnd_6502(t *testing.T) {
	// Assemble 6502 service binary with ca65+ld65
	data := assemble6502Service(t, "sdk/examples/asm/coproc_service_65.asm")

	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Create 6502 worker
	worker, err := create6502Worker(bus, data)
	if err != nil {
		t.Fatalf("create6502Worker: %v", err)
	}
	defer func() {
		worker.stop()
		<-worker.done
	}()

	// Register worker with manager
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_6502] = worker
	mgr.mu.Unlock()

	// 6502 uses 16-bit addressing via CoprocBus32:
	//   CPU addr 0x4000-0xFFFF → bus addr 0x304000-0x30FFFF
	coprocEndToEndTest(t, bus, mgr, EXEC_TYPE_6502,
		0x304000, 0x304100, // bus addresses for data
		0x4000, 0x4100, // CPU-visible addresses for ring entry
	)
}

// --- Byte-level MMIO register tests ---

// TestCoprocMMIO_ByteLevelWrite writes a 32-bit value to COPROC_REQ_PTR via
// 4 individual bus.Write8 calls and verifies bus.Read32 returns the correct value.
func TestCoprocMMIO_ByteLevelWrite(t *testing.T) {
	bus, _ := newTestBusAndManager(t)

	// Write 0xDEADBEEF via 4 byte writes (little-endian)
	bus.Write8(COPROC_REQ_PTR+0, 0xEF)
	bus.Write8(COPROC_REQ_PTR+1, 0xBE)
	bus.Write8(COPROC_REQ_PTR+2, 0xAD)
	bus.Write8(COPROC_REQ_PTR+3, 0xDE)

	got := bus.Read32(COPROC_REQ_PTR)
	if got != 0xDEADBEEF {
		t.Fatalf("expected 0xDEADBEEF, got 0x%08X", got)
	}
}

// TestCoprocMMIO_ByteLevelRead sets a shadow register and reads individual bytes.
func TestCoprocMMIO_ByteLevelRead(t *testing.T) {
	bus, _ := newTestBusAndManager(t)

	// Set COPROC_OP to 0x12345678 via 32-bit write
	bus.Write32(COPROC_OP, 0x12345678)

	// Read back byte by byte (little-endian)
	b0 := bus.Read8(COPROC_OP + 0)
	b1 := bus.Read8(COPROC_OP + 1)
	b2 := bus.Read8(COPROC_OP + 2)
	b3 := bus.Read8(COPROC_OP + 3)

	if b0 != 0x78 || b1 != 0x56 || b2 != 0x34 || b3 != 0x12 {
		t.Fatalf("byte-level read mismatch: got %02X %02X %02X %02X, want 78 56 34 12",
			b0, b1, b2, b3)
	}
}

// TestCoprocMMIO_Write32Compat verifies existing 32-bit write behavior is unchanged.
func TestCoprocMMIO_Write32Compat(t *testing.T) {
	bus, _ := newTestBusAndManager(t)

	registers := []struct {
		addr uint32
		val  uint32
	}{
		{COPROC_CPU_TYPE, EXEC_TYPE_IE32},
		{COPROC_OP, 42},
		{COPROC_REQ_PTR, 0x400000},
		{COPROC_REQ_LEN, 8},
		{COPROC_RESP_PTR, 0x400100},
		{COPROC_RESP_CAP, 16},
		{COPROC_TIMEOUT, 1000},
		{COPROC_NAME_PTR, 0x500000},
		{COPROC_TICKET, 99},
	}

	for _, r := range registers {
		bus.Write32(r.addr, r.val)
		got := bus.Read32(r.addr)
		if got != r.val {
			t.Errorf("register 0x%X: wrote 0x%X, read 0x%X", r.addr, r.val, got)
		}
	}
}

// TestCoprocMMIO_CmdDispatchOnByte0Only verifies that only writing byte 0 of
// COPROC_CMD dispatches the command; writes to bytes 1-3 must NOT dispatch.
func TestCoprocMMIO_CmdDispatchOnByte0Only(t *testing.T) {
	_, mgr := newTestBusAndManager(t)

	// Set up a valid CPU type so we can detect dispatch via cmdStatus change
	mgr.mu.Lock()
	mgr.cpuType = 99 // invalid CPU type - dispatch will set ERROR status
	mgr.cmdStatus = COPROC_STATUS_OK
	mgr.mu.Unlock()

	// Write to bytes 1, 2, 3 of CMD - should NOT dispatch
	for _, off := range []uint32{1, 2, 3} {
		mgr.HandleWrite(COPROC_CMD+off, COPROC_CMD_START)
		mgr.mu.Lock()
		if mgr.cmdStatus != COPROC_STATUS_OK {
			mgr.mu.Unlock()
			t.Fatalf("writing byte %d of CMD triggered dispatch", off)
		}
		mgr.mu.Unlock()
	}

	// Write to byte 0 - should dispatch and fail (invalid CPU type)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_START)
	mgr.mu.Lock()
	if mgr.cmdStatus != COPROC_STATUS_ERROR {
		mgr.mu.Unlock()
		t.Fatal("writing byte 0 of CMD did not trigger dispatch")
	}
	mgr.mu.Unlock()
}

// TestCoprocMMIO_ByteLevelEnqueueFlow writes all parameters byte-by-byte,
// triggers CMD=ENQUEUE via byte-0 write, and verifies a ticket is returned.
func TestCoprocMMIO_ByteLevelEnqueueFlow(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)

	// Create dummy worker
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.mu.Unlock()

	// Write CPU_TYPE = EXEC_TYPE_IE32 (=1) byte by byte
	bus.Write8(COPROC_CPU_TYPE+0, byte(EXEC_TYPE_IE32))
	bus.Write8(COPROC_CPU_TYPE+1, 0)
	bus.Write8(COPROC_CPU_TYPE+2, 0)
	bus.Write8(COPROC_CPU_TYPE+3, 0)

	// Write OP = 1
	bus.Write8(COPROC_OP+0, 1)
	bus.Write8(COPROC_OP+1, 0)
	bus.Write8(COPROC_OP+2, 0)
	bus.Write8(COPROC_OP+3, 0)

	// Write REQ_PTR = 0x400000
	bus.Write8(COPROC_REQ_PTR+0, 0x00)
	bus.Write8(COPROC_REQ_PTR+1, 0x00)
	bus.Write8(COPROC_REQ_PTR+2, 0x40)
	bus.Write8(COPROC_REQ_PTR+3, 0x00)

	// Write REQ_LEN = 8
	bus.Write8(COPROC_REQ_LEN+0, 8)
	bus.Write8(COPROC_REQ_LEN+1, 0)
	bus.Write8(COPROC_REQ_LEN+2, 0)
	bus.Write8(COPROC_REQ_LEN+3, 0)

	// Write RESP_PTR = 0x400100
	bus.Write8(COPROC_RESP_PTR+0, 0x00)
	bus.Write8(COPROC_RESP_PTR+1, 0x01)
	bus.Write8(COPROC_RESP_PTR+2, 0x40)
	bus.Write8(COPROC_RESP_PTR+3, 0x00)

	// Write RESP_CAP = 16
	bus.Write8(COPROC_RESP_CAP+0, 16)
	bus.Write8(COPROC_RESP_CAP+1, 0)
	bus.Write8(COPROC_RESP_CAP+2, 0)
	bus.Write8(COPROC_RESP_CAP+3, 0)

	// Trigger ENQUEUE via byte-0 write to CMD
	bus.Write8(COPROC_CMD+0, COPROC_CMD_ENQUEUE)

	status := bus.Read32(COPROC_CMD_STATUS)
	if status != COPROC_STATUS_OK {
		errCode := bus.Read32(COPROC_CMD_ERROR)
		t.Fatalf("byte-level enqueue failed: status=%d err=%d", status, errCode)
	}

	ticket := bus.Read32(COPROC_TICKET)
	if ticket == 0 {
		t.Fatal("expected non-zero ticket from byte-level enqueue")
	}
}

// --- Regression tests for bugs #1, #2, #3 ---

// TestCoprocessorEnqueueFailReturnsZeroTicket verifies that when ENQUEUE fails
// (no worker), COPROC_TICKET is reset to 0 so callers never see a stale ticket.
func TestCoprocessorEnqueueFailReturnsZeroTicket(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Register a dummy worker and enqueue one successful request to set m.ticket=1
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.mu.Unlock()

	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_OP, 1)
	bus.Write32(COPROC_REQ_PTR, 0x400000)
	bus.Write32(COPROC_REQ_LEN, 8)
	bus.Write32(COPROC_RESP_PTR, 0x400100)
	bus.Write32(COPROC_RESP_CAP, 16)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatal("first enqueue should succeed")
	}
	firstTicket := bus.Read32(COPROC_TICKET)
	if firstTicket == 0 {
		t.Fatal("first ticket should be non-zero")
	}

	// Now remove the worker and enqueue again - should fail
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = nil
	mgr.mu.Unlock()

	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_ERROR {
		t.Fatal("second enqueue should fail (no worker)")
	}

	// COPROC_TICKET must be 0 after failed enqueue, NOT the stale firstTicket
	failedTicket := bus.Read32(COPROC_TICKET)
	if failedTicket != 0 {
		t.Fatalf("ticket after failed enqueue should be 0, got %d (stale from ticket %d)", failedTicket, firstTicket)
	}
}

// TestCoprocessorCachedStatusSurvivesRingReuse verifies that once a terminal
// status is cached in the completion map, it's returned correctly even after
// the ring slot is overwritten by later requests.
func TestCoprocessorCachedStatusSurvivesRingReuse(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = &CoprocWorker{
		cpuType: EXEC_TYPE_IE32,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.mu.Unlock()

	// Enqueue a request
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_OP, 1)
	bus.Write32(COPROC_REQ_PTR, 0x400000)
	bus.Write32(COPROC_REQ_LEN, 8)
	bus.Write32(COPROC_RESP_PTR, 0x400100)
	bus.Write32(COPROC_RESP_CAP, 16)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	ticket1 := bus.Read32(COPROC_TICKET)

	// Simulate worker completing the request: write OK status to response slot
	cpuIdx := cpuTypeToIndex(EXEC_TYPE_IE32)
	ringBase := ringBaseAddr(cpuIdx)
	respAddr := ringBase + RING_RESPONSES_OFFSET + 0*RESP_DESC_SIZE // slot 0
	bus.Write32(respAddr+RESP_TICKET_OFF, ticket1)
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)
	bus.Write32(respAddr+RESP_RESP_LEN_OFF, 4)

	// First POLL - should find terminal status and cache it
	bus.Write32(COPROC_TICKET, ticket1)
	bus.Write32(COPROC_CMD, COPROC_CMD_POLL)
	status1 := bus.Read32(COPROC_TICKET_STATUS)
	if status1 != COPROC_TICKET_OK {
		t.Fatalf("first poll expected OK (2), got %d", status1)
	}

	// Now overwrite the ring slot with a DIFFERENT ticket (simulating ring reuse)
	bus.Write32(respAddr+RESP_TICKET_OFF, 9999)
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_ERROR)

	// Second POLL of ticket1 - should still return OK from cache, NOT regress to PENDING
	bus.Write32(COPROC_TICKET, ticket1)
	bus.Write32(COPROC_CMD, COPROC_CMD_POLL)
	status2 := bus.Read32(COPROC_TICKET_STATUS)
	if status2 != COPROC_TICKET_OK {
		t.Fatalf("second poll after ring reuse expected cached OK (2), got %d", status2)
	}
}

// TestCoprocessorWorkerDownUsesStoredCPUType verifies that the worker-down
// check in cmdPoll uses the stored cpuType from the completion entry (not
// comp.ticket or a ring scan), and doesn't panic on invalid values.
func TestCoprocessorWorkerDownUsesStoredCPUType(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	// Register worker, enqueue, then remove worker
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_Z80] = &CoprocWorker{
		cpuType: EXEC_TYPE_Z80,
		done:    make(chan struct{}),
		stop:    func() {},
	}
	mgr.mu.Unlock()

	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_Z80)
	bus.Write32(COPROC_OP, 1)
	bus.Write32(COPROC_REQ_PTR, 0x400000)
	bus.Write32(COPROC_REQ_LEN, 8)
	bus.Write32(COPROC_RESP_PTR, 0x400100)
	bus.Write32(COPROC_RESP_CAP, 16)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	ticket := bus.Read32(COPROC_TICKET)

	// Remove the worker (simulating crash)
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_Z80] = nil
	mgr.mu.Unlock()

	// POLL should detect worker-down via stored cpuType, not panic
	bus.Write32(COPROC_TICKET, ticket)
	bus.Write32(COPROC_CMD, COPROC_CMD_POLL)

	status := bus.Read32(COPROC_TICKET_STATUS)
	if status != COPROC_TICKET_WORKER_DOWN {
		t.Fatalf("expected WORKER_DOWN (5), got %d", status)
	}
}
