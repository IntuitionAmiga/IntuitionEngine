// jit_x86_wasm_emit.go - pure-Go x86 js/wasm module builder helpers.
//
// This file deliberately holds only the backend-independent wasm bytecode
// construction so native tests can validate emitted x86 wasm blocks under
// wazero even when the host toolchain cannot execute GOOS=js binaries.

package main

import "fmt"

const (
	x86WasmDriverCacheEntries = 4096
	x86WasmDriverTableInitial = 256
	x86WasmChainBudget        = 256
)

type x86WasmCompiledModule struct {
	module []byte
	block  *JITBlock
	retPC  uint32
}

func x86WasmImmediate8(ji X86JITInstr, memory []byte) byte {
	return memory[ji.opcodePC+uint32(ji.length)-1]
}

func x86WasmImmediate32(ji X86JITInstr, memory []byte) uint32 {
	immPC := ji.opcodePC + uint32(ji.length) - 4
	return uint32(memory[immPC]) |
		uint32(memory[immPC+1])<<8 |
		uint32(memory[immPC+2])<<16 |
		uint32(memory[immPC+3])<<24
}

func x86WasmIsShortJcc(op byte) bool {
	return op >= 0x70 && op <= 0x7F
}

func x86WasmSupportedInstr(ji X86JITInstr) bool {
	if ji.opcode >= 0x0F00 {
		op2 := byte(ji.opcode)
		switch op2 {
		case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47,
			0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F:
			return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0
		case 0xB6, 0xB7, 0xBE, 0xBF:
			if !ji.hasModRM {
				return false
			}
			if ji.modrm>>6 == 3 {
				return true
			}
			return ji.prefixes == 0
		case 0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97,
			0x98, 0x99, 0x9A, 0x9B, 0x9C, 0x9D, 0x9E, 0x9F:
			return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0
		case 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF:
			return true
		}
		return false
	}
	op := byte(ji.opcode)
	switch {
	case op == 0x90 || op == 0x9B:
		return true
	case op >= 0x91 && op <= 0x97:
		return true
	case op >= 0xB8 && op <= 0xBF:
		return true
	case op >= 0xB0 && op <= 0xB7:
		return true
	case op >= 0x40 && op <= 0x4F:
		return ji.prefixes == 0
	case op == 0xD1:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0 && (ji.grpOp == 4 || ji.grpOp == 5 || ji.grpOp == 7)
	case op == 0xC1 || op == 0xD3:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0 && (ji.grpOp == 4 || ji.grpOp == 5 || ji.grpOp == 7)
	case op == 0x87:
		return ji.hasModRM && ji.modrm>>6 == 3
	case op == 0xF6:
		return ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0 && (ji.grpOp == 0 || ji.grpOp == 1)
	case op == 0xF7:
		return ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0 && (ji.grpOp == 0 || ji.grpOp == 1)
	case op == 0x81 || op == 0x83:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0 &&
			(ji.grpOp == 0 || ji.grpOp == 1 || ji.grpOp == 4 || ji.grpOp == 5 || ji.grpOp == 6 || ji.grpOp == 7)
	case op == 0x01 || op == 0x03 || op == 0x05 || op == 0x29 || op == 0x2B || op == 0x2D:
		if op == 0x05 || op == 0x2D {
			return ji.prefixes == 0
		}
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0
	case op == 0x85:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0
	case op == 0x09 || op == 0x0B || op == 0x21 || op == 0x23 || op == 0x31 || op == 0x33 || op == 0x39 || op == 0x3B:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0
	case op == 0x89 || op == 0x8B:
		if !ji.hasModRM {
			return false
		}
		if ji.modrm>>6 == 3 {
			return true
		}
		return ji.prefixes == 0
	case op == 0xEB || op == 0xE9:
		return true
	case x86WasmIsShortJcc(op):
		return ji.prefixes == 0
	}
	return false
}

func x86WasmBlockTerminalPC(instrs []X86JITInstr, memory []byte, startPC uint32) (uint32, bool) {
	if len(instrs) == 0 {
		return 0, false
	}
	for i, ji := range instrs {
		op := byte(ji.opcode)
		if !x86WasmSupportedInstr(ji) {
			return 0, false
		}
		if ji.opcode >= 0x0F00 {
			switch op {
			case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47,
				0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F,
				0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97,
				0x98, 0x99, 0x9A, 0x9B, 0x9C, 0x9D, 0x9E, 0x9F,
				0xB6, 0xB7, 0xBE, 0xBF, 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF:
				continue
			default:
				return 0, false
			}
		}
		switch op {
		case 0x90, 0x9B, 0x89, 0x8B: // NOP, WAIT, register MOVs
			continue
		case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47,
			0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F:
			continue
		case 0xC1, 0xD1, 0xD3:
			continue
		case 0xF6, 0xF7:
			continue
		case 0x81, 0x83:
			continue
		case 0x01, 0x03, 0x05, 0x09, 0x0B, 0x21, 0x23, 0x29, 0x2B, 0x2D, 0x31, 0x33, 0x39, 0x3B:
			continue
		case 0x85: // TEST r/m32, r32 (register only)
			continue
		case 0x87:
			continue
		case 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97:
			continue
		case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF:
			continue
		case 0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7:
			continue
		case 0xE9, 0xEB: // JMP rel32 / rel8
			if i != len(instrs)-1 {
				return 0, false
			}
			target, ok := x86ResolveTerminatorTarget(&ji, memory, startPC)
			if !ok {
				return 0, false
			}
			return target, true
		case 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77,
			0x78, 0x79, 0x7A, 0x7B, 0x7C, 0x7D, 0x7E, 0x7F:
			if i != len(instrs)-1 {
				return 0, false
			}
			return ji.opcodePC + uint32(ji.length), true
		default:
			return 0, false
		}
	}
	last := instrs[len(instrs)-1]
	return last.opcodePC + uint32(last.length), true
}

type x86WasmBlockTerminalKind int

const (
	x86WasmTerminalFallthrough x86WasmBlockTerminalKind = iota
	x86WasmTerminalJMP
	x86WasmTerminalJcc
)

func x86WasmSelectCompiledPrefix(instrs []X86JITInstr, memory []byte, startPC uint32) ([]X86JITInstr, uint32, uint32, x86WasmBlockTerminalKind, bool) {
	if len(instrs) == 0 {
		return nil, 0, 0, x86WasmTerminalFallthrough, false
	}
	spanEnd := startPC
	for i, ji := range instrs {
		if !x86WasmSupportedInstr(ji) {
			if i == 0 {
				return nil, 0, 0, x86WasmTerminalFallthrough, false
			}
			return instrs[:i], ji.opcodePC, spanEnd, x86WasmTerminalFallthrough, true
		}
		op := byte(ji.opcode)
		spanEnd = ji.opcodePC + uint32(ji.length)
		switch {
		case op == 0xE9 || op == 0xEB:
			target, ok := x86ResolveTerminatorTarget(&ji, memory, startPC)
			if !ok {
				return nil, 0, 0, x86WasmTerminalFallthrough, false
			}
			return instrs[:i+1], target, spanEnd, x86WasmTerminalJMP, true
		case x86WasmIsShortJcc(op):
			return instrs[:i+1], spanEnd, spanEnd, x86WasmTerminalJcc, true
		}
	}
	return instrs, spanEnd, spanEnd, x86WasmTerminalFallthrough, true
}

func x86WasmEmitEA32(b *wasmBody, ji X86JITInstr, memory []byte, locRegs, locEA uint32) bool {
	if !ji.hasModRM || ji.prefixes&x86PrefAddrSize != 0 {
		return false
	}
	mod, rm := ji.modrm>>6, ji.modrm&7
	if mod == 3 {
		return false
	}
	modrmPC := x86FindModRMPC(&ji, memory)
	if modrmPC >= uint32(len(memory)) {
		return false
	}
	pos := int(modrmPC) + 1
	if rm == 4 {
		if pos >= len(memory) {
			return false
		}
		sib := memory[pos]
		pos++
		base, index, scale := sib&7, (sib>>3)&7, sib>>6
		if mod == 0 && base == 5 {
			b.i32Const(0)
			b.localSet(locEA)
		} else {
			x86WasmEmitLoadReg32(b, locRegs, base)
			b.localSet(locEA)
		}
		if index != 4 {
			b.localGet(locEA)
			x86WasmEmitLoadReg32(b, locRegs, index)
			if scale != 0 {
				b.i32Const(int32(scale))
				b.op(wasmOpI32Shl)
			}
			b.op(wasmOpI32Add)
			b.localSet(locEA)
		}
	} else if mod == 0 && rm == 5 {
		b.i32Const(0)
		b.localSet(locEA)
	} else {
		x86WasmEmitLoadReg32(b, locRegs, rm)
		b.localSet(locEA)
	}
	dispBytes := 0
	if mod == 1 {
		dispBytes = 1
	} else if mod == 2 || (mod == 0 && (rm == 5 || (rm == 4 && pos-1 < len(memory) && memory[pos-1]&7 == 5))) {
		dispBytes = 4
	}
	if pos+dispBytes > len(memory) {
		return false
	}
	var disp uint32
	if dispBytes == 1 {
		disp = uint32(int32(int8(memory[pos])))
	} else if dispBytes == 4 {
		disp = readLE32(memory, uint32(pos))
	}
	if disp != 0 {
		b.localGet(locEA)
		b.i32Const(int32(disp))
		b.op(wasmOpI32Add)
		b.localSet(locEA)
	}
	return true
}

func x86WasmEmitLoadReg32(b *wasmBody, locRegs uint32, reg byte) {
	b.localGet(locRegs)
	b.i32Const(int32(reg) * 4)
	b.op(wasmOpI32Add)
	b.i32Load(2, 0)
}

func x86WasmEmitStoreReg32(b *wasmBody, locRegs, locTmp uint32, reg byte) {
	b.localSet(locTmp)
	b.localGet(locRegs)
	b.i32Const(int32(reg) * 4)
	b.op(wasmOpI32Add)
	b.localGet(locTmp)
	b.i32Store(2, 0)
}

func x86WasmEmitExtractReg8(b *wasmBody, locRegs uint32, reg byte) {
	idx, shift := x86JITReg8Index(reg)
	x86WasmEmitLoadReg32(b, locRegs, byte(idx))
	if shift != 0 {
		b.i32Const(int32(shift))
		b.op(wasmOpI32ShrU)
	}
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
}

func x86WasmEmitInsertReg8(b *wasmBody, locRegs, locTmp uint32, reg byte) {
	idx, shift := x86JITReg8Index(reg)
	mask := ^(uint32(0xFF) << shift)
	b.localSet(locTmp)
	x86WasmEmitLoadReg32(b, locRegs, byte(idx))
	b.i32Const(int32(mask))
	b.op(wasmOpI32And)
	b.localGet(locTmp)
	if shift != 0 {
		b.i32Const(int32(shift))
		b.op(wasmOpI32Shl)
	}
	b.op(wasmOpI32Or)
	x86WasmEmitStoreReg32(b, locRegs, locTmp, byte(idx))
}

func x86WasmEmitInsertReg16(b *wasmBody, locRegs, locTmp uint32, reg byte) {
	b.localSet(locTmp)
	x86WasmEmitLoadReg32(b, locRegs, reg)
	b.i32Const(^int32(0xFFFF))
	b.op(wasmOpI32And)
	b.localGet(locTmp)
	b.i32Const(0xFFFF)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)
	x86WasmEmitStoreReg32(b, locRegs, locTmp, reg)
}

func x86WasmEmitByteSwap32(b *wasmBody, locRegs, locTmp uint32, reg byte) {
	x86WasmEmitLoadReg32(b, locRegs, reg)
	b.i32Const(24)
	b.op(wasmOpI32ShrU)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)

	x86WasmEmitLoadReg32(b, locRegs, reg)
	b.i32Const(8)
	b.op(wasmOpI32ShrU)
	b.i32Const(0xFF00)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)

	x86WasmEmitLoadReg32(b, locRegs, reg)
	b.i32Const(8)
	b.op(wasmOpI32Shl)
	b.i32Const(0xFF0000)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)

	x86WasmEmitLoadReg32(b, locRegs, reg)
	b.i32Const(24)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	x86WasmEmitStoreReg32(b, locRegs, locTmp, reg)
}

func x86WasmEmitBailReturn(b *wasmBody, locCtx, locEA uint32, retPC uint32, retCount int, mmio bool, invalSize uint32) {
	b.localGet(locCtx)
	b.i32Const(int32(retPC))
	b.i32Store(2, x86CtxOffRetPC)
	b.localGet(locCtx)
	b.i32Const(int32(retCount))
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffChainCount)
	b.op(wasmOpI32Add)
	b.i32Store(2, x86CtxOffRetCount)
	if invalSize != 0 {
		b.localGet(locCtx)
		b.localGet(locEA)
		b.i32Store(2, x86CtxOffInvalAddr)
		b.localGet(locCtx)
		b.i32Const(int32(invalSize))
		b.i32Store(2, x86CtxOffInvalSize)
		b.localGet(locCtx)
		b.i32Const(1)
		b.i32Store(2, x86CtxOffNeedInval)
	} else if mmio {
		b.localGet(locCtx)
		b.i32Const(1)
		b.i32Store(2, x86CtxOffNeedIOFallback)
	}
	b.op(wasmOpReturn)
}

func x86WasmEmitSpanGuard(b *wasmBody, locCtx, locEA, locPtr, locTmp uint32, size uint32, retPC uint32, retCount int) {
	b.localGet(locEA)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.i32Const(int32(0x100 - size))
	b.op(wasmOpI32GtU)
	b.ifVoid()
	x86WasmEmitBailReturn(b, locCtx, locEA, retPC, retCount, true, 0)
	b.end()

	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffMemSize)
	b.localSet(locTmp)
	b.localGet(locTmp)
	b.i32Const(int32(size))
	b.op(wasmOpI32LtU)
	b.ifVoid()
	x86WasmEmitBailReturn(b, locCtx, locEA, retPC, retCount, true, 0)
	b.end()

	b.localGet(locEA)
	b.localGet(locTmp)
	b.i32Const(int32(size))
	b.op(wasmOpI32Sub)
	b.op(wasmOpI32GtU)
	b.ifVoid()
	x86WasmEmitBailReturn(b, locCtx, locEA, retPC, retCount, true, 0)
	b.end()

	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffIOBitmapPtr)
	b.localSet(locPtr)
	b.localGet(locPtr)
	b.ifVoid()
	b.localGet(locPtr)
	b.localGet(locEA)
	b.i32Const(8)
	b.op(wasmOpI32ShrU)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.ifVoid()
	x86WasmEmitBailReturn(b, locCtx, locEA, retPC, retCount, true, 0)
	b.end()
	b.end()
}

func x86WasmEmitSMCStoreCheck(b *wasmBody, locCtx, locEA, locPtr uint32, size uint32, nextPC uint32, retCount int) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffCodePageBitmapPtr)
	b.localSet(locPtr)
	b.localGet(locPtr)
	b.ifVoid()
	b.localGet(locPtr)
	b.localGet(locEA)
	b.i32Const(8)
	b.op(wasmOpI32ShrU)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.ifVoid()
	x86WasmEmitBailReturn(b, locCtx, locEA, nextPC, retCount, false, size)
	b.end()
	b.end()
}

func x86WasmEmitLogicFlags(b *wasmBody, locCtx, locResult, locFlagsPtr, locFlags, locScratch uint32, width uint32) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)

	b.localGet(locResult)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(locScratch)
	for _, shift := range []int32{4, 2, 1} {
		b.localGet(locScratch)
		b.localGet(locScratch)
		b.i32Const(shift)
		b.op(wasmOpI32ShrU)
		b.op(wasmOpI32Xor)
		b.localSet(locScratch)
	}
	b.localGet(locScratch)
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(1)
	b.op(wasmOpI32Xor)
	b.i32Const(2)
	b.op(wasmOpI32Shl)
	b.localSet(locScratch)

	b.localGet(locFlags)
	b.i32Const(^int32(x86FlagCF | x86FlagOF | x86FlagZF | x86FlagSF | x86FlagPF))
	b.op(wasmOpI32And)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.i32Const(int32(x86FlagZF))
	b.i32Const(0)
	b.localGet(locResult)
	b.op(wasmOpI32Eqz)
	b.op(wasmOpSelect)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locResult)
	b.i32Const(int32(width - 1))
	b.op(wasmOpI32ShrU)
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(7)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locScratch)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlagsPtr)
	b.localGet(locFlags)
	b.i32Store(2, 0)
}

func x86WasmEmitArithFlags(b *wasmBody, locCtx, locResult, locA, locB, locFlagsPtr, locFlags, locScratch uint32, sub bool) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)

	b.localGet(locResult)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(locScratch)
	for _, shift := range []int32{4, 2, 1} {
		b.localGet(locScratch)
		b.localGet(locScratch)
		b.i32Const(shift)
		b.op(wasmOpI32ShrU)
		b.op(wasmOpI32Xor)
		b.localSet(locScratch)
	}
	b.localGet(locScratch)
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(1)
	b.op(wasmOpI32Xor)
	b.i32Const(2)
	b.op(wasmOpI32Shl)
	b.localSet(locScratch)

	b.localGet(locFlags)
	b.i32Const(^int32(x86FlagCF | x86FlagPF | x86FlagAF | x86FlagZF | x86FlagSF | x86FlagOF))
	b.op(wasmOpI32And)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.i32Const(int32(x86FlagZF))
	b.i32Const(0)
	b.localGet(locResult)
	b.op(wasmOpI32Eqz)
	b.op(wasmOpSelect)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locResult)
	b.i32Const(31)
	b.op(wasmOpI32ShrU)
	b.i32Const(7)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locScratch)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	if sub {
		b.localGet(locA)
		b.localGet(locB)
		b.op(wasmOpI32LtU)
	} else {
		b.localGet(locResult)
		b.localGet(locA)
		b.op(wasmOpI32LtU)
	}
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locA)
	b.i32Const(0xF)
	b.op(wasmOpI32And)
	b.localGet(locB)
	b.i32Const(0xF)
	b.op(wasmOpI32And)
	b.op(wasmOpI32LtU)
	b.i32Const(4)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locA)
	b.localGet(locB)
	b.op(wasmOpI32Xor)
	if !sub {
		b.i32Const(-1)
		b.op(wasmOpI32Xor)
	}
	b.localGet(locA)
	b.localGet(locResult)
	b.op(wasmOpI32Xor)
	b.op(wasmOpI32And)
	b.i32Const(31)
	b.op(wasmOpI32ShrU)
	b.i32Const(11)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlagsPtr)
	b.localGet(locFlags)
	b.i32Store(2, 0)
}

func x86WasmEmitShiftFlags32(b *wasmBody, grpOp byte, locCtx, locOrig, locCount, locResult, locFlagsPtr, locFlags, locScratch, locOldFlags uint32) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locOldFlags)

	x86WasmEmitLogicFlags(b, locCtx, locResult, locFlagsPtr, locFlags, locScratch, 32)

	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)

	switch grpOp {
	case 4, 6: // SHL/SAL
		b.localGet(locFlags)
		b.localGet(locOrig)
		b.i32Const(32)
		b.localGet(locCount)
		b.op(wasmOpI32Sub)
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Or)
		b.localSet(locFlags)
	case 5, 7: // SHR/SAR
		b.localGet(locFlags)
		b.localGet(locOrig)
		b.localGet(locCount)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Or)
		b.localSet(locFlags)
	}

	b.localGet(locCount)
	b.i32Const(1)
	b.op(wasmOpI32Eq)
	b.ifVoid()
	switch grpOp {
	case 4, 6: // SHL/SAL
		b.localGet(locFlags)
		b.localGet(locResult)
		b.i32Const(31)
		b.op(wasmOpI32ShrU)
		b.localGet(locOrig)
		b.i32Const(31)
		b.op(wasmOpI32ShrU)
		b.op(wasmOpI32Xor)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.i32Const(11)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
		b.localSet(locFlags)
	case 5: // SHR
		b.localGet(locFlags)
		b.localGet(locOrig)
		b.i32Const(31)
		b.op(wasmOpI32ShrU)
		b.i32Const(11)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
		b.localSet(locFlags)
	case 7: // SAR
		// OF = 0 for count one, already cleared by x86WasmEmitLogicFlags.
	}
	b.elseBranch()
	b.localGet(locFlags)
	b.localGet(locOldFlags)
	b.i32Const(int32(x86FlagOF))
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)
	b.end()

	b.localGet(locFlagsPtr)
	b.localGet(locFlags)
	b.i32Store(2, 0)
}

func x86WasmEmitFlagPredicate(b *wasmBody, locFlags uint32, mask uint32, invert bool) {
	b.localGet(locFlags)
	b.i32Const(int32(mask))
	b.op(wasmOpI32And)
	b.op(wasmOpI32Eqz)
	if !invert {
		b.op(wasmOpI32Eqz)
	}
}

func x86WasmEmitJccCondition(b *wasmBody, condition, locCtx, locFlagsPtr, locFlags uint32) bool {
	if condition > 0xF {
		return false
	}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)
	switch condition {
	case 0x0:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagOF, false)
	case 0x1:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagOF, true)
	case 0x2:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagCF, false)
	case 0x3:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagCF, true)
	case 0x4:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagZF, false)
	case 0x5:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagZF, true)
	case 0x6:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagCF, false)
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagZF, false)
		b.op(wasmOpI32Or)
	case 0x7:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagCF, false)
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagZF, false)
		b.op(wasmOpI32Or)
		b.op(wasmOpI32Eqz)
	case 0x8:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagSF, false)
	case 0x9:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagSF, true)
	case 0xA:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagPF, false)
	case 0xB:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagPF, true)
	case 0xC:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagSF, false)
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagOF, false)
		b.op(wasmOpI32Xor)
	case 0xD:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagSF, false)
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagOF, false)
		b.op(wasmOpI32Xor)
		b.op(wasmOpI32Eqz)
	case 0xE:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagSF, false)
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagOF, false)
		b.op(wasmOpI32Xor)
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagZF, false)
		b.op(wasmOpI32Or)
	case 0xF:
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagSF, false)
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagOF, false)
		b.op(wasmOpI32Xor)
		x86WasmEmitFlagPredicate(b, locFlags, x86FlagZF, false)
		b.op(wasmOpI32Or)
		b.op(wasmOpI32Eqz)
	default:
		return false
	}
	return true
}

func x86WasmEmitTerminalJcc(b *wasmBody, ji X86JITInstr, memory []byte, locCtx, locFlagsPtr, locFlags uint32, instrCount int) bool {
	op := byte(ji.opcode)
	if !x86WasmIsShortJcc(op) || ji.prefixes != 0 {
		return false
	}
	nextPC := ji.opcodePC + uint32(ji.length)
	target := uint32(int32(nextPC) + int32(int8(x86WasmImmediate8(ji, memory))))
	if !x86WasmEmitJccCondition(b, uint32(op&0x0F), locCtx, locFlagsPtr, locFlags) {
		return false
	}
	b.ifVoid()
	x86WasmEmitRetPCAndCount(b, target, instrCount)
	b.elseBranch()
	x86WasmEmitRetPCAndCount(b, nextPC, instrCount)
	b.end()
	return true
}

func x86WasmEmitInstr(b *wasmBody, ji X86JITInstr, memory []byte, retired int, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7 uint32) bool {
	if ji.opcode >= 0x0F00 {
		op2 := byte(ji.opcode)
		switch op2 {
		case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47,
			0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F: // CMOVcc r32/16, r/m32/16
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			if !x86WasmEmitJccCondition(b, uint32(op2&0x0F), locCtx, locTmp2, locTmp3) {
				return false
			}
			b.ifVoid()
			x86WasmEmitLoadReg32(b, locRegs, src)
			if ji.prefixes&x86PrefOpSize != 0 {
				x86WasmEmitInsertReg16(b, locRegs, locTmp, dst)
			} else {
				x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
			}
			b.end()
			return true
		case 0xB6: // MOVZX r32, r/m8
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			if ji.modrm>>6 == 3 {
				x86WasmEmitExtractReg8(b, locRegs, src)
				x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
				return true
			}
			if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
				return false
			}
			x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 1, ji.opcodePC, retired)
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffMemPtr)
			b.localGet(locTmp)
			b.op(wasmOpI32Add)
			b.i32Load8U(0, 0)
			x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
			return true
		case 0xB7: // MOVZX r32, r/m16
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			if ji.modrm>>6 == 3 {
				x86WasmEmitLoadReg32(b, locRegs, src)
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			} else {
				if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
					return false
				}
				x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 2, ji.opcodePC, retired)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffMemPtr)
				b.localGet(locTmp)
				b.op(wasmOpI32Add)
				b.i32Load16U(1, 0)
			}
			x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
			return true
		case 0xBE: // MOVSX r32, r/m8
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			if ji.modrm>>6 == 3 {
				x86WasmEmitExtractReg8(b, locRegs, src)
			} else {
				if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
					return false
				}
				x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 1, ji.opcodePC, retired)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffMemPtr)
				b.localGet(locTmp)
				b.op(wasmOpI32Add)
				b.i32Load8U(0, 0)
			}
			b.i32Const(24)
			b.op(wasmOpI32Shl)
			b.i32Const(24)
			b.op(wasmOpI32ShrS)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
			return true
		case 0xBF: // MOVSX r32, r/m16
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			if ji.modrm>>6 == 3 {
				x86WasmEmitLoadReg32(b, locRegs, src)
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			} else {
				if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
					return false
				}
				x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 2, ji.opcodePC, retired)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffMemPtr)
				b.localGet(locTmp)
				b.op(wasmOpI32Add)
				b.i32Load16U(1, 0)
			}
			b.i32Const(16)
			b.op(wasmOpI32Shl)
			b.i32Const(16)
			b.op(wasmOpI32ShrS)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
			return true
		case 0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97,
			0x98, 0x99, 0x9A, 0x9B, 0x9C, 0x9D, 0x9E, 0x9F: // SETcc r/m8
			if !x86WasmEmitJccCondition(b, uint32(op2&0x0F), locCtx, locTmp2, locTmp3) {
				return false
			}
			x86WasmEmitInsertReg8(b, locRegs, locTmp, ji.modrm&7)
			return true
		case 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF: // BSWAP r32
			x86WasmEmitByteSwap32(b, locRegs, locTmp, op2-0xC8)
			return true
		default:
			return false
		}
	}
	op := byte(ji.opcode)
	switch {
	case op == 0x90 || op == 0x9B:
		return true
	case op >= 0x91 && op <= 0x97: // XCHG EAX, r32
		reg := op - 0x90
		x86WasmEmitLoadReg32(b, locRegs, 0)
		b.localSet(locTmp2)
		x86WasmEmitLoadReg32(b, locRegs, reg)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, 0)
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, reg)
		return true
	case op >= 0xB8 && op <= 0xBF:
		b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		x86WasmEmitStoreReg32(b, locRegs, locTmp, op-0xB8)
		return true
	case op >= 0xB0 && op <= 0xB7:
		reg, shift := x86JITReg8Index(op - 0xB0)
		mask := ^(uint32(0xFF) << shift)
		x86WasmEmitLoadReg32(b, locRegs, byte(reg))
		b.i32Const(int32(mask))
		b.op(wasmOpI32And)
		b.i32Const(int32(uint32(x86WasmImmediate8(ji, memory)) << shift))
		b.op(wasmOpI32Or)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, byte(reg))
		return true
	case op == 0xD1:
		reg := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, reg)
		b.localSet(locTmp3)
		b.i32Const(1)
		b.localSet(locTmp4)
		switch ji.grpOp {
		case 4, 6:
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32Shl)
		case 5:
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32ShrU)
		case 7:
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32ShrS)
		default:
			return false
		}
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, reg)
		x86WasmEmitShiftFlags32(b, ji.grpOp, locCtx, locTmp3, locTmp4, locTmp, locTmp2, locTmp5, locTmp6, locTmp7)
		return true
	case op == 0xC1:
		reg := ji.modrm & 7
		b.i32Const(int32(uint32(x86WasmImmediate8(ji, memory)) & 31))
		b.localSet(locTmp4)
		b.localGet(locTmp4)
		b.op(wasmOpI32Eqz)
		b.ifVoid()
		b.elseBranch()
		x86WasmEmitLoadReg32(b, locRegs, reg)
		b.localSet(locTmp3)
		switch ji.grpOp {
		case 4, 6:
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32Shl)
		case 5:
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32ShrU)
		case 7:
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32ShrS)
		default:
			return false
		}
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, reg)
		x86WasmEmitShiftFlags32(b, ji.grpOp, locCtx, locTmp3, locTmp4, locTmp, locTmp2, locTmp5, locTmp6, locTmp7)
		b.end()
		return true
	case op == 0xD3:
		reg := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, 1)
		b.i32Const(31)
		b.op(wasmOpI32And)
		b.localSet(locTmp4)
		b.localGet(locTmp4)
		b.op(wasmOpI32Eqz)
		b.ifVoid()
		b.elseBranch()
		x86WasmEmitLoadReg32(b, locRegs, reg)
		b.localSet(locTmp3)
		switch ji.grpOp {
		case 4, 6:
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32Shl)
		case 5:
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32ShrU)
		case 7:
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32ShrS)
		default:
			return false
		}
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, reg)
		x86WasmEmitShiftFlags32(b, ji.grpOp, locCtx, locTmp3, locTmp4, locTmp, locTmp2, locTmp5, locTmp6, locTmp7)
		b.end()
		return true
	case op >= 0x40 && op <= 0x47:
		reg := op & 7
		x86WasmEmitLoadReg32(b, locRegs, reg)
		b.localSet(locTmp3)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFlagsPtr)
		b.localSet(locTmp2)
		b.localGet(locTmp2)
		b.i32Load(2, 0)
		b.i32Const(int32(x86FlagCF))
		b.op(wasmOpI32And)
		b.localSet(locTmp6)
		b.i32Const(1)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Add)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, reg)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp7, false)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFlagsPtr)
		b.localSet(locTmp2)
		b.localGet(locTmp2)
		b.i32Load(2, 0)
		b.localSet(locTmp5)
		b.localGet(locTmp5)
		b.i32Const(^int32(x86FlagCF))
		b.op(wasmOpI32And)
		b.localGet(locTmp6)
		b.op(wasmOpI32Or)
		b.localSet(locTmp5)
		b.localGet(locTmp2)
		b.localGet(locTmp5)
		b.i32Store(2, 0)
		return true
	case op >= 0x48 && op <= 0x4F:
		reg := op & 7
		x86WasmEmitLoadReg32(b, locRegs, reg)
		b.localSet(locTmp3)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFlagsPtr)
		b.localSet(locTmp2)
		b.localGet(locTmp2)
		b.i32Load(2, 0)
		b.i32Const(int32(x86FlagCF))
		b.op(wasmOpI32And)
		b.localSet(locTmp6)
		b.i32Const(1)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, reg)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp7, true)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFlagsPtr)
		b.localSet(locTmp2)
		b.localGet(locTmp2)
		b.i32Load(2, 0)
		b.localSet(locTmp5)
		b.localGet(locTmp5)
		b.i32Const(^int32(x86FlagCF))
		b.op(wasmOpI32And)
		b.localGet(locTmp6)
		b.op(wasmOpI32Or)
		b.localSet(locTmp5)
		b.localGet(locTmp2)
		b.localGet(locTmp5)
		b.i32Store(2, 0)
		return true
	case op == 0x81 || op == 0x83:
		dst := ji.modrm & 7
		var imm uint32
		if op == 0x81 {
			imm = x86WasmImmediate32(ji, memory)
		} else {
			imm = uint32(int32(int8(x86WasmImmediate8(ji, memory))))
		}
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.localSet(locTmp3)
		b.i32Const(int32(imm))
		b.localSet(locTmp4)
		switch ji.grpOp {
		case 0: // ADD
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32Add)
			b.localSet(locTmp)
			b.localGet(locTmp)
			x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
			x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, false)
			return true
		case 1: // OR
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32Or)
			b.localSet(locTmp)
			b.localGet(locTmp)
			x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
			x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp5, locTmp6, 32)
			return true
		case 4: // AND
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32And)
			b.localSet(locTmp)
			b.localGet(locTmp)
			x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
			x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp5, locTmp6, 32)
			return true
		case 5: // SUB
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32Sub)
			b.localSet(locTmp)
			b.localGet(locTmp)
			x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
			x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, true)
			return true
		case 6: // XOR
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32Xor)
			b.localSet(locTmp)
			b.localGet(locTmp)
			x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
			x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp5, locTmp6, 32)
			return true
		case 7: // CMP
			b.localGet(locTmp3)
			b.localGet(locTmp4)
			b.op(wasmOpI32Sub)
			b.localSet(locTmp)
			x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, true)
			return true
		default:
			return false
		}
	case op == 0x09 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		x86WasmEmitLoadReg32(b, locRegs, src)
		b.op(wasmOpI32Or)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 32)
		return true
	case op == 0x01 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.localSet(locTmp3)
		x86WasmEmitLoadReg32(b, locRegs, src)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Add)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, false)
		return true
	case op == 0x03 && ji.hasModRM && ji.modrm>>6 == 3:
		dst := (ji.modrm >> 3) & 7
		src := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.localSet(locTmp3)
		x86WasmEmitLoadReg32(b, locRegs, src)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Add)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, false)
		return true
	case op == 0x05:
		x86WasmEmitLoadReg32(b, locRegs, 0)
		b.localSet(locTmp3)
		b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Add)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, 0)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, false)
		return true
	case op == 0x0B && ji.hasModRM && ji.modrm>>6 == 3:
		dst := (ji.modrm >> 3) & 7
		src := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, src)
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.op(wasmOpI32Or)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 32)
		return true
	case op == 0x21 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		x86WasmEmitLoadReg32(b, locRegs, src)
		b.op(wasmOpI32And)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 32)
		return true
	case op == 0x23 && ji.hasModRM && ji.modrm>>6 == 3:
		dst := (ji.modrm >> 3) & 7
		src := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, src)
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.op(wasmOpI32And)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 32)
		return true
	case op == 0x31 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		x86WasmEmitLoadReg32(b, locRegs, src)
		b.op(wasmOpI32Xor)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 32)
		return true
	case op == 0x33 && ji.hasModRM && ji.modrm>>6 == 3:
		dst := (ji.modrm >> 3) & 7
		src := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, src)
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.op(wasmOpI32Xor)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 32)
		return true
	case op == 0x8B && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0:
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 4, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localSet(locTmp2)
		b.localGet(locTmp2)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Load(2, 0)
		x86WasmEmitStoreReg32(b, locRegs, locTmp3, (ji.modrm>>3)&7)
		return true
	case op == 0x89 && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0:
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 4, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localSet(locTmp2)
		b.localGet(locTmp2)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		x86WasmEmitLoadReg32(b, locRegs, (ji.modrm>>3)&7)
		b.i32Store(2, 0)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp2, 4, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0xF6 && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0 && (ji.grpOp == 0 || ji.grpOp == 1):
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 1, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Load8U(0, 0)
		b.i32Const(int32(x86WasmImmediate8(ji, memory)))
		b.op(wasmOpI32And)
		b.localSet(locTmp4)
		x86WasmEmitLogicFlags(b, locCtx, locTmp4, locTmp2, locTmp5, locTmp6, 8)
		return true
	case op == 0xF7 && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0 && (ji.grpOp == 0 || ji.grpOp == 1):
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 4, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Load(2, 0)
		b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		b.op(wasmOpI32And)
		b.localSet(locTmp4)
		x86WasmEmitLogicFlags(b, locCtx, locTmp4, locTmp2, locTmp5, locTmp6, 32)
		return true
	case op == 0x89 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, src)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
		return true
	case op == 0x8B && ji.hasModRM && ji.modrm>>6 == 3:
		dst := (ji.modrm >> 3) & 7
		src := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, src)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
		return true
	case op == 0x39 && ji.hasModRM && ji.modrm>>6 == 3:
		a := ji.modrm & 7
		breg := (ji.modrm >> 3) & 7
		x86WasmEmitLoadReg32(b, locRegs, a)
		b.localSet(locTmp3)
		x86WasmEmitLoadReg32(b, locRegs, breg)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, true)
		return true
	case op == 0x3B && ji.hasModRM && ji.modrm>>6 == 3:
		a := (ji.modrm >> 3) & 7
		breg := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, a)
		b.localSet(locTmp3)
		x86WasmEmitLoadReg32(b, locRegs, breg)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, true)
		return true
	case op == 0x29 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.localSet(locTmp3)
		x86WasmEmitLoadReg32(b, locRegs, src)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, true)
		return true
	case op == 0x2B && ji.hasModRM && ji.modrm>>6 == 3:
		dst := (ji.modrm >> 3) & 7
		src := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.localSet(locTmp3)
		x86WasmEmitLoadReg32(b, locRegs, src)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, true)
		return true
	case op == 0x2D:
		x86WasmEmitLoadReg32(b, locRegs, 0)
		b.localSet(locTmp3)
		b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, 0)
		x86WasmEmitArithFlags(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, true)
		return true
	case op == 0x85 && ji.hasModRM && ji.modrm>>6 == 3:
		dst := ji.modrm & 7
		src := (ji.modrm >> 3) & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		x86WasmEmitLoadReg32(b, locRegs, src)
		b.op(wasmOpI32And)
		b.localSet(locTmp)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 32)
		return true
	case op == 0x87 && ji.hasModRM && ji.modrm>>6 == 3: // XCHG r/m32, r32
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.localSet(locTmp2)
		x86WasmEmitLoadReg32(b, locRegs, src)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, src)
		return true
	case op == 0xE9 || op == 0xEB:
		return true
	default:
		return false
	}
}

func x86WasmEmitRetPCAndCount(b *wasmBody, retPC uint32, instrCount int) {
	b.localGet(0)
	b.i32Const(int32(retPC))
	b.i32Store(2, x86CtxOffRetPC)
	b.localGet(0)
	b.i32Const(int32(instrCount))
	b.i32Store(2, x86CtxOffRetCount)
}

func x86WasmCompileBlockModule(instrs []X86JITInstr, startPC uint32, memory []byte) (*x86WasmCompiledModule, error) {
	compiledInstrs, retPC, spanEnd, termKind, ok := x86WasmSelectCompiledPrefix(instrs, memory, startPC)
	if !ok {
		return nil, fmt.Errorf("x86 wasm: unsupported block at %#x", startPC)
	}
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)
	const (
		locCtx  = 0
		locRegs = 1
		locTmp  = 2
		locTmp2 = 3
		locTmp3 = 4
		locTmp4 = 5
		locTmp5 = 6
		locTmp6 = 7
		locTmp7 = 8
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffJITRegsPtr)
	b.localSet(locRegs)
	emittedTerminalJcc := false
	for i, ji := range compiledInstrs {
		if i == len(compiledInstrs)-1 && termKind == x86WasmTerminalJcc {
			if !x86WasmEmitTerminalJcc(b, ji, memory, locCtx, locTmp2, locTmp3, len(compiledInstrs)) {
				return nil, fmt.Errorf("x86 wasm: unsupported short Jcc %#02x at %#x", byte(ji.opcode), ji.opcodePC)
			}
			emittedTerminalJcc = true
			break
		}
		if !x86WasmEmitInstr(b, ji, memory, i, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7) {
			return nil, fmt.Errorf("x86 wasm: unsupported opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
		}
	}
	if !emittedTerminalJcc {
		x86WasmEmitRetPCAndCount(b, retPC, len(compiledInstrs))
	}
	b.end()
	fn := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32}, b.code)
	m.exportFunc("block", fn)
	block := &JITBlock{
		startPC:          uint64(startPC),
		endPC:            uint64(spanEnd),
		instrCount:       len(compiledInstrs),
		x86CyclePrefix:   x86JITCyclePrefix(compiledInstrs),
		x86TickPrefix:    x86JITTickPrefix(compiledInstrs),
		x86DynamicCycles: x86JITDynamicCycles(compiledInstrs),
	}
	return &x86WasmCompiledModule{
		module: m.build(),
		block:  block,
		retPC:  retPC,
	}, nil
}

func x86WasmCompileRegionModule(region *x86Region, memory []byte) (*x86WasmCompiledModule, error) {
	if region == nil || len(region.blocks) < 2 {
		return nil, fmt.Errorf("x86 wasm: empty or single-block region")
	}
	all := make([]X86JITInstr, 0, 16)
	covered := make([][2]uint64, 0, len(region.blocks))
	var retPC uint32
	var maxEnd uint64
	for i, block := range region.blocks {
		blockPC := region.blockPCs[i]
		nextPC, ok := x86WasmBlockTerminalPC(block, memory, blockPC)
		if !ok {
			return nil, fmt.Errorf("x86 wasm: unsupported region block %#x", blockPC)
		}
		if i != len(region.blocks)-1 && nextPC != region.blockPCs[i+1] {
			return nil, fmt.Errorf("x86 wasm: non-linear successor %#x -> %#x", blockPC, nextPC)
		}
		retPC = nextPC
		all = append(all, block...)
		start := uint64(blockPC)
		end := uint64(block[len(block)-1].opcodePC + uint32(block[len(block)-1].length))
		covered = append(covered, [2]uint64{start, end})
		if end > maxEnd {
			maxEnd = end
		}
	}
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)
	const (
		locCtx  = 0
		locRegs = 1
		locTmp  = 2
		locTmp2 = 3
		locTmp3 = 4
		locTmp4 = 5
		locTmp5 = 6
		locTmp6 = 7
		locTmp7 = 8
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffJITRegsPtr)
	b.localSet(locRegs)
	for i, ji := range all {
		if !x86WasmEmitInstr(b, ji, memory, i, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7) {
			return nil, fmt.Errorf("x86 wasm: unsupported region opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
		}
	}
	x86WasmEmitRetPCAndCount(b, retPC, len(all))
	b.end()
	fn := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32}, b.code)
	m.exportFunc("block", fn)
	block := &JITBlock{
		startPC:          uint64(region.entryPC),
		endPC:            maxEnd,
		instrCount:       len(all),
		x86CyclePrefix:   x86JITCyclePrefix(all),
		x86TickPrefix:    x86JITTickPrefix(all),
		x86DynamicCycles: x86JITDynamicCycles(all),
		tier:             2,
		coveredRanges:    covered,
	}
	return &x86WasmCompiledModule{
		module: m.build(),
		block:  block,
		retPC:  retPC,
	}, nil
}

func x86WasmBuildDriverModule(cacheBase uint32, cacheMask uint32) []byte {
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	m.importTable("env", "tab", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)

	const (
		locCtx = 0
		locPC  = 1
		locT   = 2
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffRetPC)
	b.localSet(locPC)
	b.block()
	b.loop()
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffChainBudget)
	b.localTee(locT)
	b.op(wasmOpI32Eqz)
	b.brIf(1)
	b.localGet(locCtx)
	b.localGet(locT)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.i32Store(2, x86CtxOffChainBudget)

	b.localGet(locPC)
	b.i32Const(int32(cacheMask))
	b.op(wasmOpI32And)
	b.i32Const(3)
	b.op(wasmOpI32Shl)
	b.i32Const(int32(cacheBase))
	b.op(wasmOpI32Add)
	b.localTee(locT)

	b.localGet(locT)
	b.i32Load(2, 0)
	b.localGet(locPC)
	b.op(wasmOpI32Ne)
	b.brIf(1)

	b.localGet(locT)
	b.i32Load(2, 4)
	b.localTee(locT)
	b.op(wasmOpI32Eqz)
	b.brIf(1)

	b.localGet(locCtx)
	b.localGet(locT)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.callIndirect(typ)

	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffNeedIOFallback)
	b.brIf(1)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffNeedInval)
	b.brIf(1)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffExitReason)
	b.brIf(1)

	b.localGet(locCtx)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffChainCount)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffRetCount)
	b.op(wasmOpI32Add)
	b.i32Store(2, x86CtxOffChainCount)
	b.localGet(locCtx)
	b.i32Const(0)
	b.i32Store(2, x86CtxOffRetCount)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffRetPC)
	b.localSet(locPC)
	b.br(0)
	b.end()
	b.end()
	b.end()

	fn := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32}, b.code)
	m.exportFunc("drive", fn)
	return m.build()
}
