; Coprocessor mailbox caller: x86 guest code that starts the matching worker, writes two operands to shared guest RAM, enqueues an add request and polls its ticket.
;
; The caller and service communicate through shared guest RAM and coprocessor MMIO.
; Read the
; mailbox constants, initialisation, descriptor exchange and terminal path in order.
; Compare coproc_service_x86.asm as its matching pair and the other CPU ports for their
; real addressing differences.
;
; The ie86.inc include file provides convenience macros (coproc_start,
; coproc_enqueue) that expand to sequences of `mov dword [addr], value`
; writes. Register usage is conventional:
;
;   EAX - scratch / status checks / final result
;   EBX - saved ticket number (preserved across poll loop)
;
; === MEMORY MAP ===
;
;   Address         Size    Purpose
;   ------------    ------  -------------------------------------------
;   0x001000        ~64 B   Program code
;   0x400000        varies  Service filename string (bus memory)
;   0x410000        8 B     Request data buffer (two uint32 operands)
;   0x410100        4 B     Response data buffer (one uint32 result)
;   0xF2340-0xF237F 64 B    Coprocessor MMIO registers (direct access)
;
; === BUILD AND RUN ===
;   sdk/bin/ie32asm sdk/examples/asm/coproc_caller_x86.asm
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

%include "ie86.inc"

    bits 32
    org 0x1000

; ============================================================================
; PHASE 1 - START THE X86 WORKER
; ============================================================================
;
; The coproc_start macro writes COPROC_CPU_TYPE and COPROC_NAME_PTR,
; then triggers COPROC_CMD_START. On the x86, these are direct 32-bit
; memory writes via `mov dword [addr], imm32`. We check CMD_STATUS
; afterwards: zero means success, non-zero means the worker failed to
; launch (e.g., binary not found).

    coproc_start COPROC_CPU_X86, 0x400000

    ; Verify the start command succeeded
    mov eax, [COPROC_CMD_STATUS]
    test eax, eax
    jnz error

; ============================================================================
; PHASE 2 - PREPARE REQUEST DATA AND ENQUEUE
; ============================================================================
;
; The x86 can write operands directly into bus memory. We place 10
; at 0x410000 and 20 at 0x410004, forming an 8-byte request payload
; (two uint32 values). The coproc_enqueue macro fills in all descriptor
; fields and triggers ENQUEUE. The controller allocates a ticket and
; writes it to the COPROC_TICKET register.

    ; Write the two operands into the request buffer
    mov dword [0x410000], 10           ; operand 1
    mov dword [0x410004], 20           ; operand 2

    ; Enqueue: CPU=X86, op=1(add), req=0x410000(8 bytes), resp=0x410100(4 bytes)
    coproc_enqueue COPROC_CPU_X86, 1, 0x410000, 8, 0x410100, 4

    ; Save the ticket for polling
    mov ebx, [COPROC_TICKET]           ; EBX = ticket (preserved across loop)

; ============================================================================
; PHASE 3 - POLL UNTIL COMPLETE
; ============================================================================
;
; The worker processes the request asynchronously in its own CPU
; thread. We poll by writing the ticket back to COPROC_TICKET, issuing
; COPROC_CMD_POLL, and checking COPROC_TICKET_STATUS. The status
; transitions through PENDING(0) -> RUNNING(1) -> OK(2) on success.
; We loop until we see COPROC_ST_OK.

poll_loop:
    mov dword [COPROC_TICKET], ebx
    mov dword [COPROC_CMD], COPROC_CMD_POLL
    mov eax, [COPROC_TICKET_STATUS]
    cmp eax, COPROC_ST_OK
    je done
    cmp eax, COPROC_ST_ERROR
    je error
    jmp poll_loop

; ============================================================================
; PHASE 4 - READ RESULT
; ============================================================================
;
; The worker has written the sum into the response buffer at 0x410100.
; On the x86 we can read it directly -- no bank window needed. The
; expected result is 30 (10 + 20). We load it into EAX and halt.

done:
    mov eax, [0x410100]                ; EAX = result (should be 30)
    hlt

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; If START failed, COPROC_CMD_ERROR contains a diagnostic code.
; We load it into EAX for inspection and halt.

error:
    mov eax, [COPROC_CMD_ERROR]
    hlt
