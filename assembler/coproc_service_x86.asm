; coproc_service_x86.asm - x86 Coprocessor Service Template
;
; Service binary contract:
;   1. Poll ring buffer for requests
;   2. Dispatch on op field
;   3. Op=1 (add): read two uint32 from reqPtr, add, write to respPtr
;   4. Write response descriptor (status=2/ok, respLen=4)
;   5. Advance tail, loop
;
; Memory map:
;   Code loaded at 0x320000 (WORKER_X86_BASE)
;   Mailbox at 0x820000 (MAILBOX_BASE)
;   Ring 4 (x86): 0x820000 + 4*0x300 = 0x820C00
;     head:       0x820C00 (byte)
;     tail:       0x820C01 (byte)
;     entries:    0x820C08 + tail*32
;     responses:  0x820E08 + tail*16
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

    bits 32
    org 0x320000

RING_BASE   equ 0x820C00
ENTRIES     equ RING_BASE + 0x08
RESPONSES   equ RING_BASE + 0x208

main_loop:
    ; Read tail (byte)
    movzx   eax, byte [RING_BASE+1]     ; EAX = tail

    ; Read head (byte)
    movzx   ecx, byte [RING_BASE]       ; ECX = head

    ; Compare: if tail == head, ring empty
    cmp     eax, ecx
    je      main_loop

    ; Save tail in EBX
    mov     ebx, eax

    ; Compute entry address: ENTRIES + tail*32
    shl     eax, 5                      ; EAX = tail*32
    add     eax, ENTRIES                ; EAX = entry address
    mov     esi, eax                    ; ESI = entry address

    ; Read op at entry+8
    mov     edx, [esi+8]               ; EDX = op
    cmp     edx, 1
    jne     error_resp

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

    ; Compute response address: RESPONSES + tail*16
    mov     eax, ebx                   ; EAX = tail
    shl     eax, 4                     ; EAX = tail*16
    add     eax, RESPONSES             ; EAX = response address

    ; Write response descriptor
    mov     [eax], ecx                 ; ticket
    mov     dword [eax+4], 2           ; status = 2 (ok)
    mov     dword [eax+8], 0           ; resultCode = 0
    mov     dword [eax+12], 4          ; respLen = 4

    ; Advance tail: (tail + 1) & 15
    mov     eax, ebx
    inc     eax
    and     eax, 0x0F
    mov     byte [RING_BASE+1], al

    jmp     main_loop

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
