//go:build js && wasm

package main

import (
	"syscall/js"
	"unsafe"
)

func ie32JITTryRunDirect(cpu *CPU) uint64 {
	if cpu == nil || !js.Global().Get("__goMem").Truthy() {
		return 0
	}
	// Wasm has a content-keyed module cache rather than persistent native entry
	// addresses, so an RTS cannot safely retain a direct return target.
	cpu.jit.returnCachePending = false
	cpuAddress := uintptr(unsafe.Pointer(cpu))
	if !cpu.timerEnabled.Load() && cpu.jit.testStopAfter == 0 {
		if cached, found := ie32WasmCachedEntry(cpu); found {
			if cached.cached.stamp != ie32CachedSourceStamp(cpu.memory, cached.cached) {
				ie32DropWasmCachedEntry(cpu, cpu.PC)
				cpu.jit.deoptimizations.Add(1)
				cpu.jit.sourceStampDeopts.Add(1)
				return 0
			}
			retired := cached.cached.retired
			if cached.countedPlan != nil {
				if !cached.countedStatic || cached.countedCount == 0 || cached.countedCount > 4096 {
					return 0
				}
				retired = uint64(cached.countedPlan.head) + uint64(cached.countedCount)*cached.countedPlan.bodyRetired
			}
			cached.fn.Invoke(int(uint32(cpuAddress)))
			cpu.jit.nativeEntries.Add(1)
			cpu.jit.instructions.Add(retired)
			cpu.jit.directInstructions.Add(retired)
			cpu.jit.cacheHits.Add(1)
			if cached.countedPlan != nil {
				cpu.jit.countedLoops.Add(1)
			}
			return retired
		}
	}
	limit := 0
	if cpu.jit.testStopAfter > cpu.jit.testRetired {
		limit = int(cpu.jit.testStopAfter - cpu.jit.testRetired)
	}
	if cpu.timerEnabled.Load() && (limit == 0 || limit > 1) {
		limit = 1
	}
	block := scanIE32FusedBlock(cpu.memory, cpu.PC, limit)
	if cpu.ie32JITShouldCompileRegion(cpu.PC) && !ie32BlockHasLeafFusion(block) {
		block = scanIE32Region(cpu.memory, cpu.PC, limit)
	}
	block = ie32FoldImmediateALU(block)
	block = ie32AnnotateKnownBranches(block)
	block = ie32AnnotateResidentImmediateALU(block)
	block = ie32SpecialiseKnownConstantRegisterAddresses(block)
	if cpu.jit.testStopAfter == 0 {
		if plan := ie32AnalyseCountedLoop(block); plan != nil {
			if count, ok := ie32CountedLoopInitialCount(cpu, block, plan); ok && count > 0 && count <= 4096 && ie32CountedLoopDirectMemoryAdmissible(cpu, block, plan) {
				if retired := cpu.ie32JITTryRunCountedLoop(block, plan, count); retired != 0 {
					return retired
				}
			}
		}
	}
	wasm, u8ctor, imports, runtimeOK := ie32WasmRuntimeObjects()
	if !runtimeOK {
		return 0
	}
	for len(block) > 0 {
		lowered := append([]ie32DecodedInstruction(nil), block...)
		admitted := true
		for i, in := range lowered {
			specialised, ok := ie32SpecialiseFirstIndirect(cpu, in, i == 0)
			if !ok {
				admitted = false
				break
			}
			in = specialised
			lowered[i] = in
			if (isIE32LoadOpcode(in.Opcode) || in.Opcode == STORE || ie32IsNamedStore(in.Opcode)) && in.AddrMode == ADDR_REG_IND {
				if i != 0 {
					admitted = false
					break
				}
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					admitted = false
					break
				}
				lowered[i].AddrMode = ADDR_DIRECT
				lowered[i].Operand = addr
				in = lowered[i]
			}
			if (in.Opcode == STORE || ie32IsNamedStore(in.Opcode)) && in.AddrMode == ADDR_MEM_IND {
				if i != 0 {
					admitted = false
					break
				}
				addr, ok := ie32StaticMemoryIndirectStoreAddress(cpu, in)
				if !ok {
					admitted = false
					break
				}
				lowered[i].AddrMode = ADDR_DIRECT
				lowered[i].Operand = addr
				in = lowered[i]
			}
			if (in.Opcode == INC || in.Opcode == DEC) && in.AddrMode == ADDR_MEM_IND {
				if i != 0 {
					admitted = false
					break
				}
				addr, ok := ie32StaticMemoryIndirectStoreAddress(cpu, in)
				if !ok {
					admitted = false
					break
				}
				lowered[i].AddrMode = ADDR_DIRECT
				lowered[i].Operand = addr
				in = lowered[i]
			}
			if (in.Opcode == INC || in.Opcode == DEC) && in.AddrMode == ADDR_REG_IND {
				if i != 0 {
					admitted = false
					break
				}
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok || !ie32CanDirectRAMWrite(cpu, addr) {
					admitted = false
					break
				}
				lowered[i].AddrMode = ADDR_DIRECT
				lowered[i].Operand = addr
				in = lowered[i]
			}
			if ie32IsDirectALUOpcode(in.Opcode) && in.AddrMode == ADDR_REG_IND {
				if i != 0 {
					admitted = false
					break
				}
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok {
					admitted = false
					break
				}
				lowered[i].AddrMode = ADDR_DIRECT
				lowered[i].Operand = addr
				in = lowered[i]
			}
			if (in.Opcode == DIV || in.Opcode == MOD) && in.AddrMode == ADDR_REG_IND {
				if i != 0 {
					admitted = false
					break
				}
				addr, ok := ie32StaticRegisterIndirectAddress(cpu, in)
				if !ok || cpu.Read32(addr) == 0 {
					admitted = false
					break
				}
				lowered[i].AddrMode = ADDR_DIRECT
				lowered[i].Operand = addr
				in = lowered[i]
			}
			if (in.Opcode == DIV || in.Opcode == MOD) && in.AddrMode == ADDR_DIRECT && (!ie32CanDirectRAMRead(cpu, in.Operand) || cpu.Read32(in.Operand) == 0) {
				admitted = false
				break
			}
			kind, ok := ie32FormLowering[ie32OpcodeForm{Opcode: in.Opcode, AddrMode: in.AddrMode}]
			if !ok || kind != ie32LoweringDirect {
				admitted = false
				break
			}
			if (in.Opcode == DIV || in.Opcode == MOD) && in.AddrMode == ADDR_REGISTER && (i != 0 || *cpu.getRegister(in.operandRegisterIndex()) == 0) {
				admitted = false
				break
			}
			if (in.Opcode == SHL || in.Opcode == SHR) && in.AddrMode == ADDR_REGISTER && (i != 0 || *cpu.getRegister(in.operandRegisterIndex()) >= 32) {
				admitted = false
				break
			}
			if (in.Opcode == LOAD || in.Opcode == LDA || in.Opcode == ADD || in.Opcode == SUB || in.Opcode == MUL || in.Opcode == AND || in.Opcode == OR || in.Opcode == XOR || in.Opcode == INC || in.Opcode == DEC) && (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && !ie32CanDirectRAMRead(cpu, in.Operand) {
				admitted = false
				break
			}
			if (in.Opcode == INC || in.Opcode == DEC) && (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND) && (!ie32CanDirectRAMWrite(cpu, in.Operand) || ie32WriteMutatesRemainingBlock(in, lowered)) {
				admitted = false
				break
			}
			if (in.Opcode == STORE || ie32IsNamedStore(in.Opcode)) && (in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_REGISTER || in.AddrMode == ADDR_IMMEDIATE) && (!ie32CanDirectRAMWrite(cpu, in.Operand) || ie32WriteMutatesRemainingBlock(in, lowered)) {
				admitted = false
				break
			}
		}
		if !admitted {
			block = block[:len(block)-1]
			continue
		}
		modBytes, err := compileIE32WasmBlockAtStack(lowered, cpu.SP)
		if err != nil {
			block = block[:len(block)-1]
			continue
		}
		var entry js.Value
		var cacheHit bool
		ok := func() (ok bool) {
			defer func() {
				if recover() != nil {
					ok = false
				}
			}()
			entry, cacheHit = ie32WasmBlockForBytes(wasm, u8ctor, imports, modBytes)
			entry.Invoke(int(uint32(cpuAddress)))
			if cacheHit {
				cpu.jit.cacheHits.Add(1)
			}
			return true
		}()
		if !ok {
			return 0
		}
		cpu.jit.blocks.Add(1)
		if ie32BlockIsRegion(block, len(block)) {
			cpu.jit.regions.Add(1)
		}
		cpu.jit.nativeEntries.Add(1)
		cpu.jit.instructions.Add(uint64(len(block)))
		cpu.jit.directInstructions.Add(uint64(len(block)))
		cpu.jit.residentSpillsSaved.Add(ie32ResidentALUSpillsSaved(block, len(block)))
		ie32RememberWasmCachedEntry(cpu, lowered, uint64(len(block)), entry, nil)
		return uint64(len(block))
	}
	return 0
}

func (cpu *CPU) ie32JITTryRunCountedLoop(block []ie32DecodedInstruction, plan *ie32CountedLoopPlan, count uint32) uint64 {
	if cpu == nil || cpu.jit == nil || plan == nil || count == 0 {
		return 0
	}
	retired := uint64(plan.head) + uint64(count)*plan.bodyRetired
	modBytes, err := compileIE32WasmCountedLoopBlockAtStack(block, plan, cpu.SP)
	if err != nil {
		return 0
	}
	wasm, u8ctor, imports, runtimeOK := ie32WasmRuntimeObjects()
	if !runtimeOK {
		return 0
	}
	cpuAddress := uintptr(unsafe.Pointer(cpu))
	var entry js.Value
	var cacheHit bool
	ok := func() (ok bool) {
		defer func() {
			if recover() != nil {
				ok = false
			}
		}()
		entry, cacheHit = ie32WasmBlockForBytes(wasm, u8ctor, imports, modBytes)
		entry.Invoke(int(uint32(cpuAddress)))
		if cacheHit {
			cpu.jit.cacheHits.Add(1)
		}
		return true
	}()
	if !ok {
		return 0
	}
	cpu.jit.blocks.Add(1)
	cpu.jit.nativeEntries.Add(1)
	cpu.jit.instructions.Add(retired)
	cpu.jit.directInstructions.Add(retired)
	cpu.jit.countedLoops.Add(1)
	ie32RememberWasmCachedEntry(cpu, block, retired, entry, plan)
	return retired
}

func isIE32LoadOpcode(opcode byte) bool {
	switch opcode {
	case LOAD, LDA, LDX, LDY, LDZ, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW:
		return true
	default:
		return false
	}
}
