//go:build headless

package main

// Phase 4b: per-CPU second-instance bootstrap. The JIT-capable types (M68K,
// x86, IE64) run two independent worker instances, each with its own RAM window
// and ring. IE32/6502/Z80 reject instance 1 with COPROC_ERR_INVALID_INSTANCE.

import (
	"testing"
	"time"
)

func startMemInstance(t *testing.T, bus *MachineBus, mgr *CoprocessorManager, cpuType, instance uint32, img []byte, blobAddr uint32) uint32 {
	t.Helper()
	copy(bus.GetMemory()[blobAddr:], img)
	mgr.HandleWrite(COPROC_CPU_TYPE, cpuType)
	mgr.HandleWrite(COPROC_INSTANCE, instance)
	mgr.HandleWrite(COPROC_REQ_PTR, blobAddr)
	mgr.HandleWrite(COPROC_REQ_LEN, uint32(len(img)))
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_START_MEM)
	return mgr.HandleRead(COPROC_CMD_ERROR)
}

// TestCoprocIE64_TwoInstancesIndependent runs a real IE64 poll-loop service in
// both instances and asserts each answers from its own ring with no cross-talk.
func TestCoprocIE64_TwoInstancesIndependent(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t) // gate disabled
	defer mgr.StopAll()

	img0 := buildIE64ServiceBinary(ringBaseAddr(coprocRingIndex(EXEC_TYPE_IE64, 0)))
	img1 := buildIE64ServiceBinary(ringBaseAddr(coprocRingIndex(EXEC_TYPE_IE64, 1)))
	if e := startMemInstance(t, bus, mgr, EXEC_TYPE_IE64, 0, img0, 0x20000); e != COPROC_ERR_NONE {
		t.Fatalf("IE64#0 start error %d", e)
	}
	if e := startMemInstance(t, bus, mgr, EXEC_TYPE_IE64, 1, img1, 0x28000); e != COPROC_ERR_NONE {
		t.Fatalf("IE64#1 start error %d", e)
	}

	// Distinct windows and CPU objects.
	mgr.mu.Lock()
	w0, w1 := mgr.workers[EXEC_TYPE_IE64][0], mgr.workers[EXEC_TYPE_IE64][1]
	mgr.mu.Unlock()
	if w0 == nil || w1 == nil {
		t.Fatal("both IE64 instances should be online")
	}
	if w0.debugCPU == w1.debugCPU || w0.loadBase == w1.loadBase {
		t.Fatal("IE64 instances share CPU state or window")
	}

	waitTicket := func(instance uint32) uint32 {
		mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
		mgr.HandleWrite(COPROC_INSTANCE, instance)
		mgr.HandleWrite(COPROC_OP, 0)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
		if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
			t.Fatalf("enqueue to IE64#%d failed: %d", instance, mgr.HandleRead(COPROC_CMD_ERROR))
		}
		tk := mgr.HandleRead(COPROC_TICKET)
		mgr.HandleWrite(COPROC_TICKET, tk)
		mgr.HandleWrite(COPROC_TIMEOUT, 3000)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_WAIT)
		return mgr.HandleRead(COPROC_TICKET_STATUS)
	}
	if st := waitTicket(0); st != COPROC_TICKET_OK {
		t.Errorf("IE64#0 ticket status = %d, want OK", st)
	}
	if st := waitTicket(1); st != COPROC_TICKET_OK {
		t.Errorf("IE64#1 ticket status = %d, want OK", st)
	}
}

// TestCoprocX86_TwoInstancesIndependent checks the x86 instance-1 bootstrap:
// two live instances with distinct windows, rings, and CPU state, and enqueue
// routing to the correct ring.
func TestCoprocX86_TwoInstancesIndependent(t *testing.T) {
	bus, mgr := newTestBusAndManager(t) // gate disabled
	defer mgr.StopAll()

	spin := []byte{0xEB, 0xFE} // JMP $-2
	if e := startMemInstance(t, bus, mgr, EXEC_TYPE_X86, 0, spin, 0x20000); e != COPROC_ERR_NONE {
		t.Fatalf("x86#0 start error %d", e)
	}
	if e := startMemInstance(t, bus, mgr, EXEC_TYPE_X86, 1, spin, 0x21000); e != COPROC_ERR_NONE {
		t.Fatalf("x86#1 start error %d", e)
	}

	mgr.mu.Lock()
	w0, w1 := mgr.workers[EXEC_TYPE_X86][0], mgr.workers[EXEC_TYPE_X86][1]
	mgr.mu.Unlock()
	if w0 == nil || w1 == nil {
		t.Fatal("both x86 instances should be online")
	}
	b0, _, _, _ := workerWindow(EXEC_TYPE_X86, 0)
	b1, _, _, _ := workerWindow(EXEC_TYPE_X86, 1)
	if w0.loadBase != b0 || w1.loadBase != b1 || b0 == b1 {
		t.Fatalf("x86 instances not in distinct windows: %#x / %#x", w0.loadBase, w1.loadBase)
	}
	if w0.debugCPU == w1.debugCPU {
		t.Fatal("x86 instances share CPU state")
	}

	// Enqueue to each routes to its own ring.
	for _, inst := range []uint32{0, 1} {
		mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_X86)
		mgr.HandleWrite(COPROC_INSTANCE, inst)
		mgr.HandleWrite(COPROC_OP, 1)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
		if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
			t.Fatalf("x86#%d enqueue error %d", inst, mgr.HandleRead(COPROC_CMD_ERROR))
		}
		ring := ringBaseAddr(coprocRingIndex(EXEC_TYPE_X86, inst))
		if head := bus.Read8(ring + RING_HEAD_OFFSET); head != 1 {
			t.Errorf("x86#%d ring head = %d, want 1", inst, head)
		}
	}
}

// buildIE64SeededServiceBinary is buildIE64ServiceBinary with the ring base
// taken from the host-seeded r30 register instead of a baked immediate, so ONE
// image serves whichever instance ring the manager assigned. Mirrors the
// iewarp bootstrap-patch contract.
func buildIE64SeededServiceBinary() []byte {
	var prog []byte
	emit := func(opcode byte, rd, size, rs, rt byte, imm32 uint32, xbit bool) {
		prog = append(prog, buildIE64Instr(opcode, rd, size, rs, rt, imm32, xbit)...)
	}
	// r30 = ring base (seeded by createIE64Worker); no MOVE here.
	pollOffset := uint32(len(prog))
	emit(OP_LOAD, 1, IE64_SIZE_B, 30, 0, RING_HEAD_OFFSET, true)
	emit(OP_LOAD, 2, IE64_SIZE_B, 30, 0, RING_TAIL_OFFSET, true)
	currentOffset := uint32(len(prog))
	emit(OP_BEQ, 0, IE64_SIZE_Q, 1, 2, uint32(int32(pollOffset)-int32(currentOffset)), false)
	emit(OP_MULU, 3, IE64_SIZE_L, 2, 0, RESP_DESC_SIZE, true)
	emit(OP_ADD, 3, IE64_SIZE_L, 3, 0, RING_RESPONSES_OFFSET, true)
	emit(OP_ADD, 4, IE64_SIZE_L, 30, 3, 0, false)
	emit(OP_MOVE, 5, IE64_SIZE_L, 0, 0, COPROC_TICKET_OK, true)
	emit(OP_STORE, 5, IE64_SIZE_L, 4, 0, RESP_STATUS_OFF, true)
	emit(OP_ADD, 2, IE64_SIZE_L, 2, 0, 1, true)
	emit(OP_LOAD, 6, IE64_SIZE_B, 30, 0, RING_CAPACITY_OFFSET, true)
	emit(OP_MOD64, 2, IE64_SIZE_L, 2, 6, 0, false)
	emit(OP_STORE, 2, IE64_SIZE_B, 30, 0, RING_TAIL_OFFSET, true)
	loopBackOffset := uint32(len(prog))
	emit(OP_BRA, 0, IE64_SIZE_Q, 0, 0, uint32(int32(pollOffset)-int32(loopBackOffset)), false)
	return prog
}

// TestCoprocIE64_SeededRingBaseServesBothInstances proves the P1 bootstrap-seed
// fix: a single fixed image (no baked ring) started as instance 0 and instance
// 1 answers on the correct ring for each, because the manager seeds r30 with
// the assigned ring base. Without the seed, instance 1 would poll ring 0.
func TestCoprocIE64_SeededRingBaseServesBothInstances(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t) // gate disabled
	defer mgr.StopAll()

	img := buildIE64SeededServiceBinary()
	if e := startMemInstance(t, bus, mgr, EXEC_TYPE_IE64, 0, img, 0x20000); e != COPROC_ERR_NONE {
		t.Fatalf("IE64#0 start error %d", e)
	}
	if e := startMemInstance(t, bus, mgr, EXEC_TYPE_IE64, 1, img, 0x28000); e != COPROC_ERR_NONE {
		t.Fatalf("IE64#1 start error %d", e)
	}

	wait := func(instance uint32) uint32 {
		mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_IE64)
		mgr.HandleWrite(COPROC_INSTANCE, instance)
		mgr.HandleWrite(COPROC_OP, 0)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
		if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
			t.Fatalf("enqueue IE64#%d failed: %d", instance, mgr.HandleRead(COPROC_CMD_ERROR))
		}
		tk := mgr.HandleRead(COPROC_TICKET)
		mgr.HandleWrite(COPROC_TICKET, tk)
		mgr.HandleWrite(COPROC_TIMEOUT, 3000)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_WAIT)
		return mgr.HandleRead(COPROC_TICKET_STATUS)
	}
	if st := wait(0); st != COPROC_TICKET_OK {
		t.Errorf("IE64#0 (seeded) status = %d, want OK", st)
	}
	if st := wait(1); st != COPROC_TICKET_OK {
		t.Errorf("IE64#1 (seeded) status = %d, want OK", st)
	}
}

func TestCoproc_SingleInstanceTypesReject(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	defer mgr.StopAll()

	spin := []byte{0x00, 0x00}
	for _, ct := range []uint32{EXEC_TYPE_IE32, EXEC_TYPE_6502, EXEC_TYPE_Z80} {
		e := startMemInstance(t, bus, mgr, ct, 1, spin, 0x20000)
		if e != COPROC_ERR_INVALID_INSTANCE {
			t.Errorf("type %d instance 1 START error = %d, want INVALID_INSTANCE", ct, e)
		}
		if mgr.HandleRead(COPROC_CMD_STATUS) != COPROC_STATUS_ERROR {
			t.Errorf("type %d instance 1 START should report ERROR", ct)
		}
	}
	// Let any transient goroutines settle before StopAll.
	time.Sleep(5 * time.Millisecond)
}