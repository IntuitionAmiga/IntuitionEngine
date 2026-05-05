//go:build !((amd64 || arm64) && linux) && !(windows && (amd64 || arm64)) && !(darwin && (amd64 || arm64))

package main

import "fmt"

// ExecMem is a non-executable stub on platforms without the Linux dual-map backend.
type ExecMem struct {
	used int
}

func AllocExecMem(size int) (*ExecMem, error) {
	return nil, fmt.Errorf("executable JIT memory is not available on %s/%s", runtimeGOOS, runtimeGOARCH)
}

func (em *ExecMem) Write(code []byte) (uintptr, error) {
	if em == nil {
		return 0, fmt.Errorf("executable JIT memory is not allocated")
	}
	return 0, fmt.Errorf("executable JIT memory is not available on %s/%s", runtimeGOOS, runtimeGOARCH)
}

func (em *ExecMem) Reset() {
	if em != nil {
		em.used = 0
	}
}

func (em *ExecMem) Free() {}

func (em *ExecMem) Used() int {
	if em == nil {
		return 0
	}
	return em.used
}

func lookupExecBytes(execAddr uintptr, size int) ([]byte, bool) { return nil, false }

func PatchRel32At(patchAddr, targetAddr uintptr) {}

func flushICacheDual(writableAddr, execAddr, size uintptr) {}
