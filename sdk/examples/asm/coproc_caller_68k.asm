; ============================================================================
; COPROCESSOR CALLER - M68K HOST LAUNCHING AN IE32 WORKER
; M68020 Assembly for IntuitionEngine - Headless IPC Demo
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Motorola 68020 (caller/host side)
; Video Chip:    None (headless)
; Audio Engine:  None
; Assembler:     vasmm68k_mot (VASM M68K, Motorola syntax)
; Build:         vasmm68k_mot -Fbin -m68020 -devpac -o coproc_caller_68k.ie68 coproc_caller_68k.asm
; Run:           ./bin/IntuitionEngine -m68k coproc_caller_68k.ie68 -coproc coproc_service_68k.ie68
; Porting:       Mailbox protocol is CPU-agnostic. See coproc_caller_65.asm
;                (6502), coproc_caller_x86.asm (x86), coproc_caller_z80.asm
;                (Z80) for the same demo on other CPU cores.
;
; === WHAT THIS DEMO DOES ===
; 1. Launches an IE32 coprocessor worker from a service binary
; 2. Writes two operands (10 and 20) into the request buffer
; 3. Enqueues an "add" request (operation code 1)
; 4. Polls the ticket status register until the worker completes
; 5. Reads the result (expected: 30) into D0
;
; === WHY COPROCESSOR MAILBOX IPC ===
; The Intuition Engine supports up to six heterogeneous CPU cores running
; concurrently: IE32, IE64, 6502, Z80, M68K, and x86. These cores need
; a structured way to delegate work to one another -- for example, an M68K
; host offloading computation to a 32-bit IE32 worker.
;
; The coprocessor subsystem provides this via a memory-mapped mailbox
; protocol. The caller writes commands to MMIO registers in the coprocessor
; region ($F2340-$F237F on the M68K bus). Each command is atomic: writing
; to COPROC_CMD triggers the operation using whatever values are currently
; in the other registers.
;
; This design follows a long tradition of hardware mailbox systems:
; the Amiga's copper command lists, DSP mailboxes in game consoles like
; the Super Nintendo and Sega Saturn, and modern GPU command queues all
; use the same fundamental pattern -- a producer writes descriptors into
; shared memory, a consumer processes them asynchronously, and status
; registers provide synchronisation. The ticket-based polling model here
; is deliberately simple, avoiding interrupt-driven complexity.
;
; === M68K-SPECIFIC NOTES ===
; The M68K has a flat 32-bit address space with no bank-switching or
; gateway windows -- all bus addresses are directly accessible. This makes
; the M68K the most straightforward CPU for coprocessor work: register
; writes are simple move.l instructions, and result data can be read
; directly from bus memory without any windowing.
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
;   ./bin/IntuitionEngine -m68k coproc_caller_68k.ie68 -coproc coproc_service_68k.ie68
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    include "ie68.inc"

    org $1000

; ============================================================================
; PHASE 1 - START THE IE32 WORKER
; ============================================================================
;
; WHY: The coproc_start macro writes COPROC_CPU_TYPE and COPROC_NAME_PTR,
; then triggers COPROC_CMD_START. On the M68K, these are direct 32-bit
; memory writes -- no gateway window or byte-level decomposition needed.
; We check CMD_STATUS afterwards: zero means success, non-zero means the
; worker failed to launch (e.g., binary not found).

    coproc_start COPROC_CPU_IE32,$400000

    ; Verify the start command succeeded
    move.l  COPROC_CMD_STATUS,d0
    tst.l   d0
    bne     error

; ============================================================================
; PHASE 2 - PREPARE REQUEST DATA AND ENQUEUE
; ============================================================================
;
; WHY: Unlike the 8-bit callers, the M68K can write the operands directly
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

    ; Enqueue: CPU=IE32, op=1(add), req=$410000(8 bytes), resp=$410100(4 bytes)
    coproc_enqueue COPROC_CPU_IE32,1,$410000,8,$410100,4

    ; Save the ticket for polling
    move.l  COPROC_TICKET,d2           ; D2 = ticket (preserved across loop)

; ============================================================================
; PHASE 3 - POLL UNTIL COMPLETE
; ============================================================================
;
; WHY: The worker processes the request asynchronously in its own CPU
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
    bra     poll_loop

; ============================================================================
; PHASE 4 - READ RESULT
; ============================================================================
;
; WHY: The worker has written the sum into the response buffer at $410100.
; On the M68K we can read it directly -- no bank window needed. The
; expected result is 30 (10 + 20). We load it into D0 and halt.

done:
    move.l  $410100,d0                 ; D0 = result (should be 30)
    stop    #$2700

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; WHY: If START failed, COPROC_CMD_ERROR contains a diagnostic code.
; We load it into D0 for inspection and halt.

error:
    move.l  COPROC_CMD_ERROR,d0
    stop    #$2700
