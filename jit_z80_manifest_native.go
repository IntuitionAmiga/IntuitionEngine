//go:build (amd64 || arm64) && linux

package main

import "runtime"

func z80ManifestCurrentOutcome(row z80JITOpcodeManifestRow) z80JITOpcodeOutcome {
	if runtime.GOARCH == "arm64" {
		return row.ARM64Outcome
	}
	return row.AMD64Outcome
}
