package ie64

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	opMove     = 0x01
	opMovt     = 0x02
	opMoveq    = 0x03
	opLea      = 0x04
	opLoad     = 0x10
	opStore    = 0x11
	opAdd      = 0x20
	opSub      = 0x21
	opMulu     = 0x22
	opMuls     = 0x23
	opDivu     = 0x24
	opDivs     = 0x25
	opMod      = 0x26
	opNeg      = 0x27
	opMods     = 0x28
	opMulhu    = 0x29
	opMulhs    = 0x2A
	opAnd      = 0x30
	opOr       = 0x31
	opEor      = 0x32
	opNot      = 0x33
	opLsl      = 0x34
	opLsr      = 0x35
	opAsr      = 0x36
	opClz      = 0x37
	opSext     = 0x38
	opRol      = 0x39
	opRor      = 0x3A
	opCtz      = 0x3B
	opPopcnt   = 0x3C
	opBswap    = 0x3D
	opBra      = 0x40
	opBeq      = 0x41
	opBne      = 0x42
	opBlt      = 0x43
	opBge      = 0x44
	opBgt      = 0x45
	opBle      = 0x46
	opBhi      = 0x47
	opBls      = 0x48
	opJmp      = 0x49
	opJsr      = 0x50
	opRts      = 0x51
	opPush     = 0x52
	opPop      = 0x53
	opJsrInd   = 0x54
	opFmov     = 0x60
	opFload    = 0x61
	opFstore   = 0x62
	opFadd     = 0x63
	opFsub     = 0x64
	opFmul     = 0x65
	opFdiv     = 0x66
	opFmod     = 0x67
	opFabs     = 0x68
	opFneg     = 0x69
	opFsqrt    = 0x6A
	opFint     = 0x6B
	opFcmp     = 0x6C
	opFcvtif   = 0x6D
	opFcvtfi   = 0x6E
	opFmovi    = 0x6F
	opFmovo    = 0x70
	opFsin     = 0x71
	opFcos     = 0x72
	opFtan     = 0x73
	opFatan    = 0x74
	opFlog     = 0x75
	opFexp     = 0x76
	opFpow     = 0x77
	opFmovecr  = 0x78
	opFmovsr   = 0x79
	opFmovcr   = 0x7A
	opFmovsc   = 0x7B
	opFmovcc   = 0x7C
	opDmov     = 0x80
	opDload    = 0x81
	opDstore   = 0x82
	opDadd     = 0x83
	opDsub     = 0x84
	opDmul     = 0x85
	opDdiv     = 0x86
	opDmod     = 0x87
	opDabs     = 0x88
	opDneg     = 0x89
	opDsqrt    = 0x8A
	opDint     = 0x8B
	opDcmp     = 0x8C
	opDcvtif   = 0x8D
	opDcvtfi   = 0x8E
	opFcvtsd   = 0x8F
	opFcvtds   = 0x90
	opDsin     = 0x91
	opDcos     = 0x92
	opDtan     = 0x93
	opDatan    = 0x94
	opDlog     = 0x95
	opDexp     = 0x96
	opDpow     = 0x97
	opNop      = 0xE0
	opHalt     = 0xE1
	opSei      = 0xE2
	opCli      = 0xE3
	opRti      = 0xE4
	opWait     = 0xE5
	opMtcr     = 0xE6
	opMfcr     = 0xE7
	opEret     = 0xE8
	opTlbflush = 0xE9
	opTlbinval = 0xEA
	opSyscall  = 0xEB
	opSmode    = 0xEC
	opCas      = 0xED
	opXchg     = 0xEE
	opFaa      = 0xEF
	opFand     = 0xF0
	opFor      = 0xF1
	opFxor     = 0xF2
	opSuaen    = 0xF3
	opSuadis   = 0xF4
)

const (
	sizeB = 0
	sizeW = 1
	sizeL = 2
	sizeQ = 3
)

// Diagnostic describes a single monitor-assembler error.
type Diagnostic struct {
	Message string
	Column  int
	Code    string
}

// InstructionResult is the result of assembling one IE64 instruction.
type InstructionResult struct {
	Origin      uint64
	Bytes       []byte
	Diagnostics []Diagnostic
}

type oneLineAssembler struct{}

// AssembleInstruction assembles exactly one IE64 instruction at origin.
// It intentionally has no source-file, include, label, directive, macro, or
// multi-line state. Diagnostics are deterministic and monitor-oriented.
func AssembleInstruction(origin uint64, line string) InstructionResult {
	res := InstructionResult{Origin: origin}
	clean, diag := sanitizeMonitorLine(line)
	if diag != nil {
		res.Diagnostics = append(res.Diagnostics, *diag)
		return res
	}
	if clean == "" {
		res.Diagnostics = append(res.Diagnostics, diagnostic("no instruction assembled", 1, "zero"))
		return res
	}
	a := oneLineAssembler{}
	bytes, err := a.assemble(origin, clean)
	if err != nil {
		res.Diagnostics = append(res.Diagnostics, diagnostic(err.Error(), mnemonicColumn(line), "asm"))
		return res
	}
	if len(bytes) != 8 {
		res.Diagnostics = append(res.Diagnostics, diagnostic("no instruction assembled", 1, "size"))
		return res
	}
	res.Bytes = bytes
	return res
}

func diagnostic(message string, column int, code string) Diagnostic {
	if column < 1 {
		column = 1
	}
	return Diagnostic{Message: message, Column: column, Code: code}
}

func mnemonicColumn(line string) int {
	for i, r := range line {
		if r != ' ' && r != '\t' {
			return i + 1
		}
	}
	return 1
}

func sanitizeMonitorLine(line string) (string, *Diagnostic) {
	raw := strings.TrimSpace(line)
	if raw == "" {
		return "", nil
	}
	if strings.HasPrefix(raw, ";") || strings.HasPrefix(raw, "//") {
		return "", nil
	}
	if idx := strings.Index(raw, ";"); idx >= 0 {
		tail := strings.TrimSpace(raw[idx+1:])
		if tail != "" && looksLikeInstructionStart(tail) {
			return "", ptrDiagnostic("multiple instructions on one line are not supported", idx+2, "multi")
		}
		raw = strings.TrimSpace(raw[:idx])
	}
	if idx := strings.Index(raw, "//"); idx >= 0 {
		tail := strings.TrimSpace(raw[idx+2:])
		if tail != "" && looksLikeInstructionStart(tail) {
			return "", ptrDiagnostic("multiple instructions on one line are not supported", idx+3, "multi")
		}
		raw = strings.TrimSpace(raw[:idx])
	}
	if raw == "" {
		return "", nil
	}
	if strings.Contains(raw, ":") {
		return "", ptrDiagnostic("labels are not supported in monitor assemble mode", strings.Index(raw, ":")+1, "label")
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "", nil
	}
	first := directiveKey(fields[0])
	if first == "include" {
		return "", ptrDiagnostic("include is not supported in monitor assemble mode", 1, "include")
	}
	if first == "incbin" {
		return "", ptrDiagnostic("incbin is not supported in monitor assemble mode", 1, "incbin")
	}
	if isMonitorDirective(first) || (len(fields) >= 2 && isEquSetMacroDirective(directiveKey(fields[1]))) {
		return "", ptrDiagnostic("directives are not supported in monitor assemble mode", 1, "directive")
	}
	if first == "li" {
		return "", ptrDiagnostic("pseudo-instruction expands to more than one instruction", 1, "pseudo")
	}
	return raw, nil
}

func ptrDiagnostic(message string, column int, code string) *Diagnostic {
	d := diagnostic(message, column, code)
	return &d
}

func directiveKey(tok string) string {
	tok = strings.ToLower(strings.TrimSpace(tok))
	tok = strings.TrimPrefix(tok, ".")
	return tok
}

func isEquSetMacroDirective(tok string) bool {
	switch tok {
	case "equ", "set", "macro":
		return true
	default:
		return false
	}
}

func isMonitorDirective(tok string) bool {
	switch tok {
	case "org", "align", "equ", "set", "macro", "endm", "rept", "endr", "if", "else", "endif":
		return true
	case "include", "incbin":
		return true
	}
	return strings.HasPrefix(tok, "dc.") ||
		strings.HasPrefix(tok, "ds.") ||
		strings.HasPrefix(tok, "dcb.")
}

func looksLikeInstructionStart(s string) bool {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return false
	}
	base, _ := parseSizeSuffix(strings.ToLower(fields[0]))
	if isMonitorDirective(directiveKey(base)) {
		return true
	}
	if base == "li" {
		return true
	}
	_, ok := mnemonicSet[base]
	return ok
}

var mnemonicSet = map[string]struct{}{
	"move": {}, "movt": {}, "moveq": {}, "lea": {}, "la": {},
	"load": {}, "store": {},
	"add": {}, "sub": {}, "mulu": {}, "muls": {}, "divu": {}, "divs": {}, "mod": {}, "mods": {}, "mulhu": {}, "mulhs": {},
	"neg": {}, "sext": {}, "and": {}, "or": {}, "eor": {}, "not": {}, "lsl": {}, "lsr": {}, "asr": {}, "clz": {},
	"rol": {}, "ror": {}, "ctz": {}, "popcnt": {}, "bswap": {},
	"bra": {}, "beq": {}, "bne": {}, "blt": {}, "bge": {}, "bgt": {}, "ble": {}, "bhi": {}, "bls": {},
	"beqz": {}, "bnez": {}, "bltz": {}, "bgez": {}, "bgtz": {}, "blez": {},
	"jmp": {}, "jsr": {}, "rts": {}, "push": {}, "pop": {},
	"fmov": {}, "fload": {}, "fstore": {}, "fadd": {}, "fsub": {}, "fmul": {}, "fdiv": {}, "fmod": {},
	"fabs": {}, "fneg": {}, "fsqrt": {}, "fint": {}, "fcmp": {}, "fcvtif": {}, "fcvtfi": {}, "fmovi": {}, "fmovo": {},
	"fsin": {}, "fcos": {}, "ftan": {}, "fatan": {}, "flog": {}, "fexp": {}, "fpow": {}, "fmovecr": {},
	"fmovsr": {}, "fmovcr": {}, "fmovsc": {}, "fmovcc": {},
	"dmov": {}, "dload": {}, "dstore": {}, "dadd": {}, "dsub": {}, "dmul": {}, "ddiv": {}, "dmod": {},
	"dabs": {}, "dneg": {}, "dsqrt": {}, "dint": {}, "dcmp": {}, "dcvtif": {}, "dcvtfi": {}, "fcvtsd": {}, "fcvtds": {},
	"dsin": {}, "dcos": {}, "dtan": {}, "datan": {}, "dlog": {}, "dexp": {}, "dpow": {},
	"nop": {}, "halt": {}, "sei": {}, "cli": {}, "rti": {}, "wait": {}, "mtcr": {}, "mfcr": {}, "eret": {},
	"tlbflush": {}, "tlbinval": {}, "syscall": {}, "smode": {}, "suaen": {}, "suadis": {},
	"cas": {}, "xchg": {}, "faa": {}, "fand": {}, "for": {}, "fxor": {},
}

func (a oneLineAssembler) assemble(origin uint64, line string) ([]byte, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil, fmt.Errorf("no instruction assembled")
	}
	mnemonicRaw := strings.ToLower(fields[0])
	base, size := parseSizeSuffix(mnemonicRaw)
	operandStr := strings.TrimSpace(line[len(fields[0]):])
	operands := splitOperands(operandStr)

	switch base {
	case "la":
		if len(operands) != 2 {
			return nil, fmt.Errorf("la requires 2 operands (rd, addr)")
		}
		operands[1] = fmt.Sprintf("%s(r0)", operands[1])
		return a.asmLea(operands)
	case "beqz":
		return a.asmZeroBranch(opBeq, operands, origin)
	case "bnez":
		return a.asmZeroBranch(opBne, operands, origin)
	case "bltz":
		return a.asmZeroBranch(opBlt, operands, origin)
	case "bgez":
		return a.asmZeroBranch(opBge, operands, origin)
	case "bgtz":
		return a.asmZeroBranch(opBgt, operands, origin)
	case "blez":
		return a.asmZeroBranch(opBle, operands, origin)
	case "move":
		return a.asmMove(size, operands)
	case "movt":
		return a.asmImmReg(opMovt, operands, "movt", sizeQ)
	case "moveq":
		return a.asmImmReg(opMoveq, operands, "moveq", sizeQ)
	case "lea":
		return a.asmLea(operands)
	case "load":
		return a.asmLoadStore(opLoad, size, operands)
	case "store":
		return a.asmLoadStore(opStore, size, operands)
	case "add":
		return a.asmALU3(opAdd, size, operands)
	case "sub":
		return a.asmALU3(opSub, size, operands)
	case "mulu":
		return a.asmALU3(opMulu, size, operands)
	case "muls":
		return a.asmALU3(opMuls, size, operands)
	case "divu":
		return a.asmALU3(opDivu, size, operands)
	case "divs":
		return a.asmALU3(opDivs, size, operands)
	case "mod":
		return a.asmALU3(opMod, size, operands)
	case "mods":
		return a.asmALU3(opMods, size, operands)
	case "mulhu":
		return a.asmQOnlyALU3(opMulhu, mnemonicRaw, size, operands)
	case "mulhs":
		return a.asmQOnlyALU3(opMulhs, mnemonicRaw, size, operands)
	case "neg":
		return a.asmALU2(opNeg, size, operands)
	case "sext":
		return a.asmALU2(opSext, size, operands)
	case "and":
		return a.asmALU3(opAnd, size, operands)
	case "or":
		return a.asmALU3(opOr, size, operands)
	case "eor":
		return a.asmALU3(opEor, size, operands)
	case "not":
		return a.asmALU2(opNot, size, operands)
	case "lsl":
		return a.asmALU3(opLsl, size, operands)
	case "lsr":
		return a.asmALU3(opLsr, size, operands)
	case "asr":
		return a.asmALU3(opAsr, size, operands)
	case "clz":
		return a.asmLOnlyALU2(opClz, mnemonicRaw, size, operands)
	case "rol":
		return a.asmALU3(opRol, size, operands)
	case "ror":
		return a.asmALU3(opRor, size, operands)
	case "ctz":
		return a.asmLOnlyALU2(opCtz, mnemonicRaw, size, operands)
	case "popcnt":
		return a.asmLOnlyALU2(opPopcnt, mnemonicRaw, size, operands)
	case "bswap":
		return a.asmLOnlyALU2(opBswap, mnemonicRaw, size, operands)
	case "bra":
		return a.asmBranch(opBra, operands, origin)
	case "beq":
		return a.asmBcc(opBeq, operands, origin)
	case "bne":
		return a.asmBcc(opBne, operands, origin)
	case "blt":
		return a.asmBcc(opBlt, operands, origin)
	case "bge":
		return a.asmBcc(opBge, operands, origin)
	case "bgt":
		return a.asmBcc(opBgt, operands, origin)
	case "ble":
		return a.asmBcc(opBle, operands, origin)
	case "bhi":
		return a.asmBcc(opBhi, operands, origin)
	case "bls":
		return a.asmBcc(opBls, operands, origin)
	case "jmp":
		return a.asmJmp(operands)
	case "jsr":
		return a.asmJsr(operands, origin)
	case "rts":
		return encode(opRts, 0, 0, 0, 0, 0, 0), nil
	case "push":
		return a.asmPushPop(opPush, operands)
	case "pop":
		return a.asmPushPop(opPop, operands)
	case "nop":
		return encode(opNop, 0, 0, 0, 0, 0, 0), nil
	case "halt":
		return encode(opHalt, 0, 0, 0, 0, 0, 0), nil
	case "sei":
		return encode(opSei, 0, 0, 0, 0, 0, 0), nil
	case "cli":
		return encode(opCli, 0, 0, 0, 0, 0, 0), nil
	case "rti":
		return encode(opRti, 0, 0, 0, 0, 0, 0), nil
	case "wait":
		return a.asmSingleImm(opWait, operands, "wait")
	case "mtcr":
		return a.asmMTCR(operands)
	case "mfcr":
		return a.asmMFCR(operands)
	case "eret":
		return encode(opEret, 0, 0, 0, 0, 0, 0), nil
	case "tlbflush":
		return encode(opTlbflush, 0, 0, 0, 0, 0, 0), nil
	case "tlbinval":
		return a.asmTLBINVAL(operands)
	case "syscall":
		return a.asmSingleImm(opSyscall, operands, "syscall")
	case "smode":
		return a.asmSMODE(operands)
	case "suaen":
		return encode(opSuaen, 0, 0, 0, 0, 0, 0), nil
	case "suadis":
		return encode(opSuadis, 0, 0, 0, 0, 0, 0), nil
	case "cas":
		return a.asmAtomic(opCas, operands)
	case "xchg":
		return a.asmAtomic(opXchg, operands)
	case "faa":
		return a.asmAtomic(opFaa, operands)
	case "fand":
		return a.asmAtomic(opFand, operands)
	case "for":
		return a.asmAtomic(opFor, operands)
	case "fxor":
		return a.asmAtomic(opFxor, operands)
	default:
		return a.assembleFPU(base, mnemonicRaw, operands)
	}
}

func (a oneLineAssembler) assembleFPU(base, mnemonic string, operands []string) ([]byte, error) {
	if strings.Contains(mnemonic, ".") {
		return nil, fmt.Errorf("size suffixes not allowed on FP instruction: %s", mnemonic)
	}
	switch base {
	case "fmov":
		return a.asmFP2(opFmov, operands, true, true)
	case "fload":
		return a.asmFPMem(opFload, operands)
	case "fstore":
		return a.asmFPMem(opFstore, operands)
	case "fadd":
		return a.asmFP3(opFadd, operands)
	case "fsub":
		return a.asmFP3(opFsub, operands)
	case "fmul":
		return a.asmFP3(opFmul, operands)
	case "fdiv":
		return a.asmFP3(opFdiv, operands)
	case "fmod":
		return a.asmFP3(opFmod, operands)
	case "fabs":
		return a.asmFP2(opFabs, operands, true, true)
	case "fneg":
		return a.asmFP2(opFneg, operands, true, true)
	case "fsqrt":
		return a.asmFP2(opFsqrt, operands, true, true)
	case "fint":
		return a.asmFP2(opFint, operands, true, true)
	case "fcmp":
		return a.asmFP3Int(opFcmp, operands)
	case "fcvtif":
		return a.asmFP2(opFcvtif, operands, false, true)
	case "fcvtfi":
		return a.asmFP2(opFcvtfi, operands, true, false)
	case "fmovi":
		return a.asmFP2(opFmovi, operands, false, true)
	case "fmovo":
		return a.asmFP2(opFmovo, operands, true, false)
	case "fsin":
		return a.asmFP2(opFsin, operands, true, true)
	case "fcos":
		return a.asmFP2(opFcos, operands, true, true)
	case "ftan":
		return a.asmFP2(opFtan, operands, true, true)
	case "fatan":
		return a.asmFP2(opFatan, operands, true, true)
	case "flog":
		return a.asmFP2(opFlog, operands, true, true)
	case "fexp":
		return a.asmFP2(opFexp, operands, true, true)
	case "fpow":
		return a.asmFP3(opFpow, operands)
	case "fmovecr":
		return a.asmFPImm(opFmovecr, operands)
	case "fmovsr":
		return a.asmFPStatus(opFmovsr, operands, false)
	case "fmovcr":
		return a.asmFPStatus(opFmovcr, operands, false)
	case "fmovsc":
		return a.asmFPStatus(opFmovsc, operands, true)
	case "fmovcc":
		return a.asmFPStatus(opFmovcc, operands, true)
	case "dmov":
		return a.asmFP2Even(opDmov, operands, true, true)
	case "dload":
		return a.asmFPMemEven(opDload, operands)
	case "dstore":
		return a.asmFPMemEven(opDstore, operands)
	case "dadd":
		return a.asmFP3Even(opDadd, operands)
	case "dsub":
		return a.asmFP3Even(opDsub, operands)
	case "dmul":
		return a.asmFP3Even(opDmul, operands)
	case "ddiv":
		return a.asmFP3Even(opDdiv, operands)
	case "dmod":
		return a.asmFP3Even(opDmod, operands)
	case "dabs":
		return a.asmFP2Even(opDabs, operands, true, true)
	case "dneg":
		return a.asmFP2Even(opDneg, operands, true, true)
	case "dsqrt":
		return a.asmFP2Even(opDsqrt, operands, true, true)
	case "dint":
		return a.asmFP2Even(opDint, operands, true, true)
	case "dcmp":
		return a.asmFP3EvenInt(opDcmp, operands)
	case "dcvtif":
		return a.asmFP2Even(opDcvtif, operands, false, true)
	case "dcvtfi":
		return a.asmFP2Even(opDcvtfi, operands, true, false)
	case "fcvtsd":
		return a.asmFCVTSD(operands)
	case "fcvtds":
		return a.asmFCVTDS(operands)
	case "dsin":
		return a.asmFP2Even(opDsin, operands, true, true)
	case "dcos":
		return a.asmFP2Even(opDcos, operands, true, true)
	case "dtan":
		return a.asmFP2Even(opDtan, operands, true, true)
	case "datan":
		return a.asmFP2Even(opDatan, operands, true, true)
	case "dlog":
		return a.asmFP2Even(opDlog, operands, true, true)
	case "dexp":
		return a.asmFP2Even(opDexp, operands, true, true)
	case "dpow":
		return a.asmFP3Even(opDpow, operands)
	default:
		return nil, fmt.Errorf("unknown instruction: %s", base)
	}
}

func encode(opcode byte, rd, size, xbit, rs, rt byte, imm32 uint32) []byte {
	instr := make([]byte, 8)
	instr[0] = opcode
	instr[1] = (rd << 3) | (size << 1) | xbit
	instr[2] = rs << 3
	instr[3] = rt << 3
	binary.LittleEndian.PutUint32(instr[4:], imm32)
	return instr
}

func parseRegister(name string) (byte, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "sp" {
		return 31, true
	}
	if strings.HasPrefix(name, "r") {
		n, err := strconv.Atoi(name[1:])
		if err == nil && n >= 0 && n <= 31 {
			return byte(n), true
		}
	}
	return 0, false
}

func parseFPRegister(name string) (byte, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(name, "f") {
		n, err := strconv.Atoi(name[1:])
		if err == nil && n >= 0 && n <= 15 {
			return byte(n), true
		}
	}
	return 0, false
}

func parseSizeSuffix(mnemonic string) (string, byte) {
	switch {
	case strings.HasSuffix(mnemonic, ".b"):
		return mnemonic[:len(mnemonic)-2], sizeB
	case strings.HasSuffix(mnemonic, ".w"):
		return mnemonic[:len(mnemonic)-2], sizeW
	case strings.HasSuffix(mnemonic, ".l"):
		return mnemonic[:len(mnemonic)-2], sizeL
	case strings.HasSuffix(mnemonic, ".q"):
		return mnemonic[:len(mnemonic)-2], sizeQ
	default:
		return mnemonic, sizeQ
	}
}

func splitOperands(s string) []string {
	var result []string
	depth := 0
	start := 0
	inString := false
	inChar := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if (inString || inChar) && c == '\\' {
			escaped = true
			continue
		}
		switch c {
		case '"':
			if !inChar {
				inString = !inString
			}
		case '\'':
			if !inString {
				inChar = !inChar
			}
		case '(':
			if !inString && !inChar {
				depth++
			}
		case ')':
			if !inString && !inChar && depth > 0 {
				depth--
			}
		case ',':
			if !inString && !inChar && depth == 0 {
				if part := strings.TrimSpace(s[start:i]); part != "" {
					result = append(result, part)
				}
				start = i + 1
			}
		}
	}
	if part := strings.TrimSpace(s[start:]); part != "" {
		result = append(result, part)
	}
	return result
}

func immFitsEncoded32(val int64) bool {
	if val < 0 {
		return val >= math.MinInt32
	}
	return uint64(val) <= math.MaxUint32
}

func (a oneLineAssembler) asmMove(size byte, operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("move requires 2 operands (rd, rs/#imm)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	src := strings.TrimSpace(operands[1])
	if strings.HasPrefix(src, "#") {
		val, err := evalExpr(strings.TrimSpace(src[1:]))
		if err != nil {
			return nil, fmt.Errorf("immediate value: %v", err)
		}
		if (size == sizeL || size == sizeQ) && !immFitsEncoded32(val) {
			return nil, fmt.Errorf("immediate $%X out of 32-bit encoding range", uint64(val))
		}
		return encode(opMove, rd, size, 1, 0, 0, uint32(val)), nil
	}
	rs, ok := parseRegister(src)
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", src)
	}
	return encode(opMove, rd, size, 0, rs, 0, 0), nil
}

func (a oneLineAssembler) asmImmReg(opcode byte, operands []string, name string, size byte) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("%s requires 2 operands (rd, #imm)", name)
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	src := strings.TrimSpace(operands[1])
	if !strings.HasPrefix(src, "#") {
		return nil, fmt.Errorf("%s requires immediate operand", name)
	}
	val, err := evalExpr(strings.TrimSpace(src[1:]))
	if err != nil {
		return nil, fmt.Errorf("immediate value: %v", err)
	}
	if !immFitsEncoded32(val) {
		return nil, fmt.Errorf("immediate $%X out of 32-bit range for %s", uint64(val), name)
	}
	return encode(opcode, rd, size, 1, 0, 0, uint32(val)), nil
}

func (a oneLineAssembler) asmLea(operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("lea requires 2 operands (rd, disp(rs))")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	disp, rs, err := parseDispReg(operands[1])
	if err != nil {
		return nil, fmt.Errorf("lea: %v", err)
	}
	return encode(opLea, rd, sizeQ, 1, rs, 0, uint32(disp)), nil
}

func (a oneLineAssembler) asmLoadStore(opcode byte, size byte, operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("load/store requires 2 operands")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[0])
	}
	disp, rs, err := parseDispReg(operands[1])
	if err != nil {
		return nil, err
	}
	xbit := byte(0)
	if disp != 0 {
		xbit = 1
	}
	return encode(opcode, rd, size, xbit, rs, 0, uint32(disp)), nil
}

func parseDispReg(s string) (int64, byte, error) {
	s = strings.TrimSpace(s)
	parenIdx := strings.LastIndex(s, "(")
	closeIdx := strings.LastIndex(s, ")")
	if parenIdx < 0 || closeIdx < parenIdx {
		return 0, 0, fmt.Errorf("expected disp(rs) form, got: %s", s)
	}
	dispStr := strings.TrimSpace(s[:parenIdx])
	regStr := strings.TrimSpace(s[parenIdx+1 : closeIdx])
	rs, ok := parseRegister(regStr)
	if !ok {
		return 0, 0, fmt.Errorf("invalid register in addressing mode: %s", regStr)
	}
	if dispStr == "" {
		return 0, rs, nil
	}
	disp, err := evalExpr(dispStr)
	if err != nil {
		return 0, 0, fmt.Errorf("displacement: %v", err)
	}
	return disp, rs, nil
}

func (a oneLineAssembler) asmALU3(opcode byte, size byte, operands []string) ([]byte, error) {
	if len(operands) != 3 {
		return nil, fmt.Errorf("requires 3 operands (rd, rs, rt/#imm)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	rs, ok := parseRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", operands[1])
	}
	third := strings.TrimSpace(operands[2])
	if strings.HasPrefix(third, "#") {
		val, err := evalExpr(strings.TrimSpace(third[1:]))
		if err != nil {
			return nil, fmt.Errorf("immediate: %v", err)
		}
		return encode(opcode, rd, size, 1, rs, 0, uint32(val)), nil
	}
	rt, ok := parseRegister(third)
	if !ok {
		return nil, fmt.Errorf("invalid third operand: %s", third)
	}
	return encode(opcode, rd, size, 0, rs, rt, 0), nil
}

func (a oneLineAssembler) asmALU2(opcode byte, size byte, operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("requires 2 operands (rd, rs)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	rs, ok := parseRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", operands[1])
	}
	return encode(opcode, rd, size, 0, rs, 0, 0), nil
}

func (a oneLineAssembler) asmLOnlyALU2(opcode byte, mnemonic string, size byte, operands []string) ([]byte, error) {
	if size != sizeL {
		return nil, fmt.Errorf("%s requires .l size suffix", mnemonic)
	}
	return a.asmALU2(opcode, size, operands)
}

func (a oneLineAssembler) asmQOnlyALU3(opcode byte, mnemonic string, size byte, operands []string) ([]byte, error) {
	if size != sizeQ {
		return nil, fmt.Errorf("%s is .q-only and does not accept size suffixes", mnemonic)
	}
	return a.asmALU3(opcode, size, operands)
}

func (a oneLineAssembler) asmBranch(opcode byte, operands []string, origin uint64) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("bra requires 1 operand (target)")
	}
	imm, err := branchImm32(origin, strings.TrimSpace(operands[0]))
	if err != nil {
		return nil, err
	}
	return encode(opcode, 0, sizeQ, 0, 0, 0, imm), nil
}

func (a oneLineAssembler) asmBcc(opcode byte, operands []string, origin uint64) ([]byte, error) {
	if len(operands) != 3 {
		return nil, fmt.Errorf("conditional branch requires 3 operands (rs, rt, target)")
	}
	rs, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[0])
	}
	rt, ok := parseRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[1])
	}
	imm, err := branchImm32(origin, strings.TrimSpace(operands[2]))
	if err != nil {
		return nil, err
	}
	return encode(opcode, 0, sizeQ, 0, rs, rt, imm), nil
}

func (a oneLineAssembler) asmZeroBranch(opcode byte, operands []string, origin uint64) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("zero branch requires 2 operands (rs, target)")
	}
	return a.asmBcc(opcode, []string{operands[0], "r0", operands[1]}, origin)
}

func branchImm32(origin uint64, targetExpr string) (uint32, error) {
	target, err := evalAddress(targetExpr)
	if err != nil {
		return 0, fmt.Errorf("target address: %v", err)
	}
	if target >= origin {
		diff := target - origin
		if diff > math.MaxInt32 {
			return 0, fmt.Errorf("branch target $%016X from origin $%016X out of signed 32-bit range", target, origin)
		}
		return uint32(diff), nil
	}
	diff := origin - target
	if diff > uint64(math.MaxInt32)+1 {
		return 0, fmt.Errorf("branch target $%016X from origin $%016X out of signed 32-bit range", target, origin)
	}
	return uint32(int32(-int64(diff))), nil
}

func (a oneLineAssembler) asmJmp(operands []string) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("jmp requires 1 operand (register-indirect)")
	}
	disp, rs, err := parseDispReg(operands[0])
	if err != nil {
		return nil, fmt.Errorf("jmp requires register-indirect operand: %v", err)
	}
	return encode(opJmp, 0, 0, 0, rs, 0, uint32(disp)), nil
}

func (a oneLineAssembler) asmJsr(operands []string, origin uint64) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("jsr requires 1 operand")
	}
	if disp, rs, err := parseDispReg(operands[0]); err == nil {
		return encode(opJsrInd, 0, 0, 0, rs, 0, uint32(disp)), nil
	}
	imm, err := branchImm32(origin, strings.TrimSpace(operands[0]))
	if err != nil {
		return nil, err
	}
	return encode(opJsr, 0, sizeQ, 0, 0, 0, imm), nil
}

func (a oneLineAssembler) asmPushPop(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("push/pop requires 1 operand (register)")
	}
	reg, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[0])
	}
	if opcode == opPush {
		return encode(opcode, 0, sizeQ, 0, reg, 0, 0), nil
	}
	return encode(opcode, reg, sizeQ, 0, 0, 0, 0), nil
}

func (a oneLineAssembler) asmSingleImm(opcode byte, operands []string, name string) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("%s requires 1 operand (#number)", name)
	}
	src := strings.TrimSpace(operands[0])
	if !strings.HasPrefix(src, "#") {
		return nil, fmt.Errorf("%s requires immediate operand (#number)", name)
	}
	val, err := evalExpr(strings.TrimSpace(src[1:]))
	if err != nil {
		return nil, fmt.Errorf("%s number: %v", name, err)
	}
	return encode(opcode, 0, 0, 1, 0, 0, uint32(val)), nil
}

func parseCR(name string) (byte, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "cr0", "ptbr":
		return 0, true
	case "cr1", "fault_addr":
		return 1, true
	case "cr2", "fault_cause":
		return 2, true
	case "cr3", "fault_pc":
		return 3, true
	case "cr4", "trap_vec":
		return 4, true
	case "cr5", "mmu_ctrl":
		return 5, true
	case "cr6", "tp":
		return 6, true
	case "cr7", "intr_vec":
		return 7, true
	case "cr8", "ksp":
		return 8, true
	case "cr9", "timer_period":
		return 9, true
	case "cr10", "timer_count":
		return 10, true
	case "cr11", "timer_ctrl":
		return 11, true
	case "cr12", "usp":
		return 12, true
	case "cr13", "prev_mode":
		return 13, true
	case "cr14", "saved_sua":
		return 14, true
	default:
		return 0, false
	}
}

func (a oneLineAssembler) asmMTCR(operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("mtcr requires 2 operands (cr#, rs)")
	}
	cr, ok := parseCR(operands[0])
	if !ok {
		return nil, fmt.Errorf("mtcr: invalid control register: %s", operands[0])
	}
	rs, ok := parseRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("mtcr: invalid source register: %s", operands[1])
	}
	return encode(opMtcr, cr, 0, 0, rs, 0, 0), nil
}

func (a oneLineAssembler) asmMFCR(operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("mfcr requires 2 operands (rd, cr#)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("mfcr: invalid destination register: %s", operands[0])
	}
	cr, ok := parseCR(operands[1])
	if !ok {
		return nil, fmt.Errorf("mfcr: invalid control register: %s", operands[1])
	}
	return encode(opMfcr, rd, 0, 0, cr, 0, 0), nil
}

func (a oneLineAssembler) asmTLBINVAL(operands []string) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("tlbinval requires 1 operand (rs)")
	}
	rs, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("tlbinval: invalid register: %s", operands[0])
	}
	return encode(opTlbinval, 0, 0, 0, rs, 0, 0), nil
}

func (a oneLineAssembler) asmSMODE(operands []string) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("smode requires 1 operand (rd)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("smode: invalid register: %s", operands[0])
	}
	return encode(opSmode, rd, 0, 0, 0, 0, 0), nil
}

func (a oneLineAssembler) asmAtomic(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 3 {
		return nil, fmt.Errorf("atomic instruction requires 3 operands (rd, disp(rs), rt)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("atomic: invalid destination register: %s", operands[0])
	}
	disp, rs, err := parseDispReg(operands[1])
	if err != nil {
		return nil, fmt.Errorf("atomic: invalid address operand: %v", err)
	}
	rt, ok := parseRegister(operands[2])
	if !ok {
		return nil, fmt.Errorf("atomic: invalid operand register: %s", operands[2])
	}
	return encode(opcode, rd, 0, 0, rs, rt, uint32(disp)), nil
}

func (a oneLineAssembler) asmFP2(opcode byte, operands []string, isSrcFP, isDstFP bool) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("FPU instruction requires 2 operands")
	}
	dst, ok := parseRegisterOrFP(operands[0], isDstFP)
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	src, ok := parseRegisterOrFP(operands[1], isSrcFP)
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", operands[1])
	}
	return encode(opcode, dst, sizeL, 0, src, 0, 0), nil
}

func parseRegisterOrFP(operand string, fp bool) (byte, bool) {
	if fp {
		return parseFPRegister(operand)
	}
	return parseRegister(operand)
}

func (a oneLineAssembler) asmFP3(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 3 {
		return nil, fmt.Errorf("FPU instruction requires 3 operands")
	}
	rd, ok := parseFPRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	rs, ok := parseFPRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register 1: %s", operands[1])
	}
	rt, ok := parseFPRegister(operands[2])
	if !ok {
		return nil, fmt.Errorf("invalid source register 2: %s", operands[2])
	}
	return encode(opcode, rd, sizeL, 0, rs, rt, 0), nil
}

func (a oneLineAssembler) asmFP3Int(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 3 {
		return nil, fmt.Errorf("FPU instruction requires 3 operands")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	rs, ok := parseFPRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register 1: %s", operands[1])
	}
	rt, ok := parseFPRegister(operands[2])
	if !ok {
		return nil, fmt.Errorf("invalid source register 2: %s", operands[2])
	}
	return encode(opcode, rd, sizeL, 0, rs, rt, 0), nil
}

func (a oneLineAssembler) asmFPMem(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("FPU memory instruction requires 2 operands")
	}
	freg, ok := parseFPRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid FP register: %s", operands[0])
	}
	disp, rs, err := parseDispReg(operands[1])
	if err != nil {
		return nil, err
	}
	xbit := byte(0)
	if disp != 0 {
		xbit = 1
	}
	return encode(opcode, freg, sizeL, xbit, rs, 0, uint32(disp)), nil
}

func (a oneLineAssembler) asmFPImm(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("FPU instruction requires 2 operands")
	}
	rd, ok := parseFPRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	src := strings.TrimSpace(operands[1])
	if !strings.HasPrefix(src, "#") {
		return nil, fmt.Errorf("expected immediate value (starting with #)")
	}
	val, err := evalExpr(strings.TrimSpace(src[1:]))
	if err != nil {
		return nil, err
	}
	return encode(opcode, rd, sizeL, 1, 0, 0, uint32(val)), nil
}

func (a oneLineAssembler) asmFPStatus(opcode byte, operands []string, write bool) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("FPU status instruction requires 1 operand")
	}
	reg, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[0])
	}
	if write {
		return encode(opcode, 0, sizeL, 0, reg, 0, 0), nil
	}
	return encode(opcode, reg, sizeL, 0, 0, 0, 0), nil
}

func (a oneLineAssembler) asmFP2Even(opcode byte, operands []string, isSrcFP, isDstFP bool) ([]byte, error) {
	instr, err := a.asmFP2(opcode, operands, isSrcFP, isDstFP)
	if err != nil {
		return nil, err
	}
	if isDstFP {
		reg, _ := parseFPRegister(operands[0])
		if err := requireEvenFP(reg, "destination"); err != nil {
			return nil, err
		}
	}
	if isSrcFP {
		reg, _ := parseFPRegister(operands[1])
		if err := requireEvenFP(reg, "source"); err != nil {
			return nil, err
		}
	}
	return instr, nil
}

func (a oneLineAssembler) asmFP3Even(opcode byte, operands []string) ([]byte, error) {
	instr, err := a.asmFP3(opcode, operands)
	if err != nil {
		return nil, err
	}
	for i, operand := range operands {
		reg, _ := parseFPRegister(operand)
		label := "source"
		if i == 0 {
			label = "destination"
		}
		if err := requireEvenFP(reg, label); err != nil {
			return nil, err
		}
	}
	return instr, nil
}

func (a oneLineAssembler) asmFP3EvenInt(opcode byte, operands []string) ([]byte, error) {
	instr, err := a.asmFP3Int(opcode, operands)
	if err != nil {
		return nil, err
	}
	rs, _ := parseFPRegister(operands[1])
	rt, _ := parseFPRegister(operands[2])
	if err := requireEvenFP(rs, "source"); err != nil {
		return nil, err
	}
	if err := requireEvenFP(rt, "source"); err != nil {
		return nil, err
	}
	return instr, nil
}

func (a oneLineAssembler) asmFPMemEven(opcode byte, operands []string) ([]byte, error) {
	instr, err := a.asmFPMem(opcode, operands)
	if err != nil {
		return nil, err
	}
	reg, _ := parseFPRegister(operands[0])
	if err := requireEvenFP(reg, "FP operand"); err != nil {
		return nil, err
	}
	return instr, nil
}

func (a oneLineAssembler) asmFCVTSD(operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("FPU instruction requires 2 operands")
	}
	rd, ok := parseFPRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	if err := requireEvenFP(rd, "destination"); err != nil {
		return nil, err
	}
	rs, ok := parseFPRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", operands[1])
	}
	return encode(opFcvtsd, rd, sizeL, 0, rs, 0, 0), nil
}

func (a oneLineAssembler) asmFCVTDS(operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("FPU instruction requires 2 operands")
	}
	rd, ok := parseFPRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	rs, ok := parseFPRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", operands[1])
	}
	if err := requireEvenFP(rs, "source"); err != nil {
		return nil, err
	}
	return encode(opFcvtds, rd, sizeL, 0, rs, 0, 0), nil
}

func requireEvenFP(reg byte, operand string) error {
	if reg&1 != 0 {
		return fmt.Errorf("%s must use an even-numbered FP register", operand)
	}
	return nil
}

type exprParser struct {
	input string
	pos   int
}

func evalAddress(s string) (uint64, error) {
	v, err := evalExpr(s)
	if err != nil {
		return 0, err
	}
	return uint64(v), nil
}

func evalExpr(s string) (int64, error) {
	p := &exprParser{input: strings.TrimSpace(s)}
	if p.input == "" {
		return 0, fmt.Errorf("empty expression")
	}
	val, err := p.parseLogicalOr()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.pos != len(p.input) {
		return 0, fmt.Errorf("unexpected character %q at position %d", p.input[p.pos], p.pos)
	}
	return val, nil
}

func (p *exprParser) skipSpaces() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) peek() byte {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *exprParser) peekTwo() string {
	p.skipSpaces()
	if p.pos+1 >= len(p.input) {
		return ""
	}
	return p.input[p.pos : p.pos+2]
}

func (p *exprParser) parseLogicalOr() (int64, error) {
	left, err := p.parseLogicalAnd()
	if err != nil {
		return 0, err
	}
	for p.peekTwo() == "||" {
		p.pos += 2
		right, err := p.parseLogicalAnd()
		if err != nil {
			return 0, err
		}
		left = boolInt(left != 0 || right != 0)
	}
	return left, nil
}

func (p *exprParser) parseLogicalAnd() (int64, error) {
	left, err := p.parseCompare()
	if err != nil {
		return 0, err
	}
	for p.peekTwo() == "&&" {
		p.pos += 2
		right, err := p.parseCompare()
		if err != nil {
			return 0, err
		}
		left = boolInt(left != 0 && right != 0)
	}
	return left, nil
}

func (p *exprParser) parseCompare() (int64, error) {
	left, err := p.parseBitwiseOr()
	if err != nil {
		return 0, err
	}
	for {
		tw := p.peekTwo()
		switch tw {
		case "==", "!=", "<=", ">=":
			p.pos += 2
			right, err := p.parseBitwiseOr()
			if err != nil {
				return 0, err
			}
			switch tw {
			case "==":
				left = boolInt(left == right)
			case "!=":
				left = boolInt(left != right)
			case "<=":
				left = boolInt(left <= right)
			case ">=":
				left = boolInt(left >= right)
			}
			continue
		}
		switch ch := p.peek(); {
		case ch == '<' && tw != "<<":
			p.pos++
			right, err := p.parseBitwiseOr()
			if err != nil {
				return 0, err
			}
			left = boolInt(left < right)
		case ch == '>' && tw != ">>":
			p.pos++
			right, err := p.parseBitwiseOr()
			if err != nil {
				return 0, err
			}
			left = boolInt(left > right)
		default:
			return left, nil
		}
	}
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func (p *exprParser) parseBitwiseOr() (int64, error) {
	left, err := p.parseBitwiseXor()
	if err != nil {
		return 0, err
	}
	for p.peekTwo() != "||" && p.peek() == '|' {
		p.pos++
		right, err := p.parseBitwiseXor()
		if err != nil {
			return 0, err
		}
		left |= right
	}
	return left, nil
}

func (p *exprParser) parseBitwiseXor() (int64, error) {
	left, err := p.parseBitwiseAnd()
	if err != nil {
		return 0, err
	}
	for p.peek() == '^' {
		p.pos++
		right, err := p.parseBitwiseAnd()
		if err != nil {
			return 0, err
		}
		left ^= right
	}
	return left, nil
}

func (p *exprParser) parseBitwiseAnd() (int64, error) {
	left, err := p.parseShift()
	if err != nil {
		return 0, err
	}
	for p.peekTwo() != "&&" && p.peek() == '&' {
		p.pos++
		right, err := p.parseShift()
		if err != nil {
			return 0, err
		}
		left &= right
	}
	return left, nil
}

func (p *exprParser) parseAdd() (int64, error) {
	left, err := p.parseMul()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '+':
			p.pos++
			right, err := p.parseMul()
			if err != nil {
				return 0, err
			}
			left += right
		case '-':
			p.pos++
			right, err := p.parseMul()
			if err != nil {
				return 0, err
			}
			left -= right
		default:
			return left, nil
		}
	}
}

func (p *exprParser) parseMul() (int64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '*':
			p.pos++
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			left *= right
		case '/':
			p.pos++
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero in expression")
			}
			left /= right
		default:
			return left, nil
		}
	}
}

func (p *exprParser) parseShift() (int64, error) {
	left, err := p.parseAdd()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if strings.HasPrefix(p.input[p.pos:], "<<") {
			p.pos += 2
			right, err := p.parseAdd()
			if err != nil {
				return 0, err
			}
			left <<= uint(right)
			continue
		}
		if strings.HasPrefix(p.input[p.pos:], ">>") {
			p.pos += 2
			right, err := p.parseAdd()
			if err != nil {
				return 0, err
			}
			left >>= uint(right)
			continue
		}
		return left, nil
	}
}

func (p *exprParser) parseUnary() (int64, error) {
	switch p.peek() {
	case '+':
		p.pos++
		return p.parseUnary()
	case '-':
		p.pos++
		val, err := p.parseUnary()
		return -val, err
	case '~':
		p.pos++
		val, err := p.parseUnary()
		return ^val, err
	case '!':
		p.pos++
		val, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return boolInt(val == 0), nil
	default:
		return p.parseAtom()
	}
}

func (p *exprParser) parseAtom() (int64, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if p.input[p.pos] == '(' {
		p.pos++
		val, err := p.parseLogicalOr()
		if err != nil {
			return 0, err
		}
		if p.peek() != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}
	if isIdentStart(p.input[p.pos]) {
		start := p.pos
		for p.pos < len(p.input) && (isIdentChar(p.input[p.pos]) || p.input[p.pos] == '.') {
			p.pos++
		}
		name := strings.ToLower(p.input[start:p.pos])
		if name == "lo" || name == "hi" {
			if p.peek() != '(' {
				return 0, fmt.Errorf("%s requires parenthesized expression", name)
			}
			p.pos++
			val, err := p.parseLogicalOr()
			if err != nil {
				return 0, err
			}
			if p.peek() != ')' {
				return 0, fmt.Errorf("missing closing parenthesis for %s", name)
			}
			p.pos++
			u := uint64(val)
			if name == "lo" {
				return int64(uint32(u)), nil
			}
			return int64(uint32(u >> 32)), nil
		}
		return 0, fmt.Errorf("undefined symbol: %s", p.input[start:p.pos])
	}
	return p.parseNumber()
}

func (p *exprParser) parseNumber() (int64, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	base := 10
	prefix := ""
	if p.input[p.pos] == '$' {
		base = 16
		prefix = "$"
		p.pos++
	} else if p.input[p.pos] == '%' {
		base = 2
		prefix = "%"
		p.pos++
	} else if p.pos+1 < len(p.input) && p.input[p.pos] == '0' && (p.input[p.pos+1] == 'x' || p.input[p.pos+1] == 'X') {
		base = 16
		prefix = "0x"
		p.pos += 2
	}
	start := p.pos
	for p.pos < len(p.input) && (isDigitForBase(p.input[p.pos], base) || p.input[p.pos] == '_') {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("expected numeric literal")
	}
	text := strings.ReplaceAll(p.input[start:p.pos], "_", "")
	u, err := strconv.ParseUint(text, base, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s%s", prefix, p.input[start:p.pos])
	}
	return int64(u), nil
}

func isDigitForBase(c byte, base int) bool {
	switch {
	case base == 2:
		return c == '0' || c == '1'
	case c >= '0' && c <= '9':
		return true
	case base == 16 && c >= 'a' && c <= 'f':
		return true
	case base == 16 && c >= 'A' && c <= 'F':
		return true
	default:
		return false
	}
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}
