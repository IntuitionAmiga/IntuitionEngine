// media_path_wasm.go - media loader path handling for the js/wasm build.
//
// The browser has no host filesystem and no working directory (os.Getwd fails
// under wasm_exec.js, so filepath.Abs errors and the native sanitiser would
// reject every path). Containment is not a concern here: hostReadFile resolves
// the name against the in-memory disk volume, which only ever holds the
// HTTP-seeded assets. Lexical rejection of empty and parent-escaping names is
// kept so the error surface matches native.

//go:build wasm

package main

import "strings"

func sanitizeMediaHostPath(_, path string) (string, bool) {
	if path == "" || strings.Contains(path, "..") {
		return "", false
	}
	return strings.ReplaceAll(path, "\\", "/"), true
}
