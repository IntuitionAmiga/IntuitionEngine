//go:build race

package main

// raceEnabled reports whether the binary was built with the race detector. The
// race build inhibits gc FMA fusion, so the scalar reference's raw float results
// shift by up to 1 ULP versus the release build (which the golden checksums
// tolerate). The strict bit-exact Voodoo differential targets the release build
// and skips under the race detector.
const raceEnabled = true
