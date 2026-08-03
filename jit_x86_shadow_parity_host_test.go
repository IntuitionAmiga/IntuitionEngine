//go:build !wasm

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func x86ShadowParityHostWorkload(t *testing.T, path string, largeBus bool) {
	t.Helper()
	rom, err := hostReadFile(path)
	if err != nil {
		t.Skipf("rom not present (%s): %v", path, err)
	}
	x86ShadowParityCheckpointsBytes(t, filepath.Base(path), rom, largeBus)
}

func x86ShadowParityHostMaybeWorkload(t *testing.T, path string, largeBus bool) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("rom not present (%s): %v", path, err)
	}
	x86ShadowParityHostWorkload(t, path, largeBus)
}
