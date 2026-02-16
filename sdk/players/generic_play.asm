; generic_play.asm - Generic tracker player for simple ZX Spectrum formats
; Assembled with: vasmz80_std -Fbin -o generic_play.bin generic_play.asm
;
; Loaded at: 0xC000
; INIT (0xC000): HL = module address
; PLAY (0xC003): called 50 times/sec
;
; This player handles basic note sequencing and AY register output.
; Used as a fallback for formats without dedicated players.

    .org 0xC000

; === Entry points ===
init:
    jp      init_impl
play:
    jp      play_impl

; === Storage ===
mod_addr:   .word 0
speed:      .byte 3
tick_cnt:   .byte 0
pos_count:  .byte 0
cur_pos:    .byte 0
cur_row:    .byte 0

; AY register shadow buffer
ay_regs:    .space 14

; Per-channel state
ch_a_note:  .byte 0
ch_a_vol:   .byte 15
ch_b_note:  .byte 0
ch_b_vol:   .byte 15
ch_c_note:  .byte 0
ch_c_vol:   .byte 15

; ===========================================
; INIT implementation
; HL = module base address
; ===========================================
init_impl:
    ld      a,l
    ld      (mod_addr),a
    ld      a,h
    ld      (mod_addr+1),a

    ; Read speed from byte 0 of module
    ld      a,(hl)
    or      a
    jr      nz,speed_ok
    ld      a,3
speed_ok:
    ld      (speed),a

    ; Read position count from byte 1
    inc     hl
    ld      a,(hl)
    or      a
    jr      nz,pos_ok
    ld      a,1
pos_ok:
    ld      (pos_count),a

    ; Clear state
    xor     a
    ld      (tick_cnt),a
    ld      (cur_pos),a
    ld      (cur_row),a
    ld      (ch_a_note),a
    ld      (ch_b_note),a
    ld      (ch_c_note),a

    ; Default volumes
    ld      a,15
    ld      (ch_a_vol),a
    ld      (ch_b_vol),a
    ld      (ch_c_vol),a

    ; Clear AY regs
    ld      hl,ay_regs
    ld      b,14
clr_ay:
    ld      (hl),0
    inc     hl
    djnz    clr_ay

    ; Set mixer: tone on for all channels
    ld      a,0x38
    ld      (ay_regs+7),a

    ret

; ===========================================
; PLAY implementation
; Called 50 times/sec
; ===========================================
play_impl:
    ; Check tick counter
    ld      a,(tick_cnt)
    or      a
    jr      nz,tick_dec

    ; New row - advance position if row = 64
    ld      a,(cur_row)
    cp      64
    jr      c,no_pos_advance

    ; Next position
    xor     a
    ld      (cur_row),a
    ld      a,(cur_pos)
    inc     a
    ld      b,a
    ld      a,(pos_count)
    cp      b
    jr      nz,no_wrap
    xor     a
no_wrap:
    ld      (cur_pos),a

no_pos_advance:
    ; Increment row counter
    ld      a,(cur_row)
    inc     a
    ld      (cur_row),a

    ; Reset tick counter
    ld      a,(speed)
    ld      (tick_cnt),a

tick_dec:
    ld      a,(tick_cnt)
    dec     a
    ld      (tick_cnt),a

    ; Calculate note periods and set AY registers
    call    calc_all_periods

    ; Output to AY chip
    call    output_ay

    ret

; ===========================================
; Calculate periods for all channels
; ===========================================
calc_all_periods:
    ; Channel A
    ld      a,(ch_a_note)
    call    get_period
    ld      a,e
    ld      (ay_regs+0),a
    ld      a,d
    ld      (ay_regs+1),a
    ld      a,(ch_a_vol)
    ld      (ay_regs+8),a

    ; Channel B
    ld      a,(ch_b_note)
    call    get_period
    ld      a,e
    ld      (ay_regs+2),a
    ld      a,d
    ld      (ay_regs+3),a
    ld      a,(ch_b_vol)
    ld      (ay_regs+9),a

    ; Channel C
    ld      a,(ch_c_note)
    call    get_period
    ld      a,e
    ld      (ay_regs+4),a
    ld      a,d
    ld      (ay_regs+5),a
    ld      a,(ch_c_vol)
    ld      (ay_regs+10),a

    ret

; Get period for note in A
; Returns DE = period (0 if note is 0)
get_period:
    or      a
    jr      nz,has_note
    ld      de,0
    ret
has_note:
    dec     a
    ld      e,a
    ld      d,0
    sla     e
    rl      d
    ld      hl,note_table
    add     hl,de
    ld      e,(hl)
    inc     hl
    ld      d,(hl)
    ret

; ===========================================
; Output AY registers via ZX Spectrum ports
; ===========================================
output_ay:
    ld      hl,ay_regs
    ld      d,0

oa_loop:
    ld      a,d
    ld      bc,0xFFFD
    out     (c),a
    ld      a,(hl)
    ld      bc,0xBFFD
    out     (c),a
    inc     hl
    inc     d
    ld      a,d
    cp      14
    jr      c,oa_loop
    ret

; ===========================================
; Note table - Standard ZX Spectrum AY periods
; 96 notes (8 octaves x 12 semitones)
; ===========================================
note_table:
    .word   0x0D10,0x0C55,0x0BA4,0x0AFC,0x0A5F,0x09CA,0x093D,0x08B8,0x083B,0x07C5,0x0755,0x06EC
    .word   0x0688,0x062A,0x05D2,0x057E,0x052F,0x04E5,0x049E,0x045C,0x041D,0x03E2,0x03AA,0x0376
    .word   0x0344,0x0315,0x02E9,0x02BF,0x0298,0x0272,0x024F,0x022E,0x020F,0x01F1,0x01D5,0x01BB
    .word   0x01A2,0x018B,0x0174,0x015F,0x014C,0x0139,0x0128,0x0117,0x0107,0x00F9,0x00EB,0x00DD
    .word   0x00D1,0x00C5,0x00BA,0x00B0,0x00A6,0x009D,0x0094,0x008C,0x0084,0x007C,0x0075,0x006F
    .word   0x0069,0x0063,0x005D,0x0058,0x0053,0x004E,0x004A,0x0046,0x0042,0x003E,0x003B,0x0037
    .word   0x0034,0x0031,0x002F,0x002C,0x0029,0x0027,0x0025,0x0023,0x0021,0x001F,0x001D,0x001C
    .word   0x001A,0x0019,0x0017,0x0016,0x0015,0x0014,0x0012,0x0011,0x0010,0x000F,0x000F,0x000E
