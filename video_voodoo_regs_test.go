//go:build headless

package main

import "testing"

func testVoodooSoftwareBackend(t *testing.T, v *VoodooEngine) *VoodooSoftwareBackend {
	t.Helper()
	vb, ok := v.backend.(*VulkanBackend)
	if !ok {
		t.Fatalf("headless backend type = %T, want *VulkanBackend", v.backend)
	}
	if vb.software == nil {
		t.Fatal("headless VulkanBackend has nil software backend")
	}
	return vb.software
}

func TestVoodoo_FBZColorPath_AppliedToBackend(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_FBZCOLOR_PATH, VOODOO_COMBINE_MODULATE)

	if sw.fbzColorPath != VOODOO_COMBINE_MODULATE || !sw.colorPathSet {
		t.Fatalf("software color path = %#08x set=%v", sw.fbzColorPath, sw.colorPathSet)
	}
}

func TestVoodoo_FogModeColor_AppliedToBackend(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_FOG_MODE, VOODOO_FOG_ENABLE|VOODOO_FOG_CONSTANT)
	v.HandleWrite(VOODOO_FOG_COLOR, 0x00445566)

	if sw.fogMode != VOODOO_FOG_ENABLE|VOODOO_FOG_CONSTANT || sw.fogColor != 0x00445566 {
		t.Fatalf("software fog mode/color = %#08x/%#08x", sw.fogMode, sw.fogColor)
	}
}

func TestVoodoo_ChromaRange_MinMaxCompare(t *testing.T) {
	b := NewVoodooSoftwareBackend()
	if err := b.Init(8, 8); err != nil {
		t.Fatal(err)
	}
	b.SetChromaKey(0x00102030)
	b.SetChromaRange(0x00304050)

	if !b.chromaKeyTest(0x20/255.0, 0x30/255.0, 0x40/255.0) {
		t.Fatal("color inside chroma min/max range was not discarded")
	}
	if b.chromaKeyTest(0x40/255.0, 0x30/255.0, 0x40/255.0) {
		t.Fatal("color outside chroma min/max range was discarded")
	}
}

func TestVoodoo_Stipple_AppliedToBackend(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_STIPPLE, 0xA5A55A5A)

	if sw.stipple != 0xA5A55A5A {
		t.Fatalf("software stipple = %#08x", sw.stipple)
	}
}

func TestVoodoo_LFBMode_TLOD_TexBases_AppliedToBackend(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_LFB_MODE, 0x11)
	v.HandleWrite(VOODOO_TLOD, 0x22)
	for i := 0; i < len(sw.texBase); i++ {
		v.HandleWrite(VOODOO_TEX_BASE0+uint32(i*4), uint32(0x1000+i))
	}

	if sw.lfbMode != 0x11 || sw.tlod != 0x22 {
		t.Fatalf("software lfb/tlod = %#08x/%#08x", sw.lfbMode, sw.tlod)
	}
	for i, got := range sw.texBase {
		if got != uint32(0x1000+i) {
			t.Fatalf("software texBase[%d] = %#08x", i, got)
		}
	}
}

func TestVoodoo_SlopeDeltas_AppliedToBackend(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_DRDX, 0x00001000)
	v.HandleWrite(VOODOO_DWDY, 0x00002000)
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)

	if !sw.slopesValid {
		t.Fatal("software backend did not receive slope state")
	}
	if sw.slopes.DRDX != 0x00001000 || sw.slopes.DWDY != 0x00002000 {
		t.Fatalf("software slopes = %#08x/%#08x", sw.slopes.DRDX, sw.slopes.DWDY)
	}
}

func TestVoodoo_FogTablePalette_AppliedToBackend(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_FOG_TABLE_BASE+3*VOODOO_FOG_TABLE_STRIDE, 0x0000007F)
	v.HandleWrite(VOODOO_PALETTE_BASE+5*4, 0x11223344)

	if sw.fogTable[3] != 0x7F {
		t.Fatalf("software fogTable[3] = %#08x", sw.fogTable[3])
	}
	if sw.palette[5] != 0x11223344 {
		t.Fatalf("software palette[5] = %#08x", sw.palette[5])
	}
}
