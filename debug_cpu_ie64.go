// debug_cpu_ie64.go - IE64 debug adapter for Machine Monitor

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// DebugIE64 wraps a CPU64 to implement DebuggableCPU.
type DebugIE64 struct {
	cpu *CPU64

	bpMu        sync.RWMutex
	breakpoints map[uint64]*ConditionalBreakpoint
	watchpoints map[uint64]*Watchpoint

	bpChan chan<- BreakpointEvent
	cpuID  int

	trapRunning atomic.Bool
	trapStop    chan struct{}

	workerFreeze func()
	workerResume func()
}

func NewDebugIE64(cpu *CPU64) *DebugIE64 {
	return &DebugIE64{
		cpu:         cpu,
		breakpoints: make(map[uint64]*ConditionalBreakpoint),
		watchpoints: make(map[uint64]*Watchpoint),
	}
}

func (d *DebugIE64) CPUName() string   { return "IE64" }
func (d *DebugIE64) AddressWidth() int { return 32 }

func (d *DebugIE64) GetRegisters() []RegisterInfo {
	regs := make([]RegisterInfo, 0, 33)
	for i := range 31 {
		regs = append(regs, RegisterInfo{
			Name:     fmt.Sprintf("R%d", i),
			BitWidth: 64,
			Value:    d.cpu.regs[i],
			Group:    "general",
		})
	}
	regs = append(regs, RegisterInfo{
		Name:     "SP",
		BitWidth: 64,
		Value:    d.cpu.regs[31],
		Group:    "general",
	})
	regs = append(regs, RegisterInfo{
		Name:     "PC",
		BitWidth: 64,
		Value:    d.cpu.PC,
		Group:    "general",
	})
	return regs
}

func (d *DebugIE64) GetRegister(name string) (uint64, bool) {
	upper := strings.ToUpper(name)
	if upper == "PC" {
		return d.cpu.PC, true
	}
	if upper == "SP" {
		return d.cpu.regs[31], true
	}
	if len(upper) >= 2 && upper[0] == 'R' {
		var idx int
		if _, err := fmt.Sscanf(upper, "R%d", &idx); err == nil && idx >= 0 && idx <= 31 {
			return d.cpu.regs[idx], true
		}
	}
	return 0, false
}

func (d *DebugIE64) SetRegister(name string, value uint64) bool {
	upper := strings.ToUpper(name)
	if upper == "PC" {
		d.cpu.PC = value
		return true
	}
	if upper == "SP" {
		d.cpu.regs[31] = value
		return true
	}
	if len(upper) >= 2 && upper[0] == 'R' {
		var idx int
		if _, err := fmt.Sscanf(upper, "R%d", &idx); err == nil && idx >= 0 && idx <= 31 {
			if idx == 0 {
				return false // R0 is hardwired to zero
			}
			d.cpu.regs[idx] = value
			return true
		}
	}
	return false
}

func (d *DebugIE64) GetPC() uint64     { return d.cpu.PC }
func (d *DebugIE64) SetPC(addr uint64) { d.cpu.PC = addr }

func (d *DebugIE64) IsRunning() bool {
	return d.cpu.IsRunning() || d.trapRunning.Load()
}

func (d *DebugIE64) Freeze() {
	if d.trapRunning.Load() {
		close(d.trapStop)
		for d.trapRunning.Load() {
			// spin until trap loop exits
		}
		return
	}
	if d.workerFreeze != nil {
		d.workerFreeze()
		return
	}
	d.cpu.Stop()
}

func (d *DebugIE64) Resume() {
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
	d.cpu.running.Store(true)
	d.cpu.StartExecution()
}

func (d *DebugIE64) trapLoop() {
	defer d.trapRunning.Store(false)
	defer d.cpu.running.Store(false)
	d.cpu.running.Store(true)
	for {
		select {
		case <-d.trapStop:
			return
		default:
		}

		d.bpMu.RLock()
		bp := d.breakpoints[d.cpu.PC]
		d.bpMu.RUnlock()
		if bp != nil {
			bp.HitCount++
			if evaluateConditionWithHitCount(bp.Condition, d, bp.HitCount) {
				if d.bpChan != nil {
					select {
					case d.bpChan <- BreakpointEvent{CPUID: d.cpuID, Address: d.cpu.PC}:
					default:
					}
				}
				return
			}
		}

		if d.cpu.StepOne() == 0 {
			return
		}

		// Check watchpoints. PLAN_MAX_RAM.md slice 3 widened IE64 to
		// 64-bit physical addressing; route reads through the bus so a
		// watchpoint set above the legacy 32-bit window is observed in
		// the bound Backing rather than aliasing into low memory.
		d.bpMu.RLock()
		for _, wp := range d.watchpoints {
			cur := d.readByte(wp.Address)
			if cur != wp.LastValue {
				old := wp.LastValue
				wp.LastValue = cur
				d.bpMu.RUnlock()
				if d.bpChan != nil {
					select {
					case d.bpChan <- BreakpointEvent{
						CPUID: d.cpuID, Address: d.cpu.PC,
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

func (d *DebugIE64) Step() int {
	return d.cpu.StepOne()
}

func (d *DebugIE64) Disassemble(addr uint64, count int) []DisassembledLine {
	pc := d.cpu.PC
	lines := disassembleIE64(d.ReadMemory, addr, count)
	for i := range lines {
		if lines[i].Address == pc {
			lines[i].IsPC = true
		}
	}
	return lines
}

func (d *DebugIE64) SetBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = &ConditionalBreakpoint{Address: addr}
	return true
}

func (d *DebugIE64) SetConditionalBreakpoint(addr uint64, cond *BreakpointCondition) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints[addr] = &ConditionalBreakpoint{Address: addr, Condition: cond}
	return true
}

func (d *DebugIE64) ClearBreakpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.breakpoints[addr]; ok {
		delete(d.breakpoints, addr)
		return true
	}
	return false
}

func (d *DebugIE64) ClearAllBreakpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.breakpoints = make(map[uint64]*ConditionalBreakpoint)
}

func (d *DebugIE64) ListBreakpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.breakpoints))
	for addr := range d.breakpoints {
		result = append(result, addr)
	}
	return result
}

func (d *DebugIE64) ListConditionalBreakpoints() []*ConditionalBreakpoint {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]*ConditionalBreakpoint, 0, len(d.breakpoints))
	for _, bp := range d.breakpoints {
		result = append(result, bp)
	}
	return result
}

func (d *DebugIE64) HasBreakpoint(addr uint64) bool {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	_, ok := d.breakpoints[addr]
	return ok
}

func (d *DebugIE64) GetConditionalBreakpoint(addr uint64) *ConditionalBreakpoint {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	return d.breakpoints[addr]
}

func (d *DebugIE64) SetWatchpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.watchpoints[addr] = &Watchpoint{Address: addr, LastValue: d.readByte(addr)}
	return true
}

// readByte fetches one byte at the full 64-bit physical address. Routes
// through the bus so high-address Backing reads work; falls back to the
// legacy bus.memory slice when the address lies in the low window.
func (d *DebugIE64) readByte(addr uint64) byte {
	if addr <= 0xFFFFFFFF {
		if a := uint32(addr); int(a) < len(d.cpu.memory) {
			return d.cpu.memory[a]
		}
	}
	if d.cpu.bus == nil {
		return 0
	}
	return d.cpu.bus.ReadPhys8(addr)
}

func (d *DebugIE64) ClearWatchpoint(addr uint64) bool {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	if _, ok := d.watchpoints[addr]; ok {
		delete(d.watchpoints, addr)
		return true
	}
	return false
}

func (d *DebugIE64) ClearAllWatchpoints() {
	d.bpMu.Lock()
	defer d.bpMu.Unlock()
	d.watchpoints = make(map[uint64]*Watchpoint)
}

func (d *DebugIE64) ListWatchpoints() []uint64 {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	result := make([]uint64, 0, len(d.watchpoints))
	for addr := range d.watchpoints {
		result = append(result, addr)
	}
	return result
}

func (d *DebugIE64) ReadMemory(addr uint64, size int) []byte {
	if size <= 0 {
		return nil
	}
	// Fast path: span fits inside the legacy bus.memory window.
	if addr <= 0xFFFFFFFF && addr+uint64(size) <= uint64(len(d.cpu.memory)) {
		start := uint32(addr)
		return append([]byte{}, d.cpu.memory[start:int(start)+size]...)
	}
	// Slow path: route per-byte through the bus so high-address Backing
	// reads work. Preserves the full 64-bit address (PLAN_MAX_RAM.md
	// slice 3 retired the IE64_ADDR_MASK 25-bit truncation).
	out := make([]byte, size)
	for i := 0; i < size; i++ {
		out[i] = d.readByte(addr + uint64(i))
	}
	return out
}

func (d *DebugIE64) WriteMemory(addr uint64, data []byte) {
	// Fast path: span fits inside the legacy bus.memory window.
	if addr <= 0xFFFFFFFF && addr+uint64(len(data)) <= uint64(len(d.cpu.memory)) {
		copy(d.cpu.memory[uint32(addr):], data)
		return
	}
	// Slow path: route per-byte through the bus.
	if d.cpu.bus == nil {
		return
	}
	for i, b := range data {
		d.cpu.bus.WritePhys8(addr+uint64(i), b)
	}
}

func (d *DebugIE64) SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int) {
	d.bpChan = ch
	d.cpuID = cpuID
}
