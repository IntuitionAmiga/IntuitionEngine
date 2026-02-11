// sap_6502_bus.go - 6502 bus for SAP file playback with POKEY event capture
//
// This bus emulates the Atari 800/XL/XE memory map for SAP player execution:
// - Full 64KB RAM (SAP players don't use ROM)
// - POKEY at $D200-$D209 (with mirroring)
// - Stereo POKEY at $D200 and $D210 (when enabled)
// - GTIA at $D000-$D01F (PAL/NTSC detection)
// - ANTIC at $D400-$D4FF (VCOUNT, WSYNC)

package main

const (
	// Atari memory-mapped I/O regions
	atariGTIABase  = 0xD000
	atariGTIAEnd   = 0xD0FF
	atariPOKEYBase = 0xD200
	atariPOKEYEnd  = 0xD2FF
	atariANTICBase = 0xD400
	atariANTICEnd  = 0xD4FF

	// Cycles per scanline
	atariCyclesPerScanline = 114

	// PAL/NTSC scanline counts
	atariPALScanlines  = 312
	atariNTSCScanlines = 262
)

// SAPPOKEYEvent represents a single POKEY register write captured during playback
type SAPPOKEYEvent struct {
	Cycle  uint64 // CPU cycle when write occurred
	Sample uint64 // Audio sample position (computed from cycle)
	Reg    uint8  // POKEY register (0-9)
	Value  uint8  // Value written
	Chip   int    // Chip number (0=left/mono, 1=right for stereo)
}

// SAPPlaybackBus6502 implements the memory bus for SAP 6502 player execution
type SAPPlaybackBus6502 struct {
	ram [0x10000]byte // Full 64KB RAM

	// POKEY state
	stereo     bool            // True for stereo (dual POKEY)
	pokeyRegs  [2][10]uint8    // Register state for each POKEY chip
	events     []SAPPOKEYEvent // Captured POKEY writes
	cycles     uint64          // Current CPU cycle
	frameCycle uint64          // Cycle at start of current frame

	// Video timing
	ntsc     bool // True for NTSC, false for PAL
	scanline int  // Current scanline
	random   uint32

	// Scanline cycle tracking
	scanlineCycle int
}

// newSAPPlaybackBus6502 creates a new 6502 bus for SAP playback
func newSAPPlaybackBus6502(stereo, ntsc bool) *SAPPlaybackBus6502 {
	bus := &SAPPlaybackBus6502{
		stereo: stereo,
		ntsc:   ntsc,
		random: 0x12345678, // Initial random seed
	}
	return bus
}

// Read reads a byte from the given address
func (b *SAPPlaybackBus6502) Read(addr uint16) byte {
	// Handle memory-mapped I/O regions
	switch {
	case addr >= atariGTIABase && addr <= atariGTIAEnd:
		return b.readGTIA(addr)
	case addr >= atariPOKEYBase && addr <= atariPOKEYEnd:
		return b.readPOKEY(addr)
	case addr >= atariANTICBase && addr <= atariANTICEnd:
		return b.readANTIC(addr)
	default:
		return b.ram[addr]
	}
}

// Write writes a byte to the given address
func (b *SAPPlaybackBus6502) Write(addr uint16, value byte) {
	// Handle memory-mapped I/O regions
	switch {
	case addr >= atariGTIABase && addr <= atariGTIAEnd:
		b.writeGTIA(addr, value)
	case addr >= atariPOKEYBase && addr <= atariPOKEYEnd:
		b.writePOKEY(addr, value)
	case addr >= atariANTICBase && addr <= atariANTICEnd:
		b.writeANTIC(addr, value)
	default:
		b.ram[addr] = value
	}
}

// readGTIA handles reads from GTIA registers
func (b *SAPPlaybackBus6502) readGTIA(addr uint16) byte {
	reg := byte(addr & 0x1F)
	switch reg {
	case 0x14: // CONSOL - PAL/NTSC detection
		if b.ntsc {
			return 0x0F // NTSC
		}
		return 0x01 // PAL
	default:
		return 0xFF // Open bus
	}
}

// writeGTIA handles writes to GTIA registers (mostly ignored for SAP)
func (b *SAPPlaybackBus6502) writeGTIA(addr uint16, value byte) {
	// GTIA writes ignored for audio playback
}

// readPOKEY handles reads from POKEY registers
func (b *SAPPlaybackBus6502) readPOKEY(addr uint16) byte {
	reg := byte(addr & 0x0F)

	// Determine which chip (for stereo)
	chip := 0
	if b.stereo && (addr&0x10) != 0 {
		chip = 1
	}

	switch reg {
	case 0x0A: // RANDOM - pseudo-random number
		b.random = b.random*1103515245 + 12345
		return byte(b.random >> 16)
	case 0x0F: // SKSTAT - keyboard/serial status
		return 0xFF // All bits high = no input
	default:
		if reg < 10 {
			return b.pokeyRegs[chip][reg]
		}
		return 0xFF
	}
}

// writePOKEY handles writes to POKEY registers and captures events
func (b *SAPPlaybackBus6502) writePOKEY(addr uint16, value byte) {
	// Determine register and chip
	offset := addr - atariPOKEYBase
	reg := byte(offset & 0x0F)

	// Determine which chip
	chip := 0
	if b.stereo {
		// In stereo mode: $D200-$D20F = left, $D210-$D21F = right
		if (offset & 0x10) != 0 {
			chip = 1
		}
	}

	// Only handle registers 0-9
	if reg >= 10 {
		return
	}

	// Store register value
	b.pokeyRegs[chip][reg] = value

	// Capture event
	b.events = append(b.events, SAPPOKEYEvent{
		Cycle: b.cycles,
		Reg:   reg,
		Value: value,
		Chip:  chip,
	})
}

// readANTIC handles reads from ANTIC registers
func (b *SAPPlaybackBus6502) readANTIC(addr uint16) byte {
	reg := byte(addr & 0x0F)
	switch reg {
	case 0x0B: // VCOUNT - vertical line counter (divided by 2)
		return byte(b.scanline / 2)
	case 0x0F: // NMIST - NMI status
		return 0x00
	default:
		return 0xFF
	}
}

// writeANTIC handles writes to ANTIC registers
func (b *SAPPlaybackBus6502) writeANTIC(addr uint16, value byte) {
	reg := byte(addr & 0x0F)
	switch reg {
	case 0x0A: // WSYNC - wait for horizontal sync
		// Advance to next scanline
		b.advanceToNextScanline()
	case 0x0E: // NMIEN - NMI enable
		// Ignored for SAP playback
	}
}

// advanceToNextScanline advances to the next scanline boundary
func (b *SAPPlaybackBus6502) advanceToNextScanline() {
	// Calculate remaining cycles in current scanline
	remaining := atariCyclesPerScanline - b.scanlineCycle
	if remaining > 0 {
		b.cycles += uint64(remaining)
		b.scanlineCycle = 0
		b.scanline++
	}
}

// AddCycles adds CPU cycles and updates timing state
func (b *SAPPlaybackBus6502) AddCycles(cycles int) {
	b.cycles += uint64(cycles)
	b.scanlineCycle += cycles

	// Advance scanlines
	for b.scanlineCycle >= atariCyclesPerScanline {
		b.scanlineCycle -= atariCyclesPerScanline
		b.scanline++
	}
}

// StartFrame resets frame state and clears captured events
func (b *SAPPlaybackBus6502) StartFrame() {
	b.frameCycle = b.cycles
	b.events = b.events[:0] // Clear events slice but keep capacity
}

// CollectEvents returns and clears the captured POKEY events
// CollectEvents returns events captured this frame (allocates new slice).
// DEPRECATED: Use GetEvents() for zero-allocation access.
func (b *SAPPlaybackBus6502) CollectEvents() []SAPPOKEYEvent {
	events := make([]SAPPOKEYEvent, len(b.events))
	copy(events, b.events)
	b.events = b.events[:0]
	return events
}

// GetEvents returns a direct reference to the internal events slice.
// The caller must not retain this slice after the next StartFrame call.
// This is the zero-allocation path for performance-critical code.
func (b *SAPPlaybackBus6502) GetEvents() []SAPPOKEYEvent {
	return b.events
}

// ClearEvents clears the internal events buffer without allocation.
func (b *SAPPlaybackBus6502) ClearEvents() {
	b.events = b.events[:0]
}

// GetFrameCycleStart returns the cycle count at the start of the current frame.
func (b *SAPPlaybackBus6502) GetFrameCycleStart() uint64 {
	return b.frameCycle
}

// LoadBlocks loads SAP binary blocks into RAM
func (b *SAPPlaybackBus6502) LoadBlocks(blocks []SAPBlock) {
	for _, block := range blocks {
		for i, v := range block.Data {
			addr := block.Start + uint16(i)
			if addr <= block.End {
				b.ram[addr] = v
			}
		}
	}
}

// Reset resets the bus state
func (b *SAPPlaybackBus6502) Reset() {
	b.cycles = 0
	b.frameCycle = 0
	b.scanline = 0
	b.scanlineCycle = 0
	b.events = nil

	// Clear POKEY registers
	for chip := range 2 {
		for reg := range 10 {
			b.pokeyRegs[chip][reg] = 0
		}
	}
}

// GetCycles returns the current cycle count
func (b *SAPPlaybackBus6502) GetCycles() uint64 {
	return b.cycles
}

// GetFrameCycles returns cycles elapsed since StartFrame
func (b *SAPPlaybackBus6502) GetFrameCycles() uint64 {
	return b.cycles - b.frameCycle
}

// GetScanline returns the current scanline
func (b *SAPPlaybackBus6502) GetScanline() int {
	return b.scanline
}

// SetScanline sets the current scanline (for testing)
func (b *SAPPlaybackBus6502) SetScanline(scanline int) {
	b.scanline = scanline
}

// MaxScanlines returns the max scanlines for the current video mode
func (b *SAPPlaybackBus6502) MaxScanlines() int {
	if b.ntsc {
		return atariNTSCScanlines
	}
	return atariPALScanlines
}

// GetRAM returns a pointer to the RAM array (for direct CPU access)
func (b *SAPPlaybackBus6502) GetRAM() *[0x10000]byte {
	return &b.ram
}
