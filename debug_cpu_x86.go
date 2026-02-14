// debug_cpu_x86.go - X86 debug adapter for Machine Monitor

package main

import (
	"strings"
	"sync"
	"sync/atomic"
)

type DebugX86 struct {
	cpu    *CPU_X86
	runner *CPUX86Runner

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

func NewDebugX86(cpu *CPU_X86, runner *CPUX86Runner) *DebugX86 {
	return &DebugX86{
		cpu:         cpu,
		runner:      runner,
		breakpoints: make(map[uint64]*ConditionalBreakpoint),
		watchpoints: make(map[uint64]*Watchpoint),
	}
}

func (d *DebugX86) CPUName() string   { return "X86" }
func (d *DebugX86) AddressWidth() int { return 32 }

func (d *DebugX86) GetRegisters() []RegisterInfo {
	c := d.cpu
	return []RegisterInfo{
		{Name: "EAX", BitWidth: 32, Value: uint64(c.EAX), Group: "general"},
		{Name: "EBX", BitWidth: 32, Value: uint64(c.EBX), Group: "general"},
		{Name: "ECX", BitWidth: 32, Value: uint64(c.ECX), Group: "general"},
		{Name: "EDX", BitWidth: 32, Value: uint64(c.EDX), Group: "general"},
		{Name: "ESI", BitWidth: 32, Value: uint64(c.ESI), Group: "general"},
		{Name: "EDI", BitWidth: 32, Value: uint64(c.EDI), Group: "general"},
		{Name: "EBP", BitWidth: 32, Value: uint64(c.EBP), Group: "general"},
		{Name: "ESP", BitWidth: 32, Value: uint64(c.ESP), Group: "general"},
		{Name: "EIP", BitWidth: 32, Value: uint64(c.EIP), Group: "general"},
		{Name: "EFLAGS", BitWidth: 32, Value: uint64(c.Flags), Group: "flags"},
		{Name: "CS", BitWidth: 16, Value: uint64(c.CS), Group: "segment"},
		{Name: "DS", BitWidth: 16, Value: uint64(c.DS), Group: "segment"},
		{Name: "ES", BitWidth: 16, Value: uint64(c.ES), Group: "segment"},
		{Name: "SS", BitWidth: 16, Value: uint64(c.SS), Group: "segment"},
		{Name: "FS", BitWidth: 16, Value: uint64(c.FS), Group: "segment"},
		{Name: "GS", BitWidth: 16, Value: uint64(c.GS), Group: "segment"},
	}
}

func (d *DebugX86) GetRegister(name string) (uint64, bool) {
	c := d.cpu
	switch strings.ToUpper(name) {
	case "EAX":
		return uint64(c.EAX), true
	case "EBX":
		return uint64(c.EBX), true
	case "ECX":
		return uint64(c.ECX), true
	case "EDX":
		return uint64(c.EDX), true
	case "ESI":
		return uint64(c.ESI), true
	case "EDI":
		return uint64(c.EDI), true
	case "EBP":
		return uint64(c.EBP), true
	case "ESP":
		return uint64(c.ESP), true
	case "EIP":
		return uint64(c.EIP), true
	case "FLAGS", "EFLAGS":
		return uint64(c.Flags), true
	case "CS":
		return uint64(c.CS), true
	case "DS":
		return uint64(c.DS), true
	case "ES":
		return uint64(c.ES), true
	case "SS":
		return uint64(c.SS), true
	case "FS":
		return uint64(c.FS), true
	case "GS":
		return uint64(c.GS), true
	}
	return 0, false
}

func (d *DebugX86) SetRegister(name string, value uint64) bool {
	c := d.cpu
	switch strings.ToUpper(name) {
	case "EAX":
		c.EAX = uint32(value)
	case "EBX":
		c.EBX = uint32(value)
	case "ECX":
		c.ECX = uint32(value)
	case "EDX":
		c.EDX = uint32(value)
	case "ESI":
		c.ESI = uint32(value)
	case "EDI":
		c.EDI = uint32(value)
	case "EBP":
		c.EBP = uint32(value)
	case "ESP":
		c.ESP = uint32(value)
	case "EIP":
		c.EIP = uint32(value)
	case "FLAGS", "EFLAGS":
		c.Flags = uint32(value)
	case "CS":
		c.CS = uint16(value)
	case "DS":
		c.DS = uint16(value)
	case "ES":
		c.ES = uint16(value)
	case "SS":
		c.SS = uint16(value)
	case "FS":
		c.FS = uint16(value)
	case "GS":
		c.GS = uint16(value)
	default:
		return false
	}
	return true
}

func (d *DebugX86) GetPC() uint64     { return uint64(d.cpu.EIP) }
func (d *DebugX86) SetPC(addr uint64) { d.cpu.EIP = uint32(addr) }

func (d *DebugX86) IsRunning() bool {
	return d.cpu.Running() || d.trapRunning.Load()
}

func (d *DebugX86) Freeze() {
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

func (d *DebugX86) Resume() {
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

func (d *DebugX86) trapLoop() {
	defer d.trapRunning.Store(false)
	d.cpu.SetRunning(true)
	d.cpu.Halted = false
	for {
		select {
		case <-d.trapStop:
			d.cpu.SetRunning(false)
			return
		default:
		}
		d.bpMu.RLock()
		bp := d.breakpoints[uint64(d.cpu.EIP)]
		d.bpMu.RUnlock()
		if bp != nil {
			bp.HitCount++
			if evaluateConditionWithHitCount(bp.Condition, d, bp.HitCount) {
				d.cpu.SetRunning(false)
				if d.bpChan != nil {
					select {
					case d.bpChan <- BreakpointEvent{CPUID: d.cpuID, Address: uint64(d.cpu.EIP)}:
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
			cur := d.cpu.bus.Read(uint32(wp.Address))
			if cur != wp.LastValue {
				old := wp.LastValue
				wp.LastValue = cur
				d.bpMu.RUnlock()
				d.cpu.SetRunning(false)
				if d.bpChan != nil {
					select {
					case d.bpChan <- BreakpointEvent{
						CPUID: d.cpuID, Address: uint64(d.cpu.EIP),
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

func (d *DebugX86) Step() int {
	return d.cpu.Step()
}

func (d *DebugX86) Disassemble(addr uint64, count int) []DisassembledLine {
	pc := uint64(d.cpu.EIP)
	lines := disassembleX86(d.ReadMemory, addr, count)
	for i := range lines {
		if lines[i].Address == pc {
			lines[i].IsPC = true
		}
	}
	return lines
}

func (d *DebugX86) SetBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = &ConditionalBreakpoint{Address: addr}
	return true
}

func (d *DebugX86) SetConditionalBreakpoint(addr uint64, cond *BreakpointCondition) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = &ConditionalBreakpoint{Address: addr, Condition: cond}
	return true
}

func (d *DebugX86) ClearBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.breakpoints[addr]; ok {
		delete(d.breakpoints, addr)
		return true
	}
	return false
}

func (d *DebugX86) ClearAllBreakpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints = make(map[uint64]*ConditionalBreakpoint)
}

func (d *DebugX86) ListBreakpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.breakpoints))
	for addr := range d.breakpoints {
		result = append(result, addr)
	}
	return result
}

func (d *DebugX86) ListConditionalBreakpoints() []*ConditionalBreakpoint {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]*ConditionalBreakpoint, 0, len(d.breakpoints))
	for _, bp := range d.breakpoints {
		result = append(result, bp)
	}
	return result
}

func (d *DebugX86) HasBreakpoint(addr uint64) bool {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	_, ok := d.breakpoints[addr]
	return ok
}

func (d *DebugX86) GetConditionalBreakpoint(addr uint64) *ConditionalBreakpoint {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	return d.breakpoints[addr]
}

func (d *DebugX86) SetWatchpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	val := d.cpu.bus.Read(uint32(addr))
	d.watchpoints[addr] = &Watchpoint{Address: addr, LastValue: val}
	return true
}

func (d *DebugX86) ClearWatchpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.watchpoints[addr]; ok {
		delete(d.watchpoints, addr)
		return true
	}
	return false
}

func (d *DebugX86) ClearAllWatchpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.watchpoints = make(map[uint64]*Watchpoint)
}

func (d *DebugX86) ListWatchpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.watchpoints))
	for addr := range d.watchpoints {
		result = append(result, addr)
	}
	return result
}

func (d *DebugX86) ReadMemory(addr uint64, size int) []byte {
	result := make([]byte, size)
	for i := range size {
		result[i] = d.cpu.bus.Read(uint32(addr) + uint32(i))
	}
	return result
}

func (d *DebugX86) WriteMemory(addr uint64, data []byte) {
	for i, b := range data {
		d.cpu.bus.Write(uint32(addr)+uint32(i), b)
	}
}

func (d *DebugX86) SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int) {
	d.bpChan = ch
	d.cpuID = cpuID
}
