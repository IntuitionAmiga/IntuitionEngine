// jit_m68k_blockhash_test.go - behavioral contract for the guest block
// byte-stamp hash used by the M68K JIT SMC guard. These tests pin the
// detection semantics (not the hash algorithm) so the implementation can
// change without weakening the guard.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func m68kBlockHashTestBlock(start, end uint64, ranges [][2]uint64) *JITBlock {
	return &JITBlock{startPC: start, endPC: end, coveredRanges: ranges}
}

func TestM68KBlockHash_StableOnUnchangedMemory(t *testing.T) {
	mem := make([]byte, 4096)
	for i := range mem {
		mem[i] = byte(i * 7)
	}
	block := m68kBlockHashTestBlock(0x100, 0x180, nil)
	m68kStampGuestBlockBytes(mem, block)
	if !block.guestHashValid {
		t.Fatal("stamp on in-bounds block must be valid")
	}
	for i := 0; i < 3; i++ {
		if !m68kGuestBlockBytesStillMatch(mem, block) {
			t.Fatalf("unchanged memory must match (iteration %d)", i)
		}
	}
}

func TestM68KBlockHash_DetectsEverySingleByteMutation(t *testing.T) {
	mem := make([]byte, 4096)
	for i := range mem {
		mem[i] = byte(i)
	}
	const start, end = 0x100, 0x180
	block := m68kBlockHashTestBlock(start, end, nil)
	m68kStampGuestBlockBytes(mem, block)
	for addr := start; addr < end; addr++ {
		orig := mem[addr]
		mem[addr] ^= 0xFF
		if m68kGuestBlockBytesStillMatch(mem, block) {
			t.Fatalf("mutation at %#x not detected", addr)
		}
		mem[addr] = orig
		if !m68kGuestBlockBytesStillMatch(mem, block) {
			t.Fatalf("restore at %#x must match again", addr)
		}
	}
}

func TestM68KBlockHash_MutationOutsideRangeIgnored(t *testing.T) {
	mem := make([]byte, 4096)
	block := m68kBlockHashTestBlock(0x100, 0x180, nil)
	m68kStampGuestBlockBytes(mem, block)
	mem[0x80] = 0xAA  // below range
	mem[0x200] = 0xBB // above range
	if !m68kGuestBlockBytesStillMatch(mem, block) {
		t.Fatal("mutation outside covered range must not invalidate")
	}
}

func TestM68KBlockHash_MultiRangeRegionDetection(t *testing.T) {
	mem := make([]byte, 4096)
	for i := range mem {
		mem[i] = byte(i >> 2)
	}
	ranges := [][2]uint64{{0x100, 0x140}, {0x300, 0x320}}
	block := m68kBlockHashTestBlock(0x100, 0x140, ranges)
	m68kStampGuestBlockBytes(mem, block)
	// Mutation in the second (non-contiguous) range must be caught.
	mem[0x310] ^= 0x01
	if m68kGuestBlockBytesStillMatch(mem, block) {
		t.Fatal("mutation in second covered range not detected")
	}
	mem[0x310] ^= 0x01
	// Gap between ranges is not covered.
	mem[0x200] = 0xCC
	if !m68kGuestBlockBytesStillMatch(mem, block) {
		t.Fatal("mutation in inter-range gap must not invalidate")
	}
}

func TestM68KBlockHash_BoundsAffectHash(t *testing.T) {
	// Same byte content, different covered bounds → different stamps, so a
	// block cannot alias another block's bytes at a shifted address.
	mem := make([]byte, 4096) // all zero: content identical everywhere
	a := m68kBlockHashTestBlock(0x100, 0x180, nil)
	b := m68kBlockHashTestBlock(0x200, 0x280, nil)
	ha, oka := m68kHashGuestBlockBytes(mem, a)
	hb, okb := m68kHashGuestBlockBytes(mem, b)
	if !oka || !okb {
		t.Fatal("both hashes must be computable")
	}
	if ha == hb {
		t.Fatal("identical bytes at different bounds must hash differently")
	}
}

func TestM68KBlockHash_OutOfBoundsInvalid(t *testing.T) {
	mem := make([]byte, 0x200)
	block := m68kBlockHashTestBlock(0x100, 0x300, nil) // end beyond memory
	if _, ok := m68kHashGuestBlockBytes(mem, block); ok {
		t.Fatal("out-of-bounds range must not hash")
	}
	m68kStampGuestBlockBytes(mem, block)
	if block.guestHashValid {
		t.Fatal("out-of-bounds stamp must be invalid")
	}
	// Invalid stamp means the guard is skipped (treated as match).
	if !m68kGuestBlockBytesStillMatch(mem, block) {
		t.Fatal("invalid stamp must skip the guard")
	}
}

func TestM68KBlockHash_ReversedRangeInvalid(t *testing.T) {
	mem := make([]byte, 4096)
	block := m68kBlockHashTestBlock(0x180, 0x100, nil) // endPC < startPC
	if _, ok := m68kHashGuestBlockBytes(mem, block); ok {
		t.Fatal("reversed range must not hash")
	}
}

// BenchmarkM68KBlockHash measures the per-dispatch SMC guard cost for a
// typical (64-byte) and large (1 KiB) block.
func BenchmarkM68KBlockHash_64B(b *testing.B) {
	mem := make([]byte, 1<<20)
	block := m68kBlockHashTestBlock(0x1000, 0x1040, nil)
	m68kStampGuestBlockBytes(mem, block)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !m68kGuestBlockBytesStillMatch(mem, block) {
			b.Fatal("must match")
		}
	}
}

func BenchmarkM68KBlockHash_1KiB(b *testing.B) {
	mem := make([]byte, 1<<20)
	block := m68kBlockHashTestBlock(0x1000, 0x1400, nil)
	m68kStampGuestBlockBytes(mem, block)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !m68kGuestBlockBytesStillMatch(mem, block) {
			b.Fatal("must match")
		}
	}
}
