; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==              3D ROTATING CUBE WITH HARDWARE Z-BUFFERING                ==
; ==                                                                        ==
; ==         Motorola 68020 Asm for IntuitionEngine / 3DFX Voodoo SST-1     ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
;                              ╭─────────────────╮
;                             ╱                 ╱│
;                            ╱       ▄▄▄       ╱ │
;                           ╱      ▄▀  ▀▄     ╱  │
;                          ╱      █ FRONT█   ╱   │
;                         ╱       ▀▄__▄▀    ╱    │
;                        ╱                 ╱     │
;                       ├─────────────────┤      │
;                       │                 │     ╱
;                       │  3D CUBE DEMO   │    ╱
;                       │                 │   ╱
;                       │   Z-BUFFERED    │  ╱
;                       │   TRIANGLES     │ ╱
;                       │                 │╱
;                       └─────────────────┘
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Motorola 68020
; Video Chip:    3DFX Voodoo SST-1 (hardware 3D acceleration, Z-buffer)
; Audio Engine:  None
; Assembler:     vasmm68k_mot (VASM M68K, Motorola syntax)
; Build:         vasmm68k_mot -Fbin -m68020 -o voodoo_cube_68k.ie68 voodoo_cube_68k.asm
; Run:           ./bin/IntuitionEngine -68k voodoo_cube_68k.ie68
; Porting:       Voodoo MMIO is CPU-agnostic. Any CPU core can submit triangles
;                to the Voodoo rasterizer. IE32 version: see voodoo_mega_demo.asm.
;
; ============================================================================
; REFERENCE IMPLEMENTATION FOR 3D GRAPHICS PROGRAMMING
; ============================================================================
;
; This file demonstrates fundamental 3D graphics concepts:
;   - 3D coordinate transformation (rotation matrices)
;   - Perspective-less orthographic projection
;   - Hardware-accelerated triangle rendering
;   - Z-buffering for proper occlusion
;   - Per-face solid color rendering
;
; Reading time: ~30 minutes for thorough understanding
;
; ============================================================================
; TABLE OF CONTENTS
; ============================================================================
;
;   Line    Section
;   ----    -------
;   ~80     Historical Context (3D Cubes, Voodoo, M68K)
;   ~200    Architecture Overview
;   ~280    Constants
;   ~320    Program Entry Point
;   ~360    Main Loop
;   ~400    Draw Cube Subroutine
;   ~480    Draw Face & Triangle Subroutines
;   ~580    Vertex Transformation
;   ~650    Rotation Subroutines (X, Y, Z axes)
;   ~850    Data Section (Vertices, Faces, Sine Table)
;
; ============================================================================
; BUILD AND RUN
; ============================================================================
;
;   ASSEMBLE:
;     vasmm68k_mot -Fbin -m68020 -o voodoo_cube_68k.ie68 voodoo_cube_68k.asm
;
;   RUN:
;     ./bin/IntuitionEngine -68k voodoo_cube_68k.ie68
;
; (c) 2026 Zayn Otley - GPLv3 or later
;
; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                       HISTORICAL CONTEXT                               ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE ROTATING 3D CUBE                                 │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The rotating cube is THE fundamental 3D graphics demo, dating back to the
; early 1970s. It remains essential because it teaches:
;
;   1. 3D COORDINATE SYSTEMS
;      Understanding X (left/right), Y (up/down), Z (in/out) axes
;
;   2. ROTATION MATRICES
;      The mathematical foundation for all 3D transformations
;
;   3. VERTEX TRANSFORMATION PIPELINE
;      Local coords → World coords → View coords → Screen coords
;
;   4. FACE REPRESENTATION
;      How polygons are defined by vertex indices
;
;   5. HIDDEN SURFACE REMOVAL
;      Either backface culling or z-buffering
;
; HISTORICAL TIMELINE:
;
;   1970s: WIREFRAME CUBES
;   Early vector displays (Tektronix, Evans & Sutherland) showed wireframe
;   cubes rotating in real-time. This was revolutionary!
;
;   1980s: SOLID SHADED CUBES
;   With raster displays, filled polygons became possible. Games like
;   Elite (1984) used wireframe, but arcade games achieved solid fills.
;
;   1990s: TEXTURE-MAPPED CUBES
;   SGI workstations and then consumer 3D accelerators enabled texture
;   mapping. The 3DFX Voodoo (1996) brought this to home PCs.
;
;   TODAY: THE CUBE ENDURES
;   Modern GPUs can render billions of triangles per second, but the
;   rotating cube remains the "Hello World" of 3D graphics.
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE 3DFX VOODOO SST-1 (1996)                         │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The Voodoo was the first mass-market 3D accelerator. Key features:
;
;   - 50 million textured pixels per second (revolutionary!)
;   - Hardware Z-buffering (no manual depth sorting needed)
;   - Gouraud shading (smooth color gradients)
;   - Bilinear texture filtering
;   - Alpha blending for transparency
;
; For this cube demo, we use:
;   - Z-BUFFERING: Automatically handles occlusion (back faces hidden)
;   - FLAT SHADING: One solid color per face (no gradients)
;   - NO TEXTURES: Solid colored triangles only
;
; Z-BUFFERING EXPLAINED:
;
;   Without z-buffering, you must manually sort polygons back-to-front
;   (the "painter's algorithm"). This is complex and has edge cases.
;
;   With z-buffering:
;     - Each pixel has a depth value (Z)
;     - When drawing a pixel, compare its Z to the stored Z
;     - Only draw if the new pixel is closer (smaller Z)
;     - The GPU handles this automatically per-pixel!
;
;   ┌─────────────────────────────────────────────────────────────────────┐
;   │                          Z-BUFFER                                   │
;   │                                                                     │
;   │   Framebuffer:          Z-Buffer:                                   │
;   │   ┌─────────────┐       ┌─────────────┐                            │
;   │   │ R G B R G B │       │ Z Z Z Z Z Z │                            │
;   │   │ pixel colors│       │ depth vals  │                            │
;   │   └─────────────┘       └─────────────┘                            │
;   │                                                                     │
;   │   When rendering pixel at (x,y) with depth z_new:                  │
;   │     if (z_new < z_buffer[x,y]):                                    │
;   │       framebuffer[x,y] = color                                     │
;   │       z_buffer[x,y] = z_new                                        │
;   │     else:                                                          │
;   │       pixel is occluded, skip                                      │
;   └─────────────────────────────────────────────────────────────────────┘
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE MOTOROLA 68020 CPU                               │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The M68020 (1984) was a 32-bit powerhouse used in high-end workstations,
; the Apple Macintosh II, and the Commodore Amiga 1200.
;
; KEY FEATURES FOR 3D GRAPHICS:
;
;   - Full 32-bit registers (D0-D7 data, A0-A7 address)
;   - Hardware multiply: MULS (signed 16×16→32), MULU (unsigned)
;   - Flexible addressing modes for array access
;   - MOVEM for fast register save/restore
;
; REGISTER USAGE IN THIS DEMO:
;
;   D0-D3: Temporary calculations, vertex coordinates
;   D4-D6: Vertex indices for triangle drawing
;   D7:    Loop counter
;   A0:    Source data pointer (vertices, faces)
;   A1:    Destination pointer (transformed vertices)
;   A2:    Sine table pointer
;   SP:    Stack pointer (A7)
;
; THE SIGNED ADVANTAGE:
;
; Unlike the unsigned-only IE32, the M68K has SIGNED arithmetic:
;   - MULS: Signed multiply (handles negative coordinates correctly)
;   - ASR: Arithmetic shift right (preserves sign bit)
;   - EXT: Sign-extend byte/word to long
;
; This makes 3D math much simpler - no offset tricks needed!
;
; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                      ARCHITECTURE OVERVIEW                             ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    3D GRAPHICS PIPELINE                                 │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   ┌───────────────┐
;   │ MODEL SPACE   │  Cube vertices defined relative to cube center
;   │ (Local Coords)│  e.g., corner at (-100, -100, -100)
;   └───────┬───────┘
;           │
;           │  ROTATION MATRICES (rotate_x, rotate_y, rotate_z)
;           │  Apply X, Y, Z rotations based on angle_x/y/z
;           ▼
;   ┌───────────────┐
;   │ WORLD SPACE   │  Vertices in world coordinates
;   │               │  (rotated, but not yet projected)
;   └───────┬───────┘
;           │
;           │  PROJECTION (orthographic in this demo)
;           │  Simply add screen center offset
;           ▼
;   ┌───────────────┐
;   │ SCREEN SPACE  │  2D coordinates ready for rendering
;   │               │  e.g., pixel at (320 + x, 240 + y)
;   └───────┬───────┘
;           │
;           │  RASTERIZATION (Voodoo GPU)
;           │  Convert triangles to pixels with z-test
;           ▼
;   ┌───────────────┐
;   │ FRAMEBUFFER   │  Final pixel colors displayed on screen
;   │               │
;   └───────────────┘
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    MAIN LOOP FLOW                                       │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   ╔═══════════════════════════════════════════════════════════════════╗
;   ║                        FRAME START                                ║
;   ╚═══════════════════════════════════════════════════════════════════╝
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 1. CLEAR FRAMEBUFFER                                              │
;   │    Set clear color to dark blue ($FF000040)                       │
;   │    Execute VOODOO_FAST_FILL_CMD                                   │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 2. DRAW CUBE (bsr draw_cube)                                      │
;   │    ├─ Transform 8 vertices (apply X, Y, Z rotations)              │
;   │    └─ Draw 6 faces (12 triangles total)                           │
;   │       ├─ Front (red)      ├─ Back (cyan)                          │
;   │       ├─ Top (green)      ├─ Bottom (magenta)                     │
;   │       └─ Left (blue)      └─ Right (yellow)                       │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 3. SWAP BUFFERS                                                   │
;   │    Execute VOODOO_SWAP_BUFFER_CMD                                 │
;   │    (Display rendered frame, get fresh back buffer)                │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ┌───────────────────────────────────────────────────────────────────┐
;   │ 4. UPDATE ROTATION ANGLES                                         │
;   │    angle_x += 1   (slow roll)                                     │
;   │    angle_y += 2   (faster yaw - main rotation)                    │
;   │    angle_z += 1   (slow pitch)                                    │
;   └───────────────────────────────────────────────────────────────────┘
;                                  │
;                                  ▼
;   ╔═══════════════════════════════════════════════════════════════════╗
;   ║                    LOOP TO FRAME START                            ║
;   ╚═══════════════════════════════════════════════════════════════════╝
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    THE CUBE GEOMETRY                                    │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   A cube has 8 vertices and 6 faces. Each face is a quad (4 vertices).
;   We render each quad as 2 triangles.
;
;   VERTEX NUMBERING:
;
;              0─────────1          Y-axis (up = negative)
;             ╱│        ╱│              │
;            ╱ │       ╱ │              │
;           4─────────5  │              │
;           │  │      │  │              └─────── X-axis (right = positive)
;           │  3──────│──2             ╱
;           │ ╱       │ ╱             ╱
;           │╱        │╱             Z-axis (forward = positive)
;           7─────────6
;
;   Vertex 0: (-100, -100, -100) = back-top-left
;   Vertex 1: (+100, -100, -100) = back-top-right
;   Vertex 2: (+100, +100, -100) = back-bottom-right
;   Vertex 3: (-100, +100, -100) = back-bottom-left
;   Vertex 4: (-100, -100, +100) = front-top-left
;   Vertex 5: (+100, -100, +100) = front-top-right
;   Vertex 6: (+100, +100, +100) = front-bottom-right
;   Vertex 7: (-100, +100, +100) = front-bottom-left
;
;   FACE DEFINITIONS (counter-clockwise winding when viewed from outside):
;
;   Front (+Z):  vertices 4, 5, 6, 7  →  triangles (4,5,6), (4,6,7)
;   Back  (-Z):  vertices 1, 0, 3, 2  →  triangles (1,0,3), (1,3,2)
;   Top   (-Y):  vertices 0, 1, 5, 4  →  triangles (0,1,5), (0,5,4)
;   Bottom(+Y):  vertices 7, 6, 2, 3  →  triangles (7,6,2), (7,2,3)
;   Left  (-X):  vertices 0, 4, 7, 3  →  triangles (0,4,7), (0,7,3)
;   Right (+X):  vertices 5, 1, 2, 6  →  triangles (5,1,2), (5,2,6)
;
; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                         CONSTANTS                                      ==
; ==                                                                        ==
; ============================================================================
; ============================================================================

    include "ie68.inc"

; ============================================================================
; SCREEN CONFIGURATION
; ============================================================================
; The Voodoo runs at 640x480. Screen center is where we place the cube.

; SCREEN_W and SCREEN_H are defined in ie68.inc (640x480)
CENTER_X        equ SCREEN_W/2      ; 320 - horizontal center
CENTER_Y        equ SCREEN_H/2      ; 240 - vertical center

; ============================================================================
; CUBE GEOMETRY
; ============================================================================
; CUBE_SIZE defines the distance from cube center to each face.
; Total cube width = CUBE_SIZE * 2 = 200 pixels.
; At 640x480, a 200-pixel cube fills a good portion of the screen.

CUBE_SIZE       equ 100             ; Half-width of cube (center to face)

; ============================================================================
; FIXED-POINT FORMATS
; ============================================================================
; The Voodoo uses fixed-point numbers for precise sub-pixel positioning.
;
; 12.4 FORMAT (vertex coordinates):
;   - 12 bits integer (0-4095 pixel range)
;   - 4 bits fraction (1/16 pixel precision)
;   - Multiply pixel value by 16 (shift left 4)
;
; 4.12 FORMAT (colors):
;   - 4 bits integer (0-15, but typically 0-1)
;   - 12 bits fraction
;   - $1000 = 1.0 (full intensity)

FP_12_4         equ 4               ; Shift amount for 12.4 format
FP_12_12        equ 12              ; Shift amount for 12.12 format (not used here)

; ============================================================================
; SINE TABLE CONFIGURATION
; ============================================================================
; We use a 256-entry sine table where index 0-255 maps to 0°-360°.
; Values are signed bytes: -128 to +127 representing -1.0 to +0.992.
;
; This allows efficient angle wrapping with AND #$FF.
; Cosine is obtained by adding 64 to the angle (90° phase shift).

SIN_TABLE_SIZE  equ 256             ; Entries in sine table (full circle)
SIN_SCALE       equ 128             ; Scale factor (max value = 127)

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                       PROGRAM ENTRY POINT                              ==
; ==                                                                        ==
; ============================================================================
; ============================================================================

    org     $1000                   ; Standard program start address

; ============================================================================
; START - Initialize Hardware and Enter Main Loop
; ============================================================================
; This is where execution begins. We:
;   1. Set up the stack pointer
;   2. Configure the Voodoo for 640x480 with z-buffering
;   3. Initialize rotation angles to zero
;   4. Enter the infinite main loop
; ============================================================================

start:
    ; ------------------------------------------------------------------------
    ; INITIALIZE STACK POINTER
    ; ------------------------------------------------------------------------
    ; The stack grows downward from high memory. We place it at $FF0000
    ; which is safely above our program and data areas.
    ; The M68K uses A7 (aliased as SP) for the stack pointer.
    ; ------------------------------------------------------------------------
    lea     $FF0000,sp              ; Stack at high memory

    ; ------------------------------------------------------------------------
    ; ENABLE VOODOO GRAPHICS CARD
    ; ------------------------------------------------------------------------
    ; The Voodoo is disabled by default - we must enable it before use.
    ; ------------------------------------------------------------------------
    move.l  #1,VOODOO_ENABLE

    ; ------------------------------------------------------------------------
    ; CONFIGURE VOODOO VIDEO DIMENSIONS
    ; ------------------------------------------------------------------------
    ; VOODOO_VIDEO_DIM expects (width << 16) | height in one 32-bit value.
    ; 640 << 16 = $02800000, OR with 480 = $028001E0
    ; The M68K shift operator conveniently handles this.
    ; ------------------------------------------------------------------------
    move.l  #(SCREEN_W<<16)|SCREEN_H,VOODOO_VIDEO_DIM

    ; ------------------------------------------------------------------------
    ; CONFIGURE FRAMEBUFFER/Z-BUFFER MODE
    ; ------------------------------------------------------------------------
    ; VOODOO_FBZ_MODE controls depth testing and color writing.
    ;
    ; Value $0630 sets these bits:
    ;   Bit 4 (0x10):  depth_enable    - Enable z-buffer testing
    ;   Bit 5 (0x20):  depth_function  - LESS (new z < stored z passes)
    ;   Bit 9 (0x200): rgb_write       - Write color to framebuffer
    ;   Bit 10(0x400): depth_write     - Write new z-values to z-buffer
    ;
    ; This enables proper hidden surface removal via z-buffering.
    ; ------------------------------------------------------------------------
    move.l  #$0630,VOODOO_FBZ_MODE  ; depth_enable | depth_less | rgb_write | depth_write

    ; ------------------------------------------------------------------------
    ; INITIALIZE ROTATION ANGLES
    ; ------------------------------------------------------------------------
    ; All three rotation angles start at zero. They are 16-bit values
    ; that increment each frame, wrapping naturally at 65536.
    ; Only the low 8 bits are used (masked with AND #$FF in rotation code).
    ; ------------------------------------------------------------------------
    clr.w   angle_x                 ; X rotation = 0
    clr.w   angle_y                 ; Y rotation = 0
    clr.w   angle_z                 ; Z rotation = 0

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                          MAIN LOOP                                     ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; The main loop runs continuously at approximately 60 FPS (limited by vsync).
; Each iteration:
;   1. Clears the framebuffer to dark blue
;   2. Draws the rotated cube (6 faces × 2 triangles = 12 triangles)
;   3. Swaps front and back buffers
;   4. Increments rotation angles for animation
;
; ============================================================================

main_loop:
    ; ------------------------------------------------------------------------
    ; CLEAR FRAMEBUFFER
    ; ------------------------------------------------------------------------
    ; VOODOO_COLOR0 sets the clear color in ARGB8888 format:
    ;   $FF000040 = Alpha=255, R=0, G=0, B=64 (dark blue)
    ;
    ; Writing any value to VOODOO_FAST_FILL_CMD triggers the fill.
    ; The Voodoo hardware fills the entire framebuffer in one operation.
    ; ------------------------------------------------------------------------
    move.l  #$FF000040,VOODOO_COLOR0    ; Dark blue clear color
    move.l  #0,VOODOO_FAST_FILL_CMD     ; Execute fast fill

    ; ------------------------------------------------------------------------
    ; DRAW THE ROTATING CUBE
    ; ------------------------------------------------------------------------
    ; The draw_cube subroutine:
    ;   1. Transforms all 8 vertices using current rotation angles
    ;   2. Draws all 6 faces (2 triangles each) with their colors
    ;
    ; Z-buffering ensures proper occlusion regardless of draw order.
    ; ------------------------------------------------------------------------
    bsr     draw_cube

    ; ------------------------------------------------------------------------
    ; SWAP FRONT AND BACK BUFFERS
    ; ------------------------------------------------------------------------
    ; Double-buffering prevents tearing. We draw to the back buffer while
    ; the front buffer is displayed. Swapping makes our new frame visible.
    ; The Voodoo automatically waits for vsync before swapping.
    ; ------------------------------------------------------------------------
    move.l  #1,VOODOO_SWAP_BUFFER_CMD

    ; ------------------------------------------------------------------------
    ; UPDATE ROTATION ANGLES
    ; ------------------------------------------------------------------------
    ; Different speeds for each axis create interesting tumbling motion:
    ;   X: +1 per frame (slow roll)
    ;   Y: +2 per frame (faster yaw - primary rotation axis)
    ;   Z: +1 per frame (slow pitch)
    ;
    ; At 60 FPS:
    ;   - Full X rotation: 256 frames ÷ 60 = 4.3 seconds
    ;   - Full Y rotation: 128 frames ÷ 60 = 2.1 seconds
    ;   - Full Z rotation: 256 frames ÷ 60 = 4.3 seconds
    ; ------------------------------------------------------------------------
    add.w   #1,angle_x              ; Increment X rotation
    add.w   #2,angle_y              ; Increment Y rotation (faster)
    add.w   #1,angle_z              ; Increment Z rotation

    bra     main_loop               ; Loop forever

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                       DRAW CUBE SUBROUTINE                             ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; Draw Cube - Transforms vertices and renders all 6 faces
;
; This subroutine orchestrates the cube rendering:
;   1. Transform all 8 vertices from model space to screen space
;   2. Draw each of the 6 faces with its designated color
;
; The faces are drawn in arbitrary order since z-buffering handles
; the hidden surface removal automatically. No depth sorting needed!
;
; FACE COLORS:
;   Front (+Z): Red      - The "main" face, most visible during rotation
;   Back  (-Z): Cyan     - Complementary to front (opposite on color wheel)
;   Top   (-Y): Green    - Classic "up" color
;   Bottom(+Y): Magenta  - Complementary to top
;   Left  (-X): Blue     - Classic "side" color
;   Right (+X): Yellow   - Complementary to left
;
; ============================================================================

draw_cube:
    movem.l d0-d7/a0-a6,-(sp)       ; Save all registers (we use many)

    ; ========================================================================
    ; STEP 1: TRANSFORM ALL VERTICES
    ; ========================================================================
    ; Apply the current rotation angles to all 8 cube vertices.
    ; This converts from model space (cube-relative) to world space (screen-relative).
    ; ========================================================================
    bsr     transform_vertices

    ; ========================================================================
    ; STEP 2: DRAW ALL 6 FACES
    ; ========================================================================
    ; Each face is drawn as 2 triangles with a solid color.
    ; We set face_r/g/b before calling draw_face.
    ;
    ; Color values are in 4.12 fixed-point:
    ;   $1000 = 1.0 (full intensity)
    ;   $0000 = 0.0 (zero intensity)
    ; ========================================================================

    ; --- FRONT FACE (+Z): RED ---
    ; Vertices 4, 5, 6, 7 (the face at z = +CUBE_SIZE)
    move.l  #$1000,face_r           ; R = 1.0
    move.l  #$0000,face_g           ; G = 0.0
    move.l  #$0000,face_b           ; B = 0.0
    lea     face_front,a0           ; Pointer to vertex indices
    bsr     draw_face

    ; --- BACK FACE (-Z): CYAN ---
    ; Vertices 1, 0, 3, 2 (the face at z = -CUBE_SIZE)
    ; Cyan = Green + Blue (complementary to red)
    move.l  #$0000,face_r           ; R = 0.0
    move.l  #$1000,face_g           ; G = 1.0
    move.l  #$1000,face_b           ; B = 1.0
    lea     face_back,a0
    bsr     draw_face

    ; --- TOP FACE (-Y): GREEN ---
    ; Vertices 0, 1, 5, 4 (the face at y = -CUBE_SIZE)
    move.l  #$0000,face_r           ; R = 0.0
    move.l  #$1000,face_g           ; G = 1.0
    move.l  #$0000,face_b           ; B = 0.0
    lea     face_top,a0
    bsr     draw_face

    ; --- BOTTOM FACE (+Y): MAGENTA ---
    ; Vertices 7, 6, 2, 3 (the face at y = +CUBE_SIZE)
    ; Magenta = Red + Blue (complementary to green)
    move.l  #$1000,face_r           ; R = 1.0
    move.l  #$0000,face_g           ; G = 0.0
    move.l  #$1000,face_b           ; B = 1.0
    lea     face_bottom,a0
    bsr     draw_face

    ; --- LEFT FACE (-X): BLUE ---
    ; Vertices 0, 4, 7, 3 (the face at x = -CUBE_SIZE)
    move.l  #$0000,face_r           ; R = 0.0
    move.l  #$0000,face_g           ; G = 0.0
    move.l  #$1000,face_b           ; B = 1.0
    lea     face_left,a0
    bsr     draw_face

    ; --- RIGHT FACE (+X): YELLOW ---
    ; Vertices 5, 1, 2, 6 (the face at x = +CUBE_SIZE)
    ; Yellow = Red + Green (complementary to blue)
    move.l  #$1000,face_r           ; R = 1.0
    move.l  #$1000,face_g           ; G = 1.0
    move.l  #$0000,face_b           ; B = 0.0
    lea     face_right,a0
    bsr     draw_face

    movem.l (sp)+,d0-d7/a0-a6       ; Restore all registers
    rts

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                  DRAW FACE AND TRIANGLE SUBROUTINES                    ==
; ==                                                                        ==
; ============================================================================
; ============================================================================

; ============================================================================
; DRAW FACE - Draws a quadrilateral as 2 triangles
; ============================================================================
;
; Input:
;   A0 = Pointer to face definition (4 bytes: vertex indices v0, v1, v2, v3)
;   face_r, face_g, face_b = Face color (set before calling)
;
; A quadrilateral is split into 2 triangles:
;
;   v0─────v1         Triangle 1: v0, v1, v2
;   │ ╲    │          Triangle 2: v0, v2, v3
;   │  ╲   │
;   │   ╲  │          This diagonal split works for any convex quad.
;   │    ╲ │
;   v3─────v2
;
; ============================================================================

draw_face:
    movem.l d0-d7/a0-a2,-(sp)       ; Save registers

    ; Load the 4 vertex indices from the face definition
    ; Each index is a single byte (0-7 for cube vertices)
    moveq   #0,d0                   ; Clear upper bits
    moveq   #0,d1
    moveq   #0,d2
    moveq   #0,d3
    move.b  (a0)+,d0                ; v0 - first vertex
    move.b  (a0)+,d1                ; v1 - second vertex
    move.b  (a0)+,d2                ; v2 - third vertex
    move.b  (a0)+,d3                ; v3 - fourth vertex

    ; Draw Triangle 1: v0, v1, v2
    move.l  d0,d4                   ; Vertex A index
    move.l  d1,d5                   ; Vertex B index
    move.l  d2,d6                   ; Vertex C index
    bsr     draw_triangle

    ; Draw Triangle 2: v0, v2, v3
    move.l  d0,d4                   ; Vertex A index (same as triangle 1)
    move.l  d2,d5                   ; Vertex B index
    move.l  d3,d6                   ; Vertex C index
    bsr     draw_triangle

    movem.l (sp)+,d0-d7/a0-a2       ; Restore registers
    rts

; ============================================================================
; DRAW TRIANGLE - Submit a single triangle to the Voodoo GPU
; ============================================================================
;
; Input:
;   D4 = Vertex A index (0-7)
;   D5 = Vertex B index (0-7)
;   D6 = Vertex C index (0-7)
;   face_r, face_g, face_b = Triangle color
;
; The Voodoo rendering pipeline:
;
;   1. Set color registers (START_R/G/B/A)
;   2. Set vertex A coordinates (VERTEX_AX/AY)
;   3. Set vertex B coordinates (VERTEX_BX/BY)
;   4. Set vertex C coordinates (VERTEX_CX/CY)
;   5. Set Z depth (START_Z)
;   6. Write to TRIANGLE_CMD to submit
;
; Coordinate conversion:
;   - Transformed vertices are in pixels relative to screen center
;   - Add CENTER_X/Y to get absolute screen position
;   - Multiply by 16 (shift left 4) for 12.4 fixed-point format
;
; ============================================================================

draw_triangle:
    movem.l d0-d7/a0,-(sp)          ; Save registers

    lea     transformed_verts,a0    ; Base address of transformed vertex array

    ; ========================================================================
    ; VERTEX A (from index in D4)
    ; ========================================================================
    ; Calculate offset: index × 12 bytes (3 longs: x, y, z)
    move.l  d4,d0
    mulu    #12,d0                  ; D0 = byte offset into vertex array
    move.l  0(a0,d0.l),d1           ; D1 = X coordinate
    move.l  4(a0,d0.l),d2           ; D2 = Y coordinate
    move.l  8(a0,d0.l),d3           ; D3 = Z coordinate (for z-buffer)

    ; Convert to screen coordinates and 12.4 fixed-point
    add.l   #CENTER_X,d1            ; Add horizontal offset (320)
    add.l   #CENTER_Y,d2            ; Add vertical offset (240)
    lsl.l   #FP_12_4,d1             ; Convert to 12.4: x × 16
    lsl.l   #FP_12_4,d2             ; Convert to 12.4: y × 16

    ; Submit vertex A to Voodoo
    move.l  d1,VOODOO_VERTEX_AX
    move.l  d2,VOODOO_VERTEX_AY

    ; ========================================================================
    ; VERTEX B (from index in D5)
    ; ========================================================================
    move.l  d5,d0
    mulu    #12,d0
    move.l  0(a0,d0.l),d1
    move.l  4(a0,d0.l),d2
    add.l   #CENTER_X,d1
    add.l   #CENTER_Y,d2
    lsl.l   #FP_12_4,d1
    lsl.l   #FP_12_4,d2

    move.l  d1,VOODOO_VERTEX_BX
    move.l  d2,VOODOO_VERTEX_BY

    ; ========================================================================
    ; VERTEX C (from index in D6)
    ; ========================================================================
    move.l  d6,d0
    mulu    #12,d0
    move.l  0(a0,d0.l),d1
    move.l  4(a0,d0.l),d2
    move.l  8(a0,d0.l),d7           ; Keep Z for depth calculation
    add.l   #CENTER_X,d1
    add.l   #CENTER_Y,d2
    lsl.l   #FP_12_4,d1
    lsl.l   #FP_12_4,d2

    move.l  d1,VOODOO_VERTEX_CX
    move.l  d2,VOODOO_VERTEX_CY

    ; ========================================================================
    ; SET TRIANGLE COLOR
    ; ========================================================================
    ; Colors are in 4.12 fixed-point. $1000 = 1.0 (full intensity).
    ; Alpha is always 1.0 (fully opaque).
    move.l  face_r,VOODOO_START_R
    move.l  face_g,VOODOO_START_G
    move.l  face_b,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A   ; Alpha = 1.0 (opaque)

    ; ========================================================================
    ; SET Z DEPTH FOR Z-BUFFER
    ; ========================================================================
    ; Z values from rotation are in range -CUBE_SIZE to +CUBE_SIZE.
    ; The z-buffer needs positive values, so we add CUBE_SIZE*2 to shift
    ; the range from [-100,+100] to [0,+400].
    ;
    ; We then scale by 256 (shift left 8) to get more z-buffer precision.
    ; Larger Z = farther away, Smaller Z = closer.
    ; ========================================================================
    add.l   #CUBE_SIZE*2,d7         ; Shift to positive range [0, 400]
    lsl.l   #8,d7                   ; Scale for z-buffer precision
    move.l  d7,VOODOO_START_Z       ; Set depth for z-test

    ; ========================================================================
    ; SUBMIT TRIANGLE TO GPU
    ; ========================================================================
    ; Writing any value to VOODOO_TRIANGLE_CMD triggers rasterization.
    ; The GPU fills all pixels inside the triangle, testing each against
    ; the z-buffer to handle occlusion.
    ; ========================================================================
    move.l  #0,VOODOO_TRIANGLE_CMD  ; Render the triangle!

    movem.l (sp)+,d0-d7/a0          ; Restore registers
    rts

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                    VERTEX TRANSFORMATION                               ==
; ==                                                                        ==
; ============================================================================
; ============================================================================

; ============================================================================
; TRANSFORM VERTICES - Apply rotation to all 8 cube vertices
; ============================================================================
;
; This subroutine reads the 8 original cube vertices, applies X, Y, and Z
; axis rotations, and stores the results in the transformed_verts array.
;
; The rotation order is: Y first, then X, then Z.
; Different orders produce different tumbling patterns.
;
; INPUT:
;   angle_x, angle_y, angle_z - Current rotation angles (0-255)
;   cube_vertices - Original vertex positions (8 vertices × 3 words)
;
; OUTPUT:
;   transformed_verts - Rotated vertex positions (8 vertices × 3 longs)
;
; ============================================================================

transform_vertices:
    movem.l d0-d7/a0-a2,-(sp)       ; Save registers

    lea     cube_vertices,a0        ; Source: original vertices
    lea     transformed_verts,a1    ; Destination: rotated vertices

    moveq   #7,d7                   ; Loop counter: 8 vertices (7 down to 0)

.transform_loop:
    ; ------------------------------------------------------------------------
    ; LOAD VERTEX COORDINATES
    ; ------------------------------------------------------------------------
    ; Original vertices are stored as 16-bit signed words.
    ; We extend them to 32-bit longs for the rotation math.
    ; ------------------------------------------------------------------------
    move.w  (a0)+,d0                ; X coordinate (16-bit)
    move.w  (a0)+,d1                ; Y coordinate (16-bit)
    move.w  (a0)+,d2                ; Z coordinate (16-bit)
    ext.l   d0                      ; Sign-extend X to 32-bit
    ext.l   d1                      ; Sign-extend Y to 32-bit
    ext.l   d2                      ; Sign-extend Z to 32-bit

    ; ------------------------------------------------------------------------
    ; APPLY ROTATIONS (Y, then X, then Z)
    ; ------------------------------------------------------------------------
    ; Each rotation subroutine:
    ;   - Takes D0=x, D1=y, D2=z, D3=angle
    ;   - Returns D0=x', D1=y', D2=z' (rotated coordinates)
    ; ------------------------------------------------------------------------

    ; Rotate around Y axis (yaw - left/right turn)
    move.w  angle_y,d3
    bsr     rotate_y

    ; Rotate around X axis (pitch - look up/down)
    move.w  angle_x,d3
    bsr     rotate_x

    ; Rotate around Z axis (roll - tilt head)
    move.w  angle_z,d3
    bsr     rotate_z

    ; ------------------------------------------------------------------------
    ; STORE TRANSFORMED VERTEX
    ; ------------------------------------------------------------------------
    ; Transformed coordinates are stored as 32-bit values for precision.
    ; The draw_triangle routine will add CENTER_X/Y for screen positioning.
    ; ------------------------------------------------------------------------
    move.l  d0,(a1)+                ; Store X'
    move.l  d1,(a1)+                ; Store Y'
    move.l  d2,(a1)+                ; Store Z'

    dbf     d7,.transform_loop      ; Process next vertex (decrement and branch)

    movem.l (sp)+,d0-d7/a0-a2       ; Restore registers
    rts

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                      ROTATION SUBROUTINES                              ==
; ==                                                                        ==
; ============================================================================
; ============================================================================
;
; These subroutines implement 3D rotation around each axis using the
; standard rotation matrices. Each rotation is performed using a sine
; table lookup and fixed-point multiplication.
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    ROTATION MATRICES                                    │
; └─────────────────────────────────────────────────────────────────────────┘
;
; ROTATION AROUND X AXIS (pitch):
;
;   ┌    ┐   ┌                 ┐ ┌   ┐
;   │ x' │   │ 1    0      0   │ │ x │
;   │ y' │ = │ 0   cos   -sin  │ │ y │
;   │ z' │   │ 0   sin    cos  │ │ z │
;   └    ┘   └                 ┘ └   ┘
;
;   x' = x
;   y' = y×cos(θ) - z×sin(θ)
;   z' = y×sin(θ) + z×cos(θ)
;
; ROTATION AROUND Y AXIS (yaw):
;
;   ┌    ┐   ┌                 ┐ ┌   ┐
;   │ x' │   │  cos   0   sin  │ │ x │
;   │ y' │ = │   0    1    0   │ │ y │
;   │ z' │   │ -sin   0   cos  │ │ z │
;   └    ┘   └                 ┘ └   ┘
;
;   x' = x×cos(θ) + z×sin(θ)
;   y' = y
;   z' = -x×sin(θ) + z×cos(θ)
;
; ROTATION AROUND Z AXIS (roll):
;
;   ┌    ┐   ┌                 ┐ ┌   ┐
;   │ x' │   │ cos   -sin   0  │ │ x │
;   │ y' │ = │ sin    cos   0  │ │ y │
;   │ z' │   │  0      0    1  │ │ z │
;   └    ┘   └                 ┘ └   ┘
;
;   x' = x×cos(θ) - y×sin(θ)
;   y' = x×sin(θ) + y×cos(θ)
;   z' = z
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    FIXED-POINT MULTIPLICATION                           │
; └─────────────────────────────────────────────────────────────────────────┘
;
; Sine values are scaled to -128..+127 (representing -1.0 to +0.992).
; After multiplying coordinate × sin (using MULS), we divide by 128
; (using ASR #7) to get back to the original scale.
;
; Example: x=100, sin=64 (≈0.5)
;   x × sin = 100 × 64 = 6400
;   6400 >> 7 = 50  (≈ 100 × 0.5)
;
; ============================================================================

; ============================================================================
; ROTATE_X - Rotate around X axis (pitch)
; ============================================================================
; Input:  D0=x, D1=y, D2=z, D3=angle (0-255)
; Output: D0=x (unchanged), D1=y', D2=z'
;
; Formula:
;   y' = y×cos(θ) - z×sin(θ)
;   z' = y×sin(θ) + z×cos(θ)
; ============================================================================

rotate_x:
    movem.l d4-d6,-(sp)             ; Save work registers
    and.w   #$FF,d3                 ; Mask angle to 0-255

    ; Get sin and cos values from table
    lea     sin_table,a2
    move.b  0(a2,d3.w),d4           ; D4 = sin(angle)
    ext.w   d4                      ; Sign-extend to 16-bit
    add.w   #64,d3                  ; Add 90° for cosine (cos = sin(θ+90°))
    and.w   #$FF,d3                 ; Wrap to 0-255
    move.b  0(a2,d3.w),d5           ; D5 = cos(angle)
    ext.w   d5                      ; Sign-extend to 16-bit

    ; Calculate y' = y×cos - z×sin
    move.l  d1,d6                   ; D6 = y (copy)
    muls    d5,d6                   ; D6 = y × cos
    move.l  d2,-(sp)                ; Save z (we'll modify it)
    muls    d4,d2                   ; D2 = z × sin
    sub.l   d2,d6                   ; D6 = y×cos - z×sin
    asr.l   #7,d6                   ; Scale back (divide by 128)
    move.l  (sp)+,d2                ; Restore original z

    ; Calculate z' = y×sin + z×cos
    muls    d4,d1                   ; D1 = y × sin
    muls    d5,d2                   ; D2 = z × cos
    add.l   d1,d2                   ; D2 = y×sin + z×cos
    asr.l   #7,d2                   ; Scale back

    move.l  d6,d1                   ; D1 = y' (final result)
    ; D0 unchanged (x' = x)
    ; D2 = z' (already computed)

    movem.l (sp)+,d4-d6             ; Restore work registers
    rts

; ============================================================================
; ROTATE_Y - Rotate around Y axis (yaw)
; ============================================================================
; Input:  D0=x, D1=y, D2=z, D3=angle (0-255)
; Output: D0=x', D1=y (unchanged), D2=z'
;
; Formula:
;   x' = x×cos(θ) + z×sin(θ)
;   z' = -x×sin(θ) + z×cos(θ)
; ============================================================================

rotate_y:
    movem.l d4-d6,-(sp)
    and.w   #$FF,d3

    lea     sin_table,a2
    move.b  0(a2,d3.w),d4           ; sin
    ext.w   d4
    add.w   #64,d3
    and.w   #$FF,d3
    move.b  0(a2,d3.w),d5           ; cos
    ext.w   d5

    ; Calculate x' = x×cos + z×sin
    move.l  d0,d6
    muls    d5,d6                   ; x × cos
    move.l  d2,-(sp)
    muls    d4,d2                   ; z × sin
    add.l   d2,d6                   ; x×cos + z×sin
    asr.l   #7,d6
    move.l  (sp)+,d2

    ; Calculate z' = -x×sin + z×cos
    muls    d4,d0                   ; x × sin
    neg.l   d0                      ; -(x × sin)
    muls    d5,d2                   ; z × cos
    add.l   d0,d2                   ; z×cos - x×sin
    asr.l   #7,d2

    move.l  d6,d0                   ; x' -> d0
    ; D1 unchanged (y' = y)

    movem.l (sp)+,d4-d6
    rts

; ============================================================================
; ROTATE_Z - Rotate around Z axis (roll)
; ============================================================================
; Input:  D0=x, D1=y, D2=z, D3=angle (0-255)
; Output: D0=x', D1=y', D2=z (unchanged)
;
; Formula:
;   x' = x×cos(θ) - y×sin(θ)
;   y' = x×sin(θ) + y×cos(θ)
; ============================================================================

rotate_z:
    movem.l d4-d6,-(sp)
    and.w   #$FF,d3

    lea     sin_table,a2
    move.b  0(a2,d3.w),d4           ; sin
    ext.w   d4
    add.w   #64,d3
    and.w   #$FF,d3
    move.b  0(a2,d3.w),d5           ; cos
    ext.w   d5

    ; Calculate x' = x×cos - y×sin
    move.l  d0,d6
    muls    d5,d6                   ; x × cos
    move.l  d1,-(sp)
    muls    d4,d1                   ; y × sin
    sub.l   d1,d6                   ; x×cos - y×sin
    asr.l   #7,d6
    move.l  (sp)+,d1

    ; Calculate y' = x×sin + y×cos
    muls    d4,d0                   ; x × sin
    muls    d5,d1                   ; y × cos
    add.l   d0,d1                   ; x×sin + y×cos
    asr.l   #7,d1

    move.l  d6,d0                   ; x' -> d0
    ; D2 unchanged (z' = z)

    movem.l (sp)+,d4-d6
    rts

; ============================================================================
; ============================================================================
; ==                                                                        ==
; ==                         DATA SECTION                                   ==
; ==                                                                        ==
; ============================================================================
; ============================================================================

    even                            ; Ensure word alignment

; ============================================================================
; RUNTIME VARIABLES
; ============================================================================

; Rotation angles (0-255 maps to 0°-360°)
; These increment each frame to create animation
angle_x:    dc.w    0               ; X axis rotation (pitch)
angle_y:    dc.w    0               ; Y axis rotation (yaw)
angle_z:    dc.w    0               ; Z axis rotation (roll)

; Current face color (4.12 fixed-point: $1000 = 1.0)
; Set before each draw_face call
face_r:     dc.l    0               ; Red component
face_g:     dc.l    0               ; Green component
face_b:     dc.l    0               ; Blue component

; ============================================================================
; CUBE VERTEX DATA
; ============================================================================
; The cube is defined by 8 vertices (corners) in model space.
; Each vertex has X, Y, Z coordinates as 16-bit signed words.
; The cube is centered at the origin with size ±CUBE_SIZE on each axis.
;
;              0─────────1          Coordinate System:
;             ╱│        ╱│
;            ╱ │       ╱ │          +Y
;           4─────────5  │           │    +Z
;           │  │      │  │           │   ╱
;           │  3──────│──2           │  ╱
;           │ ╱       │ ╱            │ ╱
;           │╱        │╱             │╱
;           7─────────6              └───────── +X
;
; ============================================================================
cube_vertices:
    dc.w    -CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 0: back-top-left
    dc.w     CUBE_SIZE, -CUBE_SIZE, -CUBE_SIZE  ; 1: back-top-right
    dc.w     CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 2: back-bottom-right
    dc.w    -CUBE_SIZE,  CUBE_SIZE, -CUBE_SIZE  ; 3: back-bottom-left
    dc.w    -CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 4: front-top-left
    dc.w     CUBE_SIZE, -CUBE_SIZE,  CUBE_SIZE  ; 5: front-top-right
    dc.w     CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 6: front-bottom-right
    dc.w    -CUBE_SIZE,  CUBE_SIZE,  CUBE_SIZE  ; 7: front-bottom-left

; ============================================================================
; FACE DEFINITIONS
; ============================================================================
; Each face is a quadrilateral defined by 4 vertex indices.
; Winding order is counter-clockwise when viewed from outside the cube.
; Each face is rendered as 2 triangles: (v0,v1,v2) and (v0,v2,v3).
;
; FACE COLORS (set in draw_cube):
;   Front: Red      Back: Cyan
;   Top:   Green    Bottom: Magenta
;   Left:  Blue     Right: Yellow
; ============================================================================
face_front:     dc.b    4, 5, 6, 7      ; Front face (+Z) - towards viewer
face_back:      dc.b    1, 0, 3, 2      ; Back face (-Z)  - away from viewer
face_top:       dc.b    0, 1, 5, 4      ; Top face (-Y)   - above
face_bottom:    dc.b    7, 6, 2, 3      ; Bottom face (+Y)- below
face_left:      dc.b    0, 4, 7, 3      ; Left face (-X)  - to the left
face_right:     dc.b    5, 1, 2, 6      ; Right face (+X) - to the right

    even                            ; Ensure word alignment after bytes

; ============================================================================
; TRANSFORMED VERTICES
; ============================================================================
; Working buffer for rotated vertex positions.
; 8 vertices × 3 coordinates × 4 bytes = 96 bytes.
; Filled by transform_vertices, read by draw_triangle.
; ============================================================================
transformed_verts:
    ds.l    24                      ; 8 vertices × 3 longs (x, y, z)

; ============================================================================
; SINE TABLE
; ============================================================================
; 256-entry sine table with values -128 to +127.
; Index 0-255 maps to angles 0°-360°.
;
; Values are: round(sin(index × 2π / 256) × 127)
;
; To get cosine: add 64 to the index (90° phase shift)
;   cos(θ) = sin(θ + 90°) = sin_table[(index + 64) & 0xFF]
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │  +127 ─────╮                         ╭─────                            │
; │            ╲                       ╱                                   │
; │             ╲                     ╱                                    │
; │    0 ───────┼─────────────────┼────── (zero crossing)                 │
; │             ╱                     ╲                                    │
; │            ╱                       ╲                                   │
; │  -128 ────╯                         ╰─────                            │
; │        0       64      128      192     255  (index)                  │
; │        0°      90°     180°     270°    360° (angle)                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
; The table is split into 4 quadrants:
;   Quadrant 0 (0-63):   Values rise from 0 to +127
;   Quadrant 1 (64-127): Values fall from +127 to 0
;   Quadrant 2 (128-191): Values fall from 0 to -127
;   Quadrant 3 (192-255): Values rise from -127 to 0
; ============================================================================
sin_table:
    ; Quadrant 0: 0° to 90° (indices 0-63) - rising from 0 to +127
    dc.b      0,   3,   6,   9,  12,  16,  19,  22
    dc.b     25,  28,  31,  34,  37,  40,  43,  46
    dc.b     49,  51,  54,  57,  60,  63,  65,  68
    dc.b     71,  73,  76,  78,  81,  83,  85,  88
    dc.b     90,  92,  94,  96,  98, 100, 102, 104
    dc.b    106, 107, 109, 111, 112, 113, 115, 116
    dc.b    117, 118, 120, 121, 122, 122, 123, 124
    dc.b    125, 125, 126, 126, 126, 127, 127, 127

    ; Quadrant 1: 90° to 180° (indices 64-127) - falling from +127 to 0
    dc.b    127, 127, 127, 127, 126, 126, 126, 125
    dc.b    125, 124, 123, 122, 122, 121, 120, 118
    dc.b    117, 116, 115, 113, 112, 111, 109, 107
    dc.b    106, 104, 102, 100,  98,  96,  94,  92
    dc.b     90,  88,  85,  83,  81,  78,  76,  73
    dc.b     71,  68,  65,  63,  60,  57,  54,  51
    dc.b     49,  46,  43,  40,  37,  34,  31,  28
    dc.b     25,  22,  19,  16,  12,   9,   6,   3

    ; Quadrant 2: 180° to 270° (indices 128-191) - falling from 0 to -127
    dc.b      0,  -3,  -6,  -9, -12, -16, -19, -22
    dc.b    -25, -28, -31, -34, -37, -40, -43, -46
    dc.b    -49, -51, -54, -57, -60, -63, -65, -68
    dc.b    -71, -73, -76, -78, -81, -83, -85, -88
    dc.b    -90, -92, -94, -96, -98,-100,-102,-104
    dc.b   -106,-107,-109,-111,-112,-113,-115,-116
    dc.b   -117,-118,-120,-121,-122,-122,-123,-124
    dc.b   -125,-125,-126,-126,-126,-127,-127,-127

    ; Quadrant 3: 270° to 360° (indices 192-255) - rising from -127 to 0
    dc.b   -127,-127,-127,-127,-126,-126,-126,-125
    dc.b   -125,-124,-123,-122,-122,-121,-120,-118
    dc.b   -117,-116,-115,-113,-112,-111,-109,-107
    dc.b   -106,-104,-102,-100, -98, -96, -94, -92
    dc.b    -90, -88, -85, -83, -81, -78, -76, -73
    dc.b    -71, -68, -65, -63, -60, -57, -54, -51
    dc.b    -49, -46, -43, -40, -37, -34, -31, -28
    dc.b    -25, -22, -19, -16, -12,  -9,  -6,  -3

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
;   Code size:        ~800 bytes
;   Sine table:       256 bytes
;   Vertex data:      48 bytes (original) + 96 bytes (transformed)
;   Total binary:     ~1.3 KB
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    PERFORMANCE                                          │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   Triangles per frame: 12 (6 faces × 2 triangles)
;   Vertex transforms:   8 (with 3 rotation passes each)
;   Frame rate:          60 FPS (vsync limited)
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    CUSTOMIZATION IDEAS                                  │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   - Change CUBE_SIZE for a larger or smaller cube
;   - Modify rotation speeds (add.w #N,angle_x/y/z)
;   - Add perspective projection (divide x,y by z)
;   - Implement backface culling (skip faces pointing away)
;   - Add more shapes (tetrahedron, pyramid, etc.)
;   - Implement Gouraud shading (per-vertex colors)
;   - Add keyboard control for interactive rotation
;
; ┌─────────────────────────────────────────────────────────────────────────┐
; │                    FURTHER READING                                      │
; └─────────────────────────────────────────────────────────────────────────┘
;
;   - "Computer Graphics: Principles and Practice" (Foley, van Dam, et al.)
;   - "3D Math Primer for Graphics and Game Development" (Dunn & Parberry)
;   - "Real-Time Rendering" (Akenine-Möller, Haines, Hoffman)
;
; ============================================================================

    end
