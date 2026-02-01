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
STACK_TOP       equ $FF0000             ; Top of stack (grows downward)

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
; SID Player Registers
;
; The SID player handles Commodore 64 music files (.sid) which contain
; embedded 6502 code that drives the SID sound chip. The player executes
; the 6502 init routine once, then calls the play routine each frame.
;
; Usage:
;   1. Embed SID data in your program using incbin
;   2. Set SID_PLAY_PTR to the address of the SID file data
;   3. Set SID_PLAY_LEN to the size of the SID data in bytes
;   4. Optionally set SID_SUBSONG to select a subsong (default 0)
;   5. Write to SID_PLAY_CTRL to start/stop playback
;
; SID_PLAY_CTRL bits:
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; SID_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
SID_PLAY_PTR    equ $F0E20              ; Play pointer (32-bit)
SID_PLAY_LEN    equ $F0E24              ; Play length (32-bit)
SID_PLAY_CTRL   equ $F0E28              ; Play control
SID_PLAY_STATUS equ $F0E2C              ; Play status
SID_SUBSONG     equ $F0E2D              ; Subsong selection (0-255)

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
; TED AUDIO REGISTERS (Commodore Plus/4 style)
;
; The TED chip provides 2 voices with simple square wave synthesis:
; - 10-bit frequency control per voice (0-1023)
; - Voice 2 can optionally produce white noise
; - Global 4-bit volume (0-8, where 8 is maximum)
;
; TED+ mode (write 1 to TED_PLUS_CTRL) enables enhanced audio:
; - 4x oversampling for cleaner waveforms
; - Soft low-pass filtering for smoother output
; - Subtle saturation for analog warmth
; - Room reverb for spatial depth
; - Logarithmic volume curve for more musical response
; - Per-voice stereo separation
; ============================================================================
TED_BASE        equ $F0F00              ; TED base address
TED_FREQ1_LO    equ $F0F00              ; Voice 1 frequency low byte
TED_FREQ2_LO    equ $F0F01              ; Voice 2 frequency low byte
TED_FREQ2_HI    equ $F0F02              ; Voice 2 frequency high (bits 0-1)
TED_SND_CTRL    equ $F0F03              ; Sound control register
TED_FREQ1_HI    equ $F0F04              ; Voice 1 frequency high (bits 0-1)
TED_PLUS_CTRL   equ $F0F05              ; TED+ mode (0=standard, 1=enhanced)

; TED Control register bits (TED_SND_CTRL)
TED_CTRL_SNDDC     equ $80              ; D/A mode (bit 7)
TED_CTRL_SND2NOISE equ $40              ; Voice 2 noise enable (bit 6)
TED_CTRL_SND2ON    equ $20              ; Voice 2 enable (bit 5)
TED_CTRL_SND1ON    equ $10              ; Voice 1 enable (bit 4)
TED_CTRL_VOLUME    equ $0F              ; Volume mask (bits 0-3, max=8)

; TED Player Registers
TED_PLAY_PTR    equ $F0F10              ; Play pointer (32-bit)
TED_PLAY_LEN    equ $F0F14              ; Play length (32-bit)
TED_PLAY_CTRL   equ $F0F18              ; Play control
TED_PLAY_STATUS equ $F0F1C              ; Play status

; ============================================================================
; TED VIDEO REGISTERS (Commodore Plus/4 style)
;
; The TED chip also provides video capabilities:
; - 40x25 text mode (8x8 character cells)
; - 320x200 pixel resolution (384x272 with border)
; - 121 colors (16 hues × 8 luminances)
; - Hardware cursor support
; - Compositor layer 12 (between VGA=10 and ULA=15)
;
; All registers are 4-byte aligned for copper SETBASE compatibility.
; ============================================================================
TED_VIDEO_BASE    equ $F0F20            ; TED video register base

; Control registers
TED_V_CTRL1       equ $F0F20            ; Control 1 (ECM/BMM/DEN/RSEL/YSCROLL)
TED_V_CTRL2       equ $F0F24            ; Control 2 (RES/MCM/CSEL/XSCROLL)
TED_V_CHAR_BASE   equ $F0F28            ; Character/bitmap base address
TED_V_VIDEO_BASE  equ $F0F2C            ; Video matrix base address

; Color registers
TED_V_BG_COLOR0   equ $F0F30            ; Background color 0
TED_V_BG_COLOR1   equ $F0F34            ; Background color 1 (multicolor)
TED_V_BG_COLOR2   equ $F0F38            ; Background color 2 (multicolor)
TED_V_BG_COLOR3   equ $F0F3C            ; Background color 3 (multicolor)
TED_V_BORDER      equ $F0F40            ; Border color

; Cursor registers
TED_V_CURSOR_HI   equ $F0F44            ; Cursor position high byte
TED_V_CURSOR_LO   equ $F0F48            ; Cursor position low byte
TED_V_CURSOR_CLR  equ $F0F4C            ; Cursor color

; Status registers
TED_V_RASTER_LO   equ $F0F50            ; Raster line low (read-only)
TED_V_RASTER_HI   equ $F0F54            ; Raster line high (read-only)
TED_V_ENABLE      equ $F0F58            ; Video enable (bit 0)
TED_V_STATUS      equ $F0F5C            ; Status (bit 0 = VBlank)

; TED_V_CTRL1 bits
TED_V_CTRL1_ECM      equ $40            ; Bit 6: Extended Color Mode
TED_V_CTRL1_BMM      equ $20            ; Bit 5: Bitmap Mode
TED_V_CTRL1_DEN      equ $10            ; Bit 4: Display Enable
TED_V_CTRL1_RSEL     equ $08            ; Bit 3: Row Select (0=24, 1=25)
TED_V_CTRL1_YSCROLL  equ $07            ; Bits 0-2: Vertical scroll mask

; TED_V_CTRL2 bits
TED_V_CTRL2_RES      equ $20            ; Bit 5: Reset
TED_V_CTRL2_MCM      equ $10            ; Bit 4: Multicolor Mode
TED_V_CTRL2_CSEL     equ $08            ; Bit 3: Column Select (0=38, 1=40)
TED_V_CTRL2_XSCROLL  equ $07            ; Bits 0-2: Horizontal scroll mask

; Enable/Status bits
TED_V_ENABLE_VIDEO   equ $01            ; Bit 0: Video enable
TED_V_STATUS_VBLANK  equ $01            ; Bit 0: VBlank active

; Display dimensions
TED_V_DISPLAY_W   equ 320               ; Display width in pixels
TED_V_DISPLAY_H   equ 200               ; Display height in pixels
TED_V_FRAME_W     equ 384               ; Total frame width (with border)
TED_V_FRAME_H     equ 272               ; Total frame height (with border)
TED_V_CELLS_X     equ 40                ; Text columns
TED_V_CELLS_Y     equ 25                ; Text rows

; Copper SETBASE for TED video
COP_SETBASE_TED_VIDEO equ $8003C3C8     ; TED_VIDEO_BASE ($F0F20 >> 2)

; ----- TED Video helper macros -----

; Enable TED video
ted_v_enable    macro
                move.b  #TED_V_ENABLE_VIDEO,TED_V_ENABLE
                endm

; Disable TED video
ted_v_disable   macro
                move.b  #0,TED_V_ENABLE
                endm

; Set TED border color (hue 0-15, luminance 0-7)
ted_v_border    macro   hue,lum
                move.b  #((\lum&7)<<4)|(\hue&$F),TED_V_BORDER
                endm

; Set TED background color 0 (hue 0-15, luminance 0-7)
ted_v_bgcolor   macro   hue,lum
                move.b  #((\lum&7)<<4)|(\hue&$F),TED_V_BG_COLOR0
                endm

; Wait for TED VBlank
ted_v_wait_vblank  macro
.wait_ted_vb\@:
                move.b  TED_V_STATUS,d0
                and.b   #TED_V_STATUS_VBLANK,d0
                beq.s   .wait_ted_vb\@
                endm

; ============================================================================
; AHX AUDIO REGISTERS (Amiga AHX module player)
;
; The AHX engine provides Amiga AHX/THX module playback:
; - 4 channels with waveform synthesis (triangle, sawtooth, square, noise)
; - Per-channel filter modulation
; - Instrument-based sequencing with effects
;
; AHX+ mode (write 1 to AHX_PLUS_CTRL) enables enhanced audio:
; - 4x oversampling for cleaner waveforms
; - Soft low-pass filtering for smoother output
; - Subtle saturation for analog warmth
; - Room reverb for Amiga-style ambience
; - Authentic stereo panning (L-R-R-L pattern)
; - Hardware PWM for square wave duty cycle modulation
; ============================================================================
AHX_BASE        equ $F0B80              ; AHX base address
AHX_PLUS_CTRL   equ $F0B80              ; AHX+ mode (0=standard, 1=enhanced)

; ----------------------------------------------------------------------------
; AHX Player Registers
;
; The AHX player handles Amiga AHX/THX modules which contain instrument
; definitions and pattern data for 4-channel waveform synthesis.
;
; Usage:
;   1. Embed AHX data in your program using incbin
;   2. Set AHX_PLAY_PTR to the address of the AHX file data
;   3. Set AHX_PLAY_LEN to the size of the AHX data in bytes
;   4. Optionally set AHX_SUBSONG to select a subsong (default 0)
;   5. Optionally write 1 to AHX_PLUS_CTRL to enable enhanced mode
;   6. Write to AHX_PLAY_CTRL to start/stop playback
;
; AHX_PLAY_CTRL bits:
;   bit 0 ($01) - Start playback (write 1 to begin)
;   bit 1 ($02) - Stop playback (write 1 to stop)
;   bit 2 ($04) - Enable looping (set before starting)
;
; AHX_PLAY_STATUS bits:
;   bit 0 ($01) - Busy (1 = playing, 0 = stopped)
;   bit 1 ($02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
AHX_PLAY_PTR    equ $F0B84              ; Play pointer (32-bit)
AHX_PLAY_LEN    equ $F0B88              ; Play length (32-bit)
AHX_PLAY_CTRL   equ $F0B8C              ; Play control
AHX_PLAY_STATUS equ $F0B90              ; Play status
AHX_SUBSONG     equ $F0B91              ; Subsong selection (0-255)

; ----------------------------------------------------------------------------
; AHX Player Macros
; ----------------------------------------------------------------------------

; PLAY_AHX - Load and start AHX playback
; Parameters:
;   \ptr - Address of AHX data
;   \len - Length of AHX data in bytes
; Example: PLAY_AHX ahx_data,ahx_data_end-ahx_data
PLAY_AHX    macro
    move.l  #\1,AHX_PLAY_PTR
    move.l  #\2,AHX_PLAY_LEN
    move.l  #1,AHX_PLAY_CTRL
    endm

; PLAY_AHX_LOOP - Load and start AHX playback with looping
; Parameters:
;   \ptr - Address of AHX data
;   \len - Length of AHX data in bytes
; Example: PLAY_AHX_LOOP ahx_data,ahx_data_end-ahx_data
PLAY_AHX_LOOP    macro
    move.l  #\1,AHX_PLAY_PTR
    move.l  #\2,AHX_PLAY_LEN
    move.l  #5,AHX_PLAY_CTRL
    endm

; PLAY_AHX_PLUS - Load and start AHX+ enhanced playback
; Parameters:
;   \ptr - Address of AHX data
;   \len - Length of AHX data in bytes
; Example: PLAY_AHX_PLUS ahx_data,ahx_data_end-ahx_data
PLAY_AHX_PLUS    macro
    move.l  #1,AHX_PLUS_CTRL
    move.l  #\1,AHX_PLAY_PTR
    move.l  #\2,AHX_PLAY_LEN
    move.l  #1,AHX_PLAY_CTRL
    endm

; PLAY_AHX_PLUS_LOOP - Load and start AHX+ enhanced playback with looping
; Parameters:
;   \ptr - Address of AHX data
;   \len - Length of AHX data in bytes
; Example: PLAY_AHX_PLUS_LOOP ahx_data,ahx_data_end-ahx_data
PLAY_AHX_PLUS_LOOP    macro
    move.l  #1,AHX_PLUS_CTRL
    move.l  #\1,AHX_PLAY_PTR
    move.l  #\2,AHX_PLAY_LEN
    move.l  #5,AHX_PLAY_CTRL
    endm

; STOP_AHX - Stop AHX playback
STOP_AHX    macro
    move.l  #2,AHX_PLAY_CTRL
    endm

; ============================================================================
; VGA VIDEO REGISTERS (IBM VGA compatible)
;
; The VGA chip provides IBM PC-compatible graphics modes:
; - Mode $03: 80x25 text mode, 16 colors
; - Mode $12: 640x480, 16 colors, planar
; - Mode $13: 320x200, 256 colors, linear (Mode 13h)
; - Mode $14: 320x240, 256 colors, planar (Mode X)
;
; Features:
; - Full DAC/palette support (256 colors, 6-bit RGB)
; - Planar and linear memory modes
; - VSync status for timing synchronization
; - Page flipping via CRTC start address
; ============================================================================

VGA_BASE        equ $F1000
VGA_MODE        equ $F1000              ; Mode control
VGA_STATUS      equ $F1004              ; Status register
VGA_CTRL        equ $F1008              ; Control register

; Sequencer ($3C4/$3C5 equivalent)
VGA_SEQ_INDEX   equ $F1010              ; Sequencer index
VGA_SEQ_DATA    equ $F1014              ; Sequencer data
VGA_SEQ_MAPMASK equ $F1018              ; Plane write mask (direct)

; CRTC ($3D4/$3D5 equivalent)
VGA_CRTC_INDEX  equ $F1020              ; CRTC index
VGA_CRTC_DATA   equ $F1024              ; CRTC data
VGA_CRTC_STARTHI equ $F1028             ; Start address high
VGA_CRTC_STARTLO equ $F102C             ; Start address low

; Graphics Controller ($3CE/$3CF equivalent)
VGA_GC_INDEX    equ $F1030              ; GC index
VGA_GC_DATA     equ $F1034              ; GC data
VGA_GC_READMAP  equ $F1038              ; Read plane select (direct)
VGA_GC_BITMASK  equ $F103C              ; Bit mask (direct)

; Attribute Controller
VGA_ATTR_INDEX  equ $F1040              ; Attribute index
VGA_ATTR_DATA   equ $F1044              ; Attribute data

; DAC/Palette ($3C6-$3C9 equivalent)
VGA_DAC_MASK    equ $F1050              ; Pixel mask
VGA_DAC_RINDEX  equ $F1054              ; Read index
VGA_DAC_WINDEX  equ $F1058              ; Write index
VGA_DAC_DATA    equ $F105C              ; DAC data (R,G,B sequence)

; Palette RAM (256 x 3 bytes)
VGA_PALETTE     equ $F1100              ; Palette base (768 bytes)

; VGA VRAM (64KB window)
VGA_VRAM        equ $A0000              ; VRAM window (64KB)

; Mode constants
VGA_MODE_TEXT   equ $03                 ; 80x25 text mode
VGA_MODE_12H    equ $12                 ; 640x480, 16 colors
VGA_MODE_13H    equ $13                 ; 320x200, 256 colors
VGA_MODE_X      equ $14                 ; 320x240, 256 colors

; Status bits
VGA_STATUS_VSYNC  equ $01               ; VSync active
VGA_STATUS_RETRACE equ $08              ; Vertical retrace

; Control bits
VGA_CTRL_ENABLE equ $01                 ; VGA enable

; ----- VGA helper macros -----

; Set VGA mode
vga_setmode     macro   mode
                move.b  #\mode,VGA_MODE
                endm

; Enable VGA
vga_enable      macro
                move.b  #VGA_CTRL_ENABLE,VGA_CTRL
                endm

; Set palette write index
vga_setpalette  macro   index
                move.b  #\index,VGA_DAC_WINDEX
                endm

; Write palette RGB values
vga_palette_rgb macro   r,g,b
                move.b  #\r,VGA_DAC_DATA
                move.b  #\g,VGA_DAC_DATA
                move.b  #\b,VGA_DAC_DATA
                endm

; Wait for VSync
vga_wait_vsync  macro
.wait_vga_vs\@:
                move.b  VGA_STATUS,d0
                and.b   #VGA_STATUS_VSYNC,d0
                beq.s   .wait_vga_vs\@
                endm

; Set map mask (planes to write)
vga_mapmask     macro   mask
                move.b  #\mask,VGA_SEQ_MAPMASK
                endm

; Set read map (plane to read)
vga_readmap     macro   plane
                move.b  #\plane,VGA_GC_READMAP
                endm

; ============================================================================
; CUSTOM AUDIO CHIP REGISTERS
; ============================================================================
;
; The custom audio chip provides 4-channel synthesis with modern features:
; - 5 waveform types: square, triangle, sine, noise, sawtooth
; - Per-channel ADSR envelopes with multiple envelope shapes
; - PWM for square waves with automatic modulation
; - Frequency sweep for pitch effects
; - Hard sync and ring modulation between channels
; - Global filter (low-pass, high-pass, band-pass) with resonance
; - Overdrive and reverb effects
; - polyBLEP anti-aliasing for clean high-frequency output
;
; Signal flow: Oscillators -> Envelopes -> Mix -> Filter -> Overdrive -> Reverb
; Sample rate: 44.1kHz, 32-bit floating point internal processing
; ============================================================================

; ----------------------------------------------------------------------------
; Global Audio Control ($F0800-$F083F)
; ----------------------------------------------------------------------------
AUDIO_CTRL        equ $F0800            ; Master audio control
ENV_SHAPE         equ $F0804            ; Global envelope shape

; Filter registers
FILTER_CUTOFF     equ $F0820            ; Cutoff frequency (0-255, exponential 20Hz-20kHz)
FILTER_RESONANCE  equ $F0824            ; Resonance/Q (0-255)
FILTER_TYPE       equ $F0828            ; Filter type (0=off, 1=LP, 2=HP, 3=BP)
FILTER_MOD_SOURCE equ $F082C            ; Modulation source channel (0-3)
FILTER_MOD_AMOUNT equ $F0830            ; Modulation depth (0-255)

; Filter type constants
FILTER_OFF        equ 0                 ; Filter bypassed
FILTER_LOWPASS    equ 1                 ; Low-pass filter
FILTER_HIGHPASS   equ 2                 ; High-pass filter
FILTER_BANDPASS   equ 3                 ; Band-pass filter

; Envelope shape constants
ENV_SHAPE_ADSR      equ 0               ; Standard ADSR
ENV_SHAPE_SAW_UP    equ 1               ; Linear rise to max, then hold
ENV_SHAPE_SAW_DOWN  equ 2               ; Linear fall to zero, then hold
ENV_SHAPE_LOOP      equ 3               ; ADSR but loops after release
ENV_SHAPE_SID       equ 4               ; SID-style exponential ADSR

; ----------------------------------------------------------------------------
; Square Wave Channel ($F0900-$F093F)
; ----------------------------------------------------------------------------
SQUARE_FREQ       equ $F0900            ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
SQUARE_VOL        equ $F0904            ; Volume (0-255)
SQUARE_CTRL       equ $F0908            ; Control (bit 1 = gate)
SQUARE_DUTY       equ $F090C            ; Duty cycle (0-255, 128 = 50%)
SQUARE_SWEEP      equ $F0910            ; Sweep control
SQUARE_PWM_CTRL   equ $F0922            ; PWM control (bit 7 = enable, bits 0-6 = rate)
SQUARE_ATK        equ $F0930            ; Attack time (ms)
SQUARE_DEC        equ $F0934            ; Decay time (ms)
SQUARE_SUS        equ $F0938            ; Sustain level (0-255)
SQUARE_REL        equ $F093C            ; Release time (ms)

; ----------------------------------------------------------------------------
; Triangle Wave Channel ($F0940-$F097F)
; ----------------------------------------------------------------------------
TRI_FREQ          equ $F0940            ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
TRI_VOL           equ $F0944            ; Volume (0-255)
TRI_CTRL          equ $F0948            ; Control (bit 1 = gate)
TRI_SWEEP         equ $F0914            ; Sweep control
TRI_ATK           equ $F0960            ; Attack time (ms)
TRI_DEC           equ $F0964            ; Decay time (ms)
TRI_SUS           equ $F0968            ; Sustain level (0-255)
TRI_REL           equ $F096C            ; Release time (ms)

; ----------------------------------------------------------------------------
; Sine Wave Channel ($F0980-$F09BF)
; ----------------------------------------------------------------------------
SINE_FREQ         equ $F0980            ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
SINE_VOL          equ $F0984            ; Volume (0-255)
SINE_CTRL         equ $F0988            ; Control (bit 1 = gate)
SINE_SWEEP        equ $F0918            ; Sweep control
SINE_ATK          equ $F0990            ; Attack time (ms)
SINE_DEC          equ $F0994            ; Decay time (ms)
SINE_SUS          equ $F0998            ; Sustain level (0-255)
SINE_REL          equ $F099C            ; Release time (ms)

; ----------------------------------------------------------------------------
; Noise Channel ($F09C0-$F09FF)
; ----------------------------------------------------------------------------
NOISE_FREQ        equ $F09C0            ; Frequency/pitch (16.8 fixed-point Hz, value = Hz * 256)
NOISE_VOL         equ $F09C4            ; Volume (0-255)
NOISE_CTRL        equ $F09C8            ; Control (bit 1 = gate)
NOISE_ATK         equ $F09D0            ; Attack time (ms)
NOISE_DEC         equ $F09D4            ; Decay time (ms)
NOISE_SUS         equ $F09D8            ; Sustain level (0-255)
NOISE_REL         equ $F09DC            ; Release time (ms)
NOISE_MODE        equ $F09E0            ; Noise type

; Noise mode constants
NOISE_MODE_WHITE    equ 0               ; White noise (LFSR)
NOISE_MODE_PERIODIC equ 1               ; Periodic/looping noise
NOISE_MODE_METALLIC equ 2               ; Metallic noise
NOISE_MODE_PSG      equ 3               ; PSG-style (AY/YM) LFSR

; ----------------------------------------------------------------------------
; Modulation Registers ($F0A00-$F0A1F)
; ----------------------------------------------------------------------------
; Hard sync: slave channel resets phase when master completes a cycle
SYNC_SOURCE_CH0   equ $F0A00            ; Sync source for channel 0 (0-3, or $FF=off)
SYNC_SOURCE_CH1   equ $F0A04            ; Sync source for channel 1
SYNC_SOURCE_CH2   equ $F0A08            ; Sync source for channel 2
SYNC_SOURCE_CH3   equ $F0A0C            ; Sync source for channel 3

; Ring modulation: output = ch * source (creates harmonic sidebands)
RING_MOD_SOURCE_CH0 equ $F0A10          ; Ring mod source for channel 0
RING_MOD_SOURCE_CH1 equ $F0A14          ; Ring mod source for channel 1
RING_MOD_SOURCE_CH2 equ $F0A18          ; Ring mod source for channel 2
RING_MOD_SOURCE_CH3 equ $F0A1C          ; Ring mod source for channel 3

; ----------------------------------------------------------------------------
; Sawtooth Wave Channel ($F0A20-$F0A6F)
; ----------------------------------------------------------------------------
SAW_FREQ          equ $F0A20            ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
SAW_VOL           equ $F0A24            ; Volume (0-255)
SAW_CTRL          equ $F0A28            ; Control (bit 1 = gate)
SAW_SWEEP         equ $F0A2C            ; Sweep control
SAW_ATK           equ $F0A30            ; Attack time (ms)
SAW_DEC           equ $F0A34            ; Decay time (ms)
SAW_SUS           equ $F0A38            ; Sustain level (0-255)
SAW_REL           equ $F0A3C            ; Release time (ms)
SYNC_SOURCE_CH4   equ $F0A60            ; Sync source for sawtooth
RING_MOD_SOURCE_CH4 equ $F0A64          ; Ring mod source for sawtooth

; ----------------------------------------------------------------------------
; Effects Registers ($F0A40-$F0A5F)
; ----------------------------------------------------------------------------
OVERDRIVE_CTRL    equ $F0A40            ; Drive amount (0-255 -> 0.0-4.0x gain)
REVERB_MIX        equ $F0A50            ; Dry/wet mix (0-255, 0=dry, 255=wet)
REVERB_DECAY      equ $F0A54            ; Decay time (0-255 -> 0.1-0.99)

; ----------------------------------------------------------------------------
; Flexible 4-Channel Synth Block ($F0A80-$F0B7F)
;
; Modern unified interface for all 4 channels. Each channel is $40 bytes.
; Any channel can be any waveform type. This is the preferred interface.
; ----------------------------------------------------------------------------
FLEX_CH_BASE      equ $F0A80            ; Base address of flexible block
FLEX_CH_STRIDE    equ $40               ; Bytes per channel (64)

FLEX_CH0_BASE     equ $F0A80            ; Channel 0 base
FLEX_CH1_BASE     equ $F0AC0            ; Channel 1 base
FLEX_CH2_BASE     equ $F0B00            ; Channel 2 base
FLEX_CH3_BASE     equ $F0B40            ; Channel 3 base

; Register offsets within each flexible channel (add to FLEX_CHx_BASE)
FLEX_OFF_FREQ       equ $00             ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
FLEX_OFF_VOL        equ $04             ; Volume (0-255)
FLEX_OFF_CTRL       equ $08             ; Control (bit 1 = gate)
FLEX_OFF_DUTY       equ $0C             ; Duty cycle for square wave
FLEX_OFF_SWEEP      equ $10             ; Sweep control
FLEX_OFF_ATK        equ $14             ; Attack time (ms)
FLEX_OFF_DEC        equ $18             ; Decay time (ms)
FLEX_OFF_SUS        equ $1C             ; Sustain level (0-255)
FLEX_OFF_REL        equ $20             ; Release time (ms)
FLEX_OFF_WAVE_TYPE  equ $24             ; Waveform type (0-4)
FLEX_OFF_PWM_CTRL   equ $28             ; PWM control
FLEX_OFF_NOISEMODE  equ $2C             ; Noise mode (if wave type is noise)

; Waveform type constants (for FLEX_OFF_WAVE_TYPE)
WAVE_SQUARE       equ 0                 ; Square wave with PWM
WAVE_TRIANGLE     equ 1                 ; Triangle wave
WAVE_SINE         equ 2                 ; Sine wave
WAVE_NOISE        equ 3                 ; Noise
WAVE_SAWTOOTH     equ 4                 ; Sawtooth wave

; ----------------------------------------------------------------------------
; Control Register Bit Masks
; ----------------------------------------------------------------------------
CTRL_GATE         equ $02               ; Gate bit (trigger envelope)
PWM_ENABLE        equ $80               ; PWM enable (bit 7)
PWM_RATE_MASK     equ $7F               ; PWM rate (bits 0-6)
SWEEP_ENABLE      equ $80               ; Sweep enable (bit 7)
SWEEP_DIR_UP      equ $00               ; Sweep direction up
SWEEP_DIR_DOWN    equ $08               ; Sweep direction down (bit 3)
SWEEP_PERIOD_MASK equ $70               ; Sweep period (bits 4-6)
SWEEP_SHIFT_MASK  equ $07               ; Sweep shift (bits 0-2)

; ----------------------------------------------------------------------------
; Custom Audio Chip Macros
; ----------------------------------------------------------------------------

; Set channel frequency (16.8 fixed-point: Hz * 256)
set_ch_freq macro ch,freq
    move.l  #(\freq)*256,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_FREQ
    endm

; Set channel volume
set_ch_vol macro ch,vol
    move.l  #\vol,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_VOL
    endm

; Set channel waveform
set_ch_wave macro ch,wave
    move.l  #\wave,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_WAVE_TYPE
    endm

; Set channel envelope
set_ch_env macro ch,atk,dec,sus,rel
    move.l  #\atk,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_ATK
    move.l  #\dec,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_DEC
    move.l  #\sus,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_SUS
    move.l  #\rel,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_REL
    endm

; Gate channel on (trigger envelope attack)
gate_ch_on macro ch
    move.l  #CTRL_GATE,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_CTRL
    endm

; Gate channel off (trigger envelope release)
gate_ch_off macro ch
    move.l  #0,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_CTRL
    endm

; Set filter parameters
set_filter macro cutoff,resonance,type
    move.l  #\cutoff,FILTER_CUTOFF
    move.l  #\resonance,FILTER_RESONANCE
    move.l  #\type,FILTER_TYPE
    endm

; Set reverb parameters
set_reverb macro mix,decay
    move.l  #\mix,REVERB_MIX
    move.l  #\decay,REVERB_DECAY
    endm

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
; Usage: COP_WAIT(y) = y * COP_WAIT_SCALE
COP_WAIT_MASK        equ $00000000
COP_WAIT_SCALE       equ $1000              ; Multiply Y position by this for WAIT

; Copper MOVE opcodes
COP_MOVE_RASTER_Y     equ $40120000
COP_MOVE_RASTER_H     equ $40130000
COP_MOVE_RASTER_COLOR equ $40140000
COP_MOVE_RASTER_CTRL  equ $40150000

; Copper END opcode
COP_END              equ $C0000000

; Copper SETBASE opcode - set I/O base for subsequent MOVE operations
; Format: [10][6-bit unused][24-bit base >> 2]
; Addresses are 4-byte aligned, so we encode (addr >> 2) in 24 bits
COP_SETBASE_OP       equ $80000000
COP_SETBASE_SHIFT    equ 2

; Pre-computed SETBASE instructions for common devices
COP_SETBASE_VIDEO    equ $8003C000          ; VIDEO_REG_BASE ($F0000 >> 2)
COP_SETBASE_VGA      equ $8003C400          ; VGA_BASE ($F1000 >> 2)
COP_SETBASE_VGA_DAC  equ $8003C416          ; VGA_DAC_WINDEX ($F1058 >> 2)

; MOVE register indices after SETBASE to VGA_DAC_WINDEX
COP_MOVE_VGA_WINDEX  equ $40000000          ; Offset 0: DAC write index
COP_MOVE_VGA_DATA    equ $40010000          ; Offset 1: DAC data (R,G,B seq)

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

; Set SID play pointer and length
set_sid_play macro ptr,len
    move.l  #\ptr,SID_PLAY_PTR
    move.l  #\len,SID_PLAY_LEN
    endm

; Set SID subsong
set_sid_subsong macro num
    move.b  #\num,SID_SUBSONG
    endm

; Start SID playback
start_sid_play macro
    move.l  #1,SID_PLAY_CTRL
    endm

; Start SID playback with looping
start_sid_loop macro
    move.l  #5,SID_PLAY_CTRL
    endm

; Stop SID playback
stop_sid_play macro
    move.l  #2,SID_PLAY_CTRL
    endm

; Enable SID+ enhanced mode
enable_sid_plus macro
    move.b  #1,SID_PLUS_CTRL
    endm

; ============================================================================
; ZX SPECTRUM ULA VIDEO REGISTERS
;
; The ULA chip provides ZX Spectrum compatible video output:
; - 256x192 pixel display with 32-pixel border (320x256 total)
; - Attribute-based coloring: 8x8 pixel cells share INK/PAPER colors
; - 15 unique colors (8 base + 8 bright, black can't brighten)
; - FLASH attribute: swaps INK/PAPER at ~1.6Hz
;
; Non-linear bitmap addressing formula:
;   addr = ((y & $C0) << 5) + ((y & $07) << 8) + ((y & $38) << 2) + (x >> 3)
;
; Attribute byte format: FBPPPIII
;   F (bit 7): FLASH - swap INK/PAPER when set
;   B (bit 6): BRIGHT - intensify both colors
;   PPP (bits 5-3): PAPER (background) color 0-7
;   III (bits 2-0): INK (foreground) color 0-7
;
; Color palette:
;   0=Black, 1=Blue, 2=Red, 3=Magenta, 4=Green, 5=Cyan, 6=Yellow, 7=White
; ============================================================================

ULA_BASE        equ $F2000              ; ULA register base
ULA_BORDER      equ $F2000              ; Border color (bits 0-2)
ULA_CTRL        equ $F2004              ; Control register
ULA_STATUS      equ $F2008              ; Status register

; Control register bits
ULA_CTRL_ENABLE equ $01                 ; ULA enable bit

; VRAM layout (same as authentic ZX Spectrum)
ULA_VRAM        equ $4000               ; VRAM base address
ULA_BITMAP_SIZE equ 6144                ; Bitmap: 256x192/8 = 6144 bytes
ULA_ATTR_OFFSET equ $1800               ; Attribute offset from VRAM base
ULA_ATTR_SIZE   equ 768                 ; Attributes: 32x24 = 768 bytes
ULA_VRAM_SIZE   equ 6912                ; Total: 6144 + 768 bytes

; Display dimensions
ULA_DISPLAY_W   equ 256                 ; Display width in pixels
ULA_DISPLAY_H   equ 192                 ; Display height in pixels
ULA_BORDER_SIZE equ 32                  ; Border size in pixels
ULA_FRAME_W     equ 320                 ; Total frame width (256+64)
ULA_FRAME_H     equ 256                 ; Total frame height (192+64)
ULA_CELLS_X     equ 32                  ; Character cells horizontally
ULA_CELLS_Y     equ 24                  ; Character cells vertically

; ----- ULA helper macros -----

; Set ULA border color (0-7)
ula_border      macro   color
                move.b  #\color,ULA_BORDER
                endm

; Enable ULA
ula_enable      macro
                move.b  #ULA_CTRL_ENABLE,ULA_CTRL
                endm

; Disable ULA
ula_disable     macro
                move.b  #0,ULA_CTRL
                endm

; ============================================================================
; ANTIC VIDEO REGISTERS (Atari 8-bit style)
;
; The ANTIC chip provides Atari 8-bit compatible video output:
; - Display list-driven graphics (14 modes)
; - 320x192 active display (384x240 with borders)
; - 128 colors (16 hues × 8 luminances)
; - Hardware fine scrolling
; - WSYNC for precise raster synchronization
;
; Registers are 4-byte aligned for copper compatibility.
; ============================================================================

ANTIC_BASE        equ $F2100            ; ANTIC register base
ANTIC_DMACTL      equ $F2100            ; DMA control
ANTIC_CHACTL      equ $F2104            ; Character control
ANTIC_DLISTL      equ $F2108            ; Display list pointer low
ANTIC_DLISTH      equ $F210C            ; Display list pointer high
ANTIC_HSCROL      equ $F2110            ; Horizontal scroll (0-15)
ANTIC_VSCROL      equ $F2114            ; Vertical scroll (0-15)
ANTIC_PMBASE      equ $F2118            ; Player-missile base address
ANTIC_CHBASE      equ $F211C            ; Character set base address
ANTIC_WSYNC       equ $F2120            ; Wait for sync (write only)
ANTIC_VCOUNT      equ $F2124            ; Vertical counter (read only)
ANTIC_PENH        equ $F2128            ; Light pen H (read only)
ANTIC_PENV        equ $F212C            ; Light pen V (read only)
ANTIC_NMIEN       equ $F2130            ; NMI enable
ANTIC_NMIST       equ $F2134            ; NMI status (read) / NMIRES (write)
ANTIC_ENABLE      equ $F2138            ; Video enable (IE-specific)
ANTIC_STATUS      equ $F213C            ; Status (VBlank flag)

; DMACTL bits
ANTIC_DMA_NARROW  equ $01               ; Narrow playfield
ANTIC_DMA_NORMAL  equ $02               ; Normal playfield
ANTIC_DMA_WIDE    equ $03               ; Wide playfield
ANTIC_DMA_MISSILE equ $04               ; Enable missile DMA
ANTIC_DMA_PLAYER  equ $08               ; Enable player DMA
ANTIC_DMA_PMRES   equ $10               ; Player/missile resolution
ANTIC_DMA_DL      equ $20               ; Enable display list DMA

; CHACTL bits
ANTIC_CHACTL_BLANK   equ $01            ; Blank instead of inverse
ANTIC_CHACTL_INVERT  equ $02            ; Invert video
ANTIC_CHACTL_REFLECT equ $04            ; Reflect characters

; NMIEN bits
ANTIC_NMIEN_DLI   equ $80               ; Display list interrupt enable
ANTIC_NMIEN_VBI   equ $40               ; Vertical blank interrupt enable

; Display List Instructions
; Blank lines (1-8 scanlines)
DL_BLANK1         equ $00               ; 1 blank scanline
DL_BLANK2         equ $10               ; 2 blank scanlines
DL_BLANK3         equ $20               ; 3 blank scanlines
DL_BLANK4         equ $30               ; 4 blank scanlines
DL_BLANK5         equ $40               ; 5 blank scanlines
DL_BLANK6         equ $50               ; 6 blank scanlines
DL_BLANK7         equ $60               ; 7 blank scanlines
DL_BLANK8         equ $70               ; 8 blank scanlines

; Jump instructions
DL_JMP            equ $01               ; Jump to address (2 bytes follow)
DL_JVB            equ $41               ; Jump and wait for Vertical Blank

; Graphics modes
DL_MODE2          equ $02               ; 40 col text, 8 scanlines/row
DL_MODE3          equ $03               ; 40 col text, 10 scanlines/row
DL_MODE4          equ $04               ; 40 col text, 8 scanlines, multicolor
DL_MODE5          equ $05               ; 40 col text, 16 scanlines, multicolor
DL_MODE6          equ $06               ; 20 col text, 8 scanlines
DL_MODE7          equ $07               ; 20 col text, 16 scanlines
DL_MODE8          equ $08               ; 40 pixels, 8 scanlines/row (GRAPHICS 3)
DL_MODE9          equ $09               ; 80 pixels, 4 scanlines (GRAPHICS 4)
DL_MODE10         equ $0A               ; 80 pixels, 2 scanlines (GRAPHICS 5)
DL_MODE11         equ $0B               ; 160 pixels, 1 scanline (GRAPHICS 6)
DL_MODE12         equ $0C               ; 160 pixels, 1 scanline (GRAPHICS 6+)
DL_MODE13         equ $0D               ; 160 pixels, 2 scanlines (GRAPHICS 7)
DL_MODE14         equ $0E               ; 160 pixels, 1 scanline, 4 colors
DL_MODE15         equ $0F               ; 320 pixels, 1 scanline (GRAPHICS 8)

; Display list modifiers (OR with mode)
DL_LMS            equ $40               ; Load Memory Scan (2 addr bytes follow)
DL_DLI            equ $80               ; Display List Interrupt at end of line
DL_HSCROL         equ $10               ; Horizontal scroll enable
DL_VSCROL         equ $20               ; Vertical scroll enable

; Enable/Status bits
ANTIC_ENABLE_VIDEO   equ $01            ; Bit 0: Video enable
ANTIC_STATUS_VBLANK  equ $01            ; Bit 0: VBlank active

; Display dimensions
ANTIC_DISPLAY_W   equ 320               ; Display width in pixels
ANTIC_DISPLAY_H   equ 192               ; Display height in pixels
ANTIC_FRAME_W     equ 384               ; Total frame width (with border)
ANTIC_FRAME_H     equ 240               ; Total frame height (with border)

; Copper SETBASE for ANTIC video
COP_SETBASE_ANTIC equ $8003C840         ; ANTIC_BASE ($F2100 >> 2)

; ----- ANTIC helper macros -----

; Enable ANTIC video
antic_enable      macro
                  move.b  #ANTIC_ENABLE_VIDEO,ANTIC_ENABLE
                  endm

; Disable ANTIC video
antic_disable     macro
                  move.b  #0,ANTIC_ENABLE
                  endm

; Wait for VBlank
antic_wait_vblank macro
.wvb\@            move.b  ANTIC_STATUS,d0
                  and.b   #ANTIC_STATUS_VBLANK,d0
                  beq.s   .wvb\@
                  endm

; Set display list pointer
antic_set_dlist   macro   addr
                  move.w  #(\addr)&$FFFF,d0
                  move.b  d0,ANTIC_DLISTL
                  lsr.w   #8,d0
                  move.b  d0,ANTIC_DLISTH
                  endm

; ============================================================================
; GTIA VIDEO REGISTERS (Atari 8-bit color control)
;
; GTIA (Graphics Television Interface Adapter) handles color generation
; and player-missile graphics for Atari 8-bit computers. While ANTIC
; controls the display list and timing, GTIA controls colors and sprites.
;
; Color format: HHHHLLLL (hue in high nibble, luminance in low nibble)
; - 16 hues (0-15) x 8 luminances (0,2,4,6,8,10,12,14) = 128 colors
; - Hue 0 = grayscale, others are chromatic
;
; Registers are 4-byte aligned for copper coprocessor compatibility.
; ============================================================================

GTIA_BASE         equ $F2140            ; GTIA register base

; Playfield color registers
GTIA_COLPF0       equ $F2140            ; Playfield color 0
GTIA_COLPF1       equ $F2144            ; Playfield color 1
GTIA_COLPF2       equ $F2148            ; Playfield color 2
GTIA_COLPF3       equ $F214C            ; Playfield color 3
GTIA_COLBK        equ $F2150            ; Background/border color

; Player color registers
GTIA_COLPM0       equ $F2154            ; Player/missile 0 color
GTIA_COLPM1       equ $F2158            ; Player/missile 1 color
GTIA_COLPM2       equ $F215C            ; Player/missile 2 color
GTIA_COLPM3       equ $F2160            ; Player/missile 3 color

; GTIA control registers
GTIA_PRIOR        equ $F2164            ; Priority and GTIA modes
GTIA_GRACTL       equ $F2168            ; Graphics control
GTIA_CONSOL       equ $F216C            ; Console switches (read)

; Player horizontal position registers
GTIA_HPOSP0       equ $F2170            ; Player 0 horizontal position
GTIA_HPOSP1       equ $F2174            ; Player 1 horizontal position
GTIA_HPOSP2       equ $F2178            ; Player 2 horizontal position
GTIA_HPOSP3       equ $F217C            ; Player 3 horizontal position

; Missile horizontal position registers
GTIA_HPOSM0       equ $F2180            ; Missile 0 horizontal position
GTIA_HPOSM1       equ $F2184            ; Missile 1 horizontal position
GTIA_HPOSM2       equ $F2188            ; Missile 2 horizontal position
GTIA_HPOSM3       equ $F218C            ; Missile 3 horizontal position

; Player/missile size registers
GTIA_SIZEP0       equ $F2190            ; Player 0 size (0=normal,1=double,3=quad)
GTIA_SIZEP1       equ $F2194            ; Player 1 size
GTIA_SIZEP2       equ $F2198            ; Player 2 size
GTIA_SIZEP3       equ $F219C            ; Player 3 size
GTIA_SIZEM        equ $F21A0            ; All missile sizes (2 bits each)

; Player graphics registers (directly writable)
GTIA_GRAFP0       equ $F21A4            ; Player 0 graphics (8 pixels)
GTIA_GRAFP1       equ $F21A8            ; Player 1 graphics
GTIA_GRAFP2       equ $F21AC            ; Player 2 graphics
GTIA_GRAFP3       equ $F21B0            ; Player 3 graphics
GTIA_GRAFM        equ $F21B4            ; Missile graphics (2 bits each)

; PRIOR register bits
GTIA_PRIOR_P03    equ $01               ; Players 0-3 have priority over PF
GTIA_PRIOR_P01    equ $02               ; Players 0-1 have priority
GTIA_PRIOR_P23    equ $04               ; Players 2-3 have priority
GTIA_PRIOR_MULTI  equ $10               ; Enable multicolor players
GTIA_PRIOR_FIFTH  equ $20               ; Enable fifth player
GTIA_PRIOR_GTIA1  equ $40               ; GTIA mode bit 0
GTIA_PRIOR_GTIA2  equ $80               ; GTIA mode bit 1

; GRACTL register bits
GTIA_GRACTL_MISSLE equ $01              ; Enable missile graphics
GTIA_GRACTL_PLAYER equ $02              ; Enable player graphics
GTIA_GRACTL_LATCH  equ $04              ; Latch trigger inputs

; GTIA register end
GTIA_END          equ $F216F

; Copper SETBASE for GTIA
COP_SETBASE_GTIA  equ $8003C850         ; GTIA_BASE ($F2140 >> 2)

; ----- GTIA helper macros -----

; Set background color
gtia_setbk        macro   color
                  move.b  #\color,GTIA_COLBK
                  endm

; Set playfield color 0
gtia_setpf0       macro   color
                  move.b  #\color,GTIA_COLPF0
                  endm

; Set playfield color 1
gtia_setpf1       macro   color
                  move.b  #\color,GTIA_COLPF1
                  endm

; Set playfield color 2
gtia_setpf2       macro   color
                  move.b  #\color,GTIA_COLPF2
                  endm

; Set playfield color 3
gtia_setpf3       macro   color
                  move.b  #\color,GTIA_COLPF3
                  endm

; ============================================================================
; 3DFX VOODOO GRAPHICS (SST-1) REGISTERS
; ============================================================================
;
; The Voodoo SST-1 is a 3D accelerator using Vulkan HLE (High-Level Emulation).
; Register writes are translated to GPU draw calls for hardware-accelerated
; 3D rendering.
;
; Programming Model:
;   1. Write vertex coordinates to VOODOO_VERTEX_* (12.4 fixed-point)
;   2. Write colors to VOODOO_START_* (12.12 fixed-point for 0.0-1.0 range)
;   3. Write Z depth to VOODOO_START_Z (20.12 fixed-point)
;   4. Write to VOODOO_TRIANGLE_CMD to submit triangle
;   5. Write to VOODOO_SWAP_BUFFER_CMD to present frame
;
; Fixed-point formats:
;   12.4  = 12 integer bits, 4 fractional bits (coordinates)
;   12.12 = 12 integer bits, 12 fractional bits (colors: 1.0 = $1000)
;   20.12 = 20 integer bits, 12 fractional bits (Z depth)
;
; Example: Draw a red triangle
;   move.l  #(100<<4),VOODOO_VERTEX_AX    ; Vertex A at (100, 50)
;   move.l  #(50<<4),VOODOO_VERTEX_AY
;   move.l  #(200<<4),VOODOO_VERTEX_BX    ; Vertex B at (200, 150)
;   move.l  #(150<<4),VOODOO_VERTEX_BY
;   move.l  #(50<<4),VOODOO_VERTEX_CX     ; Vertex C at (50, 150)
;   move.l  #(150<<4),VOODOO_VERTEX_CY
;   move.l  #$1000,VOODOO_START_R         ; Red = 1.0
;   move.l  #0,VOODOO_START_G             ; Green = 0.0
;   move.l  #0,VOODOO_START_B             ; Blue = 0.0
;   move.l  #$1000,VOODOO_START_A         ; Alpha = 1.0
;   move.l  #0,VOODOO_TRIANGLE_CMD        ; Submit triangle
;   move.l  #0,VOODOO_SWAP_BUFFER_CMD     ; Present frame

; Base address
VOODOO_BASE           equ $F4000

; Status register (read-only)
VOODOO_STATUS         equ $F4000            ; Status register

; Status register bits
VOODOO_STATUS_FBI_BUSY    equ $01           ; FBI (framebuffer) busy
VOODOO_STATUS_TMU_BUSY    equ $02           ; TMU (texture) busy
VOODOO_STATUS_SST_BUSY    equ $04           ; Overall chip busy
VOODOO_STATUS_VRETRACE    equ $40           ; Vertical retrace (VSync)
VOODOO_STATUS_SWAPBUF     equ $80           ; Swap buffers pending

; Vertex coordinate registers (12.4 fixed-point)
VOODOO_VERTEX_AX      equ $F4008            ; Vertex A X coordinate
VOODOO_VERTEX_AY      equ $F400C            ; Vertex A Y coordinate
VOODOO_VERTEX_BX      equ $F4010            ; Vertex B X coordinate
VOODOO_VERTEX_BY      equ $F4014            ; Vertex B Y coordinate
VOODOO_VERTEX_CX      equ $F4018            ; Vertex C X coordinate
VOODOO_VERTEX_CY      equ $F401C            ; Vertex C Y coordinate

; Vertex attribute start values (12.12 fixed-point, 0.0-1.0 range)
VOODOO_START_R        equ $F4020            ; Start red
VOODOO_START_G        equ $F4024            ; Start green
VOODOO_START_B        equ $F4028            ; Start blue
VOODOO_START_Z        equ $F402C            ; Start Z (20.12 fixed-point)
VOODOO_START_A        equ $F4030            ; Start alpha
VOODOO_START_S        equ $F4034            ; Start S texture coord (14.18)
VOODOO_START_T        equ $F4038            ; Start T texture coord (14.18)
VOODOO_START_W        equ $F403C            ; Start W (perspective, 2.30)

; Command registers
VOODOO_TRIANGLE_CMD   equ $F4080            ; Submit triangle for rendering
VOODOO_COLOR_SELECT   equ $F4088            ; Select vertex (0/1/2) for Gouraud shading
VOODOO_FBZCOLOR_PATH  equ $F4104            ; Framebuffer/color path config
VOODOO_FOG_MODE       equ $F4108            ; Fog mode
VOODOO_ALPHA_MODE     equ $F410C            ; Alpha test/blend mode
VOODOO_FBZ_MODE       equ $F4110            ; Framebuffer Z mode
VOODOO_LFB_MODE       equ $F4114            ; Linear framebuffer mode
VOODOO_CLIP_LEFT_RIGHT equ $F4118           ; Clip rectangle left/right
VOODOO_CLIP_LOW_Y_HIGH equ $F411C           ; Clip rectangle top/bottom
VOODOO_NOP_CMD        equ $F4120            ; No operation
VOODOO_FAST_FILL_CMD  equ $F4124            ; Fast fill (clear) command
VOODOO_SWAP_BUFFER_CMD equ $F4128           ; Swap front/back buffers

; Configuration registers
VOODOO_FOG_COLOR      equ $F41C4            ; Fog color (RGB)
VOODOO_ZA_COLOR       equ $F41C8            ; Z/A constant color
VOODOO_CHROMA_KEY     equ $F41CC            ; Chroma key color
VOODOO_COLOR0         equ $F41D8            ; Constant color 0 (fill color)
VOODOO_COLOR1         equ $F41DC            ; Constant color 1

; Framebuffer configuration
VOODOO_VIDEO_DIM      equ $F4214            ; Video dimensions (width<<16|height)

; Texture registers
VOODOO_TEXTURE_MODE   equ $F4300            ; Texture mode configuration
VOODOO_TEX_BASE0      equ $F430C            ; Texture base address (LOD 0)

; fbzMode bits
VOODOO_FBZ_CLIPPING     equ $0001           ; Enable scissor clipping
VOODOO_FBZ_CHROMAKEY    equ $0002           ; Enable chroma keying
VOODOO_FBZ_DEPTH_ENABLE equ $0010           ; Enable depth buffer test
VOODOO_FBZ_DEPTH_FUNC   equ $00E0           ; Depth compare function (3 bits)
VOODOO_FBZ_RGB_WRITE    equ $0200           ; Enable RGB buffer write
VOODOO_FBZ_DEPTH_WRITE  equ $0400           ; Enable depth buffer write

; Depth compare functions (shift left 5 to use in FBZ_MODE)
VOODOO_DEPTH_NEVER        equ 0
VOODOO_DEPTH_LESS         equ 1
VOODOO_DEPTH_EQUAL        equ 2
VOODOO_DEPTH_LESSEQUAL    equ 3
VOODOO_DEPTH_GREATER      equ 4
VOODOO_DEPTH_NOTEQUAL     equ 5
VOODOO_DEPTH_GREATEREQUAL equ 6
VOODOO_DEPTH_ALWAYS       equ 7

; alphaMode bits
VOODOO_ALPHA_TEST_EN    equ $0001           ; Enable alpha test
VOODOO_ALPHA_BLEND_EN   equ $0010           ; Enable alpha blending
VOODOO_ALPHA_SRC_RGB    equ $0F00           ; Source RGB blend factor
VOODOO_ALPHA_DST_RGB    equ $F000           ; Dest RGB blend factor

; Alpha blend factors (for src/dst blend settings)
; Use these values shifted to bits 8-11 for source, bits 12-15 for dest
VOODOO_BLEND_ZERO       equ 0               ; 0
VOODOO_BLEND_SRC_ALPHA  equ 1               ; src.A
VOODOO_BLEND_COLOR      equ 2               ; constant color
VOODOO_BLEND_DST_ALPHA  equ 3               ; dst.A
VOODOO_BLEND_ONE        equ 4               ; 1
VOODOO_BLEND_INV_SRC_A  equ 5               ; 1 - src.A
VOODOO_BLEND_INV_COLOR  equ 6               ; 1 - constant color
VOODOO_BLEND_INV_DST_A  equ 7               ; 1 - dst.A
VOODOO_BLEND_SATURATE   equ 15              ; min(src.A, 1-dst.A)

; swapbufferCMD bits
VOODOO_SWAP_VSYNC       equ $01             ; Wait for VSync before swap
VOODOO_SWAP_CLEAR       equ $02             ; Clear buffer after swap

; Clip register packing (for CLIP_LEFT_RIGHT and CLIP_LOW_Y_HIGH)
; clipLeftRight: bits 0-9 = right, bits 16-25 = left
; clipLowYHigh: bits 0-9 = bottom, bits 16-25 = top
VOODOO_CLIP_MASK        equ $3FF            ; 10-bit clip value mask

; Fixed-point conversion helpers (shift amounts)
VOODOO_FIXED_12_4       equ 4               ; Vertex coordinates
VOODOO_FIXED_12_12      equ 12              ; Colors (1.0 = $1000)

; ----- Voodoo helper macros -----

; Set vertex A coordinates (x, y as integers)
voodoo_vertex_a   macro   x,y
                  move.l  #(\x<<4),VOODOO_VERTEX_AX
                  move.l  #(\y<<4),VOODOO_VERTEX_AY
                  endm

; Set vertex B coordinates
voodoo_vertex_b   macro   x,y
                  move.l  #(\x<<4),VOODOO_VERTEX_BX
                  move.l  #(\y<<4),VOODOO_VERTEX_BY
                  endm

; Set vertex C coordinates
voodoo_vertex_c   macro   x,y
                  move.l  #(\x<<4),VOODOO_VERTEX_CX
                  move.l  #(\y<<4),VOODOO_VERTEX_CY
                  endm

; Set color (r, g, b as 0-255 values)
voodoo_color_rgb  macro   r,g,b
                  move.l  #(\r<<4),VOODOO_START_R
                  move.l  #(\g<<4),VOODOO_START_G
                  move.l  #(\b<<4),VOODOO_START_B
                  move.l  #$1000,VOODOO_START_A
                  endm

; Submit triangle
voodoo_triangle   macro
                  move.l  #0,VOODOO_TRIANGLE_CMD
                  endm

; Swap buffers
voodoo_swap       macro
                  move.l  #0,VOODOO_SWAP_BUFFER_CMD
                  endm

; Clear framebuffer with color (ARGB)
voodoo_clear      macro   color
                  move.l  #\color,VOODOO_COLOR0
                  move.l  #0,VOODOO_FAST_FILL_CMD
                  endm

; ============================================================================
; EOF
; ============================================================================
