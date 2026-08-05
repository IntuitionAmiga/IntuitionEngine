; 6502 Voodoo register-aperture smoke test.
;
; Target: 6502 guest with the Voodoo device enabled. The 6502 cannot address
; the full Voodoo MMIO range directly, so it first selects the $F8 register
; page through $F7F2, then uses the $E000 aperture. This is a useful starting
; point for 6502 code that needs to submit Voodoo commands.
;
; Assemble for the selected CPU launch the generated guest

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
        ; Select the Voodoo register page before touching aperture offsets.
        lda #VSMOKE_REG_PAGE
        sta VSMOKE_BANK_HI

        ; Enable the device, choose a clear colour, clear the back buffer and
        ; request a swap. Each 32-bit register is written little-endian.
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
        ; Keep the guest alive so the submitted state remains observable.
        jmp forever
