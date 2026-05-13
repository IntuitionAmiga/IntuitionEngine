package main

import "testing"

func TestDebugDeviceSnapshot_ULACompositorRoundTrip(t *testing.T) {
	bus, err := NewMachineBusSized(1 << 20)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	ula := NewULAEngine(bus)
	ula.compositorSnap.vram[17] = 0x5a
	ula.compositorSnap.border = 3
	ula.compositorSnap.flashState = true
	ula.compositorSnap.target = []byte{1, 2, 3, 4}

	version, data, err := ula.DebugSnapshot()
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}
	ula.compositorSnap.vram[17] = 0
	ula.compositorSnap.border = 0
	ula.compositorSnap.flashState = false
	ula.compositorSnap.target = nil

	if err := ula.DebugRestoreSnapshot(version, data); err != nil {
		t.Fatalf("DebugRestoreSnapshot: %v", err)
	}
	if got := ula.compositorSnap.vram[17]; got != 0x5a {
		t.Fatalf("compositor vram[17] = %#x, want 0x5a", got)
	}
	if ula.compositorSnap.border != 3 || !ula.compositorSnap.flashState {
		t.Fatalf("compositor state = border %d flash %v", ula.compositorSnap.border, ula.compositorSnap.flashState)
	}
	if len(ula.compositorSnap.target) != 4 || ula.compositorSnap.target[2] != 3 {
		t.Fatalf("compositor target = %#v", ula.compositorSnap.target)
	}
}

func TestDebugDeviceSnapshot_ANTICScanlinePMGRoundTrip(t *testing.T) {
	bus, err := NewMachineBusSized(1 << 20)
	if err != nil {
		t.Fatalf("NewMachineBusSized: %v", err)
	}
	antic := NewANTICEngine(bus)
	antic.scanlinePass.target = []byte{9, 8, 7}
	antic.scanlinePass.pfMask = []uint8{1, 2, 3}
	antic.scanlinePass.pmg.gractl = 0x03
	antic.scanlinePass.pmg.prior = 0x40
	antic.scanlinePass.pmg.sizep[2] = 0x11
	antic.scanlinePass.pmg.playerGfx[1][7] = 0x80
	antic.scanlinePass.pmg.pfMask = []uint8{4, 5, 6}
	antic.scanlinePass.pc = 0x1234
	antic.scanlinePass.screenAddr = 0x4567
	antic.scanlinePass.displayY = 12
	antic.scanlinePass.entries = 3
	antic.scanlinePass.entryLine = 2
	antic.scanlinePass.entryValid = true

	version, data, err := antic.DebugSnapshot()
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}
	antic.scanlinePass = anticScanlinePass{}

	if err := antic.DebugRestoreSnapshot(version, data); err != nil {
		t.Fatalf("DebugRestoreSnapshot: %v", err)
	}
	if antic.scanlinePass.pmg.gractl != 0x03 || antic.scanlinePass.pmg.prior != 0x40 {
		t.Fatalf("pmg control = gractl %#x prior %#x", antic.scanlinePass.pmg.gractl, antic.scanlinePass.pmg.prior)
	}
	if antic.scanlinePass.pmg.sizep[2] != 0x11 || antic.scanlinePass.pmg.playerGfx[1][7] != 0x80 {
		t.Fatalf("pmg sprite state not restored")
	}
	if len(antic.scanlinePass.pmg.pfMask) != 3 || antic.scanlinePass.pmg.pfMask[1] != 5 {
		t.Fatalf("pmg pfMask = %#v", antic.scanlinePass.pmg.pfMask)
	}
	if antic.scanlinePass.pc != 0x1234 || antic.scanlinePass.screenAddr != 0x4567 || antic.scanlinePass.displayY != 12 {
		t.Fatalf("scanline pass = pc %#x screen %#x y %d", antic.scanlinePass.pc, antic.scanlinePass.screenAddr, antic.scanlinePass.displayY)
	}
}
