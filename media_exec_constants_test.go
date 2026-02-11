package main

import "testing"

func TestMediaExecMMIORegionsDoNotOverlap(t *testing.T) {
	if MEDIA_LOADER_BASE <= FILE_IO_END && MEDIA_LOADER_END >= FILE_IO_BASE {
		t.Fatalf("media loader MMIO overlaps file I/O MMIO")
	}
	if EXEC_BASE <= MEDIA_LOADER_END && EXEC_END >= MEDIA_LOADER_BASE {
		t.Fatalf("program executor MMIO overlaps media loader MMIO")
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
}
