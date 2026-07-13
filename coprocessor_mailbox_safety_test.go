package main

// Phase 6: mailbox safety. With RING_STRIDE (0x400) >= a ring's true content
// size (RING_RESPONSES_OFFSET + RING_CAPACITY*RESP_DESC_SIZE = 0x308), the last
// slot of any ring cannot overflow into the next ring's header.

import (
	"sync"
	"testing"
)

// TestCoproc_ConcurrentProducers drives many producer goroutines at one worker.
// Each producer serialises its MMIO write sequence with a bus-arbitration mutex
// (the register set is single-driver by design), so this exercises the
// manager's internal locking, ticket allocation, and ring advancement under
// concurrency. Run under -race to catch internal data races.
func TestCoproc_ConcurrentProducers(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	mgr.mu.Lock()
	mgr.workers[EXEC_TYPE_IE32][0] = newOpenSyntheticWorker(EXEC_TYPE_IE32)
	mgr.mu.Unlock()

	const producers = 12 // < ring usable capacity (15)
	var busMu sync.Mutex
	var wg sync.WaitGroup
	tickets := make([]uint32, producers)

	for i := 0; i < producers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			busMu.Lock()
			mgr.HandleWrite(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
			mgr.HandleWrite(COPROC_INSTANCE, 0)
			mgr.HandleWrite(COPROC_OP, uint32(idx))
			mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
			ok := mgr.HandleRead(COPROC_CMD_STATUS) == COPROC_STATUS_OK
			tk := mgr.HandleRead(COPROC_TICKET)
			busMu.Unlock()
			if ok {
				tickets[idx] = tk
			}
		}(i)
	}
	wg.Wait()

	// Every producer got a distinct, non-zero ticket.
	seen := map[uint32]bool{}
	for i, tk := range tickets {
		if tk == 0 {
			t.Errorf("producer %d got ticket 0", i)
		}
		if seen[tk] {
			t.Errorf("duplicate ticket %d", tk)
		}
		seen[tk] = true
	}

	// Ring head advanced by exactly the number of enqueues; no corruption.
	ring := ringBaseAddr(coprocRingIndex(EXEC_TYPE_IE32, 0))
	if head := bus.Read8(ring + RING_HEAD_OFFSET); head != producers {
		t.Errorf("ring head = %d, want %d", head, producers)
	}
	// Each occupied request slot holds a distinct known ticket.
	slotSeen := map[uint32]bool{}
	for slot := uint32(0); slot < producers; slot++ {
		entry := ring + RING_ENTRIES_OFFSET + slot*REQ_DESC_SIZE
		tk := bus.Read32(entry + REQ_TICKET_OFF)
		if !seen[tk] {
			t.Errorf("slot %d holds unexpected ticket %d", slot, tk)
		}
		if slotSeen[tk] {
			t.Errorf("ticket %d written to two slots", tk)
		}
		slotSeen[tk] = true
	}
}

// TestMailbox_Slot15DoesNotClobberNeighbour writes the highest response slot of
// every ring and asserts the next ring's header bytes are untouched.
func TestMailbox_Slot15DoesNotClobberNeighbour(t *testing.T) {
	bus := NewMachineBus()
	mgr := NewCoprocessorManager(bus, t.TempDir())
	mgr.versionGateEnabled = false

	for i := 0; i < COPROC_RING_COUNT-1; i++ {
		base := ringBaseAddr(i)
		nextBase := ringBaseAddr(i + 1)

		// Seed the next ring's header with a sentinel.
		bus.Write8(nextBase+RING_HEAD_OFFSET, 0xAA)
		bus.Write8(nextBase+RING_TAIL_OFFSET, 0xBB)
		bus.Write8(nextBase+RING_CAPACITY_OFFSET, 0xCC)

		// Write the final response descriptor of this ring in full.
		lastResp := base + RING_RESPONSES_OFFSET + (RING_CAPACITY-1)*RESP_DESC_SIZE
		for off := uint32(0); off < RESP_DESC_SIZE; off++ {
			bus.Write8(lastResp+off, 0xFF)
		}

		if got := bus.Read8(nextBase + RING_HEAD_OFFSET); got != 0xAA {
			t.Errorf("ring %d slot-15 clobbered ring %d head: %#x", i, i+1, got)
		}
		if got := bus.Read8(nextBase + RING_TAIL_OFFSET); got != 0xBB {
			t.Errorf("ring %d slot-15 clobbered ring %d tail: %#x", i, i+1, got)
		}
		if got := bus.Read8(nextBase + RING_CAPACITY_OFFSET); got != 0xCC {
			t.Errorf("ring %d slot-15 clobbered ring %d capacity: %#x", i, i+1, got)
		}
	}
}

// TestMailbox_DescriptorFieldsLittleEndian asserts request/response descriptor
// fields written by the manager are little-endian on the shared bus, so every
// CPU adapter (all of which read LE) decodes them identically. Byte-order
// invariant for the descriptor contract across all six worker types.
func TestMailbox_DescriptorFieldsLittleEndian(t *testing.T) {
	bus, mgr := newTestBusAndManager(t)
	// Synthetic workers run no goroutine, so no StopAll cleanup is needed.

	// Inject a synthetic worker so enqueue routes, one per instance-0 type.
	for _, ct := range []uint32{EXEC_TYPE_IE32, EXEC_TYPE_6502, EXEC_TYPE_M68K, EXEC_TYPE_Z80, EXEC_TYPE_X86, EXEC_TYPE_IE64} {
		mgr.mu.Lock()
		mgr.workers[ct][0] = newOpenSyntheticWorker(ct)
		mgr.mu.Unlock()

		mgr.HandleWrite(COPROC_CPU_TYPE, ct)
		mgr.HandleWrite(COPROC_INSTANCE, 0)
		mgr.HandleWrite(COPROC_OP, 0x11223344)
		mgr.HandleWrite(COPROC_REQ_PTR, 0xDEADBEEF)
		mgr.HandleWrite(COPROC_CMD, COPROC_CMD_ENQUEUE)
		if st := mgr.HandleRead(COPROC_CMD_STATUS); st != COPROC_STATUS_OK {
			t.Fatalf("type %d enqueue failed: %d", ct, mgr.HandleRead(COPROC_CMD_ERROR))
		}
		ring := ringBaseAddr(coprocRingIndex(ct, 0))
		entry := ring + RING_ENTRIES_OFFSET // head advanced from 0, slot 0 holds it

		// op field: Read32 must equal the LE reconstruction from 4 byte reads.
		opAddr := entry + REQ_OP_OFF
		word := bus.Read32(opAddr)
		le := uint32(bus.Read8(opAddr)) | uint32(bus.Read8(opAddr+1))<<8 |
			uint32(bus.Read8(opAddr+2))<<16 | uint32(bus.Read8(opAddr+3))<<24
		if word != le || word != 0x11223344 {
			t.Errorf("type %d op field not little-endian: word=%#x le=%#x", ct, word, le)
		}
		if got := bus.Read32(entry + REQ_REQ_PTR_OFF); got != 0xDEADBEEF {
			t.Errorf("type %d reqPtr field = %#x, want 0xDEADBEEF", ct, got)
		}
	}
}

// TestMailbox_AllRingsWithinWindow asserts every ring's full content lies inside
// the mailbox window.
func TestMailbox_AllRingsWithinWindow(t *testing.T) {
	const content = RING_RESPONSES_OFFSET + RING_CAPACITY*RESP_DESC_SIZE
	for i := 0; i < COPROC_RING_COUNT; i++ {
		if ringBaseAddr(i)+content > MAILBOX_END+1 {
			t.Errorf("ring %d content exceeds mailbox window", i)
		}
	}
}
