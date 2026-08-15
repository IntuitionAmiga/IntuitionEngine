// jit_6502_wasm_emit.go - pure WebAssembly encoder for 6502 JIT blocks.
//
// This file is deliberately untagged. Host-side wazero tests execute exactly
// the module bytes the js/wasm dispatcher will instantiate against Go's linear
// memory. Context pointer fields are wasm32 byte offsets in the low 32 bits of
// the shared JIT6502Context layout.

package main

import "fmt"

const (
	// wasm32 context image offsets. These deliberately do not use the native
	// Go struct offsets: wasm pointers are 32-bit and the image is a stable
	// browser/wazero ABI.
	p65WasmCtxOffMemPtr      = 0
	p65WasmCtxOffIOBitmap    = 4
	p65WasmCtxOffCpuPtr      = 8
	p65WasmCtxOffCodePages   = 12
	p65WasmCtxOffRetCycles   = 16
	p65WasmCtxOffNeedBail    = 24
	p65WasmCtxOffNeedInval   = 28
	p65WasmCtxOffRetPC       = 32
	p65WasmCtxOffRetCount    = 36
	p65WasmCtxOffNZTable     = 40
	p65WasmCtxOffDecimalADC  = 44
	p65WasmCtxOffDecimalSBC  = 48
	p65WasmCtxOffDirectPages = 52
	p65WasmCtxOffBinaryADC   = 56
	p65WasmCtxOffBinarySBC   = 60
	p65WasmCtxImageSize      = 64

	p65WasmLCtx   = 0 // block(ctx i32)
	p65WasmLCPU   = 1
	p65WasmLSR    = 2
	p65WasmLVal   = 3
	p65WasmLFlags = 4
	p65WasmLAddr  = 5
	p65WasmLCross = 6
)

type p65WasmLogicOp byte

const (
	p65WasmAnd p65WasmLogicOp = iota
	p65WasmOra
	p65WasmEor
)

// p65WasmMutatesRAM identifies forms whose write may change the source bytes
// of a later instruction in the same cached module. The dispatcher ends a
// compiled prefix after one of these forms so source validation occurs before
// executing the next guest instruction.
func p65WasmMutatesRAM(opcode byte) bool {
	switch opcode {
	case 0x00, 0x08, 0x20, 0x48: // BRK, PHP, JSR, PHA write the hardware stack
		return true
	case 0x81, 0x85, 0x8D, 0x91, 0x95, 0x99, 0x9D, 0x84, 0x8C, 0x94, 0x86, 0x8E, 0x96: // STA/STX/STY
		return true
	case 0x06, 0x0E, 0x16, 0x1E, 0x26, 0x2E, 0x36, 0x3E, 0x46, 0x4E, 0x56, 0x5E, 0x66, 0x6E, 0x76, 0x7E: // shifts/rotates
		return true
	case 0xC6, 0xCE, 0xD6, 0xDE, 0xE6, 0xEE, 0xF6, 0xFE: // DEC/INC
		return true
	default:
		return false
	}
}

func p65WasmSetNZKnown(b *wasmBody, value byte) {
	// local SR = (SR & ^(N|Z)) | known flags
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.i32Const(^int32(ZERO_FLAG | NEGATIVE_FLAG))
	b.op(wasmOpI32And)
	flags := int32(0)
	if value == 0 {
		flags |= ZERO_FLAG
	}
	if value&0x80 != 0 {
		flags |= NEGATIVE_FLAG
	}
	b.i32Const(flags)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLSR)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Store8(0, cpu6502OffSR)
}

func p65WasmEmitImmediateLoad(b *wasmBody, value byte, cpuOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.i32Const(int32(value))
	b.i32Store8(0, cpuOffset)
	p65WasmSetNZKnown(b, value)
}

func p65WasmSetNZValue(b *wasmBody) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.i32Const(^int32(ZERO_FLAG | NEGATIVE_FLAG))
	b.op(wasmOpI32And)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffNZTable)
	b.localGet(p65WasmLVal)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLSR)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Store8(0, cpu6502OffSR)
}

func p65WasmEmitTransfer(b *wasmBody, sourceOffset, destOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, sourceOffset)
	b.localTee(p65WasmLVal)
	b.i32Store8(0, destOffset)
	p65WasmSetNZValue(b)
}

func p65WasmEmitTransferNoFlags(b *wasmBody, sourceOffset, destOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, sourceOffset)
	b.i32Store8(0, destOffset)
}

func p65WasmEmitAccumulatorShift(b *wasmBody, opcode byte) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffA)
	b.localSet(p65WasmLVal)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.localSet(p65WasmLSR)
	switch opcode {
	case 0x0A: // ASL A
		b.localGet(p65WasmLVal)
		b.i32Const(7)
		b.op(wasmOpI32ShrU)
		b.localSet(p65WasmLFlags)
		b.localGet(p65WasmLVal)
		b.i32Const(1)
		b.op(wasmOpI32Shl)
		b.localSet(p65WasmLVal)
	case 0x4A: // LSR A
		b.localGet(p65WasmLVal)
		b.i32Const(CARRY_FLAG)
		b.op(wasmOpI32And)
		b.localSet(p65WasmLFlags)
		b.localGet(p65WasmLVal)
		b.i32Const(1)
		b.op(wasmOpI32ShrU)
		b.localSet(p65WasmLVal)
	case 0x2A: // ROL A
		b.localGet(p65WasmLVal)
		b.i32Const(7)
		b.op(wasmOpI32ShrU)
		b.localSet(p65WasmLFlags)
		b.localGet(p65WasmLVal)
		b.i32Const(1)
		b.op(wasmOpI32Shl)
		b.localGet(p65WasmLSR)
		b.i32Const(CARRY_FLAG)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Or)
		b.localSet(p65WasmLVal)
	case 0x6A: // ROR A
		b.localGet(p65WasmLVal)
		b.i32Const(CARRY_FLAG)
		b.op(wasmOpI32And)
		b.localSet(p65WasmLFlags)
		b.localGet(p65WasmLVal)
		b.i32Const(1)
		b.op(wasmOpI32ShrU)
		b.localGet(p65WasmLSR)
		b.i32Const(CARRY_FLAG)
		b.op(wasmOpI32And)
		b.i32Const(7)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
		b.localSet(p65WasmLVal)
	}
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLVal)
	b.i32Store8(0, cpu6502OffA)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Const(^int32(CARRY_FLAG))
	b.op(wasmOpI32And)
	b.localGet(p65WasmLFlags)
	b.op(wasmOpI32Or)
	b.i32Store8(0, cpu6502OffSR)
	p65WasmSetNZValue(b)
}

// p65WasmEmitShiftLoadedValue transforms LVal and updates C/N/Z. The caller
// owns the memory or accumulator write-back.
func p65WasmEmitShiftLoadedValue(b *wasmBody, opcode byte) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.localSet(p65WasmLSR)
	switch opcode {
	case 0x06, 0x0E, 0x16, 0x1E: // ASL
		b.localGet(p65WasmLVal)
		b.i32Const(7)
		b.op(wasmOpI32ShrU)
		b.localSet(p65WasmLFlags)
		b.localGet(p65WasmLVal)
		b.i32Const(1)
		b.op(wasmOpI32Shl)
		b.localSet(p65WasmLVal)
	case 0x46, 0x4E, 0x56, 0x5E: // LSR
		b.localGet(p65WasmLVal)
		b.i32Const(CARRY_FLAG)
		b.op(wasmOpI32And)
		b.localSet(p65WasmLFlags)
		b.localGet(p65WasmLVal)
		b.i32Const(1)
		b.op(wasmOpI32ShrU)
		b.localSet(p65WasmLVal)
	case 0x26, 0x2E, 0x36, 0x3E: // ROL
		b.localGet(p65WasmLVal)
		b.i32Const(7)
		b.op(wasmOpI32ShrU)
		b.localSet(p65WasmLFlags)
		b.localGet(p65WasmLVal)
		b.i32Const(1)
		b.op(wasmOpI32Shl)
		b.localGet(p65WasmLSR)
		b.i32Const(CARRY_FLAG)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Or)
		b.localSet(p65WasmLVal)
	case 0x66, 0x6E, 0x76, 0x7E: // ROR
		b.localGet(p65WasmLVal)
		b.i32Const(CARRY_FLAG)
		b.op(wasmOpI32And)
		b.localSet(p65WasmLFlags)
		b.localGet(p65WasmLVal)
		b.i32Const(1)
		b.op(wasmOpI32ShrU)
		b.localGet(p65WasmLSR)
		b.i32Const(CARRY_FLAG)
		b.op(wasmOpI32And)
		b.i32Const(7)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
		b.localSet(p65WasmLVal)
	}
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Const(^int32(CARRY_FLAG))
	b.op(wasmOpI32And)
	b.localGet(p65WasmLFlags)
	b.op(wasmOpI32Or)
	b.i32Store8(0, cpu6502OffSR)
	p65WasmSetNZValue(b)
}

func p65WasmEmitRMWShiftDirect(b *wasmBody, operand uint16, opcode byte) {
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLAddr)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	p65WasmEmitShiftLoadedValue(b, opcode)
	b.localGet(p65WasmLAddr)
	b.localGet(p65WasmLVal)
	b.i32Store8(0, 0)
}

func p65WasmEmitZPIndexedRMWShift(b *wasmBody, operand, opcode byte) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffX)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	p65WasmEmitShiftLoadedValue(b, opcode)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLVal)
	b.i32Store8(0, 0)
}

func p65WasmEmitAbsIndexedRMW(b *wasmBody, operand uint16, decrement bool) {
	p65WasmEmitAbsIndexedAddress(b, operand, cpu6502OffX)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.i32Const(1)
	if decrement {
		b.op(wasmOpI32Sub)
	} else {
		b.op(wasmOpI32Add)
	}
	b.localSet(p65WasmLVal)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLVal)
	b.i32Store8(0, 0)
	p65WasmSetNZValue(b)
}

func p65WasmEmitAbsIndexedRMWShift(b *wasmBody, operand uint16, opcode byte) {
	p65WasmEmitAbsIndexedAddress(b, operand, cpu6502OffX)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	p65WasmEmitShiftLoadedValue(b, opcode)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLVal)
	b.i32Store8(0, 0)
}

func p65WasmEmitCompareImmediate(b *wasmBody, operand byte, sourceOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, sourceOffset)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Sub)
	b.localSet(p65WasmLVal)
	// Preserve all flags except C, then derive C from an unsigned comparison.
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.i32Const(^int32(CARRY_FLAG))
	b.op(wasmOpI32And)
	b.localSet(p65WasmLSR)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, sourceOffset)
	b.i32Const(int32(operand))
	b.op(wasmOpI32GeU)
	b.ifVoid()
	b.localGet(p65WasmLSR)
	b.i32Const(CARRY_FLAG)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLSR)
	b.end()
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Store8(0, cpu6502OffSR)
	p65WasmSetNZValue(b)
}

// p65WasmEmitCompareLoadedOperand consumes the operand already held in LVal.
func p65WasmEmitCompareLoadedOperand(b *wasmBody, sourceOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.i32Const(^int32(CARRY_FLAG))
	b.op(wasmOpI32And)
	b.localSet(p65WasmLSR)
	// Carry is the unsigned source >= operand relation.
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, sourceOffset)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32GeU)
	b.ifVoid()
	b.localGet(p65WasmLSR)
	b.i32Const(CARRY_FLAG)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLSR)
	b.end()
	// Keep the subtraction's low byte in LVal for N/Z materialisation.
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, sourceOffset)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32Sub)
	b.localSet(p65WasmLVal)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Store8(0, cpu6502OffSR)
	p65WasmSetNZValue(b)
}

func p65WasmEmitCompareDirect(b *wasmBody, operand uint16, sourceOffset uint32) {
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	p65WasmEmitCompareLoadedOperand(b, sourceOffset)
}

func p65WasmEmitIncDec(b *wasmBody, offset uint32, decrement bool) {
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, offset)
	b.i32Const(1)
	if decrement {
		b.op(wasmOpI32Sub)
	} else {
		b.op(wasmOpI32Add)
	}
	b.localTee(p65WasmLVal)
	b.i32Store8(0, offset)
	p65WasmSetNZValue(b)
}

func p65WasmEmitFlag(b *wasmBody, flag byte, set bool) {
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	if set {
		b.i32Const(int32(flag))
		b.op(wasmOpI32Or)
	} else {
		b.i32Const(^int32(flag))
		b.op(wasmOpI32And)
	}
	b.i32Store8(0, cpu6502OffSR)
}

func p65WasmEmitLogicImmediate(b *wasmBody, operand byte, op p65WasmLogicOp) {
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffA)
	b.i32Const(int32(operand))
	switch op {
	case p65WasmAnd:
		b.op(wasmOpI32And)
	case p65WasmOra:
		b.op(wasmOpI32Or)
	case p65WasmEor:
		b.op(wasmOpI32Xor)
	}
	b.localTee(p65WasmLVal)
	b.i32Store8(0, cpu6502OffA)
	p65WasmSetNZValue(b)
}

func p65WasmEmitLogicLoadedOperand(b *wasmBody, op p65WasmLogicOp) {
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffA)
	b.localGet(p65WasmLVal)
	switch op {
	case p65WasmAnd:
		b.op(wasmOpI32And)
	case p65WasmOra:
		b.op(wasmOpI32Or)
	case p65WasmEor:
		b.op(wasmOpI32Xor)
	}
	b.localTee(p65WasmLVal)
	b.i32Store8(0, cpu6502OffA)
	p65WasmSetNZValue(b)
}

// p65WasmEmitDecimalImmediate selects decimal or binary packed result tables.
func p65WasmEmitDecimalImmediate(b *wasmBody, operand byte, decimalTableOffset, binaryTableOffset uint32) {
	// Preserve SR for table selection and output merge.
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.localSet(p65WasmLSR)
	b.localGet(p65WasmLSR)
	b.i32Const(DECIMAL_FLAG)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	b.localGet(p65WasmLCtx)
	b.i32Load(2, binaryTableOffset)
	b.localSet(p65WasmLFlags)
	b.elseBranch()
	b.localGet(p65WasmLCtx)
	b.i32Load(2, decimalTableOffset)
	b.localSet(p65WasmLFlags)
	b.end()

	// index = A | operand<<8 | carry<<16, then byte offset *= 2.
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffA)
	b.i32Const(int32(operand))
	b.i32Const(8)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLSR)
	b.i32Const(CARRY_FLAG)
	b.op(wasmOpI32And)
	b.i32Const(16)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Add)
	b.i32Const(1)
	b.op(wasmOpI32Shl)
	b.localSet(p65WasmLVal)
	// packed result = i32.load16_u(table + byteOffset)
	b.localGet(p65WasmLFlags)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32Add)
	b.i32Load16U(1, 0)
	b.localSet(p65WasmLVal)
	// A is the low byte.
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLVal)
	b.i32Store8(0, cpu6502OffA)
	// Merge the high-byte C/V/N/Z flags into SR.
	b.localGet(p65WasmLVal)
	b.i32Const(8)
	b.op(wasmOpI32ShrU)
	b.localSet(p65WasmLFlags)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Const(^int32(CARRY_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG | ZERO_FLAG))
	b.op(wasmOpI32And)
	b.localGet(p65WasmLFlags)
	b.op(wasmOpI32Or)
	b.i32Store8(0, cpu6502OffSR)
}

// p65WasmEmitArithmeticLoadedOperand applies a packed result table to the
// direct-RAM operand already held in LVal.
func p65WasmEmitArithmeticLoadedOperand(b *wasmBody, decimalTableOffset, binaryTableOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.localSet(p65WasmLSR)
	b.localGet(p65WasmLSR)
	b.i32Const(DECIMAL_FLAG)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	b.localGet(p65WasmLCtx)
	b.i32Load(2, binaryTableOffset)
	b.localSet(p65WasmLFlags)
	b.elseBranch()
	b.localGet(p65WasmLCtx)
	b.i32Load(2, decimalTableOffset)
	b.localSet(p65WasmLFlags)
	b.end()
	// index = A | operand<<8 | carry<<16, then byte offset *= 2.
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffA)
	b.localGet(p65WasmLVal)
	b.i32Const(8)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLSR)
	b.i32Const(CARRY_FLAG)
	b.op(wasmOpI32And)
	b.i32Const(16)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Add)
	b.i32Const(1)
	b.op(wasmOpI32Shl)
	b.localSet(p65WasmLVal)
	b.localGet(p65WasmLFlags)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32Add)
	b.i32Load16U(1, 0)
	b.localSet(p65WasmLVal)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLVal)
	b.i32Store8(0, cpu6502OffA)
	b.localGet(p65WasmLVal)
	b.i32Const(8)
	b.op(wasmOpI32ShrU)
	b.localSet(p65WasmLFlags)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Const(^int32(CARRY_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG | ZERO_FLAG))
	b.op(wasmOpI32And)
	b.localGet(p65WasmLFlags)
	b.op(wasmOpI32Or)
	b.i32Store8(0, cpu6502OffSR)
}

func p65WasmEmitArithmeticDirect(b *wasmBody, operand uint16, decimalTableOffset, binaryTableOffset uint32) {
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	p65WasmEmitArithmeticLoadedOperand(b, decimalTableOffset, binaryTableOffset)
}

func p65WasmEmitBail(b *wasmBody, instrPC uint16, retired, priorCycles uint32) {
	b.localGet(p65WasmLCtx)
	b.i32Const(1)
	b.i32Store(2, p65WasmCtxOffNeedBail)
	b.localGet(p65WasmLCtx)
	b.i32Const(int32(instrPC))
	b.i32Store(2, p65WasmCtxOffRetPC)
	b.localGet(p65WasmLCtx)
	b.i32Const(int32(retired))
	b.i32Store(2, p65WasmCtxOffRetCount)
	b.localGet(p65WasmLCtx)
	b.i64Const(int64(priorCycles))
	b.i64Store(3, p65WasmCtxOffRetCycles)
	b.op(wasmOpReturn)
}

func p65WasmEmitReturn(b *wasmBody, retPC uint16, retired, cycles uint32) {
	b.localGet(p65WasmLCtx)
	b.i32Const(int32(retPC))
	b.i32Store(2, p65WasmCtxOffRetPC)
	b.localGet(p65WasmLCtx)
	b.i32Const(int32(retired))
	b.i32Store(2, p65WasmCtxOffRetCount)
	b.localGet(p65WasmLCtx)
	b.i64Const(int64(cycles))
	b.i64Store(3, p65WasmCtxOffRetCycles)
	b.op(wasmOpReturn)
}

func p65WasmEmitReturnDynamicPC(b *wasmBody, pcLocal, retired, cycles uint32) {
	b.localGet(p65WasmLCtx)
	b.localGet(pcLocal)
	b.i32Store(2, p65WasmCtxOffRetPC)
	b.localGet(p65WasmLCtx)
	b.i32Const(int32(retired))
	b.i32Store(2, p65WasmCtxOffRetCount)
	b.localGet(p65WasmLCtx)
	b.i64Const(int64(cycles))
	b.i64Store(3, p65WasmCtxOffRetCycles)
	b.op(wasmOpReturn)
}

func p65WasmEmitCrossReturn(b *wasmBody, retPC uint16, retired, cycles uint32) {
	b.localGet(p65WasmLCross)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	p65WasmEmitReturn(b, retPC, retired, cycles)
	b.elseBranch()
	p65WasmEmitReturn(b, retPC, retired, cycles+1)
	b.end()
}

// p65WasmEmitBranch preserves CPU_6502's existing penalty-only branch
// accounting: fall-through is zero cycles, a taken branch costs one, and a
// page crossing costs one more. Each arm returns so the final straight-line
// publisher cannot overwrite the dynamic target.
func p65WasmEmitBranch(b *wasmBody, instrPC uint16, operand byte, flag byte, branchWhenSet bool, retired, priorCycles uint32) {
	fallPC := uint16(uint32(instrPC) + 2)
	target := uint16(int32(fallPC) + int32(int8(operand)))
	takenCycles := priorCycles + 1
	if fallPC&0xFF00 != target&0xFF00 {
		takenCycles++
	}
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.i32Const(int32(flag))
	b.op(wasmOpI32And)
	if !branchWhenSet {
		b.op(wasmOpI32Eqz)
	}
	b.ifVoid()
	p65WasmEmitReturn(b, target, retired+1, takenCycles)
	b.elseBranch()
	p65WasmEmitReturn(b, fallPC, retired+1, priorCycles)
	b.end()
}

// p65WasmDirectGuard opens an if/else: the caller emits the direct-RAM body
// in the then arm, then calls p65WasmDirectGuardEnd to emit the exact bailout.
func p65WasmDirectGuard(b *wasmBody, page byte) {
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffDirectPages)
	b.i32Const(int32(page))
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
}

func p65WasmDirectGuardEnd(b *wasmBody, instrPC uint16, retired, priorCycles uint32) {
	b.elseBranch()
	p65WasmEmitBail(b, instrPC, retired, priorCycles)
	b.end()
}

func p65WasmDynamicDirectGuard(b *wasmBody) {
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffDirectPages)
	b.localGet(p65WasmLAddr)
	b.i32Const(8)
	b.op(wasmOpI32ShrU)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
}

func p65WasmEmitDirectLoad(b *wasmBody, operand uint16, destOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localTee(p65WasmLVal)
	b.i32Store8(0, destOffset)
	p65WasmSetNZValue(b)
}

func p65WasmEmitDirectStore(b *wasmBody, operand uint16, sourceOffset uint32) {
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, sourceOffset)
	b.i32Store8(0, 0)
}

func p65WasmEmitZPIndexedLoad(b *wasmBody, operand byte, indexOffset, destOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, indexOffset)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localTee(p65WasmLVal)
	b.i32Store8(0, destOffset)
	p65WasmSetNZValue(b)
}

func p65WasmEmitZPIndexedStore(b *wasmBody, operand byte, indexOffset, sourceOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, indexOffset)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, sourceOffset)
	b.i32Store8(0, 0)
}

func p65WasmEmitAbsIndexedAddress(b *wasmBody, operand uint16, indexOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, indexOffset)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Const(0xFFFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
}

func p65WasmEmitAbsIndexedAddressWithCross(b *wasmBody, operand uint16, indexOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, indexOffset)
	b.i32Const(int32(byte(operand)))
	b.op(wasmOpI32Add)
	b.i32Const(8)
	b.op(wasmOpI32ShrU)
	b.localSet(p65WasmLCross)
	p65WasmEmitAbsIndexedAddress(b, operand, indexOffset)
}

func p65WasmEmitStoreAtAddr(b *wasmBody, sourceOffset uint32) {
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, sourceOffset)
	b.i32Store8(0, 0)
}

// p65WasmEmitIndirectStoreAddress resolves the NMOS zero-page pointer used by
// STA (zp,X) and STA (zp),Y. The caller owns the zero-page and destination
// mapping guards, so this helper only computes LAddr without side effects.
func p65WasmEmitIndirectStoreAddress(b *wasmBody, operand byte, indexedX bool) {
	if indexedX {
		b.localGet(p65WasmLCPU)
		b.i32Load8U(0, cpu6502OffX)
		b.i32Const(int32(operand))
		b.op(wasmOpI32Add)
		b.i32Const(0xFF)
		b.op(wasmOpI32And)
	} else {
		b.i32Const(int32(operand))
	}
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	if !indexedX {
		b.localGet(p65WasmLVal)
		b.localGet(p65WasmLCPU)
		b.i32Load8U(0, cpu6502OffY)
		b.op(wasmOpI32Add)
		b.i32Const(8)
		b.op(wasmOpI32ShrU)
		b.localSet(p65WasmLCross)
	}
	b.localGet(p65WasmLAddr)
	b.i32Const(1)
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.i32Const(8)
	b.op(wasmOpI32Shl)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32Or)
	if !indexedX {
		b.localGet(p65WasmLCPU)
		b.i32Load8U(0, cpu6502OffY)
		b.op(wasmOpI32Add)
		b.i32Const(0xFFFF)
		b.op(wasmOpI32And)
	}
	b.localSet(p65WasmLAddr)
}

func p65WasmEmitLoadAtAddr(b *wasmBody, destOffset uint32) {
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localTee(p65WasmLVal)
	b.i32Store8(0, destOffset)
	p65WasmSetNZValue(b)
}

func p65WasmEmitZPIndexedLogic(b *wasmBody, operand byte, op p65WasmLogicOp) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffX)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffA)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	switch op {
	case p65WasmAnd:
		b.op(wasmOpI32And)
	case p65WasmOra:
		b.op(wasmOpI32Or)
	case p65WasmEor:
		b.op(wasmOpI32Xor)
	}
	b.localTee(p65WasmLVal)
	b.i32Store8(0, cpu6502OffA)
	p65WasmSetNZValue(b)
}

func p65WasmEmitBITDirect(b *wasmBody, operand uint16) {
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	// SR = (SR & ^(Z|V|N)) | (value & (V|N)).
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.i32Const(^int32(ZERO_FLAG | OVERFLOW_FLAG | NEGATIVE_FLAG))
	b.op(wasmOpI32And)
	b.localGet(p65WasmLVal)
	b.i32Const(OVERFLOW_FLAG | NEGATIVE_FLAG)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLSR)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffA)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Eqz)
	b.ifVoid()
	b.localGet(p65WasmLSR)
	b.i32Const(ZERO_FLAG)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLSR)
	b.end()
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Store8(0, cpu6502OffSR)
}

func p65WasmEmitRMWDirect(b *wasmBody, operand uint16, decrement bool) {
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLAddr)
	b.i32Load8U(0, 0)
	b.i32Const(1)
	if decrement {
		b.op(wasmOpI32Sub)
	} else {
		b.op(wasmOpI32Add)
	}
	b.localSet(p65WasmLVal)
	// Store expects address then value; rebuild the direct address from context.
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLVal)
	b.i32Store8(0, 0)
	p65WasmSetNZValue(b)
}

func p65WasmEmitZPIndexedRMW(b *wasmBody, operand byte, decrement bool) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffX)
	b.i32Const(int32(operand))
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLAddr)
	b.i32Load8U(0, 0)
	b.i32Const(1)
	if decrement {
		b.op(wasmOpI32Sub)
	} else {
		b.op(wasmOpI32Add)
	}
	b.localSet(p65WasmLVal)
	b.localGet(p65WasmLAddr)
	b.localGet(p65WasmLVal)
	b.i32Store8(0, 0)
	p65WasmSetNZValue(b)
}

func p65WasmEmitStack(b *wasmBody, opcode byte) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSP)
	b.localSet(p65WasmLAddr)
	switch opcode {
	case 0x48, 0x08: // PHA, PHP
		b.localGet(p65WasmLCtx)
		b.i32Load(2, p65WasmCtxOffMemPtr)
		b.i32Const(0x100)
		b.op(wasmOpI32Add)
		b.localGet(p65WasmLAddr)
		b.op(wasmOpI32Add)
		b.localGet(p65WasmLCPU)
		if opcode == 0x48 {
			b.i32Load8U(0, cpu6502OffA)
		} else {
			b.i32Load8U(0, cpu6502OffSR)
			b.i32Const(BREAK_FLAG | UNUSED_FLAG)
			b.op(wasmOpI32Or)
		}
		b.i32Store8(0, 0)
		b.localGet(p65WasmLCPU)
		b.localGet(p65WasmLAddr)
		b.i32Const(1)
		b.op(wasmOpI32Sub)
		b.i32Store8(0, cpu6502OffSP)
	case 0x68, 0x28: // PLA, PLP
		b.localGet(p65WasmLAddr)
		b.i32Const(1)
		b.op(wasmOpI32Add)
		b.i32Const(0xFF)
		b.op(wasmOpI32And)
		b.localSet(p65WasmLAddr)
		b.localGet(p65WasmLCPU)
		b.localGet(p65WasmLAddr)
		b.i32Store8(0, cpu6502OffSP)
		b.localGet(p65WasmLCtx)
		b.i32Load(2, p65WasmCtxOffMemPtr)
		b.i32Const(0x100)
		b.op(wasmOpI32Add)
		b.localGet(p65WasmLAddr)
		b.op(wasmOpI32Add)
		b.i32Load8U(0, 0)
		b.localSet(p65WasmLVal)
		if opcode == 0x68 {
			b.localGet(p65WasmLCPU)
			b.localGet(p65WasmLVal)
			b.i32Store8(0, cpu6502OffA)
			p65WasmSetNZValue(b)
		} else {
			b.localGet(p65WasmLCPU)
			b.localGet(p65WasmLVal)
			b.i32Const(^int32(BREAK_FLAG))
			b.op(wasmOpI32And)
			b.i32Const(UNUSED_FLAG)
			b.op(wasmOpI32Or)
			b.i32Store8(0, cpu6502OffSR)
		}
	}
}

func p65WasmEmitJSR(b *wasmBody, startPC uint16, target uint16) {
	returnAddr := uint16(uint32(startPC) + 2)
	// Push high then low, exactly as CPU_6502.push16 does.
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSP)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Const(int32(returnAddr >> 8))
	b.i32Store8(0, 0)
	b.localGet(p65WasmLAddr)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Const(int32(byte(returnAddr)))
	b.i32Store8(0, 0)
	b.localGet(p65WasmLAddr)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	_ = target
}

func p65WasmEmitRTS(b *wasmBody) {
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSP)
	b.i32Const(1)
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	b.localGet(p65WasmLAddr)
	b.i32Const(1)
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.i32Const(8)
	b.op(wasmOpI32Shl)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32Or)
	b.i32Const(1)
	b.op(wasmOpI32Add)
	b.i32Const(0xFFFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLVal)
}

func p65WasmEmitJMPIndirect(b *wasmBody, pointer uint16) {
	highAddr := (pointer & 0xFF00) | ((pointer + 1) & 0x00FF)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(pointer))
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(int32(highAddr))
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.i32Const(8)
	b.op(wasmOpI32Shl)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLVal)
}

func p65WasmEmitRTI(b *wasmBody) {
	// status
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSP)
	b.i32Const(1)
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.i32Const(^int32(BREAK_FLAG))
	b.op(wasmOpI32And)
	b.i32Const(UNUSED_FLAG)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLSR)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLSR)
	b.i32Store8(0, cpu6502OffSR)
	// low PC
	b.localGet(p65WasmLAddr)
	b.i32Const(1)
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	// high PC
	b.localGet(p65WasmLAddr)
	b.i32Const(1)
	b.op(wasmOpI32Add)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.i32Const(8)
	b.op(wasmOpI32Shl)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLVal)
}

func p65WasmEmitBRK(b *wasmBody, startPC uint16) {
	returnPC := uint16(uint32(startPC) + 2)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSP)
	b.localSet(p65WasmLAddr)
	// Push return high.
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Const(int32(returnPC >> 8))
	b.i32Store8(0, 0)
	b.localGet(p65WasmLAddr)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	// Push return low.
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.i32Const(int32(byte(returnPC)))
	b.i32Store8(0, 0)
	b.localGet(p65WasmLAddr)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	// Push SR with B/U asserted.
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(0x100)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLAddr)
	b.op(wasmOpI32Add)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.i32Const(BREAK_FLAG | UNUSED_FLAG)
	b.op(wasmOpI32Or)
	b.i32Store8(0, 0)
	b.localGet(p65WasmLAddr)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
	b.localSet(p65WasmLAddr)
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLAddr)
	b.i32Store8(0, cpu6502OffSP)
	// Live SR has I/U set and B clear.
	b.localGet(p65WasmLCPU)
	b.localGet(p65WasmLCPU)
	b.i32Load8U(0, cpu6502OffSR)
	b.i32Const(INTERRUPT_FLAG | UNUSED_FLAG)
	b.op(wasmOpI32Or)
	b.i32Const(^int32(BREAK_FLAG))
	b.op(wasmOpI32And)
	b.i32Store8(0, cpu6502OffSR)
	// Defined vector path, intentionally not subject to the general I/O guard.
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(IRQ_VECTOR)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.localSet(p65WasmLVal)
	b.localGet(p65WasmLCtx)
	b.i32Load(2, p65WasmCtxOffMemPtr)
	b.i32Const(IRQ_VECTOR + 1)
	b.op(wasmOpI32Add)
	b.i32Load8U(0, 0)
	b.i32Const(8)
	b.op(wasmOpI32Shl)
	b.localGet(p65WasmLVal)
	b.op(wasmOpI32Or)
	b.localSet(p65WasmLVal)
}

// p65WasmCompileBlock translates an all-supported straight-line block into a
// module exporting block(ctx i32). Unsupported forms are rejected before any
// module is produced, so a dispatcher can execute their first instruction in
// the interpreter at the same observation point.
func p65WasmCompileBlock(instrs []JIT6502Instr, startPC uint16) ([]byte, error) {
	if len(instrs) == 0 {
		return nil, fmt.Errorf("6502 wasm: empty block")
	}
	var body wasmBody
	// cpu = i32.load(ctx + CpuPtr)
	body.localGet(p65WasmLCtx)
	body.i32Load(2, p65WasmCtxOffCpuPtr)
	body.localSet(p65WasmLCPU)

	pc := uint32(startPC)
	cycles := uint32(0)
	for index, instr := range instrs {
		instrPC := pc
		nextPC := pc + uint32(instr.length)
		switch instr.opcode {
		case 0xEA: // NOP
		case 0x4C: // JMP absolute
			nextPC = uint32(instr.operand)
		case 0x20: // JSR absolute
			p65WasmDirectGuard(&body, 1)
			p65WasmEmitJSR(&body, uint16(instrPC), instr.operand)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
			nextPC = uint32(instr.operand)
		case 0x60: // RTS
			p65WasmDirectGuard(&body, 1)
			p65WasmEmitRTS(&body)
			p65WasmEmitReturnDynamicPC(&body, p65WasmLVal, uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x6C: // JMP indirect with NMOS page-wrap high byte
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			highAddr := (instr.operand & 0xFF00) | ((instr.operand + 1) & 0x00FF)
			p65WasmDirectGuard(&body, byte(highAddr>>8))
			p65WasmEmitJMPIndirect(&body, instr.operand)
			p65WasmEmitReturnDynamicPC(&body, p65WasmLVal, uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x40: // RTI
			p65WasmDirectGuard(&body, 1)
			p65WasmEmitRTI(&body)
			p65WasmEmitReturnDynamicPC(&body, p65WasmLVal, uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x00: // BRK; debug observers bail before module compilation
			p65WasmDirectGuard(&body, 1)
			p65WasmEmitBRK(&body, uint16(instrPC))
			p65WasmEmitReturnDynamicPC(&body, p65WasmLVal, uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x48, 0x08: // PHA, PHP
			p65WasmDirectGuard(&body, 1)
			p65WasmEmitStack(&body, instr.opcode)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x68, 0x28: // PLA, PLP
			p65WasmDirectGuard(&body, 1)
			p65WasmEmitStack(&body, instr.opcode)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x90: // BCC
			p65WasmEmitBranch(&body, uint16(instrPC), byte(instr.operand), CARRY_FLAG, false, uint32(index), cycles)
		case 0xB0: // BCS
			p65WasmEmitBranch(&body, uint16(instrPC), byte(instr.operand), CARRY_FLAG, true, uint32(index), cycles)
		case 0xF0: // BEQ
			p65WasmEmitBranch(&body, uint16(instrPC), byte(instr.operand), ZERO_FLAG, true, uint32(index), cycles)
		case 0xD0: // BNE
			p65WasmEmitBranch(&body, uint16(instrPC), byte(instr.operand), ZERO_FLAG, false, uint32(index), cycles)
		case 0x30: // BMI
			p65WasmEmitBranch(&body, uint16(instrPC), byte(instr.operand), NEGATIVE_FLAG, true, uint32(index), cycles)
		case 0x10: // BPL
			p65WasmEmitBranch(&body, uint16(instrPC), byte(instr.operand), NEGATIVE_FLAG, false, uint32(index), cycles)
		case 0x70: // BVS
			p65WasmEmitBranch(&body, uint16(instrPC), byte(instr.operand), OVERFLOW_FLAG, true, uint32(index), cycles)
		case 0x50: // BVC
			p65WasmEmitBranch(&body, uint16(instrPC), byte(instr.operand), OVERFLOW_FLAG, false, uint32(index), cycles)
		case 0xA9: // LDA #imm
			p65WasmEmitImmediateLoad(&body, byte(instr.operand), cpu6502OffA)
		case 0xA2: // LDX #imm
			p65WasmEmitImmediateLoad(&body, byte(instr.operand), cpu6502OffX)
		case 0xA0: // LDY #imm
			p65WasmEmitImmediateLoad(&body, byte(instr.operand), cpu6502OffY)
		case 0x69: // ADC #imm
			p65WasmEmitDecimalImmediate(&body, byte(instr.operand), p65WasmCtxOffDecimalADC, p65WasmCtxOffBinaryADC)
		case 0xE9: // SBC #imm
			p65WasmEmitDecimalImmediate(&body, byte(instr.operand), p65WasmCtxOffDecimalSBC, p65WasmCtxOffBinarySBC)
		case 0x65, 0x6D:
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitArithmeticDirect(&body, instr.operand, p65WasmCtxOffDecimalADC, p65WasmCtxOffBinaryADC)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xE5, 0xED:
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitArithmeticDirect(&body, instr.operand, p65WasmCtxOffDecimalSBC, p65WasmCtxOffBinarySBC)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x75, 0xF5:
			decimalTableOffset, binaryTableOffset := uint32(p65WasmCtxOffDecimalADC), uint32(p65WasmCtxOffBinaryADC)
			if instr.opcode == 0xF5 {
				decimalTableOffset, binaryTableOffset = p65WasmCtxOffDecimalSBC, p65WasmCtxOffBinarySBC
			}
			p65WasmDirectGuard(&body, 0)
			body.localGet(p65WasmLCPU)
			body.i32Load8U(0, cpu6502OffX)
			body.i32Const(int32(byte(instr.operand)))
			body.op(wasmOpI32Add)
			body.i32Const(0xFF)
			body.op(wasmOpI32And)
			body.localSet(p65WasmLAddr)
			body.localGet(p65WasmLCtx)
			body.i32Load(2, p65WasmCtxOffMemPtr)
			body.localGet(p65WasmLAddr)
			body.op(wasmOpI32Add)
			body.i32Load8U(0, 0)
			body.localSet(p65WasmLVal)
			p65WasmEmitArithmeticLoadedOperand(&body, decimalTableOffset, binaryTableOffset)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x79, 0x7D, 0xF9, 0xFD:
			indexOffset := uint32(cpu6502OffX)
			if instr.opcode == 0x79 || instr.opcode == 0xF9 {
				indexOffset = cpu6502OffY
			}
			decimalTableOffset, binaryTableOffset := uint32(p65WasmCtxOffDecimalADC), uint32(p65WasmCtxOffBinaryADC)
			if instr.opcode == 0xF9 || instr.opcode == 0xFD {
				decimalTableOffset, binaryTableOffset = p65WasmCtxOffDecimalSBC, p65WasmCtxOffBinarySBC
			}
			p65WasmEmitAbsIndexedAddressWithCross(&body, instr.operand, indexOffset)
			p65WasmDynamicDirectGuard(&body)
			body.localGet(p65WasmLCtx)
			body.i32Load(2, p65WasmCtxOffMemPtr)
			body.localGet(p65WasmLAddr)
			body.op(wasmOpI32Add)
			body.i32Load8U(0, 0)
			body.localSet(p65WasmLVal)
			p65WasmEmitArithmeticLoadedOperand(&body, decimalTableOffset, binaryTableOffset)
			body.localGet(p65WasmLCross)
			body.op(wasmOpI32Eqz)
			body.ifVoid()
			p65WasmEmitReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			body.elseBranch()
			p65WasmEmitReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode])+1)
			body.end()
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x19, 0x1D, 0x39, 0x3D, 0x59, 0x5D:
			indexOffset := uint32(cpu6502OffX)
			if instr.opcode == 0x19 || instr.opcode == 0x39 || instr.opcode == 0x59 {
				indexOffset = cpu6502OffY
			}
			op := p65WasmOra
			if instr.opcode == 0x39 || instr.opcode == 0x3D {
				op = p65WasmAnd
			} else if instr.opcode == 0x59 || instr.opcode == 0x5D {
				op = p65WasmEor
			}
			p65WasmEmitAbsIndexedAddressWithCross(&body, instr.operand, indexOffset)
			p65WasmDynamicDirectGuard(&body)
			body.localGet(p65WasmLCtx)
			body.i32Load(2, p65WasmCtxOffMemPtr)
			body.localGet(p65WasmLAddr)
			body.op(wasmOpI32Add)
			body.i32Load8U(0, 0)
			body.localSet(p65WasmLVal)
			p65WasmEmitLogicLoadedOperand(&body, op)
			body.localGet(p65WasmLCross)
			body.op(wasmOpI32Eqz)
			body.ifVoid()
			p65WasmEmitReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			body.elseBranch()
			p65WasmEmitReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode])+1)
			body.end()
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x29:
			p65WasmEmitLogicImmediate(&body, byte(instr.operand), p65WasmAnd)
		case 0x09:
			p65WasmEmitLogicImmediate(&body, byte(instr.operand), p65WasmOra)
		case 0x49:
			p65WasmEmitLogicImmediate(&body, byte(instr.operand), p65WasmEor)
		case 0x05, 0x0D, 0x25, 0x2D, 0x45, 0x4D:
			op := p65WasmOra
			if instr.opcode == 0x25 || instr.opcode == 0x2D {
				op = p65WasmAnd
			} else if instr.opcode == 0x45 || instr.opcode == 0x4D {
				op = p65WasmEor
			}
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			body.localGet(p65WasmLCtx)
			body.i32Load(2, p65WasmCtxOffMemPtr)
			body.i32Const(int32(instr.operand))
			body.op(wasmOpI32Add)
			body.i32Load8U(0, 0)
			body.localSet(p65WasmLVal)
			p65WasmEmitLogicLoadedOperand(&body, op)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xC9:
			p65WasmEmitCompareImmediate(&body, byte(instr.operand), cpu6502OffA)
		case 0xE0:
			p65WasmEmitCompareImmediate(&body, byte(instr.operand), cpu6502OffX)
		case 0xC0:
			p65WasmEmitCompareImmediate(&body, byte(instr.operand), cpu6502OffY)
		case 0xC5, 0xCD:
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitCompareDirect(&body, instr.operand, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xD5:
			p65WasmDirectGuard(&body, 0)
			body.localGet(p65WasmLCPU)
			body.i32Load8U(0, cpu6502OffX)
			body.i32Const(int32(byte(instr.operand)))
			body.op(wasmOpI32Add)
			body.i32Const(0xFF)
			body.op(wasmOpI32And)
			body.localSet(p65WasmLAddr)
			body.localGet(p65WasmLCtx)
			body.i32Load(2, p65WasmCtxOffMemPtr)
			body.localGet(p65WasmLAddr)
			body.op(wasmOpI32Add)
			body.i32Load8U(0, 0)
			body.localSet(p65WasmLVal)
			p65WasmEmitCompareLoadedOperand(&body, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xE4, 0xEC:
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitCompareDirect(&body, instr.operand, cpu6502OffX)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xC4, 0xCC:
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitCompareDirect(&body, instr.operand, cpu6502OffY)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xA5, 0xAD: // LDA zp, abs
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitDirectLoad(&body, instr.operand, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xA6, 0xAE: // LDX zp, abs
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitDirectLoad(&body, instr.operand, cpu6502OffX)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xA4, 0xAC: // LDY zp, abs
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitDirectLoad(&body, instr.operand, cpu6502OffY)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x85, 0x8D: // STA zp, abs
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitDirectStore(&body, instr.operand, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x86, 0x8E: // STX zp, abs
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitDirectStore(&body, instr.operand, cpu6502OffX)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x84, 0x8C: // STY zp, abs
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitDirectStore(&body, instr.operand, cpu6502OffY)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x9D: // STA abs,X
			p65WasmEmitAbsIndexedAddress(&body, instr.operand, cpu6502OffX)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitStoreAtAddr(&body, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x99: // STA abs,Y
			p65WasmEmitAbsIndexedAddress(&body, instr.operand, cpu6502OffY)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitStoreAtAddr(&body, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x81: // STA (zp,X)
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitIndirectStoreAddress(&body, byte(instr.operand), true)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitStoreAtAddr(&body, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x91: // STA (zp),Y
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitIndirectStoreAddress(&body, byte(instr.operand), false)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitStoreAtAddr(&body, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xA1: // LDA (zp,X)
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitIndirectStoreAddress(&body, byte(instr.operand), true)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitLoadAtAddr(&body, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xB1: // LDA (zp),Y
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitIndirectStoreAddress(&body, byte(instr.operand), false)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitLoadAtAddr(&body, cpu6502OffA)
			p65WasmEmitCrossReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x01, 0x11, 0x21, 0x31, 0x41, 0x51:
			indexedX := instr.opcode == 0x01 || instr.opcode == 0x21 || instr.opcode == 0x41
			op := p65WasmOra
			if instr.opcode == 0x21 || instr.opcode == 0x31 {
				op = p65WasmAnd
			} else if instr.opcode == 0x41 || instr.opcode == 0x51 {
				op = p65WasmEor
			}
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitIndirectStoreAddress(&body, byte(instr.operand), indexedX)
			p65WasmDynamicDirectGuard(&body)
			body.localGet(p65WasmLCtx)
			body.i32Load(2, p65WasmCtxOffMemPtr)
			body.localGet(p65WasmLAddr)
			body.op(wasmOpI32Add)
			body.i32Load8U(0, 0)
			body.localSet(p65WasmLVal)
			p65WasmEmitLogicLoadedOperand(&body, op)
			if !indexedX {
				p65WasmEmitCrossReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			}
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x61, 0x71, 0xE1, 0xF1:
			indexedX := instr.opcode == 0x61 || instr.opcode == 0xE1
			decimalTableOffset, binaryTableOffset := uint32(p65WasmCtxOffDecimalADC), uint32(p65WasmCtxOffBinaryADC)
			if instr.opcode == 0xE1 || instr.opcode == 0xF1 {
				decimalTableOffset, binaryTableOffset = p65WasmCtxOffDecimalSBC, p65WasmCtxOffBinarySBC
			}
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitIndirectStoreAddress(&body, byte(instr.operand), indexedX)
			p65WasmDynamicDirectGuard(&body)
			body.localGet(p65WasmLCtx)
			body.i32Load(2, p65WasmCtxOffMemPtr)
			body.localGet(p65WasmLAddr)
			body.op(wasmOpI32Add)
			body.i32Load8U(0, 0)
			body.localSet(p65WasmLVal)
			p65WasmEmitArithmeticLoadedOperand(&body, decimalTableOffset, binaryTableOffset)
			if !indexedX {
				p65WasmEmitCrossReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			}
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xC1, 0xD1:
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitIndirectStoreAddress(&body, byte(instr.operand), instr.opcode == 0xC1)
			p65WasmDynamicDirectGuard(&body)
			body.localGet(p65WasmLCtx)
			body.i32Load(2, p65WasmCtxOffMemPtr)
			body.localGet(p65WasmLAddr)
			body.op(wasmOpI32Add)
			body.i32Load8U(0, 0)
			body.localSet(p65WasmLVal)
			p65WasmEmitCompareLoadedOperand(&body, cpu6502OffA)
			if instr.opcode == 0xD1 {
				p65WasmEmitCrossReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			}
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xBD, 0xB9, 0xBE, 0xBC: // abs-indexed loads
			var indexOffset, destOffset uint32 = cpu6502OffX, cpu6502OffA
			switch instr.opcode {
			case 0xB9:
				indexOffset = cpu6502OffY
			case 0xBE:
				indexOffset, destOffset = cpu6502OffY, cpu6502OffX
			case 0xBC:
				destOffset = cpu6502OffY
			}
			p65WasmEmitAbsIndexedAddressWithCross(&body, instr.operand, indexOffset)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitLoadAtAddr(&body, destOffset)
			body.localGet(p65WasmLCross)
			body.op(wasmOpI32Eqz)
			body.ifVoid()
			p65WasmEmitReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			body.elseBranch()
			p65WasmEmitReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode])+1)
			body.end()
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xDD, 0xD9: // CMP abs,X / CMP abs,Y
			indexOffset := uint32(cpu6502OffX)
			if instr.opcode == 0xD9 {
				indexOffset = cpu6502OffY
			}
			p65WasmEmitAbsIndexedAddressWithCross(&body, instr.operand, indexOffset)
			p65WasmDynamicDirectGuard(&body)
			body.localGet(p65WasmLCtx)
			body.i32Load(2, p65WasmCtxOffMemPtr)
			body.localGet(p65WasmLAddr)
			body.op(wasmOpI32Add)
			body.i32Load8U(0, 0)
			body.localSet(p65WasmLVal)
			p65WasmEmitCompareLoadedOperand(&body, cpu6502OffA)
			body.localGet(p65WasmLCross)
			body.op(wasmOpI32Eqz)
			body.ifVoid()
			p65WasmEmitReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode]))
			body.elseBranch()
			p65WasmEmitReturn(&body, uint16(instrPC)+uint16(instr.length), uint32(index+1), cycles+uint32(jit6502BaseCycles[instr.opcode])+1)
			body.end()
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xB5: // LDA zp,X
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedLoad(&body, byte(instr.operand), cpu6502OffX, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xB6: // LDX zp,Y
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedLoad(&body, byte(instr.operand), cpu6502OffY, cpu6502OffX)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xB4: // LDY zp,X
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedLoad(&body, byte(instr.operand), cpu6502OffX, cpu6502OffY)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x35:
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedLogic(&body, byte(instr.operand), p65WasmAnd)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x15:
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedLogic(&body, byte(instr.operand), p65WasmOra)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x55:
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedLogic(&body, byte(instr.operand), p65WasmEor)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x95: // STA zp,X
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedStore(&body, byte(instr.operand), cpu6502OffX, cpu6502OffA)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x94: // STY zp,X
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedStore(&body, byte(instr.operand), cpu6502OffX, cpu6502OffY)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x96: // STX zp,Y
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedStore(&body, byte(instr.operand), cpu6502OffY, cpu6502OffX)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x24, 0x2C: // BIT zp, abs
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitBITDirect(&body, instr.operand)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xE6, 0xEE: // INC zp, abs
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitRMWDirect(&body, instr.operand, false)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xC6, 0xCE: // DEC zp, abs
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitRMWDirect(&body, instr.operand, true)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xF6: // INC zp,X
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedRMW(&body, byte(instr.operand), false)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xD6: // DEC zp,X
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedRMW(&body, byte(instr.operand), true)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xFE: // INC abs,X
			p65WasmEmitAbsIndexedAddress(&body, instr.operand, cpu6502OffX)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitAbsIndexedRMW(&body, instr.operand, false)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xDE: // DEC abs,X
			p65WasmEmitAbsIndexedAddress(&body, instr.operand, cpu6502OffX)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitAbsIndexedRMW(&body, instr.operand, true)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x1E, 0x3E, 0x5E, 0x7E: // ASL, ROL, LSR, ROR abs,X
			p65WasmEmitAbsIndexedAddress(&body, instr.operand, cpu6502OffX)
			p65WasmDynamicDirectGuard(&body)
			p65WasmEmitAbsIndexedRMWShift(&body, instr.operand, instr.opcode)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x16, 0x36, 0x56, 0x76: // ASL, ROL, LSR, ROR zp,X
			p65WasmDirectGuard(&body, 0)
			p65WasmEmitZPIndexedRMWShift(&body, byte(instr.operand), instr.opcode)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0xAA:
			p65WasmEmitTransfer(&body, cpu6502OffA, cpu6502OffX)
		case 0xA8:
			p65WasmEmitTransfer(&body, cpu6502OffA, cpu6502OffY)
		case 0x8A:
			p65WasmEmitTransfer(&body, cpu6502OffX, cpu6502OffA)
		case 0x98:
			p65WasmEmitTransfer(&body, cpu6502OffY, cpu6502OffA)
		case 0xBA:
			p65WasmEmitTransfer(&body, cpu6502OffSP, cpu6502OffX)
		case 0x9A:
			p65WasmEmitTransferNoFlags(&body, cpu6502OffX, cpu6502OffSP)
		case 0xE8:
			p65WasmEmitIncDec(&body, cpu6502OffX, false)
		case 0xC8:
			p65WasmEmitIncDec(&body, cpu6502OffY, false)
		case 0xCA:
			p65WasmEmitIncDec(&body, cpu6502OffX, true)
		case 0x88:
			p65WasmEmitIncDec(&body, cpu6502OffY, true)
		case 0x0A, 0x4A, 0x2A, 0x6A:
			p65WasmEmitAccumulatorShift(&body, instr.opcode)
		case 0x06, 0x0E, 0x46, 0x4E, 0x26, 0x2E, 0x66, 0x6E:
			p65WasmDirectGuard(&body, byte(instr.operand>>8))
			p65WasmEmitRMWShiftDirect(&body, instr.operand, instr.opcode)
			p65WasmDirectGuardEnd(&body, uint16(instrPC), uint32(index), cycles)
		case 0x18:
			p65WasmEmitFlag(&body, CARRY_FLAG, false)
		case 0x38:
			p65WasmEmitFlag(&body, CARRY_FLAG, true)
		case 0x58:
			p65WasmEmitFlag(&body, INTERRUPT_FLAG, false)
		case 0x78:
			p65WasmEmitFlag(&body, INTERRUPT_FLAG, true)
		case 0xB8:
			p65WasmEmitFlag(&body, OVERFLOW_FLAG, false)
		case 0xD8:
			p65WasmEmitFlag(&body, DECIMAL_FLAG, false)
		case 0xF8:
			p65WasmEmitFlag(&body, DECIMAL_FLAG, true)
		default:
			return nil, fmt.Errorf("6502 wasm: unsupported opcode %02X", instr.opcode)
		}
		pc = nextPC
		cycles += uint32(jit6502BaseCycles[instr.opcode])
	}

	// Publish the same return triple as native JIT backends.
	body.localGet(p65WasmLCtx)
	body.i32Const(int32(pc))
	body.i32Store(2, p65WasmCtxOffRetPC)
	body.localGet(p65WasmLCtx)
	body.i32Const(int32(len(instrs)))
	body.i32Store(2, p65WasmCtxOffRetCount)
	body.localGet(p65WasmLCtx)
	body.i64Const(int64(cycles))
	body.i64Store(3, p65WasmCtxOffRetCycles)
	body.end()

	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	typeIdx := m.addType([]byte{wasmTypeI32}, nil)
	fn := m.addFunc(typeIdx, []byte{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32}, body.code)
	m.exportFunc("block", fn)
	return m.build(), nil
}
