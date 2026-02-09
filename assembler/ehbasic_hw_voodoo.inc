; ehbasic_hw_voodoo.inc - EhBASIC IE64 Voodoo 3D Commands
;
; Statement handlers for Voodoo 3DFX hardware:
;   VOODOO ON/OFF/CLEAR/SWAP/DIM/CLIP/COMBINE/LFB/PIXEL
;   VOODOO TRICOLOR r,g,b[,a]
;   VOODOO ALPHA TEST ON/OFF, ALPHA FUNC n, ALPHA BLEND ON/OFF,
;          ALPHA SRC n, ALPHA DST n
;   VOODOO FOG ON/OFF, FOG COLOR r,g,b
;   VOODOO DITHER ON/OFF
;   VOODOO CHROMAKEY ON/OFF, CHROMAKEY COLOR r,g,b
;   VERTEX A/B/C x,y
;   TRIANGLE
;   ZBUFFER ON/OFF/FUNC/WRITE
;   TEXTURE MODE/BASE/DIM/UPLOAD/ON/OFF
;
; (c) 2024-2026 Zayn Otley - GPLv3 or later

; ============================================================================
; hw_voodoo - VOODOO sub-command dispatch
;   ON/OFF/CLEAR/SWAP/DIM/CLIP/COMBINE/LFB/PIXEL
;   TRICOLOR/ALPHA/FOG/DITHER/CHROMAKEY
; ============================================================================

hw_voodoo:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .voo_on
    move.q  r2, #0x4F            ; 'O' (OFF)
    beq     r1, r2, .voo_off_check
    move.q  r2, #TK_CLEAR
    beq     r1, r2, .voo_clear
    move.q  r2, #0x43            ; 'C' (CLEAR/CLIP/COMBINE/CHROMAKEY)
    beq     r1, r2, .voo_c_cmd
    move.q  r2, #TK_SWAP
    beq     r1, r2, .voo_swap
    move.q  r2, #0x53            ; 'S' (SWAP/STATUS)
    beq     r1, r2, .voo_s_cmd
    move.q  r2, #TK_DIM
    beq     r1, r2, .voo_dim
    move.q  r2, #0x44            ; 'D' (DIM/DITHER)
    beq     r1, r2, .voo_d_cmd
    move.q  r2, #0x4C            ; 'L' (LFB)
    beq     r1, r2, .voo_lfb
    move.q  r2, #0x50            ; 'P' (PIXEL)
    beq     r1, r2, .voo_pixel
    move.q  r2, #0x54            ; 'T' (TRI*)
    beq     r1, r2, .voo_tri_dispatch
    move.q  r2, #0x41            ; 'A' (ALPHA)
    beq     r1, r2, .voo_alpha
    move.q  r2, #0x46            ; 'F' (FOG)
    beq     r1, r2, .voo_fog
    move.q  r2, #0x52            ; 'R' (RGB)
    beq     r1, r2, .voo_rgb
    rts
.voo_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_ENABLE
    move.q  r2, #1
    store.l r2, (r1)
    rts
.voo_off_check:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46
    bne     r4, r2, .voo_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .voo_done
    add.q   r17, r3, #1
    la      r1, VOODOO_ENABLE
    store.l r0, (r1)
    rts
.voo_c_cmd:
    ; CL=CLEAR/CLIP, CO=COMBINE, CH=CHROMAKEY
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x4C            ; 'L' → CLEAR or CLIP
    beq     r4, r2, .voo_cl_cmd
    move.q  r2, #0x48            ; 'H' → CHROMAKEY
    beq     r4, r2, .voo_chromakey
    ; COMBINE — skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_FBZCOLOR_PATH
    store.l r8, (r1)
    rts
.voo_cl_cmd:
    ; CLE=CLEAR, CLI=CLIP
    add.q   r3, r17, #2
    load.b  r4, (r3)
    move.q  r2, #0x45            ; 'E' → CLEAR
    beq     r4, r2, .voo_clear
    ; CLIP — skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse left, top, right, bottom
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; left
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .voo_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; top
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .voo_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r24, r8              ; right
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .voo_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; CLIP_LEFT_RIGHT = (left << 16) | right
    lsl.l   r1, r22, #16
    or.l    r1, r1, r24
    la      r3, VOODOO_CLIP_LEFT_RIGHT
    store.l r1, (r3)
    ; CLIP_LOW_Y_HIGH = (top << 16) | bottom
    lsl.l   r1, r23, #16
    or.l    r1, r1, r8
    la      r3, VOODOO_CLIP_LOW_Y_HIGH
    store.l r1, (r3)
    rts
.voo_clear:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Optional colour (default 0)
    move.q  r22, r0
    load.b  r1, (r17)
    beqz    r1, .voo_clear_do
    move.q  r2, #0x3A
    beq     r1, r2, .voo_clear_do
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
.voo_clear_do:
    la      r1, VOODOO_COLOR0
    store.l r22, (r1)
    la      r1, VOODOO_FAST_FILL_CMD
    store.l r0, (r1)
    rts
.voo_s_cmd:
    ; SW=SWAP, ST=STATUS
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x57            ; 'W' → SWAP
    beq     r4, r2, .voo_swap
    ; STATUS is read-only, ignore as statement
    rts
.voo_swap:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, VOODOO_SWAP_BUFFER_CMD
    store.l r0, (r1)
    rts
.voo_d_cmd:
    ; DI=DIM, DIT=DITHER
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x49            ; 'I' → DIM or DITHER
    bne     r4, r2, .voo_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    move.q  r2, #0x54            ; 'T' → DITHER
    beq     r4, r2, .voo_dither
    ; DIM — fall through
.voo_dim:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse w, h
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .voo_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; VIDEO_DIM = (w << 16) | h
    lsl.l   r1, r22, #16
    or.l    r1, r1, r8
    la      r3, VOODOO_VIDEO_DIM
    store.l r1, (r3)
    rts
.voo_lfb:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_LFB_MODE
    store.l r8, (r1)
    rts
.voo_pixel:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse x, y, colour
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .voo_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .voo_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Write pixel to LFB: offset = y * width + x (assume width from VIDEO_DIM)
    la      r1, VOODOO_VIDEO_DIM
    load.l  r1, (r1)
    lsr.l   r1, r1, #16         ; width = upper 16 bits
    mulu.l  r1, r23, r1
    add.l   r1, r1, r22
    ; Write colour at VOODOO_BASE + 0x200 + offset*4 (LFB area)
    lsl.l   r1, r1, #2
    move.l  r2, #0xF4200
    add.q   r1, r1, r2
    store.l r8, (r1)
    rts
.voo_done:
    rts

; --- VOODOO TRI* sub-dispatch ---
; Checks 4th char (after TRI) to route: C=TRICOLOR, S=TRISHADE, D=TRIDEPTH,
; U=TRIUV, W=TRIW
.voo_tri_dispatch:
    add.q   r3, r17, #3          ; peek at 4th char (TRI+?)
    load.b  r4, (r3)
    move.q  r2, #0x43            ; 'C' → TRICOLOR
    beq     r4, r2, .voo_tricolor
    move.q  r2, #0x53            ; 'S' → TRISHADE
    beq     r4, r2, .voo_trishade
    move.q  r2, #0x44            ; 'D' → TRIDEPTH
    beq     r4, r2, .voo_tridepth
    move.q  r2, #0x55            ; 'U' → TRIUV
    beq     r4, r2, .voo_triuv
    move.q  r2, #0x57            ; 'W' → TRIW
    beq     r4, r2, .voo_triw
    ; Unknown TRI* — default to TRICOLOR
    bra     .voo_tricolor

; --- VOODOO TRICOLOR r,g,b[,a] ---
; Writes START_R, START_G, START_B, optionally START_A registers.
.voo_tricolor:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse r
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_START_R
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .voo_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse g
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_START_G
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .voo_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse b
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_START_B
    store.l r8, (r1)
    ; Check for optional alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .tricolor_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse a
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_START_A
    store.l r8, (r1)
.tricolor_done:
    rts

; --- VOODOO ALPHA sub-commands ---
; ALPHA TEST ON/OFF, ALPHA FUNC n, ALPHA BLEND ON/OFF,
; ALPHA SRC n, ALPHA DST n
.voo_alpha:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x54            ; 'T' → TEST
    beq     r1, r2, .alpha_test
    move.q  r2, #0x46            ; 'F' → FUNC
    beq     r1, r2, .alpha_func
    move.q  r2, #0x42            ; 'B' → BLEND
    beq     r1, r2, .alpha_blend
    move.q  r2, #0x53            ; 'S' → SRC
    beq     r1, r2, .alpha_src
    move.q  r2, #0x44            ; 'D' → DST
    beq     r1, r2, .alpha_dst
    rts
.alpha_test:
    ; ALPHA TEST ON/OFF — toggle ALPHA_TEST_EN (bit 0) in ALPHA_MODE
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .alpha_test_on
    ; OFF — skip keyword, clear bit
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, VOODOO_ALPHA_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_ALPHA_TEST_EN
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3
    store.l r2, (r1)
    rts
.alpha_test_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_ALPHA_MODE
    load.l  r2, (r1)
    or.l    r2, r2, #VOODOO_ALPHA_TEST_EN
    store.l r2, (r1)
    rts
.alpha_func:
    ; ALPHA FUNC n — set bits 1-3 of ALPHA_MODE
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_ALPHA_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_ALPHA_FUNC_MASK
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3           ; clear bits 1-3
    and.l   r3, r8, #7
    lsl.l   r3, r3, #VOODOO_ALPHA_FUNC_SHIFT
    or.l    r2, r2, r3
    store.l r2, (r1)
    rts
.alpha_blend:
    ; ALPHA BLEND ON/OFF — toggle ALPHA_BLEND_EN (bit 4) in ALPHA_MODE
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .alpha_blend_on
    ; OFF — skip keyword, clear bit
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, VOODOO_ALPHA_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_ALPHA_BLEND_EN
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3
    store.l r2, (r1)
    rts
.alpha_blend_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_ALPHA_MODE
    load.l  r2, (r1)
    or.l    r2, r2, #VOODOO_ALPHA_BLEND_EN
    store.l r2, (r1)
    rts
.alpha_src:
    ; ALPHA SRC n — set bits 8-11 of ALPHA_MODE
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_ALPHA_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_ALPHA_SRC_RGB
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3           ; clear bits 8-11
    and.l   r3, r8, #0x0F
    lsl.l   r3, r3, #8
    or.l    r2, r2, r3
    store.l r2, (r1)
    rts
.alpha_dst:
    ; ALPHA DST n — set bits 12-15 of ALPHA_MODE
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_ALPHA_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_ALPHA_DST_RGB
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3           ; clear bits 12-15
    and.l   r3, r8, #0x0F
    lsl.l   r3, r3, #12
    or.l    r2, r2, r3
    store.l r2, (r1)
    rts

; --- VOODOO FOG sub-commands ---
; FOG ON/OFF, FOG COLOR r,g,b
.voo_fog:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .fog_on
    move.q  r2, #0x4F            ; 'O' → OFF
    beq     r1, r2, .fog_off_check
    move.q  r2, #TK_COLOR        ; COLOR token (tokenizer converts COLOR keyword)
    beq     r1, r2, .fog_color
    move.q  r2, #0x43            ; 'C' → COLOR (literal fallback)
    beq     r1, r2, .fog_color
    rts
.fog_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_FOG_MODE
    load.l  r2, (r1)
    or.l    r2, r2, #VOODOO_FOG_ENABLE
    store.l r2, (r1)
    rts
.fog_off_check:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46            ; 'F'
    bne     r4, r2, .fog_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .fog_done
    add.q   r17, r3, #1
    la      r1, VOODOO_FOG_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_FOG_ENABLE
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3
    store.l r2, (r1)
    rts
.fog_color:
    ; FOG COLOR r,g,b — pack (b<<16)|(g<<8)|r into FOG_COLOR
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse r
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; r
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .fog_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse g
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; g
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .fog_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse b
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Pack: (b << 16) | (g << 8) | r
    lsl.l   r1, r8, #16
    lsl.l   r2, r23, #8
    or.l    r1, r1, r2
    or.l    r1, r1, r22
    la      r3, VOODOO_FOG_COLOR
    store.l r1, (r3)
    rts
.fog_done:
    rts

; --- VOODOO DITHER ON/OFF ---
; Toggle VOODOO_FBZ_DITHER (0x0100) in VOODOO_FBZ_MODE
.voo_dither:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .dither_on
    ; OFF — skip keyword, clear bit
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_FBZ_DITHER
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3
    store.l r2, (r1)
    rts
.dither_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    or.l    r2, r2, #VOODOO_FBZ_DITHER
    store.l r2, (r1)
    rts

; --- VOODOO CHROMAKEY sub-commands ---
; CHROMAKEY ON/OFF, CHROMAKEY COLOR r,g,b
.voo_chromakey:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .chroma_on
    move.q  r2, #0x4F            ; 'O' → OFF
    beq     r1, r2, .chroma_off_check
    move.q  r2, #TK_COLOR        ; COLOR token (tokenizer converts COLOR keyword)
    beq     r1, r2, .chroma_color
    move.q  r2, #0x43            ; 'C' → COLOR (literal fallback)
    beq     r1, r2, .chroma_color
    rts
.chroma_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    or.l    r2, r2, #VOODOO_FBZ_CHROMAKEY
    store.l r2, (r1)
    rts
.chroma_off_check:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46            ; 'F'
    bne     r4, r2, .chroma_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .chroma_done
    add.q   r17, r3, #1
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_FBZ_CHROMAKEY
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3
    store.l r2, (r1)
    rts
.chroma_color:
    ; CHROMAKEY COLOR r,g,b — pack (b<<16)|(g<<8)|r into CHROMA_KEY
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse r
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8              ; r
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .chroma_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse g
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r23, r8              ; g
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .chroma_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse b
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Pack: (b << 16) | (g << 8) | r
    lsl.l   r1, r8, #16
    lsl.l   r2, r23, #8
    or.l    r1, r1, r2
    or.l    r1, r1, r22
    la      r3, VOODOO_CHROMA_KEY
    store.l r1, (r3)
    rts
.chroma_done:
    rts

; --- VOODOO TRISHADE drdx, drdy, dgdx, dgdy, dbdx, dbdy ---
; Writes Gouraud shading delta registers for triangle interpolation.
.voo_trishade:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse and write dR/dX
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DRDX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .trishade_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dR/dY
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DRDY
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .trishade_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dG/dX
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DGDX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .trishade_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dG/dY
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DGDY
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .trishade_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dB/dX
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DBDX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .trishade_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dB/dY
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DBDY
    store.l r8, (r1)
.trishade_done:
    rts

; --- VOODOO TRIDEPTH start_z, dzdx, dzdy ---
; Writes START_Z and depth interpolation deltas.
.voo_tridepth:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse and write START_Z
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_START_Z
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .tridepth_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dZ/dX
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DZDX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .tridepth_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dZ/dY
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DZDY
    store.l r8, (r1)
.tridepth_done:
    rts

; --- VOODOO TRIUV start_s, start_t, dsdx, dtdx, dsdy, dtdy ---
; Writes texture coordinate start values and interpolation deltas.
.voo_triuv:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse and write START_S
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_START_S
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .triuv_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write START_T
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_START_T
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .triuv_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dS/dX
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DSDX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .triuv_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dT/dX
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DTDX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .triuv_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dS/dY
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DSDY
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .triuv_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dT/dY
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DTDY
    store.l r8, (r1)
.triuv_done:
    rts

; --- VOODOO TRIW start_w, dwdx, dwdy ---
; Writes perspective W (1/Z) start value and interpolation deltas.
.voo_triw:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse and write START_W
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_START_W
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .triw_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dW/dX
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DWDX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .triw_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    ; Parse and write dW/dY
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_DWDY
    store.l r8, (r1)
.triw_done:
    rts

; --- VOODOO RGB ON/OFF ---
; Toggle VOODOO_FBZ_RGB_WRITE (0x0200) in VOODOO_FBZ_MODE
.voo_rgb:
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .rgb_on
    ; OFF — skip keyword, clear bit
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_FBZ_RGB_WRITE
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3
    store.l r2, (r1)
    rts
.rgb_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    or.l    r2, r2, #VOODOO_FBZ_RGB_WRITE
    store.l r2, (r1)
    rts

; ============================================================================
; hw_vertex - VERTEX A/B/C x, y
; ============================================================================
; Coordinates are converted to 12.4 fixed-point (value << 4).

hw_vertex:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    ; A, B, or C
    move.q  r2, #0x41            ; 'A'
    beq     r1, r2, .vtx_a
    move.q  r2, #0x42            ; 'B'
    beq     r1, r2, .vtx_b
    move.q  r2, #0x43            ; 'C'
    beq     r1, r2, .vtx_c
    rts
.vtx_a:
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    lsl.l   r8, r8, #4
    la      r1, VOODOO_VERTEX_AX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .vtx_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    lsl.l   r8, r8, #4
    la      r1, VOODOO_VERTEX_AY
    store.l r8, (r1)
    rts
.vtx_b:
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    lsl.l   r8, r8, #4
    la      r1, VOODOO_VERTEX_BX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .vtx_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    lsl.l   r8, r8, #4
    la      r1, VOODOO_VERTEX_BY
    store.l r8, (r1)
    rts
.vtx_c:
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    lsl.l   r8, r8, #4
    la      r1, VOODOO_VERTEX_CX
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .vtx_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    lsl.l   r8, r8, #4
    la      r1, VOODOO_VERTEX_CY
    store.l r8, (r1)
.vtx_done:
    rts

; ============================================================================
; hw_triangle - TRIANGLE
; ============================================================================
; Submits the triangle command.

hw_triangle:
    la      r1, VOODOO_TRIANGLE_CMD
    store.l r0, (r1)
    rts

; ============================================================================
; hw_zbuffer - ZBUFFER ON/OFF/FUNC/WRITE
; ============================================================================

hw_zbuffer:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .zb_on
    move.q  r2, #0x4F            ; 'O' (OFF)
    beq     r1, r2, .zb_off_check
    move.q  r2, #0x46            ; 'F' (FUNC)
    beq     r1, r2, .zb_func
    move.q  r2, #0x57            ; 'W' (WRITE)
    beq     r1, r2, .zb_write
    rts
.zb_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    or.l    r2, r2, #VOODOO_FBZ_DEPTH_ENABLE
    store.l r2, (r1)
    rts
.zb_off_check:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46
    bne     r4, r2, .zb_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .zb_done
    add.q   r17, r3, #1
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_FBZ_DEPTH_ENABLE
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3
    store.l r2, (r1)
    rts
.zb_func:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; Depth func goes in bits 3-1 of fbzMode
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    and.l   r2, r2, #0xFFFFFFF1  ; clear bits 1-3
    and.l   r3, r8, #7
    lsl.l   r3, r3, #1
    or.l    r2, r2, r3
    store.l r2, (r1)
    rts
.zb_write:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .zb_wr_on
    ; OFF
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_FBZ_DEPTH_WRITE
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3
    store.l r2, (r1)
    rts
.zb_wr_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_FBZ_MODE
    load.l  r2, (r1)
    or.l    r2, r2, #VOODOO_FBZ_DEPTH_WRITE
    store.l r2, (r1)
    rts
.zb_done:
    rts

; ============================================================================
; hw_texture - TEXTURE MODE/BASE/DIM/UPLOAD/ON/OFF
; ============================================================================

hw_texture:
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #TK_ON
    beq     r1, r2, .tex_on
    move.q  r2, #0x4F            ; 'O' (OFF)
    beq     r1, r2, .tex_off_check
    move.q  r2, #0x4D            ; 'M' (MODE)
    beq     r1, r2, .tex_mode
    move.q  r2, #0x42            ; 'B' (BASE)
    beq     r1, r2, .tex_base
    move.q  r2, #TK_DIM
    beq     r1, r2, .tex_dim
    move.q  r2, #0x44            ; 'D' (DIM)
    beq     r1, r2, .tex_dim
    move.q  r2, #0x55            ; 'U' (UPLOAD)
    beq     r1, r2, .tex_upload
    rts
.tex_on:
    add.q   r17, r17, #1
    la      r1, VOODOO_TEXTURE_MODE
    load.l  r2, (r1)
    or.l    r2, r2, #VOODOO_TEX_ENABLE
    store.l r2, (r1)
    rts
.tex_off_check:
    add.q   r3, r17, #1
    load.b  r4, (r3)
    move.q  r2, #0x46
    bne     r4, r2, .tex_done
    add.q   r3, r3, #1
    load.b  r4, (r3)
    bne     r4, r2, .tex_done
    add.q   r17, r3, #1
    la      r1, VOODOO_TEXTURE_MODE
    load.l  r2, (r1)
    move.l  r3, #VOODOO_TEX_ENABLE
    eor.l   r3, r3, #0xFFFFFFFF
    and.l   r2, r2, r3
    store.l r2, (r1)
    rts
.tex_mode:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_TEXTURE_MODE
    store.l r8, (r1)
    rts
.tex_base:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    ; Parse LOD level
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    move.q  r22, r8
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .tex_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    ; TEX_BASE0 + lod*4
    mulu.l  r1, r22, #4
    la      r2, VOODOO_TEX_BASE0
    add.q   r1, r1, r2
    store.l r8, (r1)
    rts
.tex_dim:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_TEX_WIDTH
    store.l r8, (r1)
    jsr     exec_skip_spaces
    load.b  r1, (r17)
    move.q  r2, #0x2C
    bne     r1, r2, .tex_done
    add.q   r17, r17, #1
    jsr     exec_skip_spaces
    jsr     expr_eval
    jsr     fp_int
    jsr     fp_fix
    la      r1, VOODOO_TEX_HEIGHT
    store.l r8, (r1)
    rts
.tex_upload:
    ; Skip keyword
    add.q   r17, r17, #1
    jsr     skip_alpha
    la      r1, VOODOO_TEX_UPLOAD
    store.l r0, (r1)
    rts
.tex_done:
    rts

; ============================================================================
; EOF
; ============================================================================
