//go:build !headless

// Zfast CRT final-presentation filter for the Ebiten backend.
//
// This is a Kage port of Libretro slang-shaders commit
// d746ae6d9d4b8e335704d73bd5d667eadec6b7e4,
// crt/shaders/zfast_crt/zfast_crt_finemask.slang and zfast_crt_impl.inc.
//
// zfast_crt_standard - A simple, fast CRT shader.
// Copyright (C) 2017 Greg Hogan (SoltanGris42)
// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

type crtFilterState uint8

const (
	crtFilterUninitialised crtFilterState = iota
	crtFilterAvailable
	crtFilterFailed
)

type crtFilter struct {
	shader *ebiten.Shader
	err    error
}

// newCRTFilter deliberately owns shader construction. Production supplies the
// embedded Zfast source; tests can exercise GPU failure handling with invalid
// Kage without changing package state.
func newCRTFilter(source []byte) *crtFilter {
	shader, err := ebiten.NewShader(source)
	if err != nil {
		return &crtFilter{err: fmt.Errorf("compile CRT shader: %w", err)}
	}
	return &crtFilter{shader: shader}
}

// zfastCRTShaderSource retains the fixed Zfast fine-mask profile from the
// upstream shader: blur 0.30, low/high scanline 6/8, bright boost 1.25, mask
// darkness 0.25, and mask/scanline fade 0.8. There is intentionally no
// curvature or vignette.
const zfastCRTShaderSource = `//kage:unit pixels

package main

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	// Quilez-style scaling, sharpened as in zfast_crt_impl.inc.
	p := srcPos
	i := floor(p) + vec2(0.5)
	f := p - i
	p = i + 4.0*f*f*f
	p.x = mix(p.x, srcPos.x, 0.30)

	// Ebiten supplies pixel-centre source coordinates to a one-to-one final
	// pass, so f.y would otherwise always be zero. Derive the scanline phase
	// from the destination row, preserving Zfast's alternating scanline shape.
	scanY := fract(floor(dstPos.y) * 0.5)
	y := scanY*scanY
	yy := y*y
	whichMask := fract(floor(dstPos.x) * -0.4999)
	mask := 1.0
	if whichMask < 0.5 {
		mask -= 0.25
	}

	colour := imageSrc0At(p)
	scanLineWeight := 1.25 - 6.0*(y - 2.05*yy)
	scanLineWeightB := 1.0 - 8.0*(yy - 2.8*yy*y)
	weight := mix(scanLineWeight*mask, scanLineWeightB, dot(colour.rgb, vec3(0.3333*0.8)))
	return vec4(colour.rgb*weight, colour.a)
}
`
