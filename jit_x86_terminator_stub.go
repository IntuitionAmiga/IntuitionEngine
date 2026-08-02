//go:build !((amd64 && (linux || windows || darwin)) || (arm64 && linux))

package main

// x86ResolveTerminatorTarget is unavailable on platforms without a native x86
// JIT backend that supports direct-branch regions.
func x86ResolveTerminatorTarget(ji *X86JITInstr, memory []byte, startPC uint32) (uint32, bool) {
	return 0, false
}
