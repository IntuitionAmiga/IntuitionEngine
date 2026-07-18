//go:build !(amd64 && (linux || windows || darwin)) && !(arm64 && (linux || windows || darwin))

package main

// No M68K JIT native code on this platform: applying an invalidation range
// and resetting the code cache are no-ops. The shared enqueue/drain queue
// (jit_m68k_inval_queue.go) still exists but drains into these no-ops.
func (cpu *M68KCPU) invalidateM68KJITForGuestWrite(addr uint32, size uint32) {}

func (cpu *M68KCPU) m68kResetJITCodeCache() {}

// No M68K JIT verifier on this platform: capture is a no-op.
func (cpu *M68KCPU) m68kVerifyCaptureWrite(addr uint32, size int) bool { return false }
