; ============================================================================
; ULA Simple Test - IE65 (6502)
;
; Simple test to verify ULA is working:
; 1. Sets blue border
; 2. Enables ULA
; 3. Fills bitmap with a pattern
; 4. Sets white on black attributes
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie65.inc"

; ============================================================================
; CODE SEGMENT
; ============================================================================
.segment "CODE"

.proc start
    ; Set blue border
    lda #1                      ; Blue
    sta ULA_BORDER

    ; Enable ULA
    lda #ULA_CTRL_ENABLE
    sta ULA_CTRL

    ; Fill attribute area with white on black ($07)
    lda #<(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0
    lda #>(ULA_VRAM + ULA_ATTR_OFFSET)
    sta zp_ptr0+1

    lda #$07                    ; White ink, black paper
    ldy #0
    ldx #3                      ; 3 pages = 768 bytes

attr_loop:
    sta (zp_ptr0),y
    iny
    bne attr_loop
    inc zp_ptr0+1
    dex
    bne attr_loop

    ; Fill bitmap with diagonal stripe pattern
    lda #<ULA_VRAM
    sta zp_ptr0
    lda #>ULA_VRAM
    sta zp_ptr0+1

    ldx #24                     ; 24 pages = 6144 bytes
    lda #$55                    ; Alternating bits pattern

bitmap_loop:
    ldy #0
fill_page:
    sta (zp_ptr0),y
    eor #$FF                    ; Alternate pattern
    iny
    bne fill_page
    inc zp_ptr0+1
    dex
    bne bitmap_loop

    ; Done - just loop forever
forever:
    jmp forever
.endproc
