//go:build arm64 && linux

package main

import (
	"path/filepath"
	"testing"
)

func TestX86ARM64_ShadowParity_Rotozoomer(t *testing.T) {
	x86ShadowParityHostWorkload(t, filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_x86.ie86"), false)
}

func TestX86ARM64_ShadowParity_AnticPlasma(t *testing.T) {
	x86ShadowParityHostWorkload(t, filepath.Join("sdk", "examples", "prebuilt", "antic_plasma_x86.ie86"), false)
}

func TestX86ARM64_ShadowParity_IEDoomLinkedImage(t *testing.T) {
	x86ShadowParityHostMaybeWorkload(t, filepath.Join("..", "chocolate-doom", "build", "iedoom.ie86"), true)
}
