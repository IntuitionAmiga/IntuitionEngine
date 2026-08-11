; VGA TEXT PLASMA - Z80, VGA MODE 03H AND POKEY+ DEMONSTRATION
; ============================================================================
;
; SDK QUICK REFERENCE
;
; Target CPU:    Z80, assembled as a flat IE80 image
; Execution:     Native Z80 JIT in the x86-64 live USB image
; Video system: VGA Mode 03h, 80 by 25 character cells with 16 colours
; Music:        Hobbytronic '92 - 2 by Stephan Duesterhoeft (Benjy)
; Audio system: Embedded SAP file played through POKEY+
; Build:        make showreel-z80
; Output:       sdk/examples/prebuilt/vga_text_sap_demo.ie80
; Live image:   Demos/z80/vga_text_sap_demo.ie80
;
; WHAT THIS DEMO DOES
;
; Four moving waves form a full-screen plasma from text characters and VGA
; attributes. A masked logo follows a separate two-axis triangle-wave path. The
; plasma and logo fade on independent schedules. The scrolling message remains
; visible in every completed frame, so the published sequence always contains
; at least one layer.
;
; The Z80 constructs every visible character and attribute byte. Each frame is
; rendered into a hidden text page and published during vertical blank. Music
; comes from a SAP file embedded in the IE80 image, so the programme does not
; read a separate music file while it runs.
;
; VGA TEXT RESTRICTION
;
; Mode 03h provides 2,000 character cells rather than an addressable pixel
; bitmap. Each cell contains one character byte and one attribute byte. The
; plasma therefore uses the density characters in density_table as its shades.
; Foreground and background nibbles provide the colour pairs.
;
; Two 4,000-byte pages share the banked text window at $8000. The renderer
; writes one complete page while the other is displayed. The CRTC display start
; is measured in character cells, so page one starts at cell 2,000, or $07D0,
; even though it begins 4,000 bytes after page zero in Z80 memory.
;
; FRAME PIPELINE
;
; Each iteration performs eight ordered stages:
;
;   1. Advance the scene clock, layer fades and motion phases.
;   2. Render the plasma into the hidden text page.
;   3. Composite the faded logo.
;   4. Draw the logo credit line when that layer is visible.
;   5. Draw the permanent scrolling message.
;   6. Wait for the next vertical blank edge.
;   7. Point the CRTC at the completed page and select the other render page.
;   8. Increment frame_complete for development tools.
;
; AUDIO PIPELINE
;
; init_sap enables POKEY+, writes the embedded SAP address and length, then
; writes control value 5 to start looped playback. The image places the SAP at
; $E000 so its address remains stable and separate from code, state and tables.
;
; DEVELOPMENT INTERFACE
;
; Mutable state begins at the fixed address $0F00. The address map beside that
; block identifies the fields intended for IEScript and IE Mon. The supplied
; sdk/scripts/vga_text_sap_demo_acceptance.ies script uses the same map.
; frame_complete changes only after a completed page has been published, which
; gives scripts a reliable frame boundary.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; ============================================================================

    .include "ie80.inc"

; ============================================================================
; DISPLAY GEOMETRY, MEMORY LAYOUT AND HARDWARE REGISTERS
; ============================================================================

; Bank $2E maps VGA text memory into the Z80 window at $8000. Each page holds
; 80 by 25 character and attribute pairs.
.set TEXT_BANK,0x2E
.set TEXT_WINDOW,0x8000
.set CELL_COUNT,80*25
.set TEXT_PAGE_BYTES,80*25*2
.set STACK_TOP,0xDFF0
.set VGA_CRTC_START_HI,0x0C
.set VGA_CRTC_START_LO,0x0D

; The Z80 bridge exposes the POKEY+ selector and SAP player as byte registers.
; Four consecutive bytes carry each 32-bit pointer or length in little-endian
; order.
.set Z80_SAP_PTR_0,0xFD10
.set Z80_SAP_PTR_1,0xFD11
.set Z80_SAP_PTR_2,0xFD12
.set Z80_SAP_PTR_3,0xFD13
.set Z80_SAP_LEN_0,0xFD14
.set Z80_SAP_LEN_1,0xFD15
.set Z80_SAP_LEN_2,0xFD16
.set Z80_SAP_LEN_3,0xFD17
.set Z80_SAP_CTRL,0xFD18
.set Z80_POKEY_PLUS,0xFD09
.set SAP_DATA_LEN,sap_data_end-sap_data

; ============================================================================
; START-UP AND MAIN FRAME LOOP
; ============================================================================

; The programme has no interrupt handler, so start with interrupts disabled and
; perform frame timing by polling VGA status. Configure Mode 03h, map text
; memory, start the soundtrack, initialise all animation state and select page
; zero for the initial display.
    .org 0x0000
start:
    di
    ld sp,STACK_TOP
    ld a,VGA_CTRL_ENABLE
    out (VGA_PORT_CTRL),a
    ld a,VGA_MODE_TEXT
    out (VGA_PORT_MODE),a
    ld a,TEXT_BANK
    ld (VRAM_BANK_REG),a
    call init_sap
    call programme_full_palette
    xor a
    ld (frame_lo),a
    ld (frame_hi),a
    ld (phase_horizontal),a
    ld (phase_vertical),a
    ld (phase_diagonal),a
    ld (phase_cross),a
    ld (scene_marker),a
    ld (plasma_level),a
    ld (logo_level),a
    ld (motion_angle),a
    ld (motion_x_accumulator),a
    ld (motion_x_accumulator+1),a
    ld (motion_y_accumulator),a
    ld (motion_y_accumulator+1),a
    ld (colour_phase),a
    ld (diagonal_wave),a
    ld (hue_wave),a
    ld (displayed_page),a
    ld (logo_phase_x),a
    ld (logo_position_x),a
    ld (logo_position_y),a
    ld (logo_fraction_x),a
    ld (logo_fraction_y),a
    ld (logo_y_tick),a
    ld a,128
    ld (logo_phase_y),a
    ld a,1
    ld (render_page_index),a
    ld hl,TEXT_WINDOW+TEXT_PAGE_BYTES
    ld (render_page_base),hl
    ld a,VGA_CRTC_START_HI
    out (VGA_PORT_CRTC_IDX),a
    xor a
    out (VGA_PORT_CRTC_DATA),a
    ld a,VGA_CRTC_START_LO
    out (VGA_PORT_CRTC_IDX),a
    xor a
    out (VGA_PORT_CRTC_DATA),a
    ld hl,scroll_message
    ld (scroll_pointer),hl

main_loop:
    call advance_timeline
    call render_plasma
    call draw_logo
    call draw_credits
    call draw_scroller
    call wait_vsync
    call publish_text_page
    ld a,(frame_complete)
    inc a
    ld (frame_complete),a
    jp main_loop

; Publish the completed page by changing the CRTC display start. The CRTC value
; is a character-cell address, while render_page_base is a byte address in the
; banked Z80 window. After publication, the former display page becomes the next
; hidden render target.
publish_text_page:
    ld a,(render_page_index)
    ld (displayed_page),a
    or a
    jr z,publish_page_zero
    ld b,0x07
    ld c,0xD0
    jr publish_page_address
publish_page_zero:
    ld b,0
    ld c,0
publish_page_address:
    ld a,VGA_CRTC_START_HI
    out (VGA_PORT_CRTC_IDX),a
    ld a,b
    out (VGA_PORT_CRTC_DATA),a
    ld a,VGA_CRTC_START_LO
    out (VGA_PORT_CRTC_IDX),a
    ld a,c
    out (VGA_PORT_CRTC_DATA),a
    ld a,(render_page_index)
    xor 1
    ld (render_page_index),a
    or a
    jr z,next_render_page_zero
    ld hl,TEXT_WINDOW+TEXT_PAGE_BYTES
    jr store_render_page
next_render_page_zero:
    ld hl,TEXT_WINDOW
store_render_page:
    ld (render_page_base),hl
    ret

; Enable the enhanced POKEY+ path before starting the SAP player. Control value
; 5 means start and loop, which keeps the embedded tune playing indefinitely.
init_sap:
    ld a,1
    ld (Z80_POKEY_PLUS),a
    ld a,sap_data & 0xFF
    ld (Z80_SAP_PTR_0),a
    ld a,(sap_data >> 8) & 0xFF
    ld (Z80_SAP_PTR_1),a
    ld a,(sap_data >> 16) & 0xFF
    ld (Z80_SAP_PTR_2),a
    ld a,(sap_data >> 24) & 0xFF
    ld (Z80_SAP_PTR_3),a
    ld a,SAP_DATA_LEN & 0xFF
    ld (Z80_SAP_LEN_0),a
    ld a,(SAP_DATA_LEN >> 8) & 0xFF
    ld (Z80_SAP_LEN_1),a
    ld a,(SAP_DATA_LEN >> 16) & 0xFF
    ld (Z80_SAP_LEN_2),a
    ld a,(SAP_DATA_LEN >> 24) & 0xFF
    ld (Z80_SAP_LEN_3),a
    ld a,0x05
    ld (Z80_SAP_CTRL),a
    ret

; First leave any active vertical blank, then wait for the next blank to begin.
; Publishing after this call prevents the CRTC page change from tearing a frame.
wait_vsync:
wait_blank_end:
    in a,(VGA_PORT_STATUS)
    and VGA_STATUS_VSYNC
    jr nz,wait_blank_end
wait_blank_start:
    in a,(VGA_PORT_STATUS)
    and VGA_STATUS_VSYNC
    jr z,wait_blank_start
    ret

; ============================================================================
; TIMELINE, MOTION AND LAYER FADES
; ============================================================================

; frame_lo and frame_hi form the scene clock. The opening counter changes scene
; at 96, producing 95 completed opening frames so the logo appears promptly.
; Later scenes last 256 frames. After scene six, the sequence and scrolling
; message restart together.
advance_timeline:
    ld hl,(frame_lo)
    inc hl
    ld (frame_lo),hl
    ld a,h
    or a
    jr nz,timeline_full_scene
    ld a,l
    cp 96
    jr c,timeline_scene_valid
    ld a,1
    ld (frame_hi),a
    xor a
    ld (frame_lo),a
    jr timeline_scene_valid
timeline_full_scene:
    cp 7
    jr c,timeline_scene_valid
    xor a
    ld (frame_lo),a
    ld (frame_hi),a
    ld hl,scroll_message
    ld (scroll_pointer),hl
timeline_scene_valid:
    ld a,(frame_hi)
    ld (scene_marker),a
    call update_layer_levels
    call update_motion
    call update_logo_motion
    ret

; Read two sine velocities a quarter-cycle apart and integrate them into 8.8
; fixed-point accumulators. Their high bytes move the horizontal and vertical
; plasma phases smoothly through every cardinal and diagonal direction. The
; colour phase advances independently at one step per four frames.
update_motion:
    ld a,(motion_angle)
    ld l,a
    ld h,motion_sine_table >> 8
    ld a,(hl)
    ld (motion_velocity_y),a
    ld a,l
    add a,64
    ld l,a
    ld a,(hl)
    ld (motion_velocity_x),a

    ld e,a
    ld d,0
    bit 7,e
    jr z,motion_x_positive
    dec d
motion_x_positive:
motion_x_double_velocity:
    ld hl,(motion_x_accumulator)
    add hl,de
    add hl,de
    ld (motion_x_accumulator),hl
    ld a,h
    ld (phase_horizontal),a

    ld a,(motion_velocity_y)
    ld e,a
    ld d,0
    bit 7,e
    jr z,motion_y_positive
    dec d
motion_y_positive:
motion_y_double_velocity:
    ld hl,(motion_y_accumulator)
    add hl,de
    add hl,de
    ld (motion_y_accumulator),hl
    ld a,h
    ld (phase_vertical),a
    ld c,a
    ld a,(phase_horizontal)
    add a,c
    ld (phase_diagonal),a
    ld a,(phase_horizontal)
    sub c
    ld (phase_cross),a
    ld a,(frame_lo)
    and 1
    jr nz,motion_angle_done
    ld a,(motion_angle)
    inc a
    ld (motion_angle),a
motion_angle_done:
    ld a,(frame_lo)
    and 3
    ret nz
    ld a,(colour_phase)
    inc a
    and 15
    ld (colour_phase),a
    ret

; Move the 40 by 13 logo across its complete legal area. Separate triangle-wave
; phases and rates produce horizontal, vertical and diagonal travel. The high
; product bytes select text-cell coordinates. Bits 4 through 7 of each product's
; low byte provide a four-bit fractional position for development tools. The
; lowest four product bits are discarded.
update_logo_motion:
    ld a,(frame_lo)
    and 1
    jr nz,logo_phase_x_done
    ld a,(logo_phase_x)
    inc a
    ld (logo_phase_x),a
logo_phase_x_done:
    ld a,(logo_y_tick)
    inc a
    cp 3
    jr c,logo_phase_y_tick_store
    xor a
    ld (logo_y_tick),a
    ld a,(logo_phase_y)
    inc a
    ld (logo_phase_y),a
    jr logo_phase_y_done
logo_phase_y_tick_store:
    ld (logo_y_tick),a
logo_phase_y_done:
    ld a,(logo_phase_x)
    ld l,a
    ld h,sine_table >> 8
    ld a,(hl)
    ld l,a
    ld h,0
    ld d,h
    ld e,l
    add hl,hl
    add hl,hl
    add hl,hl
    ld b,h
    ld c,l
    add hl,hl
    add hl,hl
    add hl,bc
    add hl,de
    ld a,l
    rrca
    rrca
    rrca
    rrca
    and 15
    ld (logo_fraction_x),a
    ld a,h
    ld (logo_position_x),a
    cp 40
    jr nz,logo_position_x_done
    xor a
    ld (logo_fraction_x),a
logo_position_x_done:
    ld a,(logo_phase_y)
    ld l,a
    ld h,sine_table >> 8
    ld a,(hl)
    ld l,a
    ld h,0
    ld d,h
    ld e,l
    add hl,hl
    add hl,hl
    ld b,h
    ld c,l
    add hl,hl
    add hl,bc
    add hl,de
    ld a,l
    rrca
    rrca
    rrca
    rrca
    and 15
    ld (logo_fraction_y),a
    ld a,h
    ld (logo_position_y),a
    cp 12
    jr nz,logo_position_y_done
    xor a
    ld (logo_fraction_y),a
logo_position_y_done:
    ret

; Give the plasma and logo independent sixteen-step fades. Their schedules
; overlap for part of the sequence, but at least one main layer remains visible.
; The scroller is drawn after both and is never controlled by these levels.
update_layer_levels:
    ld a,(scene_marker)
    cp 4
    jr c,plasma_full
    jr z,plasma_fade_out
    cp 5
    jr z,plasma_hidden
plasma_fade_in:
    ld a,(frame_lo)
    rrca
    rrca
    rrca
    rrca
    and 15
    inc a
    jr store_plasma_level
plasma_hidden:
    xor a
    jr store_plasma_level
plasma_full:
    ld a,16
    jr store_plasma_level
plasma_fade_out:
    ld a,(frame_lo)
    rrca
    rrca
    rrca
    rrca
    and 15
    ld c,a
    ld a,15
    sub c
store_plasma_level:
    ld (plasma_level),a
    ld a,(scene_marker)
    or a
    jr z,logo_hidden
    cp 1
    jr z,logo_fade_in
    cp 6
    jr c,logo_full
    jr z,logo_fade_out
logo_hidden:
    xor a
    jr store_logo_level
logo_fade_in:
    ld a,(frame_lo)
    rrca
    rrca
    rrca
    rrca
    and 15
    inc a
    jr store_logo_level
logo_full:
    ld a,16
    jr store_logo_level
logo_fade_out:
    ld a,(frame_lo)
    rrca
    rrca
    rrca
    rrca
    and 15
    ld c,a
    ld a,15
    sub c
store_logo_level:
    ld (logo_level),a
    ret

; Load the brightest sixteen-colour palette once. Layer fades use ordered cell
; dithering rather than rewriting the DAC during every frame.
programme_full_palette:
    ld hl,fade_palette+15*48
    xor a
    out (VGA_PORT_DAC_WIDX),a
    ld b,48
palette_write_loop:
    ld a,(hl)
    inc hl
    out (VGA_PORT_DAC_DATA),a
    djnz palette_write_loop
    ret

; ============================================================================
; TEXT-PAGE COMPOSITION
; ============================================================================

; Render all 2,000 cells in row-major order. Horizontal, vertical, diagonal and
; cross waves select a density character. The horizontal and vertical sum plus
; colour_phase selects a foreground/background attribute pair. plasma_level is
; compared with the ordered dither matrix so fades remain spatially distributed.
render_plasma:
    ld de,(render_page_base)
    ld hl,y_phase_table
    ld (row_phase_pointer),hl
    ld a,25
    ld (rows_remaining),a
plasma_row:
    ld hl,(row_phase_pointer)
    ld a,(hl)
    inc hl
    ld (row_phase_pointer),hl
    ld c,a
    ld a,(phase_vertical)
    add a,c
    ld l,a
    ld h,sine_table >> 8
    ld a,(hl)
    ld (vertical_wave),a
    ld a,c
    ld (current_y_phase),a
    ld ix,x_phase_table
    ld a,80
    ld (columns_remaining),a
plasma_column:
    ld a,(current_y_phase)
    and 3
    rlca
    rlca
    ld c,a
    ld a,(ix+0)
    and 3
    or c
    ld l,a
    ld h,dither_table >> 8
    ld a,(hl)
    ld c,a
    ld a,(plasma_level)
    cp c
    jp z,plasma_blank_cell
    jp c,plasma_blank_cell

    ld a,(phase_horizontal)
    add a,(ix+0)
    ld l,a
    ld h,sine_table >> 8
    ld a,(hl)
    ld c,a
    ld a,(vertical_wave)
    add a,c
    ld (hue_wave),a
    and 0xF0
    rrca
    rrca
    rrca
    rrca
    and 15
    ld c,a
    ld a,(colour_phase)
    add a,c
    and 15
    rlca
    rlca
    rlca
    rlca
    and 0xF0
    ld (attribute_high),a

    ld c,(ix+0)
    ld a,(current_y_phase)
    add a,c
    ld c,a
    ld a,(phase_diagonal)
    add a,c
    ld l,a
    ld h,sine_table >> 8
    ld a,(hl)
    ld (diagonal_wave),a
    ld a,(ix+0)
    ld c,a
    ld a,(current_y_phase)
    ld b,a
    ld a,c
    sub b
    ld c,a
    ld a,(phase_cross)
    add a,c
    ld l,a
    ld h,sine_table >> 8
    ld a,(hl)
    ld (cross_wave),a
    ld c,a
    ld a,(diagonal_wave)
    add a,c
    ld c,a
    ld a,(hue_wave)
    add a,c
    rrca
    rrca
    rrca
    rrca
    and 15
    ld l,a
    ld h,0
    ld bc,density_table
    add hl,bc
    ld a,(hl)
    ld (de),a
    inc de

    ld a,(attribute_high)
    ld c,a
    rrca
    rrca
    rrca
    rrca
    and 15
    inc a
    and 15
    or c
    ld (de),a
    inc de
    jr plasma_advance
plasma_blank_cell:
    ld a,' '
    ld (de),a
    inc de
    xor a
    ld (de),a
    inc de
plasma_advance:
    inc ix
    ld a,(columns_remaining)
    dec a
    ld (columns_remaining),a
    jp nz,plasma_column
    ld a,(rows_remaining)
    dec a
    ld (rows_remaining),a
    jp nz,plasma_row
    ret

; Composite the 40 by 13 logo mask at its current cell origin. Mask values 1, 2
; and 3 select edge, interior and shadow treatments. Empty cells preserve the
; plasma below. Ordered dithering applies logo_level to complete cells. The
; renderer uses integer text-cell positions; the fractional position values
; remain available only for development checks.
draw_logo:
    ld a,(logo_level)
    or a
    ret z
    ld ix,logo_mask
    ld hl,0
    ld bc,160
    ld a,(logo_position_y)
logo_origin_row:
    or a
    jr z,logo_origin_column
    add hl,bc
    dec a
    jr logo_origin_row
logo_origin_column:
    ld a,(logo_position_x)
    add a,a
    ld c,a
    ld b,0
    add hl,bc
    ld de,(render_page_base)
    add hl,de
    ex de,hl
    ld a,13
    ld (logo_rows_remaining),a
logo_row:
    ld a,40
    ld (logo_columns_remaining),a
logo_cell:
    ld a,(ix+0)
    inc ix
    or a
    jr z,logo_advance
    ld (logo_cell_kind),a
    ld a,(logo_rows_remaining)
    and 3
    rlca
    rlca
    ld c,a
    ld a,(logo_columns_remaining)
    and 3
    or c
    ld l,a
    ld h,dither_table >> 8
    ld a,(hl)
    ld c,a
    ld a,(logo_level)
    cp c
    jr z,logo_advance
    jr c,logo_advance
    ld a,(logo_cell_kind)
    cp 2
    jr z,logo_interior
    cp 3
    jr z,logo_shadow
    ld a,0xB0
    ld (de),a
    inc de
    ld a,0x0B
    ld (de),a
    dec de
    jr logo_advance
logo_interior:
    ld a,0xDB
    ld (de),a
    inc de
    ld a,(phase_horizontal)
    rrca
    rrca
    rrca
    rrca
    and 7
    or 8
    ld (de),a
    dec de
    jr logo_advance
logo_shadow:
    ld a,0xB1
    ld (de),a
    inc de
    ld a,0x01
    ld (de),a
    dec de
logo_advance:
    inc de
    inc de
    ld a,(logo_columns_remaining)
    dec a
    ld (logo_columns_remaining),a
    jr nz,logo_cell
    ld hl,80
    add hl,de
    ex de,hl
    ld a,(logo_rows_remaining)
    dec a
    ld (logo_rows_remaining),a
    jp nz,logo_row
    ret

; Draw the short hardware credit on row 18 whenever the logo is visible. Its
; cells use the same fade level, so the credit appears and disappears with the
; graphic it describes.
draw_credits:
    ld a,(logo_level)
    or a
    ret z
    ld de,(render_page_base)
    ld hl,18*160+19*2
    add hl,de
    ex de,hl
    ld hl,credit_text
    ld b,42
    ld c,0x0B
credit_cells_loop:
    push hl
    ld a,b
    and 15
    ld l,a
    ld h,dither_table >> 8
    ld a,(logo_level)
    cp (hl)
    pop hl
    jr z,credit_cell_advance
    jr c,credit_cell_advance
    ld a,(hl)
    ld (de),a
    inc de
    ld a,c
    ld (de),a
    dec de
credit_cell_advance:
    inc hl
    inc de
    inc de
    djnz credit_cells_loop
    ret

; The scrolling message always occupies row 22. Eighty consecutive characters
; form the visible window, and the starting pointer advances every four frames.
; A zero byte wraps both the read window and the stored starting pointer.
draw_scroller:
    ld de,(render_page_base)
    ld hl,22*160
    add hl,de
    ex de,hl
    ld hl,(scroll_pointer)
    ld b,80
scroller_loop:
    ld a,(hl)
    or a
    jr nz,scroller_character
    ld hl,scroll_message
    ld a,(hl)
scroller_character:
    inc hl
    ld (de),a
    inc de
    ld a,0xEC
    ld (de),a
    inc de
    djnz scroller_loop
    ld a,(frame_lo)
    and 3
    ret nz
    ld hl,(scroll_pointer)
    inc hl
    ld a,(hl)
    or a
    jr nz,store_scroll_pointer
    ld hl,scroll_message
store_scroll_pointer:
    ld (scroll_pointer),hl
    ret

; ============================================================================
; DEVELOPMENT AND ANIMATION STATE
; ============================================================================

; Keep this block at $0F00. These byte addresses are the development interface
; used by IEScript and IE Mon:
;
;   $0F00  scene_marker          $0F17  displayed_page
;   $0F01  frame_lo              $0F18  plasma_level
;   $0F02  frame_hi              $0F19  logo_level
;   $0F03  phase_horizontal      $0F1B  motion_angle
;   $0F04  phase_vertical        $0F1C  motion_velocity_x
;   $0F05  phase_diagonal        $0F1D  motion_velocity_y
;   $0F06  phase_cross           $0F22  colour_phase
;   $0F11  frame_complete        $0F26  logo_phase_x
;   $0F16  render_page_index     $0F27  logo_phase_y
;                                  $0F28  logo_position_x
;                                  $0F29  logo_position_y
;                                  $0F2A  logo_fraction_x
;                                  $0F2B  logo_fraction_y
;
; frame_complete is the publication counter. displayed_page identifies which
; 4,000-byte text page is visible. Other fields in the block are renderer
; pointers, loop counters, accumulators and temporary wave values.
    .org 0x0F00
scene_marker:
    .byte 0
frame_lo:
    .byte 0
frame_hi:
    .byte 0
phase_horizontal:
    .byte 0
phase_vertical:
    .byte 0
phase_diagonal:
    .byte 0
phase_cross:
    .byte 0
scroll_pointer:
    .word 0
row_phase_pointer:
    .word 0
rows_remaining:
    .byte 0
columns_remaining:
    .byte 0
row_distance:
    .byte 0
vertical_wave:
    .byte 0
current_y_phase:
    .byte 0
attribute_high:
    .byte 0
frame_complete:
    .byte 0
logo_rows_remaining:
    .byte 0
logo_columns_remaining:
    .byte 0
render_page_base:
    .word 0
render_page_index:
    .byte 0
displayed_page:
    .byte 0
plasma_level:
    .byte 0
logo_level:
    .byte 0
logo_cell_kind:
    .byte 0
motion_angle:
    .byte 0
motion_velocity_x:
    .byte 0
motion_velocity_y:
    .byte 0
motion_x_accumulator:
    .word 0
motion_y_accumulator:
    .word 0
colour_phase:
    .byte 0
diagonal_wave:
    .byte 0
hue_wave:
    .byte 0
cross_wave:
    .byte 0
logo_phase_x:
    .byte 0
logo_phase_y:
    .byte 0
logo_position_x:
    .byte 0
logo_position_y:
    .byte 0
logo_fraction_x:
    .byte 0
logo_fraction_y:
    .byte 0
logo_y_tick:
    .byte 0

; ============================================================================
; WAVE, GEOMETRY AND DISPLAY DATA
; ============================================================================

; This page-aligned 256-entry triangle-wave table maps an unsigned phase to an
; unsigned value. Page alignment lets the renderer select an entry by replacing
; only the low byte of the table address.
    .org 0x1000
sine_table:
    .byte 128,130,132,134,136,138,140,142,144,146,148,150,152,154,156,158
    .byte 160,162,164,166,168,170,172,174,176,178,180,182,184,186,188,190
    .byte 192,194,196,198,200,202,204,206,208,210,212,214,216,218,220,222
    .byte 224,226,228,230,232,234,236,238,240,242,244,246,248,250,252,254
    .byte 255,253,251,249,247,245,243,241,239,237,235,233,231,229,227,225
    .byte 223,221,219,217,215,213,211,209,207,205,203,201,199,197,195,193
    .byte 191,189,187,185,183,181,179,177,175,173,171,169,167,165,163,161
    .byte 159,157,155,153,151,149,147,145,143,141,139,137,135,133,131,129
    .byte 127,125,123,121,119,117,115,113,111,109,107,105,103,101,99,97
    .byte 95,93,91,89,87,85,83,81,79,77,75,73,71,69,67,65
    .byte 63,61,59,57,55,53,51,49,47,45,43,41,39,37,35,33
    .byte 31,29,27,25,23,21,19,17,15,13,11,9,7,5,3,1
    .byte 1,3,5,7,9,11,13,15,17,19,21,23,25,27,29,31
    .byte 33,35,37,39,41,43,45,47,49,51,53,55,57,59,61,63
    .byte 65,67,69,71,73,75,77,79,81,83,85,87,89,91,93,95
    .byte 97,99,101,103,105,107,109,111,113,115,117,119,121,123,125,127

; x_phase_table and y_phase_table spread phase offsets across the 80 columns and
; 25 rows. The distance tables document the corresponding centre distances and
; are retained with the geometry data for inspection and further experiments.
    .org 0x1100
x_phase_table:
    .byte 0,5,10,15,20,25,30,35,40,45,50,55,60,65,70,75
    .byte 80,85,90,95,100,105,110,115,120,125,130,135,140,145,150,155
    .byte 160,165,170,175,180,185,190,195,200,205,210,215,220,225,230,235
    .byte 240,245,250,255,4,9,14,19,24,29,34,39,44,49,54,59
    .byte 64,69,74,79,84,89,94,99,104,109,114,119,124,129,134,139
y_phase_table:
    .byte 0,11,22,33,44,55,66,77,88,99,110,121,132,143,154,165
    .byte 176,187,198,209,220,231,242,253,8
x_distance:
    .byte 156,152,148,144,140,136,132,128,124,120,116,112,108,104,100,96
    .byte 92,88,84,80,76,72,68,64,60,56,52,48,44,40,36,32
    .byte 28,24,20,16,12,8,4,0,4,8,12,16,20,24,28,32
    .byte 36,40,44,48,52,56,60,64,68,72,76,80,84,88,92,96
    .byte 100,104,108,112,116,120,124,128,132,136,140,144,148,152,156,160
y_distance:
    .byte 84,77,70,63,56,49,42,35,28,21,14,7,0,7,14,21
    .byte 28,35,42,49,56,63,70,77,84
density_table:
    .byte 32,176,176,177,177,178,178,178,219,219,219,178,177,177,176,32
credit_text:
    .byte "      Z80 JIT  |  VGA TEXT  |  POKEY SAP      "
scroll_message:
    .byte "   WELCOME TO THE INTUITION ENGINE TEXTMODE PLASMA. FOUR WAVES, SIXTEEN COLOURS, ONE Z80 AND AN ATARI POKEY SAP SOUNDTRACK.   "
    .byte "Hobbytronic '92 - 2 by Stephan Duesterhoeft (Benjy).   "
    .byte "MODE 03H IS NOT A LIMIT. IT IS A PALETTE, A FONT AND 2,000 LITTLE PIXEL CANVASES.   "
    .byte "CODE AND DESIGN BY ZAYN OTLEY. KEEP THE OLD MACHINES DREAMING.   ",0

; Each logo byte describes one text cell: 0 is transparent, 1 is an edge, 2 is
; the solid interior and 3 is a shadow. The mask is 40 cells wide by 13 high.
logo_mask:
    .byte 0,1,2,2,2,1,2,1,2,1,2,2,2,1,2,1,2,1,2,2,2,1,2,2,2,1,2,2,2,1,2,2,2,1,2,1,2,1,0,0
    .byte 0,1,1,2,3,3,2,2,2,3,1,2,3,3,2,3,2,3,1,2,3,3,1,2,3,3,1,2,3,3,2,3,2,3,2,2,2,3,0,0
    .byte 0,0,1,2,3,1,2,2,2,3,1,2,3,1,2,3,2,3,1,2,3,0,1,2,3,0,1,2,3,1,2,3,2,3,2,2,2,3,0,0
    .byte 0,1,1,2,3,1,2,2,2,3,1,2,3,1,2,3,2,3,1,2,3,1,1,2,3,1,1,2,3,1,2,3,2,3,2,2,2,3,0,0
    .byte 0,1,2,2,2,1,2,3,2,3,1,2,3,1,2,2,2,3,2,2,2,1,1,2,3,1,2,2,2,1,2,2,2,3,2,3,2,3,0,0
    .byte 0,1,1,3,3,3,1,3,1,3,1,1,3,1,1,3,3,3,1,3,3,3,1,1,3,1,1,3,3,3,1,3,3,3,1,3,1,3,0,0
    .byte 0,0,0,0,0,0,0,1,2,2,2,1,2,1,2,1,2,2,2,1,2,2,2,1,2,1,2,1,2,2,2,1,0,0,0,0,0,0,0,0
    .byte 0,0,0,0,0,0,0,1,2,3,3,3,2,2,2,3,2,3,3,3,1,2,3,3,2,2,2,3,2,3,3,3,0,0,0,0,0,0,0,0
    .byte 0,0,0,0,0,0,0,1,2,2,1,1,2,2,2,3,2,3,2,1,1,2,3,1,2,2,2,3,2,2,1,0,0,0,0,0,0,0,0,0
    .byte 0,0,0,0,0,0,0,1,2,3,3,1,2,2,2,3,2,3,2,3,1,2,3,1,2,2,2,3,2,3,3,1,0,0,0,0,0,0,0,0
    .byte 0,0,0,0,0,0,0,1,2,2,2,1,2,3,2,3,2,2,2,3,2,2,2,1,2,3,2,3,2,2,2,1,0,0,0,0,0,0,0,0
    .byte 0,0,0,0,0,0,0,1,1,3,3,3,1,3,1,3,1,3,3,3,1,3,3,3,1,3,1,3,1,3,3,3,0,0,0,0,0,0,0,0
    .byte 0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0

; Signed sine velocities for plasma movement. Unlike the unsigned triangle-wave
; table, this table is centred on zero because update_motion integrates its
; values into positions.
    .org 0x1600
motion_sine_table:
    .byte 0,3,6,9,12,16,19,22,25,28,31,34,37,40,43,46
    .byte 49,51,54,57,60,63,65,68,71,73,76,78,81,83,85,88
    .byte 90,92,94,96,98,100,102,104,106,107,109,111,112,113,115,116
    .byte 117,118,120,121,122,122,123,124,125,125,126,126,126,127,127,127
    .byte 127,127,127,127,126,126,126,125,125,124,123,122,122,121,120,118
    .byte 117,116,115,113,112,111,109,107,106,104,102,100,98,96,94,92
    .byte 90,88,85,83,81,78,76,73,71,68,65,63,60,57,54,51
    .byte 49,46,43,40,37,34,31,28,25,22,19,16,12,9,6,3
    .byte 0,-3,-6,-9,-12,-16,-19,-22,-25,-28,-31,-34,-37,-40,-43,-46
    .byte -49,-51,-54,-57,-60,-63,-65,-68,-71,-73,-76,-78,-81,-83,-85,-88
    .byte -90,-92,-94,-96,-98,-100,-102,-104,-106,-107,-109,-111,-112,-113,-115,-116
    .byte -117,-118,-120,-121,-122,-122,-123,-124,-125,-125,-126,-126,-126,-127,-127,-127
    .byte -127,-127,-127,-127,-126,-126,-126,-125,-125,-124,-123,-122,-122,-121,-120,-118
    .byte -117,-116,-115,-113,-112,-111,-109,-107,-106,-104,-102,-100,-98,-96,-94,-92
    .byte -90,-88,-85,-83,-81,-78,-76,-73,-71,-68,-65,-63,-60,-57,-54,-51
    .byte -49,-46,-43,-40,-37,-34,-31,-28,-25,-22,-19,-16,-12,-9,-6,-3

; A 4 by 4 Bayer matrix supplies thresholds from 0 to 15 for plasma and logo
; fades. The same ordered pattern makes animation deterministic between runs.
    .org 0x1700
dither_table:
    .byte 0,8,2,10
    .byte 12,4,14,6
    .byte 3,11,1,9
    .byte 15,7,13,5

; Sixteen palette levels are stored as 16 VGA RGB triplets per level. Start-up
; reads only the final 48-byte level. The earlier levels are retained data and
; are not read by the current programme.
    .org 0x1800
fade_palette:
    .byte 0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
    .byte 0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
    .byte 0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0
    .byte 0,0,0,0,0,1,0,0,2,0,0,2,1,0,3,2
    .byte 0,3,3,1,3,4,1,2,4,2,1,4,3,1,4,4
    .byte 3,3,4,4,1,3,4,1,2,4,0,1,3,0,0,1
    .byte 0,0,0,0,0,2,0,1,3,1,1,5,2,1,6,4
    .byte 1,6,6,1,6,7,2,4,8,4,2,8,6,3,8,8
    .byte 6,5,8,8,2,7,8,1,4,8,1,2,5,0,1,3
    .byte 0,0,0,0,1,2,0,1,5,1,1,7,3,1,9,6
    .byte 1,10,8,2,8,11,3,6,13,6,4,13,9,4,13,12
    .byte 8,8,13,12,3,10,13,2,6,12,1,3,8,0,1,4
    .byte 0,0,0,0,1,3,1,2,6,2,1,10,4,1,12,7
    .byte 1,13,11,2,11,15,4,8,17,7,5,17,12,6,17,16
    .byte 11,10,17,16,4,13,17,2,9,15,1,4,11,1,2,5
    .byte 0,0,0,0,1,4,1,2,8,2,2,12,5,1,15,9
    .byte 2,16,14,3,14,18,5,10,21,9,6,21,15,7,21,20
    .byte 14,13,21,20,5,17,21,3,11,19,2,5,13,1,2,7
    .byte 0,0,0,0,1,5,1,2,10,3,2,14,6,2,18,11
    .byte 2,19,17,3,17,22,6,12,25,11,7,25,18,9,25,24
    .byte 17,15,25,24,6,20,25,3,13,23,2,6,16,1,3,8
    .byte 0,0,0,0,1,6,1,3,11,3,2,17,7,2,21,13
    .byte 2,22,20,4,20,26,7,14,29,13,8,29,21,10,29,28
    .byte 20,18,29,28,7,23,29,4,15,27,2,7,19,1,3,9
    .byte 0,0,0,0,2,6,1,3,13,4,3,19,9,2,25,15
    .byte 3,26,22,4,22,29,8,16,34,15,10,34,24,12,34,32
    .byte 22,20,34,32,9,27,34,4,17,31,3,9,21,1,4,11
    .byte 0,0,0,0,2,7,1,4,14,4,3,22,10,2,28,17
    .byte 3,29,25,5,25,33,9,18,38,17,11,38,27,13,38,36
    .byte 25,23,38,36,10,30,38,5,19,35,3,10,24,1,4,12
    .byte 0,0,0,0,2,8,1,4,16,5,3,24,11,3,31,19
    .byte 3,32,28,5,28,37,10,20,42,19,12,42,30,15,42,40
    .byte 28,25,42,40,11,33,42,5,21,39,3,11,27,1,5,13
    .byte 0,0,0,0,2,9,1,4,18,5,4,26,12,3,34,21
    .byte 4,35,31,6,31,40,11,22,46,21,13,46,33,16,46,44
    .byte 31,28,46,44,12,37,46,6,23,43,4,12,29,1,5,15
    .byte 0,0,0,0,2,10,2,5,19,6,4,29,13,3,37,22
    .byte 4,38,34,6,34,44,12,24,50,22,14,50,36,18,50,48
    .byte 34,30,50,48,13,40,50,6,26,46,4,13,32,2,6,16
    .byte 0,0,0,0,3,10,2,5,21,6,4,31,14,3,40,24
    .byte 4,42,36,7,36,48,13,26,55,24,16,55,39,19,55,52
    .byte 36,33,55,52,14,43,55,7,28,50,4,14,35,2,6,17
    .byte 0,0,0,0,3,11,2,6,22,7,5,34,15,4,43,26
    .byte 5,45,39,7,39,51,14,28,59,26,17,59,42,21,59,56
    .byte 39,35,59,56,15,47,59,7,30,54,5,15,37,2,7,19
    .byte 0,0,0,0,3,12,2,6,24,7,5,36,16,4,46,28
    .byte 5,48,42,8,42,55,15,30,63,28,18,63,45,22,63,60
    .byte 42,38,63,60,16,50,63,8,32,58,5,16,40,2,7,20

; Embed the SAP asset unchanged in the IE80 image. The assembly-time labels
; provide the exact length written to the SAP player registers.
    .org 0xE000

sap_data:
    .incbin "../assets/music/Hobbytronic_92_2.sap"
sap_data_end:
