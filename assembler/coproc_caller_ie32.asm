; coproc_caller_ie32.asm - IE32 Coprocessor Caller Example
;
; Demonstrates an IE32 program launching an IE32 worker, enqueueing
; an add request (10 + 20), polling until complete, and reading the result.
;
; Usage:
;   bin/ie32asm assembler/coproc_caller_ie32.asm
;   bin/ie assembler/coproc_caller_ie32.ie32
;
; Prerequisites:
;   - Worker binary 'coproc_service_ie32.ie32' in the working directory
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

    .include "ie32.inc"

    org 0x1000

; Data layout:
;   0x400000: service filename string
;   0x410000: request data (two uint32: val1, val2)
;   0x410100: response buffer (uint32 result)

; ---- Store service filename at 0x400000 ----
    load    A, 'c'
    store   A, 0x400000
    load    A, 'o'
    store   A, 0x400004
    load    A, 'p'
    store   A, 0x400008
    load    A, 'r'
    store   A, 0x40000C
    load    A, 'o'
    store   A, 0x400010
    load    A, 'c'
    store   A, 0x400014
    load    A, '_'
    store   A, 0x400018
    load    A, 's'
    store   A, 0x40001C
    load    A, 'e'
    store   A, 0x400020
    load    A, 'r'
    store   A, 0x400024
    load    A, 'v'
    store   A, 0x400028
    load    A, 'i'
    store   A, 0x40002C
    load    A, 'c'
    store   A, 0x400030
    load    A, 'e'
    store   A, 0x400034
    load    A, '_'
    store   A, 0x400038
    load    A, 'i'
    store   A, 0x40003C
    load    A, 'e'
    store   A, 0x400040
    load    A, '3'
    store   A, 0x400044
    load    A, '2'
    store   A, 0x400048
    load    A, '.'
    store   A, 0x40004C
    load    A, 'i'
    store   A, 0x400050
    load    A, 'e'
    store   A, 0x400054
    load    A, '3'
    store   A, 0x400058
    load    A, '2'
    store   A, 0x40005C
    load    A, 0
    store   A, 0x400060                ; null terminator

; ---- START worker ----
    load    A, COPROC_CPU_IE32
    store   A, COPROC_CPU_TYPE
    load    A, 0x400000
    store   A, COPROC_NAME_PTR
    load    A, COPROC_CMD_START
    store   A, COPROC_CMD

    ; Check status
    load    A, COPROC_CMD_STATUS
    jnz     A, error                   ; if status != 0, start failed

; ---- Write request data: 10 + 20 ----
    load    A, 10
    store   A, 0x410000                ; val1
    load    A, 20
    store   A, 0x410004                ; val2

; ---- ENQUEUE request ----
    load    A, COPROC_CPU_IE32
    store   A, COPROC_CPU_TYPE
    load    A, 1                       ; op = add
    store   A, COPROC_OP
    load    A, 0x410000
    store   A, COPROC_REQ_PTR
    load    A, 8
    store   A, COPROC_REQ_LEN
    load    A, 0x410100
    store   A, COPROC_RESP_PTR
    load    A, 4
    store   A, COPROC_RESP_CAP
    load    A, COPROC_CMD_ENQUEUE
    store   A, COPROC_CMD

    ; Save ticket from COPROC_TICKET
    load    X, COPROC_TICKET           ; X = ticket

; ---- POLL until complete ----
poll_loop:
    store   X, COPROC_TICKET
    load    A, COPROC_CMD_POLL
    store   A, COPROC_CMD
    load    A, COPROC_TICKET_STATUS
    sub     A, #COPROC_ST_OK           ; compare with OK (2)
    jz      A, done
    jmp     poll_loop

done:
    ; Result is at 0x410100 (should be 30)
    load    A, 0x410100                ; A = result
    halt

error:
    load    A, COPROC_CMD_ERROR
    halt
