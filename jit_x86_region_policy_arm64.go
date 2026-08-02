// jit_x86_region_policy_arm64.go - Linux/arm64 x86 JIT tier policy.
//
//go:build arm64 && linux

package main

import "os"

// Keep the ARM64 native-chain switch identical to amd64.  Region promotion is
// opt-in while the backend matures, so normal ARM64 execution remains a
// bounded collection of basic blocks unless explicitly requested.
var (
	x86RegionPromotionEnabled = os.Getenv("X86_JIT_REGIONS") == "1"
	x86BlockChainingEnabled   = os.Getenv("X86_JIT_CHAINS") != "0"
)
