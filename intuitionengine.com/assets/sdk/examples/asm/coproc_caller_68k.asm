; Coprocessor mailbox caller: M68020 guest code that starts the matching worker, writes two operands to shared guest RAM, enqueues an add request and polls its ticket.
;
; The caller and service communicate through shared guest RAM and coprocessor MMIO.
; Read the
; mailbox constants, initialisation, descriptor exchange and terminal path in order.
; Compare coproc_service_68k.asm as its matching pair and the other CPU ports for their
; real addressing differences.
;
; The ie68.inc include file provides convenience macros (coproc_start,
; coproc_enqueue) that expand to the required sequence of move.l writes.
; These macros make the M68K caller code significantly more compact than
; the 8-bit equivalents.
;
; Register usage:
;   D0  - scratch / status checks / final result
;   D2  - saved ticket number (preserved across poll loop)
;
; === MEMORY MAP ===
;
;   Address       Size    Purpose
;   ----------    ------  -------------------------------------------
;   $001000       ~64 B   Program code
;   $400000       varies  Service filename string (bus memory)
;   $410000       8 B     Request data buffer (two uint32 operands)
;   $410100       4 B     Response data buffer (one uint32 result)
;   $F2340-$F237F 64 B    Coprocessor MMIO registers (direct access)
;
; === BUILD AND RUN ===
;   vasmm68k_mot -Fbin -m68020 -devpac -o coproc_caller_68k.ie68 coproc_caller_68k.asm
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    include "ie68.inc"

    org $1000

; ============================================================================
; PHASE 1 - START THE M68K WORKER
; ============================================================================
;
; The coproc_start macro writes COPROC_CPU_TYPE and COPROC_NAME_PTR,
; then triggers COPROC_CMD_START. On the M68K, these are direct 32-bit
; memory writes -- no gateway window or byte-level decomposition needed.
; We check CMD_STATUS afterwards: zero means success, non-zero means the
; worker failed to launch (e.g., binary not found).

    coproc_start COPROC_CPU_M68K,$400000

    ; Verify the start command succeeded
    move.l  COPROC_CMD_STATUS,d0
    tst.l   d0
    bne     error

; ============================================================================
; PHASE 2 - PREPARE REQUEST DATA AND ENQUEUE
; ============================================================================
;
; Unlike the 8-bit callers, the M68K can write the operands directly
; into bus memory with move.l instructions. We place 10 at $410000 and 20
; at $410004, forming an 8-byte request payload (two uint32 values).
;
; The coproc_enqueue macro fills in CPU type, operation code, request
; pointer/length, and response pointer/capacity, then triggers the
; ENQUEUE command. The controller allocates a ticket and writes it to
; the COPROC_TICKET register.

    ; Write the two operands into the request buffer
    move.l  #10,$410000                ; operand 1
    move.l  #20,$410004                ; operand 2

    ; Enqueue: CPU=M68K, op=1(add), req=$410000(8 bytes), resp=$410100(4 bytes)
    coproc_enqueue COPROC_CPU_M68K,1,$410000,8,$410100,4

    ; Save the ticket for polling
    move.l  COPROC_TICKET,d2           ; D2 = ticket (preserved across loop)

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
    move.l  d2,COPROC_TICKET
    move.l  #COPROC_CMD_POLL,COPROC_CMD
    move.l  COPROC_TICKET_STATUS,d0
    cmpi.l  #COPROC_ST_OK,d0
    beq     done
    cmpi.l  #COPROC_ST_ERROR,d0
    beq     error
    bra     poll_loop

; ============================================================================
; PHASE 4 - READ RESULT
; ============================================================================
;
; The worker has written the sum into the response buffer at $410100.
; On the M68K we can read it directly -- no bank window needed. The
; expected result is 30 (10 + 20). We load it into D0 and halt.

done:
    move.l  $410100,d0                 ; D0 = result (should be 30)
    stop    #$2700

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; If START failed, COPROC_CMD_ERROR contains a diagnostic code.
; We load it into D0 for inspection and halt.

error:
    move.l  COPROC_CMD_ERROR,d0
    stop    #$2700
