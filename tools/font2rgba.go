// font2rgba.go - Convert cubeintro PNG font to raw RGBA for IE32 blitter
//
// Usage: go run font2rgba.go
// Output: ../assembler/font_rgba.bin

package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
)

func main() {
	// Read the PNG font from cubeintro
	fontPath := "/home/zayn/GolandProjects/cubeintro/font.go"

	// We need to extract the byte array from font.go
	// For simplicity, decode the PNG directly from the cubeintro package
	// by reading the embedded PNG data

	// Actually, let's read the PNG file if it exists, or extract from font.go
	// The font.go contains fontPng as a byte array starting with PNG header

	// Read font.go and extract the byte slice
	fontData, err := extractPNGFromGoFile(fontPath)
	if err != nil {
		fmt.Printf("Error extracting PNG: %v\n", err)
		os.Exit(1)
	}

	// Decode PNG
	img, err := png.Decode(bytes.NewReader(fontData))
	if err != nil {
		fmt.Printf("Error decoding PNG: %v\n", err)
		os.Exit(1)
	}

	bounds := img.Bounds()
	fmt.Printf("Font image size: %dx%d\n", bounds.Dx(), bounds.Dy())

	// Convert to RGBA
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	// Make black pixels transparent (set alpha=0 for near-black pixels)
	// This allows the blitter to skip these pixels when using alpha-aware copy
	for i := 0; i < len(rgba.Pix); i += 4 {
		r := rgba.Pix[i]
		g := rgba.Pix[i+1]
		b := rgba.Pix[i+2]
		// If pixel is near-black (all RGB components < 16), make it transparent
		if r < 16 && g < 16 && b < 16 {
			rgba.Pix[i+3] = 0 // Set alpha to 0 (transparent)
		}
	}

	// Write raw RGBA to file
	outPath := "/home/zayn/GolandProjects/IntuitionEngine/assembler/font_rgba.bin"
	err = os.WriteFile(outPath, rgba.Pix, 0644)
	if err != nil {
		fmt.Printf("Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Written %d bytes to %s\n", len(rgba.Pix), outPath)
	fmt.Printf("Font dimensions: %dx%d pixels\n", bounds.Dx(), bounds.Dy())
	fmt.Printf("Characters: %d columns x %d rows = %d chars\n",
		bounds.Dx()/32, bounds.Dy()/32, (bounds.Dx()/32)*(bounds.Dy()/32))
}

func extractPNGFromGoFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Find the start of the byte array (0x89, 0x50, 0x4e, 0x47 = PNG header)
	content := string(data)

	// Parse the Go file to extract byte values
	// Look for "var fontPng = []byte{"
	start := bytes.Index(data, []byte("var fontPng = []byte{"))
	if start == -1 {
		return nil, fmt.Errorf("could not find fontPng in file")
	}

	// Find the opening brace
	braceStart := bytes.Index(data[start:], []byte("{"))
	if braceStart == -1 {
		return nil, fmt.Errorf("could not find opening brace")
	}
	start += braceStart + 1

	// Find the closing brace
	braceEnd := bytes.Index(data[start:], []byte("}"))
	if braceEnd == -1 {
		return nil, fmt.Errorf("could not find closing brace")
	}

	byteSection := string(data[start : start+braceEnd])

	// Parse hex values like 0x89, 0x50, etc.
	var result []byte
	var current uint8
	inHex := false
	hexStr := ""

	for i := 0; i < len(byteSection); i++ {
		c := byteSection[i]

		if c == '0' && i+1 < len(byteSection) && byteSection[i+1] == 'x' {
			inHex = true
			hexStr = ""
			i++ // skip 'x'
			continue
		}

		if inHex {
			if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
				hexStr += string(c)
			} else {
				// End of hex value
				if len(hexStr) > 0 {
					_, err := fmt.Sscanf("0x"+hexStr, "0x%x", &current)
					if err == nil {
						result = append(result, current)
					}
				}
				inHex = false
				hexStr = ""
			}
		}
	}

	// Handle last value if still in hex
	if inHex && len(hexStr) > 0 {
		_, err := fmt.Sscanf("0x"+hexStr, "0x%x", &current)
		if err == nil {
			result = append(result, current)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no bytes extracted from file")
	}

	// Verify PNG header
	if len(result) < 8 || result[0] != 0x89 || result[1] != 0x50 || result[2] != 0x4e || result[3] != 0x47 {
		return nil, fmt.Errorf("extracted data is not a valid PNG (got %d bytes, header: %x)", len(result), result[:min(8, len(result))])
	}

	fmt.Printf("Extracted %d bytes of PNG data\n", len(result))
	_ = content // silence unused warning

	return result, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
