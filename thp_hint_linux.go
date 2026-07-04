//go:build linux

package main

import "golang.org/x/sys/unix"

func adviseHugePages(mem []byte) {
	if len(mem) == 0 {
		return
	}
	_ = unix.Madvise(mem, unix.MADV_HUGEPAGE)
}
