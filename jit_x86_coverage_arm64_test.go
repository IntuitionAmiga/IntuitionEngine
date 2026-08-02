//go:build arm64 && linux

package main

import "testing"

// TestX86ARM64CoverageManifest proves every advertised ARM64 path reaches the
// production emitter. Rows without native lowering are exercised through the
// production dispatcher's interpreter-resume boundary.
func TestX86ARM64CoverageManifest(t *testing.T) {
	mem := make([]byte, 0x2000)
	em, err := AllocExecMem(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(em.Free)
	for _, row := range x86JITCoverageManifest {
		if row.arm64 == x86JITCoverageUnavailable {
			continue
		}
		copy(mem[0x100:], row.sample)
		length := x86InstrLength(mem, 0x100)
		ji := x86DecodeInstr(mem, 0x100, uint16(length))
		if _, err := x86CompileBlock([]X86JITInstr{ji}, 0x100, em, mem); err != nil {
			t.Fatalf("%s (% X): advertised %s ARM64 path did not compile: %v", row.form, row.sample, row.arm64, err)
		}
		em.Reset()
	}
}
