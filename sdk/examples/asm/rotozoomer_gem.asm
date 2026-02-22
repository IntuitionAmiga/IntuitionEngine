; ============================================================================
; GEM WINDOWED ROTOZOOMER - TOS .PRG Application for EmuTOS
; M68020 Assembly for IntuitionEngine - GEM Application Programming
; ============================================================================
;
; A proper TOS .PRG that opens a GEM window under EmuTOS and renders
; a rotating, zooming checkerboard texture inside it using the IE
; hardware Mode7 blitter.
;
; Build:
;   vasmm68k_mot -Ftos -m68020 -devpac -Isdk/include \
;     -o sdk/examples/asm/rotozoomer_gem.prg sdk/examples/asm/rotozoomer_gem.asm
;
; Run:
;   mkdir -p /tmp/tos_drive
;   cp sdk/examples/asm/rotozoomer_gem.prg /tmp/tos_drive/ROTOZOOM.PRG
;   ./bin/IntuitionEngine -emutos -emutos-drive /tmp/tos_drive/
;   (Navigate to drive U: in GEM desktop, double-click ROTOZOOM.PRG)
;
; ============================================================================

                include "ie68.inc"

; ============================================================================
; GEM / AES / VDI CONSTANTS
; ============================================================================

; AES opcodes
AES_APPL_INIT   equ 10
AES_APPL_EXIT   equ 19
AES_EVNT_MULTI  equ 25
AES_WIND_CREATE equ 100
AES_WIND_OPEN   equ 101
AES_WIND_CLOSE  equ 102
AES_WIND_DELETE equ 103
AES_WIND_GET    equ 104
AES_WIND_SET    equ 105
AES_WIND_UPDATE equ 107
AES_GRAF_HANDLE equ 77

; VDI opcodes
VDI_OPNVWK      equ 100
VDI_CLSVWK      equ 101

; Wind flags
WF_NAME         equ 2
WF_WORKXYWH     equ 4
WF_CURRXYWH     equ 6
WF_FIRSTXYWH    equ 11
WF_NEXTXYWH     equ 12

; Window components
NAME            equ $01
CLOSER          equ $02
MOVER           equ $04

; Event types
MU_MESAG        equ $10
MU_TIMER        equ $20

; Message types
WM_REDRAW       equ 20
WM_TOPPED       equ 21
WM_CLOSED       equ 22
WM_MOVED        equ 28

; wind_update modes
BEG_UPDATE      equ 1
END_UPDATE      equ 0

; ============================================================================
; APPLICATION CONSTANTS
; ============================================================================

TEXTURE_BASE    equ $600000
TEX_STRIDE      equ 1024            ; 256 * 4 bytes per pixel
VRAM_STRIDE     equ 2560            ; 640 * 4 bytes per pixel

ANGLE_INC       equ 313             ; Rotation speed (8.8 fixed-point)
SCALE_INC       equ 104             ; Zoom speed (8.8 fixed-point)

; ============================================================================
; TOS .PRG STARTUP (crt0)
; ============================================================================

                text

start:
                ; --- Read basepage pointer from stack ---
                ; TOS passes a pointer to the basepage at 4(sp) on program start
                move.l  4(sp),a5                ; a5 = basepage pointer

                ; --- Mshrink: release unused TPA memory ---
                ; basepage+$18 = p_hitpa (end of TPA)
                ; basepage+$100 = start of TPA (basepage itself)
                move.l  $18(a5),d0              ; d0 = p_hitpa (end of TPA)
                sub.l   a5,d0                   ; d0 = size of memory we need
                move.l  d0,-(sp)                ; new size
                move.l  a5,-(sp)                ; start address
                clr.w   -(sp)                   ; dummy
                move.w  #$4A,-(sp)              ; Mshrink
                trap    #1
                lea     12(sp),sp

                ; --- Set up local stack ---
                lea     stack_end(pc),sp

                ; --- Main application ---
                bsr     gem_init
                tst.w   d0
                bmi     exit_no_gem

                bsr     generate_texture
                bsr     start_music
                bsr     open_window
                tst.w   d0
                bmi     exit_stop_music

                bsr     event_loop

                bsr     stop_music
                bsr     close_window
exit_stop_music:
                bsr     stop_music
exit_close_vdi:
                bsr     gem_exit
exit_no_gem:
                ; --- Pterm0: exit to desktop ---
                clr.w   -(sp)
                trap    #1

; ============================================================================
; GEM INITIALIZATION
; ============================================================================

gem_init:
                ; --- appl_init ---
                move.w  #AES_APPL_INIT,aes_control
                move.w  #0,aes_control+2        ; #intin
                move.w  #1,aes_control+4        ; #intout
                move.w  #0,aes_control+6        ; #addrin
                move.w  #0,aes_control+8        ; #addrout
                bsr     aes_call
                move.w  aes_intout,d0
                move.w  d0,ap_id
                tst.w   d0
                bmi     .fail

                ; --- graf_handle ---
                move.w  #AES_GRAF_HANDLE,aes_control
                move.w  #0,aes_control+2
                move.w  #5,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                bsr     aes_call
                move.w  aes_intout,d0
                move.w  d0,vdi_handle

                ; --- v_opnvwk: open virtual workstation ---
                ; Set up work_in array (11 words)
                lea     work_in(pc),a0
                move.w  #1,(a0)                 ; device ID
                move.w  #1,2(a0)                ; line type
                move.w  #1,4(a0)                ; line color
                move.w  #1,6(a0)                ; marker type
                move.w  #1,8(a0)                ; marker color
                move.w  #1,10(a0)               ; font ID
                move.w  #1,12(a0)               ; text color
                move.w  #1,14(a0)               ; fill interior
                move.w  #1,16(a0)               ; fill style
                move.w  #1,18(a0)               ; fill color
                move.w  #2,20(a0)               ; use RC coordinate system

                ; VDI contrl: opcode=100, ptsin=0, intin=11, sub=0
                move.w  #VDI_OPNVWK,vdi_contrl
                move.w  #0,vdi_contrl+2         ; ptsin count
                move.w  #0,vdi_contrl+4         ; ptsout count
                move.w  #11,vdi_contrl+6        ; intin count
                move.w  #0,vdi_contrl+8         ; intout count
                move.w  #0,vdi_contrl+10        ; sub-opcode

                ; Copy work_in to vdi_intin
                lea     work_in(pc),a0
                lea     vdi_intin,a1
                moveq   #10,d0
.copy_workin:   move.w  (a0)+,(a1)+
                dbra    d0,.copy_workin

                ; Set VDI handle
                move.w  vdi_handle,vdi_contrl+12

                bsr     vdi_call

                ; vdi_handle may be updated by v_opnvwk
                move.w  vdi_contrl+12,vdi_handle

                moveq   #0,d0                   ; success
                rts
.fail:
                moveq   #-1,d0
                rts

; ============================================================================
; OPEN WINDOW
; ============================================================================

open_window:
                ; --- wind_get(0, WF_WORKXYWH) to get desktop dimensions ---
                move.w  #AES_WIND_GET,aes_control
                move.w  #2,aes_control+2        ; #intin
                move.w  #5,aes_control+4        ; #intout
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  #0,aes_intin             ; handle 0 = desktop
                move.w  #WF_WORKXYWH,aes_intin+2
                bsr     aes_call

                ; intout: [0]=return, [1]=dx, [2]=dy, [3]=dw, [4]=dh
                move.w  aes_intout+2,d0          ; desktop x
                move.w  aes_intout+4,d1          ; desktop y
                move.w  aes_intout+6,d2          ; desktop w
                move.w  aes_intout+8,d3          ; desktop h

                ; Use a 320x240 window centred on the desktop
                move.w  #320,d4                  ; window width
                move.w  #240,d5                  ; window height

                ; Centre: x = dx + (dw - 320)/2, y = dy + (dh - 240)/2
                move.w  d2,d6
                sub.w   d4,d6
                asr.w   #1,d6
                add.w   d0,d6                    ; d6 = window x
                move.w  d6,win_x

                move.w  d3,d7
                sub.w   d5,d7
                asr.w   #1,d7
                add.w   d1,d7                    ; d7 = window y
                move.w  d7,win_y

                move.w  d4,win_w
                move.w  d5,win_h

                ; --- wind_create(NAME|CLOSER|MOVER, x, y, w, h) ---
                move.w  #AES_WIND_CREATE,aes_control
                move.w  #5,aes_control+2
                move.w  #1,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  #NAME|CLOSER|MOVER,aes_intin
                move.w  win_x,aes_intin+2
                move.w  win_y,aes_intin+4
                move.w  win_w,aes_intin+6
                move.w  win_h,aes_intin+8
                bsr     aes_call
                move.w  aes_intout,win_handle
                tst.w   aes_intout
                bmi     .fail

                ; --- wind_set(handle, WF_NAME, title) ---
                move.w  #AES_WIND_SET,aes_control
                move.w  #6,aes_control+2
                move.w  #1,aes_control+4
                move.w  #1,aes_control+6        ; 1 addrin
                move.w  #0,aes_control+8
                move.w  win_handle,aes_intin
                move.w  #WF_NAME,aes_intin+2
                ; Pass title pointer via intin[2..3] (high/low words)
                lea     win_title(pc),a0
                move.l  a0,d0
                swap    d0
                move.w  d0,aes_intin+4           ; high word
                swap    d0
                move.w  d0,aes_intin+6           ; low word
                bsr     aes_call

                ; --- wind_open(handle, x, y, w, h) ---
                move.w  #AES_WIND_OPEN,aes_control
                move.w  #5,aes_control+2
                move.w  #1,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  win_handle,aes_intin
                move.w  win_x,aes_intin+2
                move.w  win_y,aes_intin+4
                move.w  win_w,aes_intin+6
                move.w  win_h,aes_intin+8
                bsr     aes_call

                ; Initialize animation state
                clr.l   angle_accum
                clr.l   scale_accum

                moveq   #0,d0
                rts
.fail:
                moveq   #-1,d0
                rts

; ============================================================================
; CLOSE WINDOW
; ============================================================================

close_window:
                move.w  #AES_WIND_CLOSE,aes_control
                move.w  #1,aes_control+2
                move.w  #1,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  win_handle,aes_intin
                bsr     aes_call

                move.w  #AES_WIND_DELETE,aes_control
                move.w  #1,aes_control+2
                move.w  #1,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  win_handle,aes_intin
                bsr     aes_call
                rts

; ============================================================================
; GEM EXIT
; ============================================================================

gem_exit:
                ; --- v_clsvwk ---
                move.w  #VDI_CLSVWK,vdi_contrl
                move.w  #0,vdi_contrl+2
                move.w  #0,vdi_contrl+4
                move.w  #0,vdi_contrl+6
                move.w  #0,vdi_contrl+8
                move.w  #0,vdi_contrl+10
                move.w  vdi_handle,vdi_contrl+12
                bsr     vdi_call

                ; --- appl_exit ---
                move.w  #AES_APPL_EXIT,aes_control
                move.w  #0,aes_control+2
                move.w  #1,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                bsr     aes_call
                rts

; ============================================================================
; EVENT LOOP (evnt_multi)
; ============================================================================

event_loop:
.loop:
                ; evnt_multi: MU_MESAG | MU_TIMER
                move.w  #AES_EVNT_MULTI,aes_control
                move.w  #16,aes_control+2       ; #intin
                move.w  #7,aes_control+4        ; #intout
                move.w  #1,aes_control+6        ; #addrin
                move.w  #0,aes_control+8        ; #addrout

                move.w  #MU_MESAG|MU_TIMER,aes_intin  ; ev_flags
                move.w  #0,aes_intin+2          ; mx1 (unused)
                move.w  #0,aes_intin+4          ; my1
                move.w  #0,aes_intin+6          ; mw1
                move.w  #0,aes_intin+8          ; mh1
                move.w  #0,aes_intin+10         ; m1_flag
                move.w  #0,aes_intin+12         ; mx2
                move.w  #0,aes_intin+14         ; my2
                move.w  #0,aes_intin+16         ; mw2
                move.w  #0,aes_intin+18         ; mh2
                move.w  #0,aes_intin+20         ; m2_flag
                move.w  #0,aes_intin+22         ; bclicks
                move.w  #0,aes_intin+24         ; bmask
                move.w  #0,aes_intin+26         ; bstate
                move.w  #16,aes_intin+28        ; locount (16ms = ~60fps)
                move.w  #0,aes_intin+30         ; hicount

                ; addrin[0] = message buffer
                lea     msg_buf(pc),a0
                move.l  a0,aes_addrin
                bsr     aes_call

                ; Check which event occurred
                move.w  aes_intout,d0            ; ev_flags returned

                ; Timer event: animate and render
                btst    #5,d0                    ; MU_TIMER = bit 5
                beq.s   .check_msg
                bsr     animate_frame

.check_msg:
                btst    #4,d0                    ; MU_MESAG = bit 4
                beq     .loop

                ; Process message
                move.w  msg_buf,d0
                cmpi.w  #WM_CLOSED,d0
                beq     .done

                cmpi.w  #WM_MOVED,d0
                beq.s   .handle_moved

                cmpi.w  #WM_REDRAW,d0
                beq.s   .handle_redraw

                bra     .loop

.handle_moved:
                ; msg_buf+8/10/12/14 = new x/y/w/h
                move.w  #AES_WIND_SET,aes_control
                move.w  #6,aes_control+2
                move.w  #1,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  win_handle,aes_intin
                move.w  #WF_CURRXYWH,aes_intin+2
                move.w  msg_buf+8,aes_intin+4
                move.w  msg_buf+10,aes_intin+6
                move.w  msg_buf+12,aes_intin+8
                move.w  msg_buf+14,aes_intin+10
                bsr     aes_call
                bra     .loop

.handle_redraw:
                bsr     do_redraw
                bra     .loop

.done:
                rts

; ============================================================================
; ANIMATE FRAME - Timer-driven rendering
; ============================================================================

animate_frame:
                movem.l d0-d7/a0-a2,-(sp)

                ; wind_update(BEG_UPDATE)
                bsr     wind_update_begin

                ; Get current work area
                bsr     get_work_area
                ; Returns: d0=wx, d1=wy, d2=ww, d3=wh
                tst.w   d2
                beq     .done                    ; zero-width window, skip
                tst.w   d3
                beq     .done

                ; Compute frame parameters
                bsr     compute_frame

                ; Render Mode7 into window work area
                ; d0=wx, d1=wy, d2=ww, d3=wh already set from get_work_area
                bsr     render_window

                ; Advance animation
                bsr     advance_animation

.done:
                ; wind_update(END_UPDATE)
                bsr     wind_update_end

                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; DO_REDRAW - WM_REDRAW handler with rectangle list clipping
; ============================================================================

do_redraw:
                movem.l d0-d7/a0-a2,-(sp)

                bsr     wind_update_begin

                ; Iterate the visible rectangle list
                ; wind_get(handle, WF_FIRSTXYWH)
                move.w  #AES_WIND_GET,aes_control
                move.w  #2,aes_control+2
                move.w  #5,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  win_handle,aes_intin
                move.w  #WF_FIRSTXYWH,aes_intin+2
                bsr     aes_call

.rect_loop:
                ; intout: [1]=rx, [2]=ry, [3]=rw, [4]=rh
                move.w  aes_intout+2,d4         ; rx
                move.w  aes_intout+4,d5         ; ry
                move.w  aes_intout+6,d6         ; rw
                move.w  aes_intout+8,d7         ; rh

                ; If rw=0 and rh=0, done
                tst.w   d6
                bne.s   .have_rect
                tst.w   d7
                beq     .redraw_done

.have_rect:
                ; Get work area for intersection calculation
                bsr     get_work_area
                ; d0=wx, d1=wy, d2=ww, d3=wh

                ; Compute intersection of work area and visible rect
                ; Clip rect to work area bounds
                ; clip_x = max(rx, wx)
                cmp.w   d4,d0
                bge.s   .cx_ok
                move.w  d4,d0
.cx_ok:
                ; clip_y = max(ry, wy)
                cmp.w   d5,d1
                bge.s   .cy_ok
                move.w  d5,d1
.cy_ok:
                ; clip_right = min(rx+rw, wx+ww)
                move.w  d4,a0                    ; use a0 as temp
                add.w   d6,d4                    ; rx+rw
                move.w  a0,d6                    ; restore rx to d6 (not needed after)

                bsr     get_work_area
                move.w  d0,d6                    ; wx
                add.w   d2,d6                    ; wx+ww = work right edge

                cmp.w   d6,d4
                ble.s   .cr_ok
                move.w  d6,d4                    ; clip to work right
.cr_ok:
                ; clip_bottom = min(ry+rh, wy+wh)
                move.w  d1,d6                    ; wy
                add.w   d3,d6                    ; wy+wh = work bottom edge

                add.w   d7,d5                    ; ry+rh
                cmp.w   d6,d5
                ble.s   .cb_ok
                move.w  d6,d5                    ; clip to work bottom
.cb_ok:
                ; d0=clip_x, d1=clip_y, d4=clip_right, d5=clip_bottom
                ; Compute clipped w/h
                sub.w   d0,d4                    ; clip_w = right - x
                sub.w   d1,d5                    ; clip_h = bottom - y
                ble.s   .next_rect               ; skip if empty
                tst.w   d4
                ble.s   .next_rect

                ; Render this clipped rectangle
                move.w  d4,d2                    ; w = clip_w
                move.w  d5,d3                    ; h = clip_h
                ; d0=x, d1=y, d2=w, d3=h
                bsr     render_window

.next_rect:
                ; wind_get(handle, WF_NEXTXYWH)
                move.w  #AES_WIND_GET,aes_control
                move.w  #2,aes_control+2
                move.w  #5,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  win_handle,aes_intin
                move.w  #WF_NEXTXYWH,aes_intin+2
                bsr     aes_call
                bra     .rect_loop

.redraw_done:
                bsr     wind_update_end
                movem.l (sp)+,d0-d7/a0-a2
                rts

; ============================================================================
; GET_WORK_AREA - Returns work area in d0-d3
; ============================================================================

get_work_area:
                movem.l a0-a1,-(sp)
                move.w  #AES_WIND_GET,aes_control
                move.w  #2,aes_control+2
                move.w  #5,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  win_handle,aes_intin
                move.w  #WF_WORKXYWH,aes_intin+2
                bsr     aes_call
                move.w  aes_intout+2,d0         ; wx
                move.w  aes_intout+4,d1         ; wy
                move.w  aes_intout+6,d2         ; ww
                move.w  aes_intout+8,d3         ; wh
                movem.l (sp)+,a0-a1
                rts

; ============================================================================
; WIND_UPDATE helpers
; ============================================================================

wind_update_begin:
                move.w  #AES_WIND_UPDATE,aes_control
                move.w  #1,aes_control+2
                move.w  #1,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  #BEG_UPDATE,aes_intin
                bra     aes_call

wind_update_end:
                move.w  #AES_WIND_UPDATE,aes_control
                move.w  #1,aes_control+2
                move.w  #1,aes_control+4
                move.w  #0,aes_control+6
                move.w  #0,aes_control+8
                move.w  #END_UPDATE,aes_intin
                bra     aes_call

; ============================================================================
; RENDER WINDOW - Mode7 blit into a screen rectangle
; ============================================================================
; Input: d0=x, d1=y, d2=w, d3=h (screen coordinates, pixel units)
; Uses var_ca, var_sa, var_u0, var_v0 from compute_frame

render_window:
                movem.l d0-d7,-(sp)

                ; Clamp dimensions to sensible range
                tst.w   d2
                ble     .rw_done
                tst.w   d3
                ble     .rw_done

                ; Compute destination address in VRAM
                ; dst = VRAM_START + y * VRAM_STRIDE + x * 4
                ext.l   d1                       ; sign-extend y
                muls.w  #VRAM_STRIDE,d1          ; y * stride
                ext.l   d0                       ; sign-extend x
                lsl.l   #2,d0                    ; x * 4
                add.l   d0,d1                    ; y*stride + x*4
                add.l   #VRAM_START,d1           ; absolute VRAM address
                move.l  d1,d4                    ; d4 = dst addr

                ; Compute u0/v0 offset for the sub-rectangle within the work area.
                ; For the full work area render, work_ox=0 and work_oy=0.
                ; For clipped sub-rectangles during WM_REDRAW, the caller passes
                ; the clipped x,y as d0,d1 which are screen coordinates.
                ; The work area origin is cached by compute_frame, and u0/v0
                ; are relative to screen (0,0), so the Mode7 blitter naturally
                ; handles the offset — we just set the correct dst and dims.

                ; Set up Mode7 blitter
                move.l  #BLT_OP_MODE7,BLT_OP
                move.l  #TEXTURE_BASE,BLT_SRC
                move.l  d4,BLT_DST

                ; Width and height (zero-extend from 16-bit)
                andi.l  #$FFFF,d2
                andi.l  #$FFFF,d3
                move.l  d2,BLT_WIDTH
                move.l  d3,BLT_HEIGHT

                move.l  #TEX_STRIDE,BLT_SRC_STRIDE
                move.l  #VRAM_STRIDE,BLT_DST_STRIDE

                move.l  #255,BLT_MODE7_TEX_W
                move.l  #255,BLT_MODE7_TEX_H

                ; Get u0/v0 adjusted for the render rectangle's screen position.
                ; u0_screen = base_u0 + CA * screen_x - SA * screen_y
                ; v0_screen = base_v0 + SA * screen_x + CA * screen_y
                ; where screen_x and screen_y are the top-left pixel of the rect.
                move.l  (sp),d0                  ; restore original x from stack
                ext.l   d0
                move.l  4(sp),d1                 ; restore original y from stack
                ext.l   d1

                move.l  var_ca,d5
                move.l  var_sa,d6

                ; CA * x  (shift decomposition not needed for small window coords)
                move.l  d5,d7
                muls.w  d0,d7                    ; CA * x (fits in 32 bits for window coords)
                move.l  var_u0,d4
                add.l   d7,d4                    ; + CA*x

                move.l  d6,d7
                muls.w  d1,d7                    ; SA * y
                sub.l   d7,d4                    ; - SA*y
                move.l  d4,BLT_MODE7_U0

                ; v0 = base_v0 + SA*x + CA*y
                move.l  d6,d7
                muls.w  d0,d7                    ; SA * x
                move.l  var_v0,d4
                add.l   d7,d4                    ; + SA*x

                move.l  d5,d7
                muls.w  d1,d7                    ; CA * y
                add.l   d7,d4                    ; + CA*y
                move.l  d4,BLT_MODE7_V0

                ; du_col = CA, dv_col = SA, du_row = -SA, dv_row = CA
                move.l  d5,BLT_MODE7_DU_COL
                move.l  d6,BLT_MODE7_DV_COL
                neg.l   d6
                move.l  d6,BLT_MODE7_DU_ROW
                move.l  d5,BLT_MODE7_DV_ROW

                ; Trigger blit
                move.l  #1,BLT_CTRL

                ; Wait for completion
.wait:          move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .wait

.rw_done:
                movem.l (sp)+,d0-d7
                rts

; ============================================================================
; COMPUTE FRAME - Calculate Mode7 Parameters
; ============================================================================
; Same algorithm as the bare-metal rotozoomer, but u0/v0 centering uses
; the screen centre (320,240) rather than the window work area, because
; the render_window routine adjusts u0/v0 per-call for the actual
; screen position.

compute_frame:
                movem.l d0-d7/a0-a1,-(sp)

                ; Extract table indices from 8.8 accumulators
                move.l  angle_accum,d0
                lsr.l   #8,d0
                andi.l  #255,d0

                move.l  scale_accum,d1
                lsr.l   #8,d1
                andi.l  #255,d1

                ; cos(angle) = sin(angle + 64)
                move.l  d0,d2
                addi.l  #64,d2
                andi.l  #255,d2
                add.l   d2,d2
                lea     sine_table(pc),a0
                move.w  (a0,d2.l),d3
                ext.l   d3

                ; sin(angle)
                move.l  d0,d2
                add.l   d2,d2
                move.w  (a0,d2.l),d4
                ext.l   d4

                ; reciprocal zoom factor
                move.l  d1,d2
                add.l   d2,d2
                lea     recip_table(pc),a1
                move.w  (a1,d2.l),d5
                andi.l  #$FFFF,d5

                ; CA = cos * recip, SA = sin * recip
                move.l  d3,d6
                muls.w  d5,d6
                move.l  d4,d7
                muls.w  d5,d7

                move.l  d6,var_ca
                move.l  d7,var_sa

                ; u0 = 8388608 - CA*320 + SA*240
                ; Using shift decomposition for 320 and 240
                move.l  d6,d0
                move.l  d0,d1
                lsl.l   #8,d0
                lsl.l   #6,d1
                add.l   d1,d0                    ; CA * 320

                move.l  d7,d1
                move.l  d1,d2
                lsl.l   #8,d1
                lsl.l   #4,d2
                sub.l   d2,d1                    ; SA * 240

                move.l  #$800000,d3
                sub.l   d0,d3
                add.l   d1,d3
                move.l  d3,var_u0

                ; v0 = 8388608 - SA*320 - CA*240
                move.l  d7,d0
                move.l  d0,d1
                lsl.l   #8,d0
                lsl.l   #6,d1
                add.l   d1,d0                    ; SA * 320

                move.l  d6,d1
                move.l  d1,d2
                lsl.l   #8,d1
                lsl.l   #4,d2
                sub.l   d2,d1                    ; CA * 240

                move.l  #$800000,d3
                sub.l   d0,d3
                sub.l   d1,d3
                move.l  d3,var_v0

                movem.l (sp)+,d0-d7/a0-a1
                rts

; ============================================================================
; ADVANCE ANIMATION
; ============================================================================

advance_animation:
                move.l  angle_accum,d0
                addi.l  #ANGLE_INC,d0
                andi.l  #$FFFF,d0
                move.l  d0,angle_accum

                move.l  scale_accum,d0
                addi.l  #SCALE_INC,d0
                andi.l  #$FFFF,d0
                move.l  d0,scale_accum
                rts

; ============================================================================
; AHX MUSIC PLAYBACK
; ============================================================================

start_music:
                PLAY_AHX_LOOP ahx_data,ahx_data_end-ahx_data
                rts

stop_music:
                STOP_AHX
                rts

; ============================================================================
; GENERATE TEXTURE (256x256 Checkerboard)
; ============================================================================

generate_texture:
                ; Enable VideoChip (needed for blitter)
                move.l  #1,VIDEO_CTRL
                move.l  #0,VIDEO_MODE

                ; Top-left: white
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FFFFFFFF,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w1:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w1

                ; Top-right: black
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE+512,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FF000000,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w2:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w2

                ; Bottom-left: black
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE+131072,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FF000000,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w3:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w3

                ; Bottom-right: white
                move.l  #BLT_OP_FILL,BLT_OP
                move.l  #TEXTURE_BASE+131584,BLT_DST
                move.l  #128,BLT_WIDTH
                move.l  #128,BLT_HEIGHT
                move.l  #$FFFFFFFF,BLT_COLOR
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.w4:            move.l  BLT_STATUS,d0
                andi.l  #2,d0
                bne.s   .w4
                rts

; ============================================================================
; AES / VDI DISPATCH
; ============================================================================

aes_call:
                lea     aes_pb(pc),a0
                move.l  a0,d1
                move.w  #200,d0
                trap    #2
                rts

vdi_call:
                lea     vdi_pb(pc),a0
                move.l  a0,d1
                move.w  #$73,d0
                trap    #2
                rts

; ============================================================================
; DATA
; ============================================================================

                even

; AES parameter block
aes_pb:         dc.l    aes_control
                dc.l    aes_global
                dc.l    aes_intin
                dc.l    aes_intout
                dc.l    aes_addrin
                dc.l    aes_addrout

; VDI parameter block
vdi_pb:         dc.l    vdi_contrl
                dc.l    vdi_intin
                dc.l    vdi_ptsin
                dc.l    vdi_intout
                dc.l    vdi_ptsout

; Window title
win_title:      dc.b    "Rotozoomer",0
                even

; VDI work_in array (11 words)
work_in:        ds.w    11

; ============================================================================
; SINE TABLE - 256 Entries, Signed 16-bit
; ============================================================================

sine_table:
                dc.w    0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
                dc.w    98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
                dc.w    181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
                dc.w    237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
                dc.w    256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
                dc.w    237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
                dc.w    181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
                dc.w    98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
                dc.w    0,-6,-13,-19,-25,-31,-38,-44,-50,-56,-62,-68,-74,-80,-86,-92
                dc.w    -98,-104,-109,-115,-121,-126,-132,-137,-142,-147,-152,-157,-162,-167,-172,-177
                dc.w    -181,-185,-190,-194,-198,-202,-206,-209,-213,-216,-220,-223,-226,-229,-231,-234
                dc.w    -237,-239,-241,-243,-245,-247,-248,-250,-251,-252,-253,-254,-255,-255,-256,-256
                dc.w    -256,-256,-256,-255,-255,-254,-253,-252,-251,-250,-248,-247,-245,-243,-241,-239
                dc.w    -237,-234,-231,-229,-226,-223,-220,-216,-213,-209,-206,-202,-198,-194,-190,-185
                dc.w    -181,-177,-172,-167,-162,-157,-152,-147,-142,-137,-132,-126,-121,-115,-109,-104
                dc.w    -98,-92,-86,-80,-74,-68,-62,-56,-50,-44,-38,-31,-25,-19,-13,-6

; ============================================================================
; RECIPROCAL TABLE - 256 Entries, Unsigned 16-bit
; ============================================================================

recip_table:
                dc.w    512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
                dc.w    416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
                dc.w    359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
                dc.w    329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320
                dc.w    320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
                dc.w    329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
                dc.w    359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
                dc.w    416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505
                dc.w    512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
                dc.w    665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
                dc.w    889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
                dc.w    1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279
                dc.w    1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
                dc.w    1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
                dc.w    889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
                dc.w    665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520

; ============================================================================
; AHX MUSIC DATA
; ============================================================================

                even
ahx_data:
                incbin  "../assets/music/chopper.ahx"
ahx_data_end:

; ============================================================================
; BSS - Uninitialised Data
; ============================================================================

                bss

; Animation accumulators
angle_accum:    ds.l    1
scale_accum:    ds.l    1
var_ca:         ds.l    1
var_sa:         ds.l    1
var_u0:         ds.l    1
var_v0:         ds.l    1

; GEM state
ap_id:          ds.w    1
vdi_handle:     ds.w    1
win_handle:     ds.w    1
win_x:          ds.w    1
win_y:          ds.w    1
win_w:          ds.w    1
win_h:          ds.w    1

; AES arrays
aes_control:    ds.w    5
aes_global:     ds.w    15
aes_intin:      ds.w    16
aes_intout:     ds.w    7
aes_addrin:     ds.l    2
aes_addrout:    ds.l    2

; VDI arrays
vdi_contrl:     ds.w    12
vdi_intin:      ds.w    128
vdi_ptsin:      ds.w    128
vdi_intout:     ds.w    128
vdi_ptsout:     ds.w    128

; Message buffer
msg_buf:        ds.w    8

; Local stack (2KB)
                ds.b    2048
stack_end:

                end
