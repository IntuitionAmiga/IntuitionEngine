//go:build js

// profile_cpu_signal_js.go - the browser build has no process signals.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

// installCPUProfileSignalStop is a no-op in the browser: there is no signal
// delivery and no terminal to interrupt from.
func installCPUProfileSignalStop() {}
