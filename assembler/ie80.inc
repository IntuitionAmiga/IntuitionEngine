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
; POKEY AUDIO REGISTERS (Atari-style addresses $D200-$D209)
; ============================================================================
; For SAP file playback and Atari 8-bit sound emulation

.set POKEY_BASE,0xD200
.set POKEY_AUDF1,0xD200
.set POKEY_AUDC1,0xD201
.set POKEY_AUDF2,0xD202
.set POKEY_AUDC2,0xD203
.set POKEY_AUDF3,0xD204
.set POKEY_AUDC3,0xD205
.set POKEY_AUDF4,0xD206
.set POKEY_AUDC4,0xD207
.set POKEY_AUDCTL,0xD208
.set POKEY_PLUS_CTRL,0xD209

; AUDCTL bit masks
.set AUDCTL_CLOCK_15KHZ,0x01
.set AUDCTL_HIPASS_CH1,0x02
.set AUDCTL_HIPASS_CH2,0x04
.set AUDCTL_CH4_BY_CH3,0x08
.set AUDCTL_CH2_BY_CH1,0x10
.set AUDCTL_CH3_179MHZ,0x20
.set AUDCTL_CH1_179MHZ,0x40
.set AUDCTL_POLY9,0x80

; AUDC distortion modes (bits 5-7)
.set POKEY_DIST_POLY17_POLY5,0x00
.set POKEY_DIST_POLY5,0x20
.set POKEY_DIST_POLY17_POLY4,0x40
.set POKEY_DIST_POLY5_POLY4,0x60
.set POKEY_DIST_POLY17,0x80
.set POKEY_DIST_PURE_TONE,0xA0
.set POKEY_DIST_POLY4,0xC0
.set POKEY_DIST_POLY17_PULSE,0xE0

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
; For Z80: Use OUT (SID_PORT_SELECT),A to select register index,
;          then OUT (SID_PORT_DATA),A to write data
;
; SID+ mode (write 1 to register $19) enables enhanced audio
; ----------------------------------------------------------------------------

; Z80 port I/O for SID access
.set SID_PORT_SELECT,0xE0               ; Port to select SID register
.set SID_PORT_DATA,0xE1                 ; Port to read/write SID data

; SID register indices (for port I/O)
; Voice 1 registers (0x00-0x06)
.set SID_V1_FREQ_LO,0x00                ; Voice 1 frequency low byte
.set SID_V1_FREQ_HI,0x01                ; Voice 1 frequency high byte
.set SID_V1_PW_LO,0x02                  ; Voice 1 pulse width low byte
.set SID_V1_PW_HI,0x03                  ; Voice 1 pulse width high (bits 0-3)
.set SID_V1_CTRL,0x04                   ; Voice 1 control register
.set SID_V1_AD,0x05                     ; Voice 1 attack/decay
.set SID_V1_SR,0x06                     ; Voice 1 sustain/release

; Voice 2 registers (0x07-0x0D)
.set SID_V2_FREQ_LO,0x07                ; Voice 2 frequency low byte
.set SID_V2_FREQ_HI,0x08                ; Voice 2 frequency high byte
.set SID_V2_PW_LO,0x09                  ; Voice 2 pulse width low byte
.set SID_V2_PW_HI,0x0A                  ; Voice 2 pulse width high
.set SID_V2_CTRL,0x0B                   ; Voice 2 control register
.set SID_V2_AD,0x0C                     ; Voice 2 attack/decay
.set SID_V2_SR,0x0D                     ; Voice 2 sustain/release

; Voice 3 registers (0x0E-0x14)
.set SID_V3_FREQ_LO,0x0E                ; Voice 3 frequency low byte
.set SID_V3_FREQ_HI,0x0F                ; Voice 3 frequency high byte
.set SID_V3_PW_LO,0x10                  ; Voice 3 pulse width low byte
.set SID_V3_PW_HI,0x11                  ; Voice 3 pulse width high
.set SID_V3_CTRL,0x12                   ; Voice 3 control register
.set SID_V3_AD,0x13                     ; Voice 3 attack/decay
.set SID_V3_SR,0x14                     ; Voice 3 sustain/release

; Filter and volume registers
.set SID_FC_LO,0x15                     ; Filter cutoff low (bits 0-2)
.set SID_FC_HI,0x16                     ; Filter cutoff high byte
.set SID_RES_FILT,0x17                  ; Resonance (bits 4-7) + routing (bits 0-3)
.set SID_MODE_VOL,0x18                  ; Volume (bits 0-3) + filter mode (bits 4-7)
.set SID_PLUS_CTRL,0x19                 ; SID+ mode (0=standard, 1=enhanced)

; Voice control register bits (SID_Vx_CTRL)
.set SID_CTRL_GATE,0x01                 ; Gate bit (trigger envelope)
.set SID_CTRL_SYNC,0x02                 ; Sync with previous voice
.set SID_CTRL_RINGMOD,0x04              ; Ring modulation
.set SID_CTRL_TEST,0x08                 ; Test bit (resets oscillator)
.set SID_CTRL_TRIANGLE,0x10             ; Triangle waveform
.set SID_CTRL_SAWTOOTH,0x20             ; Sawtooth waveform
.set SID_CTRL_PULSE,0x40                ; Pulse/square waveform
.set SID_CTRL_NOISE,0x80                ; Noise waveform

; Filter routing bits (SID_RES_FILT bits 0-3)
.set SID_FILT_V1,0x01                   ; Route voice 1 through filter
.set SID_FILT_V2,0x02                   ; Route voice 2 through filter
.set SID_FILT_V3,0x04                   ; Route voice 3 through filter
.set SID_FILT_EXT,0x08                  ; Route external input through filter

; Filter mode bits (SID_MODE_VOL bits 4-7)
.set SID_MODE_LP,0x10                   ; Low-pass filter
.set SID_MODE_BP,0x20                   ; Band-pass filter
.set SID_MODE_HP,0x40                   ; High-pass filter
.set SID_MODE_3OFF,0x80                 ; Disconnect voice 3 from output

; ----------------------------------------------------------------------------
; SID Player Registers
;
; The SID player handles Commodore 64 music files (.sid) which contain
; embedded 6502 code that drives the SID sound chip. The player executes
; the 6502 init routine once, then calls the play routine each frame.
;
; Usage:
;   1. Embed SID data in your program using incbin
;   2. Set SID_PLAY_PTR bytes to the address of the SID data (little-endian)
;   3. Set SID_PLAY_LEN bytes to the size of the SID data (little-endian)
;   4. Optionally set SID_SUBSONG to select a subsong (default 0)
;   5. Write to SID_PLAY_CTRL to start/stop playback
;
; SID_PLAY_CTRL bits:
;   bit 0 (0x01) - Start playback (write 1 to begin)
;   bit 1 (0x02) - Stop playback (write 1 to stop)
;   bit 2 (0x04) - Enable looping (set before starting)
;
; SID_PLAY_STATUS bits:
;   bit 0 (0x01) - Busy (1 = playing, 0 = stopped)
;   bit 1 (0x02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
.set SID_PLAY_PTR_0,0xF0E20
.set SID_PLAY_PTR_1,0xF0E21
.set SID_PLAY_PTR_2,0xF0E22
.set SID_PLAY_PTR_3,0xF0E23
.set SID_PLAY_LEN_0,0xF0E24
.set SID_PLAY_LEN_1,0xF0E25
.set SID_PLAY_LEN_2,0xF0E26
.set SID_PLAY_LEN_3,0xF0E27
.set SID_PLAY_CTRL,0xF0E28
.set SID_PLAY_STATUS,0xF0E2C
.set SID_SUBSONG,0xF0E2D

; ============================================================================
; TED AUDIO REGISTERS (Commodore Plus/4 style)
;
; The TED chip provides 2 voices with simple square wave synthesis:
; - 10-bit frequency control per voice (0-1023)
; - Voice 2 can optionally produce white noise
; - Global 4-bit volume (0-8, where 8 is maximum)
;
; For Z80: Use OUT (TED_PORT_SELECT),A to select register index,
;          then OUT (TED_PORT_DATA),A to write data
;
; TED+ mode (write 1 to register 0x05) enables enhanced audio:
; - 4x oversampling for cleaner waveforms
; - Soft low-pass filtering for smoother output
; - Subtle saturation for analog warmth
; - Room reverb for spatial depth
; - Logarithmic volume curve for more musical response
; - Per-voice stereo separation
;
; Frequency formula: freq_hz = clock/8 / (1024 - register_value)
; Clock: 886724 Hz (PAL), 894886 Hz (NTSC)
; ============================================================================

; Z80 port I/O for TED access
.set TED_PORT_SELECT,0xF2               ; Port to select TED register
.set TED_PORT_DATA,0xF3                 ; Port to read/write TED data

; TED register indices (for port I/O)
.set TED_FREQ1_LO,0x00                  ; Voice 1 frequency low byte
.set TED_FREQ2_LO,0x01                  ; Voice 2 frequency low byte
.set TED_FREQ2_HI,0x02                  ; Voice 2 frequency high (bits 0-1)
.set TED_SND_CTRL,0x03                  ; Sound control register
.set TED_FREQ1_HI,0x04                  ; Voice 1 frequency high (bits 0-1)
.set TED_PLUS_CTRL,0x05                 ; TED+ mode (0=standard, 1=enhanced)

; TED Control register bits (TED_SND_CTRL)
.set TED_CTRL_SNDDC,0x80                ; D/A mode (bit 7)
.set TED_CTRL_SND2NOISE,0x40            ; Voice 2 noise enable (bit 6)
.set TED_CTRL_SND2ON,0x20               ; Voice 2 enable (bit 5)
.set TED_CTRL_SND1ON,0x10               ; Voice 1 enable (bit 4)
.set TED_CTRL_VOLUME,0x0F               ; Volume mask (bits 0-3, max=8)

; ----------------------------------------------------------------------------
; TED Player Registers
;
; The TED player handles Commodore Plus/4 music files (.ted) which contain
; embedded 6502 code that drives the TED sound chip. The player executes
; the 6502 init routine once, then calls the play routine each frame.
;
; Usage:
;   1. Embed TED data in your program using incbin
;   2. Set TED_PLAY_PTR bytes to the address of the TED data (little-endian)
;   3. Set TED_PLAY_LEN bytes to the size of the TED data (little-endian)
;   4. Write to TED_PLAY_CTRL to start/stop playback
;
; TED_PLAY_CTRL bits:
;   bit 0 (0x01) - Start playback (write 1 to begin)
;   bit 1 (0x02) - Stop playback (write 1 to stop)
;   bit 2 (0x04) - Enable looping (set before starting)
;
; TED_PLAY_STATUS bits:
;   bit 0 (0x01) - Busy (1 = playing, 0 = stopped)
;   bit 1 (0x02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
.set TED_PLAY_PTR_0,0xF0F10
.set TED_PLAY_PTR_1,0xF0F11
.set TED_PLAY_PTR_2,0xF0F12
.set TED_PLAY_PTR_3,0xF0F13
.set TED_PLAY_LEN_0,0xF0F14
.set TED_PLAY_LEN_1,0xF0F15
.set TED_PLAY_LEN_2,0xF0F16
.set TED_PLAY_LEN_3,0xF0F17
.set TED_PLAY_CTRL,0xF0F18
.set TED_PLAY_STATUS,0xF0F1C

; ----------------------------------------------------------------------------
; SAP Player Registers
;
; The SAP player handles Atari 8-bit music files (.sap) which contain
; embedded 6502 code that drives the POKEY sound chip. Supports TYPE B files
; where INIT is called once and PLAYER is called each frame.
;
; Usage:
;   1. Embed SAP data in your program using incbin
;   2. Set SAP_PLAY_PTR bytes to the address of the SAP data (little-endian)
;   3. Set SAP_PLAY_LEN bytes to the size of the SAP data (little-endian)
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
.set SAP_PLAY_PTR_0,0xF0D10
.set SAP_PLAY_PTR_1,0xF0D11
.set SAP_PLAY_PTR_2,0xF0D12
.set SAP_PLAY_PTR_3,0xF0D13
.set SAP_PLAY_LEN_0,0xF0D14
.set SAP_PLAY_LEN_1,0xF0D15
.set SAP_PLAY_LEN_2,0xF0D16
.set SAP_PLAY_LEN_3,0xF0D17
.set SAP_PLAY_CTRL,0xF0D18
.set SAP_PLAY_STATUS,0xF0D1C
.set SAP_SUBSONG,0xF0D1D

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
.set AHX_PLUS_CTRL,0xF0B80              ; AHX+ mode (0=standard, 1=enhanced)

; ----------------------------------------------------------------------------
; AHX Player Registers
;
; The AHX player handles Amiga AHX/THX modules which contain instrument
; definitions and pattern data for 4-channel waveform synthesis.
;
; Usage:
;   1. Embed AHX data in your program using incbin
;   2. Set AHX_PLAY_PTR bytes to the address of the AHX data (little-endian)
;   3. Set AHX_PLAY_LEN bytes to the size of the AHX data (little-endian)
;   4. Optionally set AHX_SUBSONG to select a subsong (default 0)
;   5. Optionally write 1 to AHX_PLUS_CTRL to enable enhanced mode
;   6. Write to AHX_PLAY_CTRL to start/stop playback
;
; AHX_PLAY_CTRL bits:
;   bit 0 (0x01) - Start playback (write 1 to begin)
;   bit 1 (0x02) - Stop playback (write 1 to stop)
;   bit 2 (0x04) - Enable looping (set before starting)
;
; AHX_PLAY_STATUS bits:
;   bit 0 (0x01) - Busy (1 = playing, 0 = stopped)
;   bit 1 (0x02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
.set AHX_PLAY_PTR_0,0xF0B84
.set AHX_PLAY_PTR_1,0xF0B85
.set AHX_PLAY_PTR_2,0xF0B86
.set AHX_PLAY_PTR_3,0xF0B87
.set AHX_PLAY_LEN_0,0xF0B88
.set AHX_PLAY_LEN_1,0xF0B89
.set AHX_PLAY_LEN_2,0xF0B8A
.set AHX_PLAY_LEN_3,0xF0B8B
.set AHX_PLAY_CTRL,0xF0B8C
.set AHX_PLAY_STATUS,0xF0B90
.set AHX_SUBSONG,0xF0B91

; ============================================================================
; VGA VIDEO PORTS (IBM VGA compatible via port I/O)
;
; The VGA chip provides IBM PC-compatible graphics modes:
; - Mode 0x03: 80x25 text mode, 16 colors
; - Mode 0x12: 640x480, 16 colors, planar
; - Mode 0x13: 320x200, 256 colors, linear (Mode 13h)
; - Mode 0x14: 320x240, 256 colors, planar (Mode X)
;
; For Z80: Use OUT (port),A to write, IN A,(port) to read
; ============================================================================

; VGA port I/O mapping
.set VGA_PORT_MODE,0xA0
.set VGA_PORT_STATUS,0xA1
.set VGA_PORT_CTRL,0xA2
.set VGA_PORT_SEQ_IDX,0xA3
.set VGA_PORT_SEQ_DATA,0xA4
.set VGA_PORT_CRTC_IDX,0xA5
.set VGA_PORT_CRTC_DATA,0xA6
.set VGA_PORT_GC_IDX,0xA7
.set VGA_PORT_GC_DATA,0xA8
.set VGA_PORT_DAC_WIDX,0xA9
.set VGA_PORT_DAC_DATA,0xAA
.set VGA_PORT_DAC_RIDX,0xAB
.set VGA_PORT_DAC_MASK,0xAC

; VGA VRAM window (banked at 0xA000 in Z80 address space)
.set VGA_VRAM_BANK,0xA000
.set VGA_VRAM_SIZE,0x4000

; Mode constants
.set VGA_MODE_TEXT,0x03
.set VGA_MODE_12H,0x12
.set VGA_MODE_13H,0x13
.set VGA_MODE_X,0x14

; Status bits
.set VGA_STATUS_VSYNC,0x01
.set VGA_STATUS_RETRACE,0x08

; Control bits
.set VGA_CTRL_ENABLE,0x01

; ----- VGA helper macros -----

; Set VGA mode
.macro VGA_SETMODE mode
    ld a,\mode
    out (VGA_PORT_MODE),a
.endm

; Enable VGA
.macro VGA_ENABLE
    ld a,VGA_CTRL_ENABLE
    out (VGA_PORT_CTRL),a
.endm

; Set palette write index
.macro VGA_SETPAL index
    ld a,\index
    out (VGA_PORT_DAC_WIDX),a
.endm

; Write palette RGB component
.macro VGA_PALDATA value
    ld a,\value
    out (VGA_PORT_DAC_DATA),a
.endm

; Wait for VSync
.macro VGA_WAIT_VSYNC
.wait_vga_vs\@:
    in a,(VGA_PORT_STATUS)
    and VGA_STATUS_VSYNC
    jr z,.wait_vga_vs\@
.endm

; Set sequencer register
.macro VGA_SEQ_WRITE idx,val
    ld a,\idx
    out (VGA_PORT_SEQ_IDX),a
    ld a,\val
    out (VGA_PORT_SEQ_DATA),a
.endm

; Set graphics controller register
.macro VGA_GC_WRITE idx,val
    ld a,\idx
    out (VGA_PORT_GC_IDX),a
    ld a,\val
    out (VGA_PORT_GC_DATA),a
.endm

; ============================================================================
; CUSTOM AUDIO CHIP REGISTERS
; ============================================================================
;
; The Intuition Engine features a powerful custom 4-channel audio synthesizer
; with additional flexible synthesis channels. This chip provides:
;
;   - 5 dedicated waveform channels: Square, Triangle, Sine, Noise, Sawtooth
;   - 4 flexible synthesis channels with selectable waveforms
;   - Per-voice ADSR envelope generators (16-bit parameters)
;   - Pulse width modulation with LFO
;   - Frequency sweep (portamento/pitch bend)
;   - Hard sync between oscillators
;   - Ring modulation
;   - Global resonant filter (low-pass, high-pass, band-pass)
;   - Effects: overdrive distortion, reverb
;
; Memory Map (Z80 addresses, mapped from IE32 0xF08xx-0xF0Bxx to 0xF8xx-0xFBxx):
;   0xF800-0xF807   Global audio control
;   0xF820-0xF833   Filter parameters
;   0xF900-0xF93F   Square wave channel
;   0xF940-0xF97F   Triangle wave channel
;   0xF980-0xF9BF   Sine wave channel
;   0xF9C0-0xF9FF   Noise channel
;   0xFA00-0xFA0F   Sync sources (per channel)
;   0xFA10-0xFA1F   Ring mod sources (per channel)
;   0xFA20-0xFA5F   Sawtooth wave channel
;   0xFA40-0xFA5F   Effects (overdrive, reverb)
;   0xFA80-0xFB3F   Flexible synth channels (4 channels x 0x30 bytes each)
;
; Note: All 32-bit values are stored in LITTLE-ENDIAN byte order.
; ----------------------------------------------------------------------------

; ----------------------------------------------------------------------------
; Global Audio Control
; ----------------------------------------------------------------------------
.set AUDIO_CTRL_0,0xF800
.set AUDIO_CTRL_1,0xF801
.set AUDIO_CTRL_2,0xF802
.set AUDIO_CTRL_3,0xF803
.set ENV_SHAPE_0,0xF804
.set ENV_SHAPE_1,0xF805
.set ENV_SHAPE_2,0xF806
.set ENV_SHAPE_3,0xF807

; ----------------------------------------------------------------------------
; Global Filter
; ----------------------------------------------------------------------------
.set FILTER_CUTOFF_0,0xF820
.set FILTER_CUTOFF_1,0xF821
.set FILTER_CUTOFF_2,0xF822
.set FILTER_CUTOFF_3,0xF823
.set FILTER_RESONANCE_0,0xF824
.set FILTER_RESONANCE_1,0xF825
.set FILTER_RESONANCE_2,0xF826
.set FILTER_RESONANCE_3,0xF827
.set FILTER_TYPE_0,0xF828
.set FILTER_TYPE_1,0xF829
.set FILTER_TYPE_2,0xF82A
.set FILTER_TYPE_3,0xF82B
.set FILTER_MOD_SRC_0,0xF82C
.set FILTER_MOD_SRC_1,0xF82D
.set FILTER_MOD_SRC_2,0xF82E
.set FILTER_MOD_SRC_3,0xF82F
.set FILTER_MOD_AMT_0,0xF830
.set FILTER_MOD_AMT_1,0xF831
.set FILTER_MOD_AMT_2,0xF832
.set FILTER_MOD_AMT_3,0xF833

; Filter type values
.set FILT_OFF,0
.set FILT_LOWPASS,1
.set FILT_HIGHPASS,2
.set FILT_BANDPASS,3

; ----------------------------------------------------------------------------
; Square Wave Channel (0xF900-0xF93F, maps from IE32 0xF0900)
; ----------------------------------------------------------------------------
.set SQ_FREQ_0,0xF900
.set SQ_FREQ_1,0xF901
.set SQ_FREQ_2,0xF902
.set SQ_FREQ_3,0xF903
.set SQ_VOL_0,0xF904
.set SQ_VOL_1,0xF905
.set SQ_VOL_2,0xF906
.set SQ_VOL_3,0xF907
.set SQ_CTRL_0,0xF908
.set SQ_CTRL_1,0xF909
.set SQ_CTRL_2,0xF90A
.set SQ_CTRL_3,0xF90B
.set SQ_ATTACK_0,0xF90C
.set SQ_ATTACK_1,0xF90D
.set SQ_DECAY_0,0xF910
.set SQ_DECAY_1,0xF911
.set SQ_SUSTAIN_0,0xF914
.set SQ_SUSTAIN_1,0xF915
.set SQ_RELEASE_0,0xF918
.set SQ_RELEASE_1,0xF919
.set SQ_DUTY_0,0xF91C
.set SQ_DUTY_1,0xF91D
.set SQ_PWM_RATE_0,0xF920
.set SQ_PWM_RATE_1,0xF921
.set SQ_PWM_RATE_2,0xF922
.set SQ_PWM_RATE_3,0xF923
.set SQ_PWM_DEPTH_0,0xF924
.set SQ_PWM_DEPTH_1,0xF925
.set SQ_PWM_DEPTH_2,0xF926
.set SQ_PWM_DEPTH_3,0xF927
.set SQ_SWEEP_RATE_0,0xF928
.set SQ_SWEEP_RATE_1,0xF929
.set SQ_SWEEP_RATE_2,0xF92A
.set SQ_SWEEP_RATE_3,0xF92B
.set SQ_SWEEP_DIR_0,0xF92C
.set SQ_SWEEP_DIR_1,0xF92D
.set SQ_SWEEP_DIR_2,0xF92E
.set SQ_SWEEP_DIR_3,0xF92F
.set SQ_SWEEP_AMT_0,0xF930
.set SQ_SWEEP_AMT_1,0xF931
.set SQ_SWEEP_AMT_2,0xF932
.set SQ_SWEEP_AMT_3,0xF933
.set SQ_TARGET_0,0xF934
.set SQ_TARGET_1,0xF935
.set SQ_TARGET_2,0xF936
.set SQ_TARGET_3,0xF937

; ----------------------------------------------------------------------------
; Triangle Wave Channel (0xF940-0xF97F, maps from IE32 0xF0940)
; ----------------------------------------------------------------------------
.set TRI_FREQ_0,0xF940
.set TRI_FREQ_1,0xF941
.set TRI_FREQ_2,0xF942
.set TRI_FREQ_3,0xF943
.set TRI_VOL_0,0xF944
.set TRI_VOL_1,0xF945
.set TRI_VOL_2,0xF946
.set TRI_VOL_3,0xF947
.set TRI_CTRL_0,0xF948
.set TRI_CTRL_1,0xF949
.set TRI_CTRL_2,0xF94A
.set TRI_CTRL_3,0xF94B
.set TRI_ATTACK_0,0xF94C
.set TRI_ATTACK_1,0xF94D
.set TRI_DECAY_0,0xF950
.set TRI_DECAY_1,0xF951
.set TRI_SUSTAIN_0,0xF954
.set TRI_SUSTAIN_1,0xF955
.set TRI_RELEASE_0,0xF958
.set TRI_RELEASE_1,0xF959
.set TRI_SWEEP_RATE_0,0xF968
.set TRI_SWEEP_RATE_1,0xF969
.set TRI_SWEEP_RATE_2,0xF96A
.set TRI_SWEEP_RATE_3,0xF96B
.set TRI_SWEEP_DIR_0,0xF96C
.set TRI_SWEEP_DIR_1,0xF96D
.set TRI_SWEEP_DIR_2,0xF96E
.set TRI_SWEEP_DIR_3,0xF96F
.set TRI_SWEEP_AMT_0,0xF970
.set TRI_SWEEP_AMT_1,0xF971
.set TRI_SWEEP_AMT_2,0xF972
.set TRI_SWEEP_AMT_3,0xF973
.set TRI_TARGET_0,0xF974
.set TRI_TARGET_1,0xF975
.set TRI_TARGET_2,0xF976
.set TRI_TARGET_3,0xF977

; ----------------------------------------------------------------------------
; Sine Wave Channel (0xF980-0xF9BF, maps from IE32 0xF0980)
; ----------------------------------------------------------------------------
.set SINE_FREQ_0,0xF980
.set SINE_FREQ_1,0xF981
.set SINE_FREQ_2,0xF982
.set SINE_FREQ_3,0xF983
.set SINE_VOL_0,0xF984
.set SINE_VOL_1,0xF985
.set SINE_VOL_2,0xF986
.set SINE_VOL_3,0xF987
.set SINE_CTRL_0,0xF988
.set SINE_CTRL_1,0xF989
.set SINE_CTRL_2,0xF98A
.set SINE_CTRL_3,0xF98B
.set SINE_ATTACK_0,0xF98C
.set SINE_ATTACK_1,0xF98D
.set SINE_DECAY_0,0xF990
.set SINE_DECAY_1,0xF991
.set SINE_SUSTAIN_0,0xF994
.set SINE_SUSTAIN_1,0xF995
.set SINE_RELEASE_0,0xF998
.set SINE_RELEASE_1,0xF999
.set SINE_SWEEP_RATE_0,0xF9A8
.set SINE_SWEEP_RATE_1,0xF9A9
.set SINE_SWEEP_RATE_2,0xF9AA
.set SINE_SWEEP_RATE_3,0xF9AB
.set SINE_SWEEP_DIR_0,0xF9AC
.set SINE_SWEEP_DIR_1,0xF9AD
.set SINE_SWEEP_DIR_2,0xF9AE
.set SINE_SWEEP_DIR_3,0xF9AF
.set SINE_SWEEP_AMT_0,0xF9B0
.set SINE_SWEEP_AMT_1,0xF9B1
.set SINE_SWEEP_AMT_2,0xF9B2
.set SINE_SWEEP_AMT_3,0xF9B3
.set SINE_TARGET_0,0xF9B4
.set SINE_TARGET_1,0xF9B5
.set SINE_TARGET_2,0xF9B6
.set SINE_TARGET_3,0xF9B7

; ----------------------------------------------------------------------------
; Noise Channel (0xF9C0-0xF9FF, maps from IE32 0xF09C0)
; ----------------------------------------------------------------------------
.set NOISE_FREQ_0,0xF9C0
.set NOISE_FREQ_1,0xF9C1
.set NOISE_FREQ_2,0xF9C2
.set NOISE_FREQ_3,0xF9C3
.set NOISE_VOL_0,0xF9C4
.set NOISE_VOL_1,0xF9C5
.set NOISE_VOL_2,0xF9C6
.set NOISE_VOL_3,0xF9C7
.set NOISE_CTRL_0,0xF9C8
.set NOISE_CTRL_1,0xF9C9
.set NOISE_CTRL_2,0xF9CA
.set NOISE_CTRL_3,0xF9CB
.set NOISE_ATTACK_0,0xF9CC
.set NOISE_ATTACK_1,0xF9CD
.set NOISE_DECAY_0,0xF9D0
.set NOISE_DECAY_1,0xF9D1
.set NOISE_SUSTAIN_0,0xF9D4
.set NOISE_SUSTAIN_1,0xF9D5
.set NOISE_RELEASE_0,0xF9D8
.set NOISE_RELEASE_1,0xF9D9
.set NOISE_MODE_0,0xF9DC
.set NOISE_MODE_1,0xF9DD
.set NOISE_MODE_2,0xF9DE
.set NOISE_MODE_3,0xF9DF

; Noise mode values
.set NOISE_WHITE,0
.set NOISE_PERIODIC,1
.set NOISE_METALLIC,2
.set NOISE_PSG,3

; ----------------------------------------------------------------------------
; Sync and Ring Modulation Sources (0xFA00-0xFA1F, maps from IE32 0xF0A00)
; ----------------------------------------------------------------------------
.set SYNC_SRC_CH0_0,0xFA00
.set SYNC_SRC_CH0_1,0xFA01
.set SYNC_SRC_CH0_2,0xFA02
.set SYNC_SRC_CH0_3,0xFA03
.set SYNC_SRC_CH1_0,0xFA04
.set SYNC_SRC_CH1_1,0xFA05
.set SYNC_SRC_CH1_2,0xFA06
.set SYNC_SRC_CH1_3,0xFA07
.set SYNC_SRC_CH2_0,0xFA08
.set SYNC_SRC_CH2_1,0xFA09
.set SYNC_SRC_CH2_2,0xFA0A
.set SYNC_SRC_CH2_3,0xFA0B
.set SYNC_SRC_CH3_0,0xFA0C
.set SYNC_SRC_CH3_1,0xFA0D
.set SYNC_SRC_CH3_2,0xFA0E
.set SYNC_SRC_CH3_3,0xFA0F

.set RING_SRC_CH0_0,0xFA10
.set RING_SRC_CH0_1,0xFA11
.set RING_SRC_CH0_2,0xFA12
.set RING_SRC_CH0_3,0xFA13
.set RING_SRC_CH1_0,0xFA14
.set RING_SRC_CH1_1,0xFA15
.set RING_SRC_CH1_2,0xFA16
.set RING_SRC_CH1_3,0xFA17
.set RING_SRC_CH2_0,0xFA18
.set RING_SRC_CH2_1,0xFA19
.set RING_SRC_CH2_2,0xFA1A
.set RING_SRC_CH2_3,0xFA1B
.set RING_SRC_CH3_0,0xFA1C
.set RING_SRC_CH3_1,0xFA1D
.set RING_SRC_CH3_2,0xFA1E
.set RING_SRC_CH3_3,0xFA1F

; ----------------------------------------------------------------------------
; Sawtooth Wave Channel (0xFA20-0xFA5F, maps from IE32 0xF0A20)
; ----------------------------------------------------------------------------
.set SAW_FREQ_0,0xFA20
.set SAW_FREQ_1,0xFA21
.set SAW_FREQ_2,0xFA22
.set SAW_FREQ_3,0xFA23
.set SAW_VOL_0,0xFA24
.set SAW_VOL_1,0xFA25
.set SAW_VOL_2,0xFA26
.set SAW_VOL_3,0xFA27
.set SAW_CTRL_0,0xFA28
.set SAW_CTRL_1,0xFA29
.set SAW_CTRL_2,0xFA2A
.set SAW_CTRL_3,0xFA2B
.set SAW_ATTACK_0,0xFA2C
.set SAW_ATTACK_1,0xFA2D
.set SAW_DECAY_0,0xFA30
.set SAW_DECAY_1,0xFA31
.set SAW_SUSTAIN_0,0xFA34
.set SAW_SUSTAIN_1,0xFA35
.set SAW_RELEASE_0,0xFA38
.set SAW_RELEASE_1,0xFA39
.set SAW_SWEEP_RATE_0,0xFA48
.set SAW_SWEEP_RATE_1,0xFA49
.set SAW_SWEEP_RATE_2,0xFA4A
.set SAW_SWEEP_RATE_3,0xFA4B
.set SAW_SWEEP_DIR_0,0xFA4C
.set SAW_SWEEP_DIR_1,0xFA4D
.set SAW_SWEEP_DIR_2,0xFA4E
.set SAW_SWEEP_DIR_3,0xFA4F
.set SAW_SWEEP_AMT_0,0xFA50
.set SAW_SWEEP_AMT_1,0xFA51
.set SAW_SWEEP_AMT_2,0xFA52
.set SAW_SWEEP_AMT_3,0xFA53
.set SAW_TARGET_0,0xFA54
.set SAW_TARGET_1,0xFA55
.set SAW_TARGET_2,0xFA56
.set SAW_TARGET_3,0xFA57

; ----------------------------------------------------------------------------
; Effects (0xFA40-0xFA5F, maps from IE32 0xF0A40)
; ----------------------------------------------------------------------------
.set OVERDRIVE_CTRL_0,0xFA40
.set OVERDRIVE_CTRL_1,0xFA41
.set OVERDRIVE_CTRL_2,0xFA42
.set OVERDRIVE_CTRL_3,0xFA43

.set REVERB_MIX_0,0xFA50
.set REVERB_MIX_1,0xFA51
.set REVERB_MIX_2,0xFA52
.set REVERB_MIX_3,0xFA53
.set REVERB_DECAY_0,0xFA54
.set REVERB_DECAY_1,0xFA55
.set REVERB_DECAY_2,0xFA56
.set REVERB_DECAY_3,0xFA57

; ----------------------------------------------------------------------------
; Flexible Synth Channels (0xFA80-0xFB7F, maps from IE32 0xF0A80)
;
; 4 flexible channels, each with 0x40 (64) bytes of registers.
; Each channel can be set to any waveform type and has full ADSR + effects.
;
; Channel base addresses:
;   Flex 0: 0xFA80
;   Flex 1: 0xFAC0
;   Flex 2: 0xFB00
;   Flex 3: 0xFB40
; ----------------------------------------------------------------------------
.set FLEX_CH_STRIDE,0x40

; Flex Channel 0 (0xFA80-0xFAAF)
; Frequency is 16.8 fixed-point: value = Hz * 256 (e.g., 440 Hz = 440*256 = 112640)
.set FLEX_CH0_FREQ_0,0xFA80     ; Flex 0 frequency byte 0 (low byte)
.set FLEX_CH0_FREQ_1,0xFA81     ; Flex 0 frequency byte 1
.set FLEX_CH0_FREQ_2,0xFA82     ; Flex 0 frequency byte 2
.set FLEX_CH0_FREQ_3,0xFA83     ; Flex 0 frequency byte 3 (high byte)
.set FLEX_CH0_VOL_0,0xFA84
.set FLEX_CH0_VOL_1,0xFA85
.set FLEX_CH0_VOL_2,0xFA86
.set FLEX_CH0_VOL_3,0xFA87
.set FLEX_CH0_WAVE_0,0xFA88
.set FLEX_CH0_WAVE_1,0xFA89
.set FLEX_CH0_WAVE_2,0xFA8A
.set FLEX_CH0_WAVE_3,0xFA8B
.set FLEX_CH0_CTRL_0,0xFA8C
.set FLEX_CH0_CTRL_1,0xFA8D
.set FLEX_CH0_CTRL_2,0xFA8E
.set FLEX_CH0_CTRL_3,0xFA8F
.set FLEX_CH0_ATK_0,0xFA90
.set FLEX_CH0_ATK_1,0xFA91
.set FLEX_CH0_DEC_0,0xFA94
.set FLEX_CH0_DEC_1,0xFA95
.set FLEX_CH0_SUS_0,0xFA98
.set FLEX_CH0_SUS_1,0xFA99
.set FLEX_CH0_REL_0,0xFA9C
.set FLEX_CH0_REL_1,0xFA9D
.set FLEX_CH0_DUTY_0,0xFAA0
.set FLEX_CH0_DUTY_1,0xFAA1
.set FLEX_CH0_PAN_0,0xFAA4
.set FLEX_CH0_PAN_1,0xFAA5
.set FLEX_CH0_PAN_2,0xFAA6
.set FLEX_CH0_PAN_3,0xFAA7

; Flex Channel 1 (0xFAC0-0xFAFF)
.set FLEX_CH1_FREQ_0,0xFAC0
.set FLEX_CH1_FREQ_1,0xFAC1
.set FLEX_CH1_FREQ_2,0xFAC2
.set FLEX_CH1_FREQ_3,0xFAC3
.set FLEX_CH1_VOL_0,0xFAC4
.set FLEX_CH1_VOL_1,0xFAC5
.set FLEX_CH1_VOL_2,0xFAC6
.set FLEX_CH1_VOL_3,0xFAC7
.set FLEX_CH1_WAVE_0,0xFAC8
.set FLEX_CH1_WAVE_1,0xFAC9
.set FLEX_CH1_WAVE_2,0xFACA
.set FLEX_CH1_WAVE_3,0xFACB
.set FLEX_CH1_CTRL_0,0xFACC
.set FLEX_CH1_CTRL_1,0xFACD
.set FLEX_CH1_CTRL_2,0xFACE
.set FLEX_CH1_CTRL_3,0xFACF
.set FLEX_CH1_ATK_0,0xFAD0
.set FLEX_CH1_ATK_1,0xFAD1
.set FLEX_CH1_DEC_0,0xFAD4
.set FLEX_CH1_DEC_1,0xFAD5
.set FLEX_CH1_SUS_0,0xFAD8
.set FLEX_CH1_SUS_1,0xFAD9
.set FLEX_CH1_REL_0,0xFADC
.set FLEX_CH1_REL_1,0xFADD
.set FLEX_CH1_DUTY_0,0xFAE0
.set FLEX_CH1_DUTY_1,0xFAE1
.set FLEX_CH1_PAN_0,0xFAE4
.set FLEX_CH1_PAN_1,0xFAE5
.set FLEX_CH1_PAN_2,0xFAE6
.set FLEX_CH1_PAN_3,0xFAE7

; Flex Channel 2 (0xFB00-0xFB3F)
.set FLEX_CH2_FREQ_0,0xFB00
.set FLEX_CH2_FREQ_1,0xFB01
.set FLEX_CH2_FREQ_2,0xFB02
.set FLEX_CH2_FREQ_3,0xFB03
.set FLEX_CH2_VOL_0,0xFB04
.set FLEX_CH2_VOL_1,0xFB05
.set FLEX_CH2_VOL_2,0xFB06
.set FLEX_CH2_VOL_3,0xFB07
.set FLEX_CH2_WAVE_0,0xFB08
.set FLEX_CH2_WAVE_1,0xFB09
.set FLEX_CH2_WAVE_2,0xFB0A
.set FLEX_CH2_WAVE_3,0xFB0B
.set FLEX_CH2_CTRL_0,0xFB0C
.set FLEX_CH2_CTRL_1,0xFB0D
.set FLEX_CH2_CTRL_2,0xFB0E
.set FLEX_CH2_CTRL_3,0xFB0F
.set FLEX_CH2_ATK_0,0xFB10
.set FLEX_CH2_ATK_1,0xFB11
.set FLEX_CH2_DEC_0,0xFB14
.set FLEX_CH2_DEC_1,0xFB15
.set FLEX_CH2_SUS_0,0xFB18
.set FLEX_CH2_SUS_1,0xFB19
.set FLEX_CH2_REL_0,0xFB1C
.set FLEX_CH2_REL_1,0xFB1D
.set FLEX_CH2_DUTY_0,0xFB20
.set FLEX_CH2_DUTY_1,0xFB21
.set FLEX_CH2_PAN_0,0xFB24
.set FLEX_CH2_PAN_1,0xFB25
.set FLEX_CH2_PAN_2,0xFB26
.set FLEX_CH2_PAN_3,0xFB27

; Flex Channel 3 (0xFB40-0xFB7F)
.set FLEX_CH3_FREQ_0,0xFB40
.set FLEX_CH3_FREQ_1,0xFB41
.set FLEX_CH3_FREQ_2,0xFB42
.set FLEX_CH3_FREQ_3,0xFB43
.set FLEX_CH3_VOL_0,0xFB44
.set FLEX_CH3_VOL_1,0xFB45
.set FLEX_CH3_VOL_2,0xFB46
.set FLEX_CH3_VOL_3,0xFB47
.set FLEX_CH3_WAVE_0,0xFB48
.set FLEX_CH3_WAVE_1,0xFB49
.set FLEX_CH3_WAVE_2,0xFB4A
.set FLEX_CH3_WAVE_3,0xFB4B
.set FLEX_CH3_CTRL_0,0xFB4C
.set FLEX_CH3_CTRL_1,0xFB4D
.set FLEX_CH3_CTRL_2,0xFB4E
.set FLEX_CH3_CTRL_3,0xFB4F
.set FLEX_CH3_ATK_0,0xFB50
.set FLEX_CH3_ATK_1,0xFB51
.set FLEX_CH3_DEC_0,0xFB54
.set FLEX_CH3_DEC_1,0xFB55
.set FLEX_CH3_SUS_0,0xFB58
.set FLEX_CH3_SUS_1,0xFB59
.set FLEX_CH3_REL_0,0xFB5C
.set FLEX_CH3_REL_1,0xFB5D
.set FLEX_CH3_DUTY_0,0xFB60
.set FLEX_CH3_DUTY_1,0xFB61
.set FLEX_CH3_PAN_0,0xFB64
.set FLEX_CH3_PAN_1,0xFB65
.set FLEX_CH3_PAN_2,0xFB66
.set FLEX_CH3_PAN_3,0xFB67

; ----------------------------------------------------------------------------
; Waveform Types (for flexible channels)
; ----------------------------------------------------------------------------
.set WAVE_SQUARE,0
.set WAVE_TRIANGLE,1
.set WAVE_SINE,2
.set WAVE_NOISE,3
.set WAVE_SAWTOOTH,4

; ----------------------------------------------------------------------------
; Channel Control Bits
; ----------------------------------------------------------------------------
.set CTRL_GATE,0x01
.set CTRL_PWM_EN,0x02
.set CTRL_SWEEP_EN,0x04
.set CTRL_SYNC_EN,0x08
.set CTRL_RING_EN,0x10
.set CTRL_FILTER_EN,0x20

; Sweep direction values
.set SWEEP_DOWN,0
.set SWEEP_UP,1

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

; Copper WAIT: usage is y * COP_WAIT_SCALE
.set COP_WAIT_MASK,0x00000000
.set COP_WAIT_SCALE,0x1000
.set COP_MOVE_RASTER_Y,0x40120000
.set COP_MOVE_RASTER_H,0x40130000
.set COP_MOVE_RASTER_COLOR,0x40140000
.set COP_MOVE_RASTER_CTRL,0x40150000
.set COP_END,0xC0000000

; Copper SETBASE opcode - set I/O base for subsequent MOVE operations
; Format: [10][6-bit unused][24-bit base >> 2]
; Addresses are 4-byte aligned, so we encode (addr >> 2) in 24 bits
.set COP_SETBASE_OP,0x80000000
.set COP_SETBASE_SHIFT,2

; Pre-computed SETBASE instructions for common devices
.set COP_SETBASE_VIDEO,0x8003C000       ; VIDEO_REG_BASE (0xF0000 >> 2)
.set COP_SETBASE_VGA,0x8003C400         ; VGA_BASE (0xF1000 >> 2)
.set COP_SETBASE_VGA_DAC,0x8003C416     ; VGA_DAC_WINDEX (0xF1058 >> 2)

; MOVE register indices after SETBASE to VGA_DAC_WINDEX
.set COP_MOVE_VGA_WINDEX,0x40000000     ; Offset 0: DAC write index
.set COP_MOVE_VGA_DATA,0x40010000       ; Offset 1: DAC data (R,G,B seq)

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

; Set SAP play pointer (32-bit)
.macro SET_SAP_PTR addr
    ld a,(\addr)&0xFF
    ld (SAP_PLAY_PTR_0),a
    ld a,((\addr)>>8)&0xFF
    ld (SAP_PLAY_PTR_1),a
    ld a,((\addr)>>16)&0xFF
    ld (SAP_PLAY_PTR_2),a
    ld a,((\addr)>>24)&0xFF
    ld (SAP_PLAY_PTR_3),a
.endm

; Set SAP play length (32-bit)
.macro SET_SAP_LEN len
    ld a,(\len)&0xFF
    ld (SAP_PLAY_LEN_0),a
    ld a,((\len)>>8)&0xFF
    ld (SAP_PLAY_LEN_1),a
    ld a,((\len)>>16)&0xFF
    ld (SAP_PLAY_LEN_2),a
    ld a,((\len)>>24)&0xFF
    ld (SAP_PLAY_LEN_3),a
.endm

; Set SID play pointer (32-bit)
.macro SET_SID_PTR addr
    ld a,(\addr)&0xFF
    ld (SID_PLAY_PTR_0),a
    ld a,((\addr)>>8)&0xFF
    ld (SID_PLAY_PTR_1),a
    ld a,((\addr)>>16)&0xFF
    ld (SID_PLAY_PTR_2),a
    ld a,((\addr)>>24)&0xFF
    ld (SID_PLAY_PTR_3),a
.endm

; Set SID play length (32-bit)
.macro SET_SID_LEN len
    ld a,(\len)&0xFF
    ld (SID_PLAY_LEN_0),a
    ld a,((\len)>>8)&0xFF
    ld (SID_PLAY_LEN_1),a
    ld a,((\len)>>16)&0xFF
    ld (SID_PLAY_LEN_2),a
    ld a,((\len)>>24)&0xFF
    ld (SID_PLAY_LEN_3),a
.endm

; Set SID subsong
.macro SET_SID_SUBSONG num
    ld a,\num
    ld (SID_SUBSONG),a
.endm

; Start SID playback
.macro START_SID_PLAY
    ld a,1
    ld (SID_PLAY_CTRL),a
.endm

; Start SID playback with looping
.macro START_SID_LOOP
    ld a,5
    ld (SID_PLAY_CTRL),a
.endm

; Stop SID playback
.macro STOP_SID_PLAY
    ld a,2
    ld (SID_PLAY_CTRL),a
.endm

; Write SID register via port I/O (reg in A, value in B)
.macro SID_WRITE reg,val
    ld a,\reg
    out (SID_PORT_SELECT),a
    ld a,\val
    out (SID_PORT_DATA),a
.endm

; ============================================================================
; TED AUDIO MACROS
; ============================================================================

; Set TED play pointer (32-bit)
.macro SET_TED_PTR addr
    ld a,(\addr)&0xFF
    ld (TED_PLAY_PTR_0),a
    ld a,((\addr)>>8)&0xFF
    ld (TED_PLAY_PTR_1),a
    ld a,((\addr)>>16)&0xFF
    ld (TED_PLAY_PTR_2),a
    ld a,((\addr)>>24)&0xFF
    ld (TED_PLAY_PTR_3),a
.endm

; Set TED play length (32-bit)
.macro SET_TED_LEN len
    ld a,(\len)&0xFF
    ld (TED_PLAY_LEN_0),a
    ld a,((\len)>>8)&0xFF
    ld (TED_PLAY_LEN_1),a
    ld a,((\len)>>16)&0xFF
    ld (TED_PLAY_LEN_2),a
    ld a,((\len)>>24)&0xFF
    ld (TED_PLAY_LEN_3),a
.endm

; Start TED playback
.macro START_TED_PLAY
    ld a,1
    ld (TED_PLAY_CTRL),a
.endm

; Start TED playback with looping
.macro START_TED_LOOP
    ld a,5
    ld (TED_PLAY_CTRL),a
.endm

; Stop TED playback
.macro STOP_TED_PLAY
    ld a,2
    ld (TED_PLAY_CTRL),a
.endm

; Write TED register via port I/O
.macro TED_WRITE reg,val
    ld a,\reg
    out (TED_PORT_SELECT),a
    ld a,\val
    out (TED_PORT_DATA),a
.endm

; ============================================================================
; CUSTOM AUDIO CHIP MACROS
; ============================================================================

; Set square channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_SQ_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    xor a
    ld (SQ_FREQ_0),a
    ld a,(\freq)&0xFF
    ld (SQ_FREQ_1),a
    ld a,((\freq)>>8)&0xFF
    ld (SQ_FREQ_2),a
    xor a
    ld (SQ_FREQ_3),a
.endm

; Set square channel volume (8-bit, 0-255)
.macro SET_SQ_VOL vol
    ld a,\vol
    ld (SQ_VOL_0),a
    xor a
    ld (SQ_VOL_1),a
    ld (SQ_VOL_2),a
    ld (SQ_VOL_3),a
.endm

; Set square channel duty cycle (16-bit, 0-65535; 32768 = 50%)
.macro SET_SQ_DUTY duty
    ld a,(\duty)&0xFF
    ld (SQ_DUTY_0),a
    ld a,((\duty)>>8)&0xFF
    ld (SQ_DUTY_1),a
.endm

; Set square channel ADSR (all 16-bit ms values)
.macro SET_SQ_ADSR atk,dec,sus,rel
    ld a,(\atk)&0xFF
    ld (SQ_ATTACK_0),a
    ld a,((\atk)>>8)&0xFF
    ld (SQ_ATTACK_1),a
    ld a,(\dec)&0xFF
    ld (SQ_DECAY_0),a
    ld a,((\dec)>>8)&0xFF
    ld (SQ_DECAY_1),a
    ld a,(\sus)&0xFF
    ld (SQ_SUSTAIN_0),a
    ld a,((\sus)>>8)&0xFF
    ld (SQ_SUSTAIN_1),a
    ld a,(\rel)&0xFF
    ld (SQ_RELEASE_0),a
    ld a,((\rel)>>8)&0xFF
    ld (SQ_RELEASE_1),a
.endm

; Gate square channel on (trigger envelope)
.macro GATE_SQ_ON
    ld a,(SQ_CTRL_0)
    or CTRL_GATE
    ld (SQ_CTRL_0),a
.endm

; Gate square channel off (release envelope)
.macro GATE_SQ_OFF
    ld a,(SQ_CTRL_0)
    and ~CTRL_GATE
    ld (SQ_CTRL_0),a
.endm

; Set triangle channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_TRI_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    xor a
    ld (TRI_FREQ_0),a
    ld a,(\freq)&0xFF
    ld (TRI_FREQ_1),a
    ld a,((\freq)>>8)&0xFF
    ld (TRI_FREQ_2),a
    xor a
    ld (TRI_FREQ_3),a
.endm

; Set triangle channel volume
.macro SET_TRI_VOL vol
    ld a,\vol
    ld (TRI_VOL_0),a
    xor a
    ld (TRI_VOL_1),a
    ld (TRI_VOL_2),a
    ld (TRI_VOL_3),a
.endm

; Set triangle channel ADSR
.macro SET_TRI_ADSR atk,dec,sus,rel
    ld a,(\atk)&0xFF
    ld (TRI_ATTACK_0),a
    ld a,((\atk)>>8)&0xFF
    ld (TRI_ATTACK_1),a
    ld a,(\dec)&0xFF
    ld (TRI_DECAY_0),a
    ld a,((\dec)>>8)&0xFF
    ld (TRI_DECAY_1),a
    ld a,(\sus)&0xFF
    ld (TRI_SUSTAIN_0),a
    ld a,((\sus)>>8)&0xFF
    ld (TRI_SUSTAIN_1),a
    ld a,(\rel)&0xFF
    ld (TRI_RELEASE_0),a
    ld a,((\rel)>>8)&0xFF
    ld (TRI_RELEASE_1),a
.endm

; Gate triangle channel on
.macro GATE_TRI_ON
    ld a,(TRI_CTRL_0)
    or CTRL_GATE
    ld (TRI_CTRL_0),a
.endm

; Gate triangle channel off
.macro GATE_TRI_OFF
    ld a,(TRI_CTRL_0)
    and ~CTRL_GATE
    ld (TRI_CTRL_0),a
.endm

; Set sine channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_SINE_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    xor a
    ld (SINE_FREQ_0),a
    ld a,(\freq)&0xFF
    ld (SINE_FREQ_1),a
    ld a,((\freq)>>8)&0xFF
    ld (SINE_FREQ_2),a
    xor a
    ld (SINE_FREQ_3),a
.endm

; Set sine channel volume
.macro SET_SINE_VOL vol
    ld a,\vol
    ld (SINE_VOL_0),a
    xor a
    ld (SINE_VOL_1),a
    ld (SINE_VOL_2),a
    ld (SINE_VOL_3),a
.endm

; Set sine channel ADSR
.macro SET_SINE_ADSR atk,dec,sus,rel
    ld a,(\atk)&0xFF
    ld (SINE_ATTACK_0),a
    ld a,((\atk)>>8)&0xFF
    ld (SINE_ATTACK_1),a
    ld a,(\dec)&0xFF
    ld (SINE_DECAY_0),a
    ld a,((\dec)>>8)&0xFF
    ld (SINE_DECAY_1),a
    ld a,(\sus)&0xFF
    ld (SINE_SUSTAIN_0),a
    ld a,((\sus)>>8)&0xFF
    ld (SINE_SUSTAIN_1),a
    ld a,(\rel)&0xFF
    ld (SINE_RELEASE_0),a
    ld a,((\rel)>>8)&0xFF
    ld (SINE_RELEASE_1),a
.endm

; Gate sine channel on
.macro GATE_SINE_ON
    ld a,(SINE_CTRL_0)
    or CTRL_GATE
    ld (SINE_CTRL_0),a
.endm

; Gate sine channel off
.macro GATE_SINE_OFF
    ld a,(SINE_CTRL_0)
    and ~CTRL_GATE
    ld (SINE_CTRL_0),a
.endm

; Set noise channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_NOISE_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    xor a
    ld (NOISE_FREQ_0),a
    ld a,(\freq)&0xFF
    ld (NOISE_FREQ_1),a
    ld a,((\freq)>>8)&0xFF
    ld (NOISE_FREQ_2),a
    xor a
    ld (NOISE_FREQ_3),a
.endm

; Set noise channel volume
.macro SET_NOISE_VOL vol
    ld a,\vol
    ld (NOISE_VOL_0),a
    xor a
    ld (NOISE_VOL_1),a
    ld (NOISE_VOL_2),a
    ld (NOISE_VOL_3),a
.endm

; Set noise mode
.macro SET_NOISE_MODE mode
    ld a,\mode
    ld (NOISE_MODE_0),a
    xor a
    ld (NOISE_MODE_1),a
    ld (NOISE_MODE_2),a
    ld (NOISE_MODE_3),a
.endm

; Set noise channel ADSR
.macro SET_NOISE_ADSR atk,dec,sus,rel
    ld a,(\atk)&0xFF
    ld (NOISE_ATTACK_0),a
    ld a,((\atk)>>8)&0xFF
    ld (NOISE_ATTACK_1),a
    ld a,(\dec)&0xFF
    ld (NOISE_DECAY_0),a
    ld a,((\dec)>>8)&0xFF
    ld (NOISE_DECAY_1),a
    ld a,(\sus)&0xFF
    ld (NOISE_SUSTAIN_0),a
    ld a,((\sus)>>8)&0xFF
    ld (NOISE_SUSTAIN_1),a
    ld a,(\rel)&0xFF
    ld (NOISE_RELEASE_0),a
    ld a,((\rel)>>8)&0xFF
    ld (NOISE_RELEASE_1),a
.endm

; Gate noise channel on
.macro GATE_NOISE_ON
    ld a,(NOISE_CTRL_0)
    or CTRL_GATE
    ld (NOISE_CTRL_0),a
.endm

; Gate noise channel off
.macro GATE_NOISE_OFF
    ld a,(NOISE_CTRL_0)
    and ~CTRL_GATE
    ld (NOISE_CTRL_0),a
.endm

; Set sawtooth channel frequency (16.8 fixed-point Hz, macro handles *256)
.macro SET_SAW_FREQ freq
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    xor a
    ld (SAW_FREQ_0),a
    ld a,(\freq)&0xFF
    ld (SAW_FREQ_1),a
    ld a,((\freq)>>8)&0xFF
    ld (SAW_FREQ_2),a
    xor a
    ld (SAW_FREQ_3),a
.endm

; Set sawtooth channel volume
.macro SET_SAW_VOL vol
    ld a,\vol
    ld (SAW_VOL_0),a
    xor a
    ld (SAW_VOL_1),a
    ld (SAW_VOL_2),a
    ld (SAW_VOL_3),a
.endm

; Set sawtooth channel ADSR
.macro SET_SAW_ADSR atk,dec,sus,rel
    ld a,(\atk)&0xFF
    ld (SAW_ATTACK_0),a
    ld a,((\atk)>>8)&0xFF
    ld (SAW_ATTACK_1),a
    ld a,(\dec)&0xFF
    ld (SAW_DECAY_0),a
    ld a,((\dec)>>8)&0xFF
    ld (SAW_DECAY_1),a
    ld a,(\sus)&0xFF
    ld (SAW_SUSTAIN_0),a
    ld a,((\sus)>>8)&0xFF
    ld (SAW_SUSTAIN_1),a
    ld a,(\rel)&0xFF
    ld (SAW_RELEASE_0),a
    ld a,((\rel)>>8)&0xFF
    ld (SAW_RELEASE_1),a
.endm

; Gate sawtooth channel on
.macro GATE_SAW_ON
    ld a,(SAW_CTRL_0)
    or CTRL_GATE
    ld (SAW_CTRL_0),a
.endm

; Gate sawtooth channel off
.macro GATE_SAW_OFF
    ld a,(SAW_CTRL_0)
    and ~CTRL_GATE
    ld (SAW_CTRL_0),a
.endm

; Set global filter parameters
; type: FILT_OFF, FILT_LOWPASS, FILT_HIGHPASS, FILT_BANDPASS
; cutoff: 0-65535 (maps to 20Hz-20kHz)
; resonance: 0-255
.macro SET_FILTER type,cutoff,resonance
    ld a,\type
    ld (FILTER_TYPE_0),a
    xor a
    ld (FILTER_TYPE_1),a
    ld (FILTER_TYPE_2),a
    ld (FILTER_TYPE_3),a
    ld a,(\cutoff)&0xFF
    ld (FILTER_CUTOFF_0),a
    ld a,((\cutoff)>>8)&0xFF
    ld (FILTER_CUTOFF_1),a
    xor a
    ld (FILTER_CUTOFF_2),a
    ld (FILTER_CUTOFF_3),a
    ld a,\resonance
    ld (FILTER_RESONANCE_0),a
    xor a
    ld (FILTER_RESONANCE_1),a
    ld (FILTER_RESONANCE_2),a
    ld (FILTER_RESONANCE_3),a
.endm

; Enable filter on square channel
.macro FILTER_SQ_ON
    ld a,(SQ_CTRL_0)
    or CTRL_FILTER_EN
    ld (SQ_CTRL_0),a
.endm

; Disable filter on square channel
.macro FILTER_SQ_OFF
    ld a,(SQ_CTRL_0)
    and ~CTRL_FILTER_EN
    ld (SQ_CTRL_0),a
.endm

; Set reverb parameters
; mix: 0-255 (wet/dry ratio)
; decay: 0-65535 (decay time in ms)
.macro SET_REVERB mix,decay
    ld a,\mix
    ld (REVERB_MIX_0),a
    xor a
    ld (REVERB_MIX_1),a
    ld (REVERB_MIX_2),a
    ld (REVERB_MIX_3),a
    ld a,(\decay)&0xFF
    ld (REVERB_DECAY_0),a
    ld a,((\decay)>>8)&0xFF
    ld (REVERB_DECAY_1),a
    xor a
    ld (REVERB_DECAY_2),a
    ld (REVERB_DECAY_3),a
.endm

; Set overdrive amount (0=off, 1-255=distortion amount)
.macro SET_OVERDRIVE amount
    ld a,\amount
    ld (OVERDRIVE_CTRL_0),a
    xor a
    ld (OVERDRIVE_CTRL_1),a
    ld (OVERDRIVE_CTRL_2),a
    ld (OVERDRIVE_CTRL_3),a
.endm

; Set flexible channel 0 parameters
; Frequency is 16.8 fixed-point (Hz * 256), macro handles conversion
.macro SET_FLEX0 wave,freq,vol
    ld a,\wave
    ld (FLEX_CH0_WAVE_0),a
    xor a
    ld (FLEX_CH0_WAVE_1),a
    ld (FLEX_CH0_WAVE_2),a
    ld (FLEX_CH0_WAVE_3),a
    ; freq * 256: byte 0 = 0, bytes 1-2 = freq, byte 3 = 0
    xor a
    ld (FLEX_CH0_FREQ_0),a
    ld a,(\freq)&0xFF
    ld (FLEX_CH0_FREQ_1),a
    ld a,((\freq)>>8)&0xFF
    ld (FLEX_CH0_FREQ_2),a
    xor a
    ld (FLEX_CH0_FREQ_3),a
    ld a,\vol
    ld (FLEX_CH0_VOL_0),a
    xor a
    ld (FLEX_CH0_VOL_1),a
    ld (FLEX_CH0_VOL_2),a
    ld (FLEX_CH0_VOL_3),a
.endm

; Gate flexible channel 0 on
.macro GATE_FLEX0_ON
    ld a,(FLEX_CH0_CTRL_0)
    or CTRL_GATE
    ld (FLEX_CH0_CTRL_0),a
.endm

; Gate flexible channel 0 off
.macro GATE_FLEX0_OFF
    ld a,(FLEX_CH0_CTRL_0)
    and ~CTRL_GATE
    ld (FLEX_CH0_CTRL_0),a
.endm

; Set flexible channel 0 ADSR
.macro SET_FLEX0_ADSR atk,dec,sus,rel
    ld a,(\atk)&0xFF
    ld (FLEX_CH0_ATK_0),a
    ld a,((\atk)>>8)&0xFF
    ld (FLEX_CH0_ATK_1),a
    ld a,(\dec)&0xFF
    ld (FLEX_CH0_DEC_0),a
    ld a,((\dec)>>8)&0xFF
    ld (FLEX_CH0_DEC_1),a
    ld a,(\sus)&0xFF
    ld (FLEX_CH0_SUS_0),a
    ld a,((\sus)>>8)&0xFF
    ld (FLEX_CH0_SUS_1),a
    ld a,(\rel)&0xFF
    ld (FLEX_CH0_REL_0),a
    ld a,((\rel)>>8)&0xFF
    ld (FLEX_CH0_REL_1),a
.endm

; Set PWM parameters for square channel
; rate: PWM oscillation rate
; depth: PWM depth (0-65535)
.macro SET_SQ_PWM rate,depth
    ld a,(\rate)&0xFF
    ld (SQ_PWM_RATE_0),a
    ld a,((\rate)>>8)&0xFF
    ld (SQ_PWM_RATE_1),a
    xor a
    ld (SQ_PWM_RATE_2),a
    ld (SQ_PWM_RATE_3),a
    ld a,(\depth)&0xFF
    ld (SQ_PWM_DEPTH_0),a
    ld a,((\depth)>>8)&0xFF
    ld (SQ_PWM_DEPTH_1),a
    xor a
    ld (SQ_PWM_DEPTH_2),a
    ld (SQ_PWM_DEPTH_3),a
    ; Enable PWM
    ld a,(SQ_CTRL_0)
    or CTRL_PWM_EN
    ld (SQ_CTRL_0),a
.endm

; Enable hard sync on square channel
.macro SET_SQ_SYNC src
    ld a,\src
    ld (SYNC_SRC_CH0_0),a
    xor a
    ld (SYNC_SRC_CH0_1),a
    ld (SYNC_SRC_CH0_2),a
    ld (SYNC_SRC_CH0_3),a
    ; Enable sync
    ld a,(SQ_CTRL_0)
    or CTRL_SYNC_EN
    ld (SQ_CTRL_0),a
.endm

; Enable ring modulation on square channel
.macro SET_SQ_RING src
    ld a,\src
    ld (RING_SRC_CH0_0),a
    xor a
    ld (RING_SRC_CH0_1),a
    ld (RING_SRC_CH0_2),a
    ld (RING_SRC_CH0_3),a
    ; Enable ring mod
    ld a,(SQ_CTRL_0)
    or CTRL_RING_EN
    ld (SQ_CTRL_0),a
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
; ZX SPECTRUM ULA VIDEO REGISTERS
;
; The ULA chip provides ZX Spectrum compatible video output:
; - 256x192 pixel display with 32-pixel border (320x256 total)
; - Attribute-based coloring: 8x8 pixel cells share INK/PAPER colors
; - 15 unique colors (8 base + 8 bright, black can't brighten)
; - FLASH attribute: swaps INK/PAPER at ~1.6Hz
;
; For Z80: ULA is accessed via port I/O at port 0xFE (authentic Spectrum)
; Writing to port 0xFE: bits 0-2 = border color, bit 3 = MIC, bit 4 = EAR
; VRAM is at 0x4000 (same as authentic ZX Spectrum)
;
; Non-linear bitmap addressing formula:
;   addr = ((y & 0xC0) << 5) + ((y & 0x07) << 8) + ((y & 0x38) << 2) + (x >> 3)
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

; ULA port (authentic ZX Spectrum)
.set ULA_PORT,0xFE              ; ULA I/O port (border, speaker, etc.)

; VRAM layout (same as authentic ZX Spectrum)
.set ULA_VRAM,0x4000            ; VRAM base address
.set ULA_BITMAP_SIZE,6144       ; Bitmap: 256x192/8 = 6144 bytes
.set ULA_ATTR_OFFSET,0x1800     ; Attribute offset from VRAM base
.set ULA_ATTR_SIZE,768          ; Attributes: 32x24 = 768 bytes
.set ULA_VRAM_SIZE,6912         ; Total: 6144 + 768 bytes
.set ULA_ATTR_BASE,0x5800       ; Attribute start address (0x4000 + 0x1800)

; Display dimensions
.set ULA_DISPLAY_W,256          ; Display width in pixels
.set ULA_DISPLAY_H,192          ; Display height in pixels
.set ULA_BORDER_SIZE,32         ; Border size in pixels
.set ULA_FRAME_W,320            ; Total frame width (256+64)
.set ULA_FRAME_H,256            ; Total frame height (192+64)
.set ULA_CELLS_X,32             ; Character cells horizontally
.set ULA_CELLS_Y,24             ; Character cells vertically

; ----- ULA helper macros -----

; Set ULA border color (0-7, value in bits 0-2)
.macro ULA_SET_BORDER color
    ld a,\color
    out (ULA_PORT),a
.endm

; Set border color from A register (bits 0-2)
.macro ULA_BORDER_A
    out (ULA_PORT),a
.endm

; ============================================================================
; STACK SETUP
; ============================================================================

.set STACK_TOP,0xF000

; ============================================================================
; EOF
; ============================================================================
