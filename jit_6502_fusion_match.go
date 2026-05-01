// jit_6502_fusion_match.go - Phase 7a 6502 universal-idiom fusion match
// (scaffold).
//
// Restricted to FusionIDs 1-4 per plan: the four truly universal 2-byte
// 6502 idioms (DexBne, DeyBne, InxBne, LdaImmStaZp). Bench-shaped IDs 5-9
// and full-program kernels 10-14 are out of scope per the plan's
// scope-narrowing rule.
//
// Mechanism: exit-and-resume. The JIT block scanner detects an idiom
// match at block-entry; the compiled block becomes a short prologue that
// spills mapped registers and returns a JITExitFusion<ID> exit reason.
// jit_6502_exec.go switches on the new reasons and runs the plain-Go
// fuser, advances PC, then resumes JIT execution.
//
// This file is the matcher and the four Go fusers. Wiring into the
// emitter and exec loop is the second-half deliverable of Phase 7a; this
// scaffold provides the matcher, the fuser bodies, and the exit-reason
// constants so the wiring patch is small and reviewable.
//
// Risk: per the plan, this whole sub-phase is OPTIONAL — drop if bench
// delta on Mixed is <2%.

//go:build amd64 && (linux || windows || darwin)

package main

// FusionID enumerates the universal 6502 fusion idioms covered by Phase 7a.
type FusionID int

const (
	FusionNone        FusionID = 0
	FusionDexBne      FusionID = 1 // CA D0 — DEX; BNE rel
	FusionDeyBne      FusionID = 2 // 88 D0 — DEY; BNE rel
	FusionInxBne      FusionID = 3 // E8 D0 — INX; BNE rel
	FusionLdaImmStaZp FusionID = 4 // A9 imm 85 zp — LDA #imm; STA $zp
)

// MatchFusionAtPC returns the FusionID matching the bytes at memory[pc:],
// or FusionNone. The matcher is byte-precise — it does not consult
// adjacency rules or live-flag state, because those are the JIT scanner's
// job; the matcher only answers "do the next 2-4 bytes look like idiom X?"
//
// memory must be the full guest address space (caller already resolved
// banking). pc is wrapped at 16 bits before reads to mirror 6502 PC
// behavior.
func MatchFusionAtPC(memory []byte, pc uint16) FusionID {
	if len(memory) < 0x10000 {
		return FusionNone
	}
	b0 := memory[pc]
	b1 := memory[uint16(pc+1)]
	switch b0 {
	case 0xCA: // DEX
		if b1 == 0xD0 {
			return FusionDexBne
		}
	case 0x88: // DEY
		if b1 == 0xD0 {
			return FusionDeyBne
		}
	case 0xE8: // INX
		if b1 == 0xD0 {
			return FusionInxBne
		}
	case 0xA9: // LDA #imm
		if memory[uint16(pc+2)] == 0x85 {
			return FusionLdaImmStaZp
		}
	}
	return FusionNone
}

// FusionExitReason is the JITExitReason value the emitted prologue returns
// when a fusion is dispatched. Distinct from the I/O-bail and MMU-bail
// exit reasons so the dispatcher can route to the correct fuser without
// re-decoding.
//
// Wired into jit_6502_common.go's exit-reason enum in the second-half
// patch.
type FusionExitReason int

const (
	FusionExitNone        FusionExitReason = 0
	FusionExitDexBne      FusionExitReason = 1
	FusionExitDeyBne      FusionExitReason = 2
	FusionExitInxBne      FusionExitReason = 3
	FusionExitLdaImmStaZp FusionExitReason = 4
)

// fuse6502DexBne executes one (DEX; BNE rel) iteration in plain Go and
// returns the new PC and CPU state. Consumed by jit_6502_exec.go's
// fusion-exit switch.
//
// rel is the signed 8-bit branch displacement read from memory[pc+1].
func fuse6502DexBne(x byte, sr byte, pc uint16, rel int8) (newX byte, newSR byte, newPC uint16) {
	newX = x - 1
	newSR = sr & ^byte(0x82) // clear N, Z
	if newX == 0 {
		newSR |= 0x02 // Z
	}
	if newX&0x80 != 0 {
		newSR |= 0x80 // N
	}
	// DEX (1B) + BNE rel (2B) = 3B idiom; resume PC is pc+3, and the
	// taken target is computed relative to the post-idiom PC per the
	// 6502 branch encoding.
	if newX != 0 {
		newPC = uint16(int32(pc) + 3 + int32(rel))
	} else {
		newPC = pc + 3
	}
	return
}

// fuse6502DeyBne is the DEY; BNE analog.
func fuse6502DeyBne(y byte, sr byte, pc uint16, rel int8) (newY byte, newSR byte, newPC uint16) {
	newY = y - 1
	newSR = sr & ^byte(0x82)
	if newY == 0 {
		newSR |= 0x02
	}
	if newY&0x80 != 0 {
		newSR |= 0x80
	}
	if newY != 0 {
		newPC = uint16(int32(pc) + 3 + int32(rel))
	} else {
		newPC = pc + 3
	}
	return
}

// fuse6502InxBne is the INX; BNE analog.
func fuse6502InxBne(x byte, sr byte, pc uint16, rel int8) (newX byte, newSR byte, newPC uint16) {
	newX = x + 1
	newSR = sr & ^byte(0x82)
	if newX == 0 {
		newSR |= 0x02
	}
	if newX&0x80 != 0 {
		newSR |= 0x80
	}
	if newX != 0 {
		newPC = uint16(int32(pc) + 3 + int32(rel))
	} else {
		newPC = pc + 3
	}
	return
}

// fuse6502LdaImmStaZp executes (LDA #imm; STA $zp). Writes the immediate
// to zero-page memory[zp] and returns updated A + SR. Caller advances PC
// by 4 bytes.
func fuse6502LdaImmStaZp(memory []byte, sr byte, imm byte, zp byte) (newA byte, newSR byte) {
	newA = imm
	memory[zp] = imm
	newSR = sr & ^byte(0x82)
	if newA == 0 {
		newSR |= 0x02
	}
	if newA&0x80 != 0 {
		newSR |= 0x80
	}
	return
}
