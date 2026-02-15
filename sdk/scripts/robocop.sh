#!/usr/bin/env bash
set -euo pipefail

PNG="sdk/examples/assets/robocop.png"
ASM="sdk/examples/asm/robocop_intro.asm"
RGBA="sdk/examples/assets/robocop_rgba.bin"
MASK="sdk/examples/assets/robocop_mask.bin"

if [[ ! -f "$PNG" ]]; then
  echo "Missing $PNG"
  exit 1
fi
if [[ ! -f "$ASM" ]]; then
  echo "Missing $ASM"
  exit 1
fi

# Verify size is 240x180 (requires ImageMagick identify)
SIZE=$(identify -format "%wx%h" "$PNG")
if [[ "$SIZE" != "240x180" ]]; then
  echo "Expected 240x180, got $SIZE"
  exit 1
fi

# RGBA bin
convert "$PNG" -depth 8 rgba:"$RGBA"

# Mask: alpha -> 1-bit, pack MSB-first (bit 7 = first pixel)
# Generate mask from RGBA alpha channel using Go for reliability
TMPGO=$(mktemp --suffix=.go)
cat > "$TMPGO" <<'GOSCRIPT'
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: gen_mask <rgba.bin> <mask.bin>")
		os.Exit(1)
	}
	rgba, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error reading RGBA:", err)
		os.Exit(1)
	}

	width, height := 240, 180
	pixels := width * height
	if len(rgba) < pixels*4 {
		fmt.Fprintln(os.Stderr, "RGBA file too small")
		os.Exit(1)
	}

	mask := make([]byte, (pixels+7)/8)
	for i := 0; i < pixels; i++ {
		alpha := rgba[i*4+3]
		if alpha > 0 {
			// MSB-first: bit 7 = first pixel in byte
			mask[i/8] |= (1 << (7 - (i % 8)))
		}
	}

	if err := os.WriteFile(os.Args[2], mask, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "Error writing mask:", err)
		os.Exit(1)
	}
	fmt.Printf("Generated mask: %d bytes, %d opaque pixels\n", len(mask), pixels)
}
GOSCRIPT
go run "$TMPGO" "$RGBA" "$MASK"
rm -f "$TMPGO"

# Assemble (outputs .iex beside the .asm)
sdk/bin/ie32asm "$ASM"

echo "Built ${ASM%.asm}.iex"
