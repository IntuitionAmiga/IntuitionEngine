; ie68.inc - Intuition Engine M68K (68020) Macro Library
;
; Hardware definitions for writing M68K assembly programs targeting
; the Intuition Engine virtual machine.
;
; For use with vasmm68k_mot assembler
; Include with: include "ie68.inc"
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; MEMORY MAP CONSTANTS
; ============================================================================

; System vectors
VECTOR_BASE     equ $000000
VECTOR_RESET    equ $000000
VECTOR_IRQ      equ $000004

; Program space
PROGRAM_START   equ $001000

; Bank window addresses (for 8-bit CPU compatibility)
BANK1_WINDOW    equ $002000             ; Sprite data bank window (8KB)
BANK2_WINDOW    equ $004000             ; Font data bank window (8KB)
BANK3_WINDOW    equ $006000             ; General data bank window (8KB)
VRAM_WINDOW     equ $008000             ; VRAM bank window (16KB)
BANK_SIZE       equ $002000             ; 8KB per extended bank
VRAM_BANK_SIZE  equ $004000             ; 16KB VRAM bank

; Bank control registers
BANK1_REG_LO    equ $00F700             ; Bank 1 select (low byte)
BANK1_REG_HI    equ $00F701             ; Bank 1 select (high byte)
BANK2_REG_LO    equ $00F702             ; Bank 2 select (low byte)
BANK2_REG_HI    equ $00F703             ; Bank 2 select (high byte)
BANK3_REG_LO    equ $00F704             ; Bank 3 select (low byte)
BANK3_REG_HI    equ $00F705             ; Bank 3 select (high byte)
VRAM_BANK_REG   equ $00F7F0             ; VRAM bank select

; ============================================================================
; VIDEO HARDWARE REGISTERS (I/O region at $F0000)
; ============================================================================

VIDEO_CTRL      equ $F0000              ; Video control (enable/disable)
VIDEO_MODE      equ $F0004              ; Video mode (resolution)
VIDEO_STATUS    equ $F0008              ; Video status (VBlank flag)
STATUS_VBLANK   equ 2                   ; VBlank status bit

; Copper coprocessor
COPPER_CTRL     equ $F000C              ; Copper control
COPPER_PTR      equ $F0010              ; Copper list pointer (32-bit)

; Blitter hardware
BLT_CTRL        equ $F001C              ; Blitter control
BLT_OP          equ $F0020              ; Blitter operation
BLT_SRC         equ $F0024              ; Source address (32-bit)
BLT_DST         equ $F0028              ; Dest address (32-bit)
BLT_WIDTH       equ $F002C              ; Blit width (32-bit)
BLT_HEIGHT      equ $F0030              ; Blit height (32-bit)
BLT_SRC_STRIDE  equ $F0034              ; Source stride (32-bit)
BLT_DST_STRIDE  equ $F0038              ; Dest stride (32-bit)
BLT_COLOR       equ $F003C              ; Fill color (32-bit BGRA)
BLT_MASK        equ $F0040              ; Mask address (32-bit)
BLT_STATUS      equ $F0044              ; Blitter status

; Raster registers
VIDEO_RASTER_Y       equ $F0048         ; Raster Y position (32-bit)
VIDEO_RASTER_HEIGHT  equ $F004C         ; Raster height (32-bit)
VIDEO_RASTER_COLOR   equ $F0050         ; Raster color (32-bit BGRA)
VIDEO_RASTER_CTRL    equ $F0054         ; Raster control

; ============================================================================
; PSG/AUDIO REGISTERS (AY-3-8910 style)
; ============================================================================

PSG_PLUS_CTRL   equ $F0C0E              ; PSG+ mode control

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
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; PSG_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
PSG_PLAY_PTR    equ $F0C10              ; Play pointer (32-bit)
PSG_PLAY_LEN    equ $F0C14              ; Play length (32-bit)
PSG_PLAY_CTRL   equ $F0C18              ; Play control
PSG_PLAY_STATUS equ $F0C1C              ; Play status

; ============================================================================
; POKEY AUDIO REGISTERS (Atari style)
; ============================================================================

POKEY_BASE      equ $F0D00              ; POKEY base address
POKEY_AUDF1     equ $F0D00              ; Channel 1 frequency divider
POKEY_AUDC1     equ $F0D01              ; Channel 1 control (distortion + volume)
POKEY_AUDF2     equ $F0D02              ; Channel 2 frequency divider
POKEY_AUDC2     equ $F0D03              ; Channel 2 control
POKEY_AUDF3     equ $F0D04              ; Channel 3 frequency divider
POKEY_AUDC3     equ $F0D05              ; Channel 3 control
POKEY_AUDF4     equ $F0D06              ; Channel 4 frequency divider
POKEY_AUDC4     equ $F0D07              ; Channel 4 control
POKEY_AUDCTL    equ $F0D08              ; Master audio control
POKEY_PLUS_CTRL equ $F0D09              ; POKEY+ mode (0=standard, 1=enhanced)

; AUDCTL bit masks
AUDCTL_CLOCK_15KHZ  equ $01             ; Use 15kHz base clock (else 64kHz)
AUDCTL_HIPASS_CH1   equ $02             ; High-pass filter ch1 by ch3
AUDCTL_HIPASS_CH2   equ $04             ; High-pass filter ch2 by ch4
AUDCTL_CH4_BY_CH3   equ $08             ; Ch4 clocked by ch3 (16-bit mode)
AUDCTL_CH2_BY_CH1   equ $10             ; Ch2 clocked by ch1 (16-bit mode)
AUDCTL_CH3_179MHZ   equ $20             ; Ch3 uses 1.79MHz clock
AUDCTL_CH1_179MHZ   equ $40             ; Ch1 uses 1.79MHz clock
AUDCTL_POLY9        equ $80             ; Use 9-bit poly instead of 17-bit

; AUDC distortion modes (bits 5-7)
POKEY_DIST_POLY17_POLY5 equ $00         ; 17-bit + 5-bit poly
POKEY_DIST_POLY5        equ $20         ; 5-bit poly only
POKEY_DIST_POLY17_POLY4 equ $40         ; 17-bit + 4-bit poly
POKEY_DIST_POLY5_POLY4  equ $60         ; 5-bit + 4-bit poly
POKEY_DIST_POLY17       equ $80         ; 17-bit poly only
POKEY_DIST_PURE_TONE    equ $A0         ; Pure square wave
POKEY_DIST_POLY4        equ $C0         ; 4-bit poly only
POKEY_DIST_POLY17_PULSE equ $E0         ; 17-bit + pulse

; ----------------------------------------------------------------------------
; SID AUDIO REGISTERS (MOS 6581/8580 style)
;
; The SID chip provides 3 voices with analog-style synthesis:
; - 16-bit frequency control per voice
; - 12-bit pulse width for duty cycle
; - 4 waveforms: triangle, sawtooth, pulse, noise
; - ADSR envelope per voice
; - Ring modulation and hard sync
; - Programmable resonant filter (LP/BP/HP)
;
; SID+ mode (write 1 to SID_PLUS_CTRL) enables enhanced audio:
; - 4x oversampling for cleaner waveforms
; - Soft low-pass filtering
; - Subtle saturation for analog warmth
; - Room ambience effect
; ----------------------------------------------------------------------------
SID_BASE        equ $F0E00              ; SID base address

; Voice 1 registers
SID_V1_FREQ_LO  equ $F0E00              ; Voice 1 frequency low byte
SID_V1_FREQ_HI  equ $F0E01              ; Voice 1 frequency high byte
SID_V1_PW_LO    equ $F0E02              ; Voice 1 pulse width low byte
SID_V1_PW_HI    equ $F0E03              ; Voice 1 pulse width high (bits 0-3)
SID_V1_CTRL     equ $F0E04              ; Voice 1 control register
SID_V1_AD       equ $F0E05              ; Voice 1 attack/decay
SID_V1_SR       equ $F0E06              ; Voice 1 sustain/release

; Voice 2 registers
SID_V2_FREQ_LO  equ $F0E07              ; Voice 2 frequency low byte
SID_V2_FREQ_HI  equ $F0E08              ; Voice 2 frequency high byte
SID_V2_PW_LO    equ $F0E09              ; Voice 2 pulse width low byte
SID_V2_PW_HI    equ $F0E0A              ; Voice 2 pulse width high
SID_V2_CTRL     equ $F0E0B              ; Voice 2 control register
SID_V2_AD       equ $F0E0C              ; Voice 2 attack/decay
SID_V2_SR       equ $F0E0D              ; Voice 2 sustain/release

; Voice 3 registers
SID_V3_FREQ_LO  equ $F0E0E              ; Voice 3 frequency low byte
SID_V3_FREQ_HI  equ $F0E0F              ; Voice 3 frequency high byte
SID_V3_PW_LO    equ $F0E10              ; Voice 3 pulse width low byte
SID_V3_PW_HI    equ $F0E11              ; Voice 3 pulse width high
SID_V3_CTRL     equ $F0E12              ; Voice 3 control register
SID_V3_AD       equ $F0E13              ; Voice 3 attack/decay
SID_V3_SR       equ $F0E14              ; Voice 3 sustain/release

; Filter and volume registers
SID_FC_LO       equ $F0E15              ; Filter cutoff low (bits 0-2)
SID_FC_HI       equ $F0E16              ; Filter cutoff high byte
SID_RES_FILT    equ $F0E17              ; Resonance (bits 4-7) + routing (bits 0-3)
SID_MODE_VOL    equ $F0E18              ; Volume (bits 0-3) + filter mode (bits 4-7)
SID_PLUS_CTRL   equ $F0E19              ; SID+ mode (0=standard, 1=enhanced)

; Voice control register bits (SID_Vx_CTRL)
SID_CTRL_GATE     equ $01               ; Gate bit (trigger envelope)
SID_CTRL_SYNC     equ $02               ; Sync with previous voice
SID_CTRL_RINGMOD  equ $04               ; Ring modulation
SID_CTRL_TEST     equ $08               ; Test bit (resets oscillator)
SID_CTRL_TRIANGLE equ $10               ; Triangle waveform
SID_CTRL_SAWTOOTH equ $20               ; Sawtooth waveform
SID_CTRL_PULSE    equ $40               ; Pulse/square waveform
SID_CTRL_NOISE    equ $80               ; Noise waveform

; Filter routing bits (SID_RES_FILT bits 0-3)
SID_FILT_V1       equ $01               ; Route voice 1 through filter
SID_FILT_V2       equ $02               ; Route voice 2 through filter
SID_FILT_V3       equ $04               ; Route voice 3 through filter
SID_FILT_EXT      equ $08               ; Route external input through filter

; Filter mode bits (SID_MODE_VOL bits 4-7)
SID_MODE_LP       equ $10               ; Low-pass filter
SID_MODE_BP       equ $20               ; Band-pass filter
SID_MODE_HP       equ $40               ; High-pass filter
SID_MODE_3OFF     equ $80               ; Disconnect voice 3 from output

; ----------------------------------------------------------------------------
; SAP Player Registers
;
; The SAP player handles Atari 8-bit music files (.sap) which contain
; embedded 6502 code that drives the POKEY sound chip. Supports TYPE B files
; where INIT is called once and PLAYER is called each frame.
;
; Usage:
;   1. Embed SAP data in your program using incbin
;   2. Set SAP_PLAY_PTR to the address of the SAP data
;   3. Set SAP_PLAY_LEN to the size of the SAP data in bytes
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
SAP_PLAY_PTR    equ $F0D10              ; Play pointer (32-bit)
SAP_PLAY_LEN    equ $F0D14              ; Play length (32-bit)
SAP_PLAY_CTRL   equ $F0D18              ; Play control
SAP_PLAY_STATUS equ $F0D1C              ; Play status
SAP_SUBSONG     equ $F0D1D              ; Subsong selection (0-255)

; ============================================================================
; VIDEO CONSTANTS
; ============================================================================

VRAM_START      equ $100000             ; VRAM starts at 1MB
SCREEN_W        equ 640                 ; Default screen width
SCREEN_H        equ 480                 ; Default screen height
LINE_BYTES      equ 2560                ; Bytes per scanline (640 * 4)

; Blitter operations
BLT_OP_COPY     equ 0                   ; Copy source to dest
BLT_OP_FILL     equ 1                   ; Fill rect with color
BLT_OP_LINE     equ 2                   ; Draw line
BLT_OP_MASKED   equ 3                   ; Copy with mask
BLT_OP_ALPHA    equ 4                   ; Alpha blending

; Background color (black with alpha)
BACKGROUND      equ $FF000000           ; BGRA: Black, full alpha

; ============================================================================
; COPPER OPCODES
; ============================================================================

; Copper WAIT opcode: wait for raster position
; Format: $0YYY_HHHH where YYY = Y position (0-4095), HHHH = H position
COP_WAIT_MASK        equ $00000000

; Copper MOVE opcodes
COP_MOVE_RASTER_Y     equ $40120000
COP_MOVE_RASTER_H     equ $40130000
COP_MOVE_RASTER_COLOR equ $40140000
COP_MOVE_RASTER_CTRL  equ $40150000

; Copper END opcode
COP_END              equ $C0000000

; ============================================================================
; TIMER REGISTERS
; ============================================================================

TIMER_CTRL      equ $00F800             ; Timer control
TIMER_COUNT     equ $00F804             ; Timer count
TIMER_RELOAD    equ $00F808             ; Timer reload value

; ============================================================================
; MACROS
; ============================================================================

; Wait for VBlank
wait_vblank macro
.wait_not_vblank\@:
    move.l  VIDEO_STATUS,d0
    and.l   #STATUS_VBLANK,d0
    bne.s   .wait_not_vblank\@
.wait_vblank\@:
    move.l  VIDEO_STATUS,d0
    and.l   #STATUS_VBLANK,d0
    beq.s   .wait_vblank\@
    endm

; Wait for blitter to finish
wait_blit macro
.wait_blit\@:
    move.l  BLT_CTRL,d0
    and.l   #2,d0
    bne.s   .wait_blit\@
    endm

; Start a blitter operation
start_blit macro
    move.l  #1,BLT_CTRL
    endm

; Set blitter fill color (32-bit BGRA)
set_blt_color macro color
    move.l  #\color,BLT_COLOR
    endm

; Set blitter source address
set_blt_src macro addr
    move.l  #\addr,BLT_SRC
    endm

; Set blitter dest address
set_blt_dst macro addr
    move.l  #\addr,BLT_DST
    endm

; Set blitter dimensions
set_blt_size macro width,height
    move.l  #\width,BLT_WIDTH
    move.l  #\height,BLT_HEIGHT
    endm

; Set blitter strides
set_blt_strides macro src_stride,dst_stride
    move.l  #\src_stride,BLT_SRC_STRIDE
    move.l  #\dst_stride,BLT_DST_STRIDE
    endm

; Set copper list pointer
set_copper_ptr macro addr
    move.l  #\addr,COPPER_PTR
    endm

; Enable copper
enable_copper macro
    move.l  #1,COPPER_CTRL
    endm

; Disable copper
disable_copper macro
    move.l  #0,COPPER_CTRL
    endm

; Set PSG+ play pointer and length
set_psg_play macro ptr,len
    move.l  #\ptr,PSG_PLAY_PTR
    move.l  #\len,PSG_PLAY_LEN
    endm

; Start PSG+ playback
start_psg_play macro
    move.l  #1,PSG_PLAY_CTRL
    endm

; Stop PSG+ playback
stop_psg_play macro
    move.l  #0,PSG_PLAY_CTRL
    endm

; Enable PSG+ enhanced mode
enable_psg_plus macro
    move.b  #1,PSG_PLUS_CTRL
    endm

; Set POKEY channel (freq, control)
set_pokey_ch macro ch,freq,ctrl
    move.b  #\freq,POKEY_AUDF1+(\ch*2)
    move.b  #\ctrl,POKEY_AUDC1+(\ch*2)
    endm

; Enable POKEY+ enhanced mode
enable_pokey_plus macro
    move.b  #1,POKEY_PLUS_CTRL
    endm

; Set POKEY master control
set_pokey_audctl macro val
    move.b  #\val,POKEY_AUDCTL
    endm

; Set SAP play pointer and length
set_sap_play macro ptr,len
    move.l  #\ptr,SAP_PLAY_PTR
    move.l  #\len,SAP_PLAY_LEN
    endm

; Set SAP subsong
set_sap_subsong macro num
    move.b  #\num,SAP_SUBSONG
    endm

; Start SAP playback
start_sap_play macro
    move.l  #1,SAP_PLAY_CTRL
    endm

; Start SAP playback with looping
start_sap_loop macro
    move.l  #5,SAP_PLAY_CTRL
    endm

; Stop SAP playback
stop_sap_play macro
    move.l  #2,SAP_PLAY_CTRL
    endm

; ============================================================================
; EOF
; ============================================================================
