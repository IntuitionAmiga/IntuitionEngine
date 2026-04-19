//go:build !(amd64 && (linux || windows))

package main

// x86ResolveTerminatorTarget is unavailable on platforms where the amd64/Linux
// x86 JIT emitter is not compiled in.
func x86ResolveTerminatorTarget(ji *X86JITInstr, memory []byte, startPC uint32) (uint32, bool) {
	return 0, false
}
