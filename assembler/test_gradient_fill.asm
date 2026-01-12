.equ VIDEO_MODE  0xF004
.equ VIDEO_CTRL  0xF000
.equ VRAM_START  0x100000
.equ VRAM_END    0x22C000

start:
   LDA #0
   STA @VIDEO_MODE
   LDA #1
   STA @VIDEO_CTRL

   LDX #VRAM_START     ; Current VRAM position
   LDY #0              ; Pixel counter

fill_screen:
   ; Calculate X (0-639) and Y (0-479) from pixel counter
   LDA Y
   DIV A, #640        ; Y = pixel_count / 640
   MUL B, #640        ; B = Y * 640
   SUB B, Y           ; B = -(X) (because Y = Y*640)
   NOT B              ; B = X

   ; Calculate color components
   MUL B, #255        ; Red component (X * 255/640)
   DIV B, #639        ;
   SHL B, #24         ; Shift to red position

   MUL A, #255        ; Green component (Y * 255/480)
   DIV A, #479
   SHL A, #16         ; Shift to green position

   OR  A, B           ; Combine R+G
   OR  A, #0xFF       ; Set alpha

   STA [X]            ; Write pixel
   ADD X, #4
   INC Y

   LDB #VRAM_END
   SUB B, X
   JNZ B, fill_screen

loop:
   JMP loop