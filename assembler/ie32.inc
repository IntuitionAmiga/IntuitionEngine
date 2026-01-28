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
.equ SID_BASE          0xF0E00          ; SID base address

; Voice 1 registers
.equ SID_V1_FREQ_LO    0xF0E00          ; Voice 1 frequency low byte
.equ SID_V1_FREQ_HI    0xF0E01          ; Voice 1 frequency high byte
.equ SID_V1_PW_LO      0xF0E02          ; Voice 1 pulse width low byte
.equ SID_V1_PW_HI      0xF0E03          ; Voice 1 pulse width high (bits 0-3)
.equ SID_V1_CTRL       0xF0E04          ; Voice 1 control register
.equ SID_V1_AD         0xF0E05          ; Voice 1 attack/decay
.equ SID_V1_SR         0xF0E06          ; Voice 1 sustain/release

; Voice 2 registers
.equ SID_V2_FREQ_LO    0xF0E07          ; Voice 2 frequency low byte
.equ SID_V2_FREQ_HI    0xF0E08          ; Voice 2 frequency high byte
.equ SID_V2_PW_LO      0xF0E09          ; Voice 2 pulse width low byte
.equ SID_V2_PW_HI      0xF0E0A          ; Voice 2 pulse width high
.equ SID_V2_CTRL       0xF0E0B          ; Voice 2 control register
.equ SID_V2_AD         0xF0E0C          ; Voice 2 attack/decay
.equ SID_V2_SR         0xF0E0D          ; Voice 2 sustain/release

; Voice 3 registers
.equ SID_V3_FREQ_LO    0xF0E0E          ; Voice 3 frequency low byte
.equ SID_V3_FREQ_HI    0xF0E0F          ; Voice 3 frequency high byte
.equ SID_V3_PW_LO      0xF0E10          ; Voice 3 pulse width low byte
.equ SID_V3_PW_HI      0xF0E11          ; Voice 3 pulse width high
.equ SID_V3_CTRL       0xF0E12          ; Voice 3 control register
.equ SID_V3_AD         0xF0E13          ; Voice 3 attack/decay
.equ SID_V3_SR         0xF0E14          ; Voice 3 sustain/release

; Filter and volume registers
.equ SID_FC_LO         0xF0E15          ; Filter cutoff low (bits 0-2)
.equ SID_FC_HI         0xF0E16          ; Filter cutoff high byte
.equ SID_RES_FILT      0xF0E17          ; Resonance (bits 4-7) + routing (bits 0-3)
.equ SID_MODE_VOL      0xF0E18          ; Volume (bits 0-3) + filter mode (bits 4-7)
.equ SID_PLUS_CTRL     0xF0E19          ; SID+ mode (0=standard, 1=enhanced)

; Voice control register bits (SID_Vx_CTRL)
.equ SID_CTRL_GATE     0x01             ; Gate bit (trigger envelope)
.equ SID_CTRL_SYNC     0x02             ; Sync with previous voice
.equ SID_CTRL_RINGMOD  0x04             ; Ring modulation
.equ SID_CTRL_TEST     0x08             ; Test bit (resets oscillator)
.equ SID_CTRL_TRIANGLE 0x10             ; Triangle waveform
.equ SID_CTRL_SAWTOOTH 0x20             ; Sawtooth waveform
.equ SID_CTRL_PULSE    0x40             ; Pulse/square waveform
.equ SID_CTRL_NOISE    0x80             ; Noise waveform

; Filter routing bits (SID_RES_FILT bits 0-3)
.equ SID_FILT_V1       0x01             ; Route voice 1 through filter
.equ SID_FILT_V2       0x02             ; Route voice 2 through filter
.equ SID_FILT_V3       0x04             ; Route voice 3 through filter
.equ SID_FILT_EXT      0x08             ; Route external input through filter

; Filter mode bits (SID_MODE_VOL bits 4-7)
.equ SID_MODE_LP       0x10             ; Low-pass filter
.equ SID_MODE_BP       0x20             ; Band-pass filter
.equ SID_MODE_HP       0x40             ; High-pass filter
.equ SID_MODE_3OFF     0x80             ; Disconnect voice 3 from output

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
;   bit 0 (0x01) - Start playback (write 1 to begin)
;   bit 1 (0x02) - Stop playback (write 1 to stop)
;   bit 2 (0x04) - Enable looping (set before starting)
;
; SID_PLAY_STATUS bits:
;   bit 0 (0x01) - Busy (1 = playing, 0 = stopped)
;   bit 1 (0x02) - Error (1 = load/parse error)
; ----------------------------------------------------------------------------
.equ SID_PLAY_PTR      0xF0E20          ; Play pointer (32-bit)
.equ SID_PLAY_LEN      0xF0E24          ; Play length (32-bit)
.equ SID_PLAY_CTRL     0xF0E28          ; Play control
.equ SID_PLAY_STATUS   0xF0E2C          ; Play status
.equ SID_SUBSONG       0xF0E2D          ; Subsong selection (0-255)

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
.equ TED_BASE          0xF0F00          ; TED base address
.equ TED_FREQ1_LO      0xF0F00          ; Voice 1 frequency low byte
.equ TED_FREQ2_LO      0xF0F01          ; Voice 2 frequency low byte
.equ TED_FREQ2_HI      0xF0F02          ; Voice 2 frequency high (bits 0-1)
.equ TED_SND_CTRL      0xF0F03          ; Sound control register
.equ TED_FREQ1_HI      0xF0F04          ; Voice 1 frequency high (bits 0-1)
.equ TED_PLUS_CTRL     0xF0F05          ; TED+ mode (0=standard, 1=enhanced)

; TED Control register bits (TED_SND_CTRL)
.equ TED_CTRL_SNDDC     0x80            ; D/A mode (bit 7)
.equ TED_CTRL_SND2NOISE 0x40            ; Voice 2 noise enable (bit 6)
.equ TED_CTRL_SND2ON    0x20            ; Voice 2 enable (bit 5)
.equ TED_CTRL_SND1ON    0x10            ; Voice 1 enable (bit 4)
.equ TED_CTRL_VOLUME    0x0F            ; Volume mask (bits 0-3, max=8)

; ----------------------------------------------------------------------------
; TED Player Registers
;
; The TED player handles Commodore Plus/4 music files (.ted) which contain
; embedded 6502 code that drives the TED sound chip.
;
; Usage:
;   1. Embed TED data in your program using .incbin
;   2. Set TED_PLAY_PTR to the address of the TED file data
;   3. Set TED_PLAY_LEN to the size of the TED data in bytes
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
.equ TED_PLAY_PTR      0xF0F10          ; Play pointer (32-bit)
.equ TED_PLAY_LEN      0xF0F14          ; Play length (32-bit)
.equ TED_PLAY_CTRL     0xF0F18          ; Play control
.equ TED_PLAY_STATUS   0xF0F1C          ; Play status

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
.equ AHX_BASE          0xF0B80          ; AHX base address
.equ AHX_PLUS_CTRL     0xF0B80          ; AHX+ mode (0=standard, 1=enhanced)

; ----------------------------------------------------------------------------
; AHX Player Registers
;
; The AHX player handles Amiga AHX/THX modules which contain instrument
; definitions and pattern data for 4-channel waveform synthesis.
;
; Usage:
;   1. Embed AHX data in your program using .incbin
;   2. Set AHX_PLAY_PTR to the address of the AHX file data
;   3. Set AHX_PLAY_LEN to the size of the AHX data in bytes
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
.equ AHX_PLAY_PTR      0xF0B84          ; Play pointer (32-bit)
.equ AHX_PLAY_LEN      0xF0B88          ; Play length (32-bit)
.equ AHX_PLAY_CTRL     0xF0B8C          ; Play control
.equ AHX_PLAY_STATUS   0xF0B90          ; Play status
.equ AHX_SUBSONG       0xF0B91          ; Subsong selection (0-255)

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

.equ VGA_BASE          0xF1000
.equ VGA_MODE          0xF1000          ; Mode control
.equ VGA_STATUS        0xF1004          ; Status register
.equ VGA_CTRL          0xF1008          ; Control register

; Sequencer (0x3C4/0x3C5 equivalent)
.equ VGA_SEQ_INDEX     0xF1010          ; Sequencer index
.equ VGA_SEQ_DATA      0xF1014          ; Sequencer data
.equ VGA_SEQ_MAPMASK   0xF1018          ; Plane write mask (direct)

; CRTC (0x3D4/0x3D5 equivalent)
.equ VGA_CRTC_INDEX    0xF1020          ; CRTC index
.equ VGA_CRTC_DATA     0xF1024          ; CRTC data
.equ VGA_CRTC_STARTHI  0xF1028          ; Start address high
.equ VGA_CRTC_STARTLO  0xF102C          ; Start address low

; Graphics Controller (0x3CE/0x3CF equivalent)
.equ VGA_GC_INDEX      0xF1030          ; GC index
.equ VGA_GC_DATA       0xF1034          ; GC data
.equ VGA_GC_READMAP    0xF1038          ; Read plane select (direct)
.equ VGA_GC_BITMASK    0xF103C          ; Bit mask (direct)

; Attribute Controller
.equ VGA_ATTR_INDEX    0xF1040          ; Attribute index
.equ VGA_ATTR_DATA     0xF1044          ; Attribute data

; DAC/Palette (0x3C6-0x3C9 equivalent)
.equ VGA_DAC_MASK      0xF1050          ; Pixel mask
.equ VGA_DAC_RINDEX    0xF1054          ; Read index
.equ VGA_DAC_WINDEX    0xF1058          ; Write index
.equ VGA_DAC_DATA      0xF105C          ; DAC data (R,G,B sequence)

; Palette RAM (256 x 3 bytes)
.equ VGA_PALETTE       0xF1100          ; Palette base (768 bytes)

; VGA VRAM (64KB window)
.equ VGA_VRAM          0xA0000          ; VRAM window (64KB)

; Mode constants
.equ VGA_MODE_TEXT     0x03             ; 80x25 text mode
.equ VGA_MODE_12H      0x12             ; 640x480, 16 colors
.equ VGA_MODE_13H      0x13             ; 320x200, 256 colors
.equ VGA_MODE_X        0x14             ; 320x240, 256 colors

; Status bits
.equ VGA_STATUS_VSYNC  0x01             ; VSync active
.equ VGA_STATUS_RETRACE 0x08            ; Vertical retrace

; Control bits
.equ VGA_CTRL_ENABLE   0x01             ; VGA enable

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
; Global Audio Control (0xF0800-0xF083F)
; ----------------------------------------------------------------------------
.equ AUDIO_CTRL        0xF0800          ; Master audio control
.equ ENV_SHAPE         0xF0804          ; Global envelope shape

; Filter registers
.equ FILTER_CUTOFF     0xF0820          ; Cutoff frequency (0-255, exponential 20Hz-20kHz)
.equ FILTER_RESONANCE  0xF0824          ; Resonance/Q (0-255)
.equ FILTER_TYPE       0xF0828          ; Filter type (0=off, 1=LP, 2=HP, 3=BP)
.equ FILTER_MOD_SOURCE 0xF082C          ; Modulation source channel (0-3)
.equ FILTER_MOD_AMOUNT 0xF0830          ; Modulation depth (0-255)

; Filter type constants
.equ FILTER_OFF        0                ; Filter bypassed
.equ FILTER_LOWPASS    1                ; Low-pass filter
.equ FILTER_HIGHPASS   2                ; High-pass filter
.equ FILTER_BANDPASS   3                ; Band-pass filter

; Envelope shape constants
.equ ENV_SHAPE_ADSR      0              ; Standard ADSR
.equ ENV_SHAPE_SAW_UP    1              ; Linear rise to max, then hold
.equ ENV_SHAPE_SAW_DOWN  2              ; Linear fall to zero, then hold
.equ ENV_SHAPE_LOOP      3              ; ADSR but loops after release
.equ ENV_SHAPE_SID       4              ; SID-style exponential ADSR

; ----------------------------------------------------------------------------
; Square Wave Channel (0xF0900-0xF093F)
; ----------------------------------------------------------------------------
.equ SQUARE_FREQ       0xF0900          ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
.equ SQUARE_VOL        0xF0904          ; Volume (0-255)
.equ SQUARE_CTRL       0xF0908          ; Control (bit 1 = gate)
.equ SQUARE_DUTY       0xF090C          ; Duty cycle (0-255, 128 = 50%)
.equ SQUARE_SWEEP      0xF0910          ; Sweep control
.equ SQUARE_PWM_CTRL   0xF0922          ; PWM control (bit 7 = enable, bits 0-6 = rate)
.equ SQUARE_ATK        0xF0930          ; Attack time (ms)
.equ SQUARE_DEC        0xF0934          ; Decay time (ms)
.equ SQUARE_SUS        0xF0938          ; Sustain level (0-255)
.equ SQUARE_REL        0xF093C          ; Release time (ms)

; ----------------------------------------------------------------------------
; Triangle Wave Channel (0xF0940-0xF097F)
; ----------------------------------------------------------------------------
.equ TRI_FREQ          0xF0940          ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
.equ TRI_VOL           0xF0944          ; Volume (0-255)
.equ TRI_CTRL          0xF0948          ; Control (bit 1 = gate)
.equ TRI_SWEEP         0xF0914          ; Sweep control
.equ TRI_ATK           0xF0960          ; Attack time (ms)
.equ TRI_DEC           0xF0964          ; Decay time (ms)
.equ TRI_SUS           0xF0968          ; Sustain level (0-255)
.equ TRI_REL           0xF096C          ; Release time (ms)

; ----------------------------------------------------------------------------
; Sine Wave Channel (0xF0980-0xF09BF)
; ----------------------------------------------------------------------------
.equ SINE_FREQ         0xF0980          ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
.equ SINE_VOL          0xF0984          ; Volume (0-255)
.equ SINE_CTRL         0xF0988          ; Control (bit 1 = gate)
.equ SINE_SWEEP        0xF0918          ; Sweep control
.equ SINE_ATK          0xF0990          ; Attack time (ms)
.equ SINE_DEC          0xF0994          ; Decay time (ms)
.equ SINE_SUS          0xF0998          ; Sustain level (0-255)
.equ SINE_REL          0xF099C          ; Release time (ms)

; ----------------------------------------------------------------------------
; Noise Channel (0xF09C0-0xF09FF)
; ----------------------------------------------------------------------------
.equ NOISE_FREQ        0xF09C0          ; Frequency/pitch (16.8 fixed-point Hz, value = Hz * 256)
.equ NOISE_VOL         0xF09C4          ; Volume (0-255)
.equ NOISE_CTRL        0xF09C8          ; Control (bit 1 = gate)
.equ NOISE_ATK         0xF09D0          ; Attack time (ms)
.equ NOISE_DEC         0xF09D4          ; Decay time (ms)
.equ NOISE_SUS         0xF09D8          ; Sustain level (0-255)
.equ NOISE_REL         0xF09DC          ; Release time (ms)
.equ NOISE_MODE        0xF09E0          ; Noise type

; Noise mode constants
.equ NOISE_MODE_WHITE    0              ; White noise (LFSR)
.equ NOISE_MODE_PERIODIC 1              ; Periodic/looping noise
.equ NOISE_MODE_METALLIC 2              ; Metallic noise
.equ NOISE_MODE_PSG      3              ; PSG-style (AY/YM) LFSR

; ----------------------------------------------------------------------------
; Modulation Registers (0xF0A00-0xF0A1F)
; ----------------------------------------------------------------------------
; Hard sync: slave channel resets phase when master completes a cycle
.equ SYNC_SOURCE_CH0   0xF0A00          ; Sync source for channel 0 (0-3, or 0xFF=off)
.equ SYNC_SOURCE_CH1   0xF0A04          ; Sync source for channel 1
.equ SYNC_SOURCE_CH2   0xF0A08          ; Sync source for channel 2
.equ SYNC_SOURCE_CH3   0xF0A0C          ; Sync source for channel 3

; Ring modulation: output = ch * source (creates harmonic sidebands)
.equ RING_MOD_SOURCE_CH0 0xF0A10        ; Ring mod source for channel 0
.equ RING_MOD_SOURCE_CH1 0xF0A14        ; Ring mod source for channel 1
.equ RING_MOD_SOURCE_CH2 0xF0A18        ; Ring mod source for channel 2
.equ RING_MOD_SOURCE_CH3 0xF0A1C        ; Ring mod source for channel 3

; ----------------------------------------------------------------------------
; Sawtooth Wave Channel (0xF0A20-0xF0A6F)
; ----------------------------------------------------------------------------
.equ SAW_FREQ          0xF0A20          ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
.equ SAW_VOL           0xF0A24          ; Volume (0-255)
.equ SAW_CTRL          0xF0A28          ; Control (bit 1 = gate)
.equ SAW_SWEEP         0xF0A2C          ; Sweep control
.equ SAW_ATK           0xF0A30          ; Attack time (ms)
.equ SAW_DEC           0xF0A34          ; Decay time (ms)
.equ SAW_SUS           0xF0A38          ; Sustain level (0-255)
.equ SAW_REL           0xF0A3C          ; Release time (ms)
.equ SYNC_SOURCE_CH4   0xF0A60          ; Sync source for sawtooth
.equ RING_MOD_SOURCE_CH4 0xF0A64        ; Ring mod source for sawtooth

; ----------------------------------------------------------------------------
; Effects Registers (0xF0A40-0xF0A5F)
; ----------------------------------------------------------------------------
.equ OVERDRIVE_CTRL    0xF0A40          ; Drive amount (0-255 -> 0.0-4.0x gain)
.equ REVERB_MIX        0xF0A50          ; Dry/wet mix (0-255, 0=dry, 255=wet)
.equ REVERB_DECAY      0xF0A54          ; Decay time (0-255 -> 0.1-0.99)

; ----------------------------------------------------------------------------
; Flexible 4-Channel Synth Block (0xF0A80-0xF0B7F)
;
; Modern unified interface for all 4 channels. Each channel is 0x40 bytes.
; Any channel can be any waveform type. This is the preferred interface.
; ----------------------------------------------------------------------------
.equ FLEX_CH_BASE      0xF0A80          ; Base address of flexible block
.equ FLEX_CH_STRIDE    0x40             ; Bytes per channel (64)

.equ FLEX_CH0_BASE     0xF0A80          ; Channel 0 base
.equ FLEX_CH1_BASE     0xF0AC0          ; Channel 1 base
.equ FLEX_CH2_BASE     0xF0B00          ; Channel 2 base
.equ FLEX_CH3_BASE     0xF0B40          ; Channel 3 base

; Register offsets within each flexible channel (add to FLEX_CHx_BASE)
.equ FLEX_OFF_FREQ       0x00           ; Frequency (16.8 fixed-point Hz, value = Hz * 256)
.equ FLEX_OFF_VOL        0x04           ; Volume (0-255)
.equ FLEX_OFF_CTRL       0x08           ; Control (bit 1 = gate)
.equ FLEX_OFF_DUTY       0x0C           ; Duty cycle for square wave
.equ FLEX_OFF_SWEEP      0x10           ; Sweep control
.equ FLEX_OFF_ATK        0x14           ; Attack time (ms)
.equ FLEX_OFF_DEC        0x18           ; Decay time (ms)
.equ FLEX_OFF_SUS        0x1C           ; Sustain level (0-255)
.equ FLEX_OFF_REL        0x20           ; Release time (ms)
.equ FLEX_OFF_WAVE_TYPE  0x24           ; Waveform type (0-4)
.equ FLEX_OFF_PWM_CTRL   0x28           ; PWM control
.equ FLEX_OFF_NOISEMODE  0x2C           ; Noise mode (if wave type is noise)

; Waveform type constants (for FLEX_OFF_WAVE_TYPE)
.equ WAVE_SQUARE       0                ; Square wave with PWM
.equ WAVE_TRIANGLE     1                ; Triangle wave
.equ WAVE_SINE         2                ; Sine wave
.equ WAVE_NOISE        3                ; Noise
.equ WAVE_SAWTOOTH     4                ; Sawtooth wave

; ----------------------------------------------------------------------------
; Control Register Bit Masks
; ----------------------------------------------------------------------------
.equ CTRL_GATE         0x02             ; Gate bit (trigger envelope)
.equ PWM_ENABLE        0x80             ; PWM enable (bit 7)
.equ PWM_RATE_MASK     0x7F             ; PWM rate (bits 0-6)
.equ SWEEP_ENABLE      0x80             ; Sweep enable (bit 7)
.equ SWEEP_DIR_UP      0x00             ; Sweep direction up
.equ SWEEP_DIR_DOWN    0x08             ; Sweep direction down (bit 3)
.equ SWEEP_PERIOD_MASK 0x70             ; Sweep period (bits 4-6)
.equ SWEEP_SHIFT_MASK  0x07             ; Sweep shift (bits 0-2)

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
; Usage: COP_WAIT(y) = y * COP_WAIT_SCALE
.equ COP_WAIT_MASK         0x00000000
.equ COP_WAIT_SCALE        0x1000           ; Multiply Y position by this for WAIT

; Copper MOVE opcodes
.equ COP_MOVE_RASTER_Y     0x40120000
.equ COP_MOVE_RASTER_H     0x40130000
.equ COP_MOVE_RASTER_COLOR 0x40140000
.equ COP_MOVE_RASTER_CTRL  0x40150000

; Copper END opcode
.equ COP_END               0xC0000000

; Copper SETBASE opcode - set I/O base for subsequent MOVE operations
; Format: [10][6-bit unused][24-bit base >> 2]
; Addresses are 4-byte aligned, so we encode (addr >> 2) in 24 bits
.equ COP_SETBASE_OP        0x80000000
.equ COP_SETBASE_SHIFT     2

; Pre-computed SETBASE instructions for common devices
.equ COP_SETBASE_VIDEO     0x8003C000    ; VIDEO_REG_BASE (0xF0000 >> 2)
.equ COP_SETBASE_VGA       0x8003C400    ; VGA_BASE (0xF1000 >> 2)
.equ COP_SETBASE_VGA_DAC   0x8003C416    ; VGA_DAC_WINDEX (0xF1058 >> 2)

; MOVE register indices after SETBASE to VGA_DAC_WINDEX
.equ COP_MOVE_VGA_WINDEX   0x40000000    ; Offset 0: DAC write index
.equ COP_MOVE_VGA_DATA     0x40010000    ; Offset 1: DAC data (R,G,B seq)

; ============================================================================
; TIMER REGISTERS
; ============================================================================

.equ TIMER_CTRL        0x00F800         ; Timer control
.equ TIMER_COUNT       0x00F804         ; Timer count
.equ TIMER_RELOAD      0x00F808         ; Timer reload value

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

.equ ULA_BASE          0xF2000          ; ULA register base
.equ ULA_BORDER        0xF2000          ; Border color (bits 0-2)
.equ ULA_CTRL          0xF2004          ; Control register
.equ ULA_STATUS        0xF2008          ; Status register

; Control register bits
.equ ULA_CTRL_ENABLE   0x01             ; ULA enable bit

; VRAM layout (same as authentic ZX Spectrum)
.equ ULA_VRAM          0x4000           ; VRAM base address
.equ ULA_BITMAP_SIZE   6144             ; Bitmap: 256x192/8 = 6144 bytes
.equ ULA_ATTR_OFFSET   0x1800           ; Attribute offset from VRAM base
.equ ULA_ATTR_SIZE     768              ; Attributes: 32x24 = 768 bytes
.equ ULA_VRAM_SIZE     6912             ; Total: 6144 + 768 bytes

; Display dimensions
.equ ULA_DISPLAY_W     256              ; Display width in pixels
.equ ULA_DISPLAY_H     192              ; Display height in pixels
.equ ULA_BORDER_SIZE   32               ; Border size in pixels
.equ ULA_FRAME_W       320              ; Total frame width (256+64)
.equ ULA_FRAME_H       256              ; Total frame height (192+64)
.equ ULA_CELLS_X       32               ; Character cells horizontally
.equ ULA_CELLS_Y       24               ; Character cells vertically

; ============================================================================
; EOF
; ============================================================================
