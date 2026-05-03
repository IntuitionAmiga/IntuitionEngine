; Minimal 6502 Voodoo access smoke test.
; Exercises the documented $E000 banked Voodoo aperture.

.include "ie65.inc"

VSMOKE_WINDOW       = VOODOO_6502_WINDOW_BASE
VSMOKE_BANK_HI      = VOODOO_6502_BANK_HI
VSMOKE_REG_PAGE     = VOODOO_BASE_HI

VSMOKE_ENABLE       = VSMOKE_WINDOW + $004
VSMOKE_COLOR0       = VSMOKE_WINDOW + $1D8
VSMOKE_FAST_FILL    = VSMOKE_WINDOW + $124
VSMOKE_SWAP         = VSMOKE_WINDOW + $128

.segment "CODE"

start:
        lda #VSMOKE_REG_PAGE
        sta VSMOKE_BANK_HI

        lda #$01
        sta VSMOKE_ENABLE+0
        lda #$00
        sta VSMOKE_ENABLE+1
        sta VSMOKE_ENABLE+2
        sta VSMOKE_ENABLE+3

        lda #$33
        sta VSMOKE_COLOR0+0
        lda #$66
        sta VSMOKE_COLOR0+1
        lda #$99
        sta VSMOKE_COLOR0+2
        lda #$FF
        sta VSMOKE_COLOR0+3

        lda #$00
        sta VSMOKE_FAST_FILL+0
        sta VSMOKE_FAST_FILL+1
        sta VSMOKE_FAST_FILL+2
        sta VSMOKE_FAST_FILL+3

        sta VSMOKE_SWAP+0
        sta VSMOKE_SWAP+1
        sta VSMOKE_SWAP+2
        sta VSMOKE_SWAP+3

forever:
        jmp forever
