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
	breakpoints map[uint64]bool

	bpChan chan<- BreakpointEvent
	cpuID  int

	trapRunning atomic.Bool
	trapStop    chan struct{}
}

func NewDebugIE64(cpu *CPU64) *DebugIE64 {
	return &DebugIE64{
		cpu:         cpu,
		breakpoints: make(map[uint64]bool),
	}
}

func (d *DebugIE64) CPUName() string   { return "IE64" }
func (d *DebugIE64) AddressWidth() int { return 32 }

func (d *DebugIE64) GetRegisters() []RegisterInfo {
	regs := make([]RegisterInfo, 0, 33)
	for i := 0; i < 31; i++ {
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
	d.cpu.Stop()
}

func (d *DebugIE64) Resume() {
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

func (d *DebugIE64) trapLoop() {
	defer d.trapRunning.Store(false)
	for {
		select {
		case <-d.trapStop:
			return
		default:
		}

		d.bpMu.RLock()
		if len(d.breakpoints) > 0 && d.breakpoints[d.cpu.PC] {
			d.bpMu.RUnlock()
			if d.bpChan != nil {
				select {
				case d.bpChan <- BreakpointEvent{CPUID: d.cpuID, Address: d.cpu.PC}:
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
	d.breakpoints[addr] = true
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
	d.breakpoints = make(map[uint64]bool)
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

func (d *DebugIE64) HasBreakpoint(addr uint64) bool {
	d.bpMu.RLock()
	defer d.bpMu.RUnlock()
	return d.breakpoints[addr]
}

func (d *DebugIE64) ReadMemory(addr uint64, size int) []byte {
	mem := d.cpu.memory
	start := uint32(addr & IE64_ADDR_MASK)
	if int(start)+size > len(mem) {
		end := len(mem)
		if int(start) >= end {
			return nil
		}
		return append([]byte{}, mem[start:end]...)
	}
	return append([]byte{}, mem[start:int(start)+size]...)
}

func (d *DebugIE64) WriteMemory(addr uint64, data []byte) {
	mem := d.cpu.memory
	start := uint32(addr & IE64_ADDR_MASK)
	if int(start)+len(data) > len(mem) {
		return
	}
	copy(mem[start:], data)
}

func (d *DebugIE64) SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int) {
	d.bpChan = ch
	d.cpuID = cpuID
}
