//go:build !amd64 || (!linux && !windows && !darwin)

package main

type m68kJITLockstepSession struct{}

// recordReference is a no-op on platforms without the amd64 M68K JIT. The
// lockstep comparator is an amd64-only diagnostic; cpu_m68k.go is compiled on
// every architecture and calls this through a (nil-guarded) session pointer.
func (s *m68kJITLockstepSession) recordReference(cpu *M68KCPU, count uint64) {}
