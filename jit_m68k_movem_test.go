//go:build amd64 && (linux || windows || darwin)

package main

import (
	"testing"
	"unsafe"
)

// TestM68KMOVEM_L_RegToPredecSP exercises a function-prologue style save:
//
//	MOVEM.L D0/D1/A0,-(SP)
//
// Verifies SP decremented by 12 and memory reflects D0,D1,A0 in low-to-high
// order (predec encoding stores them in canonical D0..A7 order in memory).
func TestM68KMOVEM_L_RegToPredecSP(t *testing.T) {
	r := newM68KJITTestRig(t)

	r.cpu.DataRegs[0] = 0xAAAAAAAA
	r.cpu.DataRegs[1] = 0xBBBBBBBB
	r.cpu.AddrRegs[0] = 0xCCCCCCCC
	r.cpu.AddrRegs[7] = 0x10000

	startPC := uint32(0x1000)
	// MOVEM.L D0/D1/A0,-(SP) — opcode 0x48E7 (size=L, dir=r-to-m, mode=4, reg=A7=7).
	// Predec mask: bit 15=D0, bit 14=D1, bit 7=A0 → mask = 0x8000|0x4000|0x0080 = 0xC080.
	r.compileAndRun(t, startPC, 0x48E7, 0xC080)

	if got := r.cpu.AddrRegs[7]; got != 0x10000-12 {
		t.Errorf("SP = 0x%X, want 0x%X", got, 0x10000-12)
	}
	// Memory order (low to high): D0, D1, A0.
	check := func(addr uint32, want uint32, label string) {
		t.Helper()
		got := uint32(r.cpu.memory[addr])<<24 | uint32(r.cpu.memory[addr+1])<<16 |
			uint32(r.cpu.memory[addr+2])<<8 | uint32(r.cpu.memory[addr+3])
		if got != want {
			t.Errorf("[%s] mem[0x%X] = 0x%X, want 0x%X", label, addr, got, want)
		}
	}
	base := r.cpu.AddrRegs[7]
	check(base+0, 0xAAAAAAAA, "D0")
	check(base+4, 0xBBBBBBBB, "D1")
	check(base+8, 0xCCCCCCCC, "A0")
}

// TestM68KMOVEM_L_PostincSPToReg exercises a function-epilogue style restore:
//
//	MOVEM.L (SP)+,D0/D1/A0
//
// Verifies registers receive their values and SP advances by 12.
func TestM68KMOVEM_L_PostincSPToReg(t *testing.T) {
	r := newM68KJITTestRig(t)

	r.cpu.AddrRegs[7] = 0x4000
	// Pre-populate stack with the values to restore.
	r.writeLong(0x4000, 0x11111111)
	r.writeLong(0x4004, 0x22222222)
	r.writeLong(0x4008, 0x33333333)

	startPC := uint32(0x1000)
	// MOVEM.L (SP)+,D0/D1/A0 — opcode 0x4CDF (size=L, dir=m-to-r, mode=3, reg=A7=7).
	// Postinc mask: bit 0=D0, bit 1=D1, bit 8=A0 → mask = 0x0103.
	r.compileAndRun(t, startPC, 0x4CDF, 0x0103)

	if got := r.cpu.DataRegs[0]; got != 0x11111111 {
		t.Errorf("D0 = 0x%X, want 0x11111111", got)
	}
	if got := r.cpu.DataRegs[1]; got != 0x22222222 {
		t.Errorf("D1 = 0x%X, want 0x22222222", got)
	}
	if got := r.cpu.AddrRegs[0]; got != 0x33333333 {
		t.Errorf("A0 = 0x%X, want 0x33333333", got)
	}
	if got := r.cpu.AddrRegs[7]; got != 0x4000+12 {
		t.Errorf("SP = 0x%X, want 0x%X", got, 0x4000+12)
	}
}

// TestM68KMOVEM_W_PostincSign verifies word-sized MOVEM (m-to-r) sign-extends
// the loaded value to 32 bits, both for data and address registers.
func TestM68KMOVEM_W_PostincSign(t *testing.T) {
	r := newM68KJITTestRig(t)

	r.cpu.AddrRegs[7] = 0x4000
	// Negative word values for sign-extension test.
	r.writeWord(0x4000, 0xFFFE) // -2
	r.writeWord(0x4002, 0x8000) // -32768

	startPC := uint32(0x1000)
	// MOVEM.W (SP)+,D0/A0 — opcode 0x4C9F (size=W, dir=m-to-r, mode=3, reg=7).
	// Mask: bit 0=D0, bit 8=A0 → 0x0101.
	r.compileAndRun(t, startPC, 0x4C9F, 0x0101)

	if got := r.cpu.DataRegs[0]; got != 0xFFFFFFFE {
		t.Errorf("D0 = 0x%X, want 0xFFFFFFFE (sign-extended -2)", got)
	}
	if got := r.cpu.AddrRegs[0]; got != 0xFFFF8000 {
		t.Errorf("A0 = 0x%X, want 0xFFFF8000 (sign-extended -32768)", got)
	}
	if got := r.cpu.AddrRegs[7]; got != 0x4000+4 {
		t.Errorf("SP = 0x%X, want 0x%X", got, 0x4000+4)
	}
}

// TestM68KMOVEM_RoundTrip exercises save+restore through native code,
// confirming the prolog/epilog combo round-trips bit-exact.
func TestM68KMOVEM_RoundTrip(t *testing.T) {
	r := newM68KJITTestRig(t)

	d0, d1, a0, a1 := uint32(0xDEADBEEF), uint32(0xCAFEBABE), uint32(0x12345678), uint32(0x9ABCDEF0)
	r.cpu.DataRegs[0] = d0
	r.cpu.DataRegs[1] = d1
	r.cpu.AddrRegs[0] = a0
	r.cpu.AddrRegs[1] = a1
	r.cpu.AddrRegs[7] = 0x10000

	startPC := uint32(0x1000)
	// MOVEM.L D0/D1/A0/A1,-(SP)  followed by clobber and  MOVEM.L (SP)+,D0/D1/A0/A1
	// Predec mask: D0=bit15, D1=bit14, A0=bit7, A1=bit6 → 0xC0C0.
	// Clobber regs in between to confirm restore works.
	// MOVEQ #0,D0
	// MOVEQ #0,D1
	// We need the second MOVEM. compileAndRun appends TRAP #0 terminator
	// but compiles preceding instructions as one block.
	r.compileAndRun(t, startPC,
		0x48E7, 0xC0C0, // MOVEM.L D0/D1/A0/A1,-(SP)
		0x7000,         // MOVEQ #0,D0
		0x7200,         // MOVEQ #0,D1
		0x4CDF, 0x0303, // MOVEM.L (SP)+,D0/D1/A0/A1 (mask: D0 b0, D1 b1, A0 b8, A1 b9)
	)

	if r.cpu.DataRegs[0] != d0 {
		t.Errorf("D0 = 0x%X, want 0x%X", r.cpu.DataRegs[0], d0)
	}
	if r.cpu.DataRegs[1] != d1 {
		t.Errorf("D1 = 0x%X, want 0x%X", r.cpu.DataRegs[1], d1)
	}
	if r.cpu.AddrRegs[0] != a0 {
		t.Errorf("A0 = 0x%X, want 0x%X", r.cpu.AddrRegs[0], a0)
	}
	if r.cpu.AddrRegs[1] != a1 {
		t.Errorf("A1 = 0x%X, want 0x%X", r.cpu.AddrRegs[1], a1)
	}
	if r.cpu.AddrRegs[7] != 0x10000 {
		t.Errorf("SP = 0x%X, want 0x10000 (round-trip)", r.cpu.AddrRegs[7])
	}
}

var _ = unsafe.Sizeof(int(0)) // keep unsafe import for parity

// TestM68KMOVEM_L_PredecAnInMask covers the spec-mandated case where the
// destination address register (-(An)) is itself in the move mask. The
// IE M68K interpreter decrements An before each individual write, so the
// value stored at An's own slot is the post-decrement An — which equals
// the address being written. The JIT predec fast path mirrors that by
// materializing R10+offset directly when reg == eaReg.
//
// Concrete program: MOVEM.L A7,-(A7) with A7 = 0x10000.
//   - Final A7 = 0x0FFFC.
//   - mem[0x0FFFC..0x0FFFF] (big-endian) = 0x0FFFC (the decremented A7).
func TestM68KMOVEM_L_PredecAnInMask(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.AddrRegs[7] = 0x10000

	startPC := uint32(0x1000)
	// MOVEM.L A7,-(A7) — opcode 0x48E7 (size=L, dir=r-to-m, mode=4, reg=7).
	// Predec mask: A7 corresponds to bit 0 → mask = 0x0001.
	r.compileAndRun(t, startPC, 0x48E7, 0x0001)

	if got := r.cpu.AddrRegs[7]; got != 0x10000-4 {
		t.Errorf("A7 = 0x%X, want 0x%X", got, 0x10000-4)
	}
	addr := r.cpu.AddrRegs[7]
	got := uint32(r.cpu.memory[addr])<<24 | uint32(r.cpu.memory[addr+1])<<16 |
		uint32(r.cpu.memory[addr+2])<<8 | uint32(r.cpu.memory[addr+3])
	if got != 0x10000-4 {
		t.Errorf("mem[0x%X] = 0x%X, want 0x%X (decremented A7)", addr, got, 0x10000-4)
	}
}

// TestM68KMOVEM_L_PredecAnInMaskMixed exercises the same An-in-mask path
// alongside other registers, so the An slot's stored value depends on
// which slot it occupies relative to the predecrement walk.
//
// Program: MOVEM.L A0/A7,-(A7) with A0 = 0xCAFEBABE, A7 = 0x10000.
//
//	Predec mask: A7=bit0, A0=bit7. mask = 0x0081.
//	Memory layout (low → high) follows the canonical D0..A7 order, so the
//	slots are: A0 at A7-8, A7 at A7-4. Per interpreter (decrement-before-
//	each-write): A7 decremented twice total (once per slot), final A7 =
//	0x0FFF8. Slot-0 (A0) stored at 0x0FFF8 = A0's value 0xCAFEBABE.
//	Slot-1 (A7) stored at 0x0FFFC = post-decrement A7 at that iter, which
//	per the iter-↔-slot mapping equals R10+offset = 0x0FFFC.
func TestM68KMOVEM_L_PredecAnInMaskMixed(t *testing.T) {
	r := newM68KJITTestRig(t)
	r.cpu.AddrRegs[0] = 0xCAFEBABE
	r.cpu.AddrRegs[7] = 0x10000

	startPC := uint32(0x1000)
	// MOVEM.L A0/A7,-(A7).
	r.compileAndRun(t, startPC, 0x48E7, 0x0081)

	if got := r.cpu.AddrRegs[7]; got != 0x10000-8 {
		t.Errorf("A7 = 0x%X, want 0x%X", got, 0x10000-8)
	}
	read := func(addr uint32) uint32 {
		return uint32(r.cpu.memory[addr])<<24 | uint32(r.cpu.memory[addr+1])<<16 |
			uint32(r.cpu.memory[addr+2])<<8 | uint32(r.cpu.memory[addr+3])
	}
	if got := read(0x10000 - 8); got != 0xCAFEBABE {
		t.Errorf("mem[A7-8] = 0x%X, want 0xCAFEBABE (A0's slot)", got)
	}
	if got := read(0x10000 - 4); got != 0x10000-4 {
		t.Errorf("mem[A7-4] = 0x%X, want 0x%X (decremented A7's slot)", got, 0x10000-4)
	}
}
