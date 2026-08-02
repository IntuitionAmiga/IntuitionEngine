// jit_x86_wasm_emit.go - pure-Go x86 js/wasm module builder helpers.
//
// This file deliberately holds only the backend-independent wasm bytecode
// construction so native tests can validate emitted x86 wasm blocks under
// wazero even when the host toolchain cannot execute GOOS=js binaries.

package main

import "fmt"

const (
	x86WasmDriverCacheEntries = 4096
	x86WasmDriverTableInitial = 256
	x86WasmChainBudget        = 256
)

type x86WasmCompiledModule struct {
	module []byte
	block  *JITBlock
	retPC  uint32
}

func x86WasmImmediate8(ji X86JITInstr, memory []byte) byte {
	return memory[ji.opcodePC+uint32(ji.length)-1]
}

func x86WasmImmediate32(ji X86JITInstr, memory []byte) uint32 {
	immPC := ji.opcodePC + uint32(ji.length) - 4
	return uint32(memory[immPC]) |
		uint32(memory[immPC+1])<<8 |
		uint32(memory[immPC+2])<<16 |
		uint32(memory[immPC+3])<<24
}

func x86WasmSupportedInstr(ji X86JITInstr) bool {
	if ji.opcode >= 0x0F00 {
		op2 := byte(ji.opcode)
		switch op2 {
		case 0xB6, 0xB7, 0xBE, 0xBF:
			return ji.hasModRM && ji.modrm>>6 == 3
		case 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF:
			return true
		}
		return false
	}
	op := byte(ji.opcode)
	switch {
	case op == 0x90 || op == 0x9B:
		return true
	case op >= 0x91 && op <= 0x97:
		return true
	case op >= 0xB8 && op <= 0xBF:
		return true
	case op >= 0xB0 && op <= 0xB7:
		return true
	case op == 0x87:
		return ji.hasModRM && ji.modrm>>6 == 3
	case op == 0x89 || op == 0x8B:
		return ji.hasModRM && ji.modrm>>6 == 3
	case op == 0xEB || op == 0xE9:
		return true
	}
	return false
}

func x86WasmBlockTerminalPC(instrs []X86JITInstr, memory []byte, startPC uint32) (uint32, bool) {
	if len(instrs) == 0 {
		return 0, false
	}
	for i, ji := range instrs {
		op := byte(ji.opcode)
		if !x86WasmSupportedInstr(ji) {
			return 0, false
		}
		if ji.opcode >= 0x0F00 {
			switch op {
			case 0xB6, 0xB7, 0xBE, 0xBF, 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF:
				continue
			default:
				return 0, false
			}
		}
		switch op {
		case 0x90, 0x9B, 0x89, 0x8B: // NOP, WAIT, register MOVs
			continue
		case 0x87:
			continue
		case 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97:
			continue
		case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF:
			continue
		case 0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7:
			continue
		case 0xE9, 0xEB: // JMP rel32 / rel8
			if i != len(instrs)-1 {
				return 0, false
			}
			target, ok := x86ResolveTerminatorTarget(&ji, memory, startPC)
			if !ok {
				return 0, false
			}
			return target, true
		default:
			return 0, false
		}
	}
	last := instrs[len(instrs)-1]
	return last.opcodePC + uint32(last.length), true
}

func x86WasmEmitLoadReg32(b *wasmBody, locRegs uint32, reg byte) {
	b.localGet(locRegs)
	b.i32Const(int32(reg) * 4)
	b.op(wasmOpI32Add)
	b.i32Load(2, 0)
}

func x86WasmEmitStoreReg32(b *wasmBody, locRegs, locTmp uint32, reg byte) {
	b.localSet(locTmp)
	b.localGet(locRegs)
	b.i32Const(int32(reg) * 4)
	b.op(wasmOpI32Add)
	b.localGet(locTmp)
	b.i32Store(2, 0)
}

func x86WasmEmitExtractReg8(b *wasmBody, locRegs uint32, reg byte) {
	idx, shift := x86JITReg8Index(reg)
	x86WasmEmitLoadReg32(b, locRegs, byte(idx))
	if shift != 0 {
		b.i32Const(int32(shift))
		b.op(wasmOpI32ShrU)
	}
	b.i32Const(0xFF)
	b.op(wasmOpI32And)
}

func x86WasmEmitByteSwap32(b *wasmBody, locRegs, locTmp uint32, reg byte) {
	x86WasmEmitLoadReg32(b, locRegs, reg)
	b.i32Const(24)
	b.op(wasmOpI32ShrU)
	b.i32Const(0xFF)
	b.op(wasmOpI32And)

	x86WasmEmitLoadReg32(b, locRegs, reg)
	b.i32Const(8)
	b.op(wasmOpI32ShrU)
	b.i32Const(0xFF00)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)

	x86WasmEmitLoadReg32(b, locRegs, reg)
	b.i32Const(8)
	b.op(wasmOpI32Shl)
	b.i32Const(0xFF0000)
	b.op(wasmOpI32And)
	b.op(wasmOpI32Or)

	x86WasmEmitLoadReg32(b, locRegs, reg)
	b.i32Const(24)
	b.op(wasmOpI32Shl)
	b.op(wasmOpI32Or)
	x86WasmEmitStoreReg32(b, locRegs, locTmp, reg)
}

func x86WasmEmitInstr(b *wasmBody, ji X86JITInstr, memory []byte, locRegs, locTmp, locTmp2 uint32) bool {
	if ji.opcode >= 0x0F00 {
		op2 := byte(ji.opcode)
		switch op2 {
		case 0xB6: // MOVZX r32, r/m8
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			x86WasmEmitExtractReg8(b, locRegs, src)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
			return true
		case 0xB7: // MOVZX r32, r/m16
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			x86WasmEmitLoadReg32(b, locRegs, src)
			b.i32Const(0xFFFF)
			b.op(wasmOpI32And)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
			return true
		case 0xBE: // MOVSX r32, r/m8
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			x86WasmEmitExtractReg8(b, locRegs, src)
			b.i32Const(24)
			b.op(wasmOpI32Shl)
			b.i32Const(24)
			b.op(wasmOpI32ShrS)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
			return true
		case 0xBF: // MOVSX r32, r/m16
			dst := (ji.modrm >> 3) & 7
			src := ji.modrm & 7
			x86WasmEmitLoadReg32(b, locRegs, src)
			b.i32Const(16)
			b.op(wasmOpI32Shl)
			b.i32Const(16)
			b.op(wasmOpI32ShrS)
			x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
			return true
		case 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF: // BSWAP r32
			x86WasmEmitByteSwap32(b, locRegs, locTmp, op2-0xC8)
			return true
		default:
			return false
		}
	}
	op := byte(ji.opcode)
	switch {
	case op == 0x90 || op == 0x9B:
		return true
	case op >= 0x91 && op <= 0x97: // XCHG EAX, r32
		reg := op - 0x90
		x86WasmEmitLoadReg32(b, locRegs, 0)
		b.localSet(locTmp2)
		x86WasmEmitLoadReg32(b, locRegs, reg)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, 0)
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, reg)
		return true
	case op >= 0xB8 && op <= 0xBF:
		b.i32Const(int32(x86WasmImmediate32(ji, memory)))
		x86WasmEmitStoreReg32(b, locRegs, locTmp, op-0xB8)
		return true
	case op >= 0xB0 && op <= 0xB7:
		reg, shift := x86JITReg8Index(op - 0xB0)
		mask := ^(uint32(0xFF) << shift)
		x86WasmEmitLoadReg32(b, locRegs, byte(reg))
		b.i32Const(int32(mask))
		b.op(wasmOpI32And)
		b.i32Const(int32(uint32(x86WasmImmediate8(ji, memory)) << shift))
		b.op(wasmOpI32Or)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, byte(reg))
		return true
	case op == 0x89 && ji.hasModRM && ji.modrm>>6 == 3:
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, src)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
		return true
	case op == 0x8B && ji.hasModRM && ji.modrm>>6 == 3:
		dst := (ji.modrm >> 3) & 7
		src := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, src)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
		return true
	case op == 0x87 && ji.hasModRM && ji.modrm>>6 == 3: // XCHG r/m32, r32
		src := (ji.modrm >> 3) & 7
		dst := ji.modrm & 7
		x86WasmEmitLoadReg32(b, locRegs, dst)
		b.localSet(locTmp2)
		x86WasmEmitLoadReg32(b, locRegs, src)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, dst)
		b.localGet(locTmp2)
		x86WasmEmitStoreReg32(b, locRegs, locTmp, src)
		return true
	case op == 0xE9 || op == 0xEB:
		return true
	default:
		return false
	}
}

func x86WasmEmitRetPCAndCount(b *wasmBody, retPC uint32, instrCount int) {
	b.localGet(0)
	b.i32Const(int32(retPC))
	b.i32Store(2, x86CtxOffRetPC)
	b.localGet(0)
	b.i32Const(int32(instrCount))
	b.i32Store(2, x86CtxOffRetCount)
}

func x86WasmCompileBlockModule(instrs []X86JITInstr, startPC uint32, memory []byte) (*x86WasmCompiledModule, error) {
	retPC, ok := x86WasmBlockTerminalPC(instrs, memory, startPC)
	if !ok {
		return nil, fmt.Errorf("x86 wasm: unsupported block at %#x", startPC)
	}
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)
	const (
		locCtx  = 0
		locRegs = 1
		locTmp  = 2
		locTmp2 = 3
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffJITRegsPtr)
	b.localSet(locRegs)
	for _, ji := range instrs {
		if !x86WasmEmitInstr(b, ji, memory, locRegs, locTmp, locTmp2) {
			return nil, fmt.Errorf("x86 wasm: unsupported opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
		}
	}
	x86WasmEmitRetPCAndCount(b, retPC, len(instrs))
	b.end()
	fn := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32, wasmTypeI32}, b.code)
	m.exportFunc("block", fn)
	block := &JITBlock{
		startPC:          uint64(startPC),
		endPC:            uint64(retPC),
		instrCount:       len(instrs),
		x86CyclePrefix:   x86JITCyclePrefix(instrs),
		x86TickPrefix:    x86JITTickPrefix(instrs),
		x86DynamicCycles: x86JITDynamicCycles(instrs),
	}
	return &x86WasmCompiledModule{
		module: m.build(),
		block:  block,
		retPC:  retPC,
	}, nil
}

func x86WasmCompileRegionModule(region *x86Region, memory []byte) (*x86WasmCompiledModule, error) {
	if region == nil || len(region.blocks) < 2 {
		return nil, fmt.Errorf("x86 wasm: empty or single-block region")
	}
	all := make([]X86JITInstr, 0, 16)
	covered := make([][2]uint64, 0, len(region.blocks))
	var retPC uint32
	var maxEnd uint64
	for i, block := range region.blocks {
		blockPC := region.blockPCs[i]
		nextPC, ok := x86WasmBlockTerminalPC(block, memory, blockPC)
		if !ok {
			return nil, fmt.Errorf("x86 wasm: unsupported region block %#x", blockPC)
		}
		if i != len(region.blocks)-1 && nextPC != region.blockPCs[i+1] {
			return nil, fmt.Errorf("x86 wasm: non-linear successor %#x -> %#x", blockPC, nextPC)
		}
		retPC = nextPC
		all = append(all, block...)
		start := uint64(blockPC)
		end := uint64(block[len(block)-1].opcodePC + uint32(block[len(block)-1].length))
		covered = append(covered, [2]uint64{start, end})
		if end > maxEnd {
			maxEnd = end
		}
	}
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)
	const (
		locCtx  = 0
		locRegs = 1
		locTmp  = 2
		locTmp2 = 3
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffJITRegsPtr)
	b.localSet(locRegs)
	for _, ji := range all {
		if !x86WasmEmitInstr(b, ji, memory, locRegs, locTmp, locTmp2) {
			return nil, fmt.Errorf("x86 wasm: unsupported region opcode %#02x at %#x", byte(ji.opcode), ji.opcodePC)
		}
	}
	x86WasmEmitRetPCAndCount(b, retPC, len(all))
	b.end()
	fn := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32, wasmTypeI32}, b.code)
	m.exportFunc("block", fn)
	block := &JITBlock{
		startPC:          uint64(region.entryPC),
		endPC:            maxEnd,
		instrCount:       len(all),
		x86CyclePrefix:   x86JITCyclePrefix(all),
		x86TickPrefix:    x86JITTickPrefix(all),
		x86DynamicCycles: x86JITDynamicCycles(all),
		tier:             2,
		coveredRanges:    covered,
	}
	return &x86WasmCompiledModule{
		module: m.build(),
		block:  block,
		retPC:  retPC,
	}, nil
}

func x86WasmBuildDriverModule(cacheBase uint32, cacheMask uint32) []byte {
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	m.importTable("env", "tab", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)

	const (
		locCtx = 0
		locPC  = 1
		locT   = 2
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffRetPC)
	b.localSet(locPC)
	b.block()
	b.loop()
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffChainBudget)
	b.localTee(locT)
	b.op(wasmOpI32Eqz)
	b.brIf(1)
	b.localGet(locCtx)
	b.localGet(locT)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.i32Store(2, x86CtxOffChainBudget)

	b.localGet(locPC)
	b.i32Const(int32(cacheMask))
	b.op(wasmOpI32And)
	b.i32Const(3)
	b.op(wasmOpI32Shl)
	b.i32Const(int32(cacheBase))
	b.op(wasmOpI32Add)
	b.localTee(locT)

	b.localGet(locT)
	b.i32Load(2, 0)
	b.localGet(locPC)
	b.op(wasmOpI32Ne)
	b.brIf(1)

	b.localGet(locT)
	b.i32Load(2, 4)
	b.localTee(locT)
	b.op(wasmOpI32Eqz)
	b.brIf(1)

	b.localGet(locCtx)
	b.localGet(locT)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.callIndirect(typ)

	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffNeedIOFallback)
	b.brIf(1)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffNeedInval)
	b.brIf(1)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffExitReason)
	b.brIf(1)

	b.localGet(locCtx)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffChainCount)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffRetCount)
	b.op(wasmOpI32Add)
	b.i32Store(2, x86CtxOffChainCount)
	b.localGet(locCtx)
	b.i32Const(0)
	b.i32Store(2, x86CtxOffRetCount)
	b.localGet(locCtx)
	b.i32Load(2, x86CtxOffRetPC)
	b.localSet(locPC)
	b.br(0)
	b.end()
	b.end()
	b.end()

	fn := m.addFunc(typ, []byte{wasmTypeI32, wasmTypeI32}, b.code)
	m.exportFunc("drive", fn)
	return m.build()
}
