// debug_cpu_ie32.go - IE32 debug adapter for Machine Monitor

package main

import (
	"strings"
	"sync"
	"sync/atomic"
)

type DebugIE32 struct {
	cpu         *CPU
	bpMu        sync.RWMutex
	breakpoints map[uint64]bool
	bpChan      chan<- BreakpointEvent
	cpuID       int
	trapRunning atomic.Bool
	trapStop    chan struct{}
}

func NewDebugIE32(cpu *CPU) *DebugIE32 {
	return &DebugIE32{
		cpu:         cpu,
		breakpoints: make(map[uint64]bool),
	}
}

func (d *DebugIE32) CPUName() string   { return "IE32" }
func (d *DebugIE32) AddressWidth() int { return 32 }

func (d *DebugIE32) GetRegisters() []RegisterInfo {
	regs := make([]RegisterInfo, 0, 18)
	regVals := [16]uint32{
		d.cpu.A, d.cpu.X, d.cpu.Y, d.cpu.Z,
		d.cpu.B, d.cpu.C, d.cpu.D, d.cpu.E,
		d.cpu.F, d.cpu.G, d.cpu.H, d.cpu.S,
		d.cpu.T, d.cpu.U, d.cpu.V, d.cpu.W,
	}
	for i, name := range ie32RegNames {
		regs = append(regs, RegisterInfo{Name: name, BitWidth: 32, Value: uint64(regVals[i]), Group: "general"})
	}
	regs = append(regs, RegisterInfo{Name: "SP", BitWidth: 32, Value: uint64(d.cpu.SP), Group: "general"})
	regs = append(regs, RegisterInfo{Name: "PC", BitWidth: 32, Value: uint64(d.cpu.PC), Group: "general"})
	return regs
}

func (d *DebugIE32) GetRegister(name string) (uint64, bool) {
	upper := strings.ToUpper(name)
	switch upper {
	case "PC":
		return uint64(d.cpu.PC), true
	case "SP":
		return uint64(d.cpu.SP), true
	case "A":
		return uint64(d.cpu.A), true
	case "X":
		return uint64(d.cpu.X), true
	case "Y":
		return uint64(d.cpu.Y), true
	case "Z":
		return uint64(d.cpu.Z), true
	case "B":
		return uint64(d.cpu.B), true
	case "C":
		return uint64(d.cpu.C), true
	case "D":
		return uint64(d.cpu.D), true
	case "E":
		return uint64(d.cpu.E), true
	case "F":
		return uint64(d.cpu.F), true
	case "G":
		return uint64(d.cpu.G), true
	case "H":
		return uint64(d.cpu.H), true
	case "S":
		return uint64(d.cpu.S), true
	case "T":
		return uint64(d.cpu.T), true
	case "U":
		return uint64(d.cpu.U), true
	case "V":
		return uint64(d.cpu.V), true
	case "W":
		return uint64(d.cpu.W), true
	}
	return 0, false
}

func (d *DebugIE32) SetRegister(name string, value uint64) bool {
	v := uint32(value)
	upper := strings.ToUpper(name)
	switch upper {
	case "PC":
		d.cpu.PC = v
	case "SP":
		d.cpu.SP = v
	case "A":
		d.cpu.A = v
	case "X":
		d.cpu.X = v
	case "Y":
		d.cpu.Y = v
	case "Z":
		d.cpu.Z = v
	case "B":
		d.cpu.B = v
	case "C":
		d.cpu.C = v
	case "D":
		d.cpu.D = v
	case "E":
		d.cpu.E = v
	case "F":
		d.cpu.F = v
	case "G":
		d.cpu.G = v
	case "H":
		d.cpu.H = v
	case "S":
		d.cpu.S = v
	case "T":
		d.cpu.T = v
	case "U":
		d.cpu.U = v
	case "V":
		d.cpu.V = v
	case "W":
		d.cpu.W = v
	default:
		return false
	}
	return true
}

func (d *DebugIE32) GetPC() uint64     { return uint64(d.cpu.PC) }
func (d *DebugIE32) SetPC(addr uint64) { d.cpu.PC = uint32(addr) }

func (d *DebugIE32) IsRunning() bool {
	return d.cpu.IsRunning() || d.trapRunning.Load()
}

func (d *DebugIE32) Freeze() {
	if d.trapRunning.Load() {
		close(d.trapStop)
		for d.trapRunning.Load() {
		}
		return
	}
	d.cpu.Stop()
}

func (d *DebugIE32) Resume() {
	d.bpMu.RLock()
	hasBP := len(d.breakpoints) > 0
	d.bpMu.RUnlock()
	if hasBP {
		d.trapStop = make(chan struct{})
		d.trapRunning.Store(true)
		go d.trapLoop()
		return
	}
	d.cpu.running.Store(true)
	d.cpu.StartExecution()
}

func (d *DebugIE32) trapLoop() {
	defer d.trapRunning.Store(false)
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
		if d.cpu.StepOne() == 0 {
			return
		}
	}
}

func (d *DebugIE32) Step() int { return d.cpu.StepOne() }

func (d *DebugIE32) Disassemble(addr uint64, count int) []DisassembledLine {
	pc := uint64(d.cpu.PC)
	lines := disassembleIE32(d.ReadMemory, addr, count)
	for i := range lines {
		if lines[i].Address == pc {
			lines[i].IsPC = true
		}
	}
	return lines
}

func (d *DebugIE32) SetBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = true
	return true
}

func (d *DebugIE32) ClearBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.breakpoints[addr]; ok {
		delete(d.breakpoints, addr)
		return true
	}
	return false
}

func (d *DebugIE32) ClearAllBreakpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints = make(map[uint64]bool)
}

func (d *DebugIE32) ListBreakpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.breakpoints))
	for addr := range d.breakpoints {
		result = append(result, addr)
	}
	return result
}

func (d *DebugIE32) HasBreakpoint(addr uint64) bool {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	return d.breakpoints[addr]
}

func (d *DebugIE32) ReadMemory(addr uint64, size int) []byte {
	mem := d.cpu.memory
	start := uint32(addr)
	if int(start)+size > len(mem) {
		end := len(mem)
		if int(start) >= end {
			return nil
		}
		return append([]byte{}, mem[start:end]...)
	}
	return append([]byte{}, mem[start:int(start)+size]...)
}

func (d *DebugIE32) WriteMemory(addr uint64, data []byte) {
	mem := d.cpu.memory
	start := uint32(addr)
	if int(start)+len(data) > len(mem) {
		return
	}
	copy(mem[start:], data)
}

func (d *DebugIE32) SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int) {
	d.bpChan = ch
	d.cpuID = cpuID
}
