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

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// Basic CPU parameters
	WORD_SIZE       = 4
	WORD_SIZE_BITS  = 32
	CACHE_LINE_SIZE = 64
	MEMORY_SIZE     = 16 * 1024 * 1024

	// Instruction format
	INSTRUCTION_SIZE = 8
	OPCODE_OFFSET    = 0
	REG_OFFSET       = 1
	ADDRMODE_OFFSET  = 2
	OPERAND_OFFSET   = WORD_SIZE

	// Register masks
	REG_INDEX_MASK    = 0x0F
	REG_INDIRECT_MASK = 0x03
	OFFSET_MASK       = 0xFFFFFFFC
)

const (
	// Address modes
	ADDR_IMMEDIATE = 0x00
	ADDR_REGISTER  = 0x01
	ADDR_REG_IND   = 0x02
	ADDR_MEM_IND   = 0x03
)

const (
	// Memory map
	VECTOR_TABLE = 0x0000
	PROG_START   = 0x1000
	STACK_BOTTOM = 0x2000
	STACK_START  = 0xE000
	IO_BASE      = 0xF800
	IO_LIMIT     = 0xFFFF

	// I/O registers
	TIMER_COUNT  = IO_BASE + 0x04
	TIMER_PERIOD = IO_BASE + 0x08
)

const (
	// Timer states
	TIMER_STOPPED = iota
	TIMER_RUNNING
	TIMER_EXPIRED
)

const (
	// Screen parameters
	SCREEN_HEIGHT = 25
	SCREEN_WIDTH  = 80

	// Screen characters
	SCREEN_BORDER_H  = "─"
	SCREEN_BORDER_V  = "│"
	SCREEN_BORDER_TL = "┌"
	SCREEN_BORDER_TR = "┐"
	SCREEN_BORDER_BL = "└"
	SCREEN_BORDER_BR = "┘"
	SCREEN_PIXEL     = "■"
)

const (
	// Timing
	RESET_DELAY = 50 * time.Millisecond
)

const (
	// Base instructions
	LOAD  = 0x01
	STORE = 0x02
	ADD   = 0x03
	SUB   = 0x04
	AND   = 0x05
	JMP   = 0x06
	JNZ   = 0x07
	JZ    = 0x08
	OR    = 0x09
	XOR   = 0x0A
	SHL   = 0x0B
	SHR   = 0x0C
	NOT   = 0x0D
	JGT   = 0x0E
	JGE   = 0x0F
	JLT   = 0x10
	JLE   = 0x11
	PUSH  = 0x12
	POP   = 0x13
	MUL   = 0x14
	DIV   = 0x15
	MOD   = 0x16
	WAIT  = 0x17
	JSR   = 0x18
	RTS   = 0x19
	SEI   = 0x1A
	CLI   = 0x1B
	RTI   = 0x1C

	// Register load/store
	LDA = 0x20
	LDX = 0x21
	LDY = 0x22
	LDZ = 0x23
	STA = 0x24
	STX = 0x25
	STY = 0x26
	STZ = 0x27
	INC = 0x28
	DEC = 0x29

	// Extended register load
	LDB = 0x3A
	LDC = 0x3B
	LDD = 0x3C
	LDE = 0x3D
	LDF = 0x3E
	LDG = 0x3F
	LDU = 0x40
	LDV = 0x41
	LDW = 0x42
	LDH = 0x4C
	LDS = 0x4D
	LDT = 0x4E

	// Extended register store
	STB = 0x43
	STC = 0x44
	STD = 0x45
	STE = 0x46
	STF = 0x47
	STG = 0x48
	STU = 0x49
	STV = 0x4A
	STW = 0x4B
	STH = 0x4F
	STS = 0x50
	STT = 0x51

	// System
	NOP  = 0xEE
	HALT = 0xFF
)

type CPU struct {
	/*
	   Memory Layout Analysis (64-bit system):

	   Cache Line 0 (64 bytes) - Hot Path Registers:
	   - PC            : offset 0,  size 4  - Program Counter, most accessed
	   - SP            : offset 4,  size 4  - Stack Pointer, high frequency
	   - A             : offset 8,  size 4  - Primary accumulator
	   - X, Y, Z       : offset 12, size 12 - Index registers
	   - B through G   : offset 24, size 24 - General purpose group 1
	   - H, S          : offset 48, size 8  - General purpose group 2
	   - _padding0     : offset 56, size 8  - Maintain alignment

	   Cache Line 1 (64 bytes) - Secondary Registers:
	   - T through W   : offset 64, size 16 - Less used registers
	   - Running       : offset 80, size 1  - CPU state
	   - Debug         : offset 81, size 1  - Debug flag
	   - timerState    : offset 82, size 1  - Timer status
	   - _padding1     : offset 83, size 1  - Explicit padding
	   - cycleCounter  : offset 84, size 4  - Timer tracking
	   - timerCount    : offset 88, size 4  - Timer value
	   - timerPeriod   : offset 92, size 4  - Timer config
	   - _padding2     : offset 96, size 32 - Line alignment

	   Cache Line 2 (64 bytes) - Interrupt Control:
	   - InterruptVector   : offset 128, size 4
	   - InterruptEnabled  : offset 132, size 1
	   - InInterrupt      : offset 133, size 1
	   - timerEnabled     : offset 134, size 1
	   - _padding3        : offset 135, size 1
	   - mutex            : offset 136, size 8
	   - timerMutex       : offset 144, size 8

	   Cache Lines 3+ (remaining):
	   - Screen          : [25][80]byte - Fixed display buffer
	   - Memory          : []byte       - Program/data memory
	   - bus             : MemoryBus    - Memory interface

	   Benefits:
	   1. Most accessed registers in first cache line
	   2. Timer/interrupt fields grouped by usage
	   3. Explicit padding for alignment visibility
	   4. Mutex aligned for atomic operations
	   5. Screen buffer aligned to cache line
	   6. Memory slice for dynamic allocation
	*/

	// Hot path registers (Cache Line 0)
	PC        uint32
	SP        uint32
	A         uint32
	X         uint32
	Y         uint32
	Z         uint32
	B         uint32
	C         uint32
	D         uint32
	E         uint32
	F         uint32
	G         uint32
	H         uint32
	S         uint32
	_padding0 [8]byte

	// Secondary registers (Cache Line 1)
	T            uint32
	U            uint32
	V            uint32
	W            uint32
	Running      bool
	Debug        bool
	timerState   uint8
	_padding1    byte
	cycleCounter uint32
	timerCount   uint32
	timerPeriod  uint32
	_padding2    [32]byte

	// Interrupt control (Cache Line 2)
	InterruptVector  uint32
	InterruptEnabled bool
	InInterrupt      bool
	timerEnabled     bool
	_padding3        byte
	mutex            sync.RWMutex
	timerMutex       sync.Mutex

	// Large buffers (Cache Lines 3+)
	Screen [25][80]byte
	Memory []byte
	bus    MemoryBus
}

func NewCPU(bus MemoryBus) *CPU {
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
	cpu.mutex.Lock()
	defer cpu.mutex.Unlock()
	cpu.bus.Write32(addr, value)
}

func (cpu *CPU) Read32(addr uint32) uint32 {
	cpu.mutex.Lock()
	defer cpu.mutex.Unlock()
	return cpu.bus.Read32(addr)
}

func btou32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

func (cpu *CPU) HasScreenContent() bool {
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
	if cpu.timerEnabled && cpu.InterruptEnabled && !cpu.InInterrupt {
		return cpu.timerCount == 0
	}
	return false
}

func (cpu *CPU) handleInterrupt() {
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
			*cpu.getRegister(reg) = resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case LDA, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW, LDX, LDY, LDZ:
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
			// Get the target register from the instruction
			targetReg := cpu.getRegister(reg)
			*targetReg += resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case SUB:
			// Simplified subtraction that works with any register
			targetReg := cpu.getRegister(reg)
			if resolvedOperand > *targetReg {
				*targetReg = 0
			} else {
				*targetReg -= resolvedOperand
			}
			cpu.PC += INSTRUCTION_SIZE

		case AND:
			targetReg := cpu.getRegister(reg)
			*targetReg &= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case OR:
			targetReg := cpu.getRegister(reg)
			*targetReg |= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case XOR:
			targetReg := cpu.getRegister(reg)
			*targetReg ^= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case SHL:
			targetReg := cpu.getRegister(reg)
			*targetReg <<= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case SHR:
			targetReg := cpu.getRegister(reg)
			*targetReg >>= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case NOT:
			targetReg := cpu.getRegister(reg)
			*targetReg = ^(*targetReg)
			cpu.PC += INSTRUCTION_SIZE

		case JMP:
			cpu.PC = operand

		case JNZ:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if *cpu.getRegister(reg) != 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JZ:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if *cpu.getRegister(reg) == 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JGT:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if int32(*cpu.getRegister(reg)) > 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JGE:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if int32(*cpu.getRegister(reg)) >= 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JLT:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if int32(*cpu.getRegister(reg)) < 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case JLE:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+WORD_SIZE : currentPC+INSTRUCTION_SIZE])
			if int32(*cpu.getRegister(reg)) <= 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += INSTRUCTION_SIZE
			}

		case PUSH:
			value := *cpu.getRegister(reg)
			if !cpu.Push(value) {
				return
			}
			cpu.PC += INSTRUCTION_SIZE

		case POP:
			value, ok := cpu.Pop()
			if !ok {
				return
			}
			*cpu.getRegister(reg) = value
			cpu.PC += INSTRUCTION_SIZE

		case MUL:
			targetReg := cpu.getRegister(reg)
			*targetReg *= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case DIV:
			targetReg := cpu.getRegister(reg)
			if resolvedOperand == 0 {
				fmt.Printf("Division by zero error at PC=%08x\n", cpu.PC)
				cpu.Running = false
				break
			}
			*targetReg /= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case MOD:
			targetReg := cpu.getRegister(reg)
			if resolvedOperand == 0 {
				fmt.Printf("Division by zero error at PC=%08x\n", cpu.PC)
				cpu.Running = false
				break
			}
			*targetReg %= resolvedOperand
			cpu.PC += INSTRUCTION_SIZE

		case WAIT:
			targetTime := resolvedOperand
			for i := uint32(0); i < targetTime; i++ {
			}
			cpu.PC += INSTRUCTION_SIZE

		case JSR:
			retAddr := cpu.PC + INSTRUCTION_SIZE
			if !cpu.Push(retAddr) {
				return
			}
			cpu.PC = operand

		case RTS:
			retAddr, ok := cpu.Pop()
			if !ok {
				return
			}
			cpu.PC = retAddr

		case SEI:
			cpu.InterruptEnabled = true
			cpu.PC += INSTRUCTION_SIZE

		case CLI:
			cpu.InterruptEnabled = false
			cpu.PC += INSTRUCTION_SIZE

		case RTI:
			returnPC, ok := cpu.Pop()
			if !ok {
				return
			}
			cpu.PC = returnPC
			cpu.InInterrupt = false

		case INC:
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
			cpu.PC += INSTRUCTION_SIZE

		case HALT:
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
