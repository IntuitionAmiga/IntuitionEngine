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

func createZ80Worker(bus *MachineBus, data []byte) (*CoprocWorker, error) {
	if len(data) > int(WORKER_Z80_SIZE) {
		return nil, fmt.Errorf("Z80 service binary too large: %d > %d", len(data), WORKER_Z80_SIZE)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range uint32(WORKER_Z80_SIZE) {
		mem[WORKER_Z80_BASE+i] = 0
	}

	// Copy service binary to worker region
	copy(mem[WORKER_Z80_BASE:], data)

	// Create coproc Z80 bus adapter with mailbox window at Z80 addr $2000-$3FFF
	coprocBus := &CoprocZ80Bus{
		bus:          bus,
		mem:          mem,
		bankBase:     WORKER_Z80_BASE,
		mailboxBase:  MAILBOX_BASE,
		mailboxStart: 0x2000,
		mailboxEnd:   0x4000,
	}

	// Create Z80 CPU with the coproc bus
	cpu := NewCPU_Z80(coprocBus)
	cpu.PC = 0x0000

	done := make(chan struct{})
	worker := &CoprocWorker{
		cpuType:  EXEC_TYPE_Z80,
		stop:     func() { cpu.SetRunning(false) },
		done:     done,
		loadBase: WORKER_Z80_BASE,
		loadEnd:  WORKER_Z80_END,
		debugCPU: NewDebugZ80(cpu, nil),
	}

	go func() {
		defer close(done)
		cpu.Execute()
	}()

	return worker, nil
}
