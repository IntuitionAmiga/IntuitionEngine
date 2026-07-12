package main

// Multi-instance coprocessor workers: a second worker of the same CPU type
// selected via COPROC_INSTANCE, with its own ring (index 6 for M68K#1) and
// RAM window (WORKER_M68K2). Only M68K supports a second instance today.

import (
	"testing"
	"time"
)

// m68kSpinLoop is BRA.S -2 stored big-endian (see TestCoprocWorkerM68KStartStop).
var m68kSpinLoop = []byte{0x60, 0xFE}

func TestCoprocRingIndex_InstanceMapping(t *testing.T) {
	cases := []struct {
		cpuType  uint32
		instance uint32
		want     int
	}{
		{EXEC_TYPE_M68K, 0, 2},
		{EXEC_TYPE_M68K, 1, 6},
		{EXEC_TYPE_M68K, 2, -1},
		{EXEC_TYPE_IE64, 0, 5},
		{EXEC_TYPE_IE64, 1, -1},
		{EXEC_TYPE_IE32, 1, -1},
		{EXEC_TYPE_NONE, 0, -1},
	}
	for _, c := range cases {
		if got := coprocRingIndex(c.cpuType, c.instance); got != c.want {
			t.Errorf("coprocRingIndex(%d, %d) = %d, want %d", c.cpuType, c.instance, got, c.want)
		}
	}
	if ringBaseAddr(6) != 0x791300 {
		t.Errorf("ring 6 base = %#x, want 0x791300", ringBaseAddr(6))
	}
	// A ring's true content size is 0x308 (see COPROC_RING6_BASE caveat):
	// ring 6 must start clear of ring 5's slot-15 response overflow and its
	// own overflow must stay inside the mailbox window.
	ring5End := ringBaseAddr(5) + RING_RESPONSES_OFFSET + RING_CAPACITY*RESP_DESC_SIZE
	if ringBaseAddr(6) < ring5End {
		t.Errorf("ring 6 base %#x overlaps ring 5 content ending %#x", ringBaseAddr(6), ring5End)
	}
	if end := ringBaseAddr(6) + RING_RESPONSES_OFFSET + RING_CAPACITY*RESP_DESC_SIZE; end > MAILBOX_BASE+MAILBOX_SIZE {
		t.Errorf("ring 6 content end %#x exceeds mailbox window %#x", end, MAILBOX_BASE+MAILBOX_SIZE)
	}
}

func TestCoprocInstanceLimit(t *testing.T) {
	if got := coprocInstanceLimit(EXEC_TYPE_M68K); got != 2 {
		t.Errorf("M68K instance limit = %d, want 2", got)
	}
	for _, ct := range []uint32{EXEC_TYPE_IE32, EXEC_TYPE_6502, EXEC_TYPE_Z80, EXEC_TYPE_X86, EXEC_TYPE_IE64} {
		if got := coprocInstanceLimit(ct); got != 1 {
			t.Errorf("type %d instance limit = %d, want 1", ct, got)
		}
	}
	if got := coprocInstanceLimit(EXEC_TYPE_NONE); got != 0 {
		t.Errorf("invalid type instance limit = %d, want 0", got)
	}
}

func TestCoprocInstanceLabel(t *testing.T) {
	if got := coprocInstanceLabel(EXEC_TYPE_M68K, 0); got != "coproc:M68K" {
		t.Errorf("instance 0 label = %q", got)
	}
	if got := coprocInstanceLabel(EXEC_TYPE_M68K, 1); got != "coproc:M68K#1" {
		t.Errorf("instance 1 label = %q", got)
	}
}

// startM68KInstanceViaMMIO stages a spin-loop image in guest RAM and issues
// COPROC_CMD_START_MEM for the given instance through the MMIO surface.
func startM68KInstanceViaMMIO(t *testing.T, bus *MachineBus, mgr *CoprocessorManager, instance uint32) {
	t.Helper()
	const blobAddr = 0x20000
	mem := bus.GetMemory()
	copy(mem[blobAddr:], m68kSpinLoop)

	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, instance)
	mgr.HandleWrite(COPROC_REQ_PTR, blobAddr)
	mgr.HandleWrite(COPROC_REQ_LEN, uint32(len(m68kSpinLoop)))
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_START_MEM)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
		t.Fatalf("START_MEM instance %d failed: status %d error %d",
			instance, st, mgr.HandleRead(COPROC_CMD_ERROR))
	}
}

func TestCoprocInstance_SecondM68KWorkerLoadsInSecondWindow(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	startM68KInstanceViaMMIO(t, bus, mgr, 0)
	startM68KInstanceViaMMIO(t, bus, mgr, 1)

	mem := bus.GetMemory()
	if mem[WORKER_M68K_BASE] != 0x60 || mem[WORKER_M68K_BASE+1] != 0xFE {
		t.Errorf("instance 0 image not at %#x", WORKER_M68K_BASE)
	}
	if mem[WORKER_M68K2_BASE] != 0x60 || mem[WORKER_M68K2_BASE+1] != 0xFE {
		t.Errorf("instance 1 image not at %#x", WORKER_M68K2_BASE)
	}

	state := mgr.HandleRead(COPROC_WORKER_STATE)
	want := uint32(1<<EXEC_TYPE_M68K | 1<<7)
	if state&want != want {
		t.Errorf("worker state %#x missing bits %#x", state, want)
	}

	mgr.mu.Lock()
	w0 := mgr.workers[EXEC_TYPE_M68K]
	w1 := mgr.workersAlt[EXEC_TYPE_M68K]
	mgr.mu.Unlock()
	if w0 == nil || w1 == nil {
		t.Fatalf("expected both instances online, got %v / %v", w0 != nil, w1 != nil)
	}
	if w1.loadBase != WORKER_M68K2_BASE || w1.loadEnd != WORKER_M68K2_END {
		t.Errorf("instance 1 window %#x-%#x, want %#x-%#x",
			w1.loadBase, w1.loadEnd, uint32(WORKER_M68K2_BASE), uint32(WORKER_M68K2_END))
	}
}

func TestCoprocInstance_EnqueueRoutesToRing6(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	startM68KInstanceViaMMIO(t, bus, mgr, 1)

	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_OP, 42)
	mgr.HandleWrite(COPROC_REQ_PTR, 0)
	mgr.HandleWrite(COPROC_REQ_LEN, 0)
	mgr.HandleWrite(COPROC_RESP_PTR, 0)
	mgr.HandleWrite(COPROC_RESP_CAP, 0)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
		t.Fatalf("enqueue failed: error %d", mgr.HandleRead(COPROC_CMD_ERROR))
	}
	ticket := mgr.HandleRead(COPROC_TICKET)
	if ticket == 0 {
		t.Fatal("no ticket returned")
	}

	ring6 := ringBaseAddr(6)
	if head := bus.Read8(ring6 + RING_HEAD_OFFSET); head != 1 {
		t.Errorf("ring 6 head = %d, want 1", head)
	}
	entry := ring6 + RING_ENTRIES_OFFSET
	if got := bus.Read32(entry + REQ_TICKET_OFF); got != ticket {
		t.Errorf("ring 6 descriptor ticket = %d, want %d", got, ticket)
	}
	if got := bus.Read32(entry + REQ_OP_OFF); got != 42 {
		t.Errorf("ring 6 descriptor op = %d, want 42", got)
	}
	// The default M68K ring must be untouched.
	if head := bus.Read8(ringBaseAddr(2) + RING_HEAD_OFFSET); head != 0 {
		t.Errorf("ring 2 head = %d, want 0", head)
	}
}

func TestCoprocInstance_InvalidInstanceRejected(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	const blobAddr = 0x20000
	mem := bus.GetMemory()
	copy(mem[blobAddr:], m68kSpinLoop)

	// IE64 has no second instance.
	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_REQ_PTR, blobAddr)
	mgr.HandleWrite(COPROC_REQ_LEN, uint32(len(m68kSpinLoop)))
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_START_MEM)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_ERROR {
		t.Fatal("expected START_MEM to fail for IE64 instance 1")
	}
	if e := mgr.HandleRead(COPROC_CMD_ERROR); e != COPROC_ERR_INVALID_CPU {
		t.Errorf("error = %d, want COPROC_ERR_INVALID_CPU", e)
	}

	// M68K instance 2 is beyond the limit.
	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 2)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if e := mgr.HandleRead(COPROC_CMD_ERROR); e != COPROC_ERR_INVALID_CPU {
		t.Errorf("enqueue instance 2 error = %d, want COPROC_ERR_INVALID_CPU", e)
	}
}

func TestCoprocInstance_StopIsPerInstance(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	startM68KInstanceViaMMIO(t, bus, mgr, 0)
	startM68KInstanceViaMMIO(t, bus, mgr, 1)

	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_STOP)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
		t.Fatalf("stop instance 1 failed: error %d", mgr.HandleRead(COPROC_CMD_ERROR))
	}

	deadline := time.Now().Add(time.Second)
	for {
		state := mgr.HandleRead(COPROC_WORKER_STATE)
		if state&(1<<7) == 0 {
			if state&(1<<EXEC_TYPE_M68K) == 0 {
				t.Error("instance 0 went down with instance 1")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("instance 1 still reported online after stop")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCoprocInstance_WorkerDownMarksOnlyItsTickets(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	startM68KInstanceViaMMIO(t, bus, mgr, 0)
	startM68KInstanceViaMMIO(t, bus, mgr, 1)

	enqueue := func(instance uint32) uint32 {
		mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
		mgr.HandleWrite(COPROC_INSTANCE, instance)
		mgr.HandleWrite(COPROC_OP, 7)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
		return mgr.HandleRead(COPROC_TICKET)
	}
	t0 := enqueue(0)
	t1 := enqueue(1)

	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_STOP)

	poll := func(ticket uint32) uint32 {
		mgr.HandleWrite(COPROC_TICKET, ticket)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_POLL)
		return mgr.HandleRead(COPROC_TICKET_STATUS)
	}
	if st := poll(t1); st != COPROC_TICKET_WORKER_DOWN {
		t.Errorf("instance 1 ticket status = %d, want WORKER_DOWN", st)
	}
	// The spin-loop worker never serves its ring; instance 0's ticket must
	// still be pending, not worker-down.
	if st := poll(t0); st != COPROC_TICKET_PENDING && st != COPROC_TICKET_RUNNING {
		t.Errorf("instance 0 ticket status = %d, want pending/running", st)
	}
}

func TestCoprocInstance_SecondWindowNotInSharedSwapSkip(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.CoprocMode = true

	if !cpu.isCoprocSharedAddr(0x400000) {
		t.Error("0x400000 should stay in the shared skip range")
	}
	if !cpu.isCoprocSharedAddr(0x790600) {
		t.Error("mailbox should stay in the shared skip range")
	}
	for _, a := range []uint32{WORKER_M68K2_BASE, WORKER_M68K2_BASE + 0x1000, WORKER_M68K2_END} {
		if cpu.isCoprocSharedAddr(a) {
			t.Errorf("%#x is worker RAM and must byte-swap like normal BE memory", a)
		}
	}
	if !cpu.isCoprocSharedAddr(WORKER_M68K2_END + 1) {
		t.Error("address after the worker window should stay shared")
	}
}

func TestCalibration_RestoresGuestShadowRegisters(t *testing.T) {
	_, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	// Guest-visible command state staged before calibration runs.
	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 1)
	mgr.HandleWrite(COPROC_OP, 0x1234)
	mgr.HandleWrite(COPROC_REQ_PTR, 0xAAAA)
	mgr.HandleWrite(COPROC_REQ_LEN, 0x40)
	mgr.HandleWrite(COPROC_RESP_PTR, 0xBBBB)
	mgr.HandleWrite(COPROC_RESP_CAP, 0x20)

	mgr.calibrateDispatchOverhead()

	checks := []struct {
		reg  uint32
		want uint32
		name string
	}{
		{COPROC_CPU_TYPE, EXEC_TYPE_M68K, "cpuType"},
		{COPROC_INSTANCE, 1, "instance"},
		{COPROC_OP, 0x1234, "op"},
		{COPROC_REQ_PTR, 0xAAAA, "reqPtr"},
		{COPROC_REQ_LEN, 0x40, "reqLen"},
		{COPROC_RESP_PTR, 0xBBBB, "respPtr"},
		{COPROC_RESP_CAP, 0x20, "respCap"},
	}
	for _, c := range checks {
		if got := mgr.HandleRead(c.reg); got != c.want {
			t.Errorf("calibration clobbered %s: got %#x, want %#x", c.name, got, c.want)
		}
	}
}
