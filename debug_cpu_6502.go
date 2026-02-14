// debug_cpu_6502.go - 6502 debug adapter for Machine Monitor

package main

import (
	"strings"
	"sync"
	"sync/atomic"
)

type Debug6502 struct {
	cpu    *CPU_6502
	runner *CPU6502Runner

	bpMu        sync.RWMutex
	breakpoints map[uint64]*ConditionalBreakpoint
	watchpoints map[uint64]*Watchpoint
	bpChan      chan<- BreakpointEvent
	cpuID       int
	trapRunning atomic.Bool
	trapStop    chan struct{}

	workerFreeze func()
	workerResume func()
}

func NewDebug6502(cpu *CPU_6502, runner *CPU6502Runner) *Debug6502 {
	return &Debug6502{
		cpu:         cpu,
		runner:      runner,
		breakpoints: make(map[uint64]*ConditionalBreakpoint),
		watchpoints: make(map[uint64]*Watchpoint),
	}
}

func (d *Debug6502) CPUName() string   { return "6502" }
func (d *Debug6502) AddressWidth() int { return 16 }

func (d *Debug6502) GetRegisters() []RegisterInfo {
	c := d.cpu
	return []RegisterInfo{
		{Name: "A", BitWidth: 8, Value: uint64(c.A), Group: "general"},
		{Name: "X", BitWidth: 8, Value: uint64(c.X), Group: "general"},
		{Name: "Y", BitWidth: 8, Value: uint64(c.Y), Group: "general"},
		{Name: "SP", BitWidth: 8, Value: uint64(c.SP), Group: "general"},
		{Name: "PC", BitWidth: 16, Value: uint64(c.PC), Group: "general"},
		{Name: "SR", BitWidth: 8, Value: uint64(c.SR), Group: "flags"},
	}
}

func (d *Debug6502) GetRegister(name string) (uint64, bool) {
	c := d.cpu
	switch strings.ToUpper(name) {
	case "A":
		return uint64(c.A), true
	case "X":
		return uint64(c.X), true
	case "Y":
		return uint64(c.Y), true
	case "SP":
		return uint64(c.SP), true
	case "PC":
		return uint64(c.PC), true
	case "SR":
		return uint64(c.SR), true
	}
	return 0, false
}

func (d *Debug6502) SetRegister(name string, value uint64) bool {
	c := d.cpu
	switch strings.ToUpper(name) {
	case "A":
		c.A = byte(value)
	case "X":
		c.X = byte(value)
	case "Y":
		c.Y = byte(value)
	case "SP":
		c.SP = byte(value)
	case "PC":
		c.PC = uint16(value)
	case "SR":
		c.SR = byte(value)
	default:
		return false
	}
	return true
}

func (d *Debug6502) GetPC() uint64     { return uint64(d.cpu.PC) }
func (d *Debug6502) SetPC(addr uint64) { d.cpu.PC = uint16(addr) }

func (d *Debug6502) IsRunning() bool {
	return d.cpu.Running() || d.trapRunning.Load()
}

func (d *Debug6502) Freeze() {
	if d.trapRunning.Load() {
		close(d.trapStop)
		for d.trapRunning.Load() {
		}
		return
	}
	if d.workerFreeze != nil {
		d.workerFreeze()
		return
	}
	d.runner.Stop()
}

func (d *Debug6502) Resume() {
	d.bpMu.RLock()
	hasBP := len(d.breakpoints) > 0 || len(d.watchpoints) > 0
	d.bpMu.RUnlock()
	if hasBP {
		d.trapStop = make(chan struct{})
		d.trapRunning.Store(true)
		go d.trapLoop()
		return
	}
	if d.workerResume != nil {
		d.workerResume()
		return
	}
	d.runner.StartExecution()
}

func (d *Debug6502) trapLoop() {
	defer d.trapRunning.Store(false)
	d.cpu.SetRunning(true)
	for {
		select {
		case <-d.trapStop:
			d.cpu.SetRunning(false)
			return
		default:
		}
		d.bpMu.RLock()
		bp := d.breakpoints[uint64(d.cpu.PC)]
		d.bpMu.RUnlock()
		if bp != nil {
			bp.HitCount++
			if evaluateConditionWithHitCount(bp.Condition, d, bp.HitCount) {
				d.cpu.SetRunning(false)
				if d.bpChan != nil {
					select {
					case d.bpChan <- BreakpointEvent{CPUID: d.cpuID, Address: uint64(d.cpu.PC)}:
					default:
					}
				}
				return
			}
		}
		if d.cpu.Step() == 0 {
			d.cpu.SetRunning(false)
			return
		}
		// Check watchpoints
		d.bpMu.RLock()
		for _, wp := range d.watchpoints {
			cur := d.cpu.memory.Read(uint16(wp.Address))
			if cur != wp.LastValue {
				old := wp.LastValue
				wp.LastValue = cur
				d.bpMu.RUnlock()
				d.cpu.SetRunning(false)
				if d.bpChan != nil {
					select {
					case d.bpChan <- BreakpointEvent{
						CPUID: d.cpuID, Address: uint64(d.cpu.PC),
						IsWatch: true, WatchAddr: wp.Address,
						WatchOldValue: old, WatchNewValue: cur,
					}:
					default:
					}
				}
				return
			}
		}
		d.bpMu.RUnlock()
	}
}

func (d *Debug6502) Step() int {
	return d.cpu.Step()
}

func (d *Debug6502) Disassemble(addr uint64, count int) []DisassembledLine {
	pc := uint64(d.cpu.PC)
	lines := disassemble6502(d.ReadMemory, addr, count)
	for i := range lines {
		if lines[i].Address == pc {
			lines[i].IsPC = true
		}
	}
	return lines
}

func (d *Debug6502) SetBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = &ConditionalBreakpoint{Address: addr}
	return true
}

func (d *Debug6502) SetConditionalBreakpoint(addr uint64, cond *BreakpointCondition) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = &ConditionalBreakpoint{Address: addr, Condition: cond}
	return true
}

func (d *Debug6502) ClearBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.breakpoints[addr]; ok {
		delete(d.breakpoints, addr)
		return true
	}
	return false
}

func (d *Debug6502) ClearAllBreakpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints = make(map[uint64]*ConditionalBreakpoint)
}

func (d *Debug6502) ListBreakpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.breakpoints))
	for addr := range d.breakpoints {
		result = append(result, addr)
	}
	return result
}

func (d *Debug6502) ListConditionalBreakpoints() []*ConditionalBreakpoint {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]*ConditionalBreakpoint, 0, len(d.breakpoints))
	for _, bp := range d.breakpoints {
		result = append(result, bp)
	}
	return result
}

func (d *Debug6502) HasBreakpoint(addr uint64) bool {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	_, ok := d.breakpoints[addr]
	return ok
}

func (d *Debug6502) GetConditionalBreakpoint(addr uint64) *ConditionalBreakpoint {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	return d.breakpoints[addr]
}

func (d *Debug6502) SetWatchpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	val := d.cpu.memory.Read(uint16(addr))
	d.watchpoints[addr] = &Watchpoint{Address: addr, LastValue: val}
	return true
}

func (d *Debug6502) ClearWatchpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.watchpoints[addr]; ok {
		delete(d.watchpoints, addr)
		return true
	}
	return false
}

func (d *Debug6502) ClearAllWatchpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.watchpoints = make(map[uint64]*Watchpoint)
}

func (d *Debug6502) ListWatchpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.watchpoints))
	for addr := range d.watchpoints {
		result = append(result, addr)
	}
	return result
}

func (d *Debug6502) ReadMemory(addr uint64, size int) []byte {
	result := make([]byte, size)
	for i := range size {
		result[i] = d.cpu.memory.Read(uint16(addr) + uint16(i))
	}
	return result
}

func (d *Debug6502) WriteMemory(addr uint64, data []byte) {
	for i, b := range data {
		d.cpu.memory.Write(uint16(addr)+uint16(i), b)
	}
}

func (d *Debug6502) SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int) {
	d.bpChan = ch
	d.cpuID = cpuID
}
