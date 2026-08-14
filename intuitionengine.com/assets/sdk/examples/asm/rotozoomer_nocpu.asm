; NO-CPU MODE 7 ROTOZOOMER
; =============================================================================
;
; QUICK REFERENCE
;
; Target processor: IE64 for bootstrap only
; Video hardware:  VideoChip Copper, DMA blitter with Mode 7 and Raster Effects Unit
; Music:           Embedded Standard MIDI File played by the MIDI player
; Build:           make nocpu-rotozoomer
; Live USB image:  Included in the automated Intuition Engine product demonstration
;
; WHAT THIS DEMO DOES
;
; IE64 clears the circuit state and builds 65,536 compact affine records.
; It scatters those records into the lookup layout, starts the music, enables
; Copper and executes HALT. The Standard MIDI File and texture are embedded,
; so the finished .ie64 image has no runtime file dependency.
;
; After the bootstrap, Copper selects each transform and starts the required
; blits. The blitter advances the animation state and renders each later Mode 7
; frame. IE64 remains halted while Copper, the blitter and MIDI player continue.
;
; FILE DEPENDENCIES
;
; The source includes only ie64.inc and embeds yourlove.mid and
; rotozoomtexture_nocpu.raw.
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later
; =============================================================================
include "ie64.inc"

; MEMORY MAP
;
; The texture and framebuffer occupy ordinary unified memory. STATE_BASE begins
; with zero-filled source data, followed by the bit-sliced phases, temporary
; multiplexer storage, one replicated carry mask, expanded affine lookup tables
; and angle-address lanes. Compact records sit beyond that range and are written
; and consumed only at bootstrap.
TEXTURE_BASE              equ 0x600000
VIDEO_REG_BASE            equ 0xF0000
FRAMEBUFFER_BASE          equ 0xA00000
COPPER_LIST_BASE          equ 0x1000000
STATE_BASE                equ 0x1400000
STATE_CLEAR_BYTES         equ 0x695000

ANGLE_PHASE_BITS          equ STATE_BASE+0x4000
SCALE_PHASE_BITS          equ ANGLE_PHASE_BITS+0x4000
PHASE_BIT_STRIDE          equ 0x8000
LOOKUP_WORK               equ STATE_BASE+0x84000
LOOKUP_TEMP               equ STATE_BASE+0x8C000
CARRY_BITS                equ STATE_BASE+0x90000
EXPANDED_AFFINE_TABLES    equ STATE_BASE+0x94000
ANGLE_TABLE_ADDRESSES     equ STATE_BASE+0x694000
COMPACT_AFFINE_RECORDS    equ 0x1B00000

PHASE_LANE_WORDS          equ 4096
AFFINE_RECORD_BYTES       equ 24
AFFINE_RECORD_LANES       equ AFFINE_RECORD_BYTES

DISPLAY_WIDTH             equ 640
DISPLAY_HEIGHT            equ 480
DISPLAY_CENTRE_X          equ DISPLAY_WIDTH/2
DISPLAY_CENTRE_Y          equ DISPLAY_HEIGHT/2
FRAMEBUFFER_ROW_BYTES     equ DISPLAY_WIDTH*4
TEXTURE_SIZE              equ 256
TEXTURE_MASK              equ TEXTURE_SIZE-1
TEXTURE_ROW_BYTES         equ TEXTURE_SIZE*4
TEXTURE_CENTRE_16_16      equ (TEXTURE_SIZE/2)<<16
PHASE_ENTRY_COUNT         equ 256
QUARTER_TURN_ENTRIES      equ PHASE_ENTRY_COUNT/4

; Each 16-bit phase uses 16 bit planes. Angle and scale planes alternate in
; memory. Each active plane occupies 0x4000 bytes, while PHASE_BIT_STRIDE
; advances 0x8000 bytes to the next plane of the same phase. The planes hold
; all-zero or all-one masks repeated across 4,096 32-bit words, so one Boolean
; blit applies a phase bit to every packed candidate. The high eight bits select
; a table entry and the low eight bits retain the fractional phase. The angle
; step approximates 0.03 radians per frame and the scale step approximates 0.01
; radians per frame:
;
;   round(0.03 * 256 entries * 256 fraction / (2 * pi)) = 313
;   round(0.01 * 256 entries * 256 fraction / (2 * pi)) = 104

COLOUR_MODE_RGBA32        equ 0
COLOUR_MODE_CLUT8         equ 1
MIDI_START_AND_LOOP       equ 5
COPPER_ENABLE_AND_RESET   equ 3
COPPER_MOVE_OPCODE        equ 0x40000000
COPPER_WAIT_SCANLINE_1    equ 1<<12
COPPER_END_OPCODE         equ 0xC0000000
ROP_AND                   equ 1
ROP_AND_INVERTED          equ 4
ROP_COPY                  equ 3
ROP_XOR                   equ 6
ROP_COPY_INVERTED         equ 12
ROP_OR_INVERTED           equ 13

; Emit a Copper MOVE to a VideoChip register. A MOVE occupies two longwords:
; the encoded register index followed by its 32-bit value.
copper_move macro
    dc.l COPPER_MOVE_OPCODE | (((\1-VIDEO_REG_BASE)/4)<<16)
    dc.l \2
endm

; A 640-byte CLUT8 raster band starts at the supplied unified memory address.
; Ordered suffix writes leave the intended little-endian bytes at the start of
; a blitter register shadow. The unused remainder is overwritten by later
; suffixes or lies outside the fields consumed by the next operation.
write_shadow_suffix macro
    copper_move VIDEO_FB_BASE,\1
    copper_move VIDEO_RASTER_COLOR,\2
    copper_move VIDEO_RASTER_CTRL,1
endm

; This form labels the raster colour operand. VIDEO_COLOR_MODE selects CLUT8
; raster bands, while BLT_FLAGS independently selects RGBA32 for Boolean
; arithmetic and operand-copy blits. A previous COPY blit can replace the
; complete four-byte Copper operand with a selected byte value in its low byte
; before Copper fetches it.
write_patchable_shadow_suffix macro
    copper_move VIDEO_FB_BASE,\2
    dc.l COPPER_MOVE_OPCODE | (((VIDEO_RASTER_COLOR-VIDEO_REG_BASE)/4)<<16)
\1:
    dc.l \3
    copper_move VIDEO_RASTER_CTRL,1
endm

; Emit the ordered suffix writes needed for a little-endian value.
write_shadow_bytes macro
    write_shadow_suffix \1,(\2&255)
    if \3>1
        write_shadow_bytes \1+1,(\2>>8),(\3-1)
    endif
endm

; Emit four patchable suffixes for one 32-bit blitter value.
write_patchable_shadow_long macro
    write_patchable_shadow_suffix \3,\1,((\2>>0)&255)
    write_patchable_shadow_suffix \4,\1+1,((\2>>8)&255)
    write_patchable_shadow_suffix \5,\1+2,((\2>>16)&255)
    write_patchable_shadow_suffix \6,\1+3,((\2>>24)&255)
endm

; Emit the fields required for one synchronous COPY blit. BLT_OP remains zero
; from the frame's shadow clear. The high zero byte of the height suffix restores
; both strides to zero before BLT_CTRL. Optional source labels make the four
; source-address bytes patchable.
emit_blit macro
    if narg>4
        write_patchable_shadow_long BLT_SRC,\1,\5,\6,\7,\8
    else
        write_shadow_bytes BLT_SRC,\1,4
    endif
    write_shadow_bytes BLT_DST,\2,4
    write_shadow_bytes BLT_WIDTH,\3,3
    write_shadow_bytes BLT_HEIGHT,1,2
    write_shadow_bytes BLT_FLAGS,(\4<<4),2
    copper_move BLT_CTRL,1
endm

emit_copy_blit macro
    emit_blit \1,\2,\3,ROP_COPY
endm

; Select one value from a 256-entry table. Each stage uses one bit-sliced phase
; plane to choose a half of the remaining packed candidates with Boolean blits.
emit_lookup_multiplexer macro
    if \4
        emit_copy_blit LOOKUP_WORK,LOOKUP_TEMP,((1<<(\4-1))*\3)
        emit_blit LOOKUP_WORK+(((1<<(\4-1))*\3)*4),LOOKUP_TEMP,((1<<(\4-1))*\3),ROP_XOR
        emit_blit \1+((\2+\4-1)*PHASE_BIT_STRIDE),LOOKUP_TEMP,((1<<(\4-1))*\3),ROP_AND
        emit_blit LOOKUP_TEMP,LOOKUP_WORK,((1<<(\4-1))*\3),ROP_XOR
        emit_lookup_multiplexer \1,\2,\3,(\4-1)
    endif
endm

; Copy four selected byte lanes into four later Copper colour operands. Their
; ordered suffix writes assemble one 32-bit value in the blitter shadow.
patch_copper_long_operand macro
    emit_copy_blit \1+0,\2,1
    emit_copy_blit \1+4,\3,1
    emit_copy_blit \1+8,\4,1
    emit_copy_blit \1+12,\5,1
endm

; Apply one full-adder stage to every replicated phase word. The Boolean blits
; derive the sum bit and the carry consumed by the following stage.
emit_phase_adder_bit macro
    if \2&(1<<\3)
        emit_blit CARRY_BITS,\1+(\3*PHASE_BIT_STRIDE),PHASE_LANE_WORDS,ROP_XOR
        emit_blit \1+(\3*PHASE_BIT_STRIDE),\1+(\3*PHASE_BIT_STRIDE),PHASE_LANE_WORDS,ROP_COPY_INVERTED
        if \3<15
            emit_blit \1+(\3*PHASE_BIT_STRIDE),CARRY_BITS,PHASE_LANE_WORDS,ROP_OR_INVERTED
        endif
    else
        emit_blit CARRY_BITS,\1+(\3*PHASE_BIT_STRIDE),PHASE_LANE_WORDS,ROP_XOR
        if \3<15
            emit_blit \1+(\3*PHASE_BIT_STRIDE),CARRY_BITS,PHASE_LANE_WORDS,ROP_AND_INVERTED
        endif
    endif
    if \3<15
        emit_phase_adder_bit \1,\2,(\3+1)
    endif
endm

; Advance one 16-bit bit-sliced phase with normal wrapping arithmetic.
emit_phase_adder macro
    emit_copy_blit STATE_BASE,CARRY_BITS,PHASE_LANE_WORDS
    emit_phase_adder_bit \1,\2,0
endm

; Write one 32-bit MMIO value during the IE64 bootstrap.
write_mmio_long macro
    li      r2,#\2
    store.l r2,\1(r0)
endm

org 0x1000

; BOOTSTRAP STAGE 1: CLEAR THE CIRCUIT STATE
;
; The fill establishes the zero source, initial phase planes, lookup workspace,
; carry mask and zero padding for the expanded and angle-address lanes. Later
; CPU byte stores and the CLUT8 scatter change only the low byte of each 32-bit
; lane. The upper three bytes remain zero for RGBA32 Boolean blits. BLT_CTRL
; starts the synchronous operation.
    la      r31,STACK_TOP

    store.l r0,VIDEO_MODE(r0)
    store.l r0,VIDEO_COLOR_MODE(r0)
    write_mmio_long BLT_OP,BLT_OP_FILL
    write_mmio_long BLT_DST,STATE_BASE
    write_mmio_long BLT_WIDTH,STATE_CLEAR_BYTES/4
    write_mmio_long BLT_HEIGHT,1
    store.l r0,BLT_COLOR(r0)
    store.l r0,BLT_DST_STRIDE(r0)
    store.l r0,BLT_FLAGS(r0)
    store.l r2,BLT_CTRL(r0) ; Start the fill. r2 is still 1 from BLT_HEIGHT.

; BOOTSTRAP STAGE 2: BUILD THE AFFINE RECORDS
;
; The outer loop visits 256 angles and records the base address of each
; expanded scale table. The sine table is signed 8.8 and the reciprocal table
; is unsigned 8.8. Their product gives the signed 16.16 Mode 7 column and row
; increments. Reciprocal multiplication replaces division while preserving the
; other rotozoomers' scale curve.
    la      r20,sine_table
    la      r21,reciprocal_table
    la      r26,reciprocal_table+(PHASE_ENTRY_COUNT*2)
    li      r22,#COMPACT_AFFINE_RECORDS
    li      r23,#ANGLE_TABLE_ADDRESSES
    li      r27,#EXPANDED_AFFINE_TABLES
    move.l  r24,#0
    li      r19,#PHASE_ENTRY_COUNT

build_angle_records:
    move.l  r1,r27
    store.b r1,0(r23)
    lsr.l   r1,r1,#8
    store.b r1,4(r23)
    lsr.l   r1,r1,#8
    store.b r1,8(r23)
    lsr.l   r1,r1,#8
    store.b r1,12(r23)
    add.l   r23,r23,#16
    add.l   r27,r27,#(AFFINE_RECORD_LANES*PHASE_ENTRY_COUNT*4)

    add.l   r1,r24,#QUARTER_TURN_ENTRIES
    and.l   r1,r1,#TEXTURE_MASK
    lsl.l   r1,r1,#1
    add.l   r1,r20,r1
    load.w  r7,(r1)
    sext.w  r7,r7

    lsl.l   r1,r24,#1
    add.l   r1,r20,r1
    load.w  r8,(r1)
    sext.w  r8,r8

    move.l  r25,r21
build_scale_records:
    load.w  r9,(r25)

    muls.l  r10,r7,r9
    muls.l  r11,r8,r9

    muls.l  r12,r10,#DISPLAY_CENTRE_X
    muls.l  r13,r11,#DISPLAY_CENTRE_Y
    li      r14,#TEXTURE_CENTRE_16_16
    sub.l   r14,r14,r12
    add.l   r14,r14,r13

    muls.l  r12,r11,#DISPLAY_CENTRE_X
    muls.l  r13,r10,#DISPLAY_CENTRE_Y
    li      r15,#TEXTURE_CENTRE_16_16
    sub.l   r15,r15,r12
    sub.l   r15,r15,r13

    sub.l   r16,r0,r11

; Each compact record contains u0, v0, cos*zoom, sin*zoom, -sin*zoom and
; cos*zoom. Those last four values are the U/V increments per column and row.
; The origins centre the 256 by 256 texture on the 640 by 480 display:
;
;   u0 = texture centre - cos*zoom*320 + sin*zoom*240
;   v0 = texture centre - sin*zoom*320 - cos*zoom*240
    store.l r14,0(r22)
    store.l r15,4(r22)
    store.l r10,8(r22)
    store.l r11,12(r22)
    store.l r16,16(r22)
    store.l r10,20(r22)
    add.l   r22,r22,#AFFINE_RECORD_BYTES

    add.l   r25,r25,#2
    blt     r25,r26,build_scale_records
    add.l   r24,r24,#1
    blt     r24,r19,build_angle_records

; BOOTSTRAP STAGE 3: EXPAND THE BYTE LANES
;
; One CLUT8 copy uses a source stride of one byte and a destination stride of
; four bytes. It scatters every compact record byte into a separate 32-bit lane
; so later Boolean blits can process all candidates in bulk.
    write_mmio_long BLT_OP,BLT_OP_COPY
    write_mmio_long BLT_SRC,COMPACT_AFFINE_RECORDS
    write_mmio_long BLT_DST,EXPANDED_AFFINE_TABLES
    write_mmio_long BLT_WIDTH,1
    write_mmio_long BLT_HEIGHT,PHASE_ENTRY_COUNT*PHASE_ENTRY_COUNT*AFFINE_RECORD_LANES
    write_mmio_long BLT_SRC_STRIDE,1
    write_mmio_long BLT_DST_STRIDE,4
    write_mmio_long BLT_FLAGS,BLT_FLAGS_BPP_CLUT8|(ROP_COPY<<BLT_FLAGS_DRAWMODE_SHIFT)
    write_mmio_long BLT_CTRL,1

; BOOTSTRAP STAGE 4: HAND CONTROL TO THE CUSTOM CHIPS
;
; Copper resets to COPPER_LIST_BASE at each 60 Hz frame. MIDI_PLAY_PTR and
; MIDI_PLAY_LEN describe the embedded Standard MIDI File through the player
; PTR/LEN/CTRL protocol. The volume is 180 out of 255, and control value 5
; starts playback and enables looping. MIDI playback is independent of IE64,
; so both animation and music continue after HALT.
    write_mmio_long VIDEO_FB_BASE,FRAMEBUFFER_BASE
    write_mmio_long VIDEO_CTRL,1
    write_mmio_long MIDI_PLAY_PTR,midi_data
    write_mmio_long MIDI_PLAY_LEN,midi_data_end-midi_data
    write_mmio_long MIDI_VOLUME,180
    write_mmio_long MIDI_PLAY_CTRL,MIDI_START_AND_LOOP
    write_mmio_long COPPER_PTR,COPPER_LIST_BASE
    write_mmio_long COPPER_CTRL,COPPER_ENABLE_AND_RESET
    halt

; LOOKUP TABLES AND EMBEDDED ASSETS
;
; sine_table contains one revolution in signed 8.8 format. reciprocal_table
; contains round(256 / (0.5 + 0.3 * sin(angle))) for the zoom cycle.
sine_table:
    dc.w 0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
    dc.w 98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
    dc.w 181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
    dc.w 237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
    dc.w 256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
    dc.w 237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
    dc.w 181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
    dc.w 98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
    dc.w 0,-6,-13,-19,-25,-31,-38,-44,-50,-56,-62,-68,-74,-80,-86,-92
    dc.w -98,-104,-109,-115,-121,-126,-132,-137,-142,-147,-152,-157,-162,-167,-172,-177
    dc.w -181,-185,-190,-194,-198,-202,-206,-209,-213,-216,-220,-223,-226,-229,-231,-234
    dc.w -237,-239,-241,-243,-245,-247,-248,-250,-251,-252,-253,-254,-255,-255,-256,-256
    dc.w -256,-256,-256,-255,-255,-254,-253,-252,-251,-250,-248,-247,-245,-243,-241,-239
    dc.w -237,-234,-231,-229,-226,-223,-220,-216,-213,-209,-206,-202,-198,-194,-190,-185
    dc.w -181,-177,-172,-167,-162,-157,-152,-147,-142,-137,-132,-126,-121,-115,-109,-104
    dc.w -98,-92,-86,-80,-74,-68,-62,-56,-50,-44,-38,-31,-25,-19,-13,-6

reciprocal_table:
    dc.w 512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
    dc.w 416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
    dc.w 359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
    dc.w 329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320
    dc.w 320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
    dc.w 329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
    dc.w 359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
    dc.w 416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505
    dc.w 512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
    dc.w 665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
    dc.w 889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
    dc.w 1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279
    dc.w 1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
    dc.w 1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
    dc.w 889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
    dc.w 665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520

align 4
midi_data:
incbin "../assets/music/yourlove.mid"
midi_data_end:

org TEXTURE_BASE
incbin "../assets/rotozoomtexture_nocpu.raw"

; COPPER FRAME PROGRAMME
;
; The first frame primes presentation hold. Later frames retain the preceding
; completed frame while the circuit selects the six Mode 7 parameters, advances
; both phases for the following frame and renders the selected transform. The
; WAIT for scanline 1 gives the hold a scanline boundary.
org COPPER_LIST_BASE
    copper_move VIDEO_CTRL,VIDEO_CTRL_ENABLE|VIDEO_CTRL_PRESENT_HOLD
    copper_move VIDEO_COLOR_MODE,COLOUR_MODE_CLUT8
    copper_move VIDEO_RASTER_Y,0
    copper_move VIDEO_RASTER_HEIGHT,1
    dc.l COPPER_WAIT_SCANLINE_1 ; WAIT for scanline 1, horizontal position 0.

; The first suffix clears the core blitter and Mode 7 shadow covered by the
; 640-byte raster band. BLT_FLAGS lies beyond the 640-byte band and is written
; separately for every command. Each following ordered suffix write repairs the
; fields used by the next arithmetic operation.
    write_shadow_bytes BLT_OP,0,1

; Select the angle table address from the high eight bits of the angle phase.
    emit_copy_blit ANGLE_TABLE_ADDRESSES,LOOKUP_WORK,(PHASE_ENTRY_COUNT*4)
    emit_lookup_multiplexer ANGLE_PHASE_BITS,8,4,8
    patch_copper_long_operand LOOKUP_WORK,angle_table_address_byte_0,angle_table_address_byte_1,angle_table_address_byte_2,angle_table_address_byte_3

; Select one scale record, then copy its six Mode 7 parameters into later
; Copper operands. The blitter changes those operands before Copper fetches them.
    emit_blit 0,LOOKUP_WORK,(PHASE_ENTRY_COUNT*AFFINE_RECORD_LANES),ROP_COPY,angle_table_address_byte_0,angle_table_address_byte_1,angle_table_address_byte_2,angle_table_address_byte_3
    emit_lookup_multiplexer SCALE_PHASE_BITS,8,AFFINE_RECORD_LANES,8
    patch_copper_long_operand LOOKUP_WORK+0,mode7_u0_byte_0,mode7_u0_byte_1,mode7_u0_byte_2,mode7_u0_byte_3
    patch_copper_long_operand LOOKUP_WORK+16,mode7_v0_byte_0,mode7_v0_byte_1,mode7_v0_byte_2,mode7_v0_byte_3
    patch_copper_long_operand LOOKUP_WORK+32,mode7_du_col_byte_0,mode7_du_col_byte_1,mode7_du_col_byte_2,mode7_du_col_byte_3
    patch_copper_long_operand LOOKUP_WORK+48,mode7_dv_col_byte_0,mode7_dv_col_byte_1,mode7_dv_col_byte_2,mode7_dv_col_byte_3
    patch_copper_long_operand LOOKUP_WORK+64,mode7_du_row_byte_0,mode7_du_row_byte_1,mode7_du_row_byte_2,mode7_du_row_byte_3
    patch_copper_long_operand LOOKUP_WORK+80,mode7_dv_row_byte_0,mode7_dv_row_byte_1,mode7_dv_row_byte_2,mode7_dv_row_byte_3

; Advance the 8.8 angle and scale phases for the following frame. The bit-sliced
; adders propagate carry through 16 bit planes and wrap naturally after bit 15.
    emit_phase_adder ANGLE_PHASE_BITS,313
    emit_phase_adder SCALE_PHASE_BITS,104

; Build the final Mode 7 command. The selected values are two signed 16.16
; origins and four signed 16.16 increments. A 255 mask wraps both texture axes
; across the embedded 256 by 256 image.
    write_shadow_bytes BLT_OP,BLT_OP_MODE7,2
    write_shadow_suffix BLT_SRC+2,((TEXTURE_BASE>>16)&255)
    write_shadow_suffix BLT_SRC+3,((TEXTURE_BASE>>24)&255)
    write_shadow_suffix BLT_DST+2,((FRAMEBUFFER_BASE>>16)&255)
    write_shadow_suffix BLT_DST+3,((FRAMEBUFFER_BASE>>24)&255)
    write_shadow_bytes BLT_WIDTH,DISPLAY_WIDTH,3
    write_shadow_bytes BLT_HEIGHT,DISPLAY_HEIGHT,3
    write_shadow_suffix BLT_SRC_STRIDE+1,((TEXTURE_ROW_BYTES>>8)&255)
    write_shadow_suffix BLT_SRC_STRIDE+2,((TEXTURE_ROW_BYTES>>16)&255)
    write_shadow_suffix BLT_DST_STRIDE+1,((FRAMEBUFFER_ROW_BYTES>>8)&255)
    write_shadow_suffix BLT_DST_STRIDE+2,((FRAMEBUFFER_ROW_BYTES>>16)&255)
    write_patchable_shadow_long BLT_MODE7_U0,0,mode7_u0_byte_0,mode7_u0_byte_1,mode7_u0_byte_2,mode7_u0_byte_3
    write_patchable_shadow_long BLT_MODE7_V0,0,mode7_v0_byte_0,mode7_v0_byte_1,mode7_v0_byte_2,mode7_v0_byte_3
    write_patchable_shadow_long BLT_MODE7_DU_COL,0,mode7_du_col_byte_0,mode7_du_col_byte_1,mode7_du_col_byte_2,mode7_du_col_byte_3
    write_patchable_shadow_long BLT_MODE7_DV_COL,0,mode7_dv_col_byte_0,mode7_dv_col_byte_1,mode7_dv_col_byte_2,mode7_dv_col_byte_3
    write_patchable_shadow_long BLT_MODE7_DU_ROW,0,mode7_du_row_byte_0,mode7_du_row_byte_1,mode7_du_row_byte_2,mode7_du_row_byte_3
    write_patchable_shadow_long BLT_MODE7_DV_ROW,0,mode7_dv_row_byte_0,mode7_dv_row_byte_1,mode7_dv_row_byte_2,mode7_dv_row_byte_3
    write_shadow_bytes BLT_MODE7_TEX_W,TEXTURE_MASK,2
    write_shadow_bytes BLT_MODE7_TEX_H,TEXTURE_MASK,2

; BLT_FLAGS selects RGBA32 Mode 7 pixels and VIDEO_COLOR_MODE selects RGBA32
; presentation. BLT_CTRL completes the Mode 7 render synchronously.
    write_shadow_bytes BLT_FLAGS,0,1
    copper_move VIDEO_COLOR_MODE,COLOUR_MODE_RGBA32
    copper_move BLT_CTRL,1

; Raster suffixes leave VIDEO_FB_BASE pointing into the register shadow.
; Restore VIDEO_FB_BASE to FRAMEBUFFER_BASE before releasing presentation hold.
    copper_move VIDEO_FB_BASE,FRAMEBUFFER_BASE
    copper_move VIDEO_CTRL,VIDEO_CTRL_ENABLE
    dc.l COPPER_END_OPCODE ; END. The next 60 Hz frame restarts at COPPER_LIST_BASE.
