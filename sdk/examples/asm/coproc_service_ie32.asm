; ============================================================================
; COPROCESSOR SERVICE - IE32 WORKER SIDE OF INTER-CPU COMMUNICATION
; IE32 Assembly for IntuitionEngine - Headless coprocessor service
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    IE32 (custom 32-bit RISC, 7 general-purpose registers)
; Video Chip:    None (headless coprocessor service)
; Audio Engine:  None
; Assembler:     ie32asm (built-in IE32 assembler)
; Build:         sdk/bin/ie32asm sdk/examples/asm/coproc_service_ie32.asm
; Run:           Used as -coproc argument with a matching caller binary
; Porting:       Mailbox protocol is CPU-agnostic. Same service implemented
;                on all CPU cores. See coproc_service_65.asm,
;                coproc_service_68k.asm, coproc_service_x86.asm,
;                coproc_service_z80.asm.
;
; REFERENCE IMPLEMENTATION FOR THE COPROCESSOR SERVICE PROTOCOL
; This IE32 version is the canonical implementation of the service side.
; All other CPU ports implement the same algorithm, adapted to their
; native instruction sets and addressing modes.
;
; === WHAT THIS DEMO DOES ===
; 1. Polls the shared-memory ring buffer for incoming request descriptors
; 2. Dispatches on the op field (currently supports op=1: 32-bit add)
; 3. Reads two uint32 operands from the request data pointer
; 4. Performs a native 32-bit ADD instruction
; 5. Writes the 32-bit result to the response data pointer
; 6. Fills in the response descriptor (ticket echo, status, result length)
; 7. Advances the ring tail pointer and loops back to poll
;
; === WHY COPROCESSOR MAILBOX IPC ===
; This is the SERVICE side of the coprocessor mailbox protocol. The service
; runs on a separate CPU core -- in this case the IE32 custom RISC -- and
; polls shared memory for incoming commands from a caller running on a
; different CPU. When a new request appears (head != tail), the service
; processes it and writes the result back, then advances the tail to
; signal completion.
;
; The ring buffer acts as a hardware-neutral IPC channel: any CPU can be
; the caller and any CPU can be the service, because they communicate
; solely through memory-mapped addresses. This mirrors real coprocessor
; architectures throughout computing history:
;
;   - Amiga: The 68000 writes command lists for the copper and blitter
;     coprocessors, which execute them independently from shared chip RAM
;   - SNES: The SA-1 coprocessor (a second 65C816) runs its own program
;     from shared ROM, communicating via hardware registers
;   - Modern GPUs: The CPU submits command buffers to a ring; the GPU
;     processes them asynchronously and signals completion
;
; The key insight across all these architectures is the same: decouple
; the request submission (caller) from the request processing (service)
; through a shared data structure, allowing both sides to run at their
; own pace on independent processor cores.
;
; === IE32-SPECIFIC NOTES ===
; The IE32 is a load-store RISC architecture -- all memory access goes
; through explicit load/store instructions. There are no memory-operand
; ALU instructions like the x86's "add eax, [mem]". This means every
; field read from the ring buffer requires a load into a register before
; it can be used. The [register] syntax provides register-indirect
; addressing for accessing data through computed pointers.
;
; The IE32 has 7 general-purpose registers named A-D and X-Z (r0-r6).
; With careful allocation, we can hold all the key values (tail, entry
; pointer, response pointer, ticket) in registers throughout the
; request processing, minimising redundant loads.
;
; GOTCHA: The IE32's register-indirect encoding (ADDR_REG_IND) packs
; the offset in bits[31:4] and register index in bits[3:0]. This means
; offsets smaller than 16 cannot be encoded directly -- we must add the
; offset to the base register manually before using [reg] addressing.
; This is why the code uses "move Z, B / add Z, #8 / load C, [Z]"
; instead of a hypothetical "load C, [B+8]".
;
; === MEMORY MAP ===
; $200000          Code entry point (WORKER_IE32_BASE)
; $820000          Mailbox base (MAILBOX_BASE)
; $820000          Ring 0 head pointer (IE32 is ring index 0)
; $820001          Ring 0 tail pointer
; $820008+tail*32  Request entry descriptors (32 bytes each)
; $820208+tail*16  Response descriptors (16 bytes each)
;
; === BUILD AND RUN ===
; sdk/bin/ie32asm sdk/examples/asm/coproc_service_ie32.asm
; (loaded by a caller binary via COPROC_CMD_START)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .org 0x200000

; ============================================================================
; CONSTANTS - Ring Buffer Addresses
; ============================================================================
;
; The IE32 is assigned ring index 0, so its ring lives directly at the
; mailbox base ($820000). Each ring has 16 entry slots (32 bytes each)
; starting at offset +8, and 16 response slots (16 bytes each) starting
; at offset +$208.

.equ RING_HEAD   0x820000
.equ RING_TAIL   0x820001
.equ ENTRIES     0x820008
.equ RESPONSES   0x820208

; ============================================================================
; REGISTER ALLOCATION
; ============================================================================
;
; WHY: On a register-starved RISC like the IE32 (only 7 GPRs), careful
; allocation avoids unnecessary spills to memory. This mapping holds
; throughout the main processing path:
;
;   A (r0) = scratch / computation result
;   X (r1) = tail (preserved across the entire request)
;   Y (r2) = head / scratch
;   Z (r3) = scratch for pointer arithmetic
;   B (r4) = entry base address
;   C (r5) = scratch
;   D (r6) = response data pointer (respPtr)

; ============================================================================
; MAIN POLL LOOP - Wait for Requests
; ============================================================================
;
; WHY: The service spins on head != tail. When the caller enqueues a
; request, it advances head. We detect this by comparing our cached tail
; against the current head value. Both are single bytes masked to 8 bits,
; so reads are effectively atomic.

main_loop:
    ; Read tail
    LDA @RING_TAIL                  ; A = mem[tail_addr]
    AND A, #0xFF                    ; mask to byte
    LDX A                           ; X = tail

    ; Read head
    LDA @RING_HEAD                  ; A = mem[head_addr]
    AND A, #0xFF                    ; mask to byte

    ; Compare: if tail == head, ring empty
    SUB A, X                        ; A = head - tail
    JZ  A, main_loop                ; if equal, poll again

; ============================================================================
; ENTRY ADDRESS COMPUTATION - Locate the Request Descriptor
; ============================================================================
;
; WHY: Each entry is 32 bytes, so entry address = ENTRIES + tail * 32.
; SHL #5 computes the multiply. The result is held in B (r4) for the
; duration of request processing, since we need to read multiple fields
; from the entry at different offsets.

    ; Compute entry address: ENTRIES + tail * 32
    LDB X                           ; B = tail
    SHL B, #5                       ; B = tail * 32
    ADD B, #ENTRIES                  ; B = entry base

; ============================================================================
; OPCODE DISPATCH - Check Which Operation Was Requested
; ============================================================================
;
; WHY: The op field at entry+8 tells us what computation to perform.
; On the IE32 (load-store architecture), we must compute the field
; address in a scratch register, then load through it with [Z] syntax.
; Currently only op=1 (add two uint32 values) is supported.
;
; GOTCHA: We cannot use register-indirect with offset 8 directly because
; the IE32 reg_ind encoding requires offset >= 16 in the high bits.
; Instead we copy B to Z, add the offset manually, then load via [Z].

    ; Read op: entry + 8
    LDZ B                           ; Z = entry addr
    ADD Z, #8                       ; Z = entry + 8 (op offset)
    LDC [Z]                         ; C = op

    ; Check op == 1 (add)
    LDA C
    SUB A, #1
    JNZ A, write_error              ; unsupported op

; ============================================================================
; EXTRACT REQUEST FIELDS - Read Pointers from the Entry Descriptor
; ============================================================================
;
; WHY: reqPtr (entry+16) points to the input operands. respPtr (entry+24)
; points to where we write the result. ticket (entry+0) is an opaque
; value echoed back in the response for caller correlation.
;
; Each field requires a fresh pointer computation because the IE32 has
; no displacement addressing mode -- we must manually ADD the offset
; to the base address each time.

    ; Read reqPtr: entry + 16
    LDZ B
    ADD Z, #16
    LDC [Z]                         ; C = reqPtr

    ; Read respPtr: entry + 24
    LDZ B
    ADD Z, #24
    LDD [Z]                         ; D = respPtr

    ; Read ticket: entry + 0
    LDZ [B]                         ; Z = ticket

; ============================================================================
; OP=1: 32-BIT ADDITION - Native Word-Size Operation
; ============================================================================
;
; WHY: The IE32 is a 32-bit CPU, so the addition is a single ADD
; instruction. We load val1 from [reqPtr], val2 from [reqPtr+4],
; add them, and store the result to [respPtr]. The register-indirect
; loads ([C] and [Y]) dereference the pointers we extracted above.

    ; Op=1: add two uint32 from reqPtr
    ; val1 = mem[reqPtr], val2 = mem[reqPtr+4]
    LDA [C]                         ; A = val1
    LDY C
    ADD Y, #4
    LDY [Y]                         ; Y = val2
    ADD A, Y                        ; A = val1 + val2

    ; Write result to respPtr
    STA [D]

; ============================================================================
; WRITE RESPONSE DESCRIPTOR - Signal Completion to the Caller
; ============================================================================
;
; WHY: The response descriptor tells the caller what happened. It lives
; in a separate array from the request entries (RESPONSES + tail*16).
; We fill in four fields, each a uint32:
;   +0  ticket    -- echoed from the request for correlation
;   +4  status    -- 2 = success, 3 = error
;   +8  resultCode -- 0 for success
;   +12 respLen   -- number of valid bytes in the response data buffer
;
; On the IE32, each field write requires computing the target address
; by incrementing Y (our running pointer into the response descriptor).

    ; Write response descriptor: RESPONSES + tail * 16
    LDA X                           ; A = tail
    SHL A, #4                       ; A = tail * 16
    ADD A, #RESPONSES                ; A = response addr

    ; response.ticket = Z (ticket)
    STZ [A]

    ; response.status = 2 (ok)
    LDY A
    ADD Y, #4
    LDC #2
    STC [Y]

    ; response.resultCode = 0
    ADD Y, #4
    LDC #0
    STC [Y]

    ; response.respLen = 4
    ADD Y, #4
    LDC #4
    STC [Y]

; ============================================================================
; ADVANCE TAIL - Mark This Slot as Consumed
; ============================================================================
;
; WHY: Incrementing tail modulo 16 (AND #0x0F) frees this ring slot
; for reuse by the caller. The 16-entry ring wraps naturally with a
; 4-bit mask.

    ; Advance tail: (tail + 1) & 15
    LDA X
    ADD A, #1
    AND A, #0x0F
    STA @RING_TAIL

    JMP main_loop

; ============================================================================
; ERROR RESPONSE - Unknown Opcode Handler
; ============================================================================
;
; WHY: If the caller sends an opcode we do not recognise, we must still
; advance the tail (otherwise the ring would stall forever). We write
; status=3 (error) and resultCode=1 (unknown op) so the caller knows
; the request was rejected rather than silently lost.

write_error:
    ; Unsupported op -- write error response
    LDA X                           ; A = tail
    SHL A, #4
    ADD A, #RESPONSES

    ; ticket
    LDZ [B]
    STZ [A]

    ; status = 3 (error)
    LDY A
    ADD Y, #4
    LDC #3
    STC [Y]

    ; resultCode = 1 (unknown op)
    ADD Y, #4
    LDC #1
    STC [Y]

    ; respLen = 0
    ADD Y, #4
    LDC #0
    STC [Y]

    ; Advance tail
    LDA X
    ADD A, #1
    AND A, #0x0F
    STA @RING_TAIL

    JMP main_loop
