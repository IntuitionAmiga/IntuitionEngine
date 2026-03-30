// jit_6502_common.go - 6502 JIT compiler infrastructure: context, scanner, lookup tables

package main

import (
	"unsafe"
)

// ===========================================================================
// JIT6502Context — Bridge between Go and JIT-compiled native code
// ===========================================================================

// JIT6502Context is passed to every JIT-compiled 6502 block as its sole argument.
// On ARM64 it arrives in X0; on x86-64 in RDI.
type JIT6502Context struct {
	MemPtr              uintptr // 0:  &adapter.memDirect[0]
	IOBitmapPtr         uintptr // 8:  &adapter.ioPageBitmap[0] ([]bool, 1 byte per entry)
	CpuPtr              uintptr // 16: &cpu (CPU_6502 struct pointer)
	CodePageBitmapPtr   uintptr // 24: &codePageBitmap[0] (256 bytes, one per 6502 page)
	RetCycles           uint64  // 32: accumulated cycles for this block execution
	NeedBail            uint32  // 40: re-execute current instruction via interpreter
	NeedInval           uint32  // 44: self-modification detected (store completed, invalidate cache)
	RetPC               uint32  // 48: next PC (bail: current instr PC; inval: next instr PC; normal: next PC)
	RetCount            uint32  // 52: instructions retired before bail/exit point
	FastPathLimit       uint32  // 56: 0x2000 (legacy, no longer used in fast path check)
	ChainBudget         uint32  // 60: blocks remaining before returning to Go for interrupt check
	ChainCount          uint32  // 64: accumulated instruction count during chaining
	InvalPage           uint32  // 68: page number that triggered NeedInval (for granular invalidation)
	RTSCache0PC         uint32  // 72: MRU RTS cache entry 0 — 6502 PC
	_pad1               uint32  // 76: alignment padding
	RTSCache0Addr       uintptr // 80: MRU RTS cache entry 0 — chain entry address
	RTSCache1PC         uint32  // 88: MRU RTS cache entry 1 — 6502 PC
	_pad2               uint32  // 92: alignment padding
	RTSCache1Addr       uintptr // 96: MRU RTS cache entry 1 — chain entry address
	DirectPageBitmapPtr uintptr // 104: &directPageBitmap[0] (256 bytes, 0=direct 1=bail)
}

// JIT6502Context field offsets (must match struct layout above)
const (
	j65CtxOffMemPtr              = 0
	j65CtxOffIOBitmapPtr         = 8
	j65CtxOffCpuPtr              = 16
	j65CtxOffCodePageBitmap      = 24
	j65CtxOffRetCycles           = 32
	j65CtxOffNeedBail            = 40
	j65CtxOffNeedInval           = 44
	j65CtxOffRetPC               = 48
	j65CtxOffRetCount            = 52
	j65CtxOffFastPathLimit       = 56
	j65CtxOffChainBudget         = 60
	j65CtxOffChainCount          = 64
	j65CtxOffInvalPage           = 68
	j65CtxOffRTSCache0PC         = 72
	j65CtxOffRTSCache0Addr       = 80
	j65CtxOffRTSCache1PC         = 88
	j65CtxOffRTSCache1Addr       = 96
	j65CtxOffDirectPageBitmapPtr = 104
)

// CPU_6502 struct field offsets (from CpuPtr). Must match cpu_six5go2.go layout.
const (
	cpu6502OffPC = 0 // uint16
	cpu6502OffSP = 2 // byte
	cpu6502OffA  = 3 // byte
	cpu6502OffX  = 4 // byte
	cpu6502OffY  = 5 // byte
	cpu6502OffSR = 6 // byte
)

// jit6502FastPathLimit is the upper bound for Tier 1 direct memory access.
// Addresses below this threshold (and not on I/O pages) use memDirect directly.
const jit6502FastPathLimit = 0x2000

// jit6502Available is set to true at init time on platforms that support 6502 JIT.
var jit6502Available bool

// initDirectPageBitmap computes which 6502 pages can be accessed via memDirect[addr]
// without translation. This must be called after SealMappings() — the bitmap depends
// on post-seal I/O page stability and is never recomputed.
func (cpu *CPU_6502) initDirectPageBitmap() {
	for page := 0; page < 256; page++ {
		direct := true
		if page >= 0x20 && page <= 0x7F {
			direct = false // bank windows ($2000-$7FFF) require translation
		}
		if page >= 0x80 && page <= 0xBF {
			direct = false // VRAM window ($8000-$BFFF) requires translation
		}
		if page >= 0xF0 {
			direct = false // I/O translation region ($F000-$FFFF)
		}
		if cpu.fastAdapter != nil {
			// Check MachineBus ioPageBitmap (set by MapIO/MapIOByte)
			if page < len(cpu.fastAdapter.ioPageBitmap) && cpu.fastAdapter.ioPageBitmap[page] {
				direct = false
			}
			// Check adapter's ioTable (pages with I/O handlers like POKEY, VGA, etc.)
			if cpu.fastAdapter.ioTable[page].read != nil || cpu.fastAdapter.ioTable[page].write != nil {
				direct = false
			}
		}
		if direct {
			cpu.directPageBitmap[page] = 0
		} else {
			cpu.directPageBitmap[page] = 1
		}
	}
}

func newJIT6502Context(cpu *CPU_6502) *JIT6502Context {
	if cpu.fastAdapter == nil {
		return nil
	}
	cpu.initDirectPageBitmap()
	ctx := &JIT6502Context{
		MemPtr:        uintptr(unsafe.Pointer(&cpu.fastAdapter.memDirect[0])),
		CpuPtr:        uintptr(unsafe.Pointer(cpu)),
		FastPathLimit: jit6502FastPathLimit,
	}
	if len(cpu.fastAdapter.ioPageBitmap) > 0 {
		ctx.IOBitmapPtr = uintptr(unsafe.Pointer(&cpu.fastAdapter.ioPageBitmap[0]))
	}
	ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&cpu.codePageBitmap[0]))
	ctx.DirectPageBitmapPtr = uintptr(unsafe.Pointer(&cpu.directPageBitmap[0]))
	return ctx
}

// ===========================================================================
// JIT6502Instr — Pre-decoded 6502 instruction for JIT compilation
// ===========================================================================

type JIT6502Instr struct {
	opcode   byte   // raw opcode byte
	operand  uint16 // 0, 1, or 2 byte operand (zero-extended)
	length   byte   // 1, 2, or 3
	pcOffset uint16 // byte offset from block start
}

// ===========================================================================
// Instruction Length Table
// ===========================================================================

// jit6502InstrLengths maps each opcode to its instruction length in bytes (1, 2, or 3).
// Derived from the 6502 addressing mode for each opcode.
var jit6502InstrLengths = [256]byte{
	// 0x00-0x0F
	1, 2, 1, 2, 2, 2, 2, 2, 1, 2, 1, 2, 3, 3, 3, 3,
	// 0x10-0x1F
	2, 2, 1, 2, 2, 2, 2, 2, 1, 3, 1, 3, 3, 3, 3, 3,
	// 0x20-0x2F
	3, 2, 1, 2, 2, 2, 2, 2, 1, 2, 1, 2, 3, 3, 3, 3,
	// 0x30-0x3F
	2, 2, 1, 2, 2, 2, 2, 2, 1, 3, 1, 3, 3, 3, 3, 3,
	// 0x40-0x4F
	1, 2, 1, 2, 2, 2, 2, 2, 1, 2, 1, 2, 3, 3, 3, 3,
	// 0x50-0x5F
	2, 2, 1, 2, 2, 2, 2, 2, 1, 3, 1, 3, 3, 3, 3, 3,
	// 0x60-0x6F
	1, 2, 1, 2, 2, 2, 2, 2, 1, 2, 1, 2, 3, 3, 3, 3,
	// 0x70-0x7F
	2, 2, 1, 2, 2, 2, 2, 2, 1, 3, 1, 3, 3, 3, 3, 3,
	// 0x80-0x8F
	2, 2, 2, 2, 2, 2, 2, 2, 1, 2, 1, 2, 3, 3, 3, 3,
	// 0x90-0x9F
	2, 2, 1, 2, 2, 2, 2, 2, 1, 3, 1, 3, 3, 3, 3, 3,
	// 0xA0-0xAF
	2, 2, 2, 2, 2, 2, 2, 2, 1, 2, 1, 2, 3, 3, 3, 3,
	// 0xB0-0xBF
	2, 2, 1, 2, 2, 2, 2, 2, 1, 3, 1, 3, 3, 3, 3, 3,
	// 0xC0-0xCF
	2, 2, 2, 2, 2, 2, 2, 2, 1, 2, 1, 2, 3, 3, 3, 3,
	// 0xD0-0xDF
	2, 2, 1, 2, 2, 2, 2, 2, 1, 3, 1, 3, 3, 3, 3, 3,
	// 0xE0-0xEF
	2, 2, 2, 2, 2, 2, 2, 2, 1, 2, 1, 2, 3, 3, 3, 3,
	// 0xF0-0xFF
	2, 2, 1, 2, 2, 2, 2, 2, 1, 3, 1, 3, 3, 3, 3, 3,
}

// ===========================================================================
// Base Cycle Cost Table
// ===========================================================================

// jit6502BaseCycles maps each opcode to its base cycle cost (excluding variable
// page-crossing penalties). These values match the interpreter's Cycles += N
// in each opcode handler exactly.
//
// Branch opcodes ($10,$30,$50,$70,$90,$B0,$D0,$F0) have base cost 0 because
// the interpreter's branch() function adds no base cycles — only +1 for taken
// and +1 for page cross (cpu_six5go2.go:1377-1402).
//
// JAM/KIL opcodes have 0 (they halt the CPU).
var jit6502BaseCycles = [256]byte{
	// 0x00-0x0F: BRK=7, ORA(ind,X)=6, JAM=0, SLO(ind,X)=8, SKB=3, ORA zp=3, ASL zp=5, SLO zp=5, PHP=3, ORA imm=2, ASL A=2, ANC=2, SKW=4, ORA abs=4, ASL abs=6, SLO abs=6
	7, 6, 0, 8, 3, 3, 5, 5, 3, 2, 2, 2, 4, 4, 6, 6,
	// 0x10-0x1F: BPL=0, ORA(ind),Y=5, JAM=0, SLO(ind),Y=8, SKB=4, ORA zp,X=4, ASL zp,X=6, SLO zp,X=6, CLC=2, ORA abs,Y=4, NOP=2, SLO abs,Y=7, SKW abs,X=4, ORA abs,X=4, ASL abs,X=7, SLO abs,X=7
	0, 5, 0, 8, 4, 4, 6, 6, 2, 4, 2, 7, 4, 4, 7, 7,
	// 0x20-0x2F: JSR=6, AND(ind,X)=6, JAM=0, RLA(ind,X)=8, BIT zp=3, AND zp=3, ROL zp=5, RLA zp=5, PLP=4, AND imm=2, ROL A=2, ANC=2, BIT abs=4, AND abs=4, ROL abs=6, RLA abs=6
	6, 6, 0, 8, 3, 3, 5, 5, 4, 2, 2, 2, 4, 4, 6, 6,
	// 0x30-0x3F: BMI=0, AND(ind),Y=5, JAM=0, RLA(ind),Y=8, SKB=4, AND zp,X=4, ROL zp,X=6, RLA zp,X=6, SEC=2, AND abs,Y=4, NOP=2, RLA abs,Y=7, SKW abs,X=4, AND abs,X=4, ROL abs,X=7, RLA abs,X=7
	0, 5, 0, 8, 4, 4, 6, 6, 2, 4, 2, 7, 4, 4, 7, 7,
	// 0x40-0x4F: RTI=6, EOR(ind,X)=6, JAM=0, SRE(ind,X)=8, SKB=3, EOR zp=3, LSR zp=5, SRE zp=5, PHA=3, EOR imm=2, LSR A=2, ALR=2, JMP abs=3, EOR abs=4, LSR abs=6, SRE abs=6
	6, 6, 0, 8, 3, 3, 5, 5, 3, 2, 2, 2, 3, 4, 6, 6,
	// 0x50-0x5F: BVC=0, EOR(ind),Y=5, JAM=0, SRE(ind),Y=8, SKB=4, EOR zp,X=4, LSR zp,X=6, SRE zp,X=6, CLI=2, EOR abs,Y=4, NOP=2, SRE abs,Y=7, SKW abs,X=4, EOR abs,X=4, LSR abs,X=7, SRE abs,X=7
	0, 5, 0, 8, 4, 4, 6, 6, 2, 4, 2, 7, 4, 4, 7, 7,
	// 0x60-0x6F: RTS=6, ADC(ind,X)=6, JAM=0, RRA(ind,X)=8, SKB=3, ADC zp=3, ROR zp=5, RRA zp=5, PLA=4, ADC imm=2, ROR A=2, ARR=2, JMP ind=5, ADC abs=4, ROR abs=6, RRA abs=6
	6, 6, 0, 8, 3, 3, 5, 5, 4, 2, 2, 2, 5, 4, 6, 6,
	// 0x70-0x7F: BVS=0, ADC(ind),Y=5, JAM=0, RRA(ind),Y=8, SKB=4, ADC zp,X=4, ROR zp,X=6, RRA zp,X=6, SEI=2, ADC abs,Y=4, NOP=2, RRA abs,Y=7, SKW abs,X=4, ADC abs,X=4, ROR abs,X=7, RRA abs,X=7
	0, 5, 0, 8, 4, 4, 6, 6, 2, 4, 2, 7, 4, 4, 7, 7,
	// 0x80-0x8F: SKB=2, STA(ind,X)=6, SKB=2, SAX(ind,X)=6, STY zp=3, STA zp=3, STX zp=3, SAX zp=3, DEY=2, SKB=2, TXA=2, XAA=2, STY abs=4, STA abs=4, STX abs=4, SAX abs=4
	2, 6, 2, 6, 3, 3, 3, 3, 2, 2, 2, 2, 4, 4, 4, 4,
	// 0x90-0x9F: BCC=0, STA(ind),Y=6, JAM=0, SHA(ind),Y=6, STY zp,X=4, STA zp,X=4, STX zp,Y=4, SAX zp,Y=4, TYA=2, STA abs,Y=5, TXS=2, SHS abs,Y=5, SHY abs,X=5, STA abs,X=5, SHX abs,Y=5, SHA abs,Y=5
	0, 6, 0, 6, 4, 4, 4, 4, 2, 5, 2, 5, 5, 5, 5, 5,
	// 0xA0-0xAF: LDY imm=2, LDA(ind,X)=6, LDX imm=2, LAX(ind,X)=6, LDY zp=3, LDA zp=3, LDX zp=3, LAX zp=3, TAY=2, LDA imm=2, TAX=2, LAX imm=2, LDY abs=4, LDA abs=4, LDX abs=4, LAX abs=4
	2, 6, 2, 6, 3, 3, 3, 3, 2, 2, 2, 2, 4, 4, 4, 4,
	// 0xB0-0xBF: BCS=0, LDA(ind),Y=5, JAM=0, LAX(ind),Y=5, LDY zp,X=4, LDA zp,X=4, LDX zp,Y=4, LAX zp,Y=4, CLV=2, LDA abs,Y=4, TSX=2, LAS abs,Y=4, LDY abs,X=4, LDA abs,X=4, LDX abs,Y=4, LAX abs,Y=4
	0, 5, 0, 5, 4, 4, 4, 4, 2, 4, 2, 4, 4, 4, 4, 4,
	// 0xC0-0xCF: CPY imm=2, CMP(ind,X)=6, SKB=2, DCP(ind,X)=8, CPY zp=3, CMP zp=3, DEC zp=5, DCP zp=5, INY=2, CMP imm=2, DEX=2, SBX=2, CPY abs=4, CMP abs=4, DEC abs=6, DCP abs=6
	2, 6, 2, 8, 3, 3, 5, 5, 2, 2, 2, 2, 4, 4, 6, 6,
	// 0xD0-0xDF: BNE=0, CMP(ind),Y=5, JAM=0, DCP(ind),Y=8, SKB=4, CMP zp,X=4, DEC zp,X=6, DCP zp,X=6, CLD=2, CMP abs,Y=4, NOP=2, DCP abs,Y=7, SKW abs,X=4, CMP abs,X=4, DEC abs,X=7, DCP abs,X=7
	0, 5, 0, 8, 4, 4, 6, 6, 2, 4, 2, 7, 4, 4, 7, 7,
	// 0xE0-0xEF: CPX imm=2, SBC(ind,X)=6, SKB=2, ISC(ind,X)=8, CPX zp=3, SBC zp=3, INC zp=5, ISC zp=5, INX=2, SBC imm=2, NOP=2, USBC=2, CPX abs=4, SBC abs=4, INC abs=6, ISC abs=6
	2, 6, 2, 8, 3, 3, 5, 5, 2, 2, 2, 2, 4, 4, 6, 6,
	// 0xF0-0xFF: BEQ=0, SBC(ind),Y=5, JAM=0, ISC(ind),Y=8, SKB=4, SBC zp,X=4, INC zp,X=6, ISC zp,X=6, SED=2, SBC abs,Y=4, NOP=2, ISC abs,Y=7, SKW abs,X=4, SBC abs,X=4, INC abs,X=7, ISC abs,X=7
	0, 5, 0, 8, 4, 4, 6, 6, 2, 4, 2, 7, 4, 4, 7, 7,
}

// ===========================================================================
// Compilable Opcode Table
// ===========================================================================

// jit6502IsCompilable marks which opcodes the JIT can compile natively.
// True for all documented opcodes except BRK ($00) and RTI ($40).
// False for all undocumented/illegal opcodes.
var jit6502IsCompilable = [256]bool{
	// 0x00-0x0F: BRK=no, ORA(ind,X)=yes, JAM=no, SLO=no, SKB=no, ORA zp=yes, ASL zp=yes, SLO=no, PHP=yes, ORA imm=yes, ASL A=yes, ANC=no, SKW=no, ORA abs=yes, ASL abs=yes, SLO=no
	false, true, false, false, false, true, true, false, true, true, true, false, false, true, true, false,
	// 0x10-0x1F: BPL=yes, ORA(ind),Y=yes, JAM=no, SLO=no, SKB=no, ORA zp,X=yes, ASL zp,X=yes, SLO=no, CLC=yes, ORA abs,Y=yes, NOP=no, SLO=no, SKW=no, ORA abs,X=yes, ASL abs,X=yes, SLO=no
	true, true, false, false, false, true, true, false, true, true, false, false, false, true, true, false,
	// 0x20-0x2F: JSR=yes, AND(ind,X)=yes, JAM=no, RLA=no, BIT zp=yes, AND zp=yes, ROL zp=yes, RLA=no, PLP=yes, AND imm=yes, ROL A=yes, ANC=no, BIT abs=yes, AND abs=yes, ROL abs=yes, RLA=no
	true, true, false, false, true, true, true, false, true, true, true, false, true, true, true, false,
	// 0x30-0x3F: BMI=yes, AND(ind),Y=yes, JAM=no, RLA=no, SKB=no, AND zp,X=yes, ROL zp,X=yes, RLA=no, SEC=yes, AND abs,Y=yes, NOP=no, RLA=no, SKW=no, AND abs,X=yes, ROL abs,X=yes, RLA=no
	true, true, false, false, false, true, true, false, true, true, false, false, false, true, true, false,
	// 0x40-0x4F: RTI=no, EOR(ind,X)=yes, JAM=no, SRE=no, SKB=no, EOR zp=yes, LSR zp=yes, SRE=no, PHA=yes, EOR imm=yes, LSR A=yes, ALR=no, JMP abs=yes, EOR abs=yes, LSR abs=yes, SRE=no
	false, true, false, false, false, true, true, false, true, true, true, false, true, true, true, false,
	// 0x50-0x5F: BVC=yes, EOR(ind),Y=yes, JAM=no, SRE=no, SKB=no, EOR zp,X=yes, LSR zp,X=yes, SRE=no, CLI=yes, EOR abs,Y=yes, NOP=no, SRE=no, SKW=no, EOR abs,X=yes, LSR abs,X=yes, SRE=no
	true, true, false, false, false, true, true, false, true, true, false, false, false, true, true, false,
	// 0x60-0x6F: RTS=yes, ADC(ind,X)=yes, JAM=no, RRA=no, SKB=no, ADC zp=yes, ROR zp=yes, RRA=no, PLA=yes, ADC imm=yes, ROR A=yes, ARR=no, JMP ind=yes, ADC abs=yes, ROR abs=yes, RRA=no
	true, true, false, false, false, true, true, false, true, true, true, false, true, true, true, false,
	// 0x70-0x7F: BVS=yes, ADC(ind),Y=yes, JAM=no, RRA=no, SKB=no, ADC zp,X=yes, ROR zp,X=yes, RRA=no, SEI=yes, ADC abs,Y=yes, NOP=no, RRA=no, SKW=no, ADC abs,X=yes, ROR abs,X=yes, RRA=no
	true, true, false, false, false, true, true, false, true, true, false, false, false, true, true, false,
	// 0x80-0x8F: SKB=no, STA(ind,X)=yes, SKB=no, SAX=no, STY zp=yes, STA zp=yes, STX zp=yes, SAX=no, DEY=yes, SKB=no, TXA=yes, XAA=no, STY abs=yes, STA abs=yes, STX abs=yes, SAX=no
	false, true, false, false, true, true, true, false, true, false, true, false, true, true, true, false,
	// 0x90-0x9F: BCC=yes, STA(ind),Y=yes, JAM=no, SHA=no, STY zp,X=yes, STA zp,X=yes, STX zp,Y=yes, SAX=no, TYA=yes, STA abs,Y=yes, TXS=yes, SHS=no, SHY=no, STA abs,X=yes, SHX=no, SHA=no
	true, true, false, false, true, true, true, false, true, true, true, false, false, true, false, false,
	// 0xA0-0xAF: LDY imm=yes, LDA(ind,X)=yes, LDX imm=yes, LAX=no, LDY zp=yes, LDA zp=yes, LDX zp=yes, LAX=no, TAY=yes, LDA imm=yes, TAX=yes, LAX=no, LDY abs=yes, LDA abs=yes, LDX abs=yes, LAX=no
	true, true, true, false, true, true, true, false, true, true, true, false, true, true, true, false,
	// 0xB0-0xBF: BCS=yes, LDA(ind),Y=yes, JAM=no, LAX=no, LDY zp,X=yes, LDA zp,X=yes, LDX zp,Y=yes, LAX=no, CLV=yes, LDA abs,Y=yes, TSX=yes, LAS=no, LDY abs,X=yes, LDA abs,X=yes, LDX abs,Y=yes, LAX=no
	true, true, false, false, true, true, true, false, true, true, true, false, true, true, true, false,
	// 0xC0-0xCF: CPY imm=yes, CMP(ind,X)=yes, SKB=no, DCP=no, CPY zp=yes, CMP zp=yes, DEC zp=yes, DCP=no, INY=yes, CMP imm=yes, DEX=yes, SBX=no, CPY abs=yes, CMP abs=yes, DEC abs=yes, DCP=no
	true, true, false, false, true, true, true, false, true, true, true, false, true, true, true, false,
	// 0xD0-0xDF: BNE=yes, CMP(ind),Y=yes, JAM=no, DCP=no, SKB=no, CMP zp,X=yes, DEC zp,X=yes, DCP=no, CLD=yes, CMP abs,Y=yes, NOP=no, DCP=no, SKW=no, CMP abs,X=yes, DEC abs,X=yes, DCP=no
	true, true, false, false, false, true, true, false, true, true, false, false, false, true, true, false,
	// 0xE0-0xEF: CPX imm=yes, SBC(ind,X)=yes, SKB=no, ISC=no, CPX zp=yes, SBC zp=yes, INC zp=yes, ISC=no, INX=yes, SBC imm=yes, NOP=yes, USBC=no, CPX abs=yes, SBC abs=yes, INC abs=yes, ISC=no
	true, true, false, false, true, true, true, false, true, true, true, false, true, true, true, false,
	// 0xF0-0xFF: BEQ=yes, SBC(ind),Y=yes, JAM=no, ISC=no, SKB=no, SBC zp,X=yes, INC zp,X=yes, ISC=no, SED=yes, SBC abs,Y=yes, NOP=no, ISC=no, SKW=no, SBC abs,X=yes, INC abs,X=yes, ISC=no
	true, true, false, false, false, true, true, false, true, true, false, false, false, true, true, false,
}

// ===========================================================================
// Block Scanner
// ===========================================================================

const jit6502MaxBlockSize = 128

// jit6502IsBlockTerminator returns true if the opcode ends a basic block.
func jit6502IsBlockTerminator(opcode byte) bool {
	switch opcode {
	case 0x4C, 0x6C: // JMP absolute, JMP indirect
		return true
	case 0x20: // JSR
		return true
	case 0x60: // RTS
		return true
	case 0x40: // RTI
		return true
	case 0x00: // BRK
		return true
	case 0x02, 0x12, 0x22, 0x32, 0x42, 0x52, 0x62, 0x72,
		0x92, 0xB2, 0xD2, 0xF2: // KIL/JAM opcodes
		return true
	}
	return false
}

// jit6502ScanBlock decodes 6502 instructions starting at startPC until a block
// terminator is found, an uncompilable opcode is encountered, or the max block
// size is reached. Block terminators ARE included in the returned block.
// Uncompilable opcodes cause the block to end BEFORE them (not included).
func jit6502ScanBlock(mem []byte, startPC uint16, memSize int) []JIT6502Instr {
	instrs := make([]JIT6502Instr, 0, 32)
	pc := startPC

	for len(instrs) < jit6502MaxBlockSize {
		if int(pc) >= memSize {
			break
		}

		opcode := mem[pc]
		length := jit6502InstrLengths[opcode]

		// Check if we have enough bytes for the full instruction
		if int(pc)+int(length) > memSize {
			break
		}

		// If this opcode is not compilable and not a block terminator,
		// end the block before it (don't include it)
		if !jit6502IsCompilable[opcode] && !jit6502IsBlockTerminator(opcode) {
			break
		}

		instr := JIT6502Instr{
			opcode:   opcode,
			length:   length,
			pcOffset: pc - startPC,
		}

		// Read operand bytes
		switch length {
		case 2:
			instr.operand = uint16(mem[pc+1])
		case 3:
			instr.operand = uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
		}

		instrs = append(instrs, instr)
		pc += uint16(length)

		// Block terminators end the block (but are included)
		if jit6502IsBlockTerminator(opcode) {
			break
		}

		// If the next opcode is not compilable and not a terminator,
		// the block naturally ends here on the next iteration
	}

	return instrs
}

// jit6502NeedsFallback returns true if the first instruction in a scanned block
// cannot be JIT-compiled and should fall back to the interpreter.
func jit6502NeedsFallback(instrs []JIT6502Instr) bool {
	if len(instrs) == 0 {
		return true
	}
	opcode := instrs[0].opcode
	// BRK, RTI, KIL — always fall back
	if opcode == 0x00 || opcode == 0x40 {
		return true
	}
	// KIL opcodes
	switch opcode {
	case 0x02, 0x12, 0x22, 0x32, 0x42, 0x52, 0x62, 0x72,
		0x92, 0xB2, 0xD2, 0xF2:
		return true
	}
	// Any non-compilable opcode at position 0
	return !jit6502IsCompilable[opcode]
}

// jit6502DetectBackwardBranches returns true if any conditional branch in the
// block targets an earlier instruction within the block.
func jit6502DetectBackwardBranches(instrs []JIT6502Instr, startPC uint16) bool {
	for _, instr := range instrs {
		switch instr.opcode {
		case 0x10, 0x30, 0x50, 0x70, 0x90, 0xB0, 0xD0, 0xF0: // BPL,BMI,BVC,BVS,BCC,BCS,BNE,BEQ
			// Operand is a signed 8-bit offset from the instruction AFTER the branch
			branchPC := startPC + instr.pcOffset + 2 // PC after fetching the branch
			offset := int8(instr.operand & 0xFF)
			target := int(branchPC) + int(offset)
			if target >= int(startPC) && target < int(startPC+instr.pcOffset) {
				return true
			}
		}
	}
	return false
}
