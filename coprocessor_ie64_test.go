//go:build headless

package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// u32 converts a signed int32 to uint32 for instruction encoding.
func u32(v int32) uint32 { return uint32(v) }

func TestCpuTypeToIndex_IE64(t *testing.T) {
	idx := cpuTypeToIndex(EXEC_TYPE_IE64)
	if idx != 5 {
		t.Fatalf("cpuTypeToIndex(EXEC_TYPE_IE64) = %d, want 5", idx)
	}
}

func TestIE64WorkerConstants(t *testing.T) {
	if WORKER_IE64_SIZE != WORKER_IE64_END-WORKER_IE64_BASE+1 {
		t.Fatalf("WORKER_IE64_SIZE mismatch: %d != %d", WORKER_IE64_SIZE, WORKER_IE64_END-WORKER_IE64_BASE+1)
	}
	if WORKER_IE64_SIZE != 512*1024 {
		t.Fatalf("WORKER_IE64_SIZE = %d, want 524288", WORKER_IE64_SIZE)
	}
	if WORKER_IE64_BASE != 0x3A0000 {
		t.Fatalf("WORKER_IE64_BASE = 0x%X, want 0x3A0000", WORKER_IE64_BASE)
	}
}

// buildIE64HaltBinary returns a minimal IE64 binary that immediately halts.
func buildIE64HaltBinary() []byte {
	// HALT instruction: opcode=0xE1, rest zeroed
	return []byte{OP_HALT64, 0, 0, 0, 0, 0, 0, 0}
}

// buildIE64Instr builds an 8-byte IE64 instruction.
// X bit: 0 = third operand is regs[rt], 1 = third operand is imm32
func buildIE64Instr(opcode byte, rd, size byte, rs, rt byte, imm32 uint32, xbit bool) []byte {
	instr := make([]byte, 8)
	instr[0] = opcode
	x := byte(0)
	if xbit {
		x = 1
	}
	instr[1] = (rd << 3) | (size << 1) | x
	instr[2] = rs << 3
	instr[3] = rt << 3
	binary.LittleEndian.PutUint32(instr[4:], imm32)
	return instr
}

// buildIE64ServiceBinary creates a minimal IE64 service binary that polls
// the ring buffer and processes one request, then halts.
func buildIE64ServiceBinary(ringBase uint32) []byte {
	var prog []byte
	emit := func(opcode byte, rd, size, rs, rt byte, imm32 uint32, xbit bool) {
		prog = append(prog, buildIE64Instr(opcode, rd, size, rs, rt, imm32, xbit)...)
	}

	// R10 = ringBase (MOVE with X=1 loads immediate)
	emit(OP_MOVE, 10, IE64_SIZE_L, 0, 0, ringBase, true) // r10 = ringBase

	// Poll loop: load head and tail (PC-relative branches)
	pollOffset := uint32(len(prog))                              // remember instruction offset for branch target
	emit(OP_LOAD, 1, IE64_SIZE_B, 10, 0, RING_HEAD_OFFSET, true) // r1 = mem[r10 + HEAD]
	emit(OP_LOAD, 2, IE64_SIZE_B, 10, 0, RING_TAIL_OFFSET, true) // r2 = mem[r10 + TAIL]

	// BEQ rs=1, rt=2, disp = pollOffset - currentOffset (branch back if equal)
	currentOffset := uint32(len(prog))
	disp := int32(pollOffset) - int32(currentOffset) // negative displacement
	emit(OP_BEQ, 0, IE64_SIZE_Q, 1, 2, uint32(disp), false)

	// Found a request. Compute response address:
	// respAddr = ringBase + RING_RESPONSES_OFFSET + tail * RESP_DESC_SIZE
	emit(OP_MULU, 3, IE64_SIZE_L, 2, 0, RESP_DESC_SIZE, true)       // r3 = r2 * RESP_DESC_SIZE
	emit(OP_ADD, 3, IE64_SIZE_L, 3, 0, RING_RESPONSES_OFFSET, true) // r3 += RESPONSES_OFFSET
	emit(OP_ADD, 4, IE64_SIZE_L, 10, 3, 0, false)                   // r4 = r10 + r3

	// Write COPROC_TICKET_OK to response status field
	emit(OP_MOVE, 5, IE64_SIZE_L, 0, 0, COPROC_TICKET_OK, true) // r5 = 2
	emit(OP_STORE, 5, IE64_SIZE_L, 4, 0, RESP_STATUS_OFF, true) // mem[r4 + STATUS_OFF] = r5

	// Advance tail: tail = (tail + 1) % capacity
	emit(OP_ADD, 2, IE64_SIZE_L, 2, 0, 1, true)                      // r2 = tail + 1
	emit(OP_LOAD, 6, IE64_SIZE_B, 10, 0, RING_CAPACITY_OFFSET, true) // r6 = capacity
	emit(OP_MOD64, 2, IE64_SIZE_L, 2, 6, 0, false)                   // r2 = r2 % r6
	emit(OP_STORE, 2, IE64_SIZE_B, 10, 0, RING_TAIL_OFFSET, true)    // mem[r10 + TAIL] = r2

	// Branch back to poll loop (handle multiple requests, not just one)
	loopBackOffset := uint32(len(prog))
	loopDisp := int32(pollOffset) - int32(loopBackOffset)
	emit(OP_BRA, 0, IE64_SIZE_Q, 0, 0, uint32(loopDisp), false)

	return prog
}

func TestIE64WorkerCreate(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	// Write halt binary to a file
	tmpDir := t.TempDir()
	mgr.baseDir = tmpDir
	binPath := filepath.Join(tmpDir, "test_ie64.ie64")
	if err := os.WriteFile(binPath, buildIE64HaltBinary(), 0644); err != nil {
		t.Fatal(err)
	}

	// Write filename to bus memory
	writeString(bus, 0x90000, "test_ie64.ie64")

	// Start the worker via MMIO
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_NAME_PTR, 0x90000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)

	status := bus.Read32(COPROC_CMD_STATUS)
	if status != COPROC_STATUS_OK {
		errCode := bus.Read32(COPROC_CMD_ERROR)
		t.Fatalf("IE64 worker start failed: status=%d error=%d", status, errCode)
	}

	// Verify worker state bit 2 (EXEC_TYPE_IE64 = 2, bit 2)
	time.Sleep(10 * time.Millisecond)
	state := bus.Read32(COPROC_WORKER_STATE)
	if state&(1<<EXEC_TYPE_IE64) == 0 {
		t.Fatalf("IE64 worker not visible in COPROC_WORKER_STATE: 0x%X", state)
	}
}

func TestIE64WorkerStartStop(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	tmpDir := t.TempDir()
	mgr.baseDir = tmpDir

	// Build a NOP-loop binary (NOP + BEQ r0,r0 back to start)
	var nopLoop []byte
	nopLoop = append(nopLoop, buildIE64Instr(OP_NOP64, 0, 0, 0, 0, 0, false)...)
	// BEQ r0, r0, displacement=-8 (branch back to NOP)
	nopLoop = append(nopLoop, buildIE64Instr(OP_BEQ, 0, IE64_SIZE_Q, 0, 0, u32(-8), false)...)

	binPath := filepath.Join(tmpDir, "noploop.ie64")
	if err := os.WriteFile(binPath, nopLoop, 0644); err != nil {
		t.Fatal(err)
	}

	writeString(bus, 0x90000, "noploop.ie64")
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_NAME_PTR, 0x90000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)

	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatal("start failed")
	}

	time.Sleep(10 * time.Millisecond)
	state := bus.Read32(COPROC_WORKER_STATE)
	if state&(1<<EXEC_TYPE_IE64) == 0 {
		t.Fatal("worker not running")
	}

	// Stop the worker
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_CMD, COPROC_CMD_STOP)

	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatal("stop failed")
	}

	time.Sleep(10 * time.Millisecond)
	state = bus.Read32(COPROC_WORKER_STATE)
	if state&(1<<EXEC_TYPE_IE64) != 0 {
		t.Fatal("worker still running after stop")
	}
}

func TestIE64WorkerEnqueuePoll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping IE64 service binary round-trip test in short mode")
	}
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	tmpDir := t.TempDir()
	mgr.baseDir = tmpDir

	ringIdx := cpuTypeToIndex(EXEC_TYPE_IE64)
	ringBase := ringBaseAddr(ringIdx)

	// Build a service that polls ring and completes one request
	svcBin := buildIE64ServiceBinary(ringBase)
	binPath := filepath.Join(tmpDir, "svc.ie64")
	if err := os.WriteFile(binPath, svcBin, 0644); err != nil {
		t.Fatal(err)
	}

	// Prevent auto-calibration from consuming a ring slot before our test request
	mgr.dispatchOverheadNs.Store(1)

	writeString(bus, 0x90000, "svc.ie64")
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_NAME_PTR, 0x90000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("start failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}

	// Enqueue a request
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_OP, 1) // memcpy op
	bus.Write32(COPROC_REQ_PTR, 0x80000)
	bus.Write32(COPROC_REQ_LEN, 64)
	bus.Write32(COPROC_RESP_PTR, 0x81000)
	bus.Write32(COPROC_RESP_CAP, 64)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("enqueue failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}

	ticket := bus.Read32(COPROC_TICKET)
	if ticket == 0 {
		t.Fatal("got ticket 0 from enqueue")
	}

	// Poll until completion or timeout
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		bus.Write32(COPROC_TICKET, ticket)
		bus.Write32(COPROC_CMD, COPROC_CMD_POLL)
		status := bus.Read32(COPROC_TICKET_STATUS)
		if status == COPROC_TICKET_OK {
			return // success
		}
		if status == COPROC_TICKET_ERROR {
			t.Fatal("ticket completed with error")
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timeout waiting for IE64 worker to complete request")
}

// TestIE64WorkerEnqueueManualComplete tests enqueue + manual response + poll
// to verify the ring buffer plumbing independently of the IE64 service binary.
func TestIE64WorkerEnqueueManualComplete(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	tmpDir := t.TempDir()
	mgr.baseDir = tmpDir

	// Start worker with HALT binary (stops immediately but registers the worker)
	binPath := filepath.Join(tmpDir, "halt.ie64")
	if err := os.WriteFile(binPath, buildIE64HaltBinary(), 0644); err != nil {
		t.Fatal(err)
	}

	writeString(bus, 0x90000, "halt.ie64")
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_NAME_PTR, 0x90000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("start failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}

	// Enqueue a request (worker slot exists even if CPU halted)
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_OP, 0)
	bus.Write32(COPROC_REQ_PTR, 0x80000)
	bus.Write32(COPROC_REQ_LEN, 32)
	bus.Write32(COPROC_RESP_PTR, 0x81000)
	bus.Write32(COPROC_RESP_CAP, 32)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("enqueue failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}
	ticket := bus.Read32(COPROC_TICKET)

	// Manually write OK status to the response descriptor (simulating IE64 worker)
	ringBase := ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE64))
	respAddr := ringBase + RING_RESPONSES_OFFSET + 0*RESP_DESC_SIZE // slot 0
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)

	// Poll should now return OK
	bus.Write32(COPROC_TICKET, ticket)
	bus.Write32(COPROC_CMD, COPROC_CMD_POLL)
	status := bus.Read32(COPROC_TICKET_STATUS)
	if status != COPROC_TICKET_OK {
		t.Fatalf("expected COPROC_TICKET_OK, got %d", status)
	}
}

// encodeIE64 builds an 8-byte IE64 instruction matching the CPU's actual
// bit layout: byte1 = rd<<3 | size<<1 | xbit, byte2 = rs<<3, byte3 = rt<<3.
func encodeIE64(opcode byte, rd, size, rs, rt byte, imm32 uint32, xbit bool) []byte {
	instr := make([]byte, 8)
	instr[0] = opcode
	x := byte(0)
	if xbit {
		x = 1
	}
	instr[1] = (rd&0x1F)<<3 | (size&0x03)<<1 | x
	instr[2] = (rs & 0x1F) << 3
	instr[3] = (rt & 0x1F) << 3
	binary.LittleEndian.PutUint32(instr[4:], imm32)
	return instr
}

func TestIE64CoprocMode_JIT(t *testing.T) {
	if !jitAvailable {
		t.Skip("JIT not available on this platform")
	}

	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.CoprocMode = true
	cpu.jitEnabled = true

	// Build a small IE64 binary using the correct CPU encoding:
	//   MOVE r1, #10       (xbit=true, imm=10)
	//   MOVE r2, #32       (xbit=true, imm=32)
	//   ADD  r3, r1, r2    (xbit=false, rd=3, rs=1, rt=2)
	//   STORE r3, r0, resultAddr  (store result to known address)
	//   HALT
	resultAddr := uint32(WORKER_IE64_BASE + 0x100)
	var prog []byte
	prog = append(prog, encodeIE64(OP_MOVE, 1, IE64_SIZE_L, 0, 0, 10, true)...)
	prog = append(prog, encodeIE64(OP_MOVE, 2, IE64_SIZE_L, 0, 0, 32, true)...)
	prog = append(prog, encodeIE64(OP_ADD, 3, IE64_SIZE_L, 1, 2, 0, false)...)
	prog = append(prog, encodeIE64(OP_STORE, 3, IE64_SIZE_L, 0, 0, resultAddr, true)...)
	prog = append(prog, buildIE64HaltBinary()...)

	// Place binary in WORKER_IE64_BASE region
	mem := bus.GetMemory()
	copy(mem[WORKER_IE64_BASE:], prog)

	// Set CPU state
	cpu.PC = uint64(WORKER_IE64_BASE)
	cpu.regs[31] = uint64(WORKER_IE64_END - 0xFF)
	cpu.running.Store(true)

	// Run jitExecute with a timeout via a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		cpu.jitExecute()
	}()

	select {
	case <-done:
		// completed
	case <-time.After(2 * time.Second):
		cpu.running.Store(false)
		<-done
		t.Fatal("jitExecute did not complete within timeout")
	}

	// Verify the result: 10 + 32 = 42
	got := bus.Read32(resultAddr)
	if got != 42 {
		t.Fatalf("expected result 42 at 0x%X, got %d", resultAddr, got)
	}
}

func TestIE64CoprocMode_PCRange(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)

	// Without CoprocMode, Execute should reject out-of-range PC
	cpu.PC = uint64(WORKER_IE64_BASE)
	cpu.running.Store(true)
	cpu.Execute()
	// Should have returned immediately (running set to false)
	if cpu.running.Load() {
		t.Fatal("Execute should reject out-of-range PC without CoprocMode")
	}

	// With CoprocMode, write a HALT instruction and Execute should work
	mem := bus.GetMemory()
	copy(mem[WORKER_IE64_BASE:], buildIE64HaltBinary())
	cpu.PC = uint64(WORKER_IE64_BASE)
	cpu.CoprocMode = true
	cpu.running.Store(true)
	cpu.Execute()
	// CPU should have halted normally (ran the HALT instruction)
}

func TestIE64WorkerStats(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	// Stats start at zero
	ops := bus.Read32(COPROC_STATS_OPS)
	if ops != 0 {
		t.Fatalf("initial ops = %d, want 0", ops)
	}

	tmpDir := t.TempDir()
	mgr.baseDir = tmpDir
	ringIdx := cpuTypeToIndex(EXEC_TYPE_IE64)
	ringBase := ringBaseAddr(ringIdx)
	svcBin := buildIE64ServiceBinary(ringBase)
	binPath := filepath.Join(tmpDir, "svc.ie64")
	if err := os.WriteFile(binPath, svcBin, 0644); err != nil {
		t.Fatal(err)
	}

	writeString(bus, 0x90000, "svc.ie64")
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_NAME_PTR, 0x90000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)

	// Enqueue a request
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_OP, 0)
	bus.Write32(COPROC_REQ_PTR, 0x80000)
	bus.Write32(COPROC_REQ_LEN, 128)
	bus.Write32(COPROC_RESP_PTR, 0x81000)
	bus.Write32(COPROC_RESP_CAP, 64)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)

	ops = bus.Read32(COPROC_STATS_OPS)
	if ops != 1 {
		t.Fatalf("after 1 enqueue, ops = %d, want 1", ops)
	}
	bytes := bus.Read32(COPROC_STATS_BYTES)
	if bytes != 128 {
		t.Fatalf("after 1 enqueue with reqLen=128, bytes = %d, want 128", bytes)
	}
}

func TestIE64Ticket0Protocol(t *testing.T) {
	_, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	// Poll ticket 0 should return OK
	mgr.mu.Lock()
	mgr.ticket = 0
	mgr.cmdPoll()
	if mgr.ticketStatus != COPROC_TICKET_OK {
		t.Fatalf("poll ticket 0: status=%d, want OK(%d)", mgr.ticketStatus, COPROC_TICKET_OK)
	}
	mgr.mu.Unlock()

	// Wait ticket 0 should return OK
	mgr.mu.Lock()
	mgr.ticket = 0
	mgr.cmdWait()
	if mgr.ticketStatus != COPROC_TICKET_OK {
		t.Fatalf("wait ticket 0: status=%d, want OK(%d)", mgr.ticketStatus, COPROC_TICKET_OK)
	}
	mgr.mu.Unlock()
}

func TestIE64CompletionIRQ(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	// Enable IRQ via MMIO
	bus.Write32(COPROC_IRQ_CTRL, 1)
	ctrl := bus.Read32(COPROC_IRQ_CTRL)
	if ctrl != 1 {
		t.Fatalf("IRQ ctrl read = %d, want 1", ctrl)
	}

	// Disable IRQ
	bus.Write32(COPROC_IRQ_CTRL, 0)
	ctrl = bus.Read32(COPROC_IRQ_CTRL)
	if ctrl != 0 {
		t.Fatalf("IRQ ctrl read = %d, want 0", ctrl)
	}
}

func TestIE64DispatchOverhead(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	// Initially zero
	overhead := bus.Read32(COPROC_DISPATCH_OVERHEAD)
	if overhead != 0 {
		t.Fatalf("initial overhead = %d, want 0", overhead)
	}

	// Set it programmatically and verify readback
	mgr.dispatchOverheadNs.Store(12345)
	overhead = bus.Read32(COPROC_DISPATCH_OVERHEAD)
	if overhead != 12345 {
		t.Fatalf("overhead = %d, want 12345", overhead)
	}
}

func TestIE64CompletedTicket(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	// Initially zero
	ct := bus.Read32(COPROC_COMPLETED_TICKET)
	if ct != 0 {
		t.Fatalf("initial completed ticket = %d, want 0", ct)
	}

	// Set and verify readback
	mgr.completedTicket.Store(42)
	ct = bus.Read32(COPROC_COMPLETED_TICKET)
	if ct != 42 {
		t.Fatalf("completed ticket = %d, want 42", ct)
	}
}

// newTestBusAndManagerExt creates a bus+manager with both the base and extended
// coprocessor MMIO ranges mapped.
func newTestBusAndManagerExt(t *testing.T) (*MachineBus, *CoprocessorManager) {
	t.Helper()
	bus, mgr := newTestBusAndManager(t)
	bus.MapIO(COPROC_EXT_BASE, COPROC_EXT_END, mgr.HandleRead, mgr.HandleWrite)
	return bus, mgr
}

func TestCoprocRingDepth(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)
	defer mgr.StopAll()

	tmpDir := t.TempDir()
	mgr.baseDir = tmpDir

	// Start worker with HALT binary (registers the worker slot)
	binPath := filepath.Join(tmpDir, "halt.ie64")
	if err := os.WriteFile(binPath, buildIE64HaltBinary(), 0644); err != nil {
		t.Fatal(err)
	}

	// Prevent auto-calibration from consuming a ring slot
	mgr.dispatchOverheadNs.Store(1)

	writeString(bus, 0x90000, "halt.ie64")
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_NAME_PTR, 0x90000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("start failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}

	// Ring should be empty
	depth := bus.Read32(COPROC_RING_DEPTH)
	if depth != 0 {
		t.Fatalf("initial ring depth = %d, want 0", depth)
	}

	// Enqueue first request
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_OP, 0)
	bus.Write32(COPROC_REQ_PTR, 0x80000)
	bus.Write32(COPROC_REQ_LEN, 32)
	bus.Write32(COPROC_RESP_PTR, 0x81000)
	bus.Write32(COPROC_RESP_CAP, 32)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("enqueue 1 failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}

	depth = bus.Read32(COPROC_RING_DEPTH)
	if depth != 1 {
		t.Fatalf("after 1 enqueue, ring depth = %d, want 1", depth)
	}

	// Enqueue second request
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_OP, 0)
	bus.Write32(COPROC_REQ_PTR, 0x80100)
	bus.Write32(COPROC_REQ_LEN, 64)
	bus.Write32(COPROC_RESP_PTR, 0x81100)
	bus.Write32(COPROC_RESP_CAP, 64)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("enqueue 2 failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}

	depth = bus.Read32(COPROC_RING_DEPTH)
	if depth != 2 {
		t.Fatalf("after 2 enqueues, ring depth = %d, want 2", depth)
	}
}

func TestCoprocWorkerUptime(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)
	defer mgr.StopAll()

	// Before any worker starts, uptime should be 0
	uptime := bus.Read32(COPROC_WORKER_UPTIME)
	if uptime != 0 {
		t.Fatalf("uptime before worker start = %d, want 0", uptime)
	}

	tmpDir := t.TempDir()
	mgr.baseDir = tmpDir

	// Build a NOP-loop binary so the worker stays alive
	var nopLoop []byte
	nopLoop = append(nopLoop, buildIE64Instr(OP_NOP64, 0, 0, 0, 0, 0, false)...)
	nopLoop = append(nopLoop, buildIE64Instr(OP_BEQ, 0, IE64_SIZE_Q, 0, 0, u32(-8), false)...)
	binPath := filepath.Join(tmpDir, "noploop.ie64")
	if err := os.WriteFile(binPath, nopLoop, 0644); err != nil {
		t.Fatal(err)
	}

	writeString(bus, 0x90000, "noploop.ie64")
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_NAME_PTR, 0x90000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("start failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}

	// Sleep briefly, then read uptime — it's in seconds so will likely be 0,
	// but must not error and must be >= 0
	time.Sleep(100 * time.Millisecond)
	uptime = bus.Read32(COPROC_WORKER_UPTIME)
	// uint32 is always >= 0; just verify it reads without panic

	// Stop worker and verify uptime resets to 0
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_CMD, COPROC_CMD_STOP)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatal("stop failed")
	}
	time.Sleep(10 * time.Millisecond)

	uptime = bus.Read32(COPROC_WORKER_UPTIME)
	if uptime != 0 {
		t.Fatalf("uptime after worker stop = %d, want 0", uptime)
	}
	_ = uptime // suppress unused warning
}

func TestCoprocStatsReset(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)
	defer mgr.StopAll()

	tmpDir := t.TempDir()
	mgr.baseDir = tmpDir

	// Start worker with HALT binary
	binPath := filepath.Join(tmpDir, "halt.ie64")
	if err := os.WriteFile(binPath, buildIE64HaltBinary(), 0644); err != nil {
		t.Fatal(err)
	}

	// Prevent auto-calibration from consuming a ring slot
	mgr.dispatchOverheadNs.Store(1)

	writeString(bus, 0x90000, "halt.ie64")
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_NAME_PTR, 0x90000)
	bus.Write32(COPROC_CMD, COPROC_CMD_START)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("start failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}

	// Enqueue a request to increment stats
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_OP, 0)
	bus.Write32(COPROC_REQ_PTR, 0x80000)
	bus.Write32(COPROC_REQ_LEN, 256)
	bus.Write32(COPROC_RESP_PTR, 0x81000)
	bus.Write32(COPROC_RESP_CAP, 64)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("enqueue failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}

	// Verify stats are non-zero
	ops := bus.Read32(COPROC_STATS_OPS)
	if ops == 0 {
		t.Fatal("expected COPROC_STATS_OPS > 0 after enqueue")
	}
	bytes := bus.Read32(COPROC_STATS_BYTES)
	if bytes == 0 {
		t.Fatal("expected COPROC_STATS_BYTES > 0 after enqueue")
	}

	// Write 1 to COPROC_STATS_RESET to clear stats
	bus.Write32(COPROC_STATS_RESET, 1)

	// Verify stats are zeroed
	ops = bus.Read32(COPROC_STATS_OPS)
	if ops != 0 {
		t.Fatalf("after stats reset, COPROC_STATS_OPS = %d, want 0", ops)
	}
	bytes = bus.Read32(COPROC_STATS_BYTES)
	if bytes != 0 {
		t.Fatalf("after stats reset, COPROC_STATS_BYTES = %d, want 0", bytes)
	}
}

func TestCoprocBusyPct(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)
	defer mgr.StopAll()

	// With no workers running, busy% should be 0
	pct := bus.Read32(COPROC_BUSY_PCT)
	if pct != 0 {
		t.Fatalf("idle COPROC_BUSY_PCT = %d, want 0", pct)
	}
}

func TestBusyPct_ClearedByPoll_NonM68KMode(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)

	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE64] = newOpenSyntheticWorker(EXEC_TYPE_IE64)
	mgr.mu.Unlock()

	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	bus.Write32(COPROC_REQ_PTR, 0x80000)
	bus.Write32(COPROC_REQ_LEN, 4)
	bus.Write32(COPROC_RESP_PTR, 0x81000)
	bus.Write32(COPROC_RESP_CAP, 4)
	bus.Write32(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if bus.Read32(COPROC_CMD_STATUS) != COPROC_STATUS_OK {
		t.Fatalf("enqueue failed: err=%d", bus.Read32(COPROC_CMD_ERROR))
	}
	ticket := bus.Read32(COPROC_TICKET)

	respAddr := ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE64)) + RING_RESPONSES_OFFSET
	bus.Write32(respAddr+RESP_TICKET_OFF, ticket)
	bus.Write32(respAddr+RESP_STATUS_OFF, COPROC_TICKET_OK)

	bus.Write32(COPROC_TICKET, ticket)
	bus.Write32(COPROC_CMD, COPROC_CMD_POLL)
	if got := bus.Read32(COPROC_TICKET_STATUS); got != COPROC_TICKET_OK {
		t.Fatalf("poll status=%d, want OK", got)
	}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if mgr.workerBusy {
		t.Fatal("workerBusy remained true after terminal poll")
	}
}

func TestRingDepth_PerCPU(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)

	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32] = newOpenSyntheticWorker(EXEC_TYPE_IE32)
	mgr.mu.Unlock()

	ie32Ring := ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE32))
	ie64Ring := ringBaseAddr(cpuTypeToIndex(EXEC_TYPE_IE64))
	bus.Write8(ie32Ring+RING_HEAD_OFFSET, 3)
	bus.Write8(ie32Ring+RING_TAIL_OFFSET, 1)
	bus.Write8(ie64Ring+RING_HEAD_OFFSET, 0)
	bus.Write8(ie64Ring+RING_TAIL_OFFSET, 0)

	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	if got := bus.Read32(COPROC_RING_DEPTH); got != 2 {
		t.Fatalf("IE32 ring depth=%d, want 2", got)
	}
}

func TestWorkerUptime_PerCPU(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)

	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_Z80] = newOpenSyntheticWorker(EXEC_TYPE_Z80)
	mgr.workerStartTime[EXEC_TYPE_Z80] = time.Now().Add(-2 * time.Second)
	mgr.mu.Unlock()

	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_Z80)
	if got := bus.Read32(COPROC_WORKER_UPTIME); got == 0 {
		t.Fatalf("Z80 worker uptime=%d, want nonzero", got)
	}
}
