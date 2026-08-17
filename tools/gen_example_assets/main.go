// Command gen_example_assets creates the binary assets consumed by the
// assembler examples and their parity tests.
package main

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

const (
	width  = 240
	height = 180
)

func main() {
	root := repoRoot()
	pngPath := filepath.Join(root, "sdk", "examples", "assets", "robocop.png")
	outDir := filepath.Dir(pngPath)

	f, err := os.Open(pngPath)
	if err != nil {
		fatal(err)
	}
	img, err := png.Decode(f)
	_ = f.Close()
	if err != nil {
		fatal(err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		fatalf("%s is %dx%d, want %dx%d", pngPath, bounds.Dx(), bounds.Dy(), width, height)
	}

	rgba := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)
	if err := os.WriteFile(filepath.Join(outDir, "robocop_rgba.bin"), rgba.Pix, 0o644); err != nil {
		fatal(err)
	}

	mask := make([]byte, (width*height+7)/8)
	for i := 0; i < width*height; i++ {
		if rgba.Pix[i*4+3] != 0 {
			mask[i/8] |= 1 << (7 - i%8)
		}
	}
	if err := os.WriteFile(filepath.Join(outDir, "robocop_mask.bin"), mask, 0o644); err != nil {
		fatal(err)
	}
}

func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	return wd
}

func fatal(err error) {
	fatalf("%v", err)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen_example_assets: "+format+"\n", args...)
	os.Exit(1)
}
