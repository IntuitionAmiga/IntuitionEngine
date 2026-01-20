; ie32.inc - Intuition Engine IE32 Macro Library
;
; Hardware definitions for writing IE32 assembly programs targeting
; the Intuition Engine virtual machine.
;
; For use with ie32asm assembler
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; MEMORY MAP CONSTANTS
; ============================================================================

; System vectors
.equ VECTOR_BASE       0x000000
.equ VECTOR_RESET      0x000000
.equ VECTOR_IRQ        0x000004

; Program space
.equ PROGRAM_START     0x001000

; Bank window addresses (for 8-bit CPU compatibility)
.equ BANK1_WINDOW      0x002000         ; Sprite data bank window (8KB)
.equ BANK2_WINDOW      0x004000         ; Font data bank window (8KB)
.equ BANK3_WINDOW      0x006000         ; General data bank window (8KB)
.equ VRAM_WINDOW       0x008000         ; VRAM bank window (16KB)
.equ BANK_SIZE         0x002000         ; 8KB per extended bank
.equ VRAM_BANK_SIZE    0x004000         ; 16KB VRAM bank

; Bank control registers
.equ BANK1_REG_LO      0x00F700         ; Bank 1 select (low byte)
.equ BANK1_REG_HI      0x00F701         ; Bank 1 select (high byte)
.equ BANK2_REG_LO      0x00F702         ; Bank 2 select (low byte)
.equ BANK2_REG_HI      0x00F703         ; Bank 2 select (high byte)
.equ BANK3_REG_LO      0x00F704         ; Bank 3 select (low byte)
.equ BANK3_REG_HI      0x00F705         ; Bank 3 select (high byte)
.equ VRAM_BANK_REG     0x00F7F0         ; VRAM bank select

; ============================================================================
; VIDEO HARDWARE REGISTERS (I/O region at 0xF0000)
; ============================================================================

.equ VIDEO_CTRL        0xF0000          ; Video control (enable/disable)
.equ VIDEO_MODE        0xF0004          ; Video mode (resolution)
.equ VIDEO_STATUS      0xF0008          ; Video status (VBlank flag)
.equ STATUS_VBLANK     2                ; VBlank status bit

; Copper coprocessor
.equ COPPER_CTRL       0xF000C          ; Copper control
.equ COPPER_PTR        0xF0010          ; Copper list pointer (32-bit)

; Blitter hardware
.equ BLT_CTRL          0xF001C          ; Blitter control
.equ BLT_OP            0xF0020          ; Blitter operation
.equ BLT_SRC           0xF0024          ; Source address (32-bit)
.equ BLT_DST           0xF0028          ; Dest address (32-bit)
.equ BLT_WIDTH         0xF002C          ; Blit width (32-bit)
.equ BLT_HEIGHT        0xF0030          ; Blit height (32-bit)
.equ BLT_SRC_STRIDE    0xF0034          ; Source stride (32-bit)
.equ BLT_DST_STRIDE    0xF0038          ; Dest stride (32-bit)
.equ BLT_COLOR         0xF003C          ; Fill color (32-bit BGRA)
.equ BLT_MASK          0xF0040          ; Mask address (32-bit)
.equ BLT_STATUS        0xF0044          ; Blitter status

; Raster registers
.equ VIDEO_RASTER_Y       0xF0048       ; Raster Y position (32-bit)
.equ VIDEO_RASTER_HEIGHT  0xF004C       ; Raster height (32-bit)
.equ VIDEO_RASTER_COLOR   0xF0050       ; Raster color (32-bit BGRA)
.equ VIDEO_RASTER_CTRL    0xF0054       ; Raster control

; ============================================================================
; PSG/AUDIO REGISTERS (AY-3-8910 style)
; ============================================================================

.equ PSG_PLUS_CTRL     0xF0C0E          ; PSG+ mode control

; ----------------------------------------------------------------------------
; PSG Player Registers
;
; The PSG player supports multiple music formats with automatic detection:
;   .ay   - ZX Spectrum format with embedded Z80 code (executed by emulator)
;   .sndh - Atari ST format with embedded M68K code (executed by emulator)
;   .ym   - YM2149 register dump frames (50Hz playback)
;   .vgm  - Video Game Music format with timed PSG events
;
; Usage:
;   1. Embed music data in your program using incbin
;   2. Set PSG_PLAY_PTR to the address of the music data
;   3. Set PSG_PLAY_LEN to the size of the music data in bytes
;   4. Write to PSG_PLAY_CTRL to start/stop playback
;
; PSG_PLAY_CTRL bits:
;   bit 0 (0x01) - Start playback (write 1 to begin)
;   bit 1 (0x02) - Stop playback (write 1 to stop)
;   bit 2 (0x04) - Enable looping (set before starting)
;
; PSG_PLAY_STATUS bits:
;   bit 0 (0x01) - Busy (1 = playing, 0 = stopped)
;   bit 1 (0x02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
.equ PSG_PLAY_PTR      0xF0C10          ; Play pointer (32-bit)
.equ PSG_PLAY_LEN      0xF0C14          ; Play length (32-bit)
.equ PSG_PLAY_CTRL     0xF0C18          ; Play control
.equ PSG_PLAY_STATUS   0xF0C1C          ; Play status

; ============================================================================
; POKEY AUDIO REGISTERS (Atari style)
; ============================================================================

.equ POKEY_BASE        0xF0D00          ; POKEY base address
.equ POKEY_AUDF1       0xF0D00          ; Channel 1 frequency divider
.equ POKEY_AUDC1       0xF0D01          ; Channel 1 control (distortion + volume)
.equ POKEY_AUDF2       0xF0D02          ; Channel 2 frequency divider
.equ POKEY_AUDC2       0xF0D03          ; Channel 2 control
.equ POKEY_AUDF3       0xF0D04          ; Channel 3 frequency divider
.equ POKEY_AUDC3       0xF0D05          ; Channel 3 control
.equ POKEY_AUDF4       0xF0D06          ; Channel 4 frequency divider
.equ POKEY_AUDC4       0xF0D07          ; Channel 4 control
.equ POKEY_AUDCTL      0xF0D08          ; Master audio control
.equ POKEY_PLUS_CTRL   0xF0D09          ; POKEY+ mode (0=standard, 1=enhanced)

; AUDCTL bit masks
.equ AUDCTL_CLOCK_15KHZ   0x01          ; Use 15kHz base clock (else 64kHz)
.equ AUDCTL_HIPASS_CH1    0x02          ; High-pass filter ch1 by ch3
.equ AUDCTL_HIPASS_CH2    0x04          ; High-pass filter ch2 by ch4
.equ AUDCTL_CH4_BY_CH3    0x08          ; Ch4 clocked by ch3 (16-bit mode)
.equ AUDCTL_CH2_BY_CH1    0x10          ; Ch2 clocked by ch1 (16-bit mode)
.equ AUDCTL_CH3_179MHZ    0x20          ; Ch3 uses 1.79MHz clock
.equ AUDCTL_CH1_179MHZ    0x40          ; Ch1 uses 1.79MHz clock
.equ AUDCTL_POLY9         0x80          ; Use 9-bit poly instead of 17-bit

; AUDC distortion modes (bits 5-7)
.equ POKEY_DIST_POLY17_POLY5  0x00      ; 17-bit + 5-bit poly
.equ POKEY_DIST_POLY5         0x20      ; 5-bit poly only
.equ POKEY_DIST_POLY17_POLY4  0x40      ; 17-bit + 4-bit poly
.equ POKEY_DIST_POLY5_POLY4   0x60      ; 5-bit + 4-bit poly
.equ POKEY_DIST_POLY17        0x80      ; 17-bit poly only
.equ POKEY_DIST_PURE_TONE     0xA0      ; Pure square wave
.equ POKEY_DIST_POLY4         0xC0      ; 4-bit poly only
.equ POKEY_DIST_POLY17_PULSE  0xE0      ; 17-bit + pulse

; ----------------------------------------------------------------------------
; SAP Player Registers
;
; The SAP player handles Atari 8-bit music files (.sap) which contain
; embedded 6502 code that drives the POKEY sound chip. Supports TYPE B files
; where INIT is called once and PLAYER is called each frame.
;
; Usage:
;   1. Embed SAP data in your program using .incbin
;   2. Set SAP_PLAY_PTR to the address of the SAP data
;   3. Set SAP_PLAY_LEN to the size of the SAP data in bytes
;   4. Optionally set SAP_SUBSONG to select a subsong (default 0)
;   5. Write to SAP_PLAY_CTRL to start/stop playback
;
; SAP_PLAY_CTRL bits:
;   bit 0 (0x01) - Start playback (write 1 to begin)
;   bit 1 (0x02) - Stop playback (write 1 to stop)
;   bit 2 (0x04) - Enable looping (set before starting)
;
; SAP_PLAY_STATUS bits:
;   bit 0 (0x01) - Busy (1 = playing, 0 = stopped)
;   bit 1 (0x02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
.equ SAP_PLAY_PTR      0xF0D10          ; Play pointer (32-bit)
.equ SAP_PLAY_LEN      0xF0D14          ; Play length (32-bit)
.equ SAP_PLAY_CTRL     0xF0D18          ; Play control
.equ SAP_PLAY_STATUS   0xF0D1C          ; Play status
.equ SAP_SUBSONG       0xF0D1D          ; Subsong selection (0-255)

; ============================================================================
; VIDEO CONSTANTS
; ============================================================================

.equ VRAM_START        0x100000         ; VRAM starts at 1MB
.equ SCREEN_W          640              ; Default screen width
.equ SCREEN_H          480              ; Default screen height
.equ LINE_BYTES        2560             ; Bytes per scanline (640 * 4)

; Blitter operations
.equ BLT_OP_COPY       0                ; Copy source to dest
.equ BLT_OP_FILL       1                ; Fill rect with color
.equ BLT_OP_LINE       2                ; Draw line
.equ BLT_OP_MASKED     3                ; Copy with mask
.equ BLT_OP_ALPHA      4                ; Alpha blending

; Background color (black with alpha)
.equ BACKGROUND        0xFF000000       ; BGRA: Black, full alpha

; ============================================================================
; COPPER OPCODES
; ============================================================================

; Copper WAIT opcode: wait for raster position
; Format: 0x0YYY_HHHH where YYY = Y position (0-4095), HHHH = H position
.equ COP_WAIT_MASK         0x00000000

; Copper MOVE opcodes
.equ COP_MOVE_RASTER_Y     0x40120000
.equ COP_MOVE_RASTER_H     0x40130000
.equ COP_MOVE_RASTER_COLOR 0x40140000
.equ COP_MOVE_RASTER_CTRL  0x40150000

; Copper END opcode
.equ COP_END               0xC0000000

; ============================================================================
; TIMER REGISTERS
; ============================================================================

.equ TIMER_CTRL        0x00F800         ; Timer control
.equ TIMER_COUNT       0x00F804         ; Timer count
.equ TIMER_RELOAD      0x00F808         ; Timer reload value

; ============================================================================
; EOF
; ============================================================================
