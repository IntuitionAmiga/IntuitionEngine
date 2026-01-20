; ie80.inc - Intuition Engine Z80 (IE80) Macro Library
;
; Hardware definitions and utility macros for writing Z80 assembly
; programs targeting the Intuition Engine virtual machine.
;
; For use with vasmz80_std assembler
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; MEMORY MAP CONSTANTS
; ============================================================================

; Bank window addresses (same as 6502 for compatibility)
.set BANK1_WINDOW,0x2000
.set BANK2_WINDOW,0x4000
.set BANK3_WINDOW,0x6000
.set VRAM_WINDOW,0x8000
.set BANK_SIZE,0x2000
.set VRAM_BANK_SIZE,0x4000

; Bank control registers (memory-mapped)
.set BANK1_REG_LO,0xF700
.set BANK1_REG_HI,0xF701
.set BANK2_REG_LO,0xF702
.set BANK2_REG_HI,0xF703
.set BANK3_REG_LO,0xF704
.set BANK3_REG_HI,0xF705
.set VRAM_BANK_REG,0xF7F0

; ============================================================================
; VIDEO HARDWARE REGISTERS
; ============================================================================

.set VIDEO_CTRL,0xF000
.set VIDEO_MODE,0xF004
.set VIDEO_STATUS,0xF008
.set STATUS_VBLANK,2

; Copper coprocessor
.set COPPER_CTRL,0xF00C
.set COPPER_PTR_0,0xF010
.set COPPER_PTR_1,0xF011
.set COPPER_PTR_2,0xF012
.set COPPER_PTR_3,0xF013

; Blitter hardware
.set BLT_CTRL,0xF01C
.set BLT_OP,0xF020
.set BLT_SRC_0,0xF024
.set BLT_SRC_1,0xF025
.set BLT_SRC_2,0xF026
.set BLT_SRC_3,0xF027
.set BLT_DST_0,0xF028
.set BLT_DST_1,0xF029
.set BLT_DST_2,0xF02A
.set BLT_DST_3,0xF02B
.set BLT_WIDTH_LO,0xF02C
.set BLT_WIDTH_HI,0xF02D
.set BLT_HEIGHT_LO,0xF030
.set BLT_HEIGHT_HI,0xF031
.set BLT_SRC_STRIDE_LO,0xF034
.set BLT_SRC_STRIDE_HI,0xF035
.set BLT_DST_STRIDE_LO,0xF038
.set BLT_DST_STRIDE_HI,0xF039
.set BLT_COLOR_0,0xF03C
.set BLT_COLOR_1,0xF03D
.set BLT_COLOR_2,0xF03E
.set BLT_COLOR_3,0xF03F
.set BLT_MASK_0,0xF040
.set BLT_MASK_1,0xF041
.set BLT_MASK_2,0xF042
.set BLT_MASK_3,0xF043
.set BLT_STATUS,0xF044

; Raster registers
.set VIDEO_RASTER_Y_LO,0xF048
.set VIDEO_RASTER_Y_HI,0xF049
.set VIDEO_RASTER_HEIGHT_LO,0xF04C
.set VIDEO_RASTER_HEIGHT_HI,0xF04D
.set VIDEO_RASTER_COLOR_0,0xF050
.set VIDEO_RASTER_COLOR_1,0xF051
.set VIDEO_RASTER_COLOR_2,0xF052
.set VIDEO_RASTER_COLOR_3,0xF053
.set VIDEO_RASTER_CTRL,0xF054

; ============================================================================
; PSG/AUDIO REGISTERS
; ============================================================================

.set PSG_PLUS_CTRL,0xFC0E

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
;   bit 0 (0x01) - Start playback (write 1 to begin)
;   bit 1 (0x02) - Stop playback (write 1 to stop)
;   bit 2 (0x04) - Enable looping (set before starting)
;
; PSG_PLAY_STATUS bits:
;   bit 0 (0x01) - Busy (1 = playing, 0 = stopped)
;   bit 1 (0x02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
.set PSG_PLAY_PTR_0,0xFC10
.set PSG_PLAY_PTR_1,0xFC11
.set PSG_PLAY_PTR_2,0xFC12
.set PSG_PLAY_PTR_3,0xFC13
.set PSG_PLAY_LEN_0,0xFC14
.set PSG_PLAY_LEN_1,0xFC15
.set PSG_PLAY_LEN_2,0xFC16
.set PSG_PLAY_LEN_3,0xFC17
.set PSG_PLAY_CTRL,0xFC18
.set PSG_PLAY_STATUS,0xFC1C

.set PSG_PORT_SELECT,0xF0
.set PSG_PORT_DATA,0xF1

; ============================================================================
; POKEY AUDIO REGISTERS
; ============================================================================

; POKEY memory-mapped registers
.set POKEY_BASE,0xFD00
.set POKEY_AUDF1,0xFD00      ; Channel 1 frequency divider
.set POKEY_AUDC1,0xFD01      ; Channel 1 control (distortion + volume)
.set POKEY_AUDF2,0xFD02      ; Channel 2 frequency divider
.set POKEY_AUDC2,0xFD03      ; Channel 2 control
.set POKEY_AUDF3,0xFD04      ; Channel 3 frequency divider
.set POKEY_AUDC3,0xFD05      ; Channel 3 control
.set POKEY_AUDF4,0xFD06      ; Channel 4 frequency divider
.set POKEY_AUDC4,0xFD07      ; Channel 4 control
.set POKEY_AUDCTL,0xFD08     ; Master audio control
.set POKEY_PLUS_CTRL,0xFD09  ; POKEY+ mode (0=standard, 1=enhanced)

; POKEY I/O port mapping for Z80
.set POKEY_PORT_SELECT,0xD0
.set POKEY_PORT_DATA,0xD1

; AUDCTL bit masks
.set AUDCTL_CLOCK_15KHZ,0x01 ; Use 15kHz base clock (else 64kHz)
.set AUDCTL_HIPASS_CH1,0x02  ; High-pass filter ch1 by ch3
.set AUDCTL_HIPASS_CH2,0x04  ; High-pass filter ch2 by ch4
.set AUDCTL_CH4_BY_CH3,0x08  ; Ch4 clocked by ch3 (16-bit mode)
.set AUDCTL_CH2_BY_CH1,0x10  ; Ch2 clocked by ch1 (16-bit mode)
.set AUDCTL_CH3_179MHZ,0x20  ; Ch3 uses 1.79MHz clock
.set AUDCTL_CH1_179MHZ,0x40  ; Ch1 uses 1.79MHz clock
.set AUDCTL_POLY9,0x80       ; Use 9-bit poly instead of 17-bit

; AUDC distortion modes (bits 5-7)
.set POKEY_DIST_POLY17_POLY5,0x00  ; 17-bit + 5-bit poly
.set POKEY_DIST_POLY5,0x20         ; 5-bit poly only
.set POKEY_DIST_POLY17_POLY4,0x40  ; 17-bit + 4-bit poly
.set POKEY_DIST_POLY5_POLY4,0x60   ; 5-bit + 4-bit poly
.set POKEY_DIST_POLY17,0x80        ; 17-bit poly only
.set POKEY_DIST_PURE_TONE,0xA0     ; Pure square wave
.set POKEY_DIST_POLY4,0xC0         ; 4-bit poly only
.set POKEY_DIST_POLY17_PULSE,0xE0  ; 17-bit + pulse

; ============================================================================
; VIDEO CONSTANTS
; ============================================================================

.set VRAM_START,0x100000
.set SCREEN_W,640
.set SCREEN_H,480
.set LINE_BYTES,2560

; Blitter operations
.set BLT_OP_COPY,0
.set BLT_OP_FILL,1
.set BLT_OP_LINE,2
.set BLT_OP_MASKED,3
.set BLT_OP_ALPHA,4

; Background color
.set BACKGROUND_0,0x00
.set BACKGROUND_1,0x00
.set BACKGROUND_2,0x00
.set BACKGROUND_3,0xFF

; ============================================================================
; COPPER OPCODES
; ============================================================================

.set COP_WAIT_MASK,0x00000000
.set COP_MOVE_RASTER_Y,0x40120000
.set COP_MOVE_RASTER_H,0x40130000
.set COP_MOVE_RASTER_COLOR,0x40140000
.set COP_MOVE_RASTER_CTRL,0x40150000
.set COP_END,0xC0000000

; ============================================================================
; MACROS
; ============================================================================

; Set bank 1 (sprite data)
.macro SET_BANK1 bank
    ld a,<(\bank)
    ld (BANK1_REG_LO),a
    ld a,>(\bank)
    ld (BANK1_REG_HI),a
.endm

; Set bank 2 (font data)
.macro SET_BANK2 bank
    ld a,<(\bank)
    ld (BANK2_REG_LO),a
    ld a,>(\bank)
    ld (BANK2_REG_HI),a
.endm

; Set bank 3 (general data)
.macro SET_BANK3 bank
    ld a,<(\bank)
    ld (BANK3_REG_LO),a
    ld a,>(\bank)
    ld (BANK3_REG_HI),a
.endm

; Set VRAM bank
.macro SET_VRAM_BANK bank
    ld a,\bank
    ld (VRAM_BANK_REG),a
.endm

; Store 16-bit value
.macro STORE16 addr value
    ld a,<(\value)
    ld (\addr),a
    ld a,>(\value)
    ld (\addr+1),a
.endm

; Store 32-bit value (little-endian)
.macro STORE32 addr value
    ld a,(\value)&0xFF
    ld (\addr),a
    ld a,((\value)>>8)&0xFF
    ld (\addr+1),a
    ld a,((\value)>>16)&0xFF
    ld (\addr+2),a
    ld a,((\value)>>24)&0xFF
    ld (\addr+3),a
.endm

; Wait for VBlank
.macro WAIT_VBLANK
.wait_not_vblank\@:
    ld a,(VIDEO_STATUS)
    and STATUS_VBLANK
    jr nz,.wait_not_vblank\@
.wait_vblank\@:
    ld a,(VIDEO_STATUS)
    and STATUS_VBLANK
    jr z,.wait_vblank\@
.endm

; Wait for blitter
.macro WAIT_BLIT
.wait_blit\@:
    ld a,(BLT_CTRL)
    and 2
    jr nz,.wait_blit\@
.endm

; Start blitter
.macro START_BLIT
    ld a,1
    ld (BLT_CTRL),a
.endm

; Set blitter operation
.macro SET_BLT_OP op
    ld a,\op
    ld (BLT_OP),a
.endm

; Set blitter width
.macro SET_BLT_WIDTH width
    ld a,<(\width)
    ld (BLT_WIDTH_LO),a
    ld a,>(\width)
    ld (BLT_WIDTH_HI),a
.endm

; Set blitter height
.macro SET_BLT_HEIGHT height
    ld a,<(\height)
    ld (BLT_HEIGHT_LO),a
    ld a,>(\height)
    ld (BLT_HEIGHT_HI),a
.endm

; Set source stride
.macro SET_SRC_STRIDE stride
    ld a,<(\stride)
    ld (BLT_SRC_STRIDE_LO),a
    ld a,>(\stride)
    ld (BLT_SRC_STRIDE_HI),a
.endm

; Set dest stride
.macro SET_DST_STRIDE stride
    ld a,<(\stride)
    ld (BLT_DST_STRIDE_LO),a
    ld a,>(\stride)
    ld (BLT_DST_STRIDE_HI),a
.endm

; Set blitter fill color (32-bit BGRA)
.macro SET_BLT_COLOR color
    ld a,(\color)&0xFF
    ld (BLT_COLOR_0),a
    ld a,((\color)>>8)&0xFF
    ld (BLT_COLOR_1),a
    ld a,((\color)>>16)&0xFF
    ld (BLT_COLOR_2),a
    ld a,((\color)>>24)&0xFF
    ld (BLT_COLOR_3),a
.endm

; Set blitter source address (32-bit)
.macro SET_BLT_SRC addr
    ld a,(\addr)&0xFF
    ld (BLT_SRC_0),a
    ld a,((\addr)>>8)&0xFF
    ld (BLT_SRC_1),a
    ld a,((\addr)>>16)&0xFF
    ld (BLT_SRC_2),a
    ld a,((\addr)>>24)&0xFF
    ld (BLT_SRC_3),a
.endm

; Set blitter dest address (32-bit)
.macro SET_BLT_DST addr
    ld a,(\addr)&0xFF
    ld (BLT_DST_0),a
    ld a,((\addr)>>8)&0xFF
    ld (BLT_DST_1),a
    ld a,((\addr)>>16)&0xFF
    ld (BLT_DST_2),a
    ld a,((\addr)>>24)&0xFF
    ld (BLT_DST_3),a
.endm

; Set blitter mask address (32-bit)
.macro SET_BLT_MASK addr
    ld a,(\addr)&0xFF
    ld (BLT_MASK_0),a
    ld a,((\addr)>>8)&0xFF
    ld (BLT_MASK_1),a
    ld a,((\addr)>>16)&0xFF
    ld (BLT_MASK_2),a
    ld a,((\addr)>>24)&0xFF
    ld (BLT_MASK_3),a
.endm

; Set copper list pointer (32-bit)
.macro SET_COPPER_PTR addr
    ld a,(\addr)&0xFF
    ld (COPPER_PTR_0),a
    ld a,((\addr)>>8)&0xFF
    ld (COPPER_PTR_1),a
    ld a,((\addr)>>16)&0xFF
    ld (COPPER_PTR_2),a
    ld a,((\addr)>>24)&0xFF
    ld (COPPER_PTR_3),a
.endm

; Set PSG+ play pointer (32-bit)
.macro SET_PSG_PTR addr
    ld a,(\addr)&0xFF
    ld (PSG_PLAY_PTR_0),a
    ld a,((\addr)>>8)&0xFF
    ld (PSG_PLAY_PTR_1),a
    ld a,((\addr)>>16)&0xFF
    ld (PSG_PLAY_PTR_2),a
    ld a,((\addr)>>24)&0xFF
    ld (PSG_PLAY_PTR_3),a
.endm

; Set PSG+ play length (32-bit)
.macro SET_PSG_LEN len
    ld a,(\len)&0xFF
    ld (PSG_PLAY_LEN_0),a
    ld a,((\len)>>8)&0xFF
    ld (PSG_PLAY_LEN_1),a
    ld a,((\len)>>16)&0xFF
    ld (PSG_PLAY_LEN_2),a
    ld a,((\len)>>24)&0xFF
    ld (PSG_PLAY_LEN_3),a
.endm

; ============================================================================
; UTILITY MACROS
; ============================================================================

; Add 16-bit immediate to HL
.macro ADD_HL_IMM value
    ld bc,\value
    add hl,bc
.endm

; Compare HL with 16-bit immediate
.macro CP_HL_IMM value
    push hl
    ld bc,-(\value)
    add hl,bc
    pop hl
.endm

; Load HL from memory
.macro LD_HL_MEM addr
    ld hl,(\addr)
.endm

; Store HL to memory
.macro ST_HL_MEM addr
    ld (\addr),hl
.endm

; Increment 16-bit memory location
.macro INC16 addr
    ld hl,(\addr)
    inc hl
    ld (\addr),hl
.endm

; ============================================================================
; STACK SETUP
; ============================================================================

.set STACK_TOP,0xF000

; ============================================================================
; EOF
; ============================================================================
