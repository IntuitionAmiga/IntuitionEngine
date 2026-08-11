; ULA BOING BALL - Embedded MIDI Data and IE64 C ABI Helpers
; ============================================================================
;
; SDK QUICK REFERENCE
;
; Target CPU:    IE64
; Assembler:     ie64asm, invoked by ie64-cproc
; Input asset:   sdk/examples/assets/music/shadowofthebeast.mid
; Output:        MIDI bytes linked into ula_boing_ie64.ie64
; C interface:   boing_music_data_ptr() and boing_music_data_end_ptr()
;
; WHY THIS FILE EXISTS
;
; The completed demo must contain its soundtrack rather than load an external
; file at run time. ie64asm supports incbin, which copies the source file into
; the relocatable object during assembly. The linker carries those bytes into
; the final image. The C programme then passes the embedded start address and
; byte length to the MIDI player's PTR/LEN/CTRL registers.
;
; QBE's IE64 backend does not currently select a C conversion from a linked
; pointer to the 32-bit register value required by the player. These two small
; functions materialise the linked start and end addresses explicitly. Under
; the IE64 C ABI, an unsigned 32-bit result is returned in r1.
;
; The C programme subtracts the start address from the end address. Keeping both
; labels around the incbin data means the length follows the asset automatically
; and is never repeated as a numeric constant.
;
; incbin paths are resolved relative to this source file. This file is in
; sdk/examples/c, so ../assets/music selects the SDK music directory.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

; ============================================================================
; EMBEDDED STANDARD MIDI FILE
; ============================================================================
; The MIDI parser does not require alignment. Eight-byte alignment gives the
; linked payload a naturally aligned IE memory address.
.section .rodata,"a"
align 8
.global boing_music_data
boing_music_data:
    incbin "../assets/music/shadowofthebeast.mid"
boing_music_data_end:

; ============================================================================
; IE64 C ABI ADDRESS HELPERS
; ============================================================================
; move.l installs the low 32 bits of the linked address. movt installs its upper
; half. The flat demo is linked below 4 GiB, matching the MIDI player's 32-bit
; address register. rts returns with the value in r1.
.section .text,"ax"
.global boing_music_data_ptr
.type boing_music_data_ptr,@function
boing_music_data_ptr:
    ; Return the address of the first embedded MIDI byte.
    move.l r1, #lo32(boing_music_data)
    movt r1, #hi32(boing_music_data)
    rts
.size boing_music_data_ptr,.-boing_music_data_ptr

.global boing_music_data_end_ptr
.type boing_music_data_end_ptr,@function
boing_music_data_end_ptr:
    ; Return the address immediately after the final embedded MIDI byte.
    move.l r1, #lo32(boing_music_data_end)
    movt r1, #hi32(boing_music_data_end)
    rts
.size boing_music_data_end_ptr,.-boing_music_data_end_ptr
