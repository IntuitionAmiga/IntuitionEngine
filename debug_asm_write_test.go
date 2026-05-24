package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWritePhysRAMOnlyRejectsLegacyMMIOWithoutSideEffects(t *testing.T) {
	bus := NewMachineBus()
	const addr uint64 = 0xF1000
	bus.memory[int(addr)] = 0xAA
	writes := 0
	bus.MapIO(uint32(addr), uint32(addr)+7, nil, func(addr uint32, value uint32) {
		writes++
	})

	if !bus.PhysMapped(addr, 8) {
		t.Fatal("precondition: low MMIO span should still be physically mapped")
	}
	err := bus.WritePhysRAMOnly(addr, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	if err == nil || !strings.Contains(err.Error(), "MMIO") {
		t.Fatalf("WritePhysRAMOnly err = %v, want MMIO rejection", err)
	}
	if writes != 0 {
		t.Fatalf("MMIO write handler fired %d time(s)", writes)
	}
	if bus.memory[int(addr)] != 0xAA {
		t.Fatalf("memory changed to 0x%02X after rejected MMIO write", bus.memory[int(addr)])
	}
}

func TestWritePhysRAMOnlyRejectsMapIO64WithoutSideEffects(t *testing.T) {
	bus := NewMachineBus()
	const addr uint64 = 0xF2000
	writes := 0
	bus.MapIO64(uint32(addr), uint32(addr)+7, nil, func(addr uint32, value uint64) {
		writes++
	})

	err := bus.WritePhysRAMOnly(addr, []byte{0xE0, 0, 0, 0, 0, 0, 0, 0})
	if err == nil || !strings.Contains(err.Error(), "MMIO") {
		t.Fatalf("WritePhysRAMOnly err = %v, want MapIO64 rejection", err)
	}
	if writes != 0 {
		t.Fatalf("MapIO64 write handler fired %d time(s)", writes)
	}
}

func TestWritePhysRAMOnlyRejectsOutOfRangeAndOverflowAtomically(t *testing.T) {
	bus := NewMachineBus()
	top := uint64(len(bus.memory)) - 8
	before := bytes.Repeat([]byte{0xAA}, 8)
	copy(bus.memory[int(top):], before)

	err := bus.WritePhysRAMOnly(uint64(len(bus.memory))-4, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	if err == nil || !strings.Contains(err.Error(), "not mapped writable RAM") {
		t.Fatalf("straddling err = %v", err)
	}
	if got := bus.memory[int(top):]; !bytes.Equal(got, before) {
		t.Fatalf("top memory changed after failed straddling write: % X", got)
	}

	err = bus.WritePhysRAMOnly(^uint64(0)-3, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	if err == nil || !strings.Contains(err.Error(), "overflows") {
		t.Fatalf("overflow err = %v", err)
	}
}

func TestWritePhysRAMOnlyWritesHighBacking(t *testing.T) {
	bus := NewMachineBus()
	backing := NewSparseBacking(8 * bGiB)
	bus.SetBacking(backing)

	addr := uint64(5)*bGiB + 0x1000
	want := []byte{0xE0, 0, 0, 0, 0, 0, 0, 0}
	if err := bus.WritePhysRAMOnly(addr, want); err != nil {
		t.Fatalf("WritePhysRAMOnly high backing: %v", err)
	}
	got := make([]byte, len(want))
	for i := range got {
		got[i] = backing.Read8(addr + uint64(i))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("backing bytes = % X, want % X", got, want)
	}
}

func TestWriteAssembledCodeRAMFlushesIE64JIT(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.running.Store(false)
	cpu.jitCache = NewCodeCache()
	cpu.jitCache.Put(&JITBlock{startPC: 0x1000, endPC: 0x1008})
	cpu.jitCtx = newJITContext(cpu)
	cpu.jitCtx.NeedInval = 1
	cpu.jitCtx.NeedIOFallback = 1
	cpu.jitCtx.RTSCache0PC = 0x1000
	cpu.jitCtx.RTSCache0Addr = 0x12345678

	execMem, err := AllocExecMem(4096)
	if err == nil {
		cpu.jitExecMem = execMem
		if _, err := execMem.Write([]byte{0xC3}); err != nil {
			t.Fatalf("execMem.Write: %v", err)
		}
		t.Cleanup(execMem.Free)
	}

	beforeInvalidations := globalIE64TurboStats.invalidations.Load()
	want := []byte{0xE0, 0, 0, 0, 0, 0, 0, 0}
	if err := cpu.WriteAssembledCodeRAM(0x1000, want); err != nil {
		t.Fatalf("WriteAssembledCodeRAM: %v", err)
	}
	if got := cpu.memory[0x1000 : 0x1000+8]; !bytes.Equal(got, want) {
		t.Fatalf("memory bytes = % X, want % X", got, want)
	}
	if cpu.jitCache.Get(0x1000) != nil {
		t.Fatal("JIT cache still contains patched block")
	}
	if cpu.jitCtx.NeedInval != 0 || cpu.jitCtx.NeedIOFallback != 0 ||
		cpu.jitCtx.RTSCache0PC != 0 || cpu.jitCtx.RTSCache0Addr != 0 {
		t.Fatalf("jit ctx not cleared: %+v", cpu.jitCtx)
	}
	if execMem != nil && execMem.Used() != 0 {
		t.Fatalf("execMem Used = %d, want 0", execMem.Used())
	}
	if got := globalIE64TurboStats.invalidations.Load(); got != beforeInvalidations+1 {
		t.Fatalf("invalidations = %d, want %d", got, beforeInvalidations+1)
	}
}

func TestWriteAssembledCodeRAMFailedWriteDoesNotFlushJIT(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU64(bus)
	cpu.running.Store(false)
	cpu.jitCache = NewCodeCache()
	cpu.jitCache.Put(&JITBlock{startPC: 0x1000, endPC: 0x1008})
	cpu.jitCtx = newJITContext(cpu)
	cpu.jitCtx.RTSCache0PC = 0x1000
	beforeInvalidations := globalIE64TurboStats.invalidations.Load()

	err := cpu.WriteAssembledCodeRAM(^uint64(0)-3, []byte{0xE0, 0, 0, 0, 0, 0, 0, 0})
	if err == nil {
		t.Fatal("WriteAssembledCodeRAM succeeded for overflowing range")
	}
	if cpu.jitCache.Get(0x1000) == nil {
		t.Fatal("JIT cache was flushed after failed write")
	}
	if cpu.jitCtx.RTSCache0PC != 0x1000 {
		t.Fatalf("RTS cache changed after failed write: %#x", cpu.jitCtx.RTSCache0PC)
	}
	if got := globalIE64TurboStats.invalidations.Load(); got != beforeInvalidations {
		t.Fatalf("invalidations changed after failed write: got %d want %d", got, beforeInvalidations)
	}
}
