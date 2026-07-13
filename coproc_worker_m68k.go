package main

import "fmt"

func createM68KWorker(bus *MachineBus, data []byte, instance uint32) (*CoprocWorker, error) {
	// Each instance owns its own computed window (workerWindow).
	base, end, size, ok := workerWindow(EXEC_TYPE_M68K, instance)
	if !ok {
		return nil, fmt.Errorf("M68K worker instance out of range: %d", instance)
	}
	if len(data) > int(size) {
		return nil, fmt.Errorf("M68K service binary too large: %d > %d", len(data), size)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range size {
		mem[base+i] = 0
	}
	invalidateM68KJITForGuestWrite(bus, uint64(base), uint64(size))

	// Copy service binary to worker region (raw bytes - M68K fetch handles byte ordering)
	copy(mem[base:], data)
	invalidateM68KJITForGuestWrite(bus, uint64(base), uint64(len(data)))

	// Create M68K CPU using the shared bus (M68K uses 32-bit addressing directly)
	cpu := NewM68KCPU(bus)
	cpu.CoprocMode = true // Skip byte-swap for shared data regions (mailbox + user data)
	cpu.PC = base
	// Seed A4 with the assigned ring base so a fixed-ring service image serves
	// whichever instance ring the manager selected (bootstrap patch); the
	// shipped service reads its ring through A4, not a hard-coded constant.
	if ring := coprocRingIndex(EXEC_TYPE_M68K, instance); ring >= 0 {
		cpu.AddrRegs[4] = ringBaseAddr(ring)
	}
	cpu.AddrRegs[7] = end - 0xFF // Stack at top of worker region
	cpu.SSP = cpu.AddrRegs[7]
	cpu.USP = cpu.SSP
	// NewM68KCPU tuned the stack bounds around the reset-vector SP; re-tune
	// them for the worker stack or every push (JSR, exception frame) faults.
	cpu.tuneStackBounds(cpu.AddrRegs[7])
	// Only NewM68KRunner arms the JIT; a worker CPU has no runner, so arm it
	// here or the service executes interpreted (~7x slower measured).
	cpu.m68kJitEnabled = m68kJitAvailable

	done := make(chan struct{})
	stopFn := func() { cpu.SetRunning(false) }
	// Workers run under the JIT like the main runner; interpreted service
	// loops stall guests that wait on their results.
	execFn := func() { cpu.SetRunning(true); cpu.m68kJitExecute() }

	adapter := NewDebugM68K(cpu, nil)

	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_M68K,
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
		cpu.m68kJitExecute()
	}()

	return worker, nil
}
