//go:build js && wasm

package main

import (
	"encoding/binary"
	"os"
	"syscall/js"
	"unsafe"
)

type p65WasmBlock struct {
	fn     js.Value
	source []byte
}

type p65WasmRuntime struct {
	cpu     *CPU_6502
	wasm    js.Value
	u8array js.Value
	imports js.Value
	ctx     []byte
	ctxAddr int
	cache   map[uint16]*p65WasmBlock
}

func p65WasmJITEnabled() bool {
	return os.Getenv("P65_WASM_JIT") != "0" && js.Global().Get("__goMem").Truthy()
}

func init() { jit6502Available = p65WasmJITEnabled() }

func (cpu *CPU_6502) jit6502Execute() {
	if cpu.Debug || !cpu.jitEnabled || !p65WasmJITEnabled() {
		cpu.Execute()
		return
	}
	cpu.ExecuteJIT6502()
}

func (cpu *CPU_6502) freeJIT6502() {}

func newP65WasmRuntime(cpu *CPU_6502) *p65WasmRuntime {
	if cpu.fastAdapter == nil || len(cpu.fastAdapter.memDirect) == 0 {
		return nil
	}
	mem := js.Global().Get("__goMem")
	if !mem.Truthy() {
		return nil
	}
	ctx := make([]byte, p65WasmCtxImageSize)
	rt := &p65WasmRuntime{
		cpu: cpu, wasm: js.Global().Get("WebAssembly"), u8array: js.Global().Get("Uint8Array"),
		ctx: ctx, ctxAddr: int(uintptr(unsafe.Pointer(&ctx[0]))), cache: map[uint16]*p65WasmBlock{},
	}
	env := js.Global().Get("Object").New()
	env.Set("mem", mem)
	rt.imports = js.Global().Get("Object").New()
	rt.imports.Set("env", env)
	binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffMemPtr:], uint32(uintptr(unsafe.Pointer(&cpu.fastAdapter.memDirect[0]))))
	binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffCpuPtr:], uint32(uintptr(unsafe.Pointer(cpu))))
	binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffNZTable:], uint32(uintptr(unsafe.Pointer(&nzTable[0]))))
	binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffDecimalADC:], uint32(uintptr(unsafe.Pointer(&p65DecimalADC[0]))))
	binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffDecimalSBC:], uint32(uintptr(unsafe.Pointer(&p65DecimalSBC[0]))))
	binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffBinaryADC:], uint32(uintptr(unsafe.Pointer(&p65BinaryADC[0]))))
	binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffBinarySBC:], uint32(uintptr(unsafe.Pointer(&p65BinarySBC[0]))))
	binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffDirectPages:], uint32(uintptr(unsafe.Pointer(&cpu.directPageBitmap[0]))))
	return rt
}

func (rt *p65WasmRuntime) compile(pc uint16) (blk *p65WasmBlock) {
	instrs := jit6502ScanBlockLimit(rt.cpu.fastAdapter.memDirect, pc, len(rt.cpu.fastAdapter.memDirect), rt.cpu.jitTestBlockLimit)
	if len(instrs) == 0 {
		return nil
	}
	// A RAM-mutating form is a hard module boundary. This makes source-byte
	// validation observe self-modification before a later instruction can run
	// from stale wasm code.
	for index, instr := range instrs {
		if p65WasmMutatesRAM(instr.opcode) {
			instrs = instrs[:index+1]
			break
		}
	}
	// Shorten to the largest directly encoded prefix. The first unsupported
	// form remains an interpreter boundary rather than entering a partial module.
	for len(instrs) > 0 {
		modBytes, err := p65WasmCompileBlock(instrs, pc)
		if err == nil {
			defer func() {
				if recover() != nil {
					blk = nil
				}
			}()
			u8 := rt.u8array.New(len(modBytes))
			js.CopyBytesToJS(u8, modBytes)
			mod := rt.wasm.Get("Module").New(u8)
			inst := rt.wasm.Get("Instance").New(mod, rt.imports)
			fn := inst.Get("exports").Get("block")
			if !fn.Truthy() {
				return nil
			}
			source := make([]byte, 0, 3*len(instrs))
			for _, instr := range instrs {
				source = append(source, instr.opcode)
				if instr.length >= 2 {
					source = append(source, byte(instr.operand))
				}
				if instr.length == 3 {
					source = append(source, byte(instr.operand>>8))
				}
			}
			return &p65WasmBlock{fn: fn, source: source}
		}
		instrs = instrs[:len(instrs)-1]
	}
	return nil
}

func (rt *p65WasmRuntime) sourceMatches(pc uint16, blk *p65WasmBlock) bool {
	end := int(pc) + len(blk.source)
	if end > len(rt.cpu.fastAdapter.memDirect) {
		return false
	}
	for i, value := range blk.source {
		if rt.cpu.fastAdapter.memDirect[int(pc)+i] != value {
			return false
		}
	}
	return true
}

func (rt *p65WasmRuntime) invoke(blk *p65WasmBlock) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	blk.fn.Invoke(rt.ctxAddr)
	return true
}

func (cpu *CPU_6502) interpret6502One() {
	cpu.ensureOpcodeTableReady()
	opcode := cpu.readByte(cpu.PC)
	cpu.PC++
	cpu.opcodeTable[opcode](cpu)
}

// ExecuteJIT6502 runs cached wasm modules for supported straight-line prefixes
// and otherwise preserves the interpreter's one-instruction boundary.
func (cpu *CPU_6502) ExecuteJIT6502() {
	if !p65WasmJITEnabled() {
		cpu.Execute()
		return
	}
	cpu.ensureOpcodeTableReady()
	if adapter, ok := cpu.memory.(*Bus6502Adapter); ok {
		if bus, ok := adapter.bus.(*MachineBus); ok {
			bus.SealMappings()
		}
	}
	cpu.initDirectPageBitmap()
	rt := newP65WasmRuntime(cpu)
	if rt == nil {
		cpu.Execute()
		return
	}
	cpu.executing.Store(true)
	defer cpu.executing.Store(false)
	for cpu.running.Load() {
		if cpu.debugHandleBreakIn(uint64(cpu.PC)) {
			break
		}
		// Pause at a module boundary so Reset() can safely replace the
		// architectural state, matching the interpreter and native JIT.
		if cpu.resetting.Load() {
			cpu.resetAck.Store(true)
			for cpu.resetting.Load() {
				hostCooperativeYield()
			}
			cpu.resetAck.Store(false)
			continue
		}
		if !cpu.rdyLine.Load() {
			cpu.rdyHold = true
			hostCooperativeYield()
			continue
		}
		cpu.rdyHold = false
		if cpu.nmiPending.Load() {
			cpu.handleInterrupt(NMI_VECTOR, true)
			cpu.nmiPending.Store(false)
		} else if cpu.irqPending.Load() && cpu.SR&INTERRUPT_FLAG == 0 {
			cpu.handleInterrupt(IRQ_VECTOR, false)
			cpu.irqPending.Store(false)
		}
		if cpu.jitTestStopAfter == 0 {
			if adapter, ok := cpu.memory.(*Bus6502Adapter); ok {
				if matched, retired := cpu.wasmRun6502MMIOPollLoop(adapter); matched {
					if cpu.PerfEnabled {
						cpu.InstructionCount += uint64(retired)
					}
					continue
				}
			}
		}
		pc := cpu.PC
		if cpu.fastAdapter != nil && cpu.fastAdapter.memDirect[pc] == 0x00 && cpu.debugFaults != nil {
			cpu.interpret6502One()
			continue
		}
		blk := rt.cache[pc]
		if blk != nil && !rt.sourceMatches(pc, blk) {
			delete(rt.cache, pc)
			blk = nil
		}
		if blk == nil {
			blk = rt.compile(pc)
			if blk != nil {
				rt.cache[pc] = blk
			}
		}
		if blk == nil {
			cpu.interpret6502One()
			cpu.jitStats.bails.Add(1)
			cpu.jitTestRetire(1)
			continue
		}
		binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffNeedBail:], 0)
		binary.LittleEndian.PutUint32(rt.ctx[p65WasmCtxOffRetCount:], 0)
		binary.LittleEndian.PutUint64(rt.ctx[p65WasmCtxOffRetCycles:], 0)
		if !rt.invoke(blk) {
			delete(rt.cache, pc)
			cpu.interpret6502One()
			cpu.jitStats.bails.Add(1)
			cpu.jitTestRetire(1)
			continue
		}
		needBail := binary.LittleEndian.Uint32(rt.ctx[p65WasmCtxOffNeedBail:])
		executed := uint64(binary.LittleEndian.Uint32(rt.ctx[p65WasmCtxOffRetCount:]))
		cpu.PC = uint16(binary.LittleEndian.Uint32(rt.ctx[p65WasmCtxOffRetPC:]))
		cpu.Cycles += binary.LittleEndian.Uint64(rt.ctx[p65WasmCtxOffRetCycles:])
		cpu.jitStats.nativeEntries.Add(1)
		if needBail != 0 {
			cpu.interpret6502One()
			executed++
		}
		cpu.jitTestRetire(uint32(executed))
		// A straight-line wasm block does not otherwise re-enter the
		// interpreter's cooperative scheduling path. Throttle at its boundary
		// so a supported guest loop cannot monopolise the browser event loop.
		hostCooperativeYield()
	}
}
