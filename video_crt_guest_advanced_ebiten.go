//go:build !headless

package main

import (
	"fmt"

	"github.com/hajimehoshi/ebiten/v2"
)

const guestAdvancedPersistence float32 = 0.32

// guestAdvancedCRT is the screen-only part of CRT-Guest-Advanced. It keeps
// its intermediate images explicitly because Guest-Advanced is a pass graph:
// source-space beam shaping, phosphor persistence, separable glow/bloom and
// output-space mask/deconvergence. There is intentionally no bezel, cabinet
// or reflection image in this pipeline.
type guestAdvancedCRT struct {
	raster, afterglow, linearize, blurHorizontal, blurVertical, final *ebiten.Shader
	history, persisted, linear, glowHorizontal, glowVertical          *ebiten.Image
	sourceModeKey, activeModeKey                                      string
	resetPersistence                                                  bool
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
	if g.linearize, err = compile("linearise", guestAdvancedLinearizeShaderSource); err != nil {
		return nil, err
	}
	if g.blurHorizontal, err = compile("horizontal glow", guestAdvancedHorizontalBlurShaderSource); err != nil {
		return nil, err
	}
	if g.blurVertical, err = compile("vertical bloom", guestAdvancedVerticalBlurShaderSource); err != nil {
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
	g.linear = ebiten.NewImage(width, height)
	g.glowHorizontal = ebiten.NewImage(width, height)
	g.glowVertical = ebiten.NewImage(width, height)
}

func (g *guestAdvancedCRT) disposeTargets() {
	for _, image := range []*ebiten.Image{g.history, g.persisted, g.linear, g.glowHorizontal, g.glowVertical} {
		if image != nil {
			image.Deallocate()
		}
	}
	g.history, g.persisted, g.linear, g.glowHorizontal, g.glowVertical = nil, nil, nil, nil, nil
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
func (g *guestAdvancedCRT) finish(screen, input *ebiten.Image) {
	w, h := input.Bounds().Dx(), input.Bounds().Dy()
	g.ensureTargets(w, h)
	if g.resetPersistence || g.activeModeKey != g.sourceModeKey {
		g.history.Clear()
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

	g.linear.Clear()
	linearize := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy}
	linearize.Images[0] = g.persisted
	g.linear.DrawRectShader(w, h, g.linearize, linearize)

	g.glowHorizontal.Clear()
	horizontal := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{"TexelSize": texel}}
	horizontal.Images[0] = g.linear
	g.glowHorizontal.DrawRectShader(w, h, g.blurHorizontal, horizontal)

	g.glowVertical.Clear()
	vertical := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{"TexelSize": texel}}
	vertical.Images[0] = g.glowHorizontal
	g.glowVertical.DrawRectShader(w, h, g.blurVertical, vertical)

	final := &ebiten.DrawRectShaderOptions{Blend: ebiten.BlendCopy, Uniforms: map[string]any{"BloomStrength": float32(0.28), "MaskStrength": float32(0.34), "TexelSize": texel}}
	final.Images[0], final.Images[1] = g.linear, g.glowVertical
	screen.DrawRectShader(w, h, g.final, final)

	g.history.Clear()
	g.history.DrawImage(g.persisted, nil)
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

// Guest-Advanced's glow and bloom passes operate in linear light. Ebiten
// render targets are normalised RGBA rather than floating point, so this pass
// retains the upstream gamma relationship while clamping to the portable range.
const guestAdvancedLinearizeShaderSource = `//kage:unit pixels
package main
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	c := imageSrc0At(srcPos)
	return vec4(pow(max(c.rgb, vec3(0.0)), vec3(2.2)), c.a)
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
var MaskStrength float
var TexelSize vec2
func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	p0 := imageSrc0Origin()+srcPos
	p1 := imageSrc1Origin()+srcPos
	base := imageSrc0At(p0)
	// Small channel offsets reproduce convergence error before the phosphor mask.
	r := imageSrc0At(p0+vec2(0.35, 0.0)).r
	g := base.g
	b := imageSrc0At(p0-vec2(0.35, 0.0)).b
	c := vec3(r, g, b) + imageSrc1At(p1).rgb*BloomStrength
	c = pow(max(c, vec3(0.0)), vec3(1.0/2.2))
	phase := mod(floor(dstPos.x-imageDstOrigin().x), 3.0)
	mask := vec3(1.0-MaskStrength)
	if phase < 1.0 { mask.r = 1.0 } else if phase < 2.0 { mask.g = 1.0 } else { mask.b = 1.0 }
	return vec4(c*mask, base.a)
}`
