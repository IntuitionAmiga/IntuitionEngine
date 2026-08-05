package main

// jitTestRetire provides a deterministic execution boundary for native and
// wasm 6502 integration tests. It is deliberately backend-neutral: the
// dispatcher supplies the number of guest instructions its block retired.
func (cpu *CPU_6502) jitTestRetire(count uint32) bool {
	if cpu.jitTestStopAfter == 0 || count == 0 {
		return false
	}
	cpu.jitTestRetired += uint64(count)
	if cpu.jitTestRetired < cpu.jitTestStopAfter {
		return false
	}
	cpu.running.Store(false)
	return true
}
