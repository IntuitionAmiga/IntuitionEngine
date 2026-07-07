//go:build !race

package main

// raceEnabled is false in non-race builds. See race_flag_on.go.
const raceEnabled = false
