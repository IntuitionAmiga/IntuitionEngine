//go:build (amd64 && (linux || windows)) || (arm64 && (linux || windows || darwin))

package main

import (
	"os"
	"runtime"
	"testing"
)

var requireJIT = os.Getenv("IE_REQUIRE_JIT") == "1"

func TestJIT_IE64_Availability(t *testing.T) {
	if requireJIT && !jitAvailable {
		t.Fatalf("IE64 JIT unavailable on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func TestJIT_6502_Availability(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		return
	}
	if requireJIT && !jit6502Available {
		t.Fatalf("6502 JIT unavailable on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func TestJIT_M68K_Availability(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		return
	}
	if requireJIT && !m68kJitAvailable {
		t.Fatalf("M68K JIT unavailable on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func TestJIT_X86_Availability(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		return
	}
	if requireJIT && !x86JitAvailable {
		t.Fatalf("x86 JIT unavailable on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}

func TestJIT_Z80_Availability(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		return
	}
	if requireJIT && !z80JitAvailable {
		t.Fatalf("Z80 JIT unavailable on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
}
