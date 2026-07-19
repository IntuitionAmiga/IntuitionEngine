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
// Exact-range native-exit invalidation (milestone 4): every native guest store
// consults a per-page code bitmap (emitSMCStoreCheck) and, on a hit, records
// the precise written range in NeedInval/InvalAddr/InvalSize; the dispatcher
// invalidates exactly that range on return (TestM68KARM64_SMCExactRange). The
// conservative guest-byte stamp remains a backstop and still revalidates every
// block before entry, but the precise range is what lets native chaining bypass
// the dispatcher safely: a chained successor must never run across a
// self-modified predecessor.
//
// 68881 floating point (milestone 4): the register-to-register clean-mapping
// subset (FMOVE/FADD/FSUB/FMUL/FDIV/FABS/FNEG/FSQRT/FCMP/FTST and single
// FSGLMUL/FSGLDIV) is lowered natively in A64 scalar double, matching the
// interpreter's float64 arithmetic bit for bit with eager FPSR condition codes
// and FPIAR set to the instruction PC (emitFPU). FINT/FINTRZ (FPCR rounding
// mode), transcendentals, EA-operand forms and control/FMOVEM stay on the
// interpreter's 68881.
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
	// Per-page code bitmap for native-exit exact-range SMC detection: one byte
	// per 4 KiB guest page, covering all of guest RAM. Native stores consult it
	// (emitSMCStoreCheck) and flag NeedInval with the precise byte range.
	pageCount := (uint32(len(cpu.memory)) + 4095) >> 12
	cpu.m68kJitCodeBitmap = make([]byte, pageCount)
	cpu.m68kJitCtx = newM68KJITContext(cpu, cpu.m68kJitCodeBitmap, nil, nil)
	cpu.m68kJitWarmupLimit = m68kJITCompileWarmupLimit()
	return nil
}

// m68kARM64MarkCodePages marks every 4 KiB page a freshly compiled block spans
// so native stores into it flag an SMC invalidation. Marks are conservative:
// a page stays marked after its block is removed, which only costs a redundant
// (empty) precise-range invalidation later, never a missed one.
func (cpu *M68KCPU) m68kARM64MarkCodePages(block *JITBlock) {
	if cpu.m68kJitCodeBitmap == nil || block == nil {
		return
	}
	last := block.endPC
	if last > 0 {
		last--
	}
	for p := uint32(block.startPC) >> 12; p <= uint32(last)>>12; p++ {
		if int(p) < len(cpu.m68kJitCodeBitmap) {
			cpu.m68kJitCodeBitmap[p] = 1
		}
	}
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
		cpu.m68kJitCodeBitmap = nil
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
	// Any removed block leaves a dangling chain-cache entry.
	m68kARM64ClearChainCache(cpu.m68kJitCtx)
}

// m68kARM64ChainCacheInsert records a compiled block's (startPC -> chainEntry)
// in one of the eight context chain-cache slots (reused from the RTS-cache
// fields, which the arm64 backend does not otherwise use). A chaining block's
// native exit scans these slots to tail-branch into a hot successor. cursor
// rotates so recently executed blocks stay resident. A zero chainEntry (chaining
// disabled for that block) is ignored.
func m68kARM64ChainCacheInsert(ctx *M68KJITContext, pc uint32, entry uintptr, cursor int) {
	if entry == 0 {
		return
	}
	switch cursor & 7 {
	case 0:
		ctx.RTSCache0PC, ctx.RTSCache0Addr = pc, entry
	case 1:
		ctx.RTSCache1PC, ctx.RTSCache1Addr = pc, entry
	case 2:
		ctx.RTSCache2PC, ctx.RTSCache2Addr = pc, entry
	case 3:
		ctx.RTSCache3PC, ctx.RTSCache3Addr = pc, entry
	case 4:
		ctx.RTSCache4PC, ctx.RTSCache4Addr = pc, entry
	case 5:
		ctx.RTSCache5PC, ctx.RTSCache5Addr = pc, entry
	case 6:
		ctx.RTSCache6PC, ctx.RTSCache6Addr = pc, entry
	case 7:
		ctx.RTSCache7PC, ctx.RTSCache7Addr = pc, entry
	}
}

// m68kARM64ClearChainCache zeroes every chain-cache slot. It MUST run whenever a
// block is removed or exec memory is reset: a stale chain entry would tail-branch
// into dead or reused native code. Clearing only disables chaining until the
// cache refills, so it is always safe.
func m68kARM64ClearChainCache(ctx *M68KJITContext) {
	if ctx == nil {
		return
	}
	ctx.RTSCache0PC, ctx.RTSCache0Addr = 0, 0
	ctx.RTSCache1PC, ctx.RTSCache1Addr = 0, 0
	ctx.RTSCache2PC, ctx.RTSCache2Addr = 0, 0
	ctx.RTSCache3PC, ctx.RTSCache3Addr = 0, 0
	ctx.RTSCache4PC, ctx.RTSCache4Addr = 0, 0
	ctx.RTSCache5PC, ctx.RTSCache5Addr = 0, 0
	ctx.RTSCache6PC, ctx.RTSCache6Addr = 0, 0
	ctx.RTSCache7PC, ctx.RTSCache7Addr = 0, 0
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
	// A full drop clears the code-page marks; they are rebuilt as blocks
	// recompile. The chain cache holds now-dangling entry addresses and must
	// be cleared too.
	if cpu.m68kJitCodeBitmap != nil {
		clear(cpu.m68kJitCodeBitmap)
	}
	m68kARM64ClearChainCache(cpu.m68kJitCtx)
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
	chainEnabled := m68kARM64ChainEnabled()
	// ChainBudget bounds how many instructions may run natively (across chained
	// blocks) before returning to sample interrupts; matches the interpreter's
	// poll cadence.
	const m68kARM64ChainBudget = 256
	chainCursor := 0
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
			// Removed blocks leave dangling chain-cache entries.
			if chainEnabled {
				m68kARM64ClearChainCache(ctx)
			}
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
			// The removed block may be referenced by a chain-cache entry.
			if chainEnabled {
				m68kARM64ClearChainCache(ctx)
			}
		}
		if block == nil {
			if _, bad := uncompilable[pc]; !bad && int(pc) < len(cpu.memory) && pc&1 == 0 {
				if cpu.m68kJITShouldWarmupInterpret(pc) {
					instructionCount += uint64(cpu.StepOne())
					continue
				}
				instrs := m68kScanBlock(cpu.memory, pc)
		instrs = m68kFuseJSRLeafCalls(instrs, pc, cpu.memory, cpu.ProfileTopOfRAM())
				prefix := m68kARM64SupportedPrefix(instrs, cpu.memory, pc, cpu.ProfileTopOfRAM())
				if prefix >= m68kARM64MinPrefix {
					compiled, err := m68kCompileBlockARM64(instrs[:prefix], pc, execMem, cpu.memory, cpu.ProfileTopOfRAM())
					if err == nil {
						m68kStampGuestBlockBytes(cpu.memory, compiled)
						cpu.m68kARM64PublishCodeEnv(compiled)
						cpu.m68kARM64MarkCodePages(compiled)
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
		ctx.NeedInval = 0
		ctx.InvalAddr = 0
		ctx.InvalSize = 0
		if chainEnabled {
			// Seed the interrupt-latency budget, publish the generation
			// snapshot so a chained edge can detect a cross-thread rewrite, and
			// make this block reachable as a chain target for its predecessors.
			ctx.ChainBudget = m68kARM64ChainBudget
			ctx.InvalGenSnapshot = genSnapshot
			m68kARM64ChainCacheInsert(ctx, uint32(block.startPC), block.chainEntry, chainCursor)
			chainCursor++
		}
		cpu.m68kJitNativeActive.Store(true)
		callNative(block.execAddr, m68kJITContextPtr(ctx))
		cpu.m68kJitNativeActive.Store(false)
		cpu.m68kJitExecMu.Unlock()
		cpu.PC = ctx.RetPC

		if ctx.NeedInval != 0 {
			// A native store landed on a code page: invalidate exactly the
			// written range so any block covering it recompiles on next
			// dispatch. The conservative guest-byte stamp remains a backstop;
			// this precise invalidation is the throughput refinement and the
			// prerequisite for native chaining (a chained successor must never
			// run across a self-modified predecessor).
			invalAddr, invalSize := ctx.InvalAddr, ctx.InvalSize
			ctx.NeedInval = 0
			ctx.InvalAddr = 0
			ctx.InvalSize = 0
			if invalSize != 0 {
				cpu.invalidateM68KJITForGuestWrite(invalAddr, invalSize)
			}
		}

		if ctx.NeedIOFallback != 0 {
			// A guarded memory access hit an I/O page or the RAM bound.
			// The block exited before the faulting instruction's side
			// effects; RetCount holds the fully retired predecessors and
			// RetPC the faulting instruction. Interpret that single
			// instruction and re-enter the dispatch loop.
			ctx.NeedIOFallback = 0
			// Across a chain, ChainCount holds the fully-retired predecessors
			// and RetCount the partial count within the bailing block.
			instructionCount += uint64(ctx.ChainCount)
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
