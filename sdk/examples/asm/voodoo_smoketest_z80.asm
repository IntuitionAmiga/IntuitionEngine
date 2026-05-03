; Minimal Z80 Voodoo access smoke test.
; Exercises the documented 0xB0..0xB7 Voodoo port adapter.

    .include "ie80.inc"

.set VOODOO_ENABLE_OFF,0x0004
.set VOODOO_COLOR0_OFF,0x01D8
.set VOODOO_FAST_FILL_CMD_OFF,0x0124
.set VOODOO_SWAP_BUFFER_CMD_OFF,0x0128

    .org 0x0000

start:
    ld hl, VOODOO_ENABLE_OFF
    ld de, 0001h
    xor a
    ld b, a
    call voodoo_write32

    ld hl, VOODOO_COLOR0_OFF
    ld de, 3366h
    ld b, 0FFh
    ld a, 99h
    call voodoo_write32_a

    ld hl, VOODOO_FAST_FILL_CMD_OFF
    ld de, 0000h
    xor a
    ld b, a
    call voodoo_write32

    ld hl, VOODOO_SWAP_BUFFER_CMD_OFF
    ld de, 0000h
    xor a
    ld b, a
    call voodoo_write32

halt_loop:
    halt
    jr halt_loop

; HL = Voodoo register offset, B:A:DE = little-endian dword.
voodoo_write32:
    xor a
voodoo_write32_a:
    push af
    ld a, l
    out (Z80_VOODOO_PORT_ADDR_LO), a
    ld a, h
    out (Z80_VOODOO_PORT_ADDR_HI), a
    ld a, e
    out (Z80_VOODOO_PORT_DATA0), a
    ld a, d
    out (Z80_VOODOO_PORT_DATA1), a
    pop af
    out (Z80_VOODOO_PORT_DATA2), a
    ld a, b
    out (Z80_VOODOO_PORT_DATA3), a
    ret
