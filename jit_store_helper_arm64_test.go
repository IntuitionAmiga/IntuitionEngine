// jit_store_helper_arm64_test.go — Phase 5 cycle 5.5: ARM64 STORE helper-exit.

//go:build arm64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"fmt"
	"testing"
	"unsafe"
)

func assertSTOREHelperFields(t *testing.T, ctx *JITContext, wantAddr uint64, wantVal uint64, wantSize uint32, wantPC uint64, wantSP uint64) {
	t.Helper()
	if ctx.NeedHelper != HELPER_STORE {
		t.Fatalf("NeedHelper = %d, want HELPER_STORE (%d)", ctx.NeedHelper, HELPER_STORE)
	}
	if ctx.NeedIOFallback != 0 {
		t.Fatalf("NeedIOFallback = %d, want 0", ctx.NeedIOFallback)
	}
	if ctx.HelperAddr != wantAddr {
		t.Fatalf("HelperAddr = 0x%016X, want 0x%016X", ctx.HelperAddr, wantAddr)
	}
	if ctx.HelperVal != wantVal {
		t.Fatalf("HelperVal = 0x%016X, want 0x%016X", ctx.HelperVal, wantVal)
	}
	if ctx.HelperSize != wantSize {
		t.Fatalf("HelperSize = %d, want %d", ctx.HelperSize, wantSize)
	}
	if ctx.HelperPC != wantPC {
		t.Fatalf("HelperPC = 0x%016X, want 0x%016X", ctx.HelperPC, wantPC)
	}
	if ctx.LiveSP != wantSP {
		t.Fatalf("LiveSP = 0x%016X, want 0x%016X", ctx.LiveSP, wantSP)
	}
}

func TestJIT_ARM64_STORE_MicroTLBHitPhysicalCodeAliasInvalidates(t *testing.T) {
	r := newJITTestRig(t)
	const virt = uint64(0x4A8)
	const phys = uint64(0x7000)
	r.cpu.mmuEnabled = true
	r.ctx.MMUEnabled = 1
	r.ctx.refreshMicroTLBPrefixes(r.cpu)
	idx := ie64MicroTLBIndex(ie64MicroTLBSet(virt), 0)
	r.ctx.MicroTLBKeys[idx] = ie64MicroTLBKey(r.cpu, virt, ACCESS_WRITE)
	r.ctx.MicroTLBPhys[idx] = phys
	physicalCode := make([]byte, 0x80)
	physicalCode[(phys+(virt&MMU_PAGE_MASK))>>8] = 1
	r.ctx.PhysCodeBitmapPtr = uintptr(unsafe.Pointer(&physicalCode[0]))
	r.ctx.PhysCodeBitmapLen = uint32(len(physicalCode))
	r.cpu.regs[1] = 0xBEEF
	r.cpu.regs[2] = virt

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedInval != 1 || r.ctx.InvalSize != 0 {
		t.Fatalf("physical alias invalidation = need %d size %d, want need 1 full range", r.ctx.NeedInval, r.ctx.InvalSize)
	}
}

func TestJIT_ARM64_STORE_CrossPageVirtualCodeInvalidates(t *testing.T) {
	r := newJITTestRig(t)
	code := make([]byte, 2)
	code[1] = 1
	r.ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&code[0]))
	r.ctx.CodePageBitmapLen = uint32(len(code))
	r.cpu.regs[1] = 0x1122334455667788
	r.cpu.regs[2] = 0xFF

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedInval != 1 || r.ctx.InvalAddr != 0xFF || r.ctx.InvalSize != 8 {
		t.Fatalf("virtual cross-page invalidation = need %d addr %#x size %d", r.ctx.NeedInval, r.ctx.InvalAddr, r.ctx.InvalSize)
	}
}

func TestJIT_ARM64_STORE_MicroTLBHitCrossPagePhysicalCodeInvalidates(t *testing.T) {
	r := newJITTestRig(t)
	const virt, phys = uint64(0xFF), uint64(0x7000)
	r.cpu.mmuEnabled, r.ctx.MMUEnabled = true, 1
	r.ctx.refreshMicroTLBPrefixes(r.cpu)
	idx := ie64MicroTLBIndex(ie64MicroTLBSet(virt), 0)
	r.ctx.MicroTLBKeys[idx] = ie64MicroTLBKey(r.cpu, virt, ACCESS_WRITE)
	r.ctx.MicroTLBPhys[idx] = phys
	code := make([]byte, 0x80)
	code[(phys+0x100)>>8] = 1
	r.ctx.PhysCodeBitmapPtr = uintptr(unsafe.Pointer(&code[0]))
	r.ctx.PhysCodeBitmapLen = uint32(len(code))
	r.cpu.regs[1], r.cpu.regs[2] = 0x1122334455667788, virt

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	if r.ctx.NeedInval != 1 || r.ctx.InvalSize != 0 {
		t.Fatalf("physical cross-page invalidation = need %d size %d", r.ctx.NeedInval, r.ctx.InvalSize)
	}
}

func TestJIT_ARM64_STORE_HighAddr_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const highAddr uint64 = 0x0000_0001_0000_8000
	const payload uint64 = 0xDEADBEEFCAFEBABE
	const sentinelSP uint64 = 0xCAFE_BABE_F00D_F00D
	r.cpu.regs[1] = payload
	r.cpu.regs[2] = highAddr
	r.cpu.regs[31] = sentinelSP
	r.ctx.NeedHelper = HELPER_NONE

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	assertSTOREHelperFields(t, r.ctx, highAddr, payload, uint32(IE64_SIZE_Q), PROG_START, sentinelSP)
}

func TestJIT_ARM64_STORE_MMUEnabled_SetsHelper(t *testing.T) {
	r := newJITTestRig(t)
	const lowAddr uint64 = 0x4000
	const payload uint64 = 0x1122334455667788
	r.cpu.regs[1] = payload
	r.cpu.regs[2] = lowAddr
	r.cpu.regs[31] = 0x2000
	r.ctx.NeedHelper = HELPER_NONE
	r.ctx.MMUEnabled = 1
	defer func() { r.ctx.MMUEnabled = 0 }()

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	assertSTOREHelperFields(t, r.ctx, lowAddr, payload, uint32(IE64_SIZE_Q), PROG_START, 0x2000)
}

func TestJIT_ARM64_STORE_MicroTLBMissPreservesSpilledPayloadForHelper(t *testing.T) {
	r := newJITTestRig(t)
	const payload = uint64(0xA1B2C3D4E5F60718)
	r.ctx.MMUEnabled = 1 // Empty micro-TLB forces the helper exit.
	r.cpu.regs[15], r.cpu.regs[2] = payload, 0x4000
	r.compileAndRun(t, ie64Instr(OP_STORE, 15, IE64_SIZE_Q, 0, 2, 0, 0))
	assertSTOREHelperFields(t, r.ctx, 0x4000, payload, uint32(IE64_SIZE_Q), PROG_START, r.cpu.regs[31])
}

func TestJIT_ARM64_STORE_MicroTLBHitBytePhysicalAliasInvalidates(t *testing.T) {
	r := newJITTestRig(t)
	const virt, phys = uint64(0x4A8), uint64(0x7000)
	r.cpu.mmuEnabled, r.ctx.MMUEnabled = true, 1
	r.ctx.refreshMicroTLBPrefixes(r.cpu)
	idx := ie64MicroTLBIndex(ie64MicroTLBSet(virt), 0)
	r.ctx.MicroTLBKeys[idx] = ie64MicroTLBKey(r.cpu, virt, ACCESS_WRITE)
	r.ctx.MicroTLBPhys[idx] = phys
	r.cpu.regs[1], r.cpu.regs[2] = 0x5A, virt
	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_B, 0, 2, 0, 0))
	if r.ctx.NeedInval != 1 || r.ctx.InvalSize != 0 {
		t.Fatalf("byte physical alias invalidation = need %d size %d", r.ctx.NeedInval, r.ctx.InvalSize)
	}
}

func TestJIT_ARM64_STORE_MicroTLBHitUsesTranslatedPhysicalAddress(t *testing.T) {
	for _, tc := range []struct {
		name string
		way  uint64
		rd   byte
		size byte
	}{
		{"byte_way_0", 0, 1, IE64_SIZE_B},
		{"word_way_1", 1, 1, IE64_SIZE_W},
		{"long_way_0", 0, 1, IE64_SIZE_L},
		{"quad_spilled_way_1", 1, 15, IE64_SIZE_Q},
	} {
		t.Run(fmt.Sprintf("%s", tc.name), func(t *testing.T) {
			r := newJITTestRig(t)
			const virt = uint64(0x4A8)
			const phys = uint64(0x7000)
			const payload = uint64(0x1122334455667788)
			r.cpu.mmuEnabled = true
			r.ctx.MMUEnabled = 1
			r.ctx.refreshMicroTLBPrefixes(r.cpu)
			idx := ie64MicroTLBIndex(ie64MicroTLBSet(virt), tc.way)
			r.ctx.MicroTLBKeys[idx] = ie64MicroTLBKey(r.cpu, virt, ACCESS_WRITE)
			r.ctx.MicroTLBPhys[idx] = phys
			r.ctx.NeedHelper = HELPER_NONE
			r.cpu.regs[tc.rd] = payload
			r.cpu.regs[2] = virt

			r.compileAndRun(t, ie64Instr(OP_STORE, tc.rd, tc.size, 0, 2, 0, 0))

			addr := phys + (virt & MMU_PAGE_MASK)
			var got, want uint64
			switch tc.size {
			case IE64_SIZE_B:
				got, want = uint64(r.cpu.memory[addr]), payload&0xFF
			case IE64_SIZE_W:
				got, want = uint64(binary.LittleEndian.Uint16(r.cpu.memory[addr:])), payload&0xFFFF
			case IE64_SIZE_L:
				got, want = uint64(binary.LittleEndian.Uint32(r.cpu.memory[addr:])), payload&0xFFFFFFFF
			default:
				got, want = binary.LittleEndian.Uint64(r.cpu.memory[addr:]), payload
			}
			if got != want {
				t.Fatalf("physical store = %#x, want %#x", got, want)
			}
			if r.ctx.NeedHelper != HELPER_NONE {
				t.Fatalf("NeedHelper = %d, want no helper on a micro-TLB hit", r.ctx.NeedHelper)
			}
		})
	}
}

func TestJIT_ARM64_STORE_LowAddr_NoHelper(t *testing.T) {
	r := newJITTestRig(t)
	const addr uint32 = 0x4000
	const payload uint64 = 0xC0FFEE0042424242
	r.cpu.regs[1] = payload
	r.cpu.regs[2] = uint64(addr)
	r.ctx.NeedHelper = 0xDEADBEEF

	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0))

	got := binary.LittleEndian.Uint64(r.cpu.memory[addr:])
	if got != payload {
		t.Fatalf("memory[0x%X] = 0x%016X, want 0x%016X", addr, got, payload)
	}
	if r.ctx.NeedHelper != 0xDEADBEEF {
		t.Fatalf("NeedHelper = %d, want untouched poison", r.ctx.NeedHelper)
	}
}

func TestJIT_ARM64_STORE_MMUOffByteNoInvalidation(t *testing.T) {
	r := newJITTestRig(t)
	r.cpu.regs[1], r.cpu.regs[2] = 0x5A, 0x4000
	r.compileAndRun(t, ie64Instr(OP_STORE, 1, IE64_SIZE_B, 0, 2, 0, 0))
	if r.ctx.NeedInval != 0 || r.ctx.InvalSize != 0 {
		t.Fatalf("MMU-off byte store requested invalidation: need %d size %d", r.ctx.NeedInval, r.ctx.InvalSize)
	}
}

func TestJIT_ARM64_STORE_HighAddr_HelperEndToEnd(t *testing.T) {
	const payload uint64 = 0xFEDCBA0987654321
	const highAddr uint64 = 0x0000_0001_0000_8000

	cpu, backing := runIE64HighBackingTest_ARM64(t,
		func(cpu *CPU64) {
			cpu.regs[1] = payload
			cpu.regs[2] = highAddr
		},
		ie64Instr(OP_STORE, 1, IE64_SIZE_Q, 0, 2, 0, 0),
	)

	var got uint64
	for i := uint64(0); i < 8; i++ {
		got |= uint64(backing.Read8(highAddr+i)) << (8 * i)
	}
	if got != payload {
		t.Fatalf("backing[0x%016X] = 0x%016X, want 0x%016X", highAddr, got, payload)
	}
	if cpu.regs[2] != highAddr {
		t.Fatalf("R2 clobbered = 0x%016X, want 0x%016X", cpu.regs[2], highAddr)
	}
}
