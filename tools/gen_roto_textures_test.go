package main

import "testing"

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
	for _, r := range " IESMUTOAPHWC" {
		if len(glyphs[r]) == 0 {
			t.Fatalf("glyph %q missing", r)
		}
	}
}
