; ============================================================================
; VOODOO TRIANGLE - Minimal 3DFX Triangle Rasterisation Demo
; M68K (68020) for IntuitionEngine - Voodoo 3D (640x480, triangle rasteriser)
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:     Motorola 68020
; Video chip:     3DFX Voodoo SST-1 (hardware 3D, Z-buffer, triangle rasteriser)
; Audio engine:   None
; Assembler:      vasmm68k_mot (VASM M68K, Motorola syntax)
; Include file:   ie68.inc (Voodoo MMIO register definitions)
;
; === WHAT THIS DEMO DOES ===
; Draws three overlapping coloured triangles -- red, green, and blue -- each
; at a different Z depth.  The hardware Z-buffer ensures correct occlusion:
; green (closest) obscures red (middle), which obscures blue (farthest).
; The scene is static; the frame is redrawn every vsync to demonstrate the
; Voodoo's double-buffered rendering pipeline.
;
; === WHY TRIANGLE RASTERISATION ===
; The triangle is the fundamental primitive of all 3D rendering.  Every
; polygon -- quads, meshes, entire game worlds -- is ultimately decomposed
; into triangles before rasterisation.  The 3dfx Voodoo (1996) was the first
; consumer hardware to rasterise triangles at 50 million pixels per second,
; a task that previously required expensive SGI workstations.
;
; This demo is the "Hello World" of Voodoo programming: set up three vertices,
; choose a colour, and submit the triangle.  Understanding this pipeline is
; the foundation for every Voodoo demo in the SDK.
;
; === VOODOO RENDERING PIPELINE ===
;
;   1. Set colour        VOODOO_START_R/G/B/A  (4.12 fixed-point, $1000 = 1.0)
;   2. Set Z depth       VOODOO_START_Z         (for Z-buffer test)
;   3. Set vertex A      VOODOO_VERTEX_AX/AY   (12.4 fixed-point, pixel * 16)
;   4. Set vertex B      VOODOO_VERTEX_BX/BY
;   5. Set vertex C      VOODOO_VERTEX_CX/CY
;   6. Submit triangle   VOODOO_TRIANGLE_CMD    (any write triggers rasterisation)
;
; === FIXED-POINT FORMATS ===
;   12.4  (vertex coords):  pixel_value * 16          e.g. 320 -> 5120
;   4.12  (colours):        $1000 = 1.0, $0800 = 0.5
;   20.12 (Z depth):        lower value = closer to the camera
;
; === MEMORY MAP ===
;   $001000       Program code (org)
;   $0F0000+      Voodoo MMIO registers (defined in ie68.inc)
;   $FF0000       Stack (grows downward)
;
; === BUILD AND RUN ===
;   vasmm68k_mot -Fbin -m68020 -devpac -o voodoo_triangle_68k.ie68 voodoo_triangle_68k.asm
;   ./bin/IntuitionEngine -m68k voodoo_triangle_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    include "ie68.inc"

    org     $1000

; ----------------------------------------------------------------------------
; Initialisation -- stack, Voodoo enable, display dimensions, Z-buffer mode
; ----------------------------------------------------------------------------

start:
    lea     $FF0000,sp

    move.l  #1,VOODOO_ENABLE

    move.l  #(640<<16)|480,VOODOO_VIDEO_DIM

    ; fbzMode: depth_enable | depth_less | rgb_write | depth_write = $0630
    move.l  #$0630,VOODOO_FBZ_MODE

; ============================================================================
; Main loop -- clear, draw three triangles at different depths, swap buffers
; ============================================================================

main_loop:
    ; --- Clear framebuffer to dark blue ---
    move.l  #$FF000080,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    ; ======================================================================
    ; Triangle 1: RED -- middle depth (Z = $800)
    ; ======================================================================
    ; WHY: the red triangle sits between the green and blue in Z-space.
    ; Because the Z-buffer is enabled, draw order does not matter -- the
    ; hardware tests each pixel against the stored depth value.
    ; ======================================================================

    ; Vertex A: top centre (320, 100)
    move.l  #(320<<4),VOODOO_VERTEX_AX
    move.l  #(100<<4),VOODOO_VERTEX_AY

    ; Vertex B: bottom right (500, 400)
    move.l  #(500<<4),VOODOO_VERTEX_BX
    move.l  #(400<<4),VOODOO_VERTEX_BY

    ; Vertex C: bottom left (140, 400)
    move.l  #(140<<4),VOODOO_VERTEX_CX
    move.l  #(400<<4),VOODOO_VERTEX_CY

    ; Colour: bright red (R=1.0, G=0.0, B=0.0, A=1.0)
    move.l  #$1000,VOODOO_START_R
    move.l  #$0000,VOODOO_START_G
    move.l  #$0000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A

    ; Z depth = $800 (0.5 in 20.12 -- middle distance)
    move.l  #$800,VOODOO_START_Z

    move.l  #0,VOODOO_TRIANGLE_CMD

    ; ======================================================================
    ; Triangle 2: GREEN -- closest (Z = $4CC, approx. 0.3)
    ; ======================================================================
    ; WHY: lower Z = closer to the camera.  This triangle will obscure any
    ; red or blue pixels it overlaps, regardless of draw order.
    ; ======================================================================

    ; Vertex A: (400, 150)
    move.l  #(400<<4),VOODOO_VERTEX_AX
    move.l  #(150<<4),VOODOO_VERTEX_AY

    ; Vertex B: (550, 350)
    move.l  #(550<<4),VOODOO_VERTEX_BX
    move.l  #(350<<4),VOODOO_VERTEX_BY

    ; Vertex C: (250, 350)
    move.l  #(250<<4),VOODOO_VERTEX_CX
    move.l  #(350<<4),VOODOO_VERTEX_CY

    ; Colour: bright green
    move.l  #$0000,VOODOO_START_R
    move.l  #$1000,VOODOO_START_G
    move.l  #$0000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A

    ; Z depth = $4CC (closer than red)
    move.l  #$4CC,VOODOO_START_Z

    move.l  #0,VOODOO_TRIANGLE_CMD

    ; ======================================================================
    ; Triangle 3: BLUE -- farthest (Z = $E66, approx. 0.9)
    ; ======================================================================
    ; WHY: highest Z = farthest away.  Blue pixels only survive where they
    ; are not covered by the red or green triangles.
    ; ======================================================================

    ; Vertex A: (320, 50)
    move.l  #(320<<4),VOODOO_VERTEX_AX
    move.l  #(50<<4),VOODOO_VERTEX_AY

    ; Vertex B: (580, 300)
    move.l  #(580<<4),VOODOO_VERTEX_BX
    move.l  #(300<<4),VOODOO_VERTEX_BY

    ; Vertex C: (60, 300)
    move.l  #(60<<4),VOODOO_VERTEX_CX
    move.l  #(300<<4),VOODOO_VERTEX_CY

    ; Colour: bright blue
    move.l  #$0000,VOODOO_START_R
    move.l  #$0000,VOODOO_START_G
    move.l  #$1000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A

    ; Z depth = $E66 (farthest)
    move.l  #$E66,VOODOO_START_Z

    move.l  #0,VOODOO_TRIANGLE_CMD

    ; --- Swap buffers (vsync wait built into the Voodoo) ---
    move.l  #1,VOODOO_SWAP_BUFFER_CMD

    bra     main_loop

    end
