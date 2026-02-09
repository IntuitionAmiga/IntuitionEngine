// cpu_ie64.go - Intuition Engine 64-bit RISC CPU

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
cpu_ie64.go - 64-bit RISC CPU for the Intuition Engine

This module implements a 64-bit RISC load-store CPU with a clean, regular
instruction encoding. The architecture uses compare-and-branch instead of
flags, 32 general-purpose 64-bit registers (R0 hardwired to zero), and
fixed 8-byte instructions.

Core Features:
- 32 general-purpose 64-bit registers (R0 hardwired zero)
- R31 aliases as stack pointer (SP)
- Fixed 8-byte instruction encoding
- Compare-and-branch (no flags register)
- Load-store architecture
- Size-annotated operations (.B, .W, .L, .Q)
- Hardware timer with interrupt support
- PC masked to 32MB address space

Instruction Encoding (8 bytes, little-endian):
  Byte 0: Opcode      (8 bits)
  Byte 1: Rd[4:0]     (5 bits) | Size[1:0] (2 bits) | X (1 bit)
  Byte 2: Rs[4:0]     (5 bits) | unused    (3 bits)
  Byte 3: Rt[4:0]     (5 bits) | unused    (3 bits)
  Bytes 4-7: imm32    (32-bit LE)

Thread Safety:
- running, debug use atomic.Bool for lock-free cross-thread signalling
- Timer fields use atomic types for lock-free access
- Interrupt fields use atomic types
- Execute() loop uses local running flag with periodic atomic check
*/

package main

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"
	"unsafe"
)

// ------------------------------------------------------------------------------
// IE64 Instruction Size
// ------------------------------------------------------------------------------
const (
	IE64_INSTR_SIZE = 8 // Fixed 8-byte instructions
)

// ------------------------------------------------------------------------------
// IE64 Size Codes
// ------------------------------------------------------------------------------
const (
	IE64_SIZE_B = 0 // Byte (8-bit)
	IE64_SIZE_W = 1 // Word (16-bit)
	IE64_SIZE_L = 2 // Long (32-bit)
	IE64_SIZE_Q = 3 // Quad (64-bit)
)

// ------------------------------------------------------------------------------
// IE64 Address Space
// ------------------------------------------------------------------------------
const (
	IE64_ADDR_MASK = 0x1FFFFFF // 32MB address space mask
)

// ------------------------------------------------------------------------------
// IE64 Opcodes
// ------------------------------------------------------------------------------
const (
	// Data Movement
	OP_MOVE  = 0x01 // Move register/immediate to register
	OP_MOVT  = 0x02 // Move to upper 32 bits
	OP_MOVEQ = 0x03 // Move with sign-extend 32 to 64
	OP_LEA   = 0x04 // Load effective address

	// Memory Access
	OP_LOAD  = 0x10 // Load from memory
	OP_STORE = 0x11 // Store to memory

	// Arithmetic
	OP_ADD   = 0x20 // Add
	OP_SUB   = 0x21 // Subtract
	OP_MULU  = 0x22 // Multiply unsigned
	OP_MULS  = 0x23 // Multiply signed
	OP_DIVU  = 0x24 // Divide unsigned
	OP_DIVS  = 0x25 // Divide signed
	OP_MOD64 = 0x26 // Modulo
	OP_NEG   = 0x27 // Negate

	// Logic
	OP_AND64 = 0x30 // Bitwise AND
	OP_OR64  = 0x31 // Bitwise OR
	OP_EOR   = 0x32 // Bitwise XOR
	OP_NOT64 = 0x33 // Bitwise NOT
	OP_LSL   = 0x34 // Logical shift left
	OP_LSR   = 0x35 // Logical shift right
	OP_ASR   = 0x36 // Arithmetic shift right

	// Branches (compare-and-branch, PC-relative)
	OP_BRA = 0x40 // Branch always
	OP_BEQ = 0x41 // Branch if equal (Rs == Rt)
	OP_BNE = 0x42 // Branch if not equal (Rs != Rt)
	OP_BLT = 0x43 // Branch if less than (signed)
	OP_BGE = 0x44 // Branch if greater or equal (signed)
	OP_BGT = 0x45 // Branch if greater than (signed)
	OP_BLE = 0x46 // Branch if less or equal (signed)
	OP_BHI = 0x47 // Branch if higher (unsigned)
	OP_BLS = 0x48 // Branch if lower or same (unsigned)
	OP_JMP = 0x49 // Jump register-indirect

	// Subroutine / Stack
	OP_JSR64   = 0x50 // Jump to subroutine (PC-relative)
	OP_RTS64   = 0x51 // Return from subroutine
	OP_PUSH64  = 0x52 // Push register to stack
	OP_POP64   = 0x53 // Pop from stack to register
	OP_JSR_IND = 0x54 // JSR register-indirect

	// System
	OP_NOP64  = 0xE0 // No operation
	OP_HALT64 = 0xE1 // Halt processor
	OP_SEI64  = 0xE2 // Set interrupt enable
	OP_CLI64  = 0xE3 // Clear interrupt enable
	OP_RTI64  = 0xE4 // Return from interrupt
	OP_WAIT64 = 0xE5 // Wait (microseconds in imm32)
)

// ------------------------------------------------------------------------------
// CPU64 — 64-bit RISC CPU
// ------------------------------------------------------------------------------

type CPU64 struct {
	// Cache Lines 0-3 (256 bytes): Register file
	regs [32]uint64

	// Cache Line 4: Execution state
	PC           uint64
	running      atomic.Bool
	debug        atomic.Bool
	cycleCounter uint64
	_pad4        [40]byte // pad to 64

	// Cache Line 5: Timer
	timerCount   atomic.Uint64
	timerPeriod  atomic.Uint64
	timerState   atomic.Uint32
	timerEnabled atomic.Bool
	_pad5        [35]byte

	// Cache Line 6: Interrupt
	interruptVector  uint64
	interruptEnabled atomic.Bool
	inInterrupt      atomic.Bool
	_pad6            [46]byte

	// Beyond cache lines:
	memory  []byte
	bus     *MachineBus
	memBase unsafe.Pointer

	// VRAM direct access
	vramDirect []byte
	vramStart  uint32
	vramEnd    uint32

	// Performance measurement
	PerfEnabled      bool
	InstructionCount uint64
	perfStartTime    time.Time
	lastPerfReport   time.Time
}

// ------------------------------------------------------------------------------
// Constructor
// ------------------------------------------------------------------------------

func NewCPU64(bus *MachineBus) *CPU64 {
	cpu := &CPU64{
		PC:     PROG_START,
		bus:    bus,
		memory: bus.GetMemory(),
	}
	cpu.memBase = unsafe.Pointer(&cpu.memory[0])
	cpu.regs[31] = STACK_START // R31 is the stack pointer
	cpu.running.Store(true)
	return cpu
}

// ------------------------------------------------------------------------------
// Register Access
// ------------------------------------------------------------------------------

func (cpu *CPU64) setReg(idx byte, val uint64) {
	if idx == 0 {
		return // R0 is hardwired to zero
	}
	cpu.regs[idx] = val
}

func (cpu *CPU64) getReg(idx byte) uint64 {
	return cpu.regs[idx] // R0 always reads 0 since it is never written
}

// ------------------------------------------------------------------------------
// Size Masking
// ------------------------------------------------------------------------------

var ie64SizeMask = [4]uint64{0xFF, 0xFFFF, 0xFFFFFFFF, 0xFFFFFFFFFFFFFFFF}

func maskToSize(val uint64, size byte) uint64 {
	return val & ie64SizeMask[size]
}

// ------------------------------------------------------------------------------
// Memory Access
// ------------------------------------------------------------------------------

func (cpu *CPU64) loadMem(addr uint32, size byte) uint64 {
	// VRAM direct read fast path (VRAM addresses are above IO_REGION_START)
	if cpu.vramDirect != nil && addr >= cpu.vramStart && addr < cpu.vramEnd {
		offset := addr - cpu.vramStart
		base := unsafe.Pointer(&cpu.vramDirect[offset])
		switch size {
		case IE64_SIZE_Q:
			return *(*uint64)(base)
		case IE64_SIZE_L:
			return uint64(*(*uint32)(base))
		case IE64_SIZE_W:
			return uint64(*(*uint16)(base))
		case IE64_SIZE_B:
			return uint64(*(*byte)(base))
		}
	}

	// Non-I/O fast path — direct memory read, no bus overhead
	if addr < IO_REGION_START {
		base := unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr))
		switch size {
		case IE64_SIZE_Q:
			return *(*uint64)(base)
		case IE64_SIZE_L:
			return uint64(*(*uint32)(base))
		case IE64_SIZE_W:
			return uint64(*(*uint16)(base))
		case IE64_SIZE_B:
			return uint64(*(*byte)(base))
		}
	}

	// I/O slow path — bus callbacks protect their own state
	switch size {
	case IE64_SIZE_B:
		return uint64(cpu.bus.Read8(addr))
	case IE64_SIZE_W:
		return uint64(cpu.bus.Read16(addr))
	case IE64_SIZE_L:
		return uint64(cpu.bus.Read32(addr))
	case IE64_SIZE_Q:
		return cpu.bus.Read64(addr)
	}
	return 0
}

func (cpu *CPU64) storeMem(addr uint32, val uint64, size byte) {
	// VRAM direct write fast path (VRAM addresses are above IO_REGION_START)
	if cpu.vramDirect != nil && addr >= cpu.vramStart && addr < cpu.vramEnd {
		offset := addr - cpu.vramStart
		base := unsafe.Pointer(&cpu.vramDirect[offset])
		switch size {
		case IE64_SIZE_B:
			*(*byte)(base) = byte(val)
		case IE64_SIZE_W:
			*(*uint16)(base) = uint16(val)
		case IE64_SIZE_L:
			*(*uint32)(base) = uint32(val)
		case IE64_SIZE_Q:
			*(*uint64)(base) = val
		}
		return
	}

	// Non-I/O fast path — direct memory write, no bus overhead
	if addr < IO_REGION_START {
		base := unsafe.Pointer(uintptr(cpu.memBase) + uintptr(addr))
		switch size {
		case IE64_SIZE_B:
			*(*byte)(base) = byte(val)
		case IE64_SIZE_W:
			*(*uint16)(base) = uint16(val)
		case IE64_SIZE_L:
			*(*uint32)(base) = uint32(val)
		case IE64_SIZE_Q:
			*(*uint64)(base) = val
		}
		return
	}

	// I/O slow path — bus callbacks protect their own state
	switch size {
	case IE64_SIZE_B:
		cpu.bus.Write8(addr, uint8(val))
	case IE64_SIZE_W:
		cpu.bus.Write16(addr, uint16(val))
	case IE64_SIZE_L:
		cpu.bus.Write32(addr, uint32(val))
	case IE64_SIZE_Q:
		cpu.bus.Write64(addr, val)
	}
}

// ------------------------------------------------------------------------------
// Program Loading
// ------------------------------------------------------------------------------

func (cpu *CPU64) LoadProgram(filename string) error {
	program, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	cpu.LoadProgramBytes(program)
	return nil
}

// LoadProgramBytes loads raw machine code bytes at PROG_START and resets PC.
func (cpu *CPU64) LoadProgramBytes(program []byte) {
	// Clear program area
	for i := PROG_START; i < len(cpu.memory) && i < STACK_START; i++ {
		cpu.memory[i] = 0
	}
	copy(cpu.memory[PROG_START:], program)
	cpu.PC = PROG_START
}

// ------------------------------------------------------------------------------
// Reset
// ------------------------------------------------------------------------------

func (cpu *CPU64) Reset() {
	cpu.running.Store(false)
	time.Sleep(RESET_DELAY)

	// Clear registers
	for i := range cpu.regs {
		cpu.regs[i] = 0
	}
	cpu.PC = PROG_START
	cpu.regs[31] = STACK_START
	cpu.cycleCounter = 0
	cpu.interruptVector = 0
	cpu.interruptEnabled.Store(false)
	cpu.inInterrupt.Store(false)
	cpu.timerCount.Store(0)
	cpu.timerPeriod.Store(0)
	cpu.timerState.Store(TIMER_STOPPED)
	cpu.timerEnabled.Store(false)
	cpu.InstructionCount = 0

	// Clear memory in chunks for better cache utilization
	for i := PROG_START; i < len(cpu.memory); i += CACHE_LINE_SIZE {
		end := i + CACHE_LINE_SIZE
		if end > len(cpu.memory) {
			end = len(cpu.memory)
		}
		for j := i; j < end; j++ {
			cpu.memory[j] = 0
		}
	}

	cpu.running.Store(true)
}

// ------------------------------------------------------------------------------
// VRAM Direct Access
// ------------------------------------------------------------------------------

func (cpu *CPU64) AttachDirectVRAM(buffer []byte, start, end uint32) {
	cpu.vramDirect = buffer
	cpu.vramStart = start
	cpu.vramEnd = end
}

func (cpu *CPU64) DetachDirectVRAM() {
	cpu.vramDirect = nil
}

// ------------------------------------------------------------------------------
// Execute — Main Instruction Loop
// ------------------------------------------------------------------------------

func (cpu *CPU64) Execute() {
	if cpu.PC < PROG_START || cpu.PC >= STACK_START {
		fmt.Printf("IE64: Invalid initial PC value: 0x%08x\n", cpu.PC)
		cpu.running.Store(false)
		return
	}

	// Initialize performance measurement
	cpu.perfStartTime = time.Now()
	cpu.lastPerfReport = cpu.perfStartTime
	cpu.InstructionCount = 0

	// Cache locals that never change during execution
	perfEnabled := cpu.PerfEnabled
	memBase := unsafe.Pointer(&cpu.memory[0])
	memSize := uint64(len(cpu.memory))

	// Use local running flag to avoid atomic load every iteration
	// Check external stop signal every 4096 instructions
	running := true
	checkCounter := uint32(0)

	for running {
		// Periodic check of external stop signal (every 4096 instructions)
		checkCounter++
		if checkCounter&0xFFF == 0 && !cpu.running.Load() {
			break
		}

		// Performance measurement: count instructions and report periodically
		if perfEnabled {
			cpu.InstructionCount++
			if cpu.InstructionCount&0xFFFFFF == 0 { // Every ~16M instructions
				now := time.Now()
				if now.Sub(cpu.lastPerfReport) >= time.Second {
					elapsed := now.Sub(cpu.perfStartTime).Seconds()
					mips := float64(cpu.InstructionCount) / elapsed / 1_000_000
					fmt.Printf("IE64: %.2f MIPS (%.0f instructions in %.1fs)\n", mips, float64(cpu.InstructionCount), elapsed)
					cpu.lastPerfReport = now
				}
			}
		}

		// Mask PC to 32MB address space
		pc32 := uint32(cpu.PC & IE64_ADDR_MASK)
		if uint64(pc32)+8 > memSize {
			fmt.Printf("IE64: PC out of bounds during fetch: PC=0x%08X mem=%d\n", pc32, memSize)
			cpu.running.Store(false)
			break
		}

		// Fetch entire 8-byte instruction in one read (LE platform enforced by le_check.go)
		instr := *(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(pc32)))
		opcode := byte(instr)
		byte1 := byte(instr >> 8)
		byte2 := byte(instr >> 16)
		byte3 := byte(instr >> 24)
		imm32 := uint32(instr >> 32)

		// Decode fields
		rd := byte1 >> 3            // 5 bits
		size := (byte1 >> 1) & 0x03 // 2 bits
		xbit := byte1 & 1           // 1 bit
		rs := byte2 >> 3            // 5 bits
		rt := byte3 >> 3            // 5 bits

		// Resolve third operand: immediate or register Rt
		var operand3 uint64
		if xbit == 1 {
			operand3 = uint64(imm32) // zero-extended
		} else {
			operand3 = cpu.regs[rt]
		}

		// Timer handling with lock-free atomics
		if cpu.timerEnabled.Load() {
			cpu.cycleCounter++
			if cpu.cycleCounter >= SAMPLE_RATE {
				cpu.cycleCounter = 0

				count := cpu.timerCount.Load()
				if count > 0 {
					newCount := count - 1
					cpu.timerCount.Store(newCount)
					if newCount == 0 {
						cpu.timerState.Store(TIMER_EXPIRED)
						// Inline handleInterrupt — uses memBase/memSize locals
						if cpu.interruptEnabled.Load() && !cpu.inInterrupt.Load() {
							cpu.inInterrupt.Store(true)
							cpu.regs[31] -= 8
							sp := uint32(cpu.regs[31])
							if uint64(sp)+8 > memSize {
								cpu.running.Store(false)
								running = false
								continue
							}
							*(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(sp))) = cpu.PC
							cpu.PC = cpu.interruptVector
						}
						// Reload timer if still enabled (handler may have disabled it)
						if cpu.timerEnabled.Load() {
							cpu.timerCount.Store(cpu.timerPeriod.Load())
						}
					}
				} else {
					period := cpu.timerPeriod.Load()
					if period > 0 {
						cpu.timerCount.Store(period)
						cpu.timerState.Store(TIMER_RUNNING)
					}
				}
			}
		}

		switch opcode {
		case OP_MOVE:
			if xbit == 1 {
				if rd != 0 {
					cpu.regs[rd] = maskToSize(uint64(imm32), size)
				}
			} else {
				if rd != 0 {
					cpu.regs[rd] = maskToSize(cpu.regs[rs], size)
				}
			}

		case OP_MOVT:
			if rd != 0 {
				cpu.regs[rd] = (cpu.regs[rd] & 0x00000000FFFFFFFF) | (uint64(imm32) << 32)
			}

		case OP_MOVEQ:
			if rd != 0 {
				cpu.regs[rd] = uint64(int64(int32(imm32)))
			}

		case OP_LEA:
			if rd != 0 {
				cpu.regs[rd] = uint64(int64(cpu.regs[rs]) + int64(int32(imm32)))
			}

		case OP_LOAD:
			if rd != 0 {
				addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
				cpu.regs[rd] = cpu.loadMem(addr, size)
			}

		case OP_STORE:
			addr := uint32(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.storeMem(addr, maskToSize(cpu.regs[rd], size), size)

		case OP_ADD:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]+operand3, size)
			}

		case OP_SUB:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]-operand3, size)
			}

		case OP_MULU:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]*operand3, size)
			}

		case OP_MULS:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(uint64(int64(cpu.regs[rs])*int64(operand3)), size)
			}

		case OP_DIVU:
			if rd != 0 {
				if operand3 == 0 {
					cpu.regs[rd] = 0
				} else {
					cpu.regs[rd] = maskToSize(cpu.regs[rs]/operand3, size)
				}
			}

		case OP_DIVS:
			if rd != 0 {
				if operand3 == 0 {
					cpu.regs[rd] = 0
				} else {
					cpu.regs[rd] = maskToSize(uint64(int64(cpu.regs[rs])/int64(operand3)), size)
				}
			}

		case OP_MOD64:
			if rd != 0 {
				if operand3 == 0 {
					cpu.regs[rd] = 0
				} else {
					cpu.regs[rd] = maskToSize(cpu.regs[rs]%operand3, size)
				}
			}

		case OP_NEG:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(uint64(-int64(cpu.regs[rs])), size)
			}

		case OP_AND64:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]&operand3, size)
			}

		case OP_OR64:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]|operand3, size)
			}

		case OP_EOR:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]^operand3, size)
			}

		case OP_NOT64:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(^cpu.regs[rs], size)
			}

		case OP_LSL:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]<<(operand3&63), size)
			}

		case OP_LSR:
			if rd != 0 {
				cpu.regs[rd] = maskToSize(cpu.regs[rs]>>(operand3&63), size)
			}

		case OP_ASR:
			if rd != 0 {
				shift := operand3 & 63
				var sval int64
				switch size {
				case IE64_SIZE_B:
					sval = int64(int8(cpu.regs[rs]))
				case IE64_SIZE_W:
					sval = int64(int16(cpu.regs[rs]))
				case IE64_SIZE_L:
					sval = int64(int32(cpu.regs[rs]))
				case IE64_SIZE_Q:
					sval = int64(cpu.regs[rs])
				}
				cpu.regs[rd] = maskToSize(uint64(sval>>shift), size)
			}

		case OP_BRA:
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			continue

		case OP_BEQ:
			if cpu.regs[rs] == cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BNE:
			if cpu.regs[rs] != cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BLT:
			if int64(cpu.regs[rs]) < int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BGE:
			if int64(cpu.regs[rs]) >= int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BGT:
			if int64(cpu.regs[rs]) > int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BLE:
			if int64(cpu.regs[rs]) <= int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BHI:
			if cpu.regs[rs] > cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_BLS:
			if cpu.regs[rs] <= cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}
			continue

		case OP_JMP:
			target := uint64(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.PC = target & IE64_ADDR_MASK
			continue

		case OP_JSR64:
			cpu.regs[31] -= 8
			sp := uint32(cpu.regs[31])
			if uint64(sp)+8 > memSize {
				cpu.running.Store(false)
				running = false
				continue
			}
			*(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(sp))) = cpu.PC + IE64_INSTR_SIZE
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			continue

		case OP_RTS64:
			sp := uint32(cpu.regs[31])
			if uint64(sp)+8 > memSize {
				cpu.running.Store(false)
				running = false
				continue
			}
			cpu.PC = *(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(sp)))
			cpu.regs[31] += 8
			continue

		case OP_PUSH64:
			cpu.regs[31] -= 8
			sp := uint32(cpu.regs[31])
			if uint64(sp)+8 > memSize {
				cpu.running.Store(false)
				running = false
				continue
			}
			*(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(sp))) = cpu.regs[rs]

		case OP_POP64:
			sp := uint32(cpu.regs[31])
			if uint64(sp)+8 > memSize {
				cpu.running.Store(false)
				running = false
				continue
			}
			val := *(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(sp)))
			if rd != 0 {
				cpu.regs[rd] = val
			}
			cpu.regs[31] += 8

		case OP_JSR_IND:
			cpu.regs[31] -= 8
			sp := uint32(cpu.regs[31])
			if uint64(sp)+8 > memSize {
				cpu.running.Store(false)
				running = false
				continue
			}
			*(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(sp))) = cpu.PC + IE64_INSTR_SIZE
			target := uint64(int64(cpu.regs[rs]) + int64(int32(imm32)))
			cpu.PC = target & IE64_ADDR_MASK
			continue

		case OP_NOP64:
			// default PC advance

		case OP_HALT64:
			cpu.running.Store(false)
			running = false
			continue

		case OP_SEI64:
			cpu.interruptEnabled.Store(true)

		case OP_CLI64:
			cpu.interruptEnabled.Store(false)

		case OP_RTI64:
			sp := uint32(cpu.regs[31])
			if uint64(sp)+8 > memSize {
				cpu.running.Store(false)
				running = false
				continue
			}
			cpu.PC = *(*uint64)(unsafe.Pointer(uintptr(memBase) + uintptr(sp)))
			cpu.regs[31] += 8
			cpu.inInterrupt.Store(false)
			continue

		case OP_WAIT64:
			if imm32 > 0 {
				time.Sleep(time.Duration(imm32) * time.Microsecond)
			}

		default:
			fmt.Printf("IE64: Invalid opcode 0x%02X at PC=0x%X\n", opcode, cpu.PC)
			cpu.running.Store(false)
			running = false
			continue
		}

		// Default PC advance — opcodes that set PC themselves use `continue` above
		cpu.PC += IE64_INSTR_SIZE
	}
}
