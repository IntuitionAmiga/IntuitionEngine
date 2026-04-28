// machine_bus_alloc_darwin.go - madvise discard flag for darwin.
//
// On darwin MADV_DONTNEED merely deactivates pages without releasing
// RSS. MADV_FREE marks the pages reclaimable and the kernel zeroes them
// on next access, matching the Linux MADV_DONTNEED contract closely
// enough for bus.memory reset semantics.

//go:build darwin

package main

import "golang.org/x/sys/unix"

const busMemMadviseDiscardFlag = unix.MADV_FREE
