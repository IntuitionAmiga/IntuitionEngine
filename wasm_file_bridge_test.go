//go:build wasm

// wasm_file_bridge_test.go - node-executed contract test for the browser file
// bridge. Runs under `make test-wasm-node` (GOOS=js GOARCH=wasm) because it
// drives the real syscall/js callbacks the demo page calls.

package main

import (
	"bytes"
	"syscall/js"
	"testing"
)

// TestWasmFileBridge_ImportExportRoundTrip proves the visitor-side loop: bytes
// handed to ieImportFile land in the in-memory volume where LOAD reads them, and
// ieExportFile hands the same bytes back for a Blob download. A missing file
// exports as null.
func TestWasmFileBridge_ImportExportRoundTrip(t *testing.T) {
	bus := NewMachineBus()
	dev := NewMemoryFileIODevice(bus)
	wasmFileVolume = dev
	fileBridgeReady = false
	registerWasmFileBridge(dev)

	payload := []byte("10 PRINT \"HELLO\"\x00\xff\x01 demo bytes")
	src := js.Global().Get("Uint8Array").New(len(payload))
	js.CopyBytesToJS(src, payload)

	imp := js.Global().Get("ieImportFile")
	if imp.Type() != js.TypeFunction {
		t.Fatal("ieImportFile not registered on the global object")
	}
	if ok := imp.Invoke("mydemo.bas", src).Bool(); !ok {
		t.Fatal("ieImportFile returned false for a valid file")
	}

	// The volume now serves the imported file to LOAD.
	got, ok := dev.readMemFile("mydemo.bas")
	if !ok || !bytes.Equal(got, payload) {
		t.Fatalf("volume after import = %v ok=%v, want %v", got, ok, payload)
	}

	// Export it back out and compare.
	out := js.Global().Get("ieExportFile").Invoke("mydemo.bas")
	if out.IsNull() {
		t.Fatal("ieExportFile returned null for an imported file")
	}
	back := make([]byte, out.Get("length").Int())
	js.CopyBytesToGo(back, out)
	if !bytes.Equal(back, payload) {
		t.Fatalf("export round-trip = %v, want %v", back, payload)
	}

	// A file that was never imported exports as null.
	if miss := js.Global().Get("ieExportFile").Invoke("nope.bas"); !miss.IsNull() {
		t.Fatalf("export of missing file = type %v, want null", miss.Type())
	}

	// ieDeleteFile removes the entry so a subsequent export sees nothing, which
	// is how the save flow avoids downloading pre-save bytes on an overwrite.
	if del := js.Global().Get("ieDeleteFile").Invoke("mydemo.bas").Bool(); !del {
		t.Fatal("ieDeleteFile returned false for an existing file")
	}
	if after := js.Global().Get("ieExportFile").Invoke("mydemo.bas"); !after.IsNull() {
		t.Fatal("export after delete should be null")
	}
	if del := js.Global().Get("ieDeleteFile").Invoke("mydemo.bas").Bool(); del {
		t.Fatal("ieDeleteFile of an already-removed file should return false")
	}

	// An import call with no byte argument is rejected, not a panic.
	if ok := imp.Invoke("noargs.bas").Bool(); ok {
		t.Fatal("ieImportFile with a missing byte argument should return false")
	}

	// A file over the per-file cap is rejected by the bridge and never stored,
	// independent of any UI check.
	big := js.Global().Get("Uint8Array").New(maxImportFileBytes + 1)
	if ok := imp.Invoke("big.bin", big).Bool(); ok {
		t.Fatal("import of an oversize file should be rejected")
	}
	if _, ok := dev.readMemFile("big.bin"); ok {
		t.Fatal("oversize file must not be stored")
	}

	// A file exactly at the cap is accepted.
	atCap := js.Global().Get("Uint8Array").New(maxImportFileBytes)
	if ok := imp.Invoke("atcap.bin", atCap).Bool(); !ok {
		t.Fatal("import of a file at the cap should be accepted")
	}
}
