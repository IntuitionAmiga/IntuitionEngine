; ============================================================================
; COPROCESSOR CALLER - Z80 HOST LAUNCHING AN M68K WORKER
; VASM Z80 Assembly for IntuitionEngine - Headless IPC Demo
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    Zilog Z80 (caller/host side)
; Video Chip:    None (headless)
; Audio Engine:  None
; Assembler:     vasmz80_std (VASM Z80, standard syntax)
; Build:         vasmz80_std -Fbin -o coproc_caller_z80.ie80 coproc_caller_z80.asm
; Run:           ./bin/IntuitionEngine -z80 coproc_caller_z80.ie80 -coproc coproc_service_z80.ie80
; Porting:       Mailbox protocol is CPU-agnostic. See coproc_caller_65.asm
;                (6502), coproc_caller_68k.asm (M68K), coproc_caller_x86.asm
;                (x86) for the same demo on other CPU cores.
;
; === WHAT THIS DEMO DOES ===
; 1. Launches an M68K coprocessor worker from a service binary
; 2. Enqueues an "add" request (operation code 1) with two operands
; 3. Polls the ticket status register until the worker completes
; 4. Halts upon completion (result available in bus memory)
;
; === WHY COPROCESSOR MAILBOX IPC ===
; The Intuition Engine supports up to six heterogeneous CPU cores running
; concurrently: IE32, IE64, 6502, Z80, M68K, and x86. These cores need
; a structured way to delegate work to one another -- for example, a Z80
; host offloading heavy computation to a 32-bit M68K worker.
;
; The coprocessor subsystem provides this via a memory-mapped mailbox
; protocol. The caller writes commands to a set of MMIO registers (the
; "gateway window" at 0xF200-0xF23F in Z80 address space), which the bus
; maps to the coprocessor controller at bus addresses 0xF2340-0xF237F.
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
; === Z80-SPECIFIC NOTES ===
; The Z80, like the 6502, has a 16-bit address space (0x0000-0xFFFF).
; The coprocessor registers and data buffers live at bus addresses above
; 0xFFFF (0x400000, 0x410000, etc.). The gateway window at 0xF200 solves
; register access, but data buffers must be pre-loaded into bus memory
; by the host environment before execution.
;
; All coprocessor registers are 32-bit, but the Z80 can only write one
; byte at a time. The STORE32 macro in ie80.inc handles this by writing
; four consecutive bytes in little-endian order. Reading a 32-bit ticket
; similarly requires four byte reads -- but since ticket numbers are small
; integers, only byte 0 is meaningful in practice.
;
; Note that unlike the 6502, the Z80 uses the `cp` instruction for
; comparisons (rather than CMP), and `jr` for short relative branches
; (rather than BEQ/BNE). The `or a` idiom tests whether A is zero
; (equivalent to `tst` on M68K or `test` on x86).
;
; === MEMORY MAP ===
;
;   Address         Size    Purpose
;   ------------    ------  -------------------------------------------
;   0x0000          ~128 B  Program code (origin)
;   0xF200-0xF23F   64 B   Coprocessor gateway window (MMIO registers)
;   0x400000        varies  Service filename string (bus memory, pre-loaded)
;   0x410000        8 B     Request data buffer (two uint32 operands)
;   0x410100        4 B     Response data buffer (one uint32 result)
;
; === BUILD AND RUN ===
;   vasmz80_std -Fbin -o coproc_caller_z80.ie80 coproc_caller_z80.asm
;   ./bin/IntuitionEngine -z80 coproc_caller_z80.ie80 -coproc coproc_service_z80.ie80
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .include "ie80.inc"

    .org 0x0000

; ============================================================================
; GATEWAY WINDOW OVERVIEW
; ============================================================================
; The gateway at 0xF200-0xF23F maps to bus 0xF2340-0xF237F.
; All COPROC registers are accessed via byte-level writes using STORE32.
; Data buffers use bank windows to access bus memory above 0xFFFF.

; ============================================================================
; PHASE 1 - START THE M68K WORKER
; ============================================================================
;
; WHY: Before we can enqueue any work, the coprocessor controller needs
; to know which CPU type to spawn and where to find the service binary.
; COPROC_CPU_TYPE selects the worker architecture (M68K in this case);
; COPROC_NAME_PTR points to a null-terminated filename string in bus
; memory. Writing COPROC_CMD with COPROC_CMD_START triggers the launch.
; We then check CMD_STATUS to confirm the worker started without error.
;
; Note: this Z80 caller launches an M68K worker (not IE32), demonstrating
; that any CPU can launch any other CPU type as a coprocessor.

    ; Select M68K as the worker CPU type
    STORE32 COPROC_CPU_TYPE COPROC_CPU_M68K

    ; Point to the service binary filename in bus memory
    ; (must be pre-loaded into bus memory by the host before execution)
    STORE32 COPROC_NAME_PTR 0x400000

    ; Trigger the START command
    STORE32 COPROC_CMD COPROC_CMD_START

    ; Check whether the start succeeded (0 = OK, non-zero = error)
    ; Z80 idiom: `or a` sets the zero flag if A == 0
    ld a,(COPROC_CMD_STATUS)
    or a
    jp nz,error

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
; The request buffer at 0x410000 contains two uint32 values (the operands).
; The response buffer at 0x410100 will receive one uint32 (the sum).
; These buffers must be pre-populated by the host environment.

    STORE32 COPROC_CPU_TYPE COPROC_CPU_M68K
    STORE32 COPROC_OP 1                ; op = add
    STORE32 COPROC_REQ_PTR 0x410000    ; request data at bus 0x410000
    STORE32 COPROC_REQ_LEN 8           ; two uint32 = 8 bytes
    STORE32 COPROC_RESP_PTR 0x410100   ; response buffer
    STORE32 COPROC_RESP_CAP 4          ; capacity: 4 bytes (one uint32)
    STORE32 COPROC_CMD COPROC_CMD_ENQUEUE

    ; Save the returned ticket number into register B
    ; (tickets are small integers, so only byte 0 matters on the Z80)
    ld a,(COPROC_TICKET)
    ld b,a                             ; B = ticket

; ============================================================================
; PHASE 3 - POLL UNTIL COMPLETE
; ============================================================================
;
; WHY: The worker processes the request asynchronously. We must poll the
; ticket status until it transitions from PENDING (0) or RUNNING (1) to
; a terminal state. COPROC_ST_OK (2) means the response buffer is valid.
;
; To poll, we write the ticket number back into the COPROC_TICKET register
; (all 4 bytes, clearing the upper 3 since the Z80 stored only byte 0),
; then issue COPROC_CMD_POLL. The result appears in COPROC_TICKET_STATUS.

poll_loop:
    ; Restore the full 32-bit ticket value (byte 0 from B, bytes 1-3 = 0)
    ld a,b
    ld (COPROC_TICKET),a
    ld a,0
    ld (COPROC_TICKET+1),a
    ld (COPROC_TICKET+2),a
    ld (COPROC_TICKET+3),a

    ; Issue the POLL command
    STORE32 COPROC_CMD COPROC_CMD_POLL

    ; Check the ticket status
    ld a,(COPROC_TICKET_STATUS)
    cp COPROC_ST_OK                    ; 2 = completed successfully
    jr z,done
    jr poll_loop

; ============================================================================
; PHASE 4 - COMPLETION
; ============================================================================
;
; WHY: The result now sits at bus 0x410100. On the Z80, reading 32-bit
; bus memory requires a bank window -- but for this demo we simply halt,
; and the test harness verifies the response buffer contents directly.

done:
    halt

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; WHY: If the START command failed, COPROC_CMD_ERROR contains a diagnostic
; error code. We load it into A for inspection and halt.

error:
    ld a,(COPROC_CMD_ERROR)
    halt
