// debug_cpu_m68k.go - M68K debug adapter for Machine Monitor

package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

type DebugM68K struct {
	cpu    *M68KCPU
	runner *M68KRunner

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

func NewDebugM68K(cpu *M68KCPU, runner *M68KRunner) *DebugM68K {
	return &DebugM68K{
		cpu:         cpu,
		runner:      runner,
		breakpoints: make(map[uint64]*ConditionalBreakpoint),
		watchpoints: make(map[uint64]*Watchpoint),
	}
}

func (d *DebugM68K) CPUName() string   { return "M68K" }
func (d *DebugM68K) AddressWidth() int { return 32 }

func (d *DebugM68K) GetRegisters() []RegisterInfo {
	c := d.cpu
	regs := make([]RegisterInfo, 0, 19)
	for i := range 8 {
		regs = append(regs, RegisterInfo{
			Name: fmt.Sprintf("D%d", i), BitWidth: 32,
			Value: uint64(c.DataRegs[i]), Group: "general",
		})
	}
	for i := range 8 {
		regs = append(regs, RegisterInfo{
			Name: fmt.Sprintf("A%d", i), BitWidth: 32,
			Value: uint64(c.AddrRegs[i]), Group: "general",
		})
	}
	regs = append(regs, RegisterInfo{Name: "PC", BitWidth: 32, Value: uint64(c.PC), Group: "general"})
	regs = append(regs, RegisterInfo{Name: "SR", BitWidth: 16, Value: uint64(c.SR), Group: "flags"})
	regs = append(regs, RegisterInfo{Name: "USP", BitWidth: 32, Value: uint64(c.USP), Group: "general"})
	return regs
}

func (d *DebugM68K) GetRegister(name string) (uint64, bool) {
	c := d.cpu
	upper := strings.ToUpper(name)
	switch {
	case upper == "PC":
		return uint64(c.PC), true
	case upper == "SR":
		return uint64(c.SR), true
	case upper == "USP":
		return uint64(c.USP), true
	case upper == "SSP":
		return uint64(c.SSP), true
	case len(upper) == 2 && upper[0] == 'D' && upper[1] >= '0' && upper[1] <= '7':
		return uint64(c.DataRegs[upper[1]-'0']), true
	case len(upper) == 2 && upper[0] == 'A' && upper[1] >= '0' && upper[1] <= '7':
		return uint64(c.AddrRegs[upper[1]-'0']), true
	}
	return 0, false
}

func (d *DebugM68K) SetRegister(name string, value uint64) bool {
	c := d.cpu
	v := uint32(value)
	upper := strings.ToUpper(name)
	switch {
	case upper == "PC":
		c.PC = v
	case upper == "SR":
		c.SR = uint16(value)
	case upper == "USP":
		c.USP = v
	case upper == "SSP":
		c.SSP = v
	case len(upper) == 2 && upper[0] == 'D' && upper[1] >= '0' && upper[1] <= '7':
		c.DataRegs[upper[1]-'0'] = v
	case len(upper) == 2 && upper[0] == 'A' && upper[1] >= '0' && upper[1] <= '7':
		c.AddrRegs[upper[1]-'0'] = v
	default:
		return false
	}
	return true
}

func (d *DebugM68K) GetPC() uint64     { return uint64(d.cpu.PC) }
func (d *DebugM68K) SetPC(addr uint64) { d.cpu.PC = uint32(addr) }

func (d *DebugM68K) IsRunning() bool {
	return d.cpu.Running() || d.trapRunning.Load()
}

func (d *DebugM68K) Freeze() {
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

func (d *DebugM68K) Resume() {
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

func (d *DebugM68K) trapLoop() {
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
		bp := d.breakpoints[uint64(d.cpu.PC)]
		d.bpMu.RUnlock()
		if bp != nil {
			bp.HitCount++
			if evaluateConditionWithHitCount(bp.Condition, d, bp.HitCount) {
				if d.bpChan != nil {
					select {
					case d.bpChan <- BreakpointEvent{CPUID: d.cpuID, Address: uint64(d.cpu.PC)}:
					default:
					}
				}
				return
			}
		}
		if d.cpu.StepOne() == 0 {
			return
		}
		// Check watchpoints
		d.bpMu.RLock()
		for _, wp := range d.watchpoints {
			var cur byte
			if int(uint32(wp.Address)) < len(d.cpu.memory) {
				cur = d.cpu.memory[uint32(wp.Address)]
			}
			if cur != wp.LastValue {
				old := wp.LastValue
				wp.LastValue = cur
				d.bpMu.RUnlock()
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

func (d *DebugM68K) Step() int { return d.cpu.StepOne() }

func (d *DebugM68K) Disassemble(addr uint64, count int) []DisassembledLine {
	pc := uint64(d.cpu.PC)
	lines := disassembleM68K(d.ReadMemory, addr, count)
	for i := range lines {
		if lines[i].Address == pc {
			lines[i].IsPC = true
		}
	}
	return lines
}

func (d *DebugM68K) SetBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = &ConditionalBreakpoint{Address: addr}
	return true
}

func (d *DebugM68K) SetConditionalBreakpoint(addr uint64, cond *BreakpointCondition) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = &ConditionalBreakpoint{Address: addr, Condition: cond}
	return true
}

func (d *DebugM68K) ClearBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.breakpoints[addr]; ok {
		delete(d.breakpoints, addr)
		return true
	}
	return false
}

func (d *DebugM68K) ClearAllBreakpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints = make(map[uint64]*ConditionalBreakpoint)
}

func (d *DebugM68K) ListBreakpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.breakpoints))
	for addr := range d.breakpoints {
		result = append(result, addr)
	}
	return result
}

func (d *DebugM68K) ListConditionalBreakpoints() []*ConditionalBreakpoint {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]*ConditionalBreakpoint, 0, len(d.breakpoints))
	for _, bp := range d.breakpoints {
		result = append(result, bp)
	}
	return result
}

func (d *DebugM68K) HasBreakpoint(addr uint64) bool {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	_, ok := d.breakpoints[addr]
	return ok
}

func (d *DebugM68K) GetConditionalBreakpoint(addr uint64) *ConditionalBreakpoint {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	return d.breakpoints[addr]
}

func (d *DebugM68K) SetWatchpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	var val byte
	if int(uint32(addr)) < len(d.cpu.memory) {
		val = d.cpu.memory[uint32(addr)]
	}
	d.watchpoints[addr] = &Watchpoint{Address: addr, LastValue: val}
	return true
}

func (d *DebugM68K) ClearWatchpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.watchpoints[addr]; ok {
		delete(d.watchpoints, addr)
		return true
	}
	return false
}

func (d *DebugM68K) ClearAllWatchpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.watchpoints = make(map[uint64]*Watchpoint)
}

func (d *DebugM68K) ListWatchpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.watchpoints))
	for addr := range d.watchpoints {
		result = append(result, addr)
	}
	return result
}

func (d *DebugM68K) ReadMemory(addr uint64, size int) []byte {
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

func (d *DebugM68K) WriteMemory(addr uint64, data []byte) {
	mem := d.cpu.memory
	start := uint32(addr)
	if int(start)+len(data) > len(mem) {
		return
	}
	copy(mem[start:], data)
}

func (d *DebugM68K) SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int) {
	d.bpChan = ch
	d.cpuID = cpuID
}
