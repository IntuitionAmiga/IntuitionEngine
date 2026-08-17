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

// crtProfile deliberately separates the quality-first Guest-Advanced
// presentation pipeline from the retained single-pass Zfast fallback. F7
// controls whether CRT is requested; it does not silently substitute one look
// for the other.
type crtProfile uint8

const (
	crtProfileGuestAdvanced crtProfile = iota
	crtProfileZfast
)

func (p crtProfile) String() string {
	switch p {
	case crtProfileGuestAdvanced:
		return "Guest-Advanced"
	case crtProfileZfast:
		return "Zfast"
	default:
		return "Unknown"
	}
}

// crtPresentationMode is the host-facing F7 state. Off is the default;
// flat CRT is the first enabled mode, convex curvature is opt-in.
type crtPresentationMode uint8

const (
	crtModeFlat crtPresentationMode = iota
	crtModeCurved
	crtModeOff
)

func (m crtPresentationMode) next() crtPresentationMode {
	switch m {
	case crtModeFlat:
		return crtModeCurved
	case crtModeCurved:
		return crtModeOff
	default:
		return crtModeFlat
	}
}

func (m crtPresentationMode) enabled() bool { return m != crtModeOff }

func (m crtPresentationMode) String() string {
	switch m {
	case crtModeFlat:
		return "flat"
	case crtModeCurved:
		return "curved"
	default:
		return "off"
	}
}

func crtPresentationModeFromString(value string) (crtPresentationMode, bool) {
	switch value {
	case "flat":
		return crtModeFlat, true
	case "curved":
		return crtModeCurved, true
	case "off":
		return crtModeOff, true
	default:
		return crtModeOff, false
	}
}

// crtPresentationState is a browser-automation contract. It deliberately
// distinguishes a requested CRT mode from one whose shader failed to become
// available, so a wasm smoke test cannot mistake an unfiltered fallback for
// an active presentation path.
func crtPresentationState(mode crtPresentationMode, effective bool) string {
	if mode == crtModeOff {
		return "off"
	}
	if effective {
		return mode.String() + "-active"
	}
	return mode.String() + "-unavailable"
}

func crtModeFromEnabled(enabled bool) crtPresentationMode {
	if enabled {
		return crtModeFlat
	}
	return crtModeOff
}

type crtFilter struct {
	shader *ebiten.Shader
	guest  *guestAdvancedCRT
	err    error
}

type crtPresentationGeometry struct {
	scanlinePeriod float32
	scanlineOrigin float32
}

func defaultCRTPresentationGeometry() crtPresentationGeometry {
	// Overlay and legacy framebuffer paths have no retained guest geometry.
	// Keep their existing two-row fine scanline fallback; compositor-backed
	// guest frames replace it with their actual source scale below.
	return crtPresentationGeometry{scanlinePeriod: 2}
}

// crtHardwareLayerUniforms keeps the native guest and presentation rectangle
// together. Zfast receives this at the same draw that expands a guest texture,
// never after a 320x200 or 640x480 frame has already become a 1080p image.
func crtHardwareLayerUniforms(layer *ebitenHardwareLayer) map[string]any {
	return map[string]any{
		"SourceSize": []float32{float32(layer.SourceWidth), float32(layer.SourceHeight)},
		"DestSize":   []float32{float32(layer.DestWidth), float32(layer.DestHeight)},
		"DestOrigin": []float32{float32(layer.DestX), float32(layer.DestY)},
		"Opaque":     opaqueUniform(layer.Opaque),
		"LayerMode":  float32(1),
	}
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

func newGuestAdvancedCRTFilter() *crtFilter {
	guest, err := newGuestAdvancedCRT()
	if err != nil {
		return &crtFilter{err: err}
	}
	return &crtFilter{guest: guest}
}

// zfastCRTShaderSource retains the fixed Zfast fine-mask profile from the
// upstream shader: blur 0.30, low/high scanline 6/8, bright boost 1.25, mask
// darkness 0.25, and mask/scanline fade 0.8. There is intentionally no
// curvature or vignette.
const zfastCRTShaderSource = `//kage:unit pixels

package main

var ScanlinePeriod float
var ScanlineOrigin float
var SourceSize vec2
var DestSize vec2
var DestOrigin vec2
var Opaque float
var LayerMode float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	// Quilez-style scaling, sharpened as in zfast_crt_impl.inc.
	p := srcPos
	if DestSize.x > 0.0 && DestSize.y > 0.0 {
		p = (dstPos.xy-DestOrigin)*SourceSize/DestSize
	}
	i := floor(p) + vec2(0.5)
	f := p - i
	p = i + 4.0*f*f*f
	p.x = mix(p.x, srcPos.x, 0.30)

	// p is native-source space in the hardware-compositor path, so its
	// fractional Y naturally spans every enlarged guest line. The final-image
	// fallback retains its explicit presentation-space period.
	scanY := fract((dstPos.y - ScanlineOrigin) / ScanlinePeriod)
	if DestSize.x > 0.0 && DestSize.y > 0.0 {
		scanY = f.y
	}
	y := scanY*scanY
	yy := y*y
	// Zfast's fine mask is deliberately an output-pixel aperture pattern. Only
	// the vertical scanline phase follows the enlarged guest source geometry.
	whichMask := fract(floor(dstPos.x) * -0.4999)
	mask := 1.0
	if whichMask < 0.5 {
		mask -= 0.25
	}

	colour := imageSrc0At(imageSrc0Origin() + p)
	// Preserve the hardware compositor's layer semantics. Opaque guest layers
	// use alpha only as storage and must cover everything beneath them; normal
	// layers discard a fully transparent texel instead of overwriting a lower
	// filtered layer with transparent black.
	if LayerMode != 0.0 {
		if Opaque != 0.0 {
			colour.a = 1.0
		} else if colour.a == 0.0 && colour.r == 0.0 && colour.g == 0.0 && colour.b == 0.0 {
			discard()
		}
	}
	scanLineWeight := 1.25 - 6.0*(y - 2.05*yy)
	scanLineWeightB := 1.0 - 8.0*(yy - 2.8*yy*y)
	weight := mix(scanLineWeight*mask, scanLineWeightB, dot(colour.rgb, vec3(0.3333*0.8)))
	return vec4(colour.rgb*weight, colour.a)
}
`
