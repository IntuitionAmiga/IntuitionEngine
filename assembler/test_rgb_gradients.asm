; test_rgb_gradients.asm
; Video demonstration https://youtu.be/GcP03aN4kbQ

.equ VIDEO_MODE  0xF0004
.equ VIDEO_CTRL  0xF0000
.equ VRAM_START  0x100000
.equ VRAM_END    0x22C000

start:
   ; Initialize video in 640x480 mode
   LDA #0
   STA @VIDEO_MODE
   LDA #1
   STA @VIDEO_CTRL

loop_start:
   ; First gradient: Black to Red (shift value into byte 0 - bits 0-7)
   LDX #VRAM_START     ; Current VRAM position
   LDY #0              ; Pixel counter

fill_red:
   ; Get Y coordinate (0-479)
   LDA Y
   DIV A, #640        ; Y = pixel_count / 640
   MUL B, #640        ; B = Y * 640
   SUB B, Y           ; B = -(X)
   NOT B              ; B = X

   ; Calculate color intensity based on Y coordinate
   LDA Y
   DIV A, #640        ; Get Y coordinate again (0-479)
   MUL A, #255        ; Scale to 0-255 color range
   DIV A, #479        ; Divide by max Y
   SHL A, #0          ; Put in red channel (byte 0)
   OR  A, #0xFF000000 ; Set alpha to 255

   STA [X]            ; Write pixel
   ADD X, #4          ; Move to next pixel
   INC Y              ; Increment counter

   LDB #VRAM_END      ; Check if we've reached the end
   SUB B, X
   JNZ B, fill_red

   ; Pause between gradients
   LDA #0x3000000      ; Delay duration
pause1:
   DEC A
   JNZ A, pause1

   ; Second gradient: Black to Green (shift value into byte 1 - bits 8-15)
   LDX #VRAM_START
   LDY #0

fill_green:
   LDA Y
   DIV A, #640
   MUL B, #640
   SUB B, Y
   NOT B

   LDA Y
   DIV A, #640
   MUL A, #255
   DIV A, #479
   SHL A, #8         ; Put in green channel (byte 1)
   OR  A, #0xFF000000 ; Set alpha to 255

   STA [X]
   ADD X, #4
   INC Y

   LDB #VRAM_END
   SUB B, X
   JNZ B, fill_green

   ; Second pause
   LDA #0x3000000
pause2:
   DEC A
   JNZ A, pause2

   ; ; Third gradient: Black to Blue (shift value into byte 2 - bits 16-23)
   LDX #VRAM_START
   LDY #0

fill_blue:
   LDA Y
   DIV A, #640
   MUL B, #640
   SUB B, Y
   NOT B

   LDA Y
   DIV A, #640
   MUL A, #255
   DIV A, #479
   SHL A, #16          ; Put in blue channel (byte 2)
   OR  A, #0xFF000000  ; Set alpha to 255

   STA [X]
   ADD X, #4
   INC Y

   LDB #VRAM_END
   SUB B, X
   JNZ B, fill_blue

    ; Third pause
   LDA #0x3000000
pause3:
   DEC A
   JNZ A, pause3

   ; Wait forever
loop:
   JMP loop_start