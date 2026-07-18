//go:build (amd64 || arm64) && (linux || windows || darwin)

// jit_ie64_opt_toggle_bench_test.go - Test-binary-only switch used to collect
// regression baselines: IE64_BENCH_DISABLE_OPTS=1 turns off constant folding,
// cold-exit outlining and loop hoisting so the existing benchmarks can be
// compared with and without the conservative optimisation slices under one
// binary. Not a product environment variable.
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
// Licensed under the GNU AGPL v3 or later.

package main

import "os"

func init() {
	if os.Getenv("IE64_BENCH_DISABLE_OPTS") == "1" {
		ie64ConstFoldDisabled = true
		ie64ColdExitOutlineDisabled = true
		ie64LoopHoistDisabled = true
	}
}
