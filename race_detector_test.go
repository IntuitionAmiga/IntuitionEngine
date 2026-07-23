//go:build !race

// race_detector_test.go - race detector presence, for tests with timing budgets.
//
// (c) 2024 - 2026 Zayn Otley. GPLv3 or later.

package main

// raceDetectorEnabled reports whether the binary was built with -race, which
// inflates lock and callback costs enough to invalidate timing budgets.
const raceDetectorEnabled = false
