//go:build arm64 && (linux || windows || darwin)

package main

import (
	"encoding/binary"
	"testing"
)

func TestARM64FPResidency_UsesV16ThroughV31(t *testing.T) {
	var singles [16]int
	for i := range singles {
		singles[i] = 1
	}
	plan := ie64BuildFPResidencyPlan(singles, [8]int{})
	if len(plan.bindings) != 16 {
		t.Fatalf("resident bindings = %d, want 16", len(plan.bindings))
	}
	for i, b := range plan.bindings {
		want := byte(16 + i)
		if b.xmm != want {
			t.Fatalf("binding %d host V%d, want V%d", i, b.xmm, want)
		}
		if b.xmm >= 8 && b.xmm <= 15 {
			t.Fatalf("binding %d illegally uses callee-saved V%d", i, b.xmm)
		}
	}
}

func TestARM64FPResidency_LoadStoreUseResidentVector(t *testing.T) {
	plan := ie64FPResidencyPlan{}
	for i := range plan.owner {
		plan.owner[i] = -1
	}
	plan.bindings = []ie64FPResidentBinding{{kind: ie64FPResSingle, baseSlot: 3, xmm: 16}}
	plan.owner[3] = 0
	cb := NewCodeBuffer(16)
	cb.fpPlan = &plan

	emitLoadFPReg(cb, 0, 3)
	emitStoreFPReg(cb, 0, 3)
	if got := cb.Len(); got != 8 {
		t.Fatalf("resident load+store emitted %d bytes, want two ARM64 instructions", got)
	}
	wantLoad := arm64FMOV_StoW(0, 16)
	wantStore := arm64FMOV_WtoS(16, 0)
	if got := binary.LittleEndian.Uint32(cb.Bytes()[:4]); got != wantLoad {
		t.Fatalf("resident load = 0x%08X, want 0x%08X", got, wantLoad)
	}
	if got := binary.LittleEndian.Uint32(cb.Bytes()[4:]); got != wantStore {
		t.Fatalf("resident store = 0x%08X, want 0x%08X", got, wantStore)
	}
}
