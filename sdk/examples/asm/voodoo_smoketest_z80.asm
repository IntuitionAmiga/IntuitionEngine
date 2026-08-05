; Z80 Voodoo port-adapter smoke test.
;
; Target: Z80 guest with the Voodoo device enabled. The adapter exposes a
; register offset through ports $B0-$B1 and a 32-bit little-endian value
; through $B2-$B5; writing data byte 3 commits the register write.
;
; Assemble for the selected CPU launch the resulting guest

    .include "ie80.inc"

.set VOODOO_ENABLE_OFF,0x0004
.set VOODOO_COLOR0_OFF,0x01D8
.set VOODOO_FAST_FILL_CMD_OFF,0x0124
.set VOODOO_SWAP_BUFFER_CMD_OFF,0x0128

    .org 0x0000

start:
    ; Enable Voodoo, clear the current back buffer, then present it.
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
    ; HALT yields until the next interrupt while preserving device state.
    halt
    jr halt_loop

; Submit B:A:DE as a little-endian dword at the Voodoo offset in HL. B5 is
; written last because that port commits the assembled value.
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
