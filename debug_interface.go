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
	Address      uint64
	HexBytes     string
	Mnemonic     string
	Size         int
	IsPC         bool   // true if this is the current PC
	IsBranch     bool   // true if this is a branch/jump instruction
	BranchTarget uint64 // target address for branches (0 if unknown/register-indirect)
}

// BreakpointEvent is published when a CPU hits a breakpoint or watchpoint during execution.
type BreakpointEvent struct {
	CPUID   int    // Stable CPU ID that hit the breakpoint
	Address uint64 // Address of the breakpoint

	// Watchpoint fields (zero values when this is a plain breakpoint)
	IsWatch       bool   // true if this is a watchpoint hit
	WatchAddr     uint64 // watched memory address
	WatchOldValue byte   // previous value
	WatchNewValue byte   // new value
}

// ConditionOp defines the comparison operator for breakpoint conditions.
type ConditionOp int

const (
	CondOpEqual ConditionOp = iota
	CondOpNotEqual
	CondOpLess
	CondOpGreater
	CondOpLessEqual
	CondOpGreaterEqual
)

// ConditionSource defines what is being compared in a breakpoint condition.
type ConditionSource int

const (
	CondSourceRegister ConditionSource = iota
	CondSourceMemory
	CondSourceHitCount
)

// BreakpointCondition defines a conditional expression for a breakpoint.
type BreakpointCondition struct {
	Source  ConditionSource
	RegName string // register name (for CondSourceRegister)
	MemAddr uint64 // memory address (for CondSourceMemory)
	Op      ConditionOp
	Value   uint64
}

// ConditionalBreakpoint associates a breakpoint with an optional condition.
type ConditionalBreakpoint struct {
	Address   uint64
	Condition *BreakpointCondition // nil = unconditional
	HitCount  uint64
}

// WatchpointType indicates the type of watchpoint.
type WatchpointType int

const (
	WatchWrite WatchpointType = iota // Write watchpoint (only type currently supported)
)

// Watchpoint represents a write watchpoint on a memory address.
type Watchpoint struct {
	Type      WatchpointType
	Address   uint64
	LastValue byte
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
	SetConditionalBreakpoint(addr uint64, cond *BreakpointCondition) bool
	ClearBreakpoint(addr uint64) bool
	ClearAllBreakpoints()
	ListBreakpoints() []uint64
	ListConditionalBreakpoints() []*ConditionalBreakpoint
	HasBreakpoint(addr uint64) bool
	GetConditionalBreakpoint(addr uint64) *ConditionalBreakpoint

	SetWatchpoint(addr uint64) bool
	ClearWatchpoint(addr uint64) bool
	ClearAllWatchpoints()
	ListWatchpoints() []uint64

	ReadMemory(addr uint64, size int) []byte
	WriteMemory(addr uint64, data []byte)

	SetBreakpointChannel(ch chan<- BreakpointEvent, cpuID int)
}

// MonitorAttachable is implemented by video outputs that support the machine monitor overlay.
type MonitorAttachable interface {
	AttachMonitor(monitor *MachineMonitor)
}
