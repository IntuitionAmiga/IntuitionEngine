//go:build headless

package main

import "testing"

func TestVoodoo_VulkanDepthNormalization_UsesDecodedVertexZ(t *testing.T) {
	decodedZ := fixed20_12ToFloat(0x800)
	if got := normalizeVoodooDepthForVulkan(decodedZ, 0); got != decodedZ {
		t.Fatalf("decoded vertex Z normalization = %f, want %f", got, decodedZ)
	}
	megaDemoZ := fixed20_12ToFloat(0x8000)
	if got := normalizeVoodooDepthForVulkan(megaDemoZ, 0); got >= 1 {
		t.Fatalf("mega demo Z normalization = %f, want less than clear depth 1.0", got)
	}
	if got := normalizeVoodooDepthForVulkan(2, VOODOO_FBZ_WBUFFER); got != 1 {
		t.Fatalf("W-buffer depth clamp = %f, want 1", got)
	}
	if got := normalizeVoodooDepthForVulkan(-1, 0); got != 0 {
		t.Fatalf("negative depth clamp = %f, want 0", got)
	}
}
