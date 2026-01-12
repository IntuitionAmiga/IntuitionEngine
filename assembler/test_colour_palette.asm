.equ VIDEO_MODE  0xF004
.equ VIDEO_CTRL  0xF000
.equ VRAM_START  0x100000
.equ VRAM_END    0x22C000    ; 0x100000 + (640 * 480 * 4)

start:
   LDA #0              ; MODE_640x480
   STA @VIDEO_MODE
   LDA #1              ; Enable video
   STA @VIDEO_CTRL

   LDX #VRAM_START     ; Start of VRAM
   LDA #0xFF000000     ; Start with red

fill_screen:
   STA [X]             ; Write color to current position
   ADD X, #4           ; Move to next pixel
   ADD A, #0x00010000  ; Increment green component
   LDY #VRAM_END
   SUB Y, X
   JNZ Y, fill_screen

loop:
   JMP loop