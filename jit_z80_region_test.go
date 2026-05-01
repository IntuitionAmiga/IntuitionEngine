// jit_z80_region_test.go - tests for the Z80 region scanner
// (Phase 4 sub-phase B.3.a: pure memory-driven walker that follows
// JP nn / JR e and stops on conditional or indirect terminators).
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
)

// putZ80JPnn writes JP nn (0xC3, 3 bytes) at off targeting target.
func putZ80JPnn(mem []byte, off uint16, target uint16) {
	mem[off] = 0xC3
	mem[off+1] = byte(target & 0xFF)
	mem[off+2] = byte(target >> 8)
}

// putZ80JRe writes JR e (0x18, 2 bytes) at off. Disp encoded relative
// to instrPC + 2 (Z80 spec).
func putZ80JRe(mem []byte, off uint16, target uint16) {
	disp := int16(target) - int16(off+2)
	mem[off] = 0x18
	mem[off+1] = byte(int8(disp))
}

// putZ80NOP writes a NOP (0x00, 1 byte).
func putZ80NOP(mem []byte, off uint16) {
	mem[off] = 0x00
}

// putZ80RET writes RET (0xC9, 1 byte).
func putZ80RET(mem []byte, off uint16) {
	mem[off] = 0xC9
}

// putZ80JPHL writes JP (HL) (0xE9, 1 byte) — indirect terminator.
func putZ80JPHL(mem []byte, off uint16) {
	mem[off] = 0xE9
}

// putZ80JPNZnn writes JP NZ,nn (0xC2, 3 bytes) — conditional, has two
// successors, scanner must refuse.
func putZ80JPNZnn(mem []byte, off uint16, target uint16) {
	mem[off] = 0xC2
	mem[off+1] = byte(target & 0xFF)
	mem[off+2] = byte(target >> 8)
}

// TestScanRegionZ80_FollowsJPnnChain stitches three blocks via JP nn.
func TestScanRegionZ80_FollowsJPnnChain(t *testing.T) {
	mem := make([]byte, 0x1000)
	putZ80NOP(mem, 0x100)
	putZ80JPnn(mem, 0x101, 0x200)
	putZ80NOP(mem, 0x200)
	putZ80JPnn(mem, 0x201, 0x300)
	putZ80NOP(mem, 0x300)
	putZ80RET(mem, 0x301)

	res := ScanRegionZ80(mem, 0x100)
	if got := len(res.BlockPCs); got != 3 {
		t.Fatalf("BlockPCs len = %d want 3 (%v)", got, res.BlockPCs)
	}
	want := []uint32{0x100, 0x200, 0x300}
	for i, pc := range want {
		if res.BlockPCs[i] != pc {
			t.Errorf("BlockPCs[%d] = %X want %X", i, res.BlockPCs[i], pc)
		}
	}
}

// TestScanRegionZ80_FollowsJRe stitches two blocks via JR e.
func TestScanRegionZ80_FollowsJRe(t *testing.T) {
	mem := make([]byte, 0x1000)
	putZ80NOP(mem, 0x100)
	putZ80JRe(mem, 0x101, 0x110)
	putZ80NOP(mem, 0x110)
	putZ80RET(mem, 0x111)

	res := ScanRegionZ80(mem, 0x100)
	if got := len(res.BlockPCs); got != 2 || res.BlockPCs[0] != 0x100 || res.BlockPCs[1] != 0x110 {
		t.Errorf("expected [100, 110], got %v", res.BlockPCs)
	}
}

// TestScanRegionZ80_StopsOnRET halts cleanly at RET-only block.
func TestScanRegionZ80_StopsOnRET(t *testing.T) {
	mem := make([]byte, 0x1000)
	putZ80JPnn(mem, 0x100, 0x200)
	putZ80RET(mem, 0x200)

	res := ScanRegionZ80(mem, 0x100)
	if got := len(res.BlockPCs); got != 2 {
		t.Fatalf("BlockPCs len = %d want 2 (%v)", got, res.BlockPCs)
	}
}

// TestScanRegionZ80_RejectsConditional refuses to follow JP NZ,nn
// — conditional terminators have two successors.
func TestScanRegionZ80_RejectsConditional(t *testing.T) {
	mem := make([]byte, 0x1000)
	putZ80JPNZnn(mem, 0x100, 0x200)
	putZ80RET(mem, 0x200)

	res := ScanRegionZ80(mem, 0x100)
	// Single block (just the JP NZ,nn) does not form a region.
	if len(res.BlockPCs) != 0 {
		t.Errorf("expected nil region for conditional terminator, got %v", res.BlockPCs)
	}
}

// TestScanRegionZ80_RejectsJPHL refuses to follow indirect JP (HL).
func TestScanRegionZ80_RejectsJPHL(t *testing.T) {
	mem := make([]byte, 0x1000)
	putZ80JPHL(mem, 0x100)
	putZ80RET(mem, 0x101)

	res := ScanRegionZ80(mem, 0x100)
	if len(res.BlockPCs) != 0 {
		t.Errorf("expected nil region for JP (HL), got %v", res.BlockPCs)
	}
}

// TestScanRegionZ80_DetectsBackEdge stops cleanly on a back-edge.
func TestScanRegionZ80_DetectsBackEdge(t *testing.T) {
	mem := make([]byte, 0x1000)
	putZ80NOP(mem, 0x100)
	putZ80JPnn(mem, 0x101, 0x200)
	putZ80NOP(mem, 0x200)
	putZ80JPnn(mem, 0x201, 0x100) // back to 0x100

	res := ScanRegionZ80(mem, 0x100)
	if got := len(res.BlockPCs); got != 2 {
		t.Errorf("expected 2 blocks (back-edge stop), got %d (%v)", got, res.BlockPCs)
	}
}

// TestScanRegionZ80_RejectsOutOfRangeStartPC guards against the
// uint32 → uint16 truncation aliasing high-range starts into low
// memory. startPC == 0x10000 with a 64 KiB memory must return nil
// rather than scanning from 0x0000.
func TestScanRegionZ80_RejectsOutOfRangeStartPC(t *testing.T) {
	mem := make([]byte, 0x10000)
	// Lay down a valid 2-block region at PC 0x0000 that WOULD form
	// a region if the bad cast aliased 0x10000 → 0x0000.
	putZ80NOP(mem, 0x0000)
	putZ80JPnn(mem, 0x0001, 0x0010)
	putZ80NOP(mem, 0x0010)
	putZ80RET(mem, 0x0011)

	res := ScanRegionZ80(mem, 0x10000)
	if res.BlockPCs != nil {
		t.Errorf("expected nil region for out-of-range startPC=0x10000, got %v", res.BlockPCs)
	}
}

// TestScanRegionZ80_SingleBlockRejected matches the form-region
// single-block rejection contract: walker returns nil.
func TestScanRegionZ80_SingleBlockRejected(t *testing.T) {
	mem := make([]byte, 0x1000)
	putZ80RET(mem, 0x100)

	res := ScanRegionZ80(mem, 0x100)
	if res.BlockPCs != nil {
		t.Errorf("expected nil region for single-block scan, got %v", res.BlockPCs)
	}
}
