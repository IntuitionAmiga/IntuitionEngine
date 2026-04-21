package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
)

const (
	rotoTextureWidth  = 256
	rotoTextureHeight = 256
	rotoPixelBytes    = 4
)

type rotoVariant struct {
	label   string
	base    string
	rawPath string
	pngPath string
}

var rotoVariants = []rotoVariant{
	{label: "BASIC", base: "rotozoomtexture", rawPath: "sdk/examples/assets/rotozoomtexture_ehbasic.raw", pngPath: "sdk/examples/assets/rotozoomtexture_ehbasic.png"},
	{label: "IE32", base: "rotozoomtexture", rawPath: "sdk/examples/assets/rotozoomtexture_ie32.raw", pngPath: "sdk/examples/assets/rotozoomtexture_ie32.png"},
	{label: "IE64", base: "rotozoomtexture", rawPath: "sdk/examples/assets/rotozoomtexture_ie64.raw", pngPath: "sdk/examples/assets/rotozoomtexture_ie64.png"},
	{label: "M68K", base: "rotozoomtexture", rawPath: "sdk/examples/assets/rotozoomtexture_m68k.raw", pngPath: "sdk/examples/assets/rotozoomtexture_m68k.png"},
	{label: "6502", base: "rotozoomtexture", rawPath: "sdk/examples/assets/rotozoomtexture_6502.raw", pngPath: "sdk/examples/assets/rotozoomtexture_6502.png"},
	{label: "Z80", base: "rotozoomtexture", rawPath: "sdk/examples/assets/rotozoomtexture_z80.raw", pngPath: "sdk/examples/assets/rotozoomtexture_z80.png"},
	{label: "X86", base: "rotozoomtexture", rawPath: "sdk/examples/assets/rotozoomtexture_x86.raw", pngPath: "sdk/examples/assets/rotozoomtexture_x86.png"},
}

var glyphs = map[rune][]string{
	'0': {
		" ### ",
		"#   #",
		"#   #",
		"#   #",
		"#   #",
		"#   #",
		" ### ",
	},
	'2': {
		" ### ",
		"#   #",
		"    #",
		"   # ",
		"  #  ",
		" #   ",
		"#####",
	},
	'3': {
		" ### ",
		"#   #",
		"    #",
		" ### ",
		"    #",
		"#   #",
		" ### ",
	},
	'4': {
		"   # ",
		"  ## ",
		" # # ",
		"#  # ",
		"#####",
		"   # ",
		"   # ",
	},
	'5': {
		"#####",
		"#    ",
		"#    ",
		"#### ",
		"    #",
		"#   #",
		" ### ",
	},
	'6': {
		" ### ",
		"#   #",
		"#    ",
		"#### ",
		"#   #",
		"#   #",
		" ### ",
	},
	'8': {
		" ### ",
		"#   #",
		"#   #",
		" ### ",
		"#   #",
		"#   #",
		" ### ",
	},
	'B': {
		"#### ",
		"#   #",
		"#   #",
		"#### ",
		"#   #",
		"#   #",
		"#### ",
	},
	'A': {
		" ### ",
		"#   #",
		"#   #",
		"#####",
		"#   #",
		"#   #",
		"#   #",
	},
	'C': {
		" ####",
		"#    ",
		"#    ",
		"#    ",
		"#    ",
		"#    ",
		" ####",
	},
	'E': {
		"#####",
		"#    ",
		"#    ",
		"#### ",
		"#    ",
		"#    ",
		"#####",
	},
	'I': {
		"#####",
		"  #  ",
		"  #  ",
		"  #  ",
		"  #  ",
		"  #  ",
		"#####",
	},
	'K': {
		"#   #",
		"#  # ",
		"# #  ",
		"##   ",
		"# #  ",
		"#  # ",
		"#   #",
	},
	'M': {
		"#   #",
		"## ##",
		"# # #",
		"#   #",
		"#   #",
		"#   #",
		"#   #",
	},
	'S': {
		" ####",
		"#    ",
		"#    ",
		" ### ",
		"    #",
		"    #",
		"#### ",
	},
	'X': {
		"#   #",
		"#   #",
		" # # ",
		"  #  ",
		" # # ",
		"#   #",
		"#   #",
	},
	'Z': {
		"#####",
		"    #",
		"   # ",
		"  #  ",
		" #   ",
		"#    ",
		"#####",
	},
}

func main() {
	basePath := filepath.Clean("sdk/examples/assets/rotozoomtexture.raw")
	base, err := os.ReadFile(basePath)
	if err != nil {
		failf("read base raw texture %s: %v", basePath, err)
	}
	wantSize := rotoTextureWidth * rotoTextureHeight * rotoPixelBytes
	if len(base) != wantSize {
		failf("unexpected base raw texture size %d, want %d", len(base), wantSize)
	}

	for _, variant := range rotoVariants {
		out := append([]byte(nil), base...)
		drawLabelPlate(out)
		drawLabel(out, variant.label)

		if err := os.WriteFile(variant.rawPath, out, 0o644); err != nil {
			failf("write %s: %v", variant.rawPath, err)
		}
		if err := writePNG(out, variant.pngPath); err != nil {
			failf("write %s: %v", variant.pngPath, err)
		}
		fmt.Printf("generated %s and %s\n", variant.rawPath, variant.pngPath)
	}
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func writePNG(raw []byte, path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, rotoTextureWidth, rotoTextureHeight))
	copy(img.Pix, raw)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func drawLabelPlate(raw []byte) {
	const (
		padding = 10
		panelX  = 20
		panelY  = 182
		panelW  = 216
		panelH  = 56
	)
	for y := panelY; y < panelY+panelH; y++ {
		for x := panelX; x < panelX+panelW; x++ {
			alpha := uint8(150)
			if x < panelX+padding || x >= panelX+panelW-padding || y < panelY+padding || y >= panelY+panelH-padding {
				alpha = 110
			}
			blendPixel(raw, x, y, 12, 16, 24, alpha)
		}
	}
}

func drawLabel(raw []byte, label string) {
	const (
		scale   = 6
		spacing = 1
	)
	width := labelWidth(label, scale, spacing)
	height := 7 * scale
	startX := (rotoTextureWidth - width) / 2
	startY := 189 + ((42 - height) / 2)

	drawText(raw, label, startX+2, startY+2, scale, spacing, 0, 0, 0, 190)
	drawText(raw, label, startX-1, startY, scale, spacing, 0, 0, 0, 160)
	drawText(raw, label, startX+1, startY, scale, spacing, 0, 0, 0, 160)
	drawText(raw, label, startX, startY-1, scale, spacing, 0, 0, 0, 160)
	drawText(raw, label, startX, startY+1, scale, spacing, 0, 0, 0, 160)
	drawText(raw, label, startX, startY, scale, spacing, 255, 248, 232, 255)
}

func labelWidth(label string, scale, spacing int) int {
	width := 0
	for i, r := range label {
		glyph := glyphs[r]
		if len(glyph) == 0 {
			continue
		}
		if i > 0 {
			width += spacing * scale
		}
		width += len(glyph[0]) * scale
	}
	return width
}

func drawText(raw []byte, text string, startX, startY, scale, spacing int, r, g, b, a uint8) {
	x := startX
	for _, ch := range text {
		glyph := glyphs[ch]
		if len(glyph) == 0 {
			x += (5 + spacing) * scale
			continue
		}
		drawGlyph(raw, glyph, x, startY, scale, r, g, b, a)
		x += (len(glyph[0]) + spacing) * scale
	}
}

func drawGlyph(raw []byte, glyph []string, startX, startY, scale int, r, g, b, a uint8) {
	for gy, row := range glyph {
		for gx, cell := range row {
			if cell == ' ' {
				continue
			}
			for sy := range scale {
				for sx := range scale {
					blendPixel(raw, startX+gx*scale+sx, startY+gy*scale+sy, r, g, b, a)
				}
			}
		}
	}
}

func blendPixel(raw []byte, x, y int, r, g, b, a uint8) {
	if x < 0 || x >= rotoTextureWidth || y < 0 || y >= rotoTextureHeight {
		return
	}
	offset := (y*rotoTextureWidth + x) * rotoPixelBytes
	alpha := int(a)
	inv := 255 - alpha
	raw[offset+0] = byte((int(raw[offset+0])*inv + int(r)*alpha) / 255)
	raw[offset+1] = byte((int(raw[offset+1])*inv + int(g)*alpha) / 255)
	raw[offset+2] = byte((int(raw[offset+2])*inv + int(b)*alpha) / 255)
	raw[offset+3] = 255
}
