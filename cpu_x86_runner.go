// cpu_x86_runner.go - x86 CPU Program Runner
//
// Provides the system bus implementation for running x86 programs with
// full hardware integration (VGA, audio chips, etc.)
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	defaultX86LoadAddr = 0x00000000
	x86AddressSpace    = 0x02000000 // 32MB address space

	// x86 Bank Windows (same as Z80/6502 for compatibility)
	X86_BANK1_WINDOW_BASE = 0x2000 // Sprite data bank
	X86_BANK2_WINDOW_BASE = 0x4000 // Font data bank
	X86_BANK3_WINDOW_BASE = 0x6000 // General data bank
	X86_BANK_WINDOW_SIZE  = 0x2000 // 8KB per bank window

	// x86 VRAM Bank Window (16KB)
	X86_VRAM_BANK_WINDOW_BASE = 0x8000
	X86_VRAM_BANK_WINDOW_SIZE = 0x4000

	// Bank control registers (memory-mapped)
	X86_BANK1_REG_LO   = 0xF700
	X86_BANK1_REG_HI   = 0xF701
	X86_BANK2_REG_LO   = 0xF702
	X86_BANK2_REG_HI   = 0xF703
	X86_BANK3_REG_LO   = 0xF704
	X86_BANK3_REG_HI   = 0xF705
	X86_VRAM_BANK_REG  = 0xF7F0
	X86_VRAM_BANK_RSVD = 0xF7F1

	// I/O Port ranges for audio chips
	X86_PORT_PSG_SELECT = 0xF0
	X86_PORT_PSG_DATA   = 0xF1
	X86_PORT_SID_SELECT = 0xE0
	X86_PORT_SID_DATA   = 0xE1
	X86_PORT_POKEY_BASE = 0xD0 // 0xD0-0xDF for POKEY
	X86_PORT_TED_SELECT = 0xF2
	X86_PORT_TED_DATA   = 0xF3

	// Standard VGA I/O ports
	X86_PORT_VGA_SEQ_INDEX  = 0x3C4
	X86_PORT_VGA_SEQ_DATA   = 0x3C5
	X86_PORT_VGA_DAC_MASK   = 0x3C6
	X86_PORT_VGA_DAC_RINDEX = 0x3C7
	X86_PORT_VGA_DAC_WINDEX = 0x3C8
	X86_PORT_VGA_DAC_DATA   = 0x3C9
	X86_PORT_VGA_GC_INDEX   = 0x3CE
	X86_PORT_VGA_GC_DATA    = 0x3CF
	X86_PORT_VGA_CRTC_INDEX = 0x3D4
	X86_PORT_VGA_CRTC_DATA  = 0x3D5
	X86_PORT_VGA_STATUS     = 0x3DA
)

// CPUX86Config holds configuration for the x86 runner
type CPUX86Config struct {
	LoadAddr     uint32
	Entry        uint32
	VGAEngine    *VGAEngine    // Optional VGA engine for port I/O
	VoodooEngine *VoodooEngine // Optional Voodoo engine for port I/O
}

// CPUX86Runner manages the x86 CPU and system bus
type CPUX86Runner struct {
	cpu      *CPU_X86
	bus      *X86BusAdapter
	loadAddr uint32
	entry    uint32

	// Performance monitoring (matching IE32 pattern)
	PerfEnabled      bool      // Enable MIPS reporting
	InstructionCount uint64    // Total instructions executed
	perfStartTime    time.Time // When execution started
	lastPerfReport   time.Time // Last time we printed stats

	execMu     sync.Mutex
	execDone   chan struct{}
	execActive bool
}

// X86BusAdapter provides the system bus for x86 with hardware routing
type X86BusAdapter struct {
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
	voodooTexSrc uint16        // Texture source address in RAM
	voodooEngine *VoodooEngine // Voodoo engine for port I/O access

	// Extended bank windows (same as Z80/6502)
	vramBank    uint32
	vramEnabled bool
	bank1       uint32
	bank2       uint32
	bank3       uint32
	bank1Enable bool
	bank2Enable bool
	bank3Enable bool
}

// NewX86BusAdapter creates a new x86 system bus
func NewX86BusAdapter(bus *MachineBus) *X86BusAdapter {
	return &X86BusAdapter{bus: bus, psgRegSelect: 0}
}

// NewX86BusAdapterWithVGA creates an x86 system bus with VGA engine support
func NewX86BusAdapterWithVGA(bus *MachineBus, vga *VGAEngine) *X86BusAdapter {
	return &X86BusAdapter{bus: bus, psgRegSelect: 0, vgaEngine: vga}
}

// NewX86BusAdapterWithVoodoo creates an x86 system bus with VGA and Voodoo engine support
func NewX86BusAdapterWithVoodoo(bus *MachineBus, vga *VGAEngine, voodoo *VoodooEngine) *X86BusAdapter {
	return &X86BusAdapter{bus: bus, psgRegSelect: 0, vgaEngine: vga, voodooEngine: voodoo}
}

// translateIO translates I/O addresses for the system bus
// Non-I/O addresses pass through unchanged
func (b *X86BusAdapter) translateIO(addr uint32) uint32 {
	// I/O region at 0xF0000
	if addr >= 0xF000 && addr < 0x10000 {
		return 0xF0000 + (addr - 0xF000)
	}
	return addr
}

// Read implements X86Bus.Read
func (b *X86BusAdapter) Read(addr uint32) byte {
	// Handle VRAM bank register reads
	if addr == X86_VRAM_BANK_REG {
		return byte(b.vramBank & 0xFF)
	}
	if addr == X86_VRAM_BANK_RSVD {
		return 0
	}

	// Handle extended bank register reads
	switch addr {
	case X86_BANK1_REG_LO:
		return byte(b.bank1 & 0xFF)
	case X86_BANK1_REG_HI:
		return byte((b.bank1 >> 8) & 0xFF)
	case X86_BANK2_REG_LO:
		return byte(b.bank2 & 0xFF)
	case X86_BANK2_REG_HI:
		return byte((b.bank2 >> 8) & 0xFF)
	case X86_BANK3_REG_LO:
		return byte(b.bank3 & 0xFF)
	case X86_BANK3_REG_HI:
		return byte((b.bank3 >> 8) & 0xFF)
	}

	// Handle VGA VRAM at 0xA0000-0xAFFFF (64KB window)
	if addr >= 0xA0000 && addr < 0xB0000 {
		return b.bus.Read8(addr)
	}

	// Handle extended bank window reads
	if translated, ok := b.translateExtendedBank(addr); ok {
		return b.bus.Read8(translated)
	}

	// Handle VRAM bank window reads
	if translated, ok := b.translateVRAM(addr); ok {
		return b.bus.Read8(translated)
	}

	return b.bus.Read8(b.translateIO(addr))
}

// Write implements X86Bus.Write
func (b *X86BusAdapter) Write(addr uint32, value byte) {
	// Handle VRAM bank register writes
	if addr == X86_VRAM_BANK_REG {
		b.vramBank = uint32(value)
		b.vramEnabled = true
		return
	}
	if addr == X86_VRAM_BANK_RSVD {
		return
	}

	// Handle extended bank register writes
	switch addr {
	case X86_BANK1_REG_LO:
		b.bank1 = (b.bank1 & 0xFF00) | uint32(value)
		b.bank1Enable = true
		return
	case X86_BANK1_REG_HI:
		b.bank1 = (b.bank1 & 0x00FF) | (uint32(value) << 8)
		b.bank1Enable = true
		return
	case X86_BANK2_REG_LO:
		b.bank2 = (b.bank2 & 0xFF00) | uint32(value)
		b.bank2Enable = true
		return
	case X86_BANK2_REG_HI:
		b.bank2 = (b.bank2 & 0x00FF) | (uint32(value) << 8)
		b.bank2Enable = true
		return
	case X86_BANK3_REG_LO:
		b.bank3 = (b.bank3 & 0xFF00) | uint32(value)
		b.bank3Enable = true
		return
	case X86_BANK3_REG_HI:
		b.bank3 = (b.bank3 & 0x00FF) | (uint32(value) << 8)
		b.bank3Enable = true
		return
	}

	// Handle VGA VRAM at 0xA0000-0xAFFFF (64KB window)
	if addr >= 0xA0000 && addr < 0xB0000 {
		b.bus.Write8(addr, value)
		return
	}

	// Handle extended bank window writes
	if translated, ok := b.translateExtendedBank(addr); ok {
		b.bus.Write8(translated, value)
		return
	}

	// Handle VRAM bank window writes
	if translated, ok := b.translateVRAM(addr); ok {
		b.bus.Write8(translated, value)
		return
	}

	b.bus.Write8(b.translateIO(addr), value)
}

// translateExtendedBank translates addresses in extended bank windows
func (b *X86BusAdapter) translateExtendedBank(addr uint32) (uint32, bool) {
	// Only translate 16-bit addresses for bank windows
	if addr >= 0x10000 {
		return 0, false
	}

	addr16 := uint16(addr)

	// Check Bank 1 window ($2000-$3FFF)
	if b.bank1Enable && addr16 >= X86_BANK1_WINDOW_BASE && addr16 < X86_BANK1_WINDOW_BASE+X86_BANK_WINDOW_SIZE {
		offset := uint32(addr16 - X86_BANK1_WINDOW_BASE)
		translated := (b.bank1 * X86_BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	// Check Bank 2 window ($4000-$5FFF)
	if b.bank2Enable && addr16 >= X86_BANK2_WINDOW_BASE && addr16 < X86_BANK2_WINDOW_BASE+X86_BANK_WINDOW_SIZE {
		offset := uint32(addr16 - X86_BANK2_WINDOW_BASE)
		translated := (b.bank2 * X86_BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	// Check Bank 3 window ($6000-$7FFF)
	if b.bank3Enable && addr16 >= X86_BANK3_WINDOW_BASE && addr16 < X86_BANK3_WINDOW_BASE+X86_BANK_WINDOW_SIZE {
		offset := uint32(addr16 - X86_BANK3_WINDOW_BASE)
		translated := (b.bank3 * X86_BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	return 0, false
}

// translateVRAM translates addresses in the VRAM bank window
func (b *X86BusAdapter) translateVRAM(addr uint32) (uint32, bool) {
	if addr >= 0x10000 {
		return 0, false
	}

	addr16 := uint16(addr)

	if b.vramEnabled && addr16 >= X86_VRAM_BANK_WINDOW_BASE && addr16 < X86_VRAM_BANK_WINDOW_BASE+X86_VRAM_BANK_WINDOW_SIZE {
		offset := uint32(addr16 - X86_VRAM_BANK_WINDOW_BASE)
		translated := MAIN_VRAM_BASE + (b.vramBank * X86_VRAM_BANK_WINDOW_SIZE) + offset
		if translated < DEFAULT_MEMORY_SIZE {
			return translated, true
		}
	}

	return 0, false
}

// In implements X86Bus.In for port I/O
func (b *X86BusAdapter) In(port uint16) byte {
	// PSG port I/O
	if port == X86_PORT_PSG_SELECT {
		return b.psgRegSelect
	}
	if port == X86_PORT_PSG_DATA {
		// Read from PSG register
		return b.bus.Read8(0xF0C00 + uint32(b.psgRegSelect))
	}

	// SID port I/O
	if port == X86_PORT_SID_SELECT {
		return b.sidRegSelect
	}
	if port == X86_PORT_SID_DATA {
		return b.bus.Read8(0xF0E00 + uint32(b.sidRegSelect))
	}

	// ANTIC port I/O (0xD4-0xD5) - check before POKEY range
	if port == X86_PORT_ANTIC_SELECT {
		return b.anticRegSelect
	}
	if port == X86_PORT_ANTIC_DATA {
		if b.anticRegSelect < ANTIC_REG_COUNT {
			// ANTIC registers are 4-byte aligned
			return b.bus.Read8(ANTIC_BASE + uint32(b.anticRegSelect)*4)
		}
		return 0
	}

	// GTIA port I/O (0xD6-0xD7) - check before POKEY range
	if port == X86_PORT_GTIA_SELECT {
		return b.gtiaRegSelect
	}
	if port == X86_PORT_GTIA_DATA {
		if b.gtiaRegSelect < GTIA_REG_COUNT {
			// GTIA registers are 4-byte aligned
			return b.bus.Read8(GTIA_BASE + uint32(b.gtiaRegSelect)*4)
		}
		return 0
	}

	// POKEY port I/O (0xD0-0xD3, 0xD8-0xDF) - excludes ANTIC/GTIA ports
	if port >= X86_PORT_POKEY_BASE && port < X86_PORT_POKEY_BASE+16 {
		// Skip ANTIC/GTIA ports (0xD4-0xD7)
		if port < X86_PORT_ANTIC_SELECT || port > X86_PORT_GTIA_DATA {
			offset := port - X86_PORT_POKEY_BASE
			return b.bus.Read8(0xF0D00 + uint32(offset))
		}
	}

	// TED port I/O
	if port == X86_PORT_TED_SELECT {
		return b.tedRegSelect
	}
	if port == X86_PORT_TED_DATA {
		// Audio registers 0x00-0x05, video registers 0x20-0x2F
		if b.tedRegSelect < 0x06 {
			return b.bus.Read8(0xF0F00 + uint32(b.tedRegSelect))
		} else if b.tedRegSelect >= 0x20 && b.tedRegSelect < 0x30 {
			// Video registers at 0xF0F20-0xF0F5F (4-byte aligned)
			return b.bus.Read8(0xF0F20 + uint32(b.tedRegSelect-0x20)*4)
		}
		return 0
	}

	// ULA port I/O (0xFE - same as Z80 for compatibility)
	if port == Z80_ULA_PORT {
		return b.bus.Read8(ULA_BORDER) & 0x07
	}

	// Standard VGA ports
	if b.vgaEngine != nil {
		switch port {
		case X86_PORT_VGA_SEQ_INDEX:
			return b.vgaEngine.seqIndex
		case X86_PORT_VGA_SEQ_DATA:
			if b.vgaEngine.seqIndex < 8 {
				return b.vgaEngine.seqRegs[b.vgaEngine.seqIndex]
			}
		case X86_PORT_VGA_DAC_MASK:
			return b.vgaEngine.dacMask
		case X86_PORT_VGA_GC_INDEX:
			return b.vgaEngine.gcIndex
		case X86_PORT_VGA_GC_DATA:
			if b.vgaEngine.gcIndex < 16 {
				return b.vgaEngine.gcRegs[b.vgaEngine.gcIndex]
			}
		case X86_PORT_VGA_CRTC_INDEX:
			return b.vgaEngine.crtcIndex
		case X86_PORT_VGA_CRTC_DATA:
			if b.vgaEngine.crtcIndex < 32 {
				return b.vgaEngine.crtcRegs[b.vgaEngine.crtcIndex]
			}
		case X86_PORT_VGA_STATUS:
			// Return VSync status
			status := byte(0)
			if b.vgaEngine.vsync.Load() {
				status |= 0x08 // Vertical retrace
			}
			return status
		}
	}

	// Voodoo port I/O (0xB0-0xB7)
	if b.voodooEngine != nil {
		switch port {
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

	return 0
}

// Out implements X86Bus.Out for port I/O
func (b *X86BusAdapter) Out(port uint16, value byte) {
	// PSG port I/O
	if port == X86_PORT_PSG_SELECT {
		b.psgRegSelect = value
		return
	}
	if port == X86_PORT_PSG_DATA {
		b.bus.Write8(0xF0C00+uint32(b.psgRegSelect), value)
		return
	}

	// SID port I/O
	if port == X86_PORT_SID_SELECT {
		b.sidRegSelect = value
		return
	}
	if port == X86_PORT_SID_DATA {
		b.bus.Write8(0xF0E00+uint32(b.sidRegSelect), value)
		return
	}

	// ANTIC port I/O (0xD4-0xD5) - check before POKEY range
	if port == X86_PORT_ANTIC_SELECT {
		b.anticRegSelect = value & 0x0F // 16 ANTIC registers
		return
	}
	if port == X86_PORT_ANTIC_DATA {
		if b.anticRegSelect < ANTIC_REG_COUNT {
			// ANTIC registers are 4-byte aligned
			b.bus.Write8(ANTIC_BASE+uint32(b.anticRegSelect)*4, value)
		}
		return
	}

	// GTIA port I/O (0xD6-0xD7) - check before POKEY range
	if port == X86_PORT_GTIA_SELECT {
		b.gtiaRegSelect = value & 0x0F // 12 GTIA registers
		return
	}
	if port == X86_PORT_GTIA_DATA {
		if b.gtiaRegSelect < GTIA_REG_COUNT {
			// GTIA registers are 4-byte aligned
			b.bus.Write8(GTIA_BASE+uint32(b.gtiaRegSelect)*4, value)
		}
		return
	}

	// POKEY port I/O (0xD0-0xD3, 0xD8-0xDF) - excludes ANTIC/GTIA ports
	if port >= X86_PORT_POKEY_BASE && port < X86_PORT_POKEY_BASE+16 {
		// Skip ANTIC/GTIA ports (0xD4-0xD7)
		if port < X86_PORT_ANTIC_SELECT || port > X86_PORT_GTIA_DATA {
			offset := port - X86_PORT_POKEY_BASE
			b.bus.Write8(0xF0D00+uint32(offset), value)
		}
		return
	}

	// TED port I/O
	if port == X86_PORT_TED_SELECT {
		b.tedRegSelect = value
		return
	}
	if port == X86_PORT_TED_DATA {
		if b.tedRegSelect < 0x06 {
			b.bus.Write8(0xF0F00+uint32(b.tedRegSelect), value)
		} else if b.tedRegSelect >= 0x20 && b.tedRegSelect < 0x30 {
			b.bus.Write8(0xF0F20+uint32(b.tedRegSelect-0x20)*4, value)
		}
		return
	}

	// ULA port I/O (0xFE - same as Z80 for compatibility)
	if port == Z80_ULA_PORT {
		b.bus.Write8(ULA_BORDER, value&0x07)
		return
	}

	// Standard VGA ports
	if b.vgaEngine != nil {
		switch port {
		case X86_PORT_VGA_SEQ_INDEX:
			b.vgaEngine.seqIndex = value
		case X86_PORT_VGA_SEQ_DATA:
			if b.vgaEngine.seqIndex < 8 {
				b.vgaEngine.seqRegs[b.vgaEngine.seqIndex] = value
			}
		case X86_PORT_VGA_DAC_MASK:
			b.vgaEngine.dacMask = value
		case X86_PORT_VGA_DAC_RINDEX:
			b.vgaEngine.dacReadIndex = value
			b.vgaEngine.dacReadPhase = 0
		case X86_PORT_VGA_DAC_WINDEX:
			b.vgaEngine.dacWriteIndex = value
			b.vgaEngine.dacWritePhase = 0
		case X86_PORT_VGA_DAC_DATA:
			// Write R, G, B in sequence
			idx := int(b.vgaEngine.dacWriteIndex) * 3
			if idx+int(b.vgaEngine.dacWritePhase) < len(b.vgaEngine.palette) {
				b.vgaEngine.palette[idx+int(b.vgaEngine.dacWritePhase)] = value
			}
			b.vgaEngine.dacWritePhase++
			if b.vgaEngine.dacWritePhase >= 3 {
				b.vgaEngine.dacWritePhase = 0
				b.vgaEngine.dacWriteIndex++
			}
		case X86_PORT_VGA_GC_INDEX:
			b.vgaEngine.gcIndex = value
		case X86_PORT_VGA_GC_DATA:
			if b.vgaEngine.gcIndex < 16 {
				b.vgaEngine.gcRegs[b.vgaEngine.gcIndex] = value
			}
		case X86_PORT_VGA_CRTC_INDEX:
			b.vgaEngine.crtcIndex = value
		case X86_PORT_VGA_CRTC_DATA:
			if b.vgaEngine.crtcIndex < 32 {
				b.vgaEngine.crtcRegs[b.vgaEngine.crtcIndex] = value
			}
		}
	}

	// Voodoo port I/O (0xB0-0xB7)
	if b.voodooEngine != nil {
		switch port {
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
			b.voodooData[3] = value
			data32 := uint32(b.voodooData[0]) |
				(uint32(b.voodooData[1]) << 8) |
				(uint32(b.voodooData[2]) << 16) |
				(uint32(b.voodooData[3]) << 24)
			addr := VOODOO_BASE + uint32(b.voodooAddr)
			if addr == VOODOO_TEX_UPLOAD && b.voodooTexSrc != 0 {
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
}

// Tick implements X86Bus.Tick
func (b *X86BusAdapter) Tick(cycles int) {
	// Could be used for cycle-accurate timing
}

// NewCPUX86Runner creates a new x86 CPU runner with the given configuration
func NewCPUX86Runner(bus *MachineBus, config *CPUX86Config) *CPUX86Runner {
	loadAddr := uint32(defaultX86LoadAddr)
	entry := uint32(defaultX86LoadAddr)

	if config != nil {
		if config.LoadAddr != 0 {
			loadAddr = config.LoadAddr
		}
		if config.Entry != 0 {
			entry = config.Entry
		}
	}

	var x86Bus *X86BusAdapter
	if config != nil && config.VoodooEngine != nil {
		x86Bus = NewX86BusAdapterWithVoodoo(bus, config.VGAEngine, config.VoodooEngine)
	} else if config != nil && config.VGAEngine != nil {
		x86Bus = NewX86BusAdapterWithVGA(bus, config.VGAEngine)
	} else {
		x86Bus = NewX86BusAdapter(bus)
	}

	cpu := NewCPU_X86(x86Bus)

	return &CPUX86Runner{
		cpu:      cpu,
		bus:      x86Bus,
		loadAddr: loadAddr,
		entry:    entry,
	}
}

// LoadProgramData loads a binary program from bytes into memory
func (r *CPUX86Runner) LoadProgramData(data []byte) error {
	if uint32(len(data))+r.loadAddr > x86AddressSpace {
		return fmt.Errorf("program too large: %d bytes", len(data))
	}

	// Load program into memory
	for i, b := range data {
		r.bus.bus.Write8(r.loadAddr+uint32(i), b)
	}

	// Set entry point
	r.cpu.EIP = r.entry

	return nil
}

// LoadProgram loads a binary program from a file (implements EmulatorCPU interface)
func (r *CPUX86Runner) LoadProgram(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}
	return r.LoadProgramData(data)
}

// LoadProgramFromFile is an alias for LoadProgram for backwards compatibility
func (r *CPUX86Runner) LoadProgramFromFile(filename string) error {
	return r.LoadProgram(filename)
}

// Run executes the program until halted
func (r *CPUX86Runner) Run() {
	// Initialize perf counters if enabled
	if r.PerfEnabled {
		r.perfStartTime = time.Now()
		r.lastPerfReport = r.perfStartTime
		r.InstructionCount = 0
	}

	for r.cpu.Running() && !r.cpu.Halted {
		r.cpu.Step()

		// Performance monitoring (matching IE32 pattern)
		if r.PerfEnabled {
			r.InstructionCount++
			if r.InstructionCount&0xFFFFFF == 0 { // Every ~16M instructions
				now := time.Now()
				if now.Sub(r.lastPerfReport) >= time.Second {
					elapsed := now.Sub(r.perfStartTime).Seconds()
					ips := float64(r.InstructionCount) / elapsed
					mips := ips / 1_000_000
					fmt.Printf("x86: %.2f MIPS (%.0f instructions in %.1fs)\n", mips, float64(r.InstructionCount), elapsed)
					r.lastPerfReport = now
				}
			}
		}
	}
}

// Step executes a single instruction
func (r *CPUX86Runner) Step() int {
	return r.cpu.Step()
}

// GetCPU returns the CPU instance
func (r *CPUX86Runner) GetCPU() *CPU_X86 {
	return r.cpu
}

// Reset resets the CPU
func (r *CPUX86Runner) Reset() {
	r.cpu.Reset()
	r.cpu.EIP = r.entry
}

// Execute runs the CPU in a loop until halted (for GUI integration)
func (r *CPUX86Runner) Execute() {
	if r.PerfEnabled {
		r.perfStartTime = time.Now()
		r.lastPerfReport = r.perfStartTime
		r.InstructionCount = 0
	}

	for r.cpu.Running() && !r.cpu.Halted {
		r.cpu.Step()

		if r.PerfEnabled {
			r.InstructionCount++
			if r.InstructionCount&0xFFFFFF == 0 {
				now := time.Now()
				if now.Sub(r.lastPerfReport) >= time.Second {
					elapsed := now.Sub(r.perfStartTime).Seconds()
					ips := float64(r.InstructionCount) / elapsed
					mips := ips / 1_000_000
					fmt.Printf("x86: %.2f MIPS (%.0f instructions in %.1fs)\n", mips, float64(r.InstructionCount), elapsed)
					r.lastPerfReport = now
				}
			}
		}
	}
}

// IsRunning returns whether the CPU is still running
func (r *CPUX86Runner) IsRunning() bool {
	return r.cpu.Running() && !r.cpu.Halted
}

func (r *CPUX86Runner) StartExecution() {
	r.execMu.Lock()
	defer r.execMu.Unlock()
	if r.execActive {
		return
	}
	r.execActive = true
	r.cpu.SetRunning(true)
	r.cpu.Halted = false
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

func (r *CPUX86Runner) Stop() {
	r.execMu.Lock()
	if !r.execActive {
		r.cpu.SetRunning(false)
		r.cpu.Halted = true
		r.execMu.Unlock()
		return
	}
	r.cpu.SetRunning(false)
	r.cpu.Halted = true
	done := r.execDone
	r.execMu.Unlock()
	<-done
}
