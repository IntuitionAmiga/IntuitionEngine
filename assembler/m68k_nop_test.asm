; m68k_nop_test.asm
    section code
    org     $000400

start:
    nop                 ; No operation (0x4E71)
    bra     start       ; Branch back to start