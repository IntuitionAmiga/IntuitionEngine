//go:build !js

// jit_wasm_encoder_test.go - TDD suite for the wasm module encoder.
//
// The encoder is pure Go and untagged, so these tests run natively. Emitted
// modules are validated and executed under wazero, which gives the IE64 wasm
// JIT backend a red-green cycle on Linux without a browser.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"context"
	"math"
	"testing"

	"github.com/tetratelabs/wazero"
)

// ---------------------------------------------------------------------------
// Golden-byte tests: LEB128 and section framing.
// ---------------------------------------------------------------------------

func TestWasmEnc_ULEB128(t *testing.T) {
	cases := []struct {
		in   uint64
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7f}},
		{128, []byte{0x80, 0x01}},
		{624485, []byte{0xe5, 0x8e, 0x26}},
	}
	for _, c := range cases {
		if got := wasmUleb(c.in); !bytes.Equal(got, c.want) {
			t.Errorf("wasmUleb(%d) = % x, want % x", c.in, got, c.want)
		}
	}
}

func TestWasmEnc_SLEB128(t *testing.T) {
	cases := []struct {
		in   int64
		want []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{-1, []byte{0x7f}},
		{63, []byte{0x3f}},
		{64, []byte{0xc0, 0x00}},
		{-64, []byte{0x40}},
		{-65, []byte{0xbf, 0x7f}},
		{-123456, []byte{0xc0, 0xbb, 0x78}},
	}
	for _, c := range cases {
		if got := wasmSleb(c.in); !bytes.Equal(got, c.want) {
			t.Errorf("wasmSleb(%d) = % x, want % x", c.in, got, c.want)
		}
	}
}

func TestWasmEnc_EmptyModuleFraming(t *testing.T) {
	m := newWasmModuleBuilder()
	got := m.build()
	want := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("empty module = % x, want % x", got, want)
	}
}

func TestWasmEnc_TypeSectionFraming(t *testing.T) {
	m := newWasmModuleBuilder()
	idx := m.addType([]byte{wasmTypeI32}, []byte{wasmTypeI32})
	if idx != 0 {
		t.Fatalf("first type index = %d, want 0", idx)
	}
	// Adding an identical signature must dedupe to the same index.
	if again := m.addType([]byte{wasmTypeI32}, []byte{wasmTypeI32}); again != 0 {
		t.Fatalf("duplicate type index = %d, want 0", again)
	}
	got := m.build()
	want := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x06, // type section, 6 bytes
		0x01,                         // one type
		0x60, 0x01, 0x7f, 0x01, 0x7f, // (i32) -> (i32)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("module = % x, want % x", got, want)
	}
}

// ---------------------------------------------------------------------------
// Execution tests under wazero.
// ---------------------------------------------------------------------------

func wazeroRun(t *testing.T, modBytes []byte) (wazero.Runtime, context.Context) {
	t.Helper()
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	t.Cleanup(func() { _ = r.Close(ctx) })
	return r, ctx
}

func TestWasmEnc_ExecAdd(t *testing.T) {
	m := newWasmModuleBuilder()
	typ := m.addType([]byte{wasmTypeI64, wasmTypeI64}, []byte{wasmTypeI64})
	b := &wasmBody{}
	b.localGet(0)
	b.localGet(1)
	b.op(wasmOpI64Add)
	b.end()
	fn := m.addFunc(typ, nil, b.code)
	m.exportFunc("add", fn)

	r, ctx := wazeroRun(t, nil)
	mod, err := r.Instantiate(ctx, m.build())
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	res, err := mod.ExportedFunction("add").Call(ctx, 3, 4)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res[0] != 7 {
		t.Fatalf("add(3,4) = %d, want 7", res[0])
	}
}

func TestWasmEnc_ExecF64(t *testing.T) {
	m := newWasmModuleBuilder()
	typ := m.addType(nil, []byte{wasmTypeF64})
	b := &wasmBody{}
	b.f64Const(1.5)
	b.f64Const(2.25)
	b.op(wasmOpF64Add)
	b.end()
	fn := m.addFunc(typ, nil, b.code)
	m.exportFunc("fadd", fn)

	r, ctx := wazeroRun(t, nil)
	mod, err := r.Instantiate(ctx, m.build())
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	res, err := mod.ExportedFunction("fadd").Call(ctx)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if got := math.Float64frombits(res[0]); got != 3.75 {
		t.Fatalf("fadd() = %v, want 3.75", got)
	}
}

func TestWasmEnc_ExecLoopTo100(t *testing.T) {
	// One i64 local as counter; loop with br_if until it reaches 100.
	m := newWasmModuleBuilder()
	typ := m.addType(nil, []byte{wasmTypeI64})
	b := &wasmBody{}
	b.block()
	b.loop()
	b.localGet(0)
	b.i64Const(1)
	b.op(wasmOpI64Add)
	b.localTee(0)
	b.i64Const(100)
	b.op(wasmOpI64Eq)
	b.brIf(1)
	b.br(0)
	b.end() // loop
	b.end() // block
	b.localGet(0)
	b.end()
	fn := m.addFunc(typ, []byte{wasmTypeI64}, b.code)
	m.exportFunc("count", fn)

	r, ctx := wazeroRun(t, nil)
	mod, err := r.Instantiate(ctx, m.build())
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	res, err := mod.ExportedFunction("count").Call(ctx)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res[0] != 100 {
		t.Fatalf("count() = %d, want 100", res[0])
	}
}

func TestWasmEnc_SIMDProbeExecutes(t *testing.T) {
	r, ctx := wazeroRun(t, nil)
	mod, err := r.Instantiate(ctx, wasmSIMDProbeModuleBytes())
	if err != nil {
		t.Fatalf("instantiate SIMD probe: %v", err)
	}
	res, err := mod.ExportedFunction("probe").Call(ctx)
	if err != nil {
		t.Fatalf("call probe: %v", err)
	}
	if got := math.Float64frombits(res[0]); got != 3.75 {
		t.Fatalf("probe() = %v, want 3.75", got)
	}
}

func TestWasmEnc_SIMDSubopcodeUsesULEB(t *testing.T) {
	m := newWasmModuleBuilder()
	typ := m.addType(nil, []byte{wasmTypeF64})
	b := &wasmBody{}
	b.f64Const(1.5)
	b.f64x2Splat()
	b.f64Const(2.25)
	b.f64x2Splat()
	b.f64x2Add()
	b.f64x2ExtractLane(0)
	b.end()
	fn := m.addFunc(typ, nil, b.code)
	m.exportFunc("probe", fn)
	mod := m.build()
	for _, want := range [][]byte{
		{wasmOpVecPrefix, 0x14},            // f64x2.splat
		{wasmOpVecPrefix, 0xf0, 0x01},      // f64x2.add (ULEB-encoded 0xf0)
		{wasmOpVecPrefix, 0x21, 0x00},      // f64x2.extract_lane 0
		{wasmOpF64Const, 0x00, 0x00, 0x00}, // scalar f64 constants remain plain opcodes
	} {
		if !bytes.Contains(mod, want) {
			t.Fatalf("module missing SIMD byte pattern % x in % x", want, mod)
		}
	}
}

// buildEnvModule builds the provider module used by the import tests: it
// defines and exports a one-page memory, a four-slot funcref table with a
// doubling function seeded in slot 0, and the doubling function itself.
func buildEnvModule(t *testing.T) []byte {
	t.Helper()
	m := newWasmModuleBuilder()
	typ := m.addType([]byte{wasmTypeI32}, []byte{wasmTypeI32})
	b := &wasmBody{}
	b.localGet(0)
	b.i32Const(2)
	b.op(wasmOpI32Mul)
	b.end()
	double := m.addFunc(typ, nil, b.code)
	m.defineMemory(1)
	m.defineTable(4)
	m.elemSeed(0, []uint32{double})
	m.exportFunc("double", double)
	m.exportMemory("mem")
	m.exportTable("tab")
	return m.build()
}

func TestWasmEnc_ImportedMemoryLoadStore(t *testing.T) {
	env := buildEnvModule(t)

	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	pokeT := m.addType([]byte{wasmTypeI32, wasmTypeI32}, nil)
	peekT := m.addType([]byte{wasmTypeI32}, []byte{wasmTypeI32})
	pb := &wasmBody{}
	pb.localGet(0)
	pb.localGet(1)
	pb.i32Store(2, 0)
	pb.end()
	poke := m.addFunc(pokeT, nil, pb.code)
	gb := &wasmBody{}
	gb.localGet(0)
	gb.i32Load(2, 0)
	gb.end()
	peek := m.addFunc(peekT, nil, gb.code)
	m.exportFunc("poke", poke)
	m.exportFunc("peek", peek)

	r, ctx := wazeroRun(t, nil)
	if _, err := r.InstantiateWithConfig(ctx, env, wazero.NewModuleConfig().WithName("env")); err != nil {
		t.Fatalf("instantiate env: %v", err)
	}
	mod, err := r.Instantiate(ctx, m.build())
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	if _, err := mod.ExportedFunction("poke").Call(ctx, 64, 0xCAFE); err != nil {
		t.Fatalf("poke: %v", err)
	}
	res, err := mod.ExportedFunction("peek").Call(ctx, 64)
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	if res[0] != 0xCAFE {
		t.Fatalf("peek(64) = %#x, want 0xCAFE", res[0])
	}
}

func TestWasmEnc_ImportedTableCallIndirect(t *testing.T) {
	env := buildEnvModule(t)

	m := newWasmModuleBuilder()
	m.importTable("env", "tab", 4)
	typ := m.addType([]byte{wasmTypeI32}, []byte{wasmTypeI32})
	b := &wasmBody{}
	b.localGet(0)
	b.i32Const(0) // table slot 0 holds env's double
	b.callIndirect(typ)
	b.end()
	fn := m.addFunc(typ, nil, b.code)
	m.exportFunc("callIt", fn)

	r, ctx := wazeroRun(t, nil)
	if _, err := r.InstantiateWithConfig(ctx, env, wazero.NewModuleConfig().WithName("env")); err != nil {
		t.Fatalf("instantiate env: %v", err)
	}
	mod, err := r.Instantiate(ctx, m.build())
	if err != nil {
		t.Fatalf("instantiate: %v", err)
	}
	res, err := mod.ExportedFunction("callIt").Call(ctx, 21)
	if err != nil {
		t.Fatalf("callIt: %v", err)
	}
	if res[0] != 42 {
		t.Fatalf("callIt(21) = %d, want 42 via call_indirect", res[0])
	}
}
