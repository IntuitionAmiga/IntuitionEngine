//go:build arm64 && linux

package main

import (
	"bytes"
	"fmt"
	"testing"
	"unsafe"
)

type z80ARM64EmitTestRig struct {
	cpu     *CPU_Z80
	execMem *ExecMem
	ctx     *Z80JITContext
	mem     []byte
}

func newZ80ARM64EmitTestRig(t *testing.T) *z80ARM64EmitTestRig {
	t.Helper()
	bus := NewMachineBus()
	adapter := NewZ80BusAdapter(bus)
	cpu := NewCPU_Z80(adapter)
	bus.SealMappings()
	cpu.initDirectPageBitmapZ80(adapter)
	execMem, err := AllocExecMem(64 * 1024)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	t.Cleanup(func() { execMem.Free() })
	return &z80ARM64EmitTestRig{
		cpu: cpu, execMem: execMem, ctx: newZ80JITContext(cpu, adapter), mem: bus.GetMemory(),
	}
}

// This is deliberately a native-entry proof. ExecuteJITZ80 accepts detached
// test buses by falling back to CPU_Z80.Execute, which cannot establish arm64
// opcode-lowering coverage.
func TestARM64Z80JIT_ManifestNativeDifferential(t *testing.T) {
	var compileFailures, stateFailures []string
	variants := []struct {
		name                   string
		a, f, i, r             byte
		bc, de, hl, ix, iy, sp uint16
		iff1, iff2             bool
	}{
		{"baseline", 0x5A, z80FlagC, 0x00, 0x00, 0x0400, 0x0300, 0x0200, 0x0200, 0x0300, 0x1FFE, false, false},
		{"conditions-set", 0x80, z80FlagS | z80FlagZ | z80FlagPV, 0x7F, 0x7E, 0x0101, 0x0301, 0x0201, 0x02FF, 0x03FF, 0x1FFC, true, true},
		{"carry-half-subtract", 0xFF, z80FlagC | z80FlagH | z80FlagN, 0x80, 0xFE, 0x0000, 0x0400, 0x0200, 0x0201, 0x0301, 0x1FFA, false, true},
	}
	for _, row := range z80JITOpcodeManifest() {
		row := row
		if row.ARM64Outcome != z80JITOutcomeDirect {
			continue
		}
		for _, variant := range variants {
			variant := variant
			t.Run(row.Name+"/"+variant.name, func(t *testing.T) {
				source := append(z80BlockSourceBytes([]JITZ80Instr{row.Instr}), 0x76)
				interpBus, interp := newZ80ManifestCPU(source)
				applyZ80ARM64ManifestVariant(interp, variant.a, variant.f, variant.i, variant.r, variant.bc, variant.de, variant.hl, variant.ix, variant.iy, variant.sp, variant.iff1, variant.iff2)
				interp.Step()

				rig := newZ80ARM64EmitTestRig(t)
				copy(rig.mem[0x0100:], source)
				rig.mem[0x0200], rig.mem[0x0201] = 0x5A, 0xA5
				rig.mem[0x0300], rig.mem[0x0301] = 0xC3, 0x3C
				applyZ80ARM64ManifestVariant(rig.cpu, variant.a, variant.f, variant.i, variant.r, variant.bc, variant.de, variant.hl, variant.ix, variant.iy, variant.sp, variant.iff1, variant.iff2)

				instrs := z80JITScanBlock(rig.mem, 0x0100, len(rig.mem), &rig.cpu.directPageBitmap)
				if len(instrs) != 1 {
					t.Fatalf("scanner admitted %d instructions, want one", len(instrs))
				}
				block, err := compileBlockZ80(instrs, 0x0100, rig.execMem, &rig.cpu.codePageBitmap)
				if err != nil {
					compileFailures = append(compileFailures, row.Name+"/"+variant.name)
					return
				}
				callNative(block.execAddr, uintptr(unsafe.Pointer(rig.ctx)))
				rig.cpu.PC = uint16(rig.ctx.RetPC)
				rig.cpu.Cycles += rig.ctx.RetCycles
				rig.ctx.RetCycles = 0
				if rInc := rig.ctx.ChainRIncrements; rInc != 0 {
					r := rig.cpu.R
					rig.cpu.R = (r & 0x80) | ((r + byte(rInc)) & 0x7F)
					rig.ctx.ChainRIncrements = 0
				}
				if !z80ManifestCPUEqual(interp, rig.cpu) || !bytes.Equal(interpBus.mem[:], rig.mem[:len(interpBus.mem)]) {
					stateFailures = append(stateFailures, fmt.Sprintf("%s/%s(%s <> %s)", row.Name, variant.name, z80ManifestCPUState(interp), z80ManifestCPUState(rig.cpu)))
				}
			})
		}
	}
	if len(compileFailures) != 0 || len(stateFailures) != 0 {
		t.Fatalf("ARM64 manifest differential failures: compile=%d %v; state=%d %v",
			len(compileFailures), boundedZ80FailureList(compileFailures, 80),
			len(stateFailures), boundedZ80FailureList(stateFailures, 20))
	}
}

func applyZ80ARM64ManifestVariant(cpu *CPU_Z80, a, f, i, r byte, bc, de, hl, ix, iy, sp uint16, iff1, iff2 bool) {
	cpu.PC, cpu.SP = 0x0100, sp
	cpu.SetBC(bc)
	cpu.SetDE(de)
	cpu.SetHL(hl)
	cpu.IX, cpu.IY = ix, iy
	cpu.A, cpu.F, cpu.I, cpu.R = a, f, i, r
	cpu.IFF1, cpu.IFF2 = iff1, iff2
}

func boundedZ80FailureList(failures []string, limit int) []string {
	if len(failures) <= limit {
		return failures
	}
	return append(append([]string(nil), failures[:limit]...), fmt.Sprintf("... %d more", len(failures)-limit))
}
