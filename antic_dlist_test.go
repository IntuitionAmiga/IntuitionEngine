package main

import "testing"

func anticTestPixel(frame []byte, x, y int) [4]byte {
	offset := (y*ANTIC_FRAME_WIDTH + x) * 4
	return [4]byte{frame[offset], frame[offset+1], frame[offset+2], frame[offset+3]}
}

func anticRGBA(color uint8) [4]byte {
	rgba := ANTICPaletteRGBA[color]
	return [4]byte{rgba[0], rgba[1], rgba[2], rgba[3]}
}

func TestANTICDisplayListMode15LMSGoldenPixels(t *testing.T) {
	bus, _ := newMappedANTICTestBus()
	antic := NewANTICEngine(bus)
	antic.dmactl = ANTIC_DMA_DL
	antic.dlistl = 0x00
	antic.dlisth = 0x20
	antic.colpf[0] = 0x02
	antic.colpf[1] = 0x0E

	bus.Write8(0x2000, DL_LMS|DL_MODE15)
	bus.Write8(0x2001, 0x00)
	bus.Write8(0x2002, 0x30)
	bus.Write8(0x2003, DL_JVB)
	bus.Write8(0x2004, 0x03)
	bus.Write8(0x2005, 0x20)
	bus.Write8(0x3000, 0x80)

	frame := antic.RenderFrame(nil)
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP), anticRGBA(0x0E); got != want {
		t.Fatalf("first mode15 pixel = %v, want %v", got, want)
	}
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT+1, ANTIC_BORDER_TOP), anticRGBA(0x02); got != want {
		t.Fatalf("second mode15 pixel = %v, want %v", got, want)
	}
}

func TestANTICDisplayListMode2UsesCHBASEGlyph(t *testing.T) {
	bus, _ := newMappedANTICTestBus()
	antic := NewANTICEngine(bus)
	antic.dmactl = ANTIC_DMA_DL
	antic.dlistl = 0x00
	antic.dlisth = 0x20
	antic.chbase = 0x40
	antic.colpf[0] = 0x04
	antic.colpf[1] = 0x0C

	bus.Write8(0x2000, DL_LMS|DL_MODE2)
	bus.Write8(0x2001, 0x00)
	bus.Write8(0x2002, 0x30)
	bus.Write8(0x2003, DL_JVB)
	bus.Write8(0x2004, 0x03)
	bus.Write8(0x2005, 0x20)
	bus.Write8(0x3000, 1)
	bus.Write8(0x4008, 0x80)

	frame := antic.RenderFrame(nil)
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP), anticRGBA(0x0C); got != want {
		t.Fatalf("glyph set pixel = %v, want %v", got, want)
	}
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT+1, ANTIC_BORDER_TOP), anticRGBA(0x04); got != want {
		t.Fatalf("glyph clear pixel = %v, want %v", got, want)
	}
}

func TestANTICDisplayListMode8GoldenPixels(t *testing.T) {
	bus, _ := newMappedANTICTestBus()
	antic := NewANTICEngine(bus)
	antic.dmactl = ANTIC_DMA_DL
	antic.dlistl = 0x00
	antic.dlisth = 0x20
	antic.colpf[0] = 0x06
	antic.colpf[1] = 0x0A

	bus.Write8(0x2000, DL_LMS|DL_MODE8)
	bus.Write8(0x2001, 0x00)
	bus.Write8(0x2002, 0x30)
	bus.Write8(0x2003, DL_JVB)
	bus.Write8(0x2004, 0x03)
	bus.Write8(0x2005, 0x20)
	bus.Write8(0x3000, 1)

	frame := antic.RenderFrame(nil)
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP), anticRGBA(0x0A); got != want {
		t.Fatalf("mode8 set cell pixel = %v, want %v", got, want)
	}
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT+8, ANTIC_BORDER_TOP), anticRGBA(0x06); got != want {
		t.Fatalf("mode8 clear cell pixel = %v, want %v", got, want)
	}
}

func TestANTICDisplayListJVBTerminatesSelfLoop(t *testing.T) {
	bus, _ := newMappedANTICTestBus()
	antic := NewANTICEngine(bus)
	antic.dmactl = ANTIC_DMA_DL
	antic.dlistl = 0x00
	antic.dlisth = 0x20
	bus.Write8(0x2000, DL_JVB)
	bus.Write8(0x2001, 0x00)
	bus.Write8(0x2002, 0x20)

	_ = antic.RenderFrame(nil)
}

func TestANTICDisplayListDLIEmitsInterrupt(t *testing.T) {
	bus, _ := newMappedANTICTestBus()
	antic := NewANTICEngine(bus)
	sink := &recordingInterruptSink{}
	antic.SetInterruptSink(sink)
	antic.dmactl = ANTIC_DMA_DL
	antic.nmien = ANTIC_NMIEN_DLI
	antic.dlistl = 0x00
	antic.dlisth = 0x20
	bus.Write8(0x2000, DL_DLI|DL_LMS|DL_MODE15)
	bus.Write8(0x2001, 0x00)
	bus.Write8(0x2002, 0x30)
	bus.Write8(0x2003, DL_JVB)
	bus.Write8(0x2004, 0x03)
	bus.Write8(0x2005, 0x20)

	_ = antic.RenderFrame(nil)
	if antic.nmist&ANTIC_NMIST_DLI == 0 {
		t.Fatal("DLI did not set NMIST")
	}
	if len(sink.pulses) != 1 || sink.pulses[0] != IntMaskDLI {
		t.Fatalf("DLI pulses = %v, want [DLI]", sink.pulses)
	}
}

func TestANTICDisplayListBlankProgramPreservesRasterBars(t *testing.T) {
	bus, _ := newMappedANTICTestBus()
	antic := NewANTICEngine(bus)
	antic.dmactl = ANTIC_DMA_DL
	antic.dlistl = 0x00
	antic.dlisth = 0x20
	antic.colbk = 0x24
	antic.scanlineColors[1][0] = 0x4E
	antic.writeBuffer = 0

	addr := uint32(0x2000)
	for i := 0; i < 3; i++ {
		bus.Write8(addr, DL_BLANK8)
		addr++
	}
	for i := 0; i < ANTIC_DISPLAY_HEIGHT; i++ {
		bus.Write8(addr, DL_BLANK1)
		addr++
	}
	for i := 0; i < 3; i++ {
		bus.Write8(addr, DL_BLANK8)
		addr++
	}
	bus.Write8(addr, DL_JVB)
	bus.Write8(addr+1, 0x00)
	bus.Write8(addr+2, 0x20)

	frame := antic.RenderFrame(nil)
	if got, want := anticTestPixel(frame, ANTIC_BORDER_LEFT, ANTIC_BORDER_TOP), anticRGBA(0x4E); got != want {
		t.Fatalf("blank DL overwrote first active raster pixel: got %v want %v", got, want)
	}
}
