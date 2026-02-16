; ============================================================================
; PT3 PLAYER - ProTracker 3.x (Vortex Tracker II) Playback Engine
; Z80 Assembly for ZX Spectrum AY-3-8910/8912 PSG
; ============================================================================
;
; === OVERVIEW ===
; Complete playback engine for ProTracker 3 (.pt3) modules, compatible with
; PT3 version 3.3 through 3.7. Based on the well-known PT3PROM player
; structure by Sergey Bulba / Alone Coder.
;
; === ENTRY POINTS ===
;   PT3_INIT (0xC000): Initialise player with module address in HL
;   PT3_PLAY (0xC003): Call 50 times/second to advance playback
;   PT3_MUTE (0xC006): Silence all channels immediately
;
; === CALLING CONVENTION ===
;   INIT: HL = pointer to start of PT3 module data in memory
;         Parses header, sets up channel pointers, resets state.
;   PLAY: No arguments. Advances one tick (or one row if tick counter
;         reaches zero). Writes computed AY register values via OUT.
;   MUTE: No arguments. Writes zero volume to all channels.
;
; === AY-3-8910 REGISTER ACCESS ===
;   Port 0xFFFD: Write register number (select)
;   Port 0xBFFD: Write register data
;   Standard ZX Spectrum 128K AY port addresses.
;
; === PT3 MODULE FORMAT (key offsets from module base) ===
;   +0x00..0x62: Header (song name at +0x1E, etc.)
;   +0x63:       Speed (ticks per row, typically 3-6)
;   +0x64:       Number of positions in song
;   +0x65:       Loop position (position to restart from)
;   +0x66...:    Position list (each byte = pattern number)
;   +0x02..0x03: Offset to patterns table (from module start)
;   +0x04..0x05: Offset to samples table (from module start)
;   +0x06..0x07: Offset to ornaments table (from module start)
;
; === ASSEMBLER ===
;   vasmz80_std -Fbin -o pt3play.bin pt3play.asm
;
; === REFERENCES ===
;   Sergey Bulba's PT3PROM universal player
;   Vortex Tracker II documentation
;   AY-3-8910 Programmable Sound Generator datasheet
;
; ============================================================================

    .org 0xC000

; ============================================================================
; ENTRY POINT JUMP TABLE
; ============================================================================
; Three fixed entry points at known addresses for the host program.
; The host calls PT3_INIT once, then PT3_PLAY at 50Hz (e.g. from an ISR).

PT3_INIT:
    jp      init_player
PT3_PLAY:
    jp      play_tick
PT3_MUTE:
    jp      mute_ay

; ============================================================================
; AY PORT CONSTANTS
; ============================================================================

.set AY_REG_PORT,  0xFFFD          ; AY register select port
.set AY_DATA_PORT, 0xBFFD          ; AY data write port

; ============================================================================
; PT3 MODULE HEADER OFFSETS
; ============================================================================

.set PT3_OFS_PAT_TABLE,    0x02    ; Word: offset to patterns table
.set PT3_OFS_SAM_TABLE,    0x04    ; Word: offset to samples table
.set PT3_OFS_ORN_TABLE,    0x06    ; Word: offset to ornaments table
.set PT3_OFS_SPEED,        0x63    ; Byte: initial speed (ticks/row)
.set PT3_OFS_NUM_POS,      0x64    ; Byte: number of positions
.set PT3_OFS_LOOP_POS,     0x65    ; Byte: loop position index
.set PT3_OFS_POS_LIST,     0x66    ; Bytes: position list

; ============================================================================
; CHANNEL DATA STRUCTURE OFFSETS
; ============================================================================
; Each channel (A, B, C) has a block of state variables accessed via IX.

.set CH_PAT_PTR,           0       ; Word: current position in pattern data
.set CH_SAM_PTR,           2       ; Word: pointer to current sample data
.set CH_ORN_PTR,           4       ; Word: pointer to current ornament data
.set CH_SAM_POS,           6       ; Byte: position within sample
.set CH_SAM_LEN,           7       ; Byte: sample length (total entries)
.set CH_SAM_LOOP,          8       ; Byte: sample loop start position
.set CH_ORN_POS,           9       ; Byte: position within ornament
.set CH_ORN_LEN,           10      ; Byte: ornament length
.set CH_ORN_LOOP,          11      ; Byte: ornament loop start position
.set CH_TONE,              12      ; Word: base note tone period
.set CH_VOLUME,            14      ; Byte: channel volume (0-15)
.set CH_NOTE,              15      ; Byte: current note number (0-95)
.set CH_ENABLED,           16      ; Byte: channel on/off (0=off, 1=on)
.set CH_ENVELOPE,          17      ; Byte: envelope enable flag
.set CH_TONE_SLIDE_VAL,    18      ; Word: tone slide step (signed)
.set CH_TONE_SLIDE_ACC,    20      ; Word: tone slide accumulator
.set CH_TONE_SLIDE_TO,     22      ; Word: tone slide target period
.set CH_TONE_SLIDE_ON,     24      ; Byte: tone slide active flag
.set CH_TONE_DELTA,        25      ; Word: current tone adjustment
.set CH_VIB_DELAY,         27      ; Byte: vibrato/slide delay counter
.set CH_VIB_DELAY_INIT,    28      ; Byte: vibrato delay reload value
.set CH_ARP_POS,           29      ; Byte: arpeggio table position (0-2)
.set CH_ARP_NOTE1,         30      ; Byte: arpeggio note offset 1
.set CH_ARP_NOTE2,         31      ; Byte: arpeggio note offset 2
.set CH_ARP_ON,            32      ; Byte: arpeggio active flag
.set CH_ENV_SLIDE_ADD,     33      ; Word: envelope period slide step
.set CH_NOISE_ADD,         35      ; Byte: noise pitch offset from sample
; Per-channel computed output (stored by process_channel, read by write_ay)
.set CH_OUT_TONE_LO,       36      ; Byte: computed tone period low
.set CH_OUT_TONE_HI,       37      ; Byte: computed tone period high
.set CH_OUT_VOL,           38      ; Byte: computed volume
.set CH_OUT_NOISE,         39      ; Byte: computed noise value
.set CH_OUT_TONE_ON,       40      ; Byte: tone enable (1=on)
.set CH_OUT_NOISE_ON,      41      ; Byte: noise enable (1=on)
.set CH_OUT_ENV_ON,        42      ; Byte: envelope enable
.set CH_SIZE,              43      ; Size of one channel data block

; ============================================================================
; INITIALISE PLAYER
; ============================================================================
; Input: HL = pointer to PT3 module data
; Sets up all playback state from the module header.

init_player:
    ld      (module_addr),hl

    ; --- Read speed ---
    push    hl
    ld      de,PT3_OFS_SPEED
    add     hl,de
    ld      a,(hl)
    or      a
    jr      nz,.speed_ok
    ld      a,6                    ; Default speed if header says 0
.speed_ok:
    ld      (speed),a
    ld      (tick_count),a         ; Force new row on first PLAY call
    pop     hl

    ; --- Read number of positions and loop position ---
    push    hl
    ld      de,PT3_OFS_NUM_POS
    add     hl,de
    ld      a,(hl)
    ld      (num_positions),a
    inc     hl
    ld      a,(hl)
    ld      (loop_position),a
    pop     hl

    ; --- Compute absolute pointers to tables ---
    ; Patterns table: module + word at module+0x02
    push    hl
    ld      de,PT3_OFS_PAT_TABLE
    add     hl,de
    ld      e,(hl)
    inc     hl
    ld      d,(hl)                 ; DE = offset to patterns table
    pop     hl
    push    hl
    push    de
    pop     bc                     ; BC = patterns offset
    add     hl,bc                  ; HL = absolute patterns table address
    ld      (pat_table_addr),hl
    pop     hl

    ; Samples table: module + word at module+0x04
    push    hl
    ld      de,PT3_OFS_SAM_TABLE
    add     hl,de
    ld      e,(hl)
    inc     hl
    ld      d,(hl)
    pop     hl
    push    hl
    push    de
    pop     bc
    add     hl,bc
    ld      (sam_table_addr),hl
    pop     hl

    ; Ornaments table: module + word at module+0x06
    push    hl
    ld      de,PT3_OFS_ORN_TABLE
    add     hl,de
    ld      e,(hl)
    inc     hl
    ld      d,(hl)
    pop     hl
    push    hl
    push    de
    pop     bc
    add     hl,bc
    ld      (orn_table_addr),hl
    pop     hl

    ; --- Compute position list address ---
    push    hl
    ld      de,PT3_OFS_POS_LIST
    add     hl,de
    ld      (pos_list_addr),hl
    pop     hl

    ; --- Reset playback position ---
    xor     a
    ld      (current_pos),a

    ; --- Clear channel data ---
    ld      hl,chan_a
    ld      de,chan_a+1
    ld      bc,CH_SIZE*3-1
    ld      (hl),0
    ldir

    ; --- Set default volume for all channels ---
    ld      a,15
    ld      (chan_a+CH_VOLUME),a
    ld      (chan_b+CH_VOLUME),a
    ld      (chan_c+CH_VOLUME),a

    ; --- Reset global state ---
    xor     a
    ld      (env_base_lo),a
    ld      (env_base_hi),a
    ld      (env_shape),a
    ld      (env_new_shape),a
    ld      (env_slide_lo),a
    ld      (env_slide_hi),a
    ld      (noise_base),a
    ld      (env_delay),a
    ld      (env_delay_cnt),a

    ; --- Load first position's patterns ---
    call    load_position

    ret

; ============================================================================
; PLAY ONE TICK
; ============================================================================
; Called 50 times per second. Decrements the tick counter; when it reaches
; zero, reads the next row of pattern data. Then processes samples,
; ornaments, and effects for all three channels, and writes AY registers.

play_tick:
    ; --- Check tick counter ---
    ld      a,(tick_count)
    dec     a
    ld      (tick_count),a
    or      a
    jr      nz,.no_new_row

    ; --- Tick counter expired: read new row ---
    ld      a,(speed)
    ld      (tick_count),a

    ; Process pattern data for each channel
    ld      ix,chan_a
    call    read_pattern_data
    ld      ix,chan_b
    call    read_pattern_data
    ld      ix,chan_c
    call    read_pattern_data

.no_new_row:
    ; --- Process samples/ornaments/effects for each channel ---
    ; Each call stores results into the channel's CH_OUT_* fields
    ld      ix,chan_a
    call    process_channel
    ld      ix,chan_b
    call    process_channel
    ld      ix,chan_c
    call    process_channel

    ; --- Handle envelope period slide ---
    call    process_env_slide

    ; --- Write all AY registers ---
    call    write_ay_regs

    ret

; ============================================================================
; READ PATTERN DATA FOR ONE CHANNEL
; ============================================================================
; IX points to the channel data block. Reads and decodes pattern bytes
; from the channel's current pattern pointer.
;
; PT3 pattern data encoding:
;   0x00..0x5F: Note (0-95), triggers new note
;   0x60..0x6F: Sample number (sample = byte - 0x60)
;   0x70..0x7F: Ornament number + envelope off (ornament = byte - 0x70)
;   0x80:       Rest (silence channel)
;   0x81:       Empty (no change this row)
;   0x82..0x8E: Ornament (ornament = byte - 0x80), keep envelope
;   0xB0:       Envelope command (shape, period_hi, period_lo follow)
;   0xB1..0xBF: Set speed (new speed = byte - 0xB0)
;   0xC0:       End of pattern
;   0xC1..0xCF: Volume (volume = byte - 0xC0)
;   0xD0:       End of pattern / break
;   0xD1..0xFF: Effect commands (effect number in lower bits, params follow)

read_pattern_data:
    ld      l,(ix+CH_PAT_PTR)
    ld      h,(ix+CH_PAT_PTR+1)

.read_loop:
    ld      a,(hl)
    inc     hl

    ; --- Check for note (0x00..0x5F) ---
    cp      0x60
    jp      c,.is_note

    ; --- Check for sample select (0x60..0x6F) ---
    cp      0x70
    jr      c,.is_sample

    ; --- Check for ornament + envelope off (0x70..0x7F) ---
    cp      0x80
    jp      c,.is_ornament_envoff

    ; --- Check for rest (0x80) ---
    cp      0x80
    jp      z,.is_rest

    ; --- Check for empty row (0x81) ---
    cp      0x81
    jp      z,.is_empty

    ; --- Check for ornament keep envelope (0x82..0x8E) ---
    cp      0x8F
    jp      c,.is_ornament_keep

    ; --- Check for envelope command (0xB0) ---
    cp      0xB0
    jp      z,.is_envelope

    ; --- Check for speed change (0xB1..0xBF) ---
    cp      0xC0
    jp      c,.is_speed

    ; --- Check for end of pattern (0xC0) ---
    cp      0xC0
    jp      z,.is_end_pattern

    ; --- Check for volume (0xC1..0xCF) ---
    cp      0xD0
    jp      c,.is_volume

    ; --- Check for end of pattern/break (0xD0) ---
    cp      0xD0
    jp      z,.is_end_pattern

    ; --- Effect commands (0xD1..0xFF) ---
    jp      .is_effect

; --- Note handling ---
.is_note:
    ld      (ix+CH_NOTE),a
    ld      (ix+CH_ENABLED),1

    ; Reset sample position
    ld      (ix+CH_SAM_POS),0
    ; Reset ornament position
    ld      (ix+CH_ORN_POS),0
    ; Clear tone slide
    ld      (ix+CH_TONE_SLIDE_ON),0
    ld      (ix+CH_TONE_SLIDE_ACC),0
    ld      (ix+CH_TONE_SLIDE_ACC+1),0
    ; Clear arpeggio
    ld      (ix+CH_ARP_ON),0
    ld      (ix+CH_ARP_POS),0
    ; Clear tone delta
    ld      (ix+CH_TONE_DELTA),0
    ld      (ix+CH_TONE_DELTA+1),0

    ; Look up note period
    push    hl
    ld      a,(ix+CH_NOTE)
    call    get_note_period
    ld      (ix+CH_TONE),e
    ld      (ix+CH_TONE+1),d
    pop     hl

    jp      .read_loop

; --- Sample select ---
.is_sample:
    sub     0x60
    push    hl
    call    set_sample
    pop     hl
    jp      .read_loop

; --- Ornament + envelope off ---
.is_ornament_envoff:
    sub     0x70
    ld      (ix+CH_ENVELOPE),0
    push    hl
    call    set_ornament
    pop     hl
    jp      .read_loop

; --- Rest ---
.is_rest:
    ld      (ix+CH_ENABLED),0
    jp      .done_read

; --- Empty row ---
.is_empty:
    jp      .done_read

; --- Ornament, keep envelope ---
.is_ornament_keep:
    sub     0x80
    push    hl
    call    set_ornament
    pop     hl
    ld      (ix+CH_ORN_POS),0
    jp      .read_loop

; --- Envelope command ---
.is_envelope:
    ld      a,(hl)
    inc     hl
    ld      (env_new_shape),a      ; Mark new envelope shape to write
    ld      a,(hl)
    inc     hl
    ld      (env_base_hi),a
    ld      a,(hl)
    inc     hl
    ld      (env_base_lo),a
    ld      (ix+CH_ENVELOPE),1
    ; Reset envelope slide accumulator
    xor     a
    ld      (env_slide_lo),a
    ld      (env_slide_hi),a
    ld      (env_delay_cnt),a
    jp      .read_loop

; --- Speed change ---
.is_speed:
    sub     0xB0
    ld      (speed),a
    jp      .read_loop

; --- End of pattern ---
.is_end_pattern:
    ; Advance to next position
    push    ix
    call    next_position
    pop     ix
    ; Reload pattern pointer for this channel from the new position
    ld      l,(ix+CH_PAT_PTR)
    ld      h,(ix+CH_PAT_PTR+1)
    jp      .read_loop

; --- Volume ---
.is_volume:
    sub     0xC0
    ld      (ix+CH_VOLUME),a
    jp      .read_loop

; --- Effect command ---
.is_effect:
    ; A = command byte (0xD1..0xFF)
    sub     0xD0
    ; A = effect number (1..0x2F)

    cp      0x01
    jp      z,.eff_tone_up
    cp      0x02
    jp      z,.eff_tone_down
    cp      0x03
    jp      z,.eff_portamento
    cp      0x08
    jp      z,.eff_arpeggio
    cp      0x09
    jp      z,.eff_env_slide
    cp      0x0B
    jp      z,.eff_fine_pitch

    ; Unknown effect: skip 2 parameter bytes
    inc     hl
    inc     hl
    jp      .read_loop

; --- Effect: Tone slide up (1xx) ---
; Param 1: delay, Param 2: slide step (positive = upward = decreasing period)
.eff_tone_up:
    ld      a,(hl)
    inc     hl
    ld      (ix+CH_VIB_DELAY_INIT),a
    ld      (ix+CH_VIB_DELAY),a
    ld      a,(hl)
    inc     hl
    ; Negative step = decreasing period = higher pitch
    neg
    ld      (ix+CH_TONE_SLIDE_VAL),a
    ld      (ix+CH_TONE_SLIDE_VAL+1),0xFF ; Sign extend negative
    ld      (ix+CH_TONE_SLIDE_ON),1
    ld      (ix+CH_TONE_SLIDE_TO),0
    ld      (ix+CH_TONE_SLIDE_TO+1),0
    jp      .read_loop

; --- Effect: Tone slide down (2xx) ---
; Param 1: delay, Param 2: slide step (positive = downward = increasing period)
.eff_tone_down:
    ld      a,(hl)
    inc     hl
    ld      (ix+CH_VIB_DELAY_INIT),a
    ld      (ix+CH_VIB_DELAY),a
    ld      a,(hl)
    inc     hl
    ld      (ix+CH_TONE_SLIDE_VAL),a
    ld      (ix+CH_TONE_SLIDE_VAL+1),0
    ld      (ix+CH_TONE_SLIDE_ON),1
    ld      (ix+CH_TONE_SLIDE_TO),0
    ld      (ix+CH_TONE_SLIDE_TO+1),0
    jp      .read_loop

; --- Effect: Tone portamento (3xx) ---
; Param 1: delay, Param 2: slide step magnitude
; Direction is determined by comparing current period to target.
.eff_portamento:
    ld      a,(hl)
    inc     hl
    ld      (ix+CH_VIB_DELAY_INIT),a
    ld      (ix+CH_VIB_DELAY),a
    ld      b,(hl)                 ; B = slide magnitude
    inc     hl

    ; Compare current tone to the slide target to determine direction
    push    hl
    ld      e,(ix+CH_TONE)
    ld      d,(ix+CH_TONE+1)
    ld      l,(ix+CH_TONE_SLIDE_TO)
    ld      h,(ix+CH_TONE_SLIDE_TO+1)
    or      a
    sbc     hl,de                  ; target - current
    pop     hl
    jr      nc,.port_positive
    ; Target < current: slide down (decrease period) = negative step
    ld      a,b
    neg
    ld      (ix+CH_TONE_SLIDE_VAL),a
    ld      (ix+CH_TONE_SLIDE_VAL+1),0xFF
    jr      .port_done
.port_positive:
    ; Target > current: slide up (increase period) = positive step
    ld      (ix+CH_TONE_SLIDE_VAL),b
    ld      (ix+CH_TONE_SLIDE_VAL+1),0
.port_done:
    ld      (ix+CH_TONE_SLIDE_ON),1
    jp      .read_loop

; --- Effect: Arpeggio (8xy) ---
; Param byte: high nibble = note offset 1, low nibble = note offset 2
.eff_arpeggio:
    ld      a,(hl)
    inc     hl
    ld      b,a                    ; Save original byte
    ; High nibble = note offset 1
    rrca
    rrca
    rrca
    rrca
    and     0x0F
    ld      (ix+CH_ARP_NOTE1),a
    ; Low nibble = note offset 2
    ld      a,b
    and     0x0F
    ld      (ix+CH_ARP_NOTE2),a
    ld      (ix+CH_ARP_ON),1
    ld      (ix+CH_ARP_POS),0
    ; Skip second parameter byte
    inc     hl
    jp      .read_loop

; --- Effect: Envelope slide (9xy) ---
; Param 1: delay, Param 2: slide step (signed)
.eff_env_slide:
    ld      a,(hl)
    inc     hl
    ld      (env_delay),a
    ld      (env_delay_cnt),a
    ld      a,(hl)
    inc     hl
    ld      (env_slide_lo),a
    ; Sign extend
    bit     7,a
    jr      z,.env_pos
    ld      a,0xFF
    ld      (env_slide_hi),a
    jp      .read_loop
.env_pos:
    xor     a
    ld      (env_slide_hi),a
    jp      .read_loop

; --- Effect: Fine pitch (Bxx) ---
; Param: signed pitch offset
.eff_fine_pitch:
    ld      a,(hl)
    inc     hl
    ld      (ix+CH_TONE_DELTA),a
    ; Sign extend
    bit     7,a
    jr      z,.fp_pos
    ld      (ix+CH_TONE_DELTA+1),0xFF
    jr      .fp_skip
.fp_pos:
    ld      (ix+CH_TONE_DELTA+1),0
.fp_skip:
    ; Skip second parameter byte
    inc     hl
    jr      .done_read

.done_read:
    ; Save updated pattern pointer back into channel
    ld      (ix+CH_PAT_PTR),l
    ld      (ix+CH_PAT_PTR+1),h
    ret

; ============================================================================
; SET SAMPLE FOR CHANNEL
; ============================================================================
; A = sample number, IX = channel data
; Looks up sample in the samples table and sets the channel's sample pointer,
; length, and loop point.

set_sample:
    ld      hl,(sam_table_addr)

    ; Each entry in the table is a 2-byte offset from module start
    ld      e,a
    ld      d,0
    add     hl,de
    add     hl,de              ; HL = sam_table + sample_num * 2

    ld      e,(hl)
    inc     hl
    ld      d,(hl)             ; DE = offset to sample data from module

    ld      hl,(module_addr)
    add     hl,de              ; HL = absolute address of sample data

    ; Sample header: byte 0 = loop start, byte 1 = length
    ; then sample data frames follow
    ld      a,(hl)
    ld      (ix+CH_SAM_LOOP),a
    inc     hl
    ld      a,(hl)
    ld      (ix+CH_SAM_LEN),a
    inc     hl

    ; HL now points to the actual sample frame data
    ld      (ix+CH_SAM_PTR),l
    ld      (ix+CH_SAM_PTR+1),h
    ld      (ix+CH_SAM_POS),0

    ret

; ============================================================================
; SET ORNAMENT FOR CHANNEL
; ============================================================================
; A = ornament number, IX = channel data

set_ornament:
    ld      hl,(orn_table_addr)

    ld      e,a
    ld      d,0
    add     hl,de
    add     hl,de              ; HL = orn_table + ornament_num * 2

    ld      e,(hl)
    inc     hl
    ld      d,(hl)             ; DE = offset to ornament data from module

    ld      hl,(module_addr)
    add     hl,de              ; HL = absolute ornament data address

    ; Ornament header: byte 0 = loop start, byte 1 = length
    ; then ornament data follows (signed note offsets, one byte each)
    ld      a,(hl)
    ld      (ix+CH_ORN_LOOP),a
    inc     hl
    ld      a,(hl)
    ld      (ix+CH_ORN_LEN),a
    inc     hl

    ld      (ix+CH_ORN_PTR),l
    ld      (ix+CH_ORN_PTR+1),h
    ld      (ix+CH_ORN_POS),0

    ret

; ============================================================================
; PROCESS CHANNEL (samples, ornaments, effects)
; ============================================================================
; IX = channel data block pointer
; Computes the final tone period, volume, noise, and envelope settings
; for this channel by combining the base note with sample data, ornament
; data, and any active effects.
;
; Results are stored in the channel's CH_OUT_* fields.

process_channel:
    ; Check if channel is enabled
    ld      a,(ix+CH_ENABLED)
    or      a
    jr      nz,.ch_active

    ; Channel is off: zero output
    ld      (ix+CH_OUT_VOL),0
    ld      (ix+CH_OUT_TONE_LO),0
    ld      (ix+CH_OUT_TONE_HI),0
    ld      (ix+CH_OUT_NOISE),0
    ld      (ix+CH_OUT_TONE_ON),0
    ld      (ix+CH_OUT_NOISE_ON),0
    ld      (ix+CH_OUT_ENV_ON),0
    ret

.ch_active:
    ; --- Process sample ---
    ; Read sample data at current position
    ; Each sample frame is 4 bytes:
    ;   Byte 0: bit 7 = tone on, bit 6 = noise on, bits 3-0 = volume level
    ;   Byte 1: noise pitch offset
    ;   Byte 2: tone offset low (signed 16-bit with byte 3)
    ;   Byte 3: tone offset high

    ld      l,(ix+CH_SAM_PTR)
    ld      h,(ix+CH_SAM_PTR+1)
    ld      a,(ix+CH_SAM_POS)

    ; Compute address: base + pos * 4
    ld      b,a                ; Save position
    add     a,a                ; *2
    add     a,b                ; *3
    add     a,b                ; *4
    ld      e,a
    ld      d,0
    add     hl,de              ; HL = sample data + pos * 4

    ; Read sample frame
    ld      b,(hl)             ; B = flags byte (tone/noise/volume)
    inc     hl
    ld      c,(hl)             ; C = noise offset
    inc     hl
    ld      e,(hl)             ; E = tone offset low
    inc     hl
    ld      d,(hl)             ; D = tone offset high
    ; DE = signed tone offset, B = flags, C = noise offset

    ; Advance sample position
    ld      a,(ix+CH_SAM_POS)
    inc     a
    cp      (ix+CH_SAM_LEN)
    jr      c,.sam_no_wrap
    ld      a,(ix+CH_SAM_LOOP) ; Wrap to loop point
.sam_no_wrap:
    ld      (ix+CH_SAM_POS),a

    ; --- Save sample frame data to temporaries ---
    ; Store DE (tone offset) for later use
    push    de                 ; Save tone offset on stack
    push    bc                 ; Save flags(B) and noise(C)

    ; --- Extract flags ---
    ; Tone enable (bit 7 of B)
    bit     7,b
    jr      z,.tone_off
    ld      (ix+CH_OUT_TONE_ON),1
    jr      .tone_done
.tone_off:
    ld      (ix+CH_OUT_TONE_ON),0
.tone_done:

    ; Noise enable (bit 6 of B)
    bit     6,b
    jr      z,.noise_off
    ld      (ix+CH_OUT_NOISE_ON),1
    jr      .noise_done
.noise_off:
    ld      (ix+CH_OUT_NOISE_ON),0
.noise_done:

    ; --- Compute volume ---
    ; Sample volume (bits 3-0 of B) * channel volume / 15
    ld      a,b
    and     0x0F               ; A = sample volume (0-15)
    ld      d,a                ; D = sample volume
    ld      a,(ix+CH_VOLUME)   ; A = channel volume (0-15)

    ; Multiply D * A using the volume table for speed
    ; Or use simple approach: result = (D * A + 7) >> 4
    ld      e,a                ; E = channel volume
    ; Quick 8x8 multiply: D * E -> HL
    ld      h,0
    ld      l,d
    ld      d,0                ; DE = channel volume
    ; HL = sample volume, DE = channel volume
    ; Multiply HL * E (8-bit)
    ld      a,l                ; A = sample_vol
    ld      l,0                ; HL = 0 (accumulator)
    ld      b,8
.vol_mul:
    add     hl,hl
    rla
    jr      nc,.vol_no_add
    add     hl,de
.vol_no_add:
    djnz    .vol_mul
    ; HL = sample_vol * channel_vol (max 225 = 15*15)
    ; Divide by 15: approximate as (value + 7) >> 4
    ld      a,l
    add     a,7
    rrca
    rrca
    rrca
    rrca
    and     0x0F               ; Clamp to 0-15
    ld      (ix+CH_OUT_VOL),a

    ; --- Restore noise value ---
    pop     bc                 ; Restore B=flags, C=noise
    ld      (ix+CH_OUT_NOISE),c
    ld      (ix+CH_NOISE_ADD),c

    ; --- Process ornament ---
    ; Read the signed note offset at the current ornament position
    ld      l,(ix+CH_ORN_PTR)
    ld      h,(ix+CH_ORN_PTR+1)
    ld      a,(ix+CH_ORN_POS)
    ld      e,a
    ld      d,0
    add     hl,de
    ld      a,(hl)             ; A = signed ornament note offset

    ; Advance ornament position
    push    af                 ; Save ornament value
    ld      a,(ix+CH_ORN_POS)
    inc     a
    cp      (ix+CH_ORN_LEN)
    jr      c,.orn_no_wrap
    ld      a,(ix+CH_ORN_LOOP)
.orn_no_wrap:
    ld      (ix+CH_ORN_POS),a
    pop     af                 ; Restore ornament note offset in A

    ; --- Compute effective note ---
    ld      b,a                ; B = ornament offset
    ld      a,(ix+CH_NOTE)
    add     a,b                ; A = note + ornament offset

    ; --- Apply arpeggio ---
    ld      c,a                ; C = current effective note
    ld      a,(ix+CH_ARP_ON)
    or      a
    jr      z,.no_arp

    ; Arpeggio cycles through 3 positions: base, +note1, +note2
    ld      a,(ix+CH_ARP_POS)
    or      a
    jr      z,.arp_advance     ; Position 0: no change
    cp      1
    jr      z,.arp_1
    ; Position 2:
    ld      a,(ix+CH_ARP_NOTE2)
    add     a,c
    ld      c,a
    jr      .arp_advance
.arp_1:
    ld      a,(ix+CH_ARP_NOTE1)
    add     a,c
    ld      c,a
.arp_advance:
    ld      a,(ix+CH_ARP_POS)
    inc     a
    cp      3
    jr      c,.arp_save
    xor     a
.arp_save:
    ld      (ix+CH_ARP_POS),a

.no_arp:
    ; Clamp note to valid range 0..95
    ld      a,c
    bit     7,a
    jr      nz,.note_clamp_lo  ; Negative -> clamp to 0
    cp      96
    jr      c,.note_ok
    ld      a,95               ; Too high -> clamp to 95
    jr      .note_ok
.note_clamp_lo:
    xor     a
.note_ok:
    ; Look up tone period for effective note
    call    get_note_period    ; DE = period for note A

    ; --- Add sample tone offset ---
    ; Tone offset was pushed onto stack earlier
    pop     hl                 ; HL = sample tone offset (signed 16-bit)
    add     hl,de              ; HL = period + sample tone offset
    ex      de,hl              ; DE = adjusted period

    ; --- Add tone delta (fine pitch effect) ---
    ld      l,(ix+CH_TONE_DELTA)
    ld      h,(ix+CH_TONE_DELTA+1)
    add     hl,de              ; HL = period + delta
    ex      de,hl              ; DE = final period (before slide)

    ; --- Apply tone slide ---
    ld      a,(ix+CH_TONE_SLIDE_ON)
    or      a
    jr      z,.no_slide

    ; Check delay
    ld      a,(ix+CH_VIB_DELAY)
    or      a
    jr      z,.slide_active
    dec     a
    ld      (ix+CH_VIB_DELAY),a
    jr      .no_slide

.slide_active:
    ; Add slide step to accumulator
    ld      l,(ix+CH_TONE_SLIDE_ACC)
    ld      h,(ix+CH_TONE_SLIDE_ACC+1)
    ld      c,(ix+CH_TONE_SLIDE_VAL)
    ld      b,(ix+CH_TONE_SLIDE_VAL+1)
    add     hl,bc
    ld      (ix+CH_TONE_SLIDE_ACC),l
    ld      (ix+CH_TONE_SLIDE_ACC+1),h

    ; Add accumulator to tone period
    add     hl,de
    ex      de,hl              ; DE = period with slide applied

.no_slide:
    ; --- Clamp period to valid AY range ---
    ; AY tone period is 12 bits (0..4095)
    ld      a,d
    and     0x0F
    ld      d,a

    ; Ensure period is at least 1
    ld      a,d
    or      e
    jr      nz,.period_ok
    ld      e,1
.period_ok:
    ld      (ix+CH_OUT_TONE_LO),e
    ld      (ix+CH_OUT_TONE_HI),d

    ; --- Envelope flag ---
    ld      a,(ix+CH_ENVELOPE)
    ld      (ix+CH_OUT_ENV_ON),a

    ret

; ============================================================================
; PROCESS ENVELOPE PERIOD SLIDE
; ============================================================================
; Global effect: slides the envelope period over time.

process_env_slide:
    ld      a,(env_delay)
    or      a
    ret     z                      ; No envelope slide active

    ld      a,(env_delay_cnt)
    dec     a
    ld      (env_delay_cnt),a
    ret     nz                     ; Not time yet

    ; Reload delay counter
    ld      a,(env_delay)
    ld      (env_delay_cnt),a

    ; Add envelope slide to base period
    ld      a,(env_base_lo)
    ld      l,a
    ld      a,(env_base_hi)
    ld      h,a
    ld      a,(env_slide_lo)
    ld      e,a
    ld      a,(env_slide_hi)
    ld      d,a
    add     hl,de
    ld      a,l
    ld      (env_base_lo),a
    ld      a,h
    ld      (env_base_hi),a

    ret

; ============================================================================
; WRITE AY REGISTERS
; ============================================================================
; Collects computed values from all 3 channels' CH_OUT_* fields and writes
; the 14 AY-3-8910 registers via OUT instructions.
;
; The Z80's OUT (C),r instruction requires the port address in BC, which
; clobbers any loop counter. We use an unrolled sequence for correctness.
;
; Register mapping:
;   R0:  Channel A tone period low byte
;   R1:  Channel A tone period high byte (4 bits)
;   R2:  Channel B tone period low byte
;   R3:  Channel B tone period high byte (4 bits)
;   R4:  Channel C tone period low byte
;   R5:  Channel C tone period high byte (4 bits)
;   R6:  Noise period (5 bits)
;   R7:  Mixer control (tone/noise enable per channel, active low)
;   R8:  Channel A volume (4 bits) + bit 4 = envelope mode
;   R9:  Channel B volume (4 bits) + bit 4 = envelope mode
;   R10: Channel C volume (4 bits) + bit 4 = envelope mode
;   R11: Envelope period low byte
;   R12: Envelope period high byte
;   R13: Envelope shape (4 bits), only written on new envelope command

write_ay_regs:
    ; --- Build the AY register buffer from channel output fields ---

    ; R0: Channel A tone low
    ld      a,(chan_a+CH_OUT_TONE_LO)
    ld      (ay_regs+0),a
    ; R1: Channel A tone high
    ld      a,(chan_a+CH_OUT_TONE_HI)
    ld      (ay_regs+1),a
    ; R2: Channel B tone low
    ld      a,(chan_b+CH_OUT_TONE_LO)
    ld      (ay_regs+2),a
    ; R3: Channel B tone high
    ld      a,(chan_b+CH_OUT_TONE_HI)
    ld      (ay_regs+3),a
    ; R4: Channel C tone low
    ld      a,(chan_c+CH_OUT_TONE_LO)
    ld      (ay_regs+4),a
    ; R5: Channel C tone high
    ld      a,(chan_c+CH_OUT_TONE_HI)
    ld      (ay_regs+5),a

    ; R6: Noise period (use the last channel with noise enabled, or noise_base)
    ld      a,(noise_base)
    ld      b,a
    ; Check each channel for noise contribution
    ld      a,(chan_a+CH_OUT_NOISE_ON)
    or      a
    jr      z,.noise_not_a
    ld      a,(chan_a+CH_OUT_NOISE)
    add     a,b
    ld      b,a
.noise_not_a:
    ld      a,(chan_b+CH_OUT_NOISE_ON)
    or      a
    jr      z,.noise_not_b
    ld      a,(chan_b+CH_OUT_NOISE)
    add     a,b
    ld      b,a
.noise_not_b:
    ld      a,(chan_c+CH_OUT_NOISE_ON)
    or      a
    jr      z,.noise_not_c
    ld      a,(chan_c+CH_OUT_NOISE)
    add     a,b
    ld      b,a
.noise_not_c:
    ld      a,b
    and     0x1F               ; 5-bit noise period
    ld      (ay_regs+6),a

    ; R7: Mixer register (active LOW: 0 = enabled, 1 = disabled)
    ; Bit 0: Tone A, Bit 1: Tone B, Bit 2: Tone C
    ; Bit 3: Noise A, Bit 4: Noise B, Bit 5: Noise C
    ld      b,0x3F             ; Start with everything disabled

    ; Channel A tone
    ld      a,(chan_a+CH_OUT_TONE_ON)
    or      a
    jr      z,.mix_a_tone_off
    res     0,b                ; Enable tone A (clear bit 0)
.mix_a_tone_off:
    ; Channel A noise
    ld      a,(chan_a+CH_OUT_NOISE_ON)
    or      a
    jr      z,.mix_a_noise_off
    res     3,b                ; Enable noise A (clear bit 3)
.mix_a_noise_off:

    ; Channel B tone
    ld      a,(chan_b+CH_OUT_TONE_ON)
    or      a
    jr      z,.mix_b_tone_off
    res     1,b                ; Enable tone B (clear bit 1)
.mix_b_tone_off:
    ; Channel B noise
    ld      a,(chan_b+CH_OUT_NOISE_ON)
    or      a
    jr      z,.mix_b_noise_off
    res     4,b                ; Enable noise B (clear bit 4)
.mix_b_noise_off:

    ; Channel C tone
    ld      a,(chan_c+CH_OUT_TONE_ON)
    or      a
    jr      z,.mix_c_tone_off
    res     2,b                ; Enable tone C (clear bit 2)
.mix_c_tone_off:
    ; Channel C noise
    ld      a,(chan_c+CH_OUT_NOISE_ON)
    or      a
    jr      z,.mix_c_noise_off
    res     5,b                ; Enable noise C (clear bit 5)
.mix_c_noise_off:
    ld      a,b
    ld      (ay_regs+7),a

    ; R8: Channel A volume (+ envelope flag in bit 4)
    ld      a,(chan_a+CH_OUT_VOL)
    ld      b,a
    ld      a,(chan_a+CH_OUT_ENV_ON)
    or      a
    jr      z,.vol_a_no_env
    ld      a,b
    or      0x10               ; Set envelope mode flag
    jr      .vol_a_done
.vol_a_no_env:
    ld      a,b
.vol_a_done:
    ld      (ay_regs+8),a

    ; R9: Channel B volume
    ld      a,(chan_b+CH_OUT_VOL)
    ld      b,a
    ld      a,(chan_b+CH_OUT_ENV_ON)
    or      a
    jr      z,.vol_b_no_env
    ld      a,b
    or      0x10
    jr      .vol_b_done
.vol_b_no_env:
    ld      a,b
.vol_b_done:
    ld      (ay_regs+9),a

    ; R10: Channel C volume
    ld      a,(chan_c+CH_OUT_VOL)
    ld      b,a
    ld      a,(chan_c+CH_OUT_ENV_ON)
    or      a
    jr      z,.vol_c_no_env
    ld      a,b
    or      0x10
    jr      .vol_c_done
.vol_c_no_env:
    ld      a,b
.vol_c_done:
    ld      (ay_regs+10),a

    ; R11: Envelope period low
    ld      a,(env_base_lo)
    ld      (ay_regs+11),a
    ; R12: Envelope period high
    ld      a,(env_base_hi)
    ld      (ay_regs+12),a

    ; --- Output registers R0..R12 to AY chip ---
    ; Unrolled loop: each register requires selecting via AY_REG_PORT then
    ; writing data via AY_DATA_PORT. BC is used for the 16-bit port address.

    ld      a,0
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+0)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,1
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+1)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,2
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+2)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,3
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+3)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,4
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+4)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,5
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+5)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,6
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+6)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,7
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+7)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,8
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+8)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,9
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+9)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,10
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+10)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,11
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+11)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,12
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(ay_regs+12)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ; R13: Envelope shape - only write when a new envelope command was issued.
    ; Writing to R13 restarts the envelope generator, so we must only do it
    ; when the module explicitly requests a new envelope shape.
    ld      a,(env_new_shape)
    or      a
    ret     z                  ; No new envelope shape: skip R13

    ld      (env_shape),a     ; Copy to current shape
    ld      a,13
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,(env_shape)
    ld      bc,AY_DATA_PORT
    out     (c),a

    ; Clear the new-shape flag so we don't re-trigger next frame
    xor     a
    ld      (env_new_shape),a

    ret

; ============================================================================
; LOAD POSITION
; ============================================================================
; Sets up pattern pointers for all three channels from the current position
; in the position list.
;
; Each position list entry is a pattern number. The patterns table contains
; 6 bytes per pattern: three 2-byte offsets (one per channel), each being
; an offset from the module start to the pattern channel data.

load_position:
    ; Get current position number
    ld      a,(current_pos)

    ; Look up pattern number from position list
    ld      hl,(pos_list_addr)
    ld      e,a
    ld      d,0
    add     hl,de
    ld      a,(hl)                 ; A = pattern number

    ; Compute patterns table entry address: pat_table + pattern_num * 6
    ld      hl,(pat_table_addr)
    ld      e,a
    ld      d,0
    push    hl
    ld      h,d
    ld      l,e
    add     hl,hl                  ; *2
    add     hl,de                  ; *3
    add     hl,hl                  ; *6
    ex      de,hl                  ; DE = pattern_num * 6
    pop     hl
    add     hl,de                  ; HL = patterns table entry

    ; Read channel A pattern data offset
    ld      e,(hl)
    inc     hl
    ld      d,(hl)
    inc     hl
    push    hl                     ; Save table pointer

    ; Convert to absolute address: module_addr + offset
    ld      hl,(module_addr)
    add     hl,de
    ld      (chan_a+CH_PAT_PTR),hl

    pop     hl

    ; Read channel B pattern data offset
    ld      e,(hl)
    inc     hl
    ld      d,(hl)
    inc     hl
    push    hl

    ld      hl,(module_addr)
    add     hl,de
    ld      (chan_b+CH_PAT_PTR),hl

    pop     hl

    ; Read channel C pattern data offset
    ld      e,(hl)
    inc     hl
    ld      d,(hl)

    ld      hl,(module_addr)
    add     hl,de
    ld      (chan_c+CH_PAT_PTR),hl

    ret

; ============================================================================
; NEXT POSITION
; ============================================================================
; Advances to the next position in the song. If at the end, loops back.

next_position:
    ld      a,(current_pos)
    inc     a
    ld      b,a
    ld      a,(num_positions)
    cp      b
    jr      nz,.no_loop
    ; Reached end of song, loop
    ld      a,(loop_position)
    ld      b,a
.no_loop:
    ld      a,b
    ld      (current_pos),a

    ; Load the new position's pattern data
    call    load_position

    ; Reset tick counter to force immediate row read
    ld      a,1
    ld      (tick_count),a

    ret

; ============================================================================
; GET NOTE PERIOD
; ============================================================================
; Input:  A = note number (0..95)
; Output: DE = tone period (16-bit)
; Clobbers: HL

get_note_period:
    cp      96
    jr      c,.note_valid
    ld      a,95
.note_valid:
    ld      l,a
    ld      h,0
    add     hl,hl                  ; *2 for word-sized entries
    ld      de,note_table
    add     hl,de
    ld      e,(hl)
    inc     hl
    ld      d,(hl)                 ; DE = period
    ret

; ============================================================================
; MUTE AY
; ============================================================================
; Silences all channels by setting volumes to 0 and mixer to all-off.

mute_ay:
    ; Mixer: all tone and noise disabled (all bits set = all off)
    ld      a,7
    ld      bc,AY_REG_PORT
    out     (c),a
    ld      a,0x3F
    ld      bc,AY_DATA_PORT
    out     (c),a

    ; Zero volume on all three channels
    ld      a,8
    ld      bc,AY_REG_PORT
    out     (c),a
    xor     a
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,9
    ld      bc,AY_REG_PORT
    out     (c),a
    xor     a
    ld      bc,AY_DATA_PORT
    out     (c),a

    ld      a,10
    ld      bc,AY_REG_PORT
    out     (c),a
    xor     a
    ld      bc,AY_DATA_PORT
    out     (c),a

    ret

; ============================================================================
; NOTE PERIOD TABLE
; ============================================================================
; Standard PT3/Vortex Tracker II note table for AY-3-8910 at 1.7734 MHz.
; 96 entries covering notes C-1 through B-8 (8 octaves x 12 semitones).
;
; These are the standard ProTracker 3 periods. The AY generates a square wave
; with frequency = clock / (16 * period). At 1.7734 MHz:
;   Period 3424 -> ~32.4 Hz (C-1)
;   Period 253  -> ~438 Hz  (approximately A-4)
;   Period 14   -> ~7929 Hz (B-8)

note_table:
    ; Octave 1 (C-1 to B-1): lowest octave
    .word   3424, 3232, 3048, 2880, 2712, 2560, 2416, 2280, 2152, 2032, 1920, 1812
    ; Octave 2 (C-2 to B-2)
    .word   1712, 1616, 1524, 1440, 1356, 1280, 1208, 1140, 1076, 1016, 960,  906
    ; Octave 3 (C-3 to B-3)
    .word   856,  808,  762,  720,  678,  640,  604,  570,  538,  508,  480,  453
    ; Octave 4 (C-4 to B-4): middle octave
    .word   428,  404,  381,  360,  339,  320,  302,  285,  269,  254,  240,  226
    ; Octave 5 (C-5 to B-5)
    .word   214,  202,  190,  180,  170,  160,  151,  143,  135,  127,  120,  113
    ; Octave 6 (C-6 to B-6)
    .word   107,  101,  95,   90,   85,   80,   76,   71,   67,   64,   60,   57
    ; Octave 7 (C-7 to B-7)
    .word   54,   51,   48,   45,   42,   40,   38,   36,   34,   32,   30,   28
    ; Octave 8 (C-8 to B-8): highest octave
    .word   27,   25,   24,   22,   21,   20,   19,   18,   17,   16,   15,   14

; ============================================================================
; VARIABLES AND WORKING STORAGE
; ============================================================================

; --- Module pointers (set during INIT) ---
module_addr:        .word 0        ; Base address of PT3 module
pat_table_addr:     .word 0        ; Absolute address of patterns table
sam_table_addr:     .word 0        ; Absolute address of samples table
orn_table_addr:     .word 0        ; Absolute address of ornaments table
pos_list_addr:      .word 0        ; Absolute address of position list

; --- Playback state ---
speed:              .byte 0        ; Ticks per row (from module header)
tick_count:         .byte 0        ; Current tick counter (counts down to 0)
current_pos:        .byte 0        ; Current position in song (0..num_positions-1)
num_positions:      .byte 0        ; Total number of positions
loop_position:      .byte 0        ; Position to loop back to at song end

; --- Envelope state ---
env_base_lo:        .byte 0        ; Envelope period low byte
env_base_hi:        .byte 0        ; Envelope period high byte
env_shape:          .byte 0        ; Current envelope shape for AY R13
env_new_shape:      .byte 0        ; New shape flag (non-zero = write R13)
env_slide_lo:       .byte 0        ; Envelope slide step low
env_slide_hi:       .byte 0        ; Envelope slide step high
env_delay:          .byte 0        ; Envelope slide delay (reload value)
env_delay_cnt:      .byte 0        ; Envelope slide delay counter

; --- Noise state ---
noise_base:         .byte 0        ; Base noise period

; --- AY register output buffer (R0..R12, R13 handled separately) ---
ay_regs:            .byte 0,0,0,0,0,0,0,0,0,0,0,0,0

; --- Channel data blocks (3 channels x CH_SIZE bytes each) ---
chan_a:              .space CH_SIZE
chan_b:              .space CH_SIZE
chan_c:              .space CH_SIZE

; ============================================================================
; END OF PLAYER
; ============================================================================
