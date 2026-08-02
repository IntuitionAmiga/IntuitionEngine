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
		"MOV r32,imm32":               true,
		"MOV r8,imm8":                 true,
		"MOV r32,m32 guarded":         true,
		"MOV m32,r32 guarded":         true,
		"MOVZX/MOVSX guarded memory":  true,
		"Grp1 r/m32,imm8":             true,
		"Grp2 word/dword,count one":   true,
		"Grp2 dword,imm8":             true,
		"Grp2 dword shift,CL":         true,
		"INC/DEC register":            true,
		"dword ALU r/m,r":             true,
		"ALU accumulator,imm":         true,
		"MOV r/m32,r32":               true,
		"MOV r32,r/m32":               true,
		"TEST":                        true,
		"XCHG":                        true,
		"MOVZX/MOVSX":                 true,
		"SETcc":                       true,
		"CMOVcc":                      true,
		"BSWAP":                       true,
		"ignored operand-size prefix": true,
		"WAIT":                        true,
		"near CALL":                   true,
		"near RET":                    true,
		"Jcc":                         true,
		"LOOP":                        true,
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
