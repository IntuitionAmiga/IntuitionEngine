package main

import (
	"runtime"
	"time"
)

// ie32MMIOPollYield gives bounded MMIO polling loops a scheduler boundary.
// Tests replace it to prove that the boundary is retained.
var ie32MMIOPollYield = runtime.Gosched

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
				if retired >= uint64(ie32MMIOPollIterationCap*2) && cpu.waitForIE32VBlankEdge() {
					// A complete VIDEO_STATUS wait batch saw no edge. Park on the
					// video device instead of abandoning JIT execution for the full
					// interpreter, then resume at the unchanged guest poll loop.
					cpu.jit.mmioPollParks.Add(1)
					continue
				}
				// The fast path can otherwise re-enter an unchanged VBlank poll
				// forever without giving the video refresh goroutine a turn.
				ie32MMIOPollYield()
				continue
			}
		}
		retired := uint64(0)
		if !debugActive {
			ie32JITEnterGenerated(cpu)
			for chains := 0; chains < ie32JITChainBlockBudget; chains++ {
				blockPC := cpu.PC
				if cpu.ie32JITShouldUseInterpreterForTransientFragment(blockPC) {
					break
				}
				cached, cachedHit := cpu.ie32NativeCacheCandidate(blockPC)
				var blockBefore []ie32DecodedInstruction
				var preparedWrites ie32PreparedWrites
				if !cachedHit {
					if first, ok := decodeIE32Instruction(cpu.memory, blockPC); !ok || ie32FirstInstructionNeedsHelper(cpu, first) {
						break
					}
					// The architectural timer advances before every instruction. A
					// generated block has no internal timer checkpoints, so retain a
					// one-instruction boundary while the timer is active.
					maxInstructions := 0
					if cpu.timerEnabled.Load() {
						maxInstructions = 1
					}
					blockBefore = scanIE32Block(cpu.memory, blockPC, maxInstructions)
					preparedWrites = ie32PrepareDecodedBlockWrites(cpu, blockBefore)
				}
				if cachedHit {
					cpu.noteIE32DispatchCacheHint(cached)
				}
				n := ie32JITTryRunDirect(cpu)
				if n == 0 {
					break
				}
				wroteRAM := false
				var writeRanges []ie32JITInvalidation
				exactWrites := false
				if cachedHit && n == cached.retired {
					writeRanges, exactWrites = cached.writeRanges, true
					wroteRAM = len(writeRanges) != 0
				} else {
					wroteRAM = ie32DecodedBlockWrites(blockBefore, n)
					writeRanges, exactWrites = preparedWrites.ranges(n)
				}
				if wroteRAM {
					// Native and wasm stores use raw memory for their proven RAM
					// fast path. Publish after return, before another dispatcher
					// can reuse a stale generated block. A first indirect store has
					// already resolved its admitted destination at block entry, so it
					// can publish the same exact range as a direct store.
					if exactWrites {
						for _, r := range writeRanges {
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
				if cachedHit && n == cached.retired {
					cpu.jit.returnCachePending = cached.returns
				} else {
					cpu.jit.returnCachePending = ie32DecodedBlockReturns(blockBefore, n)
				}
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
			if cpu.jit != nil && cpu.jit.resumeAfterHelper {
				cpu.jit.helperResumes.Add(1)
				cpu.jit.resumeAfterHelper = false
			}
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
		if !debugActive && cpu.tryFastIE32MMIOStore(in) {
			cpu.InstructionCount++
			if cpu.jit != nil {
				cpu.jit.instructions.Add(1)
				cpu.jit.helperInstructions.Add(1)
				cpu.jit.deoptimizations.Add(1)
				cpu.jit.helperDeopts.Add(1)
				cpu.jit.helperExits.Add(1)
				cpu.jit.mmioStoreHelpers.Add(1)
				cpu.jit.resumeAfterHelper = true
			}
			if cpu.ie32JITTestRetire(1) {
				break
			}
			continue
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
			// HALT terminates execution rather than yielding to an architectural
			// helper. Every other one-instruction fallback may resume a retained
			// direct fragment at the following dispatcher boundary.
			if in.Opcode != HALT {
				cpu.jit.helperExits.Add(1)
				cpu.jit.resumeAfterHelper = true
			}
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


// tryFastIE32MMIOStore executes a direct named-register store at the device
// boundary without the general StepOne opcode switch. CPU.Write32 still owns
// MMIO routing, device side effects, and access instrumentation, while the
// caller keeps timer and debugger boundaries in the normal dispatcher.
func (cpu *CPU) tryFastIE32MMIOStore(in ie32DecodedInstruction) bool {
	if cpu == nil || cpu.bus == nil || in.AddrMode != ADDR_DIRECT {
		return false
	}
	bus, ok := cpu.bus.(*MachineBus)
	if !ok || !bus.IsIOAddress(in.Operand) {
		return false
	}
	var value uint32
	switch in.Opcode {
	case STORE:
		value = *cpu.regs[in.registerIndex()]
	default:
		register, ok := ie32NamedStoreRegister(in.Opcode)
		if !ok {
			return false
		}
		value = *cpu.regs[register]
	}
	// StepOne resolves every operand before the opcode switch.  A direct store
	// therefore reads its MMIO operand before writing it, which matters for
	// devices whose reads acknowledge a status bit or consume FIFO data.
	cpu.Read32(in.Operand)
	cpu.Write32(in.Operand, value)
	cpu.PC += INSTRUCTION_SIZE
	return true
}

func (cpu *CPU) waitForIE32VBlankEdge() bool {
	if cpu == nil || cpu.bus == nil {
		return false
	}
	bus, ok := cpu.bus.(*MachineBus)
	if !ok {
		return false
	}
	load, ok := decodeIE32Instruction(cpu.memory, cpu.PC)
	if !ok || load.Opcode != LOAD || load.AddrMode != ADDR_DIRECT || load.Operand != VIDEO_STATUS {
		return false
	}
	branch, ok := decodeIE32Instruction(cpu.memory, cpu.PC+INSTRUCTION_SIZE)
	if !ok || branch.Opcode != JZ || branch.registerIndex() != load.registerIndex() || branch.Operand != cpu.PC {
		return false
	}
	return bus.WaitForVideoVBlankEdge(cpuWaitSafetyTimeout)
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
			// INC and DEC have both register and memory forms. Only the latter
			// publishes a RAM write and needs to end a generated block chain.
			if in.Opcode != INC && in.Opcode != DEC || in.AddrMode != ADDR_REGISTER {
				return true
			}
		}
	}
	return false
}

type ie32PreparedWriteRange struct {
	writes bool
	exact  bool
	range_ ie32JITInvalidation
}

type ie32PreparedWrites []ie32PreparedWriteRange

// ie32PrepareDecodedBlockWrites records each possible generated write before
// native execution changes guest registers or pointer slots. Direct lowering
// only admits a register- or memory-indirect write as the first instruction,
// so that entry state is sufficient for an exact publication range.
func ie32PrepareDecodedBlockWrites(cpu *CPU, block []ie32DecodedInstruction) ie32PreparedWrites {
	prepared := make(ie32PreparedWrites, len(block))
	for index, in := range block {
		switch in.Opcode {
		case STORE, STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW:
			prepared[index].writes = true
			switch in.AddrMode {
			case ADDR_IMMEDIATE, ADDR_REGISTER, ADDR_DIRECT:
				prepared[index].exact = true
				prepared[index].range_ = ie32JITInvalidation{addr: uint64(in.Operand), size: WORD_SIZE}
			case ADDR_REG_IND:
				if index == 0 {
					if addr, ok := ie32StaticRegisterIndirectAddress(cpu, in); ok && ie32CanDirectRAMWrite(cpu, addr) {
						prepared[index].exact = true
						prepared[index].range_ = ie32JITInvalidation{addr: uint64(addr), size: WORD_SIZE}
					}
				}
			case ADDR_MEM_IND:
				if index == 0 {
					if addr, ok := ie32StaticMemoryIndirectStoreAddress(cpu, in); ok {
						prepared[index].exact = true
						prepared[index].range_ = ie32JITInvalidation{addr: uint64(addr), size: WORD_SIZE}
					}
				}
			default:
				continue
			}
		case INC, DEC:
			prepared[index].writes = in.AddrMode != ADDR_REGISTER
			switch in.AddrMode {
			case ADDR_REGISTER:
				continue
			case ADDR_DIRECT:
				prepared[index].exact = true
				prepared[index].range_ = ie32JITInvalidation{addr: uint64(in.Operand), size: WORD_SIZE}
			case ADDR_REG_IND:
				if index == 0 {
					if addr, ok := ie32StaticRegisterIndirectAddress(cpu, in); ok && ie32CanDirectRAMWrite(cpu, addr) {
						prepared[index].exact = true
						prepared[index].range_ = ie32JITInvalidation{addr: uint64(addr), size: WORD_SIZE}
					}
				}
			case ADDR_MEM_IND:
				if index == 0 {
					if addr, ok := ie32StaticMemoryIndirectStoreAddress(cpu, in); ok {
						prepared[index].exact = true
						prepared[index].range_ = ie32JITInvalidation{addr: uint64(addr), size: WORD_SIZE}
					}
				}
			default:
				continue
			}
		}
	}
	return prepared
}

func (prepared ie32PreparedWrites) ranges(count uint64) ([]ie32JITInvalidation, bool) {
	if count > uint64(len(prepared)) {
		count = uint64(len(prepared))
	}
	ranges := make([]ie32JITInvalidation, 0, count)
	for _, write := range prepared[:count] {
		if !write.writes {
			continue
		}
		if !write.exact {
			return nil, false
		}
		ranges = append(ranges, write.range_)
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
	case ADDR_MEM_IND:
		return cpu.Read32(in.Operand)
	case ADDR_DIRECT:
		return cpu.Read32(in.Operand)
	default:
		return 0
	}
}
