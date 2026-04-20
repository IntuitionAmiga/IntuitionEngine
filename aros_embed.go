//go:build embed_aros

package main

import _ "embed"

func init() {
	compiledFeatures = append(compiledFeatures, "aros:embedded")
}

//go:embed sdk/roms/aros-ie-m68k.rom
var embeddedAROSImage []byte
