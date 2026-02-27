//go:build linux

package main

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// getSystemRAM detects total system RAM in bytes.
// Returns 0 if detection fails (caller should use fallback).
func getSystemRAM() int64 {
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for line := range strings.SplitSeq(string(data), "\n") {
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
						return kb * 1024
					}
				}
			}
		}
	}

	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err == nil {
		return int64(info.Totalram) * int64(info.Unit)
	}

	return 0
}
