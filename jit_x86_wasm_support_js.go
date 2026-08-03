//go:build js && wasm

package main

import "os"

var (
	x86RegionPromotionEnabled = x86RegionPromotionDefaultEnabled()
	x86BlockChainingEnabled   = os.Getenv("X86_JIT_CHAINS") != "0"
)

func x86RegionPromotionDefaultEnabled() bool {
	return os.Getenv("X86_JIT_REGIONS") == "1"
}
