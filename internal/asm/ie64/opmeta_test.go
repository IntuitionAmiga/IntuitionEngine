package ie64

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64meta"
)

func TestIE64OpcodeMetadata_InternalAssemblerValues(t *testing.T) {
	consts := parseInternalOpcodeConsts(t, "opcodes_gen.go")
	for _, row := range ie64meta.Rows {
		got, ok := consts[row.InternalAssemblerName]
		if !ok {
			t.Fatalf("%s missing from generated internal assembler opcodes", row.InternalAssemblerName)
		}
		if got != row.Opcode {
			t.Fatalf("%s=0x%02X, metadata has 0x%02X", row.InternalAssemblerName, got, row.Opcode)
		}
	}
}

func TestIE64OpcodeMetadata_InternalAssemblerForms(t *testing.T) {
	for _, row := range ie64meta.Rows {
		if !row.Assemblable {
			continue
		}
		for _, form := range row.InternalAssemblerForms {
			got := AssembleInstruction(row.InternalOrigin, form)
			if len(got.Diagnostics) != 0 {
				t.Fatalf("%s internal form %q diagnostics: %#v", row.CPUName, form, got.Diagnostics)
			}
			if len(got.Bytes) != 8 {
				t.Fatalf("%s internal form %q emitted %d bytes, want 8", row.CPUName, form, len(got.Bytes))
			}
			if got.Bytes[0] != row.Opcode {
				t.Fatalf("%s internal form %q opcode=0x%02X, want 0x%02X", row.CPUName, form, got.Bytes[0], row.Opcode)
			}
		}
	}
}

func parseInternalOpcodeConsts(t *testing.T, path string) map[string]byte {
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
