package main

// ie32LoweringKind is a mandatory, untagged classification. A helper is only
// permitted where the host must observe the machine at that instruction.
type ie32LoweringKind uint8

const (
	ie32LoweringDirect ie32LoweringKind = iota
	ie32LoweringHelper
	ie32LoweringHalt
)

type ie32OpcodeManifestEntry struct {
	Name string
	Kind ie32LoweringKind
}

type ie32OpcodeForm struct {
	Opcode   byte
	AddrMode byte
}

func ie32ManifestEntries(opcodes ...byte) map[byte]ie32OpcodeManifestEntry {
	m := make(map[byte]ie32OpcodeManifestEntry, len(opcodes))
	for _, opcode := range opcodes {
		m[opcode] = ie32OpcodeManifestEntry{Kind: ie32LoweringDirect}
	}
	return m
}

// The manifest remains available on every target. Backend files record their
// emitted provenance against these exact opcodes rather than creating target
// local inventories.
var ie32OpcodeManifest = func() map[byte]ie32OpcodeManifestEntry {
	m := ie32ManifestEntries(
		LOAD, STORE, ADD, SUB, MUL, DIV, MOD, AND, OR, XOR, NOT, SHL, SHR,
		JMP, JNZ, JZ, JGT, JGE, JLT, JLE, PUSH, POP, JSR, RTS, SEI, CLI,
		RTI, INC, DEC, LDA, LDX, LDY, LDZ, STA, STX, STY, STZ,
		LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW,
		STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW,
	)
	for opcode, entry := range m {
		entry.Name = ie32OpcodeName(opcode)
		m[opcode] = entry
	}
	// These instructions retain architectural helper boundaries in the current
	// direct backends. Keeping the distinction in the common manifest prevents
	// a target from reporting an interpreter-resumed instruction as emitted.
	m[WAIT] = ie32OpcodeManifestEntry{Name: "WAIT", Kind: ie32LoweringHelper}
	m[NOP] = ie32OpcodeManifestEntry{Name: "NOP", Kind: ie32LoweringDirect}
	m[HALT] = ie32OpcodeManifestEntry{Name: "HALT", Kind: ie32LoweringHalt}
	return m
}()

// ie32KnownOpcode is the scanner's allocation-free mirror of the manifest.
// The manifest remains the authoritative provenance table; the fixed lookup
// merely avoids hashing every opcode while forming hot blocks.
var ie32KnownOpcode = func() [256]bool {
	var known [256]bool
	for opcode := range ie32OpcodeManifest {
		known[opcode] = true
	}
	return known
}()

var ie32FormLowering = func() map[ie32OpcodeForm]ie32LoweringKind {
	forms := make(map[ie32OpcodeForm]ie32LoweringKind, len(ie32OpcodeManifest)*256)
	for opcode, entry := range ie32OpcodeManifest {
		kind := ie32LoweringHelper
		if entry.Kind == ie32LoweringHalt {
			kind = ie32LoweringHalt
		}
		for mode := 0; mode <= 0xFF; mode++ {
			forms[ie32OpcodeForm{opcode, byte(mode)}] = kind
		}
	}

	directEveryMode := []byte{NOP, NOT, JMP, JNZ, JZ, JGT, JGE, JLT, JLE, PUSH, POP, JSR, RTS}
	for _, opcode := range directEveryMode {
		setIE32FormLowering(forms, opcode, ie32LoweringDirect, 0, 1, 2, 3, 4)
	}

	loads := []byte{LOAD, LDA, LDX, LDY, LDZ, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW}
	for _, opcode := range loads {
		setIE32FormLowering(forms, opcode, ie32LoweringDirect, ADDR_IMMEDIATE, ADDR_REGISTER, ADDR_REG_IND, ADDR_DIRECT)
	}

	stores := []byte{STORE, STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW}
	for _, opcode := range stores {
		setIE32FormLowering(forms, opcode, ie32LoweringDirect, ADDR_IMMEDIATE, ADDR_REGISTER, ADDR_REG_IND, ADDR_MEM_IND, ADDR_DIRECT)
	}

	for _, opcode := range []byte{ADD, SUB, MUL, AND, OR, XOR} {
		setIE32FormLowering(forms, opcode, ie32LoweringDirect, ADDR_IMMEDIATE, ADDR_REGISTER, ADDR_REG_IND, ADDR_DIRECT)
	}
	for _, opcode := range []byte{SHL, SHR} {
		setIE32FormLowering(forms, opcode, ie32LoweringDirect, ADDR_IMMEDIATE, ADDR_REGISTER)
	}
	for _, opcode := range []byte{DIV, MOD} {
		setIE32FormLowering(forms, opcode, ie32LoweringDirect, ADDR_IMMEDIATE, ADDR_REGISTER, ADDR_REG_IND, ADDR_DIRECT)
	}
	for _, opcode := range []byte{INC, DEC} {
		setIE32FormLowering(forms, opcode, ie32LoweringDirect, ADDR_REGISTER, ADDR_REG_IND, ADDR_MEM_IND, ADDR_DIRECT)
	}
	return forms
}()

func setIE32FormLowering(forms map[ie32OpcodeForm]ie32LoweringKind, opcode byte, kind ie32LoweringKind, modes ...byte) {
	for _, mode := range modes {
		forms[ie32OpcodeForm{opcode, mode}] = kind
	}
}

func ie32OpcodeName(opcode byte) string {
	// Names are diagnostic only. Keeping the raw hexadecimal fallback makes an
	// accidental manifest hole apparent in tests and generated reports.
	for name, value := range map[string]byte{
		"LOAD": LOAD, "STORE": STORE, "ADD": ADD, "SUB": SUB, "MUL": MUL, "DIV": DIV, "MOD": MOD,
		"AND": AND, "OR": OR, "XOR": XOR, "NOT": NOT, "SHL": SHL, "SHR": SHR,
		"JMP": JMP, "JNZ": JNZ, "JZ": JZ, "JGT": JGT, "JGE": JGE, "JLT": JLT, "JLE": JLE,
		"PUSH": PUSH, "POP": POP, "JSR": JSR, "RTS": RTS, "SEI": SEI, "CLI": CLI, "RTI": RTI,
		"WAIT": WAIT, "INC": INC, "DEC": DEC, "LDA": LDA, "LDX": LDX, "LDY": LDY, "LDZ": LDZ,
		"STA": STA, "STX": STX, "STY": STY, "STZ": STZ, "LDB": LDB, "LDC": LDC, "LDD": LDD,
		"LDE": LDE, "LDF": LDF, "LDG": LDG, "LDH": LDH, "LDS": LDS, "LDT": LDT, "LDU": LDU,
		"LDV": LDV, "LDW": LDW, "STB": STB, "STC": STC, "STD": STD, "STE": STE, "STF": STF,
		"STG": STG, "STH": STH, "STS": STS, "STT": STT, "STU": STU, "STV": STV, "STW": STW,
		"NOP": NOP, "HALT": HALT,
	} {
		if value == opcode {
			return name
		}
	}
	return "unknown"
}
