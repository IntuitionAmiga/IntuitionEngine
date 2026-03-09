package main

import "fmt"

func createIE64Worker(bus *MachineBus, data []byte) (*CoprocWorker, error) {
	if len(data) > int(WORKER_IE64_SIZE) {
		return nil, fmt.Errorf("IE64 service binary too large: %d > %d", len(data), WORKER_IE64_SIZE)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range uint32(WORKER_IE64_SIZE) {
		mem[WORKER_IE64_BASE+i] = 0
	}

	// Copy service binary to worker region
	copy(mem[WORKER_IE64_BASE:], data)

	// Create IE64 CPU using the shared bus
	cpu := NewCPU64(bus)
	cpu.PC = uint64(WORKER_IE64_BASE)
	cpu.regs[31] = uint64(WORKER_IE64_END - 0xFF) // Stack at top of worker region
	cpu.CoprocMode = true                         // Skip PC range check in Execute()
	cpu.jitEnabled = jitAvailable                 // Use JIT when available

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
		loadBase:  WORKER_IE64_BASE,
		loadEnd:   WORKER_IE64_END,
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
