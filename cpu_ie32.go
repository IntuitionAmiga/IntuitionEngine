// cpu_ie32.go - Intuition Engine 32-bit RISC-like CPU

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

(c) 2024 - 2025 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
Buy me a coffee: https://ko-fi.com/intuition/tip

License: GPLv3 or later
*/

/*
cpu_ie32.go - 32-bit RISC-like CPU for the Intuition Engine

This module implements a complete 32-bit RISC-style CPU with memory-mapped I/O
capabilities, hardware interrupts, and debug features. The architecture follows
classic RISC design principles whilst adding features needed for retro-style
computer emulation.

Core Features:
- 16 general-purpose 32-bit registers
- Memory-mapped I/O capabilities
- Hardware interrupt support with priority levels
- Stack-based subroutine calls
- Multiple addressing modes
- Timer-based interrupt generation
- Debug/trace capabilities

Signal Flow:
1. Fetch instruction components from memory
2. Decode addressing mode and operands
3. Execute operation with bounds checking
4. Update program counter
5. Process any pending interrupts
6. Update timer state and counters
7. Synchronise memory access

Memory Layout Analysis (64-bit system):
Cache Line 0 (64 bytes) - Hot Path Registers:
- PC, SP, A           : Most frequently accessed
- X, Y, Z registers   : Index operations
- B-G registers       : General purpose (group 1)
- H, S registers      : General purpose (group 2)

Cache Line 1 (64 bytes) - Secondary Registers:
- T-W registers       : Less frequently used
- Running, Debug      : State flags
- Timer control       : State and counters
- Cycle counting      : Performance monitoring

Cache Line 2 (64 bytes) - Interrupt Control:
- Vector table        : Interrupt handlers
- Status flags        : Interrupt state
- Mutex controls      : Thread safety

Cache Lines 3+ - Main Memory:
- Program memory      : Instruction storage
- Stack space         : Call management
- I/O mapping         : Device interfaces

Thread Safety:
All memory access and state updates are protected by mutexes, allowing safe
interaction from external threads during execution. The implementation uses
read/write locks to optimise concurrent access patterns:
- Read locks for instruction fetch and memory reads
- Write locks for register updates and memory writes
- Separate mutex for timer state management
*/

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// ------------------------------------------------------------------------------
// Memory and Address Space Constants
// ------------------------------------------------------------------------------
const (
	WORD_SIZE       = 4                // Size of a word in bytes
	WORD_SIZE_BITS  = 32               // Word size in bits
	CACHE_LINE_SIZE = 64               // Cache line size in bytes
	MEMORY_SIZE     = 16 * 1024 * 1024 // 16MB total memory
)

// ------------------------------------------------------------------------------
// Instruction Format Constants
// ------------------------------------------------------------------------------
const (
	INSTRUCTION_SIZE = 8         // Total instruction size in bytes
	OPCODE_OFFSET    = 0         // Offset to opcode field
	REG_OFFSET       = 1         // Offset to register field
	ADDRMODE_OFFSET  = 2         // Offset to addressing mode
	OPERAND_OFFSET   = WORD_SIZE // Offset to operand data
)

// ------------------------------------------------------------------------------
// Register and Addressing Masks
// ------------------------------------------------------------------------------
const (
	REG_INDEX_MASK    = 0x0F       // Mask for register index
	REG_INDIRECT_MASK = 0x03       // Mask for indirect addressing
	OFFSET_MASK       = 0xFFFFFFFC // Mask for word-aligned offsets
)

// ------------------------------------------------------------------------------
// Addressing Modes
// ------------------------------------------------------------------------------
const (
	ADDR_IMMEDIATE = 0x00 // Immediate value
	ADDR_REGISTER  = 0x01 // Register direct
	ADDR_REG_IND   = 0x02 // Register indirect
	ADDR_MEM_IND   = 0x03 // Memory indirect
)

// ------------------------------------------------------------------------------
// Memory Map Boundaries
// ------------------------------------------------------------------------------
const (
	VECTOR_TABLE = 0x0000 // Interrupt vector table
	PROG_START   = 0x1000 // Program code start
	STACK_BOTTOM = 0x2000 // Stack bottom boundary
	STACK_START  = 0xE000 // Initial stack pointer
	IO_BASE      = 0xF800 // I/O register base
	IO_LIMIT     = 0xFFFF // I/O register limit
)

// ------------------------------------------------------------------------------
// I/O Register Locations
// ------------------------------------------------------------------------------
const (
	TIMER_COUNT  = IO_BASE + 0x04 // Timer current count
	TIMER_PERIOD = IO_BASE + 0x08 // Timer period value
)

// ------------------------------------------------------------------------------
// Timer States
// ------------------------------------------------------------------------------
const (
	TIMER_STOPPED = iota // Timer inactive
	TIMER_RUNNING        // Timer active
	TIMER_EXPIRED        // Timer reached zero
)

// ------------------------------------------------------------------------------
// Display Parameters
// ------------------------------------------------------------------------------
const (
	SCREEN_HEIGHT = 25 // Terminal height
	SCREEN_WIDTH  = 80 // Terminal width
)

// ------------------------------------------------------------------------------
// Display Characters
// ------------------------------------------------------------------------------
const (
	SCREEN_BORDER_H  = "─" // Horizontal border
	SCREEN_BORDER_V  = "│" // Vertical border
	SCREEN_BORDER_TL = "┌" // Top-left corner
	SCREEN_BORDER_TR = "┐" // Top-right corner
	SCREEN_BORDER_BL = "└" // Bottom-left corner
	SCREEN_BORDER_BR = "┘" // Bottom-right corner
	SCREEN_PIXEL     = "■" // Active pixel
)

// ------------------------------------------------------------------------------
// Timing Parameters
// ------------------------------------------------------------------------------
const (
	RESET_DELAY = 50 * time.Millisecond // CPU reset delay
)

// ------------------------------------------------------------------------------
// Base Instructions
// ------------------------------------------------------------------------------
const (
	// Data Movement
	LOAD  = 0x01 // Load value into register
	STORE = 0x02 // Store register to memory

	// Arithmetic Operations
	ADD = 0x03 // Add values
	SUB = 0x04 // Subtract values
	MUL = 0x14 // Multiply values
	DIV = 0x15 // Divide values
	MOD = 0x16 // Modulo operation

	// Logical Operations
	AND = 0x05 // Logical AND
	OR  = 0x09 // Logical OR
	XOR = 0x0A // Logical XOR
	NOT = 0x0D // Logical NOT
	SHL = 0x0B // Shift left
	SHR = 0x0C // Shift right

	// Control Flow
	JMP = 0x06 // Unconditional jump
	JNZ = 0x07 // Jump if not zero
	JZ  = 0x08 // Jump if zero
	JGT = 0x0E // Jump if greater than
	JGE = 0x0F // Jump if greater/equal
	JLT = 0x10 // Jump if less than
	JLE = 0x11 // Jump if less/equal

	// Stack Operations
	PUSH = 0x12 // Push to stack
	POP  = 0x13 // Pop from stack
	JSR  = 0x18 // Jump to subroutine
	RTS  = 0x19 // Return from subroutine

	// Interrupt Control
	SEI = 0x1A // Set interrupt enable
	CLI = 0x1B // Clear interrupt enable
	RTI = 0x1C // Return from interrupt

	// Timing
	WAIT = 0x17 // Wait specified cycles

	// System Control
	NOP  = 0xEE // No operation
	HALT = 0xFF // Halt processor
)

// ------------------------------------------------------------------------------
// Register Load/Store Operations
// ------------------------------------------------------------------------------
const (
	// Primary Register Operations
	LDA = 0x20 // Load accumulator
	LDX = 0x21 // Load X register
	LDY = 0x22 // Load Y register
	LDZ = 0x23 // Load Z register
	STA = 0x24 // Store accumulator
	STX = 0x25 // Store X register
	STY = 0x26 // Store Y register
	STZ = 0x27 // Store Z register

	// Register Increment/Decrement
	INC = 0x28 // Increment register/memory
	DEC = 0x29 // Decrement register/memory
)

// ------------------------------------------------------------------------------
// Extended Register Operations
// ------------------------------------------------------------------------------
const (
	// Extended Register Load
	LDB = 0x3A // Load B register
	LDC = 0x3B // Load C register
	LDD = 0x3C // Load D register
	LDE = 0x3D // Load E register
	LDF = 0x3E // Load F register
	LDG = 0x3F // Load G register
	LDU = 0x40 // Load U register
	LDV = 0x41 // Load V register
	LDW = 0x42 // Load W register
	LDH = 0x4C // Load H register
	LDS = 0x4D // Load S register
	LDT = 0x4E // Load T register

	// Extended Register Store
	STB = 0x43 // Store B register
	STC = 0x44 // Store C register
	STD = 0x45 // Store D register
	STE = 0x46 // Store E register
	STF = 0x47 // Store F register
	STG = 0x48 // Store G register
	STU = 0x49 // Store U register
	STV = 0x4A // Store V register
	STW = 0x4B // Store W register
	STH = 0x4F // Store H register
	STS = 0x50 // Store S register
	STT = 0x51 // Store T register
)

type CPU struct {
	/*
	   Cache Line 0 (64 bytes) - Hot Path Registers:
	   Optimised layout for frequent access patterns:
	   - PC, SP       : Program counter and stack pointer are accessed every instruction
	   - A            : Primary accumulator for arithmetic/logic
	   - X, Y, Z      : Index registers for addressing modes
	   - B through G  : General purpose registers (group 1)
	   - H, S         : General purpose registers (group 2)
	   - _padding0    : Ensures alignment

	   Cache Line 1 (64 bytes) - Secondary Registers:
	   Less frequently accessed state:
	   - T through W  : Additional general purpose registers
	   - Running      : CPU execution state
	   - Debug        : Debug mode flag
	   - timerState   : Timer control flags
	   - cycleCounter : Performance monitoring
	   - timerCount   : Timer current value
	   - timerPeriod  : Timer configuration
	   - _padding1    : Cache line alignment

	   Cache Line 2 (64 bytes) - Interrupt Management:
	   Synchronisation and control:
	   - InterruptVector  : Current interrupt handler
	   - InterruptEnabled : Global interrupt flag
	   - InInterrupt     : Interrupt processing flag
	   - timerEnabled    : Timer active state
	   - mutex           : Memory access control
	   - timerMutex      : Timer state control

	   Cache Lines 3+ - System Memory:
	   Bulk storage areas:
	   - Screen          : Display buffer [25][80]byte
	   - Memory          : Main memory []byte
	   - bus             : Memory interface
	*/

	// Hot path registers (Cache Line 0)
	PC        uint32  // Program counter
	SP        uint32  // Stack pointer
	A         uint32  // Accumulator
	X         uint32  // Index register X
	Y         uint32  // Index register Y
	Z         uint32  // Index register Z
	B         uint32  // General purpose B
	C         uint32  // General purpose C
	D         uint32  // General purpose D
	E         uint32  // General purpose E
	F         uint32  // General purpose F
	G         uint32  // General purpose G
	H         uint32  // General purpose H
	S         uint32  // General purpose S
	_padding0 [8]byte // Alignment padding

	// Secondary registers (Cache Line 1)
	T            uint32   // General purpose T
	U            uint32   // General purpose U
	V            uint32   // General purpose V
	W            uint32   // General purpose W
	Running      bool     // Execution state
	Debug        bool     // Debug enabled
	timerState   uint8    // Timer state
	_padding1    byte     // Alignment padding
	cycleCounter uint32   // Performance counter
	timerCount   uint32   // Timer counter
	timerPeriod  uint32   // Timer period
	_padding2    [32]byte // Cache line padding

	// Interrupt control (Cache Line 2)
	InterruptVector  uint32       // Interrupt handler
	InterruptEnabled bool         // Interrupts allowed
	InInterrupt      bool         // In handler
	timerEnabled     bool         // Timer active
	_padding3        byte         // Alignment padding
	mutex            sync.RWMutex // Memory lock
	timerMutex       sync.Mutex   // Timer lock

	// Large buffers (Cache Lines 3+)
	Screen [25][80]byte // Display buffer
	Memory []byte       // Main memory
	bus    MemoryBus    // Memory interface
}

func NewCPU(bus MemoryBus) *CPU {
	/*
	   NewCPU initialises a new CPU instance with default state.

	   Parameters:
	   - bus: Memory interface for I/O operations

	   Returns:
	   - *CPU: Initialised CPU instance

	   The function:
	   1. Allocates main memory
	   2. Sets default register values
	   3. Configures interrupt state
	   4. Initialises stack pointer
	   5. Establishes memory bus interface

	   Thread Safety:
	   Initial state setup requires no locks as object is not yet shared.
	*/

	cpu := &CPU{
		Memory:           make([]byte, MEMORY_SIZE),
		Running:          true,
		Debug:            false,
		SP:               STACK_START,
		PC:               PROG_START,
		InterruptEnabled: false,
		InInterrupt:      false,
		timerEnabled:     false,
		bus:              bus,
	}
	return cpu
}

func (cpu *CPU) LoadProgram(filename string) error {
	/*
	   LoadProgram loads a programme from a file into main memory.
	   It reads the file's contents and copies them into the memory area beginning at PROG_START,
	   after first clearing any existing data up to STACK_START. The programme counter is then reset to PROG_START.

	   Parameters:
	   - filename: The name of the programme file to load.

	   Returns:
	   - error: An error if the file cannot be read, or nil on success.
	*/

	program, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	// Clear program area with bounds check
	for i := PROG_START; i < len(cpu.Memory) && i < STACK_START; i++ {
		cpu.Memory[i] = 0
	}
	copy(cpu.Memory[PROG_START:], program)
	cpu.PC = PROG_START
	return nil
}

func (cpu *CPU) Write32(addr uint32, value uint32) {
	/*
	   Write32 performs a 32-bit write to memory with thread safety.

	   Parameters:
	   - addr: Target memory address
	   - value: Value to write

	   Thread Safety:
	   Full mutex lock during write operation to prevent concurrent access.
	*/

	cpu.mutex.Lock()
	defer cpu.mutex.Unlock()
	cpu.bus.Write32(addr, value)
}

func (cpu *CPU) Read32(addr uint32) uint32 {
	/*
	   Read32 performs a 32-bit read from memory with thread safety.

	   Parameters:
	   - addr: Source memory address

	   Returns:
	   - uint32: Value read from memory

	   Thread Safety:
	   Full mutex lock during read operation to prevent concurrent access.
	*/
	cpu.mutex.Lock()
	defer cpu.mutex.Unlock()
	return cpu.bus.Read32(addr)
}

func btou32(b bool) uint32 {
	/*
	   btou32 converts a boolean value to a 32-bit unsigned integer.

	   Parameters:
	   - b: The boolean value to convert.

	   Returns:
	   - uint32: 1 if b is true, or 0 if b is false.
	*/

	if b {
		return 1
	}
	return 0
}

func (cpu *CPU) HasScreenContent() bool {
	/*
	   HasScreenContent checks whether the screen buffer contains any content.
	   It iterates over the screen buffer and returns true if any pixel is non-zero.

	   Returns:
	   - bool: True if there is any content on the screen, otherwise false.
	*/

	for _, row := range cpu.Screen {
		for _, ch := range row {
			if ch != 0 {
				return true
			}
		}
	}
	return false
}

func (cpu *CPU) DisplayScreen() {
	/*
	   DisplayScreen renders the current screen buffer to the standard output.
	   It draws the top border, each row with side borders (displaying a space for zero-valued pixels
	   and SCREEN_PIXEL for non-zero pixels), and finally the bottom border.

	   This function does not return a value.
	*/

	fmt.Println("\n" + SCREEN_BORDER_TL + strings.Repeat(SCREEN_BORDER_H, SCREEN_WIDTH) + SCREEN_BORDER_TR)
	for _, row := range cpu.Screen {
		fmt.Print(SCREEN_BORDER_V)
		for _, pixel := range row {
			if pixel == 0 {
				fmt.Print(" ")
			} else {
				fmt.Print(SCREEN_PIXEL)
			}
		}
		fmt.Println(SCREEN_BORDER_V)
	}
	fmt.Println(SCREEN_BORDER_BL + strings.Repeat(SCREEN_BORDER_H, SCREEN_WIDTH) + SCREEN_BORDER_BR)
}

func (cpu *CPU) getRegister(reg byte) *uint32 {
	/*
	   getRegister retrieves a pointer to the CPU register specified by the given index.
	   The lower four bits of the supplied byte are used to determine the register as follows:
	     0  → A
	     1  → X
	     2  → Y
	     3  → Z
	     4  → B
	     5  → C
	     6  → D
	     7  → E
	     8  → F
	     9  → G
	     10 → H
	     11 → S
	     12 → T
	     13 → U
	     14 → V
	     15 → W
	   If no valid register is determined, a pointer to A is returned by default.

	   Parameters:
	   - reg: A byte whose lower four bits indicate the desired register.

	   Returns:
	   - *uint32: A pointer to the corresponding register.
	*/

	switch reg & REG_INDEX_MASK {

	case 0:
		return &cpu.A
	case 1:
		return &cpu.X
	case 2:
		return &cpu.Y
	case 3:
		return &cpu.Z
	case 4:
		return &cpu.B
	case 5:
		return &cpu.C
	case 6:
		return &cpu.D
	case 7:
		return &cpu.E
	case 8:
		return &cpu.F
	case 9:
		return &cpu.G
	case 10:
		return &cpu.H
	case 11:
		return &cpu.S
	case 12:
		return &cpu.T
	case 13:
		return &cpu.U
	case 14:
		return &cpu.V
	case 15:
		return &cpu.W
	}
	return &cpu.A
}

func (cpu *CPU) Push(value uint32) bool {
	/*
	   Push adds a 32-bit value to the stack.

	   Parameters:
	   - value: Value to push onto stack

	   Returns:
	   - bool: True if successful, false if stack overflow

	   Stack Management:
	   1. Checks for stack overflow against STACK_BOTTOM
	   2. Decrements stack pointer
	   3. Writes value to new stack position
	   4. Logs operation if debug enabled

	   Thread Safety:
	   Protected by memory write mutex.
	*/

	if cpu.SP <= STACK_BOTTOM {
		fmt.Printf("%s cpu.Push\tStack overflow error at PC=%08x (SP=%08x)\n",
			time.Now().Format("15:04:05.000"), cpu.PC, cpu.SP)
		cpu.Running = false
		return false
	}
	cpu.SP -= WORD_SIZE
	cpu.Write32(cpu.SP, value)
	if cpu.Debug {
		fmt.Printf("PUSH: %08x to SP=%08x\n", value, cpu.SP)
	}
	return true
}

func (cpu *CPU) Pop() (uint32, bool) {
	/*
	   Pop removes and returns the top 32-bit value from stack.

	   Returns:
	   - uint32: Popped value
	   - bool: True if successful, false if stack underflow

	   Stack Management:
	   1. Checks for stack underflow against STACK_START
	   2. Reads value from current stack position
	   3. Increments stack pointer
	   4. Logs operation if debug enabled

	   Thread Safety:
	   Protected by memory read mutex.
	*/

	if cpu.SP >= STACK_START {
		fmt.Printf("Stack underflow error at PC=%08x (SP=%08x)\n", cpu.PC, cpu.SP)
		cpu.Running = false
		return 0, false
	}
	value := cpu.Read32(cpu.SP)
	if cpu.Debug {
		fmt.Printf("POP: %08x from SP=%08x\n", value, cpu.SP)
	}
	cpu.SP += WORD_SIZE
	return value, true
}

func (cpu *CPU) DumpStack() {
	/*
	   DumpStack displays current stack contents for debugging.

	   Output Format:
	   - Hexadecimal address and value pairs
	   - Displayed from current SP to STACK_START
	   - Empty stack notification if SP at STACK_START

	   Thread Safety:
	   Protected by memory read mutex during dump.
	*/

	if cpu.SP >= STACK_START {
		fmt.Println("Stack is empty")
		return
	}
	fmt.Println("\nStack contents:")
	for addr := cpu.SP; addr < STACK_START; addr += WORD_SIZE {
		value := cpu.Read32(addr)
		fmt.Printf("  %0*x: %0*x\n", WORD_SIZE_BITS/4, addr, WORD_SIZE_BITS/4, value)
	}
	fmt.Println()
}

func (cpu *CPU) resolveOperand(addrMode byte, operand uint32) uint32 {
	/*
	   resolveOperand handles address mode resolution and memory access.

	   Parameters:
	   - addrMode: Addressing mode (IMMEDIATE/REGISTER/REG_IND/MEM_IND)
	   - operand: Raw operand value

	   Returns:
	   - uint32: Resolved value based on addressing mode

	   Resolution Process:
	   1. Checks I/O region access (IO_BASE to IO_LIMIT)
	   2. Resolves based on addressing mode:
	      - Immediate: Direct value
	      - Register: Register contents
	      - Register Indirect: Memory at register + offset
	      - Memory Indirect: Memory at address

	   Thread Safety:
	   Protected by memory read mutex during resolution.
	*/

	if operand >= IO_BASE && operand <= IO_LIMIT {
		return cpu.Read32(operand)
	}

	switch addrMode {
	case ADDR_IMMEDIATE:
		return operand
	case ADDR_REGISTER:
		return *cpu.getRegister(byte(operand & REG_INDIRECT_MASK))
	case ADDR_REG_IND:
		reg := byte(operand & REG_INDIRECT_MASK)
		offset := operand & OFFSET_MASK
		addr := *cpu.getRegister(reg) + offset
		return cpu.Read32(addr)
	case ADDR_MEM_IND:
		return cpu.Read32(operand)
	}
	return 0
}

func (cpu *CPU) checkInterrupts() bool {
	/*
	   checkInterrupts verifies if interrupt processing is needed.

	   Returns:
	   - bool: True if interrupt should be processed

	   Checks:
	   1. Timer enabled
	   2. Interrupts globally enabled
	   3. Not currently in interrupt handler
	   4. Timer has expired

	   Thread Safety:
	   Uses atomic operations for timer state checks.
	*/

	if cpu.timerEnabled && cpu.InterruptEnabled && !cpu.InInterrupt {
		return cpu.timerCount == 0
	}
	return false
}

func (cpu *CPU) handleInterrupt() {
	/*
	   handleInterrupt processes pending interrupts.

	   Interrupt Flow:
	   1. Verifies interrupt enabled and not in handler
	   2. Sets interrupt processing flag
	   3. Pushes return address
	   4. Loads handler address from vector table
	   5. Updates program counter

	   Thread Safety:
	   Protected by main CPU mutex during processing.
	*/

	if !cpu.InterruptEnabled || cpu.InInterrupt {
		return
	}
	cpu.InInterrupt = true
	if !cpu.Push(cpu.PC) {
		return
	}
	// Jump to the ISR address from the vector table
	cpu.PC = cpu.Read32(VECTOR_TABLE)
}

func (cpu *CPU) Reset() {
	/*
	   Reset performs a complete CPU state reset.

	   Reset Process:
	   1. Disables CPU execution
	   2. Disables video output
	   3. Waits RESET_DELAY duration
	   4. Clears memory in CACHE_LINE_SIZE chunks
	   5. Resets video state
	   6. Re-enables CPU

	   Thread Safety:
	   Full mutex protection during entire reset sequence.
	*/

	cpu.mutex.Lock()
	cpu.Running = false

	if activeFrontend != nil && activeFrontend.video != nil {
		video := activeFrontend.video
		video.mutex.Lock()
		video.enabled = false
		video.hasContent = false
		video.mutex.Unlock()
	}
	cpu.mutex.Unlock()

	time.Sleep(RESET_DELAY)

	cpu.mutex.Lock()
	// Clear memory in chunks for better cache utilization
	for i := PROG_START; i < len(cpu.Memory); i += CACHE_LINE_SIZE {
		end := i + CACHE_LINE_SIZE
		if end > len(cpu.Memory) {
			end = len(cpu.Memory)
		}
		for j := i; j < end; j++ {
			cpu.Memory[j] = 0
		}
	}

	if activeFrontend != nil && activeFrontend.video != nil {
		video := activeFrontend.video
		video.mutex.Lock()
		for i := range video.prevVRAM {
			video.prevVRAM[i] = 0
		}
		video.enabled = true
		video.mutex.Unlock()
	}

	cpu.Running = true
	cpu.mutex.Unlock()
}

func (cpu *CPU) Execute() {
	/*
	   Execute runs the main CPU instruction cycle.

	   Execution Flow:
	   1. Validate initial PC value
	   2. Enter main execution loop:
	      - Cache frequently accessed values
	      - Fetch instruction components
	      - Process timer updates
	      - Execute instruction based on opcode
	      - Update program counter
	      - Handle interrupts

	   Instruction Processing:
	   - Fetches opcode, register, addressing mode, operand
	   - Resolves operand based on addressing mode
	   - Updates timer state if enabled
	   - Executes appropriate operation
	   - Updates PC unless modified by instruction

	   Timer Management:
	   - Increments cycle counter
	   - Processes timer updates at SAMPLE_RATE
	   - Triggers interrupts on timer expiry

	   Thread Safety:
	   - Main execution protected by CPU mutex
	   - Timer operations use separate mutex
	   - Memory access synchronised via bus interface
	*/

	if cpu.PC < PROG_START || cpu.PC >= STACK_START {
		fmt.Printf("Error: Invalid initial PC value: 0x%08x\n", cpu.PC)
		cpu.Running = false
		return
	}

	for cpu.Running {
		// Cache frequently accessed values
		currentPC := cpu.PC
		mem := cpu.Memory

		// Fetch instruction components
		opcode := mem[currentPC+OPCODE_OFFSET]
		reg := mem[currentPC+REG_OFFSET]
		addrMode := mem[currentPC+ADDRMODE_OFFSET]
		operand := binary.LittleEndian.Uint32(mem[currentPC+OPERAND_OFFSET : currentPC+INSTRUCTION_SIZE])
		resolvedOperand := cpu.resolveOperand(addrMode, operand)

		// Timer handling with atomic operations
		if cpu.timerEnabled {
			cpu.cycleCounter++
			if cpu.cycleCounter >= SAMPLE_RATE {
				cpu.cycleCounter = 0

				cpu.timerMutex.Lock()
				if cpu.timerCount > 0 {
					cpu.timerCount--
					if cpu.timerCount == 0 {
						cpu.timerState = TIMER_EXPIRED
						if cpu.InterruptEnabled && !cpu.InInterrupt {
							cpu.handleInterrupt()
						}
						if cpu.timerEnabled {
							cpu.timerCount = cpu.Read32(TIMER_PERIOD)
						}
					}
				}
				cpu.timerMutex.Unlock()

				binary.LittleEndian.PutUint32(
					cpu.Memory[TIMER_COUNT:TIMER_COUNT+WORD_SIZE],
					cpu.timerCount)
			}
		}

		switch opcode {
		case LOAD:
			/*
			   Load value to register based on addressing mode.

			   Operation:
			   1. Gets target register
			   2. Sets register value from resolved operand
			   3. Advances PC by INSTRUCTION_SIZE
			*/
			*cpu.getRegister(reg) = resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case LDA, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW, LDX, LDY, LDZ:
			/*
			   Load specific register with resolved operand.

			   Operation:
			   1. Selects destination register based on opcode
			   2. Loads resolved operand value
			   3. Advances PC by INSTRUCTION_SIZE
			*/
			var dst *uint32
			switch opcode {
			case LDA:
				dst = &cpu.A
			case LDB:
				dst = &cpu.B
			case LDC:
				dst = &cpu.C
			case LDD:
				dst = &cpu.D
			case LDE:
				dst = &cpu.E
			case LDF:
				dst = &cpu.F
			case LDG:
				dst = &cpu.G
			case LDH:
				dst = &cpu.H
			case LDS:
				dst = &cpu.S
			case LDT:
				dst = &cpu.T
			case LDU:
				dst = &cpu.U
			case LDV:
				dst = &cpu.V
			case LDW:
				dst = &cpu.W
			case LDX:
				dst = &cpu.X
			case LDY:
				dst = &cpu.Y
			case LDZ:
				dst = &cpu.Z
			}
			*dst = resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case STORE:
			/*
			   Store register value to memory location.

			   Operation:
			   1. Handles different addressing modes:
			      - Register indirect: Uses register + offset
			      - Memory indirect: Uses memory address
			      - Direct: Uses operand as address
			   2. Writes register value to resolved location
			   3. Advances PC by INSTRUCTION_SIZE
			*/
			if addrMode == ADDR_REG_IND {
				addr := *cpu.getRegister(byte(operand & REG_INDIRECT_MASK)) + (operand & OFFSET_MASK)
				cpu.Write32(addr, *cpu.getRegister(reg))
			} else if addrMode == ADDR_MEM_IND {
				addr := cpu.Read32(operand)
				cpu.Write32(addr, *cpu.getRegister(reg))
			} else {
				cpu.Write32(operand, *cpu.getRegister(reg))
			}
			cpu.PC += INSTRUCTION_SIZE

		case STA, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW, STX, STY, STZ:
			/*
			   Store specific register to memory.

			   Operation:
			   1. Select source register based on opcode
			   2. Resolve target address based on mode:
			      - Register indirect: Base + offset
			      - Memory indirect: Indirect address
			      - Direct: Operand address
			   3. Write register value to memory
			   4. Advance PC by INSTRUCTION_SIZE
			*/
			var value uint32
			switch opcode {
			case STA:
				value = cpu.A
			case STB:
				value = cpu.B
			case STC:
				value = cpu.C
			case STD:
				value = cpu.D
			case STE:
				value = cpu.E
			case STF:
				value = cpu.F
			case STG:
				value = cpu.G
			case STH:
				value = cpu.H
			case STS:
				value = cpu.S
			case STT:
				value = cpu.T
			case STU:
				value = cpu.U
			case STV:
				value = cpu.V
			case STW:
				value = cpu.W
			case STX:
				value = cpu.X
			case STY:
				value = cpu.Y
			case STZ:
				value = cpu.Z
			}

			if addrMode == ADDR_REG_IND {
				addr := *cpu.getRegister(byte(operand & REG_INDIRECT_MASK)) + (operand & OFFSET_MASK)
				cpu.Write32(addr, value)
			} else if addrMode == ADDR_MEM_IND {
				addr := cpu.Read32(operand)
				cpu.Write32(addr, value)
			} else {
				cpu.Write32(operand, value)
			}
			cpu.PC += INSTRUCTION_SIZE

		case ADD:
			/*
			   Add resolved operand to target register.

			   Operation:
			   1. Get target register from instruction
			   2. Add resolved operand to register value
			   3. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			*targetReg += resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case SUB:
			/*
			   Subtract resolved operand from target register.

			   Operation:
			   1. Get target register from instruction
			   2. If operand > register, set register to 0
			   3. Otherwise subtract operand from register
			   4. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			if resolvedOperand > *targetReg {
				*targetReg = 0
			} else {
				*targetReg -= resolvedOperand
			}
			cpu.PC += INSTRUCTION_SIZE

		case AND:
			/*
			   Logical AND on target register.
			   Operation:
			   1. Get target register
			   2. AND with resolved operand
			   3. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			*targetReg &= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case OR:
			/*
			   Logical OR on target register.
			   Operation:
			   1. Get target register
			   2. OR with resolved operand
			   3. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			*targetReg |= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case XOR:
			/*
			   Logical XOR on target register.
			   Operation:
			   1. Get target register
			   2. XOR with resolved operand
			   3. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			*targetReg ^= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case SHL:
			/*
			   Shift left target register.
			   Operation:
			   1. Get target register
			   2. Shift left by operand bits
			   3. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			*targetReg <<= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case SHR:
			/*
			   Shift right target register.
			   Operation:
			   1. Get target register
			   2. Shift right by operand bits
			   3. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			*targetReg >>= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case NOT:
			/*
			   Logical NOT on target register.
			   Operation:
			   1. Get target register
			   2. Perform bitwise NOT
			   3. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			*targetReg = ^(*targetReg)
			cpu.PC += INSTRUCTION_SIZE

		case JMP:
			/*
			   Unconditional jump to operand address.
			   Operation:
			   Sets PC directly to operand value
			*/
			cpu.PC = operand

		case JNZ:
			/*
			   Jump if register not zero.
			   Operation:
			   1. Get source register
			   2. Get target address
			   3. If register != 0, set PC to target
			   4. Else advance PC by INSTRUCTION_SIZE
			*/
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if *cpu.getRegister(reg) != 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JZ:
			/*
			   Jump if register zero.
			   Operation:
			   1. Get source register
			   2. Get target address
			   3. If register == 0, set PC to target
			   4. Else advance PC by INSTRUCTION_SIZE
			*/
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if *cpu.getRegister(reg) == 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JGT:
			/*
			   Jump if register greater than zero.
			   Operation:
			   1. Get signed register value
			   2. If value > 0, set PC to target
			   3. Else advance PC by INSTRUCTION_SIZE
			*/
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if int32(*cpu.getRegister(reg)) > 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JGE:
			/*
			   Jump if register greater/equal zero.
			   Operation:
			   1. Get signed register value
			   2. If value >= 0, set PC to target
			   3. Else advance PC by INSTRUCTION_SIZE
			*/
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if int32(*cpu.getRegister(reg)) >= 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JLT:
			/*
			   Jump if register less than zero.
			   Operation:
			   1. Get signed register value
			   2. If value < 0, set PC to target
			   3. Else advance PC by INSTRUCTION_SIZE
			*/
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if int32(*cpu.getRegister(reg)) < 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JLE:
			/*
			   Jump if register less/equal zero.
			   Operation:
			   1. Get signed register value
			   2. If value <= 0, set PC to target
			   3. Else advance PC by INSTRUCTION_SIZE
			*/
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if int32(*cpu.getRegister(reg)) <= 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case PUSH:
			/*
			   Push register value to stack.
			   Operation:
			   1. Get register value
			   2. Push to stack, check for overflow
			   3. Return if stack overflow
			   4. Advance PC by INSTRUCTION_SIZE
			*/
			value := *cpu.getRegister(reg)
			if !cpu.Push(value) {
				return
			}
			cpu.PC += INSTRUCTION_SIZE

		case POP:
			/*
			   Pop stack value to register.
			   Operation:
			   1. Pop value, check for underflow
			   2. Return if stack underflow
			   3. Store to target register
			   4. Advance PC by INSTRUCTION_SIZE
			*/
			value, ok := cpu.Pop()
			if !ok {
				return
			}
			*cpu.getRegister(reg) = value
			cpu.PC += INSTRUCTION_SIZE

		case MUL:
			/*
			   Multiply register by operand.
			   Operation:
			   1. Get target register
			   2. Multiply by resolved operand
			   3. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			*targetReg *= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case DIV:
			/*
			   Divide register by operand.
			   Operation:
			   1. Check for divide by zero
			   2. If zero, halt CPU with error
			   3. Else perform division
			   4. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			if resolvedOperand == 0 {
				fmt.Printf("Division by zero error at PC=%08x\n", cpu.PC)
				cpu.Running = false
				break
			}
			*targetReg /= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case MOD:
			/*
			   Modulo operation on register.
			   Operation:
			   1. Check for zero operand
			   2. If zero, halt CPU with error
			   3. Else compute modulo
			   4. Advance PC by INSTRUCTION_SIZE
			*/
			targetReg := cpu.getRegister(reg)
			if resolvedOperand == 0 {
				fmt.Printf("Division by zero error at PC=%08x\n", cpu.PC)
				cpu.Running = false
				break
			}
			*targetReg %= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case WAIT:
			/*
			   Wait specified cycles.
			   Operation:
			   1. Get delay count from operand
			   2. Execute delay loop
			   3. Advance PC by INSTRUCTION_SIZE
			*/
			targetTime := resolvedOperand
			for i := uint32(0); i < targetTime; i++ {
			}
			cpu.PC += INSTRUCTION_SIZE

		case JSR:
			/*
			   Jump to subroutine.
			   Operation:
			   1. Calculate return address
			   2. Push return address to stack
			   3. Jump to target address
			*/
			retAddr := cpu.PC + INSTRUCTION_SIZE
			if !cpu.Push(retAddr) {
				return
			}
			cpu.PC = operand

		case RTS:
			/*
			   Return from subroutine.
			   Operation:
			   1. Pop return address
			   2. Check for stack underflow
			   3. Set PC to return address
			*/
			retAddr, ok := cpu.Pop()
			if !ok {
				return
			}
			cpu.PC = retAddr

		case SEI:
			/*
			   Set interrupt enable flag.
			   Operation:
			   1. Enable interrupts
			   2. Advance PC by INSTRUCTION_SIZE
			*/
			cpu.InterruptEnabled = true
			cpu.PC += INSTRUCTION_SIZE

		case CLI:
			/*
			   Clear interrupt enable flag.
			   Operation:
			   1. Disable interrupts
			   2. Advance PC by INSTRUCTION_SIZE
			*/
			cpu.InterruptEnabled = false
			cpu.PC += INSTRUCTION_SIZE

		case RTI:
			/*
			   Return from interrupt.
			   Operation:
			   1. Pop return PC from stack
			   2. Clear interrupt processing flag
			   3. Set PC to return address
			*/
			returnPC, ok := cpu.Pop()
			if !ok {
				return
			}
			cpu.PC = returnPC
			cpu.InInterrupt = false

		case INC:
			/*
			   Increment value based on mode.
			   Operation:
			   1. Resolve target by mode:
			      - Register: Direct increment
			      - Register indirect: Memory at reg+offset
			      - Memory indirect: Memory at address
			   2. Increment value
			   3. Store result
			   4. Advance PC by INSTRUCTION_SIZE
			*/
			if addrMode == ADDR_REGISTER {
				reg := cpu.getRegister(byte(operand & REG_INDIRECT_MASK))
				(*reg)++
			} else if addrMode == ADDR_REG_IND {
				addr := *cpu.getRegister(byte(operand & REG_INDIRECT_MASK)) + (operand & OFFSET_MASK)
				val := cpu.Read32(addr)
				cpu.Write32(addr, val+1)
			} else if addrMode == ADDR_MEM_IND {
				addr := cpu.Read32(operand)
				val := cpu.Read32(addr)
				cpu.Write32(addr, val+1)
			} else {
				val := cpu.Read32(operand)
				cpu.Write32(operand, val+1)
			}
			cpu.PC += INSTRUCTION_SIZE

		case DEC:
			/*
			   Decrement value based on mode.
			   Operation:
			   1. Resolve target by mode:
			      - Register: Direct decrement
			      - Register indirect: Memory at reg+offset
			      - Memory indirect: Memory at address
			   2. Decrement value
			   3. Store result
			   4. Advance PC by INSTRUCTION_SIZE
			*/
			if addrMode == ADDR_REGISTER {
				reg := cpu.getRegister(byte(operand & REG_INDIRECT_MASK))
				(*reg)--
			} else if addrMode == ADDR_REG_IND {
				addr := *cpu.getRegister(byte(operand & REG_INDIRECT_MASK)) + (operand & OFFSET_MASK)
				val := cpu.Read32(addr)
				cpu.Write32(addr, val-1)
			} else if addrMode == ADDR_MEM_IND {
				addr := cpu.Read32(operand)
				val := cpu.Read32(addr)
				cpu.Write32(addr, val-1)
			} else {
				val := cpu.Read32(operand)
				cpu.Write32(operand, val-1)
			}
			cpu.PC += INSTRUCTION_SIZE

		case NOP:
			/*
			   No operation.
			   Operation:
			   Advance PC by INSTRUCTION_SIZE
			*/
			cpu.PC += INSTRUCTION_SIZE

		case HALT:
			/*
			   Halt CPU execution.
			   Operation:
			   1. Log halt with PC
			   2. Set Running to false
			*/
			fmt.Printf("HALT executed at PC=%08x\n", cpu.PC)
			cpu.Running = false

		default:
			fmt.Printf("Invalid opcode: %02x at PC=%08x\n", opcode, cpu.PC)
			cpu.Running = false
		}
	}

	if cpu.HasScreenContent() {
		cpu.DisplayScreen()
	}
}
