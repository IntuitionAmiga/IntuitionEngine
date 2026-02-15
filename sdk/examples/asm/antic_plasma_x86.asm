; ============================================================================
; ANTIC PLASMA DEMO - Raster Bars with Vertical Sine Scrolltext and SID Music
; x86 Assembly for IntuitionEngine - ANTIC Display List + GTIA Colour
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    x86 (16/32-bit Intel-compatible)
; Video Chip:    ANTIC (Atari 8-bit display list processor) + GTIA
; Audio Engine:  SID (Commodore 64 sound chip, MOS 6581/8580)
; Assembler:     ie32asm (x86 mode)
; Build:         sdk/bin/ie32asm sdk/examples/asm/antic_plasma_x86.asm
; Run:           ./bin/IntuitionEngine -x86 antic_plasma_x86.ie86
; Porting:       ANTIC/GTIA MMIO is CPU-agnostic. Display list programming
;                works the same from any CPU core. SID audio setup is also
;                identical across all IntuitionEngine architectures.
;
; === WHAT THIS DEMO DOES ===
; 1. Displays animated plasma "raster bars" using per-scanline colour changes
; 2. Shows two vertical sine-wave scrolltexts using Player/Missile graphics
; 3. Uses an authentic ANTIC display list (similar to Amiga copper lists)
; 4. Plays SID music through the Commodore 64 audio emulation
;
; === WHY ANTIC DISPLAY LIST ARCHITECTURE ===
; ANTIC (Alpha-Numeric Television Interface Controller) was designed by
; Jay Miner for the Atari 400/800 in 1979. It was the first consumer
; display processor to use a programmable "display list" - essentially a
; small program stored in memory that tells the chip how to render each
; group of scanlines. This was revolutionary because it allowed mixed-mode
; displays (text and graphics on the same screen), variable resolution per
; zone, and fine scrolling - far more flexible than the fixed display
; formats of the Apple II or TRS-80.
;
; GTIA (George's Television Interface Adapter) was ANTIC's companion chip.
; While ANTIC controlled timing and display list execution, GTIA handled
; the actual colour output (128 colours: 16 hues x 8 luminances) and
; player/missile (sprite) graphics. This demo makes extensive use of both
; chips: ANTIC for display list timing and WSYNC synchronisation, GTIA for
; per-scanline background colours and Player/Missile scrolltext rendering.
;
; In the IntuitionEngine, ANTIC's display list architecture is faithfully
; emulated. The display list is stored in memory and processed by the
; video chip each frame, just as on the original Atari hardware.
;
; === WHY THESE EFFECTS MATTER (HISTORICAL CONTEXT) ===
;
; PLASMA EFFECTS:
; "Plasma" refers to smoothly animated colour patterns created by combining
; multiple sine waves. On 8-bit hardware, these were computed in real-time
; using lookup tables. The effect creates an organic, flowing appearance
; that was impressive on hardware with limited colour palettes.
;
; PLAYER/MISSILE GRAPHICS (P/M):
; Atari's hardware sprites are called "players" (8 pixels wide) and
; "missiles" (2 pixels wide). Unlike software sprites, they do not require
; erasing/redrawing - just update position registers and graphics data.
; This demo uses all 4 players for two dual-character scrolltexts.
;
; VERTICAL SCROLLING WITH SINE WAVE:
; Classic demoscene effect where text scrolls vertically through the screen
; while each character wobbles horizontally following a sine wave pattern.
; This creates a "wavy" or "rubbery" text effect popular in Amiga and
; Atari demos of the late 1980s and early 1990s.
;
; === WHY x86 FOR AN ATARI DEMO? ===
; The original Atari 8-bits used a 6502 CPU at 1.79 MHz. This demo runs on
; the IntuitionEngine's 32-bit x86 core, demonstrating that:
;   1. The ANTIC/GTIA emulation works correctly with any CPU architecture
;   2. Modern 32-bit maths makes plasma calculations trivial (vs 8-bit)
;   3. You can mix retro video hardware with modern CPU capabilities
;
; On a real 6502, this plasma effect would require extensive optimisation,
; pre-calculated tables, and careful cycle counting. On x86, we can use
; IMUL instructions and 32-bit registers freely.
;
; === ARCHITECTURE OVERVIEW ===
;
;   +-------------------------------------------------------------+
;   |                     MAIN LOOP (~60 FPS)                     |
;   |                                                             |
;   |  +-----------+    +-----------+    +-------------------+    |
;   |  | WAIT FOR  |--->|  UPDATE   |--->| WAIT FOR ACTIVE   |    |
;   |  |  VBLANK   |    |   TIME    |    |     DISPLAY       |    |
;   |  +-----------+    +-----------+    +-------------------+    |
;   |                                            |                |
;   |  +---------------------------------------------+           |
;   |  |         PER-SCANLINE LOOP (192x)             |           |
;   |  |  +----------+  +----------+  +-----------+   |           |
;   |  |  | COMPUTE  |  |   DRAW   |  |   WSYNC   |   |           |
;   |  |  | PLASMA   |  | SCROLLERS|  | (sync to  |   |           |
;   |  |  | COLOUR   |  | (P/M gfx)|  | scanline) |   |           |
;   |  |  +----------+  +----------+  +-----------+   |           |
;   |  +---------------------------------------------+           |
;   +-------------------------------------------------------------+
;
;   +-------------------------------------------------------------+
;   |           ANTIC DISPLAY LIST (runs in parallel)             |
;   |                                                             |
;   |  The ANTIC chip reads and executes the display list program |
;   |  independently of the CPU. It specifies:                    |
;   |  - How many blank lines (borders)                           |
;   |  - Which graphics mode for each line                        |
;   |  - Where screen memory is located (LMS instruction)         |
;   |  - When to trigger Display List Interrupts (DLI)            |
;   |  - When to jump back and wait for vertical blank (JVB)      |
;   +-------------------------------------------------------------+
;
;   +-------------------------------------------------------------+
;   |           SID AUDIO ENGINE (runs in parallel)               |
;   |                                                             |
;   |  The SID emulation plays Commodore 64 music files           |
;   |  independently. Once started, it synthesises audio without  |
;   |  any CPU intervention, freeing us for graphics work.        |
;   +-------------------------------------------------------------+
;
; === REGISTER USAGE ===
;   ESI - Time counter 1 (plasma wave animation)
;   EDI - Time counter 2 (plasma wave animation, different speed)
;   ECX - Current scanline number (0-191) during rendering
;   EAX, EBX, EDX, EBP - Scratch registers for calculations
;
; === MEMORY MAP ===
; 0x000000           Program entry point (org 0)
; 0xFF0000           Stack top (grows downward)
; 0xF2100-0xF213F    ANTIC registers (DMACTL, display list ptr, WSYNC, etc.)
; 0xF2140-0xF21B7    GTIA registers (COLBK, COLPM, HPOSP, GRAFP, etc.)
; 0xF0E00-0xF0E2F    SID registers (voices, filter, player control)
; display_list       ANTIC display list program (in .data section)
; sin_table          256-byte sine lookup table
; font_data          8x8 bitmap font (ASCII 32-95)
; sid_data           Embedded SID music file
;
; === BUILD AND RUN ===
; sdk/bin/ie32asm sdk/examples/asm/antic_plasma_x86.asm
; ./bin/IntuitionEngine -x86 antic_plasma_x86.ie86
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

bits 32                                 ; 32-bit protected mode code
org 0                                   ; Code starts at address 0

%include "ie86.inc"                     ; IntuitionEngine hardware definitions

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Character Rendering ---
; Each character in our 8x8 font is 8 pixels tall.
CHAR_HEIGHT     equ 8

; --- Scrolltext Horizontal Positions ---
; The two scrolltexts are positioned just left and right of screen centre.
; ANTIC/GTIA display is 320 pixels wide, so centre is 160.
; Players are positioned using HPOSP registers (0-255 range maps to screen).
SCROLL1_X_BASE  equ 152                 ; Left scrolltext base X (cyan)
SCROLL2_X_BASE  equ 184                 ; Right scrolltext base X (yellow)

; --- GTIA Colour Format ---
; GTIA colours are encoded as: HHHHLLLL
;   HHHH = Hue (0-15): 0=grey, 1=gold, 2=orange, ... 9=cyan, etc.
;   LLLL = Luminance (0-14 even numbers): 0=black, 14=white
SCROLL1_COLOR   equ 0x96                ; Dark cyan for left scrolltext
SCROLL2_COLOR   equ 0x16                ; Dark yellow/orange for right scrolltext

; --- Fixed-Point Arithmetic (16.16 Format) ---
; For smooth scrolling, we use fixed-point maths where:
;   Upper 16 bits = integer part (pixel position)
;   Lower 16 bits = fractional part (sub-pixel precision)
; scroll_y += 0x18000 means moving 1.5 pixels per frame.
SCROLL_SPEED    equ 0x18000             ; Vertical scroll speed (16.16 fixed)

; ============================================================================
; CODE SECTION
; ============================================================================
section .text
global _start

; ============================================================================
; ENTRY POINT
; ============================================================================
; The CPU begins execution here. We initialise all hardware subsystems
; before entering the main loop.
; ============================================================================

_start:
        ; --- Initialise Stack Pointer ---
        ; x86 uses a descending stack (grows toward lower addresses).
        ; ESP must point to valid RAM before any PUSH/CALL instructions.
        mov     esp, STACK_TOP

; ============================================================================
; ANTIC DISPLAY LIST SETUP
; ============================================================================
; The display list is ANTIC's program - a sequence of instructions that
; control what appears on each scanline. This is conceptually similar to
; the Amiga's copper coprocessor, but with a different instruction set.
;
; ANTIC reads the display list from memory during each frame, executing
; instructions to generate the display. The CPU can modify the display
; list between frames to create animation effects.
; ============================================================================

        ; --- Point ANTIC to Our Display List ---
        ; DLISTL/DLISTH form a 16-bit pointer to the display list.
        ; ANTIC reads instructions from this address during each frame.
        mov     eax, display_list       ; Get address of our display list
        mov     byte [ANTIC_DLISTL], al ; Low byte of address
        shr     eax, 8                  ; Shift to get high byte
        mov     byte [ANTIC_DLISTH], al ; High byte of address

        ; --- Enable ANTIC DMA ---
        ; DMACTL controls what ANTIC fetches from memory:
        ;   ANTIC_DMA_DL     = Enable display list DMA (required)
        ;   ANTIC_DMA_NORMAL = Normal playfield width (40 bytes/line)
        ;   ANTIC_DMA_PLAYER = Enable player/missile DMA
        ; Without DMA enabled, ANTIC outputs nothing.
        mov     byte [ANTIC_DMACTL], ANTIC_DMA_DL | ANTIC_DMA_NORMAL | ANTIC_DMA_PLAYER

        ; --- Enable ANTIC Video Output ---
        ; IntuitionEngine extension - turns on ANTIC rendering.
        mov     byte [ANTIC_ENABLE], ANTIC_ENABLE_VIDEO

; ============================================================================
; GTIA PLAYER/MISSILE SETUP
; ============================================================================
; GTIA provides 4 "players" (8-pixel wide sprites) that we use for scrolltext.
; Each player has:
;   HPOSP: Horizontal position (0-255)
;   GRAFP: Graphics data (8 bits = 8 pixels per scanline)
;   COLPM: Colour register
;   SIZEP: Width (0=normal, 1=double, 3=quad)
;
; We use players 0-1 for the left (cyan) scrolltext and
; players 2-3 for the right (yellow) scrolltext.
; ============================================================================

        ; --- Enable Player Graphics ---
        ; GRACTL must have the PLAYER bit set to display players.
        mov     byte [GTIA_GRACTL], GTIA_GRACTL_PLAYER

        ; --- Set Player Colours ---
        ; Each player has its own colour register. We use two colours:
        ; cyan (0x96) for left column, yellow (0x16) for right column.
        mov     byte [GTIA_COLPM0], SCROLL1_COLOR
        mov     byte [GTIA_COLPM1], SCROLL1_COLOR
        mov     byte [GTIA_COLPM2], SCROLL2_COLOR
        mov     byte [GTIA_COLPM3], SCROLL2_COLOR

        ; --- Set Player Widths ---
        ; SIZEP values: 0=normal (8 pixels), 1=double (16 pixels), 3=quad
        ; Double width makes the text more readable on the plasma background.
        mov     byte [GTIA_SIZEP0], 1
        mov     byte [GTIA_SIZEP1], 1
        mov     byte [GTIA_SIZEP2], 1
        mov     byte [GTIA_SIZEP3], 1

; ============================================================================
; SID MUSIC PLAYBACK SETUP
; ============================================================================
; The SID (Sound Interface Device) was the audio chip in the Commodore 64.
; The IntuitionEngine emulates it, allowing playback of .SID music files.
; Once started, the SID engine plays autonomously - no CPU intervention needed.
;
; Playing "OdDnB" by Jammer (Kamil Wolnikowski), a SID tune that
; demonstrates the chip's distinctive sound capabilities.
; ============================================================================

        mov     dword [SID_PLAY_PTR], sid_data
        mov     dword [SID_PLAY_LEN], sid_data_end - sid_data
        mov     dword [SID_PLAY_CTRL], 5                    ; Start playback (looped)

; ============================================================================
; INITIALISE ANIMATION STATE
; ============================================================================
; Two time counters advance at different rates to create complex,
; non-repeating plasma patterns. Using relatively prime increments (2 and 3)
; ensures the pattern does not repeat for a very long time.
; ============================================================================

        xor     esi, esi                ; Time counter 1 = 0 (increments by 2)
        xor     edi, edi                ; Time counter 2 = 0 (increments by 3)
        mov     dword [scroll_y], 0     ; Vertical scroll position = 0
        mov     dword [base_char_idx], 0

; ============================================================================
; MAIN LOOP
; ============================================================================
; Runs once per frame (~60 times per second). Each iteration:
;   1. Waits for vertical blank (top of frame)
;   2. Updates animation time counters
;   3. Waits for active display to begin
;   4. Renders each scanline with plasma colours and scrolltext
; ============================================================================

main_loop:

; --- Wait for Vertical Blank ---
; VBlank occurs when the beam returns from the bottom of the screen to
; the top. This is the safest time to update graphics state because
; nothing is being displayed.
;
; ANTIC_STATUS bit 0 = 1 during VBlank, 0 during active display.

.wait_vblank:
        test    byte [ANTIC_STATUS], ANTIC_STATUS_VBLANK
        jz      .wait_vblank

        ; --- Update Animation Time Counters ---
        ; Different increment rates create phase differences between effects.
        add     esi, 2                  ; Time1 advances slowly
        add     edi, 3                  ; Time2 advances faster (creates variation)

        ; --- Update Vertical Scroll Position ---
        ; scroll_y is in 16.16 fixed-point format:
        ;   Upper 16 bits = pixel offset
        ;   Lower 16 bits = sub-pixel fraction
        mov     eax, [scroll_y]
        add     eax, SCROLL_SPEED       ; Add ~1.5 pixels per frame
        mov     [scroll_y], eax

        ; =====================================================================
        ; PRE-CALCULATE SCROLL VALUES DURING VBLANK
        ; =====================================================================
        ; Instead of doing expensive division (DIV) 192 times per frame in
        ; the scanline loop, we calculate the base character index ONCE here.
        ;
        ; WHY: DIV takes 20-40 cycles on x86. Doing it 192 times per frame
        ; wastes 4000-8000 cycles. Doing it once during VBlank is free.
        ; =====================================================================

        ; Get integer part of scroll position
        shr     eax, 16                 ; Integer part (pixel offset)
        mov     [scroll_pixel], eax

        ; Calculate base_char_idx = (scroll_pixel / 8) % message_len
        shr     eax, 3                  ; Divide by 8 (char height)
        xor     edx, edx
        mov     ebx, message_len
        div     ebx                     ; Slow DIV, but only ONCE per frame
        mov     [base_char_idx], edx    ; Store remainder (wrapped index)

; --- Wait for Active Display ---
; Synchronise with the start of the visible area so our per-scanline
; effects align correctly with the ANTIC display list.

.wait_active:
        test    byte [ANTIC_STATUS], ANTIC_STATUS_VBLANK
        jnz     .wait_active

        ; --- Begin Scanline Rendering ---
        xor     ecx, ecx                ; ECX = scanline counter (0-191)

; ============================================================================
; PER-SCANLINE RENDERING LOOP
; ============================================================================
; Executes 192 times per frame, once for each visible scanline. On each:
;   1. Calculate and set the plasma background colour
;   2. Determine which scrolltext characters are visible
;   3. Set player graphics and positions for the scrolltext
;   4. WSYNC to synchronise with the next scanline
;
; WSYNC is crucial: writing to ANTIC_WSYNC halts the CPU until the next
; horizontal blank, ensuring our register changes take effect at the
; correct scanline.
; ============================================================================

.scanline_loop:

; ============================================================================
; PLASMA BACKGROUND CALCULATION
; ============================================================================
; The plasma effect combines multiple sine waves at different frequencies
; to create a smoothly animated colour pattern. We use 4 waves:
;
;   Waves 1-2: Control the HUE (colour) component
;   Waves 3-4: Control the LUMINANCE (brightness) component
;
; Each wave: sin_table[(scanline * freq + time) & 0xFF]
; The lookup table returns 0-255, representing a full sine cycle.
;
; Different frequencies and time offsets for each wave create complex
; interference patterns that appear to flow and shift organically.
; ============================================================================

        ; === Wave 1: Fast Vertical Frequency (Hue) ===
        ; Changes rapidly with scanline position, creating tight horizontal
        ; colour bands.
        mov     eax, ecx
        shl     eax, 3                  ; Multiply by 8 (high frequency)
        add     eax, esi                ; Add time offset (animates the wave)
        and     eax, 0xFF               ; Wrap to 0-255 (sine table size)
        movzx   eax, byte [sin_table + eax]
        mov     ebx, eax                ; EBX accumulates hue component

        ; === Wave 2: Medium Frequency with Time Offset (Hue) ===
        ; A slower wave that creates broader colour regions.
        mov     eax, ecx
        lea     eax, [eax + eax*2]      ; EAX = scanline * 3
        add     eax, edi                ; Add time2 (different phase from wave 1)
        shl     eax, 1                  ; Double for more variation
        and     eax, 0xFF
        movzx   eax, byte [sin_table + eax]
        add     ebx, eax                ; Add to hue accumulator

        ; === Wave 3: Slow Undulation (Luminance) ===
        ; Uses both time counters for complex motion.
        mov     eax, ecx
        add     eax, esi                ; Add time1
        add     eax, edi                ; Add time2 (creates beating pattern)
        and     eax, 0xFF
        movzx   eax, byte [sin_table + eax]
        mov     ebp, eax                ; EBP accumulates luminance component

        ; === Wave 4: Cross-Pattern (Luminance) ===
        ; Subtracting time creates motion in the opposite direction,
        ; producing a "crossing" effect in the plasma pattern.
        mov     eax, ecx
        imul    eax, 5                  ; Higher frequency multiplier
        add     eax, esi
        sub     eax, edi                ; SUBTRACT time2 (opposite motion)
        and     eax, 0xFF
        movzx   eax, byte [sin_table + eax]
        add     ebp, eax                ; Add to luminance accumulator

        ; === Combine into GTIA Colour Format ===
        ; GTIA colour byte: HHHHLLLL (hue in bits 4-7, luminance in bits 1-3)
        shr     ebx, 1                  ; Scale hue (0-510 -> 0-255)
        and     ebx, 0xF0               ; Keep only upper nibble (hue)

        shr     ebp, 5                  ; Scale luminance (0-510 -> 0-15)
        and     ebp, 0x0E               ; Keep bits 1-3 (even values only)

        or      ebx, ebp                ; Combine hue and luminance
        mov     byte [GTIA_COLBK], bl   ; Set background colour for this scanline

; ============================================================================
; VERTICAL SCROLLTEXT RENDERING
; ============================================================================
; Two scrolltexts using the 4 available players:
;   Players 0-1: Left scrolltext (cyan), different message portions
;   Players 2-3: Right scrolltext (yellow), different message portions
;
; Each player displays one character at a time. By updating GRAFP (graphics)
; and HPOSP (position) for each scanline, we create the vertical scrolling
; and horizontal sine-wave wobble effect.
;
; For each scanline:
;   1. Determine which character is visible (based on scroll_y)
;   2. Determine which row of that character to display (0-7)
;   3. Look up the font data for that character row
;   4. Calculate X position with sine wobble
;   5. Write to GRAFP and HPOSP registers
; ============================================================================

        ; --- Clear All Player Graphics ---
        ; Start with blank players; we set graphics only where characters appear
        mov     byte [GTIA_GRAFP0], 0
        mov     byte [GTIA_GRAFP1], 0
        mov     byte [GTIA_GRAFP2], 0
        mov     byte [GTIA_GRAFP3], 0

        ; =====================================================================
        ; CHARACTER POSITION CALCULATION (OPTIMISED)
        ; =====================================================================
        ; We pre-calculated base_char_idx during VBlank. Now we just add the
        ; scanline offset. Simple comparison/subtraction replaces DIV for the
        ; modulo wrapping, since we add at most 24 chars (192 scanlines / 8).
        ; =====================================================================

        ; Calculate char_line = (scroll_pixel + scanline) & 7
        mov     eax, [scroll_pixel]
        add     eax, ecx                ; Add current scanline
        mov     edx, eax
        and     edx, 7                  ; EDX = character row (0-7)

        ; Calculate char_index = base_char_idx + (scanline >> 3)
        mov     eax, ecx
        shr     eax, 3                  ; scanline / 8
        add     eax, [base_char_idx]

        ; Simple modulo wrap (no DIV needed)
        cmp     eax, message_len
        jl      .no_wrap
        sub     eax, message_len
.no_wrap:
        ; Store for reuse across all four player calculations
        mov     [cur_char_idx], eax
        mov     [cur_char_line], edx

; --------------------------------------------------------------------------
; SCROLLTEXT 1, CHARACTER 1 (Player 0 - Cyan, Left Column)
; --------------------------------------------------------------------------

        ; --- Get ASCII Character from Message ---
        movzx   ebx, byte [scroll_message + eax]
        sub     ebx, 32                 ; Convert ASCII to font index (space=0)
        jl      .skip_char1
        cmp     ebx, 64                 ; Font covers ASCII 32-95 (64 chars)
        jge     .skip_char1

        ; --- Look Up Font Data ---
        ; Font is 8 bytes per character: byte[char * 8 + row]
        shl     ebx, 3
        add     ebx, edx
        movzx   eax, byte [font_data + ebx]
        mov     byte [GTIA_GRAFP0], al

        ; --- Calculate X Position with Sine Wobble ---
        ; X = base_x + sin(scanline + time) / 16
        mov     eax, ecx
        add     eax, esi                ; Add time for animation
        and     eax, 0xFF
        movzx   eax, byte [sin_table + eax]
        sub     eax, 128                ; Centre around 0 (-128 to +127)
        sar     eax, 4                  ; Reduce amplitude (divide by 16)
        add     eax, SCROLL1_X_BASE
        mov     byte [GTIA_HPOSP0], al

.skip_char1:

; --------------------------------------------------------------------------
; SCROLLTEXT 1, CHARACTER 2 (Player 1 - Cyan, Left Column)
; --------------------------------------------------------------------------
; Shows a different part of the message (offset by half the message length)

        mov     eax, [cur_char_idx]
        mov     edx, [cur_char_line]
        add     eax, message_len/2
        cmp     eax, message_len
        jl      .char1b_ok
        sub     eax, message_len
.char1b_ok:
        movzx   ebx, byte [scroll_message + eax]
        sub     ebx, 32
        jl      .skip_char1b
        cmp     ebx, 64
        jge     .skip_char1b

        shl     ebx, 3
        add     ebx, edx
        movzx   eax, byte [font_data + ebx]
        mov     byte [GTIA_GRAFP1], al

        ; X position with OPPOSITE phase wobble (add 128 to sine input)
        mov     eax, ecx
        add     eax, esi
        add     eax, 128                ; Phase offset (half cycle)
        and     eax, 0xFF
        movzx   eax, byte [sin_table + eax]
        sub     eax, 128
        sar     eax, 4
        add     eax, SCROLL1_X_BASE
        add     eax, 20                 ; Horizontal offset from first char
        mov     byte [GTIA_HPOSP1], al

.skip_char1b:

; --------------------------------------------------------------------------
; SCROLLTEXT 2, CHARACTER 1 (Player 2 - Yellow, Right Column)
; --------------------------------------------------------------------------

        mov     eax, [cur_char_idx]
        mov     edx, [cur_char_line]
        add     eax, message_len/4      ; Different offset than scrolltext 1
        cmp     eax, message_len
        jl      .char2_ok
        sub     eax, message_len
.char2_ok:
        movzx   ebx, byte [scroll_message + eax]
        sub     ebx, 32
        jl      .skip_char2
        cmp     ebx, 64
        jge     .skip_char2

        shl     ebx, 3
        add     ebx, edx
        movzx   eax, byte [font_data + ebx]
        mov     byte [GTIA_GRAFP2], al

        ; Use time2 (EDI) for different animation phase
        mov     eax, ecx
        add     eax, edi
        and     eax, 0xFF
        movzx   eax, byte [sin_table + eax]
        sub     eax, 128
        sar     eax, 4
        add     eax, SCROLL2_X_BASE
        mov     byte [GTIA_HPOSP2], al

.skip_char2:

; --------------------------------------------------------------------------
; SCROLLTEXT 2, CHARACTER 2 (Player 3 - Yellow, Right Column)
; --------------------------------------------------------------------------

        mov     eax, [cur_char_idx]
        mov     edx, [cur_char_line]
        add     eax, message_len/4 + message_len/2  ; Combined offset
        cmp     eax, message_len
        jl      .char2b_ok
        sub     eax, message_len
.char2b_ok:
        cmp     eax, message_len        ; May need to wrap twice
        jl      .char2b_ok2
        sub     eax, message_len
.char2b_ok2:
        movzx   ebx, byte [scroll_message + eax]
        sub     ebx, 32
        jl      .no_scroll_char
        cmp     ebx, 64
        jge     .no_scroll_char

        shl     ebx, 3
        add     ebx, edx
        movzx   eax, byte [font_data + ebx]
        mov     byte [GTIA_GRAFP3], al

        mov     eax, ecx
        add     eax, edi
        add     eax, 128                ; Opposite phase
        and     eax, 0xFF
        movzx   eax, byte [sin_table + eax]
        sub     eax, 128
        sar     eax, 4
        add     eax, SCROLL2_X_BASE
        add     eax, 20
        mov     byte [GTIA_HPOSP3], al

.no_scroll_char:

        ; --- WSYNC: Wait for Horizontal Sync ---
        ; Writing ANY value to ANTIC_WSYNC halts the CPU until the next
        ; horizontal blank period. This ensures our colour/graphics changes
        ; take effect at the correct scanline.
        ;
        ; On the real Atari, WSYNC was used extensively for colour cycling
        ; effects, player multiplexing, and precise DLI timing.
        mov     byte [ANTIC_WSYNC], 0

        ; --- Advance to Next Scanline ---
        inc     ecx
        cmp     ecx, 192                ; 192 visible scanlines
        jl      .scanline_loop

        ; --- Loop Back for Next Frame ---
        jmp     main_loop

; ============================================================================
; DATA SECTION
; ============================================================================
section .data
        align 4

; ============================================================================
; ANTIC DISPLAY LIST
; ============================================================================
; The display list is ANTIC's "program" - a sequence of instructions that
; control video output. Each instruction specifies what to render on one
; or more scanlines.
;
; INSTRUCTION FORMAT:
;   Bits 0-3: Mode (graphics mode 0-15, or special instruction)
;   Bit 4:    HSCROL enable (horizontal fine scroll)
;   Bit 5:    VSCROL enable (vertical fine scroll)
;   Bit 6:    LMS (Load Memory Scan - next 2 bytes are screen address)
;   Bit 7:    DLI (trigger Display List Interrupt at end of this line)
;
; === WHY BLANK LINES INSTEAD OF GRAPHICS MODE LINES ===
; For plasma/raster bar effects where we only change COLBK (background
; colour), we do not need actual graphics mode lines. Mode 15 would require
; ANTIC to DMA 40 bytes per line from screen memory - that is 40 x 192
; = 7680 memory cycles per frame WASTED on unused graphics data.
;
; Using DL_BLANK1 instructions generates 1 blank line each with NO screen
; memory DMA. The CPU still changes COLBK via WSYNC. This reduces ANTIC's
; workload significantly and eliminates timing conflicts between ANTIC DMA
; and CPU memory accesses.
; ============================================================================

display_list:
        ; === Top Border: 24 blank scanlines ===
        db DL_BLANK8                    ; 8 blank lines  (scanlines 0-7)
        db DL_BLANK8                    ; 8 blank lines  (scanlines 8-15)
        db DL_BLANK8                    ; 8 blank lines  (scanlines 16-23)

        ; === Main Display Area: 192 blank scanlines ===
        ; Each DL_BLANK1 generates exactly 1 line with no DMA overhead.
        ; The CPU changes COLBK per-scanline via WSYNC.
%rep 192
        db DL_BLANK1
%endrep

        ; === Bottom Border: 24 blank scanlines ===
        db DL_BLANK8
        db DL_BLANK8
        db DL_BLANK8

        ; === Jump and Wait for Vertical Blank ===
        ; DL_JVB tells ANTIC to wait for the next VBlank, then restart
        ; the display list from the specified address. This creates an
        ; infinite loop that repeats every frame.
        db DL_JVB
        dw display_list

; ----------------------------------------------------------------------------
; Scroll State Variables
; ----------------------------------------------------------------------------

; Scroll position (16.16 fixed-point)
; Upper 16 bits = pixel position, lower 16 bits = sub-pixel fraction
scroll_y:       dd 0

; Pre-calculated values (computed once per frame during VBlank to avoid
; expensive DIV instructions in the per-scanline loop)
scroll_pixel:   dd 0                    ; Integer part of scroll_y
base_char_idx:  dd 0                    ; (scroll_pixel / 8) % message_len
cur_char_idx:   dd 0                    ; Current character index (per scanline)
cur_char_line:  dd 0                    ; Current row within character (0-7)

; ============================================================================
; SCROLL MESSAGE
; ============================================================================
; The text that scrolls vertically through the display.
; Leading/trailing spaces create pauses between repetitions.
; ============================================================================

scroll_message: db "    INTUITION ENGINE     386 ASM WITH ATARI ANTIC/GTIA PLASMA BARS AND VERTICAL HARDWARE SINUS SCROLL AND SID MUSIC     CODE BY INTUITION     ODDNB MUSIC BY JAMMER     GREETS TO ALL DEMOSCENERS    VISIT INTUITIONSUBSYNTH.COM    "
message_len:    equ $ - scroll_message

; ============================================================================
; SINE TABLE
; ============================================================================
; Pre-calculated sine wave values for one complete cycle (0-255 input).
; Output range: 0-255 (centred at 128)
;
; Formula: sin_table[i] = 128 + 127 * sin(2 * PI * i / 256)
;
; === WHY A LOOKUP TABLE ===
; On 8-bit systems, calculating sine in real-time was impossible. Even on
; x86, a lookup table is faster than FSIN for simple effects. The 256-entry
; table gives ~1.4 degree resolution - plenty for smooth animation.
;
; Usage: value = sin_table[(angle + offset) & 0xFF]
; Where angle is 0-255 representing 0-360 degrees.
; ============================================================================

sin_table:
        ; Quadrant 1: 0 to 90 degrees (indices 0-63) - rising from 128 to 255
        db 128, 131, 134, 137, 140, 143, 146, 149
        db 152, 155, 158, 162, 165, 167, 170, 173
        db 176, 179, 182, 185, 188, 190, 193, 196
        db 198, 201, 203, 206, 208, 211, 213, 215
        db 218, 220, 222, 224, 226, 228, 230, 232
        db 234, 235, 237, 238, 240, 241, 243, 244
        db 245, 246, 248, 249, 250, 250, 251, 252
        db 253, 253, 254, 254, 254, 255, 255, 255
        ; Quadrant 2: 90 to 180 degrees (indices 64-127) - falling from 255 to 128
        db 255, 255, 255, 255, 254, 254, 254, 253
        db 253, 252, 251, 250, 250, 249, 248, 246
        db 245, 244, 243, 241, 240, 238, 237, 235
        db 234, 232, 230, 228, 226, 224, 222, 220
        db 218, 215, 213, 211, 208, 206, 203, 201
        db 198, 196, 193, 190, 188, 185, 182, 179
        db 176, 173, 170, 167, 165, 162, 158, 155
        db 152, 149, 146, 143, 140, 137, 134, 131
        ; Quadrant 3: 180 to 270 degrees (indices 128-191) - falling from 128 to 0
        db 128, 124, 121, 118, 115, 112, 109, 106
        db 103, 100,  97,  93,  90,  88,  85,  82
        db  79,  76,  73,  70,  67,  65,  62,  59
        db  57,  54,  52,  49,  47,  44,  42,  40
        db  37,  35,  33,  31,  29,  27,  25,  23
        db  21,  20,  18,  17,  15,  14,  12,  11
        db  10,   9,   7,   6,   5,   5,   4,   3
        db   2,   2,   1,   1,   1,   0,   0,   0
        ; Quadrant 4: 270 to 360 degrees (indices 192-255) - rising from 0 to 128
        db   0,   0,   0,   0,   1,   1,   1,   2
        db   2,   3,   4,   5,   5,   6,   7,   9
        db  10,  11,  12,  14,  15,  17,  18,  20
        db  21,  23,  25,  27,  29,  31,  33,  35
        db  37,  40,  42,  44,  47,  49,  52,  54
        db  57,  59,  62,  65,  67,  70,  73,  76
        db  79,  82,  85,  88,  90,  93,  97, 100
        db 103, 106, 109, 112, 115, 118, 121, 124

; ============================================================================
; 8x8 BITMAP FONT
; ============================================================================
; Each character is 8 bytes (8 rows of 8 pixels).
; Bit 7 = leftmost pixel, bit 0 = rightmost pixel.
; 1 = foreground (player colour), 0 = transparent.
;
; Characters included: ASCII 32-95 (space through underscore)
; This covers uppercase letters, numbers, and common punctuation.
;
; Example: Letter 'A' (ASCII 65, font index 33)
;   Row 0: 00111000 = 0x38  (    ***   )
;   Row 1: 01101100 = 0x6C  (   ** **  )
;   Row 2: 11000110 = 0xC6  (  **   ** )
;   Row 3: 11111110 = 0xFE  (  ******* )
;   Row 4: 11000110 = 0xC6  (  **   ** )
;   Row 5: 11000110 = 0xC6  (  **   ** )
;   Row 6: 11000110 = 0xC6  (  **   ** )
;   Row 7: 00000000 = 0x00  (          )
; ============================================================================

font_data:
        ; ASCII 32: Space
        db 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
        ; ASCII 33: !
        db 0x18, 0x18, 0x18, 0x18, 0x18, 0x00, 0x18, 0x00
        ; ASCII 34: "
        db 0x6C, 0x6C, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00
        ; ASCII 35: #
        db 0x6C, 0x6C, 0xFE, 0x6C, 0xFE, 0x6C, 0x6C, 0x00
        ; ASCII 36: $
        db 0x18, 0x7E, 0xC0, 0x7C, 0x06, 0xFC, 0x18, 0x00
        ; ASCII 37: %
        db 0x00, 0xC6, 0xCC, 0x18, 0x30, 0x66, 0xC6, 0x00
        ; ASCII 38: &
        db 0x38, 0x6C, 0x38, 0x76, 0xDC, 0xCC, 0x76, 0x00
        ; ASCII 39: '
        db 0x18, 0x18, 0x30, 0x00, 0x00, 0x00, 0x00, 0x00
        ; ASCII 40: (
        db 0x0C, 0x18, 0x30, 0x30, 0x30, 0x18, 0x0C, 0x00
        ; ASCII 41: )
        db 0x30, 0x18, 0x0C, 0x0C, 0x0C, 0x18, 0x30, 0x00
        ; ASCII 42: *
        db 0x00, 0x66, 0x3C, 0xFF, 0x3C, 0x66, 0x00, 0x00
        ; ASCII 43: +
        db 0x00, 0x18, 0x18, 0x7E, 0x18, 0x18, 0x00, 0x00
        ; ASCII 44: ,
        db 0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x30
        ; ASCII 45: -
        db 0x00, 0x00, 0x00, 0x7E, 0x00, 0x00, 0x00, 0x00
        ; ASCII 46: .
        db 0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x18, 0x00
        ; ASCII 47: /
        db 0x06, 0x0C, 0x18, 0x30, 0x60, 0xC0, 0x80, 0x00
        ; ASCII 48-57: Digits 0-9
        db 0x7C, 0xC6, 0xCE, 0xD6, 0xE6, 0xC6, 0x7C, 0x00  ; 0
        db 0x18, 0x38, 0x18, 0x18, 0x18, 0x18, 0x7E, 0x00  ; 1
        db 0x7C, 0xC6, 0x06, 0x1C, 0x30, 0x66, 0xFE, 0x00  ; 2
        db 0x7C, 0xC6, 0x06, 0x3C, 0x06, 0xC6, 0x7C, 0x00  ; 3
        db 0x1C, 0x3C, 0x6C, 0xCC, 0xFE, 0x0C, 0x1E, 0x00  ; 4
        db 0xFE, 0xC0, 0xC0, 0xFC, 0x06, 0xC6, 0x7C, 0x00  ; 5
        db 0x38, 0x60, 0xC0, 0xFC, 0xC6, 0xC6, 0x7C, 0x00  ; 6
        db 0xFE, 0xC6, 0x0C, 0x18, 0x30, 0x30, 0x30, 0x00  ; 7
        db 0x7C, 0xC6, 0xC6, 0x7C, 0xC6, 0xC6, 0x7C, 0x00  ; 8
        db 0x7C, 0xC6, 0xC6, 0x7E, 0x06, 0x0C, 0x78, 0x00  ; 9
        ; ASCII 58: :
        db 0x00, 0x18, 0x18, 0x00, 0x00, 0x18, 0x18, 0x00
        ; ASCII 59: ;
        db 0x00, 0x18, 0x18, 0x00, 0x00, 0x18, 0x18, 0x30
        ; ASCII 60: <
        db 0x0C, 0x18, 0x30, 0x60, 0x30, 0x18, 0x0C, 0x00
        ; ASCII 61: =
        db 0x00, 0x00, 0x7E, 0x00, 0x00, 0x7E, 0x00, 0x00
        ; ASCII 62: >
        db 0x60, 0x30, 0x18, 0x0C, 0x18, 0x30, 0x60, 0x00
        ; ASCII 63: ?
        db 0x7C, 0xC6, 0x0C, 0x18, 0x18, 0x00, 0x18, 0x00
        ; ASCII 64: @
        db 0x7C, 0xC6, 0xDE, 0xDE, 0xDE, 0xC0, 0x78, 0x00
        ; ASCII 65-90: Uppercase letters A-Z
        db 0x38, 0x6C, 0xC6, 0xFE, 0xC6, 0xC6, 0xC6, 0x00  ; A
        db 0xFC, 0x66, 0x66, 0x7C, 0x66, 0x66, 0xFC, 0x00  ; B
        db 0x3C, 0x66, 0xC0, 0xC0, 0xC0, 0x66, 0x3C, 0x00  ; C
        db 0xF8, 0x6C, 0x66, 0x66, 0x66, 0x6C, 0xF8, 0x00  ; D
        db 0xFE, 0x62, 0x68, 0x78, 0x68, 0x62, 0xFE, 0x00  ; E
        db 0xFE, 0x62, 0x68, 0x78, 0x68, 0x60, 0xF0, 0x00  ; F
        db 0x3C, 0x66, 0xC0, 0xC0, 0xCE, 0x66, 0x3A, 0x00  ; G
        db 0xC6, 0xC6, 0xC6, 0xFE, 0xC6, 0xC6, 0xC6, 0x00  ; H
        db 0x3C, 0x18, 0x18, 0x18, 0x18, 0x18, 0x3C, 0x00  ; I
        db 0x1E, 0x0C, 0x0C, 0x0C, 0xCC, 0xCC, 0x78, 0x00  ; J
        db 0xE6, 0x66, 0x6C, 0x78, 0x6C, 0x66, 0xE6, 0x00  ; K
        db 0xF0, 0x60, 0x60, 0x60, 0x62, 0x66, 0xFE, 0x00  ; L
        db 0xC6, 0xEE, 0xFE, 0xFE, 0xD6, 0xC6, 0xC6, 0x00  ; M
        db 0xC6, 0xE6, 0xF6, 0xDE, 0xCE, 0xC6, 0xC6, 0x00  ; N
        db 0x7C, 0xC6, 0xC6, 0xC6, 0xC6, 0xC6, 0x7C, 0x00  ; O
        db 0xFC, 0x66, 0x66, 0x7C, 0x60, 0x60, 0xF0, 0x00  ; P
        db 0x7C, 0xC6, 0xC6, 0xC6, 0xD6, 0x7C, 0x0E, 0x00  ; Q
        db 0xFC, 0x66, 0x66, 0x7C, 0x6C, 0x66, 0xE6, 0x00  ; R
        db 0x7C, 0xC6, 0xE0, 0x78, 0x0E, 0xC6, 0x7C, 0x00  ; S
        db 0x7E, 0x7E, 0x5A, 0x18, 0x18, 0x18, 0x3C, 0x00  ; T
        db 0xC6, 0xC6, 0xC6, 0xC6, 0xC6, 0xC6, 0x7C, 0x00  ; U
        db 0xC6, 0xC6, 0xC6, 0xC6, 0x6C, 0x38, 0x10, 0x00  ; V
        db 0xC6, 0xC6, 0xD6, 0xFE, 0xFE, 0xEE, 0xC6, 0x00  ; W
        db 0xC6, 0xC6, 0x6C, 0x38, 0x6C, 0xC6, 0xC6, 0x00  ; X
        db 0x66, 0x66, 0x66, 0x3C, 0x18, 0x18, 0x3C, 0x00  ; Y
        db 0xFE, 0xC6, 0x8C, 0x18, 0x32, 0x66, 0xFE, 0x00  ; Z
        ; ASCII 91: [
        db 0x3C, 0x30, 0x30, 0x30, 0x30, 0x30, 0x3C, 0x00
        ; ASCII 92: backslash
        db 0xC0, 0x60, 0x30, 0x18, 0x0C, 0x06, 0x02, 0x00
        ; ASCII 93: ]
        db 0x3C, 0x0C, 0x0C, 0x0C, 0x0C, 0x0C, 0x3C, 0x00
        ; ASCII 94: ^
        db 0x10, 0x38, 0x6C, 0xC6, 0x00, 0x00, 0x00, 0x00
        ; ASCII 95: _
        db 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF

; ============================================================================
; SID MUSIC DATA
; ============================================================================
; "OdDnB" by Jammer (Kamil Wolnikowski) - a SID tune demonstrating
; the Commodore 64's distinctive sound chip capabilities.
;
; The SID file format contains header metadata, load address,
; initialisation routines, and music data (patterns, instruments,
; sequences). The IntuitionEngine's SID player handles all the
; complexity; we just provide the file data and it plays automatically.
; ============================================================================

        align 4
sid_data:
        incbin "../assets/music/OdDnB.sid"
sid_data_end:
