; test_rgb_gradients_68k_final.asm - 68020 port with endianness fixes
; For VASM assembler with Devpac syntax

; Constants for memory-mapped registers
VIDEO_MODE  equ $F004    ; Video mode register
VIDEO_CTRL  equ $F000    ; Video control register
VIDEO_STATUS equ $F008   ; Video status register
VRAM_START  equ $100000  ; Start of video RAM
VRAM_END    equ $4FFFFF  ; End of video RAM (4MB VRAM)

    section code
    org     $000400      ; Start at the load address

start:
    ; Wait for any previous operations to complete
    move.l  #$400000,d0
idle_wait:
    subq.l  #1,d0
    bne     idle_wait

    ; Initialize video in 640x480 mode (mode 0)
    moveq   #0,d0
    move.l  d0,VIDEO_MODE

    ; Enable video (write 1 to VIDEO_CTRL)
    moveq   #1,d0
    move.l  d0,VIDEO_CTRL

    ; Wait for video to initialize
    move.l  #$400000,d0
init_wait:
    subq.l  #1,d0
    bne     init_wait

loop_start:
    ; First gradient: Black to Red (RGBA format, little-endian)
    move.l  #VRAM_START,a0     ; Current VRAM position
    moveq   #0,d5              ; Y coordinate

red_y_loop:
    ; Calculate intensity based on Y (0-479)
    move.l  d5,d3              ; Copy Y
    mulu    #255,d3            ; Scale to 0-255
    divu    #479,d3            ; Normalize to max Y
    and.l   #$ff,d3            ; Keep only 8 bits

    ; Build RGBA color value (little-endian: ABGR)
    move.l  #$000000FF,d2      ; Alpha = 255
    or.l    d3,d2              ; Add red component

    ; Fill one row with the calculated color
    moveq   #0,d6              ; X counter
    move.l  a0,a1              ; Save row start position

red_x_loop:
    move.l  d2,(a0)+           ; Write pixel
    addq.l  #1,d6              ; Increment X
    cmpi.l  #640,d6            ; Check if row complete
    blt     red_x_loop

    addq.l  #1,d5              ; Increment Y
    cmpi.l  #480,d5            ; Check if all rows done
    bge     red_done

    ; Move to next row start
    adda.l  #640*4,a1          ; Calculate next row address
    move.l  a1,a0              ; Update current position
    bra     red_y_loop

red_done:
    ; Pause between gradients
    move.l  #$2000000,d0
pause1:
    subq.l  #1,d0
    bne     pause1

    ; Second gradient: Black to Green (RGBA format, little-endian)
    move.l  #VRAM_START,a0
    moveq   #0,d5              ; Y coordinate

green_y_loop:
    ; Calculate intensity based on Y (0-479)
    move.l  d5,d3              ; Copy Y
    mulu    #255,d3            ; Scale to 0-255
    divu    #479,d3            ; Normalize to max Y
    and.l   #$ff,d3            ; Keep only 8 bits

    ; Build RGBA color value (little-endian: ABGR)
    lsl.l   #8,d3              ; Shift to green position
    move.l  #$000000FF,d2      ; Alpha = 255
    or.l    d3,d2              ; Add green component

    ; Fill one row with the calculated color
    moveq   #0,d6              ; X counter
    move.l  a0,a1              ; Save row start position

green_x_loop:
    move.l  d2,(a0)+           ; Write pixel
    addq.l  #1,d6              ; Increment X
    cmpi.l  #640,d6            ; Check if row complete
    blt     green_x_loop

    addq.l  #1,d5              ; Increment Y
    cmpi.l  #480,d5            ; Check if all rows done
    bge     green_done

    ; Move to next row start
    move.l  a1,a0              ; Reset to row start
    adda.l  #640*4,a0          ; Move to next row
    move.l  a0,a1              ; Save new row position
    bra     green_y_loop

green_done:
    ; Pause between gradients
    move.l  #$2000000,d0
pause2:
    subq.l  #1,d0
    bne     pause2

    ; Third gradient: Black to Blue (RGBA format, little-endian)
    move.l  #VRAM_START,a0
    moveq   #0,d5              ; Y coordinate

blue_y_loop:
    ; Calculate intensity based on Y (0-479)
    move.l  d5,d3              ; Copy Y
    mulu    #255,d3            ; Scale to 0-255
    divu    #479,d3            ; Normalize to max Y
    and.l   #$ff,d3            ; Keep only 8 bits

    ; Build RGBA color value (little-endian: ABGR)
    lsl.l   #8,d3              ; Shift left 8 bits
    lsl.l   #8,d3              ; Shift left another 8 bits (total 16)
    move.l  #$000000FF,d2      ; Alpha = 255
    or.l    d3,d2              ; Add blue component

    ; Fill one row with the calculated color
    moveq   #0,d6              ; X counter
    move.l  a0,a1              ; Save row start position

blue_x_loop:
    move.l  d2,(a0)+           ; Write pixel
    addq.l  #1,d6              ; Increment X
    cmpi.l  #640,d6            ; Check if row complete
    blt     blue_x_loop

    addq.l  #1,d5              ; Increment Y
    cmpi.l  #480,d5            ; Check if all rows done
    bge     blue_done

    ; Move to next row start
    move.l  a1,a0              ; Reset to row start
    adda.l  #640*4,a0          ; Move to next row
    move.l  a0,a1              ; Save new row position
    bra     blue_y_loop

blue_done:
    ; Pause between gradients
    move.l  #$2000000,d0
pause3:
    subq.l  #1,d0
    bne     pause3

    ; Loop back to start
    bra     loop_start