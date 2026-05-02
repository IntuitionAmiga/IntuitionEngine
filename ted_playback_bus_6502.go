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

// TEDPlaybackBus6502 implements a minimal Plus/4-like memory bus for TED playback.
type TEDPlaybackBus6502 struct {
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
	timer2Latch      uint16
	timer2Counter    uint16
	timer2Running    bool
	timer3Latch      uint16
	timer3Counter    uint16
	timer3Running    bool
	irqFlags         uint8 // $FF09 interrupt source flags
	irqMask          uint8 // $FF0A interrupt mask
	irqPending       bool  // Signal to CPU that IRQ should fire

	// Raster interrupt control
	rasterIRQEnabled    bool // Legacy compatibility flag; does not synthesize IRQs.
	rasterCmp           uint16
	cmpLatchedThisFrame bool
	frameCount          int // Frame counter

	ntsc bool
}

// newTEDPlaybackBus6502 creates a new Plus/4 memory bus.
func newTEDPlaybackBus6502(ntsc bool) *TEDPlaybackBus6502 {
	bus := &TEDPlaybackBus6502{
		ntsc: ntsc,
	}
	bus.installVectors()
	return bus
}

// EnableKERNALTimer sets up Timer 1 like the Plus/4 KERNAL does.
// Many TED music players assume the KERNAL has already configured the system timer
// to fire at 50Hz (PAL) / 60Hz (NTSC) and just hook their code via $0314.
func (b *TEDPlaybackBus6502) EnableKERNALTimer() {
	// Plus/4 KERNAL configures Timer 1 to fire at the frame rate
	// PAL: 886724 Hz / 50 = 17734 cycles per tick
	// NTSC: 894886 Hz / 60 = 14915 cycles per tick
	var timerLatch uint16
	if b.ntsc {
		timerLatch = 14915
	} else {
		timerLatch = 17734
	}

	b.timer1Latch = timerLatch
	// Start counter at 1 so the first IRQ fires almost immediately
	// This simulates the timer already running when the music program starts
	b.timer1Counter = 1
	b.timer1Running = true

	// Enable Timer 1 interrupts (bit 3 of IRQ mask)
	b.irqMask = TED_IRQ_TIMER1

	// Set up default IRQ indirect vector ($0314/$0315)
	// Point to a minimal RTI stub if not already set by the program
	// The program will overwrite this with its own handler
	if b.ram[0x0314] == 0 && b.ram[0x0315] == 0 {
		// Point to an RTI instruction
		b.ram[0x0314] = 0x40 // Low byte of $0040
		b.ram[0x0315] = 0x00 // High byte
		b.ram[0x0040] = 0x40 // RTI at $0040
	}
}

// Read reads a byte from the given address.
func (b *TEDPlaybackBus6502) Read(addr uint16) byte {
	// TED registers at $FF00-$FF3F
	if addr >= 0xFF00 && addr <= 0xFF3F {
		return b.readTED(addr)
	}
	return b.ram[addr]
}

// Write writes a byte to the given address.
func (b *TEDPlaybackBus6502) Write(addr uint16, value byte) {
	// TED registers at $FF00-$FF3F
	if addr >= 0xFF00 && addr <= 0xFF3F {
		b.writeTED(addr, value)
		return
	}
	b.ram[addr] = value
}

// TED video/timing registers
const (
	PLUS4_TED_TIMER1_LO     = 0xFF00 // Timer 1 latch low byte
	PLUS4_TED_TIMER1_HI     = 0xFF01 // Timer 1 latch high byte
	PLUS4_TED_TIMER2_LO     = 0xFF02 // Timer 2 latch low byte
	PLUS4_TED_TIMER2_HI     = 0xFF03 // Timer 2 latch high byte
	PLUS4_TED_TIMER3_LO     = 0xFF04 // Timer 3 latch low byte
	PLUS4_TED_TIMER3_HI     = 0xFF05 // Timer 3 latch high byte
	PLUS4_TED_IRQ_FLAGS     = 0xFF09 // Interrupt flag register
	PLUS4_TED_IRQ_MASK      = 0xFF0A // Interrupt mask register
	PLUS4_TED_RASTER_CMP_LO = 0xFF0B // Raster compare low byte
	PLUS4_TED_RASTER_LO     = 0xFF1C // Raster line low byte
	PLUS4_TED_RASTER_HI     = 0xFF1D // Raster line high bit + flags
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
func (b *TEDPlaybackBus6502) readTED(addr uint16) byte {
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
	case PLUS4_TED_TIMER2_LO:
		return byte(b.timer2Counter & 0xFF)
	case PLUS4_TED_TIMER2_HI:
		return byte(b.timer2Counter >> 8)
	case PLUS4_TED_TIMER3_LO:
		return byte(b.timer3Counter & 0xFF)
	case PLUS4_TED_TIMER3_HI:
		return byte(b.timer3Counter >> 8)

	case PLUS4_TED_IRQ_FLAGS:
		return b.irqFlagsWithSummary()

	case PLUS4_TED_IRQ_MASK:
		return b.irqMask

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
func (b *TEDPlaybackBus6502) updateRasterPosition() {
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

func (b *TEDPlaybackBus6502) maxRasterLines() uint16 {
	if b.ntsc {
		return TED_NTSC_LINES
	}
	return TED_PAL_LINES
}

func (b *TEDPlaybackBus6502) rasterLineAt(cycles uint64) uint16 {
	frameCycles := cycles - b.frameCycle
	return uint16(frameCycles/TED_CYCLES_PER_LINE) % b.maxRasterLines()
}

func (b *TEDPlaybackBus6502) irqFlagsWithSummary() uint8 {
	flags := b.irqFlags & 0x7F
	active := flags & b.irqMask & (TED_IRQ_RASTER | TED_IRQ_TIMER1 | TED_IRQ_TIMER2 | TED_IRQ_TIMER3)
	if active != 0 {
		flags |= 0x80
	}
	return flags
}

func (b *TEDPlaybackBus6502) setIRQFlag(flag uint8) {
	b.irqFlags |= flag
	if b.irqMask&flag != 0 {
		b.irqPending = true
	}
}

func (b *TEDPlaybackBus6502) refreshIRQPending() {
	active := b.irqFlags & b.irqMask & (TED_IRQ_RASTER | TED_IRQ_TIMER1 | TED_IRQ_TIMER2 | TED_IRQ_TIMER3)
	if active != 0 {
		b.irqPending = true
	}
}

func (b *TEDPlaybackBus6502) rasterCrossed(prevLine, newLine uint16, wrapped bool) bool {
	maxLines := b.maxRasterLines()
	if b.rasterCmp >= maxLines {
		return false
	}

	target := b.rasterCmp
	if wrapped {
		return target > prevLine || target <= newLine
	}
	return target > prevLine && target <= newLine
}

func (b *TEDPlaybackBus6502) advanceRaster(prevCycles, newCycles uint64) {
	prevLine := b.rasterLineAt(prevCycles)
	newLine := b.rasterLineAt(newCycles)
	linesPerFrame := uint64(b.maxRasterLines())
	prevFrameCycles := prevCycles - b.frameCycle
	newFrameCycles := newCycles - b.frameCycle
	wrapped := newFrameCycles/TED_CYCLES_PER_LINE/linesPerFrame != prevFrameCycles/TED_CYCLES_PER_LINE/linesPerFrame

	if wrapped {
		b.cmpLatchedThisFrame = false
	}
	if !b.cmpLatchedThisFrame && b.rasterCrossed(prevLine, newLine, wrapped) {
		b.setIRQFlag(TED_IRQ_RASTER)
		b.cmpLatchedThisFrame = true
	}
	b.rasterLine = newLine
}

// writeTED writes to a TED register and captures events for sound registers
func (b *TEDPlaybackBus6502) writeTED(addr uint16, value byte) {
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
		if addr == PLUS4_TED_TIMER2_LO {
			b.timer2Latch = (b.timer2Latch & 0xFF00) | uint16(value)
		} else {
			b.timer2Latch = (b.timer2Latch & 0x00FF) | (uint16(value) << 8)
		}
		b.timer2Counter = b.timer2Latch
		b.timer2Running = true
		return
	case PLUS4_TED_TIMER3_LO, PLUS4_TED_TIMER3_HI:
		if addr == PLUS4_TED_TIMER3_LO {
			b.timer3Latch = (b.timer3Latch & 0xFF00) | uint16(value)
		} else {
			b.timer3Latch = (b.timer3Latch & 0x00FF) | (uint16(value) << 8)
		}
		b.timer3Counter = b.timer3Latch
		b.timer3Running = true
		return
	case PLUS4_TED_IRQ_FLAGS:
		// Writing 1s to $FF09 clears the corresponding interrupt flags
		b.irqFlags &^= value & 0x7E
		return
	case PLUS4_TED_IRQ_MASK:
		// IRQ mask register - controls which interrupts can fire
		b.irqMask = value
		b.rasterCmp = (b.rasterCmp & 0x0FF) | (uint16(value&0x01) << 8)
		b.refreshIRQPending()
		return
	case PLUS4_TED_RASTER_CMP_LO:
		b.rasterCmp = (b.rasterCmp & 0x100) | uint16(value)
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
func (b *TEDPlaybackBus6502) installVectors() {
	// KERNAL-like IRQ stub that properly saves/restores state
	// This mimics the Plus/4 KERNAL IRQ handler structure:
	// 1. Save registers (A, X, Y)
	// 2. Call user handler via indirect jump with return address on stack
	// 3. Restore registers
	// 4. RTI
	//
	// NOTE: Stub is placed at $FFC0 to avoid TED registers at $FF00-$FF3F
	//
	// Layout at $FFC0:
	// $FFC0: PHA          ; Save A
	// $FFC1: TXA
	// $FFC2: PHA          ; Save X
	// $FFC3: TYA
	// $FFC4: PHA          ; Save Y
	// $FFC5: JSR $FFD0    ; Call helper (pushes return addr $FFC8)
	// $FFC8: PLA
	// $FFC9: TAY          ; Restore Y
	// $FFCA: PLA
	// $FFCB: TAX          ; Restore X
	// $FFCC: PLA          ; Restore A
	// $FFCD: RTI
	// ...
	// $FFD0: JMP ($0314)  ; Jump to user handler (handler does RTS to $FFC8)

	b.ram[0xFFC0] = 0x48 // PHA
	b.ram[0xFFC1] = 0x8A // TXA
	b.ram[0xFFC2] = 0x48 // PHA
	b.ram[0xFFC3] = 0x98 // TYA
	b.ram[0xFFC4] = 0x48 // PHA
	b.ram[0xFFC5] = 0x20 // JSR $FFD0
	b.ram[0xFFC6] = 0xD0
	b.ram[0xFFC7] = 0xFF
	b.ram[0xFFC8] = 0x68 // PLA
	b.ram[0xFFC9] = 0xA8 // TAY
	b.ram[0xFFCA] = 0x68 // PLA
	b.ram[0xFFCB] = 0xAA // TAX
	b.ram[0xFFCC] = 0x68 // PLA
	b.ram[0xFFCD] = 0x40 // RTI
	// Helper at $FFD0
	b.ram[0xFFD0] = 0x6C // JMP ($0314)
	b.ram[0xFFD1] = 0x14
	b.ram[0xFFD2] = 0x03

	// Set up IRQ vector to point to stub at $FFC0
	b.ram[0xFFFE] = 0xC0 // IRQ vector low
	b.ram[0xFFFF] = 0xFF // IRQ vector high -> $FFC0

	// RESET vector
	b.ram[0xFFFC] = 0x00
	b.ram[0xFFFD] = 0x10 // Point to $1000

	// NMI vector (unused but set up)
	b.ram[0xFFFA] = 0x40 // RTI at $0040
	b.ram[0xFFFB] = 0x00
	b.ram[0x0040] = 0x40 // RTI instruction
}

// AddCycles advances the bus clock and updates timers
func (b *TEDPlaybackBus6502) AddCycles(cycles int) {
	if cycles <= 0 {
		return
	}
	prevCycles := b.cycles
	b.cycles += uint64(cycles)
	b.advanceRaster(prevCycles, b.cycles)

	// Update Timer 1 - counts down every cycle when enabled
	// (TED actually counts every other cycle, but we simplify)
	if b.timer1Running {
		for range cycles {
			if b.timer1Counter == 0 {
				// Timer underflowed - set IRQ flag bit 3
				b.setIRQFlag(TED_IRQ_TIMER1)
				// Reload from latch (tedplay uses t1start-1)
				b.timer1Counter = b.timer1Latch
			} else {
				b.timer1Counter--
			}
		}
	}
	b.advanceTimer(cycles, &b.timer2Counter, b.timer2Latch, b.timer2Running, TED_IRQ_TIMER2)
	b.advanceTimer(cycles, &b.timer3Counter, b.timer3Latch, b.timer3Running, TED_IRQ_TIMER3)
}

func (b *TEDPlaybackBus6502) advanceTimer(cycles int, counter *uint16, latch uint16, running bool, flag uint8) {
	if !running {
		return
	}
	for range cycles {
		if *counter == 0 {
			b.setIRQFlag(flag)
			*counter = latch
		} else {
			*counter--
		}
	}
}

// StartFrame starts a new frame and clears captured events
// Also generates a raster interrupt (TED fires raster IRQ at frame end)
func (b *TEDPlaybackBus6502) StartFrame() {
	b.frameCycle = b.cycles
	b.events = b.events[:0]
	b.rasterLine = 0
	b.cmpLatchedThisFrame = false
	b.frameCount++
}

// EnableRasterIRQ enables per-frame raster interrupts
// Call this after init is complete
func (b *TEDPlaybackBus6502) EnableRasterIRQ() {
	b.rasterIRQEnabled = true
}

// CollectEvents returns captured TED events and clears the list
func (b *TEDPlaybackBus6502) CollectEvents() []TEDEvent {
	if len(b.events) == 0 {
		return nil
	}
	events := make([]TEDEvent, len(b.events))
	copy(events, b.events)
	b.events = b.events[:0]
	return events
}

// LoadBinary loads binary data into memory at the specified address
func (b *TEDPlaybackBus6502) LoadBinary(addr uint16, data []byte) {
	for i, v := range data {
		target := addr + uint16(i)
		b.ram[target] = v
	}
}

// Reset resets the bus state
func (b *TEDPlaybackBus6502) Reset() {
	b.cycles = 0
	b.frameCycle = 0
	b.events = nil
	b.rasterLine = 0
	b.timer1Latch = 0
	b.timer1Counter = 0
	b.timer1Running = false
	b.timer1LastUpdate = 0
	b.timer2Latch = 0
	b.timer2Counter = 0
	b.timer2Running = false
	b.timer3Latch = 0
	b.timer3Counter = 0
	b.timer3Running = false
	b.irqFlags = 0
	b.irqMask = 0
	b.irqPending = false
	b.rasterCmp = 0
	b.cmpLatchedThisFrame = false

	for i := range b.tedRegs {
		b.tedRegs[i] = 0
	}

	b.installVectors()
}

// CheckIRQ returns true if an IRQ is pending and clears the pending flag
func (b *TEDPlaybackBus6502) CheckIRQ() bool {
	if b.irqPending {
		b.irqPending = false
		return true
	}
	return false
}

// GetCycles returns the total cycle count
func (b *TEDPlaybackBus6502) GetCycles() uint64 {
	return b.cycles
}

// GetFrameCycles returns cycles since frame start
func (b *TEDPlaybackBus6502) GetFrameCycles() uint64 {
	return b.cycles - b.frameCycle
}
