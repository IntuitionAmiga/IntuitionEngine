// jit_m68k_emit_wasm.go - WebAssembly bytecode emitter for the M68020 JIT
// (parity plan milestone 5, the minimal wasm backend).
//
// Runtime-boundary decision (recorded here per the milestone 5 prerequisite):
// this is an M68020-SPECIFIC wasm backend. It reuses only the generic wasm
// binary encoder (jit_wasm_encoder.go) and the architecture-neutral analysis
// in jit_m68k_common.go (instruction scanning, length, CCR liveness, register
// analysis). It deliberately does NOT reuse jit_wasm_runtime.go, jit_exec_wasm.go
// or the IE64 chain driver: those are tied to CPU64, JITContext, 64-bit PCs,
// IE64 helper handling, IE64 timers, MMU gating and IE64 accounting. Bending
// them into a multi-ISA abstraction is exactly the hazard the plan warns
// against, so the M68020 gets its own dispatcher (jit_m68k_dispatch_wasm.go)
// that mirrors the arm64 dispatcher's contract against the shared
// M68KJITContext layout.
//
// The file is pure Go and untagged on purpose: the translator is exercised
// natively under wazero in the test suite (mirroring the IE64 wasm backend),
// while the browser runtime feeds the same module bytes to
// WebAssembly.instantiate.
//
// Model:
//   - Each compiled block is one wasm function `block(ctx i32) -> ()`, importing
//     the host's `env.mem` linear memory. The context image lives in linear
//     memory at the ctx offset; its pointer fields (DataRegsPtr, AddrRegsPtr,
//     MemPtr, SRPtr, ...) hold LINEAR-MEMORY byte offsets, not host pointers.
//   - The M68020 data and address register files are eight uint32 each, read
//     and written directly in linear memory.
//   - wasm has no flags register, so the CCR is modelled in a wasm local in
//     M68K bit order (X N Z V C at bits 4..0, matching the arm64 W4 model) and
//     materialised into cpu.SR's low byte in every block epilogue.
//   - Correctness-first: only register-direct and immediate integer operands
//     are lowered in this slice. Anything else rejects, and the dispatcher
//     interprets it. Full CCR is materialised on every flag-setting
//     instruction; there is no liveness elision yet.
//
// Parity is with the interpreter, not the M68000PRM: AND/OR/EOR preserve V and
// C (SetFlagsNZ); MOVE/TST/CLR clear V and C and preserve X; ADD/SUB/CMP set
// the full NZVC(X) set with the interpreter's exact overflow/carry formulas
// (reproduced here via a top-of-word alignment so a native-style sign/carry
// read matches the sized operation bit for bit).

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import "os"

// CCR bit positions inside the modelled CCR local (M68K SR low-byte order).
const (
	wm68kBitC = 0
	wm68kBitV = 1
	wm68kBitZ = 2
	wm68kBitN = 3
	wm68kBitX = 4
)

// wasm local indices. Index 0 is the ctx parameter; the rest are declared
// locals. A generous fixed set is allocated up front so later slices (memory,
// branches, FPU) do not renumber the integer core.
const (
	wm68kLCtx   = 0  // param: ctx image base (linear-memory offset)
	wm68kLCCR   = 1  // modelled CCR (i32)
	wm68kLDBase = 2  // &DataRegs[0] (linear offset, i32)
	wm68kLABase = 3  // &AddrRegs[0] (linear offset, i32)
	wm68kLMBase = 4  // &memory[0] (linear offset, i32)
	wm68kLMSize = 5  // len(memory) (i32)
	wm68kLT0    = 6  // scratch (i32)
	wm68kLT1    = 7  // scratch (i32)
	wm68kLT2    = 8  // scratch (i32)
	wm68kLT3    = 9  // scratch (i32)
	wm68kLT4    = 10 // scratch (i32)
	wm68kLEA    = 11 // effective-address scratch (i32)
	wm68kLMV    = 12 // captured memory operand value/scratch, guard-safe (i32)
	wm68kLMB    = 13 // second memory scratch, guard-safe (i32)
	wm68kLQ0    = 14 // scratch (i64)
	wm68kLQ1    = 15 // scratch (i64)
	wm68kLF0    = 16 // scratch (f64)
	wm68kLF1    = 17 // scratch (f64)
	wm68kLF2    = 18 // scratch (f64)
	wm68kLF3    = 19 // scratch (f64)
	// Milestone 6 loop support (declared after the f64 group; wasm local
	// indices are stable because groups are declared in order).
	wm68kLRet    = 20 // retired instructions from completed loop iterations (i32)
	wm68kLBudget = 21 // remaining loop-iteration budget (i32)
)

// wm68kLoopBudget bounds the iterations a structured in-block loop may run
// before the block exits to the dispatcher, so interrupt sampling and the
// cooperative yield keep their latency bound. Mirrors the role of the native
// ChainBudget.
const wm68kLoopBudget = 1024

// wm68kLocalDecl is the extra-local declaration list for addFunc (params are
// declared by the signature). 13 i32, 2 i64, 4 f64, then 2 i32 (loop state).
func wm68kLocalDecl() []byte {
	locals := make([]byte, 0, 21)
	for i := 0; i < 13; i++ {
		locals = append(locals, wasmTypeI32)
	}
	locals = append(locals, wasmTypeI64, wasmTypeI64)
	locals = append(locals, wasmTypeF64, wasmTypeF64, wasmTypeF64, wasmTypeF64)
	locals = append(locals, wasmTypeI32, wasmTypeI32)
	return locals
}

// ---------------------------------------------------------------------------
// Decode
// ---------------------------------------------------------------------------

// wasm-backend instruction classes (integer core).
const (
	wm68kClassNOP = iota
	wm68kClassMOVEQ
	wm68kClassMOVE     // MOVE.size <ea>,<ea> (reg/imm operands only in this slice)
	wm68kClassMOVEA    // MOVEA.size <ea>,An
	wm68kClassALUToD   // ADD/SUB/AND/OR/CMP <ea>,Dn
	wm68kClassALUToD2  // ADD/SUB/AND/OR/EOR Dn,<ea=Dn>
	wm68kClassQuickD   // ADDQ/SUBQ #q,Dn
	wm68kClassQuickA   // ADDQ/SUBQ #q,An (no flags)
	wm68kClassImmALU   // ADDI/SUBI/ANDI/ORI/EORI/CMPI #imm,Dn
	wm68kClassTST      // TST Dn
	wm68kClassCLR      // CLR Dn
	wm68kClassALUToMem // ADD/SUB/AND/OR/EOR Dn,<ea=mem> (read-modify-write)
	wm68kClassQuickM   // ADDQ/SUBQ #q,<ea=mem> (read-modify-write)
	// Block terminators (control flow). Each ends the block and publishes a
	// resume PC to the dispatcher; the wasm block returns after emitting one.
	wm68kClassBRA  // unconditional branch (q = target)
	wm68kClassBcc  // conditional branch (cond, q = target)
	wm68kClassDBcc // decrement and branch (cond, reg = Dn, q = target)
	wm68kClassBSR  // branch to subroutine (q = target)
	wm68kClassJMP  // jump (src EA -> target)
	wm68kClassJSR  // jump to subroutine (src EA -> target)
	wm68kClassRTS  // return from subroutine
	wm68kClassFPU  // 68881 register-to-register clean-mapping op
)

// wm68kIsTerminator reports whether a class ends the block.
func wm68kIsTerminator(class int) bool {
	switch class {
	case wm68kClassBRA, wm68kClassBcc, wm68kClassDBcc, wm68kClassBSR,
		wm68kClassJMP, wm68kClassJSR, wm68kClassRTS:
		return true
	}
	return false
}

// ALU operation selectors (carried in op.alu).
const (
	wm68kALUAdd = iota
	wm68kALUSub
	wm68kALUAnd
	wm68kALUOr
	wm68kALUCmp
	wm68kALUEor
)

// operand kinds this slice accepts.
const (
	wm68kOpDn = iota
	wm68kOpAn
	wm68kOpImm
	wm68kOpMem // memory EA (see memKind)
)

// memory EA sub-kinds (only forms without an index register are lowered here;
// index and 68020 full-format modes reject to the interpreter).
const (
	wm68kMemInd  = iota // (An)
	wm68kMemPost        // (An)+
	wm68kMemPre         // -(An)
	wm68kMemDisp        // (d16,An)
	wm68kMemAbs         // (xxx).W, (xxx).L, (d16,PC): constant address
)

type wm68kOperand struct {
	kind    int
	reg     uint16 // Dn/An number, or base An for memory forms
	imm     uint32 // immediate (sized, zero-extended)
	memKind int    // wm68kMem* when kind == wm68kOpMem
	disp    int32  // (d16,An) displacement
	abs     uint32 // constant address for wm68kMemAbs
	pcRel   bool   // wm68kMemAbs came from (d16,PC): never a legal destination
}

func (o wm68kOperand) isMem() bool { return o.kind == wm68kOpMem }

type wm68kOp struct {
	class int
	size  uint32 // 1/2/4
	alu   int
	sub   bool // SUBQ vs ADDQ
	src   wm68kOperand
	dst   wm68kOperand
	q     uint32 // ADDQ/SUBQ count
	imm   uint32 // MOVEQ / immediate ALU value (already sign/zero handled)

	// Branch fields.
	cond   int    // M68K condition code (Bcc/DBcc)
	target uint32 // static branch target (BRA/Bcc/BSR); JMP/JSR use src EA
	fallPC uint32 // fall-through PC (instrPC + length), for Bcc/DBcc not-taken

	// FPU fields.
	fpOp   m68kFPUNativeOp
	fpSrc  int
	fpDst  int
	fpPrec int
}

func wm68kSizeFromBits(bits uint16) uint32 {
	switch bits {
	case 0:
		return 1
	case 1:
		return 2
	case 2:
		return 4
	}
	return 0
}

func wm68kReadImm(memory []byte, pc uint32, size uint32) (uint32, bool) {
	switch size {
	case 1:
		if int(pc)+3 >= len(memory) {
			return 0, false
		}
		return uint32(memory[pc+3]), true // byte immediate in low 8 bits of the ext word
	case 2:
		if int(pc)+3 >= len(memory) {
			return 0, false
		}
		return uint32(memory[pc+2])<<8 | uint32(memory[pc+3]), true
	case 4:
		if int(pc)+5 >= len(memory) {
			return 0, false
		}
		return uint32(memory[pc+2])<<24 | uint32(memory[pc+3])<<16 |
			uint32(memory[pc+4])<<8 | uint32(memory[pc+5]), true
	}
	return 0, false
}

// m68kWasmDecode classifies one instruction for the integer-core wasm emitter.
// Returns ok=false when the shape is not lowered natively (the dispatcher then
// interprets it). The decoded length must agree with the scanner so a bail PC
// is never wrong.
func m68kWasmDecode(ji *M68KJITInstr, memory []byte, instrPC uint32) (wm68kOp, bool) {
	op := ji.opcode
	bad := wm68kOp{}

	switch {
	case op == 0x4E75: // RTS
		if ji.length != 2 {
			return bad, false
		}
		return wm68kOp{class: wm68kClassRTS}, true

	case op&0xFFC0 == 0x4EC0 || op&0xFFC0 == 0x4E80: // JMP / JSR
		ea, ext, ok := wm68kDecodeEA((op>>3)&7, op&7, 4, memory, instrPC+2)
		if !ok || ea.kind != wm68kOpMem || ea.memKind == wm68kMemPost || ea.memKind == wm68kMemPre {
			return bad, false // control addressing only: (An)/(d16,An)/abs/(d16,PC)
		}
		if 2+uint16(ext) != ji.length {
			return bad, false
		}
		class := wm68kClassJMP
		if op&0xFFC0 == 0x4E80 {
			class = wm68kClassJSR
		}
		return wm68kOp{class: class, src: ea, fallPC: instrPC + uint32(ji.length)}, true

	case op&0xF000 == 0x6000: // BRA / BSR / Bcc
		cond := int((op >> 8) & 0xF)
		disp8 := int32(int8(op & 0xFF))
		var d int32
		switch {
		case (op & 0xFF) == 0x00: // 16-bit displacement
			dd, ok := wm68kReadDisp16(memory, instrPC+2)
			if !ok || ji.length != 4 {
				return bad, false
			}
			d = dd
		case (op & 0xFF) == 0xFF: // 32-bit displacement (68020)
			if int(instrPC)+5 >= len(memory) || ji.length != 6 {
				return bad, false
			}
			d = int32(uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
				uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5]))
		default: // 8-bit displacement
			if ji.length != 2 {
				return bad, false
			}
			d = disp8
		}
		target := uint32(int32(instrPC+2) + d)
		switch cond {
		case 0: // BRA
			return wm68kOp{class: wm68kClassBRA, target: target}, true
		case 1: // BSR
			return wm68kOp{class: wm68kClassBSR, target: target, fallPC: instrPC + uint32(ji.length)}, true
		default: // Bcc
			return wm68kOp{class: wm68kClassBcc, cond: cond, target: target, fallPC: instrPC + uint32(ji.length)}, true
		}

	case op == 0x4E71: // NOP
		if ji.length != 2 {
			return bad, false
		}
		return wm68kOp{class: wm68kClassNOP}, true

	case op&0xF000 == 0xF000: // Line-F: 68881 general register-to-register op
		if ji.length != 4 || int(instrPC)+3 >= len(memory) {
			return bad, false
		}
		cmdWord := uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3])
		fpOp, src, dst, prec, ok := m68kDecodeNativeFPURegToReg(op, cmdWord)
		if !ok {
			// The shared decode gates FINT/FINTRZ on host SSE4.1 (an amd64
			// concern); wasm always has the full f64 rounding set, so decode
			// those two forms directly with the same field checks.
			fpOp, src, dst, prec, ok = wm68kDecodeFPURoundToInt(op, cmdWord)
		}
		if !ok || !wm68kFPUEmittable(fpOp) {
			return bad, false
		}
		return wm68kOp{class: wm68kClassFPU, fpOp: fpOp, fpSrc: src, fpDst: dst, fpPrec: prec}, true

	case op&0xF100 == 0x7000: // MOVEQ
		if ji.length != 2 {
			return bad, false
		}
		imm := uint32(int32(int8(op & 0xFF)))
		return wm68kOp{class: wm68kClassMOVEQ, dst: wm68kOperand{kind: wm68kOpDn, reg: (op >> 9) & 7}, imm: imm}, true

	case op&0xC000 == 0x0000 && op&0x3000 != 0x0000: // MOVE / MOVEA
		var size uint32
		switch op & 0x3000 {
		case 0x1000:
			size = 1
		case 0x3000:
			size = 2
		case 0x2000:
			size = 4
		}
		srcMode := (op >> 3) & 7
		srcReg := op & 7
		dstMode := (op >> 6) & 7
		dstReg := (op >> 9) & 7
		src, srcExt, ok := wm68kDecodeEA(srcMode, srcReg, size, memory, instrPC+2)
		if !ok {
			return bad, false
		}
		if src.kind == wm68kOpAn && size == 1 {
			return bad, false // byte An source is illegal
		}
		// Decode the destination EA. MOVEA (dst An) has no dest extension words.
		var dst wm68kOperand
		var dstExt int
		if dstMode == 1 { // MOVEA
			if size == 1 {
				return bad, false
			}
			dst = wm68kOperand{kind: wm68kOpAn, reg: dstReg}
		} else {
			dst, dstExt, ok = wm68kDecodeEA(dstMode, dstReg, size, memory, instrPC+2+uint32(srcExt))
			if !ok {
				return bad, false
			}
			// Destination must be a writable data alterable EA: no An, no #imm,
			// no PC-relative. (An abs from (d16,PC) reaches here as wm68kMemAbs;
			// disallow write to a PC-relative dest by construction — MOVE dst
			// modes never encode PC-relative, so this is moot, but keep An/imm
			// out.)
			if dst.kind == wm68kOpAn || dst.kind == wm68kOpImm {
				return bad, false
			}
		}
		// At most one memory operand per instruction in this slice (sidesteps
		// the double-side-effect bail hazard when both operands auto-adjust the
		// same An).
		if src.isMem() && dst.isMem() {
			return bad, false
		}
		if 2+uint16(srcExt)+uint16(dstExt) != ji.length {
			return bad, false
		}
		if dstMode == 1 {
			return wm68kOp{class: wm68kClassMOVEA, size: size, src: src, dst: dst}, true
		}
		return wm68kOp{class: wm68kClassMOVE, size: size, src: src, dst: dst}, true

	case op&0xF0F8 == 0x50C8: // DBcc: 0101 cccc 11001 rrr
		d, ok := wm68kReadDisp16(memory, instrPC+2)
		if !ok || ji.length != 4 {
			return bad, false
		}
		cond := int((op >> 8) & 0xF)
		target := uint32(int32(instrPC+2) + d)
		return wm68kOp{class: wm68kClassDBcc, cond: cond,
			dst:    wm68kOperand{kind: wm68kOpDn, reg: op & 7},
			target: target, fallPC: instrPC + uint32(ji.length)}, true

	case op&0xF000 == 0x5000: // ADDQ/SUBQ (Scc/DBcc excluded: bits 7-6 == 11)
		if (op>>6)&3 == 3 {
			return bad, false
		}
		size := wm68kSizeFromBits((op >> 6) & 3)
		q := uint32((op >> 9) & 7)
		if q == 0 {
			q = 8
		}
		sub := op&0x0100 != 0
		mode := (op >> 3) & 7
		reg := op & 7
		switch mode {
		case 0:
			if ji.length != 2 {
				return bad, false
			}
			return wm68kOp{class: wm68kClassQuickD, size: size, q: q, sub: sub, dst: wm68kOperand{kind: wm68kOpDn, reg: reg}}, true
		case 1:
			if ji.length != 2 {
				return bad, false
			}
			if size == 1 {
				return bad, false // byte ADDQ/SUBQ to An is illegal
			}
			return wm68kOp{class: wm68kClassQuickA, size: 4, q: q, sub: sub, dst: wm68kOperand{kind: wm68kOpAn, reg: reg}}, true
		default: // memory destination (milestone 6 RMW)
			dst, ext, ok := wm68kDecodeEA(mode, reg, size, memory, instrPC+2)
			if !ok || !dst.isMem() || dst.pcRel {
				return bad, false
			}
			if 2+uint16(ext) != ji.length {
				return bad, false
			}
			return wm68kOp{class: wm68kClassQuickM, size: size, q: q, sub: sub, dst: dst}, true
		}

	case op&0xF000 == 0x0000: // immediate ALU group (ORI/ANDI/SUBI/ADDI/EORI/CMPI)
		alu, ok := wm68kImmALUSelector(op)
		if !ok {
			return bad, false
		}
		size := wm68kSizeFromBits((op >> 6) & 3)
		if size == 0 {
			return bad, false
		}
		if (op>>3)&7 != 0 { // dst must be Dn in this slice
			return bad, false
		}
		imm, ok := wm68kReadImm(memory, instrPC, size)
		if !ok {
			return bad, false
		}
		if 2+uint16(wm68kImmExtBytes(size)) != ji.length {
			return bad, false
		}
		return wm68kOp{class: wm68kClassImmALU, size: size, alu: alu, imm: imm, dst: wm68kOperand{kind: wm68kOpDn, reg: op & 7}}, true

	case op&0xFF00 == 0x4200: // CLR
		size := wm68kSizeFromBits((op >> 6) & 3)
		if size == 0 {
			return bad, false
		}
		dst, ext, ok := wm68kDecodeEA((op>>3)&7, op&7, size, memory, instrPC+2)
		if !ok || dst.kind == wm68kOpAn || dst.kind == wm68kOpImm {
			return bad, false
		}
		if 2+uint16(ext) != ji.length {
			return bad, false
		}
		return wm68kOp{class: wm68kClassCLR, size: size, dst: dst}, true

	case op&0xFF00 == 0x4A00: // TST
		size := wm68kSizeFromBits((op >> 6) & 3)
		if size == 0 {
			return bad, false
		}
		dst, ext, ok := wm68kDecodeEA((op>>3)&7, op&7, size, memory, instrPC+2)
		if !ok || dst.kind == wm68kOpAn || dst.kind == wm68kOpImm {
			return bad, false
		}
		if 2+uint16(ext) != ji.length {
			return bad, false
		}
		return wm68kOp{class: wm68kClassTST, size: size, dst: dst}, true

	default:
		if alu, dir, size, ok := wm68kDecodeALU(op); ok {
			dn := (op >> 9) & 7
			eaMode := (op >> 3) & 7
			eaReg := op & 7
			ea, ext, ok := wm68kDecodeEA(eaMode, eaReg, size, memory, instrPC+2)
			if !ok {
				return bad, false
			}
			if 2+uint16(ext) != ji.length {
				return bad, false
			}
			if !dir { // <ea>,Dn  (memory or reg/imm source; An source only for word/long)
				if ea.kind == wm68kOpAn && size == 1 {
					return bad, false
				}
				return wm68kOp{class: wm68kClassALUToD, size: size, alu: alu,
					src: ea, dst: wm68kOperand{kind: wm68kOpDn, reg: dn}}, true
			}
			// Dn,<ea>.
			if ea.kind == wm68kOpDn {
				return wm68kOp{class: wm68kClassALUToD2, size: size, alu: alu,
					src: wm68kOperand{kind: wm68kOpDn, reg: dn}, dst: ea}, true
			}
			// Read-modify-write to a memory destination (milestone 6). CMP has
			// no dir=true form (that encoding is EOR); An and PC-relative
			// destinations are not data alterable.
			if !ea.isMem() || ea.pcRel {
				return bad, false
			}
			return wm68kOp{class: wm68kClassALUToMem, size: size, alu: alu,
				src: wm68kOperand{kind: wm68kOpDn, reg: dn}, dst: ea}, true
		}
		return bad, false
	}
}

// wm68kDecodeFPURoundToInt decodes the FINT/FINTRZ register-to-register forms
// with the same field checks as m68kDecodeNativeFPURegToReg, bypassing that
// function's amd64 SSE4.1 gate. Everything else stays rejected.
func wm68kDecodeFPURoundToInt(opcode, cmdWord uint16) (op m68kFPUNativeOp, src, dst, precision int, ok bool) {
	if (opcode>>6)&0x7 != 0 {
		return // not a general FPU instruction
	}
	if cmdWord&0x8000 != 0 || (cmdWord>>14)&1 != 0 {
		return // control register / FMOVEM / EA source
	}
	src = int((cmdWord >> 10) & 0x7)
	dst = int((cmdWord >> 7) & 0x7)
	baseOp, prec := m68kFPUDecodePrecisionOpmode(cmdWord & 0x7F)
	precision = prec
	switch baseOp {
	case FPU_OP_FINT:
		return m68kFPUNativeFINT, src, dst, precision, true
	case FPU_OP_FINTRZ:
		return m68kFPUNativeFINTRZ, src, dst, precision, true
	}
	return 0, 0, 0, 0, false
}

// wm68kDecodeEA decodes an effective address into an operand and returns the
// number of extension bytes it consumes. Only register-direct, immediate and
// the non-indexed memory forms are lowered; index and 68020 full-format modes
// reject to the interpreter. srcExtPC points at this operand's first extension
// word.
func wm68kDecodeEA(mode, reg uint16, size uint32, memory []byte, extPC uint32) (wm68kOperand, int, bool) {
	switch mode {
	case 0: // Dn
		return wm68kOperand{kind: wm68kOpDn, reg: reg}, 0, true
	case 1: // An
		return wm68kOperand{kind: wm68kOpAn, reg: reg}, 0, true
	case 2: // (An)
		return wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemInd, reg: reg}, 0, true
	case 3: // (An)+
		return wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemPost, reg: reg}, 0, true
	case 4: // -(An)
		return wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemPre, reg: reg}, 0, true
	case 5: // (d16,An)
		d, ok := wm68kReadDisp16(memory, extPC)
		if !ok {
			return wm68kOperand{}, 0, false
		}
		return wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemDisp, reg: reg, disp: d}, 2, true
	case 7:
		switch reg {
		case 0: // (xxx).W
			if int(extPC)+1 >= len(memory) {
				return wm68kOperand{}, 0, false
			}
			addr := uint32(int32(int16(uint16(memory[extPC])<<8 | uint16(memory[extPC+1]))))
			return wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemAbs, abs: addr}, 2, true
		case 1: // (xxx).L
			if int(extPC)+3 >= len(memory) {
				return wm68kOperand{}, 0, false
			}
			addr := uint32(memory[extPC])<<24 | uint32(memory[extPC+1])<<16 |
				uint32(memory[extPC+2])<<8 | uint32(memory[extPC+3])
			return wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemAbs, abs: addr}, 4, true
		case 2: // (d16,PC): constant address relative to the extension word PC
			d, ok := wm68kReadDisp16(memory, extPC)
			if !ok {
				return wm68kOperand{}, 0, false
			}
			return wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemAbs, abs: uint32(int32(extPC) + d), pcRel: true}, 2, true
		case 4: // #imm
			imm, ok := wm68kReadImm2(memory, extPC, size)
			if !ok {
				return wm68kOperand{}, 0, false
			}
			return wm68kOperand{kind: wm68kOpImm, imm: imm}, wm68kImmExtBytes(size), true
		}
	}
	return wm68kOperand{}, 0, false
}

func wm68kReadDisp16(memory []byte, pc uint32) (int32, bool) {
	if int(pc)+1 >= len(memory) {
		return 0, false
	}
	return int32(int16(uint16(memory[pc])<<8 | uint16(memory[pc+1]))), true
}

// wm68kReadImm2 reads an immediate whose first byte is at extPC (the extension
// word), unlike wm68kReadImm which is anchored at the opcode.
func wm68kReadImm2(memory []byte, extPC uint32, size uint32) (uint32, bool) {
	switch size {
	case 1:
		if int(extPC)+1 >= len(memory) {
			return 0, false
		}
		return uint32(memory[extPC+1]), true // byte immediate in low 8 bits of the ext word
	case 2:
		if int(extPC)+1 >= len(memory) {
			return 0, false
		}
		return uint32(memory[extPC])<<8 | uint32(memory[extPC+1]), true
	case 4:
		if int(extPC)+3 >= len(memory) {
			return 0, false
		}
		return uint32(memory[extPC])<<24 | uint32(memory[extPC+1])<<16 |
			uint32(memory[extPC+2])<<8 | uint32(memory[extPC+3]), true
	}
	return 0, false
}

func wm68kImmExtBytes(size uint32) int {
	switch size {
	case 1, 2:
		return 2
	case 4:
		return 4
	}
	return 0
}

// wm68kImmALUSelector maps a group-0 opcode to its ALU op, rejecting the
// bit-manipulation and MOVEP encodings that also live in group 0.
func wm68kImmALUSelector(op uint16) (int, bool) {
	if op&0x0100 != 0 { // dynamic bit ops / MOVEP
		return 0, false
	}
	switch op & 0x0F00 {
	case 0x0000:
		return wm68kALUOr, true
	case 0x0200:
		return wm68kALUAnd, true
	case 0x0400:
		return wm68kALUSub, true
	case 0x0600:
		return wm68kALUAdd, true
	case 0x0A00:
		return wm68kALUEor, true
	case 0x0C00:
		return wm68kALUCmp, true
	}
	return 0, false
}

// wm68kDecodeALU decodes the register-form ADD/SUB/AND/OR/CMP/EOR group.
// dir=false means <ea>,Dn; dir=true means Dn,<ea>. CMP only exists in dir=false;
// EOR only in dir=true (group 0xB). Returns ok=false for MUL/DIV/ADDA/CMPA/etc.
func wm68kDecodeALU(op uint16) (alu int, dir bool, size uint32, ok bool) {
	opmode := (op >> 6) & 7
	size = wm68kSizeFromBits(opmode & 3)
	if size == 0 { // opmode 3 or 7 = word/long An or MUL/DIV forms
		return 0, false, 0, false
	}
	dir = opmode&4 != 0
	switch op & 0xF000 {
	case 0xD000: // ADD
		return wm68kALUAdd, dir, size, true
	case 0x9000: // SUB
		return wm68kALUSub, dir, size, true
	case 0xC000: // AND
		return wm68kALUAnd, dir, size, true
	case 0x8000: // OR
		return wm68kALUOr, dir, size, true
	case 0xB000: // CMP (dir=false) / EOR (dir=true)
		if dir {
			return wm68kALUEor, true, size, true
		}
		return wm68kALUCmp, false, size, true
	}
	return 0, false, 0, false
}

// ---------------------------------------------------------------------------
// Emitter
// ---------------------------------------------------------------------------

// m68kWasmEmitter builds one block body against the fixed local layout above.
type m68kWasmEmitter struct {
	b       *wasmBody
	startPC uint32
	// Per-instruction context for mid-block memory bails.
	curPC      uint32 // guest PC of the instruction being emitted
	curRetired int    // instructions fully retired before this one
	curLen     uint32 // byte length of the instruction being emitted
	// Milestone 6 state.
	loopMode  bool // block is a structured in-block self-loop
	flagsDead bool // current instruction's CCR production is provably dead
}

// flushCCR materialises the modelled CCR into cpu.SR's low byte. Shared by the
// success epilogue and the mid-block bail path.
func (e *m68kWasmEmitter) flushCCR() {
	b := e.b
	e.ctxLoadI32(m68kCtxOffSRPtr)
	b.localSet(wm68kLT0) // SR base
	b.localGet(wm68kLT0) // addr for store
	b.localGet(wm68kLT0) // load current SR
	b.memOp(wasmOpI32Load16U, 1, 0)
	b.i32Const(0xFF00)
	b.op(wasmOpI32And)
	b.localGet(wm68kLCCR)
	b.i32Const(0x1F)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)
	b.memOp(wasmOpI32Store16, 1, 0)
}

// ctxLoadI32 pushes ctx.<field> (a 32-bit field or the low 32 bits of a
// pointer field, which holds a linear-memory offset).
func (e *m68kWasmEmitter) ctxLoadI32(off uint32) {
	e.b.localGet(wm68kLCtx)
	e.b.i32Load(2, off)
}

// ctxStoreI32 stores the value on top of the stack to ctx.<field>. The caller
// pushes the value; this helper prefixes the address and appends the store, so
// callers must arrange address-then-value ordering. To keep call sites simple
// we instead expose storeCtxConst / storeCtxLocal below.
func (e *m68kWasmEmitter) storeCtxConst(off uint32, v int32) {
	e.b.localGet(wm68kLCtx)
	e.b.i32Const(v)
	e.b.i32Store(2, off)
}

func (e *m68kWasmEmitter) storeCtxLocal(off, local uint32) {
	e.b.localGet(wm68kLCtx)
	e.b.localGet(local)
	e.b.i32Store(2, off)
}

// storeRetCountPlus publishes RetCount = LRet + c. LRet is zero except in loop
// blocks, where it accumulates the instructions retired by completed
// iterations, so every straight-line block keeps its old constant count.
func (e *m68kWasmEmitter) storeRetCountPlus(c int) {
	b := e.b
	b.localGet(wm68kLCtx)
	b.localGet(wm68kLRet)
	b.i32Const(int32(c))
	b.op(wasmOpI32Add)
	b.i32Store(2, m68kCtxOffRetCount)
}

// prologue loads the register-file bases, memory base/size and the live CCR.
func (e *m68kWasmEmitter) prologue() {
	b := e.b
	e.ctxLoadI32(m68kCtxOffDataRegsPtr)
	b.localSet(wm68kLDBase)
	e.ctxLoadI32(m68kCtxOffAddrRegsPtr)
	b.localSet(wm68kLABase)
	e.ctxLoadI32(m68kCtxOffMemPtr)
	b.localSet(wm68kLMBase)
	e.ctxLoadI32(m68kCtxOffMemSize)
	b.localSet(wm68kLMSize)
	// CCR = SR & 0x1F.
	e.ctxLoadI32(m68kCtxOffSRPtr)
	b.localSet(wm68kLT0) // SR base
	b.localGet(wm68kLT0)
	b.memOp(wasmOpI32Load16U, 1, 0)
	b.i32Const(0x1F)
	b.op(wasmOpI32And)
	b.localSet(wm68kLCCR)
}

// epilogue flushes the CCR into cpu.SR's low byte and publishes RetPC/RetCount.
func (e *m68kWasmEmitter) epilogue(retPC uint32, retCount int) {
	e.flushCCR()
	e.storeCtxConst(m68kCtxOffRetPC, int32(retPC))
	e.storeRetCountPlus(retCount)
}

// ---------------------------------------------------------------------------
// Memory effective addresses, guarded big-endian access, mid-block bail
// ---------------------------------------------------------------------------

// emitBail closes the block early, publishes RetPC=faulting PC,
// RetCount=retired predecessors and NeedIOFallback, then returns from the wasm
// function. Guards normally precede their instruction's mutation. For a fused
// synthetic RTS, earlier JSR and leaf effects are already committed and curPC
// is the real RTS source PC, so only that return is interpreted.
func (e *m68kWasmEmitter) emitBail() {
	e.flushCCR()
	e.storeCtxConst(m68kCtxOffRetPC, int32(e.curPC))
	e.storeRetCountPlus(e.curRetired)
	e.storeCtxConst(m68kCtxOffNeedIOFallback, 1)
	e.b.op(wasmOpReturn)
}

// emitIOProbeOr ORs an I/O-page fault for pageAddrLocal (a guest address whose
// 256-byte page is probed) into the running fault flag in wm68kLT4. Uses T0-T2
// scratch. A nil bitmap or an out-of-range page contributes no fault.
func (e *m68kWasmEmitter) emitIOProbeOr(pageAddrLocal uint32) {
	b := e.b
	e.ctxLoadI32(m68kCtxOffIOPageBitmapPtr)
	b.localSet(wm68kLT0) // bitmap base (linear offset; 0 = none)
	b.localGet(wm68kLT0)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	// no bitmap: nothing to add
	b.elseBranch()
	b.localGet(pageAddrLocal)
	b.i32Const(8)
	b.op(wasmOpI32ShrU)
	b.localSet(wm68kLT1) // page index
	b.localGet(wm68kLT1)
	e.ctxLoadI32(m68kCtxOffIOPageBitmapLen)
	b.op(wasmOpI32LtU)
	b.ifVoid()
	b.localGet(wm68kLT0)
	b.localGet(wm68kLT1)
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load8U, 0, 0)
	b.i32Const(0)
	b.op(wasmOpI32Ne)
	b.localGet(wm68kLT4)
	b.op(wasmOpI32Or)
	b.localSet(wm68kLT4)
	b.end() // if page<len
	b.end() // if bitmap
}

// emitGuardBail faults the block if the access [EA, EA+size) is out of RAM
// bounds or lands on an I/O page. EA must already be in wm68kLEA.
func (e *m68kWasmEmitter) emitGuardBail(size uint32) {
	b := e.b
	// Bounds: EA >= MemSize OR EA+size > MemSize.
	b.localGet(wm68kLEA)
	b.localGet(wm68kLMSize)
	b.op(wasmOpI32GeU)
	b.localGet(wm68kLEA)
	b.i32Const(int32(size))
	b.op(wasmOpI32Add)
	b.localGet(wm68kLMSize)
	b.op(wasmOpI32GtU)
	b.op(wasmOpI32Or)
	b.localSet(wm68kLT4) // running fault flag
	// I/O page probe (first and, for multi-byte, last touched page).
	e.emitIOProbeOr(wm68kLEA)
	if size > 1 {
		b.localGet(wm68kLEA)
		b.i32Const(int32(size - 1))
		b.op(wasmOpI32Add)
		b.localSet(wm68kLT2) // reuse as last-byte address holder
		e.emitIOProbeOr(wm68kLT2)
	}
	b.localGet(wm68kLT4)
	b.ifVoid()
	e.emitBail()
	b.end()
}

// wm68kStep returns the (An)+/-(An) adjustment for a base register and size,
// keeping A7 word-aligned on byte access.
func wm68kStep(reg uint16, size uint32) int32 {
	if reg == 7 && size == 1 {
		return 2
	}
	return int32(size)
}

// emitEAAddr computes the access address of a memory operand into wm68kLEA,
// guards it, and only then commits any (An)+/-(An) side effect so a bailed
// instruction re-executes cleanly on the interpreter.
func (e *m68kWasmEmitter) emitEAAddr(o wm68kOperand, size uint32) {
	b := e.b
	switch o.memKind {
	case wm68kMemInd:
		e.pushAReg(o.reg)
		b.localSet(wm68kLEA)
		e.emitGuardBail(size)
	case wm68kMemDisp:
		e.pushAReg(o.reg)
		b.i32Const(o.disp)
		b.op(wasmOpI32Add)
		b.localSet(wm68kLEA)
		e.emitGuardBail(size)
	case wm68kMemAbs:
		b.i32Const(int32(o.abs))
		b.localSet(wm68kLEA)
		e.emitGuardBail(size)
	case wm68kMemPost:
		e.pushAReg(o.reg)
		b.localSet(wm68kLEA)
		e.emitGuardBail(size)
		// Commit An += step.
		b.localGet(wm68kLABase)
		e.pushAReg(o.reg)
		b.i32Const(wm68kStep(o.reg, size))
		b.op(wasmOpI32Add)
		b.i32Store(2, uint32(o.reg)*4)
	case wm68kMemPre:
		e.pushAReg(o.reg)
		b.i32Const(wm68kStep(o.reg, size))
		b.op(wasmOpI32Sub)
		b.localSet(wm68kLEA)
		e.emitGuardBail(size)
		// Commit An = EA.
		b.localGet(wm68kLABase)
		b.localGet(wm68kLEA)
		b.i32Store(2, uint32(o.reg)*4)
	}
}

// pushMemValueBE loads a sized big-endian value from [MemBase+EA] onto the
// stack, zero-extended. Uses T0 as an address scratch.
func (e *m68kWasmEmitter) pushMemValueBE(size uint32) {
	b := e.b
	b.localGet(wm68kLMBase)
	b.localGet(wm68kLEA)
	b.op(wasmOpI32Add)
	b.localSet(wm68kLT0)
	switch size {
	case 1:
		b.localGet(wm68kLT0)
		b.memOp(wasmOpI32Load8U, 0, 0)
	case 2:
		b.localGet(wm68kLT0)
		b.memOp(wasmOpI32Load8U, 0, 0)
		b.i32Const(8)
		b.op(wasmOpI32Shl)
		b.localGet(wm68kLT0)
		b.memOp(wasmOpI32Load8U, 0, 1)
		b.op(wasmOpI32Or)
	default:
		b.localGet(wm68kLT0)
		b.memOp(wasmOpI32Load8U, 0, 0)
		b.i32Const(24)
		b.op(wasmOpI32Shl)
		b.localGet(wm68kLT0)
		b.memOp(wasmOpI32Load8U, 0, 1)
		b.i32Const(16)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
		b.localGet(wm68kLT0)
		b.memOp(wasmOpI32Load8U, 0, 2)
		b.i32Const(8)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
		b.localGet(wm68kLT0)
		b.memOp(wasmOpI32Load8U, 0, 3)
		b.op(wasmOpI32Or)
	}
}

// storeMemValueBE stores the sized low bits of valLocal big-endian to
// [MemBase+EA]. Uses T0 as an address scratch. After the store it records an
// exact SMC invalidation range if the written page holds compiled code.
func (e *m68kWasmEmitter) storeMemValueBE(size, valLocal uint32) {
	b := e.b
	b.localGet(wm68kLMBase)
	b.localGet(wm68kLEA)
	b.op(wasmOpI32Add)
	b.localSet(wm68kLT0)
	switch size {
	case 1:
		b.localGet(wm68kLT0)
		b.localGet(valLocal)
		b.memOp(wasmOpI32Store8, 0, 0)
	case 2:
		b.localGet(wm68kLT0)
		b.localGet(valLocal)
		b.i32Const(8)
		b.op(wasmOpI32ShrU)
		b.memOp(wasmOpI32Store8, 0, 0)
		b.localGet(wm68kLT0)
		b.localGet(valLocal)
		b.memOp(wasmOpI32Store8, 0, 1)
	default:
		b.localGet(wm68kLT0)
		b.localGet(valLocal)
		b.i32Const(24)
		b.op(wasmOpI32ShrU)
		b.memOp(wasmOpI32Store8, 0, 0)
		b.localGet(wm68kLT0)
		b.localGet(valLocal)
		b.i32Const(16)
		b.op(wasmOpI32ShrU)
		b.memOp(wasmOpI32Store8, 0, 1)
		b.localGet(wm68kLT0)
		b.localGet(valLocal)
		b.i32Const(8)
		b.op(wasmOpI32ShrU)
		b.memOp(wasmOpI32Store8, 0, 2)
		b.localGet(wm68kLT0)
		b.localGet(valLocal)
		b.memOp(wasmOpI32Store8, 0, 3)
	}
	e.emitSMCStoreCheck(size)
	if e.loopMode {
		// A structured loop re-executes its own body without returning to the
		// dispatcher, so a store that hits a compiled-code page (NeedInval set
		// by the check above) must exit the loop: the modified code could be
		// this very block. The instruction is complete at this point (flags are
		// emitted before the store in every loop-eligible path), so resume at
		// the next instruction with this one counted as retired.
		b.localGet(wm68kLCtx)
		b.i32Load(2, m68kCtxOffNeedInval)
		b.ifVoid()
		e.flushCCR()
		e.storeCtxConst(m68kCtxOffRetPC, int32(e.curPC+e.curLen))
		e.storeRetCountPlus(e.curRetired + 1)
		b.op(wasmOpReturn)
		b.end()
	}
}

// emitSMCStoreCheck sets NeedInval/InvalAddr/InvalSize when the just-written
// guest range (EA in wm68kLEA) lands on a page holding compiled code. The
// per-page code bitmap (CodePageBitmapPtr, one byte per 4 KiB page) is nil in
// test contexts and when no block is compiled, which short-circuits. This
// mirrors the arm64 emitSMCStoreCheck so native chaining (milestone 6) can rely
// on precise invalidation. Uses T0-T2 scratch.
func (e *m68kWasmEmitter) emitSMCStoreCheck(size uint32) {
	b := e.b
	e.ctxLoadI32(m68kCtxOffCodePageBitmapPtr)
	b.localSet(wm68kLT0) // bitmap base (0 = none)
	b.localGet(wm68kLT0)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	b.elseBranch()
	// hit = bitmap[EA>>12] | (size>1 ? bitmap[(EA+size-1)>>12] : 0)
	b.localGet(wm68kLT0)
	b.localGet(wm68kLEA)
	b.i32Const(12)
	b.op(wasmOpI32ShrU)
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load8U, 0, 0)
	if size > 1 {
		b.localGet(wm68kLT0)
		b.localGet(wm68kLEA)
		b.i32Const(int32(size - 1))
		b.op(wasmOpI32Add)
		b.i32Const(12)
		b.op(wasmOpI32ShrU)
		b.op(wasmOpI32Add)
		b.memOp(wasmOpI32Load8U, 0, 0)
		b.op(wasmOpI32Or)
	}
	b.localSet(wm68kLT1)
	b.localGet(wm68kLT1)
	b.ifVoid()
	e.storeCtxLocal(m68kCtxOffInvalAddr, wm68kLEA)
	e.storeCtxConst(m68kCtxOffInvalSize, int32(size))
	e.storeCtxConst(m68kCtxOffNeedInval, 1)
	b.end() // if hit
	b.end() // if bitmap present
}

// ---- register access ----

func (e *m68kWasmEmitter) pushDReg(n uint16) {
	e.b.localGet(wm68kLDBase)
	e.b.i32Load(2, uint32(n)*4)
}

func (e *m68kWasmEmitter) pushAReg(n uint16) {
	e.b.localGet(wm68kLABase)
	e.b.i32Load(2, uint32(n)*4)
}

func wm68kSizeMask(size uint32) uint32 {
	switch size {
	case 1:
		return 0xFF
	case 2:
		return 0xFFFF
	}
	return 0xFFFFFFFF
}

func wm68kTopShift(size uint32) uint32 { return (4 - size) * 8 }

// writeDRegLocal writes local wm68kLTx (holding a sized, zero-extended value)
// into Dn, merging sub-word writes so the high bits of the register survive.
func (e *m68kWasmEmitter) writeDRegLocal(n uint16, size uint32, valLocal uint32) {
	b := e.b
	mask := wm68kSizeMask(size)
	b.localGet(wm68kLDBase) // store address
	if size == 4 {
		b.localGet(valLocal)
	} else {
		// (old & ~mask) | (val & mask)
		b.localGet(wm68kLDBase)
		b.i32Load(2, uint32(n)*4)
		b.i32Const(int32(^mask))
		b.op(wasmOpI32And)
		b.localGet(valLocal)
		b.i32Const(int32(mask))
		b.op(wasmOpI32And)
		b.op(wasmOpI32Or)
	}
	b.i32Store(2, uint32(n)*4)
}

// pushOperand pushes a sized, zero-extended source operand value.
func (e *m68kWasmEmitter) pushOperand(o wm68kOperand, size uint32) {
	b := e.b
	switch o.kind {
	case wm68kOpDn:
		e.pushDReg(o.reg)
		if size != 4 {
			b.i32Const(int32(wm68kSizeMask(size)))
			b.op(wasmOpI32And)
		}
	case wm68kOpAn:
		e.pushAReg(o.reg)
		if size != 4 {
			b.i32Const(int32(wm68kSizeMask(size)))
			b.op(wasmOpI32And)
		}
	case wm68kOpImm:
		b.i32Const(int32(o.imm & wm68kSizeMask(size)))
	case wm68kOpMem:
		// Value was captured into LMV by loadMemToLMV before any CCR mutation.
		b.localGet(wm68kLMV)
		if size != 4 {
			b.i32Const(int32(wm68kSizeMask(size)))
			b.op(wasmOpI32And)
		}
	}
}

// loadMemToLMV computes a memory operand's EA (guarding, which may bail before
// any CCR mutation), reads the sized big-endian value and captures it in the
// guard-safe LMV local. Must be called at the point the interpreter reads the
// operand.
func (e *m68kWasmEmitter) loadMemToLMV(o wm68kOperand, size uint32) {
	e.emitEAAddr(o, size)
	e.pushMemValueBE(size)
	e.b.localSet(wm68kLMV)
}

// storeMemDest computes a memory destination EA (guarding) and stores the sized
// low bits of valLocal big-endian. The guard must precede any CCR mutation by
// the caller.
func (e *m68kWasmEmitter) storeMemDest(o wm68kOperand, size, valLocal uint32) {
	e.emitEAAddr(o, size)
	e.storeMemValueBE(size, valLocal)
}

// ---- flag materialisation ----

// flagsArith computes the full NZVC(X) set for an ADD/SUB/CMP whose top-aligned
// dest is in T1, top-aligned source in T2 and top-aligned result in T3. When
// setX is false (CMP), X is preserved from the current CCR.
func (e *m68kWasmEmitter) flagsArith(isSub, setX bool) {
	if e.flagsDead {
		return // CCR production proven dead by m68kCCRLiveness
	}
	b := e.b
	// C -> T0.
	if isSub {
		b.localGet(wm68kLT1)
		b.localGet(wm68kLT2)
		b.op(wasmOpI32LtU) // borrow: dest < source
	} else {
		b.localGet(wm68kLT3)
		b.localGet(wm68kLT1)
		b.op(wasmOpI32LtU) // carry: result < dest
	}
	b.localSet(wm68kLT0)
	// V -> T4.
	b.localGet(wm68kLT1)
	b.localGet(wm68kLT2)
	b.op(wasmOpI32Xor)
	if !isSub {
		b.i32Const(-1)
		b.op(wasmOpI32Xor) // ~(dest^source) for add
	}
	b.localGet(wm68kLT1)
	b.localGet(wm68kLT3)
	b.op(wasmOpI32Xor)
	b.op(wasmOpI32And)
	b.i32Const(31)
	b.op(wasmOpI32ShrU)
	b.localSet(wm68kLT4)
	// Assemble CCR = C | V<<1 | Z<<2 | N<<3 | X<<4.
	b.localGet(wm68kLT0) // C
	b.localGet(wm68kLT4)
	b.i32Const(wm68kBitV)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localGet(wm68kLT3)
	b.op(wasmOpI32Eqz) // Z
	b.i32Const(wm68kBitZ)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localGet(wm68kLT3)
	b.i32Const(31)
	b.op(wasmOpI32ShrU) // N
	b.i32Const(wm68kBitN)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	if setX {
		b.localGet(wm68kLT0) // X = C
	} else {
		b.localGet(wm68kLCCR) // preserve old X
		b.i32Const(wm68kBitX)
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
	}
	b.i32Const(wm68kBitX)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(wm68kLCCR)
}

// flagsLogicNZ sets N and Z from the top-aligned result in T3, preserving X,
// V and C (interpreter SetFlagsNZ parity for AND/OR/EOR/NOT).
func (e *m68kWasmEmitter) flagsLogicNZ() {
	if e.flagsDead {
		return // CCR production proven dead by m68kCCRLiveness
	}
	b := e.b
	// CCR = (CCR & (X|V|C)) | Z<<2 | N<<3.
	b.localGet(wm68kLCCR)
	b.i32Const((1 << wm68kBitX) | (1 << wm68kBitV) | (1 << wm68kBitC))
	b.op(wasmOpI32And)
	b.localGet(wm68kLT3)
	b.op(wasmOpI32Eqz)
	b.i32Const(wm68kBitZ)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localGet(wm68kLT3)
	b.i32Const(31)
	b.op(wasmOpI32ShrU)
	b.i32Const(wm68kBitN)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(wm68kLCCR)
}

// flagsMoveNZ sets N and Z from the top-aligned value in T3, clears V and C,
// preserves X (MOVE/TST).
func (e *m68kWasmEmitter) flagsMoveNZ() {
	if e.flagsDead {
		return // CCR production proven dead by m68kCCRLiveness
	}
	b := e.b
	// CCR = (CCR & X) | Z<<2 | N<<3.
	b.localGet(wm68kLCCR)
	b.i32Const(1 << wm68kBitX)
	b.op(wasmOpI32And)
	b.localGet(wm68kLT3)
	b.op(wasmOpI32Eqz)
	b.i32Const(wm68kBitZ)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localGet(wm68kLT3)
	b.i32Const(31)
	b.op(wasmOpI32ShrU)
	b.i32Const(wm68kBitN)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(wm68kLCCR)
}

// flagsStatic sets a compile-time N/Z pair (V=C=0, X preserved): MOVEQ, CLR.
func (e *m68kWasmEmitter) flagsStatic(negative, zero bool) {
	if e.flagsDead {
		return // CCR production proven dead by m68kCCRLiveness
	}
	b := e.b
	bits := int32(0)
	if negative {
		bits |= 1 << wm68kBitN
	}
	if zero {
		bits |= 1 << wm68kBitZ
	}
	b.localGet(wm68kLCCR)
	b.i32Const(1 << wm68kBitX)
	b.op(wasmOpI32And)
	b.i32Const(bits)
	b.op(wasmOpI32Or)
	b.localSet(wm68kLCCR)
}

// ---- top-aligned operand staging for arithmetic ----

// stageTopAligned pushes a sized operand and stores it top-aligned in target.
func (e *m68kWasmEmitter) stageTopAligned(o wm68kOperand, size uint32, target uint32) {
	b := e.b
	e.pushOperand(o, size)
	if sh := wm68kTopShift(size); sh != 0 {
		b.i32Const(int32(sh))
		b.op(wasmOpI32Shl)
	}
	b.localSet(target)
}

// stageTopAlignedImm stores an immediate top-aligned in target.
func (e *m68kWasmEmitter) stageTopAlignedImm(imm, size, target uint32) {
	b := e.b
	b.i32Const(int32((imm & wm68kSizeMask(size)) << wm68kTopShift(size)))
	b.localSet(target)
}

// ---------------------------------------------------------------------------
// Per-class emit
// ---------------------------------------------------------------------------

func (e *m68kWasmEmitter) emit(op wm68kOp) {
	switch op.class {
	case wm68kClassNOP:
		// nothing
	case wm68kClassMOVEQ:
		e.emitMOVEQ(op)
	case wm68kClassMOVE:
		e.emitMOVE(op)
	case wm68kClassMOVEA:
		e.emitMOVEA(op)
	case wm68kClassALUToD:
		e.emitALU(op, op.dst.reg, op.src, false)
	case wm68kClassALUToD2:
		e.emitALU(op, op.dst.reg, op.src, true)
	case wm68kClassImmALU:
		e.emitImmALU(op)
	case wm68kClassQuickD:
		e.emitQuickD(op)
	case wm68kClassQuickA:
		e.emitQuickA(op)
	case wm68kClassTST:
		e.emitTST(op)
	case wm68kClassCLR:
		e.emitCLR(op)
	case wm68kClassALUToMem:
		e.emitALUToMem(op)
	case wm68kClassQuickM:
		e.emitQuickM(op)
	case wm68kClassFPU:
		e.emitFPU(op)
	}
}

func (e *m68kWasmEmitter) emitMOVEQ(op wm68kOp) {
	b := e.b
	b.i32Const(int32(op.imm))
	b.localSet(wm68kLT0)
	e.writeDRegLocal(op.dst.reg, 4, wm68kLT0)
	e.flagsStatic(int32(op.imm) < 0, op.imm == 0)
}

func (e *m68kWasmEmitter) emitMOVE(op wm68kOp) {
	b := e.b
	// Read the source first (interpreter order). A memory source guards here,
	// before any CCR mutation.
	if op.src.isMem() {
		e.loadMemToLMV(op.src, op.size)
	}
	e.pushOperand(op.src, op.size)
	b.localSet(wm68kLMB) // sized moved value (guard/store-safe local)
	// For a memory destination, guard (and commit the EA side effect) before
	// touching the CCR so a bail leaves the CCR at its pre-instruction value.
	if op.dst.isMem() {
		e.emitEAAddr(op.dst, op.size)
	}
	// Flags: NZ from the moved value, V=C=0, X preserved.
	b.localGet(wm68kLMB)
	if sh := wm68kTopShift(op.size); sh != 0 {
		b.i32Const(int32(sh))
		b.op(wasmOpI32Shl)
	}
	b.localSet(wm68kLT3)
	e.flagsMoveNZ()
	// Writeback.
	if op.dst.isMem() {
		e.storeMemValueBE(op.size, wm68kLMB)
	} else {
		e.writeDRegLocal(op.dst.reg, op.size, wm68kLMB)
	}
}

func (e *m68kWasmEmitter) emitMOVEA(op wm68kOp) {
	b := e.b
	// MOVEA sign-extends a word source to 32 bits; long is verbatim. No flags.
	if op.src.isMem() {
		e.loadMemToLMV(op.src, op.size)
	}
	e.pushOperand(op.src, op.size)
	if op.size == 2 {
		b.i32Const(16)
		b.op(wasmOpI32Shl)
		b.i32Const(16)
		b.op(wasmOpI32ShrS)
	}
	b.localSet(wm68kLT0)
	b.localGet(wm68kLABase)
	b.localGet(wm68kLT0)
	b.i32Store(2, uint32(op.dst.reg)*4)
}

// emitALU lowers ADD/SUB/AND/OR/CMP/EOR with a Dn destination. A memory source
// is captured before any CCR mutation so a guard bail leaves the CCR intact.
func (e *m68kWasmEmitter) emitALU(op wm68kOp, dstReg uint16, src wm68kOperand, dnDst bool) {
	b := e.b
	if src.isMem() {
		e.loadMemToLMV(src, op.size)
	}
	dst := wm68kOperand{kind: wm68kOpDn, reg: dstReg}
	switch op.alu {
	case wm68kALUAdd, wm68kALUSub, wm68kALUCmp:
		e.stageTopAligned(dst, op.size, wm68kLT1) // A = dest
		e.stageTopAligned(src, op.size, wm68kLT2) // B = source
		b.localGet(wm68kLT1)
		b.localGet(wm68kLT2)
		if op.alu == wm68kALUAdd {
			b.op(wasmOpI32Add)
		} else {
			b.op(wasmOpI32Sub)
		}
		b.localSet(wm68kLT3) // RES top-aligned
		if op.alu != wm68kALUCmp {
			// writeback: RES >> sh (zero-extended sized result).
			b.localGet(wm68kLT3)
			if sh := wm68kTopShift(op.size); sh != 0 {
				b.i32Const(int32(sh))
				b.op(wasmOpI32ShrU)
			}
			b.localSet(wm68kLT0)
			e.writeDRegLocal(dstReg, op.size, wm68kLT0)
		}
		e.flagsArith(op.alu != wm68kALUAdd, op.alu != wm68kALUCmp)
	default: // AND/OR/EOR
		e.pushOperand(dst, op.size)
		e.pushOperand(src, op.size)
		switch op.alu {
		case wm68kALUAnd:
			b.op(wasmOpI32And)
		case wm68kALUOr:
			b.op(wasmOpI32Or)
		case wm68kALUEor:
			b.op(wasmOpI32Xor)
		}
		b.localSet(wm68kLT0) // sized result (may have high bits for long)
		e.writeDRegLocal(dstReg, op.size, wm68kLT0)
		b.localGet(wm68kLT0)
		if sh := wm68kTopShift(op.size); sh != 0 {
			b.i32Const(int32(sh))
			b.op(wasmOpI32Shl)
		}
		b.localSet(wm68kLT3)
		e.flagsLogicNZ()
	}
}

// emitALUToMem lowers ADD/SUB/AND/OR/EOR Dn,<ea=mem>: one EA computation, one
// guard, read-modify-write against the fixed address in LEA. The guard (and
// any (An)+/-(An) commit) runs before every CCR mutation, and the flags are
// emitted before the store so a loop-mode SMC exit at the store observes the
// completed instruction's CCR.
func (e *m68kWasmEmitter) emitALUToMem(op wm68kOp) {
	b := e.b
	e.emitEAAddr(op.dst, op.size)
	e.pushMemValueBE(op.size)
	b.localSet(wm68kLMV)                  // old destination value
	mem := wm68kOperand{kind: wm68kOpMem} // pushOperand reads LMV
	switch op.alu {
	case wm68kALUAdd, wm68kALUSub:
		e.stageTopAligned(mem, op.size, wm68kLT1)    // A = dest (memory)
		e.stageTopAligned(op.src, op.size, wm68kLT2) // B = source Dn
		b.localGet(wm68kLT1)
		b.localGet(wm68kLT2)
		if op.alu == wm68kALUAdd {
			b.op(wasmOpI32Add)
		} else {
			b.op(wasmOpI32Sub)
		}
		b.localSet(wm68kLT3)
		b.localGet(wm68kLT3)
		if sh := wm68kTopShift(op.size); sh != 0 {
			b.i32Const(int32(sh))
			b.op(wasmOpI32ShrU)
		}
		b.localSet(wm68kLMB) // sized result for the store
		e.flagsArith(op.alu == wm68kALUSub, true)
	default: // AND/OR/EOR: NZ from result, V/C/X preserved
		e.pushOperand(mem, op.size)
		e.pushOperand(op.src, op.size)
		switch op.alu {
		case wm68kALUAnd:
			b.op(wasmOpI32And)
		case wm68kALUOr:
			b.op(wasmOpI32Or)
		case wm68kALUEor:
			b.op(wasmOpI32Xor)
		}
		b.localSet(wm68kLMB)
		b.localGet(wm68kLMB)
		if sh := wm68kTopShift(op.size); sh != 0 {
			b.i32Const(int32(sh))
			b.op(wasmOpI32Shl)
		}
		b.localSet(wm68kLT3)
		e.flagsLogicNZ()
	}
	e.storeMemValueBE(op.size, wm68kLMB)
}

// emitQuickM lowers ADDQ/SUBQ #q,<ea=mem> with full arithmetic flags.
func (e *m68kWasmEmitter) emitQuickM(op wm68kOp) {
	b := e.b
	e.emitEAAddr(op.dst, op.size)
	e.pushMemValueBE(op.size)
	b.localSet(wm68kLMV)
	mem := wm68kOperand{kind: wm68kOpMem}
	e.stageTopAligned(mem, op.size, wm68kLT1)
	e.stageTopAlignedImm(op.q, op.size, wm68kLT2)
	b.localGet(wm68kLT1)
	b.localGet(wm68kLT2)
	if op.sub {
		b.op(wasmOpI32Sub)
	} else {
		b.op(wasmOpI32Add)
	}
	b.localSet(wm68kLT3)
	b.localGet(wm68kLT3)
	if sh := wm68kTopShift(op.size); sh != 0 {
		b.i32Const(int32(sh))
		b.op(wasmOpI32ShrU)
	}
	b.localSet(wm68kLMB)
	e.flagsArith(op.sub, true)
	e.storeMemValueBE(op.size, wm68kLMB)
}

func (e *m68kWasmEmitter) emitImmALU(op wm68kOp) {
	src := wm68kOperand{kind: wm68kOpImm, imm: op.imm}
	e.emitALU(op, op.dst.reg, src, true)
}

func (e *m68kWasmEmitter) emitQuickD(op wm68kOp) {
	b := e.b
	dst := wm68kOperand{kind: wm68kOpDn, reg: op.dst.reg}
	e.stageTopAligned(dst, op.size, wm68kLT1)
	e.stageTopAlignedImm(op.q, op.size, wm68kLT2)
	b.localGet(wm68kLT1)
	b.localGet(wm68kLT2)
	if op.sub {
		b.op(wasmOpI32Sub)
	} else {
		b.op(wasmOpI32Add)
	}
	b.localSet(wm68kLT3)
	b.localGet(wm68kLT3)
	if sh := wm68kTopShift(op.size); sh != 0 {
		b.i32Const(int32(sh))
		b.op(wasmOpI32ShrU)
	}
	b.localSet(wm68kLT0)
	e.writeDRegLocal(op.dst.reg, op.size, wm68kLT0)
	e.flagsArith(op.sub, true)
}

func (e *m68kWasmEmitter) emitQuickA(op wm68kOp) {
	b := e.b
	// ADDQ/SUBQ to An: full 32-bit, no flags.
	e.pushAReg(op.dst.reg)
	b.i32Const(int32(op.q))
	if op.sub {
		b.op(wasmOpI32Sub)
	} else {
		b.op(wasmOpI32Add)
	}
	b.localSet(wm68kLT0)
	b.localGet(wm68kLABase)
	b.localGet(wm68kLT0)
	b.i32Store(2, uint32(op.dst.reg)*4)
}

func (e *m68kWasmEmitter) emitTST(op wm68kOp) {
	b := e.b
	if op.dst.isMem() {
		e.loadMemToLMV(op.dst, op.size) // read-only; guards before flags
		e.pushOperand(op.dst, op.size)
	} else {
		e.pushDReg(op.dst.reg)
	}
	if sh := wm68kTopShift(op.size); sh != 0 {
		b.i32Const(int32(sh))
		b.op(wasmOpI32Shl)
	}
	b.localSet(wm68kLT3)
	e.flagsMoveNZ()
}

func (e *m68kWasmEmitter) emitCLR(op wm68kOp) {
	b := e.b
	if op.dst.isMem() {
		// Guard (and commit the EA side effect) before the CCR update.
		e.emitEAAddr(op.dst, op.size)
		b.i32Const(0)
		b.localSet(wm68kLMB)
		e.flagsStatic(false, true)
		e.storeMemValueBE(op.size, wm68kLMB)
		return
	}
	b.i32Const(0)
	b.localSet(wm68kLT0)
	e.writeDRegLocal(op.dst.reg, op.size, wm68kLT0)
	e.flagsStatic(false, true)
}

// ---------------------------------------------------------------------------
// Prefix admission + compile
// ---------------------------------------------------------------------------

// m68kWasmSupportedPrefix returns the number of leading instructions the wasm
// backend can lower. A block-ending control-flow instruction is included and
// stops the prefix (instructions after it belong to a different block).
func m68kWasmSupportedPrefix(instrs []M68KJITInstr, memory []byte, startPC uint32) int {
	n := 0
	for i := range instrs {
		if instrs[i].fusedFlag&(m68kFusedJSRLeafCall|m68kFusedRTSLeafReturn) != 0 {
			n++
			continue
		}
		instrPC := startPC + instrs[i].pcOffset
		op, ok := m68kWasmDecode(&instrs[i], memory, instrPC)
		if !ok {
			break
		}
		n++
		if wm68kIsTerminator(op.class) {
			break
		}
	}
	return n
}

// wm68kInstrStores reports whether a decoded op writes guest memory (used to
// pin its CCR production live in loop mode, where the store is a potential
// mid-loop SMC observation point).
func wm68kInstrStores(op wm68kOp) bool {
	switch op.class {
	case wm68kClassALUToMem, wm68kClassQuickM:
		return true
	case wm68kClassMOVE, wm68kClassCLR:
		return op.dst.isMem()
	}
	return false
}

// wm68kDetectLoop reports whether the supported prefix forms a structured
// in-block self-loop: the terminator is a Bcc or DBcc whose static target is
// the block start. JSR/JMP/BSR/RTS/BRA never loop in-block (BRA to self would
// be an empty infinite loop the budget would spin on; leave it to the
// dispatcher).
func wm68kDetectLoop(instrs []M68KJITInstr, memory []byte, startPC uint32) bool {
	if len(instrs) == 0 {
		return false
	}
	last := len(instrs) - 1
	op, ok := m68kWasmDecode(&instrs[last], memory, startPC+instrs[last].pcOffset)
	if !ok {
		return false
	}
	if op.class != wm68kClassBcc && op.class != wm68kClassDBcc {
		return false
	}
	return op.target == startPC
}

// m68kWasmCompileBlock translates the given supported prefix into a wasm module
// exporting block(ctx i32) -> (). blockBytes is the total guest byte length of
// the prefix (for the RetPC).
func m68kWasmCompileBlock(instrs []M68KJITInstr, memory []byte, startPC uint32) ([]byte, error) {
	body := &wasmBody{}
	e := &m68kWasmEmitter{b: body, startPC: startPC}
	foldPlan := m68kAnalyseConstFold(instrs, startPC, memory)
	e.loopMode = os.Getenv("M68K_WASM_LOOPS") != "0" &&
		wm68kDetectLoop(instrs, memory, startPC)

	// Within-block CCR liveness elision (milestone 6; M68K_WASM_CCR_LIVENESS=0
	// disables). The shared frontend analysis marks a producer dead only when
	// every CCR bit it writes is overwritten before any consumer, bail-capable
	// instruction or block exit. In loop mode, memory-store instructions gain
	// an extra observation point (the mid-loop SMC exit publishes the CCR), so
	// their own production is pinned live there.
	var ccrLive JITFlagLiveness
	if os.Getenv("M68K_WASM_CCR_LIVENESS") != "0" {
		ccrLive = m68kCCRLiveness(instrs)
	}

	e.prologue()
	if e.loopMode {
		body.i32Const(wm68kLoopBudget)
		body.localSet(wm68kLBudget)
		body.loop()
	}

	var blockBytes uint32
	terminated := false
	for i := range instrs {
		instrPC := startPC + instrs[i].pcOffset
		e.curPC = instrPC
		e.curRetired = i
		e.curLen = uint32(instrs[i].length)
		if instrs[i].fusedFlag&m68kFusedJSRLeafCall != 0 {
			e.emitFusedJSRPush(instrPC + uint32(instrs[i].length))
			if end := instrs[i].pcOffset + uint32(instrs[i].length); end > blockBytes {
				blockBytes = end
			}
			continue
		}
		if instrs[i].fusedFlag&m68kFusedRTSLeafReturn != 0 {
			e.curPC = m68kInstrBailPC(startPC, &instrs[i])
			e.emitFusedRTSPop()
			continue
		}
		op, ok := m68kWasmDecode(&instrs[i], memory, instrPC)
		if !ok {
			return nil, errM68KWasmUnsupported
		}
		e.flagsDead = ccrLive != nil && !ccrLive[i] &&
			!(e.loopMode && wm68kInstrStores(op))
		if end := instrs[i].pcOffset + uint32(instrs[i].length); end > blockBytes {
			blockBytes = end
		}
		if foldPlan != nil && i < len(foldPlan) && foldPlan[i].folded {
			e.emitConstFold(foldPlan[i])
			continue
		}
		if wm68kIsTerminator(op.class) {
			if i != len(instrs)-1 {
				return nil, errM68KWasmUnsupported // terminator must be last
			}
			if e.loopMode {
				e.emitLoopTerminator(op, i+1)
			} else {
				e.emitTerminator(op, i+1)
			}
			terminated = true
			break
		}
		e.emit(op)
	}
	if e.loopMode {
		body.end() // loop; every path inside returned or branched to the head
	}
	if !terminated {
		e.epilogue(startPC+blockBytes, len(instrs))
	}
	body.end()

	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)
	fn := m.addFunc(typ, wm68kLocalDecl(), body.code)
	m.exportFunc("block", fn)
	return m.build(), nil
}

type m68kWasmError string

func (e m68kWasmError) Error() string { return string(e) }

const errM68KWasmUnsupported = m68kWasmError("m68k wasm: unsupported instruction in prefix")

// ---------------------------------------------------------------------------
// Branch condition evaluation and block terminators
// ---------------------------------------------------------------------------

// emitPushStackGuard bails when a long push would wrap A7 below zero
// (Push32's oldSP < 4 underflow wrap) or drop the decremented A7 below
// cpu.stackLowerBound (the interpreter's stack floor, a bus error). A zero
// StackLowerBoundPtr (test contexts that predate the field) skips the floor
// check only. Runs before any state mutation, so the interpreter re-executes
// the whole instruction and raises the architectural exception.
func (e *m68kWasmEmitter) emitPushStackGuard() {
	b := e.b
	// Wrap: A7 < 4.
	e.pushAReg(7)
	b.i32Const(4)
	b.op(wasmOpI32LtU)
	b.ifVoid()
	e.emitBail()
	b.end()
	// Floor: A7-4 < *stackLowerBound.
	e.ctxLoadI32(m68kCtxOffStackLowerBoundPtr)
	b.localSet(wm68kLT0)
	b.localGet(wm68kLT0)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	b.elseBranch()
	e.pushAReg(7)
	b.i32Const(4)
	b.op(wasmOpI32Sub)
	b.localGet(wm68kLT0)
	b.i32Load(2, 0)
	b.op(wasmOpI32LtU)
	b.ifVoid()
	e.emitBail()
	b.end()
	b.end()
}

// emitPopStackGuard bails when A7 >= *stackUpperBound before a long pop
// (Pop32's stack-ceiling bus error). A zero pointer skips the check.
func (e *m68kWasmEmitter) emitPopStackGuard() {
	b := e.b
	e.ctxLoadI32(m68kCtxOffStackUpperBoundPtr)
	b.localSet(wm68kLT0)
	b.localGet(wm68kLT0)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	b.elseBranch()
	e.pushAReg(7)
	b.localGet(wm68kLT0)
	b.i32Load(2, 0)
	b.op(wasmOpI32GeU)
	b.ifVoid()
	e.emitBail()
	b.end()
	b.end()
}

func (e *m68kWasmEmitter) emitFusedJSRPush(returnPC uint32) {
	e.emitPushStackGuard()
	e.emitEAAddr(wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemPre, reg: 7}, 4)
	e.b.i32Const(int32(returnPC))
	e.b.localSet(wm68kLMB)
	e.storeMemValueBE(4, wm68kLMB)
}

func (e *m68kWasmEmitter) emitFusedRTSPop() {
	e.emitPopStackGuard()
	e.emitEAAddr(wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemPost, reg: 7}, 4)
	e.pushMemValueBE(4)
	e.b.localSet(wm68kLT0)
}

func (e *m68kWasmEmitter) emitConstFold(f m68kFoldEntry) {
	b := e.b
	if f.setsReg {
		b.localGet(wm68kLDBase)
		b.i32Const(int32(f.value))
		b.i32Store(2, uint32(f.reg)*4)
	}
	if f.ccrMask != 0 && !e.flagsDead {
		b.localGet(wm68kLCCR)
		b.i32Const(int32(uint8(^f.ccrMask) & 0x1F))
		b.op(wasmOpI32And)
		b.i32Const(int32(f.ccrVal & f.ccrMask))
		b.op(wasmOpI32Or)
		b.localSet(wm68kLCCR)
	}
	m68kFoldedConstEmits.Add(1)
}

// pushCCRBit pushes bit `bit` of the modelled CCR (0 or 1).
func (e *m68kWasmEmitter) pushCCRBit(bit int) {
	b := e.b
	b.localGet(wm68kLCCR)
	b.i32Const(int32(bit))
	b.op(wasmOpI32ShrU)
	b.i32Const(1)
	b.op(wasmOpI32And)
}

// emitCond pushes a 0/1 boolean for M68K condition code `cond` from the CCR.
func (e *m68kWasmEmitter) emitCond(cond int) {
	b := e.b
	switch cond {
	case 0: // T
		b.i32Const(1)
	case 1: // F
		b.i32Const(0)
	case 2: // HI = !C & !Z
		e.pushCCRBit(wm68kBitC)
		b.op(wasmOpI32Eqz)
		e.pushCCRBit(wm68kBitZ)
		b.op(wasmOpI32Eqz)
		b.op(wasmOpI32And)
	case 3: // LS = C | Z
		e.pushCCRBit(wm68kBitC)
		e.pushCCRBit(wm68kBitZ)
		b.op(wasmOpI32Or)
	case 4: // CC/HS = !C
		e.pushCCRBit(wm68kBitC)
		b.op(wasmOpI32Eqz)
	case 5: // CS/LO = C
		e.pushCCRBit(wm68kBitC)
	case 6: // NE = !Z
		e.pushCCRBit(wm68kBitZ)
		b.op(wasmOpI32Eqz)
	case 7: // EQ = Z
		e.pushCCRBit(wm68kBitZ)
	case 8: // VC = !V
		e.pushCCRBit(wm68kBitV)
		b.op(wasmOpI32Eqz)
	case 9: // VS = V
		e.pushCCRBit(wm68kBitV)
	case 10: // PL = !N
		e.pushCCRBit(wm68kBitN)
		b.op(wasmOpI32Eqz)
	case 11: // MI = N
		e.pushCCRBit(wm68kBitN)
	case 12: // GE = N == V
		e.pushCCRBit(wm68kBitN)
		e.pushCCRBit(wm68kBitV)
		b.op(wasmOpI32Eq)
	case 13: // LT = N != V
		e.pushCCRBit(wm68kBitN)
		e.pushCCRBit(wm68kBitV)
		b.op(wasmOpI32Ne)
	case 14: // GT = !Z & (N == V)
		e.pushCCRBit(wm68kBitZ)
		b.op(wasmOpI32Eqz)
		e.pushCCRBit(wm68kBitN)
		e.pushCCRBit(wm68kBitV)
		b.op(wasmOpI32Eq)
		b.op(wasmOpI32And)
	case 15: // LE = Z | (N != V)
		e.pushCCRBit(wm68kBitZ)
		e.pushCCRBit(wm68kBitN)
		e.pushCCRBit(wm68kBitV)
		b.op(wasmOpI32Ne)
		b.op(wasmOpI32Or)
	}
}

// setRetPCConst / setRetPCLocal publish the resume PC to the dispatcher.
func (e *m68kWasmEmitter) setRetPCConst(pc uint32) { e.storeCtxConst(m68kCtxOffRetPC, int32(pc)) }
func (e *m68kWasmEmitter) setRetPCLocal(local uint32) {
	e.storeCtxLocal(m68kCtxOffRetPC, local)
}

// emitBranchEnd flushes the CCR, records the retired count and returns. The
// resume PC must already be stored by the branch emitter.
func (e *m68kWasmEmitter) emitBranchEnd(retCount int) {
	e.flushCCR()
	e.storeRetCountPlus(retCount)
	e.b.op(wasmOpReturn)
}

// emitTerminator lowers a block-ending control-flow instruction. retCount is
// the number of instructions retired including this one.
func (e *m68kWasmEmitter) emitTerminator(op wm68kOp, retCount int) {
	b := e.b
	switch op.class {
	case wm68kClassBRA:
		e.setRetPCConst(op.target)
	case wm68kClassBcc:
		e.emitCond(op.cond)
		b.ifVoid()
		e.setRetPCConst(op.target)
		b.elseBranch()
		e.setRetPCConst(op.fallPC)
		b.end()
	case wm68kClassDBcc:
		e.emitCond(op.cond)
		b.ifVoid()
		// Condition true: no decrement, fall through.
		e.setRetPCConst(op.fallPC)
		b.elseBranch()
		// Decrement Dn low word, preserving the high word.
		e.pushDReg(op.dst.reg)
		b.localSet(wm68kLT0) // old Dn
		b.localGet(wm68kLDBase)
		// new = (old & 0xFFFF0000) | ((old-1) & 0xFFFF)
		b.localGet(wm68kLT0)
		b.i32Const(-0x10000) // 0xFFFF0000 high-word mask
		b.op(wasmOpI32And)
		b.localGet(wm68kLT0)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Or)
		b.i32Store(2, uint32(op.dst.reg)*4)
		// counter = (old-1) & 0xFFFF; branch if counter != 0xFFFF.
		b.localGet(wm68kLT0)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32And)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32Ne)
		b.ifVoid()
		e.setRetPCConst(op.target)
		b.elseBranch()
		e.setRetPCConst(op.fallPC)
		b.end()
		b.end() // cond if
	case wm68kClassBSR:
		// Push the return address (fallPC) via a guarded -(A7), then jump.
		e.emitPushStackGuard()
		e.emitEAAddr(wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemPre, reg: 7}, 4)
		b.i32Const(int32(op.fallPC))
		b.localSet(wm68kLMB)
		e.storeMemValueBE(4, wm68kLMB)
		e.setRetPCConst(op.target)
	case wm68kClassJMP:
		e.pushJMPTarget(op.src)
		b.localSet(wm68kLT0)
		e.setRetPCLocal(wm68kLT0)
	case wm68kClassJSR:
		// Compute the target address first (ExecJsr ordering), then push return.
		// The target is held in the guard-safe LMV local because emitEAAddr's
		// bounds probe clobbers the T-scratch registers.
		e.pushJMPTarget(op.src)
		b.localSet(wm68kLMV) // target
		e.emitPushStackGuard()
		e.emitEAAddr(wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemPre, reg: 7}, 4)
		b.i32Const(int32(op.fallPC))
		b.localSet(wm68kLMB)
		e.storeMemValueBE(4, wm68kLMB)
		e.setRetPCLocal(wm68kLMV)
	case wm68kClassRTS:
		// Pop the return address via a guarded (A7)+.
		e.emitPopStackGuard()
		e.emitEAAddr(wm68kOperand{kind: wm68kOpMem, memKind: wm68kMemPost, reg: 7}, 4)
		e.pushMemValueBE(4)
		b.localSet(wm68kLT2)
		e.setRetPCLocal(wm68kLT2)
	}
	e.emitBranchEnd(retCount)
}

// emitLoopTerminator lowers the Bcc/DBcc terminator of a structured in-block
// self-loop. The taken path accumulates this iteration's retired count into
// LRet, decrements the iteration budget and branches back to the loop head;
// when the budget is exhausted it exits with RetPC = the loop head so the
// dispatcher re-enters after its interrupt/yield boundary work. The not-taken
// path exits exactly like the straight-line terminator.
func (e *m68kWasmEmitter) emitLoopTerminator(op wm68kOp, retCount int) {
	b := e.b
	// takenBackEdge emits: LRet += retCount; if --LBudget != 0 branch to the
	// loop head at label depth `loopDepth`; otherwise publish the head PC and
	// return.
	takenBackEdge := func(loopDepth uint32) {
		b.localGet(wm68kLRet)
		b.i32Const(int32(retCount))
		b.op(wasmOpI32Add)
		b.localSet(wm68kLRet)
		b.localGet(wm68kLBudget)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.localSet(wm68kLBudget)
		b.localGet(wm68kLBudget)
		b.brIf(loopDepth)
		e.setRetPCConst(op.target)
		e.emitBranchEnd(0) // LRet already includes this iteration
	}
	switch op.class {
	case wm68kClassBcc:
		e.emitCond(op.cond)
		b.ifVoid()
		takenBackEdge(1) // labels: if=0, loop=1
		b.elseBranch()
		e.setRetPCConst(op.fallPC)
		e.emitBranchEnd(retCount)
		b.end()
	case wm68kClassDBcc:
		e.emitCond(op.cond)
		b.ifVoid()
		// Condition true: no decrement, fall through out of the loop.
		e.setRetPCConst(op.fallPC)
		e.emitBranchEnd(retCount)
		b.elseBranch()
		// Decrement Dn low word, preserving the high word.
		e.pushDReg(op.dst.reg)
		b.localSet(wm68kLT0)
		b.localGet(wm68kLDBase)
		b.localGet(wm68kLT0)
		b.i32Const(-0x10000)
		b.op(wasmOpI32And)
		b.localGet(wm68kLT0)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Or)
		b.i32Store(2, uint32(op.dst.reg)*4)
		// Branch back while counter != -1.
		b.localGet(wm68kLT0)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32And)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32Ne)
		b.ifVoid()
		takenBackEdge(2) // labels: inner if=0, outer if=1, loop=2
		b.elseBranch()
		e.setRetPCConst(op.fallPC)
		e.emitBranchEnd(retCount)
		b.end()
		b.end()
	}
}

// pushJMPTarget pushes the control-address of a JMP/JSR EA (the address itself,
// not memory content). Only (An)/(d16,An)/abs/(d16,PC) reach here.
func (e *m68kWasmEmitter) pushJMPTarget(o wm68kOperand) {
	b := e.b
	switch o.memKind {
	case wm68kMemInd:
		e.pushAReg(o.reg)
	case wm68kMemDisp:
		e.pushAReg(o.reg)
		b.i32Const(o.disp)
		b.op(wasmOpI32Add)
	case wm68kMemAbs:
		b.i32Const(int32(o.abs))
	}
}

// ---------------------------------------------------------------------------
// 68881 clean-mapping FPU subset (wasm f64)
// ---------------------------------------------------------------------------

// wm68kFPUEmittable reports whether a decoded native FPU op is lowered by the
// wasm backend. The clean-mapping subset is FMOVE/FADD/FSUB/FMUL/FDIV/FABS/
// FNEG/FSQRT/FCMP/FTST. FSGLMUL/FSGLDIV (float32 operand rounding), FINT/FINTRZ
// (FPCR rounding mode) and every transcendental stay on the interpreter's
// 68881, matching the split the amd64/arm64 native paths draw.
func wm68kFPUEmittable(op m68kFPUNativeOp) bool {
	switch op {
	case m68kFPUNativeFMOVE, m68kFPUNativeFADD, m68kFPUNativeFSUB,
		m68kFPUNativeFMUL, m68kFPUNativeFDIV, m68kFPUNativeFABS,
		m68kFPUNativeFNEG, m68kFPUNativeFSQRT, m68kFPUNativeFCMP,
		m68kFPUNativeFTST,
		// Milestone 6: single-precision arithmetic maps to wasm f32 ops, and
		// wasm has the full f64 rounding set (nearest/trunc/floor/ceil) for
		// FINT's FPCR-selected mode and FINTRZ's truncation.
		m68kFPUNativeFSGLMUL, m68kFPUNativeFSGLDIV,
		m68kFPUNativeFINT, m68kFPUNativeFINTRZ:
		return true
	}
	return false
}

// fpBase loads the FP register file base (linear offset) into T1.
func (e *m68kWasmEmitter) fpBase() {
	e.ctxLoadI32(m68kCtxOffFPRegsPtr)
	e.b.localSet(wm68kLT1)
}

// pushFPReg pushes fp[reg] (f64). fpBase must have set T1.
func (e *m68kWasmEmitter) pushFPReg(reg int) {
	e.b.localGet(wm68kLT1)
	e.b.f64Load(3, uint32(reg)*8)
}

// applyFPPrecision rounds the f64 on the stack to the result precision: single
// round-trips through f32; extended and double leave the double verbatim.
func (e *m68kWasmEmitter) applyFPPrecision(prec int) {
	if prec == m68kFPURoundSingle {
		e.b.op(wasmOpF32DemoteF64)
		e.b.op(wasmOpF64PromoteF32)
	}
}

// emitFPU lowers one 68881 register-to-register op and sets the FPSR condition
// codes eagerly, plus FPIAR to the instruction PC (matching the interpreter's
// general-data-op behaviour).
func (e *m68kWasmEmitter) emitFPU(op wm68kOp) {
	b := e.b
	e.fpBase()
	// FPIAR = instruction PC.
	e.ctxLoadI32(m68kCtxOffFPIARPtr)
	b.localSet(wm68kLT0)
	b.localGet(wm68kLT0)
	b.i32Const(int32(e.curPC))
	b.i32Store(2, 0)

	if op.fpOp == m68kFPUNativeFCMP {
		e.emitFCMP(op)
		return
	}

	// Compute the result value (f64) on the stack.
	switch op.fpOp {
	case m68kFPUNativeFMOVE:
		e.pushFPReg(op.fpSrc)
	case m68kFPUNativeFABS:
		e.pushFPReg(op.fpSrc)
		b.op(wasmOpF64Abs)
	case m68kFPUNativeFNEG:
		e.pushFPReg(op.fpSrc)
		b.op(wasmOpF64Neg)
	case m68kFPUNativeFSQRT:
		e.pushFPReg(op.fpSrc)
		b.op(wasmOpF64Sqrt)
	case m68kFPUNativeFTST:
		e.pushFPReg(op.fpSrc) // test the source; no store
	case m68kFPUNativeFADD:
		e.pushFPReg(op.fpDst)
		e.pushFPReg(op.fpSrc)
		b.op(wasmOpF64Add)
	case m68kFPUNativeFSUB:
		e.pushFPReg(op.fpDst)
		e.pushFPReg(op.fpSrc)
		b.op(wasmOpF64Sub)
	case m68kFPUNativeFMUL:
		e.pushFPReg(op.fpDst)
		e.pushFPReg(op.fpSrc)
		b.op(wasmOpF64Mul)
	case m68kFPUNativeFDIV:
		e.pushFPReg(op.fpDst)
		e.pushFPReg(op.fpSrc)
		b.op(wasmOpF64Div)
	case m68kFPUNativeFSGLMUL, m68kFPUNativeFSGLDIV:
		// Interpreter parity: both operands demoted to float32, the operation
		// performed in float32, then widened (FSGLMUL/FSGLDIV in fpu_m68881.go).
		e.pushFPReg(op.fpDst)
		b.op(wasmOpF32DemoteF64)
		e.pushFPReg(op.fpSrc)
		b.op(wasmOpF32DemoteF64)
		if op.fpOp == m68kFPUNativeFSGLMUL {
			b.op(wasmOpF32Mul)
		} else {
			b.op(wasmOpF32Div)
		}
		b.op(wasmOpF64PromoteF32)
	case m68kFPUNativeFINTRZ:
		e.pushFPReg(op.fpSrc)
		b.op(wasmOpF64Trnc)
	case m68kFPUNativeFINT:
		// Round per the FPCR rounding mode (bits 5:4): 0 nearest-even,
		// 1 toward zero, 2 toward minus infinity, 3 toward plus infinity.
		e.pushFPReg(op.fpSrc)
		b.localSet(wm68kLF1)
		e.ctxLoadI32(m68kCtxOffFPCRPtr)
		b.localSet(wm68kLT0)
		b.localGet(wm68kLT0)
		b.i32Load(2, 0)
		b.i32Const(4)
		b.op(wasmOpI32ShrU)
		b.i32Const(3)
		b.op(wasmOpI32And)
		b.localSet(wm68kLT2) // rounding mode
		b.localGet(wm68kLT2)
		b.i32Const(1)
		b.op(wasmOpI32Eq)
		b.ifVoid()
		b.localGet(wm68kLF1)
		b.op(wasmOpF64Trnc)
		b.localSet(wm68kLF1)
		b.elseBranch()
		b.localGet(wm68kLT2)
		b.i32Const(2)
		b.op(wasmOpI32Eq)
		b.ifVoid()
		b.localGet(wm68kLF1)
		b.op(wasmOpF64Flr)
		b.localSet(wm68kLF1)
		b.elseBranch()
		b.localGet(wm68kLT2)
		b.i32Const(3)
		b.op(wasmOpI32Eq)
		b.ifVoid()
		b.localGet(wm68kLF1)
		b.op(wasmOpF64Ceil)
		b.localSet(wm68kLF1)
		b.elseBranch()
		b.localGet(wm68kLF1)
		b.op(wasmOpF64Nrst)
		b.localSet(wm68kLF1)
		b.end()
		b.end()
		b.end()
		b.localGet(wm68kLF1)
	}
	if op.fpOp != m68kFPUNativeFTST {
		e.applyFPPrecision(op.fpPrec)
	}
	// Capture the value, store it (except FTST) and set the CC from its bits.
	b.localSet(wm68kLF0)
	if op.fpOp != m68kFPUNativeFTST {
		b.localGet(wm68kLT1)
		b.localGet(wm68kLF0)
		b.f64Store(3, uint32(op.fpDst)*8)
	}
	b.localGet(wm68kLF0)
	b.op(wasmOpI64ReinterpretF64)
	b.localSet(wm68kLQ0)
	e.emitFPUSetCCFromBits()
}

// emitFPUSetCCFromBits computes the FPSR N/Z/I/NAN bits for the f64 result whose
// raw bits are in Q0, exactly mirroring m68kFPUConditionBits, and merges them
// into cpu.FPSR (preserving the non-CC bits).
func (e *m68kWasmEmitter) emitFPUSetCCFromBits() {
	b := e.b
	const maxExp = int64(0x7FF0000000000000)
	const fracMask = int64(0x000FFFFFFFFFFFFF)
	const absMask = int64(0x7FFFFFFFFFFFFFFF)
	// sgn -> T0
	b.localGet(wm68kLQ0)
	b.i64Const(63)
	b.op(wasmOpI64ShrU)
	b.op(wasmOpI32WrapI64)
	b.localSet(wm68kLT0)
	// e_infnan -> T3
	b.localGet(wm68kLQ0)
	b.i64Const(maxExp)
	b.op(wasmOpI64And)
	b.i64Const(maxExp)
	b.op(wasmOpI64Eq)
	b.localSet(wm68kLT3)
	// fracZero -> T4
	b.localGet(wm68kLQ0)
	b.i64Const(fracMask)
	b.op(wasmOpI64And)
	b.op(wasmOpI64Eqz)
	b.localSet(wm68kLT4)
	// zeroFlag -> T2 (exp|frac == 0)
	b.localGet(wm68kLQ0)
	b.i64Const(absMask)
	b.op(wasmOpI64And)
	b.op(wasmOpI64Eqz)
	b.localSet(wm68kLT2)
	// nan = e_infnan & !fracZero  ; inf = e_infnan & fracZero
	// z = zeroFlag ; n = sgn & !nan & !z
	// cc on stack = (nan<<24)|(inf<<25)|(z<<26)|(n<<27)
	e.pushFPUFlag_nan()
	b.i32Const(24)
	b.op(wasmOpI32Shl)
	// inf
	b.localGet(wm68kLT3)
	b.localGet(wm68kLT4)
	b.op(wasmOpI32And)
	b.i32Const(25)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	// z
	b.localGet(wm68kLT2)
	b.i32Const(26)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	// n = sgn & !nan & !z
	b.localGet(wm68kLT0)
	e.pushFPUFlag_nan()
	b.op(wasmOpI32Eqz)
	b.op(wasmOpI32And)
	b.localGet(wm68kLT2)
	b.op(wasmOpI32Eqz)
	b.op(wasmOpI32And)
	b.i32Const(27)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(wm68kLT2) // cc (reuses T2, zeroFlag no longer needed)
	e.mergeFPSR(wm68kLT2)
}

// pushFPUFlag_nan pushes (e_infnan & !fracZero) using T3/T4.
func (e *m68kWasmEmitter) pushFPUFlag_nan() {
	b := e.b
	b.localGet(wm68kLT3)
	b.localGet(wm68kLT4)
	b.op(wasmOpI32Eqz)
	b.op(wasmOpI32And)
}

// mergeFPSR sets cpu.FPSR = (FPSR & ~fpuCCMask) | cc, where cc is in ccLocal.
func (e *m68kWasmEmitter) mergeFPSR(ccLocal uint32) {
	b := e.b
	e.ctxLoadI32(m68kCtxOffFPSRPtr)
	b.localSet(wm68kLT3) // FPSR base
	b.localGet(wm68kLT3)
	b.localGet(wm68kLT3)
	b.i32Load(2, 0)
	b.i32Const(^int32(0x0F000000)) // ~fpuCCMask
	b.op(wasmOpI32And)
	b.localGet(ccLocal)
	b.op(wasmOpI32Or)
	b.i32Store(2, 0)
}

// emitFCMP computes the FPSR CC for FCMP FPsrc,FPdst: NaN operands set NAN;
// otherwise the sign of (dst - src) sets Z (==0) or N (<0), no I flag. Mirrors
// (*M68881FPU).FCMP.
func (e *m68kWasmEmitter) emitFCMP(op wm68kOp) {
	b := e.b
	e.pushFPReg(op.fpDst)
	b.localSet(wm68kLF0) // a
	e.pushFPReg(op.fpSrc)
	b.localSet(wm68kLF1) // b
	// nan = (a!=a) | (b!=b) -> T0
	b.localGet(wm68kLF0)
	b.localGet(wm68kLF0)
	b.op(wasmOpF64Ne)
	b.localGet(wm68kLF1)
	b.localGet(wm68kLF1)
	b.op(wasmOpF64Ne)
	b.op(wasmOpI32Or)
	b.localSet(wm68kLT0)
	// diff = a - b -> F2
	b.localGet(wm68kLF0)
	b.localGet(wm68kLF1)
	b.op(wasmOpF64Sub)
	b.localSet(wm68kLF2)
	// zf = diff == 0 -> T3 ; nf = diff < 0 -> T4
	b.localGet(wm68kLF2)
	b.f64Const(0)
	b.op(wasmOpF64Eq)
	b.localSet(wm68kLT3)
	b.localGet(wm68kLF2)
	b.f64Const(0)
	b.op(wasmOpF64Lt)
	b.localSet(wm68kLT4)
	// cc = (nan<<24) | (((1-nan)&zf)<<26) | (((1-nan)&!zf&nf)<<27)
	b.localGet(wm68kLT0)
	b.i32Const(24)
	b.op(wasmOpI32Shl)
	// z
	b.localGet(wm68kLT0)
	b.op(wasmOpI32Eqz)
	b.localGet(wm68kLT3)
	b.op(wasmOpI32And)
	b.i32Const(26)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	// n
	b.localGet(wm68kLT0)
	b.op(wasmOpI32Eqz)
	b.localGet(wm68kLT3)
	b.op(wasmOpI32Eqz)
	b.op(wasmOpI32And)
	b.localGet(wm68kLT4)
	b.op(wasmOpI32And)
	b.i32Const(27)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(wm68kLT2)
	e.mergeFPSR(wm68kLT2)
}
