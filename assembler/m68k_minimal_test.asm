; m68k_minimal_test.asm
TERM_OUT    equ $F900     ; Terminal output register


    section code
    org     $000400

start:
    move.l  #$48,TERM_OUT    ; ASCII 'H'
    move.l  #$69,TERM_OUT    ; ASCII 'i'
    bra     start            ; Loop forever