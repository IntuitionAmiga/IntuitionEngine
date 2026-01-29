package main

import (
	"fmt"
	"os"
)

const (
	defaultZ80LoadAddr = 0x0000
	z80AddressSpace    = 0x10000

	// Z80 Extended Bank Windows (same as 6502 for compatibility)
	// These allow Z80 programs to access >64KB data through banking
	Z80_BANK1_WINDOW_BASE = 0x2000 // Sprite data bank
	Z80_BANK2_WINDOW_BASE = 0x4000 // Font data bank
	Z80_BANK3_WINDOW_BASE = 0x6000 // General data/AY bank
	Z80_BANK_WINDOW_SIZE  = 0x2000 // 8KB per bank window

	// Z80 VRAM Bank Window (16KB)
	Z80_VRAM_BANK_WINDOW_BASE = 0x8000
	Z80_VRAM_BANK_WINDOW_SIZE = 0x4000

	// Bank control registers (memory-mapped, not I/O ports)
	Z80_BANK1_REG_LO   = 0xF700 // Sprite bank select (low byte)
	Z80_BANK1_REG_HI   = 0xF701 // Sprite bank select (high byte)
	Z80_BANK2_REG_LO   = 0xF702 // Font bank select (low byte)
	Z80_BANK2_REG_HI   = 0xF703 // Font bank select (high byte)
	Z80_BANK3_REG_LO   = 0xF704 // General bank select (low byte)
	Z80_BANK3_REG_HI   = 0xF705 // General bank select (high byte)
	Z80_VRAM_BANK_REG  = 0xF7F0 // VRAM bank select
	Z80_VRAM_BANK_RSVD = 0xF7F1 // Reserved
)

type CPUZ80Config struct {
	LoadAddr  uint16
	Entry     uint16
	VGAEngine *VGAEngine // Optional VGA engine for port I/O
}

type CPUZ80Runner struct {
	cpu      *CPU_Z80
	bus      *SystemBus
	loadAddr uint16
	entry    uint16
}

type Z80SystemBus struct {
	bus          *SystemBus
	psgRegSelect byte       // Currently selected PSG register for port I/O
	vgaEngine    *VGAEngine // VGA engine for port I/O access

	// Extended bank windows for IE80 support (same layout as 6502)
	vramBank    uint32
	vramEnabled bool
	bank1       uint32 // Bank number for $2000-$3FFF window
	bank2       uint32 // Bank number for $4000-$5FFF window
	bank3       uint32 // Bank number for $6000-$7FFF window
	bank1Enable bool   // Bank 1 enabled
	bank2Enable bool   // Bank 2 enabled
	bank3Enable bool   // Bank 3 enabled
}

func NewZ80SystemBus(bus *SystemBus) *Z80SystemBus {
	return &Z80SystemBus{bus: bus, psgRegSelect: 0}
}

// NewZ80SystemBusWithVGA creates a Z80 system bus with VGA engine support
func NewZ80SystemBusWithVGA(bus *SystemBus, vga *VGAEngine) *Z80SystemBus {
	return &Z80SystemBus{bus: bus, psgRegSelect: 0, vgaEngine: vga}
}

// translateIO8Bit converts 16-bit I/O addresses (0xF000-0xFFFF) to
// 32-bit addresses (0xF0000-0xF0FFF) for 8-bit CPU compatibility.
// Non-I/O addresses pass through unchanged.
func translateIO8Bit(addr uint16) uint32 {
	if addr >= 0xF000 {
		return 0xF0000 + uint32(addr-0xF000)
	}
	return uint32(addr)
}

func (b *Z80SystemBus) Read(addr uint16) byte {
	// Handle VRAM bank register reads
	if addr == Z80_VRAM_BANK_REG {
		return byte(b.vramBank & 0xFF)
	}
	if addr == Z80_VRAM_BANK_RSVD {
		return 0
	}

	// Handle extended bank register reads
	switch addr {
	case Z80_BANK1_REG_LO:
		return byte(b.bank1 & 0xFF)
	case Z80_BANK1_REG_HI:
		return byte((b.bank1 >> 8) & 0xFF)
	case Z80_BANK2_REG_LO:
		return byte(b.bank2 & 0xFF)
	case Z80_BANK2_REG_HI:
		return byte((b.bank2 >> 8) & 0xFF)
	case Z80_BANK3_REG_LO:
		return byte(b.bank3 & 0xFF)
	case Z80_BANK3_REG_HI:
		return byte((b.bank3 >> 8) & 0xFF)
	}

	// Handle extended bank window reads (IE80 mode)
	if translated, ok := b.translateExtendedBank(addr); ok {
		return b.bus.Read8(translated)
	}

	// Handle VRAM bank window reads
	if translated, ok := b.translateVRAM(addr); ok {
		return b.bus.Read8(translated)
	}

	return b.bus.Read8(translateIO8Bit(addr))
}

func (b *Z80SystemBus) Write(addr uint16, value byte) {
	// Handle VRAM bank register writes
	if addr == Z80_VRAM_BANK_REG {
		b.vramBank = uint32(value)
		b.vramEnabled = true
		return
	}
	if addr == Z80_VRAM_BANK_RSVD {
		return
	}

	// Handle extended bank register writes
	switch addr {
	case Z80_BANK1_REG_LO:
		b.bank1 = (b.bank1 & 0xFF00) | uint32(value)
		b.bank1Enable = true
		return
	case Z80_BANK1_REG_HI:
		b.bank1 = (b.bank1 & 0x00FF) | (uint32(value) << 8)
		b.bank1Enable = true
		return
	case Z80_BANK2_REG_LO:
		b.bank2 = (b.bank2 & 0xFF00) | uint32(value)
		b.bank2Enable = true
		return
	case Z80_BANK2_REG_HI:
		b.bank2 = (b.bank2 & 0x00FF) | (uint32(value) << 8)
		b.bank2Enable = true
		return
	case Z80_BANK3_REG_LO:
		b.bank3 = (b.bank3 & 0xFF00) | uint32(value)
		b.bank3Enable = true
		return
	case Z80_BANK3_REG_HI:
		b.bank3 = (b.bank3 & 0x00FF) | (uint32(value) << 8)
		b.bank3Enable = true
		return
	}

	// Handle extended bank window writes (IE80 mode)
	if translated, ok := b.translateExtendedBank(addr); ok {
		b.bus.Write8(translated, value)
		return
	}

	// Handle VRAM bank window writes
	if translated, ok := b.translateVRAM(addr); ok {
		b.bus.Write8(translated, value)
		return
	}

	b.bus.Write8(translateIO8Bit(addr), value)
}

// translateExtendedBank translates addresses in the extended bank windows
// to their actual 32-bit addresses.
//
// Bank window layout:
// - $2000-$3FFF: Bank 1 (sprite data)
// - $4000-$5FFF: Bank 2 (font data)
// - $6000-$7FFF: Bank 3 (general data)
//
// Each bank window maps to:
// base_address = bank_number * 8KB
// actual_address = base_address + (addr - window_base)
func (b *Z80SystemBus) translateExtendedBank(addr uint16) (uint32, bool) {
	// Check Bank 1 window ($2000-$3FFF)
	if b.bank1Enable && addr >= Z80_BANK1_WINDOW_BASE && addr < Z80_BANK1_WINDOW_BASE+Z80_BANK_WINDOW_SIZE {
		offset := uint32(addr - Z80_BANK1_WINDOW_BASE)
		translated := (b.bank1 * Z80_BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	// Check Bank 2 window ($4000-$5FFF)
	if b.bank2Enable && addr >= Z80_BANK2_WINDOW_BASE && addr < Z80_BANK2_WINDOW_BASE+Z80_BANK_WINDOW_SIZE {
		offset := uint32(addr - Z80_BANK2_WINDOW_BASE)
		translated := (b.bank2 * Z80_BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	// Check Bank 3 window ($6000-$7FFF)
	if b.bank3Enable && addr >= Z80_BANK3_WINDOW_BASE && addr < Z80_BANK3_WINDOW_BASE+Z80_BANK_WINDOW_SIZE {
		offset := uint32(addr - Z80_BANK3_WINDOW_BASE)
		translated := (b.bank3 * Z80_BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	return 0, false
}

// translateVRAM translates addresses in the VRAM bank window to their
// actual 32-bit addresses. Supports both VGA text buffer (0xB8000) and
// main VRAM (0x100000+) depending on bank value.
func (b *Z80SystemBus) translateVRAM(addr uint16) (uint32, bool) {
	if !b.vramEnabled {
		return 0, false
	}

	if addr < Z80_VRAM_BANK_WINDOW_BASE || addr >= Z80_VRAM_BANK_WINDOW_BASE+Z80_VRAM_BANK_WINDOW_SIZE {
		return 0, false
	}

	// Calculate 32-bit address: bank * 16KB + offset within window
	// This allows accessing:
	// - VGA text buffer at 0xB8000 (bank 0x2E = 46)
	// - Main VRAM at 0x100000+ (bank 0x40+ = 64+)
	translated := (b.vramBank * Z80_VRAM_BANK_WINDOW_SIZE) +
		uint32(addr-Z80_VRAM_BANK_WINDOW_BASE)

	// Allow access to VGA text buffer (0xB8000-0xBFFFF) or main VRAM (0x100000+)
	isVGAText := translated >= VGA_TEXT_WINDOW && translated < VGA_TEXT_WINDOW+VGA_TEXT_SIZE
	isMainVRAM := translated >= uint32(VRAM_START) && translated < uint32(VRAM_START+VRAM_SIZE)

	if !isVGAText && !isMainVRAM {
		return 0, false
	}

	return translated, true
}

// ResetBank resets all bank registers to their default state
func (b *Z80SystemBus) ResetBank() {
	b.vramBank = 0
	b.vramEnabled = false
	b.bank1 = 0
	b.bank2 = 0
	b.bank3 = 0
	b.bank1Enable = false
	b.bank2Enable = false
	b.bank3Enable = false
}

func (b *Z80SystemBus) In(port uint16) byte {
	lowPort := byte(port)

	// Handle PSG port I/O
	switch lowPort {
	case Z80_PSG_PORT_SELECT:
		// Read returns the currently selected register
		return b.psgRegSelect
	case Z80_PSG_PORT_DATA:
		// Read from currently selected PSG register
		if b.psgRegSelect < PSG_REG_COUNT {
			return b.bus.Read8(PSG_BASE + uint32(b.psgRegSelect))
		}
		return 0
	}

	// Handle VGA port I/O (0xA0-0xAA)
	if b.vgaEngine != nil {
		switch lowPort {
		case Z80_VGA_PORT_MODE:
			return byte(b.vgaEngine.HandleRead(VGA_MODE))
		case Z80_VGA_PORT_STATUS:
			return byte(b.vgaEngine.HandleRead(VGA_STATUS))
		case Z80_VGA_PORT_CTRL:
			return byte(b.vgaEngine.HandleRead(VGA_CTRL))
		case Z80_VGA_PORT_SEQ_IDX:
			return byte(b.vgaEngine.HandleRead(VGA_SEQ_INDEX))
		case Z80_VGA_PORT_SEQ_DATA:
			return byte(b.vgaEngine.HandleRead(VGA_SEQ_DATA))
		case Z80_VGA_PORT_CRTC_IDX:
			return byte(b.vgaEngine.HandleRead(VGA_CRTC_INDEX))
		case Z80_VGA_PORT_CRTC_DATA:
			return byte(b.vgaEngine.HandleRead(VGA_CRTC_DATA))
		case Z80_VGA_PORT_GC_IDX:
			return byte(b.vgaEngine.HandleRead(VGA_GC_INDEX))
		case Z80_VGA_PORT_GC_DATA:
			return byte(b.vgaEngine.HandleRead(VGA_GC_DATA))
		case Z80_VGA_PORT_DAC_WIDX:
			return byte(b.vgaEngine.HandleRead(VGA_DAC_WINDEX))
		case Z80_VGA_PORT_DAC_DATA:
			return byte(b.vgaEngine.HandleRead(VGA_DAC_DATA))
		}
	}

	return b.bus.Read8(translateIO8Bit(port))
}

func (b *Z80SystemBus) Out(port uint16, value byte) {
	lowPort := byte(port)

	// Handle PSG port I/O
	switch lowPort {
	case Z80_PSG_PORT_SELECT:
		// Select PSG register (mask to valid range)
		b.psgRegSelect = value & 0x0F
		return
	case Z80_PSG_PORT_DATA:
		// Write to currently selected PSG register
		if b.psgRegSelect < PSG_REG_COUNT {
			b.bus.Write8(PSG_BASE+uint32(b.psgRegSelect), value)
		}
		return
	}

	// Handle VGA port I/O (0xA0-0xAA)
	if b.vgaEngine != nil {
		switch lowPort {
		case Z80_VGA_PORT_MODE:
			b.vgaEngine.HandleWrite(VGA_MODE, uint32(value))
			return
		case Z80_VGA_PORT_STATUS:
			// Status is read-only, but accept writes silently
			return
		case Z80_VGA_PORT_CTRL:
			b.vgaEngine.HandleWrite(VGA_CTRL, uint32(value))
			return
		case Z80_VGA_PORT_SEQ_IDX:
			b.vgaEngine.HandleWrite(VGA_SEQ_INDEX, uint32(value))
			return
		case Z80_VGA_PORT_SEQ_DATA:
			b.vgaEngine.HandleWrite(VGA_SEQ_DATA, uint32(value))
			return
		case Z80_VGA_PORT_CRTC_IDX:
			b.vgaEngine.HandleWrite(VGA_CRTC_INDEX, uint32(value))
			return
		case Z80_VGA_PORT_CRTC_DATA:
			b.vgaEngine.HandleWrite(VGA_CRTC_DATA, uint32(value))
			return
		case Z80_VGA_PORT_GC_IDX:
			b.vgaEngine.HandleWrite(VGA_GC_INDEX, uint32(value))
			return
		case Z80_VGA_PORT_GC_DATA:
			b.vgaEngine.HandleWrite(VGA_GC_DATA, uint32(value))
			return
		case Z80_VGA_PORT_DAC_WIDX:
			b.vgaEngine.HandleWrite(VGA_DAC_WINDEX, uint32(value))
			return
		case Z80_VGA_PORT_DAC_DATA:
			b.vgaEngine.HandleWrite(VGA_DAC_DATA, uint32(value))
			return
		}
	}

	b.bus.Write8(translateIO8Bit(port), value)
}

func (b *Z80SystemBus) Tick(cycles int) {}

func NewCPUZ80Runner(bus *SystemBus, config CPUZ80Config) *CPUZ80Runner {
	loadAddr := config.LoadAddr
	if loadAddr == 0 {
		loadAddr = defaultZ80LoadAddr
	}

	// Create Z80 system bus with optional VGA engine for port I/O
	var z80Bus *Z80SystemBus
	if config.VGAEngine != nil {
		z80Bus = NewZ80SystemBusWithVGA(bus, config.VGAEngine)
	} else {
		z80Bus = NewZ80SystemBus(bus)
	}

	return &CPUZ80Runner{
		cpu:      NewCPU_Z80(z80Bus),
		bus:      bus,
		loadAddr: loadAddr,
		entry:    config.Entry,
	}
}

func (r *CPUZ80Runner) LoadProgram(filename string) error {
	program, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	r.bus.Reset()

	entry := r.entry
	if entry == 0 {
		entry = r.loadAddr
	}

	endAddr := uint32(r.loadAddr) + uint32(len(program))

	// Allow loading larger binaries with embedded data for DMA devices (blitter, audio).
	// The Z80 CPU can only address 64KB, but embedded data beyond that is accessed
	// by hardware peripherals through the full 16MB bus.
	if endAddr > DEFAULT_MEMORY_SIZE {
		return fmt.Errorf("z80 program too large: end=0x%X, limit=0x%X", endAddr, DEFAULT_MEMORY_SIZE)
	}

	for i, value := range program {
		r.bus.Write8(uint32(r.loadAddr)+uint32(i), value)
	}

	r.cpu.Reset()
	r.cpu.PC = entry
	return nil
}

func (r *CPUZ80Runner) Reset() {
	r.cpu.Reset()
}

func (r *CPUZ80Runner) Execute() {
	r.cpu.Execute()
}

func (r *CPUZ80Runner) CPU() *CPU_Z80 {
	return r.cpu
}
