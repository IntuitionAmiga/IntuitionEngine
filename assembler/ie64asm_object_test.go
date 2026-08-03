//go:build ie64

package main

import (
	"bytes"
	"debug/elf"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
)

func TestIE64ObjectAssemblerSectionsSymbolsAndCallRelocation(t *testing.T) {
	src := `.section .text,"ax"
.global _start
.type _start,@function
_start:
    move.l r1,#lo32(callee+4)
    movt r1,#hi32(callee+4)
    jsr callee
.size _start,8
.global callee
.section .rodata,"a"
.align 8
message:
    dc.q callee
`
	data, err := AssembleIE64Object(src, "test.s", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := elf.NewFile(bytes.NewReader(data)); err != nil {
		t.Fatalf("invalid ELF: %v", err)
	}
	obj, err := ie64obj.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(obj.Sections) != 2 || obj.Sections[0].Name != ".text" || obj.Sections[1].Name != ".rodata" {
		t.Fatalf("sections = %#v", obj.Sections)
	}
	if len(obj.Sections[0].Relocations) != 3 || obj.Sections[0].Relocations[0].Type != ie64obj.RLO32 || obj.Sections[0].Relocations[1].Type != ie64obj.RHI32 || obj.Sections[0].Relocations[2].Type != ie64obj.RPC32 || obj.Sections[0].Relocations[0].Addend != 4 {
		t.Fatalf("text relocations = %#v", obj.Sections[0].Relocations)
	}
	if len(obj.Sections[1].Relocations) != 1 || obj.Sections[1].Relocations[0].Type != ie64obj.RABS64 {
		t.Fatalf("rodata relocations = %#v", obj.Sections[1].Relocations)
	}
	if len(obj.Symbols) != 3 || obj.Symbols[0].Name != "message" || obj.Symbols[1].Name != "_start" || obj.Symbols[2].Name != "callee" {
		t.Fatalf("symbols = %#v", obj.Symbols)
	}
}

func TestIE64ObjectAssemblerLegacySourceLivesInText(t *testing.T) {
	data, err := AssembleIE64Object("entry:\n dc.q 1\n", "legacy.s", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := ie64obj.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(obj.Sections) != 1 || obj.Sections[0].Name != ".text" || len(obj.Sections[0].Data) != 8 {
		t.Fatalf("sections = %#v", obj.Sections)
	}
}

func TestIE64ObjectAssemblerRejectsUnsupportedVisibility(t *testing.T) {
	if _, err := AssembleIE64Object(".global x\n.visibility x,protected\nx:\n halt\n", "bad.s", nil, nil); err == nil {
		t.Fatal("expected visibility error")
	}
}

func TestIE64ObjectAssemblerKeepsNonzeroLocalLabelValue(t *testing.T) {
	src := `.section .text,"ax"
.global first
first:
    halt
.align 16
.local .Lloop
.Lloop:
    bne r1,r0,.Lloop
`
	data, err := AssembleIE64Object(src, "labels.s", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := ie64obj.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, symbol := range obj.Symbols {
		if symbol.Name == "first.Lloop" {
			if symbol.Value != 16 {
				t.Fatalf("first.Lloop value = %#x, want %#x", symbol.Value, uint64(16))
			}
			return
		}
	}
	t.Fatal("missing first.Lloop symbol")
}

func TestIE64ObjectAssemblerAllowsLocalLabelBeforeGlobal(t *testing.T) {
	source := `.data
.Lstring:
    dc.q .Lstring
`
	data, err := AssembleIE64Object(source, "local-first.s", nil, nil)
	if err != nil {
		t.Fatalf("assemble object: %v", err)
	}
	obj, err := ie64obj.Parse(data)
	if err != nil {
		t.Fatalf("parse object: %v", err)
	}
	var found bool
	for _, sym := range obj.Symbols {
		if sym.Name == "__ie64_object_scope_2.Lstring" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("missing canonical local symbol")
	}
}
