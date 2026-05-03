//go:build darwin

package main

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func detectUsableRAM() (uint64, string, error) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0, "", fmt.Errorf("read hw.memsize: %w", err)
	}
	// Darwin exposes total physical RAM portably, while "available" memory
	// depends on VM pressure accounting. Treat half of total RAM as usable so
	// the normal per-platform reserve is applied to a conservative base.
	return pageAlignDown(total / 2), "darwin-hw.memsize-conservative", nil
}
