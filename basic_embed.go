//go:build embed_basic

package main

import _ "embed"

func init() {
	compiledFeatures = append(compiledFeatures, "basic:embedded")
}

//go:embed sdk/examples/asm/ehbasic_ie64.ie64
var embeddedBasicImage []byte
