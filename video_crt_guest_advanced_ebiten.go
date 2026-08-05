//go:build !headless

package main

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	guestAdvancedPersistence float32 = 0.32
	// These are the restrained convex-screen values documented by the upstream
	// Guest-Advanced profile. They warp the final CRT face, never individual
	// guest layers, so every native mode shares one physical screen shape.
	guestAdvancedCurvatureX     float32 = 0.03
	guestAdvancedCurvatureY     float32 = 0.04
	guestAdvancedCurvatureShape float32 = 0.25
)

// guestAdvancedWarpUV is the upstream convex-screen transform in normalised
// presentation coordinates. Keeping it in Go gives the screen geometry a
// non-GPU oracle; guestAdvancedFinalShaderSource carries the same calculation
// for the displayed image.
func guestAdvancedWarpUV(u, v, curvatureX, curvatureY, shape float32) (float32, float32) {
	if shape <= 0 {
		return u, v
	}
	x, y := u*2-1, v*2-1
	warpedX := x / float32(math.Sqrt(float64(1-shape*y*y)))
	warpedY := y / float32(math.Sqrt(float64(1-shape*x*x)))
	x = x + (warpedX-x)*curvatureX/shape
	y = y + (warpedY-y)*curvatureY/shape
	return x*0.5 + 0.5, y*0.5 + 0.5
}

func guestAdvancedCurvature(curved bool) (float32, float32, float32) {
	if !curved {
		return 0, 0, 0
	}
	return guestAdvancedCurvatureX, guestAdvancedCurvatureY, guestAdvancedCurvatureShape
}

// guestAdvancedCRT is the screen-only part of CRT-Guest-Advanced. It keeps
// its intermediate images explicitly because Guest-Advanced is a pass graph:
// source-space beam shaping, phosphor persistence, separable glow/bloom and
// output-space mask/deconvergence. There is intentionally no bezel, cabinet
// or reflection image in this pipeline.
type guestAdvancedCRT struct {
	raster, afterglow, preAfterglow, averageLuminancePass, linearize                     *ebiten.Shader
	gaussianHorizontalPass, gaussianVerticalPass, bloomHorizontalPass, bloomVerticalPass *ebiten.Shader
	final                                                                                *ebiten.Shader
	history, persisted, prepared, averageLuminance, luminanceHistory                     *ebiten.Image
	linear, gaussianHorizontal, gaussianVertical, bloomHorizontalTarget                  *ebiten.Image
	bloomVertical                                                                        *ebiten.Image
	sourceModeKey, activeModeKey                                                         string
	resetPersistence                                                                     bool
}

func (g *guestAdvancedCRT) setSourceMode(key string) { g.sourceModeKey = key }

// resetAfterglow makes the next enabled frame start without light retained
// while CRT presentation was disabled. The reset is deliberately deferred so
// the history target is only touched when finish owns the render pass again.
func (g *guestAdvancedCRT) resetAfterglow() { g.resetPersistence = true }

func crtHardwareGuestModeKey(layers []ebitenHardwareLayer) string {
	key := fmt.Sprintf("layers=%d", len(layers))
	for i := range layers {
		layer := &layers[i]
		key += fmt.Sprintf("/%dx%d@%d,%d:%dx%d", layer.SourceWidth, layer.SourceHeight, layer.DestX, layer.DestY, layer.DestWidth, layer.DestHeight)
	}
	return key
}

func guestAdvancedRasterUniforms(sourceWidth, sourceHeight, destWidth, destHeight int) map[string]any {
	return map[string]any{
		"SourceSize": []float32{float32(sourceWidth), float32(sourceHeight)},
		"DestSize":   []float32{float32(destWidth), float32(destHeight)},
		"DestOrigin": []float32{0, 0},
		"Opaque":     float32(1),
		"LayerMode":  float32(1),
	}
}

func guestAdvancedOverlayUniforms(width, height int) map[string]any {
	uniforms := guestAdvancedRasterUniforms(width, height, width, height)
	uniforms["Opaque"] = float32(0)
	return uniforms
}

func newGuestAdvancedCRT() (*guestAdvancedCRT, error) {
	compile := func(name, source string) (*ebiten.Shader, error) {
		shader, err := ebiten.NewShader([]byte(source))
		if err != nil {
			return nil, fmt.Errorf("compile Guest-Advanced %s pass: %w", name, err)
		}
		return shader, nil
	}
	g := &guestAdvancedCRT{}
	var err error
	if g.raster, err = compile("raster", guestAdvancedRasterShaderSource); err != nil {
		return nil, err
	}
	if g.afterglow, err = compile("afterglow", guestAdvancedAfterglowShaderSource); err != nil {
		return nil, err
	}
	if g.preAfterglow, err = compile("afterglow preparation", guestAdvancedPreAfterglowShaderSource); err != nil {
		return nil, err
	}
	if g.averageLuminancePass, err = compile("average luminance", guestAdvancedAverageLuminanceShaderSource); err != nil {
		return nil, err
	}
	if g.linearize, err = compile("linearise", guestAdvancedLinearizeShaderSource); err != nil {
		return nil, err
	}
	if g.gaussianHorizontalPass, err = compile("horizontal Gaussian glow", guestAdvancedGaussianHorizontalShaderSource); err != nil {
		return nil, err
	}
	if g.gaussianVerticalPass, err = compile("vertical Gaussian glow", guestAdvancedGaussianVerticalShaderSource); err != nil {
		return nil, err
	}
	if g.bloomHorizontalPass, err = compile("horizontal bloom", guestAdvancedHorizontalBlurShaderSource); err != nil {
		return nil, err
	}
	if g.bloomVerticalPass, err = compile("vertical bloom", guestAdvancedVerticalBlurShaderSource); err != nil {
		return nil, err
	}
	if g.final, err = compile("mask and deconvergence", guestAdvancedFinalShaderSource); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *guestAdvancedCRT) ensureTargets(width, height int) {
	if g.persisted != nil && g.persisted.Bounds().Dx() == width && g.persisted.Bounds().Dy() == height {
		return
	}
	g.disposeTargets()
	g.history = ebiten.NewImage(width, height)
	g.persisted = ebiten.NewImage(width, height)
	g.prepared = ebiten.NewImage(width, height)
	g.averageLuminance = ebiten.NewImage(width, height)
	g.luminanceHistory = ebiten.NewImage(width, height)
	g.linear = ebiten.NewImage(width, height)
	g.gaussianHorizontal = ebiten.NewImage(width, height)
	g.gaussianVertical = ebiten.NewImage(width, height)
	g.bloomHorizontalTarget = ebiten.NewImage(width, height)
	g.bloomVertical = ebiten.NewImage(width, height)
}

func (g *guestAdvancedCRT) disposeTargets() {
	for _, image := range []*ebiten.Image{g.history, g.persisted, g.prepared, g.averageLuminance, g.luminanceHistory, g.linear, g.gaussianHorizontal, g.gaussianVertical, g.bloomHorizontalTarget, g.bloomVertical} {
		if image != nil {
			image.Deallocate()
		}
	}
	g.history, g.persisted, g.prepared, g.averageLuminance, g.luminanceHistory = nil, nil, nil, nil, nil
	g.linear, g.gaussianHorizontal, g.gaussianVertical = nil, nil, nil
	g.bloomHorizontalTarget, g.bloomVertical = nil, nil
}

// drawRaster applies Guest-Advanced's source-space interpolation and
// luminance-dependent beam. It is used while each compositor layer expands
// from its native guest dimensions, so 320x200 and 640x480 retain their own
// scanline cadence at 1080p.
func (g *guestAdvancedCRT) drawRaster(dst, source *ebiten.Image, uniforms map[string]any, triangles *ebiten.DrawTrianglesShaderOptions, vertices []ebiten.Vertex) {
	if triangles != nil {
		triangles.Images[0] = source
		triangles.Uniforms = uniforms
		dst.DrawTrianglesShader(vertices, []uint16{0, 1, 2, 1, 3, 2}, g.raster, triangles)
		return
	}
	op := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: uniforms}
	op.Images[0] = source
	dst.DrawRectShader(dst.Bounds().Dx(), dst.Bounds().Dy(), g.raster, op)
}

// finish supplies the viewport-space passes. Persistence and bloom are
// deliberately after composition: light from adjacent guest layers behaves
// like light on one CRT face, while source-space rasterisation remains per
// layer. The history is reset when the output size changes in ensureTargets.
func (g *guestAdvancedCRT) finish(screen, input *ebiten.Image, curved bool) {
	w, h := input.Bounds().Dx(), input.Bounds().Dy()
	g.ensureTargets(w, h)
	if g.resetPersistence || g.activeModeKey != g.sourceModeKey {
		g.history.Clear()
		g.luminanceHistory.Clear()
		g.activeModeKey = g.sourceModeKey
		g.resetPersistence = false
	}
	texel := []float32{1 / float32(w), 1 / float32(h)}

	g.persisted.Clear()
	// Guest-Advanced default PR/PG/PB is 0.32. The upstream values are equal,
	// so a scalar preserves the reference profile without a needless channel
	// branch in this pass.
	afterglow := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{"Persistence": guestAdvancedPersistence}}
	afterglow.Images[0], afterglow.Images[1] = input, g.history
	g.persisted.DrawRectShader(w, h, g.afterglow, afterglow)

	g.prepared.Clear()
	prepare := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy}
	prepare.Images[0], prepare.Images[1] = input, g.persisted
	g.prepared.DrawRectShader(w, h, g.preAfterglow, prepare)

	g.averageLuminance.Clear()
	average := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{"SourceSize": []float32{float32(w), float32(h)}}}
	average.Images[0], average.Images[1] = g.prepared, g.luminanceHistory
	g.averageLuminance.DrawRectShader(w, h, g.averageLuminancePass, average)

	g.linear.Clear()
	linearize := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy}
	linearize.Images[0], linearize.Images[1] = g.prepared, g.averageLuminance
	g.linear.DrawRectShader(w, h, g.linearize, linearize)

	g.gaussianHorizontal.Clear()
	gaussianHorizontal := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{"TexelSize": texel}}
	gaussianHorizontal.Images[0] = g.linear
	g.gaussianHorizontal.DrawRectShader(w, h, g.gaussianHorizontalPass, gaussianHorizontal)

	g.gaussianVertical.Clear()
	gaussianVertical := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{"TexelSize": texel}}
	gaussianVertical.Images[0] = g.gaussianHorizontal
	g.gaussianVertical.DrawRectShader(w, h, g.gaussianVerticalPass, gaussianVertical)

	g.bloomHorizontalTarget.Clear()
	bloomHorizontal := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{"TexelSize": texel}}
	bloomHorizontal.Images[0] = g.linear
	g.bloomHorizontalTarget.DrawRectShader(w, h, g.bloomHorizontalPass, bloomHorizontal)

	g.bloomVertical.Clear()
	bloomVertical := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{"TexelSize": texel}}
	bloomVertical.Images[0] = g.bloomHorizontalTarget
	g.bloomVertical.DrawRectShader(w, h, g.bloomVerticalPass, bloomVertical)

	curvatureX, curvatureY, curvatureShape := guestAdvancedCurvature(curved)
	final := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{
		"BloomStrength": float32(0.28), "GlowStrength": float32(0.12), "MaskStrength": float32(0.34), "TexelSize": texel,
		"ScreenSize": []float32{float32(w), float32(h)},
		"CurvatureX": curvatureX, "CurvatureY": curvatureY, "CurvatureShape": curvatureShape,
	}}
	final.Images[0], final.Images[1], final.Images[2] = g.linear, g.bloomVertical, g.gaussianVertical
	screen.DrawRectShader(w, h, g.final, final)

	g.history.Clear()
	g.history.DrawImage(g.persisted, nil)
	g.luminanceHistory.Clear()
	g.luminanceHistory.DrawImage(g.averageLuminance, nil)
}

// Ported effect structure from libretro/slang-shaders crt-guest-advanced
// (guest(r), GPL-2.0-or-later). This Kage implementation intentionally uses
// only screen effects from the upstream preset, not the optional Mega Bezel
// presentation stack.
const guestAdvancedRasterShaderSource = `//kage:unit pixels
package main
var SourceSize vec2
var DestSize vec2
var DestOrigin vec2
var Opaque float
var LayerMode float
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	local := dstPos.xy-imageDstOrigin()
	p := local
	if DestSize.x > 0.0 && DestSize.y > 0.0 { p = (local-DestOrigin)*SourceSize/DestSize }
	i := floor(p)+vec2(0.5)
	f := p-i
	p = i+4.0*f*f*f
	p = clamp(p, vec2(0.0), SourceSize-vec2(1.0))
	colour := imageSrc0At(imageSrc0Origin()+p)
	if LayerMode != 0.0 {
		if Opaque != 0.0 { colour.a = 1.0 } else if colour.a == 0.0 && colour.r == 0.0 && colour.g == 0.0 && colour.b == 0.0 { discard() }
	}
	luma := dot(colour.rgb, vec3(0.2126, 0.7152, 0.0722))
	beam := mix(1.30, 0.92, luma)
	scan := exp2(-8.0*beam*f.y*f.y)
	return vec4(colour.rgb*scan, colour.a)
}`

const guestAdvancedAfterglowShaderSource = `//kage:unit pixels
package main
var Persistence float
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	now := imageSrc0At(srcPos)
	previous := imageSrc1At(srcPos)
	return vec4(max(now.rgb, previous.rgb*Persistence), max(now.a, previous.a*Persistence))
}`

// The upstream pre-shaders-afterglow pass combines the current signal with
// colour-shaped retained phosphor light before the linear-light blur stages.
// IE deliberately keeps the neutral profile: no LUT, vignette or cabinet
// treatment, but the retained light is saturated and added separately from
// the current raster rather than being treated as the source image itself.
const guestAdvancedPreAfterglowShaderSource = `//kage:unit pixels
package main
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	p := imageSrc0Origin()+srcPos
	now := imageSrc0At(p)
	persisted := imageSrc1At(imageSrc1Origin()+srcPos)
	trail := max(persisted.rgb-now.rgb, vec3(0.0))
	light := length(trail)
	if light > 0.0 { trail = normalize(trail+vec3(0.00001))*light }
	return vec4(min(now.rgb+trail, vec3(1.0)), max(now.a, persisted.a))
}`

// avg-lum is the screen-only equivalent of Guest-Advanced's AvgLumPass. Kage
// does not expose Slang's mip LOD sampling, so it uses five stable screen
// probes plus the local pixel and temporal alpha feedback. The result is kept
// in alpha for the final CRT pass while RGB carries local edge deltas for
// later profile extensions.
const guestAdvancedAverageLuminanceShaderSource = `//kage:unit pixels
package main
var SourceSize vec2
func luma(c vec3) float { return dot(c, vec3(0.2126, 0.7152, 0.0722)) }
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	origin := imageSrc0Origin()
	p := origin+srcPos
	local := imageSrc0At(p).rgb
	c0 := imageSrc0At(origin+SourceSize*vec2(0.20, 0.20)).rgb
	c1 := imageSrc0At(origin+SourceSize*vec2(0.80, 0.20)).rgb
	c2 := imageSrc0At(origin+SourceSize*vec2(0.20, 0.80)).rgb
	c3 := imageSrc0At(origin+SourceSize*vec2(0.80, 0.80)).rgb
	c4 := imageSrc0At(origin+SourceSize*vec2(0.50, 0.50)).rgb
	scene := (luma(c0)+luma(c1)+luma(c2)+luma(c3)+luma(c4))*0.20
	previous := imageSrc1At(imageSrc1Origin()+srcPos).a
	adapted := mix(max(scene, luma(local)*0.20), previous, 0.70)
	dx := abs(luma(imageSrc0At(p+vec2(1.0, 0.0)).rgb)-luma(imageSrc0At(p-vec2(1.0, 0.0)).rgb))
	dy := abs(luma(imageSrc0At(p+vec2(0.0, 1.0)).rgb)-luma(imageSrc0At(p-vec2(0.0, 1.0)).rgb))
	return vec4(dx, dy, max(dx, dy), adapted)
}`

// Guest-Advanced's glow and bloom passes operate in linear light. Ebiten
// render targets are normalised RGBA rather than floating point, so this pass
// retains the upstream gamma relationship while clamping to the portable range.
const guestAdvancedLinearizeShaderSource = `//kage:unit pixels
package main
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	c := imageSrc0At(srcPos)
	adaptiveLuminance := imageSrc1At(imageSrc1Origin()+srcPos).a
	// The AvgLumPass feeds a restrained gain into linear light. This retains
	// the CRT's stable beam while letting bright and dark scenes diverge.
	gain := mix(0.92, 1.05, adaptiveLuminance)
	return vec4(pow(max(c.rgb*gain, vec3(0.0)), vec3(2.2)), c.a)
}`

// Guest-Advanced keeps a tight Gaussian glow distinct from its broader bloom.
// The pair makes bright phosphors softly bleed into immediately adjacent
// pixels while the later bloom pair remains responsible for the larger halo.
const guestAdvancedGaussianHorizontalShaderSource = `//kage:unit pixels
package main
var TexelSize vec2
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	p := imageSrc0Origin()+srcPos
	d := vec2(1.0, 0.0)
	c := imageSrc0At(p)*0.40
	c += (imageSrc0At(p-d)+imageSrc0At(p+d))*0.24
	c += (imageSrc0At(p-2.0*d)+imageSrc0At(p+2.0*d))*0.06
	return c
}`

const guestAdvancedGaussianVerticalShaderSource = `//kage:unit pixels
package main
var TexelSize vec2
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	p := imageSrc0Origin()+srcPos
	d := vec2(0.0, 1.0)
	c := imageSrc0At(p)*0.40
	c += (imageSrc0At(p-d)+imageSrc0At(p+d))*0.24
	c += (imageSrc0At(p-2.0*d)+imageSrc0At(p+2.0*d))*0.06
	return c
}`

const guestAdvancedHorizontalBlurShaderSource = `//kage:unit pixels
package main
var TexelSize vec2
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	d := vec2(3.0, 0.0)
	p := imageSrc0Origin()+srcPos
	c := imageSrc0At(p)*0.40
	c += imageSrc0At(p-d)*0.24 + imageSrc0At(p+d)*0.24
	c += imageSrc0At(p-2.0*d)*0.06 + imageSrc0At(p+2.0*d)*0.06
	return c
}`

const guestAdvancedVerticalBlurShaderSource = `//kage:unit pixels
package main
var TexelSize vec2
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	d := vec2(0.0, 2.0)
	p := imageSrc0Origin()+srcPos
	c := imageSrc0At(p)*0.42
	c += imageSrc0At(p-d)*0.23 + imageSrc0At(p+d)*0.23
	c += imageSrc0At(p-2.0*d)*0.06 + imageSrc0At(p+2.0*d)*0.06
	return c
}`

const guestAdvancedFinalShaderSource = `//kage:unit pixels
package main
var BloomStrength float
var GlowStrength float
var MaskStrength float
var TexelSize vec2
var ScreenSize vec2
var CurvatureX float
var CurvatureY float
var CurvatureShape float
func warpUV(uv vec2) vec2 {
	if CurvatureShape <= 0.0 { return uv }
	p := uv*2.0-vec2(1.0)
	warped := vec2(p.x/sqrt(1.0-CurvatureShape*p.y*p.y), p.y/sqrt(1.0-CurvatureShape*p.x*p.x))
	p = p+(warped-p)*vec2(CurvatureX, CurvatureY)/CurvatureShape
	return p*0.5+vec2(0.5)
}
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	uv := warpUV(srcPos/ScreenSize)
	// The curved face has no signal outside its nominal raster. This avoids
	// edge-clamp streaks at the convex corners and keeps the full screen opaque.
	if uv.x < 0.0 || uv.x > 1.0 || uv.y < 0.0 || uv.y > 1.0 { return vec4(0.0, 0.0, 0.0, 1.0) }
	p0 := imageSrc0Origin()+uv*ScreenSize
	p1 := imageSrc1Origin()+uv*ScreenSize
	p2 := imageSrc2Origin()+uv*ScreenSize
	base := imageSrc0At(p0)
	// Small channel offsets reproduce convergence error before the phosphor mask.
	r := imageSrc0At(p0+vec2(0.35, 0.0)).r
	g := base.g
	b := imageSrc0At(p0-vec2(0.35, 0.0)).b
	c := vec3(r, g, b) + imageSrc2At(p2).rgb*GlowStrength + imageSrc1At(p1).rgb*BloomStrength
	c = pow(max(c, vec3(0.0)), vec3(1.0/2.2))
	phase := mod(floor(dstPos.x-imageDstOrigin().x), 3.0)
	mask := vec3(1.0-MaskStrength)
	if phase < 1.0 { mask.r = 1.0 } else if phase < 2.0 { mask.g = 1.0 } else { mask.b = 1.0 }
	return vec4(c*mask, base.a)
}`
