; coproc_caller_x86.asm - x86 Coprocessor Caller Example
;
; Demonstrates an x86 program launching an IE32 worker, enqueueing
; an add request (10 + 20), polling until complete, and reading the result.
;
; Usage:
;   nasm -f bin -o coproc_caller_x86.ie86 coproc_caller_x86.asm
;   bin/ie -x86 coproc_caller_x86.ie86
;
; Prerequisites:
;   - Worker binary 'coproc_service_ie32.ie32' in the working directory
;   - Service filename string at bus address 0x400000
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

%include "ie86.inc"

    bits 32
    org 0x1000

; ---- START IE32 worker ----
    coproc_start COPROC_CPU_IE32, 0x400000

    ; Check status
    mov eax, [COPROC_CMD_STATUS]
    test eax, eax
    jnz error

; ---- Write request data: 10 + 20 ----
    mov dword [0x410000], 10           ; val1
    mov dword [0x410004], 20           ; val2

; ---- ENQUEUE request ----
    coproc_enqueue COPROC_CPU_IE32, 1, 0x410000, 8, 0x410100, 4

    ; Save ticket
    mov ebx, [COPROC_TICKET]           ; EBX = ticket

; ---- POLL until complete ----
poll_loop:
    mov dword [COPROC_TICKET], ebx
    mov dword [COPROC_CMD], COPROC_CMD_POLL
    mov eax, [COPROC_TICKET_STATUS]
    cmp eax, COPROC_ST_OK
    je done
    jmp poll_loop

done:
    ; Result is at 0x410100 (should be 30)
    mov eax, [0x410100]                ; EAX = result
    hlt

error:
    mov eax, [COPROC_CMD_ERROR]
    hlt
