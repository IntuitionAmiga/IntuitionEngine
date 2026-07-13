package main

// Phase 1b: enforced START-time layout-version handshake. A conforming worker
// echoes COPROC_LAYOUT_VERSION into its ring's RING_ACK_VERSION_OFFSET at
// startup; the manager refuses to route work to one that does not.

import (
	"encoding/binary"
	"testing"
	"time"
)

// newGateManager builds a manager with the version gate enabled and both the
// primary and EXT2 register blocks mapped.
func newGateManager(t *testing.T) (*MachineBus, *CoprocessorManager) {
	t.Helper()
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	mgr.versionGateTimeout = 60 * time.Millisecond
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)
	bus.MapIO(COPROC_EXT2_BASE, COPROC_EXT2_END, mgr.HandleRead, mgr.HandleWrite)
	return bus, mgr
}

// conformingM68KImage builds a minimal big-endian M68K service that writes
// COPROC_LAYOUT_VERSION to its ring's ack byte, then spins: MOVE.B #ver,(ack).L
// followed by BRA.S -2.
func conformingM68KImage(cpuType, instance uint32) []byte {
	ackAddr := ringBaseAddr(coprocRingIndex(cpuType, instance)) + RING_ACK_VERSION_OFFSET
	img := []byte{0x13, 0xFC, 0x00, COPROC_LAYOUT_VERSION} // MOVE.B #ver,(abs).L
	addr := make([]byte, 4)
	binary.BigEndian.PutUint32(addr, ackAddr)
	img = append(img, addr...)
	img = append(img, 0x60, 0xFE) // BRA.S -2
	return img
}

func startMemImage(t *testing.T, bus *MachineBus, mgr *CoprocessorManager, cpuType, instance uint32, img []byte) uint32 {
	t.Helper()
	const blobAddr = 0x20000
	copy(bus.GetMemory()[blobAddr:], img)
	mgr.HandleWrite(COPROC_CPU_TYPE, cpuType)
	mgr.HandleWrite(COPROC_INSTANCE, instance)
	mgr.HandleWrite(COPROC_REQ_PTR, blobAddr)
	mgr.HandleWrite(COPROC_REQ_LEN, uint32(len(img)))
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_START_MEM)
	return mgr.HandleRead(COPROC_CMD_ERROR)
}

func TestVersionGate_ConformingWorkerAccepted(t *testing.T) {
	bus, mgr := newGateManager(t)
	defer mgr.StopAll()

	img := conformingM68KImage(EXEC_TYPE_M68K, 0)
	if e := startMemImage(t, bus, mgr, EXEC_TYPE_M68K, 0, img); e != COPROC_ERR_NONE {
		t.Fatalf("conforming worker rejected: error %d", e)
	}
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
		t.Fatalf("START status = %d, want OK", st)
	}
	if !mgr.IsWorkerRunning(EXEC_TYPE_M68K) {
		t.Fatal("conforming worker not running after START")
	}

	// ENQUEUE must route (a descriptor is published).
	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 0)
	mgr.HandleWrite(COPROC_OP, 5)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
		t.Fatalf("enqueue rejected: error %d", mgr.HandleRead(COPROC_CMD_ERROR))
	}
}

// TestVersionGate_AckClearedBetweenStarts pins that a leftover ack from a
// previous conforming worker cannot let a stale replacement pass the gate.
func TestVersionGate_AckClearedBetweenStarts(t *testing.T) {
	bus, mgr := newGateManager(t)
	defer mgr.StopAll()

	// First worker acks and is accepted.
	if e := startMemImage(t, bus, mgr, EXEC_TYPE_M68K, 0, conformingM68KImage(EXEC_TYPE_M68K, 0)); e != COPROC_ERR_NONE {
		t.Fatalf("conforming worker rejected: %d", e)
	}
	// Confirm the ring ack is set.
	ackAddr := ringBaseAddr(coprocRingIndex(EXEC_TYPE_M68K, 0)) + RING_ACK_VERSION_OFFSET
	if bus.Read8(ackAddr) != COPROC_LAYOUT_VERSION {
		t.Fatal("expected ring ack set after conforming start")
	}

	// Stop it, then start a stale spin image that never acks. It must be
	// rejected despite the leftover ack from the first worker.
	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 0)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_STOP)

	stale := []byte{0x60, 0xFE}
	// replace=false path: the slot is free after stop, so START_MEM starts fresh.
	if e := startMemImage(t, bus, mgr, EXEC_TYPE_M68K, 0, stale); e != COPROC_ERR_STALE_WORKER {
		t.Fatalf("stale replacement accepted on leftover ack: error %d", e)
	}
	if mgr.IsWorkerRunning(EXEC_TYPE_M68K) {
		t.Fatal("stale replacement left running")
	}
}

// TestVersionGate_AckAfterOwnershipLostFails pins that a worker which acks but
// no longer owns its slot (stopped or replaced while m.mu was released for the
// ack poll) does NOT report START success. Exercises awaitWorkerAckLocked's
// acked-but-not-owner branch directly.
func TestVersionGate_AckAfterOwnershipLostFails(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	mgr.versionGateTimeout = 50 * time.Millisecond

	// Pre-set the ring ack so the poll observes acked=true immediately.
	ackAddr := ringBaseAddr(coprocRingIndex(EXEC_TYPE_M68K, 0)) + RING_ACK_VERSION_OFFSET
	bus.Write8(ackAddr, COPROC_LAYOUT_VERSION)

	// The worker under test; a synthetic one whose stopCPU closes done so the
	// orphan teardown returns promptly.
	done := make(chan struct{})
	stopped := false
	w := &CoprocWorker{cpuType: EXEC_TYPE_M68K, monitorID: -1, done: done,
		stopCPU: func() { stopped = true; close(done) }}

	// Slot is owned by a DIFFERENT worker, simulating a concurrent replace.
	other := newOpenSyntheticWorker(EXEC_TYPE_M68K)
	slot := &other

	mgr.mu.Lock()
	err := mgr.awaitWorkerAckLocked(EXEC_TYPE_M68K, 0, slot, w)
	mgr.mu.Unlock()

	if err == nil {
		t.Fatal("START reported success for a worker that lost slot ownership")
	}
	if le, ok := err.(*coprocLifecycleError); !ok || le.code != COPROC_ERR_LOAD_FAILED {
		t.Errorf("error = %v, want COPROC_ERR_LOAD_FAILED", err)
	}
	if !stopped {
		t.Error("orphaned worker was not stopped")
	}
	if *slot != other {
		t.Error("the other worker's slot was disturbed")
	}
}

func TestVersionGate_StaleImageRejectedAtStart(t *testing.T) {
	bus, mgr := newGateManager(t)
	defer mgr.StopAll()

	// Bare spin loop never acks the layout version.
	stale := []byte{0x60, 0xFE}
	if e := startMemImage(t, bus, mgr, EXEC_TYPE_M68K, 0, stale); e != COPROC_ERR_STALE_WORKER {
		t.Fatalf("stale worker error = %d, want COPROC_ERR_STALE_WORKER", e)
	}
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_ERROR {
		t.Fatalf("START status = %d, want ERROR", st)
	}
	if mgr.IsWorkerRunning(EXEC_TYPE_M68K) {
		t.Fatal("stale worker left running after rejection")
	}
	// No request descriptor was ever published to the ring.
	ring := ringBaseAddr(coprocRingIndex(EXEC_TYPE_M68K, 0))
	if head := bus.Read8(ring + RING_HEAD_OFFSET); head != 0 {
		t.Errorf("ring head = %d, want 0 (no work routed)", head)
	}
}

// TestVersionGate_EnqueueRejectedWhilePending pins that a worker installed but
// still awaiting its version-gate ack (the window where START polls with m.mu
// released) receives no enqueued work, and that enqueue is allowed once it
// clears the gate.
func TestVersionGate_EnqueueRejectedWhilePending(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir()) // gate enabled
	bus.MapIO(COPROC_BASE, COPROC_END, mgr.HandleRead, mgr.HandleWrite)

	w := newOpenSyntheticWorker(EXEC_TYPE_M68K)
	w.gatePending = true
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_M68K][0] = w
	mgr.mu.Unlock()

	mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	mgr.HandleWrite(COPROC_INSTANCE, 0)
	mgr.HandleWrite(COPROC_OP, 1)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if e := mgr.HandleRead(COPROC_CMD_ERROR); e != COPROC_ERR_STALE_WORKER {
		t.Errorf("enqueue to startup-pending worker error = %d, want STALE_WORKER", e)
	}
	ring := ringBaseAddr(coprocRingIndex(EXEC_TYPE_M68K, 0))
	if head := bus.Read8(ring + RING_HEAD_OFFSET); head != 0 {
		t.Errorf("descriptor routed to pending worker: ring head = %d", head)
	}

	// Once the worker clears the gate, enqueue is allowed.
	w.mu.Lock()
	w.gatePending = false
	w.gateAcked = true
	w.mu.Unlock()
	bus.Write8(ring+RING_ACK_VERSION_OFFSET, COPROC_LAYOUT_VERSION)
	mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
	if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
		t.Errorf("enqueue after ack rejected: error %d", mgr.HandleRead(COPROC_CMD_ERROR))
	}
}
