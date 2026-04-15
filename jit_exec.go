// jit_exec.go - JIT dispatcher loop and CPU64 integration

//go:build (amd64 || arm64) && linux

package main

import (
	"fmt"
	"time"
	"unsafe"
)

// JIT configuration constants
const (
	jitExecMemSize = 16 * 1024 * 1024 // 16MB executable memory pool
)

// jitExecMem returns the typed *ExecMem from the cpu's any field.
func (cpu *CPU64) getJITExecMem() *ExecMem {
	if cpu.jitExecMem == nil {
		return nil
	}
	return cpu.jitExecMem.(*ExecMem)
}

// initJIT initializes JIT state on the CPU. Called once before execution.
func (cpu *CPU64) initJIT() error {
	if cpu.jitExecMem != nil {
		return nil // already initialized
	}
	execMem, err := AllocExecMem(jitExecMemSize)
	if err != nil {
		return fmt.Errorf("JIT init failed: %w", err)
	}
	cpu.jitExecMem = execMem
	cpu.jitCache = NewCodeCache()
	cpu.jitCtx = newJITContext(cpu)
	return nil
}

// freeJIT releases all JIT resources. If jitPersist is set (used by benchmarks),
// the code cache and exec memory are kept alive for reuse across runs.
func (cpu *CPU64) freeJIT() {
	if cpu.jitPersist {
		return
	}
	if em := cpu.getJITExecMem(); em != nil {
		em.Free()
		cpu.jitExecMem = nil
	}
	cpu.jitCache = nil
	cpu.jitCtx = nil
}

// compileBlockMMU wraps compileBlock, marking all guest-memory-touching
// instructions for interpreter bail. Used when MMU is enabled.
func compileBlockMMU(instrs []JITInstr, startPC uint32, execMem *ExecMem) (*JITBlock, error) {
	for i := range instrs {
		switch instrs[i].opcode {
		case OP_LOAD, OP_STORE, OP_FLOAD, OP_FSTORE, OP_DLOAD, OP_DSTORE,
			OP_JSR64, OP_RTS64, OP_PUSH64, OP_POP64, OP_JSR_IND,
			OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
			instrs[i].mmuBail = true
		}
	}
	return compileBlock(instrs, startPC, execMem)
}

// interpretOne executes one IE64 instruction at cpu.PC using the interpreter.
// Unlike StepOne(), this is designed to be called from within the JIT execution
// loop for instructions that can't be JIT-compiled (FPU, WAIT, HALT).
func (cpu *CPU64) interpretOne() {
	cpu.StepOne()
}

// ExecuteJIT is the main JIT execution loop. It replaces Execute() when
// JIT compilation is enabled.
func (cpu *CPU64) ExecuteJIT() {
	if !cpu.CoprocMode && (cpu.PC < PROG_START || cpu.PC >= STACK_START) {
		fmt.Printf("IE64 JIT: Invalid initial PC value: 0x%08x\n", cpu.PC)
		cpu.running.Store(false)
		return
	}

	if err := cpu.initJIT(); err != nil {
		fmt.Printf("IE64 JIT: %v, falling back to interpreter\n", err)
		cpu.Execute()
		return
	}
	defer cpu.freeJIT()

	execMem := cpu.getJITExecMem()

	// Initialize performance measurement
	cpu.perfStartTime = time.Now()
	cpu.lastPerfReport = cpu.perfStartTime
	cpu.InstructionCount = 0
	perfEnabled := cpu.PerfEnabled

	// Diagnostic counters
	var diagBlocksExec uint64    // JIT blocks executed via callNative
	var diagBlockInstrs uint64   // sum of instrCount for JIT-executed blocks
	var diagCacheHits uint64     // block found in cache
	var diagCacheMisses uint64   // block compiled (cache miss)
	var diagFallbackInstr uint64 // instructions via interpretOne
	var diagIOBails uint64       // I/O fallback bail count

	// Use local running flag to avoid atomic load every iteration
	running := true
	checkCounter := uint32(0)

	for running {
		// Periodic check of external stop signal
		checkCounter++
		if checkCounter&0xFFF == 0 && !cpu.running.Load() {
			break
		}

		// MMU state change: flush cache before any block lookup
		// This must be at the TOP of the loop because interpretOne() paths
		// (needsFallback, compilation failure) use continue and skip post-callNative checks.
		if cpu.jitNeedInval {
			cpu.jitCache.Invalidate()
			execMem.Reset()
			cpu.jitNeedInval = false
		}

		pcVirt := uint32(cpu.PC & IE64_ADDR_MASK)

		// MMU: translate virtual PC to physical for block fetch
		pcPhys := pcVirt
		if cpu.mmuEnabled {
			phys, fault, cause := cpu.translateAddr(pcVirt, ACCESS_EXEC)
			if fault {
				cpu.trapFault(cause, pcVirt)
				continue // re-enter loop at trap handler PC
			}
			pcPhys = phys
		}

		// Bounds check (on physical address)
		if uint64(pcPhys)+IE64_INSTR_SIZE > uint64(len(cpu.memory)) {
			fmt.Printf("IE64 JIT: PC out of bounds: 0x%08X\n", pcPhys)
			cpu.running.Store(false)
			break
		}

		// Check for HALT at current PC (physical)
		opcode := cpu.memory[pcPhys]
		if opcode == OP_HALT64 {
			cpu.running.Store(false)
			break
		}

		// Cache is keyed by virtual PC
		block := cpu.jitCache.Get(pcVirt)
		if block == nil {
			// Scan from physical memory
			var instrs []JITInstr
			if cpu.mmuEnabled {
				// Page boundary limit: don't scan past end of current 4 KiB page
				pageEnd := (pcPhys & ^uint32(MMU_PAGE_MASK)) + MMU_PAGE_SIZE
				instrs = scanBlockWithLimit(cpu.memory, pcPhys, pageEnd)
			} else {
				instrs = scanBlock(cpu.memory, pcPhys)
			}

			if needsFallback(instrs) {
				// FPU, WAIT, RTI, MMU ops — use interpreter for single instruction
				cpu.interpretOne()
				cpu.InstructionCount++
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				continue
			}

			var err error
			if cpu.mmuEnabled {
				// Compile with virtual startPC (branch offsets are virtual)
				block, err = compileBlockMMU(instrs, pcVirt, execMem)
			} else {
				block, err = compileBlock(instrs, pcPhys, execMem)
			}
			if err != nil {
				// Compilation failed (e.g., exec mem exhausted) — interpret
				cpu.interpretOne()
				cpu.InstructionCount++
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				continue
			}
			cpu.jitCache.Put(block)
			diagCacheMisses++
		} else {
			diagCacheHits++
		}

		// Execute the native code block
		callNative(block.execAddr, uintptr(unsafe.Pointer(cpu.jitCtx)))

		// Read packed PC+count from regs[0] (return channel).
		// Lower 32 bits = next PC, upper 32 bits = retired instruction count.
		combined := cpu.regs[0]
		cpu.regs[0] = 0
		cpu.PC = uint64(uint32(combined))
		executed := combined >> 32
		if executed == 0 {
			executed = uint64(block.instrCount) // safety fallback
		}

		ioBail := cpu.jitCtx.NeedIOFallback != 0
		if ioBail {
			diagIOBails++
		}
		diagBlocksExec++
		diagBlockInstrs += uint64(block.instrCount)

		// Self-modifying code: invalidate cache
		if cpu.jitCtx.NeedInval != 0 {
			cpu.jitCache.Invalidate()
			execMem.Reset()
			cpu.jitCtx.NeedInval = 0
		}

		// I/O fallback: re-execute the bailing instruction via interpreter
		if ioBail {
			cpu.jitCtx.NeedIOFallback = 0
			cpu.interpretOne()
			executed++ // count the re-executed instruction
			diagFallbackInstr++
			if !cpu.running.Load() {
				break
			}
		}

		// Timer check: decrement by number of executed instructions
		if cpu.timerEnabled.Load() {
			count := cpu.timerCount.Load()
			if count > 0 {
				if executed >= count {
					cpu.timerCount.Store(0)
					cpu.handleTimerJIT()
				} else {
					cpu.timerCount.Store(count - executed)
				}
			}
		}

		// Retired instruction count (uniform with interpreter)
		cpu.InstructionCount += executed

		// Performance reporting
		if perfEnabled && checkCounter&0x3FFF == 0 {
			now := time.Now()
			if now.Sub(cpu.lastPerfReport) >= time.Second {
				elapsed := now.Sub(cpu.perfStartTime).Seconds()
				mips := float64(cpu.InstructionCount) / elapsed / 1_000_000
				avgBlock := float64(0)
				if diagBlocksExec > 0 {
					avgBlock = float64(diagBlockInstrs) / float64(diagBlocksExec)
				}
				hitRate := float64(0)
				if diagCacheHits+diagCacheMisses > 0 {
					hitRate = float64(diagCacheHits) / float64(diagCacheHits+diagCacheMisses) * 100
				}
				fallbackPct := float64(0)
				if cpu.InstructionCount > 0 {
					fallbackPct = float64(diagFallbackInstr) / float64(cpu.InstructionCount) * 100
				}
				fmt.Printf("\rIE64 JIT: %.2f MIPS | avg blk %.1f | cache %.0f%% | fallback %.1f%% | io %d   ",
					mips, avgBlock, hitRate, fallbackPct, diagIOBails)
				cpu.lastPerfReport = now
			}
		}
	}
}

// handleTimerJIT handles timer expiry at JIT block boundaries.
// Called when TIMER_COUNT has reached 0.
func (cpu *CPU64) handleTimerJIT() {
	cpu.timerState.Store(TIMER_EXPIRED)
	if cpu.interruptEnabled.Load() && !cpu.inInterrupt.Load() {
		if cpu.mmuEnabled && cpu.intrVector != 0 {
			// ERET-model interrupt entry
			cpu.trapEntry()
			cpu.faultPC = cpu.PC
			cpu.faultAddr = 0
			cpu.faultCause = FAULT_TIMER
			cpu.PC = cpu.intrVector
		} else {
			// Legacy push-PC/RTI model
			cpu.inInterrupt.Store(true)
			cpu.regs[31] -= 8
			sp := uint32(cpu.regs[31])
			memSize := uint64(len(cpu.memory))
			if uint64(sp)+8 <= memSize {
				memBase := unsafe.Pointer(&cpu.memory[0])
				*(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(sp))) = cpu.PC
				cpu.PC = cpu.interruptVector
			}
		}
	}
	// Reload timer
	if cpu.timerEnabled.Load() {
		cpu.timerCount.Store(cpu.timerPeriod.Load())
	}
}
