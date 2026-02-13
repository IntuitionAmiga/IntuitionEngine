package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// cpuTarget describes how to assemble one rotozoomer variant and how its
// tables are encoded in the resulting binary.
type cpuTarget struct {
	name       string
	asmFile    string
	binExt     string
	entryWidth int  // bytes per table entry: 2 (16-bit) or 4 (32-bit)
	bigEndian  bool // true for M68K, false for all others
	assemble   func(t *testing.T, srcFile, outFile string)
}

// rotozoomerRepoRoot returns the project root directory.
func rotozoomerRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve repo root")
	}
	return filepath.Dir(file)
}

// buildIE32Assembler builds the IE32 assembler from source and returns
// the path to the compiled binary.
func buildIE32Assembler(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "ie32asm")
	cmd := exec.Command("go", "build", "-o", binPath,
		filepath.Join(rotozoomerRepoRoot(t), "assembler", "ie32asm.go"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build ie32asm: %v\n%s", err, out)
	}
	return binPath
}

// buildIE64Assembler builds the IE64 assembler from source and returns
// the path to the compiled binary.
func buildIE64Assembler(t *testing.T) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "ie64asm")
	cmd := exec.Command("go", "build", "-tags", "ie64", "-o", binPath,
		filepath.Join(rotozoomerRepoRoot(t), "assembler", "ie64asm.go"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build ie64asm: %v\n%s", err, out)
	}
	return binPath
}

// requireTool skips the test if the named tool is not on PATH.
func requireTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not found, skipping", name)
	}
}

// computeRefSine returns the 256-entry reference sine table.
// Each entry is round(sin(i * 2π / 256) * 256), range -256..+256.
func computeRefSine() [256]int16 {
	var tbl [256]int16
	for i := 0; i < 256; i++ {
		angle := float64(i) * 2.0 * math.Pi / 256.0
		tbl[i] = int16(math.Round(math.Sin(angle) * 256.0))
	}
	return tbl
}

// computeRefRecip returns the 256-entry reference reciprocal table.
// Each entry is round(256 / (0.5 + sin(i * 2π / 256) * 0.3)), range 320..1280.
func computeRefRecip() [256]uint16 {
	var tbl [256]uint16
	for i := 0; i < 256; i++ {
		angle := float64(i) * 2.0 * math.Pi / 256.0
		tbl[i] = uint16(math.Round(256.0 / (0.5 + math.Sin(angle)*0.3)))
	}
	return tbl
}

// buildSentinel creates the byte pattern for the first 4 sine table entries
// given the target's entry width and byte order.
func buildSentinel(refSine [256]int16, entryWidth int, bigEndian bool) []byte {
	n := 4
	sentinel := make([]byte, n*entryWidth)
	for i := 0; i < n; i++ {
		v := refSine[i]
		switch {
		case entryWidth == 2 && !bigEndian:
			binary.LittleEndian.PutUint16(sentinel[i*2:], uint16(v))
		case entryWidth == 2 && bigEndian:
			binary.BigEndian.PutUint16(sentinel[i*2:], uint16(v))
		case entryWidth == 4 && !bigEndian:
			binary.LittleEndian.PutUint32(sentinel[i*4:], uint32(int32(v)))
		case entryWidth == 4 && bigEndian:
			binary.BigEndian.PutUint32(sentinel[i*4:], uint32(int32(v)))
		}
	}
	return sentinel
}

// findAndValidateSineTable scans the binary for the sine table sentinel,
// validates the candidate, and returns the byte offset or -1.
func findAndValidateSineTable(t *testing.T, data []byte, entryWidth int, bigEndian bool, refSine [256]int16) int {
	t.Helper()
	sentinel := buildSentinel(refSine, entryWidth, bigEndian)
	tableBytes := 256 * entryWidth
	recipBytes := 256 * entryWidth

	for off := 0; off <= len(data)-tableBytes-recipBytes; off++ {
		if !bytes.Equal(data[off:off+len(sentinel)], sentinel) {
			continue
		}

		// Candidate found — validate non-decreasing in first quadrant (entries 0..63)
		valid := true
		for i := 1; i <= 63; i++ {
			v := readSignedEntry(data, off+i*entryWidth, entryWidth, bigEndian)
			prev := readSignedEntry(data, off+(i-1)*entryWidth, entryWidth, bigEndian)
			if v < prev {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}

		// Validate reciprocal entries are all in range 320..1280
		recipOff := off + tableBytes
		recipValid := true
		for i := 0; i < 256; i++ {
			rv := readUnsignedEntry(data, recipOff+i*entryWidth, entryWidth, bigEndian)
			if rv < 320 || rv > 1280 {
				recipValid = false
				break
			}
		}
		if !recipValid {
			continue
		}

		return off
	}
	return -1
}

// readSignedEntry reads a signed integer from the binary at the given offset.
func readSignedEntry(data []byte, off, width int, bigEndian bool) int16 {
	switch {
	case width == 2 && !bigEndian:
		return int16(binary.LittleEndian.Uint16(data[off:]))
	case width == 2 && bigEndian:
		return int16(binary.BigEndian.Uint16(data[off:]))
	case width == 4 && !bigEndian:
		return int16(int32(binary.LittleEndian.Uint32(data[off:])))
	case width == 4 && bigEndian:
		return int16(int32(binary.BigEndian.Uint32(data[off:])))
	}
	return 0
}

// readUnsignedEntry reads an unsigned integer from the binary at the given offset.
func readUnsignedEntry(data []byte, off, width int, bigEndian bool) uint16 {
	switch {
	case width == 2 && !bigEndian:
		return binary.LittleEndian.Uint16(data[off:])
	case width == 2 && bigEndian:
		return binary.BigEndian.Uint16(data[off:])
	case width == 4 && !bigEndian:
		return uint16(binary.LittleEndian.Uint32(data[off:]))
	case width == 4 && bigEndian:
		return uint16(binary.BigEndian.Uint32(data[off:]))
	}
	return 0
}

func TestRotozoomerTables(t *testing.T) {
	refSine := computeRefSine()
	refRecip := computeRefRecip()

	root := rotozoomerRepoRoot(t)
	asmDir := filepath.Join(root, "assembler")

	targets := []cpuTarget{
		{
			name: "M68K", asmFile: "rotozoomer_68k.asm", binExt: ".ie68",
			entryWidth: 2, bigEndian: true,
			assemble: func(t *testing.T, src, out string) {
				requireTool(t, "vasmm68k_mot")
				cmd := exec.Command("vasmm68k_mot", "-Fbin", "-m68020", "-devpac", "-o", out, src)
				cmd.Dir = asmDir
				if o, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("vasmm68k_mot failed: %v\n%s", err, o)
				}
			},
		},
		{
			name: "IE64", asmFile: "rotozoomer_ie64.asm", binExt: ".ie64",
			entryWidth: 2, bigEndian: false,
			assemble: func(t *testing.T, src, out string) {
				ie64asm := buildIE64Assembler(t)
				cmd := exec.Command(ie64asm, src)
				cmd.Dir = asmDir
				if o, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("ie64asm failed: %v\n%s", err, o)
				}
				// ie64asm writes output alongside source with .ie64 extension
				generated := src[:len(src)-4] + ".ie64"
				data, err := os.ReadFile(generated)
				if err != nil {
					t.Fatalf("reading ie64 binary: %v", err)
				}
				if err := os.WriteFile(out, data, 0644); err != nil {
					t.Fatalf("writing ie64 binary: %v", err)
				}
			},
		},
		{
			name: "x86", asmFile: "rotozoomer_x86.asm", binExt: ".ie86",
			entryWidth: 2, bigEndian: false,
			assemble: func(t *testing.T, src, out string) {
				requireTool(t, "nasm")
				cmd := exec.Command("nasm", "-f", "bin", "-I", asmDir+"/", "-o", out, src)
				if o, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("nasm failed: %v\n%s", err, o)
				}
			},
		},
		{
			name: "IE32", asmFile: "rotozoomer.asm", binExt: ".iex",
			entryWidth: 4, bigEndian: false,
			assemble: func(t *testing.T, src, out string) {
				ie32asm := buildIE32Assembler(t)
				cmd := exec.Command(ie32asm, src)
				cmd.Dir = asmDir
				if o, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("ie32asm failed: %v\n%s", err, o)
				}
				// ie32asm writes output alongside source with .iex extension
				generated := src[:len(src)-4] + ".iex"
				data, err := os.ReadFile(generated)
				if err != nil {
					t.Fatalf("reading ie32 binary: %v", err)
				}
				if err := os.WriteFile(out, data, 0644); err != nil {
					t.Fatalf("writing ie32 binary: %v", err)
				}
			},
		},
		{
			name: "Z80", asmFile: "rotozoomer_z80.asm", binExt: ".ie80",
			entryWidth: 2, bigEndian: false,
			assemble: func(t *testing.T, src, out string) {
				requireTool(t, "vasmz80_std")
				cmd := exec.Command("vasmz80_std", "-Fbin", "-I", asmDir, "-o", out, src)
				if o, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("vasmz80_std failed: %v\n%s", err, o)
				}
			},
		},
		{
			name: "6502", asmFile: "rotozoomer_65.asm", binExt: ".ie65",
			entryWidth: 2, bigEndian: false,
			assemble: func(t *testing.T, src, out string) {
				requireTool(t, "ca65")
				requireTool(t, "ld65")
				tmpDir := t.TempDir()
				objFile := filepath.Join(tmpDir, "rotozoom.o")
				cmd := exec.Command("ca65", "-I", asmDir, "-o", objFile, src)
				if o, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("ca65 failed: %v\n%s", err, o)
				}
				cfgFile := filepath.Join(asmDir, "ie65.cfg")
				cmd = exec.Command("ld65", "-C", cfgFile, "-o", out, objFile)
				if o, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("ld65 failed: %v\n%s", err, o)
				}
			},
		},
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			srcFile := filepath.Join(asmDir, target.asmFile)
			tmpDir := t.TempDir()
			outFile := filepath.Join(tmpDir, "rotozoom"+target.binExt)

			target.assemble(t, srcFile, outFile)

			data, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatalf("reading binary: %v", err)
			}

			sineOff := findAndValidateSineTable(t, data, target.entryWidth, target.bigEndian, refSine)
			if sineOff < 0 {
				t.Fatalf("sine table sentinel not found in %d-byte binary", len(data))
			}

			// Validate all 256 sine entries
			for i := 0; i < 256; i++ {
				got := readSignedEntry(data, sineOff+i*target.entryWidth, target.entryWidth, target.bigEndian)
				if got != refSine[i] {
					t.Errorf("sine[%d]: got %d, want %d", i, got, refSine[i])
				}
			}

			// Validate all 256 reciprocal entries
			recipOff := sineOff + 256*target.entryWidth
			for i := 0; i < 256; i++ {
				got := readUnsignedEntry(data, recipOff+i*target.entryWidth, target.entryWidth, target.bigEndian)
				if got != refRecip[i] {
					t.Errorf("recip[%d]: got %d, want %d", i, got, refRecip[i])
				}
			}

			// Sweep 32 angles × 4 scale buckets (128 pairs)
			// Compute CA/SA/u0/v0 and compare against Go reference
			for ai := 0; ai < 32; ai++ {
				angleIdx := ai * 8 // 0, 8, 16, ..., 248
				for si := 0; si < 4; si++ {
					scaleIdx := si * 64 // 0, 64, 128, 192

					cosIdx := (angleIdx + 64) & 255
					sinVal := int32(refSine[angleIdx])
					cosVal := int32(refSine[cosIdx])
					recipVal := int32(refRecip[scaleIdx])

					// CA = cos * recip (16.16 fixed-point)
					ca := cosVal * recipVal
					// SA = sin * recip
					sa := sinVal * recipVal

					// u0 = 8388608 - (CA*320) + (SA*240)
					// CA*320 = CA<<8 + CA<<6 = CA*256 + CA*64
					ca320 := ca*256 + ca*64
					// SA*240 = SA<<8 - SA<<4 = SA*256 - SA*16
					sa240 := sa*256 - sa*16
					u0 := int32(8388608) - ca320 + sa240

					// v0 = 8388608 - (SA*320) - (CA*240)
					sa320 := sa*256 + sa*64
					ca240 := ca*256 - ca*16
					v0 := int32(8388608) - sa320 - ca240

					// Read the same values from the embedded tables
					embCos := int32(readSignedEntry(data, sineOff+cosIdx*target.entryWidth, target.entryWidth, target.bigEndian))
					embSin := int32(readSignedEntry(data, sineOff+angleIdx*target.entryWidth, target.entryWidth, target.bigEndian))
					embRecip := int32(readUnsignedEntry(data, recipOff+scaleIdx*target.entryWidth, target.entryWidth, target.bigEndian))

					embCA := embCos * embRecip
					embSA := embSin * embRecip
					embCA320 := embCA*256 + embCA*64
					embSA240 := embSA*256 - embSA*16
					embU0 := int32(8388608) - embCA320 + embSA240
					embSA320 := embSA*256 + embSA*64
					embCA240 := embCA*256 - embCA*16
					embV0 := int32(8388608) - embSA320 - embCA240

					if embCA != ca {
						t.Errorf("angle=%d scale=%d: CA got %d, want %d", angleIdx, scaleIdx, embCA, ca)
					}
					if embSA != sa {
						t.Errorf("angle=%d scale=%d: SA got %d, want %d", angleIdx, scaleIdx, embSA, sa)
					}
					if embU0 != u0 {
						t.Errorf("angle=%d scale=%d: u0 got %d, want %d", angleIdx, scaleIdx, embU0, u0)
					}
					if embV0 != v0 {
						t.Errorf("angle=%d scale=%d: v0 got %d, want %d", angleIdx, scaleIdx, embV0, v0)
					}
				}
			}

			t.Logf("OK: sine table at offset 0x%X, recip table at offset 0x%X, 128 sweep pairs validated",
				sineOff, recipOff)
		})
	}
}
