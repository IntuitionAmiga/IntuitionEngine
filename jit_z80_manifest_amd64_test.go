//go:build amd64 && linux

package main

import (
	"bytes"
	"testing"
	"unsafe"
)

// This is deliberately a native-entry proof. ExecuteJITZ80 accepts detached
// test buses by falling back to CPU_Z80.Execute, which cannot establish amd64
// opcode-lowering coverage.
func TestAMD64Z80JIT_ManifestNativeDifferential(t *testing.T) {
	for _, row := range z80JITOpcodeManifest() {
		row := row
		if row.AMD64Outcome != z80JITOutcomeDirect {
			continue
		}
		t.Run(row.Name, func(t *testing.T) {
			source := append(z80BlockSourceBytes([]JITZ80Instr{row.Instr}), 0x76)
			interpBus, interp := newZ80ManifestCPU(source)
			interp.Step()

			rig := newZ80EmitTestRig(t)
			copy(rig.mem[0x0100:], source)
			rig.mem[0x0200], rig.mem[0x0201] = 0x5A, 0xA5
			rig.mem[0x0300], rig.mem[0x0301] = 0xC3, 0x3C
			rig.cpu.PC, rig.cpu.SP = 0x0100, 0x1FFE
			rig.cpu.SetBC(0x0400)
			rig.cpu.SetDE(0x0300)
			rig.cpu.SetHL(0x0200)
			rig.cpu.IX, rig.cpu.IY = 0x0200, 0x0300
			rig.cpu.A, rig.cpu.F = 0x5A, z80FlagC

			instrs := z80JITScanBlock(rig.mem, 0x0100, len(rig.mem), &rig.cpu.directPageBitmap)
			if len(instrs) != 1 {
				t.Fatalf("scanner admitted %d instructions, want one", len(instrs))
			}
			block, err := compileBlockZ80(instrs, 0x0100, rig.execMem, &rig.cpu.codePageBitmap)
			if err != nil {
				t.Fatalf("native compile failed: %v", err)
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
				t.Fatalf("interpreter/native mismatch: interp={%s} native={%s}", z80ManifestCPUState(interp), z80ManifestCPUState(rig.cpu))
			}
		})
	}
}
