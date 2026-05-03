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
	events      *adapterEventSink
	trapRunning atomic.Bool
	trapStop    chan struct{}
	frozenCh    chan struct{}

	workerFreeze func()
	workerResume func()
}

func NewDebugM68K(cpu *M68KCPU, runner *M68KRunner) *DebugM68K {
	return &DebugM68K{
		cpu:         cpu,
		runner:      runner,
		breakpoints: make(map[uint64]*ConditionalBreakpoint),
		watchpoints: make(map[uint64]*Watchpoint),
		events:      newAdapterEventSink(),
	}
}

func (d *DebugM68K) CPUName() string   { return "M68K" }
func (d *DebugM68K) AddressWidth() int { return 32 }

func (d *DebugM68K) GetRegisters() []RegisterInfo {
	c := d.cpu
	regs := make([]RegisterInfo, 0, 20)
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
	regs = append(regs, RegisterInfo{Name: "SSP", BitWidth: 32, Value: uint64(c.SSP), Group: "general"})
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
		if d.frozenCh != nil {
			<-d.frozenCh
		}
		return
	}
	if d.workerFreeze != nil {
		d.workerFreeze()
		return
	}
	if d.runner != nil {
		d.runner.Stop()
	}
}

func (d *DebugM68K) Resume() {
	d.bpMu.Lock()
	hasBP := len(d.breakpoints) > 0 || len(d.watchpoints) > 0
	if hasBP {
		d.trapStop = make(chan struct{})
		d.frozenCh = make(chan struct{})
		d.trapRunning.Store(true)
		go d.trapLoop()
		d.bpMu.Unlock()
		return
	}
	d.bpMu.Unlock()
	if d.workerResume != nil {
		d.workerResume()
		return
	}
	if d.runner != nil {
		d.runner.StartExecution()
	}
}

func (d *DebugM68K) trapLoop() {
	frozenCh := d.frozenCh
	defer d.trapRunning.Store(false)
	defer d.cpu.SetRunning(false)
	defer close(frozenCh)
	d.cpu.SetRunning(true)
	for {
		select {
		case <-d.trapStop:
			return
		default:
		}
		if bp, ok := d.SnapshotBreakpoint(uint64(d.cpu.PC)); ok {
			hitCount, _ := d.IncrementBreakpointHit(uint64(d.cpu.PC))
			if evaluateConditionWithHitCount(bp.Condition, d, hitCount) {
				d.events.Publish(BreakpointEvent{Address: uint64(d.cpu.PC)})
				return
			}
		}
		if d.cpu.StepOne() == 0 {
			return
		}
		// Check watchpoints
		d.bpMu.RLock()
		for _, wp := range d.watchpoints {
			cur := d.readByte(wp.Address)
			if cur != wp.LastValue {
				old := wp.LastValue
				addr := wp.Address
				d.bpMu.RUnlock()
				d.UpdateWatchpointLastValue(addr, cur)
				d.events.Publish(BreakpointEvent{
					Address: uint64(d.cpu.PC), IsWatch: true, WatchAddr: addr,
					WatchOldValue: old, WatchNewValue: cur,
				})
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
		cp := cloneBreakpoint(bp)
		result = append(result, &cp)
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
	bp, ok := d.SnapshotBreakpoint(addr)
	if !ok {
		return nil
	}
	return &bp
}

func (d *DebugM68K) SnapshotBreakpoint(addr uint64) (BreakpointSnapshot, bool) {
	return snapshotBreakpointLocked(&d.bpMu, d.breakpoints, addr)
}

func (d *DebugM68K) IncrementBreakpointHit(addr uint64) (uint64, bool) {
	return incrementBreakpointHitLocked(&d.bpMu, d.breakpoints, addr)
}

func (d *DebugM68K) SetBreakpointCondition(addr uint64, cond *BreakpointCondition) bool {
	return setBreakpointConditionLocked(&d.bpMu, d.breakpoints, addr, cond)
}

func (d *DebugM68K) ListBreakpointSnapshots() []BreakpointSnapshot {
	return listBreakpointSnapshotsLocked(&d.bpMu, d.breakpoints)
}

func (d *DebugM68K) SetWatchpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.watchpoints[addr] = &Watchpoint{Address: addr, LastValue: d.readByte(addr)}
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

func (d *DebugM68K) SnapshotWatchpoint(addr uint64) (WatchpointSnapshot, bool) {
	return snapshotWatchpointLocked(&d.bpMu, d.watchpoints, addr)
}

func (d *DebugM68K) UpdateWatchpointLastValue(addr uint64, val byte) bool {
	return updateWatchpointLastValueLocked(&d.bpMu, d.watchpoints, addr, val)
}

func (d *DebugM68K) ListWatchpointSnapshots() []WatchpointSnapshot {
	return listWatchpointSnapshotsLocked(&d.bpMu, d.watchpoints)
}

func (d *DebugM68K) ValidateAddress(addr uint64) error {
	return validateAddressWidth(d.CPUName(), d.AddressWidth(), addr)
}

func (d *DebugM68K) readByte(addr uint64) byte {
	if d.cpu.bus != nil {
		return d.cpu.bus.Read8(uint32(addr))
	}
	if addr <= 0xFFFFFFFF {
		a := uint32(addr)
		if int(a) < len(d.cpu.memory) {
			return d.cpu.memory[a]
		}
	}
	return 0
}

func (d *DebugM68K) ReadMemory(addr uint64, size int) []byte {
	if size <= 0 {
		return nil
	}
	mem := d.cpu.memory
	if addr <= 0xFFFFFFFF && addr+uint64(size) <= uint64(len(mem)) && !d.memoryRangeHasIO(addr, size) {
		start := uint32(addr)
		return append([]byte{}, mem[start:int(start)+size]...)
	}
	out := make([]byte, size)
	if d.cpu.bus != nil {
		for i := range out {
			out[i] = d.cpu.bus.Read8(uint32(addr + uint64(i)))
		}
		return out
	}
	start := uint32(addr)
	if int(start) >= len(mem) {
		return nil
	}
	return append([]byte{}, mem[start:min(int(start)+size, len(mem))]...)
}

func (d *DebugM68K) WriteMemory(addr uint64, data []byte) {
	mem := d.cpu.memory
	if addr <= 0xFFFFFFFF && addr+uint64(len(data)) <= uint64(len(mem)) && !d.memoryRangeHasIO(addr, len(data)) {
		copy(mem[uint32(addr):], data)
		return
	}
	if d.cpu.bus != nil {
		for i, b := range data {
			d.cpu.bus.Write8(uint32(addr+uint64(i)), b)
		}
		return
	}
	start := uint32(addr)
	if int(start)+len(data) > len(mem) {
		return
	}
	copy(mem[start:], data)
}

func (d *DebugM68K) memoryRangeHasIO(addr uint64, size int) bool {
	bus, ok := d.cpu.bus.(*MachineBus)
	if !ok || size <= 0 || addr > 0xFFFFFFFF {
		return false
	}
	start := uint32(addr)
	end64 := addr + uint64(size) - 1
	if end64 > 0xFFFFFFFF {
		end64 = 0xFFFFFFFF
	}
	end := uint32(end64)
	for page := start & PAGE_MASK; ; page += PAGE_SIZE {
		for _, region := range bus.mapping[page] {
			if start <= region.end && end >= region.start {
				return true
			}
		}
		if page >= end&PAGE_MASK {
			break
		}
	}
	return false
}

func (d *DebugM68K) SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int) {
	d.events.Set(ch, cpuID)
}
