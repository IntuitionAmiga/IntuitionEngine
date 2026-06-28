//go:build !(amd64 && (linux || windows || darwin))

package main

func (cpu *M68KCPU) invalidateM68KJITForGuestWrite(addr uint32, size uint32) {}

func invalidateM68KJITForGuestWrite(bus Bus32, addr uint64, size uint64) {}

// No M68K JIT on this platform: invalidation enqueue is a no-op.
func (cpu *M68KCPU) m68kEnqueueJITInvalidation(addr, size uint32) {}

func (cpu *M68KCPU) m68kDrainPendingJITInvalidations() {}

// No M68K JIT verifier on this platform: capture is a no-op.
func (cpu *M68KCPU) m68kVerifyCaptureWrite(addr uint32, size int) bool { return false }
