// machine_bus_alloc_other.go - PLAN_MAX_RAM slice 10 reviewer P1 fallback.
//
// Platforms without an mmap-backed allocator (notably windows): bus.memory
// is allocated from the Go heap. The boot-time clamp in main.go keeps
// memSize sensible on these platforms so we do not eagerly commit a
// multi-GiB Go slice.

//go:build !linux && !darwin

package main

// busMemBootClamp keeps the appliance from eagerly committing a multi-
// GiB Go slice on platforms without an mmap-backed allocator. 256 MiB
// is large enough for AROS (2 GiB profile clamps to bus.memory anyway),
// EmuTOS (32 MiB), and any IE32/x86/bare-M68K demo workload, while
// staying within reasonable Go-heap limits.
const busMemBootClamp uint64 = 256 * 1024 * 1024

func defaultBusMemAllocator(size uint64) []byte {
	return make([]byte, size)
}

// allocateBusMemory wraps the allocator. Non-mmap platforms use the heap
// allocator and a nil reset closure (machine_bus.go falls back to a
// byte-loop zero, matching the legacy behavior).
func allocateBusMemory(size uint64, allocator func(size uint64) []byte) ([]byte, func()) {
	return allocator(size), nil
}
