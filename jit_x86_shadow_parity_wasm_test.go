//go:build js && wasm

package main

import (
	"embed"
	"io/fs"
	"testing"
)

//go:embed sdk/examples/prebuilt/rotozoomer_x86.ie86 sdk/examples/prebuilt/antic_plasma_x86.ie86 intuitionengine.com/assets/Demos/x86/iedoom.ie86
var x86ShadowParityWorkloads embed.FS

func x86ShadowEmbeddedWorkload(t *testing.T, path string, largeBus bool) {
	t.Helper()
	rom, err := fs.ReadFile(x86ShadowParityWorkloads, path)
	if err != nil {
		t.Skipf("embedded rom missing (%s): %v", path, err)
	}
	x86ShadowParityCheckpointsBytes(t, path, rom, largeBus)
}

func TestX86WasmJIT_ShadowParity_Rotozoomer(t *testing.T) {
	x86ShadowEmbeddedWorkload(t, "sdk/examples/prebuilt/rotozoomer_x86.ie86", false)
}

func TestX86WasmJIT_ShadowParity_AnticPlasma(t *testing.T) {
	x86ShadowEmbeddedWorkload(t, "sdk/examples/prebuilt/antic_plasma_x86.ie86", false)
}

func TestX86WasmJIT_ShadowParity_IEDoomLinkedImage(t *testing.T) {
	x86ShadowEmbeddedWorkload(t, "intuitionengine.com/assets/Demos/x86/iedoom.ie86", true)
}
