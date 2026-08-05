; Coprocessor mailbox service: M68020 guest code that waits for mailbox entries, reads an add request from shared guest RAM, writes the response and advances the ring tail.
;
; The caller and service communicate through shared guest RAM and coprocessor MMIO.
; Read the
; mailbox constants, initialisation, descriptor exchange and terminal path in order.
; Compare coproc_caller_68k.asm as its matching pair and the other CPU ports for their
; real addressing differences.
;


    include "ie68.inc"

    org $280000

; ============================================================================
; CONSTANTS - Ring Buffer Addresses
; ============================================================================
;
; The M68K is assigned ring index 4 (instance 0), so its ring lives at
; mailbox base + 4 * $400 = $791000. Each ring has 16 entry slots (32 bytes
; each) starting at offset +8, and 16 response slots (16 bytes each) starting
; at offset +$208. The running code addresses the ring through the seeded A4.

; M68K is ring index 4 (cpuTypeIndex 2 * 2 + instance 0) at the uniform $400
; stride: MAILBOX_BASE + 4 * $400 = $791000.
RING_BASE   equ $791000
ENTRIES     equ RING_BASE+8
RESPONSES   equ RING_BASE+$208
RING_ACK    equ RING_BASE+4     ; version-gate ack byte
LAYOUT_VER  equ 1               ; COPROC_LAYOUT_VERSION

; ============================================================================
; MAIN POLL LOOP - Wait for Requests
; ============================================================================
;
; The service spins on head != tail. The caller advances head when
; it enqueues a request; we advance tail when we finish processing one.
; Both are single bytes, so reads are atomic on any architecture.
;
; Register usage in the main loop:
;   D0 = tail (zero-extended from byte)
;   D1 = head (zero-extended from byte)
;   D2 = scratch for address computation
;   D3 = op field from entry
;   D4 = ticket from entry
;   D5 = computation result
;   A0 = entry descriptor pointer
;   A1 = request data pointer (reqPtr)
;   A2 = response data pointer (respPtr)
;   A3 = response descriptor pointer

    ; A4 = assigned ring base, seeded by the host at worker entry so the same
    ; image serves whichever instance ring the manager selected (RING_BASE is
    ; only the instance-0 default, kept for documentation). All ring accesses
    ; below are A4-relative.

    ; Version-gate handshake: echo the mailbox layout version so the host
    ; routes work to this service.
    move.b  #LAYOUT_VER,4(a4)   ; RING_ACK = ring base + 4

main_loop:
    ; Read tail (byte)
    move.b  1(a4),d0            ; ring base + 1
    andi.l  #$FF,d0             ; D0 = tail

    ; Read head (byte)
    move.b  (a4),d1             ; ring base + 0
    andi.l  #$FF,d1             ; D1 = head

    ; Compare: if tail == head, ring empty
    cmp.l   d1,d0
    beq     main_loop

; ============================================================================
; ENTRY ADDRESS COMPUTATION - Locate the Request Descriptor
; ============================================================================
;
; Each entry is 32 bytes, so entry address = ENTRIES + tail * 32.
; The 68020's barrel shifter computes tail*32 in a single LSL.L #5
; instruction. LEA loads the entries base into A0, then ADDA adds the
; offset -- a clean two-instruction sequence.

    ; Compute entry address: ENTRIES + tail*32
    move.l  d0,d2
    lsl.l   #5,d2               ; D2 = tail*32
    lea     8(a4),a0           ; ENTRIES = ring base + 8
    adda.l  d2,a0               ; A0 = entry address

; ============================================================================
; OPCODE DISPATCH - Check Which Operation Was Requested
; ============================================================================

    ; Read op at entry+8
    move.l  8(a0),d3            ; D3 = op
    cmpi.l  #1,d3
    bne     error_resp

; ============================================================================
; EXTRACT REQUEST FIELDS AND COMPUTE - Read Pointers, Perform Addition
; ============================================================================
;
; The 68020's address registers can load pointers directly from
; memory with displacement addressing. A1 = reqPtr, A2 = respPtr,
; D4 = ticket -- all extracted from the entry descriptor in three
; instructions. The actual computation (ADD.L) is a single opcode
; because the 68020 is a native 32-bit CPU.

    ; Read reqPtr at entry+16
    move.l  16(a0),a1           ; A1 = reqPtr

    ; Read respPtr at entry+24
    move.l  24(a0),a2           ; A2 = respPtr

    ; Read ticket at entry+0
    move.l  (a0),d4             ; D4 = ticket

    ; Op=1: add two uint32
    move.l  (a1),d5             ; D5 = val1
    add.l   4(a1),d5            ; D5 = val1 + val2
    move.l  d5,(a2)             ; write result

; ============================================================================
; WRITE RESPONSE DESCRIPTOR - Signal Completion to the Caller
; ============================================================================
;
; The response descriptor tells the caller what happened. We echo
; back the ticket for correlation, set status=2 (success), resultCode=0,
; and respLen=4 (one uint32 of result data). The 68020 can write all
; four fields with immediate MOVE.L instructions using displacement
; addressing -- no pointer arithmetic needed between writes.

    ; Compute response address: RESPONSES + tail*16
    move.l  d0,d2
    lsl.l   #4,d2               ; D2 = tail*16
    lea     $208(a4),a3        ; RESPONSES = ring base + $208
    adda.l  d2,a3               ; A3 = response address

    ; Write response descriptor
    move.l  d4,(a3)             ; ticket
    move.l  #2,4(a3)            ; status = 2 (ok)
    move.l  #0,8(a3)            ; resultCode = 0
    move.l  #4,12(a3)           ; respLen = 4

; ============================================================================
; ADVANCE TAIL - Mark This Slot as Consumed
; ============================================================================
;
; Incrementing tail modulo 16 frees this ring slot for reuse.
; The AND mask ($0F) provides the wrap-around for a 16-entry ring.

    ; Advance tail: (tail + 1) & 15
    move.l  d0,d2
    addq.l  #1,d2
    andi.l  #$0F,d2
    move.b  d2,1(a4)            ; advance tail

    bra     main_loop

; ============================================================================
; ERROR RESPONSE - Unknown Opcode Handler
; ============================================================================
;
; If the caller sends an opcode we do not recognise, we must still
; advance the tail (otherwise the ring would stall forever). We write
; status=3 (error) and resultCode=1 (unknown op) so the caller knows
; the request was rejected rather than lost.

error_resp:
    ; Compute response address
    move.l  d0,d2
    lsl.l   #4,d2
    lea     $208(a4),a3        ; RESPONSES = ring base + $208
    adda.l  d2,a3

    ; Write error response
    move.l  (a0),d4             ; ticket
    move.l  d4,(a3)
    move.l  #3,4(a3)            ; status = 3 (error)
    move.l  #1,8(a3)            ; resultCode = 1
    move.l  #0,12(a3)           ; respLen = 0

    ; Advance tail
    move.l  d0,d2
    addq.l  #1,d2
    andi.l  #$0F,d2
    move.b  d2,1(a4)            ; advance tail (ring base + 1)

    bra     main_loop
