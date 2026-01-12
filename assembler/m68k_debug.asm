; m68k_debug_output.asm - Simple test for terminal output

; Terminal output register
TERM_OUT     equ $F900     ; Memory-mapped terminal output register

    section code
    org     $000400

start:
    ; Print a simple string to verify terminal output
    lea     hello_msg,a0

    ; Direct character output loop
output_loop:
    move.b  (a0)+,d0       ; Get next character
    beq     done           ; Exit if null terminator

    ; Output directly to terminal register
    move.b  d0,TERM_OUT    ; Write directly to terminal

    ; Add a small delay
    move.l  #$10000,d1
delay_loop:
    subq.l  #1,d1
    bne     delay_loop

    bra     output_loop

done:
    ; Infinite loop
    bra     done

    org     $001000
hello_msg:  dc.b    "Hello from the 68IE020",13,10,0