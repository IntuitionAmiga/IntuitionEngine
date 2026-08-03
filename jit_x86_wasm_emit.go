// jit_x86_wasm_emit.go - pure-Go x86 js/wasm module builder helpers.
//
// This file deliberately holds only the backend-independent wasm bytecode
// construction so native tests can validate emitted x86 wasm blocks under
// wazero even when the host toolchain cannot execute GOOS=js binaries.

package main

import (
	"fmt"
	"math"
)

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

type x86WasmConditionalRegion struct {
	entryPC     uint32
	entryBlock  []X86JITInstr
	fallBlock   []X86JITInstr
	targetBlock []X86JITInstr
	fallPC      uint32
	targetPC    uint32
	exitPC      uint32
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

func x86WasmImmediate16(ji X86JITInstr, memory []byte) uint16 {
	immPC := ji.opcodePC + uint32(ji.length) - 2
	return uint16(memory[immPC]) | uint16(memory[immPC+1])<<8
}

func x86WasmIsShortJcc(op byte) bool {
	return op >= 0x70 && op <= 0x7F
}

func x86WasmIsNearJcc(ji X86JITInstr) bool {
	return ji.opcode >= 0x0F80 && ji.opcode <= 0x0F8F
}

func x86WasmTerminalJccTarget(ji X86JITInstr, memory []byte) (condition byte, target, nextPC uint32, ok bool) {
	nextPC = ji.opcodePC + uint32(ji.length)
	switch {
	case x86WasmIsShortJcc(byte(ji.opcode)):
		if ji.prefixes != 0 || ji.length < 2 {
			return 0, 0, 0, false
		}
		condition = byte(ji.opcode) & 0x0F
		target = uint32(int32(nextPC) + int32(int8(x86WasmImmediate8(ji, memory))))
		return condition, target, nextPC, true
	case x86WasmIsNearJcc(ji):
		if ji.prefixes&^x86PrefOpSize != 0 {
			return 0, 0, 0, false
		}
		condition = byte(ji.opcode) & 0x0F
		if ji.prefixes&x86PrefOpSize != 0 {
			if ji.length < 4 {
				return 0, 0, 0, false
			}
			target = uint32(int32(nextPC) + int32(int16(x86WasmImmediate16(ji, memory))))
			return condition, target, nextPC, true
		}
		if ji.length < 6 {
			return 0, 0, 0, false
		}
		target = uint32(int32(nextPC) + int32(x86WasmImmediate32(ji, memory)))
		return condition, target, nextPC, true
	default:
		return 0, 0, 0, false
	}
}

func x86WasmSupportedInstr(ji X86JITInstr) bool {
	if ji.opcode >= 0xD8 && ji.opcode <= 0xDF {
		return ji.hasModRM
	}
	if ji.opcode >= 0x0F00 {
		op2 := byte(ji.opcode)
		switch op2 {
		case 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87,
			0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x8D, 0x8E, 0x8F:
			return ji.prefixes&^x86PrefOpSize == 0
		case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47,
			0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F:
			return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0
		case 0xA3:
			return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0
		case 0xA4, 0xA5, 0xAC, 0xAD:
			return ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0
		case 0xBA:
			return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0 && ji.grpOp == 4
		case 0xB6, 0xB7, 0xBE, 0xBF:
			if !ji.hasModRM {
				return false
			}
			if ji.modrm>>6 == 3 {
				return true
			}
			return ji.prefixes == 0
		case 0xBC, 0xBD:
			return ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0
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
	case op == 0x98 || op == 0x99 || op == 0xD6 || op == 0xFA || op == 0xFB:
		return ji.prefixes&^x86PrefOpSize == 0
	case op >= 0xE0 && op <= 0xE3:
		return ji.prefixes&^(x86PrefAddrSize|x86PrefOpSize) == 0
	case op == 0xE8:
		return ji.prefixes&^x86PrefOpSize == 0
	case op >= 0x91 && op <= 0x97:
		return true
	case op >= 0x50 && op <= 0x5F:
		return ji.prefixes&^x86PrefOpSize == 0
	case op == 0x60 || op == 0x61:
		return ji.prefixes&^x86PrefOpSize == 0
	case op == 0x69 || op == 0x6B:
		return ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0
	case op == 0x06 || op == 0x07 || op == 0x0E || op == 0x16 || op == 0x17 || op == 0x1E || op == 0x1F:
		return ji.prefixes == 0
	case op == 0x68 || op == 0x6A:
		return ji.prefixes&^x86PrefOpSize == 0
	case op == 0xC8:
		return ji.prefixes == 0 && ji.length == 4
	case op == 0xC9:
		return ji.prefixes&^x86PrefOpSize == 0
	case op == 0xD7:
		return ji.prefixes == 0
	case op == 0x8F:
		return ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes&^x86PrefOpSize == 0 && ji.grpOp == 0
	case op >= 0xB8 && op <= 0xBF:
		return true
	case op >= 0xB0 && op <= 0xB7:
		return true
	case op >= 0x40 && op <= 0x4F:
		return ji.prefixes == 0
	case op == 0xD0:
		return ji.hasModRM && ji.prefixes == 0 && ji.grpOp <= 7
	case op == 0xD1:
		return ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0 && ji.grpOp <= 7
	case op == 0xC0:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0 && ji.grpOp <= 7
	case op == 0xC1:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0 && ji.grpOp <= 7
	case op == 0xD2:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0 && ji.grpOp <= 7
	case op == 0xD3:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0 && ji.grpOp <= 7
	case op == 0x87:
		return ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0
	case op == 0x8C || op == 0x8E:
		return ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0
	case op == 0x88 || op == 0x8A:
		return ji.hasModRM && ji.prefixes == 0
	case op == 0x8D:
		return ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0
	case op == 0xC4 || op == 0xC5:
		return ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0
	case op == 0xA0 || op == 0xA2:
		return ji.prefixes == 0
	case op == 0xA1 || op == 0xA3:
		return ji.prefixes&^x86PrefOpSize == 0
	case op == 0xA4 || op == 0xA6 || op == 0xAA || op == 0xAC || op == 0xAE:
		return ji.prefixes&^(x86PrefRep|x86PrefRepNE) == 0
	case op == 0xA5 || op == 0xA7 || op == 0xAB || op == 0xAD || op == 0xAF:
		return ji.prefixes&^(x86PrefOpSize|x86PrefRep|x86PrefRepNE) == 0
	case op == 0x9C || op == 0x9D:
		return ji.prefixes&^x86PrefOpSize == 0
	case op == 0xF6:
		return ji.hasModRM && ji.prefixes == 0 &&
			((ji.modrm>>6 != 3 && (ji.grpOp == 0 || ji.grpOp == 1)) ||
				ji.grpOp == 0 || ji.grpOp == 1 || ji.grpOp == 2 || ji.grpOp == 3 ||
				ji.grpOp == 4 || ji.grpOp == 5 || ji.grpOp == 6 || ji.grpOp == 7)
	case op == 0xF7:
		return ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0 &&
			((ji.modrm>>6 != 3 && (ji.grpOp == 0 || ji.grpOp == 1)) ||
				ji.grpOp == 0 || ji.grpOp == 1 || ji.grpOp == 2 || ji.grpOp == 3 ||
				ji.grpOp == 4 || ji.grpOp == 5 || ji.grpOp == 6 || ji.grpOp == 7)
	case op == 0xC6:
		return ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0 && ji.grpOp == 0
	case op == 0xC7:
		return ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes&^x86PrefOpSize == 0 && ji.grpOp == 0
	case op == 0x81 || op == 0x83:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0 &&
			(ji.grpOp == 0 || ji.grpOp == 1 || ji.grpOp == 4 || ji.grpOp == 5 || ji.grpOp == 6 || ji.grpOp == 7)
	case op == 0x00 || op == 0x08 || op == 0x20 || op == 0x28 || op == 0x30 || op == 0x38:
		return ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0
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
	case op == 0xC3 || op == 0xC2:
		return ji.prefixes == 0
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
			case 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87,
				0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x8D, 0x8E, 0x8F:
				if i != len(instrs)-1 {
					return 0, false
				}
				return ji.opcodePC + uint32(ji.length), true
			case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47,
				0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F,
				0xA3, 0xA4, 0xA5, 0xAC, 0xAD, 0xBA,
				0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97,
				0x98, 0x99, 0x9A, 0x9B, 0x9C, 0x9D, 0x9E, 0x9F,
				0xB6, 0xB7, 0xBC, 0xBD, 0xBE, 0xBF,
				0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF:
				continue
			default:
				return 0, false
			}
		}
		switch op {
		case 0x90, 0x98, 0x99, 0x9B, 0xD6, 0xFA, 0xFB, 0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x8D, 0x8E, 0xC4, 0xC5: // NOP, WAIT, MOVs, LEA, control
			continue
		case 0xA0, 0xA1, 0xA2, 0xA3:
			continue
		case 0xA4, 0xA5, 0xA6, 0xA7, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF:
			continue
		case 0x06, 0x07, 0x0E, 0x16, 0x17, 0x1E, 0x1F, 0x68, 0x6A, 0x8F, 0x9C, 0x9D:
			continue
		case 0x60, 0x61, 0xC8, 0xC9, 0xD7:
			continue
		case 0x69, 0x6B:
			continue
		case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47,
			0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F:
			continue
		case 0xC0, 0xC1, 0xD0, 0xD1, 0xD2, 0xD3:
			continue
		case 0xC6, 0xC7:
			continue
		case 0xF6, 0xF7:
			continue
		case 0x81, 0x83:
			continue
		case 0x00, 0x08, 0x20, 0x28, 0x30, 0x38:
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
		case 0x50, 0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57,
			0x58, 0x59, 0x5A, 0x5B, 0x5C, 0x5D, 0x5E, 0x5F:
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
	x86WasmTerminalCALL
	x86WasmTerminalRET
	x86WasmTerminalLoop
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
		case x86WasmIsNearJcc(ji):
			return instrs[:i+1], spanEnd, spanEnd, x86WasmTerminalJcc, true
		case op >= 0xE0 && op <= 0xE3:
			return instrs[:i+1], spanEnd, spanEnd, x86WasmTerminalLoop, true
		case op == 0xE8:
			return instrs[:i+1], 0, spanEnd, x86WasmTerminalCALL, true
		case op == 0xC3 || op == 0xC2:
			return instrs[:i+1], 0, spanEnd, x86WasmTerminalRET, true
		case op == 0xE9 || op == 0xEB:
			target, ok := x86ResolveTerminatorTarget(&ji, memory, startPC)
			if !ok {
				return nil, 0, 0, x86WasmTerminalFallthrough, false
			}
			return instrs[:i+1], target, spanEnd, x86WasmTerminalJMP, true
		case op >= 0xD8 && op <= 0xDF:
			return instrs[:i+1], spanEnd, spanEnd, x86WasmTerminalFallthrough, true
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

func x86WasmEmitEA16(b *wasmBody, ji X86JITInstr, memory []byte, locRegs, locEA uint32) bool {
	if !ji.hasModRM || ji.prefixes&x86PrefAddrSize == 0 {
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
	b.i32Const(0)
	b.localSet(locEA)
	addReg16 := func(reg byte) {
		b.localGet(locEA)
		x86WasmEmitLoadReg32(b, locRegs, reg)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Add)
		b.localSet(locEA)
	}
	switch rm {
	case 0:
		addReg16(3)
		addReg16(6)
	case 1:
		addReg16(3)
		addReg16(7)
	case 2:
		addReg16(5)
		addReg16(6)
	case 3:
		addReg16(5)
		addReg16(7)
	case 4:
		addReg16(6)
	case 5:
		addReg16(7)
	case 6:
		if mod != 0 {
			addReg16(5)
		}
	case 7:
		addReg16(3)
	}
	pos := int(modrmPC) + 1
	dispBytes := 0
	if mod == 1 {
		dispBytes = 1
	} else if mod == 2 || (mod == 0 && rm == 6) {
		dispBytes = 2
	}
	if pos+dispBytes > len(memory) {
		return false
	}
	disp := uint32(0)
	if dispBytes == 1 {
		disp = uint32(int32(int8(memory[pos])))
	} else if dispBytes == 2 {
		disp = uint32(int32(int16(uint16(memory[pos]) | uint16(memory[pos+1])<<8)))
	}
	if dispBytes != 0 {
		b.localGet(locEA)
		b.i32Const(int32(disp))
		b.op(wasmOpI32Add)
		b.localSet(locEA)
	}
	b.localGet(locEA)
	b.i32Const(0xFFFF)
	b.op(wasmOpI32And)
	b.localSet(locEA)
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

func x86WasmEmitLoadSeg16(b *wasmBody, locCtx, locSegPtr uint32, seg byte) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffSegRegsPtr)
	b.localSet(locSegPtr)
	b.localGet(locSegPtr)
	if seg != 0 {
		b.i32Const(int32(seg) * 2)
		b.op(wasmOpI32Add)
	}
	b.i32Load16U(1, 0)
}

func x86WasmEmitStoreSeg16(b *wasmBody, locCtx, locValue, locSegPtr uint32, seg byte) {
	b.localSet(locValue)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffSegRegsPtr)
	b.localSet(locSegPtr)
	b.localGet(locSegPtr)
	if seg != 0 {
		b.i32Const(int32(seg) * 2)
		b.op(wasmOpI32Add)
	}
	b.localGet(locValue)
	b.i32Store16(1, 0)
}

func x86WasmEmitStringAdvance(b *wasmBody, locCtx, locValue, locFlagsPtr, locFlags uint32, width uint32) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.i32Const(int32(x86FlagDF))
	b.op(wasmOpI32And)
	b.ifVoid()
	b.localGet(locValue)
	b.i32Const(int32(width))
	b.op(wasmOpI32Sub)
	b.localSet(locValue)
	b.elseBranch()
	b.localGet(locValue)
	b.i32Const(int32(width))
	b.op(wasmOpI32Add)
	b.localSet(locValue)
	b.end()
}

func x86WasmEmitStringDynamicCredit(b *wasmBody, locCtx, locInitial, locCurrent, locCycles, locTicks uint32) {
	b.localGet(locInitial)
	b.localGet(locCurrent)
	b.op(wasmOpI32Sub)
	b.localSet(locCycles)
	b.localGet(locCycles)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	b.i32Const(0)
	b.localSet(locTicks)
	b.elseBranch()
	b.localGet(locCycles)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.localSet(locTicks)
	b.end()
	x86WasmEmitDynamicChainCredit(b, locCtx, locCycles, locTicks)
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

func x86WasmEmitExitReasonReturn(b *wasmBody, locCtx uint32, retPC uint32, retCount int, reason uint32) {
	b.localGet(locCtx)
	b.i32Const(int32(retPC))
	b.i32Store(2, x86CtxOffRetPC)
	b.localGet(locCtx)
	b.i32Const(int32(retCount))
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffChainCount)
	b.op(wasmOpI32Add)
	b.i32Store(2, x86CtxOffRetCount)
	b.localGet(locCtx)
	b.i32Const(int32(reason))
	b.i32Store(2, x86CtxOffExitReason)
	b.op(wasmOpReturn)
}

func x86WasmEmitStoreCtxByte(b *wasmBody, locCtx uint32, off uint32, value byte) {
	b.localGet(locCtx)
	b.i32Const(int32(value))
	b.i32Store8(0, off)
}

func x86WasmEmitFPUHelperExit(b *wasmBody, ji X86JITInstr, memory []byte, retired int, locCtx, locRegs, locSeg, locEA uint32) bool {
	payload, ok := x86FPUHelperPayloadFor(ji, memory, 0)
	if !ok {
		return false
	}
	segment, ok := x86FPUHelperSegmentFromPayload(payload)
	if !ok {
		return false
	}
	memForm := ji.modrm>>6 != 3
	width := uint32(0)
	if memForm {
		if ji.prefixes&x86PrefAddrSize != 0 {
			if !x86WasmEmitEA16(b, ji, memory, locRegs, locEA) {
				return false
			}
		} else {
			if !x86WasmEmitEA32(b, ji, memory, locRegs, locEA) {
				return false
			}
		}
		width = x86FPUHelperAccessWidthFromOpcode(payload.Escape, payload.ModRM)
	} else {
		b.i32Const(0)
		b.localSet(locEA)
	}

	b.localGet(locCtx)
	b.i32Const(int32(payload.InstrPC))
	b.i32Store(2, x86CtxOffFPUHelperInstrPC)

	x86WasmEmitLoadSeg16(b, locCtx, locSeg, x86SegCS)
	b.localSet(locSeg)
	b.localGet(locCtx)
	b.localGet(locSeg)
	b.i32Store16(1, x86CtxOffFPUHelperCS)

	x86WasmEmitStoreCtxByte(b, locCtx, x86CtxOffFPUHelperEscape, payload.Escape)
	x86WasmEmitStoreCtxByte(b, locCtx, x86CtxOffFPUHelperModRM, payload.ModRM)
	x86WasmEmitStoreCtxByte(b, locCtx, x86CtxOffFPUHelperPrefixes, payload.Prefixes)
	x86WasmEmitStoreCtxByte(b, locCtx, x86CtxOffFPUHelperLength, payload.Length)
	x86WasmEmitStoreCtxByte(b, locCtx, x86CtxOffFPUHelperSegment, segment)
	for off, byt := range payload.Bytes {
		x86WasmEmitStoreCtxByte(b, locCtx, x86CtxOffFPUHelperBytes+uint32(off), byt)
	}

	b.localGet(locCtx)
	b.localGet(locEA)
	b.i32Store(2, x86CtxOffFPUHelperEA)
	b.localGet(locCtx)
	b.i32Const(int32(width))
	b.i32Store(2, x86CtxOffFPUHelperWidth)
	b.localGet(locCtx)
	b.i32Const(1)
	b.i32Store(2, x86CtxOffNeedIOFallback)
	x86WasmEmitExitReasonReturn(b, locCtx, payload.InstrPC, retired, x86JITExitFPUHelper)
	return true
}

func x86WasmEmitFPUCaptureOp(b *wasmBody, ji X86JITInstr, memory []byte, locCtx, locFPU, locTmp uint32) bool {
	modrmPC := x86FindModRMPC(&ji, memory)
	if modrmPC < 1 {
		return false
	}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFPUPtr)
	b.localSet(locFPU)
	b.localGet(locFPU)
	b.i32Const(int32(modrmPC - 1))
	b.i32Store(2, x86FPUOffFIP)
	x86WasmEmitLoadSeg16(b, locCtx, locTmp, x86SegCS)
	b.localSet(locTmp)
	b.localGet(locFPU)
	b.localGet(locTmp)
	b.i32Store16(1, x86FPUOffFCS)
	b.localGet(locFPU)
	b.i32Const(int32((uint16(byte(ji.opcode)-0xD8) << 8) | uint16(ji.modrm)))
	b.i32Store16(1, x86FPUOffFOP)
	return true
}

func x86WasmEmitFPUReadTop(b *wasmBody, locFPU, locTop uint32) {
	b.localGet(locFPU)
	b.i32Load16U(1, x86FPUOffFSW)
	b.i32Const(11)
	b.op(wasmOpI32ShrU)
	b.i32Const(7)
	b.op(wasmOpI32And)
	b.localSet(locTop)
}

func x86WasmEmitFPUTagAtPhys(b *wasmBody, locFPU, locPhys, locTag uint32) {
	b.localGet(locPhys)
	b.i32Const(1)
	b.op(wasmOpI32Shl)
	b.localSet(locTag)
	b.localGet(locFPU)
	b.i32Load16U(1, x86FPUOffFTW)
	b.localGet(locTag)
	b.op(wasmOpI32ShrU)
	b.i32Const(3)
	b.op(wasmOpI32And)
	b.localSet(locTag)
}

func x86WasmEmitFPUSetTagAtPhys(b *wasmBody, locFPU, locPhys, locTag, locTmp uint32) {
	b.localGet(locPhys)
	b.i32Const(1)
	b.op(wasmOpI32Shl)
	b.localSet(locTmp)
	b.localGet(locFPU)
	b.localGet(locFPU)
	b.i32Load16U(1, x86FPUOffFTW)
	b.i32Const(3)
	b.localGet(locTmp)
	b.op(wasmOpI32Shl)
	b.i32Const(-1)
	b.op(wasmOpI32Xor)
	b.op(wasmOpI32And)
	b.localGet(locTag)
	b.i32Const(3)
	b.op(wasmOpI32And)
	b.localGet(locTmp)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.i32Store16(1, x86FPUOffFTW)
}

func x86WasmEmitFPUSetTop(b *wasmBody, locFPU, locTop, locTmp uint32) {
	b.localGet(locFPU)
	b.localGet(locFPU)
	b.i32Load16U(1, x86FPUOffFSW)
	b.i32Const(^int32(x87FSW_TOPMask))
	b.op(wasmOpI32And)
	b.localGet(locTop)
	b.i32Const(7)
	b.op(wasmOpI32And)
	b.i32Const(x87FSW_TOPShift)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.i32Store16(1, x86FPUOffFSW)
}

func x86WasmEmitFPUPhysFromTopPlus(b *wasmBody, locTop uint32, delta int32, locOut uint32) {
	b.localGet(locTop)
	if delta != 0 {
		b.i32Const(delta)
		b.op(wasmOpI32Add)
	}
	b.i32Const(7)
	b.op(wasmOpI32And)
	b.localSet(locOut)
}

func x86WasmEmitFPUCompareDirect(b *wasmBody, ji X86JITInstr, memory []byte, retired int, locCtx, locRegs, locTop, locPhys, locFPU, locTagA, locTagB uint32, zeroSrc, pop bool) bool {
	if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locFPU, locTagA) {
		return false
	}
	x86WasmEmitFPUReadTop(b, locFPU, locTop)
	x86WasmEmitFPUTagAtPhys(b, locFPU, locTop, locTagA)
	b.localGet(locTagA)
	b.i32Const(int32(x87TagEmpty))
	b.op(wasmOpI32Eq)
	b.ifVoid()
	if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTagA, locTop) {
		return false
	}
	b.end()
	b.localGet(locTagA)
	b.i32Const(int32(x87TagSpecial))
	b.op(wasmOpI32Eq)
	b.ifVoid()
	if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTagA, locTop) {
		return false
	}
	b.end()

	if !zeroSrc {
		x86WasmEmitFPUPhysFromTopPlus(b, locTop, int32(ji.modrm&7), locPhys)
		x86WasmEmitFPUTagAtPhys(b, locFPU, locPhys, locTagB)
		b.localGet(locTagB)
		b.i32Const(int32(x87TagEmpty))
		b.op(wasmOpI32Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTagB, locPhys) {
			return false
		}
		b.end()
		b.localGet(locTagB)
		b.i32Const(int32(x87TagSpecial))
		b.op(wasmOpI32Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTagB, locPhys) {
			return false
		}
		b.end()
	}

	b.localGet(locFPU)
	b.i32Load16U(1, x86FPUOffFSW)
	b.i32Const(^int32(x87FSW_C0 | x87FSW_C1 | x87FSW_C2 | x87FSW_C3))
	b.op(wasmOpI32And)
	b.localSet(locTagA)

	loadSrc := func() {
		if zeroSrc {
			b.f64Const(0)
			return
		}
		b.localGet(locFPU)
		b.localGet(locPhys)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.f64Load(3, 0)
	}
	loadST0 := func() {
		b.localGet(locFPU)
		b.localGet(locTop)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.f64Load(3, 0)
	}

	loadST0()
	loadSrc()
	b.op(wasmOpF64Lt)
	b.ifVoid()
	b.localGet(locTagA)
	b.i32Const(int32(x87FSW_C0))
	b.op(wasmOpI32Or)
	b.localSet(locTagA)
	b.end()

	loadST0()
	loadSrc()
	b.op(wasmOpF64Eq)
	b.ifVoid()
	b.localGet(locTagA)
	b.i32Const(int32(x87FSW_C3))
	b.op(wasmOpI32Or)
	b.localSet(locTagA)
	b.end()

	b.localGet(locFPU)
	b.localGet(locTagA)
	b.i32Store16(1, x86FPUOffFSW)

	if pop {
		b.i32Const(int32(x87TagEmpty))
		b.localSet(locTagA)
		x86WasmEmitFPUSetTagAtPhys(b, locFPU, locTop, locTagA, locTagB)
		x86WasmEmitFPUPhysFromTopPlus(b, locTop, 1, locTop)
		x86WasmEmitFPUSetTop(b, locFPU, locTop, locTagA)
	}

	return true
}

func x86WasmEmitFPUDirectOrHelper(b *wasmBody, ji X86JITInstr, memory []byte, retired int, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5 uint32) bool {
	switch {
	case ji.opcode == 0xD9 && ji.modrm == 0xD0: // FNOP
		return x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2)
	case ji.opcode == 0xDB && ji.modrm == 0xE2: // FNCLEX
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		b.localGet(locTmp3)
		b.localGet(locTmp3)
		b.i32Load16U(1, x86FPUOffFSW)
		b.i32Const(0x7F00)
		b.op(wasmOpI32And)
		b.i32Store16(1, x86FPUOffFSW)
		return true
	case ji.opcode == 0xDB && ji.modrm == 0xE3: // FNINIT
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFPUPtr)
		b.localSet(locTmp3)
		for off := uint32(0); off < 64; off += 8 {
			b.localGet(locTmp3)
			b.i64Const(0)
			b.i64Store(3, off)
		}
		b.localGet(locTmp3)
		b.i32Const(0x037F)
		b.i32Store16(1, x86FPUOffFCW)
		b.localGet(locTmp3)
		b.i32Const(0)
		b.i32Store16(1, x86FPUOffFSW)
		b.localGet(locTmp3)
		b.i32Const(0xFFFF)
		b.i32Store16(1, x86FPUOffFTW)
		b.localGet(locTmp3)
		b.i32Const(0)
		b.i32Store(2, x86FPUOffFIP)
		b.localGet(locTmp3)
		b.i32Const(0)
		b.i32Store16(1, x86FPUOffFCS)
		b.localGet(locTmp3)
		b.i32Const(0)
		b.i32Store(2, x86FPUOffFDP)
		b.localGet(locTmp3)
		b.i32Const(0)
		b.i32Store16(1, x86FPUOffFDS)
		b.localGet(locTmp3)
		b.i32Const(0)
		b.i32Store16(1, x86FPUOffFOP)
		return true
	case ji.opcode == 0xDF && ji.modrm == 0xE0: // FNSTSW AX
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		b.localGet(locTmp3)
		b.i32Load16U(1, x86FPUOffFSW)
		x86WasmEmitInsertReg16(b, locRegs, locTmp, 0)
		return true
	case ji.opcode == 0xD9 && (ji.modrm == 0xE0 || ji.modrm == 0xE1): // FCHS/FABS
		const locI64Tmp = 10
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		x86WasmEmitFPUReadTop(b, locTmp3, locTmp)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp, locTmp2)
		b.localGet(locTmp2)
		b.i32Const(3)
		b.op(wasmOpI32Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp2, locTmp) {
			return false
		}
		b.end()
		b.localGet(locTmp3)
		b.localGet(locTmp)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.localSet(locTmp)
		b.localGet(locTmp)
		b.i64Load(3, 0)
		if ji.modrm == 0xE0 {
			b.i64Const(-9223372036854775808)
			b.op(wasmOpI64Xor)
		} else {
			b.i64Const(0x7FFFFFFFFFFFFFFF)
			b.op(wasmOpI64And)
		}
		b.localSet(locI64Tmp)
		b.localGet(locTmp)
		b.localGet(locI64Tmp)
		b.i64Store(3, 0)
		return true
	case ji.opcode == 0xD9 && ji.modrm >= 0xC0 && ji.modrm <= 0xC7: // FLD ST(i)
		const locI64Tmp = 10
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		x86WasmEmitFPUReadTop(b, locTmp3, locTmp)
		x86WasmEmitFPUPhysFromTopPlus(b, locTmp, int32(ji.modrm&7), locTmp2)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp2, locTmp4)
		b.localGet(locTmp4)
		b.i32Const(3)
		b.op(wasmOpI32Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp4, locTmp2) {
			return false
		}
		b.end()
		x86WasmEmitFPUPhysFromTopPlus(b, locTmp, -1, locTmp)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp, locTmp5)
		b.localGet(locTmp5)
		b.i32Const(3)
		b.op(wasmOpI32Ne)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp5, locTmp) {
			return false
		}
		b.end()
		x86WasmEmitFPUSetTop(b, locTmp3, locTmp, locTmp2)
		x86WasmEmitFPUSetTagAtPhys(b, locTmp3, locTmp, locTmp4, locTmp5)
		b.localGet(locTmp3)
		b.localGet(locTmp2)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.i64Load(3, 0)
		b.localSet(locI64Tmp)
		b.localGet(locTmp3)
		b.localGet(locTmp)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.localGet(locI64Tmp)
		b.i64Store(3, 0)
		return true
	case ji.opcode == 0xD9 && ji.modrm >= 0xC8 && ji.modrm <= 0xCF: // FXCH ST(i)
		const locI64Tmp = 10
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		x86WasmEmitFPUReadTop(b, locTmp3, locTmp)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp, locTmp4)
		b.localGet(locTmp4)
		b.i32Const(3)
		b.op(wasmOpI32Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp4, locTmp) {
			return false
		}
		b.end()
		x86WasmEmitFPUPhysFromTopPlus(b, locTmp, int32(ji.modrm&7), locTmp2)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp2, locTmp5)
		b.localGet(locTmp5)
		b.i32Const(3)
		b.op(wasmOpI32Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp5, locTmp2) {
			return false
		}
		b.end()
		b.localGet(locTmp3)
		b.localGet(locTmp2)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.i64Load(3, 0)
		b.localSet(locI64Tmp)
		b.localGet(locTmp3)
		b.localGet(locTmp2)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.localGet(locTmp3)
		b.localGet(locTmp)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.i64Load(3, 0)
		b.i64Store(3, 0)
		b.localGet(locTmp3)
		b.localGet(locTmp)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.localGet(locI64Tmp)
		b.i64Store(3, 0)
		x86WasmEmitFPUSetTagAtPhys(b, locTmp3, locTmp, locTmp5, locTmp4)
		x86WasmEmitFPUSetTagAtPhys(b, locTmp3, locTmp2, locTmp4, locTmp5)
		return true
	case ji.opcode == 0xDD && ji.modrm>>6 == 3 && (ji.modrm>>3)&7 == 0: // FFREE ST(i)
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		x86WasmEmitFPUReadTop(b, locTmp3, locTmp)
		if i := int32(ji.modrm & 7); i != 0 {
			b.localGet(locTmp)
			b.i32Const(i)
			b.op(wasmOpI32Add)
			b.i32Const(7)
			b.op(wasmOpI32And)
			b.localSet(locTmp)
		}
		b.localGet(locTmp)
		b.i32Const(1)
		b.op(wasmOpI32Shl)
		b.localSet(locTmp2)
		b.localGet(locTmp3)
		b.localGet(locTmp3)
		b.i32Load16U(1, x86FPUOffFTW)
		b.i32Const(3)
		b.localGet(locTmp2)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
		b.i32Store16(1, x86FPUOffFTW)
		return true
	case ji.opcode == 0xDD && ji.modrm >= 0xD8 && ji.modrm <= 0xDF: // FSTP ST(i)
		const locI64Tmp = 10
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		x86WasmEmitFPUReadTop(b, locTmp3, locTmp)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp, locTmp4)
		b.localGet(locTmp4)
		b.i32Const(3)
		b.op(wasmOpI32Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp4, locTmp) {
			return false
		}
		b.end()
		x86WasmEmitFPUPhysFromTopPlus(b, locTmp, int32(ji.modrm&7), locTmp2)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp2, locTmp5)
		b.localGet(locTmp5)
		b.i32Const(3)
		b.op(wasmOpI32Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp5, locTmp2) {
			return false
		}
		b.end()
		b.localGet(locTmp3)
		b.localGet(locTmp)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.i64Load(3, 0)
		b.localSet(locI64Tmp)
		b.localGet(locTmp3)
		b.localGet(locTmp2)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.localGet(locI64Tmp)
		b.i64Store(3, 0)
		x86WasmEmitFPUSetTagAtPhys(b, locTmp3, locTmp2, locTmp4, locTmp5)
		b.i32Const(int32(x87TagEmpty))
		b.localSet(locTmp5)
		x86WasmEmitFPUSetTagAtPhys(b, locTmp3, locTmp, locTmp5, locTmp4)
		x86WasmEmitFPUPhysFromTopPlus(b, locTmp, 1, locTmp)
		x86WasmEmitFPUSetTop(b, locTmp3, locTmp, locTmp2)
		return true
	case ji.opcode == 0xD9 && (ji.modrm == 0xF6 || ji.modrm == 0xF7): // FDECSTP/FINCSTP
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		x86WasmEmitFPUReadTop(b, locTmp3, locTmp)
		if ji.modrm == 0xF6 {
			x86WasmEmitFPUPhysFromTopPlus(b, locTmp, -1, locTmp)
		} else {
			x86WasmEmitFPUPhysFromTopPlus(b, locTmp, 1, locTmp)
		}
		x86WasmEmitFPUSetTop(b, locTmp3, locTmp, locTmp2)
		return true
	case ji.opcode == 0xD9 && ji.modrm >= 0xE8 && ji.modrm <= 0xEE: // x87 constants
		const locI64Tmp = 10
		var bits uint64
		switch ji.modrm {
		case 0xE8:
			bits = 0x3FF0000000000000
		case 0xEE:
			bits = 0
		default:
			bits = math.Float64bits(x87ConstTable[ji.modrm-0xE8])
		}
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		x86WasmEmitFPUReadTop(b, locTmp3, locTmp)
		x86WasmEmitFPUPhysFromTopPlus(b, locTmp, -1, locTmp)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp, locTmp2)
		b.localGet(locTmp2)
		b.i32Const(3)
		b.op(wasmOpI32Ne)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp2, locTmp) {
			return false
		}
		b.end()
		x86WasmEmitFPUSetTop(b, locTmp3, locTmp, locTmp2)
		b.i64Const(int64(bits))
		b.localSet(locI64Tmp)
		b.localGet(locTmp3)
		b.localGet(locTmp)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.localGet(locI64Tmp)
		b.i64Store(3, 0)
		tag := int32(x87TagValid)
		if bits == 0 {
			tag = int32(x87TagZero)
		}
		b.i32Const(tag)
		b.localSet(locTmp2)
		x86WasmEmitFPUSetTagAtPhys(b, locTmp3, locTmp, locTmp2, locTmp4)
		return true
	case ji.opcode == 0xD8 && ji.modrm>>6 == 3:
		const (
			locI64Tmp = 10
			expMask   = int64(0x7FF0000000000000)
		)
		op := (ji.modrm >> 3) & 7
		if op == 2 || op == 3 {
			return x86WasmEmitFPUCompareDirect(b, ji, memory, retired, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, false, op == 3)
		}
		if op != 0 && op != 1 && op != 4 && op != 5 && op != 6 && op != 7 {
			return x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp2, locTmp)
		}
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		x86WasmEmitFPUReadTop(b, locTmp3, locTmp)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp, locTmp4)
		b.localGet(locTmp4)
		b.i32Const(int32(x87TagValid))
		b.op(wasmOpI32Ne)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp4, locTmp) {
			return false
		}
		b.end()
		x86WasmEmitFPUPhysFromTopPlus(b, locTmp, int32(ji.modrm&7), locTmp2)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp2, locTmp5)
		b.localGet(locTmp5)
		b.i32Const(int32(x87TagValid))
		b.op(wasmOpI32Ne)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp5, locTmp2) {
			return false
		}
		b.end()
		if op == 5 || op == 7 {
			b.localGet(locTmp3)
			b.localGet(locTmp2)
			b.i32Const(3)
			b.op(wasmOpI32Shl)
			b.op(wasmOpI32Add)
			b.f64Load(3, 0)
			b.localGet(locTmp3)
			b.localGet(locTmp)
			b.i32Const(3)
			b.op(wasmOpI32Shl)
			b.op(wasmOpI32Add)
			b.f64Load(3, 0)
		} else {
			b.localGet(locTmp3)
			b.localGet(locTmp)
			b.i32Const(3)
			b.op(wasmOpI32Shl)
			b.op(wasmOpI32Add)
			b.f64Load(3, 0)
			b.localGet(locTmp3)
			b.localGet(locTmp2)
			b.i32Const(3)
			b.op(wasmOpI32Shl)
			b.op(wasmOpI32Add)
			b.f64Load(3, 0)
		}
		switch op {
		case 0:
			b.op(wasmOpF64Add)
		case 1:
			b.op(wasmOpF64Mul)
		case 4, 5:
			b.op(wasmOpF64Sub)
		case 6, 7:
			b.op(wasmOpF64Div)
		}
		b.op(wasmOpI64ReinterpretF64)
		b.localSet(locI64Tmp)
		b.localGet(locI64Tmp)
		b.i64Const(expMask)
		b.op(wasmOpI64And)
		b.op(wasmOpI64Eqz)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp4, locTmp) {
			return false
		}
		b.end()
		b.localGet(locI64Tmp)
		b.i64Const(expMask)
		b.op(wasmOpI64And)
		b.i64Const(expMask)
		b.op(wasmOpI64Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp5, locTmp2) {
			return false
		}
		b.end()
		b.localGet(locTmp3)
		b.localGet(locTmp)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.localGet(locI64Tmp)
		b.i64Store(3, 0)
		return true
	case ji.opcode == 0xDE && ji.modrm>>6 == 3:
		const (
			locI64Tmp = 10
			expMask   = int64(0x7FF0000000000000)
		)
		op := (ji.modrm >> 3) & 7
		mapOp := int(op)
		switch op {
		case 4:
			mapOp = 5
		case 5:
			mapOp = 4
		case 6:
			mapOp = 7
		case 7:
			mapOp = 6
		}
		if op != 0 && op != 1 && op != 4 && op != 5 && op != 6 && op != 7 {
			return x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp2, locTmp)
		}
		if !x86WasmEmitFPUCaptureOp(b, ji, memory, locCtx, locTmp3, locTmp2) {
			return false
		}
		x86WasmEmitFPUReadTop(b, locTmp3, locTmp)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp, locTmp4)
		b.localGet(locTmp4)
		b.i32Const(int32(x87TagValid))
		b.op(wasmOpI32Ne)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp4, locTmp) {
			return false
		}
		b.end()
		x86WasmEmitFPUPhysFromTopPlus(b, locTmp, int32(ji.modrm&7), locTmp2)
		x86WasmEmitFPUTagAtPhys(b, locTmp3, locTmp2, locTmp5)
		b.localGet(locTmp5)
		b.i32Const(int32(x87TagValid))
		b.op(wasmOpI32Ne)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp5, locTmp2) {
			return false
		}
		b.end()

		loadST := func(locPhys uint32) {
			b.localGet(locTmp3)
			b.localGet(locPhys)
			b.i32Const(3)
			b.op(wasmOpI32Shl)
			b.op(wasmOpI32Add)
			b.f64Load(3, 0)
		}
		if mapOp == 5 || mapOp == 7 {
			loadST(locTmp)
			loadST(locTmp2)
		} else {
			loadST(locTmp2)
			loadST(locTmp)
		}
		switch mapOp {
		case 0:
			b.op(wasmOpF64Add)
		case 1:
			b.op(wasmOpF64Mul)
		case 4, 5:
			b.op(wasmOpF64Sub)
		case 6, 7:
			b.op(wasmOpF64Div)
		}
		b.op(wasmOpI64ReinterpretF64)
		b.localSet(locI64Tmp)
		b.localGet(locI64Tmp)
		b.i64Const(expMask)
		b.op(wasmOpI64And)
		b.op(wasmOpI64Eqz)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp4, locTmp) {
			return false
		}
		b.end()
		b.localGet(locI64Tmp)
		b.i64Const(expMask)
		b.op(wasmOpI64And)
		b.i64Const(expMask)
		b.op(wasmOpI64Eq)
		b.ifVoid()
		if !x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp5, locTmp2) {
			return false
		}
		b.end()
		b.localGet(locTmp3)
		b.localGet(locTmp2)
		b.i32Const(3)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.localGet(locI64Tmp)
		b.i64Store(3, 0)
		x86WasmEmitFPUSetTagAtPhys(b, locTmp3, locTmp2, locTmp4, locTmp5)
		b.i32Const(int32(x87TagEmpty))
		b.localSet(locTmp5)
		x86WasmEmitFPUSetTagAtPhys(b, locTmp3, locTmp, locTmp5, locTmp4)
		x86WasmEmitFPUPhysFromTopPlus(b, locTmp, 1, locTmp)
		x86WasmEmitFPUSetTop(b, locTmp3, locTmp, locTmp4)
		return true
	case ji.opcode == 0xD9 && ji.modrm == 0xE4: // FTST
		return x86WasmEmitFPUCompareDirect(b, ji, memory, retired, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, true, false)
	case ji.opcode == 0xDD && ji.modrm>>6 == 3 && (ji.grpOp == 4 || ji.grpOp == 5): // FUCOM/FUCOMP ST(i)
		return x86WasmEmitFPUCompareDirect(b, ji, memory, retired, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, false, ji.grpOp == 5)
	default:
		return x86WasmEmitFPUHelperExit(b, ji, memory, retired, locCtx, locRegs, locTmp2, locTmp)
	}
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
	b.localGet(locB)
	b.op(wasmOpI32Xor)
	b.localGet(locResult)
	b.op(wasmOpI32Xor)
	b.i32Const(0x10)
	b.op(wasmOpI32And)
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

func x86WasmEmitArithFlags8(b *wasmBody, locCtx, locResult, locA, locB, locMasked, locFlagsPtr, locFlags, locScratch uint32, sub bool) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)

	b.localGet(locResult)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(locMasked)

	b.localGet(locMasked)
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
	b.localGet(locMasked)
	b.op(wasmOpI32Eqz)
	b.op(wasmOpSelect)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locMasked)
	b.i32Const(7)
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

	b.localGet(locFlags)
	if sub {
		b.localGet(locA)
		b.localGet(locB)
		b.op(wasmOpI32LtU)
	} else {
		b.localGet(locResult)
		b.i32Const(0xFF)
		b.op(wasmOpI32GtU)
	}
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locA)
	b.localGet(locB)
	b.op(wasmOpI32Xor)
	b.localGet(locMasked)
	b.op(wasmOpI32Xor)
	b.i32Const(0x10)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	if sub {
		b.localGet(locA)
		b.localGet(locB)
		b.op(wasmOpI32Xor)
	} else {
		b.localGet(locA)
		b.localGet(locB)
		b.op(wasmOpI32Xor)
		b.i32Const(-1)
		b.op(wasmOpI32Xor)
	}
	b.localGet(locA)
	b.localGet(locMasked)
	b.op(wasmOpI32Xor)
	b.op(wasmOpI32And)
	b.i32Const(7)
	b.op(wasmOpI32ShrU)
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(11)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlagsPtr)
	b.localGet(locFlags)
	b.i32Store(2, 0)
}

func x86WasmEmitSubFlagsWidth(b *wasmBody, locCtx, locA, locB, locResult, locFlagsPtr, locFlags uint32, width uint32) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)

	if width == 1 {
		b.localGet(locResult)
		b.i32Const(0xFF)
		b.op(wasmOpI32And)
		b.localSet(locResult)
	} else if width == 2 {
		b.localGet(locResult)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32And)
		b.localSet(locResult)
	}

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

	signShift := int32(width*8 - 1)
	b.localGet(locFlags)
	b.localGet(locResult)
	b.i32Const(signShift)
	b.op(wasmOpI32ShrU)
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(7)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locA)
	b.localGet(locB)
	b.op(wasmOpI32LtU)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locA)
	b.localGet(locB)
	b.op(wasmOpI32Xor)
	b.localGet(locResult)
	b.op(wasmOpI32Xor)
	b.i32Const(0x10)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlags)
	b.localGet(locA)
	b.localGet(locB)
	b.op(wasmOpI32Xor)
	b.localGet(locA)
	b.localGet(locResult)
	b.op(wasmOpI32Xor)
	b.op(wasmOpI32And)
	b.i32Const(signShift)
	b.op(wasmOpI32ShrU)
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(11)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locResult)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(locResult)
	for _, shift := range []int32{4, 2, 1} {
		b.localGet(locResult)
		b.localGet(locResult)
		b.i32Const(shift)
		b.op(wasmOpI32ShrU)
		b.op(wasmOpI32Xor)
		b.localSet(locResult)
	}
	b.localGet(locFlags)
	b.localGet(locResult)
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(1)
	b.op(wasmOpI32Xor)
	b.i32Const(2)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlagsPtr)
	b.localGet(locFlags)
	b.i32Store(2, 0)
}

func x86WasmEmitNEGFlagsWidth(b *wasmBody, locCtx, locOrig, locResult, locFlagsPtr, locFlags, locScratch uint32, width uint32) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)

	mask := int32(-1)
	signShift := int32(31)
	overflowConst := int32(-2147483648)
	switch width {
	case 1:
		mask = 0xFF
		signShift = 7
		overflowConst = 0x80
	case 2:
		mask = 0xFFFF
		signShift = 15
		overflowConst = 0x8000
	}
	if width != 4 {
		b.localGet(locResult)
		b.i32Const(mask)
		b.op(wasmOpI32And)
		b.localSet(locResult)
	}

	b.localGet(locFlags)
	b.i32Const(^int32(x86FlagCF | x86FlagPF | x86FlagAF | x86FlagZF | x86FlagSF | x86FlagOF))
	b.op(wasmOpI32And)
	b.localSet(locFlags)

	b.localGet(locOrig)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	b.elseBranch()
	b.localGet(locFlags)
	b.i32Const(int32(x86FlagCF))
	b.op(wasmOpI32Or)
	b.localSet(locFlags)
	b.end()

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
	b.i32Const(signShift)
	b.op(wasmOpI32ShrU)
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(7)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locOrig)
	b.i32Const(0xF)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	b.elseBranch()
	b.localGet(locFlags)
	b.i32Const(int32(x86FlagAF))
	b.op(wasmOpI32Or)
	b.localSet(locFlags)
	b.end()

	b.localGet(locOrig)
	b.i32Const(overflowConst)
	b.op(wasmOpI32Eq)
	b.ifVoid()
	b.localGet(locFlags)
	b.i32Const(int32(x86FlagOF))
	b.op(wasmOpI32Or)
	b.localSet(locFlags)
	b.elseBranch()
	b.end()

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
	b.localGet(locFlags)
	b.localGet(locScratch)
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(1)
	b.op(wasmOpI32Xor)
	b.i32Const(2)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlagsPtr)
	b.localGet(locFlags)
	b.i32Store(2, 0)
}

func x86WasmEmitMulOverflowFlags(b *wasmBody, locCtx, locA, locB, locResult, locFlagsPtr, locFlags, locScratch uint32, width uint32) {
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)

	if width == 2 {
		b.localGet(locResult)
		b.i32Const(16)
		b.op(wasmOpI32Shl)
		b.i32Const(16)
		b.op(wasmOpI32ShrS)
		b.localGet(locResult)
		b.op(wasmOpI32Ne)
	} else {
		b.localGet(locA)
		b.op(wasmOpI64ExtendI32S)
		b.localGet(locB)
		b.op(wasmOpI64ExtendI32S)
		b.op(wasmOpI64Mul)
		b.localGet(locResult)
		b.op(wasmOpI64ExtendI32S)
		b.op(wasmOpI64Ne)
	}
	b.localSet(locScratch)

	b.localGet(locFlags)
	b.i32Const(^int32(x86FlagCF | x86FlagOF))
	b.op(wasmOpI32And)
	b.localSet(locFlags)
	b.localGet(locScratch)
	b.ifVoid()
	b.localGet(locFlags)
	b.i32Const(int32(x86FlagCF | x86FlagOF))
	b.op(wasmOpI32Or)
	b.localSet(locFlags)
	b.end()
	b.localGet(locFlagsPtr)
	b.localGet(locFlags)
	b.i32Store(2, 0)
}

func x86WasmEmitDoubleShiftFlags(b *wasmBody, locCtx, locResult, locCF, locFlagsPtr, locFlags, locScratch uint32, width uint32) {
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
	b.i32Const(^int32(x86FlagCF | x86FlagPF | x86FlagZF | x86FlagSF))
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

	b.localGet(locFlags)
	b.localGet(locCF)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locFlagsPtr)
	b.localGet(locFlags)
	b.i32Store(2, 0)
}

func x86WasmEmitDoubleShiftValue(b *wasmBody, locDst, locSrc, locCount, locResult, locCF uint32, width uint32, right bool) {
	if width == 2 {
		b.localGet(locCount)
		b.i32Const(16)
		b.op(wasmOpI32LeU)
		b.ifVoid()
		if right {
			b.localGet(locDst)
			b.localGet(locCount)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.op(wasmOpI32ShrU)
			b.i32Const(1)
			b.op(wasmOpI32And)
			b.localSet(locCF)

			b.localGet(locDst)
			b.localGet(locCount)
			b.op(wasmOpI32ShrU)
			b.localGet(locSrc)
			b.i32Const(16)
			b.localGet(locCount)
			b.op(wasmOpI32Sub)
			b.op(wasmOpI32Shl)
			b.op(wasmOpI32Or)
		} else {
			b.localGet(locDst)
			b.i32Const(16)
			b.localGet(locCount)
			b.op(wasmOpI32Sub)
			b.op(wasmOpI32ShrU)
			b.i32Const(1)
			b.op(wasmOpI32And)
			b.localSet(locCF)

			b.localGet(locDst)
			b.localGet(locCount)
			b.op(wasmOpI32Shl)
			b.localGet(locSrc)
			b.i32Const(16)
			b.localGet(locCount)
			b.op(wasmOpI32Sub)
			b.op(wasmOpI32ShrU)
			b.op(wasmOpI32Or)
		}
		b.i32Const(0xFFFF)
		b.op(wasmOpI32And)
		b.localSet(locResult)
		b.elseBranch()
		if right {
			b.i32Const(0)
			b.localSet(locResult)
		} else {
			b.localGet(locDst)
			b.localGet(locCount)
			b.op(wasmOpI32Shl)
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
			b.localSet(locResult)
		}
		b.i32Const(0)
		b.localSet(locCF)
		b.end()
		return
	}
	if right {
		b.localGet(locDst)
		b.localGet(locCount)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.localSet(locCF)

		b.localGet(locDst)
		b.localGet(locCount)
		b.op(wasmOpI32ShrU)
		b.localGet(locSrc)
		b.i32Const(32)
		b.localGet(locCount)
		b.op(wasmOpI32Sub)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
	} else {
		b.localGet(locDst)
		b.i32Const(32)
		b.localGet(locCount)
		b.op(wasmOpI32Sub)
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.localSet(locCF)

		b.localGet(locDst)
		b.localGet(locCount)
		b.op(wasmOpI32Shl)
		b.localGet(locSrc)
		b.i32Const(32)
		b.localGet(locCount)
		b.op(wasmOpI32Sub)
		b.op(wasmOpI32ShrU)
		b.op(wasmOpI32Or)
	}
	b.localSet(locResult)
}

func x86WasmEmitShiftValue(b *wasmBody, grpOp byte, locOrig, locCount, locResult, locCF uint32, width uint32) {
	limit := int32(width * 8)
	switch grpOp {
	case 4, 6: // SHL/SAL
		if width != 4 {
			b.localGet(locCount)
			b.i32Const(limit)
			b.op(wasmOpI32GeU)
			b.ifVoid()
			b.i32Const(0)
			b.localSet(locCF)
			b.i32Const(0)
			b.localSet(locResult)
			b.elseBranch()
		}
		b.localGet(locOrig)
		b.i32Const(limit)
		b.localGet(locCount)
		b.op(wasmOpI32Sub)
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.localSet(locCF)
		b.localGet(locOrig)
		b.localGet(locCount)
		b.op(wasmOpI32Shl)
		if width == 1 {
			b.i32Const(0xFF)
			b.op(wasmOpI32And)
		} else if width == 2 {
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
		}
		b.localSet(locResult)
		if width != 4 {
			b.end()
		}
	case 5: // SHR
		if width != 4 {
			b.localGet(locCount)
			b.i32Const(limit)
			b.op(wasmOpI32GeU)
			b.ifVoid()
			b.i32Const(0)
			b.localSet(locCF)
			b.i32Const(0)
			b.localSet(locResult)
			b.elseBranch()
		}
		b.localGet(locOrig)
		b.localGet(locCount)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.localSet(locCF)
		b.localGet(locOrig)
		b.localGet(locCount)
		b.op(wasmOpI32ShrU)
		if width == 1 {
			b.i32Const(0xFF)
			b.op(wasmOpI32And)
		} else if width == 2 {
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
		}
		b.localSet(locResult)
		if width != 4 {
			b.end()
		}
	case 7: // SAR
		if width == 1 {
			b.localGet(locCount)
			b.i32Const(8)
			b.op(wasmOpI32GeU)
			b.ifVoid()
			b.i32Const(0)
			b.localSet(locCF)
			b.localGet(locOrig)
			b.i32Const(7)
			b.op(wasmOpI32ShrU)
			b.ifVoid()
			b.i32Const(0xFF)
			b.localSet(locResult)
			b.elseBranch()
			b.i32Const(0)
			b.localSet(locResult)
			b.end()
			b.elseBranch()
			b.localGet(locOrig)
			b.localGet(locCount)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.op(wasmOpI32ShrU)
			b.i32Const(1)
			b.op(wasmOpI32And)
			b.localSet(locCF)
			b.localGet(locOrig)
			b.i32Const(24)
			b.op(wasmOpI32Shl)
			b.i32Const(24)
			b.op(wasmOpI32ShrS)
			b.localGet(locCount)
			b.op(wasmOpI32ShrS)
			b.i32Const(0xFF)
			b.op(wasmOpI32And)
			b.localSet(locResult)
			b.end()
			return
		}
		if width == 2 {
			b.localGet(locCount)
			b.i32Const(16)
			b.op(wasmOpI32GeU)
			b.ifVoid()
			b.localGet(locOrig)
			b.i32Const(15)
			b.op(wasmOpI32ShrU)
			b.ifVoid()
			b.i32Const(1)
			b.localSet(locCF)
			b.i32Const(0xFFFF)
			b.localSet(locResult)
			b.elseBranch()
			b.i32Const(0)
			b.localSet(locCF)
			b.i32Const(0)
			b.localSet(locResult)
			b.end()
			b.elseBranch()
			b.localGet(locOrig)
			b.localGet(locCount)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.op(wasmOpI32ShrU)
			b.i32Const(1)
			b.op(wasmOpI32And)
			b.localSet(locCF)
			b.localGet(locOrig)
			b.i32Const(16)
			b.op(wasmOpI32Shl)
			b.i32Const(16)
			b.op(wasmOpI32ShrS)
			b.localGet(locCount)
			b.op(wasmOpI32ShrS)
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
			b.localSet(locResult)
			b.end()
			return
		}
		b.localGet(locOrig)
		b.localGet(locCount)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.localSet(locCF)
		b.localGet(locOrig)
		b.localGet(locCount)
		b.op(wasmOpI32ShrS)
		b.localSet(locResult)
	}
}

func x86WasmEmitRotateValue(b *wasmBody, grpOp byte, locOrig, locCount, locResult, locCF, locScratch uint32, width uint32) {
	bitWidth := int32(width * 8)
	switch grpOp {
	case 0: // ROL
		if width == 1 {
			b.localGet(locCount)
			b.i32Const(7)
			b.op(wasmOpI32And)
			b.localSet(locCount)
		} else if width == 2 {
			b.localGet(locCount)
			b.i32Const(15)
			b.op(wasmOpI32And)
			b.localSet(locCount)
		}
		if width == 4 {
			b.localGet(locOrig)
			b.localGet(locCount)
			b.op(wasmOpI32Rotl)
		} else {
			b.localGet(locOrig)
			b.localGet(locCount)
			b.op(wasmOpI32Shl)
			b.localGet(locOrig)
			b.i32Const(bitWidth)
			b.localGet(locCount)
			b.op(wasmOpI32Sub)
			b.op(wasmOpI32ShrU)
			b.op(wasmOpI32Or)
			if width == 1 {
				b.i32Const(0xFF)
				b.op(wasmOpI32And)
			} else {
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
		}
		b.localSet(locResult)
		b.localGet(locResult)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.localSet(locCF)
	case 1: // ROR
		if width == 1 {
			b.localGet(locCount)
			b.i32Const(7)
			b.op(wasmOpI32And)
			b.localSet(locCount)
		} else if width == 2 {
			b.localGet(locCount)
			b.i32Const(15)
			b.op(wasmOpI32And)
			b.localSet(locCount)
		}
		if width == 4 {
			b.localGet(locOrig)
			b.localGet(locCount)
			b.op(wasmOpI32Rotr)
		} else {
			b.localGet(locOrig)
			b.localGet(locCount)
			b.op(wasmOpI32ShrU)
			b.localGet(locOrig)
			b.i32Const(bitWidth)
			b.localGet(locCount)
			b.op(wasmOpI32Sub)
			b.op(wasmOpI32Shl)
			b.op(wasmOpI32Or)
			if width == 1 {
				b.i32Const(0xFF)
				b.op(wasmOpI32And)
			} else {
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
		}
		b.localSet(locResult)
		b.localGet(locResult)
		b.i32Const(bitWidth - 1)
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.localSet(locCF)
	case 2, 3: // RCL/RCR
		if width == 1 {
			b.localGet(locCount)
			b.i32Const(9)
			b.op(wasmOpI32RemU)
			b.localSet(locCount)
		} else if width == 2 {
			b.localGet(locCount)
			b.i32Const(17)
			b.op(wasmOpI32RemU)
			b.localSet(locCount)
		}
		b.localGet(locOrig)
		b.localSet(locResult)
		b.localGet(locCF)
		b.localSet(locScratch)
		b.block()
		b.loop()
		b.localGet(locCount)
		b.op(wasmOpI32Eqz)
		b.brIf(1)
		if grpOp == 2 {
			b.localGet(locResult)
			b.i32Const(bitWidth - 1)
			b.op(wasmOpI32ShrU)
			b.i32Const(1)
			b.op(wasmOpI32And)
			b.localSet(locCF)
			b.localGet(locResult)
			b.i32Const(1)
			b.op(wasmOpI32Shl)
			b.localGet(locScratch)
			b.op(wasmOpI32Or)
		} else {
			b.localGet(locResult)
			b.i32Const(1)
			b.op(wasmOpI32And)
			b.localSet(locCF)
			b.localGet(locResult)
			b.i32Const(1)
			b.op(wasmOpI32ShrU)
			b.localGet(locScratch)
			b.i32Const(bitWidth - 1)
			b.op(wasmOpI32Shl)
			b.op(wasmOpI32Or)
		}
		if width == 1 {
			b.i32Const(0xFF)
			b.op(wasmOpI32And)
		} else if width == 2 {
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
		}
		b.localSet(locResult)
		b.localGet(locCF)
		b.localSet(locScratch)
		b.localGet(locCount)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.localSet(locCount)
		b.br(0)
		b.end()
		b.end()
		b.localGet(locScratch)
		b.localSet(locCF)
	}
}

func x86WasmEmitRotateFlagsWidth(b *wasmBody, grpOp byte, locCtx, locCount, locResult, locCF, locFlagsPtr, locFlags uint32, width uint32) {
	bitWidth := width * 8
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)
	b.localGet(locFlags)
	b.i32Const(^int32(x86FlagCF))
	b.op(wasmOpI32And)
	b.localGet(locCF)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locCount)
	b.i32Const(1)
	b.op(wasmOpI32Eq)
	b.ifVoid()
	b.localGet(locFlags)
	b.i32Const(^int32(x86FlagOF))
	b.op(wasmOpI32And)
	if grpOp == 0 || grpOp == 2 {
		b.localGet(locResult)
		b.i32Const(int32(bitWidth - 1))
		b.op(wasmOpI32ShrU)
		b.localGet(locCF)
		b.op(wasmOpI32Xor)
	} else {
		b.localGet(locResult)
		b.i32Const(int32(bitWidth - 1))
		b.op(wasmOpI32ShrU)
		b.localGet(locResult)
		b.i32Const(int32(bitWidth - 2))
		b.op(wasmOpI32ShrU)
		b.i32Const(1)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Xor)
	}
	b.i32Const(1)
	b.op(wasmOpI32And)
	b.i32Const(11)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)
	b.end()

	b.localGet(locFlagsPtr)
	b.localGet(locFlags)
	b.i32Store(2, 0)
}

func x86WasmEmitShiftFlagsWidth(b *wasmBody, grpOp byte, locCtx, locOrig, locCount, locResult, locCF, locFlagsPtr, locFlags, locScratch uint32, width uint32) {
	bitWidth := width * 8
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locFlagsPtr)
	b.localGet(locFlagsPtr)
	b.i32Load(2, 0)
	b.localSet(locFlags)
	b.localGet(locFlags)
	b.i32Const(int32(x86FlagOF))
	b.op(wasmOpI32And)
	b.localSet(locFlagsPtr)

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
	b.i32Const(int32(bitWidth - 1))
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

	b.localGet(locFlags)
	b.localGet(locCF)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)

	b.localGet(locCount)
	b.i32Const(1)
	b.op(wasmOpI32Eq)
	b.ifVoid()
	switch grpOp {
	case 4, 6: // SHL/SAL
		b.localGet(locFlags)
		b.i32Const(^int32(x86FlagOF))
		b.op(wasmOpI32And)
		b.localGet(locResult)
		b.i32Const(int32(bitWidth - 1))
		b.op(wasmOpI32ShrU)
		b.localGet(locOrig)
		b.i32Const(int32(bitWidth - 1))
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
		b.i32Const(^int32(x86FlagOF))
		b.op(wasmOpI32And)
		b.localGet(locOrig)
		b.i32Const(int32(bitWidth - 1))
		b.op(wasmOpI32ShrU)
		b.i32Const(11)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
		b.localSet(locFlags)
	case 7: // SAR
		b.localGet(locFlags)
		b.i32Const(^int32(x86FlagOF))
		b.op(wasmOpI32And)
		b.localSet(locFlags)
	}
	b.elseBranch()
	b.localGet(locFlags)
	b.localGet(locFlagsPtr)
	b.op(wasmOpI32Or)
	b.localSet(locFlags)
	b.end()

	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffFlagsPtr)
	b.localSet(locScratch)
	b.localGet(locScratch)
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

func x86WasmEmitTerminalJcc(b *wasmBody, ji X86JITInstr, memory []byte, locCtx, locFlagsPtr, locFlags uint32, instrCount int, cycles, ticks uint32) bool {
	condition, target, nextPC, ok := x86WasmTerminalJccTarget(ji, memory)
	if !ok {
		return false
	}
	if !x86WasmEmitJccCondition(b, uint32(condition), locCtx, locFlagsPtr, locFlags) {
		return false
	}
	b.ifVoid()
	x86WasmEmitRetPCAndCount(b, target, instrCount, cycles, ticks)
	b.elseBranch()
	x86WasmEmitRetPCAndCount(b, nextPC, instrCount, cycles, ticks)
	b.end()
	return true
}

func x86WasmEmitTerminalLoop(b *wasmBody, ji X86JITInstr, memory []byte, locCtx, locRegs, locCount, locTmp, locFlagsPtr, locFlags uint32, instrCount int, cycles, ticks uint32) bool {
	op := byte(ji.opcode)
	if op < 0xE0 || op > 0xE3 || ji.prefixes&^(x86PrefAddrSize|x86PrefOpSize) != 0 || ji.length < 2 {
		return false
	}
	nextPC := ji.opcodePC + uint32(ji.length)
	target := uint32(int32(nextPC) + int32(int8(x86WasmImmediate8(ji, memory))))

	x86WasmEmitLoadReg32(b, locRegs, 1)
	b.localSet(locTmp)
	if ji.prefixes&x86PrefAddrSize != 0 {
		b.localGet(locTmp)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32And)
		b.localSet(locCount)
		if op != 0xE3 {
			b.localGet(locCount)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.localSet(locCount)
			b.localGet(locCount)
			x86WasmEmitInsertReg16(b, locRegs, locTmp, 1)
		}
	} else {
		if op == 0xE3 {
			b.localGet(locTmp)
			b.localSet(locCount)
		} else {
			b.localGet(locTmp)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.localSet(locCount)
			b.localGet(locCount)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, 1)
		}
	}

	b.localGet(locCount)
	b.op(wasmOpI32Eqz)
	if op == 0xE3 {
		// JCXZ/JECXZ: branch when count is zero.
	} else {
		// LOOP/LOOPE/LOOPNE: branch when decremented count is non-zero.
		b.op(wasmOpI32Eqz)
		if op == 0xE0 || op == 0xE1 {
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffFlagsPtr)
			b.localSet(locFlagsPtr)
			b.localGet(locFlagsPtr)
			b.i32Load(2, 0)
			b.localSet(locFlags)
			x86WasmEmitFlagPredicate(b, locFlags, x86FlagZF, op == 0xE0)
			b.op(wasmOpI32And)
		}
	}

	b.ifVoid()
	x86WasmEmitRetPCAndCount(b, target, instrCount, cycles, ticks)
	b.elseBranch()
	x86WasmEmitRetPCAndCount(b, nextPC, instrCount, cycles, ticks)
	b.end()
	return true
}

func x86WasmEmitInstr(b *wasmBody, ji X86JITInstr, memory []byte, retired int, cycles, ticks uint32, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, locTmp8 uint32) bool {
	if ji.opcode >= 0xD8 && ji.opcode <= 0xDF {
		return x86WasmEmitFPUDirectOrHelper(b, ji, memory, retired, locCtx, locRegs, locTmp, locTmp2, locTmp5, locTmp6, locTmp7)
	}
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
		case 0xA3: // BT r/m16/32, r16/32
			widthMask := int32(31)
			if ji.prefixes&x86PrefOpSize != 0 {
				widthMask = 15
			}
			x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
			if ji.prefixes&x86PrefOpSize != 0 {
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
			b.localSet(locTmp)
			x86WasmEmitLoadReg32(b, locRegs, (ji.modrm>>3)&7)
			b.i32Const(widthMask)
			b.op(wasmOpI32And)
			b.localSet(locTmp2)
			b.localGet(locTmp)
			b.localGet(locTmp2)
			b.op(wasmOpI32ShrU)
			b.i32Const(1)
			b.op(wasmOpI32And)
			b.localSet(locTmp3)
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffFlagsPtr)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.i32Load(2, 0)
			b.i32Const(^int32(x86FlagCF))
			b.op(wasmOpI32And)
			b.localGet(locTmp3)
			b.op(wasmOpI32Or)
			b.localSet(locTmp5)
			b.localGet(locTmp4)
			b.localGet(locTmp5)
			b.i32Store(2, 0)
			return true
		case 0xBA: // Grp8 BT r/m16/32, imm8
			if ji.grpOp != 4 {
				return false
			}
			widthMask := int32(31)
			if ji.prefixes&x86PrefOpSize != 0 {
				widthMask = 15
			}
			x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
			if ji.prefixes&x86PrefOpSize != 0 {
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
			b.localSet(locTmp)
			b.i32Const(int32(uint32(x86WasmImmediate8(ji, memory)) & uint32(widthMask)))
			b.localSet(locTmp2)
			b.localGet(locTmp)
			b.localGet(locTmp2)
			b.op(wasmOpI32ShrU)
			b.i32Const(1)
			b.op(wasmOpI32And)
			b.localSet(locTmp3)
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffFlagsPtr)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.i32Load(2, 0)
			b.i32Const(^int32(x86FlagCF))
			b.op(wasmOpI32And)
			b.localGet(locTmp3)
			b.op(wasmOpI32Or)
			b.localSet(locTmp5)
			b.localGet(locTmp4)
			b.localGet(locTmp5)
			b.i32Store(2, 0)
			return true
		case 0xA4, 0xA5, 0xAC, 0xAD: // SHLD/SHRD Ev, Gv, Ib/CL
			width := uint32(4)
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 2
			}
			if ji.modrm>>6 == 3 {
				x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
				if width == 2 {
					b.i32Const(0xFFFF)
					b.op(wasmOpI32And)
				}
				b.localSet(locTmp)
			} else {
				if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp2) {
					return false
				}
				x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp3, locTmp4, width, ji.opcodePC, retired)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffMemPtr)
				b.localGet(locTmp2)
				b.op(wasmOpI32Add)
				if width == 2 {
					b.i32Load16U(1, 0)
				} else {
					b.i32Load(2, 0)
				}
				b.localSet(locTmp)
			}
			x86WasmEmitLoadReg32(b, locRegs, (ji.modrm>>3)&7)
			if width == 2 {
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
			b.localSet(locTmp3)
			if op2 == 0xA4 || op2 == 0xAC {
				b.i32Const(int32(uint32(x86WasmImmediate8(ji, memory)) & 0x1F))
			} else {
				x86WasmEmitExtractReg8(b, locRegs, 1)
				b.i32Const(0x1F)
				b.op(wasmOpI32And)
			}
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.ifVoid()
			b.elseBranch()
			x86WasmEmitDoubleShiftValue(b, locTmp, locTmp3, locTmp4, locTmp5, locTmp6, width, op2 == 0xAC || op2 == 0xAD)
			if ji.modrm>>6 == 3 {
				if width == 2 {
					b.localGet(locTmp5)
					x86WasmEmitInsertReg16(b, locRegs, locTmp7, ji.modrm&7)
				} else {
					b.localGet(locTmp5)
					x86WasmEmitStoreReg32(b, locRegs, locTmp7, ji.modrm&7)
				}
			} else {
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffMemPtr)
				b.localGet(locTmp2)
				b.op(wasmOpI32Add)
				b.localGet(locTmp5)
				if width == 2 {
					b.i32Store16(1, 0)
				} else {
					b.i32Store(2, 0)
				}
				x86WasmEmitSMCStoreCheck(b, locCtx, locTmp2, locTmp7, width, ji.opcodePC+uint32(ji.length), retired+1)
			}
			x86WasmEmitDoubleShiftFlags(b, locCtx, locTmp5, locTmp6, locTmp7, locTmp2, locTmp3, width)
			b.end()
			return true
		case 0xBC, 0xBD: // BSF/BSR r32/16, r/m32/16
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			width := uint32(4)
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 2
			}
			if ji.modrm>>6 == 3 {
				x86WasmEmitLoadReg32(b, locRegs, src)
				if width == 2 {
					b.i32Const(0xFFFF)
					b.op(wasmOpI32And)
				}
			} else {
				if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
					return false
				}
				x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffMemPtr)
				b.localGet(locTmp)
				b.op(wasmOpI32Add)
				if width == 2 {
					b.i32Load16U(1, 0)
				} else {
					b.i32Load(2, 0)
				}
			}
			b.localSet(locTmp)
			b.localGet(locTmp)
			b.op(wasmOpI32Eqz)
			b.ifVoid()
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffFlagsPtr)
			b.localSet(locTmp2)
			b.localGet(locTmp2)
			b.i32Load(2, 0)
			b.i32Const(int32(x86FlagZF))
			b.op(wasmOpI32Or)
			b.localSet(locTmp3)
			b.localGet(locTmp2)
			b.localGet(locTmp3)
			b.i32Store(2, 0)
			b.elseBranch()
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffFlagsPtr)
			b.localSet(locTmp2)
			b.localGet(locTmp2)
			b.i32Load(2, 0)
			b.i32Const(^int32(x86FlagZF))
			b.op(wasmOpI32And)
			b.localSet(locTmp3)
			b.localGet(locTmp)
			if op2 == 0xBC {
				b.op(wasmOpI32Ctz)
			} else {
				b.op(wasmOpI32Clz)
				b.i32Const(31)
				b.op(wasmOpI32Xor)
			}
			if width == 2 {
				x86WasmEmitInsertReg16(b, locRegs, locTmp4, dst)
			} else {
				x86WasmEmitStoreReg32(b, locRegs, locTmp4, dst)
			}
			b.localGet(locTmp2)
			b.localGet(locTmp3)
			b.i32Store(2, 0)
			b.end()
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
	case op == 0x98: // CBW/CWDE
		x86WasmEmitLoadReg32(b, locRegs, 0)
		if ji.prefixes&x86PrefOpSize != 0 {
			b.i32Const(24)
			b.op(wasmOpI32Shl)
			b.i32Const(24)
			b.op(wasmOpI32ShrS)
			x86WasmEmitInsertReg16(b, locRegs, locTmp, 0)
		} else {
			b.i32Const(16)
			b.op(wasmOpI32Shl)
			b.i32Const(16)
			b.op(wasmOpI32ShrS)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, 0)
		}
		return true
	case op == 0x99: // CWD/CDQ
		x86WasmEmitLoadReg32(b, locRegs, 0)
		if ji.prefixes&x86PrefOpSize != 0 {
			b.i32Const(15)
			b.op(wasmOpI32ShrU)
			b.i32Const(1)
			b.op(wasmOpI32And)
			b.ifVoid()
			b.i32Const(0xFFFF)
			x86WasmEmitInsertReg16(b, locRegs, locTmp, 2)
			b.elseBranch()
			b.i32Const(0)
			x86WasmEmitInsertReg16(b, locRegs, locTmp, 2)
			b.end()
		} else {
			b.i32Const(31)
			b.op(wasmOpI32ShrU)
			b.ifVoid()
			b.i32Const(-1)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, 2)
			b.elseBranch()
			b.i32Const(0)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, 2)
			b.end()
		}
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
	case op == 0x8C && ji.hasModRM: // MOV Ev,Sreg
		seg := (ji.modrm >> 3) & 7
		if ji.modrm>>6 == 3 {
			if seg <= x86SegGS {
				x86WasmEmitLoadSeg16(b, locCtx, locTmp2, seg)
			} else {
				b.i32Const(0)
			}
			x86WasmEmitInsertReg16(b, locRegs, locTmp, ji.modrm&7)
			return true
		}
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if seg <= x86SegGS {
			x86WasmEmitLoadSeg16(b, locCtx, locTmp2, seg)
		} else {
			b.i32Const(0)
		}
		if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp2, width, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0x8E && ji.hasModRM: // MOV Sreg,Ev
		seg := (ji.modrm >> 3) & 7
		if ji.modrm>>6 == 3 {
			if seg > x86SegGS {
				return true
			}
			x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
			x86WasmEmitStoreSeg16(b, locCtx, locTmp2, locTmp3, seg)
			return true
		}
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		if seg > x86SegGS {
			return true
		}
		x86WasmEmitStoreSeg16(b, locCtx, locTmp2, locTmp3, seg)
		return true
	case op == 0xC4 || op == 0xC5: // LES/LDS Ev,Mp
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if ji.modrm>>6 == 3 {
			x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
			b.localSet(locTmp)
		} else if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width+2, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localSet(locTmp2)
		dst := (ji.modrm >> 3) & 7
		b.localGet(locTmp2)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Load16U(1, 0)
			x86WasmEmitInsertReg16(b, locRegs, locTmp3, dst)
		} else {
			b.i32Load(2, 0)
			x86WasmEmitStoreReg32(b, locRegs, locTmp3, dst)
		}
		b.localGet(locTmp)
		b.i32Const(int32(width))
		b.op(wasmOpI32Add)
		b.localSet(locTmp3)
		b.localGet(locTmp2)
		b.localGet(locTmp3)
		b.op(wasmOpI32Add)
		b.i32Load16U(1, 0)
		if op == 0xC4 {
			x86WasmEmitStoreSeg16(b, locCtx, locTmp4, locTmp5, x86SegES)
		} else {
			x86WasmEmitStoreSeg16(b, locCtx, locTmp4, locTmp5, x86SegDS)
		}
		return true
	case op >= 0x50 && op <= 0x57: // PUSH reg
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		reg := op - 0x50
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		if width == 2 {
			if reg == 4 {
				b.localGet(locTmp)
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			} else {
				x86WasmEmitLoadReg32(b, locRegs, reg)
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
		} else {
			if reg == 4 {
				b.localGet(locTmp)
			} else {
				x86WasmEmitLoadReg32(b, locRegs, reg)
			}
		}
		b.localSet(locTmp3)
		b.localGet(locTmp)
		b.i32Const(int32(width))
		b.op(wasmOpI32Sub)
		b.localSet(locTmp2)
		x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp4, locTmp5, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp2)
		b.op(wasmOpI32Add)
		b.localGet(locTmp3)
		if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp4, 4)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp2, locTmp4, width, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op >= 0x58 && op <= 0x5F: // POP reg
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		reg := op - 0x58
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		b.localSet(locTmp2)
		b.localGet(locTmp)
		b.i32Const(int32(width))
		b.op(wasmOpI32Add)
		b.localSet(locTmp3)
		if reg == 4 {
			if width == 2 {
				b.localGet(locTmp2)
				x86WasmEmitInsertReg16(b, locRegs, locTmp4, 4)
			} else {
				b.localGet(locTmp2)
				x86WasmEmitStoreReg32(b, locRegs, locTmp4, 4)
			}
		} else {
			if width == 2 {
				b.localGet(locTmp2)
				x86WasmEmitInsertReg16(b, locRegs, locTmp4, reg)
			} else {
				b.localGet(locTmp2)
				x86WasmEmitStoreReg32(b, locRegs, locTmp4, reg)
			}
			b.localGet(locTmp3)
			x86WasmEmitStoreReg32(b, locRegs, locTmp4, 4)
		}
		return true
	case op == 0x60: // PUSHA / PUSHAD
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		total := width * 8
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		b.localGet(locTmp)
		b.i32Const(int32(total))
		b.op(wasmOpI32Sub)
		b.localSet(locTmp2)
		x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp4, locTmp5, total, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localSet(locTmp3)
		storePUSHA := func(off uint32, reg byte, originalESP bool) {
			b.localGet(locTmp3)
			b.localGet(locTmp2)
			b.op(wasmOpI32Add)
			if off != 0 {
				b.i32Const(int32(off))
				b.op(wasmOpI32Add)
			}
			if originalESP {
				b.localGet(locTmp)
			} else {
				x86WasmEmitLoadReg32(b, locRegs, reg)
			}
			if width == 2 {
				b.i32Store16(1, 0)
			} else {
				b.i32Store(2, 0)
			}
		}
		storePUSHA(0*width, 7, false)
		storePUSHA(1*width, 6, false)
		storePUSHA(2*width, 5, false)
		storePUSHA(3*width, 4, true)
		storePUSHA(4*width, 3, false)
		storePUSHA(5*width, 2, false)
		storePUSHA(6*width, 1, false)
		storePUSHA(7*width, 0, false)
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp4, 4)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp2, locTmp4, total, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0x61: // POPA / POPAD
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		total := width * 8
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, total, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localSet(locTmp2)
		loadPOPA := func(off uint32, reg byte) {
			b.localGet(locTmp2)
			b.localGet(locTmp)
			b.op(wasmOpI32Add)
			if off != 0 {
				b.i32Const(int32(off))
				b.op(wasmOpI32Add)
			}
			if width == 2 {
				b.i32Load16U(1, 0)
				x86WasmEmitInsertReg16(b, locRegs, locTmp3, reg)
			} else {
				b.i32Load(2, 0)
				x86WasmEmitStoreReg32(b, locRegs, locTmp3, reg)
			}
		}
		loadPOPA(0*width, 7)
		loadPOPA(1*width, 6)
		loadPOPA(2*width, 5)
		loadPOPA(4*width, 3)
		loadPOPA(5*width, 2)
		loadPOPA(6*width, 1)
		loadPOPA(7*width, 0)
		b.localGet(locTmp)
		b.i32Const(int32(total))
		b.op(wasmOpI32Add)
		x86WasmEmitStoreReg32(b, locRegs, locTmp3, 4)
		return true
	case op == 0x06 || op == 0x0E || op == 0x16 || op == 0x1E: // PUSH ES/CS/SS/DS
		var seg byte
		switch op {
		case 0x06:
			seg = 0
		case 0x0E:
			seg = 1
		case 0x16:
			seg = 2
		default:
			seg = 3
		}
		x86WasmEmitLoadSeg16(b, locCtx, locTmp4, seg)
		b.localSet(locTmp3)
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		b.localGet(locTmp)
		b.i32Const(2)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp2)
		x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp4, locTmp5, 2, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp2)
		b.op(wasmOpI32Add)
		b.localGet(locTmp3)
		b.i32Store16(1, 0)
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp4, 4)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp2, locTmp4, 2, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0x07 || op == 0x17 || op == 0x1F: // POP ES/SS/DS
		var seg byte
		switch op {
		case 0x07:
			seg = 0
		case 0x17:
			seg = 2
		default:
			seg = 3
		}
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 2, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Load16U(1, 0)
		b.localSet(locTmp2)
		b.localGet(locTmp)
		b.i32Const(2)
		b.op(wasmOpI32Add)
		x86WasmEmitStoreReg32(b, locRegs, locTmp4, 4)
		b.localGet(locTmp2)
		x86WasmEmitStoreSeg16(b, locCtx, locTmp4, locTmp5, seg)
		return true
	case op == 0x68 || op == 0x6A: // PUSH imm
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		if op == 0x68 {
			if width == 2 {
				b.i32Const(int32(uint32(x86WasmImmediate16(ji, memory))))
			} else {
				b.i32Const(int32(x86WasmImmediate32(ji, memory)))
			}
		} else {
			imm8 := int8(x86WasmImmediate8(ji, memory))
			if width == 2 {
				b.i32Const(int32(uint16(int16(imm8))))
			} else {
				b.i32Const(int32(imm8))
			}
		}
		b.localSet(locTmp3)
		b.localGet(locTmp)
		b.i32Const(int32(width))
		b.op(wasmOpI32Sub)
		b.localSet(locTmp2)
		x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp4, locTmp5, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp2)
		b.op(wasmOpI32Add)
		b.localGet(locTmp3)
		if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp4, 4)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp2, locTmp4, width, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0x69 || op == 0x6B: // IMUL Gv, Ev, imm
		width := uint32(4)
		imm := int32(x86WasmImmediate32(ji, memory))
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
			imm = int32(int16(x86WasmImmediate16(ji, memory)))
		}
		if op == 0x6B {
			imm = int32(int8(x86WasmImmediate8(ji, memory)))
		}
		dst := (ji.modrm >> 3) & 7
		if ji.modrm>>6 == 3 {
			x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
			if width == 2 {
				b.i32Const(16)
				b.op(wasmOpI32Shl)
				b.i32Const(16)
				b.op(wasmOpI32ShrS)
			}
		} else {
			if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
				return false
			}
			x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffMemPtr)
			b.localGet(locTmp)
			b.op(wasmOpI32Add)
			if width == 2 {
				b.i32Load16U(1, 0)
				b.i32Const(16)
				b.op(wasmOpI32Shl)
				b.i32Const(16)
				b.op(wasmOpI32ShrS)
			} else {
				b.i32Load(2, 0)
			}
		}
		b.localSet(locTmp3)
		b.i32Const(imm)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Mul)
		b.localSet(locTmp)
		if width == 2 {
			b.localGet(locTmp)
			x86WasmEmitInsertReg16(b, locRegs, locTmp2, dst)
		} else {
			b.localGet(locTmp)
			x86WasmEmitStoreReg32(b, locRegs, locTmp2, dst)
		}
		x86WasmEmitMulOverflowFlags(b, locCtx, locTmp3, locTmp4, locTmp, locTmp2, locTmp5, locTmp6, width)
		return true
	case op == 0x9C: // PUSHF
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFlagsPtr)
		b.i32Load(2, 0)
		if width == 2 {
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
		}
		b.localSet(locTmp3)
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		b.localGet(locTmp)
		b.i32Const(int32(width))
		b.op(wasmOpI32Sub)
		b.localSet(locTmp2)
		x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp4, locTmp5, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp2)
		b.op(wasmOpI32Add)
		b.localGet(locTmp3)
		if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp4, 4)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp2, locTmp4, width, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0x9D: // POPF
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		b.localSet(locTmp2)
		b.localGet(locTmp)
		b.i32Const(int32(width))
		b.op(wasmOpI32Add)
		x86WasmEmitStoreReg32(b, locRegs, locTmp3, 4)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFlagsPtr)
		b.localSet(locTmp4)
		if width == 2 {
			b.localGet(locTmp4)
			b.i32Load(2, 0)
			b.i32Const(^int32(0xFFFF))
			b.op(wasmOpI32And)
			b.localGet(locTmp2)
			b.op(wasmOpI32Or)
			b.localSet(locTmp5)
			b.localGet(locTmp4)
			b.localGet(locTmp5)
			b.i32Store(2, 0)
		} else {
			b.localGet(locTmp4)
			b.localGet(locTmp2)
			b.i32Store(2, 0)
		}
		return true
	case op == 0xD6: // SALC
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFlagsPtr)
		b.localSet(locTmp2)
		b.localGet(locTmp2)
		b.i32Load(2, 0)
		b.i32Const(int32(x86FlagCF))
		b.op(wasmOpI32And)
		b.ifVoid()
		b.i32Const(0xFF)
		x86WasmEmitInsertReg8(b, locRegs, locTmp, 0)
		b.elseBranch()
		b.i32Const(0)
		x86WasmEmitInsertReg8(b, locRegs, locTmp, 0)
		b.end()
		return true
	case op == 0xD7: // XLAT
		x86WasmEmitLoadReg32(b, locRegs, 3)
		x86WasmEmitExtractReg8(b, locRegs, 0)
		b.op(wasmOpI32Add)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 1, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Load8U(0, 0)
		x86WasmEmitInsertReg8(b, locRegs, locTmp2, 0)
		return true
	case op == 0xC8: // ENTER imm16,0
		immPC := int(ji.opcodePC) + int(ji.length) - 3
		if immPC < 0 || immPC+3 > len(memory) || memory[immPC+2]&0x1F != 0 {
			return false
		}
		frameSize := uint32(uint16(memory[immPC]) | uint16(memory[immPC+1])<<8)
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		b.localGet(locTmp)
		b.i32Const(4)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp2)
		x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp4, locTmp5, 4, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp2)
		b.op(wasmOpI32Add)
		x86WasmEmitLoadReg32(b, locRegs, 5)
		b.i32Store(2, 0)
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp4, 5)
		b.localGet(locTmp2)
		b.i32Const(int32(frameSize))
		b.op(wasmOpI32Sub)
		x86WasmEmitStoreReg32(b, locRegs, locTmp4, 4)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp2, locTmp4, 4, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0xC9: // LEAVE
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if width == 2 {
			x86WasmEmitLoadReg32(b, locRegs, 4)
			b.i32Const(^int32(0xFFFF))
			b.op(wasmOpI32And)
			b.localSet(locTmp2)
			x86WasmEmitLoadReg32(b, locRegs, 5)
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
			b.localGet(locTmp2)
			b.op(wasmOpI32Or)
			b.localSet(locTmp)
		} else {
			x86WasmEmitLoadReg32(b, locRegs, 5)
			b.localSet(locTmp)
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		b.localSet(locTmp4)
		b.localGet(locTmp)
		b.i32Const(int32(width))
		b.op(wasmOpI32Add)
		x86WasmEmitStoreReg32(b, locRegs, locTmp3, 4)
		if width == 2 {
			b.localGet(locTmp4)
			x86WasmEmitInsertReg16(b, locRegs, locTmp3, 5)
		} else {
			b.localGet(locTmp4)
			x86WasmEmitStoreReg32(b, locRegs, locTmp3, 5)
		}
		return true
	case op == 0xA4 || op == 0xA5: // MOVS
		width := uint32(1)
		rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
		if op == 0xA5 {
			width = 4
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 2
			}
		}
		if rep {
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.ifVoid()
			b.elseBranch()
			b.localGet(locTmp4)
			b.localSet(locTmp5)
			b.block()
			b.loop()
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.brIf(1)
		}
		x86WasmEmitLoadReg32(b, locRegs, 6)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp6, locTmp7, width, ji.opcodePC, retired)
		x86WasmEmitLoadReg32(b, locRegs, 7)
		b.localSet(locTmp2)
		x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp6, locTmp7, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 1 {
			b.i32Load8U(0, 0)
		} else if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		b.localSet(locTmp3)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp2)
		b.op(wasmOpI32Add)
		b.localGet(locTmp3)
		if width == 1 {
			b.i32Store8(0, 0)
		} else if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		x86WasmEmitStringAdvance(b, locCtx, locTmp, locTmp6, locTmp7, width)
		x86WasmEmitStringAdvance(b, locCtx, locTmp2, locTmp6, locTmp7, width)
		if rep {
			b.localGet(locTmp4)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			x86WasmEmitStoreReg32(b, locRegs, locTmp6, 1)
		}
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp6, 6)
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp6, 7)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp2, locTmp6, width, ji.opcodePC+uint32(ji.length), retired+1)
		if !rep {
			b.i32Const(1)
			b.localSet(locTmp6)
			b.i32Const(0)
			b.localSet(locTmp7)
			x86WasmEmitDynamicChainCredit(b, locCtx, locTmp6, locTmp7)
		}
		if rep {
			b.br(0)
			b.end()
			b.end()
			x86WasmEmitStringDynamicCredit(b, locCtx, locTmp5, locTmp4, locTmp6, locTmp7)
			b.end()
		}
		return true
	case op == 0xAA || op == 0xAB: // STOS
		width := uint32(1)
		rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
		if op == 0xAB {
			width = 4
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 2
			}
		}
		if rep {
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.ifVoid()
			b.elseBranch()
			b.localGet(locTmp4)
			b.localSet(locTmp5)
			b.block()
			b.loop()
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.brIf(1)
		}
		x86WasmEmitLoadReg32(b, locRegs, 7)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp6, locTmp7, width, ji.opcodePC, retired)
		if width == 1 {
			x86WasmEmitExtractReg8(b, locRegs, 0)
		} else {
			x86WasmEmitLoadReg32(b, locRegs, 0)
			if width == 2 {
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
		}
		b.localSet(locTmp2)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.localGet(locTmp2)
		if width == 1 {
			b.i32Store8(0, 0)
		} else if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		x86WasmEmitStringAdvance(b, locCtx, locTmp, locTmp6, locTmp7, width)
		if rep {
			b.localGet(locTmp4)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			x86WasmEmitStoreReg32(b, locRegs, locTmp6, 1)
		}
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp6, 7)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp6, width, ji.opcodePC+uint32(ji.length), retired+1)
		if !rep {
			b.i32Const(1)
			b.localSet(locTmp6)
			b.i32Const(0)
			b.localSet(locTmp7)
			x86WasmEmitDynamicChainCredit(b, locCtx, locTmp6, locTmp7)
		}
		if rep {
			b.br(0)
			b.end()
			b.end()
			x86WasmEmitStringDynamicCredit(b, locCtx, locTmp5, locTmp4, locTmp6, locTmp7)
			b.end()
		}
		return true
	case op == 0xAC || op == 0xAD: // LODS
		width := uint32(1)
		rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
		if op == 0xAD {
			width = 4
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 2
			}
		}
		if rep {
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.ifVoid()
			b.elseBranch()
			b.localGet(locTmp4)
			b.localSet(locTmp5)
			b.block()
			b.loop()
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.brIf(1)
		}
		x86WasmEmitLoadReg32(b, locRegs, 6)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp6, locTmp7, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 1 {
			b.i32Load8U(0, 0)
			b.localSet(locTmp2)
			b.localGet(locTmp2)
			x86WasmEmitInsertReg8(b, locRegs, locTmp6, 0)
		} else if width == 2 {
			b.i32Load16U(1, 0)
			b.localSet(locTmp2)
			b.localGet(locTmp2)
			x86WasmEmitInsertReg16(b, locRegs, locTmp6, 0)
		} else {
			b.i32Load(2, 0)
			x86WasmEmitStoreReg32(b, locRegs, locTmp6, 0)
		}
		x86WasmEmitStringAdvance(b, locCtx, locTmp, locTmp6, locTmp7, width)
		if rep {
			b.localGet(locTmp4)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			x86WasmEmitStoreReg32(b, locRegs, locTmp6, 1)
		}
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp6, 6)
		if !rep {
			b.i32Const(1)
			b.localSet(locTmp6)
			b.i32Const(0)
			b.localSet(locTmp7)
			x86WasmEmitDynamicChainCredit(b, locCtx, locTmp6, locTmp7)
		}
		if rep {
			b.br(0)
			b.end()
			b.end()
			x86WasmEmitStringDynamicCredit(b, locCtx, locTmp5, locTmp4, locTmp6, locTmp7)
			b.end()
		}
		return true
	case op == 0xA6 || op == 0xA7: // CMPS
		width := uint32(1)
		rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
		repNE := ji.prefixes&x86PrefRepNE != 0
		if op == 0xA7 {
			width = 4
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 2
			}
		}
		if rep {
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.ifVoid()
			b.elseBranch()
			b.localGet(locTmp4)
			b.localSet(locTmp5)
			b.block()
			b.loop()
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.brIf(1)
		}
		x86WasmEmitLoadReg32(b, locRegs, 6)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp6, locTmp7, width, ji.opcodePC, retired)
		x86WasmEmitLoadReg32(b, locRegs, 7)
		b.localSet(locTmp2)
		x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp6, locTmp7, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 1 {
			b.i32Load8U(0, 0)
		} else if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		b.localSet(locTmp3)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp2)
		b.op(wasmOpI32Add)
		if width == 1 {
			b.i32Load8U(0, 0)
		} else if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		b.localSet(locTmp6)
		b.localGet(locTmp3)
		b.localGet(locTmp6)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp7)
		b.localGet(locTmp7)
		b.op(wasmOpI32Eqz)
		b.localSet(locTmp8)
		x86WasmEmitSubFlagsWidth(b, locCtx, locTmp3, locTmp6, locTmp7, locTmp, locTmp2, width)
		x86WasmEmitLoadReg32(b, locRegs, 6)
		b.localSet(locTmp)
		x86WasmEmitLoadReg32(b, locRegs, 7)
		b.localSet(locTmp2)
		x86WasmEmitStringAdvance(b, locCtx, locTmp, locTmp6, locTmp7, width)
		x86WasmEmitStringAdvance(b, locCtx, locTmp2, locTmp6, locTmp7, width)
		if rep {
			b.localGet(locTmp4)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			x86WasmEmitStoreReg32(b, locRegs, locTmp7, 1)
		}
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp7, 6)
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp7, 7)
		if !rep {
			b.i32Const(1)
			b.localSet(locTmp6)
			b.i32Const(0)
			b.localSet(locTmp7)
			x86WasmEmitDynamicChainCredit(b, locCtx, locTmp6, locTmp7)
		}
		if rep {
			b.localGet(locTmp8)
			if repNE {
				b.brIf(1)
				b.br(0)
			} else {
				b.ifVoid()
				b.br(1)
				b.elseBranch()
				b.br(2)
				b.end()
			}
			b.end()
			b.end()
			x86WasmEmitStringDynamicCredit(b, locCtx, locTmp5, locTmp4, locTmp6, locTmp7)
			b.end()
		}
		return true
	case op == 0xAE || op == 0xAF: // SCAS
		width := uint32(1)
		rep := ji.prefixes&(x86PrefRep|x86PrefRepNE) != 0
		repNE := ji.prefixes&x86PrefRepNE != 0
		if op == 0xAF {
			width = 4
			if ji.prefixes&x86PrefOpSize != 0 {
				width = 2
			}
		}
		if rep {
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.ifVoid()
			b.elseBranch()
			b.localGet(locTmp4)
			b.localSet(locTmp5)
			b.block()
			b.loop()
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			b.op(wasmOpI32Eqz)
			b.brIf(1)
		}
		x86WasmEmitLoadReg32(b, locRegs, 7)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp6, locTmp7, width, ji.opcodePC, retired)
		if width == 1 {
			x86WasmEmitExtractReg8(b, locRegs, 0)
		} else {
			x86WasmEmitLoadReg32(b, locRegs, 0)
			if width == 2 {
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
		}
		b.localSet(locTmp2)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 1 {
			b.i32Load8U(0, 0)
		} else if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		b.localSet(locTmp6)
		b.localGet(locTmp2)
		b.localGet(locTmp6)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp7)
		b.localGet(locTmp7)
		b.op(wasmOpI32Eqz)
		b.localSet(locTmp8)
		x86WasmEmitSubFlagsWidth(b, locCtx, locTmp2, locTmp6, locTmp7, locTmp, locTmp3, width)
		x86WasmEmitLoadReg32(b, locRegs, 7)
		b.localSet(locTmp)
		x86WasmEmitStringAdvance(b, locCtx, locTmp, locTmp6, locTmp7, width)
		if rep {
			b.localGet(locTmp4)
			b.i32Const(1)
			b.op(wasmOpI32Sub)
			b.localSet(locTmp4)
			b.localGet(locTmp4)
			x86WasmEmitStoreReg32(b, locRegs, locTmp6, 1)
		}
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp6, 7)
		if !rep {
			b.i32Const(1)
			b.localSet(locTmp6)
			b.i32Const(0)
			b.localSet(locTmp7)
			x86WasmEmitDynamicChainCredit(b, locCtx, locTmp6, locTmp7)
		}
		if rep {
			b.localGet(locTmp8)
			if repNE {
				b.brIf(1)
				b.br(0)
			} else {
				b.ifVoid()
				b.br(1)
				b.elseBranch()
				b.br(2)
				b.end()
			}
			b.end()
			b.end()
			x86WasmEmitStringDynamicCredit(b, locCtx, locTmp5, locTmp4, locTmp6, locTmp7)
			b.end()
		}
		return true
	case op == 0xFA || op == 0xFB: // CLI/STI
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFlagsPtr)
		b.localSet(locTmp2)
		b.localGet(locTmp2)
		b.i32Load(2, 0)
		if op == 0xFA {
			b.i32Const(^int32(x86FlagIF))
			b.op(wasmOpI32And)
		} else {
			b.i32Const(int32(x86FlagIF))
			b.op(wasmOpI32Or)
		}
		b.localSet(locTmp3)
		b.localGet(locTmp2)
		b.localGet(locTmp3)
		b.i32Store(2, 0)
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
	case op == 0xD0 || op == 0xD1 || op == 0xC0 || op == 0xC1 || op == 0xD2 || op == 0xD3:
		width := uint32(4)
		if op == 0xD0 || op == 0xC0 || op == 0xD2 {
			width = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		switch op {
		case 0xD0, 0xD1:
			b.i32Const(1)
		case 0xC0, 0xC1:
			b.i32Const(int32(uint32(x86WasmImmediate8(ji, memory)) & 31))
		case 0xD2:
			x86WasmEmitExtractReg8(b, locRegs, 1)
			b.i32Const(31)
			b.op(wasmOpI32And)
		case 0xD3:
			x86WasmEmitLoadReg32(b, locRegs, 1)
			b.i32Const(31)
			b.op(wasmOpI32And)
		}
		b.localSet(locTmp4)
		b.localGet(locTmp4)
		b.op(wasmOpI32Eqz)
		b.ifVoid()
		b.elseBranch()
		if op == 0xD0 || op == 0xD1 {
			if ji.modrm>>6 == 3 {
				if width == 1 {
					x86WasmEmitExtractReg8(b, locRegs, ji.modrm&7)
				} else {
					x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
					if width == 2 {
						b.i32Const(0xFFFF)
						b.op(wasmOpI32And)
					}
				}
			} else {
				if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp2) {
					return false
				}
				x86WasmEmitSpanGuard(b, locCtx, locTmp2, locTmp5, locTmp6, width, ji.opcodePC, retired)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffMemPtr)
				b.localGet(locTmp2)
				b.op(wasmOpI32Add)
				if width == 1 {
					b.i32Load8U(0, 0)
				} else if width == 2 {
					b.i32Load16U(1, 0)
				} else {
					b.i32Load(2, 0)
				}
			}
		} else {
			if width == 1 {
				x86WasmEmitExtractReg8(b, locRegs, ji.modrm&7)
			} else {
				x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
				if width == 2 {
					b.i32Const(0xFFFF)
					b.op(wasmOpI32And)
				}
			}
		}
		b.localSet(locTmp3)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffFlagsPtr)
		b.i32Load(2, 0)
		b.i32Const(int32(x86FlagCF))
		b.op(wasmOpI32And)
		b.localSet(locTmp6)
		if ji.grpOp <= 3 {
			x86WasmEmitRotateValue(b, ji.grpOp, locTmp3, locTmp4, locTmp, locTmp6, locTmp7, width)
		} else {
			x86WasmEmitShiftValue(b, ji.grpOp, locTmp3, locTmp4, locTmp, locTmp6, width)
		}
		if op == 0xD0 || op == 0xD1 {
			if ji.modrm>>6 == 3 {
				if width == 1 {
					b.localGet(locTmp)
					x86WasmEmitInsertReg8(b, locRegs, locTmp5, ji.modrm&7)
				} else if width == 2 {
					b.localGet(locTmp)
					x86WasmEmitInsertReg16(b, locRegs, locTmp5, ji.modrm&7)
				} else {
					b.localGet(locTmp)
					x86WasmEmitStoreReg32(b, locRegs, locTmp5, ji.modrm&7)
				}
			} else {
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffMemPtr)
				b.localGet(locTmp2)
				b.op(wasmOpI32Add)
				b.localGet(locTmp)
				if width == 1 {
					b.i32Store8(0, 0)
				} else if width == 2 {
					b.i32Store16(1, 0)
				} else {
					b.i32Store(2, 0)
				}
				x86WasmEmitSMCStoreCheck(b, locCtx, locTmp2, locTmp5, width, ji.opcodePC+uint32(ji.length), retired+1)
			}
		} else {
			if width == 1 {
				b.localGet(locTmp)
				x86WasmEmitInsertReg8(b, locRegs, locTmp5, ji.modrm&7)
			} else if width == 2 {
				b.localGet(locTmp)
				x86WasmEmitInsertReg16(b, locRegs, locTmp5, ji.modrm&7)
			} else {
				b.localGet(locTmp)
				x86WasmEmitStoreReg32(b, locRegs, locTmp5, ji.modrm&7)
			}
		}
		if ji.grpOp <= 3 {
			x86WasmEmitRotateFlagsWidth(b, ji.grpOp, locCtx, locTmp4, locTmp, locTmp6, locTmp5, locTmp2, width)
		} else {
			x86WasmEmitShiftFlagsWidth(b, ji.grpOp, locCtx, locTmp3, locTmp4, locTmp, locTmp6, locTmp5, locTmp2, locTmp7, width)
		}
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
	case op == 0x00 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitExtractReg8(b, locRegs, dst)
		b.localSet(locTmp3)
		x86WasmEmitExtractReg8(b, locRegs, src)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Add)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitInsertReg8(b, locRegs, locTmp2, dst)
		x86WasmEmitArithFlags8(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, locTmp7, false)
		return true
	case op == 0x08 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitExtractReg8(b, locRegs, dst)
		x86WasmEmitExtractReg8(b, locRegs, src)
		b.op(wasmOpI32Or)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitInsertReg8(b, locRegs, locTmp2, dst)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 8)
		return true
	case op == 0x20 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitExtractReg8(b, locRegs, dst)
		x86WasmEmitExtractReg8(b, locRegs, src)
		b.op(wasmOpI32And)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitInsertReg8(b, locRegs, locTmp2, dst)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 8)
		return true
	case op == 0x28 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitExtractReg8(b, locRegs, dst)
		b.localSet(locTmp3)
		x86WasmEmitExtractReg8(b, locRegs, src)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitInsertReg8(b, locRegs, locTmp2, dst)
		x86WasmEmitArithFlags8(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, locTmp7, true)
		return true
	case op == 0x30 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitExtractReg8(b, locRegs, dst)
		x86WasmEmitExtractReg8(b, locRegs, src)
		b.op(wasmOpI32Xor)
		b.localSet(locTmp)
		b.localGet(locTmp)
		x86WasmEmitInsertReg8(b, locRegs, locTmp2, dst)
		x86WasmEmitLogicFlags(b, locCtx, locTmp, locTmp2, locTmp3, locTmp4, 8)
		return true
	case op == 0x38 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitExtractReg8(b, locRegs, dst)
		b.localSet(locTmp3)
		x86WasmEmitExtractReg8(b, locRegs, src)
		b.localSet(locTmp4)
		b.localGet(locTmp3)
		b.localGet(locTmp4)
		b.op(wasmOpI32Sub)
		b.localSet(locTmp)
		x86WasmEmitArithFlags8(b, locCtx, locTmp, locTmp3, locTmp4, locTmp2, locTmp5, locTmp6, locTmp7, true)
		return true
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
	case op == 0x88 && ji.hasModRM:
		src := (ji.modrm >> 3) & 7
		if ji.modrm>>6 == 3 {
			x86WasmEmitExtractReg8(b, locRegs, src)
			x86WasmEmitInsertReg8(b, locRegs, locTmp, ji.modrm&7)
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
		x86WasmEmitExtractReg8(b, locRegs, src)
		b.i32Store8(0, 0)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp2, 1, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0x8A && ji.hasModRM:
		dst := (ji.modrm >> 3) & 7
		if ji.modrm>>6 == 3 {
			x86WasmEmitExtractReg8(b, locRegs, ji.modrm&7)
			x86WasmEmitInsertReg8(b, locRegs, locTmp, dst)
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
		x86WasmEmitInsertReg8(b, locRegs, locTmp2, dst)
		return true
	case op == 0x8D && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0:
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp2, (ji.modrm>>3)&7)
		return true
	case op == 0xA0 && ji.prefixes == 0:
		b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 1, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Load8U(0, 0)
		x86WasmEmitInsertReg8(b, locRegs, locTmp2, 0)
		return true
	case op == 0xA2 && ji.prefixes == 0:
		b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 1, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		x86WasmEmitExtractReg8(b, locRegs, 0)
		b.i32Store8(0, 0)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp2, 1, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0xA1 && ji.prefixes&^x86PrefOpSize == 0:
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Load16U(1, 0)
			x86WasmEmitInsertReg16(b, locRegs, locTmp2, 0)
		} else {
			b.i32Load(2, 0)
			x86WasmEmitStoreReg32(b, locRegs, locTmp2, 0)
		}
		return true
	case op == 0xA3 && ji.prefixes&^x86PrefOpSize == 0:
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		x86WasmEmitLoadReg32(b, locRegs, 0)
		if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp2, width, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0x8F && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes&^x86PrefOpSize == 0 && ji.grpOp == 0:
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		b.localSet(locTmp2)
		b.localGet(locTmp)
		b.i32Const(int32(width))
		b.op(wasmOpI32Add)
		x86WasmEmitStoreReg32(b, locRegs, locTmp3, 4)
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp3, locTmp4, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.localGet(locTmp2)
		if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp3, width, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0xE8 && ji.prefixes&^x86PrefOpSize == 0:
		width := uint32(4)
		returnPC := ji.opcodePC + uint32(ji.length)
		target := uint32(0)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
			target = uint32(int32(returnPC) + int32(int16(x86WasmImmediate16(ji, memory))))
		} else {
			target = uint32(int32(returnPC) + int32(x86WasmImmediate32(ji, memory)))
		}
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.i32Const(int32(width))
		b.op(wasmOpI32Sub)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localSet(locTmp2)
		b.localGet(locTmp2)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Const(int32(returnPC))
		if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		b.localGet(locTmp)
		x86WasmEmitStoreReg32(b, locRegs, locTmp3, 4)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp2, width, target, retired+1)
		x86WasmEmitRetPCAndCount(b, target, retired+1, cycles, ticks)
		return true
	case op == 0xC3 && ji.prefixes == 0:
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 4, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Load(2, 0)
		b.localSet(locTmp2)
		b.localGet(locTmp)
		b.i32Const(4)
		b.op(wasmOpI32Add)
		x86WasmEmitStoreReg32(b, locRegs, locTmp3, 4)
		x86WasmEmitDynamicRetPCAndCount(b, locCtx, locTmp2, retired+1, cycles, ticks)
		return true
	case op == 0xC2 && ji.prefixes == 0:
		x86WasmEmitLoadReg32(b, locRegs, 4)
		b.localSet(locTmp)
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 4, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Load(2, 0)
		b.localSet(locTmp2)
		b.localGet(locTmp)
		b.i32Const(int32(4 + uint32(x86WasmImmediate16(ji, memory))))
		b.op(wasmOpI32Add)
		x86WasmEmitStoreReg32(b, locRegs, locTmp3, 4)
		x86WasmEmitDynamicRetPCAndCount(b, locCtx, locTmp2, retired+1, cycles, ticks)
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
	case op == 0xF6 && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes == 0 && (ji.grpOp == 0 || ji.grpOp == 1):
		x86WasmEmitExtractReg8(b, locRegs, ji.modrm&7)
		b.i32Const(int32(uint32(x86WasmImmediate8(ji, memory))))
		b.op(wasmOpI32And)
		b.localSet(locTmp4)
		x86WasmEmitLogicFlags(b, locCtx, locTmp4, locTmp2, locTmp5, locTmp6, 8)
		return true
	case op == 0xF7 && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes&^x86PrefOpSize == 0 && (ji.grpOp == 0 || ji.grpOp == 1):
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Load16U(1, 0)
			b.i32Const(int32(uint32(x86WasmImmediate16(ji, memory))))
		} else {
			b.i32Load(2, 0)
			b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		}
		b.op(wasmOpI32And)
		b.localSet(locTmp4)
		x86WasmEmitLogicFlags(b, locCtx, locTmp4, locTmp2, locTmp5, locTmp6, width*8)
		return true
	case op == 0xF7 && ji.hasModRM && ji.modrm>>6 == 3 && ji.prefixes&^x86PrefOpSize == 0 && (ji.grpOp == 0 || ji.grpOp == 1):
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
		if width == 2 {
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
			b.i32Const(int32(uint32(x86WasmImmediate16(ji, memory))))
		} else {
			b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		}
		b.op(wasmOpI32And)
		b.localSet(locTmp4)
		x86WasmEmitLogicFlags(b, locCtx, locTmp4, locTmp2, locTmp5, locTmp6, width*8)
		return true
	case (op == 0xF6 && ji.hasModRM && ji.prefixes == 0 && (ji.grpOp == 4 || ji.grpOp == 5)) ||
		(op == 0xF7 && ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0 && (ji.grpOp == 4 || ji.grpOp == 5)):
		width := uint32(4)
		if op == 0xF6 {
			width = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if ji.modrm>>6 == 3 {
			if width == 1 {
				x86WasmEmitExtractReg8(b, locRegs, ji.modrm&7)
			} else {
				x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
				if width == 2 {
					b.i32Const(0xFFFF)
					b.op(wasmOpI32And)
				}
			}
		} else {
			if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
				return false
			}
			x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffMemPtr)
			b.localGet(locTmp)
			b.op(wasmOpI32Add)
			if width == 1 {
				b.i32Load8U(0, 0)
			} else if width == 2 {
				b.i32Load16U(1, 0)
			} else {
				b.i32Load(2, 0)
			}
		}
		b.localSet(locTmp3) // operand
		if ji.grpOp == 4 {  // MUL
			switch width {
			case 1:
				x86WasmEmitExtractReg8(b, locRegs, 0)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.localGet(locTmp3)
				b.op(wasmOpI32Mul)
				b.localSet(locTmp4)
				b.localGet(locTmp4)
				x86WasmEmitInsertReg16(b, locRegs, locTmp5, 0)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffFlagsPtr)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.i32Load(2, 0)
				b.i32Const(^int32(x86FlagCF | x86FlagOF))
				b.op(wasmOpI32And)
				b.localSet(locTmp5)
				b.localGet(locTmp4)
				b.i32Const(8)
				b.op(wasmOpI32ShrU)
				b.op(wasmOpI32Eqz)
				b.ifVoid()
				b.elseBranch()
				b.localGet(locTmp5)
				b.i32Const(int32(x86FlagCF | x86FlagOF))
				b.op(wasmOpI32Or)
				b.localSet(locTmp5)
				b.end()
				b.localGet(locTmp2)
				b.localGet(locTmp5)
				b.i32Store(2, 0)
			case 2:
				x86WasmEmitLoadReg32(b, locRegs, 0)
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.localGet(locTmp3)
				b.op(wasmOpI32Mul)
				b.localSet(locTmp4)
				b.localGet(locTmp4)
				x86WasmEmitInsertReg16(b, locRegs, locTmp5, 0)
				b.localGet(locTmp4)
				b.i32Const(16)
				b.op(wasmOpI32ShrU)
				x86WasmEmitInsertReg16(b, locRegs, locTmp5, 2)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffFlagsPtr)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.i32Load(2, 0)
				b.i32Const(^int32(x86FlagCF | x86FlagOF))
				b.op(wasmOpI32And)
				b.localSet(locTmp5)
				b.localGet(locTmp4)
				b.i32Const(16)
				b.op(wasmOpI32ShrU)
				b.op(wasmOpI32Eqz)
				b.ifVoid()
				b.elseBranch()
				b.localGet(locTmp5)
				b.i32Const(int32(x86FlagCF | x86FlagOF))
				b.op(wasmOpI32Or)
				b.localSet(locTmp5)
				b.end()
				b.localGet(locTmp2)
				b.localGet(locTmp5)
				b.i32Store(2, 0)
			default:
				x86WasmEmitLoadReg32(b, locRegs, 0)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.op(wasmOpI64ExtendI32U)
				b.localGet(locTmp3)
				b.op(wasmOpI64ExtendI32U)
				b.op(wasmOpI64Mul)
				b.op(wasmOpI32WrapI64)
				x86WasmEmitStoreReg32(b, locRegs, locTmp5, 0)
				b.localGet(locTmp2)
				b.op(wasmOpI64ExtendI32U)
				b.localGet(locTmp3)
				b.op(wasmOpI64ExtendI32U)
				b.op(wasmOpI64Mul)
				b.i64Const(32)
				b.op(wasmOpI64ShrU)
				b.op(wasmOpI32WrapI64)
				x86WasmEmitStoreReg32(b, locRegs, locTmp5, 2)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffFlagsPtr)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.i32Load(2, 0)
				b.i32Const(^int32(x86FlagCF | x86FlagOF))
				b.op(wasmOpI32And)
				b.localSet(locTmp5)
				x86WasmEmitLoadReg32(b, locRegs, 2)
				b.op(wasmOpI32Eqz)
				b.ifVoid()
				b.elseBranch()
				b.localGet(locTmp5)
				b.i32Const(int32(x86FlagCF | x86FlagOF))
				b.op(wasmOpI32Or)
				b.localSet(locTmp5)
				b.end()
				b.localGet(locTmp2)
				b.localGet(locTmp5)
				b.i32Store(2, 0)
			}
		} else { // IMUL
			switch width {
			case 1:
				x86WasmEmitExtractReg8(b, locRegs, 0)
				b.i32Const(24)
				b.op(wasmOpI32Shl)
				b.i32Const(24)
				b.op(wasmOpI32ShrS)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.localGet(locTmp3)
				b.i32Const(24)
				b.op(wasmOpI32Shl)
				b.i32Const(24)
				b.op(wasmOpI32ShrS)
				b.op(wasmOpI32Mul)
				b.localSet(locTmp4)
				b.localGet(locTmp4)
				x86WasmEmitInsertReg16(b, locRegs, locTmp5, 0)
				b.localGet(locCtx)
				b.i32Load(2, x86CtxOffFlagsPtr)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.i32Load(2, 0)
				b.i32Const(^int32(x86FlagCF | x86FlagOF))
				b.op(wasmOpI32And)
				b.localSet(locTmp5)
				b.localGet(locTmp4)
				b.i32Const(8)
				b.op(wasmOpI32Shl)
				b.i32Const(8)
				b.op(wasmOpI32ShrS)
				b.localGet(locTmp4)
				b.op(wasmOpI32Eq)
				b.ifVoid()
				b.elseBranch()
				b.localGet(locTmp5)
				b.i32Const(int32(x86FlagCF | x86FlagOF))
				b.op(wasmOpI32Or)
				b.localSet(locTmp5)
				b.end()
				b.localGet(locTmp2)
				b.localGet(locTmp5)
				b.i32Store(2, 0)
			case 2:
				x86WasmEmitLoadReg32(b, locRegs, 0)
				b.i32Const(16)
				b.op(wasmOpI32Shl)
				b.i32Const(16)
				b.op(wasmOpI32ShrS)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.localGet(locTmp3)
				b.i32Const(16)
				b.op(wasmOpI32Shl)
				b.i32Const(16)
				b.op(wasmOpI32ShrS)
				b.op(wasmOpI32Mul)
				b.localSet(locTmp4)
				b.localGet(locTmp4)
				x86WasmEmitInsertReg16(b, locRegs, locTmp5, 0)
				b.localGet(locTmp4)
				b.i32Const(16)
				b.op(wasmOpI32ShrU)
				x86WasmEmitInsertReg16(b, locRegs, locTmp5, 2)
				x86WasmEmitMulOverflowFlags(b, locCtx, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, 2)
			default:
				x86WasmEmitLoadReg32(b, locRegs, 0)
				b.localSet(locTmp2)
				b.localGet(locTmp2)
				b.op(wasmOpI64ExtendI32S)
				b.localGet(locTmp3)
				b.op(wasmOpI64ExtendI32S)
				b.op(wasmOpI64Mul)
				b.op(wasmOpI32WrapI64)
				x86WasmEmitStoreReg32(b, locRegs, locTmp5, 0)
				b.localGet(locTmp2)
				b.op(wasmOpI64ExtendI32S)
				b.localGet(locTmp3)
				b.op(wasmOpI64ExtendI32S)
				b.op(wasmOpI64Mul)
				b.i64Const(32)
				b.op(wasmOpI64ShrU)
				b.op(wasmOpI32WrapI64)
				x86WasmEmitStoreReg32(b, locRegs, locTmp5, 2)
				x86WasmEmitLoadReg32(b, locRegs, 0)
				b.localSet(locTmp4)
				x86WasmEmitMulOverflowFlags(b, locCtx, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, 4)
			}
		}
		return true
	case (op == 0xF6 && ji.hasModRM && ji.prefixes == 0 && (ji.grpOp == 6 || ji.grpOp == 7)) ||
		(op == 0xF7 && ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0 && (ji.grpOp == 6 || ji.grpOp == 7)):
		x86WasmEmitExitReasonReturn(b, locCtx, ji.opcodePC, retired, x86JITExitInterpFallback)
		return true
	case (op == 0xF6 && ji.hasModRM && ji.prefixes == 0 && (ji.grpOp == 2 || ji.grpOp == 3)) ||
		(op == 0xF7 && ji.hasModRM && ji.prefixes&^x86PrefOpSize == 0 && (ji.grpOp == 2 || ji.grpOp == 3)):
		width := uint32(4)
		if op == 0xF6 {
			width = 1
		} else if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if ji.modrm>>6 == 3 {
			if width == 1 {
				x86WasmEmitExtractReg8(b, locRegs, ji.modrm&7)
			} else {
				x86WasmEmitLoadReg32(b, locRegs, ji.modrm&7)
				if width == 2 {
					b.i32Const(0xFFFF)
					b.op(wasmOpI32And)
				}
			}
		} else {
			if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
				return false
			}
			x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffMemPtr)
			b.localGet(locTmp)
			b.op(wasmOpI32Add)
			if width == 1 {
				b.i32Load8U(0, 0)
			} else if width == 2 {
				b.i32Load16U(1, 0)
			} else {
				b.i32Load(2, 0)
			}
		}
		b.localSet(locTmp3)
		if ji.grpOp == 2 { // NOT
			b.localGet(locTmp3)
			b.i32Const(-1)
			b.op(wasmOpI32Xor)
			if width == 1 {
				b.i32Const(0xFF)
				b.op(wasmOpI32And)
			} else if width == 2 {
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
			b.localSet(locTmp4)
		} else { // NEG
			b.i32Const(0)
			b.localGet(locTmp3)
			b.op(wasmOpI32Sub)
			if width == 1 {
				b.i32Const(0xFF)
				b.op(wasmOpI32And)
			} else if width == 2 {
				b.i32Const(0xFFFF)
				b.op(wasmOpI32And)
			}
			b.localSet(locTmp4)
		}
		if ji.modrm>>6 == 3 {
			if width == 1 {
				b.localGet(locTmp4)
				x86WasmEmitInsertReg8(b, locRegs, locTmp5, ji.modrm&7)
			} else if width == 2 {
				b.localGet(locTmp4)
				x86WasmEmitInsertReg16(b, locRegs, locTmp5, ji.modrm&7)
			} else {
				b.localGet(locTmp4)
				x86WasmEmitStoreReg32(b, locRegs, locTmp5, ji.modrm&7)
			}
		} else {
			b.localGet(locCtx)
			b.i32Load(2, x86CtxOffMemPtr)
			b.localGet(locTmp)
			b.op(wasmOpI32Add)
			b.localGet(locTmp4)
			if width == 1 {
				b.i32Store8(0, 0)
			} else if width == 2 {
				b.i32Store16(1, 0)
			} else {
				b.i32Store(2, 0)
			}
			x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp5, width, ji.opcodePC+uint32(ji.length), retired+1)
		}
		if ji.grpOp == 3 {
			x86WasmEmitNEGFlagsWidth(b, locCtx, locTmp3, locTmp4, locTmp5, locTmp6, locTmp2, width)
		}
		return true
	case op == 0xC6 && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0 && ji.grpOp == 0:
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, 1, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.i32Const(int32(x86WasmImmediate8(ji, memory)))
		b.i32Store8(0, 0)
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp2, 1, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0xC7 && ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes&^x86PrefOpSize == 0 && ji.grpOp == 0:
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Const(int32(uint32(x86WasmImmediate16(ji, memory))))
			b.i32Store16(1, 0)
		} else {
			b.i32Const(int32(x86WasmImmediate32(ji, memory)))
			b.i32Store(2, 0)
		}
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp2, width, ji.opcodePC+uint32(ji.length), retired+1)
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
	case op == 0x87 && ji.hasModRM: // XCHG r/m32/16, r32/16
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		width := uint32(4)
		if ji.prefixes&x86PrefOpSize != 0 {
			width = 2
		}
		if ji.modrm>>6 == 3 {
			x86WasmEmitLoadReg32(b, locRegs, dst)
			b.localSet(locTmp2)
			x86WasmEmitLoadReg32(b, locRegs, src)
			if width == 2 {
				x86WasmEmitInsertReg16(b, locRegs, locTmp, dst)
				b.localGet(locTmp2)
				x86WasmEmitInsertReg16(b, locRegs, locTmp, src)
			} else {
				x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
				b.localGet(locTmp2)
				x86WasmEmitStoreReg32(b, locRegs, locTmp, src)
			}
			return true
		}
		if !x86WasmEmitEA32(b, ji, memory, locRegs, locTmp) {
			return false
		}
		x86WasmEmitSpanGuard(b, locCtx, locTmp, locTmp2, locTmp3, width, ji.opcodePC, retired)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		if width == 2 {
			b.i32Load16U(1, 0)
		} else {
			b.i32Load(2, 0)
		}
		b.localSet(locTmp2) // old memory
		x86WasmEmitLoadReg32(b, locRegs, src)
		if width == 2 {
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
		}
		b.localSet(locTmp3) // reg payload for memory
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffMemPtr)
		b.localGet(locTmp)
		b.op(wasmOpI32Add)
		b.localGet(locTmp3)
		if width == 2 {
			b.i32Store16(1, 0)
		} else {
			b.i32Store(2, 0)
		}
		b.localGet(locTmp2)
		if width == 2 {
			x86WasmEmitInsertReg16(b, locRegs, locTmp4, src)
		} else {
			x86WasmEmitStoreReg32(b, locRegs, locTmp4, src)
		}
		x86WasmEmitSMCStoreCheck(b, locCtx, locTmp, locTmp3, width, ji.opcodePC+uint32(ji.length), retired+1)
		return true
	case op == 0xE9 || op == 0xEB:
		return true
	default:
		return false
	}
}

func x86WasmEmitChainCredit(b *wasmBody, locCtx uint32, cycles, ticks uint32) {
	if cycles != 0 {
		b.localGet(locCtx)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffChainCycles)
		b.i32Const(int32(cycles))
		b.op(wasmOpI32Add)
		b.i32Store(2, x86CtxOffChainCycles)
	}
	if ticks != 0 {
		b.localGet(locCtx)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffChainTicks)
		b.i32Const(int32(ticks))
		b.op(wasmOpI32Add)
		b.i32Store(2, x86CtxOffChainTicks)
	}
}

func x86WasmEmitDynamicChainCredit(b *wasmBody, locCtx, locCycles, locTicks uint32) {
	b.localGet(locCtx)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffChainCycles)
	b.localGet(locCycles)
	b.op(wasmOpI32Add)
	b.i32Store(2, x86CtxOffChainCycles)

	b.localGet(locCtx)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffChainTicks)
	b.localGet(locTicks)
	b.op(wasmOpI32Add)
	b.i32Store(2, x86CtxOffChainTicks)
}

func x86WasmEmitRetPCAndCount(b *wasmBody, retPC uint32, instrCount int, cycles, ticks uint32) {
	x86WasmEmitChainCredit(b, 0, cycles, ticks)
	b.localGet(0)
	b.i32Const(int32(retPC))
	b.i32Store(2, x86CtxOffRetPC)
	b.localGet(0)
	b.i32Const(int32(instrCount))
	b.i32Store(2, x86CtxOffRetCount)
}

func x86WasmEmitDynamicRetPCAndCount(b *wasmBody, locCtx, locRetPC uint32, instrCount int, cycles, ticks uint32) {
	x86WasmEmitChainCredit(b, locCtx, cycles, ticks)
	b.localGet(locCtx)
	b.localGet(locRetPC)
	b.i32Store(2, x86CtxOffRetPC)
	b.localGet(locCtx)
	b.i32Const(int32(instrCount))
	b.i32Store(2, x86CtxOffRetCount)
}

func x86WasmEmitDynamicRetPCAndLocalCount(b *wasmBody, locCtx, locRetPC, locRetCount, locCycles, locTicks uint32) {
	x86WasmEmitDynamicChainCredit(b, locCtx, locCycles, locTicks)
	b.localGet(locCtx)
	b.localGet(locRetPC)
	b.i32Store(2, x86CtxOffRetPC)
	b.localGet(locCtx)
	b.localGet(locRetCount)
	b.i32Store(2, x86CtxOffRetCount)
}

func x86WasmCompileBlockModule(instrs []X86JITInstr, startPC uint32, memory []byte) (*x86WasmCompiledModule, error) {
	compiledInstrs, retPC, spanEnd, termKind, ok := x86WasmSelectCompiledPrefix(instrs, memory, startPC)
	if !ok {
		return nil, fmt.Errorf("x86 wasm: unsupported block at %#x", startPC)
	}
	cyclePrefix := x86JITCyclePrefix(compiledInstrs)
	tickPrefix := x86JITTickPrefix(compiledInstrs)
	var totalCycles, totalTicks uint32
	if n := len(cyclePrefix); n != 0 {
		totalCycles = uint32(cyclePrefix[n-1])
		totalTicks = uint32(tickPrefix[n-1])
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
		locTmp8 = 9
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffJITRegsPtr)
	b.localSet(locRegs)
	emittedTerminalState := false
	for i, ji := range compiledInstrs {
		if i == len(compiledInstrs)-1 && termKind == x86WasmTerminalJcc {
			if !x86WasmEmitTerminalJcc(b, ji, memory, locCtx, locTmp2, locTmp3, len(compiledInstrs), totalCycles, totalTicks) {
				return nil, fmt.Errorf("x86 wasm: unsupported short Jcc %#02x at %#x", byte(ji.opcode), ji.opcodePC)
			}
			emittedTerminalState = true
			break
		}
		if i == len(compiledInstrs)-1 && termKind == x86WasmTerminalLoop {
			if !x86WasmEmitTerminalLoop(b, ji, memory, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, len(compiledInstrs), totalCycles, totalTicks) {
				return nil, fmt.Errorf("x86 wasm: unsupported loop %#02x at %#x", byte(ji.opcode), ji.opcodePC)
			}
			emittedTerminalState = true
			break
		}
		if !x86WasmEmitInstr(b, ji, memory, i, totalCycles, totalTicks, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, locTmp8) {
			return nil, fmt.Errorf("x86 wasm: unsupported opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
		}
		if i == len(compiledInstrs)-1 && (termKind == x86WasmTerminalCALL || termKind == x86WasmTerminalRET) {
			emittedTerminalState = true
		}
	}
	if !emittedTerminalState {
		x86WasmEmitRetPCAndCount(b, retPC, len(compiledInstrs), totalCycles, totalTicks)
	}
	b.end()
	fn := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI64}, b.code)
	m.exportFunc("block", fn)
	block := &JITBlock{
		startPC:          uint64(startPC),
		endPC:            uint64(spanEnd),
		instrCount:       len(compiledInstrs),
		x86CyclePrefix:   cyclePrefix,
		x86TickPrefix:    tickPrefix,
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
	x86WasmInstrMayBail := func(ji X86JITInstr) bool {
		if ji.opcode >= 0x0F00 {
			op2 := byte(ji.opcode)
			switch op2 {
			case 0xB6, 0xB7, 0xBE, 0xBF:
				return ji.hasModRM && ji.modrm>>6 != 3
			}
			return false
		}
		op := byte(ji.opcode)
		switch op {
		case 0xD8, 0xD9, 0xDA, 0xDB, 0xDC, 0xDD, 0xDE, 0xDF:
			return true
		case 0x89, 0x8B:
			return ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0
		case 0xF6, 0xF7:
			return ji.hasModRM && ji.modrm>>6 != 3 && ji.prefixes == 0 && (ji.grpOp == 0 || ji.grpOp == 1)
		case 0xE8, 0xC3, 0xC2:
			return true
		}
		return false
	}
	all := make([]X86JITInstr, 0, 16)
	covered := make([][2]uint64, 0, len(region.blocks))
	var retPC uint32
	var maxEnd uint64
	backEdgeSource := -1
	backEdgeTarget := -1
	if len(region.backEdges) != 0 {
		if len(region.backEdges) != 1 {
			return nil, fmt.Errorf("x86 wasm: multiple loop back-edges unsupported")
		}
		for src, dst := range region.backEdges {
			backEdgeSource, backEdgeTarget = src, dst
		}
		if backEdgeSource != len(region.blocks)-1 || backEdgeTarget < 0 || backEdgeTarget >= backEdgeSource {
			return nil, fmt.Errorf("x86 wasm: unsupported back-edge shape")
		}
	}
	for i, block := range region.blocks {
		blockPC := region.blockPCs[i]
		nextPC, ok := x86WasmBlockTerminalPC(block, memory, blockPC)
		if !ok {
			return nil, fmt.Errorf("x86 wasm: unsupported region block %#x", blockPC)
		}
		if i != len(region.blocks)-1 && nextPC != region.blockPCs[i+1] {
			return nil, fmt.Errorf("x86 wasm: non-linear successor %#x -> %#x", blockPC, nextPC)
		}
		if i == len(region.blocks)-1 && backEdgeSource >= 0 && nextPC != region.blockPCs[backEdgeTarget] {
			return nil, fmt.Errorf("x86 wasm: final loop edge %#x -> %#x does not target loop head", blockPC, nextPC)
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
	if backEdgeSource >= 0 {
		if len(x86JITDynamicCycles(all)) != 0 {
			return nil, fmt.Errorf("x86 wasm: loop region with dynamic-cycle forms unsupported")
		}
		for _, ji := range all {
			if x86WasmInstrMayBail(ji) {
				return nil, fmt.Errorf("x86 wasm: loop region with bailing instruction unsupported")
			}
		}
	}
	cyclePrefix := x86JITCyclePrefix(all)
	tickPrefix := x86JITTickPrefix(all)
	var totalCycles, totalTicks uint32
	if n := len(cyclePrefix); n != 0 {
		totalCycles = uint32(cyclePrefix[n-1])
		totalTicks = uint32(tickPrefix[n-1])
	}
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)
	const (
		locCtx        = 0
		locRegs       = 1
		locTmp        = 2
		locTmp2       = 3
		locTmp3       = 4
		locTmp4       = 5
		locTmp5       = 6
		locTmp6       = 7
		locTmp7       = 8
		locTmp8       = 9
		locLoopRet    = 10
		locLoopCycle  = 11
		locLoopTick   = 12
		locLoopBudget = 13
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffJITRegsPtr)
	b.localSet(locRegs)
	if backEdgeSource < 0 {
		for i, ji := range all {
			if !x86WasmEmitInstr(b, ji, memory, i, totalCycles, totalTicks, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, locTmp8) {
				return nil, fmt.Errorf("x86 wasm: unsupported region opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
			}
		}
		x86WasmEmitRetPCAndCount(b, retPC, len(all), totalCycles, totalTicks)
	} else {
		loopHeadPC := region.blockPCs[backEdgeTarget]
		prefixInstrs := 0
		for i := 0; i < backEdgeTarget; i++ {
			prefixInstrs += len(region.blocks[i])
		}
		loopInstrs := len(all) - prefixInstrs
		var prefixCycles, prefixTicks uint32
		if prefixInstrs > 0 {
			prefixCycles = uint32(cyclePrefix[prefixInstrs-1])
			prefixTicks = uint32(tickPrefix[prefixInstrs-1])
		}
		loopCycles := totalCycles - prefixCycles
		loopTicks := totalTicks - prefixTicks

		b.i32Const(int32(prefixInstrs))
		b.localSet(locLoopRet)
		b.i32Const(int32(prefixCycles))
		b.localSet(locLoopCycle)
		b.i32Const(int32(prefixTicks))
		b.localSet(locLoopTick)

		for i := 0; i < prefixInstrs; i++ {
			ji := all[i]
			if !x86WasmEmitInstr(b, ji, memory, i, 0, 0, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, locTmp8) {
				return nil, fmt.Errorf("x86 wasm: unsupported loop-prefix opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
			}
		}

		b.loop()
		for i := prefixInstrs; i < len(all); i++ {
			ji := all[i]
			if !x86WasmEmitInstr(b, ji, memory, i, 0, 0, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, locTmp8) {
				return nil, fmt.Errorf("x86 wasm: unsupported loop-body opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
			}
		}

		b.localGet(locLoopRet)
		b.i32Const(int32(loopInstrs))
		b.op(wasmOpI32Add)
		b.localSet(locLoopRet)
		b.localGet(locLoopCycle)
		b.i32Const(int32(loopCycles))
		b.op(wasmOpI32Add)
		b.localSet(locLoopCycle)
		b.localGet(locLoopTick)
		b.i32Const(int32(loopTicks))
		b.op(wasmOpI32Add)
		b.localSet(locLoopTick)

		b.localGet(locCtx)
		b.localGet(locCtx)
		b.i32Load(2, x86CtxOffChainBudget)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.localTee(locLoopBudget)
		b.i32Store(2, x86CtxOffChainBudget)
		b.localGet(locLoopBudget)
		b.op(wasmOpI32Eqz)
		b.ifVoid()
		b.i32Const(int32(loopHeadPC))
		b.localSet(locTmp)
		x86WasmEmitDynamicRetPCAndLocalCount(b, locCtx, locTmp, locLoopRet, locLoopCycle, locLoopTick)
		b.op(wasmOpReturn)
		b.elseBranch()
		b.br(1)
		b.end()
		b.end()
	}
	b.end()
	fn := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32}, b.code)
	m.exportFunc("block", fn)
	block := &JITBlock{
		startPC:          uint64(region.entryPC),
		endPC:            maxEnd,
		instrCount:       len(all),
		x86CyclePrefix:   cyclePrefix,
		x86TickPrefix:    tickPrefix,
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

func x86WasmFormConditionalRegion(entryPC uint32, memory []byte) *x86WasmConditionalRegion {
	if entryPC >= uint32(len(memory)) {
		return nil
	}
	scannedEntry := x86ScanBlock(memory, entryPC)
	if len(scannedEntry) == 0 || x86NeedsFallback(scannedEntry) {
		return nil
	}
	entry, _, _, termKind, ok := x86WasmSelectCompiledPrefix(scannedEntry, memory, entryPC)
	if !ok || len(entry) == 0 || termKind != x86WasmTerminalJcc {
		return nil
	}
	for _, ji := range entry {
		if x86ShouldStepInInterpreter(ji) {
			return nil
		}
	}
	last := entry[len(entry)-1]
	if !x86WasmIsNearJcc(last) && !x86WasmIsShortJcc(byte(last.opcode)) {
		return nil
	}
	_, targetPC, fallthroughPC, ok := x86WasmTerminalJccTarget(last, memory)
	if !ok || targetPC <= fallthroughPC {
		return nil
	}

	fallBlock := x86ScanBlock(memory, fallthroughPC)
	target := x86ScanBlock(memory, targetPC)
	if len(fallBlock) == 0 || len(target) == 0 || x86NeedsFallback(fallBlock) || x86NeedsFallback(target) {
		return nil
	}
	for _, block := range [][]X86JITInstr{fallBlock, target} {
		for _, ji := range block {
			if x86ShouldStepInInterpreter(ji) {
				return nil
			}
		}
	}
	fLast := fallBlock[len(fallBlock)-1]
	tLast := target[len(target)-1]
	if byte(fLast.opcode) != 0xE9 && byte(fLast.opcode) != 0xEB {
		return nil
	}
	if byte(tLast.opcode) != 0xE9 && byte(tLast.opcode) != 0xEB {
		return nil
	}
	fExit, ok := x86ResolveTerminatorTarget(&fLast, memory, fallthroughPC)
	if !ok {
		return nil
	}
	tExit, ok := x86ResolveTerminatorTarget(&tLast, memory, targetPC)
	if !ok || tExit != fExit {
		return nil
	}
	return &x86WasmConditionalRegion{
		entryPC:     entryPC,
		entryBlock:  entry,
		fallBlock:   fallBlock,
		targetBlock: target,
		fallPC:      fallthroughPC,
		targetPC:    targetPC,
		exitPC:      fExit,
	}
}

func x86WasmCompileConditionalRegionModule(region *x86WasmConditionalRegion, memory []byte) (*x86WasmCompiledModule, error) {
	if region == nil || len(region.entryBlock) == 0 || len(region.fallBlock) == 0 || len(region.targetBlock) == 0 {
		return nil, fmt.Errorf("x86 wasm: empty conditional region")
	}
	all := make([]X86JITInstr, 0, len(region.entryBlock)+len(region.fallBlock)+len(region.targetBlock))
	all = append(all, region.entryBlock...)
	all = append(all, region.fallBlock...)
	all = append(all, region.targetBlock...)
	cyclePrefix := x86JITCyclePrefix(all)
	tickPrefix := x86JITTickPrefix(all)
	covered := [][2]uint64{
		{uint64(region.entryPC), uint64(region.entryBlock[len(region.entryBlock)-1].opcodePC + uint32(region.entryBlock[len(region.entryBlock)-1].length))},
		{uint64(region.fallPC), uint64(region.fallBlock[len(region.fallBlock)-1].opcodePC + uint32(region.fallBlock[len(region.fallBlock)-1].length))},
		{uint64(region.targetPC), uint64(region.targetBlock[len(region.targetBlock)-1].opcodePC + uint32(region.targetBlock[len(region.targetBlock)-1].length))},
	}
	entryCount := len(region.entryBlock)
	fallCount := len(region.fallBlock)
	targetCount := len(region.targetBlock)
	entryCycles := uint32(cyclePrefix[entryCount-1])
	entryTicks := uint32(tickPrefix[entryCount-1])
	fallCycles := uint32(cyclePrefix[entryCount+fallCount-1] - cyclePrefix[entryCount-1])
	fallTicks := uint32(tickPrefix[entryCount+fallCount-1] - tickPrefix[entryCount-1])
	targetCycles := uint32(cyclePrefix[len(all)-1] - cyclePrefix[entryCount+fallCount-1])
	targetTicks := uint32(tickPrefix[len(all)-1] - tickPrefix[entryCount+fallCount-1])

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
		locTmp8 = 9
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffJITRegsPtr)
	b.localSet(locRegs)

	for i := 0; i < entryCount-1; i++ {
		ji := region.entryBlock[i]
		if !x86WasmEmitInstr(b, ji, memory, i, 0, 0, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, locTmp8) {
			return nil, fmt.Errorf("x86 wasm: unsupported conditional-entry opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
		}
	}
	entryJcc := region.entryBlock[entryCount-1]
	condition, _, _, ok := x86WasmTerminalJccTarget(entryJcc, memory)
	if !ok || !x86WasmEmitJccCondition(b, uint32(condition), locCtx, locTmp2, locTmp3) {
		return nil, fmt.Errorf("x86 wasm: unsupported conditional-entry branch")
	}
	b.ifVoid()
	for i := 0; i < targetCount-1; i++ {
		ji := region.targetBlock[i]
		if !x86WasmEmitInstr(b, ji, memory, entryCount+fallCount+i, 0, 0, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, locTmp8) {
			return nil, fmt.Errorf("x86 wasm: unsupported conditional-target opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
		}
	}
	x86WasmEmitRetPCAndCount(b, region.exitPC, entryCount+targetCount, entryCycles+targetCycles, entryTicks+targetTicks)
	b.elseBranch()
	for i := 0; i < fallCount-1; i++ {
		ji := region.fallBlock[i]
		if !x86WasmEmitInstr(b, ji, memory, entryCount+i, 0, 0, locCtx, locRegs, locTmp, locTmp2, locTmp3, locTmp4, locTmp5, locTmp6, locTmp7, locTmp8) {
			return nil, fmt.Errorf("x86 wasm: unsupported conditional-fallthrough opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
		}
	}
	x86WasmEmitRetPCAndCount(b, region.exitPC, entryCount+fallCount, entryCycles+fallCycles, entryTicks+fallTicks)
	b.end()
	b.end()
	fn := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32}, b.code)
	m.exportFunc("block", fn)
	block := &JITBlock{
		startPC:        uint64(region.entryPC),
		endPC:          uint64(region.targetBlock[len(region.targetBlock)-1].opcodePC + uint32(region.targetBlock[len(region.targetBlock)-1].length)),
		instrCount:     len(all),
		x86CyclePrefix: cyclePrefix,
		x86TickPrefix:  tickPrefix,
		tier:           2,
		coveredRanges:  covered,
	}
	return &x86WasmCompiledModule{
		module: m.build(),
		block:  block,
		retPC:  region.exitPC,
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
