// jit_m68k_region_residency_test.go - Milestone 7 region GPR residency.
// Map-builder ranking tests, a shape test on the residency counter, and a
// native-execution parity check for a region hot in non-fixed registers.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

func TestM68KJIT_RegionRegMapRanking(t *testing.T) {
	scan := func(words ...uint16) []M68KJITInstr {
		mem := make([]byte, 0x1000)
		for i, w := range words {
			mem[0x100+i*2] = byte(w >> 8)
			mem[0x100+i*2+1] = byte(w)
		}
		return m68kScanBlock(mem, 0x100)
	}

	// D2/D3-heavy stream: custom map binds D2 and D3.
	instrs := scan(0x7405, 0xD682, 0x5283, 0xB682, 0x6002) // MOVEQ #5,D2; ADD.L D2,D3; ADDQ.L #1,D3; CMP.L D2,D3; BRA
	m := m68kBuildRegionRegMap(instrs)
	if m == nil {
		t.Fatal("no custom map for D2/D3-heavy region")
	}
	if m.dataHost[2] == 0 || m.dataHost[3] == 0 {
		t.Fatalf("hot D2/D3 not bound: %+v", m)
	}

	// Fixed-register stream: ranking reproduces the fixed set, no map.
	instrs = scan(0x7005, 0x7201, 0xD081, 0x6002) // MOVEQ D0; MOVEQ D1; ADD.L D1,D0; BRA
	if m := m68kBuildRegionRegMap(instrs); m != nil {
		t.Fatalf("fixed-set region got a custom map: %+v", m)
	}
}

// A two-block region hot in D2/D3 compiles with the custom map and matches
// the interpreter after native execution.
func TestM68KJIT_RegionResidencyParity(t *testing.T) {
	if !m68kJitAvailable {
		t.Skip("M68K JIT not available")
	}
	build := func(cpu *M68KCPU) {
		// Block A @0x100: MOVEQ #5,D2 ; ADD.L D2,D3 ; BRA.W +0x200
		// Block B @0x304: ADDQ.L #1,D3 ; EOR.L D2,D3 ; RTS
		w := func(pc uint32, words ...uint16) {
			for i, x := range words {
				cpu.Write16(pc+uint32(i*2), x)
			}
		}
		w(0x100, 0x7405, 0xD682, 0x6000, 0x0200)
		w(0x304, 0x5283, 0xB583, 0x4E75)
	}

	rig := newM68KDiffJITTestRig(t)
	jit := rig.cpu
	m68kDiffSetupCPU(jit)
	build(jit)
	jit.AddrRegs[7] = 0x8000
	jit.Write32(0x8000, 0x600) // return address for RTS

	region := m68kFormRegion(0x100, jit.memory)
	if region == nil || len(region.blocks) < 2 {
		t.Fatalf("region not formed: %+v", region)
	}
	before := m68kRegionResidencyEmits.Load()
	rig.execMem.Reset()
	block, err := m68kCompileRegion(region, rig.execMem, jit.memory)
	if err != nil {
		t.Fatalf("m68kCompileRegion: %v", err)
	}
	if m68kRegionResidencyEmits.Load() == before {
		t.Fatal("region compiled without the custom residency map")
	}

	rig.ctx.DataRegsPtr = uintptr(unsafe.Pointer(&jit.DataRegs[0]))
	rig.ctx.AddrRegsPtr = uintptr(unsafe.Pointer(&jit.AddrRegs[0]))
	rig.ctx.MemPtr = uintptr(unsafe.Pointer(&jit.memory[0]))
	rig.ctx.SRPtr = uintptr(unsafe.Pointer(&jit.SR))
	rig.ctx.ChainBudget = 1000
	rig.ctx.RetPC = 0
	rig.ctx.NeedIOFallback = 0
	callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
	jit.PC = rig.ctx.RetPC

	// Interpreter reference.
	interp := newM68KDiffTestProgramCPU(t, 0x100)
	m68kDiffSetupCPU(interp)
	build(interp)
	interp.AddrRegs[7] = 0x8000
	interp.Write32(0x8000, 0x600)
	interp.PC = 0x100
	for i := 0; i < 10 && interp.PC != 0x600; i++ {
		if cycles := interp.StepOne(); cycles == 0 {
			t.Fatalf("interpreter stopped at %d", i)
		}
	}
	if jit.PC != interp.PC {
		t.Fatalf("PC: got %08X want %08X", jit.PC, interp.PC)
	}
	assertM68KCoreStateEqual(t, jit, interp)
}
