; ie65.inc - Intuition Engine 6502 (IE65) Macro Library
;
; Hardware definitions and utility macros for writing 6502 assembly
; programs targeting the Intuition Engine virtual machine.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; MEMORY MAP CONSTANTS
; ============================================================================

; Bank window addresses
BANK1_WINDOW    = $2000         ; Sprite data bank window (8KB)
BANK2_WINDOW    = $4000         ; Font data bank window (8KB)
BANK3_WINDOW    = $6000         ; General data bank window (8KB)
VRAM_WINDOW     = $8000         ; VRAM bank window (16KB)
BANK_SIZE       = $2000         ; 8KB per extended bank
VRAM_BANK_SIZE  = $4000         ; 16KB VRAM bank

; Bank control registers
BANK1_REG_LO    = $F700         ; Bank 1 select (low byte)
BANK1_REG_HI    = $F701         ; Bank 1 select (high byte)
BANK2_REG_LO    = $F702         ; Bank 2 select (low byte)
BANK2_REG_HI    = $F703         ; Bank 2 select (high byte)
BANK3_REG_LO    = $F704         ; Bank 3 select (low byte)
BANK3_REG_HI    = $F705         ; Bank 3 select (high byte)
VRAM_BANK_REG   = $F7F0         ; VRAM bank select

; ============================================================================
; VIDEO HARDWARE REGISTERS (mapped from IE32 $F0000-$F00FF to 6502 $F000-$F0FF)
; ============================================================================

VIDEO_CTRL      = $F000         ; Video control (enable/disable)
VIDEO_MODE      = $F004         ; Video mode (resolution)
VIDEO_STATUS    = $F008         ; Video status (VBlank flag)
STATUS_VBLANK   = 2             ; VBlank status bit

; Copper coprocessor
COPPER_CTRL     = $F00C         ; Copper control
COPPER_PTR_0    = $F010         ; Copper list pointer byte 0 (LSB)
COPPER_PTR_1    = $F011         ; Copper list pointer byte 1
COPPER_PTR_2    = $F012         ; Copper list pointer byte 2
COPPER_PTR_3    = $F013         ; Copper list pointer byte 3 (MSB)

; Blitter hardware
BLT_CTRL        = $F01C         ; Blitter control
BLT_OP          = $F020         ; Blitter operation
BLT_SRC_0       = $F024         ; Source address byte 0 (LSB)
BLT_SRC_1       = $F025         ; Source address byte 1
BLT_SRC_2       = $F026         ; Source address byte 2
BLT_SRC_3       = $F027         ; Source address byte 3 (MSB)
BLT_DST_0       = $F028         ; Dest address byte 0 (LSB)
BLT_DST_1       = $F029         ; Dest address byte 1
BLT_DST_2       = $F02A         ; Dest address byte 2
BLT_DST_3       = $F02B         ; Dest address byte 3 (MSB)
BLT_WIDTH_LO    = $F02C         ; Blit width (low byte)
BLT_WIDTH_HI    = $F02D         ; Blit width (high byte)
BLT_HEIGHT_LO   = $F030         ; Blit height (low byte)
BLT_HEIGHT_HI   = $F031         ; Blit height (high byte)
BLT_SRC_STRIDE_LO = $F034       ; Source stride (low byte)
BLT_SRC_STRIDE_HI = $F035       ; Source stride (high byte)
BLT_DST_STRIDE_LO = $F038       ; Dest stride (low byte)
BLT_DST_STRIDE_HI = $F039       ; Dest stride (high byte)
BLT_COLOR_0     = $F03C         ; Fill color byte 0
BLT_COLOR_1     = $F03D         ; Fill color byte 1
BLT_COLOR_2     = $F03E         ; Fill color byte 2
BLT_COLOR_3     = $F03F         ; Fill color byte 3
BLT_MASK_0      = $F040         ; Mask address byte 0 (LSB)
BLT_MASK_1      = $F041         ; Mask address byte 1
BLT_MASK_2      = $F042         ; Mask address byte 2
BLT_MASK_3      = $F043         ; Mask address byte 3 (MSB)
BLT_STATUS      = $F044         ; Blitter status

; Raster registers
VIDEO_RASTER_Y_LO     = $F048   ; Raster Y position (low)
VIDEO_RASTER_Y_HI     = $F049   ; Raster Y position (high)
VIDEO_RASTER_HEIGHT_LO = $F04C  ; Raster height (low)
VIDEO_RASTER_HEIGHT_HI = $F04D  ; Raster height (high)
VIDEO_RASTER_COLOR_0  = $F050   ; Raster color byte 0
VIDEO_RASTER_COLOR_1  = $F051   ; Raster color byte 1
VIDEO_RASTER_COLOR_2  = $F052   ; Raster color byte 2
VIDEO_RASTER_COLOR_3  = $F053   ; Raster color byte 3
VIDEO_RASTER_CTRL     = $F054   ; Raster control

; ============================================================================
; PSG/AUDIO REGISTERS (mapped from IE32 $F0C00+ to 6502 $FC00+)
; Translation: 6502 $FCxx -> IE32 $F0Cxx via formula $F0000 + (addr - $F000)
; ============================================================================

PSG_PLUS_CTRL   = $FC0E         ; PSG+ mode control (IE32 $F0C0E)

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
;   2. Set PSG_PLAY_PTR bytes to the address of the music data (little-endian)
;   3. Set PSG_PLAY_LEN bytes to the size of the music data (little-endian)
;   4. Write to PSG_PLAY_CTRL to start/stop playback
;
; PSG_PLAY_CTRL bits:
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; PSG_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
PSG_PLAY_PTR_0  = $FC10         ; Play pointer byte 0 (IE32 $F0C10)
PSG_PLAY_PTR_1  = $FC11         ; Play pointer byte 1
PSG_PLAY_PTR_2  = $FC12         ; Play pointer byte 2
PSG_PLAY_PTR_3  = $FC13         ; Play pointer byte 3
PSG_PLAY_LEN_0  = $FC14         ; Play length byte 0 (IE32 $F0C14)
PSG_PLAY_LEN_1  = $FC15         ; Play length byte 1
PSG_PLAY_LEN_2  = $FC16         ; Play length byte 2
PSG_PLAY_LEN_3  = $FC17         ; Play length byte 3
PSG_PLAY_CTRL   = $FC18         ; Play control (IE32 $F0C18)
PSG_PLAY_STATUS = $FC1C         ; Play status (IE32 $F0C1C)

; ============================================================================
; POKEY AUDIO REGISTERS (Atari-style addresses $D200-$D209)
; ============================================================================

POKEY_BASE      = $D200         ; POKEY base address
POKEY_AUDF1     = $D200         ; Channel 1 frequency divider
POKEY_AUDC1     = $D201         ; Channel 1 control (distortion + volume)
POKEY_AUDF2     = $D202         ; Channel 2 frequency divider
POKEY_AUDC2     = $D203         ; Channel 2 control
POKEY_AUDF3     = $D204         ; Channel 3 frequency divider
POKEY_AUDC3     = $D205         ; Channel 3 control
POKEY_AUDF4     = $D206         ; Channel 4 frequency divider
POKEY_AUDC4     = $D207         ; Channel 4 control
POKEY_AUDCTL    = $D208         ; Master audio control
POKEY_PLUS_CTRL = $D209         ; POKEY+ mode (0=standard, 1=enhanced)

; AUDCTL bit masks
AUDCTL_CLOCK_15KHZ  = $01       ; Use 15kHz base clock (else 64kHz)
AUDCTL_HIPASS_CH1   = $02       ; High-pass filter ch1 by ch3
AUDCTL_HIPASS_CH2   = $04       ; High-pass filter ch2 by ch4
AUDCTL_CH4_BY_CH3   = $08       ; Ch4 clocked by ch3 (16-bit mode)
AUDCTL_CH2_BY_CH1   = $10       ; Ch2 clocked by ch1 (16-bit mode)
AUDCTL_CH3_179MHZ   = $20       ; Ch3 uses 1.79MHz clock
AUDCTL_CH1_179MHZ   = $40       ; Ch1 uses 1.79MHz clock
AUDCTL_POLY9        = $80       ; Use 9-bit poly instead of 17-bit

; AUDC distortion modes (bits 5-7)
POKEY_DIST_POLY17_POLY5 = $00   ; 17-bit + 5-bit poly
POKEY_DIST_POLY5        = $20   ; 5-bit poly only
POKEY_DIST_POLY17_POLY4 = $40   ; 17-bit + 4-bit poly
POKEY_DIST_POLY5_POLY4  = $60   ; 5-bit + 4-bit poly
POKEY_DIST_POLY17       = $80   ; 17-bit poly only
POKEY_DIST_PURE_TONE    = $A0   ; Pure square wave
POKEY_DIST_POLY4        = $C0   ; 4-bit poly only
POKEY_DIST_POLY17_PULSE = $E0   ; 17-bit + pulse

; ----------------------------------------------------------------------------
; SAP Player Registers
;
; The SAP player handles Atari 8-bit music files (.sap) which contain
; embedded 6502 code that drives the POKEY sound chip. Supports TYPE B files
; where INIT is called once and PLAYER is called each frame.
;
; Usage:
;   1. Embed SAP data in your program using .incbin
;   2. Set SAP_PLAY_PTR bytes to the address of the SAP data (little-endian)
;   3. Set SAP_PLAY_LEN bytes to the size of the SAP data (little-endian)
;   4. Optionally set SAP_SUBSONG to select a subsong (default 0)
;   5. Write to SAP_PLAY_CTRL to start/stop playback
;
; SAP_PLAY_CTRL bits:
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; SAP_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
SAP_PLAY_PTR_0  = $FD10         ; Play pointer byte 0 (IE32 $F0D10)
SAP_PLAY_PTR_1  = $FD11         ; Play pointer byte 1
SAP_PLAY_PTR_2  = $FD12         ; Play pointer byte 2
SAP_PLAY_PTR_3  = $FD13         ; Play pointer byte 3
SAP_PLAY_LEN_0  = $FD14         ; Play length byte 0 (IE32 $F0D14)
SAP_PLAY_LEN_1  = $FD15         ; Play length byte 1
SAP_PLAY_LEN_2  = $FD16         ; Play length byte 2
SAP_PLAY_LEN_3  = $FD17         ; Play length byte 3
SAP_PLAY_CTRL   = $FD18         ; Play control (IE32 $F0D18)
SAP_PLAY_STATUS = $FD1C         ; Play status (IE32 $F0D1C)
SAP_SUBSONG     = $FD1D         ; Subsong selection (0-255)

; ============================================================================
; VIDEO CONSTANTS
; ============================================================================

VRAM_START      = $100000       ; VRAM starts at 1MB in 32-bit space
SCREEN_W        = 640           ; Default screen width
SCREEN_H        = 480           ; Default screen height
LINE_BYTES      = 2560          ; Bytes per scanline (640 * 4)

; Blitter operations
BLT_OP_COPY     = 0             ; Copy source to dest
BLT_OP_FILL     = 1             ; Fill rect with color
BLT_OP_LINE     = 2             ; Draw line
BLT_OP_MASKED   = 3             ; Copy with mask
BLT_OP_ALPHA    = 4             ; Alpha blending

; Background color (black with alpha)
BACKGROUND_0    = $00           ; Blue
BACKGROUND_1    = $00           ; Green
BACKGROUND_2    = $00           ; Red
BACKGROUND_3    = $FF           ; Alpha

; ============================================================================
; COPPER OPCODES
; ============================================================================

; Copper WAIT opcode: wait for raster position
; Format: $0YYY_HHHH where YYY = Y position (0-4095), HHHH = H position
COP_WAIT_MASK   = $00000000

; Copper MOVE opcodes
COP_MOVE_RASTER_Y     = $40120000
COP_MOVE_RASTER_H     = $40130000
COP_MOVE_RASTER_COLOR = $40140000
COP_MOVE_RASTER_CTRL  = $40150000

; Copper END opcode
COP_END         = $C0000000

; ============================================================================
; MACROS
; ============================================================================

; Set bank 1 (sprite data) - 16-bit bank number
.macro SET_BANK1 bank
    lda #<(bank)
    sta BANK1_REG_LO
    lda #>(bank)
    sta BANK1_REG_HI
.endmacro

; Set bank 2 (font data) - 16-bit bank number
.macro SET_BANK2 bank
    lda #<(bank)
    sta BANK2_REG_LO
    lda #>(bank)
    sta BANK2_REG_HI
.endmacro

; Set bank 3 (general data) - 16-bit bank number
.macro SET_BANK3 bank
    lda #<(bank)
    sta BANK3_REG_LO
    lda #>(bank)
    sta BANK3_REG_HI
.endmacro

; Set VRAM bank
.macro SET_VRAM_BANK bank
    lda #bank
    sta VRAM_BANK_REG
.endmacro

; Store 16-bit value to address
.macro STORE16 addr, value
    lda #<(value)
    sta addr
    lda #>(value)
    sta addr+1
.endmacro

; Store 32-bit value to I/O register (little-endian)
.macro STORE32 addr, value
    lda #<(value)
    sta addr
    lda #>(value)
    sta addr+1
    lda #^(value)
    sta addr+2
    lda #.lobyte(.hiword(value) >> 8)
    sta addr+3
.endmacro

; Store 32-bit value from zero page pointers
.macro STORE32_ZP addr, zp_ptr
    ldy #0
    lda (zp_ptr),y
    sta addr
    iny
    lda (zp_ptr),y
    sta addr+1
    iny
    lda (zp_ptr),y
    sta addr+2
    iny
    lda (zp_ptr),y
    sta addr+3
.endmacro

; Wait for VBlank
.macro WAIT_VBLANK
:   lda VIDEO_STATUS
    and #STATUS_VBLANK
    beq :-
.endmacro

; Wait for blitter to finish
.macro WAIT_BLIT
:   lda BLT_CTRL
    and #2
    bne :-
.endmacro

; Start a blitter operation (write 1 to control)
.macro START_BLIT
    lda #1
    sta BLT_CTRL
.endmacro

; Set blitter operation type
.macro SET_BLT_OP op
    lda #op
    sta BLT_OP
.endmacro

; Set blitter width (16-bit)
.macro SET_BLT_WIDTH width
    lda #<(width)
    sta BLT_WIDTH_LO
    lda #>(width)
    sta BLT_WIDTH_HI
.endmacro

; Set blitter height (16-bit)
.macro SET_BLT_HEIGHT height
    lda #<(height)
    sta BLT_HEIGHT_LO
    lda #>(height)
    sta BLT_HEIGHT_HI
.endmacro

; Set source stride (16-bit)
.macro SET_SRC_STRIDE stride
    lda #<(stride)
    sta BLT_SRC_STRIDE_LO
    lda #>(stride)
    sta BLT_SRC_STRIDE_HI
.endmacro

; Set dest stride (16-bit)
.macro SET_DST_STRIDE stride
    lda #<(stride)
    sta BLT_DST_STRIDE_LO
    lda #>(stride)
    sta BLT_DST_STRIDE_HI
.endmacro

; Set blitter fill color (32-bit BGRA)
.macro SET_BLT_COLOR color
    lda #<(color)
    sta BLT_COLOR_0
    lda #>(color)
    sta BLT_COLOR_1
    lda #^(color)
    sta BLT_COLOR_2
    lda #.lobyte(.hiword(color) >> 8)
    sta BLT_COLOR_3
.endmacro

; Add 16-bit value to zero page 16-bit variable
.macro ADD16 zp_var, value
    clc
    lda zp_var
    adc #<(value)
    sta zp_var
    lda zp_var+1
    adc #>(value)
    sta zp_var+1
.endmacro

; Increment 16-bit zero page variable
.macro INC16 zp_var
    inc zp_var
    bne :+
    inc zp_var+1
:
.endmacro

; Compare 16-bit zero page variable with immediate
.macro CMP16 zp_var, value
    lda zp_var
    cmp #<(value)
    bne :+
    lda zp_var+1
    cmp #>(value)
:
.endmacro

; ============================================================================
; ZERO PAGE ALLOCATION HELPER
; ============================================================================

; Define commonly used zero page variables
.zeropage
    zp_ptr0:    .res 2          ; General purpose pointer 0
    zp_ptr1:    .res 2          ; General purpose pointer 1
    zp_ptr2:    .res 2          ; General purpose pointer 2
    zp_ptr3:    .res 2          ; General purpose pointer 3
    zp_tmp0:    .res 4          ; 32-bit temporary 0
    zp_tmp1:    .res 4          ; 32-bit temporary 1
    zp_frame:   .res 2          ; Frame counter
    zp_scratch: .res 8          ; Scratch space
