//go:build linux

package main

import "syscall"

const arosHostSocketCreateFlagMask = syscall.SOCK_CLOEXEC | syscall.SOCK_NONBLOCK
