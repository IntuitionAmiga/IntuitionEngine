//go:build !amd64 || !(linux || windows || darwin)

package main

func z80CompileTurboNative(tb *z80TurboBlock, execMem *ExecMem) (*JITBlock, bool) {
	return nil, false
}
