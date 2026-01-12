; m68k_simple_test.asm - Minimal terminal output test
; For VASM assembler with Devpac syntax

TERM_OUT    equ $F900     ; Terminal output register

    section code
    org     $000400       ; Note the proper syntax with indentation

start:
    lea     message,a0    ; Load message address
loop:
    move.b  (a0)+,d0      ; Get next character
    beq     done          ; Exit if null terminator
    move.l  d0,TERM_OUT   ; Output to terminal
    bra     loop          ; Continue
done:
    bra     done          ; Infinite loop

message:
    dc.b    "Hello from M68K! This is a terminal output test.",13,10,0