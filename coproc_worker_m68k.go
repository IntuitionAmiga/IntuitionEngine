package main

import "fmt"

func createM68KWorker(bus *MachineBus, data []byte) (*CoprocWorker, error) {
	if len(data) > int(WORKER_M68K_SIZE) {
		return nil, fmt.Errorf("M68K service binary too large: %d > %d", len(data), WORKER_M68K_SIZE)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range uint32(WORKER_M68K_SIZE) {
		mem[WORKER_M68K_BASE+i] = 0
	}

	// Copy service binary to worker region (raw bytes - M68K fetch handles byte ordering)
	copy(mem[WORKER_M68K_BASE:], data)

	// Create M68K CPU using the shared bus (M68K uses 32-bit addressing directly)
	cpu := NewM68KCPU(bus)
	cpu.CoprocMode = true // Skip byte-swap for shared data regions (mailbox + user data)
	cpu.PC = WORKER_M68K_BASE
	cpu.AddrRegs[7] = WORKER_M68K_END - 0xFF // Stack at top of worker region

	done := make(chan struct{})
	stopFn := func() { cpu.SetRunning(false) }
	execFn := func() { cpu.SetRunning(true); cpu.ExecuteInstruction() }

	adapter := NewDebugM68K(cpu, nil)

	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_M68K,
		monitorID: -1,
		stop:      stopFn,
		stopCPU:   stopFn,
		execCPU:   execFn,
		done:      done,
		loadBase:  WORKER_M68K_BASE,
		loadEnd:   WORKER_M68K_END,
		debugCPU:  adapter,
	}

	adapter.workerFreeze = worker.Pause
	adapter.workerResume = worker.Unpause

	go func() {
		defer close(done)
		cpu.ExecuteInstruction()
	}()

	return worker, nil
}
