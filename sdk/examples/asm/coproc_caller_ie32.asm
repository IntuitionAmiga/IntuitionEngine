; ============================================================================
; COPROCESSOR CALLER - INTER-CPU COMMUNICATION REFERENCE
; IE32 Assembly for IntuitionEngine - Multi-CPU IPC via Ring Buffer
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC) - caller side
; Video Chip:    None (terminal output only)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         ./bin/ie32asm sdk/examples/asm/coproc_caller_ie32.asm
;                ./bin/ie32asm sdk/examples/asm/coproc_service_ie32.asm
; Run:           ./bin/IntuitionEngine -ie32 coproc_caller_ie32.iex
; Porting:       Coprocessor MMIO is CPU-agnostic. Caller/service pairs can
;                mix CPU cores (e.g. M68K caller + Z80 service).
;
; === WHAT THIS DEMO DOES ===
; Demonstrates an IE32 program launching an IE32 worker, enqueueing
; an add request (10 + 20), polling until complete, and reading the result.
; This is the reference implementation for inter-CPU communication using
; the coprocessor mailbox ring buffer protocol.
;
; Prerequisites:
;   - Worker binary 'coproc_service_ie32.iex' in the working directory
;   - Service filename string at bus address 0x400000
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .include "ie32.inc"

    .org 0x1000

; Data layout:
;   0x400000: service filename string (pre-loaded by host)
;   0x410000: request data (two uint32: val1, val2)
;   0x410100: response buffer (uint32 result)

; ---- START worker ----
    LOAD    A, #COPROC_CPU_IE32
    STORE   A, COPROC_CPU_TYPE
    LOAD    A, #0x400000
    STORE   A, COPROC_NAME_PTR
    LOAD    A, #COPROC_CMD_START
    STORE   A, COPROC_CMD

    ; Check status
    LOAD    A, COPROC_CMD_STATUS
    JNZ     A, error                   ; if status != 0, start failed

; ---- Write request data: 10 + 20 ----
    LOAD    A, #10
    STORE   A, 0x410000                ; val1
    LOAD    A, #20
    STORE   A, 0x410004                ; val2

; ---- ENQUEUE request ----
    LOAD    A, #COPROC_CPU_IE32
    STORE   A, COPROC_CPU_TYPE
    LOAD    A, #1                      ; op = add
    STORE   A, COPROC_OP
    LOAD    A, #0x410000
    STORE   A, COPROC_REQ_PTR
    LOAD    A, #8
    STORE   A, COPROC_REQ_LEN
    LOAD    A, #0x410100
    STORE   A, COPROC_RESP_PTR
    LOAD    A, #4
    STORE   A, COPROC_RESP_CAP
    LOAD    A, #COPROC_CMD_ENQUEUE
    STORE   A, COPROC_CMD

    ; Save ticket from COPROC_TICKET
    LOAD    X, COPROC_TICKET           ; X = ticket

; ---- POLL until complete ----
poll_loop:
    STORE   X, COPROC_TICKET
    LOAD    A, #COPROC_CMD_POLL
    STORE   A, COPROC_CMD
    LOAD    A, COPROC_TICKET_STATUS
    SUB     A, #COPROC_ST_OK           ; compare with OK (2)
    JZ      A, done
    JMP     poll_loop

done:
    ; Result is at 0x410100 (should be 30)
    LOAD    A, 0x410100                ; A = result
    HALT

error:
    LOAD    A, COPROC_CMD_ERROR
    HALT
