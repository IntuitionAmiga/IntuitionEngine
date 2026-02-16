; stcplay.asm - Sound Tracker Compiled (STC) player for ZX Spectrum
; Assembled with: vasmz80_std -Fbin -o stcplay.bin stcplay.asm
; Loaded at: 0xC000
; INIT (0xC000): HL = module address
; PLAY (0xC003): called 50 times/sec

    .org 0xC000

.set MODULE,       0xC800
.set SPEED,        0xC802
.set POS_COUNT,    0xC803
.set CUR_POS,      0xC804
.set CUR_TICK,     0xC805
.set CUR_ROW,      0xC806

.set CH_STATE,     0xC810
.set CH_SIZE,      16
.set CH_NOTE,      0
.set CH_SAMPLE,    1
.set CH_ORNAMENT,  2
.set CH_VOLUME,    3
.set CH_TONE_L,    4
.set CH_TONE_H,    5
.set CH_PAT_PTR_L, 6
.set CH_PAT_PTR_H, 7
.set CH_SAMP_POS,  8
.set CH_ORN_POS,   9
.set CH_ENV_FLAG,  10
.set CH_NOISE,     11

.set AY_REGS,      0xC870

; === Entry points ===
INIT:
    jp      INIT_IMPL
PLAY:
    jp      PLAY_IMPL

; ============================================================
; INIT implementation
; Input: HL = module address
; ============================================================
INIT_IMPL:
    ld      a,l
    ld      (MODULE),a
    ld      a,h
    ld      (MODULE+1),a

    ; Read speed
    ld      a,(hl)
    or      a
    jr      nz,.speed_ok
    ld      a,3
.speed_ok:
    ld      (SPEED),a
    inc     hl
    ; Read position count
    ld      a,(hl)
    or      a
    jr      nz,.pos_ok
    ld      a,1
.pos_ok:
    ld      (POS_COUNT),a

    xor     a
    ld      (CUR_POS),a
    ld      (CUR_TICK),a
    ld      (CUR_ROW),a

    ; Clear channel state
    ld      hl,CH_STATE
    ld      b,CH_SIZE*3
.clr:
    ld      (hl),0
    inc     hl
    djnz    .clr

    ; Default volumes = 15
    ld      a,15
    ld      (CH_STATE+CH_VOLUME),a
    ld      (CH_STATE+CH_SIZE+CH_VOLUME),a
    ld      (CH_STATE+CH_SIZE*2+CH_VOLUME),a

    ; Clear AY shadow
    ld      hl,AY_REGS
    ld      b,14
.clray:
    ld      (hl),0
    inc     hl
    djnz    .clray

    ; Mixer: tone on all, noise off
    ld      a,0x38
    ld      (AY_REGS+7),a

    ; Load first position
    call    LOAD_POSITION
    ret

; ============================================================
; PLAY implementation - called 50 times/sec
; ============================================================
PLAY_IMPL:
    ld      a,(CUR_TICK)
    or      a
    jr      nz,.tick_only

    ; New row
    call    PROCESS_ROW

    ; Reset tick
    ld      a,(SPEED)
    ld      (CUR_TICK),a

.tick_only:
    ld      a,(CUR_TICK)
    dec     a
    ld      (CUR_TICK),a

    call    CALC_PERIODS
    call    OUTPUT_AY
    ret

; ============================================================
; PROCESS_ROW
; ============================================================
PROCESS_ROW:
    ld      ix,CH_STATE
    call    PROCESS_CHANNEL
    ld      ix,CH_STATE+CH_SIZE
    call    PROCESS_CHANNEL
    ld      ix,CH_STATE+CH_SIZE*2
    call    PROCESS_CHANNEL

    ; Advance row
    ld      a,(CUR_ROW)
    inc     a
    cp      64
    jr      c,.no_next_pos

    ; Next position
    xor     a
    ld      (CUR_ROW),a
    ld      a,(CUR_POS)
    inc     a
    ld      b,a
    ld      a,(POS_COUNT)
    cp      b
    jr      nz,.no_wrap
    xor     a
.no_wrap:
    ld      (CUR_POS),a
    call    LOAD_POSITION
    ret

.no_next_pos:
    ld      (CUR_ROW),a
    ret

; ============================================================
; PROCESS_CHANNEL
; Input: IX = channel state base
; ============================================================
PROCESS_CHANNEL:
    ld      l,(ix+CH_PAT_PTR_L)
    ld      h,(ix+CH_PAT_PTR_H)
    ld      a,h
    or      l
    ret     z

    ld      a,(hl)

    cp      0x80
    ret     z

    or      a
    jr      z,.advance

    cp      0xFF
    jr      z,.silence

    cp      0x70
    jr      nc,.ornament

    cp      0x61
    jr      nc,.sample

    ; Note 1-96
    ld      (ix+CH_NOTE),a
    ld      (ix+CH_SAMP_POS),0
    ld      (ix+CH_ORN_POS),0
    jr      .advance

.silence:
    ld      (ix+CH_VOLUME),0
    jr      .advance

.sample:
    sub     0x61
    ld      (ix+CH_SAMPLE),a
    jr      .advance

.ornament:
    sub     0x70
    ld      (ix+CH_ORNAMENT),a

.advance:
    inc     hl
    ld      (ix+CH_PAT_PTR_L),l
    ld      (ix+CH_PAT_PTR_H),h
    ret

; ============================================================
; LOAD_POSITION
; ============================================================
LOAD_POSITION:
    ld      a,(MODULE)
    ld      l,a
    ld      a,(MODULE+1)
    ld      h,a
    ld      de,2
    add     hl,de
    ld      a,(CUR_POS)
    ld      b,a
    or      a
    jr      z,.got_pos
    ld      de,3
.lp:
    add     hl,de
    djnz    .lp
.got_pos:
    ld      a,l
    ld      (CH_STATE+CH_PAT_PTR_L),a
    ld      a,h
    ld      (CH_STATE+CH_PAT_PTR_H),a
    inc     hl
    ld      a,l
    ld      (CH_STATE+CH_SIZE+CH_PAT_PTR_L),a
    ld      a,h
    ld      (CH_STATE+CH_SIZE+CH_PAT_PTR_H),a
    inc     hl
    ld      a,l
    ld      (CH_STATE+CH_SIZE*2+CH_PAT_PTR_L),a
    ld      a,h
    ld      (CH_STATE+CH_SIZE*2+CH_PAT_PTR_H),a
    ret

; ============================================================
; CALC_PERIODS
; ============================================================
CALC_PERIODS:
    ld      ix,CH_STATE
    ld      iy,AY_REGS
    call    CALC_ONE

    ld      ix,CH_STATE+CH_SIZE
    call    CALC_ONE

    ld      ix,CH_STATE+CH_SIZE*2
    call    CALC_ONE

    ld      a,0x38
    ld      (AY_REGS+7),a

    ld      a,(CH_STATE+CH_VOLUME)
    ld      (AY_REGS+8),a
    ld      a,(CH_STATE+CH_SIZE+CH_VOLUME)
    ld      (AY_REGS+9),a
    ld      a,(CH_STATE+CH_SIZE*2+CH_VOLUME)
    ld      (AY_REGS+10),a
    ret

CALC_ONE:
    ld      a,(ix+CH_NOTE)
    or      a
    jr      z,.silent

    dec     a
    ld      e,a
    ld      d,0
    sla     e
    rl      d
    ld      hl,NOTE_TABLE
    add     hl,de
    ld      e,(hl)
    inc     hl
    ld      d,(hl)

    ld      (iy+0),e
    ld      (iy+1),d
    ld      bc,2
    add     iy,bc
    ret

.silent:
    ld      (iy+0),0
    ld      (iy+1),0
    ld      bc,2
    add     iy,bc
    ret

; ============================================================
; OUTPUT_AY - via ZX Spectrum ports
; ============================================================
OUTPUT_AY:
    ld      hl,AY_REGS
    ld      d,0

.loop:
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
    jr      c,.loop
    ret

; ============================================================
; Note table (8 octaves x 12 semitones)
; ============================================================
NOTE_TABLE:
    .word   0x0D10,0x0C55,0x0BA4,0x0AFC,0x0A5F,0x09CA,0x093D,0x08B8,0x083B,0x07C5,0x0755,0x06EC
    .word   0x0688,0x062A,0x05D2,0x057E,0x052F,0x04E5,0x049E,0x045C,0x041D,0x03E2,0x03AA,0x0376
    .word   0x0344,0x0315,0x02E9,0x02BF,0x0298,0x0272,0x024F,0x022E,0x020F,0x01F1,0x01D5,0x01BB
    .word   0x01A2,0x018B,0x0174,0x015F,0x014C,0x0139,0x0128,0x0117,0x0107,0x00F9,0x00EB,0x00DD
    .word   0x00D1,0x00C5,0x00BA,0x00B0,0x00A6,0x009D,0x0094,0x008C,0x0084,0x007C,0x0075,0x006F
    .word   0x0069,0x0063,0x005D,0x0058,0x0053,0x004E,0x004A,0x0046,0x0042,0x003E,0x003B,0x0037
    .word   0x0034,0x0031,0x002F,0x002C,0x0029,0x0027,0x0025,0x0023,0x0021,0x001F,0x001D,0x001C
    .word   0x001A,0x0019,0x0017,0x0016,0x0015,0x0014,0x0012,0x0011,0x0010,0x000F,0x000F,0x000E
