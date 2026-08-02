//go:build !((amd64 && (linux || windows || darwin)) || (arm64 && linux) || (js && wasm))

package main

// x86ResolveTerminatorTarget is unavailable on platforms without an x86 JIT
// frontend that can form direct-branch regions.
func x86ResolveTerminatorTarget(ji *X86JITInstr, memory []byte, startPC uint32) (uint32, bool) {
	return 0, false
}
