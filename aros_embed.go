//go:build embed_aros

package main

import _ "embed"

func init() {
	compiledFeatures = append(compiledFeatures, "aros:embedded")
}

//go:embed sdk/examples/prebuilt/aros-ie.rom
var embeddedAROSImage []byte
