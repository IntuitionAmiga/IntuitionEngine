; ============================================================================
; ULA aperture demo for IE64
; ============================================================================
;
; Build:
;   sdk/bin/ie64asm -I sdk/include sdk/examples/asm/ula_demo_ie64.asm
;
; Run:
;   ./bin/IntuitionEngine -ie64 sdk/examples/asm/ula_demo_ie64.ie64
;
; This drives ULA through the IE-native register block and VRAM aperture.

include "ie64.inc"

org 0x1000

start:
    la      r31, STACK_TOP

    ; Enable ULA and set blue border.
    la      r1, ULA_BORDER
    move.q  r2, #1
    store.l r2, (r1)

    la      r1, ULA_CTRL
    move.q  r2, #ULA_CTRL_ENABLE
    store.l r2, (r1)

    ; Set all attributes to bright white ink on black paper.
    la      r1, ULA_VRAM
    move.q  r2, #ULA_ATTR_OFFSET
    add.q   r1, r1, r2
    move.q  r2, #0x47
    move.q  r3, #ULA_ATTR_SIZE
attr_loop:
    store.b r2, (r1)
    add.q   r1, r1, #1
    sub.q   r3, r3, #1
    bnez    r3, attr_loop

    ; Draw a diagonal across the first ZX-style 8-line character band.
    la      r1, ULA_VRAM
    move.q  r2, #0x80
    la      r4, ula_y_band_offsets
    move.q  r3, #8
line_loop:
    load.l  r5, (r4)
    add.q   r6, r1, r5
    store.b r2, (r6)
    add.q   r4, r4, #4
    lsr.l   r2, r2, #1
    sub.q   r3, r3, #1
    bnez    r3, line_loop

idle:
    bra     idle

ula_y_band_offsets:
    dc.l    0x0000,0x0100,0x0200,0x0300,0x0400,0x0500,0x0600,0x0700
