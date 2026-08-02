//go:build !js

package main

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
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

func TestX86WasmCompileBlockModule_TESTAndJccUsesGuestFlags(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x200)
	copy(mem[startPC:], []byte{
		0x85, 0xC0, // TEST EAX,EAX
		0x74, 0x02, // JZ +2
		0x90, // NOP that must remain outside the compiled prefix
		0xF4, // HLT
	})
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := compiled.block.instrCount; got != 2 {
		t.Fatalf("instrCount=%d want 2", got)
	}
	if got := compiled.block.endPC; got != 0x104 {
		t.Fatalf("endPC=%#x want %#x", got, uint64(0x104))
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	runCase := func(name string, eax, flags, wantPC, wantFlags uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) {
				t.Fatal("write ctx")
			}
			if !memExport.Write(regsAddr, make([]byte, 32)) {
				t.Fatal("write regs")
			}
			if !memExport.Write(flagsAddr, make([]byte, 4)) {
				t.Fatal("write flags")
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, eax)
			if !memExport.Write(regsAddr, buf) {
				t.Fatal("seed EAX")
			}
			binary.LittleEndian.PutUint32(buf, flags)
			if !memExport.Write(flagsAddr, buf) {
				t.Fatal("seed flags")
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != 2 {
				t.Fatalf("RetCount=%d want 2", got)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantPC {
				t.Fatalf("RetPC=%#x want %#x", got, wantPC)
			}
			if got := readU32(flagsAddr); got&x86VisibleFlagsMask != wantFlags {
				t.Fatalf("Flags=%#x want %#x", got&x86VisibleFlagsMask, wantFlags)
			}
		})
	}
	runCase("zero_sets_zf_and_takes_branch", 0, x86FlagAF|x86FlagCF|x86FlagOF, 0x106, x86FlagAF|x86FlagZF|x86FlagPF)
	runCase("nonzero_clears_zf_and_falls_through", 1, x86FlagAF|x86FlagZF|x86FlagPF, 0x104, x86FlagAF)
}

func TestX86WasmCompileBlockModule_SETccAndCMOVccUseGuestFlags(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{
		0x0F, 0x94, 0xC4, // SETZ AH
		0x0F, 0x44, 0xC8, // CMOVZ ECX,EAX
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	readReg := func(idx int) uint32 {
		b, ok := memExport.Read(regsAddr+uint32(idx*4), 4)
		if !ok {
			t.Fatalf("read reg %d", idx)
		}
		return binary.LittleEndian.Uint32(b)
	}
	runCase := func(name string, flags, wantEAX, wantECX uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) {
				t.Fatal("write ctx")
			}
			if !memExport.Write(regsAddr, make([]byte, 32)) {
				t.Fatal("write regs")
			}
			if !memExport.Write(flagsAddr, make([]byte, 4)) {
				t.Fatal("write flags")
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, 0x11223344)
			if !memExport.Write(regsAddr+0*4, buf) {
				t.Fatal("seed EAX")
			}
			binary.LittleEndian.PutUint32(buf, 0xAABBCCDD)
			if !memExport.Write(regsAddr+1*4, buf) {
				t.Fatal("seed ECX")
			}
			binary.LittleEndian.PutUint32(buf, flags)
			if !memExport.Write(flagsAddr, buf) {
				t.Fatal("seed flags")
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			if got := readReg(0); got != wantEAX {
				t.Fatalf("EAX=%#x want %#x", got, wantEAX)
			}
			if got := readReg(1); got != wantECX {
				t.Fatalf("ECX=%#x want %#x", got, wantECX)
			}
		})
	}
	runCase("zf_set", x86FlagZF, 0x11220144, 0x11220144)
	runCase("zf_clear", 0, 0x11220044, 0xAABBCCDD)
}

func TestX86WasmCompileBlockModule_BSFBSRParity(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x400)
	copy(mem[startPC:], []byte{
		0x0F, 0xBC, 0xC3, // BSF EAX,EBX
		0x0F, 0xBD, 0x43, 0x04, // BSR EAX,[EBX+4]
		0x66, 0x0F, 0xBC, 0xC3, // BSF AX,BX
		0xEB, 0x00,
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
		guestBase = uint32(0x180)
		ioBMAddr  = uint32(0x280)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	readReg := func(idx int) uint32 {
		b, ok := memExport.Read(regsAddr+uint32(idx*4), 4)
		if !ok {
			t.Fatalf("read reg %d", idx)
		}
		return binary.LittleEndian.Uint32(b)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if !memExport.Write(ctxAddr, make([]byte, 256)) ||
		!memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) ||
		!memExport.Write(guestBase, make([]byte, 256)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x00000800)
	if !memExport.Write(regsAddr+3*4, word) { // EBX
		t.Fatal("seed EBX")
	}
	binary.LittleEndian.PutUint32(word, 0xAABBCCDD)
	if !memExport.Write(regsAddr, word) { // EAX
		t.Fatal("seed EAX")
	}
	binary.LittleEndian.PutUint32(word, x86FlagCF)
	if !memExport.Write(flagsAddr, word) {
		t.Fatal("seed flags")
	}
	binary.LittleEndian.PutUint32(word, 0x1000)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	binary.LittleEndian.PutUint32(word, 0x00001000)
	if !memExport.Write(guestBase+0x804, word) {
		t.Fatal("seed guest dword")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	if got := readReg(0); got != 0x0000000B {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x0000000B))
	}
	if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != x86FlagCF {
		t.Fatalf("Flags=%#x want %#x", got, uint32(x86FlagCF))
	}

	wordMem := make([]byte, 0x200)
	copy(wordMem[startPC:], []byte{
		0x66, 0x0F, 0xBC, 0xC3, // BSF AX,BX
		0xEB, 0x00,
	})
	wordInstrs := x86ScanBlock(wordMem, startPC)
	wordCompiled, err := x86WasmCompileBlockModule(wordInstrs, startPC, wordMem)
	if err != nil {
		t.Fatalf("compile word-size block: %v", err)
	}
	wordMod, err := r.Instantiate(ctx, wordCompiled.module)
	if err != nil {
		t.Fatalf("instantiate word-size block: %v", err)
	}
	if !memExport.Write(ctxAddr, make([]byte, 256)) ||
		!memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("reset word-size state")
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	binary.LittleEndian.PutUint32(word, 0xAABBCCDD)
	if !memExport.Write(regsAddr, word) { // EAX
		t.Fatal("seed word-size EAX")
	}
	binary.LittleEndian.PutUint32(word, 0x00000800)
	if !memExport.Write(regsAddr+3*4, word) { // EBX
		t.Fatal("seed word-size EBX")
	}
	binary.LittleEndian.PutUint32(word, x86FlagCF)
	if !memExport.Write(flagsAddr, word) {
		t.Fatal("seed word-size flags")
	}
	if _, err := wordMod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call word-size block: %v", err)
	}
	if got := readReg(0); got != 0xAABB000B {
		t.Fatalf("word-size EAX=%#x want %#x", got, uint32(0xAABB000B))
	}
	if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != x86FlagCF {
		t.Fatalf("word-size Flags=%#x want %#x", got, uint32(x86FlagCF))
	}

	zeroMem := make([]byte, 0x200)
	copy(zeroMem[startPC:], []byte{
		0x0F, 0xBC, 0xC3, // BSF EAX,EBX
		0xEB, 0x00,
	})
	zeroInstrs := x86ScanBlock(zeroMem, startPC)
	zeroCompiled, err := x86WasmCompileBlockModule(zeroInstrs, startPC, zeroMem)
	if err != nil {
		t.Fatalf("compile zero-source block: %v", err)
	}
	zeroMod, err := r.Instantiate(ctx, zeroCompiled.module)
	if err != nil {
		t.Fatalf("instantiate zero-source block: %v", err)
	}
	if !memExport.Write(ctxAddr, make([]byte, 256)) ||
		!memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("reset zero-source state")
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	binary.LittleEndian.PutUint32(word, 0x11223344)
	if !memExport.Write(regsAddr, word) { // EAX
		t.Fatal("seed zero-source EAX")
	}
	binary.LittleEndian.PutUint32(word, 0)
	if !memExport.Write(regsAddr+3*4, word) { // EBX
		t.Fatal("seed zero-source EBX")
	}
	binary.LittleEndian.PutUint32(word, x86FlagCF)
	if !memExport.Write(flagsAddr, word) {
		t.Fatal("seed zero-source flags")
	}
	if _, err := zeroMod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call zero-source block: %v", err)
	}
	if got := readReg(0); got != 0x11223344 {
		t.Fatalf("zero-source EAX=%#x want %#x", got, uint32(0x11223344))
	}
	if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != (x86FlagCF | x86FlagZF) {
		t.Fatalf("zero-source Flags=%#x want %#x", got, uint32(x86FlagCF|x86FlagZF))
	}
}

func TestX86WasmCompileBlockModule_LogicalALUAndCMPFeedConditionals(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0xBB, 0x0F, 0x00, 0x00, 0x00, // MOV EBX,0x0000000F
		0xBE, 0xF0, 0x00, 0x00, 0x00, // MOV ESI,0x000000F0
		0x21, 0xDE, // AND ESI,EBX -> 0
		0x0F, 0x44, 0xFB, // CMOVZ EDI,EBX -> 0x0F
		0xB8, 0xF0, 0x00, 0x00, 0x00, // MOV EAX,0x000000F0
		0x09, 0xD8, // OR EAX,EBX -> 0xFF
		0x0F, 0x9A, 0xC1, // SETP CL -> 1
		0x31, 0xC0, // XOR EAX,EAX -> 0
		0x0F, 0x44, 0xD3, // CMOVZ EDX,EBX -> 0x0F
		0x39, 0xD0, // CMP EAX,EDX -> CF=1
		0x72, 0x02, // JB +2
		0x90, // NOP outside compiled prefix
		0xF4, // HLT
	})
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := compiled.block.instrCount; got != 11 {
		t.Fatalf("instrCount=%d want 11", got)
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) {
		t.Fatal("write ctx")
	}
	if !memExport.Write(regsAddr, make([]byte, 32)) {
		t.Fatal("write regs")
	}
	if !memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("write flags")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
		t.Fatal("seed FlagsPtr")
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
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readReg(0); got != 0 {
		t.Fatalf("EAX=%#x want 0", got)
	}
	if got := readReg(1); got != 1 {
		t.Fatalf("ECX=%#x want 1", got)
	}
	if got := readReg(2); got != 0x0F {
		t.Fatalf("EDX=%#x want %#x", got, uint32(0x0F))
	}
	if got := readReg(6); got != 0 {
		t.Fatalf("ESI=%#x want 0", got)
	}
	if got := readReg(7); got != 0x0F {
		t.Fatalf("EDI=%#x want %#x", got, uint32(0x0F))
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0x124 {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x124))
	}
	if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != (x86FlagCF | x86FlagAF | x86FlagSF) {
		t.Fatalf("Flags=%#x want %#x", got, uint32(x86FlagCF|x86FlagAF|x86FlagSF))
	}
}

func TestX86WasmCompileBlockModule_DirectByteALURMReg(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0xB8, 0x80, 0xFE, 0x22, 0x11, // MOV EAX,0x1122FE80
		0xBB, 0x03, 0x03, 0x44, 0x33, // MOV EBX,0x33440303
		0x00, 0xD8, // ADD AL,BL -> 0x83
		0x08, 0xFC, // OR AH,BH -> 0xFF
		0x20, 0xD8, // AND AL,BL -> 0x03
		0x28, 0xFC, // SUB AH,BH -> 0xFC
		0x30, 0xFC, // XOR AH,BH -> 0xFF
		0x38, 0xD8, // CMP AL,BL -> ZF=1
		0x0F, 0x94, 0xC1, // SETZ CL
		0xEB, 0x00,
	})
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := compiled.block.instrCount; got != 10 {
		t.Fatalf("instrCount=%d want 10", got)
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) {
		t.Fatal("write ctx")
	}
	if !memExport.Write(regsAddr, make([]byte, 32)) {
		t.Fatal("write regs")
	}
	if !memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("write flags")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
		t.Fatal("seed FlagsPtr")
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
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readReg(0); got != 0x1122FF03 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x1122FF03))
	}
	if got := readReg(1); got != 1 {
		t.Fatalf("ECX=%#x want 1", got)
	}
	if got := readReg(3); got != 0x33440303 {
		t.Fatalf("EBX=%#x want %#x", got, uint32(0x33440303))
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0x11b {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x11b))
	}
	if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != (x86FlagZF | x86FlagPF) {
		t.Fatalf("Flags=%#x want %#x", got, uint32(x86FlagZF|x86FlagPF))
	}
}

func TestX86WasmCompileBlockModule_AddSubFeedConditionals(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0xB8, 0x03, 0x00, 0x00, 0x00, // MOV EAX,3
		0xBB, 0x01, 0x00, 0x00, 0x00, // MOV EBX,1
		0x29, 0xD8, // SUB EAX,EBX -> 2
		0x01, 0xD8, // ADD EAX,EBX -> 3
		0x2D, 0x03, 0x00, 0x00, 0x00, // SUB EAX,3 -> 0
		0x0F, 0x44, 0xCB, // CMOVZ ECX,EBX -> 1
		0xB8, 0xFF, 0xFF, 0xFF, 0x7F, // MOV EAX,0x7fffffff
		0x05, 0x01, 0x00, 0x00, 0x00, // ADD EAX,1 -> 0x80000000, OF=1
		0x70, 0x02, // JO +2
		0x90, // NOP outside compiled prefix
		0xF4, // HLT
	})
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := compiled.block.instrCount; got != 9 {
		t.Fatalf("instrCount=%d want 9", got)
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) {
		t.Fatal("write ctx")
	}
	if !memExport.Write(regsAddr, make([]byte, 32)) {
		t.Fatal("write regs")
	}
	if !memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("write flags")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
		t.Fatal("seed FlagsPtr")
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
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readReg(0); got != 0x80000000 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x80000000))
	}
	if got := readReg(1); got != 1 {
		t.Fatalf("ECX=%#x want 1", got)
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0x124 {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x124))
	}
	if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != (x86FlagOF | x86FlagSF | x86FlagPF) {
		t.Fatalf("Flags=%#x want %#x", got, uint32(x86FlagOF|x86FlagSF|x86FlagPF))
	}
}

func TestX86WasmCompileBlockModule_Group1ImmediateAndIncDecFeedConditionals(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0xB8, 0x05, 0x00, 0x00, 0x00, // MOV EAX,5
		0x40,             // INC EAX -> 6
		0x48,             // DEC EAX -> 5
		0x83, 0xE8, 0x05, // SUB EAX,+5 -> 0
		0x0F, 0x44, 0xCB, // CMOVZ ECX,EBX
		0x81, 0xF0, 0xFF, 0x00, 0x00, 0x00, // XOR EAX,0xFF -> 0xFF
		0x83, 0xC0, 0x01, // ADD EAX,+1 -> 0x100
		0x81, 0xF8, 0x00, 0x01, 0x00, 0x00, // CMP EAX,0x100 -> ZF=1
		0x0F, 0x44, 0xDA, // CMOVZ EBX,EDX
		0xB8, 0xFF, 0xFF, 0xFF, 0x7F, // MOV EAX,0x7fffffff
		0x40,       // INC EAX -> 0x80000000, CF preserved, OF set
		0x70, 0x02, // JO +2
		0x90,
		0xF4,
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) {
		t.Fatal("write ctx")
	}
	if !memExport.Write(regsAddr, make([]byte, 32)) {
		t.Fatal("write regs")
	}
	if !memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("write flags")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
		t.Fatal("seed FlagsPtr")
	}
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x11111111)
	if !memExport.Write(regsAddr+1*4, word) {
		t.Fatal("seed ECX")
	}
	binary.LittleEndian.PutUint32(word, 0x00000005)
	if !memExport.Write(regsAddr+3*4, word) {
		t.Fatal("seed EBX")
	}
	binary.LittleEndian.PutUint32(word, 0x12345678)
	if !memExport.Write(regsAddr+2*4, word) {
		t.Fatal("seed EDX")
	}
	binary.LittleEndian.PutUint32(word, x86FlagCF)
	if !memExport.Write(flagsAddr, word) {
		t.Fatal("seed flags")
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
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readReg(0); got != 0x80000000 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x80000000))
	}
	if got := readReg(1); got != 0x00000005 {
		t.Fatalf("ECX=%#x want %#x", got, uint32(0x00000005))
	}
	if got := readReg(3); got != 0x12345678 {
		t.Fatalf("EBX=%#x want %#x", got, uint32(0x12345678))
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0x129 {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x129))
	}
	if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != (x86FlagOF | x86FlagSF | x86FlagPF) {
		t.Fatalf("Flags=%#x want %#x", got, uint32(x86FlagOF|x86FlagSF|x86FlagPF))
	}
}

func TestX86WasmCompileBlockModule_ShiftsFeedConditionals(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // MOV EAX,1
		0xC1, 0xE0, 0x03, // SHL EAX,3 -> 8
		0xB9, 0x01, 0x00, 0x00, 0x00, // MOV ECX,1
		0xD3, 0xE8, // SHR EAX,CL -> 4
		0xD1, 0xF8, // SAR EAX,1 -> 2
		0x0F, 0x95, 0xC3, // SETNZ BL -> 1
		0xB8, 0x40, 0x00, 0x00, 0x00, // MOV EAX,0x40
		0xD1, 0xE0, // SHL EAX,1 -> 0x80
		0x73, 0x02, // JNC +2
		0x90,
		0xF4,
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) {
		t.Fatal("write ctx")
	}
	if !memExport.Write(regsAddr, make([]byte, 32)) {
		t.Fatal("write regs")
	}
	if !memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("write flags")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
		t.Fatal("seed FlagsPtr")
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
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readReg(0); got != 0x80 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x80))
	}
	if got := readReg(3); got != 1 {
		t.Fatalf("EBX=%#x want 1", got)
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0x11f {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x11f))
	}
	if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != 0 {
		t.Fatalf("Flags=%#x want 0", got)
	}
}

func TestX86WasmCompileBlockModule_DirectMemoryMOVReadsAndGuardBails(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{
		0x8B, 0x03, // MOV EAX,[EBX]
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	run := func(name string, ebx uint32, markMMIO bool, wantEAX uint32, wantRetPC uint32, wantRetCount uint32, wantIO uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
				!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) ||
				!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
				t.Fatal("seed memory")
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			writePtr(x86CtxOffMemPtr, guestBase)
			writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
			writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, 0x44332211)
			if !memExport.Write(guestBase+0x40, word) {
				t.Fatal("seed guest dword")
			}
			binary.LittleEndian.PutUint32(word, ebx)
			if !memExport.Write(regsAddr+3*4, word) { // EBX
				t.Fatal("seed EBX")
			}
			binary.LittleEndian.PutUint32(word, 0x200)
			if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
				t.Fatal("seed MemSize")
			}
			if markMMIO {
				if !memExport.Write(ioBMAddr, []byte{1}) {
					t.Fatal("seed io bitmap")
				}
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			if got := readU32(regsAddr + 0*4); got != wantEAX {
				t.Fatalf("EAX=%#x want %#x", got, wantEAX)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantRetPC {
				t.Fatalf("RetPC=%#x want %#x", got, wantRetPC)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != wantRetCount {
				t.Fatalf("RetCount=%d want %d", got, wantRetCount)
			}
			if got := readU32(ctxAddr + x86CtxOffNeedIOFallback); got != wantIO {
				t.Fatalf("NeedIOFallback=%d want %d", got, wantIO)
			}
		})
	}
	run("safe", 0x40, false, 0x44332211, 0x104, 2, 0)
	run("span_bail", 0xFF, false, 0, 0x100, 0, 1)
	run("mmio_bail", 0x40, true, 0, 0x100, 0, 1)
}

func TestX86WasmCompileBlockModule_DirectMemoryMOVStorePublishesInvalidation(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{
		0x89, 0x03, // MOV [EBX],EAX
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x20)
	if !memExport.Write(regsAddr+3*4, word) { // EBX
		t.Fatal("seed EBX")
	}
	binary.LittleEndian.PutUint32(word, 0x11223344)
	if !memExport.Write(regsAddr+0*4, word) { // EAX
		t.Fatal("seed EAX")
	}
	binary.LittleEndian.PutUint32(word, 0x200)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if !memExport.Write(codeBMAddr, []byte{1}) {
		t.Fatal("seed code bitmap")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(guestBase + 0x20); got != 0x11223344 {
		t.Fatalf("store=%#x want %#x", got, uint32(0x11223344))
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0x102 {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x102))
	}
	if got := readU32(ctxAddr + x86CtxOffRetCount); got != 1 {
		t.Fatalf("RetCount=%d want 1", got)
	}
	if got := readU32(ctxAddr + x86CtxOffNeedInval); got != 1 {
		t.Fatalf("NeedInval=%d want 1", got)
	}
}

func TestX86WasmCompileBlockModule_DirectMemoryByteMOVAndImmediateStores(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0x8A, 0x23, // MOV AH,[EBX]
		0x88, 0x43, 0x01, // MOV [EBX+1],AL
		0xC6, 0x43, 0x02, 0x7E, // MOV byte [EBX+2],7E
		0xC7, 0x43, 0x04, 0x44, 0x33, 0x22, 0x11, // MOV dword [EBX+4],11223344
		0xEB, 0x00,
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x40)
	if !memExport.Write(regsAddr+3*4, word) { // EBX
		t.Fatal("seed EBX")
	}
	binary.LittleEndian.PutUint32(word, 0x11223344)
	if !memExport.Write(regsAddr, word) { // EAX
		t.Fatal("seed EAX")
	}
	binary.LittleEndian.PutUint32(word, 0x200)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if !memExport.Write(guestBase+0x40, []byte{0xAB}) {
		t.Fatal("seed guest byte")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 0*4); got != 0x1122AB44 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x1122AB44))
	}
	if got, ok := memExport.Read(guestBase+0x41, 7); !ok {
		t.Fatal("read guest stores")
	} else if want := []byte{0x44, 0x7E, 0x00, 0x44, 0x33, 0x22, 0x11}; string(got) != string(want) {
		t.Fatalf("guest stores=% X want % X", got, want)
	}
}

func TestX86WasmCompileBlockModule_DirectMemoryMOVExtensions(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0x0F, 0xB6, 0x43, 0x01, // MOVZX EAX,byte [EBX+1]
		0x0F, 0xBF, 0x4B, 0x02, // MOVSX ECX,word [EBX+2]
		0xEB, 0x00,
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0x200)
		flagsAddr = uint32(0x280)
		guestBase = uint32(0x300)
		ioBMAddr  = uint32(0x600)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x20)
	if !memExport.Write(regsAddr+3*4, word) {
		t.Fatal("seed EBX")
	}
	binary.LittleEndian.PutUint32(word, 0x200)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if !memExport.Write(guestBase+0x21, []byte{0x80}) {
		t.Fatal("seed byte")
	}
	if !memExport.Write(guestBase+0x22, []byte{0x34, 0xFF}) {
		t.Fatal("seed word")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 0*4); got != 0x80 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x80))
	}
	if got := readU32(regsAddr + 1*4); got != 0xFFFFFF34 {
		t.Fatalf("ECX=%#x want %#x", got, uint32(0xFFFFFF34))
	}
}

func TestX86WasmCompileBlockModule_DirectMoffsMOV(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x300)
	copy(mem[startPC:], []byte{
		0xA0, 0x00, 0x04, 0x00, 0x00, // MOV AL,[0400]
		0xA2, 0x10, 0x04, 0x00, 0x00, // MOV [0410],AL
		0xA1, 0x00, 0x04, 0x00, 0x00, // MOV EAX,[0400]
		0xA3, 0x08, 0x04, 0x00, 0x00, // MOV [0408],EAX
		0x66, 0xA1, 0x04, 0x04, 0x00, 0x00, // MOV AX,[0404]
		0x66, 0xA3, 0x0C, 0x04, 0x00, 0x00, // MOV [040C],AX
		0xEB, 0x00,
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x900)
		codeBMAddr = uint32(0x980)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 2048)) ||
		!memExport.Write(ioBMAddr, make([]byte, 32)) || !memExport.Write(codeBMAddr, make([]byte, 32)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x1000)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if !memExport.Write(guestBase+0x400, []byte{0x44, 0x33, 0x22, 0x11, 0xCD, 0xAB}) {
		t.Fatal("seed guest source")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 0*4); got != 0x1122ABCD {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x1122ABCD))
	}
	if got, ok := memExport.Read(guestBase+0x408, 9); !ok {
		t.Fatal("read moffs stores")
	} else if want := []byte{0x44, 0x33, 0x22, 0x11, 0xCD, 0xAB, 0x00, 0x00, 0x44}; string(got) != string(want) {
		t.Fatalf("moffs stores=% X want % X", got, want)
	}
}

func TestX86WasmCompileBlockModule_DirectMemoryXCHG(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{
		0x87, 0x03, // XCHG dword [EBX],EAX
		0x66, 0x87, 0x4B, 0x04, // XCHG word [EBX+4],CX
		0xEB, 0x00,
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x20)
	if !memExport.Write(regsAddr+3*4, word) { // EBX
		t.Fatal("seed EBX")
	}
	binary.LittleEndian.PutUint32(word, 0x11223344)
	if !memExport.Write(regsAddr+0*4, word) { // EAX
		t.Fatal("seed EAX")
	}
	binary.LittleEndian.PutUint32(word, 0xBEEFCAFE)
	if !memExport.Write(regsAddr+1*4, word) { // ECX
		t.Fatal("seed ECX")
	}
	binary.LittleEndian.PutUint32(word, 0x200)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if !memExport.Write(guestBase+0x20, []byte{0xDD, 0xCC, 0xBB, 0xAA, 0x34, 0x12}) {
		t.Fatal("seed guest memory")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 0*4); got != 0xAABBCCDD {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0xAABBCCDD))
	}
	if got := readU32(regsAddr + 1*4); got != 0xBEEF1234 {
		t.Fatalf("ECX=%#x want %#x", got, uint32(0xBEEF1234))
	}
	if got, ok := memExport.Read(guestBase+0x20, 6); !ok {
		t.Fatal("read guest xchg memory")
	} else if want := []byte{0x44, 0x33, 0x22, 0x11, 0xFE, 0xCA}; string(got) != string(want) {
		t.Fatalf("guest xchg memory=% X want % X", got, want)
	}
}

func TestX86WasmCompileBlockModule_DirectRegisterPushPop(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{
		0x50,       // PUSH EAX
		0x66, 0x53, // PUSH BX
		0x59,       // POP ECX
		0x66, 0x5A, // POP DX
		0x54, // PUSH ESP
		0x5C, // POP ESP
		0xEB, 0x00,
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 2048)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	word := make([]byte, 4)
	for idx, val := range []uint32{0x11223344, 0xDEADBEEF, 0xA5A5A5A5, 0xBEEFCAFE, 0x500} {
		reg := []int{0, 1, 2, 3, 4}[idx]
		binary.LittleEndian.PutUint32(word, val)
		if !memExport.Write(regsAddr+uint32(reg*4), word) {
			t.Fatalf("seed reg %d", reg)
		}
	}
	binary.LittleEndian.PutUint32(word, 0x700)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 1*4); got != 0x3344CAFE {
		t.Fatalf("ECX=%#x want %#x", got, uint32(0x3344CAFE))
	}
	if got := readU32(regsAddr + 2*4); got != 0xA5A51122 {
		t.Fatalf("EDX=%#x want %#x", got, uint32(0xA5A51122))
	}
	if got := readU32(regsAddr + 4*4); got != 0x500 {
		t.Fatalf("ESP=%#x want %#x", got, uint32(0x500))
	}
	if got, ok := memExport.Read(guestBase+0x4F8, 8); !ok {
		t.Fatal("read stack bytes")
	} else if want := []byte{0x00, 0x00, 0xFE, 0xCA, 0x00, 0x05, 0x00, 0x00}; string(got) != string(want) {
		t.Fatalf("stack bytes=% X want % X", got, want)
	}
}

func TestX86WasmCompileBlockModule_DirectImmediatePush(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{
		0x68, 0x44, 0x33, 0x22, 0x11,
		0x66, 0x68, 0x34, 0x12,
		0x6A, 0x80,
		0x66, 0x6A, 0x80,
		0xEB, 0x00,
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 2048)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x500)
	if !memExport.Write(regsAddr+4*4, word) {
		t.Fatal("seed ESP")
	}
	binary.LittleEndian.PutUint32(word, 0x700)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 4*4); got != 0x4F4 {
		t.Fatalf("ESP=%#x want %#x", got, uint32(0x4F4))
	}
	if got, ok := memExport.Read(guestBase+0x4F4, 12); !ok {
		t.Fatal("read push stack")
	} else if want := []byte{0x80, 0xFF, 0x80, 0xFF, 0xFF, 0xFF, 0x34, 0x12, 0x44, 0x33, 0x22, 0x11}; string(got) != string(want) {
		t.Fatalf("push stack=% X want % X", got, want)
	}
}

func TestX86WasmCompileBlockModule_DirectSegmentPushPop(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x200)
	copy(mem[startPC:], []byte{0x06, 0x16, 0x1F, 0x07, 0xEB, 0x00})
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
		segsAddr   = uint32(0x700)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 2048)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) ||
		!memExport.Write(segsAddr, make([]byte, 12)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	writePtr(x86CtxOffSegRegsPtr, segsAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x500)
	if !memExport.Write(regsAddr+4*4, word) {
		t.Fatal("seed ESP")
	}
	if !memExport.Write(segsAddr+0, []byte{0x11, 0x11}) || !memExport.Write(segsAddr+4, []byte{0x22, 0x22}) || !memExport.Write(segsAddr+6, []byte{0x33, 0x33}) {
		t.Fatal("seed seg regs")
	}
	binary.LittleEndian.PutUint32(word, 0x700)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU16 := func(addr uint32) uint16 {
		b, ok := memExport.Read(addr, 2)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint16(b)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU16(segsAddr + 0); got != 0x1111 {
		t.Fatalf("ES=%#x want %#x", got, uint16(0x1111))
	}
	if got := readU16(segsAddr + 6); got != 0x2222 {
		t.Fatalf("DS=%#x want %#x", got, uint16(0x2222))
	}
	if got := readU32(regsAddr + 4*4); got != 0x500 {
		t.Fatalf("ESP=%#x want %#x", got, uint32(0x500))
	}
}

func TestX86WasmCompileBlockModule_DirectPushfPopfAndPopMemory(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{
		0x9C,       // PUSHFD
		0x58,       // POP EAX
		0x9D,       // POPFD
		0x8F, 0x03, // POP [EBX]
		0x66, 0x8F, 0x43, 0x04, // POP word [EBX+4]
		0xEB, 0x00,
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 2048)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x20)
	if !memExport.Write(regsAddr+3*4, word) {
		t.Fatal("seed EBX")
	}
	binary.LittleEndian.PutUint32(word, 0x500)
	if !memExport.Write(regsAddr+4*4, word) {
		t.Fatal("seed ESP")
	}
	binary.LittleEndian.PutUint32(word, 0xA5A50123)
	if !memExport.Write(flagsAddr, word) {
		t.Fatal("seed flags")
	}
	if !memExport.Write(guestBase+0x500, []byte{0xEF, 0xBE, 0xAD, 0xDE, 0x44, 0x33, 0x22, 0x11, 0x34, 0x12}) {
		t.Fatal("seed stack source")
	}
	binary.LittleEndian.PutUint32(word, 0x700)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 0*4); got != 0xA5A50123 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0xA5A50123))
	}
	if got := readU32(flagsAddr); got != 0xDEADBEEF {
		t.Fatalf("Flags=%#x want %#x", got, uint32(0xDEADBEEF))
	}
	if got := readU32(regsAddr + 4*4); got != 0x50A {
		t.Fatalf("ESP=%#x want %#x", got, uint32(0x50A))
	}
	if got, ok := memExport.Read(guestBase+0x20, 6); !ok {
		t.Fatal("read pop memory")
	} else if want := []byte{0x44, 0x33, 0x22, 0x11, 0x34, 0x12}; string(got) != string(want) {
		t.Fatalf("pop memory=% X want % X", got, want)
	}
}

func TestX86WasmCompileBlockModule_DirectPushaPopa(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{0x60, 0x61, 0x66, 0x60, 0x66, 0x61, 0xEB, 0x00})
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x900)
		codeBMAddr = uint32(0x980)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(guestBase, make([]byte, 2048)) || !memExport.Write(ioBMAddr, make([]byte, 16)) ||
		!memExport.Write(codeBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	word := make([]byte, 4)
	for idx, val := range []uint32{0x11223344, 0x55667788, 0x99AABBCC, 0xDDEEFF00, 0x500, 0x13572468, 0x24681357, 0xCAFEBEEF} {
		binary.LittleEndian.PutUint32(word, val)
		if !memExport.Write(regsAddr+uint32(idx*4), word) {
			t.Fatalf("seed reg %d", idx)
		}
	}
	binary.LittleEndian.PutUint32(word, 0x800)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
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
	for idx, want := range []uint32{0x11223344, 0x55667788, 0x99AABBCC, 0xDDEEFF00, 0x500, 0x13572468, 0x24681357, 0xCAFEBEEF} {
		if got := readReg(idx); got != want {
			t.Fatalf("reg %d=%#x want %#x", idx, got, want)
		}
	}
	if got, ok := memExport.Read(guestBase+0x4E0, 0x20); !ok {
		t.Fatal("read stack bytes")
	} else if want := []byte{
		0xEF, 0xBE, 0xFE, 0xCA, 0x57, 0x13, 0x68, 0x24,
		0x68, 0x24, 0x57, 0x13, 0x00, 0x05, 0x00, 0x00,
		0xEF, 0xBE, 0x57, 0x13, 0x68, 0x24, 0x00, 0x05,
		0x00, 0xFF, 0xCC, 0xBB, 0x88, 0x77, 0x44, 0x33,
	}; string(got) != string(want) {
		t.Fatalf("stack bytes=% X want % X", got, want)
	}
}

func TestX86WasmCompileBlockModule_DirectXLAT(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x200)
	copy(mem[startPC:], []byte{0xD7, 0xEB, 0x00})
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0x200)
		guestBase = uint32(0x300)
		ioBMAddr  = uint32(0x600)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(guestBase, make([]byte, 1024)) || !memExport.Write(ioBMAddr, make([]byte, 16)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0xDEAD00A5)
	if !memExport.Write(regsAddr+0*4, word) {
		t.Fatal("seed EAX")
	}
	binary.LittleEndian.PutUint32(word, 0x400)
	if !memExport.Write(regsAddr+3*4, word) {
		t.Fatal("seed EBX")
	}
	binary.LittleEndian.PutUint32(word, 0x800)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if !memExport.Write(guestBase+0x4A5, []byte{0x7E}) {
		t.Fatal("seed XLAT table")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	if got, ok := memExport.Read(regsAddr, 4); !ok {
		t.Fatal("read EAX")
	} else if have := binary.LittleEndian.Uint32(got); have != 0xDEAD007E {
		t.Fatalf("EAX=%#x want %#x", have, uint32(0xDEAD007E))
	}
}

func TestX86WasmCompileBlockModule_DirectLeaveAndEnterLevelZero(t *testing.T) {
	t.Run("leave", func(t *testing.T) {
		const startPC = uint32(0x100)
		mem := make([]byte, 0x240)
		copy(mem[startPC:], []byte{0xC9, 0x66, 0xC9, 0xEB, 0x00})
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
			ctxAddr   = uint32(0x80)
			regsAddr  = uint32(0x200)
			guestBase = uint32(0x300)
			ioBMAddr  = uint32(0x600)
		)
		if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
			!memExport.Write(guestBase, make([]byte, 1024)) || !memExport.Write(ioBMAddr, make([]byte, 16)) {
			t.Fatal("seed memory")
		}
		writePtr := func(off, addr uint32) {
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(addr))
			if !memExport.Write(ctxAddr+off, buf) {
				t.Fatalf("seed ptr off %d", off)
			}
		}
		writePtr(x86CtxOffJITRegsPtr, regsAddr)
		writePtr(x86CtxOffMemPtr, guestBase)
		writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
		word := make([]byte, 4)
		binary.LittleEndian.PutUint32(word, 0xDEAD0400)
		if !memExport.Write(regsAddr+4*4, word) {
			t.Fatal("seed ESP")
		}
		binary.LittleEndian.PutUint32(word, 0x500)
		if !memExport.Write(regsAddr+5*4, word) {
			t.Fatal("seed EBP")
		}
		binary.LittleEndian.PutUint32(word, 0x800)
		if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
			t.Fatal("seed MemSize")
		}
		if !memExport.Write(guestBase+0x500, []byte{0x00, 0x06, 0x00, 0x00}) || !memExport.Write(guestBase+0x600, []byte{0x34, 0x12}) {
			t.Fatal("seed leave stack")
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
		if got := readReg(4); got != 0x602 {
			t.Fatalf("ESP=%#x want %#x", got, uint32(0x602))
		}
		if got := readReg(5); got != 0x1234 {
			t.Fatalf("EBP=%#x want %#x", got, uint32(0x1234))
		}
	})

	t.Run("enter_level_zero", func(t *testing.T) {
		const startPC = uint32(0x100)
		mem := make([]byte, 0x240)
		copy(mem[startPC:], []byte{0xC8, 0x08, 0x00, 0x00, 0xC9, 0xEB, 0x00})
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
			ctxAddr    = uint32(0x80)
			regsAddr   = uint32(0x200)
			guestBase  = uint32(0x300)
			ioBMAddr   = uint32(0x600)
			codeBMAddr = uint32(0x680)
		)
		if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
			!memExport.Write(guestBase, make([]byte, 1024)) || !memExport.Write(ioBMAddr, make([]byte, 16)) ||
			!memExport.Write(codeBMAddr, make([]byte, 16)) {
			t.Fatal("seed memory")
		}
		writePtr := func(off, addr uint32) {
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(addr))
			if !memExport.Write(ctxAddr+off, buf) {
				t.Fatalf("seed ptr off %d", off)
			}
		}
		writePtr(x86CtxOffJITRegsPtr, regsAddr)
		writePtr(x86CtxOffMemPtr, guestBase)
		writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
		writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
		word := make([]byte, 4)
		binary.LittleEndian.PutUint32(word, 0x500)
		if !memExport.Write(regsAddr+4*4, word) {
			t.Fatal("seed ESP")
		}
		binary.LittleEndian.PutUint32(word, 0x12345678)
		if !memExport.Write(regsAddr+5*4, word) {
			t.Fatal("seed EBP")
		}
		binary.LittleEndian.PutUint32(word, 0x800)
		if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
			t.Fatal("seed MemSize")
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
		if got := readReg(4); got != 0x500 {
			t.Fatalf("ESP=%#x want %#x", got, uint32(0x500))
		}
		if got := readReg(5); got != 0x12345678 {
			t.Fatalf("EBP=%#x want %#x", got, uint32(0x12345678))
		}
		if got, ok := memExport.Read(guestBase+0x4F4, 12); !ok {
			t.Fatal("read frame bytes")
		} else if want := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x78, 0x56, 0x34, 0x12}; string(got) != string(want) {
			t.Fatalf("frame bytes=% X want % X", got, want)
		}
	})
}

func TestX86WasmCompileBlockModule_DirectSignExtendInstructions(t *testing.T) {
	const startPC = uint32(0x100)
	run := func(t *testing.T, code []byte, eax, edx, wantEAX, wantEDX uint32) {
		t.Helper()
		mem := make([]byte, 0x200)
		copy(mem[startPC:], code)
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
		if !memExport.Write(ctxAddr, make([]byte, 128)) || !memExport.Write(regsAddr, make([]byte, 32)) {
			t.Fatal("seed state")
		}
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
		if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
			t.Fatal("seed JITRegsPtr")
		}
		word := make([]byte, 4)
		binary.LittleEndian.PutUint32(word, eax)
		if !memExport.Write(regsAddr+0*4, word) {
			t.Fatal("seed EAX")
		}
		binary.LittleEndian.PutUint32(word, edx)
		if !memExport.Write(regsAddr+2*4, word) {
			t.Fatal("seed EDX")
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
		if got := readReg(0); got != wantEAX {
			t.Fatalf("EAX=%#x want %#x", got, wantEAX)
		}
		if got := readReg(2); got != wantEDX {
			t.Fatalf("EDX=%#x want %#x", got, wantEDX)
		}
	}
	run(t, []byte{0x66, 0x98, 0x98, 0xEB, 0x00}, 0xBEEF8081, 0, 0xFFFFFF81, 0)
	run(t, []byte{0x66, 0x99, 0xEB, 0x00}, 0xDEAD8001, 0xBEEF1234, 0xDEAD8001, 0xBEEFFFFF)
	run(t, []byte{0x99, 0xEB, 0x00}, 0x80000001, 0x12345678, 0x80000001, 0xFFFFFFFF)
}

func TestX86WasmCompileBlockModule_DirectSALC(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x200)
	copy(mem[startPC:], []byte{
		0xD6, // SALC
		0xEB, 0x00,
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	run := func(name string, flags, wantEAX, wantFlags uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 128)) || !memExport.Write(regsAddr, make([]byte, 32)) || !memExport.Write(flagsAddr, make([]byte, 4)) {
				t.Fatal("seed state")
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, 0xAABBCC12)
			if !memExport.Write(regsAddr+0*4, word) {
				t.Fatal("seed EAX")
			}
			binary.LittleEndian.PutUint32(word, flags)
			if !memExport.Write(flagsAddr, word) {
				t.Fatal("seed flags")
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			readU32 := func(addr uint32) uint32 {
				b, ok := memExport.Read(addr, 4)
				if !ok {
					t.Fatalf("read %#x", addr)
				}
				return binary.LittleEndian.Uint32(b)
			}
			if got := readU32(regsAddr + 0*4); got != wantEAX {
				t.Fatalf("EAX=%#x want %#x", got, wantEAX)
			}
			if got := readU32(flagsAddr); got != wantFlags {
				t.Fatalf("Flags=%#x want %#x", got, wantFlags)
			}
		})
	}
	run("cf_set", x86FlagCF|x86FlagOF|x86FlagIF|0x2, 0xAABBCCFF, x86FlagCF|x86FlagOF|x86FlagIF|0x2)
	run("cf_clear", x86FlagOF|x86FlagIF|0x2, 0xAABBCC00, x86FlagOF|x86FlagIF|0x2)
}

func TestX86WasmCompileBlockModule_DirectCLI_STI(t *testing.T) {
	const startPC = uint32(0x100)
	run := func(t *testing.T, code []byte, initFlags, wantFlags uint32) {
		t.Helper()
		mem := make([]byte, 0x200)
		copy(mem[startPC:], code)
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
			ctxAddr   = uint32(0x80)
			regsAddr  = uint32(0xC0)
			flagsAddr = uint32(0x100)
		)
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
		flagsPtr := make([]byte, 8)
		binary.LittleEndian.PutUint64(flagsPtr, uint64(flagsAddr))
		if !memExport.Write(ctxAddr, make([]byte, 128)) || !memExport.Write(regsAddr, make([]byte, 32)) || !memExport.Write(flagsAddr, make([]byte, 4)) {
			t.Fatal("seed state")
		}
		if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
			t.Fatal("seed JITRegsPtr")
		}
		if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, flagsPtr) {
			t.Fatal("seed FlagsPtr")
		}
		word := make([]byte, 4)
		binary.LittleEndian.PutUint32(word, initFlags)
		if !memExport.Write(flagsAddr, word) {
			t.Fatal("seed flags")
		}
		if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
			t.Fatalf("call block: %v", err)
		}
		got, ok := memExport.Read(flagsAddr, 4)
		if !ok {
			t.Fatal("read flags")
		}
		if have := binary.LittleEndian.Uint32(got); have != wantFlags {
			t.Fatalf("Flags=%#x want %#x", have, wantFlags)
		}
	}
	t.Run("cli", func(t *testing.T) {
		run(t, []byte{0xFA, 0xEB, 0x00}, x86FlagCF|x86FlagOF|x86FlagIF|0x2, x86FlagCF|x86FlagOF|0x2)
	})
	t.Run("sti", func(t *testing.T) {
		run(t, []byte{0xFB, 0xEB, 0x00}, x86FlagCF|x86FlagOF|0x2, x86FlagCF|x86FlagOF|x86FlagIF|0x2)
	})
}

func TestX86WasmCompileBlockModule_DirectSegmentMOV(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0x8C, 0xD8, // MOV AX,DS
		0x8E, 0xE1, // MOV FS,CX
		0x8C, 0x5B, 0x00, // MOV [EBX],DS
		0x8E, 0x63, 0x08, // MOV FS,[EBX+8]
		0x8C, 0x4B, 0x04, // MOV [EBX+4],CS
		0xEB, 0x00,
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
		segsAddr   = uint32(0x700)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 1024)) ||
		!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) ||
		!memExport.Write(segsAddr, make([]byte, 12)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
	writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
	writePtr(x86CtxOffSegRegsPtr, segsAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x40)
	if !memExport.Write(regsAddr+3*4, word) {
		t.Fatal("seed EBX")
	}
	binary.LittleEndian.PutUint32(word, 0xCAFE2468)
	if !memExport.Write(regsAddr+1*4, word) {
		t.Fatal("seed ECX")
	}
	binary.LittleEndian.PutUint32(word, 0xBEEF0000)
	if !memExport.Write(regsAddr+0*4, word) {
		t.Fatal("seed EAX")
	}
	if !memExport.Write(segsAddr+2, []byte{0x57, 0x13}) || !memExport.Write(segsAddr+4, []byte{0x00, 0x00}) ||
		!memExport.Write(segsAddr+8, []byte{0x00, 0x00}) {
		t.Fatal("seed segment regs")
	}
	if !memExport.Write(guestBase+0x48, []byte{0xEF, 0xBE}) {
		t.Fatal("seed memory segment source")
	}
	if !memExport.Write(segsAddr+0, []byte{0xAA, 0xAA}) || !memExport.Write(segsAddr+6, []byte{0x57, 0x13}) {
		t.Fatal("seed ES/DS")
	}
	if !memExport.Write(segsAddr+8, []byte{0x00, 0x00}) {
		t.Fatal("seed FS")
	}
	binary.LittleEndian.PutUint32(word, 0x400)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	if !memExport.Write(segsAddr+0, []byte{0x11, 0x11}) || !memExport.Write(segsAddr+2, []byte{0x68, 0x24}) || !memExport.Write(segsAddr+6, []byte{0x57, 0x13}) {
		t.Fatal("seed ES/CS/DS")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU16 := func(addr uint32) uint16 {
		b, ok := memExport.Read(addr, 2)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint16(b)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 0*4); got != 0xBEEF1357 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0xBEEF1357))
	}
	if got := readU16(segsAddr + 8); got != 0xBEEF {
		t.Fatalf("FS=%#x want %#x", got, uint16(0xBEEF))
	}
	if got, ok := memExport.Read(guestBase+0x40, 6); !ok {
		t.Fatal("read segment mov memory")
	} else if want := []byte{0x57, 0x13, 0x00, 0x00, 0x68, 0x24}; string(got) != string(want) {
		t.Fatalf("segment memory=% X want % X", got, want)
	}
}

func TestX86WasmCompileBlockModule_DirectLESLDS(t *testing.T) {
	const startPC = uint32(0x100)
	run := func(t *testing.T, code []byte, init func(memExport api.Memory), dataAddr uint32, data []byte, wantEAX, wantECX, wantEDX uint32, wantES, wantDS uint16) {
		t.Helper()
		mem := make([]byte, 0x300)
		copy(mem[startPC:], code)
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
			ctxAddr   = uint32(0x80)
			regsAddr  = uint32(0x200)
			guestBase = uint32(0x300)
			ioBMAddr  = uint32(0x700)
			segsAddr  = uint32(0x780)
		)
		if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
			!memExport.Write(guestBase, make([]byte, 1024)) || !memExport.Write(ioBMAddr, make([]byte, 16)) ||
			!memExport.Write(segsAddr, make([]byte, 12)) {
			t.Fatal("seed state")
		}
		writePtr := func(off, addr uint32) {
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(addr))
			if !memExport.Write(ctxAddr+off, buf) {
				t.Fatalf("seed ptr off %d", off)
			}
		}
		writePtr(x86CtxOffJITRegsPtr, regsAddr)
		writePtr(x86CtxOffMemPtr, guestBase)
		writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
		writePtr(x86CtxOffSegRegsPtr, segsAddr)
		if init != nil {
			init(memExport)
		}
		word := make([]byte, 4)
		binary.LittleEndian.PutUint32(word, 0x400)
		if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
			t.Fatal("seed MemSize")
		}
		if !memExport.Write(guestBase+dataAddr, data) {
			t.Fatal("seed far pointer")
		}
		if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
			t.Fatalf("call block: %v", err)
		}
		readU16 := func(addr uint32) uint16 {
			b, ok := memExport.Read(addr, 2)
			if !ok {
				t.Fatalf("read %#x", addr)
			}
			return binary.LittleEndian.Uint16(b)
		}
		readU32 := func(addr uint32) uint32 {
			b, ok := memExport.Read(addr, 4)
			if !ok {
				t.Fatalf("read %#x", addr)
			}
			return binary.LittleEndian.Uint32(b)
		}
		if got := readU32(regsAddr + 0*4); got != wantEAX {
			t.Fatalf("EAX=%#x want %#x", got, wantEAX)
		}
		if got := readU32(regsAddr + 1*4); got != wantECX {
			t.Fatalf("ECX=%#x want %#x", got, wantECX)
		}
		if got := readU32(regsAddr + 2*4); got != wantEDX {
			t.Fatalf("EDX=%#x want %#x", got, wantEDX)
		}
		if got := readU16(segsAddr + 0); got != wantES {
			t.Fatalf("ES=%#x want %#x", got, wantES)
		}
		if got := readU16(segsAddr + 6); got != wantDS {
			t.Fatalf("DS=%#x want %#x", got, wantDS)
		}
	}
	t.Run("les32_memory", func(t *testing.T) {
		run(t, []byte{0xC4, 0x03, 0xEB, 0x00}, func(memExport api.Memory) {
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, 0x20)
			if !memExport.Write(0x200+3*4, word) {
				t.Fatal("seed EBX")
			}
		}, 0x20, []byte{0x78, 0x56, 0x34, 0x12, 0xCD, 0xAB}, 0x12345678, 0, 0, 0xABCD, 0)
	})
	t.Run("lds16_memory", func(t *testing.T) {
		run(t, []byte{0x66, 0xC5, 0x13, 0xEB, 0x00}, func(memExport api.Memory) {
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, 0x24)
			if !memExport.Write(0x200+3*4, word) {
				t.Fatal("seed EBX")
			}
			binary.LittleEndian.PutUint32(word, 0xDEAD0000)
			if !memExport.Write(0x200+2*4, word) {
				t.Fatal("seed EDX")
			}
		}, 0x24, []byte{0x34, 0x12, 0xDC, 0xBA}, 0, 0, 0xDEAD1234, 0, 0xBADC)
	})
	t.Run("les_register_address", func(t *testing.T) {
		run(t, []byte{0xC4, 0xCB, 0xEB, 0x00}, func(memExport api.Memory) {
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, 0x28)
			if !memExport.Write(0x200+3*4, word) {
				t.Fatal("seed EBX")
			}
		}, 0x28, []byte{0x11, 0x22, 0x33, 0x44, 0x66, 0x55}, 0, 0x44332211, 0, 0x5566, 0)
	})
}

func TestX86WasmCompileBlockModule_DirectLEASIBAndAbsolute(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x200)
	copy(mem[startPC:], []byte{
		0x8D, 0x44, 0x8D, 0xF0, // LEA EAX,[EBP+ECX*4-16]
		0x8D, 0x1D, 0x78, 0x56, 0x34, 0x12, // LEA EBX,[12345678]
		0xEB, 0x00,
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
	if !memExport.Write(ctxAddr, make([]byte, 128)) || !memExport.Write(regsAddr, make([]byte, 32)) {
		t.Fatal("seed state")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x1000)
	if !memExport.Write(regsAddr+5*4, word) { // EBP
		t.Fatal("seed EBP")
	}
	binary.LittleEndian.PutUint32(word, 3)
	if !memExport.Write(regsAddr+1*4, word) { // ECX
		t.Fatal("seed ECX")
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
	if got := readReg(0); got != 0xFFC {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0xFFC))
	}
	if got := readReg(3); got != 0x12345678 {
		t.Fatalf("EBX=%#x want %#x", got, uint32(0x12345678))
	}
}

func TestX86WasmCompileBlockModule_DirectBitTest(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{
		0xB8, 0x08, 0x00, 0x08, 0x00, // MOV EAX,0x00080008
		0xB9, 0x13, 0x00, 0x00, 0x00, // MOV ECX,19
		0x0F, 0xA3, 0xC8, // BT EAX,ECX -> CF=1
		0x0F, 0x92, 0xC2, // SETC DL -> 1
		0xB9, 0x04, 0x00, 0x00, 0x00, // MOV ECX,4
		0x66, 0x0F, 0xA3, 0xC8, // BT AX,CX -> CF=0
		0x0F, 0x92, 0xC3, // SETC BL -> 0
		0x0F, 0xBA, 0xE0, 0x03, // BT EAX,3 -> CF=1
		0x0F, 0x92, 0xC1, // SETC CL -> 1
		0xEB, 0x00,
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x100)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("seed memory")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
		t.Fatal("seed FlagsPtr")
	}
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, x86FlagZF|x86FlagOF)
	if !memExport.Write(flagsAddr, word) {
		t.Fatal("seed Flags")
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
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readReg(0); got != 0x00080008 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(0x00080008))
	}
	if got := readReg(1); got != 1 {
		t.Fatalf("ECX=%#x want 1", got)
	}
	if got := readReg(2); got != 1 {
		t.Fatalf("EDX=%#x want 1", got)
	}
	if got := readReg(3); got != 0 {
		t.Fatalf("EBX=%#x want 0", got)
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0x125 {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x125))
	}
	if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != (x86FlagCF | x86FlagZF | x86FlagOF) {
		t.Fatalf("Flags=%#x want %#x", got, uint32(x86FlagCF|x86FlagZF|x86FlagOF))
	}
}

func TestX86WasmCompileBlockModule_DirectIMULImmediate(t *testing.T) {
	const startPC = uint32(0x100)
	t.Run("register_and_word", func(t *testing.T) {
		mem := make([]byte, 0x240)
		copy(mem[startPC:], []byte{
			0x6B, 0xC1, 0xFE, // IMUL EAX,ECX,-2
			0x66, 0x6B, 0xD3, 0x02, // IMUL DX,BX,2
			0xEB, 0x00,
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
			ctxAddr   = uint32(0x80)
			regsAddr  = uint32(0xC0)
			flagsAddr = uint32(0x100)
		)
		if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
			!memExport.Write(flagsAddr, make([]byte, 4)) {
			t.Fatal("seed memory")
		}
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
		if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
			t.Fatal("seed JITRegsPtr")
		}
		binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
		if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
			t.Fatal("seed FlagsPtr")
		}
		word := make([]byte, 4)
		binary.LittleEndian.PutUint32(word, 0xB4B60000)
		if !memExport.Write(regsAddr+0*4, word) {
			t.Fatal("seed EAX")
		}
		binary.LittleEndian.PutUint32(word, 7)
		if !memExport.Write(regsAddr+1*4, word) {
			t.Fatal("seed ECX")
		}
		binary.LittleEndian.PutUint32(word, 0x00007000)
		if !memExport.Write(regsAddr+3*4, word) {
			t.Fatal("seed EBX")
		}
		binary.LittleEndian.PutUint32(word, x86FlagPF|x86FlagAF|x86FlagZF)
		if !memExport.Write(flagsAddr, word) {
			t.Fatal("seed flags")
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
		readU32 := func(addr uint32) uint32 {
			b, ok := memExport.Read(addr, 4)
			if !ok {
				t.Fatalf("read %#x", addr)
			}
			return binary.LittleEndian.Uint32(b)
		}
		if got := readReg(0); got != 0xFFFFFFF2 {
			t.Fatalf("EAX=%#x want %#x", got, uint32(0xFFFFFFF2))
		}
		if got := readReg(2); got != 0x0000E000 {
			t.Fatalf("EDX=%#x want %#x", got, uint32(0x0000E000))
		}
		if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != (x86FlagCF | x86FlagOF | x86FlagPF | x86FlagAF | x86FlagZF) {
			t.Fatalf("Flags=%#x want %#x", got, uint32(x86FlagCF|x86FlagOF|x86FlagPF|x86FlagAF|x86FlagZF))
		}
	})

	t.Run("guarded_memory", func(t *testing.T) {
		mem := make([]byte, 0x240)
		copy(mem[startPC:], []byte{
			0x6B, 0x03, 0xFE, // IMUL EAX,dword [EBX],-2
			0xEB, 0x00,
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
			ctxAddr   = uint32(0x80)
			regsAddr  = uint32(0x200)
			flagsAddr = uint32(0x280)
			guestBase = uint32(0x300)
			ioBMAddr  = uint32(0x600)
		)
		writePtr := func(off, addr uint32) {
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(addr))
			if !memExport.Write(ctxAddr+off, buf) {
				t.Fatalf("seed ptr off %d", off)
			}
		}
		readU32 := func(addr uint32) uint32 {
			b, ok := memExport.Read(addr, 4)
			if !ok {
				t.Fatalf("read %#x", addr)
			}
			return binary.LittleEndian.Uint32(b)
		}
		run := func(name string, ebx, memSize uint32, markMMIO bool, wantEAX, wantRetPC, wantRetCount, wantIO, wantFlags uint32) {
			t.Run(name, func(t *testing.T) {
				if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
					!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) ||
					!memExport.Write(ioBMAddr, make([]byte, 16)) {
					t.Fatal("seed memory")
				}
				writePtr(x86CtxOffJITRegsPtr, regsAddr)
				writePtr(x86CtxOffFlagsPtr, flagsAddr)
				writePtr(x86CtxOffMemPtr, guestBase)
				writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
				word := make([]byte, 4)
				binary.LittleEndian.PutUint32(word, ebx)
				if !memExport.Write(regsAddr+3*4, word) {
					t.Fatal("seed EBX")
				}
				binary.LittleEndian.PutUint32(word, 0xB4B60000)
				if !memExport.Write(regsAddr+0*4, word) {
					t.Fatal("seed EAX")
				}
				binary.LittleEndian.PutUint32(word, memSize)
				if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
					t.Fatal("seed MemSize")
				}
				if !memExport.Write(guestBase+ebx, []byte{0x07, 0x00, 0x00, 0x00}) {
					t.Fatal("seed IMUL source")
				}
				binary.LittleEndian.PutUint32(word, x86FlagPF|x86FlagAF|x86FlagZF)
				if !memExport.Write(flagsAddr, word) {
					t.Fatal("seed flags")
				}
				if markMMIO {
					if !memExport.Write(ioBMAddr, []byte{1}) {
						t.Fatal("seed io bitmap")
					}
				}
				if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
					t.Fatalf("call block: %v", err)
				}
				if got := readU32(regsAddr); got != wantEAX {
					t.Fatalf("EAX=%#x want %#x", got, wantEAX)
				}
				if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantRetPC {
					t.Fatalf("RetPC=%#x want %#x", got, wantRetPC)
				}
				if got := readU32(ctxAddr + x86CtxOffRetCount); got != wantRetCount {
					t.Fatalf("RetCount=%d want %d", got, wantRetCount)
				}
				if got := readU32(ctxAddr + x86CtxOffNeedIOFallback); got != wantIO {
					t.Fatalf("NeedIOFallback=%d want %d", got, wantIO)
				}
				if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != wantFlags {
					t.Fatalf("Flags=%#x want %#x", got, wantFlags)
				}
			})
		}
		run("safe", 0x20, 0x200, false, 0xFFFFFFF2, 0x105, 2, 0, x86FlagPF|x86FlagAF|x86FlagZF)
		run("ceiling_bail", 0x80, 0x80, false, 0xB4B60000, 0x100, 0, 1, x86FlagPF|x86FlagAF|x86FlagZF)
		run("mmio_bail", 0x20, 0x200, true, 0xB4B60000, 0x100, 0, 1, x86FlagPF|x86FlagAF|x86FlagZF)
	})
}

func TestX86WasmCompileBlockModule_DirectDoubleShift(t *testing.T) {
	const startPC = uint32(0x1000)
	run := func(t *testing.T, code []byte, setup func(*CPU_X86), checkMem func(*testing.T, *CPU_X86, api.Memory, uint32)) {
		t.Helper()
		interp := runX86InterpreterProgramWithSetup(t, startPC, setup, code...)

		mem := make([]byte, int(startPC)+len(code)+0x10)
		copy(mem[startPC:], code)
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
			ctxAddr    = uint32(0x80)
			regsAddr   = uint32(0x200)
			flagsAddr  = uint32(0x280)
			guestBase  = uint32(0x300)
			ioBMAddr   = uint32(0x900)
			codeBMAddr = uint32(0x980)
		)
		if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
			!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 0x2000)) ||
			!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
			t.Fatal("seed memory")
		}
		writePtr := func(off, addr uint32) {
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(addr))
			if !memExport.Write(ctxAddr+off, buf) {
				t.Fatalf("seed ptr off %d", off)
			}
		}
		writePtr(x86CtxOffJITRegsPtr, regsAddr)
		writePtr(x86CtxOffFlagsPtr, flagsAddr)
		writePtr(x86CtxOffMemPtr, guestBase)
		writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
		writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
		word := make([]byte, 4)
		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		cpu := NewCPU_X86(adapter)
		cpu.memory = adapter.GetMemory()
		copy(cpu.memory[startPC:], code)
		if setup != nil {
			setup(cpu)
		}
		for reg, val := range []uint32{cpu.EAX, cpu.ECX, cpu.EDX, cpu.EBX, cpu.ESP, cpu.EBP, cpu.ESI, cpu.EDI} {
			binary.LittleEndian.PutUint32(word, val)
			if !memExport.Write(regsAddr+uint32(reg*4), word) {
				t.Fatalf("seed reg %d", reg)
			}
		}
		binary.LittleEndian.PutUint32(word, cpu.Flags)
		if !memExport.Write(flagsAddr, word) {
			t.Fatal("seed flags")
		}
		binary.LittleEndian.PutUint32(word, 0x2000)
		if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
			t.Fatal("seed MemSize")
		}
		if !memExport.Write(guestBase, cpu.memory[:0x2000]) {
			t.Fatal("seed guest memory")
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
		readU32 := func(addr uint32) uint32 {
			b, ok := memExport.Read(addr, 4)
			if !ok {
				t.Fatalf("read %#x", addr)
			}
			return binary.LittleEndian.Uint32(b)
		}
		if got, want := readReg(0), interp.EAX; got != want {
			t.Fatalf("EAX=%#x want %#x", got, want)
		}
		if got, want := readReg(1), interp.ECX; got != want {
			t.Fatalf("ECX=%#x want %#x", got, want)
		}
		if got, want := readReg(3), interp.EBX; got != want {
			t.Fatalf("EBX=%#x want %#x", got, want)
		}
		if got, want := readU32(flagsAddr), interp.Flags; got != want {
			t.Fatalf("Flags=%#x want %#x", got, want)
		}
		if got, want := readU32(ctxAddr+x86CtxOffRetPC), startPC+uint32(len(code))-1; got != want {
			t.Fatalf("RetPC=%#x want %#x", got, want)
		}
		if checkMem != nil {
			checkMem(t, interp, memExport, guestBase)
		}
	}

	t.Run("register_immediate_and_cl", func(t *testing.T) {
		code := []byte{
			0x0F, 0xA4, 0xD8, 0x07, // SHLD EAX,EBX,7
			0x0F, 0xAD, 0xC8, // SHRD EAX,ECX,CL
			0x66, 0x0F, 0xA4, 0xC8, 0x10, // SHLD AX,CX,16
			0xEB, 0x00,
			0xF4,
		}
		setup := func(cpu *CPU_X86) {
			cpu.EAX = 0x89ABCDEF
			cpu.EBX = 0x01234567
			cpu.ECX = 0xBEEF0004
			cpu.Flags = x86FlagOF | x86FlagAF
		}
		run(t, code, setup, nil)
	})

	t.Run("memory_immediate_and_word", func(t *testing.T) {
		code := []byte{
			0x0F, 0xA4, 0x03, 0x04, // SHLD dword [EBX],EAX,4
			0x66, 0x0F, 0xAC, 0x03, 0x04, // SHRD word [EBX],AX,4
			0xEB, 0x00,
			0xF4,
		}
		setup := func(cpu *CPU_X86) {
			cpu.EAX = 0xDEAD89AB
			cpu.EBX = 0x500
			cpu.Flags = x86FlagOF | x86FlagAF
			cpu.memory[0x500] = 0xEF
			cpu.memory[0x501] = 0xBE
			cpu.memory[0x502] = 0xAD
			cpu.memory[0x503] = 0xDE
		}
		run(t, code, setup, func(t *testing.T, interp *CPU_X86, memExport api.Memory, guestBase uint32) {
			got, ok := memExport.Read(guestBase+0x500, 4)
			if !ok {
				t.Fatal("read guest memory")
			}
			if want := interp.memory[0x500:0x504]; string(got) != string(want) {
				t.Fatalf("memory=% X want % X", got, want)
			}
		})
	})
}

func TestX86WasmCompileBlockModule_DirectShiftFamilies(t *testing.T) {
	const startPC = uint32(0x1000)
	run := func(t *testing.T, code []byte, setup func(*CPU_X86), check func(*testing.T, *CPU_X86, api.Memory, uint32)) {
		t.Helper()
		interp := runX86InterpreterProgramWithSetup(t, startPC, setup, code...)
		mem := make([]byte, int(startPC)+len(code)+0x10)
		copy(mem[startPC:], code)
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
			ctxAddr    = uint32(0x80)
			regsAddr   = uint32(0x200)
			flagsAddr  = uint32(0x280)
			guestBase  = uint32(0x300)
			ioBMAddr   = uint32(0x900)
			codeBMAddr = uint32(0x980)
		)
		if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
			!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 0x2000)) ||
			!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
			t.Fatal("seed memory")
		}
		writePtr := func(off, addr uint32) {
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(addr))
			if !memExport.Write(ctxAddr+off, buf) {
				t.Fatalf("seed ptr off %d", off)
			}
		}
		writePtr(x86CtxOffJITRegsPtr, regsAddr)
		writePtr(x86CtxOffFlagsPtr, flagsAddr)
		writePtr(x86CtxOffMemPtr, guestBase)
		writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
		writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)

		bus := NewMachineBus()
		adapter := NewX86BusAdapter(bus)
		cpu := NewCPU_X86(adapter)
		cpu.memory = adapter.GetMemory()
		copy(cpu.memory[startPC:], code)
		if setup != nil {
			setup(cpu)
		}
		word := make([]byte, 4)
		for reg, val := range []uint32{cpu.EAX, cpu.ECX, cpu.EDX, cpu.EBX, cpu.ESP, cpu.EBP, cpu.ESI, cpu.EDI} {
			binary.LittleEndian.PutUint32(word, val)
			if !memExport.Write(regsAddr+uint32(reg*4), word) {
				t.Fatalf("seed reg %d", reg)
			}
		}
		binary.LittleEndian.PutUint32(word, cpu.Flags)
		if !memExport.Write(flagsAddr, word) {
			t.Fatal("seed flags")
		}
		binary.LittleEndian.PutUint32(word, 0x2000)
		if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
			t.Fatal("seed MemSize")
		}
		if !memExport.Write(guestBase, cpu.memory[:0x2000]) {
			t.Fatal("seed guest memory")
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
		readU32 := func(addr uint32) uint32 {
			b, ok := memExport.Read(addr, 4)
			if !ok {
				t.Fatalf("read %#x", addr)
			}
			return binary.LittleEndian.Uint32(b)
		}
		for reg, want := range []uint32{interp.EAX, interp.ECX, interp.EDX, interp.EBX} {
			if got := readReg(reg); got != want {
				t.Fatalf("reg %d=%#x want %#x", reg, got, want)
			}
		}
		if got, want := readU32(flagsAddr), interp.Flags; got != want {
			t.Fatalf("Flags=%#x want %#x", got, want)
		}
		if got, want := readU32(ctxAddr+x86CtxOffRetPC), startPC+uint32(len(code))-1; got != want {
			t.Fatalf("RetPC=%#x want %#x", got, want)
		}
		if check != nil {
			check(t, interp, memExport, guestBase)
		}
	}

	t.Run("register_count_one", func(t *testing.T) {
		code := []byte{0xD0, 0xE0, 0x66, 0xD1, 0xE8, 0xD1, 0xF8, 0xEB, 0x00, 0xF4}
		setup := func(cpu *CPU_X86) {
			cpu.EAX = 0x8123A55A
			cpu.Flags = x86FlagAF | x86FlagPF | x86FlagZF | x86FlagOF
		}
		run(t, code, setup, nil)
	})

	t.Run("memory_count_one", func(t *testing.T) {
		code := []byte{0xD0, 0x23, 0x66, 0xD1, 0x2B, 0xD1, 0x3B, 0xEB, 0x00, 0xF4}
		setup := func(cpu *CPU_X86) {
			cpu.EBX = 0x500
			cpu.Flags = x86FlagAF | x86FlagPF | x86FlagZF
			cpu.memory[0x500] = 0x80
			cpu.memory[0x501] = 0x34
			cpu.memory[0x502] = 0x12
			cpu.memory[0x503] = 0xF0
		}
		run(t, code, setup, func(t *testing.T, interp *CPU_X86, memExport api.Memory, guestBase uint32) {
			got, ok := memExport.Read(guestBase+0x500, 4)
			if !ok {
				t.Fatal("read guest memory")
			}
			if want := interp.memory[0x500:0x504]; string(got) != string(want) {
				t.Fatalf("memory=% X want % X", got, want)
			}
		})
	})

	t.Run("byte_shift_cl", func(t *testing.T) {
		code := []byte{0xD2, 0xE0, 0xD2, 0xE8, 0xD2, 0xF8, 0xEB, 0x00, 0xF4}
		setup := func(cpu *CPU_X86) {
			cpu.EAX = 0x1234A55A
			cpu.ECX = 0xCAFE0004
			cpu.Flags = x86FlagOF | x86FlagAF
		}
		run(t, code, setup, nil)
	})

	t.Run("word_shift_cl", func(t *testing.T) {
		code := []byte{0x66, 0xD3, 0xE0, 0x66, 0xD3, 0xE8, 0x66, 0xD3, 0xF8, 0xEB, 0x00, 0xF4}
		setup := func(cpu *CPU_X86) {
			cpu.EAX = 0x89ABCDEF
			cpu.ECX = 0x12340004
			cpu.Flags = x86FlagOF | x86FlagAF
		}
		run(t, code, setup, nil)
	})

	t.Run("carry_rotate_cl", func(t *testing.T) {
		code := []byte{
			0xD2, 0xD4, // RCL AH,CL
			0xD2, 0xDC, // RCR AH,CL
			0x66, 0xD3, 0xD0, // RCL AX,CL
			0x66, 0xD3, 0xD8, // RCR AX,CL
			0xD3, 0xD0, // RCL EAX,CL
			0xD3, 0xD8, // RCR EAX,CL
			0xEB, 0x00,
			0xF4,
		}
		setup := func(cpu *CPU_X86) {
			cpu.EAX = 0x89ABCDEF
			cpu.ECX = 0x12340011
			cpu.Flags = x86FlagCF | x86FlagOF | x86FlagAF
		}
		run(t, code, setup, nil)
	})

	t.Run("narrow_rotate_full_width_counts", func(t *testing.T) {
		cases := []struct {
			name  string
			code  []byte
			eax   uint32
			ecx   uint32
			flags uint32
		}{
			{
				name:  "rol_ah_cl_8",
				code:  []byte{0xD2, 0xC4, 0xEB, 0x00, 0xF4},
				eax:   0x0000815A,
				ecx:   8,
				flags: x86FlagCF | x86FlagOF,
			},
			{
				name:  "ror_ah_cl_8",
				code:  []byte{0xD2, 0xCC, 0xEB, 0x00, 0xF4},
				eax:   0x0000815A,
				ecx:   8,
				flags: x86FlagCF | x86FlagOF,
			},
			{
				name:  "rol_ax_cl_16",
				code:  []byte{0x66, 0xD3, 0xC0, 0xEB, 0x00, 0xF4},
				eax:   0x1234815A,
				ecx:   16,
				flags: x86FlagCF | x86FlagOF,
			},
			{
				name:  "ror_ax_cl_16",
				code:  []byte{0x66, 0xD3, 0xC8, 0xEB, 0x00, 0xF4},
				eax:   0x1234815A,
				ecx:   16,
				flags: x86FlagCF | x86FlagOF,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				run(t, tc.code, func(cpu *CPU_X86) {
					cpu.EAX = tc.eax
					cpu.ECX = tc.ecx
					cpu.Flags = tc.flags
				}, nil)
			})
		}
	})
}

func TestX86WasmCompileBlockModule_DirectMemoryTESTImmediateAndGuardBails(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0xF6, 0x03, 0x0F, // TEST byte [EBX],0x0F
		0x74, 0x02, // JZ +2
		0x90,
		0xF4,
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0x200)
		flagsAddr = uint32(0x280)
		guestBase = uint32(0x300)
		ioBMAddr  = uint32(0x600)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	run := func(name string, ebx, memSize uint32, markMMIO bool, memByte byte, wantRetPC, wantRetCount, wantIO, wantFlags uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
				!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) ||
				!memExport.Write(ioBMAddr, make([]byte, 16)) {
				t.Fatal("seed memory")
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			writePtr(x86CtxOffMemPtr, guestBase)
			writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, ebx)
			if !memExport.Write(regsAddr+3*4, word) {
				t.Fatal("seed EBX")
			}
			binary.LittleEndian.PutUint32(word, memSize)
			if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
				t.Fatal("seed MemSize")
			}
			if !memExport.Write(guestBase+ebx, []byte{memByte}) {
				t.Fatal("seed test byte")
			}
			if markMMIO {
				if !memExport.Write(ioBMAddr, []byte{1}) {
					t.Fatal("seed io bitmap")
				}
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantRetPC {
				t.Fatalf("RetPC=%#x want %#x", got, wantRetPC)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != wantRetCount {
				t.Fatalf("RetCount=%d want %d", got, wantRetCount)
			}
			if got := readU32(ctxAddr + x86CtxOffNeedIOFallback); got != wantIO {
				t.Fatalf("NeedIOFallback=%d want %d", got, wantIO)
			}
			if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != wantFlags {
				t.Fatalf("Flags=%#x want %#x", got, wantFlags)
			}
		})
	}
	run("safe_zero_branch", 0x20, 0x200, false, 0xF0, 0x107, 2, 0, x86FlagZF|x86FlagPF)
	run("ceiling_bail", 0x80, 0x80, false, 0x00, 0x100, 0, 1, 0)
	run("mmio_bail", 0x20, 0x200, true, 0x00, 0x100, 0, 1, 0)
}

func TestX86WasmCompileBlockModule_DirectCALLWritesReturnAndGuardBails(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0xE8, 0x01, 0x00, 0x00, 0x00, // CALL +1
		0xF4,
		0x90,
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
		ctxAddr    = uint32(0x80)
		regsAddr   = uint32(0x200)
		flagsAddr  = uint32(0x280)
		guestBase  = uint32(0x300)
		ioBMAddr   = uint32(0x600)
		codeBMAddr = uint32(0x680)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	run := func(name string, esp, memSize uint32, markMMIO, markCode, wantStore bool, wantESP, wantRetPC, wantRetCount, wantIO, wantInval, wantInvalAddr, wantInvalSize, wantStack uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
				!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) ||
				!memExport.Write(ioBMAddr, make([]byte, 16)) || !memExport.Write(codeBMAddr, make([]byte, 16)) {
				t.Fatal("seed memory")
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			writePtr(x86CtxOffMemPtr, guestBase)
			writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
			writePtr(x86CtxOffCodePageBitmapPtr, codeBMAddr)
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, esp)
			if !memExport.Write(regsAddr+4*4, word) {
				t.Fatal("seed ESP")
			}
			binary.LittleEndian.PutUint32(word, memSize)
			if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
				t.Fatal("seed MemSize")
			}
			if markMMIO {
				if !memExport.Write(ioBMAddr, []byte{1}) {
					t.Fatal("seed io bitmap")
				}
			}
			if markCode {
				if !memExport.Write(codeBMAddr, []byte{1}) {
					t.Fatal("seed code bitmap")
				}
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			if got := readU32(regsAddr + 4*4); got != wantESP {
				t.Fatalf("ESP=%#x want %#x", got, wantESP)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantRetPC {
				t.Fatalf("RetPC=%#x want %#x", got, wantRetPC)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != wantRetCount {
				t.Fatalf("RetCount=%d want %d", got, wantRetCount)
			}
			if got := readU32(ctxAddr + x86CtxOffNeedIOFallback); got != wantIO {
				t.Fatalf("NeedIOFallback=%d want %d", got, wantIO)
			}
			if got := readU32(ctxAddr + x86CtxOffNeedInval); got != wantInval {
				t.Fatalf("NeedInval=%d want %d", got, wantInval)
			}
			if got := readU32(ctxAddr + x86CtxOffInvalAddr); got != wantInvalAddr {
				t.Fatalf("InvalAddr=%#x want %#x", got, wantInvalAddr)
			}
			if got := readU32(ctxAddr + x86CtxOffInvalSize); got != wantInvalSize {
				t.Fatalf("InvalSize=%d want %d", got, wantInvalSize)
			}
			if wantStore {
				if got := readU32(guestBase + wantESP); got != wantStack {
					t.Fatalf("stack=%#x want %#x", got, wantStack)
				}
			}
		})
	}
	run("safe", 0x40, 0x200, false, false, true, 0x3c, 0x106, 1, 0, 0, 0, 0, 0x105)
	run("ceiling_bail", 0x102, 0x200, false, false, false, 0x102, 0x100, 0, 1, 0, 0, 0, 0)
	run("mmio_bail", 0x40, 0x200, true, false, false, 0x40, 0x100, 0, 1, 0, 0, 0, 0)
	run("code_inval", 0x40, 0x200, false, true, true, 0x3c, 0x106, 1, 0, 1, 0x3c, 4, 0x105)
}

func TestX86WasmCompileBlockModule_DirectRETReadsReturnPCAndGuardBails(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{0xC3})
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0x200)
		flagsAddr = uint32(0x280)
		guestBase = uint32(0x300)
		ioBMAddr  = uint32(0x600)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	run := func(name string, esp, memSize, returnPC uint32, markMMIO bool, wantESP, wantRetPC, wantRetCount, wantIO uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
				!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) ||
				!memExport.Write(ioBMAddr, make([]byte, 16)) {
				t.Fatal("seed memory")
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			writePtr(x86CtxOffMemPtr, guestBase)
			writePtr(x86CtxOffIOBitmapPtr, ioBMAddr)
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, esp)
			if !memExport.Write(regsAddr+4*4, word) {
				t.Fatal("seed ESP")
			}
			binary.LittleEndian.PutUint32(word, memSize)
			if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
				t.Fatal("seed MemSize")
			}
			binary.LittleEndian.PutUint32(word, returnPC)
			if esp <= 0x1FC && !memExport.Write(guestBase+esp, word) {
				t.Fatal("seed return PC")
			}
			if markMMIO {
				if !memExport.Write(ioBMAddr, []byte{1}) {
					t.Fatal("seed io bitmap")
				}
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			if got := readU32(regsAddr + 4*4); got != wantESP {
				t.Fatalf("ESP=%#x want %#x", got, wantESP)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantRetPC {
				t.Fatalf("RetPC=%#x want %#x", got, wantRetPC)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != wantRetCount {
				t.Fatalf("RetCount=%d want %d", got, wantRetCount)
			}
			if got := readU32(ctxAddr + x86CtxOffNeedIOFallback); got != wantIO {
				t.Fatalf("NeedIOFallback=%d want %d", got, wantIO)
			}
		})
	}
	run("safe", 0x20, 0x200, 0x12345678, false, 0x24, 0x12345678, 1, 0)
	run("ceiling_bail", 0xFF, 0x200, 0, false, 0xFF, 0x100, 0, 1)
	run("mmio_bail", 0x20, 0x200, 0x12345678, true, 0x20, 0x100, 0, 1)
}

func TestX86WasmCompileBlockModule_DirectRETImm16AdjustsESP(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x240)
	copy(mem[startPC:], []byte{0xC2, 0x04, 0x00})
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0x200)
		flagsAddr = uint32(0x280)
		guestBase = uint32(0x300)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) || !memExport.Write(guestBase, make([]byte, 512)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	writePtr(x86CtxOffMemPtr, guestBase)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 0x20)
	if !memExport.Write(regsAddr+4*4, word) {
		t.Fatal("seed ESP")
	}
	binary.LittleEndian.PutUint32(word, 0x200)
	if !memExport.Write(ctxAddr+x86CtxOffMemSize, word) {
		t.Fatal("seed MemSize")
	}
	binary.LittleEndian.PutUint32(word, 0xCAFEBABE)
	if !memExport.Write(guestBase+0x20, word) {
		t.Fatal("seed return PC")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 4*4); got != 0x28 {
		t.Fatalf("ESP=%#x want %#x", got, uint32(0x28))
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0xCAFEBABE {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0xCAFEBABE))
	}
	if got := readU32(ctxAddr + x86CtxOffRetCount); got != 1 {
		t.Fatalf("RetCount=%d want 1", got)
	}
}

func TestX86WasmCompileBlockModule_NearJccUsesGuestFlags(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0x85, 0xC0, // TEST EAX,EAX
		0x0F, 0x84, 0x05, 0x00, 0x00, 0x00, // JZ +5 -> 0x10d
		0x90, // fallthrough landing at 0x108
		0xF4, // HLT
		0x90, 0x90, 0x90,
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0x200)
		flagsAddr = uint32(0x280)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	run := func(name string, eax, wantPC, wantCount, wantFlags uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
				!memExport.Write(flagsAddr, make([]byte, 4)) {
				t.Fatal("seed memory")
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, eax)
			if !memExport.Write(regsAddr+0*4, word) {
				t.Fatal("seed EAX")
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantPC {
				t.Fatalf("RetPC=%#x want %#x", got, wantPC)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != wantCount {
				t.Fatalf("RetCount=%d want %d", got, wantCount)
			}
			if got := readU32(flagsAddr) & x86VisibleFlagsMask; got != wantFlags {
				t.Fatalf("Flags=%#x want %#x", got, wantFlags)
			}
		})
	}
	run("taken", 0, 0x10d, 2, x86FlagZF|x86FlagPF)
	run("not_taken", 1, 0x108, 2, 0)
}

func TestX86WasmCompileBlockModule_NearJccOperandSizeUsesRel16(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{
		0x85, 0xC0, // TEST EAX,EAX
		0x66, 0x0F, 0x85, 0x03, 0x00, // JNZ +3 -> 0x10a
		0x90, // fallthrough
		0xF4, // target
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0x200)
		flagsAddr = uint32(0x280)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("seed memory")
	}
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	writePtr(x86CtxOffJITRegsPtr, regsAddr)
	writePtr(x86CtxOffFlagsPtr, flagsAddr)
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 1)
	if !memExport.Write(regsAddr, word) {
		t.Fatal("seed EAX")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call block: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0x10a {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x10a))
	}
	if got := readU32(ctxAddr + x86CtxOffRetCount); got != 2 {
		t.Fatalf("RetCount=%d want 2", got)
	}
}

func TestX86WasmCompileBlockModule_LoopFormsUpdateCountAndSelectPC(t *testing.T) {
	const startPC = uint32(0x100)
	mem := make([]byte, 0x280)
	copy(mem[startPC:], []byte{0xE2, 0xFE}) // LOOP -2
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0x200)
		flagsAddr = uint32(0x280)
	)
	writePtr := func(off, addr uint32) {
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, uint64(addr))
		if !memExport.Write(ctxAddr+off, buf) {
			t.Fatalf("seed ptr off %d", off)
		}
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	run := func(name string, ecx, flags, wantECX, wantPC, wantFlags uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
				!memExport.Write(flagsAddr, make([]byte, 4)) {
				t.Fatal("seed memory")
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, ecx)
			if !memExport.Write(regsAddr+1*4, word) {
				t.Fatal("seed ECX")
			}
			binary.LittleEndian.PutUint32(word, flags)
			if !memExport.Write(flagsAddr, word) {
				t.Fatal("seed Flags")
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			if got := readU32(regsAddr + 1*4); got != wantECX {
				t.Fatalf("ECX=%#x want %#x", got, wantECX)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantPC {
				t.Fatalf("RetPC=%#x want %#x", got, wantPC)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != 1 {
				t.Fatalf("RetCount=%d want 1", got)
			}
			if got := readU32(flagsAddr); got != wantFlags {
				t.Fatalf("Flags=%#x want %#x", got, wantFlags)
			}
		})
	}
	run("loop_taken", 2, x86FlagCF|x86FlagZF, 1, 0x100, x86FlagCF|x86FlagZF)
	run("loop_fallthrough", 1, x86FlagSF, 0, 0x102, x86FlagSF)
}

func TestX86WasmCompileBlockModule_LoopeLoopneAndJecxzParity(t *testing.T) {
	const startPC = uint32(0x100)
	for _, tc := range []struct {
		name      string
		code      []byte
		ecx       uint32
		flags     uint32
		wantECX   uint32
		wantRetPC uint32
		wantFlags uint32
	}{
		{"loope_taken", []byte{0xE1, 0xFE}, 2, x86FlagZF | x86FlagOF, 1, 0x100, x86FlagZF | x86FlagOF},
		{"loope_stop", []byte{0xE1, 0xFE}, 2, x86FlagOF, 1, 0x102, x86FlagOF},
		{"loopne_taken", []byte{0xE0, 0xFE}, 2, x86FlagCF, 1, 0x100, x86FlagCF},
		{"loopne_stop", []byte{0xE0, 0xFE}, 2, x86FlagZF | x86FlagCF, 1, 0x102, x86FlagZF | x86FlagCF},
		{"jecxz_taken", []byte{0xE3, 0x00}, 0, x86FlagPF, 0, 0x102, x86FlagPF},
		{"jecxz_not_taken", []byte{0xE3, 0x00}, 1, x86FlagPF, 1, 0x102, x86FlagPF},
		{"loop_cx_taken", []byte{0x67, 0xE2, 0xFE}, 0x12340002, x86FlagZF, 0x12340001, 0x101, x86FlagZF},
		{"loop_cx_stop", []byte{0x67, 0xE2, 0xFE}, 0x12340001, x86FlagSF, 0x12340000, 0x103, x86FlagSF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mem := make([]byte, 0x280)
			copy(mem[startPC:], tc.code)
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
				ctxAddr   = uint32(0x80)
				regsAddr  = uint32(0x200)
				flagsAddr = uint32(0x280)
			)
			if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
				!memExport.Write(flagsAddr, make([]byte, 4)) {
				t.Fatal("seed memory")
			}
			writePtr := func(off, addr uint32) {
				buf := make([]byte, 8)
				binary.LittleEndian.PutUint64(buf, uint64(addr))
				if !memExport.Write(ctxAddr+off, buf) {
					t.Fatalf("seed ptr off %d", off)
				}
			}
			writePtr(x86CtxOffJITRegsPtr, regsAddr)
			writePtr(x86CtxOffFlagsPtr, flagsAddr)
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, tc.ecx)
			if !memExport.Write(regsAddr+1*4, word) {
				t.Fatal("seed ECX")
			}
			binary.LittleEndian.PutUint32(word, tc.flags)
			if !memExport.Write(flagsAddr, word) {
				t.Fatal("seed Flags")
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call block: %v", err)
			}
			readU32 := func(addr uint32) uint32 {
				b, ok := memExport.Read(addr, 4)
				if !ok {
					t.Fatalf("read %#x", addr)
				}
				return binary.LittleEndian.Uint32(b)
			}
			if got := readU32(regsAddr + 1*4); got != tc.wantECX {
				t.Fatalf("ECX=%#x want %#x", got, tc.wantECX)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != tc.wantRetPC {
				t.Fatalf("RetPC=%#x want %#x", got, tc.wantRetPC)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != 1 {
				t.Fatalf("RetCount=%d want 1", got)
			}
			if got := readU32(flagsAddr); got != tc.wantFlags {
				t.Fatalf("Flags=%#x want %#x", got, tc.wantFlags)
			}
		})
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
	x86WasmEmitRetPCAndCount(b0, secondPC, 1, 3, 5)
	b0.end()
	f0 := m.addFunc(typ, nil, b0.code)

	b1 := &wasmBody{}
	x86WasmEmitRetPCAndCount(b1, finalPC, 1, 7, 11)
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
	if got := readU32(x86CtxOffChainCycles); got != 10 {
		t.Fatalf("ChainCycles=%d want 10", got)
	}
	if got := readU32(x86CtxOffChainTicks); got != 16 {
		t.Fatalf("ChainTicks=%d want 16", got)
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

func TestX86WasmCompileRegionModule_SupportsBackEdgeLoop(t *testing.T) {
	mem := make([]byte, 0x140)
	copy(mem[0x100:], []byte{
		0xB8, 0x01, 0x00, 0x00, 0x00, // MOV EAX,1
		0xEB, 0x09, // -> 0x110
	})
	copy(mem[0x110:], []byte{
		0x40,       // INC EAX
		0xEB, 0x0D, // -> 0x120
	})
	copy(mem[0x120:], []byte{
		0x49,       // DEC ECX
		0xEB, 0xED, // -> 0x110
	})
	region := x86FormRegion(0x100, NewCodeCache(), mem)
	if region == nil || len(region.blocks) != 3 || len(region.backEdges) != 1 || region.backEdges[2] != 1 {
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x120)
	)
	if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
		!memExport.Write(flagsAddr, make([]byte, 4)) {
		t.Fatal("seed memory")
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
		t.Fatal("seed JITRegsPtr")
	}
	binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
	if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
		t.Fatal("seed FlagsPtr")
	}
	word := make([]byte, 4)
	binary.LittleEndian.PutUint32(word, 7)
	if !memExport.Write(regsAddr+1*4, word) {
		t.Fatal("seed ECX")
	}
	binary.LittleEndian.PutUint32(word, 3)
	if !memExport.Write(ctxAddr+x86CtxOffChainBudget, word) {
		t.Fatal("seed ChainBudget")
	}
	if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
		t.Fatalf("call region: %v", err)
	}
	readU32 := func(addr uint32) uint32 {
		b, ok := memExport.Read(addr, 4)
		if !ok {
			t.Fatalf("read %#x", addr)
		}
		return binary.LittleEndian.Uint32(b)
	}
	if got := readU32(regsAddr + 0*4); got != 4 {
		t.Fatalf("EAX=%#x want %#x", got, uint32(4))
	}
	if got := readU32(regsAddr + 1*4); got != 4 {
		t.Fatalf("ECX=%#x want %#x", got, uint32(4))
	}
	if got := readU32(ctxAddr + x86CtxOffRetPC); got != 0x110 {
		t.Fatalf("RetPC=%#x want %#x", got, uint32(0x110))
	}
	if got := readU32(ctxAddr + x86CtxOffRetCount); got != 14 {
		t.Fatalf("RetCount=%d want 14", got)
	}
	if got := readU32(ctxAddr + x86CtxOffChainBudget); got != 0 {
		t.Fatalf("ChainBudget=%d want 0", got)
	}
	all := make([]X86JITInstr, 0, 6)
	for _, block := range region.blocks {
		all = append(all, block...)
	}
	prefixInstrs := len(region.blocks[0])
	cyclePrefix := x86JITCyclePrefix(all)
	tickPrefix := x86JITTickPrefix(all)
	prefixCycles := cyclePrefix[prefixInstrs-1]
	prefixTicks := tickPrefix[prefixInstrs-1]
	totalCycles := cyclePrefix[len(cyclePrefix)-1]
	totalTicks := tickPrefix[len(tickPrefix)-1]
	loopCycles := totalCycles - prefixCycles
	loopTicks := totalTicks - prefixTicks
	wantCycles := prefixCycles + 3*loopCycles
	wantTicks := prefixTicks + 3*loopTicks
	if got := uint64(readU32(ctxAddr + x86CtxOffChainCycles)); got != wantCycles {
		t.Fatalf("ChainCycles=%d want %d", got, wantCycles)
	}
	if got := uint64(readU32(ctxAddr + x86CtxOffChainTicks)); got != wantTicks {
		t.Fatalf("ChainTicks=%d want %d", got, wantTicks)
	}
}

func TestX86WasmCompileConditionalRegionModule_BranchesInternally(t *testing.T) {
	mem := make([]byte, 0x140)
	copy(mem[0x100:], []byte{
		0x85, 0xC0, // TEST EAX,EAX
		0x0F, 0x84, 0x08, 0x00, 0x00, 0x00, // JZ 0x110
	})
	copy(mem[0x108:], []byte{
		0xBB, 0x01, 0x00, 0x00, 0x00, // MOV EBX,1
		0xEB, 0x0C, // -> 0x11b
	})
	copy(mem[0x110:], []byte{
		0xBB, 0x02, 0x00, 0x00, 0x00, // MOV EBX,2
		0xEB, 0x04, // -> 0x11b
	})
	region := x86WasmFormConditionalRegion(0x100, mem)
	if region == nil {
		t.Fatal("conditional region not formed")
	}
	if got, want := region.exitPC, uint32(0x11b); got != want {
		t.Fatalf("exitPC=%#x want %#x", got, want)
	}
	compiled, err := x86WasmCompileConditionalRegionModule(region, mem)
	if err != nil {
		t.Fatalf("compile conditional region: %v", err)
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x120)
	)
	run := func(name string, eax, wantEBX, wantPC, wantCount uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
				!memExport.Write(flagsAddr, make([]byte, 4)) {
				t.Fatal("seed memory")
			}
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
			if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
				t.Fatal("seed JITRegsPtr")
			}
			binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
			if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
				t.Fatal("seed FlagsPtr")
			}
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, eax)
			if !memExport.Write(regsAddr, word) {
				t.Fatal("seed EAX")
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call region: %v", err)
			}
			readU32 := func(addr uint32) uint32 {
				b, ok := memExport.Read(addr, 4)
				if !ok {
					t.Fatalf("read %#x", addr)
				}
				return binary.LittleEndian.Uint32(b)
			}
			if got := readU32(regsAddr + 3*4); got != wantEBX {
				t.Fatalf("EBX=%#x want %#x", got, wantEBX)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantPC {
				t.Fatalf("RetPC=%#x want %#x", got, wantPC)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != wantCount {
				t.Fatalf("RetCount=%d want %d", got, wantCount)
			}
		})
	}
	run("fallthrough", 1, 1, 0x11b, 4)
	run("taken", 0, 2, 0x11b, 4)
}

func TestX86WasmCompileConditionalRegionModule_ShortJccBranchesInternally(t *testing.T) {
	mem := make([]byte, 0x140)
	copy(mem[0x100:], []byte{
		0x85, 0xC0, // TEST EAX,EAX
		0x74, 0x08, // JZ 0x10c
	})
	copy(mem[0x104:], []byte{
		0xBB, 0x01, 0x00, 0x00, 0x00, // MOV EBX,1
		0xEB, 0x10, // -> 0x11b
	})
	copy(mem[0x10c:], []byte{
		0xBB, 0x02, 0x00, 0x00, 0x00, // MOV EBX,2
		0xEB, 0x08, // -> 0x11b
	})
	region := x86WasmFormConditionalRegion(0x100, mem)
	if region == nil {
		t.Fatal("short-jcc conditional region not formed")
	}
	if got, want := region.exitPC, uint32(0x11b); got != want {
		t.Fatalf("exitPC=%#x want %#x", got, want)
	}
	compiled, err := x86WasmCompileConditionalRegionModule(region, mem)
	if err != nil {
		t.Fatalf("compile short-jcc conditional region: %v", err)
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
		ctxAddr   = uint32(0x80)
		regsAddr  = uint32(0xC0)
		flagsAddr = uint32(0x120)
	)
	run := func(name string, eax, wantEBX, wantPC, wantCount uint32) {
		t.Run(name, func(t *testing.T) {
			if !memExport.Write(ctxAddr, make([]byte, 256)) || !memExport.Write(regsAddr, make([]byte, 32)) ||
				!memExport.Write(flagsAddr, make([]byte, 4)) {
				t.Fatal("seed memory")
			}
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, uint64(regsAddr))
			if !memExport.Write(ctxAddr+x86CtxOffJITRegsPtr, buf) {
				t.Fatal("seed JITRegsPtr")
			}
			binary.LittleEndian.PutUint64(buf, uint64(flagsAddr))
			if !memExport.Write(ctxAddr+x86CtxOffFlagsPtr, buf) {
				t.Fatal("seed FlagsPtr")
			}
			word := make([]byte, 4)
			binary.LittleEndian.PutUint32(word, eax)
			if !memExport.Write(regsAddr, word) {
				t.Fatal("seed EAX")
			}
			if _, err := mod.ExportedFunction("block").Call(ctx, uint64(ctxAddr)); err != nil {
				t.Fatalf("call region: %v", err)
			}
			readU32 := func(addr uint32) uint32 {
				b, ok := memExport.Read(addr, 4)
				if !ok {
					t.Fatalf("read %#x", addr)
				}
				return binary.LittleEndian.Uint32(b)
			}
			if got := readU32(regsAddr + 3*4); got != wantEBX {
				t.Fatalf("EBX=%#x want %#x", got, wantEBX)
			}
			if got := readU32(ctxAddr + x86CtxOffRetPC); got != wantPC {
				t.Fatalf("RetPC=%#x want %#x", got, wantPC)
			}
			if got := readU32(ctxAddr + x86CtxOffRetCount); got != wantCount {
				t.Fatalf("RetCount=%d want %d", got, wantCount)
			}
		})
	}
	run("fallthrough", 1, 1, 0x11b, 4)
	run("taken", 0, 2, 0x11b, 4)
}

func TestX86WasmCompileSubsetManifestRows(t *testing.T) {
	supported := map[string]bool{
		"MOV r32,imm32":                 true,
		"MOV r8,imm8":                   true,
		"MOV r/m8,r8":                   true,
		"MOV r32,m32 guarded":           true,
		"MOV moffs guarded":             true,
		"MOV r8,m8 guarded":             true,
		"MOV m32,r32 guarded":           true,
		"MOV m,imm guarded":             true,
		"LEA SIB/disp32":                true,
		"MOVZX/MOVSX guarded memory":    true,
		"Grp1 r/m32,imm8":               true,
		"Grp2 byte,count one":           true,
		"Grp2 word/dword,count one":     true,
		"Grp2 memory,count one":         true,
		"Grp2 dword,imm8":               true,
		"Grp2 byte shift,CL":            true,
		"Grp2 word shift,CL":            true,
		"Grp2 dword shift,CL":           true,
		"Grp2 carry rotate,CL":          true,
		"INC/DEC register":              true,
		"byte ALU r/m,r":                true,
		"dword ALU r/m,r":               true,
		"ALU accumulator,imm":           true,
		"IMUL immediate":                true,
		"PUSH/POP register":             true,
		"MOV r/m32,r32":                 true,
		"MOV r32,r/m32":                 true,
		"POP guarded memory":            true,
		"PUSH immediate":                true,
		"PUSH/POP segment":              true,
		"PUSHA/POPA":                    true,
		"PUSHF/POPF":                    true,
		"CLI/STI":                       true,
		"segment MOV":                   true,
		"segment MOV guarded memory":    true,
		"TEST":                          true,
		"XCHG":                          true,
		"XCHG guarded memory":           true,
		"CBW/CWDE and CWD/CDQ":          true,
		"LES/LDS":                       true,
		"XLAT":                          true,
		"ENTER/LEAVE":                   true,
		"LEAVE":                         true,
		"MOVZX/MOVSX":                   true,
		"SETcc":                         true,
		"CMOVcc":                        true,
		"BSF/BSR":                       true,
		"BSWAP":                         true,
		"SALC":                          true,
		"ignored operand-size prefix":   true,
		"WAIT":                          true,
		"near CALL":                     true,
		"near RET":                      true,
		"Jcc":                           true,
		"LOOP":                          true,
		"near JMP":                      true,
		"bit test":                      true,
		"double shift":                  true,
		"double shift 16-bit immediate": true,
		"double shift CL":               true,
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
