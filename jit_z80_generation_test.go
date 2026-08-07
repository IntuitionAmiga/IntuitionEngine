//go:build (amd64 || arm64) && linux

package main

import "testing"

func TestZ80JIT_AllPhysicalMutationRoutesPublishAfterMutation(t *testing.T) {
	tests := []struct {
		name  string
		write func(*MachineBus, *CPU_Z80)
	}{
		{"guest-byte", func(bus *MachineBus, _ *CPU_Z80) { bus.Write8(0x1200, 0x76) }},
		{"guest-word", func(bus *MachineBus, _ *CPU_Z80) { bus.Write16(0x1200, 0x7676) }},
		{"guest-long", func(bus *MachineBus, _ *CPU_Z80) { bus.Write32(0x1200, 0x76767676) }},
		{"guest-quad", func(bus *MachineBus, _ *CPU_Z80) { bus.Write64(0x1200, 0x7676767676767676) }},
		{"faulting-api", func(bus *MachineBus, _ *CPU_Z80) { _ = bus.Write32WithFault(0x1200, 0x76767676) }},
		{"direct-vram", func(bus *MachineBus, _ *CPU_Z80) { bus.WriteMemoryDirect(0x1200, 0x76) }},
		{"loader-dma-span", func(bus *MachineBus, _ *CPU_Z80) { bus.WriteSpan(0x1200, []byte{0x76, 0x76}) }},
		{"host-ram-span", func(bus *MachineBus, _ *CPU_Z80) { _ = bus.WritePhysRAMOnly(0x1200, []byte{0x76, 0x76}) }},
		{"physical-byte", func(bus *MachineBus, _ *CPU_Z80) { bus.WritePhys8(0x1200, 0x76) }},
		{"physical-word", func(bus *MachineBus, _ *CPU_Z80) { bus.WritePhys16(0x1200, 0x7676) }},
		{"physical-long", func(bus *MachineBus, _ *CPU_Z80) { bus.WritePhys32(0x1200, 0x76767676) }},
		{"physical-quad", func(bus *MachineBus, _ *CPU_Z80) { bus.WritePhys64(0x1200, 0x7676767676767676) }},
		{"debugger", func(bus *MachineBus, cpu *CPU_Z80) {
			NewDebugZ80(cpu, nil).WriteMemory(0x1200, []byte{0x76})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			cpu := NewCPU_Z80(NewZ80BusAdapter(bus))
			cpu.jitCache = NewCodeCache()
			cpu.jitCache.Put(&JITBlock{startPC: 0x1200, endPC: 0x1210})
			unregister := bus.RegisterZ80JITInvalidator(cpu.noteZ80JITWrite)
			defer unregister()
			tc.write(bus, cpu)
			if got := bus.ReadPhys8(0x1200); got != 0x76 {
				t.Fatalf("mutation not visible before publication check: got %02X", got)
			}
			if got := cpu.jitCodeGeneration[0x12].Load(); got == 0 {
				t.Fatal("mutation route did not publish code-page generation")
			}
			ctx := &Z80JITContext{RTSCache0PC: 0x1200, RTSCache0Addr: 1}
			cpu.drainZ80JITInvalidations(ctx)
			if cpu.jitCache.Get(0x1200) != nil {
				t.Fatal("mutation route retained live compiled code")
			}
			if ctx.RTSCache0Addr != 0 {
				t.Fatal("mutation route retained a stale return target")
			}
		})
	}
}

func TestZ80JIT_PageSpanningMutationPublishesEveryPage(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_Z80(NewZ80BusAdapter(bus))
	unregister := bus.RegisterZ80JITInvalidator(cpu.noteZ80JITWrite)
	defer unregister()
	bus.WriteSpan(0x12FF, []byte{1, 2, 3})
	if cpu.jitCodeGeneration[0x12].Load() == 0 || cpu.jitCodeGeneration[0x13].Load() == 0 {
		t.Fatalf("spanning generations: page12=%d page13=%d", cpu.jitCodeGeneration[0x12].Load(), cpu.jitCodeGeneration[0x13].Load())
	}
}

func TestZ80JIT_ConcurrentCPUOwnersDrainIndependently(t *testing.T) {
	bus := NewMachineBus()
	first := NewCPU_Z80(NewZ80BusAdapter(bus))
	second := NewCPU_Z80(NewZ80BusAdapter(bus))
	for _, cpu := range []*CPU_Z80{first, second} {
		cpu.jitCache = NewCodeCache()
		cpu.jitCache.Put(&JITBlock{startPC: 0x1200, endPC: 0x1210})
	}
	unregisterFirst := bus.RegisterZ80JITInvalidator(first.noteZ80JITWrite)
	unregisterSecond := bus.RegisterZ80JITInvalidator(second.noteZ80JITWrite)
	defer unregisterFirst()
	defer unregisterSecond()
	bus.Write8(0x1204, 0x76)
	first.drainZ80JITInvalidations(&Z80JITContext{})
	if first.jitCache.Get(0x1200) != nil {
		t.Fatal("first owner retained stale block")
	}
	if second.jitCache.Get(0x1200) == nil {
		t.Fatal("first owner mutated second owner's cache")
	}
	second.drainZ80JITInvalidations(&Z80JITContext{})
	if second.jitCache.Get(0x1200) != nil {
		t.Fatal("second owner retained stale block after its own drain")
	}
}

func TestZ80JIT_BankedWritePublishesResolvedPhysicalPageOnly(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	cpu.jitCache = NewCodeCache()
	cpu.jitCache.Put(&JITBlock{startPC: 0x0000, endPC: 0x0010})
	cpu.jitCache.Put(&JITBlock{startPC: 0x0100, endPC: 0x0110})
	unregister := bus.RegisterZ80JITInvalidator(cpu.noteZ80JITWrite)
	defer unregister()
	adapter.bank1Enable = true
	adapter.bank1 = 0
	adapter.Write(0x2004, 0x76)
	cpu.drainZ80JITInvalidations(&Z80JITContext{})
	if cpu.jitCache.Get(0x0000) != nil {
		t.Fatal("banked write retained block on resolved physical page")
	}
	if cpu.jitCache.Get(0x0100) == nil {
		t.Fatal("banked write invalidated an unrelated physical page")
	}
}

func TestZ80JIT_PhysicalWritePublisherAdvancesOnlyAffectedPages(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_Z80(NewZ80BusAdapter(bus))
	unregister := bus.RegisterZ80JITInvalidator(cpu.noteZ80JITWrite)
	t.Cleanup(unregister)

	bus.Write8(0x01FF, 0x12)
	bus.Write8(0x0200, 0x34)

	if got := cpu.jitCodeGeneration[0x01].Load(); got != 1 {
		t.Fatalf("page 01 generation = %d, want 1", got)
	}
	if got := cpu.jitCodeGeneration[0x02].Load(); got != 1 {
		t.Fatalf("page 02 generation = %d, want 1", got)
	}
	if got := cpu.jitCodeGeneration[0x03].Load(); got != 0 {
		t.Fatalf("unwritten page 03 generation = %d, want 0", got)
	}
}

func TestZ80JIT_OwningDispatcherDrainsPhysicalGeneration(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewCPU_Z80(NewZ80BusAdapter(bus))
	cpu.jitCache = NewCodeCache()
	cpu.jitCache.Put(&JITBlock{startPC: 0x0100, endPC: 0x0110})
	ctx := &Z80JITContext{RTSCache0PC: 0x0100, RTSCache0Addr: 1}

	cpu.noteZ80JITWrite(0x0108, 1)
	cpu.drainZ80JITInvalidations(ctx)

	if block := cpu.jitCache.Get(0x0100); block != nil {
		t.Fatalf("compiled block survived physical write: %+v", block)
	}
	if ctx.RTSCache0Addr != 0 {
		t.Fatalf("return-target cache = %#x, want cleared", ctx.RTSCache0Addr)
	}
	if got := cpu.jitStats.invalidations.Load(); got != 1 {
		t.Fatalf("invalidation count = %d, want 1", got)
	}
}

func TestZ80JIT_BankRemapPublishesMappingGeneration(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	for _, addr := range []uint16{
		Z80_BANK1_REG_LO, Z80_BANK1_REG_HI,
		Z80_BANK2_REG_LO, Z80_BANK2_REG_HI,
		Z80_BANK3_REG_LO, Z80_BANK3_REG_HI,
		Z80_VRAM_BANK_REG,
	} {
		before := adapter.mappingGeneration.Load()
		adapter.Write(addr, 1)
		if got := adapter.mappingGeneration.Load(); got != before+1 {
			t.Fatalf("register %04X generation = %d, want %d", addr, got, before+1)
		}
	}
	before := adapter.mappingGeneration.Load()
	adapter.ResetBank()
	if got := adapter.mappingGeneration.Load(); got != before+1 {
		t.Fatalf("ResetBank generation = %d, want %d", got, before+1)
	}
}

func TestZ80JIT_BankAndVRAMRemapsDiscardPromotedChains(t *testing.T) {
	if !z80JitAvailable {
		t.Skip("Z80 native JIT unavailable")
	}
	for _, tc := range []struct {
		name string
		addr uint16
	}{
		{"bank-window", Z80_BANK1_REG_LO},
		{"vram-window", Z80_VRAM_BANK_REG},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := NewMachineBus()
			adapter := NewZ80BusAdapter(bus)
			cpu := NewCPU_Z80(adapter)
			bus.SealMappings()
			cpu.initDirectPageBitmapZ80(adapter)
			if err := cpu.initZ80JIT(adapter); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cpu.freeZ80JIT)
			mem := bus.GetMemory()
			mem[0x0100], mem[0x0101], mem[0x0102] = 0xC3, 0x00, 0x02
			mem[0x0200], mem[0x0201], mem[0x0202] = 0xC3, 0x00, 0x03
			mem[0x0300] = 0xC9
			if got := cpu.z80PromoteStaticRegion(adapter, mem, 0x0100); got != 3 {
				t.Fatalf("promoted blocks = %d, want 3", got)
			}
			ctx := cpu.jitCtx.(*Z80JITContext)
			ctx.RTSCache0PC, ctx.RTSCache0Addr = 0x0200, 1
			adapter.Write(tc.addr, 1)
			cpu.drainZ80JITMappingChange(adapter, ctx)
			if cpu.jitCache.Len() != 0 {
				t.Fatalf("mapping change retained %d promoted blocks", cpu.jitCache.Len())
			}
			if ctx.RTSCache0Addr != 0 {
				t.Fatal("mapping change retained stale return target")
			}
		})
	}
}

func TestZ80JIT_BlockSourceStampRejectsChangedCode(t *testing.T) {
	mem := make([]byte, 0x10000)
	mem[0x0100] = 0x00 // NOP
	block := &JITBlock{startPC: 0x0100, z80Source: []byte{0x00}}

	if !z80BlockSourceMatches(mem, 0x0100, block) {
		t.Fatal("unchanged source stamp did not match")
	}
	mem[0x0100] = 0x76 // HALT
	if z80BlockSourceMatches(mem, 0x0100, block) {
		t.Fatal("changed source bytes matched a stale block")
	}
}

func TestZ80JIT_BlockGenerationSnapshotRejectsChangedCodeOrMapping(t *testing.T) {
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	block := &JITBlock{startPC: 0x1200, z80Source: []byte{0x00, 0x00}}
	z80CaptureBlockGenerations(cpu, adapter, block)
	if !z80BlockGenerationsMatch(cpu, adapter, block) {
		t.Fatal("fresh generation snapshot did not match")
	}
	cpu.jitCodeGeneration[0x12].Add(1)
	if z80BlockGenerationsMatch(cpu, adapter, block) {
		t.Fatal("physical generation change retained block")
	}
	z80CaptureBlockGenerations(cpu, adapter, block)
	adapter.mappingGeneration.Add(1)
	if z80BlockGenerationsMatch(cpu, adapter, block) {
		t.Fatal("mapping generation change retained block")
	}
}

func TestZ80JIT_CanonicalHelperUsesFrozenInstructionBytes(t *testing.T) {
	rig := newCPUZ80TestRig()
	rig.cpu.PC = 0x0100
	rig.cpu.A = 0x5A
	rig.bus.mem[0x0100] = 0xD3 // OUT (n),A
	rig.bus.mem[0x0101] = 0x10
	payload := z80CanonicalHelperPayload{
		StartPC: 0x0100,
		Bytes:   [4]byte{0xD3, 0x10},
		Length:  2,
	}
	// A concurrent writer may replace the source after the helper payload has
	// been published. The helper must still use its immutable operand.
	rig.bus.mem[0x0101] = 0x20

	rig.cpu.executeZ80CanonicalHelper(payload)

	if got := rig.bus.io[0x5A10]; got != 0x5A {
		t.Fatalf("OUT used port %04X value %02X, want frozen port 5A10 value 5A", 0x5A10, got)
	}
	if got := rig.bus.io[0x5A20]; got != 0 {
		t.Fatalf("helper re-read mutated operand: port 5A20 = %02X", got)
	}
}
