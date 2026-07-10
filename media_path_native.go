// media_path_native.go - media loader path confinement for hosts with a real
// filesystem. The guest supplies an arbitrary file name over MMIO, so the
// resolved path must stay inside the media base directory, following symlinks.

//go:build !wasm

package main

import (
	"path/filepath"
	"strings"
)

func sanitizeMediaHostPath(baseDir, path string) (string, bool) {
	if path == "" || strings.Contains(path, "..") {
		return "", false
	}
	root, err := filepath.EvalSymlinks(baseDir)
	if err != nil {
		root = filepath.Clean(baseDir)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", false
	}

	var fullPath string
	if filepath.IsAbs(path) {
		fullPath = filepath.Clean(path)
	} else {
		fullPath = filepath.Join(root, path)
	}

	target := fullPath
	if resolved, err := filepath.EvalSymlinks(fullPath); err == nil {
		target = resolved
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return fullPath, true
}
