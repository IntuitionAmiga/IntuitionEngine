//go:build ie64dis

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

type opRow struct {
	cpuName  string
	opcode   byte
	mnemonic string
}

// If you add an opcode to cpu_ie64.go, append it to documentedOpcodes and
// monDocumentedOpcodes and ensure both disassemblers render it.
var documentedOpcodes = []opRow{
	{"OP_MOVE", 0x01, "move"}, {"OP_MOVT", 0x02, "movt"}, {"OP_MOVEQ", 0x03, "moveq"}, {"OP_LEA", 0x04, "lea"},
	{"OP_LOAD", 0x10, "load"}, {"OP_STORE", 0x11, "store"},
	{"OP_ADD", 0x20, "add"}, {"OP_SUB", 0x21, "sub"}, {"OP_MULU", 0x22, "mulu"}, {"OP_MULS", 0x23, "muls"},
	{"OP_DIVU", 0x24, "divu"}, {"OP_DIVS", 0x25, "divs"}, {"OP_MOD64", 0x26, "mod"}, {"OP_NEG", 0x27, "neg"},
	{"OP_MODS", 0x28, "mods"}, {"OP_MULHU", 0x29, "mulhu"}, {"OP_MULHS", 0x2A, "mulhs"},
	{"OP_AND64", 0x30, "and"}, {"OP_OR64", 0x31, "or"}, {"OP_EOR", 0x32, "eor"}, {"OP_NOT64", 0x33, "not"},
	{"OP_LSL", 0x34, "lsl"}, {"OP_LSR", 0x35, "lsr"}, {"OP_ASR", 0x36, "asr"}, {"OP_CLZ", 0x37, "clz"},
	{"OP_SEXT", 0x38, "sext"}, {"OP_ROL", 0x39, "rol"}, {"OP_ROR", 0x3A, "ror"}, {"OP_CTZ", 0x3B, "ctz"},
	{"OP_POPCNT", 0x3C, "popcnt"}, {"OP_BSWAP", 0x3D, "bswap"},
	{"OP_BRA", 0x40, "bra"}, {"OP_BEQ", 0x41, "beq"}, {"OP_BNE", 0x42, "bne"}, {"OP_BLT", 0x43, "blt"},
	{"OP_BGE", 0x44, "bge"}, {"OP_BGT", 0x45, "bgt"}, {"OP_BLE", 0x46, "ble"}, {"OP_BHI", 0x47, "bhi"},
	{"OP_BLS", 0x48, "bls"}, {"OP_JMP", 0x49, "jmp"},
	{"OP_JSR64", 0x50, "jsr"}, {"OP_RTS64", 0x51, "rts"}, {"OP_PUSH64", 0x52, "push"}, {"OP_POP64", 0x53, "pop"},
	{"OP_JSR_IND", 0x54, "jsr"},
	{"OP_FMOV", 0x60, "fmov"}, {"OP_FLOAD", 0x61, "fload"}, {"OP_FSTORE", 0x62, "fstore"},
	{"OP_FADD", 0x63, "fadd"}, {"OP_FSUB", 0x64, "fsub"}, {"OP_FMUL", 0x65, "fmul"}, {"OP_FDIV", 0x66, "fdiv"},
	{"OP_FMOD", 0x67, "fmod"}, {"OP_FABS", 0x68, "fabs"}, {"OP_FNEG", 0x69, "fneg"}, {"OP_FSQRT", 0x6A, "fsqrt"},
	{"OP_FINT", 0x6B, "fint"}, {"OP_FCMP", 0x6C, "fcmp"}, {"OP_FCVTIF", 0x6D, "fcvtif"}, {"OP_FCVTFI", 0x6E, "fcvtfi"},
	{"OP_FMOVI", 0x6F, "fmovi"}, {"OP_FMOVO", 0x70, "fmovo"}, {"OP_FSIN", 0x71, "fsin"}, {"OP_FCOS", 0x72, "fcos"},
	{"OP_FTAN", 0x73, "ftan"}, {"OP_FATAN", 0x74, "fatan"}, {"OP_FLOG", 0x75, "flog"}, {"OP_FEXP", 0x76, "fexp"},
	{"OP_FPOW", 0x77, "fpow"}, {"OP_FMOVECR", 0x78, "fmovecr"}, {"OP_FMOVSR", 0x79, "fmovsr"}, {"OP_FMOVCR", 0x7A, "fmovcr"},
	{"OP_FMOVSC", 0x7B, "fmovsc"}, {"OP_FMOVCC", 0x7C, "fmovcc"},
	{"OP_DMOV", 0x80, "dmov"}, {"OP_DLOAD", 0x81, "dload"}, {"OP_DSTORE", 0x82, "dstore"},
	{"OP_DADD", 0x83, "dadd"}, {"OP_DSUB", 0x84, "dsub"}, {"OP_DMUL", 0x85, "dmul"}, {"OP_DDIV", 0x86, "ddiv"},
	{"OP_DMOD", 0x87, "dmod"}, {"OP_DABS", 0x88, "dabs"}, {"OP_DNEG", 0x89, "dneg"}, {"OP_DSQRT", 0x8A, "dsqrt"},
	{"OP_DINT", 0x8B, "dint"}, {"OP_DCMP", 0x8C, "dcmp"}, {"OP_DCVTIF", 0x8D, "dcvtif"}, {"OP_DCVTFI", 0x8E, "dcvtfi"},
	{"OP_FCVTSD", 0x8F, "fcvtsd"}, {"OP_FCVTDS", 0x90, "fcvtds"},
	{"OP_NOP64", 0xE0, "nop"}, {"OP_HALT64", 0xE1, "halt"}, {"OP_SEI64", 0xE2, "sei"}, {"OP_CLI64", 0xE3, "cli"},
	{"OP_RTI64", 0xE4, "rti"}, {"OP_WAIT64", 0xE5, "wait"}, {"OP_MTCR", 0xE6, "mtcr"}, {"OP_MFCR", 0xE7, "mfcr"},
	{"OP_ERET", 0xE8, "eret"}, {"OP_TLBFLUSH", 0xE9, "tlbflush"}, {"OP_TLBINVAL", 0xEA, "tlbinval"}, {"OP_SYSCALL", 0xEB, "syscall"},
	{"OP_SMODE", 0xEC, "smode"}, {"OP_CAS", 0xED, "cas"}, {"OP_XCHG", 0xEE, "xchg"}, {"OP_FAA", 0xEF, "faa"},
	{"OP_FAND", 0xF0, "fand"}, {"OP_FOR", 0xF1, "for"}, {"OP_FXOR", 0xF2, "fxor"}, {"OP_SUAEN", 0xF3, "suaen"},
	{"OP_SUADIS", 0xF4, "suadis"},
}

func TestIE64Dis_FPSinglePrecision(t *testing.T) {
	tests := []struct {
		name   string
		opcode byte
		instr  []byte
		want   string
	}{
		{"FMOV", 0x60, encodeInstr(0x60, 1, 0, 0, 2, 0, 0), "fmov f1, f2"},
		{"FLOAD", 0x61, encodeInstr(0x61, 1, 0, 0, 2, 0, 16), "fload f1, 16(r2)"},
		{"FSTORE", 0x62, encodeInstr(0x62, 1, 0, 0, 2, 0, neg32(-8)), "fstore f1, -8(r2)"},
		{"FCMP", 0x6C, encodeInstr(0x6C, 3, 0, 0, 1, 2, 0), "fcmp r3, f1, f2"},
		{"FCVTIF", 0x6D, encodeInstr(0x6D, 1, 0, 0, 2, 0, 0), "fcvtif f1, r2"},
		{"FCVTFI", 0x6E, encodeInstr(0x6E, 2, 0, 0, 1, 0, 0), "fcvtfi r2, f1"},
		{"FMOVI", 0x6F, encodeInstr(0x6F, 1, 0, 0, 2, 0, 0), "fmovi f1, r2"},
		{"FMOVO", 0x70, encodeInstr(0x70, 2, 0, 0, 1, 0, 0), "fmovo r2, f1"},
		{"FMOVECR", 0x78, encodeInstr(0x78, 1, 0, 0, 0, 0, 5), "fmovecr f1, #5"},
		{"FMOVSR", 0x79, encodeInstr(0x79, 3, 0, 0, 0, 0, 0), "fmovsr r3"},
		{"FMOVCR", 0x7A, encodeInstr(0x7A, 3, 0, 0, 0, 0, 0), "fmovcr r3"},
		{"FMOVSC", 0x7B, encodeInstr(0x7B, 0, 0, 0, 3, 0, 0), "fmovsc r3"},
		{"FMOVCC", 0x7C, encodeInstr(0x7C, 0, 0, 0, 3, 0, 0), "fmovcc r3"},
	}
	for _, row := range []struct {
		opcode byte
		want   string
	}{{0x63, "fadd f0, f1, f2"}, {0x64, "fsub f0, f1, f2"}, {0x65, "fmul f0, f1, f2"}, {0x66, "fdiv f0, f1, f2"}, {0x67, "fmod f0, f1, f2"}, {0x77, "fpow f0, f1, f2"}} {
		tests = append(tests, struct {
			name   string
			opcode byte
			instr  []byte
			want   string
		}{row.want, row.opcode, encodeInstr(row.opcode, 0, 0, 0, 1, 2, 0), row.want})
	}
	for _, row := range []struct {
		opcode byte
		want   string
	}{{0x68, "fabs f0, f1"}, {0x69, "fneg f0, f1"}, {0x6A, "fsqrt f0, f1"}, {0x6B, "fint f0, f1"}, {0x71, "fsin f0, f1"}, {0x72, "fcos f0, f1"}, {0x73, "ftan f0, f1"}, {0x74, "fatan f0, f1"}, {0x75, "flog f0, f1"}, {0x76, "fexp f0, f1"}} {
		tests = append(tests, struct {
			name   string
			opcode byte
			instr  []byte
			want   string
		}{row.want, row.opcode, encodeInstr(row.opcode, 0, 0, 0, 1, 0, 0), row.want})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decode(tt.instr, 0x1000)
			_, asm := FormatInstruction(d)
			if asm != tt.want {
				t.Fatalf("opcode 0x%02X got %q, want %q", tt.opcode, asm, tt.want)
			}
		})
	}
}

func TestIE64Dis_BranchAbove4GiB(t *testing.T) {
	d := Decode(encodeInstr(dis64_BRA, 0, 0, 0, 0, 0, 8), 0x100001000)
	_, asm := FormatInstruction(d)
	if asm != "bra $100001008" {
		t.Fatalf("got %q", asm)
	}
}

func TestIE64Dis_UnknownPreservesAllBytes(t *testing.T) {
	d := Decode([]byte{0xCC, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}, 0x1000)
	_, asm := FormatInstruction(d)
	want := "dc.b $CC, $11, $22, $33, $44, $55, $66, $77  ; unknown opcode"
	if asm != want {
		t.Fatalf("got %q, want %q", asm, want)
	}
}

func TestIE64Dis_MoveqDecimalNegative(t *testing.T) {
	d := Decode(encodeInstr(dis64_MOVEQ, 1, 0, 0, 0, 0, neg32(-8)), 0x1000)
	_, asm := FormatInstruction(d)
	if !strings.Contains(asm, "#-8") || strings.Contains(asm, "FFFFFFF8") {
		t.Fatalf("unexpected moveq rendering: %q", asm)
	}
}

func TestIE64Dis_DLOADNegativeDisp(t *testing.T) {
	d := Decode(encodeInstr(dis64_DLOAD, 1, 0, 0, 2, 0, neg32(-16)), 0x1000)
	_, asm := FormatInstruction(d)
	if asm != "dload f1, -16(r2)" {
		t.Fatalf("got %q", asm)
	}
}

func TestIE64Dis_TrailingBytesDollar(t *testing.T) {
	lines := Disassemble([]byte{0xAB, 0xCD, 0xEF}, 0x1000)
	if len(lines) != 1 || !strings.Contains(lines[0], "dc.b $AB, $CD, $EF") {
		t.Fatalf("unexpected trailing byte output: %#v", lines)
	}
}

func TestIE64Dis_CLI_RejectsMultipleFiles(t *testing.T) {
	_, _, err := parseArgs([]string{"first.ie64", "second.ie64"})
	if err == nil || !strings.Contains(err.Error(), "multiple input files") {
		t.Fatalf("expected multiple-file error, got %v", err)
	}
}

func TestIE64Dis_DecodeShortInput(t *testing.T) {
	d := Decode([]byte{dis64_NOP}, 0x1000)
	if d.Opcode != 0 || d.PC != 0x1000 {
		t.Fatalf("short decode = %#v", d)
	}
}

func TestIE64Dis_DocumentedOpcodes(t *testing.T) {
	for _, row := range documentedOpcodes {
		t.Run(row.cpuName, func(t *testing.T) {
			d := Decode(standaloneInstrForOpcode(row.opcode), 0x1000)
			_, asm := FormatInstruction(d)
			if !strings.HasPrefix(asm, row.mnemonic) {
				t.Fatalf("opcode 0x%02X got %q, want prefix %q", row.opcode, asm, row.mnemonic)
			}
			if strings.Contains(asm, "unknown") || strings.Contains(asm, "???") {
				t.Fatalf("opcode 0x%02X rendered invalid output: %q", row.opcode, asm)
			}
		})
	}

	known := map[byte]bool{}
	for _, row := range documentedOpcodes {
		known[row.opcode] = true
	}
	for op := 0x00; op <= 0xF4; op++ {
		if known[byte(op)] {
			continue
		}
		d := Decode(encodeInstr(byte(op), 1, 0, 0, 2, 3, 4), 0x1000)
		_, asm := FormatInstruction(d)
		if !strings.Contains(asm, "unknown opcode") {
			t.Fatalf("opcode hole 0x%02X got %q", op, asm)
		}
	}
}

func TestIE64Dis_OpcodesMatchSource(t *testing.T) {
	sourceOps := parseIE64OpcodeConstBlock(t, "../cpu_ie64.go")
	rows := map[string]byte{}
	for _, row := range documentedOpcodes {
		rows[row.cpuName] = row.opcode
	}
	for _, op := range sourceOps {
		value, ok := rows[op.name]
		if !ok {
			t.Fatalf("%s=0x%02X missing from documentedOpcodes", op.name, op.value)
		}
		if value != op.value {
			t.Fatalf("%s documented as 0x%02X, cpu source has 0x%02X", op.name, value, op.value)
		}
	}
}

func standaloneInstrForOpcode(op byte) []byte {
	switch {
	case op == dis64_MOVE:
		return encodeInstr(op, 1, 3, 0, 2, 0, 0)
	case op == dis64_MOVT || op == dis64_MOVEQ:
		return encodeInstr(op, 1, 0, 0, 0, 0, 0x42)
	case op == dis64_LEA:
		return encodeInstr(op, 1, 0, 0, 2, 0, 8)
	case op == dis64_LOAD || op == dis64_STORE:
		return encodeInstr(op, 1, 2, 0, 2, 0, 4)
	case isALU3(op):
		return encodeInstr(op, 1, 2, 0, 2, 3, 0)
	case isUnaryALU(op):
		return encodeInstr(op, 1, 2, 0, 2, 0, 0)
	case op == dis64_BRA || op == dis64_JSR:
		return encodeInstr(op, 0, 0, 0, 0, 0, 0x10)
	case isConditionalBranch(op):
		return encodeInstr(op, 0, 0, 0, 1, 2, 0x10)
	case op == dis64_JMP || op == dis64_JSRI:
		return encodeInstr(op, 0, 0, 0, 5, 0, 0)
	case op == dis64_PUSH:
		return encodeInstr(op, 0, 0, 0, 5, 0, 0)
	case op == dis64_POP:
		return encodeInstr(op, 5, 0, 0, 0, 0, 0)
	case op == dis64_WAIT:
		return encodeInstr(op, 0, 0, 0, 0, 0, 500)
	case op == dis64_MTCR:
		return encodeInstr(op, 1, 0, 0, 2, 0, 0)
	case op == dis64_MFCR:
		return encodeInstr(op, 1, 0, 0, 2, 0, 0)
	case op == dis64_TLBINVAL:
		return encodeInstr(op, 0, 0, 0, 2, 0, 0)
	case op == dis64_SYSCALL:
		return encodeInstr(op, 0, 0, 0, 0, 0, 42)
	case op == dis64_SMODE:
		return encodeInstr(op, 1, 0, 0, 0, 0, 0)
	case op >= dis64_FMOV && op <= dis64_FMOVCC:
		return standaloneFPUInstrForOpcode(op)
	case op >= dis64_DMOV && op <= dis64_FCVTDS:
		return standaloneFPUInstrForOpcode(op)
	case op >= dis64_CAS && op <= dis64_FXOR:
		return encodeInstr(op, 2, 0, 0, 1, 3, 0)
	default:
		return encodeInstr(op, 0, 0, 0, 0, 0, 0)
	}
}

func standaloneFPUInstrForOpcode(op byte) []byte {
	switch op {
	case dis64_FLOAD, dis64_FSTORE, dis64_DLOAD, dis64_DSTORE:
		return encodeInstr(op, 1, 0, 0, 2, 0, 4)
	case dis64_FADD, dis64_FSUB, dis64_FMUL, dis64_FDIV, dis64_FMOD, dis64_FPOW,
		dis64_DADD, dis64_DSUB, dis64_DMUL, dis64_DDIV, dis64_DMOD:
		return encodeInstr(op, 1, 0, 0, 2, 3, 0)
	case dis64_FCMP, dis64_DCMP:
		return encodeInstr(op, 1, 0, 0, 2, 3, 0)
	case dis64_FCVTIF, dis64_DCVTIF:
		return encodeInstr(op, 1, 0, 0, 2, 0, 0)
	case dis64_FCVTFI, dis64_DCVTFI:
		return encodeInstr(op, 1, 0, 0, 2, 0, 0)
	case dis64_FMOVECR:
		return encodeInstr(op, 1, 0, 0, 0, 0, 5)
	case dis64_FMOVSR, dis64_FMOVCR:
		return encodeInstr(op, 1, 0, 0, 0, 0, 0)
	case dis64_FMOVSC, dis64_FMOVCC:
		return encodeInstr(op, 0, 0, 0, 1, 0, 0)
	default:
		return encodeInstr(op, 1, 0, 0, 2, 0, 0)
	}
}

type parsedOpcode struct {
	name  string
	value byte
}

func parseIE64OpcodeConstBlock(t *testing.T, path string) []parsedOpcode {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		hasOPMove := false
		for _, spec := range gen.Specs {
			valueSpec := spec.(*ast.ValueSpec)
			for _, name := range valueSpec.Names {
				if name.Name == "OP_MOVE" {
					hasOPMove = true
				}
			}
		}
		if !hasOPMove {
			continue
		}
		var ops []parsedOpcode
		for _, spec := range gen.Specs {
			valueSpec := spec.(*ast.ValueSpec)
			for i, name := range valueSpec.Names {
				if !strings.HasPrefix(name.Name, "OP_") {
					continue
				}
				if i >= len(valueSpec.Values) {
					t.Fatalf("%s has implicit opcode value; parser helper expects explicit values", name.Name)
				}
				lit, ok := valueSpec.Values[i].(*ast.BasicLit)
				if !ok {
					t.Fatalf("%s has non-literal opcode value", name.Name)
				}
				value, err := strconv.ParseUint(lit.Value, 0, 8)
				if err != nil {
					t.Fatalf("%s has invalid opcode value %s: %v", name.Name, lit.Value, err)
				}
				ops = append(ops, parsedOpcode{name: name.Name, value: byte(value)})
			}
		}
		return ops
	}
	t.Fatalf("instruction opcode const block not found in %s", path)
	return nil
}
