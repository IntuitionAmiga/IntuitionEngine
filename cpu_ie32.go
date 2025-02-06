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
	// Opcodes
	LOAD  = 0x01
	LDA   = 0x20
	LDX   = 0x21
	LDY   = 0x22
	LDZ   = 0x23
	STORE = 0x02
	STA   = 0x24
	STX   = 0x25
	STY   = 0x26
	STZ   = 0x27
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
	INC   = 0x28
	DEC   = 0x29

	// New load opcodes
	LDB = 0x3A
	LDC = 0x3B
	LDD = 0x3C
	LDE = 0x3D
	LDF = 0x3E
	LDG = 0x3F
	LDH = 0x4C
	LDS = 0x4D
	LDT = 0x4E
	LDU = 0x40
	LDV = 0x41
	LDW = 0x42

	// New store opcodes
	STB = 0x43
	STC = 0x44
	STD = 0x45
	STE = 0x46
	STF = 0x47
	STG = 0x48
	STH = 0x4F
	STS = 0x50
	STT = 0x51
	STU = 0x49
	STV = 0x4A
	STW = 0x4B

	NOP  = 0xEE
	HALT = 0xFF
)
const (
	// Addressing mode flags
	ADDR_IMMEDIATE = 0x00
	ADDR_REGISTER  = 0x01
	ADDR_REG_IND   = 0x02
	ADDR_MEM_IND   = 0x03
)

const (
	// Memory Map
	PROG_START   = 0x1000
	STACK_START  = 0xE000
	STACK_BOTTOM = 0x2000

	TIMER_COUNT  uint32 = 0xF804
	TIMER_PERIOD uint32 = 0xF808

	SAVE_INT_VEC uint32 = 0x0000
)
const (
	TIMER_STOPPED = iota
	TIMER_RUNNING
	TIMER_EXPIRED
)

type CPU struct {
	A uint32
	X uint32
	Y uint32
	Z uint32
	B uint32
	C uint32
	D uint32
	E uint32
	F uint32
	G uint32
	H uint32
	S uint32
	T uint32
	U uint32
	V uint32
	W uint32

	PC uint32
	SP uint32

	Memory  []byte
	Running bool
	Screen  [25][80]byte
	Debug   bool

	InterruptVector  uint32
	InterruptEnabled bool
	InInterrupt      bool

	timerEnabled bool
	timerCount   uint32
	timerPeriod  uint32

	cycleCounter uint32 // Counts cycles between timer decrements

	mutex      sync.RWMutex
	timerMutex sync.Mutex

	timerState uint8 // States like TIMER_STOPPED, TIMER_RUNNING, TIMER_EXPIRED

	bus MemoryBus
}

func NewCPU(bus MemoryBus) *CPU {
	return &CPU{
		Memory:           make([]byte, 16*1024*1024),
		Running:          true,
		Screen:           [25][80]byte{},
		Debug:            false,
		SP:               STACK_START,
		PC:               PROG_START,
		A:                0,
		X:                0,
		Y:                0,
		Z:                0,
		InterruptEnabled: false,
		InInterrupt:      false,
		timerEnabled:     false,
		bus:              bus, // Use passed-in bus
	}
}

func (cpu *CPU) LoadProgram(filename string) error {
	program, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	// Clear only program area, preserve vectors
	for i := PROG_START; i < STACK_START; i++ {
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
	blackSquare := "\u25A0"

	fmt.Println("\n┌" + strings.Repeat("─", 80) + "┐")
	for _, row := range cpu.Screen {
		fmt.Print("│")
		for _, pixel := range row {
			if pixel == 0 {
				fmt.Print(" ")
			} else {
				fmt.Print(blackSquare)
			}
		}
		fmt.Println("│")
	}
	fmt.Println("└" + strings.Repeat("─", 80) + "┘")
}

func (cpu *CPU) getRegister(reg byte) *uint32 {
	switch reg & 0x0F {
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
		fmt.Printf("%s cpu.Push\tStack overflow error at PC=%08x (SP=%08x)\n", time.Now().Format("15:04:05.000"), cpu.PC, cpu.SP)
		cpu.Running = false
		return false
	}
	cpu.SP -= 4
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
	cpu.SP += 4
	return value, true
}

func (cpu *CPU) DumpStack() {
	if cpu.SP >= STACK_START {
		fmt.Println("Stack is empty")
		return
	}
	fmt.Println("\nStack contents:")
	for addr := cpu.SP; addr < STACK_START; addr += 4 {
		value := cpu.Read32(addr)
		fmt.Printf("  %08x: %08x\n", addr, value)
	}
	fmt.Println()
}

func (cpu *CPU) resolveOperand(addrMode byte, operand uint32) uint32 {
	// Special handling for memory-mapped I/O addresses
	if operand >= 0xF800 && operand <= 0xFFFF {
		// Always use memory-indirect access for hardware registers
		return cpu.Read32(operand)
	}

	switch addrMode {
	case ADDR_IMMEDIATE:
		return operand
	case ADDR_REGISTER:
		return *cpu.getRegister(byte(operand & 0x03))
	case ADDR_REG_IND:
		reg := byte(operand & 0x03)    // Bottom 2 bits select register
		offset := operand & 0xFFFFFFFC // Upper 30 bits are offset
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
	cpu.PC = cpu.Read32(SAVE_INT_VEC)
}

func (cpu *CPU) Reset() {
	cpu.mutex.Lock()
	cpu.Running = false

	// Stop video monitoring immediately
	if activeFrontend != nil && activeFrontend.video != nil {
		video := activeFrontend.video
		video.mutex.Lock()
		video.enabled = false // This will make the monitor skip processing
		video.hasContent = false
		video.mutex.Unlock()
	}
	cpu.mutex.Unlock()

	time.Sleep(time.Millisecond * 50)

	cpu.mutex.Lock()
	// Clear memory
	for i := PROG_START; i < len(cpu.Memory); i++ {
		cpu.Memory[i] = 0
	}

	// Re-enable video after memory is cleared
	if activeFrontend != nil && activeFrontend.video != nil {
		video := activeFrontend.video
		video.mutex.Lock()
		// Clear the prevVRAM to match cleared memory
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
		currentPC := cpu.PC
		opcode := cpu.Memory[currentPC]
		reg := cpu.Memory[currentPC+1]
		addrMode := cpu.Memory[currentPC+2]
		operand := binary.LittleEndian.Uint32(cpu.Memory[currentPC+4 : currentPC+8])
		resolvedOperand := cpu.resolveOperand(addrMode, operand)

		if cpu.timerEnabled {
			cpu.cycleCounter++
			if cpu.cycleCounter >= SAMPLE_RATE { // Reset every second
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
					cpu.Memory[TIMER_COUNT:TIMER_COUNT+4],
					cpu.timerCount)
			}
		}

		switch opcode {
		case LOAD:
			*cpu.getRegister(reg) = resolvedOperand
			cpu.PC += 8

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
			cpu.PC += 8

		case STORE:
			if addrMode == ADDR_REG_IND {
				addr := *cpu.getRegister(byte(operand & 0x03)) + (operand & 0xFFFFFFFC)
				cpu.Write32(addr, *cpu.getRegister(reg))
			} else if addrMode == ADDR_MEM_IND {
				addr := cpu.Read32(operand)
				cpu.Write32(addr, *cpu.getRegister(reg))
			} else {
				cpu.Write32(operand, *cpu.getRegister(reg))
			}
			cpu.PC += 8

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
				addr := *cpu.getRegister(byte(operand & 0x03)) + (operand & 0xFFFFFFFC)
				cpu.Write32(addr, value)
			} else if addrMode == ADDR_MEM_IND {
				addr := cpu.Read32(operand)
				cpu.Write32(addr, value)
			} else {
				cpu.Write32(operand, value)
			}
			cpu.PC += 8

		case ADD:
			// Get the target register from the instruction
			targetReg := cpu.getRegister(reg)
			*targetReg += resolvedOperand
			cpu.PC += 8

		case SUB:
			// Simplified subtraction that works with any register
			targetReg := cpu.getRegister(reg)
			if resolvedOperand > *targetReg {
				*targetReg = 0
			} else {
				*targetReg -= resolvedOperand
			}
			cpu.PC += 8

		case AND:
			targetReg := cpu.getRegister(reg)
			*targetReg &= resolvedOperand
			cpu.PC += 8

		case OR:
			targetReg := cpu.getRegister(reg)
			*targetReg |= resolvedOperand
			cpu.PC += 8

		case XOR:
			targetReg := cpu.getRegister(reg)
			*targetReg ^= resolvedOperand
			cpu.PC += 8

		case SHL:
			targetReg := cpu.getRegister(reg)
			*targetReg <<= resolvedOperand
			cpu.PC += 8

		case SHR:
			targetReg := cpu.getRegister(reg)
			*targetReg >>= resolvedOperand
			cpu.PC += 8

		case NOT:
			targetReg := cpu.getRegister(reg)
			*targetReg = ^(*targetReg)
			cpu.PC += 8

		case JMP:
			cpu.PC = operand

		case JNZ:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+4 : currentPC+8])
			if *cpu.getRegister(reg) != 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += 8
			}

		case JZ:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+4 : currentPC+8])
			if *cpu.getRegister(reg) == 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += 8
			}

		case JGT:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+4 : currentPC+8])
			if int32(*cpu.getRegister(reg)) > 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += 8
			}

		case JGE:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+4 : currentPC+8])
			if int32(*cpu.getRegister(reg)) >= 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += 8
			}

		case JLT:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+4 : currentPC+8])
			if int32(*cpu.getRegister(reg)) < 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += 8
			}

		case JLE:
			reg := cpu.Memory[currentPC+1]
			targetAddr := binary.LittleEndian.Uint32(cpu.Memory[currentPC+4 : currentPC+8])
			if int32(*cpu.getRegister(reg)) <= 0 {
				cpu.PC = targetAddr
			} else {
				cpu.PC += 8
			}

		case PUSH:
			value := *cpu.getRegister(reg)
			if !cpu.Push(value) {
				return
			}
			cpu.PC += 8

		case POP:
			value, ok := cpu.Pop()
			if !ok {
				return
			}
			*cpu.getRegister(reg) = value
			cpu.PC += 8

		case MUL:
			targetReg := cpu.getRegister(reg)
			*targetReg *= resolvedOperand
			cpu.PC += 8

		case DIV:
			targetReg := cpu.getRegister(reg)
			if resolvedOperand == 0 {
				fmt.Printf("Division by zero error at PC=%08x\n", cpu.PC)
				cpu.Running = false
				break
			}
			*targetReg /= resolvedOperand
			cpu.PC += 8

		case MOD:
			targetReg := cpu.getRegister(reg)
			if resolvedOperand == 0 {
				fmt.Printf("Division by zero error at PC=%08x\n", cpu.PC)
				cpu.Running = false
				break
			}
			*targetReg %= resolvedOperand
			cpu.PC += 8

		case WAIT:
			targetTime := resolvedOperand
			for i := uint32(0); i < targetTime; i++ {
			}
			cpu.PC += 8

		case JSR:
			retAddr := cpu.PC + 8
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
			cpu.PC += 8

		case CLI:
			cpu.InterruptEnabled = false
			cpu.PC += 8

		case RTI:
			returnPC, ok := cpu.Pop()
			if !ok {
				return
			}
			cpu.PC = returnPC
			cpu.InInterrupt = false

		case INC:
			if addrMode == ADDR_REGISTER {
				reg := cpu.getRegister(byte(operand & 0x03))
				(*reg)++
			} else if addrMode == ADDR_REG_IND {
				addr := *cpu.getRegister(byte(operand & 0x03)) + (operand & 0xFFFFFFFC)
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
			cpu.PC += 8

		case DEC:
			if addrMode == ADDR_REGISTER {
				reg := cpu.getRegister(byte(operand & 0x03))
				(*reg)--
			} else if addrMode == ADDR_REG_IND {
				addr := *cpu.getRegister(byte(operand & 0x03)) + (operand & 0xFFFFFFFC)
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
			cpu.PC += 8

		case NOP:
			cpu.PC += 8

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
