//go:build ie64

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestIE64CProcCRTIsLinkableAndUsesFrozenStack(t *testing.T) {
	crt, err := os.ReadFile("../sdk/lib/ie64-cproc/crt0.s")
	if err != nil {
		t.Fatalf("read crt0.s: %v", err)
	}
	source := string(crt)
	if strings.Contains(strings.ToLower(source), "\norg") {
		t.Fatal("crt0.s must be linkable: the driver, not the CRT, owns the image origin")
	}
	if !strings.Contains(source, "#0x0009F000") {
		t.Fatal("crt0.s does not use the frozen bare-metal stack top")
	}
	if !strings.Contains(source, "mfcr    r1, cr15") {
		t.Fatal("crt0.s does not read CR_RAM_SIZE_BYTES")
	}

	image, err := NewIE64Assembler().Assemble("org 0x1000\n" + source + "\nmain:\n halt\n")
	if err != nil {
		t.Fatalf("assemble CRT composition: %v", err)
	}
	if len(image) != 96 {
		t.Fatalf("CRT composition length = %d, want 96 bytes", len(image))
	}
	if !bytes.Equal(image[:8], encodeInstr(OP64_MOVE, 31, SIZE_L, 1, 0, 0, 0x9f000)) {
		t.Fatalf("first instruction does not initialise R31 to 0x9f000: % x", image[:8])
	}
}
