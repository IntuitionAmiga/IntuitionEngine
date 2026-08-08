//go:build linux && amd64

package main

import "unsafe"

// ie32JITTryRunDirect lowers a small, side-effect-free x64 block. Memory,
// timing and control-flow instructions remain at the architectural dispatcher
// boundary until their dedicated lowerers are present.
func ie32JITTryRunDirect(cpu *CPU) uint64 {
	if cpu == nil || cpu.jit == nil || cpu.jit.execMem == nil {
		return 0
	}
	if cached, ok := cpu.takeIE32ReturnCache(); ok {
		callNative(cached.addr, uintptr(unsafe.Pointer(cpu)))
		cpu.jit.nativeEntries.Add(1)
		cpu.jit.instructions.Add(cached.retired)
		cpu.jit.directInstructions.Add(cached.retired)
		cpu.jit.cacheHits.Add(1)
		cpu.jit.residentSpillsSaved.Add(cached.residentSpillsSaved)
		return cached.retired
	}
	if !cpu.timerEnabled.Load() && cpu.jit.testStopAfter == 0 && cpu.jit.nativeCache != nil {
		if cached, ok := cpu.jit.nativeCache[cpu.PC]; ok {
			if cached.stamp == ie32CachedSourceStamp(cpu.memory, cached) {
				callNative(cached.addr, uintptr(unsafe.Pointer(cpu)))
				cpu.jit.nativeEntries.Add(1)
				cpu.jit.instructions.Add(cached.retired)
				cpu.jit.directInstructions.Add(cached.retired)
				cpu.jit.cacheHits.Add(1)
				cpu.jit.residentSpillsSaved.Add(cached.residentSpillsSaved)
				return cached.retired
			}
			cpu.dropIE32NativeCodeCache()
			cpu.jit.deoptimizations.Add(1)
			cpu.jit.sourceStampDeopts.Add(1)
			return 0
		}
	}
	limit := 0
	if cpu.jit.testStopAfter > cpu.jit.testRetired {
		limit = int(cpu.jit.testStopAfter - cpu.jit.testRetired)
	}
	if cpu.timerEnabled.Load() && (limit == 0 || limit > 1) {
		limit = 1
	}
	regionTier := cpu.ie32JITShouldCompileRegion(cpu.PC)
	block := scanIE32FusedBlock(cpu.memory, cpu.PC, limit)
	if regionTier && !ie32BlockHasLeafFusion(block) {
		block = scanIE32Region(cpu.memory, cpu.PC, limit)
	}
	block = ie32FoldImmediateALU(block)
	block = ie32AnnotateKnownBranches(block)
	block = ie32AnnotateResidentImmediateALU(block)
	block = ie32SpecialiseKnownConstantRegisterAddresses(block)
	if cpu.jit.testStopAfter == 0 {
		if plan := ie32AnalyseCountedLoop(block); plan != nil {
			if retired := cpu.ie32JITTryRunCountedLoop(block, plan); retired != 0 {
				return retired
			}
		}
	}
	live := ie32RegisterLiveness(block)
	code := make([]byte, 0, 32)
	retired := 0
	terminated := false
	stackCursor := cpu.SP
	for index, in := range block {
		if ie32ElideDeadImmediateLoadWithLiveness(block, live, index) {
			retired++
			continue
		}
		specialised, ok := ie32SpecialiseFirstIndirect(cpu, in, retired == 0)
		if !ok {
			goto done
		}
		in = specialised
		kind, ok := ie32FormLowering[ie32OpcodeForm{Opcode: in.Opcode, AddrMode: in.AddrMode}]
		if !ok || kind != ie32LoweringDirect {
			goto done
		}
		switch in.Opcode {
		case NOP:
			// No guest state besides PC changes.
		case LDA:
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.A)), in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				src, _ := ie32AMD64RegisterOffset(in.operandRegisterIndex())
				emitIE32AMD64LoadEAX(&code, src)
				emitIE32AMD64StoreEAX(&code, uint32(unsafe.Offsetof(CPU{}.A)))
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				emitIE32AMD64LoadEAXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), addr)
				emitIE32AMD64StoreEAX(&code, uint32(unsafe.Offsetof(CPU{}.A)))
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) {
				emitIE32AMD64LoadEAXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
				emitIE32AMD64StoreEAX(&code, uint32(unsafe.Offsetof(CPU{}.A)))
			} else {
				goto done
			}
		case LOAD:
			off, ok := ie32AMD64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32AMD64StoreImm32(&code, off, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				src, _ := ie32AMD64RegisterOffset(in.operandRegisterIndex())
				emitIE32AMD64LoadEAX(&code, src)
				emitIE32AMD64StoreEAX(&code, off)
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				emitIE32AMD64LoadEAXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), addr)
				emitIE32AMD64StoreEAX(&code, off)
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) {
				emitIE32AMD64LoadEAXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
				emitIE32AMD64StoreEAX(&code, off)
			} else {
				goto done
			}
		case STORE:
			if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				in.AddrMode, in.Operand = ADDR_DIRECT, addr
			}
			if in.AddrMode == ADDR_MEM_IND && retired == 0 {
				addr, ok := ie32StaticMemoryIndirectStoreAddress(cpu, in)
				if !ok {
					goto done
				}
				in.AddrMode, in.Operand = ADDR_DIRECT, addr
			}
			if (in.AddrMode != ADDR_DIRECT && in.AddrMode != ADDR_REGISTER && in.AddrMode != ADDR_IMMEDIATE) || !ie32CanDirectRAMWrite(cpu, in.Operand) || ie32WriteMutatesRemainingBlock(in, block) {
				goto done
			}
			off, ok := ie32AMD64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			emitIE32AMD64LoadEAX(&code, off)
			emitIE32AMD64StoreEAXToRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
		case LDX, LDY, LDZ, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW:
			off, ok := ie32AMD64NamedLoadOffset(in.Opcode)
			if !ok {
				goto done
			}
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32AMD64StoreImm32(&code, off, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				src, _ := ie32AMD64RegisterOffset(in.operandRegisterIndex())
				emitIE32AMD64LoadEAX(&code, src)
				emitIE32AMD64StoreEAX(&code, off)
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				emitIE32AMD64LoadEAXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), addr)
				emitIE32AMD64StoreEAX(&code, off)
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) {
				emitIE32AMD64LoadEAXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
				emitIE32AMD64StoreEAX(&code, off)
			} else {
				goto done
			}
		case STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW:
			if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				in.AddrMode, in.Operand = ADDR_DIRECT, addr
			}
			if in.AddrMode == ADDR_MEM_IND && retired == 0 {
				addr, ok := ie32StaticMemoryIndirectStoreAddress(cpu, in)
				if !ok {
					goto done
				}
				in.AddrMode, in.Operand = ADDR_DIRECT, addr
			}
			if (in.AddrMode != ADDR_DIRECT && in.AddrMode != ADDR_REGISTER && in.AddrMode != ADDR_IMMEDIATE) || !ie32CanDirectRAMWrite(cpu, in.Operand) || ie32WriteMutatesRemainingBlock(in, block) {
				goto done
			}
			off, ok := ie32AMD64NamedStoreOffset(in.Opcode)
			if !ok {
				goto done
			}
			emitIE32AMD64LoadEAX(&code, off)
			emitIE32AMD64StoreEAXToRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
		case ADD, SUB, AND, OR, XOR, MUL:
			off, ok := ie32AMD64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			if !in.residentALU || in.residentALUStart {
				emitIE32AMD64LoadEAX(&code, off)
			}
			if in.AddrMode == ADDR_REGISTER {
				src, _ := ie32AMD64RegisterOffset(in.operandRegisterIndex())
				code = append(code, 0x8B, 0x97, byte(src), byte(src>>8), byte(src>>16), byte(src>>24))
				ops := map[byte][]byte{ADD: {0x01, 0xD0}, SUB: {0x29, 0xD0}, AND: {0x21, 0xD0}, OR: {0x09, 0xD0}, XOR: {0x31, 0xD0}, MUL: {0x0F, 0xAF, 0xC2}}
				code = append(code, ops[in.Opcode]...)
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				emitIE32AMD64LoadEDXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), addr)
				ops := map[byte][]byte{ADD: {0x01, 0xD0}, SUB: {0x29, 0xD0}, AND: {0x21, 0xD0}, OR: {0x09, 0xD0}, XOR: {0x31, 0xD0}, MUL: {0x0F, 0xAF, 0xC2}}
				code = append(code, ops[in.Opcode]...)
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) {
				emitIE32AMD64LoadEDXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
				ops := map[byte][]byte{ADD: {0x01, 0xD0}, SUB: {0x29, 0xD0}, AND: {0x21, 0xD0}, OR: {0x09, 0xD0}, XOR: {0x31, 0xD0}, MUL: {0x0F, 0xAF, 0xC2}}
				code = append(code, ops[in.Opcode]...)
			} else if in.AddrMode != ADDR_IMMEDIATE {
				goto done
			} else if in.Opcode == MUL {
				// imul eax, eax, imm32
				code = append(code, 0x69, 0xC0, byte(in.Operand), byte(in.Operand>>8), byte(in.Operand>>16), byte(in.Operand>>24))
			} else {
				emitIE32AMD64ALUImm32(&code, in.Opcode, in.Operand)
			}
			if !in.residentALU || in.residentALUEnd {
				emitIE32AMD64StoreEAX(&code, off)
			}
		case DIV, MOD:
			off, ok := ie32AMD64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			emitIE32AMD64LoadEAX(&code, off)
			code = append(code, 0x31, 0xD2) // xor edx, edx
			if in.AddrMode == ADDR_IMMEDIATE && in.Operand != 0 {
				code = append(code, 0xB9, byte(in.Operand), byte(in.Operand>>8), byte(in.Operand>>16), byte(in.Operand>>24))
			} else if in.AddrMode == ADDR_REGISTER && retired == 0 && *cpu.getRegister(in.operandRegisterIndex()) != 0 {
				src, _ := ie32AMD64RegisterOffset(in.operandRegisterIndex())
				code = append(code, 0x8B, 0x8F, byte(src), byte(src>>8), byte(src>>16), byte(src>>24))
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok || cpu.Read32(addr) == 0 {
					goto done
				}
				emitIE32AMD64LoadECXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), addr)
			} else if in.AddrMode == ADDR_DIRECT && ie32CanDirectRAMRead(cpu, in.Operand) && cpu.Read32(in.Operand) != 0 {
				emitIE32AMD64LoadECXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
			} else {
				goto done
			}
			code = append(code, 0xF7, 0xF1) // div ecx
			if in.Opcode == DIV {
				emitIE32AMD64StoreEAX(&code, off)
			} else {
				emitIE32AMD64StoreEDX(&code, off)
			}
		case INC, DEC:
			if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok || !ie32CanDirectRAMWrite(cpu, addr) {
					goto done
				}
				in.AddrMode, in.Operand = ADDR_DIRECT, addr
			}
			if in.AddrMode == ADDR_MEM_IND && retired == 0 {
				addr, ok := ie32StaticMemoryIndirectStoreAddress(cpu, in)
				if !ok {
					goto done
				}
				in.AddrMode, in.Operand = ADDR_DIRECT, addr
			}
			if in.AddrMode == ADDR_REGISTER {
				off, ok := ie32AMD64RegisterOffset(in.operandRegisterIndex())
				if !ok {
					goto done
				}
				if in.Opcode == INC {
					code = append(code, 0xFF, 0x87)
				} else {
					code = append(code, 0xFF, 0x8F)
				}
				code = append(code, byte(off), byte(off>>8), byte(off>>16), byte(off>>24))
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) && ie32CanDirectRAMWrite(cpu, in.Operand) && !ie32WriteMutatesRemainingBlock(in, block) {
				emitIE32AMD64LoadEAXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
				emitIE32AMD64ALUImm32(&code, map[bool]byte{true: ADD, false: SUB}[in.Opcode == INC], 1)
				emitIE32AMD64StoreEAXToRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
			} else {
				goto done
			}
		case PUSH:
			if stackCursor < STACK_BOTTOM+WORD_SIZE || !ie32CanDirectRAMWrite(cpu, stackCursor-WORD_SIZE) {
				goto done
			}
			stackCursor -= WORD_SIZE
			off, _ := ie32AMD64RegisterOffset(in.registerIndex())
			emitIE32AMD64LoadEAX(&code, off)
			emitIE32AMD64StoreEAXToRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), stackCursor)
			emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.SP)), stackCursor)
		case POP:
			if stackCursor >= STACK_START || !ie32CanDirectRAMRead(cpu, stackCursor) {
				goto done
			}
			off, _ := ie32AMD64RegisterOffset(in.registerIndex())
			emitIE32AMD64LoadEAXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), stackCursor)
			emitIE32AMD64StoreEAX(&code, off)
			stackCursor += WORD_SIZE
			emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.SP)), stackCursor)
		case JSR:
			if in.fusedLeafCall {
				break
			}
			if stackCursor < STACK_BOTTOM+WORD_SIZE || !ie32CanDirectRAMWrite(cpu, stackCursor-WORD_SIZE) {
				goto done
			}
			stackCursor -= WORD_SIZE
			emitIE32AMD64StoreImmToRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), stackCursor, in.PC+INSTRUCTION_SIZE)
			emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.SP)), stackCursor)
			emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.PC)), in.Operand)
			retired++
			terminated = true
			goto done
		case RTS, RTI:
			if in.fusedLeafReturn {
				break
			}
			if stackCursor >= STACK_START || !ie32CanDirectRAMRead(cpu, stackCursor) {
				goto done
			}
			emitIE32AMD64LoadEAXFromRAM(&code, uint32(unsafe.Offsetof(CPU{}.memBase)), stackCursor)
			emitIE32AMD64StoreEAX(&code, uint32(unsafe.Offsetof(CPU{}.PC)))
			stackCursor += WORD_SIZE
			emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.SP)), stackCursor)
			if in.Opcode == RTI {
				emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.inInterrupt)), 0)
			}
			retired++
			terminated = true
			goto done
		case JMP:
			if in.chasedJump {
				retired++
				continue
			}
			emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.PC)), in.Operand)
			retired++
			terminated = true
			goto done
		case JNZ, JZ, JGT, JGE, JLT, JLE:
			if in.knownBranch {
				target := in.PC + INSTRUCTION_SIZE
				if in.branchTaken {
					target = in.Operand
				}
				emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.PC)), target)
				retired++
				terminated = true
				goto done
			}
			off, ok := ie32AMD64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			fall := in.PC + INSTRUCTION_SIZE
			if in.residentALUBranch {
				code = append(code, 0x85, 0xC0) // test eax,eax
			} else {
				code = append(code, 0x83, 0xBF, byte(off), byte(off>>8), byte(off>>16), byte(off>>24), 0)
			}
			code = append(code, 0xB8, byte(fall), byte(fall>>8), byte(fall>>16), byte(fall>>24))
			code = append(code, 0xB9, byte(in.Operand), byte(in.Operand>>8), byte(in.Operand>>16), byte(in.Operand>>24))
			cmov := map[byte]byte{JNZ: 0x45, JZ: 0x44, JGT: 0x4F, JGE: 0x4D, JLT: 0x4C, JLE: 0x4E}[in.Opcode]
			code = append(code, 0x0F, cmov, 0xC1)
			emitIE32AMD64StoreEAX(&code, uint32(unsafe.Offsetof(CPU{}.PC)))
			retired++
			terminated = true
			goto done
		case SEI, CLI:
			value := uint32(0)
			if in.Opcode == SEI {
				value = 1
			}
			emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.interruptEnabled)), value)
		case NOT:
			off, ok := ie32AMD64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			emitIE32AMD64LoadEAX(&code, off)
			code = append(code, 0xF7, 0xD0) // not eax
			emitIE32AMD64StoreEAX(&code, off)
		case SHL, SHR:
			off, ok := ie32AMD64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			emitIE32AMD64LoadEAX(&code, off)
			if in.AddrMode == ADDR_IMMEDIATE && in.Operand < 32 {
				if in.Opcode == SHL {
					code = append(code, 0xC1, 0xE0, byte(in.Operand))
				} else {
					code = append(code, 0xC1, 0xE8, byte(in.Operand))
				}
			} else if in.AddrMode == ADDR_REGISTER && retired == 0 && *cpu.getRegister(in.operandRegisterIndex()) < 32 {
				src, _ := ie32AMD64RegisterOffset(in.operandRegisterIndex())
				code = append(code, 0x8B, 0x8F, byte(src), byte(src>>8), byte(src>>16), byte(src>>24))
				if in.Opcode == SHL {
					code = append(code, 0xD3, 0xE0)
				} else {
					code = append(code, 0xD3, 0xE8)
				}
			} else {
				goto done
			}
			emitIE32AMD64StoreEAX(&code, off)
		default:
			goto done
		}
		retired++
		if retired >= len(block) {
			break
		}
	}
done:
	if retired == 0 {
		return 0
	}
	if !terminated {
		emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.PC)), ie32BlockNextPC(block, retired))
	}
	code = append(code, 0xC3)
	addr, ok := cpu.writeIE32NativeCode(code)
	if !ok {
		return 0
	}
	callNative(addr, uintptr(unsafe.Pointer(cpu)))
	residentSpillsSaved := ie32ResidentALUSpillsSaved(block, retired)
	cpu.jit.blocks.Add(1)
	if ie32BlockIsRegion(block, retired) {
		cpu.jit.regions.Add(1)
	}
	cpu.jit.nativeEntries.Add(1)
	cpu.jit.instructions.Add(uint64(retired))
	cpu.jit.directInstructions.Add(uint64(retired))
	cpu.jit.residentSpillsSaved.Add(residentSpillsSaved)
	if !cpu.timerEnabled.Load() && cpu.jit.testStopAfter == 0 && ie32CacheableNativeBlock(block, retired) && (!ie32BlockEndsInJMP(block, retired) || ie32BlockIsRegion(block, retired)) {
		if cpu.jit.nativeCache == nil {
			cpu.jit.nativeCache = make(map[uint32]ie32NativeCachedBlock)
		}
		cpu.jit.nativeCache[block[0].PC] = ie32NativeCachedBlock{
			pc:                  block[0].PC,
			addr:                addr,
			retired:             uint64(retired),
			residentSpillsSaved: residentSpillsSaved,
			stamp:               ie32DecodedBlockSourceStamp(cpu.memory, block, retired),
			sourceStart:         uint64(block[0].PC),
			sourceEnd:           uint64(ie32BlockNextPC(block, retired)),
			sourceRanges:        ie32DecodedBlockSourceRanges(block, retired),
		}
		cpu.rememberIE32ReturnCache(cpu.jit.nativeCache[block[0].PC])
	}
	return uint64(retired)
}

// ie32JITTryRunCountedLoop emits a bounded pure-register counted loop as one
// native call. R8D counts retired guest instructions and is returned in EAX,
// so the dispatcher preserves exact architectural accounting without a Go
// round-trip for every backward branch.
func (cpu *CPU) ie32JITTryRunCountedLoop(block []ie32DecodedInstruction, plan *ie32CountedLoopPlan) uint64 {
	if cpu == nil || cpu.jit == nil || cpu.jit.execMem == nil || plan == nil {
		return 0
	}
	count, ok := ie32CountedLoopInitialCount(cpu, block, plan)
	if !ok || count == 0 || count > 4096 || !ie32CountedLoopDirectMemoryAdmissible(cpu, block, plan) {
		return 0
	}
	code := make([]byte, 0, 96)
	code = append(code, 0x45, 0x31, 0xC0) // xor r8d,r8d
	loopStart := -1
	for i, in := range block {
		if i == plan.head {
			loopStart = len(code)
		}
		if i == plan.back {
			off, ok := ie32AMD64RegisterOffset(plan.counter)
			if !ok || loopStart < 0 {
				return 0
			}
			code = append(code, 0x41, 0xFF, 0xC0) // inc r8d, retiring JNZ
			code = append(code, 0x83, 0xBF, byte(off), byte(off>>8), byte(off>>16), byte(off>>24), 0)
			code = append(code, 0x0F, 0x85)
			dispAt := len(code)
			code = append(code, 0, 0, 0, 0)
			disp := loopStart - (dispAt + 4)
			code[dispAt] = byte(disp)
			code[dispAt+1] = byte(disp >> 8)
			code[dispAt+2] = byte(disp >> 16)
			code[dispAt+3] = byte(disp >> 24)
			continue
		}
		if !ie32EmitAMD64CountedLoopInstruction(&code, cpu, in) {
			return 0
		}
		code = append(code, 0x41, 0xFF, 0xC0) // inc r8d
	}
	emitIE32AMD64StoreImm32(&code, uint32(unsafe.Offsetof(CPU{}.PC)), ie32BlockNextPC(block, len(block)))
	code = append(code, 0x44, 0x89, 0xC0, 0xC3) // mov eax,r8d; ret
	addr, ok := cpu.writeIE32NativeCode(code)
	if !ok {
		return 0
	}
	retired := uint64(callNativeArgRet(addr, uintptr(unsafe.Pointer(cpu))))
	if retired == 0 {
		return 0
	}
	cpu.jit.blocks.Add(1)
	cpu.jit.nativeEntries.Add(1)
	cpu.jit.instructions.Add(retired)
	cpu.jit.directInstructions.Add(retired)
	cpu.jit.countedLoops.Add(1)
	return retired
}

func ie32EmitAMD64CountedLoopInstruction(code *[]byte, cpu *CPU, in ie32DecodedInstruction) bool {
	switch in.Opcode {
	case NOP:
		return true
	case JSR:
		return in.fusedLeafCall
	case RTS:
		return in.fusedLeafReturn
	case LOAD:
		off, ok := ie32AMD64RegisterOffset(in.registerIndex())
		if !ok {
			return false
		}
		if in.AddrMode == ADDR_IMMEDIATE {
			emitIE32AMD64StoreImm32(code, off, in.Operand)
		} else if in.AddrMode == ADDR_DIRECT && ie32CanDirectRAMRead(cpu, in.Operand) {
			emitIE32AMD64LoadEAXFromRAM(code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
			emitIE32AMD64StoreEAX(code, off)
		} else {
			return false
		}
		return true
	case STORE:
		if in.AddrMode != ADDR_DIRECT || !ie32CanDirectRAMWrite(cpu, in.Operand) {
			return false
		}
		off, ok := ie32AMD64RegisterOffset(in.registerIndex())
		if !ok {
			return false
		}
		emitIE32AMD64LoadEAX(code, off)
		emitIE32AMD64StoreEAXToRAM(code, uint32(unsafe.Offsetof(CPU{}.memBase)), in.Operand)
		return true
	case ADD, SUB, AND, OR, XOR, MUL:
		if in.AddrMode != ADDR_IMMEDIATE {
			return false
		}
		off, ok := ie32AMD64RegisterOffset(in.registerIndex())
		if !ok {
			return false
		}
		emitIE32AMD64LoadEAX(code, off)
		if in.Opcode == MUL {
			*code = append(*code, 0x69, 0xC0, byte(in.Operand), byte(in.Operand>>8), byte(in.Operand>>16), byte(in.Operand>>24))
		} else {
			emitIE32AMD64ALUImm32(code, in.Opcode, in.Operand)
		}
		emitIE32AMD64StoreEAX(code, off)
		return true
	default:
		return false
	}
}

func emitIE32AMD64LoadEAX(code *[]byte, offset uint32) {
	*code = append(*code, 0x8B, 0x87, byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24))
}

func emitIE32AMD64StoreEAX(code *[]byte, offset uint32) {
	*code = append(*code, 0x89, 0x87, byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24))
}

func emitIE32AMD64StoreEDX(code *[]byte, offset uint32) {
	*code = append(*code, 0x89, 0x97, byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24))
}

func emitIE32AMD64LoadEAXFromRAM(code *[]byte, memBaseOffset, addr uint32) {
	*code = append(*code, 0x48, 0x8B, 0x87, byte(memBaseOffset), byte(memBaseOffset>>8), byte(memBaseOffset>>16), byte(memBaseOffset>>24))
	*code = append(*code, 0x8B, 0x80, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24))
}

func emitIE32AMD64LoadEDXFromRAM(code *[]byte, memBaseOffset, addr uint32) {
	*code = append(*code, 0x48, 0x8B, 0x8F, byte(memBaseOffset), byte(memBaseOffset>>8), byte(memBaseOffset>>16), byte(memBaseOffset>>24))
	*code = append(*code, 0x8B, 0x91, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24))
}

func emitIE32AMD64LoadECXFromRAM(code *[]byte, memBaseOffset, addr uint32) {
	*code = append(*code, 0x48, 0x8B, 0x8F, byte(memBaseOffset), byte(memBaseOffset>>8), byte(memBaseOffset>>16), byte(memBaseOffset>>24))
	*code = append(*code, 0x8B, 0x89, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24))
}

func emitIE32AMD64StoreEAXToRAM(code *[]byte, memBaseOffset, addr uint32) {
	// mov rcx,[rdi+memBaseOffset]; mov [rcx+addr],eax
	*code = append(*code, 0x48, 0x8B, 0x8F, byte(memBaseOffset), byte(memBaseOffset>>8), byte(memBaseOffset>>16), byte(memBaseOffset>>24))
	*code = append(*code, 0x89, 0x81, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24))
}

func emitIE32AMD64StoreImmToRAM(code *[]byte, memBaseOffset, addr, value uint32) {
	*code = append(*code, 0x48, 0x8B, 0x8F, byte(memBaseOffset), byte(memBaseOffset>>8), byte(memBaseOffset>>16), byte(memBaseOffset>>24))
	*code = append(*code, 0xC7, 0x81, byte(addr), byte(addr>>8), byte(addr>>16), byte(addr>>24), byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}

func emitIE32AMD64ALUImm32(code *[]byte, opcode byte, value uint32) {
	var op byte
	switch opcode {
	case ADD:
		op = 0x05
	case SUB:
		op = 0x2D
	case AND:
		op = 0x25
	case OR:
		op = 0x0D
	case XOR:
		op = 0x35
	}
	*code = append(*code, op, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}

func emitIE32AMD64StoreImm32(code *[]byte, offset, value uint32) {
	// mov dword ptr [rdi+disp32], imm32
	*code = append(*code, 0xC7, 0x87,
		byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24),
		byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
}

func ie32AMD64RegisterOffset(reg byte) (uint32, bool) {
	offsets := [...]uintptr{
		unsafe.Offsetof(CPU{}.A), unsafe.Offsetof(CPU{}.X), unsafe.Offsetof(CPU{}.Y), unsafe.Offsetof(CPU{}.Z),
		unsafe.Offsetof(CPU{}.B), unsafe.Offsetof(CPU{}.C), unsafe.Offsetof(CPU{}.D), unsafe.Offsetof(CPU{}.E),
		unsafe.Offsetof(CPU{}.F), unsafe.Offsetof(CPU{}.G), unsafe.Offsetof(CPU{}.H), unsafe.Offsetof(CPU{}.S),
		unsafe.Offsetof(CPU{}.T), unsafe.Offsetof(CPU{}.U), unsafe.Offsetof(CPU{}.V), unsafe.Offsetof(CPU{}.W),
	}
	return uint32(offsets[reg&REG_INDEX_MASK]), true
}

func ie32AMD64NamedLoadOffset(opcode byte) (uint32, bool) {
	registers := map[byte]byte{
		LDX: 1, LDY: 2, LDZ: 3, LDB: 4, LDC: 5, LDD: 6, LDE: 7,
		LDF: 8, LDG: 9, LDH: 10, LDS: 11, LDT: 12, LDU: 13, LDV: 14, LDW: 15,
	}
	reg, ok := registers[opcode]
	if !ok {
		return 0, false
	}
	return ie32AMD64RegisterOffset(reg)
}

func ie32AMD64NamedStoreOffset(opcode byte) (uint32, bool) {
	registers := map[byte]byte{STA: 0, STX: 1, STY: 2, STZ: 3, STB: 4, STC: 5, STD: 6, STE: 7, STF: 8, STG: 9, STH: 10, STS: 11, STT: 12, STU: 13, STV: 14, STW: 15}
	reg, ok := registers[opcode]
	if !ok {
		return 0, false
	}
	return ie32AMD64RegisterOffset(reg)
}
