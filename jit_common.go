// jit_common.go - JIT compiler infrastructure: CodeBuffer, block scanner, code cache

package main

import (
	"encoding/binary"
	"unsafe"
)

// fp32AbsInf is |+Inf| in IEEE-754 binary32. Over sign-stripped bits:
// x is infinite iff abs == fp32AbsInf, NaN iff abs > fp32AbsInf, and both
// (that is, "special") iff abs >= fp32AbsInf. Zero iff abs == 0.
//
// Shared by the amd64 and arm64 FP32 sticky-flag classifiers, which must agree
// with each other and with the interpreter on these boundaries.
const fp32AbsInf = 0x7F800000

// ie64AccessBytes returns the byte count for an IE64 size encoding
// (B=1, W=2, L=4, Q=8). Used by size-aware high-address bail checks on
// both AMD64 and ARM64.
func ie64AccessBytes(size byte) uint32 {
	switch size {
	case IE64_SIZE_B:
		return 1
	case IE64_SIZE_W:
		return 2
	case IE64_SIZE_L:
		return 4
	case IE64_SIZE_Q:
		return 8
	}
	return 8
}

// ===========================================================================
// JITContext — Bridge between Go and JIT-compiled native code
// ===========================================================================

// JITContext is passed to every JIT-compiled block as its sole argument.
// On ARM64 it arrives in X0; on x86-64 in RDI.
type JITContext struct {
	RegsPtr        uintptr // 0:  &cpu.regs[0]
	MemPtr         uintptr // 8:  &cpu.memory[0]
	MemSize        uint32  // 16: len(cpu.memory)
	IOStart        uint32  // 20: IO_REGION_START
	PCPtr          uintptr // 24: &cpu.PC
	LoadMemFn      uintptr // 32: Go helper for I/O reads (future)
	StoreMemFn     uintptr // 40: Go helper for I/O writes (future)
	CpuPtr         uintptr // 48: &cpu for Go callouts
	NeedInval      uint32  // 56: set to 1 when code cache needs invalidation
	NeedIOFallback uint32  // 60: set to 1 when LOAD/STORE hits I/O page
	IOBitmapPtr    uintptr // 64: &cpu.bus.ioPageBitmap[0]
	FPUPtr         uintptr // 72: &cpu.FPU (pointer to IE64FPU struct)
	ChainBudget    uint32  // 80: blocks remaining before returning to Go
	ChainCount     uint32  // 84: accumulated instruction count during chaining
	// Phase 3: RTS cache PC fields widened to uint64. Cache compares are
	// 64-bit on AMD64; the high-bit bypass from Phase 2 is no longer
	// required because the cache PC field now matches the full popped
	// return address.
	RTSCache0PC   uint64  // 88:  MRU entry 0 - return PC
	RTSCache0Addr uintptr // 96:  MRU entry 0 - chain entry address
	RTSCache1PC   uint64  // 104: MRU entry 1 - return PC
	RTSCache1Addr uintptr // 112: MRU entry 1 - chain entry address
	RTSCache2PC   uint64  // 120: MRU entry 2 - return PC
	RTSCache2Addr uintptr // 128: MRU entry 2 - chain entry address
	RTSCache3PC   uint64  // 136: MRU entry 3 - return PC
	RTSCache3Addr uintptr // 144: MRU entry 3 - chain entry address
	// Phase 2: explicit 64-bit PC return channel.
	//
	// Replaces the legacy regs[0]-packed (lowerPC | upperCount) format with
	// dedicated fields so that block exits can return a full 64-bit PC and
	// the retired-instruction count without colliding in a single host
	// register. Chain accumulation now happens via ADD to ChainCount at the
	// source block's chain exit, so the chain entry no longer needs to
	// extract count from R15.
	RetPC    uint64 // 152: next PC after block exit (full 64-bit)
	RetCount uint32 // 160: retired instruction count for the exiting block
	// Phase 4: refreshed by the Go dispatcher before every callNative. The
	// native emitters will start checking this in Phase 5 to route MMU-on
	// memory/stack ops through a helper exit; it lives here now so the
	// field offset is stable before any emitter wires it up.
	MMUEnabled uint32 // 164: 1 when MMU translation is active for the next block
	// Phase 5: helper-exit protocol. Native emitted code writes these
	// fields and returns when it hits a high address, MMU-on operation,
	// or unsupported addressing mode. The Go-side dispatcher in
	// ExecuteJIT inspects NeedHelper, performs the equivalent semantic
	// operation via the interpreter helpers (loadMem/storeMem/
	// mmuStackRead/mmuStackWrite), advances PC, and re-enters JIT.
	NeedHelper uint32 // 168: helper opcode (HELPER_* constants); 0 = no helper request
	HelperSize uint32 // 172: IE64_SIZE_B/W/L/Q for memory ops
	HelperRd   uint32 // 176: destination/source register or FP register index
	HelperAddr uint64 // 184: virtual address (data ops) or call target (control flow)
	HelperVal  uint64 // 192: value to store/push (input only); LOAD/POP results go to an integer reg via setReg and FLOAD/DLOAD results go to the FPU via FP setters, never returned through this field
	HelperPC   uint64 // 200: PC of the requesting instruction for trapFault.faultPC
	LiveSP     uint64 // 208: SP flushed from host register before helper exit
	// A1a: optional helper continuation. AMD64 helper exits for
	// fall-through operations can publish a native continuation entry
	// after the bailing instruction. The dispatcher uses it only when the
	// helper completed cleanly and no interrupt, invalidation, or MMU
	// address-space change occurred.
	ResumeAddr       uintptr // 216: native continuation entry, execution-view address
	ResumePC         uint64  // 224: guest PC expected after helper completion
	ResumePTBR       uint64  // 232: PTBR captured by dispatcher before native entry
	ResumeCountBase  uint32  // 240: instruction count already retired before continuation
	ResumeMMUEnabled uint32  // 244: MMUEnabled value captured by helper exit
	ResumeValid      uint32  // 248: non-zero when ResumeAddr/ResumePC are usable
	// A1b: small native-probed MMU translation cache for dense-RAM data
	// accesses. Key prefixes are refreshed by the dispatcher from CPU
	// privilege state; helper dispatch fills entries only after a clean
	// interpreter translation.
	MicroTLBReadPrefix  uint64    // 256: valid/access/mode prefix for LOAD probes
	MicroTLBWritePrefix uint64    // 264: valid/access/mode prefix for STORE probes
	MicroTLBKeys        [4]uint64 // 272: valid-prefixed VPN keys
	MicroTLBPhys        [4]uint64 // 304: physical page bases
	CodePageBitmapPtr   uintptr   // 336: &cpu.jitCodePageBitmap[0], 256-byte pages
	InvalAddr           uint64    // 344: self-modifying write address
	InvalSize           uint32    // 352: self-modifying write size in bytes
	CodePageBitmapLen   uint32    // 356: len(cpu.jitCodePageBitmap), bounds native SMC probes
	CodeHighStartPage   uint64    // 360: first compiled code page outside CodePageBitmapLen, 0 if none
	CodeHighEndPage     uint64    // 368: last compiled code page outside CodePageBitmapLen, 0 if none
	PhysCodeBitmapPtr   uintptr   // 376: &cpu.jitPhysCodePageBitmap[0], physical 256-byte pages
	PhysCodeBitmapLen   uint32    // 384: len(cpu.jitPhysCodePageBitmap), bounds native MMU SMC probes
	// CodePageSpansPtr points at two bytes per 256-byte code page holding the
	// inclusive [min, max] byte offsets of compiled code within that page
	// (0xFF/0x00 when the page holds none). The wasm store probe consults it
	// so a store into data that merely shares a page with compiled code never
	// exits; backends that do not maintain spans leave the pointer zero and
	// must not emit span-refined probes.
	CodePageSpansPtr uintptr // 392: &cpu.jitCodePageSpans[0], 2 bytes per page
}

// HELPER_* opcodes for the JITContext.NeedHelper field. Phase 5: native
// emitted code sets NeedHelper to one of these values and exits when it
// cannot service a memory/stack/control-flow operation locally (MMU on,
// high physical address, etc.). The Go-side dispatcher then performs
// the equivalent operation via the interpreter helpers and re-enters
// the JIT loop.
const (
	HELPER_NONE    uint32 = 0
	HELPER_LOAD    uint32 = 1  // integer load → setReg(HelperRd, loadMem(HelperAddr, HelperSize))
	HELPER_STORE   uint32 = 2  // integer store → storeMem(HelperAddr, HelperVal, HelperSize)
	HELPER_FLOAD   uint32 = 3  // 32-bit FP load → FPU.FPRegs[HelperRd] = loadMem(HelperAddr, L)
	HELPER_FSTORE  uint32 = 4  // 32-bit FP store → storeMem(HelperAddr, FPU.FPRegs[HelperRd], L)
	HELPER_DLOAD   uint32 = 5  // 64-bit FP load via storeFP64Pair
	HELPER_DSTORE  uint32 = 6  // 64-bit FP store via loadFP64Pair
	HELPER_PUSH    uint32 = 7  // mmuStackWrite(SP-8, HelperVal); SP -= 8
	HELPER_POP     uint32 = 8  // val = mmuStackRead(SP); SP += 8 → setReg(HelperRd, val)
	HELPER_JSR     uint32 = 9  // push retAddr (HelperVal); PC = HelperAddr (call target)
	HELPER_RTS     uint32 = 10 // pop val; PC = val
	HELPER_JSR_IND uint32 = 11 // push retAddr (HelperVal); PC = HelperAddr (rs + imm32)
	HELPER_DTRANS  uint32 = 12 // FP64 transcendental; HelperSize carries the IE64 opcode
)

// JITContext field offsets (must match struct layout above)
const (
	jitCtxOffRegsPtr             = 0
	jitCtxOffMemPtr              = 8
	jitCtxOffMemSize             = 16
	jitCtxOffIOStart             = 20
	jitCtxOffPCPtr               = 24
	jitCtxOffLoadMemFn           = 32
	jitCtxOffStoreMemFn          = 40
	jitCtxOffCpuPtr              = 48
	jitCtxOffNeedInval           = 56
	jitCtxOffNeedIOFallback      = 60
	jitCtxOffIOBitmapPtr         = 64
	jitCtxOffFPUPtr              = 72
	jitCtxOffChainBudget         = 80
	jitCtxOffChainCount          = 84
	jitCtxOffRTSCache0PC         = 88
	jitCtxOffRTSCache0Addr       = 96
	jitCtxOffRTSCache1PC         = 104
	jitCtxOffRTSCache1Addr       = 112
	jitCtxOffRTSCache2PC         = 120
	jitCtxOffRTSCache2Addr       = 128
	jitCtxOffRTSCache3PC         = 136
	jitCtxOffRTSCache3Addr       = 144
	jitCtxOffRetPC               = 152
	jitCtxOffRetCount            = 160
	jitCtxOffMMUEnabled          = 164
	jitCtxOffNeedHelper          = 168
	jitCtxOffHelperSize          = 172
	jitCtxOffHelperRd            = 176
	jitCtxOffHelperAddr          = 184
	jitCtxOffHelperVal           = 192
	jitCtxOffHelperPC            = 200
	jitCtxOffLiveSP              = 208
	jitCtxOffResumeAddr          = 216
	jitCtxOffResumePC            = 224
	jitCtxOffResumePTBR          = 232
	jitCtxOffResumeCountBase     = 240
	jitCtxOffResumeMMUEnabled    = 244
	jitCtxOffResumeValid         = 248
	jitCtxOffMicroTLBReadPrefix  = 256
	jitCtxOffMicroTLBWritePrefix = 264
	jitCtxOffMicroTLBKeys        = 272
	jitCtxOffMicroTLBPhys        = 304
	jitCtxOffCodePageBitmapPtr   = 336
	jitCtxOffInvalAddr           = 344
	jitCtxOffInvalSize           = 352
	jitCtxOffCodePageBitmapLen   = 356
	jitCtxOffCodeHighStartPage   = 360
	jitCtxOffCodeHighEndPage     = 368
	jitCtxOffPhysCodeBitmapPtr   = 376
	jitCtxOffPhysCodeBitmapLen   = 384
	jitCtxOffCodePageSpansPtr    = 392
	jitCtxMicroTLBEntries        = 4
	jitCtxMicroTLBStride         = 8
)

// ie64ChainBudget is the per-callNative chain dispatch budget (number of
// chained block transitions before falling back to the Go dispatcher for
// interrupt/timer polls). Aligned with the M68K backend's value.
const ie64ChainBudget = 256

// jitAvailable is set to true at init time on platforms that support JIT.
var jitAvailable bool

func newJITContext(cpu *CPU64) *JITContext {
	ctx := &JITContext{
		RegsPtr: uintptr(unsafe.Pointer(&cpu.regs[0])),
		MemPtr:  uintptr(unsafe.Pointer(&cpu.memory[0])),
		MemSize: uint32(len(cpu.memory)),
		IOStart: IO_REGION_START,
		PCPtr:   uintptr(unsafe.Pointer(&cpu.PC)),
		CpuPtr:  uintptr(unsafe.Pointer(cpu)),
	}
	if cpu.bus != nil && len(cpu.bus.ioPageBitmap) > 0 {
		ctx.IOBitmapPtr = uintptr(unsafe.Pointer(&cpu.bus.ioPageBitmap[0]))
	}
	if cpu.FPU != nil {
		ctx.FPUPtr = uintptr(unsafe.Pointer(cpu.FPU))
	}
	if len(cpu.jitCodePageBitmap) > 0 {
		ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&cpu.jitCodePageBitmap[0]))
		ctx.CodePageBitmapLen = uint32(len(cpu.jitCodePageBitmap))
	}
	if len(cpu.jitPhysCodePageBitmap) > 0 {
		ctx.PhysCodeBitmapPtr = uintptr(unsafe.Pointer(&cpu.jitPhysCodePageBitmap[0]))
		ctx.PhysCodeBitmapLen = uint32(len(cpu.jitPhysCodePageBitmap))
	}
	return ctx
}

// ===========================================================================
// JITInstr — Pre-decoded IE64 instruction for JIT compilation
// ===========================================================================

type JITInstr struct {
	opcode     byte
	rd         byte
	size       byte
	xbit       byte
	rs         byte
	rt         byte
	mmuBail    bool  // when true, emit bail-to-interpreter instead of native memory access
	fpsrCCDead bool  // when true, elide the trailing FPSR condition-code update (dead write)
	fpsrCCSink bool  // when true, defer the FPSR condition-code update to the block's exit funnels
	fusedFlag  uint8 // see ie64Fused* constants
	imm32      uint32
	pcOffset   uint32 // byte offset from block start
}

// Fusion flags for JITInstr.fusedFlag.
const (
	ie64FusedJSRLeafCall   uint8 = 1 << 0 // JSR replaced by inlined leaf body — emit nothing
	ie64FusedRTSLeafReturn uint8 = 1 << 1 // synthetic RTS marker for fused leaf — emit nothing
)

// ===========================================================================
// Block Scanner
// ===========================================================================

const jitMaxBlockSize = 256

// isBlockTerminator returns true if the opcode ends a basic block.
func isBlockTerminator(opcode byte) bool {
	switch opcode {
	case OP_BRA, OP_JMP, OP_JSR64, OP_RTS64, OP_JSR_IND, OP_HALT64, OP_RTI64, OP_WAIT64:
		return true
	// MMU/privilege opcodes: all are block terminators to ensure they are always
	// the last instruction, so the dispatcher re-enters with updated state.
	case OP_SYSCALL, OP_ERET, OP_MTCR, OP_MFCR, OP_TLBFLUSH, OP_TLBINVAL, OP_SMODE,
		OP_SUAEN, OP_SUADIS:
		return true
	// Atomic RMW: block terminators because they can trap (alignment, MMU)
	case OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
		return true
	}
	return false
}

// scanBlock decodes IE64 instructions starting at startPC until a block
// terminator is found or the max block size is reached. The terminating
// instruction IS included in the block (branches need to be compiled).
func scanBlock(memory []byte, startPC uint64) []JITInstr {
	instrs := make([]JITInstr, 0, 32)
	memSize := uint64(len(memory))
	pc := startPC

	for len(instrs) < jitMaxBlockSize {
		if pc+IE64_INSTR_SIZE > memSize {
			break
		}

		instr := binary.LittleEndian.Uint64(memory[pc:])
		opcode := byte(instr)
		byte1 := byte(instr >> 8)
		byte2 := byte(instr >> 16)
		byte3 := byte(instr >> 24)
		imm32 := uint32(instr >> 32)

		ji := JITInstr{
			opcode:   opcode,
			rd:       byte1 >> 3,
			size:     (byte1 >> 1) & 0x03,
			xbit:     byte1 & 1,
			rs:       byte2 >> 3,
			rt:       byte3 >> 3,
			imm32:    imm32,
			pcOffset: uint32(pc - startPC),
		}

		// JSR with fusable register-only leaf: inline leaf body in place
		// of the JSR (no stack push, no chain). The block continues past
		// the JSR's returnPC. All inlined leaf instrs adopt the JSR's
		// pcOffset so any bail re-executes via interpreter from the JSR
		// (which restores correct stack semantics). instrCount accounting
		// stays interpreter-equivalent because JSR + body + RTS marker
		// all occupy slots in the instrs array.
		//
		// Gated by ie64ScanJSRLeafFusionEnabled because the fused
		// markers (ie64FusedJSRLeafCall / ie64FusedRTSLeafReturn) are
		// only honored by the AMD64 IE64 emitter. The ARM64 IE64 emitter
		// in jit_emit_arm64.go treats them as plain JSR/RTS and would
		// emit a real call+inlined-body+real-return, corrupting stack
		// semantics. Set per-arch in jit_common_{amd64,arm64}.go.
		if opcode == OP_JSR64 && ie64ScanJSRLeafFusionEnabled {
			targetPC := uint64(int64(pc) + int64(int32(imm32)))
			if leafBody, ok := analyzeJSRLeafFusion(memory, targetPC); ok {
				// Skip fusion if the resulting fused sequence (JSR
				// marker + body + synthetic RTS marker) plus at least
				// one slot for the still-to-scan post-JSR continuation
				// or terminator would exceed the block-size cap. This
				// guarantees a fused-RTS marker is never the last
				// instruction in instrs, so compileBlock's
				// last-instruction-based fallthrough PC + final
				// epilogue checks remain correct.
				expandedLen := len(instrs) + 1 + len(leafBody) + 1
				if expandedLen+1 <= jitMaxBlockSize {
					jsrInstr := ji
					jsrInstr.fusedFlag |= ie64FusedJSRLeafCall
					instrs = append(instrs, jsrInstr)
					for _, lji := range leafBody {
						lji.pcOffset = ji.pcOffset
						instrs = append(instrs, lji)
					}
					rtsMarker := JITInstr{
						opcode:    OP_RTS64,
						pcOffset:  ji.pcOffset,
						fusedFlag: ie64FusedRTSLeafReturn,
					}
					instrs = append(instrs, rtsMarker)
					pc += IE64_INSTR_SIZE
					continue
				}
			}
		}

		instrs = append(instrs, ji)

		if isBlockTerminator(opcode) {
			break
		}
		pc += IE64_INSTR_SIZE
	}

	return instrs
}

// analyzeJSRLeafFusion validates whether a JSR target is a fusable
// register-only leaf: ≤ 4 body instructions terminated by RTS, no memory
// access, no R31 (SP) manipulation, no embedded control flow. Returns
// the leaf body (excluding the trailing RTS) on success.
//
// Restricting to register-only ops keeps bail semantics simple — none of
// the inlined instructions can fault mid-block, so the dispatcher never
// has to re-execute the leaf, only the JSR (which never executed in JIT).
func analyzeJSRLeafFusion(memory []byte, targetPC uint64) ([]JITInstr, bool) {
	const maxBodyInstrs = 4
	memSize := uint64(len(memory))
	// Region promotion / leaf fusion is gated to low memory at the call
	// site, but defensively refuse fusion for targets that escape the
	// memory slice so the byte read below cannot panic.
	if targetPC >= memSize {
		return nil, false
	}
	pc := targetPC
	body := make([]JITInstr, 0, maxBodyInstrs)

	for i := 0; i < maxBodyInstrs+1; i++ {
		if pc+IE64_INSTR_SIZE > memSize {
			return nil, false
		}
		instr := binary.LittleEndian.Uint64(memory[pc:])
		opcode := byte(instr)
		if opcode == OP_RTS64 {
			return body, true
		}
		if !isLeafFusionSafe(opcode, instr) {
			return nil, false
		}
		byte1 := byte(instr >> 8)
		byte2 := byte(instr >> 16)
		byte3 := byte(instr >> 24)
		imm32 := uint32(instr >> 32)
		body = append(body, JITInstr{
			opcode:   opcode,
			rd:       byte1 >> 3,
			size:     (byte1 >> 1) & 0x03,
			xbit:     byte1 & 1,
			rs:       byte2 >> 3,
			rt:       byte3 >> 3,
			imm32:    imm32,
			pcOffset: 0,
		})
		pc += IE64_INSTR_SIZE
	}
	return nil, false
}

// isLeafFusionSafe returns true iff the opcode is a register-only
// instruction safe to inline at a JSR site: no memory access, no R31
// (SP) destination, no control flow, no FPU/atomic side effects.
func isLeafFusionSafe(opcode byte, instr uint64) bool {
	rd := byte(instr>>8) >> 3
	if rd == 31 {
		return false
	}
	switch opcode {
	case OP_NOP64:
		return true
	case OP_MOVE, OP_MOVT, OP_MOVEQ, OP_LEA:
		return true
	case OP_ADD, OP_SUB, OP_AND64, OP_OR64, OP_EOR,
		OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64,
		OP_NEG, OP_NOT64, OP_CLZ, OP_SEXT,
		OP_LSL, OP_LSR, OP_ASR:
		return true
	}
	return false
}

// scanBlockBus is the high-physical-address variant of scanBlock. It fetches
// every 8-byte instruction word through bus.ReadPhys64WithFault so that code
// placed in sparse / high-physical backing (above the legacy cpu.memory[]
// window) can be JITed. For low-PC fast path callers that already hold a
// memory slice, prefer the older scanBlock — this routine pays a per-instr
// bus dispatch.
//
// Scanning stops on the first unmapped fetch (matches the interpreter, which
// halts execution on unmapped instruction fetches). The terminating
// instruction is included in the returned slice when valid.
func scanBlockBus(bus *MachineBus, startPC uint64) []JITInstr {
	instrs := make([]JITInstr, 0, 32)
	pc := startPC

	for len(instrs) < jitMaxBlockSize {
		instrWord, ok := bus.ReadPhys64WithFault(pc)
		if !ok {
			break
		}
		opcode := byte(instrWord)
		byte1 := byte(instrWord >> 8)
		byte2 := byte(instrWord >> 16)
		byte3 := byte(instrWord >> 24)
		imm32 := uint32(instrWord >> 32)

		ji := JITInstr{
			opcode:   opcode,
			rd:       byte1 >> 3,
			size:     (byte1 >> 1) & 0x03,
			xbit:     byte1 & 1,
			rs:       byte2 >> 3,
			rt:       byte3 >> 3,
			imm32:    imm32,
			pcOffset: uint32(pc - startPC),
		}
		instrs = append(instrs, ji)

		if isBlockTerminator(opcode) {
			break
		}
		// Subtraction-form guard: pc+IE64_INSTR_SIZE wraps near
		// MaxUint64 and would re-enter low addresses. Stop instead.
		if pc > ^uint64(0)-IE64_INSTR_SIZE {
			break
		}
		pc += IE64_INSTR_SIZE
	}

	return instrs
}

// scanBlockBusWithLimit is the bus-aware variant of scanBlockWithLimit, used
// for MMU-on scanning where the high physical page is reached via bus phys.
func scanBlockBusWithLimit(bus *MachineBus, startPC, maxPC uint64) []JITInstr {
	instrs := make([]JITInstr, 0, 32)
	pc := startPC

	for len(instrs) < jitMaxBlockSize {
		// Subtraction-form bound: pc+IE64_INSTR_SIZE wraps near
		// MaxUint64; use maxPC - IE64_INSTR_SIZE so the bound check is
		// safe regardless of how high startPC sits. maxPC is required
		// to be >= IE64_INSTR_SIZE by callers (page-aligned).
		if maxPC < IE64_INSTR_SIZE || pc > maxPC-IE64_INSTR_SIZE {
			break
		}
		instrWord, ok := bus.ReadPhys64WithFault(pc)
		if !ok {
			break
		}
		opcode := byte(instrWord)
		byte1 := byte(instrWord >> 8)
		byte2 := byte(instrWord >> 16)
		byte3 := byte(instrWord >> 24)
		imm32 := uint32(instrWord >> 32)

		ji := JITInstr{
			opcode:   opcode,
			rd:       byte1 >> 3,
			size:     (byte1 >> 1) & 0x03,
			xbit:     byte1 & 1,
			rs:       byte2 >> 3,
			rt:       byte3 >> 3,
			imm32:    imm32,
			pcOffset: uint32(pc - startPC),
		}
		instrs = append(instrs, ji)

		if isBlockTerminator(opcode) {
			break
		}
		if pc > ^uint64(0)-IE64_INSTR_SIZE {
			break
		}
		pc += IE64_INSTR_SIZE
	}

	return instrs
}

// scanBlockWithLimit is like scanBlock but stops at maxPC (exclusive).
// Used when MMU is enabled to prevent scanning across page boundaries.
func scanBlockWithLimit(memory []byte, startPC, maxPC uint64) []JITInstr {
	instrs := make([]JITInstr, 0, 32)
	pc := startPC

	for len(instrs) < jitMaxBlockSize {
		if pc+IE64_INSTR_SIZE > maxPC {
			break
		}

		instr := binary.LittleEndian.Uint64(memory[pc:])
		opcode := byte(instr)
		byte1 := byte(instr >> 8)
		byte2 := byte(instr >> 16)
		byte3 := byte(instr >> 24)
		imm32 := uint32(instr >> 32)

		ji := JITInstr{
			opcode:   opcode,
			rd:       byte1 >> 3,
			size:     (byte1 >> 1) & 0x03,
			xbit:     byte1 & 1,
			rs:       byte2 >> 3,
			rt:       byte3 >> 3,
			imm32:    imm32,
			pcOffset: uint32(pc - startPC),
		}
		instrs = append(instrs, ji)

		if isBlockTerminator(opcode) {
			break
		}
		if pc > ^uint64(0)-IE64_INSTR_SIZE {
			break
		}
		pc += IE64_INSTR_SIZE
	}

	return instrs
}

// containsStackOp returns true if any instruction in the block touches
// the stack (PUSH/POP/JSR/RTS/JSR_IND). The non-MMU stack emitters
// still address the stack as raw [memBase+R31], so a high-phys block
// whose SP is also in high RAM would read/write past cpu.memory. Used
// as a Phase 4 interim guard until Phase 5 wires bus-aware helpers.
func containsStackOp(instrs []JITInstr) bool {
	for i := range instrs {
		switch instrs[i].opcode {
		case OP_PUSH64, OP_POP64, OP_JSR64, OP_RTS64, OP_JSR_IND:
			return true
		}
	}
	return false
}

// needsFallback returns true if the block contains any instruction that
// the JIT cannot safely compile. The first-instruction-only opcodes
// (HALT/WAIT/RTI/MMU privileged) preserve legacy block-entry behavior;
// the new full-block scan catches PLAN_MAX_RAM.md slice 4 hazards where
// the JIT memory emitters and dynamic-JMP target masking still operate
// in 32-bit address space — a LOAD/STORE to a high-phys backing page or
// a JMP/JSR_IND to a high VA would either alias or wrap. Forcing those
// blocks through the interpreter is correct until the JIT emitters
// widen to 64-bit addressing.
func needsFallback(instrs []JITInstr) bool {
	if len(instrs) == 0 {
		return true
	}
	// Block-entry-only checks (legacy behavior).
	op := instrs[0].opcode
	switch op {
	case OP_FMOD, OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP, OP_FPOW, OP_DMOD:
		return true
	case OP_HALT64, OP_WAIT64, OP_RTI64:
		return true
	case OP_SYSCALL, OP_ERET, OP_MTCR, OP_MFCR, OP_TLBFLUSH, OP_TLBINVAL, OP_SMODE,
		OP_SUAEN, OP_SUADIS:
		return true
	}
	// Full-block scan. The only remaining whole-block bail trigger is an
	// invalid FPU register encoding (below). The memory and branch opcodes
	// are NOT block-bail:
	//
	// - LOAD / STORE / FLOAD / FSTORE / DLOAD / DSTORE use native low-RAM
	//   fast paths and helper exits for MMU/high-address/MMIO cases; they do
	//   not force a whole-block bail.
	// - Atomic RMW ops use native non-MMU low-RAM fast paths and interpreter
	//   exits for MMU, high-address, MMIO, or alignment-trap cases.
	// - JMP / JSR_IND (PLAN_MAX_RAM.md slice 8 phase 8): emitJMP / emitJSR_IND
	//   no longer AND the target with the legacy IE64_ADDR_MASK, so jumps
	//   reach the full uint32 PC; with MMU on compileBlockMMU still bails
	//   JSR_IND per instruction.
	//
	// Phase4d safety boundary (mmu_ie64_phase4b_test.go):
	//   (a) MMU-off uint32 window — direct emit, asserted parity vs
	//       interpreter via TestPhase4d_NonMMU_AllowsMemOps_NoBlockBail
	//       and the broader ExecuteJIT memory tests.
	//   (b) MMU-on — compileBlockMMU sets mmuBail per memory op, asserted
	//       by TestPhase4d_MMU_BailsAllMemOps + TestPhase4d_MMU_BailsAllAtomics.
	//   (c) Dispatch — exec.go selects compileBlockMMU vs compileBlock
	//       on cpu.mmuEnabled, asserted by TestPhase4d_DispatchSelectsMMUCompiler.
	for i := range instrs {
		if isIE64FPUOpcode(instrs[i].opcode) && !validIE64FPUEncoding(instrs[i].opcode, instrs[i].rd, instrs[i].rs, instrs[i].rt) {
			return true
		}
	}
	return false
}

// ===========================================================================
// Register Liveness Analysis
// ===========================================================================

// blockRegs holds register usage bitmasks for a JIT block.
// Bit i corresponds to IE64 register i (0-31). R0 is always cleared (XZR).
type blockRegs struct {
	read              uint32 // IE64 registers that are read by any instruction
	written           uint32 // IE64 registers that are written by any instruction
	used              uint32 // read | written (determines callee-saved pairs)
	hasFPU            bool   // true if any FPU opcode (0x60-0x7C) is in the block
	hasBackwardBranch bool   // true if any Bcc/BRA targets an earlier instruction
}

// analyzeBlockRegs scans a block's instructions and returns bitmasks of
// which IE64 registers are read and written. Used to minimize prologue/epilogue
// overhead — only load/store registers the block actually touches.
func analyzeBlockRegs(instrs []JITInstr) blockRegs {
	var read, written uint32
	hasFPU := false
	for _, ji := range instrs {
		switch ji.opcode {
		case OP_MOVE:
			if ji.xbit == 0 {
				read |= 1 << ji.rs
			}
			written |= 1 << ji.rd
		case OP_MOVT:
			read |= 1 << ji.rd // read-modify-write (preserves lower 32 bits)
			written |= 1 << ji.rd
		case OP_MOVEQ:
			written |= 1 << ji.rd
		case OP_LEA:
			read |= 1 << ji.rs
			written |= 1 << ji.rd
		case OP_ADD, OP_SUB, OP_AND64, OP_OR64, OP_EOR:
			read |= 1 << ji.rs
			if ji.xbit == 0 {
				read |= 1 << ji.rt
			}
			written |= 1 << ji.rd
		case OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64, OP_MODS, OP_MULHU, OP_MULHS:
			read |= 1 << ji.rs
			if ji.xbit == 0 {
				read |= 1 << ji.rt
			}
			written |= 1 << ji.rd
		case OP_NEG, OP_NOT64, OP_CLZ, OP_SEXT, OP_CTZ, OP_POPCNT, OP_BSWAP:
			read |= 1 << ji.rs
			written |= 1 << ji.rd
		case OP_LSL, OP_LSR, OP_ASR, OP_ROL, OP_ROR:
			read |= 1 << ji.rs
			if ji.xbit == 0 {
				read |= 1 << ji.rt
			}
			written |= 1 << ji.rd
		case OP_LOAD:
			read |= 1 << ji.rs
			written |= 1 << ji.rd
		case OP_STORE:
			read |= 1 << ji.rs
			read |= 1 << ji.rd // rd is value to store (read)
		case OP_CAS:
			read |= 1 << ji.rs // address base
			read |= 1 << ji.rt // replacement value
			read |= 1 << ji.rd // expected value
			written |= 1 << ji.rd
		case OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
			read |= 1 << ji.rs // address base
			read |= 1 << ji.rt // RMW operand
			written |= 1 << ji.rd
		case OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
			read |= 1 << ji.rs
			read |= 1 << ji.rt
		case OP_JMP:
			read |= 1 << ji.rs
		case OP_JSR64:
			read |= 1 << 31
			written |= 1 << 31
		case OP_RTS64:
			read |= 1 << 31
			written |= 1 << 31
		case OP_PUSH64:
			read |= 1 << ji.rs
			read |= 1 << 31
			written |= 1 << 31
		case OP_POP64:
			written |= 1 << ji.rd
			read |= 1 << 31
			written |= 1 << 31
		case OP_JSR_IND:
			read |= 1 << ji.rs
			read |= 1 << 31
			written |= 1 << 31

		// FPU opcodes that touch integer registers
		case OP_FMOVI:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs
		case OP_FMOVO:
			hasFPU = true
			written |= 1 << ji.rd // writes integer rd
		case OP_FCMP:
			hasFPU = true
			written |= 1 << ji.rd // writes integer rd (comparison result)
		case OP_FCVTIF:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs
		case OP_FCVTFI:
			hasFPU = true
			written |= 1 << ji.rd // writes integer rd
		case OP_FMOVSR, OP_FMOVCR:
			hasFPU = true
			written |= 1 << ji.rd // writes integer rd (FPSR/FPCR value)
		case OP_FMOVSC, OP_FMOVCC:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs
		case OP_FLOAD:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs (address base)
		case OP_FSTORE:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs (address base)
		case OP_FMOV, OP_FABS, OP_FNEG, OP_FMOVECR,
			OP_FADD, OP_FSUB, OP_FMUL, OP_FDIV, OP_FSQRT, OP_FINT,
			OP_FMOD, OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP, OP_FPOW:
			hasFPU = true
		case OP_DCVTIF:
			hasFPU = true
			read |= 1 << ji.rs
		case OP_DCMP, OP_DCVTFI:
			hasFPU = true
			written |= 1 << ji.rd
		case OP_DLOAD, OP_DSTORE:
			hasFPU = true
			read |= 1 << ji.rs
		case OP_DMOV, OP_DABS, OP_DNEG, OP_DSQRT, OP_DINT, OP_FCVTSD, OP_FCVTDS,
			OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV, OP_DMOD,
			OP_DSIN, OP_DCOS, OP_DTAN, OP_DATAN, OP_DLOG, OP_DEXP, OP_DPOW:
			hasFPU = true

		// RTI pops return address from stack (reads & writes R31/SP)
		case OP_RTI64:
			read |= 1 << 31
			written |= 1 << 31
		}
	}
	// R0 is XZR — never loaded or stored
	read &^= 1
	written &^= 1
	return blockRegs{read: read, written: written, used: read | written, hasFPU: hasFPU}
}

// instrWrittenRegs returns a bitmask of IE64 registers written by a single
// instruction. Used to track writtenSoFar for I/O bail epilogues.
func instrWrittenRegs(ji *JITInstr) uint32 {
	var w uint32
	switch ji.opcode {
	case OP_MOVE, OP_MOVT, OP_MOVEQ, OP_LEA, OP_LOAD:
		w = 1 << ji.rd
	case OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
		w = 1 << ji.rd
	case OP_ADD, OP_SUB, OP_AND64, OP_OR64, OP_EOR,
		OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64, OP_MODS, OP_MULHU, OP_MULHS,
		OP_NEG, OP_NOT64, OP_CLZ, OP_SEXT, OP_CTZ, OP_POPCNT, OP_BSWAP,
		OP_LSL, OP_LSR, OP_ASR, OP_ROL, OP_ROR:
		w = 1 << ji.rd
	case OP_JSR64, OP_RTS64, OP_JSR_IND:
		w = 1 << 31
	case OP_PUSH64:
		w = 1 << 31
	case OP_POP64:
		w = (1 << ji.rd) | (1 << 31)
	// FPU opcodes that write integer registers
	case OP_FMOVO, OP_FCMP, OP_FCVTFI, OP_FMOVSR, OP_FMOVCR, OP_DCMP, OP_DCVTFI:
		w = 1 << ji.rd
	// RTI writes R31 (SP += 8)
	case OP_RTI64:
		w = 1 << 31
	}
	return w &^ 1 // clear R0
}

// detectBackwardBranches returns true if any conditional branch (BEQ-BLS) or
// BRA targets an earlier instruction within the same block. Used to enable
// native backward branches with budget-based timer safety.
func detectBackwardBranches(instrs []JITInstr, startPC uint64) bool {
	for _, ji := range instrs {
		var isBranch bool
		switch ji.opcode {
		case OP_BRA, OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
			isBranch = true
		}
		if !isBranch {
			continue
		}
		instrPC := startPC + uint64(ji.pcOffset)
		targetPC := uint64(int64(instrPC) + int64(int32(ji.imm32)))
		if targetPC >= startPC && targetPC < instrPC && (targetPC-startPC)%IE64_INSTR_SIZE == 0 {
			return true
		}
	}
	return false
}

// ===========================================================================
// CodeBuffer — Byte buffer for emitting native machine code
// ===========================================================================

type fixup struct {
	name   string
	offset int // byte offset in buf where patch is needed
	size   int // patch size in bytes (4 for ARM64, variable for x86-64)
	pcBase int // base PC for PC-relative calculations
}

type CodeBuffer struct {
	buf    []byte
	labels map[string]int // label name -> byte offset
	fixups []fixup
}

func NewCodeBuffer(capacity int) *CodeBuffer {
	return &CodeBuffer{
		buf:    make([]byte, 0, capacity),
		labels: make(map[string]int),
	}
}

// Emit32 appends a 32-bit value (little-endian). Used for ARM64 fixed-width instructions.
func (cb *CodeBuffer) Emit32(val uint32) {
	cb.buf = append(cb.buf, byte(val), byte(val>>8), byte(val>>16), byte(val>>24))
}

// EmitBytes appends raw bytes. Used for x86-64 variable-length instructions.
func (cb *CodeBuffer) EmitBytes(b ...byte) {
	cb.buf = append(cb.buf, b...)
}

// Emit16 appends a 16-bit value (little-endian).
func (cb *CodeBuffer) Emit16(val uint16) {
	cb.buf = append(cb.buf, byte(val), byte(val>>8))
}

// Emit64 appends a 64-bit value (little-endian).
func (cb *CodeBuffer) Emit64(val uint64) {
	cb.buf = append(cb.buf,
		byte(val), byte(val>>8), byte(val>>16), byte(val>>24),
		byte(val>>32), byte(val>>40), byte(val>>48), byte(val>>56))
}

// Label records the current byte offset for a named label.
func (cb *CodeBuffer) Label(name string) {
	cb.labels[name] = len(cb.buf)
}

// FixupRel32 records a 32-bit PC-relative fixup at the current position.
// pcBase is the reference point for the relative calculation.
func (cb *CodeBuffer) FixupRel32(name string, pcBase int) {
	cb.fixups = append(cb.fixups, fixup{
		name:   name,
		offset: len(cb.buf),
		size:   4,
		pcBase: pcBase,
	})
	// Emit placeholder
	cb.buf = append(cb.buf, 0, 0, 0, 0)
}

// FixupExistingRel32 records a PC-relative fixup for an already-emitted rel32
// placeholder. Use this with helpers that emit a Jcc/JMP placeholder and return
// the offset of its rel32 field.
func (cb *CodeBuffer) FixupExistingRel32(name string, rel32Off int) {
	cb.fixups = append(cb.fixups, fixup{
		name:   name,
		offset: rel32Off,
		size:   4,
		pcBase: rel32Off + 4,
	})
}

// Resolve patches all forward-reference fixups with actual label offsets.
func (cb *CodeBuffer) Resolve() {
	for _, f := range cb.fixups {
		target, ok := cb.labels[f.name]
		if !ok {
			continue
		}
		rel := int32(target - f.pcBase)
		binary.LittleEndian.PutUint32(cb.buf[f.offset:], uint32(rel))
	}
	cb.fixups = cb.fixups[:0]
}

// Len returns the current code size in bytes.
func (cb *CodeBuffer) Len() int {
	return len(cb.buf)
}

// Bytes returns the emitted code.
func (cb *CodeBuffer) Bytes() []byte {
	return cb.buf
}

// PatchUint32 overwrites 4 bytes at the given offset.
func (cb *CodeBuffer) PatchUint32(offset int, val uint32) {
	binary.LittleEndian.PutUint32(cb.buf[offset:], val)
}

// ===========================================================================
// Code Cache
// ===========================================================================

// chainSlot records a patchable chain exit point within a compiled block.
type chainSlot struct {
	targetPC  uint64  // 6502/IE64 PC this exit targets (full uint64 for IE64)
	patchAddr uintptr // address of JMP rel32 displacement in ExecMem
}

type JITBlock struct {
	startPC        uint64
	endPC          uint64
	instrCount     int
	execAddr       uintptr
	execSize       int
	chainEntry     uintptr     // lightweight entry point for chained transitions (0 = none)
	chainSlots     []chainSlot // patchable exit points
	execCount      uint32      // execution count for hot-block detection (Tier 2)
	tier           int         // compilation tier (0=Tier 1, 1=Tier 2)
	regMap         [8]byte     // x86 JIT: guest-to-host register mapping for chain compatibility
	chainHits      uint32      // times this block was entered via chain (not Go dispatch)
	unchainedExits uint32      // times this block exited via unchained path
	ioBails        uint32      // times this block triggered I/O fallback
	lastPromoteAt  uint32      // exec count when last promoted (hysteresis)
	dominantDeopt  DeoptReason // first observed deopt reason for this block
	rIncrements    int         // Z80: total R register increments for this block
	// ptbr is the MMU page-table-base address active when this block was
	// compiled, or 0 for non-MMU backends. Used by IE64's chain patcher
	// to scope inbound/outbound chain links to a single address space —
	// without this filter, two address spaces sharing a virtual PC could
	// cross-link native blocks and execute the wrong physical code.
	ptbr uint64

	// IE64 region planning metadata. The current emitter still uses
	// the fixed Tier-1 register mapping; these fields expose the pressure
	// plan used for diagnostics and optional admission gating.
	regionRegMask   uint32
	regionSpillOps  int
	regionFPUSpills int

	// coveredRanges optionally enumerates every guest [start, end) span
	// the block's native code was compiled from. Non-nil only for
	// region blocks whose constituent guest blocks are non-contiguous
	// in the address space — a region 0x100→0x5000→0x200 cannot be
	// described by a single [startPC, endPC) span and would silently
	// miss SMC invalidation for the 0x5000 block. Nil means the
	// canonical [startPC, endPC) span is exact.
	coveredRanges [][2]uint64

	guestHash      uint64
	guestHashValid bool
	execReleased   bool
}

type chainPatchRef struct {
	patchAddr uintptr
	ptbr      uint64
}

// JITBlockCoveredRanges returns the guest PC ranges the block's native
// code was compiled from. For per-block compiles this is just the
// canonical [startPC, endPC) span; for region compiles whose
// constituent blocks are non-contiguous it is the explicit list set
// at compile time. SMC invalidation and code-page bitmap marking must
// iterate this slice rather than [startPC, endPC) directly.
func JITBlockCoveredRanges(b *JITBlock) [][2]uint64 {
	if b.coveredRanges != nil {
		return b.coveredRanges
	}
	return [][2]uint64{{b.startPC, b.endPC}}
}

// ie64ResolveTerminatorTarget computes the static branch target for a
// region-eligible IE64 terminator. Returns (targetPC, true) for BRA
// (PC-relative imm32) and JMP with rs == 0 (absolute imm32, since the
// target is rs + sign_extend(imm32) and rs == R0/XZR resolves to a
// statically known target). Calls (JSR64, JSR_IND) and indirect/system
// terminators (RTS64, RTI64, HALT64, WAIT64, SYSCALL, ERET, MTCR/MFCR,
// TLBFLUSH/INVAL, SMODE, SUAEN/SUADIS, atomic RMWs) return (0, false)
// — region formation does not follow them.
//
// instrPC is the PC of the terminating instruction itself.
func ie64ResolveTerminatorTarget(opcode byte, rs byte, imm32 uint32, instrPC uint64) (uint64, bool) {
	switch opcode {
	case OP_BRA:
		return uint64(int64(instrPC) + int64(int32(imm32))), true
	case OP_JMP:
		if rs != 0 {
			return 0, false
		}
		return uint64(int64(int32(imm32))), true
	}
	return 0, false
}

// ie64WindowCoversIORegion is a compile-time proof that the low guest-RAM
// window always covers the whole sub-I/O address range. The window is never
// smaller than MIN_GUEST_RAM, so any address below IO_REGION_START is both
// mapped RAM and inside the window. If this invariant were ever broken the
// subtraction would underflow an unsigned constant and fail to compile,
// invalidating the constant-address elision in ie64ConstLowRAMAccess.
const ie64WindowCoversIORegion uint64 = MIN_GUEST_RAM - IO_REGION_START

// ie64ConstLowRAMAccess reports whether a LOAD/STORE with base register rs,
// signed 32-bit displacement imm32 and the given operand size has a
// compile-time constant effective address that provably lands in low RAM
// below the I/O region. When it returns ok, the access needs neither an
// I/O-page probe nor a window bound check: base R0 is hardwired zero, the
// displacement is non-negative, and the whole access [addr, addr+bytes) lies
// below IO_REGION_START, which is always inside the window (see
// ie64WindowCoversIORegion). MMU translation is a separate concern and is not
// elided by this. Shared, untagged, so the amd64/arm64 emitters and the wasm
// backend apply the identical proof.
func ie64ConstLowRAMAccess(rs byte, imm32 uint32, size byte) (uint64, bool) {
	if rs != 0 {
		return 0, false
	}
	if int32(imm32) < 0 {
		return 0, false
	}
	addr := uint64(imm32)
	if addr+uint64(ie64AccessBytes(size)) > IO_REGION_START {
		return 0, false
	}
	return addr, true
}

// ie64CacheKey is the exact composite key used by IE64's MMU mode so that
// two address spaces sharing the same virtual PC cannot collide on the
// JIT cache. The legacy `(ptbr * golden_ratio) ^ pcVirt` hash was lossy
// and could produce identical keys for distinct {ptbr, pc} pairs, causing
// the dispatcher to execute the wrong physical block on context switch.
type ie64CacheKey struct {
	ptbr uint64
	pc   uint64
}

type CodeCache struct {
	blocks            map[uint64]*JITBlock       // non-MMU: keyed by guest PC
	mmuBlocks         map[ie64CacheKey]*JITBlock // MMU mode: exact (ptbr, vPC) composite
	pageBlocks        map[uint64]map[*JITBlock]struct{}
	mmuPageBlocks     map[ie64PageCacheKey]map[*JITBlock]struct{}
	inboundChainSlots map[uint64][]chainPatchRef // chain slots keyed by target PC
	dispatch          *JITDispatchCache          // direct-mapped dispatch lookup cache
	generation        uint64                     // bumped on invalidation so dispatch entries expire in O(1)
}

type ie64PageCacheKey struct {
	ptbr uint64
	page uint64
}

func NewCodeCache() *CodeCache {
	cc := &CodeCache{
		blocks:            make(map[uint64]*JITBlock),
		mmuBlocks:         make(map[ie64CacheKey]*JITBlock),
		pageBlocks:        make(map[uint64]map[*JITBlock]struct{}),
		mmuPageBlocks:     make(map[ie64PageCacheKey]map[*JITBlock]struct{}),
		inboundChainSlots: make(map[uint64][]chainPatchRef),
	}
	if !jitDispatchCacheDisabled {
		cc.dispatch = newJITDispatchCache()
	}
	return cc
}

func (cc *CodeCache) Len() int {
	if cc == nil {
		return 0
	}
	return len(cc.blocks) + len(cc.mmuBlocks)
}

func resetExecMemWhenCacheEmpty(cc *CodeCache, execMem *ExecMem) bool {
	if cc == nil || execMem == nil || cc.Len() != 0 {
		return false
	}
	execMem.Reset()
	return true
}

func jitRangesOverlap(lo, hi uint64, r [2]uint64) bool {
	return r[1] > r[0] && r[1] > lo && r[0] < hi
}

func codeCachePagesForRange(lo, hi uint64, visit func(page uint64) bool) {
	if hi <= lo {
		return
	}
	startPage := lo >> 8
	endPage := (hi - 1) >> 8
	for p := startPage; p <= endPage; p++ {
		if !visit(p) || p == ^uint64(0) {
			return
		}
	}
}

func codeCachePagesForBlock(block *JITBlock, visit func(page uint64)) {
	for _, r := range JITBlockCoveredRanges(block) {
		if r[1] <= r[0] {
			continue
		}
		codeCachePagesForRange(r[0], r[1], func(page uint64) bool {
			visit(page)
			return true
		})
	}
}

func addCodeCachePageBlock(index map[uint64]map[*JITBlock]struct{}, page uint64, block *JITBlock) {
	set := index[page]
	if set == nil {
		set = make(map[*JITBlock]struct{}, 1)
		index[page] = set
	}
	set[block] = struct{}{}
}

func removeCodeCachePageBlock(index map[uint64]map[*JITBlock]struct{}, page uint64, block *JITBlock) {
	set := index[page]
	if set == nil {
		return
	}
	delete(set, block)
	if len(set) == 0 {
		delete(index, page)
	}
}

func addCodeCacheMMUPageBlock(index map[ie64PageCacheKey]map[*JITBlock]struct{}, ptbr, page uint64, block *JITBlock) {
	key := ie64PageCacheKey{ptbr: ptbr, page: page}
	set := index[key]
	if set == nil {
		set = make(map[*JITBlock]struct{}, 1)
		index[key] = set
	}
	set[block] = struct{}{}
}

func removeCodeCacheMMUPageBlock(index map[ie64PageCacheKey]map[*JITBlock]struct{}, ptbr, page uint64, block *JITBlock) {
	key := ie64PageCacheKey{ptbr: ptbr, page: page}
	set := index[key]
	if set == nil {
		return
	}
	delete(set, block)
	if len(set) == 0 {
		delete(index, key)
	}
}

func (cc *CodeCache) indexBlock(block *JITBlock) {
	if cc == nil || block == nil {
		return
	}
	codeCachePagesForBlock(block, func(page uint64) {
		addCodeCachePageBlock(cc.pageBlocks, page, block)
	})
}

func (cc *CodeCache) unindexBlock(block *JITBlock) {
	if cc == nil || block == nil {
		return
	}
	codeCachePagesForBlock(block, func(page uint64) {
		removeCodeCachePageBlock(cc.pageBlocks, page, block)
	})
}

func (cc *CodeCache) indexMMUBlock(ptbr uint64, block *JITBlock) {
	if cc == nil || block == nil {
		return
	}
	codeCachePagesForBlock(block, func(page uint64) {
		addCodeCacheMMUPageBlock(cc.mmuPageBlocks, ptbr, page, block)
	})
}

func (cc *CodeCache) unindexMMUBlock(ptbr uint64, block *JITBlock) {
	if cc == nil || block == nil {
		return
	}
	codeCachePagesForBlock(block, func(page uint64) {
		removeCodeCacheMMUPageBlock(cc.mmuPageBlocks, ptbr, page, block)
	})
}

func codeCacheBlockOverlapsRange(block *JITBlock, lo, hi uint64) bool {
	for _, r := range JITBlockCoveredRanges(block) {
		if jitRangesOverlap(lo, hi, r) {
			return true
		}
	}
	return false
}

// GetMMU looks up an MMU-scoped block with an exact composite key.
func (cc *CodeCache) GetMMU(ptbr, pc uint64) *JITBlock {
	if block := cc.dispatch.get(pc, ptbr, cc.generation); block != nil {
		return block
	}
	block := cc.mmuBlocks[ie64CacheKey{ptbr: ptbr, pc: pc}]
	cc.dispatch.put(pc, ptbr, cc.generation, block)
	return block
}

func (cc *CodeCache) OverlapsRange(lo, hi uint64) bool {
	if cc == nil || hi <= lo {
		return false
	}
	startPage := lo >> 8
	endPage := (hi - 1) >> 8
	if startPage == endPage {
		for block := range cc.pageBlocks[startPage] {
			if codeCacheBlockOverlapsRange(block, lo, hi) {
				return true
			}
		}
		return false
	}
	seen := make(map[*JITBlock]struct{})
	found := false
	codeCachePagesForRange(lo, hi, func(page uint64) bool {
		for block := range cc.pageBlocks[page] {
			if _, ok := seen[block]; ok {
				continue
			}
			seen[block] = struct{}{}
			if codeCacheBlockOverlapsRange(block, lo, hi) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func (cc *CodeCache) OverlapsRangeScoped(ptbr, lo, hi uint64) bool {
	if cc == nil || hi <= lo {
		return false
	}
	startPage := lo >> 8
	endPage := (hi - 1) >> 8
	if startPage == endPage {
		for block := range cc.mmuPageBlocks[ie64PageCacheKey{ptbr: ptbr, page: startPage}] {
			if codeCacheBlockOverlapsRange(block, lo, hi) {
				return true
			}
		}
		return false
	}
	seen := make(map[*JITBlock]struct{})
	found := false
	codeCachePagesForRange(lo, hi, func(page uint64) bool {
		for block := range cc.mmuPageBlocks[ie64PageCacheKey{ptbr: ptbr, page: page}] {
			if _, ok := seen[block]; ok {
				continue
			}
			seen[block] = struct{}{}
			if codeCacheBlockOverlapsRange(block, lo, hi) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// PutMMU stores an MMU-scoped block under its exact composite key.
func (cc *CodeCache) PutMMU(ptbr, pc uint64, block *JITBlock) {
	if old := cc.mmuBlocks[ie64CacheKey{ptbr: ptbr, pc: pc}]; old != nil {
		cc.unindexMMUBlock(ptbr, old)
		cc.unregisterChainSlots(old)
		if old != block {
			releaseJITBlockExecMem(old)
		}
	}
	block.ptbr = ptbr
	cc.mmuBlocks[ie64CacheKey{ptbr: ptbr, pc: pc}] = block
	cc.indexMMUBlock(ptbr, block)
	cc.registerChainSlots(block)
	cc.dispatch.put(pc, ptbr, cc.generation, block)
}

func (cc *CodeCache) Get(pc uint64) *JITBlock {
	if block := cc.dispatch.get(pc, 0, cc.generation); block != nil {
		return block
	}
	block := cc.blocks[pc]
	cc.dispatch.put(pc, 0, cc.generation, block)
	return block
}

func (cc *CodeCache) Put(block *JITBlock) {
	if old := cc.blocks[block.startPC]; old != nil {
		cc.unindexBlock(old)
		cc.unregisterChainSlots(old)
		if old != block {
			releaseJITBlockExecMem(old)
		}
	}
	cc.blocks[block.startPC] = block
	cc.indexBlock(block)
	cc.registerChainSlots(block)
	cc.dispatch.put(block.startPC, 0, cc.generation, block)
}

func (cc *CodeCache) GetKey(key uint64) *JITBlock {
	if block := cc.dispatch.get(key, 0, cc.generation); block != nil {
		return block
	}
	block := cc.blocks[key]
	cc.dispatch.put(key, 0, cc.generation, block)
	return block
}

func (cc *CodeCache) PutKey(key uint64, block *JITBlock) {
	if old := cc.blocks[key]; old != nil {
		cc.unindexBlock(old)
		cc.unregisterChainSlots(old)
		if old != block {
			releaseJITBlockExecMem(old)
		}
	}
	cc.blocks[key] = block
	cc.indexBlock(block)
	cc.registerChainSlots(block)
	cc.dispatch.put(key, 0, cc.generation, block)
}

// Invalidate clears the entire code cache (both non-MMU and MMU maps).
func (cc *CodeCache) Invalidate() {
	cc.releaseAllExecMem()
	clear(cc.blocks)
	clear(cc.mmuBlocks)
	clear(cc.pageBlocks)
	clear(cc.mmuPageBlocks)
	clear(cc.inboundChainSlots)
	cc.generation++
	cc.dispatch.reset()
}

// InvalidateRange removes any blocks whose covered guest PC ranges
// overlap [lo, hi). Region blocks may have multiple non-contiguous
// covered ranges; iterating JITBlockCoveredRanges catches an SMC write
// to a region's middle block that the canonical [startPC, endPC)
// span would miss.
func (cc *CodeCache) InvalidateRange(lo, hi uint64) int {
	removed := 0
	for key, block := range cc.blocks {
		for _, r := range JITBlockCoveredRanges(block) {
			if jitRangesOverlap(lo, hi, r) {
				cc.unpatchChainsToBlock(block)
				cc.unindexBlock(block)
				cc.unregisterChainSlots(block)
				delete(cc.blocks, key)
				releaseJITBlockExecMem(block)
				removed++
				break
			}
		}
	}
	for key, block := range cc.mmuBlocks {
		for _, r := range JITBlockCoveredRanges(block) {
			if jitRangesOverlap(lo, hi, r) {
				cc.unpatchChainsToBlock(block)
				cc.unindexMMUBlock(key.ptbr, block)
				cc.unregisterChainSlots(block)
				delete(cc.mmuBlocks, key)
				releaseJITBlockExecMem(block)
				removed++
				break
			}
		}
	}
	if removed != 0 {
		cc.generation++
		cc.dispatch.reset()
	}
	return removed
}

func (cc *CodeCache) InvalidateRangeScoped(ptbr, lo, hi uint64) int {
	removed := 0
	for key, block := range cc.mmuBlocks {
		if key.ptbr != ptbr && block.ptbr != ptbr {
			continue
		}
		for _, r := range JITBlockCoveredRanges(block) {
			if jitRangesOverlap(lo, hi, r) {
				cc.unpatchChainsToBlock(block)
				cc.unindexMMUBlock(key.ptbr, block)
				cc.unregisterChainSlots(block)
				delete(cc.mmuBlocks, key)
				releaseJITBlockExecMem(block)
				removed++
				break
			}
		}
	}
	if removed != 0 {
		cc.generation++
		cc.dispatch.reset()
	}
	return removed
}

func (cc *CodeCache) RemoveBlock(target *JITBlock) bool {
	if cc == nil || target == nil {
		return false
	}
	removed := false
	for key, block := range cc.blocks {
		if block == target {
			cc.unpatchChainsToBlock(block)
			cc.unindexBlock(block)
			cc.unregisterChainSlots(block)
			delete(cc.blocks, key)
			releaseJITBlockExecMem(block)
			removed = true
		}
	}
	for key, block := range cc.mmuBlocks {
		if block == target {
			cc.unpatchChainsToBlock(block)
			cc.unindexMMUBlock(key.ptbr, block)
			cc.unregisterChainSlots(block)
			delete(cc.mmuBlocks, key)
			releaseJITBlockExecMem(block)
			removed = true
		}
	}
	if removed {
		cc.generation++
		cc.dispatch.reset()
	}
	return removed
}

func (cc *CodeCache) releaseAllExecMem() {
	if cc == nil {
		return
	}
	for _, block := range cc.blocks {
		releaseJITBlockExecMem(block)
	}
	for _, block := range cc.mmuBlocks {
		releaseJITBlockExecMem(block)
	}
}

// PatchChainsTo scans all cached blocks for chain slots targeting targetPC
// and patches their JMP rel32 to jump to chainEntry.
func (cc *CodeCache) PatchChainsTo(targetPC uint64, chainEntry uintptr) {
	for _, ref := range cc.inboundChainSlots[targetPC] {
		if ref.patchAddr != 0 {
			PatchRel32At(ref.patchAddr, chainEntry)
		}
	}
}

// PatchChainsToScoped is the MMU-aware variant of PatchChainsTo: only
// patches chain slots in source blocks whose ptbr matches the supplied
// scopePtbr. IE64 backends call this when MMU is enabled — without
// the ptbr filter, a block compiled in one address space would cross-
// link to chain slots from another address space sharing the same
// virtual PC, executing the wrong physical code on the next chained
// transition.
func (cc *CodeCache) PatchChainsToScoped(targetPC uint64, chainEntry uintptr, scopePtbr uint64) {
	for _, ref := range cc.inboundChainSlots[targetPC] {
		if ref.ptbr == scopePtbr && ref.patchAddr != 0 {
			PatchRel32At(ref.patchAddr, chainEntry)
		}
	}
}

func (cc *CodeCache) registerChainSlots(block *JITBlock) {
	if block == nil || len(block.chainSlots) == 0 {
		return
	}
	for _, slot := range block.chainSlots {
		if slot.patchAddr == 0 {
			continue
		}
		cc.inboundChainSlots[slot.targetPC] = append(cc.inboundChainSlots[slot.targetPC], chainPatchRef{
			patchAddr: slot.patchAddr,
			ptbr:      block.ptbr,
		})
	}
}

func (cc *CodeCache) unregisterChainSlots(block *JITBlock) {
	if block == nil || len(block.chainSlots) == 0 {
		return
	}
	for _, slot := range block.chainSlots {
		if slot.patchAddr == 0 {
			continue
		}
		refs := cc.inboundChainSlots[slot.targetPC]
		for i := 0; i < len(refs); i++ {
			if refs[i].patchAddr == slot.patchAddr && refs[i].ptbr == block.ptbr {
				refs[i] = refs[len(refs)-1]
				refs = refs[:len(refs)-1]
				i--
			}
		}
		if len(refs) == 0 {
			delete(cc.inboundChainSlots, slot.targetPC)
		} else {
			cc.inboundChainSlots[slot.targetPC] = refs
		}
	}
}

func (cc *CodeCache) unpatchChainsToBlock(target *JITBlock) {
	if cc == nil || target == nil {
		return
	}
	targeted := func(pc uint64) bool {
		if pc == target.startPC {
			return true
		}
		for _, r := range JITBlockCoveredRanges(target) {
			if pc >= r[0] && pc < r[1] {
				return true
			}
		}
		return false
	}
	for _, block := range cc.blocks {
		for _, slot := range block.chainSlots {
			if slot.patchAddr == 0 || !targeted(slot.targetPC) {
				continue
			}
			PatchRel32At(slot.patchAddr, slot.patchAddr+4)
		}
	}
	for _, block := range cc.mmuBlocks {
		for _, slot := range block.chainSlots {
			if slot.patchAddr == 0 || !targeted(slot.targetPC) {
				continue
			}
			PatchRel32At(slot.patchAddr, slot.patchAddr+4)
		}
	}
}

// UnpatchChainsInRange resets chain slots that target any block whose covered
// guest ranges overlap [lo, hi). This must match the same overlap condition
// used by InvalidateRange, so that every block about to be removed has all
// inbound chain jumps reset to their unchained fallback first.
// Must be called BEFORE InvalidateRange.
func (cc *CodeCache) UnpatchChainsInRange(lo, hi uint64) {
	type doomedRange struct {
		lo uint64
		hi uint64
	}
	var doomed []doomedRange
	for _, block := range cc.blocks {
		for _, r := range JITBlockCoveredRanges(block) {
			if r[1] > lo && r[0] < hi {
				doomed = append(doomed, doomedRange{lo: r[0], hi: r[1]})
				break
			}
		}
	}
	for _, block := range cc.mmuBlocks {
		for _, r := range JITBlockCoveredRanges(block) {
			if r[1] > lo && r[0] < hi {
				doomed = append(doomed, doomedRange{lo: r[0], hi: r[1]})
				break
			}
		}
	}
	if len(doomed) == 0 {
		return
	}

	targetDoomed := func(pc uint64) bool {
		for _, r := range doomed {
			if pc >= r.lo && pc < r.hi {
				return true
			}
		}
		return false
	}

	// Unpatch every chain slot in every surviving block that targets a doomed block.
	for _, block := range cc.blocks {
		for _, slot := range block.chainSlots {
			if slot.patchAddr == 0 {
				continue
			}
			if targetDoomed(slot.targetPC) {
				PatchRel32At(slot.patchAddr, slot.patchAddr+4)
			}
		}
	}
	for _, block := range cc.mmuBlocks {
		for _, slot := range block.chainSlots {
			if slot.patchAddr == 0 {
				continue
			}
			if targetDoomed(slot.targetPC) {
				PatchRel32At(slot.patchAddr, slot.patchAddr+4)
			}
		}
	}
}
