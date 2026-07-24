//go:build js

// perf_report_exit_js.go - no terminal signals in the browser build.

package main

// installPerfReportExit is a no-op on js/wasm: there is no terminal to interrupt.
func installPerfReportExit() {}
