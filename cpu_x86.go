// cpu_x86.go - Intel x86 CPU Emulator (8086 base + 386 32-bit extensions)
//
// This implements an x86 CPU with:
// - Full 8086/8088 instruction set (testable via SingleStepTests/8088)
// - 386 32-bit register/operand extensions
// - Flat memory model (simplified segmentation)
// - Port I/O support for hardware integration
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import (
	"fmt"
	"sync/atomic"
)

// X86Bus defines the interface for x86 memory and I/O operations
type X86Bus interface {
	Read(addr uint32) byte
	Write(addr uint32, value byte)
	In(port uint16) byte
	Out(port uint16, value byte)
	Tick(cycles int)
}

// CPU_X86 represents the x86 CPU state
type CPU_X86 struct {
	// General purpose registers (32-bit)
	EAX uint32
	EBX uint32
	ECX uint32
	EDX uint32
	ESI uint32
	EDI uint32
	EBP uint32
	ESP uint32

	// Instruction pointer
	EIP uint32

	// Segment registers (16-bit)
	CS uint16
	DS uint16
	ES uint16
	SS uint16
	FS uint16
	GS uint16

	// Flags register
	Flags uint32

	// Execution state
	Halted  bool
	running atomic.Bool // Atomic for lock-free access (was: Running bool)
	Cycles  uint64

	// Interrupt state
	irqLine    bool
	irqPending atomic.Bool
	irqVector  atomic.Uint32

	// Current instruction state
	prefixSeg      int  // Segment override (-1 = none, 0-5 = ES/CS/SS/DS/FS/GS)
	prefixRep      int  // REP prefix (0 = none, 1 = REP/REPE, 2 = REPNE)
	prefixOpSize   bool // Operand size prefix (0x66)
	prefixAddrSize bool // Address size prefix (0x67)
	opcode         byte // Current opcode
	modrm          byte // ModR/M byte
	modrmLoaded    bool // ModR/M already fetched
	sib            byte // SIB byte
	sibLoaded      bool // SIB already fetched

	// Bus interface
	bus X86Bus

	// Instruction dispatch tables
	baseOps     [256]func(*CPU_X86)
	extendedOps [256]func(*CPU_X86) // 0x0F prefix opcodes

	// Register pointer array for O(1) lookup (avoids switch overhead)
	// Order: EAX, ECX, EDX, EBX, ESP, EBP, ESI, EDI
	regs32 [8]*uint32
}

// Flag bit positions
const (
	x86FlagCF   = 1 << 0  // Carry Flag
	x86FlagPF   = 1 << 2  // Parity Flag
	x86FlagAF   = 1 << 4  // Auxiliary Carry Flag
	x86FlagZF   = 1 << 6  // Zero Flag
	x86FlagSF   = 1 << 7  // Sign Flag
	x86FlagTF   = 1 << 8  // Trap Flag
	x86FlagIF   = 1 << 9  // Interrupt Enable Flag
	x86FlagDF   = 1 << 10 // Direction Flag
	x86FlagOF   = 1 << 11 // Overflow Flag
	x86FlagIOPL = 3 << 12 // I/O Privilege Level (2 bits)
	x86FlagNT   = 1 << 14 // Nested Task
	x86FlagRF   = 1 << 16 // Resume Flag
	x86FlagVM   = 1 << 17 // Virtual-8086 Mode
	x86FlagAC   = 1 << 18 // Alignment Check
	x86FlagVIF  = 1 << 19 // Virtual Interrupt Flag
	x86FlagVIP  = 1 << 20 // Virtual Interrupt Pending
	x86FlagID   = 1 << 21 // ID Flag
)

// Segment register indices
const (
	x86SegES = 0
	x86SegCS = 1
	x86SegSS = 2
	x86SegDS = 3
	x86SegFS = 4
	x86SegGS = 5
)

// Memory size constants
const (
	x86MemorySize  = 32 * 1024 * 1024 // 32MB address space
	x86AddressMask = 0x01FFFFFF       // 25-bit address mask (32MB)
)

// NewCPU_X86 creates a new x86 CPU instance
func NewCPU_X86(bus X86Bus) *CPU_X86 {
	cpu := &CPU_X86{
		bus: bus,
	}
	// Initialize register pointer array for O(1) lookup
	cpu.regs32 = [8]*uint32{
		&cpu.EAX, &cpu.ECX, &cpu.EDX, &cpu.EBX,
		&cpu.ESP, &cpu.EBP, &cpu.ESI, &cpu.EDI,
	}
	cpu.initBaseOps()
	cpu.initExtendedOps()
	cpu.Reset()
	return cpu
}

// Reset initializes the CPU to its power-on state
func (c *CPU_X86) Reset() {
	// Clear general purpose registers
	c.EAX = 0
	c.EBX = 0
	c.ECX = 0
	c.EDX = 0
	c.ESI = 0
	c.EDI = 0
	c.EBP = 0
	c.ESP = 0

	// Reset EIP to standard x86 reset vector
	// In real mode, this would be CS:IP = F000:FFF0
	// For our flat model, we'll use 0x00000000 as the entry point
	c.EIP = 0x00000000

	// Initialize segment registers
	// For flat model, all segments effectively point to base 0
	c.CS = 0
	c.DS = 0
	c.ES = 0
	c.SS = 0
	c.FS = 0
	c.GS = 0

	// Reset flags (IF set by default for interrupts)
	c.Flags = x86FlagIF

	// Clear prefix state
	c.prefixSeg = -1
	c.prefixRep = 0
	c.prefixOpSize = false
	c.prefixAddrSize = false
	c.modrmLoaded = false
	c.sibLoaded = false

	// Clear interrupt state
	c.irqLine = false
	c.irqPending.Store(false)
	c.irqVector.Store(0)

	// Set execution state
	c.Halted = false
	c.running.Store(true)
	c.Cycles = 0
}

// Running returns the execution state (thread-safe)
func (c *CPU_X86) Running() bool {
	return c.running.Load()
}

// SetRunning sets the execution state (thread-safe)
func (c *CPU_X86) SetRunning(state bool) {
	c.running.Store(state)
}

// -----------------------------------------------------------------------------
// Register Access Helpers
// -----------------------------------------------------------------------------

// AX returns the lower 16 bits of EAX
func (c *CPU_X86) AX() uint16 {
	return uint16(c.EAX & 0xFFFF)
}

// SetAX sets the lower 16 bits of EAX
func (c *CPU_X86) SetAX(v uint16) {
	c.EAX = (c.EAX & 0xFFFF0000) | uint32(v)
}

// AL returns the lower 8 bits of EAX
func (c *CPU_X86) AL() byte {
	return byte(c.EAX & 0xFF)
}

// SetAL sets the lower 8 bits of EAX
func (c *CPU_X86) SetAL(v byte) {
	c.EAX = (c.EAX & 0xFFFFFF00) | uint32(v)
}

// AH returns bits 8-15 of EAX
func (c *CPU_X86) AH() byte {
	return byte((c.EAX >> 8) & 0xFF)
}

// SetAH sets bits 8-15 of EAX
func (c *CPU_X86) SetAH(v byte) {
	c.EAX = (c.EAX & 0xFFFF00FF) | (uint32(v) << 8)
}

// BX returns the lower 16 bits of EBX
func (c *CPU_X86) BX() uint16 {
	return uint16(c.EBX & 0xFFFF)
}

// SetBX sets the lower 16 bits of EBX
func (c *CPU_X86) SetBX(v uint16) {
	c.EBX = (c.EBX & 0xFFFF0000) | uint32(v)
}

// BL returns the lower 8 bits of EBX
func (c *CPU_X86) BL() byte {
	return byte(c.EBX & 0xFF)
}

// SetBL sets the lower 8 bits of EBX
func (c *CPU_X86) SetBL(v byte) {
	c.EBX = (c.EBX & 0xFFFFFF00) | uint32(v)
}

// BH returns bits 8-15 of EBX
func (c *CPU_X86) BH() byte {
	return byte((c.EBX >> 8) & 0xFF)
}

// SetBH sets bits 8-15 of EBX
func (c *CPU_X86) SetBH(v byte) {
	c.EBX = (c.EBX & 0xFFFF00FF) | (uint32(v) << 8)
}

// CX returns the lower 16 bits of ECX
func (c *CPU_X86) CX() uint16 {
	return uint16(c.ECX & 0xFFFF)
}

// SetCX sets the lower 16 bits of ECX
func (c *CPU_X86) SetCX(v uint16) {
	c.ECX = (c.ECX & 0xFFFF0000) | uint32(v)
}

// CL returns the lower 8 bits of ECX
func (c *CPU_X86) CL() byte {
	return byte(c.ECX & 0xFF)
}

// SetCL sets the lower 8 bits of ECX
func (c *CPU_X86) SetCL(v byte) {
	c.ECX = (c.ECX & 0xFFFFFF00) | uint32(v)
}

// CH returns bits 8-15 of ECX
func (c *CPU_X86) CH() byte {
	return byte((c.ECX >> 8) & 0xFF)
}

// SetCH sets bits 8-15 of ECX
func (c *CPU_X86) SetCH(v byte) {
	c.ECX = (c.ECX & 0xFFFF00FF) | (uint32(v) << 8)
}

// DX returns the lower 16 bits of EDX
func (c *CPU_X86) DX() uint16 {
	return uint16(c.EDX & 0xFFFF)
}

// SetDX sets the lower 16 bits of EDX
func (c *CPU_X86) SetDX(v uint16) {
	c.EDX = (c.EDX & 0xFFFF0000) | uint32(v)
}

// DL returns the lower 8 bits of EDX
func (c *CPU_X86) DL() byte {
	return byte(c.EDX & 0xFF)
}

// SetDL sets the lower 8 bits of EDX
func (c *CPU_X86) SetDL(v byte) {
	c.EDX = (c.EDX & 0xFFFFFF00) | uint32(v)
}

// DH returns bits 8-15 of EDX
func (c *CPU_X86) DH() byte {
	return byte((c.EDX >> 8) & 0xFF)
}

// SetDH sets bits 8-15 of EDX
func (c *CPU_X86) SetDH(v byte) {
	c.EDX = (c.EDX & 0xFFFF00FF) | (uint32(v) << 8)
}

// SI returns the lower 16 bits of ESI
func (c *CPU_X86) SI() uint16 {
	return uint16(c.ESI & 0xFFFF)
}

// SetSI sets the lower 16 bits of ESI
func (c *CPU_X86) SetSI(v uint16) {
	c.ESI = (c.ESI & 0xFFFF0000) | uint32(v)
}

// DI returns the lower 16 bits of EDI
func (c *CPU_X86) DI() uint16 {
	return uint16(c.EDI & 0xFFFF)
}

// SetDI sets the lower 16 bits of EDI
func (c *CPU_X86) SetDI(v uint16) {
	c.EDI = (c.EDI & 0xFFFF0000) | uint32(v)
}

// BP returns the lower 16 bits of EBP
func (c *CPU_X86) BP() uint16 {
	return uint16(c.EBP & 0xFFFF)
}

// SetBP sets the lower 16 bits of EBP
func (c *CPU_X86) SetBP(v uint16) {
	c.EBP = (c.EBP & 0xFFFF0000) | uint32(v)
}

// SP returns the lower 16 bits of ESP
func (c *CPU_X86) SP() uint16 {
	return uint16(c.ESP & 0xFFFF)
}

// SetSP sets the lower 16 bits of ESP
func (c *CPU_X86) SetSP(v uint16) {
	c.ESP = (c.ESP & 0xFFFF0000) | uint32(v)
}

// IP returns the lower 16 bits of EIP
func (c *CPU_X86) IP() uint16 {
	return uint16(c.EIP & 0xFFFF)
}

// SetIP sets the lower 16 bits of EIP
func (c *CPU_X86) SetIP(v uint16) {
	c.EIP = (c.EIP & 0xFFFF0000) | uint32(v)
}

// -----------------------------------------------------------------------------
// Register access by index
// -----------------------------------------------------------------------------

// getReg8 returns an 8-bit register value by index (0-7: AL, CL, DL, BL, AH, CH, DH, BH)
func (c *CPU_X86) getReg8(idx byte) byte {
	switch idx & 7 {
	case 0:
		return c.AL()
	case 1:
		return c.CL()
	case 2:
		return c.DL()
	case 3:
		return c.BL()
	case 4:
		return c.AH()
	case 5:
		return c.CH()
	case 6:
		return c.DH()
	case 7:
		return c.BH()
	}
	return 0
}

// setReg8 sets an 8-bit register value by index
func (c *CPU_X86) setReg8(idx byte, v byte) {
	switch idx & 7 {
	case 0:
		c.SetAL(v)
	case 1:
		c.SetCL(v)
	case 2:
		c.SetDL(v)
	case 3:
		c.SetBL(v)
	case 4:
		c.SetAH(v)
	case 5:
		c.SetCH(v)
	case 6:
		c.SetDH(v)
	case 7:
		c.SetBH(v)
	}
}

// getReg16 returns a 16-bit register value by index (0-7: AX, CX, DX, BX, SP, BP, SI, DI)
func (c *CPU_X86) getReg16(idx byte) uint16 {
	switch idx & 7 {
	case 0:
		return c.AX()
	case 1:
		return c.CX()
	case 2:
		return c.DX()
	case 3:
		return c.BX()
	case 4:
		return c.SP()
	case 5:
		return c.BP()
	case 6:
		return c.SI()
	case 7:
		return c.DI()
	}
	return 0
}

// setReg16 sets a 16-bit register value by index
func (c *CPU_X86) setReg16(idx byte, v uint16) {
	switch idx & 7 {
	case 0:
		c.SetAX(v)
	case 1:
		c.SetCX(v)
	case 2:
		c.SetDX(v)
	case 3:
		c.SetBX(v)
	case 4:
		c.SetSP(v)
	case 5:
		c.SetBP(v)
	case 6:
		c.SetSI(v)
	case 7:
		c.SetDI(v)
	}
}

// getReg32 returns a 32-bit register value by index (0-7: EAX, ECX, EDX, EBX, ESP, EBP, ESI, EDI)
// Uses pointer array for O(1) lookup instead of switch statement
func (c *CPU_X86) getReg32(idx byte) uint32 {
	return *c.regs32[idx&7]
}

// setReg32 sets a 32-bit register value by index
// Uses pointer array for O(1) lookup instead of switch statement
func (c *CPU_X86) setReg32(idx byte, v uint32) {
	*c.regs32[idx&7] = v
}

// getSeg returns a segment register value by index
func (c *CPU_X86) getSeg(idx int) uint16 {
	switch idx {
	case x86SegES:
		return c.ES
	case x86SegCS:
		return c.CS
	case x86SegSS:
		return c.SS
	case x86SegDS:
		return c.DS
	case x86SegFS:
		return c.FS
	case x86SegGS:
		return c.GS
	}
	return 0
}

// setSeg sets a segment register value by index
func (c *CPU_X86) setSeg(idx int, v uint16) {
	switch idx {
	case x86SegES:
		c.ES = v
	case x86SegCS:
		c.CS = v
	case x86SegSS:
		c.SS = v
	case x86SegDS:
		c.DS = v
	case x86SegFS:
		c.FS = v
	case x86SegGS:
		c.GS = v
	}
}

// -----------------------------------------------------------------------------
// Flag Helpers
// -----------------------------------------------------------------------------

// getFlag returns true if the specified flag is set
func (c *CPU_X86) getFlag(flag uint32) bool {
	return (c.Flags & flag) != 0
}

// setFlag sets or clears a flag
func (c *CPU_X86) setFlag(flag uint32, set bool) {
	if set {
		c.Flags |= flag
	} else {
		c.Flags &^= flag
	}
}

// CF returns the Carry Flag
func (c *CPU_X86) CF() bool {
	return c.getFlag(x86FlagCF)
}

// ZF returns the Zero Flag
func (c *CPU_X86) ZF() bool {
	return c.getFlag(x86FlagZF)
}

// SF returns the Sign Flag
func (c *CPU_X86) SF() bool {
	return c.getFlag(x86FlagSF)
}

// OF returns the Overflow Flag
func (c *CPU_X86) OF() bool {
	return c.getFlag(x86FlagOF)
}

// PF returns the Parity Flag
func (c *CPU_X86) PF() bool {
	return c.getFlag(x86FlagPF)
}

// AF returns the Auxiliary Carry Flag
func (c *CPU_X86) AF() bool {
	return c.getFlag(x86FlagAF)
}

// DF returns the Direction Flag
func (c *CPU_X86) DF() bool {
	return c.getFlag(x86FlagDF)
}

// IF returns the Interrupt Enable Flag
func (c *CPU_X86) IF() bool {
	return c.getFlag(x86FlagIF)
}

// parity returns the parity of the low byte (true = even, false = odd)
func parity(v byte) bool {
	v ^= v >> 4
	v ^= v >> 2
	v ^= v >> 1
	return (v & 1) == 0
}

// setFlagsArith8 sets flags after an 8-bit arithmetic operation
func (c *CPU_X86) setFlagsArith8(result uint16, a, b byte, sub bool) {
	r := byte(result)
	c.setFlag(x86FlagCF, result > 0xFF)
	c.setFlag(x86FlagZF, r == 0)
	c.setFlag(x86FlagSF, (r&0x80) != 0)
	c.setFlag(x86FlagPF, parity(r))

	// Overflow: sign of result differs from expected
	if sub {
		c.setFlag(x86FlagOF, ((a^b)&(a^r)&0x80) != 0)
		c.setFlag(x86FlagAF, (a&0x0F) < (b&0x0F))
	} else {
		c.setFlag(x86FlagOF, ((^(a ^ b))&(a^r)&0x80) != 0)
		c.setFlag(x86FlagAF, ((a&0x0F)+(b&0x0F)) > 0x0F)
	}
}

// setFlagsArith16 sets flags after a 16-bit arithmetic operation
func (c *CPU_X86) setFlagsArith16(result uint32, a, b uint16, sub bool) {
	r := uint16(result)
	c.setFlag(x86FlagCF, result > 0xFFFF)
	c.setFlag(x86FlagZF, r == 0)
	c.setFlag(x86FlagSF, (r&0x8000) != 0)
	c.setFlag(x86FlagPF, parity(byte(r)))

	if sub {
		c.setFlag(x86FlagOF, ((a^b)&(a^r)&0x8000) != 0)
		c.setFlag(x86FlagAF, (a&0x0F) < (b&0x0F))
	} else {
		c.setFlag(x86FlagOF, ((^(a ^ b))&(a^r)&0x8000) != 0)
		c.setFlag(x86FlagAF, ((a&0x0F)+(b&0x0F)) > 0x0F)
	}
}

// setFlagsArith32 sets flags after a 32-bit arithmetic operation
func (c *CPU_X86) setFlagsArith32(result uint64, a, b uint32, sub bool) {
	r := uint32(result)
	c.setFlag(x86FlagCF, result > 0xFFFFFFFF)
	c.setFlag(x86FlagZF, r == 0)
	c.setFlag(x86FlagSF, (r&0x80000000) != 0)
	c.setFlag(x86FlagPF, parity(byte(r)))

	if sub {
		c.setFlag(x86FlagOF, ((a^b)&(a^r)&0x80000000) != 0)
		c.setFlag(x86FlagAF, (a&0x0F) < (b&0x0F))
	} else {
		c.setFlag(x86FlagOF, ((^(a ^ b))&(a^r)&0x80000000) != 0)
		c.setFlag(x86FlagAF, ((a&0x0F)+(b&0x0F)) > 0x0F)
	}
}

// setFlagsLogic8 sets flags after an 8-bit logical operation
func (c *CPU_X86) setFlagsLogic8(result byte) {
	c.setFlag(x86FlagCF, false)
	c.setFlag(x86FlagOF, false)
	c.setFlag(x86FlagZF, result == 0)
	c.setFlag(x86FlagSF, (result&0x80) != 0)
	c.setFlag(x86FlagPF, parity(result))
	// AF is undefined for logical ops
}

// setFlagsLogic16 sets flags after a 16-bit logical operation
func (c *CPU_X86) setFlagsLogic16(result uint16) {
	c.setFlag(x86FlagCF, false)
	c.setFlag(x86FlagOF, false)
	c.setFlag(x86FlagZF, result == 0)
	c.setFlag(x86FlagSF, (result&0x8000) != 0)
	c.setFlag(x86FlagPF, parity(byte(result)))
}

// setFlagsLogic32 sets flags after a 32-bit logical operation
func (c *CPU_X86) setFlagsLogic32(result uint32) {
	c.setFlag(x86FlagCF, false)
	c.setFlag(x86FlagOF, false)
	c.setFlag(x86FlagZF, result == 0)
	c.setFlag(x86FlagSF, (result&0x80000000) != 0)
	c.setFlag(x86FlagPF, parity(byte(result)))
}

// -----------------------------------------------------------------------------
// Memory Access
// -----------------------------------------------------------------------------

// fetch8 fetches a byte at EIP and increments EIP
func (c *CPU_X86) fetch8() byte {
	v := c.bus.Read(c.EIP & x86AddressMask)
	c.EIP++
	return v
}

// fetch16 fetches a 16-bit word at EIP (little-endian) and increments EIP
func (c *CPU_X86) fetch16() uint16 {
	lo := c.bus.Read(c.EIP & x86AddressMask)
	c.EIP++
	hi := c.bus.Read(c.EIP & x86AddressMask)
	c.EIP++
	return uint16(lo) | (uint16(hi) << 8)
}

// fetch32 fetches a 32-bit dword at EIP (little-endian) and increments EIP
func (c *CPU_X86) fetch32() uint32 {
	b0 := c.bus.Read(c.EIP & x86AddressMask)
	c.EIP++
	b1 := c.bus.Read(c.EIP & x86AddressMask)
	c.EIP++
	b2 := c.bus.Read(c.EIP & x86AddressMask)
	c.EIP++
	b3 := c.bus.Read(c.EIP & x86AddressMask)
	c.EIP++
	return uint32(b0) | (uint32(b1) << 8) | (uint32(b2) << 16) | (uint32(b3) << 24)
}

// read8 reads a byte from memory
func (c *CPU_X86) read8(addr uint32) byte {
	return c.bus.Read(addr & x86AddressMask)
}

// read16 reads a 16-bit word from memory (little-endian)
func (c *CPU_X86) read16(addr uint32) uint16 {
	lo := c.bus.Read(addr & x86AddressMask)
	hi := c.bus.Read((addr + 1) & x86AddressMask)
	return uint16(lo) | (uint16(hi) << 8)
}

// read32 reads a 32-bit dword from memory (little-endian)
func (c *CPU_X86) read32(addr uint32) uint32 {
	b0 := c.bus.Read(addr & x86AddressMask)
	b1 := c.bus.Read((addr + 1) & x86AddressMask)
	b2 := c.bus.Read((addr + 2) & x86AddressMask)
	b3 := c.bus.Read((addr + 3) & x86AddressMask)
	return uint32(b0) | (uint32(b1) << 8) | (uint32(b2) << 16) | (uint32(b3) << 24)
}

// write8 writes a byte to memory
func (c *CPU_X86) write8(addr uint32, v byte) {
	c.bus.Write(addr&x86AddressMask, v)
}

// write16 writes a 16-bit word to memory (little-endian)
func (c *CPU_X86) write16(addr uint32, v uint16) {
	c.bus.Write(addr&x86AddressMask, byte(v))
	c.bus.Write((addr+1)&x86AddressMask, byte(v>>8))
}

// write32 writes a 32-bit dword to memory (little-endian)
func (c *CPU_X86) write32(addr uint32, v uint32) {
	c.bus.Write(addr&x86AddressMask, byte(v))
	c.bus.Write((addr+1)&x86AddressMask, byte(v>>8))
	c.bus.Write((addr+2)&x86AddressMask, byte(v>>16))
	c.bus.Write((addr+3)&x86AddressMask, byte(v>>24))
}

// -----------------------------------------------------------------------------
// Stack Operations
// -----------------------------------------------------------------------------

// push16 pushes a 16-bit value onto the stack
func (c *CPU_X86) push16(v uint16) {
	c.ESP -= 2
	c.write16(c.ESP, v)
}

// pop16 pops a 16-bit value from the stack
func (c *CPU_X86) pop16() uint16 {
	v := c.read16(c.ESP)
	c.ESP += 2
	return v
}

// push32 pushes a 32-bit value onto the stack
func (c *CPU_X86) push32(v uint32) {
	c.ESP -= 4
	c.write32(c.ESP, v)
}

// pop32 pops a 32-bit value from the stack
func (c *CPU_X86) pop32() uint32 {
	v := c.read32(c.ESP)
	c.ESP += 4
	return v
}

// -----------------------------------------------------------------------------
// ModR/M and SIB Decoding
// -----------------------------------------------------------------------------

// fetchModRM fetches and caches the ModR/M byte
func (c *CPU_X86) fetchModRM() byte {
	if !c.modrmLoaded {
		c.modrm = c.fetch8()
		c.modrmLoaded = true
	}
	return c.modrm
}

// getModRMReg returns the reg field of ModR/M (bits 5-3)
func (c *CPU_X86) getModRMReg() byte {
	return (c.fetchModRM() >> 3) & 7
}

// getModRMRM returns the r/m field of ModR/M (bits 2-0)
func (c *CPU_X86) getModRMRM() byte {
	return c.fetchModRM() & 7
}

// getModRMMod returns the mod field of ModR/M (bits 7-6)
func (c *CPU_X86) getModRMMod() byte {
	return (c.fetchModRM() >> 6) & 3
}

// fetchSIB fetches and caches the SIB byte
func (c *CPU_X86) fetchSIB() byte {
	if !c.sibLoaded {
		c.sib = c.fetch8()
		c.sibLoaded = true
	}
	return c.sib
}

// getSIBScale returns the scale field (bits 7-6)
func (c *CPU_X86) getSIBScale() byte {
	return (c.fetchSIB() >> 6) & 3
}

// getSIBIndex returns the index field (bits 5-3)
func (c *CPU_X86) getSIBIndex() byte {
	return (c.fetchSIB() >> 3) & 7
}

// getSIBBase returns the base field (bits 2-0)
func (c *CPU_X86) getSIBBase() byte {
	return c.fetchSIB() & 7
}

// calcEffectiveAddress16 calculates effective address for 16-bit addressing mode
func (c *CPU_X86) calcEffectiveAddress16() uint32 {
	mod := c.getModRMMod()
	rm := c.getModRMRM()

	var base uint16
	var seg int = x86SegDS // Default segment

	switch rm {
	case 0: // [BX+SI]
		base = c.BX() + c.SI()
	case 1: // [BX+DI]
		base = c.BX() + c.DI()
	case 2: // [BP+SI]
		base = c.BP() + c.SI()
		seg = x86SegSS
	case 3: // [BP+DI]
		base = c.BP() + c.DI()
		seg = x86SegSS
	case 4: // [SI]
		base = c.SI()
	case 5: // [DI]
		base = c.DI()
	case 6: // [BP] or [disp16]
		if mod == 0 {
			base = c.fetch16()
		} else {
			base = c.BP()
			seg = x86SegSS
		}
	case 7: // [BX]
		base = c.BX()
	}

	// Add displacement
	switch mod {
	case 1: // 8-bit displacement
		disp := int8(c.fetch8())
		base = uint16(int16(base) + int16(disp))
	case 2: // 16-bit displacement
		disp := c.fetch16()
		base += disp
	}

	// Apply segment override if present
	if c.prefixSeg >= 0 {
		seg = c.prefixSeg
	}

	// For flat model, segment base is 0
	_ = seg
	return uint32(base)
}

// calcEffectiveAddress32 calculates effective address for 32-bit addressing mode
func (c *CPU_X86) calcEffectiveAddress32() uint32 {
	mod := c.getModRMMod()
	rm := c.getModRMRM()

	var addr uint32
	var seg int = x86SegDS

	if rm == 4 {
		// SIB byte follows
		scale := c.getSIBScale()
		index := c.getSIBIndex()
		base := c.getSIBBase()

		// Calculate base
		if base == 5 && mod == 0 {
			addr = c.fetch32()
		} else {
			addr = c.getReg32(base)
			if base == 4 || base == 5 { // ESP or EBP
				seg = x86SegSS
			}
		}

		// Add scaled index (index 4 = no index)
		if index != 4 {
			addr += c.getReg32(index) << scale
		}
	} else if rm == 5 && mod == 0 {
		// Direct 32-bit address
		addr = c.fetch32()
	} else {
		addr = c.getReg32(rm)
		if rm == 4 || rm == 5 { // ESP or EBP
			seg = x86SegSS
		}
	}

	// Add displacement
	switch mod {
	case 1: // 8-bit displacement (sign-extended)
		disp := int8(c.fetch8())
		addr = uint32(int32(addr) + int32(disp))
	case 2: // 32-bit displacement
		addr += c.fetch32()
	}

	// Apply segment override if present
	if c.prefixSeg >= 0 {
		seg = c.prefixSeg
	}
	_ = seg

	return addr
}

// getEffectiveAddress returns the effective address for the current ModR/M
func (c *CPU_X86) getEffectiveAddress() uint32 {
	if c.prefixAddrSize {
		return c.calcEffectiveAddress16()
	}
	return c.calcEffectiveAddress32()
}

// readRM8 reads an 8-bit value from register or memory based on ModR/M
func (c *CPU_X86) readRM8() byte {
	if c.getModRMMod() == 3 {
		return c.getReg8(c.getModRMRM())
	}
	return c.read8(c.getEffectiveAddress())
}

// writeRM8 writes an 8-bit value to register or memory based on ModR/M
func (c *CPU_X86) writeRM8(v byte) {
	if c.getModRMMod() == 3 {
		c.setReg8(c.getModRMRM(), v)
	} else {
		c.write8(c.getEffectiveAddress(), v)
	}
}

// readRM16 reads a 16-bit value from register or memory based on ModR/M
func (c *CPU_X86) readRM16() uint16 {
	if c.getModRMMod() == 3 {
		return c.getReg16(c.getModRMRM())
	}
	return c.read16(c.getEffectiveAddress())
}

// writeRM16 writes a 16-bit value to register or memory based on ModR/M
func (c *CPU_X86) writeRM16(v uint16) {
	if c.getModRMMod() == 3 {
		c.setReg16(c.getModRMRM(), v)
	} else {
		c.write16(c.getEffectiveAddress(), v)
	}
}

// readRM32 reads a 32-bit value from register or memory based on ModR/M
func (c *CPU_X86) readRM32() uint32 {
	if c.getModRMMod() == 3 {
		return c.getReg32(c.getModRMRM())
	}
	return c.read32(c.getEffectiveAddress())
}

// writeRM32 writes a 32-bit value to register or memory based on ModR/M
func (c *CPU_X86) writeRM32(v uint32) {
	if c.getModRMMod() == 3 {
		c.setReg32(c.getModRMRM(), v)
	} else {
		c.write32(c.getEffectiveAddress(), v)
	}
}

// -----------------------------------------------------------------------------
// Instruction Execution
// -----------------------------------------------------------------------------

// Step executes a single instruction
func (c *CPU_X86) Step() int {
	if c.Halted || !c.running.Load() {
		return 0
	}

	// Check for pending interrupt
	if c.irqPending.Load() && c.IF() {
		c.handleInterrupt(byte(c.irqVector.Load()))
		c.irqPending.Store(false)
	}

	// Reset prefix state
	c.prefixSeg = -1
	c.prefixRep = 0
	c.prefixOpSize = false
	c.prefixAddrSize = false
	c.modrmLoaded = false
	c.sibLoaded = false

	startCycles := c.Cycles

	// Fetch and handle prefixes
	for {
		c.opcode = c.fetch8()

		switch c.opcode {
		case 0x26: // ES:
			c.prefixSeg = x86SegES
		case 0x2E: // CS:
			c.prefixSeg = x86SegCS
		case 0x36: // SS:
			c.prefixSeg = x86SegSS
		case 0x3E: // DS:
			c.prefixSeg = x86SegDS
		case 0x64: // FS:
			c.prefixSeg = x86SegFS
		case 0x65: // GS:
			c.prefixSeg = x86SegGS
		case 0x66: // Operand size
			c.prefixOpSize = true
		case 0x67: // Address size
			c.prefixAddrSize = true
		case 0xF0: // LOCK (ignore for now)
			continue
		case 0xF2: // REPNE
			c.prefixRep = 2
		case 0xF3: // REP/REPE
			c.prefixRep = 1
		default:
			// Not a prefix, execute the instruction
			if handler := c.baseOps[c.opcode]; handler != nil {
				handler(c)
			} else {
				// Undefined opcode - halt
				fmt.Printf("X86: Undefined opcode 0x%02X at EIP=0x%08X, halting\n", c.opcode, c.EIP-1)
				c.Halted = true
			}
			goto done
		}
	}

done:
	cycles := int(c.Cycles - startCycles)
	if cycles == 0 {
		cycles = 1 // Minimum 1 cycle per instruction
	}
	c.bus.Tick(cycles)
	return cycles
}

// handleInterrupt handles an interrupt
func (c *CPU_X86) handleInterrupt(vector byte) {
	// Push flags, CS, IP
	c.push16(uint16(c.Flags))
	c.push16(c.CS)
	c.push16(c.IP())

	// Clear IF and TF
	c.setFlag(x86FlagIF, false)
	c.setFlag(x86FlagTF, false)

	// Load interrupt vector
	// In real mode, vector table is at 0x0000:0x0000
	addr := uint32(vector) * 4
	newIP := c.read16(addr)
	newCS := c.read16(addr + 2)

	c.SetIP(newIP)
	c.CS = newCS
}

// SetIRQ sets or clears the interrupt request line
func (c *CPU_X86) SetIRQ(active bool, vector byte) {
	c.irqLine = active
	if active {
		c.irqPending.Store(true)
		c.irqVector.Store(uint32(vector))
	}
}

// -----------------------------------------------------------------------------
// Instruction Table Initialization
// -----------------------------------------------------------------------------

// initBaseOps initializes the base opcode dispatch table
func (c *CPU_X86) initBaseOps() {
	// Clear all entries first
	for i := range c.baseOps {
		c.baseOps[i] = nil
	}

	// 0x00-0x05: ADD
	c.baseOps[0x00] = (*CPU_X86).opADD_Eb_Gb
	c.baseOps[0x01] = (*CPU_X86).opADD_Ev_Gv
	c.baseOps[0x02] = (*CPU_X86).opADD_Gb_Eb
	c.baseOps[0x03] = (*CPU_X86).opADD_Gv_Ev
	c.baseOps[0x04] = (*CPU_X86).opADD_AL_Ib
	c.baseOps[0x05] = (*CPU_X86).opADD_AX_Iv

	// 0x06-0x07: PUSH/POP ES
	c.baseOps[0x06] = (*CPU_X86).opPUSH_ES
	c.baseOps[0x07] = (*CPU_X86).opPOP_ES

	// 0x08-0x0D: OR
	c.baseOps[0x08] = (*CPU_X86).opOR_Eb_Gb
	c.baseOps[0x09] = (*CPU_X86).opOR_Ev_Gv
	c.baseOps[0x0A] = (*CPU_X86).opOR_Gb_Eb
	c.baseOps[0x0B] = (*CPU_X86).opOR_Gv_Ev
	c.baseOps[0x0C] = (*CPU_X86).opOR_AL_Ib
	c.baseOps[0x0D] = (*CPU_X86).opOR_AX_Iv

	// 0x0E: PUSH CS
	c.baseOps[0x0E] = (*CPU_X86).opPUSH_CS

	// 0x0F: Two-byte opcode prefix
	c.baseOps[0x0F] = (*CPU_X86).opTwoBytePrefix

	// 0x10-0x15: ADC
	c.baseOps[0x10] = (*CPU_X86).opADC_Eb_Gb
	c.baseOps[0x11] = (*CPU_X86).opADC_Ev_Gv
	c.baseOps[0x12] = (*CPU_X86).opADC_Gb_Eb
	c.baseOps[0x13] = (*CPU_X86).opADC_Gv_Ev
	c.baseOps[0x14] = (*CPU_X86).opADC_AL_Ib
	c.baseOps[0x15] = (*CPU_X86).opADC_AX_Iv

	// 0x16-0x17: PUSH/POP SS
	c.baseOps[0x16] = (*CPU_X86).opPUSH_SS
	c.baseOps[0x17] = (*CPU_X86).opPOP_SS

	// 0x18-0x1D: SBB
	c.baseOps[0x18] = (*CPU_X86).opSBB_Eb_Gb
	c.baseOps[0x19] = (*CPU_X86).opSBB_Ev_Gv
	c.baseOps[0x1A] = (*CPU_X86).opSBB_Gb_Eb
	c.baseOps[0x1B] = (*CPU_X86).opSBB_Gv_Ev
	c.baseOps[0x1C] = (*CPU_X86).opSBB_AL_Ib
	c.baseOps[0x1D] = (*CPU_X86).opSBB_AX_Iv

	// 0x1E-0x1F: PUSH/POP DS
	c.baseOps[0x1E] = (*CPU_X86).opPUSH_DS
	c.baseOps[0x1F] = (*CPU_X86).opPOP_DS

	// 0x20-0x25: AND
	c.baseOps[0x20] = (*CPU_X86).opAND_Eb_Gb
	c.baseOps[0x21] = (*CPU_X86).opAND_Ev_Gv
	c.baseOps[0x22] = (*CPU_X86).opAND_Gb_Eb
	c.baseOps[0x23] = (*CPU_X86).opAND_Gv_Ev
	c.baseOps[0x24] = (*CPU_X86).opAND_AL_Ib
	c.baseOps[0x25] = (*CPU_X86).opAND_AX_Iv

	// 0x27: DAA
	c.baseOps[0x27] = (*CPU_X86).opDAA

	// 0x28-0x2D: SUB
	c.baseOps[0x28] = (*CPU_X86).opSUB_Eb_Gb
	c.baseOps[0x29] = (*CPU_X86).opSUB_Ev_Gv
	c.baseOps[0x2A] = (*CPU_X86).opSUB_Gb_Eb
	c.baseOps[0x2B] = (*CPU_X86).opSUB_Gv_Ev
	c.baseOps[0x2C] = (*CPU_X86).opSUB_AL_Ib
	c.baseOps[0x2D] = (*CPU_X86).opSUB_AX_Iv

	// 0x2F: DAS
	c.baseOps[0x2F] = (*CPU_X86).opDAS

	// 0x30-0x35: XOR
	c.baseOps[0x30] = (*CPU_X86).opXOR_Eb_Gb
	c.baseOps[0x31] = (*CPU_X86).opXOR_Ev_Gv
	c.baseOps[0x32] = (*CPU_X86).opXOR_Gb_Eb
	c.baseOps[0x33] = (*CPU_X86).opXOR_Gv_Ev
	c.baseOps[0x34] = (*CPU_X86).opXOR_AL_Ib
	c.baseOps[0x35] = (*CPU_X86).opXOR_AX_Iv

	// 0x37: AAA
	c.baseOps[0x37] = (*CPU_X86).opAAA

	// 0x38-0x3D: CMP
	c.baseOps[0x38] = (*CPU_X86).opCMP_Eb_Gb
	c.baseOps[0x39] = (*CPU_X86).opCMP_Ev_Gv
	c.baseOps[0x3A] = (*CPU_X86).opCMP_Gb_Eb
	c.baseOps[0x3B] = (*CPU_X86).opCMP_Gv_Ev
	c.baseOps[0x3C] = (*CPU_X86).opCMP_AL_Ib
	c.baseOps[0x3D] = (*CPU_X86).opCMP_AX_Iv

	// 0x3F: AAS
	c.baseOps[0x3F] = (*CPU_X86).opAAS

	// 0x40-0x47: INC r16/r32
	for i := 0; i < 8; i++ {
		idx := i
		c.baseOps[0x40+i] = func(cpu *CPU_X86) { cpu.opINC_reg(byte(idx)) }
	}

	// 0x48-0x4F: DEC r16/r32
	for i := 0; i < 8; i++ {
		idx := i
		c.baseOps[0x48+i] = func(cpu *CPU_X86) { cpu.opDEC_reg(byte(idx)) }
	}

	// 0x50-0x57: PUSH r16/r32
	for i := 0; i < 8; i++ {
		idx := i
		c.baseOps[0x50+i] = func(cpu *CPU_X86) { cpu.opPUSH_reg(byte(idx)) }
	}

	// 0x58-0x5F: POP r16/r32
	for i := 0; i < 8; i++ {
		idx := i
		c.baseOps[0x58+i] = func(cpu *CPU_X86) { cpu.opPOP_reg(byte(idx)) }
	}

	// 0x60: PUSHA
	c.baseOps[0x60] = (*CPU_X86).opPUSHA

	// 0x61: POPA
	c.baseOps[0x61] = (*CPU_X86).opPOPA

	// 0x68: PUSH Iv
	c.baseOps[0x68] = (*CPU_X86).opPUSH_Iv

	// 0x69: IMUL Gv,Ev,Iv
	c.baseOps[0x69] = (*CPU_X86).opIMUL_Gv_Ev_Iv

	// 0x6A: PUSH Ib
	c.baseOps[0x6A] = (*CPU_X86).opPUSH_Ib

	// 0x6B: IMUL Gv,Ev,Ib
	c.baseOps[0x6B] = (*CPU_X86).opIMUL_Gv_Ev_Ib

	// 0x6C-0x6F: INS/OUTS
	c.baseOps[0x6C] = (*CPU_X86).opINSB
	c.baseOps[0x6D] = (*CPU_X86).opINSW
	c.baseOps[0x6E] = (*CPU_X86).opOUTSB
	c.baseOps[0x6F] = (*CPU_X86).opOUTSW

	// 0x70-0x7F: Jcc rel8
	c.baseOps[0x70] = (*CPU_X86).opJO_rel8
	c.baseOps[0x71] = (*CPU_X86).opJNO_rel8
	c.baseOps[0x72] = (*CPU_X86).opJB_rel8
	c.baseOps[0x73] = (*CPU_X86).opJNB_rel8
	c.baseOps[0x74] = (*CPU_X86).opJZ_rel8
	c.baseOps[0x75] = (*CPU_X86).opJNZ_rel8
	c.baseOps[0x76] = (*CPU_X86).opJBE_rel8
	c.baseOps[0x77] = (*CPU_X86).opJNBE_rel8
	c.baseOps[0x78] = (*CPU_X86).opJS_rel8
	c.baseOps[0x79] = (*CPU_X86).opJNS_rel8
	c.baseOps[0x7A] = (*CPU_X86).opJP_rel8
	c.baseOps[0x7B] = (*CPU_X86).opJNP_rel8
	c.baseOps[0x7C] = (*CPU_X86).opJL_rel8
	c.baseOps[0x7D] = (*CPU_X86).opJNL_rel8
	c.baseOps[0x7E] = (*CPU_X86).opJLE_rel8
	c.baseOps[0x7F] = (*CPU_X86).opJNLE_rel8

	// 0x80: Grp1 Eb,Ib
	c.baseOps[0x80] = (*CPU_X86).opGrp1_Eb_Ib

	// 0x81: Grp1 Ev,Iv
	c.baseOps[0x81] = (*CPU_X86).opGrp1_Ev_Iv

	// 0x82: Grp1 Eb,Ib (alias)
	c.baseOps[0x82] = (*CPU_X86).opGrp1_Eb_Ib

	// 0x83: Grp1 Ev,Ib
	c.baseOps[0x83] = (*CPU_X86).opGrp1_Ev_Ib

	// 0x84-0x85: TEST
	c.baseOps[0x84] = (*CPU_X86).opTEST_Eb_Gb
	c.baseOps[0x85] = (*CPU_X86).opTEST_Ev_Gv

	// 0x86-0x87: XCHG
	c.baseOps[0x86] = (*CPU_X86).opXCHG_Eb_Gb
	c.baseOps[0x87] = (*CPU_X86).opXCHG_Ev_Gv

	// 0x88-0x8B: MOV
	c.baseOps[0x88] = (*CPU_X86).opMOV_Eb_Gb
	c.baseOps[0x89] = (*CPU_X86).opMOV_Ev_Gv
	c.baseOps[0x8A] = (*CPU_X86).opMOV_Gb_Eb
	c.baseOps[0x8B] = (*CPU_X86).opMOV_Gv_Ev

	// 0x8C: MOV Ev,Sw
	c.baseOps[0x8C] = (*CPU_X86).opMOV_Ev_Sw

	// 0x8D: LEA
	c.baseOps[0x8D] = (*CPU_X86).opLEA

	// 0x8E: MOV Sw,Ew
	c.baseOps[0x8E] = (*CPU_X86).opMOV_Sw_Ew

	// 0x8F: POP Ev
	c.baseOps[0x8F] = (*CPU_X86).opPOP_Ev

	// 0x90: NOP (XCHG AX,AX)
	c.baseOps[0x90] = (*CPU_X86).opNOP

	// 0x91-0x97: XCHG AX,r16
	for i := 1; i < 8; i++ {
		idx := i
		c.baseOps[0x90+i] = func(cpu *CPU_X86) { cpu.opXCHG_AX_reg(byte(idx)) }
	}

	// 0x98: CBW/CWDE
	c.baseOps[0x98] = (*CPU_X86).opCBW

	// 0x99: CWD/CDQ
	c.baseOps[0x99] = (*CPU_X86).opCWD

	// 0x9A: CALL far
	c.baseOps[0x9A] = (*CPU_X86).opCALL_far

	// 0x9B: WAIT
	c.baseOps[0x9B] = (*CPU_X86).opWAIT

	// 0x9C: PUSHF
	c.baseOps[0x9C] = (*CPU_X86).opPUSHF

	// 0x9D: POPF
	c.baseOps[0x9D] = (*CPU_X86).opPOPF

	// 0x9E: SAHF
	c.baseOps[0x9E] = (*CPU_X86).opSAHF

	// 0x9F: LAHF
	c.baseOps[0x9F] = (*CPU_X86).opLAHF

	// 0xA0-0xA3: MOV AL/AX,moffs
	c.baseOps[0xA0] = (*CPU_X86).opMOV_AL_moffs
	c.baseOps[0xA1] = (*CPU_X86).opMOV_AX_moffs
	c.baseOps[0xA2] = (*CPU_X86).opMOV_moffs_AL
	c.baseOps[0xA3] = (*CPU_X86).opMOV_moffs_AX

	// 0xA4-0xA7: MOVS/CMPS
	c.baseOps[0xA4] = (*CPU_X86).opMOVSB
	c.baseOps[0xA5] = (*CPU_X86).opMOVSW
	c.baseOps[0xA6] = (*CPU_X86).opCMPSB
	c.baseOps[0xA7] = (*CPU_X86).opCMPSW

	// 0xA8-0xA9: TEST AL/AX,imm
	c.baseOps[0xA8] = (*CPU_X86).opTEST_AL_Ib
	c.baseOps[0xA9] = (*CPU_X86).opTEST_AX_Iv

	// 0xAA-0xAF: STOS/LODS/SCAS
	c.baseOps[0xAA] = (*CPU_X86).opSTOSB
	c.baseOps[0xAB] = (*CPU_X86).opSTOSW
	c.baseOps[0xAC] = (*CPU_X86).opLODSB
	c.baseOps[0xAD] = (*CPU_X86).opLODSW
	c.baseOps[0xAE] = (*CPU_X86).opSCASB
	c.baseOps[0xAF] = (*CPU_X86).opSCASW

	// 0xB0-0xB7: MOV r8,imm8
	for i := 0; i < 8; i++ {
		idx := i
		c.baseOps[0xB0+i] = func(cpu *CPU_X86) { cpu.opMOV_r8_imm8(byte(idx)) }
	}

	// 0xB8-0xBF: MOV r16/r32,imm16/imm32
	for i := 0; i < 8; i++ {
		idx := i
		c.baseOps[0xB8+i] = func(cpu *CPU_X86) { cpu.opMOV_r_imm(byte(idx)) }
	}

	// 0xC0: Grp2 Eb,Ib
	c.baseOps[0xC0] = (*CPU_X86).opGrp2_Eb_Ib

	// 0xC1: Grp2 Ev,Ib
	c.baseOps[0xC1] = (*CPU_X86).opGrp2_Ev_Ib

	// 0xC2: RET imm16
	c.baseOps[0xC2] = (*CPU_X86).opRET_imm16

	// 0xC3: RET
	c.baseOps[0xC3] = (*CPU_X86).opRET

	// 0xC4: LES
	c.baseOps[0xC4] = (*CPU_X86).opLES

	// 0xC5: LDS
	c.baseOps[0xC5] = (*CPU_X86).opLDS

	// 0xC6: MOV Eb,Ib
	c.baseOps[0xC6] = (*CPU_X86).opMOV_Eb_Ib

	// 0xC7: MOV Ev,Iv
	c.baseOps[0xC7] = (*CPU_X86).opMOV_Ev_Iv

	// 0xC8: ENTER
	c.baseOps[0xC8] = (*CPU_X86).opENTER

	// 0xC9: LEAVE
	c.baseOps[0xC9] = (*CPU_X86).opLEAVE

	// 0xCA: RETF imm16
	c.baseOps[0xCA] = (*CPU_X86).opRETF_imm16

	// 0xCB: RETF
	c.baseOps[0xCB] = (*CPU_X86).opRETF

	// 0xCC: INT 3
	c.baseOps[0xCC] = (*CPU_X86).opINT3

	// 0xCD: INT imm8
	c.baseOps[0xCD] = (*CPU_X86).opINT

	// 0xCE: INTO
	c.baseOps[0xCE] = (*CPU_X86).opINTO

	// 0xCF: IRET
	c.baseOps[0xCF] = (*CPU_X86).opIRET

	// 0xD0-0xD3: Grp2 shift/rotate
	c.baseOps[0xD0] = (*CPU_X86).opGrp2_Eb_1
	c.baseOps[0xD1] = (*CPU_X86).opGrp2_Ev_1
	c.baseOps[0xD2] = (*CPU_X86).opGrp2_Eb_CL
	c.baseOps[0xD3] = (*CPU_X86).opGrp2_Ev_CL

	// 0xD4: AAM
	c.baseOps[0xD4] = (*CPU_X86).opAAM

	// 0xD5: AAD
	c.baseOps[0xD5] = (*CPU_X86).opAAD

	// 0xD6: SALC (undocumented)
	c.baseOps[0xD6] = (*CPU_X86).opSALC

	// 0xD7: XLAT
	c.baseOps[0xD7] = (*CPU_X86).opXLAT

	// 0xD8-0xDF: FPU escape (NOP for now)
	for i := 0xD8; i <= 0xDF; i++ {
		c.baseOps[i] = (*CPU_X86).opFPU_escape
	}

	// 0xE0-0xE3: LOOP/JCXZ
	c.baseOps[0xE0] = (*CPU_X86).opLOOPNE
	c.baseOps[0xE1] = (*CPU_X86).opLOOPE
	c.baseOps[0xE2] = (*CPU_X86).opLOOP
	c.baseOps[0xE3] = (*CPU_X86).opJCXZ

	// 0xE4-0xE7: IN/OUT imm8
	c.baseOps[0xE4] = (*CPU_X86).opIN_AL_imm8
	c.baseOps[0xE5] = (*CPU_X86).opIN_AX_imm8
	c.baseOps[0xE6] = (*CPU_X86).opOUT_imm8_AL
	c.baseOps[0xE7] = (*CPU_X86).opOUT_imm8_AX

	// 0xE8: CALL rel16/rel32
	c.baseOps[0xE8] = (*CPU_X86).opCALL_rel

	// 0xE9: JMP rel16/rel32
	c.baseOps[0xE9] = (*CPU_X86).opJMP_rel

	// 0xEA: JMP far
	c.baseOps[0xEA] = (*CPU_X86).opJMP_far

	// 0xEB: JMP rel8
	c.baseOps[0xEB] = (*CPU_X86).opJMP_rel8

	// 0xEC-0xEF: IN/OUT DX
	c.baseOps[0xEC] = (*CPU_X86).opIN_AL_DX
	c.baseOps[0xED] = (*CPU_X86).opIN_AX_DX
	c.baseOps[0xEE] = (*CPU_X86).opOUT_DX_AL
	c.baseOps[0xEF] = (*CPU_X86).opOUT_DX_AX

	// 0xF4: HLT
	c.baseOps[0xF4] = (*CPU_X86).opHLT

	// 0xF5: CMC
	c.baseOps[0xF5] = (*CPU_X86).opCMC

	// 0xF6: Grp3 Eb
	c.baseOps[0xF6] = (*CPU_X86).opGrp3_Eb

	// 0xF7: Grp3 Ev
	c.baseOps[0xF7] = (*CPU_X86).opGrp3_Ev

	// 0xF8: CLC
	c.baseOps[0xF8] = (*CPU_X86).opCLC

	// 0xF9: STC
	c.baseOps[0xF9] = (*CPU_X86).opSTC

	// 0xFA: CLI
	c.baseOps[0xFA] = (*CPU_X86).opCLI

	// 0xFB: STI
	c.baseOps[0xFB] = (*CPU_X86).opSTI

	// 0xFC: CLD
	c.baseOps[0xFC] = (*CPU_X86).opCLD

	// 0xFD: STD
	c.baseOps[0xFD] = (*CPU_X86).opSTD

	// 0xFE: Grp4 Eb
	c.baseOps[0xFE] = (*CPU_X86).opGrp4_Eb

	// 0xFF: Grp5 Ev
	c.baseOps[0xFF] = (*CPU_X86).opGrp5_Ev
}

// initExtendedOps initializes the 0x0F prefixed opcode dispatch table
func (c *CPU_X86) initExtendedOps() {
	// Clear all entries first
	for i := range c.extendedOps {
		c.extendedOps[i] = nil
	}

	// 0x80-0x8F: Jcc rel16/rel32
	c.extendedOps[0x80] = (*CPU_X86).opJO_rel16
	c.extendedOps[0x81] = (*CPU_X86).opJNO_rel16
	c.extendedOps[0x82] = (*CPU_X86).opJB_rel16
	c.extendedOps[0x83] = (*CPU_X86).opJNB_rel16
	c.extendedOps[0x84] = (*CPU_X86).opJZ_rel16
	c.extendedOps[0x85] = (*CPU_X86).opJNZ_rel16
	c.extendedOps[0x86] = (*CPU_X86).opJBE_rel16
	c.extendedOps[0x87] = (*CPU_X86).opJNBE_rel16
	c.extendedOps[0x88] = (*CPU_X86).opJS_rel16
	c.extendedOps[0x89] = (*CPU_X86).opJNS_rel16
	c.extendedOps[0x8A] = (*CPU_X86).opJP_rel16
	c.extendedOps[0x8B] = (*CPU_X86).opJNP_rel16
	c.extendedOps[0x8C] = (*CPU_X86).opJL_rel16
	c.extendedOps[0x8D] = (*CPU_X86).opJNL_rel16
	c.extendedOps[0x8E] = (*CPU_X86).opJLE_rel16
	c.extendedOps[0x8F] = (*CPU_X86).opJNLE_rel16

	// 0x90-0x9F: SETcc
	c.extendedOps[0x90] = (*CPU_X86).opSETO
	c.extendedOps[0x91] = (*CPU_X86).opSETNO
	c.extendedOps[0x92] = (*CPU_X86).opSETB
	c.extendedOps[0x93] = (*CPU_X86).opSETNB
	c.extendedOps[0x94] = (*CPU_X86).opSETZ
	c.extendedOps[0x95] = (*CPU_X86).opSETNZ
	c.extendedOps[0x96] = (*CPU_X86).opSETBE
	c.extendedOps[0x97] = (*CPU_X86).opSETNBE
	c.extendedOps[0x98] = (*CPU_X86).opSETS
	c.extendedOps[0x99] = (*CPU_X86).opSETNS
	c.extendedOps[0x9A] = (*CPU_X86).opSETP
	c.extendedOps[0x9B] = (*CPU_X86).opSETNP
	c.extendedOps[0x9C] = (*CPU_X86).opSETL
	c.extendedOps[0x9D] = (*CPU_X86).opSETNL
	c.extendedOps[0x9E] = (*CPU_X86).opSETLE
	c.extendedOps[0x9F] = (*CPU_X86).opSETNLE

	// 0xA0-0xA1: PUSH/POP FS
	c.extendedOps[0xA0] = (*CPU_X86).opPUSH_FS
	c.extendedOps[0xA1] = (*CPU_X86).opPOP_FS

	// 0xA3: BT
	c.extendedOps[0xA3] = (*CPU_X86).opBT_Ev_Gv

	// 0xA4-0xA5: SHLD
	c.extendedOps[0xA4] = (*CPU_X86).opSHLD_Ev_Gv_Ib
	c.extendedOps[0xA5] = (*CPU_X86).opSHLD_Ev_Gv_CL

	// 0xA8-0xA9: PUSH/POP GS
	c.extendedOps[0xA8] = (*CPU_X86).opPUSH_GS
	c.extendedOps[0xA9] = (*CPU_X86).opPOP_GS

	// 0xAB: BTS
	c.extendedOps[0xAB] = (*CPU_X86).opBTS_Ev_Gv

	// 0xAC-0xAD: SHRD
	c.extendedOps[0xAC] = (*CPU_X86).opSHRD_Ev_Gv_Ib
	c.extendedOps[0xAD] = (*CPU_X86).opSHRD_Ev_Gv_CL

	// 0xAF: IMUL Gv,Ev
	c.extendedOps[0xAF] = (*CPU_X86).opIMUL_Gv_Ev

	// 0xB3: BTR
	c.extendedOps[0xB3] = (*CPU_X86).opBTR_Ev_Gv

	// 0xB6-0xB7: MOVZX
	c.extendedOps[0xB6] = (*CPU_X86).opMOVZX_Gv_Eb
	c.extendedOps[0xB7] = (*CPU_X86).opMOVZX_Gv_Ew

	// 0xBA: Grp8 (BT/BTS/BTR/BTC with immediate)
	c.extendedOps[0xBA] = (*CPU_X86).opGrp8_Ev_Ib

	// 0xBB: BTC
	c.extendedOps[0xBB] = (*CPU_X86).opBTC_Ev_Gv

	// 0xBC-0xBD: BSF/BSR
	c.extendedOps[0xBC] = (*CPU_X86).opBSF_Gv_Ev
	c.extendedOps[0xBD] = (*CPU_X86).opBSR_Gv_Ev

	// 0xBE-0xBF: MOVSX
	c.extendedOps[0xBE] = (*CPU_X86).opMOVSX_Gv_Eb
	c.extendedOps[0xBF] = (*CPU_X86).opMOVSX_Gv_Ew
}

// opTwoBytePrefix handles the 0x0F two-byte opcode prefix
func (c *CPU_X86) opTwoBytePrefix() {
	opcode := c.fetch8()
	if handler := c.extendedOps[opcode]; handler != nil {
		handler(c)
	} else {
		// Undefined extended opcode
		c.Halted = true
	}
}
