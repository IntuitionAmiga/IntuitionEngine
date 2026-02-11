; ehbasic_hw_audio.inc - EhBASIC IE64 SoundChip Commands
;
; Statement handlers for audio hardware commands:
;   SOUND ch, freq, vol [,wave [,duty]]
;   SOUND FILTER cutoff, resonance, type
;   SOUND REVERB mix, decay
;   SOUND OVERDRIVE amount
;   SOUND NOISE ch, mode
;   SOUND WAVE ch, type
;   SOUND SWEEP ch, enable, period, shift
;   SOUND SYNC ch, source
;   SOUND RINGMOD ch, source
;   SOUND STOP
;   ENVELOPE ch, atk, dec, sus, rel
;   GATE ch, ON / GATE ch, OFF
;
; Uses the flexible 4-channel interface (FLEX_CH0_BASE, stride 0x40).
; Channel 0-3 map to flex channels.
;
; Register conventions:
;   R16 = interpreter state base
;   R17 = text pointer
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; hw_calc_ch_base - Calculate channel base address
; ============================================================================
; Input:  R22 = channel number (0-3)
; Output: R22 = base address for that channel's registers
; Clobbers: R1

hw_calc_ch_base:
    ; base = FLEX_CH0_BASE + ch * FLEX_CH_STRIDE
    mulu.l  r1, r22, #FLEX_CH_STRIDE
    move.l  r22, #FLEX_CH0_BASE
    add.q   r22, r22, r1
    rts

; ============================================================================
; hw_sound - SOUND ch, freq, vol [,wave [,duty]]
; ============================================================================
; Sets frequency, volume, and optionally waveform type and duty cycle.
; freq is in Hz (converted to 16.8 fixed-point: Hz * 256).

hw_sound:
    jsr     exec_skip_spaces
    ; Check for sub-commands by first letter
    load.b  r1, (r17)
    move.q  r2, #0x46            ; 'F' (FILTER)
    beq     r1, r2, .snd_filter
    move.q  r2, #0x4E            ; 'N' (NOISE)
    beq     r1, r2, .snd_noise
    move.q  r2, #0x4F            ; 'O' (OVERDRIVE)
    beq     r1, r2, .snd_overdrive
    move.q  r2, #0x50            ; 'P' (PLAY)
    beq     r1, r2, .snd_play_cmd
    move.q  r2, #TK_STOP         ; STOP token (SOUND STOP)
    beq     r1, r2, .snd_stop
    move.q  r2, #0x52            ; 'R' (REVERB / RINGMOD)
    beq     r1, r2, .snd_r_cmd
    move.q  r2, #0x53            ; 'S' (SWEEP / SYNC)
    beq     r1, r2, .snd_s_cmd
    move.q  r2, #0x57            ; 'W' (WAVE)
    beq     r1, r2, .snd_wave
    ; Parse channel number
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = channel (0-3)
    push    r22
    ; Parse comma + frequency
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = frequency (Hz)
    ; Parse comma + volume
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_freq_only
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8              ; R24 = volume (0-255)
    ; Calculate channel base
    pop     r22
    jsr     hw_calc_ch_base      ; R22 = base address
    ; Write frequency (Hz * 256 for 16.8 fixed-point)
    lsl.l   r1, r23, #8
    add.q   r3, r22, #FLEX_OFF_FREQ
    store.l r1, (r3)
    ; Write volume
    add.q   r3, r22, #FLEX_OFF_VOL
    store.l r24, (r3)
    ; Check for optional waveform type
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Write wave type
    add.q   r3, r22, #FLEX_OFF_WAVE_TYPE
    store.l r8, (r3)
    ; Check for optional duty cycle
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Write duty cycle
    add.q   r3, r22, #FLEX_OFF_DUTY
    store.l r8, (r3)
    rts

; --- SOUND PLAY "file"[,subsong] / SOUND PLAY STOP ---
.snd_play_cmd:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces

    ; Optional STOP sub-command
    load.b  r1, (r17)
    move.q  r2, #0x53            ; 'S'
    beq     r1, r2, .snd_play_stop
    move.q  r2, #TK_STOP
    beq     r1, r2, .snd_play_stop

    ; Parse filename string expression
    jsr     str_eval             ; R8 = filename pointer
    move.q  r10, r8
    la      r11, FILE_NAME_BUF
.snd_play_copy_name:
    load.b  r1, (r10)
    store.b r1, (r11)
    add.q   r10, r10, #1
    add.q   r11, r11, #1
    bnez    r1, .snd_play_copy_name

    ; Optional ",subsong"
    move.q  r20, r0              ; default subsong=0
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_play_trigger
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r20, r8

.snd_play_trigger:
    la      r1, MEDIA_NAME_PTR
    la      r2, FILE_NAME_BUF
    store.l r2, (r1)
    la      r1, MEDIA_SUBSONG
    store.l r20, (r1)

    ; Stop any currently playing track before starting the new one.
    la      r1, MEDIA_CTRL
    move.q  r2, #2
    store.l r2, (r1)

    la      r1, MEDIA_CTRL
    move.q  r2, #1
    store.l r2, (r1)

    ; Bounded status poll so async load failures are visible in BASIC.
    ; status: 0=idle, 1=loading, 2=playing, 3=error
    move.q  r25, #0x40000
.snd_play_wait_status:
    la      r1, MEDIA_STATUS
    load.l  r2, (r1)
    move.q  r3, #1
    bne     r2, r3, .snd_play_status_done
    sub.q   r25, r25, #1
    bnez    r25, .snd_play_wait_status
    bra     .snd_done

.snd_play_status_done:
    move.q  r3, #3
    bne     r2, r3, .snd_done
    la      r8, .msg_play_error
    jsr     print_string
    jsr     print_crlf
    rts

.snd_play_stop:
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, MEDIA_CTRL
    move.q  r2, #2
    store.l r2, (r1)
    rts

; --- SOUND STOP (alias of SOUND PLAY STOP) ---
.snd_stop:
    la      r1, MEDIA_CTRL
    move.q  r2, #2
    store.l r2, (r1)
    rts
.msg_play_error:
    dc.b    "?PLAY ERROR", 0
    align   4

.snd_filter:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     hw_filter
    rts

; --- SOUND REVERB mix, decay ---
.snd_reverb:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, REVERB_MIX
    store.l r8, (r3)
    ; Parse comma + decay
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, REVERB_DECAY
    store.l r8, (r3)
    rts

; --- SOUND RINGMOD ch, source ---
.snd_ringmod:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse channel
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = channel (0-3)
    ; Parse comma + source
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; RING_MOD_SOURCE_CH0 + ch * 4
    mulu.l  r1, r22, #4
    la      r3, RING_MOD_SOURCE_CH0
    add.q   r3, r3, r1
    store.l r8, (r3)
    rts

; --- SOUND R dispatch: REVERB vs RINGMOD ---
.snd_r_cmd:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x49            ; 'I' → RINGMOD
    beq     r4, r2, .snd_ringmod
    ; Default to REVERB (RE...)
    bra     .snd_reverb

; --- SOUND OVERDRIVE amount ---
.snd_overdrive:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, OVERDRIVE_CTRL
    store.l r8, (r3)
    rts

; --- SOUND NOISE ch, mode ---
.snd_noise:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse channel
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = channel
    ; Parse comma + mode
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, NOISE_MODE
    store.l r8, (r3)
    rts

; --- SOUND WAVE ch, type ---
.snd_wave:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse channel
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = channel (0-3)
    jsr     hw_calc_ch_base      ; R22 = base address
    ; Parse comma + wave type
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    add.q   r3, r22, #FLEX_OFF_WAVE_TYPE
    store.l r8, (r3)
    rts

; --- SOUND SWEEP ch, enable, period, shift ---
.snd_sweep:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse channel
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = channel (0-3)
    jsr     hw_calc_ch_base      ; R22 = base address
    push    r22
    ; Parse comma + enable
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_sweep_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; R23 = enable
    ; Parse comma + period
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_sweep_write
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8              ; R24 = period
    ; Parse comma + shift
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_sweep_write
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Pack sweep: enable | (period << 8) | (shift << 16)
    lsl.l   r24, r24, #8
    or.l    r23, r23, r24
    lsl.l   r1, r8, #16
    or.l    r23, r23, r1
.snd_sweep_write:
    pop     r22
    add.q   r3, r22, #FLEX_OFF_SWEEP
    store.l r23, (r3)
    rts
.snd_sweep_done:
    pop     r22
    rts

; --- SOUND SYNC ch, source ---
.snd_sync:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse channel
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; R22 = channel (0-3)
    ; Parse comma + source
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .snd_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; SYNC_SOURCE_CH0 + ch * 4
    mulu.l  r1, r22, #4
    la      r3, SYNC_SOURCE_CH0
    add.q   r3, r3, r1
    store.l r8, (r3)
    rts

; --- SOUND S dispatch: SWEEP vs SYNC ---
.snd_s_cmd:
    ; Accept raw "SOUND STOP" when STOP was not tokenized.
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x54            ; 'T' → STOP
    beq     r4, r2, .snd_play_stop
    move.q  r2, #0x57            ; 'W' → SWEEP
    beq     r4, r2, .snd_sweep
    move.q  r2, #0x59            ; 'Y' → SYNC
    beq     r4, r2, .snd_sync
    ; Unrecognised S sub-command — fall through to done
    rts

.snd_freq_only:
    pop     r22
.snd_done:
    rts

; ============================================================================
; hw_envelope - ENVELOPE ch, atk, dec, sus, rel
; ============================================================================

hw_envelope:
    jsr     exec_skip_spaces
    ; Parse channel
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     hw_calc_ch_base      ; R22 = base address
    ; Parse atk
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .env_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    add.q   r3, r22, #FLEX_OFF_ATK
    store.l r8, (r3)
    ; Parse dec
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .env_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    add.q   r3, r22, #FLEX_OFF_DEC
    store.l r8, (r3)
    ; Parse sus
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .env_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    add.q   r3, r22, #FLEX_OFF_SUS
    store.l r8, (r3)
    ; Parse rel
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .env_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    add.q   r3, r22, #FLEX_OFF_REL
    store.l r8, (r3)
.env_done:
    rts

; ============================================================================
; hw_gate - GATE ch, ON / GATE ch, OFF
; ============================================================================

hw_gate:
    jsr     exec_skip_spaces
    ; Parse channel
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     hw_calc_ch_base      ; R22 = base address
    ; Skip comma
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .gate_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Check for ON token or OFF text
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .gate_on
    ; Check for 'O' (OFF)
    move.q  r2, #0x4F
    bne     r1, r2, .gate_done
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46            ; 'F'
    bne     r4, r2, .gate_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .gate_done
    ; OFF confirmed
    add.q   r17, r3, #1
    add.q   r3, r22, #FLEX_OFF_CTRL
    load.l  r1, (r3)
    and.l   r1, r1, #0xFFFFFFFD  ; clear bit 1 (gate)
    store.l r1, (r3)
    rts
.gate_on:
    add.q   r17, r17, #1         ; consume ON token
    add.q   r3, r22, #FLEX_OFF_CTRL
    load.l  r1, (r3)
    or.l    r1, r1, #2           ; set bit 1 (gate)
    store.l r1, (r3)
.gate_done:
    rts

; ============================================================================
; hw_filter - FILTER cutoff, resonance, type
; ============================================================================

hw_filter:
    jsr     exec_skip_spaces
    ; Check for FILTER MOD sub-command
    load.b  r1, (r17)
    move.q  r2, #0x4D            ; 'M' (MOD)
    beq     r1, r2, .filt_mod
    ; Parse cutoff
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, FILTER_CUTOFF
    store.l r8, (r3)
    ; Parse resonance
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .filt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, FILTER_RESONANCE
    store.l r8, (r3)
    ; Parse type
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .filt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, FILTER_TYPE
    store.l r8, (r3)
.filt_done:
    rts
.filt_mod:
    ; FILTER MOD source, amount
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse source
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, FILTER_MOD_SOURCE
    store.l r8, (r3)
    ; Parse amount
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .filt_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r3, FILTER_MOD_AMOUNT
    store.l r8, (r3)
    rts

; ============================================================================
; hw_psg - PSG ch,freq,vol / PSG PLUS ON/OFF / PSG PLAY/STOP
; ============================================================================

hw_psg:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    ; Check for 'P' (PLUS/PLAY)
    move.q  r2, #0x50
    beq     r1, r2, .psg_p_cmd
    ; Check for 'S' (STOP/STATUS) or TK_STOP token
    move.q  r2, #0x53
    beq     r1, r2, .psg_stop
    move.q  r2, #0x90
    beq     r1, r2, .psg_stop
    ; Check for 'M' (MIXER)
    move.q  r2, #0x4D
    beq     r1, r2, .psg_mixer
    ; Check for 'E' (ENVELOPE) or TK_ENVELOPE token
    move.q  r2, #0x45
    beq     r1, r2, .psg_env
    move.q  r2, #0xF0
    beq     r1, r2, .psg_env
    ; Otherwise it's PSG ch,freq,vol — numeric expression
    ; Parse channel
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; ch (0-3)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .psg_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse freq
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; PSG freq register: pairs at POKEY_AUDF1 + ch*2
    ; (PSG uses the same register layout)
    mulu.l  r1, r22, #2
    la      r2, PSG_BASE
    add.q   r1, r1, r2
    store.b r8, (r1)             ; freq divider
    ; Parse vol
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .psg_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Volume in control register: PSG_BASE + ch*2 + 1
    mulu.l  r1, r22, #2
    add.q   r1, r1, #1
    la      r2, PSG_BASE
    add.q   r1, r1, r2
    store.b r8, (r1)
    rts
.psg_p_cmd:
    ; PL=PLUS, PLA=PLAY
    add.q   r3, r17, #2
    load.b  r4, (r3)
    move.q  r2, #0x55            ; 'U' → PLUS
    beq     r4, r2, .psg_plus
    ; PLAY — skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse addr
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, PSG_PLAY_PTR
    store.l r8, (r1)
    ; Parse len
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .psg_play_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, PSG_PLAY_LEN
    store.l r8, (r1)
.psg_play_start:
    la      r1, PSG_PLAY_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.psg_plus:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .psg_plus_on
    ; OFF
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, PSG_PLUS_CTRL
    store.b r0, (r1)
    rts
.psg_plus_on:
    add.q   r17, r17, #1
    la      r1, PSG_PLUS_CTRL
    move.q  r2, #1
    store.b r2, (r1)
    rts
.psg_stop:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, PSG_PLAY_CTRL
    store.l r0, (r1)
    rts
.psg_mixer:
    ; Skip sub-command — write to PSG mixer register
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; PSG mixer at PSG_BASE + 0x07 (AY-3-8910 mixer register)
    la      r1, PSG_BASE
    add.q   r1, r1, #7
    store.b r8, (r1)
    rts
.psg_env:
    ; Skip sub-command — shape, period
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Envelope shape at PSG_BASE + 0x0D
    la      r1, PSG_BASE
    add.q   r1, r1, #0x0D
    store.b r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .psg_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Envelope period low at PSG_BASE + 0x0B
    la      r1, PSG_BASE
    add.q   r1, r1, #0x0B
    and.l   r2, r8, #0xFF
    store.b r2, (r1)
    ; Envelope period high at PSG_BASE + 0x0C
    add.q   r1, r1, #1
    lsr.l   r2, r8, #8
    store.b r2, (r1)
.psg_done:
    rts

; ============================================================================
; hw_sid - SID VOICE/FILTER/VOLUME/PLUS/PLAY/STOP
; ============================================================================

hw_sid:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x56            ; 'V' (VOICE/VOLUME)
    beq     r1, r2, .sid_v_cmd
    move.q  r2, #0x46            ; 'F' (FILTER)
    beq     r1, r2, .sid_filter
    move.q  r2, #0x50            ; 'P' (PLUS/PLAY)
    beq     r1, r2, .sid_p_cmd
    move.q  r2, #0x53            ; 'S' (STOP)
    beq     r1, r2, .sid_stop
    move.q  r2, #0x90            ; TK_STOP token
    beq     r1, r2, .sid_stop
    rts
.sid_v_cmd:
    ; VO=VOICE or VOLUME
    add.q   r3, r17, #2
    load.b  r4, (r3)
    move.q  r2, #0x49            ; 'I' → VOICE
    beq     r4, r2, .sid_voice
    ; VOLUME — skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, SID_MODE_VOL
    ; Read current, preserve filter mode bits (4-7), set volume (0-3)
    load.b  r2, (r1)
    and.l   r2, r2, #0xF0
    and.l   r3, r8, #0x0F
    or.l    r2, r2, r3
    store.b r2, (r1)
    rts
.sid_voice:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse voice (1-3) → convert to 0-based
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    sub.l   r22, r8, #1          ; 0-based voice
    ; Calculate voice base: SID_BASE + voice*7
    mulu.l  r1, r22, #7
    la      r2, SID_BASE
    add.q   r22, r1, r2          ; R22 = voice base
    ; Parse freq
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; freq low/high
    store.b r8, (r22)
    lsr.l   r1, r8, #8
    add.q   r3, r22, #1
    store.b r1, (r3)
    ; Parse pw
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    add.q   r3, r22, #2
    store.b r8, (r3)
    lsr.l   r1, r8, #8
    add.q   r3, r22, #3
    store.b r1, (r3)
    ; Parse ctrl
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    add.q   r3, r22, #4
    store.b r8, (r3)
    ; Parse AD
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    add.q   r3, r22, #5
    store.b r8, (r3)
    ; Parse SR
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    add.q   r3, r22, #6
    store.b r8, (r3)
    rts
.sid_filter:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse cutoff
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; FC_LO = cutoff & 7, FC_HI = cutoff >> 3
    and.l   r1, r8, #7
    la      r2, SID_FC_LO
    store.b r1, (r2)
    lsr.l   r1, r8, #3
    la      r2, SID_FC_HI
    store.b r1, (r2)
    ; Parse resonance
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    ; Parse routing
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; RES_FILT = (resonance << 4) | routing
    lsl.l   r1, r22, #4
    and.l   r2, r8, #0x0F
    or.l    r1, r1, r2
    la      r2, SID_RES_FILT
    store.b r1, (r2)
    ; Parse mode
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; MODE_VOL: preserve vol (bits 0-3), set mode (bits 4-7)
    la      r2, SID_MODE_VOL
    load.b  r1, (r2)
    and.l   r1, r1, #0x0F
    and.l   r3, r8, #0x0F
    lsl.l   r3, r3, #4
    or.l    r1, r1, r3
    store.b r1, (r2)
    rts
.sid_p_cmd:
    add.q   r3, r17, #2
    load.b  r4, (r3)
    move.q  r2, #0x55            ; 'U' → PLUS
    beq     r4, r2, .sid_plus
    ; PLAY — skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, SID_PLAY_PTR
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_play_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, SID_PLAY_LEN
    store.l r8, (r1)
    ; Optional subsong
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sid_play_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, SID_SUBSONG
    store.b r8, (r1)
.sid_play_start:
    la      r1, SID_PLAY_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.sid_plus:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .sid_plus_on
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, SID_PLUS_CTRL
    store.b r0, (r1)
    rts
.sid_plus_on:
    add.q   r17, r17, #1
    la      r1, SID_PLUS_CTRL
    move.q  r2, #1
    store.b r2, (r1)
    rts
.sid_stop:
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, SID_PLAY_CTRL
    move.q  r2, #2
    store.l r2, (r1)
.sid_done:
    rts

; ============================================================================
; hw_pokey - POKEY ch,freq,ctrl / POKEY CTRL / POKEY PLUS
; ============================================================================

hw_pokey:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x43            ; 'C' (CTRL)
    beq     r1, r2, .pokey_ctrl
    move.q  r2, #0x50            ; 'P' (PLUS)
    beq     r1, r2, .pokey_plus
    ; Numeric — POKEY ch, freq, ctrl
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; ch (1-4) → 0-based
    sub.l   r22, r22, #1
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .pokey_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse freq
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; AUDF at POKEY_AUDF1 + ch*2
    mulu.l  r1, r22, #2
    la      r2, POKEY_AUDF1
    add.q   r1, r1, r2
    store.b r8, (r1)
    ; Parse ctrl
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .pokey_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; AUDC at POKEY_AUDC1 + ch*2
    mulu.l  r1, r22, #2
    la      r2, POKEY_AUDC1
    add.q   r1, r1, r2
    store.b r8, (r1)
    rts
.pokey_ctrl:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, POKEY_AUDCTL
    store.b r8, (r1)
    rts
.pokey_plus:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .pokey_plus_on
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, POKEY_PLUS_CTRL
    store.b r0, (r1)
    rts
.pokey_plus_on:
    add.q   r17, r17, #1
    la      r1, POKEY_PLUS_CTRL
    move.q  r2, #1
    store.b r2, (r1)
.pokey_done:
    rts

; ============================================================================
; hw_ted_audio - TED TONE/VOL/NOISE/PLUS/PLAY/STOP
; ============================================================================
; Note: TED video commands are in ehbasic_hw_video.inc (hw_ted_cmd).
; This handles audio sub-commands routed by the TED token.
; We distinguish: TONE, VOL, NOISE, PLUS, PLAY, STOP

hw_ted_audio:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x54            ; 'T' (TONE)
    beq     r1, r2, .teda_tone
    move.q  r2, #0xA9            ; TK_TO token (TONE tokenized as TO+NE)
    beq     r1, r2, .teda_tone
    move.q  r2, #0x56            ; 'V' (VOL)
    beq     r1, r2, .teda_vol
    move.q  r2, #0x4E            ; 'N' (NOISE)
    beq     r1, r2, .teda_noise
    move.q  r2, #0x50            ; 'P' (PLUS/PLAY)
    beq     r1, r2, .teda_p_cmd
    move.q  r2, #0x53            ; 'S' (STOP)
    beq     r1, r2, .teda_stop
    move.q  r2, #0x90            ; TK_STOP token
    beq     r1, r2, .teda_stop
    rts
.teda_tone:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse channel (1-2)
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .teda_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse freq (10-bit value)
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Ch 1: FREQ1_LO + FREQ1_HI, Ch 2: FREQ2_LO + FREQ2_HI
    move.q  r2, #1
    beq     r22, r2, .teda_ch1
    ; Channel 2
    la      r1, TED_FREQ2_LO
    and.l   r2, r8, #0xFF
    store.b r2, (r1)
    la      r1, TED_FREQ2_HI
    lsr.l   r2, r8, #8
    and.l   r2, r2, #3
    store.b r2, (r1)
    rts
.teda_ch1:
    la      r1, TED_FREQ1_LO
    and.l   r2, r8, #0xFF
    store.b r2, (r1)
    la      r1, TED_FREQ1_HI
    lsr.l   r2, r8, #8
    and.l   r2, r2, #3
    store.b r2, (r1)
    rts
.teda_vol:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Volume in SND_CTRL bits 0-3
    la      r1, TED_SND_CTRL
    load.b  r2, (r1)
    and.l   r2, r2, #0xF0       ; preserve upper bits
    and.l   r3, r8, #0x0F
    or.l    r2, r2, r3
    store.b r2, (r1)
    rts
.teda_noise:
    ; Skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .teda_noise_on
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, TED_SND_CTRL
    load.b  r2, (r1)
    and.l   r2, r2, #0x7F       ; clear bit 7 (noise)
    store.b r2, (r1)
    rts
.teda_noise_on:
    add.q   r17, r17, #1
    la      r1, TED_SND_CTRL
    load.b  r2, (r1)
    or.l    r2, r2, #0x80       ; set bit 7 (noise)
    store.b r2, (r1)
    rts
.teda_p_cmd:
    add.q   r3, r17, #2
    load.b  r4, (r3)
    move.q  r2, #0x55            ; 'U' → PLUS
    beq     r4, r2, .teda_plus
    ; PLAY — skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, TED_PLAY_PTR
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .teda_play_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, TED_PLAY_LEN
    store.l r8, (r1)
.teda_play_start:
    la      r1, TED_PLAY_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.teda_plus:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .teda_plus_on
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, TED_PLUS_CTRL
    store.b r0, (r1)
    rts
.teda_plus_on:
    add.q   r17, r17, #1
    la      r1, TED_PLUS_CTRL
    move.q  r2, #1
    store.b r2, (r1)
    rts
.teda_stop:
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, TED_PLAY_CTRL
    store.l r0, (r1)
.teda_done:
    rts

; ============================================================================
; hw_ahx - AHX PLAY/STOP/PLUS
; ============================================================================

hw_ahx:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x50            ; 'P' (PLAY/PLUS)
    beq     r1, r2, .ahx_p_cmd
    move.q  r2, #0x53            ; 'S' (STOP)
    beq     r1, r2, .ahx_stop
    move.q  r2, #0x90            ; TK_STOP token
    beq     r1, r2, .ahx_stop
    rts
.ahx_p_cmd:
    add.q   r3, r17, #2
    load.b  r4, (r3)
    move.q  r2, #0x55            ; 'U' → PLUS
    beq     r4, r2, .ahx_plus
    ; PLAY — skip sub-command
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, AHX_PLAY_PTR
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .ahx_play_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, AHX_PLAY_LEN
    store.l r8, (r1)
.ahx_play_start:
    la      r1, AHX_PLAY_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.ahx_plus:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .ahx_plus_on
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, AHX_PLUS_CTRL
    store.l r0, (r1)
    rts
.ahx_plus_on:
    add.q   r17, r17, #1
    la      r1, AHX_PLUS_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.ahx_stop:
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, AHX_PLAY_CTRL
    move.q  r2, #2
    store.l r2, (r1)
    rts

; ============================================================================
; hw_sap - SAP PLAY/STOP
; ============================================================================

hw_sap:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x50            ; 'P' (PLAY)
    beq     r1, r2, .sap_play
    move.q  r2, #0x53            ; 'S' (STOP)
    beq     r1, r2, .sap_stop
    move.q  r2, #0x90            ; TK_STOP token
    beq     r1, r2, .sap_stop
    rts
.sap_play:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, SAP_PLAY_PTR
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sap_play_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, SAP_PLAY_LEN
    store.l r8, (r1)
    ; Optional subsong
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .sap_play_start
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, SAP_SUBSONG
    store.b r8, (r1)
.sap_play_start:
    la      r1, SAP_PLAY_CTRL
    move.q  r2, #1
    store.l r2, (r1)
    rts
.sap_stop:
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, SAP_PLAY_CTRL
    move.q  r2, #2
    store.l r2, (r1)
    rts

; ============================================================================
; EOF
; ============================================================================
