//go:build embed_ab3d2

package main

import _ "embed"

func init() {
	compiledFeatures = append(compiledFeatures, "ab3d2:embedded")
}

//go:embed embedded/ab3d2/ab3d2_ie68_redux_high.ie68
var embeddedAB3D2Image []byte

//go:embed embedded/ab3d2/_build.zip
var embeddedAB3D2AssetZip []byte
