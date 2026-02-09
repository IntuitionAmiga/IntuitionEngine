//go:build embed_basic

package main

import _ "embed"

//go:embed assembler/ehbasic_ie64.ie64
var embeddedBasicImage []byte
