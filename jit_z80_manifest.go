package main

import (
	"fmt"
)

// z80JITOpcodeOutcome is the frontend contract for a decoded CPU_Z80 form.
// Canonical-helper rows are deliberately distinct from an interpreter bailout:
// they identify work that must acquire a decoded-payload helper before it can
// be considered backend coverage.
type z80JITOpcodeOutcome uint8

const (
	z80JITOutcomeUnclassified z80JITOpcodeOutcome = iota
	z80JITOutcomeDirect
	z80JITOutcomeCanonicalHelper
	z80JITOutcomeHalt
)

type z80JITOpcodeManifestRow struct {
	Name  string
	Instr JITZ80Instr
	// Outcome remains the amd64 compatibility alias for existing callers.
	// Backend-specific fields are the parity contract.
	Outcome      z80JITOpcodeOutcome
	AMD64Outcome z80JITOpcodeOutcome
	ARM64Outcome z80JITOpcodeOutcome
	WasmOutcome  z80JITOpcodeOutcome
	CPUHalts     bool
	Proof        string // compatibility alias for AMD64Proof
	AMD64Proof   string
	ARM64Proof   string
	WasmProof    string
}

// z80JITOpcodeManifest enumerates every opcode form decoded by CPU_Z80. The
// rows are constructed from the scanner's single instruction classification
// path, preventing a newly decoded opcode from being silently omitted.
func z80JITOpcodeManifest() []z80JITOpcodeManifestRow {
	rows := make([]z80JITOpcodeManifestRow, 0, 7*256)
	appendFamily := func(family string, makeInstr func(byte) JITZ80Instr) {
		for opcode := 0; opcode < 256; opcode++ {
			if family == "base" && (opcode == 0xCB || opcode == 0xDD || opcode == 0xED || opcode == 0xFD) {
				continue // prefix bytes are represented by their decoded families
			}
			if (family == "dd" || family == "fd") && (opcode == 0xDD || opcode == 0xED || opcode == 0xFD) {
				continue // repeated/nested prefixes are decode streams, not opcode forms
			}
			instr := z80ManifestDecodedInstruction(makeInstr(byte(opcode)))
			halts := instr.prefix == z80JITPrefixNone && instr.opcode == 0x76 ||
				(instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD) && instr.opcode == 0x76
			amd64Outcome := z80ManifestAMD64Outcome(instr, halts)
			arm64Outcome := z80ManifestARM64Outcome(instr, halts)
			wasmOutcome := z80ManifestWasmOutcome(instr, halts)
			amd64Proof := z80ManifestNativeProof(amd64Outcome, "AMD64")
			arm64Proof := z80ManifestNativeProof(arm64Outcome, "ARM64")
			wasmProof := z80ManifestWasmProof(wasmOutcome)
			// Wasm has the same semantic admission contract as the native
			// backends: only genuine host-observation forms may use the canonical
			// helper. Backend tests prove every direct declaration reaches emitted
			// WebAssembly rather than silently falling back.
			// ARM64 has the same semantic admission contract as AMD64: every
			// non-observation form is lowered natively. Backend-specific emitter
			// tests prove that this declaration reaches generated ARM64 code.
			rows = append(rows, z80JITOpcodeManifestRow{
				Name:         fmt.Sprintf("%s:%02X", family, opcode),
				Instr:        instr,
				Outcome:      amd64Outcome,
				AMD64Outcome: amd64Outcome,
				ARM64Outcome: arm64Outcome,
				WasmOutcome:  wasmOutcome,
				CPUHalts:     halts,
				Proof:        amd64Proof,
				AMD64Proof:   amd64Proof,
				ARM64Proof:   arm64Proof,
				WasmProof:    wasmProof,
			})
		}
	}

	appendFamily("base", func(op byte) JITZ80Instr { return JITZ80Instr{opcode: op} })
	appendFamily("cb", func(op byte) JITZ80Instr { return JITZ80Instr{prefix: z80JITPrefixCB, opcode: op} })
	appendFamily("ed", func(op byte) JITZ80Instr { return JITZ80Instr{prefix: z80JITPrefixED, opcode: op} })
	appendFamily("dd", func(op byte) JITZ80Instr { return JITZ80Instr{prefix: z80JITPrefixDD, opcode: op} })
	appendFamily("fd", func(op byte) JITZ80Instr { return JITZ80Instr{prefix: z80JITPrefixFD, opcode: op} })
	appendFamily("ddcb", func(op byte) JITZ80Instr { return JITZ80Instr{prefix: z80JITPrefixDD, opcode: 0xCB, cbSubOp: op} })
	appendFamily("fdcb", func(op byte) JITZ80Instr { return JITZ80Instr{prefix: z80JITPrefixFD, opcode: 0xCB, cbSubOp: op} })
	return rows
}

func z80ManifestNativeProof(outcome z80JITOpcodeOutcome, arch string) string {
	switch outcome {
	case z80JITOutcomeDirect:
		return "Test" + arch + "Z80JIT_ManifestNativeDifferential"
	case z80JITOutcomeCanonicalHelper:
		return "TestZ80JIT_ManifestCanonicalHelperDispatchDifferential"
	case z80JITOutcomeHalt:
		return "TestZ80JIT_ManifestHaltContract"
	default:
		return ""
	}
}

func z80ManifestWasmProof(outcome z80JITOpcodeOutcome) string {
	switch outcome {
	case z80JITOutcomeDirect:
		return "TestWasmJIT_Z80ManifestNativeDifferential"
	case z80JITOutcomeCanonicalHelper:
		return "TestWasmJIT_Z80ManifestCanonicalHelperDispatchDifferential"
	case z80JITOutcomeHalt:
		return "TestWasmJIT_Z80ManifestHaltContract"
	default:
		return ""
	}
}

func z80ManifestAMD64Outcome(instr JITZ80Instr, halts bool) z80JITOpcodeOutcome {
	return z80ManifestBackendOutcome(instr, halts, z80ManifestAMD64Observation)
}

func z80ManifestARM64Outcome(instr JITZ80Instr, halts bool) z80JITOpcodeOutcome {
	return z80ManifestBackendOutcome(instr, halts, z80ManifestARM64Observation)
}

func z80ManifestWasmOutcome(instr JITZ80Instr, halts bool) z80JITOpcodeOutcome {
	return z80ManifestBackendOutcome(instr, halts, z80ManifestWasmObservation)
}

func z80ManifestBackendOutcome(instr JITZ80Instr, halts bool, observation func(JITZ80Instr) bool) z80JITOpcodeOutcome {
	if halts {
		return z80JITOutcomeHalt
	}
	if observation(instr) {
		return z80JITOutcomeCanonicalHelper
	}
	return z80JITOutcomeDirect
}

// Keep backend policies separate even where their present observation sets
// agree. A backend change must alter its own classifier and proving test.
func z80ManifestAMD64Observation(instr JITZ80Instr) bool { return z80ManifestHostObservation(instr) }
func z80ManifestARM64Observation(instr JITZ80Instr) bool { return z80ManifestHostObservation(instr) }
func z80ManifestWasmObservation(instr JITZ80Instr) bool  { return z80ManifestHostObservation(instr) }

func z80ManifestHostObservation(instr JITZ80Instr) bool {
	if instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD {
		if !z80ManifestDDFDExplicitOpcode(instr.opcode) {
			instr.prefix = z80JITPrefixNone
			return z80ManifestHostObservation(instr)
		}
		return false
	}
	if instr.prefix == z80JITPrefixNone {
		return instr.opcode == 0xD3 || instr.opcode == 0xDB
	}
	if instr.prefix != z80JITPrefixED {
		return false
	}
	switch instr.opcode {
	case 0x40, 0x48, 0x50, 0x58, 0x60, 0x68, 0x70, 0x78,
		0x41, 0x49, 0x51, 0x59, 0x61, 0x69, 0x71, 0x79,
		0xA2, 0xAA, 0xB2, 0xBA, 0xA3, 0xAB, 0xB3, 0xBB:
		return true
	default:
		return false
	}
}

func z80ManifestDDFDExplicitOpcode(op byte) bool {
	return op == 0x21 || op == 0x22 || op == 0x2A || op == 0xE5 || op == 0xE1 ||
		op == 0xF9 || op == 0x36 || op == 0x34 || op == 0x35 || op == 0xE9 ||
		op == 0xCB || op == 0xE3 || op == 0x09 || op == 0x19 || op == 0x29 ||
		op == 0x39 || op == 0x23 || op == 0x2B ||
		(op&0xC7 == 0x46 && op != 0x76) ||
		(op >= 0x70 && op <= 0x77 && op != 0x76) || op&0xC7 == 0x86
}

// z80ManifestDecodedInstruction supplies the length and placeholder operand
// shape for a representative decode of an opcode form. Its bytes are then a
// valid canonical-helper payload independent of the operand's value.
func z80ManifestDecodedInstruction(instr JITZ80Instr) JITZ80Instr {
	switch instr.prefix {
	case z80JITPrefixCB:
		instr.length = 2
		instr.rIncrements = 2
	case z80JITPrefixED:
		instr.length = 2
		instr.rIncrements = 2
		if z80ManifestEDHasImm16(instr.opcode) {
			instr.length = 4
			instr.hasOperand = true
		}
	case z80JITPrefixDD, z80JITPrefixFD:
		instr.rIncrements = 2
		switch {
		case instr.opcode == z80JITPrefixCB:
			instr.length = 4
			instr.rIncrements = 3
		case z80ManifestDDFDHasDisplacement(instr.opcode):
			instr.length = 3
			if instr.opcode == 0x36 {
				instr.length = 4
				instr.hasOperand = true
			}
		case z80ManifestDDFDHasImm16(instr.opcode):
			instr.length = 4
			instr.hasOperand = true
		case instr.opcode == 0x26 || instr.opcode == 0x2E:
			instr.length = 3
			instr.hasOperand = true
		case z80ManifestBaseHasImm8(instr.opcode) || z80ManifestBaseHasRelJump(instr.opcode):
			// Ignored DD/FD prefixes delegate to the base handler, whose
			// operand remains part of the encoded instruction stream.
			instr.length = 3
			instr.hasOperand = true
		case z80ManifestBaseHasImm16(instr.opcode):
			instr.length = 4
			instr.hasOperand = true
		default:
			instr.length = 2
		}
	default:
		instr.rIncrements = 1
		instr.length = 1
		if z80ManifestBaseHasImm8(instr.opcode) || z80ManifestBaseHasRelJump(instr.opcode) {
			instr.length = 2
			instr.hasOperand = true
		} else if z80ManifestBaseHasImm16(instr.opcode) {
			instr.length = 3
			instr.hasOperand = true
		}
	}
	return instr
}

func z80ManifestSourceBytes(instr JITZ80Instr) []byte {
	switch instr.prefix {
	case z80JITPrefixCB:
		return []byte{z80JITPrefixCB, instr.opcode}
	case z80JITPrefixED:
		bytes := []byte{z80JITPrefixED, instr.opcode}
		if instr.length == 4 {
			bytes = append(bytes, byte(instr.operand), byte(instr.operand>>8))
		}
		return bytes
	case z80JITPrefixDD, z80JITPrefixFD:
		if instr.opcode == z80JITPrefixCB {
			return []byte{instr.prefix, z80JITPrefixCB, byte(instr.displacement), instr.cbSubOp}
		}
		bytes := []byte{instr.prefix, instr.opcode}
		if z80ManifestDDFDHasDisplacement(instr.opcode) {
			bytes = append(bytes, byte(instr.displacement))
		}
		for len(bytes) < int(instr.length) {
			shift := 8 * (len(bytes) - 2)
			if z80ManifestDDFDHasDisplacement(instr.opcode) {
				shift = 8 * (len(bytes) - 3)
			}
			bytes = append(bytes, byte(instr.operand>>shift))
		}
		return bytes
	default:
		bytes := []byte{instr.opcode}
		for len(bytes) < int(instr.length) {
			bytes = append(bytes, byte(instr.operand>>(8*(len(bytes)-1))))
		}
		return bytes
	}
}

func z80ManifestBaseHasImm8(op byte) bool {
	return op&0xC7 == 0x06 || op&0xC7 == 0xC6 || op == 0xD3 || op == 0xDB
}

func z80ManifestBaseHasImm16(op byte) bool {
	return op&0xCF == 0x01 || op == 0x22 || op == 0x2A || op == 0x32 || op == 0x3A ||
		op == 0xC3 || op&0xC7 == 0xC2 || op == 0xCD || op&0xC7 == 0xC4
}

func z80ManifestBaseHasRelJump(op byte) bool {
	return op == 0x10 || op == 0x18 || op == 0x20 || op == 0x28 || op == 0x30 || op == 0x38
}

func z80ManifestDDFDHasDisplacement(op byte) bool {
	return op == 0x34 || op == 0x35 || op == 0x36 ||
		(op&0xC7 == 0x46 && op != 0x76) || (op >= 0x70 && op <= 0x77 && op != 0x76) || op&0xC7 == 0x86
}

func z80ManifestDDFDHasImm16(op byte) bool {
	return op == 0x21 || op == 0x22 || op == 0x2A || z80ManifestBaseHasImm16(op)
}

func z80ManifestEDHasImm16(op byte) bool {
	switch op {
	case 0x43, 0x4B, 0x53, 0x5B, 0x63, 0x6B, 0x73, 0x7B:
		return true
	default:
		return false
	}
}
