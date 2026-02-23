//go:build embed_emutos

package main

import _ "embed"

func init() {
	compiledFeatures = append(compiledFeatures, "emutos:embedded")
}

//go:embed sdk/examples/prebuilt/etos256us.img
var embeddedEmuTOSImage []byte
