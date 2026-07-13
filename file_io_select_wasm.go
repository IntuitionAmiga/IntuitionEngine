// file_io_select_wasm.go - runtime FileIO selection and asset preload for the
// js/wasm browser build.
//
// The browser has no host filesystem, so BASIC's disk volume is an in-memory
// volume seeded at boot from the document-root assets folder over HTTP: the
// MANIFEST registers every path, and file contents are fetched lazily (via the
// browser Fetch API through syscall/js, which parks the goroutine) on first
// read. To add content, drop files into intuitionengine.com/assets/ and re-run
// make web-demos (or make wasm) to regenerate the MANIFEST.

//go:build wasm

package main

import (
	"fmt"
	"strings"
	"syscall/js"
)

// assetBase is the document-root assets folder, relative to the /demo/ page.
const assetBase = "../assets/"

func newRuntimeFileIODevice(bus *MachineBus, _ string) *FileIODevice {
	dev := NewMemoryFileIODevice(bus)
	// Lazy fetch: load a file's bytes over HTTP on first LOAD/RUN. net/http on
	// js parks the goroutine and uses Fetch, so a cache-miss read blocks the CPU
	// briefly (the yield lets the event loop run) rather than stalling boot.
	dev.memFetch = func(relPath string) ([]byte, bool) {
		data, err := httpGetBytes(assetBase + relPath)
		if err != nil {
			fmt.Printf("wasm assets: fetch %s: %v\n", relPath, err)
			return nil, false
		}
		return data, true
	}
	// Register the volume so hostReadFile (used by the launcher, Program
	// Executor and media loader) reads from the same in-memory disk.
	wasmFileVolume = dev
	// Expose ieImportFile/ieExportFile so the demo page can add the visitor's
	// own file to this volume and read a saved file back out for download.
	registerWasmFileBridge(dev)
	return dev
}

// seedRuntimeFileIOAssets fetches only the asset manifest at boot and registers
// each path as a known file; contents load lazily on first read. A missing
// manifest leaves the volume empty (non-fatal).
func seedRuntimeFileIOAssets(f *FileIODevice) {
	body, err := httpGetBytes(assetBase + "MANIFEST")
	if err != nil {
		fmt.Printf("wasm assets: no %sMANIFEST (%v); BASIC disk volume is empty\n", assetBase, err)
		return
	}
	known := 0
	for _, line := range strings.Split(string(body), "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasPrefix(name, "#") || strings.EqualFold(name, "MANIFEST") {
			continue
		}
		if strings.EqualFold(filepathBase(name), "README.TXT") {
			continue
		}
		f.RegisterMemPath(name)
		known++
	}
	fmt.Printf("wasm assets: %d file(s) in the BASIC disk volume (loaded on demand)\n", known)
}

// filepathBase returns the last path segment (forward-slash paths).
func filepathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// httpGetBytes fetches a URL with the browser Fetch API via syscall/js,
// parking the calling goroutine until the promise settles. Using Fetch
// directly instead of net/http keeps the TLS/x509/HTTP stack (about 2.5 MB of
// code) out of the wasm binary; the browser handles the transport.
func httpGetBytes(url string) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)

	var onResp, onBuf, onErr js.Func
	release := func() {
		onResp.Release()
		onBuf.Release()
		onErr.Release()
	}
	onErr = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "fetch failed"
		if len(args) > 0 {
			msg = args[0].Call("toString").String()
		}
		done <- result{nil, fmt.Errorf("%s", msg)}
		return nil
	})
	onBuf = js.FuncOf(func(_ js.Value, args []js.Value) any {
		u8 := js.Global().Get("Uint8Array").New(args[0])
		data := make([]byte, u8.Get("length").Int())
		js.CopyBytesToGo(data, u8)
		done <- result{data, nil}
		return nil
	})
	onResp = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resp := args[0]
		if !resp.Get("ok").Bool() {
			done <- result{nil, fmt.Errorf("status %d", resp.Get("status").Int())}
			return nil
		}
		resp.Call("arrayBuffer").Call("then", onBuf).Call("catch", onErr)
		return nil
	})
	js.Global().Call("fetch", url).Call("then", onResp).Call("catch", onErr)
	r := <-done
	release()
	return r.data, r.err
}
