package main

import (
	"fmt"
	"unsafe"
)

// compileIE32WasmBlock emits block(cpu i32) against Go's imported linear
// memory. cpu is the wasm32 address of CPU, so this uses the same fixed CPU
// layout as the native backends without a host callback.
func compileIE32WasmBlock(block []ie32DecodedInstruction) ([]byte, error) {
	return compileIE32WasmBlockAtStack(block, STACK_START)
}

func compileIE32WasmBlockAtStack(block []ie32DecodedInstruction, initialStack uint32) ([]byte, error) {
	if len(block) == 0 {
		return nil, fmt.Errorf("empty IE32 wasm block")
	}
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 0)
	typ := m.addType([]byte{wasmTypeI32}, nil)
	var b wasmBody
	terminated := false
	stackCursor := initialStack
	live := ie32RegisterLiveness(block)
emit:
	for index, in := range block {
		if ie32ElideDeadImmediateLoadWithLiveness(block, live, index) {
			continue
		}
		switch in.Opcode {
		case NOP:
		case LDA:
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.A)), in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				emitIE32WasmCopyReg(&b, ie32WasmRegisterOffset(in.operandRegisterIndex()), uint32(unsafe.Offsetof(CPU{}.A)))
			} else if in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND {
				emitIE32WasmLoadRAMToReg(&b, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand, uint32(unsafe.Offsetof(CPU{}.A)))
			} else {
				return nil, fmt.Errorf("LDA mode")
			}
		case LOAD:
			off := ie32WasmRegisterOffset(in.registerIndex())
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32WasmStoreImm(&b, off, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				emitIE32WasmCopyReg(&b, ie32WasmRegisterOffset(in.operandRegisterIndex()), off)
			} else if in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND {
				emitIE32WasmLoadRAMToReg(&b, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand, off)
			} else {
				return nil, fmt.Errorf("LOAD mode")
			}
		case STORE:
			if in.AddrMode != ADDR_DIRECT && in.AddrMode != ADDR_REGISTER && in.AddrMode != ADDR_IMMEDIATE {
				return nil, fmt.Errorf("STORE mode")
			}
			emitIE32WasmStoreRegToRAM(&b, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand, ie32WasmRegisterOffset(in.registerIndex()))
		case LDX, LDY, LDZ, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW:
			off, ok := ie32WasmNamedLoadOffset(in.Opcode)
			if !ok {
				return nil, fmt.Errorf("named load")
			}
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32WasmStoreImm(&b, off, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				emitIE32WasmCopyReg(&b, ie32WasmRegisterOffset(in.operandRegisterIndex()), off)
			} else if in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND {
				emitIE32WasmLoadRAMToReg(&b, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand, off)
			} else {
				return nil, fmt.Errorf("named load mode")
			}
		case STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW:
			if in.AddrMode != ADDR_DIRECT && in.AddrMode != ADDR_REGISTER && in.AddrMode != ADDR_IMMEDIATE {
				return nil, fmt.Errorf("named store mode")
			}
			off, ok := ie32WasmNamedStoreOffset(in.Opcode)
			if !ok {
				return nil, fmt.Errorf("named store")
			}
			emitIE32WasmStoreRegToRAM(&b, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand, off)
		case ADD, SUB, AND, OR, XOR, MUL:
			if in.AddrMode == ADDR_IMMEDIATE {
				if in.residentALU {
					emitIE32WasmResidentALUImm(&b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, in.Operand, in.residentALUStart, in.residentALUEnd)
				} else {
					emitIE32WasmALUImm(&b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, in.Operand)
				}
			} else if in.AddrMode == ADDR_REGISTER {
				emitIE32WasmALUReg(&b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, ie32WasmRegisterOffset(in.operandRegisterIndex()))
			} else if in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND {
				emitIE32WasmALURAM(&b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand)
			} else {
				return nil, fmt.Errorf("ALU mode")
			}
		case DIV, MOD:
			if in.AddrMode == ADDR_IMMEDIATE && in.Operand != 0 {
				emitIE32WasmALUImm(&b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				emitIE32WasmALUReg(&b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, ie32WasmRegisterOffset(in.operandRegisterIndex()))
			} else if in.AddrMode == ADDR_DIRECT {
				emitIE32WasmALURAM(&b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand)
			} else {
				return nil, fmt.Errorf("divide mode")
			}
		case INC, DEC:
			op := byte(ADD)
			if in.Opcode == DEC {
				op = SUB
			}
			if in.AddrMode == ADDR_REGISTER {
				emitIE32WasmALUImm(&b, ie32WasmRegisterOffset(in.operandRegisterIndex()), op, 1)
			} else if in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND {
				emitIE32WasmRMWRAM(&b, op, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand)
			} else {
				return nil, fmt.Errorf("INC/DEC mode")
			}
		case PUSH:
			if stackCursor < STACK_BOTTOM+WORD_SIZE {
				return nil, fmt.Errorf("push stack")
			}
			stackCursor -= WORD_SIZE
			emitIE32WasmStoreRegToRAM(&b, uint32(unsafe.Offsetof(CPU{}.memory)), stackCursor, ie32WasmRegisterOffset(in.registerIndex()))
			emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.SP)), stackCursor)
		case POP:
			if stackCursor >= STACK_START {
				return nil, fmt.Errorf("pop stack")
			}
			emitIE32WasmLoadRAMToReg(&b, uint32(unsafe.Offsetof(CPU{}.memory)), stackCursor, ie32WasmRegisterOffset(in.registerIndex()))
			stackCursor += WORD_SIZE
			emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.SP)), stackCursor)
		case JSR:
			if in.fusedLeafCall {
				continue
			}
			if stackCursor < STACK_BOTTOM+WORD_SIZE {
				return nil, fmt.Errorf("jsr stack")
			}
			stackCursor -= WORD_SIZE
			emitIE32WasmStoreImmToRAM(&b, uint32(unsafe.Offsetof(CPU{}.memory)), stackCursor, in.PC+INSTRUCTION_SIZE)
			emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.SP)), stackCursor)
			emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.PC)), in.Operand)
			terminated = true
			break emit
		case RTS, RTI:
			if in.fusedLeafReturn {
				continue
			}
			if stackCursor >= STACK_START {
				return nil, fmt.Errorf("rts stack")
			}
			emitIE32WasmLoadRAMToReg(&b, uint32(unsafe.Offsetof(CPU{}.memory)), stackCursor, uint32(unsafe.Offsetof(CPU{}.PC)))
			stackCursor += WORD_SIZE
			emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.SP)), stackCursor)
			if in.Opcode == RTI {
				emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.inInterrupt)), 0)
			}
			terminated = true
			break emit
		case JMP:
			if in.chasedJump {
				continue
			}
			emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.PC)), in.Operand)
			terminated = true
			break emit
		case JNZ, JZ, JGT, JGE, JLT, JLE:
			if in.knownBranch {
				target := in.PC + INSTRUCTION_SIZE
				if in.branchTaken {
					target = in.Operand
				}
				emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.PC)), target)
				terminated = true
				break emit
			}
			if in.residentALUBranch {
				emitIE32WasmResidentBranchPC(&b, uint32(unsafe.Offsetof(CPU{}.PC)), ie32WasmRegisterOffset(in.registerIndex()), in.Operand, in.PC+INSTRUCTION_SIZE, in.Opcode)
			} else {
				emitIE32WasmBranchPC(&b, uint32(unsafe.Offsetof(CPU{}.PC)), ie32WasmRegisterOffset(in.registerIndex()), in.Operand, in.PC+INSTRUCTION_SIZE, in.Opcode)
			}
			terminated = true
			break emit
		case SEI, CLI:
			value := uint32(0)
			if in.Opcode == SEI {
				value = 1
			}
			emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.interruptEnabled)), value)
		case NOT:
			emitIE32WasmALUImm(&b, ie32WasmRegisterOffset(in.registerIndex()), XOR, ^uint32(0))
		case SHL, SHR:
			if in.AddrMode == ADDR_IMMEDIATE && in.Operand < 32 {
				emitIE32WasmALUImm(&b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				emitIE32WasmALUReg(&b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, ie32WasmRegisterOffset(in.operandRegisterIndex()))
			} else {
				return nil, fmt.Errorf("shift mode")
			}
		default:
			return nil, fmt.Errorf("unsupported IE32 wasm opcode %#x", in.Opcode)
		}
	}
	if !terminated {
		emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.PC)), ie32BlockNextPC(block, len(block)))
	}
	b.end()
	idx := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32}, b.code)
	m.exportFunc("block", idx)
	return m.build(), nil
}

func compileIE32WasmCountedLoopBlockAtStack(block []ie32DecodedInstruction, plan *ie32CountedLoopPlan, initialStack uint32) ([]byte, error) {
	_ = initialStack // counted loops are register-only and do not touch guest SP
	if plan == nil || plan.head < 0 || plan.back >= len(block) || plan.head >= plan.back {
		return nil, fmt.Errorf("invalid IE32 wasm counted loop")
	}
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 0)
	typ := m.addType([]byte{wasmTypeI32}, nil)
	var b wasmBody
	for i := 0; i < plan.head; i++ {
		if !emitIE32WasmCountedLoopPrefixInstruction(&b, block[i]) {
			return nil, fmt.Errorf("unsupported IE32 wasm loop prefix")
		}
	}
	locals, localCount := ie32WasmCountedLoopLocals(block, plan)
	for reg, local := range locals {
		if local == 0 {
			continue
		}
		emitIE32WasmLoadCPURegToLocal(&b, ie32WasmRegisterOffset(byte(reg)), local)
	}
	b.loop()
	for i := plan.head; i < plan.back; i++ {
		if !emitIE32WasmCountedLoopInstruction(&b, block[i], locals) {
			return nil, fmt.Errorf("unsupported IE32 wasm loop body")
		}
	}
	// The terminal JNZ is represented by a structured back-edge after its
	// preceding SUB has written the counter.
	b.localGet(locals[plan.counter])
	b.brIf(0)
	b.end()
	for reg, local := range locals {
		if local == 0 {
			continue
		}
		emitIE32WasmStoreLocalToCPUReg(&b, local, ie32WasmRegisterOffset(byte(reg)))
	}
	emitIE32WasmStoreImm(&b, uint32(unsafe.Offsetof(CPU{}.PC)), ie32BlockNextPC(block, len(block)))
	b.end()
	functionLocals := make([]byte, 2+localCount)
	for i := range functionLocals {
		functionLocals[i] = wasmTypeI32
	}
	idx := m.addFunc(typ, functionLocals, b.code)
	m.exportFunc("block", idx)
	return m.build(), nil
}

func emitIE32WasmCountedLoopPrefixInstruction(b *wasmBody, in ie32DecodedInstruction) bool {
	switch in.Opcode {
	case NOP:
		return true
	case LOAD:
		if in.AddrMode == ADDR_IMMEDIATE {
			emitIE32WasmStoreImm(b, ie32WasmRegisterOffset(in.registerIndex()), in.Operand)
			return true
		}
		if in.AddrMode == ADDR_DIRECT {
			emitIE32WasmLoadRAMToReg(b, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand, ie32WasmRegisterOffset(in.registerIndex()))
			return true
		}
	case STORE:
		if in.AddrMode == ADDR_DIRECT {
			emitIE32WasmStoreRegToRAM(b, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand, ie32WasmRegisterOffset(in.registerIndex()))
			return true
		}
	case ADD, SUB, AND, OR, XOR, MUL:
		if in.AddrMode == ADDR_IMMEDIATE {
			emitIE32WasmALUImm(b, ie32WasmRegisterOffset(in.registerIndex()), in.Opcode, in.Operand)
			return true
		}
	case JSR:
		return in.fusedLeafCall
	case RTS:
		return in.fusedLeafReturn
	}
	return false
}

// ie32WasmCountedLoopLocals allocates wasm locals for every architectural
// register touched inside the loop. Locals 1 and 2 remain the shared scratch
// registers used by the existing RAM helpers; loop locals start at 3.
func ie32WasmCountedLoopLocals(block []ie32DecodedInstruction, plan *ie32CountedLoopPlan) ([16]uint32, int) {
	var locals [16]uint32
	if plan == nil {
		return locals, 0
	}
	next := uint32(3)
	for i := plan.head; i < plan.back; i++ {
		in := block[i]
		if in.Opcode == NOP || (in.Opcode == JSR && in.fusedLeafCall) || (in.Opcode == RTS && in.fusedLeafReturn) {
			continue
		}
		reg := in.registerIndex()
		if locals[reg] == 0 {
			locals[reg] = next
			next++
		}
	}
	if locals[plan.counter] == 0 {
		locals[plan.counter] = next
		next++
	}
	return locals, int(next - 3)
}

func emitIE32WasmCountedLoopInstruction(b *wasmBody, in ie32DecodedInstruction, locals [16]uint32) bool {
	local := locals[in.registerIndex()]
	if local == 0 {
		return false
	}
	switch in.Opcode {
	case NOP:
		return true
	case JSR:
		return in.fusedLeafCall
	case RTS:
		return in.fusedLeafReturn
	case LOAD:
		if in.AddrMode == ADDR_IMMEDIATE {
			b.i32Const(int32(in.Operand))
			b.localSet(local)
		} else if in.AddrMode == ADDR_DIRECT {
			emitIE32WasmLoadRAMToLocal(b, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand, local)
		} else {
			return false
		}
		return true
	case STORE:
		if in.AddrMode != ADDR_DIRECT {
			return false
		}
		emitIE32WasmStoreLocalToRAM(b, uint32(unsafe.Offsetof(CPU{}.memory)), in.Operand, local)
		return true
	case ADD, SUB, AND, OR, XOR, MUL:
		if in.AddrMode != ADDR_IMMEDIATE {
			return false
		}
		emitIE32WasmLocalALUImm(b, local, in.Opcode, in.Operand)
		return true
	default:
		return false
	}
}

func emitIE32WasmLoadCPURegToLocal(b *wasmBody, offset, local uint32) {
	b.localGet(0)
	b.i32Const(int32(offset))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.localSet(local)
}

func emitIE32WasmStoreLocalToCPUReg(b *wasmBody, local, offset uint32) {
	b.localGet(0)
	b.i32Const(int32(offset))
	b.op(wasmOpI32Add)
	b.localGet(local)
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmLoadRAMToLocal(b *wasmBody, memoryFieldOff, addr, local uint32) {
	b.localGet(0)
	b.i32Const(int32(memoryFieldOff))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.i32Const(int32(addr))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.localSet(local)
}

func emitIE32WasmStoreLocalToRAM(b *wasmBody, memoryFieldOff, addr, local uint32) {
	b.localGet(0)
	b.i32Const(int32(memoryFieldOff))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.i32Const(int32(addr))
	b.op(wasmOpI32Add)
	b.localGet(local)
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmLocalALUImm(b *wasmBody, local uint32, opcode byte, value uint32) {
	b.localGet(local)
	b.i32Const(int32(value))
	switch opcode {
	case ADD:
		b.op(wasmOpI32Add)
	case SUB:
		b.op(wasmOpI32Sub)
	case AND:
		b.op(wasmOpI32And)
	case OR:
		b.op(wasmOpI32Or)
	case XOR:
		b.op(wasmOpI32Xor)
	case MUL:
		b.op(wasmOpI32Mul)
	}
	b.localSet(local)
}

func emitIE32WasmLoadRAMToReg(b *wasmBody, memoryFieldOff, addr, regOff uint32) {
	b.localGet(0)
	b.i32Const(int32(memoryFieldOff))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.i32Const(int32(addr))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.localSet(2)
	b.localGet(0)
	b.i32Const(int32(regOff))
	b.op(wasmOpI32Add)
	b.localGet(2)
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmCopyReg(b *wasmBody, from, to uint32) {
	b.localGet(0)
	b.i32Const(int32(from))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.localSet(2)
	b.localGet(0)
	b.i32Const(int32(to))
	b.op(wasmOpI32Add)
	b.localGet(2)
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmStoreRegToRAM(b *wasmBody, memoryFieldOff, addr, regOff uint32) {
	b.localGet(0)
	b.i32Const(int32(memoryFieldOff))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.i32Const(int32(addr))
	b.op(wasmOpI32Add)
	b.localGet(0)
	b.i32Const(int32(regOff))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmStoreImmToRAM(b *wasmBody, memoryFieldOff, addr, value uint32) {
	b.localGet(0)
	b.i32Const(int32(memoryFieldOff))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.i32Const(int32(addr))
	b.op(wasmOpI32Add)
	b.i32Const(int32(value))
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmBranchPC(b *wasmBody, pcOff, regOff, target, fall uint32, opcode byte) {
	b.localGet(0)
	b.i32Const(int32(pcOff))
	b.op(wasmOpI32Add)
	b.i32Const(int32(target))
	b.i32Const(int32(fall))
	b.localGet(0)
	b.i32Const(int32(regOff))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	switch opcode {
	case JZ:
		b.op(wasmOpI32Eqz)
	case JGT, JGE, JLT, JLE:
		b.i32Const(0)
		switch opcode {
		case JGT:
			b.op(wasmOpI32GtS)
		case JGE:
			b.op(wasmOpI32GeS)
		case JLT:
			b.op(wasmOpI32LtS)
		case JLE:
			b.op(wasmOpI32LeS)
		}
	}
	b.op(wasmOpSelect)
	b.memOp(wasmOpI32Store, 2, 0)
}

// emitIE32WasmResidentBranchPC reuses local 2 from an immediate-ALU run for
// the branch comparison while preserving the final register spill.
func emitIE32WasmResidentBranchPC(b *wasmBody, pcOff, regOff, target, fall uint32, opcode byte) {
	b.localGet(0)
	b.i32Const(int32(regOff))
	b.op(wasmOpI32Add)
	b.localGet(2)
	b.memOp(wasmOpI32Store, 2, 0)
	b.localGet(0)
	b.i32Const(int32(pcOff))
	b.op(wasmOpI32Add)
	b.i32Const(int32(target))
	b.i32Const(int32(fall))
	b.localGet(2)
	switch opcode {
	case JZ:
		b.op(wasmOpI32Eqz)
	case JGT, JGE, JLT, JLE:
		b.i32Const(0)
		switch opcode {
		case JGT:
			b.op(wasmOpI32GtS)
		case JGE:
			b.op(wasmOpI32GeS)
		case JLT:
			b.op(wasmOpI32LtS)
		case JLE:
			b.op(wasmOpI32LeS)
		}
	}
	b.op(wasmOpSelect)
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmALUImm(b *wasmBody, offset uint32, opcode byte, value uint32) {
	// local 1 is the register address and local 2 holds the computed value,
	// keeping the wasm store operand order explicit.
	b.localGet(0)
	b.i32Const(int32(offset))
	b.op(wasmOpI32Add)
	b.localSet(1)
	b.localGet(1)
	b.memOp(wasmOpI32Load, 2, 0)
	b.i32Const(int32(value))
	switch opcode {
	case ADD:
		b.op(wasmOpI32Add)
	case SUB:
		b.op(wasmOpI32Sub)
	case AND:
		b.op(wasmOpI32And)
	case OR:
		b.op(wasmOpI32Or)
	case XOR:
		b.op(wasmOpI32Xor)
	case MUL:
		b.op(wasmOpI32Mul)
	case DIV:
		b.op(wasmOpI32DivU)
	case MOD:
		b.op(wasmOpI32RemU)
	case SHL:
		b.op(wasmOpI32Shl)
	case SHR:
		b.op(wasmOpI32ShrU)
	}
	b.localSet(2)
	b.localGet(1)
	b.localGet(2)
	b.memOp(wasmOpI32Store, 2, 0)
}

// emitIE32WasmResidentALUImm keeps one immediate-ALU run in local 2. Local 1
// is the fixed guest-register address, so only the final instruction writes
// back to CPU memory.
func emitIE32WasmResidentALUImm(b *wasmBody, offset uint32, opcode byte, value uint32, start, end bool) {
	if start {
		b.localGet(0)
		b.i32Const(int32(offset))
		b.op(wasmOpI32Add)
		b.localSet(1)
		b.localGet(1)
		b.memOp(wasmOpI32Load, 2, 0)
		b.localSet(2)
	}
	b.localGet(2)
	b.i32Const(int32(value))
	switch opcode {
	case ADD:
		b.op(wasmOpI32Add)
	case SUB:
		b.op(wasmOpI32Sub)
	case AND:
		b.op(wasmOpI32And)
	case OR:
		b.op(wasmOpI32Or)
	case XOR:
		b.op(wasmOpI32Xor)
	case MUL:
		b.op(wasmOpI32Mul)
	}
	b.localSet(2)
	if end {
		b.localGet(1)
		b.localGet(2)
		b.memOp(wasmOpI32Store, 2, 0)
	}
}

func emitIE32WasmALUReg(b *wasmBody, offset uint32, opcode byte, source uint32) {
	b.localGet(0)
	b.i32Const(int32(offset))
	b.op(wasmOpI32Add)
	b.localSet(1)
	b.localGet(1)
	b.memOp(wasmOpI32Load, 2, 0)
	b.localGet(0)
	b.i32Const(int32(source))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	switch opcode {
	case ADD:
		b.op(wasmOpI32Add)
	case SUB:
		b.op(wasmOpI32Sub)
	case AND:
		b.op(wasmOpI32And)
	case OR:
		b.op(wasmOpI32Or)
	case XOR:
		b.op(wasmOpI32Xor)
	case MUL:
		b.op(wasmOpI32Mul)
	case DIV:
		b.op(wasmOpI32DivU)
	case MOD:
		b.op(wasmOpI32RemU)
	}
	b.localSet(2)
	b.localGet(1)
	b.localGet(2)
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmALURAM(b *wasmBody, offset uint32, opcode byte, memoryFieldOff, addr uint32) {
	b.localGet(0)
	b.i32Const(int32(offset))
	b.op(wasmOpI32Add)
	b.localSet(1)
	b.localGet(1)
	b.memOp(wasmOpI32Load, 2, 0)
	b.localGet(0)
	b.i32Const(int32(memoryFieldOff))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.i32Const(int32(addr))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	switch opcode {
	case ADD:
		b.op(wasmOpI32Add)
	case SUB:
		b.op(wasmOpI32Sub)
	case AND:
		b.op(wasmOpI32And)
	case OR:
		b.op(wasmOpI32Or)
	case XOR:
		b.op(wasmOpI32Xor)
	case MUL:
		b.op(wasmOpI32Mul)
	}
	b.localSet(2)
	b.localGet(1)
	b.localGet(2)
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmRMWRAM(b *wasmBody, opcode byte, memoryFieldOff, addr uint32) {
	b.localGet(0)
	b.i32Const(int32(memoryFieldOff))
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load, 2, 0)
	b.i32Const(int32(addr))
	b.op(wasmOpI32Add)
	b.localSet(1)
	b.localGet(1)
	b.memOp(wasmOpI32Load, 2, 0)
	b.i32Const(1)
	if opcode == ADD {
		b.op(wasmOpI32Add)
	} else {
		b.op(wasmOpI32Sub)
	}
	b.localSet(2)
	b.localGet(1)
	b.localGet(2)
	b.memOp(wasmOpI32Store, 2, 0)
}

func emitIE32WasmStoreImm(b *wasmBody, offset, value uint32) {
	b.localGet(0)
	b.i32Const(int32(offset))
	b.op(wasmOpI32Add)
	b.i32Const(int32(value))
	b.memOp(wasmOpI32Store, 2, 0)
}

func ie32WasmRegisterOffset(reg byte) uint32 {
	offsets := [...]uintptr{unsafe.Offsetof(CPU{}.A), unsafe.Offsetof(CPU{}.X), unsafe.Offsetof(CPU{}.Y), unsafe.Offsetof(CPU{}.Z), unsafe.Offsetof(CPU{}.B), unsafe.Offsetof(CPU{}.C), unsafe.Offsetof(CPU{}.D), unsafe.Offsetof(CPU{}.E), unsafe.Offsetof(CPU{}.F), unsafe.Offsetof(CPU{}.G), unsafe.Offsetof(CPU{}.H), unsafe.Offsetof(CPU{}.S), unsafe.Offsetof(CPU{}.T), unsafe.Offsetof(CPU{}.U), unsafe.Offsetof(CPU{}.V), unsafe.Offsetof(CPU{}.W)}
	return uint32(offsets[reg&REG_INDEX_MASK])
}

func ie32WasmNamedLoadOffset(opcode byte) (uint32, bool) {
	registers := map[byte]byte{LDX: 1, LDY: 2, LDZ: 3, LDB: 4, LDC: 5, LDD: 6, LDE: 7, LDF: 8, LDG: 9, LDH: 10, LDS: 11, LDT: 12, LDU: 13, LDV: 14, LDW: 15}
	reg, ok := registers[opcode]
	return ie32WasmRegisterOffset(reg), ok
}

func ie32WasmNamedStoreOffset(opcode byte) (uint32, bool) {
	registers := map[byte]byte{STA: 0, STX: 1, STY: 2, STZ: 3, STB: 4, STC: 5, STD: 6, STE: 7, STF: 8, STG: 9, STH: 10, STS: 11, STT: 12, STU: 13, STV: 14, STW: 15}
	reg, ok := registers[opcode]
	return ie32WasmRegisterOffset(reg), ok
}
