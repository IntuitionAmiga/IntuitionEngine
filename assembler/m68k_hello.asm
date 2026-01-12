; m68k_hello.asm - Hello world for the 68k core of IntuitionEngine
;
; https://ko-fi.com/intuition/tip
; https://github.com/IntuitionEngine

        ORG     $1000

START   LEA     message,A0      ; Load effective address of message into A0
        MOVE.W  #$F900,A1       ; Terminal output address in A1

loop:
        TST.B   (A0)            ; Check for null terminator
        BEQ     done            ; If zero, we're done
        MOVE.B  (A0)+,(A1)      ; Copy byte from message to terminal
        BRA     loop            ; Repeat

done:
        STOP    #0              ; Halt execution

message:
        DC.B    "Hello, from the 68020 CPU on Intuition Engine!",0       ; Null-terminated message

        END     START          ; End of program, entry point is START
