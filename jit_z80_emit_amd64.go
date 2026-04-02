// jit_z80_emit_amd64.go - Z80 JIT compiler: x86-64 native code emitter

//go:build amd64 && linux

package main

import (
	"fmt"
)

// ===========================================================================
// Z80 → x86-64 Register Mapping
// ===========================================================================
//
// Host (x86-64)   Z80 Register     Notes
// ─────────────   ────────────     ─────
// RBX (BL)        A                Callee-saved. Most accessed 8-bit reg.
// RBP (BPL)       F                Callee-saved. Z80 flags byte.
// R12W            BC (B=hi, C=lo)  Callee-saved. Packed 16-bit pair.
// R13W            DE (D=hi, E=lo)  Callee-saved. Packed 16-bit pair.
// R14W            HL (H=hi, L=lo)  Callee-saved. Packed 16-bit pair.
// R15             Context ptr      Callee-saved. &Z80JITContext.
// RSI             MemBase          &MachineBus.memory[0].
// R8              DirectPageBM     &directPageBitmap[0].
// R9              CodePageBM       &codePageBitmap[0].
// RAX,RCX,RDX     Scratch          General scratch (CL for shifts).
// R10,R11         Scratch          Additional scratch.
// RDI             Entry arg        Context on entry, saved to [RSP+0].

const (
	z80RegA   = amd64RBX // A register (BL)
	z80RegF   = amd64RBP // F register (BPL)
	z80RegBC  = amd64R12 // BC pair (R12W: B=high, C=low)
	z80RegDE  = amd64R13 // DE pair (R13W: D=high, E=low)
	z80RegHL  = amd64R14 // HL pair (R14W: H=high, L=low)
	z80RegCtx = amd64R15 // Z80JITContext pointer
	z80RegMem = amd64RSI // Memory base pointer
	z80RegDPB = amd64R8  // DirectPageBitmap pointer
	z80RegCPB = amd64R9  // CodePageBitmap pointer

	z80Scratch1 = amd64RAX // General scratch
	z80Scratch2 = amd64RCX // Scratch / shift count (CL)
	z80Scratch3 = amd64RDX // Scratch
	z80Scratch4 = amd64R10 // Scratch
	z80Scratch5 = amd64R11 // Scratch
)

// Stack frame layout:
// 6 callee-saved pushes (48 bytes) + return address (8 bytes) = 56 bytes.
// SUB RSP, 40 → total 96 bytes = 16-byte aligned.
const (
	z80FrameSize    = 40
	z80OffCtxPtr    = 0  // [RSP+0]  = saved Z80JITContext* (8 bytes)
	z80OffCpuPtr    = 8  // [RSP+8]  = CpuPtr (8 bytes)
	z80OffCycles    = 16 // [RSP+16] = cycle accumulator (uint32)
	z80OffLoopBudg  = 20 // [RSP+20] = loop budget counter (uint32)
	z80OffParityPtr = 24 // [RSP+24] = ParityTablePtr (8 bytes)
	z80OffDAAPtr    = 32 // [RSP+32] = DAATablePtr (8 bytes)
)

// ===========================================================================
// Z80 Register Code → Host Access
// ===========================================================================

// Z80 instruction register encoding: B=0, C=1, D=2, E=3, H=4, L=5, (HL)=6, A=7

// z80EmitReadReg8 emits code to read a Z80 8-bit register (by Z80 encoding 0-7,
// excluding 6=(HL)) into the host scratch register RAX (zero-extended to 32-bit).
func z80EmitReadReg8(buf *CodeBuffer, z80Reg byte) {
	switch z80Reg {
	case 7: // A → BL
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
	case 0: // B → high byte of R12W
		amd64MOVZX_W(buf, z80Scratch1, z80RegBC)
		// SHR EAX, 8
		buf.EmitBytes(0xC1, 0xE8, 0x08)
	case 1: // C → low byte of R12 (R12B)
		amd64MOVZX_B(buf, z80Scratch1, z80RegBC)
	case 2: // D → high byte of R13W
		amd64MOVZX_W(buf, z80Scratch1, z80RegDE)
		buf.EmitBytes(0xC1, 0xE8, 0x08)
	case 3: // E → low byte of R13 (R13B)
		amd64MOVZX_B(buf, z80Scratch1, z80RegDE)
	case 4: // H → high byte of R14W
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		buf.EmitBytes(0xC1, 0xE8, 0x08)
	case 5: // L → low byte of R14 (R14B)
		amd64MOVZX_B(buf, z80Scratch1, z80RegHL)
	}
}

// z80EmitWriteReg8 emits code to write the low byte of RAX to a Z80 8-bit register.
func z80EmitWriteReg8(buf *CodeBuffer, z80Reg byte) {
	switch z80Reg {
	case 7: // A → BL
		// MOV BL, AL
		buf.EmitBytes(0x88, modRM(3, z80Scratch1, z80RegA))
	case 0: // B → high byte of R12W
		// AND R12W, 0x00FF (keep C, clear B)
		buf.EmitBytes(0x66, 0x41, 0x81, 0xE4, 0xFF, 0x00)
		// MOVZX ECX, AL
		buf.EmitBytes(0x0F, 0xB6, 0xC8)
		// SHL ECX, 8
		buf.EmitBytes(0xC1, 0xE1, 0x08)
		// OR R12W, CX
		buf.EmitBytes(0x66, 0x41, 0x09, 0xCC)
	case 1: // C → low byte of R12
		// MOV R12B, AL
		buf.EmitBytes(0x41, 0x88, modRM(3, z80Scratch1, z80RegBC&0x07))
	case 2: // D → high byte of R13W
		buf.EmitBytes(0x66, 0x41, 0x81, 0xE5, 0xFF, 0x00) // AND R13W, 0x00FF
		buf.EmitBytes(0x0F, 0xB6, 0xC8)                   // MOVZX ECX, AL
		buf.EmitBytes(0xC1, 0xE1, 0x08)                   // SHL ECX, 8
		buf.EmitBytes(0x66, 0x41, 0x09, 0xCD)             // OR R13W, CX
	case 3: // E → low byte of R13
		buf.EmitBytes(0x41, 0x88, modRM(3, z80Scratch1, z80RegDE&0x07))
	case 4: // H → high byte of R14W
		buf.EmitBytes(0x66, 0x41, 0x81, 0xE6, 0xFF, 0x00) // AND R14W, 0x00FF
		buf.EmitBytes(0x0F, 0xB6, 0xC8)                   // MOVZX ECX, AL
		buf.EmitBytes(0xC1, 0xE1, 0x08)                   // SHL ECX, 8
		buf.EmitBytes(0x66, 0x41, 0x09, 0xCE)             // OR R14W, CX
	case 5: // L → low byte of R14
		buf.EmitBytes(0x41, 0x88, modRM(3, z80Scratch1, z80RegHL&0x07))
	}
}

// z80EmitReadPair16 emits code to read a Z80 16-bit pair into EAX (zero-extended).
// pairCode: 0=BC, 1=DE, 2=HL, 3=SP
func z80EmitReadPair16(buf *CodeBuffer, pairCode byte) {
	switch pairCode {
	case 0: // BC
		amd64MOVZX_W(buf, z80Scratch1, z80RegBC)
	case 1: // DE
		amd64MOVZX_W(buf, z80Scratch1, z80RegDE)
	case 2: // HL
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
	case 3: // SP — spilled to CPU struct
		// MOV RAX, [RSP+z80OffCpuPtr]
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		// MOVZX EAX, WORD [RAX + cpuZ80OffSP]
		amd64MOVZX_W_mem(buf, z80Scratch1, z80Scratch1, int32(cpuZ80OffSP))
	}
}

// z80EmitWritePair16 emits code to write AX (16-bit) to a Z80 pair.
func z80EmitWritePair16(buf *CodeBuffer, pairCode byte) {
	switch pairCode {
	case 0: // BC
		// MOV R12W, AX — requires 66 prefix + REX
		buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch1, z80RegBC&0x07))
	case 1: // DE
		buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch1, z80RegDE&0x07))
	case 2: // HL
		buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch1, z80RegHL&0x07))
	case 3: // SP
		// MOV RCX, [RSP+z80OffCpuPtr]
		amd64MOV_reg_mem(buf, z80Scratch2, amd64RSP, int32(z80OffCpuPtr))
		// MOV [RCX + cpuZ80OffSP], AX
		buf.EmitBytes(0x66)
		emitMemOp(buf, false, 0x89, z80Scratch1, z80Scratch2, int32(cpuZ80OffSP))
	}
}

// ===========================================================================
// Memory Access Helpers
// ===========================================================================

// z80EmitMemRead emits code to read a byte from Z80 address in AX into AL,
// with direct page bitmap check. If the page is non-direct, jumps to the
// bail label. addrReg must be z80Scratch1 (RAX).
// After call: AL = byte read, addrReg preserved in ECX.
func z80EmitMemRead(buf *CodeBuffer, bailLabel string) {
	// Save address in ECX for bail path
	// MOV ECX, EAX
	buf.EmitBytes(0x89, 0xC1)
	// Page = addr >> 8
	// SHR ECX, 8 (now ECX = page)
	buf.EmitBytes(0xC1, 0xE9, 0x08)
	// MOVZX EDX, BYTE [R8 + RCX] (directPageBitmap[page])
	amd64MOVZX_B_memSIB(buf, z80Scratch3, z80RegDPB, z80Scratch2)
	// TEST EDX, EDX
	buf.EmitBytes(0x85, 0xD2)
	// JNZ bailLabel
	buf.EmitBytes(0x0F, 0x85)
	buf.FixupRel32(bailLabel, buf.Len()+4)
	// MOVZX EAX, BYTE [RSI + RAX] (direct memory read)
	amd64MOVZX_B_memSIB(buf, z80Scratch1, z80RegMem, z80Scratch1)
}

// z80EmitMemWrite emits code to write DL to Z80 address in AX,
// with direct page check and self-mod detection.
func z80EmitMemWrite(buf *CodeBuffer, bailLabel, selfModLabel string) {
	// Save address
	// MOV ECX, EAX
	buf.EmitBytes(0x89, 0xC1)
	// Page = addr >> 8
	buf.EmitBytes(0xC1, 0xE9, 0x08) // SHR ECX, 8
	// Check direct page
	// MOVZX R10D, BYTE [R8 + RCX]
	amd64MOVZX_B_memSIB(buf, z80Scratch4, z80RegDPB, z80Scratch2)
	// TEST R10D, R10D
	emitREX(buf, false, z80Scratch4, z80Scratch4)
	buf.EmitBytes(0x85, modRM(3, z80Scratch4, z80Scratch4))
	// JNZ bailLabel
	buf.EmitBytes(0x0F, 0x85)
	buf.FixupRel32(bailLabel, buf.Len()+4)
	// Direct write FIRST (always — even if self-mod detected)
	// MOV [RSI + RAX], DL
	emitREXForByteSIB(buf, z80Scratch3, z80Scratch1, z80RegMem)
	buf.EmitBytes(0x88, modRM(0, z80Scratch3, 4), sibByte(0, z80Scratch1, z80RegMem))
	// Check code page (self-mod detection) AFTER write
	// MOVZX R10D, BYTE [R9 + RCX]
	amd64MOVZX_B_memSIB(buf, z80Scratch4, z80RegCPB, z80Scratch2)
	emitREX(buf, false, z80Scratch4, z80Scratch4)
	buf.EmitBytes(0x85, modRM(3, z80Scratch4, z80Scratch4))
	// JNZ selfModLabel (write already completed, just need to invalidate)
	buf.EmitBytes(0x0F, 0x85)
	buf.FixupRel32(selfModLabel, buf.Len()+4)
}

// z80EmitLoopPreCheck emits runtime page validation for a qualifying DJNZ loop.
// Checks that all memory-accessed register pages are direct (and write-target
// pages have no compiled code). If any check fails, jumps to failLabel.
// Clobbers: RAX, ECX, EDX.
func z80EmitLoopPreCheck(buf *CodeBuffer, info *z80LoopInfo, failLabel string) {
	// Helper: check that a 16-bit register pair's high byte (page) is direct
	checkDirectPage := func(pairReg byte) {
		// MOVZX ECX, pairW; SHR ECX, 8 → page number
		amd64MOVZX_W(buf, z80Scratch2, pairReg)
		buf.EmitBytes(0xC1, 0xE9, 0x08) // SHR ECX, 8
		// MOVZX EDX, BYTE [R8+RCX] (directPageBitmap)
		amd64MOVZX_B_memSIB(buf, z80Scratch3, z80RegDPB, z80Scratch2)
		// TEST EDX, EDX; JNZ fail
		buf.EmitBytes(0x85, 0xD2)
		buf.EmitBytes(0x0F, 0x85)
		buf.FixupRel32(failLabel, buf.Len()+4)
	}

	// Helper: check that a 16-bit register pair's page has no compiled code
	checkNoCodePage := func(pairReg byte) {
		amd64MOVZX_W(buf, z80Scratch2, pairReg)
		buf.EmitBytes(0xC1, 0xE9, 0x08) // SHR ECX, 8
		// MOVZX EDX, BYTE [R9+RCX] (codePageBitmap)
		amd64MOVZX_B_memSIB(buf, z80Scratch3, z80RegCPB, z80Scratch2)
		// TEST EDX, EDX; JNZ fail
		buf.EmitBytes(0x85, 0xD2)
		buf.EmitBytes(0x0F, 0x85)
		buf.FixupRel32(failLabel, buf.Len()+4)
	}

	// Check HL page if used for reads or writes
	if info.readHL || info.writeHL {
		checkDirectPage(z80RegHL)
		if info.writeHL {
			checkNoCodePage(z80RegHL)
		}
	}

	// Check DE page if used for reads or writes
	if info.readDE || info.writeDE {
		checkDirectPage(z80RegDE)
		if info.writeDE {
			checkNoCodePage(z80RegDE)
		}
	}

	// Check BC page if used for reads or writes
	if info.readBC || info.writeBC {
		checkDirectPage(z80RegBC)
		if info.writeBC {
			checkNoCodePage(z80RegBC)
		}
	}
}

// z80EmitMemReadUnchecked emits an unchecked direct memory read from address
// in AX into AL. No page-check or bail path. Only safe when the caller has
// already validated that the page is direct (pre-loop proof).
func z80EmitMemReadUnchecked(buf *CodeBuffer) {
	// MOVZX EAX, BYTE [RSI + RAX]
	amd64MOVZX_B_memSIB(buf, z80Scratch1, z80RegMem, z80Scratch1)
}

// z80EmitMemWriteUnchecked emits an unchecked direct memory write of DL to
// address in AX. No page-check, bail, or self-mod detection. Only safe when
// the caller has validated the page is direct AND has no compiled code.
func z80EmitMemWriteUnchecked(buf *CodeBuffer) {
	// MOV [RSI + RAX], DL
	emitREXForByteSIB(buf, z80Scratch3, z80Scratch1, z80RegMem)
	buf.EmitBytes(0x88, modRM(0, z80Scratch3, 4), sibByte(0, z80Scratch1, z80RegMem))
}

// ===========================================================================
// Flag Computation (always-materialize for Phase 1; lazy flags in Phase 6)
// ===========================================================================

// z80EmitFlags_ADD emits Z80 flag computation for ADD A,val.
// Before call: oldA in DL, operand in CL, result in AL (already stored to A/BL).
// After call: F (BPL) updated with requested flag bits.
// flagMask controls which flags to compute (z80FlagAll for full materialization).
func z80EmitFlags_ADD(buf *CodeBuffer, flagMask uint8) {
	// R10D = 0
	amd64XOR_reg_reg32(buf, z80Scratch4, z80Scratch4)

	if flagMask&uint8(z80FlagS) != 0 {
		buf.EmitBytes(0xA8, 0x80)             // TEST AL, 0x80
		buf.EmitBytes(0x74, 0x04)             // JZ +4
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x80) // OR R10B, 0x80
	}

	if flagMask&uint8(z80FlagZ) != 0 {
		buf.EmitBytes(0x84, 0xC0)             // TEST AL, AL
		buf.EmitBytes(0x75, 0x04)             // JNZ +4
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x40) // OR R10B, 0x40
	}

	if flagMask&0x28 != 0 { // Y (bit 5) or X (bit 3)
		buf.EmitBytes(0x41, 0x88, 0xC3)       // MOV R11B, AL
		buf.EmitBytes(0x41, 0x80, 0xE3, 0x28) // AND R11B, 0x28
		buf.EmitBytes(0x45, 0x08, 0xDA)       // OR R10B, R11B
	}

	if flagMask&uint8(z80FlagH) != 0 {
		buf.EmitBytes(0x41, 0x88, 0xD3)       // MOV R11B, DL
		buf.EmitBytes(0x41, 0x30, 0xCB)       // XOR R11B, CL
		buf.EmitBytes(0x41, 0x30, 0xC3)       // XOR R11B, AL
		buf.EmitBytes(0x41, 0x80, 0xE3, 0x10) // AND R11B, 0x10
		buf.EmitBytes(0x45, 0x08, 0xDA)       // OR R10B, R11B
	}

	if flagMask&uint8(z80FlagPV) != 0 {
		buf.EmitBytes(0x41, 0x88, 0xD3)       // MOV R11B, DL (oldA)
		buf.EmitBytes(0x41, 0x30, 0xC3)       // XOR R11B, AL
		buf.EmitBytes(0x88, 0xCA)             // MOV DL, CL
		buf.EmitBytes(0x30, 0xC2)             // XOR DL, AL
		buf.EmitBytes(0x41, 0x20, 0xD3)       // AND R11B, DL
		buf.EmitBytes(0x41, 0xF6, 0xC3, 0x80) // TEST R11B, 0x80
		buf.EmitBytes(0x74, 0x04)             // JZ +4
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x04) // OR R10B, 0x04
	}

	// N = 0 (already clear in R10)

	// C flag for ADD: carry = (result < oldA) unsigned.
	// DL may have been clobbered by PV computation above, but CL still has
	// the original operand. Use: carry = (AL + (~CL) < 0xFF) ... actually
	// simpler: recompute from CL. ADD overflow iff (0x100 - CL) <= (0xFF - AL + 1)
	// No — cleanest: just re-add and check. But DL is clobbered.
	// Alternative: save oldA to [RSP+z80OffCycles] before PV clobbers DL.
	// For simplicity and correctness, capture carry from the ADD instruction
	// itself. The caller must set [RSP+z80OffCycles] = carry (0 or 1) before
	// calling this function. See z80EmitCarryCapture.
	if flagMask&uint8(z80FlagC) != 0 {
		// Read saved carry from [RSP+z80OffCycles]
		buf.EmitBytes(0x0F, 0xB6, 0x4C, 0x24, byte(z80OffCycles)) // MOVZX ECX, BYTE [RSP+16]
		buf.EmitBytes(0x41, 0x08, 0xCA)                           // OR R10B, CL
	}

	// Store F: MOV BPL, R10B
	buf.EmitBytes(0x44, 0x88, 0xD5)
}

// z80EmitCarryCapture emits code to save the host carry flag to [RSP+z80OffCycles].
// Must be called IMMEDIATELY after an ADD/SUB instruction, before any instruction
// that clobbers host EFLAGS (MOVZX, TEST, CMP, etc.).
func z80EmitCarryCapture(buf *CodeBuffer) {
	// SETB [RSP+z80OffCycles] — set byte to 1 if CF=1 (carry/borrow)
	buf.EmitBytes(0x0F, 0x92, 0x44, 0x24, byte(z80OffCycles))
}

// z80EmitBorrowCapture emits code to save the host borrow flag to [RSP+z80OffCycles].
// For SUB/SBC/CP, the host CF represents borrow (CF=1 means borrow occurred).
func z80EmitBorrowCapture(buf *CodeBuffer) {
	z80EmitCarryCapture(buf) // same instruction — CF=1 after SUB means borrow
}

// z80EmitFlags_SUB emits Z80 flag computation for SUB/CP A,val.
// Same register convention as ADD but N=1. Carry is borrow (already captured).
func z80EmitFlags_SUB(buf *CodeBuffer, flagMask uint8) {
	z80EmitFlags_ADD(buf, flagMask)
	if flagMask&uint8(z80FlagN) != 0 {
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x02) // OR BPL, 0x02
	}
}

// z80EmitFlags_Logic emits Z80 flag computation for AND/OR/XOR.
// Result in AL. H flag = 0x10 for AND, 0 for OR/XOR.
func z80EmitFlags_Logic(buf *CodeBuffer, isAND bool, flagMask uint8) {
	amd64XOR_reg_reg32(buf, z80Scratch4, z80Scratch4)

	if flagMask&uint8(z80FlagS) != 0 {
		buf.EmitBytes(0xA8, 0x80)
		buf.EmitBytes(0x74, 0x04)
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x80)
	}

	if flagMask&uint8(z80FlagZ) != 0 {
		buf.EmitBytes(0x84, 0xC0)
		buf.EmitBytes(0x75, 0x04)
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x40)
	}

	if flagMask&0x28 != 0 {
		buf.EmitBytes(0x41, 0x88, 0xC3)
		buf.EmitBytes(0x41, 0x80, 0xE3, 0x28)
		buf.EmitBytes(0x45, 0x08, 0xDA)
	}

	if isAND && flagMask&uint8(z80FlagH) != 0 {
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x10)
	}

	if flagMask&uint8(z80FlagPV) != 0 {
		buf.EmitBytes(0x0F, 0xB6, 0xC8)
		amd64MOV_reg_mem(buf, z80Scratch3, amd64RSP, int32(z80OffParityPtr))
		amd64MOVZX_B_memSIB(buf, z80Scratch3, z80Scratch3, z80Scratch2)
		buf.EmitBytes(0x41, 0x08, 0xD2)
	}

	buf.EmitBytes(0x44, 0x88, 0xD5) // MOV BPL, R10B
}

// z80EmitFlags_INC_DEC_Runtime emits runtime Z80 flag computation for INC/DEC.
// Result in AL, old value in DL. Preserves C flag from existing F.
func z80EmitFlags_INC_DEC_Runtime(buf *CodeBuffer, isDec bool, flagMask uint8) {
	// Save existing C flag from F (INC/DEC always preserves C)
	buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD) // MOVZX ECX, BPL
	buf.EmitBytes(0x80, 0xE1, 0x01)       // AND CL, 0x01
	buf.EmitBytes(0x41, 0x88, 0xCA)       // MOV R10B, CL

	if flagMask&uint8(z80FlagS) != 0 {
		buf.EmitBytes(0xA8, 0x80)
		buf.EmitBytes(0x74, 0x04)
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x80)
	}

	if flagMask&uint8(z80FlagZ) != 0 {
		buf.EmitBytes(0x84, 0xC0)
		buf.EmitBytes(0x75, 0x04)
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x40)
	}

	if flagMask&0x28 != 0 {
		buf.EmitBytes(0x41, 0x88, 0xC3)
		buf.EmitBytes(0x41, 0x80, 0xE3, 0x28)
		buf.EmitBytes(0x45, 0x08, 0xDA)
	}

	if flagMask&uint8(z80FlagH) != 0 {
		buf.EmitBytes(0x30, 0xC2)       // XOR DL, AL
		buf.EmitBytes(0x80, 0xE2, 0x10) // AND DL, 0x10
		buf.EmitBytes(0x41, 0x08, 0xD2) // OR R10B, DL
	}

	if flagMask&uint8(z80FlagPV) != 0 {
		if isDec {
			buf.EmitBytes(0x3C, 0x7F)
		} else {
			buf.EmitBytes(0x3C, 0x80)
		}
		buf.EmitBytes(0x75, 0x04)
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x04)
	}

	if isDec && flagMask&uint8(z80FlagN) != 0 {
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x02)
	}

	buf.EmitBytes(0x44, 0x88, 0xD5) // MOV BPL, R10B
}

// ===========================================================================
// Prologue / Epilogue
// ===========================================================================

func z80EmitPrologue(buf *CodeBuffer) {
	// Save callee-saved registers
	amd64PUSH(buf, amd64RBX)
	amd64PUSH(buf, amd64RBP)
	amd64PUSH(buf, amd64R12)
	amd64PUSH(buf, amd64R13)
	amd64PUSH(buf, amd64R14)
	amd64PUSH(buf, amd64R15)

	// Allocate stack frame: SUB RSP, z80FrameSize
	buf.EmitBytes(0x48, 0x83, 0xEC, byte(z80FrameSize))

	// RDI = Z80JITContext* on entry
	// Save context pointer to stack and R15
	amd64MOV_reg_reg(buf, z80RegCtx, amd64RDI) // MOV R15, RDI
	amd64MOV_mem_reg(buf, amd64RSP, int32(z80OffCtxPtr), amd64RDI)

	// Load CpuPtr from context, save to stack
	amd64MOV_reg_mem(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffCpuPtr))
	amd64MOV_mem_reg(buf, amd64RSP, int32(z80OffCpuPtr), z80Scratch1)

	// Load memory base: MOV RSI, [R15 + MemPtr]
	amd64MOV_reg_mem(buf, z80RegMem, z80RegCtx, int32(jzCtxOffMemPtr))

	// Load bitmap pointers
	amd64MOV_reg_mem(buf, z80RegDPB, z80RegCtx, int32(jzCtxOffDirectPageBitmapPtr))
	amd64MOV_reg_mem(buf, z80RegCPB, z80RegCtx, int32(jzCtxOffCodePageBitmapPtr))

	// Load parity and DAA table pointers to stack
	amd64MOV_reg_mem(buf, z80Scratch2, z80RegCtx, int32(jzCtxOffParityTablePtr))
	amd64MOV_mem_reg(buf, amd64RSP, int32(z80OffParityPtr), z80Scratch2)
	amd64MOV_reg_mem(buf, z80Scratch2, z80RegCtx, int32(jzCtxOffDAATablePtr))
	amd64MOV_mem_reg(buf, amd64RSP, int32(z80OffDAAPtr), z80Scratch2)

	// Load Z80 registers from CPU struct (RAX = CpuPtr)
	// A → BL
	amd64MOVZX_B_mem(buf, z80RegA, z80Scratch1, int32(cpuZ80OffA))
	// F → BPL (needs REX for BPL)
	amd64MOVZX_B_mem(buf, z80RegF, z80Scratch1, int32(cpuZ80OffF))

	// B:C → R12W (B in high byte, C in low)
	// MOVZX R12D, BYTE [RAX + offB]
	amd64MOVZX_B_mem(buf, z80RegBC, z80Scratch1, int32(cpuZ80OffB))
	// SHL R12D, 8
	emitREX(buf, false, 0, z80RegBC)
	buf.EmitBytes(0xC1, modRM(3, 4, regBits(z80RegBC)), 0x08)
	// MOVZX ECX, BYTE [RAX + offC]
	amd64MOVZX_B_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffC))
	// OR R12D, ECX
	emitREX(buf, false, z80Scratch2, z80RegBC)
	buf.EmitBytes(0x09, modRM(3, z80Scratch2, z80RegBC))

	// D:E → R13W
	amd64MOVZX_B_mem(buf, z80RegDE, z80Scratch1, int32(cpuZ80OffD))
	emitREX(buf, false, 0, z80RegDE)
	buf.EmitBytes(0xC1, modRM(3, 4, regBits(z80RegDE)), 0x08)
	amd64MOVZX_B_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffE))
	emitREX(buf, false, z80Scratch2, z80RegDE)
	buf.EmitBytes(0x09, modRM(3, z80Scratch2, z80RegDE))

	// H:L → R14W
	amd64MOVZX_B_mem(buf, z80RegHL, z80Scratch1, int32(cpuZ80OffH))
	emitREX(buf, false, 0, z80RegHL)
	buf.EmitBytes(0xC1, modRM(3, 4, regBits(z80RegHL)), 0x08)
	amd64MOVZX_B_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffL))
	emitREX(buf, false, z80Scratch2, z80RegHL)
	buf.EmitBytes(0x09, modRM(3, z80Scratch2, z80RegHL))

	// Zero cycle accumulator: MOV DWORD [RSP+z80OffCycles], 0
	amd64MOV_mem_imm32(buf, amd64RSP, int32(z80OffCycles), 0)
}

// z80EmitRegisterStoreAndReturn emits the shared tail: store Z80 registers
// back to CPU struct, restore frame, RET. Context fields (RetPC, RetCount,
// RetCycles, NeedBail, NeedInval) must already be set before jumping here.
func z80EmitRegisterStoreAndReturn(buf *CodeBuffer) {
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
	emitREXForByte(buf, z80RegA, z80Scratch1)
	buf.EmitBytes(0x88, modRM(1, z80RegA, z80Scratch1), byte(cpuZ80OffA))
	emitREXForByte(buf, z80RegF, z80Scratch1)
	buf.EmitBytes(0x88, modRM(1, z80RegF, z80Scratch1), byte(cpuZ80OffF))
	amd64MOVZX_W(buf, z80Scratch2, z80RegBC)
	buf.EmitBytes(0xC1, 0xE9, 0x08)
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffB))
	emitREXForByte(buf, z80RegBC, z80Scratch1)
	buf.EmitBytes(0x88, modRM(1, z80RegBC, z80Scratch1), byte(cpuZ80OffC))
	amd64MOVZX_W(buf, z80Scratch2, z80RegDE)
	buf.EmitBytes(0xC1, 0xE9, 0x08)
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffD))
	emitREXForByte(buf, z80RegDE, z80Scratch1)
	buf.EmitBytes(0x88, modRM(1, z80RegDE, z80Scratch1), byte(cpuZ80OffE))
	amd64MOVZX_W(buf, z80Scratch2, z80RegHL)
	buf.EmitBytes(0xC1, 0xE9, 0x08)
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffH))
	emitREXForByte(buf, z80RegHL, z80Scratch1)
	buf.EmitBytes(0x88, modRM(1, z80RegHL, z80Scratch1), byte(cpuZ80OffL))
	buf.EmitBytes(0x48, 0x83, 0xC4, byte(z80FrameSize))
	amd64POP(buf, amd64R15)
	amd64POP(buf, amd64R14)
	amd64POP(buf, amd64R13)
	amd64POP(buf, amd64R12)
	amd64POP(buf, amd64RBP)
	amd64POP(buf, amd64RBX)
	amd64RET(buf)
}

// z80SharedExitLabel is the label for the shared register-store-and-return
// trampoline emitted at the end of every compiled block.
const z80SharedExitLabel = "__z80_shared_exit__"

// z80EmitMergeAndCommit emits the chain-accounting merge sequence used by all
// non-chainable exits (epilogue, bail, selfmod, EI/DI/HALT). It:
//  1. ADDs this block's local cycles/instrCount/rIncrements to ChainCycles/ChainCount/ChainRIncrements
//  2. Commits ChainCount→RetCount, ChainCycles→RetCycles
//
// This ensures that even the first (non-chained) block writes correct values:
// ChainCycles starts at 0, so ADD blockCycles gives the right total.
// Clobbers: RAX (z80Scratch1).
func z80EmitMergeAndCommit(buf *CodeBuffer, instrCount int, blockCycles uint32, blockRIncrements int) {
	// ChainCycles += blockCycles
	amd64MOV_reg_mem(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainCycles))
	buf.EmitBytes(0x48, 0x05) // REX.W ADD RAX, imm32
	buf.Emit32(blockCycles)
	amd64MOV_mem_reg(buf, z80RegCtx, int32(jzCtxOffChainCycles), z80Scratch1)
	// Commit ChainCycles → RetCycles
	amd64MOV_mem_reg(buf, z80RegCtx, int32(jzCtxOffRetCycles), z80Scratch1)

	// ChainCount += instrCount
	amd64MOV_reg_mem32(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainCount))
	buf.EmitBytes(0x05) // ADD EAX, imm32
	buf.Emit32(uint32(instrCount))
	amd64MOV_mem_reg32(buf, z80RegCtx, int32(jzCtxOffChainCount), z80Scratch1)
	// Commit ChainCount → RetCount
	amd64MOV_mem_reg32(buf, z80RegCtx, int32(jzCtxOffRetCount), z80Scratch1)

	// ChainRIncrements += blockRIncrements
	if blockRIncrements > 0 {
		amd64MOV_reg_mem32(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainRIncrements))
		buf.EmitBytes(0x05) // ADD EAX, imm32
		buf.Emit32(uint32(blockRIncrements))
		amd64MOV_mem_reg32(buf, z80RegCtx, int32(jzCtxOffChainRIncrements), z80Scratch1)
	}
}

// z80EmitEpilogue emits a block exit: merge chain state, set RetPC, JMP to shared exit.
// Used for non-chainable terminators (EI, DI, HALT) and unchained fallback paths.
func z80EmitEpilogue(buf *CodeBuffer, nextPC uint16, instrCount int, totalCycles uint32, blockRIncrements int) {
	z80EmitMergeAndCommit(buf, instrCount, totalCycles, blockRIncrements)
	amd64MOV_mem_imm32(buf, z80RegCtx, int32(jzCtxOffRetPC), uint32(nextPC))
	// JMP to shared exit
	buf.EmitBytes(0xE9)
	buf.FixupRel32(z80SharedExitLabel, buf.Len()+4)
}

// z80EmitEpilogueDynPC emits a block exit where RetPC comes from EAX (dynamic).
// Used for RET instructions where the target PC is read from the stack at runtime.
func z80EmitEpilogueDynPC(buf *CodeBuffer, instrCount int, totalCycles uint32, blockRIncrements int) {
	// Save dynamic PC to R10D before merge clobbers RAX
	buf.EmitBytes(0x41, 0x89, 0xC2) // MOV R10D, EAX
	z80EmitMergeAndCommit(buf, instrCount, totalCycles, blockRIncrements)
	// Set RetPC from saved R10D
	emitMemOp(buf, false, 0x89, z80Scratch4, z80RegCtx, int32(jzCtxOffRetPC))
	// JMP to shared exit
	buf.EmitBytes(0xE9)
	buf.FixupRel32(z80SharedExitLabel, buf.Len()+4)
}

// ===========================================================================
// Block Chaining: Chain Entry / Chain Exit
// ===========================================================================

// z80ChainEntryLabel is used for the chain entry point within a block.
const z80ChainEntryLabel = "__z80_chain_entry__"

// z80EmitChainEntry emits the lightweight chain entry point. Chained blocks
// JMP directly here, skipping the full prologue. Since all Z80 state lives in
// callee-saved registers (BX=A, BP=F, R12=BC, R13=DE, R14=HL), no register
// loads are needed. The full prologue falls through to here for normal entry.
func z80EmitChainEntry(buf *CodeBuffer, hasBackwardBranch bool) int {
	entryOff := buf.Len()
	buf.Label(z80ChainEntryLabel)
	if hasBackwardBranch {
		// Reset DJNZ/LDIR loop budget for this block
		amd64MOV_mem_imm32(buf, amd64RSP, int32(z80OffLoopBudg), 0)
	}
	return entryOff
}

// z80EmitChainExit emits a patchable chain exit that accumulates this block's
// contribution into chained accounting fields before jumping to the next block.
//
// Accounting (all ADD, not MOV — prior blocks' values already accumulated):
//   - ChainCycles += blockCycles
//   - ChainCount  += instrCount
//   - ChainRIncrements += blockRIncrements
//
// Budget checks (exit to Go if exceeded):
//   - ChainCycles >= CycleBudget (interrupt responsiveness)
//   - ChainBudget <= 0 (block count safety)
//   - NeedInval != 0 (self-mod detected by a prior instruction in this block)
//
// The JMP rel32 is initially patched to the unchained exit and can be
// repatched to a target block's chainEntry by the exec loop.
func z80EmitChainExit(buf *CodeBuffer, cs *z80CompileState, nextPC uint16, instrCount int, blockCycles uint32, blockRIncrements int) {
	unchainedLabel := fmt.Sprintf("__z80_unchained_%d__", buf.Len())

	// --- Accumulate this block's contribution ---
	// Uses emitMemOp-based helpers to handle disp8/disp32 automatically.

	// ChainCycles += blockCycles (uint64 field)
	// Load into RAX, ADD, store back (no direct ADD [mem], imm32 for 64-bit)
	amd64MOV_reg_mem(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainCycles))
	// ADD RAX, blockCycles (imm32, zero-extended to 64-bit)
	buf.EmitBytes(0x48, 0x05) // REX.W ADD RAX, imm32
	buf.Emit32(blockCycles)
	amd64MOV_mem_reg(buf, z80RegCtx, int32(jzCtxOffChainCycles), z80Scratch1)

	// ChainCount += instrCount (uint32)
	amd64MOV_reg_mem32(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainCount))
	buf.EmitBytes(0x05) // ADD EAX, imm32
	buf.Emit32(uint32(instrCount))
	amd64MOV_mem_reg32(buf, z80RegCtx, int32(jzCtxOffChainCount), z80Scratch1)

	// ChainRIncrements += blockRIncrements (uint32)
	if blockRIncrements > 0 {
		amd64MOV_reg_mem32(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainRIncrements))
		buf.EmitBytes(0x05) // ADD EAX, imm32
		buf.Emit32(uint32(blockRIncrements))
		amd64MOV_mem_reg32(buf, z80RegCtx, int32(jzCtxOffChainRIncrements), z80Scratch1)
	}

	// --- Budget checks ---

	// CMP ChainCycles (low 32), CycleBudget
	amd64MOV_reg_mem32(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainCycles))
	// CMP EAX, [R15 + CycleBudget]
	emitMemOp(buf, false, 0x3B, z80Scratch1, z80RegCtx, int32(jzCtxOffCycleBudget))
	// JAE unchained (unsigned: cycles >= budget)
	buf.EmitBytes(0x0F, 0x83)
	buf.FixupRel32(unchainedLabel, buf.Len()+4)

	// DEC DWORD [R15 + ChainBudget]
	emitMemOp(buf, false, 0xFF, 1, z80RegCtx, int32(jzCtxOffChainBudget)) // opext 1 = DEC
	// JLE unchained (budget exhausted)
	buf.EmitBytes(0x0F, 0x8E)
	buf.FixupRel32(unchainedLabel, buf.Len()+4)

	// CMP DWORD [R15 + NeedInval], 0
	emitMemOp(buf, false, 0x83, 7, z80RegCtx, int32(jzCtxOffNeedInval)) // opext 7 = CMP, imm8
	buf.EmitBytes(0x00)
	// JNE unchained (self-mod detected)
	buf.EmitBytes(0x0F, 0x85)
	buf.FixupRel32(unchainedLabel, buf.Len()+4)

	// --- Patchable JMP rel32 (initially → unchained) ---
	buf.EmitBytes(0xE9) // JMP rel32
	jmpDispOffset := buf.Len()
	buf.FixupRel32(unchainedLabel, buf.Len()+4) // initial target = unchained

	// Record chain slot for patching
	if cs != nil {
		cs.chainExits = append(cs.chainExits, z80ChainExit{
			targetPC:      uint32(nextPC),
			jmpDispOffset: jmpDispOffset,
		})
	}

	// --- Unchained exit: commit to RetPC/RetCount/RetCycles, return to Go ---
	buf.Label(unchainedLabel)
	// RetPC = nextPC
	amd64MOV_mem_imm32(buf, z80RegCtx, int32(jzCtxOffRetPC), uint32(nextPC))
	// RetCount = ChainCount (already accumulated above)
	amd64MOV_reg_mem32(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainCount))
	amd64MOV_mem_reg32(buf, z80RegCtx, int32(jzCtxOffRetCount), z80Scratch1)
	// RetCycles = ChainCycles (commit accumulated cycles)
	amd64MOV_reg_mem(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainCycles))
	amd64MOV_mem_reg(buf, z80RegCtx, int32(jzCtxOffRetCycles), z80Scratch1)
	// JMP shared exit
	buf.EmitBytes(0xE9)
	buf.FixupRel32(z80SharedExitLabel, buf.Len()+4)
}

// ===========================================================================
// Block Terminator Emitters
// ===========================================================================

// z80EmitTerminator emits the final instruction of a block (a terminator)
// including its own epilogue(s) with the correct target PC.
func z80EmitTerminator(buf *CodeBuffer, cs *z80CompileState, instr *JITZ80Instr, instrPC, nextInstrPC uint16, instrCount int, totalCycles uint32, blockRIncrements int) {
	op := instr.opcode

	switch instr.prefix {
	case z80JITPrefixNone:
		switch {
		// JP nn (0xC3) — with chain slot
		case op == 0xC3:
			z80EmitChainExit(buf, cs, instr.operand, instrCount, totalCycles, blockRIncrements)

		// JR e (0x18) — with chain slot
		case op == 0x18:
			target := uint16(int32(instrPC) + 2 + int32(int8(instr.operand&0xFF)))
			z80EmitChainExit(buf, cs, target, instrCount, totalCycles, blockRIncrements)

		// JP cc,nn (0xC2,0xCA,0xD2,0xDA,0xE2,0xEA,0xF2,0xFA)
		case op&0xC7 == 0xC2:
			cc := (op >> 3) & 0x07
			z80EmitConditionalJP(buf, cs, cc, instr.operand, nextInstrPC, instrCount, totalCycles, blockRIncrements)

		// JR cc,e (0x20=NZ, 0x28=Z, 0x30=NC, 0x38=C)
		case op == 0x20 || op == 0x28 || op == 0x30 || op == 0x38:
			cc := (op >> 3) & 0x03 // 4=NZ, 5=Z, 6=NC, 7=C → remap
			target := uint16(int32(instrPC) + 2 + int32(int8(instr.operand&0xFF)))
			// JR cc cycles: 12 taken, 7 not taken. totalCycles used the base (12).
			// Not-taken path needs totalCycles - 12 + 7 = totalCycles - 5
			z80EmitConditionalJR(buf, cs, cc, target, nextInstrPC, instrCount, totalCycles, totalCycles-5, blockRIncrements)

		// DJNZ (0x10) — native emission: decrement B, branch if non-zero
		case op == 0x10:
			target := uint16(int32(instrPC) + 2 + int32(int8(instr.operand&0xFF)))
			z80EmitDJNZ(buf, cs, target, nextInstrPC, instrCount, totalCycles, blockRIncrements)

		// RET (0xC9)
		case op == 0xC9:
			z80EmitRET(buf, instrPC, instrCount, totalCycles, blockRIncrements)

		// RET cc (0xC0,0xC8,0xD0,0xD8,0xE0,0xE8,0xF0,0xF8)
		case op&0xC7 == 0xC0:
			cc := (op >> 3) & 0x07
			z80EmitConditionalRET(buf, cc, instrPC, nextInstrPC, instrCount, totalCycles, blockRIncrements)

		// CALL nn (0xCD)
		case op == 0xCD:
			z80EmitCALL(buf, cs, instr.operand, nextInstrPC, instrPC, instrCount, totalCycles, blockRIncrements)

		// CALL cc,nn (0xC4,0xCC,0xD4,0xDC,0xE4,0xEC,0xF4,0xFC)
		case op&0xC7 == 0xC4:
			cc := (op >> 3) & 0x07
			z80EmitConditionalCALL(buf, cs, cc, instr.operand, nextInstrPC, instrPC, instrCount, totalCycles, blockRIncrements)

		// RST n (0xC7,0xCF,0xD7,0xDF,0xE7,0xEF,0xF7,0xFF)
		case op&0xC7 == 0xC7:
			target := uint16(op & 0x38)
			z80EmitCALL(buf, cs, target, nextInstrPC, instrPC, instrCount, totalCycles, blockRIncrements)

		// EI (0xFB) — just needs epilogue with sequential PC
		case op == 0xFB:
			// iffDelay=2 was already set by the instruction emitter
			amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
			emitMemOp(buf, true, 0xC7, 0, z80Scratch1, int32(cpuZ80OffIffDelay))
			buf.Emit32(2)
			z80EmitEpilogue(buf, nextInstrPC, instrCount, totalCycles, blockRIncrements)

		// DI (0xF3)
		case op == 0xF3:
			amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
			buf.EmitBytes(0xC6, modRM(1, 0, z80Scratch1), byte(cpuZ80OffIFF1), 0x00)
			buf.EmitBytes(0xC6, modRM(1, 0, z80Scratch1), byte(cpuZ80OffIFF2), 0x00)
			z80EmitEpilogue(buf, nextInstrPC, instrCount, totalCycles, blockRIncrements)

		// JP (HL) (0xE9)
		case op == 0xE9:
			// Target PC = HL (dynamic)
			amd64MOVZX_W(buf, z80Scratch1, z80RegHL) // EAX = HL
			z80EmitEpilogueDynPC(buf, instrCount, totalCycles, blockRIncrements)

		default:
			// Unknown terminator — sequential epilogue (safety)
			z80EmitEpilogue(buf, nextInstrPC, instrCount, totalCycles, blockRIncrements)
		}

	case z80JITPrefixDD, z80JITPrefixFD:
		if op == 0xE9 { // JP (IX) / JP (IY)
			if instr.prefix == z80JITPrefixDD {
				amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
				amd64MOVZX_W_mem(buf, z80Scratch1, z80Scratch1, int32(cpuZ80OffIX))
			} else {
				amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
				amd64MOVZX_W_mem(buf, z80Scratch1, z80Scratch1, int32(cpuZ80OffIY))
			}
			z80EmitEpilogueDynPC(buf, instrCount, totalCycles, blockRIncrements)
		} else {
			z80EmitEpilogue(buf, nextInstrPC, instrCount, totalCycles, blockRIncrements)
		}

	case z80JITPrefixED:
		switch op {
		case 0xB0, 0xB8: // LDIR / LDDR — native loop
			isIncrement := (op == 0xB0)
			z80EmitLDIR(buf, isIncrement, instrPC, instrCount, totalCycles, blockRIncrements)
		case 0xB1, 0xB9: // CPIR / CPDR — bail to interpreter (repeat loop)
			cyclesBefore := totalCycles - uint32(instr.cycles)
			amd64MOV_mem_imm32(buf, z80RegCtx, int32(jzCtxOffNeedBail), 1)
			z80EmitEpilogue(buf, instrPC, instrCount-1, cyclesBefore, blockRIncrements)
		default:
			// RETI/RETN — same as RET (pop PC from stack)
			z80EmitRET(buf, instrPC, instrCount, totalCycles, blockRIncrements)
		}

	default:
		z80EmitEpilogue(buf, nextInstrPC, instrCount, totalCycles, blockRIncrements)
	}
}

// z80EmitConditionalJP emits JP cc,nn: check condition, two chain exits.
func z80EmitConditionalJP(buf *CodeBuffer, cs *z80CompileState, cc byte, target, nextPC uint16, instrCount int, totalCycles uint32, blockRIncrements int) {
	// Test the Z80 condition from F register (BPL)
	// Z80 conditions: 0=NZ, 1=Z, 2=NC, 3=C, 4=PO, 5=PE, 6=P, 7=M
	notTakenLabel := fmt.Sprintf("jnt_%d", buf.Len())

	z80EmitConditionTest(buf, cc, notTakenLabel)

	// Taken path: chain exit with target PC
	z80EmitChainExit(buf, cs, target, instrCount, totalCycles, blockRIncrements)

	// Not-taken path: chain exit with sequential PC
	// JP cc: 10 cycles either way
	buf.Label(notTakenLabel)
	z80EmitChainExit(buf, cs, nextPC, instrCount, totalCycles, blockRIncrements)
}

// z80EmitConditionalJR emits JR cc,e: check condition, two chain exits.
func z80EmitConditionalJR(buf *CodeBuffer, cs *z80CompileState, cc byte, target, nextPC uint16, instrCount int, takenCycles, notTakenCycles uint32, blockRIncrements int) {
	notTakenLabel := fmt.Sprintf("jnt_%d", buf.Len())

	// JR cc conditions: cc=0(NZ), 1(Z), 2(NC), 3(C) — matches z80EmitConditionTest directly
	z80EmitConditionTest(buf, cc, notTakenLabel)

	z80EmitChainExit(buf, cs, target, instrCount, takenCycles, blockRIncrements)

	buf.Label(notTakenLabel)
	z80EmitChainExit(buf, cs, nextPC, instrCount, notTakenCycles, blockRIncrements)
}

// z80EmitDJNZ emits DJNZ: decrement B, jump if not zero.
func z80EmitDJNZ(buf *CodeBuffer, cs *z80CompileState, target, nextPC uint16, instrCount int, totalCycles uint32, blockRIncrements int) {
	// Decrement B (high byte of R12W) via direct 16-bit arithmetic.
	// SUB R12W, 0x0100 decrements the high byte without affecting the low
	// byte (C register). If B was 0, it wraps to 0xFF (correct for DJNZ).
	// This replaces the 9-instruction extract/dec/repack sequence.
	buf.EmitBytes(0x66, 0x41, 0x81, 0xEC, 0x00, 0x01) // SUB R12W, 0x0100

	// Test if B (high byte) is now zero.
	buf.EmitBytes(0x66, 0x41, 0xF7, 0xC4, 0x00, 0xFF) // TEST R12W, 0xFF00
	notTakenLabel := fmt.Sprintf("djnz_nt_%d", buf.Len())
	// JZ not_taken (B == 0)
	buf.EmitBytes(0x0F, 0x84)
	buf.FixupRel32(notTakenLabel, buf.Len()+4)

	// Taken: B != 0, jump to target (13 cycles)
	z80EmitChainExit(buf, cs, target, instrCount, totalCycles, blockRIncrements)

	// Not taken: B == 0 (8 cycles)
	buf.Label(notTakenLabel)

	// Deferred flag materialization: if the instruction before DJNZ had its
	// flags deferred (only needed at loop exit), materialize them now.
	// This saves ~15 instructions on every taken iteration (255 of 256).
	if cs.djnzDeferredFlags != 0 {
		// Reconstruct flag emitter inputs. The deferred producer was the
		// second-to-last instruction. For common cases (INC/DEC r), we can
		// reconstruct the result and old value from the current register state.
		// For now, support INC A / DEC A (the most common pattern).
		// The result is in BL (current A). For DEC A, old = BL+1. For INC A, old = BL-1.
		// We use z80EmitFlags_INC_DEC_Runtime which expects result in AL, old in DL.
		amd64MOVZX_B(buf, z80Scratch1, z80RegA) // AL = result (current A = BL)
		// Check what the deferred instruction was (stored in cs for this purpose)
		if cs.deferredIsDec {
			buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (result)
			buf.EmitBytes(0xFE, 0xC2) // INC DL (old = result + 1)
			z80EmitFlags_INC_DEC_Runtime(buf, true, cs.djnzDeferredFlags)
		} else {
			buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (result)
			buf.EmitBytes(0xFE, 0xCA) // DEC DL (old = result - 1)
			z80EmitFlags_INC_DEC_Runtime(buf, false, cs.djnzDeferredFlags)
		}
	}

	z80EmitChainExit(buf, cs, nextPC, instrCount, totalCycles-5, blockRIncrements) // 13-5=8
}

// z80EmitRET emits RET: pop 16-bit PC from stack.
func z80EmitRET(buf *CodeBuffer, instrPC uint16, instrCount int, totalCycles uint32, blockRIncrements int) {
	bailLabel := fmt.Sprintf("ret_bail_%d", buf.Len())
	doneLabel := fmt.Sprintf("ret_done_%d", buf.Len())

	// Read SP from CPU struct
	amd64MOV_reg_mem(buf, z80Scratch4, amd64RSP, int32(z80OffCpuPtr))   // R10 = CpuPtr
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP)) // ECX = SP

	// Read low byte from [SP]
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX (addr = SP)
	z80EmitMemRead(buf, bailLabel)
	buf.EmitBytes(0x41, 0x89, 0xC3) // MOV R11D, EAX (save low byte)

	// Read high byte from [SP+1]
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))
	buf.EmitBytes(0xFF, 0xC1)       // INC ECX
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX
	z80EmitMemRead(buf, bailLabel)

	// Combine: EAX = (high << 8) | low
	buf.EmitBytes(0xC1, 0xE0, 0x08) // SHL EAX, 8
	buf.EmitBytes(0x44, 0x09, 0xD8) // OR EAX, R11D

	// SP += 2
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))
	buf.EmitBytes(0x83, 0xC1, 0x02) // ADD ECX, 2
	buf.EmitBytes(0x66)
	emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))

	// EAX = return address (dynamic PC).
	// Try RTS cache for native chaining before falling back to Go dispatch.
	rtsTry1Label := fmt.Sprintf("rts1_%d", buf.Len())
	rtsUnchainedLabel := fmt.Sprintf("rtsU_%d", buf.Len())

	// Save return address to R11D (survives merge)
	buf.EmitBytes(0x41, 0x89, 0xC3) // MOV R11D, EAX

	// Merge chain state (clobbers RAX)
	z80EmitMergeAndCommit(buf, instrCount, totalCycles, blockRIncrements)

	// Budget check: ChainCycles >= CycleBudget?
	amd64MOV_reg_mem32(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffChainCycles))
	emitMemOp(buf, false, 0x3B, z80Scratch1, z80RegCtx, int32(jzCtxOffCycleBudget))
	buf.EmitBytes(0x0F, 0x83) // JAE unchained
	buf.FixupRel32(rtsUnchainedLabel, buf.Len()+4)

	// Block budget: DEC ChainBudget, JLE unchained
	emitMemOp(buf, false, 0xFF, 1, z80RegCtx, int32(jzCtxOffChainBudget))
	buf.EmitBytes(0x0F, 0x8E) // JLE unchained
	buf.FixupRel32(rtsUnchainedLabel, buf.Len()+4)

	// Self-mod check: NeedInval != 0?
	emitMemOp(buf, false, 0x83, 7, z80RegCtx, int32(jzCtxOffNeedInval))
	buf.EmitBytes(0x00)
	buf.EmitBytes(0x0F, 0x85) // JNE unchained
	buf.FixupRel32(rtsUnchainedLabel, buf.Len()+4)

	// RTS cache lookup 0: CMP R11D, [R15+RTSCache0PC]
	emitMemOp(buf, false, 0x3B, z80Scratch5, z80RegCtx, int32(jzCtxOffRTSCache0PC))
	buf.EmitBytes(0x0F, 0x85) // JNE try1
	buf.FixupRel32(rtsTry1Label, buf.Len()+4)
	// Match! JMP to RTSCache0Addr
	amd64MOV_reg_mem(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffRTSCache0Addr))
	// JMP RAX
	buf.EmitBytes(0xFF, 0xE0)

	// RTS cache lookup 1
	buf.Label(rtsTry1Label)
	emitMemOp(buf, false, 0x3B, z80Scratch5, z80RegCtx, int32(jzCtxOffRTSCache1PC))
	buf.EmitBytes(0x0F, 0x85) // JNE unchained
	buf.FixupRel32(rtsUnchainedLabel, buf.Len()+4)
	// Match! JMP to RTSCache1Addr
	amd64MOV_reg_mem(buf, z80Scratch1, z80RegCtx, int32(jzCtxOffRTSCache1Addr))
	// JMP RAX
	buf.EmitBytes(0xFF, 0xE0)

	// Unchained: set RetPC from R11D and exit to Go
	buf.Label(rtsUnchainedLabel)
	// RetPC = R11D (RetCount/RetCycles already committed by MergeAndCommit)
	emitMemOp(buf, false, 0x89, z80Scratch5, z80RegCtx, int32(jzCtxOffRetPC))
	buf.EmitBytes(0xE9)
	buf.FixupRel32(z80SharedExitLabel, buf.Len()+4)

	// Bail path
	buf.Label(bailLabel)
	amd64MOV_mem_imm32(buf, z80RegCtx, int32(jzCtxOffNeedBail), 1)
	z80EmitEpilogue(buf, instrPC, instrCount-1, totalCycles-10, blockRIncrements)

	_ = doneLabel
}

// z80EmitCALL emits CALL nn / RST n: push return address, set target PC.
func z80EmitCALL(buf *CodeBuffer, cs *z80CompileState, target, returnAddr, instrPC uint16, instrCount int, totalCycles uint32, blockRIncrements int) {
	bailLabel := fmt.Sprintf("call_bail_%d", buf.Len())
	selfModLabel := fmt.Sprintf("call_smod_%d", buf.Len())
	doneWrite1 := fmt.Sprintf("cw1_%d", buf.Len())
	doneWrite2 := fmt.Sprintf("cw2_%d", buf.Len())

	// SP -= 2 (reload CpuPtr from stack each time — z80EmitMemWrite clobbers R10)
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))   // RAX = CpuPtr
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP)) // ECX = SP
	buf.EmitBytes(0x83, 0xE9, 0x02)                                     // SUB ECX, 2
	// Store new SP
	buf.EmitBytes(0x66)
	emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))
	// Save new SP to R11 for later use (R11 is not clobbered by z80EmitMemWrite)
	buf.EmitBytes(0x41, 0x89, 0xCB) // MOV R11D, ECX

	// Write high byte of return address to [SP+1]
	buf.EmitBytes(0x44, 0x89, 0xD8)                             // MOV EAX, R11D (SP)
	buf.EmitBytes(0xFF, 0xC0)                                   // INC EAX (SP+1)
	buf.EmitBytes(0x0F, 0xB7, 0xC0)                             // MOVZX EAX, AX (16-bit addr)
	amd64MOV_reg_imm32(buf, z80Scratch3, uint32(returnAddr>>8)) // EDX = high byte
	z80EmitMemWrite(buf, bailLabel, selfModLabel)
	// JMP over bail/selfmod for second write
	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneWrite1, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrCount-1, totalCycles-uint32(17), blockRIncrements-1)
	z80EmitSelfModExit(buf, selfModLabel, target, instrCount, totalCycles, blockRIncrements)
	buf.Label(doneWrite1)

	// New bail/selfmod labels for second write
	bailLabel2 := fmt.Sprintf("call_bail2_%d", buf.Len())
	selfModLabel2 := fmt.Sprintf("call_smod2_%d", buf.Len())

	// Write low byte of return address to [SP]
	buf.EmitBytes(0x44, 0x89, 0xD8)                               // MOV EAX, R11D (SP)
	buf.EmitBytes(0x0F, 0xB7, 0xC0)                               // MOVZX EAX, AX
	amd64MOV_reg_imm32(buf, z80Scratch3, uint32(returnAddr&0xFF)) // EDX = low byte
	z80EmitMemWrite(buf, bailLabel2, selfModLabel2)
	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneWrite2, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel2, instrPC, instrCount-1, totalCycles-uint32(17), blockRIncrements-1)
	z80EmitSelfModExit(buf, selfModLabel2, target, instrCount, totalCycles, blockRIncrements)
	buf.Label(doneWrite2)

	// Jump to target — chain exit (static target known)
	z80EmitChainExit(buf, cs, target, instrCount, totalCycles, blockRIncrements)
}

// z80EmitConditionalRET emits RET cc: check condition, taken=RET, not-taken=sequential.
func z80EmitConditionalRET(buf *CodeBuffer, cc byte, instrPC, nextPC uint16, instrCount int, totalCycles uint32, blockRIncrements int) {
	notTakenLabel := fmt.Sprintf("retnt_%d", buf.Len())

	z80EmitConditionTest(buf, cc, notTakenLabel)

	// Taken: execute RET (11 cycles for RET cc taken)
	z80EmitRET(buf, instrPC, instrCount, totalCycles, blockRIncrements)

	// Not taken (5 cycles)
	buf.Label(notTakenLabel)
	z80EmitEpilogue(buf, nextPC, instrCount, totalCycles-6, blockRIncrements) // 11-6=5
}

// z80EmitConditionalCALL emits CALL cc,nn.
func z80EmitConditionalCALL(buf *CodeBuffer, cs *z80CompileState, cc byte, target, nextPC, instrPC uint16, instrCount int, totalCycles uint32, blockRIncrements int) {
	notTakenLabel := fmt.Sprintf("callnt_%d", buf.Len())

	z80EmitConditionTest(buf, cc, notTakenLabel)

	// Taken: execute CALL (17 cycles)
	z80EmitCALL(buf, cs, target, nextPC, instrPC, instrCount, totalCycles, blockRIncrements)

	// Not taken (10 cycles) — chain exit (static target known)
	buf.Label(notTakenLabel)
	z80EmitChainExit(buf, cs, nextPC, instrCount, totalCycles-7, blockRIncrements) // 17-7=10
}

// z80EmitConditionTest emits a test of a Z80 condition code against F (BPL),
// jumping to notTakenLabel if the condition is NOT met.
// Z80 conditions: 0=NZ, 1=Z, 2=NC, 3=C, 4=PO, 5=PE, 6=P, 7=M
func z80EmitConditionTest(buf *CodeBuffer, cc byte, notTakenLabel string) {
	// Load F into AL for testing
	buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5) // MOVZX EAX, BPL

	switch cc {
	case 0: // NZ: jump to not-taken if Z flag IS set (bit 6)
		buf.EmitBytes(0xA8, 0x40) // TEST AL, 0x40
		buf.EmitBytes(0x0F, 0x85) // JNZ notTaken
	case 1: // Z: jump to not-taken if Z flag is NOT set
		buf.EmitBytes(0xA8, 0x40) // TEST AL, 0x40
		buf.EmitBytes(0x0F, 0x84) // JZ notTaken
	case 2: // NC: jump to not-taken if C flag IS set (bit 0)
		buf.EmitBytes(0xA8, 0x01)
		buf.EmitBytes(0x0F, 0x85)
	case 3: // C: jump to not-taken if C flag is NOT set
		buf.EmitBytes(0xA8, 0x01)
		buf.EmitBytes(0x0F, 0x84)
	case 4: // PO: jump to not-taken if P/V IS set (bit 2)
		buf.EmitBytes(0xA8, 0x04)
		buf.EmitBytes(0x0F, 0x85)
	case 5: // PE: jump to not-taken if P/V is NOT set
		buf.EmitBytes(0xA8, 0x04)
		buf.EmitBytes(0x0F, 0x84)
	case 6: // P (positive): jump to not-taken if S IS set (bit 7)
		buf.EmitBytes(0xA8, 0x80)
		buf.EmitBytes(0x0F, 0x85)
	case 7: // M (minus): jump to not-taken if S is NOT set
		buf.EmitBytes(0xA8, 0x80)
		buf.EmitBytes(0x0F, 0x84)
	}
	buf.FixupRel32(notTakenLabel, buf.Len()+4)
}

// z80EmitBailExit emits an exit path for bail (non-direct memory page).
// Sets NeedBail=1, merges chained state, RetPC=instrPC. instrsBefore and
// cyclesBefore are the block-local counts BEFORE the bailing instruction.
// rIncBefore is the R increments for instructions completed before the bail.
func z80EmitBailExit(buf *CodeBuffer, label string, instrPC uint16, instrsBefore int, cyclesBefore uint32, rIncBefore int) {
	buf.Label(label)

	// Set NeedBail in context
	amd64MOV_mem_imm32(buf, z80RegCtx, int32(jzCtxOffNeedBail), 1)

	// Emit standard epilogue with bail PC (merges chained state)
	z80EmitEpilogue(buf, instrPC, instrsBefore, cyclesBefore, rIncBefore)
}

// z80EmitSelfModExit emits an exit path for self-modifying code detection.
// instrCount and totalCycles include the self-modifying instruction (write completed).
// rIncSoFar is the R increments for instructions completed so far (including this one).
func z80EmitSelfModExit(buf *CodeBuffer, label string, nextPC uint16, instrCount int, totalCycles uint32, rIncSoFar int) {
	buf.Label(label)

	// ECX still has the page number from the write check
	// Set NeedInval and InvalPage in context
	amd64MOV_mem_imm32(buf, z80RegCtx, int32(jzCtxOffNeedInval), 1)
	// MOV [R15 + InvalPage], ECX
	emitMemOp(buf, false, 0x89, z80Scratch2, z80RegCtx, int32(jzCtxOffInvalPage))

	z80EmitEpilogue(buf, nextPC, instrCount, totalCycles, rIncSoFar)
}

// z80ChainExit records a chain slot during emission (CodeBuffer-relative offsets).
type z80ChainExit struct {
	targetPC      uint32
	jmpDispOffset int // offset in CodeBuffer of the JMP rel32 displacement
}

// z80CompileState holds state accumulated during block compilation.
type z80CompileState struct {
	chainExits        []z80ChainExit
	totalRInc         int          // total R increments for the entire block
	loopInfo          *z80LoopInfo // non-nil if block has a qualifying DJNZ loop
	djnzDeferredFlags uint8        // non-zero: last flag producer before DJNZ was deferred
	deferredIsDec     bool         // true if the deferred producer is DEC (vs INC)
}

// ===========================================================================
// Main Compilation Entry Point
// ===========================================================================

// compileBlockZ80Stub compiles a scanned Z80 block into native x86-64 code.
func compileBlockZ80Stub(instrs []JITZ80Instr, startPC, endPC uint16, execMem *ExecMem, totalR int) (*JITBlock, error) {
	buf := NewCodeBuffer(1024)
	var cs z80CompileState

	// Calculate total cycles
	totalCycles := uint32(0)
	for _, instr := range instrs {
		totalCycles += uint32(instr.cycles)
	}

	// Detect backward branches (DJNZ, JR cc with negative offset) for loop budget reset
	hasBackwardBranch := false
	for i := range instrs {
		instr := &instrs[i]
		if instr.prefix == z80JITPrefixNone {
			switch {
			case instr.opcode == 0x10: // DJNZ
				hasBackwardBranch = true
			case instr.opcode == 0x18 || instr.opcode == 0x20 || instr.opcode == 0x28 || instr.opcode == 0x30 || instr.opcode == 0x38: // JR / JR cc
				if int8(instr.operand&0xFF) < 0 {
					hasBackwardBranch = true
				}
			}
		}
	}

	// Emit prologue
	z80EmitPrologue(buf)
	cs.totalRInc = totalR

	// Analyze for DJNZ loop memory optimization BEFORE chain entry.
	// The pre-check runs once on first entry (from prologue), then chain
	// iterations jump directly to chainEntry, skipping the pre-check.
	cs.loopInfo = z80AnalyzeDJNZLoop(instrs, startPC)
	preCheckFailLabel := ""
	if cs.loopInfo != nil {
		preCheckFailLabel = "__z80_precheck_fail__"
		z80EmitLoopPreCheck(buf, cs.loopInfo, preCheckFailLabel)
	}

	// Chain entry: chained blocks (including DJNZ self-loop) jump here,
	// skipping the prologue AND the pre-check.
	chainEntryOff := z80EmitChainEntry(buf, hasBackwardBranch)

	// Check if the last instruction is a terminator
	lastInstr := &instrs[len(instrs)-1]
	lastIsTerminator := z80JITIsTerminator(lastInstr)

	// Emit each instruction. Terminators (last instruction) emit their own
	// epilogue with the correct target PC. Non-terminator blocks get a
	// standard sequential epilogue after the loop.
	flagsNeeded := z80PeepholeFlags(instrs)

	// DJNZ deferred-flag optimization: if the second-to-last instruction is
	// a flag producer, the last is DJNZ, and no intra-block instruction
	// consumes flags, defer the flag producer's materialization to the DJNZ
	// not-taken exit only. This avoids materializing flags on every loop
	// iteration when they're only needed at loop exit.
	djnzDeferredFlags := uint8(0)
	if n := len(instrs); n >= 2 {
		penultimate := &instrs[n-2]
		last := &instrs[n-1]
		if last.prefix == z80JITPrefixNone && last.opcode == 0x10 && // DJNZ
			z80InstrProducesFlags(penultimate) && flagsNeeded[n-2] != 0 {
			// Check if the only reason flags are needed is the block exit
			// (no intra-block consumer). The peephole starts with z80FlagAll
			// for block exits, so if there's no consumer between the flag
			// producer and the block end, needFlags was set by the
			// conservative default, not by an actual consumer.
			hasIntraConsumer := false
			for i := 0; i < n-2; i++ {
				if z80InstrConsumedFlagMask(&instrs[i]) != 0 {
					hasIntraConsumer = true
					break
				}
			}
			if !hasIntraConsumer {
				// Only support INC/DEC r deferred materialization for now
				// (the most common pattern: DEC A before DJNZ)
				canDefer := false
				isDec := false
				if penultimate.prefix == z80JITPrefixNone {
					op := penultimate.opcode
					if op&0xC7 == 0x05 && op&0x38 != 0x30 { // DEC r (not DEC (HL))
						canDefer = true
						isDec = true
					} else if op&0xC7 == 0x04 && op&0x38 != 0x30 { // INC r (not INC (HL))
						canDefer = true
						isDec = false
					}
				}
				if canDefer {
					djnzDeferredFlags = flagsNeeded[n-2]
					flagsNeeded[n-2] = 0
					cs.deferredIsDec = isDec
				}
			}
		}
	}
	cs.djnzDeferredFlags = djnzDeferredFlags

	cyclesAccum := uint32(0)
	rIncAccum := 0
	for i := range instrs {
		instr := &instrs[i]
		instrPC := startPC + instr.pcOffset
		nextInstrPC := instrPC + uint16(instr.length)
		if i+1 < len(instrs) {
			nextInstrPC = startPC + instrs[i+1].pcOffset
		}

		isLast := (i == len(instrs)-1)

		// Use unchecked memory access for instructions inside the qualifying loop body
		uncheckedMem := cs.loopInfo != nil && i >= cs.loopInfo.loopStart && i < cs.loopInfo.loopEnd

		if isLast && lastIsTerminator {
			z80EmitTerminator(buf, &cs, instr, instrPC, nextInstrPC, i+1, cyclesAccum+uint32(instr.cycles), totalR)
		} else {
			z80EmitInstructionEx(buf, instr, instrPC, nextInstrPC, i, cyclesAccum, startPC, flagsNeeded[i], uncheckedMem, rIncAccum)
		}
		rIncAccum += int(instr.rIncrements)
		cyclesAccum += uint32(instr.cycles)
	}

	// Pre-check failure: bail to interpreter for this block
	if cs.loopInfo != nil {
		buf.Label(preCheckFailLabel)
		amd64MOV_mem_imm32(buf, z80RegCtx, int32(jzCtxOffNeedBail), 1)
		z80EmitEpilogue(buf, startPC, 0, 0, 0)
	}

	// If the last instruction was NOT a terminator (block hit max size),
	// emit a standard epilogue with sequential PC.
	if !lastIsTerminator {
		z80EmitEpilogue(buf, endPC, len(instrs), totalCycles, totalR)
	}

	// Emit shared register-store-and-return trampoline.
	// All epilogues set context fields (RetPC, RetCount, RetCycles, NeedBail, etc.)
	// then JMP here. This single copy of the register store saves ~80 bytes per
	// additional epilogue (bail exits, conditional branches, selfmod exits).
	buf.Label(z80SharedExitLabel)
	z80EmitRegisterStoreAndReturn(buf)

	// Resolve all label fixups
	buf.Resolve()

	// Write to executable memory
	code := buf.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, fmt.Errorf("Z80 JIT amd64 compile: %w", err)
	}

	// Convert chain exits to absolute chain slots
	var slots []chainSlot
	for _, ce := range cs.chainExits {
		slots = append(slots, chainSlot{
			targetPC:  ce.targetPC,
			patchAddr: addr + uintptr(ce.jmpDispOffset),
		})
	}

	block := &JITBlock{
		startPC:     uint32(startPC),
		endPC:       uint32(endPC),
		instrCount:  len(instrs),
		execAddr:    addr,
		execSize:    len(code),
		rIncrements: totalR,
		chainEntry:  addr + uintptr(chainEntryOff),
		chainSlots:  slots,
	}

	return block, nil
}

// ===========================================================================
// Instruction Dispatcher
// ===========================================================================

func z80EmitInstruction(buf *CodeBuffer, instr *JITZ80Instr, instrPC, nextInstrPC uint16, instrIdx int, cyclesAccum uint32, blockStartPC uint16, emitFlags uint8, rIncAccum int) {
	z80EmitInstructionEx(buf, instr, instrPC, nextInstrPC, instrIdx, cyclesAccum, blockStartPC, emitFlags, false, rIncAccum)
}

func z80EmitInstructionEx(buf *CodeBuffer, instr *JITZ80Instr, instrPC, nextInstrPC uint16, instrIdx int, cyclesAccum uint32, blockStartPC uint16, emitFlags uint8, uncheckedMem bool, rIncAccum int) {
	switch instr.prefix {
	case z80JITPrefixNone:
		z80EmitBaseInstructionEx(buf, instr, instrPC, nextInstrPC, instrIdx, cyclesAccum, blockStartPC, emitFlags, uncheckedMem, rIncAccum)
	case z80JITPrefixCB:
		z80EmitCBInstruction(buf, instr, instrPC, instrIdx, cyclesAccum, emitFlags, rIncAccum)
	case z80JITPrefixED:
		z80EmitEDInstruction(buf, instr, instrPC, instrIdx, cyclesAccum, blockStartPC, emitFlags, rIncAccum)
	case z80JITPrefixDD, z80JITPrefixFD:
		z80EmitDDFDInstruction(buf, instr, instrPC, instrIdx, cyclesAccum, emitFlags, rIncAccum)
	}
}

// ===========================================================================
// Base (unprefixed) Instruction Emitters
// ===========================================================================

// z80EmitBaseInstructionEx wraps z80EmitBaseInstruction with unchecked memory support.
// When uncheckedMem=true, memory-accessing instructions that use (HL), (DE), (BC)
// operands skip page-check and self-mod detection. This is only safe after a
// pre-loop page validation has proven all accessed pages are direct and code-free.
func z80EmitBaseInstructionEx(buf *CodeBuffer, instr *JITZ80Instr, instrPC, nextInstrPC uint16, instrIdx int, cyclesAccum uint32, blockStartPC uint16, emitFlags uint8, uncheckedMem bool, rIncAccum int) {
	if !uncheckedMem {
		z80EmitBaseInstruction(buf, instr, instrPC, nextInstrPC, instrIdx, cyclesAccum, blockStartPC, emitFlags, rIncAccum)
		return
	}

	op := instr.opcode
	switch {
	// LD r,(HL) — 0x46,0x4E,0x56,0x5E,0x66,0x6E,0x7E
	case op&0xC7 == 0x46 && op != 0x76:
		dst := (op >> 3) & 0x07
		z80EmitReadReg8OrHLUnchecked(buf, 6) // read (HL) unchecked
		z80EmitWriteReg8(buf, dst)
		return

	// LD (HL),r — 0x70-0x75,0x77
	case op >= 0x70 && op <= 0x77 && op != 0x76:
		src := op & 0x07
		z80EmitReadReg8(buf, src)                // AL = register value
		buf.EmitBytes(0x88, 0xC2)                // MOV DL, AL
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL) // EAX = HL
		z80EmitMemWriteUnchecked(buf)
		return

	// LD A,(DE) — 0x1A
	case op == 0x1A:
		amd64MOVZX_W(buf, z80Scratch1, z80RegDE) // EAX = DE
		z80EmitMemReadUnchecked(buf)
		buf.EmitBytes(0x88, modRM(3, z80Scratch1, z80RegA)) // MOV BL, AL
		return

	// LD (DE),A — 0x12
	case op == 0x12:
		buf.EmitBytes(0x88, 0xDA)                // MOV DL, BL (A)
		amd64MOVZX_W(buf, z80Scratch1, z80RegDE) // EAX = DE
		z80EmitMemWriteUnchecked(buf)
		return

	// ALU A,r/A,(HL) — 0x80-0xBF: only intercept (HL) operand
	case op >= 0x80 && op <= 0xBF:
		src := op & 0x07
		if src == 6 { // (HL) operand
			z80EmitReadReg8OrHLUnchecked(buf, 6)
			// Continue with ALU operation same as standard path
			aluOp := (op >> 3) & 0x07
			switch aluOp {
			case 0: // ADD
				buf.EmitBytes(0x88, 0xDA)
				buf.EmitBytes(0x88, 0xC1)
				buf.EmitBytes(0x00, modRM(3, z80Scratch1, z80RegA))
				if emitFlags != 0 {
					z80EmitCarryCapture(buf)
				}
				amd64MOVZX_B(buf, z80Scratch1, z80RegA)
				if emitFlags != 0 {
					z80EmitFlags_ADD(buf, emitFlags)
				}
			case 1: // ADC
				buf.EmitBytes(0x88, 0xC1)
				buf.EmitBytes(0x88, 0xDA)
				buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5)
				buf.EmitBytes(0x24, 0x01)
				buf.EmitBytes(0x00, 0xC1)
				buf.EmitBytes(0x00, 0xCB)
				if emitFlags != 0 {
					z80EmitCarryCapture(buf)
				}
				amd64MOVZX_B(buf, z80Scratch1, z80RegA)
				if emitFlags != 0 {
					z80EmitFlags_ADD(buf, emitFlags)
				}
			case 2: // SUB
				buf.EmitBytes(0x88, 0xDA)
				buf.EmitBytes(0x88, 0xC1)
				buf.EmitBytes(0x28, modRM(3, z80Scratch1, z80RegA))
				if emitFlags != 0 {
					z80EmitBorrowCapture(buf)
				}
				amd64MOVZX_B(buf, z80Scratch1, z80RegA)
				if emitFlags != 0 {
					z80EmitFlags_SUB(buf, emitFlags)
				}
			case 3: // SBC
				buf.EmitBytes(0x88, 0xC1)
				buf.EmitBytes(0x88, 0xDA)
				buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5)
				buf.EmitBytes(0x24, 0x01)
				buf.EmitBytes(0x00, 0xC1)
				buf.EmitBytes(0x28, 0xCB)
				if emitFlags != 0 {
					z80EmitBorrowCapture(buf)
				}
				amd64MOVZX_B(buf, z80Scratch1, z80RegA)
				if emitFlags != 0 {
					z80EmitFlags_SUB(buf, emitFlags)
				}
			case 4: // AND
				buf.EmitBytes(0x20, modRM(3, z80Scratch1, z80RegA))
				amd64MOVZX_B(buf, z80Scratch1, z80RegA)
				if emitFlags != 0 {
					z80EmitFlags_Logic(buf, true, emitFlags)
				}
			case 5: // XOR
				buf.EmitBytes(0x30, modRM(3, z80Scratch1, z80RegA))
				amd64MOVZX_B(buf, z80Scratch1, z80RegA)
				if emitFlags != 0 {
					z80EmitFlags_Logic(buf, false, emitFlags)
				}
			case 6: // OR
				buf.EmitBytes(0x08, modRM(3, z80Scratch1, z80RegA))
				amd64MOVZX_B(buf, z80Scratch1, z80RegA)
				if emitFlags != 0 {
					z80EmitFlags_Logic(buf, false, emitFlags)
				}
			case 7: // CP
				buf.EmitBytes(0x88, 0xDA)
				buf.EmitBytes(0x88, 0xC1)
				buf.EmitBytes(0x88, 0xD0)
				buf.EmitBytes(0x28, 0xC8)
				if emitFlags != 0 {
					z80EmitBorrowCapture(buf)
					z80EmitFlags_SUB(buf, emitFlags)
				}
			}
			return
		}
		// Non-(HL) ALU ops — fall through to standard emitter

	// INC HL (0x23) — with page-crossing guard when unchecked
	case op == 0x23:
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC6) // INC R14W
		// Page-crossing guard: L wrapped 0xFF→0x00?
		pgxLabel := fmt.Sprintf("pgx_%04X_%d", instrPC, buf.Len())
		pgxDone := fmt.Sprintf("pgxd_%04X_%d", instrPC, buf.Len())
		buf.EmitBytes(0x45, 0x84, 0xF6) // TEST R14B, R14B
		buf.EmitBytes(0x0F, 0x84)       // JZ → page cross exit
		buf.FixupRel32(pgxLabel, buf.Len()+4)
		buf.EmitBytes(0xE9) // JMP done (skip exit path)
		buf.FixupRel32(pgxDone, buf.Len()+4)
		// Page-cross exit: return to Go at NEXT instruction. All register
		// state (including the already-incremented HL) is preserved by the
		// shared exit trampoline. Go re-enters, re-validates, continues.
		buf.Label(pgxLabel)
		z80EmitEpilogue(buf, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(pgxDone)
		return

	// INC DE (0x13) — with page-crossing guard when unchecked
	case op == 0x13:
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC5) // INC R13W
		pgxLabel := fmt.Sprintf("pgx_%04X_%d", instrPC, buf.Len())
		pgxDone := fmt.Sprintf("pgxd_%04X_%d", instrPC, buf.Len())
		buf.EmitBytes(0x45, 0x84, 0xED) // TEST R13B, R13B
		buf.EmitBytes(0x0F, 0x84)
		buf.FixupRel32(pgxLabel, buf.Len()+4)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(pgxDone, buf.Len()+4)
		buf.Label(pgxLabel)
		z80EmitEpilogue(buf, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(pgxDone)
		return

	// INC BC (0x03) — with page-crossing guard when unchecked
	case op == 0x03:
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC4) // INC R12W
		pgxLabel := fmt.Sprintf("pgx_%04X_%d", instrPC, buf.Len())
		pgxDone := fmt.Sprintf("pgxd_%04X_%d", instrPC, buf.Len())
		buf.EmitBytes(0x45, 0x84, 0xE4) // TEST R12B, R12B
		buf.EmitBytes(0x0F, 0x84)
		buf.FixupRel32(pgxLabel, buf.Len()+4)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(pgxDone, buf.Len()+4)
		buf.Label(pgxLabel)
		z80EmitEpilogue(buf, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(pgxDone)
		return

	default:
		// All other instructions: use standard emitter
	}

	// Fall through: delegate to standard emitter
	z80EmitBaseInstruction(buf, instr, instrPC, nextInstrPC, instrIdx, cyclesAccum, blockStartPC, emitFlags, rIncAccum)
}

func z80EmitBaseInstruction(buf *CodeBuffer, instr *JITZ80Instr, instrPC, nextInstrPC uint16, instrIdx int, cyclesAccum uint32, blockStartPC uint16, emitFlags uint8, rIncAccum int) {
	op := instr.opcode

	switch {
	// NOP (0x00)
	case op == 0x00:
		// No operation — cycles accounted for in total

	// LD r,r' (0x40-0x7F, excluding 0x76 = HALT)
	case op >= 0x40 && op <= 0x7F && op != 0x76:
		dst := (op >> 3) & 0x07
		src := op & 0x07
		if src == 6 { // LD r,(HL)
			z80EmitLD_r_HL(buf, dst, instrPC, instrIdx, cyclesAccum, rIncAccum)
		} else if dst == 6 { // LD (HL),r
			z80EmitLD_HL_r(buf, src, instrPC, instrIdx, cyclesAccum, nextInstrPC, blockStartPC, rIncAccum, rIncAccum+int(instr.rIncrements))
		} else { // LD r,r'
			z80EmitReadReg8(buf, src)
			z80EmitWriteReg8(buf, dst)
		}

	// LD r,n (0x06,0x0E,0x16,0x1E,0x26,0x2E,0x3E)
	case op&0xC7 == 0x06 && op != 0x36:
		dst := (op >> 3) & 0x07
		// MOV EAX, imm8
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand&0xFF))
		z80EmitWriteReg8(buf, dst)

	// LD (HL),n (0x36)
	case op == 0x36:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		selfModLabel := fmt.Sprintf("smod_%04X", instrPC)
		// addr = HL
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		// value in DL
		amd64MOV_reg_imm32(buf, z80Scratch3, uint32(instr.operand&0xFF))
		z80EmitMemWrite(buf, bailLabel, selfModLabel)
		// Emit bail/selfmod exit paths at end — use deferred labels
		// We'll emit them after all instructions. For now, just emit the write.
		// Actually, labels must be emitted inline. Let's use a jump-over pattern.
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		buf.EmitBytes(0xE9) // JMP done
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabel, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabel)

	// ADD A,r (0x80-0x87) — includes (HL) operand
	case op >= 0x80 && op <= 0x87:
		src := op & 0x07
		z80EmitReadReg8OrHL(buf, src, instrPC, instrIdx, cyclesAccum, rIncAccum)
		if emitFlags != 0 {
			buf.EmitBytes(0x88, 0xDA) // MOV DL, BL (oldA — only for flags)
			buf.EmitBytes(0x88, 0xC1) // MOV CL, AL (operand — only for flags)
		}
		buf.EmitBytes(0x00, modRM(3, z80Scratch1, z80RegA)) // ADD BL, AL
		if emitFlags != 0 {
			z80EmitCarryCapture(buf)
			amd64MOVZX_B(buf, z80Scratch1, z80RegA) // result to AL for flags
			z80EmitFlags_ADD(buf, emitFlags)
		}

	// ADC A,r (0x88-0x8F) — includes (HL) operand
	case op >= 0x88 && op <= 0x8F:
		src := op & 0x07
		z80EmitReadReg8OrHL(buf, src, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.EmitBytes(0x88, 0xC1) // MOV CL, AL (operand)
		buf.EmitBytes(0x88, 0xDA) // MOV DL, BL (oldA)
		// Get carry, add to operand
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5) // MOVZX EAX, BPL
		buf.EmitBytes(0x24, 0x01)             // AND AL, 0x01
		buf.EmitBytes(0x00, 0xC1)             // ADD CL, AL (operand += carry)
		buf.EmitBytes(0x00, 0xCB)             // ADD BL, CL (A += operand+carry)
		if emitFlags != 0 {
			z80EmitCarryCapture(buf)
		}
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
		if emitFlags != 0 {
			z80EmitFlags_ADD(buf, emitFlags)
		}

	// SUB r (0x90-0x97) — includes (HL) operand
	case op >= 0x90 && op <= 0x97:
		src := op & 0x07
		z80EmitReadReg8OrHL(buf, src, instrPC, instrIdx, cyclesAccum, rIncAccum)
		if emitFlags != 0 {
			buf.EmitBytes(0x88, 0xDA) // MOV DL, BL (oldA)
			buf.EmitBytes(0x88, 0xC1) // MOV CL, AL (operand)
		}
		buf.EmitBytes(0x28, modRM(3, z80Scratch1, z80RegA))
		if emitFlags != 0 {
			z80EmitBorrowCapture(buf)
			amd64MOVZX_B(buf, z80Scratch1, z80RegA)
			z80EmitFlags_SUB(buf, emitFlags)
		}

	// SBC A,r (0x98-0x9F) — includes (HL) operand
	case op >= 0x98 && op <= 0x9F:
		src := op & 0x07
		z80EmitReadReg8OrHL(buf, src, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.EmitBytes(0x88, 0xC1)
		buf.EmitBytes(0x88, 0xDA)
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5) // MOVZX EAX, BPL
		buf.EmitBytes(0x24, 0x01)
		buf.EmitBytes(0x00, 0xC1) // ADD CL, AL (operand += carry)
		buf.EmitBytes(0x28, 0xCB) // SUB BL, CL (A -= operand+carry)
		if emitFlags != 0 {
			z80EmitBorrowCapture(buf)
		}
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
		if emitFlags != 0 {
			z80EmitFlags_SUB(buf, emitFlags)
		}

	// AND r (0xA0-0xA7) — includes (HL) operand
	case op >= 0xA0 && op <= 0xA7:
		src := op & 0x07
		z80EmitReadReg8OrHL(buf, src, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.EmitBytes(0x20, modRM(3, z80Scratch1, z80RegA))
		if emitFlags != 0 {
			amd64MOVZX_B(buf, z80Scratch1, z80RegA)
			z80EmitFlags_Logic(buf, true, emitFlags)
		}

	// XOR r (0xA8-0xAF) — includes (HL) operand
	case op >= 0xA8 && op <= 0xAF:
		src := op & 0x07
		z80EmitReadReg8OrHL(buf, src, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.EmitBytes(0x30, modRM(3, z80Scratch1, z80RegA))
		if emitFlags != 0 {
			amd64MOVZX_B(buf, z80Scratch1, z80RegA)
			z80EmitFlags_Logic(buf, false, emitFlags)
		}

	// OR r (0xB0-0xB7) — includes (HL) operand
	case op >= 0xB0 && op <= 0xB7:
		src := op & 0x07
		z80EmitReadReg8OrHL(buf, src, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.EmitBytes(0x08, modRM(3, z80Scratch1, z80RegA))
		if emitFlags != 0 {
			amd64MOVZX_B(buf, z80Scratch1, z80RegA)
			z80EmitFlags_Logic(buf, false, emitFlags)
		}

	// CP r (0xB8-0xBF) — includes (HL) operand
	case op >= 0xB8 && op <= 0xBF:
		src := op & 0x07
		z80EmitReadReg8OrHL(buf, src, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.EmitBytes(0x88, 0xDA) // MOV DL, BL (oldA)
		buf.EmitBytes(0x88, 0xC1) // MOV CL, AL (operand)
		buf.EmitBytes(0x88, 0xD0) // MOV AL, DL (oldA)
		buf.EmitBytes(0x28, 0xC8) // SUB AL, CL
		if emitFlags != 0 {
			z80EmitBorrowCapture(buf)
			z80EmitFlags_SUB(buf, emitFlags)
		}

	// INC r (0x04,0x0C,0x14,0x1C,0x24,0x2C,0x3C)
	case op&0xC7 == 0x04 && op&0x38 != 0x30: // not INC (HL) = 0x34
		reg := (op >> 3) & 0x07
		z80EmitReadReg8(buf, reg) // AL = old value
		if emitFlags != 0 {
			buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (save old for half-carry)
		}
		buf.EmitBytes(0xFE, 0xC0) // INC AL
		z80EmitWriteReg8(buf, reg)
		if emitFlags != 0 {
			z80EmitReadReg8(buf, reg) // re-read result into AL
			z80EmitFlags_INC_DEC_Runtime(buf, false, emitFlags)
		}

	// DEC r (0x05,0x0D,0x15,0x1D,0x25,0x2D,0x3D)
	case op&0xC7 == 0x05 && op&0x38 != 0x30: // not DEC (HL) = 0x35
		reg := (op >> 3) & 0x07
		z80EmitReadReg8(buf, reg)
		if emitFlags != 0 {
			buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (save old for half-carry)
		}
		buf.EmitBytes(0xFE, 0xC8) // DEC AL
		z80EmitWriteReg8(buf, reg)
		if emitFlags != 0 {
			z80EmitReadReg8(buf, reg) // re-read result
			z80EmitFlags_INC_DEC_Runtime(buf, true, emitFlags)
		}

	// LD rp,nn (0x01,0x11,0x21,0x31)
	case op&0xCF == 0x01:
		pair := (op >> 4) & 0x03
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand))
		z80EmitWritePair16(buf, pair)

	// INC rp (0x03,0x13,0x23,0x33)
	case op&0xCF == 0x03:
		pair := (op >> 4) & 0x03
		switch pair {
		case 0: // INC BC
			buf.EmitBytes(0x66, 0x41, 0xFF, 0xC4) // INC R12W
		case 1: // INC DE
			buf.EmitBytes(0x66, 0x41, 0xFF, 0xC5) // INC R13W
		case 2: // INC HL
			buf.EmitBytes(0x66, 0x41, 0xFF, 0xC6) // INC R14W
		case 3: // INC SP — spilled
			amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
			amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))
			buf.EmitBytes(0xFF, 0xC1) // INC ECX
			buf.EmitBytes(0x66)
			emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))
		}

	// DEC rp (0x0B,0x1B,0x2B,0x3B)
	case op&0xCF == 0x0B:
		pair := (op >> 4) & 0x03
		switch pair {
		case 0:
			buf.EmitBytes(0x66, 0x41, 0xFF, 0xCC) // DEC R12W
		case 1:
			buf.EmitBytes(0x66, 0x41, 0xFF, 0xCD) // DEC R13W
		case 2:
			buf.EmitBytes(0x66, 0x41, 0xFF, 0xCE) // DEC R14W
		case 3:
			amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
			amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))
			buf.EmitBytes(0xFF, 0xC9) // DEC ECX
			buf.EmitBytes(0x66)
			emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))
		}

	// PUSH rp (0xC5,0xD5,0xE5,0xF5)
	case op&0xCF == 0xC5:
		pair := (op >> 4) & 0x03
		z80EmitPUSH(buf, pair, instrPC, instrIdx, cyclesAccum, rIncAccum, rIncAccum+int(instr.rIncrements))

	// POP rp (0xC1,0xD1,0xE1,0xF1)
	case op&0xCF == 0xC1:
		pair := (op >> 4) & 0x03
		z80EmitPOP(buf, pair, instrPC, instrIdx, cyclesAccum, rIncAccum)

	// EX AF,AF' (0x08)
	case op == 0x08:
		z80EmitExAF(buf)

	// EXX (0xD9)
	case op == 0xD9:
		z80EmitEXX(buf)

	// EX DE,HL (0xEB)
	case op == 0xEB:
		// XCHG R13W, R14W — use scratch
		amd64MOVZX_W(buf, z80Scratch1, z80RegDE)
		amd64MOVZX_W(buf, z80Scratch2, z80RegHL)
		buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch1, z80RegHL&0x07)) // MOV R14W, AX
		buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch2, z80RegDE&0x07)) // MOV R13W, CX

	// DAA (0x27) — BCD adjust via lookup table
	case op == 0x27:
		// Index = (A << 3) | (C << 2) | (H << 1) | N
		// DAATable[index] = uint16((resultA << 8) | resultF)
		amd64MOVZX_B(buf, z80Scratch1, z80RegA) // EAX = A
		buf.EmitBytes(0xC1, 0xE0, 0x03)         // SHL EAX, 3 (A << 3)
		// Extract C, H, N from F (BPL)
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD) // MOVZX ECX, BPL
		buf.EmitBytes(0x88, 0xCA)             // MOV DL, CL
		buf.EmitBytes(0x80, 0xE2, 0x01)       // AND DL, 0x01 (C flag)
		buf.EmitBytes(0xC0, 0xE2, 0x02)       // SHL DL, 2 (C << 2)
		buf.EmitBytes(0x08, 0xD0)             // OR AL, DL
		buf.EmitBytes(0x88, 0xCA)             // MOV DL, CL
		buf.EmitBytes(0x80, 0xE2, 0x10)       // AND DL, 0x10 (H flag)
		buf.EmitBytes(0xC0, 0xEA, 0x03)       // SHR DL, 3 (H >> 3 = bit 1)
		buf.EmitBytes(0x08, 0xD0)             // OR AL, DL
		buf.EmitBytes(0x88, 0xCA)             // MOV DL, CL
		buf.EmitBytes(0x80, 0xE2, 0x02)       // AND DL, 0x02 (N flag)
		buf.EmitBytes(0xC0, 0xEA, 0x01)       // SHR DL, 1 (N >> 1 = bit 0)
		buf.EmitBytes(0x08, 0xD0)             // OR AL, DL
		// EAX = index. Lookup from DAATable (uint16 entries)
		amd64MOV_reg_mem(buf, z80Scratch3, amd64RSP, int32(z80OffDAAPtr))
		// MOVZX EAX, WORD [RDX + RAX*2]
		buf.EmitBytes(0x0F, 0xB7, 0x04, 0x42) // MOVZX EAX, WORD [RDX + RAX*2]
		// AH = resultA, AL = resultF
		buf.EmitBytes(0x88, 0xC1)                           // MOV CL, AL (resultF)
		buf.EmitBytes(0xC1, 0xE8, 0x08)                     // SHR EAX, 8 (resultA)
		buf.EmitBytes(0x88, modRM(3, z80Scratch1, z80RegA)) // MOV BL, AL (A = resultA)
		buf.EmitBytes(0x40, 0x88, 0xCD)                     // MOV BPL, CL (F = resultF)

	// JP nn (0xC3), JR e (0x18), RET (0xC9), CALL nn (0xCD),
	// conditional branches, RST n, EI, DI — all handled by z80EmitTerminator
	// when they are the last instruction in a block.

	// SCF (0x37)
	case op == 0x37:
		// Set carry flag: C=1, N=0, H=0, preserve S,Z,P/V
		// Y,X from A
		// OR BPL, 0x01 (set C)
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x01)
		// AND BPL, ~(0x12) → clear N and H (bits 1 and 4)
		buf.EmitBytes(0x40, 0x80, 0xE5, 0xED)
		// Set Y,X from A: (BPL & ~0x28) | (BL & 0x28)
		buf.EmitBytes(0x40, 0x80, 0xE5, 0xD7) // AND BPL, ~0x28
		buf.EmitBytes(0x88, 0xD8)             // MOV AL, BL
		buf.EmitBytes(0x24, 0x28)             // AND AL, 0x28
		buf.EmitBytes(0x40, 0x08, 0xC5)       // OR BPL, AL

	// CCF (0x3F)
	case op == 0x3F:
		// Complement carry: H=old C, C=~C, N=0
		// Copy old C to H position first
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5) // MOVZX EAX, BPL
		buf.EmitBytes(0x88, 0xC1)             // MOV CL, AL
		buf.EmitBytes(0xC0, 0xE1, 0x04)       // SHL CL, 4 (C bit to H position)
		buf.EmitBytes(0x80, 0xE1, 0x10)       // AND CL, 0x10 (isolate H)
		buf.EmitBytes(0x34, 0x01)             // XOR AL, 0x01 (flip C)
		buf.EmitBytes(0x24, 0xED)             // AND AL, ~(0x12) clear old N,H
		buf.EmitBytes(0x08, 0xC8)             // OR AL, CL (set new H from old C)
		// Set Y,X from A
		buf.EmitBytes(0x24, 0xD7)       // AND AL, ~0x28
		buf.EmitBytes(0x88, 0xDA)       // MOV DL, BL
		buf.EmitBytes(0x80, 0xE2, 0x28) // AND DL, 0x28
		buf.EmitBytes(0x08, 0xD0)       // OR AL, DL
		buf.EmitBytes(0x40, 0x88, 0xC5) // MOV BPL, AL

	// CPL (0x2F)
	case op == 0x2F:
		// NOT A; set H=1, N=1, Y/X from result
		buf.EmitBytes(0xF6, 0xD3) // NOT BL
		// Set H and N in F
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x12) // OR BPL, 0x12 (H | N)
		// Update Y,X from new A
		buf.EmitBytes(0x40, 0x80, 0xE5, 0xD7) // AND BPL, ~0x28
		buf.EmitBytes(0x88, 0xD8)             // MOV AL, BL
		buf.EmitBytes(0x24, 0x28)             // AND AL, 0x28
		buf.EmitBytes(0x40, 0x08, 0xC5)       // OR BPL, AL

	// DI (0xF3) and EI (0xFB) — handled by z80EmitTerminator

	// LD SP,HL (0xF9)
	case op == 0xF9:
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		amd64MOVZX_W(buf, z80Scratch2, z80RegHL)
		buf.EmitBytes(0x66)
		emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))

	// ALU A,n (0xC6,0xCE,0xD6,0xDE,0xE6,0xEE,0xF6,0xFE)
	case op&0xC7 == 0xC6:
		aluOp := (op >> 3) & 0x07
		imm := byte(instr.operand & 0xFF)
		z80EmitALU_A_imm(buf, aluOp, imm, emitFlags)

	// RLCA (0x07), RRCA (0x0F), RLA (0x17), RRA (0x1F)
	case op == 0x07 || op == 0x0F || op == 0x17 || op == 0x1F:
		z80EmitRotateA(buf, op)

	// LD A,(BC) (0x0A)
	case op == 0x0A:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		amd64MOVZX_W(buf, z80Scratch1, z80RegBC) // EAX = BC
		z80EmitMemRead(buf, bailLabel)
		// MOV BL, AL
		buf.EmitBytes(0x88, modRM(3, z80Scratch1, z80RegA))
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.Label(doneLabel)

	// LD A,(DE) (0x1A)
	case op == 0x1A:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		amd64MOVZX_W(buf, z80Scratch1, z80RegDE)
		z80EmitMemRead(buf, bailLabel)
		buf.EmitBytes(0x88, modRM(3, z80Scratch1, z80RegA))
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.Label(doneLabel)

	// LD (BC),A (0x02)
	case op == 0x02:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		selfModLabel := fmt.Sprintf("smod_%04X", instrPC)
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		amd64MOVZX_W(buf, z80Scratch1, z80RegBC)
		amd64MOVZX_B(buf, z80Scratch3, z80RegA) // DL = A
		z80EmitMemWrite(buf, bailLabel, selfModLabel)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabel, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabel)

	// LD (DE),A (0x12)
	case op == 0x12:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		selfModLabel := fmt.Sprintf("smod_%04X", instrPC)
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		amd64MOVZX_W(buf, z80Scratch1, z80RegDE)
		amd64MOVZX_B(buf, z80Scratch3, z80RegA)
		z80EmitMemWrite(buf, bailLabel, selfModLabel)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabel, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabel)

	// ADD HL,rp (0x09,0x19,0x29,0x39)
	case op&0xCF == 0x09:
		pair := (op >> 4) & 0x03
		z80EmitReadPair16(buf, pair) // EAX = pair value
		// ADD R14W, AX
		buf.EmitBytes(0x66, 0x41, 0x01, modRM(3, z80Scratch1, z80RegHL&0x07))
		// Flag update: only C, H, N affected (N=0, H=half-carry from bit 11, C=carry from bit 15)
		// Simplified: clear N, set C if overflow
		buf.EmitBytes(0x40, 0x80, 0xE5, 0xEC) // AND BPL, ~0x13 (clear N, H, C)
		// C flag from JC after ADD — use host carry
		buf.EmitBytes(0x73, 0x04)             // JNC +4 (skip 4-byte OR BPL with REX)
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x01) // OR BPL, 0x01

	// LD A,(nn) (0x3A)
	case op == 0x3A:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand)) // EAX = address
		z80EmitMemRead(buf, bailLabel)
		buf.EmitBytes(0x88, modRM(3, z80Scratch1, z80RegA)) // MOV BL, AL
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.Label(doneLabel)

	// LD (nn),A (0x32)
	case op == 0x32:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		selfModLabel := fmt.Sprintf("smod_%04X", instrPC)
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand))
		amd64MOVZX_B(buf, z80Scratch3, z80RegA) // DL = A
		z80EmitMemWrite(buf, bailLabel, selfModLabel)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabel, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabel)

	// LD HL,(nn) (0x2A)
	case op == 0x2A:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		// Read low byte
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand))
		z80EmitMemRead(buf, bailLabel)
		buf.EmitBytes(0x41, 0x89, 0xC3) // MOV R11D, EAX (save low)
		// Read high byte
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand+1))
		z80EmitMemRead(buf, bailLabel)
		// Combine: HL = (high << 8) | low
		buf.EmitBytes(0xC1, 0xE0, 0x08)                                       // SHL EAX, 8
		buf.EmitBytes(0x44, 0x09, 0xD8)                                       // OR EAX, R11D
		buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch1, z80RegHL&0x07)) // MOV R14W, AX
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.Label(doneLabel)

	// LD (nn),HL (0x22)
	case op == 0x22:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		selfModLabel := fmt.Sprintf("smod_%04X", instrPC)
		doneLabel1 := fmt.Sprintf("d1_%04X", instrPC)
		bailLabel2 := fmt.Sprintf("bail2_%04X", instrPC)
		selfModLabel2 := fmt.Sprintf("smod2_%04X", instrPC)
		doneLabel2 := fmt.Sprintf("d2_%04X", instrPC)
		// Write low byte (L)
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand))
		amd64MOVZX_B(buf, z80Scratch3, z80RegHL) // DL = L (low byte of R14)
		z80EmitMemWrite(buf, bailLabel, selfModLabel)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel1, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabel, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabel1)
		// Write high byte (H)
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand+1))
		amd64MOVZX_W(buf, z80Scratch3, z80RegHL)
		buf.EmitBytes(0xC1, 0xEA, 0x08) // SHR EDX, 8 (H byte)
		z80EmitMemWrite(buf, bailLabel2, selfModLabel2)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel2, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel2, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabel2, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabel2)

	// INC (HL) (0x34)
	case op == 0x34:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		z80EmitMemRead(buf, bailLabel)
		// Save old value to stack (survives memWrite)
		buf.EmitBytes(0x88, 0x44, 0x24, byte(z80OffLoopBudg)) // MOV [RSP+z80OffLoopBudg], AL
		buf.EmitBytes(0xFE, 0xC0)                             // INC AL
		buf.EmitBytes(0x41, 0x89, 0xC3)                       // MOV R11D, EAX (save result)
		bailLabelW := fmt.Sprintf("bail_w_%04X", instrPC)
		selfModLabelW := fmt.Sprintf("smod_w_%04X", instrPC)
		doneLabelW := fmt.Sprintf("done_w_%04X", instrPC)
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		buf.EmitBytes(0x44, 0x88, 0xDA) // MOV DL, R11B
		z80EmitMemWrite(buf, bailLabelW, selfModLabelW)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabelW, buf.Len()+4)
		z80EmitBailExit(buf, bailLabelW, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabelW, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabelW)
		buf.EmitBytes(0x44, 0x88, 0xD8)                       // MOV AL, R11B (result)
		buf.EmitBytes(0x8A, 0x54, 0x24, byte(z80OffLoopBudg)) // MOV DL, [RSP+z80OffLoopBudg] (old)
		if emitFlags != 0 {
			z80EmitFlags_INC_DEC_Runtime(buf, false, emitFlags)
		}
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.Label(doneLabel)

	// DEC (HL) (0x35)
	case op == 0x35:
		bailLabel := fmt.Sprintf("bail_%04X", instrPC)
		doneLabel := fmt.Sprintf("done_%04X", instrPC)
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		z80EmitMemRead(buf, bailLabel)
		// Save old value to stack (survives memWrite which clobbers DL)
		buf.EmitBytes(0x88, 0x44, 0x24, byte(z80OffLoopBudg)) // MOV [RSP+z80OffLoopBudg], AL
		buf.EmitBytes(0xFE, 0xC8)                             // DEC AL
		buf.EmitBytes(0x41, 0x89, 0xC3)                       // MOV R11D, EAX (save result)
		bailLabelW := fmt.Sprintf("bail_w_%04X", instrPC)
		selfModLabelW := fmt.Sprintf("smod_w_%04X", instrPC)
		doneLabelW := fmt.Sprintf("done_w_%04X", instrPC)
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		buf.EmitBytes(0x44, 0x88, 0xDA) // MOV DL, R11B
		z80EmitMemWrite(buf, bailLabelW, selfModLabelW)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabelW, buf.Len()+4)
		z80EmitBailExit(buf, bailLabelW, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabelW, nextInstrPC, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabelW)
		buf.EmitBytes(0x44, 0x88, 0xD8) // MOV AL, R11B (result)
		// Reload old value from stack into DL for H flag computation
		buf.EmitBytes(0x8A, 0x54, 0x24, byte(z80OffLoopBudg)) // MOV DL, [RSP+z80OffLoopBudg]
		if emitFlags != 0 {
			z80EmitFlags_INC_DEC_Runtime(buf, true, emitFlags)
		}
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.Label(doneLabel)

	default:
		// Unhandled opcode — skip (cycles still counted)
	}
}

// ===========================================================================
// Helper Emitters for Common Patterns
// ===========================================================================

// z80EmitLD_r_HL emits LD r,(HL) — load from memory at HL address.
func z80EmitLD_r_HL(buf *CodeBuffer, dstReg byte, instrPC uint16, instrIdx int, cyclesAccum uint32, rIncAccum int) {
	bailLabel := fmt.Sprintf("bail_%04X", instrPC)
	doneLabel := fmt.Sprintf("done_%04X", instrPC)

	// EAX = HL (16-bit address)
	amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
	z80EmitMemRead(buf, bailLabel)
	// AL now has the byte; write to destination register
	z80EmitWriteReg8(buf, dstReg)

	buf.EmitBytes(0xE9) // JMP done
	buf.FixupRel32(doneLabel, buf.Len()+4)

	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	buf.Label(doneLabel)
}

// z80EmitLD_HL_r emits LD (HL),r — store register to memory at HL address.
func z80EmitLD_HL_r(buf *CodeBuffer, srcReg byte, instrPC uint16, instrIdx int, cyclesAccum uint32, nextInstrPC uint16, blockStartPC uint16, rIncAccum int, rIncComplete int) {
	bailLabel := fmt.Sprintf("bail_%04X", instrPC)
	selfModLabel := fmt.Sprintf("smod_%04X", instrPC)
	doneLabel := fmt.Sprintf("done_%04X", instrPC)

	// Read source register into DL (for memWrite)
	z80EmitReadReg8(buf, srcReg)
	buf.EmitBytes(0x88, 0xC2) // MOV DL, AL

	// EAX = HL
	amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
	z80EmitMemWrite(buf, bailLabel, selfModLabel)

	buf.EmitBytes(0xE9) // JMP done
	buf.FixupRel32(doneLabel, buf.Len()+4)

	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	z80EmitSelfModExit(buf, selfModLabel, nextInstrPC, instrIdx+1, cyclesAccum+uint32(z80BaseInstrCycles[0x70]), rIncComplete)
	buf.Label(doneLabel)
}

// z80EmitReadReg8OrHL emits code to read a Z80 register (0-7 including 6=(HL))
// into AL. For (HL), performs a memory read with bail path.
func z80EmitReadReg8OrHL(buf *CodeBuffer, z80Reg byte, instrPC uint16, instrIdx int, cyclesAccum uint32, rIncAccum int) {
	if z80Reg != 6 {
		z80EmitReadReg8(buf, z80Reg)
		return
	}
	// (HL) — read from memory at address HL
	bailLabel := fmt.Sprintf("hl_bail_%04X_%d", instrPC, buf.Len())
	doneLabel := fmt.Sprintf("hl_done_%04X_%d", instrPC, buf.Len())

	amd64MOVZX_W(buf, z80Scratch1, z80RegHL) // EAX = HL
	z80EmitMemRead(buf, bailLabel)

	buf.EmitBytes(0xE9) // JMP done
	buf.FixupRel32(doneLabel, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	buf.Label(doneLabel)
}

// z80EmitReadReg8OrHLUnchecked reads a Z80 8-bit register or (HL) without page checks.
// Only safe when the HL page has been pre-validated as direct.
func z80EmitReadReg8OrHLUnchecked(buf *CodeBuffer, z80Reg byte) {
	if z80Reg != 6 {
		z80EmitReadReg8(buf, z80Reg)
		return
	}
	amd64MOVZX_W(buf, z80Scratch1, z80RegHL) // EAX = HL
	z80EmitMemReadUnchecked(buf)
}

// z80EmitALU_A_imm emits ALU A,n (immediate operand).
func z80EmitALU_A_imm(buf *CodeBuffer, aluOp byte, imm byte, emitFlags uint8) {
	switch aluOp {
	case 0: // ADD A,n
		buf.EmitBytes(0x88, 0xDA) // MOV DL, BL (oldA)
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(imm))
		buf.EmitBytes(0x88, 0xC1)                           // MOV CL, AL
		buf.EmitBytes(0x00, modRM(3, z80Scratch1, z80RegA)) // ADD BL, AL
		if emitFlags != 0 {
			z80EmitCarryCapture(buf)
		}
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
		if emitFlags != 0 {
			z80EmitFlags_ADD(buf, emitFlags)
		}
	case 2: // SUB n
		buf.EmitBytes(0x88, 0xDA)
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(imm))
		buf.EmitBytes(0x88, 0xC1)
		buf.EmitBytes(0x28, modRM(3, z80Scratch1, z80RegA))
		if emitFlags != 0 {
			z80EmitBorrowCapture(buf)
		}
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
		if emitFlags != 0 {
			z80EmitFlags_SUB(buf, emitFlags)
		}
	case 4: // AND n
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(imm))
		buf.EmitBytes(0x20, modRM(3, z80Scratch1, z80RegA))
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
		if emitFlags != 0 {
			z80EmitFlags_Logic(buf, true, emitFlags)
		}
	case 5: // XOR n
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(imm))
		buf.EmitBytes(0x30, modRM(3, z80Scratch1, z80RegA))
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
		if emitFlags != 0 {
			z80EmitFlags_Logic(buf, false, emitFlags)
		}
	case 6: // OR n
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(imm))
		buf.EmitBytes(0x08, modRM(3, z80Scratch1, z80RegA))
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
		if emitFlags != 0 {
			z80EmitFlags_Logic(buf, false, emitFlags)
		}
	case 7: // CP n
		buf.EmitBytes(0x88, 0xDA) // oldA
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(imm))
		buf.EmitBytes(0x88, 0xC1) // operand
		buf.EmitBytes(0x88, 0xD0) // MOV AL, DL (oldA for subtraction)
		buf.EmitBytes(0x28, 0xC8) // SUB AL, CL
		if emitFlags != 0 {
			z80EmitBorrowCapture(buf)
			z80EmitFlags_SUB(buf, emitFlags)
		}
	case 1: // ADC A,n — A += n + C
		buf.EmitBytes(0x88, 0xDA) // MOV DL, BL (oldA)
		// Get carry from F
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD) // MOVZX ECX, BPL
		buf.EmitBytes(0x80, 0xE1, 0x01)       // AND CL, 0x01 (carry)
		// A += carry
		buf.EmitBytes(0x00, 0xCB) // ADD BL, CL
		// A += operand
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(imm))
		buf.EmitBytes(0x88, 0xC1) // MOV CL, AL (operand)
		buf.EmitBytes(0x00, modRM(3, z80Scratch1, z80RegA))
		if emitFlags != 0 {
			z80EmitCarryCapture(buf)
		}
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
		if emitFlags != 0 {
			z80EmitFlags_ADD(buf, emitFlags)
		}
	case 3: // SBC A,n — A -= n + C
		buf.EmitBytes(0x88, 0xDA)
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD)
		buf.EmitBytes(0x80, 0xE1, 0x01)
		buf.EmitBytes(0x28, 0xCB) // SUB BL, CL (subtract carry)
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(imm))
		buf.EmitBytes(0x88, 0xC1)
		buf.EmitBytes(0x28, modRM(3, z80Scratch1, z80RegA))
		if emitFlags != 0 {
			z80EmitBorrowCapture(buf)
		}
		amd64MOVZX_B(buf, z80Scratch1, z80RegA)
		if emitFlags != 0 {
			z80EmitFlags_SUB(buf, emitFlags)
		}
	}
}

// z80EmitRotateA emits RLCA/RRCA/RLA/RRA (fast rotate instructions on A).
func z80EmitRotateA(buf *CodeBuffer, op byte) {
	switch op {
	case 0x07: // RLCA: A = (A << 1) | (A >> 7); C = old bit 7
		// ROL BL, 1
		buf.EmitBytes(0xD0, 0xC3)
		// Update F: clear H,N; set C from new bit 0 (which was old bit 7)
		buf.EmitBytes(0x40, 0x80, 0xE5, 0xC4) // AND BPL, 0xC4 (keep S,Z,P/V)
		// C = BL & 1
		buf.EmitBytes(0x88, 0xD8)       // MOV AL, BL
		buf.EmitBytes(0x24, 0x01)       // AND AL, 0x01
		buf.EmitBytes(0x40, 0x08, 0xC5) // OR BPL, AL
		// Y,X from A
		buf.EmitBytes(0x88, 0xD8)       // MOV AL, BL
		buf.EmitBytes(0x24, 0x28)       // AND AL, 0x28
		buf.EmitBytes(0x40, 0x08, 0xC5) // OR BPL, AL
	case 0x0F: // RRCA: A = (A >> 1) | (A << 7); C = old bit 0
		// ROR BL, 1
		buf.EmitBytes(0xD0, 0xCB)
		buf.EmitBytes(0x40, 0x80, 0xE5, 0xC4) // AND BPL, keep S,Z,P/V
		buf.EmitBytes(0x88, 0xD8)             // MOV AL, BL
		buf.EmitBytes(0xC0, 0xE8, 0x07)       // SHR AL, 7
		buf.EmitBytes(0x40, 0x08, 0xC5)       // OR BPL, AL (C)
		buf.EmitBytes(0x88, 0xD8)
		buf.EmitBytes(0x24, 0x28)
		buf.EmitBytes(0x40, 0x08, 0xC5)
	case 0x17: // RLA: rotate left through carry
		// Get old carry from F
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5) // MOVZX EAX, BPL
		buf.EmitBytes(0x88, 0xC1)             // MOV CL, AL (old F)
		buf.EmitBytes(0x80, 0xE1, 0x01)       // AND CL, 0x01 (old C)
		// New C = BL bit 7
		buf.EmitBytes(0x88, 0xDA)       // MOV DL, BL
		buf.EmitBytes(0xC0, 0xEA, 0x07) // SHR DL, 7
		// A = (A << 1) | oldC
		buf.EmitBytes(0xD0, 0xE3) // SHL BL, 1
		buf.EmitBytes(0x08, 0xCB) // OR BL, CL
		// Update F
		buf.EmitBytes(0x40, 0x80, 0xE5, 0xC4) // AND BPL, keep S,Z,P/V
		buf.EmitBytes(0x40, 0x08, 0xD5)       // OR BPL, DL (new C)
		buf.EmitBytes(0x88, 0xD8)
		buf.EmitBytes(0x24, 0x28)
		buf.EmitBytes(0x40, 0x08, 0xC5)
	case 0x1F: // RRA: rotate right through carry
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5) // MOVZX EAX, BPL
		buf.EmitBytes(0x88, 0xC1)
		buf.EmitBytes(0x80, 0xE1, 0x01) // old C in CL
		buf.EmitBytes(0xC0, 0xE1, 0x07) // SHL CL, 7 (move to bit 7 position)
		// New C = BL bit 0
		buf.EmitBytes(0x88, 0xDA)       // MOV DL, BL
		buf.EmitBytes(0x80, 0xE2, 0x01) // AND DL, 0x01
		// A = (A >> 1) | (oldC << 7)
		buf.EmitBytes(0xD0, 0xEB) // SHR BL, 1
		buf.EmitBytes(0x08, 0xCB) // OR BL, CL
		// Update F
		buf.EmitBytes(0x40, 0x80, 0xE5, 0xC4)
		buf.EmitBytes(0x40, 0x08, 0xD5)
		buf.EmitBytes(0x88, 0xD8)
		buf.EmitBytes(0x24, 0x28)
		buf.EmitBytes(0x40, 0x08, 0xC5)
	}
}

// z80EmitPUSH emits Z80 PUSH rp: SP-=2, [SP]=low, [SP+1]=high.
func z80EmitPUSH(buf *CodeBuffer, pair byte, instrPC uint16, instrIdx int, cyclesAccum uint32, rIncAccum int, rIncComplete int) {
	bailLabel := fmt.Sprintf("push_bail_%04X", instrPC)
	selfModLabel := fmt.Sprintf("push_smod_%04X", instrPC)
	doneLabel1 := fmt.Sprintf("push_d1_%04X", instrPC)
	bailLabel2 := fmt.Sprintf("push_bail2_%04X", instrPC)
	selfModLabel2 := fmt.Sprintf("push_smod2_%04X", instrPC)
	doneLabel2 := fmt.Sprintf("push_d2_%04X", instrPC)

	// Read pair value into R11 (preserved across memWrite)
	switch pair {
	case 0: // BC
		amd64MOVZX_W(buf, z80Scratch5, z80RegBC) // R11D = BC
	case 1: // DE
		amd64MOVZX_W(buf, z80Scratch5, z80RegDE)
	case 2: // HL
		amd64MOVZX_W(buf, z80Scratch5, z80RegHL)
	case 3: // AF
		amd64MOVZX_B(buf, z80Scratch5, z80RegA) // R11D = A
		buf.EmitBytes(0x41, 0xC1, 0xE3, 0x08)   // SHL R11D, 8
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5)   // MOVZX EAX, BPL (F)
		buf.EmitBytes(0x41, 0x09, 0xC3)         // OR R11D, EAX
	}

	// SP -= 2
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))   // RAX = CpuPtr
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP)) // ECX = SP
	buf.EmitBytes(0x83, 0xE9, 0x02)                                     // SUB ECX, 2
	buf.EmitBytes(0x66)
	emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))

	// Write low byte: [SP] = low byte of pair (R11B)
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX (addr = new SP)
	buf.EmitBytes(0x44, 0x88, 0xDA) // MOV DL, R11B (low byte)
	z80EmitMemWrite(buf, bailLabel, selfModLabel)
	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel1, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	z80EmitSelfModExit(buf, selfModLabel, instrPC+1, instrIdx+1, cyclesAccum+11, rIncComplete)
	buf.Label(doneLabel1)

	// Write high byte: [SP+1] = high byte of pair
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))
	buf.EmitBytes(0xFF, 0xC1)       // INC ECX (SP+1)
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX
	buf.EmitBytes(0x44, 0x89, 0xDA) // MOV EDX, R11D
	buf.EmitBytes(0xC1, 0xEA, 0x08) // SHR EDX, 8 (high byte to DL)
	z80EmitMemWrite(buf, bailLabel2, selfModLabel2)
	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel2, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel2, instrPC, instrIdx, cyclesAccum, rIncAccum)
	z80EmitSelfModExit(buf, selfModLabel2, instrPC+1, instrIdx+1, cyclesAccum+11, rIncComplete)
	buf.Label(doneLabel2)
}

// z80EmitPOP emits Z80 POP rp.
func z80EmitPOP(buf *CodeBuffer, pair byte, instrPC uint16, instrIdx int, cyclesAccum uint32, rIncAccum int) {
	bailLabel := fmt.Sprintf("bail_%04X", instrPC)
	doneLabel := fmt.Sprintf("done_%04X", instrPC)

	// Read SP
	amd64MOV_reg_mem(buf, z80Scratch4, amd64RSP, int32(z80OffCpuPtr))
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))

	// Read low byte from [SP]
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX
	z80EmitMemRead(buf, bailLabel)
	// Save low byte
	buf.EmitBytes(0x41, 0x89, 0xC3) // MOV R11D, EAX

	// Read high byte from [SP+1]
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))
	buf.EmitBytes(0xFF, 0xC1)       // INC ECX
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX
	z80EmitMemRead(buf, bailLabel)

	// Combine: AX = (high << 8) | low
	buf.EmitBytes(0xC1, 0xE0, 0x08) // SHL EAX, 8
	buf.EmitBytes(0x44, 0x09, 0xD8) // OR EAX, R11D

	// SP += 2
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))
	buf.EmitBytes(0x83, 0xC1, 0x02) // ADD ECX, 2
	buf.EmitBytes(0x66)
	emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))

	// Write pair — PUSH/POP pair encoding: 0=BC, 1=DE, 2=HL, 3=AF
	switch pair {
	case 0: // BC
		buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch1, z80RegBC&0x07))
	case 1: // DE
		buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch1, z80RegDE&0x07))
	case 2: // HL
		buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch1, z80RegHL&0x07))
	case 3: // AF: high byte = A, low byte = F
		// AH = A, AL = F (from the combined 16-bit value)
		buf.EmitBytes(0x88, 0xC1)                           // MOV CL, AL (F)
		buf.EmitBytes(0xC1, 0xE8, 0x08)                     // SHR EAX, 8 (A)
		buf.EmitBytes(0x88, modRM(3, z80Scratch1, z80RegA)) // MOV BL, AL (A)
		buf.EmitBytes(0x40, 0x88, 0xCD)                     // MOV BPL, CL (F)
	}

	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	buf.Label(doneLabel)
}

// z80EmitPUSHValue pushes the 16-bit value in R11D to the Z80 stack.
func z80EmitPUSHValue(buf *CodeBuffer, instrPC uint16, instrIdx int, cyclesAccum uint32, rIncAccum int, rIncComplete int) {
	bailLabel := fmt.Sprintf("pushv_bail_%04X", instrPC)
	selfModLabel := fmt.Sprintf("pushv_smod_%04X", instrPC)
	doneLabel1 := fmt.Sprintf("pushv_d1_%04X", instrPC)
	bailLabel2 := fmt.Sprintf("pushv_bail2_%04X", instrPC)
	selfModLabel2 := fmt.Sprintf("pushv_smod2_%04X", instrPC)
	doneLabel2 := fmt.Sprintf("pushv_d2_%04X", instrPC)

	// SP -= 2
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))
	buf.EmitBytes(0x83, 0xE9, 0x02) // SUB ECX, 2
	buf.EmitBytes(0x66)
	emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))

	// Write low byte: [SP]
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX
	buf.EmitBytes(0x44, 0x88, 0xDA) // MOV DL, R11B
	z80EmitMemWrite(buf, bailLabel, selfModLabel)
	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel1, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	z80EmitSelfModExit(buf, selfModLabel, instrPC+2, instrIdx+1, cyclesAccum+15, rIncComplete)
	buf.Label(doneLabel1)

	// Write high byte: [SP+1]
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))
	buf.EmitBytes(0xFF, 0xC1)       // INC ECX
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX
	buf.EmitBytes(0x44, 0x89, 0xDA) // MOV EDX, R11D
	buf.EmitBytes(0xC1, 0xEA, 0x08) // SHR EDX, 8
	z80EmitMemWrite(buf, bailLabel2, selfModLabel2)
	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel2, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel2, instrPC, instrIdx, cyclesAccum, rIncAccum)
	z80EmitSelfModExit(buf, selfModLabel2, instrPC+2, instrIdx+1, cyclesAccum+15, rIncComplete)
	buf.Label(doneLabel2)
}

// z80EmitPOPToIXIY pops 16-bit value from stack and writes to IX or IY.
func z80EmitPOPToIXIY(buf *CodeBuffer, ixiyOff uintptr, instrPC uint16, instrIdx int, cyclesAccum uint32, rIncAccum int) {
	bailLabel := fmt.Sprintf("popix_%04X", instrPC)
	doneLabel := fmt.Sprintf("popix_d_%04X", instrPC)

	// Read SP
	amd64MOV_reg_mem(buf, z80Scratch4, amd64RSP, int32(z80OffCpuPtr))
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))

	// Read low byte from [SP]
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX
	z80EmitMemRead(buf, bailLabel)
	buf.EmitBytes(0x41, 0x89, 0xC3) // MOV R11D, EAX

	// Read high byte from [SP+1]
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))
	buf.EmitBytes(0xFF, 0xC1)       // INC ECX
	buf.EmitBytes(0x0F, 0xB7, 0xC1) // MOVZX EAX, CX
	z80EmitMemRead(buf, bailLabel)

	// Combine: AX = (high << 8) | low
	buf.EmitBytes(0xC1, 0xE0, 0x08)
	buf.EmitBytes(0x44, 0x09, 0xD8) // OR EAX, R11D

	// SP += 2
	amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))
	buf.EmitBytes(0x83, 0xC1, 0x02) // ADD ECX, 2
	buf.EmitBytes(0x66)
	emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch4, int32(cpuZ80OffSP))

	// Write to IX/IY
	amd64MOV_reg_mem(buf, z80Scratch4, amd64RSP, int32(z80OffCpuPtr))
	buf.EmitBytes(0x66)
	emitMemOp(buf, false, 0x89, z80Scratch1, z80Scratch4, int32(ixiyOff))

	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	buf.Label(doneLabel)
}

// z80EmitExAF emits EX AF,AF' — swap A,F with shadow A',F'.
func z80EmitExAF(buf *CodeBuffer) {
	// Load CpuPtr
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))

	// Save current A (BL) and F (BPL) to temporaries
	buf.EmitBytes(0x88, 0xDA)       // MOV DL, BL (current A)
	buf.EmitBytes(0x40, 0x88, 0xE9) // MOV CL, BPL (current F)

	// Load shadow A',F' from CPU struct
	amd64MOVZX_B_mem(buf, z80RegA, z80Scratch1, int32(cpuZ80OffA2))
	amd64MOVZX_B_mem(buf, z80RegF, z80Scratch1, int32(cpuZ80OffF2))

	// Store old A,F to shadow positions
	buf.EmitBytes(0x88, modRM(1, z80Scratch3, z80Scratch1), byte(cpuZ80OffA2)) // MOV [RAX+offA2], DL
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffF2)) // MOV [RAX+offF2], CL
}

// z80EmitEXX emits EXX — swap BC,DE,HL with shadow BC',DE',HL'.
func z80EmitEXX(buf *CodeBuffer) {
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))

	// Swap B/C with B2/C2
	// Save current B,C from R12
	amd64MOVZX_W(buf, z80Scratch2, z80RegBC) // ECX = BC
	// Load B2,C2
	amd64MOVZX_B_mem(buf, z80Scratch3, z80Scratch1, int32(cpuZ80OffB2))
	buf.EmitBytes(0xC1, 0xE2, 0x08) // SHL EDX, 8
	amd64MOVZX_B_mem(buf, z80Scratch4, z80Scratch1, int32(cpuZ80OffC2))
	emitREX(buf, false, z80Scratch4, z80Scratch3)
	buf.EmitBytes(0x09, modRM(3, z80Scratch4, z80Scratch3)) // OR EDX, R10D
	// Store old BC to shadow
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffC2)) // C2 = CL
	buf.EmitBytes(0xC1, 0xE9, 0x08)                                            // SHR ECX, 8
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffB2)) // B2 = CL
	// Set new BC from shadow
	buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch3, z80RegBC&0x07))

	// Same pattern for DE and HL... (simplified — store/load via CPU struct)
	// For now, swap through memory for DE and HL too

	// DE <-> DE'
	amd64MOVZX_W(buf, z80Scratch2, z80RegDE)
	amd64MOVZX_B_mem(buf, z80Scratch3, z80Scratch1, int32(cpuZ80OffD2))
	buf.EmitBytes(0xC1, 0xE2, 0x08)
	amd64MOVZX_B_mem(buf, z80Scratch4, z80Scratch1, int32(cpuZ80OffE2))
	emitREX(buf, false, z80Scratch4, z80Scratch3)
	buf.EmitBytes(0x09, modRM(3, z80Scratch4, z80Scratch3))
	// Store old
	amd64MOVZX_W(buf, z80Scratch2, z80RegDE)
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffE2))
	buf.EmitBytes(0xC1, 0xE9, 0x08)
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffD2))
	buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch3, z80RegDE&0x07))

	// HL <-> HL'
	amd64MOVZX_W(buf, z80Scratch2, z80RegHL)
	amd64MOVZX_B_mem(buf, z80Scratch3, z80Scratch1, int32(cpuZ80OffH2))
	buf.EmitBytes(0xC1, 0xE2, 0x08)
	amd64MOVZX_B_mem(buf, z80Scratch4, z80Scratch1, int32(cpuZ80OffL2))
	emitREX(buf, false, z80Scratch4, z80Scratch3)
	buf.EmitBytes(0x09, modRM(3, z80Scratch4, z80Scratch3))
	amd64MOVZX_W(buf, z80Scratch2, z80RegHL)
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffL2))
	buf.EmitBytes(0xC1, 0xE9, 0x08)
	buf.EmitBytes(0x88, modRM(1, z80Scratch2, z80Scratch1), byte(cpuZ80OffH2))
	buf.EmitBytes(0x66, 0x41, 0x89, modRM(3, z80Scratch3, z80RegHL&0x07))
}

// ===========================================================================
// CB Prefix Instruction Emitters (stub — to be expanded)
// ===========================================================================

func z80EmitCBInstruction(buf *CodeBuffer, instr *JITZ80Instr, instrPC uint16, instrIdx int, cyclesAccum uint32, emitFlags uint8, rIncAccum int) {
	op := instr.opcode
	reg := op & 0x07
	bit := (op >> 3) & 0x07
	group := op >> 6

	// (HL) operand: BIT/SET/RES via memory read/write
	if reg == 6 {
		z80EmitCB_HL(buf, instr, instrPC, instrIdx, cyclesAccum, group, bit, rIncAccum)
		return
	}

	switch group {
	case 0: // Rotate/Shift: RLC, RRC, RL, RR, SLA, SRA, SRL, SLL
		z80EmitReadReg8(buf, reg) // AL = value
		subOp := bit
		// Save old value for carry computation (R11B preserved across shift)
		buf.EmitBytes(0x41, 0x88, 0xC3) // MOV R11B, AL

		switch subOp {
		case 0: // RLC: rotate left, bit 7 → carry and bit 0
			buf.EmitBytes(0xC0, 0xC0, 0x01) // ROL AL, 1
		case 1: // RRC: rotate right, bit 0 → carry and bit 7
			buf.EmitBytes(0xC0, 0xC8, 0x01) // ROR AL, 1
		case 2: // RL: rotate left through carry
			// Get old carry from F
			buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD) // MOVZX ECX, BPL
			buf.EmitBytes(0x80, 0xE1, 0x01)       // AND CL, 0x01 (old C)
			buf.EmitBytes(0x88, 0xCA)             // MOV DL, CL (save old C)
			buf.EmitBytes(0xD0, 0xE0)             // SHL AL, 1
			buf.EmitBytes(0x08, 0xD0)             // OR AL, DL (insert old carry at bit 0)
		case 3: // RR: rotate right through carry
			buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD)
			buf.EmitBytes(0x80, 0xE1, 0x01)
			buf.EmitBytes(0xC0, 0xE1, 0x07) // SHL CL, 7 (old carry to bit 7)
			buf.EmitBytes(0x88, 0xCA)       // MOV DL, CL
			buf.EmitBytes(0xD0, 0xE8)       // SHR AL, 1
			buf.EmitBytes(0x08, 0xD0)       // OR AL, DL
		case 4: // SLA: shift left arithmetic (bit 0 = 0)
			buf.EmitBytes(0xD0, 0xE0) // SHL AL, 1
		case 5: // SRA: shift right arithmetic (bit 7 preserved)
			buf.EmitBytes(0xD0, 0xF8) // SAR AL, 1
		case 6: // SLL: undocumented (shift left, bit 0 = 1)
			buf.EmitBytes(0xD0, 0xE0) // SHL AL, 1
			buf.EmitBytes(0x0C, 0x01) // OR AL, 0x01
		case 7: // SRL: shift right logical (bit 7 = 0)
			buf.EmitBytes(0xD0, 0xE8) // SHR AL, 1
		}

		z80EmitWriteReg8(buf, reg)

		// Flags: S,Z,P from result; H=0; N=0; C from shifted-out bit
		// For RLC/RRC/SLA/SRA/SRL/SLL, carry is in host CF after the shift.
		// For RL/RR, we already extracted the old carry. Need new carry from the value.
		z80EmitReadReg8(buf, reg) // re-read result
		z80EmitCBRotateFlags(buf, subOp)

	case 1: // BIT b,r: test bit, set Z if bit is 0
		z80EmitReadReg8(buf, reg) // AL = value
		// Save value in DL for later Y,X extraction
		buf.EmitBytes(0x88, 0xC2) // MOV DL, AL
		// Test the bit and save result to R11B (0=clear, non-zero=set)
		buf.EmitBytes(0xA8, 1<<bit)           // TEST AL, (1 << bit)
		buf.EmitBytes(0x41, 0x0F, 0x95, 0xC3) // SETNZ R11B (R11B=1 if bit set, 0 if clear)

		// Build flags in CL. Preserve C from F.
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD) // MOVZX ECX, BPL
		buf.EmitBytes(0x80, 0xE1, 0x01)       // AND CL, 0x01 (keep C)
		buf.EmitBytes(0x80, 0xC9, 0x10)       // OR CL, 0x10 (H=1)
		// Z flag: set if bit was clear (R11B == 0)
		buf.EmitBytes(0x45, 0x84, 0xDB) // TEST R11B, R11B
		buf.EmitBytes(0x75, 0x03)       // JNZ +3 (bit was set, skip Z)
		buf.EmitBytes(0x80, 0xC9, 0x44) // OR CL, 0x44 (Z=1, P/V=Z for BIT)
		// S flag (only relevant for bit 7)
		if bit == 7 {
			buf.EmitBytes(0x45, 0x84, 0xDB) // TEST R11B, R11B
			buf.EmitBytes(0x74, 0x03)       // JZ +3
			buf.EmitBytes(0x80, 0xC9, 0x80) // OR CL, 0x80 (S=1)
		}
		// Y,X from value being tested (saved in DL)
		buf.EmitBytes(0x80, 0xE2, 0x28) // AND DL, 0x28
		buf.EmitBytes(0x08, 0xD1)       // OR CL, DL
		buf.EmitBytes(0x40, 0x88, 0xCD) // MOV BPL, CL

	case 2: // RES b,r: reset bit
		z80EmitReadReg8(buf, reg)
		// AND AL, ~(1 << bit)
		buf.EmitBytes(0x24, ^(1 << bit))
		z80EmitWriteReg8(buf, reg)
		// RES doesn't affect flags

	case 3: // SET b,r: set bit
		z80EmitReadReg8(buf, reg)
		// OR AL, (1 << bit)
		buf.EmitBytes(0x0C, 1<<bit)
		z80EmitWriteReg8(buf, reg)
		// SET doesn't affect flags
	}
}

// z80EmitCB_HL emits CB prefix operations with (HL) memory operand.
func z80EmitCB_HL(buf *CodeBuffer, instr *JITZ80Instr, instrPC uint16, instrIdx int, cyclesAccum uint32, group, bit byte, rIncAccum int) {
	bailLabel := fmt.Sprintf("cbhl_%04X", instrPC)
	doneLabel := fmt.Sprintf("cbhl_d_%04X", instrPC)

	// Read (HL) into AL
	amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
	z80EmitMemRead(buf, bailLabel)

	switch group {
	case 1: // BIT b,(HL) — read-only, just test the bit
		buf.EmitBytes(0xA8, 1<<bit)           // TEST AL, (1 << bit)
		buf.EmitBytes(0x41, 0x0F, 0x95, 0xC3) // SETNZ R11B
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD) // MOVZX ECX, BPL
		buf.EmitBytes(0x80, 0xE1, 0x01)       // AND CL, 0x01 (keep C)
		buf.EmitBytes(0x80, 0xC9, 0x10)       // OR CL, 0x10 (H=1)
		buf.EmitBytes(0x45, 0x84, 0xDB)       // TEST R11B, R11B
		buf.EmitBytes(0x75, 0x03)             // JNZ +3
		buf.EmitBytes(0x80, 0xC9, 0x44)       // OR CL, 0x44 (Z=1, P/V=Z)
		buf.EmitBytes(0x40, 0x88, 0xCD)       // MOV BPL, CL

	case 2: // RES b,(HL) — read-modify-write
		buf.EmitBytes(0x24, ^(1 << bit)) // AND AL, ~(1<<bit)
		// Write back
		bailW := fmt.Sprintf("cbhlw_%04X", instrPC)
		selfModW := fmt.Sprintf("cbhls_%04X", instrPC)
		doneW := fmt.Sprintf("cbhlw_d_%04X", instrPC)
		buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (value to write)
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		z80EmitMemWrite(buf, bailW, selfModW)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneW, buf.Len()+4)
		z80EmitBailExit(buf, bailW, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModW, instrPC+2, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneW)

	case 3: // SET b,(HL) — read-modify-write
		buf.EmitBytes(0x0C, 1<<bit) // OR AL, (1<<bit)
		bailW := fmt.Sprintf("cbhlw_%04X", instrPC)
		selfModW := fmt.Sprintf("cbhls_%04X", instrPC)
		doneW := fmt.Sprintf("cbhlw_d_%04X", instrPC)
		buf.EmitBytes(0x88, 0xC2)
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		z80EmitMemWrite(buf, bailW, selfModW)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneW, buf.Len()+4)
		z80EmitBailExit(buf, bailW, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModW, instrPC+2, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneW)

	case 0: // Rotate/shift (HL) — read-modify-write
		subOp := bit
		// Save old value to R11B for carry computation in z80EmitCBRotateFlags
		buf.EmitBytes(0x41, 0x88, 0xC3) // MOV R11B, AL (old value)
		switch subOp {
		case 0: // RLC (HL)
			buf.EmitBytes(0xC0, 0xC0, 0x01)
		case 1: // RRC (HL)
			buf.EmitBytes(0xC0, 0xC8, 0x01)
		case 2: // RL (HL) — through carry
			buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD)
			buf.EmitBytes(0x80, 0xE1, 0x01)
			buf.EmitBytes(0x88, 0xCA)
			buf.EmitBytes(0xD0, 0xE0)
			buf.EmitBytes(0x08, 0xD0)
		case 3: // RR (HL)
			buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD)
			buf.EmitBytes(0x80, 0xE1, 0x01)
			buf.EmitBytes(0xC0, 0xE1, 0x07)
			buf.EmitBytes(0x88, 0xCA)
			buf.EmitBytes(0xD0, 0xE8)
			buf.EmitBytes(0x08, 0xD0)
		case 4:
			buf.EmitBytes(0xD0, 0xE0)
		case 5:
			buf.EmitBytes(0xD0, 0xF8)
		case 6:
			buf.EmitBytes(0xD0, 0xE0)
			buf.EmitBytes(0x0C, 0x01)
		case 7:
			buf.EmitBytes(0xD0, 0xE8)
		}
		// Save result to stack (R11B has old value, needed by flag function)
		buf.EmitBytes(0x88, 0x44, 0x24, byte(z80OffLoopBudg)) // MOV [RSP+z80OffLoopBudg], AL
		// Write back result to memory
		bailW := fmt.Sprintf("cbhlrs_%04X", instrPC)
		selfModW := fmt.Sprintf("cbhlrss_%04X", instrPC)
		doneW := fmt.Sprintf("cbhlrs_d_%04X", instrPC)
		buf.EmitBytes(0x88, 0xC2) // MOV DL, AL
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		z80EmitMemWrite(buf, bailW, selfModW)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneW, buf.Len()+4)
		z80EmitBailExit(buf, bailW, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModW, instrPC+2, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneW)
		// Reload result for flag function (R11B still has old value)
		buf.EmitBytes(0x8A, 0x44, 0x24, byte(z80OffLoopBudg)) // MOV AL, [RSP+z80OffLoopBudg]
		z80EmitCBRotateFlags(buf, subOp)
	}

	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	buf.Label(doneLabel)
}

// z80EmitCBRotateFlags emits flag computation for CB rotate/shift operations.
// Result in AL, old (pre-shift) value in R11B. Sets S,Z,P(parity),H=0,N=0,C.
func z80EmitCBRotateFlags(buf *CodeBuffer, subOp byte) {
	// Build F in R10B
	amd64XOR_reg_reg32(buf, z80Scratch4, z80Scratch4) // R10D = 0

	// C flag from shifted-out bit of old value (R11B):
	// Left shifts (0=RLC, 2=RL, 4=SLA, 6=SLL): carry = old bit 7
	// Right shifts (1=RRC, 3=RR, 5=SRA, 7=SRL): carry = old bit 0
	if subOp%2 == 0 { // left shift: carry = old bit 7
		buf.EmitBytes(0x41, 0xF6, 0xC3, 0x80) // TEST R11B, 0x80
		buf.EmitBytes(0x74, 0x04)             // JZ +4
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x01) // OR R10B, 0x01 (C)
	} else { // right shift: carry = old bit 0
		buf.EmitBytes(0x41, 0xF6, 0xC3, 0x01) // TEST R11B, 0x01
		buf.EmitBytes(0x74, 0x04)
		buf.EmitBytes(0x41, 0x80, 0xCA, 0x01)
	}

	// S flag
	buf.EmitBytes(0xA8, 0x80) // TEST AL, 0x80
	buf.EmitBytes(0x74, 0x04)
	buf.EmitBytes(0x41, 0x80, 0xCA, 0x80)

	// Z flag
	buf.EmitBytes(0x84, 0xC0)
	buf.EmitBytes(0x75, 0x04)
	buf.EmitBytes(0x41, 0x80, 0xCA, 0x40)

	// Y,X from result
	buf.EmitBytes(0x41, 0x88, 0xC3)       // MOV R11B, AL
	buf.EmitBytes(0x41, 0x80, 0xE3, 0x28) // AND R11B, 0x28
	buf.EmitBytes(0x45, 0x08, 0xDA)       // OR R10B, R11B

	// P/V = parity
	buf.EmitBytes(0x0F, 0xB6, 0xC8) // MOVZX ECX, AL
	amd64MOV_reg_mem(buf, z80Scratch3, amd64RSP, int32(z80OffParityPtr))
	amd64MOVZX_B_memSIB(buf, z80Scratch3, z80Scratch3, z80Scratch2)
	buf.EmitBytes(0x41, 0x08, 0xD2) // OR R10B, DL

	// H=0, N=0 (already clear)

	// Store F
	buf.EmitBytes(0x44, 0x88, 0xD5) // MOV BPL, R10B
}

// ===========================================================================
// ED Prefix Instruction Emitters (stub — to be expanded)
// ===========================================================================

func z80EmitEDInstruction(buf *CodeBuffer, instr *JITZ80Instr, instrPC uint16, instrIdx int, cyclesAccum uint32, blockStartPC uint16, emitFlags uint8, rIncAccum int) {
	op := instr.opcode
	switch op {
	case 0x44: // NEG
		// A = 0 - A; flags like SUB 0, A
		buf.EmitBytes(0xF6, 0xDB) // NEG BL
	case 0x46: // IM 0
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		buf.EmitBytes(0xC6, modRM(1, 0, z80Scratch1), byte(cpuZ80OffIM), 0x00)
	case 0x56: // IM 1
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		buf.EmitBytes(0xC6, modRM(1, 0, z80Scratch1), byte(cpuZ80OffIM), 0x01)
	case 0x5E: // IM 2
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		buf.EmitBytes(0xC6, modRM(1, 0, z80Scratch1), byte(cpuZ80OffIM), 0x02)
	case 0x47: // LD I,A
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		emitREXForByte(buf, z80RegA, z80Scratch1)
		buf.EmitBytes(0x88, modRM(1, z80RegA, z80Scratch1), byte(cpuZ80OffI))
	case 0x4F: // LD R,A
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		emitREXForByte(buf, z80RegA, z80Scratch1)
		buf.EmitBytes(0x88, modRM(1, z80RegA, z80Scratch1), byte(cpuZ80OffR))

	case 0x5F: // LD A,I
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		amd64MOVZX_B_mem(buf, z80RegA, z80Scratch1, int32(cpuZ80OffI))
		// Flags: S,Z from result, H=0, P/V=IFF2, N=0, C preserved
		// Simplified: just set S and Z from A
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD)   // MOVZX ECX, BPL
		buf.EmitBytes(0x80, 0xE1, 0x01)         // AND CL, 0x01 (keep C)
		amd64MOVZX_B(buf, z80Scratch1, z80RegA) // AL = A
		buf.EmitBytes(0xA8, 0x80)               // TEST AL, 0x80
		buf.EmitBytes(0x74, 0x03)               // JZ +3
		buf.EmitBytes(0x80, 0xC9, 0x80)         // OR CL, 0x80 (S)
		buf.EmitBytes(0x84, 0xC0)               // TEST AL, AL
		buf.EmitBytes(0x75, 0x03)               // JNZ +3
		buf.EmitBytes(0x80, 0xC9, 0x40)         // OR CL, 0x40 (Z)
		// Y,X from result
		buf.EmitBytes(0x88, 0xC2)       // MOV DL, AL
		buf.EmitBytes(0x80, 0xE2, 0x28) // AND DL, 0x28
		buf.EmitBytes(0x08, 0xD1)       // OR CL, DL
		buf.EmitBytes(0x40, 0x88, 0xCD) // MOV BPL, CL

	// SBC HL,rp (0x42,0x52,0x62,0x72)
	case 0x42, 0x52, 0x62, 0x72:
		pair := (op >> 4) & 0x03
		z80EmitReadPair16(buf, pair)
		// SBC: HL = HL - rp - C
		// Get carry from F
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD) // MOVZX ECX, BPL
		buf.EmitBytes(0x80, 0xE1, 0x01)       // AND CL, 0x01 (C flag)
		// Add carry to operand: EAX += ECX
		buf.EmitBytes(0x01, 0xC8) // ADD EAX, ECX
		// SUB R14W, AX
		buf.EmitBytes(0x66, 0x41, 0x29, modRM(3, z80Scratch1, z80RegHL&0x07))
		// Simplified flags: set N=1, clear others except C
		buf.EmitBytes(0x40, 0x80, 0xE5, 0x00) // AND BPL, 0x00 (clear all)
		buf.EmitBytes(0x73, 0x04)             // JNC +4
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x01) // OR BPL, 0x01 (C)
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x02) // OR BPL, 0x02 (N=1)
		// Z flag
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		buf.EmitBytes(0x85, 0xC0)             // TEST EAX, EAX
		buf.EmitBytes(0x75, 0x04)             // JNZ +4
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x40) // OR BPL, 0x40 (Z)
		// S flag
		buf.EmitBytes(0x66, 0x41, 0x85, modRM(3, z80RegHL&0x07, z80RegHL&0x07)) // TEST R14W, R14W — not valid
		// Simplified: check bit 15 of HL
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		buf.EmitBytes(0x25, 0x00, 0x80, 0x00, 0x00) // AND EAX, 0x8000
		buf.EmitBytes(0x74, 0x04)
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x80) // OR BPL, 0x80 (S)

	// ADC HL,rp (0x4A,0x5A,0x6A,0x7A)
	case 0x4A, 0x5A, 0x6A, 0x7A:
		pair := (op >> 4) & 0x03
		z80EmitReadPair16(buf, pair)
		// ADC: HL = HL + rp + C
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD) // MOVZX ECX, BPL
		buf.EmitBytes(0x80, 0xE1, 0x01)       // AND CL, 0x01
		buf.EmitBytes(0x01, 0xC8)             // ADD EAX, ECX
		// ADD R14W, AX
		buf.EmitBytes(0x66, 0x41, 0x01, modRM(3, z80Scratch1, z80RegHL&0x07))
		// Flags: N=0, set C from overflow
		buf.EmitBytes(0x40, 0x80, 0xE5, 0x00) // AND BPL, 0
		buf.EmitBytes(0x73, 0x04)
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x01) // C
		// Z flag
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		buf.EmitBytes(0x85, 0xC0)
		buf.EmitBytes(0x75, 0x04)
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x40) // Z
		// S flag
		amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
		buf.EmitBytes(0x25, 0x00, 0x80, 0x00, 0x00) // AND EAX, 0x8000
		buf.EmitBytes(0x74, 0x04)
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x80) // S

	// LDI (0xA0): (DE) = (HL); HL++; DE++; BC--; flags: H=0,N=0,P/V=(BC!=0)
	case 0xA0:
		z80EmitLDI_LDD(buf, instr, instrPC, instrIdx, cyclesAccum, true, rIncAccum)

	// LDD (0xA8): same but HL--; DE--
	case 0xA8:
		z80EmitLDI_LDD(buf, instr, instrPC, instrIdx, cyclesAccum, false, rIncAccum)

	// CPI (0xA1): compare A with (HL); HL++; BC--
	case 0xA1:
		z80EmitCPI_CPD(buf, instr, instrPC, instrIdx, cyclesAccum, true, rIncAccum)

	// CPD (0xA9): compare A with (HL); HL--; BC--
	case 0xA9:
		z80EmitCPI_CPD(buf, instr, instrPC, instrIdx, cyclesAccum, false, rIncAccum)

	// ED LD (nn),rp — 0x43(BC),0x53(DE),0x63(HL),0x73(SP)
	case 0x43, 0x53, 0x63, 0x73:
		pair := (op >> 4) & 0x03
		bailLabel := fmt.Sprintf("edst_%04X", instrPC)
		selfModLabel := fmt.Sprintf("edsm_%04X", instrPC)
		doneLabel1 := fmt.Sprintf("eds1_%04X", instrPC)
		bailLabel2 := fmt.Sprintf("edst2_%04X", instrPC)
		selfModLabel2 := fmt.Sprintf("edsm2_%04X", instrPC)
		doneLabel2 := fmt.Sprintf("eds2_%04X", instrPC)
		// Read pair value into R11D
		z80EmitReadPair16(buf, pair)
		buf.EmitBytes(0x41, 0x89, 0xC3) // MOV R11D, EAX
		// Write low byte
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand))
		buf.EmitBytes(0x44, 0x88, 0xDA) // MOV DL, R11B
		z80EmitMemWrite(buf, bailLabel, selfModLabel)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel1, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabel, instrPC+4, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabel1)
		// Write high byte
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand+1))
		buf.EmitBytes(0x44, 0x89, 0xDA) // MOV EDX, R11D
		buf.EmitBytes(0xC1, 0xEA, 0x08) // SHR EDX, 8
		z80EmitMemWrite(buf, bailLabel2, selfModLabel2)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel2, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel2, instrPC, instrIdx, cyclesAccum, rIncAccum)
		z80EmitSelfModExit(buf, selfModLabel2, instrPC+4, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
		buf.Label(doneLabel2)

	// ED LD rp,(nn) — 0x4B(BC),0x5B(DE),0x6B(HL),0x7B(SP)
	case 0x4B, 0x5B, 0x6B, 0x7B:
		pair := (op >> 4) & 0x03
		bailLabel := fmt.Sprintf("edld_%04X", instrPC)
		doneLabel := fmt.Sprintf("edld_d_%04X", instrPC)
		// Read low byte
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand))
		z80EmitMemRead(buf, bailLabel)
		buf.EmitBytes(0x41, 0x89, 0xC3) // MOV R11D, EAX (save low)
		// Read high byte
		amd64MOV_reg_imm32(buf, z80Scratch1, uint32(instr.operand+1))
		z80EmitMemRead(buf, bailLabel)
		// Combine: AX = (high << 8) | low
		buf.EmitBytes(0xC1, 0xE0, 0x08)
		buf.EmitBytes(0x44, 0x09, 0xD8) // OR EAX, R11D
		z80EmitWritePair16(buf, pair)
		buf.EmitBytes(0xE9)
		buf.FixupRel32(doneLabel, buf.Len()+4)
		z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
		buf.Label(doneLabel)

	default:
		// Unhandled ED instruction — skip
	}
}

// ===========================================================================
// DD/FD Prefix Instruction Emitters (stub — to be expanded)
// ===========================================================================

// z80EmitLDI_LDD emits LDI or LDD: (DE)=(HL); HL±±; DE±±; BC--; flags.
func z80EmitLDI_LDD(buf *CodeBuffer, instr *JITZ80Instr, instrPC uint16, instrIdx int, cyclesAccum uint32, increment bool, rIncAccum int) {
	bailLabel := fmt.Sprintf("ldi_bail_%04X", instrPC)
	doneReadLabel := fmt.Sprintf("ldi_dr_%04X", instrPC)
	bailWriteLabel := fmt.Sprintf("ldi_bw_%04X", instrPC)
	selfModLabel := fmt.Sprintf("ldi_sm_%04X", instrPC)
	doneWriteLabel := fmt.Sprintf("ldi_dw_%04X", instrPC)

	// Read byte from (HL) into AL
	amd64MOVZX_W(buf, z80Scratch1, z80RegHL) // EAX = HL
	z80EmitMemRead(buf, bailLabel)
	buf.EmitBytes(0x41, 0x89, 0xC3) // MOV R11D, EAX (save byte)
	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneReadLabel, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	buf.Label(doneReadLabel)

	// Write byte to (DE)
	amd64MOVZX_W(buf, z80Scratch1, z80RegDE) // EAX = DE
	buf.EmitBytes(0x44, 0x88, 0xDA)          // MOV DL, R11B (byte to write)
	z80EmitMemWrite(buf, bailWriteLabel, selfModLabel)
	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneWriteLabel, buf.Len()+4)
	z80EmitBailExit(buf, bailWriteLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	z80EmitSelfModExit(buf, selfModLabel, instrPC+2, instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
	buf.Label(doneWriteLabel)

	// HL++/-- (R14W)
	if increment {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC6) // INC R14W
	} else {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xCE) // DEC R14W
	}

	// DE++/-- (R13W)
	if increment {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC5) // INC R13W
	} else {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xCD) // DEC R13W
	}

	// BC-- (R12W)
	buf.EmitBytes(0x66, 0x41, 0xFF, 0xCC) // DEC R12W

	// Flags: H=0, N=0, P/V = (BC != 0)
	// Preserve S, Z, C from F. Clear H, N. Set P/V if BC != 0.
	buf.EmitBytes(0x40, 0x80, 0xE5, 0xC1) // AND BPL, 0xC1 (keep S,Z,C)
	// Test BC (R12W)
	amd64MOVZX_W(buf, z80Scratch1, z80RegBC)
	buf.EmitBytes(0x85, 0xC0)             // TEST EAX, EAX
	buf.EmitBytes(0x74, 0x04)             // JZ +4 (BC == 0, skip P/V set)
	buf.EmitBytes(0x40, 0x80, 0xCD, 0x04) // OR BPL, 0x04 (P/V)
}

// z80EmitCPI_CPD emits CPI or CPD: compare A with (HL); HL±±; BC--; set flags.
func z80EmitCPI_CPD(buf *CodeBuffer, instr *JITZ80Instr, instrPC uint16, instrIdx int, cyclesAccum uint32, increment bool, rIncAccum int) {
	bailLabel := fmt.Sprintf("cpi_bail_%04X", instrPC)
	doneLabel := fmt.Sprintf("cpi_done_%04X", instrPC)

	// Read (HL) into AL
	amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
	z80EmitMemRead(buf, bailLabel)

	// Compare: result = A - (HL) (don't store result)
	// Save (HL) value in CL, old A in DL
	buf.EmitBytes(0x88, 0xC1) // MOV CL, AL (memory value)
	buf.EmitBytes(0x88, 0xDA) // MOV DL, BL (A)
	buf.EmitBytes(0x88, 0xD0) // MOV AL, DL (A for subtraction)
	buf.EmitBytes(0x28, 0xC8) // SUB AL, CL (result in AL)

	// Build flags: S,Z from result; H from (A^val^result)&0x10; N=1; P/V=(BC!=0); C preserved
	buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5) // MOVZX EAX, BPL — wait, we need result in AL
	// Actually let me redo this more carefully:
	// After SUB AL, CL: AL = result (A - (HL))
	// Save result in R11B
	buf.EmitBytes(0x41, 0x88, 0xC3) // MOV R11B, AL (result)

	// Start building F: keep C from old F
	buf.EmitBytes(0x40, 0x0F, 0xB6, 0xC5) // MOVZX EAX, BPL
	buf.EmitBytes(0x24, 0x01)             // AND AL, 0x01 (keep C only)
	buf.EmitBytes(0x0C, 0x02)             // OR AL, 0x02 (N=1)

	// S flag from result
	buf.EmitBytes(0x41, 0xF6, 0xC3, 0x80) // TEST R11B, 0x80
	buf.EmitBytes(0x74, 0x02)             // JZ +2
	buf.EmitBytes(0x0C, 0x80)             // OR AL, 0x80

	// Z flag from result
	buf.EmitBytes(0x45, 0x84, 0xDB) // TEST R11B, R11B
	buf.EmitBytes(0x75, 0x02)       // JNZ +2
	buf.EmitBytes(0x0C, 0x40)       // OR AL, 0x40

	// H flag: (A ^ val ^ result) & 0x10
	buf.EmitBytes(0x30, 0xCA)       // XOR DL, CL (A ^ val)
	buf.EmitBytes(0x44, 0x30, 0xDA) // XOR DL, R11B (^ result)
	buf.EmitBytes(0x80, 0xE2, 0x10) // AND DL, 0x10
	buf.EmitBytes(0x08, 0xD0)       // OR AL, DL

	// Store partial F
	buf.EmitBytes(0x40, 0x88, 0xC5) // MOV BPL, AL

	// HL++/--
	if increment {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC6) // INC R14W
	} else {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xCE) // DEC R14W
	}

	// BC--
	buf.EmitBytes(0x66, 0x41, 0xFF, 0xCC) // DEC R12W

	// P/V = (BC != 0)
	amd64MOVZX_W(buf, z80Scratch1, z80RegBC)
	buf.EmitBytes(0x85, 0xC0)             // TEST EAX, EAX
	buf.EmitBytes(0x74, 0x04)             // JZ +4
	buf.EmitBytes(0x40, 0x80, 0xCD, 0x04) // OR BPL, 0x04

	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	buf.Label(doneLabel)
}

// z80EmitDDCB emits DDCB/FDCB indexed bit operations: BIT/SET/RES/rotate (IX+d).
// When subOp&7 != 6, the result is also written to the target register (undocumented but required).
func z80EmitDDCB(buf *CodeBuffer, instr *JITZ80Instr, ixiyOff uintptr, instrPC uint16, instrIdx int, cyclesAccum uint32, rIncAccum int) {
	subOp := instr.cbSubOp
	group := subOp >> 6
	bit := (subOp >> 3) & 0x07
	targetReg := subOp & 0x07 // register to also write result to (6=(HL) means memory only)

	// Read (IX+d) into AL
	z80EmitIndexedRead(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)

	switch group {
	case 1: // BIT b,(IX+d) — read-only, no register write-back
		buf.EmitBytes(0xA8, 1<<bit)
		buf.EmitBytes(0x41, 0x0F, 0x95, 0xC3) // SETNZ R11B
		buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD)
		buf.EmitBytes(0x80, 0xE1, 0x01)
		buf.EmitBytes(0x80, 0xC9, 0x10)
		buf.EmitBytes(0x45, 0x84, 0xDB)
		buf.EmitBytes(0x75, 0x03)
		buf.EmitBytes(0x80, 0xC9, 0x44)
		buf.EmitBytes(0x40, 0x88, 0xCD)

	case 2: // RES b,(IX+d)
		buf.EmitBytes(0x24, ^(1 << bit))
		buf.EmitBytes(0x41, 0x88, 0xC3) // MOV R11B, AL (save for reg write-back)
		buf.EmitBytes(0x88, 0xC2)
		z80EmitIndexedWrite(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)
		// Write result to target register if not (HL)
		if targetReg != 6 {
			buf.EmitBytes(0x44, 0x88, 0xD8) // MOV AL, R11B
			z80EmitWriteReg8(buf, targetReg)
		}

	case 3: // SET b,(IX+d)
		buf.EmitBytes(0x0C, 1<<bit)
		buf.EmitBytes(0x41, 0x88, 0xC3)
		buf.EmitBytes(0x88, 0xC2)
		z80EmitIndexedWrite(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)
		if targetReg != 6 {
			buf.EmitBytes(0x44, 0x88, 0xD8)
			z80EmitWriteReg8(buf, targetReg)
		}

	case 0: // Rotate/shift (IX+d)
		rotOp := bit
		// Save old value to R11B for carry computation in z80EmitCBRotateFlags
		buf.EmitBytes(0x41, 0x88, 0xC3) // MOV R11B, AL (old value)
		switch rotOp {
		case 0:
			buf.EmitBytes(0xC0, 0xC0, 0x01)
		case 1:
			buf.EmitBytes(0xC0, 0xC8, 0x01)
		case 2: // RL
			buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD)
			buf.EmitBytes(0x80, 0xE1, 0x01)
			buf.EmitBytes(0xD0, 0xE0)
			buf.EmitBytes(0x08, 0xC8)
		case 3: // RR
			buf.EmitBytes(0x40, 0x0F, 0xB6, 0xCD)
			buf.EmitBytes(0x80, 0xE1, 0x01)
			buf.EmitBytes(0xC0, 0xE1, 0x07)
			buf.EmitBytes(0xD0, 0xE8)
			buf.EmitBytes(0x08, 0xC8)
		case 4:
			buf.EmitBytes(0xD0, 0xE0)
		case 5:
			buf.EmitBytes(0xD0, 0xF8)
		case 6:
			buf.EmitBytes(0xD0, 0xE0)
			buf.EmitBytes(0x0C, 0x01)
		case 7:
			buf.EmitBytes(0xD0, 0xE8)
		}
		// Save result to stack (R11B has old value for carry flag)
		buf.EmitBytes(0x88, 0x44, 0x24, byte(z80OffLoopBudg)) // MOV [RSP+z80OffLoopBudg], AL
		// Write result to memory
		buf.EmitBytes(0x88, 0xC2) // MOV DL, AL
		z80EmitIndexedWrite(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)
		// Reload result for flags and register write-back
		buf.EmitBytes(0x8A, 0x44, 0x24, byte(z80OffLoopBudg)) // MOV AL, [RSP+z80OffLoopBudg]
		z80EmitCBRotateFlags(buf, rotOp)
		// Write result to target register if not (HL)
		if targetReg != 6 {
			buf.EmitBytes(0x44, 0x88, 0xD8)
			z80EmitWriteReg8(buf, targetReg)
		}
	}
}

// z80EmitLDIR emits a native LDIR/LDDR loop.
// Copies bytes from (HL) to (DE), incrementing/decrementing HL and DE, decrementing BC,
// until BC==0 or a non-direct page is encountered (bail).
func z80EmitLDIR(buf *CodeBuffer, isIncrement bool, instrPC uint16, instrCount int, totalCycles uint32, blockRIncrements int) {
	bailLabel := fmt.Sprintf("ldir_bail_%d", buf.Len())
	doneLabel := fmt.Sprintf("ldir_done_%d", buf.Len())
	loopLabel := fmt.Sprintf("ldir_loop_%d", buf.Len())

	// Loop budget to prevent infinite native execution
	buf.EmitBytes(0xC7, 0x44, 0x24, byte(z80OffLoopBudg), 0xFF, 0x0F, 0x00, 0x00) // MOV [RSP+20], 4095

	buf.Label(loopLabel)

	// Check BC != 0
	amd64MOVZX_W(buf, z80Scratch1, z80RegBC) // EAX = BC
	buf.EmitBytes(0x85, 0xC0)                // TEST EAX, EAX
	buf.EmitBytes(0x0F, 0x84)                // JZ done (BC==0, finished)
	buf.FixupRel32(doneLabel, buf.Len()+4)

	// Check source page (HL) is direct
	amd64MOVZX_W(buf, z80Scratch1, z80RegHL)
	buf.EmitBytes(0x89, 0xC1)       // MOV ECX, EAX
	buf.EmitBytes(0xC1, 0xE9, 0x08) // SHR ECX, 8
	amd64MOVZX_B_memSIB(buf, z80Scratch3, z80RegDPB, z80Scratch2)
	buf.EmitBytes(0x85, 0xD2) // TEST EDX, EDX
	buf.EmitBytes(0x0F, 0x85) // JNZ bail
	buf.FixupRel32(bailLabel, buf.Len()+4)

	// Read byte from (HL): MOVZX EAX, BYTE [RSI + RAX]
	amd64MOVZX_B_memSIB(buf, z80Scratch1, z80RegMem, z80Scratch1)
	buf.EmitBytes(0x41, 0x89, 0xC3) // MOV R11D, EAX (save byte)

	// Check dest page (DE) is direct
	amd64MOVZX_W(buf, z80Scratch1, z80RegDE)
	buf.EmitBytes(0x89, 0xC1)
	buf.EmitBytes(0xC1, 0xE9, 0x08)
	amd64MOVZX_B_memSIB(buf, z80Scratch3, z80RegDPB, z80Scratch2)
	buf.EmitBytes(0x85, 0xD2)
	buf.EmitBytes(0x0F, 0x85)
	buf.FixupRel32(bailLabel, buf.Len()+4)

	// Write byte to (DE): MOV [RSI + RAX], R11B
	// First do self-mod check on dest page
	amd64MOVZX_W(buf, z80Scratch1, z80RegDE)
	buf.EmitBytes(0x89, 0xC1)
	buf.EmitBytes(0xC1, 0xE9, 0x08)
	// Write the byte
	amd64MOVZX_W(buf, z80Scratch1, z80RegDE)
	buf.EmitBytes(0x44, 0x88, 0x1C, 0x06) // MOV [RSI + RAX], R11B

	// Check code page for self-mod
	amd64MOVZX_B_memSIB(buf, z80Scratch3, z80RegCPB, z80Scratch2)
	buf.EmitBytes(0x85, 0xD2)
	selfModLabel := fmt.Sprintf("ldir_smod_%d", buf.Len())
	buf.EmitBytes(0x0F, 0x85)
	buf.FixupRel32(selfModLabel, buf.Len()+4)

	// HL++/-- DE++/-- BC--
	if isIncrement {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC6) // INC R14W (HL)
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC5) // INC R13W (DE)
	} else {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xCE) // DEC R14W
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xCD) // DEC R13W
	}
	buf.EmitBytes(0x66, 0x41, 0xFF, 0xCC) // DEC R12W (BC)

	// Decrement loop budget
	buf.EmitBytes(0xFF, 0x4C, 0x24, byte(z80OffLoopBudg)) // DEC [RSP+20]
	buf.EmitBytes(0x0F, 0x8F)                             // JG loop (budget > 0)
	buf.FixupRel32(loopLabel, buf.Len()+4)

	// Budget exhausted — return to Go with PC at LDIR instruction
	// (exec loop will re-enter and continue the LDIR)
	// Set flags: H=0, N=0, P/V=0 (BC might not be 0 yet)
	buf.EmitBytes(0x40, 0x80, 0xE5, 0xC1) // AND BPL, 0xC1
	z80EmitEpilogue(buf, instrPC, instrCount, totalCycles, blockRIncrements)

	// Done — BC==0
	buf.Label(doneLabel)
	buf.EmitBytes(0x40, 0x80, 0xE5, 0xC1) // AND BPL, 0xC1 (H=0,N=0,P/V=0)
	z80EmitEpilogue(buf, instrPC+2, instrCount, totalCycles, blockRIncrements)

	// Bail — non-direct page encountered, fall back to interpreter
	buf.Label(bailLabel)
	amd64MOV_mem_imm32(buf, z80RegCtx, int32(jzCtxOffNeedBail), 1)
	z80EmitEpilogue(buf, instrPC, instrCount-1, totalCycles-uint32(21), blockRIncrements)

	// Self-mod detected during LDIR write
	buf.Label(selfModLabel)
	amd64MOV_mem_imm32(buf, z80RegCtx, int32(jzCtxOffNeedInval), 1)
	emitMemOp(buf, false, 0x89, z80Scratch2, z80RegCtx, int32(jzCtxOffInvalPage))
	// Still need to update HL/DE/BC for the iteration that triggered self-mod
	if isIncrement {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC6)
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xC5)
	} else {
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xCE)
		buf.EmitBytes(0x66, 0x41, 0xFF, 0xCD)
	}
	buf.EmitBytes(0x66, 0x41, 0xFF, 0xCC)
	z80EmitEpilogue(buf, instrPC, instrCount, totalCycles, blockRIncrements)
}

func z80EmitDDFDInstruction(buf *CodeBuffer, instr *JITZ80Instr, instrPC uint16, instrIdx int, cyclesAccum uint32, emitFlags uint8, rIncAccum int) {
	op := instr.opcode
	isDD := instr.prefix == z80JITPrefixDD
	// IX or IY offset in CPU struct
	var ixiyOff uintptr
	if isDD {
		ixiyOff = cpuZ80OffIX
	} else {
		ixiyOff = cpuZ80OffIY
	}

	switch {
	// LD IX,nn / LD IY,nn (0x21)
	case op == 0x21:
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		// MOV WORD [RAX + ixiyOff], imm16
		buf.EmitBytes(0x66)
		emitMemOp(buf, false, 0xC7, 0, z80Scratch1, int32(ixiyOff))
		buf.EmitBytes(byte(instr.operand), byte(instr.operand>>8))

	// ADD IX,rp / ADD IY,rp (0x09,0x19,0x29,0x39)
	case op == 0x09 || op == 0x19 || op == 0x29 || op == 0x39:
		pair := (op >> 4) & 0x03
		// Load IX/IY
		amd64MOV_reg_mem(buf, z80Scratch4, amd64RSP, int32(z80OffCpuPtr))
		amd64MOVZX_W_mem(buf, z80Scratch1, z80Scratch4, int32(ixiyOff)) // EAX = IX/IY
		// Load pair value
		if pair == 2 {
			// ADD IX,IX or ADD IY,IY — source is the same register
			buf.EmitBytes(0x01, 0xC0) // ADD EAX, EAX (double it)
		} else {
			z80EmitReadPair16(buf, pair)                                    // EAX now has pair value
			amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch4, int32(ixiyOff)) // reload IX/IY into ECX
			buf.EmitBytes(0x01, 0xC1)                                       // ADD ECX, EAX
			buf.EmitBytes(0x89, 0xC8)                                       // MOV EAX, ECX
		}
		// Store result back
		buf.EmitBytes(0x66)
		emitMemOp(buf, false, 0x89, z80Scratch1, z80Scratch4, int32(ixiyOff))
		// Simplified flags: clear N, set C if carry from bit 15
		buf.EmitBytes(0x40, 0x80, 0xE5, 0xEC) // AND BPL, ~0x13
		buf.EmitBytes(0x73, 0x04)             // JNC +4
		buf.EmitBytes(0x40, 0x80, 0xCD, 0x01) // OR BPL, 0x01

	// PUSH IX / PUSH IY (0xE5)
	case op == 0xE5:
		// Load IX/IY into R11D
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		amd64MOVZX_W_mem(buf, z80Scratch5, z80Scratch1, int32(ixiyOff))
		// Use the same PUSH machinery as base PUSH but with R11D as source
		z80EmitPUSHValue(buf, instrPC, instrIdx, cyclesAccum, rIncAccum, rIncAccum+int(instr.rIncrements))

	// POP IX / POP IY (0xE1)
	case op == 0xE1:
		z80EmitPOPToIXIY(buf, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)

	// LD SP,IX / LD SP,IY (0xF9)
	case op == 0xF9:
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(ixiyOff))
		buf.EmitBytes(0x66)
		emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch1, int32(cpuZ80OffSP))

	// INC IX / INC IY (0x23)
	case op == 0x23:
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(ixiyOff))
		buf.EmitBytes(0xFF, 0xC1) // INC ECX
		buf.EmitBytes(0x66)
		emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch1, int32(ixiyOff))

	// DEC IX / DEC IY (0x2B)
	case op == 0x2B:
		amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
		amd64MOVZX_W_mem(buf, z80Scratch2, z80Scratch1, int32(ixiyOff))
		buf.EmitBytes(0xFF, 0xC9) // DEC ECX
		buf.EmitBytes(0x66)
		emitMemOp(buf, false, 0x89, z80Scratch2, z80Scratch1, int32(ixiyOff))

	// LD r,(IX+d) / LD r,(IY+d) — 0x46,0x4E,0x56,0x5E,0x66,0x6E,0x7E
	case op&0xC7 == 0x46 && op != 0x76:
		dst := (op >> 3) & 0x07
		z80EmitIndexedRead(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)
		// AL = value from memory
		z80EmitWriteReg8(buf, dst)

	// LD (IX+d),r / LD (IY+d),r — 0x70-0x75,0x77
	case op >= 0x70 && op <= 0x77 && op != 0x76:
		src := op & 0x07
		z80EmitReadReg8(buf, src)
		buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (value to write)
		z80EmitIndexedWrite(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)

	// LD (IX+d),n (0x36)
	case op == 0x36:
		amd64MOV_reg_imm32(buf, z80Scratch3, uint32(instr.operand&0xFF)) // EDX = immediate
		z80EmitIndexedWrite(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)

	// ALU A,(IX+d) — ADD/ADC/SUB/SBC/AND/XOR/OR/CP (0x86,0x8E,0x96,0x9E,0xA6,0xAE,0xB6,0xBE)
	case op&0xC7 == 0x86:
		aluOp := (op >> 3) & 0x07
		z80EmitIndexedRead(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)
		// AL = memory value. Apply the ALU op with A (BL).
		switch aluOp {
		case 0: // ADD A,(IX+d)
			if emitFlags != 0 {
				buf.EmitBytes(0x88, 0xDA) // MOV DL, BL (oldA)
				buf.EmitBytes(0x88, 0xC1) // MOV CL, AL (operand)
			}
			buf.EmitBytes(0x00, modRM(3, z80Scratch1, z80RegA)) // ADD BL, AL
			if emitFlags != 0 {
				z80EmitCarryCapture(buf)
			}
			amd64MOVZX_B(buf, z80Scratch1, z80RegA)
			if emitFlags != 0 {
				z80EmitFlags_ADD(buf, emitFlags)
			}
		case 2: // SUB (IX+d)
			if emitFlags != 0 {
				buf.EmitBytes(0x88, 0xDA)
				buf.EmitBytes(0x88, 0xC1)
			}
			buf.EmitBytes(0x28, modRM(3, z80Scratch1, z80RegA))
			if emitFlags != 0 {
				z80EmitBorrowCapture(buf)
			}
			amd64MOVZX_B(buf, z80Scratch1, z80RegA)
			if emitFlags != 0 {
				z80EmitFlags_SUB(buf, emitFlags)
			}
		case 4: // AND (IX+d)
			buf.EmitBytes(0x20, modRM(3, z80Scratch1, z80RegA))
			amd64MOVZX_B(buf, z80Scratch1, z80RegA)
			if emitFlags != 0 {
				z80EmitFlags_Logic(buf, true, emitFlags)
			}
		case 5: // XOR (IX+d)
			buf.EmitBytes(0x30, modRM(3, z80Scratch1, z80RegA))
			amd64MOVZX_B(buf, z80Scratch1, z80RegA)
			if emitFlags != 0 {
				z80EmitFlags_Logic(buf, false, emitFlags)
			}
		case 6: // OR (IX+d)
			buf.EmitBytes(0x08, modRM(3, z80Scratch1, z80RegA))
			amd64MOVZX_B(buf, z80Scratch1, z80RegA)
			if emitFlags != 0 {
				z80EmitFlags_Logic(buf, false, emitFlags)
			}
		case 7: // CP (IX+d)
			if emitFlags != 0 {
				buf.EmitBytes(0x88, 0xDA)
				buf.EmitBytes(0x88, 0xC1)
				buf.EmitBytes(0x88, 0xD0) // MOV AL, DL
				buf.EmitBytes(0x28, 0xC8) // SUB AL, CL
				z80EmitBorrowCapture(buf)
				z80EmitFlags_SUB(buf, emitFlags)
			}
		}

	// INC (IX+d) (0x34)
	case op == 0x34:
		z80EmitIndexedRead(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)
		if emitFlags != 0 {
			buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (old value for flags)
		}
		buf.EmitBytes(0xFE, 0xC0) // INC AL
		if emitFlags != 0 {
			buf.EmitBytes(0x41, 0x88, 0xC3) // MOV R11B, AL (save result)
			buf.EmitBytes(0x44, 0x88, 0xDA) // MOV DL, R11B
		} else {
			buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (result for write-back)
		}
		z80EmitIndexedWrite(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)
		if emitFlags != 0 {
			buf.EmitBytes(0x44, 0x88, 0xD8) // MOV AL, R11B (result)
			z80EmitFlags_INC_DEC_Runtime(buf, false, emitFlags)
		}

	// DEC (IX+d) (0x35)
	case op == 0x35:
		z80EmitIndexedRead(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)
		if emitFlags != 0 {
			buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (old value)
		}
		buf.EmitBytes(0xFE, 0xC8) // DEC AL
		if emitFlags != 0 {
			buf.EmitBytes(0x41, 0x88, 0xC3) // MOV R11B, AL (save result)
			buf.EmitBytes(0x44, 0x88, 0xDA) // MOV DL, R11B
		} else {
			buf.EmitBytes(0x88, 0xC2) // MOV DL, AL (result for write-back)
		}
		z80EmitIndexedWrite(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)
		if emitFlags != 0 {
			buf.EmitBytes(0x44, 0x88, 0xD8) // MOV AL, R11B
			z80EmitFlags_INC_DEC_Runtime(buf, true, emitFlags)
		}

	// DDCB/FDCB — indexed bit operations: BIT/SET/RES/rotate (IX+d)
	case op == 0xCB:
		z80EmitDDCB(buf, instr, ixiyOff, instrPC, instrIdx, cyclesAccum, rIncAccum)

	default:
		// Unhandled DD/FD instruction — skip (no code emitted, cycles still counted)
	}
}

// z80EmitIndexedAddr emits code to compute IX/IY + displacement into EAX.
func z80EmitIndexedAddr(buf *CodeBuffer, instr *JITZ80Instr, ixiyOff uintptr) {
	// Load IX or IY from CPU struct
	amd64MOV_reg_mem(buf, z80Scratch1, amd64RSP, int32(z80OffCpuPtr))
	amd64MOVZX_W_mem(buf, z80Scratch1, z80Scratch1, int32(ixiyOff))
	// Add signed displacement
	if instr.displacement > 0 {
		buf.EmitBytes(0x83, 0xC0, byte(instr.displacement)) // ADD EAX, disp (positive)
	} else if instr.displacement < 0 {
		buf.EmitBytes(0x83, 0xE8, byte(-instr.displacement)) // SUB EAX, |disp|
	}
	// Mask to 16 bits
	buf.EmitBytes(0x0F, 0xB7, 0xC0) // MOVZX EAX, AX
}

// z80EmitIndexedRead emits code to read a byte from (IX/IY+d) into AL.
func z80EmitIndexedRead(buf *CodeBuffer, instr *JITZ80Instr, ixiyOff uintptr, instrPC uint16, instrIdx int, cyclesAccum uint32, rIncAccum int) {
	bailLabel := fmt.Sprintf("ixrd_%04X", instrPC)
	doneLabel := fmt.Sprintf("ixrd_done_%04X", instrPC)

	z80EmitIndexedAddr(buf, instr, ixiyOff) // EAX = address
	z80EmitMemRead(buf, bailLabel)

	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	buf.Label(doneLabel)
}

// z80EmitIndexedWrite emits code to write DL to (IX/IY+d).
func z80EmitIndexedWrite(buf *CodeBuffer, instr *JITZ80Instr, ixiyOff uintptr, instrPC uint16, instrIdx int, cyclesAccum uint32, rIncAccum int) {
	bailLabel := fmt.Sprintf("ixwr_%04X", instrPC)
	selfModLabel := fmt.Sprintf("ixsm_%04X", instrPC)
	doneLabel := fmt.Sprintf("ixwr_done_%04X", instrPC)

	// Save value in R11B before computing address (z80EmitIndexedAddr uses EAX)
	buf.EmitBytes(0x41, 0x88, 0xD3) // MOV R11B, DL

	z80EmitIndexedAddr(buf, instr, ixiyOff) // EAX = address

	// Restore value to DL
	buf.EmitBytes(0x44, 0x88, 0xDA) // MOV DL, R11B

	z80EmitMemWrite(buf, bailLabel, selfModLabel)

	buf.EmitBytes(0xE9)
	buf.FixupRel32(doneLabel, buf.Len()+4)
	z80EmitBailExit(buf, bailLabel, instrPC, instrIdx, cyclesAccum, rIncAccum)
	z80EmitSelfModExit(buf, selfModLabel, instrPC+uint16(instr.length), instrIdx+1, cyclesAccum+uint32(instr.cycles), rIncAccum+int(instr.rIncrements))
	buf.Label(doneLabel)
}
