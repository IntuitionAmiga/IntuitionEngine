//go:build !js

package main

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/tetratelabs/wazero"
)

func buildWasmMemoryModule(t *testing.T) []byte {
	t.Helper()
	m := newWasmModuleBuilder()
	m.defineMemory(1)
	m.exportMemory("mem")
	return m.build()
}

func instantiateNamed(t *testing.T, r wazero.Runtime, ctx context.Context, name string, mod []byte) {
	t.Helper()
	if _, err := r.InstantiateWithConfig(ctx, mod, wazero.NewModuleConfig().WithName(name)); err != nil {
		t.Fatalf("instantiate %s: %v", name, err)
	}
}

func TestX86WasmCompileBlockModule_JMPWritesRetPCAndCount(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x200)
	mem[startPC+0] = 0xEB
	mem[startPC+1] = 0x0E // -> 0x110
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	r := wazero.NewRuntime(context.Background())
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	ctx := context.Background()
	instantiateNamed(t, r, ctx, "env", buildWasmMemoryModule(t))
	mod, err := r.Instantiate(ctx, compiled.module)
	if err != nil {
		t.Fatalf("instantiate block: %v", err)
	}
	memExport := mod.Memory()
	ctxAddr := uint32(0x80)
	if !memExport.Write(ctxAddr, make([]byte, 256)) {
		t.Fatal("write ctx")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	buf, ok := memExport.Read(ctxAddr+x86CtxOffRetPC, 4)
	if !ok {
		t.Fatal("read RetPC")
	}
	if got := binary.LittleEndian.Uint32(buf); got != 0x110 {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x110))
	}
	buf, ok = memExport.Read(ctxAddr+x86CtxOffRetCount, 4)
	if !ok {
		t.Fatal("read RetCount")
	}
	if got := binary.LittleEndian.Uint32(buf); got != 1 {
		t.Fatalf("RetCount=%d want 1", got)
	}
}

func TestX86WasmCompileBlockModule_MOVFormsMutateJITRegs(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x200)
	copy(mem[startPC:], []byte{
		0xB8, 0x44, 0x33, 0x22, 0x11, // MOV EAX,0x11223344
		0xB0, 0x7F, // MOV AL,0x7F
		0x89, 0xC1, // MOV ECX,EAX
		0xEB, 0x00, // JMP +0 -> 0x10B
	})
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	r := wazero.NewRuntime(context.Background())
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	ctx := context.Background()
	instantiateNamed(t, r, ctx, "env", buildWasmMemoryModule(t))
	mod, err := r.Instantiate(ctx, compiled.module)
	if err != nil {
		t.Fatalf("instantiate block: %v", err)
	}
	memExport := mod.Memory()
	const (
		ctxAddr  = uint32(0x80)
		regsAddr = uint32(0xC0)
	)
	if !memExport.Write(ctxAddr, make([]byte, 128)) {
		t.Fatal("write ctx")
	}
	if !memExport.Write(regsAddr, make([]byte, 32)) {
		t.Fatal("write regs")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readReg := func(idx int) uint32 {
		b, ok := memExport.Read(regsAddr+uint32(idx*4), 4)
		if !ok {
			t.Fatalf("read reg %d", idx)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readReg(0); got != 0x1122337F {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x1122337F))
	}
	if got := readReg(1); got != 0x1122337F {
		t.Fatalf("ECX=%#x want %#x", got, uint32(0x1122337F))
	}
	if got := compiled.retPC; got != 0x10B {
		t.Fatalf("retPC=%#x want %#x", got, uint32(0x10B))
	}
}

func TestX86WasmCompileBlockModule_RegisterTransforms(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{
		0xB8, 0x78, 0x56, 0x34, 0x12, // MOV EAX,0x12345678
		0x0F, 0xC8, // BSWAP EAX
		0xBB, 0x80, 0xFF, 0x00, 0x00, // MOV EBX,0x0000FF80
		0x0F, 0xBE, 0xCB, // MOVSX ECX,BL
		0x87, 0xD8, // XCHG EAX,EBX
		0xEB, 0x00, // JMP +0
	})
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	r := wazero.NewRuntime(context.Background())
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	ctx := context.Background()
	instantiateNamed(t, r, ctx, "env", buildWasmMemoryModule(t))
	mod, err := r.Instantiate(ctx, compiled.module)
	if err != nil {
		t.Fatalf("instantiate block: %v", err)
	}
	memExport := mod.Memory()
	const (
		ctxAddr  = uint32(0x80)
		regsAddr = uint32(0xC0)
	)
	if !memExport.Write(ctxAddr, make([]byte, 128)) {
		t.Fatal("write ctx")
	}
	if !memExport.Write(regsAddr, make([]byte, 32)) {
		t.Fatal("write regs")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readReg := func(idx int) uint32 {
		b, ok := memExport.Read(regsAddr+uint32(idx*4), 4)
		if !ok {
			t.Fatalf("read reg %d", idx)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readReg(0); got != 0x0000FF80 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x0000FF80))
	}
	if got := readReg(1); got != 0xFFFFFF80 {
		t.Fatalf("ECX=%#x want %#x", got, uint32(0xFFFFFF80))
	}
	if got := readReg(3); got != 0x78563412 {
		t.Fatalf("EBX=%#x want %#x", got, uint32(0x78563412))
	}
}

func TestX86WasmBuildDriverModule_ChainsAcrossCachedBlocks(t *testing.T) {
	const (
		ctxAddr    = uint32(0x80)
		cacheBase  = uint32(0x200)
		cacheMask  = uint32(0x0F)
		startPC    = uint32(0x100)
		secondPC   = uint32(0x108)
		finalPC    = uint32(0x118)
		entry0Slot = uint32(1)
		entry1Slot = uint32(2)
	)
	m := newWasmModuleBuilder()
	m.defineMemory(1)
	m.defineTable(4)
	typ := m.addType([]byte{wasmTypeI32}, nil)

	b0 := &wasmBody{}
	x86WasmEmitRetPCAndCount(b0, secondPC, 1)
	b0.end()
	f0 := m.addFunc(typ, nil, b0.code)

	b1 := &wasmBody{}
	x86WasmEmitRetPCAndCount(b1, finalPC, 1)
	b1.end()
	f1 := m.addFunc(typ, nil, b1.code)

	m.elemSeed(0, []uint32{f0, f1})
	m.exportMemory("mem")
	m.exportTable("tab")
	env := m.build()

	r := wazero.NewRuntime(context.Background())
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	ctx := context.Background()
	instantiateNamed(t, r, ctx, "env", env)
	driver, err := r.Instantiate(ctx, x86WasmBuildDriverModule(cacheBase, cacheMask))
	if err != nil {
		t.Fatalf("instantiate driver: %v", err)
	}
	memExport := driver.Memory()
	if !memExport.Write(ctxAddr, make([]byte, 512)) {
		t.Fatal("write ctx")
	}
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, startPC)
	if !memExport.Write(ctxAddr+x86CtxOffRetPC, buf) {
		t.Fatal("seed RetPC")
	}
	binary.LittleEndian.PutUint32(buf, 4)
	if !memExport.Write(ctxAddr+x86CtxOffChainBudget, buf) {
		t.Fatal("seed ChainBudget")
	}

	writeCache := func(pc, slot uint32) {
		idx := pc & cacheMask
		entry := cacheBase + (idx << 3)
		binary.LittleEndian.PutUint32(buf, pc)
		if !memExport.Write(entry, buf) {
			t.Fatalf("write cache tag %#x", pc)
		}
		binary.LittleEndian.PutUint32(buf, slot)
		if !memExport.Write(entry+4, buf) {
			t.Fatalf("write cache slot %#x", pc)
		}
	}
	writeCache(startPC, entry0Slot)
	writeCache(secondPC, entry1Slot)

	if _, err := driver.ExportedFunction("drive").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call driver: %v", err)
	}
	readU32 := func(off uint32) uint32 {
		b, ok := memExport.Read(ctxAddr+off, 4)
		if !ok {
			t.Fatalf("read off %d", off)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(x86CtxOffRetPC); got != finalPC {
		t.Fatalf("RetPC=%#x want %#x", got, finalPC)
	}
	if got := readU32(x86CtxOffChainCount); got != 2 {
		t.Fatalf("ChainCount=%d want 2", got)
	}
	if got := readU32(x86CtxOffRetCount); got != 0 {
		t.Fatalf("RetCount=%d want 0 after chained accumulation", got)
	}
}

func TestX86WasmCompileRegionModule_FlattensForwardJumpChain(t *testing.T) {
	mem := make([]byte, 0x130)
	copy(mem[0x100:], []byte{
		0xB8, 0x78, 0x56, 0x34, 0x12, // MOV EAX,0x12345678
		0xEB, 0x09, // -> 0x110
	})
	copy(mem[0x110:], []byte{
		0x8B, 0xC8, // MOV ECX,EAX
		0xEB, 0x0C, // -> 0x120
	})
	copy(mem[0x120:], []byte{
		0xB0, 0xAA, // MOV AL,0xAA
		0xEB, 0x0C, // -> 0x130
	})
	region := x86FormRegion(0x100, NewCodeCache(), mem)
	if region == nil || len(region.blocks) != 3 {
		t.Fatalf("region=%#v", region)
	}
	compiled, err := x86WasmCompileRegionModule(region, mem)
	if err != nil {
		t.Fatalf("compile region: %v", err)
	}
	r := wazero.NewRuntime(context.Background())
	t.Cleanup(func() { _ = r.Close(context.Background()) })
	ctx := context.Background()
	instantiateNamed(t, r, ctx, "env", buildWasmMemoryModule(t))
	mod, err := r.Instantiate(ctx, compiled.module)
	if err != nil {
		t.Fatalf("instantiate region: %v", err)
	}
	memExport := mod.Memory()
	const (
		ctxAddr  = uint32(0x80)
		regsAddr = uint32(0xC0)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) {
		t.Fatal("write ctx")
	}
	if !memExport.Write(regsAddr, make([]byte, 32)) {
		t.Fatal("write regs")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call region: %v", err)
	}
	readU32 := func(off uint32) uint32 {
		b, ok := memExport.Read(ctxAddr+off, 4)
		if !ok {
			t.Fatalf("read off %d", off)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(x86CtxOffRetPC); got != 0x130 {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x130))
	}
	if got := readU32(x86CtxOffRetCount); got != 6 {
		t.Fatalf("RetCount=%d want 6", got)
	}
	if got := compiled.block.tier; got != 2 {
		t.Fatalf("tier=%d want 2", got)
	}
	readReg := func(idx int) uint32 {
		b, ok := memExport.Read(regsAddr+uint32(idx*4), 4)
		if !ok {
			t.Fatalf("read reg %d", idx)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readReg(0); got != 0x123456AA {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x123456AA))
	}
	if got := readReg(1); got != 0x12345678 {
		t.Fatalf("ECX=%#x want %#x", got, uint32(0x12345678))
	}
}

func TestX86WasmCompileSubsetManifestRows(t *testing.T) {
	supported := map[string]bool{
		"MOV r32,imm32":               true,
		"MOV r8,imm8":                 true,
		"MOV r/m32,r32":               true,
		"MOV r32,r/m32":               true,
		"XCHG":                        true,
		"MOVZX/MOVSX":                 true,
		"BSWAP":                       true,
		"ignored operand-size prefix": true,
		"WAIT":                        true,
		"near JMP":                    true,
	}
	const pc = uint32(0x100)
	for _, row := range x86JITCoverageManifest {
		if !supported[row.form] {
			continue
		}
		mem := make([]byte, int(pc)+len(row.sample))
		copy(mem[pc:], row.sample)
		instrs := x86ScanBlock(mem, pc)
		if len(instrs) == 0 {
			t.Fatalf("%s: scanner returned no instructions", row.form)
		}
		if _, err := x86WasmCompileBlockModule(instrs, pc, mem); err != nil {
			t.Fatalf("%s (% X): compile failed: %v", row.form, row.sample, err)
		}
	}
}
