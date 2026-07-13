package main

import "fmt"

func createIE32Worker(bus *MachineBus, data []byte, instance uint32) (*CoprocWorker, error) {
	base, end, size, ok := workerWindow(EXEC_TYPE_IE32, instance)
	if !ok {
		return nil, fmt.Errorf("IE32 worker instance out of range: %d", instance)
	}
	if len(data) > int(size) {
		return nil, fmt.Errorf("IE32 service binary too large: %d > %d", len(data), size)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range size {
		mem[base+i] = 0
	}

	// Copy service binary to worker region
	copy(mem[base:], data)

	// Create IE32 CPU using the shared bus
	cpu := NewCPU(bus)
	cpu.PC = base
	cpu.SP = end - 0xFF // Stack at top of worker region
	cpu.CoprocMode = true           // Skip PC range check in Execute()

	done := make(chan struct{})
	stopFn := func() { cpu.running.Store(false) }
	execFn := func() { cpu.running.Store(true); cpu.Execute() }

	adapter := NewDebugIE32(cpu)

	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_IE32,
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
		cpu.Execute()
	}()

	return worker, nil
}
