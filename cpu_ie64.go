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
	"encoding/binary"
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

	// Subroutine / Stack
	OP_JSR64  = 0x50 // Jump to subroutine (PC-relative)
	OP_RTS64  = 0x51 // Return from subroutine
	OP_PUSH64 = 0x52 // Push register to stack
	OP_POP64  = 0x53 // Pop from stack to register

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
	bus     *SystemBus
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

func NewCPU64(bus *SystemBus) *CPU64 {
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

func maskToSize(val uint64, size byte) uint64 {
	switch size {
	case IE64_SIZE_B:
		return val & 0xFF
	case IE64_SIZE_W:
		return val & 0xFFFF
	case IE64_SIZE_L:
		return val & 0xFFFFFFFF
	case IE64_SIZE_Q:
		return val
	}
	return val
}

// ------------------------------------------------------------------------------
// Memory Access
// ------------------------------------------------------------------------------

func (cpu *CPU64) loadMem(addr uint32, size byte) uint64 {
	switch size {
	case IE64_SIZE_B:
		return uint64(cpu.bus.Read8(addr))
	case IE64_SIZE_W:
		return uint64(cpu.bus.Read16(addr))
	case IE64_SIZE_L:
		return uint64(cpu.bus.Read32(addr))
	case IE64_SIZE_Q:
		// Direct memory access for 64-bit loads.
		// TODO: Replace with bus.Read64() when MemoryBus64 interface is available.
		return binary.LittleEndian.Uint64(cpu.memory[addr:])
	}
	return 0
}

func (cpu *CPU64) storeMem(addr uint32, val uint64, size byte) {
	// VRAM direct write fast path
	if cpu.vramDirect != nil && addr >= cpu.vramStart && addr < cpu.vramEnd {
		offset := addr - cpu.vramStart
		switch size {
		case IE64_SIZE_B:
			if offset < uint32(len(cpu.vramDirect)) {
				cpu.vramDirect[offset] = byte(val)
				return
			}
		case IE64_SIZE_W:
			if offset+2 <= uint32(len(cpu.vramDirect)) {
				binary.LittleEndian.PutUint16(cpu.vramDirect[offset:], uint16(val))
				return
			}
		case IE64_SIZE_L:
			if offset+4 <= uint32(len(cpu.vramDirect)) {
				binary.LittleEndian.PutUint32(cpu.vramDirect[offset:], uint32(val))
				return
			}
		case IE64_SIZE_Q:
			if offset+8 <= uint32(len(cpu.vramDirect)) {
				binary.LittleEndian.PutUint64(cpu.vramDirect[offset:], val)
				return
			}
		}
	}

	switch size {
	case IE64_SIZE_B:
		cpu.bus.Write8(addr, uint8(val))
	case IE64_SIZE_W:
		cpu.bus.Write16(addr, uint16(val))
	case IE64_SIZE_L:
		cpu.bus.Write32(addr, uint32(val))
	case IE64_SIZE_Q:
		// Direct memory access for 64-bit stores.
		// TODO: Replace with bus.Write64() when MemoryBus64 interface is available.
		binary.LittleEndian.PutUint64(cpu.memory[addr:], val)
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
	// Clear program area
	for i := PROG_START; i < len(cpu.memory) && i < STACK_START; i++ {
		cpu.memory[i] = 0
	}
	copy(cpu.memory[PROG_START:], program)
	cpu.PC = PROG_START
	return nil
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
// Interrupt Handling
// ------------------------------------------------------------------------------

func (cpu *CPU64) handleInterrupt() {
	if !cpu.interruptEnabled.Load() || cpu.inInterrupt.Load() {
		return
	}
	cpu.inInterrupt.Store(true)

	// Push return address (PC) onto stack
	cpu.regs[31] -= 8
	sp := uint32(cpu.regs[31])
	binary.LittleEndian.PutUint64(cpu.memory[sp:], cpu.PC)

	// Jump to interrupt vector
	cpu.PC = cpu.interruptVector
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

	// Use local running flag to avoid atomic load every iteration
	// Check external stop signal every 4096 instructions
	running := true
	checkCounter := uint32(0)

	// Unsafe base pointer for bounds-check-free memory access
	memBase := unsafe.Pointer(&cpu.memory[0])

	for running {
		// Periodic check of external stop signal (every 4096 instructions)
		checkCounter++
		if checkCounter&0xFFF == 0 && !cpu.running.Load() {
			break
		}

		// Performance measurement: count instructions and report periodically
		if cpu.PerfEnabled {
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

		// Fetch instruction using unsafe pointer (no bounds checking)
		instrPtr := unsafe.Pointer(uintptr(memBase) + uintptr(pc32))

		opcode := *(*byte)(instrPtr)
		byte1 := *(*byte)(unsafe.Pointer(uintptr(instrPtr) + 1))
		byte2 := *(*byte)(unsafe.Pointer(uintptr(instrPtr) + 2))
		byte3 := *(*byte)(unsafe.Pointer(uintptr(instrPtr) + 3))
		imm32 := *(*uint32)(unsafe.Pointer(uintptr(instrPtr) + 4))

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
						if cpu.interruptEnabled.Load() && !cpu.inInterrupt.Load() {
							cpu.handleInterrupt()
						}
						// Reload timer if still enabled (handler may have disabled it)
						if cpu.timerEnabled.Load() {
							period := cpu.timerPeriod.Load()
							cpu.timerCount.Store(period)
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
				cpu.setReg(rd, maskToSize(uint64(imm32), size))
			} else {
				cpu.setReg(rd, maskToSize(cpu.regs[rs], size))
			}
			cpu.PC += IE64_INSTR_SIZE

		case OP_MOVT:
			val := cpu.regs[rd]
			val = (val & 0x00000000FFFFFFFF) | (uint64(imm32) << 32)
			cpu.setReg(rd, val)
			cpu.PC += IE64_INSTR_SIZE

		case OP_MOVEQ:
			cpu.setReg(rd, uint64(int64(int32(imm32)))) // sign-extend 32 to 64
			cpu.PC += IE64_INSTR_SIZE

		case OP_LEA:
			disp := int64(int32(imm32)) // sign-extend displacement
			cpu.setReg(rd, uint64(int64(cpu.regs[rs])+disp))
			cpu.PC += IE64_INSTR_SIZE

		case OP_LOAD:
			disp := int64(int32(imm32))
			addr := uint32(int64(cpu.regs[rs]) + disp)
			cpu.setReg(rd, cpu.loadMem(addr, size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_STORE:
			disp := int64(int32(imm32))
			addr := uint32(int64(cpu.regs[rs]) + disp)
			cpu.storeMem(addr, maskToSize(cpu.regs[rd], size), size)
			cpu.PC += IE64_INSTR_SIZE

		case OP_ADD:
			result := cpu.regs[rs] + operand3
			cpu.setReg(rd, maskToSize(result, size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_SUB:
			result := cpu.regs[rs] - operand3
			cpu.setReg(rd, maskToSize(result, size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_MULU:
			result := cpu.regs[rs] * operand3
			cpu.setReg(rd, maskToSize(result, size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_MULS:
			a := int64(cpu.regs[rs])
			b := int64(operand3)
			cpu.setReg(rd, maskToSize(uint64(a*b), size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_DIVU:
			if operand3 == 0 {
				cpu.setReg(rd, 0)
			} else {
				cpu.setReg(rd, maskToSize(cpu.regs[rs]/operand3, size))
			}
			cpu.PC += IE64_INSTR_SIZE

		case OP_DIVS:
			if operand3 == 0 {
				cpu.setReg(rd, 0)
			} else {
				a := int64(cpu.regs[rs])
				b := int64(operand3)
				cpu.setReg(rd, maskToSize(uint64(a/b), size))
			}
			cpu.PC += IE64_INSTR_SIZE

		case OP_MOD64:
			if operand3 == 0 {
				cpu.setReg(rd, 0)
			} else {
				cpu.setReg(rd, maskToSize(cpu.regs[rs]%operand3, size))
			}
			cpu.PC += IE64_INSTR_SIZE

		case OP_NEG:
			cpu.setReg(rd, maskToSize(uint64(-int64(cpu.regs[rs])), size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_AND64:
			cpu.setReg(rd, maskToSize(cpu.regs[rs]&operand3, size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_OR64:
			cpu.setReg(rd, maskToSize(cpu.regs[rs]|operand3, size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_EOR:
			cpu.setReg(rd, maskToSize(cpu.regs[rs]^operand3, size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_NOT64:
			cpu.setReg(rd, maskToSize(^cpu.regs[rs], size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_LSL:
			shift := operand3 & 63
			cpu.setReg(rd, maskToSize(cpu.regs[rs]<<shift, size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_LSR:
			shift := operand3 & 63
			cpu.setReg(rd, maskToSize(cpu.regs[rs]>>shift, size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_ASR:
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
			cpu.setReg(rd, maskToSize(uint64(sval>>shift), size))
			cpu.PC += IE64_INSTR_SIZE

		case OP_BRA:
			offset := int64(int32(imm32))
			cpu.PC = uint64(int64(cpu.PC) + offset)

		case OP_BEQ:
			if cpu.regs[rs] == cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}

		case OP_BNE:
			if cpu.regs[rs] != cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}

		case OP_BLT:
			if int64(cpu.regs[rs]) < int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}

		case OP_BGE:
			if int64(cpu.regs[rs]) >= int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}

		case OP_BGT:
			if int64(cpu.regs[rs]) > int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}

		case OP_BLE:
			if int64(cpu.regs[rs]) <= int64(cpu.regs[rt]) {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}

		case OP_BHI:
			if cpu.regs[rs] > cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}

		case OP_BLS:
			if cpu.regs[rs] <= cpu.regs[rt] {
				cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))
			} else {
				cpu.PC += IE64_INSTR_SIZE
			}

		case OP_JSR64:
			cpu.regs[31] -= 8 // SP -= 8
			sp := uint32(cpu.regs[31])
			retAddr := cpu.PC + IE64_INSTR_SIZE
			binary.LittleEndian.PutUint64(cpu.memory[sp:], retAddr)
			cpu.PC = uint64(int64(cpu.PC) + int64(int32(imm32)))

		case OP_RTS64:
			sp := uint32(cpu.regs[31])
			cpu.PC = binary.LittleEndian.Uint64(cpu.memory[sp:])
			cpu.regs[31] += 8

		case OP_PUSH64:
			cpu.regs[31] -= 8
			sp := uint32(cpu.regs[31])
			binary.LittleEndian.PutUint64(cpu.memory[sp:], cpu.regs[rs])
			cpu.PC += IE64_INSTR_SIZE

		case OP_POP64:
			sp := uint32(cpu.regs[31])
			cpu.setReg(rd, binary.LittleEndian.Uint64(cpu.memory[sp:]))
			cpu.regs[31] += 8
			cpu.PC += IE64_INSTR_SIZE

		case OP_NOP64:
			cpu.PC += IE64_INSTR_SIZE

		case OP_HALT64:
			cpu.running.Store(false)
			running = false

		case OP_SEI64:
			cpu.interruptEnabled.Store(true)
			cpu.PC += IE64_INSTR_SIZE

		case OP_CLI64:
			cpu.interruptEnabled.Store(false)
			cpu.PC += IE64_INSTR_SIZE

		case OP_RTI64:
			sp := uint32(cpu.regs[31])
			cpu.PC = binary.LittleEndian.Uint64(cpu.memory[sp:])
			cpu.regs[31] += 8
			cpu.inInterrupt.Store(false)

		case OP_WAIT64:
			if imm32 > 0 {
				time.Sleep(time.Duration(imm32) * time.Microsecond)
			}
			cpu.PC += IE64_INSTR_SIZE

		default:
			fmt.Printf("IE64: Invalid opcode 0x%02X at PC=0x%X\n", opcode, cpu.PC)
			cpu.running.Store(false)
			running = false
		}
	}
}
