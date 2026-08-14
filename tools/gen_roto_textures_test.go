package main

import (
	"bytes"
	"image/png"
	"os"
	"testing"
)

func TestRotoTextureVariantsIncludeExpectedLabels(t *testing.T) {
	want := map[string]struct {
		raw string
		png string
	}{
		"IES": {
			raw: "sdk/examples/assets/rotozoomtexture_ies.raw",
			png: "sdk/examples/assets/rotozoomtexture_ies.png",
		},
		"EMUTOS": {
			raw: "sdk/examples/assets/rotozoomtexture_emutos.raw",
			png: "sdk/examples/assets/rotozoomtexture_emutos.png",
		},
		"APIASM": {
			raw: "sdk/examples/assets/rotozoomtexture_api_asm.raw",
			png: "sdk/examples/assets/rotozoomtexture_api_asm.png",
		},
		"HW ASM": {
			raw: "sdk/examples/assets/rotozoomtexture_hw_asm.raw",
			png: "sdk/examples/assets/rotozoomtexture_hw_asm.png",
		},
		"API C": {
			raw: "sdk/examples/assets/rotozoomtexture_api_c.raw",
			png: "sdk/examples/assets/rotozoomtexture_api_c.png",
		},
		"HW C": {
			raw: "sdk/examples/assets/rotozoomtexture_hw_c.raw",
			png: "sdk/examples/assets/rotozoomtexture_hw_c.png",
		},
		"NO CPU": {
			raw: "sdk/examples/assets/rotozoomtexture_nocpu.raw",
			png: "sdk/examples/assets/rotozoomtexture_nocpu.png",
		},
	}
	for _, variant := range rotoVariants {
		paths, ok := want[variant.label]
		if !ok {
			continue
		}
		delete(want, variant.label)
		if variant.rawPath != paths.raw {
			t.Fatalf("%s raw path = %q", variant.label, variant.rawPath)
		}
		if variant.pngPath != paths.png {
			t.Fatalf("%s png path = %q", variant.label, variant.pngPath)
		}
	}
	for label := range want {
		t.Fatalf("roto texture variants missing %s", label)
	}
	for _, r := range " IESMUTOAPHWCN" {
		if len(glyphs[r]) == 0 {
			t.Fatalf("glyph %q missing", r)
		}
	}
}

func TestNoCPURotoTextureMatchesRawAndPreservesCanonicalPixels(t *testing.T) {
	base, err := os.ReadFile("../sdk/examples/assets/rotozoomtexture.raw")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../sdk/examples/assets/rotozoomtexture_nocpu.raw")
	if err != nil {
		t.Fatal(err)
	}
	pngFile, err := os.Open("../sdk/examples/assets/rotozoomtexture_nocpu.png")
	if err != nil {
		t.Fatal(err)
	}
	defer pngFile.Close()
	img, err := png.Decode(pngFile)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := len(raw), rotoTextureWidth*rotoTextureHeight*rotoPixelBytes; got != want {
		t.Fatalf("raw size = %d, want %d", got, want)
	}
	if got := img.Bounds(); got.Dx() != rotoTextureWidth || got.Dy() != rotoTextureHeight {
		t.Fatalf("PNG dimensions = %dx%d, want %dx%d", got.Dx(), got.Dy(), rotoTextureWidth, rotoTextureHeight)
	}

	decoded := make([]byte, 0, len(raw))
	for y := 0; y < rotoTextureHeight; y++ {
		for x := 0; x < rotoTextureWidth; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			decoded = append(decoded, byte(r>>8), byte(g>>8), byte(b>>8), byte(a>>8))
		}
	}
	if !bytes.Equal(decoded, raw) {
		t.Fatal("PNG pixels do not match the raw texture")
	}

	changed := false
	for y := 0; y < rotoTextureHeight; y++ {
		for x := 0; x < rotoTextureWidth; x++ {
			offset := (y*rotoTextureWidth + x) * rotoPixelBytes
			if y < 182 || y >= 238 || x < 20 || x >= 236 {
				if !bytes.Equal(raw[offset:offset+rotoPixelBytes], base[offset:offset+rotoPixelBytes]) {
					t.Fatalf("canonical pixel changed outside the label plate at %d,%d", x, y)
				}
			} else if !bytes.Equal(raw[offset:offset+rotoPixelBytes], base[offset:offset+rotoPixelBytes]) {
				changed = true
			}
		}
	}
	if !changed {
		t.Fatal("NO CPU label plate did not change any pixels")
	}
}
