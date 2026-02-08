// sndh_68k_bus.go - Memory bus for SNDH playback with YM2149 interception.
//
// This bus provides a 256KB address space for 68K SNDH code execution,
// with hardware register interception for YM2149 at Atari ST addresses.
//
// Atari ST YM2149 addresses:
//   0xFF8800: Register select (write) / Register data (read)
//   0xFF8802: Register data (write)

package main

import (
	"unsafe"
)

const (
	// Atari ST YM2149 hardware addresses
	YM_REG_SELECT = 0xFF8800 // Write: select register, Read: read data
	YM_REG_DATA   = 0xFF8802 // Write: write data

	// SNDH bus memory size (256KB)
	SNDH_BUS_SIZE = 256 * 1024

	// MFP 68901 registers (at 0xFFFA00)
	MFP_BASE  = 0xFFFA00
	MFP_IERA  = 0xFFFA07 // Interrupt Enable Register A
	MFP_IERB  = 0xFFFA09 // Interrupt Enable Register B
	MFP_IPRA  = 0xFFFA0B // Interrupt Pending Register A (Timer A/B)
	MFP_IPRB  = 0xFFFA0D // Interrupt Pending Register B (Timer C/D)
	MFP_ISRA  = 0xFFFA0F // Interrupt In-Service Register A
	MFP_ISRB  = 0xFFFA11 // Interrupt In-Service Register B
	MFP_IMRA  = 0xFFFA13 // Interrupt Mask Register A
	MFP_IMRB  = 0xFFFA15 // Interrupt Mask Register B
	MFP_VR    = 0xFFFA17 // Vector Register
	MFP_TACR  = 0xFFFA19 // Timer A Control Register
	MFP_TBCR  = 0xFFFA1B // Timer B Control Register
	MFP_TCDCR = 0xFFFA1D // Timer C/D Control Register
	MFP_TADR  = 0xFFFA1F // Timer A Data Register
	MFP_TBDR  = 0xFFFA21 // Timer B Data Register
	MFP_TCDR  = 0xFFFA23 // Timer C Data Register
	MFP_TDDR  = 0xFFFA25 // Timer D Data Register

	// MFP clock is 2.4576 MHz, but we use 8MHz/4 = 2MHz for simplicity
	MFP_CLOCK = 2000000
)

// sndhPlaybackWrite68K records a YM2149 register write with timing
type sndhPlaybackWrite68K struct {
	Reg   uint8
	Value uint8
	Cycle uint64
}

// mfpTimer represents one MFP timer
type mfpTimer struct {
	control  uint8  // Control register (prescaler mode)
	data     uint8  // Data register (countdown value)
	counter  uint8  // Current counter value
	cycleAcc uint64 // Accumulated cycles for this timer
}

// sndhPlaybackBus68K provides memory and YM2149 I/O for SNDH playback
type sndhPlaybackBus68K struct {
	memory    []byte
	regSelect uint8                  // Currently selected YM register
	regs      [PSG_REG_COUNT]byte    // Shadow copy of YM registers
	writes    []sndhPlaybackWrite68K // Captured register writes
	cycles    uint64                 // Cycle counter

	// MFP timer state
	timerA mfpTimer
	timerB mfpTimer
	timerC mfpTimer
	timerD mfpTimer
	iera   uint8 // Interrupt Enable Register A
	ierb   uint8 // Interrupt Enable Register B
	ipra   uint8 // Interrupt Pending Register A
	iprb   uint8 // Interrupt Pending Register B
	imra   uint8 // Interrupt Mask Register A
	imrb   uint8 // Interrupt Mask Register B
	vr     uint8 // Vector Register

	// Last cycle count for timer updates
	lastTimerCycles uint64
}

// newSndhPlaybackBus68K creates a new bus for SNDH playback
func newSndhPlaybackBus68K() *sndhPlaybackBus68K {
	return &sndhPlaybackBus68K{
		memory: make([]byte, SNDH_BUS_SIZE),
		writes: make([]sndhPlaybackWrite68K, 0, 1024),
	}
}

// MFP timer prescaler divisors (index = control register lower 3 bits)
// 0=stopped, 1=/4, 2=/10, 3=/16, 4=/50, 5=/64, 6=/100, 7=/200
var mfpPrescaler = [8]uint32{0, 4, 10, 16, 50, 64, 100, 200}

// Reset clears the bus state
func (b *sndhPlaybackBus68K) Reset() {
	for i := range b.memory {
		b.memory[i] = 0
	}
	b.regSelect = 0
	for i := range b.regs {
		b.regs[i] = 0
	}
	b.writes = b.writes[:0]
	b.cycles = 0

	// Reset MFP timers
	b.timerA = mfpTimer{}
	b.timerB = mfpTimer{}
	b.timerC = mfpTimer{}
	b.timerD = mfpTimer{}
	b.iera = 0xFF // All interrupts enabled by default
	b.ierb = 0xFF
	b.ipra = 0
	b.iprb = 0
	b.imra = 0xFF // All interrupts enabled by default
	b.imrb = 0xFF
	b.vr = 0
	b.lastTimerCycles = 0
}

// GetMemory returns the raw memory array
func (b *sndhPlaybackBus68K) GetMemory() []byte {
	return b.memory
}

// ResetWrites clears the recorded writes (called at frame start)
func (b *sndhPlaybackBus68K) ResetWrites() {
	b.writes = b.writes[:0]
}

// GetWrites returns the recorded writes since last reset
func (b *sndhPlaybackBus68K) GetWrites() []sndhPlaybackWrite68K {
	return b.writes
}

// AddCycles adds to the cycle counter (called by CPU after each instruction)
func (b *sndhPlaybackBus68K) AddCycles(cycles int) {
	b.cycles += uint64(cycles)
	// Update MFP timers
	b.tickTimers()
}

// GetCycles returns the current cycle count
func (b *sndhPlaybackBus68K) GetCycles() uint64 {
	return b.cycles
}

// debugMFP enables MFP timer debugging
var debugMFP = false

// tickTimers updates MFP timer counters based on elapsed cycles
func (b *sndhPlaybackBus68K) tickTimers() {
	elapsed := b.cycles - b.lastTimerCycles
	if elapsed == 0 {
		return
	}
	b.lastTimerCycles = b.cycles

	// Tick each timer
	b.tickTimer(&b.timerA, elapsed, 0x20)   // Timer A sets bit 5 in IPRA
	b.tickTimer(&b.timerB, elapsed, 0x01)   // Timer B sets bit 0 in IPRA
	b.tickTimerCD(&b.timerC, elapsed, 0x20) // Timer C sets bit 5 in IPRB
	b.tickTimerCD(&b.timerD, elapsed, 0x10) // Timer D sets bit 4 in IPRB
}

// tickTimer ticks a single timer (A or B) and sets interrupt pending if needed
func (b *sndhPlaybackBus68K) tickTimer(t *mfpTimer, elapsed uint64, pendingBit uint8) {
	prescale := mfpPrescaler[t.control&0x07]
	if prescale == 0 {
		return // Timer stopped
	}

	// Convert CPU cycles to timer ticks
	// MFP clock = 2.4576 MHz, CPU clock = 8 MHz
	// Ratio is roughly 8/2.4576 ≈ 3.26, we use 4 for simplicity
	t.cycleAcc += elapsed
	timerTicks := t.cycleAcc / (4 * uint64(prescale))
	t.cycleAcc -= timerTicks * 4 * uint64(prescale)

	// Count down
	for timerTicks > 0 {
		if t.counter == 0 {
			t.counter = t.data   // Reload
			b.ipra |= pendingBit // Set interrupt pending
			if debugMFP {
				println("Timer fired! IPRA=", b.ipra, "data=", t.data, "prescale=", prescale)
			}
		}
		t.counter--
		timerTicks--
	}
}

// tickTimerCD ticks Timer C or D (sets bits in IPRB)
func (b *sndhPlaybackBus68K) tickTimerCD(t *mfpTimer, elapsed uint64, pendingBit uint8) {
	prescale := mfpPrescaler[t.control&0x07]
	if prescale == 0 {
		return // Timer stopped
	}

	t.cycleAcc += elapsed
	timerTicks := t.cycleAcc / (4 * uint64(prescale))
	t.cycleAcc -= timerTicks * 4 * uint64(prescale)

	for timerTicks > 0 {
		if t.counter == 0 {
			t.counter = t.data
			b.iprb |= pendingBit
			if debugMFP {
				println("Timer C/D fired! IPRB=", b.iprb, "data=", t.data, "prescale=", prescale)
			}
		}
		t.counter--
		timerTicks--
	}
}

// debugMFPRead enables tracing of MFP register reads
var debugMFPRead = false

// readMFP reads an MFP register
func (b *sndhPlaybackBus68K) readMFP(addr uint32) uint8 {
	var result uint8
	switch addr {
	case MFP_IERA:
		result = b.iera
	case MFP_IERB:
		result = b.ierb
	case MFP_IPRA:
		result = b.ipra
		if debugMFPRead {
			println("  readMFP IPRA: addr=", addr, "MFP_IPRA=", MFP_IPRA, "b.ipra=", b.ipra, "result=", result)
		}
	case MFP_IPRB:
		result = b.iprb
	case MFP_ISRA:
		result = 0 // In-service register (not tracking)
	case MFP_ISRB:
		result = 0
	case MFP_IMRA:
		result = b.imra
	case MFP_IMRB:
		result = b.imrb
	case MFP_VR:
		result = b.vr
	case MFP_TACR:
		result = b.timerA.control
	case MFP_TBCR:
		result = b.timerB.control
	case MFP_TCDCR:
		result = (b.timerC.control << 4) | (b.timerD.control & 0x0F)
	case MFP_TADR:
		result = b.timerA.counter
	case MFP_TBDR:
		result = b.timerB.counter
	case MFP_TCDR:
		result = b.timerC.counter
	case MFP_TDDR:
		result = b.timerD.counter
	default:
		result = 0
	}
	if debugMFPRead {
		println("MFP READ addr=", addr, "value=", result)
	}
	return result
}

// writeMFP writes an MFP register
func (b *sndhPlaybackBus68K) writeMFP(addr uint32, value uint8) {
	if debugMFP {
		println("MFP write addr=", addr, "value=", value)
	}
	switch addr {
	case MFP_IERA:
		b.iera = value
	case MFP_IERB:
		b.ierb = value
	case MFP_IPRA:
		// Writing clears the bits that are 0 in the written value
		b.ipra &= value
	case MFP_IPRB:
		b.iprb &= value
	case MFP_IMRA:
		b.imra = value
	case MFP_IMRB:
		b.imrb = value
	case MFP_VR:
		b.vr = value
	case MFP_TACR:
		b.timerA.control = value & 0x0F
		if value&0x10 != 0 {
			// Reset counter when bit 4 is set
			b.timerA.counter = b.timerA.data
		}
	case MFP_TBCR:
		b.timerB.control = value & 0x0F
		if value&0x10 != 0 {
			b.timerB.counter = b.timerB.data
		}
	case MFP_TCDCR:
		b.timerC.control = (value >> 4) & 0x07
		b.timerD.control = value & 0x07
	case MFP_TADR:
		b.timerA.data = value
		if b.timerA.counter == 0 {
			b.timerA.counter = value
		}
	case MFP_TBDR:
		b.timerB.data = value
		if b.timerB.counter == 0 {
			b.timerB.counter = value
		}
	case MFP_TCDR:
		b.timerC.data = value
		if b.timerC.counter == 0 {
			b.timerC.counter = value
		}
	case MFP_TDDR:
		b.timerD.data = value
		if b.timerD.counter == 0 {
			b.timerD.counter = value
		}
	}
}

// Read8 reads a byte from memory or hardware register
func (b *sndhPlaybackBus68K) Read8(addr uint32) uint8 {
	// Mask to 24-bit address space (Atari ST)
	addr24 := addr & 0x00FFFFFF

	// Check for YM2149 register read
	if addr24 == (YM_REG_SELECT & 0x00FFFFFF) {
		if b.regSelect < PSG_REG_COUNT {
			return b.regs[b.regSelect]
		}
		return 0
	}

	// Handle MFP (68901) registers
	if addr24 >= 0xFFFA00 && addr24 <= 0xFFFA3F {
		return b.readMFP(addr24)
	}
	if addr24 >= 0xFF8200 && addr24 <= 0xFF82FF {
		// Video registers - return 0
		return 0
	}
	if addr24 >= 0xFF8800 && addr24 <= 0xFF88FF {
		// PSG register space (return shadow)
		if addr24 == 0xFF8800 && b.regSelect < PSG_REG_COUNT {
			return b.regs[b.regSelect]
		}
		return 0
	}

	// Mask address to bus size for memory access
	return b.memory[addr24&(SNDH_BUS_SIZE-1)]
}

// debugYM enables YM register write tracing
var debugYM = false

// Write8 writes a byte to memory or hardware register
func (b *sndhPlaybackBus68K) Write8(addr uint32, value uint8) {
	// Mask to 24-bit address space (Atari ST)
	addr24 := addr & 0x00FFFFFF

	// Trace all YM-region accesses for debugging
	if debugYM && addr24 >= 0xFF8800 && addr24 <= 0xFF88FF {
		println("YM WRITE8 addr=", addr24, "value=", value, "regSelect=", b.regSelect)
	}

	// Check for YM2149 register select
	if addr24 == (YM_REG_SELECT & 0x00FFFFFF) {
		if debugYM {
			println("  -> REG SELECT:", value&0x0F)
		}
		b.regSelect = value & 0x0F
		return
	}

	// Check for YM2149 data write
	// On Atari ST, both 0xFF8801 (odd byte of register port) and 0xFF8802 are data write addresses
	// Word writes to 0xFF8800 split into: 0xFF8800 (reg select) + 0xFF8801 (data write)
	if addr24 == (YM_REG_DATA&0x00FFFFFF) || addr24 == ((YM_REG_SELECT+1)&0x00FFFFFF) {
		if debugYM {
			println("YM DATA WRITE: reg=", b.regSelect, "value=", value, "addr=", addr24)
		}
		if b.regSelect < PSG_REG_COUNT {
			b.regs[b.regSelect] = value
			b.writes = append(b.writes, sndhPlaybackWrite68K{
				Reg:   b.regSelect,
				Value: value,
				Cycle: b.cycles,
			})
		}
		return
	}

	// Handle MFP (68901) register writes
	if addr24 >= 0xFFFA00 && addr24 <= 0xFFFA3F {
		b.writeMFP(addr24, value)
		return
	}
	if addr24 >= 0xFF8200 && addr24 <= 0xFF82FF {
		// Video registers - ignore
		return
	}

	// Mask address to bus size for memory write
	b.memory[addr24&(SNDH_BUS_SIZE-1)] = value
}

// Read16 reads a word from memory
// Note: M68K CPU converts to little-endian before calling bus, so bus uses little-endian
func (b *sndhPlaybackBus68K) Read16(addr uint32) uint16 {
	// Check for YM2149 register read
	if addr == YM_REG_SELECT || addr == YM_REG_SELECT+1 {
		hi := b.Read8(YM_REG_SELECT)
		lo := b.Read8(YM_REG_SELECT + 1)
		return uint16(hi)<<8 | uint16(lo)
	}

	// Mask address to bus size
	addr &= SNDH_BUS_SIZE - 1
	if addr+1 >= SNDH_BUS_SIZE {
		return uint16(b.memory[addr])
	}
	return *(*uint16)(unsafe.Pointer(&b.memory[addr]))
}

// Write16 writes a word to memory
// Note: M68K CPU converts to little-endian before calling bus, so bus uses little-endian
func (b *sndhPlaybackBus68K) Write16(addr uint32, value uint16) {
	// Check for YM2149 register select (write to 0xFF8800)
	// CPU sends little-endian, but we need big-endian for register/data extraction
	// Convert back: original big-endian byte order was (reg, data)
	if addr == YM_REG_SELECT {
		// Swap bytes back to big-endian for YM register handling
		beValue := ((value & 0xFF) << 8) | ((value & 0xFF00) >> 8)
		b.Write8(YM_REG_SELECT, uint8(beValue>>8)) // Register select
		b.Write8(YM_REG_SELECT+1, uint8(beValue))  // Data value
		return
	}

	// Check for YM2149 data write (write to 0xFF8802)
	if addr == YM_REG_DATA {
		// Swap bytes back to big-endian
		beValue := ((value & 0xFF) << 8) | ((value & 0xFF00) >> 8)
		b.Write8(YM_REG_DATA, uint8(beValue>>8))
		b.Write8(YM_REG_DATA+1, uint8(beValue))
		return
	}

	// Mask address to bus size
	addr &= SNDH_BUS_SIZE - 1
	if addr+1 >= SNDH_BUS_SIZE {
		b.memory[addr] = uint8(value)
		return
	}
	*(*uint16)(unsafe.Pointer(&b.memory[addr])) = value
}

// Read32 reads a long from memory
// Note: M68K CPU converts to little-endian before calling bus, so bus uses little-endian
func (b *sndhPlaybackBus68K) Read32(addr uint32) uint32 {
	// Mask address to bus size
	addr &= SNDH_BUS_SIZE - 1
	if addr+3 >= SNDH_BUS_SIZE {
		return 0
	}
	return *(*uint32)(unsafe.Pointer(&b.memory[addr]))
}

// Write32 writes a long to memory
// Note: M68K CPU converts to little-endian before calling bus, so bus uses little-endian
func (b *sndhPlaybackBus68K) Write32(addr uint32, value uint32) {
	// Mask to 24-bit address space
	addr24 := addr & 0x00FFFFFF

	// Check for YM2149 register write (long write spans reg select + data)
	if addr24 == (YM_REG_SELECT & 0x00FFFFFF) {
		// On Atari ST, MOVE.L #$RRXXVVXX, ($8800).W writes:
		//   byte 0 to $FF8800 (register select)
		//   byte 1 to $FF8801 (ignored on real hardware)
		//   byte 2 to $FF8802 (data write!)
		//   byte 3 to $FF8803 (ignored)
		//
		// In our little-endian storage:
		//   bits 0-7   = memory[N+0] = reg select
		//   bits 8-15  = memory[N+1] = ignored
		//   bits 16-23 = memory[N+2] = data value!
		//   bits 24-31 = memory[N+3] = ignored
		regSelect := uint8(value & 0xFF)
		dataValue := uint8((value >> 16) & 0xFF) // Byte 2, not byte 1!
		b.Write8(YM_REG_SELECT, regSelect)
		b.Write8(YM_REG_DATA, dataValue) // Write to $FF8802
		return
	}

	// Handle MFP region - split into byte writes
	if addr24 >= 0xFFFA00 && addr24 <= 0xFFFA3F {
		// Little-endian: LSB first, so byte 0 is at bits 0-7
		b.writeMFP(addr24, uint8(value&0xFF))
		b.writeMFP(addr24+1, uint8((value>>8)&0xFF))
		b.writeMFP(addr24+2, uint8((value>>16)&0xFF))
		b.writeMFP(addr24+3, uint8((value>>24)&0xFF))
		return
	}

	// Mask address to bus size for memory write
	addr &= SNDH_BUS_SIZE - 1
	if addr+3 >= SNDH_BUS_SIZE {
		return
	}
	*(*uint32)(unsafe.Pointer(&b.memory[addr])) = value
}

// LoadSNDH loads SNDH data into the bus memory
func (b *sndhPlaybackBus68K) LoadSNDH(data []byte) {
	// Copy data to memory starting at address 0
	copyLen := len(data)
	if copyLen > SNDH_BUS_SIZE {
		copyLen = SNDH_BUS_SIZE
	}
	copy(b.memory[:copyLen], data[:copyLen])
}
