package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// These are the reviewed prebuilt parity fixtures. Changing one requires an
// intentional fixture-update review, rather than silently changing a runtime
// JIT acceptance input.
var p65ParityFixtureSHA256 = map[string]string{
	"sdk/examples/prebuilt/rotozoomer_65.ie65":     "1eb0a6d6369d1f7ce95844cd2314a07fe5a26734b97d558622f42fc7b3d7f731",
	"sdk/examples/assets/rotozoomtexture_6502.raw": "fbbfe207e4f342326fa44ba9ab9b30307c58d0fc7bb80e7831d4aeb70fe07ba0",
	"sdk/examples/prebuilt/robocop_intro_65.ie65":  "048e6010e24696cf866e6f5c1af20a04bc08b84ef66e08798807ee0ee65db0a7",
	"sdk/examples/assets/robocop_rgba.bin":         "58642cc3c58786077f0e55ea6bce55d3270578b41f82863c62cca973afb5596f",
	"sdk/examples/assets/robocop_mask.bin":         "e7a6e8cf96d9fbf91331c74e9bff479541ec2b3e6cb80fcd91878c0cad3b0b31",
	"sdk/examples/assets/music/Robocop1.ay":        "e3944edd7de175debf950663f2949381413cfc100e32afc5441932455bb9c03c",
	"sdk/examples/assets/font_rgba.bin":            "bc6c0fe3fb6005fac55ec543ddeefc35fa63d4fca322727090d49f3710635744",
}

func TestP65JITParityFixturesSHA256(t *testing.T) {
	for path, want := range p65ParityFixtureSHA256 {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read fixture %s: %v", path, err)
		}
		if got := sha256.Sum256(data); hex.EncodeToString(got[:]) != want {
			t.Errorf("fixture %s SHA-256=%x, want %s", path, got, want)
		}
	}
}
