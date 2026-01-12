.equ VIDEO_MODE 0xF004
.equ VIDEO_CTRL 0xF000
.equ VRAM_START 0x100000
.equ GREEN 0x00FF00FF
.equ YELLOW 0xFFFF00FF

start:
   LDA #0
   STA @VIDEO_MODE
   LDA #1
   STA @VIDEO_CTRL

   ; Fill background
   LDX #VRAM_START
fill_bg:
   LDA #YELLOW
   STA [X]
   ADD X, #4
   LDY #0x230000  ; End of VRAM
   SUB Y, X
   JNZ Y, fill_bg

main_loop:
   JMP main_loop





;   ; Draw text starting at pixel offset
;   LDX #VRAM_START
;   ADD X, #153284     ; Center on screen
;
;   ; I
;   LDA #GREEN
;   STA [X]
;   ADD X, #2560       ; Next row (640*4)
;   STA [X]
;   ADD X, #2560
;   STA [X]
;
;   ; N
;   ADD X, #16         ; Next character position
;   STA [X]
;   ADD X, #2560
;   STA [X]
;   ADD X, #2560
;   STA [X]
;   SUB X, #5120       ; Back to top
;   ADD X, #4          ; Right leg of N
;   STA [X]
;   ADD X, #2560
;   STA [X]
;   ADD X, #2560
;   STA [X]

;main_loop:
;   JMP main_loop