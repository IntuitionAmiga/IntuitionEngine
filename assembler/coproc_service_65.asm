; coproc_service_65.asm - 6502 Coprocessor Service Template
;
; Service binary contract:
;   1. Poll ring buffer for requests
;   2. Dispatch on op field
;   3. Op=1 (add): read two uint32 from reqPtr, add, write to respPtr
;   4. Write response descriptor (status=2/ok, respLen=4)
;   5. Advance tail, loop
;
; Memory map (6502 address space via CoprocBus32):
;   Code loaded at CPU addr $0000
;   Mailbox at CPU addr $2000 (mapped to bus MAILBOX_BASE)
;   Ring 1 (6502): mailbox offset = 1*$300 = $0300
;     ring base:  $2300
;     head:       $2300
;     tail:       $2301
;     entries:    $2308 + tail*32
;     responses:  $2508 + tail*16
;
; Reset vector at $FFFC-$FFFD points to $0000
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

.org $0000

; Constants
RING_HEAD   = $2300
RING_TAIL   = $2301
ENTRIES_LO  = $08          ; low byte of entries offset from ring base ($2308)
ENTRIES_HI  = $23          ; high byte
RESP_LO     = $08          ; low byte of responses offset ($2508)
RESP_HI     = $25          ; high byte

; Zero page scratch
ZP_TAIL     = $00
ZP_ENTL     = $02          ; entry addr low
ZP_ENTH     = $03          ; entry addr high
ZP_RESPL    = $04          ; response addr low
ZP_RESPH    = $05          ; response addr high
ZP_REQPL    = $06          ; reqPtr low
ZP_REQPH    = $07          ; reqPtr high
ZP_REQPL2   = $08          ; reqPtr+1
ZP_REQPH2   = $09
ZP_RSPL     = $0A          ; respPtr low
ZP_RSPH     = $0B          ; respPtr high
ZP_RSPL2    = $0C          ; respPtr+1
ZP_RSPH2    = $0D
ZP_TICKL    = $0E          ; ticket low bytes
ZP_TICKH    = $0F

main_loop:
    ; Read tail
    LDA RING_TAIL
    STA ZP_TAIL

    ; Read head
    LDA RING_HEAD

    ; Compare: if tail == head, ring empty
    CMP ZP_TAIL
    BEQ main_loop           ; empty, poll again

    ; Compute entry address: $2308 + tail*32
    ; tail*32 = tail << 5
    LDA ZP_TAIL
    ASL A                   ; *2
    ASL A                   ; *4
    ASL A                   ; *8
    ASL A                   ; *16
    ASL A                   ; *32
    CLC
    ADC #ENTRIES_LO         ; + $08
    STA ZP_ENTL
    LDA #ENTRIES_HI         ; $23
    ADC #$00                ; carry from low add
    STA ZP_ENTH

    ; Read op at entry+8
    LDY #8
    LDA (ZP_ENTL),Y         ; op low byte
    CMP #1                  ; op == 1?
    BEQ op_ok
    JMP error_resp
op_ok:

    ; Read reqPtr at entry+16 (little-endian uint32, we use low 16 bits)
    LDY #16
    LDA (ZP_ENTL),Y
    STA ZP_REQPL
    INY
    LDA (ZP_ENTL),Y
    STA ZP_REQPH

    ; Read respPtr at entry+24
    LDY #24
    LDA (ZP_ENTL),Y
    STA ZP_RSPL
    INY
    LDA (ZP_ENTL),Y
    STA ZP_RSPH

    ; Read ticket at entry+0 (low 2 bytes for response)
    LDY #0
    LDA (ZP_ENTL),Y
    STA ZP_TICKL
    INY
    LDA (ZP_ENTL),Y
    STA ZP_TICKH

    ; Op=1: add two uint32 from reqPtr
    ; For simplicity, do 32-bit add (4 bytes each)
    LDY #0
    CLC
    LDA (ZP_REQPL),Y        ; val1 byte 0
    LDY #4
    ADC (ZP_REQPL),Y        ; + val2 byte 0
    LDY #0
    STA (ZP_RSPL),Y         ; result byte 0

    LDY #1
    LDA (ZP_REQPL),Y        ; val1 byte 1
    LDY #5
    ADC (ZP_REQPL),Y        ; + val2 byte 1
    LDY #1
    STA (ZP_RSPL),Y         ; result byte 1

    LDY #2
    LDA (ZP_REQPL),Y        ; val1 byte 2
    LDY #6
    ADC (ZP_REQPL),Y        ; + val2 byte 2
    LDY #2
    STA (ZP_RSPL),Y         ; result byte 2

    LDY #3
    LDA (ZP_REQPL),Y        ; val1 byte 3
    LDY #7
    ADC (ZP_REQPL),Y        ; + val2 byte 3
    LDY #3
    STA (ZP_RSPL),Y         ; result byte 3

    ; Write response descriptor at $2508 + tail*16
    LDA ZP_TAIL
    ASL A                   ; *2
    ASL A                   ; *4
    ASL A                   ; *8
    ASL A                   ; *16
    CLC
    ADC #RESP_LO            ; + $08
    STA ZP_RESPL
    LDA #RESP_HI            ; $25
    ADC #$00
    STA ZP_RESPH

    ; response.ticket (4 bytes, low 2 from saved)
    LDY #0
    LDA ZP_TICKL
    STA (ZP_RESPL),Y
    INY
    LDA ZP_TICKH
    STA (ZP_RESPL),Y
    INY
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

    ; response.status = 2 (ok)
    LDY #4
    LDA #2
    STA (ZP_RESPL),Y
    INY
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

    ; response.resultCode = 0
    LDY #8
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

    ; response.respLen = 4
    LDY #12
    LDA #4
    STA (ZP_RESPL),Y
    INY
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

    ; Advance tail: (tail + 1) & 15
    LDA ZP_TAIL
    CLC
    ADC #1
    AND #$0F
    STA RING_TAIL

    JMP main_loop

error_resp:
    ; Write error response (status=3)
    LDA ZP_TAIL
    ASL A
    ASL A
    ASL A
    ASL A
    CLC
    ADC #RESP_LO
    STA ZP_RESPL
    LDA #RESP_HI
    ADC #$00
    STA ZP_RESPH

    ; ticket
    LDY #0
    LDA (ZP_ENTL),Y
    STA (ZP_RESPL),Y
    INY
    LDA (ZP_ENTL),Y
    STA (ZP_RESPL),Y
    LDY #2
    LDA #0
    STA (ZP_RESPL),Y
    INY
    STA (ZP_RESPL),Y

    ; status = 3 (error)
    LDY #4
    LDA #3
    STA (ZP_RESPL),Y

    ; Advance tail
    LDA ZP_TAIL
    CLC
    ADC #1
    AND #$0F
    STA RING_TAIL

    JMP main_loop

; Reset vector
.org $FFFC
.word $0000                 ; RESET → $0000
.word $0000                 ; IRQ/BRK → $0000
