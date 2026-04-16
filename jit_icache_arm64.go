// jit_icache_arm64.go - ARM64 instruction cache flush

//go:build arm64 && linux

package main

// flushICacheASM uses DC CVAU + IC IVAU + DSB ISH + ISB to flush
// the instruction cache for the given address range on a single VA.
// Implemented in jit_icache_arm64.s.
func flushICacheASM(addr, size uintptr)

// dcCleanToPoUASM cleans the D-cache by VA to PoU over [addr, addr+size)
// and issues DSB ISH. Implemented in jit_icache_arm64.s.
func dcCleanToPoUASM(addr, size uintptr)

// icInvalidateToPoUASM invalidates the I-cache by VA to PoU over
// [addr, addr+size) and issues DSB ISH + ISB. Implemented in
// jit_icache_arm64.s.
func icInvalidateToPoUASM(addr, size uintptr)

// flushICache ensures instruction cache coherency for newly written code
// on ARM64 when the writable and exec aliases coincide (single-VA).
// ARM64 does not guarantee icache/dcache coherency, so after writing code
// to memory we must invalidate the icache.
func flushICache(addr, size uintptr) {
	if size == 0 {
		return
	}
	flushICacheASM(addr, size)
}

// flushICacheDual is the correct icache-coherency sequence for a
// dual-mapped JIT region where stores went through one alias
// (writableAddr) and fetches happen through another (execAddr). The
// D-cache is cleaned by writable VA and the I-cache is invalidated by
// exec VA; barriers are issued between and after.
func flushICacheDual(writableAddr, execAddr, size uintptr) {
	if size == 0 {
		return
	}
	dcCleanToPoUASM(writableAddr, size)
	icInvalidateToPoUASM(execAddr, size)
}
