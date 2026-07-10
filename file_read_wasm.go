// file_read_wasm.go - host file reads for the js/wasm build.
//
// The browser has no host filesystem, so every "load a file by path" site reads
// from the in-memory disk volume (the FileIO device seeded from assets over
// HTTP). The launcher, Program Executor and media loader pass a filename that is
// present in that volume. os.ReadFile would fail on js, so hostReadFile resolves
// against the registered volume and errors cleanly if the file is absent.

//go:build wasm

package main

import (
	"io/fs"
)

// wasmFileVolume is the in-memory disk volume registered by the wasm FileIO
// selector at construction. hostReadFile reads from it.
var wasmFileVolume *FileIODevice

func hostReadFile(name string) ([]byte, error) {
	if data, ok := wasmFileVolume.lookupMem(name); ok {
		return data, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// hostStatExists reports existence against the in-memory volume (files only, no
// directories are executed), so the Program Executor's pre-check works without
// os.
func hostStatExists(name string) (isDir bool, exists bool) {
	return false, wasmFileVolume.memPathKnown(name)
}
