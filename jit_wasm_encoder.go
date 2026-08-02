// jit_wasm_encoder.go - WebAssembly binary module encoder for the IE64 wasm
// JIT backend.
//
// Pure Go and untagged on purpose: the translator that uses it is exercised
// natively under wazero in the test suite, while the browser runtime feeds
// the same bytes to WebAssembly.instantiate. Only the subset of the wasm MVP
// binary format the backend needs is implemented.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"bytes"
	"encoding/binary"
	"math"
)

// Value types (wasm binary encoding).
const (
	wasmTypeI32  byte = 0x7f
	wasmTypeI64  byte = 0x7e
	wasmTypeV128 byte = 0x7b
	wasmTypeF64  byte = 0x7c
)

// Opcodes. Named constants only for the ones emitted through op(); operands
// with immediates get dedicated wasmBody methods.
const (
	wasmOpUnreachable byte = 0x00
	wasmOpNop         byte = 0x01
	wasmOpBlock       byte = 0x02
	wasmOpLoop        byte = 0x03
	wasmOpIf          byte = 0x04
	wasmOpElse        byte = 0x05
	wasmOpEnd         byte = 0x0b
	wasmOpBr          byte = 0x0c
	wasmOpBrIf        byte = 0x0d
	wasmOpReturn      byte = 0x0f
	wasmOpCall        byte = 0x10
	wasmOpCallInd     byte = 0x11
	wasmOpDrop        byte = 0x1a
	wasmOpSelect      byte = 0x1b

	wasmOpLocalGet  byte = 0x20
	wasmOpLocalSet  byte = 0x21
	wasmOpLocalTee  byte = 0x22
	wasmOpGlobalGet byte = 0x23
	wasmOpGlobalSet byte = 0x24

	wasmOpI32Load    byte = 0x28
	wasmOpI64Load    byte = 0x29
	wasmOpF64Load    byte = 0x2b
	wasmOpI32Load8U  byte = 0x2d
	wasmOpI32Load16U byte = 0x2f
	wasmOpI64Load8U  byte = 0x31
	wasmOpI64Load16U byte = 0x33
	wasmOpI64Load32U byte = 0x35
	wasmOpI32Store   byte = 0x36
	wasmOpI64Store   byte = 0x37
	wasmOpF64Store   byte = 0x39
	wasmOpI32Store8  byte = 0x3a
	wasmOpI32Store16 byte = 0x3b
	wasmOpI64Store8  byte = 0x3c
	wasmOpI64Store16 byte = 0x3d
	wasmOpI64Store32 byte = 0x3e

	wasmOpI32Const byte = 0x41
	wasmOpI64Const byte = 0x42
	wasmOpF64Const byte = 0x44

	wasmOpI32Eqz byte = 0x45
	wasmOpI32Eq  byte = 0x46
	wasmOpI32Ne  byte = 0x47
	wasmOpI32LtS byte = 0x48
	wasmOpI32LtU byte = 0x49
	wasmOpI32GtS byte = 0x4a
	wasmOpI32GtU byte = 0x4b
	wasmOpI32LeS byte = 0x4c
	wasmOpI32LeU byte = 0x4d
	wasmOpI32GeS byte = 0x4e
	wasmOpI32GeU byte = 0x4f

	wasmOpI64Eqz byte = 0x50
	wasmOpI64Eq  byte = 0x51
	wasmOpI64Ne  byte = 0x52
	wasmOpI64LtS byte = 0x53
	wasmOpI64LtU byte = 0x54
	wasmOpI64GtS byte = 0x55
	wasmOpI64GtU byte = 0x56
	wasmOpI64LeS byte = 0x57
	wasmOpI64LeU byte = 0x58
	wasmOpI64GeS byte = 0x59
	wasmOpI64GeU byte = 0x5a

	wasmOpF64Eq byte = 0x61
	wasmOpF64Ne byte = 0x62
	wasmOpF64Lt byte = 0x63
	wasmOpF64Gt byte = 0x64
	wasmOpF64Le byte = 0x65
	wasmOpF64Ge byte = 0x66

	wasmOpI32Clz    byte = 0x67
	wasmOpI32Ctz    byte = 0x68
	wasmOpI32Popcnt byte = 0x69
	wasmOpI32Add    byte = 0x6a
	wasmOpI32Sub    byte = 0x6b
	wasmOpI32Mul    byte = 0x6c
	wasmOpI32DivU   byte = 0x6e
	wasmOpI32RemU   byte = 0x70
	wasmOpI32And    byte = 0x71
	wasmOpI32Or     byte = 0x72
	wasmOpI32Xor    byte = 0x73
	wasmOpI32Shl    byte = 0x74
	wasmOpI32ShrS   byte = 0x75
	wasmOpI32ShrU   byte = 0x76
	wasmOpI32Rotl   byte = 0x77
	wasmOpI32Rotr   byte = 0x78

	wasmOpI64Add  byte = 0x7c
	wasmOpI64Sub  byte = 0x7d
	wasmOpI64Mul  byte = 0x7e
	wasmOpI64DivS byte = 0x7f
	wasmOpI64DivU byte = 0x80
	wasmOpI64RemS byte = 0x81
	wasmOpI64RemU byte = 0x82
	wasmOpI64And  byte = 0x83
	wasmOpI64Or   byte = 0x84
	wasmOpI64Xor  byte = 0x85
	wasmOpI64Shl  byte = 0x86
	wasmOpI64ShrS byte = 0x87
	wasmOpI64ShrU byte = 0x88
	wasmOpI64Rotl byte = 0x89
	wasmOpI64Rotr byte = 0x8a

	wasmOpF64Abs  byte = 0x99
	wasmOpF64Neg  byte = 0x9a
	wasmOpF64Ceil byte = 0x9b
	wasmOpF64Flr  byte = 0x9c
	wasmOpF64Trnc byte = 0x9d
	wasmOpF64Nrst byte = 0x9e
	wasmOpF64Sqrt byte = 0x9f
	wasmOpF64Add  byte = 0xa0
	wasmOpF64Sub  byte = 0xa1
	wasmOpF64Mul  byte = 0xa2
	wasmOpF64Div  byte = 0xa3

	wasmOpF32Add byte = 0x92
	wasmOpF32Sub byte = 0x93
	wasmOpF32Mul byte = 0x94
	wasmOpF32Div byte = 0x95

	wasmOpF32DemoteF64  byte = 0xb6
	wasmOpF64PromoteF32 byte = 0xbb

	wasmOpI32WrapI64        byte = 0xa7
	wasmOpI64ExtendI32S     byte = 0xac
	wasmOpI64ExtendI32U     byte = 0xad
	wasmOpI64TruncF64S      byte = 0xb0
	wasmOpF64ConvertI64S    byte = 0xb9
	wasmOpF64ConvertI64U    byte = 0xba
	wasmOpI64ReinterpretF64 byte = 0xbd
	wasmOpI32ReinterpretF32 byte = 0xbc
	wasmOpF64ReinterpretI64 byte = 0xbf
	wasmOpF32ReinterpretI32 byte = 0xbe
	wasmOpI64Extend8S       byte = 0xc2
	wasmOpI64Extend16S      byte = 0xc3
	wasmOpI64Extend32S      byte = 0xc4
	wasmOpVecPrefix         byte = 0xfd
)

// Vector SIMD sub-opcodes in the 0xFD prefixed space. Only the subset needed
// by the shared x86/wasm backend is defined here; further SIMD lowerings can
// extend this table as they become live.
type wasmVecOp uint32

const (
	wasmVecV128Load  wasmVecOp = 0x00
	wasmVecV128Store wasmVecOp = 0x0b
	wasmVecV128Const wasmVecOp = 0x0c

	wasmVecV128And wasmVecOp = 0x4e
	wasmVecV128Or  wasmVecOp = 0x50
	wasmVecV128Xor wasmVecOp = 0x51

	wasmVecF64x2Splat       wasmVecOp = 0x14
	wasmVecF64x2ExtractLane wasmVecOp = 0x21
	wasmVecF64x2ReplaceLane wasmVecOp = 0x22
	wasmVecF64x2Abs         wasmVecOp = 0xec
	wasmVecF64x2Neg         wasmVecOp = 0xed
	wasmVecF64x2Sqrt        wasmVecOp = 0xef
	wasmVecF64x2Add         wasmVecOp = 0xf0
	wasmVecF64x2Sub         wasmVecOp = 0xf1
	wasmVecF64x2Mul         wasmVecOp = 0xf2
	wasmVecF64x2Div         wasmVecOp = 0xf3
	wasmVecF64x2Min         wasmVecOp = 0xf4
	wasmVecF64x2Max         wasmVecOp = 0xf5
)

// wasmUleb encodes v as unsigned LEB128.
func wasmUleb(v uint64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

// wasmSleb encodes v as signed LEB128.
func wasmSleb(v int64) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if (v == 0 && b&0x40 == 0) || (v == -1 && b&0x40 != 0) {
			out = append(out, b)
			return out
		}
		out = append(out, b|0x80)
	}
}

// ---------------------------------------------------------------------------
// Function body writer
// ---------------------------------------------------------------------------

// wasmBody accumulates the instruction bytes of one function body. Locals
// are declared separately via addFunc.
type wasmBody struct {
	code []byte
}

func (b *wasmBody) op(o byte)     { b.code = append(b.code, o) }
func (b *wasmBody) raw(p []byte)  { b.code = append(b.code, p...) }
func (b *wasmBody) uleb(v uint64) { b.code = append(b.code, wasmUleb(v)...) }
func (b *wasmBody) vecOp(o wasmVecOp) {
	b.op(wasmOpVecPrefix)
	b.uleb(uint64(o))
}

func (b *wasmBody) localGet(i uint32) { b.op(wasmOpLocalGet); b.uleb(uint64(i)) }
func (b *wasmBody) localSet(i uint32) { b.op(wasmOpLocalSet); b.uleb(uint64(i)) }
func (b *wasmBody) localTee(i uint32) { b.op(wasmOpLocalTee); b.uleb(uint64(i)) }

func (b *wasmBody) i32Const(v int32) { b.op(wasmOpI32Const); b.raw(wasmSleb(int64(v))) }
func (b *wasmBody) i64Const(v int64) { b.op(wasmOpI64Const); b.raw(wasmSleb(v)) }
func (b *wasmBody) f64Const(v float64) {
	b.op(wasmOpF64Const)
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], math.Float64bits(v))
	b.raw(buf[:])
}

// memArg emits the align/offset immediate pair shared by all load/store ops.
func (b *wasmBody) memOp(o byte, align, offset uint32) {
	b.op(o)
	b.uleb(uint64(align))
	b.uleb(uint64(offset))
}

func (b *wasmBody) vecMemOp(o wasmVecOp, align, offset uint32) {
	b.vecOp(o)
	b.uleb(uint64(align))
	b.uleb(uint64(offset))
}

func (b *wasmBody) i32Load(align, offset uint32) { b.memOp(wasmOpI32Load, align, offset) }
func (b *wasmBody) i32Load8U(align, offset uint32) {
	b.memOp(wasmOpI32Load8U, align, offset)
}
func (b *wasmBody) i32Load16U(align, offset uint32) {
	b.memOp(wasmOpI32Load16U, align, offset)
}
func (b *wasmBody) i64Load(align, offset uint32)  { b.memOp(wasmOpI64Load, align, offset) }
func (b *wasmBody) f64Load(align, offset uint32)  { b.memOp(wasmOpF64Load, align, offset) }
func (b *wasmBody) i32Store(align, offset uint32) { b.memOp(wasmOpI32Store, align, offset) }
func (b *wasmBody) i32Store16(align, offset uint32) {
	b.memOp(wasmOpI32Store16, align, offset)
}
func (b *wasmBody) i32Store8(align, offset uint32) {
	b.memOp(wasmOpI32Store8, align, offset)
}
func (b *wasmBody) i64Store(align, offset uint32) { b.memOp(wasmOpI64Store, align, offset) }
func (b *wasmBody) f64Store(align, offset uint32) { b.memOp(wasmOpF64Store, align, offset) }
func (b *wasmBody) v128Load(align, offset uint32) { b.vecMemOp(wasmVecV128Load, align, offset) }
func (b *wasmBody) v128Store(align, offset uint32) {
	b.vecMemOp(wasmVecV128Store, align, offset)
}
func (b *wasmBody) v128Const(v [16]byte) {
	b.vecOp(wasmVecV128Const)
	b.raw(v[:])
}
func (b *wasmBody) v128And() { b.vecOp(wasmVecV128And) }
func (b *wasmBody) v128Or()  { b.vecOp(wasmVecV128Or) }
func (b *wasmBody) v128Xor() { b.vecOp(wasmVecV128Xor) }
func (b *wasmBody) f64x2Splat() {
	b.vecOp(wasmVecF64x2Splat)
}
func (b *wasmBody) f64x2ExtractLane(lane byte) {
	b.vecOp(wasmVecF64x2ExtractLane)
	b.op(lane)
}
func (b *wasmBody) f64x2ReplaceLane(lane byte) {
	b.vecOp(wasmVecF64x2ReplaceLane)
	b.op(lane)
}
func (b *wasmBody) f64x2Abs()  { b.vecOp(wasmVecF64x2Abs) }
func (b *wasmBody) f64x2Neg()  { b.vecOp(wasmVecF64x2Neg) }
func (b *wasmBody) f64x2Sqrt() { b.vecOp(wasmVecF64x2Sqrt) }
func (b *wasmBody) f64x2Add()  { b.vecOp(wasmVecF64x2Add) }
func (b *wasmBody) f64x2Sub()  { b.vecOp(wasmVecF64x2Sub) }
func (b *wasmBody) f64x2Mul()  { b.vecOp(wasmVecF64x2Mul) }
func (b *wasmBody) f64x2Div()  { b.vecOp(wasmVecF64x2Div) }
func (b *wasmBody) f64x2Min()  { b.vecOp(wasmVecF64x2Min) }
func (b *wasmBody) f64x2Max()  { b.vecOp(wasmVecF64x2Max) }

// block/loop open void-typed structured control frames; end closes the
// innermost frame (or the function itself as the final end).
func (b *wasmBody) block()          { b.op(wasmOpBlock); b.op(0x40) }
func (b *wasmBody) loop()           { b.op(wasmOpLoop); b.op(0x40) }
func (b *wasmBody) br(depth uint32) { b.op(wasmOpBr); b.uleb(uint64(depth)) }
func (b *wasmBody) brIf(depth uint32) {
	b.op(wasmOpBrIf)
	b.uleb(uint64(depth))
}
func (b *wasmBody) end() { b.op(wasmOpEnd) }

// ifTyped opens an if frame whose branches leave one value of the given type
// on the stack; elseBranch switches to the else arm.
func (b *wasmBody) ifTyped(valType byte) { b.op(wasmOpIf); b.op(valType) }
func (b *wasmBody) ifVoid()              { b.op(wasmOpIf); b.op(0x40) }
func (b *wasmBody) elseBranch()          { b.op(wasmOpElse) }

// call emits a direct call to a function by index.
func (b *wasmBody) call(fnIdx uint32) {
	b.op(wasmOpCall)
	b.uleb(uint64(fnIdx))
}

// callIndirect calls through table 0 with the given type index; the callee
// table slot is the value on top of the stack.
func (b *wasmBody) callIndirect(typeIdx uint32) {
	b.op(wasmOpCallInd)
	b.uleb(uint64(typeIdx))
	b.op(0x00) // table index 0
}

// ---------------------------------------------------------------------------
// Module builder
// ---------------------------------------------------------------------------

type wasmFuncDef struct {
	typeIdx uint32
	locals  []byte // one value type per extra local, in declaration order
	body    []byte
}

type wasmImportDef struct {
	module, name string
	kind         byte // 0x01 table, 0x02 memory
	min          uint32
}

type wasmExportDef struct {
	name string
	kind byte // 0x00 func, 0x01 table, 0x02 memory
	idx  uint32
}

type wasmElemDef struct {
	offset  uint32
	funcIdx []uint32
}

// wasmModuleBuilder assembles a complete wasm binary. Function indices are
// simple: the backend imports no functions, so the first added function has
// index 0.
type wasmModuleBuilder struct {
	types     [][]byte // full encoded func types (0x60 params results)
	imports   []wasmImportDef
	funcs     []wasmFuncDef
	hasMemory bool
	memoryMin uint32
	hasTable  bool
	tableMin  uint32
	exports   []wasmExportDef
	elems     []wasmElemDef
}

func newWasmModuleBuilder() *wasmModuleBuilder {
	return &wasmModuleBuilder{}
}

// addType interns the signature and returns its type index.
func (m *wasmModuleBuilder) addType(params, results []byte) uint32 {
	enc := []byte{0x60}
	enc = append(enc, wasmUleb(uint64(len(params)))...)
	enc = append(enc, params...)
	enc = append(enc, wasmUleb(uint64(len(results)))...)
	enc = append(enc, results...)
	for i, t := range m.types {
		if bytes.Equal(t, enc) {
			return uint32(i)
		}
	}
	m.types = append(m.types, enc)
	return uint32(len(m.types) - 1)
}

func (m *wasmModuleBuilder) importMemory(module, name string, minPages uint32) {
	m.imports = append(m.imports, wasmImportDef{module, name, 0x02, minPages})
}

func (m *wasmModuleBuilder) importTable(module, name string, minElems uint32) {
	m.imports = append(m.imports, wasmImportDef{module, name, 0x01, minElems})
}

func (m *wasmModuleBuilder) defineMemory(minPages uint32) {
	m.hasMemory = true
	m.memoryMin = minPages
}

func (m *wasmModuleBuilder) defineTable(minElems uint32) {
	m.hasTable = true
	m.tableMin = minElems
}

// addFunc registers a function and returns its index. locals lists the value
// type of each extra local (params come from the signature).
func (m *wasmModuleBuilder) addFunc(typeIdx uint32, locals []byte, body []byte) uint32 {
	m.funcs = append(m.funcs, wasmFuncDef{typeIdx, locals, body})
	return uint32(len(m.funcs) - 1)
}

func (m *wasmModuleBuilder) exportFunc(name string, idx uint32) {
	m.exports = append(m.exports, wasmExportDef{name, 0x00, idx})
}

func (m *wasmModuleBuilder) exportTable(name string) {
	m.exports = append(m.exports, wasmExportDef{name, 0x01, 0})
}

func (m *wasmModuleBuilder) exportMemory(name string) {
	m.exports = append(m.exports, wasmExportDef{name, 0x02, 0})
}

// elemSeed places funcIdx values into the module's table starting at offset.
func (m *wasmModuleBuilder) elemSeed(offset uint32, funcIdx []uint32) {
	m.elems = append(m.elems, wasmElemDef{offset, funcIdx})
}

func wasmSection(id byte, payload []byte) []byte {
	out := []byte{id}
	out = append(out, wasmUleb(uint64(len(payload)))...)
	return append(out, payload...)
}

func wasmName(s string) []byte {
	out := wasmUleb(uint64(len(s)))
	return append(out, s...)
}

// wasmLimits encodes min-only limits (flag 0x00).
func wasmLimits(min uint32) []byte {
	out := []byte{0x00}
	return append(out, wasmUleb(uint64(min))...)
}

func (m *wasmModuleBuilder) build() []byte {
	mod := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

	// Type section (1).
	if len(m.types) > 0 {
		p := wasmUleb(uint64(len(m.types)))
		for _, t := range m.types {
			p = append(p, t...)
		}
		mod = append(mod, wasmSection(1, p)...)
	}

	// Import section (2).
	if len(m.imports) > 0 {
		p := wasmUleb(uint64(len(m.imports)))
		for _, im := range m.imports {
			p = append(p, wasmName(im.module)...)
			p = append(p, wasmName(im.name)...)
			p = append(p, im.kind)
			if im.kind == 0x01 {
				p = append(p, 0x70) // funcref
			}
			p = append(p, wasmLimits(im.min)...)
		}
		mod = append(mod, wasmSection(2, p)...)
	}

	// Function section (3).
	if len(m.funcs) > 0 {
		p := wasmUleb(uint64(len(m.funcs)))
		for _, f := range m.funcs {
			p = append(p, wasmUleb(uint64(f.typeIdx))...)
		}
		mod = append(mod, wasmSection(3, p)...)
	}

	// Table section (4).
	if m.hasTable {
		p := []byte{0x01, 0x70}
		p = append(p, wasmLimits(m.tableMin)...)
		mod = append(mod, wasmSection(4, p)...)
	}

	// Memory section (5).
	if m.hasMemory {
		p := []byte{0x01}
		p = append(p, wasmLimits(m.memoryMin)...)
		mod = append(mod, wasmSection(5, p)...)
	}

	// Export section (7).
	if len(m.exports) > 0 {
		p := wasmUleb(uint64(len(m.exports)))
		for _, e := range m.exports {
			p = append(p, wasmName(e.name)...)
			p = append(p, e.kind)
			p = append(p, wasmUleb(uint64(e.idx))...)
		}
		mod = append(mod, wasmSection(7, p)...)
	}

	// Element section (9), active segments on table 0.
	if len(m.elems) > 0 {
		p := wasmUleb(uint64(len(m.elems)))
		for _, e := range m.elems {
			p = append(p, 0x00) // active, table 0, expr offset
			p = append(p, wasmOpI32Const)
			p = append(p, wasmSleb(int64(e.offset))...)
			p = append(p, wasmOpEnd)
			p = append(p, wasmUleb(uint64(len(e.funcIdx)))...)
			for _, fi := range e.funcIdx {
				p = append(p, wasmUleb(uint64(fi))...)
			}
		}
		mod = append(mod, wasmSection(9, p)...)
	}

	// Code section (10).
	if len(m.funcs) > 0 {
		p := wasmUleb(uint64(len(m.funcs)))
		for _, f := range m.funcs {
			var body []byte
			// Locals are declared as (count, type) runs; adjacent locals of
			// the same type collapse into one run.
			var runs [][2]uint32 // count, type
			for _, lt := range f.locals {
				if n := len(runs); n > 0 && runs[n-1][1] == uint32(lt) {
					runs[n-1][0]++
				} else {
					runs = append(runs, [2]uint32{1, uint32(lt)})
				}
			}
			body = append(body, wasmUleb(uint64(len(runs)))...)
			for _, r := range runs {
				body = append(body, wasmUleb(uint64(r[0]))...)
				body = append(body, byte(r[1]))
			}
			body = append(body, f.body...)
			p = append(p, wasmUleb(uint64(len(body)))...)
			p = append(p, body...)
		}
		mod = append(mod, wasmSection(10, p)...)
	}

	return mod
}
