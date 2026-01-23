// ted_6502_bus.go - Plus/4 6502 memory bus emulation for TED playback

/*
This implements a minimal Plus/4-like memory bus for playing .ted music files.
The Plus/4 has a different memory map than the C64:

Memory Map:
  $0000-$00FF  Zero page
  $0100-$01FF  Stack
  $0200-$03FF  System variables
  $0400-$0FFF  Screen RAM (when banked in)
  $1001-$FCFF  BASIC/Program space
  $FD00-$FEFF  I/O registers
  $FF00-$FF3F  TED chip registers
  $FF40-$FFFF  ROM/Vectors

TED Sound Registers (we care about):
  $FF0E - Voice 1 frequency low
  $FF0F - Voice 2 frequency low
  $FF10 - Voice 2 frequency high (bits 0-1)
  $FF11 - Sound control (DA/noise/ch2on/ch1on/volume)
  $FF12 - Voice 1 frequency high (bits 0-1)
*/

package main

// TED6502Bus implements a minimal Plus/4-like memory bus for TED playback.
type TED6502Bus struct {
	ram     [0x10000]byte
	tedRegs [TED_REG_COUNT]uint8
	events  []TEDEvent

	cycles     uint64
	frameCycle uint64

	// Simulated raster position (computed from frame cycles)
	rasterLine uint16

	// TED Timer 1 simulation
	timer1Latch      uint16 // Timer reload value
	timer1Counter    uint16 // Current counter value
	timer1Running    bool   // Timer is active
	timer1LastUpdate uint64 // Cycles when timer was last updated
	irqFlags         uint8  // $FF09 interrupt flags (bit 3 = timer 1)
	irqMask          uint8  // $FF0A interrupt mask
	irqPending       bool   // Signal to CPU that IRQ should fire

	// Raster interrupt control
	rasterIRQEnabled bool // Set to true after init to enable per-frame raster IRQs
	frameCount       int  // Frame counter

	ntsc bool
}

// newTED6502Bus creates a new Plus/4 memory bus.
func newTED6502Bus(ntsc bool) *TED6502Bus {
	bus := &TED6502Bus{
		ntsc: ntsc,
	}
	bus.installVectors()
	return bus
}

// Read reads a byte from the given address.
func (b *TED6502Bus) Read(addr uint16) byte {
	// TED registers at $FF00-$FF3F
	if addr >= 0xFF00 && addr <= 0xFF3F {
		return b.readTED(addr)
	}
	return b.ram[addr]
}

// Write writes a byte to the given address.
func (b *TED6502Bus) Write(addr uint16, value byte) {
	// TED registers at $FF00-$FF3F
	if addr >= 0xFF00 && addr <= 0xFF3F {
		b.writeTED(addr, value)
		return
	}
	b.ram[addr] = value
}

// TED video/timing registers
const (
	PLUS4_TED_TIMER1_LO = 0xFF00 // Timer 1 latch low byte
	PLUS4_TED_TIMER1_HI = 0xFF01 // Timer 1 latch high byte
	PLUS4_TED_TIMER2_LO = 0xFF02 // Timer 2 latch low byte
	PLUS4_TED_TIMER2_HI = 0xFF03 // Timer 2 latch high byte
	PLUS4_TED_TIMER3_LO = 0xFF04 // Timer 3 latch low byte
	PLUS4_TED_TIMER3_HI = 0xFF05 // Timer 3 latch high byte
	PLUS4_TED_IRQ_FLAGS = 0xFF09 // Interrupt flag register
	PLUS4_TED_IRQ_MASK  = 0xFF0A // Interrupt mask register
	PLUS4_TED_RASTER_LO = 0xFF1C // Raster line low byte
	PLUS4_TED_RASTER_HI = 0xFF1D // Raster line high bit + flags
)

// TED IRQ flag bits
const (
	TED_IRQ_RASTER = 0x02 // Bit 1: Raster interrupt
	TED_IRQ_TIMER1 = 0x08 // Bit 3: Timer 1 interrupt
	TED_IRQ_TIMER2 = 0x10 // Bit 4: Timer 2 interrupt
	TED_IRQ_TIMER3 = 0x40 // Bit 6: Timer 3 interrupt
)

// Cycles per scanline for PAL TED
// PAL: 886724 Hz / 50 fps / 312 lines = ~56.8 cycles per line
const TED_CYCLES_PER_LINE = 57

// Total scanlines per frame (PAL: 312, NTSC: 262)
const (
	TED_PAL_LINES  = 312
	TED_NTSC_LINES = 262
)

// readTED reads from a TED register
func (b *TED6502Bus) readTED(addr uint16) byte {
	// Map Plus/4 addresses to our register array
	switch addr {
	case PLUS4_TED_FREQ1_LO:
		return b.tedRegs[TED_REG_FREQ1_LO]
	case PLUS4_TED_FREQ2_LO:
		return b.tedRegs[TED_REG_FREQ2_LO]
	case PLUS4_TED_FREQ2_HI:
		return b.tedRegs[TED_REG_FREQ2_HI]
	case PLUS4_TED_SND_CTRL:
		return b.tedRegs[TED_REG_SND_CTRL]
	case PLUS4_TED_FREQ1_HI:
		return b.tedRegs[TED_REG_FREQ1_HI]

	case PLUS4_TED_TIMER1_LO:
		// Return current counter low byte
		return byte(b.timer1Counter & 0xFF)
	case PLUS4_TED_TIMER1_HI:
		// Return current counter high byte
		return byte(b.timer1Counter >> 8)

	case PLUS4_TED_IRQ_FLAGS:
		// Return IRQ flags - reading clears them (active-low on real hardware)
		// Bit 3 = Timer 1 interrupt flag
		flags := b.irqFlags
		// Note: On real TED, writing 1s to $FF09 clears the corresponding flags
		// For polling, we just return current state
		return flags

	case PLUS4_TED_IRQ_MASK:
		// Return IRQ mask register
		return b.ram[addr]

	case PLUS4_TED_RASTER_LO:
		// $FF1C: Raster line low byte (bits 0-7)
		b.updateRasterPosition()
		return byte(b.rasterLine & 0xFF)
	case PLUS4_TED_RASTER_HI:
		// $FF1D: Video control register with raster bit 8
		// Bit 0 = raster line bit 8
		// Bits 1-7 = various control flags
		// TED players often wait for specific raster lines (e.g., $CD = 205)
		// by comparing $FF1D directly with the target line number
		b.updateRasterPosition()
		// For compatibility with wait loops that do CMP $FF1D with values like $CD,
		// return the full raster line as if it were the "comparison value"
		// This lets wait loops eventually match their target
		return byte(b.rasterLine & 0xFF)
	default:
		// Other TED registers - return from RAM for now
		return b.ram[addr]
	}
}

// updateRasterPosition computes the raster position from absolute frame cycles
func (b *TED6502Bus) updateRasterPosition() {
	// Calculate raster line directly from cycles since frame start
	// This ensures consistent timing regardless of polling frequency
	frameCycles := b.cycles - b.frameCycle

	maxLines := uint16(TED_PAL_LINES)
	if b.ntsc {
		maxLines = TED_NTSC_LINES
	}

	// Calculate line position within frame
	b.rasterLine = uint16(frameCycles/TED_CYCLES_PER_LINE) % maxLines
}

// writeTED writes to a TED register and captures events for sound registers
func (b *TED6502Bus) writeTED(addr uint16, value byte) {
	// Store in RAM too (for non-sound registers)
	b.ram[addr] = value

	// Handle timer and IRQ registers
	switch addr {
	case PLUS4_TED_TIMER1_LO:
		// Write to timer 1 latch low byte and enable timer
		b.timer1Latch = (b.timer1Latch & 0xFF00) | uint16(value)
		b.timer1Counter = b.timer1Latch
		b.timer1Running = true
		return
	case PLUS4_TED_TIMER1_HI:
		// Write to timer 1 latch high byte and enable timer
		b.timer1Latch = (b.timer1Latch & 0x00FF) | (uint16(value) << 8)
		b.timer1Counter = b.timer1Latch
		b.timer1Running = true
		return
	case PLUS4_TED_TIMER2_LO, PLUS4_TED_TIMER2_HI:
		// Timer 2 - not implemented yet, just store in RAM
		return
	case PLUS4_TED_TIMER3_LO, PLUS4_TED_TIMER3_HI:
		// Timer 3 - not implemented yet, just store in RAM
		return
	case PLUS4_TED_IRQ_FLAGS:
		// Writing 1s to $FF09 clears the corresponding interrupt flags
		b.irqFlags &= ^value
		return
	case PLUS4_TED_IRQ_MASK:
		// IRQ mask register - controls which interrupts can fire
		b.irqMask = value
		return
	}

	// Map Plus/4 addresses to our register array and capture events
	var reg uint8
	var isSoundReg bool

	switch addr {
	case PLUS4_TED_FREQ1_LO:
		reg = TED_REG_FREQ1_LO
		isSoundReg = true
	case PLUS4_TED_FREQ2_LO:
		reg = TED_REG_FREQ2_LO
		isSoundReg = true
	case PLUS4_TED_FREQ2_HI:
		reg = TED_REG_FREQ2_HI
		isSoundReg = true
	case PLUS4_TED_SND_CTRL:
		reg = TED_REG_SND_CTRL
		isSoundReg = true
	case PLUS4_TED_FREQ1_HI:
		reg = TED_REG_FREQ1_HI
		isSoundReg = true
	}

	if isSoundReg {
		b.tedRegs[reg] = value
		b.events = append(b.events, TEDEvent{
			Cycle: b.cycles,
			Reg:   reg,
			Value: value,
		})
	}
}

// installVectors sets up CPU vectors for IRQ handling
func (b *TED6502Bus) installVectors() {
	// IRQ stub: JMP ($0314) - same as C64 pattern
	b.ram[0xFF00] = 0x6C // JMP ($0314)
	b.ram[0xFF01] = 0x14
	b.ram[0xFF02] = 0x03

	// Set up IRQ vector to point to stub
	b.ram[0xFFFE] = 0x00 // IRQ vector low
	b.ram[0xFFFF] = 0xFF // IRQ vector high -> $FF00

	// RESET vector
	b.ram[0xFFFC] = 0x00
	b.ram[0xFFFD] = 0x10 // Point to $1000

	// NMI vector (unused but set up)
	b.ram[0xFFFA] = 0x40 // RTI
	b.ram[0xFFFB] = 0x00
}

// AddCycles advances the bus clock and updates timers
func (b *TED6502Bus) AddCycles(cycles int) {
	if cycles <= 0 {
		return
	}
	b.cycles += uint64(cycles)

	// Update Timer 1 - counts down every cycle when enabled
	// (TED actually counts every other cycle, but we simplify)
	if b.timer1Running {
		for i := 0; i < cycles; i++ {
			if b.timer1Counter == 0 {
				// Timer underflowed - set IRQ flag bit 3
				b.irqFlags |= TED_IRQ_TIMER1
				// If mask bit 3 is set, also set bit 7 (indicates active interrupt)
				if (b.irqMask & TED_IRQ_TIMER1) != 0 {
					b.irqFlags |= 0x80
					b.irqPending = true
				}
				// Reload from latch (tedplay uses t1start-1)
				b.timer1Counter = b.timer1Latch
			} else {
				b.timer1Counter--
			}
		}
	}
}

// StartFrame starts a new frame and clears captured events
// Also generates a raster interrupt (TED fires raster IRQ at frame end)
func (b *TED6502Bus) StartFrame() {
	b.frameCycle = b.cycles
	b.events = b.events[:0]
	b.rasterLine = 0
	b.frameCount++

	// Generate raster interrupt at frame start (simulates VBlank IRQ)
	// Only after raster IRQs are enabled (skip first frame which is init)
	if b.rasterIRQEnabled {
		b.irqFlags |= TED_IRQ_RASTER
		b.irqFlags |= 0x80 // Set bit 7 to indicate active interrupt
		b.irqPending = true
	}
}

// EnableRasterIRQ enables per-frame raster interrupts
// Call this after init is complete
func (b *TED6502Bus) EnableRasterIRQ() {
	b.rasterIRQEnabled = true
}

// CollectEvents returns captured TED events and clears the list
func (b *TED6502Bus) CollectEvents() []TEDEvent {
	if len(b.events) == 0 {
		return nil
	}
	events := make([]TEDEvent, len(b.events))
	copy(events, b.events)
	b.events = b.events[:0]
	return events
}

// LoadBinary loads binary data into memory at the specified address
func (b *TED6502Bus) LoadBinary(addr uint16, data []byte) {
	for i, v := range data {
		target := addr + uint16(i)
		b.ram[target] = v
	}
}

// Reset resets the bus state
func (b *TED6502Bus) Reset() {
	b.cycles = 0
	b.frameCycle = 0
	b.events = nil
	b.rasterLine = 0
	b.timer1Latch = 0
	b.timer1Counter = 0
	b.timer1Running = false
	b.timer1LastUpdate = 0
	b.irqFlags = 0
	b.irqMask = 0
	b.irqPending = false

	for i := range b.tedRegs {
		b.tedRegs[i] = 0
	}

	b.installVectors()
}

// CheckIRQ returns true if an IRQ is pending and clears the pending flag
func (b *TED6502Bus) CheckIRQ() bool {
	if b.irqPending {
		b.irqPending = false
		return true
	}
	return false
}

// GetCycles returns the total cycle count
func (b *TED6502Bus) GetCycles() uint64 {
	return b.cycles
}

// GetFrameCycles returns cycles since frame start
func (b *TED6502Bus) GetFrameCycles() uint64 {
	return b.cycles - b.frameCycle
}
