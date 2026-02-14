// debug_interface.go - DebuggableCPU interface and supporting types for Machine Monitor

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

// RegisterInfo describes a single CPU register for display in the monitor.
type RegisterInfo struct {
	Name     string // "PC", "D0", "R14"
	BitWidth int    // 8, 16, 32, or 64
	Value    uint64
	Group    string // "general", "index", "status", "shadow", "flags"
}

// DisassembledLine represents one disassembled instruction.
type DisassembledLine struct {
	Address  uint64
	HexBytes string
	Mnemonic string
	Size     int
	IsPC     bool // true if this is the current PC
}

// BreakpointEvent is published when a CPU hits a breakpoint during execution.
type BreakpointEvent struct {
	CPUID   int    // Stable CPU ID that hit the breakpoint
	Address uint64 // Address of the breakpoint
}

// DebuggableCPU is the interface that all CPU debug adapters must implement.
type DebuggableCPU interface {
	CPUName() string
	AddressWidth() int // 16, 24, 32, or 64

	GetRegisters() []RegisterInfo
	GetRegister(name string) (uint64, bool)
	SetRegister(name string, value uint64) bool
	GetPC() uint64
	SetPC(addr uint64)

	IsRunning() bool
	Freeze() // Stop execution, preserve state
	Resume() // Restart execution goroutine

	Step() int // Execute one instruction, return cycles

	Disassemble(addr uint64, count int) []DisassembledLine

	SetBreakpoint(addr uint64) bool
	ClearBreakpoint(addr uint64) bool
	ClearAllBreakpoints()
	ListBreakpoints() []uint64
	HasBreakpoint(addr uint64) bool

	ReadMemory(addr uint64, size int) []byte
	WriteMemory(addr uint64, data []byte)

	SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int)
}

// MonitorAttachable is implemented by video outputs that support the machine monitor overlay.
type MonitorAttachable interface {
	AttachMonitor(monitor *MachineMonitor)
}
