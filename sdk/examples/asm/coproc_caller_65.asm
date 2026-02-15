; ============================================================================
; COPROCESSOR CALLER - 6502 HOST LAUNCHING AN IE32 WORKER
; ca65/ld65 Assembly for IntuitionEngine - Headless IPC Demo
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    MOS 6502 (caller/host side)
; Video Chip:    None (headless)
; Audio Engine:  None
; Assembler:     ca65/ld65 (cc65 toolchain)
; Build:         make ie65asm SRC=sdk/examples/asm/coproc_caller_65.asm
; Run:           ./bin/IntuitionEngine -6502 coproc_caller_65.ie65 -coproc coproc_service_65.ie65
; Porting:       Mailbox protocol is CPU-agnostic. See coproc_caller_68k.asm
;                (M68K), coproc_caller_x86.asm (x86), coproc_caller_z80.asm
;                (Z80) for the same demo on other CPU cores.
;
; === WHAT THIS DEMO DOES ===
; 1. Launches an IE32 coprocessor worker from a service binary
; 2. Enqueues an "add" request (operation code 1) with two operands
; 3. Polls the ticket status register until the worker completes
; 4. Reads the result and signals completion via a memory flag
;
; === WHY COPROCESSOR MAILBOX IPC ===
; The Intuition Engine supports up to six heterogeneous CPU cores running
; concurrently: IE32, IE64, 6502, Z80, M68K, and x86. These cores need
; a structured way to delegate work to one another -- for example, a 6502
; host offloading heavy computation to a 32-bit IE32 worker.
;
; The coprocessor subsystem provides this via a memory-mapped mailbox
; protocol. The caller writes commands to a set of MMIO registers (the
; "gateway window" at $F200-$F23F in 6502 address space), which the bus
; maps to the coprocessor controller at bus addresses $F2340-$F237F.
; Each command is atomic: writing to COPROC_CMD triggers the operation
; using whatever values are currently in the other registers.
;
; This design follows a long tradition of hardware mailbox systems:
; the Amiga's copper command lists, DSP mailboxes in game consoles like
; the Super Nintendo and Sega Saturn, and modern GPU command queues all
; use the same fundamental pattern -- a producer writes descriptors into
; shared memory, a consumer processes them asynchronously, and status
; registers provide synchronisation. The ticket-based polling model here
; is deliberately simple: it avoids interrupts and lock-free ring buffer
; complexity on the caller side, making it accessible from even the most
; constrained 8-bit CPUs.
;
; === 6502-SPECIFIC NOTES ===
; The 6502 has a 16-bit address space ($0000-$FFFF), but the coprocessor
; registers and data buffers live at bus addresses above $FFFF ($400000,
; $410000, etc.). The gateway window at $F200 solves the register access
; problem, but data buffers must be pre-loaded into bus memory by the
; host environment before execution.
;
; All coprocessor registers are 32-bit, but the 6502 can only write one
; byte at a time. The STORE32 macro in ie65.inc handles this by writing
; four consecutive bytes in little-endian order. Similarly, reading a
; 32-bit ticket requires reading up to four bytes -- but since ticket
; numbers are small integers, only byte 0 is meaningful in practice.
;
; === MEMORY MAP ===
;
;   Address       Size    Purpose
;   ----------    ------  -------------------------------------------
;   $0000-$00FF   256 B   Zero page (ticket stored at $00)
;   $0200-$0201   2 B     Status flags (completion / error reporting)
;   $0600+        ~128 B  Program code
;   $F200-$F23F   64 B    Coprocessor gateway window (MMIO registers)
;   $400000       varies  Service filename string (bus memory, pre-loaded)
;   $410000       8 B     Request data buffer (two uint32 operands)
;   $410100       4 B     Response data buffer (one uint32 result)
;
; === BUILD AND RUN ===
;   make ie65asm SRC=sdk/examples/asm/coproc_caller_65.asm
;   ./bin/IntuitionEngine -6502 coproc_caller_65.ie65 -coproc coproc_service_65.ie65
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

.include "ie65.inc"

.segment "CODE"
.org $0600

; ============================================================================
; GATEWAY WINDOW OVERVIEW
; ============================================================================
; The gateway at $F200-$F23F maps to bus $F2340-$F237F.
; All COPROC registers are accessed via byte-level writes using STORE32.
; Data buffers use bank windows to access bus memory above $FFFF.

; ============================================================================
; PHASE 1 - START THE IE32 WORKER
; ============================================================================
;
; WHY: Before we can enqueue any work, the coprocessor controller needs
; to know which CPU type to spawn and where to find the service binary.
; COPROC_CPU_TYPE selects the worker architecture; COPROC_NAME_PTR points
; to a null-terminated filename string in bus memory. Writing COPROC_CMD
; with COPROC_CMD_START triggers the launch. We then check CMD_STATUS to
; confirm the worker started without error.

    ; Select IE32 as the worker CPU type
    STORE32 COPROC_CPU_TYPE, COPROC_CPU_IE32

    ; Point to the service binary filename in bus memory
    ; (must be pre-loaded into bus memory by the host before execution)
    STORE32 COPROC_NAME_PTR, $400000

    ; Trigger the START command
    STORE32 COPROC_CMD, COPROC_CMD_START

    ; Check whether the start succeeded (0 = OK, non-zero = error)
    LDA COPROC_CMD_STATUS
    BEQ no_start_err
    JMP error
no_start_err:

; ============================================================================
; PHASE 2 - ENQUEUE AN ADD REQUEST
; ============================================================================
;
; WHY: Now that the worker is running, we fill in the request descriptor.
; The protocol requires: CPU type (to route to the correct worker ring),
; operation code (op=1 means "add"), pointers to the request and response
; buffers in bus memory, and their sizes. Writing COPROC_CMD_ENQUEUE
; submits the request and returns a ticket number in COPROC_TICKET.
;
; The request buffer at $410000 contains two uint32 values (the operands).
; The response buffer at $410100 will receive one uint32 (the sum).
; These buffers must be pre-populated by the host environment.

    STORE32 COPROC_CPU_TYPE, COPROC_CPU_IE32
    STORE32 COPROC_OP, 1               ; op = add
    STORE32 COPROC_REQ_PTR, $410000    ; request data at bus $410000
    STORE32 COPROC_REQ_LEN, 8          ; two uint32 = 8 bytes
    STORE32 COPROC_RESP_PTR, $410100   ; response buffer
    STORE32 COPROC_RESP_CAP, 4         ; capacity: 4 bytes (one uint32)
    STORE32 COPROC_CMD, COPROC_CMD_ENQUEUE

    ; Save the returned ticket number into zero page
    ; (tickets are small integers, so only byte 0 matters on the 6502)
    LDA COPROC_TICKET
    STA $00                            ; ZP $00 = ticket

; ============================================================================
; PHASE 3 - POLL UNTIL COMPLETE
; ============================================================================
;
; WHY: The worker processes the request asynchronously. We must poll the
; ticket status until it transitions from PENDING (0) or RUNNING (1) to
; a terminal state. COPROC_ST_OK (2) means the response buffer is valid.
;
; To poll, we write the ticket number back into the COPROC_TICKET register
; (all 4 bytes, clearing the upper 3 since the 6502 wrote only byte 0),
; then issue COPROC_CMD_POLL. The result appears in COPROC_TICKET_STATUS.

poll_loop:
    ; Restore the full 32-bit ticket value (byte 0 from ZP, bytes 1-3 = 0)
    LDA $00
    STA COPROC_TICKET
    LDA #0
    STA COPROC_TICKET+1
    STA COPROC_TICKET+2
    STA COPROC_TICKET+3

    ; Issue the POLL command
    STORE32 COPROC_CMD, COPROC_CMD_POLL

    ; Check the ticket status
    LDA COPROC_TICKET_STATUS
    CMP #COPROC_ST_OK                  ; 2 = completed successfully
    BEQ done
    JMP poll_loop

; ============================================================================
; PHASE 4 - READ RESULT AND SIGNAL COMPLETION
; ============================================================================
;
; WHY: The result now sits at bus $410100. On the 6502, reading 32-bit
; bus memory requires a bank window -- but for this demo we simply write
; a completion flag to $0200 so the test harness can verify success.

done:
    LDA #$FF
    STA $0200                          ; signal completion to test harness
    JMP *                              ; halt: spin forever

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; WHY: If the START command failed, COPROC_CMD_ERROR contains a diagnostic
; error code. We store it at $0201 for the test harness to inspect.

error:
    LDA COPROC_CMD_ERROR
    STA $0201                          ; store error code for inspection
    JMP *                              ; halt: spin forever
