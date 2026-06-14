//go:build darwin

package main

import "syscall"

func arosHostSocketSelect(nfd int, r, w, e *syscall.FdSet, timeout *arosHostSocketTimeval) (int, error) {
	var tv *syscall.Timeval
	if timeout != nil {
		v := syscall.NsecToTimeval((int64(timeout.Sec) * 1000000000) + (int64(timeout.Usec) * 1000))
		tv = &v
	}
	if err := syscall.Select(nfd, r, w, e, tv); err != nil {
		return -1, err
	}
	return fdSetReadyCount(nfd, r, w, e), nil
}

func fdSetReadyCount(nfd int, sets ...*syscall.FdSet) int {
	ready := 0
	for _, set := range sets {
		if set == nil {
			continue
		}
		for fd := 0; fd < nfd; fd++ {
			if fdIsSet(set, fd) {
				ready++
			}
		}
	}
	return ready
}
