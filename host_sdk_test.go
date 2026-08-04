package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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
	defineRE := regexp.MustCompile(`(?m)^#define ([A-Z0-9_]+) 0x([0-9a-fA-F]+)u$`)
	defines := make(map[string]uint64)
	for _, match := range defineRE.FindAllStringSubmatch(text, -1) {
		value, err := strconv.ParseUint(match[2], 16, 64)
		if err != nil {
			t.Fatalf("parse %s: %v", match[1], err)
		}
		defines[match[1]] = value
	}
	for name, want := range map[string]uint64{
		"IE_VIDEO_CTRL":      VIDEO_CTRL,
		"IE_INPUT_TERM_OUT":  TERM_OUT,
		"IE_AUDIO_BASE":      AUDIO_CTRL,
		"IE_FILE_BASE":       FILE_IO_BASE,
		"IE_EXEC_BASE":       EXEC_BASE,
		"IE_COPROC_BASE":     COPROC_BASE,
		"IE_NET_SOCKET_BASE": HOST_SOCKET_BASE,
		"IE_VOODOO_BASE":     VOODOO_BASE,
	} {
		if got, ok := defines[name]; !ok || got != want {
			t.Errorf("public header %s = 0x%X, want executable-source value 0x%X", name, got, want)
		}
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
