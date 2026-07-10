// file_read_native.go - host file reads on native builds.
//
// hostReadFile is the single indirection every "load a file by path" site uses
// (the Machine loader default, the Program Executor, the media loader) so the
// js/wasm build can serve them from its in-memory disk volume instead of os.
// On native it is exactly os.ReadFile.

//go:build !wasm

package main

import "os"

func hostReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

// hostStatExists reports whether a path exists and whether it is a directory.
func hostStatExists(name string) (isDir bool, exists bool) {
	st, err := os.Stat(name)
	if err != nil {
		return false, false
	}
	return st.IsDir(), true
}
