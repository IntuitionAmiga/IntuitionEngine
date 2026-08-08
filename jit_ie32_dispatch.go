package main

import "time"

// ExecuteJITIE32 is the IE32 JIT router. The frontend is introduced before
// individual lowering families so production construction and launch paths
// have one execution entry point throughout the TDD rollout.
func (cpu *CPU) ExecuteJITIE32() {
	// Keep the normal router's entry validation identical to the interpreter.
	// Coprocessor workers intentionally own a wider PC range.
	if !cpu.CoprocMode && (cpu.PC < PROG_START || cpu.PC >= STACK_START) {
		cpu.executeInterpreter()
		if cpu.jit != nil {
			cpu.jit.instructions.Add(cpu.InstructionCount)
		}
		return
	}
	cpu.InstructionCount = 0
	for cpu.running.Load() {
		cpu.drainIE32JITInvalidations()
		debugActive := cpu.debugAccess != nil && cpu.debugAccess.AnyActive(cpu.debugCPUID)
		if debugActive && cpu.debugHandleBreakIn(uint64(cpu.PC)) {
			break
		}
		if cpu.timerEnabled.Load() {
			cpu.advanceTimer()
			if !cpu.running.Load() {
				break
			}
		}
		if !debugActive && !cpu.timerEnabled.Load() && cpu.jit != nil && cpu.jit.testStopAfter == 0 {
			if retired := cpu.tryFastIE32MMIOPollLoop(); retired != 0 {
				cpu.InstructionCount += retired
				cpu.jit.instructions.Add(retired)
				cpu.jit.helperInstructions.Add(retired)
				cpu.jit.deoptimizations.Add(1)
				cpu.jit.helperDeopts.Add(1)
				continue
			}
		}
		retired := uint64(0)
		if !debugActive {
			ie32JITEnterGenerated(cpu)
			for chains := 0; chains < ie32JITChainBlockBudget; chains++ {
				blockPC := cpu.PC
				blockBefore := scanIE32Block(cpu.memory, blockPC, 0)
				n := ie32JITTryRunDirect(cpu)
				if n == 0 {
					break
				}
				wroteRAM := ie32DecodedBlockWrites(blockBefore, n)
				if wroteRAM {
					// Native and wasm stores use raw memory for their proven RAM
					// fast path. Publish after return, before another dispatcher
					// can reuse a stale generated block. Only statically resolved
					// direct destinations retain a range; dynamic addressing
					// deliberately invalidates every retained block.
					if ranges, exact := ie32DecodedBlockWriteRanges(blockBefore, n); exact {
						for _, r := range ranges {
							cpu.publishIE32JITWrite(r.addr, r.size)
						}
					} else {
						cpu.publishIE32JITWrite(0, 0)
					}
				}
				if chains != 0 {
					cpu.jit.chains.Add(1)
				}
				retired += n
				cpu.jit.returnCachePending = ie32DecodedBlockReturns(blockBefore, n)
				// Differential fixtures request an exact guest-instruction
				// checkpoint. A generated block already respects the remaining
				// limit, but a subsequent chained block could otherwise cross it
				// before ie32JITTestRetire observes the accumulated total.
				if cpu.jit.testExactRetirement {
					break
				}
				if chains+1 == ie32JITChainBlockBudget {
					cpu.jit.chainBudgetExits.Add(1)
				}
				if wroteRAM {
					// A static chain may otherwise enter a previously cached target
					// before the next dispatcher boundary observes this publication.
					break
				}
			}
		}
		if retired != 0 {
			cpu.InstructionCount += retired
			if cpu.ie32JITTestRetire(retired) {
				break
			}
			if cpu.InstructionCount&0xFFF == 0 {
				hostCooperativeYield()
			}
			continue
		}

		in, ok := decodeIE32Instruction(cpu.memory, cpu.PC)
		if !ok {
			cpu.running.Store(false)
			break
		}
		// WAIT is an unavoidable timing helper. StepOne deliberately does not
		// sleep for debugger use, so execute the timing effect here before its
		// one-instruction semantic helper updates architectural state.
		if in.Opcode == WAIT {
			if delay := ie32ResolveOperand(cpu, in); delay != 0 {
				time.Sleep(time.Duration(delay) * time.Microsecond)
			}
		}
		stepped := cpu.StepOne()
		// A successful decode is retired even when StepOne subsequently raises
		// an invalid-opcode fault, matching the full interpreter's pre-dispatch
		// accounting. A failed decode returned above without retirement.
		cpu.InstructionCount++
		if cpu.jit != nil {
			cpu.jit.instructions.Add(1)
			cpu.jit.helperInstructions.Add(1)
			cpu.jit.deoptimizations.Add(1)
			cpu.jit.helperDeopts.Add(1)
		}
		if cpu.ie32JITTestRetire(1) {
			break
		}
		if stepped == 0 {
			break
		}
		if cpu.InstructionCount&0xFFF == 0 {
			if !cpu.running.Load() {
				break
			}
			hostCooperativeYield()
		}
	}
}

const ie32MMIOPollIterationCap = 4096

// tryFastIE32MMIOPollLoop recognises LOAD r,[MMIO] followed by JZ/JNZ r back
// to the load. It executes each pair through StepOne, retaining MMIO read and
// branch semantics while avoiding repeated dispatcher and compilation work.
func (cpu *CPU) tryFastIE32MMIOPollLoop() uint64 {
	if cpu == nil || cpu.bus == nil || cpu.PC > uint32(len(cpu.memory)) {
		return 0
	}
	bus, ok := cpu.bus.(*MachineBus)
	if !ok {
		return 0
	}
	load, ok := decodeIE32Instruction(cpu.memory, cpu.PC)
	if !ok || load.Opcode != LOAD || load.AddrMode != ADDR_DIRECT || !bus.IsIOAddress(load.Operand) {
		return 0
	}
	branch, ok := decodeIE32Instruction(cpu.memory, cpu.PC+INSTRUCTION_SIZE)
	if !ok || (branch.Opcode != JZ && branch.Opcode != JNZ) || branch.registerIndex() != load.registerIndex() || branch.Operand != cpu.PC {
		return 0
	}
	var retired uint64
	for iterations := 0; iterations < ie32MMIOPollIterationCap && cpu.running.Load(); iterations++ {
		if cpu.StepOne() == 0 {
			break
		}
		if cpu.StepOne() == 0 {
			break
		}
		retired += 2
		cpu.jit.mmioPollIterations.Add(1)
		if cpu.PC != load.PC {
			break
		}
	}
	return retired
}

func ie32DecodedBlockReturns(block []ie32DecodedInstruction, count uint64) bool {
	if count == 0 || count > uint64(len(block)) {
		return false
	}
	last := block[count-1]
	return last.Opcode == RTS || last.Opcode == RTI
}

func ie32DecodedBlockWrites(block []ie32DecodedInstruction, count uint64) bool {
	if count > uint64(len(block)) {
		count = uint64(len(block))
	}
	for _, in := range block[:count] {
		switch in.Opcode {
		case STORE, STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW, INC, DEC:
			return true
		}
	}
	return false
}

// ie32DecodedBlockWriteRanges returns exact physical write ranges only when
// the source encoding fixes every generated write destination. It is called
// after generated execution; register- and memory-indirect destinations are
// therefore intentionally conservative whole-cache invalidations.
func ie32DecodedBlockWriteRanges(block []ie32DecodedInstruction, count uint64) ([]ie32JITInvalidation, bool) {
	if count > uint64(len(block)) {
		count = uint64(len(block))
	}
	ranges := make([]ie32JITInvalidation, 0, count)
	for _, in := range block[:count] {
		switch in.Opcode {
		case STORE, STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW:
			switch in.AddrMode {
			case ADDR_IMMEDIATE, ADDR_REGISTER, ADDR_DIRECT:
				ranges = append(ranges, ie32JITInvalidation{addr: uint64(in.Operand), size: WORD_SIZE})
			default:
				return nil, false
			}
		case INC, DEC:
			switch in.AddrMode {
			case ADDR_REGISTER:
				continue
			case ADDR_DIRECT:
				ranges = append(ranges, ie32JITInvalidation{addr: uint64(in.Operand), size: WORD_SIZE})
			default:
				return nil, false
			}
		}
	}
	return ranges, true
}

const ie32JITChainBlockBudget = 64

func ie32ResolveOperand(cpu *CPU, in ie32DecodedInstruction) uint32 {
	switch in.AddrMode {
	case ADDR_IMMEDIATE:
		return in.Operand
	case ADDR_REGISTER:
		return *cpu.regs[in.operandRegisterIndex()]
	case ADDR_REG_IND:
		return cpu.Read32(*cpu.regs[in.operandRegisterIndex()] + (in.Operand & ^uint32(REG_INDEX_MASK)))
	case ADDR_MEM_IND, ADDR_DIRECT:
		return cpu.Read32(in.Operand)
	default:
		return 0
	}
}
