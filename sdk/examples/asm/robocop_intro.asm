; ============================================================================
; ROBOCOP INTRO - Blitter Sprite, Copper Rasterbars and PSG Music Demo
; IE32 assembly for IntuitionEngine - VideoChip + Copper + PSG audio
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC)
; Video Chip:    IEVideoChip Mode 0 (640x480, 32bpp true colour)
; Audio Engine:  PSG (AY-3-8910 compatible, PSG+ enhanced mode)
; Assembler:     ie32asm
; Build:         sdk/bin/ie32asm sdk/examples/asm/robocop_intro.asm
; Run:           ./IntuitionEngine -ie32 robocop_intro.iex
; Porting:       See robocop_intro_65.asm (6502), robocop_intro_68k.asm
;                (M68K), robocop_intro_z80.asm (Z80)
;
; === WHAT THIS DEMO DOES ===
; 1. Clears the screen to solid black using a blitter fill operation
; 2. Loads and plays an AY-format music file (Robocop theme) via PSG+
; 3. Programmes a 16-bar copper list for animated rasterbar colour cycling
; 4. Moves a masked Robocop sprite along a sine/cosine Lissajous path
; 5. Renders a sine-wave scrolltext along the bottom of the screen
; 6. Animates copper bar colours each frame using a scrolling gradient
;
; === WHY BLITTER IMAGE DISPLAY + COPPER EFFECTS ===
; This demo recreates the style of classic 8-bit and 16-bit game intro
; screens -- specifically inspired by the Robocop (1988) home computer
; ports by Ocean Software. On machines like the ZX Spectrum and Amstrad
; CPC, the loading screen was often the player's first impression of a
; game, and developers used every hardware trick available to make it
; memorable.
;
; The copper coprocessor is analogous to the Amiga's copper -- a simple
; programmable display coprocessor that can modify video registers at
; specific scanline positions. This enables effects like colour gradient
; bars, split-screen palettes, and per-scanline colour changes without
; any CPU intervention. The 16 rainbow bars here cycle their colours each
; frame, producing a flowing gradient wave reminiscent of demoscene
; rasterbar effects from the late 1980s.
;
; The hardware blitter handles all pixel operations: clearing the previous
; sprite position, drawing the masked sprite at its new location, and
; rendering scrolltext characters. This frees the CPU to focus on
; animation logic (sine table lookups, copper list updates) rather than
; pushing individual pixels -- exactly the division of labour that made
; the Amiga and Atari ST so effective for games and demos.
;
; === IE32-SPECIFIC NOTES ===
; This is the reference implementation. IE32 is a 32-bit RISC architecture
; with eight general-purpose registers (A-F, T, U) and direct memory-mapped
; I/O. All hardware registers are accessed via absolute addresses in the
; 0xF0000 I/O region. The IE32 can address the full 20-bit bus directly,
; so no banking is required -- data (sprite, mask, music, font) is simply
; appended after the code and referenced by label.
;
; === MEMORY MAP ===
; 0x0000 - 0x1FF7   Program code (~8KB)
; 0x2000 - 0x27FF   Sine/cosine lookup tables (256 entries each, 32-bit)
; 0x2800 - 0x283F   Colour palette (16 entries, 32-bit BGRA)
; 0x2840 - 0x2A83   Copper list (16 bars x 9 longwords + END marker)
; 0x2A84 onwards     Sprite RGBA, mask, AY music, font and scrolltext data
; 0x8800 - 0x880F   Runtime variables (frame counter, positions, scroll)
; 0xF0000           I/O registers (video, blitter, copper, PSG)
; 0x100000          VRAM start (640x480x4 = 1,228,800 bytes)
;
; === BUILD AND RUN ===
; Build:  sdk/bin/ie32asm sdk/examples/asm/robocop_intro.asm
; Run:    ./IntuitionEngine -ie32 robocop_intro.iex
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

; ----------------------------------------------------------------------------
; HARDWARE REGISTERS (I/O region at 0xF0000)
; ----------------------------------------------------------------------------
.equ VIDEO_CTRL        0xF0000
.equ VIDEO_MODE        0xF0004
.equ VIDEO_STATUS      0xF0008
.equ STATUS_VBLANK     2
.equ COPPER_CTRL       0xF000C
.equ COPPER_PTR        0xF0010
.equ BLT_CTRL          0xF001C
.equ BLT_OP            0xF0020
.equ BLT_SRC           0xF0024
.equ BLT_DST           0xF0028
.equ BLT_WIDTH         0xF002C
.equ BLT_HEIGHT        0xF0030
.equ BLT_SRC_STRIDE    0xF0034
.equ BLT_DST_STRIDE    0xF0038
.equ BLT_COLOR         0xF003C
.equ BLT_MASK          0xF0040
.equ BLT_STATUS        0xF0044
.equ VIDEO_RASTER_Y     0xF0048
.equ VIDEO_RASTER_HEIGHT 0xF004C
.equ VIDEO_RASTER_COLOR  0xF0050
.equ VIDEO_RASTER_CTRL   0xF0054

.equ PSG_PLUS_CTRL     0xF0C0E
.equ PSG_PLAY_PTR      0xF0C10
.equ PSG_PLAY_LEN      0xF0C14
.equ PSG_PLAY_CTRL     0xF0C18

; ----------------------------------------------------------------------------
; DISPLAY AND SPRITE CONSTANTS
; ----------------------------------------------------------------------------
.equ VRAM_START        0x100000
.equ SCREEN_W          640
.equ SCREEN_H          480
.equ LINE_BYTES        2560            ; 640 pixels x 4 bytes per pixel
.equ SPRITE_W          240
.equ SPRITE_H          180
.equ SPRITE_STRIDE     960             ; 240 pixels x 4 bytes per pixel
.equ CENTER_X          200             ; Horizontal centre of Lissajous path
.equ CENTER_Y          150             ; Vertical centre of Lissajous path
.equ BACKGROUND        0xFF000000      ; Opaque black (BGRA)

; Blitter operation codes
.equ BLT_OP_COPY       0
.equ BLT_OP_FILL       1
.equ BLT_OP_LINE       2
.equ BLT_OP_MASKED     3
.equ BLT_OP_ALPHA      4

; Copper bar layout constants
.equ BAR_COUNT         16
.equ BAR_STRIDE        36              ; Bytes per bar entry in copper list
.equ BAR_WAIT_OFFSET   0
.equ BAR_Y_OFFSET      8
.equ BAR_HEIGHT_OFFSET 16
.equ BAR_COLOR_OFFSET  24              ; Offset to colour value within each bar
.equ BAR_HEIGHT        12
.equ BAR_SPACING       20

; Runtime variable addresses (scratch RAM)
.equ VAR_FRAME_ADDR    0x8800
.equ VAR_PREV_X_ADDR   0x8804
.equ VAR_PREV_Y_ADDR   0x8808
.equ VAR_SCROLL_X      0x880C

.equ ROBOCOP_AY_LEN    24525

; Scrolltext constants
.equ SCROLL_Y          430             ; Baseline Y position for scrolltext
.equ SCROLL_SPEED      4               ; Pixels per frame horizontal scroll
.equ CHAR_WIDTH        32
.equ CHAR_HEIGHT       32
.equ FONT_STRIDE       1280            ; 320 pixels x 4 bytes (10 chars/row)

; Scrolltext data addresses (calculated: AY ends at 0x34269)
.equ SCROLL_SINE_ADDR  0x34269
.equ SCROLL_CHAR_ADDR  0x34669
.equ SCROLL_FONT_ADDR  0x34869
.equ SCROLL_MSG_ADDR   0x73069

; Data layout (DATA_START = 0x2000, after code is padded)
.equ SIN_X_ADDR        0x2000
.equ SIN_Y_ADDR        0x2000
.equ COS_Y_ADDR        0x2400
.equ PALETTE_ADDR      0x2800
.equ COPPER_LIST_ADDR  0x2840
.equ ROBOCOP_RGBA_ADDR 0x2A84
.equ ROBOCOP_MASK_ADDR 0x2CD84
.equ ROBOCOP_AY_ADDR   0x2E29C

; Copper MOVE opcodes
.equ COP_MOVE_RASTER_Y     0x40120000
.equ COP_MOVE_RASTER_H     0x40130000
.equ COP_MOVE_RASTER_COLOR 0x40140000
.equ COP_MOVE_RASTER_CTRL  0x40150000
.equ COP_END              0xC0000000

; ============================================================================
; INITIALISATION
; ============================================================================
start:
    ; Set 640x480 true colour mode and enable the display
    LDA #0
    STA @VIDEO_MODE
    LDA #1
    STA @VIDEO_CTRL

    ; --- Clear screen to black using a blitter fill ---
    JSR wait_blit
    LDA #BLT_OP_FILL
    STA @BLT_OP
    LDA #VRAM_START
    STA @BLT_DST
    LDA #SCREEN_W
    STA @BLT_WIDTH
    LDA #SCREEN_H
    STA @BLT_HEIGHT
    LDA #BACKGROUND
    STA @BLT_COLOR
    LDA #LINE_BYTES
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL
    JSR wait_blit

    ; --- Start PSG+ music playback ---
    ; Enable PSG+ enhanced mode, point to the embedded AY file,
    ; and start looped playback (bit 0 = start, bit 2 = loop)
    LDA #1
    STA @PSG_PLUS_CTRL
    LDA #ROBOCOP_AY_ADDR
    STA @PSG_PLAY_PTR
    LDA #ROBOCOP_AY_LEN
    STA @PSG_PLAY_LEN
    LDA #5
    STA @PSG_PLAY_CTRL

    ; --- Programme the copper coprocessor ---
    ; Disable copper, set the list pointer, then re-enable
    LDA #2
    STA @COPPER_CTRL
    LDA #COPPER_LIST_ADDR
    STA @COPPER_PTR
    LDA #1
    STA @COPPER_CTRL

    ; --- Initialise runtime state ---
    LDA #0
    STA @VAR_FRAME_ADDR
    LDA #0
    JSR compute_xy
    STC @VAR_PREV_X_ADDR
    STD @VAR_PREV_Y_ADDR

    ; Initialise scrolltext horizontal position
    LDA #0
    STA @VAR_SCROLL_X

; ============================================================================
; MAIN LOOP
; Each iteration: advance frame, update copper colours, move sprite,
; synchronise to VBlank, then draw.
; ============================================================================
main_loop:
    ; Advance frame counter
    LDA @VAR_FRAME_ADDR
    ADD A, #1
    STA @VAR_FRAME_ADDR

    ; Update copper bar colours with scrolling gradient effect
    LDA @VAR_FRAME_ADDR
    JSR update_bars

    ; Compute new sprite position from sine/cosine tables (C=x, D=y)
    LDA @VAR_FRAME_ADDR
    JSR compute_xy

    ; Calculate VRAM address of previous sprite position -> T
    LDE @VAR_PREV_Y_ADDR
    LDF #LINE_BYTES
    MUL E, F
    LDF @VAR_PREV_X_ADDR
    SHL F, #2
    ADD E, F
    ADD E, #VRAM_START
    LDT E

    ; Calculate VRAM address of new sprite position -> U
    LDE D
    LDF #LINE_BYTES
    MUL E, F
    LDF C
    SHL F, #2
    ADD E, F
    ADD E, #VRAM_START
    LDU E

    ; Save current position for next frame's erase
    STC @VAR_PREV_X_ADDR
    STD @VAR_PREV_Y_ADDR

    ; --- Synchronise to vertical blank before drawing ---
    ; This prevents tearing by ensuring all blitter operations
    ; happen during the blanking interval
    JSR wait_frame

    ; --- Erase previous sprite position ---
    ; Fill the old rectangle with the background colour
    JSR wait_blit
    LDA #BLT_OP_FILL
    STA @BLT_OP
    LDA T
    STA @BLT_DST
    LDA #SPRITE_W
    STA @BLT_WIDTH
    LDA #SPRITE_H
    STA @BLT_HEIGHT
    LDA #BACKGROUND
    STA @BLT_COLOR
    LDA #LINE_BYTES
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL
    JSR wait_blit

    ; --- Draw sprite at new position using masked blit ---
    ; The mask ensures transparent pixels are not written to VRAM,
    ; preserving the copper rasterbar effect behind the sprite
    LDA #BLT_OP_MASKED
    STA @BLT_OP
    LDA #ROBOCOP_RGBA_ADDR
    STA @BLT_SRC
    LDA #ROBOCOP_MASK_ADDR
    STA @BLT_MASK
    LDA U
    STA @BLT_DST
    LDA #SPRITE_W
    STA @BLT_WIDTH
    LDA #SPRITE_H
    STA @BLT_HEIGHT
    LDA #SPRITE_STRIDE
    STA @BLT_SRC_STRIDE
    LDA #LINE_BYTES
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL

    ; --- Render scrolltext ---
    JSR clear_scroll_area
    JSR draw_scrolltext
    LDA @VAR_SCROLL_X
    ADD A, #SCROLL_SPEED
    STA @VAR_SCROLL_X

    JMP main_loop

; ============================================================================
; SUBROUTINES
; ============================================================================

; ----------------------------------------------------------------------------
; wait_blit - Poll the blitter busy flag until the current operation completes
; ----------------------------------------------------------------------------
wait_blit:
    LDA @BLT_CTRL
    AND A, #2
    JNZ A, wait_blit
    RTS

; ----------------------------------------------------------------------------
; wait_vblank - Wait until the vertical blanking interval begins
; Used when you only need to detect the start of VBlank, not a full
; frame boundary.
; ----------------------------------------------------------------------------
wait_vblank:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JZ A, wait_vblank
    RTS

; ----------------------------------------------------------------------------
; wait_frame - Wait for exactly one complete frame boundary
; First waits for VBlank to END (active scan period begins), then waits
; for VBlank to START again (new frame). This ensures exactly one frame
; elapses per call, preventing the animation from running at double speed
; if called while already in VBlank.
; ----------------------------------------------------------------------------
wait_frame:
.wait_not_vblank:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JNZ A, .wait_not_vblank
.wait_vblank_start:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JZ A, .wait_vblank_start
    RTS

; ----------------------------------------------------------------------------
; update_bars - Animate copper bar colours with a scrolling sine gradient
;
; Each frame, every bar's colour is recalculated as:
;   colour_index = (bar_index + sine_scroll_offset + frame/4) mod 16
; This produces a flowing rainbow wave that shifts across all 16 bars.
;
; Input:  A = frame counter
; Clobbers: A-F, T, U
; ----------------------------------------------------------------------------
update_bars:
    LDB #0                      ; B = bar index (0-15)
    LDE #COPPER_LIST_ADDR       ; E = copper list pointer
    ADD E, #BAR_COLOR_OFFSET    ; point to first colour value
    LDF #PALETTE_ADDR           ; F = palette base

    ; Calculate global scroll offset from sine table
    LDC A                       ; C = frame
    SHL C, #1                   ; faster scroll
    AND C, #0xFF                ; wrap to 256 entries
    SHL C, #2                   ; * 4 bytes per entry
    ADD C, #SIN_X_ADDR          ; C = &sin_table[index]
    LDT [C]                     ; T = sine offset (-200 to +200)
    ADD T, #200                 ; T = 0 to 400
    SHR T, #4                   ; T = 0 to 25 (scroll offset)

bar_loop:
    ; Colour index = bar_index + scroll_offset + frame/4
    LDC A                       ; C = frame
    SHR C, #2                   ; slow down colour cycling
    ADD C, B                    ; + bar_index
    ADD C, T                    ; + sine scroll offset
    AND C, #0x0F                ; wrap to 16 colours
    SHL C, #2                   ; * 4 bytes per colour
    ADD C, F                    ; C = &palette[index]
    LDU [C]                     ; U = colour value
    STU [E]                     ; store colour in copper list

    ; Advance to next bar
    ADD E, #BAR_STRIDE
    ADD B, #1
    LDC #BAR_COUNT
    SUB C, B
    JNZ C, bar_loop
    RTS

; ----------------------------------------------------------------------------
; compute_xy - Calculate sprite position from sine/cosine tables
;
; The sprite follows a Lissajous curve: X uses sin(frame), Y uses
; cos(frame*2). The doubled frequency on Y creates an infinity-symbol
; (figure-8) motion path.
;
; Input:  A = frame counter
; Output: C = X position, D = Y position
; Clobbers: B, E
; ----------------------------------------------------------------------------
compute_xy:
    LDB A
    AND B, #0xFF
    SHL B, #2
    ADD B, #SIN_X_ADDR
    LDC [B]
    LDE #CENTER_X
    ADD C, E

    LDB A
    SHL B, #1
    AND B, #0xFF
    SHL B, #2
    ADD B, #COS_Y_ADDR
    LDD [B]
    LDE #CENTER_Y
    ADD D, E
    RTS

; ----------------------------------------------------------------------------
; clear_scroll_area - Erase the bottom 90 scanlines for scrolltext
; Uses the blitter fill operation to clear Y=390..479 to black.
; ----------------------------------------------------------------------------
clear_scroll_area:
    JSR wait_blit
    LDA #BLT_OP_FILL
    STA @BLT_OP
    LDA #390
    MUL A, #LINE_BYTES
    ADD A, #VRAM_START
    STA @BLT_DST
    LDA #SCREEN_W
    STA @BLT_WIDTH
    LDA #90
    STA @BLT_HEIGHT
    LDA #LINE_BYTES
    STA @BLT_DST_STRIDE
    LDA #BACKGROUND
    STA @BLT_COLOR
    LDA #1
    STA @BLT_CTRL
    RTS

; ----------------------------------------------------------------------------
; draw_scrolltext - Render sine-wave scrolling text using the blitter
;
; Characters are drawn from a pre-rendered RGBA font bitmap. Each
; character's Y position is offset by a sine wave lookup, creating the
; classic demoscene "bouncing scrolltext" effect. The scroll position
; advances each frame, and the message wraps when the null terminator
; is reached.
; ----------------------------------------------------------------------------
draw_scrolltext:
    LDB @VAR_SCROLL_X
    LDC B
    SHR C, #5
    LDD B
    AND D, #0x1F
    LDF #0
    SUB F, D
    LDD F
    LDE #0

.scroll_loop:
    LDF #SCROLL_MSG_ADDR
    ADD F, C
    LDT [F]
    AND T, #0xFF
    JZ T, .scroll_wrap

    ; Skip if character is off-screen to the left
    LDA D
    AND A, #0x80000000
    JNZ A, .scroll_next

    ; Skip if character is off-screen to the right
    LDA #608
    SUB A, D
    AND A, #0x80000000
    JNZ A, .scroll_done

    ; Preserve loop state across the blitter call
    PUSH C
    PUSH D
    PUSH E

    ; Look up character glyph offset in the font atlas
    LDF #SCROLL_CHAR_ADDR
    LDA T
    SHL A, #2
    ADD F, A
    LDA [F]
    ADD A, #SCROLL_FONT_ADDR
    LDF A

    ; Calculate Y with sine offset for wave motion
    LDA D
    ADD A, @VAR_SCROLL_X
    AND A, #0xFF
    SHL A, #2
    ADD A, #SCROLL_SINE_ADDR
    LDU [A]
    ADD U, #SCROLL_Y

    ; Calculate destination VRAM address
    LDT U
    MUL T, #LINE_BYTES
    ADD T, #VRAM_START
    LDA D
    SHL A, #2
    ADD T, A

    ; Blit the character glyph with alpha blending
    JSR wait_blit
    LDA #BLT_OP_ALPHA
    STA @BLT_OP
    LDA F
    STA @BLT_SRC
    LDA T
    STA @BLT_DST
    LDA #CHAR_WIDTH
    STA @BLT_WIDTH
    LDA #CHAR_HEIGHT
    STA @BLT_HEIGHT
    LDA #FONT_STRIDE
    STA @BLT_SRC_STRIDE
    LDA #LINE_BYTES
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL

    POP E
    POP D
    POP C

.scroll_next:
    ADD C, #1
    ADD D, #CHAR_WIDTH
    ADD E, #1
    LDA #21
    SUB A, E
    JNZ A, .scroll_loop

.scroll_done:
    RTS

.scroll_wrap:
    LDA @VAR_SCROLL_X
    AND A, #0x1F
    STA @VAR_SCROLL_X
    LDC #0
    JMP .scroll_loop

; ----------------------------------------------------------------------------
; PAD CODE TO 0x2000 SO DATA LABELS MATCH LOAD ADDRESS
; ----------------------------------------------------------------------------
.org 0x1FF8
    NOP

; ============================================================================
; DATA SECTION (placed at 0x2000)
; ============================================================================

; ----------------------------------------------------------------------------
; Sine table for X movement
; 256 entries, 32-bit signed values scaled to +/-200 pixels
; Pre-computed: sin(i * 2pi / 256) * 200
; ----------------------------------------------------------------------------
data_sin_x:
.word 0x00000000
.word 0x00000005
.word 0x0000000A
.word 0x0000000F
.word 0x00000014
.word 0x00000018
.word 0x0000001D
.word 0x00000022
.word 0x00000027
.word 0x0000002C
.word 0x00000031
.word 0x00000035
.word 0x0000003A
.word 0x0000003F
.word 0x00000043
.word 0x00000048
.word 0x0000004D
.word 0x00000051
.word 0x00000056
.word 0x0000005A
.word 0x0000005E
.word 0x00000063
.word 0x00000067
.word 0x0000006B
.word 0x0000006F
.word 0x00000073
.word 0x00000077
.word 0x0000007B
.word 0x0000007F
.word 0x00000083
.word 0x00000086
.word 0x0000008A
.word 0x0000008D
.word 0x00000091
.word 0x00000094
.word 0x00000097
.word 0x0000009B
.word 0x0000009E
.word 0x000000A1
.word 0x000000A4
.word 0x000000A6
.word 0x000000A9
.word 0x000000AC
.word 0x000000AE
.word 0x000000B0
.word 0x000000B3
.word 0x000000B5
.word 0x000000B7
.word 0x000000B9
.word 0x000000BB
.word 0x000000BC
.word 0x000000BE
.word 0x000000BF
.word 0x000000C1
.word 0x000000C2
.word 0x000000C3
.word 0x000000C4
.word 0x000000C5
.word 0x000000C6
.word 0x000000C6
.word 0x000000C7
.word 0x000000C7
.word 0x000000C8
.word 0x000000C8
.word 0x000000C8
.word 0x000000C8
.word 0x000000C8
.word 0x000000C7
.word 0x000000C7
.word 0x000000C6
.word 0x000000C6
.word 0x000000C5
.word 0x000000C4
.word 0x000000C3
.word 0x000000C2
.word 0x000000C1
.word 0x000000BF
.word 0x000000BE
.word 0x000000BC
.word 0x000000BB
.word 0x000000B9
.word 0x000000B7
.word 0x000000B5
.word 0x000000B3
.word 0x000000B0
.word 0x000000AE
.word 0x000000AC
.word 0x000000A9
.word 0x000000A6
.word 0x000000A4
.word 0x000000A1
.word 0x0000009E
.word 0x0000009B
.word 0x00000097
.word 0x00000094
.word 0x00000091
.word 0x0000008D
.word 0x0000008A
.word 0x00000086
.word 0x00000083
.word 0x0000007F
.word 0x0000007B
.word 0x00000077
.word 0x00000073
.word 0x0000006F
.word 0x0000006B
.word 0x00000067
.word 0x00000063
.word 0x0000005E
.word 0x0000005A
.word 0x00000056
.word 0x00000051
.word 0x0000004D
.word 0x00000048
.word 0x00000043
.word 0x0000003F
.word 0x0000003A
.word 0x00000035
.word 0x00000031
.word 0x0000002C
.word 0x00000027
.word 0x00000022
.word 0x0000001D
.word 0x00000018
.word 0x00000014
.word 0x0000000F
.word 0x0000000A
.word 0x00000005
.word 0x00000000
.word 0xFFFFFFFB
.word 0xFFFFFFF6
.word 0xFFFFFFF1
.word 0xFFFFFFEC
.word 0xFFFFFFE8
.word 0xFFFFFFE3
.word 0xFFFFFFDE
.word 0xFFFFFFD9
.word 0xFFFFFFD4
.word 0xFFFFFFCF
.word 0xFFFFFFCB
.word 0xFFFFFFC6
.word 0xFFFFFFC1
.word 0xFFFFFFBD
.word 0xFFFFFFB8
.word 0xFFFFFFB3
.word 0xFFFFFFAF
.word 0xFFFFFFAA
.word 0xFFFFFFA6
.word 0xFFFFFFA2
.word 0xFFFFFF9D
.word 0xFFFFFF99
.word 0xFFFFFF95
.word 0xFFFFFF91
.word 0xFFFFFF8D
.word 0xFFFFFF89
.word 0xFFFFFF85
.word 0xFFFFFF81
.word 0xFFFFFF7D
.word 0xFFFFFF7A
.word 0xFFFFFF76
.word 0xFFFFFF73
.word 0xFFFFFF6F
.word 0xFFFFFF6C
.word 0xFFFFFF69
.word 0xFFFFFF65
.word 0xFFFFFF62
.word 0xFFFFFF5F
.word 0xFFFFFF5C
.word 0xFFFFFF5A
.word 0xFFFFFF57
.word 0xFFFFFF54
.word 0xFFFFFF52
.word 0xFFFFFF50
.word 0xFFFFFF4D
.word 0xFFFFFF4B
.word 0xFFFFFF49
.word 0xFFFFFF47
.word 0xFFFFFF45
.word 0xFFFFFF44
.word 0xFFFFFF42
.word 0xFFFFFF41
.word 0xFFFFFF3F
.word 0xFFFFFF3E
.word 0xFFFFFF3D
.word 0xFFFFFF3C
.word 0xFFFFFF3B
.word 0xFFFFFF3A
.word 0xFFFFFF3A
.word 0xFFFFFF39
.word 0xFFFFFF39
.word 0xFFFFFF38
.word 0xFFFFFF38
.word 0xFFFFFF38
.word 0xFFFFFF38
.word 0xFFFFFF38
.word 0xFFFFFF39
.word 0xFFFFFF39
.word 0xFFFFFF3A
.word 0xFFFFFF3A
.word 0xFFFFFF3B
.word 0xFFFFFF3C
.word 0xFFFFFF3D
.word 0xFFFFFF3E
.word 0xFFFFFF3F
.word 0xFFFFFF41
.word 0xFFFFFF42
.word 0xFFFFFF44
.word 0xFFFFFF45
.word 0xFFFFFF47
.word 0xFFFFFF49
.word 0xFFFFFF4B
.word 0xFFFFFF4D
.word 0xFFFFFF50
.word 0xFFFFFF52
.word 0xFFFFFF54
.word 0xFFFFFF57
.word 0xFFFFFF5A
.word 0xFFFFFF5C
.word 0xFFFFFF5F
.word 0xFFFFFF62
.word 0xFFFFFF65
.word 0xFFFFFF69
.word 0xFFFFFF6C
.word 0xFFFFFF6F
.word 0xFFFFFF73
.word 0xFFFFFF76
.word 0xFFFFFF7A
.word 0xFFFFFF7D
.word 0xFFFFFF81
.word 0xFFFFFF85
.word 0xFFFFFF89
.word 0xFFFFFF8D
.word 0xFFFFFF91
.word 0xFFFFFF95
.word 0xFFFFFF99
.word 0xFFFFFF9D
.word 0xFFFFFFA2
.word 0xFFFFFFA6
.word 0xFFFFFFAA
.word 0xFFFFFFAF
.word 0xFFFFFFB3
.word 0xFFFFFFB8
.word 0xFFFFFFBD
.word 0xFFFFFFC1
.word 0xFFFFFFC6
.word 0xFFFFFFCB
.word 0xFFFFFFCF
.word 0xFFFFFFD4
.word 0xFFFFFFD9
.word 0xFFFFFFDE
.word 0xFFFFFFE3
.word 0xFFFFFFE8
.word 0xFFFFFFEC
.word 0xFFFFFFF1
.word 0xFFFFFFF6
.word 0xFFFFFFFB

; ----------------------------------------------------------------------------
; Cosine table for Y movement
; 256 entries, 32-bit signed values scaled to +/-150 pixels
; Pre-computed: cos(i * 2pi / 256) * 150
; ----------------------------------------------------------------------------
data_cos_y:
.word 0x00000096
.word 0x00000096
.word 0x00000096
.word 0x00000096
.word 0x00000095
.word 0x00000095
.word 0x00000094
.word 0x00000094
.word 0x00000093
.word 0x00000092
.word 0x00000092
.word 0x00000091
.word 0x00000090
.word 0x0000008E
.word 0x0000008D
.word 0x0000008C
.word 0x0000008B
.word 0x00000089
.word 0x00000088
.word 0x00000086
.word 0x00000084
.word 0x00000083
.word 0x00000081
.word 0x0000007F
.word 0x0000007D
.word 0x0000007B
.word 0x00000078
.word 0x00000076
.word 0x00000074
.word 0x00000072
.word 0x0000006F
.word 0x0000006D
.word 0x0000006A
.word 0x00000067
.word 0x00000065
.word 0x00000062
.word 0x0000005F
.word 0x0000005C
.word 0x00000059
.word 0x00000056
.word 0x00000053
.word 0x00000050
.word 0x0000004D
.word 0x0000004A
.word 0x00000047
.word 0x00000043
.word 0x00000040
.word 0x0000003D
.word 0x00000039
.word 0x00000036
.word 0x00000033
.word 0x0000002F
.word 0x0000002C
.word 0x00000028
.word 0x00000024
.word 0x00000021
.word 0x0000001D
.word 0x0000001A
.word 0x00000016
.word 0x00000012
.word 0x0000000F
.word 0x0000000B
.word 0x00000007
.word 0x00000004
.word 0x00000000
.word 0xFFFFFFFC
.word 0xFFFFFFF9
.word 0xFFFFFFF5
.word 0xFFFFFFF1
.word 0xFFFFFFEE
.word 0xFFFFFFEA
.word 0xFFFFFFE6
.word 0xFFFFFFE3
.word 0xFFFFFFDF
.word 0xFFFFFFDC
.word 0xFFFFFFD8
.word 0xFFFFFFD4
.word 0xFFFFFFD1
.word 0xFFFFFFCD
.word 0xFFFFFFCA
.word 0xFFFFFFC7
.word 0xFFFFFFC3
.word 0xFFFFFFC0
.word 0xFFFFFFBD
.word 0xFFFFFFB9
.word 0xFFFFFFB6
.word 0xFFFFFFB3
.word 0xFFFFFFB0
.word 0xFFFFFFAD
.word 0xFFFFFFAA
.word 0xFFFFFFA7
.word 0xFFFFFFA4
.word 0xFFFFFFA1
.word 0xFFFFFF9E
.word 0xFFFFFF9B
.word 0xFFFFFF99
.word 0xFFFFFF96
.word 0xFFFFFF93
.word 0xFFFFFF91
.word 0xFFFFFF8E
.word 0xFFFFFF8C
.word 0xFFFFFF8A
.word 0xFFFFFF88
.word 0xFFFFFF85
.word 0xFFFFFF83
.word 0xFFFFFF81
.word 0xFFFFFF7F
.word 0xFFFFFF7D
.word 0xFFFFFF7C
.word 0xFFFFFF7A
.word 0xFFFFFF78
.word 0xFFFFFF77
.word 0xFFFFFF75
.word 0xFFFFFF74
.word 0xFFFFFF73
.word 0xFFFFFF72
.word 0xFFFFFF70
.word 0xFFFFFF6F
.word 0xFFFFFF6E
.word 0xFFFFFF6E
.word 0xFFFFFF6D
.word 0xFFFFFF6C
.word 0xFFFFFF6C
.word 0xFFFFFF6B
.word 0xFFFFFF6B
.word 0xFFFFFF6A
.word 0xFFFFFF6A
.word 0xFFFFFF6A
.word 0xFFFFFF6A
.word 0xFFFFFF6A
.word 0xFFFFFF6A
.word 0xFFFFFF6A
.word 0xFFFFFF6B
.word 0xFFFFFF6B
.word 0xFFFFFF6C
.word 0xFFFFFF6C
.word 0xFFFFFF6D
.word 0xFFFFFF6E
.word 0xFFFFFF6E
.word 0xFFFFFF6F
.word 0xFFFFFF70
.word 0xFFFFFF72
.word 0xFFFFFF73
.word 0xFFFFFF74
.word 0xFFFFFF75
.word 0xFFFFFF77
.word 0xFFFFFF78
.word 0xFFFFFF7A
.word 0xFFFFFF7C
.word 0xFFFFFF7D
.word 0xFFFFFF7F
.word 0xFFFFFF81
.word 0xFFFFFF83
.word 0xFFFFFF85
.word 0xFFFFFF88
.word 0xFFFFFF8A
.word 0xFFFFFF8C
.word 0xFFFFFF8E
.word 0xFFFFFF91
.word 0xFFFFFF93
.word 0xFFFFFF96
.word 0xFFFFFF99
.word 0xFFFFFF9B
.word 0xFFFFFF9E
.word 0xFFFFFFA1
.word 0xFFFFFFA4
.word 0xFFFFFFA7
.word 0xFFFFFFAA
.word 0xFFFFFFAD
.word 0xFFFFFFB0
.word 0xFFFFFFB3
.word 0xFFFFFFB6
.word 0xFFFFFFB9
.word 0xFFFFFFBD
.word 0xFFFFFFC0
.word 0xFFFFFFC3
.word 0xFFFFFFC7
.word 0xFFFFFFCA
.word 0xFFFFFFCD
.word 0xFFFFFFD1
.word 0xFFFFFFD4
.word 0xFFFFFFD8
.word 0xFFFFFFDC
.word 0xFFFFFFDF
.word 0xFFFFFFE3
.word 0xFFFFFFE6
.word 0xFFFFFFEA
.word 0xFFFFFFEE
.word 0xFFFFFFF1
.word 0xFFFFFFF5
.word 0xFFFFFFF9
.word 0xFFFFFFFC
.word 0x00000000
.word 0x00000004
.word 0x00000007
.word 0x0000000B
.word 0x0000000F
.word 0x00000012
.word 0x00000016
.word 0x0000001A
.word 0x0000001D
.word 0x00000021
.word 0x00000024
.word 0x00000028
.word 0x0000002C
.word 0x0000002F
.word 0x00000033
.word 0x00000036
.word 0x00000039
.word 0x0000003D
.word 0x00000040
.word 0x00000043
.word 0x00000047
.word 0x0000004A
.word 0x0000004D
.word 0x00000050
.word 0x00000053
.word 0x00000056
.word 0x00000059
.word 0x0000005C
.word 0x0000005F
.word 0x00000062
.word 0x00000065
.word 0x00000067
.word 0x0000006A
.word 0x0000006D
.word 0x0000006F
.word 0x00000072
.word 0x00000074
.word 0x00000076
.word 0x00000078
.word 0x0000007B
.word 0x0000007D
.word 0x0000007F
.word 0x00000081
.word 0x00000083
.word 0x00000084
.word 0x00000086
.word 0x00000088
.word 0x00000089
.word 0x0000008B
.word 0x0000008C
.word 0x0000008D
.word 0x0000008E
.word 0x00000090
.word 0x00000091
.word 0x00000092
.word 0x00000092
.word 0x00000093
.word 0x00000094
.word 0x00000094
.word 0x00000095
.word 0x00000095
.word 0x00000096
.word 0x00000096
.word 0x00000096

; ----------------------------------------------------------------------------
; Colour palette for copper bars (16 entries, BGRA format)
; These colours form a rainbow gradient that the update_bars routine
; cycles through each frame.
; ----------------------------------------------------------------------------
data_palette:
.word 0xFF0000FF
.word 0xFF0040FF
.word 0xFF0080FF
.word 0xFF00C0FF
.word 0xFF00FF80
.word 0xFF00FF00
.word 0xFF40FF00
.word 0xFF80FF00
.word 0xFFFFFF00
.word 0xFFFFC000
.word 0xFFFF8000
.word 0xFFFF4000
.word 0xFFFF0000
.word 0xFFFF00FF
.word 0xFF8000FF
.word 0xFF4000FF

; ----------------------------------------------------------------------------
; Copper list - 16 horizontal raster bars
; Each bar consists of 9 longwords:
;   WAIT scanline, MOVE raster_y, MOVE raster_height, MOVE colour, MOVE ctrl
; The colour field at offset 24 is updated dynamically each frame by
; update_bars to produce the scrolling gradient effect.
; ----------------------------------------------------------------------------
data_copper_list:
; Bar 1: Y=40
.word 0x00028000                ; WAIT Y=40 (40*0x1000)
.word 0x40120000                ; COP_MOVE_RASTER_Y
.word 40
.word 0x40130000                ; COP_MOVE_RASTER_H
.word 12
.word 0x40140000                ; COP_MOVE_RASTER_COLOR
.word 0xFF0000FF
.word 0x40150000                ; COP_MOVE_RASTER_CTRL
.word 1
; Bar 2: Y=64
.word 0x00040000                ; WAIT Y=64
.word 0x40120000
.word 64
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFF0040FF
.word 0x40150000
.word 1
; Bar 3: Y=88
.word 0x00058000                ; WAIT Y=88
.word 0x40120000
.word 88
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFF0080FF
.word 0x40150000
.word 1
; Bar 4: Y=112
.word 0x00070000                ; WAIT Y=112
.word 0x40120000
.word 112
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFF00C0FF
.word 0x40150000
.word 1
; Bar 5: Y=136
.word 0x00088000                ; WAIT Y=136
.word 0x40120000
.word 136
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFF00FF80
.word 0x40150000
.word 1
; Bar 6: Y=160
.word 0x000A0000                ; WAIT Y=160
.word 0x40120000
.word 160
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFF00FF00
.word 0x40150000
.word 1
; Bar 7: Y=184
.word 0x000B8000                ; WAIT Y=184
.word 0x40120000
.word 184
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFF40FF00
.word 0x40150000
.word 1
; Bar 8: Y=208
.word 0x000D0000                ; WAIT Y=208
.word 0x40120000
.word 208
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFF80FF00
.word 0x40150000
.word 1
; Bar 9: Y=232
.word 0x000E8000                ; WAIT Y=232
.word 0x40120000
.word 232
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFFFFFF00
.word 0x40150000
.word 1
; Bar 10: Y=256
.word 0x00100000                ; WAIT Y=256
.word 0x40120000
.word 256
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFFFFC000
.word 0x40150000
.word 1
; Bar 11: Y=280
.word 0x00118000                ; WAIT Y=280
.word 0x40120000
.word 280
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFFFF8000
.word 0x40150000
.word 1
; Bar 12: Y=304
.word 0x00130000                ; WAIT Y=304
.word 0x40120000
.word 304
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFFFF4000
.word 0x40150000
.word 1
; Bar 13: Y=328
.word 0x00148000                ; WAIT Y=328
.word 0x40120000
.word 328
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFFFF0000
.word 0x40150000
.word 1
; Bar 14: Y=352
.word 0x00160000                ; WAIT Y=352
.word 0x40120000
.word 352
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFFFF00FF
.word 0x40150000
.word 1
; Bar 15: Y=376
.word 0x00178000                ; WAIT Y=376
.word 0x40120000
.word 376
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFF8000FF
.word 0x40150000
.word 1
; Bar 16: Y=400
.word 0x00190000                ; WAIT Y=400
.word 0x40120000
.word 400
.word 0x40130000
.word 12
.word 0x40140000
.word 0xFF4000FF
.word 0x40150000
.word 1
; End of copper list
.word 0xC0000000                ; COP_END

; ----------------------------------------------------------------------------
; Embedded binary data
; Sprite RGBA (240x180x4 = 172,800 bytes), mask, AY music file
; ----------------------------------------------------------------------------
data_robocop_rgba:
.incbin "../assets/robocop_rgba.bin"

data_robocop_mask:
.incbin "../assets/robocop_mask.bin"

data_robocop_ay:
.incbin "../assets/music/Robocop1.ay"

; ============================================================================
; SCROLLTEXT DATA
; ============================================================================

; ----------------------------------------------------------------------------
; Scrolltext sine table
; 256 entries of pre-calculated Y offsets: sin(i * 2pi / 256) * 20
; Range: -20 to +20 pixels. No runtime division needed.
; ----------------------------------------------------------------------------
scroll_sine_table:
.word 0x00000000
.word 0x00000000
.word 0x00000001
.word 0x00000001
.word 0x00000002
.word 0x00000002
.word 0x00000003
.word 0x00000003
.word 0x00000004
.word 0x00000004
.word 0x00000005
.word 0x00000005
.word 0x00000006
.word 0x00000006
.word 0x00000006
.word 0x00000007
.word 0x00000007
.word 0x00000008
.word 0x00000008
.word 0x00000009
.word 0x00000009
.word 0x00000009
.word 0x0000000A
.word 0x0000000A
.word 0x0000000A
.word 0x0000000B
.word 0x0000000B
.word 0x0000000B
.word 0x0000000C
.word 0x0000000C
.word 0x0000000C
.word 0x0000000D
.word 0x0000000D
.word 0x0000000D
.word 0x0000000D
.word 0x0000000E
.word 0x0000000E
.word 0x0000000E
.word 0x0000000E
.word 0x0000000F
.word 0x0000000F
.word 0x0000000F
.word 0x0000000F
.word 0x0000000F
.word 0x00000010
.word 0x00000010
.word 0x00000010
.word 0x00000010
.word 0x00000010
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000012
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000011
.word 0x00000010
.word 0x00000010
.word 0x00000010
.word 0x00000010
.word 0x00000010
.word 0x0000000F
.word 0x0000000F
.word 0x0000000F
.word 0x0000000F
.word 0x0000000F
.word 0x0000000E
.word 0x0000000E
.word 0x0000000E
.word 0x0000000E
.word 0x0000000D
.word 0x0000000D
.word 0x0000000D
.word 0x0000000D
.word 0x0000000C
.word 0x0000000C
.word 0x0000000C
.word 0x0000000B
.word 0x0000000B
.word 0x0000000B
.word 0x0000000A
.word 0x0000000A
.word 0x0000000A
.word 0x00000009
.word 0x00000009
.word 0x00000009
.word 0x00000008
.word 0x00000008
.word 0x00000007
.word 0x00000007
.word 0x00000006
.word 0x00000006
.word 0x00000006
.word 0x00000005
.word 0x00000005
.word 0x00000004
.word 0x00000004
.word 0x00000003
.word 0x00000003
.word 0x00000002
.word 0x00000002
.word 0x00000001
.word 0x00000001
.word 0x00000000
.word 0x00000000
.word 0xFFFFFFFF
.word 0xFFFFFFFF
.word 0xFFFFFFFE
.word 0xFFFFFFFE
.word 0xFFFFFFFD
.word 0xFFFFFFFD
.word 0xFFFFFFFC
.word 0xFFFFFFFC
.word 0xFFFFFFFB
.word 0xFFFFFFFB
.word 0xFFFFFFFA
.word 0xFFFFFFFA
.word 0xFFFFFFFA
.word 0xFFFFFFF9
.word 0xFFFFFFF9
.word 0xFFFFFFF8
.word 0xFFFFFFF8
.word 0xFFFFFFF7
.word 0xFFFFFFF7
.word 0xFFFFFFF7
.word 0xFFFFFFF6
.word 0xFFFFFFF6
.word 0xFFFFFFF6
.word 0xFFFFFFF5
.word 0xFFFFFFF5
.word 0xFFFFFFF5
.word 0xFFFFFFF4
.word 0xFFFFFFF4
.word 0xFFFFFFF4
.word 0xFFFFFFF3
.word 0xFFFFFFF3
.word 0xFFFFFFF3
.word 0xFFFFFFF3
.word 0xFFFFFFF2
.word 0xFFFFFFF2
.word 0xFFFFFFF2
.word 0xFFFFFFF2
.word 0xFFFFFFF1
.word 0xFFFFFFF1
.word 0xFFFFFFF1
.word 0xFFFFFFF1
.word 0xFFFFFFF1
.word 0xFFFFFFF0
.word 0xFFFFFFF0
.word 0xFFFFFFF0
.word 0xFFFFFFF0
.word 0xFFFFFFF0
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEE
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFEF
.word 0xFFFFFFF0
.word 0xFFFFFFF0
.word 0xFFFFFFF0
.word 0xFFFFFFF0
.word 0xFFFFFFF0
.word 0xFFFFFFF1
.word 0xFFFFFFF1
.word 0xFFFFFFF1
.word 0xFFFFFFF1
.word 0xFFFFFFF1
.word 0xFFFFFFF2
.word 0xFFFFFFF2
.word 0xFFFFFFF2
.word 0xFFFFFFF2
.word 0xFFFFFFF3
.word 0xFFFFFFF3
.word 0xFFFFFFF3
.word 0xFFFFFFF3
.word 0xFFFFFFF4
.word 0xFFFFFFF4
.word 0xFFFFFFF4
.word 0xFFFFFFF5
.word 0xFFFFFFF5
.word 0xFFFFFFF5
.word 0xFFFFFFF6
.word 0xFFFFFFF6
.word 0xFFFFFFF6
.word 0xFFFFFFF7
.word 0xFFFFFFF7
.word 0xFFFFFFF7
.word 0xFFFFFFF8
.word 0xFFFFFFF8
.word 0xFFFFFFF9
.word 0xFFFFFFF9
.word 0xFFFFFFFA
.word 0xFFFFFFFA
.word 0xFFFFFFFA
.word 0xFFFFFFFB
.word 0xFFFFFFFB
.word 0xFFFFFFFC
.word 0xFFFFFFFC
.word 0xFFFFFFFD
.word 0xFFFFFFFD
.word 0xFFFFFFFE
.word 0xFFFFFFFE
.word 0xFFFFFFFF
.word 0xFFFFFFFF

; ----------------------------------------------------------------------------
; Character lookup table
; Maps ASCII codes (0-127) to byte offsets into the font RGBA bitmap.
; Each entry is a 32-bit offset. Zero means no glyph available.
; ----------------------------------------------------------------------------
scroll_char_table:
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 128
.word 256
.word 0
.word 0
.word 0
.word 40960
.word 896
.word 1024
.word 1152
.word 512
.word 41600
.word 41216
.word 41344
.word 41472
.word 0
.word 41728
.word 41856
.word 41984
.word 42112
.word 81920
.word 82048
.word 82176
.word 82304
.word 82432
.word 82560
.word 82688
.word 82816
.word 0
.word 82944
.word 0
.word 122880
.word 384
.word 123264
.word 123392
.word 123520
.word 123648
.word 123776
.word 123904
.word 124032
.word 163840
.word 163968
.word 164096
.word 164224
.word 164352
.word 164480
.word 164608
.word 164736
.word 164864
.word 164992
.word 204800
.word 204928
.word 205056
.word 205184
.word 205312
.word 205440
.word 205568
.word 205696
.word 205824
.word 83072
.word 0
.word 122752
.word 768
.word 0
.word 205952
.word 123264
.word 123392
.word 123520
.word 123648
.word 123776
.word 123904
.word 124032
.word 163840
.word 163968
.word 164096
.word 164224
.word 164352
.word 164480
.word 164608
.word 164736
.word 164864
.word 164992
.word 204800
.word 204928
.word 205056
.word 205184
.word 205312
.word 205440
.word 205568
.word 205696
.word 205824
.word 123008
.word 0
.word 0
.word 41088
.word 0

; ----------------------------------------------------------------------------
; Font RGBA bitmap (pre-rendered 32x32 character glyphs, 10 chars per row)
; ----------------------------------------------------------------------------
scroll_font_data:
.incbin "../assets/font_rgba.bin"

; ----------------------------------------------------------------------------
; Scroll message (null-terminated ASCII)
; ----------------------------------------------------------------------------
scroll_message:
.ascii "    ...ROBOCOP DUAL CPU IE32 AND Z80 INTRO FOR THE INTUITION ENGINE... ...100 PERCENT ASM CODE... ...IE32 ASM FOR DEMO EFFECTS... ...Z80 ASM FOR MUSIC REPLAY ROUTINE... ...ALL CODE BY INTUITON...  MUSIC BY JONATHAN DUNN FROM THE 1987 ZX SPECTRUM GAME ROBOCOP BY OCEAN SOFTWARE... ...AY REGISTERS ARE REMAPPED TO THE INTUITON ENGINE SYNTH FOR SUPERIOR SOUND QUALITY... ...GREETS TO ...GADGETMASTER... ...KARLOS... ...BLOODLINE... ...VISIT INTUITIONSUBSYNTH.COM......................."
.byte 0
