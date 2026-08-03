package ie64obj

import (
	"bytes"
	"debug/elf"
	"testing"
)

func TestWriteRelocatableIdentityAndDeterminism(t *testing.T) {
	obj := &Object{
		Sections: []Section{{Name: ".text", Type: SHTProgBits, Flags: SHFAlloc | SHFExecInstr, Align: 8, Data: make([]byte, 8)}},
		Symbols:  []Symbol{{Name: "entry", Bind: STBGlobal, Type: STTFunc, Section: 1, Size: 8}},
	}
	a, err := obj.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	b, err := obj.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("identical objects produced different bytes")
	}
	f, err := elf.NewFile(bytes.NewReader(a))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB || f.Type != elf.ET_REL {
		t.Fatalf("identity = class %v data %v type %v", f.Class, f.Data, f.Type)
	}
	if got := uint16(f.Machine); got != EMIE64 {
		t.Fatalf("machine = %#x, want %#x", got, EMIE64)
	}
	if f.Entry != 0 {
		t.Fatalf("entry = %#x, want zero", f.Entry)
	}
}

func TestWriteRelocatableSymbolsAndRELA(t *testing.T) {
	obj := &Object{
		Sections: []Section{{
			Name: ".text", Type: SHTProgBits, Flags: SHFAlloc | SHFExecInstr, Align: 8,
			Data:        make([]byte, 8),
			Relocations: []Relocation{{Offset: 0, Symbol: 2, Type: RPC32, Addend: -8}},
		}},
		Symbols: []Symbol{
			{Name: "caller", Bind: STBGlobal, Type: STTFunc, Section: 1, Size: 8},
			{Name: "callee", Bind: STBGlobal, Type: STTFunc, Section: SHNUndef},
		},
	}
	data, err := obj.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Flags != EFIE64ABIV2 {
		t.Fatalf("flags = %#x, want %#x", parsed.Flags, EFIE64ABIV2)
	}
	if len(parsed.Sections) != 1 || len(parsed.Sections[0].Relocations) != 1 {
		t.Fatalf("sections/relocations = %#v", parsed.Sections)
	}
	r := parsed.Sections[0].Relocations[0]
	if r.Type != RPC32 || r.Symbol != 2 || r.Addend != -8 {
		t.Fatalf("relocation = %#v", r)
	}
	if len(parsed.Symbols) != 2 || parsed.Symbols[1].Section != SHNUndef {
		t.Fatalf("symbols = %#v", parsed.Symbols)
	}
}

func TestRejectReservedABIFlagBits(t *testing.T) {
	obj := &Object{Flags: EFIE64ABIV2 | 0x10}
	if _, err := obj.Marshal(); err == nil {
		t.Fatal("expected reserved e_flags rejection")
	}
}
