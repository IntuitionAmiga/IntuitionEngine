//go:build linux

package main

import (
	"fmt"
	"os"
)

func detectUsableRAM() (uint64, string, error) {
	text, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, "", fmt.Errorf("read /proc/meminfo: %w", err)
	}
	return ParseMeminfo(string(text))
}
