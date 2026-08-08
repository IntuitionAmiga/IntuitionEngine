//go:build linux && (amd64 || arm64)

package main

// ie32JITExecMemSize retains the generated working set of long-running guest
// programs. A 16 MiB arena matches the established IE64 and x86 native JIT
// pool size without adding a second cache-management policy.
const ie32JITExecMemSize = 16 * 1024 * 1024

// ie32JITEnterGenerated reserves the executable-memory arena used by lowered
// blocks. The marker is never called: provenance counters must describe real
// opcode lowering, not a no-op host return inserted by the dispatcher.
func ie32JITEnterGenerated(cpu *CPU) {
	if cpu == nil || cpu.jit == nil {
		return
	}
	if cpu.jit.execMem == nil {
		em, err := AllocExecMem(ie32JITExecMemSize)
		if err != nil {
			return
		}
		cpu.jit.execMem = em
		addr, err := em.Write(ie32JITReturnCode())
		if err != nil {
			em.Free()
			cpu.jit.execMem = nil
			return
		}
		cpu.jitMarker = addr
	}
}

func ie32JITReturnCode() []byte {
	if hostArchARM64 {
		return []byte{0xC0, 0x03, 0x5F, 0xD6} // RET
	}
	return []byte{0xC3} // ret
}
