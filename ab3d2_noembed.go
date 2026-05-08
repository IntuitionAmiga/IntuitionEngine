//go:build !embed_ab3d2

package main

func init() {
	compiledFeatures = append(compiledFeatures, "ab3d2:external")
}

var embeddedAB3D2Image []byte
var embeddedAB3D2AssetZip []byte
