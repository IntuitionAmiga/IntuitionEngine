//go:build headless

package main

import "testing"

// A textured triangle whose texels carry alpha=0 must leave the
// framebuffer untouched when alpha blending (src_alpha/inv_src_alpha)
// and FBZ alpha planes are enabled — the cutout contract menu
// textures rely on.
func TestVoodoo_AlphaZeroTexelCutout(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque|VOODOO_FBZ_ALPHA_PLANES)
	v.HandleWrite(VOODOO_COLOR0, 0xFF200030)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	// Texture: single texel, grey RGB, alpha 0.
	v.HandleTexMemWrite(VOODOO_TEXMEM_BASE, uint32(0x7F)|uint32(0x7F)<<8|uint32(0x7F)<<16|0x00<<24)
	v.HandleWrite(VOODOO_TEX_WIDTH, 1)
	v.HandleWrite(VOODOO_TEX_HEIGHT, 1)
	v.HandleWrite(VOODOO_TEX_UPLOAD, 1)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn)

	// Blend: src_alpha / inv_src_alpha.
	v.HandleWrite(VOODOO_ALPHA_MODE, (1<<4)|(1<<8)|(5<<12))

	sbSubmitTri(v, 40, 40, 160, 40, 40, 160, 1, 1, 1)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
	v.WaitSwapIdle()

	// Pixel inside the triangle must still be the fill colour.
	idx := (60*640 + 60) * 4
	r, g, b := sw.colorBuffer[idx], sw.colorBuffer[idx+1], sw.colorBuffer[idx+2]
	if r > 0x40 || g > 0x40 {
		t.Fatalf("alpha-0 texel wrote colour: got %d,%d,%d want fill 0x20,0x00,0x30-ish", r, g, b)
	}
}

// Same cutout contract via the gouraud (COLOR_SELECT) vertex path and
// the game's texture-mode bits (perspective + clamps) — mirrors the
// mk64 title-logo draw state byte for byte.
func TestVoodoo_AlphaZeroTexelCutout_GouraudPerspective(t *testing.T) {
	_, v := newMappedTestVoodoo(t)
	sw := testVoodooSoftwareBackend(t, v)

	v.HandleWrite(VOODOO_ENABLE, 1)
	v.HandleWrite(VOODOO_FBZ_MODE, sbFbzOpaque|VOODOO_FBZ_ALPHA_PLANES|1) // clipping too
	v.HandleWrite(VOODOO_CLIP_LEFT_RIGHT, 640)
	v.HandleWrite(VOODOO_CLIP_LOW_Y_HIGH, 480)
	v.HandleWrite(VOODOO_COLOR0, 0xFF200030)
	v.HandleWrite(VOODOO_FAST_FILL_CMD, 0)

	v.HandleTexMemWrite(VOODOO_TEXMEM_BASE, uint32(0x7F)|uint32(0x7F)<<8|uint32(0x7F)<<16)
	v.HandleWrite(VOODOO_TEX_WIDTH, 1)
	v.HandleWrite(VOODOO_TEX_HEIGHT, 1)
	v.HandleWrite(VOODOO_TEX_UPLOAD, 1)
	v.HandleWrite(VOODOO_TEXTURE_MODE, sbTexOn|(1<<14)|(1<<4)|(1<<5)|(1<<6))

	v.HandleWrite(VOODOO_ALPHA_MODE, 0x5110)

	xs := []uint32{40 * 16, 160 * 16, 40 * 16}
	ys := []uint32{40 * 16, 40 * 16, 160 * 16}
	xr := []uint32{VOODOO_VERTEX_AX, VOODOO_VERTEX_BX, VOODOO_VERTEX_CX}
	yr := []uint32{VOODOO_VERTEX_AY, VOODOO_VERTEX_BY, VOODOO_VERTEX_CY}
	ss := []uint32{0, 262144 * 1, 0}
	tt := []uint32{0, 0, 262144 * 1}
	for k := 0; k < 3; k++ {
		v.HandleWrite(VOODOO_COLOR_SELECT, uint32(k))
		v.HandleWrite(VOODOO_START_W, 1<<30)
		v.HandleWrite(xr[k], xs[k])
		v.HandleWrite(yr[k], ys[k])
		v.HandleWrite(VOODOO_START_S, ss[k])
		v.HandleWrite(VOODOO_START_T, tt[k])
		v.HandleWrite(VOODOO_START_R, 4096)
		v.HandleWrite(VOODOO_START_G, 4096)
		v.HandleWrite(VOODOO_START_B, 4096)
		v.HandleWrite(VOODOO_START_A, 4096)
		v.HandleWrite(VOODOO_START_Z, 2048)
	}
	v.HandleWrite(VOODOO_TRIANGLE_CMD, 0)
	v.HandleWrite(VOODOO_SWAP_BUFFER_CMD, 0)
	v.WaitSwapIdle()

	idx := (60*640 + 60) * 4
	r, g, b := sw.colorBuffer[idx], sw.colorBuffer[idx+1], sw.colorBuffer[idx+2]
	if r > 0x40 || g > 0x40 {
		t.Fatalf("gouraud alpha-0 texel wrote colour: got %d,%d,%d", r, g, b)
	}
}
