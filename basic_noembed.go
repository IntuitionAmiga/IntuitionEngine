//go:build !embed_basic

package main

func init() {
	compiledFeatures = append(compiledFeatures, "basic:external")
}

var embeddedBasicImage []byte // nil when not embedded
