//go:build js && wasm

package main

import "testing"

// The opcode contract must be the same compiled object used by the browser
// runtime, not a second inventory reconstructed in a js-only test.
func TestWasmJIT_Z80CanonicalManifestIsShared(t *testing.T) {
	rows := z80JITOpcodeManifest()
	if got, want := len(rows), 7*256-10; got != want {
		t.Fatalf("manifest rows = %d, want %d", got, want)
	}
	for _, row := range rows {
		if row.WasmOutcome == z80JITOutcomeUnclassified || row.WasmProof == "" {
			t.Fatalf("%s has no proved wasm outcome", row.Name)
		}
	}
}
