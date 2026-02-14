// debug_cpu_z80.go - Z80 debug adapter for Machine Monitor

package main

import (
	"strings"
	"sync"
	"sync/atomic"
)

type DebugZ80 struct {
	cpu    *CPU_Z80
	runner *CPUZ80Runner

	bpMu        sync.RWMutex
	breakpoints map[uint64]bool
	bpChan      chan<- BreakpointEvent
	cpuID       int
	trapRunning atomic.Bool
	trapStop    chan struct{}
}

func NewDebugZ80(cpu *CPU_Z80, runner *CPUZ80Runner) *DebugZ80 {
	return &DebugZ80{
		cpu:         cpu,
		runner:      runner,
		breakpoints: make(map[uint64]bool),
	}
}

func (d *DebugZ80) CPUName() string   { return "Z80" }
func (d *DebugZ80) AddressWidth() int { return 16 }

func (d *DebugZ80) GetRegisters() []RegisterInfo {
	c := d.cpu
	return []RegisterInfo{
		{Name: "A", BitWidth: 8, Value: uint64(c.A), Group: "general"},
		{Name: "F", BitWidth: 8, Value: uint64(c.F), Group: "flags"},
		{Name: "B", BitWidth: 8, Value: uint64(c.B), Group: "general"},
		{Name: "C", BitWidth: 8, Value: uint64(c.C), Group: "general"},
		{Name: "D", BitWidth: 8, Value: uint64(c.D), Group: "general"},
		{Name: "E", BitWidth: 8, Value: uint64(c.E), Group: "general"},
		{Name: "H", BitWidth: 8, Value: uint64(c.H), Group: "general"},
		{Name: "L", BitWidth: 8, Value: uint64(c.L), Group: "general"},
		{Name: "A'", BitWidth: 8, Value: uint64(c.A2), Group: "shadow"},
		{Name: "F'", BitWidth: 8, Value: uint64(c.F2), Group: "shadow"},
		{Name: "B'", BitWidth: 8, Value: uint64(c.B2), Group: "shadow"},
		{Name: "C'", BitWidth: 8, Value: uint64(c.C2), Group: "shadow"},
		{Name: "D'", BitWidth: 8, Value: uint64(c.D2), Group: "shadow"},
		{Name: "E'", BitWidth: 8, Value: uint64(c.E2), Group: "shadow"},
		{Name: "H'", BitWidth: 8, Value: uint64(c.H2), Group: "shadow"},
		{Name: "L'", BitWidth: 8, Value: uint64(c.L2), Group: "shadow"},
		{Name: "IX", BitWidth: 16, Value: uint64(c.IX), Group: "index"},
		{Name: "IY", BitWidth: 16, Value: uint64(c.IY), Group: "index"},
		{Name: "SP", BitWidth: 16, Value: uint64(c.SP), Group: "general"},
		{Name: "PC", BitWidth: 16, Value: uint64(c.PC), Group: "general"},
		{Name: "I", BitWidth: 8, Value: uint64(c.I), Group: "status"},
		{Name: "R", BitWidth: 8, Value: uint64(c.R), Group: "status"},
		{Name: "IM", BitWidth: 8, Value: uint64(c.IM), Group: "status"},
	}
}

func (d *DebugZ80) GetRegister(name string) (uint64, bool) {
	c := d.cpu
	switch strings.ToUpper(name) {
	case "A":
		return uint64(c.A), true
	case "F":
		return uint64(c.F), true
	case "B":
		return uint64(c.B), true
	case "C":
		return uint64(c.C), true
	case "D":
		return uint64(c.D), true
	case "E":
		return uint64(c.E), true
	case "H":
		return uint64(c.H), true
	case "L":
		return uint64(c.L), true
	case "IX":
		return uint64(c.IX), true
	case "IY":
		return uint64(c.IY), true
	case "SP":
		return uint64(c.SP), true
	case "PC":
		return uint64(c.PC), true
	case "I":
		return uint64(c.I), true
	case "R":
		return uint64(c.R), true
	case "IM":
		return uint64(c.IM), true
	}
	return 0, false
}

func (d *DebugZ80) SetRegister(name string, value uint64) bool {
	c := d.cpu
	switch strings.ToUpper(name) {
	case "A":
		c.A = byte(value)
	case "F":
		c.F = byte(value)
	case "B":
		c.B = byte(value)
	case "C":
		c.C = byte(value)
	case "D":
		c.D = byte(value)
	case "E":
		c.E = byte(value)
	case "H":
		c.H = byte(value)
	case "L":
		c.L = byte(value)
	case "IX":
		c.IX = uint16(value)
	case "IY":
		c.IY = uint16(value)
	case "SP":
		c.SP = uint16(value)
	case "PC":
		c.PC = uint16(value)
	default:
		return false
	}
	return true
}

func (d *DebugZ80) GetPC() uint64     { return uint64(d.cpu.PC) }
func (d *DebugZ80) SetPC(addr uint64) { d.cpu.PC = uint16(addr) }

func (d *DebugZ80) IsRunning() bool {
	return d.cpu.Running() || d.trapRunning.Load()
}

func (d *DebugZ80) Freeze() {
	if d.trapRunning.Load() {
		close(d.trapStop)
		for d.trapRunning.Load() {
		}
		return
	}
	d.runner.Stop()
}

func (d *DebugZ80) Resume() {
	d.bpMu.RLock()
	hasBP := len(d.breakpoints) > 0
	d.bpMu.RUnlock()
	if hasBP {
		d.trapStop = make(chan struct{})
		d.trapRunning.Store(true)
		go d.trapLoop()
		return
	}
	d.runner.StartExecution()
}

func (d *DebugZ80) trapLoop() {
	defer d.trapRunning.Store(false)
	defer d.cpu.SetRunning(false)
	d.cpu.SetRunning(true)
	for {
		select {
		case <-d.trapStop:
			return
		default:
		}
		d.bpMu.RLock()
		if len(d.breakpoints) > 0 && d.breakpoints[uint64(d.cpu.PC)] {
			d.bpMu.RUnlock()
			if d.bpChan != nil {
				select {
				case d.bpChan <- BreakpointEvent{CPUID: d.cpuID, Address: uint64(d.cpu.PC)}:
				default:
				}
			}
			return
		}
		d.bpMu.RUnlock()
		d.cpu.Step()
	}
}

func (d *DebugZ80) Step() int {
	d.cpu.SetRunning(true)
	d.cpu.Step()
	d.cpu.SetRunning(false)
	return 1
}

func (d *DebugZ80) Disassemble(addr uint64, count int) []DisassembledLine {
	pc := uint64(d.cpu.PC)
	lines := disassembleZ80(d.ReadMemory, addr, count)
	for i := range lines {
		if lines[i].Address == pc {
			lines[i].IsPC = true
		}
	}
	return lines
}

func (d *DebugZ80) SetBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = true
	return true
}

func (d *DebugZ80) ClearBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.breakpoints[addr]; ok {
		delete(d.breakpoints, addr)
		return true
	}
	return false
}

func (d *DebugZ80) ClearAllBreakpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints = make(map[uint64]bool)
}

func (d *DebugZ80) ListBreakpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.breakpoints))
	for addr := range d.breakpoints {
		result = append(result, addr)
	}
	return result
}

func (d *DebugZ80) HasBreakpoint(addr uint64) bool {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	return d.breakpoints[addr]
}

func (d *DebugZ80) ReadMemory(addr uint64, size int) []byte {
	result := make([]byte, size)
	for i := range size {
		result[i] = d.cpu.bus.Read(uint16(addr) + uint16(i))
	}
	return result
}

func (d *DebugZ80) WriteMemory(addr uint64, data []byte) {
	for i, b := range data {
		d.cpu.bus.Write(uint16(addr)+uint16(i), b)
	}
}

func (d *DebugZ80) SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int) {
	d.bpChan = ch
	d.cpuID = cpuID
}
