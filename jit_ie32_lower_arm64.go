//go:build linux && arm64

package main

import "unsafe"

// ie32JITTryRunDirect is the ARM64 counterpart of the x64 immediate/register
// subset. It uses X0 for the CPU pointer, as supplied by callNative.
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
	code := make([]byte, 0, 48)
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
		case LDA:
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32ARM64MovImm32(&code, 1, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				src, _ := ie32ARM64RegisterOffset(in.operandRegisterIndex())
				emitIE32ARM64LdrW(&code, 1, src)
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				emitIE32ARM64LoadWAtRAM(&code, 1, addr)
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) {
				emitIE32ARM64LoadWAtRAM(&code, 1, in.Operand)
			} else {
				goto done
			}
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.A)))
		case LOAD:
			off, ok := ie32ARM64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32ARM64MovImm32(&code, 1, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				src, _ := ie32ARM64RegisterOffset(in.operandRegisterIndex())
				emitIE32ARM64LdrW(&code, 1, src)
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				emitIE32ARM64LoadWAtRAM(&code, 1, addr)
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) {
				emitIE32ARM64LoadWAtRAM(&code, 1, in.Operand)
			} else {
				goto done
			}
			emitIE32ARM64StrW(&code, 1, off)
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
			off, ok := ie32ARM64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			emitIE32ARM64LdrW(&code, 1, off)
			emitIE32ARM64StoreWAtRAM(&code, 1, in.Operand)
		case LDX, LDY, LDZ, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW:
			off, ok := ie32ARM64NamedLoadOffset(in.Opcode)
			if !ok {
				goto done
			}
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32ARM64MovImm32(&code, 1, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				src, _ := ie32ARM64RegisterOffset(in.operandRegisterIndex())
				emitIE32ARM64LdrW(&code, 1, src)
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				emitIE32ARM64LoadWAtRAM(&code, 1, addr)
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) {
				emitIE32ARM64LoadWAtRAM(&code, 1, in.Operand)
			} else {
				goto done
			}
			emitIE32ARM64StrW(&code, 1, off)
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
			off, ok := ie32ARM64NamedStoreOffset(in.Opcode)
			if !ok {
				goto done
			}
			emitIE32ARM64LdrW(&code, 1, off)
			emitIE32ARM64StoreWAtRAM(&code, 1, in.Operand)
		case ADD, SUB, AND, OR, XOR, MUL:
			off, ok := ie32ARM64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			if !in.residentALU || in.residentALUStart {
				emitIE32ARM64LdrW(&code, 1, off)
			}
			if in.AddrMode == ADDR_IMMEDIATE {
				emitIE32ARM64MovImm32(&code, 2, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER {
				src, _ := ie32ARM64RegisterOffset(in.operandRegisterIndex())
				emitIE32ARM64LdrW(&code, 2, src)
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					goto done
				}
				emitIE32ARM64LoadWAtRAM(&code, 2, addr)
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) {
				emitIE32ARM64LoadWAtRAM(&code, 2, in.Operand)
			} else {
				goto done
			}
			emitIE32ARM64ALU(&code, in.Opcode)
			if !in.residentALU || in.residentALUEnd {
				emitIE32ARM64StrW(&code, 1, off)
			}
		case DIV, MOD:
			off, ok := ie32ARM64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			emitIE32ARM64LdrW(&code, 1, off)
			if in.AddrMode == ADDR_IMMEDIATE && in.Operand != 0 {
				emitIE32ARM64MovImm32(&code, 2, in.Operand)
			} else if in.AddrMode == ADDR_REGISTER && retired == 0 && *cpu.getRegister(in.operandRegisterIndex()) != 0 {
				src, _ := ie32ARM64RegisterOffset(in.operandRegisterIndex())
				emitIE32ARM64LdrW(&code, 2, src)
			} else if in.AddrMode == ADDR_REG_IND && retired == 0 {
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok || cpu.Read32(addr) == 0 {
					goto done
				}
				emitIE32ARM64LoadWAtRAM(&code, 2, addr)
			} else if in.AddrMode == ADDR_DIRECT && ie32CanDirectRAMRead(cpu, in.Operand) && cpu.Read32(in.Operand) != 0 {
				emitIE32ARM64LoadWAtRAM(&code, 2, in.Operand)
			} else {
				goto done
			}
			if in.Opcode == DIV {
				emitIE32ARM64(&code, 0x1AC20821) // udiv w1,w1,w2
			} else {
				emitIE32ARM64(&code, 0x1AC20823) // udiv w3,w1,w2
				emitIE32ARM64(&code, 0x1B028461) // msub w1,w3,w2,w1
			}
			emitIE32ARM64StrW(&code, 1, off)
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
				off, ok := ie32ARM64RegisterOffset(in.operandRegisterIndex())
				if !ok {
					goto done
				}
				emitIE32ARM64LdrW(&code, 1, off)
				if in.Opcode == INC {
					emitIE32ARM64(&code, 0x11000421)
				} else {
					emitIE32ARM64(&code, 0x51000421)
				}
				emitIE32ARM64StrW(&code, 1, off)
			} else if (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && ie32CanDirectRAMRead(cpu, in.Operand) && ie32CanDirectRAMWrite(cpu, in.Operand) && !ie32WriteMutatesRemainingBlock(in, block) {
				emitIE32ARM64LoadWAtRAM(&code, 1, in.Operand)
				if in.Opcode == INC {
					emitIE32ARM64(&code, 0x11000421)
				} else {
					emitIE32ARM64(&code, 0x51000421)
				}
				emitIE32ARM64StoreWAtRAM(&code, 1, in.Operand)
			} else {
				goto done
			}
		case PUSH:
			if stackCursor < STACK_BOTTOM+WORD_SIZE || !ie32CanDirectRAMWrite(cpu, stackCursor-WORD_SIZE) {
				goto done
			}
			stackCursor -= WORD_SIZE
			off, _ := ie32ARM64RegisterOffset(in.registerIndex())
			emitIE32ARM64LdrW(&code, 1, off)
			emitIE32ARM64StoreWAtRAM(&code, 1, stackCursor)
			emitIE32ARM64MovImm32(&code, 1, stackCursor)
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.SP)))
		case POP:
			if stackCursor >= STACK_START || !ie32CanDirectRAMRead(cpu, stackCursor) {
				goto done
			}
			off, _ := ie32ARM64RegisterOffset(in.registerIndex())
			emitIE32ARM64LoadWAtRAM(&code, 1, stackCursor)
			emitIE32ARM64StrW(&code, 1, off)
			stackCursor += WORD_SIZE
			emitIE32ARM64MovImm32(&code, 1, stackCursor)
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.SP)))
		case JSR:
			if in.fusedLeafCall {
				break
			}
			if stackCursor < STACK_BOTTOM+WORD_SIZE || !ie32CanDirectRAMWrite(cpu, stackCursor-WORD_SIZE) {
				goto done
			}
			stackCursor -= WORD_SIZE
			emitIE32ARM64MovImm32(&code, 1, in.PC+INSTRUCTION_SIZE)
			emitIE32ARM64StoreWAtRAM(&code, 1, stackCursor)
			emitIE32ARM64MovImm32(&code, 1, stackCursor)
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.SP)))
			emitIE32ARM64MovImm32(&code, 1, in.Operand)
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.PC)))
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
			emitIE32ARM64LoadWAtRAM(&code, 1, stackCursor)
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.PC)))
			stackCursor += WORD_SIZE
			emitIE32ARM64MovImm32(&code, 1, stackCursor)
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.SP)))
			if in.Opcode == RTI {
				emitIE32ARM64MovImm32(&code, 1, 0)
				emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.inInterrupt)))
			}
			retired++
			terminated = true
			goto done
		case JMP:
			if in.chasedJump {
				retired++
				continue
			}
			emitIE32ARM64MovImm32(&code, 1, in.Operand)
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.PC)))
			retired++
			terminated = true
			goto done
		case JNZ, JZ, JGT, JGE, JLT, JLE:
			if in.knownBranch {
				target := in.PC + INSTRUCTION_SIZE
				if in.branchTaken {
					target = in.Operand
				}
				emitIE32ARM64MovImm32(&code, 1, target)
				emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.PC)))
				retired++
				terminated = true
				goto done
			}
			off, ok := ie32ARM64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			if !in.residentALUBranch {
				emitIE32ARM64LdrW(&code, 1, off)
			}
			emitIE32ARM64MovImm32(&code, 2, in.Operand)
			emitIE32ARM64MovImm32(&code, 3, in.PC+INSTRUCTION_SIZE)
			emitIE32ARM64(&code, 0x7100003F) // cmp w1,#0
			cond := map[byte]uint32{JNZ: 1, JZ: 0, JGT: 12, JGE: 10, JLT: 11, JLE: 13}[in.Opcode]
			emitIE32ARM64(&code, 0x1A800000|(3<<16)|(cond<<12)|(2<<5)|1)
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.PC)))
			retired++
			terminated = true
			goto done
		case NOT:
			off, ok := ie32ARM64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			emitIE32ARM64LdrW(&code, 1, off)
			emitIE32ARM64(&code, 0x2A2103E1) // mvn w1,w1
			emitIE32ARM64StrW(&code, 1, off)
		case SHL, SHR:
			off, ok := ie32ARM64RegisterOffset(in.registerIndex())
			if !ok {
				goto done
			}
			emitIE32ARM64LdrW(&code, 1, off)
			if in.AddrMode == ADDR_IMMEDIATE && in.Operand < 32 && in.Opcode == SHL {
				shift := in.Operand
				emitIE32ARM64(&code, 0x53000000|((32-shift)&31)<<16|((31-shift)&31)<<10|(1<<5)|1)
			} else if in.AddrMode == ADDR_IMMEDIATE && in.Operand < 32 {
				emitIE32ARM64(&code, 0x53000000|(in.Operand<<16)|(31<<10)|(1<<5)|1)
			} else if in.AddrMode == ADDR_REGISTER && retired == 0 && *cpu.getRegister(in.operandRegisterIndex()) < 32 {
				src, _ := ie32ARM64RegisterOffset(in.operandRegisterIndex())
				emitIE32ARM64LdrW(&code, 2, src)
				if in.Opcode == SHL {
					emitIE32ARM64(&code, 0x1AC22021) // lslv w1,w1,w2
				} else {
					emitIE32ARM64(&code, 0x1AC22421) // lsrv w1,w1,w2
				}
			} else {
				goto done
			}
			emitIE32ARM64StrW(&code, 1, off)
		case SEI, CLI:
			value := uint32(0)
			if in.Opcode == SEI {
				value = 1
			}
			emitIE32ARM64MovImm32(&code, 1, value)
			emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.interruptEnabled)))
		default:
			goto done
		}
		retired++
	}
done:
	if retired == 0 {
		return 0
	}
	if !terminated {
		emitIE32ARM64MovImm32(&code, 1, ie32BlockNextPC(block, retired))
		emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.PC)))
	}
	emitIE32ARM64(&code, 0xD65F03C0) // ret
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

func (cpu *CPU) ie32JITTryRunCountedLoop(block []ie32DecodedInstruction, plan *ie32CountedLoopPlan) uint64 {
	if cpu == nil || cpu.jit == nil || cpu.jit.execMem == nil || plan == nil {
		return 0
	}
	count, ok := ie32CountedLoopInitialCount(cpu, block, plan)
	if !ok || count == 0 || count > 4096 || !ie32CountedLoopDirectMemoryAdmissible(cpu, block, plan) {
		return 0
	}
	code := make([]byte, 0, 96)
	emitIE32ARM64(&code, 0x52800009) // mov w9,#0
	loopStart := -1
	for i, in := range block {
		if i == plan.head {
			loopStart = len(code)
		}
		if i == plan.back {
			off, ok := ie32ARM64RegisterOffset(plan.counter)
			if !ok || loopStart < 0 {
				return 0
			}
			emitIE32ARM64(&code, 0x11000529) // add w9,w9,#1
			emitIE32ARM64LdrW(&code, 1, off)
			emitIE32ARM64(&code, 0x7100003F) // cmp w1,#0
			branchAt := len(code)
			disp := (loopStart - branchAt) / 4
			emitIE32ARM64(&code, 0x54000001|(uint32(disp)&0x7FFFF)<<5) // b.ne loop
			continue
		}
		if !ie32EmitARM64CountedLoopInstruction(&code, cpu, in) {
			return 0
		}
		emitIE32ARM64(&code, 0x11000529) // add w9,w9,#1
	}
	emitIE32ARM64MovImm32(&code, 1, ie32BlockNextPC(block, len(block)))
	emitIE32ARM64StrW(&code, 1, uint32(unsafe.Offsetof(CPU{}.PC)))
	emitIE32ARM64(&code, 0x2A0903E0) // mov w0,w9
	emitIE32ARM64(&code, 0xD65F03C0) // ret
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

func ie32EmitARM64CountedLoopInstruction(code *[]byte, cpu *CPU, in ie32DecodedInstruction) bool {
	switch in.Opcode {
	case NOP:
		return true
	case JSR:
		return in.fusedLeafCall
	case RTS:
		return in.fusedLeafReturn
	case LOAD:
		off, ok := ie32ARM64RegisterOffset(in.registerIndex())
		if !ok {
			return false
		}
		if in.AddrMode == ADDR_IMMEDIATE {
			emitIE32ARM64MovImm32(code, 1, in.Operand)
			emitIE32ARM64StrW(code, 1, off)
		} else if in.AddrMode == ADDR_DIRECT && ie32CanDirectRAMRead(cpu, in.Operand) {
			emitIE32ARM64LoadWAtRAM(code, 1, in.Operand)
			emitIE32ARM64StrW(code, 1, off)
		} else {
			return false
		}
		return true
	case STORE:
		if in.AddrMode != ADDR_DIRECT || !ie32CanDirectRAMWrite(cpu, in.Operand) {
			return false
		}
		off, ok := ie32ARM64RegisterOffset(in.registerIndex())
		if !ok {
			return false
		}
		emitIE32ARM64LdrW(code, 1, off)
		emitIE32ARM64StoreWAtRAM(code, 1, in.Operand)
		return true
	case ADD, SUB, AND, OR, XOR, MUL:
		if in.AddrMode != ADDR_IMMEDIATE {
			return false
		}
		off, ok := ie32ARM64RegisterOffset(in.registerIndex())
		if !ok {
			return false
		}
		emitIE32ARM64LdrW(code, 1, off)
		emitIE32ARM64MovImm32(code, 2, in.Operand)
		emitIE32ARM64ALU(code, in.Opcode)
		emitIE32ARM64StrW(code, 1, off)
		return true
	default:
		return false
	}
}

func emitIE32ARM64(code *[]byte, word uint32) {
	*code = append(*code, byte(word), byte(word>>8), byte(word>>16), byte(word>>24))
}

func emitIE32ARM64MovImm32(code *[]byte, reg byte, value uint32) {
	emitIE32ARM64(code, 0x52800000|uint32(reg)|((value&0xFFFF)<<5))
	if value>>16 != 0 {
		emitIE32ARM64(code, 0x72A00000|uint32(reg)|((value>>16)<<5))
	}
}

func emitIE32ARM64LdrW(code *[]byte, reg byte, offset uint32) {
	emitIE32ARM64(code, 0xB9400000|uint32(reg)|((offset/4)<<10))
}

func emitIE32ARM64LdrX(code *[]byte, reg byte, offset uint32) {
	emitIE32ARM64(code, 0xF9400000|uint32(reg)|((offset/8)<<10))
}
func emitIE32ARM64LdrWBase(code *[]byte, reg, base byte, offset uint32) {
	emitIE32ARM64(code, 0xB9400000|uint32(reg)|(uint32(base)<<5)|((offset/4)<<10))
}

func emitIE32ARM64StrWBase(code *[]byte, reg, base byte, offset uint32) {
	emitIE32ARM64(code, 0xB9000000|uint32(reg)|(uint32(base)<<5)|((offset/4)<<10))
}

func emitIE32ARM64StoreWAtRAM(code *[]byte, reg byte, addr uint32) {
	emitIE32ARM64LdrX(code, 3, uint32(unsafe.Offsetof(CPU{}.memBase)))
	emitIE32ARM64MovImm32(code, 4, addr)
	emitIE32ARM64(code, 0x8B040063)             // add x3,x3,x4
	emitIE32ARM64(code, 0xB9000060|uint32(reg)) // str w(reg),[x3]
}

func emitIE32ARM64LoadWAtRAM(code *[]byte, reg byte, addr uint32) {
	emitIE32ARM64LdrX(code, 3, uint32(unsafe.Offsetof(CPU{}.memBase)))
	emitIE32ARM64MovImm32(code, 4, addr)
	emitIE32ARM64(code, 0x8B040063)             // add x3,x3,x4
	emitIE32ARM64(code, 0xB9400060|uint32(reg)) // ldr w(reg),[x3]
}

func emitIE32ARM64StrW(code *[]byte, reg byte, offset uint32) {
	emitIE32ARM64(code, 0xB9000000|uint32(reg)|((offset/4)<<10))
}

func emitIE32ARM64ALU(code *[]byte, opcode byte) {
	base := uint32(0)
	switch opcode {
	case ADD:
		base = 0x0B000000
	case SUB:
		base = 0x4B000000
	case AND:
		base = 0x0A000000
	case OR:
		base = 0x2A000000
	case XOR:
		base = 0x4A000000
	case MUL:
		// mul w1, w1, w2 is madd w1,w1,w2,wzr.
		emitIE32ARM64(code, 0x1B027C21)
		return
	}
	emitIE32ARM64(code, base|(2<<16)|(1<<5)|1) // w1 = w1 op w2
}

func ie32ARM64RegisterOffset(reg byte) (uint32, bool) {
	offsets := [...]uintptr{
		unsafe.Offsetof(CPU{}.A), unsafe.Offsetof(CPU{}.X), unsafe.Offsetof(CPU{}.Y), unsafe.Offsetof(CPU{}.Z),
		unsafe.Offsetof(CPU{}.B), unsafe.Offsetof(CPU{}.C), unsafe.Offsetof(CPU{}.D), unsafe.Offsetof(CPU{}.E),
		unsafe.Offsetof(CPU{}.F), unsafe.Offsetof(CPU{}.G), unsafe.Offsetof(CPU{}.H), unsafe.Offsetof(CPU{}.S),
		unsafe.Offsetof(CPU{}.T), unsafe.Offsetof(CPU{}.U), unsafe.Offsetof(CPU{}.V), unsafe.Offsetof(CPU{}.W),
	}
	return uint32(offsets[reg&REG_INDEX_MASK]), true
}

func ie32ARM64NamedLoadOffset(opcode byte) (uint32, bool) {
	registers := map[byte]byte{
		LDX: 1, LDY: 2, LDZ: 3, LDB: 4, LDC: 5, LDD: 6, LDE: 7,
		LDF: 8, LDG: 9, LDH: 10, LDS: 11, LDT: 12, LDU: 13, LDV: 14, LDW: 15,
	}
	reg, ok := registers[opcode]
	if !ok {
		return 0, false
	}
	return ie32ARM64RegisterOffset(reg)
}

func ie32ARM64NamedStoreOffset(opcode byte) (uint32, bool) {
	registers := map[byte]byte{STA: 0, STX: 1, STY: 2, STZ: 3, STB: 4, STC: 5, STD: 6, STE: 7, STF: 8, STG: 9, STH: 10, STS: 11, STT: 12, STU: 13, STV: 14, STW: 15}
	reg, ok := registers[opcode]
	if !ok {
		return 0, false
	}
	return ie32ARM64RegisterOffset(reg)
}
