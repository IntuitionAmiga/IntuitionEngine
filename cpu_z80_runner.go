package main

import (
	"fmt"
	"os"
	"sync"
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
	LoadAddr     uint16
	Entry        uint16
	VGAEngine    *VGAEngine    // Optional VGA engine for port I/O
	VoodooEngine *VoodooEngine // Optional Voodoo engine for port I/O
}

type CPUZ80Runner struct {
	cpu         *CPU_Z80
	bus         *MachineBus
	loadAddr    uint16
	entry       uint16
	PerfEnabled bool

	execMu     sync.Mutex
	execDone   chan struct{}
	execActive bool
}

type Z80BusAdapter struct {
	bus            *MachineBus
	psgRegSelect   byte       // Currently selected PSG register for port I/O
	sidRegSelect   byte       // Currently selected SID register for port I/O
	pokeyRegSelect byte       // Currently selected POKEY register for port I/O
	tedRegSelect   byte       // Currently selected TED register for port I/O
	anticRegSelect byte       // Currently selected ANTIC register for port I/O
	gtiaRegSelect  byte       // Currently selected GTIA register for port I/O
	vgaEngine      *VGAEngine // VGA engine for port I/O access

	// Voodoo 32-bit register access via 8-bit ports
	voodooAddr   uint16        // Target register offset from VOODOO_BASE
	voodooData   [4]byte       // 32-bit data accumulator (little-endian)
	voodooTexSrc uint16        // Texture source address in Z80 RAM
	voodooEngine *VoodooEngine // Voodoo engine for port I/O access

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

func NewZ80BusAdapter(bus *MachineBus) *Z80BusAdapter {
	return &Z80BusAdapter{bus: bus, psgRegSelect: 0}
}

// NewZ80BusAdapterWithVGA creates a Z80 system bus with VGA engine support
func NewZ80BusAdapterWithVGA(bus *MachineBus, vga *VGAEngine) *Z80BusAdapter {
	return &Z80BusAdapter{bus: bus, psgRegSelect: 0, vgaEngine: vga}
}

// NewZ80BusAdapterWithVoodoo creates a Z80 system bus with VGA and Voodoo engine support
func NewZ80BusAdapterWithVoodoo(bus *MachineBus, vga *VGAEngine, voodoo *VoodooEngine) *Z80BusAdapter {
	return &Z80BusAdapter{bus: bus, psgRegSelect: 0, vgaEngine: vga, voodooEngine: voodoo}
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

func (b *Z80BusAdapter) Read(addr uint16) byte {
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

	// Handle coprocessor gateway window (0xF200-0xF23F → COPROC_BASE on bus)
	if addr >= COPROC_GATEWAY_BASE && addr <= COPROC_GATEWAY_END {
		return b.bus.Read8(COPROC_BASE + uint32(addr-COPROC_GATEWAY_BASE))
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

func (b *Z80BusAdapter) Write(addr uint16, value byte) {
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

	// Handle coprocessor gateway window (0xF200-0xF23F → COPROC_BASE on bus)
	if addr >= COPROC_GATEWAY_BASE && addr <= COPROC_GATEWAY_END {
		b.bus.Write8(COPROC_BASE+uint32(addr-COPROC_GATEWAY_BASE), value)
		return
	}

	// Handle extended bank window writes (IE80 mode)
	if translated, ok := b.translateExtendedBank(addr); ok {
		b.bus.Write8(translated, value)
		return
	}

	// Handle VRAM bank window writes
	// Use WriteMemoryDirect to bypass VideoChip handler, which does
	// 32-bit writes even for single bytes (corrupting adjacent bytes)
	if translated, ok := b.translateVRAM(addr); ok {
		b.bus.WriteMemoryDirect(translated, value)
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
func (b *Z80BusAdapter) translateExtendedBank(addr uint16) (uint32, bool) {
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
func (b *Z80BusAdapter) translateVRAM(addr uint16) (uint32, bool) {
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
func (b *Z80BusAdapter) ResetBank() {
	b.vramBank = 0
	b.vramEnabled = false
	b.bank1 = 0
	b.bank2 = 0
	b.bank3 = 0
	b.bank1Enable = false
	b.bank2Enable = false
	b.bank3Enable = false
}

func (b *Z80BusAdapter) In(port uint16) byte {
	lowPort := byte(port)

	// Handle PSG port I/O (0xF0-0xF1)
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

	// Handle SID port I/O (0xE0-0xE1)
	switch lowPort {
	case Z80_SID_PORT_SELECT:
		return b.sidRegSelect
	case Z80_SID_PORT_DATA:
		if b.sidRegSelect < SID_REG_COUNT {
			return b.bus.Read8(SID_BASE + uint32(b.sidRegSelect))
		}
		return 0
	}

	// Handle POKEY port I/O (0xD0-0xD1)
	switch lowPort {
	case Z80_POKEY_PORT_SELECT:
		return b.pokeyRegSelect
	case Z80_POKEY_PORT_DATA:
		if b.pokeyRegSelect < POKEY_REG_COUNT {
			return b.bus.Read8(POKEY_BASE + uint32(b.pokeyRegSelect))
		}
		return 0
	}

	// Handle TED port I/O (0xF2-0xF3)
	// Register indices 0x00-0x05 = TED audio, 0x20-0x2F = TED video
	switch lowPort {
	case Z80_TED_PORT_SELECT:
		return b.tedRegSelect
	case Z80_TED_PORT_DATA:
		if b.tedRegSelect < TED_REG_COUNT {
			// TED audio registers (0x00-0x05)
			return b.bus.Read8(TED_BASE + uint32(b.tedRegSelect))
		} else if b.tedRegSelect >= Z80_TED_V_INDEX_BASE && b.tedRegSelect <= Z80_TED_V_INDEX_END {
			// TED video registers (0x20-0x2F) - map to 4-byte aligned addresses
			vidReg := uint32(b.tedRegSelect - Z80_TED_V_INDEX_BASE)
			return b.bus.Read8(TED_VIDEO_BASE + (vidReg * 4))
		}
		return 0
	}

	// Handle ANTIC port I/O (0xD4-0xD5)
	switch lowPort {
	case Z80_ANTIC_PORT_SELECT:
		return b.anticRegSelect
	case Z80_ANTIC_PORT_DATA:
		if b.anticRegSelect < ANTIC_REG_COUNT {
			// ANTIC registers are 4-byte aligned
			return b.bus.Read8(ANTIC_BASE + uint32(b.anticRegSelect)*4)
		}
		return 0
	}

	// Handle GTIA port I/O (0xD6-0xD7)
	switch lowPort {
	case Z80_GTIA_PORT_SELECT:
		return b.gtiaRegSelect
	case Z80_GTIA_PORT_DATA:
		if b.gtiaRegSelect < GTIA_REG_COUNT {
			// GTIA registers are 4-byte aligned
			return b.bus.Read8(GTIA_BASE + uint32(b.gtiaRegSelect)*4)
		}
		return 0
	}

	// Handle ULA port I/O (0xFE)
	if lowPort == Z80_ULA_PORT {
		// Read returns border color from ULA_BORDER register
		return b.bus.Read8(ULA_BORDER) & 0x07
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

	// Handle Voodoo port I/O (0xB0-0xB7)
	if b.voodooEngine != nil {
		switch lowPort {
		case Z80_VOODOO_PORT_ADDR_LO:
			return byte(b.voodooAddr & 0xFF)
		case Z80_VOODOO_PORT_ADDR_HI:
			return byte(b.voodooAddr >> 8)
		case Z80_VOODOO_PORT_DATA0:
			return b.voodooData[0]
		case Z80_VOODOO_PORT_DATA1:
			return b.voodooData[1]
		case Z80_VOODOO_PORT_DATA2:
			return b.voodooData[2]
		case Z80_VOODOO_PORT_DATA3:
			return b.voodooData[3]
		case Z80_VOODOO_PORT_TEXSRC_LO:
			return byte(b.voodooTexSrc & 0xFF)
		case Z80_VOODOO_PORT_TEXSRC_HI:
			return byte(b.voodooTexSrc >> 8)
		}
	}

	return b.bus.Read8(translateIO8Bit(port))
}

func (b *Z80BusAdapter) Out(port uint16, value byte) {
	lowPort := byte(port)

	// Handle PSG port I/O (0xF0-0xF1)
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

	// Handle SID port I/O (0xE0-0xE1)
	switch lowPort {
	case Z80_SID_PORT_SELECT:
		b.sidRegSelect = value & 0x1F // SID has ~26 registers
		return
	case Z80_SID_PORT_DATA:
		if b.sidRegSelect < SID_REG_COUNT {
			b.bus.Write8(SID_BASE+uint32(b.sidRegSelect), value)
		}
		return
	}

	// Handle POKEY port I/O (0xD0-0xD1)
	switch lowPort {
	case Z80_POKEY_PORT_SELECT:
		b.pokeyRegSelect = value & 0x0F // POKEY has 10 registers
		return
	case Z80_POKEY_PORT_DATA:
		if b.pokeyRegSelect < POKEY_REG_COUNT {
			b.bus.Write8(POKEY_BASE+uint32(b.pokeyRegSelect), value)
		}
		return
	}

	// Handle TED port I/O (0xF2-0xF3)
	// Register indices 0x00-0x05 = TED audio, 0x20-0x2F = TED video
	switch lowPort {
	case Z80_TED_PORT_SELECT:
		b.tedRegSelect = value & 0x3F // Allow 0x00-0x3F for audio + video indices
		return
	case Z80_TED_PORT_DATA:
		if b.tedRegSelect < TED_REG_COUNT {
			// TED audio registers (0x00-0x05)
			b.bus.Write8(TED_BASE+uint32(b.tedRegSelect), value)
		} else if b.tedRegSelect >= Z80_TED_V_INDEX_BASE && b.tedRegSelect <= Z80_TED_V_INDEX_END {
			// TED video registers (0x20-0x2F) - map to 4-byte aligned addresses
			vidReg := uint32(b.tedRegSelect - Z80_TED_V_INDEX_BASE)
			b.bus.Write8(TED_VIDEO_BASE+(vidReg*4), value)
		}
		return
	}

	// Handle ANTIC port I/O (0xD4-0xD5)
	switch lowPort {
	case Z80_ANTIC_PORT_SELECT:
		b.anticRegSelect = value & 0x0F // 16 ANTIC registers
		return
	case Z80_ANTIC_PORT_DATA:
		if b.anticRegSelect < ANTIC_REG_COUNT {
			// ANTIC registers are 4-byte aligned
			b.bus.Write8(ANTIC_BASE+uint32(b.anticRegSelect)*4, value)
		}
		return
	}

	// Handle GTIA port I/O (0xD6-0xD7)
	switch lowPort {
	case Z80_GTIA_PORT_SELECT:
		b.gtiaRegSelect = value & 0x0F // 12 GTIA registers
		return
	case Z80_GTIA_PORT_DATA:
		if b.gtiaRegSelect < GTIA_REG_COUNT {
			// GTIA registers are 4-byte aligned
			b.bus.Write8(GTIA_BASE+uint32(b.gtiaRegSelect)*4, value)
		}
		return
	}

	// Handle ULA port I/O (0xFE)
	if lowPort == Z80_ULA_PORT {
		// Write sets border color (bits 0-2) to ULA_BORDER register
		b.bus.Write8(ULA_BORDER, value&0x07)
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

	// Handle Voodoo port I/O (0xB0-0xB7)
	// Allows Z80 to write 32-bit values to Voodoo registers via 8-bit port interface
	if b.voodooEngine != nil {
		switch lowPort {
		case Z80_VOODOO_PORT_ADDR_LO:
			b.voodooAddr = (b.voodooAddr & 0xFF00) | uint16(value)
			return
		case Z80_VOODOO_PORT_ADDR_HI:
			b.voodooAddr = (b.voodooAddr & 0x00FF) | (uint16(value) << 8)
			return
		case Z80_VOODOO_PORT_DATA0:
			b.voodooData[0] = value
			return
		case Z80_VOODOO_PORT_DATA1:
			b.voodooData[1] = value
			return
		case Z80_VOODOO_PORT_DATA2:
			b.voodooData[2] = value
			return
		case Z80_VOODOO_PORT_DATA3:
			// Writing DATA3 triggers the 32-bit write to Voodoo
			b.voodooData[3] = value
			data32 := uint32(b.voodooData[0]) |
				(uint32(b.voodooData[1]) << 8) |
				(uint32(b.voodooData[2]) << 16) |
				(uint32(b.voodooData[3]) << 24)
			addr := VOODOO_BASE + uint32(b.voodooAddr)

			// Special handling for texture upload - copy from Z80 RAM to texture memory
			if addr == VOODOO_TEX_UPLOAD && b.voodooTexSrc != 0 {
				// Copy texture data from Z80 RAM to Voodoo texture memory
				texSize := b.voodooEngine.textureWidth * b.voodooEngine.textureHeight * 4
				if texSize > 0 && texSize <= VOODOO_TEXMEM_SIZE {
					for i := range texSize {
						b.voodooEngine.textureMemory[i] = b.bus.Read8(uint32(b.voodooTexSrc) + uint32(i))
					}
				}
			}
			b.voodooEngine.HandleWrite(addr, data32)
			return
		case Z80_VOODOO_PORT_TEXSRC_LO:
			b.voodooTexSrc = (b.voodooTexSrc & 0xFF00) | uint16(value)
			return
		case Z80_VOODOO_PORT_TEXSRC_HI:
			b.voodooTexSrc = (b.voodooTexSrc & 0x00FF) | (uint16(value) << 8)
			return
		}
	}

	b.bus.Write8(translateIO8Bit(port), value)
}

func (b *Z80BusAdapter) Tick(cycles int) {}

func NewCPUZ80Runner(bus *MachineBus, config CPUZ80Config) *CPUZ80Runner {
	loadAddr := config.LoadAddr
	if loadAddr == 0 {
		loadAddr = defaultZ80LoadAddr
	}

	// Create Z80 system bus with optional VGA and Voodoo engine for port I/O
	var z80Bus *Z80BusAdapter
	if config.VoodooEngine != nil {
		z80Bus = NewZ80BusAdapterWithVoodoo(bus, config.VGAEngine, config.VoodooEngine)
	} else if config.VGAEngine != nil {
		z80Bus = NewZ80BusAdapterWithVGA(bus, config.VGAEngine)
	} else {
		z80Bus = NewZ80BusAdapter(bus)
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
	r.cpu.PerfEnabled = r.PerfEnabled
	r.cpu.Execute()
}

func (r *CPUZ80Runner) CPU() *CPU_Z80 {
	return r.cpu
}

func (r *CPUZ80Runner) IsRunning() bool {
	return r.cpu.Running()
}

func (r *CPUZ80Runner) StartExecution() {
	r.execMu.Lock()
	defer r.execMu.Unlock()
	if r.execActive {
		return
	}
	r.execActive = true
	r.cpu.SetRunning(true)
	r.execDone = make(chan struct{})
	go func() {
		defer func() {
			r.execMu.Lock()
			r.execActive = false
			close(r.execDone)
			r.execMu.Unlock()
		}()
		r.Execute()
	}()
}

func (r *CPUZ80Runner) Stop() {
	r.execMu.Lock()
	if !r.execActive {
		r.cpu.SetRunning(false)
		r.execMu.Unlock()
		return
	}
	r.cpu.SetRunning(false)
	done := r.execDone
	r.execMu.Unlock()
	<-done
}
