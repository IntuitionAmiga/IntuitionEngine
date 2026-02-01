; ie86.inc - Intuition Engine x86 (32-bit) Macro Library
;
; Hardware definitions for writing x86 assembly programs targeting
; the Intuition Engine virtual machine.
;
; For use with NASM or FASM assembler
; Include with: %include "ie86.inc"
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; MEMORY MAP CONSTANTS
; ============================================================================

; System vectors
VECTOR_BASE     equ 0x000000
VECTOR_RESET    equ 0x000000

; Program space
PROGRAM_START   equ 0x001000
STACK_TOP       equ 0xFF0000             ; Top of stack (grows downward)

; Bank window addresses (for compatibility with 8-bit CPUs)
BANK1_WINDOW    equ 0x002000             ; Sprite data bank window (8KB)
BANK2_WINDOW    equ 0x004000             ; Font data bank window (8KB)
BANK3_WINDOW    equ 0x006000             ; General data bank window (8KB)
VRAM_WINDOW     equ 0x008000             ; VRAM bank window (16KB)
BANK_SIZE       equ 0x002000             ; 8KB per extended bank
VRAM_BANK_SIZE  equ 0x004000             ; 16KB VRAM bank

; Bank control registers (memory-mapped)
BANK1_REG_LO    equ 0x00F700             ; Bank 1 select (low byte)
BANK1_REG_HI    equ 0x00F701             ; Bank 1 select (high byte)
BANK2_REG_LO    equ 0x00F702             ; Bank 2 select (low byte)
BANK2_REG_HI    equ 0x00F703             ; Bank 2 select (high byte)
BANK3_REG_LO    equ 0x00F704             ; Bank 3 select (low byte)
BANK3_REG_HI    equ 0x00F705             ; Bank 3 select (high byte)
VRAM_BANK_REG   equ 0x00F7F0             ; VRAM bank select

; ============================================================================
; VIDEO HARDWARE REGISTERS (I/O region at 0xF0000)
; ============================================================================

VIDEO_CTRL      equ 0xF0000              ; Video control (enable/disable)
VIDEO_MODE      equ 0xF0004              ; Video mode (resolution)
VIDEO_STATUS    equ 0xF0008              ; Video status (VBlank flag)
STATUS_VBLANK   equ 2                    ; VBlank status bit

; Copper coprocessor
COPPER_CTRL     equ 0xF000C              ; Copper control
COPPER_PTR      equ 0xF0010              ; Copper list pointer (32-bit)

; Blitter hardware
BLT_CTRL        equ 0xF001C              ; Blitter control
BLT_OP          equ 0xF0020              ; Blitter operation
BLT_SRC         equ 0xF0024              ; Source address (32-bit)
BLT_DST         equ 0xF0028              ; Dest address (32-bit)
BLT_WIDTH       equ 0xF002C              ; Blit width (32-bit)
BLT_HEIGHT      equ 0xF0030              ; Blit height (32-bit)
BLT_SRC_STRIDE  equ 0xF0034              ; Source stride (32-bit)
BLT_DST_STRIDE  equ 0xF0038              ; Dest stride (32-bit)
BLT_COLOR       equ 0xF003C              ; Fill color (32-bit BGRA)
BLT_MASK        equ 0xF0040              ; Mask address (32-bit)
BLT_STATUS      equ 0xF0044              ; Blitter status

; Raster registers
VIDEO_RASTER_Y       equ 0xF0048         ; Raster Y position (32-bit)
VIDEO_RASTER_HEIGHT  equ 0xF004C         ; Raster height (32-bit)
VIDEO_RASTER_COLOR   equ 0xF0050         ; Raster color (32-bit BGRA)
VIDEO_RASTER_CTRL    equ 0xF0054         ; Raster control

; ============================================================================
; PSG/AUDIO REGISTERS (AY-3-8910 style)
; ============================================================================

; Port I/O for PSG access (x86 specific)
PSG_PORT_SELECT equ 0xF0                 ; Port to select PSG register
PSG_PORT_DATA   equ 0xF1                 ; Port to read/write PSG data

PSG_PLUS_CTRL   equ 0xF0C0E              ; PSG+ mode control

; PSG Player Registers (memory-mapped)
PSG_PLAY_PTR    equ 0xF0C10              ; Play pointer (32-bit)
PSG_PLAY_LEN    equ 0xF0C14              ; Play length (32-bit)
PSG_PLAY_CTRL   equ 0xF0C18              ; Play control
PSG_PLAY_STATUS equ 0xF0C1C              ; Play status

; ============================================================================
; POKEY AUDIO REGISTERS (Atari style)
; ============================================================================

; Port I/O for POKEY access (x86 specific)
; Ports 0xD0-0xDF map directly to POKEY registers
POKEY_PORT_BASE     equ 0xD0             ; Base port for POKEY

; Memory-mapped POKEY registers
POKEY_BASE      equ 0xF0D00              ; POKEY base address
POKEY_AUDF1     equ 0xF0D00              ; Channel 1 frequency divider
POKEY_AUDC1     equ 0xF0D01              ; Channel 1 control (distortion + volume)
POKEY_AUDF2     equ 0xF0D02              ; Channel 2 frequency divider
POKEY_AUDC2     equ 0xF0D03              ; Channel 2 control
POKEY_AUDF3     equ 0xF0D04              ; Channel 3 frequency divider
POKEY_AUDC3     equ 0xF0D05              ; Channel 3 control
POKEY_AUDF4     equ 0xF0D06              ; Channel 4 frequency divider
POKEY_AUDC4     equ 0xF0D07              ; Channel 4 control
POKEY_AUDCTL    equ 0xF0D08              ; Master audio control
POKEY_PLUS_CTRL equ 0xF0D09              ; POKEY+ mode (0=standard, 1=enhanced)

; AUDCTL bit masks
AUDCTL_CLOCK_15KHZ  equ 0x01             ; Use 15kHz base clock (else 64kHz)
AUDCTL_HIPASS_CH1   equ 0x02             ; High-pass filter ch1 by ch3
AUDCTL_HIPASS_CH2   equ 0x04             ; High-pass filter ch2 by ch4
AUDCTL_CH4_BY_CH3   equ 0x08             ; Ch4 clocked by ch3 (16-bit mode)
AUDCTL_CH2_BY_CH1   equ 0x10             ; Ch2 clocked by ch1 (16-bit mode)
AUDCTL_CH3_179MHZ   equ 0x20             ; Ch3 uses 1.79MHz clock
AUDCTL_CH1_179MHZ   equ 0x40             ; Ch1 uses 1.79MHz clock
AUDCTL_POLY9        equ 0x80             ; Use 9-bit poly instead of 17-bit

; AUDC distortion modes (bits 5-7)
POKEY_DIST_POLY17_POLY5 equ 0x00         ; 17-bit + 5-bit poly
POKEY_DIST_POLY5        equ 0x20         ; 5-bit poly only
POKEY_DIST_POLY17_POLY4 equ 0x40         ; 17-bit + 4-bit poly
POKEY_DIST_POLY5_POLY4  equ 0x60         ; 5-bit + 4-bit poly
POKEY_DIST_POLY17       equ 0x80         ; 17-bit poly only
POKEY_DIST_PURE_TONE    equ 0xA0         ; Pure square wave
POKEY_DIST_POLY4        equ 0xC0         ; 4-bit poly only
POKEY_DIST_POLY17_PULSE equ 0xE0         ; 17-bit + pulse

; SAP Player Registers
SAP_PLAY_PTR    equ 0xF0D10              ; Play pointer (32-bit)
SAP_PLAY_LEN    equ 0xF0D14              ; Play length (32-bit)
SAP_PLAY_CTRL   equ 0xF0D18              ; Play control
SAP_PLAY_STATUS equ 0xF0D1C              ; Play status
SAP_SUBSONG     equ 0xF0D1D              ; Subsong selection (0-255)

; ============================================================================
; SID AUDIO REGISTERS (MOS 6581/8580 style)
; ============================================================================

; Port I/O for SID access (x86 specific)
SID_PORT_SELECT equ 0xE0                 ; Port to select SID register
SID_PORT_DATA   equ 0xE1                 ; Port to read/write SID data

; Memory-mapped SID registers
SID_BASE        equ 0xF0E00              ; SID base address

; Voice 1 registers
SID_V1_FREQ_LO  equ 0xF0E00              ; Voice 1 frequency low byte
SID_V1_FREQ_HI  equ 0xF0E01              ; Voice 1 frequency high byte
SID_V1_PW_LO    equ 0xF0E02              ; Voice 1 pulse width low byte
SID_V1_PW_HI    equ 0xF0E03              ; Voice 1 pulse width high (bits 0-3)
SID_V1_CTRL     equ 0xF0E04              ; Voice 1 control register
SID_V1_AD       equ 0xF0E05              ; Voice 1 attack/decay
SID_V1_SR       equ 0xF0E06              ; Voice 1 sustain/release

; Voice 2 registers
SID_V2_FREQ_LO  equ 0xF0E07              ; Voice 2 frequency low byte
SID_V2_FREQ_HI  equ 0xF0E08              ; Voice 2 frequency high byte
SID_V2_PW_LO    equ 0xF0E09              ; Voice 2 pulse width low byte
SID_V2_PW_HI    equ 0xF0E0A              ; Voice 2 pulse width high
SID_V2_CTRL     equ 0xF0E0B              ; Voice 2 control register
SID_V2_AD       equ 0xF0E0C              ; Voice 2 attack/decay
SID_V2_SR       equ 0xF0E0D              ; Voice 2 sustain/release

; Voice 3 registers
SID_V3_FREQ_LO  equ 0xF0E0E              ; Voice 3 frequency low byte
SID_V3_FREQ_HI  equ 0xF0E0F              ; Voice 3 frequency high byte
SID_V3_PW_LO    equ 0xF0E10              ; Voice 3 pulse width low byte
SID_V3_PW_HI    equ 0xF0E11              ; Voice 3 pulse width high
SID_V3_CTRL     equ 0xF0E12              ; Voice 3 control register
SID_V3_AD       equ 0xF0E13              ; Voice 3 attack/decay
SID_V3_SR       equ 0xF0E14              ; Voice 3 sustain/release

; Filter and volume registers
SID_FC_LO       equ 0xF0E15              ; Filter cutoff low (bits 0-2)
SID_FC_HI       equ 0xF0E16              ; Filter cutoff high byte
SID_RES_FILT    equ 0xF0E17              ; Resonance (bits 4-7) + routing (bits 0-3)
SID_MODE_VOL    equ 0xF0E18              ; Volume (bits 0-3) + filter mode (bits 4-7)
SID_PLUS_CTRL   equ 0xF0E19              ; SID+ mode (0=standard, 1=enhanced)

; Voice control register bits (SID_Vx_CTRL)
SID_CTRL_GATE     equ 0x01               ; Gate bit (trigger envelope)
SID_CTRL_SYNC     equ 0x02               ; Sync with previous voice
SID_CTRL_RINGMOD  equ 0x04               ; Ring modulation
SID_CTRL_TEST     equ 0x08               ; Test bit (resets oscillator)
SID_CTRL_TRIANGLE equ 0x10               ; Triangle waveform
SID_CTRL_SAWTOOTH equ 0x20               ; Sawtooth waveform
SID_CTRL_PULSE    equ 0x40               ; Pulse/square waveform
SID_CTRL_NOISE    equ 0x80               ; Noise waveform

; Filter routing bits (SID_RES_FILT bits 0-3)
SID_FILT_V1       equ 0x01               ; Route voice 1 through filter
SID_FILT_V2       equ 0x02               ; Route voice 2 through filter
SID_FILT_V3       equ 0x04               ; Route voice 3 through filter
SID_FILT_EXT      equ 0x08               ; Route external input through filter

; Filter mode bits (SID_MODE_VOL bits 4-7)
SID_MODE_LP       equ 0x10               ; Low-pass filter
SID_MODE_BP       equ 0x20               ; Band-pass filter
SID_MODE_HP       equ 0x40               ; High-pass filter
SID_MODE_3OFF     equ 0x80               ; Disconnect voice 3 from output

; SID Player Registers
SID_PLAY_PTR    equ 0xF0E20              ; Play pointer (32-bit)
SID_PLAY_LEN    equ 0xF0E24              ; Play length (32-bit)
SID_PLAY_CTRL   equ 0xF0E28              ; Play control
SID_PLAY_STATUS equ 0xF0E2C              ; Play status
SID_SUBSONG     equ 0xF0E2D              ; Subsong selection (0-255)

; ============================================================================
; TED AUDIO REGISTERS (Commodore Plus/4 style)
; ============================================================================

; Port I/O for TED access (x86 specific)
TED_PORT_SELECT equ 0xF2                 ; Port to select TED register
TED_PORT_DATA   equ 0xF3                 ; Port to read/write TED data

; TED register indices (for port I/O)
TED_REG_FREQ1_LO    equ 0x00             ; Voice 1 frequency low byte
TED_REG_FREQ2_LO    equ 0x01             ; Voice 2 frequency low byte
TED_REG_FREQ2_HI    equ 0x02             ; Voice 2 frequency high (bits 0-1)
TED_REG_SND_CTRL    equ 0x03             ; Sound control register
TED_REG_FREQ1_HI    equ 0x04             ; Voice 1 frequency high (bits 0-1)
TED_REG_PLUS_CTRL   equ 0x05             ; TED+ mode (0=standard, 1=enhanced)

; Memory-mapped TED audio registers
TED_BASE        equ 0xF0F00              ; TED base address
TED_FREQ1_LO    equ 0xF0F00              ; Voice 1 frequency low byte
TED_FREQ2_LO    equ 0xF0F01              ; Voice 2 frequency low byte
TED_FREQ2_HI    equ 0xF0F02              ; Voice 2 frequency high (bits 0-1)
TED_SND_CTRL    equ 0xF0F03              ; Sound control register
TED_FREQ1_HI    equ 0xF0F04              ; Voice 1 frequency high (bits 0-1)
TED_PLUS_CTRL   equ 0xF0F05              ; TED+ mode (0=standard, 1=enhanced)

; TED Control register bits (TED_SND_CTRL)
TED_CTRL_SNDDC     equ 0x80              ; D/A mode (bit 7)
TED_CTRL_SND2NOISE equ 0x40              ; Voice 2 noise enable (bit 6)
TED_CTRL_SND2ON    equ 0x20              ; Voice 2 enable (bit 5)
TED_CTRL_SND1ON    equ 0x10              ; Voice 1 enable (bit 4)
TED_CTRL_VOLUME    equ 0x0F              ; Volume mask (bits 0-3, max=8)

; TED Player Registers
TED_PLAY_PTR    equ 0xF0F10              ; Play pointer (32-bit)
TED_PLAY_LEN    equ 0xF0F14              ; Play length (32-bit)
TED_PLAY_CTRL   equ 0xF0F18              ; Play control
TED_PLAY_STATUS equ 0xF0F1C              ; Play status

; ============================================================================
; TED VIDEO REGISTERS
; ============================================================================

; TED video register indices (for port I/O, indices 0x20-0x2F)
TED_V_CTRL1       equ 0x20               ; Control 1 (ECM/BMM/DEN/RSEL/YSCROLL)
TED_V_CTRL2       equ 0x21               ; Control 2 (RES/MCM/CSEL/XSCROLL)
TED_V_CHAR_BASE   equ 0x22               ; Character/bitmap base address
TED_V_VIDEO_BASE  equ 0x23               ; Video matrix base address
TED_V_BG0         equ 0x24               ; Background color 0
TED_V_BG1         equ 0x25               ; Background color 1 (multicolor)
TED_V_BG2         equ 0x26               ; Background color 2 (multicolor)
TED_V_BG3         equ 0x27               ; Background color 3 (multicolor)
TED_V_BORDER      equ 0x28               ; Border color
TED_V_CURSOR_HI   equ 0x29               ; Cursor position high byte
TED_V_CURSOR_LO   equ 0x2A               ; Cursor position low byte
TED_V_CURSOR_CLR  equ 0x2B               ; Cursor color
TED_V_RASTER_LO   equ 0x2C               ; Raster line low (read-only)
TED_V_RASTER_HI   equ 0x2D               ; Raster line high (read-only)
TED_V_ENABLE      equ 0x2E               ; Video enable (bit 0)
TED_V_STATUS      equ 0x2F               ; Status (bit 0 = VBlank)

; Memory-mapped TED video registers (4-byte aligned)
TED_VIDEO_BASE    equ 0xF0F20            ; TED video register base
TED_V_CTRL1_MM    equ 0xF0F20            ; Control 1
TED_V_CTRL2_MM    equ 0xF0F24            ; Control 2
TED_V_CHAR_BASE_MM equ 0xF0F28           ; Character/bitmap base
TED_V_VIDEO_BASE_MM equ 0xF0F2C          ; Video matrix base
TED_V_BG_COLOR0   equ 0xF0F30            ; Background color 0
TED_V_BG_COLOR1   equ 0xF0F34            ; Background color 1
TED_V_BG_COLOR2   equ 0xF0F38            ; Background color 2
TED_V_BG_COLOR3   equ 0xF0F3C            ; Background color 3
TED_V_BORDER_MM   equ 0xF0F40            ; Border color
TED_V_ENABLE_MM   equ 0xF0F58            ; Video enable
TED_V_STATUS_MM   equ 0xF0F5C            ; Status

; TED_V_CTRL1 bits
TED_V_CTRL1_ECM      equ 0x40            ; Bit 6: Extended Color Mode
TED_V_CTRL1_BMM      equ 0x20            ; Bit 5: Bitmap Mode
TED_V_CTRL1_DEN      equ 0x10            ; Bit 4: Display Enable
TED_V_CTRL1_RSEL     equ 0x08            ; Bit 3: Row Select (0=24, 1=25)
TED_V_CTRL1_YSCROLL  equ 0x07            ; Bits 0-2: Vertical scroll mask

; ============================================================================
; AHX AUDIO REGISTERS (Amiga AHX module player)
; ============================================================================

AHX_BASE        equ 0xF0B80              ; AHX base address
AHX_PLUS_CTRL   equ 0xF0B80              ; AHX+ mode (0=standard, 1=enhanced)
AHX_PLAY_PTR    equ 0xF0B84              ; Play pointer (32-bit)
AHX_PLAY_LEN    equ 0xF0B88              ; Play length (32-bit)
AHX_PLAY_CTRL   equ 0xF0B8C              ; Play control
AHX_PLAY_STATUS equ 0xF0B90              ; Play status
AHX_SUBSONG     equ 0xF0B91              ; Subsong selection (0-255)

; ============================================================================
; VGA VIDEO REGISTERS (IBM VGA compatible)
; ============================================================================

; Standard VGA I/O ports (x86 compatible)
VGA_PORT_SEQ_INDEX   equ 0x3C4           ; Sequencer index
VGA_PORT_SEQ_DATA    equ 0x3C5           ; Sequencer data
VGA_PORT_DAC_MASK    equ 0x3C6           ; DAC pixel mask
VGA_PORT_DAC_RINDEX  equ 0x3C7           ; DAC read index
VGA_PORT_DAC_WINDEX  equ 0x3C8           ; DAC write index
VGA_PORT_DAC_DATA    equ 0x3C9           ; DAC data (R,G,B sequence)
VGA_PORT_GC_INDEX    equ 0x3CE           ; Graphics controller index
VGA_PORT_GC_DATA     equ 0x3CF           ; Graphics controller data
VGA_PORT_CRTC_INDEX  equ 0x3D4           ; CRTC index
VGA_PORT_CRTC_DATA   equ 0x3D5           ; CRTC data
VGA_PORT_STATUS      equ 0x3DA           ; Input status 1

; Memory-mapped VGA registers
VGA_BASE        equ 0xF1000
VGA_MODE        equ 0xF1000              ; Mode control
VGA_STATUS      equ 0xF1004              ; Status register
VGA_CTRL        equ 0xF1008              ; Control register

; Sequencer registers
VGA_SEQ_INDEX   equ 0xF1010              ; Sequencer index
VGA_SEQ_DATA    equ 0xF1014              ; Sequencer data
VGA_SEQ_MAPMASK equ 0xF1018              ; Plane write mask (direct)

; CRTC registers
VGA_CRTC_INDEX  equ 0xF1020              ; CRTC index
VGA_CRTC_DATA   equ 0xF1024              ; CRTC data
VGA_CRTC_STARTHI equ 0xF1028             ; Start address high
VGA_CRTC_STARTLO equ 0xF102C             ; Start address low

; Graphics Controller registers
VGA_GC_INDEX    equ 0xF1030              ; GC index
VGA_GC_DATA     equ 0xF1034              ; GC data
VGA_GC_READMAP  equ 0xF1038              ; Read plane select (direct)
VGA_GC_BITMASK  equ 0xF103C              ; Bit mask (direct)

; Attribute Controller
VGA_ATTR_INDEX  equ 0xF1040              ; Attribute index
VGA_ATTR_DATA   equ 0xF1044              ; Attribute data

; DAC/Palette
VGA_DAC_MASK    equ 0xF1050              ; Pixel mask
VGA_DAC_RINDEX  equ 0xF1054              ; Read index
VGA_DAC_WINDEX  equ 0xF1058              ; Write index
VGA_DAC_DATA    equ 0xF105C              ; DAC data (R,G,B sequence)

; Palette RAM (256 x 3 bytes)
VGA_PALETTE     equ 0xF1100              ; Palette base (768 bytes)

; VGA VRAM (64KB window - standard PC location)
VGA_VRAM        equ 0xA0000              ; VRAM window (64KB)

; Mode constants
VGA_MODE_TEXT   equ 0x03                 ; 80x25 text mode
VGA_MODE_12H    equ 0x12                 ; 640x480, 16 colors
VGA_MODE_13H    equ 0x13                 ; 320x200, 256 colors
VGA_MODE_X      equ 0x14                 ; 320x240, 256 colors

; Status bits
VGA_STATUS_VSYNC  equ 0x01               ; VSync active
VGA_STATUS_RETRACE equ 0x08              ; Vertical retrace

; Control bits
VGA_CTRL_ENABLE equ 0x01                 ; VGA enable

; ============================================================================
; CUSTOM AUDIO CHIP REGISTERS
; ============================================================================

; Global Audio Control
AUDIO_CTRL        equ 0xF0800            ; Master audio control
ENV_SHAPE         equ 0xF0804            ; Global envelope shape

; Filter registers
FILTER_CUTOFF     equ 0xF0820            ; Cutoff frequency (0-255)
FILTER_RESONANCE  equ 0xF0824            ; Resonance/Q (0-255)
FILTER_TYPE       equ 0xF0828            ; Filter type (0=off, 1=LP, 2=HP, 3=BP)
FILTER_MOD_SOURCE equ 0xF082C            ; Modulation source channel (0-3)
FILTER_MOD_AMOUNT equ 0xF0830            ; Modulation depth (0-255)

; Filter type constants
FILTER_OFF        equ 0                  ; Filter bypassed
FILTER_LOWPASS    equ 1                  ; Low-pass filter
FILTER_HIGHPASS   equ 2                  ; High-pass filter
FILTER_BANDPASS   equ 3                  ; Band-pass filter

; Envelope shape constants
ENV_SHAPE_ADSR      equ 0                ; Standard ADSR
ENV_SHAPE_SAW_UP    equ 1                ; Linear rise to max, then hold
ENV_SHAPE_SAW_DOWN  equ 2                ; Linear fall to zero, then hold
ENV_SHAPE_LOOP      equ 3                ; ADSR but loops after release
ENV_SHAPE_SID       equ 4                ; SID-style exponential ADSR

; Square Wave Channel
SQUARE_FREQ       equ 0xF0900            ; Frequency (16.8 fixed-point Hz)
SQUARE_VOL        equ 0xF0904            ; Volume (0-255)
SQUARE_CTRL       equ 0xF0908            ; Control (bit 1 = gate)
SQUARE_DUTY       equ 0xF090C            ; Duty cycle (0-255, 128 = 50%)
SQUARE_ATK        equ 0xF0930            ; Attack time (ms)
SQUARE_DEC        equ 0xF0934            ; Decay time (ms)
SQUARE_SUS        equ 0xF0938            ; Sustain level (0-255)
SQUARE_REL        equ 0xF093C            ; Release time (ms)

; Triangle Wave Channel
TRI_FREQ          equ 0xF0940            ; Frequency (16.8 fixed-point Hz)
TRI_VOL           equ 0xF0944            ; Volume (0-255)
TRI_CTRL          equ 0xF0948            ; Control (bit 1 = gate)
TRI_ATK           equ 0xF0960            ; Attack time (ms)
TRI_DEC           equ 0xF0964            ; Decay time (ms)
TRI_SUS           equ 0xF0968            ; Sustain level (0-255)
TRI_REL           equ 0xF096C            ; Release time (ms)

; Sine Wave Channel
SINE_FREQ         equ 0xF0980            ; Frequency (16.8 fixed-point Hz)
SINE_VOL          equ 0xF0984            ; Volume (0-255)
SINE_CTRL         equ 0xF0988            ; Control (bit 1 = gate)
SINE_ATK          equ 0xF0990            ; Attack time (ms)
SINE_DEC          equ 0xF0994            ; Decay time (ms)
SINE_SUS          equ 0xF0998            ; Sustain level (0-255)
SINE_REL          equ 0xF099C            ; Release time (ms)

; Noise Channel
NOISE_FREQ        equ 0xF09C0            ; Frequency/pitch (16.8 fixed-point Hz)
NOISE_VOL         equ 0xF09C4            ; Volume (0-255)
NOISE_CTRL        equ 0xF09C8            ; Control (bit 1 = gate)
NOISE_ATK         equ 0xF09D0            ; Attack time (ms)
NOISE_DEC         equ 0xF09D4            ; Decay time (ms)
NOISE_SUS         equ 0xF09D8            ; Sustain level (0-255)
NOISE_REL         equ 0xF09DC            ; Release time (ms)
NOISE_MODE        equ 0xF09E0            ; Noise type

; Noise mode constants
NOISE_MODE_WHITE    equ 0                ; White noise (LFSR)
NOISE_MODE_PERIODIC equ 1                ; Periodic/looping noise
NOISE_MODE_METALLIC equ 2                ; Metallic noise
NOISE_MODE_PSG      equ 3                ; PSG-style (AY/YM) LFSR

; Sawtooth Wave Channel
SAW_FREQ          equ 0xF0A20            ; Frequency (16.8 fixed-point Hz)
SAW_VOL           equ 0xF0A24            ; Volume (0-255)
SAW_CTRL          equ 0xF0A28            ; Control (bit 1 = gate)
SAW_ATK           equ 0xF0A30            ; Attack time (ms)
SAW_DEC           equ 0xF0A34            ; Decay time (ms)
SAW_SUS           equ 0xF0A38            ; Sustain level (0-255)
SAW_REL           equ 0xF0A3C            ; Release time (ms)

; Effects Registers
OVERDRIVE_CTRL    equ 0xF0A40            ; Drive amount (0-255)
REVERB_MIX        equ 0xF0A50            ; Dry/wet mix (0-255)
REVERB_DECAY      equ 0xF0A54            ; Decay time (0-255)

; Flexible 4-Channel Synth Block
FLEX_CH_BASE      equ 0xF0A80            ; Base address of flexible block
FLEX_CH_STRIDE    equ 0x40               ; Bytes per channel (64)

FLEX_CH0_BASE     equ 0xF0A80            ; Channel 0 base
FLEX_CH1_BASE     equ 0xF0AC0            ; Channel 1 base
FLEX_CH2_BASE     equ 0xF0B00            ; Channel 2 base
FLEX_CH3_BASE     equ 0xF0B40            ; Channel 3 base

; Register offsets within each flexible channel
FLEX_OFF_FREQ       equ 0x00             ; Frequency (16.8 fixed-point Hz)
FLEX_OFF_VOL        equ 0x04             ; Volume (0-255)
FLEX_OFF_CTRL       equ 0x08             ; Control (bit 1 = gate)
FLEX_OFF_DUTY       equ 0x0C             ; Duty cycle for square wave
FLEX_OFF_SWEEP      equ 0x10             ; Sweep control
FLEX_OFF_ATK        equ 0x14             ; Attack time (ms)
FLEX_OFF_DEC        equ 0x18             ; Decay time (ms)
FLEX_OFF_SUS        equ 0x1C             ; Sustain level (0-255)
FLEX_OFF_REL        equ 0x20             ; Release time (ms)
FLEX_OFF_WAVE_TYPE  equ 0x24             ; Waveform type (0-4)
FLEX_OFF_PWM_CTRL   equ 0x28             ; PWM control
FLEX_OFF_NOISEMODE  equ 0x2C             ; Noise mode (if wave type is noise)

; Waveform type constants
WAVE_SQUARE       equ 0                  ; Square wave with PWM
WAVE_TRIANGLE     equ 1                  ; Triangle wave
WAVE_SINE         equ 2                  ; Sine wave
WAVE_NOISE        equ 3                  ; Noise
WAVE_SAWTOOTH     equ 4                  ; Sawtooth wave

; Control Register Bit Masks
CTRL_GATE         equ 0x02               ; Gate bit (trigger envelope)
PWM_ENABLE        equ 0x80               ; PWM enable (bit 7)

; ============================================================================
; VIDEO CONSTANTS
; ============================================================================

VRAM_START      equ 0x100000             ; VRAM starts at 1MB
SCREEN_W        equ 640                  ; Default screen width
SCREEN_H        equ 480                  ; Default screen height
LINE_BYTES      equ 2560                 ; Bytes per scanline (640 * 4)

; Blitter operations
BLT_OP_COPY     equ 0                    ; Copy source to dest
BLT_OP_FILL     equ 1                    ; Fill rect with color
BLT_OP_LINE     equ 2                    ; Draw line
BLT_OP_MASKED   equ 3                    ; Copy with mask
BLT_OP_ALPHA    equ 4                    ; Alpha blending

; Background color (black with alpha)
BACKGROUND      equ 0xFF000000           ; BGRA: Black, full alpha

; ============================================================================
; COPPER OPCODES
; ============================================================================

COP_WAIT_MASK        equ 0x00000000
COP_WAIT_SCALE       equ 0x1000          ; Multiply Y position by this for WAIT

; Copper MOVE opcodes
COP_MOVE_RASTER_Y     equ 0x40120000
COP_MOVE_RASTER_H     equ 0x40130000
COP_MOVE_RASTER_COLOR equ 0x40140000
COP_MOVE_RASTER_CTRL  equ 0x40150000

; Copper END opcode
COP_END              equ 0xC0000000

; Copper SETBASE opcode
COP_SETBASE_OP       equ 0x80000000
COP_SETBASE_SHIFT    equ 2

; Pre-computed SETBASE instructions
COP_SETBASE_VIDEO    equ 0x8003C000      ; VIDEO_REG_BASE (0xF0000 >> 2)
COP_SETBASE_VGA      equ 0x8003C400      ; VGA_BASE (0xF1000 >> 2)
COP_SETBASE_VGA_DAC  equ 0x8003C416      ; VGA_DAC_WINDEX (0xF1058 >> 2)

; ============================================================================
; ZX SPECTRUM ULA VIDEO REGISTERS
; ============================================================================

ULA_BASE        equ 0xF2000              ; ULA register base
ULA_BORDER      equ 0xF2000              ; Border color (bits 0-2)
ULA_CTRL        equ 0xF2004              ; Control register
ULA_STATUS      equ 0xF2008              ; Status register

; ULA port I/O (same as Z80 for compatibility)
ULA_PORT        equ 0xFE                 ; Border color via port I/O

; Control register bits
ULA_CTRL_ENABLE equ 0x01                 ; ULA enable bit

; VRAM layout
ULA_VRAM        equ 0x4000               ; VRAM base address
ULA_BITMAP_SIZE equ 6144                 ; Bitmap: 256x192/8 = 6144 bytes
ULA_ATTR_OFFSET equ 0x1800               ; Attribute offset from VRAM base
ULA_ATTR_SIZE   equ 768                  ; Attributes: 32x24 = 768 bytes
ULA_VRAM_SIZE   equ 6912                 ; Total: 6144 + 768 bytes

; Display dimensions
ULA_DISPLAY_W   equ 256                  ; Display width in pixels
ULA_DISPLAY_H   equ 192                  ; Display height in pixels
ULA_BORDER_SIZE equ 32                   ; Border size in pixels
ULA_FRAME_W     equ 320                  ; Total frame width (256+64)
ULA_FRAME_H     equ 256                  ; Total frame height (192+64)
ULA_CELLS_X     equ 32                   ; Character cells horizontally
ULA_CELLS_Y     equ 24                   ; Character cells vertically

; ============================================================================
; TIMER REGISTERS
; ============================================================================

TIMER_CTRL      equ 0x00F800             ; Timer control
TIMER_COUNT     equ 0x00F804             ; Timer count
TIMER_RELOAD    equ 0x00F808             ; Timer reload value

; ============================================================================
; MACROS (NASM syntax)
; ============================================================================

; Wait for VBlank
%macro wait_vblank 0
%%wait_not_vblank:
    mov eax, [VIDEO_STATUS]
    test eax, STATUS_VBLANK
    jnz %%wait_not_vblank
%%wait_vblank:
    mov eax, [VIDEO_STATUS]
    test eax, STATUS_VBLANK
    jz %%wait_vblank
%endmacro

; Wait for VGA VSync (using port I/O)
%macro vga_wait_vsync 0
%%wait_retrace:
    mov dx, VGA_PORT_STATUS
    in al, dx
    test al, VGA_STATUS_RETRACE
    jz %%wait_retrace
%endmacro

; Wait for blitter to finish
%macro wait_blit 0
%%wait_blit:
    mov eax, [BLT_CTRL]
    test eax, 2
    jnz %%wait_blit
%endmacro

; Start a blitter operation
%macro start_blit 0
    mov dword [BLT_CTRL], 1
%endmacro

; Set blitter fill color (32-bit BGRA)
%macro set_blt_color 1
    mov dword [BLT_COLOR], %1
%endmacro

; Set blitter dimensions
%macro set_blt_size 2
    mov dword [BLT_WIDTH], %1
    mov dword [BLT_HEIGHT], %2
%endmacro

; Set VGA mode
%macro vga_setmode 1
    mov byte [VGA_MODE], %1
%endmacro

; Enable VGA
%macro vga_enable 0
    mov byte [VGA_CTRL], VGA_CTRL_ENABLE
%endmacro

; Set VGA palette entry (using ports)
; Arguments: index, r, g, b
%macro vga_setpalette 4
    mov dx, VGA_PORT_DAC_WINDEX
    mov al, %1
    out dx, al
    mov dx, VGA_PORT_DAC_DATA
    mov al, %2
    out dx, al
    mov al, %3
    out dx, al
    mov al, %4
    out dx, al
%endmacro

; Set copper list pointer
%macro set_copper_ptr 1
    mov dword [COPPER_PTR], %1
%endmacro

; Enable copper
%macro enable_copper 0
    mov dword [COPPER_CTRL], 1
%endmacro

; Disable copper
%macro disable_copper 0
    mov dword [COPPER_CTRL], 0
%endmacro

; Write PSG register via port I/O
; Arguments: register, value
%macro psg_write 2
    mov al, %1
    out PSG_PORT_SELECT, al
    mov al, %2
    out PSG_PORT_DATA, al
%endmacro

; Write SID register via port I/O
; Arguments: register, value
%macro sid_write 2
    mov al, %1
    out SID_PORT_SELECT, al
    mov al, %2
    out SID_PORT_DATA, al
%endmacro

; Write POKEY register via port I/O
; Arguments: register offset (0-15), value
%macro pokey_write 2
    mov dx, POKEY_PORT_BASE + %1
    mov al, %2
    out dx, al
%endmacro

; Write TED register via port I/O
; Arguments: register, value
%macro ted_write 2
    mov al, %1
    out TED_PORT_SELECT, al
    mov al, %2
    out TED_PORT_DATA, al
%endmacro

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

ANTIC_BASE        equ 0xF2100           ; ANTIC register base
ANTIC_DMACTL      equ 0xF2100           ; DMA control
ANTIC_CHACTL      equ 0xF2104           ; Character control
ANTIC_DLISTL      equ 0xF2108           ; Display list pointer low
ANTIC_DLISTH      equ 0xF210C           ; Display list pointer high
ANTIC_HSCROL      equ 0xF2110           ; Horizontal scroll (0-15)
ANTIC_VSCROL      equ 0xF2114           ; Vertical scroll (0-15)
ANTIC_PMBASE      equ 0xF2118           ; Player-missile base address
ANTIC_CHBASE      equ 0xF211C           ; Character set base address
ANTIC_WSYNC       equ 0xF2120           ; Wait for sync (write only)
ANTIC_VCOUNT      equ 0xF2124           ; Vertical counter (read only)
ANTIC_PENH        equ 0xF2128           ; Light pen H (read only)
ANTIC_PENV        equ 0xF212C           ; Light pen V (read only)
ANTIC_NMIEN       equ 0xF2130           ; NMI enable
ANTIC_NMIST       equ 0xF2134           ; NMI status (read) / NMIRES (write)
ANTIC_ENABLE      equ 0xF2138           ; Video enable (IE-specific)
ANTIC_STATUS      equ 0xF213C           ; Status (VBlank flag)

; DMACTL bits
ANTIC_DMA_NARROW  equ 0x01              ; Narrow playfield
ANTIC_DMA_NORMAL  equ 0x02              ; Normal playfield
ANTIC_DMA_WIDE    equ 0x03              ; Wide playfield
ANTIC_DMA_MISSILE equ 0x04              ; Enable missile DMA
ANTIC_DMA_PLAYER  equ 0x08              ; Enable player DMA
ANTIC_DMA_PMRES   equ 0x10              ; Player/missile resolution
ANTIC_DMA_DL      equ 0x20              ; Enable display list DMA

; CHACTL bits
ANTIC_CHACTL_BLANK   equ 0x01           ; Blank instead of inverse
ANTIC_CHACTL_INVERT  equ 0x02           ; Invert video
ANTIC_CHACTL_REFLECT equ 0x04           ; Reflect characters

; NMIEN bits
ANTIC_NMIEN_DLI   equ 0x80              ; Display list interrupt enable
ANTIC_NMIEN_VBI   equ 0x40              ; Vertical blank interrupt enable

; Display List Instructions
; Blank lines (1-8 scanlines)
DL_BLANK1         equ 0x00              ; 1 blank scanline
DL_BLANK2         equ 0x10              ; 2 blank scanlines
DL_BLANK3         equ 0x20              ; 3 blank scanlines
DL_BLANK4         equ 0x30              ; 4 blank scanlines
DL_BLANK5         equ 0x40              ; 5 blank scanlines
DL_BLANK6         equ 0x50              ; 6 blank scanlines
DL_BLANK7         equ 0x60              ; 7 blank scanlines
DL_BLANK8         equ 0x70              ; 8 blank scanlines

; Jump instructions
DL_JMP            equ 0x01              ; Jump to address (2 bytes follow)
DL_JVB            equ 0x41              ; Jump and wait for Vertical Blank

; Graphics modes
DL_MODE2          equ 0x02              ; 40 col text, 8 scanlines/row
DL_MODE3          equ 0x03              ; 40 col text, 10 scanlines/row
DL_MODE4          equ 0x04              ; 40 col text, 8 scanlines, multicolor
DL_MODE5          equ 0x05              ; 40 col text, 16 scanlines, multicolor
DL_MODE6          equ 0x06              ; 20 col text, 8 scanlines
DL_MODE7          equ 0x07              ; 20 col text, 16 scanlines
DL_MODE8          equ 0x08              ; 40 pixels, 8 scanlines/row (GRAPHICS 3)
DL_MODE9          equ 0x09              ; 80 pixels, 4 scanlines (GRAPHICS 4)
DL_MODE10         equ 0x0A              ; 80 pixels, 2 scanlines (GRAPHICS 5)
DL_MODE11         equ 0x0B              ; 160 pixels, 1 scanline (GRAPHICS 6)
DL_MODE12         equ 0x0C              ; 160 pixels, 1 scanline (GRAPHICS 6+)
DL_MODE13         equ 0x0D              ; 160 pixels, 2 scanlines (GRAPHICS 7)
DL_MODE14         equ 0x0E              ; 160 pixels, 1 scanline, 4 colors
DL_MODE15         equ 0x0F              ; 320 pixels, 1 scanline (GRAPHICS 8)

; Display list modifiers (OR with mode)
DL_LMS            equ 0x40              ; Load Memory Scan (2 addr bytes follow)
DL_DLI            equ 0x80              ; Display List Interrupt at end of line
DL_HSCROL         equ 0x10              ; Horizontal scroll enable
DL_VSCROL         equ 0x20              ; Vertical scroll enable

; Enable/Status bits
ANTIC_ENABLE_VIDEO   equ 0x01           ; Bit 0: Video enable
ANTIC_STATUS_VBLANK  equ 0x01           ; Bit 0: VBlank active

; Display dimensions
ANTIC_DISPLAY_W   equ 320               ; Display width in pixels
ANTIC_DISPLAY_H   equ 192               ; Display height in pixels
ANTIC_FRAME_W     equ 384               ; Total frame width (with border)
ANTIC_FRAME_H     equ 240               ; Total frame height (with border)

; ============================================================================
; GTIA VIDEO REGISTERS (Atari 8-bit color control)
;
; GTIA (Graphics Television Interface Adapter) handles color generation
; and player-missile graphics for Atari 8-bit computers. While ANTIC
; controls the display list and timing, GTIA controls colors and sprites.
;
; Color format: HHHHLLLL (hue in high nibble, luminance in low nibble)
; - 16 hues (0-15) × 8 luminances (0,2,4,6,8,10,12,14) = 128 colors
; - Hue 0 = grayscale, others are chromatic
;
; Registers are 4-byte aligned for copper coprocessor compatibility.
; ============================================================================

GTIA_BASE         equ 0xF2140           ; GTIA register base

; Playfield color registers
GTIA_COLPF0       equ 0xF2140           ; Playfield color 0
GTIA_COLPF1       equ 0xF2144           ; Playfield color 1
GTIA_COLPF2       equ 0xF2148           ; Playfield color 2
GTIA_COLPF3       equ 0xF214C           ; Playfield color 3
GTIA_COLBK        equ 0xF2150           ; Background/border color

; Player color registers
GTIA_COLPM0       equ 0xF2154           ; Player/missile 0 color
GTIA_COLPM1       equ 0xF2158           ; Player/missile 1 color
GTIA_COLPM2       equ 0xF215C           ; Player/missile 2 color
GTIA_COLPM3       equ 0xF2160           ; Player/missile 3 color

; GTIA control registers
GTIA_PRIOR        equ 0xF2164           ; Priority and GTIA modes
GTIA_GRACTL       equ 0xF2168           ; Graphics control
GTIA_CONSOL       equ 0xF216C           ; Console switches (read)

; Player horizontal position registers
GTIA_HPOSP0       equ 0xF2170           ; Player 0 horizontal position
GTIA_HPOSP1       equ 0xF2174           ; Player 1 horizontal position
GTIA_HPOSP2       equ 0xF2178           ; Player 2 horizontal position
GTIA_HPOSP3       equ 0xF217C           ; Player 3 horizontal position

; Missile horizontal position registers
GTIA_HPOSM0       equ 0xF2180           ; Missile 0 horizontal position
GTIA_HPOSM1       equ 0xF2184           ; Missile 1 horizontal position
GTIA_HPOSM2       equ 0xF2188           ; Missile 2 horizontal position
GTIA_HPOSM3       equ 0xF218C           ; Missile 3 horizontal position

; Player/missile size registers
GTIA_SIZEP0       equ 0xF2190           ; Player 0 size (0=normal,1=double,3=quad)
GTIA_SIZEP1       equ 0xF2194           ; Player 1 size
GTIA_SIZEP2       equ 0xF2198           ; Player 2 size
GTIA_SIZEP3       equ 0xF219C           ; Player 3 size
GTIA_SIZEM        equ 0xF21A0           ; All missile sizes (2 bits each)

; Player graphics registers (directly writable)
GTIA_GRAFP0       equ 0xF21A4           ; Player 0 graphics (8 pixels)
GTIA_GRAFP1       equ 0xF21A8           ; Player 1 graphics
GTIA_GRAFP2       equ 0xF21AC           ; Player 2 graphics
GTIA_GRAFP3       equ 0xF21B0           ; Player 3 graphics
GTIA_GRAFM        equ 0xF21B4           ; Missile graphics (2 bits each)

; PRIOR register bits
GTIA_PRIOR_P03    equ 0x01              ; Players 0-3 have priority over PF
GTIA_PRIOR_P01    equ 0x02              ; Players 0-1 have priority
GTIA_PRIOR_P23    equ 0x04              ; Players 2-3 have priority
GTIA_PRIOR_MULTI  equ 0x10              ; Enable multicolor players
GTIA_PRIOR_FIFTH  equ 0x20              ; Enable fifth player
GTIA_PRIOR_GTIA1  equ 0x40              ; GTIA mode bit 0
GTIA_PRIOR_GTIA2  equ 0x80              ; GTIA mode bit 1

; GRACTL register bits
GTIA_GRACTL_MISSLE equ 0x01             ; Enable missile graphics
GTIA_GRACTL_PLAYER equ 0x02             ; Enable player graphics
GTIA_GRACTL_LATCH  equ 0x04             ; Latch trigger inputs

; GTIA register end
GTIA_END          equ 0xF216F

; Wait for GTIA WSYNC (writes to WSYNC wait for horizontal sync)
%macro gtia_wsync 0
    mov byte [ANTIC_WSYNC], 0
%endmacro

; Set background color
%macro gtia_setbk 1
    mov byte [GTIA_COLBK], %1
%endmacro

; ============================================================================
; 3DFX VOODOO GRAPHICS (SST-1) REGISTERS
; ============================================================================
; Vulkan-accelerated 3D graphics. See ie68.inc for detailed documentation.

; Base address
VOODOO_BASE           equ 0xF4000

; Status register (read-only)
VOODOO_STATUS         equ 0xF4000

; Vertex coordinate registers (12.4 fixed-point)
VOODOO_VERTEX_AX      equ 0xF4008
VOODOO_VERTEX_AY      equ 0xF400C
VOODOO_VERTEX_BX      equ 0xF4010
VOODOO_VERTEX_BY      equ 0xF4014
VOODOO_VERTEX_CX      equ 0xF4018
VOODOO_VERTEX_CY      equ 0xF401C

; Vertex attribute start values (12.12 fixed-point)
VOODOO_START_R        equ 0xF4020
VOODOO_START_G        equ 0xF4024
VOODOO_START_B        equ 0xF4028
VOODOO_START_Z        equ 0xF402C
VOODOO_START_A        equ 0xF4030
VOODOO_START_S        equ 0xF4034
VOODOO_START_T        equ 0xF4038
VOODOO_START_W        equ 0xF403C

; Command registers
VOODOO_TRIANGLE_CMD   equ 0xF4080
VOODOO_COLOR_SELECT   equ 0xF4088            ; Select vertex (0/1/2) for Gouraud shading
VOODOO_ALPHA_MODE     equ 0xF410C
VOODOO_FBZ_MODE       equ 0xF4110
VOODOO_CLIP_LEFT_RIGHT equ 0xF4118
VOODOO_CLIP_LOW_Y_HIGH equ 0xF411C
VOODOO_FAST_FILL_CMD  equ 0xF4124
VOODOO_SWAP_BUFFER_CMD equ 0xF4128

; Configuration registers
VOODOO_COLOR0         equ 0xF41D8
VOODOO_VIDEO_DIM      equ 0xF4214

; fbzMode bits
VOODOO_FBZ_CLIPPING     equ 0x0001
VOODOO_FBZ_DEPTH_ENABLE equ 0x0010
VOODOO_FBZ_RGB_WRITE    equ 0x0200
VOODOO_FBZ_DEPTH_WRITE  equ 0x0400

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
; Bit 0:     ALPHA_TEST_EN - Enable alpha test
; Bits 1-3:  ALPHA_FUNC - Alpha test function (0-7, same as depth functions)
; Bit 4:     ALPHA_BLEND_EN - Enable alpha blending
; Bits 8-11: SRC_BLEND - Source blend factor
; Bits 12-15: DST_BLEND - Destination blend factor
; Bits 24-31: ALPHA_REF - Alpha reference value (0-255)
VOODOO_ALPHA_TEST_EN    equ 0x0001          ; Enable alpha test
VOODOO_ALPHA_FUNC_MASK  equ 0x000E          ; Alpha function mask (bits 1-3)
VOODOO_ALPHA_FUNC_SHIFT equ 1               ; Shift for alpha function
VOODOO_ALPHA_BLEND_EN   equ 0x0010          ; Enable alpha blending
VOODOO_ALPHA_SRC_RGB    equ 0x0F00          ; Source RGB blend factor (bits 8-11)
VOODOO_ALPHA_DST_RGB    equ 0xF000          ; Dest RGB blend factor (bits 12-15)
VOODOO_ALPHA_REF_SHIFT  equ 24              ; Shift for alpha reference value

; Alpha test functions (shift left 1 to position in alphaMode)
; Same comparison semantics as depth functions
VOODOO_ALPHA_NEVER        equ 0             ; Never pass
VOODOO_ALPHA_LESS         equ 1             ; Pass if alpha < reference
VOODOO_ALPHA_EQUAL        equ 2             ; Pass if alpha == reference
VOODOO_ALPHA_LESSEQUAL    equ 3             ; Pass if alpha <= reference
VOODOO_ALPHA_GREATER      equ 4             ; Pass if alpha > reference
VOODOO_ALPHA_NOTEQUAL     equ 5             ; Pass if alpha != reference
VOODOO_ALPHA_GREATEREQUAL equ 6             ; Pass if alpha >= reference
VOODOO_ALPHA_ALWAYS       equ 7             ; Always pass

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

; textureMode bits
; Bit layout:
;   Bit 0:     TEX_ENABLE - Enable texture mapping
;   Bits 1-3:  TEX_MINIFY - Minification filter (0=point)
;   Bit 4:     TEX_MAGNIFY - Magnification filter (0=point, 1=bilinear)
;   Bit 5:     TEX_CLAMP_S - Clamp S coordinate (vs wrap)
;   Bit 6:     TEX_CLAMP_T - Clamp T coordinate (vs wrap)
;   Bits 8-11: TEX_FORMAT - Texture format
VOODOO_TEX_ENABLE       equ 0001h           ; Enable texture mapping
VOODOO_TEX_MINIFY       equ 000Eh           ; Minification filter mask (bits 1-3)
VOODOO_TEX_MAGNIFY      equ 0010h           ; Magnification filter (0=point, 1=bilinear)
VOODOO_TEX_CLAMP_S      equ 0020h           ; Clamp S coordinate
VOODOO_TEX_CLAMP_T      equ 0040h           ; Clamp T coordinate
VOODOO_TEX_FORMAT       equ 0F00h           ; Texture format mask (bits 8-11)
VOODOO_TEX_FORMAT_SHIFT equ 8               ; Shift for texture format

; Texture format constants (for TEX_FORMAT field)
VOODOO_TEX_FMT_PALETTE  equ 0               ; 8-bit paletted
VOODOO_TEX_FMT_P8       equ 5               ; 8-bit palette (alternative)
VOODOO_TEX_FMT_ARGB1555 equ 8               ; ARGB 1555
VOODOO_TEX_FMT_ARGB4444 equ 9               ; ARGB 4444
VOODOO_TEX_FMT_ARGB8888 equ 10              ; ARGB 8888 (default)

; Fixed-point shift amounts
VOODOO_FIXED_12_4       equ 4
VOODOO_FIXED_12_12      equ 12

; fbzColorPath bit fields (Color Combine)
; The fbzColorPath register controls how texture and iterated (vertex) colors are combined
VOODOO_FCP_RGB_SELECT_MASK  equ 0x03        ; Bits 0-1: RGB source select
VOODOO_FCP_RGB_SELECT_SHIFT equ 0           ; Shift for RGB select
VOODOO_FCP_A_SELECT_MASK    equ 0x0C        ; Bits 2-3: Alpha source select
VOODOO_FCP_A_SELECT_SHIFT   equ 2           ; Shift for alpha select
VOODOO_FCP_CC_MSELECT_MASK  equ 0x70        ; Bits 4-6: Color combine mode select
VOODOO_FCP_CC_MSELECT_SHIFT equ 4           ; Shift for CC mode
VOODOO_FCP_TEXTURE_ENABLE   equ 0x08000000  ; Bit 27: Enable texture in color path

; Color source select values (for RGB_SELECT and A_SELECT)
VOODOO_CC_ITERATED      equ 0               ; Use iterated (vertex) color
VOODOO_CC_TEXTURE       equ 1               ; Use texture color
VOODOO_CC_COLOR1        equ 2               ; Use constant color1
VOODOO_CC_LFB           equ 3               ; Use linear framebuffer color

; Color combine function modes (for CC_MSELECT)
; These define how the two color sources are combined
VOODOO_CC_ZERO          equ 0               ; Output zero (black)
VOODOO_CC_CSUB_CL       equ 1               ; cother - clocal (subtract)
VOODOO_CC_ALOCAL        equ 2               ; clocal * alocal (modulate by local alpha)
VOODOO_CC_AOTHER        equ 3               ; clocal * aother (modulate by other alpha)
VOODOO_CC_CLOCAL        equ 4               ; clocal only (pass through)
VOODOO_CC_ALOCAL_T      equ 5               ; alocal * texture (alpha * texture)
VOODOO_CC_CLOC_MUL      equ 6               ; clocal * cother (multiply/modulate)
VOODOO_CC_AOTHER_T      equ 7               ; aother * texture

; Simplified color combine modes for common operations
; These are convenience values that combine select + mode bits
VOODOO_COMBINE_UNSET    equ 0xFFFFFFFF      ; Not explicitly set (use defaults)
VOODOO_COMBINE_ITERATED equ 0x00            ; Vertex color only (default when no texture)
VOODOO_COMBINE_TEXTURE  equ 0x01            ; Texture color only
VOODOO_COMBINE_MODULATE equ 0x61            ; tex * vert (texture modulated by vertex color)
VOODOO_COMBINE_ADD      equ 0x81            ; tex + vert (additive blend, clamped)
VOODOO_COMBINE_DECAL    equ 0x41            ; texture with vertex alpha

; Phase 6: fogMode bit fields
; The fogMode register controls depth-based fog blending
VOODOO_FOG_ENABLE       equ 0x0001          ; Enable fog processing
VOODOO_FOG_ADD          equ 0x0002          ; Add fog color to output (vs blend)
VOODOO_FOG_MULT         equ 0x0004          ; Multiply fog factor by alpha
VOODOO_FOG_ZALPHA       equ 0x0008          ; Use Z alpha for fog (vs iterated)
VOODOO_FOG_CONSTANT     equ 0x0010          ; Use constant fog alpha
VOODOO_FOG_DITHER       equ 0x0020          ; Apply dithering to fog
VOODOO_FOG_ZONES        equ 0x0040          ; Enable fog zones (table-based fog)
VOODOO_FOG_TABLE_SHIFT  equ 8               ; Shift for fog table index
VOODOO_FOG_TABLE_MASK   equ 0x3F            ; 6-bit fog table index mask

; Phase 6: Fog table constants
VOODOO_FOG_TABLE_BASE   equ 0xF4140         ; Fog table base address (64 entries)
VOODOO_FOG_TABLE_SIZE   equ 64              ; Number of fog table entries

; Phase 6: fbzMode dithering bits (add to fbzMode)
VOODOO_FBZ_DITHER       equ 0x0100          ; Enable 4x4 ordered dithering
VOODOO_FBZ_DITHER_2X2   equ 0x0800          ; Use 2x2 dither pattern (vs 4x4)

; ----- Voodoo helper macros -----

; Submit triangle
%macro voodoo_triangle 0
    mov dword [VOODOO_TRIANGLE_CMD], 0
%endmacro

; Swap buffers
%macro voodoo_swap 0
    mov dword [VOODOO_SWAP_BUFFER_CMD], 0
%endmacro

; Clear framebuffer with color (ARGB)
%macro voodoo_clear 1
    mov dword [VOODOO_COLOR0], %1
    mov dword [VOODOO_FAST_FILL_CMD], 0
%endmacro

; ============================================================================
; EOF
; ============================================================================
