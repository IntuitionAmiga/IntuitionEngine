; Minimal Z80 Voodoo access smoke test.
; Exercises the documented 0xB0..0xB7 Voodoo port adapter.

Z80_VOODOO_PORT_ADDR_LO   equ 0B0h
Z80_VOODOO_PORT_ADDR_HI   equ 0B1h
Z80_VOODOO_PORT_DATA0     equ 0B2h
Z80_VOODOO_PORT_DATA1     equ 0B3h
Z80_VOODOO_PORT_DATA2     equ 0B4h
Z80_VOODOO_PORT_DATA3     equ 0B5h

VOODOO_ENABLE_OFF         equ 0004h
VOODOO_COLOR0_OFF         equ 01D8h
VOODOO_FAST_FILL_CMD_OFF  equ 0124h
VOODOO_SWAP_BUFFER_CMD_OFF equ 0128h

org 0000h

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
