// jit_m68k_const_addr.go - Compile-time constant-address proof for the
// M68020 JIT (milestone 7 optimisation slice, the M68020 analogue of
// ie64ConstLowRAMAccess).
//
// Absolute short/long and (d16,PC) effective addresses are compile-time
// constants. When the whole access [addr, addr+size) provably lies inside
// guest RAM and touches no I/O page, the emitted access needs neither the
// RAM-bounds check nor the I/O page-bitmap probe. The proof is taken
// against a snapshot of the same inputs the runtime guards consult:
// ctx.MemSize is len(cpu.memory) (fixed for the CPU's lifetime) and the
// I/O page set is cpu.m68kJitIOPageBitmap, built once in initM68KJIT after
// the bus mappings are sealed (MapIO panics once execution has started),
// so the snapshot cannot go stale while compiled blocks are live. Pages at
// or beyond the end of the bitmap are plain RAM, mirroring the runtime
// guard's bounds branch.
//
// SMC detection is a separate concern and is never elided by this proof:
// constant-address stores keep their store-side SMC invalidation check.
//
// Architecture-neutral and untagged so every backend applies the identical
// proof.

package main

import "os"

// Kill switch: IE_M68K_JIT_DISABLE_CONST_ADDR=1 restores the full runtime
// guards on constant-address accesses.
var m68kJITConstAddrDisabled = os.Getenv("IE_M68K_JIT_DISABLE_CONST_ADDR") == "1"

// m68kConstAddrProof snapshots the guard inputs for one CPU's JIT session.
type m68kConstAddrProof struct {
	ioPageBitmap []bool // snapshot identity: cpu.m68kJitIOPageBitmap
	memSize      uint32 // len(cpu.memory), the runtime ctx.MemSize
}

// m68kNewConstAddrProof builds the proof context for a CPU whose JIT
// session has already snapshotted its I/O page bitmap.
func m68kNewConstAddrProof(cpu *M68KCPU) *m68kConstAddrProof {
	if m68kJITConstAddrDisabled {
		return nil
	}
	return &m68kConstAddrProof{
		ioPageBitmap: cpu.m68kJitIOPageBitmap,
		memSize:      uint32(len(cpu.memory)),
	}
}

// DirectRAM reports whether the constant access [addr, addr+sizeBytes) is
// provably a plain-RAM access: fully inside guest memory and touching no
// I/O page. When true, the RAM-bounds check and the I/O bitmap probe can
// be elided at compile time.
func (p *m68kConstAddrProof) DirectRAM(addr uint32, sizeBytes uint32) bool {
	if p == nil || sizeBytes == 0 {
		return false
	}
	end := uint64(addr) + uint64(sizeBytes)
	if end > uint64(p.memSize) {
		return false
	}
	lastPage := uint32((end - 1) >> 8)
	for page := addr >> 8; ; page++ {
		if page < uint32(len(p.ioPageBitmap)) && p.ioPageBitmap[page] {
			return false
		}
		if page == lastPage {
			return true
		}
	}
}
