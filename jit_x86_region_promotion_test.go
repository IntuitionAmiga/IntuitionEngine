//go:build amd64 && linux

package main

import "testing"

func TestX86RegionPromotion_DefaultOffOptIn(t *testing.T) {
	t.Setenv("X86_JIT_REGIONS", "")
	if x86RegionPromotionDefaultEnabled() {
		t.Fatal("x86 region promotion should be disabled by default")
	}
	t.Setenv("X86_JIT_REGIONS", "1")
	if !x86RegionPromotionDefaultEnabled() {
		t.Fatal("X86_JIT_REGIONS=1 should enable x86 region promotion")
	}
}

func execRel32Target(t *testing.T, patchAddr uintptr) uintptr {
	t.Helper()
	disp := mustExecRel32(t, patchAddr)
	return uintptr(int64(patchAddr) + 4 + int64(disp))
}

func TestX86RegionPromotion_InboundChainsRedirectedOrUnpatched(t *testing.T) {
	em, err := AllocExecMem(4096)
	if err != nil {
		t.Fatalf("AllocExecMem: %v", err)
	}
	defer em.Free()

	writeJMP := func() uintptr {
		t.Helper()
		addr, err := em.Write([]byte{0xE9, 0, 0, 0, 0, 0x90})
		if err != nil {
			t.Fatalf("Write JMP: %v", err)
		}
		return addr + 1
	}
	writeEntry := func() uintptr {
		t.Helper()
		addr, err := em.Write([]byte{0x90, 0xC3})
		if err != nil {
			t.Fatalf("Write entry: %v", err)
		}
		return addr
	}

	targetPC := uint64(0x2000)
	compatiblePatch := writeJMP()
	incompatiblePatch := writeJMP()
	oldEntry := writeEntry()
	newEntry := writeEntry()

	compatibleMap := x86DefaultRegMap()
	incompatibleMap := compatibleMap
	incompatibleMap[0], incompatibleMap[1] = incompatibleMap[1], incompatibleMap[0]

	cc := NewCodeCache()
	cc.Put(&JITBlock{
		startPC: 0x1000,
		endPC:   0x1004,
		regMap:  compatibleMap,
		chainSlots: []chainSlot{
			{targetPC: targetPC, patchAddr: compatiblePatch},
		},
	})
	cc.Put(&JITBlock{
		startPC: 0x1100,
		endPC:   0x1104,
		regMap:  incompatibleMap,
		chainSlots: []chainSlot{
			{targetPC: targetPC, patchAddr: incompatiblePatch},
		},
	})
	cc.Put(&JITBlock{
		startPC:    targetPC,
		endPC:      targetPC + 4,
		chainEntry: oldEntry,
		regMap:     compatibleMap,
	})

	PatchRel32At(compatiblePatch, oldEntry)
	PatchRel32At(incompatiblePatch, oldEntry)

	promoted := &JITBlock{
		startPC:    targetPC,
		endPC:      targetPC + 8,
		chainEntry: newEntry,
		regMap:     compatibleMap,
		tier:       1,
	}
	cc.Put(promoted)
	x86RetargetPromotionChainsTo(cc, promoted)

	if got := execRel32Target(t, compatiblePatch); got != newEntry {
		t.Fatalf("compatible inbound chain target = 0x%X, want new entry 0x%X", got, newEntry)
	}
	if got, want := execRel32Target(t, incompatiblePatch), incompatiblePatch+4; got != want {
		t.Fatalf("incompatible inbound chain target = 0x%X, want unchained fallback 0x%X", got, want)
	}
}

func TestX86RegionPromotion_InvalidatesMatchingRTSCache(t *testing.T) {
	ctx := &X86JITContext{
		RTSCache0PC:     0x2000,
		RTSCache0Addr:   0xCAFE,
		RTSCache0RegMap: 0x11,
		RTSCache1PC:     0x3000,
		RTSCache1Addr:   0xBEEF,
		RTSCache1RegMap: 0x22,
	}

	x86InvalidateRTSCacheForPC(ctx, 0x2000)

	if ctx.RTSCache0PC != 0 || ctx.RTSCache0Addr != 0 || ctx.RTSCache0RegMap != 0 ||
		ctx.RTSCache1PC != 0 || ctx.RTSCache1Addr != 0 || ctx.RTSCache1RegMap != 0 {
		t.Fatalf("RTS cache was not cleared after matching promotion: %+v", ctx)
	}
}
