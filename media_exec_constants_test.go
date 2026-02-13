package main

import "testing"

func TestMediaExecMMIORegionsDoNotOverlap(t *testing.T) {
	if MEDIA_LOADER_BASE <= FILE_IO_END && MEDIA_LOADER_END >= FILE_IO_BASE {
		t.Fatalf("media loader MMIO overlaps file I/O MMIO")
	}
	if EXEC_BASE <= MEDIA_LOADER_END && EXEC_END >= MEDIA_LOADER_BASE {
		t.Fatalf("program executor MMIO overlaps media loader MMIO")
	}
	// Coprocessor MMIO must not overlap any existing MMIO
	if COPROC_BASE <= EXEC_END && COPROC_END >= EXEC_BASE {
		t.Fatalf("coprocessor MMIO overlaps program executor MMIO")
	}
	if COPROC_BASE <= MEDIA_LOADER_END && COPROC_END >= MEDIA_LOADER_BASE {
		t.Fatalf("coprocessor MMIO overlaps media loader MMIO")
	}
	if COPROC_BASE <= FILE_IO_END && COPROC_END >= FILE_IO_BASE {
		t.Fatalf("coprocessor MMIO overlaps file I/O MMIO")
	}
}

func TestMediaStagingRegionBounds(t *testing.T) {
	if MEDIA_STAGING_SIZE != 0x10000 {
		t.Fatalf("MEDIA_STAGING_SIZE=%#x, want 0x10000", MEDIA_STAGING_SIZE)
	}
	if MEDIA_STAGING_END != MEDIA_STAGING_BASE+MEDIA_STAGING_SIZE-1 {
		t.Fatalf("MEDIA_STAGING_END not consistent with base/size")
	}
	if MEDIA_STAGING_END >= DEFAULT_MEMORY_SIZE {
		t.Fatalf("MEDIA_STAGING exceeds bus memory (end=%#x, memsize=%#x)", MEDIA_STAGING_END, DEFAULT_MEMORY_SIZE)
	}
	if MEDIA_STAGING_BASE <= STACK_START {
		t.Fatalf("MEDIA_STAGING should be above program/stack region")
	}
	// Must not overlap any MMIO-mapped regions
	if MEDIA_STAGING_BASE <= MEDIA_LOADER_END && MEDIA_STAGING_END >= MEDIA_LOADER_BASE {
		t.Fatalf("MEDIA_STAGING overlaps MediaLoader MMIO")
	}
	if MEDIA_STAGING_BASE <= EXEC_END && MEDIA_STAGING_END >= EXEC_BASE {
		t.Fatalf("MEDIA_STAGING overlaps ProgramExecutor MMIO")
	}
	if MEDIA_STAGING_BASE <= COPROC_END && MEDIA_STAGING_END >= COPROC_BASE {
		t.Fatalf("MEDIA_STAGING overlaps Coprocessor MMIO")
	}
}

func TestCoprocMailboxRegionBounds(t *testing.T) {
	if MAILBOX_END >= DEFAULT_MEMORY_SIZE {
		t.Fatalf("MAILBOX exceeds bus memory (end=%#x, memsize=%#x)", MAILBOX_END, DEFAULT_MEMORY_SIZE)
	}
	if MAILBOX_END != MAILBOX_BASE+MAILBOX_SIZE-1 {
		t.Fatalf("MAILBOX_END not consistent with base/size")
	}
	// Mailbox must not overlap MMIO ranges
	if MAILBOX_BASE <= COPROC_END && MAILBOX_END >= COPROC_BASE {
		t.Fatalf("MAILBOX overlaps Coprocessor MMIO")
	}
	if MAILBOX_BASE <= EXEC_END && MAILBOX_END >= EXEC_BASE {
		t.Fatalf("MAILBOX overlaps ProgramExecutor MMIO")
	}
	if MAILBOX_BASE <= MEDIA_LOADER_END && MAILBOX_END >= MEDIA_LOADER_BASE {
		t.Fatalf("MAILBOX overlaps MediaLoader MMIO")
	}
	if MAILBOX_BASE <= FILE_IO_END && MAILBOX_END >= FILE_IO_BASE {
		t.Fatalf("MAILBOX overlaps FileIO MMIO")
	}
	// Mailbox must not overlap media staging
	if MAILBOX_BASE <= MEDIA_STAGING_END && MAILBOX_END >= MEDIA_STAGING_BASE {
		t.Fatalf("MAILBOX overlaps MEDIA_STAGING")
	}
	// Worker regions must not overlap mailbox
	workerRegions := []struct {
		name      string
		base, end uint32
	}{
		{"IE32", WORKER_IE32_BASE, WORKER_IE32_END},
		{"M68K", WORKER_M68K_BASE, WORKER_M68K_END},
		{"6502", WORKER_6502_BASE, WORKER_6502_END},
		{"Z80", WORKER_Z80_BASE, WORKER_Z80_END},
		{"x86", WORKER_X86_BASE, WORKER_X86_END},
	}
	for _, r := range workerRegions {
		if r.base <= MAILBOX_END && r.end >= MAILBOX_BASE {
			t.Fatalf("Worker region %s overlaps MAILBOX", r.name)
		}
	}
	// Worker regions must not overlap each other
	for i := range workerRegions {
		for j := i + 1; j < len(workerRegions); j++ {
			a, b := workerRegions[i], workerRegions[j]
			if a.base <= b.end && a.end >= b.base {
				t.Fatalf("Worker region %s overlaps %s", a.name, b.name)
			}
		}
	}
}
