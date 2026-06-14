//go:build linux

package main

import "syscall"

func arosHostSocketSelect(nfd int, r, w, e *syscall.FdSet, timeout *arosHostSocketTimeval) (int, error) {
	var tv *syscall.Timeval
	if timeout != nil {
		v := syscall.NsecToTimeval((int64(timeout.Sec) * 1000000000) + (int64(timeout.Usec) * 1000))
		tv = &v
	}
	return syscall.Select(nfd, r, w, e, tv)
}
