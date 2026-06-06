//go:build ie64dis

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64meta"
)

func TestIE64OpcodeMetadata_StandaloneDisassemblerNamesAndFormats(t *testing.T) {
	consts := parseIE64DisOpcodeConsts(t, "ie64dis_opcodes_gen.go")
	for _, row := range ie64meta.Rows {
		gotValue, ok := consts[row.StandaloneDisName]
		if !ok {
			t.Fatalf("%s missing from generated standalone disassembler opcodes", row.StandaloneDisName)
		}
		if gotValue != row.Opcode {
			t.Fatalf("%s=0x%02X, metadata has 0x%02X", row.StandaloneDisName, gotValue, row.Opcode)
		}
		gotName, ok := opcodeNames[row.Opcode]
		if !ok {
			t.Fatalf("opcode 0x%02X missing from standalone disassembler names", row.Opcode)
		}
		if gotName != row.CanonicalMnemonic {
			t.Fatalf("opcode 0x%02X standalone name=%q, want %q", row.Opcode, gotName, row.CanonicalMnemonic)
		}
		if row.FormatClass == "" {
			t.Fatalf("%s has empty format class", row.CPUName)
		}
	}
}

func parseIE64DisOpcodeConsts(t *testing.T, path string) map[string]byte {
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
