// jit_x86_emit_amd64.go - x86-64 native code emitter for x86 guest JIT compiler
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
)

// ===========================================================================
// x86-64 Register Mapping for x86 Guest JIT (Tier 1: Fixed Allocation)
// ===========================================================================
//
// Host      Guest      Purpose
// ------    ------     -------
// R15       --         JITContext pointer (callee-saved)
// RSI       --         &memory[0] (memory base)
// R9        --         &x86IOBitmap[0]
// RBX       EAX        Mapped 32-bit (callee-saved)
// RBP       ECX        Mapped 32-bit (callee-saved)
// R12       EDX        Mapped 32-bit (callee-saved)
// R13       EBX(guest) Mapped 32-bit (callee-saved)
// R14       ESP        Mapped 32-bit (callee-saved)
// RAX       --         Scratch (8-bit ops, MUL/DIV, LAHF)
// RCX       --         Scratch (shift count CL)
// RDX       --         Scratch (MUL/DIV output)
// R8        --         Scratch
// R10       --         Scratch
// R11       --         Scratch

const (
	// Dedicated registers
	x86AMD64RegCtx     = amd64R15 // JITContext pointer
	x86AMD64RegMemBase = amd64RSI // &memory[0]
	x86AMD64RegIOBM    = amd64R9  // &x86IOBitmap[0]

	// Mapped guest registers (Tier 1: fixed)
	x86AMD64RegGuestEAX = amd64RBX // guest EAX -> RBX
	x86AMD64RegGuestECX = amd64RBP // guest ECX -> RBP
	x86AMD64RegGuestEDX = amd64R12 // guest EDX -> R12
	x86AMD64RegGuestEBX = amd64R13 // guest EBX -> R13
	x86AMD64RegGuestESP = amd64R14 // guest ESP -> R14

	// Stack frame layout
	// 6 callee-saved pushes (48 bytes) + SUB RSP,40 = 88 + 8 (ret addr) = 96 (16-aligned)
	x86AMD64FrameSize      = 40
	x86AMD64OffLoopBudget  = 0  // [RSP+0]:  loop budget counter (int32)
	x86AMD64OffLoopRetired = 8  // [RSP+8]:  loop retired instruction counter (int32)
	x86AMD64OffLoopStartPC = 16 // [RSP+16]: loop start PC for budget-exhaustion exit
	x86AMD64OffSavedEFlags = 24 // [RSP+24]: captured host EFLAGS after last guest flag-modifying instr
	// [RSP+32..39]: reserved / alignment
)

// x86VisibleFlagsMask is the EFLAGS subset that maps 1:1 to guest x86 Flags
// for the slice-1 / slice-2 ISA cut: CF, PF, AF, ZF, SF, OF. Reserved and
// system bits (TF, IF, DF, IOPL, NT, RF, VM, AC, VIF, VIP, ID) are
// preserved unchanged on the guest side because slice 1 does not yet
// emit instructions that modify them through native paths.
const x86VisibleFlagsMask = uint32(0x0000_08D5)

// x86InvVisibleFlagsMaskI32 is ^x86VisibleFlagsMask reinterpreted as
// int32 (necessary because amd64ALU_reg_imm32_32bit takes a signed imm
// and the bitwise complement overflows int32 as an untyped constant).
var x86InvVisibleFlagsMaskI32 = -int32(x86VisibleFlagsMask) - 1

const x86LoopBudget = 65535 // iterations before returning to Go

// x86CurrentCS is the active compile state. Set during block compilation.
var x86CurrentCS *x86CompileState

// x86CompileIOBitmap and x86CompileCodeBitmap are set by the execution loop
// before calling x86CompileBlock, allowing compile-time page safety checks.
var x86CompileIOBitmap []byte
var x86CompileCodeBitmap []byte

// x86GuestRegToHost maps guest x86 register index (0=EAX..7=EDI) to host register.
// Uses the current compile state's register mapping.
func x86GuestRegToHost(guestReg byte) (byte, bool) {
	if x86CurrentCS != nil && guestReg < 8 {
		host := x86CurrentCS.regMap[guestReg]
		if host != 0 {
			return host, true
		}
		return 0, false
	}
	// Fallback to fixed mapping (shouldn't happen during compilation)
	switch guestReg {
	case 0:
		return x86AMD64RegGuestEAX, true
	case 1:
		return x86AMD64RegGuestECX, true
	case 2:
		return x86AMD64RegGuestEDX, true
	case 3:
		return x86AMD64RegGuestEBX, true
	case 4:
		return x86AMD64RegGuestESP, true
	}
	return 0, false
}

// ===========================================================================
// EFLAGS State Tracking
// ===========================================================================

// x86FlagState is a type alias for the shared JITFlagState (jit_flags_common.go,
// introduced in Phase 2 of the JIT unification). The legacy backend constants
// remain as untyped aliases so existing emit-site references keep compiling
// unchanged.
type x86FlagState = JITFlagState

const (
	x86FlagsDead         = JITFlagDead         // no valid flag state
	x86FlagsLiveArith    = JITFlagLiveArith    // host EFLAGS from ADD/SUB/CMP/ADC/SBB
	x86FlagsLiveLogic    = JITFlagLiveLogic    // host EFLAGS from AND/OR/XOR/TEST (CF=OF=0)
	x86FlagsLiveInc      = JITFlagLiveInc      // host EFLAGS from INC/DEC (CF preserved)
	x86FlagsMaterialized = JITFlagMaterialized // guest Flags word is up-to-date
)

type x86CompileState struct {
	flagState      x86FlagState
	regMap         [8]byte         // guest reg -> host reg (0 = spilled)
	tier           int             // 0 = Tier 1, 1 = Tier 2
	flagsNeeded    []bool          // per-instruction: true if this instruction's flags are consumed
	isLoop         bool            // block contains a backward Jcc to its own startPC
	loopStartLabel int             // code buffer offset of loop body start (after prologue)
	instrPerIter   int             // number of guest instructions per loop iteration
	dirtyMask      byte            // bit i set = guest reg i was written and needs store-back
	ioBitmap       []byte          // I/O bitmap for compile-time page checks (nil if unavailable)
	codeBitmap     []byte          // code page bitmap for compile-time self-mod elision
	host           x86HostFeatures // detected host CPU features for optimal encoding selection
	// hasNonSelfLoopJcc is set when the block contains any conditional
	// branch whose target is not the block's own startPC. Such branches
	// create internal control-flow splits that invalidate the linear
	// peephole flag-liveness analysis; flag-capture skipping is disabled
	// when this is true.
	hasNonSelfLoopJcc bool
	// flagShadowed[i] is true when instruction i's host-EFLAGS effects
	// are fully shadowed by a downstream same-block instruction whose
	// capture path overwrites every visible bit (an x86FlagOpArith
	// producer reachable with no in-between flag consumer). Logic-kind
	// producers preserve prior AF, so they do NOT fully shadow upstream
	// arith producers. The flag-capture skip uses this to avoid eliding
	// a SUB whose AF a downstream AND would otherwise read back from
	// the captured slot's stale prologue-init value.
	flagShadowed []bool

	// flagCaptureDone is set by an emitter that has already captured
	// guest EFLAGS into the savedEFlags slot before some flag-clobbering
	// bookkeeping (e.g. the post-store self-mod check on memory-dest
	// ALU), so the compile loop's post-instr generic capture must skip
	// this instruction — running it would overwrite the correct
	// guest flags with the bookkeeping-clobbered host EFLAGS. Reset by
	// the compile loop after each instruction.
	flagCaptureDone bool

	// chainExits points at the block compiler's chain-exit list so that
	// emitters which produce a chain exit from inside the block (notably
	// non-self-loop Jcc rel8) can register a patchable slot. nil outside
	// the compile loop. Patched bidirectionally by x86PatchCompatibleChainsTo
	// once the target block is in the cache.
	chainExits *[]x86ChainExitInfo
}

// x86DefaultRegMap returns the Tier 1 fixed register mapping.
func x86DefaultRegMap() [8]byte {
	return [8]byte{
		x86AMD64RegGuestEAX, // EAX -> RBX
		x86AMD64RegGuestECX, // ECX -> RBP
		x86AMD64RegGuestEDX, // EDX -> R12
		x86AMD64RegGuestEBX, // EBX -> R13
		x86AMD64RegGuestESP, // ESP -> R14
		0,                   // EBP -> spilled
		0,                   // ESI -> spilled
		0,                   // EDI -> spilled
	}
}

// ===========================================================================
// Guest Register Load/Store Helpers
// ===========================================================================

// x86EmitLoadGuestReg32 loads a 32-bit guest register into a host register.
// If the guest reg is mapped, emits a MOV from the mapped host reg.
// If spilled, loads from jitRegs[guestReg] via the context.
func x86EmitLoadGuestReg32(cb *CodeBuffer, dstHost byte, guestReg byte) {
	if hostReg, mapped := x86GuestRegToHost(guestReg); mapped {
		if dstHost != hostReg {
			amd64MOV_reg_reg32(cb, dstHost, hostReg)
		}
		return
	}
	// Spilled: load from [R15 + JITRegsPtr] -> pointer, then [ptr + guestReg*4]
	// Actually, R15 points to the context, and JITRegsPtr is a pointer in the context.
	// We need: load RAX = [R15 + x86CtxOffJITRegsPtr], then load dst = [RAX + guestReg*4]
	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffJITRegsPtr))
	amd64MOV_reg_mem32(cb, dstHost, amd64RAX, int32(guestReg)*4)
}

// x86MarkDirty marks a guest register as modified, requiring store-back at exit.
func x86MarkDirty(guestReg byte) {
	if x86CurrentCS != nil && guestReg < 8 {
		x86CurrentCS.dirtyMask |= 1 << guestReg
	}
}

// x86EmitStoreGuestReg32 stores a 32-bit value from hostSrc into a guest register.
// Also marks the register as dirty in the compile state for selective epilogue store-back.
func x86EmitStoreGuestReg32(cb *CodeBuffer, guestReg byte, hostSrc byte) {
	// Mark dirty for selective store-back at exit
	x86MarkDirty(guestReg)

	if hostReg, mapped := x86GuestRegToHost(guestReg); mapped {
		if hostSrc != hostReg {
			amd64MOV_reg_reg32(cb, hostReg, hostSrc)
		}
		return
	}
	// Spilled: store to [jitRegs + guestReg*4]
	scratch := byte(amd64RAX)
	if hostSrc == amd64RAX {
		scratch = amd64R10
	}
	amd64MOV_reg_mem(cb, scratch, x86AMD64RegCtx, int32(x86CtxOffJITRegsPtr))
	amd64MOV_mem_reg32(cb, scratch, int32(guestReg)*4, hostSrc)
}

// ===========================================================================
// Effective Address Computation + Memory Access Helpers
// ===========================================================================

// x86EmitComputeEA emits code to compute the effective address for a memory
// operand into dstReg (must be a scratch register). Uses the ModR/M and SIB
// bytes from the pre-decoded instruction, plus displacement bytes from memory.
// Returns false if the addressing mode is not supported.
func x86EmitComputeEA(cb *CodeBuffer, ji *X86JITInstr, memory []byte, dstReg byte) bool {
	if !ji.hasModRM {
		return false
	}
	modrm := ji.modrm
	mod := modrm >> 6
	rm := modrm & 7

	if mod == 3 {
		return false // register mode, not memory
	}

	// Find the byte position after opcode+modrm in the instruction
	// We need to locate displacement bytes within memory
	// The modrm byte is at a fixed offset from opcodePC depending on prefixes/opcode size
	modrmPC := x86FindModRMPC(ji)

	if rm == 4 {
		// SIB byte follows ModR/M
		sibPC := modrmPC + 1
		if sibPC >= uint32(len(memory)) {
			return false
		}
		sib := memory[sibPC]
		scale := sib >> 6
		index := (sib >> 3) & 7
		base := sib & 7

		dispPC := sibPC + 1

		if base == 5 && mod == 0 {
			// disp32 only (no base register)
			if dispPC+4 > uint32(len(memory)) {
				return false
			}
			disp32 := readLE32(memory, dispPC)
			amd64MOV_reg_imm32(cb, dstReg, disp32)
		} else {
			// base register
			x86EmitLoadGuestReg32(cb, dstReg, base)
		}

		// Add scaled index (index=4 means no index)
		if index != 4 {
			x86EmitLoadGuestReg32(cb, amd64R11, index)
			if scale > 0 {
				amd64SHL_imm32(cb, amd64R11, scale)
			}
			amd64ALU_reg_reg32(cb, 0x01, dstReg, amd64R11) // ADD dst, R11
		}

		// Add displacement
		if mod == 1 {
			disp8 := int32(int8(memory[dispPC]))
			if disp8 != 0 {
				amd64ALU_reg_imm32_32bit(cb, 0, dstReg, disp8) // ADD dst, disp8
			}
		} else if mod == 2 {
			disp32 := int32(readLE32Signed(memory, dispPC))
			if disp32 != 0 {
				amd64ALU_reg_imm32_32bit(cb, 0, dstReg, disp32)
			}
		}
	} else if rm == 5 && mod == 0 {
		// [disp32] -- absolute address
		dispPC := modrmPC + 1
		if dispPC+4 > uint32(len(memory)) {
			return false
		}
		disp32 := readLE32(memory, dispPC)
		amd64MOV_reg_imm32(cb, dstReg, disp32)
	} else {
		// [reg], [reg+disp8], or [reg+disp32]
		x86EmitLoadGuestReg32(cb, dstReg, rm)

		dispPC := modrmPC + 1
		if mod == 1 {
			disp8 := int32(int8(memory[dispPC]))
			if disp8 != 0 {
				amd64ALU_reg_imm32_32bit(cb, 0, dstReg, disp8)
			}
		} else if mod == 2 {
			disp32 := int32(readLE32Signed(memory, dispPC))
			if disp32 != 0 {
				amd64ALU_reg_imm32_32bit(cb, 0, dstReg, disp32)
			}
		}
	}

	// Mask to 25-bit address space

	return true
}

// x86FindModRMPC returns the absolute memory address of the ModR/M byte.
func x86FindModRMPC(ji *X86JITInstr) uint32 {
	pc := ji.opcodePC
	// Skip prefixes
	opcode := ji.opcode
	if opcode >= 0x0F00 {
		// Two-byte opcode: prefixes + 0x0F + opcode2 + modrm
		// modrm is at opcodePC + (length - instruction_body_after_modrm)
		// Simpler: count prefix bytes, then skip opcode bytes
		return ji.opcodePC + uint32(ji.length) - x86ModRMBodyLen(ji)
	}
	// Single-byte opcode: prefixes + opcode + modrm
	_ = pc
	return ji.opcodePC + uint32(ji.length) - x86ModRMBodyLen(ji)
}

// x86ModRMBodyLen returns the number of bytes from (and including) the ModR/M byte
// to the end of the instruction.
func x86ModRMBodyLen(ji *X86JITInstr) uint32 {
	if !ji.hasModRM {
		return 0
	}
	modrm := ji.modrm
	mod := modrm >> 6
	rm := modrm & 7

	n := uint32(1) // ModR/M byte itself

	if mod != 3 {
		if rm == 4 {
			n++ // SIB byte
			if mod == 0 {
				sib_base := modrm // we need the actual SIB byte, not modrm
				// We don't have the SIB byte cached. Approximate: just use the
				// displacement size from the length calculator.
				_ = sib_base
			}
		}
		switch mod {
		case 0:
			if rm == 5 {
				n += 4 // disp32
			} else if rm == 4 {
				// SIB: base=5 might add disp32, but we handle that separately
			}
		case 1:
			n += 1 // disp8
		case 2:
			n += 4 // disp32
		}
	}

	// Add immediate size based on opcode
	op := byte(ji.opcode)
	if ji.opcode >= 0x0F00 {
		op = byte(ji.opcode)
		// Most 0x0F opcodes with modrm have no immediate
		switch op {
		case 0xBA: // Grp8 imm8
			n++
		case 0xA4, 0xAC: // SHLD/SHRD imm8
			n++
		}
	} else {
		switch op {
		case 0x80, 0x82, 0xC0, 0xC1, 0x6B, 0xC6: // +imm8
			n++
		case 0x81, 0x69, 0xC7: // +imm32
			n += 4
		case 0x83: // +imm8
			n++
		case 0xF6: // Grp3 Eb - TEST has imm8
			if (modrm>>3)&7 <= 1 {
				n++
			}
		case 0xF7: // Grp3 Ev - TEST has imm32
			if (modrm>>3)&7 <= 1 {
				n += 4
			}
		}
	}

	return n
}

// readLE32 reads a little-endian uint32 from memory at pc.
func readLE32(memory []byte, pc uint32) uint32 {
	return uint32(memory[pc]) | uint32(memory[pc+1])<<8 | uint32(memory[pc+2])<<16 | uint32(memory[pc+3])<<24
}

// readLE32Signed reads a little-endian int32 from memory at pc.
func readLE32Signed(memory []byte, pc uint32) int32 {
	return int32(readLE32(memory, pc))
}

// x86DeferredBail records a deferred bail site to be resolved at end of block.
type x86DeferredBail struct {
	jccOffset int    // offset of the Jcc rel32 displacement in CodeBuffer
	retPC     uint32 // guest PC to return to
	instrIdx  int    // instruction count at bail point
	kind      byte   // 0 = IO bail, 1 = self-mod bail
}

// x86TryConstantEA returns (address, true) if the instruction's EA is a compile-time
// constant (mod=0, rm=5 = [disp32]). Returns (0, false) otherwise.
func x86TryConstantEA(ji *X86JITInstr, memory []byte) (uint32, bool) {
	if !ji.hasModRM {
		return 0, false
	}
	mod := ji.modrm >> 6
	rm := ji.modrm & 7
	if mod == 0 && rm == 5 {
		// [disp32] -- absolute constant address
		modrmPC := ji.opcodePC + uint32(ji.length) - x86ModRMBodyLen(ji)
		dispPC := modrmPC + 1 // past ModR/M byte
		if dispPC+4 <= uint32(len(memory)) {
			addr := readLE32(memory, dispPC)
			return addr, true
		}
	}
	return 0, false
}

// x86EmitIOCheckMaybeElide emits an IO check only if the EA is not provably safe.
// If constAddr is provided and the page is safe at compile time, skips the check.
func x86EmitIOCheckMaybeElide(cb *CodeBuffer, addrReg byte, ji *X86JITInstr, memory []byte, instrIdx int) {
	if addr, isConst := x86TryConstantEA(ji, memory); isConst {
		if x86IsPageSafeAtCompileTime(addr) {
			return // compile-time safe -- no runtime check needed
		}
	}
	x86EmitIOCheck(cb, addrReg, ji.opcodePC, instrIdx)
}

// x86EmitSelfModCheckMaybeElide emits a self-mod check only if the EA might be on a code page.
func x86EmitSelfModCheckMaybeElide(cb *CodeBuffer, addrReg byte, ji *X86JITInstr, memory []byte, nextPC uint32, instrCount int) {
	if addr, isConst := x86TryConstantEA(ji, memory); isConst {
		if !x86IsCodePageAtCompileTime(addr) {
			return // compile-time: not a code page -- no check needed
		}
	}
	x86EmitSelfModCheck(cb, addrReg, nextPC, instrCount)
}

// x86IsPageSafeAtCompileTime checks if a given address is on a non-I/O page
// using the compile-time I/O bitmap. Returns true if safe (no runtime check needed).
func x86IsPageSafeAtCompileTime(addr uint32) bool {
	cs := x86CurrentCS
	if cs == nil || cs.ioBitmap == nil {
		return false
	}
	page := addr >> 8
	if page < uint32(len(cs.ioBitmap)) {
		return cs.ioBitmap[page] == 0
	}
	return false
}

// x86IsCodePageAtCompileTime checks if a given address is on a code page.
func x86IsCodePageAtCompileTime(addr uint32) bool {
	cs := x86CurrentCS
	if cs == nil || cs.codeBitmap == nil {
		return false // conservative: assume it might be code
	}
	page := addr >> 8
	if page < uint32(len(cs.codeBitmap)) {
		return cs.codeBitmap[page] != 0
	}
	return false
}

// x86EmitIOCheck emits an I/O bitmap check for the address in addrReg.
// If the page is marked as I/O, jumps to a deferred bail stub (emitted later).
func x86EmitIOCheck(cb *CodeBuffer, addrReg byte, retPC uint32, instrCount int) {
	jccOff, ok := emitAMD64FastPathBitmapProbe(cb, FPBitmapMMIO, x86AMD64RegIOBM, addrReg, amd64RCX, amd64RCX, true)
	if !ok {
		panic("x86 I/O bitmap probe unavailable")
	}

	// Record deferred bail for later resolution
	if x86CurrentBails != nil {
		*x86CurrentBails = append(*x86CurrentBails, x86DeferredBail{
			jccOffset: jccOff, retPC: retPC, instrIdx: instrCount, kind: 0,
		})
	}
	// Fast path continues inline (no jump-over needed)
}

// x86CurrentBails collects deferred bail sites during block compilation.
var x86CurrentBails *[]x86DeferredBail

// x86EmitDeferredBails emits the shared slow path stubs at the end of the block.
// Each bail site gets a tiny stub (write RetPC/RetCount) that falls through to
// a single shared exit sequence.
func x86EmitDeferredBails(cb *CodeBuffer) {
	if x86CurrentBails == nil || len(*x86CurrentBails) == 0 {
		return
	}

	bails := *x86CurrentBails

	// Emit shared exit epilogue at the very end
	// First emit per-bail stubs that set RetPC/RetCount then JMP to shared exit
	var sharedExitJmps []int

	for i := range bails {
		bail := &bails[i]
		stubLabel := cb.Len()
		// Patch the Jcc to jump here
		patchRel32(cb, bail.jccOffset, stubLabel)

		// Write RetPC and RetCount
		x86EmitRetPC(cb, bail.retPC, uint32(bail.instrIdx))

		// Set the appropriate flag
		if bail.kind == 0 {
			amd64MOV_mem_imm32(cb, x86AMD64RegCtx, int32(x86CtxOffNeedIOFallback), 1)
		} else {
			amd64MOV_mem_imm32(cb, x86AMD64RegCtx, int32(x86CtxOffNeedInval), 1)
		}

		// JMP to shared exit
		jmpOff := amd64JMP_rel32(cb)
		sharedExitJmps = append(sharedExitJmps, jmpOff)
	}

	// Shared exit: lightweight epilogue + full exit (emitted once)
	sharedExitLabel := cb.Len()
	for _, jmpOff := range sharedExitJmps {
		patchRel32(cb, jmpOff, sharedExitLabel)
	}
	x86EmitLightweightEpilogue(cb)
	x86EmitFullEpilogueEnd(cb)
}

// x86EmitSelfModCheck emits a self-modification check after a memory store.
// addrReg holds the (already masked) address that was written to.
// If the page is marked as code, defers to the shared bail exit.
func x86EmitSelfModCheck(cb *CodeBuffer, addrReg byte, nextPC uint32, instrCount int) {
	// Load code page bitmap pointer from context
	amd64MOV_reg_mem(cb, amd64RCX, x86AMD64RegCtx, int32(x86CtxOffCodePageBitmapPtr))

	// TEST BYTE [RCX + addr>>8], 1
	amd64MOV_reg_reg32(cb, amd64R11, addrReg)
	amd64SHR_imm32(cb, amd64R11, 8)

	emitREX_SIB(cb, false, 0, amd64R11, amd64RCX)
	cb.EmitBytes(0xF6, modRM(0, 0, 4), sibByte(0, amd64R11, amd64RCX))
	cb.EmitBytes(0x01)

	// JNZ to deferred self-mod bail
	jccOff := amd64Jcc_rel32(cb, amd64CondNE)

	if x86CurrentBails != nil {
		*x86CurrentBails = append(*x86CurrentBails, x86DeferredBail{
			jccOffset: jccOff, retPC: nextPC, instrIdx: instrCount, kind: 1,
		})
	}
}

// x86EmitMemLoad32 emits a 32-bit load from [memBase + addrReg] into dstReg.
func x86EmitMemLoad32(cb *CodeBuffer, dstReg byte, addrReg byte) {
	// MOV dst32, [RSI + addr]
	emitREX_SIB(cb, false, dstReg, addrReg, x86AMD64RegMemBase)
	cb.EmitBytes(0x8B, modRM(0, dstReg, 4), sibByte(0, addrReg, x86AMD64RegMemBase))
}

// x86EmitMemStore32 emits a 32-bit store of srcReg to [memBase + addrReg].
func x86EmitMemStore32(cb *CodeBuffer, addrReg byte, srcReg byte) {
	emitREX_SIB(cb, false, srcReg, addrReg, x86AMD64RegMemBase)
	cb.EmitBytes(0x89, modRM(0, srcReg, 4), sibByte(0, addrReg, x86AMD64RegMemBase))
}

// x86EmitMemLoad8 emits an 8-bit load (zero-extended to 32) from [memBase + addrReg] into dstReg.
func x86EmitMemLoad8(cb *CodeBuffer, dstReg byte, addrReg byte) {
	emitREX_SIB(cb, false, dstReg, addrReg, x86AMD64RegMemBase)
	cb.EmitBytes(0x0F, 0xB6, modRM(0, dstReg, 4), sibByte(0, addrReg, x86AMD64RegMemBase))
}

// x86EmitMemStore8 emits an 8-bit store of low byte of srcReg to [memBase + addrReg].
func x86EmitMemStore8(cb *CodeBuffer, addrReg byte, srcReg byte) {
	// Need to use a register that has an 8-bit encoding without REX conflicts
	// If srcReg is RAX/RCX/RDX/RBX, we can use the low byte directly
	emitREX_SIB(cb, false, srcReg, addrReg, x86AMD64RegMemBase)
	cb.EmitBytes(0x88, modRM(0, srcReg, 4), sibByte(0, addrReg, x86AMD64RegMemBase))
}

// x86EmitMemLoad16 emits a 16-bit load (zero-extended) from [memBase + addrReg] into dstReg.
func x86EmitMemLoad16(cb *CodeBuffer, dstReg byte, addrReg byte) {
	emitREX_SIB(cb, false, dstReg, addrReg, x86AMD64RegMemBase)
	cb.EmitBytes(0x0F, 0xB7, modRM(0, dstReg, 4), sibByte(0, addrReg, x86AMD64RegMemBase))
}

// ===========================================================================
// Prologue / Epilogue
// ===========================================================================

// x86EmitInitFlagsSlot loads the guest cpu.Flags into the captured-EFLAGS
// stack slot at prologue time. If no flag-modifying guest instruction
// runs before block exit, the slot still holds the original value and
// the exit-time merge writes it back unchanged. Caller must have set up
// the stack frame (RSP -= frameSize) and have RDI / R15 = JITContext.
func x86EmitInitFlagsSlot(cb *CodeBuffer) {
	// RAX = FlagsPtr; EAX = *FlagsPtr; [RSP+slot] = EAX.
	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffFlagsPtr))
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RAX, 0)
	amd64MOV_mem_reg32(cb, amd64RSP, int32(x86AMD64OffSavedEFlags), amd64RAX)
}

// x86FlagOpKind classifies how an x86 instruction updates the visible
// EFLAGS subset. Drives whether the per-instruction capture path
// preserves AF (Intel-undefined for logic / shift / rotate ops) or
// adopts the host's computed AF (well-defined for arith ops).
type x86FlagOpKind int

const (
	x86FlagOpNone  x86FlagOpKind = iota // does not modify visible flags
	x86FlagOpArith                      // ADD/SUB/CMP/NEG/INC/DEC/MUL/DIV — AF defined
	x86FlagOpLogic                      // AND/OR/XOR/TEST/SHL/SHR/SAR/ROL/ROR — AF undefined; preserve guest's prior AF
	x86FlagOpManip                      // CLC/STC/CMC/CLD/STD — direct manipulation; full host EFLAGS captured
)

// x86EmitCaptureFlagsArith emits the per-instruction capture for an
// arithmetic op that has well-defined effects on every visible flag
// (AF included). Sequence: PUSHFQ; POP RAX; MOV [RSP+slot], EAX. Must
// be emitted immediately after the guest instruction so host EFLAGS
// still reflects that instruction's outputs.
func x86EmitCaptureFlagsArith(cb *CodeBuffer) {
	cb.EmitBytes(0x9C) // PUSHFQ
	amd64POP(cb, amd64RAX)
	amd64MOV_mem_reg32(cb, amd64RSP, int32(x86AMD64OffSavedEFlags), amd64RAX)
}

// x86EmitCaptureFlagsLogic emits the capture sequence for an op whose
// AF is Intel-undefined (AND/OR/XOR/TEST and the shift/rotate family).
// The interpreter leaves guest AF unchanged for these ops, so the JIT
// must do the same: copy guest's prior AF (carried in the existing
// savedEFlags slot) into the new capture, then store.
//
// The AND/OR/AND sequence used to merge AF clobbers host EFLAGS, which
// breaks downstream Jcc / SETcc / CMOVcc consumers that expect the
// host's logic-op flag output to still be live. The capture restores
// host EFLAGS via PUSH/POPFQ at the end, with AF replaced by the guest's
// prior value — that matches the IE x86 interpreter's "AF preserved
// after AND/OR/XOR/SHL/SHR/ROL/ROR" policy and keeps the live-flag
// fast path correct.
func x86EmitCaptureFlagsLogic(cb *CodeBuffer) {
	const afBit = int32(0x10)
	cb.EmitBytes(0x9C)     // PUSHFQ
	amd64POP(cb, amd64RAX) // RAX = host EFLAGS (AF undefined)
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, int32(x86AMD64OffSavedEFlags))
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, ^afBit) // AND EAX, ~AF
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, afBit)  // AND ECX, AF
	amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)  // OR EAX, ECX (preserve prior AF)
	amd64MOV_mem_reg32(cb, amd64RSP, int32(x86AMD64OffSavedEFlags), amd64RAX)
	// Restore host EFLAGS so the live-flag fast path (Jcc/SETcc/CMOVcc
	// chained off this logic op) still sees the right ZF/SF/PF/CF/OF.
	// AF in the restored state reflects the guest's prior value, which
	// matches what a Jcc-on-AF (e.g. JP after TEST) would expect under
	// the IE interpreter's preserve-AF policy.
	amd64PUSH(cb, amd64RAX)
	cb.EmitBytes(0x9D) // POPFQ
}

// x86EmitMergeFlagsToGuest writes the captured-EFLAGS stack slot into
// guest cpu.Flags, preserving non-visible bits (system flags). Called at
// the top of every block-exit path BEFORE any host ALU teardown that
// would clobber EFLAGS or RAX/RCX/RDX. The merge sequence itself
// clobbers host EFLAGS via AND/OR — that is fine because the merge
// runs strictly after the last guest flag-consumer in the block.
func x86EmitMergeFlagsToGuest(cb *CodeBuffer) {
	// EAX = captured visible bits.
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(x86AMD64OffSavedEFlags))
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, int32(x86VisibleFlagsMask)) // AND EAX, mask
	// RDX = &cpu.Flags; ECX = current Flags; ECX &= ~mask; ECX |= EAX; *Flags = ECX.
	amd64MOV_reg_mem(cb, amd64RDX, x86AMD64RegCtx, int32(x86CtxOffFlagsPtr))
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RDX, 0)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, x86InvVisibleFlagsMaskI32) // AND ECX, ~mask
	amd64ALU_reg_reg32(cb, 0x09, amd64RCX, amd64RAX)                     // OR ECX, EAX
	amd64MOV_mem_reg32(cb, amd64RDX, 0, amd64RCX)
}

// x86EmitRestoreGuestCF installs the guest CF (bit 0 of the savedEFlags
// stack slot) into host RFLAGS' CF, leaving other host flags clobbered
// (BT loads CF only; ZF/SF/PF/AF/OF undefined per Intel SDM). Called
// before flag-consuming arith ops (ADC, SBB, RCL, RCR) whose only
// EFLAGS input is CF: the JIT bookkeeping between the previous flag-
// producer and the consumer (chain-budget DEC, NeedInval CMP, the flag-
// capture sequence itself, etc.) clobbers host RFLAGS, so the live
// host CF is stale by the time ADC/SBB/RCL/RCR runs. Without this
// restore, the operation reads arbitrary host CF and the guest's
// architectural CF input is silently lost.
//
// Encoding: BT DWORD PTR [RSP+slot], 0  =  0x0F 0xBA /4 [SIB+disp32] imm8.
//
//	ModRM = 0x84 (mod=10, reg=4 (/4), r/m=100 → SIB),
//	SIB   = 0x24 (scale=00, index=100 (none), base=100 (RSP)),
//	disp32 = OffSavedEFlags (little-endian),
//	imm8 = 0 (bit index for CF).
func x86EmitRestoreGuestCF(cb *CodeBuffer) {
	cb.EmitBytes(0x0F, 0xBA, 0x84, 0x24)
	cb.Emit32(uint32(x86AMD64OffSavedEFlags))
	cb.EmitBytes(0x00)
}

// x86InstrFlagOpKind classifies which capture variant the per-
// instruction flag-capture sequence should emit. Returns x86FlagOpNone
// for ops that do not modify the visible EFLAGS subset.
//
// AF semantics (Intel SDM Vol 1 §3.4.3.1):
//   - Arith ops (ADD/SUB/CMP/NEG/INC/DEC/MUL/DIV): AF defined.
//   - Logic ops (AND/OR/XOR/TEST/SHL/SHR/SAR/ROL/ROR/RCL/RCR): AF
//     undefined — the IE x86 interpreter preserves prior AF, so the
//     JIT capture must too (x86FlagOpLogic).
//   - Direct manipulation (CLC/STC/CMC/CLD/STD): manipulate specific
//     bits; full host EFLAGS capture is correct.
//
// Group-encoded ops (Grp1 0x80/0x81/0x82/0x83, Grp2 0xC0/0xC1/0xD*,
// Grp3 0xF6/0xF7, Grp4/5 0xFE/0xFF) carry the actual op kind in the
// ModR/M reg field. The decoded modrm is required for accurate
// classification; callers pass it via the modrm parameter.
func x86InstrFlagOpKind(opcode uint16, modrm byte) x86FlagOpKind {
	if opcode >= 0x0F00 {
		op2 := byte(opcode)
		switch {
		case op2 == 0xAF: // IMUL Gv, Ev — defined for CF/OF; AF undefined
			return x86FlagOpLogic
		case op2 >= 0xBC && op2 <= 0xBD: // BSF, BSR — only ZF defined
			return x86FlagOpLogic
		}
		return x86FlagOpNone
	}
	op := byte(opcode)
	switch op {
	// ADD/OR/AND/SUB/XOR/CMP r/m, r and r, r/m
	case 0x01, 0x03, 0x29, 0x2B, 0x39, 0x3B:
		return x86FlagOpArith // ADD/SUB/CMP
	case 0x09, 0x0B, 0x21, 0x23, 0x31, 0x33:
		return x86FlagOpLogic // OR/AND/XOR
	// ADC/SBB r/m forms — arith with carry-in. They read CF and define
	// every visible flag, so the post-instr capture must run; without
	// it the epilogue merges stale guest EFLAGS and any downstream
	// flag consumer (in this block or a chained continuation) sees
	// pre-ADC/SBB flags.
	case 0x10, 0x11, 0x12, 0x13: // ADC
		return x86FlagOpArith
	case 0x18, 0x19, 0x1A, 0x1B: // SBB
		return x86FlagOpArith
	// ALU EAX, imm32 / AL, imm8
	case 0x05, 0x2D, 0x3D, 0x04, 0x2C, 0x3C:
		return x86FlagOpArith // ADD/SUB/CMP imm
	// ADC/SBB AL,Ib (0x14, 0x1C) and EAX,Iv (0x15, 0x1D) — arith
	// flag producers. Same rationale as the r/m forms above.
	case 0x14, 0x15: // ADC AL/EAX, imm
		return x86FlagOpArith
	case 0x1C, 0x1D: // SBB AL/EAX, imm
		return x86FlagOpArith
	case 0x0D, 0x25, 0x35, 0x0C, 0x24, 0x34:
		return x86FlagOpLogic // OR/AND/XOR imm
	// INC/DEC r32 — AF defined.
	case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47,
		0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F:
		return x86FlagOpArith
	// TEST — logic semantics.
	case 0x84, 0x85, 0xA8, 0xA9:
		return x86FlagOpLogic
	// String-op compares with REP/REPE/REPNE prefix (CMPSB/CMPSD/SCASB/SCASD).
	// Each iteration's CMP defines the visible flags arith-style.
	case 0xA6, 0xA7, 0xAE, 0xAF:
		return x86FlagOpArith
	// POPF / SAHF — direct manipulation, full host EFLAGS capture works.
	case 0x9D, 0x9E:
		return x86FlagOpManip
	// CMC/CLC/STC/CLD/STD — direct flag bits.
	case 0xF5, 0xF8, 0xF9, 0xFC, 0xFD:
		return x86FlagOpManip
	// Grp1 r/m, imm — sub-op selects ADD(0)/OR(1)/ADC(2)/SBB(3)/AND(4)/SUB(5)/XOR(6)/CMP(7).
	case 0x80, 0x81, 0x82, 0x83:
		sub := (modrm >> 3) & 7
		switch sub {
		case 0, 2, 3, 5, 7: // ADD, ADC, SBB, SUB, CMP
			return x86FlagOpArith
		case 1, 4, 6: // OR, AND, XOR
			return x86FlagOpLogic
		}
		return x86FlagOpArith
	// Grp2 shifts — ROL(0)/ROR(1)/RCL(2)/RCR(3)/SHL(4)/SHR(5)/SAR(7).
	// All have undefined AF; treat as logic (preserve prior AF).
	case 0xC0, 0xC1, 0xD0, 0xD1, 0xD2, 0xD3:
		return x86FlagOpLogic
	// Grp3 — TEST(0/1) / NOT(2) / NEG(3) / MUL(4) / IMUL(5) / DIV(6) / IDIV(7).
	case 0xF6, 0xF7:
		sub := (modrm >> 3) & 7
		switch sub {
		case 0, 1: // TEST
			return x86FlagOpLogic
		case 2: // NOT — does not modify flags
			return x86FlagOpNone
		case 3: // NEG — arith
			return x86FlagOpArith
		case 4, 5, 6, 7: // MUL/IMUL/DIV/IDIV — defined CF/OF; AF undefined
			return x86FlagOpLogic
		}
		return x86FlagOpArith
	// Grp4/5 — INC(0)/DEC(1)/CALL/JMP/PUSH variants. INC/DEC are arith.
	case 0xFE, 0xFF:
		sub := (modrm >> 3) & 7
		switch sub {
		case 0, 1:
			return x86FlagOpArith
		}
		return x86FlagOpNone
	}
	return x86FlagOpNone
}

func x86EmitPrologue(cb *CodeBuffer, cs *x86CompileState) {
	// Save callee-saved registers
	amd64PUSH(cb, amd64RBX)
	amd64PUSH(cb, amd64RBP)
	amd64PUSH(cb, amd64R12)
	amd64PUSH(cb, amd64R13)
	amd64PUSH(cb, amd64R14)
	amd64PUSH(cb, amd64R15)

	// Allocate stack frame
	amd64ALU_reg_imm32(cb, 5, amd64RSP, int32(x86AMD64FrameSize)) // SUB RSP, 40

	// Save JITContext pointer to R15 (callee-saved)
	amd64MOV_reg_reg(cb, x86AMD64RegCtx, amd64RDI) // R15 = RDI

	// Load base pointers from X86JITContext
	amd64MOV_reg_mem(cb, x86AMD64RegMemBase, x86AMD64RegCtx, int32(x86CtxOffMemPtr))
	amd64MOV_reg_mem(cb, x86AMD64RegIOBM, x86AMD64RegCtx, int32(x86CtxOffIOBitmapPtr))

	// Load mapped guest registers from jitRegs array using the compile state's mapping
	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffJITRegsPtr))
	for guest := byte(0); guest < 8; guest++ {
		if host := cs.regMap[guest]; host != 0 {
			amd64MOV_reg_mem32(cb, host, amd64RAX, int32(guest)*4)
		}
	}

	// Initialize loop counters if this is a self-loop block
	if cs.isLoop {
		amd64MOV_mem_imm32(cb, amd64RSP, int32(x86AMD64OffLoopBudget), x86LoopBudget)
		amd64MOV_mem_imm32(cb, amd64RSP, int32(x86AMD64OffLoopRetired), 0)
	}

	// Seed the captured-EFLAGS stack slot with the current guest Flags so
	// blocks that exit before any flag-modifying instruction runs round-
	// trip the value unchanged through x86EmitMergeFlagsToGuest.
	x86EmitInitFlagsSlot(cb)
}

func x86EmitEpilogue(cb *CodeBuffer, cs *x86CompileState) {
	// Materialize captured guest Flags before any teardown ALU clobbers
	// EFLAGS / RAX / RCX / RDX. The merge writes the visible-bits subset
	// (CF/PF/AF/ZF/SF/OF) from the per-instr capture stack slot into
	// *cpu.Flags, preserving non-visible system bits.
	x86EmitMergeFlagsToGuest(cb)

	// Store only dirty mapped guest registers back to jitRegs
	dirty := cs.dirtyMask
	needStore := false
	for guest := byte(0); guest < 8; guest++ {
		if cs.regMap[guest] != 0 && dirty&(1<<guest) != 0 {
			needStore = true
			break
		}
	}
	if needStore {
		amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffJITRegsPtr))
		for guest := byte(0); guest < 8; guest++ {
			if host := cs.regMap[guest]; host != 0 && dirty&(1<<guest) != 0 {
				amd64MOV_mem_reg32(cb, amd64RAX, int32(guest)*4, host)
			}
		}
	}

	// Deallocate stack frame
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(x86AMD64FrameSize)) // ADD RSP, 40

	// Restore callee-saved registers
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)

	amd64RET(cb)
}

// x86EmitRetPC writes RetPC and RetCount to the JITContext.
func x86EmitRetPC(cb *CodeBuffer, pc uint32, count uint32) {
	amd64MOV_mem_imm32(cb, x86AMD64RegCtx, int32(x86CtxOffRetPC), pc)
	amd64MOV_mem_imm32(cb, x86AMD64RegCtx, int32(x86CtxOffRetCount), count)
}

// ===========================================================================
// Instruction Emitters
// ===========================================================================

// x86EmitInstruction emits native code for a single x86 guest instruction.
// Returns true if the instruction was compiled, false if it needs fallback.
func x86EmitInstruction(cb *CodeBuffer, ji *X86JITInstr, memory []byte, startPC uint32, cs *x86CompileState, instrIdx int) bool {
	opcode := ji.opcode

	// Handle two-byte opcodes (0x0F xx) first to avoid low-byte collisions
	if opcode >= 0x0F00 {
		op2 := byte(opcode)
		switch {
		case op2 == 0xB6:
			return x86EmitMOVZX_Gv_Eb(cb, ji)
		case op2 == 0xB7:
			return x86EmitMOVZX_Gv_Ew(cb, ji)
		case op2 == 0xBE:
			return x86EmitMOVSX_Gv_Eb(cb, ji)
		case op2 == 0xBF:
			return x86EmitMOVSX_Gv_Ew(cb, ji)
		case op2 == 0xAF:
			return x86EmitIMUL_Gv_Ev(cb, ji, cs)

		// SETcc (0x0F 90-9F) -- register mode only
		case op2 >= 0x90 && op2 <= 0x9F:
			return x86EmitSETcc(cb, ji, op2-0x90, cs)

		// CMOVcc (0x0F 40-4F) -- register mode only
		case op2 >= 0x40 && op2 <= 0x4F:
			return x86EmitCMOVcc(cb, ji, op2-0x40, cs)

		// BSF (0x0F BC), BSR (0x0F BD)
		case op2 == 0xBC || op2 == 0xBD:
			return x86EmitBSx(cb, ji, op2, cs)
		}
		return false
	}

	op := byte(opcode)

	switch {
	// NOP
	case op == 0x90:
		// No-op: nothing to emit
		return true

	// MOV r32, imm32 (0xB8-0xBF) / MOV r16, imm16 with 0x66 prefix
	case op >= 0xB8 && op <= 0xBF:
		if ji.prefixes&x86PrefOpSize != 0 {
			return x86EmitMOV_r16_imm16(cb, ji, memory)
		}
		return x86EmitMOV_r32_imm32(cb, ji, memory)

	// MOV AL, moffs8 (0xA0) / MOV moffs8, AL (0xA2)
	case op == 0xA2:
		return x86EmitMOV_moffs8_AL(cb, ji, memory, instrIdx)

	// MOV EAX, moffs32 (0xA1) / MOV moffs32, EAX (0xA3)
	case op == 0xA1:
		return x86EmitMOV_EAX_moffs32(cb, ji, memory, instrIdx)
	case op == 0xA3:
		return x86EmitMOV_moffs32_EAX(cb, ji, memory, instrIdx)

	// MOV r8, imm8 (0xB0-0xB7)
	case op >= 0xB0 && op <= 0xB7:
		return x86EmitMOV_r8_imm8(cb, ji, memory)

	// INC r32 (0x40-0x47)
	case op >= 0x40 && op <= 0x47:
		return x86EmitINC_r32(cb, ji, cs)

	// DEC r32 (0x48-0x4F)
	case op >= 0x48 && op <= 0x4F:
		return x86EmitDEC_r32(cb, ji, cs)

	// ADD Ev, Gv (0x01) / ADD Gv, Ev (0x03)
	case op == 0x01:
		return x86EmitALU_Ev_Gv(cb, ji, 0x01, cs, memory, instrIdx)
	case op == 0x03:
		return x86EmitALU_Gv_Ev(cb, ji, 0, cs, memory, instrIdx) // ADD
	// OR Ev, Gv (0x09) / OR Gv, Ev (0x0B)
	case op == 0x09:
		return x86EmitALU_Ev_Gv(cb, ji, 0x09, cs, memory, instrIdx)
	case op == 0x0B:
		return x86EmitALU_Gv_Ev(cb, ji, 1, cs, memory, instrIdx) // OR
	// AND Ev, Gv (0x21) / AND Gv, Ev (0x23)
	case op == 0x21:
		return x86EmitALU_Ev_Gv(cb, ji, 0x21, cs, memory, instrIdx)
	case op == 0x23:
		return x86EmitALU_Gv_Ev(cb, ji, 4, cs, memory, instrIdx) // AND
	// SUB Ev, Gv (0x29) / SUB Gv, Ev (0x2B)
	case op == 0x29:
		return x86EmitALU_Ev_Gv(cb, ji, 0x29, cs, memory, instrIdx)
	case op == 0x2B:
		return x86EmitALU_Gv_Ev(cb, ji, 5, cs, memory, instrIdx) // SUB
	// XOR Ev, Gv (0x31) / XOR Gv, Ev (0x33)
	case op == 0x31:
		return x86EmitALU_Ev_Gv(cb, ji, 0x31, cs, memory, instrIdx)
	case op == 0x33:
		return x86EmitALU_Gv_Ev(cb, ji, 6, cs, memory, instrIdx) // XOR
	// CMP Ev, Gv (0x39) / CMP Gv, Ev (0x3B)
	case op == 0x39:
		return x86EmitALU_Ev_Gv(cb, ji, 0x39, cs, memory, instrIdx)
	case op == 0x3B:
		return x86EmitALU_Gv_Ev(cb, ji, 7, cs, memory, instrIdx) // CMP

	// MOV Eb, Gb (0x88)
	case op == 0x88:
		return x86EmitMOV_Eb_Gb(cb, ji, memory, instrIdx)

	// MOV Ev, Gv (0x89)
	case op == 0x89:
		return x86EmitMOV_Ev_Gv(cb, ji, memory, instrIdx)
	// MOV Gv, Ev (0x8B) -- reg-reg only for now
	case op == 0x8B:
		return x86EmitMOV_Gv_Ev(cb, ji, memory, instrIdx)

	// LEA Gv, M (0x8D)
	case op == 0x8D:
		return x86EmitLEA(cb, ji, memory)

	// ADD EAX, imm32 (0x05)
	case op == 0x05:
		return x86EmitALU_EAX_imm32(cb, ji, memory, 0, cs) // ADD
	// OR EAX, imm32 (0x0D)
	case op == 0x0D:
		return x86EmitALU_EAX_imm32(cb, ji, memory, 1, cs)
	// AND EAX, imm32 (0x25)
	case op == 0x25:
		return x86EmitALU_EAX_imm32(cb, ji, memory, 4, cs)
	// SUB EAX, imm32 (0x2D)
	case op == 0x2D:
		return x86EmitALU_EAX_imm32(cb, ji, memory, 5, cs)
	// XOR EAX, imm32 (0x35)
	case op == 0x35:
		return x86EmitALU_EAX_imm32(cb, ji, memory, 6, cs)
	// CMP EAX, imm32 (0x3D)
	case op == 0x3D:
		return x86EmitALU_EAX_imm32(cb, ji, memory, 7, cs)

	// Grp1 Ev, Iv (0x81)
	case op == 0x81:
		return x86EmitGrp1_Ev_Iv(cb, ji, memory, cs)

	// Grp1 Ev, Ib (0x83)
	case op == 0x83:
		return x86EmitGrp1_Ev_Ib(cb, ji, memory, cs)

	// CLC/STC/CLD/STD/CLI/STI/CMC
	case op == 0xF8 || op == 0xF9 || op == 0xFC || op == 0xFD ||
		op == 0xFA || op == 0xFB || op == 0xF5:
		return x86EmitFlagManip(cb, ji, cs)

	// XCHG EAX, r32 (0x91-0x97)
	case op >= 0x91 && op <= 0x97:
		return x86EmitXCHG_EAX_r32(cb, ji)

	// CBW/CWDE (0x98), CWD/CDQ (0x99)
	case op == 0x98 || op == 0x99:
		return x86EmitSignExtend(cb, ji)

	// Grp2 shifts: Eb,Ib (0xC0), Ev,Ib (0xC1), Eb,1 (0xD0), Ev,1 (0xD1), Eb,CL (0xD2), Ev,CL (0xD3)
	case op == 0xC1:
		return x86EmitGrp2_Ev_Ib(cb, ji, memory, cs, instrIdx)
	case op == 0xD1:
		return x86EmitGrp2_Ev_1(cb, ji, cs, instrIdx)
	case op == 0xD3:
		return x86EmitGrp2_Ev_CL(cb, ji, cs, instrIdx)

	// Grp3: Eb (0xF6) and Ev (0xF7) -- NOT/NEG/MUL/IMUL/DIV/IDIV + TEST
	case op == 0xF7:
		return x86EmitGrp3_Ev(cb, ji, memory, cs)

	// Grp1 Eb,Ib (0x80/0x82)
	case op == 0x80 || op == 0x82:
		return x86EmitGrp1_Eb_Ib(cb, ji, memory, cs)

	// TEST Eb,Gb (0x84) and TEST Ev,Gv (0x85)
	case op == 0x84:
		return false // 8-bit TEST -- TODO
	case op == 0x85:
		return x86EmitTEST_Ev_Gv(cb, ji, cs)

	// TEST AL,Ib (0xA8)
	case op == 0xA8:
		return x86EmitTEST_AL_Ib(cb, ji, memory, cs)

	// TEST EAX,Iv (0xA9)
	case op == 0xA9:
		return x86EmitTEST_EAX_Iv(cb, ji, memory, cs)

	// ADD AL,Ib (0x04), OR (0x0C), ADC (0x14), SBB (0x1C), AND (0x24), SUB (0x2C), XOR (0x34), CMP (0x3C)
	case op == 0x04 || op == 0x0C || op == 0x14 || op == 0x1C ||
		op == 0x24 || op == 0x2C || op == 0x34 || op == 0x3C:
		return x86EmitALU_AL_Ib(cb, ji, memory, cs)

	// PUSH r32 (0x50-0x57)
	case op >= 0x50 && op <= 0x57:
		return x86EmitPUSH_r32(cb, ji)

	// POP r32 (0x58-0x5F)
	case op >= 0x58 && op <= 0x5F:
		return x86EmitPOP_r32(cb, ji)

	// PUSH imm32 (0x68)
	case op == 0x68:
		return x86EmitPUSH_imm32(cb, ji, memory)

	// PUSH imm8 (0x6A)
	case op == 0x6A:
		return x86EmitPUSH_imm8(cb, ji, memory)

	// MOV Ev,Sw (0x8C) -- read segment register
	case op == 0x8C:
		return x86EmitMOV_Ev_Sw(cb, ji)

	// PUSHF (0x9C) / POPF (0x9D)
	case op == 0x9C:
		return x86EmitPUSHF(cb, ji, cs, instrIdx)

	// SAHF (0x9E) / LAHF (0x9F)
	case op == 0x9E || op == 0x9F:
		return false // TODO

	// LEAVE (0xC9)
	case op == 0xC9:
		return x86EmitLEAVE(cb, ji)

	// LOOP/LOOPE/LOOPNE (0xE0-0xE2)
	case op == 0xE2:
		return x86EmitLOOP(cb, ji, memory, instrIdx)

	// Jcc rel8 (0x70-0x7F) -- conditional branches
	case op >= 0x70 && op <= 0x7F:
		return x86EmitJcc_rel8(cb, ji, memory, startPC, cs, instrIdx)

	// MOV Eb,Ib (0xC6)
	case op == 0xC6:
		return x86EmitMOV_Eb_Ib(cb, ji, memory, instrIdx)

	// MOV Ev,Iv (0xC7)
	case op == 0xC7:
		return x86EmitMOV_Ev_Iv(cb, ji, memory, instrIdx)

	// Grp4 Eb (0xFE) -- INC/DEC byte
	case op == 0xFE:
		return false // 8-bit INC/DEC -- TODO

	// x87 FPU escapes (D8-DF)
	case op >= 0xD8 && op <= 0xDF:
		return x86EmitFPU(cb, ji, memory, instrIdx)

	// MOVS/STOS string ops
	case op == 0xA4: // MOVSB (single or REP)
		if ji.prefixes&x86PrefRep != 0 {
			return x86EmitREP_MOVSB(cb, ji, instrIdx)
		}
		return false
	case op == 0xA5: // MOVSD (single or REP)
		if ji.prefixes&x86PrefRep != 0 {
			return x86EmitREP_MOVSD(cb, ji, instrIdx)
		}
		return false
	case op == 0xA6: // CMPSB (single or REPE/REPNE)
		if ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0 {
			return x86EmitREP_CMPSB(cb, ji, instrIdx, cs)
		}
		return false
	case op == 0xAA: // STOSB (single or REP)
		if ji.prefixes&x86PrefRep != 0 {
			return x86EmitREP_STOSB(cb, ji, instrIdx)
		}
		return false
	case op == 0xAB: // STOSD (single or REP)
		if ji.prefixes&x86PrefRep != 0 {
			return x86EmitREP_STOSD(cb, ji, instrIdx)
		}
		return false
	case op == 0xAE: // SCASB (single or REPE/REPNE)
		if ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0 {
			return x86EmitREP_SCASB(cb, ji, instrIdx, cs)
		}
		return false
	}

	return false // Not yet implemented
}

// ===========================================================================
// MOV Emitters
// ===========================================================================

// x86EmitMOV_r32_imm32 handles MOV r32, imm32 (0xB8-0xBF).
func x86EmitMOV_r32_imm32(cb *CodeBuffer, ji *X86JITInstr, memory []byte) bool {
	guestReg := byte(ji.opcode) - 0xB8
	pc := ji.opcodePC + uint32(ji.length) - 4
	imm := uint32(memory[pc]) | uint32(memory[pc+1])<<8 | uint32(memory[pc+2])<<16 | uint32(memory[pc+3])<<24
	x86MarkDirty(guestReg)

	if hostReg, mapped := x86GuestRegToHost(guestReg); mapped {
		amd64MOV_reg_imm32(cb, hostReg, imm)
	} else {
		// Spilled: load jitRegs pointer, store imm
		amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffJITRegsPtr))
		amd64MOV_mem_imm32(cb, amd64RAX, int32(guestReg)*4, imm)
	}
	return true
}

// x86EmitMOV_r16_imm16 handles MOV r16, imm16 (0xB8-0xBF with 0x66 prefix).
func x86EmitMOV_r16_imm16(cb *CodeBuffer, ji *X86JITInstr, memory []byte) bool {
	guestReg := byte(ji.opcode) - 0xB8
	x86MarkDirty(guestReg)
	immPC := ji.opcodePC + uint32(ji.length) - 2 // imm16 is at end
	imm := uint32(memory[immPC]) | uint32(memory[immPC+1])<<8

	// Load current 32-bit value, clear lower 16, OR in new value
	x86EmitLoadGuestReg32(cb, amd64RAX, guestReg)
	// AND EAX, 0xFFFF0000 (keep upper 16 bits) -- use raw encoding to avoid int32 overflow
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0x81, modRM(3, 4, amd64RAX))
	cb.Emit32(0xFFFF0000)
	amd64ALU_reg_imm32_32bit(cb, 1, amd64RAX, int32(imm)) // OR EAX, imm16
	x86EmitStoreGuestReg32(cb, guestReg, amd64RAX)
	return true
}

// x86EmitMOV_r8_imm8 handles MOV r8, imm8 (0xB0-0xB7).
func x86EmitMOV_r8_imm8(cb *CodeBuffer, ji *X86JITInstr, memory []byte) bool {
	r8 := byte(ji.opcode) - 0xB0
	pc := ji.opcodePC + uint32(ji.length) - 1
	imm := memory[pc]

	// r8 encoding: 0-3 = AL/CL/DL/BL (low byte), 4-7 = AH/CH/DH/BH (high byte)
	guestReg := r8 & 3 // maps to EAX(0)/ECX(1)/EDX(2)/EBX(3)
	isHigh := r8 >= 4

	// Load current 32-bit value into scratch
	x86EmitLoadGuestReg32(cb, amd64RAX, guestReg)

	if isHigh {
		// Set bits 15:8: AND EAX, ~0xFF00; OR EAX, (imm << 8)
		// Use explicit 32-bit constant to avoid int32 overflow
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0x81, modRM(3, 4, amd64RAX)) // AND EAX, imm32
		cb.Emit32(0xFFFF00FF)
		if imm != 0 {
			emitREX(cb, false, 0, amd64RAX)
			cb.EmitBytes(0x81, modRM(3, 1, amd64RAX)) // OR EAX, imm32
			cb.Emit32(uint32(imm) << 8)
		}
	} else {
		// Set bits 7:0: AND EAX, ~0xFF; OR EAX, imm
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0x81, modRM(3, 4, amd64RAX)) // AND EAX, imm32
		cb.Emit32(0xFFFFFF00)
		if imm != 0 {
			amd64ALU_reg_imm32_32bit(cb, 1, amd64RAX, int32(imm)) // OR
		}
	}

	x86EmitStoreGuestReg32(cb, guestReg, amd64RAX)
	return true
}

func x86MoffsDispPC(ji *X86JITInstr) uint32 {
	return ji.opcodePC + uint32(ji.length) - 4
}

func x86EmitMOV_EAX_moffs32(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if ji.length < 5 {
		return false
	}
	addr := readLE32(memory, ji.opcodePC+1)
	amd64MOV_reg_imm32(cb, amd64R10, addr)
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)
	x86EmitMemLoad32(cb, amd64R8, amd64R10)
	x86EmitStoreGuestReg32(cb, 0, amd64R8) // EAX
	return true
}

func x86EmitMOV_moffs32_EAX(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if ji.length < 5 {
		return false
	}
	addr := readLE32(memory, ji.opcodePC+1)
	amd64MOV_reg_imm32(cb, amd64R10, addr)
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)
	x86EmitLoadGuestReg32(cb, amd64R8, 0) // EAX
	x86EmitMemStore32(cb, amd64R10, amd64R8)
	x86EmitSelfModCheckMaybeElide(cb, amd64R10, ji, memory, ji.opcodePC+uint32(ji.length), instrIdx+1)
	return true
}

func x86EmitMOV_moffs8_AL(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if ji.length < 5 {
		return false
	}
	addr := readLE32(memory, x86MoffsDispPC(ji))
	amd64MOV_reg_imm32(cb, amd64R10, addr)
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)
	x86EmitLoadGuestReg32(cb, amd64R8, 0) // EAX
	x86EmitMemStore8(cb, amd64R10, amd64R8)
	x86EmitSelfModCheckMaybeElide(cb, amd64R10, ji, memory, ji.opcodePC+uint32(ji.length), instrIdx+1)
	return true
}

// x86EmitMOV_Eb_Gb handles MOV Eb, Gb (0x88) for register and memory modes.
func x86EmitMOV_Eb_Gb(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if !ji.hasModRM {
		return false
	}
	mod := ji.modrm >> 6
	srcR8 := (ji.modrm >> 3) & 7
	srcGuestReg := srcR8 & 3
	srcHigh := srcR8 >= 4

	if mod == 3 {
		dstR8 := ji.modrm & 7
		dstGuestReg := dstR8 & 3
		dstHigh := dstR8 >= 4

		x86EmitLoadGuestReg32(cb, amd64R8, dstGuestReg)
		x86EmitLoadGuestReg32(cb, amd64R9, srcGuestReg)
		if srcHigh {
			amd64SHR_imm32(cb, amd64R9, 8)
		}
		if dstHigh {
			emitREX(cb, false, 0, amd64R8)
			cb.EmitBytes(0x81, modRM(3, 4, amd64R8))
			cb.Emit32(0xFFFF00FF)
			emitREX(cb, false, amd64R9, amd64R9)
			cb.EmitBytes(0x0F, 0xB6, modRM(3, amd64R9, amd64R9))
			amd64SHL_imm32(cb, amd64R9, 8)
		} else {
			emitREX(cb, false, 0, amd64R8)
			cb.EmitBytes(0x81, modRM(3, 4, amd64R8))
			cb.Emit32(0xFFFFFF00)
			emitREX(cb, false, amd64R9, amd64R9)
			cb.EmitBytes(0x0F, 0xB6, modRM(3, amd64R9, amd64R9))
		}
		amd64ALU_reg_reg32(cb, 0x09, amd64R8, amd64R9) // OR
		x86EmitStoreGuestReg32(cb, dstGuestReg, amd64R8)
		return true
	}

	if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
		return false
	}
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)
	x86EmitLoadGuestReg32(cb, amd64R8, srcGuestReg)
	if srcHigh {
		amd64SHR_imm32(cb, amd64R8, 8)
	}
	x86EmitMemStore8(cb, amd64R10, amd64R8)
	x86EmitSelfModCheckMaybeElide(cb, amd64R10, ji, memory, ji.opcodePC+uint32(ji.length), instrIdx+1)
	return true
}

// x86EmitMOV_Ev_Gv handles MOV Ev, Gv (0x89) -- register and memory modes.
func x86EmitMOV_Ev_Gv(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if !ji.hasModRM {
		return false
	}
	mod := ji.modrm >> 6
	srcReg := (ji.modrm >> 3) & 7

	if mod == 3 {
		dstReg := ji.modrm & 7
		x86EmitLoadGuestReg32(cb, amd64R8, srcReg)
		x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
		return true
	}

	// Memory store: MOV [EA], reg
	if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
		return false
	}
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)
	x86EmitLoadGuestReg32(cb, amd64R8, srcReg)
	x86EmitMemStore32(cb, amd64R10, amd64R8)
	x86EmitSelfModCheckMaybeElide(cb, amd64R10, ji, memory, ji.opcodePC+uint32(ji.length), instrIdx+1)
	return true
}

// x86EmitMOV_Gv_Ev handles MOV Gv, Ev (0x8B) -- register and memory modes.
func x86EmitMOV_Gv_Ev(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if !ji.hasModRM {
		return false
	}
	mod := ji.modrm >> 6
	dstReg := (ji.modrm >> 3) & 7

	if mod == 3 {
		srcReg := ji.modrm & 7
		x86EmitLoadGuestReg32(cb, amd64R8, srcReg)
		x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
		return true
	}

	// Memory load: MOV reg, [EA]
	if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
		return false
	}
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)
	x86EmitMemLoad32(cb, amd64R8, amd64R10)
	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// ===========================================================================
// ALU Emitters (register-register, mod=11 only for Tier 1)
// ===========================================================================

// x86EmitALU_Ev_Gv handles ALU Ev, Gv where hostOpcode is the x86 ALU opcode.
// Supports both register and memory destination modes.
func x86EmitALU_Ev_Gv(cb *CodeBuffer, ji *X86JITInstr, hostOpcode byte, cs *x86CompileState, memory []byte, instrIdx int) bool {
	if !ji.hasModRM {
		return false
	}
	mod := ji.modrm >> 6
	srcReg := (ji.modrm >> 3) & 7

	if mod == 3 {
		// Register-register
		dstReg := ji.modrm & 7
		// Fast path: both guest regs mapped to host regs — emit op directly,
		// skip the load-into-scratch-and-store-back roundtrip.
		if hostDst, dstMapped := x86GuestRegToHost(dstReg); dstMapped {
			if hostSrc, srcMapped := x86GuestRegToHost(srcReg); srcMapped {
				emitREX(cb, false, hostSrc, hostDst)
				cb.EmitBytes(hostOpcode, modRM(3, hostSrc, hostDst))
				switch hostOpcode {
				case 0x01, 0x29:
					cs.flagState = x86FlagsLiveArith
				case 0x09, 0x21, 0x31:
					cs.flagState = x86FlagsLiveLogic
				case 0x39:
					cs.flagState = x86FlagsLiveArith
					return true
				}
				x86MarkDirty(dstReg)
				return true
			}
		}
		x86EmitLoadGuestReg32(cb, amd64R8, dstReg)
		x86EmitLoadGuestReg32(cb, amd64R10, srcReg)
		emitREX(cb, false, amd64R10, amd64R8)
		cb.EmitBytes(hostOpcode, modRM(3, amd64R10, amd64R8))

		switch hostOpcode {
		case 0x01, 0x29:
			cs.flagState = x86FlagsLiveArith
		case 0x09, 0x21, 0x31:
			cs.flagState = x86FlagsLiveLogic
		case 0x39:
			cs.flagState = x86FlagsLiveArith
			return true
		}
		x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
		return true
	}

	// Memory destination: ALU [EA], reg
	if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
		return false
	}
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)

	// Load memory value
	x86EmitMemLoad32(cb, amd64R8, amd64R10)
	// Load source register
	x86EmitLoadGuestReg32(cb, amd64R11, srcReg)
	// Perform ALU
	emitREX(cb, false, amd64R11, amd64R8)
	cb.EmitBytes(hostOpcode, modRM(3, amd64R11, amd64R8))

	switch hostOpcode {
	case 0x01, 0x29:
		cs.flagState = x86FlagsLiveArith
	case 0x09, 0x21, 0x31:
		cs.flagState = x86FlagsLiveLogic
	case 0x39:
		cs.flagState = x86FlagsLiveArith
		return true // CMP doesn't store
	}
	// Store result back to memory (MOV — does not modify EFLAGS).
	x86EmitMemStore32(cb, amd64R10, amd64R8)

	// Capture guest EFLAGS NOW, before the SMC check below clobbers
	// them via SHR/TEST. The compile loop's post-instr generic capture
	// runs after this emitter returns and would otherwise record the
	// SMC check's flags instead of the guest ALU's. flagCaptureDone
	// tells the compile loop to skip the generic capture for this
	// instruction.
	switch hostOpcode {
	case 0x01, 0x29: // ADD/SUB
		x86EmitCaptureFlagsArith(cb)
		cs.flagCaptureDone = true
	case 0x09, 0x21, 0x31: // OR/AND/XOR
		x86EmitCaptureFlagsLogic(cb)
		cs.flagCaptureDone = true
	}

	x86EmitSelfModCheckMaybeElide(cb, amd64R10, ji, memory, ji.opcodePC+uint32(ji.length), instrIdx+1)
	return true
}

// x86EmitALU_Gv_Ev handles ALU Gv, Ev (register destination, memory/register source).
// aluOp: 0=ADD, 1=OR, 4=AND, 5=SUB, 6=XOR, 7=CMP
func x86EmitALU_Gv_Ev(cb *CodeBuffer, ji *X86JITInstr, aluOp byte, cs *x86CompileState, memory []byte, instrIdx int) bool {
	if !ji.hasModRM {
		return false
	}
	mod := ji.modrm >> 6
	dstReg := (ji.modrm >> 3) & 7

	// Native ALU opcode for reg,reg: aluOp*8 + 3 (Gv, Ev form)
	nativeOp := aluOp*8 + 3

	if mod == 3 {
		srcReg := ji.modrm & 7
		// Fast path: both guest regs mapped — emit op directly between host regs.
		if hostDst, dstMapped := x86GuestRegToHost(dstReg); dstMapped {
			if hostSrc, srcMapped := x86GuestRegToHost(srcReg); srcMapped {
				emitREX(cb, false, hostDst, hostSrc)
				cb.EmitBytes(nativeOp, modRM(3, hostDst, hostSrc))
				switch aluOp {
				case 0, 5:
					cs.flagState = x86FlagsLiveArith
				case 1, 4, 6:
					cs.flagState = x86FlagsLiveLogic
				case 7:
					cs.flagState = x86FlagsLiveArith
					return true
				}
				x86MarkDirty(dstReg)
				return true
			}
		}
		x86EmitLoadGuestReg32(cb, amd64R8, dstReg)
		x86EmitLoadGuestReg32(cb, amd64R10, srcReg)
		emitREX(cb, false, amd64R8, amd64R10)
		cb.EmitBytes(nativeOp, modRM(3, amd64R8, amd64R10))

		switch aluOp {
		case 0, 5:
			cs.flagState = x86FlagsLiveArith
		case 1, 4, 6:
			cs.flagState = x86FlagsLiveLogic
		case 7:
			cs.flagState = x86FlagsLiveArith
			return true
		}
		x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
		return true
	}

	// Memory source: ALU reg, [EA]
	if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
		return false
	}
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)
	x86EmitMemLoad32(cb, amd64R11, amd64R10)
	x86EmitLoadGuestReg32(cb, amd64R8, dstReg)
	emitREX(cb, false, amd64R8, amd64R11)
	cb.EmitBytes(nativeOp, modRM(3, amd64R8, amd64R11))

	switch aluOp {
	case 0, 5:
		cs.flagState = x86FlagsLiveArith
	case 1, 4, 6:
		cs.flagState = x86FlagsLiveLogic
	case 7:
		cs.flagState = x86FlagsLiveArith
		return true
	}
	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// x86EmitALU_EAX_imm32 handles ADD/OR/AND/SUB/XOR/CMP EAX, imm32.
func x86EmitALU_EAX_imm32(cb *CodeBuffer, ji *X86JITInstr, memory []byte, aluOp byte, cs *x86CompileState) bool {
	pc := ji.opcodePC + 1 // skip opcode byte, account for prefixes
	// Find the immediate position
	immPC := ji.opcodePC + uint32(ji.length) - 4
	imm := int32(int32(memory[immPC]) | int32(memory[immPC+1])<<8 | int32(memory[immPC+2])<<16 | int32(memory[immPC+3])<<24)
	_ = pc

	// Load EAX into scratch
	x86EmitLoadGuestReg32(cb, amd64R8, 0) // guest EAX

	// Perform ALU: R8d op imm32
	amd64ALU_reg_imm32_32bit(cb, aluOp, amd64R8, imm)

	// Update flag state
	switch aluOp {
	case 0, 5: // ADD, SUB
		cs.flagState = x86FlagsLiveArith
	case 1, 4, 6: // OR, AND, XOR
		cs.flagState = x86FlagsLiveLogic
	case 7: // CMP
		cs.flagState = x86FlagsLiveArith
		return true // Don't store result
	}

	x86EmitStoreGuestReg32(cb, 0, amd64R8)
	return true
}

// x86EmitGrp1_Ev_Iv handles Grp1 Ev, Iv (0x81) -- register mode only.
func x86EmitGrp1_Ev_Iv(cb *CodeBuffer, ji *X86JITInstr, memory []byte, cs *x86CompileState) bool {
	if !ji.hasModRM {
		return false
	}
	mod := ji.modrm >> 6
	if mod != 3 {
		return false
	}
	aluOp := (ji.modrm >> 3) & 7
	dstReg := ji.modrm & 7

	immPC := ji.opcodePC + uint32(ji.length) - 4
	imm := int32(int32(memory[immPC]) | int32(memory[immPC+1])<<8 | int32(memory[immPC+2])<<16 | int32(memory[immPC+3])<<24)

	x86EmitLoadGuestReg32(cb, amd64R8, dstReg)
	// ADC/SBB read host CF; reinstall guest CF before the ALU.
	if aluOp == 2 || aluOp == 3 {
		x86EmitRestoreGuestCF(cb)
	}
	amd64ALU_reg_imm32_32bit(cb, aluOp, amd64R8, imm)

	switch aluOp {
	case 0, 5, 2, 3: // ADD, SUB, ADC, SBB
		cs.flagState = x86FlagsLiveArith
	case 1, 4, 6: // OR, AND, XOR
		cs.flagState = x86FlagsLiveLogic
	case 7: // CMP
		cs.flagState = x86FlagsLiveArith
		return true
	}

	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// x86EmitGrp1_Ev_Ib handles Grp1 Ev, Ib (0x83) -- register mode only.
func x86EmitGrp1_Ev_Ib(cb *CodeBuffer, ji *X86JITInstr, memory []byte, cs *x86CompileState) bool {
	if !ji.hasModRM {
		return false
	}
	mod := ji.modrm >> 6
	if mod != 3 {
		return false
	}
	aluOp := (ji.modrm >> 3) & 7
	dstReg := ji.modrm & 7

	immPC := ji.opcodePC + uint32(ji.length) - 1
	imm := int32(int8(memory[immPC])) // sign-extend imm8

	x86EmitLoadGuestReg32(cb, amd64R8, dstReg)
	// ADC/SBB read host CF; reinstall guest CF before the ALU.
	if aluOp == 2 || aluOp == 3 {
		x86EmitRestoreGuestCF(cb)
	}
	amd64ALU_reg_imm32_32bit(cb, aluOp, amd64R8, imm)

	switch aluOp {
	case 0, 5, 2, 3:
		cs.flagState = x86FlagsLiveArith
	case 1, 4, 6:
		cs.flagState = x86FlagsLiveLogic
	case 7:
		cs.flagState = x86FlagsLiveArith
		return true
	}

	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// ===========================================================================
// INC / DEC Emitters
// ===========================================================================

func x86EmitINC_r32(cb *CodeBuffer, ji *X86JITInstr, cs *x86CompileState) bool {
	guestReg := byte(ji.opcode) - 0x40

	// Fast path: guest reg mapped — INC the host reg directly.
	if hostReg, mapped := x86GuestRegToHost(guestReg); mapped {
		emitREX(cb, false, 0, hostReg)
		cb.EmitBytes(0xFF, modRM(3, 0, hostReg))
		cs.flagState = x86FlagsLiveInc
		x86MarkDirty(guestReg)
		return true
	}

	x86EmitLoadGuestReg32(cb, amd64R8, guestReg)

	// INC R8d
	emitREX(cb, false, 0, amd64R8)
	cb.EmitBytes(0xFF, modRM(3, 0, amd64R8)) // /0 = INC

	cs.flagState = x86FlagsLiveInc

	x86EmitStoreGuestReg32(cb, guestReg, amd64R8)
	return true
}

func x86EmitDEC_r32(cb *CodeBuffer, ji *X86JITInstr, cs *x86CompileState) bool {
	guestReg := byte(ji.opcode) - 0x48

	// Fast path: guest reg mapped — DEC the host reg directly.
	if hostReg, mapped := x86GuestRegToHost(guestReg); mapped {
		emitREX(cb, false, 0, hostReg)
		cb.EmitBytes(0xFF, modRM(3, 1, hostReg))
		cs.flagState = x86FlagsLiveInc
		x86MarkDirty(guestReg)
		return true
	}

	x86EmitLoadGuestReg32(cb, amd64R8, guestReg)

	// DEC R8d
	emitREX(cb, false, 0, amd64R8)
	cb.EmitBytes(0xFF, modRM(3, 1, amd64R8)) // /1 = DEC

	cs.flagState = x86FlagsLiveInc

	x86EmitStoreGuestReg32(cb, guestReg, amd64R8)
	return true
}

// ===========================================================================
// VEX Encoding Helpers (for BMI2, AVX2)
// ===========================================================================

// emitVEX3 emits a 3-byte VEX prefix.
// pp: 00=none, 01=66, 10=F3, 11=F2
// mmmmm: opcode map (00001=0F, 00010=0F38, 00011=0F3A)
// W: REX.W equivalent (0 for 32-bit, 1 for 64-bit)
// vvvv: source register (inverted, 4 bits)
// L: vector length (0=128/scalar, 1=256)
// reg: ModR/M reg field register (for REX.R)
// rm: ModR/M r/m field register (for REX.B)
func emitVEX3(cb *CodeBuffer, pp, mmmmm, W, vvvv, L, reg, rm byte) {
	// Byte 0: 0xC4
	cb.EmitBytes(0xC4)
	// Byte 1: ~R.~X.~B.mmmmm
	R := byte(0)
	if isExtReg(reg) {
		R = 1
	}
	B := byte(0)
	if isExtReg(rm) {
		B = 1
	}
	b1 := ((^R & 1) << 7) | (1 << 6) | ((^B & 1) << 5) | (mmmmm & 0x1F)
	cb.EmitBytes(b1)
	// Byte 2: W.~vvvv.L.pp
	b2 := ((W & 1) << 7) | ((^vvvv & 0xF) << 3) | ((L & 1) << 2) | (pp & 3)
	cb.EmitBytes(b2)
}

// emitBMI2Shift emits a BMI2 SHLX/SHRX/SARX instruction (non-flag-affecting shift).
// pp: 01=SHLX(66), 11=SHRX(F2), 10=SARX(F3)
// dst, src: guest register values loaded into host registers
// countReg: host register holding the shift count
func emitBMI2Shift(cb *CodeBuffer, pp byte, dst, src, countReg byte) {
	// VEX.LZ.{pp}.0F38.W0 F7 /r
	emitVEX3(cb, pp, 0x02, 0, countReg, 0, dst, src) // mmmmm=2 = 0F38
	cb.EmitBytes(0xF7, modRM(3, dst, src))
}

// ===========================================================================
// Grp2 Shift/Rotate Emitters
// ===========================================================================

// x86EmitGrp2_Ev_Ib handles Grp2 Ev, Ib (0xC1) -- register mode only.
// Sub-ops: 0=ROL, 1=ROR, 2=RCL, 3=RCR, 4=SHL, 5=SHR, 7=SAR
// When BMI2 is available and flags output is dead, uses SHLX/SHRX/SARX
// (non-flag-affecting) to preserve host EFLAGS across the shift.
func x86EmitGrp2_Ev_Ib(cb *CodeBuffer, ji *X86JITInstr, memory []byte, cs *x86CompileState, instrIdx int) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	shiftOp := (ji.modrm >> 3) & 7
	dstReg := ji.modrm & 7
	immPC := ji.opcodePC + uint32(ji.length) - 1
	imm := memory[immPC]

	x86EmitLoadGuestReg32(cb, amd64R8, dstReg)

	// BMI2 path: use SHLX/SHRX/SARX when flags aren't needed
	flagsDead := instrIdx < len(cs.flagsNeeded) && !cs.flagsNeeded[instrIdx]
	if cs.host.HasBMI2 && flagsDead && (shiftOp == 4 || shiftOp == 5 || shiftOp == 7) {
		// Load shift count into a scratch register
		amd64MOV_reg_imm32(cb, amd64RCX, uint32(imm))
		var pp byte
		switch shiftOp {
		case 4:
			pp = 0x01 // SHLX: VEX.66
		case 5:
			pp = 0x03 // SHRX: VEX.F2
		case 7:
			pp = 0x02 // SARX: VEX.F3
		}
		emitBMI2Shift(cb, pp, amd64R8, amd64R8, amd64RCX)
		// Flags NOT modified -- preserve prior flag state
		x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
		return true
	}

	// Standard path: flags-affecting shift.
	// RCL/RCR (shiftOp 2/3) consume host CF; restore guest CF first since
	// JIT bookkeeping between the prior flag-producer and this rotate
	// clobbers host RFLAGS.
	if shiftOp == 2 || shiftOp == 3 {
		x86EmitRestoreGuestCF(cb)
	}
	emitREX(cb, false, 0, amd64R8)
	cb.EmitBytes(0xC1, modRM(3, shiftOp, amd64R8), imm)

	cs.flagState = x86FlagsLiveArith
	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// x86EmitGrp2_Ev_1 handles Grp2 Ev, 1 (0xD1) -- register mode only.
func x86EmitGrp2_Ev_1(cb *CodeBuffer, ji *X86JITInstr, cs *x86CompileState, instrIdx int) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	shiftOp := (ji.modrm >> 3) & 7
	dstReg := ji.modrm & 7

	// Fast path: dst mapped — shift the host reg in place.
	if hostReg, mapped := x86GuestRegToHost(dstReg); mapped {
		// RCL/RCR consume host CF; restore guest CF first.
		if shiftOp == 2 || shiftOp == 3 {
			x86EmitRestoreGuestCF(cb)
		}
		emitREX(cb, false, 0, hostReg)
		cb.EmitBytes(0xD1, modRM(3, shiftOp, hostReg))
		cs.flagState = x86FlagsLiveArith
		x86MarkDirty(dstReg)
		return true
	}

	x86EmitLoadGuestReg32(cb, amd64R8, dstReg)

	// BMI2 path for SHL/SHR/SAR by 1 when flags dead
	flagsDead := instrIdx < len(cs.flagsNeeded) && !cs.flagsNeeded[instrIdx]
	if cs.host.HasBMI2 && flagsDead && (shiftOp == 4 || shiftOp == 5 || shiftOp == 7) {
		amd64MOV_reg_imm32(cb, amd64RCX, 1)
		var pp byte
		switch shiftOp {
		case 4:
			pp = 0x01
		case 5:
			pp = 0x03
		case 7:
			pp = 0x02
		}
		emitBMI2Shift(cb, pp, amd64R8, amd64R8, amd64RCX)
		x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
		return true
	}

	// RCL/RCR consume host CF; restore guest CF first.
	if shiftOp == 2 || shiftOp == 3 {
		x86EmitRestoreGuestCF(cb)
	}
	emitREX(cb, false, 0, amd64R8)
	cb.EmitBytes(0xD1, modRM(3, shiftOp, amd64R8))

	cs.flagState = x86FlagsLiveArith
	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// x86EmitGrp2_Ev_CL handles Grp2 Ev, CL (0xD3) -- register mode only.
func x86EmitGrp2_Ev_CL(cb *CodeBuffer, ji *X86JITInstr, cs *x86CompileState, instrIdx int) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	shiftOp := (ji.modrm >> 3) & 7
	dstReg := ji.modrm & 7

	x86EmitLoadGuestReg32(cb, amd64R8, dstReg)
	x86EmitLoadGuestReg32(cb, amd64RCX, 1) // guest ECX = shift count

	// BMI2 path for SHL/SHR/SAR by CL when flags dead
	flagsDead := instrIdx < len(cs.flagsNeeded) && !cs.flagsNeeded[instrIdx]
	if cs.host.HasBMI2 && flagsDead && (shiftOp == 4 || shiftOp == 5 || shiftOp == 7) {
		var pp byte
		switch shiftOp {
		case 4:
			pp = 0x01
		case 5:
			pp = 0x03
		case 7:
			pp = 0x02
		}
		emitBMI2Shift(cb, pp, amd64R8, amd64R8, amd64RCX)
		x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
		return true
	}

	// RCL/RCR consume host CF; restore guest CF first.
	if shiftOp == 2 || shiftOp == 3 {
		x86EmitRestoreGuestCF(cb)
	}
	emitREX(cb, false, 0, amd64R8)
	cb.EmitBytes(0xD3, modRM(3, shiftOp, amd64R8))

	cs.flagState = x86FlagsLiveArith
	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// ===========================================================================
// Grp3 Emitters (NOT/NEG/MUL/IMUL/DIV/IDIV/TEST)
// ===========================================================================

// x86EmitGrp3_Ev handles Grp3 Ev (0xF7) -- register mode only.
func x86EmitGrp3_Ev(cb *CodeBuffer, ji *X86JITInstr, memory []byte, cs *x86CompileState) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	subOp := (ji.modrm >> 3) & 7
	rmReg := ji.modrm & 7

	switch subOp {
	case 0, 1: // TEST Ev, Iv
		immPC := ji.opcodePC + uint32(ji.length) - 4
		imm := int32(int32(memory[immPC]) | int32(memory[immPC+1])<<8 | int32(memory[immPC+2])<<16 | int32(memory[immPC+3])<<24)
		x86EmitLoadGuestReg32(cb, amd64R8, rmReg)
		// TEST R8d, imm32: F7 C0+reg imm32
		emitREX(cb, false, 0, amd64R8)
		cb.EmitBytes(0xF7, modRM(3, 0, amd64R8))
		cb.Emit32(uint32(imm))
		cs.flagState = x86FlagsLiveLogic
		return true

	case 2: // NOT Ev
		x86EmitLoadGuestReg32(cb, amd64R8, rmReg)
		emitREX(cb, false, 0, amd64R8)
		cb.EmitBytes(0xF7, modRM(3, 2, amd64R8))
		// NOT doesn't affect flags
		x86EmitStoreGuestReg32(cb, rmReg, amd64R8)
		return true

	case 3: // NEG Ev
		x86EmitLoadGuestReg32(cb, amd64R8, rmReg)
		amd64NEG32(cb, amd64R8)
		cs.flagState = x86FlagsLiveArith
		x86EmitStoreGuestReg32(cb, rmReg, amd64R8)
		return true

	case 4: // MUL Ev (unsigned: EDX:EAX = EAX * r/m32)
		x86EmitLoadGuestReg32(cb, amd64RAX, 0) // guest EAX
		x86EmitLoadGuestReg32(cb, amd64R8, rmReg)
		// MUL R8d: F7 E0+reg
		emitREX(cb, false, 0, amd64R8)
		cb.EmitBytes(0xF7, modRM(3, 4, amd64R8))
		cs.flagState = x86FlagsLiveArith
		x86EmitStoreGuestReg32(cb, 0, amd64RAX) // guest EAX = low
		x86EmitStoreGuestReg32(cb, 2, amd64RDX) // guest EDX = high
		return true

	case 5: // IMUL Ev (signed: EDX:EAX = EAX * r/m32)
		x86EmitLoadGuestReg32(cb, amd64RAX, 0)
		x86EmitLoadGuestReg32(cb, amd64R8, rmReg)
		emitREX(cb, false, 0, amd64R8)
		cb.EmitBytes(0xF7, modRM(3, 5, amd64R8))
		cs.flagState = x86FlagsLiveArith
		x86EmitStoreGuestReg32(cb, 0, amd64RAX)
		x86EmitStoreGuestReg32(cb, 2, amd64RDX)
		return true

	case 6: // DIV Ev (unsigned: EAX = EDX:EAX / r/m32, EDX = remainder)
		x86EmitLoadGuestReg32(cb, amd64RAX, 0)
		x86EmitLoadGuestReg32(cb, amd64RDX, 2) // guest EDX
		x86EmitLoadGuestReg32(cb, amd64R8, rmReg)
		emitREX(cb, false, 0, amd64R8)
		cb.EmitBytes(0xF7, modRM(3, 6, amd64R8))
		cs.flagState = x86FlagsDead // DIV: flags undefined
		x86EmitStoreGuestReg32(cb, 0, amd64RAX)
		x86EmitStoreGuestReg32(cb, 2, amd64RDX)
		return true

	case 7: // IDIV Ev (signed)
		x86EmitLoadGuestReg32(cb, amd64RAX, 0)
		x86EmitLoadGuestReg32(cb, amd64RDX, 2)
		x86EmitLoadGuestReg32(cb, amd64R8, rmReg)
		emitREX(cb, false, 0, amd64R8)
		cb.EmitBytes(0xF7, modRM(3, 7, amd64R8))
		cs.flagState = x86FlagsDead
		x86EmitStoreGuestReg32(cb, 0, amd64RAX)
		x86EmitStoreGuestReg32(cb, 2, amd64RDX)
		return true
	}

	return false
}

// ===========================================================================
// TEST Emitters
// ===========================================================================

func x86EmitTEST_Ev_Gv(cb *CodeBuffer, ji *X86JITInstr, cs *x86CompileState) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	srcReg := (ji.modrm >> 3) & 7
	dstReg := ji.modrm & 7

	x86EmitLoadGuestReg32(cb, amd64R8, dstReg)
	x86EmitLoadGuestReg32(cb, amd64R10, srcReg)

	// TEST R8d, R10d
	emitREX(cb, false, amd64R10, amd64R8)
	cb.EmitBytes(0x85, modRM(3, amd64R10, amd64R8))

	cs.flagState = x86FlagsLiveLogic
	return true
}

func x86EmitTEST_AL_Ib(cb *CodeBuffer, ji *X86JITInstr, memory []byte, cs *x86CompileState) bool {
	immPC := ji.opcodePC + uint32(ji.length) - 1
	imm := memory[immPC]

	x86EmitLoadGuestReg32(cb, amd64RAX, 0) // guest EAX
	// TEST AL, imm8: A8 ib
	cb.EmitBytes(0xA8, imm)

	cs.flagState = x86FlagsLiveLogic
	return true
}

func x86EmitTEST_EAX_Iv(cb *CodeBuffer, ji *X86JITInstr, memory []byte, cs *x86CompileState) bool {
	immPC := ji.opcodePC + uint32(ji.length) - 4
	imm := uint32(memory[immPC]) | uint32(memory[immPC+1])<<8 | uint32(memory[immPC+2])<<16 | uint32(memory[immPC+3])<<24

	x86EmitLoadGuestReg32(cb, amd64RAX, 0)
	// TEST EAX, imm32: A9 id
	cb.EmitBytes(0xA9)
	cb.Emit32(imm)

	cs.flagState = x86FlagsLiveLogic
	return true
}

// ===========================================================================
// ALU AL, Ib Emitters
// ===========================================================================

func x86EmitALU_AL_Ib(cb *CodeBuffer, ji *X86JITInstr, memory []byte, cs *x86CompileState) bool {
	op := byte(ji.opcode)
	immPC := ji.opcodePC + uint32(ji.length) - 1
	imm := memory[immPC]

	// Determine ALU sub-op from opcode
	aluOp := (op - 0x04) / 8 // 0=ADD,1=OR,2=ADC,3=SBB,4=AND,5=SUB,6=XOR,7=CMP

	// Load guest EAX into scratch, extract AL, operate, merge back
	x86EmitLoadGuestReg32(cb, amd64RAX, 0)

	// ADC/SBB read host CF as input. Bookkeeping between the previous
	// flag producer and this op (chain-budget DEC, NeedInval CMP, the
	// per-instr capture sequence, etc.) clobbers host RFLAGS, so we
	// must reinstall guest CF from the captured slot first. ADD/OR/
	// AND/SUB/XOR/CMP do not read flags, no restore needed.
	if aluOp == 2 || aluOp == 3 { // ADC, SBB
		x86EmitRestoreGuestCF(cb)
	}

	// Emit ALU AL, imm8 directly (opcode is op itself)
	cb.EmitBytes(op, imm)

	switch aluOp {
	case 0, 2, 3, 5: // ADD, ADC, SBB, SUB
		cs.flagState = x86FlagsLiveArith
	case 1, 4, 6: // OR, AND, XOR
		cs.flagState = x86FlagsLiveLogic
	case 7: // CMP
		cs.flagState = x86FlagsLiveArith
		return true // Don't store result
	}

	x86EmitStoreGuestReg32(cb, 0, amd64RAX)
	return true
}

// ===========================================================================
// PUSH / POP Emitters
// ===========================================================================

func x86EmitPUSH_r32(cb *CodeBuffer, ji *X86JITInstr) bool {
	guestReg := byte(ji.opcode) - 0x50
	x86MarkDirty(4) // ESP modified

	// ESP -= 4
	amd64ALU_reg_imm32_32bit(cb, 5, x86AMD64RegGuestESP, 4) // SUB R14d, 4

	// Load value to push
	x86EmitLoadGuestReg32(cb, amd64R8, guestReg)

	// Write to [memory + ESP]
	amd64MOV_reg_reg32(cb, amd64R10, x86AMD64RegGuestESP)

	// MOV [RSI + R10], R8d
	emitREX_SIB(cb, false, amd64R8, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x89, modRM(0, amd64R8, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	return true
}

func x86EmitPOP_r32(cb *CodeBuffer, ji *X86JITInstr) bool {
	guestReg := byte(ji.opcode) - 0x58
	x86MarkDirty(4) // ESP modified

	// Read from [memory + ESP]
	amd64MOV_reg_reg32(cb, amd64R10, x86AMD64RegGuestESP)

	// MOV R8d, [RSI + R10]
	emitREX_SIB(cb, false, amd64R8, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x8B, modRM(0, amd64R8, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	// Store to guest register
	x86EmitStoreGuestReg32(cb, guestReg, amd64R8)

	// ESP += 4
	amd64ALU_reg_imm32_32bit(cb, 0, x86AMD64RegGuestESP, 4) // ADD R14d, 4

	return true
}

func x86EmitPUSH_imm32(cb *CodeBuffer, ji *X86JITInstr, memory []byte) bool {
	x86MarkDirty(4) // ESP modified
	immPC := ji.opcodePC + uint32(ji.length) - 4
	imm := uint32(memory[immPC]) | uint32(memory[immPC+1])<<8 | uint32(memory[immPC+2])<<16 | uint32(memory[immPC+3])<<24

	// ESP -= 4
	amd64ALU_reg_imm32_32bit(cb, 5, x86AMD64RegGuestESP, 4)

	// Write imm32 to [memory + ESP]
	amd64MOV_reg_reg32(cb, amd64R10, x86AMD64RegGuestESP)

	// MOV DWORD [RSI + R10], imm32
	amd64MOV_reg_imm32(cb, amd64R8, imm)
	emitREX_SIB(cb, false, amd64R8, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x89, modRM(0, amd64R8, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	return true
}

func x86EmitPUSH_imm8(cb *CodeBuffer, ji *X86JITInstr, memory []byte) bool {
	x86MarkDirty(4) // ESP modified
	immPC := ji.opcodePC + uint32(ji.length) - 1
	imm := uint32(int32(int8(memory[immPC]))) // sign-extend

	amd64ALU_reg_imm32_32bit(cb, 5, x86AMD64RegGuestESP, 4)
	amd64MOV_reg_reg32(cb, amd64R10, x86AMD64RegGuestESP)

	amd64MOV_reg_imm32(cb, amd64R8, imm)
	emitREX_SIB(cb, false, amd64R8, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x89, modRM(0, amd64R8, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	return true
}

// ===========================================================================
// MOVZX / MOVSX Emitters (two-byte opcodes)
// ===========================================================================

func x86EmitMOVZX_Gv_Eb(cb *CodeBuffer, ji *X86JITInstr) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	dstReg := (ji.modrm >> 3) & 7
	srcR8 := ji.modrm & 7

	// r8 encoding: 0-3 = AL/CL/DL/BL, 4-7 = AH/CH/DH/BH
	srcGuestReg := srcR8 & 3
	isHigh := srcR8 >= 4

	x86EmitLoadGuestReg32(cb, amd64R8, srcGuestReg)
	if isHigh {
		amd64SHR_imm32(cb, amd64R8, 8)
	}
	// MOVZX R8d from R8b (zero-extend byte)
	emitREX(cb, false, amd64R8, amd64R8)
	cb.EmitBytes(0x0F, 0xB6, modRM(3, amd64R8, amd64R8))

	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

func x86EmitMOVZX_Gv_Ew(cb *CodeBuffer, ji *X86JITInstr) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	dstReg := (ji.modrm >> 3) & 7
	srcReg := ji.modrm & 7

	x86EmitLoadGuestReg32(cb, amd64R8, srcReg)
	// MOVZX R8d, R8w (zero-extend word)
	emitREX(cb, false, amd64R8, amd64R8)
	cb.EmitBytes(0x0F, 0xB7, modRM(3, amd64R8, amd64R8))

	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

func x86EmitMOVSX_Gv_Eb(cb *CodeBuffer, ji *X86JITInstr) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	dstReg := (ji.modrm >> 3) & 7
	srcR8 := ji.modrm & 7

	srcGuestReg := srcR8 & 3
	isHigh := srcR8 >= 4

	x86EmitLoadGuestReg32(cb, amd64R8, srcGuestReg)
	if isHigh {
		amd64SHR_imm32(cb, amd64R8, 8)
	}
	// MOVSX R8d, R8b (sign-extend byte)
	emitREX(cb, false, amd64R8, amd64R8)
	cb.EmitBytes(0x0F, 0xBE, modRM(3, amd64R8, amd64R8))

	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

func x86EmitMOVSX_Gv_Ew(cb *CodeBuffer, ji *X86JITInstr) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	dstReg := (ji.modrm >> 3) & 7
	srcReg := ji.modrm & 7

	x86EmitLoadGuestReg32(cb, amd64R8, srcReg)
	// MOVSX R8d, R8w
	emitREX(cb, false, amd64R8, amd64R8)
	cb.EmitBytes(0x0F, 0xBF, modRM(3, amd64R8, amd64R8))

	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// ===========================================================================
// IMUL Gv,Ev (two-byte: 0x0F AF)
// ===========================================================================

func x86EmitIMUL_Gv_Ev(cb *CodeBuffer, ji *X86JITInstr, cs *x86CompileState) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	dstReg := (ji.modrm >> 3) & 7
	srcReg := ji.modrm & 7

	x86EmitLoadGuestReg32(cb, amd64R8, dstReg)
	x86EmitLoadGuestReg32(cb, amd64R10, srcReg)

	// IMUL R8d, R10d: 0F AF /r
	emitREX(cb, false, amd64R8, amd64R10)
	cb.EmitBytes(0x0F, 0xAF, modRM(3, amd64R8, amd64R10))

	cs.flagState = x86FlagsLiveArith
	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// ===========================================================================
// Grp1 Eb,Ib (0x80/0x82) -- 8-bit ALU with immediate
// ===========================================================================

func x86EmitGrp1_Eb_Ib(cb *CodeBuffer, ji *X86JITInstr, memory []byte, cs *x86CompileState) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	aluOp := (ji.modrm >> 3) & 7
	r8 := ji.modrm & 7
	immPC := ji.opcodePC + uint32(ji.length) - 1
	imm := memory[immPC]

	guestReg := r8 & 3
	isHigh := r8 >= 4

	// Load full 32-bit, operate on byte, merge back
	x86EmitLoadGuestReg32(cb, amd64RAX, guestReg)

	if isHigh {
		// Extract high byte to R8, operate, merge back
		amd64MOV_reg_reg32(cb, amd64R8, amd64RAX)
		amd64SHR_imm32(cb, amd64R8, 8)

		// ADC/SBB read host CF; reinstall guest CF before the byte ALU.
		if aluOp == 2 || aluOp == 3 {
			x86EmitRestoreGuestCF(cb)
		}

		// ALU R8b, imm8 — sets host EFLAGS based on byte result.
		emitREX(cb, false, 0, amd64R8)
		cb.EmitBytes(0x80, modRM(3, aluOp, amd64R8), imm)

		// Capture guest EFLAGS NOW, before the merge-back AND/SHL/OR
		// sequence below clobbers them. The compile loop's generic
		// post-instr capture would otherwise record the merge
		// bookkeeping flags instead of the guest ALU output. Mark
		// flagCaptureDone so the loop skips the generic capture.
		switch aluOp {
		case 0, 2, 3, 5, 7: // ADD, ADC, SBB, SUB, CMP — arith
			x86EmitCaptureFlagsArith(cb)
			cs.flagCaptureDone = true
		case 1, 4, 6: // OR, AND, XOR — logic (preserve guest's prior AF)
			x86EmitCaptureFlagsLogic(cb)
			cs.flagCaptureDone = true
		}

		if aluOp != 7 { // not CMP
			// Merge back: clear bits 15:8 of EAX, insert R8 << 8
			emitREX(cb, false, 0, amd64RAX)
			cb.EmitBytes(0x81, modRM(3, 4, amd64RAX)) // AND EAX, ~0xFF00
			cb.Emit32(0xFFFF00FF)
			amd64ALU_reg_imm32_32bit(cb, 4, amd64R8, 0xFF) // AND R8, 0xFF
			amd64SHL_imm32(cb, amd64R8, 8)
			amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64R8) // OR EAX, R8
			x86EmitStoreGuestReg32(cb, guestReg, amd64RAX)
		}
	} else {
		// ADC/SBB read host CF; reinstall guest CF before the byte ALU.
		if aluOp == 2 || aluOp == 3 {
			x86EmitRestoreGuestCF(cb)
		}
		// ALU AL, imm8
		cb.EmitBytes(0x80, modRM(3, aluOp, amd64RAX), imm)
		if aluOp != 7 {
			x86EmitStoreGuestReg32(cb, guestReg, amd64RAX)
		}
	}

	switch aluOp {
	case 0, 2, 3, 5: // ADD, ADC, SBB, SUB
		cs.flagState = x86FlagsLiveArith
	case 1, 4, 6: // OR, AND, XOR
		cs.flagState = x86FlagsLiveLogic
	case 7: // CMP
		cs.flagState = x86FlagsLiveArith
	}
	return true
}

// ===========================================================================
// MOV Ev,Sw (0x8C) -- Read segment register
// ===========================================================================

func x86EmitMOV_Ev_Sw(cb *CodeBuffer, ji *X86JITInstr) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	segIdx := (ji.modrm >> 3) & 7
	dstReg := ji.modrm & 7

	if segIdx > 5 {
		return false // invalid segment
	}

	// Load from jitSegRegs[segIdx]
	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffSegRegsPtr))
	// MOVZX R8d, WORD [RAX + segIdx*2]
	emitREX(cb, false, amd64R8, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7) // MOVZX r32, r/m16
	if segIdx*2 == 0 {
		cb.EmitBytes(modRM(0, amd64R8, amd64RAX))
	} else {
		cb.EmitBytes(modRM(1, amd64R8, amd64RAX), byte(segIdx*2))
	}

	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// ===========================================================================
// PUSHF Emitter
// ===========================================================================

func x86EmitPUSHF(cb *CodeBuffer, ji *X86JITInstr, cs *x86CompileState, instrIdx int) bool {
	// PUSHF reads cpu.Flags directly. cpu.Flags is materialized only at
	// unchained exit (x86EmitFullEpilogueEnd → x86EmitMergeFlagsToGuest);
	// chained transitions skip the merge, leaving cpu.Flags potentially
	// stale relative to a producer in the source block whose host-EFLAGS
	// were captured into the savedEFlags slot but never folded into
	// cpu.Flags. If PUSHF is the first instruction of a block, the block
	// can be entered via a chain JMP from another block's terminator
	// (CALL/JMP/Jcc) — at which point cpu.Flags reflects the *previous*
	// merge boundary, not the live guest state. Bail compile so the
	// dispatcher's full-epilogue-then-reentry path runs and merges flags
	// before this instruction observes them. Subsequent in-block PUSHF
	// (instrIdx > 0) is safe: the prior in-block flag-producer's merge is
	// not load-bearing for the cpu.Flags read either, but in practice the
	// peephole flag-capture also doesn't touch cpu.Flags, and PUSHF after
	// any in-block instruction is rare enough that this narrow gate is
	// the right scope. The gate is stricter than necessary at instrIdx==0
	// (it bails even when flagState happens to be live from an inline
	// fallback path), but the over-bail is harmless: PUSHF as first
	// instruction is uncommon in hot paths.
	if instrIdx == 0 && cs.flagState == x86FlagsDead {
		return false
	}
	x86MarkDirty(4) // ESP modified
	// Push guest Flags register to guest stack
	// Load Flags from context
	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffFlagsPtr))
	amd64MOV_reg_mem32(cb, amd64R8, amd64RAX, 0) // R8d = *FlagsPtr

	// ESP -= 4
	amd64ALU_reg_imm32_32bit(cb, 5, x86AMD64RegGuestESP, 4)

	// Write to [memory + ESP]
	amd64MOV_reg_reg32(cb, amd64R10, x86AMD64RegGuestESP)

	emitREX_SIB(cb, false, amd64R8, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x89, modRM(0, amd64R8, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	return true
}

// ===========================================================================
// LEAVE Emitter (0xC9) -- MOV ESP, EBP; POP EBP
// ===========================================================================

func x86EmitLEAVE(cb *CodeBuffer, ji *X86JITInstr) bool {
	x86MarkDirty(4) // ESP modified
	x86MarkDirty(5) // EBP modified
	// MOV ESP, EBP
	x86EmitLoadGuestReg32(cb, amd64R8, 5)  // guest EBP
	x86EmitStoreGuestReg32(cb, 4, amd64R8) // guest ESP = EBP

	// POP EBP: read [memory + ESP], ESP += 4
	amd64MOV_reg_reg32(cb, amd64R10, x86AMD64RegGuestESP)

	emitREX_SIB(cb, false, amd64R8, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x8B, modRM(0, amd64R8, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	x86EmitStoreGuestReg32(cb, 5, amd64R8) // guest EBP = popped value

	amd64ALU_reg_imm32_32bit(cb, 0, x86AMD64RegGuestESP, 4) // ESP += 4

	return true
}

// ===========================================================================
// MOV Eb,Ib (0xC6) and MOV Ev,Iv (0xC7)
// ===========================================================================

func x86EmitMOV_Eb_Ib(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if !ji.hasModRM {
		return false
	}
	immPC := ji.opcodePC + uint32(ji.length) - 1
	imm := memory[immPC]

	if ji.modrm>>6 != 3 {
		if ji.grpOp != 0 {
			return false
		}
		if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
			return false
		}
		x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)
		amd64MOV_reg_imm32(cb, amd64R8, uint32(imm))
		x86EmitMemStore8(cb, amd64R10, amd64R8)
		x86EmitSelfModCheckMaybeElide(cb, amd64R10, ji, memory, ji.opcodePC+uint32(ji.length), instrIdx+1)
		return true
	}

	r8 := ji.modrm & 7

	guestReg := r8 & 3
	isHigh := r8 >= 4

	x86EmitLoadGuestReg32(cb, amd64RAX, guestReg)

	if isHigh {
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0x81, modRM(3, 4, amd64RAX))
		cb.Emit32(0xFFFF00FF) // AND EAX, ~0xFF00
		if imm != 0 {
			emitREX(cb, false, 0, amd64RAX)
			cb.EmitBytes(0x81, modRM(3, 1, amd64RAX))
			cb.Emit32(uint32(imm) << 8) // OR EAX, imm<<8
		}
	} else {
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0x81, modRM(3, 4, amd64RAX))
		cb.Emit32(0xFFFFFF00) // AND EAX, ~0xFF
		if imm != 0 {
			amd64ALU_reg_imm32_32bit(cb, 1, amd64RAX, int32(imm)) // OR EAX, imm
		}
	}

	x86EmitStoreGuestReg32(cb, guestReg, amd64RAX)
	return true
}

func x86EmitMOV_Ev_Iv(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if !ji.hasModRM {
		return false
	}
	immPC := ji.opcodePC + uint32(ji.length) - 4
	imm := uint32(memory[immPC]) | uint32(memory[immPC+1])<<8 | uint32(memory[immPC+2])<<16 | uint32(memory[immPC+3])<<24

	if ji.modrm>>6 != 3 {
		if ji.grpOp != 0 {
			return false
		}
		if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
			return false
		}
		x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)
		amd64MOV_reg_imm32(cb, amd64R8, imm)
		x86EmitMemStore32(cb, amd64R10, amd64R8)
		x86EmitSelfModCheckMaybeElide(cb, amd64R10, ji, memory, ji.opcodePC+uint32(ji.length), instrIdx+1)
		return true
	}

	dstReg := ji.modrm & 7
	x86MarkDirty(dstReg)
	if hostReg, mapped := x86GuestRegToHost(dstReg); mapped {
		amd64MOV_reg_imm32(cb, hostReg, imm)
	} else {
		amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffJITRegsPtr))
		amd64MOV_mem_imm32(cb, amd64RAX, int32(dstReg)*4, imm)
	}
	return true
}

// ===========================================================================
// SETcc / CMOVcc / BSF / BSR / LOOP Emitters
// ===========================================================================

// x86EmitSETcc handles SETcc r/m8 (0x0F 90-9F) -- register mode only.
func x86EmitSETcc(cb *CodeBuffer, ji *X86JITInstr, cond byte, cs *x86CompileState) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	r8 := ji.modrm & 7
	guestReg := r8 & 3
	isHigh := r8 >= 4

	if cs.flagState != x86FlagsLiveArith && cs.flagState != x86FlagsLiveLogic && cs.flagState != x86FlagsLiveInc {
		return false // flags not live
	}

	// SETcc into R8b (use R8 which has no REX conflict for SETcc)
	// Actually SETcc with REX uses the low byte of extended registers
	amd64SETcc(cb, cond, amd64R8) // R8b = condition ? 1 : 0

	// Merge into guest register byte
	x86EmitLoadGuestReg32(cb, amd64RAX, guestReg)
	if isHigh {
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0x81, modRM(3, 4, amd64RAX))
		cb.Emit32(0xFFFF00FF) // AND clear bits 15:8
		amd64MOVZX_B(cb, amd64R8, amd64R8)
		amd64SHL_imm32(cb, amd64R8, 8)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64R8)
	} else {
		emitREX(cb, false, 0, amd64RAX)
		cb.EmitBytes(0x81, modRM(3, 4, amd64RAX))
		cb.Emit32(0xFFFFFF00)
		amd64MOVZX_B(cb, amd64R8, amd64R8)
		amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64R8)
	}
	x86EmitStoreGuestReg32(cb, guestReg, amd64RAX)
	return true
}

// x86EmitCMOVcc handles CMOVcc Gv, Ev (0x0F 40-4F) -- register mode only.
func x86EmitCMOVcc(cb *CodeBuffer, ji *X86JITInstr, cond byte, cs *x86CompileState) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	if cs.flagState != x86FlagsLiveArith && cs.flagState != x86FlagsLiveLogic && cs.flagState != x86FlagsLiveInc {
		return false
	}

	dstReg := (ji.modrm >> 3) & 7
	srcReg := ji.modrm & 7

	x86EmitLoadGuestReg32(cb, amd64R8, dstReg)
	x86EmitLoadGuestReg32(cb, amd64R10, srcReg)

	// CMOV R8d, R10d
	emitREX(cb, false, amd64R8, amd64R10)
	cb.EmitBytes(0x0F, 0x40+cond, modRM(3, amd64R8, amd64R10))

	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// x86EmitBSx handles BSF/BSR Gv, Ev (0x0F BC/BD) -- register mode only.
// x86EmitBSx handles BSF/BSR (0x0F BC/BD) -- register mode only.
// When LZCNT is available, uses TZCNT/LZCNT for better throughput (no false
// dependency on destination register). Preserves BSF/BSR zero-input semantics:
// on zero input, destination is unchanged and ZF=1.
func x86EmitBSx(cb *CodeBuffer, ji *X86JITInstr, op2 byte, cs *x86CompileState) bool {
	if !ji.hasModRM || ji.modrm>>6 != 3 {
		return false
	}
	dstReg := (ji.modrm >> 3) & 7
	srcReg := ji.modrm & 7

	x86EmitLoadGuestReg32(cb, amd64R10, srcReg)

	if cs.host.HasLZCNT {
		// TZCNT/LZCNT path with zero-input preservation:
		// 1. TEST R10, R10
		// 2. JZ zero_case (ZF=1, skip destination write)
		// 3. TZCNT/LZCNT R8, R10
		// 4. For BSR: XOR R8, 31 (convert LZCNT to bit position)
		// 5. Store result
		// 6. JMP done
		// 7. zero_case: (destination unchanged, ZF already set by TEST)
		// 8. done:

		emitREX(cb, false, amd64R10, amd64R10)
		cb.EmitBytes(0x85, modRM(3, amd64R10, amd64R10)) // TEST R10d, R10d
		zeroJmp := amd64Jcc_rel32(cb, amd64CondE)        // JZ zero_case

		// Non-zero path: use TZCNT or LZCNT
		if op2 == 0xBC {
			// TZCNT R8d, R10d: F3 0F BC /r
			cb.EmitBytes(0xF3)
			emitREX(cb, false, amd64R8, amd64R10)
			cb.EmitBytes(0x0F, 0xBC, modRM(3, amd64R8, amd64R10))
		} else {
			// LZCNT R8d, R10d: F3 0F BD /r
			cb.EmitBytes(0xF3)
			emitREX(cb, false, amd64R8, amd64R10)
			cb.EmitBytes(0x0F, 0xBD, modRM(3, amd64R8, amd64R10))
			// Convert LZCNT result to BSR result: bit_pos = 31 - lzcnt
			amd64ALU_reg_imm32_32bit(cb, 6, amd64R8, 31) // XOR R8d, 31
		}

		x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
		doneJmp := amd64JMP_rel32(cb)

		// zero_case: destination unchanged (ZF=1 from TEST)
		zeroLabel := cb.Len()
		patchRel32(cb, zeroJmp, zeroLabel)

		doneLabel := cb.Len()
		patchRel32(cb, doneJmp, doneLabel)

		cs.flagState = x86FlagsLiveLogic
		return true
	}

	// Standard BSF/BSR path (no LZCNT)
	// Note: standard BSF/BSR on x86 already leave dest unchanged on zero input
	// and set ZF=1, so no special handling needed.
	x86EmitLoadGuestReg32(cb, amd64R8, dstReg) // pre-load dest for preservation
	emitREX(cb, false, amd64R8, amd64R10)
	cb.EmitBytes(0x0F, op2, modRM(3, amd64R8, amd64R10))

	cs.flagState = x86FlagsLiveLogic
	x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
	return true
}

// x86EmitLOOP handles LOOP rel8 (0xE2) -- decrement ECX, jump if not zero.
func x86EmitLOOP(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	immPC := ji.opcodePC + uint32(ji.length) - 1
	rel := int32(int8(memory[immPC]))
	nextPC := ji.opcodePC + uint32(ji.length)
	targetPC := uint32(int32(nextPC) + rel)

	// DEC ECX
	x86EmitLoadGuestReg32(cb, amd64RCX, 1)       // guest ECX
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1) // SUB ECX, 1
	x86EmitStoreGuestReg32(cb, 1, amd64RCX)

	// TEST ECX, ECX
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))

	// JNZ -> exit to targetPC
	exitOff := amd64Jcc_rel32(cb, amd64CondNE)
	fallThroughJmp := amd64JMP_rel32(cb)

	exitLabel := cb.Len()
	patchRel32(cb, exitOff, exitLabel)
	x86EmitRetPC(cb, targetPC, uint32(instrIdx+1))
	x86EmitLightweightEpilogue(cb)
	x86EmitFullEpilogueEnd(cb)

	fallThroughLabel := cb.Len()
	patchRel32(cb, fallThroughJmp, fallThroughLabel)

	return true
}

// ===========================================================================
// x87 FPU Emitters (Tier 1: SSE2 on x86-64 host)
// ===========================================================================

// FPU_X87 struct field offsets
const (
	fpuOffRegs = 0  // regs [8]float64 at offset 0
	fpuOffFCW  = 64 // FCW uint16
	fpuOffFSW  = 66 // FSW uint16
	fpuOffFTW  = 68 // FTW uint16
)

// x86EmitFPU dispatches x87 FPU instructions (D8-DF).
func x86EmitFPU(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if !ji.hasModRM {
		return false
	}
	escape := byte(ji.opcode)
	modrm := ji.modrm
	mod := modrm >> 6
	regOp := (modrm >> 3) & 7
	rm := modrm & 7

	// Load FPU pointer
	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffFPUPtr))

	switch escape {
	case 0xD8: // FADD/FMUL/FCOM/FCOMP/FSUB/FSUBR/FDIV/FDIVR ST(0), ST(i)
		if mod == 3 {
			switch regOp {
			case 0: // FADD ST(0), ST(i)
				return x86EmitFPUBinaryOp(cb, rm, 0x58, ji.opcodePC, instrIdx) // ADDSD
			case 1: // FMUL ST(0), ST(i)
				return x86EmitFPUBinaryOp(cb, rm, 0x59, ji.opcodePC, instrIdx) // MULSD
			case 4: // FSUB ST(0), ST(i)
				return x86EmitFPUBinaryOp(cb, rm, 0x5C, ji.opcodePC, instrIdx) // SUBSD
			case 6: // FDIV ST(0), ST(i)
				return x86EmitFPUBinaryOp(cb, rm, 0x5E, ji.opcodePC, instrIdx) // DIVSD
			}
		}

	case 0xD9:
		if mod == 3 {
			if modrm >= 0xC0 && modrm <= 0xC7 { // FLD ST(i)
				return x86EmitFLD_STi(cb, rm)
			}
			if modrm >= 0xC8 && modrm <= 0xCF { // FXCH ST(i)
				return x86EmitFXCH(cb, rm)
			}
			if modrm == 0xE0 { // FCHS
				return x86EmitFCHS(cb)
			}
			if modrm == 0xE1 { // FABS
				return x86EmitFABS(cb)
			}
		} else {
			// D9 /0: FLD mem32 (single-precision)
			if regOp == 0 {
				return x86EmitFLD_mem32(cb, ji, memory, instrIdx)
			}
		}

	case 0xDD:
		if mod == 3 {
			if modrm >= 0xD8 && modrm <= 0xDF { // FSTP ST(i)
				return x86EmitFSTP_STi(cb, rm)
			}
			if modrm >= 0xD0 && modrm <= 0xD7 { // FST ST(i)
				return x86EmitFST_STi(cb, rm)
			}
		} else {
			if regOp == 0 { // FLD mem64 (double-precision)
				return x86EmitFLD_mem64(cb, ji, memory, instrIdx)
			}
			if regOp == 3 { // FSTP mem64
				return x86EmitFSTP_mem64(cb, ji, memory, instrIdx)
			}
			if regOp == 2 { // FST mem64
				return x86EmitFST_mem64(cb, ji, memory, instrIdx)
			}
		}
	}

	return false
}

// x86EmitFPUReadTOP emits code to read the FPU TOP field into the given register.
// RAX must point to FPU_X87 struct.
func x86EmitFPUReadTOP(cb *CodeBuffer, dstReg byte) {
	emitREX(cb, false, dstReg, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, dstReg, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, dstReg, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, dstReg, 7)
}

// x86EmitFPUCheckTag emits a tag check for a physical FPU register.
// physRegReg holds the physical register index (0-7).
// If the tag equals badTag, bails to interpreter.
// RAX must point to FPU_X87 struct.
func x86EmitFPUCheckTag(cb *CodeBuffer, physRegReg byte, badTag uint16, retPC uint32, instrIdx int) {
	// FTW is at fpuOffFTW. Tag for phys reg i is bits (i*2+1):(i*2) of FTW.
	// Load FTW
	emitREX(cb, false, amd64R11, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64R11, amd64RAX), byte(fpuOffFTW)) // MOVZX R11, [RAX+FTW]

	// Shift right by physReg*2: SHR R11, CL (save/restore CL)
	amd64MOV_reg_reg32(cb, amd64R8, amd64RCX) // save RCX
	amd64MOV_reg_reg32(cb, amd64RCX, physRegReg)
	amd64SHL_imm32(cb, amd64RCX, 1) // *2
	amd64SHR_CL32(cb, amd64R11)
	amd64MOV_reg_reg32(cb, amd64RCX, amd64R8)    // restore RCX
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, 3) // AND R11, 3

	// CMP R11, badTag
	amd64ALU_reg_imm32_32bit(cb, 7, amd64R11, int32(badTag))
	// JE -> bail
	bailOff := amd64Jcc_rel32(cb, amd64CondE)
	skipOff := amd64JMP_rel32(cb)

	bailLabel := cb.Len()
	patchRel32(cb, bailOff, bailLabel)
	// Bail to interpreter
	amd64MOV_mem_imm32(cb, x86AMD64RegCtx, int32(x86CtxOffNeedIOFallback), 1)
	x86EmitRetPC(cb, retPC, uint32(instrIdx))
	x86EmitLightweightEpilogue(cb)
	x86EmitFullEpilogueEnd(cb)

	skipLabel := cb.Len()
	patchRel32(cb, skipOff, skipLabel)
}

// x86EmitFPUBinaryOp emits FADD/FMUL/FSUB/FDIV ST(0), ST(i) using SSE2.
// sseOp is the SSE2 opcode byte (0x58=ADDSD, 0x59=MULSD, 0x5C=SUBSD, 0x5E=DIVSD).
func x86EmitFPUBinaryOp(cb *CodeBuffer, stIdx byte, sseOp byte, retPC uint32, instrIdx int) bool {
	_, _ = retPC, instrIdx // available for future tag checks
	// RAX = FPUPtr (already loaded by caller)
	x86EmitFPUReadTOP(cb, amd64RCX)
	// Note: tag checks omitted to match interpreter behavior (operates regardless of tags)

	// physST0 = TOP & 7 (already in ECX)
	// physSTi = (TOP + i) & 7
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RCX)
	if stIdx != 0 {
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, int32(stIdx))
		amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 7)
	}

	// Load ST(0) into XMM0: MOVSD XMM0, [RAX + ECX*8]
	// SHL ECX, 3 to get byte offset
	amd64SHL_imm32(cb, amd64RCX, 3)
	// MOVSD XMM0, [RAX + RCX] (F2 0F 10 04 08)
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64RCX, amd64RAX))

	// Load ST(i) into XMM1: MOVSD XMM1, [RAX + EDX*8]
	amd64SHL_imm32(cb, amd64RDX, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 1, 4), sibByte(0, amd64RDX, amd64RAX))

	// Perform SSE2 op: XMM0 = XMM0 op XMM1
	cb.EmitBytes(0xF2, 0x0F, sseOp, modRM(3, 0, 1))

	// Store result back to ST(0): MOVSD [RAX + RCX], XMM0
	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64RCX, amd64RAX))

	return true
}

// x86EmitFLD_STi emits FLD ST(i) -- push ST(i) onto stack.
func x86EmitFLD_STi(cb *CodeBuffer, stIdx byte) bool {
	// RAX = FPUPtr (already loaded)
	// Read TOP
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)

	// physSTi = (TOP + i) & 7
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RCX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, int32(stIdx))
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 7)

	// Load ST(i) value: MOVSD XMM0, [RAX + RDX*8]
	amd64SHL_imm32(cb, amd64RDX, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64RDX, amd64RAX))

	// newTOP = (TOP - 1) & 7
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1) // SUB ECX, 1
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7) // AND ECX, 7

	// Update FSW with new TOP
	x86EmitUpdateFSWTop(cb, amd64RCX)

	// Store value to new ST(0) = regs[newTOP]
	amd64SHL_imm32(cb, amd64RCX, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64RCX, amd64RAX))

	return true
}

// x86EmitFSTP_STi emits FSTP ST(i) -- copy ST(0) to ST(i), then pop.
func x86EmitFSTP_STi(cb *CodeBuffer, stIdx byte) bool {
	// RAX = FPUPtr
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)

	// Load ST(0): MOVSD XMM0, [RAX + TOP*8]
	amd64MOV_reg_reg32(cb, amd64R8, amd64RCX)
	amd64SHL_imm32(cb, amd64R8, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64R8, amd64RAX))

	// physSTi = (TOP + i) & 7
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RCX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, int32(stIdx))
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 7)
	amd64SHL_imm32(cb, amd64RDX, 3)

	// Store to ST(i)
	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64RDX, amd64RAX))

	// Pop: TOP = (TOP + 1) & 7
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RCX, 1)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)
	x86EmitUpdateFSWTop(cb, amd64RCX)

	return true
}

// x86EmitFST_STi emits FST ST(i) -- copy ST(0) to ST(i), no pop.
func x86EmitFST_STi(cb *CodeBuffer, stIdx byte) bool {
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)

	amd64MOV_reg_reg32(cb, amd64R8, amd64RCX)
	amd64SHL_imm32(cb, amd64R8, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64R8, amd64RAX))

	amd64MOV_reg_reg32(cb, amd64RDX, amd64RCX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, int32(stIdx))
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 7)
	amd64SHL_imm32(cb, amd64RDX, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64RDX, amd64RAX))

	return true
}

// x86EmitFXCH emits FXCH ST(i) -- swap ST(0) and ST(i).
func x86EmitFXCH(cb *CodeBuffer, stIdx byte) bool {
	// RAX = FPUPtr
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)

	// physST0 offset
	amd64MOV_reg_reg32(cb, amd64R8, amd64RCX)
	amd64SHL_imm32(cb, amd64R8, 3)

	// physSTi offset
	amd64MOV_reg_reg32(cb, amd64RDX, amd64RCX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, int32(stIdx))
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RDX, 7)
	amd64SHL_imm32(cb, amd64RDX, 3)

	// XMM0 = ST(0), XMM1 = ST(i)
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64R8, amd64RAX))
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 1, 4), sibByte(0, amd64RDX, amd64RAX))

	// ST(0) = XMM1, ST(i) = XMM0
	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 1, 4), sibByte(0, amd64R8, amd64RAX))
	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64RDX, amd64RAX))

	return true
}

// x86EmitFCHS emits FCHS -- negate ST(0).
func x86EmitFCHS(cb *CodeBuffer) bool {
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)

	amd64SHL_imm32(cb, amd64RCX, 3)
	// Load ST(0) into XMM0
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64RCX, amd64RAX))

	// XOR with sign bit mask: XORPD XMM0, [sign_mask]
	// Use PXOR + shift to create sign mask, or use subtraction from 0
	// Simplest: XORPD with -0.0 loaded into XMM1
	// Load -0.0 into XMM1 via integer path: MOV R8, 0x8000000000000000; MOVQ XMM1, R8
	amd64MOV_reg_imm64(cb, amd64R8, 0x8000000000000000)
	// MOVQ XMM1, R8: 66 49 0F 6E C8
	cb.EmitBytes(0x66)
	emitREX(cb, true, 1, amd64R8)
	cb.EmitBytes(0x0F, 0x6E, modRM(3, 1, amd64R8))
	// XORPD XMM0, XMM1: 66 0F 57 C1
	cb.EmitBytes(0x66, 0x0F, 0x57, modRM(3, 0, 1))

	// Store back
	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64RCX, amd64RAX))
	return true
}

// x86EmitFABS emits FABS -- absolute value of ST(0).
func x86EmitFABS(cb *CodeBuffer) bool {
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)

	amd64SHL_imm32(cb, amd64RCX, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64RCX, amd64RAX))

	// AND with abs mask (clear sign bit)
	amd64MOV_reg_imm64(cb, amd64R8, 0x7FFFFFFFFFFFFFFF)
	cb.EmitBytes(0x66)
	emitREX(cb, true, 1, amd64R8)
	cb.EmitBytes(0x0F, 0x6E, modRM(3, 1, amd64R8))
	// ANDPD XMM0, XMM1: 66 0F 54 C1
	cb.EmitBytes(0x66, 0x0F, 0x54, modRM(3, 0, 1))

	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64RCX, amd64RAX))
	return true
}

// x86EmitFLD_mem64 emits FLD qword [mem] -- push double from memory.
func x86EmitFLD_mem64(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	// Compute EA
	if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
		return false
	}
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)

	// RAX = FPUPtr (reload since IOCheck may have clobbered)
	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffFPUPtr))

	// Load double from [memBase + R10] into XMM0
	// MOVSD XMM0, [RSI + R10]
	cb.EmitBytes(0xF2)
	emitREX_SIB(cb, false, 0, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	// Read TOP, decrement, push
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1) // TOP - 1
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7) // & 7

	x86EmitUpdateFSWTop(cb, amd64RCX)

	// Store to regs[newTOP]
	amd64SHL_imm32(cb, amd64RCX, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64RCX, amd64RAX))

	return true
}

// x86EmitFLD_mem32 emits FLD dword [mem] -- push single from memory, convert to double.
func x86EmitFLD_mem32(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
		return false
	}
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)

	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffFPUPtr))

	// Load float32 from [memBase + R10] into XMM0: MOVSS XMM0, [RSI+R10]
	cb.EmitBytes(0xF3)
	emitREX_SIB(cb, false, 0, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	// Convert float32 to float64: CVTSS2SD XMM0, XMM0
	cb.EmitBytes(0xF3, 0x0F, 0x5A, modRM(3, 0, 0))

	// Push onto FPU stack (same as FLD_mem64 from here)
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)
	x86EmitUpdateFSWTop(cb, amd64RCX)
	amd64SHL_imm32(cb, amd64RCX, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64RCX, amd64RAX))

	return true
}

// x86EmitFSTP_mem64 emits FSTP qword [mem] -- store double to memory, pop.
func x86EmitFSTP_mem64(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
		return false
	}
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)

	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffFPUPtr))

	// Read TOP
	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)

	// Load ST(0): MOVSD XMM0, [RAX + TOP*8]
	amd64MOV_reg_reg32(cb, amd64R8, amd64RCX)
	amd64SHL_imm32(cb, amd64R8, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64R8, amd64RAX))

	// Store to [memBase + R10]: MOVSD [RSI+R10], XMM0
	cb.EmitBytes(0xF2)
	emitREX_SIB(cb, false, 0, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	// Pop: TOP = (TOP + 1) & 7
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RCX, 1)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)
	x86EmitUpdateFSWTop(cb, amd64RCX)

	return true
}

// x86EmitFST_mem64 emits FST qword [mem] -- store double to memory, no pop.
func x86EmitFST_mem64(cb *CodeBuffer, ji *X86JITInstr, memory []byte, instrIdx int) bool {
	if !x86EmitComputeEA(cb, ji, memory, amd64R10) {
		return false
	}
	x86EmitIOCheckMaybeElide(cb, amd64R10, ji, memory, instrIdx)

	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffFPUPtr))

	emitREX(cb, false, amd64RCX, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64RCX, amd64RAX), byte(fpuOffFSW))
	amd64SHR_imm32(cb, amd64RCX, 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, 7)

	amd64MOV_reg_reg32(cb, amd64R8, amd64RCX)
	amd64SHL_imm32(cb, amd64R8, 3)
	cb.EmitBytes(0xF2, 0x0F, 0x10, modRM(0, 0, 4), sibByte(0, amd64R8, amd64RAX))

	cb.EmitBytes(0xF2)
	emitREX_SIB(cb, false, 0, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x0F, 0x11, modRM(0, 0, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))

	return true
}

// x86EmitUpdateFSWTop updates the TOP field in FSW. topReg has the new TOP value (0-7).
// RAX must point to FPU_X87 struct.
func x86EmitUpdateFSWTop(cb *CodeBuffer, topReg byte) {
	// FSW = (FSW & ~TOPMask) | (newTOP << 11)
	// Load current FSW into R11
	emitREX(cb, false, amd64R11, amd64RAX)
	cb.EmitBytes(0x0F, 0xB7, modRM(1, amd64R11, amd64RAX), byte(fpuOffFSW)) // MOVZX R11w, [RAX+FSW]

	// Clear TOP bits: AND R11, ~(7 << 11)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64R11, int32(0xFFFF&^(7<<11))) // AND R11d, ~TOPMask

	// Shift new TOP into position: SHL topReg_copy, 11
	amd64MOV_reg_reg32(cb, amd64R8, topReg)
	amd64SHL_imm32(cb, amd64R8, 11)

	// OR into FSW
	amd64ALU_reg_reg32(cb, 0x09, amd64R11, amd64R8) // OR R11, R8

	// Store back: MOV WORD [RAX + FSW], R11w
	cb.EmitBytes(0x66) // 16-bit prefix
	emitREX(cb, false, amd64R11, amd64RAX)
	cb.EmitBytes(0x89, modRM(1, amd64R11, amd64RAX), byte(fpuOffFSW))
}

// ===========================================================================
// REP String Operation Emitters
// ===========================================================================

// x86EmitREP_MOVSB emits a native loop for REP MOVSB (byte copy ESI->EDI, ECX times).
func x86EmitREP_MOVSB(cb *CodeBuffer, ji *X86JITInstr, instrIdx int) bool {
	// DF check: bail to interpreter if DF=1 (reverse direction)
	x86EmitDFCheck(cb, ji.opcodePC, instrIdx)
	x86EmitLoadGuestReg32(cb, amd64RCX, 1) // count
	x86EmitLoadGuestReg32(cb, amd64R8, 6)  // src
	x86EmitLoadGuestReg32(cb, amd64R10, 7) // dst

	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	doneJmp := amd64Jcc_rel32(cb, amd64CondE)

	// Range-safety: check src and dst page ranges
	// Save src/dst since range check clobbers R8/R11
	amd64MOV_reg_mem(cb, amd64RDX, amd64RSP, 0)  // save RSP[0] to RDX temp (will restore)
	amd64MOV_mem_reg32(cb, amd64RSP, 0, amd64R8) // save src to stack[0]
	x86EmitRangePageCheck(cb, amd64R8, amd64RCX, 1)
	slowJmpSrc := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem32(cb, amd64R8, amd64RSP, 0) // restore src
	x86EmitRangePageCheck(cb, amd64R10, amd64RCX, 1)
	slowJmpDst := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem32(cb, amd64R8, amd64RSP, 0) // restore src again after 2nd check

	// Fast path: both ranges safe
	if x86CurrentCS != nil && x86CurrentCS.host.HasERMS {
		// Hardware REP MOVSB: save RSI, set up RDI/RSI/RCX, REP MOVSB, restore
		amd64MOV_mem_reg(cb, amd64RSP, 24, x86AMD64RegMemBase) // save RSI

		// RSI = memBase + masked_src
		amd64MOV_reg_reg32(cb, amd64R11, amd64R8)
		amd64MOV_reg_reg(cb, x86AMD64RegMemBase, x86AMD64RegMemBase) // keep RSI as 64-bit
		// Actually need: host RSI = memBase + masked_src_offset
		amd64MOV_reg_mem(cb, amd64RDX, amd64RSP, 24)   // RDX = original RSI (memBase)
		amd64MOV_reg_reg(cb, amd64RSI, amd64RDX)       // host RSI = memBase
		amd64ALU_reg_reg(cb, 0x01, amd64RSI, amd64R11) // RSI += masked_src

		// RDI = memBase + masked_dst
		amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
		amd64MOV_reg_reg(cb, amd64RDI, amd64RDX)       // RDI = memBase
		amd64ALU_reg_reg(cb, 0x01, amd64RDI, amd64R11) // RDI += masked_dst

		// RCX already has count
		cb.EmitBytes(0xFC)       // CLD
		cb.EmitBytes(0xF3, 0xA4) // REP MOVSB

		// Save post-REP RSI (source) and RDI (dest) before restoring memBase
		amd64MOV_reg_reg(cb, amd64R11, amd64RSI) // R11 = post-REP source pointer
		amd64MOV_reg_reg(cb, amd64RDX, amd64RDI) // RDX = post-REP dest pointer

		// Restore RSI (memBase)
		amd64MOV_reg_mem(cb, x86AMD64RegMemBase, amd64RSP, 24)

		// Compute new guest offsets: postPtr - memBase
		amd64ALU_reg_reg(cb, 0x29, amd64R11, x86AMD64RegMemBase) // R11 = postSrc - memBase = new ESI
		amd64ALU_reg_reg(cb, 0x29, amd64RDX, x86AMD64RegMemBase) // RDX = postDst - memBase = new EDI
		amd64MOV_reg_reg32(cb, amd64R8, amd64R11)                // R8 = new guest ESI
		amd64MOV_reg_reg32(cb, amd64R10, amd64RDX)               // R10 = new guest EDI
		amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX)               // ECX = 0
	} else {
		// Scalar fast path: no per-iteration masking
		amd64MOV_reg_reg32(cb, amd64R11, amd64R8)
		amd64MOV_reg_reg32(cb, amd64RDX, amd64R10)
		fastLoopLabel := cb.Len()
		x86EmitMemLoad8(cb, amd64RAX, amd64R11)
		x86EmitMemStore8(cb, amd64RDX, amd64RAX)
		emitREX(cb, false, amd64R11, amd64R11)
		cb.EmitBytes(0xFF, modRM(3, 0, amd64R11))
		emitREX(cb, false, amd64RDX, amd64RDX)
		cb.EmitBytes(0xFF, modRM(3, 0, amd64RDX))
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
		fastLoopJmp := amd64Jcc_rel32(cb, amd64CondNE)
		patchRel32(cb, fastLoopJmp, fastLoopLabel)
		amd64MOV_reg_reg32(cb, amd64R8, amd64R11)
		amd64MOV_reg_reg32(cb, amd64R10, amd64RDX)
	}
	fastDoneJmp := amd64JMP_rel32(cb)

	// Slow path: restore src from stack, then per-iteration masking
	slowLabel := cb.Len()
	patchRel32(cb, slowJmpSrc, slowLabel)
	patchRel32(cb, slowJmpDst, slowLabel)
	amd64MOV_reg_mem32(cb, amd64R8, amd64RSP, 0) // restore src from stack
	slowLoopLabel := cb.Len()
	amd64MOV_reg_reg32(cb, amd64R11, amd64R8)
	x86EmitMemLoad8(cb, amd64RAX, amd64R11)
	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	x86EmitMemStore8(cb, amd64R11, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R8, 1)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	slowLoopJmp := amd64Jcc_rel32(cb, amd64CondNE)
	patchRel32(cb, slowLoopJmp, slowLoopLabel)

	doneLabel := cb.Len()
	patchRel32(cb, doneJmp, doneLabel)
	patchRel32(cb, fastDoneJmp, doneLabel)

	x86EmitStoreGuestReg32(cb, 1, amd64RCX)
	x86EmitStoreGuestReg32(cb, 6, amd64R8)
	x86EmitStoreGuestReg32(cb, 7, amd64R10)
	return true
}

// x86EmitREP_MOVSD emits a native loop for REP MOVSD (dword copy ESI->EDI, ECX times).
func x86EmitREP_MOVSD(cb *CodeBuffer, ji *X86JITInstr, instrIdx int) bool {
	x86EmitDFCheck(cb, ji.opcodePC, instrIdx)
	x86EmitLoadGuestReg32(cb, amd64RCX, 1)
	x86EmitLoadGuestReg32(cb, amd64R8, 6)
	x86EmitLoadGuestReg32(cb, amd64R10, 7)

	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	doneJmp := amd64Jcc_rel32(cb, amd64CondE)

	// Range-safety: save src, check pages for 4-byte stride
	amd64MOV_mem_reg32(cb, amd64RSP, 0, amd64R8) // save src
	x86EmitRangePageCheck(cb, amd64R8, amd64RCX, 4)
	slowJmpSrc := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem32(cb, amd64R8, amd64RSP, 0) // restore src
	x86EmitRangePageCheck(cb, amd64R10, amd64RCX, 4)
	slowJmpDst := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem32(cb, amd64R8, amd64RSP, 0) // restore src

	// Fast path
	amd64MOV_reg_reg32(cb, amd64R11, amd64R8)
	amd64MOV_reg_reg32(cb, amd64RDX, amd64R10)
	fastLoopLabel := cb.Len()
	x86EmitMemLoad32(cb, amd64RAX, amd64R11)
	x86EmitMemStore32(cb, amd64RDX, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, 4)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RDX, 4)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	fastLoopJmp := amd64Jcc_rel32(cb, amd64CondNE)
	patchRel32(cb, fastLoopJmp, fastLoopLabel)
	amd64MOV_reg_reg32(cb, amd64R8, amd64R11)
	amd64MOV_reg_reg32(cb, amd64R10, amd64RDX)
	fastDoneJmp := amd64JMP_rel32(cb)

	// Slow path: restore src, then per-iteration masking
	slowLabel := cb.Len()
	patchRel32(cb, slowJmpSrc, slowLabel)
	patchRel32(cb, slowJmpDst, slowLabel)
	amd64MOV_reg_mem32(cb, amd64R8, amd64RSP, 0) // restore src
	slowLoopLabel := cb.Len()
	amd64MOV_reg_reg32(cb, amd64R11, amd64R8)
	x86EmitMemLoad32(cb, amd64RAX, amd64R11)
	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	x86EmitMemStore32(cb, amd64R11, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R8, 4)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 4)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	slowLoopJmp := amd64Jcc_rel32(cb, amd64CondNE)
	patchRel32(cb, slowLoopJmp, slowLoopLabel)

	doneLabel := cb.Len()
	patchRel32(cb, doneJmp, doneLabel)
	patchRel32(cb, fastDoneJmp, doneLabel)

	x86EmitStoreGuestReg32(cb, 1, amd64RCX)
	x86EmitStoreGuestReg32(cb, 6, amd64R8)
	x86EmitStoreGuestReg32(cb, 7, amd64R10)
	return true
}

// x86EmitDFCheck emits a runtime check of the guest Direction Flag (DF, bit 10 of Flags).
// If DF=1, returns false (bails to interpreter). If DF=0, continues.
// Returns the JNZ offset for patching if the caller wants to handle the bail inline,
// or -1 if the check is skipped.
func x86EmitDFCheck(cb *CodeBuffer, retPC uint32, instrIdx int) {
	// Load guest Flags from context
	amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffFlagsPtr))
	amd64MOV_reg_mem32(cb, amd64RAX, amd64RAX, 0) // EAX = *FlagsPtr

	// TEST EAX, (1 << 10) = 0x400 (DF bit)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, 0) // dummy to not affect actual value
	// Actually: TEST EAX, 0x400
	emitREX(cb, false, 0, amd64RAX)
	cb.EmitBytes(0xF7, modRM(3, 0, amd64RAX)) // TEST EAX, imm32
	cb.Emit32(0x400)                          // DF = bit 10

	// If DF=1: bail via deferred stub
	if x86CurrentBails != nil {
		jccOff := amd64Jcc_rel32(cb, amd64CondNE) // JNZ = DF is set
		*x86CurrentBails = append(*x86CurrentBails, x86DeferredBail{
			jccOffset: jccOff, retPC: retPC, instrIdx: instrIdx, kind: 0,
		})
	}
}

// x86EmitRangePageCheck emits code to scan IO bitmap pages from baseReg to
// baseReg + countReg*stride - 1. Sets ZF=1 if all pages safe, ZF=0 (NE) if any unsafe.
// Clobbers R8 and R11. baseReg and countReg are NOT modified.
func x86EmitRangePageCheck(cb *CodeBuffer, baseReg byte, countReg byte, stride int) {
	// startPage = (base & mask) >> 8
	amd64MOV_reg_reg32(cb, amd64R8, baseReg)
	amd64SHR_imm32(cb, amd64R8, 8)

	// endPage = ((base + count*stride - 1) & mask) >> 8
	amd64MOV_reg_reg32(cb, amd64R11, countReg)
	if stride > 1 {
		// IMUL R11, stride: use SHL for powers of 2
		switch stride {
		case 2:
			amd64SHL_imm32(cb, amd64R11, 1)
		case 4:
			amd64SHL_imm32(cb, amd64R11, 2)
		}
	}
	amd64ALU_reg_reg32(cb, 0x01, amd64R11, baseReg) // R11 = base + count*stride
	amd64ALU_reg_imm32_32bit(cb, 5, amd64R11, 1)    // -1
	amd64SHR_imm32(cb, amd64R11, 8)                 // endPage

	// Scan: for p = startPage; p <= endPage; p++ if bitmap[p] → set NE
	scanLabel := cb.Len()
	emitREX_SIB(cb, false, 0, amd64R8, x86AMD64RegIOBM)
	cb.EmitBytes(0xF6, modRM(0, 0, 4), sibByte(0, amd64R8, x86AMD64RegIOBM))
	cb.EmitBytes(0x01) // TEST BYTE [R9+R8], 1
	// If any page unsafe, the JNZ after this function call handles it
	unsafeOff := amd64Jcc_rel32(cb, amd64CondNE)

	emitREX(cb, false, amd64R8, amd64R8)
	cb.EmitBytes(0xFF, modRM(3, 0, amd64R8)) // INC R8
	emitREX(cb, false, amd64R8, amd64R11)
	cb.EmitBytes(0x39, modRM(3, amd64R11, amd64R8)) // CMP R8, R11
	scanLoopJmp := amd64Jcc_rel32(cb, amd64CondBE)
	patchRel32(cb, scanLoopJmp, scanLabel)

	// All safe: set ZF=1 (XOR R8, R8 sets ZF)
	amd64XOR_reg_reg32(cb, amd64R8, amd64R8) // ZF=1
	safeJmp := amd64JMP_rel32(cb)

	// Unsafe: set ZF=0
	unsafeLabel := cb.Len()
	patchRel32(cb, unsafeOff, unsafeLabel)
	// TEST with non-zero to ensure NE
	amd64MOV_reg_imm32(cb, amd64R8, 1)
	emitREX(cb, false, amd64R8, amd64R8)
	cb.EmitBytes(0x85, modRM(3, amd64R8, amd64R8)) // TEST R8, R8 → NE

	safeLabel := cb.Len()
	patchRel32(cb, safeJmp, safeLabel)
	// Caller checks JNE for unsafe
}

// x86EmitREP_STOSB emits a native loop for REP STOSB (fill EDI with AL, ECX times).
// Includes range-safety fast path: verifies all destination pages are non-I/O upfront.
func x86EmitREP_STOSB(cb *CodeBuffer, ji *X86JITInstr, instrIdx int) bool {
	x86EmitDFCheck(cb, ji.opcodePC, instrIdx)
	x86EmitLoadGuestReg32(cb, amd64RCX, 1) // count
	x86EmitLoadGuestReg32(cb, amd64R10, 7) // dst
	x86EmitLoadGuestReg32(cb, amd64RAX, 0) // AL (low byte of EAX)

	// TEST ECX, ECX; JZ done
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	doneJmp := amd64Jcc_rel32(cb, amd64CondE)

	// Range-safety check: verify all pages in [EDI, EDI+ECX) are non-I/O
	// Start page = (EDI & mask) >> 8, End page = ((EDI + ECX - 1) & mask) >> 8
	// Scan all pages from start to end in the IO bitmap
	amd64MOV_reg_reg32(cb, amd64R8, amd64R10) // R8 = EDI
	amd64SHR_imm32(cb, amd64R8, 8)            // start page

	// R11 = end page = ((EDI + ECX - 1) & mask) >> 8
	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	amd64ALU_reg_reg32(cb, 0x01, amd64R11, amd64RCX) // R11 = EDI + ECX
	amd64ALU_reg_imm32_32bit(cb, 5, amd64R11, 1)     // -1
	amd64SHR_imm32(cb, amd64R11, 8)                  // end page

	// Scan pages: for p = startPage; p <= endPage; p++ { if bitmap[p] { goto slow } }
	scanLabel := cb.Len()
	emitREX_SIB(cb, false, 0, amd64R8, x86AMD64RegIOBM)
	cb.EmitBytes(0xF6, modRM(0, 0, 4), sibByte(0, amd64R8, x86AMD64RegIOBM))
	cb.EmitBytes(0x01)                         // TEST BYTE [R9+R8], 1
	slowJmp := amd64Jcc_rel32(cb, amd64CondNE) // page has I/O → slow path

	emitREX(cb, false, amd64R8, amd64R8)
	cb.EmitBytes(0xFF, modRM(3, 0, amd64R8)) // INC R8d
	emitREX(cb, false, amd64R8, amd64R11)
	cb.EmitBytes(0x39, modRM(3, amd64R11, amd64R8)) // CMP R8d, R11d
	scanLoopJmp := amd64Jcc_rel32(cb, amd64CondBE)
	patchRel32(cb, scanLoopJmp, scanLabel) // JBE scan (unsigned <= )

	// All pages safe → mask base once, then fast loop
	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)

	// Hardware REP STOSB fast path: when ERMS available and DF=0, use native REP STOSB
	if x86CurrentCS != nil && x86CurrentCS.host.HasERMS {
		// Save RSI (our memory base) to stack
		amd64MOV_mem_reg(cb, amd64RSP, 24, x86AMD64RegMemBase) // [RSP+24] = RSI

		// Set up for native REP STOSB: RDI = memBase + masked_dest, AL = fill, RCX = count
		amd64MOV_reg_reg(cb, amd64RDI, x86AMD64RegMemBase)
		amd64ALU_reg_reg(cb, 0x01, amd64RDI, amd64R11) // RDI = RSI + masked_offset

		// CLD (ensure host DF=0 -- we already checked guest DF=0)
		cb.EmitBytes(0xFC) // CLD

		// REP STOSB: F3 AA
		cb.EmitBytes(0xF3, 0xAA)

		// Restore RSI
		amd64MOV_reg_mem(cb, x86AMD64RegMemBase, amd64RSP, 24)

		// Update guest EDI: R10 = R11 + bytes_written. Since RCX=0 after REP,
		// RDI = original RDI + count. We can compute: R10 = RDI - memBase
		amd64MOV_reg_reg(cb, amd64R10, amd64RDI)
		amd64ALU_reg_reg(cb, 0x29, amd64R10, x86AMD64RegMemBase) // R10 = RDI - RSI
		// ECX is already 0 from REP
		amd64XOR_reg_reg32(cb, amd64RCX, amd64RCX)

		fastDoneJmp := amd64JMP_rel32(cb)
		_ = fastDoneJmp

		// Slow path (some pages are I/O)
		slowLabel := cb.Len()
		patchRel32(cb, slowJmp, slowLabel)
		slowLoopLabel := cb.Len()
		amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
		x86EmitMemStore8(cb, amd64R11, amd64RAX)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
		slowLoopJmp := amd64Jcc_rel32(cb, amd64CondNE)
		patchRel32(cb, slowLoopJmp, slowLoopLabel)

		doneLabel := cb.Len()
		patchRel32(cb, doneJmp, doneLabel)
		patchRel32(cb, fastDoneJmp, doneLabel)

		x86EmitStoreGuestReg32(cb, 1, amd64RCX)
		x86EmitStoreGuestReg32(cb, 7, amd64R10)
		return true
	}

	// Scalar fast path: byte loop without per-iteration masking
	fastLoopLabel := cb.Len()
	x86EmitMemStore8(cb, amd64R11, amd64RAX)
	emitREX(cb, false, amd64R11, amd64R11)
	cb.EmitBytes(0xFF, modRM(3, 0, amd64R11))    // INC R11d
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1) // DEC count
	fastLoopJmpBack := amd64Jcc_rel32(cb, amd64CondNE)
	patchRel32(cb, fastLoopJmpBack, fastLoopLabel)
	amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
	fastDoneJmp := amd64JMP_rel32(cb)

	// Slow path: per-iteration masked loop (original behavior)
	slowLabel := cb.Len()
	patchRel32(cb, slowJmp, slowLabel)
	// Reload count (it was not modified by the scan)
	slowLoopLabel := cb.Len()
	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	x86EmitMemStore8(cb, amd64R11, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	slowLoopJmp := amd64Jcc_rel32(cb, amd64CondNE)
	patchRel32(cb, slowLoopJmp, slowLoopLabel)

	// done:
	doneLabel := cb.Len()
	patchRel32(cb, doneJmp, doneLabel)
	patchRel32(cb, fastDoneJmp, doneLabel)

	x86EmitStoreGuestReg32(cb, 1, amd64RCX)
	x86EmitStoreGuestReg32(cb, 7, amd64R10)
	return true
}

// x86EmitREP_STOSD emits a native loop for REP STOSD (fill EDI with EAX, ECX times).
func x86EmitREP_STOSD(cb *CodeBuffer, ji *X86JITInstr, instrIdx int) bool {
	x86EmitDFCheck(cb, ji.opcodePC, instrIdx)
	x86EmitLoadGuestReg32(cb, amd64RCX, 1)
	x86EmitLoadGuestReg32(cb, amd64R10, 7)
	x86EmitLoadGuestReg32(cb, amd64RAX, 0)

	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	doneJmp := amd64Jcc_rel32(cb, amd64CondE)

	// Range-safety for 4-byte stride
	x86EmitRangePageCheck(cb, amd64R10, amd64RCX, 4)
	slowJmp := amd64Jcc_rel32(cb, amd64CondNE)

	// Fast path
	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	fastLoopLabel := cb.Len()
	x86EmitMemStore32(cb, amd64R11, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R11, 4)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	fastLoopJmp := amd64Jcc_rel32(cb, amd64CondNE)
	patchRel32(cb, fastLoopJmp, fastLoopLabel)
	amd64MOV_reg_reg32(cb, amd64R10, amd64R11)
	fastDoneJmp := amd64JMP_rel32(cb)

	// Slow path
	slowLabel := cb.Len()
	patchRel32(cb, slowJmp, slowLabel)
	slowLoopLabel := cb.Len()
	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	x86EmitMemStore32(cb, amd64R11, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 4)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)
	slowLoopJmp := amd64Jcc_rel32(cb, amd64CondNE)
	patchRel32(cb, slowLoopJmp, slowLoopLabel)

	doneLabel := cb.Len()
	patchRel32(cb, doneJmp, doneLabel)
	patchRel32(cb, fastDoneJmp, doneLabel)

	x86EmitStoreGuestReg32(cb, 1, amd64RCX)
	x86EmitStoreGuestReg32(cb, 7, amd64R10)
	return true
}

// ===========================================================================
// REP CMPSB / REP SCASB Emitters
// ===========================================================================

// x86EmitREP_CMPSB emits REPE/REPNE CMPSB: compare ESI vs EDI bytes, ECX times.
func x86EmitREP_CMPSB(cb *CodeBuffer, ji *X86JITInstr, instrIdx int, cs *x86CompileState) bool {
	x86EmitDFCheck(cb, ji.opcodePC, instrIdx)
	isRepNE := ji.prefixes&x86PrefRepNE != 0

	x86EmitLoadGuestReg32(cb, amd64RCX, 1) // count
	x86EmitLoadGuestReg32(cb, amd64R8, 6)  // ESI
	x86EmitLoadGuestReg32(cb, amd64R10, 7) // EDI

	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	doneJmp := amd64Jcc_rel32(cb, amd64CondE)

	loopLabel := cb.Len()

	// Load [ESI] and [EDI], compare
	amd64MOV_reg_reg32(cb, amd64R11, amd64R8)
	x86EmitMemLoad8(cb, amd64RAX, amd64R11) // AL = [ESI]

	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	x86EmitMemLoad8(cb, amd64RDX, amd64R11) // DL = [EDI]

	// CMP AL, DL — sets the comparison flags REPE/REPNE will branch on.
	cb.EmitBytes(0x38, modRM(3, amd64RDX, amd64RAX)) // CMP AL, DL

	// LAHF immediately to capture the comparison flags before any host
	// arithmetic clobbers them. The INC ESI/EDI and DEC ECX below modify
	// host EFLAGS; without this capture, the JE/JNE later in the loop
	// would test those instead of the CMP and exit after one iteration.
	cb.EmitBytes(0x9F) // LAHF

	// ESI++, EDI++, ECX-- (clobber EFLAGS — fine, captured value sits in AH).
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R8, 1)
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)

	// Check termination: ECX == 0 → done
	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	ecxZeroJmp := amd64Jcc_rel32(cb, amd64CondE)

	// Restore flags for comparison check
	cb.EmitBytes(0x9E) // SAHF

	// REPE: continue if equal (ZF=1); REPNE: continue if not equal (ZF=0)
	var continueJmp int
	if isRepNE {
		continueJmp = amd64Jcc_rel32(cb, amd64CondNE) // JNE loop (continue while not equal)
	} else {
		continueJmp = amd64Jcc_rel32(cb, amd64CondE) // JE loop (continue while equal)
	}
	patchRel32(cb, continueJmp, loopLabel)

	// Termination: mismatch (REPE) or match (REPNE)
	terminateJmp := amd64JMP_rel32(cb)

	// ECX == 0 exit
	ecxZeroLabel := cb.Len()
	patchRel32(cb, ecxZeroJmp, ecxZeroLabel)
	cb.EmitBytes(0x9E) // SAHF (restore comparison flags)

	terminateLabel := cb.Len()
	patchRel32(cb, terminateJmp, terminateLabel)

	doneLabel := cb.Len()
	patchRel32(cb, doneJmp, doneLabel)

	cs.flagState = x86FlagsLiveArith // CMP result in flags
	x86EmitStoreGuestReg32(cb, 1, amd64RCX)
	x86EmitStoreGuestReg32(cb, 6, amd64R8)
	x86EmitStoreGuestReg32(cb, 7, amd64R10)
	return true
}

// x86EmitREP_SCASB emits REPE/REPNE SCASB: scan EDI for AL match, ECX times.
func x86EmitREP_SCASB(cb *CodeBuffer, ji *X86JITInstr, instrIdx int, cs *x86CompileState) bool {
	x86EmitDFCheck(cb, ji.opcodePC, instrIdx)
	isRepNE := ji.prefixes&x86PrefRepNE != 0

	x86EmitLoadGuestReg32(cb, amd64RCX, 1) // count
	x86EmitLoadGuestReg32(cb, amd64R10, 7) // EDI
	x86EmitLoadGuestReg32(cb, amd64RAX, 0) // AL (search byte)

	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	doneJmp := amd64Jcc_rel32(cb, amd64CondE)

	loopLabel := cb.Len()

	// Load [EDI], compare with AL
	amd64MOV_reg_reg32(cb, amd64R11, amd64R10)
	x86EmitMemLoad8(cb, amd64RDX, amd64R11) // DL = [EDI]

	// CMP AL, DL — sets the comparison flags REPE/REPNE will branch on.
	cb.EmitBytes(0x38, modRM(3, amd64RDX, amd64RAX))

	// LAHF immediately to capture the comparison flags before any host
	// arithmetic clobbers them. Same fix as REP_CMPSB.
	cb.EmitBytes(0x9F) // LAHF

	// EDI++, ECX-- (clobber EFLAGS — fine, captured value sits in AH).
	amd64ALU_reg_imm32_32bit(cb, 0, amd64R10, 1)
	amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1)

	emitREX(cb, false, amd64RCX, amd64RCX)
	cb.EmitBytes(0x85, modRM(3, amd64RCX, amd64RCX))
	ecxZeroJmp := amd64Jcc_rel32(cb, amd64CondE)

	cb.EmitBytes(0x9E) // SAHF

	var continueJmp int
	if isRepNE {
		continueJmp = amd64Jcc_rel32(cb, amd64CondNE)
	} else {
		continueJmp = amd64Jcc_rel32(cb, amd64CondE)
	}
	patchRel32(cb, continueJmp, loopLabel)
	terminateJmp := amd64JMP_rel32(cb)

	ecxZeroLabel := cb.Len()
	patchRel32(cb, ecxZeroJmp, ecxZeroLabel)
	cb.EmitBytes(0x9E)

	terminateLabel := cb.Len()
	patchRel32(cb, terminateJmp, terminateLabel)

	doneLabel := cb.Len()
	patchRel32(cb, doneJmp, doneLabel)

	cs.flagState = x86FlagsLiveArith
	x86EmitStoreGuestReg32(cb, 1, amd64RCX)
	x86EmitStoreGuestReg32(cb, 7, amd64R10)
	return true
}

// ===========================================================================
// Jcc rel8 Emitter
// ===========================================================================

// x86EmitJcc_rel8 handles Jcc rel8 (0x70-0x7F).
// For backward branches (loops), we emit a chain exit when condition is true.
// For forward branches, we emit a native Jcc that skips instructions.
// Currently only backward branches (loop exits) are supported in Tier 1.
func x86EmitJcc_rel8(cb *CodeBuffer, ji *X86JITInstr, memory []byte, startPC uint32, cs *x86CompileState, instrIdx int) bool {
	op := byte(ji.opcode)
	cond := op - 0x70 // x86 condition code (0-15)

	immPC := ji.opcodePC + uint32(ji.length) - 1
	rel := int32(int8(memory[immPC]))
	nextPC := ji.opcodePC + uint32(ji.length)
	targetPC := uint32(int32(nextPC) + rel)

	if cs.flagState != x86FlagsLiveArith && cs.flagState != x86FlagsLiveLogic && cs.flagState != x86FlagsLiveInc {
		return false // flags not live
	}

	// Self-loop: backward Jcc to startPC → native loop with budget counter
	if cs.isLoop && targetPC == startPC && cs.loopStartLabel > 0 {
		instrThisIter := instrIdx + 1 // guest instructions in this iteration (including Jcc)

		// Accumulate retired instructions: [RSP+OffLoopRetired] += instrThisIter
		// Use LAHF/SAHF to preserve flags across the counter updates
		cb.EmitBytes(0x9F) // LAHF -- save flags to AH
		amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, int32(x86AMD64OffLoopRetired))
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RCX, int32(instrThisIter)) // ADD
		amd64MOV_mem_reg32(cb, amd64RSP, int32(x86AMD64OffLoopRetired), amd64RCX)

		// Decrement budget: [RSP+OffLoopBudget] -= 1
		amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, int32(x86AMD64OffLoopBudget))
		amd64ALU_reg_imm32_32bit(cb, 5, amd64RCX, 1) // SUB
		amd64MOV_mem_reg32(cb, amd64RSP, int32(x86AMD64OffLoopBudget), amd64RCX)
		cb.EmitBytes(0x9E) // SAHF -- restore flags from AH

		// If budget exhausted (RCX was <= 0 before SAHF restored flags), exit
		// We check budget separately: TEST ECX, ECX after the SAHF
		// But SAHF restored the guest flags, not the budget comparison...
		// Solution: save the budget-exhausted condition before SAHF
		// Actually simpler: check budget BEFORE the Jcc, then do the Jcc

		// The approach: LAHF saves guest flags. Do budget work. SAHF restores them.
		// Then emit native Jcc for the loop condition.
		// If budget <= 0, jump to exhaustion exit instead of loop.

		// Budget exhaustion check: RCX still has the decremented budget
		// TEST ECX, ECX was clobbered by SAHF. We need another approach.
		// Save budget <= 0 into a scratch before SAHF:
		// After SUB RCX, 1: SETBE R8b (budget exhausted if <= 0, treating as signed: SETLE)
		// Then after SAHF, check R8b

		// Let me restructure: do budget accounting with explicit flag save/restore
		// Actually, the simplest correct approach:

		// 1. Save flags with PUSHFQ (push native RFLAGS to stack)
		// 2. Do budget accounting
		// 3. If budget exhausted, jump to exit
		// 4. POPFQ (restore native RFLAGS)
		// 5. Emit native Jcc back to loop start

		// But PUSHFQ/POPFQ are expensive. Better approach:
		// Use a register we don't care about for the budget check.
		// R11 is scratch and never holds guest state at this point.

		// Restructure completely:
		// 1. Accumulate retired count (doesn't affect flags if we use LEA)
		// 2. Decrement budget (doesn't need to affect flags if we use LEA+CMP)
		// 3. If budget exhausted → exit
		// 4. Emit native Jcc (guest flags still live from the DEC/CMP before us)

		// Wait - the guest flags ARE still live because we haven't emitted any
		// flag-affecting instruction yet. The accumulation and budget check can
		// use LEA (doesn't affect flags) and CMP/TEST on scratch.

		// But SUB/ADD affect flags. Use LEA instead:
		// LEA RCX, [retired + instrThisIter]  -- doesn't affect flags!
		// LEA R11, [budget - 1]               -- doesn't affect flags!

		// This is the key insight. Let me redo this cleanly.

		// Guest flags are live in host EFLAGS from the previous instruction.
		// We must NOT clobber EFLAGS before the Jcc.

		// Use LEA for arithmetic (doesn't touch flags):
		// Load retired count, add instrThisIter via LEA, store back
		amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, int32(x86AMD64OffLoopRetired))
		// LEA ECX, [RCX + instrThisIter]: 8D 89 imm32
		emitREX(cb, false, amd64RCX, amd64RCX)
		cb.EmitBytes(0x8D, modRM(2, amd64RCX, amd64RCX)) // LEA ECX, [RCX+disp32]
		cb.Emit32(uint32(instrThisIter))
		amd64MOV_mem_reg32(cb, amd64RSP, int32(x86AMD64OffLoopRetired), amd64RCX)

		// Load budget, subtract 1 via LEA, store back
		amd64MOV_reg_mem32(cb, amd64R11, amd64RSP, int32(x86AMD64OffLoopBudget))
		// LEA R11d, [R11 - 1]
		emitREX(cb, false, amd64R11, amd64R11)
		cb.EmitBytes(0x8D, modRM(1, amd64R11, amd64R11), 0xFF) // LEA R11d, [R11-1] (disp8=-1)
		amd64MOV_mem_reg32(cb, amd64RSP, int32(x86AMD64OffLoopBudget), amd64R11)

		// Now emit the native Jcc for the loop condition (flags still live!)
		// If condition true → check budget and loop back
		// If condition false → fall through (loop done)

		// Jcc to loopContinue label
		loopContOff := amd64Jcc_rel32(cb, cond)

		// Fall-through: loop condition false → exit normally
		fallThroughJmp := amd64JMP_rel32(cb)

		// loopContinue: condition was true, check budget
		loopContLabel := cb.Len()
		patchRel32(cb, loopContOff, loopContLabel)

		// TEST R11d, R11d (is budget <= 0? R11 has budget-1, so check if < 0)
		emitREX(cb, false, amd64R11, amd64R11)
		cb.EmitBytes(0x85, modRM(3, amd64R11, amd64R11)) // TEST R11d, R11d

		// JLE budgetExhausted (if budget <= 0, exit to Go)
		budgetExhOff := amd64Jcc_rel32(cb, amd64CondLE)

		// Budget OK → native JMP back to loop start
		loopBackOff := amd64JMP_rel32(cb)
		patchRel32(cb, loopBackOff, cs.loopStartLabel)

		// budgetExhausted: store RetPC = startPC (re-enter loop next time)
		budgetExhLabel := cb.Len()
		patchRel32(cb, budgetExhOff, budgetExhLabel)

		// RetPC = startPC, RetCount = loopRetiredCount
		x86EmitRetPC(cb, startPC, 0) // placeholder count
		// Overwrite RetCount with actual retired count from stack
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(x86AMD64OffLoopRetired))
		amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffRetCount), amd64RAX)
		x86EmitLightweightEpilogue(cb)
		x86EmitFullEpilogueEnd(cb)

		// Fall-through: loop done (condition false)
		fallThroughLabel := cb.Len()
		patchRel32(cb, fallThroughJmp, fallThroughLabel)

		// RetPC = nextPC (after Jcc), RetCount = loopRetiredCount
		// (will be set by the block compiler's normal exit path)

		return true
	}

	// Non-self-loop Jcc: emit as conditional CHAIN exit. Taken branch
	// runs the full chain-exit sequence (lightweight epilog, ChainCount
	// accum, ChainBudget DEC, NeedInval check, patchable JMP rel32 to
	// target.chainEntry — initial JMP goes to the unchained-exit blob).
	// The chain slot is registered with the block compiler so
	// x86PatchCompatibleChainsTo wires it once target is cached.
	// Fall-through skips the entire chain+unchained blob (which
	// terminates with RET and is therefore unreachable by fall-through).
	exitOff := amd64Jcc_rel32(cb, cond)
	fallThroughJmp := amd64JMP_rel32(cb)

	exitLabel := cb.Len()
	patchRel32(cb, exitOff, exitLabel)
	info := x86EmitChainExit(cb, targetPC, uint32(instrIdx+1))
	if cs.chainExits != nil {
		*cs.chainExits = append(*cs.chainExits, info)
	}

	fallThroughLabel := cb.Len()
	patchRel32(cb, fallThroughJmp, fallThroughLabel)

	return true
}

// ===========================================================================
// LEA Emitter
// ===========================================================================

func x86EmitLEA(cb *CodeBuffer, ji *X86JITInstr, memory []byte) bool {
	if !ji.hasModRM {
		return false
	}
	mod := ji.modrm >> 6
	if mod == 3 {
		return false // LEA with mod=3 is undefined
	}

	dstReg := (ji.modrm >> 3) & 7
	rm := ji.modrm & 7

	// For Tier 1, handle the common case: LEA reg, [reg + disp8]
	if mod == 1 && rm != 4 { // disp8, no SIB
		baseReg := rm
		dispPC := ji.opcodePC + 2 // opcode + modrm
		// Account for prefixes
		prefixBytes := uint32(ji.length) - 3 // total - opcode - modrm - disp8
		if prefixBytes > 0 {
			dispPC += prefixBytes
		}
		dispPC = ji.opcodePC + uint32(ji.length) - 1 // disp8 is last byte
		disp := int32(int8(memory[dispPC]))

		x86EmitLoadGuestReg32(cb, amd64R8, baseReg)
		amd64ALU_reg_imm32_32bit(cb, 0, amd64R8, disp) // ADD R8d, disp
		x86EmitStoreGuestReg32(cb, dstReg, amd64R8)
		return true
	}

	return false // Other LEA forms not yet in Tier 1
}

// ===========================================================================
// Flag Manipulation Emitters
// ===========================================================================

func x86EmitFlagManip(cb *CodeBuffer, ji *X86JITInstr, cs *x86CompileState) bool {
	// These modify guest Flags directly, not via ALU
	// For now, fall back to interpreter for complex flag ops
	// TODO: implement in Tier 2
	return false
}

// ===========================================================================
// XCHG EAX, r32
// ===========================================================================

func x86EmitXCHG_EAX_r32(cb *CodeBuffer, ji *X86JITInstr) bool {
	guestReg := byte(ji.opcode) - 0x90

	x86EmitLoadGuestReg32(cb, amd64R8, 0)         // R8 = EAX
	x86EmitLoadGuestReg32(cb, amd64R10, guestReg) // R10 = other
	x86EmitStoreGuestReg32(cb, 0, amd64R10)       // EAX = other
	x86EmitStoreGuestReg32(cb, guestReg, amd64R8) // other = EAX
	return true
}

// ===========================================================================
// Sign Extend Emitters (CBW/CWDE, CWD/CDQ)
// ===========================================================================

func x86EmitSignExtend(cb *CodeBuffer, ji *X86JITInstr) bool {
	op := byte(ji.opcode)

	if op == 0x98 { // CWDE (sign-extend AX to EAX)
		x86EmitLoadGuestReg32(cb, amd64RAX, 0)
		// MOVSX EAX, AX: 0F BF C0
		cb.EmitBytes(0x0F, 0xBF, 0xC0)
		x86EmitStoreGuestReg32(cb, 0, amd64RAX)
		return true
	}

	if op == 0x99 { // CDQ (sign-extend EAX to EDX:EAX)
		x86EmitLoadGuestReg32(cb, amd64RAX, 0)
		// CDQ: 99
		cb.EmitBytes(0x99)
		// EDX = sign extension of EAX
		x86EmitStoreGuestReg32(cb, 2, amd64RDX) // guest EDX
		return true
	}

	return false
}

// ===========================================================================
// Block Chaining — Chain Entry / Lightweight Epilogue / Chain Exit
// ===========================================================================

// x86ChainExitInfo records a patchable chain exit point.
type x86ChainExitInfo struct {
	targetPC      uint32 // guest x86 PC this exit targets
	jmpDispOffset int    // offset within CodeBuffer of the JMP rel32 displacement
}

// x86EmitChainEntry emits the lightweight chain entry point.
// Chained blocks JMP directly here, skipping the full prologue.
// Must reload all mapped registers from jitRegs since the previous block stored them.
// Returns the code buffer offset of the chain entry label.
func x86EmitChainEntry(cb *CodeBuffer, cs *x86CompileState) int {
	entryOff := cb.Len()

	// In Tier 1 with fixed mapping, mapped registers are LIVE in host callee-saved
	// registers from the previous block's execution. No reload needed for mapped regs.
	// Only spilled registers (if any are read by this block) need loading.
	// For simplicity and correctness on first chain entry (from prologue fall-through),
	// we still reload -- but the prologue already loaded them, so these are redundant
	// on first entry. The cost is acceptable since chain entry happens once per block
	// and the registers are in L1 cache.

	// Skip register reload entirely -- mapped registers stay live across chains.
	// Spilled register loads happen on-demand in the instruction emitters.

	return entryOff
}

// x86EmitLightweightEpilogue stores mapped registers back to jitRegs.
// Does NOT pop callee-saved or RET -- used before chain exits.
func x86EmitLightweightEpilogue(cb *CodeBuffer) {
	cs := x86CurrentCS
	if cs == nil {
		// Fallback for calls outside compilation context — store all mapped
		amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffJITRegsPtr))
		amd64MOV_mem_reg32(cb, amd64RAX, 0*4, x86AMD64RegGuestEAX)
		amd64MOV_mem_reg32(cb, amd64RAX, 1*4, x86AMD64RegGuestECX)
		amd64MOV_mem_reg32(cb, amd64RAX, 2*4, x86AMD64RegGuestEDX)
		amd64MOV_mem_reg32(cb, amd64RAX, 3*4, x86AMD64RegGuestEBX)
		amd64MOV_mem_reg32(cb, amd64RAX, 4*4, x86AMD64RegGuestESP)
		return
	}

	// Store only dirty mapped registers
	dirty := cs.dirtyMask
	needStore := false
	for guest := byte(0); guest < 8; guest++ {
		if cs.regMap[guest] != 0 && dirty&(1<<guest) != 0 {
			needStore = true
			break
		}
	}
	if needStore {
		amd64MOV_reg_mem(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffJITRegsPtr))
		for guest := byte(0); guest < 8; guest++ {
			if host := cs.regMap[guest]; host != 0 && dirty&(1<<guest) != 0 {
				amd64MOV_mem_reg32(cb, amd64RAX, int32(guest)*4, host)
			}
		}
	}
}

// x86EmitFullEpilogueEnd emits the stack frame dealloc + callee-saved pop + RET.
// Also runs the deferred Flags-to-guest merge so chain-exit unchained paths
// (which reach this through x86EmitChainExit's tail) restore guest Flags
// alongside the regs-to-jitRegs writeback.
func x86EmitFullEpilogueEnd(cb *CodeBuffer) {
	x86EmitMergeFlagsToGuest(cb)
	amd64ALU_reg_imm32(cb, 0, amd64RSP, int32(x86AMD64FrameSize)) // ADD RSP, 40
	amd64POP(cb, amd64R15)
	amd64POP(cb, amd64R14)
	amd64POP(cb, amd64R13)
	amd64POP(cb, amd64R12)
	amd64POP(cb, amd64RBP)
	amd64POP(cb, amd64RBX)
	amd64RET(cb)
}

// x86CmpReg64Mem emits CMP reg64, qword [base + disp]. 64-bit form
// (REX.W=1 + opcode 0x3B). Used by the RTS cache probe to compare a
// regMap (packed uint64) against a context slot.
func x86CmpReg64Mem(cb *CodeBuffer, reg, base byte, disp int32) {
	emitMemOp(cb, true, 0x3B, reg, base, disp)
}

// x86EmitRET emits a native RET (0xC3) terminator with a 2-entry RTS
// inline cache. On hit, the block transfers directly to the cached
// caller's chainEntry via an indirect JMP; on miss it falls back to the
// unchained-exit path which writes the popped return PC into RetPC and
// returns to the Go dispatcher. The cache is populated by the dispatcher
// (jit_x86_exec.go) on every native block invocation, so any caller
// block that has previously been Go-dispatched is reachable on the next
// RET to its return address.
//
// Intended for slice-3 cross-block CALL/RET chaining: a caller block
// ending in CALL chains forward to the callee's chainEntry; the callee's
// RET probes the cache for the caller's continuation block (whose PC is
// the byte after the CALL) and JMPs to it on hit. Without the cache, RET
// always falls through to the dispatcher and pays the Go round-trip per
// return.
func x86EmitRET(cb *CodeBuffer, instrCount uint32) {
	// Pop return address: R11d = [memBase + ESP]; ESP += 4.
	espHost, _ := x86GuestRegToHost(4)
	x86MarkDirty(4)
	amd64MOV_reg_reg32(cb, amd64R10, espHost)
	emitREX_SIB(cb, false, amd64R11, amd64R10, x86AMD64RegMemBase)
	cb.EmitBytes(0x8B, modRM(0, amd64R11, 4), sibByte(0, amd64R10, x86AMD64RegMemBase))
	amd64ALU_reg_imm32_32bit(cb, 0, espHost, 4) // ADD ESP, 4

	// 2-entry MRU cache probe by guest PC only. The RTSCacheNRegMap
	// fields remain in JITContext (still written by the exec loop's
	// promotion path) but are not consulted at probe time — Tier-2
	// per-block regalloc is currently disabled, so all blocks share
	// the default regMap and the gate would always trivially pass at
	// the cost of ~2 CMP+Jcc per probe + a 10-byte 64-bit imm load.
	// When Tier-2 single-block recompile is re-enabled, the regMap
	// gate must come back in lockstep.

	// Slot 0: cmp PC; jne miss0; load addr.
	amd64ALU_reg_mem32_cmp(cb, amd64R11, x86AMD64RegCtx, int32(x86CtxOffRTSCache0PC))
	miss0PCOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, x86AMD64RegCtx, int32(x86CtxOffRTSCache0Addr))
	hit0Off := amd64JMP_rel32(cb)

	// Slot 1.
	patchRel32(cb, miss0PCOff, cb.Len())
	amd64ALU_reg_mem32_cmp(cb, amd64R11, x86AMD64RegCtx, int32(x86CtxOffRTSCache1PC))
	miss1PCOff := amd64Jcc_rel32(cb, amd64CondNE)
	amd64MOV_reg_mem(cb, amd64R10, x86AMD64RegCtx, int32(x86CtxOffRTSCache1Addr))

	// Empty-slot guard. The RTS cache is zero-initialized; an unused
	// slot has both PC=0 and Addr=0. If the guest legitimately RETs to
	// PC=0 (rare but legal: e.g. a program that JSRs from address 0
	// before initializing its stack, or one that deliberately pushes 0
	// as a sentinel return), the PC compare above matches the empty
	// slot, R10 loads the empty Addr=0, and the JMP R10 below would
	// transfer to host address 0 → SIGSEGV. Detect the empty-slot hit
	// by testing R10 nonzero; on zero, fall through to the miss path.
	// A real promoted entry's chainEntry pointer is the JIT code-cache
	// page address, which is never 0.
	patchRel32(cb, hit0Off, cb.Len())
	amd64TEST_reg_reg(cb, amd64R10, amd64R10)
	emptySlotMissOff := amd64Jcc_rel32(cb, amd64CondE)

	// Hit path: R10 = caller's chainEntry. ChainCount += instrCount
	// (counts RET itself plus prior body of this block). Then verify
	// chain-budget / NeedInval before committing the indirect JMP; bail
	// out to the common-exit path if either fails — RetCount still
	// reflects the bumped ChainCount so the dispatcher accounts for the
	// instrs retired up to and including the bailing RET.
	amd64MOV_reg_mem32(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrCount))
	amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffChainCount), amd64RAX)

	amd64DEC_mem32(cb, x86AMD64RegCtx, int32(x86CtxOffChainBudget))
	budgetOff := amd64Jcc_rel32(cb, amd64CondLE)

	amd64ALU_mem_imm8(cb, 7, x86AMD64RegCtx, int32(x86CtxOffNeedInval), 0)
	invalOff := amd64Jcc_rel32(cb, amd64CondNE)

	// Lightweight reg store + indirect JMP to caller's chainEntry.
	// Stash R10 across lightweight-epilogue clobber on RAX.
	amd64MOV_mem_reg(cb, amd64RSP, int32(x86AMD64FrameSize-8), amd64R10)
	x86EmitLightweightEpilogue(cb)
	amd64MOV_reg_mem(cb, amd64R10, amd64RSP, int32(x86AMD64FrameSize-8))

	// Restore guest condition bits into host RFLAGS before chaining to
	// the caller's continuation. x86 CALL/RET preserve EFLAGS
	// architecturally, so a caller continuation with a flag consumer
	// (e.g. JZ following a callee that set flags) must observe the
	// callee's last guest flag state. The chain-hit bookkeeping above
	// (ChainCount accumulation, DEC ChainBudget, CMP NeedInval) and
	// the lightweight-epilogue MOV-into-RAX clobbered host EFLAGS.
	//
	// Only the visible condition subset (CF/PF/AF/ZF/SF/OF =
	// x86VisibleFlagsMask) is restored from the captured slot;
	// non-visible / system bits (DF, TF, IOPL, IF, etc.) are taken
	// from the host's current RFLAGS and re-applied unchanged.
	// Without that mask, a guest STD/POPF that set DF=1 in the saved
	// slot would flip the host's direction flag during the JMP, which
	// violates the Go runtime / host ABI and can break MOVS/STOS/etc.
	// in subsequent native code. Same concern for TF (single-step) —
	// a guest TF=1 would trap the host on the next instruction.
	//
	// Sequence:
	//   PUSHFQ; POP RAX        ; RAX = host RFLAGS
	//   AND EAX, ~visible      ; clear visible bits in host
	//   MOV ECX, [slot]        ; ECX = saved guest flags
	//   AND ECX, visible       ; isolate guest visible bits
	//   OR EAX, ECX            ; merge: host system + guest visible
	//   PUSH RAX; POPFQ        ; install
	cb.EmitBytes(0x9C) // PUSHFQ
	amd64POP(cb, amd64RAX)
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RAX, x86InvVisibleFlagsMaskI32) // AND EAX, ~mask
	amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, int32(x86AMD64OffSavedEFlags))
	amd64ALU_reg_imm32_32bit(cb, 4, amd64RCX, int32(x86VisibleFlagsMask)) // AND ECX, mask
	amd64ALU_reg_reg32(cb, 0x09, amd64RAX, amd64RCX)                      // OR EAX, ECX
	amd64PUSH(cb, amd64RAX)
	cb.EmitBytes(0x9D) // POPFQ

	emitREX(cb, false, 0, amd64R10)
	cb.EmitBytes(0xFF, 0xE0|byte(amd64R10&7)) // JMP R10

	// Miss path: ChainCount += instrCount (mirroring the hit-path
	// increment so the dispatcher sees the same retired-instruction
	// total whether or not the RET chained natively). PC misses (slot
	// 0's was redirected through slot 1's probe; slot 1 is the final
	// miss) and the empty-slot guard's JZ both converge here.
	patchRel32(cb, miss1PCOff, cb.Len())
	patchRel32(cb, emptySlotMissOff, cb.Len())
	amd64MOV_reg_mem32(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrCount))
	amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffChainCount), amd64RAX)

	// Common exit (miss + budget/inval bail). RetPC = popped retAddr,
	// RetCount = ChainCount. Full epilogue merges Flags to guest, stores
	// dirty regs, pops frame, RET.
	patchRel32(cb, budgetOff, cb.Len())
	patchRel32(cb, invalOff, cb.Len())
	amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffRetPC), amd64R11)
	amd64MOV_reg_mem32(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffChainCount))
	amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffRetCount), amd64RAX)
	x86EmitLightweightEpilogue(cb)
	x86EmitFullEpilogueEnd(cb)
}

// x86EmitChainExit emits a chain exit sequence for a block terminator.
//  1. Lightweight epilogue (store mapped registers)
//  2. Accumulate instruction count into ChainCount
//  3. Decrement ChainBudget; if exhausted -> unchained exit
//  4. Check NeedInval; if set -> unchained exit
//  5. Patchable JMP rel32 (initially to unchained exit)
//  6. Unchained exit: set RetPC/RetCount, full pop/ret
func x86EmitChainExit(cb *CodeBuffer, targetPC uint32, instrCount uint32) x86ChainExitInfo {
	// Store this block's dirty mapped guest registers back to jitRegs
	// before chaining forward. Chained blocks share the host stack frame
	// but each block tracks its own dirtyMask — regs modified in the
	// source block won't appear in the target's dirtyMask, so the
	// target's eventual unchained exit would skip them. Flushing here
	// makes cross-block register writes durable. Host regs themselves
	// keep the same values, so the chained target still reads them
	// directly without reloading.
	x86EmitLightweightEpilogue(cb)

	// Accumulate instruction count: ChainCount += instrCount
	amd64MOV_reg_mem32(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffChainCount))
	amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrCount)) // ADD EAX, instrCount
	amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffChainCount), amd64RAX)

	// DEC DWORD [R15 + ChainBudget]
	amd64DEC_mem32(cb, x86AMD64RegCtx, int32(x86CtxOffChainBudget))

	// JLE .unchained (budget exhausted)
	unchainedOff1 := amd64Jcc_rel32(cb, amd64CondLE)

	// CMP DWORD [R15 + NeedInval], 0
	amd64ALU_mem_imm8(cb, 7, x86AMD64RegCtx, int32(x86CtxOffNeedInval), 0) // CMP
	// JNE .unchained (self-mod detected)
	unchainedOff2 := amd64Jcc_rel32(cb, amd64CondNE)

	// Patchable JMP rel32 -- initially jumps to .unchained
	jmpOff := cb.Len()
	cb.EmitBytes(0xE9, 0, 0, 0, 0) // JMP rel32 placeholder
	jmpDispOffset := jmpOff + 1

	// .unchained label
	unchainedLabel := cb.Len()
	patchRel32(cb, unchainedOff1, unchainedLabel)
	patchRel32(cb, unchainedOff2, unchainedLabel)
	patchRel32(cb, jmpDispOffset, unchainedLabel)

	// Set RetPC = targetPC
	amd64MOV_mem_imm32(cb, x86AMD64RegCtx, int32(x86CtxOffRetPC), targetPC)
	// RetCount = ChainCount (already accumulated)
	amd64MOV_reg_mem32(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffChainCount))
	amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffRetCount), amd64RAX)

	// Unchained exit: must store mapped registers back before returning to Go
	x86EmitLightweightEpilogue(cb)
	x86EmitFullEpilogueEnd(cb)

	return x86ChainExitInfo{
		targetPC:      targetPC,
		jmpDispOffset: jmpDispOffset,
	}
}

// ===========================================================================
// Block Compiler
// ===========================================================================

// x86CompileBlock compiles a slice of pre-decoded x86 instructions into native code.
// Single-block Tier-2 promotion is retired; region-only Tier-2 (see
// x86CompileRegion) is the sole per-block-regalloc promotion path.
func x86CompileBlock(instrs []X86JITInstr, startPC uint32, execMem *ExecMem, memory []byte) (*JITBlock, error) {
	if len(instrs) == 0 {
		return nil, fmt.Errorf("empty instruction list")
	}

	cb := &CodeBuffer{}
	br := x86AnalyzeBlockRegs(instrs, memory, startPC)
	cs := &x86CompileState{flagState: x86FlagsDead, tier: 0, dirtyMask: br.written}

	cs.ioBitmap = x86CompileIOBitmap
	cs.codeBitmap = x86CompileCodeBitmap
	cs.host = x86Host

	cs.regMap = x86DefaultRegMap()

	// Run peephole optimizer for flag analysis (all tiers benefit)
	cs.flagsNeeded = x86PeepholeFlags(instrs)

	// The peephole walk is linear and unsafe for blocks with internal
	// control-flow splits: a forward Jcc that branches over a flag
	// producer means the not-taken path retires the producer (whose
	// flags may be observed downstream), but the taken path skips it
	// and observes flags from BEFORE the Jcc — peephole can't represent
	// that and may flag a producer dead when one path actually needs it.
	// Detect any non-self-loop Jcc and disable the per-instruction flag
	// capture skip in that case (force every producer to capture). The
	// startPC self-loop case is safe because the Jcc target is the loop
	// head and the loop body is straight-line.
	cs.hasNonSelfLoopJcc = false
	for i := range instrs {
		ji := &instrs[i]
		// 1-byte Jcc rel8 (0x70-0x7F).
		if ji.opcode >= 0x70 && ji.opcode <= 0x7F && ji.length >= 2 {
			immPC := ji.opcodePC + uint32(ji.length) - 1
			if immPC < uint32(len(memory)) {
				rel := int32(int8(memory[immPC]))
				targetPC := uint32(int32(ji.opcodePC+uint32(ji.length)) + rel)
				if targetPC != startPC {
					cs.hasNonSelfLoopJcc = true
					break
				}
			}
			continue
		}
		// 2-byte Jcc rel32 (0x0F 0x80-0x8F).
		if ji.opcode >= 0x0F80 && ji.opcode <= 0x0F8F && ji.length >= 6 {
			immPC := ji.opcodePC + uint32(ji.length) - 4
			if immPC+4 <= uint32(len(memory)) {
				rel := int32(uint32(memory[immPC]) |
					uint32(memory[immPC+1])<<8 |
					uint32(memory[immPC+2])<<16 |
					uint32(memory[immPC+3])<<24)
				targetPC := uint32(int32(ji.opcodePC+uint32(ji.length)) + rel)
				if targetPC != startPC {
					cs.hasNonSelfLoopJcc = true
					break
				}
			}
			continue
		}
		// LOOP/LOOPE/LOOPNE/JECXZ — also intra-block branches.
		if ji.opcode == 0xE0 || ji.opcode == 0xE1 || ji.opcode == 0xE2 || ji.opcode == 0xE3 {
			cs.hasNonSelfLoopJcc = true
			break
		}
	}

	// Force the last flag-emitting instruction in the block to keep its
	// capture live, because block exit must leave guest cpu.Flags coherent
	// for downstream chained blocks / IRQ handlers / Go observers. The
	// peephole's backward demand walk assumes no exit consumer and would
	// otherwise mark the final producer dead. Uses x86InstrFlagOpKind so
	// the predicate matches the per-instr capture decision exactly
	// (POPF/SAHF/CLC/STC/CLD/STD/Grp4-5 INC-DEC included).
	for i := len(instrs) - 1; i >= 0; i-- {
		if x86InstrFlagOpKind(instrs[i].opcode, instrs[i].modrm) != x86FlagOpNone {
			if i < len(cs.flagsNeeded) {
				cs.flagsNeeded[i] = true
			}
			break
		}
	}

	// Compute flagShadowed[i]: is instruction i's flag-capture fully
	// redundant because a downstream same-block instruction overwrites
	// every bit i contributed to the captured-EFLAGS slot, with no
	// in-between flag READ?
	//
	// Per-kind rules (writes to slot):
	//   - x86FlagOpArith / x86FlagOpManip: writes ALL 6 visible bits
	//     (CF, PF, AF, ZF, SF, OF). Shadowable only by another arith /
	//     manip producer (logic preserves AF, would blend in slot's
	//     stale value).
	//   - x86FlagOpLogic: writes 5 bits (CF, PF, ZF, SF, OF) and READS
	//     AF from the slot (preserving guest AF across logic ops).
	//     Shadowable by arith/manip (overwrites all) or logic
	//     (overwrites the 5 bits, AF passes through unchanged).
	//
	// A flag-READ between position i and any downstream producer breaks
	// the shadow chain.
	cs.flagShadowed = make([]bool, len(instrs))
	const (
		shadowNone  = 0 // no downstream overwriter / chain broken by reader
		shadowArith = 1 // downstream arith/manip — overwrites all 6 visible bits
		shadowLogic = 2 // downstream logic — overwrites 5 bits (preserves AF)
	)
	downstream := shadowNone
	for i := len(instrs) - 1; i >= 0; i-- {
		ji := &instrs[i]
		op := byte(ji.opcode)
		readsFlags := false
		if ji.opcode < 0x0F00 {
			switch {
			case op >= 0x70 && op <= 0x7F:
				readsFlags = true
			case op == 0x9C, op == 0x9F:
				readsFlags = true
			case op == 0x10, op == 0x11, op == 0x12, op == 0x13,
				op == 0x14, op == 0x15:
				readsFlags = true
			case op == 0x18, op == 0x19, op == 0x1A, op == 0x1B,
				op == 0x1C, op == 0x1D:
				readsFlags = true
			case op == 0xD0, op == 0xD1, op == 0xD2, op == 0xD3:
				if ji.hasModRM {
					sub := (ji.modrm >> 3) & 7
					if sub == 2 || sub == 3 {
						readsFlags = true
					}
				}
			}
		} else {
			if ji.opcode >= 0x0F80 && ji.opcode <= 0x0F8F {
				readsFlags = true
			}
		}
		if readsFlags {
			downstream = shadowNone
			continue
		}
		kind := x86InstrFlagOpKind(ji.opcode, ji.modrm)
		if kind == x86FlagOpNone {
			continue
		}
		// Decide shadowability based on producer kind and downstream.
		switch kind {
		case x86FlagOpArith, x86FlagOpManip:
			// Arith needs a downstream arith/manip overwrite; logic
			// preserves AF and is not sufficient.
			if downstream == shadowArith {
				cs.flagShadowed[i] = true
			}
			downstream = shadowArith
		case x86FlagOpLogic:
			// Logic shadowable by either kind (both overwrite the 5
			// bits logic writes; AF is preserved through both anyway).
			if downstream == shadowArith || downstream == shadowLogic {
				cs.flagShadowed[i] = true
			}
			// Logic preserves AF, so for upstream-arith purposes the
			// arith's AF is still needed downstream; treat the running
			// downstream tag as logic (does NOT overwrite AF).
			if downstream != shadowArith {
				downstream = shadowLogic
			}
		}
	}

	// Detect self-loops: backward Jcc targeting startPC
	for i := range instrs {
		ji := &instrs[i]
		op := byte(ji.opcode)
		if op >= 0x70 && op <= 0x7F && ji.length >= 2 {
			immPC := ji.opcodePC + uint32(ji.length) - 1
			if immPC < uint32(len(memory)) {
				rel := int32(int8(memory[immPC]))
				nextPC := ji.opcodePC + uint32(ji.length)
				targetPC := uint32(int32(nextPC) + rel)
				if targetPC == startPC {
					cs.isLoop = true
					cs.instrPerIter = len(instrs) // approximate
					break
				}
			}
		}
	}

	// Set the current compile state for instruction emitters to use
	x86CurrentCS = cs
	defer func() { x86CurrentCS = nil }()

	// Emit prologue
	x86EmitPrologue(cb, cs)

	// Emit chain entry point (lightweight entry for chained transitions)
	chainEntryOff := x86EmitChainEntry(cb, cs)

	// Record loop start label (after prologue + chain entry, before first instruction)
	if cs.isLoop {
		cs.loopStartLabel = cb.Len()
	}

	// Set up deferred bail collection
	var deferredBails []x86DeferredBail
	x86CurrentBails = &deferredBails
	defer func() { x86CurrentBails = nil }()

	// Emit instructions
	var chainExits []x86ChainExitInfo
	cs.chainExits = &chainExits
	instrCount := 0
	lastPC := startPC
	for i := range instrs {
		ji := &instrs[i]

		// Segment-override prefix (FS/GS/DS/ES/CS/SS) bails to interp.
		// The current native emitters compute effective addresses as
		// flat offsets and do not add a segment base; running a
		// prefixed instr through them would produce different memory
		// addresses than the interpreter (which honors prefixSeg via
		// readSegBase). End the block here so the dispatcher steps the
		// prefixed instr via cpu.Step(), which correctly applies the
		// segment base. Slice 3+ may add native segment-base support
		// for FS:/GS: TLS-style access.
		if ji.prefixes&x86PrefSeg != 0 {
			break
		}

		// RET (0xC3): native emit with 2-entry RTS inline cache. Closes
		// the cross-block CALL/RET chain — caller's CALL chains forward
		// to the callee, callee's RET hits the cache on caller's
		// continuation block on the next return.
		if byte(ji.opcode) == 0xC3 && ji.opcode < 0x100 {
			instrCount++
			lastPC = ji.opcodePC + uint32(ji.length)
			x86EmitRET(cb, uint32(instrCount))
			goto done
		}

		// Check if this is a block terminator that can use a chain exit
		if x86IsBlockTerminator(ji.opcode) && ji.opcode != 0x00F4 { // Not HLT
			// For CALL rel32 (0xE8) and JMP rel32 (0xE9) / JMP rel8 (0xEB),
			// compute the target PC and emit a chain exit
			targetPC, hasTarget := x86ResolveTerminatorTarget(ji, memory, startPC)
			if hasTarget {
				instrCount++
				lastPC = ji.opcodePC + uint32(ji.length)

				// For CALL, we need to push the return address first
				if byte(ji.opcode) == 0xE8 {
					x86MarkDirty(4) // ESP modified by CALL
					retAddr := ji.opcodePC + uint32(ji.length)
					// ESP -= 4 (use dynamic mapping)
					espHost, _ := x86GuestRegToHost(4)
					amd64ALU_reg_imm32_32bit(cb, 5, espHost, 4)
					// Write return address to [memory + ESP]
					amd64MOV_reg_reg32(cb, amd64R8, espHost)
					// Mask address
					// MOV DWORD [RSI + R8], retAddr
					emitMemOpSIB(cb, false, 0xC7, 0, x86AMD64RegMemBase, amd64R8, 0)
					cb.Emit32(retAddr)
				}

				info := x86EmitChainExit(cb, targetPC, uint32(instrCount))
				chainExits = append(chainExits, info)
				goto done
			}
		}

		if !x86EmitInstruction(cb, ji, memory, startPC, cs, instrCount) {
			break
		}
		// Eager-capture the host EFLAGS state immediately after any
		// flag-modifying guest instruction. The capture stack slot is
		// merged into *cpu.Flags at every block-exit point, so the
		// guest sees correct flag state even if subsequent JIT scratch
		// arithmetic or the epilogue itself clobbers host EFLAGS.
		// Logic / shift / rotate ops use a separate capture path that
		// preserves the guest's prior AF (Intel-undefined for those
		// kinds; the IE interpreter leaves AF untouched).
		// Skip the capture when the peephole has determined this
		// instruction's flag output is dead (overwritten by a later
		// in-block flag producer with no intervening consumer), or
		// when the emitter already captured pre-bookkeeping (e.g.
		// memory-dest ALU emitting capture before its post-store SMC
		// check clobbers EFLAGS).
		flagsLive := cs.hasNonSelfLoopJcc ||
			i >= len(cs.flagsNeeded) || cs.flagsNeeded[i] ||
			i >= len(cs.flagShadowed) || !cs.flagShadowed[i]
		if flagsLive && !cs.flagCaptureDone {
			switch x86InstrFlagOpKind(ji.opcode, ji.modrm) {
			case x86FlagOpArith, x86FlagOpManip:
				x86EmitCaptureFlagsArith(cb)
			case x86FlagOpLogic:
				x86EmitCaptureFlagsLogic(cb)
			}
		}
		cs.flagCaptureDone = false
		instrCount++
		lastPC = ji.opcodePC + uint32(ji.length)
	}

	if instrCount == 0 {
		return nil, fmt.Errorf("no instructions compiled")
	}

	// Non-terminator exit: emit RetPC/RetCount + full epilogue. The
	// dispatcher accounting reads RetCount when nonzero and ignores
	// ChainCount in that case, so any predecessor chain accumulation
	// must be folded in here — without the add, a chained sequence
	// that exits through this fallthrough path would drop every
	// predecessor's retired-instruction count.
	if cs.isLoop {
		// For self-loops, RetCount = loopRetired + final iteration + ChainCount.
		x86EmitRetPC(cb, lastPC, 0) // placeholder
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(x86AMD64OffLoopRetired))
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(instrCount))
		amd64MOV_reg_mem32(cb, amd64RDX, x86AMD64RegCtx, int32(x86CtxOffChainCount))
		amd64ALU_reg_reg32(cb, 0x01, amd64RAX, amd64RDX) // ADD EAX, EDX
		amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffRetCount), amd64RAX)
	} else {
		x86EmitRetPC(cb, lastPC, uint32(instrCount))
		// Fold ChainCount into RetCount so chained predecessors are
		// counted alongside this block's local instrCount.
		amd64MOV_reg_mem32(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffChainCount))
		amd64MOV_reg_mem32(cb, amd64RDX, x86AMD64RegCtx, int32(x86CtxOffRetCount))
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64RAX) // ADD EDX, EAX
		amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffRetCount), amd64RDX)
	}
	x86EmitEpilogue(cb, cs)

done:
	// Emit shared deferred bail stubs (IO check failures, self-mod detection)
	x86EmitDeferredBails(cb)
	// Resolve labels
	cb.Resolve()

	// Copy to executable memory
	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, fmt.Errorf("execMem.Write: %w", err)
	}

	// Convert code buffer offsets to absolute ExecMem addresses
	chainEntry := addr + uintptr(chainEntryOff)
	var slots []chainSlot
	for _, ce := range chainExits {
		slots = append(slots, chainSlot{
			targetPC:  ce.targetPC,
			patchAddr: addr + uintptr(ce.jmpDispOffset),
		})
	}

	return &JITBlock{
		startPC:    startPC,
		endPC:      lastPC,
		instrCount: instrCount,
		execAddr:   addr,
		execSize:   len(code),
		chainEntry: chainEntry,
		chainSlots: slots,
		regMap:     cs.regMap,
	}, nil
}

// ===========================================================================
// Multi-Block Region Compiler
// ===========================================================================

// x86CompileRegion compiles a multi-block region as a single native unit.
// Single prologue, internal blocks connected by native jumps, single epilogue.
func x86CompileRegion(region *x86Region, execMem *ExecMem, memory []byte) (*JITBlock, error) {
	if region == nil || len(region.blocks) < 2 {
		return nil, fmt.Errorf("region too small")
	}

	// Compute region-wide register analysis
	var allInstrs []X86JITInstr
	for _, block := range region.blocks {
		allInstrs = append(allInstrs, block...)
	}
	br := x86AnalyzeBlockRegs(allInstrs, memory, region.entryPC)

	cb := &CodeBuffer{}
	cs := &x86CompileState{
		flagState: x86FlagsDead,
		tier:      2,
		dirtyMask: br.written,
		regMap:    x86Tier2RegAlloc(allInstrs, memory, region.entryPC),
	}
	cs.flagsNeeded = x86PeepholeFlags(allInstrs)
	cs.ioBitmap = x86CompileIOBitmap
	cs.codeBitmap = x86CompileCodeBitmap
	cs.host = x86Host

	x86CurrentCS = cs
	defer func() { x86CurrentCS = nil }()

	var deferredBails []x86DeferredBail
	x86CurrentBails = &deferredBails
	defer func() { x86CurrentBails = nil }()

	// Emit prologue
	x86EmitPrologue(cb, cs)
	chainEntryOff := x86EmitChainEntry(cb, cs)

	// Initialize loop counters (for back-edge loops within the region)
	hasBackEdge := len(region.backEdges) > 0
	if hasBackEdge {
		amd64MOV_mem_imm32(cb, amd64RSP, int32(x86AMD64OffLoopBudget), x86LoopBudget)
		amd64MOV_mem_imm32(cb, amd64RSP, int32(x86AMD64OffLoopRetired), 0)
	}

	// Record code buffer offsets for each block's start (for internal jumps)
	blockLabels := make([]int, len(region.blocks))
	totalInstrCount := 0
	instrCountAtBlock := make([]int, len(region.blocks))

	// Forward-jump fixups: patches to apply after all blocks are emitted
	type fwdFixup struct {
		jmpDispOff  int // offset of JMP rel32 displacement in CodeBuffer
		targetBlock int // target block index
	}
	var fwdFixups []fwdFixup

	// Emit all blocks
	for bi, block := range region.blocks {
		blockLabels[bi] = cb.Len()
		instrCountAtBlock[bi] = totalInstrCount

		for ii := range block {
			ji := &block[ii]
			if !x86EmitInstruction(cb, ji, memory, region.blockPCs[bi], cs, totalInstrCount) {
				break
			}
			totalInstrCount++
		}

		// Handle block terminator
		if len(block) > 0 {
			last := &block[len(block)-1]
			if x86IsBlockTerminator(last.opcode) {
				targetPC, hasTarget := x86ResolveTerminatorTarget(last, memory, region.blockPCs[bi])
				if hasTarget {
					// Check if target is within the region
					targetBlockIdx := -1
					for ti, bpc := range region.blockPCs {
						if bpc == targetPC {
							targetBlockIdx = ti
							break
						}
					}
					if targetBlockIdx >= 0 {
						// Internal jump: emit native JMP to target block label
						if _, isBackEdge := region.backEdges[bi]; isBackEdge {
							// Back-edge: budget check + native loop
							// Accumulate retired instructions
							amd64MOV_reg_mem32(cb, amd64RCX, amd64RSP, int32(x86AMD64OffLoopRetired))
							emitREX(cb, false, amd64RCX, amd64RCX)
							cb.EmitBytes(0x8D, modRM(2, amd64RCX, amd64RCX))
							cb.Emit32(uint32(totalInstrCount - instrCountAtBlock[bi]))
							amd64MOV_mem_reg32(cb, amd64RSP, int32(x86AMD64OffLoopRetired), amd64RCX)

							// Budget check
							amd64MOV_reg_mem32(cb, amd64R11, amd64RSP, int32(x86AMD64OffLoopBudget))
							emitREX(cb, false, amd64R11, amd64R11)
							cb.EmitBytes(0x8D, modRM(1, amd64R11, amd64R11), 0xFF) // LEA R11, [R11-1]
							amd64MOV_mem_reg32(cb, amd64RSP, int32(x86AMD64OffLoopBudget), amd64R11)

							emitREX(cb, false, amd64R11, amd64R11)
							cb.EmitBytes(0x85, modRM(3, amd64R11, amd64R11)) // TEST R11, R11
							budgetExhOff := amd64Jcc_rel32(cb, amd64CondLE)

							// Jump back to target block
							backJmp := amd64JMP_rel32(cb)
							patchRel32(cb, backJmp, blockLabels[targetBlockIdx])

							// Budget exhausted: exit
							budgetExhLabel := cb.Len()
							patchRel32(cb, budgetExhOff, budgetExhLabel)
							x86EmitRetPC(cb, targetPC, 0)
							amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(x86AMD64OffLoopRetired))
							amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffRetCount), amd64RAX)
							x86EmitLightweightEpilogue(cb)
							x86EmitFullEpilogueEnd(cb)
						} else {
							// Forward jump: record for patching after all blocks emitted
							fwdJmp := amd64JMP_rel32(cb)
							fwdFixups = append(fwdFixups, fwdFixup{jmpDispOff: fwdJmp, targetBlock: targetBlockIdx})
						}
						continue
					}
				}
			}
		}
	}

	// Patch forward jumps now that all block labels are known
	for _, fix := range fwdFixups {
		patchRel32(cb, fix.jmpDispOff, blockLabels[fix.targetBlock])
	}

	// Default exit (fall-through from last block). Same ChainCount
	// fold as the per-block compiler: if this region was reached via a
	// chained predecessor, RetCount must include that prior count or
	// the dispatcher will undercount retired instructions.
	if hasBackEdge {
		x86EmitRetPC(cb, region.blockPCs[len(region.blocks)-1], 0)
		amd64MOV_reg_mem32(cb, amd64RAX, amd64RSP, int32(x86AMD64OffLoopRetired))
		amd64ALU_reg_imm32_32bit(cb, 0, amd64RAX, int32(totalInstrCount))
		amd64MOV_reg_mem32(cb, amd64RDX, x86AMD64RegCtx, int32(x86CtxOffChainCount))
		amd64ALU_reg_reg32(cb, 0x01, amd64RAX, amd64RDX) // ADD EAX, EDX
		amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffRetCount), amd64RAX)
	} else {
		lastBlock := region.blocks[len(region.blocks)-1]
		lastInstr := &lastBlock[len(lastBlock)-1]
		lastPC := lastInstr.opcodePC + uint32(lastInstr.length)
		x86EmitRetPC(cb, lastPC, uint32(totalInstrCount))
		amd64MOV_reg_mem32(cb, amd64RAX, x86AMD64RegCtx, int32(x86CtxOffChainCount))
		amd64MOV_reg_mem32(cb, amd64RDX, x86AMD64RegCtx, int32(x86CtxOffRetCount))
		amd64ALU_reg_reg32(cb, 0x01, amd64RDX, amd64RAX) // ADD EDX, EAX
		amd64MOV_mem_reg32(cb, x86AMD64RegCtx, int32(x86CtxOffRetCount), amd64RDX)
	}
	x86EmitEpilogue(cb, cs)

	// Emit deferred bails
	x86EmitDeferredBails(cb)
	cb.Resolve()

	code := cb.Bytes()
	addr, err := execMem.Write(code)
	if err != nil {
		return nil, fmt.Errorf("execMem.Write: %w", err)
	}

	lastBlock := region.blocks[len(region.blocks)-1]
	lastInstr := &lastBlock[len(lastBlock)-1]
	endPC := lastInstr.opcodePC + uint32(lastInstr.length)

	return &JITBlock{
		startPC:    region.entryPC,
		endPC:      endPC,
		instrCount: totalInstrCount,
		execAddr:   addr,
		execSize:   len(code),
		chainEntry: addr + uintptr(chainEntryOff),
		regMap:     cs.regMap,
		tier:       2,
	}, nil
}

// x86ResolveTerminatorTarget computes the target PC for a block-terminating
// instruction, if it has a statically known target.
func x86ResolveTerminatorTarget(ji *X86JITInstr, memory []byte, startPC uint32) (uint32, bool) {
	op := byte(ji.opcode)
	nextPC := ji.opcodePC + uint32(ji.length)

	switch op {
	case 0xE8: // CALL rel32
		immPC := ji.opcodePC + uint32(ji.length) - 4
		rel := int32(memory[immPC]) | int32(memory[immPC+1])<<8 | int32(memory[immPC+2])<<16 | int32(memory[immPC+3])<<24
		return uint32(int32(nextPC) + rel), true

	case 0xE9: // JMP rel32
		immPC := ji.opcodePC + uint32(ji.length) - 4
		rel := int32(memory[immPC]) | int32(memory[immPC+1])<<8 | int32(memory[immPC+2])<<16 | int32(memory[immPC+3])<<24
		return uint32(int32(nextPC) + rel), true

	case 0xEB: // JMP rel8
		immPC := ji.opcodePC + uint32(ji.length) - 1
		rel := int32(int8(memory[immPC]))
		return uint32(int32(nextPC) + rel), true

	case 0xC3: // RET -- target depends on stack, not statically known
		return 0, false

	case 0xC2: // RET imm16 -- not statically known
		return 0, false
	}

	return 0, false
}
