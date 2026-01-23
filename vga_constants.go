// vga_constants.go - IBM VGA chip register addresses and constants

package main

// VGA Register Base Address
const (
	VGA_BASE    = 0xF1000
	VGA_REG_END = 0xF13FF

	// Core control registers
	VGA_MODE   = 0xF1000 // Mode select (0x03=text, 0x12=640x480x16, 0x13=320x200x256)
	VGA_STATUS = 0xF1004 // Status (bit 0=vsync, bit 3=retrace)
	VGA_CTRL   = 0xF1008 // Control (bit 0=enable)

	// Sequencer registers (0x3C4/0x3C5 equivalent)
	VGA_SEQ_INDEX   = 0xF1010 // Sequencer index
	VGA_SEQ_DATA    = 0xF1014 // Sequencer data
	VGA_SEQ_MAPMASK = 0xF1018 // Plane write mask (direct access)

	// CRTC registers (0x3D4/0x3D5 equivalent)
	VGA_CRTC_INDEX   = 0xF1020 // CRTC index
	VGA_CRTC_DATA    = 0xF1024 // CRTC data
	VGA_CRTC_STARTHI = 0xF1028 // Start address high (for page flip)
	VGA_CRTC_STARTLO = 0xF102C // Start address low

	// Graphics Controller registers (0x3CE/0x3CF equivalent)
	VGA_GC_INDEX   = 0xF1030 // GC index
	VGA_GC_DATA    = 0xF1034 // GC data
	VGA_GC_READMAP = 0xF1038 // Read plane select (direct)
	VGA_GC_BITMASK = 0xF103C // Bit mask (direct)

	// Attribute Controller
	VGA_ATTR_INDEX = 0xF1040 // Attribute index/data
	VGA_ATTR_DATA  = 0xF1044 // Attribute read

	// DAC/Palette registers (0x3C6-0x3C9 equivalent)
	VGA_DAC_MASK   = 0xF1050 // Pixel mask
	VGA_DAC_RINDEX = 0xF1054 // Read index
	VGA_DAC_WINDEX = 0xF1058 // Write index
	VGA_DAC_DATA   = 0xF105C // DAC data (R,G,B sequence)

	// Palette RAM (256 x 3 bytes = 768 bytes)
	VGA_PALETTE     = 0xF1100 // Palette base
	VGA_PALETTE_END = 0xF13FF // Palette end

	// VGA VRAM window (matches PC convention)
	VGA_VRAM_WINDOW = 0xA0000 // VRAM window start
	VGA_VRAM_SIZE   = 0x10000 // 64KB VRAM window

	// Text mode buffer (0xB8000 equivalent)
	VGA_TEXT_WINDOW = 0xB8000 // Text buffer start
	VGA_TEXT_SIZE   = 0x8000  // 32KB text buffer
)

// VGA Mode Constants
const (
	VGA_MODE_TEXT = 0x03 // 80x25 text mode, 16 colors
	VGA_MODE_12H  = 0x12 // 640x480, 16 colors, planar
	VGA_MODE_13H  = 0x13 // 320x200, 256 colors, linear (Mode 13h)
	VGA_MODE_X    = 0x14 // 320x240, 256 colors, planar (Mode X)
)

// VGA Status bits
const (
	VGA_STATUS_VSYNC   = 1 << 0 // Vertical sync active
	VGA_STATUS_RETRACE = 1 << 3 // Vertical retrace active
)

// VGA Control bits
const (
	VGA_CTRL_ENABLE = 1 << 0 // VGA enabled
)

// Sequencer register indices
const (
	VGA_SEQ_RESET     = 0x00 // Reset register
	VGA_SEQ_CLKMODE   = 0x01 // Clocking mode
	VGA_SEQ_MAPMASK_R = 0x02 // Map mask (which planes to write)
	VGA_SEQ_CHARMAP   = 0x03 // Character map select
	VGA_SEQ_MEMMODE   = 0x04 // Memory mode
	VGA_SEQ_REG_COUNT = 5
)

// Sequencer memory mode bits
const (
	VGA_SEQ_MEMMODE_CHAIN4 = 1 << 3 // Chain-4 mode (Mode 13h)
	VGA_SEQ_MEMMODE_OE     = 1 << 2 // Odd/even disable
	VGA_SEQ_MEMMODE_EXT    = 1 << 1 // Extended memory
)

// CRTC register indices
const (
	VGA_CRTC_HTOTAL       = 0x00 // Horizontal total
	VGA_CRTC_HDISPLAY     = 0x01 // Horizontal display end
	VGA_CRTC_HBLANK_ST    = 0x02 // Horizontal blanking start
	VGA_CRTC_HBLANK_END   = 0x03 // Horizontal blanking end
	VGA_CRTC_HRETRACE_ST  = 0x04 // Horizontal retrace start
	VGA_CRTC_HRETRACE_END = 0x05 // Horizontal retrace end
	VGA_CRTC_VTOTAL       = 0x06 // Vertical total
	VGA_CRTC_OVERFLOW     = 0x07 // Overflow
	VGA_CRTC_PRESET_ROW   = 0x08 // Preset row scan
	VGA_CRTC_MAX_SCAN     = 0x09 // Maximum scan line
	VGA_CRTC_CURSOR_ST    = 0x0A // Cursor start
	VGA_CRTC_CURSOR_END   = 0x0B // Cursor end
	VGA_CRTC_START_HI     = 0x0C // Start address high
	VGA_CRTC_START_LO     = 0x0D // Start address low
	VGA_CRTC_CURSOR_HI    = 0x0E // Cursor location high
	VGA_CRTC_CURSOR_LO    = 0x0F // Cursor location low
	VGA_CRTC_VRETRACE_ST  = 0x10 // Vertical retrace start
	VGA_CRTC_VRETRACE_END = 0x11 // Vertical retrace end
	VGA_CRTC_VDISPLAY     = 0x12 // Vertical display end
	VGA_CRTC_OFFSET       = 0x13 // Offset (logical width)
	VGA_CRTC_UNDERLINE    = 0x14 // Underline location
	VGA_CRTC_VBLANK_ST    = 0x15 // Vertical blanking start
	VGA_CRTC_VBLANK_END   = 0x16 // Vertical blanking end
	VGA_CRTC_MODE_CTRL    = 0x17 // Mode control
	VGA_CRTC_LINE_CMP     = 0x18 // Line compare
	VGA_CRTC_REG_COUNT    = 25
)

// Graphics Controller register indices
const (
	VGA_GC_SET_RESET   = 0x00 // Set/Reset
	VGA_GC_ENABLE_SR   = 0x01 // Enable Set/Reset
	VGA_GC_COLOR_CMP   = 0x02 // Color compare
	VGA_GC_DATA_ROTATE = 0x03 // Data rotate
	VGA_GC_READ_MAP_R  = 0x04 // Read map select
	VGA_GC_MODE        = 0x05 // Graphics mode
	VGA_GC_MISC        = 0x06 // Miscellaneous
	VGA_GC_COLOR_DONT  = 0x07 // Color don't care
	VGA_GC_BITMASK_R   = 0x08 // Bit mask
	VGA_GC_REG_COUNT   = 9
)

// Graphics Controller mode bits
const (
	VGA_GC_MODE_WRITE_MASK = 0x03   // Write mode (0-3)
	VGA_GC_MODE_READ_MODE  = 1 << 3 // Read mode
	VGA_GC_MODE_HOST_OE    = 1 << 4 // Host odd/even
	VGA_GC_MODE_SHIFT_256  = 1 << 6 // 256-color shift mode
)

// Attribute Controller register indices
const (
	VGA_ATTR_PALETTE_BASE = 0x00 // Palette registers 0-15
	VGA_ATTR_MODE_CTRL    = 0x10 // Attribute mode control
	VGA_ATTR_OVERSCAN     = 0x11 // Overscan color
	VGA_ATTR_PLANE_EN     = 0x12 // Color plane enable
	VGA_ATTR_HPAN         = 0x13 // Horizontal panning
	VGA_ATTR_COLOR_SEL    = 0x14 // Color select
	VGA_ATTR_REG_COUNT    = 21
)

// VGA dimensions
const (
	VGA_TEXT_COLS   = 80
	VGA_TEXT_ROWS   = 25
	VGA_FONT_WIDTH  = 8
	VGA_FONT_HEIGHT = 16

	VGA_MODE13H_WIDTH  = 320
	VGA_MODE13H_HEIGHT = 200

	VGA_MODE12H_WIDTH  = 640
	VGA_MODE12H_HEIGHT = 480

	VGA_MODEX_WIDTH  = 320
	VGA_MODEX_HEIGHT = 240
)

// VGA timing constants
const (
	VGA_PALETTE_SIZE  = 256   // Number of palette entries
	VGA_PALETTE_BYTES = 768   // 256 * 3 bytes (R,G,B)
	VGA_PLANE_COUNT   = 4     // Number of bit planes
	VGA_PLANE_SIZE    = 65536 // 64KB per plane
)

// Z80 VGA port I/O mapping
const (
	Z80_VGA_PORT_MODE      = 0xA0
	Z80_VGA_PORT_STATUS    = 0xA1
	Z80_VGA_PORT_CTRL      = 0xA2
	Z80_VGA_PORT_SEQ_IDX   = 0xA3
	Z80_VGA_PORT_SEQ_DATA  = 0xA4
	Z80_VGA_PORT_CRTC_IDX  = 0xA5
	Z80_VGA_PORT_CRTC_DATA = 0xA6
	Z80_VGA_PORT_GC_IDX    = 0xA7
	Z80_VGA_PORT_GC_DATA   = 0xA8
	Z80_VGA_PORT_DAC_WIDX  = 0xA9
	Z80_VGA_PORT_DAC_DATA  = 0xAA
)

// 6502 VGA memory mapping
const (
	C6502_VGA_BASE      = 0xD700
	C6502_VGA_MODE      = 0xD700
	C6502_VGA_STATUS    = 0xD701
	C6502_VGA_CTRL      = 0xD702
	C6502_VGA_SEQ_IDX   = 0xD703
	C6502_VGA_SEQ_DATA  = 0xD704
	C6502_VGA_CRTC_IDX  = 0xD705
	C6502_VGA_CRTC_DATA = 0xD706
	C6502_VGA_GC_IDX    = 0xD707
	C6502_VGA_GC_DATA   = 0xD708
	C6502_VGA_DAC_WIDX  = 0xD709
	C6502_VGA_DAC_DATA  = 0xD70A
	C6502_VGA_END       = 0xD70A
)
