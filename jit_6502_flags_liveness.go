// jit_6502_flags_liveness.go - per-instruction NZ-flag liveness for 6502 JIT
// (Phase 2c of the six-CPU JIT unification plan).
//
// Sibling backends already have liveness analysis for their flag word: x86
// has flagsNeeded populated by x86PeepholeFlags (jit_x86_emit_amd64.go),
// Z80 has flagsNeeded populated by z80PeepholeFlags
// (jit_z80_emit_amd64.go:1261). M68K's was added in Phase 2b
// (jit_m68k_ccr_liveness.go). 6502 lacked both; this file fills that gap.
//
// The 6502 status register has 7 bits (NV-BDIZC) but the JIT only needs
// per-instruction tracking of N+Z (the two bits the bulk of arithmetic and
// load opcodes write). Carry has its own dedicated state in the lazy-flag
// machine — it is preserved across instructions that don't touch C and is
// materialized on demand. This analyzer is for NZ specifically.
//
// Algorithm (reverse-walk demand propagation):
//
//   demand := true            // block exit materializes the latest NZ
//   for i := len-1; i >= 0; i-- {
//     switch class(instrs[i]) {
//       case writesNZ:  live[i] = demand; demand = false
//       case consumer:  demand = true
//       case overwrite: demand = false   // PLP installs full SR
//     }
//   }
//
// Producers (writesNZ): LDA/LDX/LDY, ADC/SBC, AND/ORA/EOR, CMP/CPX/CPY,
// INC/DEC/INX/INY/DEX/DEY, ASL/LSR/ROL/ROR, BIT, TAX/TAY/TXA/TYA/TSX,
// PLA. Stores and TXS do NOT write NZ.
// Consumers: BPL (0x10), BMI (0x30), BNE (0xD0), BEQ (0xF0), PHP (0x08).
// Overwriter: PLP (0x28) installs a fresh SR from the stack.

//go:build amd64 && (linux || windows || darwin)

package main

// p65WritesNZ[op] reports whether the 6502 opcode `op` updates the N and
// Z status bits. Documented opcodes only; undocumented variants are
// false (the conservative scaffold path treats them as non-producers,
// so an undocumented producer's NZ would remain materialized at block
// exit via the normal end-of-block flush — no correctness loss).
var p65WritesNZ = [256]bool{
	// LDA
	0xA9: true, 0xA5: true, 0xB5: true, 0xAD: true, 0xBD: true, 0xB9: true, 0xA1: true, 0xB1: true,
	// LDX
	0xA2: true, 0xA6: true, 0xB6: true, 0xAE: true, 0xBE: true,
	// LDY
	0xA0: true, 0xA4: true, 0xB4: true, 0xAC: true, 0xBC: true,
	// AND
	0x29: true, 0x25: true, 0x35: true, 0x2D: true, 0x3D: true, 0x39: true, 0x21: true, 0x31: true,
	// ORA
	0x09: true, 0x05: true, 0x15: true, 0x0D: true, 0x1D: true, 0x19: true, 0x01: true, 0x11: true,
	// EOR
	0x49: true, 0x45: true, 0x55: true, 0x4D: true, 0x5D: true, 0x59: true, 0x41: true, 0x51: true,
	// ADC
	0x69: true, 0x65: true, 0x75: true, 0x6D: true, 0x7D: true, 0x79: true, 0x61: true, 0x71: true,
	// SBC
	0xE9: true, 0xE5: true, 0xF5: true, 0xED: true, 0xFD: true, 0xF9: true, 0xE1: true, 0xF1: true,
	// CMP / CPX / CPY
	0xC9: true, 0xC5: true, 0xD5: true, 0xCD: true, 0xDD: true, 0xD9: true, 0xC1: true, 0xD1: true,
	0xE0: true, 0xE4: true, 0xEC: true,
	0xC0: true, 0xC4: true, 0xCC: true,
	// INC / DEC
	0xE6: true, 0xF6: true, 0xEE: true, 0xFE: true,
	0xC6: true, 0xD6: true, 0xCE: true, 0xDE: true,
	0xE8: true, 0xC8: true, 0xCA: true, 0x88: true, // INX/INY/DEX/DEY
	// ASL / LSR / ROL / ROR
	0x0A: true, 0x06: true, 0x16: true, 0x0E: true, 0x1E: true,
	0x4A: true, 0x46: true, 0x56: true, 0x4E: true, 0x5E: true,
	0x2A: true, 0x26: true, 0x36: true, 0x2E: true, 0x3E: true,
	0x6A: true, 0x66: true, 0x76: true, 0x6E: true, 0x7E: true,
	// BIT
	0x24: true, 0x2C: true,
	// Transfers (writes NZ except TXS)
	0xAA: true, 0xA8: true, 0x8A: true, 0x98: true, 0xBA: true,
	// PLA
	0x68: true,
}

// p65ConsumesNZ[op] reports whether the 6502 opcode `op` reads the N or
// Z status bit, OR is a control-flow boundary that may exit the JIT
// block — any block exit observes the guest SR via the JIT epilogue,
// so upstream pending NZ must stay live across it. This includes:
//
//   - All 8 conditional branches (BPL/BMI/BVC/BVS/BCC/BCS/BNE/BEQ).
//     Even branches whose own condition does not read NZ (BCC/BCS/
//     BVC/BVS) can SIDE-EXIT the block when the guest target is
//     outside the compiled basic block; the exit epilogue
//     materialises pending NZ.
//   - PHP, which pushes a copy of SR (including NZ) for the guest to
//     observe.
//   - JMP, JSR, RTS, RTI, BRK — unconditional control-flow exits.
var p65ConsumesNZ = [256]bool{
	// Conditional branches — all 8 are potential block side exits.
	0x10: true, // BPL — N
	0x30: true, // BMI — N
	0x50: true, // BVC — exits block on V=0
	0x70: true, // BVS — exits block on V=1
	0x90: true, // BCC — exits block on C=0
	0xB0: true, // BCS — exits block on C=1
	0xD0: true, // BNE — Z
	0xF0: true, // BEQ — Z
	// SR-observing op.
	0x08: true, // PHP
	// Unconditional control-flow exits — block epilogue materialises
	// pending NZ.
	0x4C: true, // JMP abs
	0x6C: true, // JMP (ind)
	0x20: true, // JSR abs
	0x60: true, // RTS
	0x40: true, // RTI
	0x00: true, // BRK
}

// p65OverwritesNZ[op] reports whether the opcode installs a fresh SR
// independent of any prior NZ. PLP pops a complete SR from the stack;
// any pending NZ demand from a downstream consumer is satisfied by the
// PLP, so producers earlier in the block can be marked dead.
var p65OverwritesNZ = [256]bool{
	0x28: true, // PLP
}

// p65NeverBails[op] enumerates the opcodes the 6502 JIT compiles
// without any runtime page-bitmap fast-path check (immediate, implicit,
// accumulator, relative-branch, flag-op encodings). Every other opcode
// can bail to the interpreter on a guarded address probe — and the
// bail epilogue materialises the guest SR (including pending NZ) for
// the interpreter to observe. That makes a bail-capable instruction an
// implicit consumer of upstream NZ: even with no explicit BPL/BNE/PHP
// downstream, a later bail forces upstream pending NZ to be live so
// the materialised SR is correct.
var p65NeverBails = [256]bool{
	// Immediate-mode loads/CMP/ALU.
	//
	// NOTE: ADC #imm (0x69) and SBC #imm (0xE9) are deliberately
	// EXCLUDED from this list. Even though their addressing mode is
	// pure-immediate, the JIT emits a decimal-mode bail check
	// (emit6502DecimalBailCheck) ahead of the actual ADC/SBC: when
	// SR.D is set, the block bails so the interpreter handles BCD
	// arithmetic. That bail is a hidden CCR consumer — pending NZ
	// from upstream must materialise into guest SR before resuming.
	0xA9: true, 0xA2: true, 0xA0: true,
	0xC9: true, 0xE0: true, 0xC0: true,
	0x29: true, 0x09: true, 0x49: true,
	// Accumulator-mode shifts.
	0x0A: true, 0x4A: true, 0x2A: true, 0x6A: true,
	// Implicit transfers / increments / decrements.
	0xAA: true, 0xA8: true, 0x8A: true, 0x98: true, 0xBA: true,
	0xE8: true, 0xCA: true, 0xC8: true, 0x88: true,
	0xEA: true, // NOP
	// Relative branches.
	0x10: true, 0x30: true, 0x50: true, 0x70: true,
	0x90: true, 0xB0: true, 0xD0: true, 0xF0: true,
	// Flag ops.
	0x18: true, 0x38: true, 0x58: true, 0x78: true,
	0xB8: true, 0xD8: true, 0xF8: true,
}

// p65PeepholeFlags analyzes a sequence of 6502 JIT instructions and
// returns, for each slot i, a boolean recording whether that
// instruction's NZ output is consumed by a downstream instruction in
// the same block (true) or is dead (false).
//
// Returns nil for empty input so range loops are safe.
func p65PeepholeFlags(instrs []JIT6502Instr) JITFlagLiveness {
	if len(instrs) == 0 {
		return nil
	}
	live := make(JITFlagLiveness, len(instrs))
	demand := true // block-exit materialization
	for i := len(instrs) - 1; i >= 0; i-- {
		op := instrs[i].opcode
		// Producer-or-overwriter effect (occurs at end of instruction).
		switch {
		case p65WritesNZ[op]:
			live[i] = demand
			demand = false
		case p65OverwritesNZ[op]:
			demand = false
		}
		// Consumer effect (occurs at start of instruction). Explicit
		// branch/PHP consumers reassert demand on prior NZ. Any
		// bail-capable opcode is also an implicit consumer because
		// the bail epilogue materialises the guest SR for the
		// interpreter — upstream pending NZ must therefore stay
		// live.
		if p65ConsumesNZ[op] || !p65NeverBails[op] {
			demand = true
		}
	}
	return live
}

// p65NZConsumers reports whether the 6502 instruction at slot
// instrs[i] is an NZ consumer (BPL, BMI, BNE, BEQ, PHP). Helper exposed
// so future tightening can phrase the question as "any NZ consumer
// between i and the next producer?".
func p65NZConsumers(instrs []JIT6502Instr, i int) bool {
	if i < 0 || i >= len(instrs) {
		return false
	}
	return p65ConsumesNZ[instrs[i].opcode]
}
