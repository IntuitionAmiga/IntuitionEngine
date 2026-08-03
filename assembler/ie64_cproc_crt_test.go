//go:build ie64

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/intuitionamiga/IntuitionEngine/internal/ie64link"
	"github.com/intuitionamiga/IntuitionEngine/internal/ie64obj"
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

	crtData, err := AssembleIE64Object(source, "crt0.s", nil, nil)
	if err != nil {
		t.Fatalf("assemble CRT object: %v", err)
	}
	crtObject, err := ie64obj.Parse(crtData)
	if err != nil {
		t.Fatalf("parse CRT object: %v", err)
	}
	supportData, err := AssembleIE64Object(`.section .text,"ax"
.global main
.global __libc_init_array
.global exit
main:
    halt
__libc_init_array:
    rts
exit:
    halt
`, "support.s", nil, nil)
	if err != nil {
		t.Fatalf("assemble CRT support object: %v", err)
	}
	supportObject, err := ie64obj.Parse(supportData)
	if err != nil {
		t.Fatalf("parse CRT support object: %v", err)
	}
	result, err := ie64link.Link([]ie64link.Input{
		{Name: "crt0.o", Object: crtObject},
		{Name: "support.o", Object: supportObject},
	}, ie64link.Options{Entry: "_start"})
	if err != nil {
		t.Fatalf("link CRT composition: %v", err)
	}
	if !bytes.Equal(result.Image[:8], encodeInstr(OP64_MOVE, 31, SIZE_L, 1, 0, 0, 0x9f000)) {
		t.Fatalf("first instruction does not initialise R31 to 0x9f000: % x", result.Image[:8])
	}
}
