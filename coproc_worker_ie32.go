package main

import "fmt"

func createIE32Worker(bus *MachineBus, data []byte) (*CoprocWorker, error) {
	if len(data) > int(WORKER_IE32_SIZE) {
		return nil, fmt.Errorf("IE32 service binary too large: %d > %d", len(data), WORKER_IE32_SIZE)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range uint32(WORKER_IE32_SIZE) {
		mem[WORKER_IE32_BASE+i] = 0
	}

	// Copy service binary to worker region
	copy(mem[WORKER_IE32_BASE:], data)

	// Create IE32 CPU using the shared bus
	cpu := NewCPU(bus)
	cpu.PC = WORKER_IE32_BASE
	cpu.SP = WORKER_IE32_END - 0xFF // Stack at top of worker region
	cpu.CoprocMode = true           // Skip PC range check in Execute()

	done := make(chan struct{})
	worker := &CoprocWorker{
		cpuType:  EXEC_TYPE_IE32,
		stop:     func() { cpu.running.Store(false) },
		done:     done,
		loadBase: WORKER_IE32_BASE,
		loadEnd:  WORKER_IE32_END,
		debugCPU: NewDebugIE32(cpu),
	}

	go func() {
		defer close(done)
		cpu.Execute()
	}()

	return worker, nil
}
