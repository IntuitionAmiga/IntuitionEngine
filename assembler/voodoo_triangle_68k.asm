; voodoo_triangle_68k.asm - Simple Triangle Demo for Voodoo Graphics
;
; Minimal example demonstrating the 3DFX Voodoo SST-1 emulation:
; - Draws a single colored triangle
; - Useful for testing basic Voodoo functionality
;
; Build: vasmm68k_mot -Fbin -m68020 -o voodoo_triangle_68k.ie68 voodoo_triangle_68k.asm
; Run:   ./IntuitionEngine -68k voodoo_triangle_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

    include "ie68.inc"

    org     $1000

start:
    ; Initialize stack
    lea     $FF0000,sp

    ; Set video dimensions (640x480)
    move.l  #(640<<16)|480,VOODOO_VIDEO_DIM

    ; Enable depth test with LESS function, RGB write, depth write
    ; fbzMode = depth_enable(0x10) | rgb_write(0x200) | depth_write(0x400) | depth_less(1<<5)
    move.l  #$0630,VOODOO_FBZ_MODE

main_loop:
    ; Clear framebuffer to dark blue (ARGB format)
    move.l  #$FF000080,VOODOO_COLOR0
    move.l  #0,VOODOO_FAST_FILL_CMD

    ; ========================================
    ; Draw a red triangle
    ; ========================================

    ; Vertex A: top center (320, 100)
    ; 12.4 fixed-point: multiply by 16 (shift left 4)
    move.l  #(320<<4),VOODOO_VERTEX_AX
    move.l  #(100<<4),VOODOO_VERTEX_AY

    ; Vertex B: bottom right (500, 400)
    move.l  #(500<<4),VOODOO_VERTEX_BX
    move.l  #(400<<4),VOODOO_VERTEX_BY

    ; Vertex C: bottom left (140, 400)
    move.l  #(140<<4),VOODOO_VERTEX_CX
    move.l  #(400<<4),VOODOO_VERTEX_CY

    ; Set color: bright red
    ; 12.12 fixed-point: 1.0 = 0x1000
    move.l  #$1000,VOODOO_START_R       ; R = 1.0
    move.l  #$0000,VOODOO_START_G       ; G = 0.0
    move.l  #$0000,VOODOO_START_B       ; B = 0.0
    move.l  #$1000,VOODOO_START_A       ; A = 1.0 (opaque)

    ; Set Z depth (20.12 fixed-point, 0.5 = 0x800)
    move.l  #$800,VOODOO_START_Z

    ; Submit triangle for rendering
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; ========================================
    ; Draw a green triangle (overlapping)
    ; ========================================

    ; Vertex A: (400, 150)
    move.l  #(400<<4),VOODOO_VERTEX_AX
    move.l  #(150<<4),VOODOO_VERTEX_AY

    ; Vertex B: (550, 350)
    move.l  #(550<<4),VOODOO_VERTEX_BX
    move.l  #(350<<4),VOODOO_VERTEX_BY

    ; Vertex C: (250, 350)
    move.l  #(250<<4),VOODOO_VERTEX_CX
    move.l  #(350<<4),VOODOO_VERTEX_CY

    ; Set color: bright green
    move.l  #$0000,VOODOO_START_R
    move.l  #$1000,VOODOO_START_G
    move.l  #$0000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A

    ; Set Z depth (closer than red triangle, Z = 0.3)
    move.l  #$4CC,VOODOO_START_Z        ; 0.3 * 0x1000 = 0x4CC

    ; Submit triangle
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; ========================================
    ; Draw a blue triangle (behind both)
    ; ========================================

    ; Vertex A: (320, 50)
    move.l  #(320<<4),VOODOO_VERTEX_AX
    move.l  #(50<<4),VOODOO_VERTEX_AY

    ; Vertex B: (580, 300)
    move.l  #(580<<4),VOODOO_VERTEX_BX
    move.l  #(300<<4),VOODOO_VERTEX_BY

    ; Vertex C: (60, 300)
    move.l  #(60<<4),VOODOO_VERTEX_CX
    move.l  #(300<<4),VOODOO_VERTEX_CY

    ; Set color: bright blue
    move.l  #$0000,VOODOO_START_R
    move.l  #$0000,VOODOO_START_G
    move.l  #$1000,VOODOO_START_B
    move.l  #$1000,VOODOO_START_A

    ; Set Z depth (far, Z = 0.9)
    move.l  #$E66,VOODOO_START_Z        ; 0.9 * 0x1000 = 0xE66

    ; Submit triangle
    move.l  #0,VOODOO_TRIANGLE_CMD

    ; ========================================
    ; Present frame
    ; ========================================

    ; Swap buffers with vsync wait
    move.l  #1,VOODOO_SWAP_BUFFER_CMD

    ; Loop forever
    bra     main_loop

    end
