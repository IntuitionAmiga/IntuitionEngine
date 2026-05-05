; ============================================================================
; AROS ROTOZOOMER — ASM + Hardware Direct (Blitter COPY to screen VRAM)
; ============================================================================
; Opens an Intuition CUSTOMSCREEN (640x480 RGBA32), renders via Mode7 blitter
; into an OS-allocated back buffer, then uses BLIT COPY to transfer directly
; to the screen bitmap's VRAM (obtained via LockBitMapTagList).
;
; Build: vasmm68k_mot -Fhunk -m68020 -devpac -Isdk/include \
;        -o RotoHW sdk/examples/asm/rotozoomer_aros_hw.asm
; ============================================================================

                include "ie68.inc"

; --- AmigaOS Constants ---
MEMF_ANY        equ     0
MEMF_CLEAR      equ     (1<<16)
TAG_DONE        equ     0
TAG_USER        equ     $80000000

; Intuition screen tags
SA_Dummy        equ     (TAG_USER+32)
SA_Width        equ     (SA_Dummy+3)
SA_Height       equ     (SA_Dummy+4)
SA_Depth        equ     (SA_Dummy+5)
SA_Title        equ     (SA_Dummy+8)
SA_Type         equ     (SA_Dummy+13)
SA_DisplayID    equ     (SA_Dummy+18)
SA_ShowTitle    equ     (SA_Dummy+22)
SA_Quiet        equ     (SA_Dummy+24)
CUSTOMSCREEN    equ     $000F

; Intuition window tags
WA_Dummy        equ     (TAG_USER+99)
WA_Width        equ     (WA_Dummy+3)
WA_Height       equ     (WA_Dummy+4)
WA_IDCMP        equ     (WA_Dummy+7)
WA_CustomScreen equ     (WA_Dummy+13)
WA_Backdrop     equ     (WA_Dummy+34)
WA_Borderless   equ     (WA_Dummy+37)
WA_Activate     equ     (WA_Dummy+38)
IDCMP_RAWKEY    equ     (1<<10)

; CyberGraphX tags
CYBRBIDTG_TB    equ     (TAG_USER+$50000)
CYBRBIDTG_Depth equ     (CYBRBIDTG_TB+0)
CYBRBIDTG_NominalWidth  equ (CYBRBIDTG_TB+1)
CYBRBIDTG_NominalHeight equ (CYBRBIDTG_TB+2)

; LockBitMapTagList tags
LBMI_BYTESPERROW equ    $84001006
LBMI_BASEADDRESS equ    $84001007

; Exec LVOs
_LVOOpenLibrary     equ -552            ; LVO 92
_LVOCloseLibrary    equ -414            ; LVO 69
_LVOGetMsg          equ -372            ; LVO 62
_LVOReplyMsg        equ -378            ; LVO 63
_LVOAllocMem        equ -198            ; LVO 33
_LVOFreeMem         equ -210            ; LVO 35

; Intuition LVOs
_LVOOpenScreenTagList  equ -612         ; LVO 102
_LVOCloseScreen        equ -66          ; LVO 11
_LVOOpenWindowTagList  equ -606         ; LVO 101
_LVOCloseWindow        equ -72          ; LVO 12

; CyberGraphX LVOs
_LVOBestCModeIDTagList equ -60          ; LVO 10
_LVOLockBitMapTagList  equ -168         ; LVO 28
_LVOUnLockBitMap       equ -174         ; LVO 29

; Screen struct offsets
sc_RastPort     equ     84              ; offset to embedded RastPort
rp_BitMap       equ     4               ; RastPort->BitMap pointer

; IntuiMessage offsets
im_Class        equ     20
im_Code         equ     24

; Window struct offsets
wd_UserPort     equ     86

; Screen dimensions
RENDER_W        equ     640
RENDER_H        equ     480
LINE_BYTES      set     2560            ; 640 * 4 (override ie68.inc default)

; Texture
TEX_SIZE        equ     (256*256*4)     ; 262144 bytes
TEX_STRIDE      equ     1024            ; 256 * 4

; Back buffer
BACKBUF_SIZE    equ     (640*480*4)     ; 1228800 bytes

; Animation increments (8.8 fixed-point)
ANGLE_INC       equ     313
SCALE_INC       equ     104

; Raw key code for ESC
RAWKEY_ESC      equ     $45

; Media loader MMIO
MEDIA_NAME_PTR  equ     $F2300
MEDIA_SUBSONG   equ     $F2304
MEDIA_CTRL      equ     $F2308
MEDIA_OP_PLAY   equ     1
MEDIA_OP_STOP   equ     2

; ============================================================================
; ENTRY POINT
; ============================================================================

                section text,code

start:
                movem.l d0-d7/a0-a6,-(sp)

                ; --- Get ExecBase ---
                movea.l 4.w,a6
                move.l  a6,_ExecBase

                ; --- Open intuition.library ---
                lea     intuition_name(pc),a1
                moveq   #39,d0
                jsr     _LVOOpenLibrary(a6)
                move.l  d0,_IntuitionBase
                beq     .exit

                ; --- Open cybergraphics.library ---
                lea     cybergfx_name(pc),a1
                moveq   #40,d0
                movea.l _ExecBase,a6
                jsr     _LVOOpenLibrary(a6)
                move.l  d0,_CyberGfxBase
                beq     .close_intuition

                ; --- AllocMem: texture buffer ---
                move.l  #TEX_SIZE,d0
                move.l  #MEMF_ANY|MEMF_CLEAR,d1
                movea.l _ExecBase,a6
                jsr     _LVOAllocMem(a6)
                move.l  d0,texture_buf
                beq     .close_cgfx

                ; --- AllocMem: back buffer ---
                move.l  #BACKBUF_SIZE,d0
                move.l  #MEMF_ANY|MEMF_CLEAR,d1
                movea.l _ExecBase,a6
                jsr     _LVOAllocMem(a6)
                move.l  d0,back_buf
                beq     .free_texture

                ; --- Load texture into texture_buf ---
                bsr     load_texture

                ; --- BestCModeIDTagList ---
                lea     bestmode_tags(pc),a0
                movea.l _CyberGfxBase,a6
                jsr     _LVOBestCModeIDTagList(a6)
                move.l  d0,display_id
                cmpi.l  #$FFFFFFFF,d0
                beq     .free_backbuf

                ; --- Patch screen tags with DisplayID ---
                lea     scr_displayid_val,a0
                move.l  d0,(a0)

                ; --- OpenScreenTagList ---
                suba.l  a0,a0                   ; NewScreen = NULL
                lea     screen_tags,a1
                movea.l _IntuitionBase,a6
                jsr     _LVOOpenScreenTagList(a6)
                move.l  d0,screen_ptr
                beq     .free_backbuf

                ; --- Patch window tags with screen pointer ---
                lea     win_screen_val,a0
                move.l  screen_ptr,(a0)

                ; --- OpenWindowTagList ---
                suba.l  a0,a0                   ; NewWindow = NULL
                lea     window_tags,a1
                movea.l _IntuitionBase,a6
                jsr     _LVOOpenWindowTagList(a6)
                move.l  d0,window_ptr
                beq     .close_screen

                ; --- Init animation ---
                clr.l   angle_accum
                clr.l   scale_accum
                bsr     start_music

; ============================================================================
; MAIN LOOP
; ============================================================================
.main_loop:
                bsr     compute_frame
                bsr     render_mode7
                bsr     blit_to_screen
                bsr     wait_vsync
                bsr     advance_animation
                bsr     check_idcmp
                tst.l   d0
                beq     .main_loop

; ============================================================================
; CLEANUP
; ============================================================================
                bsr     stop_music

.close_window:
                movea.l window_ptr,a0
                movea.l _IntuitionBase,a6
                jsr     _LVOCloseWindow(a6)

.close_screen:
                movea.l screen_ptr,a0
                movea.l _IntuitionBase,a6
                jsr     _LVOCloseScreen(a6)

.free_backbuf:
                move.l  back_buf,d0
                beq.s   .free_texture
                movea.l d0,a1
                move.l  #BACKBUF_SIZE,d0
                movea.l _ExecBase,a6
                jsr     _LVOFreeMem(a6)

.free_texture:
                move.l  texture_buf,d0
                beq.s   .close_cgfx
                movea.l d0,a1
                move.l  #TEX_SIZE,d0
                movea.l _ExecBase,a6
                jsr     _LVOFreeMem(a6)

.close_cgfx:
                move.l  _CyberGfxBase,d0
                beq.s   .close_intuition
                movea.l d0,a1
                movea.l _ExecBase,a6
                jsr     _LVOCloseLibrary(a6)

.close_intuition:
                move.l  _IntuitionBase,d0
                beq.s   .exit
                movea.l d0,a1
                movea.l _ExecBase,a6
                jsr     _LVOCloseLibrary(a6)

.exit:
                movem.l (sp)+,d0-d7/a0-a6
                moveq   #0,d0
                rts

; ============================================================================
; LOAD TEXTURE — BLIT COPY embedded raw data into AllocMem'd texture buffer
; ============================================================================
load_texture:
                move.l  #BLT_OP_COPY,BLT_OP
                lea     texture_data(pc),a0
                move.l  a0,BLT_SRC
                move.l  texture_buf,BLT_DST
                move.l  #256,BLT_WIDTH
                move.l  #256,BLT_HEIGHT
                move.l  #TEX_STRIDE,BLT_SRC_STRIDE
                move.l  #TEX_STRIDE,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.wait:          move.l  BLT_CTRL,d0
                andi.l  #2,d0
                bne.s   .wait
                rts

; ============================================================================
; MUSIC
; ============================================================================
start_music:
                lea     music_path(pc),a0
                move.l  a0,MEDIA_NAME_PTR
                clr.l   MEDIA_SUBSONG
                move.l  #MEDIA_OP_PLAY,MEDIA_CTRL
                rts

stop_music:
                move.l  #MEDIA_OP_STOP,MEDIA_CTRL
                rts

; ============================================================================
; COMPUTE FRAME — identical to bare-metal version
; ============================================================================
compute_frame:
                movem.l d0-d7,-(sp)

                move.l  angle_accum,d0
                lsr.l   #8,d0
                andi.l  #255,d0

                move.l  scale_accum,d1
                lsr.l   #8,d1
                andi.l  #255,d1

                ; cos = sine_table[(angle_idx + 64) & 255]
                move.l  d0,d2
                addi.l  #64,d2
                andi.l  #255,d2
                add.l   d2,d2
                lea     sine_table(pc),a0
                move.w  (a0,d2.l),d3
                ext.l   d3

                ; sin = sine_table[angle_idx]
                move.l  d0,d2
                add.l   d2,d2
                move.w  (a0,d2.l),d4
                ext.l   d4

                ; recip = recip_table[scale_idx]
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

                ; u0 = 0x800000 - CA*320 + SA*240
                move.l  d6,d0
                move.l  d0,d1
                lsl.l   #8,d0
                lsl.l   #6,d1
                add.l   d1,d0           ; CA * 320

                move.l  d7,d1
                move.l  d1,d2
                lsl.l   #8,d1
                lsl.l   #4,d2
                sub.l   d2,d1           ; SA * 240

                move.l  #$800000,d3
                sub.l   d0,d3
                add.l   d1,d3
                move.l  d3,var_u0

                ; v0 = 0x800000 - SA*320 - CA*240
                move.l  d7,d0
                move.l  d0,d1
                lsl.l   #8,d0
                lsl.l   #6,d1
                add.l   d1,d0           ; SA * 320

                move.l  d6,d1
                move.l  d1,d2
                lsl.l   #8,d1
                lsl.l   #4,d2
                sub.l   d2,d1           ; CA * 240

                move.l  #$800000,d3
                sub.l   d0,d3
                sub.l   d1,d3
                move.l  d3,var_v0

                movem.l (sp)+,d0-d7
                rts

; ============================================================================
; RENDER MODE7 — texture_buf → back_buf via Mode7 blitter
; ============================================================================
render_mode7:
                move.l  #BLT_OP_MODE7,BLT_OP
                move.l  texture_buf,BLT_SRC
                move.l  back_buf,BLT_DST
                move.l  #RENDER_W,BLT_WIDTH
                move.l  #RENDER_H,BLT_HEIGHT
                move.l  #TEX_STRIDE,BLT_SRC_STRIDE
                move.l  #LINE_BYTES,BLT_DST_STRIDE
                move.l  #255,BLT_MODE7_TEX_W
                move.l  #255,BLT_MODE7_TEX_H

                move.l  var_u0,d0
                move.l  d0,BLT_MODE7_U0
                move.l  var_v0,d0
                move.l  d0,BLT_MODE7_V0
                move.l  var_ca,d0
                move.l  d0,BLT_MODE7_DU_COL
                move.l  var_sa,d0
                move.l  d0,BLT_MODE7_DV_COL
                neg.l   d0
                move.l  d0,BLT_MODE7_DU_ROW
                move.l  var_ca,d0
                move.l  d0,BLT_MODE7_DV_ROW

                move.l  #1,BLT_CTRL
.wait:          move.l  BLT_CTRL,d0
                andi.l  #2,d0
                bne.s   .wait
                rts

; ============================================================================
; BLIT TO SCREEN — LockBitMap + BLIT COPY back_buf → screen VRAM + UnLock
; ============================================================================
blit_to_screen:
                ; Lock the screen bitmap
                movea.l screen_ptr,a0
                lea     sc_RastPort(a0),a0
                movea.l rp_BitMap(a0),a0        ; a0 = BitMap
                lea     lbmi_tags,a1
                movea.l _CyberGfxBase,a6
                jsr     _LVOLockBitMapTagList(a6)
                tst.l   d0
                beq.s   .done
                move.l  d0,lock_handle

                ; BLIT COPY from back buffer to screen VRAM
                move.l  #BLT_OP_COPY,BLT_OP
                move.l  back_buf,BLT_SRC
                move.l  screen_vram,BLT_DST
                move.l  #RENDER_W,BLT_WIDTH
                move.l  #RENDER_H,BLT_HEIGHT
                move.l  #LINE_BYTES,BLT_SRC_STRIDE
                move.l  screen_stride,BLT_DST_STRIDE
                move.l  #1,BLT_CTRL
.waitblt:       move.l  BLT_CTRL,d0
                andi.l  #2,d0
                bne.s   .waitblt

                ; Unlock
                movea.l lock_handle,a0
                movea.l _CyberGfxBase,a6
                jsr     _LVOUnLockBitMap(a6)
.done:
                rts

; ============================================================================
; WAIT VSYNC — two-phase edge detection
; ============================================================================
wait_vsync:
.wait_end:      move.l  VIDEO_STATUS,d0
                andi.l  #STATUS_VBLANK,d0
                bne.s   .wait_end
.wait_start:    move.l  VIDEO_STATUS,d0
                andi.l  #STATUS_VBLANK,d0
                beq.s   .wait_start
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
; CHECK IDCMP — returns d0=1 if ESC pressed, 0 otherwise
; ============================================================================
check_idcmp:
                movem.l a1-a6,-(sp)
                moveq   #0,d0
                move.l  d0,-(sp)                ; result on stack

                movea.l window_ptr,a0
                movea.l wd_UserPort(a0),a0       ; a0 = MsgPort
                movea.l _ExecBase,a6
.loop:
                jsr     _LVOGetMsg(a6)
                tst.l   d0
                beq.s   .no_more

                movea.l d0,a1                   ; a1 = IntuiMessage
                move.l  im_Class(a1),d1
                move.w  im_Code(a1),d2
                jsr     _LVOReplyMsg(a6)

                ; Check for RAWKEY + ESC
                cmpi.l  #IDCMP_RAWKEY,d1
                bne.s   .loop
                cmpi.w  #RAWKEY_ESC,d2
                bne.s   .loop
                move.l  #1,(sp)                 ; set result = exit
                bra.s   .loop                   ; drain remaining messages

.no_more:
                move.l  (sp)+,d0                ; get result
                movem.l (sp)+,a1-a6
                rts

; ============================================================================
; DATA (same section as code for single-hunk executable)
; ============================================================================

; Library names
intuition_name: dc.b    'intuition.library',0
cybergfx_name:  dc.b    'cybergraphics.library',0
                even

; Library bases
_ExecBase:      dc.l    0
_IntuitionBase: dc.l    0
_CyberGfxBase:  dc.l    0

; Pointers
screen_ptr:     dc.l    0
window_ptr:     dc.l    0
texture_buf:    dc.l    0
back_buf:       dc.l    0
lock_handle:    dc.l    0
display_id:     dc.l    0

; LockBitMapTagList output storage
screen_vram:    dc.l    0
screen_stride:  dc.l    0

; Animation state
angle_accum:    dc.l    0
scale_accum:    dc.l    0
var_ca:         dc.l    0
var_sa:         dc.l    0
var_u0:         dc.l    0
var_v0:         dc.l    0

; BestCModeIDTagList tags
bestmode_tags:
                dc.l    CYBRBIDTG_NominalWidth,640
                dc.l    CYBRBIDTG_NominalHeight,480
                dc.l    CYBRBIDTG_Depth,32
                dc.l    TAG_DONE

; OpenScreenTagList tags
screen_tags:
                dc.l    SA_Type,CUSTOMSCREEN
                dc.l    SA_DisplayID
scr_displayid_val:
                dc.l    0                       ; patched at runtime
                dc.l    SA_Width,RENDER_W
                dc.l    SA_Height,RENDER_H
                dc.l    SA_Depth,32
                dc.l    SA_Title
                dc.l    scr_title
                dc.l    SA_ShowTitle,0
                dc.l    SA_Quiet,1
                dc.l    TAG_DONE

; OpenWindowTagList tags
window_tags:
                dc.l    WA_CustomScreen
win_screen_val: dc.l    0                       ; patched at runtime
                dc.l    WA_Width,RENDER_W
                dc.l    WA_Height,RENDER_H
                dc.l    WA_IDCMP,IDCMP_RAWKEY
                dc.l    WA_Borderless,1
                dc.l    WA_Backdrop,1
                dc.l    WA_Activate,1
                dc.l    TAG_DONE

; LockBitMapTagList tags — ti_Data points to writable storage
lbmi_tags:
                dc.l    LBMI_BASEADDRESS
                dc.l    screen_vram
                dc.l    LBMI_BYTESPERROW
                dc.l    screen_stride
                dc.l    TAG_DONE

scr_title:      dc.b    'Rotozoomer HW',0
                even

; ============================================================================
; LOOKUP TABLES
; ============================================================================

; Sine table: 256 entries, signed 16-bit
; Values: round(sin(i * 2*pi / 256) * 256)
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

; Reciprocal table: 256 entries, unsigned 16-bit
; Values: round(256 / (0.5 + sin(i * 2*pi / 256) * 0.3))
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
; TEXTURE DATA
; ============================================================================
                even
texture_data:
                incbin  "../assets/rotozoomtexture.raw"

music_path:
                dc.b    "sdk/examples/assets/music/chopper.ahx",0
                even
