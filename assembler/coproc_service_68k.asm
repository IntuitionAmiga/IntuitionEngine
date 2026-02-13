; coproc_service_68k.asm - M68K Coprocessor Service Template
;
; Service binary contract:
;   1. Poll ring buffer for requests
;   2. Dispatch on op field
;   3. Op=1 (add): read two uint32 from reqPtr, add, write to respPtr
;   4. Write response descriptor (status=2/ok, respLen=4)
;   5. Advance tail, loop
;
; Memory map:
;   Code loaded at 0x280000 (WORKER_M68K_BASE)
;   Mailbox at 0x820000 (MAILBOX_BASE)
;   Ring 2 (M68K): 0x820000 + 2*0x300 = 0x820600
;     head:       0x820600 (byte)
;     tail:       0x820601 (byte)
;     entries:    0x820608 + tail*32
;     responses:  0x820808 + tail*16
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

    org $280000

RING_BASE   equ $820600
ENTRIES     equ RING_BASE+8
RESPONSES   equ RING_BASE+$208

main_loop:
    ; Read tail (byte)
    move.b  (RING_BASE+1),d0
    andi.l  #$FF,d0             ; D0 = tail

    ; Read head (byte)
    move.b  (RING_BASE),d1
    andi.l  #$FF,d1             ; D1 = head

    ; Compare: if tail == head, ring empty
    cmp.l   d1,d0
    beq     main_loop

    ; Compute entry address: ENTRIES + tail*32
    move.l  d0,d2
    lsl.l   #5,d2               ; D2 = tail*32
    lea     ENTRIES,a0
    adda.l  d2,a0               ; A0 = entry address

    ; Read op at entry+8
    move.l  8(a0),d3            ; D3 = op
    cmpi.l  #1,d3
    bne     error_resp

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

    ; Compute response address: RESPONSES + tail*16
    move.l  d0,d2
    lsl.l   #4,d2               ; D2 = tail*16
    lea     RESPONSES,a3
    adda.l  d2,a3               ; A3 = response address

    ; Write response descriptor
    move.l  d4,(a3)             ; ticket
    move.l  #2,4(a3)            ; status = 2 (ok)
    move.l  #0,8(a3)            ; resultCode = 0
    move.l  #4,12(a3)           ; respLen = 4

    ; Advance tail: (tail + 1) & 15
    move.l  d0,d2
    addq.l  #1,d2
    andi.l  #$0F,d2
    move.b  d2,(RING_BASE+1)

    bra     main_loop

error_resp:
    ; Compute response address
    move.l  d0,d2
    lsl.l   #4,d2
    lea     RESPONSES,a3
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
    move.b  d2,(RING_BASE+1)

    bra     main_loop
