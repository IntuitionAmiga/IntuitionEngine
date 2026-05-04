//go:build aix || android || darwin || dragonfly || freebsd || hurd || illumos || ios || linux || netbsd || openbsd || solaris

package main

import "syscall"

func makeTestFIFO(path string) error {
	return syscall.Mkfifo(path, 0o600)
}
