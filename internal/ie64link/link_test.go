package ie64link

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64archive"
	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
)

func TestLinkResolvesPC32AcrossObjects(t *testing.T) {
	caller := &ie64obj.Object{
		Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8), Relocations: []ie64obj.Relocation{{Symbol: 1, Type: ie64obj.RPC32}}}},
		Symbols:  []ie64obj.Symbol{{Name: "callee", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: ie64obj.SHNUndef}},
	}
	callee := &ie64obj.Object{
		Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8)}},
		Symbols:  []ie64obj.Symbol{{Name: "callee", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1}},
	}
	result, err := Link([]Input{{Name: "caller.o", Object: caller}, {Name: "callee.o", Object: callee}}, Options{Entry: "callee"})
	if err == nil || !strings.Contains(err.Error(), "must resolve to 0x1000") {
		t.Fatalf("displaced entry error = %v", err)
	}
	caller.Symbols = append(caller.Symbols, ie64obj.Symbol{Name: "_start", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1})
	result, err = Link([]Input{{Name: "caller.o", Object: caller}, {Name: "callee.o", Object: callee}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := int32(binary.LittleEndian.Uint32(result.Image[4:8])); got != 8 {
		t.Fatalf("PC32 displacement = %d, want 8", got)
	}
	if result.Symbols["_start"] != ProgStart || result.Symbols["callee"] != ProgStart+8 {
		t.Fatalf("symbols = %#v", result.Symbols)
	}
}

func TestLinkAppliesRelative64(t *testing.T) {
	obj := &ie64obj.Object{
		Sections: []ie64obj.Section{{
			Name: ".text", Type: ie64obj.SHTProgBits,
			Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8,
			Data: make([]byte, 8), Relocations: []ie64obj.Relocation{{Type: ie64obj.RRelative64, Addend: 0x28}},
		}},
		Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1}},
	}
	result, err := Link([]Input{{Name: "relative.o", Object: obj}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := binary.LittleEndian.Uint64(result.Image[:8]); got != ProgStart+0x28 {
		t.Fatalf("RELATIVE64 value = %#x, want %#x", got, ProgStart+0x28)
	}
	obj.Sections[0].Relocations[0].Symbol = 1
	if _, err := Link([]Input{{Name: "relative-symbol.o", Object: obj}}, Options{}); err == nil || !strings.Contains(err.Error(), "nonzero symbol") {
		t.Fatalf("RELATIVE64 symbol error = %v", err)
	}
}

func TestResolveArchivesRepeatsToFixedPointAndIgnoresWeakUndefined(t *testing.T) {
	makeObj := func(def string, undefs ...ie64obj.Symbol) *ie64obj.Object {
		o := &ie64obj.Object{Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)}}}
		if def != "" {
			o.Symbols = append(o.Symbols, ie64obj.Symbol{Name: def, Bind: ie64obj.STBGlobal, Section: 1})
		}
		o.Symbols = append(o.Symbols, undefs...)
		return o
	}
	start := makeObj("_start", ie64obj.Symbol{Name: "a", Bind: ie64obj.STBGlobal, Section: ie64obj.SHNUndef}, ie64obj.Symbol{Name: "weak", Bind: ie64obj.STBWeak, Section: ie64obj.SHNUndef})
	a := makeObj("a", ie64obj.Symbol{Name: "b", Bind: ie64obj.STBGlobal, Section: ie64obj.SHNUndef})
	b := makeObj("b")
	weak := makeObj("weak")
	bytesOf := func(o *ie64obj.Object) []byte {
		x, e := o.Marshal()
		if e != nil {
			t.Fatal(e)
		}
		return x
	}
	arcData, e := ie64archive.Marshal([]ie64archive.Member{{Name: "b.o", Data: bytesOf(b)}, {Name: "weak.o", Data: bytesOf(weak)}, {Name: "a.o", Data: bytesOf(a)}})
	if e != nil {
		t.Fatal(e)
	}
	arc, e := ie64archive.Parse(arcData)
	if e != nil {
		t.Fatal(e)
	}
	inputs, e := ResolveArguments([]Argument{{Name: "start.o", Object: start}, {Name: "lib.a", Archive: arc}})
	if e != nil {
		t.Fatal(e)
	}
	if len(inputs) != 3 || inputs[1].Name != "lib.a(a.o)" || inputs[2].Name != "lib.a(b.o)" {
		t.Fatalf("extracted = %#v", inputs)
	}
}

func TestResolveArchivesRetainsPicolibcOnExitArray(t *testing.T) {
	start := &ie64obj.Object{
		Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)}},
		Symbols:  []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Section: 1}},
	}
	onExit := &ie64obj.Object{
		Sections: []ie64obj.Section{
			{Name: ".text", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)},
			{Name: ".fini_array_onexit", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)},
		},
		Symbols: []ie64obj.Symbol{{Name: "__call_exitprocs", Bind: ie64obj.STBGlobal, Section: 1}},
	}
	member, err := onExit.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	archiveData, err := ie64archive.Marshal([]ie64archive.Member{{Name: "exitprocs.o", Data: member}})
	if err != nil {
		t.Fatal(err)
	}
	archive, err := ie64archive.Parse(archiveData)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := ResolveArguments([]Argument{{Name: "start.o", Object: start}, {Name: "libc.a", Archive: archive}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || inputs[1].Name != "libc.a(exitprocs.o)" {
		t.Fatalf("retained inputs = %#v", inputs)
	}
}

func TestLinkBSSAndBoundaries(t *testing.T) {
	obj := &ie64obj.Object{
		Sections: []ie64obj.Section{
			{Name: ".text", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8)},
			{Name: ".bss", Type: ie64obj.SHTNoBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFWrite, Align: 16, Size: 32},
		},
		Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1}},
	}
	result, err := Link([]Input{{Name: "a.o", Object: obj}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Image) != 8 {
		t.Fatalf("file size = %d, want 8", len(result.Image))
	}
	if result.Symbols["__bss_start"] != 0x1010 || result.Symbols["__bss_end"] != 0x1030 || result.Symbols["__heap_end"] != HeapEnd {
		t.Fatalf("boundary symbols = %#v", result.Symbols)
	}
}

func TestLinkPlacesInterruptVectorWithoutMovingHeap(t *testing.T) {
	obj := &ie64obj.Object{
		Sections: []ie64obj.Section{
			{Name: ".text", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8)},
			{Name: ".interrupt_vector", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: bytes.Repeat([]byte{0xa5}, 8)},
		},
		Symbols: []ie64obj.Symbol{
			{Name: "_start", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1},
			{Name: "interrupt_handler", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 2},
		},
	}
	result, err := Link([]Input{{Name: "vectors.o", Object: obj}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Symbols["interrupt_handler"] != InterruptVector {
		t.Fatalf("interrupt handler = %#x, want %#x", result.Symbols["interrupt_handler"], InterruptVector)
	}
	if result.Symbols["__heap_start"] != ProgStart+8 {
		t.Fatalf("heap start = %#x, want %#x", result.Symbols["__heap_start"], ProgStart+8)
	}
	if !bytes.Equal(result.Image[InterruptVector-ProgStart:InterruptVector-ProgStart+8], bytes.Repeat([]byte{0xa5}, 8)) {
		t.Fatal("interrupt vector payload was not placed at the fixed address")
	}
}

func TestLinkRejectsCommonOverlapWithInterruptVector(t *testing.T) {
	obj := &ie64obj.Object{
		Sections: []ie64obj.Section{
			{Name: ".text", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8)},
			{Name: ".interrupt_vector", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8)},
		},
		Symbols: []ie64obj.Symbol{
			{Name: "_start", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1},
			{Name: "oversized", Bind: ie64obj.STBGlobal, Section: ie64obj.SHNCommon, Value: 8, Size: InterruptVector - ProgStart},
		},
	}
	if _, err := Link([]Input{{Name: "common-vector.o", Object: obj}}, Options{}); err == nil || !strings.Contains(err.Error(), "common symbol oversized overlaps interrupt vector") {
		t.Fatalf("common/vector overlap error = %v", err)
	}
}

func TestLinkRejectsRelocationCrossingSectionBoundary(t *testing.T) {
	for _, test := range []struct {
		name   string
		typ    uint32
		offset uint64
	}{
		{name: "abs32", typ: ie64obj.RABS32, offset: 5},
		{name: "abs64", typ: ie64obj.RABS64, offset: 1},
		{name: "pc32", typ: ie64obj.RPC32, offset: 1},
		{name: "lo32", typ: ie64obj.RLO32, offset: 1},
		{name: "hi32", typ: ie64obj.RHI32, offset: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			obj := &ie64obj.Object{
				Sections: []ie64obj.Section{
					{Name: ".text", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc | ie64obj.SHFExecInstr, Align: 8, Data: make([]byte, 8), Relocations: []ie64obj.Relocation{{Offset: test.offset, Symbol: 1, Type: test.typ}}},
					{Name: ".rodata", Type: ie64obj.SHTProgBits, Flags: ie64obj.SHFAlloc, Align: 1, Data: bytes.Repeat([]byte{0xa5}, 8)},
				},
				Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Type: ie64obj.STTFunc, Section: 1}},
			}
			if _, err := Link([]Input{{Name: "crossing.o", Object: obj}}, Options{}); err == nil || !strings.Contains(err.Error(), "relocation outside section .text") {
				t.Fatalf("cross-section relocation error = %v", err)
			}
		})
	}
}

func TestLinkRejectsDuplicateAndUnresolved(t *testing.T) {
	def := func() *ie64obj.Object {
		return &ie64obj.Object{Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)}}, Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Section: 1}}}
	}
	if _, err := Link([]Input{{Name: "a.o", Object: def()}, {Name: "b.o", Object: def()}}, Options{}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
	u := def()
	u.Symbols = append(u.Symbols, ie64obj.Symbol{Name: "missing", Bind: ie64obj.STBGlobal, Section: ie64obj.SHNUndef})
	if _, err := Link([]Input{{Name: "u.o", Object: u}}, Options{}); err == nil || !strings.Contains(err.Error(), "undefined symbol missing") {
		t.Fatalf("undefined error = %v", err)
	}
}

func TestLinkOrdersAndValidatesCallableArrays(t *testing.T) {
	obj := &ie64obj.Object{Sections: []ie64obj.Section{
		{Name: ".text", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)},
		{Name: ".init_array", Type: ie64obj.SHTProgBits, Align: 8, Data: bytes.Repeat([]byte{0xbb}, 8)},
		{Name: ".init_array.20", Type: ie64obj.SHTProgBits, Align: 1, Data: bytes.Repeat([]byte{0x20}, 8)},
		{Name: ".init_array.3", Type: ie64obj.SHTProgBits, Align: 2, Data: bytes.Repeat([]byte{0x03}, 8)},
		{Name: ".fini_array_onexit", Type: ie64obj.SHTProgBits, Align: 8, Data: bytes.Repeat([]byte{0xee}, 8)},
		{Name: ".fini_array.10", Type: ie64obj.SHTProgBits, Align: 4, Data: bytes.Repeat([]byte{0x10}, 8)},
		{Name: ".fini_array", Type: ie64obj.SHTProgBits, Align: 8, Data: bytes.Repeat([]byte{0xff}, 8)},
	}, Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Section: 1}}}
	r, e := Link([]Input{{Name: "arrays.o", Object: obj}}, Options{})
	if e != nil {
		t.Fatal(e)
	}
	if got := r.Image[8:32]; !bytes.Equal(got, append(append(bytes.Repeat([]byte{3}, 8), bytes.Repeat([]byte{0x20}, 8)...), bytes.Repeat([]byte{0xbb}, 8)...)) {
		t.Fatalf("init order = %x", got)
	}
	if r.Symbols["__init_array_start"] != 0x1008 || r.Symbols["__init_array_end"] != 0x1020 || r.Symbols["__preinit_array_end"] != r.Symbols["__init_array_start"] {
		t.Fatalf("array boundaries = %#v", r.Symbols)
	}
	if got := r.Image[32:48]; !bytes.Equal(got, append(bytes.Repeat([]byte{0x10}, 8), bytes.Repeat([]byte{0xff}, 8)...)) {
		t.Fatalf("fini order = %x", got)
	}
	bad := *obj
	bad.Sections = append([]ie64obj.Section(nil), obj.Sections...)
	bad.Sections[1].Align = 16
	if _, e := Link([]Input{{Name: "bad.o", Object: &bad}}, Options{}); e == nil || !strings.Contains(e.Error(), "invalid alignment") {
		t.Fatalf("alignment error = %v", e)
	}
}

func TestLinkCollapsesAbsentInitArrayAfterPreinit(t *testing.T) {
	obj := &ie64obj.Object{Sections: []ie64obj.Section{
		{Name: ".text", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)},
		{Name: ".preinit_array", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)},
		{Name: ".data", Type: ie64obj.SHTProgBits, Align: 32, Data: make([]byte, 8)},
	}, Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Section: 1}}}
	r, err := Link([]Input{{Name: "preinit-only.o", Object: obj}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	preEnd := r.Symbols["__preinit_array_end"]
	if r.Symbols["__init_array_start"] != preEnd || r.Symbols["__init_array_end"] != preEnd {
		t.Fatalf("preinit-only boundaries = preEnd %#x, init [%#x,%#x)", preEnd,
			r.Symbols["__init_array_start"], r.Symbols["__init_array_end"])
	}
}

func TestLinkCoalescesCommonSymbols(t *testing.T) {
	obj := &ie64obj.Object{Sections: []ie64obj.Section{{Name: ".text", Type: ie64obj.SHTProgBits, Align: 8, Data: make([]byte, 8)}}, Symbols: []ie64obj.Symbol{{Name: "_start", Bind: ie64obj.STBGlobal, Section: 1}, {Name: "shared", Bind: ie64obj.STBGlobal, Section: ie64obj.SHNCommon, Value: 16, Size: 24}}}
	r, e := Link([]Input{{Name: "common.o", Object: obj}}, Options{})
	if e != nil {
		t.Fatal(e)
	}
	if r.Symbols["shared"] != 0x1010 || r.Symbols["__bss_end"] != 0x1028 {
		t.Fatalf("common symbols = %#v", r.Symbols)
	}
}
