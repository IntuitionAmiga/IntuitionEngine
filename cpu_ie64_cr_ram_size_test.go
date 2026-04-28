// cpu_ie64_cr_ram_size_test.go - PLAN_MAX_RAM slice 10e2 TDD coverage.
//
// Pins CR_RAM_SIZE_BYTES live-read of bus.ActiveVisibleRAM() (no snapshot
// at boot — every read sees the current value), and MTCR raising
// FAULT_ILLEGAL_INSTRUCTION since the CR is read-only.

package main

import "testing"

func TestIE64_CR_COUNT_Bumped(t *testing.T) {
	if CR_COUNT != 16 {
		t.Fatalf("CR_COUNT = %d, want 16 (CR_RAM_SIZE_BYTES added at index 15)", CR_COUNT)
	}
}

func TestIE64_CR_RAM_SIZE_BYTES_Index(t *testing.T) {
	// Pin the ABI-visible index for SDK header consumers.
	if CR_RAM_SIZE_BYTES != 15 {
		t.Fatalf("CR_RAM_SIZE_BYTES = %d, want 15", CR_RAM_SIZE_BYTES)
	}
}

// TestIE64_CR_RAM_SIZE_BYTES_LiveMatchesActiveVisible runs MFCR
// CR_RAM_SIZE_BYTES into R1 and verifies the result tracks the
// currently-published ActiveVisibleRAM (live read, no snapshot).
func TestIE64_CR_RAM_SIZE_BYTES_LiveMatchesActiveVisible(t *testing.T) {
	r := newIE64TestRig()

	r.bus.ApplyProfileVisibleCeiling(uint64(DEFAULT_MEMORY_SIZE))
	r.executeN(
		ie64Instr(OP_MFCR, 1, 0, 0, CR_RAM_SIZE_BYTES, 0, 0),
	)
	if got := r.cpu.regs[1]; got != uint64(DEFAULT_MEMORY_SIZE) {
		t.Fatalf("MFCR CR_RAM_SIZE_BYTES (32 MiB ceiling) = %d, want %d", got, DEFAULT_MEMORY_SIZE)
	}

	// Re-publish a different ceiling and confirm the CR re-reads live.
	r.cpu.regs[1] = 0
	r.bus.ApplyProfileVisibleCeiling(16 * 1024 * 1024)
	r.cpu.PC = PROG_START
	r.cpu.running.Store(true)
	r.executeN(
		ie64Instr(OP_MFCR, 1, 0, 0, CR_RAM_SIZE_BYTES, 0, 0),
	)
	if got := r.cpu.regs[1]; got != 16*1024*1024 {
		t.Fatalf("MFCR CR_RAM_SIZE_BYTES (16 MiB ceiling) = %d, want %d (live read)", got, 16*1024*1024)
	}
}

// TestIE64_CR_RAM_SIZE_BYTES_MTCR_Faults pins that MTCR to a read-only CR
// raises FAULT_ILLEGAL_INSTRUCTION.
func TestIE64_CR_RAM_SIZE_BYTES_MTCR_Faults(t *testing.T) {
	r := newIE64TestRig()
	r.bus.ApplyProfileVisibleCeiling(uint64(DEFAULT_MEMORY_SIZE))
	preActive := r.bus.ActiveVisibleRAM()

	r.cpu.regs[1] = 0xDEADBEEFCAFEBABE
	r.executeN(
		ie64Instr(OP_MTCR, CR_RAM_SIZE_BYTES, 0, 0, 1, 0, 0),
	)

	if r.cpu.faultCause != FAULT_ILLEGAL_INSTRUCTION {
		t.Fatalf("faultCause = %d, want FAULT_ILLEGAL_INSTRUCTION (=%d)", r.cpu.faultCause, FAULT_ILLEGAL_INSTRUCTION)
	}
	if got := r.bus.ActiveVisibleRAM(); got != preActive {
		t.Fatalf("ActiveVisibleRAM mutated by MTCR: got %d, want %d", got, preActive)
	}
}

// TestIE64_CR_RAM_SIZE_BYTES_FullyBackedTotalAbove4GiB verifies the live-
// read returns the full backed total even when total > 4 GiB-page (the
// only case where IE64 might see ActiveVisibleRAM > bus.memory window).
func TestIE64_CR_RAM_SIZE_BYTES_FullyBackedTotalAbove4GiB(t *testing.T) {
	r := newIE64TestRig()

	const eightGiB uint64 = 8 * 1024 * 1024 * 1024
	r.bus.SetBacking(NewSparseBacking(eightGiB))
	r.bus.SetSizing(MemorySizing{TotalGuestRAM: eightGiB})
	r.bus.ApplyProfileVisibleCeiling(eightGiB)

	r.executeN(
		ie64Instr(OP_MFCR, 1, 0, 0, CR_RAM_SIZE_BYTES, 0, 0),
	)
	if got := r.cpu.regs[1]; got != eightGiB {
		t.Fatalf("MFCR CR_RAM_SIZE_BYTES (8 GiB total) = %d, want %d", got, eightGiB)
	}
}
