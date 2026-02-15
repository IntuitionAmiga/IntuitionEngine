; ============================================================================
; COPROCESSOR CALLER - x86 HOST LAUNCHING AN IE32 WORKER
; NASM (ie32asm) Assembly for IntuitionEngine - Headless IPC Demo
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    x86 (32-bit protected mode, caller/host side)
; Video Chip:    None (headless)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler, NASM-compatible syntax)
; Build:         sdk/bin/ie32asm sdk/examples/asm/coproc_caller_x86.asm
; Run:           ./bin/IntuitionEngine -x86 coproc_caller_x86.ie86 -coproc coproc_service_x86.ie86
; Porting:       Mailbox protocol is CPU-agnostic. See coproc_caller_65.asm
;                (6502), coproc_caller_68k.asm (M68K), coproc_caller_z80.asm
;                (Z80) for the same demo on other CPU cores.
;
; === WHAT THIS DEMO DOES ===
; 1. Launches an IE32 coprocessor worker from a service binary
; 2. Writes two operands (10 and 20) into the request buffer
; 3. Enqueues an "add" request (operation code 1)
; 4. Polls the ticket status register until the worker completes
; 5. Reads the result (expected: 30) into EAX
;
; === WHY COPROCESSOR MAILBOX IPC ===
; The Intuition Engine supports up to six heterogeneous CPU cores running
; concurrently: IE32, IE64, 6502, Z80, M68K, and x86. These cores need
; a structured way to delegate work to one another -- for example, an x86
; host offloading computation to an IE32 worker.
;
; The coprocessor subsystem provides this via a memory-mapped mailbox
; protocol. The caller writes commands to MMIO registers in the coprocessor
; region (0xF2340-0xF237F on the x86 flat address space). Each command is
; atomic: writing to COPROC_CMD triggers the operation using whatever
; values are currently in the other registers.
;
; This design follows a long tradition of hardware mailbox systems:
; the Amiga's copper command lists, DSP mailboxes in game consoles like
; the Super Nintendo and Sega Saturn, and modern GPU command queues all
; use the same fundamental pattern -- a producer writes descriptors into
; shared memory, a consumer processes them asynchronously, and status
; registers provide synchronisation. The ticket-based polling model here
; is deliberately simple, avoiding interrupt-driven complexity.
;
; === x86-SPECIFIC NOTES ===
; Like the M68K, the x86 operates in a flat 32-bit address space with no
; bank-switching or gateway windows. All bus addresses (coprocessor MMIO,
; request/response buffers) are directly accessible via standard mov
; instructions with memory operands.
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
;   ./bin/IntuitionEngine -x86 coproc_caller_x86.ie86 -coproc coproc_service_x86.ie86
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

%include "ie86.inc"

    bits 32
    org 0x1000

; ============================================================================
; PHASE 1 - START THE IE32 WORKER
; ============================================================================
;
; WHY: The coproc_start macro writes COPROC_CPU_TYPE and COPROC_NAME_PTR,
; then triggers COPROC_CMD_START. On the x86, these are direct 32-bit
; memory writes via `mov dword [addr], imm32`. We check CMD_STATUS
; afterwards: zero means success, non-zero means the worker failed to
; launch (e.g., binary not found).

    coproc_start COPROC_CPU_IE32, 0x400000

    ; Verify the start command succeeded
    mov eax, [COPROC_CMD_STATUS]
    test eax, eax
    jnz error

; ============================================================================
; PHASE 2 - PREPARE REQUEST DATA AND ENQUEUE
; ============================================================================
;
; WHY: The x86 can write operands directly into bus memory. We place 10
; at 0x410000 and 20 at 0x410004, forming an 8-byte request payload
; (two uint32 values). The coproc_enqueue macro fills in all descriptor
; fields and triggers ENQUEUE. The controller allocates a ticket and
; writes it to the COPROC_TICKET register.

    ; Write the two operands into the request buffer
    mov dword [0x410000], 10           ; operand 1
    mov dword [0x410004], 20           ; operand 2

    ; Enqueue: CPU=IE32, op=1(add), req=0x410000(8 bytes), resp=0x410100(4 bytes)
    coproc_enqueue COPROC_CPU_IE32, 1, 0x410000, 8, 0x410100, 4

    ; Save the ticket for polling
    mov ebx, [COPROC_TICKET]           ; EBX = ticket (preserved across loop)

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
    mov dword [COPROC_TICKET], ebx
    mov dword [COPROC_CMD], COPROC_CMD_POLL
    mov eax, [COPROC_TICKET_STATUS]
    cmp eax, COPROC_ST_OK
    je done
    jmp poll_loop

; ============================================================================
; PHASE 4 - READ RESULT
; ============================================================================
;
; WHY: The worker has written the sum into the response buffer at 0x410100.
; On the x86 we can read it directly -- no bank window needed. The
; expected result is 30 (10 + 20). We load it into EAX and halt.

done:
    mov eax, [0x410100]                ; EAX = result (should be 30)
    hlt

; ============================================================================
; ERROR HANDLER
; ============================================================================
;
; WHY: If START failed, COPROC_CMD_ERROR contains a diagnostic code.
; We load it into EAX for inspection and halt.

error:
    mov eax, [COPROC_CMD_ERROR]
    hlt
