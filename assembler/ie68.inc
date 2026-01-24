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
SQUARE_FREQ       equ $F0900            ; Frequency (Hz)
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
TRI_FREQ          equ $F0940            ; Frequency (Hz)
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
SINE_FREQ         equ $F0980            ; Frequency (Hz)
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
NOISE_FREQ        equ $F09C0            ; Frequency/pitch (Hz)
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
SAW_FREQ          equ $F0A20            ; Frequency (Hz)
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
; Flexible 4-Channel Synth Block ($F0A80-$F0B3F)
;
; Modern unified interface for all 4 channels. Each channel is $30 bytes.
; Any channel can be any waveform type. This is the preferred interface.
; ----------------------------------------------------------------------------
FLEX_CH_BASE      equ $F0A80            ; Base address of flexible block
FLEX_CH_STRIDE    equ $30               ; Bytes per channel (48)

FLEX_CH0_BASE     equ $F0A80            ; Channel 0 base
FLEX_CH1_BASE     equ $F0AB0            ; Channel 1 base
FLEX_CH2_BASE     equ $F0AE0            ; Channel 2 base
FLEX_CH3_BASE     equ $F0B10            ; Channel 3 base

; Register offsets within each flexible channel (add to FLEX_CHx_BASE)
FLEX_OFF_FREQ       equ $00             ; Frequency (Hz)
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

; Set channel frequency
set_ch_freq macro ch,freq
    move.l  #\freq,FLEX_CH0_BASE+(\ch*FLEX_CH_STRIDE)+FLEX_OFF_FREQ
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
; EOF
; ============================================================================
