package main

import (
	"testing"
	"unsafe"
)

// Slice 3 of native M68K FPU JIT: expose the FP register file and FPSR/FPCR to
// native code via the JIT context. The offset constants must match the struct
// layout (native code addresses these by absolute byte offset), and the
// populator must point them at the live FPU storage.

func TestM68KJITContext_FPUOffsetsMatchLayout(t *testing.T) {
	var ctx M68KJITContext
	cases := []struct {
		name  string
		konst uintptr
		got   uintptr
	}{
		{"FPRegsPtr", m68kCtxOffFPRegsPtr, unsafe.Offsetof(ctx.FPRegsPtr)},
		{"FPSRPtr", m68kCtxOffFPSRPtr, unsafe.Offsetof(ctx.FPSRPtr)},
		{"FPCRPtr", m68kCtxOffFPCRPtr, unsafe.Offsetof(ctx.FPCRPtr)},
		{"FPIARPtr", m68kCtxOffFPIARPtr, unsafe.Offsetof(ctx.FPIARPtr)},
	}
	for _, c := range cases {
		if c.konst != c.got {
			t.Errorf("%s: const offset %d != struct offset %d", c.name, c.konst, c.got)
		}
	}
}

func TestM68KJITContext_NilFPUDoesNotPanic(t *testing.T) {
	// The FPU is optional (cpu.FPU may be nil). Building the JIT context must
	// not dereference it; the FP pointers stay zero and native FPU emission is
	// gated elsewhere so FPU opcodes fall back to Line-F handling.
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	cpu.FPU = nil

	ctx := newM68KJITContext(cpu, cpu.m68kJitCodeBitmap, cpu.m68kJitCodePageMin, cpu.m68kJitCodePageMax)
	if ctx.FPRegsPtr != 0 || ctx.FPSRPtr != 0 || ctx.FPCRPtr != 0 || ctx.FPIARPtr != 0 {
		t.Fatalf("nil FPU: pointers should be zero, got FPRegs=%#x FPSR=%#x FPCR=%#x", ctx.FPRegsPtr, ctx.FPSRPtr, ctx.FPCRPtr)
	}
}

func TestM68KJITContext_FPUPointersWired(t *testing.T) {
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	ctx := newM68KJITContext(cpu, cpu.m68kJitCodeBitmap, cpu.m68kJitCodePageMin, cpu.m68kJitCodePageMax)

	if want := uintptr(unsafe.Pointer(&cpu.FPU.fp[0])); ctx.FPRegsPtr != want {
		t.Errorf("FPRegsPtr = %#x, want &FPU.fp[0] %#x", ctx.FPRegsPtr, want)
	}
	if want := uintptr(unsafe.Pointer(&cpu.FPU.FPSR)); ctx.FPSRPtr != want {
		t.Errorf("FPSRPtr = %#x, want &FPU.FPSR %#x", ctx.FPSRPtr, want)
	}
	if want := uintptr(unsafe.Pointer(&cpu.FPU.FPCR)); ctx.FPCRPtr != want {
		t.Errorf("FPCRPtr = %#x, want &FPU.FPCR %#x", ctx.FPCRPtr, want)
	}
	if want := uintptr(unsafe.Pointer(&cpu.FPU.FPIAR)); ctx.FPIARPtr != want {
		t.Errorf("FPIARPtr = %#x, want &FPU.FPIAR %#x", ctx.FPIARPtr, want)
	}
}
