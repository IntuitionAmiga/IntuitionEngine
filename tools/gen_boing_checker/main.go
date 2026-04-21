package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

const (
	texW     = 64
	texH     = 64
	cellSize = 8
	bytesPP  = 4
	alpha    = 0xFF
)

var (
	boingRed   = [4]byte{0xD8, 0x18, 0x20, alpha}
	boingWhite = [4]byte{0xF0, 0xF0, 0xF0, alpha}
)

func main() {
	_, selfPath, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Println("Error: unable to resolve generator path")
		os.Exit(1)
	}
	outPath := filepath.Join(filepath.Dir(selfPath), "..", "..", "sdk", "examples", "assets", "boing_checker_64.bin")

	buf := make([]byte, texW*texH*bytesPP)
	for y := range texH {
		for x := range texW {
			cellX := x / cellSize
			cellY := y / cellSize
			color := boingWhite
			if (cellX+cellY)%2 == 0 {
				color = boingRed
			}
			i := (y*texW + x) * bytesPP
			copy(buf[i:i+bytesPP], color[:])
		}
	}

	if err := os.WriteFile(outPath, buf, 0644); err != nil {
		fmt.Printf("Error writing texture: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %s (%dx%d RGBA, %d bytes)\n", outPath, texW, texH, len(buf))
}
