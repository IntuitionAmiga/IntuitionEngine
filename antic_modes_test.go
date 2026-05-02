package main

import "testing"

func newModeTestANTIC(mode uint8, scanlines int) (*MachineBus, *ANTICEngine) {
	bus, _ := newMappedANTICTestBus()
	antic := NewANTICEngine(bus)
	antic.dmactl = ANTIC_DMA_DL
	antic.dlistl = 0x00
	antic.dlisth = 0x20
	antic.colpf = [4]uint8{0x02, 0x04, 0x06, 0x08}
	bus.Write8(0x2000, DL_LMS|mode)
	bus.Write8(0x2001, 0x00)
	bus.Write8(0x2002, 0x30)
	bus.Write8(0x2003, DL_JVB)
	bus.Write8(0x2004, 0x03)
	bus.Write8(0x2005, 0x20)
	return bus, antic
}

func TestANTICRemainingBitmapModesRenderGoldenPixels(t *testing.T) {
	for _, tc := range []struct {
		mode  uint8
		value uint8
		want  uint8
	}{
		{DL_MODE9, 0xC0, 0x08},
		{DL_MODE10, 0x80, 0x06},
		{DL_MODE11, 0x40, 0x04},
		{DL_MODE12, 0x00, 0x02},
		{DL_MODE13, 0xC0, 0x08},
		{DL_MODE14, 0x80, 0x06},
	} {
		t.Run("mode", func(t *testing.T) {
			bus, antic := newModeTestANTIC(tc.mode, 1)
			bus.Write8(0x3000, tc.value)
			frame := antic.RenderFrame(nil)
			if got := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP); got != anticRGBA(tc.want) {
				t.Fatalf("mode 0x%02X first pixel got %v want color 0x%02X", tc.mode, got, tc.want)
			}
		})
	}
}

func TestANTICTextModes6And7Use20ColumnsDoubleWidth(t *testing.T) {
	bus, antic := newModeTestANTIC(DL_MODE6, 8)
	antic.chbase = 0x40
	bus.Write8(0x3000, 1)
	bus.Write8(0x4008, 0x80)

	frame := antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP); got != anticRGBA(0x04) {
		t.Fatalf("mode6 first pixel got %v", got)
	}
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT+1, ANTIC_BORDER_TOP); got != anticRGBA(0x04) {
		t.Fatalf("mode6 double-width second pixel got %v", got)
	}
}

func TestANTICHSCROLShiftsMode15Pixels(t *testing.T) {
	bus, antic := newModeTestANTIC(DL_HSCROL|DL_MODE15, 1)
	antic.hscrol = 3
	bus.Write8(0x3000, 0x80)

	frame := antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT-3, ANTIC_BORDER_TOP); got != anticRGBA(0x04) {
		t.Fatalf("hscroll shifted pixel got %v", got)
	}
}

func TestANTICCHACTLBlankInvertReflect(t *testing.T) {
	bus, antic := newModeTestANTIC(DL_MODE2, 8)
	antic.chbase = 0x40
	bus.Write8(0x3000, 1)
	bus.Write8(0x4008, 0x80)
	bus.Write8(0x400F, 0x01)

	antic.chactl = ANTIC_CHACTL_BLANK
	frame := antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP); got != anticRGBA(0x02) {
		t.Fatalf("blank chactl pixel got %v", got)
	}

	antic.chactl = ANTIC_CHACTL_INVERT
	frame = antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT+1, ANTIC_BORDER_TOP); got != anticRGBA(0x04) {
		t.Fatalf("invert chactl clear-bit pixel got %v", got)
	}

	antic.chactl = ANTIC_CHACTL_REFLECT
	frame = antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT+7, ANTIC_BORDER_TOP); got != anticRGBA(0x04) {
		t.Fatalf("reflect chactl bottom-row pixel got %v", got)
	}
}

func TestANTICGTIAPseudoModesAffectColorSelection(t *testing.T) {
	bus, antic := newModeTestANTIC(DL_MODE15, 1)
	antic.prior = GTIA_PRIOR_GTIA1
	antic.colpf[1] = 0xA0
	bus.Write8(0x3000, 0x8F)
	frame := antic.RenderFrame(nil)
	if got := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP); got != anticRGBA(0xAF) {
		t.Fatalf("GTIA mode 9 color got %v want 0xAF", got)
	}
}
