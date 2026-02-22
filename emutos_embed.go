//go:build embed_emutos

package main

import _ "embed"

func init() {
	compiledFeatures = append(compiledFeatures, "emutos:embedded")
}

//go:embed etos256us.img
var embeddedEmuTOSImage []byte
