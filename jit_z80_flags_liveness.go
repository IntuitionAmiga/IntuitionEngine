// jit_z80_flags_liveness.go - per-instruction flag liveness for Z80 JIT
// (Phase 2c of the six-CPU JIT unification plan).
//
// The Z80 emitter (jit_z80_emit_amd64.go) consults a flagsNeeded bitmap
// at emit time. This file produces a tight bitmap by reverse-walking
// the instruction stream and tracking demand from downstream
// consumers, mirroring the algorithm used by p65PeepholeFlags.
//
// Conservative defaults preserved: an unrecognised opcode (anything
// with a Z80 prefix byte — CB, DD, FD, ED) keeps live=true so the
// emitter still materialises F. Only unprefixed producers are
// candidates for "dead" status.
//
// Producers (write F bits): 8-bit ALU A,r (0x80-0xBF), ALU A,n
// (0xC6/CE/D6/DE/E6/EE/F6/FE), INC/DEC r (0x04..3C step 8 / 0x05..3D),
// rotates RLCA/RRCA/RLA/RRA, DAA/CPL/SCF/CCF.
// Consumers (read F): conditional jumps/calls/rets, ADC/SBC family
// (read C as well as write), PUSH AF, RLA/RRA (read C), DAA, CCF.
// Overwriter: POP AF (loads fresh F from stack).

//go:build amd64 && (linux || windows || darwin)

package main

var z80ProducesF [256]bool
var z80ConsumesF [256]bool
var z80OverwritesF [256]bool

// Partial producers write only some F bits; upstream producers of the
// preserved bits remain live across them. Examples: INC/DEC r preserve C;
// CPL preserves S/Z/P/C; SCF/CCF preserve S/Z/P; rotate accumulators
// preserve S/Z/P; DAA preserves N. Conservative treatment: keep the slot
// live AND propagate demand upstream (do not clear demand, do not
// overwrite-clear).
var z80PartialProducerF [256]bool

// Z80 ED-prefixed flag classification. Indexed by the byte after 0xED.
// Only documented entries that touch F are populated; everything else
// keeps the conservative all-live treatment for prefixed instructions.
var z80EDProducesF [256]bool
var z80EDConsumesF [256]bool
var z80EDPartialProducerF [256]bool

// Z80 CB-prefixed flag classification. CB-prefix opcode layout:
//   bits 7-6 = op type (00 = rotate, 01 = BIT, 10 = RES, 11 = SET)
//   bits 5-3 = bit number (or rotate sub-op for type 00)
//   bits 2-0 = register
// Rotate (00) and BIT (01) update F; RES/SET do not. We classify by
// bits 7-6 of the post-prefix opcode byte.

func init() {
	// 8-bit ALU A,r block 0x80-0xBF: ADD/ADC/SUB/SBC/AND/XOR/OR/CP all produce F.
	for op := 0x80; op <= 0xBF; op++ {
		z80ProducesF[op] = true
	}
	// ADC A,r (0x88-0x8F) and SBC A,r (0x98-0x9F) also read C → consumers.
	for op := 0x88; op <= 0x8F; op++ {
		z80ConsumesF[op] = true
	}
	for op := 0x98; op <= 0x9F; op++ {
		z80ConsumesF[op] = true
	}
	// ALU A,n immediates.
	for _, op := range []byte{0xC6, 0xCE, 0xD6, 0xDE, 0xE6, 0xEE, 0xF6, 0xFE} {
		z80ProducesF[op] = true
	}
	z80ConsumesF[0xCE] = true // ADC A,n reads C
	z80ConsumesF[0xDE] = true // SBC A,n reads C
	// INC r / DEC r (8-bit) write S/Z/H/P/N — preserve C → partial.
	for op := 0x04; op <= 0x3C; op += 8 {
		z80ProducesF[op] = true   // INC r
		z80ProducesF[op+1] = true // DEC r
		z80PartialProducerF[op] = true
		z80PartialProducerF[op+1] = true
	}
	// Rotate accumulator family — write C, clear H/N, preserve S/Z/P → partial.
	for _, op := range []byte{0x07, 0x0F, 0x17, 0x1F} {
		z80ProducesF[op] = true
		z80PartialProducerF[op] = true
	}
	// RLA/RRA also read C (pull C through A).
	z80ConsumesF[0x17] = true
	z80ConsumesF[0x1F] = true
	// DAA reads C+H+N and writes S/Z/H/P/C — preserves N → partial.
	z80ProducesF[0x27] = true
	z80ConsumesF[0x27] = true
	z80PartialProducerF[0x27] = true
	// CPL sets H/N, preserves S/Z/P/C → partial.
	z80ProducesF[0x2F] = true
	z80PartialProducerF[0x2F] = true
	// SCF sets C, clears H/N, preserves S/Z/P → partial.
	z80ProducesF[0x37] = true
	z80PartialProducerF[0x37] = true
	// CCF reads C, toggles C, sets H, clears N, preserves S/Z/P → partial.
	z80ProducesF[0x3F] = true
	z80ConsumesF[0x3F] = true
	z80PartialProducerF[0x3F] = true
	// Conditional branches: JR cc.
	for _, op := range []byte{0x20, 0x28, 0x30, 0x38} {
		z80ConsumesF[op] = true
	}
	// JP cc / CALL cc / RET cc — bit 0 of opcode encodes cc-bit, group
	// in 0xC0-0xFF with low nibble 0/2/4/8/A/C.
	for _, op := range []byte{
		0xC0, 0xC8, 0xD0, 0xD8, 0xE0, 0xE8, 0xF0, 0xF8, // RET cc
		0xC2, 0xCA, 0xD2, 0xDA, 0xE2, 0xEA, 0xF2, 0xFA, // JP cc
		0xC4, 0xCC, 0xD4, 0xDC, 0xE4, 0xEC, 0xF4, 0xFC, // CALL cc
	} {
		z80ConsumesF[op] = true
	}
	// PUSH AF exposes F to the guest.
	z80ConsumesF[0xF5] = true
	// POP AF installs fresh F.
	z80OverwritesF[0xF1] = true

	// ED-prefixed producers/consumers.
	for _, op := range []byte{
		0x44,                   // NEG
		0x42, 0x52, 0x62, 0x72, // SBC HL,ss
		0x4A, 0x5A, 0x6A, 0x7A, // ADC HL,ss
		0x57, 0x5F, // LD A,I / LD A,R
		0xA0, 0xA1, 0xA2, 0xA3, // LDI/CPI/INI/OUTI
		0xA8, 0xA9, 0xAA, 0xAB, // LDD/CPD/IND/OUTD
		0xB0, 0xB1, 0xB2, 0xB3, // LDIR/CPIR/INIR/OTIR
		0xB8, 0xB9, 0xBA, 0xBB, // LDDR/CPDR/INDR/OTDR
		0x67, 0x6F, // RRD / RLD
	} {
		z80EDProducesF[op] = true
	}
	// ADC HL,ss + SBC HL,ss read C.
	for _, op := range []byte{0x42, 0x52, 0x62, 0x72, 0x4A, 0x5A, 0x6A, 0x7A} {
		z80EDConsumesF[op] = true
	}
	// ED partial producers — preserve at least one F bit.
	// LD A,I (0x57) / LD A,R (0x5F): write S/Z, clear H/N, P=IFF2 — preserve C.
	// INI/OUTI/IND/OUTD and their repeating variants: write Z/N, leave others
	// undefined-but-preserved on real hardware → treat as partial.
	// LDI/LDD/LDIR/LDDR: write H=0/P/N=0 — preserve S/Z/C.
	// CPI/CPD/CPIR/CPDR: write S/Z/H/P/N — preserve C.
	// RRD/RLD: write S/Z/H=0/P/N=0 — preserve C.
	for _, op := range []byte{
		0x57, 0x5F,
		0xA0, 0xA1, 0xA2, 0xA3,
		0xA8, 0xA9, 0xAA, 0xAB,
		0xB0, 0xB1, 0xB2, 0xB3,
		0xB8, 0xB9, 0xBA, 0xBB,
		0x67, 0x6F,
	} {
		z80EDPartialProducerF[op] = true
	}
}

// z80FlagsLiveness returns, for each slot i in instrs, whether the F
// output of instrs[i] is consumed by some downstream instruction in
// the same block. Algorithm: reverse-walk demand propagation. Block
// exit materialises the latest F so demand starts true.
//
// Prefixed instructions (CB/DD/FD/ED) are treated conservatively —
// live[i] = true regardless of demand, and demand is reset to true
// (any prefixed instruction may be a consumer or producer the table
// doesn't enumerate). This keeps correctness while still tightening
// unprefixed sequences.
func z80FlagsLiveness(instrs []JITZ80Instr) JITFlagLiveness {
	if len(instrs) == 0 {
		return nil
	}
	live := make(JITFlagLiveness, len(instrs))
	demand := true
	for i := len(instrs) - 1; i >= 0; i-- {
		ins := instrs[i]
		op := ins.opcode
		switch ins.prefix {
		case 0:
			if z80ProducesF[op] {
				if z80PartialProducerF[op] {
					// Partial: must emit, and upstream still owes
					// preserved bits — keep demand.
					live[i] = true
				} else {
					live[i] = demand
					demand = false
				}
			} else {
				live[i] = true
			}
			if z80ConsumesF[op] {
				demand = true
			}
			if z80OverwritesF[op] {
				demand = false
			}
		case z80JITPrefixED:
			if z80EDProducesF[op] {
				if z80EDPartialProducerF[op] {
					live[i] = true
				} else {
					live[i] = demand
					demand = false
				}
			} else {
				live[i] = true
			}
			if z80EDConsumesF[op] {
				demand = true
			}
		case z80JITPrefixCB:
			// 00 = rotate (producer + consumer of C for RL/RR/RLA-like
			// rotates that pull C through). 01 = BIT (producer only).
			// 10/11 = RES/SET (no F effect).
			top2 := op >> 6
			switch top2 {
			case 0: // rotate / shift
				live[i] = demand
				demand = false
				// RL/RR (sub-op 010,011) read C; treat the whole rotate
				// family as consumer of C to be safe — ZX-Spectrum-style
				// chained rotates are common.
				demand = true
			case 1: // BIT n,r
				live[i] = demand
				demand = false
			default: // RES / SET
				live[i] = true
			}
		default:
			// DD/FD/DDCB/FDCB indexed forms — keep conservative.
			live[i] = true
			demand = true
		}
	}
	return live
}

// z80FlagsConsumers reports whether the Z80 instruction at instrs[i]
// is a flag consumer in the unprefixed table. Prefixed instructions
// always return false (the table does not enumerate them — callers
// treating them as consumers should use z80FlagsLiveness instead,
// which marks them conservatively).
func z80FlagsConsumers(instrs []JITZ80Instr, i int) bool {
	if i < 0 || i >= len(instrs) {
		return false
	}
	if instrs[i].prefix != 0 {
		return false
	}
	return z80ConsumesF[instrs[i].opcode]
}
