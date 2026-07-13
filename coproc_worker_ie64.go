package main

import "fmt"

func createIE64Worker(bus *MachineBus, data []byte, instance uint32) (*CoprocWorker, error) {
	base, end, size, ok := workerWindow(EXEC_TYPE_IE64, instance)
	if !ok {
		return nil, fmt.Errorf("IE64 worker instance out of range: %d", instance)
	}
	if len(data) > int(size) {
		return nil, fmt.Errorf("IE64 service binary too large: %d > %d", len(data), size)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range size {
		mem[base+i] = 0
	}

	// Copy service binary to worker region
	copy(mem[base:], data)

	// Create IE64 CPU using the shared bus
	cpu := NewCPU64(bus)
	cpu.PC = uint64(base)
	// Seed r30 with the assigned ring base so a fixed-ring service image serves
	// whichever instance ring the manager selected (bootstrap patch). iewarp
	// reads its ring through r30 instead of a hard-coded constant.
	if ring := coprocRingIndex(EXEC_TYPE_IE64, instance); ring >= 0 {
		cpu.regs[30] = uint64(ringBaseAddr(ring))
	}
	cpu.regs[31] = uint64(end - 0xFF)  // Stack at top of worker region
	cpu.CoprocMode = true              // Skip PC range check in Execute()
	cpu.jitEnabled = jitAvailable      // Use JIT when available

	done := make(chan struct{})
	stopFn := func() { cpu.running.Store(false) }
	execFn := func() { cpu.running.Store(true); cpu.jitExecute() }

	adapter := NewDebugIE64(cpu)

	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_IE64,
		monitorID: -1,
		stop:      stopFn,
		stopCPU:   stopFn,
		execCPU:   execFn,
		done:      done,
		loadBase:  base,
		loadEnd:   end,
		debugCPU:  adapter,
	}

	adapter.workerFreeze = worker.Pause
	adapter.workerResume = worker.Unpause

	go func() {
		defer close(done)
		cpu.running.Store(true)
		cpu.jitExecute()
	}()

	return worker, nil
}
