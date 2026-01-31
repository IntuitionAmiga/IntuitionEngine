; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==        TWISTING STARFIELD TUNNEL WITH RAINBOW BITMAP SCROLLTEXT        ==
; ==                                                                        ==
; ==            IE32 Assembly for IntuitionEngine / 3DFX Voodoo SST-1       ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
;                           ╭─────────────────╮
;                          ╱    *     *    *   ╲
;                         │   *   ★      *     │
;                        │  *    *    ★    *   │
;       ╭───────────────│──────────────────────│───────────────╮
;       │               │ *   STARFIELD   *    │               │
;       │  *       *    │     TUNNEL     *     │    *      *   │
;       │    *          │  *     ★    *        │         *     │
;       │       *   *  ╱   *          *    *    ╲  *    *      │
;       │  *          ╱       *    *      *      ╲         *   │
;       │     *      ╱    *         *         *   ╲    *       │
;       │         * ╱  *      *         *    *     ╲ *         │
;       │    *      ───────────────────────────────      *     │
;       │                                                      │
;       │   ▄▄▄▄▄ ▄▄▄▄▄ ▄▄▄▄▄ ▄   ▄ ▄▄▄▄▄ ▄▄▄▄▄ ▄▄▄▄▄ ▄▄▄▄▄   │
;       │     █     █     █   █   █   █     █   █   █ █   █   │
;       │     █     █     █   █   █   █     █   █   █ █   █   │
;       │   ╲ Rainbow Scrolling Text (Sine Wave Wobble) ╱      │
;       │                                                      │
;       ╰──────────────────────────────────────────────────────╯
;
; ============================================================================
; REFERENCE IMPLEMENTATION FOR DEMOSCENE PROGRAMMING
; ============================================================================
;
; This file is HEAVILY COMMENTED to serve as a teaching resource for:
;   - 3D starfield/tunnel rendering techniques
;   - Hardware-accelerated triangle rendering (3DFX Voodoo)
;   - Bitmap font scrolltext with per-character effects
;   - Fixed-point math on unsigned-only architectures
;   - SID music playback through 6502 CPU core
;
; Reading time: ~45 minutes for thorough understanding
;
; ============================================================================
; TABLE OF CONTENTS
; ============================================================================
;
;   Line    Section
;   ----    -------
;   ~100    Historical Context & Why These Effects Matter
;   ~200    Architecture Overview (ASCII Art Diagrams)
;   ~350    Constants & Configuration
;   ~450    Memory Map
;   ~550    Program Entry & Hardware Initialization
;   ~650    Main Loop (Frame Rendering)
;   ~900    Star Processing (Update, Project, Draw)
;   ~1150   Scrolltext Rendering (Bitmap Font)
;   ~1450   Subroutines (Sine Table, Random, Font Lookup)
;   ~1600   Data Section (Sine Table, Font Data, SID Music)
;
; ============================================================================
; WHAT THIS DEMO DOES
; ============================================================================
;
;   1. TWISTING STARFIELD TUNNEL
;      - 256 stars distributed in a cylindrical pattern
;      - Stars rush toward the camera (Z movement)
;      - Tunnel center oscillates via sine waves (the "twist")
;      - Depth-based coloring: white (close) → cyan → blue (far)
;      - Creates a "hyperspace warp" visual effect
;
;   2. RAINBOW BITMAP SCROLLTEXT
;      - Classic 5x7 pixel bitmap font (64 characters)
;      - Characters rendered as individual colored quads
;      - Horizontal scrolling with Y-axis sine wave wobble
;      - Per-character rainbow color cycling
;      - Floats in front of the starfield
;
;   3. SID MUSIC PLAYBACK
;      - "Reggae 2" by Djinn/Fraction (1998)
;      - Real 6502 code executed on internal CPU core
;      - Authentic Commodore 64 sound synthesis
;
; ============================================================================
; WHY THESE EFFECTS MATTER (HISTORICAL CONTEXT)
; ============================================================================
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                     THE STARFIELD TUNNEL EFFECT                         │
; └─────────────────────────────────────────────────────────────────────────┘
;
; Starfield effects were THE classic demoscene effect since the 1980s.
; The evolution went like this:
;
;   1985-1988: 2D STARFIELDS
;   Points moving outward from screen center. Simple but effective.
;   Formula: screen_pos = (star_pos - center) * speed / distance + center
;
;             *      *                    *   *
;         *       .  *   *            *    .     *
;       *    *  . . .    *    ===>  *   . . . .   *
;         *   . . o . .  *            . . . o . .
;       *    . . . . .   *          *  . . . . . *
;           *  . .  * *              *   . .  *
;             *    *                   *     *
;
;   1988-1992: 3D STARFIELDS
;   True Z-depth with perspective projection. Stars have X, Y, Z coords.
;   More realistic depth perception.
;
;   1992-1996: TUNNEL EFFECTS
;   Stars constrained to a cylindrical or toroidal shape. The viewer
;   appears to fly through an infinite tunnel. Often combined with:
;   - Tunnel center movement (twisting)
;   - Texture-mapped walls
;   - Plasma/fire effects inside
;
;   1996-2000: HARDWARE ACCELERATION
;   With 3DFX Voodoo, tunnels could have textured, lit, fog-enhanced
;   geometry at high framerates. The demoscene adapted quickly.
;
; This demo implements a TWISTING TUNNEL - the center point oscillates
; using sine waves with different frequencies for X and Y, creating a
; Lissajous-like flight path that feels organic and hypnotic.
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE 5x7 BITMAP FONT SCROLLTEXT                       │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The scrolling text ("scroller") was ubiquitous in demos from 1985 onwards.
; Nearly every demo had credits and greetings scrolling across the screen.
;
; WHY 5x7 PIXELS?
; This was the standard "small" font size that balanced readability with
; efficiency. Larger fonts (8x8, 8x16) used more memory and CPU time.
; The 5x7 format:
;   - 5 pixels wide (fits in one byte, with 3 bits spare)
;   - 7 pixels tall (readable at small sizes)
;   - 7 bytes per character × 64 characters = 448 bytes total
;
; Character "A" in 5x7:
;
;   Row 0:  . # # # .   =  0x0E  =  01110
;   Row 1:  # . . . #   =  0x11  =  10001
;   Row 2:  # . . . #   =  0x11  =  10001
;   Row 3:  # # # # #   =  0x1F  =  11111
;   Row 4:  # . . . #   =  0x11  =  10001
;   Row 5:  # . . . #   =  0x11  =  10001
;   Row 6:  # . . . #   =  0x11  =  10001
;
; THE SINE WAVE WOBBLE
; A straight horizontal scroller is boring. Adding a sine wave to the
; Y position creates the classic "wavy" demoscene look:
;
;       ╭─╮     ╭─╮     ╭─╮
;      H   E   L   L   O
;           ╰─╯     ╰─╯
;
; Each character's Y offset = sin(x_position + frame_counter) * amplitude
;
; RAINBOW COLOR CYCLING
; Per-character color creates a flowing rainbow effect. We use phase-
; shifted sine waves for each RGB channel:
;
;   Red   = sin(angle + 0°)   × intensity
;   Green = sin(angle + 120°) × intensity
;   Blue  = sin(angle + 240°) × intensity
;
; As angle increases (per-character or per-frame), the color cycles
; through the spectrum: Red → Yellow → Green → Cyan → Blue → Magenta → Red
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE 3DFX VOODOO GRAPHICS (1996)                      │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The 3DFX Voodoo was REVOLUTIONARY. Before it, 3D games either:
;   - Used software rendering (slow, limited resolution)
;   - Required expensive SGI workstations ($50,000+)
;
; The Voodoo brought hardware 3D to consumers for ~$300:
;   - 50 million textured pixels per second
;   - Hardware Z-buffering (no more sorting polygons!)
;   - Bilinear texture filtering
;   - Gouraud shading
;   - Alpha blending and fog
;
; HOW WE USE IT:
; The Intuition Engine emulates the Voodoo's register interface.
; To draw a triangle, we:
;   1. Write vertex coordinates to VOODOO_VERTEX_AX/AY, BX/BY, CX/CY
;   2. Write color to VOODOO_START_R/G/B/A
;   3. Write to VOODOO_TRIANGLE_CMD to submit
;
; The GPU handles rasterization, depth testing, and framebuffer writes.
; Much faster than software pixel-by-pixel drawing!
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE IE32 CPU ARCHITECTURE                            │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The IE32 is a custom 32-bit RISC CPU with these characteristics:
;
;   REGISTERS:
;   - A, B: General-purpose 32-bit accumulators
;   - X, Y: Index/pointer registers
;   - SP: Stack pointer
;   - PC: Program counter
;
;   KEY LIMITATION: UNSIGNED ONLY!
;   All arithmetic treats values as unsigned 32-bit integers.
;   This creates challenges for 3D graphics where coordinates can be
;   negative (to the left of center, above center, etc.).
;
;   See "THE UNSIGNED ARITHMETIC CHALLENGE" below for our solution.
;
; ============================================================================
; ARCHITECTURE OVERVIEW
; ============================================================================
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    SYSTEM ARCHITECTURE                                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │                         INTUITION ENGINE                            │
;   │                                                                     │
;   │  ┌─────────────┐    ┌─────────────┐    ┌─────────────────────────┐ │
;   │  │   IE32      │    │   6502      │    │    VOODOO SST-1         │ │
;   │  │   CPU       │    │   CPU       │    │    GPU                  │ │
;   │  │  (Demo)     │    │ (SID Music) │    │ (Triangle Rendering)    │ │
;   │  └──────┬──────┘    └──────┬──────┘    └───────────┬─────────────┘ │
;   │         │                  │                       │               │
;   │         │   Memory Bus     │                       │               │
;   │         └──────────────────┴───────────────────────┘               │
;   │                            │                                       │
;   │  ┌─────────────────────────┴───────────────────────────────────┐   │
;   │  │                    MEMORY MAP                               │   │
;   │  │  $0000-$0FFF: Stack/Reserved                                │   │
;   │  │  $1000-$8FFF: Program Code                                  │   │
;   │  │  $10000-$1FFFF: Sine Tables                                 │   │
;   │  │  $20000-$2FFFF: Star Data                                   │   │
;   │  │  $0F0000+: Hardware I/O (Voodoo, SID Player)                │   │
;   │  └─────────────────────────────────────────────────────────────┘   │
;   │                                                                     │
;   │  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐            │
;   │  │ SID Engine  │    │ Framebuffer │    │ Audio Out   │            │
;   │  │ (Synth)     │--->│ (Display)   │    │ (Speakers)  │            │
;   │  └─────────────┘    └─────────────┘    └─────────────┘            │
;   └─────────────────────────────────────────────────────────────────────┘
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    MAIN LOOP FLOW (~60 FPS)                             │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   ╔═══════════════════════════════════════════════════════════════════╗
;   ║                        FRAME START                                ║
;   ╚═══════════════════════════════════════════════════════════════════╝
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 1. CLEAR FRAMEBUFFER                                              │
;   │    Write dark blue (0xFF040410) to VOODOO_COLOR0                  │
;   │    Trigger VOODOO_FAST_FILL_CMD                                   │
;   │    (Hardware clears entire screen in one operation)               │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 2. CALCULATE TUNNEL TWIST                                         │
;   │    twist_x = sin(frame * 1) * TWIST_AMP / 64 - center             │
;   │    twist_y = sin(frame * 1.5 + 64) * TWIST_AMP / 64 - center      │
;   │    (Different frequencies create Lissajous-like motion)           │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 3. PROCESS ALL 256 STARS (Loop)                                   │
;   │    ┌─────────────────────────────────────────────────────────┐    │
;   │    │ 3a. Load star data (angle, radius, z, speed)            │    │
;   │    └─────────────────────────────────────────────────────────┘    │
;   │                            │                                      │
;   │                            ▼                                      │
;   │    ┌─────────────────────────────────────────────────────────┐    │
;   │    │ 3b. Update Z position: z -= speed (move toward camera)  │    │
;   │    │     If z < NEAR_PLANE: respawn at FAR_PLANE             │    │
;   │    └─────────────────────────────────────────────────────────┘    │
;   │                            │                                      │
;   │                            ▼                                      │
;   │    ┌─────────────────────────────────────────────────────────┐    │
;   │    │ 3c. 3D → 2D Projection                                  │    │
;   │    │     local_x = cos(angle) * radius + WORLD_OFFSET        │    │
;   │    │     local_y = sin(angle) * radius + WORLD_OFFSET        │    │
;   │    │     world_x = local_x + twist_x                         │    │
;   │    │     world_y = local_y + twist_y                         │    │
;   │    │     screen_x = CENTER + world_x * FOCAL / z - offset    │    │
;   │    │     screen_y = CENTER + world_y * FOCAL / z - offset    │    │
;   │    └─────────────────────────────────────────────────────────┘    │
;   │                            │                                      │
;   │                            ▼                                      │
;   │    ┌─────────────────────────────────────────────────────────┐    │
;   │    │ 3d. Render star triangle                                │    │
;   │    │     Size = 5000 / z (closer = bigger)                   │    │
;   │    │     Color based on depth (white/cyan/blue)              │    │
;   │    │     Submit 3 vertices to Voodoo GPU                     │    │
;   │    └─────────────────────────────────────────────────────────┘    │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 4. DRAW SCROLLTEXT (JSR draw_scrolltext)                          │
;   │    For each visible character (24 max):                           │
;   │    ├─ Calculate X position from scroll_offset                     │
;   │    ├─ Calculate Y position with sine wave wobble                  │
;   │    ├─ Set rainbow color (phase-shifted sine waves)                │
;   │    ├─ Look up 5x7 font bitmap                                     │
;   │    └─ Draw each "on" pixel as a quad (2 triangles)                │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 5. SWAP BUFFERS                                                   │
;   │    Write to VOODOO_SWAP_BUFFER_CMD                                │
;   │    (Display rendered frame, get fresh back buffer)                │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 6. UPDATE ANIMATION STATE                                         │
;   │    frame_counter++  (for twist animation)                         │
;   │    scroll_offset += SCROLL_SPEED (for text scroll)                │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ╔═══════════════════════════════════════════════════════════════════╗
;   ║                    LOOP TO FRAME START                            ║
;   ╚═══════════════════════════════════════════════════════════════════╝
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE VOODOO RENDERING PIPELINE                        │
; └─────────────────────────────────────────────────────────────────────────┘
;
; To draw a single triangle, we write to these memory-mapped registers:
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │ STEP 1: SET TRIANGLE COLOR (Flat shading - same color for all verts)│
;   └─────────────────────────────────────────────────────────────────────┘
;
;   VOODOO_START_R ← Red   (4.12 fixed-point, $1000 = 1.0)
;   VOODOO_START_G ← Green (4.12 fixed-point, $1000 = 1.0)
;   VOODOO_START_B ← Blue  (4.12 fixed-point, $1000 = 1.0)
;   VOODOO_START_A ← Alpha (4.12 fixed-point, $1000 = 1.0)
;   VOODOO_START_Z ← Z depth for z-buffer test
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │ STEP 2: SET TRIANGLE VERTICES (12.4 fixed-point coordinates)       │
;   └─────────────────────────────────────────────────────────────────────┘
;
;             A (ax, ay)
;            /\
;           /  \
;          /    \
;         /      \
;        /________\
;   B (bx, by)    C (cx, cy)
;
;   VOODOO_VERTEX_AX ← (pixel_x * 16)    ; 12.4 format
;   VOODOO_VERTEX_AY ← (pixel_y * 16)
;   VOODOO_VERTEX_BX ← ...
;   VOODOO_VERTEX_BY ← ...
;   VOODOO_VERTEX_CX ← ...
;   VOODOO_VERTEX_CY ← ...
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │ STEP 3: SUBMIT TRIANGLE                                            │
;   └─────────────────────────────────────────────────────────────────────┘
;
;   VOODOO_TRIANGLE_CMD ← 0  (any write triggers rendering)
;
;   The GPU then:
;   ├─ Computes edge equations
;   ├─ Iterates over pixels inside triangle
;   ├─ Tests Z-buffer (skip if occluded)
;   ├─ Writes color to framebuffer
;   └─ Updates Z-buffer
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │ FIXED-POINT FORMAT EXPLANATION                                     │
;   └─────────────────────────────────────────────────────────────────────┘
;
;   12.4 FORMAT (Vertex Coordinates):
;   ┌─────────────────────────────────────────────────────────────────┐
;   │ Bit 31                                                    Bit 0 │
;   │ [unused: 16 bits] [integer: 12 bits] [fraction: 4 bits]         │
;   │                   ├─ 0-4095 pixels ─┤├─ 0.0625 step ─┤          │
;   └─────────────────────────────────────────────────────────────────┘
;
;   Example: Pixel 320 → 320 * 16 = 5120 = 0x1400
;
;   4.12 FORMAT (Colors):
;   ┌─────────────────────────────────────────────────────────────────┐
;   │ Bit 31                                                    Bit 0 │
;   │ [unused: 16 bits] [integer: 4 bits] [fraction: 12 bits]         │
;   │                   ├─ 0-15 ─┤├─ 0.000244 step ──────────┤        │
;   └─────────────────────────────────────────────────────────────────┘
;
;   Example: 1.0 intensity → 0x1000, 0.5 intensity → 0x0800
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                THE UNSIGNED ARITHMETIC CHALLENGE                        │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The IE32 CPU uses UNSIGNED 32-bit integers exclusively. This creates
; serious problems for 3D graphics where coordinates can be negative.
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │ THE PROBLEM                                                             │
; └─────────────────────────────────────────────────────────────────────────┘
;
; In 3D graphics, we need coordinates relative to screen center:
;
;           │ ← screen_x = -200 (left of center)
;           │
;     ──────┼────── CENTER (320, 240)
;           │
;           │ → screen_x = +150 (right of center)
;
; When cosine returns a negative value (angles 90°-270°), and we multiply
; by radius, we get negative world coordinates. For example:
;
;   cos(180°) = -1  →  local_x = -1 * 100 = -100
;
; But in UNSIGNED arithmetic:
;   -100 stored as unsigned = 0xFFFFFF9C = 4,294,967,196
;
; Then when we divide: 4,294,967,196 / 500 = 8,589,934  (WRONG!)
; We wanted: -100 / 500 = 0 (approximately)
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │ THE SOLUTION: OFFSET-BASED COORDINATES                                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
; We add a large positive offset (WORLD_OFFSET = 600) to ALL coordinates
; before doing any math. This keeps everything positive:
;
;   Original range: -300 to +300  (can go negative!)
;   With offset:    300 to 900    (always positive!)
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │                                                                     │
;   │ ─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────┬─────        │
;   │    -300        0      +300      600       900                       │
;   │      │←── original ──→│        │←── with offset ──→│               │
;   │           range                      range                          │
;   │                                                                     │
;   └─────────────────────────────────────────────────────────────────────┘
;
; THE OFFSET CORRECTION PROCESS:
;
;   1. local_x = cos(angle) * radius / 64 + WORLD_OFFSET
;      (Value is now guaranteed positive for unsigned math)
;
;   2. proj_x = local_x * FOCAL / z
;      (Safe unsigned division because local_x > 0)
;
;   3. offset_proj = WORLD_OFFSET * FOCAL / z
;      (Calculate where pure offset would project to)
;
;   4. screen_x = CENTER + proj_x - offset_proj
;      (Subtract the offset's contribution)
;
; WHY THIS WORKS:
;
;   proj_x = (value + offset) * focal / z
;          = value * focal / z + offset * focal / z
;          = true_projection + offset_projection
;
;   screen_x = proj_x - offset_proj + CENTER
;            = true_projection + offset_proj - offset_proj + CENTER
;            = true_projection + CENTER  ✓
;
; This technique is explained in detail at each relevant code section.
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │ THE SINE TABLE ENCODING                                                 │
; └─────────────────────────────────────────────────────────────────────────┘
;
; Our sine table stores values 0-254, where 127 represents zero:
;
;   sin_table[angle] = sin(angle * 2π / 256) * 127 + 127
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │  254 ─────╮                         ╭─────                          │
;   │           ╲                       ╱                                 │
;   │            ╲                     ╱                                  │
;   │  127 ──────┼──────────────────┼────── (zero crossing)              │
;   │            ╱                     ╲                                  │
;   │           ╱                       ╲                                 │
;   │    0 ────╯                         ╰─────                          │
;   │        0       64      128      192     255  (angle)               │
;   │        0°      90°     180°     270°    360°                       │
;   └─────────────────────────────────────────────────────────────────────┘
;
; When we multiply sin_table[angle] by radius, we get:
;   - angle 0°:   127 * radius  (zero contribution)
;   - angle 90°:  254 * radius  (positive peak)
;   - angle 180°: 127 * radius  (zero crossing)
;   - angle 270°: 0 * radius    (negative peak... but it's just 0!)
;
; The WORLD_OFFSET handles the "negative" values by making everything
; relative to offset + 127*radius, which we subtract after projection.
;
; ============================================================================

.include "ie32.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Voodoo Display Configuration ---
; The Voodoo supports various resolutions. We use 640x480, the most common
; mode for Voodoo demos. The VIDEO_DIM register encodes width in the upper
; 16 bits and height in the lower 16 bits.
.equ SCREEN_W       640             ; Horizontal resolution
.equ SCREEN_H       480             ; Vertical resolution
.equ CENTER_X       320             ; Screen center X (640/2)
.equ CENTER_Y       240             ; Screen center Y (480/2)

; --- Fixed-Point Configuration ---
; Voodoo uses 12.4 fixed-point for vertex coordinates.
; This means: actual_value = stored_value / 16
; To convert integer pixels to 12.4: multiply by 16 (shift left 4)
.equ FP_SHIFT       4               ; Bits of fractional precision

; --- Starfield Parameters ---
; These control the density and behavior of the starfield.
.equ NUM_STARS      256             ; Number of stars in the tunnel
                                    ; More stars = denser field, more CPU
                                    ; 256 is a good balance for 60fps

; --- Tunnel Geometry ---
; The stars are distributed in a cylinder. TUNNEL_RADIUS controls how
; far from the center axis stars can spawn.
.equ TUNNEL_RADIUS  150             ; Maximum distance from tunnel center
                                    ; Larger = wider tunnel, more spread

; --- Depth (Z) Parameters ---
; Stars move from FAR_PLANE toward the camera at NEAR_PLANE.
; When a star reaches NEAR_PLANE, it "respawns" at FAR_PLANE.
.equ NEAR_PLANE     80              ; Respawn threshold (close to camera)
.equ FAR_PLANE      1000            ; Initial spawn distance (far away)
.equ FOCAL_LENGTH   200             ; Perspective projection focal length
                                    ; Larger = less extreme perspective
                                    ; Smaller = more "fisheye" distortion

; --- Twist Animation ---
; The tunnel center oscillates using sine waves. TWIST_AMP controls
; how far the center moves from the screen center.
.equ TWIST_AMP      120             ; Amplitude of tunnel center movement
                                    ; Larger = more dramatic swooping
                                    ; 120 gives good visible motion

; --- Unsigned Math Workaround ---
; WORLD_OFFSET keeps all coordinates positive during calculation.
; Must be larger than any possible negative coordinate value.
; With TUNNEL_RADIUS=150 and TWIST_AMP=120, max negative offset is ~270,
; so 600 gives comfortable headroom.
.equ WORLD_OFFSET   600             ; Offset to keep world coords positive

; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    SCROLLTEXT PARAMETERS                                │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The scrolltext renders using a 5x7 bitmap font, where each "on" pixel
; is drawn as a small colored quad (2 triangles).
;
;   CHARACTER STRUCTURE:
;   ┌───┬───┬───┬───┬───┬───┐
;   │ 5 pixels      │ 1 │    ← CHAR_WIDTH = 6 (5 content + 1 spacing)
;   ├───┴───┴───┴───┴───┼───┤
;   │  ████████████     │   │    ← Each pixel is PIXEL_SIZE × PIXEL_SIZE
;   │  ████████████     │   │      on screen (4x4 pixels = 16 screen pixels)
;   │  ██          ██   │   │
;   │  ████████████     │   │    ← CHAR_HEIGHT = 7 rows
;   │  ██          ██   │   │
;   │  ██          ██   │   │
;   │  ██          ██   │   │
;   └───────────────────┴───┘
;   │← 6 × 4 = 24 pixels →│
;
;   SCREEN LAYOUT:
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │                           STARFIELD                                 │
;   │                                                                     │
;   │                                                                     │
;   │  ←─────────────── 640 pixels ───────────────→                      │
;   │                                                                     │
;   │  ┌─────────────────────────────────────────────────────────────┐   │
;   │  │        SCROLLTEXT AREA (Y = 340)                            │   │
;   │  │  ╭─╮     ╭─╮     ╭─╮                                        │   │
;   │  │ I   N   T   U   I   T   I   O   N  ← Sine wave wobble       │   │
;   │  │      ╰─╯     ╰─╯     ╰─╯                                    │   │
;   │  └─────────────────────────────────────────────────────────────┘   │
;   │                                                                     │
;   └─────────────────────────────────────────────────────────────────────┘
;
.equ SCROLL_SPEED   2               ; Pixels per frame (higher = faster scroll)
                                    ; At 60fps, 2 pixels/frame = 120 pixels/sec
                                    ; Full message (70 chars × 24 px) scrolls in ~14 sec

.equ CHAR_WIDTH     6               ; Character width in font pixels (5 + 1 spacing)
                                    ; The extra pixel provides inter-character gap

.equ CHAR_HEIGHT    7               ; Character height in font pixels
                                    ; 7 rows is minimum for readable lowercase

.equ PIXEL_SIZE     4               ; Screen pixels per font pixel
                                    ; Font pixel → 4×4 screen pixel block
                                    ; Total char size: 24×28 screen pixels

.equ SCROLL_Y_POS   340             ; Y position on screen (pixels from top)
                                    ; 340 puts text in lower portion of 480-high screen

.equ WOBBLE_AMP     50              ; Sine wave amplitude for Y wobble (pixels)
                                    ; Characters oscillate ±40 pixels vertically

.equ MAX_CHARS      24              ; Maximum characters visible on screen
                                    ; 640 / 24 ≈ 26, so 24 is safe with margin

; ============================================================================
; MEMORY MAP
; ============================================================================
; The IE32 memory layout for this demo:
;
;   $0000-$0FFF     Reserved / Stack
;   $1000-$87FF     Program code
;   $8800-$88FF     Runtime variables (detailed below)
;   $10000-$103FF   Sine lookup table (256 entries * 4 bytes)
;   $20000-$20FFF   Star data array (256 stars * 16 bytes)
;
; --- Runtime Variables ($8800-$88FF) ---
; These are working variables used during frame rendering.
.equ frame_counter  0x8800          ; Animation frame number (increments each frame)
.equ rand_seed      0x8804          ; Random number generator state
.equ twist_x        0x8808          ; Current X offset of tunnel center
.equ twist_y        0x880C          ; Current Y offset of tunnel center
.equ cur_z          0x8810          ; Current star's Z depth (temp)
.equ offset_proj    0x8814          ; Projected offset for coordinate correction

; --- Per-Star Temporary Variables ---
; Used during processing of each individual star.
; (Could use stack, but direct addressing is clearer for teaching)
; $8820 = angle      Star's position around tunnel circumference
; $8824 = radius     Star's distance from tunnel axis
; $8828 = z          Star's depth (distance from camera)
; $882C = speed      How fast star moves toward camera
; $8830 = cos_val    Cosine of star's angle
; $8834 = sin_val    Sine of star's angle
; $8838 = local_x_pos  Local X before twist (with offset)
; $883C = local_y_pos  Local Y before twist (with offset)
; $8848 = proj_x     X after perspective projection
; $884C = proj_y     Y after perspective projection
; $8850 = screen_x   Final screen X coordinate
; $8854 = screen_y   Final screen Y coordinate
; $8858 = star_size  Size of star based on depth
; $8870 = quadrant   Used in sine table generation

; --- Scrolltext Runtime Variables ($8880-$88DF) ---
.equ scroll_offset    0x8880        ; Current scroll X position (pixels)
.equ scroll_char_num  0x8884        ; Current character number in message
.equ scroll_char_code 0x8888        ; ASCII code of current character
.equ scroll_pixel_x   0x888C        ; Current pixel X within character
.equ scroll_pixel_y   0x8890        ; Current pixel Y within character
.equ scroll_screen_x  0x8894        ; Screen X for current pixel
.equ scroll_screen_y  0x8898        ; Screen Y for current pixel
.equ scroll_font_ptr  0x889C        ; Pointer to font data for character
.equ scroll_font_row  0x88A0        ; Current font row data
.equ scroll_base_x    0x88A4        ; Base X for current character
.equ scroll_base_y    0x88A8        ; Base Y for current character (with wobble)
.equ scroll_msg_len   0x88AC        ; Length of scroll message

; --- Lookup Tables ---
.equ sin_table      0x10000         ; 256-entry sine table, values 0-254
                                    ; (127 = zero, 0 = -1, 254 = +1)

; --- Star Data Array ---
; Each star has 16 bytes of data (4 longwords):
;   +0:  angle    Position around tunnel (0-255, like degrees)
;   +4:  radius   Distance from tunnel center axis
;   +8:  z        Depth into screen (larger = farther)
;   +12: speed    Movement rate toward camera per frame
.equ star_array     0x20000

; --- Embedded SID Music ---
; Size of the Reggae 2 SID file in bytes
.equ sid_data_size  4790

; ============================================================================
; PROGRAM ENTRY POINT
; ============================================================================
.org 0x1000

start:
    ; ========================================================================
    ; INITIALIZE VOODOO GRAPHICS HARDWARE
    ; ========================================================================
    ; The Voodoo requires configuration before rendering.
    ;
    ; VOODOO_VIDEO_DIM: Sets resolution as (width << 20) | height
    ;   640 << 20 = 0x02800000
    ;   0x02800000 | 480 = 0x028001E0
    ;
    ; VOODOO_FBZ_MODE: Configures framebuffer/Z-buffer behavior
    ;   0x0670 enables RGB write, depth test, and double buffering
    ; ========================================================================
    LDA #0x02800000                 ; Width 640 in upper bits
    OR A, #0x1E0                    ; Height 480 (0x1E0) in lower bits
    STA @VOODOO_VIDEO_DIM           ; Configure display dimensions

    LDA #0x0670                     ; Enable depth, RGB write, dithering
    STA @VOODOO_FBZ_MODE            ; Set framebuffer mode

    ; ========================================================================
    ; INITIALIZE RUNTIME STATE
    ; ========================================================================
    ; Zero the frame counter and seed the random number generator.
    ; The RNG seed can be any non-zero value; 54321 is arbitrary but tested.
    ; ========================================================================
    LDA #0
    STA @frame_counter              ; Start at frame 0
    STA @scroll_offset              ; Start scroll at 0

    LDA #54321                      ; Arbitrary seed for RNG
    STA @rand_seed                  ; Initialize random state

    ; ========================================================================
    ; BUILD SINE LOOKUP TABLE
    ; ========================================================================
    ; Pre-computing sine values avoids expensive trig math every frame.
    ; We build a 256-entry table where:
    ;   sin_table[angle] = sin(angle * 2π / 256) * 127 + 127
    ;
    ; This maps the sine wave to 0-254, with 127 representing zero.
    ; See build_sin_table subroutine for the quarter-wave algorithm.
    ; ========================================================================
    JSR build_sin_table

    ; ========================================================================
    ; INITIALIZE STAR POSITIONS
    ; ========================================================================
    ; Scatter stars randomly throughout the tunnel volume.
    ; Each star gets a random angle, radius, depth, and speed.
    ; ========================================================================
    JSR init_stars

    ; ========================================================================
    ; START SID MUSIC PLAYBACK
    ; ========================================================================
    ; Initialize and start the SID music player with looping enabled.
    ;
    ; === WHAT IS SID MUSIC? ===
    ; SID (Sound Interface Device) was the legendary audio chip in the
    ; Commodore 64. A .SID file contains actual 6502 machine code that
    ; drives the SID chip registers. The Intuition Engine runs this code
    ; on its internal 6502 CPU core, with SID register writes remapped
    ; to the native synthesizer engine.
    ;
    ; This means authentic C64 music plays without modification!
    ;
    ; === REGGAE 2 ===
    ; Reggae 2 (1998) by Kamil Degorski (Djinn) of Fraction is a classic
    ; C64 SID tune with that distinctive reggae-influenced demoscene style
    ; from the late 90s golden era.
    ;
    ; === SID PLAYER REGISTERS ===
    ; SID_PLAY_PTR:  32-bit pointer to .SID file data
    ; SID_PLAY_LEN:  32-bit length of .SID file in bytes
    ; SID_PLAY_CTRL: Control register
    ;                Bit 0 (0x01): Start playback
    ;                Bit 1 (0x02): Stop playback
    ;                Bit 2 (0x04): Enable looping
    ;
    ; We use 0x05 (start + loop) for continuous music during the demo.
    ; ========================================================================
    LDA #sid_data                   ; Address of embedded SID file
    STA @SID_PLAY_PTR               ; Set playback pointer

    LDA #sid_data_size              ; SID file size in bytes (calculated by assembler)
    STA @SID_PLAY_LEN               ; Set playback length

    LDA #0x05                       ; Start (bit 0) + Loop (bit 2)
    STA @SID_PLAY_CTRL              ; Begin music playback!

; ============================================================================
; MAIN LOOP
; ============================================================================
; This loop runs continuously, rendering one frame per iteration.
; At 60fps, we have ~16.6ms per frame for all processing.
;
; Frame structure:
;   1. Clear the framebuffer (dark blue background)
;   2. Calculate tunnel twist from frame counter
;   3. For each star: update position, project 3D->2D, draw triangle
;   4. Swap front/back buffers (display what we just rendered)
;   5. Increment frame counter
; ============================================================================
main_loop:
    ; ========================================================================
    ; CLEAR FRAMEBUFFER
    ; ========================================================================
    ; The Voodoo has a hardware fast-fill command that clears the entire
    ; framebuffer in one operation. Much faster than drawing a fullscreen quad!
    ;
    ; COLOR0 sets the clear color in ARGB8888 format:
    ;   0xFF040410 = fully opaque (FF), R=4, G=4, B=16
    ;   This creates a very dark blue, suggesting deep space.
    ; ========================================================================
    LDA #0xFF040410                 ; ARGB: nearly black with blue tint
    STA @VOODOO_COLOR0              ; Set clear color

    LDA #0
    STA @VOODOO_FAST_FILL_CMD       ; Execute clear (any value triggers)

    ; ========================================================================
    ; CALCULATE TUNNEL TWIST (X AXIS)
    ; ========================================================================
    ; The tunnel center oscillates using sine waves. We use the frame
    ; counter as the angle input, creating smooth animation.
    ;
    ; Formula: twist_x = (sin[frame] - 127) * TWIST_AMP / 64
    ;
    ; Breaking it down:
    ;   1. sin[frame & 255] gives 0-254 (127 = zero crossing)
    ;   2. Multiply by TWIST_AMP (120) to scale amplitude
    ;   3. Divide by 64 (SHR 6) to normalize
    ;   4. Subtract 238 to center around zero
    ;      (238 = 127 * 120 / 64, the offset introduced by step 1-3)
    ;
    ; Result: twist_x ranges approximately from -238 to +238
    ; ========================================================================
    LDA @frame_counter
    AND A, #255                     ; Use low 8 bits as angle (0-255)
    JSR get_sin                     ; Look up sine value (0-254)
    MUL A, #TWIST_AMP               ; Scale by amplitude (0 to 30480)
    SHR A, #6                       ; Divide by 64 (0 to 476)
    SUB A, #238                     ; Center around zero (-238 to +238)
    STA @twist_x                    ; Store X twist offset

    ; ========================================================================
    ; CALCULATE TUNNEL TWIST (Y AXIS)
    ; ========================================================================
    ; Use a DIFFERENT frequency for Y to create figure-8 / Lissajous motion.
    ; If both axes used the same frequency, the tunnel would just move
    ; in a circle. Different frequencies create more interesting paths.
    ;
    ; frame * 3 / 2 + 64 gives a different rate and phase offset.
    ; The phase offset (64 = 90°) ensures X and Y don't start in sync.
    ; ========================================================================
    LDA @frame_counter
    MUL A, #3                       ; Different frequency multiplier
    SHR A, #1                       ; Divide by 2 for slower motion
    ADD A, #64                      ; Phase offset (90° in 256-unit system)
    AND A, #255                     ; Wrap to valid angle range
    JSR get_sin                     ; Look up sine value
    MUL A, #TWIST_AMP               ; Scale by amplitude
    SHR A, #6                       ; Normalize
    SUB A, #238                     ; Center around zero
    STA @twist_y                    ; Store Y twist offset

    ; ========================================================================
    ; PROCESS ALL STARS
    ; ========================================================================
    ; Loop through each star in the array, updating its position and
    ; rendering it as a triangle. Y register tracks current star index.
    ; ========================================================================
    LDY #0                          ; Start with star 0

star_loop:
    ; ========================================================================
    ; CALCULATE STAR ARRAY ADDRESS
    ; ========================================================================
    ; Each star occupies 16 bytes. Address = star_array + (index * 16)
    ; We use SHL by 4 to multiply by 16 quickly.
    ; ========================================================================
    LDA Y                           ; Current star index
    SHL A, #4                       ; Multiply by 16 (bytes per star)
    LDX #star_array                 ; Base address of star array
    ADD X, A                        ; X now points to this star's data

    ; ========================================================================
    ; LOAD STAR DATA INTO WORKING VARIABLES
    ; ========================================================================
    ; Star data structure (16 bytes):
    ;   [X+0]  = angle  (position around tunnel, 0-255)
    ;   [X+4]  = radius (distance from tunnel center)
    ;   [X+8]  = z      (depth, larger = farther away)
    ;   [X+12] = speed  (movement rate per frame)
    ;
    ; We copy to working variables for easier manipulation.
    ; The IE32 doesn't support [X+offset] addressing, so we manually
    ; advance X between loads.
    ; ========================================================================
    LDA [X]                         ; Load angle
    STA @0x8820                     ; Store in angle temp variable

    PUSH X                          ; Save base pointer
    ADD X, #4                       ; Move to radius field
    LDA [X]                         ; Load radius
    STA @0x8824                     ; Store in radius temp

    ADD X, #4                       ; Move to z field
    LDA [X]                         ; Load z depth
    STA @0x8828                     ; Store in z temp

    ADD X, #4                       ; Move to speed field
    LDA [X]                         ; Load speed
    STA @0x882C                     ; Store in speed temp
    POP X                           ; Restore base pointer

    ; ========================================================================
    ; UPDATE STAR Z POSITION (MOVE TOWARD CAMERA)
    ; ========================================================================
    ; Stars move toward the camera each frame. Subtracting speed from Z
    ; makes the star appear closer. When Z becomes very small, the star
    ; has "passed" the camera and needs to respawn.
    ; ========================================================================
    LDA @0x8828                     ; Load current Z
    SUB A, @0x882C                  ; Subtract speed (move closer)
    STA @0x8828                     ; Store updated Z

    ; ========================================================================
    ; CHECK IF STAR NEEDS TO RESPAWN
    ; ========================================================================
    ; If Z has become less than NEAR_PLANE, the star is too close (or has
    ; passed the camera). We respawn it at a random position far away.
    ;
    ; Comparison: if (z - NEAR_PLANE) < 0, respawn needed
    ; We check using JGT (jump if greater than), which skips respawn
    ; if (z - NEAR_PLANE) > 0.
    ; ========================================================================
    LDA @0x8828                     ; Load updated Z
    SUB A, #NEAR_PLANE              ; Compare to near plane threshold
    JGT A, no_respawn               ; If still visible, skip respawn

    ; --- Respawn Star at Random Position ---
    ; Generate new random values for all star parameters.
    ; This creates the illusion of infinite stars streaming past.

    ; New random angle (0-255)
    JSR random                      ; Get random value
    AND A, #255                     ; Mask to 8 bits (0-255)
    STA @0x8820                     ; Store as new angle

    ; New random radius (15 to 15+TUNNEL_RADIUS*255/256)
    ; Formula: radius = (random & 255) * TUNNEL_RADIUS / 256 + 15
    ; The +15 ensures stars don't spawn exactly on center axis.
    JSR random
    AND A, #255
    MUL A, #TUNNEL_RADIUS           ; Scale to tunnel radius
    SHR A, #8                       ; Divide by 256
    ADD A, #15                      ; Minimum radius offset
    STA @0x8824                     ; Store as new radius

    ; New Z position (FAR_PLANE to FAR_PLANE+511)
    ; Stars respawn at various far distances to avoid them all arriving
    ; at once (which would look unnatural).
    JSR random
    AND A, #511                     ; Random offset 0-511
    ADD A, #FAR_PLANE               ; Add to far plane distance
    STA @0x8828                     ; Store as new Z

    ; New speed (2-9 units per frame)
    ; Varying speeds create parallax depth effect.
    JSR random
    AND A, #7                       ; Random 0-7
    ADD A, #2                       ; Add base speed (range 2-9)
    STA @0x882C                     ; Store as new speed

no_respawn:
    ; ========================================================================
    ; WRITE UPDATED STAR DATA BACK TO ARRAY
    ; ========================================================================
    ; Whether respawned or just moved, save the updated values.
    ; ========================================================================
    LDA @0x8820                     ; Load angle
    STA [X]                         ; Store to star array
    ADD X, #4                       ; Advance to radius field
    LDA @0x8824                     ; Load radius
    STA [X]                         ; Store to star array
    ADD X, #4                       ; Advance to z field
    LDA @0x8828                     ; Load z
    STA [X]                         ; Store to star array
    ADD X, #4                       ; Advance to speed field
    LDA @0x882C                     ; Load speed
    STA [X]                         ; Store to star array

    ; ========================================================================
    ; 3D TO 2D PROJECTION
    ; ========================================================================
    ; Now we project the star's 3D position to 2D screen coordinates.
    ; This is the most mathematically complex part of the demo.
    ;
    ; POLAR TO CARTESIAN CONVERSION:
    ; Stars are stored in polar coordinates (angle, radius) around the
    ; tunnel axis. We convert to Cartesian (x, y) using:
    ;   local_x = cos(angle) * radius
    ;   local_y = sin(angle) * radius
    ;
    ; TWIST APPLICATION:
    ; We add the tunnel twist to offset the entire coordinate system:
    ;   world_x = local_x + twist_x
    ;   world_y = local_y + twist_y
    ;
    ; PERSPECTIVE PROJECTION:
    ; Finally, we project 3D to 2D using the pinhole camera model:
    ;   screen_x = CENTER_X + (world_x * FOCAL_LENGTH / z)
    ;   screen_y = CENTER_Y + (world_y * FOCAL_LENGTH / z)
    ; ========================================================================

    ; --- Get Cosine Value ---
    ; cos(angle) = sin(angle + 64), where 64 = 90° in 256-unit system
    LDA @0x8820                     ; Load star's angle
    ADD A, #64                      ; Add 90° for cosine
    AND A, #255                     ; Wrap to valid range
    JSR get_sin                     ; Look up from sine table
    STA @0x8830                     ; Store cosine value (0-254)

    ; --- Get Sine Value ---
    LDA @0x8820                     ; Load star's angle
    JSR get_sin                     ; Look up sine value
    STA @0x8834                     ; Store sine value (0-254)

    ; ========================================================================
    ; COMPUTE LOCAL POSITION WITH OFFSET
    ; ========================================================================
    ; This is where we handle the unsigned arithmetic challenge.
    ;
    ; PROBLEM: cos/sin values in our table are 0-254 (127 = zero).
    ; A true signed multiplication would give negative results for
    ; values 0-126, but unsigned MUL always gives positive results.
    ;
    ; SOLUTION: Add WORLD_OFFSET to keep everything positive.
    ; We'll subtract the projected offset later.
    ;
    ; local_x_pos = cos_val * radius / 64 + WORLD_OFFSET
    ;
    ; The /64 (SHR 6) normalizes the trig multiplication result.
    ; ========================================================================

    ; Calculate local_x_pos
    LDA @0x8830                     ; Load cos_val (0-254)
    MUL A, @0x8824                  ; Multiply by radius
    SHR A, #6                       ; Divide by 64 to normalize
    ADD A, #WORLD_OFFSET            ; Add offset to keep positive
    STA @0x8838                     ; Store local_x_pos

    ; Calculate local_y_pos (same formula with sin)
    LDA @0x8834                     ; Load sin_val (0-254)
    MUL A, @0x8824                  ; Multiply by radius
    SHR A, #6                       ; Normalize
    ADD A, #WORLD_OFFSET            ; Add offset
    STA @0x883C                     ; Store local_y_pos

    ; ========================================================================
    ; APPLY TWIST OFFSET
    ; ========================================================================
    ; Add the tunnel twist to create the swooping motion.
    ; We also add 128 as additional safety margin for the offset math.
    ; ========================================================================
    LDA @0x8838                     ; Load local_x_pos
    ADD A, @twist_x                 ; Add X twist (-238 to +238)
    ADD A, #128                     ; Safety offset
    STA @0x8838                     ; Store adjusted position

    LDA @0x883C                     ; Load local_y_pos
    ADD A, @twist_y                 ; Add Y twist
    ADD A, #128                     ; Safety offset
    STA @0x883C                     ; Store adjusted position

    ; ========================================================================
    ; VALIDATE Z DEPTH
    ; ========================================================================
    ; Ensure Z is large enough for safe division. Very small Z values
    ; would cause extreme perspective distortion or division issues.
    ; ========================================================================
    LDA @0x8828                     ; Load star's Z
    STA @cur_z                      ; Copy to cur_z for division

    LDB #10                         ; Minimum safe Z value
    SUB B, A                        ; Compare: 10 - z
    JGT B, skip_star                ; If z < 10, skip this star

    ; ========================================================================
    ; PERSPECTIVE PROJECTION
    ; ========================================================================
    ; Project 3D world coordinates to 2D screen coordinates.
    ;
    ; Formula: proj_coord = world_coord * FOCAL_LENGTH / z
    ;
    ; The FOCAL_LENGTH (200) determines how "zoomed in" the view is.
    ; Larger values = more telephoto (less perspective distortion).
    ; Smaller values = more wide-angle (extreme distortion).
    ; ========================================================================

    ; Project X coordinate
    LDA @0x8838                     ; Load world X (with offset)
    MUL A, #FOCAL_LENGTH            ; Multiply by focal length
    DIV A, @cur_z                   ; Divide by Z depth
    STA @0x8848                     ; Store projected X

    ; Project Y coordinate
    LDA @0x883C                     ; Load world Y (with offset)
    MUL A, #FOCAL_LENGTH            ; Multiply by focal length
    DIV A, @cur_z                   ; Divide by Z depth
    STA @0x884C                     ; Store projected Y

    ; ========================================================================
    ; COMPUTE AND SUBTRACT PROJECTED OFFSET
    ; ========================================================================
    ; Earlier, we added WORLD_OFFSET + 128 + (127*radius/64) to local coords.
    ; Now we need to subtract how that offset projects to screen space.
    ;
    ; The 127*radius/64 term comes from the sine table center value (127)
    ; times the radius, normalized. This represents the "neutral" position
    ; contribution from the trig multiplication.
    ;
    ; offset_proj = (WORLD_OFFSET + 128 + 127*radius/64) * FOCAL / z
    ;
    ; Then: screen_coord = proj_coord - offset_proj + CENTER
    ; ========================================================================

    ; Calculate the offset that needs to be subtracted
    LDA #127                        ; Sine table center value
    MUL A, @0x8824                  ; Multiply by radius
    SHR A, #6                       ; Normalize (same as local coord calc)
    ADD A, #WORLD_OFFSET            ; Add world offset
    ADD A, #128                     ; Add safety offset (must match above)
    MUL A, #FOCAL_LENGTH            ; Project the offset
    DIV A, @cur_z                   ; Divide by Z
    STA @offset_proj                ; Store projected offset

    ; Calculate final screen X
    LDA @0x8848                     ; Load projected X
    SUB A, @offset_proj             ; Subtract the projected offset
    ADD A, #CENTER_X                ; Add screen center
    STA @0x8850                     ; Store final screen X

    ; Calculate final screen Y
    LDA @0x884C                     ; Load projected Y
    SUB A, @offset_proj             ; Subtract the projected offset
    ADD A, #CENTER_Y                ; Add screen center
    STA @0x8854                     ; Store final screen Y

    ; ========================================================================
    ; SCREEN BOUNDS CHECKING
    ; ========================================================================
    ; Stars outside the visible screen area shouldn't be drawn.
    ; We check using the sign bit trick: negative values have bit 31 set.
    ; Values >= SCREEN_W or SCREEN_H after subtraction indicate overflow.
    ; ========================================================================

    ; Check X >= 0 (sign bit not set)
    LDA @0x8850
    AND A, #0x80000000              ; Isolate sign bit
    JNZ A, skip_star                ; If negative, skip

    ; Check X < SCREEN_W
    LDA @0x8850
    SUB A, #SCREEN_W                ; If X >= 640, result is positive
    AND A, #0x80000000              ; Check sign bit
    JZ A, skip_star                 ; If NOT negative (X >= 640), skip

    ; Check Y >= 0
    LDA @0x8854
    AND A, #0x80000000
    JNZ A, skip_star

    ; Check Y < SCREEN_H
    LDA @0x8854
    SUB A, #SCREEN_H
    AND A, #0x80000000
    JZ A, skip_star

    ; ========================================================================
    ; CALCULATE STAR SIZE BASED ON DEPTH
    ; ========================================================================
    ; Stars closer to the camera appear larger. We compute size using:
    ;   size = 5000 / z
    ;
    ; This gives inverse relationship: small z = large star.
    ; We clamp the result to reasonable bounds (12-96 in 12.4 units).
    ; ========================================================================
    LDA #5000                       ; Size numerator constant
    DIV A, @cur_z                   ; Divide by depth
    STA @0x8858                     ; Store raw size

    ; Clamp minimum size to 12 (prevents invisible tiny stars)
    LDA @0x8858
    SUB A, #12                      ; Compare to minimum
    JGT A, size_min_ok              ; If size > 12, keep it
    LDA #12                         ; Clamp to minimum
    STA @0x8858
size_min_ok:

    ; Clamp maximum size to 96 (prevents huge ugly triangles)
    LDA @0x8858
    SUB A, #96                      ; Compare to maximum
    JLT A, size_max_ok              ; If size < 96, keep it
    LDA #96                         ; Clamp to maximum
    STA @0x8858
size_max_ok:

    ; ========================================================================
    ; SET STAR COLOR BASED ON DEPTH
    ; ========================================================================
    ; Depth-based coloring creates atmospheric perspective:
    ;   - Close stars (z < 200): Bright white (hot, intense)
    ;   - Medium stars (200 <= z < 500): Cyan (cooler)
    ;   - Far stars (z >= 500): Deep blue (distant, faded)
    ;
    ; Voodoo colors are in 4.12 fixed-point format:
    ;   $1000 = full intensity (1.0)
    ;   $0800 = half intensity (0.5)
    ;   $0000 = zero intensity (0.0)
    ; ========================================================================

    ; Check if close (z < 200) -> white
    LDA @cur_z
    SUB A, #200
    JLT A, color_white              ; z < 200, use white

    ; Check if medium (z < 500) -> cyan
    LDA @cur_z
    SUB A, #500
    JLT A, color_cyan               ; 200 <= z < 500, use cyan

    ; Far stars (z >= 500) -> deep blue
    LDA #0x0500                     ; R = dim (0.3125)
    STA @VOODOO_START_R
    LDA #0x0800                     ; G = dim (0.5)
    STA @VOODOO_START_G
    LDA #0x1000                     ; B = full (1.0)
    STA @VOODOO_START_B
    JMP color_ok

color_cyan:
    ; Medium distance: cyan (reduced red, high green/blue)
    LDA #0x0800                     ; R = half (0.5)
    STA @VOODOO_START_R
    LDA #0x0E00                     ; G = high (0.875)
    STA @VOODOO_START_G
    LDA #0x1000                     ; B = full (1.0)
    STA @VOODOO_START_B
    JMP color_ok

color_white:
    ; Close distance: pure white (all channels full)
    LDA #0x1000                     ; R = full (1.0)
    STA @VOODOO_START_R
    STA @VOODOO_START_G             ; G = full
    STA @VOODOO_START_B             ; B = full

color_ok:
    ; Set alpha (fully opaque) and Z depth for rendering
    LDA #0x1000                     ; Alpha = full (1.0)
    STA @VOODOO_START_A
    LDA #0x8000                     ; Z = mid-depth (for z-buffer)
    STA @VOODOO_START_Z

    ; ========================================================================
    ; CONVERT COORDINATES TO 12.4 FIXED-POINT
    ; ========================================================================
    ; Voodoo vertex coordinates use 12.4 format (4 fractional bits).
    ; Multiply integer pixel coordinates by 16 (shift left 4).
    ; ========================================================================
    LDA @0x8850                     ; Load screen X (integer pixels)
    SHL A, #4                       ; Convert to 12.4 format
    STA @0x8850                     ; Store back

    LDA @0x8854                     ; Load screen Y (integer pixels)
    SHL A, #4                       ; Convert to 12.4 format
    STA @0x8854                     ; Store back

    ; ========================================================================
    ; DRAW STAR AS TRIANGLE
    ; ========================================================================
    ; Each star is drawn as a small triangle pointing upward.
    ; Triangle vertices:
    ;   A (top):          (x, y - size)
    ;   B (bottom-left):  (x - size, y + size)
    ;   C (bottom-right): (x + size, y + size)
    ;
    ; This creates a simple equilateral-ish triangle shape.
    ;
    ;           A
    ;          /\
    ;         /  \
    ;        /    \
    ;       B------C
    ;
    ; ========================================================================

    ; Vertex A (top center)
    LDA @0x8850                     ; Center X
    STA @VOODOO_VERTEX_AX           ; Vertex A X
    LDA @0x8854                     ; Center Y
    SUB A, @0x8858                  ; Y - size (top point)
    STA @VOODOO_VERTEX_AY           ; Vertex A Y

    ; Vertex B (bottom left)
    LDA @0x8850
    SUB A, @0x8858                  ; X - size (left point)
    STA @VOODOO_VERTEX_BX           ; Vertex B X
    LDA @0x8854
    ADD A, @0x8858                  ; Y + size (bottom)
    STA @VOODOO_VERTEX_BY           ; Vertex B Y

    ; Vertex C (bottom right)
    LDA @0x8850
    ADD A, @0x8858                  ; X + size (right point)
    STA @VOODOO_VERTEX_CX           ; Vertex C X
    LDA @0x8854
    ADD A, @0x8858                  ; Y + size (bottom)
    STA @VOODOO_VERTEX_CY           ; Vertex C Y

    ; Submit triangle to GPU (any write triggers rendering)
    LDA #0
    STA @VOODOO_TRIANGLE_CMD        ; Draw the triangle!

skip_star:
    ; ========================================================================
    ; NEXT STAR
    ; ========================================================================
    ; Increment star index and loop until all stars processed.
    ; ========================================================================
    ADD Y, #1                       ; Next star index
    LDB #NUM_STARS                  ; Total number of stars
    SUB B, Y                        ; Compare: NUM_STARS - Y
    JNZ B, star_loop                ; If not done, process next star

    ; ========================================================================
    ; DRAW 3D SCROLLTEXT
    ; ========================================================================
    ; Render scrolling text in front of the starfield.
    ; ========================================================================
    JSR draw_scrolltext

    ; ========================================================================
    ; SWAP BUFFERS
    ; ========================================================================
    ; The Voodoo uses double buffering: we draw to the back buffer while
    ; the front buffer is displayed. Swapping makes our newly drawn frame
    ; visible and gives us a fresh buffer to draw the next frame.
    ; ========================================================================
    LDA #1
    STA @VOODOO_SWAP_BUFFER_CMD     ; Swap front and back buffers

    ; ========================================================================
    ; ADVANCE ANIMATION
    ; ========================================================================
    ; Increment frame counter for twist animation.
    ; The counter naturally wraps at 32-bit overflow (4+ billion frames).
    ; ========================================================================
    LDA @frame_counter
    ADD A, #1
    STA @frame_counter

    ; Update scroll position
    LDA @scroll_offset
    ADD A, #SCROLL_SPEED
    STA @scroll_offset

    JMP main_loop                   ; Repeat forever

; ============================================================================
; SUBROUTINES
; ============================================================================

; ============================================================================
; GET_SIN - Look Up Sine Value from Table
; ============================================================================
; Input:  A = angle (0-255, representing 0° to 360°)
; Output: A = sine value (0-254, where 127 = 0, 0 = -1, 254 = +1)
;
; The sine table stores 256 entries as 32-bit words. We mask the angle to
; 8 bits, multiply by 4 (bytes per entry), and add to table base address.
; ============================================================================
get_sin:
    AND A, #255                     ; Ensure angle is 0-255
    SHL A, #2                       ; Multiply by 4 (32-bit entries)
    LDX #sin_table                  ; Base address of sine table
    ADD X, A                        ; Calculate entry address
    LDA [X]                         ; Load sine value
    RTS

; ============================================================================
; RANDOM - Linear Congruential Generator (LCG)
; ============================================================================
; Output: A = pseudo-random 32-bit value
;
; Uses the classic LCG formula: seed = seed * 1664525 + 1013904223
; These constants are from Numerical Recipes and produce a full-period
; sequence of 2^32 values before repeating.
;
; LCGs are fast but not cryptographically secure. For demo effects,
; the visual randomness is more than adequate.
; ============================================================================
random:
    LDA @rand_seed                  ; Load current seed
    MUL A, #1664525                 ; Multiply by magic constant
    ADD A, #1013904223              ; Add magic constant
    STA @rand_seed                  ; Store new seed
    RTS                             ; Return random value in A

; ============================================================================
; DRAW_SCROLLTEXT - Render Bitmap Font Scrolling Text with Rainbow Colors
; ============================================================================
;
; This subroutine renders the scrolling message text using a 5x7 bitmap
; font. Each "on" pixel in the font is drawn as a colored quad (2 triangles).
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    SCROLLTEXT RENDERING ALGORITHM                       │
; └─────────────────────────────────────────────────────────────────────────┘
;
; OVERVIEW:
;   1. Calculate which characters are currently visible on screen
;   2. For each visible character:
;      a. Look up the character in the message string
;      b. Calculate its X position (based on scroll_offset)
;      c. Calculate its Y position (base + sine wave wobble)
;      d. Set rainbow color (phase-shifted sine waves)
;      e. Look up the 5x7 bitmap font data
;      f. For each "on" pixel in the bitmap, draw a quad
;
; CHARACTER POSITIONING:
;
;   scroll_offset increases by SCROLL_SPEED each frame
;   ─────────────────────────────────────────────────────────────────→ time
;
;   scroll_offset = 0:
;   ┌────────────────────────────────────────────────────────┐
;   │ [I][N][T][U][I][T][I][O][N][ ][ ][ ]...                │
;   │ ↑ first visible character                              │
;   └────────────────────────────────────────────────────────┘
;
;   scroll_offset = 48 (2 characters worth):
;   ┌────────────────────────────────────────────────────────┐
;   │ [T][U][I][T][I][O][N][ ][ ][ ]...                      │
;   │ ↑ "IN" have scrolled off-screen                        │
;   └────────────────────────────────────────────────────────┘
;
; SINE WAVE WOBBLE:
;
;   Each character's Y position is modulated by a sine wave:
;   y = SCROLL_Y_POS + sin((base_x + frame) / 8) * WOBBLE_AMP / 128
;
;   The /8 slows down the wave spatially (smoother curves)
;   Adding frame_counter makes the wave animate over time
;
;        y_offset
;          │
;    +40 ──┼───╭───╮───────╭───╮───────
;          │  ╱     ╲     ╱     ╲
;      0 ──┼─╱───────╲───╱───────╲─────
;          │╱         ╲ ╱         ╲
;    -40 ──┼───────────╰───────────╰───
;          │
;          └────────────────────────────→ x_position
;
; RAINBOW COLOR CYCLING:
;
;   We use phase-shifted sine waves for RGB:
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │  Intensity                                                          │
;   │  ↑                                                                  │
;   │  │  ╭─╮ R     ╭─╮ R                                                │
;   │  │ ╱   ╲     ╱   ╲        Phase offsets:                           │
;   │  │╱     ╲   ╱     ╲       Red:   0°   (0)                          │
;   │  │   ╭─╮ G ╭─╮ G          Green: 120° (85 in 0-255 scale)          │
;   │  │  ╱   ╲ ╱   ╲           Blue:  240° (170 in 0-255 scale)         │
;   │  │ ╱     ╰     ╲                                                   │
;   │  │    ╭─╮ B  ╭─╮ B                                                 │
;   │  │   ╱   ╲  ╱   ╲                                                  │
;   │  └─────────────────────────────────────────────────────────→ angle │
;   │    0°  120° 240° 360°                                              │
;   └─────────────────────────────────────────────────────────────────────┘
;
;   As the angle increases (per character position + time), colors cycle:
;   Red → Orange → Yellow → Green → Cyan → Blue → Magenta → Red
;
; BITMAP FONT RENDERING:
;
;   Each character is 7 bytes (rows), 5 bits per row (columns 4-0):
;
;   Character 'A' (ASCII 65):
;   font_data[(65-32) * 7 + row]
;
;   Row 0: 0x0E = 0 1 1 1 0    . █ █ █ .
;   Row 1: 0x11 = 1 0 0 0 1    █ . . . █
;   Row 2: 0x11 = 1 0 0 0 1    █ . . . █
;   Row 3: 0x1F = 1 1 1 1 1    █ █ █ █ █
;   Row 4: 0x11 = 1 0 0 0 1    █ . . . █
;   Row 5: 0x11 = 1 0 0 0 1    █ . . . █
;   Row 6: 0x11 = 1 0 0 0 1    █ . . . █
;
;   For each "on" bit, we draw a PIXEL_SIZE × PIXEL_SIZE quad at:
;   screen_x = base_x + column * PIXEL_SIZE
;   screen_y = base_y + row * PIXEL_SIZE
;
; INPUTS:
;   scroll_offset - Current scroll position (modified by main loop)
;   frame_counter - Animation frame (for wobble and color cycling)
;
; OUTPUTS:
;   Triangles rendered to Voodoo GPU
;
; REGISTERS MODIFIED:
;   A, B, X, Y (Y is saved/restored)
;
; ============================================================================
draw_scrolltext:
    PUSH Y                          ; Save Y (star loop uses it)

    ; ------------------------------------------------------------------------
    ; SET FIXED RENDERING PARAMETERS
    ; ------------------------------------------------------------------------
    ; Alpha and Z are the same for all scrolltext pixels.
    ; Color (R, G, B) will be set per-character.
    ; ------------------------------------------------------------------------
    LDA #0x1000                     ; Alpha = 1.0 (fully opaque)
    STA @VOODOO_START_A
    LDA #0x2000                     ; Z = front of z-buffer
    STA @VOODOO_START_Z             ; (renders in front of stars)

    ; ------------------------------------------------------------------------
    ; CALCULATE FIRST VISIBLE CHARACTER INDEX
    ; ------------------------------------------------------------------------
    ; As scroll_offset increases, earlier characters scroll off-screen.
    ; We calculate which character index starts at the left edge.
    ;
    ; Each character occupies CHAR_WIDTH * PIXEL_SIZE = 6 * 4 = 24 pixels.
    ; first_char = scroll_offset / 24 ≈ scroll_offset / 32 (using SHR 5)
    ;
    ; The approximation (32 instead of 24) means characters may slightly
    ; overlap at edges, but this is imperceptible and much faster.
    ; ------------------------------------------------------------------------
    LDA @scroll_offset
    SHR A, #5                       ; Divide by 32 (approximate character index)
    STA @scroll_char_num

    LDY #0                          ; Character counter (0 to MAX_CHARS-1)

; ============================================================================
; CHARACTER RENDERING LOOP
; ============================================================================
; We render up to MAX_CHARS (24) characters per frame. This loop:
;   1. Determines which character from the message to display
;   2. Calculates its screen position with wobble
;   3. Sets rainbow color
;   4. Renders the 5x7 bitmap as quads
; ============================================================================
scroll_char_loop:
    ; ------------------------------------------------------------------------
    ; CHECK LOOP TERMINATION
    ; ------------------------------------------------------------------------
    ; Y is our character counter (0 to MAX_CHARS-1)
    ; ------------------------------------------------------------------------
    LDA Y
    SUB A, #MAX_CHARS               ; Compare: Y - MAX_CHARS
    JGE A, scroll_text_done         ; If Y >= MAX_CHARS, we're done

    ; ------------------------------------------------------------------------
    ; CALCULATE MESSAGE CHARACTER INDEX (with wraparound)
    ; ------------------------------------------------------------------------
    ; The message loops continuously. We need:
    ;   msg_index = (first_visible_char + screen_position) % message_length
    ;
    ; Example with message "HELLO" (length 5):
    ;   first_char=7, Y=0 → msg_index = (7+0) % 5 = 2 → 'L'
    ;   first_char=7, Y=1 → msg_index = (7+1) % 5 = 3 → 'L'
    ;   first_char=7, Y=2 → msg_index = (7+2) % 5 = 4 → 'O'
    ;   first_char=7, Y=3 → msg_index = (7+3) % 5 = 0 → 'H' (wrapped!)
    ;
    ; We implement modulo by repeated subtraction (no MOD instruction).
    ; ------------------------------------------------------------------------
    LDA @scroll_char_num            ; Load first visible character index
    ADD A, Y                        ; Add screen position offset
    LDX #scroll_message             ; (X unused here, but keeps syntax valid)
scroll_mod:
    SUB A, #218                     ; Subtract message length (218 chars)
    JGE A, scroll_mod               ; If still >= 0, subtract again
    ADD A, #218                     ; Went negative, restore last valid value

    ; ------------------------------------------------------------------------
    ; FETCH CHARACTER FROM MESSAGE
    ; ------------------------------------------------------------------------
    LDX #scroll_message             ; Base address of message string
    ADD X, A                        ; Add character index
    LDA [X]                         ; Load ASCII character code
    AND A, #0xFF                    ; Mask to single byte (clear upper bits)
    STA @scroll_char_code           ; Store for later use

    ; Check for null terminator (shouldn't happen with our looping, but safety)
    JZ A, scroll_text_done

    ; ------------------------------------------------------------------------
    ; CALCULATE CHARACTER X POSITION
    ; ------------------------------------------------------------------------
    ; Each character has a base X determined by its screen position (Y index)
    ; minus the sub-character scroll offset (smooth scrolling within char).
    ;
    ; base_x = Y * 24 - (scroll_offset % 32)
    ;
    ; The 24 = CHAR_WIDTH × PIXEL_SIZE = 6 × 4
    ; The % 32 gives smooth sub-pixel scrolling
    ; ------------------------------------------------------------------------
    LDA Y
    MUL A, #24                      ; Y × 24 pixels per character
    STA @scroll_base_x

    ; Subtract sub-character scroll offset for smooth movement
    LDB @scroll_offset
    AND B, #31                      ; scroll_offset % 32 (sub-char position)
    LDA @scroll_base_x
    SUB A, B                        ; Adjust for smooth scrolling
    STA @scroll_base_x

    ; ------------------------------------------------------------------------
    ; CALCULATE CHARACTER Y POSITION (with sine wave wobble)
    ; ------------------------------------------------------------------------
    ; The classic demoscene "wavy text" effect!
    ;
    ; wobble_angle = (base_x + frame_counter) / 8
    ; y_offset = sin(wobble_angle) * WOBBLE_AMP / 128
    ; base_y = SCROLL_Y_POS + y_offset - centering_adjustment
    ;
    ; The /8 (SHR #3) controls wavelength - larger = gentler waves
    ; Adding frame_counter makes the wave animate over time
    ; The -20 centers the wobble around SCROLL_Y_POS
    ; ------------------------------------------------------------------------
    LDA @scroll_base_x
    ADD A, @frame_counter           ; Phase = position + time (animation!)
    SHR A, #3                       ; Divide by 8 (wavelength control)
    AND A, #255                     ; Ensure valid sine table index
    JSR get_sin                     ; Look up sine value (0-254)
    MUL A, #WOBBLE_AMP              ; Scale by amplitude (0 to 10,160)
    SHR A, #7                       ; Divide by 128 (normalize to 0-79)
    ADD A, #SCROLL_Y_POS            ; Add base Y position
    SUB A, #20                      ; Center the wobble (sin goes 0-254, not -127 to +127)
    STA @scroll_base_y

    ; ------------------------------------------------------------------------
    ; CALCULATE FONT DATA POINTER
    ; ------------------------------------------------------------------------
    ; Our font covers ASCII 32-95 (space through underscore), 64 characters.
    ; Each character uses 7 bytes (one per row of the 5x7 bitmap).
    ;
    ; font_ptr = font_data + (ascii_code - 32) * 7
    ;
    ; Example: 'A' = ASCII 65
    ;   offset = (65 - 32) * 7 = 33 * 7 = 231
    ;   font_ptr = font_data + 231
    ; ------------------------------------------------------------------------
    LDA @scroll_char_code
    SUB A, #32                      ; Convert ASCII to font index (space = 0)
    STA @scroll_font_ptr            ; Store temporarily

    ; BOUNDS CHECK: Ensure character is in valid range (32-95)
    ; If char < 32, the subtraction wraps to a huge unsigned value
    ; (bit 31 will be set, which we use as a "negative" indicator)
    LDA @scroll_font_ptr
    AND A, #0x80000000              ; Check sign bit
    JNZ A, scroll_next_chr          ; If "negative", skip this character

    ; Check upper bound: skip if offset >= 64 (char > 95)
    LDA @scroll_font_ptr
    SUB A, #64                      ; Compare to max valid offset
    AND A, #0x80000000              ; Check if result went "negative"
    JZ A, scroll_next_chr           ; If NOT negative, offset >= 64, skip

    ; Calculate complete font address
    LDA @scroll_font_ptr            ; Reload valid offset (0-63)
    MUL A, #7                       ; 7 bytes per character
    STA @scroll_font_ptr            ; Store byte offset
    LDA #font_data                  ; Load font data base address
    ADD A, @scroll_font_ptr         ; Add byte offset
    STA @scroll_font_ptr            ; Store complete address

    ; ------------------------------------------------------------------------
    ; SET RAINBOW COLOR (Phase-Shifted Sine Waves)
    ; ------------------------------------------------------------------------
    ; Classic demoscene rainbow effect using phase-shifted sine waves.
    ;
    ; The angle for each channel is:
    ;   base_angle = Y * 32 + frame_counter * 2
    ;
    ; The phase offsets create the rainbow:
    ;   Red:   angle + 0°   (0)
    ;   Green: angle + 120° (85 in 0-255 scale, since 120/360 * 256 ≈ 85)
    ;   Blue:  angle + 240° (170)
    ;
    ; Color intensity calculation:
    ;   sin_val = get_sin(angle)        ; 0-254
    ;   scaled = sin_val / 16           ; 0-15
    ;   color = scaled * 256 + 0x0400   ; 0x0400-0x0F00 (in 4.12 fixed point)
    ;
    ; The +0x0400 boost ensures minimum brightness (no fully black pixels).
    ; ------------------------------------------------------------------------

    ; RED CHANNEL (phase offset = 0°)
    LDA Y                           ; Character position (spreads colors spatially)
    SHL A, #5                       ; × 32 (color spread rate)
    ADD A, @frame_counter           ; + time (animation)
    SHL A, #1                       ; × 2 (animation speed)
    AND A, #255                     ; Wrap to valid angle
    JSR get_sin                     ; Look up sine (0-254)
    SHR A, #4                       ; Divide by 16 (scale to 0-15)
    SHL A, #8                       ; Shift to 4.12 format (0x0000-0x0F00)
    ADD A, #0x0400                  ; Add minimum brightness
    STA @VOODOO_START_R             ; Set red channel

    ; GREEN CHANNEL (phase offset = 120° = 85)
    LDA Y
    SHL A, #5
    ADD A, @frame_counter
    SHL A, #1
    ADD A, #85                      ; Phase offset: 120° ≈ 85 (in 0-255)
    AND A, #255
    JSR get_sin
    SHR A, #4
    SHL A, #8
    ADD A, #0x0400
    STA @VOODOO_START_G             ; Set green channel

    ; BLUE CHANNEL (phase offset = 240° = 170)
    LDA Y
    SHL A, #5
    ADD A, @frame_counter
    SHL A, #1
    ADD A, #170                     ; Phase offset: 240° ≈ 170 (in 0-255)
    AND A, #255
    JSR get_sin
    SHR A, #4
    SHL A, #8
    ADD A, #0x0400
    STA @VOODOO_START_B             ; Set blue channel

    ; ========================================================================
    ; RENDER CHARACTER BITMAP (5×7 pixels → quads)
    ; ========================================================================
    ; We now iterate through the 7 rows and 5 columns of the character
    ; bitmap. For each "on" pixel, we draw a PIXEL_SIZE × PIXEL_SIZE quad.
    ;
    ; CHARACTER BITMAP LAYOUT:
    ;
    ;   Column:  4   3   2   1   0   (bit positions in each row byte)
    ;   Row 0:   ▪   █   █   █   ▪   ← font_row = 0x0E = 01110
    ;   Row 1:   █   ▪   ▪   ▪   █   ← font_row = 0x11 = 10001
    ;   Row 2:   █   ▪   ▪   ▪   █
    ;   Row 3:   █   █   █   █   █   ← font_row = 0x1F = 11111
    ;   Row 4:   █   ▪   ▪   ▪   █
    ;   Row 5:   █   ▪   ▪   ▪   █
    ;   Row 6:   █   ▪   ▪   ▪   █
    ;
    ; Each █ becomes a 4×4 pixel quad on screen (2 triangles).
    ; ========================================================================
    PUSH Y                          ; Save outer character counter
    LDY #0                          ; Row counter (0-6)

scroll_row_loop:
    ; Check if all 7 rows processed
    LDA Y
    SUB A, #7
    JGE A, scroll_row_done          ; Y >= 7 means done with this character

    ; ------------------------------------------------------------------------
    ; LOAD FONT ROW DATA
    ; ------------------------------------------------------------------------
    ; Each row is a single byte where bits 4-0 represent columns 4-0.
    ; Bit 4 = leftmost column, bit 0 = rightmost column.
    ; ------------------------------------------------------------------------
    LDA @scroll_font_ptr            ; Font data address for this character
    LDX A                           ; Copy to X for indexed access
    ADD X, Y                        ; Add row offset (font_ptr + row)
    LDA [X]                         ; Load the row byte
    AND A, #0xFF                    ; Mask to 8 bits (clear garbage)
    STA @scroll_font_row            ; Store for bit testing

    ; Process each of 5 columns in this row
    LDX #0                          ; Column counter (0-4)

scroll_pixel_loop:
    ; Check if all 5 columns processed
    LDA X
    SUB A, #5
    JGE A, scroll_pixel_done        ; X >= 5 means done with this row

    ; ------------------------------------------------------------------------
    ; TEST IF PIXEL IS SET
    ; ------------------------------------------------------------------------
    ; We need to check bit (4-X) in font_row.
    ; Column 0 → bit 4 (0x10)
    ; Column 1 → bit 3 (0x08)
    ; Column 2 → bit 2 (0x04)
    ; Column 3 → bit 1 (0x02)
    ; Column 4 → bit 0 (0x01)
    ;
    ; We compute the mask by starting with 0x10 and shifting right X times.
    ; ------------------------------------------------------------------------
    LDA #16                         ; Start with bit 4 mask (0x10)
    PUSH X                          ; Save column counter
scroll_shift:
    LDB X                           ; Load remaining shift count
    JZ B, scroll_shift_done         ; If zero, we're done shifting
    SHR A, #1                       ; Shift mask right once
    SUB X, #1                       ; Decrement shift counter
    JMP scroll_shift                ; Repeat until X = 0
scroll_shift_done:
    POP X                           ; Restore column counter
    ; A now contains the bitmask for column X

    ; Test if this pixel is "on"
    LDB @scroll_font_row            ; Load row bitmap
    AND B, A                        ; Isolate the target bit
    JZ B, scroll_skip_pixel         ; If zero, pixel is off - skip

    ; ------------------------------------------------------------------------
    ; CALCULATE SCREEN POSITION FOR THIS PIXEL
    ; ------------------------------------------------------------------------
    ; screen_x = base_x + column * PIXEL_SIZE
    ; screen_y = base_y + row * PIXEL_SIZE
    ; ------------------------------------------------------------------------
    LDA X                           ; Column index (0-4)
    MUL A, #PIXEL_SIZE              ; × 4 = pixel offset
    ADD A, @scroll_base_x           ; + character base X
    STA @scroll_screen_x

    PUSH Y                          ; Save row counter (need Y register)
    LDA Y                           ; Row index (0-6)
    MUL A, #PIXEL_SIZE              ; × 4 = pixel offset
    ADD A, @scroll_base_y           ; + character base Y
    STA @scroll_screen_y
    POP Y                           ; Restore row counter

    ; ------------------------------------------------------------------------
    ; SCREEN BOUNDS CHECKING
    ; ------------------------------------------------------------------------
    ; Skip pixels that are off-screen (left, right, top, or bottom).
    ; We use the sign bit trick: "negative" unsigned values have bit 31 set.
    ; ------------------------------------------------------------------------
    LDA @scroll_screen_x
    AND A, #0x80000000              ; Check if X is "negative" (off left edge)
    JNZ A, scroll_skip_pixel

    LDA @scroll_screen_x
    SUB A, #SCREEN_W                ; Check if X >= 640 (off right edge)
    JGE A, scroll_skip_pixel

    LDA @scroll_screen_y
    AND A, #0x80000000              ; Check if Y is "negative" (off top edge)
    JNZ A, scroll_skip_pixel

    LDA @scroll_screen_y
    SUB A, #SCREEN_H                ; Check if Y >= 480 (off bottom edge)
    JGE A, scroll_skip_pixel

    ; ------------------------------------------------------------------------
    ; CONVERT TO 12.4 FIXED-POINT
    ; ------------------------------------------------------------------------
    ; Voodoo expects 12.4 format: multiply pixel coords by 16 (SHL 4).
    ; PIXEL_SIZE (4) in 12.4 = 4 × 16 = 64.
    ; ------------------------------------------------------------------------
    LDA @scroll_screen_x
    SHL A, #4                       ; Convert to 12.4
    STA @scroll_screen_x
    LDA @scroll_screen_y
    SHL A, #4                       ; Convert to 12.4
    STA @scroll_screen_y

    ; ------------------------------------------------------------------------
    ; DRAW PIXEL AS QUAD (2 TRIANGLES)
    ; ------------------------------------------------------------------------
    ; A quad requires two triangles sharing an edge:
    ;
    ;   A────B          Triangle 1: A-B-C (top-left, top-right, bottom-left)
    ;   │╲   │          Triangle 2: B-D-C (top-right, bottom-right, bottom-left)
    ;   │ ╲  │
    ;   │  ╲ │          Both triangles share vertices B and C.
    ;   C────D
    ;
    ; Vertex positions (in 12.4):
    ;   A = (screen_x, screen_y)              = top-left
    ;   B = (screen_x + 64, screen_y)         = top-right
    ;   C = (screen_x, screen_y + 64)         = bottom-left
    ;   D = (screen_x + 64, screen_y + 64)    = bottom-right
    ; ------------------------------------------------------------------------

    ; TRIANGLE 1: A-B-C (top-left half of quad)
    LDA @scroll_screen_x            ; Vertex A: top-left
    STA @VOODOO_VERTEX_AX
    LDA @scroll_screen_y
    STA @VOODOO_VERTEX_AY

    LDA @scroll_screen_x            ; Vertex B: top-right
    ADD A, #64                      ; + PIXEL_SIZE in 12.4 format
    STA @VOODOO_VERTEX_BX
    LDA @scroll_screen_y
    STA @VOODOO_VERTEX_BY

    LDA @scroll_screen_x            ; Vertex C: bottom-left
    STA @VOODOO_VERTEX_CX
    LDA @scroll_screen_y
    ADD A, #64                      ; + PIXEL_SIZE in 12.4 format
    STA @VOODOO_VERTEX_CY

    LDA #0
    STA @VOODOO_TRIANGLE_CMD        ; Submit triangle 1 to GPU

    ; TRIANGLE 2: B-D-C (bottom-right half of quad)
    LDA @scroll_screen_x            ; Vertex A (really B): top-right
    ADD A, #64
    STA @VOODOO_VERTEX_AX
    LDA @scroll_screen_y
    STA @VOODOO_VERTEX_AY

    LDA @scroll_screen_x            ; Vertex B (really D): bottom-right
    ADD A, #64
    STA @VOODOO_VERTEX_BX
    LDA @scroll_screen_y
    ADD A, #64
    STA @VOODOO_VERTEX_BY

    LDA @scroll_screen_x            ; Vertex C: bottom-left
    STA @VOODOO_VERTEX_CX
    LDA @scroll_screen_y
    ADD A, #64
    STA @VOODOO_VERTEX_CY

    LDA #0
    STA @VOODOO_TRIANGLE_CMD        ; Submit triangle 2 to GPU

scroll_skip_pixel:
    ADD X, #1                       ; Next column
    JMP scroll_pixel_loop           ; Process next pixel in row

scroll_pixel_done:
    ADD Y, #1                       ; Next row
    JMP scroll_row_loop             ; Process next row in character

scroll_row_done:
    POP Y                           ; Restore outer character counter

scroll_next_chr:
    ADD Y, #1                       ; Next character
    JMP scroll_char_loop            ; Process next character in message

scroll_text_done:
    POP Y                           ; Restore star loop's Y register
    RTS                             ; Return to main loop

; ============================================================================
; BUILD_SIN_TABLE - Generate Sine Lookup Table
; ============================================================================
; Builds a 256-entry sine table using quarter-wave symmetry.
;
; === THE QUARTER-WAVE OPTIMIZATION ===
;
; A full sine wave has mirror symmetry: we only need to store one quarter
; (0-90°) and derive the rest mathematically.
;
;     Quadrant 0 (0-63):   sin(angle) = quarter_sin[angle]
;     Quadrant 1 (64-127): sin(angle) = quarter_sin[63 - (angle-64)]
;     Quadrant 2 (128-191): sin(angle) = -sin(angle - 128)
;     Quadrant 3 (192-255): sin(angle) = -sin(256 - angle)
;
; Our table stores values 0-254 (127 = zero crossing):
;     Quadrants 0-1: value = quarter_sin[index] + 127
;     Quadrants 2-3: value = 127 - quarter_sin[index]
;
; The quarter_sin table at the end of this file contains 64 pre-calculated
; values for the first quarter of the sine wave.
; ============================================================================
build_sin_table:
    LDY #0                          ; Angle counter (0-255)
    LDX #sin_table                  ; Destination pointer

sin_loop:
    ; Determine quadrant (0-3) from bits 7-6 of angle
    LDA Y                           ; Current angle
    SHR A, #6                       ; Shift to get quadrant (0-3)
    STA @0x8870                     ; Store quadrant

    ; Get index within quadrant (bits 5-0)
    LDA Y
    AND A, #63                      ; Index 0-63 within quadrant

    ; For odd quadrants (1, 3), mirror the index: 63 - index
    LDB @0x8870                     ; Load quadrant
    AND B, #1                       ; Check if odd quadrant
    JZ B, no_mirror                 ; Even quadrant, no mirror
    LDB #63
    SUB B, A                        ; Mirror: 63 - index
    LDA B                           ; A = mirrored index
no_mirror:

    ; Look up quarter sine value
    SHL A, #2                       ; Multiply by 4 (32-bit entries)
    PUSH X                          ; Save destination pointer
    LDX #quarter_sin                ; Quarter sine table address
    ADD X, A                        ; Calculate entry address
    LDA [X]                         ; Load quarter sine value (0-127)
    POP X                           ; Restore destination pointer

    ; Apply sign based on quadrant (quadrants 2-3 are negative)
    LDB @0x8870                     ; Load quadrant
    AND B, #2                       ; Check bit 1 (quadrants 2-3)
    JZ B, sin_pos                   ; Quadrants 0-1: positive
    ; Negative half: value = 127 - quarter_sin
    LDB #127
    SUB B, A
    LDA B
    JMP store_sin

sin_pos:
    ; Positive half: value = quarter_sin + 127
    ADD A, #127

store_sin:
    STA [X]                         ; Store to sine table
    ADD X, #4                       ; Next entry (32-bit)
    ADD Y, #1                       ; Next angle
    LDB #256
    SUB B, Y                        ; Compare: 256 - Y
    JNZ B, sin_loop                 ; Continue until all 256 entries
    RTS

; ============================================================================
; INIT_STARS - Initialize Star Array with Random Positions
; ============================================================================
; Scatters NUM_STARS (256) stars throughout the tunnel volume.
; Each star gets random initial values for angle, radius, z, and speed.
;
; This provides the initial state before animation begins. Stars will
; subsequently be updated and respawned as they pass the camera.
; ============================================================================
init_stars:
    LDY #0                          ; Star counter
    LDX #star_array                 ; Start of star data array

init_loop:
    ; Random angle (0-255)
    JSR random
    AND A, #255
    STA [X]                         ; Store angle at offset 0
    ADD X, #4

    ; Random radius (15 to 15 + TUNNEL_RADIUS*255/256)
    JSR random
    AND A, #255
    MUL A, #TUNNEL_RADIUS
    SHR A, #8                       ; Divide by 256
    ADD A, #15                      ; Minimum radius
    STA [X]                         ; Store radius at offset 4
    ADD X, #4

    ; Random initial Z (NEAR_PLANE to NEAR_PLANE+2047)
    ; Using wider range than respawn so initial frame looks full
    JSR random
    AND A, #2047                    ; Larger range for initial spread
    ADD A, #NEAR_PLANE
    STA [X]                         ; Store z at offset 8
    ADD X, #4

    ; Random speed (2-9)
    JSR random
    AND A, #7
    ADD A, #2
    STA [X]                         ; Store speed at offset 12
    ADD X, #4

    ; Next star
    ADD Y, #1
    LDB #NUM_STARS
    SUB B, Y
    JNZ B, init_loop
    RTS

; ============================================================================
; DATA SECTION
; ============================================================================

; ============================================================================
; QUARTER SINE TABLE
; ============================================================================
; Pre-calculated sine values for angles 0-90° (indices 0-63).
; Values range from 0 to 127, representing sin(0°)=0 to sin(90°)=1.
;
; These values were generated by: round(sin(i * π / 128) * 127)
;
; The build_sin_table routine uses these 64 values plus symmetry to
; construct the full 256-entry table at runtime.
;
; Why not pre-compute the full table? This approach:
;   1. Saves ROM space (64 entries vs 256)
;   2. Demonstrates the quarter-wave algorithm
;   3. Provides a teaching example of sine wave properties
; ============================================================================
quarter_sin:
    .word   0,   3,   6,  10,  13,  16,  19,  22
    .word  25,  28,  31,  34,  37,  40,  43,  46
    .word  49,  51,  54,  57,  60,  62,  65,  68
    .word  70,  73,  75,  78,  80,  82,  85,  87
    .word  89,  91,  94,  96,  98, 100, 102, 103
    .word 105, 107, 108, 110, 112, 113, 114, 116
    .word 117, 118, 119, 120, 121, 122, 123, 124
    .word 124, 125, 125, 126, 126, 127, 127, 127

; ============================================================================
; SCROLL MESSAGE
; ============================================================================
; The text that scrolls across the screen. This loops continuously, so the
; trailing spaces provide a gap before it repeats.
;
; Total length: 218 characters (hardcoded in scroll_mod loop)
;
; CUSTOMIZATION TIP:
; To change the message, edit the text below and update the #218 constant
; in the scroll_mod: SUB A, #218 instruction in draw_scrolltext.
; ============================================================================
scroll_message:
    .ascii "     INTUITION ENGINE     3DFX VOODOO TWISTING STARFIELD TUNNEL     CODE: IE32 RISC ASM BY INTUITION      MUSIC: REGGAE 2 BY DJINN (6502 + SID)     GREETINGS TO ALL DEMOSCENERS...     VISIT INTUITIONSUBSYNTH.COM      "
    .byte 0                         ; Null terminator (for safety)

; ============================================================================
; 5×7 BITMAP FONT DATA
; ============================================================================
;
; This is a classic 5×7 pixel bitmap font covering ASCII 32-95 (64 characters).
; Each character uses 7 bytes (one per row), with bits 4-0 representing columns.
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    FONT MEMORY LAYOUT                                   │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   font_data + 0:   Space (ASCII 32) - 7 bytes
;   font_data + 7:   !     (ASCII 33) - 7 bytes
;   font_data + 14:  "     (ASCII 34) - 7 bytes
;   ...
;   font_data + 231: A     (ASCII 65) - 7 bytes  [(65-32) × 7 = 231]
;   ...
;   font_data + 441: _     (ASCII 95) - 7 bytes  [(95-32) × 7 = 441]
;
;   Total: 64 characters × 7 bytes = 448 bytes
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    ROW BYTE FORMAT                                      │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   Bit:    7  6  5  4  3  2  1  0
;           ─  ─  ─  ┬──┬──┬──┬──┬── pixel columns
;           unused   │  │  │  │  └── column 0 (rightmost)
;                    │  │  │  └───── column 1
;                    │  │  └──────── column 2
;                    │  └─────────── column 3
;                    └────────────── column 4 (leftmost)
;
;   Example: Row byte 0x0E = 00001110
;     Bit 4=0: column 4 off  ▪
;     Bit 3=1: column 3 ON   █
;     Bit 2=1: column 2 ON   █
;     Bit 1=1: column 1 ON   █
;     Bit 0=0: column 0 off  ▪
;     Display: ▪███▪
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    SAMPLE CHARACTER: 'A' (ASCII 65)                     │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   Row 0:  0x0E  =  0 1 1 1 0  =   ▪ █ █ █ ▪
;   Row 1:  0x11  =  1 0 0 0 1  =   █ ▪ ▪ ▪ █
;   Row 2:  0x11  =  1 0 0 0 1  =   █ ▪ ▪ ▪ █
;   Row 3:  0x1F  =  1 1 1 1 1  =   █ █ █ █ █
;   Row 4:  0x11  =  1 0 0 0 1  =   █ ▪ ▪ ▪ █
;   Row 5:  0x11  =  1 0 0 0 1  =   █ ▪ ▪ ▪ █
;   Row 6:  0x11  =  1 0 0 0 1  =   █ ▪ ▪ ▪ █
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    HISTORICAL NOTE                                      │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The 5×7 font format was ubiquitous in the 8-bit era because:
;   - 5 pixels width fits in one byte (with 3 bits to spare)
;   - 7 pixels height is minimum for readable lowercase with descenders
;   - An 8×8 character cell allows 1 pixel spacing on each side
;   - The Commodore 64, Apple II, and many arcade games used this format
;
; This font is based on the classic "system" fonts of that era, with minor
; adjustments for clarity at small sizes.
;
; ============================================================================
font_data:
    ; Space (32)
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
    ; ! (33)
    .byte 0x04, 0x04, 0x04, 0x04, 0x00, 0x04, 0x00
    ; " (34)
    .byte 0x0A, 0x0A, 0x00, 0x00, 0x00, 0x00, 0x00
    ; # (35)
    .byte 0x0A, 0x1F, 0x0A, 0x0A, 0x1F, 0x0A, 0x00
    ; $ (36)
    .byte 0x04, 0x0F, 0x14, 0x0E, 0x05, 0x1E, 0x04
    ; % (37)
    .byte 0x18, 0x19, 0x02, 0x04, 0x08, 0x13, 0x03
    ; & (38)
    .byte 0x08, 0x14, 0x14, 0x08, 0x15, 0x12, 0x0D
    ; ' (39)
    .byte 0x04, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00
    ; ( (40)
    .byte 0x02, 0x04, 0x08, 0x08, 0x08, 0x04, 0x02
    ; ) (41)
    .byte 0x08, 0x04, 0x02, 0x02, 0x02, 0x04, 0x08
    ; * (42)
    .byte 0x00, 0x04, 0x15, 0x0E, 0x15, 0x04, 0x00
    ; + (43)
    .byte 0x00, 0x04, 0x04, 0x1F, 0x04, 0x04, 0x00
    ; , (44)
    .byte 0x00, 0x00, 0x00, 0x00, 0x04, 0x04, 0x08
    ; - (45)
    .byte 0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00
    ; . (46)
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00
    ; / (47)
    .byte 0x01, 0x02, 0x02, 0x04, 0x08, 0x08, 0x10
    ; 0 (48)
    .byte 0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E
    ; 1 (49)
    .byte 0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E
    ; 2 (50)
    .byte 0x0E, 0x11, 0x01, 0x06, 0x08, 0x10, 0x1F
    ; 3 (51)
    .byte 0x0E, 0x11, 0x01, 0x06, 0x01, 0x11, 0x0E
    ; 4 (52)
    .byte 0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02
    ; 5 (53)
    .byte 0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E
    ; 6 (54)
    .byte 0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E
    ; 7 (55)
    .byte 0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08
    ; 8 (56)
    .byte 0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E
    ; 9 (57)
    .byte 0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C
    ; : (58)
    .byte 0x00, 0x00, 0x04, 0x00, 0x04, 0x00, 0x00
    ; ; (59)
    .byte 0x00, 0x00, 0x04, 0x00, 0x04, 0x04, 0x08
    ; < (60)
    .byte 0x02, 0x04, 0x08, 0x10, 0x08, 0x04, 0x02
    ; = (61)
    .byte 0x00, 0x00, 0x1F, 0x00, 0x1F, 0x00, 0x00
    ; > (62)
    .byte 0x08, 0x04, 0x02, 0x01, 0x02, 0x04, 0x08
    ; ? (63)
    .byte 0x0E, 0x11, 0x01, 0x02, 0x04, 0x00, 0x04
    ; @ (64)
    .byte 0x0E, 0x11, 0x17, 0x15, 0x17, 0x10, 0x0E
    ; A (65)
    .byte 0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11
    ; B (66)
    .byte 0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E
    ; C (67)
    .byte 0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E
    ; D (68)
    .byte 0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E
    ; E (69)
    .byte 0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F
    ; F (70)
    .byte 0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10
    ; G (71)
    .byte 0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0E
    ; H (72)
    .byte 0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11
    ; I (73)
    .byte 0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E
    ; J (74)
    .byte 0x01, 0x01, 0x01, 0x01, 0x11, 0x11, 0x0E
    ; K (75)
    .byte 0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11
    ; L (76)
    .byte 0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F
    ; M (77)
    .byte 0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11
    ; N (78)
    .byte 0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11
    ; O (79)
    .byte 0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E
    ; P (80)
    .byte 0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10
    ; Q (81)
    .byte 0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D
    ; R (82)
    .byte 0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11
    ; S (83)
    .byte 0x0E, 0x11, 0x10, 0x0E, 0x01, 0x11, 0x0E
    ; T (84)
    .byte 0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04
    ; U (85)
    .byte 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E
    ; V (86)
    .byte 0x11, 0x11, 0x11, 0x11, 0x0A, 0x0A, 0x04
    ; W (87)
    .byte 0x11, 0x11, 0x11, 0x15, 0x15, 0x1B, 0x11
    ; X (88)
    .byte 0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11
    ; Y (89)
    .byte 0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04
    ; Z (90)
    .byte 0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F
    ; [ (91)
    .byte 0x0E, 0x08, 0x08, 0x08, 0x08, 0x08, 0x0E
    ; \ (92)
    .byte 0x10, 0x08, 0x08, 0x04, 0x02, 0x02, 0x01
    ; ] (93)
    .byte 0x0E, 0x02, 0x02, 0x02, 0x02, 0x02, 0x0E
    ; ^ (94)
    .byte 0x04, 0x0A, 0x11, 0x00, 0x00, 0x00, 0x00
    ; _ (95)
    .byte 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1F

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                    SID MUSIC DATA - REGGAE 2                           ==
; ==                                                                        ==
; ==                By Kamil Degorski (Djinn) / Fraction                    ==
; ==                            (1998)                                      ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    WHAT'S IN A .SID FILE?                               │
; └─────────────────────────────────────────────────────────────────────────┘
;
; A .SID file is NOT just audio data - it's a complete music program!
;
; Contents:
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │ HEADER (0x7C bytes for PSID v2)                                     │
;   │   Magic: "PSID" or "RSID"                                           │
;   │   Version, data offset, load/init/play addresses                    │
;   │   Song count, default song, speed flags                             │
;   │   Title, author, copyright (32 bytes each)                          │
;   ├─────────────────────────────────────────────────────────────────────┤
;   │ 6502 MACHINE CODE (music player routine)                            │
;   │   The actual C64 assembler code that drives the SID chip!           │
;   │   This code was written by the composer to play their music.        │
;   ├─────────────────────────────────────────────────────────────────────┤
;   │ PATTERN DATA (interpreted by the player code)                       │
;   │   Note sequences, instrument definitions, effects                   │
;   │   Format varies by composer/tracker used                            │
;   └─────────────────────────────────────────────────────────────────────┘
;
; The Intuition Engine has a REAL 6502 CPU CORE that executes this code!
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    HOW SID PLAYBACK WORKS                               │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   ┌─────────────────┐        ┌─────────────────┐        ┌─────────────────┐
;   │                 │        │                 │        │                 │
;   │   IE32 CPU      │───────►│   6502 CPU      │───────►│   NATIVE        │
;   │   (This Demo)   │        │   (SID Code)    │        │   SYNTHESIZER   │
;   │                 │        │                 │        │                 │
;   └────────┬────────┘        └────────┬────────┘        └────────┬────────┘
;            │                          │                          │
;            │ 1. Sets SID_PLAY_PTR     │ 2. Executes 6502         │ 3. Generates
;            │    SID_PLAY_LEN          │    music player          │    waveforms
;            │    SID_PLAY_CTRL         │    code ~50×/sec         │    (audio out)
;            │                          │                          │
;            │ Triggers playback        │ Writes to $D400+         │
;            ▼                          ▼                          ▼
;     ┌─────────────┐           ┌─────────────┐           ┌─────────────┐
;     │ Load .SID   │           │ "STA $D400" │           │ PWM/PCM     │
;     │ file into   │           │ remapped to │           │ audio       │
;     │ memory      │           │ synth regs  │           │ output      │
;     └─────────────┘           └─────────────┘           └─────────────┘
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE SID REGISTER REMAPPING                           │
; └─────────────────────────────────────────────────────────────────────────┘
;
; On a real Commodore 64, the SID chip occupies $D400-$D41C:
;
;   Address   │ Function
;   ──────────┼──────────────────────────────────────────────────
;   $D400-01  │ Voice 1 frequency (16-bit, low/high)
;   $D402-03  │ Voice 1 pulse width (12-bit for pulse waveform)
;   $D404     │ Voice 1 control (waveform, gate, sync, ring mod)
;   $D405-06  │ Voice 1 ADSR envelope (attack, decay, sustain, release)
;   ──────────┼──────────────────────────────────────────────────
;   $D407-0D  │ Voice 2 (same layout as voice 1)
;   $D40E-14  │ Voice 3 (same layout as voice 1)
;   ──────────┼──────────────────────────────────────────────────
;   $D415-16  │ Filter cutoff frequency
;   $D417     │ Filter resonance + voice routing
;   $D418     │ Master volume + filter mode
;
; When the 6502 player code executes instructions like "STA $D400",
; the Intuition Engine INTERCEPTS these writes and translates them
; to our native synthesizer parameters. This gives authentic C64
; sound without cycle-accurate analog circuit emulation!
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    ABOUT "REGGAE 2"                                     │
; └─────────────────────────────────────────────────────────────────────────┘
;
; Title:     Reggae 2
; Composer:  Kamil Degorski (Djinn)
; Group:     Fraction
; Year:      1998
;
; Djinn was known for his reggae and dub-influenced SID compositions,
; a style that became popular in the late 90s demoscene. "Reggae 2"
; features the characteristic:
;   - Offbeat chord stabs
;   - Deep bass lines
;   - Echo/delay effects
;   - Relaxed, groovy tempo
;
; This tune represents the "golden era" of SID music (late 90s) when
; composers had mastered the chip's quirks and were creating complex,
; musically sophisticated works despite the 3-voice limitation.
;
; ============================================================================
sid_data:
    .incbin "Reggae_2.sid"
sid_data_end:

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                         END OF PROGRAM                                 ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    ASSEMBLY STATISTICS                                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
; Code size:      ~3 KB (excluding SID data)
; SID music:      ~4.7 KB
; Total binary:   ~8 KB
; Sine table:     1 KB (generated at runtime)
; Star array:     4 KB (256 stars × 16 bytes)
; Font data:      448 bytes (64 chars × 7 bytes)
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    BUILD & RUN                                          │
; └─────────────────────────────────────────────────────────────────────────┘
;
; ASSEMBLE:
;   bin/ie32asm assembler/voodoo_mega_demo.asm
;
; RUN:
;   ./bin/IntuitionEngine -ie32 assembler/voodoo_mega_demo.iex
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    WHAT YOU SHOULD SEE                                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   - 256 stars rushing toward you in a cylindrical tunnel
;   - Tunnel center twisting in a Lissajous-like pattern
;   - Stars colored white (close), cyan (medium), blue (far)
;   - Rainbow-colored scrolling text with sine wave wobble
;   - "Reggae 2" by Djinn playing on the SID synthesizer
;   - Smooth 60 FPS animation
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    CUSTOMIZATION IDEAS                                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   - Change TWIST_AMP for more/less tunnel movement
;   - Adjust NUM_STARS for denser/sparser starfield
;   - Modify star colors (color_white/cyan/blue sections)
;   - Edit scroll_message text (update length constant too!)
;   - Change WOBBLE_AMP for more/less text wave amplitude
;   - Try different SID tunes (update sid_data_size)
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    CREDITS                                              │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   Demo code:    Zayn Otley
;   Music:        "Reggae 2" by Djinn/Fraction (1998)
;   Font design:  Classic 5×7 bitmap (public domain)
;   Voodoo SST-1: 3DFX Interactive (1996)
;
;   This demo was created to demonstrate:
;     - Hardware-accelerated 3D graphics on the Voodoo
;     - Classic demoscene effects (starfield, scrolltext)
;     - SID music playback through 6502 CPU core
;     - Fixed-point math on unsigned architectures
;
;   Greets to all demosceners, past and present!
;
; ============================================================================
