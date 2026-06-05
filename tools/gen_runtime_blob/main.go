// Command gen_runtime_blob generates the standalone COMPILE runtime blob that the
// EhBASIC COMPILE path bundles into standalone .ie64 images.
//
// It assembles sdk/include/aot_runtime_blob.asm (the expr/var/string/maths/io/exec
// closure linked at AOT_RT_BASE) with ie64asm, then strips the leading
// [PROG_START, AOT_RT_BASE) origin padding ie64asm keeps, so byte 0 of the output
// maps to guest AOT_RT_BASE. The result is written to aot_runtime_blob.bin.
//
// The blob is deterministic from the committed runtime sources and is NOT committed
// as a binary (the repo ignores *.bin); the COMPILE path reads the generated .bin
// over the File I/O ABI at compile time. The same assemble+trim is exercised by
// TestAOTRuntimeBlob in ehbasic_aot_runtime_blob_test.go.
//
// Usage: go run ./tools/gen_runtime_blob [-asm sdk/bin/ie64asm] [-out sdk/include/aot_runtime_blob.bin]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	progStart    = 0x001000 // PROG_START
	aotRTBase    = 0x043000 // AOT_RT_BASE
	aotRTLimit   = 0x070000 // AOT_RT_LIMIT
	aotRTBlobMax = 0x10000  // AOT_RT_BLOB_MAX: compile-time staging size; hard cap
	orgPad       = aotRTBase - progStart
)

func main() {
	asm := flag.String("asm", "sdk/bin/ie64asm", "path to the ie64asm binary")
	inc := flag.String("inc", "sdk/include", "include directory")
	src := flag.String("src", "sdk/include/aot_runtime_blob.asm", "runtime blob source")
	out := flag.String("out", "sdk/include/aot_runtime_blob.bin", "trimmed blob output")
	flag.Parse()

	tmp, err := os.MkdirTemp("", "runtime_blob")
	if err != nil {
		fail("temp dir: %v", err)
	}
	defer os.RemoveAll(tmp)

	rawPath := filepath.Join(tmp, "aot_runtime_blob.ie64")
	cmd := exec.Command(*asm, "-I", *inc, "-o", rawPath, *src)
	if o, err := cmd.CombinedOutput(); err != nil {
		fail("assemble %s: %v\n%s", *src, err, o)
	}
	raw, err := os.ReadFile(rawPath)
	if err != nil {
		fail("read assembled blob: %v", err)
	}

	if len(raw) <= orgPad {
		fail("assembled blob %d bytes <= org pad %d: nothing emitted at AOT_RT_BASE", len(raw), orgPad)
	}
	for i := 0; i < orgPad; i++ {
		if raw[i] != 0 {
			fail("non-zero byte %#02x at offset %#x in the org pad: padding not clean", raw[i], i)
		}
	}
	blob := raw[orgPad:]
	if top := aotRTBase + len(blob); top > aotRTLimit {
		fail("trimmed blob %d bytes; top %#x exceeds placement-B limit %#x", len(blob), top, aotRTLimit)
	}
	if len(blob) > aotRTBlobMax {
		fail("trimmed blob %d bytes exceeds AOT_RT_BLOB_MAX %#x: it would overflow the compile-time staging buffer; "+
			"raise AOT_RT_BLOB_MAX (and the staging allocation) in ie64.inc and this tool together", len(blob), aotRTBlobMax)
	}

	if err := os.WriteFile(*out, blob, 0644); err != nil {
		fail("write %s: %v", *out, err)
	}
	fmt.Printf("gen_runtime_blob: wrote %d bytes to %s (occupies [%#x, %#x))\n",
		len(blob), *out, aotRTBase, aotRTBase+len(blob))
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "gen_runtime_blob: "+format+"\n", a...)
	os.Exit(1)
}
