; Coprocessor mailbox service: x86 guest code that waits for mailbox entries, reads an add request from shared guest RAM, writes the response and advances the ring tail.
;
; The caller and service communicate through shared guest RAM and coprocessor MMIO.
; The Host SDK builds both guest binaries; it is not present at runtime. Read the
; mailbox constants, initialisation, descriptor exchange and terminal path in order.
; Run the caller with `go run . -x86 -coproc-svc <service> <caller>`.
; Compare coproc_caller_x86.asm as its matching pair and the other CPU ports for their
; real addressing differences.
;
%include "ie86.inc"

    bits 32
    org 0x320000

; ============================================================================
; CONSTANTS - Ring Buffer Addresses
; ============================================================================
;
; The x86 is assigned ring index 8 (cpuTypeIndex 4 * 2 + instance 0) at the
; uniform $400 stride, so its ring lives at mailbox base + 8 * $400 = $792000.
; Each ring has 16 entry slots (32 bytes each) starting at offset +8, and 16
; response slots (16 bytes each) starting at offset +$208.

RING_BASE   equ 0x792000
ENTRIES     equ RING_BASE + 0x08
RESPONSES   equ RING_BASE + 0x208
RING_ACK    equ RING_BASE + 0x04    ; version-gate ack byte
LAYOUT_VER  equ 1                   ; COPROC_LAYOUT_VERSION

; ============================================================================
; MAIN POLL LOOP - Wait for Requests
; ============================================================================
;
; The service spins on head != tail. The caller advances head when
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
;   EBP = assigned ring base (seeded by host)

    ; EBP = assigned ring base, seeded by the host at worker entry so the same
    ; image serves whichever instance ring the manager selected (bootstrap
    ; patch). RING_BASE below is only the instance-0 default, kept for
    ; documentation; the running code uses EBP-relative addressing.

    ; Version-gate handshake: echo the mailbox layout version so the host
    ; routes work to this service. A stale image built for the old layout
    ; never reaches this address and is refused at START.
    mov     byte [ebp+4], LAYOUT_VER    ; RING_ACK = ring base + 4

main_loop:
    ; Read tail (byte)
    movzx   eax, byte [ebp+1]           ; EAX = tail

    ; Read head (byte)
    movzx   ecx, byte [ebp]             ; ECX = head

    ; Compare: if tail == head, ring empty
    cmp     eax, ecx
    je      main_loop

; ============================================================================
; ENTRY ADDRESS COMPUTATION - Locate the Request Descriptor
; ============================================================================
;
; Each entry is 32 bytes, so entry address = ENTRIES + tail * 32.
; The x86 barrel shifter computes tail*32 in a single SHL instruction.
; We save tail in EBX before clobbering EAX with the address computation,
; since we need the original tail value later for the response descriptor
; and tail advancement.

    ; Save tail in EBX
    mov     ebx, eax

    ; Compute entry address: ring base + 8 (ENTRIES) + tail*32
    shl     eax, 5                      ; EAX = tail*32
    lea     esi, [ebp+eax+8]            ; ESI = entry address

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
; The x86 CISC architecture lets us load pointers directly from
; memory using displacement addressing ([esi+16], [esi+24]). The actual
; add operation uses a memory source operand ([edi+4]) so we load val1
; into EAX, then add val2 from memory in a single instruction. This is
; noticeably more compact than the IE32 version, which needs separate
; LOAD instructions for every memory access.

    ; Read reqPtr at entry+16
    mov     edi, [esi+16]              ; EDI = reqPtr

    ; Read ticket at entry+0
    mov     ecx, [esi]                 ; ECX = ticket

    ; Op=1: add two uint32
    mov     eax, [edi]                 ; EAX = val1
    add     eax, [edi+4]              ; EAX = val1 + val2
    ; EBP is the ring base, so respPtr is re-read into EDI (reqPtr is done).
    mov     edi, [esi+24]             ; EDI = respPtr
    mov     [edi], eax                ; write result

; ============================================================================
; WRITE RESPONSE DESCRIPTOR - Signal Completion to the Caller
; ============================================================================
;
; The response descriptor tells the caller what happened. The x86
; can write immediate values directly to memory with displacement
; addressing (e.g., "mov dword [eax+4], 2"), making the response
; construction very compact -- four MOV instructions fill all four fields.

    ; Compute response address: RESPONSES + tail*16
    mov     eax, ebx                   ; EAX = tail
    shl     eax, 4                     ; EAX = tail*16
    lea     eax, [ebp+eax+0x208]       ; ring base + $208 (RESPONSES) + tail*16

    ; Write response descriptor
    mov     [eax], ecx                 ; ticket
    mov     dword [eax+4], 2           ; status = 2 (ok)
    mov     dword [eax+8], 0           ; resultCode = 0
    mov     dword [eax+12], 4          ; respLen = 4

; ============================================================================
; ADVANCE TAIL - Mark This Slot as Consumed
; ============================================================================
;
; Incrementing tail modulo 16 frees this ring slot for reuse.
; We write only the low byte (AL) back to the tail pointer, since it
; is defined as a single byte in the ring buffer header.

    ; Advance tail: (tail + 1) & 15
    mov     eax, ebx
    inc     eax
    and     eax, 0x0F
    mov     byte [ebp+1], al

    jmp     main_loop

; ============================================================================
; ERROR RESPONSE - Unknown Opcode Handler
; ============================================================================
;
; If the caller sends an opcode we do not recognise, we must still
; advance the tail (otherwise the ring would stall forever). We write
; status=3 (error) and resultCode=1 (unknown op) so the caller knows
; the request was rejected rather than silently lost.

error_resp:
    ; Compute response address
    mov     eax, ebx
    shl     eax, 4
    lea     eax, [ebp+eax+0x208]

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
    mov     byte [ebp+1], al

    jmp     main_loop
