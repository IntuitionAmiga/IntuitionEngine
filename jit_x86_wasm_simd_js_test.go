//go:build js && wasm

package main

import (
	"math"
	"syscall/js"
	"testing"
)

func TestX86WasmSIMD_NodeDirectX87BlockInstantiates(t *testing.T) {
	if !wasmSIMDSupported() {
		t.Fatal("SIMD probe rejected by the hosting JS engine")
	}
	const startPC = uint32(0x1000)
	mem := make([]byte, 0x2000)
	copy(mem[startPC:], []byte{0xD8, 0xC1}) // FADD ST(0),ST(1)
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	global := js.Global()
	wa := global.Get("WebAssembly")
	u8 := global.Get("Uint8Array").New(len(compiled.module))
	js.CopyBytesToJS(u8, compiled.module)
	memDesc := global.Get("Object").New()
	memDesc.Set("initial", 1)
	memObj := wa.Get("Memory").New(memDesc)
	env := global.Get("Object").New()
	env.Set("mem", memObj)
	imports := global.Get("Object").New()
	imports.Set("env", env)
	inst := wa.Get("Instance").New(wa.Get("Module").New(u8), imports)
	if !inst.Truthy() {
		t.Fatal("x86 wasm SIMD block did not instantiate")
	}
}

func TestX86WasmSIMD_NodeDirectX87BlockExecutes(t *testing.T) {
	if !wasmSIMDSupported() {
		t.Fatal("SIMD probe rejected by the hosting JS engine")
	}
	const startPC = uint32(0x1000)
	mem := make([]byte, 0x2000)
	copy(mem[startPC:], []byte{0xD8, 0xC1}) // FADD ST(0),ST(1)
	instrs := x86ScanBlock(mem, startPC)
	compiled, err := x86WasmCompileBlockModule(instrs, startPC, mem)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	global := js.Global()
	wa := global.Get("WebAssembly")
	u8 := global.Get("Uint8Array").New(len(compiled.module))
	js.CopyBytesToJS(u8, compiled.module)
	memDesc := global.Get("Object").New()
	memDesc.Set("initial", 1)
	memObj := wa.Get("Memory").New(memDesc)
	env := global.Get("Object").New()
	env.Set("mem", memObj)
	imports := global.Get("Object").New()
	imports.Set("env", env)
	inst := wa.Get("Instance").New(wa.Get("Module").New(u8), imports)
	memView := global.Get("Uint8Array").New(memObj.Get("buffer"))
	ctxAddr := 0x80
	fpuAddr := 0x300
	flagsAddr := 0x280
	segsAddr := 0x2C0
	image := make([]byte, 0x1000)
	putU32 := func(off uint32, v uint32) {
		image[off+0] = byte(v)
		image[off+1] = byte(v >> 8)
		image[off+2] = byte(v >> 16)
		image[off+3] = byte(v >> 24)
	}
	putU16 := func(off uint32, v uint16) {
		image[off+0] = byte(v)
		image[off+1] = byte(v >> 8)
	}
	putF64 := func(off uint32, v float64) {
		b := math.Float64bits(v)
		putU32(off, uint32(b))
		putU32(off+4, uint32(b>>32))
	}
	putU32(uint32(ctxAddr+x86CtxOffFlagsPtr), uint32(flagsAddr))
	putU32(uint32(ctxAddr+x86CtxOffSegRegsPtr), uint32(segsAddr))
	putU32(uint32(ctxAddr+x86CtxOffFPUPtr), uint32(fpuAddr))
	putU32(uint32(ctxAddr+x86CtxOffMemSize), 0x2000)
	putU16(uint32(segsAddr+x86SegCS*2), 0x3456)
	putF64(uint32(fpuAddr+x86FPUOffRegs+0*8), 1.5)
	putF64(uint32(fpuAddr+x86FPUOffRegs+1*8), 2.25)
	putU16(uint32(fpuAddr+x86FPUOffFCW), 0x037F)
	putU16(uint32(fpuAddr+x86FPUOffFTW), 0xFFF0)
	js.CopyBytesToJS(memView, image)
	inst.Get("exports").Get("block").Invoke(ctxAddr)
	fpuBytes := make([]byte, x86FPUSize)
	js.CopyBytesToGo(fpuBytes, global.Get("Uint8Array").New(memObj.Get("buffer")).Call("subarray", fpuAddr, fpuAddr+x86FPUSize))
	if got := math.Float64frombits(uint64(fpuBytes[x86FPUOffRegs+0]) |
		uint64(fpuBytes[x86FPUOffRegs+1])<<8 |
		uint64(fpuBytes[x86FPUOffRegs+2])<<16 |
		uint64(fpuBytes[x86FPUOffRegs+3])<<24 |
		uint64(fpuBytes[x86FPUOffRegs+4])<<32 |
		uint64(fpuBytes[x86FPUOffRegs+5])<<40 |
		uint64(fpuBytes[x86FPUOffRegs+6])<<48 |
		uint64(fpuBytes[x86FPUOffRegs+7])<<56); math.Abs(got-3.75) > 1e-12 {
		t.Fatalf("ST0=%v want 3.75", got)
	}
}
