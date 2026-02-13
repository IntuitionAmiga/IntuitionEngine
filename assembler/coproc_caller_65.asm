; coproc_caller_65.asm - 6502 Coprocessor Caller Example
;
; Demonstrates a 6502 program launching an IE32 worker via the gateway
; window ($F200-$F23F), enqueueing an add request, polling until
; complete, and reading the result.
;
; Usage:
;   make ie65asm SRC=assembler/coproc_caller_65.asm
;   bin/ie -6502 assembler/coproc_caller_65.ie65
;
; Prerequisites:
;   - Worker binary 'coproc_service_ie32.ie32' in the working directory
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

.include "ie65.inc"

.segment "CODE"
.org $0600

; The gateway at $F200-$F23F maps to bus $F2340-$F237F.
; All COPROC registers are accessed via byte-level writes using STORE32.
; Data buffers use bank windows to access bus memory above $FFFF.

; ---- START IE32 worker ----
    ; Set CPU type = IE32 (1)
    STORE32 COPROC_CPU_TYPE, COPROC_CPU_IE32

    ; Set name pointer to bus address where filename string lives
    ; (Must be pre-loaded into bus memory by the host before execution)
    STORE32 COPROC_NAME_PTR, $400000

    ; Issue START command
    STORE32 COPROC_CMD, COPROC_CMD_START

    ; Check status
    LDA COPROC_CMD_STATUS
    BNE error

; ---- ENQUEUE request ----
    STORE32 COPROC_CPU_TYPE, COPROC_CPU_IE32
    STORE32 COPROC_OP, 1               ; op = add
    STORE32 COPROC_REQ_PTR, $410000    ; request data at bus $410000
    STORE32 COPROC_REQ_LEN, 8          ; two uint32
    STORE32 COPROC_RESP_PTR, $410100   ; response buffer
    STORE32 COPROC_RESP_CAP, 4         ; 4 bytes
    STORE32 COPROC_CMD, COPROC_CMD_ENQUEUE

    ; Save ticket (byte 0 — tickets are small integers)
    LDA COPROC_TICKET
    STA $00                            ; save ticket in ZP

; ---- POLL until complete ----
poll_loop:
    ; Write ticket back to register
    LDA $00
    STA COPROC_TICKET
    LDA #0
    STA COPROC_TICKET+1
    STA COPROC_TICKET+2
    STA COPROC_TICKET+3

    ; Issue POLL command
    STORE32 COPROC_CMD, COPROC_CMD_POLL

    ; Read ticket status
    LDA COPROC_TICKET_STATUS
    CMP #COPROC_ST_OK                  ; compare with OK (2)
    BEQ done
    JMP poll_loop

done:
    ; Result is at bus $410100 — read via bank window or verify from host
    LDA #$FF
    STA $0200                          ; signal completion
    JMP *                              ; spin

error:
    LDA COPROC_CMD_ERROR
    STA $0201                          ; store error code
    JMP *                              ; spin
