// file_io_select_native.go - runtime FileIO selection for native builds.
//
// Native targets have a real host filesystem, so BASIC's disk volume is a host
// directory. The js/wasm build has none and uses an in-memory volume seeded
// from the server's assets folder (file_io_select_wasm.go).

//go:build !wasm

package main

func newRuntimeFileIODevice(bus *MachineBus, baseDir string) *FileIODevice {
	return NewFileIODevice(bus, baseDir)
}

// seedRuntimeFileIOAssets is a no-op on native: the disk volume is the host
// filesystem, nothing to preload.
func seedRuntimeFileIOAssets(_ *FileIODevice) {}
