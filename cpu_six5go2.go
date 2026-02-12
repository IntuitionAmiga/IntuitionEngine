// cpu_six5go2.go - 6502 CPU Emulation Core for Intuition Engine

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
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
cpu_six5go2.go - Cycle-Accurate 6502 CPU Emulator

This module implements a complete MOS Technology 6502 processor emulation with
cycle-accurate timing and full support for all documented and undocumented
opcodes. The implementation focuses on accuracy while maintaining high
performance through careful memory and cache optimization.

Core Features:
- Complete 6502 instruction set implementation
- Cycle-accurate instruction timing
- Full decimal mode support
- Hardware interrupt handling (IRQ, NMI, Reset)
- Memory-mapped I/O capabilities
- Unofficial opcode support
- Comprehensive debugging features

Memory Layout Analysis (64-bit system):
Cache Line 0 (64 bytes) - Core State:
- PC, SP        : Program Counter and Stack Pointer
- A, X, Y       : Primary registers
- SR            : Status Register flags
- Running       : Execution state
- Debug         : Debug mode flag
- _padding0     : Alignment padding

Cache Line 1 (64 bytes) - Interrupt Control:
- irqPending    : IRQ signal state
- nmiLine       : NMI signal state
- nmiPending    : NMI detection flag
- nmiPrevious   : NMI edge detection
- resetPending  : Reset signal state
- InInterrupt   : Interrupt handler flag
- mutex         : State synchronization
- _padding1     : Cache line alignment

Cache Line 2 (64 bytes) - Timing Control:
- Cycles        : Cycle counter
- rdyLine       : RDY line state
- rdyHold       : RDY suspension flag
- memory        : Memory interface
- breakpoints   : Debug breakpoints
- breakpointHit : Debug notification

Thread Safety:
All CPU state modifications are protected by mutexes to ensure thread-safe
operation, particularly important for:
- Memory access operations
- Interrupt signal handling
- Debug breakpoint processing
- Timer and cycle counting
*/

package main

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

const (
	// CPU Configuration Constants

	STACK_BASE   = 0x0100 // Stack memory location
	RESET_VECTOR = 0xFFFC // Reset vector location
	IRQ_VECTOR   = 0xFFFE // IRQ vector location
	NMI_VECTOR   = 0xFFFA // NMI vector location
)

const (
	// 6502 VRAM Bank Window (16KB)
	VRAM_BANK_WINDOW_BASE = 0x8000
	VRAM_BANK_WINDOW_SIZE = 0x4000
	VRAM_BANK_REG         = 0xF7F0
	VRAM_BANK_REG_RSVD    = 0xF7F1

	// 6502 Extended Bank Windows for IE65 (8KB each)
	// These allow 6502 programs to access >64KB data through banking
	BANK1_WINDOW_BASE = 0x2000 // Sprite data bank
	BANK2_WINDOW_BASE = 0x4000 // Font data bank
	BANK3_WINDOW_BASE = 0x6000 // General data/AY bank
	BANK_WINDOW_SIZE  = 0x2000 // 8KB per bank window

	// Bank control registers (16-bit bank number each)
	BANK1_REG_LO = 0xF700 // Sprite bank select (low byte)
	BANK1_REG_HI = 0xF701 // Sprite bank select (high byte)
	BANK2_REG_LO = 0xF702 // Font bank select (low byte)
	BANK2_REG_HI = 0xF703 // Font bank select (high byte)
	BANK3_REG_LO = 0xF704 // General bank select (low byte)
	BANK3_REG_HI = 0xF705 // General bank select (high byte)
)

const (
	// Status Register Flags

	CARRY_FLAG     = 0x01 // Carry flag
	ZERO_FLAG      = 0x02 // Zero flag
	INTERRUPT_FLAG = 0x04 // Interrupt disable
	DECIMAL_FLAG   = 0x08 // Decimal mode
	BREAK_FLAG     = 0x10 // Break command
	UNUSED_FLAG    = 0x20 // Unused (always 1)
	OVERFLOW_FLAG  = 0x40 // Overflow flag
	NEGATIVE_FLAG  = 0x80 // Negative flag
)

var nzTable [256]byte

func init() {
	for i := 0; i < 256; i++ {
		if i == 0 {
			nzTable[i] |= ZERO_FLAG
		}
		if i&0x80 != 0 {
			nzTable[i] |= NEGATIVE_FLAG
		}
	}
}

type CPU_6502 struct {
	/*
	   CPU_6502 implements a cycle-accurate MOS Technology 6502 processor.

	   Core Registers:
	   - PC: Programme counter (16-bit)
	   - SP: Stack pointer (8-bit)
	   - A: Accumulator (8-bit)
	   - X, Y: Index registers (8-bit)
	   - SR: Status register (8-bit)

	   Control State:
	   - Running: Execution enabled
	   - Debug: Debug mode active
	   - Cycles: Performance counter
	   - InInterrupt: Interrupt handling flag

	   Signal Lines:
	   - irqPending: IRQ request
	   - nmiLine: NMI signal
	   - nmiPending: NMI detected
	   - nmiPrevious: NMI edge detection
	   - rdyLine: RDY signal
	   - rdyHold: RDY suspension

	   System Interface:
	   - memory: Memory bus access
	   - mutex: State synchronisation
	   - breakpoints: Debug control
	   - breakpointHit: Debug notification

	   Cache Optimisation:
	   Fields ordered for optimal cache line utilisation on 64-bit systems.
	   See detailed cache line documentation above struct fields.
	*/

	/*
	   Cache Line Layout (64-byte alignment):

	   Cache Line 0 (64 bytes) - Hot Path Registers:
	   Programme counter and primary registers are arranged for optimal L1 cache utilisation.
	   These values are accessed on nearly every instruction cycle.
	   - PC        uint16    // Programme counter, most frequently accessed
	   - SP        byte      // Stack pointer, frequent stack operations
	   - A         byte      // Accumulator, primary arithmetic
	   - X         byte      // X index, addressing modes
	   - Y         byte      // Y index, addressing modes
	   - SR        byte      // Status register, flags
	   - Running   bool      // Execution state
	   - _padding0 [57]byte  // Alignment padding

	   Cache Line 1 (64 bytes) - Interrupt Control:
	   Interrupt state is grouped for atomic access and isolation.
	   - irqPending   bool      // IRQ signal state
	   - resetPending bool      // Reset signal state
	   - InInterrupt  bool      // Handler active flag
	   - nmiLine      bool      // NMI signal state
	   - nmiPending   bool      // NMI detection
	   - nmiPrevious  bool      // Edge detection
	   - rdyLine      bool      // RDY signal state
	   - rdyHold      bool      // RDY suspension
	   - mutex        sync.RWMutex  // State protection
	   - _padding1    [24]byte     // Cache alignment

	   Cache Line 2 (64 bytes) - System Interface:
	   Memory and debug interfaces are grouped for coherent access.
	   - Cycles        uint64           // Performance counter
	   - memory        Bus6502   // Memory interface
	   - Debug         bool             // Debug mode
	   - breakpoints   map[uint16]bool  // Debug points
	   - breakpointHit chan uint16      // Debug channel
	*/

	// Cache Line 0 - Core State
	PC uint16 // Program Counter
	SP byte   // Stack Pointer
	A  byte   // Accumulator
	X  byte   // X Index Register
	Y  byte   // Y Index Register
	SR byte   // Status Register
	_  [57]byte

	// Cache Line 1 - Interrupt Control
	running      atomic.Bool // Execution state (atomic for lock-free access)
	irqPending   atomic.Bool // IRQ signal
	resetPending bool        // Reset signal
	InInterrupt  bool        // In handler
	nmiLine      atomic.Bool // NMI signal
	nmiPending   atomic.Bool // NMI detected
	resetting    atomic.Bool // Set during Reset() to pause Execute() at instruction boundary
	resetAck     atomic.Bool // Execute() signals it has paused for reset
	executing    atomic.Bool // True while Execute() is in its main loop
	_            [32]byte

	// Cache Line 2 - System Control
	Cycles        uint64          // Cycle counter
	rdyLine       atomic.Bool     // RDY signal
	rdyHold       bool            // RDY active
	Debug         bool            // Debug mode
	memory        Bus6502         // Memory interface
	fastAdapter   *Bus6502Adapter // Direct memory fast path (nil if non-standard bus)
	opcodeTable   [256]func(*CPU_6502)
	breakpoints   map[uint16]bool // Debug points
	breakpointHit chan uint16     // Debug channel
	_             [24]byte

	// Performance monitoring (matching IE32 pattern)
	PerfEnabled      bool      // Enable MIPS reporting
	InstructionCount uint64    // Total instructions executed
	perfStartTime    time.Time // When execution started
	lastPerfReport   time.Time // Last time we printed stats
}

// Running returns the execution state (thread-safe)
func (cpu_6502 *CPU_6502) Running() bool {
	return cpu_6502.running.Load()
}

// SetRunning sets the execution state (thread-safe)
func (cpu_6502 *CPU_6502) SetRunning(state bool) {
	cpu_6502.running.Store(state)
}

type Bus6502 interface {
	/*
	   Bus6502 defines the memory access protocol for the 6502.

	   Required Methods:
	   - Read: Fetches byte from specified address
	   - Write: Stores byte at specified address

	   Implementation Notes:
	   - Must handle memory-mapped I/O regions
	   - Must ensure proper byte-level access
	   - Should manage endianness for multi-byte operations
	*/

	Read(addr uint16) byte
	Write(addr uint16, value byte)
}
type Bus6502Adapter struct {
	/*
	   Bus6502Adapter adapts the 32-bit memory bus for 8-bit access.

	   Structure:
	   - bus: Reference to 32-bit system bus
	   - vramBank: Active VRAM bank for 6502 window access
	   - bank1/2/3: Extended bank windows for IE65 support
	   - vgaEngine: VGA engine for memory-mapped I/O access

	   Purpose:
	   Provides translation layer between 8-bit 6502 and 32-bit memory system.
	   Extended banking allows 6502 programs to access the full 16MB address
	   space through three additional 8KB bank windows at $2000, $4000, $6000.
	*/

	bus         Bus32
	vramBank    uint32
	vramEnabled bool
	vgaEngine   *VGAEngine // VGA engine for memory-mapped I/O access

	// Extended bank windows for IE65 support
	bank1       uint32 // Bank number for $2000-$3FFF window
	bank2       uint32 // Bank number for $4000-$5FFF window
	bank3       uint32 // Bank number for $6000-$7FFF window
	bank1Enable bool   // Bank 1 enabled
	bank2Enable bool   // Bank 2 enabled
	bank3Enable bool   // Bank 3 enabled

	// Direct memory fast path
	memDirect    []byte // cached from bus.GetMemory()
	ioPageBitmap []bool // reference to MachineBus.ioPageBitmap
	ioTable      [256]ioHandler
}

type ioHandler struct {
	read  func(*Bus6502Adapter, uint16) byte
	write func(*Bus6502Adapter, uint16, byte)
}

func NewCPU_6502(bus Bus32) *CPU_6502 {
	/*
	   NewCPU_6502 creates and initialises a new CPU instance.

	   Parameters:
	   - bus: Memory interface for system operations

	   Returns:
	   - *CPU_6502: Initialised CPU with:
	       - SR set to unused flag
	       - SP set to 0xFF (top of stack)
	       - Breakpoint map initialised
	       - Memory bus configured
	       - Running state enabled

	   Thread Safety:
	   Initial state setup requires no locks as object is not yet shared.
	*/
	adapter := NewBus6502Adapter(bus)
	cpu := &CPU_6502{
		memory:        adapter,
		fastAdapter:   adapter, // Set fastAdapter since we just created it
		SP:            0xFF,
		SR:            UNUSED_FLAG,
		breakpoints:   make(map[uint16]bool),
		breakpointHit: make(chan uint16, 1),
	}
	cpu.InitOpcodeTable()
	cpu.rdyLine.Store(true)
	cpu.running.Store(true)
	return cpu
}

func (cpu_6502 *CPU_6502) InitOpcodeTable() {
	cpu_6502.initOpcodeTableGenerated()
}

func (cpu_6502 *CPU_6502) ensureOpcodeTableReady() {
	if cpu_6502.opcodeTable[0xEA] == nil {
		panic("CPU_6502: opcodeTable not initialized, call InitOpcodeTable()")
	}
}

func (cpu_6502 *CPU_6502) readByte(addr uint16) byte {
	if cpu_6502.fastAdapter != nil {
		return cpu_6502.fastAdapter.ReadFast(addr)
	}
	return cpu_6502.memory.Read(addr)
}

func (cpu_6502 *CPU_6502) writeByte(addr uint16, value byte) {
	if cpu_6502.fastAdapter != nil {
		cpu_6502.fastAdapter.WriteFast(addr, value)
		return
	}
	cpu_6502.memory.Write(addr, value)
}

func (cpu_6502 *CPU_6502) updateNZ(value byte) {
	cpu_6502.SR = (cpu_6502.SR &^ (ZERO_FLAG | NEGATIVE_FLAG)) | nzTable[value]
}
func NewBus6502Adapter(bus Bus32) *Bus6502Adapter {
	/*
	   NewBus6502Adapter creates memory bus adapter instance.

	   Parameters:
	   - bus: System memory bus interface

	   Returns:
	   - *Bus6502Adapter: Configured adapter
	*/
	a := &Bus6502Adapter{bus: bus}
	a.memDirect = bus.GetMemory()
	if mb, ok := bus.(*MachineBus); ok {
		a.ioPageBitmap = mb.ioPageBitmap
	}
	a.initIOTable()
	return a
}

// NewBus6502AdapterWithVGA creates a 6502 memory bus adapter with VGA engine support
func NewBus6502AdapterWithVGA(bus Bus32, vga *VGAEngine) *Bus6502Adapter {
	a := &Bus6502Adapter{bus: bus, vgaEngine: vga}
	a.memDirect = bus.GetMemory()
	if mb, ok := bus.(*MachineBus); ok {
		a.ioPageBitmap = mb.ioPageBitmap
	}
	a.initIOTable()
	return a
}

func (a *Bus6502Adapter) initIOTable() {
	a.ioTable[0xD2] = ioHandler{read: readPOKEYPage, write: writePOKEYPage}
	a.ioTable[0xD4] = ioHandler{read: readPSGPage, write: writePSGPage}
	a.ioTable[0xD5] = ioHandler{read: readSIDPage, write: writeSIDPage}
	a.ioTable[0xD6] = ioHandler{read: readTEDPage, write: writeTEDPage}
	a.ioTable[0xD7] = ioHandler{read: readVGAPage, write: writeVGAPage}
	a.ioTable[0xD8] = ioHandler{read: readULAPage, write: writeULAPage}
	a.ioTable[0xF7] = ioHandler{read: readBankRegPage, write: writeBankRegPage}
}

func readPOKEYPage(a *Bus6502Adapter, addr uint16) byte {
	if addr >= C6502_POKEY_BASE && addr <= C6502_POKEY_END {
		return a.bus.Read8(POKEY_BASE + uint32(addr-C6502_POKEY_BASE))
	}
	return a.bus.Read8(translateIO8Bit_6502(addr))
}

func writePOKEYPage(a *Bus6502Adapter, addr uint16, value byte) {
	if addr >= C6502_POKEY_BASE && addr <= C6502_POKEY_END {
		a.bus.Write8(POKEY_BASE+uint32(addr-C6502_POKEY_BASE), value)
		return
	}
	a.bus.Write8(translateIO8Bit_6502(addr), value)
}

func readPSGPage(a *Bus6502Adapter, addr uint16) byte {
	if addr >= C6502_PSG_BASE && addr <= C6502_PSG_END {
		return a.bus.Read8(PSG_BASE + uint32(addr-C6502_PSG_BASE))
	}
	return a.bus.Read8(translateIO8Bit_6502(addr))
}

func writePSGPage(a *Bus6502Adapter, addr uint16, value byte) {
	if addr >= C6502_PSG_BASE && addr <= C6502_PSG_END {
		a.bus.Write8(PSG_BASE+uint32(addr-C6502_PSG_BASE), value)
		return
	}
	a.bus.Write8(translateIO8Bit_6502(addr), value)
}

func readSIDPage(a *Bus6502Adapter, addr uint16) byte {
	if addr >= C6502_SID_BASE && addr <= C6502_SID_END {
		return a.bus.Read8(SID_BASE + uint32(addr-C6502_SID_BASE))
	}
	return a.bus.Read8(translateIO8Bit_6502(addr))
}

func writeSIDPage(a *Bus6502Adapter, addr uint16, value byte) {
	if addr >= C6502_SID_BASE && addr <= C6502_SID_END {
		a.bus.Write8(SID_BASE+uint32(addr-C6502_SID_BASE), value)
		return
	}
	a.bus.Write8(translateIO8Bit_6502(addr), value)
}

func readTEDPage(a *Bus6502Adapter, addr uint16) byte {
	if addr >= C6502_TED_BASE && addr <= C6502_TED_END {
		return a.bus.Read8(TED_BASE + uint32(addr-C6502_TED_BASE))
	}
	if addr >= C6502_TED_V_BASE && addr <= C6502_TED_V_END {
		return a.bus.Read8(TED_VIDEO_BASE + uint32((addr-C6502_TED_V_BASE)*4))
	}
	return a.bus.Read8(translateIO8Bit_6502(addr))
}

func writeTEDPage(a *Bus6502Adapter, addr uint16, value byte) {
	if addr >= C6502_TED_BASE && addr <= C6502_TED_END {
		a.bus.Write8(TED_BASE+uint32(addr-C6502_TED_BASE), value)
		return
	}
	if addr >= C6502_TED_V_BASE && addr <= C6502_TED_V_END {
		a.bus.Write8(TED_VIDEO_BASE+uint32((addr-C6502_TED_V_BASE)*4), value)
		return
	}
	a.bus.Write8(translateIO8Bit_6502(addr), value)
}

func readVGAPage(a *Bus6502Adapter, addr uint16) byte {
	if a.vgaEngine != nil && addr >= C6502_VGA_BASE && addr <= C6502_VGA_END {
		switch addr {
		case C6502_VGA_MODE:
			return byte(a.vgaEngine.HandleRead(VGA_MODE))
		case C6502_VGA_STATUS:
			return byte(a.vgaEngine.HandleRead(VGA_STATUS))
		case C6502_VGA_CTRL:
			return byte(a.vgaEngine.HandleRead(VGA_CTRL))
		case C6502_VGA_SEQ_IDX:
			return byte(a.vgaEngine.HandleRead(VGA_SEQ_INDEX))
		case C6502_VGA_SEQ_DATA:
			return byte(a.vgaEngine.HandleRead(VGA_SEQ_DATA))
		case C6502_VGA_CRTC_IDX:
			return byte(a.vgaEngine.HandleRead(VGA_CRTC_INDEX))
		case C6502_VGA_CRTC_DATA:
			return byte(a.vgaEngine.HandleRead(VGA_CRTC_DATA))
		case C6502_VGA_GC_IDX:
			return byte(a.vgaEngine.HandleRead(VGA_GC_INDEX))
		case C6502_VGA_GC_DATA:
			return byte(a.vgaEngine.HandleRead(VGA_GC_DATA))
		case C6502_VGA_DAC_WIDX:
			return byte(a.vgaEngine.HandleRead(VGA_DAC_WINDEX))
		case C6502_VGA_DAC_DATA:
			return byte(a.vgaEngine.HandleRead(VGA_DAC_DATA))
		}
	}
	return a.bus.Read8(translateIO8Bit_6502(addr))
}

func writeVGAPage(a *Bus6502Adapter, addr uint16, value byte) {
	if a.vgaEngine != nil && addr >= C6502_VGA_BASE && addr <= C6502_VGA_END {
		switch addr {
		case C6502_VGA_MODE:
			a.vgaEngine.HandleWrite(VGA_MODE, uint32(value))
			return
		case C6502_VGA_STATUS:
			return
		case C6502_VGA_CTRL:
			a.vgaEngine.HandleWrite(VGA_CTRL, uint32(value))
			return
		case C6502_VGA_SEQ_IDX:
			a.vgaEngine.HandleWrite(VGA_SEQ_INDEX, uint32(value))
			return
		case C6502_VGA_SEQ_DATA:
			a.vgaEngine.HandleWrite(VGA_SEQ_DATA, uint32(value))
			return
		case C6502_VGA_CRTC_IDX:
			a.vgaEngine.HandleWrite(VGA_CRTC_INDEX, uint32(value))
			return
		case C6502_VGA_CRTC_DATA:
			a.vgaEngine.HandleWrite(VGA_CRTC_DATA, uint32(value))
			return
		case C6502_VGA_GC_IDX:
			a.vgaEngine.HandleWrite(VGA_GC_INDEX, uint32(value))
			return
		case C6502_VGA_GC_DATA:
			a.vgaEngine.HandleWrite(VGA_GC_DATA, uint32(value))
			return
		case C6502_VGA_DAC_WIDX:
			a.vgaEngine.HandleWrite(VGA_DAC_WINDEX, uint32(value))
			return
		case C6502_VGA_DAC_DATA:
			a.vgaEngine.HandleWrite(VGA_DAC_DATA, uint32(value))
			return
		}
	}
	a.bus.Write8(translateIO8Bit_6502(addr), value)
}

func readULAPage(a *Bus6502Adapter, addr uint16) byte {
	if addr >= C6502_ULA_BASE && addr <= C6502_ULA_BASE+0x0F {
		return a.bus.Read8(ULA_BASE + uint32(addr-C6502_ULA_BASE))
	}
	return a.bus.Read8(translateIO8Bit_6502(addr))
}

func writeULAPage(a *Bus6502Adapter, addr uint16, value byte) {
	if addr >= C6502_ULA_BASE && addr <= C6502_ULA_BASE+0x0F {
		a.bus.Write8(ULA_BASE+uint32(addr-C6502_ULA_BASE), value)
		return
	}
	a.bus.Write8(translateIO8Bit_6502(addr), value)
}

func readBankRegPage(a *Bus6502Adapter, addr uint16) byte {
	switch addr {
	case VRAM_BANK_REG:
		return byte(a.vramBank & 0xFF)
	case VRAM_BANK_REG_RSVD:
		return 0
	case BANK1_REG_LO:
		return byte(a.bank1 & 0xFF)
	case BANK1_REG_HI:
		return byte((a.bank1 >> 8) & 0xFF)
	case BANK2_REG_LO:
		return byte(a.bank2 & 0xFF)
	case BANK2_REG_HI:
		return byte((a.bank2 >> 8) & 0xFF)
	case BANK3_REG_LO:
		return byte(a.bank3 & 0xFF)
	case BANK3_REG_HI:
		return byte((a.bank3 >> 8) & 0xFF)
	default:
		return a.bus.Read8(translateIO8Bit_6502(addr))
	}
}

func writeBankRegPage(a *Bus6502Adapter, addr uint16, value byte) {
	switch addr {
	case VRAM_BANK_REG:
		a.vramBank = uint32(value)
		a.vramEnabled = true
		return
	case VRAM_BANK_REG_RSVD:
		return
	case BANK1_REG_LO:
		a.bank1 = (a.bank1 & 0xFF00) | uint32(value)
		a.bank1Enable = true
		return
	case BANK1_REG_HI:
		a.bank1 = (a.bank1 & 0x00FF) | (uint32(value) << 8)
		a.bank1Enable = true
		return
	case BANK2_REG_LO:
		a.bank2 = (a.bank2 & 0xFF00) | uint32(value)
		a.bank2Enable = true
		return
	case BANK2_REG_HI:
		a.bank2 = (a.bank2 & 0x00FF) | (uint32(value) << 8)
		a.bank2Enable = true
		return
	case BANK3_REG_LO:
		a.bank3 = (a.bank3 & 0xFF00) | uint32(value)
		a.bank3Enable = true
		return
	case BANK3_REG_HI:
		a.bank3 = (a.bank3 & 0x00FF) | (uint32(value) << 8)
		a.bank3Enable = true
		return
	default:
		a.bus.Write8(translateIO8Bit_6502(addr), value)
	}
}

func (a *Bus6502Adapter) ReadFast(addr uint16) byte {
	// Fast path: has bitmap, no I/O on this page, and within standard 6502 address space
	// We check addr < 0x2000 because ZP and Stack are there, and it's the most common RAM area.
	// We also fall back to slow path for banking regions ($2000-$7FFF) or VRAM window ($8000-$BFFF).
	if a.ioPageBitmap != nil && addr < 0x2000 && !a.ioPageBitmap[addr>>8] {
		return a.memDirect[addr]
	}
	// Slow path: no bitmap, banking, I/O, or VRAM
	return a.Read(addr)
}

func (a *Bus6502Adapter) WriteFast(addr uint16, val byte) {
	if a.ioPageBitmap != nil && addr < 0x2000 && !a.ioPageBitmap[addr>>8] {
		a.memDirect[addr] = val
		return
	}
	a.Write(addr, val)
}

func (a *Bus6502Adapter) ReadZP(addr byte) byte {
	// Zero page is $0000-$00FF. Always page 0.
	if a.ioPageBitmap != nil && !a.ioPageBitmap[0] {
		return a.memDirect[addr]
	}
	return a.Read(uint16(addr))
}

func (a *Bus6502Adapter) WriteZP(addr byte, val byte) {
	if a.ioPageBitmap != nil && !a.ioPageBitmap[0] {
		a.memDirect[addr] = val
		return
	}
	a.Write(uint16(addr), val)
}

func (a *Bus6502Adapter) ReadStack(sp byte) byte {
	// Stack is $0100-$01FF. Always page 1.
	if a.ioPageBitmap != nil && !a.ioPageBitmap[1] {
		return a.memDirect[0x0100|uint16(sp)]
	}
	return a.Read(0x0100 | uint16(sp))
}

func (a *Bus6502Adapter) WriteStack(sp byte, val byte) {
	if a.ioPageBitmap != nil && !a.ioPageBitmap[1] {
		a.memDirect[0x0100|uint16(sp)] = val
		return
	}
	a.Write(0x0100|uint16(sp), val)
}

func (cpu_6502 *CPU_6502) rmw(addr uint16, operation func(byte) byte) {
	/*
	   rmw performs read-modify-write operations.

	   Parameters:
	   - addr: Target address
	   - operation: Modification function

	   Operation:
	   1. Reads original value
	   2. Writes back original (spurious)
	   3. Applies operation
	   4. Writes modified value
	*/

	value := cpu_6502.readByte(addr)
	cpu_6502.writeByte(addr, value) // Spurious write of original value
	result := operation(value)
	cpu_6502.writeByte(addr, result)
}
func (cpu_6502 *CPU_6502) read16(addr uint16) uint16 {
	/*
	   read16 performs a 16-bit read operation from memory.

	   Parameters:
	   - addr: Source memory address

	   Returns:
	   - uint16: 16-bit value in little-endian format

	   Memory Access:
	   1. Reads low byte from addr
	   2. Reads high byte from addr+1
	   3. Combines into 16-bit value

	   Thread Safety:
	   Protected by memory interface mutex
	*/

	lo := uint16(cpu_6502.readByte(addr))
	hi := uint16(cpu_6502.readByte(addr + 1))
	return (hi << 8) | lo
}
func (cpu_6502 *CPU_6502) push(value byte) {
	/*
	   push adds a byte to the stack.

	   Parameters:
	   - value: Byte to push onto stack

	   Stack Management:
	   1. Computes stack address from STACK_BASE and SP
	   2. Writes value to stack
	   3. Decrements stack pointer

	   Thread Safety:
	   Protected by memory write mutex
	*/

	if cpu_6502.fastAdapter != nil {
		cpu_6502.fastAdapter.WriteStack(cpu_6502.SP, value)
	} else {
		cpu_6502.writeByte(STACK_BASE|uint16(cpu_6502.SP), value)
	}
	cpu_6502.SP--
}
func (cpu_6502 *CPU_6502) push16(value uint16) {
	/*
	   push16 adds a 16-bit value to the stack.

	   Parameters:
	   - value: 16-bit value to push

	   Stack Management:
	   1. Pushes high byte first
	   2. Pushes low byte second
	   3. Uses push() for each byte

	   Thread Safety:
	   Protected by push() mutex locking
	*/

	cpu_6502.push(byte(value >> 8))
	cpu_6502.push(byte(value & 0xFF))
}
func (cpu_6502 *CPU_6502) pop() byte {
	/*
	   pop removes and returns a byte from the stack.

	   Returns:
	   - byte: Value popped from stack

	   Stack Management:
	   1. Increments stack pointer
	   2. Reads value from updated stack position

	   Thread Safety:
	   Protected by memory read mutex
	*/

	cpu_6502.SP++
	if cpu_6502.fastAdapter != nil {
		return cpu_6502.fastAdapter.ReadStack(cpu_6502.SP)
	}
	return cpu_6502.readByte(STACK_BASE | uint16(cpu_6502.SP))
}
func (cpu_6502 *CPU_6502) pop16() uint16 {
	/*
	   pop16 removes and returns a 16-bit value from stack.

	   Returns:
	   - uint16: 16-bit value in correct endianness

	   Stack Management:
	   1. Pops low byte first
	   2. Pops high byte second
	   3. Combines into 16-bit value

	   Thread Safety:
	   Protected by pop() mutex locking
	*/

	lo := uint16(cpu_6502.pop())
	hi := uint16(cpu_6502.pop())
	return (hi << 8) | lo
}

func (cpu_6502 *CPU_6502) setFlag(flag byte, value bool) {
	/*
	   setFlag modifies a status register flag.

	   Parameters:
	   - flag: Flag bit to modify
	   - value: New flag state

	   Operation:
	   1. For true: Sets flag bit using OR
	   2. For false: Clears flag bit using AND NOT

	   Thread Safety:
	   Direct register access requires no mutex
	*/

	if value {
		cpu_6502.SR |= flag
	} else {
		cpu_6502.SR &^= flag
	}
}
func (cpu_6502 *CPU_6502) getFlag(flag byte) bool {
	/*
	   getFlag reads a status register flag state.

	   Parameters:
	   - flag: Flag bit to read

	   Returns:
	   - bool: Current flag state

	   Operation:
	   Tests specified bit in status register

	   Thread Safety:
	   Direct register read requires no mutex
	*/

	return (cpu_6502.SR & flag) != 0
}

func (cpu_6502 *CPU_6502) getAbsolute() uint16 {
	/*
	   getAbsolute resolves absolute addressing.

	   Returns:
	   - uint16: 16-bit memory address

	   Operation:
	   1. Reads low byte from PC
	   2. Reads high byte from PC+1
	   3. Combines into 16-bit address
	   4. Increments PC by 2

	   Thread Safety:
	   Protected by memory read mutex
	*/

	addr := cpu_6502.read16(cpu_6502.PC)
	cpu_6502.PC += 2
	return addr
}
func (cpu_6502 *CPU_6502) getAbsoluteX() (uint16, bool) {
	/*
	   getAbsoluteX resolves absolute X-indexed addressing.

	   Returns:
	   - uint16: Computed address
	   - bool: True if page boundary crossed

	   Operation:
	   1. Gets absolute address
	   2. Adds X register
	   3. Checks page crossing
	   4. Updates PC

	   Thread Safety:
	   Protected by memory read mutex
	*/

	base := cpu_6502.read16(cpu_6502.PC)
	cpu_6502.PC += 2
	addr := base + uint16(cpu_6502.X)
	return addr, (base & 0xFF00) != (addr & 0xFF00)
}
func (cpu_6502 *CPU_6502) getAbsoluteY() (uint16, bool) {
	/*
	   getAbsoluteY resolves absolute Y-indexed addressing.

	   Returns:
	   - uint16: Computed address
	   - bool: True if page crossed

	   Operation:
	   1. Reads absolute address
	   2. Adds Y register
	   3. Checks page crossing
	*/

	base := cpu_6502.read16(cpu_6502.PC)
	cpu_6502.PC += 2
	addr := base + uint16(cpu_6502.Y)
	return addr, (base & 0xFF00) != (addr & 0xFF00)
}
func (cpu_6502 *CPU_6502) getZeroPage() uint16 {
	/*
	   getZeroPage resolves zero page addressing.

	   Returns:
	   - uint16: Zero page address

	   Operation:
	   1. Reads byte from PC
	   2. Increments PC
	   3. Returns as zero page address

	   Thread Safety:
	   Protected by memory read mutex
	*/

	addr := uint16(cpu_6502.readByte(cpu_6502.PC))
	cpu_6502.PC++
	return addr
}
func (cpu_6502 *CPU_6502) getZeroPageX() uint16 {
	/*
	   getZeroPageX resolves zero page X-indexed addressing.

	   Returns:
	   - uint16: Zero page address with X offset

	   Operation:
	   1. Reads base address from PC
	   2. Adds X register (wraps in zero page)
	   3. Increments PC

	   Thread Safety:
	   Protected by memory read mutex
	*/

	addr := (uint16(cpu_6502.readByte(cpu_6502.PC)) + uint16(cpu_6502.X)) & 0xFF
	cpu_6502.PC++
	return addr
}
func (cpu_6502 *CPU_6502) getZeroPageY() uint16 {
	/*
	   getZeroPageY resolves zero page Y-indexed addressing.

	   Returns:
	   - uint16: Zero page address with Y offset

	   Operation:
	   1. Reads base address from PC
	   2. Adds Y register (wraps in zero page)
	   3. Increments PC

	   Thread Safety:
	   Protected by memory read mutex
	*/

	addr := (uint16(cpu_6502.readByte(cpu_6502.PC)) + uint16(cpu_6502.Y)) & 0xFF
	cpu_6502.PC++
	return addr
}
func (cpu_6502 *CPU_6502) getIndirectX() uint16 {
	/*
	   getIndirectX resolves X-indexed indirect addressing.

	   Returns:
	   - uint16: Resolved address

	   Operation:
	   1. Reads zero page address
	   2. Adds X register (zero page wrap)
	   3. Reads address from result
	*/

	base := cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++
	ptr := (uint16(base) + uint16(cpu_6502.X)) & 0xFF
	if cpu_6502.fastAdapter != nil {
		lo := uint16(cpu_6502.fastAdapter.ReadZP(byte(ptr)))
		hi := uint16(cpu_6502.fastAdapter.ReadZP(byte((ptr + 1) & 0xFF)))
		return lo | (hi << 8)
	}
	return uint16(cpu_6502.readByte(ptr)) | uint16(cpu_6502.readByte((ptr+1)&0xFF))<<8
}
func (cpu_6502 *CPU_6502) getIndirectY() (uint16, bool) {
	/*
	   getIndirectY resolves indirect Y-indexed addressing.

	   Returns:
	   - uint16: Resolved address
	   - bool: True if page crossed

	   Operation:
	   1. Reads zero page address
	   2. Reads indirect address
	   3. Adds Y register
	   4. Checks page crossing
	*/

	ptr := uint16(cpu_6502.readByte(cpu_6502.PC))
	cpu_6502.PC++
	var base uint16
	if cpu_6502.fastAdapter != nil {
		base = uint16(cpu_6502.fastAdapter.ReadZP(byte(ptr))) |
			uint16(cpu_6502.fastAdapter.ReadZP(byte((ptr+1)&0xFF)))<<8
	} else {
		base = uint16(cpu_6502.readByte(ptr)) | uint16(cpu_6502.readByte((ptr+1)&0xFF))<<8
	}
	addr := base + uint16(cpu_6502.Y)
	return addr, (base & 0xFF00) != (addr & 0xFF00)
}

func (cpu_6502 *CPU_6502) adc(value byte) {
	/*
	   adc performs addition with carry.

	   Parameters:
	   - value: Value to add to accumulator

	   Operation Modes:
	   - Binary: Standard two's complement
	   - Decimal: BCD arithmetic if decimal flag set

	   Flag Updates:
	   - Carry: Set on overflow
	   - Zero: Set if result zero
	   - Overflow: Set on signed overflow
	   - Negative: Set if bit 7 set

	   Thread Safety:
	   Direct register access requires no mutex
	*/

	if cpu_6502.SR&DECIMAL_FLAG != 0 {
		a := uint16(cpu_6502.A)
		b := uint16(value)
		carry := btou16(cpu_6502.SR&CARRY_FLAG != 0)

		lo_a := a & 0x0F
		hi_a := (a >> 4) & 0x0F
		lo_b := b & 0x0F
		hi_b := (b >> 4) & 0x0F

		lo_sum := lo_a + lo_b + carry
		carry = 0
		if lo_sum > 9 {
			lo_sum -= 10
			carry = 1
		}

		hi_sum := hi_a + hi_b + carry
		carry = 0
		if hi_sum > 9 {
			hi_sum -= 10
			carry = 1
		}

		result := (hi_sum << 4) | lo_sum

		cpu_6502.setFlag(CARRY_FLAG, carry == 1)
		cpu_6502.updateNZ(byte(result))

		old_a := cpu_6502.A
		cpu_6502.A = byte(result)

		// V Flag is set based on two's complement overflow
		overflow := (old_a^value)&0x80 == 0 && (old_a^cpu_6502.A)&0x80 != 0
		cpu_6502.setFlag(OVERFLOW_FLAG, overflow)
	} else {
		temp := uint16(cpu_6502.A) + uint16(value)
		if cpu_6502.SR&CARRY_FLAG != 0 {
			temp++
		}

		carry := temp > 0xFF
		result := byte(temp)

		cpu_6502.setFlag(CARRY_FLAG, carry)
		cpu_6502.updateNZ(result)

		overflow := (cpu_6502.A^value)&0x80 == 0 && (cpu_6502.A^result)&0x80 != 0
		cpu_6502.setFlag(OVERFLOW_FLAG, overflow)

		cpu_6502.A = result
	}
}
func (cpu_6502 *CPU_6502) sbc(value byte) {
	/*
	   sbc performs subtraction with carry.

	   Parameters:
	   - value: Value to subtract from accumulator

	   Operation Modes:
	   - Binary: Standard two's complement
	   - Decimal: BCD arithmetic if decimal flag set

	   Flag Updates:
	   - Carry: Set on borrow
	   - Zero: Set if result zero
	   - Overflow: Set on signed overflow
	   - Negative: Set if bit 7 set

	   Thread Safety:
	   Direct register access requires no mutex
	*/

	if cpu_6502.SR&DECIMAL_FLAG != 0 {
		a := uint16(cpu_6502.A)
		b := uint16(value)
		borrow := btou16(cpu_6502.SR&CARRY_FLAG == 0)

		lo_a := a & 0x0F
		hi_a := (a >> 4) & 0x0F
		lo_b := b & 0x0F
		hi_b := (b >> 4) & 0x0F

		lo_diff := lo_a - lo_b - borrow
		borrow = 0
		if lo_diff&0x10 != 0 {
			lo_diff = (lo_diff - 6) & 0x0F
			borrow = 1
		}

		hi_diff := hi_a - hi_b - borrow
		borrow = 0
		if hi_diff&0x10 != 0 {
			hi_diff = (hi_diff - 6) & 0x0F
			borrow = 1
		}

		result := (hi_diff << 4) | lo_diff

		cpu_6502.setFlag(CARRY_FLAG, borrow == 0)
		cpu_6502.updateNZ(byte(result))

		old_a := cpu_6502.A
		cpu_6502.A = byte(result)

		overflow := (old_a^value)&0x80 != 0 && (old_a^cpu_6502.A)&0x80 != 0
		cpu_6502.setFlag(OVERFLOW_FLAG, overflow)
	} else {
		temp := uint16(cpu_6502.A) - uint16(value)
		if cpu_6502.SR&CARRY_FLAG == 0 {
			temp--
		}

		result := byte(temp)
		carry := temp < 0x100

		cpu_6502.setFlag(CARRY_FLAG, carry)
		cpu_6502.updateNZ(result)

		overflow := (cpu_6502.A^value)&0x80 != 0 && (cpu_6502.A^result)&0x80 != 0
		cpu_6502.setFlag(OVERFLOW_FLAG, overflow)

		cpu_6502.A = result
	}
}
func (cpu_6502 *CPU_6502) inc(addr uint16) byte {
	/*
	   inc performs memory increment.

	   Parameters:
	   - addr: Target address

	   Returns:
	   - byte: Incremented value

	   Flag Updates:
	   - Zero: Set if result zero
	   - Negative: Set if bit 7 set
	*/

	var result byte
	cpu_6502.rmw(addr, func(value byte) byte {
		result = value + 1
		cpu_6502.updateNZ(result)
		return result
	})
	return result
}
func (cpu_6502 *CPU_6502) dec(addr uint16) byte {
	/*
	   dec performs memory decrement.

	   Parameters:
	   - addr: Target address

	   Returns:
	   - byte: Decremented value

	   Flag Updates:
	   - Zero: Set if result zero
	   - Negative: Set if bit 7 set
	*/

	var result byte
	cpu_6502.rmw(addr, func(value byte) byte {
		result = value - 1
		cpu_6502.updateNZ(result)
		return result
	})
	return result
}

func (cpu_6502 *CPU_6502) asl(addr uint16, accumulator bool) byte {
	/*
	   asl performs arithmetic shift left.

	   Parameters:
	   - addr: Memory address (if !accumulator)
	   - accumulator: True for A register operation

	   Returns:
	   - byte: Shifted value

	   Flag Updates:
	   - Carry: Old bit 7
	   - Zero: Set if result zero
	   - Negative: New bit 7
	*/

	if accumulator {
		cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x80 != 0)
		cpu_6502.A <<= 1
		cpu_6502.updateNZ(cpu_6502.A)
		return cpu_6502.A
	}

	var result byte
	cpu_6502.rmw(addr, func(value byte) byte {
		cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
		result = value << 1
		cpu_6502.updateNZ(result)
		return result
	})
	return result
}
func (cpu_6502 *CPU_6502) lsr(addr uint16, accumulator bool) byte {
	/*
	   lsr performs logical shift right.

	   Parameters:
	   - addr: Memory address (if !accumulator)
	   - accumulator: True for A register operation

	   Returns:
	   - byte: Shifted value

	   Flag Updates:
	   - Carry: Old bit 0
	   - Zero: Set if result zero
	   - Negative: Always clear
	*/

	if accumulator {
		cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x01 != 0)
		cpu_6502.A >>= 1
		cpu_6502.updateNZ(cpu_6502.A)
		return cpu_6502.A
	}

	var result byte
	cpu_6502.rmw(addr, func(value byte) byte {
		cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
		result = value >> 1
		cpu_6502.updateNZ(result)
		return result
	})
	return result
}
func (cpu_6502 *CPU_6502) rol(addr uint16, accumulator bool) byte {
	/*
	   rol performs rotate left through carry.

	   Parameters:
	   - addr: Memory address (if !accumulator)
	   - accumulator: True for A register operation

	   Returns:
	   - byte: Rotated value

	   Operation:
	   1. Shifts bits left
	   2. Moves carry into bit 0
	   3. Updates carry flag
	*/

	if accumulator {
		carry := btou8(cpu_6502.SR&CARRY_FLAG != 0)
		cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x80 != 0)
		cpu_6502.A = (cpu_6502.A << 1) | carry
		cpu_6502.updateNZ(cpu_6502.A)
		return cpu_6502.A
	}

	var result byte
	oldCarry := btou8(cpu_6502.SR&CARRY_FLAG != 0)
	cpu_6502.rmw(addr, func(value byte) byte {
		cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
		result = (value << 1) | oldCarry
		cpu_6502.updateNZ(result)
		return result
	})
	return result
}
func (cpu_6502 *CPU_6502) ror(addr uint16, accumulator bool) byte {
	/*
	   ror performs rotate right through carry.

	   Parameters:
	   - addr: Memory address (if !accumulator)
	   - accumulator: True for A register operation

	   Returns:
	   - byte: Rotated value

	   Operation:
	   1. Shifts bits right
	   2. Moves carry into bit 7
	   3. Updates carry flag
	*/

	if accumulator {
		carry := btou8(cpu_6502.SR&CARRY_FLAG != 0)
		cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x01 != 0)
		cpu_6502.A = (cpu_6502.A >> 1) | (carry << 7)
		cpu_6502.updateNZ(cpu_6502.A)
		return cpu_6502.A
	}

	var result byte
	oldCarry := btou8(cpu_6502.SR&CARRY_FLAG != 0)
	cpu_6502.rmw(addr, func(value byte) byte {
		cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
		result = (value >> 1) | (oldCarry << 7)
		cpu_6502.updateNZ(result)
		return result
	})
	return result
}

func (cpu_6502 *CPU_6502) compare(reg, value byte) {
	/*
	   compare performs register comparison.

	   Parameters:
	   - reg: Register value
	   - value: Comparison value

	   Flag Updates:
	   - Carry: Set if reg >= value
	   - Zero: Set if reg == value
	   - Negative: Set if result bit 7 set
	*/

	temp := uint16(reg) - uint16(value)
	cpu_6502.setFlag(CARRY_FLAG, reg >= value)
	cpu_6502.updateNZ(byte(temp))
}
func (cpu_6502 *CPU_6502) branch(condition bool) {
	/*
	   branch performs conditional branch.

	   Parameters:
	   - condition: Branch condition

	   Operation:
	   1. Reads offset byte
	   2. If condition true:
	      - Adds offset to PC
	      - Adds cycle for branch
	      - Adds cycle for page cross
	*/

	offset := int8(cpu_6502.readByte(cpu_6502.PC))
	cpu_6502.PC++
	if condition {
		cpu_6502.Cycles++
		oldPC := cpu_6502.PC
		cpu_6502.PC = uint16(int32(cpu_6502.PC) + int32(offset))
		if (oldPC & 0xFF00) != (cpu_6502.PC & 0xFF00) {
			cpu_6502.Cycles++
		}
	}
}

func (cpu_6502 *CPU_6502) handleInterrupt(vector uint16, isNMI bool) {
	/*
	   handleInterrupt processes hardware interrupts.

	   Parameters:
	   - vector: Interrupt vector address
	   - isNMI: True for NMI, false for IRQ

	   Interrupt Flow:
	   1. Checks interrupt disable flag (except NMI)
	   2. Pushes return address to stack
	   3. Pushes status register to stack
	   4. Sets interrupt disable flag
	   5. Loads new PC from vector
	   6. Adds interrupt cycle cost

	   Thread Safety:
	   Protected by CPU mutex during processing
	*/

	if !isNMI && cpu_6502.SR&INTERRUPT_FLAG != 0 {
		return
	}

	cpu_6502.push16(cpu_6502.PC)
	cpu_6502.push(cpu_6502.SR)
	cpu_6502.setFlag(INTERRUPT_FLAG, true)
	cpu_6502.PC = cpu_6502.read16(vector)
	cpu_6502.Cycles += 7
}
func (cpu_6502 *CPU_6502) SetNMILine(state bool) {
	/*
	   SetNMILine controls the NMI line state.

	   Parameters:
	   - state: New NMI line state

	   Operation:
	   1. Updates NMI line state
	   2. Detects falling edge via atomic Swap
	   3. Sets NMI pending on edge
	*/

	old := cpu_6502.nmiLine.Swap(state)
	// Detect falling edge (1->0 transition)
	if old && !state {
		cpu_6502.nmiPending.Store(true)
	}
}
func (cpu_6502 *CPU_6502) SetRDYLine(state bool) {
	/*
	   SetRDYLine controls the RDY line state.

	   Parameters:
	   - state: New RDY line state

	   Operation:
	   1. Updates RDY state atomically
	*/

	cpu_6502.rdyLine.Store(state)
}

func (cpu_6502 *CPU_6502) Reset() {
	/*
	   Reset initialises the CPU to power-up state.

	   Reset Process:
	   1. Acquires state mutex
	   2. Clears all registers
	   3. Sets stack pointer to 0xFF
	   4. Sets status register defaults
	   5. Loads PC from reset vector
	   6. Clears cycle counter
	   7. Resets interrupt state
	   8. Resets NMI edge detection

	   Thread Safety:
	   Uses resetting/resetAck handshake to pause Execute() at an
	   instruction boundary before modifying registers.
	*/

	// Signal Execute() to pause at the next instruction boundary
	cpu_6502.resetting.Store(true)
	if cpu_6502.executing.Load() {
		for !cpu_6502.resetAck.Load() {
			if !cpu_6502.executing.Load() {
				break
			}
			runtime.Gosched()
		}
	}

	// Execute() is now paused — safe to modify all registers
	cpu_6502.A = 0
	cpu_6502.X = 0
	cpu_6502.Y = 0
	cpu_6502.SP = 0xFF
	cpu_6502.SR = UNUSED_FLAG | INTERRUPT_FLAG
	cpu_6502.PC = cpu_6502.read16(RESET_VECTOR)
	cpu_6502.Cycles = 0
	cpu_6502.running.Store(true)
	cpu_6502.InInterrupt = false

	//NMI
	cpu_6502.nmiLine.Store(false)
	cpu_6502.nmiPending.Store(false)
	cpu_6502.irqPending.Store(false)
	cpu_6502.resetPending = false

	if adapter, ok := cpu_6502.memory.(*Bus6502Adapter); ok {
		adapter.ResetBank()
	}

	// Resume Execute()
	cpu_6502.resetting.Store(false)
}

func (cpu_6502 *CPU_6502) Execute() {
	/*
	   Execute runs the main CPU instruction cycle.

	   Execution Flow:
	   1. While CPU is running:
	       - Check RDY line state
	       - Process pending interrupts
	       - Fetch next instruction
	       - Handle breakpoints
	       - Execute instruction
	       - Update cycle count
	   2. On halt:
	       - Log final state
	       - Exit cleanly

	   Instruction Processing:
	   - Fetches opcode
	   - Determines addressing mode
	   - Resolves operands
	   - Executes operation
	   - Updates PC unless modified

	   Interrupt Handling:
	   - Checks NMI edge
	   - Processes IRQ if enabled
	   - Updates interrupt state

	   Thread Safety:
	   - Main loop protected by CPU mutex
	   - Memory access via sync'd interface
	   - Breakpoints use channel sync
	*/

	// Initialize perf counters if enabled
	if cpu_6502.PerfEnabled {
		cpu_6502.perfStartTime = time.Now()
		cpu_6502.lastPerfReport = cpu_6502.perfStartTime
		cpu_6502.InstructionCount = 0
	}

	cpu_6502.executing.Store(true)
	defer cpu_6502.executing.Store(false)
	cpu_6502.ensureOpcodeTableReady()

	if adapter, ok := cpu_6502.memory.(*Bus6502Adapter); ok {
		if mb, ok := adapter.bus.(*MachineBus); ok {
			mb.SealMappings()
		}
	}

	for cpu_6502.running.Load() {
		for range 4096 {
			// Pause at instruction boundary if Reset() or external observer requests it
			if cpu_6502.resetting.Load() {
				cpu_6502.resetAck.Store(true)
				for cpu_6502.resetting.Load() {
					runtime.Gosched()
				}
				cpu_6502.resetAck.Store(false)
				break
			}

			// Check for RDY line hold
			if !cpu_6502.rdyLine.Load() {
				cpu_6502.rdyHold = true
				break
			}
			cpu_6502.rdyHold = false

			// Check for interrupts
			if cpu_6502.nmiPending.Load() {
				cpu_6502.handleInterrupt(NMI_VECTOR, true)
				cpu_6502.nmiPending.Store(false)
			} else if cpu_6502.irqPending.Load() && cpu_6502.SR&INTERRUPT_FLAG == 0 {
				cpu_6502.handleInterrupt(IRQ_VECTOR, false)
				cpu_6502.irqPending.Store(false)
			}

			// Fetch and execute instruction
			opcode := cpu_6502.readByte(cpu_6502.PC)
			cpu_6502.PC++

			// Handle breakpoints
			if cpu_6502.Debug {
				if _, exists := cpu_6502.breakpoints[cpu_6502.PC-1]; exists {
					cpu_6502.breakpointHit <- cpu_6502.PC - 1
					<-cpu_6502.breakpointHit
				}
			}

			// Execute opcode via dispatch table
			cpu_6502.opcodeTable[opcode](cpu_6502)

			if !cpu_6502.running.Load() {
				break
			}

			// Performance monitoring (matching IE32 pattern)
			if cpu_6502.PerfEnabled {
				cpu_6502.InstructionCount++
			}
		}

		if cpu_6502.PerfEnabled {
			now := time.Now()
			if now.Sub(cpu_6502.lastPerfReport) >= time.Second {
				elapsed := now.Sub(cpu_6502.perfStartTime).Seconds()
				if elapsed > 0 {
					ips := float64(cpu_6502.InstructionCount) / elapsed
					mips := ips / 1_000_000
					fmt.Printf("6502: %.2f MIPS (%.0f instructions in %.1fs)\n", mips, float64(cpu_6502.InstructionCount), elapsed)
				}
				cpu_6502.lastPerfReport = now
			}
		}
	}
}

// Step executes a single instruction and returns the number of cycles consumed.
// This is useful for embedding the CPU in other systems that need precise control.
func (cpu_6502 *CPU_6502) Step() int {
	cpu_6502.ensureOpcodeTableReady()
	if adapter, ok := cpu_6502.memory.(*Bus6502Adapter); ok {
		if mb, ok := adapter.bus.(*MachineBus); ok {
			mb.SealMappings()
		}
	}

	if !cpu_6502.running.Load() || cpu_6502.resetting.Load() {
		return 0
	}

	// Check for RDY line hold
	if !cpu_6502.rdyLine.Load() {
		cpu_6502.rdyHold = true
		return 0
	}
	cpu_6502.rdyHold = false

	// Check for interrupts
	if cpu_6502.nmiPending.Load() {
		cpu_6502.handleInterrupt(NMI_VECTOR, true)
		cpu_6502.nmiPending.Store(false)
	} else if cpu_6502.irqPending.Load() && cpu_6502.SR&INTERRUPT_FLAG == 0 {
		cpu_6502.handleInterrupt(IRQ_VECTOR, false)
		cpu_6502.irqPending.Store(false)
	}

	startCycles := cpu_6502.Cycles

	// Fetch opcode
	opcode := cpu_6502.readByte(cpu_6502.PC)
	cpu_6502.PC++

	// Execute the instruction using the same dispatch table as Execute()
	cpu_6502.opcodeTable[opcode](cpu_6502)

	return int(cpu_6502.Cycles - startCycles)
}

// executeOpcodeSwitch executes a single opcode through the legacy switch.
func (cpu_6502 *CPU_6502) executeOpcodeSwitch(opcode byte) {
	switch opcode {

	// Load/Store Operations
	case 0xA9: // LDA Immediate
		cpu_6502.A = cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 2

	case 0xA5: // LDA Zero Page
		cpu_6502.A = cpu_6502.readByte(cpu_6502.getZeroPage())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 3

	case 0xB5: // LDA Zero Page,X
		cpu_6502.A = cpu_6502.readByte(cpu_6502.getZeroPageX())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0xAD: // LDA Absolute
		cpu_6502.A = cpu_6502.readByte(cpu_6502.getAbsolute())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0xBD: // LDA Absolute,X
		addr, crossed := cpu_6502.getAbsoluteX()
		cpu_6502.A = cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xB9: // LDA Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		cpu_6502.A = cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xA1: // LDA (Indirect,X)
		cpu_6502.A = cpu_6502.readByte(cpu_6502.getIndirectX())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0xB1: // LDA (Indirect),Y
		addr, crossed := cpu_6502.getIndirectY()
		cpu_6502.A = cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 5
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xA2: // LDX Immediate
		cpu_6502.X = cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 2

	case 0xA6: // LDX Zero Page
		cpu_6502.X = cpu_6502.readByte(cpu_6502.getZeroPage())
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 3

	case 0xB6: // LDX Zero Page,Y
		cpu_6502.X = cpu_6502.readByte(cpu_6502.getZeroPageY())
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 4

	case 0xAE: // LDX Absolute
		cpu_6502.X = cpu_6502.readByte(cpu_6502.getAbsolute())
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 4

	case 0xBE: // LDX Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		cpu_6502.X = cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xA0: // LDY Immediate
		cpu_6502.Y = cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		cpu_6502.updateNZ(cpu_6502.Y)
		cpu_6502.Cycles += 2

	case 0xA4: // LDY Zero Page
		cpu_6502.Y = cpu_6502.readByte(cpu_6502.getZeroPage())
		cpu_6502.updateNZ(cpu_6502.Y)
		cpu_6502.Cycles += 3

	case 0xB4: // LDY Zero Page,X
		cpu_6502.Y = cpu_6502.readByte(cpu_6502.getZeroPageX())
		cpu_6502.updateNZ(cpu_6502.Y)
		cpu_6502.Cycles += 4

	case 0xAC: // LDY Absolute
		cpu_6502.Y = cpu_6502.readByte(cpu_6502.getAbsolute())
		cpu_6502.updateNZ(cpu_6502.Y)
		cpu_6502.Cycles += 4

	case 0xBC: // LDY Absolute,X
		addr, crossed := cpu_6502.getAbsoluteX()
		cpu_6502.Y = cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.Y)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	// Store operations
	case 0x85: // STA Zero Page
		cpu_6502.writeByte(cpu_6502.getZeroPage(), cpu_6502.A)
		cpu_6502.Cycles += 3

	case 0x95: // STA Zero Page,X
		cpu_6502.writeByte(cpu_6502.getZeroPageX(), cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0x8D: // STA Absolute
		cpu_6502.writeByte(cpu_6502.getAbsolute(), cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0x9D: // STA Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		cpu_6502.writeByte(addr, cpu_6502.A)
		cpu_6502.Cycles += 5

	case 0x99: // STA Absolute,Y
		addr, _ := cpu_6502.getAbsoluteY()
		cpu_6502.writeByte(addr, cpu_6502.A)
		cpu_6502.Cycles += 5

	case 0x81: // STA (Indirect,X)
		cpu_6502.writeByte(cpu_6502.getIndirectX(), cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x91: // STA (Indirect),Y
		addr, _ := cpu_6502.getIndirectY()
		cpu_6502.writeByte(addr, cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x86: // STX Zero Page
		cpu_6502.writeByte(cpu_6502.getZeroPage(), cpu_6502.X)
		cpu_6502.Cycles += 3

	case 0x96: // STX Zero Page,Y
		cpu_6502.writeByte(cpu_6502.getZeroPageY(), cpu_6502.X)
		cpu_6502.Cycles += 4

	case 0x8E: // STX Absolute
		cpu_6502.writeByte(cpu_6502.getAbsolute(), cpu_6502.X)
		cpu_6502.Cycles += 4

	case 0x84: // STY Zero Page
		cpu_6502.writeByte(cpu_6502.getZeroPage(), cpu_6502.Y)
		cpu_6502.Cycles += 3

	case 0x94: // STY Zero Page,X
		cpu_6502.writeByte(cpu_6502.getZeroPageX(), cpu_6502.Y)
		cpu_6502.Cycles += 4

	case 0x8C: // STY Absolute
		cpu_6502.writeByte(cpu_6502.getAbsolute(), cpu_6502.Y)
		cpu_6502.Cycles += 4

	// Register Transfer Operations
	case 0xAA: // TAX
		cpu_6502.X = cpu_6502.A
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 2

	case 0x8A: // TXA
		cpu_6502.A = cpu_6502.X
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 2

	case 0xA8: // TAY
		cpu_6502.Y = cpu_6502.A
		cpu_6502.updateNZ(cpu_6502.Y)
		cpu_6502.Cycles += 2

	case 0x98: // TYA
		cpu_6502.A = cpu_6502.Y
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 2

	case 0xBA: // TSX
		cpu_6502.X = cpu_6502.SP
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 2

	case 0x9A: // TXS
		cpu_6502.SP = cpu_6502.X
		cpu_6502.Cycles += 2

	// Stack Operations
	case 0x48: // PHA
		cpu_6502.push(cpu_6502.A)
		cpu_6502.Cycles += 3

	case 0x68: // PLA
		cpu_6502.A = cpu_6502.pop()
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0x08: // PHP
		cpu_6502.push(cpu_6502.SR | BREAK_FLAG | UNUSED_FLAG)
		cpu_6502.Cycles += 3

	case 0x28: // PLP
		cpu_6502.SR = (cpu_6502.pop() & 0xEF) | UNUSED_FLAG
		cpu_6502.Cycles += 4

	// Arithmetic Operations
	case 0x69: // ADC Immediate
		cpu_6502.adc(cpu_6502.readByte(cpu_6502.PC))
		cpu_6502.PC++
		cpu_6502.Cycles += 2

	case 0x65: // ADC Zero Page
		cpu_6502.adc(cpu_6502.readByte(cpu_6502.getZeroPage()))
		cpu_6502.Cycles += 3

	case 0x75: // ADC Zero Page,X
		cpu_6502.adc(cpu_6502.readByte(cpu_6502.getZeroPageX()))
		cpu_6502.Cycles += 4

	case 0x6D: // ADC Absolute
		cpu_6502.adc(cpu_6502.readByte(cpu_6502.getAbsolute()))
		cpu_6502.Cycles += 4

	case 0x7D: // ADC Absolute,X
		addr, crossed := cpu_6502.getAbsoluteX()
		cpu_6502.adc(cpu_6502.readByte(addr))
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x79: // ADC Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		cpu_6502.adc(cpu_6502.readByte(addr))
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x61: // ADC (Indirect,X)
		cpu_6502.adc(cpu_6502.readByte(cpu_6502.getIndirectX()))
		cpu_6502.Cycles += 6

	case 0x71: // ADC (Indirect),Y
		addr, crossed := cpu_6502.getIndirectY()
		cpu_6502.adc(cpu_6502.readByte(addr))
		cpu_6502.Cycles += 5
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xE9: // SBC Immediate
		cpu_6502.sbc(cpu_6502.readByte(cpu_6502.PC))
		cpu_6502.PC++
		cpu_6502.Cycles += 2

	case 0xE5: // SBC Zero Page
		cpu_6502.sbc(cpu_6502.readByte(cpu_6502.getZeroPage()))
		cpu_6502.Cycles += 3

	case 0xF5: // SBC Zero Page,X
		cpu_6502.sbc(cpu_6502.readByte(cpu_6502.getZeroPageX()))
		cpu_6502.Cycles += 4

	case 0xED: // SBC Absolute
		cpu_6502.sbc(cpu_6502.readByte(cpu_6502.getAbsolute()))
		cpu_6502.Cycles += 4

	case 0xFD: // SBC Absolute,X
		addr, crossed := cpu_6502.getAbsoluteX()
		cpu_6502.sbc(cpu_6502.readByte(addr))
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xF9: // SBC Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		cpu_6502.sbc(cpu_6502.readByte(addr))
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xE1: // SBC (Indirect,X)
		cpu_6502.sbc(cpu_6502.readByte(cpu_6502.getIndirectX()))
		cpu_6502.Cycles += 6

	case 0xF1: // SBC (Indirect),Y
		addr, crossed := cpu_6502.getIndirectY()
		cpu_6502.sbc(cpu_6502.readByte(addr))
		cpu_6502.Cycles += 5
		if crossed {
			cpu_6502.Cycles++
		}

	// Increment/Decrement Operations
	case 0xE6: // INC Zero Page
		cpu_6502.inc(cpu_6502.getZeroPage())
		cpu_6502.Cycles += 5

	case 0xF6: // INC Zero Page,X
		cpu_6502.inc(cpu_6502.getZeroPageX())
		cpu_6502.Cycles += 6

	case 0xEE: // INC Absolute
		cpu_6502.inc(cpu_6502.getAbsolute())
		cpu_6502.Cycles += 6

	case 0xFE: // INC Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		cpu_6502.inc(addr)
		cpu_6502.Cycles += 7

	case 0xC6: // DEC Zero Page
		cpu_6502.dec(cpu_6502.getZeroPage())
		cpu_6502.Cycles += 5

	case 0xD6: // DEC Zero Page,X
		cpu_6502.dec(cpu_6502.getZeroPageX())
		cpu_6502.Cycles += 6

	case 0xCE: // DEC Absolute
		cpu_6502.dec(cpu_6502.getAbsolute())
		cpu_6502.Cycles += 6

	case 0xDE: // DEC Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		cpu_6502.dec(addr)
		cpu_6502.Cycles += 7

	case 0xE8: // INX
		cpu_6502.X++
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 2

	case 0xC8: // INY
		cpu_6502.Y++
		cpu_6502.updateNZ(cpu_6502.Y)
		cpu_6502.Cycles += 2

	case 0xCA: // DEX
		cpu_6502.X--
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 2

	case 0x88: // DEY
		cpu_6502.Y--
		cpu_6502.updateNZ(cpu_6502.Y)
		cpu_6502.Cycles += 2

	// Logical Operations
	case 0x29: // AND Immediate
		cpu_6502.A &= cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 2

	case 0x25: // AND Zero Page
		cpu_6502.A &= cpu_6502.readByte(cpu_6502.getZeroPage())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 3

	case 0x35: // AND Zero Page,X
		cpu_6502.A &= cpu_6502.readByte(cpu_6502.getZeroPageX())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0x2D: // AND Absolute
		cpu_6502.A &= cpu_6502.readByte(cpu_6502.getAbsolute())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0x3D: // AND Absolute,X
		addr, crossed := cpu_6502.getAbsoluteX()
		cpu_6502.A &= cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x39: // AND Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		cpu_6502.A &= cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x21: // AND (Indirect,X)
		cpu_6502.A &= cpu_6502.readByte(cpu_6502.getIndirectX())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x31: // AND (Indirect),Y
		addr, crossed := cpu_6502.getIndirectY()
		cpu_6502.A &= cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 5
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x09: // ORA Immediate
		cpu_6502.A |= cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 2

	case 0x05: // ORA Zero Page
		cpu_6502.A |= cpu_6502.readByte(cpu_6502.getZeroPage())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 3

	case 0x15: // ORA Zero Page,X
		cpu_6502.A |= cpu_6502.readByte(cpu_6502.getZeroPageX())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0x0D: // ORA Absolute
		cpu_6502.A |= cpu_6502.readByte(cpu_6502.getAbsolute())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0x1D: // ORA Absolute,X
		addr, crossed := cpu_6502.getAbsoluteX()
		cpu_6502.A |= cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x19: // ORA Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		cpu_6502.A |= cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x01: // ORA (Indirect,X)
		cpu_6502.A |= cpu_6502.readByte(cpu_6502.getIndirectX())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x11: // ORA (Indirect),Y
		addr, crossed := cpu_6502.getIndirectY()
		cpu_6502.A |= cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 5
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x49: // EOR Immediate
		cpu_6502.A ^= cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 2

	case 0x45: // EOR Zero Page
		cpu_6502.A ^= cpu_6502.readByte(cpu_6502.getZeroPage())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 3

	case 0x55: // EOR Zero Page,X
		cpu_6502.A ^= cpu_6502.readByte(cpu_6502.getZeroPageX())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0x4D: // EOR Absolute
		cpu_6502.A ^= cpu_6502.readByte(cpu_6502.getAbsolute())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0x5D: // EOR Absolute,X
		addr, crossed := cpu_6502.getAbsoluteX()
		cpu_6502.A ^= cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x59: // EOR Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		cpu_6502.A ^= cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x41: // EOR (Indirect,X)
		cpu_6502.A ^= cpu_6502.readByte(cpu_6502.getIndirectX())
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x51: // EOR (Indirect),Y
		addr, crossed := cpu_6502.getIndirectY()
		cpu_6502.A ^= cpu_6502.readByte(addr)
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 5
		if crossed {
			cpu_6502.Cycles++
		}

	// Bit Operations
	case 0x24: // BIT Zero Page
		value := cpu_6502.readByte(cpu_6502.getZeroPage())
		cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A&value == 0)
		cpu_6502.setFlag(OVERFLOW_FLAG, value&0x40 != 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
		cpu_6502.Cycles += 3

	case 0x2C: // BIT Absolute
		value := cpu_6502.readByte(cpu_6502.getAbsolute())
		cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A&value == 0)
		cpu_6502.setFlag(OVERFLOW_FLAG, value&0x40 != 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
		cpu_6502.Cycles += 4

	// Shift/Rotate Operations
	case 0x0A: // ASL Accumulator
		cpu_6502.asl(0, true)
		cpu_6502.Cycles += 2

	case 0x06: // ASL Zero Page
		cpu_6502.asl(cpu_6502.getZeroPage(), false)
		cpu_6502.Cycles += 5

	case 0x16: // ASL Zero Page,X
		cpu_6502.asl(cpu_6502.getZeroPageX(), false)
		cpu_6502.Cycles += 6

	case 0x0E: // ASL Absolute
		cpu_6502.asl(cpu_6502.getAbsolute(), false)
		cpu_6502.Cycles += 6

	case 0x1E: // ASL Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		cpu_6502.asl(addr, false)
		cpu_6502.Cycles += 7

	case 0x4A: // LSR Accumulator
		cpu_6502.lsr(0, true)
		cpu_6502.Cycles += 2

	case 0x46: // LSR Zero Page
		cpu_6502.lsr(cpu_6502.getZeroPage(), false)
		cpu_6502.Cycles += 5

	case 0x56: // LSR Zero Page,X
		cpu_6502.lsr(cpu_6502.getZeroPageX(), false)
		cpu_6502.Cycles += 6

	case 0x4E: // LSR Absolute
		cpu_6502.lsr(cpu_6502.getAbsolute(), false)
		cpu_6502.Cycles += 6

	case 0x5E: // LSR Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		cpu_6502.lsr(addr, false)
		cpu_6502.Cycles += 7

	case 0x2A: // ROL Accumulator
		cpu_6502.rol(0, true)
		cpu_6502.Cycles += 2

	case 0x26: // ROL Zero Page
		cpu_6502.rol(cpu_6502.getZeroPage(), false)
		cpu_6502.Cycles += 5

	case 0x36: // ROL Zero Page,X
		cpu_6502.rol(cpu_6502.getZeroPageX(), false)
		cpu_6502.Cycles += 6

	case 0x2E: // ROL Absolute
		cpu_6502.rol(cpu_6502.getAbsolute(), false)
		cpu_6502.Cycles += 6

	case 0x3E: // ROL Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		cpu_6502.rol(addr, false)
		cpu_6502.Cycles += 7

	case 0x6A: // ROR Accumulator
		cpu_6502.ror(0, true)
		cpu_6502.Cycles += 2

	case 0x66: // ROR Zero Page
		cpu_6502.ror(cpu_6502.getZeroPage(), false)
		cpu_6502.Cycles += 5

	case 0x76: // ROR Zero Page,X
		cpu_6502.ror(cpu_6502.getZeroPageX(), false)
		cpu_6502.Cycles += 6

	case 0x6E: // ROR Absolute
		cpu_6502.ror(cpu_6502.getAbsolute(), false)
		cpu_6502.Cycles += 6

	case 0x7E: // ROR Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		cpu_6502.ror(addr, false)
		cpu_6502.Cycles += 7

	// Compare Operations
	case 0xC9: // CMP Immediate
		cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.PC))
		cpu_6502.PC++
		cpu_6502.Cycles += 2

	case 0xC5: // CMP Zero Page
		cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.getZeroPage()))
		cpu_6502.Cycles += 3

	case 0xD5: // CMP Zero Page,X
		cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.getZeroPageX()))
		cpu_6502.Cycles += 4

	case 0xCD: // CMP Absolute
		cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.getAbsolute()))
		cpu_6502.Cycles += 4

	case 0xDD: // CMP Absolute,X
		addr, crossed := cpu_6502.getAbsoluteX()
		cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(addr))
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xD9: // CMP Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(addr))
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xC1: // CMP (Indirect,X)
		cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(cpu_6502.getIndirectX()))
		cpu_6502.Cycles += 6

	case 0xD1: // CMP (Indirect),Y
		addr, crossed := cpu_6502.getIndirectY()
		cpu_6502.compare(cpu_6502.A, cpu_6502.readByte(addr))
		cpu_6502.Cycles += 5
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xE0: // CPX Immediate
		cpu_6502.compare(cpu_6502.X, cpu_6502.readByte(cpu_6502.PC))
		cpu_6502.PC++
		cpu_6502.Cycles += 2

	case 0xE4: // CPX Zero Page
		cpu_6502.compare(cpu_6502.X, cpu_6502.readByte(cpu_6502.getZeroPage()))
		cpu_6502.Cycles += 3

	case 0xEC: // CPX Absolute
		cpu_6502.compare(cpu_6502.X, cpu_6502.readByte(cpu_6502.getAbsolute()))
		cpu_6502.Cycles += 4

	case 0xC0: // CPY Immediate
		cpu_6502.compare(cpu_6502.Y, cpu_6502.readByte(cpu_6502.PC))
		cpu_6502.PC++
		cpu_6502.Cycles += 2

	case 0xC4: // CPY Zero Page
		cpu_6502.compare(cpu_6502.Y, cpu_6502.readByte(cpu_6502.getZeroPage()))
		cpu_6502.Cycles += 3

	case 0xCC: // CPY Absolute
		cpu_6502.compare(cpu_6502.Y, cpu_6502.readByte(cpu_6502.getAbsolute()))
		cpu_6502.Cycles += 4

	// Branch Operations
	case 0x90: // BCC
		cpu_6502.branch(cpu_6502.SR&CARRY_FLAG == 0)

	case 0xB0: // BCS
		cpu_6502.branch(cpu_6502.SR&CARRY_FLAG != 0)

	case 0xF0: // BEQ
		cpu_6502.branch(cpu_6502.SR&ZERO_FLAG != 0)

	case 0xD0: // BNE
		cpu_6502.branch(cpu_6502.SR&ZERO_FLAG == 0)

	case 0x30: // BMI
		cpu_6502.branch(cpu_6502.SR&NEGATIVE_FLAG != 0)

	case 0x10: // BPL
		cpu_6502.branch(cpu_6502.SR&NEGATIVE_FLAG == 0)

	case 0x70: // BVS
		cpu_6502.branch(cpu_6502.SR&OVERFLOW_FLAG != 0)

	case 0x50: // BVC
		cpu_6502.branch(cpu_6502.SR&OVERFLOW_FLAG == 0)

	// Jump/Call Operations
	case 0x4C: // JMP Absolute
		cpu_6502.PC = cpu_6502.getAbsolute()
		cpu_6502.Cycles += 3

	case 0x6C: // JMP Indirect
		low := cpu_6502.readByte(cpu_6502.PC)
		high := cpu_6502.readByte(cpu_6502.PC + 1)
		addr := uint16(low) | uint16(high)<<8
		// 6502 bug: wraps within page
		low2 := cpu_6502.readByte(addr)
		high2 := cpu_6502.readByte((addr & 0xFF00) | ((addr + 1) & 0x00FF))
		cpu_6502.PC = uint16(low2) | uint16(high2)<<8
		cpu_6502.Cycles += 5

	case 0x20: // JSR
		addr := cpu_6502.getAbsolute()
		cpu_6502.push16(cpu_6502.PC - 1)
		cpu_6502.PC = addr
		cpu_6502.Cycles += 6

	case 0x60: // RTS
		cpu_6502.PC = cpu_6502.pop16() + 1
		cpu_6502.Cycles += 6

	// Flag Operations
	case 0x18: // CLC
		cpu_6502.SR &^= CARRY_FLAG
		cpu_6502.Cycles += 2

	case 0x38: // SEC
		cpu_6502.SR |= CARRY_FLAG
		cpu_6502.Cycles += 2

	case 0x58: // CLI
		cpu_6502.SR &^= INTERRUPT_FLAG
		cpu_6502.Cycles += 2

	case 0x78: // SEI
		cpu_6502.SR |= INTERRUPT_FLAG
		cpu_6502.Cycles += 2

	case 0xB8: // CLV
		cpu_6502.SR &^= OVERFLOW_FLAG
		cpu_6502.Cycles += 2

	case 0xD8: // CLD
		cpu_6502.SR &^= DECIMAL_FLAG
		cpu_6502.Cycles += 2

	case 0xF8: // SED
		cpu_6502.SR |= DECIMAL_FLAG
		cpu_6502.Cycles += 2

	// Special Operations
	case 0x00: // BRK
		cpu_6502.PC++
		cpu_6502.push16(cpu_6502.PC)
		cpu_6502.push(cpu_6502.SR | BREAK_FLAG | UNUSED_FLAG)
		cpu_6502.setFlag(INTERRUPT_FLAG, true)
		cpu_6502.SR &= ^byte(BREAK_FLAG)
		cpu_6502.PC = cpu_6502.read16(IRQ_VECTOR)
		cpu_6502.Cycles += 7

	case 0x40: // RTI
		cpu_6502.SR = (cpu_6502.pop() & 0xEF) | UNUSED_FLAG
		cpu_6502.PC = cpu_6502.pop16()
		cpu_6502.Cycles += 6

	case 0xEA: // NOP
		cpu_6502.Cycles += 2

	// Unofficial NOPs
	case 0x1A, 0x3A, 0x5A, 0x7A, 0xDA, 0xFA: // NOP (implied)
		cpu_6502.Cycles += 2

	case 0x80, 0x82, 0x89, 0xC2, 0xE2: // NOP Immediate
		cpu_6502.PC++
		cpu_6502.Cycles += 2

	case 0x04, 0x44, 0x64: // NOP Zero Page
		cpu_6502.PC++
		cpu_6502.Cycles += 3

	case 0x14, 0x34, 0x54, 0x74, 0xD4, 0xF4: // NOP Zero Page,X
		cpu_6502.PC++
		cpu_6502.Cycles += 4

	case 0x0C: // NOP Absolute
		cpu_6502.PC += 2
		cpu_6502.Cycles += 4

	case 0x1C, 0x3C, 0x5C, 0x7C, 0xDC, 0xFC: // NOP Absolute,X
		addr, crossed := cpu_6502.getAbsoluteX()
		_ = cpu_6502.readByte(addr)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	// Unofficial opcodes
	case 0xA7: // LAX Zero Page
		cpu_6502.A = cpu_6502.readByte(cpu_6502.getZeroPage())
		cpu_6502.X = cpu_6502.A
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 3

	case 0xB7: // LAX Zero Page,Y
		cpu_6502.A = cpu_6502.readByte(cpu_6502.getZeroPageY())
		cpu_6502.X = cpu_6502.A
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0xAF: // LAX Absolute
		cpu_6502.A = cpu_6502.readByte(cpu_6502.getAbsolute())
		cpu_6502.X = cpu_6502.A
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4

	case 0xBF: // LAX Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		cpu_6502.A = cpu_6502.readByte(addr)
		cpu_6502.X = cpu_6502.A
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xA3: // LAX (Indirect,X)
		cpu_6502.A = cpu_6502.readByte(cpu_6502.getIndirectX())
		cpu_6502.X = cpu_6502.A
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0xB3: // LAX (Indirect),Y
		addr, crossed := cpu_6502.getIndirectY()
		cpu_6502.A = cpu_6502.readByte(addr)
		cpu_6502.X = cpu_6502.A
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 5
		if crossed {
			cpu_6502.Cycles++
		}

	case 0x87: // SAX Zero Page
		cpu_6502.writeByte(cpu_6502.getZeroPage(), cpu_6502.A&cpu_6502.X)
		cpu_6502.Cycles += 3

	case 0x97: // SAX Zero Page,Y
		cpu_6502.writeByte(cpu_6502.getZeroPageY(), cpu_6502.A&cpu_6502.X)
		cpu_6502.Cycles += 4

	case 0x8F: // SAX Absolute
		cpu_6502.writeByte(cpu_6502.getAbsolute(), cpu_6502.A&cpu_6502.X)
		cpu_6502.Cycles += 4

	case 0x83: // SAX (Indirect,X)
		cpu_6502.writeByte(cpu_6502.getIndirectX(), cpu_6502.A&cpu_6502.X)
		cpu_6502.Cycles += 6

	case 0xEB: // SBC Immediate (unofficial)
		cpu_6502.sbc(cpu_6502.readByte(cpu_6502.PC))
		cpu_6502.PC++
		cpu_6502.Cycles += 2

	case 0xC7: // DCP Zero Page
		addr := cpu_6502.getZeroPage()
		val := cpu_6502.readByte(addr) - 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.compare(cpu_6502.A, val)
		cpu_6502.Cycles += 5

	case 0xD7: // DCP Zero Page,X
		addr := cpu_6502.getZeroPageX()
		val := cpu_6502.readByte(addr) - 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.compare(cpu_6502.A, val)
		cpu_6502.Cycles += 6

	case 0xCF: // DCP Absolute
		addr := cpu_6502.getAbsolute()
		val := cpu_6502.readByte(addr) - 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.compare(cpu_6502.A, val)
		cpu_6502.Cycles += 6

	case 0xDF: // DCP Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		val := cpu_6502.readByte(addr) - 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.compare(cpu_6502.A, val)
		cpu_6502.Cycles += 7

	case 0xDB: // DCP Absolute,Y
		addr, _ := cpu_6502.getAbsoluteY()
		val := cpu_6502.readByte(addr) - 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.compare(cpu_6502.A, val)
		cpu_6502.Cycles += 7

	case 0xC3: // DCP (Indirect,X)
		addr := cpu_6502.getIndirectX()
		val := cpu_6502.readByte(addr) - 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.compare(cpu_6502.A, val)
		cpu_6502.Cycles += 8

	case 0xD3: // DCP (Indirect),Y
		addr, _ := cpu_6502.getIndirectY()
		val := cpu_6502.readByte(addr) - 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.compare(cpu_6502.A, val)
		cpu_6502.Cycles += 8

	case 0xE7: // ISC Zero Page
		addr := cpu_6502.getZeroPage()
		val := cpu_6502.readByte(addr) + 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.sbc(val)
		cpu_6502.Cycles += 5

	case 0xF7: // ISC Zero Page,X
		addr := cpu_6502.getZeroPageX()
		val := cpu_6502.readByte(addr) + 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.sbc(val)
		cpu_6502.Cycles += 6

	case 0xEF: // ISC Absolute
		addr := cpu_6502.getAbsolute()
		val := cpu_6502.readByte(addr) + 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.sbc(val)
		cpu_6502.Cycles += 6

	case 0xFF: // ISC Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		val := cpu_6502.readByte(addr) + 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.sbc(val)
		cpu_6502.Cycles += 7

	case 0xFB: // ISC Absolute,Y
		addr, _ := cpu_6502.getAbsoluteY()
		val := cpu_6502.readByte(addr) + 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.sbc(val)
		cpu_6502.Cycles += 7

	case 0xE3: // ISC (Indirect,X)
		addr := cpu_6502.getIndirectX()
		val := cpu_6502.readByte(addr) + 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.sbc(val)
		cpu_6502.Cycles += 8

	case 0xF3: // ISC (Indirect),Y
		addr, _ := cpu_6502.getIndirectY()
		val := cpu_6502.readByte(addr) + 1
		cpu_6502.writeByte(addr, val)
		cpu_6502.sbc(val)
		cpu_6502.Cycles += 8

	case 0x07: // SLO Zero Page
		addr := cpu_6502.getZeroPage()
		val := cpu_6502.asl(addr, false)
		cpu_6502.A |= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 5

	case 0x17: // SLO Zero Page,X
		addr := cpu_6502.getZeroPageX()
		val := cpu_6502.asl(addr, false)
		cpu_6502.A |= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x0F: // SLO Absolute
		addr := cpu_6502.getAbsolute()
		val := cpu_6502.asl(addr, false)
		cpu_6502.A |= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x1F: // SLO Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		val := cpu_6502.asl(addr, false)
		cpu_6502.A |= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 7

	case 0x1B: // SLO Absolute,Y
		addr, _ := cpu_6502.getAbsoluteY()
		val := cpu_6502.asl(addr, false)
		cpu_6502.A |= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 7

	case 0x03: // SLO (Indirect,X)
		addr := cpu_6502.getIndirectX()
		val := cpu_6502.asl(addr, false)
		cpu_6502.A |= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 8

	case 0x13: // SLO (Indirect),Y
		addr, _ := cpu_6502.getIndirectY()
		val := cpu_6502.asl(addr, false)
		cpu_6502.A |= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 8

	case 0x27: // RLA Zero Page
		addr := cpu_6502.getZeroPage()
		val := cpu_6502.rol(addr, false)
		cpu_6502.A &= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 5

	case 0x37: // RLA Zero Page,X
		addr := cpu_6502.getZeroPageX()
		val := cpu_6502.rol(addr, false)
		cpu_6502.A &= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x2F: // RLA Absolute
		addr := cpu_6502.getAbsolute()
		val := cpu_6502.rol(addr, false)
		cpu_6502.A &= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x3F: // RLA Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		val := cpu_6502.rol(addr, false)
		cpu_6502.A &= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 7

	case 0x3B: // RLA Absolute,Y
		addr, _ := cpu_6502.getAbsoluteY()
		val := cpu_6502.rol(addr, false)
		cpu_6502.A &= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 7

	case 0x23: // RLA (Indirect,X)
		addr := cpu_6502.getIndirectX()
		val := cpu_6502.rol(addr, false)
		cpu_6502.A &= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 8

	case 0x33: // RLA (Indirect),Y
		addr, _ := cpu_6502.getIndirectY()
		val := cpu_6502.rol(addr, false)
		cpu_6502.A &= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 8

	case 0x47: // SRE Zero Page
		addr := cpu_6502.getZeroPage()
		val := cpu_6502.lsr(addr, false)
		cpu_6502.A ^= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 5

	case 0x57: // SRE Zero Page,X
		addr := cpu_6502.getZeroPageX()
		val := cpu_6502.lsr(addr, false)
		cpu_6502.A ^= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x4F: // SRE Absolute
		addr := cpu_6502.getAbsolute()
		val := cpu_6502.lsr(addr, false)
		cpu_6502.A ^= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 6

	case 0x5F: // SRE Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		val := cpu_6502.lsr(addr, false)
		cpu_6502.A ^= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 7

	case 0x5B: // SRE Absolute,Y
		addr, _ := cpu_6502.getAbsoluteY()
		val := cpu_6502.lsr(addr, false)
		cpu_6502.A ^= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 7

	case 0x43: // SRE (Indirect,X)
		addr := cpu_6502.getIndirectX()
		val := cpu_6502.lsr(addr, false)
		cpu_6502.A ^= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 8

	case 0x53: // SRE (Indirect),Y
		addr, _ := cpu_6502.getIndirectY()
		val := cpu_6502.lsr(addr, false)
		cpu_6502.A ^= val
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 8

	case 0x67: // RRA Zero Page
		addr := cpu_6502.getZeroPage()
		val := cpu_6502.ror(addr, false)
		cpu_6502.adc(val)
		cpu_6502.Cycles += 5

	case 0x77: // RRA Zero Page,X
		addr := cpu_6502.getZeroPageX()
		val := cpu_6502.ror(addr, false)
		cpu_6502.adc(val)
		cpu_6502.Cycles += 6

	case 0x6F: // RRA Absolute
		addr := cpu_6502.getAbsolute()
		val := cpu_6502.ror(addr, false)
		cpu_6502.adc(val)
		cpu_6502.Cycles += 6

	case 0x7F: // RRA Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		val := cpu_6502.ror(addr, false)
		cpu_6502.adc(val)
		cpu_6502.Cycles += 7

	case 0x7B: // RRA Absolute,Y
		addr, _ := cpu_6502.getAbsoluteY()
		val := cpu_6502.ror(addr, false)
		cpu_6502.adc(val)
		cpu_6502.Cycles += 7

	case 0x63: // RRA (Indirect,X)
		addr := cpu_6502.getIndirectX()
		val := cpu_6502.ror(addr, false)
		cpu_6502.adc(val)
		cpu_6502.Cycles += 8

	case 0x73: // RRA (Indirect),Y
		addr, _ := cpu_6502.getIndirectY()
		val := cpu_6502.ror(addr, false)
		cpu_6502.adc(val)
		cpu_6502.Cycles += 8

	// ANC
	case 0x0B, 0x2B: // ANC Immediate
		cpu_6502.A &= cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x80 != 0)
		cpu_6502.Cycles += 2

	case 0x4B: // ALR Immediate
		cpu_6502.A &= cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&1 != 0)
		cpu_6502.A >>= 1
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.Cycles += 2

	case 0x6B: // ARR Immediate
		cpu_6502.A &= cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		carry := cpu_6502.SR&CARRY_FLAG != 0
		carryBit := byte(0)
		if carry {
			carryBit = 0x80
		}
		cpu_6502.A = (cpu_6502.A >> 1) | carryBit
		cpu_6502.updateNZ(cpu_6502.A)
		cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x40 != 0)
		cpu_6502.setFlag(OVERFLOW_FLAG, ((cpu_6502.A>>6)^(cpu_6502.A>>5))&1 != 0)
		cpu_6502.Cycles += 2

	case 0xCB: // AXS Immediate
		val := cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		result := int(cpu_6502.A&cpu_6502.X) - int(val)
		cpu_6502.X = byte(result)
		cpu_6502.setFlag(CARRY_FLAG, result >= 0)
		cpu_6502.updateNZ(cpu_6502.X)
		cpu_6502.Cycles += 2

	case 0x9F: // SHA Absolute,Y
		addr, _ := cpu_6502.getAbsoluteY()
		value := cpu_6502.A & cpu_6502.X & byte((addr>>8)+1)
		cpu_6502.writeByte(addr, value)
		cpu_6502.Cycles += 5

	case 0x93: // SHA (Indirect),Y
		addr, _ := cpu_6502.getIndirectY()
		value := cpu_6502.A & cpu_6502.X & byte((addr>>8)+1)
		cpu_6502.writeByte(addr, value)
		cpu_6502.Cycles += 6

	case 0x9E: // SHX Absolute,Y
		addr, _ := cpu_6502.getAbsoluteY()
		value := cpu_6502.X & byte((addr>>8)+1)
		if (addr&0xFF)+uint16(cpu_6502.Y) > 0xFF {
			value &= byte(addr >> 8)
		}
		cpu_6502.writeByte(addr, value)
		cpu_6502.Cycles += 5

	case 0x9C: // SHY Absolute,X
		addr, _ := cpu_6502.getAbsoluteX()
		value := cpu_6502.Y & byte((addr>>8)+1)
		if (addr&0xFF)+uint16(cpu_6502.X) > 0xFF {
			value &= byte(addr >> 8)
		}
		cpu_6502.writeByte(addr, value)
		cpu_6502.Cycles += 5

	case 0x9B: // TAS Absolute,Y
		cpu_6502.SP = cpu_6502.A & cpu_6502.X
		addr, _ := cpu_6502.getAbsoluteY()
		value := cpu_6502.SP & byte((addr>>8)+1)
		cpu_6502.writeByte(addr, value)
		cpu_6502.Cycles += 5

	case 0xBB: // LAS Absolute,Y
		addr, crossed := cpu_6502.getAbsoluteY()
		value := cpu_6502.readByte(addr) & cpu_6502.SP
		cpu_6502.A = value
		cpu_6502.X = value
		cpu_6502.SP = value
		cpu_6502.updateNZ(value)
		cpu_6502.Cycles += 4
		if crossed {
			cpu_6502.Cycles++
		}

	case 0xAB: // LAX Immediate (unstable)
		val := cpu_6502.readByte(cpu_6502.PC)
		cpu_6502.PC++
		cpu_6502.A = val
		cpu_6502.X = val
		cpu_6502.updateNZ(val)
		cpu_6502.Cycles += 2

	case 0x02, 0x12, 0x22, 0x32, 0x42, 0x52, 0x62, 0x72, 0x92, 0xB2, 0xD2, 0xF2: // KIL (halt)
		cpu_6502.running.Store(false)

	default:
		fmt.Printf("Unknown opcode: %02X at PC=%04X\n", opcode, cpu_6502.PC-1)
		cpu_6502.running.Store(false)
	}
}

// translateIO8Bit_6502 converts 16-bit I/O addresses (0xF000-0xFFFF) to
// 32-bit addresses (0xF0000-0xF0FFF) for 6502 compatibility.
// translateIO8Bit_6502 converts 16-bit I/O addresses (0xF000-0xFFF9) to
// 32-bit addresses (0xF0000-0xF0FF9) for 6502 compatibility.
// Vector addresses (0xFFFA-0xFFFF) are NOT translated to preserve
// the 6502's standard NMI/RESET/IRQ vector locations.
func translateIO8Bit_6502(addr uint16) uint32 {
	// Don't translate vector area - preserve standard 6502 vectors
	if addr >= 0xFFFA {
		return uint32(addr)
	}
	// Translate I/O region to 32-bit space
	if addr >= 0xF000 {
		return 0xF0000 + uint32(addr-0xF000)
	}
	return uint32(addr)
}

func (adapter *Bus6502Adapter) Read(addr uint16) byte {
	/*
	   Read performs 8-bit memory read.

	   Parameters:
	   - addr: Memory address

	   Returns:
	   - byte: Value at address

	   Operation:
	   1. Check for bank register reads
	   2. Check for banked memory access
	   3. Fall back to direct memory access
	*/

	if h := adapter.ioTable[addr>>8]; h.read != nil {
		return h.read(adapter, addr)
	}

	// Handle VRAM bank register reads
	if addr == VRAM_BANK_REG {
		return byte(adapter.vramBank & 0xFF)
	}
	if addr == VRAM_BANK_REG_RSVD {
		return 0
	}

	// Handle extended bank register reads
	switch addr {
	case BANK1_REG_LO:
		return byte(adapter.bank1 & 0xFF)
	case BANK1_REG_HI:
		return byte((adapter.bank1 >> 8) & 0xFF)
	case BANK2_REG_LO:
		return byte(adapter.bank2 & 0xFF)
	case BANK2_REG_HI:
		return byte((adapter.bank2 >> 8) & 0xFF)
	case BANK3_REG_LO:
		return byte(adapter.bank3 & 0xFF)
	case BANK3_REG_HI:
		return byte((adapter.bank3 >> 8) & 0xFF)
	}

	// Handle PSG register reads (C64 SID-style mapping at $D400-$D40D)
	if addr >= C6502_PSG_BASE && addr <= C6502_PSG_END {
		psgReg := uint32(addr - C6502_PSG_BASE)
		return adapter.bus.Read8(PSG_BASE + psgReg)
	}

	// Handle SID register reads ($D500-$D51C)
	if addr >= C6502_SID_BASE && addr <= C6502_SID_END {
		sidReg := uint32(addr - C6502_SID_BASE)
		return adapter.bus.Read8(SID_BASE + sidReg)
	}

	// Handle POKEY register reads ($D200-$D209)
	if addr >= C6502_POKEY_BASE && addr <= C6502_POKEY_END {
		pokeyReg := uint32(addr - C6502_POKEY_BASE)
		return adapter.bus.Read8(POKEY_BASE + pokeyReg)
	}

	// Handle TED audio register reads ($D600-$D605)
	if addr >= C6502_TED_BASE && addr <= C6502_TED_END {
		tedReg := uint32(addr - C6502_TED_BASE)
		return adapter.bus.Read8(TED_BASE + tedReg)
	}

	// Handle TED video register reads ($D620-$D62F)
	if addr >= C6502_TED_V_BASE && addr <= C6502_TED_V_END {
		tedVReg := uint32(addr - C6502_TED_V_BASE)
		return adapter.bus.Read8(TED_VIDEO_BASE + (tedVReg * 4))
	}

	// Handle ULA register reads ($D800-$D80F)
	if addr >= C6502_ULA_BASE && addr <= C6502_ULA_BASE+0x0F {
		ulaReg := uint32(addr - C6502_ULA_BASE)
		return adapter.bus.Read8(ULA_BASE + ulaReg)
	}

	// Handle VGA register reads ($D700-$D70A)
	if adapter.vgaEngine != nil && addr >= C6502_VGA_BASE && addr <= C6502_VGA_END {
		switch addr {
		case C6502_VGA_MODE:
			return byte(adapter.vgaEngine.HandleRead(VGA_MODE))
		case C6502_VGA_STATUS:
			return byte(adapter.vgaEngine.HandleRead(VGA_STATUS))
		case C6502_VGA_CTRL:
			return byte(adapter.vgaEngine.HandleRead(VGA_CTRL))
		case C6502_VGA_SEQ_IDX:
			return byte(adapter.vgaEngine.HandleRead(VGA_SEQ_INDEX))
		case C6502_VGA_SEQ_DATA:
			return byte(adapter.vgaEngine.HandleRead(VGA_SEQ_DATA))
		case C6502_VGA_CRTC_IDX:
			return byte(adapter.vgaEngine.HandleRead(VGA_CRTC_INDEX))
		case C6502_VGA_CRTC_DATA:
			return byte(adapter.vgaEngine.HandleRead(VGA_CRTC_DATA))
		case C6502_VGA_GC_IDX:
			return byte(adapter.vgaEngine.HandleRead(VGA_GC_INDEX))
		case C6502_VGA_GC_DATA:
			return byte(adapter.vgaEngine.HandleRead(VGA_GC_DATA))
		case C6502_VGA_DAC_WIDX:
			return byte(adapter.vgaEngine.HandleRead(VGA_DAC_WINDEX))
		case C6502_VGA_DAC_DATA:
			return byte(adapter.vgaEngine.HandleRead(VGA_DAC_DATA))
		}
	}

	// Handle extended bank window reads (IE65 mode)
	if translated, ok := adapter.translateExtendedBank(addr); ok {
		return adapter.bus.Read8(translated)
	}

	// Handle VRAM bank window reads
	if translated, ok := adapter.translateVRAM(addr); ok {
		return adapter.bus.Read8(translated)
	}

	return adapter.bus.Read8(translateIO8Bit_6502(addr))
}

func (adapter *Bus6502Adapter) Write(addr uint16, value byte) {
	/*
	   Write performs 8-bit memory write.

	   Parameters:
	   - addr: Target address
	   - value: Byte to write

	   Operation:
	   1. Check for bank register writes
	   2. Check for banked memory access
	   3. Fall back to direct memory access
	*/

	if h := adapter.ioTable[addr>>8]; h.write != nil {
		h.write(adapter, addr, value)
		return
	}

	// Handle VRAM bank register writes
	if addr == VRAM_BANK_REG {
		adapter.vramBank = uint32(value)
		adapter.vramEnabled = true
		return
	}
	if addr == VRAM_BANK_REG_RSVD {
		return
	}

	// Handle extended bank register writes
	switch addr {
	case BANK1_REG_LO:
		adapter.bank1 = (adapter.bank1 & 0xFF00) | uint32(value)
		adapter.bank1Enable = true
		return
	case BANK1_REG_HI:
		adapter.bank1 = (adapter.bank1 & 0x00FF) | (uint32(value) << 8)
		adapter.bank1Enable = true
		return
	case BANK2_REG_LO:
		adapter.bank2 = (adapter.bank2 & 0xFF00) | uint32(value)
		adapter.bank2Enable = true
		return
	case BANK2_REG_HI:
		adapter.bank2 = (adapter.bank2 & 0x00FF) | (uint32(value) << 8)
		adapter.bank2Enable = true
		return
	case BANK3_REG_LO:
		adapter.bank3 = (adapter.bank3 & 0xFF00) | uint32(value)
		adapter.bank3Enable = true
		return
	case BANK3_REG_HI:
		adapter.bank3 = (adapter.bank3 & 0x00FF) | (uint32(value) << 8)
		adapter.bank3Enable = true
		return
	}

	// Handle PSG register writes (C64 SID-style mapping at $D400-$D40D)
	if addr >= C6502_PSG_BASE && addr <= C6502_PSG_END {
		psgReg := uint32(addr - C6502_PSG_BASE)
		adapter.bus.Write8(PSG_BASE+psgReg, value)
		return
	}

	// Handle SID register writes ($D500-$D51C)
	if addr >= C6502_SID_BASE && addr <= C6502_SID_END {
		sidReg := uint32(addr - C6502_SID_BASE)
		adapter.bus.Write8(SID_BASE+sidReg, value)
		return
	}

	// Handle POKEY register writes ($D200-$D209)
	if addr >= C6502_POKEY_BASE && addr <= C6502_POKEY_END {
		pokeyReg := uint32(addr - C6502_POKEY_BASE)
		adapter.bus.Write8(POKEY_BASE+pokeyReg, value)
		return
	}

	// Handle TED audio register writes ($D600-$D605)
	if addr >= C6502_TED_BASE && addr <= C6502_TED_END {
		tedReg := uint32(addr - C6502_TED_BASE)
		adapter.bus.Write8(TED_BASE+tedReg, value)
		return
	}

	// Handle TED video register writes ($D620-$D62F)
	if addr >= C6502_TED_V_BASE && addr <= C6502_TED_V_END {
		tedVReg := uint32(addr - C6502_TED_V_BASE)
		adapter.bus.Write8(TED_VIDEO_BASE+(tedVReg*4), value)
		return
	}

	// Handle ULA register writes ($D800-$D80F)
	if addr >= C6502_ULA_BASE && addr <= C6502_ULA_BASE+0x0F {
		ulaReg := uint32(addr - C6502_ULA_BASE)
		adapter.bus.Write8(ULA_BASE+ulaReg, value)
		return
	}

	// Handle VGA register writes ($D700-$D70A)
	if adapter.vgaEngine != nil && addr >= C6502_VGA_BASE && addr <= C6502_VGA_END {
		switch addr {
		case C6502_VGA_MODE:
			adapter.vgaEngine.HandleWrite(VGA_MODE, uint32(value))
			return
		case C6502_VGA_STATUS:
			// Status is read-only, but accept writes silently
			return
		case C6502_VGA_CTRL:
			adapter.vgaEngine.HandleWrite(VGA_CTRL, uint32(value))
			return
		case C6502_VGA_SEQ_IDX:
			adapter.vgaEngine.HandleWrite(VGA_SEQ_INDEX, uint32(value))
			return
		case C6502_VGA_SEQ_DATA:
			adapter.vgaEngine.HandleWrite(VGA_SEQ_DATA, uint32(value))
			return
		case C6502_VGA_CRTC_IDX:
			adapter.vgaEngine.HandleWrite(VGA_CRTC_INDEX, uint32(value))
			return
		case C6502_VGA_CRTC_DATA:
			adapter.vgaEngine.HandleWrite(VGA_CRTC_DATA, uint32(value))
			return
		case C6502_VGA_GC_IDX:
			adapter.vgaEngine.HandleWrite(VGA_GC_INDEX, uint32(value))
			return
		case C6502_VGA_GC_DATA:
			adapter.vgaEngine.HandleWrite(VGA_GC_DATA, uint32(value))
			return
		case C6502_VGA_DAC_WIDX:
			adapter.vgaEngine.HandleWrite(VGA_DAC_WINDEX, uint32(value))
			return
		case C6502_VGA_DAC_DATA:
			adapter.vgaEngine.HandleWrite(VGA_DAC_DATA, uint32(value))
			return
		}
	}

	// Handle extended bank window writes (IE65 mode)
	if translated, ok := adapter.translateExtendedBank(addr); ok {
		adapter.bus.Write8(translated, value)
		return
	}

	// Handle VRAM bank window writes
	if translated, ok := adapter.translateVRAM(addr); ok {
		adapter.bus.Write8(translated, value)
		return
	}

	// Debug: track writes to ULA range even if not matched
	if addr >= 0xD800 && addr <= 0xD8FF {
		fmt.Printf("6502 write in ULA range: addr=0x%04X value=0x%02X (NOT handled as ULA)\n", addr, value)
	}
	adapter.bus.Write8(translateIO8Bit_6502(addr), value)
}

func (adapter *Bus6502Adapter) ResetBank() {
	adapter.vramBank = 0
	adapter.vramEnabled = false
	adapter.bank1 = 0
	adapter.bank2 = 0
	adapter.bank3 = 0
	adapter.bank1Enable = false
	adapter.bank2Enable = false
	adapter.bank3Enable = false
}

func (adapter *Bus6502Adapter) translateExtendedBank(addr uint16) (uint32, bool) {
	/*
	   translateExtendedBank translates addresses in the extended bank windows
	   to their actual 32-bit addresses.

	   Bank window layout:
	   - $2000-$3FFF: Bank 1 (sprite data)
	   - $4000-$5FFF: Bank 2 (font data)
	   - $6000-$7FFF: Bank 3 (general data)

	   Each bank window maps to:
	   base_address = bank_number * 8KB
	   actual_address = base_address + (addr - window_base)
	*/

	// Check Bank 1 window ($2000-$3FFF)
	if adapter.bank1Enable && addr >= BANK1_WINDOW_BASE && addr < BANK1_WINDOW_BASE+BANK_WINDOW_SIZE {
		offset := uint32(addr - BANK1_WINDOW_BASE)
		translated := (adapter.bank1 * BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	// Check Bank 2 window ($4000-$5FFF)
	if adapter.bank2Enable && addr >= BANK2_WINDOW_BASE && addr < BANK2_WINDOW_BASE+BANK_WINDOW_SIZE {
		offset := uint32(addr - BANK2_WINDOW_BASE)
		translated := (adapter.bank2 * BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	// Check Bank 3 window ($6000-$7FFF)
	if adapter.bank3Enable && addr >= BANK3_WINDOW_BASE && addr < BANK3_WINDOW_BASE+BANK_WINDOW_SIZE {
		offset := uint32(addr - BANK3_WINDOW_BASE)
		translated := (adapter.bank3 * BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	return 0, false
}

func (adapter *Bus6502Adapter) translateVRAM(addr uint16) (uint32, bool) {
	if !adapter.vramEnabled {
		return 0, false
	}

	if addr < VRAM_BANK_WINDOW_BASE || addr >= VRAM_BANK_WINDOW_BASE+VRAM_BANK_WINDOW_SIZE {
		return 0, false
	}

	if adapter.isMappedIO(addr) {
		return 0, false
	}

	translated := uint32(VRAM_START) +
		(adapter.vramBank * VRAM_BANK_WINDOW_SIZE) +
		uint32(addr-VRAM_BANK_WINDOW_BASE)

	if translated >= uint32(VRAM_START+VRAM_SIZE) {
		return 0, false
	}

	return translated, true
}

func (adapter *Bus6502Adapter) isMappedIO(addr uint16) bool {
	bus, ok := adapter.bus.(*MachineBus)
	if !ok {
		return false
	}

	page := uint32(addr) & PAGE_MASK
	regions, exists := bus.mapping[page]
	if !exists {
		return false
	}
	for _, region := range regions {
		if uint32(addr) >= region.start && uint32(addr) <= region.end {
			return true
		}
	}
	return false
}

func btou8(b bool) byte {
	/*
	   btou8 converts boolean to unsigned 8-bit.

	   Parameters:
	   - b: Boolean value

	   Returns:
	   - byte: 1 if true, 0 if false
	*/

	if b {
		return 1
	}
	return 0
}
func btou16(b bool) uint16 {
	/*
	   btou16 converts boolean to unsigned 16-bit.

	   Parameters:
	   - b: Boolean value

	   Returns:
	   - uint16: 1 if true, 0 if false
	*/

	if b {
		return 1
	}
	return 0
}
