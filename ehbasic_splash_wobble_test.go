package main

import (
	"os"
	"strings"
	"testing"
)

func TestEhBASICSplashWobbleAssetSize(t *testing.T) {
	info, err := os.Stat("sdk/examples/assets/splash_640x92.rgba")
	if err != nil {
		t.Fatalf("splash asset missing: %v", err)
	}
	const want = int64(640 * 92 * 4)
	if info.Size() != want {
		t.Fatalf("splash asset size = %d, want %d", info.Size(), want)
	}

	midiInfo, err := os.Stat("sdk/examples/assets/music/enjoythesilence.mid")
	if err != nil {
		t.Fatalf("MIDI asset missing: %v", err)
	}
	if midiInfo.Size() == 0 {
		t.Fatal("MIDI asset is empty")
	}
}

func TestEhBASICSplashWobbleSpansStayInBounds(t *testing.T) {
	const (
		srcBase     = 0x600000
		backBase    = 0x900000
		screenW     = 640
		screenH     = 480
		imageH      = 92
		top         = 194
		strideBytes = 2560
		pixelBytes  = 4
	)

	srcEnd := srcBase + screenW*imageH*pixelBytes
	backEnd := backBase + screenW*screenH*pixelBytes
	for _, x := range []int{-24, 0, 24} {
		for y := 0; y < imageH; y++ {
			dy := top + y
			dx := x
			sx := 0
			cw := screenW
			if dx < 0 {
				sx = 0 - dx
				cw = screenW - sx
				dx = 0
			}
			if dx+cw > screenW {
				cw = screenW - dx
			}
			if cw <= 0 {
				continue
			}

			srcStart := srcBase + y*strideBytes + sx*pixelBytes
			srcStop := srcStart + cw*pixelBytes
			dstStart := backBase + dy*strideBytes + dx*pixelBytes
			dstStop := dstStart + cw*pixelBytes

			if srcStart < srcBase || srcStop > srcEnd {
				t.Fatalf("x=%d y=%d source span [%#x,%#x) outside [%#x,%#x)",
					x, y, srcStart, srcStop, srcBase, srcEnd)
			}
			if dstStart < backBase || dstStop > backEnd {
				t.Fatalf("x=%d y=%d destination span [%#x,%#x) outside [%#x,%#x)",
					x, y, dstStart, dstStop, backBase, backEnd)
			}
		}
	}
}

func TestEhBASICSplashWobbleSetsAllocatedFBBase(t *testing.T) {
	program, err := os.ReadFile("sdk/examples/basic/splash_wobble.bas")
	if err != nil {
		t.Fatalf("read splash_wobble.bas: %v", err)
	}
	text := string(program)
	for _, want := range []string{
		"FB=MEMALLOC(1228800):SR=MEMALLOC(235520):BB=MEMALLOC(1228800)",
		"POKE32 &HF0084,FB",
		"POKE32 &HF0000,1",
		"PEEK32(&HF2310)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("splash_wobble.bas missing %q", want)
		}
	}
}

func TestEhBASICWobbleZoomProgramShape(t *testing.T) {
	program, err := os.ReadFile("sdk/examples/basic/wobble_zoom.bas")
	if err != nil {
		t.Fatalf("read wobble_zoom.bas: %v", err)
	}
	text := string(program)
	for _, want := range []string{
		"FB=MEMALLOC(1228800):BB=MEMALLOC(1228800):TX=MEMALLOC(2097152):SR=MEMALLOC(235520)",
		"POKE32 &HF0084,FB",
		"PEEK32(&HF2310)",
		"BLIT MODE7",
		"1023,511,TS,ST",
		"SC=1.7+SIN(Z)*0.9",
		"BLIT MEMCOPY BB,FB,1228800",
		"SOUND PLAY \"sdk/examples/assets/music/enjoythesilence.mid\"",
		"BLOAD \"sdk/examples/assets/splash_640x92.rgba\",SR",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("wobble_zoom.bas missing %q", want)
		}
	}
	if strings.Index(text, "SOUND PLAY") > strings.Index(text, "BLOAD") {
		t.Fatal("wobble_zoom.bas must start MIDI before loading splash assets")
	}
	if strings.Index(text, "SOUND PLAY") > strings.Index(text, "POKE32 &HF0000") {
		t.Fatal("wobble_zoom.bas must start MIDI before video setup")
	}
}

func TestEhBASICSplashWobbleFullFramebufferFlip(t *testing.T) {
	program, err := os.ReadFile("sdk/examples/basic/splash_wobble.bas")
	if err != nil {
		t.Fatalf("read splash_wobble.bas: %v", err)
	}
	text := string(program)
	if !strings.Contains(text, "BLIT MEMCOPY BB,FB,1228800") {
		t.Fatal("splash_wobble.bas must copy the full 640x480 RGBA32 framebuffer")
	}
}

func TestEhBASICWobbleZoomSpansStayInBounds(t *testing.T) {
	const (
		frontBase     = 0x100000
		backBase      = 0x230000
		textureBase   = 0x360000
		srcBase       = 0x600000
		vramBase      = 0x100000
		vramEnd       = 0x600000
		screenW       = 640
		screenH       = 480
		imageW        = 640
		imageH        = 92
		textureW      = 1024
		textureH      = 512
		originX       = 192
		originY       = 210
		screenStride  = 2560
		textureStride = 4096
		pixelBytes    = 4
	)

	frontEnd := frontBase + screenW*screenH*pixelBytes
	backEnd := backBase + screenW*screenH*pixelBytes
	textureEnd := textureBase + textureW*textureH*pixelBytes
	srcEnd := srcBase + imageW*imageH*pixelBytes

	for name, span := range map[string][2]int{
		"front":   {frontBase, frontEnd},
		"back":    {backBase, backEnd},
		"texture": {textureBase, textureEnd},
	} {
		if span[0] < vramBase || span[1] > vramEnd {
			t.Fatalf("%s span [%#x,%#x) outside VRAM [%#x,%#x)", name, span[0], span[1], vramBase, vramEnd)
		}
	}

	for _, shift := range []int{-24, 0, 24} {
		for y := 0; y < imageH; y++ {
			dy := originY + y
			dx := originX + shift
			sx := 0
			cw := imageW
			if dx < 0 {
				sx = 0 - dx
				cw = imageW - sx
				dx = 0
			}
			if dx+cw > textureW {
				cw = textureW - dx
			}
			if cw <= 0 {
				continue
			}

			srcStart := srcBase + y*screenStride + sx*pixelBytes
			srcStop := srcStart + cw*pixelBytes
			dstStart := textureBase + dy*textureStride + dx*pixelBytes
			dstStop := dstStart + cw*pixelBytes

			if srcStart < srcBase || srcStop > srcEnd {
				t.Fatalf("shift=%d y=%d source span [%#x,%#x) outside [%#x,%#x)",
					shift, y, srcStart, srcStop, srcBase, srcEnd)
			}
			if dstStart < textureBase || dstStop > textureEnd {
				t.Fatalf("shift=%d y=%d texture span [%#x,%#x) outside [%#x,%#x)",
					shift, y, dstStart, dstStop, textureBase, textureEnd)
			}
		}
	}
}
