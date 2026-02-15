; ============================================================================
; VOODOO TUNNEL DEMO - 3D TEXTURED TUNNEL ON Z80
; Z80 Assembly for IntuitionEngine - 3DFX Voodoo Graphics Accelerator
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Zilog Z80 (8-bit, 8MHz)
; Video Chip:    3DFX Voodoo (SST-1 compatible, hardware-accelerated 3D)
; Audio Engine:  None (visual-only demo)
; Assembler:     vasmz80_std (VASM Z80, standard syntax)
; Build:         vasmz80_std -Fbin -o voodoo_tunnel_z80.ie80 voodoo_tunnel_z80.asm
; Run:           bin/IntuitionEngine -z80 voodoo_tunnel_z80.ie80
; Porting:       Voodoo register interface is CPU-agnostic. Port effort:
;                replace Z80 I/O port writes with memory-mapped writes for
;                other CPUs. The maths routines (sin/cos, multiply, divide)
;                need rewriting for each target architecture.
;
; === WHAT THIS DEMO DOES ===
; 1. Initialises the Voodoo 3D accelerator at 640x480 with depth testing
; 2. Uploads a 64x64 RGBA texture (Amiga Boing-style checker pattern)
; 3. Generates a tunnel of 12 octagonal rings receding into the distance
; 4. Each ring is connected to the previous by textured, depth-shaded quads
; 5. The tunnel rotates, twists, and zooms forward continuously
; 6. Double-buffered with vsync for tear-free 3D rendering
;
; === WHY 3D TUNNEL RENDERING ON A Z80 ===
;
; The Zilog Z80 is an 8-bit processor from 1976, running at 4-8 MHz with
; no hardware multiply, no floating point, and only 16-bit address space.
; Rendering a 3D textured tunnel on such hardware would normally be
; impossible - but the Voodoo Graphics accelerator changes everything.
;
; The Voodoo chip (3DFX SST-1, 1996) was the first mass-market 3D
; accelerator for PCs. It handles:
;   - Triangle rasterisation with sub-pixel precision
;   - Perspective-correct texture mapping
;   - Per-pixel depth testing (Z-buffer)
;   - Gouraud shading (per-vertex colour interpolation)
;   - Double buffering with hardware page flip
;
; The Z80's job is reduced to GEOMETRY ONLY: computing vertex positions
; and submitting triangles. The Voodoo does all the heavy per-pixel work.
; This is the same CPU/GPU division of labour used in modern graphics,
; just taken to an extreme - a 1976 CPU driving a 1996 GPU.
;
; === WHY I/O PORTS FOR VOODOO ACCESS ===
;
; The Z80's address bus is only 16 bits (64KB), which is too small to
; memory-map the Voodoo's register space alongside programme memory.
; Instead, the Intuition Engine provides a set of I/O ports:
;
;   Port 0x40: Address low byte    Port 0x44: Data byte 0 (LSB)
;   Port 0x41: Address high byte   Port 0x45: Data byte 1
;                                  Port 0x46: Data byte 2
;                                  Port 0x47: Data byte 3 (MSB, triggers write)
;
; Writing byte 3 triggers the actual 32-bit register write. This means
; every Voodoo operation requires 6 I/O port writes from the Z80.
;
; === TUNNEL GEOMETRY ===
;
; The tunnel is constructed from 12 concentric rings, each an octagon
; (8 vertices). Adjacent rings are connected by quadrilaterals, each
; split into 2 triangles for Voodoo submission.
;
; Perspective is computed per-ring: closer rings have larger radii.
; A twist offset rotates each ring slightly relative to its neighbour,
; creating a spiral effect. Depth-based brightness darkens distant
; rings for a natural fog-like fade.
;
; === MEMORY MAP ===
;   0x0000-0x00FF   Entry point and jump to main code
;   0x0100-0x2FFF   Programme code
;   0x3000-0x30FF   Sine table (256 signed bytes)
;   0x3200-0x32FF   Current ring vertex buffer (8 vertices x 8 bytes)
;   0x3300-0x33FF   Previous ring vertex buffer (8 vertices x 8 bytes)
;   0x3400-0x340F   Animation variables
;   0x4000+         Texture data (64x64 RGBA, loaded via .incbin)
;   0xE000          Stack (grows downward)
;
; === BUILD AND RUN ===
;   vasmz80_std -Fbin -o voodoo_tunnel_z80.ie80 voodoo_tunnel_z80.asm
;   bin/IntuitionEngine -z80 voodoo_tunnel_z80.ie80
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .include "ie80.inc"

; ============================================================================
; CONSTANTS
; ============================================================================

; --- Screen configuration ---
.set SCREEN_W,640
.set SCREEN_H,480
.set CENTER_X,SCREEN_W/2               ; 320
.set CENTER_Y,SCREEN_H/2               ; 240

; --- Tunnel geometry ---
.set NUM_RINGS,12                       ; Visible rings receding into distance
.set VERTS_PER_RING,8                   ; Octagonal cross-section
.set RING_SPACING,40                    ; Z distance between adjacent rings
.set NEAR_Z,6                           ; Z of the closest ring
.set TUNNEL_RADIUS,550                  ; Base radius (large enough to fill corners)
.set TWIST_SPEED,4                      ; Rotation offset per ring (spiral)

; --- Animation speeds ---
.set ROTATION_SPEED,2                   ; Rotation increment per frame
.set ZOOM_SPEED,3                       ; Forward movement per frame

; --- Fixed-point formats used by Voodoo ---
.set FP_12_4,4                          ; 12.4 format for vertex coordinates
.set FP_12_12,12                        ; 12.12 format for colours and Z

; ============================================================================
; MEMORY MAP
; ============================================================================
; 0x0000-0x00FF   Entry point
; 0x0100-0x2FFF   Programme code
; 0x3000-0x30FF   Sine table (256 bytes, page-aligned for fast lookup)
; 0x3200-0x32FF   Current ring vertices (8 x 8 bytes = 64 bytes)
; 0x3300-0x33FF   Previous ring vertices (8 x 8 bytes = 64 bytes)
; 0x3400-0x340F   Variables
; 0x4000+         Texture data
; 0xE000          Stack top

    .org 0x0000

; ============================================================================
; ENTRY POINT
; ============================================================================

start:
    ; Initialise stack pointer
    ld sp, 0xE000

    ; Initialise the Voodoo 3D accelerator
    call init_voodoo

    ; Upload the tunnel texture to Voodoo texture memory
    call init_texture

    ; Clear animation variables
    xor a
    ld (rotation_angle), a
    ld (rotation_angle+1), a
    ld (zoom_offset), a
    ld (zoom_offset+1), a

; ============================================================================
; MAIN LOOP
; ============================================================================

main_loop:
    call clear_screen
    call draw_tunnel
    call swap_buffers
    call update_animation
    jp main_loop

; ============================================================================
; voodoo_write32 - Write a 32-bit value to a Voodoo register via I/O ports
; ============================================================================
; The Z80 accesses Voodoo registers through a 6-port protocol:
;   1. Write register offset low byte to port 0x40
;   2. Write register offset high byte to port 0x41
;   3. Write data bytes 0-2 to ports 0x44-0x46
;   4. Write data byte 3 to port 0x47 (this triggers the actual write)
;
; Input:  HL = register offset from VOODOO_BASE
;         BCDE = 32-bit value (B=byte3, C=byte2, D=byte1, E=byte0)
; Destroys: A

voodoo_write32:
    ; Set register address
    ld a, l
    out (Z80_VOODOO_PORT_ADDR_LO), a
    ld a, h
    out (Z80_VOODOO_PORT_ADDR_HI), a
    ; Write data (little-endian: E=LSB, B=MSB)
    ld a, e
    out (Z80_VOODOO_PORT_DATA0), a
    ld a, d
    out (Z80_VOODOO_PORT_DATA1), a
    ld a, c
    out (Z80_VOODOO_PORT_DATA2), a
    ld a, b
    out (Z80_VOODOO_PORT_DATA3), a      ; Byte 3 triggers the 32-bit write
    ret

; ============================================================================
; voodoo_write_coord - Write a 16-bit coordinate, sign-extended to 32 bits
; ============================================================================
; Vertex coordinates are signed 16-bit values that must be sign-extended
; to 32 bits before writing to the Voodoo. This avoids passing explicit
; high bytes for every coordinate.
;
; Input:  HL = register offset, DE = 16-bit signed value
; Destroys: A, BC

voodoo_write_coord:
    ; Set register address
    ld a, l
    out (Z80_VOODOO_PORT_ADDR_LO), a
    ld a, h
    out (Z80_VOODOO_PORT_ADDR_HI), a
    ; Write low 16 bits
    ld a, e
    out (Z80_VOODOO_PORT_DATA0), a
    ld a, d
    out (Z80_VOODOO_PORT_DATA1), a
    ; Sign-extend: if bit 7 of D is set, fill high bytes with 0xFF
    bit 7, d
    jr z, .positive
    ld a, 0xFF
    jr .write_high
.positive:
    xor a
.write_high:
    out (Z80_VOODOO_PORT_DATA2), a
    out (Z80_VOODOO_PORT_DATA3), a      ; Triggers the write
    ret

; ============================================================================
; init_voodoo - Initialise the Voodoo graphics accelerator
; ============================================================================
; Enables the Voodoo, sets the display resolution to 640x480, and
; configures the framebuffer mode for depth testing and colour writes.

init_voodoo:
    ; Enable the Voodoo (disabled by default)
    ld hl, 0x004                        ; VOODOO_ENABLE offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x01
    call voodoo_write32

    ; Set display dimensions: (640 << 16) | 480 = 0x028001E0
    ld hl, 0x214                        ; VOODOO_VIDEO_DIM offset
    ld b, 0x02                          ; 640 >> 8
    ld c, 0x80                          ; 640 & 0xFF
    ld d, 0x01                          ; 480 >> 8
    ld e, 0xE0                          ; 480 & 0xFF
    call voodoo_write32

    ; Configure fbzMode: depth test + RGB write + depth write + LESS function
    ; 0x0630 = depth_enable | LESS<<5 | rgb_write | depth_write
    ld hl, 0x110                        ; VOODOO_FBZ_MODE offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x06
    ld e, 0x30
    call voodoo_write32

    ret

; ============================================================================
; init_texture - Upload a 64x64 RGBA texture to Voodoo memory
; ============================================================================
; Configures the texture unit for ARGB8888 format and triggers an upload
; from Z80 RAM (where the .incbin data resides) to Voodoo texture memory.
; Also sets the colour combine mode to modulate texture with vertex colour,
; allowing depth-based shading to affect the textured surface.

init_texture:
    ; Set texture mode: ARGB8888 format (10 << 8) + enable
    ld hl, 0x300                        ; VOODOO_TEXTURE_MODE offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x0A                          ; Format = ARGB8888
    ld e, 0x01                          ; Enable texture
    call voodoo_write32

    ; Set texture dimensions (64x64)
    ld hl, 0x330                        ; VOODOO_TEX_WIDTH offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 64
    call voodoo_write32

    ld hl, 0x334                        ; VOODOO_TEX_HEIGHT offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 64
    call voodoo_write32

    ; Point texture source at Z80 RAM where the binary data is embedded
    ld a, texture_data & 0xFF
    out (Z80_VOODOO_PORT_TEXSRC_LO), a
    ld a, texture_data >> 8
    out (Z80_VOODOO_PORT_TEXSRC_HI), a

    ; Trigger texture upload from Z80 RAM to Voodoo texture memory
    ld hl, 0x338                        ; VOODOO_TEX_UPLOAD offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 1
    call voodoo_write32

    ; Set colour combine: modulate texture with vertex colour (0x61)
    ; This allows per-vertex brightness to shade the texture
    ld hl, 0x104                        ; VOODOO_FBZCOLOR_PATH offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x61
    call voodoo_write32

    ret

; ============================================================================
; clear_screen - Clear the framebuffer to a reddish-grey background
; ============================================================================

clear_screen:
    ; Set clear colour (matches the outer tunnel tint)
    ld hl, 0x1D8                        ; VOODOO_COLOR0 offset
    ld b, 0xFF                          ; Alpha
    ld c, 0xA0                          ; Red
    ld d, 0x80                          ; Green
    ld e, 0x80                          ; Blue
    call voodoo_write32

    ; Execute hardware fast fill
    ld hl, 0x124                        ; VOODOO_FAST_FILL_CMD offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ret

; ============================================================================
; swap_buffers - Double-buffer flip with vertical sync
; ============================================================================

swap_buffers:
    ld hl, 0x128                        ; VOODOO_SWAP_BUFFER_CMD offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x01                          ; VOODOO_SWAP_VSYNC
    call voodoo_write32
    ret

; ============================================================================
; draw_corner_fill - Fill screen corners with solid grey
; ============================================================================
; Draws 2 full-screen triangles to provide a background behind the tunnel.
; These are drawn first; the tunnel geometry is drawn on top.

draw_corner_fill:
    ; Set grey vertex colour for the fill rectangles
    ; R = G = B = 144/255 (mid-grey)
    ld hl, 0x1A0                        ; VOODOO_START_R offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x90
    call voodoo_write32

    ld hl, 0x1A4                        ; VOODOO_START_G offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x90
    call voodoo_write32

    ld hl, 0x1A8                        ; VOODOO_START_B offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x90
    call voodoo_write32

    ld hl, 0x1AC                        ; VOODOO_START_A offset
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0xFF
    call voodoo_write32

    ; --- Triangle 1: top-left half of screen ---
    ; Vertex 0: (0, 0)
    ld hl, 0x000                        ; VOODOO_VERTEX_X
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x004                        ; VOODOO_VERTEX_Y
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ; Vertex 1: (640, 0) in 12.4 = 0x2800
    ld hl, 0x000
    ld b, 0x00
    ld c, 0x00
    ld d, 0x28
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x004
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ; Vertex 2: (0, 480) in 12.4 = 0x1E00
    ld hl, 0x000
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x004
    ld b, 0x00
    ld c, 0x00
    ld d, 0x1E
    ld e, 0x00
    call voodoo_write32

    ; Submit triangle
    ld hl, 0x120                        ; VOODOO_TRIANGLE_CMD
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x01
    call voodoo_write32

    ; --- Triangle 2: bottom-right half of screen ---
    ; Vertex 0: (640, 0)
    ld hl, 0x000
    ld b, 0x00
    ld c, 0x00
    ld d, 0x28
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x004
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ; Vertex 1: (640, 480)
    ld hl, 0x000
    ld b, 0x00
    ld c, 0x00
    ld d, 0x28
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x004
    ld b, 0x00
    ld c, 0x00
    ld d, 0x1E
    ld e, 0x00
    call voodoo_write32

    ; Vertex 2: (0, 480)
    ld hl, 0x000
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x004
    ld b, 0x00
    ld c, 0x00
    ld d, 0x1E
    ld e, 0x00
    call voodoo_write32

    ; Submit triangle
    ld hl, 0x120
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x01
    call voodoo_write32

    ret

; ============================================================================
; debug_test_triangle - Draw a static white triangle for hardware testing
; ============================================================================
; Verifies that the I/O port mechanism works for Voodoo triangle rendering.
; Vertices: A(320,100), B(500,380), C(140,380) -- a centred triangle.

debug_test_triangle:
    ; Disable texture for a pure vertex-colour test
    ld hl, 0x300                        ; VOODOO_TEXTURE_MODE
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ; Vertex colour only (no texture combine)
    ld hl, 0x104                        ; VOODOO_FBZCOLOR_PATH
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ; Clear screen to bright green (confirms code is executing)
    ld hl, 0x1D8                        ; VOODOO_COLOR0
    ld b, 0xFF
    ld c, 0x00
    ld d, 0xFF
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x124                        ; VOODOO_FAST_FILL_CMD
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ; --- Vertex A: (320, 100) in 12.4 fixed-point ---
    ; X = 320 * 16 = 5120 = 0x1400
    ld hl, 0x008                        ; VOODOO_VERTEX_AX
    ld b, 0x00
    ld c, 0x00
    ld d, 0x14
    ld e, 0x00
    call voodoo_write32

    ; Y = 100 * 16 = 1600 = 0x0640
    ld hl, 0x00C                        ; VOODOO_VERTEX_AY
    ld b, 0x00
    ld c, 0x00
    ld d, 0x06
    ld e, 0x40
    call voodoo_write32

    ; --- Vertex B: (500, 380) ---
    ; X = 500 * 16 = 8000 = 0x1F40
    ld hl, 0x010                        ; VOODOO_VERTEX_BX
    ld b, 0x00
    ld c, 0x00
    ld d, 0x1F
    ld e, 0x40
    call voodoo_write32

    ; Y = 380 * 16 = 6080 = 0x17C0
    ld hl, 0x014                        ; VOODOO_VERTEX_BY
    ld b, 0x00
    ld c, 0x00
    ld d, 0x17
    ld e, 0xC0
    call voodoo_write32

    ; --- Vertex C: (140, 380) ---
    ; X = 140 * 16 = 2240 = 0x08C0
    ld hl, 0x018                        ; VOODOO_VERTEX_CX
    ld b, 0x00
    ld c, 0x00
    ld d, 0x08
    ld e, 0xC0
    call voodoo_write32

    ; Y = 380 * 16 = 6080 = 0x17C0
    ld hl, 0x01C                        ; VOODOO_VERTEX_CY
    ld b, 0x00
    ld c, 0x00
    ld d, 0x17
    ld e, 0xC0
    call voodoo_write32

    ; --- White vertex colour (1.0 in 12.12 = 0x1000) ---
    ld hl, 0x020                        ; VOODOO_START_R
    ld b, 0x00
    ld c, 0x00
    ld d, 0x10
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x024                        ; VOODOO_START_G
    ld b, 0x00
    ld c, 0x00
    ld d, 0x10
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x028                        ; VOODOO_START_B
    ld b, 0x00
    ld c, 0x00
    ld d, 0x10
    ld e, 0x00
    call voodoo_write32

    ld hl, 0x030                        ; VOODOO_START_A
    ld b, 0x00
    ld c, 0x00
    ld d, 0x10
    ld e, 0x00
    call voodoo_write32

    ; Submit triangle
    ld hl, 0x080                        ; VOODOO_TRIANGLE_CMD
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ; Swap to display the debug triangle
    ld hl, 0x128
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x01
    call voodoo_write32

    ; Halt here to keep the triangle visible
.debug_loop:
    jp .debug_loop

; ============================================================================
; update_animation - Advance rotation and zoom each frame
; ============================================================================
; Rotation increases continuously (wraps naturally via 16-bit overflow).
; Zoom advances by ZOOM_SPEED and wraps at RING_SPACING so the tunnel
; appears to move forward endlessly.

update_animation:
    ; Advance rotation angle
    ld hl, (rotation_angle)
    ld de, ROTATION_SPEED
    add hl, de
    ld (rotation_angle), hl

    ; Advance zoom offset, wrapping at RING_SPACING
    ld hl, (zoom_offset)
    ld de, ZOOM_SPEED
    add hl, de

    ld a, h
    or a
    jr nz, .wrap_zoom
    ld a, l
    cp RING_SPACING
    jr c, .no_wrap
.wrap_zoom:
    ld hl, 0
.no_wrap:
    ld (zoom_offset), hl

    ret

; ============================================================================
; draw_tunnel - Draw all tunnel rings from back to front
; ============================================================================
; Iterates from the farthest ring to the nearest. For each ring, generates
; 8 vertices (octagon), then draws quads connecting to the previous ring.
; Drawing back-to-front ensures closer geometry properly occludes distant
; geometry via the depth buffer.

draw_tunnel:
    ld b, NUM_RINGS - 1                 ; Start from farthest ring

.ring_loop:
    push bc

    ; Generate this ring's 8 vertices
    call generate_ring

    ; Skip drawing for the first (farthest) ring - no previous ring to connect to
    ld a, b
    cp NUM_RINGS - 1
    jr z, .skip_draw

    call draw_ring_quads

.skip_draw:
    ; Copy current ring to previous ring buffer for next iteration
    call copy_ring_buffer

    pop bc
    dec b
    jp p, .ring_loop                    ; Continue while B >= 0

    ret

; ============================================================================
; generate_ring - Compute 8 vertex positions for one tunnel ring
; ============================================================================
; Input: B = ring index (0 = nearest, NUM_RINGS-1 = farthest)
;
; For each vertex, computes:
;   world_z = NEAR_Z + ring_index * RING_SPACING - zoom_offset
;   radius = TUNNEL_RADIUS * 256 / world_z  (perspective projection)
;   angle = rotation + twist_offset + vertex_index * 32
;   x = CENTER_X + cos(angle) * radius / 64
;   y = CENTER_Y + sin(angle) * radius / 64
;   brightness = 255 - (world_z / 4)  (depth-based fog)

generate_ring:
    push bc

    ; Calculate world Z for this ring
    ld a, b
    ld l, a
    ld h, 0

    ; Multiply by RING_SPACING (40 = 32 + 8)
    push hl
    add hl, hl                          ; * 2
    add hl, hl                          ; * 4
    add hl, hl                          ; * 8
    push hl                             ; Save * 8
    add hl, hl                          ; * 16
    add hl, hl                          ; * 32
    pop de                              ; DE = * 8
    add hl, de                          ; HL = * 40
    pop de

    ; Add NEAR_Z
    ld de, NEAR_Z
    add hl, de

    ; Subtract zoom_offset
    ex de, hl
    ld hl, (zoom_offset)
    ex de, hl
    or a
    sbc hl, de
    ld (ring_z), hl

    ; Calculate twist offset = ring_index * TWIST_SPEED (4)
    pop bc
    push bc
    ld a, b
    ld e, a
    ld d, 0
    sla e
    rl d
    sla e
    rl d                                ; DE = ring_index * 4
    ld (twist_offset), de

    ; Calculate perspective-scaled radius
    ld hl, (ring_z)
    call get_perspective_radius
    ld (ring_radius), hl

    ; --- Generate 8 vertices ---
    ld ix, current_ring
    ld b, VERTS_PER_RING
    ld c, 0                             ; Vertex index

.vertex_loop:
    push bc

    ; Vertex angle = rotation + twist + (vertex_index * 32)
    ; 256/8 = 32, so each vertex is 32 angle units apart
    ld a, c
    sla a
    sla a
    sla a
    sla a
    sla a                               ; * 32
    ld e, a
    ld d, 0

    ld hl, (rotation_angle)
    add hl, de
    ld de, (twist_offset)
    add hl, de

    ; Look up sin and cos from the table
    ld a, l                             ; Low byte = angle (0-255)
    call get_sin_cos                    ; D = sin, E = cos

    ; --- Compute X = CENTER_X + (cos * radius) / 64 ---
    push de                             ; Save sin/cos
    ld a, e                             ; cos
    ld hl, (ring_radius)
    call signed_multiply_8x16           ; HL = cos * radius
    ; Divide by 64 (arithmetic shift right 6 times)
    sra h
    rr l
    sra h
    rr l
    sra h
    rr l
    sra h
    rr l
    sra h
    rr l
    sra h
    rr l
    ld de, CENTER_X
    add hl, de
    ld (ix+0), l                        ; Store X low byte
    ld (ix+1), h                        ; Store X high byte

    ; --- Compute Y = CENTER_Y + (sin * radius) / 64 ---
    pop de                              ; Restore D=sin, E=cos
    ld a, d                             ; sin
    ld hl, (ring_radius)
    call signed_multiply_8x16           ; HL = sin * radius
    sra h
    rr l
    sra h
    rr l
    sra h
    rr l
    sra h
    rr l
    sra h
    rr l
    sra h
    rr l
    ld de, CENTER_Y
    add hl, de
    ld (ix+2), l                        ; Store Y low byte
    ld (ix+3), h                        ; Store Y high byte

    ; Store Z (for depth buffer)
    ld hl, (ring_z)
    ld (ix+4), l
    ld (ix+5), h

    ; --- Compute brightness = 255 - (z / 4) ---
    ; Closer rings are brighter; distant rings fade to black
    ld hl, (ring_z)
    sra h
    rr l
    sra h
    rr l                                ; z / 4
    ld a, 0xFF
    sub l
    jr nc, .brightness_ok
    xor a                               ; Clamp to 0
.brightness_ok:
    ld (ix+6), a                        ; Store brightness
    ld (ix+7), c                        ; Store vertex index (for texture S coord)

    ; Advance to next vertex (8 bytes per vertex)
    ld de, 8
    add ix, de

    pop bc
    inc c
    dec b
    jp nz, .vertex_loop

    pop bc
    ret

; ============================================================================
; get_perspective_radius - Compute radius based on distance from viewer
; ============================================================================
; Implements: radius = 3096 / z  (giving 516 at z=6, clamped for safety)
; Uses repeated subtraction for division (simple but functional on Z80).
;
; Input:  HL = world Z
; Output: HL = perspective-scaled radius

get_perspective_radius:
    ; Clamp minimum Z to NEAR_Z to avoid division overflow
    ld a, h
    or a
    jr nz, .calc_radius
    ld a, l
    cp 6
    jr nc, .calc_radius
    ld l, 6

.calc_radius:
    push de
    push bc

    ; Divide 3096 by z using repeated subtraction
    ex de, hl                           ; DE = z
    ld bc, 0                            ; BC = quotient
    ld hl, 3096                         ; HL = dividend

.div_loop:
    or a
    sbc hl, de
    jr c, .div_done
    inc bc
    jr .div_loop

.div_done:
    ld h, b
    ld l, c                             ; HL = result

    pop bc
    pop de
    ret

; ============================================================================
; get_sin_cos - Look up sine and cosine from the 256-entry table
; ============================================================================
; The sine table is page-aligned at 0x3000, so the high byte of the
; address is constant. Cosine is sin(angle + 64) since cos = sin + 90deg.
;
; Input:  A = angle (0-255, representing 0-360 degrees)
; Output: D = sin(angle), E = cos(angle) (signed bytes, -127 to +127)

get_sin_cos:
    push hl

    ; Look up sin
    ld l, a
    ld h, >sin_table                    ; High byte of sin_table address
    ld d, (hl)                          ; D = sin(angle)

    ; Look up cos (sin + 64 = sin + 90 degrees)
    add a, 64
    ld l, a
    ld e, (hl)                          ; E = cos(angle)

    pop hl
    ret

; ============================================================================
; signed_multiply_8x16 - Multiply signed 8-bit by unsigned 16-bit
; ============================================================================
; Input:  A = signed 8-bit multiplier, HL = unsigned 16-bit multiplicand
; Output: HL = result (signed 16-bit, truncated from full product)

signed_multiply_8x16:
    bit 7, a
    jr nz, .negative

    ; Positive: multiply directly
    call multiply_8x16
    ret

.negative:
    ; Negate, multiply, negate result
    neg
    call multiply_8x16
    ; Negate HL
    xor a
    sub l
    ld l, a
    sbc a, a
    sub h
    ld h, a
    ret

; ============================================================================
; multiply_8x16 - Unsigned 8-bit x 16-bit multiply (shift-and-add)
; ============================================================================
; Input:  A = 8-bit multiplier, HL = 16-bit multiplicand
; Output: HL = low 16 bits of product

multiply_8x16:
    push bc
    push de

    ld d, 0
    ld e, a                             ; DE = multiplier
    ld b, h
    ld c, l                             ; BC = multiplicand

    ld hl, 0                            ; Accumulator

    ld a, 8                             ; 8 bit positions to check
.mul_loop:
    srl e                               ; Shift multiplier right, bit 0 -> carry
    jr nc, .no_add
    add hl, bc                          ; Add multiplicand if bit was set
.no_add:
    sla c
    rl b                                ; Shift multiplicand left
    dec a
    jr nz, .mul_loop

    pop de
    pop bc
    ret

; ============================================================================
; draw_ring_quads - Draw textured quads between current and previous rings
; ============================================================================
; Each pair of adjacent vertices on the current and previous rings forms
; a quadrilateral. Each quad is split into 2 triangles for Voodoo.

draw_ring_quads:
    push bc

    ld b, VERTS_PER_RING                ; 8 segments around the ring
    ld c, 0

.quad_loop:
    push bc

    ; Compute vertex indices (wrapping at 8 for octagonal ring)
    ld a, c
    and 7                               ; E = current vertex index
    ld e, a

    inc a
    and 7                               ; D = next vertex index
    ld d, a

    ; Triangle 1: current[i], current[i+1], prev[i]
    call submit_triangle_1

    ; Triangle 2: current[i+1], prev[i+1], prev[i]
    call submit_triangle_2

    pop bc
    inc c
    djnz .quad_loop

    pop bc
    ret

; ============================================================================
; submit_triangle_1 / submit_triangle_2 - Submit quad triangles to Voodoo
; ============================================================================
; Input: E = vertex index i, D = vertex index i+1

submit_triangle_1:
    push de

    ; Vertex A: current[i]
    ld a, e
    call get_current_vertex_addr
    call submit_vertex_a

    pop de
    push de

    ; Vertex B: current[i+1]
    ld a, d
    call get_current_vertex_addr
    call submit_vertex_b

    pop de
    push de

    ; Vertex C: prev[i]
    ld a, e
    call get_prev_vertex_addr
    call submit_vertex_c

    call submit_triangle_cmd

    pop de
    ret

submit_triangle_2:
    push de

    ; Vertex A: current[i+1]
    ld a, d
    call get_current_vertex_addr
    call submit_vertex_a

    pop de
    push de

    ; Vertex B: prev[i+1]
    ld a, d
    call get_prev_vertex_addr
    call submit_vertex_b

    pop de
    push de

    ; Vertex C: prev[i]
    ld a, e
    call get_prev_vertex_addr
    call submit_vertex_c

    call submit_triangle_cmd

    pop de
    ret

; ============================================================================
; get_current_vertex_addr / get_prev_vertex_addr
; ============================================================================
; Input:  A = vertex index (0-7)
; Output: IX = pointer to the 8-byte vertex record

get_current_vertex_addr:
    push de
    ld ix, current_ring
    sla a
    sla a
    sla a                               ; A * 8 (bytes per vertex)
    ld e, a
    ld d, 0
    add ix, de
    pop de
    ret

get_prev_vertex_addr:
    push de
    ld ix, prev_ring
    sla a
    sla a
    sla a                               ; A * 8
    ld e, a
    ld d, 0
    add ix, de
    pop de
    ret

; ============================================================================
; submit_vertex_a / b / c - Submit vertex coordinates to Voodoo
; ============================================================================
; Each routine selects the appropriate Voodoo vertex slot (A/B/C), converts
; the 16-bit pixel coordinates to 12.4 fixed-point, and sets colour, depth,
; and texture attributes.
;
; Input: IX = pointer to vertex data

submit_vertex_a:
    ; Select vertex 0 for Gouraud attributes
    ld hl, 0x088                        ; VOODOO_COLOR_SELECT
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ; X coordinate -> 12.4 fixed-point (shift left 4)
    ld e, (ix+0)
    ld d, (ix+1)
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    ld hl, 0x008                        ; VOODOO_VERTEX_AX
    call voodoo_write_coord

    ; Y coordinate -> 12.4 fixed-point
    ld e, (ix+2)
    ld d, (ix+3)
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    ld hl, 0x00C                        ; VOODOO_VERTEX_AY
    call voodoo_write_coord

    call set_vertex_attributes
    ret

submit_vertex_b:
    ; Select vertex 1
    ld hl, 0x088
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x01
    call voodoo_write32

    ; X -> 12.4
    ld e, (ix+0)
    ld d, (ix+1)
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    ld hl, 0x010                        ; VOODOO_VERTEX_BX
    call voodoo_write_coord

    ; Y -> 12.4
    ld e, (ix+2)
    ld d, (ix+3)
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    ld hl, 0x014                        ; VOODOO_VERTEX_BY
    call voodoo_write_coord

    call set_vertex_attributes
    ret

submit_vertex_c:
    ; Select vertex 2
    ld hl, 0x088
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x02
    call voodoo_write32

    ; X -> 12.4
    ld e, (ix+0)
    ld d, (ix+1)
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    ld hl, 0x018                        ; VOODOO_VERTEX_CX
    call voodoo_write_coord

    ; Y -> 12.4
    ld e, (ix+2)
    ld d, (ix+3)
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    ld hl, 0x01C                        ; VOODOO_VERTEX_CY
    call voodoo_write_coord

    call set_vertex_attributes
    ret

; ============================================================================
; set_vertex_attributes - Set colour, depth, W, and texture coordinates
; ============================================================================
; Uses the vertex brightness (ix+6) for greyscale Gouraud shading,
; the vertex Z (ix+4/5) for depth buffer testing, and the vertex index
; (ix+7) for texture S coordinate mapping around the tunnel circumference.
;
; Input: IX = vertex address

set_vertex_attributes:
    push af
    push de

    ; --- Vertex colour: greyscale from brightness ---
    ; brightness (0-255) scaled to 12.12 format: brightness * 16
    ld a, (ix+6)
    ld e, a
    ld d, 0
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d
    sla e
    rl d                                ; DE = brightness * 16

    ; R channel
    push de
    ld hl, 0x020                        ; VOODOO_START_R
    ld b, 0x00
    ld c, 0x00
    call voodoo_write32
    pop de

    ; G channel
    push de
    ld hl, 0x024                        ; VOODOO_START_G
    ld b, 0x00
    ld c, 0x00
    call voodoo_write32
    pop de

    ; B channel
    ld hl, 0x028                        ; VOODOO_START_B
    ld b, 0x00
    ld c, 0x00
    call voodoo_write32

    ; Alpha = 1.0 (0x1000 in 12.12)
    ld hl, 0x030                        ; VOODOO_START_A
    ld b, 0x00
    ld c, 0x00
    ld d, 0x10
    ld e, 0x00
    call voodoo_write32

    ; --- Z coordinate in 20.12 fixed-point ---
    ; ring_z << 12 = ring_z * 4096
    ; Constructed as: ring_z << 16 then >> 4
    ld e, (ix+4)
    ld d, (ix+5)                        ; DE = ring_z
    ld b, d
    ld c, e                             ; BC = ring_z
    ld d, 0
    ld e, 0                             ; BC:DE = ring_z << 16
    ; Shift right 4 to get ring_z << 12
    srl b
    rr c
    rr d
    rr e
    srl b
    rr c
    rr d
    rr e
    srl b
    rr c
    rr d
    rr e
    srl b
    rr c
    rr d
    rr e                                ; BC:DE = ring_z << 12 (20.12 format)
    ld hl, 0x02C                        ; VOODOO_START_Z
    call voodoo_write32

    ; --- W = 1.0 in 2.30 fixed-point (0x40000000) ---
    ld hl, 0x03C                        ; VOODOO_START_W
    ld b, 0x40
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32

    ; --- Texture S coordinate (wraps around tunnel circumference) ---
    ; S = vertex_index * 0x8000 (index << 15) in 14.18 fixed-point
    ld a, (ix+7)
    ld c, a
    srl c                               ; C = index >> 1 (byte2)
    ld d, a
    sla d
    sla d
    sla d
    sla d
    sla d
    sla d
    sla d                               ; D = (index << 7) & 0xFF (byte1)
    ld e, 0                             ; E = 0 (byte0)
    ld b, 0                             ; B = 0 (byte3)
    ld hl, 0x034                        ; VOODOO_START_S
    call voodoo_write32

    ; --- Texture T coordinate (tiles along tunnel depth) ---
    ; T = ring_z << 10 (14.18 fixed-point, for visible texture tiling)
    ld e, (ix+4)
    ld d, (ix+5)                        ; DE = ring_z
    ld b, 0
    ld c, 0
    ; Shift left 10 bits
    sla e
    rl d
    rl c
    sla e
    rl d
    rl c
    sla e
    rl d
    rl c
    sla e
    rl d
    rl c
    sla e
    rl d
    rl c
    sla e
    rl d
    rl c
    sla e
    rl d
    rl c
    sla e
    rl d
    rl c
    sla e
    rl d
    rl c
    sla e
    rl d
    rl c                                ; C:D:E = ring_z << 10
    ld b, 0
    ld hl, 0x038                        ; VOODOO_START_T
    call voodoo_write32

    pop de
    pop af
    ret

; ============================================================================
; submit_triangle_cmd - Issue the triangle draw command to Voodoo
; ============================================================================

submit_triangle_cmd:
    ld hl, 0x080                        ; VOODOO_TRIANGLE_CMD
    ld b, 0x00
    ld c, 0x00
    ld d, 0x00
    ld e, 0x00
    call voodoo_write32
    ret

; ============================================================================
; copy_ring_buffer - Copy current ring vertices to previous ring buffer
; ============================================================================
; After drawing quads between current and previous, the current ring
; becomes the "previous" for the next iteration.

copy_ring_buffer:
    ld hl, current_ring
    ld de, prev_ring
    ld bc, VERTS_PER_RING * 8           ; 64 bytes
    ldir
    ret

; ============================================================================
; DATA SECTION
; ============================================================================

    .org 0x3000

; --- Sine table (256 entries, signed bytes -127 to +127) ---
; Page-aligned at 0x3000 so the high byte is constant (0x30), allowing
; fast lookup with just: ld h, >sin_table / ld l, angle / ld d, (hl)

sin_table:
    .byte 0, 3, 6, 9, 12, 16, 19, 22, 25, 28, 31, 34, 37, 40, 43, 46
    .byte 49, 51, 54, 57, 60, 63, 65, 68, 71, 73, 76, 78, 81, 83, 85, 88
    .byte 90, 92, 94, 96, 98, 100, 102, 104, 106, 107, 109, 111, 112, 113, 115, 116
    .byte 117, 118, 120, 121, 122, 122, 123, 124, 125, 125, 126, 126, 126, 127, 127, 127
    .byte 127, 127, 127, 127, 126, 126, 126, 125, 125, 124, 123, 122, 122, 121, 120, 118
    .byte 117, 116, 115, 113, 112, 111, 109, 107, 106, 104, 102, 100, 98, 96, 94, 92
    .byte 90, 88, 85, 83, 81, 78, 76, 73, 71, 68, 65, 63, 60, 57, 54, 51
    .byte 49, 46, 43, 40, 37, 34, 31, 28, 25, 22, 19, 16, 12, 9, 6, 3
    .byte 0, -3, -6, -9, -12, -16, -19, -22, -25, -28, -31, -34, -37, -40, -43, -46
    .byte -49, -51, -54, -57, -60, -63, -65, -68, -71, -73, -76, -78, -81, -83, -85, -88
    .byte -90, -92, -94, -96, -98, -100, -102, -104, -106, -107, -109, -111, -112, -113, -115, -116
    .byte -117, -118, -120, -121, -122, -122, -123, -124, -125, -125, -126, -126, -126, -127, -127, -127
    .byte -127, -127, -127, -127, -126, -126, -126, -125, -125, -124, -123, -122, -122, -121, -120, -118
    .byte -117, -116, -115, -113, -112, -111, -109, -107, -106, -104, -102, -100, -98, -96, -94, -92
    .byte -90, -88, -85, -83, -81, -78, -76, -73, -71, -68, -65, -63, -60, -57, -54, -51
    .byte -49, -46, -43, -40, -37, -34, -31, -28, -25, -22, -19, -16, -12, -9, -6, -3

; ============================================================================
; VARIABLES
; ============================================================================

    .org 0x3400

rotation_angle:     .word 0             ; Current rotation angle (16-bit)
zoom_offset:        .word 0             ; Current zoom/forward position
ring_z:             .word 0             ; Z coordinate of ring being generated
ring_radius:        .word 0             ; Perspective-scaled radius
twist_offset:       .word 0             ; Rotation offset for spiral twist

; ============================================================================
; VERTEX BUFFERS
; ============================================================================

    .org 0x3200
current_ring:       .space 64           ; Current ring (8 vertices x 8 bytes)

    .org 0x3300
prev_ring:          .space 64           ; Previous ring (8 vertices x 8 bytes)

; ============================================================================
; TEXTURE DATA (64x64 RGBA = 16KB)
; ============================================================================

    .org 0x4000

texture_data:
    .incbin "../assets/boing_checker_64.bin"
