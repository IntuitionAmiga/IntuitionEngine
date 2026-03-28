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

		pc := uint32(cpu.PC & IE64_ADDR_MASK)

		// Bounds check
		if uint64(pc)+IE64_INSTR_SIZE > uint64(len(cpu.memory)) {
			fmt.Printf("IE64 JIT: PC out of bounds: 0x%08X\n", pc)
			cpu.running.Store(false)
			break
		}

		// Check for HALT at current PC before doing anything
		opcode := cpu.memory[pc]
		if opcode == OP_HALT64 {
			cpu.running.Store(false)
			break
		}

		// Try to get a cached block
		block := cpu.jitCache.Get(pc)
		if block == nil {
			// Scan and potentially compile a new block
			instrs := scanBlock(cpu.memory, pc)
			if needsFallback(instrs) {
				// FPU, WAIT, RTI — use interpreter for single instruction
				cpu.interpretOne()
				cpu.InstructionCount++
				diagFallbackInstr++
				if !cpu.running.Load() {
					break
				}
				continue
			}

			var err error
			block, err = compileBlock(instrs, pc, execMem)
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

		// Timer check
		if cpu.timerEnabled.Load() {
			cpu.cycleCounter += executed
			if cpu.cycleCounter >= SAMPLE_RATE {
				cpu.cycleCounter = 0
				cpu.handleTimerJIT()
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

// handleTimerJIT handles timer interrupt checking at JIT block boundaries.
func (cpu *CPU64) handleTimerJIT() {
	count := cpu.timerCount.Load()
	if count > 0 {
		newCount := count - 1
		cpu.timerCount.Store(newCount)
		if newCount == 0 {
			cpu.timerState.Store(TIMER_EXPIRED)
			if cpu.interruptEnabled.Load() && !cpu.inInterrupt.Load() {
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
			if cpu.timerEnabled.Load() {
				cpu.timerCount.Store(cpu.timerPeriod.Load())
			}
		}
	} else {
		period := cpu.timerPeriod.Load()
		if period > 0 {
			cpu.timerCount.Store(period)
			cpu.timerState.Store(TIMER_RUNNING)
		}
	}
}
