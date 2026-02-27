//go:build !linux

package main

// getSystemRAM returns 0 on non-Linux platforms.
// The caller handles this by using fallback memory limits.
func getSystemRAM() int64 {
	return 0
}
