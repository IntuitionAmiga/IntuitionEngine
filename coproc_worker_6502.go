package main

import (
	"encoding/binary"
	"fmt"
)

// CoprocBus32 implements Bus32 for 6502/Z80 coprocessor workers.
// It flat-maps the 16-bit CPU address space to a dedicated bus region,
// with a window for the shared mailbox.
type CoprocBus32 struct {
	bus          *MachineBus
	mem          []byte // direct reference to bus memory
	bankBase     uint32 // CPU addr 0 → bus addr bankBase
	mailboxBase  uint32 // bus addr of mailbox region
	mailboxStart uint16 // CPU addr where mailbox is mapped
	mailboxEnd   uint16 // CPU addr end of mailbox window
}

func (b *CoprocBus32) translate(addr uint32) uint32 {
	a16 := uint16(addr)
	if a16 >= b.mailboxStart && a16 < b.mailboxEnd {
		return b.mailboxBase + uint32(a16-b.mailboxStart)
	}
	return b.bankBase + addr
}

func (b *CoprocBus32) Read8(addr uint32) uint8 {
	return b.mem[b.translate(addr)]
}

func (b *CoprocBus32) Write8(addr uint32, value uint8) {
	b.mem[b.translate(addr)] = value
}

func (b *CoprocBus32) Read16(addr uint32) uint16 {
	a := b.translate(addr)
	return binary.LittleEndian.Uint16(b.mem[a:])
}

func (b *CoprocBus32) Write16(addr uint32, value uint16) {
	a := b.translate(addr)
	binary.LittleEndian.PutUint16(b.mem[a:], value)
}

func (b *CoprocBus32) Read32(addr uint32) uint32 {
	a := b.translate(addr)
	return binary.LittleEndian.Uint32(b.mem[a:])
}

func (b *CoprocBus32) Write32(addr uint32, value uint32) {
	a := b.translate(addr)
	binary.LittleEndian.PutUint32(b.mem[a:], value)
}

func (b *CoprocBus32) Reset() {
	// No-op: coprocessor workers NEVER reset the bus
}

func (b *CoprocBus32) GetMemory() []byte {
	return b.mem[b.bankBase : b.bankBase+0x10000]
}

func create6502Worker(bus *MachineBus, data []byte) (*CoprocWorker, error) {
	if len(data) > int(WORKER_6502_SIZE) {
		return nil, fmt.Errorf("6502 service binary too large: %d > %d", len(data), WORKER_6502_SIZE)
	}

	// Zero the worker's dedicated memory region
	mem := bus.GetMemory()
	for i := range uint32(WORKER_6502_SIZE) {
		mem[WORKER_6502_BASE+i] = 0
	}

	// Copy service binary to worker region at offset 0 (CPU addr $0000)
	copy(mem[WORKER_6502_BASE:], data)

	// Create coproc bus adapter with mailbox window at CPU addr $2000-$3FFF
	coprocBus := &CoprocBus32{
		bus:          bus,
		mem:          mem,
		bankBase:     WORKER_6502_BASE,
		mailboxBase:  MAILBOX_BASE,
		mailboxStart: 0x2000,
		mailboxEnd:   0x4000,
	}

	// Set reset vector at CPU addr $FFFC-$FFFD to point to entry (CPU addr $0000)
	entryAddr := uint16(0x0000)
	coprocBus.Write8(0xFFFC, uint8(entryAddr&0xFF))
	coprocBus.Write8(0xFFFD, uint8(entryAddr>>8))

	// Create 6502 CPU with the coproc bus adapter
	cpu := NewCPU_6502(coprocBus)
	cpu.Reset()
	cpu.SetRDYLine(true)

	done := make(chan struct{})
	stopFn := func() { cpu.SetRunning(false) }
	execFn := func() { cpu.SetRunning(true); cpu.Execute() }

	adapter := NewDebug6502(cpu, nil)

	worker := &CoprocWorker{
		cpuType:   EXEC_TYPE_6502,
		monitorID: -1,
		stop:      stopFn,
		stopCPU:   stopFn,
		execCPU:   execFn,
		done:      done,
		loadBase:  WORKER_6502_BASE,
		loadEnd:   WORKER_6502_END,
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
