package main

import "fmt"

func createX86Worker(bus *MachineBus, data []byte) (*CoprocWorker, error) {
	if len(data) > int(WORKER_X86_SIZE) {
		return nil, fmt.Errorf("x86 service binary too large: %d > %d", len(data), WORKER_X86_SIZE)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range uint32(WORKER_X86_SIZE) {
		mem[WORKER_X86_BASE+i] = 0
	}

	// Copy service binary to worker region
	copy(mem[WORKER_X86_BASE:], data)

	// Create x86 bus adapter (32-bit addressing, no VGA/Voodoo for workers)
	adapter := NewX86BusAdapter(bus)

	// Create x86 CPU with the adapter
	cpu := NewCPU_X86(adapter)
	cpu.EIP = WORKER_X86_BASE
	cpu.ESP = WORKER_X86_END - 0xFF // Stack at top of worker region

	done := make(chan struct{})
	stopFn := func() { cpu.SetRunning(false) }
	execFn := func() {
		cpu.SetRunning(true)
		for cpu.Running() {
			cpu.Step()
		}
	}

	dbg := NewDebugX86(cpu, nil)

	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_X86,
		monitorID: -1,
		stop:      stopFn,
		stopCPU:   stopFn,
		execCPU:   execFn,
		done:      done,
		loadBase:  WORKER_X86_BASE,
		loadEnd:   WORKER_X86_END,
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
