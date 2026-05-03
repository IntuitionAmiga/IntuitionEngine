// aros_2gib_test.go - PLAN_MAX_RAM slice 10h.
//
// Pins the AROS profile-cap raise from 32 MiB to 2 GiB. AROS_PROFILE_TOP
// is uint32; its new value (= 0x80000000) is the largest page-aligned
// quantity that survives the M68K profile's uint32 representation. The
// runtime minimum stays at 32 MiB so legacy test rigs still satisfy the
// profile gate; clampM68KProfileToBus reduces TopOfRAM when the bus is
// smaller than the new cap.

package main

import "testing"

func TestAROS_ProfileBoundsTopRaisedTo2GiB(t *testing.T) {
	const twoGiB uint32 = 2 * 1024 * 1024 * 1024
	if AROS_PROFILE_TOP != twoGiB {
		t.Fatalf("AROS_PROFILE_TOP = 0x%X, want 0x%X (slice 10h raise)", AROS_PROFILE_TOP, twoGiB)
	}
	bus := fakeProfileBus{activeVisible: uint64(twoGiB)}
	pb := AROSProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("AROSProfileBounds err = %v", pb.Err)
	}
	if pb.TopOfRAM != twoGiB {
		t.Fatalf("pb.TopOfRAM = 0x%X, want 0x%X", pb.TopOfRAM, twoGiB)
	}
}

func TestAROS_ProfileBoundsClampsToActiveBelow2GiB(t *testing.T) {
	bus := fakeProfileBus{activeVisible: 256 * 1024 * 1024}
	pb := AROSProfileBounds(bus)
	if pb.Err != nil {
		t.Fatalf("err = %v", pb.Err)
	}
	if uint64(pb.TopOfRAM) != bus.activeVisible {
		t.Fatalf("pb.TopOfRAM = 0x%X, want 0x%X (clamped to bus active visible)",
			pb.TopOfRAM, bus.activeVisible)
	}
}

func TestAROS_DirectVRAMUnchanged(t *testing.T) {
	// VRAM at 0x1E00000..0x2000000 is 30 MiB — well within the new 2 GiB
	// cap. Pin the contract so the raise does not silently move it.
	bus := fakeProfileBus{activeVisible: uint64(AROS_PROFILE_TOP)}
	pb := AROSProfileBounds(bus)
	if pb.VRAMBase != 0x1E00000 {
		t.Fatalf("VRAMBase = 0x%X, want 0x1E00000", pb.VRAMBase)
	}
	if pb.VRAMEnd != 0x2000000 {
		t.Fatalf("VRAMEnd = 0x%X, want 0x2000000", pb.VRAMEnd)
	}
}

func TestAROS_PaulaDMA_ProfileTopUsesAROSContract(t *testing.T) {
	// The AROS audio DMA fetch is bounded by AROSProfileBounds.TopOfRAM. With
	// active visible at the profile cap (2 GiB), the DMA bound matches
	// AROS_PROFILE_TOP. SetBacking + SetSizing fakes the 2 GiB sizing
	// without committing 2 GiB of heap (SparseBacking is page-keyed).
	bus, err := NewMachineBusSized(64 * 1024 * 1024)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	bus.SetBacking(NewSparseBacking(uint64(AROS_PROFILE_TOP)))
	bus.SetSizing(MemorySizing{
		TotalGuestRAM:    uint64(AROS_PROFILE_TOP),
		ActiveVisibleRAM: uint64(AROS_PROFILE_TOP),
	})
	dma := NewArosAudioDMA(bus, nil, nil)
	if got := dma.profileTop; got != AROS_PROFILE_TOP {
		t.Fatalf("dma.profileTop = 0x%X, want 0x%X (AROS_PROFILE_TOP)", got, AROS_PROFILE_TOP)
	}
}

func TestAROS_NoSignedAddressFlipAt2GiBMinus1(t *testing.T) {
	// 2 GiB - 1 = 0x7FFFFFFF, which becomes int32(0x7FFFFFFF) = MaxInt32
	// when re-interpreted as signed. The boundary value is the largest
	// positive signed-32-bit integer; sign-flip would require crossing
	// to 0x80000000 which is the cap itself (one past the last valid
	// byte address). Pin that AROS_PROFILE_TOP - 1 still represents a
	// positive int32.
	const want = int32(0x7FFFFFFF)
	got := int32(AROS_PROFILE_TOP - 1)
	if got != want {
		t.Fatalf("int32(AROS_PROFILE_TOP-1) = %d, want %d", got, want)
	}
}

// TestDiscovery_AROS_AllPathsAgreeUpToActiveVisible boots mode AROS via
// the slice-10f cap pipeline and confirms SYSINFO returns the AROS
// profile cap. (The full IE64 MFCR-CR_RAM_SIZE_BYTES discovery test
// belongs to an IE64 sub-runtime; AROS proper is M68K.)
func TestDiscovery_AROS_AllPathsAgreeAt2GiB(t *testing.T) {
	bus := bootSimulate(t, modeAros, eightGiB, sparseAllocator)
	if got := bus.TotalGuestRAM(); got != uint64(AROS_PROFILE_TOP) {
		t.Errorf("TotalGuestRAM = %d, want %d", got, AROS_PROFILE_TOP)
	}
	if got := bus.ActiveVisibleRAM(); got != uint64(AROS_PROFILE_TOP) {
		t.Errorf("ActiveVisibleRAM = %d, want %d", got, AROS_PROFILE_TOP)
	}
	if got := sysinfoActiveRAM(bus); got != uint64(AROS_PROFILE_TOP) {
		t.Errorf("SYSINFO_ACTIVE_RAM = %d, want %d", got, AROS_PROFILE_TOP)
	}
}
