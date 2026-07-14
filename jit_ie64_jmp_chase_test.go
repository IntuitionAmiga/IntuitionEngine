package main

import (
	"encoding/binary"
	"testing"
)

// chaseWord builds an 8-byte IE64 instruction word from its fields, matching
// the little-endian layout decoded by scanBlock.
func chaseWord(opcode, rd, rs, rt byte, imm32 uint32) uint64 { //nolint:unparam
	byte1 := rd << 3
	byte2 := rs << 3
	byte3 := rt << 3
	return uint64(opcode) | uint64(byte1)<<8 | uint64(byte2)<<16 | uint64(byte3)<<24 | uint64(imm32)<<32
}

func TestIE64DecodeStaticJumpTarget(t *testing.T) {
	// BRA +0x20 at PC 0x100 -> 0x120
	if got, ok := ie64DecodeStaticJumpTarget(chaseWord(OP_BRA, 0, 0, 0, 0x20), 0x100); !ok || got != 0x120 {
		t.Fatalf("BRA: got (0x%x, %v), want (0x120, true)", got, ok)
	}
	// BRA -8 at PC 0x100 -> 0xF8
	neg8 := int32(-8)
	if got, ok := ie64DecodeStaticJumpTarget(chaseWord(OP_BRA, 0, 0, 0, uint32(neg8)), 0x100); !ok || got != 0xF8 {
		t.Fatalf("BRA back: got (0x%x, %v), want (0xF8, true)", got, ok)
	}
	// JMP R0,0x2000 -> absolute 0x2000
	if got, ok := ie64DecodeStaticJumpTarget(chaseWord(OP_JMP, 0, 0, 0, 0x2000), 0x100); !ok || got != 0x2000 {
		t.Fatalf("JMP R0: got (0x%x, %v), want (0x2000, true)", got, ok)
	}
	// JMP R1,... is register-based -> not static
	if _, ok := ie64DecodeStaticJumpTarget(chaseWord(OP_JMP, 0, 1, 0, 0x2000), 0x100); ok {
		t.Fatalf("JMP R1 should not be a static jump")
	}
	// Non-jump opcode
	if _, ok := ie64DecodeStaticJumpTarget(chaseWord(OP_ADD, 1, 2, 3, 0), 0x100); ok {
		t.Fatalf("ADD should not be a static jump")
	}
}

// chaseMem builds a small flat memory image, writes the given instruction
// words at the given byte offsets, and returns a fetch closure over it.
func chaseFetch(words map[uint64]uint64) func(uint64) (uint64, bool) {
	return func(p uint64) (uint64, bool) {
		w, ok := words[p]
		return w, ok
	}
}

func TestIE64ChaseStaticJumps_CollapsesChain(t *testing.T) {
	// 0x00: BRA +0x40  -> 0x40
	// 0x40: JMP R0,0x80 -> 0x80
	// 0x80: ADD (non-jump)  land here
	words := map[uint64]uint64{
		0x00: chaseWord(OP_BRA, 0, 0, 0, 0x40),
		0x40: chaseWord(OP_JMP, 0, 0, 0, 0x80),
		0x80: chaseWord(OP_ADD, 1, 2, 3, 0),
	}
	pc, retired := ie64ChaseStaticJumps(0x00, chaseFetch(words))
	if pc != 0x80 || retired != 2 {
		t.Fatalf("got (0x%x, %d), want (0x80, 2)", pc, retired)
	}
}

func TestIE64ChaseStaticJumps_SelfLoopStops(t *testing.T) {
	// BRA .  (offset 0) is a self-loop; chase must not collapse it.
	words := map[uint64]uint64{
		0x100: chaseWord(OP_BRA, 0, 0, 0, 0),
	}
	pc, retired := ie64ChaseStaticJumps(0x100, chaseFetch(words))
	if pc != 0x100 || retired != 0 {
		t.Fatalf("self-loop: got (0x%x, %d), want (0x100, 0)", pc, retired)
	}
}

func TestIE64ChaseStaticJumps_TwoNodeCycleStops(t *testing.T) {
	// A -> B -> A ...  chase must terminate, collapsing at most one full lap.
	words := map[uint64]uint64{
		0x00: chaseWord(OP_JMP, 0, 0, 0, 0x40),
		0x40: chaseWord(OP_JMP, 0, 0, 0, 0x00),
	}
	pc, retired := ie64ChaseStaticJumps(0x00, chaseFetch(words))
	// Steps 0x00->0x40 (visited{0x00}), then 0x40->0x00 but 0x00 is visited: stop at 0x40.
	if retired >= ie64StaticJumpChaseCap {
		t.Fatalf("cycle did not terminate early: retired=%d", retired)
	}
	if pc != 0x40 || retired != 1 {
		t.Fatalf("2-cycle: got (0x%x, %d), want (0x40, 1)", pc, retired)
	}
}

func TestIE64ChaseStaticJumps_UnmappedStops(t *testing.T) {
	words := map[uint64]uint64{
		0x00: chaseWord(OP_BRA, 0, 0, 0, 0x40), // target 0x40 not present
	}
	pc, retired := ie64ChaseStaticJumps(0x00, chaseFetch(words))
	if pc != 0x40 || retired != 1 {
		t.Fatalf("unmapped: got (0x%x, %d), want (0x40, 1)", pc, retired)
	}
}

func TestIE64ChaseStaticJumps_NonJumpStartNoop(t *testing.T) {
	words := map[uint64]uint64{
		0x200: chaseWord(OP_ADD, 1, 2, 3, 0),
	}
	pc, retired := ie64ChaseStaticJumps(0x200, chaseFetch(words))
	if pc != 0x200 || retired != 0 {
		t.Fatalf("non-jump start: got (0x%x, %d), want (0x200, 0)", pc, retired)
	}
}

var _ = binary.LittleEndian
