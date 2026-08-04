package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostSDKPublicHeaderContract(t *testing.T) {
	header, err := os.ReadFile("sdk/include/intuitionengine.h")
	if err != nil {
		t.Fatal(err)
	}
	text := string(header)
	for _, want := range []string{
		"IE_TARGET_IE64", "IE_TARGET_M68K", "IE_TARGET_Z80", "IE_TARGET_6502", "IE_TARGET_X86",
		"IE_HAS_BANK_WINDOWS", "IE_HAS_X86_PORT_IO", "IE_HAS_IE64_CONTROL_REGISTERS", "IE_HAS_IE64_ATOMICS", "IE_HAS_IE64_FPU",
		"IE_VIDEO_", "IE_AUDIO_", "IE_INPUT_", "IE_FILE_", "IE_NET_", "IE_COPROC_", "IE_VOODOO_",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("public header is missing %q", want)
		}
	}
	for _, forbidden := range []string{"sdk/include/ie64.h", "ie64/platform.h"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("public header mentions removed private header %q", forbidden)
		}
	}
	if !strings.Contains(text, "#define IE_COPROC_BASE 0x0f2340u") {
		t.Fatal("public header has an incorrect coprocessor MMIO base")
	}
}

func TestHostSDKAssemblyIncludeInventory(t *testing.T) {
	for _, name := range []string{"ie32.inc", "ie64.inc", "ie68.inc", "ie80.inc", "ie65.inc", "ie86.inc"} {
		if _, err := os.Stat(filepath.Join("sdk", "include", name)); err != nil {
			t.Fatalf("missing public assembly include %s: %v", name, err)
		}
	}
	for _, removed := range []string{"ie64.h", filepath.Join("ie64", "platform.h")} {
		if _, err := os.Stat(filepath.Join("sdk", "include", removed)); !os.IsNotExist(err) {
			t.Fatalf("removed private header remains %s", removed)
		}
	}
}
