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
	"sync"
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
	   - memory        MemoryBus_6502   // Memory interface
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

	// Cache Line 1 - Interrupt Control
	Running      bool // Execution state
	irqPending   bool // IRQ signal
	resetPending bool // Reset signal
	InInterrupt  bool // In handler
	nmiLine      bool // NMI signal
	nmiPending   bool // NMI detected
	nmiPrevious  bool // NMI previous
	_padding0    byte // Alignment

	// Cache Line 2 - System Control
	Cycles        uint64          // Cycle counter
	rdyLine       bool            // RDY signal
	rdyHold       bool            // RDY active
	Debug         bool            // Debug mode
	memory        MemoryBus_6502  // Memory interface
	mutex         sync.RWMutex    // State protection
	breakpoints   map[uint16]bool // Debug points
	breakpointHit chan uint16     // Debug channel
}

type MemoryBus_6502 interface {
	/*
	   MemoryBus_6502 defines the memory access protocol for the 6502.

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
type MemoryBusAdapter_6502 struct {
	/*
	   MemoryBusAdapter_6502 adapts the 32-bit memory bus for 8-bit access.

	   Structure:
	   - bus: Reference to 32-bit system bus
	   - vramBank: Active VRAM bank for 6502 window access

	   Purpose:
	   Provides translation layer between 8-bit 6502 and 32-bit memory system
	*/

	bus         MemoryBus
	vramBank    uint32
	vramEnabled bool
}

func NewCPU_6502(bus MemoryBus) *CPU_6502 {
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
	adapter := NewMemoryBusAdapter_6502(bus)
	return &CPU_6502{
		memory:        adapter,
		SP:            0xFF,
		SR:            UNUSED_FLAG,
		Running:       true,
		breakpoints:   make(map[uint16]bool),
		breakpointHit: make(chan uint16, 1),
	}
}
func NewMemoryBusAdapter_6502(bus MemoryBus) *MemoryBusAdapter_6502 {
	/*
	   NewMemoryBusAdapter_6502 creates memory bus adapter instance.

	   Parameters:
	   - bus: System memory bus interface

	   Returns:
	   - *MemoryBusAdapter_6502: Configured adapter
	*/

	return &MemoryBusAdapter_6502{bus: bus}
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

	value := cpu_6502.memory.Read(addr)
	cpu_6502.memory.Write(addr, value) // Spurious write of original value
	result := operation(value)
	cpu_6502.memory.Write(addr, result)
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

	lo := uint16(cpu_6502.memory.Read(addr))
	hi := uint16(cpu_6502.memory.Read(addr + 1))
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

	cpu_6502.memory.Write(STACK_BASE|uint16(cpu_6502.SP), value)
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
	return cpu_6502.memory.Read(STACK_BASE | uint16(cpu_6502.SP))
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

	addr := uint16(cpu_6502.memory.Read(cpu_6502.PC))
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

	addr := (uint16(cpu_6502.memory.Read(cpu_6502.PC)) + uint16(cpu_6502.X)) & 0xFF
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

	addr := (uint16(cpu_6502.memory.Read(cpu_6502.PC)) + uint16(cpu_6502.Y)) & 0xFF
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

	base := cpu_6502.memory.Read(cpu_6502.PC)
	cpu_6502.PC++
	ptr := (uint16(base) + uint16(cpu_6502.X)) & 0xFF
	return uint16(cpu_6502.memory.Read(ptr)) | uint16(cpu_6502.memory.Read((ptr+1)&0xFF))<<8
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

	ptr := uint16(cpu_6502.memory.Read(cpu_6502.PC))
	cpu_6502.PC++
	base := uint16(cpu_6502.memory.Read(ptr)) | uint16(cpu_6502.memory.Read((ptr+1)&0xFF))<<8
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

	if cpu_6502.getFlag(DECIMAL_FLAG) {
		a := uint16(cpu_6502.A)
		b := uint16(value)
		carry := btou16(cpu_6502.getFlag(CARRY_FLAG))

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
		cpu_6502.setFlag(ZERO_FLAG, (result&0xFF) == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, result&0x80 != 0)

		old_a := cpu_6502.A
		cpu_6502.A = byte(result)

		// V Flag is set based on two's complement overflow
		overflow := (old_a^value)&0x80 == 0 && (old_a^cpu_6502.A)&0x80 != 0
		cpu_6502.setFlag(OVERFLOW_FLAG, overflow)
	} else {
		temp := uint16(cpu_6502.A) + uint16(value)
		if cpu_6502.getFlag(CARRY_FLAG) {
			temp++
		}

		carry := temp > 0xFF
		result := byte(temp)

		cpu_6502.setFlag(CARRY_FLAG, carry)
		cpu_6502.setFlag(ZERO_FLAG, result == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, result&0x80 != 0)

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

	if cpu_6502.getFlag(DECIMAL_FLAG) {
		a := uint16(cpu_6502.A)
		b := uint16(value)
		borrow := btou16(!cpu_6502.getFlag(CARRY_FLAG))

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
		cpu_6502.setFlag(ZERO_FLAG, (result&0xFF) == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, result&0x80 != 0)

		old_a := cpu_6502.A
		cpu_6502.A = byte(result)

		overflow := (old_a^value)&0x80 != 0 && (old_a^cpu_6502.A)&0x80 != 0
		cpu_6502.setFlag(OVERFLOW_FLAG, overflow)
	} else {
		temp := uint16(cpu_6502.A) - uint16(value)
		if !cpu_6502.getFlag(CARRY_FLAG) {
			temp--
		}

		result := byte(temp)
		carry := temp < 0x100

		cpu_6502.setFlag(CARRY_FLAG, carry)
		cpu_6502.setFlag(ZERO_FLAG, result == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, result&0x80 != 0)

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
		cpu_6502.setFlag(ZERO_FLAG, result == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, result&0x80 != 0)
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
		cpu_6502.setFlag(ZERO_FLAG, result == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, result&0x80 != 0)
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
		cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
		return cpu_6502.A
	}

	var result byte
	cpu_6502.rmw(addr, func(value byte) byte {
		cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
		result = value << 1
		cpu_6502.setFlag(ZERO_FLAG, result == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, result&0x80 != 0)
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
		cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, false)
		return cpu_6502.A
	}

	var result byte
	cpu_6502.rmw(addr, func(value byte) byte {
		cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
		result = value >> 1
		cpu_6502.setFlag(ZERO_FLAG, result == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, false)
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
		carry := btou8(cpu_6502.getFlag(CARRY_FLAG))
		cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x80 != 0)
		cpu_6502.A = (cpu_6502.A << 1) | carry
		cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
		return cpu_6502.A
	}

	var result byte
	oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
	cpu_6502.rmw(addr, func(value byte) byte {
		cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
		result = (value << 1) | oldCarry
		cpu_6502.setFlag(ZERO_FLAG, result == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, result&0x80 != 0)
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
		carry := btou8(cpu_6502.getFlag(CARRY_FLAG))
		cpu_6502.setFlag(CARRY_FLAG, cpu_6502.A&0x01 != 0)
		cpu_6502.A = (cpu_6502.A >> 1) | (carry << 7)
		cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
		return cpu_6502.A
	}

	var result byte
	oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
	cpu_6502.rmw(addr, func(value byte) byte {
		cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
		result = (value >> 1) | (oldCarry << 7)
		cpu_6502.setFlag(ZERO_FLAG, result == 0)
		cpu_6502.setFlag(NEGATIVE_FLAG, result&0x80 != 0)
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
	cpu_6502.setFlag(ZERO_FLAG, reg == value)
	cpu_6502.setFlag(NEGATIVE_FLAG, (temp&0x80) != 0)
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

	offset := int8(cpu_6502.memory.Read(cpu_6502.PC))
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

	if !isNMI && cpu_6502.getFlag(INTERRUPT_FLAG) {
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
	   2. Detects falling edge
	   3. Sets NMI pending on edge
	*/

	cpu_6502.mutex.Lock()
	defer cpu_6502.mutex.Unlock()

	cpu_6502.nmiLine = state
	// Detect falling edge (1->0 transition)
	if cpu_6502.nmiPrevious && !state {
		cpu_6502.nmiPending = true
	}
	cpu_6502.nmiPrevious = state
}
func (cpu_6502 *CPU_6502) SetRDYLine(state bool) {
	/*
	   SetRDYLine controls the RDY line state.

	   Parameters:
	   - state: New RDY line state

	   Operation:
	   1. Acquires mutex
	   2. Updates RDY state
	*/

	cpu_6502.mutex.Lock()
	defer cpu_6502.mutex.Unlock()
	cpu_6502.rdyLine = state
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
	   Full mutex protection during reset sequence
	*/

	cpu_6502.mutex.Lock()
	defer cpu_6502.mutex.Unlock()

	cpu_6502.A = 0
	cpu_6502.X = 0
	cpu_6502.Y = 0
	cpu_6502.SP = 0xFF
	cpu_6502.SR = UNUSED_FLAG | INTERRUPT_FLAG
	cpu_6502.PC = cpu_6502.read16(RESET_VECTOR)
	cpu_6502.Cycles = 0
	cpu_6502.Running = true
	cpu_6502.InInterrupt = false

	//NMI
	cpu_6502.nmiLine = false
	cpu_6502.nmiPending = false
	cpu_6502.nmiPrevious = false
	cpu_6502.irqPending = false
	cpu_6502.resetPending = false

	if adapter, ok := cpu_6502.memory.(*MemoryBusAdapter_6502); ok {
		adapter.ResetBank()
	}
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

	for cpu_6502.Running {
		cpu_6502.mutex.Lock()

		// Check for RDY line hold
		if !cpu_6502.rdyLine {
			cpu_6502.rdyHold = true
			cpu_6502.mutex.Unlock()
			continue
		}
		cpu_6502.rdyHold = false

		// Check for interrupts
		if cpu_6502.nmiPending {
			cpu_6502.handleInterrupt(NMI_VECTOR, true)
			cpu_6502.nmiPending = false
		} else if cpu_6502.irqPending && !cpu_6502.getFlag(INTERRUPT_FLAG) {
			cpu_6502.handleInterrupt(IRQ_VECTOR, false)
			cpu_6502.irqPending = false
		}

		// Fetch and execute instruction
		opcode := cpu_6502.memory.Read(cpu_6502.PC)
		cpu_6502.PC++

		// Handle breakpoints
		if _, exists := cpu_6502.breakpoints[cpu_6502.PC-1]; exists {
			cpu_6502.breakpointHit <- cpu_6502.PC - 1
			<-cpu_6502.breakpointHit
		}

		switch opcode {

		// Load/Store Operations
		case 0xA9: // LDA Immediate
			cpu_6502.A = cpu_6502.memory.Read(cpu_6502.PC)
			cpu_6502.PC++
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0xA5: // LDA Zero Page
			cpu_6502.A = cpu_6502.memory.Read(cpu_6502.getZeroPage())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 3

		case 0xB5: // LDA Zero Page,X
			cpu_6502.A = cpu_6502.memory.Read(cpu_6502.getZeroPageX())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0xAD: // LDA Absolute
			cpu_6502.A = cpu_6502.memory.Read(cpu_6502.getAbsolute())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0xBD: // LDA Absolute,X
			addr, crossed := cpu_6502.getAbsoluteX()
			cpu_6502.A = cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0xB9: // LDA Absolute,Y
			addr, crossed := cpu_6502.getAbsoluteY()
			cpu_6502.A = cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0xA1: // LDA (Indirect,X)
			cpu_6502.A = cpu_6502.memory.Read(cpu_6502.getIndirectX())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 6

		case 0xB1: // LDA (Indirect),Y
			addr, crossed := cpu_6502.getIndirectY()
			cpu_6502.A = cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 5
			if crossed {
				cpu_6502.Cycles++
			}
		case 0xA2: // LDX Immediate
			cpu_6502.X = cpu_6502.memory.Read(cpu_6502.PC)
			cpu_6502.PC++
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.X == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.X&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0xA6: // LDX Zero Page
			cpu_6502.X = cpu_6502.memory.Read(cpu_6502.getZeroPage())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.X == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.X&0x80 != 0)
			cpu_6502.Cycles += 3

		case 0xB6: // LDX Zero Page,Y
			cpu_6502.X = cpu_6502.memory.Read(cpu_6502.getZeroPageY())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.X == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.X&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0xAE: // LDX Absolute
			cpu_6502.X = cpu_6502.memory.Read(cpu_6502.getAbsolute())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.X == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.X&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0xBE: // LDX Absolute,Y
			addr, crossed := cpu_6502.getAbsoluteY()
			cpu_6502.X = cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.X == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.X&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0xA0: // LDY Immediate
			cpu_6502.Y = cpu_6502.memory.Read(cpu_6502.PC)
			cpu_6502.PC++
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.Y == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.Y&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0xA4: // LDY Zero Page
			cpu_6502.Y = cpu_6502.memory.Read(cpu_6502.getZeroPage())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.Y == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.Y&0x80 != 0)
			cpu_6502.Cycles += 3

		case 0xB4: // LDY Zero Page,X
			cpu_6502.Y = cpu_6502.memory.Read(cpu_6502.getZeroPageX())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.Y == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.Y&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0xAC: // LDY Absolute
			cpu_6502.Y = cpu_6502.memory.Read(cpu_6502.getAbsolute())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.Y == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.Y&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0xBC: // LDY Absolute,X
			addr, crossed := cpu_6502.getAbsoluteX()
			cpu_6502.Y = cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.Y == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.Y&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

			// Store operations
		case 0x85: // STA Zero Page
			cpu_6502.memory.Write(cpu_6502.getZeroPage(), cpu_6502.A)
			cpu_6502.Cycles += 3

		case 0x95: // STA Zero Page,X
			cpu_6502.memory.Write(cpu_6502.getZeroPageX(), cpu_6502.A)
			cpu_6502.Cycles += 4

		case 0x8D: // STA Absolute
			cpu_6502.memory.Write(cpu_6502.getAbsolute(), cpu_6502.A)
			cpu_6502.Cycles += 4

		case 0x9D: // STA Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			cpu_6502.memory.Write(addr, cpu_6502.A)
			cpu_6502.Cycles += 5

		case 0x99: // STA Absolute,Y
			addr, _ := cpu_6502.getAbsoluteY()
			cpu_6502.memory.Write(addr, cpu_6502.A)
			cpu_6502.Cycles += 5

		case 0x81: // STA (Indirect,X)
			cpu_6502.memory.Write(cpu_6502.getIndirectX(), cpu_6502.A)
			cpu_6502.Cycles += 6

		case 0x91: // STA (Indirect),Y
			addr, _ := cpu_6502.getIndirectY()
			cpu_6502.memory.Write(addr, cpu_6502.A)
			cpu_6502.Cycles += 6

		case 0x86: // STX Zero Page
			cpu_6502.memory.Write(cpu_6502.getZeroPage(), cpu_6502.X)
			cpu_6502.Cycles += 3

		case 0x96: // STX Zero Page,Y
			cpu_6502.memory.Write(cpu_6502.getZeroPageY(), cpu_6502.X)
			cpu_6502.Cycles += 4

		case 0x8E: // STX Absolute
			cpu_6502.memory.Write(cpu_6502.getAbsolute(), cpu_6502.X)
			cpu_6502.Cycles += 4

		case 0x84: // STY Zero Page
			cpu_6502.memory.Write(cpu_6502.getZeroPage(), cpu_6502.Y)
			cpu_6502.Cycles += 3

		case 0x94: // STY Zero Page,X
			cpu_6502.memory.Write(cpu_6502.getZeroPageX(), cpu_6502.Y)
			cpu_6502.Cycles += 4

		case 0x8C: // STY Absolute
			cpu_6502.memory.Write(cpu_6502.getAbsolute(), cpu_6502.Y)
			cpu_6502.Cycles += 4

			// Register Transfer Operations
		case 0xAA: // TAX
			cpu_6502.X = cpu_6502.A
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.X == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.X&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0x8A: // TXA
			cpu_6502.A = cpu_6502.X
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0xA8: // TAY
			cpu_6502.Y = cpu_6502.A
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.Y == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.Y&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0x98: // TYA
			cpu_6502.A = cpu_6502.Y
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0xBA: // TSX
			cpu_6502.X = cpu_6502.SP
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.X == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.X&0x80 != 0)
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
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0x08: // PHP
			cpu_6502.push(cpu_6502.SR | BREAK_FLAG | UNUSED_FLAG)
			cpu_6502.Cycles += 3

		case 0x28: // PLP
			cpu_6502.SR = (cpu_6502.pop() & 0xEF) | UNUSED_FLAG
			cpu_6502.Cycles += 4

			// Arithmetic Operations
		case 0x69: // ADC Immediate
			cpu_6502.adc(cpu_6502.memory.Read(cpu_6502.PC))
			cpu_6502.PC++
			cpu_6502.Cycles += 2

		case 0x65: // ADC Zero Page
			cpu_6502.adc(cpu_6502.memory.Read(cpu_6502.getZeroPage()))
			cpu_6502.Cycles += 3

		case 0x75: // ADC Zero Page,X
			cpu_6502.adc(cpu_6502.memory.Read(cpu_6502.getZeroPageX()))
			cpu_6502.Cycles += 4

		case 0x6D: // ADC Absolute
			cpu_6502.adc(cpu_6502.memory.Read(cpu_6502.getAbsolute()))
			cpu_6502.Cycles += 4

		case 0x7D: // ADC Absolute,X
			addr, crossed := cpu_6502.getAbsoluteX()
			cpu_6502.adc(cpu_6502.memory.Read(addr))
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x79: // ADC Absolute,Y
			addr, crossed := cpu_6502.getAbsoluteY()
			cpu_6502.adc(cpu_6502.memory.Read(addr))
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x61: // ADC (Indirect,X)
			cpu_6502.adc(cpu_6502.memory.Read(cpu_6502.getIndirectX()))
			cpu_6502.Cycles += 6

		case 0x71: // ADC (Indirect),Y
			addr, crossed := cpu_6502.getIndirectY()
			cpu_6502.adc(cpu_6502.memory.Read(addr))
			cpu_6502.Cycles += 5
			if crossed {
				cpu_6502.Cycles++
			}

		case 0xE9: // SBC Immediate
			cpu_6502.sbc(cpu_6502.memory.Read(cpu_6502.PC))
			cpu_6502.PC++
			cpu_6502.Cycles += 2

		case 0xE5: // SBC Zero Page
			cpu_6502.sbc(cpu_6502.memory.Read(cpu_6502.getZeroPage()))
			cpu_6502.Cycles += 3

		case 0xF5: // SBC Zero Page,X
			cpu_6502.sbc(cpu_6502.memory.Read(cpu_6502.getZeroPageX()))
			cpu_6502.Cycles += 4

		case 0xED: // SBC Absolute
			cpu_6502.sbc(cpu_6502.memory.Read(cpu_6502.getAbsolute()))
			cpu_6502.Cycles += 4

		case 0xFD: // SBC Absolute,X
			addr, crossed := cpu_6502.getAbsoluteX()
			cpu_6502.sbc(cpu_6502.memory.Read(addr))
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0xF9: // SBC Absolute,Y
			addr, crossed := cpu_6502.getAbsoluteY()
			cpu_6502.sbc(cpu_6502.memory.Read(addr))
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0xE1: // SBC (Indirect,X)
			cpu_6502.sbc(cpu_6502.memory.Read(cpu_6502.getIndirectX()))
			cpu_6502.Cycles += 6

		case 0xF1: // SBC (Indirect),Y
			addr, crossed := cpu_6502.getIndirectY()
			cpu_6502.sbc(cpu_6502.memory.Read(addr))
			cpu_6502.Cycles += 5
			if crossed {
				cpu_6502.Cycles++
			}

			// Logical Operations
		case 0x24: // BIT Zero Page
			value := cpu_6502.memory.Read(cpu_6502.getZeroPage())
			cpu_6502.setFlag(ZERO_FLAG, (cpu_6502.A&value) == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.setFlag(OVERFLOW_FLAG, value&0x40 != 0)
			cpu_6502.Cycles += 3

		case 0x2C: // BIT Absolute
			value := cpu_6502.memory.Read(cpu_6502.getAbsolute())
			cpu_6502.setFlag(ZERO_FLAG, (cpu_6502.A&value) == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.setFlag(OVERFLOW_FLAG, value&0x40 != 0)
			cpu_6502.Cycles += 4

		case 0x29: // AND Immediate
			cpu_6502.A &= cpu_6502.memory.Read(cpu_6502.PC)
			cpu_6502.PC++
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0x25: // AND Zero Page
			cpu_6502.A &= cpu_6502.memory.Read(cpu_6502.getZeroPage())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 3

		case 0x35: // AND Zero Page,X
			cpu_6502.A &= cpu_6502.memory.Read(cpu_6502.getZeroPageX())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0x2D: // AND Absolute
			cpu_6502.A &= cpu_6502.memory.Read(cpu_6502.getAbsolute())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0x3D: // AND Absolute,X
			addr, crossed := cpu_6502.getAbsoluteX()
			cpu_6502.A &= cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x39: // AND Absolute,Y
			addr, crossed := cpu_6502.getAbsoluteY()
			cpu_6502.A &= cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x21: // AND (Indirect,X)
			cpu_6502.A &= cpu_6502.memory.Read(cpu_6502.getIndirectX())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 6

		case 0x31: // AND (Indirect),Y
			addr, crossed := cpu_6502.getIndirectY()
			cpu_6502.A &= cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 5
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x09: // ORA Immediate
			cpu_6502.A |= cpu_6502.memory.Read(cpu_6502.PC)
			cpu_6502.PC++
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0x05: // ORA Zero Page
			cpu_6502.A |= cpu_6502.memory.Read(cpu_6502.getZeroPage())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 3

		case 0x15: // ORA Zero Page,X
			cpu_6502.A |= cpu_6502.memory.Read(cpu_6502.getZeroPageX())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0x0D: // ORA Absolute
			cpu_6502.A |= cpu_6502.memory.Read(cpu_6502.getAbsolute())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0x1D: // ORA Absolute,X
			addr, crossed := cpu_6502.getAbsoluteX()
			cpu_6502.A |= cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x19: // ORA Absolute,Y
			addr, crossed := cpu_6502.getAbsoluteY()
			cpu_6502.A |= cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x01: // ORA (Indirect,X)
			cpu_6502.A |= cpu_6502.memory.Read(cpu_6502.getIndirectX())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 6

		case 0x11: // ORA (Indirect),Y
			addr, crossed := cpu_6502.getIndirectY()
			cpu_6502.A |= cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 5
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x49: // EOR Immediate
			cpu_6502.A ^= cpu_6502.memory.Read(cpu_6502.PC)
			cpu_6502.PC++
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0x45: // EOR Zero Page
			cpu_6502.A ^= cpu_6502.memory.Read(cpu_6502.getZeroPage())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 3

		case 0x55: // EOR Zero Page,X
			cpu_6502.A ^= cpu_6502.memory.Read(cpu_6502.getZeroPageX())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0x4D: // EOR Absolute
			cpu_6502.A ^= cpu_6502.memory.Read(cpu_6502.getAbsolute())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0x5D: // EOR Absolute,X
			addr, crossed := cpu_6502.getAbsoluteX()
			cpu_6502.A ^= cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x59: // EOR Absolute,Y
			addr, crossed := cpu_6502.getAbsoluteY()
			cpu_6502.A ^= cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x41: // EOR (Indirect,X)
			cpu_6502.A ^= cpu_6502.memory.Read(cpu_6502.getIndirectX())
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 6

		case 0x51: // EOR (Indirect),Y
			addr, crossed := cpu_6502.getIndirectY()
			cpu_6502.A ^= cpu_6502.memory.Read(addr)
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 5
			if crossed {
				cpu_6502.Cycles++
			}

			// Shift & Rotate Operations
		case 0x0A: // ASL Accumulator
			cpu_6502.asl(0, true)
			cpu_6502.Cycles += 2

		case 0x06: // ASL Zero Page
			addr := cpu_6502.getZeroPage()
			cpu_6502.asl(addr, false)
			cpu_6502.Cycles += 5

		case 0x16: // ASL Zero Page,X
			addr := cpu_6502.getZeroPageX()
			cpu_6502.asl(addr, false)
			cpu_6502.Cycles += 6

		case 0x0E: // ASL Absolute
			addr := cpu_6502.getAbsolute()
			cpu_6502.asl(addr, false)
			cpu_6502.Cycles += 6

		case 0x1E: // ASL Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			cpu_6502.asl(addr, false)
			cpu_6502.Cycles += 7

		case 0x4A: // LSR Accumulator
			cpu_6502.lsr(0, true)
			cpu_6502.Cycles += 2

		case 0x46: // LSR Zero Page
			addr := cpu_6502.getZeroPage()
			cpu_6502.lsr(addr, false)
			cpu_6502.Cycles += 5

		case 0x56: // LSR Zero Page,X
			addr := cpu_6502.getZeroPageX()
			cpu_6502.lsr(addr, false)
			cpu_6502.Cycles += 6

		case 0x4E: // LSR Absolute
			addr := cpu_6502.getAbsolute()
			cpu_6502.lsr(addr, false)
			cpu_6502.Cycles += 6

		case 0x5E: // LSR Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			cpu_6502.lsr(addr, false)
			cpu_6502.Cycles += 7

		case 0x2A: // ROL Accumulator
			cpu_6502.rol(0, true)
			cpu_6502.Cycles += 2

		case 0x26: // ROL Zero Page
			addr := cpu_6502.getZeroPage()
			cpu_6502.rol(addr, false)
			cpu_6502.Cycles += 5

		case 0x36: // ROL Zero Page,X
			addr := cpu_6502.getZeroPageX()
			cpu_6502.rol(addr, false)
			cpu_6502.Cycles += 6

		case 0x2E: // ROL Absolute
			addr := cpu_6502.getAbsolute()
			cpu_6502.rol(addr, false)
			cpu_6502.Cycles += 6

		case 0x3E: // ROL Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			cpu_6502.rol(addr, false)
			cpu_6502.Cycles += 7

		case 0x6A: // ROR Accumulator
			cpu_6502.ror(0, true)
			cpu_6502.Cycles += 2

		case 0x66: // ROR Zero Page
			addr := cpu_6502.getZeroPage()
			cpu_6502.ror(addr, false)
			cpu_6502.Cycles += 5

		case 0x76: // ROR Zero Page,X
			addr := cpu_6502.getZeroPageX()
			cpu_6502.ror(addr, false)
			cpu_6502.Cycles += 6

		case 0x6E: // ROR Absolute
			addr := cpu_6502.getAbsolute()
			cpu_6502.ror(addr, false)
			cpu_6502.Cycles += 6

		case 0x7E: // ROR Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			cpu_6502.ror(addr, false)
			cpu_6502.Cycles += 7

			// Increment/Decrement
		case 0xE6: // INC Zero Page
			addr := cpu_6502.getZeroPage()
			value := cpu_6502.memory.Read(addr) + 1
			cpu_6502.memory.Write(addr, value)
			cpu_6502.setFlag(ZERO_FLAG, value == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.Cycles += 5

		case 0xF6: // INC Zero Page,X
			addr := cpu_6502.getZeroPageX()
			value := cpu_6502.memory.Read(addr) + 1
			cpu_6502.memory.Write(addr, value)
			cpu_6502.setFlag(ZERO_FLAG, value == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.Cycles += 6

		case 0xEE: // INC Absolute
			addr := cpu_6502.getAbsolute()
			value := cpu_6502.memory.Read(addr) + 1
			cpu_6502.memory.Write(addr, value)
			cpu_6502.setFlag(ZERO_FLAG, value == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.Cycles += 6

		case 0xFE: // INC Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			value := cpu_6502.memory.Read(addr) + 1
			cpu_6502.memory.Write(addr, value)
			cpu_6502.setFlag(ZERO_FLAG, value == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.Cycles += 7
		case 0xC6: // DEC Zero Page
			addr := cpu_6502.getZeroPage()
			value := cpu_6502.memory.Read(addr) - 1
			cpu_6502.memory.Write(addr, value)
			cpu_6502.setFlag(ZERO_FLAG, value == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.Cycles += 5

		case 0xD6: // DEC Zero Page,X
			addr := cpu_6502.getZeroPageX()
			value := cpu_6502.memory.Read(addr) - 1
			cpu_6502.memory.Write(addr, value)
			cpu_6502.setFlag(ZERO_FLAG, value == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.Cycles += 6

		case 0xCE: // DEC Absolute
			addr := cpu_6502.getAbsolute()
			value := cpu_6502.memory.Read(addr) - 1
			cpu_6502.memory.Write(addr, value)
			cpu_6502.setFlag(ZERO_FLAG, value == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.Cycles += 6

		case 0xDE: // DEC Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			value := cpu_6502.memory.Read(addr) - 1
			cpu_6502.memory.Write(addr, value)
			cpu_6502.setFlag(ZERO_FLAG, value == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, value&0x80 != 0)
			cpu_6502.Cycles += 7

		case 0xE8: // INX
			cpu_6502.X++
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.X == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.X&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0xC8: // INY
			cpu_6502.Y++
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.Y == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.Y&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0xCA: // DEX
			cpu_6502.X--
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.X == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.X&0x80 != 0)
			cpu_6502.Cycles += 2

		case 0x88: // DEY
			cpu_6502.Y--
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.Y == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.Y&0x80 != 0)
			cpu_6502.Cycles += 2

			// Jump & Call Operations
		case 0x4C: // JMP Absolute
			cpu_6502.PC = cpu_6502.getAbsolute()
			cpu_6502.Cycles += 3

		case 0x6C: // JMP Indirect
			addr := cpu_6502.getAbsolute()
			if (addr & 0xFF) == 0xFF {
				// Hardware bug: if address is on page boundary, fetch high byte from start of page
				lo := cpu_6502.memory.Read(addr)
				hi := cpu_6502.memory.Read(addr & 0xFF00)
				cpu_6502.PC = uint16(hi)<<8 | uint16(lo)
			} else {
				cpu_6502.PC = cpu_6502.read16(addr)
			}
			cpu_6502.Cycles += 5

		case 0x20: // JSR
			cpu_6502.push16(cpu_6502.PC + 1)
			cpu_6502.PC = cpu_6502.getAbsolute()
			cpu_6502.Cycles += 6

		case 0x60: // RTS
			cpu_6502.PC = cpu_6502.pop16() + 1
			cpu_6502.Cycles += 6

			// Branch Instructions
		case 0x90: // BCC
			cpu_6502.branch(!cpu_6502.getFlag(CARRY_FLAG))

		case 0xB0: // BCS
			cpu_6502.branch(cpu_6502.getFlag(CARRY_FLAG))

		case 0xF0: // BEQ
			cpu_6502.branch(cpu_6502.getFlag(ZERO_FLAG))

		case 0x30: // BMI
			cpu_6502.branch(cpu_6502.getFlag(NEGATIVE_FLAG))

		case 0xD0: // BNE
			cpu_6502.branch(!cpu_6502.getFlag(ZERO_FLAG))

		case 0x10: // BPL
			cpu_6502.branch(!cpu_6502.getFlag(NEGATIVE_FLAG))

		case 0x50: // BVC
			cpu_6502.branch(!cpu_6502.getFlag(OVERFLOW_FLAG))

		case 0x70: // BVS
			cpu_6502.branch(cpu_6502.getFlag(OVERFLOW_FLAG))

			// Flag Operations
		case 0x18: // CLC
			cpu_6502.setFlag(CARRY_FLAG, false)
			cpu_6502.Cycles += 2

		case 0x38: // SEC
			cpu_6502.setFlag(CARRY_FLAG, true)
			cpu_6502.Cycles += 2

		case 0xD8: // CLD
			cpu_6502.setFlag(DECIMAL_FLAG, false)
			cpu_6502.Cycles += 2

		case 0xF8: // SED
			cpu_6502.setFlag(DECIMAL_FLAG, true)
			cpu_6502.Cycles += 2

		case 0x58: // CLI
			cpu_6502.setFlag(INTERRUPT_FLAG, false)
			cpu_6502.Cycles += 2

		case 0x78: // SEI
			cpu_6502.setFlag(INTERRUPT_FLAG, true)
			cpu_6502.Cycles += 2

		case 0xB8: // CLV
			cpu_6502.setFlag(OVERFLOW_FLAG, false)
			cpu_6502.Cycles += 2

			// Compare Operations
		case 0xC9: // CMP Immediate
			cpu_6502.compare(cpu_6502.A, cpu_6502.memory.Read(cpu_6502.PC))
			cpu_6502.PC++
			cpu_6502.Cycles += 2

		case 0xC5: // CMP Zero Page
			cpu_6502.compare(cpu_6502.A, cpu_6502.memory.Read(cpu_6502.getZeroPage()))
			cpu_6502.Cycles += 3

		case 0xD5: // CMP Zero Page,X
			cpu_6502.compare(cpu_6502.A, cpu_6502.memory.Read(cpu_6502.getZeroPageX()))
			cpu_6502.Cycles += 4

		case 0xCD: // CMP Absolute
			cpu_6502.compare(cpu_6502.A, cpu_6502.memory.Read(cpu_6502.getAbsolute()))
			cpu_6502.Cycles += 4

		case 0xDD: // CMP Absolute,X
			addr, crossed := cpu_6502.getAbsoluteX()
			cpu_6502.compare(cpu_6502.A, cpu_6502.memory.Read(addr))
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0xD9: // CMP Absolute,Y
			addr, crossed := cpu_6502.getAbsoluteY()
			cpu_6502.compare(cpu_6502.A, cpu_6502.memory.Read(addr))
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0xC1: // CMP (Indirect,X)
			cpu_6502.compare(cpu_6502.A, cpu_6502.memory.Read(cpu_6502.getIndirectX()))
			cpu_6502.Cycles += 6

		case 0xD1: // CMP (Indirect),Y
			addr, crossed := cpu_6502.getIndirectY()
			cpu_6502.compare(cpu_6502.A, cpu_6502.memory.Read(addr))
			cpu_6502.Cycles += 5
			if crossed {
				cpu_6502.Cycles++
			}
		case 0xE0: // CPX Immediate
			cpu_6502.compare(cpu_6502.X, cpu_6502.memory.Read(cpu_6502.PC))
			cpu_6502.PC++
			cpu_6502.Cycles += 2

		case 0xE4: // CPX Zero Page
			cpu_6502.compare(cpu_6502.X, cpu_6502.memory.Read(cpu_6502.getZeroPage()))
			cpu_6502.Cycles += 3

		case 0xEC: // CPX Absolute
			cpu_6502.compare(cpu_6502.X, cpu_6502.memory.Read(cpu_6502.getAbsolute()))
			cpu_6502.Cycles += 4

		case 0xC0: // CPY Immediate
			cpu_6502.compare(cpu_6502.Y, cpu_6502.memory.Read(cpu_6502.PC))
			cpu_6502.PC++
			cpu_6502.Cycles += 2

		case 0xC4: // CPY Zero Page
			cpu_6502.compare(cpu_6502.Y, cpu_6502.memory.Read(cpu_6502.getZeroPage()))
			cpu_6502.Cycles += 3

		case 0xCC: // CPY Absolute
			cpu_6502.compare(cpu_6502.Y, cpu_6502.memory.Read(cpu_6502.getAbsolute()))
			cpu_6502.Cycles += 4

			// Special Operations
		case 0x00: // BRK
			cpu_6502.PC++
			cpu_6502.push16(cpu_6502.PC)
			// Set B flag only in pushed copy
			cpu_6502.push(cpu_6502.SR | BREAK_FLAG | UNUSED_FLAG)
			cpu_6502.setFlag(INTERRUPT_FLAG, true)
			// Don't set B flag in actual SR
			cpu_6502.SR &= ^byte(BREAK_FLAG)
			cpu_6502.PC = cpu_6502.read16(IRQ_VECTOR)
			cpu_6502.Cycles += 7

		case 0x40: // RTI
			cpu_6502.SR = (cpu_6502.pop() & 0xEF) | UNUSED_FLAG
			cpu_6502.PC = cpu_6502.pop16()
			cpu_6502.Cycles += 6

		case 0xEA: // NOP
			cpu_6502.Cycles += 2

		case 0xFC: // NOP Absolute,X (Unofficial)
			addr, crossed := cpu_6502.getAbsoluteX()
			_ = cpu_6502.memory.Read(addr)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		// Unofficial Opcodes
		case 0xA7: // LAX Zero Page
			addr := cpu_6502.getZeroPage()
			cpu_6502.A = cpu_6502.memory.Read(addr)
			cpu_6502.X = cpu_6502.A
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 3

		case 0xB7: // LAX Zero Page,Y
			addr := cpu_6502.getZeroPageY()
			cpu_6502.A = cpu_6502.memory.Read(addr)
			cpu_6502.X = cpu_6502.A
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0xAF: // LAX Absolute
			addr := cpu_6502.getAbsolute()
			cpu_6502.A = cpu_6502.memory.Read(addr)
			cpu_6502.X = cpu_6502.A
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4

		case 0xBF: // LAX Absolute,Y
			addr, crossed := cpu_6502.getAbsoluteY()
			cpu_6502.A = cpu_6502.memory.Read(addr)
			cpu_6502.X = cpu_6502.A
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 4
			if crossed {
				cpu_6502.Cycles++
			}

		case 0xA3: // LAX (Indirect,X)
			addr := cpu_6502.getIndirectX()
			cpu_6502.A = cpu_6502.memory.Read(addr)
			cpu_6502.X = cpu_6502.A
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 6

		case 0xB3: // LAX (Indirect),Y
			addr, crossed := cpu_6502.getIndirectY()
			cpu_6502.A = cpu_6502.memory.Read(addr)
			cpu_6502.X = cpu_6502.A
			cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
			cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
			cpu_6502.Cycles += 5
			if crossed {
				cpu_6502.Cycles++
			}

		case 0x87: // SAX Zero Page
			value := cpu_6502.A & cpu_6502.X
			cpu_6502.memory.Write(cpu_6502.getZeroPage(), value)
			cpu_6502.Cycles += 3

		case 0x97: // SAX Zero Page,Y
			value := cpu_6502.A & cpu_6502.X
			cpu_6502.memory.Write(cpu_6502.getZeroPageY(), value)
			cpu_6502.Cycles += 4

		case 0x8F: // SAX Absolute
			value := cpu_6502.A & cpu_6502.X
			cpu_6502.memory.Write(cpu_6502.getAbsolute(), value)
			cpu_6502.Cycles += 4

		case 0x83: // SAX (Indirect,X)
			value := cpu_6502.A & cpu_6502.X
			cpu_6502.memory.Write(cpu_6502.getIndirectX(), value)
			cpu_6502.Cycles += 6

		case 0xC7: // DCP Zero Page
			addr := cpu_6502.getZeroPage()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value - 1
				cpu_6502.compare(cpu_6502.A, result)
				return result
			})
			cpu_6502.Cycles += 5

		case 0xD7: // DCP Zero Page,X
			addr := cpu_6502.getZeroPageX()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value - 1
				cpu_6502.compare(cpu_6502.A, result)
				return result
			})
			cpu_6502.Cycles += 6

		case 0xCF: // DCP Absolute
			addr := cpu_6502.getAbsolute()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value - 1
				cpu_6502.compare(cpu_6502.A, result)
				return result
			})
			cpu_6502.Cycles += 6

		case 0xDF: // DCP Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value - 1
				cpu_6502.compare(cpu_6502.A, result)
				return result
			})
			cpu_6502.Cycles += 7

		case 0xDB: // DCP Absolute,Y
			addr, _ := cpu_6502.getAbsoluteY()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value - 1
				cpu_6502.compare(cpu_6502.A, result)
				return result
			})
			cpu_6502.Cycles += 7

		case 0xC3: // DCP (Indirect,X)
			addr := cpu_6502.getIndirectX()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value - 1
				cpu_6502.compare(cpu_6502.A, result)
				return result
			})
			cpu_6502.Cycles += 8

		case 0xD3: // DCP (Indirect),Y
			addr, _ := cpu_6502.getIndirectY()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value - 1
				cpu_6502.compare(cpu_6502.A, result)
				return result
			})
			cpu_6502.Cycles += 8

			// ISC (Increase memory then SBC) implementations
		case 0xE7: // ISC Zero Page
			addr := cpu_6502.getZeroPage()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value + 1
				cpu_6502.sbc(result)
				return result
			})
			cpu_6502.Cycles += 5

		case 0xF7: // ISC Zero Page,X
			addr := cpu_6502.getZeroPageX()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value + 1
				cpu_6502.sbc(result)
				return result
			})
			cpu_6502.Cycles += 6

		case 0xEF: // ISC Absolute
			addr := cpu_6502.getAbsolute()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value + 1
				cpu_6502.sbc(result)
				return result
			})
			cpu_6502.Cycles += 6

		case 0xFF: // ISC Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value + 1
				cpu_6502.sbc(result)
				return result
			})
			cpu_6502.Cycles += 7

		case 0xFB: // ISC Absolute,Y
			addr, _ := cpu_6502.getAbsoluteY()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value + 1
				cpu_6502.sbc(result)
				return result
			})
			cpu_6502.Cycles += 7

		case 0xE3: // ISC (Indirect,X)
			addr := cpu_6502.getIndirectX()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value + 1
				cpu_6502.sbc(result)
				return result
			})
			cpu_6502.Cycles += 8

		case 0xF3: // ISC (Indirect),Y
			addr, _ := cpu_6502.getIndirectY()
			cpu_6502.rmw(addr, func(value byte) byte {
				result := value + 1
				cpu_6502.sbc(result)
				return result
			})
			cpu_6502.Cycles += 8

		case 0x07: // SLO Zero Page
			addr := cpu_6502.getZeroPage()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := value << 1
				cpu_6502.A |= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 5

		case 0x17: // SLO Zero Page,X
			addr := cpu_6502.getZeroPageX()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := value << 1
				cpu_6502.A |= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 6

		case 0x0F: // SLO Absolute
			addr := cpu_6502.getAbsolute()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := value << 1
				cpu_6502.A |= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 6

		case 0x1F: // SLO Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := value << 1
				cpu_6502.A |= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 7

		case 0x1B: // SLO Absolute,Y
			addr, _ := cpu_6502.getAbsoluteY()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := value << 1
				cpu_6502.A |= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 7

		case 0x03: // SLO (Indirect,X)
			addr := cpu_6502.getIndirectX()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := value << 1
				cpu_6502.A |= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 8

		case 0x13: // SLO (Indirect),Y
			addr, _ := cpu_6502.getIndirectY()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := value << 1
				cpu_6502.A |= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 8

		case 0x27: // RLA Zero Page
			addr := cpu_6502.getZeroPage()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := (value << 1) | oldCarry
				cpu_6502.A &= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 5

		case 0x37: // RLA Zero Page,X
			addr := cpu_6502.getZeroPageX()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := (value << 1) | oldCarry
				cpu_6502.A &= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 6

		case 0x2F: // RLA Absolute
			addr := cpu_6502.getAbsolute()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := (value << 1) | oldCarry
				cpu_6502.A &= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 6

		case 0x3F: // RLA Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := (value << 1) | oldCarry
				cpu_6502.A &= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 7

		case 0x3B: // RLA Absolute,Y
			addr, _ := cpu_6502.getAbsoluteY()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := (value << 1) | oldCarry
				cpu_6502.A &= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 7

		case 0x23: // RLA (Indirect,X)
			addr := cpu_6502.getIndirectX()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := (value << 1) | oldCarry
				cpu_6502.A &= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 8

		case 0x33: // RLA (Indirect),Y
			addr, _ := cpu_6502.getIndirectY()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x80 != 0)
				result := (value << 1) | oldCarry
				cpu_6502.A &= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 8

		case 0x47: // SRE Zero Page
			addr := cpu_6502.getZeroPage()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := value >> 1
				cpu_6502.A ^= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 5

		case 0x57: // SRE Zero Page,X
			addr := cpu_6502.getZeroPageX()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := value >> 1
				cpu_6502.A ^= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 6

		case 0x4F: // SRE Absolute
			addr := cpu_6502.getAbsolute()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := value >> 1
				cpu_6502.A ^= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 6

		case 0x5F: // SRE Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := value >> 1
				cpu_6502.A ^= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 7

		case 0x5B: // SRE Absolute,Y
			addr, _ := cpu_6502.getAbsoluteY()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := value >> 1
				cpu_6502.A ^= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 7

		case 0x43: // SRE (Indirect,X)
			addr := cpu_6502.getIndirectX()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := value >> 1
				cpu_6502.A ^= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 8

		case 0x53: // SRE (Indirect),Y
			addr, _ := cpu_6502.getIndirectY()
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := value >> 1
				cpu_6502.A ^= result
				cpu_6502.setFlag(ZERO_FLAG, cpu_6502.A == 0)
				cpu_6502.setFlag(NEGATIVE_FLAG, cpu_6502.A&0x80 != 0)
				return result
			})
			cpu_6502.Cycles += 8

		case 0x67: // RRA Zero Page
			addr := cpu_6502.getZeroPage()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := (value >> 1) | (oldCarry << 7)
				cpu_6502.adc(result)
				return result
			})
			cpu_6502.Cycles += 5

		case 0x77: // RRA Zero Page,X
			addr := cpu_6502.getZeroPageX()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := (value >> 1) | (oldCarry << 7)
				cpu_6502.adc(result)
				return result
			})
			cpu_6502.Cycles += 6

		case 0x6F: // RRA Absolute
			addr := cpu_6502.getAbsolute()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := (value >> 1) | (oldCarry << 7)
				cpu_6502.adc(result)
				return result
			})
			cpu_6502.Cycles += 6

		case 0x7F: // RRA Absolute,X
			addr, _ := cpu_6502.getAbsoluteX()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := (value >> 1) | (oldCarry << 7)
				cpu_6502.adc(result)
				return result
			})
			cpu_6502.Cycles += 7

		case 0x7B: // RRA Absolute,Y
			addr, _ := cpu_6502.getAbsoluteY()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := (value >> 1) | (oldCarry << 7)
				cpu_6502.adc(result)
				return result
			})
			cpu_6502.Cycles += 7

		case 0x63: // RRA (Indirect,X)
			addr := cpu_6502.getIndirectX()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := (value >> 1) | (oldCarry << 7)
				cpu_6502.adc(result)
				return result
			})
			cpu_6502.Cycles += 8

		case 0x73: // RRA (Indirect),Y
			addr, _ := cpu_6502.getIndirectY()
			oldCarry := btou8(cpu_6502.getFlag(CARRY_FLAG))
			cpu_6502.rmw(addr, func(value byte) byte {
				cpu_6502.setFlag(CARRY_FLAG, value&0x01 != 0)
				result := (value >> 1) | (oldCarry << 7)
				cpu_6502.adc(result)
				return result
			})
			cpu_6502.Cycles += 8

		case 0x9C: // SHY Absolute,X
			addr := cpu_6502.getAbsolute() + uint16(cpu_6502.X)
			value := cpu_6502.Y & byte((addr>>8)+1)
			// If page boundary crossed, value becomes unstable
			if (addr&0xFF)+uint16(cpu_6502.X) > 0xFF {
				value &= byte(addr >> 8)
			}
			cpu_6502.memory.Write(addr, value)
			cpu_6502.Cycles += 5

		case 0x9E: // SHX Absolute,Y
			addr := cpu_6502.getAbsolute() + uint16(cpu_6502.Y)
			value := cpu_6502.X & byte((addr>>8)+1)
			if (addr&0xFF)+uint16(cpu_6502.Y) > 0xFF {
				value &= byte(addr >> 8)
			}
			cpu_6502.memory.Write(addr, value)
			cpu_6502.Cycles += 5

		default:
			fmt.Printf("Unknown opcode: %02X at PC=%04X\n", opcode, cpu_6502.PC-1)
			cpu_6502.Running = false
		}

		cpu_6502.mutex.Unlock()
	}
}

func (adapter *MemoryBusAdapter_6502) Read(addr uint16) byte {
	/*
	   Read performs 8-bit memory read.

	   Parameters:
	   - addr: Memory address

	   Returns:
	   - byte: Value at address

	   Operation:
	   1. Reads 32-bit word
	   2. Extracts correct byte
	*/

	if addr == VRAM_BANK_REG {
		return byte(adapter.vramBank & 0xFF)
	}

	if addr == VRAM_BANK_REG_RSVD {
		return 0
	}

	if translated, ok := adapter.translateVRAM(addr); ok {
		return adapter.bus.Read8(translated)
	}

	return adapter.bus.Read8(uint32(addr))
}
func (adapter *MemoryBusAdapter_6502) Write(addr uint16, value byte) {
	/*
	   Write performs 8-bit memory write.

	   Parameters:
	   - addr: Target address
	   - value: Byte to write

	   Operation:
	   1. Reads existing 32-bit word
	   2. Modifies correct byte
	   3. Writes back full word
	*/

	if addr == VRAM_BANK_REG {
		adapter.vramBank = uint32(value)
		adapter.vramEnabled = true
		return
	}

	if addr == VRAM_BANK_REG_RSVD {
		return
	}

	if translated, ok := adapter.translateVRAM(addr); ok {
		adapter.bus.Write8(translated, value)
		return
	}

	adapter.bus.Write8(uint32(addr), value)
}

func (adapter *MemoryBusAdapter_6502) ResetBank() {
	adapter.vramBank = 0
	adapter.vramEnabled = false
}

func (adapter *MemoryBusAdapter_6502) translateVRAM(addr uint16) (uint32, bool) {
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

func (adapter *MemoryBusAdapter_6502) isMappedIO(addr uint16) bool {
	bus, ok := adapter.bus.(*SystemBus)
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
