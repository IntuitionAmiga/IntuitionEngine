; coproc_caller_z80.asm - Z80 Coprocessor Caller Example
;
; Demonstrates a Z80 program launching an M68K worker via the gateway
; window (0xF200-0xF23F), enqueueing an add request, polling until
; complete, and reading the result.
;
; Usage:
;   vasmz80_std -Fbin -o coproc_caller_z80.ie80 coproc_caller_z80.asm
;   bin/ie -z80 coproc_caller_z80.ie80
;
; Prerequisites:
;   - Worker binary 'coproc_service_68k.ie68' in the working directory
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

    .include "ie80.inc"

    .org 0x0000

; The gateway at 0xF200-0xF23F maps to bus 0xF2340-0xF237F.
; All COPROC registers are accessed via byte-level writes using STORE32.
; Data buffers use bank windows to access bus memory above 0xFFFF.

; ---- START M68K worker ----
    ; Set CPU type = M68K (4)
    STORE32 COPROC_CPU_TYPE COPROC_CPU_M68K

    ; Set name pointer to bus address where filename string lives
    ; (Must be pre-loaded into bus memory by the host before execution)
    STORE32 COPROC_NAME_PTR 0x400000

    ; Issue START command
    STORE32 COPROC_CMD COPROC_CMD_START

    ; Check status (read byte 0 of CMD_STATUS)
    ld a,(COPROC_CMD_STATUS)
    or a
    jp nz,error

; ---- ENQUEUE request ----
    STORE32 COPROC_CPU_TYPE COPROC_CPU_M68K
    STORE32 COPROC_OP 1                ; op = add
    STORE32 COPROC_REQ_PTR 0x410000    ; request data at bus 0x410000
    STORE32 COPROC_REQ_LEN 8           ; two uint32
    STORE32 COPROC_RESP_PTR 0x410100   ; response buffer
    STORE32 COPROC_RESP_CAP 4          ; 4 bytes
    STORE32 COPROC_CMD COPROC_CMD_ENQUEUE

    ; Read ticket (byte 0 only — tickets are small integers)
    ld a,(COPROC_TICKET)
    ld b,a                             ; B = ticket

; ---- POLL until complete ----
poll_loop:
    ; Write ticket back
    ld a,b
    ld (COPROC_TICKET),a
    ld a,0
    ld (COPROC_TICKET+1),a
    ld (COPROC_TICKET+2),a
    ld (COPROC_TICKET+3),a

    ; Issue POLL command
    STORE32 COPROC_CMD COPROC_CMD_POLL

    ; Read ticket status (byte 0)
    ld a,(COPROC_TICKET_STATUS)
    cp COPROC_ST_OK                    ; compare with OK (2)
    jr z,done
    jr poll_loop

done:
    ; Result is at bus 0x410100 — read via bank window or verify from host
    halt

error:
    ; CMD_ERROR contains error code
    ld a,(COPROC_CMD_ERROR)
    halt
