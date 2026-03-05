//go:build !embed_aros

package main

func init() {
	compiledFeatures = append(compiledFeatures, "aros:external")
}

var embeddedAROSImage []byte
