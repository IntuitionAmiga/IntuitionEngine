package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64meta"
)

func TestIE64OpcodeMetadata_RuntimeValues(t *testing.T) {
	consts := parseOpcodeGeneratedConsts(t, "cpu_ie64_opcodes_gen.go")
	for _, row := range ie64meta.Rows {
		got, ok := consts[row.CPUName]
		if !ok {
			t.Fatalf("%s missing from generated CPU opcodes", row.CPUName)
		}
		if got != row.Opcode {
			t.Fatalf("%s=0x%02X, metadata has 0x%02X", row.CPUName, got, row.Opcode)
		}
	}
}

func TestIE64OpcodeMetadata_DebugDisassemblerNamesAndFormats(t *testing.T) {
	for _, row := range ie64meta.Rows {
		got, ok := ie64OpcodeNames[row.Opcode]
		if !ok {
			t.Fatalf("opcode 0x%02X missing from monitor disassembler names", row.Opcode)
		}
		if got != row.CanonicalMnemonic {
			t.Fatalf("opcode 0x%02X monitor name=%q, want %q", row.Opcode, got, row.CanonicalMnemonic)
		}
		if row.FormatClass == "" {
			t.Fatalf("%s has empty format class", row.CPUName)
		}
	}
}

func TestIE64OpcodeMetadata_AssemblerFormsAreSelfContained(t *testing.T) {
	for _, row := range ie64meta.Rows {
		if !row.Assemblable {
			continue
		}
		if len(row.PublicAssemblerForms) == 0 {
			t.Fatalf("%s assemblable without public assembler forms", row.CPUName)
		}
		if len(row.InternalAssemblerForms) == 0 {
			t.Fatalf("%s assemblable without internal assembler forms", row.CPUName)
		}
		for _, form := range append(row.PublicAssemblerForms, row.InternalAssemblerForms...) {
			if form == "" || bytes.ContainsAny([]byte(form), "\n\r") {
				t.Fatalf("%s has invalid one-line form %q", row.CPUName, form)
			}
		}
	}
}

func TestIE64OpcodeMetadata_GeneratorFreshness(t *testing.T) {
	tmp := t.TempDir()
	cmd := exec.Command("go", "run", "./cmd/gen_ie64_opmeta", "-out", tmp)
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(t.TempDir(), "gocache"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generator failed: %v\n%s", err, out)
	}
	for _, rel := range []string{
		"cpu_ie64_opcodes_gen.go",
		"assembler/ie64asm_opcodes_gen.go",
		"internal/asm/ie64/opcodes_gen.go",
		"debug_disasm_ie64_opcodes_gen.go",
		"assembler/ie64dis_opcodes_gen.go",
	} {
		want, err := os.ReadFile(rel)
		if err != nil {
			t.Fatalf("read checked-in %s: %v", rel, err)
		}
		got, err := os.ReadFile(filepath.Join(tmp, rel))
		if err != nil {
			t.Fatalf("read generated %s: %v", rel, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s is stale; rerun go generate ./...", rel)
		}
	}
}

func parseOpcodeGeneratedConsts(t *testing.T, path string) map[string]byte {
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
