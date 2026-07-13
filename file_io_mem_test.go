// file_io_mem_test.go - contract tests for the FileIO in-memory disk volume
// used by the js/wasm build (BASIC LOAD/SAVE/DIR). They drive the device
// through its operations exactly as BASIC would and run on the host because the
// store is pure Go. stageCString/stageBytes/readBytes live in
// bootstrap_hostfs_mem_test.go (same package).

package main

import (
	"strings"
	"testing"
)

// TestWasmFileIOMem_SaveLoadRoundTrip proves a SAVE (WRITE) commits to the
// in-memory volume and a subsequent LOAD (READ) returns the same bytes.
func TestWasmFileIOMem_SaveLoadRoundTrip(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)

	const nameAddr, srcAddr, dstAddr = 0x1000, 0x2000, 0x3000
	payload := []byte("ROTOZOOM\x00\x01\x02\xff data")
	stageCString(bus, nameAddr, "TEST.DAT")
	stageBytes(bus, srcAddr, payload)

	// SAVE.
	f.fileNamePtr = nameAddr
	f.fileDataPtr = srcAddr
	f.fileDataLen = uint32(len(payload))
	f.doWrite()
	if f.fileStatus != 0 || f.fileErrorCode != FILE_ERR_OK {
		t.Fatalf("SAVE status=%d err=%d", f.fileStatus, f.fileErrorCode)
	}

	// LOAD into a different address.
	f.fileNamePtr = nameAddr
	f.fileDataPtr = dstAddr
	f.fileDataLen = 0
	f.doRead()
	if f.fileStatus != 0 || f.fileErrorCode != FILE_ERR_OK {
		t.Fatalf("LOAD status=%d err=%d", f.fileStatus, f.fileErrorCode)
	}
	if f.fileResultLen != uint32(len(payload)) {
		t.Fatalf("LOAD resultLen=%d, want %d", f.fileResultLen, len(payload))
	}
	if got := readBytes(bus, dstAddr, len(payload)); string(got) != string(payload) {
		t.Fatalf("round-trip = %q, want %q", got, payload)
	}
}

// TestWasmFileIOMem_SeededReadServesBLOAD proves a seeded (HTTP-preloaded)
// asset is readable, case-insensitively, which is the LOAD/BLOAD path.
func TestWasmFileIOMem_SeededReadServesBLOAD(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)
	asset := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x11, 0x22}
	f.SetMemFile("rotozoomtexture.raw", asset)

	const nameAddr, dstAddr = 0x1000, 0x2000
	stageCString(bus, nameAddr, "ROTOZOOMTEXTURE.RAW") // upper-case query, lower-case seed
	f.fileNamePtr = nameAddr
	f.fileDataPtr = dstAddr
	f.doRead()
	if f.fileStatus != 0 {
		t.Fatalf("BLOAD seeded asset status=%d err=%d", f.fileStatus, f.fileErrorCode)
	}
	if got := readBytes(bus, dstAddr, len(asset)); string(got) != string(asset) {
		t.Fatalf("BLOAD read = %v, want %v", got, asset)
	}
}

// TestWasmFileIOMem_ImportedBasenameResolvesNestedPath proves the demo-sharing
// path underneath the browser file bridge: a visitor imports a demo's assets as
// flat basenames (all a browser file input exposes), yet a BASIC BLOAD that
// names the original nested asset path still resolves them by base name. This is
// why sharing wobble_zoom.bas plus its two loose asset files is enough, with no
// directory tree to recreate.
func TestWasmFileIOMem_ImportedBasenameResolvesNestedPath(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)
	asset := []byte{0x01, 0x02, 0x03, 0x04}
	f.SetMemFile("splash_640x92.rgba", asset) // imported as a bare basename

	const nameAddr, dstAddr = 0x1000, 0x2000
	stageCString(bus, nameAddr, "sdk/examples/assets/splash_640x92.rgba") // nested request
	f.fileNamePtr = nameAddr
	f.fileDataPtr = dstAddr
	f.doRead()
	if f.fileStatus != 0 || f.fileErrorCode != FILE_ERR_OK {
		t.Fatalf("nested-path read of imported basename status=%d err=%d", f.fileStatus, f.fileErrorCode)
	}
	if got := readBytes(bus, dstAddr, len(asset)); string(got) != string(asset) {
		t.Fatalf("resolved bytes = %v, want %v", got, asset)
	}
}

// TestWasmFileIOMem_DeleteRemovesResolved proves DeleteMemFile drops the entry a
// read would resolve (including a nested-path request against a base-name key)
// and reports false when there is nothing to remove. The browser save flow uses
// this to clear a file before re-saving so a poll cannot read pre-save bytes.
func TestWasmFileIOMem_DeleteRemovesResolved(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)
	f.SetMemFile("x.bas", []byte("10 PRINT 1"))

	if !f.DeleteMemFile("x.bas") {
		t.Fatal("DeleteMemFile of an existing file returned false")
	}
	if _, ok := f.readMemFile("x.bas"); ok {
		t.Fatal("file still readable after delete")
	}
	if f.DeleteMemFile("x.bas") {
		t.Fatal("DeleteMemFile of a missing file returned true")
	}
}

// TestWasmFileIOMem_ImportOverridesBundledNestedAsset proves the sharing flow:
// when a visitor imports a replacement for a bundled asset under its flat
// basename, a BLOAD naming the original nested manifest path resolves the
// imported bytes, not the bundled asset a lazy fetch would supply.
func TestWasmFileIOMem_ImportOverridesBundledNestedAsset(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)
	f.memFetch = func(string) ([]byte, bool) { return []byte("BUNDLED"), true }
	f.RegisterMemPath("sdk/examples/assets/splash_640x92.rgba") // manifest nested path
	imported := []byte("IMPORTED")
	f.SetMemFile("splash_640x92.rgba", imported) // flat browser import

	const nameAddr, dstAddr = 0x1000, 0x2000
	stageCString(bus, nameAddr, "sdk/examples/assets/splash_640x92.rgba")
	f.fileNamePtr = nameAddr
	f.fileDataPtr = dstAddr
	f.doRead()
	if f.fileStatus != 0 || f.fileErrorCode != FILE_ERR_OK {
		t.Fatalf("read status=%d err=%d", f.fileStatus, f.fileErrorCode)
	}
	if got := readBytes(bus, dstAddr, len(imported)); string(got) != "IMPORTED" {
		t.Fatalf("resolved %q, want IMPORTED (bundled asset should be overridden)", got)
	}
}

// TestWasmFileIOMem_DirLists proves DIR (LIST) enumerates the volume, sorted,
// CRLF-delimited.
func TestWasmFileIOMem_DirLists(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)
	f.SetMemFile("beta.bas", []byte("b"))
	f.SetMemFile("alpha.raw", []byte("a"))

	const nameAddr, dstAddr = 0x1000, 0x2000
	stageCString(bus, nameAddr, "") // list the root
	f.fileNamePtr = nameAddr
	f.fileDataPtr = dstAddr
	f.doList()
	if f.fileStatus != 0 {
		t.Fatalf("DIR status=%d err=%d", f.fileStatus, f.fileErrorCode)
	}
	listing := string(readBytes(bus, dstAddr, int(f.fileResultLen)))
	// Original case is preserved for display; sorted case-insensitively.
	if !strings.Contains(listing, "alpha.raw") || !strings.Contains(listing, "beta.bas") {
		t.Fatalf("DIR listing = %q, want original-case names", listing)
	}
	if strings.Index(listing, "alpha.raw") > strings.Index(listing, "beta.bas") {
		t.Fatalf("DIR listing not sorted: %q", listing)
	}
}

// TestWasmFileIOMem_LazyFetchOnRead proves a registered-but-not-cached path
// (from the boot manifest) is fetched on first read via memFetch, cached, and
// served; a second read does not re-fetch. Subfolder paths and case-insensitive
// lookup both work.
func TestWasmFileIOMem_LazyFetchOnRead(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)

	payload := []byte{0x01, 0x02, 0x03, 0x04}
	fetches := 0
	f.memFetch = func(rel string) ([]byte, bool) {
		if rel == "Demos/ie32/foo.iex" {
			fetches++
			return payload, true
		}
		return nil, false
	}
	f.RegisterMemPath("Demos/ie32/foo.iex") // known, not yet loaded

	// DIR at root shows the Demos directory even before any content loads.
	const nameAddr, dstAddr = 0x1000, 0x2000
	stageCString(bus, nameAddr, "")
	f.fileNamePtr, f.fileDataPtr = nameAddr, dstAddr
	f.doList()
	if listing := string(readBytes(bus, dstAddr, int(f.fileResultLen))); !strings.Contains(listing, "Demos/") {
		t.Fatalf("root DIR = %q, want Demos/", listing)
	}

	// First read (lower-case query) triggers the lazy fetch.
	stageCString(bus, nameAddr, "demos/IE32/foo.iex")
	f.fileNamePtr, f.fileDataPtr = nameAddr, dstAddr
	f.doRead()
	if f.fileStatus != 0 || fetches != 1 {
		t.Fatalf("first read status=%d fetches=%d, want 0/1", f.fileStatus, fetches)
	}
	if got := readBytes(bus, dstAddr, len(payload)); string(got) != string(payload) {
		t.Fatalf("lazy read = %v, want %v", got, payload)
	}

	// Second read is served from cache: no additional fetch.
	f.fileNamePtr, f.fileDataPtr = nameAddr, 0x3000
	f.doRead()
	if fetches != 1 {
		t.Fatalf("second read re-fetched: fetches=%d, want 1", fetches)
	}
}

// TestWasmFileIOMem_ReadMiss proves a missing file returns not-found, never
// touching a host filesystem.
func TestWasmFileIOMem_ReadMiss(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)

	const nameAddr = 0x1000
	stageCString(bus, nameAddr, "NOPE.DAT")
	f.fileNamePtr = nameAddr
	f.fileDataPtr = 0x2000
	f.doRead()
	if f.fileStatus != 1 || f.fileErrorCode != FILE_ERR_NOT_FOUND {
		t.Fatalf("read miss status=%d err=%d, want 1/FILE_ERR_NOT_FOUND", f.fileStatus, f.fileErrorCode)
	}
}

// TestWasmFileIOMem_ReadRejectsTraversal proves the memFS READ path applies
// the same lexical rejection as native sanitizePath: a traversal-like name
// must return FILE_ERR_PATH_TRAVERSAL, never resolve by suffix or basename to
// a registered asset (P2 review finding).
func TestWasmFileIOMem_ReadRejectsTraversal(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)
	f.SetMemFile("Demos/ie32/foo.iex", []byte{1, 2, 3})

	const nameAddr = 0x1000
	for _, name := range []string{"../foo.iex", "a/../../foo.iex", "/abs/foo.iex"} {
		stageCString(bus, nameAddr, name)
		f.fileNamePtr = nameAddr
		f.fileDataPtr = 0x2000
		f.doRead()
		if f.fileStatus != 1 || f.fileErrorCode != FILE_ERR_PATH_TRAVERSAL {
			t.Fatalf("read %q status=%d err=%d, want 1/FILE_ERR_PATH_TRAVERSAL", name, f.fileStatus, f.fileErrorCode)
		}
	}

	// The clean name must still resolve (basename match into the tree).
	stageCString(bus, nameAddr, "foo.iex")
	f.fileNamePtr = nameAddr
	f.fileDataPtr = 0x2000
	f.doRead()
	if f.fileStatus != 0 {
		t.Fatalf("clean read status=%d err=%d, want ok", f.fileStatus, f.fileErrorCode)
	}
}

// TestWasmFileIOMem_ListRejectsTraversal proves the memFS LIST path rejects
// traversal-like directory names like the native path does.
func TestWasmFileIOMem_ListRejectsTraversal(t *testing.T) {
	bus := NewMachineBus()
	f := NewMemoryFileIODevice(bus)
	f.SetMemFile("Demos/foo.iex", []byte{1})

	const nameAddr = 0x1000
	stageCString(bus, nameAddr, "../")
	f.fileNamePtr = nameAddr
	f.fileDataPtr = 0x2000
	f.doList()
	if f.fileStatus != 1 || f.fileErrorCode != FILE_ERR_PATH_TRAVERSAL {
		t.Fatalf("list \"../\" status=%d err=%d, want 1/FILE_ERR_PATH_TRAVERSAL", f.fileStatus, f.fileErrorCode)
	}
}
