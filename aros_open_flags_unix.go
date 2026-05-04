//go:build aix || android || darwin || dragonfly || freebsd || hurd || illumos || ios || linux || netbsd || openbsd || solaris

package main

import "syscall"

const arosOpenNoFollow = syscall.O_NOFOLLOW
