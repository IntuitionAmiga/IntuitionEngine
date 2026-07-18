// jit_m68k_exec_arm64.go - M68020 JIT execution loop for arm64
// (parity plan milestone 3, slice 1).
//
// Correctness-first dispatcher: per-block native execution of supported
// straight-line prefixes, single-instruction interpreter fallback for
// everything else, interrupt and exception sampling at every block boundary,
// and the retired-instruction contract from day one (total = RetCount here;
// this slice has no chaining, so ChainCount is always zero and the shared
// m68kJITRetiredInstructionCount contract is trivially satisfied).
//
// Self-modifying code:
//   - Same-thread (guest) writes: conservative guest byte stamp. Every
//     compiled block records a hash of the guest bytes it covers and the
//     dispatcher recompiles on mismatch before entering native code.
//   - Cross-thread (host goroutine) writes: the shared invalidation queue
//     and generation counter (jit_m68k_inval_queue.go, wired through the
//     bus invalidator registered in NewM68KCPU). The dispatcher drains the
//     queue at each loop head, snapshots m68kJitInvalGen, and re-checks the
//     snapshot immediately before entering native code. A bus write that
//     overlaps compiled code is serialised with final validation and native
//     execution, closing the check-to-call race.
// Exact-range native-exit invalidation (the amd64 refinement where a block
// that stores into its own code region exits carrying the precise byte range)
// is a throughput optimisation over the conservative guest-byte stamp used
// here: the stamp recompiles on ANY change to a block's bytes before the block
// is re-entered, so stale native code never runs across a dispatch boundary
// (TestM68KARM64_SMCStampMismatch). It ports alongside register pinning in
// milestone 4. Blocks are short straight-line prefixes, and every dispatch
// revalidates the stamp, so the conservative scheme is correct; only the
// precise-range fast path is deferred.
//
// 68881 floating point: F-line instructions (opcode 0xF000..) are never
// admitted into a native block, so they always fall back to the interpreter's
// 68881 implementation. This is the explicitly staged FPU fallback the parity
// plan permits; native 68881 lowering is milestone 4 work.
//
// Block chaining and the interrupt boundary:
//   The arm64 backend operates directly on cpu.DataRegs/cpu.AddrRegs through
//   the context base pointers and keeps only the live CCR (W4) and the resume
//   PC in host registers, flushing the CCR into cpu.SR in every block's
//   epilogue before returning. The dispatcher then samples pending exceptions
//   and interrupts at the top of the loop (m68kARM64CheckPending), so the
//   status register a successor observes is always the predecessor's flushed
//   value: the interrupt-boundary SR-publication invariant the parity plan
//   requires is satisfied by the returning dispatcher without native chaining.
//   Native block chaining itself (a direct branch from one block's exit into a
//   successor's entry, bypassing the dispatcher) is a throughput optimisation.
//   On amd64 it is coupled to register pinning across the chain edge; the
//   arm64 backend defers register pinning to milestone 4, so native chaining
//   ports alongside it there rather than in the correctness foundation. Until
//   then ChainCount stays zero and every boundary is a dispatcher round trip.

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"fmt"
)

// m68kJitExecMemSizeARM64 is deliberately small: the slice-1 backend compiles
// short straight-line prefixes only.
const m68kJitExecMemSizeARM64 = 16 * 1024 * 1024

// m68kARM64MinPrefix: single-instruction native blocks are not worth the
// dispatch round trip; the interpreter handles them.
const m68kARM64MinPrefix = 2

func (cpu *M68KCPU) m68kGetJITExecMem() *ExecMem {
	if cpu.m68kJitExecMem == nil {
		return nil
	}
	return cpu.m68kJitExecMem.(*ExecMem)
}

func (cpu *M68KCPU) initM68KJIT() error {
	if cpu.m68kJitExecMem != nil {
		return nil
	}
	execMem, err := AllocExecMem(m68kJitExecMemSizeARM64)
	if err != nil {
		return fmt.Errorf("exec mem alloc failed: %w", err)
	}
	cpu.m68kJitExecMem = execMem
	cpu.m68kJitCache = NewCodeCache()
	// The I/O page bitmap must exist before the context snapshot: the
	// slice-2 emitter's inline access guards read it through the context.
	cpu.m68kBuildJITIOPageBitmap()
	cpu.m68kJitCtx = newM68KJITContext(cpu, nil, nil, nil)
	cpu.m68kJitWarmupLimit = m68kJITCompileWarmupLimit()
	return nil
}

func (cpu *M68KCPU) freeM68KJIT() {
	if !cpu.m68kJitPersist {
		if execMem := cpu.m68kGetJITExecMem(); execMem != nil {
			execMem.Free()
		}
		cpu.m68kJitExecMem = nil
		cpu.m68kJitCache = nil
		cpu.m68kJitCtx = nil
		cpu.m68kJitWarmupCounts = nil
	}
}

// invalidateM68KJITForGuestWrite applies one invalidation range to the code
// cache. Must run on the CPU/dispatcher goroutine (or while the cache is
// quiescent — loaders and single-threaded tests before dispatch starts);
// cross-thread callers go through m68kEnqueueJITInvalidation via the bus
// invalidator.
func (cpu *M68KCPU) invalidateM68KJITForGuestWrite(addr uint32, size uint32) {
	if cpu == nil || cpu.m68kJitCache == nil || size == 0 {
		return
	}
	end := uint64(addr) + uint64(size)
	cpu.m68kJitCache.InvalidateRange(uint64(addr), end)
}

// m68kResetJITCodeCache drops every compiled block (cross-thread queue
// overflow path). Exec memory is bump-allocated, so a full cache drop also
// resets the allocator.
func (cpu *M68KCPU) m68kResetJITCodeCache() {
	if cpu == nil || cpu.m68kJitCache == nil {
		return
	}
	cpu.m68kJitCache.Invalidate()
	if execMem := cpu.m68kGetJITExecMem(); execMem != nil {
		execMem.Reset()
	}
}

// m68kVerifyCaptureWrite: the native-vs-interpreter self-verifier is an
// amd64 diagnostic; no capture on arm64.
func (cpu *M68KCPU) m68kVerifyCaptureWrite(addr uint32, size int) bool { return false }

// m68kARM64PublishCodeEnv widens the global code envelope to cover a newly
// cached block and publishes it for the cross-thread bus invalidator gate
// (NewM68KCPU only enqueues writes that fall inside the envelope; an empty
// envelope means "no compiled code" and drops the write). Must be called
// BEFORE the block becomes reachable in the cache.
func (cpu *M68KCPU) m68kARM64PublishCodeEnv(block *JITBlock) {
	lo, hi := cpu.m68kJitCodeLoAddr, cpu.m68kJitCodeHiAddr
	if hi <= lo {
		lo, hi = uint32(block.startPC), uint32(block.endPC)
	} else {
		if uint32(block.startPC) < lo {
			lo = uint32(block.startPC)
		}
		if uint32(block.endPC) > hi {
			hi = uint32(block.endPC)
		}
	}
	cpu.m68kJitCodeLoAddr, cpu.m68kJitCodeHiAddr = lo, hi
	cpu.m68kJitCodeEnv.Store(uint64(lo)<<32 | uint64(hi))
}

// m68kARM64CheckPending delivers a pending exception or interrupt, mirroring
// the amd64 dispatcher's boundary handling. Returns true if anything was
// delivered (PC/SR changed).
func (cpu *M68KCPU) m68kARM64CheckPending() bool {
	delivered := false
	if pendingException := cpu.pendingException.Load(); pendingException != 0 {
		cpu.pendingException.Store(0)
		cpu.ProcessException(uint8(pendingException))
		delivered = true
	}
	if pending := cpu.pendingInterrupt.Load(); pending != 0 {
		ipl := uint32((cpu.SR & M68K_SR_IPL) >> M68K_SR_SHIFT)
		for level := uint32(7); level >= 1; level-- {
			if pending&(1<<level) != 0 && (level > ipl || level == 7) {
				if cpu.ProcessInterrupt(uint8(level)) {
					delivered = true
					for {
						old := cpu.pendingInterrupt.Load()
						if cpu.pendingInterrupt.CompareAndSwap(old, old&^(1<<level)) {
							break
						}
					}
				}
				break
			}
		}
	}
	return delivered
}

// M68KExecuteJIT is the arm64 M68020 JIT dispatcher.
func (cpu *M68KCPU) M68KExecuteJIT() {
	if err := cpu.initM68KJIT(); err != nil {
		fmt.Printf("M68K JIT (arm64): %v, falling back to interpreter\n", err)
		cpu.ExecuteInstruction()
		return
	}
	defer cpu.freeM68KJIT()

	cpu.m68kJitDispatchActive.Store(true)
	defer cpu.m68kJitDispatchActive.Store(false)

	execMem := cpu.m68kGetJITExecMem()
	ctx := cpu.m68kJitCtx
	uncompilable := make(map[uint32]struct{}, 1024)
	var instructionCount uint64
	if cpu.PerfEnabled {
		cpu.InstructionCount = 0
	}
	publishCount := func() {
		if cpu.PerfEnabled {
			cpu.InstructionCount = instructionCount
		}
	}
	defer publishCount()

	for cpu.running.Load() {
		if cpu.debugHandleBreakIn(uint64(cpu.PC)) {
			break
		}
		if cpu.stopped.Load() {
			// STOP state: only an exception or interrupt can wake the CPU.
			woke := cpu.m68kARM64CheckPending()
			if woke {
				cpu.stopped.Store(false)
				cpu.stopSpinCount.Store(0)
				continue
			}
			if cpu.StoppedIdleHook != nil {
				cpu.StoppedIdleHook(cpu)
			}
			continue
		}

		cpu.runInstructionCountHook(instructionCount)
		cpu.m68kARM64CheckPending()
		if !cpu.running.Load() || cpu.stopped.Load() {
			continue
		}

		// Drain cross-thread invalidations queued by host goroutines, then
		// snapshot the generation. Any enqueue after this point bumps the
		// generation and is caught by the re-check before native entry.
		if cpu.m68kJitHasPendingInval.Load() {
			cpu.m68kDrainPendingJITInvalidations()
			// Invalidation may have removed blocks whose PCs were marked
			// uncompilable-adjacent or replaced code under them; rewritten
			// code must get a fresh admission decision.
			clear(uncompilable)
		}
		genSnapshot := cpu.m68kJitInvalGen.Load()

		pc := cpu.PC
		var block *JITBlock
		if cpu.m68kJitCache != nil {
			block = cpu.m68kJitCache.Get(uint64(pc))
		}
		if block != nil && !m68kGuestBlockBytesStillMatch(cpu.memory, block) {
			cpu.m68kJitCache.RemoveBlock(block)
			block = nil
			delete(uncompilable, pc)
		}
		if block == nil {
			if _, bad := uncompilable[pc]; !bad && int(pc) < len(cpu.memory) && pc&1 == 0 {
				if cpu.m68kJITShouldWarmupInterpret(pc) {
					instructionCount += uint64(cpu.StepOne())
					continue
				}
				instrs := m68kScanBlock(cpu.memory, pc)
				prefix := m68kARM64SupportedPrefix(instrs, cpu.memory, pc, cpu.ProfileTopOfRAM())
				if prefix >= m68kARM64MinPrefix {
					compiled, err := m68kCompileBlockARM64(instrs[:prefix], pc, execMem, cpu.memory, cpu.ProfileTopOfRAM())
					if err == nil {
						m68kStampGuestBlockBytes(cpu.memory, compiled)
						cpu.m68kARM64PublishCodeEnv(compiled)
						cpu.m68kJitCache.Put(compiled)
						block = compiled
					} else {
						uncompilable[pc] = struct{}{}
					}
				} else {
					uncompilable[pc] = struct{}{}
				}
			}
		}
		if block == nil {
			// Single-instruction interpreter fallback (StepOne; the
			// misleadingly named ExecuteInstruction is the full run loop).
			instructionCount += uint64(cpu.StepOne())
			continue
		}

		// Final stale-code gate: a host goroutine may have queued an
		// invalidation after the drain/stamp check above. If the generation
		// moved, re-loop (drain, re-check, possibly recompile) instead of
		// running a potentially stale block.
		cpu.m68kJitExecMu.Lock()
		if cpu.m68kJitInvalGen.Load() != genSnapshot ||
			!m68kGuestBlockBytesStillMatch(cpu.memory, block) {
			cpu.m68kJitExecMu.Unlock()
			continue
		}

		ctx.RetPC = 0
		ctx.RetCount = 0
		ctx.ChainCount = 0
		ctx.NeedIOFallback = 0
		cpu.m68kJitNativeActive.Store(true)
		callNative(block.execAddr, m68kJITContextPtr(ctx))
		cpu.m68kJitNativeActive.Store(false)
		cpu.m68kJitExecMu.Unlock()
		cpu.PC = ctx.RetPC

		if ctx.NeedIOFallback != 0 {
			// A guarded memory access hit an I/O page or the RAM bound.
			// The block exited before the faulting instruction's side
			// effects; RetCount holds the fully retired predecessors and
			// RetPC the faulting instruction. Interpret that single
			// instruction and re-enter the dispatch loop.
			ctx.NeedIOFallback = 0
			instructionCount += uint64(ctx.RetCount)
			instructionCount += uint64(cpu.StepOne())
			if cpu.PerfEnabled {
				cpu.InstructionCount = instructionCount
			}
			continue
		}

		retired := m68kJITRetiredInstructionCount(ctx.RetCount, ctx.ChainCount, block.instrCount, false)
		instructionCount += retired
		m68kBumpJITBlockHotness(block, 1)
		if cpu.PerfEnabled {
			cpu.InstructionCount = instructionCount
		}
	}
}
