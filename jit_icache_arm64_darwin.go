//go:build arm64 && darwin

package main

func flushICache(addr, size uintptr) {
	if err := darwinICacheInvalidate(addr, size); err != nil {
		panic(err)
	}
}

func flushICacheDual(writableAddr, execAddr, size uintptr) {
	if err := darwinICacheInvalidate(execAddr, size); err != nil {
		panic(err)
	}
}
