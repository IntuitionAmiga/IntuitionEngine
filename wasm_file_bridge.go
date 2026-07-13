//go:build wasm

// wasm_file_bridge.go - browser bridge for adding a visitor's own file to the
// in-memory disk volume and reading a saved file back out for download.
//
// Everything stays inside the tab. ieImportFile copies bytes from a File the
// visitor picked into the same in-memory volume BASIC's LOAD/BLOAD/DIR read
// (memFiles, in the page's wasm memory). ieExportFile copies a file's bytes back
// out to JS so the page can offer it as a Blob download. Neither touches the
// network: the server only ever serves the static demo, and no upload endpoint
// exists. Imported files live for the session and vanish on reload.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import "syscall/js"

// Persistent js.Func values, kept for the life of the page (never released, so
// the callbacks stay valid on window).
// maxImportFileBytes bounds a single imported file so a runaway selection cannot
// allocate unbounded guest memory through the bridge, independent of any
// UI-level check. Generous for a BASIC programme and its assets; the whole guest
// RAM is 256 MiB.
const maxImportFileBytes = 64 << 20 // 64 MiB

var (
	importFileFunc  js.Func
	exportFileFunc  js.Func
	deleteFileFunc  js.Func
	fileBridgeReady bool
)

// registerWasmFileBridge exposes ieImportFile and ieExportFile on the global
// object (window in the browser) for the given in-memory volume. Idempotent: a
// second call is a no-op once the bridge is up.
func registerWasmFileBridge(dev *FileIODevice) {
	if dev == nil || fileBridgeReady {
		return
	}
	global := js.Global()

	// ieImportFile(name string, bytes Uint8Array) -> bool. Writes the bytes into
	// the in-memory volume under name; a following LOAD "name" (or BLOAD) reads
	// them back. The volume's base-name resolver means a demo's loose asset files
	// resolve even when the programme names them by their original nested path.
	importFileFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 2 || args[1].IsNull() || args[1].IsUndefined() {
			return false
		}
		name := args[0].String()
		if name == "" {
			return false
		}
		src := args[1]
		n := src.Get("length").Int()
		if n < 0 || n > maxImportFileBytes {
			return false
		}
		data := make([]byte, n)
		js.CopyBytesToGo(data, src)
		dev.SetMemFile(name, data)
		return true
	})
	global.Set("ieImportFile", importFileFunc)

	// ieExportFile(name string) -> Uint8Array | null. Returns the file's bytes
	// from the volume, or null when there is no such file.
	exportFileFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return js.Null()
		}
		data, ok := dev.readMemFile(args[0].String())
		if !ok {
			return js.Null()
		}
		out := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(out, data)
		return out
	})
	global.Set("ieExportFile", exportFileFunc)

	// ieDeleteFile(name string) -> bool. Removes the file the name resolves to,
	// used to clear an entry before a re-save so a poll cannot read stale bytes.
	deleteFileFunc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return false
		}
		return dev.DeleteMemFile(args[0].String())
	})
	global.Set("ieDeleteFile", deleteFileFunc)

	fileBridgeReady = true
}
