package main

import "testing"

func TestRotoTextureVariantsIncludeIES(t *testing.T) {
	var found bool
	for _, variant := range rotoVariants {
		if variant.label != "IES" {
			continue
		}
		found = true
		if variant.rawPath != "sdk/examples/assets/rotozoomtexture_ies.raw" {
			t.Fatalf("IES raw path = %q", variant.rawPath)
		}
		if variant.pngPath != "sdk/examples/assets/rotozoomtexture_ies.png" {
			t.Fatalf("IES png path = %q", variant.pngPath)
		}
	}
	if !found {
		t.Fatal("roto texture variants missing IES")
	}
	for _, r := range "IES" {
		if len(glyphs[r]) == 0 {
			t.Fatalf("glyph %q missing", r)
		}
	}
}
