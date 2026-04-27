// mmu_ie64_phase4a_test.go - PLAN_MAX_RAM.md slice 4a acceptance tests:
// PTE/TLB types must accept widened (uint64) PPN/VPN. The legacy 16-bit
// surface caps physical RAM at 256 MiB (uint16 PPN * 4 KiB) which is far
// below the IE64 large-memory model. These tests pin the new contract:
//
//   - makePTE accepts a uint64 PPN > 0xFFFF and round-trips through parsePTE.
//   - parsePTE returns a uint64 PPN.
//   - PTE_PPN_MASK accommodates at least 51 bits (PTE bits 13..63).
//   - TLBEntry.vpn / TLBEntry.ppn store uint64.
//   - tlbInsert / tlbLookup / tlbInvalidate accept uint64 VPNs.
//
// Tests written before implementation of slice 4a; once mmu_ie64.go widens
// the surface, both these and the existing PTE/TLB tests must remain green.
package main

import (
	"testing"
	"unsafe"
)

func TestPhase4a_PTERoundTripWidePPN(t *testing.T) {
	// Pick PPNs that overflow uint16 / uint32 so a stale narrow type would
	// silently truncate. 0x100000 is the first PPN that does not fit in 16
	// bits; 0x10_0000_0000 covers a TiB-class physical page and exercises
	// the uint64 path past 32-bit truncation.
	cases := []struct {
		name string
		ppn  uint64
	}{
		{"just_above_uint16", 0x10000},
		{"above_uint16_with_set_high_bits", 0x12345},
		{"large_36bit", 0x10_0000_0000}, // beyond uint32
		{"max_52bit", (uint64(1) << 52) - 1},
	}
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pte := makePTE(tc.ppn, flags)
			gotPPN, gotFlags := parsePTE(pte)
			if gotPPN != tc.ppn {
				t.Fatalf("parsePTE ppn = 0x%X, want 0x%X (pte=0x%016X)", gotPPN, tc.ppn, pte)
			}
			if gotFlags != flags {
				t.Fatalf("parsePTE flags = 0x%02X, want 0x%02X", gotFlags, flags)
			}
		})
	}
}

func TestPhase4a_PTEFlagBitsNotClobberedByWidePPN(t *testing.T) {
	// Widened PPNs must not collide with permission/A/D bits (bits 0..6)
	// or the reserved gap (bits 7..11).
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U | PTE_A | PTE_D)
	pte := makePTE((uint64(1)<<52)-1, flags)
	if pte&0x7F != uint64(flags) {
		t.Fatalf("flag bits clobbered: pte=0x%016X flags=0x%02X", pte, flags)
	}
}

func TestPhase4a_PTEPPNMaskIs52Bits(t *testing.T) {
	const want uint64 = (uint64(1) << 52) - 1
	if uint64(PTE_PPN_MASK) != want {
		t.Fatalf("PTE_PPN_MASK = 0x%X, want 0x%X (52-bit PPN field)", uint64(PTE_PPN_MASK), want)
	}
	if PTE_PPN_BITS != 52 {
		t.Fatalf("PTE_PPN_BITS = %d, want 52", PTE_PPN_BITS)
	}
	if PTE_PPN_SHIFT != 12 {
		t.Fatalf("PTE_PPN_SHIFT = %d, want 12 (PPN at PTE bits 12..63)", PTE_PPN_SHIFT)
	}
	if IE64_PHYS_ADDR_BITS != 64 {
		t.Fatalf("IE64_PHYS_ADDR_BITS = %d, want 64 (52-bit PPN + 12-bit offset)", IE64_PHYS_ADDR_BITS)
	}
}

func TestPhase4a_VirtualAddressCeilingIsFullUint64(t *testing.T) {
	// Architectural decision: full 64-bit VA, no reserved high bits,
	// IE64_VIRT_ADDR_MAX == ^uint64(0). The walk masking must therefore
	// preserve all 52 VPN bits; bit 63 in vaddr maps to a distinct VPN
	// from bit 0.
	if IE64_VIRT_ADDR_MAX != ^uint64(0) {
		t.Fatalf("IE64_VIRT_ADDR_MAX = 0x%X, want 0x%X (full 64-bit VA)", IE64_VIRT_ADDR_MAX, ^uint64(0))
	}
	high := uint64(1) << 63
	low := uint64(0)
	if (high>>MMU_PAGE_SHIFT)&PTE_PPN_MASK == (low>>MMU_PAGE_SHIFT)&PTE_PPN_MASK {
		t.Fatalf("vaddr 0x%X aliases to same VPN as vaddr 0; mask is too narrow", high)
	}
}

func TestPhase4a_TLBEntryFieldsAreUint64(t *testing.T) {
	var e TLBEntry
	if got := unsafe.Sizeof(e.vpn); got < unsafe.Sizeof(uint64(0)) {
		t.Fatalf("TLBEntry.vpn size = %d, want >= %d (uint64)", got, unsafe.Sizeof(uint64(0)))
	}
	if got := unsafe.Sizeof(e.ppn); got < unsafe.Sizeof(uint64(0)) {
		t.Fatalf("TLBEntry.ppn size = %d, want >= %d (uint64)", got, unsafe.Sizeof(uint64(0)))
	}
}

func TestPhase4a_TLBInsertLookupWideVPN(t *testing.T) {
	rig := newIE64TestRig()
	cpu := rig.cpu

	cases := []struct {
		name     string
		vpn, ppn uint64
	}{
		{"low_vpn", 5, 10},
		{"vpn_just_above_uint16", 0x10000, 0x10001},
		{"vpn_at_36bit", 0x10_0000_0000, 0x10_0000_0001},
		{"vpn_high_52bit_minus_1", (uint64(1) << 52) - 2, (uint64(1) << 52) - 1},
	}
	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cpu.tlbFlush()
			cpu.tlbInsert(tc.vpn, tc.ppn, 0, flags)
			entry, hit := cpu.tlbLookup(tc.vpn)
			if !hit {
				t.Fatalf("tlbLookup(0x%X) miss after insert", tc.vpn)
			}
			if entry.vpn != tc.vpn {
				t.Fatalf("entry.vpn = 0x%X, want 0x%X", entry.vpn, tc.vpn)
			}
			if entry.ppn != tc.ppn {
				t.Fatalf("entry.ppn = 0x%X, want 0x%X", entry.ppn, tc.ppn)
			}
			if entry.flags != flags {
				t.Fatalf("entry.flags = 0x%02X, want 0x%02X", entry.flags, flags)
			}

			cpu.tlbInvalidate(tc.vpn)
			if _, hit := cpu.tlbLookup(tc.vpn); hit {
				t.Fatalf("tlbLookup(0x%X) hit after invalidate", tc.vpn)
			}
		})
	}
}

func TestPhase4a_TLBLookupRejectsAliasOnSameSlot(t *testing.T) {
	// Direct-mapped TLB indexes by low bits of VPN. Two VPNs that share
	// the same low index but differ above the uint16 ceiling must NOT
	// false-hit on each other's slot. With a stale uint16 vpn key, an
	// insert of `low` would alias to a lookup of `high` because the
	// truncated keys would match. The widened uint64 key is what
	// rejects the alias.
	rig := newIE64TestRig()
	cpu := rig.cpu
	cpu.tlbFlush()

	flags := byte(PTE_P | PTE_R | PTE_W | PTE_X | PTE_U)
	low := uint64(7)
	high := uint64(7) | (uint64(1) << 32) // same low 16 bits, different high bits

	cpu.tlbInsert(low, 100, 0, flags)
	if entry, hit := cpu.tlbLookup(high); hit {
		t.Fatalf("false-positive: lookup(high=0x%X) hit on slot inserted with low=0x%X (entry.vpn=0x%X ppn=%d)",
			high, low, entry.vpn, entry.ppn)
	}
	if _, hit := cpu.tlbLookup(low); !hit {
		t.Fatalf("lookup(low=0x%X) miss after insert", low)
	}
}
