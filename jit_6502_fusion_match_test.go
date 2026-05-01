// jit_6502_fusion_match_test.go - tests for Phase 7a fusion matcher.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func TestMatchFusionAtPC_Universal2Byte(t *testing.T) {
	mem := make([]byte, 0x10000)
	cases := []struct {
		name string
		pc   uint16
		seed []byte
		want FusionID
	}{
		{"DexBne", 0x100, []byte{0xCA, 0xD0, 0xFE}, FusionDexBne},
		{"DeyBne", 0x200, []byte{0x88, 0xD0, 0xFE}, FusionDeyBne},
		{"InxBne", 0x300, []byte{0xE8, 0xD0, 0xFE}, FusionInxBne},
		{"LdaImmStaZp", 0x400, []byte{0xA9, 0x42, 0x85, 0x10}, FusionLdaImmStaZp},
		{"NoMatchDexNonBne", 0x500, []byte{0xCA, 0xEA}, FusionNone},
		{"NoMatchLdaNonSta", 0x600, []byte{0xA9, 0x42, 0xAA}, FusionNone},
	}
	for _, c := range cases {
		copy(mem[c.pc:], c.seed)
		if got := MatchFusionAtPC(mem, c.pc); got != c.want {
			t.Errorf("%s: MatchFusionAtPC=%d want %d", c.name, got, c.want)
		}
	}
}

func TestFuse6502DexBne_TakenAndFallthrough(t *testing.T) {
	// X=2: branch taken (X→1, BNE follows). DEX(1B)+BNE(2B)=3B idiom;
	// taken target is (pc+3)+rel.
	x, sr, pc := fuse6502DexBne(2, 0x00, 0x1000, 0x10)
	if x != 1 || pc != 0x1000+3+0x10 {
		t.Errorf("taken: x=%d pc=%04X", x, pc)
	}
	if sr&0x02 != 0 {
		t.Errorf("taken: Z should be clear, sr=%02X", sr)
	}
	// X=1: branch not taken (X→0, BNE falls through). Resume PC = pc+3.
	x, sr, pc = fuse6502DexBne(1, 0x00, 0x1000, 0x10)
	if x != 0 || pc != 0x1003 {
		t.Errorf("fallthrough: x=%d pc=%04X", x, pc)
	}
	if sr&0x02 == 0 {
		t.Errorf("fallthrough: Z should be set, sr=%02X", sr)
	}
}

// TestFuse6502BneFamily_PCArithmetic locks the +3 advance against
// regression. DEX/DEY/INX (1B) + BNE rel (2B) = 3B idiom. The 6502
// branch target is computed from the post-idiom PC, so taken paths
// land at pc+3+rel and fallthrough lands at pc+3.
func TestFuse6502BneFamily_PCArithmetic(t *testing.T) {
	const start uint16 = 0x2000
	// Forward taken target.
	if _, _, got := fuse6502DexBne(5, 0, start, 0x10); got != start+3+0x10 {
		t.Errorf("DexBne taken fwd: got %04X want %04X", got, start+3+0x10)
	}
	if _, _, got := fuse6502DeyBne(5, 0, start, 0x10); got != start+3+0x10 {
		t.Errorf("DeyBne taken fwd: got %04X want %04X", got, start+3+0x10)
	}
	if _, _, got := fuse6502InxBne(5, 0, start, 0x10); got != start+3+0x10 {
		t.Errorf("InxBne taken fwd: got %04X want %04X", got, start+3+0x10)
	}
	// Backward taken target (rel = -3 → resumes at start). int32 math
	// preserves sign extension; final uint16 wrap is the 6502 behavior.
	if _, _, got := fuse6502DexBne(5, 0, start, -3); got != start {
		t.Errorf("DexBne taken back: got %04X want %04X", got, start)
	}
	// Fallthrough = pc+3.
	if _, _, got := fuse6502DexBne(1, 0, start, 0x10); got != start+3 {
		t.Errorf("DexBne fall: got %04X want %04X", got, start+3)
	}
	if _, _, got := fuse6502DeyBne(1, 0, start, 0x10); got != start+3 {
		t.Errorf("DeyBne fall: got %04X want %04X", got, start+3)
	}
	if _, _, got := fuse6502InxBne(0xFF, 0, start, 0x10); got != start+3 {
		t.Errorf("InxBne fall: got %04X want %04X", got, start+3)
	}
}

func TestFuse6502LdaImmStaZp_StoresAndFlags(t *testing.T) {
	mem := make([]byte, 256)
	a, sr := fuse6502LdaImmStaZp(mem, 0x00, 0x80, 0x10)
	if a != 0x80 {
		t.Errorf("a=%02X want 80", a)
	}
	if mem[0x10] != 0x80 {
		t.Errorf("mem[0x10]=%02X want 80", mem[0x10])
	}
	if sr&0x80 == 0 {
		t.Errorf("N should be set on negative immediate, sr=%02X", sr)
	}
	a, sr = fuse6502LdaImmStaZp(mem, 0x00, 0x00, 0x20)
	if sr&0x02 == 0 {
		t.Errorf("Z should be set on zero immediate, sr=%02X", sr)
	}
	_ = a
}
