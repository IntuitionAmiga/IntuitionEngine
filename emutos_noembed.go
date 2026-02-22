//go:build !embed_emutos

package main

func init() {
	compiledFeatures = append(compiledFeatures, "emutos:external")
}

var embeddedEmuTOSImage []byte
