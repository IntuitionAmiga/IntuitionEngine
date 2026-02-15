; ============================================================================
; COPROCESSOR CALLER - INTER-CPU MAILBOX IPC REFERENCE
; IE32 Assembly for IntuitionEngine - Multi-CPU Ring Buffer Communication
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC) - caller side
; Video Chip:    None (terminal output only)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         sdk/bin/ie32asm sdk/examples/asm/coproc_caller_ie32.asm
;                sdk/bin/ie32asm sdk/examples/asm/coproc_service_ie32.asm
; Run:           ./bin/IntuitionEngine -ie32 coproc_caller_ie32.iex
;                    -coproc coproc_service_ie32.iex
; Porting:       Coprocessor MMIO is CPU-agnostic. Caller/service pairs can
;                mix CPU cores (e.g. M68K caller + Z80 service). See
;                coproc_caller_68k.asm, coproc_caller_z80.asm,
;                coproc_caller_65.asm, coproc_caller_x86.asm for ports.
;
; REFERENCE IMPLEMENTATION FOR THE INTUITION ENGINE SDK
; This file is heavily commented to teach inter-processor communication
; concepts on the IE32 custom CPU, with particular attention to the
; mailbox ring buffer protocol.
;
; === WHAT THIS DEMO DOES ===
; 1. Starts a coprocessor worker (another IE32 CPU running the service binary)
; 2. Writes request data to shared memory (two operands: 10 and 20)
; 3. Enqueues an "add" operation via the coprocessor mailbox registers
; 4. Receives a ticket number for tracking the request
; 5. Polls the ticket status until the coprocessor signals completion
; 6. Reads the result (30) from the shared response buffer
;
; === WHY COPROCESSOR MAILBOX IPC ===
; Multi-processor communication is one of the fundamental challenges in
; computer architecture. When two CPUs share memory but run independently,
; they need a protocol to exchange work requests and results without
; corruption or deadlock.
;
; The IntuitionEngine's coprocessor subsystem uses a mailbox protocol
; inspired by real-world multi-processor designs:
;
;   - Amiga (1985): The 68000 communicated with the copper and blitter
;     via memory-mapped command registers. The CPU wrote parameters to
;     specific addresses and the coprocessors acted on them autonomously.
;
;   - SNES SA-1 (1995): The SA-1 coprocessor shared ROM and RAM with
;     the main 65816, communicating through hardware mailbox registers
;     for synchronisation.
;
;   - Modern GPUs: The CPU submits command buffers to a ring buffer;
;     the GPU processes them asynchronously and signals completion via
;     fence values - essentially the same ticket-polling pattern used here.
;
; The protocol follows a simple sequence:
;   1. CALLER writes parameters to shared memory
;   2. CALLER writes ENQUEUE command to mailbox registers
;   3. CALLER receives a ticket (monotonic counter)
;   4. SERVICE detects the new entry, processes it, writes result
;   5. CALLER polls ticket status until it reads COMPLETE
;   6. CALLER reads result from shared memory
;
; This lock-free design means the caller and service never need to
; agree on timing - the mailbox registers handle synchronisation.
;
; === IE32-SPECIFIC NOTES ===
; The IE32 uses memory-mapped I/O with the @ prefix for absolute
; addresses. However, the coprocessor registers are defined as
; constants in ie32.inc, so we use STORE/LOAD with those symbols
; directly. The IE32's register-rich design (20 GPRs) means we can
; keep the ticket in register X across the poll loop without needing
; stack saves.
;
; === MEMORY MAP ===
;
;   Address      Size      Purpose
;   ---------    --------  ------------------------------------------
;   0x001000     ~256B     Program code (this file)
;   0x400000     256B      Service filename string (pre-loaded by host)
;   0x410000     8B        Request data (two uint32: val1, val2)
;   0x410100     4B        Response buffer (uint32 result)
;   0xF00300+    (regs)    Coprocessor MMIO registers (ie32.inc)
;
; === BUILD AND RUN ===
;
; Assemble both caller and service:
;   sdk/bin/ie32asm sdk/examples/asm/coproc_caller_ie32.asm
;   sdk/bin/ie32asm sdk/examples/asm/coproc_service_ie32.asm
;
; Run:
;   ./bin/IntuitionEngine -ie32 coproc_caller_ie32.iex \
;       -coproc coproc_service_ie32.iex
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .include "ie32.inc"

    .org 0x1000

; ============================================================================
; SHARED MEMORY LAYOUT
; ============================================================================
;
; The caller and service communicate through three shared memory regions:
;
;   0x400000: Service filename string (host pre-loads the binary path here)
;   0x410000: Request data  - two uint32 values (val1 at +0, val2 at +4)
;   0x410100: Response buffer - one uint32 result
;
; These addresses are arbitrary but must match between caller and service.
; They sit above VRAM (0x100000-0x22C000) to avoid any overlap.

; ============================================================================
; PHASE 1 - START THE COPROCESSOR WORKER
; ============================================================================
;
; Before we can send work, we must launch the service binary on a separate
; CPU core. We write the CPU type and binary path to the coprocessor control
; registers, then issue the START command. The host loads the service binary
; and begins executing it on a new IE32 core.

    LOAD    A, #COPROC_CPU_IE32
    STORE   A, COPROC_CPU_TYPE
    LOAD    A, #0x400000
    STORE   A, COPROC_NAME_PTR
    LOAD    A, #COPROC_CMD_START
    STORE   A, COPROC_CMD

    ; Check that the start succeeded - status 0 means OK
    LOAD    A, COPROC_CMD_STATUS
    JNZ     A, error                   ; if status != 0, start failed

; ============================================================================
; PHASE 2 - WRITE REQUEST DATA TO SHARED MEMORY
; ============================================================================
;
; We want the service to compute 10 + 20 = 30. The request format is
; simply two consecutive uint32 values at the request address. The service
; will read both values, add them, and write the sum to the response buffer.

    LOAD    A, #10
    STORE   A, 0x410000                ; val1 = 10
    LOAD    A, #20
    STORE   A, 0x410004                ; val2 = 20

; ============================================================================
; PHASE 3 - ENQUEUE THE REQUEST VIA MAILBOX REGISTERS
; ============================================================================
;
; The mailbox protocol requires us to set several parameters before
; issuing the ENQUEUE command:
;   - CPU type:     which coprocessor to target
;   - Operation:    what to do (1 = add)
;   - Request ptr:  where to find the input data
;   - Request len:  how many bytes of input (8 = two uint32s)
;   - Response ptr: where to write the output
;   - Response cap: how many bytes of output space (4 = one uint32)
;
; After ENQUEUE, the COPROC_TICKET register contains a unique ticket
; number that we use to track this specific request.

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

    ; Save the ticket - we need this to poll for completion
    LOAD    X, COPROC_TICKET           ; X = ticket number

; ============================================================================
; PHASE 4 - POLL FOR COMPLETION
; ============================================================================
;
; The caller has no way to know when the service will finish processing.
; We poll by writing our ticket to COPROC_TICKET, issuing a POLL command,
; and reading COPROC_TICKET_STATUS. When the status equals COPROC_ST_OK (2),
; the result is ready in the response buffer.
;
; This is analogous to GPU fence polling: submit work, get a fence value,
; spin until the fence signals completion. In production code you might
; yield or do other work between polls, but for this demo we busy-wait.

poll_loop:
    STORE   X, COPROC_TICKET
    LOAD    A, #COPROC_CMD_POLL
    STORE   A, COPROC_CMD
    LOAD    A, COPROC_TICKET_STATUS
    SUB     A, #COPROC_ST_OK           ; compare with OK (2)
    JZ      A, done
    JMP     poll_loop

; ============================================================================
; PHASE 5 - READ THE RESULT
; ============================================================================
;
; The service has written the sum (10 + 20 = 30) to address 0x410100.
; We load it into register A and halt. In a real application, this result
; would feed into further computation or be displayed to the user.

done:
    LOAD    A, 0x410100                ; A = result (should be 30)
    HALT

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; If the coprocessor START command fails (e.g. binary not found, CPU type
; unsupported), we read the error code and halt. The error code can be
; inspected in a debugger to diagnose the failure.

error:
    LOAD    A, COPROC_CMD_ERROR
    HALT
