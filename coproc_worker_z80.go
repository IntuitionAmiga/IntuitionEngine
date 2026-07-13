package main

import "fmt"

// CoprocZ80Bus implements Z80Bus for coprocessor workers.
// It flat-maps the 16-bit Z80 address space to a dedicated bus region,
// with a window for the shared mailbox.
type CoprocZ80Bus struct {
	bus          *MachineBus
	mem          []byte // direct reference to bus memory
	bankBase     uint32 // Z80 addr 0 → bus addr bankBase
	mailboxBase  uint32 // bus addr of mailbox region
	mailboxStart uint16 // Z80 addr where mailbox is mapped
	mailboxEnd   uint16 // Z80 addr end of mailbox window
}

func (b *CoprocZ80Bus) translate(addr uint16) uint32 {
	if addr >= b.mailboxStart && addr < b.mailboxEnd {
		return b.mailboxBase + uint32(addr-b.mailboxStart)
	}
	return b.bankBase + uint32(addr)
}

func (b *CoprocZ80Bus) Read(addr uint16) byte {
	return b.mem[b.translate(addr)]
}

func (b *CoprocZ80Bus) Write(addr uint16, v byte) {
	b.mem[b.translate(addr)] = v
}

func (b *CoprocZ80Bus) In(port uint16) byte {
	return 0 // No I/O ports for coprocessor workers
}

func (b *CoprocZ80Bus) Out(port uint16, v byte) {
	// No I/O ports for coprocessor workers
}

func (b *CoprocZ80Bus) Tick(cycles int) {
	// No cycle-accurate timing for coprocessor workers
}

func createZ80Worker(bus *MachineBus, data []byte, instance uint32) (*CoprocWorker, error) {
	base, end, size, ok := workerWindow(EXEC_TYPE_Z80, instance)
	if !ok {
		return nil, fmt.Errorf("Z80 worker instance out of range: %d", instance)
	}
	if len(data) > int(size) {
		return nil, fmt.Errorf("Z80 service binary too large: %d > %d", len(data), size)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range size {
		mem[base+i] = 0
	}

	// Copy service binary to worker region
	copy(mem[base:], data)

	// Create coproc Z80 bus adapter with mailbox window at Z80 addr
	// $2000 through $2000+MAILBOX_SIZE-1.
	coprocBus := &CoprocZ80Bus{
		bus:          bus,
		mem:          mem,
		bankBase:     base,
		mailboxBase:  MAILBOX_BASE,
		mailboxStart: 0x2000,
		mailboxEnd:   0x2000 + uint16(MAILBOX_SIZE),
	}

	// Create Z80 CPU with the coproc bus
	cpu := NewCPU_Z80(coprocBus)
	cpu.PC = 0x0000

	done := make(chan struct{})
	stopFn := func() { cpu.SetRunning(false) }
	// Workers run under the JIT like the main runner; interpreted service
	// loops stall guests that wait on their results.
	execFn := func() { cpu.SetRunning(true); cpu.z80JitExecute() }

	adapter := NewDebugZ80(cpu, nil)

	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_Z80,
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
		cpu.z80JitExecute()
	}()

	return worker, nil
}
