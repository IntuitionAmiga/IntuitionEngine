//go:build !linux && !darwin && !windows

package main

import "fmt"

func detectUsableRAM() (uint64, string, error) {
	return 0, "", fmt.Errorf("%w: usable RAM discovery unavailable on this platform", ErrUnsupportedPlatform)
}
