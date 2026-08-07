// jit_z80_wasm_emit.go - minimal, real WebAssembly lowering for Z80 blocks.
//
// This encoder is deliberately untagged: wazero executes the exact module
// bytes instantiated by the browser dispatcher.  Unsupported forms are never
// silently skipped; the dispatcher routes them through the frozen helper.
package main

import "fmt"

// The browser ABI is an explicit wasm32 image, not a native Go structure.
// The CPU pointer refers to Go linear memory.  The emitted subset only touches
// the leading byte registers, whose layout is fixed by CPU_Z80 and guarded by
// TestZ80WasmCPURegisterLayout.
const (
	z80WasmCtxOffCPUPtr      = 0
	z80WasmCtxOffRetPC       = 4
	z80WasmCtxOffRetCycles   = 8
	z80WasmCtxOffRetCount    = 16
	z80WasmCtxOffRIncrements = 20
	// These pointers publish the same guarded-memory ABI used by native
	// emitters. They are wasm32 offsets into Go's imported linear memory.
	z80WasmCtxOffMemPtr           = 24
	z80WasmCtxOffDirectPageBitmap = 28
	z80WasmCtxOffCodePageBitmap   = 32
	// NeedBail lets an emitted guarded direct-memory operation return before
	// observing a bank, VRAM or MMIO page. The dispatcher alone performs the
	// architectural fallback at the instruction boundary.
	z80WasmCtxOffNeedBail = 36
	// NeedInval is set only after a committed direct write to a page that
	// contains compiled source. InvalPage names that guest page.
	z80WasmCtxOffNeedInval = 40
	z80WasmCtxOffInvalPage = 44
	z80WasmCtxOffIFFDelay  = 48
	z80WasmCtxImageSize    = 52

	z80WasmCPUOffA    = 0
	z80WasmCPUOffF    = 1
	z80WasmCPUOffB    = 2
	z80WasmCPUOffC    = 3
	z80WasmCPUOffD    = 4
	z80WasmCPUOffE    = 5
	z80WasmCPUOffH    = 6
	z80WasmCPUOffL    = 7
	z80WasmCPUOffA2   = 8
	z80WasmCPUOffF2   = 9
	z80WasmCPUOffB2   = 10
	z80WasmCPUOffC2   = 11
	z80WasmCPUOffD2   = 12
	z80WasmCPUOffE2   = 13
	z80WasmCPUOffH2   = 14
	z80WasmCPUOffL2   = 15
	z80WasmCPUOffIX   = 16
	z80WasmCPUOffIY   = 18
	z80WasmCPUOffSP   = 20
	z80WasmCPUOffI    = 24
	z80WasmCPUOffR    = 25
	z80WasmCPUOffIM   = 26
	z80WasmCPUOffWZ   = 28
	z80WasmCPUOffIFF1 = 30
	z80WasmCPUOffIFF2 = 31
)

type z80WasmInstr struct {
	prefix       byte
	opcode       byte
	operand      byte
	operandHi    byte
	displacement int8
	indexedCB    bool
}

func z80WasmInstructionFor(prefix, opcode, operand byte) (length, cycles, rInc byte, err error) {
	return z80WasmInstructionMeta(z80WasmInstr{prefix: prefix, opcode: opcode, operand: operand})
}

func z80WasmInstructionMeta(instr z80WasmInstr) (length, cycles, rInc byte, err error) {
	prefix, opcode, operand := instr.prefix, instr.opcode, instr.operand
	if instr.indexedCB {
		if opcode>>6 == 1 {
			return 4, 20, 3, nil
		}
		return 4, 23, 3, nil
	}
	if prefix == z80JITPrefixCB && opcode&7 == 6 {
		if opcode>>6 == 1 {
			return 2, 12, 2, nil
		}
		return 2, 15, 2, nil
	}
	if prefix == z80JITPrefixCB && (z80CBRegisterOpcode(opcode) || z80CBRLCRegisterOpcode(opcode) || z80CBRRCRegisterOpcode(opcode) || z80CBRLRegisterOpcode(opcode) || z80CBRRRegisterOpcode(opcode) || z80CBSLARegisterOpcode(opcode) || z80CBSRLRegisterOpcode(opcode) || z80CBSRARegisterOpcode(opcode) || z80CBSLLRegisterOpcode(opcode)) {
		return 2, 8, 2, nil
	}
	if prefix == z80JITPrefixDD || prefix == z80JITPrefixFD {
		switch {
		case opcode == 0x21:
			return 4, 14, 2, nil
		case opcode == 0x22 || opcode == 0x2A:
			return 4, 20, 2, nil
		case opcode == 0x09 || opcode == 0x19 || opcode == 0x29 || opcode == 0x39:
			return 2, 15, 2, nil
		case opcode == 0x23 || opcode == 0x2B:
			return 2, 10, 2, nil
		case opcode == 0xF9:
			return 2, 10, 2, nil
		case opcode == 0xE5:
			return 2, 15, 2, nil
		case opcode == 0xE1:
			return 2, 14, 2, nil
		case opcode == 0xE9:
			return 2, 8, 2, nil
		case opcode == 0xE3:
			return 2, 23, 2, nil
		case opcode == 0x34 || opcode == 0x35:
			return 3, 23, 2, nil
		case opcode == 0x36:
			return 4, 19, 2, nil
		case opcode&0xC7 == 0x46 && opcode != 0x76,
			opcode >= 0x70 && opcode <= 0x77 && opcode != 0x76,
			opcode&0xC7 == 0x86:
			return 3, 19, 2, nil
		}
		if !z80WasmDDFDExplicitOpcode(opcode) {
			baseLength, baseCycles, baseR, baseErr := z80WasmInstruction(opcode, operand)
			if baseErr == nil {
				return baseLength + 1, baseCycles + 4, baseR + 1, nil
			}
		}
		if z80IgnoredIndexPrefixDirectOpcode(opcode) {
			return 2, 8, 2, nil
		}
		if z80IgnoredIndexPrefixLDImmOpcode(opcode) {
			return 3, 11, 2, nil
		}
		if z80IgnoredIndexPrefixLDRegOpcode(opcode) {
			return 2, 8, 2, nil
		}
		if z80IgnoredIndexPrefixIncDecOpcode(opcode) {
			return 2, 8, 2, nil
		}
		if z80IgnoredIndexPrefixALURegOpcode(opcode) {
			return 2, 8, 2, nil
		}
		if z80IgnoredIndexPrefixPairLoadOpcode(opcode) {
			return 4, 14, 2, nil
		}
		if z80IgnoredIndexPrefixPairIncDecOpcode(opcode) {
			return 2, 10, 2, nil
		}
	}
	if prefix == z80JITPrefixED {
		if !z80EDDefinedOpcode(opcode) {
			return 2, 8, 2, nil // Undefined ED forms are architectural 8T NOPs.
		}
		switch opcode {
		case 0x44, 0x4C, 0x54, 0x5C, 0x64, 0x6C, 0x74, 0x7C: // NEG aliases
			return 2, 8, 2, nil
		case 0x47: // LD I,A
			return 2, 9, 2, nil
		case 0x57: // LD A,I
			return 2, 9, 2, nil
		case 0x4F, 0x5F: // LD R,A / LD A,R
			return 2, 9, 2, nil
		case 0x42, 0x52, 0x62, 0x72, 0x4A, 0x5A, 0x6A, 0x7A: // SBC/ADC HL,rp
			return 2, 15, 2, nil
		case 0x43, 0x4B, 0x53, 0x5B, 0x63, 0x6B, 0x73, 0x7B:
			return 4, 20, 2, nil
		case 0x45, 0x4D, 0x55, 0x5D, 0x65, 0x6D, 0x75, 0x7D:
			return 2, 14, 2, nil
		case 0x67, 0x6F:
			return 2, 18, 2, nil
		case 0xA0, 0xA8, 0xA1, 0xA9, 0xB0, 0xB8, 0xB1, 0xB9:
			return 2, 16, 2, nil
		case 0x46, 0x4E, 0x66, 0x6E, 0x56, 0x76, 0x5E, 0x7E: // IM aliases
			return 2, 8, 2, nil
		}
	}
	if prefix != z80JITPrefixNone {
		return 0, 0, 0, fmt.Errorf("z80 wasm: unsupported prefix %02X", prefix)
	}
	return z80WasmInstruction(opcode, operand)
}

func z80EDDefinedOpcode(opcode byte) bool {
	switch opcode {
	case 0x40, 0x48, 0x50, 0x58, 0x60, 0x68, 0x70, 0x78,
		0x41, 0x49, 0x51, 0x59, 0x61, 0x69, 0x71, 0x79,
		0x44, 0x4C, 0x54, 0x5C, 0x64, 0x6C, 0x74, 0x7C,
		0x47, 0x4F, 0x57, 0x5F,
		0x46, 0x4E, 0x56, 0x5E, 0x66, 0x6E, 0x76, 0x7E,
		0x45, 0x4D, 0x55, 0x5D, 0x65, 0x6D, 0x75, 0x7D,
		0x67, 0x6F,
		0xA0, 0xA1, 0xA2, 0xA3, 0xA8, 0xA9, 0xAA, 0xAB,
		0xB0, 0xB1, 0xB2, 0xB3, 0xB8, 0xB9, 0xBA, 0xBB,
		0x43, 0x4B, 0x53, 0x5B, 0x63, 0x6B, 0x73, 0x7B,
		0x4A, 0x5A, 0x6A, 0x7A, 0x42, 0x52, 0x62, 0x72:
		return true
	default:
		return false
	}
}

func z80IgnoredIndexPrefixDirectOpcode(opcode byte) bool {
	switch opcode {
	case 0x00, 0x07, 0x0F, 0x17, 0x1F, 0x2F, 0x37, 0x3F, 0x08, 0xD9, 0xEB:
		return true
	}
	return false
}

func z80IgnoredIndexPrefixLDImmOpcode(opcode byte) bool {
	if opcode&0xC7 != 0x06 {
		return false
	}
	switch (opcode >> 3) & 7 {
	case 0, 1, 2, 3, 7:
		return true
	}
	return false
}

func z80IgnoredIndexPrefixLDRegOpcode(opcode byte) bool {
	if opcode < 0x40 || opcode > 0x7F || opcode == 0x76 {
		return false
	}
	dst, src := (opcode>>3)&7, opcode&7
	return (dst == 0 || dst == 1 || dst == 2 || dst == 3 || dst == 7) && (src == 0 || src == 1 || src == 2 || src == 3 || src == 7)
}

func z80IgnoredIndexPrefixIncDecOpcode(opcode byte) bool {
	if opcode&0xC7 != 0x04 && opcode&0xC7 != 0x05 {
		return false
	}
	switch (opcode >> 3) & 7 {
	case 0, 1, 2, 3, 7:
		return true
	}
	return false
}

// z80IgnoredIndexPrefixALURegOpcode identifies ALU A,r forms for which a
// DD/FD prefix is genuinely ignored. H, L and (HL) are excluded: those name
// IXH/IYH, IXL/IYL and (IX/IY+d) under an index prefix.
func z80IgnoredIndexPrefixALURegOpcode(opcode byte) bool {
	if opcode < 0x80 || opcode > 0xBF {
		return false
	}
	switch opcode & 7 {
	case 0, 1, 2, 3, 7:
		return true
	}
	return false
}

// DD/FD only replace the HL register-pair slot. The BC, DE and SP pair forms
// retain their base semantics, while the prefix itself consumes one M1 cycle.
func z80IgnoredIndexPrefixPairLoadOpcode(opcode byte) bool {
	if opcode&0xCF != 0x01 {
		return false
	}
	switch (opcode >> 4) & 3 {
	case 0, 1, 3:
		return true
	}
	return false
}

func z80IgnoredIndexPrefixPairIncDecOpcode(opcode byte) bool {
	if opcode&0xCF != 0x03 && opcode&0xCF != 0x0B {
		return false
	}
	switch (opcode >> 4) & 3 {
	case 0, 1, 3:
		return true
	}
	return false
}

// SET/RES CB forms only modify their register operand; unlike rotate and BIT,
// they have no flag calculation. The (HL) encoding remains a guarded-memory
// operation and is intentionally not admitted by this register-only helper.
func z80CBSetResRegisterOpcode(opcode byte) bool {
	return opcode>>6 >= 2 && opcode&7 != 6
}
func z80CBSetResHLOpcode(opcode byte) bool { return opcode>>6 >= 2 && opcode&7 == 6 }

func z80CBRegisterOpcode(opcode byte) bool {
	return (opcode>>6 == 1 || z80CBSetResRegisterOpcode(opcode)) && opcode&7 != 6
}

func z80CBRLCRegisterOpcode(opcode byte) bool {
	return opcode&0xF8 == 0 && opcode&7 != 6
}

func z80CBRRCRegisterOpcode(opcode byte) bool {
	return opcode&0xF8 == 0x08 && opcode&7 != 6
}
func z80CBRLRegisterOpcode(opcode byte) bool { return opcode&0xF8 == 0x10 && opcode&7 != 6 }
func z80CBRRRegisterOpcode(opcode byte) bool { return opcode&0xF8 == 0x18 && opcode&7 != 6 }

func z80CBSLARegisterOpcode(opcode byte) bool { return opcode&0xF8 == 0x20 && opcode&7 != 6 }
func z80CBSRLRegisterOpcode(opcode byte) bool { return opcode&0xF8 == 0x38 && opcode&7 != 6 }
func z80CBSRARegisterOpcode(opcode byte) bool { return opcode&0xF8 == 0x28 && opcode&7 != 6 }
func z80CBSLLRegisterOpcode(opcode byte) bool { return opcode&0xF8 == 0x30 && opcode&7 != 6 }

func z80WasmRegisterOffset(index byte) (uint32, bool) {
	switch index {
	case 0:
		return z80WasmCPUOffB, true
	case 1:
		return z80WasmCPUOffC, true
	case 2:
		return z80WasmCPUOffD, true
	case 3:
		return z80WasmCPUOffE, true
	case 4:
		return z80WasmCPUOffH, true
	case 5:
		return z80WasmCPUOffL, true
	case 7:
		return z80WasmCPUOffA, true
	default: // index 6 is (HL), an observation boundary
		return 0, false
	}
}

func z80WasmInstrRegisterOffset(instr z80WasmInstr, index byte) (uint32, bool) {
	if (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && z80WasmDDFDUsesIndexBytes(instr.opcode) {
		base := uint32(z80WasmCPUOffIX)
		if instr.prefix == z80JITPrefixFD {
			base = z80WasmCPUOffIY
		}
		switch index {
		case 4:
			return base + 1, true
		case 5:
			return base, true
		}
	}
	return z80WasmRegisterOffset(index)
}

func z80WasmDDFDExplicitOpcode(op byte) bool {
	return op == 0x21 || op == 0x22 || op == 0x2A || op == 0xE5 || op == 0xE1 ||
		op == 0xF9 || op == 0x36 || op == 0x34 || op == 0x35 || op == 0xE9 ||
		op == 0xCB || op == 0xE3 || op == 0x09 || op == 0x19 || op == 0x29 ||
		op == 0x39 || op == 0x23 || op == 0x2B ||
		(op&0xC7 == 0x46 && op != 0x76) ||
		(op >= 0x70 && op <= 0x77 && op != 0x76) || op&0xC7 == 0x86
}

func z80WasmDDFDUsesIndexBytes(op byte) bool {
	if (op&0xC7 == 0x04 || op&0xC7 == 0x05 || op&0xC7 == 0x06) && ((op>>3)&7 == 4 || (op>>3)&7 == 5) {
		return true
	}
	if op >= 0x40 && op <= 0x7F && op != 0x76 {
		dst, src := (op>>3)&7, op&7
		return dst == 4 || dst == 5 || src == 4 || src == 5
	}
	return op >= 0x80 && op <= 0xBF && (op&7 == 4 || op&7 == 5)
}

func z80WasmPair8Offsets(pair byte) (high, low uint32) {
	switch pair {
	case 0:
		return z80WasmCPUOffB, z80WasmCPUOffC
	case 1:
		return z80WasmCPUOffD, z80WasmCPUOffE
	case 2:
		return z80WasmCPUOffH, z80WasmCPUOffL
	default:
		panic("wasm Z80 invalid register pair")
	}
}

func z80WasmEmitSetWZ(body *wasmBody, value uint16) {
	body.localGet(1)
	body.i32Const(int32(value))
	body.i32Store16(1, z80WasmCPUOffWZ)
}

// z80WasmEmitCondition leaves the Z80 condition-code truth value on stack.
func z80WasmEmitCondition(body *wasmBody, condition byte) {
	mask := byte(z80FlagZ)
	switch condition >> 1 {
	case 1:
		mask = z80FlagC
	case 2:
		mask = z80FlagPV
	case 3:
		mask = z80FlagS
	}
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(int32(mask))
	body.op(wasmOpI32And)
	if condition&1 == 0 {
		body.op(wasmOpI32Eqz)
	} else {
		body.op(wasmOpI32Eqz)
		body.op(wasmOpI32Eqz)
	}
}

func z80WasmInstruction(opcode, operand byte) (length, cycles, rInc byte, err error) {
	switch {
	case opcode == 0x00: // NOP
		return 1, 4, 1, nil
	case opcode == 0x02 || opcode == 0x12: // LD (BC)/(DE),A
		return 1, 7, 1, nil
	case opcode == 0x3A: // LD A,(nn)
		return 3, 13, 1, nil
	case opcode == 0x2A: // LD HL,(nn)
		return 3, 16, 1, nil
	case opcode == 0x22: // LD (nn),HL
		return 3, 16, 1, nil
	case opcode == 0x32: // LD (nn),A
		return 3, 13, 1, nil
	case opcode == 0x0A || opcode == 0x1A: // LD A,(BC)/(DE)
		return 1, 7, 1, nil
	case opcode&0xC7 == 0x06: // LD r,n, excluding LD (HL),n
		if opcode == 0x36 { // LD (HL),n
			return 2, 10, 1, nil
		}
		if _, ok := z80WasmRegisterOffset((opcode >> 3) & 7); !ok {
			return 0, 0, 0, fmt.Errorf("z80 wasm: non-register LD r,n %02X", opcode)
		}
		return 2, 7, 1, nil
	case opcode >= 0x70 && opcode <= 0x77 && opcode != 0x76: // LD (HL),r
		return 1, 7, 1, nil
	case opcode&0xCF == 0x01: // LD rp,nn
		return 3, 10, 1, nil
	case opcode&0xCF == 0xC5: // PUSH rp
		return 1, 11, 1, nil
	case opcode&0xCF == 0xC1: // POP rp
		return 1, 10, 1, nil
	case opcode&0xCF == 0x03 || opcode&0xCF == 0x0B: // INC/DEC rp
		return 1, 6, 1, nil
	case opcode&0xCF == 0x09: // ADD HL,rp
		return 1, 11, 1, nil
	case opcode == 0x08 || opcode == 0xD9 || opcode == 0xEB: // EX AF,AF' / EXX / EX DE,HL
		return 1, 4, 1, nil
	case opcode == 0x37 || opcode == 0x3F || opcode == 0x2F: // SCF / CCF / CPL
		return 1, 4, 1, nil
	case opcode == 0x07 || opcode == 0x0F || opcode == 0x17 || opcode == 0x1F:
		return 1, 4, 1, nil
	case opcode == 0x27: // DAA
		return 1, 4, 1, nil
	case opcode&0xC7 == 0x04 || opcode&0xC7 == 0x05: // INC/DEC r, excluding (HL)
		if opcode == 0x34 || opcode == 0x35 {
			return 1, 11, 1, nil
		}
		if _, ok := z80WasmRegisterOffset((opcode >> 3) & 7); !ok {
			return 0, 0, 0, fmt.Errorf("z80 wasm: memory INC/DEC %02X", opcode)
		}
		return 1, 4, 1, nil
	case opcode == 0xF9: // LD SP,HL
		return 1, 6, 1, nil
	case opcode >= 0x40 && opcode <= 0x7F && opcode != 0x76: // LD r,r
		dst := (opcode >> 3) & 7
		src := opcode & 7
		if src == 6 && dst != 6 { // LD r,(HL), guarded direct-memory form
			return 1, 7, 1, nil
		}
		if _, dstOK := z80WasmRegisterOffset(dst); !dstOK {
			return 0, 0, 0, fmt.Errorf("z80 wasm: non-register LD r,r %02X", opcode)
		} else if _, srcOK := z80WasmRegisterOffset(src); !srcOK {
			return 0, 0, 0, fmt.Errorf("z80 wasm: non-register LD r,r %02X", opcode)
		}
		return 1, 4, 1, nil
	case opcode >= 0x80 && opcode <= 0x87: // ADD A,r
		if opcode == 0x86 { // ADD A,(HL), guarded direct-memory form
			return 1, 7, 1, nil
		}
		if _, ok := z80WasmRegisterOffset(opcode & 7); !ok {
			return 0, 0, 0, fmt.Errorf("z80 wasm: memory ADD %02X", opcode)
		}
		return 1, 4, 1, nil
	case opcode == 0xC6: // ADD A,n
		return 2, 7, 1, nil
	case opcode >= 0x88 && opcode <= 0x8F: // ADC A,r
		if opcode&7 == 6 {
			return 1, 7, 1, nil
		}
		if _, ok := z80WasmRegisterOffset(opcode & 7); !ok {
			return 0, 0, 0, fmt.Errorf("z80 wasm: memory ADC %02X", opcode)
		}
		return 1, 4, 1, nil
	case opcode == 0xCE: // ADC A,n
		return 2, 7, 1, nil
	case opcode >= 0x90 && opcode <= 0x97: // SUB A,r
		if opcode&7 == 6 {
			return 1, 7, 1, nil
		}
		if _, ok := z80WasmRegisterOffset(opcode & 7); !ok {
			return 0, 0, 0, fmt.Errorf("z80 wasm: memory SUB %02X", opcode)
		}
		return 1, 4, 1, nil
	case opcode == 0xD6: // SUB A,n
		return 2, 7, 1, nil
	case opcode >= 0x98 && opcode <= 0x9F: // SBC A,r
		if opcode&7 == 6 {
			return 1, 7, 1, nil
		}
		if _, ok := z80WasmRegisterOffset(opcode & 7); !ok {
			return 0, 0, 0, fmt.Errorf("z80 wasm: memory SBC %02X", opcode)
		}
		return 1, 4, 1, nil
	case opcode == 0xDE: // SBC A,n
		return 2, 7, 1, nil
	case opcode >= 0xB8 && opcode <= 0xBF: // CP A,r
		if opcode&7 == 6 {
			return 1, 7, 1, nil
		}
		if _, ok := z80WasmRegisterOffset(opcode & 7); !ok {
			return 0, 0, 0, fmt.Errorf("z80 wasm: memory CP %02X", opcode)
		}
		return 1, 4, 1, nil
	case opcode == 0xFE: // CP A,n
		return 2, 7, 1, nil
	case opcode == 0xC3: // JP nn, statically known dispatcher target
		return 3, 10, 1, nil
	case opcode == 0x18: // JR e, statically known dispatcher target
		return 2, 12, 1, nil
	case opcode == 0x10: // DJNZ e
		return 2, 8, 1, nil
	case opcode == 0x20 || opcode == 0x28 || opcode == 0x30 || opcode == 0x38: // JR cc,e
		return 2, 7, 1, nil
	case opcode&0xC7 == 0xC2: // JP cc,nn
		return 3, 10, 1, nil
	case opcode == 0xCD: // CALL nn
		return 3, 17, 1, nil
	case opcode == 0xC9: // RET
		return 1, 10, 1, nil
	case opcode&0xC7 == 0xC4: // CALL cc,nn
		return 3, 10, 1, nil
	case opcode&0xC7 == 0xC0: // RET cc
		return 1, 5, 1, nil
	case opcode&0xC7 == 0xC7: // RST n
		return 1, 11, 1, nil
	case opcode == 0xE9: // JP (HL)
		return 1, 4, 1, nil
	case opcode == 0xF3 || opcode == 0xFB: // DI / EI
		return 1, 4, 1, nil
	case opcode == 0xE3: // EX (SP),HL
		return 1, 19, 1, nil
	case opcode >= 0xA0 && opcode <= 0xB7: // AND/XOR/OR A,r
		if opcode&7 == 6 {
			return 1, 7, 1, nil
		}
		if _, ok := z80WasmRegisterOffset(opcode & 7); !ok {
			return 0, 0, 0, fmt.Errorf("z80 wasm: memory logical ALU %02X", opcode)
		}
		return 1, 4, 1, nil
	case opcode == 0xE6 || opcode == 0xEE || opcode == 0xF6: // AND/XOR/OR A,n
		return 2, 7, 1, nil
	default:
		return 0, 0, 0, fmt.Errorf("z80 wasm: unsupported opcode %02X", opcode)
	}
}

// z80WasmCompileBlock creates a module importing Go's linear memory as
// env.mem and exporting block(ctx i32).  It contains no host calls.
func z80WasmCompileBlock(instrs []z80WasmInstr, startPC uint16) ([]byte, error) {
	if len(instrs) == 0 {
		return nil, fmt.Errorf("z80 wasm: empty block")
	}
	var body wasmBody
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffCPUPtr)
	body.localSet(1)

	pc := uint32(startPC)
	cycles, rIncrements := uint32(0), uint32(0)
	dynamicPC := false
	dynamicCycles := false
	for i, instr := range instrs {
		length, cost, rInc, err := z80WasmInstructionMeta(instr)
		if err != nil {
			return nil, err
		}
		switch {
		case instr.indexedCB:
			z80WasmEmitIndexedAddress(&body, instr.prefix, instr.displacement)
			z80WasmEmitGuardedLoadAddress(&body, uint16(pc), uint32(i), cycles, rIncrements)
			switch instr.opcode >> 6 {
			case 0:
				z80WasmEmitCBMemoryRotate(&body, instr.opcode)
				z80WasmEmitIndexedResultRegister(&body, instr.opcode)
				z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
			case 1:
				z80WasmEmitCBBITValue(&body, instr.opcode)
			case 2, 3:
				body.localGet(3)
				body.i32Const(int32(1 << ((instr.opcode >> 3) & 7)))
				if instr.opcode>>6 == 2 {
					body.i32Const(0xFF)
					body.op(wasmOpI32Xor)
					body.op(wasmOpI32And)
				} else {
					body.op(wasmOpI32Or)
				}
				body.localSet(3)
				z80WasmEmitIndexedResultRegister(&body, instr.opcode)
				z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
			}
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0x21:
			body.localGet(1)
			body.i32Const(int32(uint16(instr.operand) | uint16(instr.operandHi)<<8))
			body.i32Store16(1, z80WasmIndexOffset(instr.prefix))
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0x22:
			body.localGet(1)
			body.i32Load16U(1, z80WasmIndexOffset(instr.prefix))
			body.localSet(3)
			address := uint16(instr.operand) | uint16(instr.operandHi)<<8
			z80WasmEmitStoreWordAddress(&body, address, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitSetWZ(&body, address+1)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0x2A:
			address := uint16(instr.operand) | uint16(instr.operandHi)<<8
			z80WasmEmitLoadWordAddress(&body, address, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(1)
			body.localGet(3)
			body.i32Store16(1, z80WasmIndexOffset(instr.prefix))
			z80WasmEmitSetWZ(&body, address+1)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && (instr.opcode == 0x23 || instr.opcode == 0x2B):
			body.localGet(1)
			body.localGet(1)
			body.i32Load16U(1, z80WasmIndexOffset(instr.prefix))
			body.i32Const(1)
			if instr.opcode == 0x23 {
				body.op(wasmOpI32Add)
			} else {
				body.op(wasmOpI32Sub)
			}
			body.i32Store16(1, z80WasmIndexOffset(instr.prefix))
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && (instr.opcode == 0x09 || instr.opcode == 0x19 || instr.opcode == 0x29 || instr.opcode == 0x39):
			z80WasmEmitAddIndexPair(&body, instr.prefix, (instr.opcode>>4)&3)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0xF9:
			body.localGet(1)
			body.localGet(1)
			body.i32Load16U(1, z80WasmIndexOffset(instr.prefix))
			body.i32Store16(1, z80WasmCPUOffSP)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0xE5:
			body.localGet(1)
			body.i32Load16U(1, z80WasmIndexOffset(instr.prefix))
			body.localSet(3)
			z80WasmEmitPushWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0xE1:
			z80WasmEmitPopWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(1)
			body.localGet(3)
			body.i32Store16(1, z80WasmIndexOffset(instr.prefix))
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0xE9:
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("z80 wasm: non-terminal JP (IX/IY)")
			}
			body.localGet(1)
			body.i32Load16U(1, z80WasmIndexOffset(instr.prefix))
			body.localSet(4)
			body.localGet(1)
			body.localGet(4)
			body.i32Store16(1, z80WasmCPUOffWZ)
			dynamicPC = true
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0xE3:
			z80WasmEmitEXSPIndex(&body, instr.prefix, uint16(pc), uint32(i), cycles, rIncrements)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && (instr.opcode == 0x34 || instr.opcode == 0x35):
			z80WasmEmitIndexedAddress(&body, instr.prefix, instr.displacement)
			z80WasmEmitGuardedLoadAddress(&body, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitMemoryIncDec(&body, instr.opcode == 0x35)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0x36:
			z80WasmEmitIndexedAddress(&body, instr.prefix, instr.displacement)
			body.i32Const(int32(instr.operand))
			body.localSet(3)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode&0xC7 == 0x46 && instr.opcode != 0x76:
			z80WasmEmitIndexedAddress(&body, instr.prefix, instr.displacement)
			z80WasmEmitGuardedLoadAddress(&body, uint16(pc), uint32(i), cycles, rIncrements)
			dst, _ := z80WasmRegisterOffset((instr.opcode >> 3) & 7)
			body.localGet(1)
			body.localGet(3)
			body.i32Store8(0, dst)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode >= 0x70 && instr.opcode <= 0x77 && instr.opcode != 0x76:
			z80WasmEmitIndexedAddress(&body, instr.prefix, instr.displacement)
			src, _ := z80WasmRegisterOffset(instr.opcode & 7)
			body.localGet(1)
			body.i32Load8U(0, src)
			body.localSet(3)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
		case (instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode&0xC7 == 0x86:
			z80WasmEmitIndexedAddress(&body, instr.prefix, instr.displacement)
			z80WasmEmitGuardedLoadAddress(&body, uint16(pc), uint32(i), cycles, rIncrements)
			switch (instr.opcode >> 3) & 7 {
			case 0:
				z80WasmEmitAddReg(&body, false)
			case 1:
				z80WasmEmitAddReg(&body, true)
			case 2:
				z80WasmEmitSubReg(&body, false)
			case 3:
				z80WasmEmitSubReg(&body, true)
			case 4, 5, 6:
				body.localGet(3)
				body.localSet(2)
				z80WasmEmitLogicALU(&body, 0xE6+byte(((instr.opcode>>3)&7)-4)*8)
			case 7:
				z80WasmEmitSubReg(&body, false)
				body.localGet(1)
				body.localGet(2)
				body.i32Store8(0, z80WasmCPUOffA)
			}
		case instr.prefix == z80JITPrefixED && !z80EDDefinedOpcode(instr.opcode):
			// CPU_Z80 treats every undefined ED form as an 8T NOP.
		case instr.prefix == z80JITPrefixCB && instr.opcode&7 == 6 && instr.opcode>>6 == 0:
			z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitCBMemoryRotate(&body, instr.opcode)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
		case instr.prefix == z80JITPrefixCB && instr.opcode&7 == 6 && instr.opcode>>6 == 1:
			z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitCBBITValue(&body, instr.opcode)
		case instr.prefix == z80JITPrefixCB && z80CBSetResHLOpcode(instr.opcode):
			z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(3)
			body.i32Const(int32(1 << ((instr.opcode >> 3) & 7)))
			if instr.opcode>>6 == 2 {
				body.i32Const(0xFF)
				body.op(wasmOpI32Xor)
				body.op(wasmOpI32And)
			} else {
				body.op(wasmOpI32Or)
			}
			body.localSet(3)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), false)
		case instr.prefix == z80JITPrefixCB && z80CBRLCRegisterOpcode(instr.opcode):
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			z80WasmEmitCBRLCReg(&body, offset)
		case instr.prefix == z80JITPrefixCB && z80CBRRCRegisterOpcode(instr.opcode):
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			z80WasmEmitCBRRCReg(&body, offset)
		case instr.prefix == z80JITPrefixCB && z80CBRLRegisterOpcode(instr.opcode):
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			z80WasmEmitCBRLReg(&body, offset)
		case instr.prefix == z80JITPrefixCB && z80CBRRRegisterOpcode(instr.opcode):
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			z80WasmEmitCBRRReg(&body, offset)
		case instr.prefix == z80JITPrefixCB && z80CBSLARegisterOpcode(instr.opcode):
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			z80WasmEmitCBSLAReg(&body, offset)
		case instr.prefix == z80JITPrefixCB && z80CBSRLRegisterOpcode(instr.opcode):
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			z80WasmEmitCBSRLReg(&body, offset)
		case instr.prefix == z80JITPrefixCB && z80CBSRARegisterOpcode(instr.opcode):
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			z80WasmEmitCBSRAReg(&body, offset)
		case instr.prefix == z80JITPrefixCB && z80CBSLLRegisterOpcode(instr.opcode):
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			z80WasmEmitCBSLLReg(&body, offset)
		case instr.prefix == z80JITPrefixED && (instr.opcode == 0x57 || instr.opcode == 0x5F): // LD A,I / LD A,R
			if instr.opcode == 0x57 {
				body.localGet(1)
				body.i32Load8U(0, z80WasmCPUOffI)
			} else {
				body.localGet(1)
				body.i32Load8U(0, z80WasmCPUOffR)
				body.localSet(2)
				body.localGet(2)
				body.i32Const(0x80)
				body.op(wasmOpI32And)
				body.localGet(2)
				body.i32Const(int32(rIncrements + uint32(rInc)))
				body.op(wasmOpI32Add)
				body.i32Const(0x7F)
				body.op(wasmOpI32And)
				body.op(wasmOpI32Or)
			}
			body.localSet(2)
			body.localGet(1)
			body.localGet(2)
			body.i32Store8(0, z80WasmCPUOffA)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffF)
			body.i32Const(z80FlagC)
			body.op(wasmOpI32And)
			body.localSet(4)
			body.localGet(4)
			body.localGet(2)
			body.i32Const(z80FlagS | z80FlagX | z80FlagY)
			body.op(wasmOpI32And)
			body.op(wasmOpI32Or)
			body.localSet(4)
			body.localGet(2)
			body.op(wasmOpI32Eqz)
			body.ifVoid()
			body.localGet(4)
			body.i32Const(z80FlagZ)
			body.op(wasmOpI32Or)
			body.localSet(4)
			body.end()
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffIFF2)
			body.ifVoid()
			body.localGet(4)
			body.i32Const(z80FlagPV)
			body.op(wasmOpI32Or)
			body.localSet(4)
			body.end()
			body.localGet(1)
			body.localGet(4)
			body.i32Store8(0, z80WasmCPUOffF)
		case instr.prefix == z80JITPrefixED && z80EDNEGOpcode(instr.opcode): // NEG = 0-A
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffA)
			body.localSet(3)
			body.localGet(1)
			body.i32Const(0)
			body.i32Store8(0, z80WasmCPUOffA)
			z80WasmEmitSubReg(&body, false)
		case instr.prefix == z80JITPrefixED && z80EDInterruptMode(instr.opcode) >= 0:
			mode := z80EDInterruptMode(instr.opcode)
			body.localGet(1)
			body.i32Const(int32(mode))
			body.i32Store8(0, z80WasmCPUOffIM)
		case instr.prefix == z80JITPrefixED && instr.opcode == 0x47:
			body.localGet(1)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffA)
			body.i32Store8(0, z80WasmCPUOffI)
		case instr.prefix == z80JITPrefixED && instr.opcode == 0x4F: // LD R,A
			body.localGet(1)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffA)
			body.localSet(2)
			body.localGet(2)
			body.i32Const(0x80)
			body.op(wasmOpI32And)
			body.localGet(2)
			body.i32Const(int32(rIncrements + uint32(rInc)))
			body.op(wasmOpI32Sub)
			body.i32Const(0x7F)
			body.op(wasmOpI32And)
			body.op(wasmOpI32Or)
			body.i32Store8(0, z80WasmCPUOffR)
		case instr.prefix == z80JITPrefixED && (instr.opcode&0xCF == 0x42 || instr.opcode&0xCF == 0x4A):
			z80WasmEmitADCSBCHL(&body, (instr.opcode>>4)&3, instr.opcode&0xCF == 0x42)
		case instr.prefix == z80JITPrefixED && instr.opcode&0x0F == 0x03:
			z80WasmEmitLoadRPPair(&body, (instr.opcode>>4)&3)
			z80WasmEmitStoreWordAddress(&body, uint16(instr.operand)|uint16(instr.operandHi)<<8, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitSetWZ(&body, uint16(instr.operand)+uint16(instr.operandHi)<<8+1)
		case instr.prefix == z80JITPrefixED && instr.opcode&0x0F == 0x0B:
			z80WasmEmitLoadWordAddress(&body, uint16(instr.operand)|uint16(instr.operandHi)<<8, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitStoreRPPair(&body, (instr.opcode>>4)&3)
			z80WasmEmitSetWZ(&body, uint16(instr.operand)+uint16(instr.operandHi)<<8+1)
		case instr.prefix == z80JITPrefixED && (instr.opcode == 0x45 || instr.opcode == 0x4D || instr.opcode == 0x55 || instr.opcode == 0x5D || instr.opcode == 0x65 || instr.opcode == 0x6D || instr.opcode == 0x75 || instr.opcode == 0x7D):
			z80WasmEmitPopWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(1)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffIFF2)
			body.i32Store8(0, z80WasmCPUOffIFF1)
			body.localGet(3)
			body.localSet(4)
			dynamicPC = true
		case instr.prefix == z80JITPrefixED && (instr.opcode == 0x67 || instr.opcode == 0x6F):
			z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(3)
			body.localSet(2)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffA)
			body.localSet(4)
			body.localGet(1)
			body.localGet(4)
			body.i32Const(0xF0)
			body.op(wasmOpI32And)
			body.localGet(2)
			if instr.opcode == 0x67 {
				body.i32Const(0x0F)
				body.op(wasmOpI32And)
			} else {
				body.i32Const(4)
				body.op(wasmOpI32ShrU)
			}
			body.op(wasmOpI32Or)
			body.i32Store8(0, z80WasmCPUOffA)
			body.localGet(2)
			if instr.opcode == 0x67 {
				body.i32Const(4)
				body.op(wasmOpI32ShrU)
				body.localGet(4)
				body.i32Const(4)
				body.op(wasmOpI32Shl)
			} else {
				body.i32Const(4)
				body.op(wasmOpI32Shl)
				body.localGet(4)
				body.i32Const(0x0F)
				body.op(wasmOpI32And)
			}
			body.op(wasmOpI32Or)
			body.i32Const(0xFF)
			body.op(wasmOpI32And)
			body.localSet(3)
			z80WasmEmitAParityFlags(&body)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
		case instr.prefix == z80JITPrefixED && (instr.opcode == 0xA0 || instr.opcode == 0xA8 || instr.opcode == 0xB0 || instr.opcode == 0xB8):
			z80WasmEmitBlockTransfer(&body, instr.opcode == 0xA8 || instr.opcode == 0xB8, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc))
			if instr.opcode == 0xB0 || instr.opcode == 0xB8 {
				nextPC := uint16(pc) + uint16(length)
				body.localGet(1)
				body.i32Load8U(0, z80WasmCPUOffB)
				body.localGet(1)
				body.i32Load8U(0, z80WasmCPUOffC)
				body.op(wasmOpI32Or)
				body.ifVoid()
				body.i32Const(int32(uint16(pc)))
				body.localSet(4)
				body.i32Const(5)
				body.localSet(7)
				body.elseBranch()
				body.i32Const(int32(nextPC))
				body.localSet(4)
				body.i32Const(0)
				body.localSet(7)
				body.end()
				dynamicPC, dynamicCycles = true, true
			}
		case instr.prefix == z80JITPrefixED && (instr.opcode == 0xA1 || instr.opcode == 0xA9 || instr.opcode == 0xB1 || instr.opcode == 0xB9):
			z80WasmEmitBlockCompare(&body, instr.opcode == 0xA9 || instr.opcode == 0xB9, uint16(pc), uint32(i), cycles, rIncrements)
			if instr.opcode == 0xB1 || instr.opcode == 0xB9 {
				nextPC := uint16(pc) + uint16(length)
				body.localGet(1)
				body.i32Load8U(0, z80WasmCPUOffB)
				body.localGet(1)
				body.i32Load8U(0, z80WasmCPUOffC)
				body.op(wasmOpI32Or)
				body.op(wasmOpI32Eqz)
				body.op(wasmOpI32Eqz)
				body.localGet(1)
				body.i32Load8U(0, z80WasmCPUOffF)
				body.i32Const(z80FlagZ)
				body.op(wasmOpI32And)
				body.op(wasmOpI32Eqz)
				body.op(wasmOpI32And)
				body.ifVoid()
				body.i32Const(int32(uint16(pc)))
				body.localSet(4)
				body.i32Const(5)
				body.localSet(7)
				body.elseBranch()
				body.i32Const(int32(nextPC))
				body.localSet(4)
				body.i32Const(0)
				body.localSet(7)
				body.end()
				dynamicPC, dynamicCycles = true, true
			}
		case instr.prefix == z80JITPrefixCB && z80CBSetResRegisterOpcode(instr.opcode):
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			body.localGet(1)
			body.localGet(1)
			body.i32Load8U(0, offset)
			if instr.opcode>>6 == 2 { // RES
				body.i32Const(int32(^byte(1 << ((instr.opcode >> 3) & 7))))
				body.op(wasmOpI32And)
			} else { // SET
				body.i32Const(int32(1 << ((instr.opcode >> 3) & 7)))
				body.op(wasmOpI32Or)
			}
			body.i32Store8(0, offset)
		case instr.prefix == z80JITPrefixCB && instr.opcode>>6 == 1: // BIT b,r
			offset, _ := z80WasmRegisterOffset(instr.opcode & 7)
			body.localGet(1)
			body.i32Load8U(0, offset)
			body.localSet(3)
			z80WasmEmitCBBITValue(&body, instr.opcode)
		case instr.opcode&0xC7 == 0x06:
			if instr.opcode == 0x36 {
				body.i32Const(int32(instr.operand))
				body.localSet(3)
				z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), false)
				break
			}
			dst, _ := z80WasmInstrRegisterOffset(instr, (instr.opcode>>3)&7)
			body.localGet(1)
			body.i32Const(int32(instr.operand))
			body.i32Store8(0, dst)
		case instr.opcode == 0x02 || instr.opcode == 0x12:
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffA)
			body.localSet(3)
			high, low := z80WasmPair8Offsets((instr.opcode >> 4) & 1)
			body.localGet(1)
			body.i32Load8U(0, high)
			body.i32Const(8)
			body.op(wasmOpI32Shl)
			body.localGet(1)
			body.i32Load8U(0, low)
			body.op(wasmOpI32Or)
			body.localSet(6)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
		case instr.opcode == 0x3A:
			body.i32Const(int32(uint16(instr.operand) | uint16(instr.operandHi)<<8))
			body.localSet(6)
			z80WasmEmitGuardedLoadAddress(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(1)
			body.localGet(3)
			body.i32Store8(0, z80WasmCPUOffA)
			z80WasmEmitSetWZ(&body, uint16(instr.operand)|uint16(instr.operandHi)<<8)
		case instr.opcode == 0x22:
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffH)
			body.i32Const(8)
			body.op(wasmOpI32Shl)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffL)
			body.op(wasmOpI32Or)
			body.localSet(3)
			address := uint16(instr.operand) | uint16(instr.operandHi)<<8
			z80WasmEmitStoreWordAddress(&body, address, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitSetWZ(&body, address+1)
		case instr.opcode == 0x2A:
			address := uint16(instr.operand) | uint16(instr.operandHi)<<8
			body.i32Const(int32(address))
			body.localSet(6)
			z80WasmEmitGuardedLoadAddress(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(3)
			body.localSet(4) // low byte, no architectural write yet
			body.i32Const(int32(uint16(address + 1)))
			body.localSet(6)
			z80WasmEmitGuardedLoadAddress(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(1)
			body.localGet(3)
			body.i32Store8(0, z80WasmCPUOffH)
			body.localGet(1)
			body.localGet(4)
			body.i32Store8(0, z80WasmCPUOffL)
			z80WasmEmitSetWZ(&body, address+1)
		case instr.opcode >= 0x70 && instr.opcode <= 0x77 && instr.opcode != 0x76:
			src, _ := z80WasmRegisterOffset(instr.opcode & 7)
			body.localGet(1)
			body.i32Load8U(0, src)
			body.localSet(3)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), false)
		case instr.opcode == 0x32:
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffA)
			body.localSet(3)
			address := uint16(instr.operand) | uint16(instr.operandHi)<<8
			body.i32Const(int32(address))
			body.localSet(6)
			z80WasmEmitGuardDirectAddress(&body, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitSetWZ(&body, address)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
		case instr.opcode == 0x0A || instr.opcode == 0x1A:
			high, low := z80WasmPair8Offsets((instr.opcode >> 4) & 1)
			z80WasmEmitGuardedLoadPair(&body, uint16(pc), uint32(i), cycles, rIncrements, high, low)
			body.localGet(1)
			body.localGet(3)
			body.i32Store8(0, z80WasmCPUOffA)
		case instr.opcode&0xCF == 0x01:
			pair := (instr.opcode >> 4) & 0x03
			if pair == 3 {
				body.localGet(1)
				body.i32Const(int32(uint16(instr.operand) | uint16(instr.operandHi)<<8))
				body.i32Store16(1, z80WasmCPUOffSP)
			} else {
				high, low := z80WasmPair8Offsets(pair)
				body.localGet(1)
				body.i32Const(int32(instr.operandHi))
				body.i32Store8(0, high)
				body.localGet(1)
				body.i32Const(int32(instr.operand))
				body.i32Store8(0, low)
			}
		case instr.opcode&0xCF == 0xC5:
			z80WasmEmitLoadStackPair(&body, (instr.opcode>>4)&3)
			z80WasmEmitPushWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
		case instr.opcode&0xCF == 0xC1:
			z80WasmEmitPopWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitStoreStackPair(&body, (instr.opcode>>4)&3)
		case instr.opcode&0xCF == 0x03 || instr.opcode&0xCF == 0x0B:
			z80WasmEmitPairIncDec(&body, (instr.opcode>>4)&0x03, instr.opcode&0xCF == 0x0B)
		case instr.opcode&0xCF == 0x09:
			z80WasmEmitAddHLPair(&body, (instr.opcode>>4)&3)
		case instr.opcode >= 0x40 && instr.opcode <= 0x7F && instr.opcode != 0x76:
			dst, _ := z80WasmInstrRegisterOffset(instr, (instr.opcode>>3)&7)
			if instr.opcode&7 == 6 {
				z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
				body.localGet(1)
				body.localGet(3)
				body.i32Store8(0, dst)
				break
			}
			src, _ := z80WasmInstrRegisterOffset(instr, instr.opcode&7)
			body.localGet(1)
			body.localGet(1)
			body.i32Load8U(0, src)
			body.i32Store8(0, dst)
		case instr.opcode == 0xF9:
			body.localGet(1)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffL)
			body.i32Store8(0, z80WasmCPUOffSP)
			body.localGet(1)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffH)
			body.i32Store8(0, z80WasmCPUOffSP+1)
		case instr.opcode == 0xEB:
			z80WasmSwapByte(&body, z80WasmCPUOffD, z80WasmCPUOffH)
			z80WasmSwapByte(&body, z80WasmCPUOffE, z80WasmCPUOffL)
		case instr.opcode == 0x08:
			z80WasmSwapByte(&body, z80WasmCPUOffA, z80WasmCPUOffA2)
			z80WasmSwapByte(&body, z80WasmCPUOffF, z80WasmCPUOffF2)
		case instr.opcode == 0xD9:
			z80WasmSwapByte(&body, z80WasmCPUOffB, z80WasmCPUOffB2)
			z80WasmSwapByte(&body, z80WasmCPUOffC, z80WasmCPUOffC2)
			z80WasmSwapByte(&body, z80WasmCPUOffD, z80WasmCPUOffD2)
			z80WasmSwapByte(&body, z80WasmCPUOffE, z80WasmCPUOffE2)
			z80WasmSwapByte(&body, z80WasmCPUOffH, z80WasmCPUOffH2)
			z80WasmSwapByte(&body, z80WasmCPUOffL, z80WasmCPUOffL2)
		case instr.opcode == 0x37:
			z80WasmEmitSCF(&body)
		case instr.opcode == 0x3F:
			z80WasmEmitCCF(&body)
		case instr.opcode == 0x2F:
			z80WasmEmitCPL(&body)
		case instr.opcode == 0x27:
			z80WasmEmitDAA(&body)
		case instr.opcode == 0x07 || instr.opcode == 0x0F || instr.opcode == 0x17 || instr.opcode == 0x1F:
			z80WasmEmitRotateA(&body, instr.opcode)
		case instr.prefix == z80JITPrefixNone && (instr.opcode == 0x34 || instr.opcode == 0x35):
			z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			z80WasmEmitMemoryIncDec(&body, instr.opcode == 0x35)
			z80WasmEmitGuardedStore(&body, uint16(pc), uint32(i), cycles, rIncrements, uint32(length), uint32(cost), uint32(rInc), true)
		case instr.opcode&0xC7 == 0x04:
			offset, _ := z80WasmInstrRegisterOffset(instr, (instr.opcode>>3)&7)
			z80WasmEmitIncDecReg(&body, offset, false)
		case instr.opcode&0xC7 == 0x05:
			offset, _ := z80WasmInstrRegisterOffset(instr, (instr.opcode>>3)&7)
			z80WasmEmitIncDecReg(&body, offset, true)
		case instr.opcode >= 0xA0 && instr.opcode <= 0xB7:
			if instr.opcode&7 == 6 {
				z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
				body.localGet(3)
				body.localSet(2)
			} else {
				offset, _ := z80WasmInstrRegisterOffset(instr, instr.opcode&7)
				body.localGet(1)
				body.i32Load8U(0, offset)
				body.localSet(2)
			}
			z80WasmEmitLogicALU(&body, instr.opcode)
		case instr.opcode >= 0x80 && instr.opcode <= 0x87:
			if instr.opcode == 0x86 {
				z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			} else {
				offset, _ := z80WasmInstrRegisterOffset(instr, instr.opcode&7)
				body.localGet(1)
				body.i32Load8U(0, offset)
				body.localSet(3)
			}
			z80WasmEmitAddReg(&body, false)
		case instr.opcode == 0xC6:
			body.i32Const(int32(instr.operand))
			body.localSet(3)
			z80WasmEmitAddReg(&body, false)
		case instr.opcode >= 0x88 && instr.opcode <= 0x8F:
			if instr.opcode&7 == 6 {
				z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			} else {
				offset, _ := z80WasmInstrRegisterOffset(instr, instr.opcode&7)
				body.localGet(1)
				body.i32Load8U(0, offset)
				body.localSet(3)
			}
			z80WasmEmitAddReg(&body, true)
		case instr.opcode == 0xCE:
			body.i32Const(int32(instr.operand))
			body.localSet(3)
			z80WasmEmitAddReg(&body, true)
		case instr.opcode >= 0x90 && instr.opcode <= 0x97:
			if instr.opcode&7 == 6 {
				z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			} else {
				offset, _ := z80WasmInstrRegisterOffset(instr, instr.opcode&7)
				body.localGet(1)
				body.i32Load8U(0, offset)
				body.localSet(3)
			}
			z80WasmEmitSubReg(&body, false)
		case instr.opcode == 0xD6:
			body.i32Const(int32(instr.operand))
			body.localSet(3)
			z80WasmEmitSubReg(&body, false)
		case instr.opcode >= 0x98 && instr.opcode <= 0x9F:
			if instr.opcode&7 == 6 {
				z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			} else {
				offset, _ := z80WasmInstrRegisterOffset(instr, instr.opcode&7)
				body.localGet(1)
				body.i32Load8U(0, offset)
				body.localSet(3)
			}
			z80WasmEmitSubReg(&body, true)
		case instr.opcode == 0xDE:
			body.i32Const(int32(instr.operand))
			body.localSet(3)
			z80WasmEmitSubReg(&body, true)
		case instr.opcode >= 0xB8 && instr.opcode <= 0xBF:
			if instr.opcode&7 == 6 {
				z80WasmEmitGuardedLoadHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
			} else {
				offset, _ := z80WasmInstrRegisterOffset(instr, instr.opcode&7)
				body.localGet(1)
				body.i32Load8U(0, offset)
				body.localSet(3)
			}
			z80WasmEmitSubReg(&body, false)
			body.localGet(1)
			body.localGet(2)
			body.i32Store8(0, z80WasmCPUOffA)
		case instr.opcode == 0xFE:
			body.i32Const(int32(instr.operand))
			body.localSet(3)
			z80WasmEmitSubReg(&body, false)
			body.localGet(1)
			body.localGet(2)
			body.i32Store8(0, z80WasmCPUOffA)
		case instr.opcode == 0xE6 || instr.opcode == 0xEE || instr.opcode == 0xF6:
			body.i32Const(int32(instr.operand))
			body.localSet(2)
			z80WasmEmitLogicALU(&body, instr.opcode)
		case instr.opcode == 0xE9:
			if i != len(instrs)-1 {
				return nil, fmt.Errorf("z80 wasm: non-terminal JP (HL)")
			}
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffH)
			body.i32Const(8)
			body.op(wasmOpI32Shl)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffL)
			body.op(wasmOpI32Or)
			body.localSet(4)
			body.localGet(1)
			body.localGet(4)
			body.i32Store16(1, z80WasmCPUOffWZ)
			dynamicPC = true
		case instr.opcode == 0xF3: // DI
			body.localGet(1)
			body.i32Const(0)
			body.i32Store8(0, z80WasmCPUOffIFF1)
			body.localGet(1)
			body.i32Const(0)
			body.i32Store8(0, z80WasmCPUOffIFF2)
			body.localGet(0)
			body.i32Const(0)
			body.i32Store(2, z80WasmCtxOffIFFDelay)
		case instr.opcode == 0xFB: // EI
			body.localGet(0)
			// CPU_Z80.finishInstruction consumes the first half of EI's
			// two-instruction delay before Step returns.
			body.i32Const(1)
			body.i32Store(2, z80WasmCtxOffIFFDelay)
		case instr.opcode == 0xE3:
			z80WasmEmitEXSPHL(&body, uint16(pc), uint32(i), cycles, rIncrements)
		case instr.opcode == 0x20 || instr.opcode == 0x28 || instr.opcode == 0x30 || instr.opcode == 0x38:
			nextPC := uint16(pc) + uint16(length)
			target := nextPC + uint16(int16(int8(instr.operand)))
			z80WasmEmitCondition(&body, (instr.opcode>>3)&3)
			body.ifVoid()
			body.i32Const(int32(target))
			body.localSet(4)
			body.i32Const(5)
			body.localSet(7)
			body.elseBranch()
			body.i32Const(int32(nextPC))
			body.localSet(4)
			body.i32Const(0)
			body.localSet(7)
			body.end()
			dynamicPC, dynamicCycles = true, true
		case instr.opcode == 0x10:
			nextPC := uint16(pc) + uint16(length)
			target := nextPC + uint16(int16(int8(instr.operand)))
			body.localGet(1)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffB)
			body.i32Const(1)
			body.op(wasmOpI32Sub)
			body.i32Store8(0, z80WasmCPUOffB)
			body.localGet(1)
			body.i32Load8U(0, z80WasmCPUOffB)
			body.ifVoid()
			body.i32Const(int32(target))
			body.localSet(4)
			body.i32Const(5)
			body.localSet(7)
			body.elseBranch()
			body.i32Const(int32(nextPC))
			body.localSet(4)
			body.i32Const(0)
			body.localSet(7)
			body.end()
			dynamicPC, dynamicCycles = true, true
		case instr.opcode&0xC7 == 0xC2:
			nextPC := uint16(pc) + uint16(length)
			target := uint16(instr.operand) | uint16(instr.operandHi)<<8
			z80WasmEmitCondition(&body, (instr.opcode>>3)&7)
			body.ifVoid()
			body.i32Const(int32(target))
			body.localSet(4)
			body.elseBranch()
			body.i32Const(int32(nextPC))
			body.localSet(4)
			body.end()
			dynamicPC = true
		case instr.opcode == 0xCD:
			nextPC := uint16(pc) + uint16(length)
			body.i32Const(int32(nextPC))
			body.localSet(3)
			z80WasmEmitPushWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.i32Const(int32(uint16(instr.operand) | uint16(instr.operandHi)<<8))
			body.localSet(4)
			dynamicPC = true
		case instr.opcode == 0xC9:
			z80WasmEmitPopWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(3)
			body.localSet(4)
			dynamicPC = true
		case instr.opcode&0xC7 == 0xC4:
			nextPC := uint16(pc) + uint16(length)
			z80WasmEmitCondition(&body, (instr.opcode>>3)&7)
			body.ifVoid()
			body.i32Const(int32(nextPC))
			body.localSet(3)
			z80WasmEmitPushWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.i32Const(int32(uint16(instr.operand) | uint16(instr.operandHi)<<8))
			body.localSet(4)
			body.i32Const(7)
			body.localSet(7)
			body.elseBranch()
			body.i32Const(int32(nextPC))
			body.localSet(4)
			body.i32Const(0)
			body.localSet(7)
			body.end()
			dynamicPC, dynamicCycles = true, true
		case instr.opcode&0xC7 == 0xC0:
			nextPC := uint16(pc) + uint16(length)
			z80WasmEmitCondition(&body, (instr.opcode>>3)&7)
			body.ifVoid()
			z80WasmEmitPopWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.localGet(3)
			body.localSet(4)
			body.i32Const(6)
			body.localSet(7)
			body.elseBranch()
			body.i32Const(int32(nextPC))
			body.localSet(4)
			body.i32Const(0)
			body.localSet(7)
			body.end()
			dynamicPC, dynamicCycles = true, true
		case instr.opcode&0xC7 == 0xC7:
			nextPC := uint16(pc) + uint16(length)
			body.i32Const(int32(nextPC))
			body.localSet(3)
			z80WasmEmitPushWord(&body, uint16(pc), uint32(i), cycles, rIncrements)
			body.i32Const(int32(instr.opcode & 0x38))
			body.localSet(4)
			dynamicPC = true
		}
		pc += uint32(length)
		cycles += uint32(cost)
		rIncrements += uint32(rInc)
		if !instr.indexedCB && (instr.prefix == 0 || instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0xC3 {
			pc = uint32(instr.operand) | uint32(instr.operandHi)<<8
			break
		}
		if instr.opcode == 0x18 {
			pc = uint32(uint16(uint16(pc) + uint16(int16(int8(instr.operand)))))
			break
		}
	}
	body.localGet(0)
	if dynamicPC {
		body.localGet(4)
	} else {
		body.i32Const(int32(pc))
	}
	body.i32Store(2, z80WasmCtxOffRetPC)
	body.localGet(0)
	body.i64Const(int64(cycles))
	if dynamicCycles {
		body.localGet(7)
		body.op(wasmOpI64ExtendI32U)
		body.op(wasmOpI64Add)
	}
	body.i64Store(3, z80WasmCtxOffRetCycles)
	body.localGet(0)
	body.i32Const(int32(len(instrs)))
	body.i32Store(2, z80WasmCtxOffRetCount)
	body.localGet(0)
	body.i32Const(int32(rIncrements))
	body.i32Store(2, z80WasmCtxOffRIncrements)
	body.end()

	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	typeIdx := m.addType([]byte{wasmTypeI32}, nil)
	fn := m.addFunc(typeIdx, []byte{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32}, body.code)
	m.exportFunc("block", fn)
	return m.build(), nil
}

func z80EDInterruptMode(opcode byte) int32 {
	switch opcode {
	case 0x46, 0x4E, 0x66, 0x6E:
		return 0
	case 0x56, 0x76:
		return 1
	case 0x5E, 0x7E:
		return 2
	}
	return -1
}

func z80EDNEGOpcode(opcode byte) bool {
	switch opcode {
	case 0x44, 0x4C, 0x54, 0x5C, 0x64, 0x6C, 0x74, 0x7C:
		return true
	}
	return false
}

// z80WasmEmitGuardedLoadHL loads (HL) only after the shared direct-page bitmap has
// admitted its page.  A rejected page returns at this instruction boundary:
// preceding instructions remain retired, while the dispatcher invokes the
// frozen canonical helper before any non-direct memory is observed.
func z80WasmEmitGuardedLoadHL(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	z80WasmEmitGuardedLoadPair(body, instrPC, retired, priorCycles, priorRIncrements, z80WasmCPUOffH, z80WasmCPUOffL)
}

// z80WasmEmitGuardedStoreHL commits a direct RAM write, then exits only if
// that page holds JIT source so the dispatcher can invalidate stale modules.
func z80WasmEmitGuardedStore(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements, length, cycles, rIncrements uint32, addressSet bool) {
	if !addressSet {
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffH)
		body.i32Const(8)
		body.op(wasmOpI32Shl)
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffL)
		body.op(wasmOpI32Or)
		body.localSet(6)
	}
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffDirectPageBitmap)
	body.localGet(6)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.op(wasmOpI32Add)
	body.i32Load8U(0, 0)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffMemPtr)
	body.localGet(6)
	body.op(wasmOpI32Add)
	body.localGet(3)
	body.i32Store8(0, 0)
	// Page is local 5 while checking code-page membership.
	body.localGet(6)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.localSet(5)
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffCodePageBitmap)
	body.localGet(5)
	body.op(wasmOpI32Add)
	body.i32Load8U(0, 0)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.elseBranch()
	body.localGet(0)
	body.i32Const(1)
	body.i32Store(2, z80WasmCtxOffNeedInval)
	body.localGet(0)
	body.localGet(5)
	body.i32Store(2, z80WasmCtxOffInvalPage)
	body.localGet(0)
	body.i32Const(int32(instrPC) + int32(length))
	body.i32Store(2, z80WasmCtxOffRetPC)
	body.localGet(0)
	body.i64Const(int64(priorCycles + cycles))
	body.i64Store(3, z80WasmCtxOffRetCycles)
	body.localGet(0)
	body.i32Const(int32(retired + 1))
	body.i32Store(2, z80WasmCtxOffRetCount)
	body.localGet(0)
	body.i32Const(int32(priorRIncrements + rIncrements))
	body.i32Store(2, z80WasmCtxOffRIncrements)
	body.op(wasmOpReturn)
	body.end()
	body.elseBranch()
	body.localGet(0)
	body.i32Const(1)
	body.i32Store(2, z80WasmCtxOffNeedBail)
	body.localGet(0)
	body.i32Const(int32(instrPC))
	body.i32Store(2, z80WasmCtxOffRetPC)
	body.localGet(0)
	body.i64Const(int64(priorCycles))
	body.i64Store(3, z80WasmCtxOffRetCycles)
	body.localGet(0)
	body.i32Const(int32(retired))
	body.i32Store(2, z80WasmCtxOffRetCount)
	body.localGet(0)
	body.i32Const(int32(priorRIncrements))
	body.i32Store(2, z80WasmCtxOffRIncrements)
	body.op(wasmOpReturn)
	body.end()
}

// z80WasmEmitGuardedLoadPair leaves the admitted byte in local 3.
func z80WasmEmitGuardedLoadPair(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements, high, low uint32) {
	// local 6 = HL.
	body.localGet(1)
	body.i32Load8U(0, high)
	body.i32Const(8)
	body.op(wasmOpI32Shl)
	body.localGet(1)
	body.i32Load8U(0, low)
	body.op(wasmOpI32Or)
	body.localSet(6)

	z80WasmEmitGuardedLoadAddress(body, instrPC, retired, priorCycles, priorRIncrements)
}

// z80WasmEmitGuardedLoadAddress admits the address in local 6 and leaves its
// byte in local 3. It never observes a rejected page.
func z80WasmEmitGuardedLoadAddress(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	// bitmap[address>>8] == 0 is the direct-memory admission contract.
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffDirectPageBitmap)
	body.localGet(6)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.op(wasmOpI32Add)
	body.i32Load8U(0, 0)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	// Direct page: MemPtr + HL supplies the operand.
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffMemPtr)
	body.localGet(6)
	body.op(wasmOpI32Add)
	body.i32Load8U(0, 0)
	body.localSet(3)
	body.elseBranch()
	// Non-direct page: publish the completed prefix accounting and return
	// without reading the memory page.
	body.localGet(0)
	body.i32Const(1)
	body.i32Store(2, z80WasmCtxOffNeedBail)
	body.localGet(0)
	body.i32Const(int32(instrPC))
	body.i32Store(2, z80WasmCtxOffRetPC)
	body.localGet(0)
	body.i64Const(int64(priorCycles))
	body.i64Store(3, z80WasmCtxOffRetCycles)
	body.localGet(0)
	body.i32Const(int32(retired))
	body.i32Store(2, z80WasmCtxOffRetCount)
	body.localGet(0)
	body.i32Const(int32(priorRIncrements))
	body.i32Store(2, z80WasmCtxOffRIncrements)
	body.op(wasmOpReturn)
	body.end()
}

// z80WasmEmitGuardStackAddress requires local 6 to name ordinary direct RAM
// that is not compiled code. Stack writes bail before either byte is committed
// when this guard fails, preserving atomic word semantics.
func z80WasmEmitGuardStackAddress(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffDirectPageBitmap)
	body.localGet(6)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.op(wasmOpI32Add)
	body.i32Load8U(0, 0)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffCodePageBitmap)
	body.localGet(6)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.op(wasmOpI32Add)
	body.i32Load8U(0, 0)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.elseBranch()
	z80WasmEmitBailReturn(body, instrPC, retired, priorCycles, priorRIncrements)
	body.end()
	body.elseBranch()
	z80WasmEmitBailReturn(body, instrPC, retired, priorCycles, priorRIncrements)
	body.end()
}

func z80WasmEmitGuardDirectAddress(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffDirectPageBitmap)
	body.localGet(6)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.op(wasmOpI32Add)
	body.i32Load8U(0, 0)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.elseBranch()
	z80WasmEmitBailReturn(body, instrPC, retired, priorCycles, priorRIncrements)
	body.end()
}

func z80WasmEmitBailReturn(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	body.localGet(0)
	body.i32Const(1)
	body.i32Store(2, z80WasmCtxOffNeedBail)
	body.localGet(0)
	body.i32Const(int32(instrPC))
	body.i32Store(2, z80WasmCtxOffRetPC)
	body.localGet(0)
	body.i64Const(int64(priorCycles))
	body.i64Store(3, z80WasmCtxOffRetCycles)
	body.localGet(0)
	body.i32Const(int32(retired))
	body.i32Store(2, z80WasmCtxOffRetCount)
	body.localGet(0)
	body.i32Const(int32(priorRIncrements))
	body.i32Store(2, z80WasmCtxOffRIncrements)
	body.op(wasmOpReturn)
}

func z80WasmEmitLoadStackPair(body *wasmBody, pair byte) {
	if pair == 3 {
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffA)
		body.i32Const(8)
		body.op(wasmOpI32Shl)
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffF)
		body.op(wasmOpI32Or)
	} else {
		high, low := z80WasmPair8Offsets(pair)
		body.localGet(1)
		body.i32Load8U(0, high)
		body.i32Const(8)
		body.op(wasmOpI32Shl)
		body.localGet(1)
		body.i32Load8U(0, low)
		body.op(wasmOpI32Or)
	}
	body.localSet(3)
}

func z80WasmEmitStoreStackPair(body *wasmBody, pair byte) {
	if pair == 3 {
		body.localGet(1)
		body.localGet(3)
		body.i32Const(8)
		body.op(wasmOpI32ShrU)
		body.i32Store8(0, z80WasmCPUOffA)
		body.localGet(1)
		body.localGet(3)
		body.i32Store8(0, z80WasmCPUOffF)
		return
	}
	high, low := z80WasmPair8Offsets(pair)
	body.localGet(1)
	body.localGet(3)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Store8(0, high)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, low)
}

func z80WasmEmitLoadRPPair(body *wasmBody, pair byte) {
	if pair == 3 {
		body.localGet(1)
		body.i32Load16U(1, z80WasmCPUOffSP)
		body.localSet(3)
		return
	}
	z80WasmEmitLoadStackPair(body, pair)
}

func z80WasmEmitStoreRPPair(body *wasmBody, pair byte) {
	if pair == 3 {
		body.localGet(1)
		body.localGet(3)
		body.i32Store16(1, z80WasmCPUOffSP)
		return
	}
	z80WasmEmitStoreStackPair(body, pair)
}

// local 3 contains the word to push.
func z80WasmEmitPushWord(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	body.localGet(1)
	body.i32Load16U(1, z80WasmCPUOffSP)
	body.i32Const(1)
	body.op(wasmOpI32Sub)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(4) // high-byte address
	body.localGet(4)
	body.i32Const(1)
	body.op(wasmOpI32Sub)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(5) // low-byte address / resulting SP
	body.localGet(4)
	body.localSet(6)
	z80WasmEmitGuardStackAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(5)
	body.localSet(6)
	z80WasmEmitGuardStackAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffMemPtr)
	body.localGet(4)
	body.op(wasmOpI32Add)
	body.localGet(3)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Store8(0, 0)
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffMemPtr)
	body.localGet(5)
	body.op(wasmOpI32Add)
	body.localGet(3)
	body.i32Store8(0, 0)
	body.localGet(1)
	body.localGet(5)
	body.i32Store16(1, z80WasmCPUOffSP)
}

// z80WasmEmitPopWord leaves the popped word in local 3.
func z80WasmEmitPopWord(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	body.localGet(1)
	body.i32Load16U(1, z80WasmCPUOffSP)
	body.localSet(6)
	z80WasmEmitGuardedLoadAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(3)
	body.localSet(4)
	body.localGet(1)
	body.i32Load16U(1, z80WasmCPUOffSP)
	body.i32Const(1)
	body.op(wasmOpI32Add)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(6)
	z80WasmEmitGuardedLoadAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(3)
	body.i32Const(8)
	body.op(wasmOpI32Shl)
	body.localGet(4)
	body.op(wasmOpI32Or)
	body.localSet(3)
	body.localGet(1)
	body.localGet(1)
	body.i32Load16U(1, z80WasmCPUOffSP)
	body.i32Const(2)
	body.op(wasmOpI32Add)
	body.i32Store16(1, z80WasmCPUOffSP)
}

// z80WasmEmitLoadWordAddress leaves the little-endian word in local 3 and
// performs no architectural mutation until both direct-page guards pass.
func z80WasmEmitLoadWordAddress(body *wasmBody, address, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	body.i32Const(int32(address))
	body.localSet(6)
	z80WasmEmitGuardedLoadAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(3)
	body.localSet(4)
	body.i32Const(int32(address + 1))
	body.localSet(6)
	z80WasmEmitGuardedLoadAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(3)
	body.i32Const(8)
	body.op(wasmOpI32Shl)
	body.localGet(4)
	body.op(wasmOpI32Or)
	body.localSet(3)
}

// local 3 contains the word. Both destination bytes are admitted before the
// first write, including a code-page rejection that delegates SMC publication
// to the canonical helper.
func z80WasmEmitStoreWordAddress(body *wasmBody, address, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	body.i32Const(int32(address))
	body.localSet(6)
	z80WasmEmitGuardStackAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.i32Const(int32(address + 1))
	body.localSet(6)
	z80WasmEmitGuardStackAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffMemPtr)
	body.i32Const(int32(address))
	body.op(wasmOpI32Add)
	body.localGet(3)
	body.i32Store8(0, 0)
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffMemPtr)
	body.i32Const(int32(address + 1))
	body.op(wasmOpI32Add)
	body.localGet(3)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Store8(0, 0)
}

func z80WasmEmitEXSPHL(body *wasmBody, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	z80WasmEmitEXSP(body, 0, instrPC, retired, priorCycles, priorRIncrements)
}

func z80WasmEmitEXSPIndex(body *wasmBody, prefix byte, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	z80WasmEmitEXSP(body, prefix, instrPC, retired, priorCycles, priorRIncrements)
}

func z80WasmEmitEXSP(body *wasmBody, prefix byte, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	body.localGet(1)
	body.i32Load16U(1, z80WasmCPUOffSP)
	body.localSet(5) // address
	body.localGet(5)
	body.localSet(6)
	z80WasmEmitGuardedLoadAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(3)
	body.localSet(4) // memory low
	body.localGet(5)
	body.i32Const(1)
	body.op(wasmOpI32Add)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(6)
	z80WasmEmitGuardedLoadAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(3)
	body.i32Const(8)
	body.op(wasmOpI32Shl)
	body.localGet(4)
	body.op(wasmOpI32Or)
	body.localSet(7) // memory word
	// Reject either non-direct or code destination before the first write.
	body.localGet(5)
	body.localSet(6)
	z80WasmEmitGuardStackAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(5)
	body.i32Const(1)
	body.op(wasmOpI32Add)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(6)
	z80WasmEmitGuardStackAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	// local 3 becomes the old HL/IX/IY word.
	if prefix == 0 {
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffH)
		body.i32Const(8)
		body.op(wasmOpI32Shl)
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffL)
		body.op(wasmOpI32Or)
	} else {
		body.localGet(1)
		body.i32Load16U(1, z80WasmIndexOffset(prefix))
	}
	body.localSet(3)
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffMemPtr)
	body.localGet(5)
	body.op(wasmOpI32Add)
	body.localGet(3)
	body.i32Store8(0, 0)
	body.localGet(0)
	body.i32Load(2, z80WasmCtxOffMemPtr)
	body.localGet(5)
	body.i32Const(1)
	body.op(wasmOpI32Add)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Add)
	body.localGet(3)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Store8(0, 0)
	if prefix == 0 {
		body.localGet(1)
		body.localGet(7)
		body.i32Store8(0, z80WasmCPUOffL)
		body.localGet(1)
		body.localGet(7)
		body.i32Const(8)
		body.op(wasmOpI32ShrU)
		body.i32Store8(0, z80WasmCPUOffH)
	} else {
		body.localGet(1)
		body.localGet(7)
		body.i32Store16(1, z80WasmIndexOffset(prefix))
	}
	body.localGet(1)
	body.localGet(7)
	body.i32Store16(1, z80WasmCPUOffWZ)
}

func z80WasmEmitPairWordToLocal(body *wasmBody, pair byte, local uint32) {
	high, low := z80WasmPair8Offsets(pair)
	body.localGet(1)
	body.i32Load8U(0, high)
	body.i32Const(8)
	body.op(wasmOpI32Shl)
	body.localGet(1)
	body.i32Load8U(0, low)
	body.op(wasmOpI32Or)
	body.localSet(local)
}

func z80WasmEmitLocalToPairWord(body *wasmBody, local uint32, pair byte) {
	high, low := z80WasmPair8Offsets(pair)
	body.localGet(1)
	body.localGet(local)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Store8(0, high)
	body.localGet(1)
	body.localGet(local)
	body.i32Store8(0, low)
}

func z80WasmEmitBlockTransfer(body *wasmBody, decrement bool, instrPC uint16, retired, priorCycles, priorRIncrements, length, cycles, rIncrements uint32) {
	z80WasmEmitGuardedLoadHL(body, instrPC, retired, priorCycles, priorRIncrements)
	body.localGet(3)
	body.localSet(2)                       // transferred byte
	z80WasmEmitPairWordToLocal(body, 1, 5) // DE
	body.localGet(5)
	body.localSet(6)
	z80WasmEmitGuardDirectAddress(body, instrPC, retired, priorCycles, priorRIncrements)
	z80WasmEmitPairWordToLocal(body, 2, 4) // HL
	z80WasmEmitPairWordToLocal(body, 0, 7) // BC
	body.localGet(4)
	body.i32Const(1)
	if decrement {
		body.op(wasmOpI32Sub)
	} else {
		body.op(wasmOpI32Add)
	}
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(4)
	body.localGet(5)
	body.i32Const(1)
	if decrement {
		body.op(wasmOpI32Sub)
	} else {
		body.op(wasmOpI32Add)
	}
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(5)
	body.localGet(7)
	body.i32Const(1)
	body.op(wasmOpI32Sub)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(7)
	z80WasmEmitLocalToPairWord(body, 4, 2)
	z80WasmEmitLocalToPairWord(body, 5, 1)
	z80WasmEmitLocalToPairWord(body, 7, 0)
	// Preserve S/Z/C, publish PV from BC and X/Y from A+transferred byte.
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagS | z80FlagZ | z80FlagC)
	body.op(wasmOpI32And)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localGet(2)
	body.op(wasmOpI32Add)
	body.i32Const(z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.localGet(7)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffF)
	body.localGet(5) // DE was already advanced; recover destination.
	body.i32Const(1)
	if decrement {
		body.op(wasmOpI32Add)
	} else {
		body.op(wasmOpI32Sub)
	}
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(6)
	body.localGet(2)
	body.localSet(3)
	z80WasmEmitGuardedStore(body, instrPC, retired, priorCycles, priorRIncrements, length, cycles, rIncrements, true)
}

func z80WasmEmitBlockCompare(body *wasmBody, decrement bool, instrPC uint16, retired, priorCycles, priorRIncrements uint32) {
	z80WasmEmitGuardedLoadHL(body, instrPC, retired, priorCycles, priorRIncrements)
	z80WasmEmitSubReg(body, false)
	body.localGet(1)
	body.localGet(2) // restore A after CP-style flag calculation
	body.i32Store8(0, z80WasmCPUOffA)
	z80WasmEmitPairWordToLocal(body, 2, 4)
	body.localGet(4)
	body.i32Const(1)
	if decrement {
		body.op(wasmOpI32Sub)
	} else {
		body.op(wasmOpI32Add)
	}
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(4)
	z80WasmEmitLocalToPairWord(body, 4, 2)
	z80WasmEmitPairWordToLocal(body, 0, 5)
	body.localGet(5)
	body.i32Const(1)
	body.op(wasmOpI32Sub)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(5)
	z80WasmEmitLocalToPairWord(body, 5, 0)
	body.localGet(1)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(0xFF ^ z80FlagPV)
	body.op(wasmOpI32And)
	body.localSet(4)
	body.localGet(5)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitPairIncDec(body *wasmBody, pair byte, decrement bool) {
	offset := uint32(z80WasmCPUOffSP)
	needsSwap := false
	if pair != 3 {
		offset, _ = z80WasmPair8Offsets(pair)
		needsSwap = true
	}
	body.localGet(1)
	body.i32Load16U(1, offset)
	if needsSwap {
		// CPU byte pairs are high,low while wasm halfword loads are little endian.
		body.localTee(2)
		body.i32Const(0xFF)
		body.op(wasmOpI32And)
		body.i32Const(8)
		body.op(wasmOpI32Shl)
		body.localGet(2)
		body.i32Const(8)
		body.op(wasmOpI32ShrU)
		body.i32Const(0xFF)
		body.op(wasmOpI32And)
		body.op(wasmOpI32Or)
	}
	body.i32Const(1)
	if decrement {
		body.op(wasmOpI32Sub)
	} else {
		body.op(wasmOpI32Add)
	}
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	if needsSwap {
		body.localTee(2)
		body.i32Const(0xFF)
		body.op(wasmOpI32And)
		body.i32Const(8)
		body.op(wasmOpI32Shl)
		body.localGet(2)
		body.i32Const(8)
		body.op(wasmOpI32ShrU)
		body.i32Const(0xFF)
		body.op(wasmOpI32And)
		body.op(wasmOpI32Or)
	}
	// Stores take address before value; retain the transformed pair while
	// materialising the CPU pointer.
	body.localSet(2)
	body.localGet(1)
	body.localGet(2)
	body.i32Store16(1, offset)
}

func z80WasmLoadPair16(body *wasmBody, pair byte, dst uint32) {
	if pair == 3 {
		body.localGet(1)
		body.i32Load16U(1, z80WasmCPUOffSP)
		body.localSet(dst)
		return
	}
	high, low := z80WasmPair8Offsets(pair)
	body.localGet(1)
	body.i32Load8U(0, high)
	body.i32Const(8)
	body.op(wasmOpI32Shl)
	body.localGet(1)
	body.i32Load8U(0, low)
	body.op(wasmOpI32Or)
	body.localSet(dst)
}

func z80WasmEmitAddHLPair(body *wasmBody, pair byte) {
	// locals: 2 old HL, 3 source, 4 full sum, 5 flags.
	z80WasmLoadPair16(body, 2, 2)
	z80WasmLoadPair16(body, pair, 3)
	body.localGet(2)
	body.localGet(3)
	body.op(wasmOpI32Add)
	body.localSet(4)
	// H and L take the low 16 bits of the sum.
	body.localGet(1)
	body.localGet(4)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Store8(0, z80WasmCPUOffH)
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffL)
	// Preserve S, Z, P/V then add X/Y from result high byte.
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagS | z80FlagZ | z80FlagPV)
	body.op(wasmOpI32And)
	body.localGet(4)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Const(z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.localSet(5)
	// Half carry from bit 11.
	body.localGet(2)
	body.i32Const(0x0FFF)
	body.op(wasmOpI32And)
	body.localGet(3)
	body.i32Const(0x0FFF)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Add)
	body.i32Const(12)
	body.op(wasmOpI32ShrU)
	body.i32Const(1)
	body.op(wasmOpI32And)
	body.i32Const(4)
	body.op(wasmOpI32Shl)
	body.localGet(5)
	body.op(wasmOpI32Or)
	body.localSet(5)
	// Carry from bit 15.
	body.localGet(4)
	body.i32Const(16)
	body.op(wasmOpI32ShrU)
	body.localGet(5)
	body.op(wasmOpI32Or)
	body.localSet(5)
	body.localGet(1)
	body.localGet(5)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitADCSBCHL(body *wasmBody, pair byte, subtract bool) {
	// local 2 = HL, local 3 = rhs, local 4 = carry, local 5 = full result.
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffH)
	body.i32Const(8)
	body.op(wasmOpI32Shl)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffL)
	body.op(wasmOpI32Or)
	body.localSet(2)
	if pair == 3 {
		body.localGet(1)
		body.i32Load16U(1, z80WasmCPUOffSP)
	} else {
		high, low := z80WasmPair8Offsets(pair)
		body.localGet(1)
		body.i32Load8U(0, high)
		body.i32Const(8)
		body.op(wasmOpI32Shl)
		body.localGet(1)
		body.i32Load8U(0, low)
		body.op(wasmOpI32Or)
	}
	body.localSet(3)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	body.localSet(4)
	body.localGet(2)
	body.localGet(3)
	if subtract {
		body.op(wasmOpI32Sub)
		body.localGet(4)
		body.op(wasmOpI32Sub)
	} else {
		body.op(wasmOpI32Add)
		body.localGet(4)
		body.op(wasmOpI32Add)
	}
	body.localSet(5)

	body.localGet(5)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(7)
	body.localGet(7)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Const(z80FlagS | z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	if subtract {
		body.i32Const(z80FlagN)
		body.op(wasmOpI32Or)
	}
	body.localSet(6)
	body.localGet(7)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(6)
	body.i32Const(z80FlagZ)
	body.op(wasmOpI32Or)
	body.localSet(6)
	body.end()
	// Half carry/borrow.
	body.localGet(2)
	body.i32Const(0x0FFF)
	body.op(wasmOpI32And)
	body.localGet(3)
	body.i32Const(0x0FFF)
	body.op(wasmOpI32And)
	if subtract {
		body.localGet(4)
		body.op(wasmOpI32Add)
		body.op(wasmOpI32LtU)
	} else {
		body.op(wasmOpI32Add)
		body.localGet(4)
		body.op(wasmOpI32Add)
		body.i32Const(0x0FFF)
		body.op(wasmOpI32GtU)
	}
	body.ifVoid()
	body.localGet(6)
	body.i32Const(z80FlagH)
	body.op(wasmOpI32Or)
	body.localSet(6)
	body.end()
	// Signed overflow.
	body.localGet(2)
	body.localGet(3)
	body.op(wasmOpI32Xor)
	if !subtract {
		body.i32Const(0xFFFF)
		body.op(wasmOpI32Xor)
	}
	body.localGet(2)
	body.localGet(7)
	body.op(wasmOpI32Xor)
	body.op(wasmOpI32And)
	body.i32Const(0x8000)
	body.op(wasmOpI32And)
	body.ifVoid()
	body.localGet(6)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(6)
	body.end()
	// Carry/borrow.
	if subtract {
		body.localGet(2)
		body.localGet(3)
		body.localGet(4)
		body.op(wasmOpI32Add)
		body.op(wasmOpI32LtU)
	} else {
		body.localGet(5)
		body.i32Const(0xFFFF)
		body.op(wasmOpI32GtU)
	}
	body.ifVoid()
	body.localGet(6)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32Or)
	body.localSet(6)
	body.end()
	body.localGet(1)
	body.localGet(7)
	body.i32Store8(0, z80WasmCPUOffL)
	body.localGet(1)
	body.localGet(7)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Store8(0, z80WasmCPUOffH)
	body.localGet(1)
	body.localGet(6)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmSwapByte(body *wasmBody, first, second uint32) {
	body.localGet(1)
	body.i32Load8U(0, first)
	body.localSet(2)
	body.localGet(1)
	body.localGet(1)
	body.i32Load8U(0, second)
	body.i32Store8(0, first)
	body.localGet(1)
	body.localGet(2)
	body.i32Store8(0, second)
}

func z80WasmEmitSCF(body *wasmBody) {
	body.localGet(1)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagS | z80FlagZ | z80FlagPV)
	body.op(wasmOpI32And)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.i32Const(z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32Or)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitCCF(body *wasmBody) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	body.localSet(2)
	body.localGet(1)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagS | z80FlagZ | z80FlagPV)
	body.op(wasmOpI32And)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.i32Const(z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.localGet(2)
	body.i32Const(15)
	body.op(wasmOpI32Mul)
	body.i32Const(1)
	body.op(wasmOpI32Add)
	body.op(wasmOpI32Or)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitCPL(body *wasmBody) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.i32Const(0xFF)
	body.op(wasmOpI32Xor)
	body.localSet(2)
	body.localGet(1)
	body.localGet(2)
	body.i32Store8(0, z80WasmCPUOffA)
	body.localGet(1)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagS | z80FlagZ | z80FlagPV | z80FlagC)
	body.op(wasmOpI32And)
	body.i32Const(z80FlagH | z80FlagN)
	body.op(wasmOpI32Or)
	body.localGet(2)
	body.i32Const(z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitDAA(body *wasmBody) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localSet(2) // original A
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.localSet(3) // original F
	body.i32Const(0)
	body.localSet(4) // adjustment

	// Low decimal correction: H || (!N && low nibble > 9).
	body.localGet(3)
	body.i32Const(z80FlagH)
	body.op(wasmOpI32And)
	body.ifVoid()
	body.i32Const(6)
	body.localSet(4)
	body.elseBranch()
	body.localGet(3)
	body.i32Const(z80FlagN)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(2)
	body.i32Const(0x0F)
	body.op(wasmOpI32And)
	body.i32Const(9)
	body.op(wasmOpI32GtU)
	body.ifVoid()
	body.i32Const(6)
	body.localSet(4)
	body.end()
	body.end()
	body.end()

	// High decimal correction: C || (!N && A > $99).
	body.localGet(3)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(0x60)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.elseBranch()
	body.localGet(3)
	body.i32Const(z80FlagN)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(2)
	body.i32Const(0x99)
	body.op(wasmOpI32GtU)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(0x60)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	body.end()
	body.end()

	body.localGet(4)
	body.i32Const(0x60)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.op(wasmOpI32Eqz)
	body.localSet(6) // carry result
	body.localGet(3)
	body.i32Const(z80FlagN)
	body.op(wasmOpI32And)
	body.ifTyped(wasmTypeI32)
	body.localGet(2)
	body.localGet(4)
	body.op(wasmOpI32Sub)
	body.elseBranch()
	body.localGet(2)
	body.localGet(4)
	body.op(wasmOpI32Add)
	body.end()
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(5) // result
	body.localGet(1)
	body.localGet(5)
	body.i32Store8(0, z80WasmCPUOffA)

	body.localGet(3)
	body.i32Const(z80FlagN)
	body.op(wasmOpI32And)
	body.localGet(5)
	body.i32Const(z80FlagS | z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.localSet(7)
	body.localGet(5)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(7)
	body.i32Const(z80FlagZ)
	body.op(wasmOpI32Or)
	body.localSet(7)
	body.end()

	// Even parity.
	body.localGet(5)
	body.localSet(6)
	for _, shift := range []int32{4, 2, 1} {
		body.localGet(6)
		body.localGet(6)
		body.i32Const(shift)
		body.op(wasmOpI32ShrU)
		body.op(wasmOpI32Xor)
		body.localSet(6)
	}
	body.localGet(6)
	body.i32Const(1)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(7)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(7)
	body.end()

	// H follows the interpreter's add/sub-specific correction rule.
	body.localGet(3)
	body.i32Const(z80FlagN)
	body.op(wasmOpI32And)
	body.ifVoid()
	body.localGet(2)
	body.localGet(5)
	body.op(wasmOpI32Xor)
	body.i32Const(0x10)
	body.op(wasmOpI32And)
	body.ifVoid()
	body.localGet(7)
	body.i32Const(z80FlagH)
	body.op(wasmOpI32Or)
	body.localSet(7)
	body.end()
	body.elseBranch()
	body.localGet(2)
	body.i32Const(0x0F)
	body.op(wasmOpI32And)
	body.localGet(4)
	body.i32Const(0x0F)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Add)
	body.i32Const(0x0F)
	body.op(wasmOpI32GtU)
	body.ifVoid()
	body.localGet(7)
	body.i32Const(z80FlagH)
	body.op(wasmOpI32Or)
	body.localSet(7)
	body.end()
	body.end()

	body.localGet(4)
	body.i32Const(0x60)
	body.op(wasmOpI32And)
	body.ifVoid()
	body.localGet(7)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32Or)
	body.localSet(7)
	body.end()
	body.localGet(1)
	body.localGet(7)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitRotateA(body *wasmBody, opcode byte) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localSet(2)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.localSet(3)
	if opcode == 0x07 || opcode == 0x17 {
		body.localGet(2)
		body.i32Const(7)
		body.op(wasmOpI32ShrU)
		body.localSet(4)
		body.localGet(2)
		body.i32Const(1)
		body.op(wasmOpI32Shl)
		if opcode == 0x07 {
			body.localGet(4)
		} else {
			body.localGet(3)
			body.i32Const(z80FlagC)
			body.op(wasmOpI32And)
		}
		body.op(wasmOpI32Or)
	} else {
		body.localGet(2)
		body.i32Const(z80FlagC)
		body.op(wasmOpI32And)
		body.localSet(4)
		body.localGet(2)
		body.i32Const(1)
		body.op(wasmOpI32ShrU)
		if opcode == 0x0F {
			body.localGet(4)
		} else {
			body.localGet(3)
			body.i32Const(z80FlagC)
			body.op(wasmOpI32And)
		}
		body.i32Const(7)
		body.op(wasmOpI32Shl)
		body.op(wasmOpI32Or)
	}
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(2)
	body.localGet(1)
	body.localGet(2)
	body.i32Store8(0, z80WasmCPUOffA)
	body.localGet(3)
	body.i32Const(z80FlagS | z80FlagZ | z80FlagPV)
	body.op(wasmOpI32And)
	body.localGet(2)
	body.i32Const(z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.localGet(4)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitIncDecReg(body *wasmBody, offset uint32, decrement bool) {
	body.localGet(1)
	body.i32Load8U(0, offset)
	body.localSet(2) // old value
	body.localGet(2)
	body.i32Const(1)
	if decrement {
		body.op(wasmOpI32Sub)
	} else {
		body.op(wasmOpI32Add)
	}
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(3) // result
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
	// Start with preserved carry, and N for decrement.
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	if decrement {
		body.i32Const(z80FlagN)
		body.op(wasmOpI32Or)
	}
	body.localSet(4)
	// S, X and Y are result bits.
	body.localGet(4)
	body.localGet(3)
	body.i32Const(z80FlagS | z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.localSet(4)
	// Z
	body.localGet(3)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagZ)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	// H uses the pre-operation low nibble.
	body.localGet(2)
	body.i32Const(0x0F)
	body.op(wasmOpI32And)
	if decrement {
		body.i32Const(0)
	} else {
		body.i32Const(0x0F)
	}
	body.op(wasmOpI32Eq)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagH)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	// P/V is the signed boundary in the pre-operation value.
	body.localGet(2)
	if decrement {
		body.i32Const(0x80)
	} else {
		body.i32Const(0x7F)
	}
	body.op(wasmOpI32Eq)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffF)
}

// z80WasmEmitLogicALU consumes the byte operand in local 2 and writes A/F.
// z80WasmEmitAddReg directly lowers ADD A,r. Local 3 holds the register
// operand; locals 2, 4, 5 and 6 are A, result, flags and full-width sum.
func z80WasmEmitAddReg(body *wasmBody, carry bool) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localSet(2)
	body.localGet(2)
	body.localGet(3)
	body.op(wasmOpI32Add)
	if carry {
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffF)
		body.i32Const(z80FlagC)
		body.op(wasmOpI32And)
		body.op(wasmOpI32Add)
	}
	body.localSet(6)
	body.localGet(6)
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(4)
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffA)

	// S, X and Y are result bits; Z is set for zero.
	body.localGet(4)
	body.i32Const(z80FlagS | z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.localSet(5)
	body.localGet(4)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(5)
	body.i32Const(z80FlagZ)
	body.op(wasmOpI32Or)
	body.localSet(5)
	body.end()
	// H is the carry from bit three.
	body.localGet(2)
	body.i32Const(0x0F)
	body.op(wasmOpI32And)
	body.localGet(3)
	body.i32Const(0x0F)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Add)
	if carry {
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffF)
		body.i32Const(z80FlagC)
		body.op(wasmOpI32And)
		body.op(wasmOpI32Add)
	}
	body.i32Const(4)
	body.op(wasmOpI32ShrU)
	body.i32Const(1)
	body.op(wasmOpI32And)
	body.i32Const(4)
	body.op(wasmOpI32Shl)
	body.localGet(5)
	body.op(wasmOpI32Or)
	body.localSet(5)
	// P/V is signed overflow: ~(A xor r) & (A xor result) & $80.
	body.localGet(2)
	body.localGet(3)
	body.op(wasmOpI32Xor)
	body.i32Const(0xFF)
	body.op(wasmOpI32Xor)
	body.localGet(2)
	body.localGet(4)
	body.op(wasmOpI32Xor)
	body.op(wasmOpI32And)
	body.i32Const(0x80)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.op(wasmOpElse)
	body.localGet(5)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(5)
	body.end()
	// C is the ninth bit of the full sum.
	body.localGet(6)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Const(1)
	body.op(wasmOpI32And)
	body.localGet(5)
	body.op(wasmOpI32Or)
	body.localSet(5)
	body.localGet(1)
	body.localGet(5)
	body.i32Store8(0, z80WasmCPUOffF)
}

// z80WasmEmitSubReg directly lowers SUB A,r. Local 3 holds the register or
// immediate operand; locals 2, 4, 5 and 6 are A, result, flags and scratch.
func z80WasmEmitSubReg(body *wasmBody, carry bool) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localSet(2)
	body.localGet(2)
	body.localGet(3)
	body.op(wasmOpI32Sub)
	if carry {
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffF)
		body.i32Const(z80FlagC)
		body.op(wasmOpI32And)
		body.op(wasmOpI32Sub)
	}
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(4)
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffA)

	// S, X, Y and N are fixed by the result and operation; add Z if needed.
	body.localGet(4)
	body.i32Const(z80FlagS | z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.i32Const(z80FlagN)
	body.op(wasmOpI32Or)
	body.localSet(5)
	body.localGet(4)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(5)
	body.i32Const(z80FlagZ)
	body.op(wasmOpI32Or)
	body.localSet(5)
	body.end()
	// H is a borrow from bit three.
	body.localGet(2)
	body.localGet(3)
	body.op(wasmOpI32Xor)
	body.localGet(4)
	body.op(wasmOpI32Xor)
	body.i32Const(z80FlagH)
	body.op(wasmOpI32And)
	body.localGet(5)
	body.op(wasmOpI32Or)
	body.localSet(5)
	// P/V is signed overflow: (A xor r) & (A xor result) & $80.
	body.localGet(2)
	body.localGet(3)
	body.op(wasmOpI32Xor)
	body.localGet(2)
	body.localGet(4)
	body.op(wasmOpI32Xor)
	body.op(wasmOpI32And)
	body.i32Const(0x80)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.op(wasmOpElse)
	body.localGet(5)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(5)
	body.end()
	// C is an unsigned borrow.
	body.localGet(2)
	body.localGet(3)
	if carry {
		body.localGet(1)
		body.i32Load8U(0, z80WasmCPUOffF)
		body.i32Const(z80FlagC)
		body.op(wasmOpI32And)
		body.op(wasmOpI32Add)
	}
	body.op(wasmOpI32LtU)
	body.ifVoid()
	body.localGet(5)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32Or)
	body.localSet(5)
	body.end()
	body.localGet(1)
	body.localGet(5)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitLogicALU(body *wasmBody, opcode byte) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localGet(2)
	switch opcode {
	case 0xE6, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7:
		body.op(wasmOpI32And)
	case 0xEE, 0xA8, 0xA9, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF:
		body.op(wasmOpI32Xor)
	default:
		body.op(wasmOpI32Or)
	}
	body.localSet(3)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, z80WasmCPUOffA)
	// S, X and Y directly reflect the result.
	body.localGet(3)
	body.i32Const(z80FlagS | z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.localSet(4)
	// Z.
	body.localGet(3)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagZ)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	// Fold to parity; P/V is set for even parity.
	body.localGet(3)
	body.localSet(2)
	for _, shift := range []int32{4, 2, 1} {
		body.localGet(2)
		body.localGet(2)
		body.i32Const(shift)
		body.op(wasmOpI32ShrU)
		body.op(wasmOpI32Xor)
		body.localSet(2)
	}
	body.localGet(2)
	body.i32Const(1)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	if opcode == 0xE6 || opcode >= 0xA0 && opcode <= 0xA7 {
		body.localGet(4)
		body.i32Const(z80FlagH)
		body.op(wasmOpI32Or)
		body.localSet(4)
	}
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffF)
}

// z80WasmEmitCBRLCReg implements CB RLC r. Locals 2..5 are scratch; unlike
// the accumulator rotates it publishes S/Z/PV and X/Y from the result.
func z80WasmEmitCBRLCReg(body *wasmBody, offset uint32) {
	body.localGet(1)
	body.i32Load8U(0, offset)
	body.localSet(2) // old
	body.localGet(2)
	body.i32Const(1)
	body.op(wasmOpI32Shl)
	body.localGet(2)
	body.i32Const(7)
	body.op(wasmOpI32ShrU)
	body.op(wasmOpI32Or)
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(3) // result
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
	body.localGet(3)
	body.i32Const(z80FlagS | z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.localSet(4)
	body.localGet(3)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagZ)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	body.localGet(3)
	body.localSet(5)
	for _, shift := range []int32{4, 2, 1} {
		body.localGet(5)
		body.localGet(5)
		body.i32Const(shift)
		body.op(wasmOpI32ShrU)
		body.op(wasmOpI32Xor)
		body.localSet(5)
	}
	body.localGet(5)
	body.i32Const(1)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	body.localGet(2)
	body.i32Const(7)
	body.op(wasmOpI32ShrU)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	body.localGet(4)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffF)
}

// z80WasmEmitCBMemoryRotate applies a CB rotate/shift to the byte in local 3.
// The register helpers already implement the exact flag rules, so A is used
// as a temporary while local 7 preserves its architectural value. Local 6
// remains the guarded memory address for the following store.
func z80WasmEmitCBMemoryRotate(body *wasmBody, opcode byte) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localSet(7)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, z80WasmCPUOffA)
	switch (opcode >> 3) & 7 {
	case 0:
		z80WasmEmitCBRLCReg(body, z80WasmCPUOffA)
	case 1:
		z80WasmEmitCBRRCReg(body, z80WasmCPUOffA)
	case 2:
		z80WasmEmitCBRLReg(body, z80WasmCPUOffA)
	case 3:
		z80WasmEmitCBRRReg(body, z80WasmCPUOffA)
	case 4:
		z80WasmEmitCBSLAReg(body, z80WasmCPUOffA)
	case 5:
		z80WasmEmitCBSRAReg(body, z80WasmCPUOffA)
	case 6:
		z80WasmEmitCBSLLReg(body, z80WasmCPUOffA)
	case 7:
		z80WasmEmitCBSRLReg(body, z80WasmCPUOffA)
	}
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localSet(3)
	body.localGet(1)
	body.localGet(7)
	body.i32Store8(0, z80WasmCPUOffA)
}

func z80WasmEmitMemoryIncDec(body *wasmBody, decrement bool) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localSet(7)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, z80WasmCPUOffA)
	z80WasmEmitIncDecReg(body, z80WasmCPUOffA, decrement)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localSet(3)
	body.localGet(1)
	body.localGet(7)
	body.i32Store8(0, z80WasmCPUOffA)
}

func z80WasmEmitIndexedAddress(body *wasmBody, prefix byte, displacement int8) {
	offset := z80WasmIndexOffset(prefix)
	body.localGet(1)
	body.i32Load16U(1, offset)
	body.i32Const(int32(displacement))
	body.op(wasmOpI32Add)
	body.i32Const(0xFFFF)
	body.op(wasmOpI32And)
	body.localSet(6)
}

func z80WasmIndexOffset(prefix byte) uint32 {
	if prefix == z80JITPrefixFD {
		return z80WasmCPUOffIY
	}
	return z80WasmCPUOffIX
}

func z80WasmEmitAddIndexPair(body *wasmBody, prefix, pair byte) {
	// Reuse the flag-exact ADD HL,rp implementation with HL as a temporary.
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffH)
	body.localSet(6)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffL)
	body.localSet(7)
	body.localGet(1)
	body.i32Load16U(1, z80WasmIndexOffset(prefix))
	body.localSet(2)
	body.localGet(1)
	body.localGet(2)
	body.i32Store8(0, z80WasmCPUOffL)
	body.localGet(1)
	body.localGet(2)
	body.i32Const(8)
	body.op(wasmOpI32ShrU)
	body.i32Store8(0, z80WasmCPUOffH)
	z80WasmEmitAddHLPair(body, pair)
	body.localGet(1)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffH)
	body.i32Const(8)
	body.op(wasmOpI32Shl)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffL)
	body.op(wasmOpI32Or)
	body.i32Store16(1, z80WasmIndexOffset(prefix))
	body.localGet(1)
	body.localGet(6)
	body.i32Store8(0, z80WasmCPUOffH)
	body.localGet(1)
	body.localGet(7)
	body.i32Store8(0, z80WasmCPUOffL)
}

func z80WasmEmitIndexedResultRegister(body *wasmBody, opcode byte) {
	if opcode&7 == 6 {
		return
	}
	offset, _ := z80WasmRegisterOffset(opcode & 7)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
}

// z80WasmEmitCBBITValue applies BIT flags to the operand in local 3.
func z80WasmEmitCBBITValue(body *wasmBody, opcode byte) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	body.i32Const(z80FlagH)
	body.op(wasmOpI32Or)
	body.localGet(3)
	body.i32Const(z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.localGet(3)
	body.i32Const(int32(1 << ((opcode >> 3) & 7)))
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagZ | z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	if opcode>>3&7 == 7 {
		body.localGet(3)
		body.i32Const(0x80)
		body.op(wasmOpI32And)
		body.ifVoid()
		body.localGet(4)
		body.i32Const(z80FlagS)
		body.op(wasmOpI32Or)
		body.localSet(4)
		body.end()
	}
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitAParityFlags(body *wasmBody) {
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffA)
	body.localSet(2)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	body.localGet(2)
	body.i32Const(z80FlagS | z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.localGet(2)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagZ)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	body.localGet(2)
	body.localSet(5)
	for _, shift := range []int32{4, 2, 1} {
		body.localGet(5)
		body.localGet(5)
		body.i32Const(shift)
		body.op(wasmOpI32ShrU)
		body.op(wasmOpI32Xor)
		body.localSet(5)
	}
	body.localGet(5)
	body.i32Const(1)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(4)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(4)
	body.end()
	body.localGet(1)
	body.localGet(4)
	body.i32Store8(0, z80WasmCPUOffF)
}

func z80WasmEmitCBRRCReg(body *wasmBody, offset uint32) {
	body.localGet(1)
	body.i32Load8U(0, offset)
	body.localSet(2)
	body.localGet(2)
	body.i32Const(1)
	body.op(wasmOpI32ShrU)
	body.localGet(2)
	body.i32Const(7)
	body.op(wasmOpI32Shl)
	body.op(wasmOpI32Or)
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(3)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
	z80WasmEmitCBRotateResultFlags(body, 2, 3, 4, 5, false)
}

func z80WasmEmitCBRLReg(body *wasmBody, offset uint32) {
	body.localGet(1)
	body.i32Load8U(0, offset)
	body.localSet(2)
	body.localGet(2)
	body.i32Const(1)
	body.op(wasmOpI32Shl)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(3)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
	z80WasmEmitCBRotateResultFlags(body, 2, 3, 4, 5, true)
}

func z80WasmEmitCBRRReg(body *wasmBody, offset uint32) {
	body.localGet(1)
	body.i32Load8U(0, offset)
	body.localSet(2)
	body.localGet(2)
	body.i32Const(1)
	body.op(wasmOpI32ShrU)
	body.localGet(1)
	body.i32Load8U(0, z80WasmCPUOffF)
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	body.i32Const(7)
	body.op(wasmOpI32Shl)
	body.op(wasmOpI32Or)
	body.localSet(3)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
	z80WasmEmitCBRotateResultFlags(body, 2, 3, 4, 5, false)
}

func z80WasmEmitCBSLAReg(body *wasmBody, offset uint32) {
	body.localGet(1)
	body.i32Load8U(0, offset)
	body.localSet(2)
	body.localGet(2)
	body.i32Const(1)
	body.op(wasmOpI32Shl)
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(3)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
	z80WasmEmitCBRotateResultFlags(body, 2, 3, 4, 5, true)
}

func z80WasmEmitCBSRLReg(body *wasmBody, offset uint32) {
	body.localGet(1)
	body.i32Load8U(0, offset)
	body.localSet(2)
	body.localGet(2)
	body.i32Const(1)
	body.op(wasmOpI32ShrU)
	body.localSet(3)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
	z80WasmEmitCBRotateResultFlags(body, 2, 3, 4, 5, false)
}

func z80WasmEmitCBSRAReg(body *wasmBody, offset uint32) {
	body.localGet(1)
	body.i32Load8U(0, offset)
	body.localSet(2)
	body.localGet(2)
	body.i32Const(1)
	body.op(wasmOpI32ShrU)
	body.localGet(2)
	body.i32Const(0x80)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Or)
	body.localSet(3)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
	z80WasmEmitCBRotateResultFlags(body, 2, 3, 4, 5, false)
}

func z80WasmEmitCBSLLReg(body *wasmBody, offset uint32) {
	body.localGet(1)
	body.i32Load8U(0, offset)
	body.localSet(2)
	body.localGet(2)
	body.i32Const(1)
	body.op(wasmOpI32Shl)
	body.i32Const(1)
	body.op(wasmOpI32Or)
	body.i32Const(0xFF)
	body.op(wasmOpI32And)
	body.localSet(3)
	body.localGet(1)
	body.localGet(3)
	body.i32Store8(0, offset)
	z80WasmEmitCBRotateResultFlags(body, 2, 3, 4, 5, true)
}

func z80WasmEmitCBRotateResultFlags(body *wasmBody, old, result, flags, scratch uint32, carryHigh bool) {
	body.localGet(result)
	body.i32Const(z80FlagS | z80FlagX | z80FlagY)
	body.op(wasmOpI32And)
	body.localSet(flags)
	body.localGet(result)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(flags)
	body.i32Const(z80FlagZ)
	body.op(wasmOpI32Or)
	body.localSet(flags)
	body.end()
	body.localGet(result)
	body.localSet(scratch)
	for _, shift := range []int32{4, 2, 1} {
		body.localGet(scratch)
		body.localGet(scratch)
		body.i32Const(shift)
		body.op(wasmOpI32ShrU)
		body.op(wasmOpI32Xor)
		body.localSet(scratch)
	}
	body.localGet(scratch)
	body.i32Const(1)
	body.op(wasmOpI32And)
	body.op(wasmOpI32Eqz)
	body.ifVoid()
	body.localGet(flags)
	body.i32Const(z80FlagPV)
	body.op(wasmOpI32Or)
	body.localSet(flags)
	body.end()
	body.localGet(old)
	if carryHigh {
		body.i32Const(7)
		body.op(wasmOpI32ShrU)
	} else {
		body.i32Const(1)
		body.op(wasmOpI32And)
	}
	body.i32Const(z80FlagC)
	body.op(wasmOpI32And)
	body.localGet(flags)
	body.op(wasmOpI32Or)
	body.localSet(flags)
	body.localGet(1)
	body.localGet(flags)
	body.i32Store8(0, z80WasmCPUOffF)
}
