// jit_smc_range.go - range-scoped self-modifying-code invalidation controls.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import "os"

var jitSMCRangeDisabled = os.Getenv("IE_JIT_SMC_RANGE") == "0"
