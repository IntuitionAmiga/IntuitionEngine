; Minimal 6502 Voodoo access smoke test.
; Exercises the documented $E000 banked Voodoo aperture.

VOODOO_WINDOW       = $E000
VOODOO_BANK_HI      = $F7F2
VOODOO_REG_PAGE     = $F8

VOODOO_ENABLE       = VOODOO_WINDOW + $004
VOODOO_COLOR0       = VOODOO_WINDOW + $1D8
VOODOO_FAST_FILL    = VOODOO_WINDOW + $124
VOODOO_SWAP         = VOODOO_WINDOW + $128

        .org $0800

start:
        lda #VOODOO_REG_PAGE
        sta VOODOO_BANK_HI

        lda #$01
        sta VOODOO_ENABLE+0
        lda #$00
        sta VOODOO_ENABLE+1
        sta VOODOO_ENABLE+2
        sta VOODOO_ENABLE+3

        lda #$33
        sta VOODOO_COLOR0+0
        lda #$66
        sta VOODOO_COLOR0+1
        lda #$99
        sta VOODOO_COLOR0+2
        lda #$FF
        sta VOODOO_COLOR0+3

        lda #$00
        sta VOODOO_FAST_FILL+0
        sta VOODOO_FAST_FILL+1
        sta VOODOO_FAST_FILL+2
        sta VOODOO_FAST_FILL+3

        sta VOODOO_SWAP+0
        sta VOODOO_SWAP+1
        sta VOODOO_SWAP+2
        sta VOODOO_SWAP+3

forever:
        jmp forever
