// jit_z80_emit_arm64.go - Z80 JIT compiler: ARM64 native code emitter

//go:build arm64 && linux

package main

import (
	"fmt"
)

// ===========================================================================
// Z80 → ARM64 Register Mapping
// ===========================================================================
//
// Host (ARM64)   Z80 Register     Notes
// ────────────   ────────────     ─────
// W19            A                Callee-saved.
// W20            F                Callee-saved.
// W21            BC (B=hi, C=lo)  Callee-saved, packed 16-bit.
// W22            DE (D=hi, E=lo)  Callee-saved, packed 16-bit.
// W23            HL (H=hi, L=lo)  Callee-saved, packed 16-bit.
// W24            SP (Z80)         Callee-saved. 16-bit stack pointer.
// X25            MemBase          &MachineBus.memory[0].
// X26            Context          &Z80JITContext.
// X27            DirectPageBM     &directPageBitmap[0].
// X28            CpuPtr           &CPU_Z80.
// X0             Entry arg        Context on entry.
// W1-W9          Scratch          Caller-saved.
// X10-X17        Scratch          More scratch.
// X29/X30        FP/LR            Saved/restored.

// compileBlockZ80Stub emits a minimal ARM64 native code stub that sets up the
// return context (RetPC, RetCount, RetCycles) and returns immediately.
func compileBlockZ80Stub(instrs []JITZ80Instr, startPC, endPC uint16, execMem *ExecMem, totalR int) (*JITBlock, error) {
	var buf CodeBuffer

	totalCycles := uint32(0)
	for _, instr := range instrs {
		totalCycles += uint32(instr.cycles)
	}

	// ARM64 stub: X0 = context pointer on entry
	//
	// MOV W1, #endPC
	arm64MovImm32(&buf, 1, uint32(endPC))
	// STR W1, [X0, #jzCtxOffRetPC]
	arm64StrImm(&buf, 1, 0, jzCtxOffRetPC, false)

	// MOV W1, #instrCount
	arm64MovImm32(&buf, 1, uint32(len(instrs)))
	// STR W1, [X0, #jzCtxOffRetCount]
	arm64StrImm(&buf, 1, 0, jzCtxOffRetCount, false)

	// MOV X1, #totalCycles
	arm64MovImm32(&buf, 1, totalCycles)
	// STR X1, [X0, #jzCtxOffRetCycles]
	arm64StrImm(&buf, 1, 0, jzCtxOffRetCycles, true)

	// RET
	buf.Emit32(0xD65F03C0)

	code := buf.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, fmt.Errorf("Z80 JIT ARM64 stub: %w", err)
	}
	flushICache(addr, uintptr(len(code)))

	block := &JITBlock{
		startPC:     uint32(startPC),
		endPC:       uint32(endPC),
		instrCount:  len(instrs),
		execAddr:    addr,
		execSize:    len(code),
		rIncrements: totalR,
	}

	return block, nil
}

// arm64MovImm32 emits MOV Wd, #imm (using MOVZ + optional MOVK for values > 16 bits)
func arm64MovImm32(buf *CodeBuffer, rd int, imm uint32) {
	// MOVZ Wd, #(imm & 0xFFFF)
	buf.Emit32(0x52800000 | uint32(rd) | ((imm & 0xFFFF) << 5))
	if imm > 0xFFFF {
		// MOVK Wd, #(imm >> 16), LSL #16
		buf.Emit32(0x72A00000 | uint32(rd) | (((imm >> 16) & 0xFFFF) << 5))
	}
}

// arm64StrImm emits STR Wt/Xt, [Xn, #imm] (unsigned offset)
func arm64StrImm(buf *CodeBuffer, rt, rn, offset int, is64 bool) {
	if is64 {
		// STR Xt, [Xn, #offset] — 64-bit store, offset must be 8-byte aligned
		scaledOff := uint32(offset / 8)
		buf.Emit32(0xF9000000 | (scaledOff << 10) | (uint32(rn) << 5) | uint32(rt))
	} else {
		// STR Wt, [Xn, #offset] — 32-bit store, offset must be 4-byte aligned
		scaledOff := uint32(offset / 4)
		buf.Emit32(0xB9000000 | (scaledOff << 10) | (uint32(rn) << 5) | uint32(rt))
	}
}
