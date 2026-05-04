package main

import (
	"encoding/binary"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

type monRow struct {
	cpuName    string
	specOpcode byte
	cpuValue   byte
	mnemonic   string
}

// If you add an opcode to cpu_ie64.go, append it to documentedOpcodes and
// monDocumentedOpcodes and ensure both disassemblers render it.
var monDocumentedOpcodes = []monRow{
	{"OP_MOVE", 0x01, OP_MOVE, "move"}, {"OP_MOVT", 0x02, OP_MOVT, "movt"}, {"OP_MOVEQ", 0x03, OP_MOVEQ, "moveq"}, {"OP_LEA", 0x04, OP_LEA, "lea"},
	{"OP_LOAD", 0x10, OP_LOAD, "load"}, {"OP_STORE", 0x11, OP_STORE, "store"},
	{"OP_ADD", 0x20, OP_ADD, "add"}, {"OP_SUB", 0x21, OP_SUB, "sub"}, {"OP_MULU", 0x22, OP_MULU, "mulu"}, {"OP_MULS", 0x23, OP_MULS, "muls"},
	{"OP_DIVU", 0x24, OP_DIVU, "divu"}, {"OP_DIVS", 0x25, OP_DIVS, "divs"}, {"OP_MOD64", 0x26, OP_MOD64, "mod"}, {"OP_NEG", 0x27, OP_NEG, "neg"},
	{"OP_MODS", 0x28, OP_MODS, "mods"}, {"OP_MULHU", 0x29, OP_MULHU, "mulhu"}, {"OP_MULHS", 0x2A, OP_MULHS, "mulhs"},
	{"OP_AND64", 0x30, OP_AND64, "and"}, {"OP_OR64", 0x31, OP_OR64, "or"}, {"OP_EOR", 0x32, OP_EOR, "eor"}, {"OP_NOT64", 0x33, OP_NOT64, "not"},
	{"OP_LSL", 0x34, OP_LSL, "lsl"}, {"OP_LSR", 0x35, OP_LSR, "lsr"}, {"OP_ASR", 0x36, OP_ASR, "asr"}, {"OP_CLZ", 0x37, OP_CLZ, "clz"},
	{"OP_SEXT", 0x38, OP_SEXT, "sext"}, {"OP_ROL", 0x39, OP_ROL, "rol"}, {"OP_ROR", 0x3A, OP_ROR, "ror"}, {"OP_CTZ", 0x3B, OP_CTZ, "ctz"},
	{"OP_POPCNT", 0x3C, OP_POPCNT, "popcnt"}, {"OP_BSWAP", 0x3D, OP_BSWAP, "bswap"},
	{"OP_BRA", 0x40, OP_BRA, "bra"}, {"OP_BEQ", 0x41, OP_BEQ, "beq"}, {"OP_BNE", 0x42, OP_BNE, "bne"}, {"OP_BLT", 0x43, OP_BLT, "blt"},
	{"OP_BGE", 0x44, OP_BGE, "bge"}, {"OP_BGT", 0x45, OP_BGT, "bgt"}, {"OP_BLE", 0x46, OP_BLE, "ble"}, {"OP_BHI", 0x47, OP_BHI, "bhi"},
	{"OP_BLS", 0x48, OP_BLS, "bls"}, {"OP_JMP", 0x49, OP_JMP, "jmp"},
	{"OP_JSR64", 0x50, OP_JSR64, "jsr"}, {"OP_RTS64", 0x51, OP_RTS64, "rts"}, {"OP_PUSH64", 0x52, OP_PUSH64, "push"}, {"OP_POP64", 0x53, OP_POP64, "pop"},
	{"OP_JSR_IND", 0x54, OP_JSR_IND, "jsr"},
	{"OP_FMOV", 0x60, OP_FMOV, "fmov"}, {"OP_FLOAD", 0x61, OP_FLOAD, "fload"}, {"OP_FSTORE", 0x62, OP_FSTORE, "fstore"},
	{"OP_FADD", 0x63, OP_FADD, "fadd"}, {"OP_FSUB", 0x64, OP_FSUB, "fsub"}, {"OP_FMUL", 0x65, OP_FMUL, "fmul"}, {"OP_FDIV", 0x66, OP_FDIV, "fdiv"},
	{"OP_FMOD", 0x67, OP_FMOD, "fmod"}, {"OP_FABS", 0x68, OP_FABS, "fabs"}, {"OP_FNEG", 0x69, OP_FNEG, "fneg"}, {"OP_FSQRT", 0x6A, OP_FSQRT, "fsqrt"},
	{"OP_FINT", 0x6B, OP_FINT, "fint"}, {"OP_FCMP", 0x6C, OP_FCMP, "fcmp"}, {"OP_FCVTIF", 0x6D, OP_FCVTIF, "fcvtif"}, {"OP_FCVTFI", 0x6E, OP_FCVTFI, "fcvtfi"},
	{"OP_FMOVI", 0x6F, OP_FMOVI, "fmovi"}, {"OP_FMOVO", 0x70, OP_FMOVO, "fmovo"}, {"OP_FSIN", 0x71, OP_FSIN, "fsin"}, {"OP_FCOS", 0x72, OP_FCOS, "fcos"},
	{"OP_FTAN", 0x73, OP_FTAN, "ftan"}, {"OP_FATAN", 0x74, OP_FATAN, "fatan"}, {"OP_FLOG", 0x75, OP_FLOG, "flog"}, {"OP_FEXP", 0x76, OP_FEXP, "fexp"},
	{"OP_FPOW", 0x77, OP_FPOW, "fpow"}, {"OP_FMOVECR", 0x78, OP_FMOVECR, "fmovecr"}, {"OP_FMOVSR", 0x79, OP_FMOVSR, "fmovsr"}, {"OP_FMOVCR", 0x7A, OP_FMOVCR, "fmovcr"},
	{"OP_FMOVSC", 0x7B, OP_FMOVSC, "fmovsc"}, {"OP_FMOVCC", 0x7C, OP_FMOVCC, "fmovcc"},
	{"OP_DMOV", 0x80, OP_DMOV, "dmov"}, {"OP_DLOAD", 0x81, OP_DLOAD, "dload"}, {"OP_DSTORE", 0x82, OP_DSTORE, "dstore"},
	{"OP_DADD", 0x83, OP_DADD, "dadd"}, {"OP_DSUB", 0x84, OP_DSUB, "dsub"}, {"OP_DMUL", 0x85, OP_DMUL, "dmul"}, {"OP_DDIV", 0x86, OP_DDIV, "ddiv"},
	{"OP_DMOD", 0x87, OP_DMOD, "dmod"}, {"OP_DABS", 0x88, OP_DABS, "dabs"}, {"OP_DNEG", 0x89, OP_DNEG, "dneg"}, {"OP_DSQRT", 0x8A, OP_DSQRT, "dsqrt"},
	{"OP_DINT", 0x8B, OP_DINT, "dint"}, {"OP_DCMP", 0x8C, OP_DCMP, "dcmp"}, {"OP_DCVTIF", 0x8D, OP_DCVTIF, "dcvtif"}, {"OP_DCVTFI", 0x8E, OP_DCVTFI, "dcvtfi"},
	{"OP_FCVTSD", 0x8F, OP_FCVTSD, "fcvtsd"}, {"OP_FCVTDS", 0x90, OP_FCVTDS, "fcvtds"},
	{"OP_NOP64", 0xE0, OP_NOP64, "nop"}, {"OP_HALT64", 0xE1, OP_HALT64, "halt"}, {"OP_SEI64", 0xE2, OP_SEI64, "sei"}, {"OP_CLI64", 0xE3, OP_CLI64, "cli"},
	{"OP_RTI64", 0xE4, OP_RTI64, "rti"}, {"OP_WAIT64", 0xE5, OP_WAIT64, "wait"}, {"OP_MTCR", 0xE6, OP_MTCR, "mtcr"}, {"OP_MFCR", 0xE7, OP_MFCR, "mfcr"},
	{"OP_ERET", 0xE8, OP_ERET, "eret"}, {"OP_TLBFLUSH", 0xE9, OP_TLBFLUSH, "tlbflush"}, {"OP_TLBINVAL", 0xEA, OP_TLBINVAL, "tlbinval"}, {"OP_SYSCALL", 0xEB, OP_SYSCALL, "syscall"},
	{"OP_SMODE", 0xEC, OP_SMODE, "smode"}, {"OP_CAS", 0xED, OP_CAS, "cas"}, {"OP_XCHG", 0xEE, OP_XCHG, "xchg"}, {"OP_FAA", 0xEF, OP_FAA, "faa"},
	{"OP_FAND", 0xF0, OP_FAND, "fand"}, {"OP_FOR", 0xF1, OP_FOR, "for"}, {"OP_FXOR", 0xF2, OP_FXOR, "fxor"}, {"OP_SUAEN", 0xF3, OP_SUAEN, "suaen"},
	{"OP_SUADIS", 0xF4, OP_SUADIS, "suadis"},
}

func TestIE64Disassemble_HighPC(t *testing.T) {
	d := ie64Decode(debugIE64Instr(OP_BRA, 0, 0, 0, 0, 0, 8), 0x100001000)
	_, asm := ie64FormatInstruction(d)
	if asm != "bra $100001008" {
		t.Fatalf("got %q", asm)
	}

	lines := disassembleIE64(func(addr uint64, size int) []byte {
		if addr != 0x100001000 || size != 8 {
			return nil
		}
		return debugIE64Instr(OP_BRA, 0, 0, 0, 0, 0, 8)
	}, 0x100001000, 1)
	if len(lines) != 1 || lines[0].BranchTarget != 0x100001008 {
		t.Fatalf("unexpected branch annotation: %#v", lines)
	}
}

func TestIE64Disassemble_FCMP(t *testing.T) {
	d := ie64Decode(debugIE64Instr(OP_FCMP, 3, 0, 0, 1, 2, 0), 0x1000)
	_, asm := ie64FormatInstruction(d)
	if asm != "fcmp r3, f1, f2" {
		t.Fatalf("got %q", asm)
	}
}

func TestDebugIE64Dis_UnknownPreservesAllBytes(t *testing.T) {
	d := ie64Decode([]byte{0xCC, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}, 0x1000)
	_, asm := ie64FormatInstruction(d)
	want := "dc.b $CC, $11, $22, $33, $44, $55, $66, $77  ; unknown opcode"
	if asm != want {
		t.Fatalf("got %q, want %q", asm, want)
	}
}

func TestDebugIE64Dis_DocumentedOpcodes(t *testing.T) {
	for _, row := range monDocumentedOpcodes {
		t.Run(row.cpuName, func(t *testing.T) {
			if row.cpuValue != row.specOpcode {
				t.Fatalf("%s CPU value 0x%02X, want spec 0x%02X", row.cpuName, row.cpuValue, row.specOpcode)
			}
			d := ie64Decode(debugInstrForOpcode(row.cpuValue), 0x1000)
			_, asm := ie64FormatInstruction(d)
			if !strings.HasPrefix(asm, row.mnemonic) {
				t.Fatalf("opcode 0x%02X got %q, want prefix %q", row.cpuValue, asm, row.mnemonic)
			}
			if strings.Contains(asm, "unknown") || strings.Contains(asm, "???") {
				t.Fatalf("opcode 0x%02X rendered invalid output: %q", row.cpuValue, asm)
			}
		})
	}
}

func TestDebugIE64Dis_OpcodesMatchSource(t *testing.T) {
	sourceOps := parseDebugIE64OpcodeConstBlock(t, "cpu_ie64.go")
	rows := map[string]byte{}
	for _, row := range monDocumentedOpcodes {
		rows[row.cpuName] = row.cpuValue
	}
	for _, op := range sourceOps {
		value, ok := rows[op.name]
		if !ok {
			t.Fatalf("%s=0x%02X missing from monDocumentedOpcodes", op.name, op.value)
		}
		if value != op.value {
			t.Fatalf("%s documented as 0x%02X, cpu source has 0x%02X", op.name, value, op.value)
		}
	}
}

func debugIE64Instr(opcode byte, rd, size, xbit, rs, rt byte, imm32 uint32) []byte {
	instr := make([]byte, 8)
	instr[0] = opcode
	instr[1] = (rd << 3) | (size << 1) | xbit
	instr[2] = rs << 3
	instr[3] = rt << 3
	binary.LittleEndian.PutUint32(instr[4:], imm32)
	return instr
}

func debugInstrForOpcode(op byte) []byte {
	switch {
	case op == OP_MOVE:
		return debugIE64Instr(op, 1, 3, 0, 2, 0, 0)
	case op == OP_MOVT || op == OP_MOVEQ:
		return debugIE64Instr(op, 1, 0, 0, 0, 0, 0x42)
	case op == OP_LEA:
		return debugIE64Instr(op, 1, 0, 0, 2, 0, 8)
	case op == OP_LOAD || op == OP_STORE:
		return debugIE64Instr(op, 1, 2, 0, 2, 0, 4)
	case ie64IsALU3(op):
		return debugIE64Instr(op, 1, 2, 0, 2, 3, 0)
	case ie64IsUnaryALU(op):
		return debugIE64Instr(op, 1, 2, 0, 2, 0, 0)
	case op == OP_BRA || op == OP_JSR64:
		return debugIE64Instr(op, 0, 0, 0, 0, 0, 0x10)
	case ie64IsConditionalBranch(op):
		return debugIE64Instr(op, 0, 0, 0, 1, 2, 0x10)
	case op == OP_JMP || op == OP_JSR_IND:
		return debugIE64Instr(op, 0, 0, 0, 5, 0, 0)
	case op == OP_PUSH64:
		return debugIE64Instr(op, 0, 0, 0, 5, 0, 0)
	case op == OP_POP64:
		return debugIE64Instr(op, 5, 0, 0, 0, 0, 0)
	case op == OP_WAIT64:
		return debugIE64Instr(op, 0, 0, 0, 0, 0, 500)
	case op == OP_MTCR:
		return debugIE64Instr(op, 1, 0, 0, 2, 0, 0)
	case op == OP_MFCR:
		return debugIE64Instr(op, 1, 0, 0, 2, 0, 0)
	case op == OP_TLBINVAL:
		return debugIE64Instr(op, 0, 0, 0, 2, 0, 0)
	case op == OP_SYSCALL:
		return debugIE64Instr(op, 0, 0, 0, 0, 0, 42)
	case op == OP_SMODE:
		return debugIE64Instr(op, 1, 0, 0, 0, 0, 0)
	case op >= OP_FMOV && op <= OP_FMOVCC:
		return debugFPUInstrForOpcode(op)
	case op >= OP_DMOV && op <= OP_FCVTDS:
		return debugFPUInstrForOpcode(op)
	case op >= OP_CAS && op <= OP_FXOR:
		return debugIE64Instr(op, 2, 0, 0, 1, 3, 0)
	default:
		return debugIE64Instr(op, 0, 0, 0, 0, 0, 0)
	}
}

func debugFPUInstrForOpcode(op byte) []byte {
	switch op {
	case OP_FLOAD, OP_FSTORE, OP_DLOAD, OP_DSTORE:
		return debugIE64Instr(op, 1, 0, 0, 2, 0, 4)
	case OP_FADD, OP_FSUB, OP_FMUL, OP_FDIV, OP_FMOD, OP_FPOW,
		OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV, OP_DMOD:
		return debugIE64Instr(op, 1, 0, 0, 2, 3, 0)
	case OP_FCMP, OP_DCMP:
		return debugIE64Instr(op, 1, 0, 0, 2, 3, 0)
	case OP_FMOVECR:
		return debugIE64Instr(op, 1, 0, 0, 0, 0, 5)
	case OP_FMOVSR, OP_FMOVCR:
		return debugIE64Instr(op, 1, 0, 0, 0, 0, 0)
	case OP_FMOVSC, OP_FMOVCC:
		return debugIE64Instr(op, 0, 0, 0, 1, 0, 0)
	default:
		return debugIE64Instr(op, 1, 0, 0, 2, 0, 0)
	}
}

type debugParsedOpcode struct {
	name  string
	value byte
}

func parseDebugIE64OpcodeConstBlock(t *testing.T, path string) []debugParsedOpcode {
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
		var ops []debugParsedOpcode
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
				ops = append(ops, debugParsedOpcode{name: name.Name, value: byte(value)})
			}
		}
		return ops
	}
	t.Fatalf("instruction opcode const block not found in %s", path)
	return nil
}
