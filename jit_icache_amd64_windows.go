//go:build amd64 && windows

package main

func flushICache(addr, size uintptr) {}

func flushICacheDual(writableAddr, execAddr, size uintptr) {}
