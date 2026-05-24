package main

import "fmt"

// WriteAssembledCodeRAM validates and writes monitor-assembled IE64 code to
// physical RAM, then performs a full IE64 JIT/code-cache flush.
func (cpu *CPU64) WriteAssembledCodeRAM(addr uint64, data []byte) error {
	if cpu == nil {
		return fmt.Errorf("IE64 CPU unavailable")
	}
	if cpu.IsRunning() {
		return fmt.Errorf("IE64 CPU must be stopped before monitor code writes")
	}
	if len(data) == 0 {
		return fmt.Errorf("no bytes to write")
	}
	if cpu.bus == nil {
		return fmt.Errorf("machine bus unavailable")
	}
	if err := cpu.bus.WritePhysRAMOnly(addr, data); err != nil {
		return err
	}
	cpu.FlushIE64JITFull()
	return nil
}

// FlushIE64JITFull mirrors the full-cache cleanup used by IE64 self-modifying
// code invalidation. It is a safe no-op when JIT state is uninitialized.
func (cpu *CPU64) FlushIE64JITFull() {
	if cpu == nil {
		return
	}
	flushed := false
	if cpu.jitCache != nil {
		cpu.jitCache.Invalidate()
		flushed = true
	}
	if em, ok := cpu.jitExecMem.(*ExecMem); ok && em != nil {
		em.Reset()
		flushed = true
	}
	if cpu.jitCtx != nil {
		cpu.jitCtx.NeedInval = 0
		cpu.jitCtx.NeedIOFallback = 0
		cpu.jitCtx.RTSCache0PC = 0
		cpu.jitCtx.RTSCache0Addr = 0
		cpu.jitCtx.RTSCache1PC = 0
		cpu.jitCtx.RTSCache1Addr = 0
		cpu.jitCtx.RTSCache2PC = 0
		cpu.jitCtx.RTSCache2Addr = 0
		cpu.jitCtx.RTSCache3PC = 0
		cpu.jitCtx.RTSCache3Addr = 0
		flushed = true
	}
	cpu.jitNeedInval = false
	if flushed {
		globalIE64TurboStats.invalidations.Add(1)
	}
}
