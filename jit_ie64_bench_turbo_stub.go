// jit_ie64_bench_turbo_stub.go - non-amd64 IE64 benchmark turbo stubs.

//go:build arm64 && (linux || windows || darwin)

package main

func (cpu *CPU64) tryIE64TurboProgram(pcPhys uint32, statsEnabled bool) (bool, uint64) {
	return false, 0
}

func ie64TurboStartCandidate(mem []byte, pc uint32) bool {
	return false
}
