package main

import (
	"fmt"
	"os"
)

const (
	default6502LoadAddr = 0x0600
)

type CPU6502Config struct {
	LoadAddr uint16
	Entry    uint16
}

type CPU6502Runner struct {
	cpu         *CPU_6502
	bus         *MachineBus
	loadAddr    uint16
	entry       uint16
	PerfEnabled bool
}

func NewCPU6502Runner(bus *MachineBus, config CPU6502Config) *CPU6502Runner {
	loadAddr := config.LoadAddr
	if loadAddr == 0 {
		loadAddr = default6502LoadAddr
	}

	return &CPU6502Runner{
		cpu:      NewCPU_6502(bus),
		bus:      bus,
		loadAddr: loadAddr,
		entry:    config.Entry,
	}
}

func (r *CPU6502Runner) LoadProgram(filename string) error {
	program, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	r.bus.Reset()

	entry := r.entry
	if entry == 0 {
		entry = r.loadAddr
	}

	endAddr := uint32(r.loadAddr) + uint32(len(program))
	if endAddr > DEFAULT_MEMORY_SIZE {
		return fmt.Errorf("6502 program too large: end=0x%X, limit=0x%X", endAddr, DEFAULT_MEMORY_SIZE)
	}

	for i, value := range program {
		r.bus.Write8(uint32(r.loadAddr)+uint32(i), value)
	}

	// Reset/NMI/IRQ vectors point at entry by default.
	r.bus.Write8(RESET_VECTOR, uint8(entry&0x00FF))
	r.bus.Write8(RESET_VECTOR+1, uint8(entry>>8))
	r.bus.Write8(NMI_VECTOR, uint8(entry&0x00FF))
	r.bus.Write8(NMI_VECTOR+1, uint8(entry>>8))
	r.bus.Write8(IRQ_VECTOR, uint8(entry&0x00FF))
	r.bus.Write8(IRQ_VECTOR+1, uint8(entry>>8))

	r.cpu.Reset()
	r.cpu.SetRDYLine(true)
	return nil
}

func (r *CPU6502Runner) Reset() {
	r.cpu.Reset()
	r.cpu.SetRDYLine(true)
}

func (r *CPU6502Runner) Execute() {
	r.cpu.PerfEnabled = r.PerfEnabled
	r.cpu.Execute()
}

func (r *CPU6502Runner) CPU() *CPU_6502 {
	return r.cpu
}
