//go:build arm64 && (linux || windows || darwin)

package main

// The fast MMIO poll recognizers are currently implemented only for the
// amd64 emitters. Arm64 keeps correctness by falling through to the normal
// JIT/interpreter path.
func (cpu *CPU64) tryFastIE64MMIOPollLoop() (bool, uint32) {
	return false, 0
}

func (cpu *CPU_Z80) tryFastZ80MMIOPollLoop(adapter *Z80BusAdapter) (bool, uint32, uint32) {
	return false, 0, 0
}
