; ============================================================================
; COPROCESSOR SERVICE - x86 WORKER SIDE OF INTER-CPU COMMUNICATION
; NASM-style x86-32 syntax for IntuitionEngine - Headless coprocessor service
; ============================================================================
;
; === SDK QUICK REFERENCE ===
; Target CPU:    x86-32 (Intel 386+ compatible, CISC with memory operands)
; Video Chip:    None (headless coprocessor service)
; Audio Engine:  None
; Assembler:     ie32asm (built-in assembler, NASM-compatible x86 mode)
; Build:         sdk/bin/ie32asm sdk/examples/asm/coproc_service_x86.asm
; Run:           Used as -coproc argument with a matching caller binary
; Porting:       Same service implemented on all CPU cores. See
;                coproc_service_ie32.asm (reference), coproc_service_65.asm,
;                coproc_service_68k.asm, coproc_service_z80.asm.
;
; === WHAT THIS DEMO DOES ===
; 1. Polls the shared-memory ring buffer for incoming request descriptors
; 2. Dispatches on the op field (currently supports op=1: 32-bit add)
; 3. Reads two uint32 operands from the request data pointer
; 4. Performs a native 32-bit ADD with a memory source operand
; 5. Writes the 32-bit result to the response data pointer
; 6. Fills in the response descriptor (ticket echo, status, result length)
; 7. Advances the ring tail pointer and loops back to poll
;
; === WHY COPROCESSOR MAILBOX IPC ===
; This is the SERVICE side of the coprocessor mailbox protocol. The service
; runs on a separate CPU core -- in this case an x86-32 -- and polls shared
; memory for incoming commands from a caller running on a different CPU.
; When a new request appears (head != tail), the service processes it and
; writes the result back, then advances the tail to signal completion.
;
; The ring buffer acts as a hardware-neutral IPC channel: any CPU can be
; the caller and any CPU can be the service, because they communicate
; solely through memory-mapped addresses. This mirrors real coprocessor
; architectures throughout computing history:
;
;   - Amiga: The 68000 writes command lists for the copper and blitter
;     coprocessors, which execute them independently from shared chip RAM
;   - SNES: The SA-1 coprocessor runs its own program from shared ROM,
;     communicating via hardware registers
;   - Modern GPUs: The CPU submits command buffers to a ring; the GPU
;     processes them asynchronously and signals completion
;
; === x86-SPECIFIC NOTES ===
; The x86 is a CISC architecture with memory-operand instructions, so
; "add eax, [edi+4]" can load from memory and add in a single opcode.
; This makes the x86 version particularly compact -- there is no need
; for separate LOAD instructions before every ALU operation as on the
; IE32 (load-store RISC) or the indirect-indexed gymnastics of the 6502.
;
; The MOVZX instruction zero-extends a byte to a 32-bit register, which
; is exactly what we need for reading the 8-bit head and tail pointers
; into 32-bit registers for comparison and arithmetic.
;
; EBX is used to preserve the tail value across the processing path,
; since EAX is frequently clobbered by address computations. ESI holds
; the entry pointer and EBP holds the response data pointer (respPtr).
;
; === MEMORY MAP ===
; $320000          Code entry point (WORKER_X86_BASE)
; $820000          Mailbox base (MAILBOX_BASE)
; $820C00          Ring 4 head pointer (x86 is ring index 4)
; $820C01          Ring 4 tail pointer
; $820C08+tail*32  Request entry descriptors (32 bytes each)
; $820E08+tail*16  Response descriptors (16 bytes each)
;
; === BUILD AND RUN ===
; sdk/bin/ie32asm sdk/examples/asm/coproc_service_x86.asm
; (loaded by a caller binary via COPROC_CMD_START)
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

%include "ie86.inc"

    bits 32
    org 0x320000

; ============================================================================
; CONSTANTS - Ring Buffer Addresses
; ============================================================================
;
; The x86 is assigned ring index 4, so its ring lives at mailbox base
; + 4 * $300 = $820C00. Each ring has 16 entry slots (32 bytes each)
; starting at offset +8, and 16 response slots (16 bytes each) starting
; at offset +$208.

RING_BASE   equ 0x820C00
ENTRIES     equ RING_BASE + 0x08
RESPONSES   equ RING_BASE + 0x208

; ============================================================================
; MAIN POLL LOOP - Wait for Requests
; ============================================================================
;
; WHY: The service spins on head != tail. The caller advances head when
; it enqueues a request; we advance tail when we finish processing one.
; MOVZX zero-extends the 8-bit head/tail bytes into 32-bit registers
; so the CMP instruction compares correctly.
;
; Register usage in the main loop:
;   EAX = scratch (tail initially, then address computation)
;   EBX = tail (preserved across the entire request)
;   ECX = head / ticket
;   EDX = op field
;   ESI = entry descriptor pointer
;   EDI = request data pointer (reqPtr)
;   EBP = response data pointer (respPtr)

main_loop:
    ; Read tail (byte)
    movzx   eax, byte [RING_BASE+1]     ; EAX = tail

    ; Read head (byte)
    movzx   ecx, byte [RING_BASE]       ; ECX = head

    ; Compare: if tail == head, ring empty
    cmp     eax, ecx
    je      main_loop

; ============================================================================
; ENTRY ADDRESS COMPUTATION - Locate the Request Descriptor
; ============================================================================
;
; WHY: Each entry is 32 bytes, so entry address = ENTRIES + tail * 32.
; The x86 barrel shifter computes tail*32 in a single SHL instruction.
; We save tail in EBX before clobbering EAX with the address computation,
; since we need the original tail value later for the response descriptor
; and tail advancement.

    ; Save tail in EBX
    mov     ebx, eax

    ; Compute entry address: ENTRIES + tail*32
    shl     eax, 5                      ; EAX = tail*32
    add     eax, ENTRIES                ; EAX = entry address
    mov     esi, eax                    ; ESI = entry address

; ============================================================================
; OPCODE DISPATCH - Check Which Operation Was Requested
; ============================================================================

    ; Read op at entry+8
    mov     edx, [esi+8]               ; EDX = op
    cmp     edx, 1
    jne     error_resp

; ============================================================================
; EXTRACT REQUEST FIELDS AND COMPUTE - Read Pointers, Perform Addition
; ============================================================================
;
; WHY: The x86 CISC architecture lets us load pointers directly from
; memory using displacement addressing ([esi+16], [esi+24]). The actual
; add operation uses a memory source operand ([edi+4]) so we load val1
; into EAX, then add val2 from memory in a single instruction. This is
; noticeably more compact than the IE32 version, which needs separate
; LOAD instructions for every memory access.

    ; Read reqPtr at entry+16
    mov     edi, [esi+16]              ; EDI = reqPtr

    ; Read respPtr at entry+24
    mov     ebp, [esi+24]             ; EBP = respPtr

    ; Read ticket at entry+0
    mov     ecx, [esi]                 ; ECX = ticket

    ; Op=1: add two uint32
    mov     eax, [edi]                 ; EAX = val1
    add     eax, [edi+4]              ; EAX = val1 + val2
    mov     [ebp], eax                ; write result

; ============================================================================
; WRITE RESPONSE DESCRIPTOR - Signal Completion to the Caller
; ============================================================================
;
; WHY: The response descriptor tells the caller what happened. The x86
; can write immediate values directly to memory with displacement
; addressing (e.g., "mov dword [eax+4], 2"), making the response
; construction very compact -- four MOV instructions fill all four fields.

    ; Compute response address: RESPONSES + tail*16
    mov     eax, ebx                   ; EAX = tail
    shl     eax, 4                     ; EAX = tail*16
    add     eax, RESPONSES             ; EAX = response address

    ; Write response descriptor
    mov     [eax], ecx                 ; ticket
    mov     dword [eax+4], 2           ; status = 2 (ok)
    mov     dword [eax+8], 0           ; resultCode = 0
    mov     dword [eax+12], 4          ; respLen = 4

; ============================================================================
; ADVANCE TAIL - Mark This Slot as Consumed
; ============================================================================
;
; WHY: Incrementing tail modulo 16 frees this ring slot for reuse.
; We write only the low byte (AL) back to the tail pointer, since it
; is defined as a single byte in the ring buffer header.

    ; Advance tail: (tail + 1) & 15
    mov     eax, ebx
    inc     eax
    and     eax, 0x0F
    mov     byte [RING_BASE+1], al

    jmp     main_loop

; ============================================================================
; ERROR RESPONSE - Unknown Opcode Handler
; ============================================================================
;
; WHY: If the caller sends an opcode we do not recognise, we must still
; advance the tail (otherwise the ring would stall forever). We write
; status=3 (error) and resultCode=1 (unknown op) so the caller knows
; the request was rejected rather than silently lost.

error_resp:
    ; Compute response address
    mov     eax, ebx
    shl     eax, 4
    add     eax, RESPONSES

    ; Write error response
    mov     ecx, [esi]                 ; ticket
    mov     [eax], ecx
    mov     dword [eax+4], 3           ; status = 3 (error)
    mov     dword [eax+8], 1           ; resultCode = 1
    mov     dword [eax+12], 0          ; respLen = 0

    ; Advance tail
    mov     eax, ebx
    inc     eax
    and     eax, 0x0F
    mov     byte [RING_BASE+1], al

    jmp     main_loop
