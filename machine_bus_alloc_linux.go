// machine_bus_alloc_linux.go - madvise discard flag for Linux.
//
// Linux MADV_DONTNEED on an anonymous private mapping releases the pages
// back to the kernel and guarantees subsequent reads return zero (the
// pages are demand-faulted from the zero page). This is the correct
// flag for a bus.memory reset that wants both correctness (zero-on-read)
// and immediate RSS reclaim.

//go:build linux

package main

import "golang.org/x/sys/unix"

const busMemMadviseDiscardFlag = unix.MADV_DONTNEED
