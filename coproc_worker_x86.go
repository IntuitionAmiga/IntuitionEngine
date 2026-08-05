package main

import (
	"fmt"
)

func createX86Worker(bus *MachineBus, data []byte, instance uint32) (*CoprocWorker, error) {
	base, end, size, ok := workerWindow(EXEC_TYPE_X86, instance)
	if !ok {
		return nil, fmt.Errorf("x86 worker instance out of range: %d", instance)
	}
	if len(data) > int(size) {
		return nil, fmt.Errorf("x86 service binary too large: %d > %d", len(data), size)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range size {
		mem[base+i] = 0
	}

	// Copy service binary to worker region
	copy(mem[base:], data)

	// Create x86 bus adapter (32-bit addressing, no VGA/Voodoo for workers)
	adapter := NewX86BusAdapter(bus)

	// Create x86 CPU with the adapter, with the same JIT wiring as the
	// main runner: flat memory base and the I/O page bitmap the emitted
	// code consults on every memory access.
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitEnabled = x86JitAvailable
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
	cpu.EIP = base
	cpu.ESP = end - 0xFF // Stack at top of worker region
	// Seed EBP with the assigned ring base so a fixed-ring service image serves
	// whichever instance ring the manager selected (bootstrap patch); the
	// shipped service reads its ring through EBP, not a hard-coded constant.
	if ring := coprocRingIndex(EXEC_TYPE_X86, instance); ring >= 0 {
		cpu.EBP = ringBaseAddr(ring)
	}

	done := make(chan struct{})
	stopFn := func() { cpu.SetRunning(false) }
	execFn := func() {
		cpu.SetRunning(true)
		cpu.x86JitExecute()
	}

	dbg := NewDebugX86(cpu, nil)

	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_X86,
		monitorID: -1,
		stop:      stopFn,
		stopCPU:   stopFn,
		execCPU:   execFn,
		done:      done,
		loadBase:  base,
		loadEnd:   end,
		debugCPU:  dbg,
	}

	dbg.workerFreeze = worker.Pause
	dbg.workerResume = worker.Unpause

	go func() {
		defer close(done)
		execFn()
	}()

	return worker, nil
}
