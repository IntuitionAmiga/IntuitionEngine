//go:build ie64

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64meta"
)

func TestIE64OpcodeMetadata_PublicAssemblerValues(t *testing.T) {
	consts := parseIE64AssemblerOpcodeConsts(t, "ie64asm_opcodes_gen.go")
	for _, row := range ie64meta.Rows {
		got, ok := consts[row.PublicAssemblerName]
		if !ok {
			t.Fatalf("%s missing from generated public assembler opcodes", row.PublicAssemblerName)
		}
		if got != row.Opcode {
			t.Fatalf("%s=0x%02X, metadata has 0x%02X", row.PublicAssemblerName, got, row.Opcode)
		}
	}
}

func TestIE64OpcodeMetadata_PublicAssemblerForms(t *testing.T) {
	for _, row := range ie64meta.Rows {
		if !row.Assemblable {
			continue
		}
		for _, form := range row.PublicAssemblerForms {
			asm := NewIE64Assembler()
			src := "org $1000\n" + form + "\n"
			got, err := asm.Assemble(src)
			if err != nil {
				t.Fatalf("%s public form %q failed: %v", row.CPUName, form, err)
			}
			if len(got) != 8 {
				t.Fatalf("%s public form %q emitted %d bytes, want 8", row.CPUName, form, len(got))
			}
			if got[0] != row.Opcode {
				t.Fatalf("%s public form %q opcode=0x%02X, want 0x%02X", row.CPUName, form, got[0], row.Opcode)
			}
		}
	}
}

func parseIE64AssemblerOpcodeConsts(t *testing.T, path string) map[string]byte {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := map[string]byte{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec := spec.(*ast.ValueSpec)
			for i, name := range valueSpec.Names {
				if i >= len(valueSpec.Values) {
					t.Fatalf("%s has implicit value", name.Name)
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok {
					t.Fatalf("%s has non-literal value", name.Name)
				}
				value, err := strconv.ParseUint(lit.Value, 0, 8)
				if err != nil {
					t.Fatalf("%s has invalid value %s: %v", name.Name, lit.Value, err)
				}
				out[name.Name] = byte(value)
			}
		}
	}
	return out
}
