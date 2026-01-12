.equ VIDEO_MODE  0xF004    ; Video mode register
.equ VIDEO_CTRL  0xF000    ; Video control register
.equ VRAM_START  0x100000  ; Start of video RAM
.equ LINE_WIDTH  2560      ; 640 pixels * 4 bytes per pixel (RGBA)

start:
    ; Initialize video in 640x480 mode and ENABLE IT FIRST
    LDA #0              ; Mode 0 = 640x480
    STA @VIDEO_MODE
    LDA #1              ; Enable video - THIS IS CRUCIAL
    STA @VIDEO_CTRL

    ; Wait a tiny bit to ensure video is enabled
    LDA #1000
    WAIT A

    ; Setup registers
    LDX #VRAM_START    ; X = current VRAM position
    LDA #0xFF          ; Start with full alpha
    LDY #480          ; Y = number of lines to draw

draw_lines:
    ; Remember line start position
    LDB #640          ; B = pixels to draw in this line
    LDC X             ; C = save current X position

draw_line:
    STA [X]           ; Write color to VRAM
    ADD X, #4         ; Move to next pixel
    DEC B             ; Decrement pixel counter
    JNZ B, draw_line  ; Continue if not end of line

    ADD A, #0x01000000 ; Increment just the red component
    LDX C             ; Restore X to start of current line
    ADD X, #2560      ; Move X to next line start (640*4)
    DEC Y             ; Decrement line counter
    JNZ Y, draw_lines ; Continue if not last line

loop:
    JMP loop          ; Infinite loop