; coproc_caller_68k.asm - M68K Coprocessor Caller Example
;
; Demonstrates an M68K program launching an IE32 worker, enqueueing
; an add request (10 + 20), polling until complete, and reading the result.
;
; Usage:
;   vasmm68k_mot -Fbin -m68020 -devpac -o coproc_caller_68k.ie68 coproc_caller_68k.asm
;   bin/ie -m68k coproc_caller_68k.ie68
;
; Prerequisites:
;   - Worker binary 'coproc_service_ie32.ie32' in the working directory
;   - Service filename string at bus address $400000
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

    include "ie68.inc"

    org $1000

; ---- START IE32 worker ----
    coproc_start COPROC_CPU_IE32,$400000

    ; Check status
    move.l  COPROC_CMD_STATUS,d0
    tst.l   d0
    bne     error

; ---- Write request data: 10 + 20 ----
    move.l  #10,$410000                ; val1
    move.l  #20,$410004                ; val2

; ---- ENQUEUE request ----
    coproc_enqueue COPROC_CPU_IE32,1,$410000,8,$410100,4

    ; Save ticket
    move.l  COPROC_TICKET,d2           ; D2 = ticket

; ---- POLL until complete ----
poll_loop:
    move.l  d2,COPROC_TICKET
    move.l  #COPROC_CMD_POLL,COPROC_CMD
    move.l  COPROC_TICKET_STATUS,d0
    cmpi.l  #COPROC_ST_OK,d0
    beq     done
    bra     poll_loop

done:
    ; Result is at $410100 (should be 30)
    move.l  $410100,d0                 ; D0 = result
    stop    #$2700

error:
    move.l  COPROC_CMD_ERROR,d0
    stop    #$2700
