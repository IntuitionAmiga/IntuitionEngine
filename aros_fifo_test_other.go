//go:build !(aix || android || darwin || dragonfly || freebsd || hurd || illumos || ios || linux || netbsd || openbsd || solaris)

package main

import "errors"

var errTestFIFONotSupported = errors.New("FIFO tests are not supported on this platform")

func makeTestFIFO(path string) error {
	return errTestFIFONotSupported
}
