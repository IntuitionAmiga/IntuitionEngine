package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Z80Bus interface {
	Read(addr uint16) byte
	Write(addr uint16, value byte)
	In(port uint16) byte
	Out(port uint16, value byte)
	Tick(cycles int)
}

type CPU_Z80 struct {
	// Hot path registers (most frequently accessed)
	A  byte
	F  byte
	B  byte
	C  byte
	D  byte
	E  byte
	H  byte
	L  byte
	A2 byte
	F2 byte
	B2 byte
	C2 byte
	D2 byte
	E2 byte
	H2 byte
	L2 byte

	IX uint16
	IY uint16
	SP uint16
	PC uint16

	I  byte
	R  byte
	IM byte
	WZ uint16

	IFF1 bool
	IFF2 bool

	Halted  bool
	running atomic.Bool // Atomic for lock-free access (was: Running bool)
	Cycles  uint64

	irqLine    bool
	nmiLine    bool
	nmiPending bool
	nmiPrev    bool
	iffDelay   int
	irqVector  byte

	bus   Z80Bus
	mutex sync.RWMutex

	baseOps [256]func(*CPU_Z80)
	cbOps   [256]func(*CPU_Z80)
	ddOps   [256]func(*CPU_Z80)
	fdOps   [256]func(*CPU_Z80)
	edOps   [256]func(*CPU_Z80)

	prefixMode   byte
	prefixOpcode byte

	// Register pointer array for O(1) lookup (8-bit registers)
	regs8 [8]*byte // B, C, D, E, H, L, (HL), A - index matches Z80 encoding

	// Performance monitoring (matching IE32 pattern)
	PerfEnabled      bool      // Enable MIPS reporting
	InstructionCount uint64    // Total instructions executed
	perfStartTime    time.Time // When execution started
	lastPerfReport   time.Time // Last time we printed stats
}

// Running returns the execution state (thread-safe)
func (c *CPU_Z80) Running() bool {
	return c.running.Load()
}

// SetRunning sets the execution state (thread-safe)
func (c *CPU_Z80) SetRunning(state bool) {
	c.running.Store(state)
}

const (
	z80FlagS  = 0x80
	z80FlagZ  = 0x40
	z80FlagY  = 0x20
	z80FlagH  = 0x10
	z80FlagX  = 0x08
	z80FlagPV = 0x04
	z80FlagN  = 0x02
	z80FlagC  = 0x01
)

const (
	z80PrefixNone byte = iota
	z80PrefixDD
	z80PrefixFD
)

func NewCPU_Z80(bus Z80Bus) *CPU_Z80 {
	cpu := &CPU_Z80{
		bus: bus,
	}
	cpu.initBaseOps()
	cpu.initCBOps()
	cpu.initDDOps()
	cpu.initFDOps()
	cpu.initEDOps()
	cpu.Reset()
	return cpu
}

func (c *CPU_Z80) Reset() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.A = 0
	c.F = 0
	c.B = 0
	c.C = 0
	c.D = 0
	c.E = 0
	c.H = 0
	c.L = 0
	c.A2 = 0
	c.F2 = 0
	c.B2 = 0
	c.C2 = 0
	c.D2 = 0
	c.E2 = 0
	c.H2 = 0
	c.L2 = 0
	c.IX = 0
	c.IY = 0
	c.SP = 0xFFFF
	c.PC = 0
	c.I = 0
	c.R = 0
	c.IM = 0
	c.WZ = 0
	c.prefixMode = z80PrefixNone
	c.prefixOpcode = 0
	c.IFF1 = false
	c.IFF2 = false
	c.irqLine = false
	c.nmiLine = false
	c.nmiPending = false
	c.nmiPrev = false
	c.iffDelay = 0
	c.irqVector = 0xFF
	c.Halted = false
	c.running.Store(true)
	c.Cycles = 0

	// Initialize register pointer array for O(1) lookup
	// Index matches Z80 encoding: B=0, C=1, D=2, E=3, H=4, L=5, (HL)=6 (nil), A=7
	c.regs8 = [8]*byte{&c.B, &c.C, &c.D, &c.E, &c.H, &c.L, nil, &c.A}
}

func (c *CPU_Z80) AF() uint16 {
	return uint16(c.A)<<8 | uint16(c.F)
}

func (c *CPU_Z80) BC() uint16 {
	return uint16(c.B)<<8 | uint16(c.C)
}

func (c *CPU_Z80) DE() uint16 {
	return uint16(c.D)<<8 | uint16(c.E)
}

func (c *CPU_Z80) HL() uint16 {
	return uint16(c.H)<<8 | uint16(c.L)
}

func (c *CPU_Z80) AF2() uint16 {
	return uint16(c.A2)<<8 | uint16(c.F2)
}

func (c *CPU_Z80) BC2() uint16 {
	return uint16(c.B2)<<8 | uint16(c.C2)
}

func (c *CPU_Z80) DE2() uint16 {
	return uint16(c.D2)<<8 | uint16(c.E2)
}

func (c *CPU_Z80) HL2() uint16 {
	return uint16(c.H2)<<8 | uint16(c.L2)
}

func (c *CPU_Z80) SetAF(value uint16) {
	c.A = byte(value >> 8)
	c.F = byte(value)
}

func (c *CPU_Z80) SetBC(value uint16) {
	c.B = byte(value >> 8)
	c.C = byte(value)
}

func (c *CPU_Z80) SetDE(value uint16) {
	c.D = byte(value >> 8)
	c.E = byte(value)
}

func (c *CPU_Z80) SetHL(value uint16) {
	c.H = byte(value >> 8)
	c.L = byte(value)
}

func (c *CPU_Z80) SetAF2(value uint16) {
	c.A2 = byte(value >> 8)
	c.F2 = byte(value)
}

func (c *CPU_Z80) SetBC2(value uint16) {
	c.B2 = byte(value >> 8)
	c.C2 = byte(value)
}

func (c *CPU_Z80) SetDE2(value uint16) {
	c.D2 = byte(value >> 8)
	c.E2 = byte(value)
}

func (c *CPU_Z80) SetHL2(value uint16) {
	c.H2 = byte(value >> 8)
	c.L2 = byte(value)
}

func (c *CPU_Z80) Flag(mask byte) bool {
	return c.F&mask != 0
}

func (c *CPU_Z80) SetFlag(mask byte, on bool) {
	if on {
		c.F |= mask
	} else {
		c.F &^= mask
	}
}

func (c *CPU_Z80) ExAF() {
	c.A, c.A2 = c.A2, c.A
	c.F, c.F2 = c.F2, c.F
}

func (c *CPU_Z80) Exx() {
	c.B, c.B2 = c.B2, c.B
	c.C, c.C2 = c.C2, c.C
	c.D, c.D2 = c.D2, c.D
	c.E, c.E2 = c.E2, c.E
	c.H, c.H2 = c.H2, c.H
	c.L, c.L2 = c.L2, c.L
}

func (c *CPU_Z80) Step() {
	// Lock only for state that could be modified externally
	c.mutex.Lock()

	if !c.running.Load() {
		c.mutex.Unlock()
		return
	}

	if c.nmiLine && !c.nmiPrev {
		c.nmiPending = true
	}
	c.nmiPrev = c.nmiLine

	if c.nmiPending {
		c.serviceNMI()
		c.mutex.Unlock()
		return
	}

	if c.irqLine && c.IFF1 {
		c.serviceIRQ()
		c.mutex.Unlock()
		return
	}

	if c.Halted {
		c.tick(4)
		c.mutex.Unlock()
		return
	}

	// Release mutex before instruction execution (bus calls may need it)
	c.mutex.Unlock()

	opcode := c.fetchOpcode()
	c.baseOps[opcode](c)
	c.finishInstruction()
}

func (c *CPU_Z80) Execute() {
	// Initialize perf counters if enabled
	if c.PerfEnabled {
		c.perfStartTime = time.Now()
		c.lastPerfReport = c.perfStartTime
		c.InstructionCount = 0
	}

	for c.running.Load() {
		c.Step()

		// Performance monitoring (matching IE32 pattern)
		if c.PerfEnabled {
			c.InstructionCount++
			if c.InstructionCount&0xFFFFFF == 0 { // Every ~16M instructions
				now := time.Now()
				if now.Sub(c.lastPerfReport) >= time.Second {
					elapsed := now.Sub(c.perfStartTime).Seconds()
					ips := float64(c.InstructionCount) / elapsed
					mips := ips / 1_000_000
					fmt.Printf("Z80: %.2f MIPS (%.0f instructions in %.1fs)\n", mips, float64(c.InstructionCount), elapsed)
					c.lastPerfReport = now
				}
			}
		}
	}
}

func (c *CPU_Z80) SetIRQLine(assert bool) {
	c.mutex.Lock()
	c.irqLine = assert
	c.mutex.Unlock()
}

func (c *CPU_Z80) SetNMILine(assert bool) {
	c.mutex.Lock()
	c.nmiLine = assert
	c.mutex.Unlock()
}

func (c *CPU_Z80) SetIRQVector(vector byte) {
	c.mutex.Lock()
	c.irqVector = vector
	c.mutex.Unlock()
}

func (c *CPU_Z80) incrementR() {
	c.R = (c.R & 0x80) | ((c.R + 1) & 0x7F)
}

func (c *CPU_Z80) fetchOpcode() byte {
	opcode := c.read(c.PC)
	c.PC++
	c.incrementR()
	return opcode
}

func (c *CPU_Z80) fetchByte() byte {
	value := c.read(c.PC)
	c.PC++
	return value
}

func (c *CPU_Z80) read(addr uint16) byte {
	return c.bus.Read(addr)
}

func (c *CPU_Z80) write(addr uint16, value byte) {
	c.bus.Write(addr, value)
}

func (c *CPU_Z80) in(port uint16) byte {
	return c.bus.In(port)
}

func (c *CPU_Z80) out(port uint16, value byte) {
	c.bus.Out(port, value)
}

func (c *CPU_Z80) tick(cycles int) {
	c.Cycles += uint64(cycles)
	c.bus.Tick(cycles)
}

func (c *CPU_Z80) finishInstruction() {
	if c.iffDelay > 0 {
		c.iffDelay--
		if c.iffDelay == 0 {
			c.IFF1 = true
			c.IFF2 = true
		}
	}
}

func (c *CPU_Z80) readReg8(code byte) byte {
	switch code {
	case 0:
		return c.B
	case 1:
		return c.C
	case 2:
		return c.D
	case 3:
		return c.E
	case 4:
		return c.readIndexHigh()
	case 5:
		return c.readIndexLow()
	case 6:
		return c.read(c.HL())
	case 7:
		return c.A
	default:
		return 0
	}
}

func (c *CPU_Z80) writeReg8(code byte, value byte) {
	switch code {
	case 0:
		c.B = value
	case 1:
		c.C = value
	case 2:
		c.D = value
	case 3:
		c.E = value
	case 4:
		c.writeIndexHigh(value)
	case 5:
		c.writeIndexLow(value)
	case 6:
		c.write(c.HL(), value)
	case 7:
		c.A = value
	}
}

func (c *CPU_Z80) readReg8Plain(code byte) byte {
	switch code {
	case 0:
		return c.B
	case 1:
		return c.C
	case 2:
		return c.D
	case 3:
		return c.E
	case 4:
		return c.H
	case 5:
		return c.L
	case 6:
		return c.read(c.HL())
	case 7:
		return c.A
	default:
		return 0
	}
}

func (c *CPU_Z80) writeReg8Plain(code byte, value byte) {
	switch code {
	case 0:
		c.B = value
	case 1:
		c.C = value
	case 2:
		c.D = value
	case 3:
		c.E = value
	case 4:
		c.H = value
	case 5:
		c.L = value
	case 6:
		c.write(c.HL(), value)
	case 7:
		c.A = value
	}
}

func (c *CPU_Z80) readIndexHigh() byte {
	switch c.prefixMode {
	case z80PrefixDD:
		return byte(c.IX >> 8)
	case z80PrefixFD:
		return byte(c.IY >> 8)
	default:
		return c.H
	}
}

func (c *CPU_Z80) readIndexLow() byte {
	switch c.prefixMode {
	case z80PrefixDD:
		return byte(c.IX)
	case z80PrefixFD:
		return byte(c.IY)
	default:
		return c.L
	}
}

func (c *CPU_Z80) writeIndexHigh(value byte) {
	switch c.prefixMode {
	case z80PrefixDD:
		c.IX = (c.IX & 0x00FF) | uint16(value)<<8
	case z80PrefixFD:
		c.IY = (c.IY & 0x00FF) | uint16(value)<<8
	default:
		c.H = value
	}
}

func (c *CPU_Z80) writeIndexLow(value byte) {
	switch c.prefixMode {
	case z80PrefixDD:
		c.IX = (c.IX & 0xFF00) | uint16(value)
	case z80PrefixFD:
		c.IY = (c.IY & 0xFF00) | uint16(value)
	default:
		c.L = value
	}
}

func (c *CPU_Z80) initBaseOps() {
	for i := range c.baseOps {
		c.baseOps[i] = (*CPU_Z80).opUnimplemented
	}

	c.baseOps[0x00] = (*CPU_Z80).opNOP
	c.baseOps[0x76] = (*CPU_Z80).opHALT

	for opcode := 0x40; opcode <= 0x7F; opcode++ {
		if opcode == 0x76 {
			continue
		}
		op := opcode
		dest := byte((op >> 3) & 0x07)
		src := byte(op & 0x07)
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opLDRegReg(dest, src)
		}
	}

	ldRegImmOpcodes := map[byte]byte{
		0x06: 0,
		0x0E: 1,
		0x16: 2,
		0x1E: 3,
		0x26: 4,
		0x2E: 5,
		0x36: 6,
		0x3E: 7,
	}
	for opcode, reg := range ldRegImmOpcodes {
		op := opcode
		dest := reg
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opLDRegImm(dest)
		}
	}

	for opcode := 0x80; opcode <= 0x87; opcode++ {
		op := opcode
		src := byte(op & 0x07)
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUReg(aluAdd, src)
		}
	}
	for opcode := 0x88; opcode <= 0x8F; opcode++ {
		op := opcode
		src := byte(op & 0x07)
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUReg(aluAdc, src)
		}
	}
	for opcode := 0x90; opcode <= 0x97; opcode++ {
		op := opcode
		src := byte(op & 0x07)
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUReg(aluSub, src)
		}
	}
	for opcode := 0x98; opcode <= 0x9F; opcode++ {
		op := opcode
		src := byte(op & 0x07)
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUReg(aluSbc, src)
		}
	}
	for opcode := 0xA0; opcode <= 0xA7; opcode++ {
		op := opcode
		src := byte(op & 0x07)
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUReg(aluAnd, src)
		}
	}
	for opcode := 0xA8; opcode <= 0xAF; opcode++ {
		op := opcode
		src := byte(op & 0x07)
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUReg(aluXor, src)
		}
	}
	for opcode := 0xB0; opcode <= 0xB7; opcode++ {
		op := opcode
		src := byte(op & 0x07)
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUReg(aluOr, src)
		}
	}
	for opcode := 0xB8; opcode <= 0xBF; opcode++ {
		op := opcode
		src := byte(op & 0x07)
		c.baseOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUReg(aluCp, src)
		}
	}

	c.baseOps[0xC6] = (*CPU_Z80).opADDImm
	c.baseOps[0xCE] = (*CPU_Z80).opADCImm
	c.baseOps[0xD6] = (*CPU_Z80).opSUBImm
	c.baseOps[0xDE] = (*CPU_Z80).opSBCImm
	c.baseOps[0xE6] = (*CPU_Z80).opANDImm
	c.baseOps[0xEE] = (*CPU_Z80).opXORImm
	c.baseOps[0xF6] = (*CPU_Z80).opORImm
	c.baseOps[0xFE] = (*CPU_Z80).opCPImm

	c.baseOps[0x27] = (*CPU_Z80).opDAA
	c.baseOps[0x2F] = (*CPU_Z80).opCPL
	c.baseOps[0x37] = (*CPU_Z80).opSCF
	c.baseOps[0x3F] = (*CPU_Z80).opCCF

	c.baseOps[0x01] = (*CPU_Z80).opLDBCNN
	c.baseOps[0x11] = (*CPU_Z80).opLDDENN
	c.baseOps[0x21] = (*CPU_Z80).opLDHLImm
	c.baseOps[0x31] = (*CPU_Z80).opLDSPNN
	c.baseOps[0x09] = (*CPU_Z80).opADDHLBC
	c.baseOps[0x19] = (*CPU_Z80).opADDHLDE
	c.baseOps[0x29] = (*CPU_Z80).opADDHLHL
	c.baseOps[0x39] = (*CPU_Z80).opADDHLSP
	c.baseOps[0x03] = (*CPU_Z80).opINCBC
	c.baseOps[0x13] = (*CPU_Z80).opINCDE
	c.baseOps[0x23] = (*CPU_Z80).opINCHL
	c.baseOps[0x33] = (*CPU_Z80).opINCSP
	c.baseOps[0x0B] = (*CPU_Z80).opDECBC
	c.baseOps[0x1B] = (*CPU_Z80).opDECDE
	c.baseOps[0x2B] = (*CPU_Z80).opDECHL
	c.baseOps[0x3B] = (*CPU_Z80).opDECSP
	c.baseOps[0xC5] = (*CPU_Z80).opPUSHBC
	c.baseOps[0xD5] = (*CPU_Z80).opPUSHDE
	c.baseOps[0xE5] = (*CPU_Z80).opPUSHLH
	c.baseOps[0xF5] = (*CPU_Z80).opPUSHAF
	c.baseOps[0xC1] = (*CPU_Z80).opPOPBC
	c.baseOps[0xD1] = (*CPU_Z80).opPOPDE
	c.baseOps[0xE1] = (*CPU_Z80).opPOPHL
	c.baseOps[0xF1] = (*CPU_Z80).opPOPAF
	c.baseOps[0xC3] = (*CPU_Z80).opJPNN
	c.baseOps[0x18] = (*CPU_Z80).opJR
	c.baseOps[0x10] = (*CPU_Z80).opDJNZ
	c.baseOps[0xCD] = (*CPU_Z80).opCALLNN
	c.baseOps[0xC9] = (*CPU_Z80).opRET
	c.baseOps[0xE3] = (*CPU_Z80).opEXSPHL
	c.baseOps[0x08] = (*CPU_Z80).opEXAF
	c.baseOps[0xEB] = (*CPU_Z80).opEXDEHL
	c.baseOps[0xD9] = (*CPU_Z80).opEXX
	c.baseOps[0xE9] = (*CPU_Z80).opJPHL
	c.baseOps[0x22] = (*CPU_Z80).opLDNNHL
	c.baseOps[0x2A] = (*CPU_Z80).opLDHLNN
	c.baseOps[0x32] = (*CPU_Z80).opLDNNA
	c.baseOps[0x3A] = (*CPU_Z80).opLDANN
	c.baseOps[0x02] = (*CPU_Z80).opLDBCA
	c.baseOps[0x0A] = (*CPU_Z80).opLDABC
	c.baseOps[0x12] = (*CPU_Z80).opLDDEA
	c.baseOps[0x1A] = (*CPU_Z80).opLDABD
	c.baseOps[0xF9] = (*CPU_Z80).opLDSPHL
	c.baseOps[0xD3] = (*CPU_Z80).opOUTNA
	c.baseOps[0xDB] = (*CPU_Z80).opINAN
	c.baseOps[0x07] = (*CPU_Z80).opRLCA
	c.baseOps[0x0F] = (*CPU_Z80).opRRCA
	c.baseOps[0x17] = (*CPU_Z80).opRLA
	c.baseOps[0x1F] = (*CPU_Z80).opRRA
	c.baseOps[0xC7] = (*CPU_Z80).opRST00
	c.baseOps[0xCF] = (*CPU_Z80).opRST08
	c.baseOps[0xD7] = (*CPU_Z80).opRST10
	c.baseOps[0xDF] = (*CPU_Z80).opRST18
	c.baseOps[0xE7] = (*CPU_Z80).opRST20
	c.baseOps[0xEF] = (*CPU_Z80).opRST28
	c.baseOps[0xF7] = (*CPU_Z80).opRST30
	c.baseOps[0xFF] = (*CPU_Z80).opRST38
	c.baseOps[0x04] = (*CPU_Z80).opINCB
	c.baseOps[0x0C] = (*CPU_Z80).opINCC
	c.baseOps[0x14] = (*CPU_Z80).opINCD
	c.baseOps[0x1C] = (*CPU_Z80).opINCE
	c.baseOps[0x24] = (*CPU_Z80).opINCH
	c.baseOps[0x2C] = (*CPU_Z80).opINCL
	c.baseOps[0x34] = (*CPU_Z80).opINCHLMem
	c.baseOps[0x3C] = (*CPU_Z80).opINCA
	c.baseOps[0x05] = (*CPU_Z80).opDECB
	c.baseOps[0x0D] = (*CPU_Z80).opDECC
	c.baseOps[0x15] = (*CPU_Z80).opDECD
	c.baseOps[0x1D] = (*CPU_Z80).opDECE
	c.baseOps[0x25] = (*CPU_Z80).opDECH
	c.baseOps[0x2D] = (*CPU_Z80).opDECL
	c.baseOps[0x35] = (*CPU_Z80).opDECHLMem
	c.baseOps[0x3D] = (*CPU_Z80).opDECA
	c.baseOps[0xC2] = (*CPU_Z80).opJPNZ
	c.baseOps[0xCA] = (*CPU_Z80).opJPZ
	c.baseOps[0xD2] = (*CPU_Z80).opJPNC
	c.baseOps[0xDA] = (*CPU_Z80).opJPC
	c.baseOps[0xE2] = (*CPU_Z80).opJPPO
	c.baseOps[0xEA] = (*CPU_Z80).opJPPE
	c.baseOps[0xF2] = (*CPU_Z80).opJPNS
	c.baseOps[0xFA] = (*CPU_Z80).opJPS
	c.baseOps[0x20] = (*CPU_Z80).opJRNZ
	c.baseOps[0x28] = (*CPU_Z80).opJRZ
	c.baseOps[0x30] = (*CPU_Z80).opJRNC
	c.baseOps[0x38] = (*CPU_Z80).opJRC
	c.baseOps[0xC4] = (*CPU_Z80).opCALLNZ
	c.baseOps[0xCC] = (*CPU_Z80).opCALLZ
	c.baseOps[0xD4] = (*CPU_Z80).opCALLNC
	c.baseOps[0xDC] = (*CPU_Z80).opCALLC
	c.baseOps[0xE4] = (*CPU_Z80).opCALLPO
	c.baseOps[0xEC] = (*CPU_Z80).opCALLPE
	c.baseOps[0xF4] = (*CPU_Z80).opCALLNS
	c.baseOps[0xFC] = (*CPU_Z80).opCALLS
	c.baseOps[0xC0] = (*CPU_Z80).opRETNZ
	c.baseOps[0xC8] = (*CPU_Z80).opRETZ
	c.baseOps[0xD0] = (*CPU_Z80).opRETNC
	c.baseOps[0xD8] = (*CPU_Z80).opRETC
	c.baseOps[0xE0] = (*CPU_Z80).opRETPO
	c.baseOps[0xE8] = (*CPU_Z80).opRETPE
	c.baseOps[0xF0] = (*CPU_Z80).opRETNS
	c.baseOps[0xF8] = (*CPU_Z80).opRETS
	c.baseOps[0xCB] = (*CPU_Z80).opCBPrefix
	c.baseOps[0xDD] = (*CPU_Z80).opDDPrefix
	c.baseOps[0xFD] = (*CPU_Z80).opFDPrefix
	c.baseOps[0xED] = (*CPU_Z80).opEDPrefix
	c.baseOps[0xF3] = (*CPU_Z80).opDI
	c.baseOps[0xFB] = (*CPU_Z80).opEI
}

func (c *CPU_Z80) opUnimplemented() {
	c.tick(4)
}

func (c *CPU_Z80) opNOP() {
	c.tick(4)
}

func (c *CPU_Z80) opHALT() {
	c.Halted = true
	c.tick(4)
}

func (c *CPU_Z80) opLDRegReg(dest, src byte) {
	value := c.readReg8(src)
	c.writeReg8(dest, value)
	if dest == 6 || src == 6 {
		c.tick(7)
	} else {
		c.tick(4)
	}
}

func (c *CPU_Z80) opLDRegImm(dest byte) {
	value := c.fetchByte()
	c.writeReg8(dest, value)
	if dest == 6 {
		c.tick(10)
	} else {
		c.tick(7)
	}
}

type aluOp byte

const (
	aluAdd aluOp = iota
	aluAdc
	aluSub
	aluSbc
	aluAnd
	aluXor
	aluOr
	aluCp
)

func (c *CPU_Z80) opALUReg(op aluOp, src byte) {
	value := c.readReg8(src)
	c.performALU(op, value)
	if src == 6 {
		c.tick(7)
	} else {
		c.tick(4)
	}
}

func (c *CPU_Z80) opADDImm() {
	value := c.fetchByte()
	c.performALU(aluAdd, value)
	c.tick(7)
}

func (c *CPU_Z80) opADCImm() {
	value := c.fetchByte()
	c.performALU(aluAdc, value)
	c.tick(7)
}

func (c *CPU_Z80) opSUBImm() {
	value := c.fetchByte()
	c.performALU(aluSub, value)
	c.tick(7)
}

func (c *CPU_Z80) opSBCImm() {
	value := c.fetchByte()
	c.performALU(aluSbc, value)
	c.tick(7)
}

func (c *CPU_Z80) opANDImm() {
	value := c.fetchByte()
	c.performALU(aluAnd, value)
	c.tick(7)
}

func (c *CPU_Z80) opXORImm() {
	value := c.fetchByte()
	c.performALU(aluXor, value)
	c.tick(7)
}

func (c *CPU_Z80) opORImm() {
	value := c.fetchByte()
	c.performALU(aluOr, value)
	c.tick(7)
}

func (c *CPU_Z80) opCPImm() {
	value := c.fetchByte()
	c.performALU(aluCp, value)
	c.tick(7)
}

func (c *CPU_Z80) opDAA() {
	a := c.A
	adj := byte(0)
	carry := c.Flag(z80FlagC)
	if c.Flag(z80FlagH) || (!c.Flag(z80FlagN) && (a&0x0F) > 0x09) {
		adj |= 0x06
	}
	if carry || (!c.Flag(z80FlagN) && a > 0x99) {
		adj |= 0x60
	}

	var res byte
	if c.Flag(z80FlagN) {
		res = a - adj
	} else {
		res = a + adj
	}

	c.A = res
	c.F &^= z80FlagS | z80FlagZ | z80FlagPV | z80FlagH | z80FlagC | z80FlagX | z80FlagY
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x80 != 0 {
		c.F |= z80FlagS
	}
	if parity8(res) {
		c.F |= z80FlagPV
	}
	if c.Flag(z80FlagN) {
		if (a^res)&0x10 != 0 {
			c.F |= z80FlagH
		}
	} else if (a&0x0F)+byte(adj&0x0F) > 0x0F {
		c.F |= z80FlagH
	}
	if adj >= 0x60 {
		c.F |= z80FlagC
	}
	c.F |= res & (z80FlagX | z80FlagY)
	c.tick(4)
}

func (c *CPU_Z80) opCPL() {
	c.A = ^c.A
	c.F = (c.F & (z80FlagS | z80FlagZ | z80FlagPV | z80FlagC)) | z80FlagH | z80FlagN
	c.F |= c.A & (z80FlagX | z80FlagY)
	c.tick(4)
}

func (c *CPU_Z80) opSCF() {
	c.F = (c.F & (z80FlagS | z80FlagZ | z80FlagPV)) | z80FlagC
	c.F |= c.A & (z80FlagX | z80FlagY)
	c.tick(4)
}

func (c *CPU_Z80) opCCF() {
	carry := c.Flag(z80FlagC)
	c.F = (c.F & (z80FlagS | z80FlagZ | z80FlagPV)) | (c.A & (z80FlagX | z80FlagY))
	if carry {
		c.F |= z80FlagH
	} else {
		c.F |= z80FlagC
	}
	c.tick(4)
}

func (c *CPU_Z80) opLDBCNN() {
	c.SetBC(c.fetchWord())
	c.tick(10)
}

func (c *CPU_Z80) opLDDENN() {
	c.SetDE(c.fetchWord())
	c.tick(10)
}

func (c *CPU_Z80) opLDHLImm() {
	c.SetHL(c.fetchWord())
	c.tick(10)
}

func (c *CPU_Z80) opLDSPNN() {
	c.SP = c.fetchWord()
	c.tick(10)
}

func (c *CPU_Z80) opADDHLBC() {
	c.addHL(c.BC())
	c.tick(11)
}

func (c *CPU_Z80) opADDHLDE() {
	c.addHL(c.DE())
	c.tick(11)
}

func (c *CPU_Z80) opADDHLHL() {
	c.addHL(c.HL())
	c.tick(11)
}

func (c *CPU_Z80) opADDHLSP() {
	c.addHL(c.SP)
	c.tick(11)
}

func (c *CPU_Z80) opINCBC() {
	c.SetBC(c.BC() + 1)
	c.tick(6)
}

func (c *CPU_Z80) opINCDE() {
	c.SetDE(c.DE() + 1)
	c.tick(6)
}

func (c *CPU_Z80) opINCHL() {
	c.SetHL(c.HL() + 1)
	c.tick(6)
}

func (c *CPU_Z80) opINCSP() {
	c.SP++
	c.tick(6)
}

func (c *CPU_Z80) opDECBC() {
	c.SetBC(c.BC() - 1)
	c.tick(6)
}

func (c *CPU_Z80) opDECDE() {
	c.SetDE(c.DE() - 1)
	c.tick(6)
}

func (c *CPU_Z80) opDECHL() {
	c.SetHL(c.HL() - 1)
	c.tick(6)
}

func (c *CPU_Z80) opDECSP() {
	c.SP--
	c.tick(6)
}

func (c *CPU_Z80) opPUSHBC() {
	c.pushWord(c.BC())
	c.tick(11)
}

func (c *CPU_Z80) opPUSHDE() {
	c.pushWord(c.DE())
	c.tick(11)
}

func (c *CPU_Z80) opPUSHLH() {
	c.pushWord(c.HL())
	c.tick(11)
}

func (c *CPU_Z80) opPUSHAF() {
	c.pushWord(c.AF())
	c.tick(11)
}

func (c *CPU_Z80) opPOPBC() {
	c.SetBC(c.popWord())
	c.tick(10)
}

func (c *CPU_Z80) opPOPDE() {
	c.SetDE(c.popWord())
	c.tick(10)
}

func (c *CPU_Z80) opPOPHL() {
	c.SetHL(c.popWord())
	c.tick(10)
}

func (c *CPU_Z80) opPOPAF() {
	c.SetAF(c.popWord())
	c.tick(10)
}

func (c *CPU_Z80) opJPNN() {
	c.PC = c.fetchWord()
	c.tick(10)
}

func (c *CPU_Z80) opJR() {
	disp := int8(c.fetchByte())
	c.PC = uint16(int32(c.PC) + int32(disp))
	c.tick(12)
}

func (c *CPU_Z80) opDJNZ() {
	disp := int8(c.fetchByte())
	c.B--
	if c.B != 0 {
		c.PC = uint16(int32(c.PC) + int32(disp))
		c.tick(13)
	} else {
		c.tick(8)
	}
}

func (c *CPU_Z80) opCALLNN() {
	addr := c.fetchWord()
	c.pushWord(c.PC)
	c.PC = addr
	c.tick(17)
}

func (c *CPU_Z80) opRET() {
	c.PC = c.popWord()
	c.tick(10)
}

func (c *CPU_Z80) opEXSPHL() {
	low := c.read(c.SP)
	high := c.read(c.SP + 1)
	memVal := uint16(high)<<8 | uint16(low)
	hl := c.HL()
	c.write(c.SP, byte(hl))
	c.write(c.SP+1, byte(hl>>8))
	c.SetHL(memVal)
	c.WZ = memVal
	c.tick(19)
}

func (c *CPU_Z80) opEXAF() {
	c.ExAF()
	c.tick(4)
}

func (c *CPU_Z80) opEXDEHL() {
	c.D, c.H = c.H, c.D
	c.E, c.L = c.L, c.E
	c.tick(4)
}

func (c *CPU_Z80) opEXX() {
	c.Exx()
	c.tick(4)
}

func (c *CPU_Z80) opJPHL() {
	c.PC = c.HL()
	c.WZ = c.PC
	c.tick(4)
}

func (c *CPU_Z80) opLDNNHL() {
	addr := c.fetchWord()
	value := c.HL()
	c.write(addr, byte(value))
	c.write(addr+1, byte(value>>8))
	c.WZ = addr + 1
	c.tick(16)
}

func (c *CPU_Z80) opLDHLNN() {
	addr := c.fetchWord()
	low := c.read(addr)
	high := c.read(addr + 1)
	c.SetHL(uint16(high)<<8 | uint16(low))
	c.WZ = addr + 1
	c.tick(16)
}

func (c *CPU_Z80) opLDNNA() {
	addr := c.fetchWord()
	c.write(addr, c.A)
	c.WZ = addr
	c.tick(13)
}

func (c *CPU_Z80) opLDANN() {
	addr := c.fetchWord()
	c.A = c.read(addr)
	c.WZ = addr
	c.tick(13)
}

func (c *CPU_Z80) opLDBCA() {
	c.write(c.BC(), c.A)
	c.tick(7)
}

func (c *CPU_Z80) opLDABC() {
	c.A = c.read(c.BC())
	c.tick(7)
}

func (c *CPU_Z80) opLDDEA() {
	c.write(c.DE(), c.A)
	c.tick(7)
}

func (c *CPU_Z80) opLDABD() {
	c.A = c.read(c.DE())
	c.tick(7)
}

func (c *CPU_Z80) opLDSPHL() {
	c.SP = c.HL()
	c.tick(6)
}

func (c *CPU_Z80) opOUTNA() {
	port := uint16(c.A)<<8 | uint16(c.fetchByte())
	c.out(port, c.A)
	c.tick(11)
}

func (c *CPU_Z80) opINAN() {
	port := uint16(c.A)<<8 | uint16(c.fetchByte())
	c.A = c.in(port)
	c.updateInFlags(c.A)
	c.tick(11)
}

func (c *CPU_Z80) opRLCA() {
	carry := c.A&0x80 != 0
	c.A = c.A<<1 | c.A>>7
	c.updateRotateFlags(carry)
	c.tick(4)
}

func (c *CPU_Z80) opRRCA() {
	carry := c.A&0x01 != 0
	c.A = c.A>>1 | c.A<<7
	c.updateRotateFlags(carry)
	c.tick(4)
}

func (c *CPU_Z80) opRLA() {
	carryIn := c.Flag(z80FlagC)
	carryOut := c.A&0x80 != 0
	c.A = c.A << 1
	if carryIn {
		c.A |= 0x01
	}
	c.updateRotateFlags(carryOut)
	c.tick(4)
}

func (c *CPU_Z80) opRRA() {
	carryIn := c.Flag(z80FlagC)
	carryOut := c.A&0x01 != 0
	c.A = c.A >> 1
	if carryIn {
		c.A |= 0x80
	}
	c.updateRotateFlags(carryOut)
	c.tick(4)
}

func (c *CPU_Z80) opRST00() {
	c.opRST(0x00)
}

func (c *CPU_Z80) opRST08() {
	c.opRST(0x08)
}

func (c *CPU_Z80) opRST10() {
	c.opRST(0x10)
}

func (c *CPU_Z80) opRST18() {
	c.opRST(0x18)
}

func (c *CPU_Z80) opRST20() {
	c.opRST(0x20)
}

func (c *CPU_Z80) opRST28() {
	c.opRST(0x28)
}

func (c *CPU_Z80) opRST30() {
	c.opRST(0x30)
}

func (c *CPU_Z80) opRST38() {
	c.opRST(0x38)
}

func (c *CPU_Z80) opRST(vector uint16) {
	c.pushWord(c.PC)
	c.PC = vector
	c.tick(11)
}

func (c *CPU_Z80) opCBPrefix() {
	opcode := c.fetchOpcode()
	c.cbOps[opcode](c)
}

func (c *CPU_Z80) opDDPrefix() {
	opcode := c.fetchOpcode()
	prev := c.prefixMode
	c.prefixMode = z80PrefixDD
	c.prefixOpcode = opcode
	c.ddOps[opcode](c)
	c.prefixMode = prev
}

func (c *CPU_Z80) opFDPrefix() {
	opcode := c.fetchOpcode()
	prev := c.prefixMode
	c.prefixMode = z80PrefixFD
	c.prefixOpcode = opcode
	c.fdOps[opcode](c)
	c.prefixMode = prev
}

func (c *CPU_Z80) opEDPrefix() {
	opcode := c.fetchOpcode()
	c.edOps[opcode](c)
}

func (c *CPU_Z80) serviceNMI() {
	c.nmiPending = false
	c.Halted = false
	c.incrementR()
	c.pushWord(c.PC)
	c.IFF1 = false
	c.PC = 0x0066
	c.tick(11)
}

func (c *CPU_Z80) serviceIRQ() {
	c.Halted = false
	c.incrementR()
	c.IFF1 = false
	c.IFF2 = false
	switch c.IM {
	case 0:
		c.pushWord(c.PC)
		c.PC = c.im0Vector()
		c.WZ = c.PC
		c.tick(13)
	case 2:
		vector := uint16(c.I)<<8 | uint16(c.irqVector)
		low := c.read(vector)
		high := c.read(vector + 1)
		c.pushWord(c.PC)
		c.PC = uint16(high)<<8 | uint16(low)
		c.WZ = vector + 1
		c.tick(19)
	default:
		c.pushWord(c.PC)
		c.PC = 0x0038
		c.WZ = c.PC
		c.tick(13)
	}
}

func (c *CPU_Z80) im0Vector() uint16 {
	vector := c.irqVector
	if vector&0xC7 == 0xC7 {
		return uint16(vector & 0x38)
	}
	return 0x0038
}

func (c *CPU_Z80) opINCB() {
	c.B = c.inc8(c.B)
	c.tick(4)
}

func (c *CPU_Z80) opINCC() {
	c.C = c.inc8(c.C)
	c.tick(4)
}

func (c *CPU_Z80) opINCD() {
	c.D = c.inc8(c.D)
	c.tick(4)
}

func (c *CPU_Z80) opINCE() {
	c.E = c.inc8(c.E)
	c.tick(4)
}

func (c *CPU_Z80) opINCH() {
	c.writeReg8(4, c.inc8(c.readReg8(4)))
	c.tick(4)
}

func (c *CPU_Z80) opINCL() {
	c.writeReg8(5, c.inc8(c.readReg8(5)))
	c.tick(4)
}

func (c *CPU_Z80) opINCHLMem() {
	addr := c.HL()
	value := c.read(addr)
	value = c.inc8(value)
	c.write(addr, value)
	c.tick(11)
}

func (c *CPU_Z80) opINCA() {
	c.A = c.inc8(c.A)
	c.tick(4)
}

func (c *CPU_Z80) opDECB() {
	c.B = c.dec8(c.B)
	c.tick(4)
}

func (c *CPU_Z80) opDECC() {
	c.C = c.dec8(c.C)
	c.tick(4)
}

func (c *CPU_Z80) opDECD() {
	c.D = c.dec8(c.D)
	c.tick(4)
}

func (c *CPU_Z80) opDECE() {
	c.E = c.dec8(c.E)
	c.tick(4)
}

func (c *CPU_Z80) opDECH() {
	c.writeReg8(4, c.dec8(c.readReg8(4)))
	c.tick(4)
}

func (c *CPU_Z80) opDECL() {
	c.writeReg8(5, c.dec8(c.readReg8(5)))
	c.tick(4)
}

func (c *CPU_Z80) opDECHLMem() {
	addr := c.HL()
	value := c.read(addr)
	value = c.dec8(value)
	c.write(addr, value)
	c.tick(11)
}

func (c *CPU_Z80) opDECA() {
	c.A = c.dec8(c.A)
	c.tick(4)
}

func (c *CPU_Z80) opDI() {
	c.IFF1 = false
	c.IFF2 = false
	c.iffDelay = 0
	c.tick(4)
}

func (c *CPU_Z80) opEI() {
	c.iffDelay = 2
	c.tick(4)
}

func (c *CPU_Z80) opJPNZ() {
	c.jpCond(!c.Flag(z80FlagZ))
}

func (c *CPU_Z80) opJPZ() {
	c.jpCond(c.Flag(z80FlagZ))
}

func (c *CPU_Z80) opJPNC() {
	c.jpCond(!c.Flag(z80FlagC))
}

func (c *CPU_Z80) opJPC() {
	c.jpCond(c.Flag(z80FlagC))
}

func (c *CPU_Z80) opJPPO() {
	c.jpCond(!c.Flag(z80FlagPV))
}

func (c *CPU_Z80) opJPPE() {
	c.jpCond(c.Flag(z80FlagPV))
}

func (c *CPU_Z80) opJPNS() {
	c.jpCond(!c.Flag(z80FlagS))
}

func (c *CPU_Z80) opJPS() {
	c.jpCond(c.Flag(z80FlagS))
}

func (c *CPU_Z80) opJRNZ() {
	c.jrCond(!c.Flag(z80FlagZ))
}

func (c *CPU_Z80) opJRZ() {
	c.jrCond(c.Flag(z80FlagZ))
}

func (c *CPU_Z80) opJRNC() {
	c.jrCond(!c.Flag(z80FlagC))
}

func (c *CPU_Z80) opJRC() {
	c.jrCond(c.Flag(z80FlagC))
}

func (c *CPU_Z80) opCALLNZ() {
	c.callCond(!c.Flag(z80FlagZ))
}

func (c *CPU_Z80) opCALLZ() {
	c.callCond(c.Flag(z80FlagZ))
}

func (c *CPU_Z80) opCALLNC() {
	c.callCond(!c.Flag(z80FlagC))
}

func (c *CPU_Z80) opCALLC() {
	c.callCond(c.Flag(z80FlagC))
}

func (c *CPU_Z80) opCALLPO() {
	c.callCond(!c.Flag(z80FlagPV))
}

func (c *CPU_Z80) opCALLPE() {
	c.callCond(c.Flag(z80FlagPV))
}

func (c *CPU_Z80) opCALLNS() {
	c.callCond(!c.Flag(z80FlagS))
}

func (c *CPU_Z80) opCALLS() {
	c.callCond(c.Flag(z80FlagS))
}

func (c *CPU_Z80) opRETNZ() {
	c.retCond(!c.Flag(z80FlagZ))
}

func (c *CPU_Z80) opRETZ() {
	c.retCond(c.Flag(z80FlagZ))
}

func (c *CPU_Z80) opRETNC() {
	c.retCond(!c.Flag(z80FlagC))
}

func (c *CPU_Z80) opRETC() {
	c.retCond(c.Flag(z80FlagC))
}

func (c *CPU_Z80) opRETPO() {
	c.retCond(!c.Flag(z80FlagPV))
}

func (c *CPU_Z80) opRETPE() {
	c.retCond(c.Flag(z80FlagPV))
}

func (c *CPU_Z80) opRETNS() {
	c.retCond(!c.Flag(z80FlagS))
}

func (c *CPU_Z80) opRETS() {
	c.retCond(c.Flag(z80FlagS))
}

func (c *CPU_Z80) addHL(value uint16) {
	hl := c.HL()
	sum := uint32(hl) + uint32(value)

	c.F &^= z80FlagH | z80FlagN | z80FlagC | z80FlagX | z80FlagY
	if ((hl&0x0FFF)+(value&0x0FFF))&0x1000 != 0 {
		c.F |= z80FlagH
	}
	if sum > 0xFFFF {
		c.F |= z80FlagC
	}
	result := uint16(sum)
	c.SetHL(result)
	c.F |= byte((result >> 8) & 0x28)
}

func (c *CPU_Z80) addIX(value uint16) {
	sum := uint32(c.IX) + uint32(value)
	c.F &^= z80FlagH | z80FlagN | z80FlagC | z80FlagX | z80FlagY
	if ((c.IX&0x0FFF)+(value&0x0FFF))&0x1000 != 0 {
		c.F |= z80FlagH
	}
	if sum > 0xFFFF {
		c.F |= z80FlagC
	}
	c.IX = uint16(sum)
	c.F |= byte((c.IX >> 8) & 0x28)
}

func (c *CPU_Z80) addIY(value uint16) {
	sum := uint32(c.IY) + uint32(value)
	c.F &^= z80FlagH | z80FlagN | z80FlagC | z80FlagX | z80FlagY
	if ((c.IY&0x0FFF)+(value&0x0FFF))&0x1000 != 0 {
		c.F |= z80FlagH
	}
	if sum > 0xFFFF {
		c.F |= z80FlagC
	}
	c.IY = uint16(sum)
	c.F |= byte((c.IY >> 8) & 0x28)
}

func (c *CPU_Z80) adcHL(value uint16) {
	hl := c.HL()
	carry := uint16(0)
	if c.Flag(z80FlagC) {
		carry = 1
	}
	sum := uint32(hl) + uint32(value) + uint32(carry)
	res := uint16(sum)

	c.F = 0
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x8000 != 0 {
		c.F |= z80FlagS
	}
	if ((hl&0x0FFF)+(value&0x0FFF)+carry)&0x1000 != 0 {
		c.F |= z80FlagH
	}
	if ((^(hl ^ value))&(hl^res))&0x8000 != 0 {
		c.F |= z80FlagPV
	}
	if sum > 0xFFFF {
		c.F |= z80FlagC
	}
	c.F |= byte((res >> 8) & 0x28)
	c.SetHL(res)
}

func (c *CPU_Z80) sbcHL(value uint16) {
	hl := c.HL()
	carry := uint16(0)
	if c.Flag(z80FlagC) {
		carry = 1
	}
	diff := int32(hl) - int32(value) - int32(carry)
	res := uint16(diff)

	c.F = z80FlagN
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x8000 != 0 {
		c.F |= z80FlagS
	}
	if int32(hl&0x0FFF)-int32(value&0x0FFF)-int32(carry) < 0 {
		c.F |= z80FlagH
	}
	if ((hl ^ value) & (hl ^ res) & 0x8000) != 0 {
		c.F |= z80FlagPV
	}
	if diff < 0 {
		c.F |= z80FlagC
	}
	c.F |= byte((res >> 8) & 0x28)
	c.SetHL(res)
}

func (c *CPU_Z80) inc8(value byte) byte {
	res := value + 1
	c.F = (c.F & z80FlagC)
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x80 != 0 {
		c.F |= z80FlagS
	}
	if (value&0x0F)+1 > 0x0F {
		c.F |= z80FlagH
	}
	if value == 0x7F {
		c.F |= z80FlagPV
	}
	c.F |= res & (z80FlagX | z80FlagY)
	return res
}

func (c *CPU_Z80) dec8(value byte) byte {
	res := value - 1
	c.F = (c.F & z80FlagC) | z80FlagN
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x80 != 0 {
		c.F |= z80FlagS
	}
	if value&0x0F == 0 {
		c.F |= z80FlagH
	}
	if value == 0x80 {
		c.F |= z80FlagPV
	}
	c.F |= res & (z80FlagX | z80FlagY)
	return res
}

func (c *CPU_Z80) updateInFlags(value byte) {
	carry := c.F & z80FlagC
	c.F = carry
	c.setSZPFlags(value)
}

func (c *CPU_Z80) updateAParityFlagsPreserveCarry() {
	carry := c.F & z80FlagC
	value := c.A
	c.F = carry
	if value == 0 {
		c.F |= z80FlagZ
	}
	if value&0x80 != 0 {
		c.F |= z80FlagS
	}
	if parity8(value) {
		c.F |= z80FlagPV
	}
	c.F |= value & (z80FlagX | z80FlagY)
}

func (c *CPU_Z80) updateLDAIRFlags() {
	carry := c.F & z80FlagC
	value := c.A
	c.F = carry
	if value == 0 {
		c.F |= z80FlagZ
	}
	if value&0x80 != 0 {
		c.F |= z80FlagS
	}
	if c.IFF2 {
		c.F |= z80FlagPV
	}
	c.F |= value & (z80FlagX | z80FlagY)
}

func (c *CPU_Z80) updateLDIFlags(value byte, bc uint16) {
	sum := c.A + value
	c.F = c.F & (z80FlagS | z80FlagZ | z80FlagC)
	if bc != 0 {
		c.F |= z80FlagPV
	}
	c.F |= sum & (z80FlagX | z80FlagY)
}

func (c *CPU_Z80) updateBlockIOFlags() {
	keep := c.F & (z80FlagS | z80FlagH | z80FlagPV | z80FlagC | z80FlagX | z80FlagY)
	c.F = keep | z80FlagN
	if c.B == 0 {
		c.F |= z80FlagZ
	}
}

func (c *CPU_Z80) updateRotateFlags(carry bool) {
	f := c.F & (z80FlagS | z80FlagZ | z80FlagPV)
	if carry {
		f |= z80FlagC
	}
	f |= c.A & (z80FlagX | z80FlagY)
	c.F = f
}

func (c *CPU_Z80) rotate8Left(value byte, carryIn bool) (byte, bool) {
	newCarry := value&0x80 != 0
	res := value << 1
	if carryIn {
		res |= 0x01
	}
	return res, newCarry
}

func (c *CPU_Z80) rotate8Right(value byte, carryIn bool) (byte, bool) {
	newCarry := value&0x01 != 0
	res := value >> 1
	if carryIn {
		res |= 0x80
	}
	return res, newCarry
}

func (c *CPU_Z80) shiftLeftArithmetic(value byte) (byte, bool) {
	newCarry := value&0x80 != 0
	res := value << 1
	return res, newCarry
}

func (c *CPU_Z80) shiftRightArithmetic(value byte) (byte, bool) {
	newCarry := value&0x01 != 0
	res := (value >> 1) | (value & 0x80)
	return res, newCarry
}

func (c *CPU_Z80) shiftRightLogical(value byte) (byte, bool) {
	newCarry := value&0x01 != 0
	res := value >> 1
	return res, newCarry
}

func (c *CPU_Z80) setSZPFlags(value byte) {
	c.F &^= z80FlagS | z80FlagZ | z80FlagPV | z80FlagX | z80FlagY
	if value == 0 {
		c.F |= z80FlagZ
	}
	if value&0x80 != 0 {
		c.F |= z80FlagS
	}
	if parity8(value) {
		c.F |= z80FlagPV
	}
	c.F |= value & (z80FlagX | z80FlagY)
}

func (c *CPU_Z80) initCBOps() {
	for i := range c.cbOps {
		c.cbOps[i] = (*CPU_Z80).opUnimplemented
	}

	for opcode := 0x00; opcode <= 0x3F; opcode++ {
		op := byte(opcode)
		group := op >> 3
		reg := op & 0x07
		c.cbOps[op] = func(cpu *CPU_Z80) {
			cpu.opCBRotateShift(group, reg)
		}
	}

	for opcode := 0x40; opcode <= 0x7F; opcode++ {
		op := byte(opcode)
		bit := (op >> 3) & 0x07
		reg := op & 0x07
		c.cbOps[op] = func(cpu *CPU_Z80) {
			cpu.opCBBIT(bit, reg)
		}
	}

	for opcode := 0x80; opcode <= 0xBF; opcode++ {
		op := byte(opcode)
		bit := (op >> 3) & 0x07
		reg := op & 0x07
		c.cbOps[op] = func(cpu *CPU_Z80) {
			cpu.opCBRES(bit, reg)
		}
	}

	for opcode := 0xC0; opcode <= 0xFF; opcode++ {
		op := byte(opcode)
		bit := (op >> 3) & 0x07
		reg := op & 0x07
		c.cbOps[op] = func(cpu *CPU_Z80) {
			cpu.opCBSET(bit, reg)
		}
	}
}

func (c *CPU_Z80) initDDOps() {
	for i := range c.ddOps {
		c.ddOps[i] = (*CPU_Z80).opDDUnimplemented
	}
	c.ddOps[0x21] = (*CPU_Z80).opLDIXNN
	c.ddOps[0x22] = (*CPU_Z80).opLDNNIX
	c.ddOps[0x2A] = (*CPU_Z80).opLDIXNNMem
	c.ddOps[0xE5] = (*CPU_Z80).opPUSHIX
	c.ddOps[0xE1] = (*CPU_Z80).opPOPIX
	c.ddOps[0xF9] = (*CPU_Z80).opLDSPX
	c.ddOps[0x36] = (*CPU_Z80).opLDIXdN
	c.ddOps[0x34] = (*CPU_Z80).opINCIXd
	c.ddOps[0x35] = (*CPU_Z80).opDECIXd
	c.ddOps[0xE9] = (*CPU_Z80).opJPIX
	c.ddOps[0xCB] = (*CPU_Z80).opDDCBPrefix
	c.ddOps[0xE3] = (*CPU_Z80).opEXSPIX
	c.ddOps[0x09] = (*CPU_Z80).opADDIXBC
	c.ddOps[0x19] = (*CPU_Z80).opADDIXDE
	c.ddOps[0x29] = (*CPU_Z80).opADDIXIX
	c.ddOps[0x39] = (*CPU_Z80).opADDIXSP
	c.ddOps[0x23] = (*CPU_Z80).opINCIX
	c.ddOps[0x2B] = (*CPU_Z80).opDECIX

	for opcode := byte(0x46); opcode <= 0x7E; opcode += 0x08 {
		if opcode == 0x76 {
			continue
		}
		op := opcode
		dest := byte((op >> 3) & 0x07)
		c.ddOps[op] = func(cpu *CPU_Z80) {
			cpu.opLDRegIXd(dest)
		}
	}
	for opcode := byte(0x70); opcode <= 0x77; opcode++ {
		if opcode == 0x76 {
			continue
		}
		op := opcode
		src := byte(op & 0x07)
		c.ddOps[op] = func(cpu *CPU_Z80) {
			cpu.opLDIXdReg(src)
		}
	}
	for opcode := byte(0x86); opcode <= 0xBE; opcode += 0x08 {
		op := opcode
		alu := aluOp((op >> 3) & 0x07)
		c.ddOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUIXd(alu)
		}
	}
}

func (c *CPU_Z80) initFDOps() {
	for i := range c.fdOps {
		c.fdOps[i] = (*CPU_Z80).opFDUnimplemented
	}
	c.fdOps[0x21] = (*CPU_Z80).opLDIYNN
	c.fdOps[0x22] = (*CPU_Z80).opLDNNIY
	c.fdOps[0x2A] = (*CPU_Z80).opLDIYNNMem
	c.fdOps[0xE5] = (*CPU_Z80).opPUSHIY
	c.fdOps[0xE1] = (*CPU_Z80).opPOPIY
	c.fdOps[0xF9] = (*CPU_Z80).opLDSPY
	c.fdOps[0x36] = (*CPU_Z80).opLDIYdN
	c.fdOps[0x34] = (*CPU_Z80).opINCIYd
	c.fdOps[0x35] = (*CPU_Z80).opDECIYd
	c.fdOps[0xE9] = (*CPU_Z80).opJPIY
	c.fdOps[0xCB] = (*CPU_Z80).opFDCBPrefix
	c.fdOps[0xE3] = (*CPU_Z80).opEXSPIY
	c.fdOps[0x09] = (*CPU_Z80).opADDIYBC
	c.fdOps[0x19] = (*CPU_Z80).opADDIYDE
	c.fdOps[0x29] = (*CPU_Z80).opADDIYIY
	c.fdOps[0x39] = (*CPU_Z80).opADDIYSP
	c.fdOps[0x23] = (*CPU_Z80).opINCIY
	c.fdOps[0x2B] = (*CPU_Z80).opDECIY

	for opcode := byte(0x46); opcode <= 0x7E; opcode += 0x08 {
		if opcode == 0x76 {
			continue
		}
		op := opcode
		dest := byte((op >> 3) & 0x07)
		c.fdOps[op] = func(cpu *CPU_Z80) {
			cpu.opLDRegIYd(dest)
		}
	}
	for opcode := byte(0x70); opcode <= 0x77; opcode++ {
		if opcode == 0x76 {
			continue
		}
		op := opcode
		src := byte(op & 0x07)
		c.fdOps[op] = func(cpu *CPU_Z80) {
			cpu.opLDIYdReg(src)
		}
	}
	for opcode := byte(0x86); opcode <= 0xBE; opcode += 0x08 {
		op := opcode
		alu := aluOp((op >> 3) & 0x07)
		c.fdOps[op] = func(cpu *CPU_Z80) {
			cpu.opALUIYd(alu)
		}
	}
}

func (c *CPU_Z80) initEDOps() {
	for i := range c.edOps {
		c.edOps[i] = (*CPU_Z80).opEDUnimplemented
	}

	c.edOps[0x40] = (*CPU_Z80).opINBC
	c.edOps[0x48] = (*CPU_Z80).opINRC
	c.edOps[0x50] = (*CPU_Z80).opINDC
	c.edOps[0x58] = (*CPU_Z80).opINEC
	c.edOps[0x60] = (*CPU_Z80).opINHC
	c.edOps[0x68] = (*CPU_Z80).opINLC
	c.edOps[0x70] = (*CPU_Z80).opINCM
	c.edOps[0x78] = (*CPU_Z80).opINAC

	c.edOps[0x41] = (*CPU_Z80).opOUTBC
	c.edOps[0x49] = (*CPU_Z80).opOUTCC
	c.edOps[0x51] = (*CPU_Z80).opOUTDC
	c.edOps[0x59] = (*CPU_Z80).opOUTEC
	c.edOps[0x61] = (*CPU_Z80).opOUTHC
	c.edOps[0x69] = (*CPU_Z80).opOUTLC
	c.edOps[0x71] = (*CPU_Z80).opOUTC0
	c.edOps[0x79] = (*CPU_Z80).opOUTAC

	c.edOps[0x44] = (*CPU_Z80).opNEG
	c.edOps[0x4C] = (*CPU_Z80).opNEG
	c.edOps[0x54] = (*CPU_Z80).opNEG
	c.edOps[0x5C] = (*CPU_Z80).opNEG
	c.edOps[0x64] = (*CPU_Z80).opNEG
	c.edOps[0x6C] = (*CPU_Z80).opNEG
	c.edOps[0x74] = (*CPU_Z80).opNEG
	c.edOps[0x7C] = (*CPU_Z80).opNEG

	c.edOps[0x47] = (*CPU_Z80).opLDIA
	c.edOps[0x4F] = (*CPU_Z80).opLDRA
	c.edOps[0x57] = (*CPU_Z80).opLDAI
	c.edOps[0x5F] = (*CPU_Z80).opLDAR

	c.edOps[0x46] = (*CPU_Z80).opIM0
	c.edOps[0x56] = (*CPU_Z80).opIM1
	c.edOps[0x5E] = (*CPU_Z80).opIM2
	c.edOps[0x66] = (*CPU_Z80).opIM0
	c.edOps[0x6E] = (*CPU_Z80).opIM0
	c.edOps[0x76] = (*CPU_Z80).opIM1
	c.edOps[0x7E] = (*CPU_Z80).opIM2

	c.edOps[0x45] = (*CPU_Z80).opRETN
	c.edOps[0x4D] = (*CPU_Z80).opRETI
	c.edOps[0x55] = (*CPU_Z80).opRETN
	c.edOps[0x5D] = (*CPU_Z80).opRETN
	c.edOps[0x65] = (*CPU_Z80).opRETN
	c.edOps[0x6D] = (*CPU_Z80).opRETN
	c.edOps[0x75] = (*CPU_Z80).opRETN
	c.edOps[0x7D] = (*CPU_Z80).opRETN

	c.edOps[0x67] = (*CPU_Z80).opRRD
	c.edOps[0x6F] = (*CPU_Z80).opRLD

	c.edOps[0xA0] = (*CPU_Z80).opLDI
	c.edOps[0xB0] = (*CPU_Z80).opLDIR
	c.edOps[0xA8] = (*CPU_Z80).opLDD
	c.edOps[0xB8] = (*CPU_Z80).opLDDR
	c.edOps[0xA1] = (*CPU_Z80).opCPI
	c.edOps[0xB1] = (*CPU_Z80).opCPIR
	c.edOps[0xA9] = (*CPU_Z80).opCPD
	c.edOps[0xB9] = (*CPU_Z80).opCPDR
	c.edOps[0xA2] = (*CPU_Z80).opINI
	c.edOps[0xB2] = (*CPU_Z80).opINIR
	c.edOps[0xAA] = (*CPU_Z80).opIND
	c.edOps[0xBA] = (*CPU_Z80).opINDR
	c.edOps[0xA3] = (*CPU_Z80).opOUTI
	c.edOps[0xB3] = (*CPU_Z80).opOTIR
	c.edOps[0xAB] = (*CPU_Z80).opOUTD
	c.edOps[0xBB] = (*CPU_Z80).opOTDR

	c.edOps[0x43] = (*CPU_Z80).opLDNNBC
	c.edOps[0x4B] = (*CPU_Z80).opLDBCNNED
	c.edOps[0x53] = (*CPU_Z80).opLDNNDE
	c.edOps[0x5B] = (*CPU_Z80).opLDDENNED
	c.edOps[0x63] = (*CPU_Z80).opLDNNHLed
	c.edOps[0x6B] = (*CPU_Z80).opLDHLNNed
	c.edOps[0x73] = (*CPU_Z80).opLDNNSP
	c.edOps[0x7B] = (*CPU_Z80).opLDSPNNED

	c.edOps[0x4A] = (*CPU_Z80).opADCHLBC
	c.edOps[0x5A] = (*CPU_Z80).opADCHLDE
	c.edOps[0x6A] = (*CPU_Z80).opADCHLHL
	c.edOps[0x7A] = (*CPU_Z80).opADCHLSP
	c.edOps[0x42] = (*CPU_Z80).opSBCHLBC
	c.edOps[0x52] = (*CPU_Z80).opSBCHLDE
	c.edOps[0x62] = (*CPU_Z80).opSBCHLHL
	c.edOps[0x72] = (*CPU_Z80).opSBCHLSP
}

func (c *CPU_Z80) opEDUnimplemented() {
	c.tick(8)
}

func (c *CPU_Z80) opDDUnimplemented() {
	c.tick(4)
	c.baseOps[c.prefixOpcode](c)
}

func (c *CPU_Z80) opFDUnimplemented() {
	c.tick(4)
	c.baseOps[c.prefixOpcode](c)
}

func (c *CPU_Z80) opLDIXNN() {
	c.IX = c.fetchWord()
	c.tick(14)
}

func (c *CPU_Z80) opLDNNIX() {
	addr := c.fetchWord()
	c.write(addr, byte(c.IX))
	c.write(addr+1, byte(c.IX>>8))
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opLDIXNNMem() {
	addr := c.fetchWord()
	low := c.read(addr)
	high := c.read(addr + 1)
	c.IX = uint16(high)<<8 | uint16(low)
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opPUSHIX() {
	c.pushWord(c.IX)
	c.tick(15)
}

func (c *CPU_Z80) opPOPIX() {
	c.IX = c.popWord()
	c.tick(14)
}

func (c *CPU_Z80) opLDSPX() {
	c.SP = c.IX
	c.tick(10)
}

func (c *CPU_Z80) opLDIXdN() {
	disp := int8(c.fetchByte())
	value := c.fetchByte()
	addr := uint16(int32(c.IX) + int32(disp))
	c.write(addr, value)
	c.tick(19)
}

func (c *CPU_Z80) opINCIXd() {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IX) + int32(disp))
	value := c.read(addr)
	value = c.inc8(value)
	c.write(addr, value)
	c.tick(23)
}

func (c *CPU_Z80) opDECIXd() {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IX) + int32(disp))
	value := c.read(addr)
	value = c.dec8(value)
	c.write(addr, value)
	c.tick(23)
}

func (c *CPU_Z80) opJPIX() {
	c.PC = c.IX
	c.WZ = c.PC
	c.tick(8)
}

func (c *CPU_Z80) opEXSPIX() {
	low := c.read(c.SP)
	high := c.read(c.SP + 1)
	memVal := uint16(high)<<8 | uint16(low)
	c.write(c.SP, byte(c.IX))
	c.write(c.SP+1, byte(c.IX>>8))
	c.IX = memVal
	c.WZ = memVal
	c.tick(23)
}

func (c *CPU_Z80) opADDIXBC() {
	c.addIX(c.BC())
	c.tick(15)
}

func (c *CPU_Z80) opADDIXDE() {
	c.addIX(c.DE())
	c.tick(15)
}

func (c *CPU_Z80) opADDIXIX() {
	c.addIX(c.IX)
	c.tick(15)
}

func (c *CPU_Z80) opADDIXSP() {
	c.addIX(c.SP)
	c.tick(15)
}

func (c *CPU_Z80) opINCIX() {
	c.IX++
	c.tick(10)
}

func (c *CPU_Z80) opDECIX() {
	c.IX--
	c.tick(10)
}

func (c *CPU_Z80) opLDIYNN() {
	c.IY = c.fetchWord()
	c.tick(14)
}

func (c *CPU_Z80) opLDNNIY() {
	addr := c.fetchWord()
	c.write(addr, byte(c.IY))
	c.write(addr+1, byte(c.IY>>8))
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opLDIYNNMem() {
	addr := c.fetchWord()
	low := c.read(addr)
	high := c.read(addr + 1)
	c.IY = uint16(high)<<8 | uint16(low)
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opPUSHIY() {
	c.pushWord(c.IY)
	c.tick(15)
}

func (c *CPU_Z80) opPOPIY() {
	c.IY = c.popWord()
	c.tick(14)
}

func (c *CPU_Z80) opLDSPY() {
	c.SP = c.IY
	c.tick(10)
}

func (c *CPU_Z80) opLDIYdN() {
	disp := int8(c.fetchByte())
	value := c.fetchByte()
	addr := uint16(int32(c.IY) + int32(disp))
	c.write(addr, value)
	c.tick(19)
}

func (c *CPU_Z80) opINCIYd() {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IY) + int32(disp))
	value := c.read(addr)
	value = c.inc8(value)
	c.write(addr, value)
	c.tick(23)
}

func (c *CPU_Z80) opDECIYd() {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IY) + int32(disp))
	value := c.read(addr)
	value = c.dec8(value)
	c.write(addr, value)
	c.tick(23)
}

func (c *CPU_Z80) opJPIY() {
	c.PC = c.IY
	c.WZ = c.PC
	c.tick(8)
}

func (c *CPU_Z80) opEXSPIY() {
	low := c.read(c.SP)
	high := c.read(c.SP + 1)
	memVal := uint16(high)<<8 | uint16(low)
	c.write(c.SP, byte(c.IY))
	c.write(c.SP+1, byte(c.IY>>8))
	c.IY = memVal
	c.WZ = memVal
	c.tick(23)
}

func (c *CPU_Z80) opADDIYBC() {
	c.addIY(c.BC())
	c.tick(15)
}

func (c *CPU_Z80) opADDIYDE() {
	c.addIY(c.DE())
	c.tick(15)
}

func (c *CPU_Z80) opADDIYIY() {
	c.addIY(c.IY)
	c.tick(15)
}

func (c *CPU_Z80) opADDIYSP() {
	c.addIY(c.SP)
	c.tick(15)
}

func (c *CPU_Z80) opINCIY() {
	c.IY++
	c.tick(10)
}

func (c *CPU_Z80) opDECIY() {
	c.IY--
	c.tick(10)
}

func (c *CPU_Z80) opLDRegIXd(dest byte) {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IX) + int32(disp))
	c.writeReg8Plain(dest, c.read(addr))
	c.tick(19)
}

func (c *CPU_Z80) opLDIXdReg(src byte) {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IX) + int32(disp))
	c.write(addr, c.readReg8Plain(src))
	c.tick(19)
}

func (c *CPU_Z80) opALUIXd(op aluOp) {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IX) + int32(disp))
	c.performALU(op, c.read(addr))
	c.tick(19)
}

func (c *CPU_Z80) opLDRegIYd(dest byte) {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IY) + int32(disp))
	c.writeReg8Plain(dest, c.read(addr))
	c.tick(19)
}

func (c *CPU_Z80) opLDIYdReg(src byte) {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IY) + int32(disp))
	c.write(addr, c.readReg8Plain(src))
	c.tick(19)
}

func (c *CPU_Z80) opALUIYd(op aluOp) {
	disp := int8(c.fetchByte())
	addr := uint16(int32(c.IY) + int32(disp))
	c.performALU(op, c.read(addr))
	c.tick(19)
}

func (c *CPU_Z80) inRegC(dest *byte) {
	value := c.in(c.BC())
	*dest = value
	c.updateInFlags(value)
	c.tick(12)
}

func (c *CPU_Z80) outRegC(value byte) {
	c.out(c.BC(), value)
	c.tick(12)
}

func (c *CPU_Z80) opINBC() {
	c.inRegC(&c.B)
}

func (c *CPU_Z80) opINRC() {
	c.inRegC(&c.C)
}

func (c *CPU_Z80) opINDC() {
	c.inRegC(&c.D)
}

func (c *CPU_Z80) opINEC() {
	c.inRegC(&c.E)
}

func (c *CPU_Z80) opINHC() {
	c.inRegC(&c.H)
}

func (c *CPU_Z80) opINLC() {
	c.inRegC(&c.L)
}

func (c *CPU_Z80) opINAC() {
	c.inRegC(&c.A)
}

func (c *CPU_Z80) opINCM() {
	value := c.in(c.BC())
	c.updateInFlags(value)
	c.tick(12)
}

func (c *CPU_Z80) opOUTBC() {
	c.outRegC(c.B)
}

func (c *CPU_Z80) opOUTCC() {
	c.outRegC(c.C)
}

func (c *CPU_Z80) opOUTDC() {
	c.outRegC(c.D)
}

func (c *CPU_Z80) opOUTEC() {
	c.outRegC(c.E)
}

func (c *CPU_Z80) opOUTHC() {
	c.outRegC(c.H)
}

func (c *CPU_Z80) opOUTLC() {
	c.outRegC(c.L)
}

func (c *CPU_Z80) opOUTAC() {
	c.outRegC(c.A)
}

func (c *CPU_Z80) opOUTC0() {
	c.outRegC(0x00)
}

func (c *CPU_Z80) opNEG() {
	a := c.A
	res := byte(0 - int(a))
	c.A = res
	c.F = z80FlagN
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x80 != 0 {
		c.F |= z80FlagS
	}
	if a&0x0F != 0 {
		c.F |= z80FlagH
	}
	if a == 0x80 {
		c.F |= z80FlagPV
	}
	if a != 0 {
		c.F |= z80FlagC
	}
	c.F |= res & (z80FlagX | z80FlagY)
	c.tick(8)
}

func (c *CPU_Z80) opLDIA() {
	c.I = c.A
	c.tick(9)
}

func (c *CPU_Z80) opLDRA() {
	c.R = c.A
	c.tick(9)
}

func (c *CPU_Z80) opLDAI() {
	c.A = c.I
	c.updateLDAIRFlags()
	c.tick(9)
}

func (c *CPU_Z80) opLDAR() {
	c.A = c.R
	c.updateLDAIRFlags()
	c.tick(9)
}

func (c *CPU_Z80) opIM0() {
	c.IM = 0
	c.tick(8)
}

func (c *CPU_Z80) opIM1() {
	c.IM = 1
	c.tick(8)
}

func (c *CPU_Z80) opIM2() {
	c.IM = 2
	c.tick(8)
}

func (c *CPU_Z80) opRETN() {
	c.PC = c.popWord()
	c.IFF1 = c.IFF2
	c.tick(14)
}

func (c *CPU_Z80) opRETI() {
	c.PC = c.popWord()
	c.IFF1 = c.IFF2
	c.tick(14)
}

func (c *CPU_Z80) opRRD() {
	addr := c.HL()
	value := c.read(addr)
	c.write(addr, (value>>4)|(c.A<<4))
	c.A = (c.A & 0xF0) | (value & 0x0F)
	c.updateAParityFlagsPreserveCarry()
	c.tick(18)
}

func (c *CPU_Z80) opRLD() {
	addr := c.HL()
	value := c.read(addr)
	c.write(addr, (value<<4)|(c.A&0x0F))
	c.A = (c.A & 0xF0) | (value >> 4)
	c.updateAParityFlagsPreserveCarry()
	c.tick(18)
}

func (c *CPU_Z80) opLDI() {
	value := c.read(c.HL())
	c.write(c.DE(), value)
	c.SetHL(c.HL() + 1)
	c.SetDE(c.DE() + 1)
	bc := c.BC() - 1
	c.SetBC(bc)
	c.updateLDIFlags(value, bc)
	c.tick(16)
}

func (c *CPU_Z80) opLDIR() {
	c.opLDI()
	if c.BC() != 0 {
		c.PC -= 2
		c.tick(5)
	}
}

func (c *CPU_Z80) opLDD() {
	value := c.read(c.HL())
	c.write(c.DE(), value)
	c.SetHL(c.HL() - 1)
	c.SetDE(c.DE() - 1)
	bc := c.BC() - 1
	c.SetBC(bc)
	c.updateLDIFlags(value, bc)
	c.tick(16)
}

func (c *CPU_Z80) opLDDR() {
	c.opLDD()
	if c.BC() != 0 {
		c.PC -= 2
		c.tick(5)
	}
}

func (c *CPU_Z80) opCPI() {
	value := c.read(c.HL())
	c.SetHL(c.HL() + 1)
	bc := c.BC() - 1
	c.SetBC(bc)
	c.subA(value, 0, false)
	if bc != 0 {
		c.F |= z80FlagPV
	} else {
		c.F &^= z80FlagPV
	}
	c.tick(16)
}

func (c *CPU_Z80) opCPIR() {
	c.opCPI()
	if c.BC() != 0 && !c.Flag(z80FlagZ) {
		c.PC -= 2
		c.tick(5)
	}
}

func (c *CPU_Z80) opCPD() {
	value := c.read(c.HL())
	c.SetHL(c.HL() - 1)
	bc := c.BC() - 1
	c.SetBC(bc)
	c.subA(value, 0, false)
	if bc != 0 {
		c.F |= z80FlagPV
	} else {
		c.F &^= z80FlagPV
	}
	c.tick(16)
}

func (c *CPU_Z80) opCPDR() {
	c.opCPD()
	if c.BC() != 0 && !c.Flag(z80FlagZ) {
		c.PC -= 2
		c.tick(5)
	}
}

func (c *CPU_Z80) opINI() {
	port := c.BC()
	value := c.in(port)
	c.write(c.HL(), value)
	c.B--
	c.SetHL(c.HL() + 1)
	c.updateBlockIOFlags()
	c.tick(16)
}

func (c *CPU_Z80) opINIR() {
	c.opINI()
	if c.B != 0 {
		c.PC -= 2
		c.tick(5)
	}
}

func (c *CPU_Z80) opIND() {
	port := c.BC()
	value := c.in(port)
	c.write(c.HL(), value)
	c.B--
	c.SetHL(c.HL() - 1)
	c.updateBlockIOFlags()
	c.tick(16)
}

func (c *CPU_Z80) opINDR() {
	c.opIND()
	if c.B != 0 {
		c.PC -= 2
		c.tick(5)
	}
}

func (c *CPU_Z80) opOUTI() {
	value := c.read(c.HL())
	c.B--
	c.out(c.BC(), value)
	c.SetHL(c.HL() + 1)
	c.updateBlockIOFlags()
	c.tick(16)
}

func (c *CPU_Z80) opOTIR() {
	c.opOUTI()
	if c.B != 0 {
		c.PC -= 2
		c.tick(5)
	}
}

func (c *CPU_Z80) opOUTD() {
	value := c.read(c.HL())
	c.B--
	c.out(c.BC(), value)
	c.SetHL(c.HL() - 1)
	c.updateBlockIOFlags()
	c.tick(16)
}

func (c *CPU_Z80) opOTDR() {
	c.opOUTD()
	if c.B != 0 {
		c.PC -= 2
		c.tick(5)
	}
}

func (c *CPU_Z80) opLDNNBC() {
	addr := c.fetchWord()
	value := c.BC()
	c.write(addr, byte(value))
	c.write(addr+1, byte(value>>8))
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opLDBCNNED() {
	addr := c.fetchWord()
	low := c.read(addr)
	high := c.read(addr + 1)
	c.SetBC(uint16(high)<<8 | uint16(low))
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opLDNNDE() {
	addr := c.fetchWord()
	value := c.DE()
	c.write(addr, byte(value))
	c.write(addr+1, byte(value>>8))
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opLDDENNED() {
	addr := c.fetchWord()
	low := c.read(addr)
	high := c.read(addr + 1)
	c.SetDE(uint16(high)<<8 | uint16(low))
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opLDNNHLed() {
	addr := c.fetchWord()
	value := c.HL()
	c.write(addr, byte(value))
	c.write(addr+1, byte(value>>8))
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opLDHLNNed() {
	addr := c.fetchWord()
	low := c.read(addr)
	high := c.read(addr + 1)
	c.SetHL(uint16(high)<<8 | uint16(low))
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opLDNNSP() {
	addr := c.fetchWord()
	c.write(addr, byte(c.SP))
	c.write(addr+1, byte(c.SP>>8))
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opLDSPNNED() {
	addr := c.fetchWord()
	low := c.read(addr)
	high := c.read(addr + 1)
	c.SP = uint16(high)<<8 | uint16(low)
	c.WZ = addr + 1
	c.tick(20)
}

func (c *CPU_Z80) opADCHLBC() {
	c.adcHL(c.BC())
	c.tick(15)
}

func (c *CPU_Z80) opADCHLDE() {
	c.adcHL(c.DE())
	c.tick(15)
}

func (c *CPU_Z80) opADCHLHL() {
	c.adcHL(c.HL())
	c.tick(15)
}

func (c *CPU_Z80) opADCHLSP() {
	c.adcHL(c.SP)
	c.tick(15)
}

func (c *CPU_Z80) opSBCHLBC() {
	c.sbcHL(c.BC())
	c.tick(15)
}

func (c *CPU_Z80) opSBCHLDE() {
	c.sbcHL(c.DE())
	c.tick(15)
}

func (c *CPU_Z80) opSBCHLHL() {
	c.sbcHL(c.HL())
	c.tick(15)
}

func (c *CPU_Z80) opSBCHLSP() {
	c.sbcHL(c.SP)
	c.tick(15)
}

func (c *CPU_Z80) opDDCBPrefix() {
	disp := int8(c.fetchByte())
	opcode := c.fetchOpcode()
	addr := uint16(int32(c.IX) + int32(disp))
	c.cbOpsIndexed(addr, opcode, disp)
}

func (c *CPU_Z80) opFDCBPrefix() {
	disp := int8(c.fetchByte())
	opcode := c.fetchOpcode()
	addr := uint16(int32(c.IY) + int32(disp))
	c.cbOpsIndexed(addr, opcode, disp)
}

func (c *CPU_Z80) cbOpsIndexed(addr uint16, opcode byte, disp int8) {
	group := opcode >> 6
	switch group {
	case 0:
		c.cbIndexedRotateShift(addr, opcode)
	case 1:
		c.cbIndexedBIT(addr, opcode)
	case 2:
		c.cbIndexedRES(addr, opcode)
	case 3:
		c.cbIndexedSET(addr, opcode)
	}
}

func (c *CPU_Z80) cbIndexedRotateShift(addr uint16, opcode byte) {
	value := c.read(addr)
	reg := opcode & 0x07
	group := (opcode >> 3) & 0x07
	var res byte
	var carry bool

	switch group {
	case 0: // RLC
		carry = value&0x80 != 0
		res = value<<1 | value>>7
	case 1: // RRC
		carry = value&0x01 != 0
		res = value>>1 | value<<7
	case 2: // RL
		res, carry = c.rotate8Left(value, c.Flag(z80FlagC))
	case 3: // RR
		res, carry = c.rotate8Right(value, c.Flag(z80FlagC))
	case 4: // SLA
		res, carry = c.shiftLeftArithmetic(value)
	case 5: // SRA
		res, carry = c.shiftRightArithmetic(value)
	case 6: // SLL (undocumented, add later)
		res, carry = c.shiftLeftArithmetic(value)
		res |= 0x01
	case 7: // SRL
		res, carry = c.shiftRightLogical(value)
	}

	c.F &^= z80FlagN | z80FlagH | z80FlagC
	if carry {
		c.F |= z80FlagC
	}
	c.setSZPFlags(res)

	c.write(addr, res)
	if reg != 6 {
		c.writeReg8Plain(reg, res)
	}
	c.tick(23)
}

func (c *CPU_Z80) cbIndexedBIT(addr uint16, opcode byte) {
	value := c.read(addr)
	bit := (opcode >> 3) & 0x07
	mask := byte(1 << bit)
	c.F &^= z80FlagN | z80FlagZ | z80FlagS | z80FlagPV | z80FlagX | z80FlagY
	c.F |= z80FlagH
	if value&mask == 0 {
		c.F |= z80FlagZ | z80FlagPV
	}
	if bit == 7 && value&mask != 0 {
		c.F |= z80FlagS
	}
	c.F |= value & (z80FlagX | z80FlagY)
	c.tick(20)
}

func (c *CPU_Z80) cbIndexedRES(addr uint16, opcode byte) {
	bit := (opcode >> 3) & 0x07
	res := c.read(addr) &^ (1 << bit)
	c.write(addr, res)
	reg := opcode & 0x07
	if reg != 6 {
		c.writeReg8Plain(reg, res)
	}
	c.tick(23)
}

func (c *CPU_Z80) cbIndexedSET(addr uint16, opcode byte) {
	bit := (opcode >> 3) & 0x07
	res := c.read(addr) | (1 << bit)
	c.write(addr, res)
	reg := opcode & 0x07
	if reg != 6 {
		c.writeReg8Plain(reg, res)
	}
	c.tick(23)
}

func (c *CPU_Z80) opCBRotateShift(group, reg byte) {
	value := c.readReg8(reg)
	var res byte
	var carry bool
	switch group {
	case 0: // RLC
		carry = value&0x80 != 0
		res = value<<1 | value>>7
	case 1: // RRC
		carry = value&0x01 != 0
		res = value>>1 | value<<7
	case 2: // RL
		res, carry = c.rotate8Left(value, c.Flag(z80FlagC))
	case 3: // RR
		res, carry = c.rotate8Right(value, c.Flag(z80FlagC))
	case 4: // SLA
		res, carry = c.shiftLeftArithmetic(value)
	case 5: // SRA
		res, carry = c.shiftRightArithmetic(value)
	case 6: // SLL (undocumented, add later)
		res, carry = c.shiftLeftArithmetic(value)
		res |= 0x01
	case 7: // SRL
		res, carry = c.shiftRightLogical(value)
	}

	c.writeReg8(reg, res)
	c.F &^= z80FlagN | z80FlagH | z80FlagC
	if carry {
		c.F |= z80FlagC
	}
	c.setSZPFlags(res)

	if reg == 6 {
		c.tick(15)
	} else {
		c.tick(8)
	}
}

func (c *CPU_Z80) opCBBIT(bit, reg byte) {
	value := c.readReg8(reg)
	mask := byte(1 << bit)
	c.F &^= z80FlagN | z80FlagZ | z80FlagS | z80FlagPV | z80FlagX | z80FlagY
	c.F |= z80FlagH
	if value&mask == 0 {
		c.F |= z80FlagZ | z80FlagPV
	}
	if bit == 7 && value&mask != 0 {
		c.F |= z80FlagS
	}
	if reg == 6 {
		c.F |= (byte(value) & (z80FlagX | z80FlagY))
		c.tick(12)
	} else {
		c.F |= byte(value) & (z80FlagX | z80FlagY)
		c.tick(8)
	}
}

func (c *CPU_Z80) opCBRES(bit, reg byte) {
	value := c.readReg8(reg)
	res := value &^ (1 << bit)
	c.writeReg8(reg, res)
	if reg == 6 {
		c.tick(15)
	} else {
		c.tick(8)
	}
}

func (c *CPU_Z80) opCBSET(bit, reg byte) {
	value := c.readReg8(reg)
	res := value | (1 << bit)
	c.writeReg8(reg, res)
	if reg == 6 {
		c.tick(15)
	} else {
		c.tick(8)
	}
}

func (c *CPU_Z80) jpCond(cond bool) {
	addr := c.fetchWord()
	if cond {
		c.PC = addr
	}
	c.tick(10)
}

func (c *CPU_Z80) jrCond(cond bool) {
	disp := int8(c.fetchByte())
	if cond {
		c.PC = uint16(int32(c.PC) + int32(disp))
		c.tick(12)
	} else {
		c.tick(7)
	}
}

func (c *CPU_Z80) callCond(cond bool) {
	addr := c.fetchWord()
	if cond {
		c.pushWord(c.PC)
		c.PC = addr
		c.tick(17)
	} else {
		c.tick(10)
	}
}

func (c *CPU_Z80) retCond(cond bool) {
	if cond {
		c.PC = c.popWord()
		c.tick(11)
	} else {
		c.tick(5)
	}
}

func (c *CPU_Z80) fetchWord() uint16 {
	low := c.fetchByte()
	high := c.fetchByte()
	return uint16(high)<<8 | uint16(low)
}

func (c *CPU_Z80) pushWord(value uint16) {
	c.SP--
	c.write(c.SP, byte(value>>8))
	c.SP--
	c.write(c.SP, byte(value))
}

func (c *CPU_Z80) popWord() uint16 {
	low := c.read(c.SP)
	c.SP++
	high := c.read(c.SP)
	c.SP++
	return uint16(high)<<8 | uint16(low)
}

func (c *CPU_Z80) performALU(op aluOp, value byte) {
	switch op {
	case aluAdd:
		c.addA(value, 0)
	case aluAdc:
		carry := byte(0)
		if c.Flag(z80FlagC) {
			carry = 1
		}
		c.addA(value, carry)
	case aluSub:
		c.subA(value, 0, true)
	case aluSbc:
		carry := byte(0)
		if c.Flag(z80FlagC) {
			carry = 1
		}
		c.subA(value, carry, true)
	case aluAnd:
		c.andA(value)
	case aluXor:
		c.xorA(value)
	case aluOr:
		c.orA(value)
	case aluCp:
		c.subA(value, 0, false)
	}
}

func (c *CPU_Z80) addA(value byte, carry byte) {
	a := c.A
	sum := uint16(a) + uint16(value) + uint16(carry)
	res := byte(sum)

	c.A = res
	c.F = 0
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x80 != 0 {
		c.F |= z80FlagS
	}
	if ((a&0x0F)+(value&0x0F)+carry)&0x10 != 0 {
		c.F |= z80FlagH
	}
	if ((^(a ^ value))&(a^res))&0x80 != 0 {
		c.F |= z80FlagPV
	}
	if sum > 0xFF {
		c.F |= z80FlagC
	}
	c.F |= res & (z80FlagX | z80FlagY)
}

func (c *CPU_Z80) subA(value byte, carry byte, store bool) {
	a := c.A
	diff := int(a) - int(value) - int(carry)
	res := byte(diff)

	if store {
		c.A = res
	}

	c.F = z80FlagN
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x80 != 0 {
		c.F |= z80FlagS
	}
	if int(a&0x0F)-int(value&0x0F)-int(carry) < 0 {
		c.F |= z80FlagH
	}
	if ((a ^ value) & (a ^ res) & 0x80) != 0 {
		c.F |= z80FlagPV
	}
	if diff < 0 {
		c.F |= z80FlagC
	}
	c.F |= res & (z80FlagX | z80FlagY)
}

func (c *CPU_Z80) andA(value byte) {
	res := c.A & value
	c.A = res
	c.F = z80FlagH
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x80 != 0 {
		c.F |= z80FlagS
	}
	if parity8(res) {
		c.F |= z80FlagPV
	}
	c.F |= res & (z80FlagX | z80FlagY)
}

func (c *CPU_Z80) xorA(value byte) {
	res := c.A ^ value
	c.A = res
	c.F = 0
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x80 != 0 {
		c.F |= z80FlagS
	}
	if parity8(res) {
		c.F |= z80FlagPV
	}
	c.F |= res & (z80FlagX | z80FlagY)
}

func (c *CPU_Z80) orA(value byte) {
	res := c.A | value
	c.A = res
	c.F = 0
	if res == 0 {
		c.F |= z80FlagZ
	}
	if res&0x80 != 0 {
		c.F |= z80FlagS
	}
	if parity8(res) {
		c.F |= z80FlagPV
	}
	c.F |= res & (z80FlagX | z80FlagY)
}

func parity8(value byte) bool {
	value ^= value >> 4
	value ^= value >> 2
	value ^= value >> 1
	return value&1 == 0
}
