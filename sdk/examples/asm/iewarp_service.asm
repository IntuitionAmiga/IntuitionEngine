; iewarp_service.asm — IE64 coprocessor worker service for iewarp.library
;
; This program runs as an IE64 coprocessor worker, polling its ring buffer
; for operation requests from the M68K host. It dispatches each operation,
; writes the response, and advances the ring tail pointer.
;
; Ring buffer layout (at RING_BASE = MAILBOX_BASE + 5 * RING_STRIDE):
;   +0x00: head (byte) — next write slot (producer / Go manager)
;   +0x01: tail (byte) — next read slot (consumer / this worker)
;   +0x02: capacity (byte) — ring depth (16)
;   +0x08: request descriptors (16 × 32 bytes = 512 bytes)
;   +0x208: response descriptors (16 × 16 bytes = 256 bytes)
;
; Request descriptor (32 bytes):
;   +0x00: ticket (u32)
;   +0x04: cpuType (u32)
;   +0x08: op (u32)
;   +0x0C: flags (u32) — extra parameter (mask ptr, strides, etc.)
;   +0x10: reqPtr (u32)
;   +0x14: reqLen (u32)
;   +0x18: respPtr (u32)
;   +0x1C: respCap (u32)
;
; Response descriptor (16 bytes):
;   +0x00: ticket (u32)
;   +0x04: status (u32) — 0=pending, 2=OK, 3=error
;   +0x08: resultCode (u32)
;   +0x0C: respLen (u32)
;
; (c) 2026 Zayn Otley - GPLv3 or later

include "ie64.inc"

; ============================================================================
; Ring buffer constants
; ============================================================================

WORKER_IE64_BASE  equ 0x3A0000

MAILBOX_BASE      equ 0x790000
RING_STRIDE       equ 0x300
RING_INDEX        equ 5                          ; IE64 is ring index 5
RING_BASE         equ MAILBOX_BASE + (RING_INDEX * RING_STRIDE)

; Ring offsets
RING_HEAD         equ 0x00
RING_TAIL         equ 0x01
RING_ENTRIES      equ 0x08
RING_RESPONSES    equ 0x208

; Request descriptor offsets
REQ_TICKET        equ 0x00
REQ_OP            equ 0x08
REQ_FLAGS         equ 0x0C
REQ_REQPTR        equ 0x10
REQ_REQLEN        equ 0x14
REQ_RESPPTR       equ 0x18
REQ_RESPCAP       equ 0x1C

; Request descriptor size
REQ_SIZE          equ 32

; Response descriptor offsets
RESP_TICKET       equ 0x00
RESP_STATUS       equ 0x04
RESP_RESULT       equ 0x08
RESP_LEN          equ 0x0C

; Response descriptor size
RESP_SIZE         equ 16

; Status codes
STATUS_PENDING    equ 0
STATUS_OK         equ 2
STATUS_ERROR      equ 3

; Operation codes (must match libraries/iewarp.h)
OP_NOP            equ 0
OP_MEMCPY         equ 1
OP_MEMCPY_QUICK   equ 2
OP_MEMSET         equ 3
OP_MEMMOVE        equ 4
OP_BLIT_COPY      equ 10
OP_BLIT_MASK      equ 11
OP_BLIT_SCALE     equ 12
OP_BLIT_CONVERT   equ 13
OP_BLIT_ALPHA     equ 14
OP_FILL_RECT      equ 15
OP_AREA_FILL      equ 16
OP_PIXEL_PROCESS  equ 17
OP_AUDIO_MIX      equ 20
OP_AUDIO_RESAMPLE equ 21
OP_AUDIO_DECODE   equ 22
OP_FP_BATCH       equ 30
OP_MATRIX_MUL     equ 31
OP_CRC32          equ 32
OP_GRADIENT_FILL  equ 40
OP_GLYPH_RENDER   equ 41
OP_SCROLL         equ 42

; Pixel processing sub-operations (match cybergraphx POP_* constants)
POP_TINT          equ 0
POP_BRIGHTEN      equ 2
POP_DARKEN        equ 3
POP_SETALPHA      equ 4
POP_COLOR2GREY    equ 6
POP_NEGATIVE      equ 7

; FP batch sub-operations
FP_OP_ADD         equ 0
FP_OP_SUB         equ 1
FP_OP_MUL         equ 2
FP_OP_DIV         equ 3
FP_OP_SQRT        equ 4
FP_OP_SIN         equ 5
FP_OP_COS         equ 6
FP_OP_TAN         equ 7
FP_OP_ATAN        equ 8
FP_OP_LOG         equ 9
FP_OP_EXP         equ 10
FP_OP_POW         equ 11
FP_OP_ABS         equ 12
FP_OP_NEG         equ 13

; Audio codec IDs
CODEC_IMA_ADPCM   equ 1

; ============================================================================
; Entry point
; ============================================================================

    org WORKER_IE64_BASE

    ; R30 = ring base address (constant)
    la r30, RING_BASE

; ============================================================================
; Main poll loop — spin on head != tail
; ============================================================================

poll_loop:
    ; Read head and tail bytes
    load.b r2, RING_HEAD(r30)             ; r2 = head
    load.b r3, RING_TAIL(r30)             ; r3 = tail
    beq r2, r3, poll_loop                 ; head == tail → empty, spin

    ; We have a request at slot [tail]
    ; Compute request descriptor address: ring_base + RING_ENTRIES + tail * REQ_SIZE
    mulu.l r4, r3, #REQ_SIZE             ; r4 = tail * 32
    add.l r4, r4, #RING_ENTRIES           ; r4 = offset to request
    add.l r5, r30, r4                     ; r5 = &request[tail]

    ; Read request fields
    load.l r10, REQ_TICKET(r5)            ; r10 = ticket
    load.l r11, REQ_OP(r5)                ; r11 = op
    load.l r12, REQ_REQPTR(r5)            ; r12 = reqPtr
    load.l r13, REQ_REQLEN(r5)            ; r13 = reqLen
    load.l r14, REQ_RESPPTR(r5)           ; r14 = respPtr
    load.l r15, REQ_RESPCAP(r5)           ; r15 = respCap
    load.l r29, REQ_FLAGS(r5)             ; r29 = flags (extra param)

    ; Compute response descriptor address: ring_base + RING_RESPONSES + tail * RESP_SIZE
    mulu.l r6, r3, #RESP_SIZE            ; r6 = tail * 16
    add.l r6, r6, #RING_RESPONSES         ; r6 = offset to response
    add.l r20, r30, r6                    ; r20 = &response[tail]

    ; Write response ticket (always, even before dispatch)
    store.l r10, RESP_TICKET(r20)
    ; Mark as pending
    move.l r7, #STATUS_PENDING
    store.l r7, RESP_STATUS(r20)

    ; Dispatch based on opcode
    beq r11, r0, op_nop

    move.l r8, #OP_MEMCPY
    beq r11, r8, op_memcpy
    move.l r8, #OP_MEMCPY_QUICK
    beq r11, r8, op_memcpy_quick
    move.l r8, #OP_MEMSET
    beq r11, r8, op_memset
    move.l r8, #OP_MEMMOVE
    beq r11, r8, op_memmove

    move.l r8, #OP_BLIT_COPY
    beq r11, r8, op_blit_copy
    move.l r8, #OP_BLIT_MASK
    beq r11, r8, op_blit_mask
    move.l r8, #OP_BLIT_SCALE
    beq r11, r8, op_blit_scale
    move.l r8, #OP_BLIT_CONVERT
    beq r11, r8, op_blit_convert
    move.l r8, #OP_BLIT_ALPHA
    beq r11, r8, op_blit_alpha
    move.l r8, #OP_FILL_RECT
    beq r11, r8, op_fill_rect
    move.l r8, #OP_AREA_FILL
    beq r11, r8, op_area_fill
    move.l r8, #OP_PIXEL_PROCESS
    beq r11, r8, op_pixel_process

    move.l r8, #OP_AUDIO_MIX
    beq r11, r8, op_audio_mix
    move.l r8, #OP_AUDIO_RESAMPLE
    beq r11, r8, op_audio_resample
    move.l r8, #OP_AUDIO_DECODE
    beq r11, r8, op_audio_decode

    move.l r8, #OP_FP_BATCH
    beq r11, r8, op_fp_batch
    move.l r8, #OP_MATRIX_MUL
    beq r11, r8, op_matrix_mul
    move.l r8, #OP_CRC32
    beq r11, r8, op_crc32

    move.l r8, #OP_GRADIENT_FILL
    beq r11, r8, op_gradient_fill
    move.l r8, #OP_GLYPH_RENDER
    beq r11, r8, op_glyph_render
    move.l r8, #OP_SCROLL
    beq r11, r8, op_scroll

    ; Unknown op — return error
    bra op_error

; ============================================================================
; Operation handlers — Memory primitives
; ============================================================================

; ── NOP ────────────────────────────────────────────────────────────
op_nop:
    bra op_done_ok

; ── MEMCPY ─────────────────────────────────────────────────────────
; reqPtr = src, reqLen = len, respPtr = dst
op_memcpy:
    ; r12 = src, r13 = len, r14 = dst
    beq r13, r0, op_done_ok               ; zero length → done

    ; Try ULONG-aligned fast path (len >= 4 and both aligned)
    move.l r7, #3
    and.l r8, r12, r7                     ; src & 3
    bne r8, r0, memcpy_byte               ; not aligned
    and.l r8, r14, r7                     ; dst & 3
    bne r8, r0, memcpy_byte               ; not aligned

    ; Both aligned — copy LONGs first
    move.l r16, r13                       ; r16 = remaining
    lsr.l r17, r16, #2                    ; r17 = count of LONGs
    beq r17, r0, memcpy_tail

    move.l r18, r12                       ; r18 = src cursor
    move.l r19, r14                       ; r19 = dst cursor

memcpy_long_loop:
    load.l r7, 0(r18)
    store.l r7, 0(r19)
    add.l r18, r18, #4
    add.l r19, r19, #4
    sub.l r17, r17, #1
    bne r17, r0, memcpy_long_loop

    ; Remaining bytes
    and.l r16, r13, #3                    ; remaining = len & 3
    beq r16, r0, op_done_ok

    ; Copy remaining bytes
    move.l r12, r18                       ; update src
    move.l r14, r19                       ; update dst
    move.l r13, r16                       ; update len
    bra memcpy_byte

memcpy_tail:
    ; Fall through to byte copy for < 4 bytes remaining
memcpy_byte:
    move.l r18, r12                       ; r18 = src
    move.l r19, r14                       ; r19 = dst
    move.l r16, r13                       ; r16 = count

memcpy_byte_loop:
    load.b r7, 0(r18)
    store.b r7, 0(r19)
    add.l r18, r18, #1
    add.l r19, r19, #1
    sub.l r16, r16, #1
    bne r16, r0, memcpy_byte_loop
    bra op_done_ok

; ── MEMCPY_QUICK ───────────────────────────────────────────────────
; Same as MEMCPY but assumes ULONG alignment
op_memcpy_quick:
    beq r13, r0, op_done_ok

    lsr.l r17, r13, #2                    ; count of LONGs
    beq r17, r0, op_done_ok               ; nothing to copy

    move.l r18, r12                       ; src
    move.l r19, r14                       ; dst

memcpyq_loop:
    load.l r7, 0(r18)
    store.l r7, 0(r19)
    add.l r18, r18, #4
    add.l r19, r19, #4
    sub.l r17, r17, #1
    bne r17, r0, memcpyq_loop
    bra op_done_ok

; ── MEMSET ─────────────────────────────────────────────────────────
; reqPtr = fill value (packed in ptr field), reqLen = len, respPtr = dst
op_memset:
    beq r13, r0, op_done_ok

    ; r12 = fill value (low byte), r14 = dst, r13 = len
    and.l r7, r12, #0xFF                  ; extract byte value
    move.l r18, r14                       ; dst cursor
    move.l r16, r13                       ; count

memset_byte_loop:
    store.b r7, 0(r18)
    add.l r18, r18, #1
    sub.l r16, r16, #1
    bne r16, r0, memset_byte_loop
    bra op_done_ok

; ── MEMMOVE ────────────────────────────────────────────────────────
; reqPtr = src, reqLen = len, respPtr = dst
; Handles overlapping regions
op_memmove:
    beq r13, r0, op_done_ok

    ; If dst < src or dst >= src+len, copy forward
    blt r14, r12, memmove_fwd
    add.l r7, r12, r13                    ; r7 = src + len
    bge r14, r7, memmove_fwd

    ; Overlap: copy backward
    add.l r18, r12, r13                   ; r18 = src + len
    sub.l r18, r18, #1                    ; r18 = src + len - 1
    add.l r19, r14, r13                   ; r19 = dst + len
    sub.l r19, r19, #1                    ; r19 = dst + len - 1
    move.l r16, r13

memmove_bwd_loop:
    load.b r7, 0(r18)
    store.b r7, 0(r19)
    sub.l r18, r18, #1
    sub.l r19, r19, #1
    sub.l r16, r16, #1
    bne r16, r0, memmove_bwd_loop
    bra op_done_ok

memmove_fwd:
    move.l r18, r12
    move.l r19, r14
    move.l r16, r13

memmove_fwd_loop:
    load.b r7, 0(r18)
    store.b r7, 0(r19)
    add.l r18, r18, #1
    add.l r19, r19, #1
    sub.l r16, r16, #1
    bne r16, r0, memmove_fwd_loop
    bra op_done_ok

; ============================================================================
; Operation handlers — Graphics primitives
; ============================================================================

; ── BLIT_COPY ──────────────────────────────────────────────────────
; reqPtr = src, reqLen = width|(height<<16), respPtr = dst
; respCap = srcStride|(dstStride<<16)
op_blit_copy:
    ; Extract width/height from reqLen
    and.l r16, r13, #0xFFFF              ; width
    lsr.l r17, r13, #16                  ; height
    beq r17, r0, op_done_ok
    beq r16, r0, op_done_ok

    ; Extract strides from respCap
    and.l r18, r15, #0xFFFF              ; srcStride
    lsr.l r19, r15, #16                  ; dstStride

    ; Row-by-row copy
    move.l r21, r12                       ; src cursor
    move.l r22, r14                       ; dst cursor

blit_row_loop:
    ; Copy one row (width bytes)
    move.l r23, r16                       ; bytes remaining in row
    move.l r24, r21                       ; row src
    move.l r25, r22                       ; row dst

blit_col_loop:
    load.b r7, 0(r24)
    store.b r7, 0(r25)
    add.l r24, r24, #1
    add.l r25, r25, #1
    sub.l r23, r23, #1
    bne r23, r0, blit_col_loop

    ; Advance to next row
    add.l r21, r21, r18                   ; src += srcStride
    add.l r22, r22, r19                   ; dst += dstStride
    sub.l r17, r17, #1
    bne r17, r0, blit_row_loop
    bra op_done_ok

; ── BLIT_MASK ──────────────────────────────────────────────────────
; Masked blit: copy src→dst where mask byte is non-zero.
; reqPtr = src, reqLen = width|(height<<16), respPtr = dst
; respCap = srcStride|(dstStride<<16), flags = maskPtr
; Mask has same dimensions/stride as src (one byte per pixel).
op_blit_mask:
    and.l r16, r13, #0xFFFF              ; width (bytes)
    lsr.l r17, r13, #16                  ; height
    beq r17, r0, op_done_ok
    beq r16, r0, op_done_ok

    and.l r18, r15, #0xFFFF              ; srcStride
    lsr.l r19, r15, #16                  ; dstStride
    ; r29 = maskPtr (from flags)

    move.l r21, r12                       ; src cursor
    move.l r22, r14                       ; dst cursor
    move.l r26, r29                       ; mask cursor

mask_row_loop:
    move.l r23, r16
    move.l r24, r21                       ; row src
    move.l r25, r22                       ; row dst
    move.l r27, r26                       ; row mask

mask_col_loop:
    load.b r7, 0(r27)                    ; mask byte
    beq r7, r0, mask_skip                ; mask=0 → skip pixel
    load.b r7, 0(r24)                    ; src pixel
    store.b r7, 0(r25)                   ; write to dst

mask_skip:
    add.l r24, r24, #1
    add.l r25, r25, #1
    add.l r27, r27, #1
    sub.l r23, r23, #1
    bne r23, r0, mask_col_loop

    add.l r21, r21, r18                   ; src += srcStride
    add.l r22, r22, r19                   ; dst += dstStride
    add.l r26, r26, r18                   ; mask += srcStride (same stride)
    sub.l r17, r17, #1
    bne r17, r0, mask_row_loop
    bra op_done_ok

; ── BLIT_SCALE ─────────────────────────────────────────────────────
; Nearest-neighbor bitmap scale using DDA.
; reqPtr = src, reqLen = srcW|(srcH<<16), respPtr = dst
; respCap = dstW|(dstH<<16), flags = srcStride|(dstStride<<16)
op_blit_scale:
    and.l r16, r13, #0xFFFF              ; srcW
    lsr.l r17, r13, #16                  ; srcH
    beq r16, r0, op_done_ok
    beq r17, r0, op_done_ok

    and.l r18, r15, #0xFFFF              ; dstW
    lsr.l r19, r15, #16                  ; dstH
    beq r18, r0, op_done_ok
    beq r19, r0, op_done_ok

    ; Extract strides from flags
    and.l r21, r29, #0xFFFF              ; srcStride
    lsr.l r22, r29, #16                  ; dstStride

    ; r12 = src base, r14 = dst base
    ; Outer loop: for each dst row
    move.l r23, r14                       ; dstRow cursor
    move.l r24, r0                        ; dstY = 0

scale_row_loop:
    ; srcY = (dstY * srcH) / dstH
    mulu.l r7, r24, r17                  ; dstY * srcH
    divu.l r7, r7, r19                   ; / dstH = srcY
    mulu.l r8, r7, r21                   ; srcY * srcStride
    add.l r25, r12, r8                   ; srcRow = src + srcY * srcStride

    ; Inner loop: for each dst pixel in row
    move.l r26, r23                       ; dstPixel cursor
    move.l r27, r0                        ; dstX = 0

scale_col_loop:
    ; srcX = (dstX * srcW) / dstW
    mulu.l r7, r27, r16                  ; dstX * srcW
    divu.l r7, r7, r18                   ; / dstW = srcX
    ; Copy one byte (CLUT8)
    add.l r8, r25, r7                    ; src + srcX
    load.b r7, 0(r8)
    store.b r7, 0(r26)

    add.l r26, r26, #1
    add.l r27, r27, #1
    blt r27, r18, scale_col_loop

    add.l r23, r23, r22                   ; dstRow += dstStride
    add.l r24, r24, #1
    blt r24, r19, scale_row_loop
    bra op_done_ok

; ── BLIT_CONVERT ───────────────────────────────────────────────────
; Pixel format conversion with channel reordering.
; reqPtr = src, reqLen = width|(height<<16), respPtr = dst
; respCap = srcFmt|(dstFmt<<16)
; flags = srcStride|(dstStride<<16), or 0 for tightly packed
;
; Format codes (values >= 16 enable format-aware conversion):
;   PIXFMT_RGBA32 = 16  (R,G,B,A — IE native)
;   PIXFMT_ARGB32 = 17  (A,R,G,B)
;   PIXFMT_RGB24  = 18  (R,G,B — 3 bytes)
;   PIXFMT_BGR24  = 19  (B,G,R — 3 bytes)
;   PIXFMT_BGRA32 = 20  (B,G,R,A)
;   PIXFMT_ABGR32 = 21  (A,B,G,R)
; Legacy: values 1-4 = BPP copy mode (backward compatible)
; Special: srcFmt=1, dstFmt=4 = CLUT8→RGBA32 greyscale

PIXFMT_RGBA32 equ 16
PIXFMT_ARGB32 equ 17
PIXFMT_RGB24  equ 18
PIXFMT_BGR24  equ 19
PIXFMT_BGRA32 equ 20
PIXFMT_ABGR32 equ 21

op_blit_convert:
    and.l r16, r13, #0xFFFF              ; width (pixels)
    lsr.l r17, r13, #16                  ; height
    beq r16, r0, op_done_ok
    beq r17, r0, op_done_ok

    and.l r18, r15, #0xFFFF              ; srcFmt
    lsr.l r19, r15, #16                  ; dstFmt

    ; Compute total pixels
    mulu.l r21, r16, r17                 ; totalPixels = width * height

    ; Check if format-aware mode (either fmt >= 16)
    move.l r7, #16
    bge r18, r7, convert_format_aware
    bge r19, r7, convert_format_aware

    ; Legacy BPP mode: check CLUT8→RGBA32 (1→4)
    move.l r7, #1
    bne r18, r7, convert_legacy_generic
    move.l r7, #4
    bne r19, r7, convert_legacy_generic

    ; CLUT8→RGBA32: expand each byte to 4 bytes (greyscale)
    move.l r22, r12                       ; src cursor
    move.l r23, r14                       ; dst cursor
    move.l r24, r21                       ; pixel count

clut8_rgba_loop:
    load.b r7, 0(r22)                    ; palette index (used as grey value)
    store.b r7, 0(r23)                   ; R
    store.b r7, 1(r23)                   ; G
    store.b r7, 2(r23)                   ; B
    move.l r8, #255
    store.b r8, 3(r23)                   ; A = 255 (opaque)
    add.l r22, r22, #1
    add.l r23, r23, #4
    sub.l r24, r24, #1
    bne r24, r0, clut8_rgba_loop
    bra op_done_ok

convert_legacy_generic:
    ; Legacy generic: copy min(srcBpp, dstBpp) bytes + zero-fill
    move.l r22, r12
    move.l r23, r14
    move.l r24, r21

convert_pixel_loop:
    move.l r25, r18
    blt r19, r18, convert_use_dst_bpp
    bra convert_copy_bytes

convert_use_dst_bpp:
    move.l r25, r19

convert_copy_bytes:
    move.l r26, r0
    move.l r27, r22
    move.l r28, r23

convert_byte_loop:
    bge r26, r25, convert_zero_fill
    load.b r7, 0(r27)
    store.b r7, 0(r28)
    add.l r27, r27, #1
    add.l r28, r28, #1
    add.l r26, r26, #1
    bra convert_byte_loop

convert_zero_fill:
    bge r26, r19, convert_next_pixel
    store.b r0, 0(r28)
    add.l r28, r28, #1
    add.l r26, r26, #1
    bra convert_zero_fill

convert_next_pixel:
    add.l r22, r22, r18
    add.l r23, r23, r19
    sub.l r24, r24, #1
    bne r24, r0, convert_pixel_loop
    bra op_done_ok

    ; ── Format-aware conversion ──
convert_format_aware:
    move.l r22, r12                       ; src cursor
    move.l r23, r14                       ; dst cursor
    move.l r24, r21                       ; pixel count

    ; Same format → straight copy (4 bytes per pixel for 32-bit formats)
    beq r18, r19, cvt_same_fmt

    ; ARGB32(17) → RGBA32(16): rotate bytes left: [A,R,G,B] → [R,G,B,A]
    move.l r7, #PIXFMT_ARGB32
    bne r18, r7, cvt_check_rgb24
    move.l r7, #PIXFMT_RGBA32
    bne r19, r7, cvt_check_rgb24
    bra cvt_argb_to_rgba

    ; RGB24(18) → RGBA32(16): expand 3 → 4, A=0xFF
cvt_check_rgb24:
    move.l r7, #PIXFMT_RGB24
    bne r18, r7, cvt_check_bgra
    move.l r7, #PIXFMT_RGBA32
    bne r19, r7, cvt_check_bgra
    bra cvt_rgb24_to_rgba

    ; BGRA32(20) → RGBA32(16): swap R,B: [B,G,R,A] → [R,G,B,A]
cvt_check_bgra:
    move.l r7, #PIXFMT_BGRA32
    bne r18, r7, cvt_check_bgr24
    move.l r7, #PIXFMT_RGBA32
    bne r19, r7, cvt_check_bgr24
    bra cvt_bgra_to_rgba

    ; BGR24(19) → RGBA32(16): reverse + expand: [B,G,R] → [R,G,B,0xFF]
cvt_check_bgr24:
    move.l r7, #PIXFMT_BGR24
    bne r18, r7, cvt_check_abgr
    move.l r7, #PIXFMT_RGBA32
    bne r19, r7, cvt_check_abgr
    bra cvt_bgr24_to_rgba

    ; ABGR32(21) → RGBA32(16): reverse: [A,B,G,R] → [R,G,B,A]
cvt_check_abgr:
    move.l r7, #PIXFMT_ABGR32
    bne r18, r7, cvt_unsupported
    move.l r7, #PIXFMT_RGBA32
    bne r19, r7, cvt_unsupported

    ; ABGR32 → RGBA32: [A,B,G,R] → [R,G,B,A]
cvt_abgr_loop:
    load.b r7, 0(r22)                    ; A
    load.b r8, 1(r22)                    ; B
    load.b r9, 2(r22)                    ; G
    load.b r10, 3(r22)                   ; R
    store.b r10, 0(r23)                  ; R
    store.b r9, 1(r23)                   ; G
    store.b r8, 2(r23)                   ; B
    store.b r7, 3(r23)                   ; A
    add.l r22, r22, #4
    add.l r23, r23, #4
    sub.l r24, r24, #1
    bne r24, r0, cvt_abgr_loop
    bra op_done_ok

cvt_same_fmt:
    ; Same format: 4-byte copy per pixel (assumes 32-bit formats)
    load.l r7, 0(r22)
    store.l r7, 0(r23)
    add.l r22, r22, #4
    add.l r23, r23, #4
    sub.l r24, r24, #1
    bne r24, r0, cvt_same_fmt
    bra op_done_ok

cvt_argb_to_rgba:
    ; [A,R,G,B] → [R,G,B,A]: byte rotate left
    load.b r7, 0(r22)                    ; A
    load.b r8, 1(r22)                    ; R
    load.b r9, 2(r22)                    ; G
    load.b r10, 3(r22)                   ; B
    store.b r8, 0(r23)                   ; R
    store.b r9, 1(r23)                   ; G
    store.b r10, 2(r23)                  ; B
    store.b r7, 3(r23)                   ; A
    add.l r22, r22, #4
    add.l r23, r23, #4
    sub.l r24, r24, #1
    bne r24, r0, cvt_argb_to_rgba
    bra op_done_ok

cvt_rgb24_to_rgba:
    ; [R,G,B] → [R,G,B,0xFF]
    load.b r7, 0(r22)                    ; R
    load.b r8, 1(r22)                    ; G
    load.b r9, 2(r22)                    ; B
    store.b r7, 0(r23)                   ; R
    store.b r8, 1(r23)                   ; G
    store.b r9, 2(r23)                   ; B
    move.l r10, #255
    store.b r10, 3(r23)                  ; A = 0xFF
    add.l r22, r22, #3
    add.l r23, r23, #4
    sub.l r24, r24, #1
    bne r24, r0, cvt_rgb24_to_rgba
    bra op_done_ok

cvt_bgra_to_rgba:
    ; [B,G,R,A] → [R,G,B,A]: swap bytes 0,2
    load.b r7, 0(r22)                    ; B
    load.b r8, 1(r22)                    ; G
    load.b r9, 2(r22)                    ; R
    load.b r10, 3(r22)                   ; A
    store.b r9, 0(r23)                   ; R
    store.b r8, 1(r23)                   ; G
    store.b r7, 2(r23)                   ; B
    store.b r10, 3(r23)                  ; A
    add.l r22, r22, #4
    add.l r23, r23, #4
    sub.l r24, r24, #1
    bne r24, r0, cvt_bgra_to_rgba
    bra op_done_ok

cvt_bgr24_to_rgba:
    ; [B,G,R] → [R,G,B,0xFF]
    load.b r7, 0(r22)                    ; B
    load.b r8, 1(r22)                    ; G
    load.b r9, 2(r22)                    ; R
    store.b r9, 0(r23)                   ; R
    store.b r8, 1(r23)                   ; G
    store.b r7, 2(r23)                   ; B
    move.l r10, #255
    store.b r10, 3(r23)                  ; A = 0xFF
    add.l r22, r22, #3
    add.l r23, r23, #4
    sub.l r24, r24, #1
    bne r24, r0, cvt_bgr24_to_rgba
    bra op_done_ok

cvt_unsupported:
    ; Unsupported format pair — return error
    move.l r7, #STATUS_ERROR
    store.l r7, RESP_STATUS(r20)
    store.l r0, RESP_RESULT(r20)
    store.l r0, RESP_LEN(r20)
    bra advance_tail

; ── BLIT_ALPHA ─────────────────────────────────────────────────────
; Alpha template blend: per-pixel alpha modulates destination RGBA32.
; reqPtr = src (8-bit alpha template, one byte per pixel)
; reqLen = width|(height<<16)  — width in PIXELS
; respPtr = dst (RGBA32 framebuffer)
; respCap = srcMod|(dstStride<<16)
;
; For each pixel: dst_channel = dst_channel * alpha / 255
op_blit_alpha:
    ; Source-over alpha compositing: dst = fg * alpha/255 + dst * (255-alpha)/255
    ; r12=src (8-bit alpha mask), r13=width|(height<<16)
    ; r14=dst (RGBA32), r15=srcMod|(dstStride<<16), r29=fg color (RGBA32 packed)

    ; Extract width/height from reqLen
    and.l r16, r13, #0xFFFF              ; width (pixels)
    lsr.l r17, r13, #16                  ; height
    beq r17, r0, op_done_ok
    beq r16, r0, op_done_ok

    ; Extract strides from respCap
    and.l r18, r15, #0xFFFF              ; srcMod (bytes per alpha row)
    lsr.l r19, r15, #16                  ; dstStride (bytes per RGBA row)

    ; Extract FG color channels once (RGBA32: byte 0=R, 1=G, 2=B, 3=A)
    and.l r26, r29, #0xFF               ; fg_R
    lsr.l r6, r29, #8
    and.l r27, r6, #0xFF                 ; fg_G
    lsr.l r6, r29, #16
    and.l r28, r6, #0xFF                 ; fg_B

    ; Row pointers
    move.l r21, r12                       ; src (alpha) cursor
    move.l r22, r14                       ; dst (RGBA) cursor

alpha_row_loop:
    move.l r23, r16                       ; pixels remaining in row
    move.l r24, r21                       ; row alpha ptr
    move.l r25, r22                       ; row RGBA ptr

alpha_col_loop:
    ; Read alpha byte
    load.b r7, 0(r24)

    ; Fast paths (source-over semantics):
    ;   alpha=0   → fully transparent, dst unchanged (skip)
    ;   alpha=255 → fully opaque, dst = fg color
    move.l r8, #255
    beq r7, r0, alpha_skip                ; alpha=0 → keep dst unchanged
    beq r7, r8, alpha_opaque              ; alpha=255 → replace dst with fg

    ; General case: dst_ch = fg_ch * alpha / 255 + dst_ch * (255-alpha) / 255
    sub.l r9, r8, r7                     ; inv_alpha = 255 - alpha

    ; R channel
    load.b r10, 0(r25)                   ; dst_R
    mulu.l r8, r26, r7                   ; fg_R * alpha
    mulu.l r10, r10, r9                  ; dst_R * inv_alpha
    add.l r8, r8, r10
    divu.l r8, r8, #255
    store.b r8, 0(r25)

    ; G channel
    load.b r10, 1(r25)                   ; dst_G
    mulu.l r8, r27, r7                   ; fg_G * alpha
    mulu.l r10, r10, r9                  ; dst_G * inv_alpha
    add.l r8, r8, r10
    divu.l r8, r8, #255
    store.b r8, 1(r25)

    ; B channel
    load.b r10, 2(r25)                   ; dst_B
    mulu.l r8, r28, r7                   ; fg_B * alpha
    mulu.l r10, r10, r9                  ; dst_B * inv_alpha
    add.l r8, r8, r10
    divu.l r8, r8, #255
    store.b r8, 2(r25)

    ; A channel: set fully opaque (text rendering always produces opaque result)
    store.b r0, 3(r25)
    move.l r8, #255
    store.b r8, 3(r25)
    bra alpha_next

alpha_opaque:
    ; alpha=255 → replace dst with fg color entirely
    store.b r26, 0(r25)                  ; fg_R
    store.b r27, 1(r25)                  ; fg_G
    store.b r28, 2(r25)                  ; fg_B
    move.l r8, #255
    store.b r8, 3(r25)                   ; A=255

alpha_next:
alpha_skip:
    add.l r24, r24, #1                   ; next alpha byte
    add.l r25, r25, #4                   ; next RGBA pixel
    sub.l r23, r23, #1
    bne r23, r0, alpha_col_loop

    ; Advance to next row
    add.l r21, r21, r18                   ; src += srcMod
    add.l r22, r22, r19                   ; dst += dstStride
    sub.l r17, r17, #1
    bne r17, r0, alpha_row_loop
    bra op_done_ok

; ── FILL_RECT ──────────────────────────────────────────────────────
; reqPtr = color (packed), reqLen = width|(height<<16)
; respPtr = dst, respCap = stride
op_fill_rect:
    ; Extract width/height
    and.l r16, r13, #0xFFFF              ; width (in bytes)
    lsr.l r17, r13, #16                  ; height
    beq r17, r0, op_done_ok
    beq r16, r0, op_done_ok

    move.l r21, r14                       ; dst cursor
    ; r12 = color value, r15 = stride

    ; Check if width is LONG-aligned for fast fill
    move.l r7, #3
    and.l r8, r16, r7
    bne r8, r0, fill_byte_rows

    ; LONG-aligned fast fill
    lsr.l r18, r16, #2                    ; LONGs per row

fill_long_row:
    move.l r22, r21                       ; row cursor
    move.l r23, r18                       ; count

fill_long_col:
    store.l r12, 0(r22)
    add.l r22, r22, #4
    sub.l r23, r23, #1
    bne r23, r0, fill_long_col

    add.l r21, r21, r15                   ; dst += stride
    sub.l r17, r17, #1
    bne r17, r0, fill_long_row
    bra op_done_ok

fill_byte_rows:
    ; Byte-by-byte fill (handles any width)
    and.l r7, r12, #0xFF                  ; extract byte value

fill_byte_row:
    move.l r22, r21
    move.l r23, r16

fill_byte_col:
    store.b r7, 0(r22)
    add.l r22, r22, #1
    sub.l r23, r23, #1
    bne r23, r0, fill_byte_col

    add.l r21, r21, r15
    sub.l r17, r17, #1
    bne r17, r0, fill_byte_row
    bra op_done_ok

; ── AREA_FILL ──────────────────────────────────────────────────────
; Scanline polygon fill using even-odd rule.
; reqPtr = vertex table (WORD pairs: x0,y0,x1,y1,...), reqLen = vertex count
; respPtr = bitmap base, respCap = stride (bytes per row)
op_area_fill:
    ; r12 = vertex table ptr, r13 = vertex count, r14 = bitmap, r15 = stride
    move.l r7, #3
    blt r13, r7, op_done_ok               ; need at least 3 vertices

    ; First pass: compute bounding box (minY..maxY)
    load.w r16, 2(r12)                    ; minY = vtx[0].y (offset +2)
    move.l r17, r16                       ; maxY = minY
    move.l r18, #1                        ; i = 1
    move.l r19, r12
    add.l r19, r19, #4                    ; ptr to vtx[1]

af_bbox_loop:
    bge r18, r13, af_bbox_done
    load.w r7, 2(r19)                    ; vtx[i].y
    ; Sign-extend WORD to LONG
    lsl.l r7, r7, #16
    asr.l r7, r7, #16
    ; Update minY/maxY
    bge r7, r16, af_not_miny
    move.l r16, r7
af_not_miny:
    blt r7, r17, af_not_maxy
    move.l r17, r7
af_not_maxy:
    add.l r19, r19, #4
    add.l r18, r18, #1
    bra af_bbox_loop

af_bbox_done:
    ; Sign-extend initial minY/maxY
    lsl.l r16, r16, #16
    asr.l r16, r16, #16
    lsl.l r17, r17, #16
    asr.l r17, r17, #16

    ; Scanline loop: for y = minY to maxY
    move.l r21, r16                       ; y = minY

af_scan_loop:
    blt r17, r21, op_done_ok              ; y > maxY → done

    ; Find edge intersections for this scanline
    ; Use a small stack area for intersections (max 32)
    ; We'll use registers r23..r28 for the first few, then spill to memory
    ; Simple approach: just find pairs and fill directly
    ; Count intersections first
    move.l r22, r0                        ; nints = 0
    move.l r18, r0                        ; j = 0
    ; Intersection X values stored at a temp area (use stack-like region at top of worker memory)
    la r24, WORKER_IE64_BASE
    move.l r7, #0x7F000
    add.l r24, r24, r7                    ; temp buffer at worker+0x7F000

af_edge_loop:
    bge r18, r13, af_edges_done
    ; j2 = (j + 1) % count
    add.l r19, r18, #1
    ; if j2 >= count, j2 = 0
    blt r19, r13, af_j2_ok
    move.l r19, r0
af_j2_ok:
    ; Load vtx[j] and vtx[j2]
    mulu.l r7, r18, #4                   ; j * 4
    add.l r25, r12, r7                   ; &vtx[j]
    mulu.l r7, r19, #4                   ; j2 * 4
    add.l r26, r12, r7                   ; &vtx[j2]

    load.w r27, 2(r25)                   ; y1 = vtx[j].y
    lsl.l r27, r27, #16
    asr.l r27, r27, #16
    load.w r28, 2(r26)                   ; y2 = vtx[j2].y
    lsl.l r28, r28, #16
    asr.l r28, r28, #16

    ; Check if edge crosses scanline y
    ; (y1 <= y && y2 > y) || (y2 <= y && y1 > y)
    blt r21, r27, af_check_alt           ; y < y1 → check alt
    blt r21, r28, af_crosses             ; y1 <= y < y2 → crosses
    bra af_check_alt2
af_check_alt:
    blt r21, r28, af_no_cross            ; y < both → no cross
    bra af_crosses                        ; y2 <= y < y1 → crosses
af_check_alt2:
    blt r28, r21, af_no_cross            ; both <= y, neither straddles
    ; y1 <= y, y2 = y → horizontal edge, skip
    beq r28, r21, af_no_cross

af_crosses:
    ; Compute intersection X: x1 + (y - y1) * (x2 - x1) / (y2 - y1)
    load.w r7, 0(r25)                   ; x1
    lsl.l r7, r7, #16
    asr.l r7, r7, #16
    load.w r8, 0(r26)                   ; x2
    lsl.l r8, r8, #16
    asr.l r8, r8, #16

    sub.l r9, r21, r27                   ; y - y1
    sub.l r23, r8, r7                    ; x2 - x1
    muls.l r9, r9, r23                   ; (y - y1) * (x2 - x1)
    sub.l r23, r28, r27                  ; y2 - y1
    beq r23, r0, af_no_cross             ; horizontal edge (shouldn't happen)
    divs.l r9, r9, r23                   ; / (y2 - y1)
    add.l r9, r9, r7                     ; + x1 = intersection X

    ; Store intersection at temp buffer
    mulu.l r7, r22, #4
    add.l r7, r24, r7
    store.l r9, 0(r7)
    add.l r22, r22, #1

af_no_cross:
    add.l r18, r18, #1
    bra af_edge_loop

af_edges_done:
    ; Sort intersections (insertion sort — usually very few)
    move.l r18, #1                        ; i = 1
af_sort_outer:
    bge r18, r22, af_fill_spans
    mulu.l r7, r18, #4
    add.l r7, r24, r7
    load.l r25, 0(r7)                    ; key = ints[i]
    sub.l r19, r18, #1                   ; k = i - 1

af_sort_inner:
    blt r19, r0, af_sort_insert
    mulu.l r7, r19, #4
    add.l r7, r24, r7
    load.l r26, 0(r7)                    ; ints[k]
    blt r26, r25, af_sort_insert         ; ints[k] <= key → done (signed)
    beq r26, r25, af_sort_insert
    store.l r26, 4(r7)                   ; ints[k+1] = ints[k]
    sub.l r19, r19, #1
    bra af_sort_inner

af_sort_insert:
    add.l r19, r19, #1
    mulu.l r7, r19, #4
    add.l r7, r24, r7
    store.l r25, 0(r7)                   ; ints[k+1] = key
    add.l r18, r18, #1
    bra af_sort_outer

af_fill_spans:
    ; Fill horizontal spans in pairs (even-odd rule)
    move.l r18, r0                        ; j = 0
af_span_loop:
    add.l r7, r18, #1
    bge r7, r22, af_scan_next            ; j+1 >= nints → done

    mulu.l r7, r18, #4
    add.l r7, r24, r7
    load.l r25, 0(r7)                    ; x_start = ints[j]
    load.l r26, 4(r7)                    ; x_end = ints[j+1]

    ; Fill pixels from x_start to x_end in bitmap at row y
    ; dst = bitmap + y * stride + x_start
    mulu.l r7, r21, r15                  ; y * stride
    add.l r7, r14, r7                    ; bitmap + y * stride
    add.l r27, r7, r25                   ; + x_start = row start

    sub.l r28, r26, r25                  ; span width
    add.l r28, r28, #1                   ; inclusive
    beq r28, r0, af_span_next

af_fill_span:
    move.l r7, #0xFF                     ; fill with foreground (0xFF)
    store.b r7, 0(r27)
    add.l r27, r27, #1
    sub.l r28, r28, #1
    bne r28, r0, af_fill_span

af_span_next:
    add.l r18, r18, #2
    bra af_span_loop

af_scan_next:
    add.l r21, r21, #1                    ; y++
    bra af_scan_loop

; ── PIXEL_PROCESS ──────────────────────────────────────────────────
; Per-pixel processing (brighten, darken, tint, negative, greyscale, etc.)
; reqPtr = data (RGBA32 pixel buffer), reqLen = width|(height<<16)
; respPtr = operation (POP_* constant), respCap = stride|(value<<16)
op_pixel_process:
    and.l r16, r13, #0xFFFF              ; width (pixels)
    lsr.l r17, r13, #16                  ; height
    beq r16, r0, op_done_ok
    beq r17, r0, op_done_ok

    ; r14 = operation, respCap has stride and value
    and.l r18, r15, #0xFFFF              ; stride
    lsr.l r19, r15, #16                  ; value (0-255)

    move.l r21, r12                       ; data cursor

    ; Dispatch on sub-operation
    move.l r7, #POP_BRIGHTEN
    beq r14, r7, pp_brighten
    move.l r7, #POP_DARKEN
    beq r14, r7, pp_darken
    move.l r7, #POP_SETALPHA
    beq r14, r7, pp_setalpha
    move.l r7, #POP_COLOR2GREY
    beq r14, r7, pp_greyscale
    move.l r7, #POP_NEGATIVE
    beq r14, r7, pp_negative
    ; Unsupported sub-op → error
    bra op_error

; ── BRIGHTEN ──
pp_brighten:
    move.l r22, r17                       ; row count
pp_bright_row:
    move.l r23, r16                       ; pixel count
    move.l r24, r21                       ; row ptr
pp_bright_pixel:
    ; For each RGB channel: channel = min(channel + value, 255)
    load.b r7, 0(r24)                    ; R
    add.l r7, r7, r19
    move.l r8, #255
    blt r7, r8, pp_br_r_ok
    move.l r7, #255
pp_br_r_ok:
    store.b r7, 0(r24)
    load.b r7, 1(r24)                    ; G
    add.l r7, r7, r19
    blt r7, r8, pp_br_g_ok
    move.l r7, #255
pp_br_g_ok:
    store.b r7, 1(r24)
    load.b r7, 2(r24)                    ; B
    add.l r7, r7, r19
    blt r7, r8, pp_br_b_ok
    move.l r7, #255
pp_br_b_ok:
    store.b r7, 2(r24)
    add.l r24, r24, #4
    sub.l r23, r23, #1
    bne r23, r0, pp_bright_pixel
    add.l r21, r21, r18                   ; next row
    sub.l r22, r22, #1
    bne r22, r0, pp_bright_row
    bra op_done_ok

; ── DARKEN ──
pp_darken:
    move.l r22, r17
pp_dark_row:
    move.l r23, r16
    move.l r24, r21
pp_dark_pixel:
    ; For each RGB channel: channel = max(channel - value, 0)
    load.b r7, 0(r24)                    ; R
    sub.l r7, r7, r19
    bge r7, r0, pp_dk_r_ok
    move.l r7, r0
pp_dk_r_ok:
    store.b r7, 0(r24)
    load.b r7, 1(r24)                    ; G
    sub.l r7, r7, r19
    bge r7, r0, pp_dk_g_ok
    move.l r7, r0
pp_dk_g_ok:
    store.b r7, 1(r24)
    load.b r7, 2(r24)                    ; B
    sub.l r7, r7, r19
    bge r7, r0, pp_dk_b_ok
    move.l r7, r0
pp_dk_b_ok:
    store.b r7, 2(r24)
    add.l r24, r24, #4
    sub.l r23, r23, #1
    bne r23, r0, pp_dark_pixel
    add.l r21, r21, r18
    sub.l r22, r22, #1
    bne r22, r0, pp_dark_row
    bra op_done_ok

; ── SETALPHA ──
pp_setalpha:
    move.l r22, r17
pp_sa_row:
    move.l r23, r16
    move.l r24, r21
pp_sa_pixel:
    store.b r19, 3(r24)                  ; set alpha channel
    add.l r24, r24, #4
    sub.l r23, r23, #1
    bne r23, r0, pp_sa_pixel
    add.l r21, r21, r18
    sub.l r22, r22, #1
    bne r22, r0, pp_sa_row
    bra op_done_ok

; ── GREYSCALE ──
pp_greyscale:
    move.l r22, r17
pp_grey_row:
    move.l r23, r16
    move.l r24, r21
pp_grey_pixel:
    ; grey = (R*77 + G*150 + B*29) >> 8 (ITU-R BT.601 weights)
    load.b r7, 0(r24)                    ; R
    mulu.l r7, r7, #77
    load.b r8, 1(r24)                    ; G
    mulu.l r8, r8, #150
    add.l r7, r7, r8
    load.b r8, 2(r24)                    ; B
    mulu.l r8, r8, #29
    add.l r7, r7, r8
    lsr.l r7, r7, #8                     ; >> 8
    store.b r7, 0(r24)                   ; R = grey
    store.b r7, 1(r24)                   ; G = grey
    store.b r7, 2(r24)                   ; B = grey
    add.l r24, r24, #4
    sub.l r23, r23, #1
    bne r23, r0, pp_grey_pixel
    add.l r21, r21, r18
    sub.l r22, r22, #1
    bne r22, r0, pp_grey_row
    bra op_done_ok

; ── NEGATIVE ──
pp_negative:
    move.l r22, r17
pp_neg_row:
    move.l r23, r16
    move.l r24, r21
pp_neg_pixel:
    ; Invert RGB: channel = 255 - channel
    load.b r7, 0(r24)
    move.l r8, #255
    sub.l r7, r8, r7
    store.b r7, 0(r24)
    load.b r7, 1(r24)
    sub.l r7, r8, r7
    store.b r7, 1(r24)
    load.b r7, 2(r24)
    sub.l r7, r8, r7
    store.b r7, 2(r24)
    add.l r24, r24, #4
    sub.l r23, r23, #1
    bne r23, r0, pp_neg_pixel
    add.l r21, r21, r18
    sub.l r22, r22, #1
    bne r22, r0, pp_neg_row
    bra op_done_ok

; ============================================================================
; Operation handlers — Audio primitives
; ============================================================================

; ── AUDIO_MIX ──────────────────────────────────────────────────────
; Mix N channels into output buffer with volume scaling.
; reqPtr = channels array (array of N pointers to int16 sample buffers)
; reqLen = numChannels|(numSamples<<16)
; respPtr = dst (int16 output buffer), flags = volumes array (N uint16 volumes, 0-256)
op_audio_mix:
    and.l r16, r13, #0xFFFF              ; numChannels
    lsr.l r17, r13, #16                  ; numSamples
    beq r16, r0, op_done_ok
    beq r17, r0, op_done_ok

    ; r12 = channels array, r14 = dst, r29 = volumes array
    ; For each sample: sum all channels with volume, clamp to int16
    move.l r21, r0                        ; sample index

amix_sample_loop:
    bge r21, r17, op_done_ok
    move.l r22, r0                        ; accumulator = 0
    move.l r23, r0                        ; channel index

amix_channel_loop:
    bge r23, r16, amix_write_sample

    ; Load channel buffer pointer: channels[ch]
    mulu.l r7, r23, #4
    add.l r7, r12, r7
    load.l r24, 0(r7)                    ; channel buffer ptr

    ; Load sample: channelBuf[sampleIdx] (int16)
    mulu.l r7, r21, #2
    add.l r7, r24, r7
    load.w r25, 0(r7)                    ; sample (int16)
    ; Sign-extend
    lsl.l r25, r25, #16
    asr.l r25, r25, #16

    ; Load volume: volumes[ch] (uint16, 0-256 where 256=full)
    mulu.l r7, r23, #2
    add.l r7, r29, r7
    load.w r26, 0(r7)                    ; volume
    and.l r26, r26, #0xFFFF

    ; Scale: sample = sample * volume / 256
    muls.l r25, r25, r26
    asr.l r25, r25, #8                   ; / 256

    ; Accumulate
    add.l r22, r22, r25

    add.l r23, r23, #1
    bra amix_channel_loop

amix_write_sample:
    ; Clamp to int16 range (-32768..32767)
    move.l r7, #32767
    blt r22, r7, amix_clamp_lo
    move.l r22, r7
    bra amix_store
amix_clamp_lo:
    move.l r7, #0xFFFF8000               ; -32768 as unsigned
    ; Check if accumulator < -32768 (signed comparison)
    ; Use sub+sign check: if acc is very negative
    bge r22, r7, amix_store
    move.l r22, r7

amix_store:
    ; Store int16 to dst
    mulu.l r7, r21, #2
    add.l r7, r14, r7
    store.w r22, 0(r7)

    add.l r21, r21, #1
    bra amix_sample_loop

; ── AUDIO_RESAMPLE ─────────────────────────────────────────────────
; Linear interpolation sample rate conversion.
; reqPtr = src (int16 samples), reqLen = numSrcSamples
; respPtr = dst (int16 output), respCap = srcRate|(dstRate<<16)
op_audio_resample:
    ; r12 = src, r13 = numSrcSamples, r14 = dst
    beq r13, r0, op_done_ok

    and.l r16, r15, #0xFFFF              ; srcRate
    lsr.l r17, r15, #16                  ; dstRate
    beq r16, r0, op_done_ok
    beq r17, r0, op_done_ok

    ; Compute numDstSamples = numSrcSamples * dstRate / srcRate
    mulu.l r18, r13, r17                 ; numSrc * dstRate
    divu.l r18, r18, r16                 ; / srcRate = numDst

    ; For each output sample: position = i * srcRate / dstRate
    ; Interpolate between src[pos] and src[pos+1]
    move.l r21, r0                        ; dst index

resamp_loop:
    bge r21, r18, op_done_ok

    ; srcPos = dstIdx * srcRate (fixed point)
    ; srcIdx = srcPos / dstRate
    ; frac = srcPos % dstRate
    mulu.l r22, r21, r16                 ; dstIdx * srcRate
    divu.l r23, r22, r17                 ; srcIdx = / dstRate
    mulu.l r7, r23, r17                  ; srcIdx * dstRate
    sub.l r24, r22, r7                   ; frac = remainder

    ; Clamp srcIdx to valid range
    sub.l r7, r13, #1
    blt r23, r7, resamp_in_range
    move.l r23, r7
    move.l r24, r0                        ; frac = 0 at boundary
resamp_in_range:

    ; Load src[srcIdx] and src[srcIdx+1]
    mulu.l r7, r23, #2
    add.l r7, r12, r7
    load.w r25, 0(r7)                    ; s0
    load.w r26, 2(r7)                    ; s1
    ; Sign-extend
    lsl.l r25, r25, #16
    asr.l r25, r25, #16
    lsl.l r26, r26, #16
    asr.l r26, r26, #16

    ; Interpolate: result = s0 + (s1 - s0) * frac / dstRate
    sub.l r7, r26, r25                   ; s1 - s0
    muls.l r7, r7, r24                   ; * frac
    divs.l r7, r7, r17                   ; / dstRate
    add.l r7, r7, r25                    ; + s0

    ; Store result
    mulu.l r8, r21, #2
    add.l r8, r14, r8
    store.w r7, 0(r8)

    add.l r21, r21, #1
    bra resamp_loop

; ── AUDIO_DECODE ───────────────────────────────────────────────────
; Decode IMA-ADPCM compressed audio to int16 PCM.
; reqPtr = src (compressed data), reqLen = srcLen (bytes)
; respPtr = dst (int16 output buffer), respCap = codec ID
;
; IMA-ADPCM: 4 bits per sample, decodes to 16-bit PCM.
; Step table and index table are embedded in the code.
op_audio_decode:
    beq r13, r0, op_done_ok

    ; Only support IMA-ADPCM (codec=1)
    move.l r7, #CODEC_IMA_ADPCM
    bne r15, r7, op_error

    ; Initialize decoder state
    move.l r16, r0                        ; predictor = 0
    move.l r17, r0                        ; step_index = 0
    move.l r18, r12                       ; src cursor
    move.l r19, r14                       ; dst cursor
    move.l r21, r13                       ; bytes remaining

    ; Initial step size
    move.l r22, #7                        ; step = ima_step_table[0] = 7

adpcm_byte_loop:
    beq r21, r0, op_done_ok
    load.b r23, 0(r18)                   ; load compressed byte (2 nibbles)

    ; Process low nibble
    and.l r24, r23, #0x0F                ; low nibble
    ; Decode nibble: diff = (nibble & 7) * step / 4 + step / 8
    and.l r7, r24, #7
    mulu.l r7, r7, r22                   ; (nibble & 7) * step
    lsr.l r7, r7, #2                     ; / 4
    lsr.l r8, r22, #3                    ; step / 8
    add.l r7, r7, r8                     ; diff = total
    ; If nibble & 8, negate diff
    and.l r8, r24, #8
    beq r8, r0, adpcm_lo_pos
    sub.l r7, r0, r7                     ; diff = -diff
adpcm_lo_pos:
    add.l r16, r16, r7                   ; predictor += diff
    ; Clamp predictor to int16 range
    move.l r7, #32767
    blt r16, r7, adpcm_lo_clamp_lo
    move.l r16, r7
    bra adpcm_lo_store
adpcm_lo_clamp_lo:
    move.l r7, #0xFFFF8000
    bge r16, r7, adpcm_lo_store
    move.l r16, r7
adpcm_lo_store:
    store.w r16, 0(r19)
    add.l r19, r19, #2
    ; Update step index
    ; index_adjust for nibble values: -1,-1,-1,-1, 2, 4, 6, 8
    and.l r7, r24, #7
    move.l r8, #4
    blt r7, r8, adpcm_lo_idx_dec
    ; index += 2 * (nibble & 3) + 2 (approximation: (n-4)*2+2)
    sub.l r7, r7, #4
    lsl.l r7, r7, #1
    add.l r7, r7, #2
    add.l r17, r17, r7
    bra adpcm_lo_idx_clamp
adpcm_lo_idx_dec:
    sub.l r17, r17, #1
adpcm_lo_idx_clamp:
    bge r17, r0, adpcm_lo_idx_max
    move.l r17, r0
adpcm_lo_idx_max:
    move.l r7, #88
    blt r17, r7, adpcm_hi_nibble
    move.l r17, #88

adpcm_hi_nibble:
    ; Recompute step from index (approximate: step = 7 * 1.1^index)
    ; Simplified: step = 7 + index * index / 2 (quadratic approximation)
    mulu.l r22, r17, r17                 ; index^2
    lsr.l r22, r22, #1                   ; / 2
    add.l r22, r22, #7                   ; + 7

    ; Process high nibble
    lsr.l r24, r23, #4                   ; high nibble
    and.l r24, r24, #0x0F
    and.l r7, r24, #7
    mulu.l r7, r7, r22
    lsr.l r7, r7, #2
    lsr.l r8, r22, #3
    add.l r7, r7, r8
    and.l r8, r24, #8
    beq r8, r0, adpcm_hi_pos
    sub.l r7, r0, r7
adpcm_hi_pos:
    add.l r16, r16, r7
    move.l r7, #32767
    blt r16, r7, adpcm_hi_clamp_lo
    move.l r16, r7
    bra adpcm_hi_store
adpcm_hi_clamp_lo:
    move.l r7, #0xFFFF8000
    bge r16, r7, adpcm_hi_store
    move.l r16, r7
adpcm_hi_store:
    store.w r16, 0(r19)
    add.l r19, r19, #2
    ; Update step index for high nibble
    and.l r7, r24, #7
    move.l r8, #4
    blt r7, r8, adpcm_hi_idx_dec
    sub.l r7, r7, #4
    lsl.l r7, r7, #1
    add.l r7, r7, #2
    add.l r17, r17, r7
    bra adpcm_hi_idx_clamp
adpcm_hi_idx_dec:
    sub.l r17, r17, #1
adpcm_hi_idx_clamp:
    bge r17, r0, adpcm_hi_idx_max
    move.l r17, r0
adpcm_hi_idx_max:
    move.l r7, #88
    blt r17, r7, adpcm_step_update
    move.l r17, #88

adpcm_step_update:
    mulu.l r22, r17, r17
    lsr.l r22, r22, #1
    add.l r22, r22, #7

    add.l r18, r18, #1                   ; next src byte
    sub.l r21, r21, #1
    bra adpcm_byte_loop

; ============================================================================
; Operation handlers — Math primitives
; ============================================================================

; ── FP_BATCH ───────────────────────────────────────────────────────
; Batch IEEE 754 single-precision FP operations.
; reqPtr = src (array of FP op descriptors, 12 bytes each)
;   Descriptor: [opcode:u32] [operand1:f32] [operand2:f32]
; reqLen = count of operations
; respPtr = dst (array of f32 results)
op_fp_batch:
    beq r13, r0, op_done_ok

    move.l r16, r12                       ; src descriptor cursor
    move.l r17, r14                       ; dst result cursor
    move.l r18, r13                       ; count

fpb_loop:
    beq r18, r0, op_done_ok
    load.l r21, 0(r16)                   ; opcode
    fload f0, 4(r16)                     ; operand1
    fload f1, 8(r16)                     ; operand2

    ; Dispatch on FP sub-operation
    beq r21, r0, fpb_add
    move.l r7, #FP_OP_SUB
    beq r21, r7, fpb_sub
    move.l r7, #FP_OP_MUL
    beq r21, r7, fpb_mul
    move.l r7, #FP_OP_DIV
    beq r21, r7, fpb_div
    move.l r7, #FP_OP_SQRT
    beq r21, r7, fpb_sqrt
    move.l r7, #FP_OP_SIN
    beq r21, r7, fpb_sin
    move.l r7, #FP_OP_COS
    beq r21, r7, fpb_cos
    move.l r7, #FP_OP_TAN
    beq r21, r7, fpb_tan
    move.l r7, #FP_OP_ATAN
    beq r21, r7, fpb_atan
    move.l r7, #FP_OP_LOG
    beq r21, r7, fpb_log
    move.l r7, #FP_OP_EXP
    beq r21, r7, fpb_exp
    move.l r7, #FP_OP_POW
    beq r21, r7, fpb_pow
    move.l r7, #FP_OP_ABS
    beq r21, r7, fpb_abs
    move.l r7, #FP_OP_NEG
    beq r21, r7, fpb_neg
    ; Unknown FP op → store 0
    fstore f0, 0(r17)
    bra fpb_next

fpb_add:
    fadd f2, f0, f1
    fstore f2, 0(r17)
    bra fpb_next
fpb_sub:
    fsub f2, f0, f1
    fstore f2, 0(r17)
    bra fpb_next
fpb_mul:
    fmul f2, f0, f1
    fstore f2, 0(r17)
    bra fpb_next
fpb_div:
    fdiv f2, f0, f1
    fstore f2, 0(r17)
    bra fpb_next
fpb_sqrt:
    fsqrt f2, f0
    fstore f2, 0(r17)
    bra fpb_next
fpb_sin:
    fsin f2, f0
    fstore f2, 0(r17)
    bra fpb_next
fpb_cos:
    fcos f2, f0
    fstore f2, 0(r17)
    bra fpb_next
fpb_tan:
    ftan f2, f0
    fstore f2, 0(r17)
    bra fpb_next
fpb_atan:
    fatan f2, f0
    fstore f2, 0(r17)
    bra fpb_next
fpb_log:
    flog f2, f0
    fstore f2, 0(r17)
    bra fpb_next
fpb_exp:
    fexp f2, f0
    fstore f2, 0(r17)
    bra fpb_next
fpb_pow:
    fpow f2, f0, f1
    fstore f2, 0(r17)
    bra fpb_next
fpb_abs:
    fabs f2, f0
    fstore f2, 0(r17)
    bra fpb_next
fpb_neg:
    fneg f2, f0
    fstore f2, 0(r17)

fpb_next:
    add.l r16, r16, #12                  ; next descriptor (12 bytes)
    add.l r17, r17, #4                   ; next result (4 bytes)
    sub.l r18, r18, #1
    bra fpb_loop

; ── MATRIX_MUL ─────────────────────────────────────────────────────
; 4×4 float32 matrix multiply: C = A × B
; reqPtr = matA (16 × f32 = 64 bytes, row-major)
; respPtr = matB (16 × f32 = 64 bytes, row-major)
; flags = dstPtr (output matrix, 64 bytes)
op_matrix_mul:
    ; r12 = matA, r14 = matB, r29 = dstC
    ; C[i][j] = sum(A[i][k] * B[k][j], k=0..3)
    move.l r16, r0                        ; i = 0

matmul_i:
    move.l r7, #4
    bge r16, r7, op_done_ok
    move.l r17, r0                        ; j = 0

matmul_j:
    move.l r7, #4
    bge r17, r7, matmul_next_i

    ; Compute dot product A[i][0..3] * B[0..3][j]
    ; Use FP accumulator in f4
    move.l r7, #0
    fmovi f4, r7                         ; acc = 0.0

    move.l r18, r0                        ; k = 0
matmul_k:
    move.l r7, #4
    bge r18, r7, matmul_store

    ; A[i][k] offset = (i*4 + k) * 4
    mulu.l r7, r16, #4
    add.l r7, r7, r18                    ; i*4 + k
    lsl.l r7, r7, #2                     ; * 4 bytes
    add.l r7, r12, r7
    fload f0, 0(r7)                      ; A[i][k]

    ; B[k][j] offset = (k*4 + j) * 4
    mulu.l r7, r18, #4
    add.l r7, r7, r17                    ; k*4 + j
    lsl.l r7, r7, #2
    add.l r7, r14, r7
    fload f1, 0(r7)                      ; B[k][j]

    fmul f2, f0, f1                      ; A[i][k] * B[k][j]
    fadd f4, f4, f2                      ; acc += product

    add.l r18, r18, #1
    bra matmul_k

matmul_store:
    ; C[i][j] offset = (i*4 + j) * 4
    mulu.l r7, r16, #4
    add.l r7, r7, r17
    lsl.l r7, r7, #2
    add.l r7, r29, r7
    fstore f4, 0(r7)                     ; C[i][j] = acc

    add.l r17, r17, #1
    bra matmul_j

matmul_next_i:
    add.l r16, r16, #1
    bra matmul_i

; ── CRC32 ──────────────────────────────────────────────────────────
; Compute CRC32 of data buffer.
; reqPtr = data pointer, reqLen = data length
; Result written to response descriptor's resultCode field.
op_crc32:
    beq r13, r0, op_done_ok_crc

    ; CRC32 with polynomial 0xEDB88320 (bit-reversed 0x04C11DB7)
    move.l r16, r29                       ; crc = initial (from flags/TIMEOUT)
    move.l r17, r12                       ; data cursor
    move.l r18, r13                       ; remaining bytes

crc_byte_loop:
    beq r18, r0, crc_done
    load.b r7, 0(r17)
    eor.l r16, r16, r7                   ; crc ^= byte

    ; Process 8 bits
    move.l r19, #8
crc_bit_loop:
    beq r19, r0, crc_next_byte
    and.l r7, r16, #1                    ; crc & 1
    lsr.l r16, r16, #1                   ; crc >>= 1
    beq r7, r0, crc_bit_skip
    move.l r8, #0xEDB88320
    eor.l r16, r16, r8                   ; crc ^= polynomial
crc_bit_skip:
    sub.l r19, r19, #1
    bra crc_bit_loop

crc_next_byte:
    add.l r17, r17, #1
    sub.l r18, r18, #1
    bra crc_byte_loop

crc_done:
    ; Final XOR
    move.l r7, #0xFFFFFFFF
    eor.l r16, r16, r7                   ; crc = ~crc

    ; Write CRC to respPtr (r14) so caller can read it
    bne r14, r0, crc_write_resp
    bra crc_write_desc
crc_write_resp:
    store.l r16, 0(r14)
crc_write_desc:
    ; Write CRC to response resultCode field
    move.l r7, #STATUS_OK
    store.l r7, RESP_STATUS(r20)
    store.l r16, RESP_RESULT(r20)
    move.l r7, #4
    store.l r7, RESP_LEN(r20)
    bra advance_tail

op_done_ok_crc:
    ; Zero-length data: CRC = initial (unmodified)
    ; Write to respPtr if provided
    bne r14, r0, crc_zero_write_resp
    bra crc_zero_write_desc
crc_zero_write_resp:
    store.l r29, 0(r14)
crc_zero_write_desc:
    move.l r7, #STATUS_OK
    store.l r7, RESP_STATUS(r20)
    store.l r29, RESP_RESULT(r20)
    store.l r0, RESP_LEN(r20)
    bra advance_tail

; ============================================================================
; Operation handlers — Rendering primitives
; ============================================================================

; ── GRADIENT_FILL ──────────────────────────────────────────────────
; Fill rectangle with linear gradient (vertical or horizontal).
; reqPtr = startColor (RGBA32), reqLen = width|(height<<16)
; respPtr = dst, respCap = stride (low 16) | (direction << 16)
;   direction: 0 = vertical (interpolate per row)
;              1 = horizontal (interpolate per column)
; flags = endColor (RGBA32)
op_gradient_fill:
    and.l r16, r13, #0xFFFF              ; width
    lsr.l r17, r13, #16                  ; height
    beq r16, r0, op_done_ok
    beq r17, r0, op_done_ok

    ; Extract stride and direction from respCap
    and.l r6, r15, #0xFFFF               ; stride (low 16)
    lsr.l r8, r15, #16                   ; direction (0=vert, 1=horiz)

    ; r12 = startColor, r29 = endColor, r14 = dst, r6 = stride
    ; Extract RGBA channels from startColor
    and.l r21, r12, #0xFF                ; startR
    lsr.l r7, r12, #8
    and.l r22, r7, #0xFF                 ; startG
    lsr.l r7, r12, #16
    and.l r23, r7, #0xFF                 ; startB
    lsr.l r24, r12, #24                  ; startA

    ; Extract RGBA channels from endColor
    and.l r25, r29, #0xFF                ; endR
    lsr.l r7, r29, #8
    and.l r26, r7, #0xFF                 ; endG
    lsr.l r7, r29, #16
    and.l r27, r7, #0xFF                 ; endB
    lsr.l r28, r29, #24                  ; endA

    ; Check direction
    bne r8, r0, grad_horizontal

    ; ── Vertical gradient: interpolate per row ──
    move.l r18, r14                       ; row cursor
    move.l r19, r0                        ; y = 0
    sub.l r9, r17, #1                    ; height - 1
    beq r9, r0, grad_single_row          ; only 1 row

grad_row_loop:
    bge r19, r17, op_done_ok

    ; Compute interpolated color for this row
    ; R = startR + (endR - startR) * y / (h-1)
    sub.l r7, r25, r21                   ; endR - startR
    muls.l r7, r7, r19                   ; * y
    divs.l r7, r7, r9                    ; / (h-1)
    add.l r7, r7, r21                    ; + startR
    and.l r7, r7, #0xFF

    sub.l r8, r26, r22
    muls.l r8, r8, r19
    divs.l r8, r8, r9
    add.l r8, r8, r22
    and.l r8, r8, #0xFF

    sub.l r10, r27, r23
    muls.l r10, r10, r19
    divs.l r10, r10, r9
    add.l r10, r10, r23
    and.l r10, r10, #0xFF

    sub.l r11, r28, r24
    muls.l r11, r11, r19
    divs.l r11, r11, r9
    add.l r11, r11, r24
    and.l r11, r11, #0xFF

    ; Pack RGBA into u32: R | (G<<8) | (B<<16) | (A<<24)
    lsl.l r8, r8, #8
    or.l r7, r7, r8
    lsl.l r10, r10, #16
    or.l r7, r7, r10
    lsl.l r11, r11, #24
    or.l r7, r7, r11

    ; Fill row with this color
    move.l r24, r18                       ; pixel cursor (reuse r24 here)
    ; Note: we just overwrote r24 (startA). That's ok since we use
    ; the packed computation above which already captured the value.
    move.l r10, r16                       ; pixel count

grad_fill_row:
    store.l r7, 0(r24)
    add.l r24, r24, #4
    sub.l r10, r10, #1
    bne r10, r0, grad_fill_row

    add.l r18, r18, r6                   ; next row (using stride from r6)
    add.l r19, r19, #1

    ; Reload startA since we clobbered r24
    lsr.l r24, r12, #24
    bra grad_row_loop

grad_single_row:
    ; Single row: just fill with start color
    move.l r24, r18
    move.l r10, r16
grad_single_fill:
    store.l r12, 0(r24)
    add.l r24, r24, #4
    sub.l r10, r10, #1
    bne r10, r0, grad_single_fill
    bra op_done_ok

    ; ── Horizontal gradient: interpolate per column ──
    ; Each column x has a single color, fill entire column height
grad_horizontal:
    sub.l r9, r16, #1                    ; width - 1
    beq r9, r0, grad_single_col          ; only 1 column
    move.l r19, r0                        ; x = 0

grad_col_loop:
    bge r19, r16, op_done_ok

    ; Compute interpolated color for this column
    ; R = startR + (endR - startR) * x / (w-1)
    sub.l r7, r25, r21                   ; endR - startR
    muls.l r7, r7, r19                   ; * x
    divs.l r7, r7, r9                    ; / (w-1)
    add.l r7, r7, r21                    ; + startR
    and.l r7, r7, #0xFF

    sub.l r8, r26, r22
    muls.l r8, r8, r19
    divs.l r8, r8, r9
    add.l r8, r8, r22
    and.l r8, r8, #0xFF

    sub.l r10, r27, r23
    muls.l r10, r10, r19
    divs.l r10, r10, r9
    add.l r10, r10, r23
    and.l r10, r10, #0xFF

    sub.l r11, r28, r24
    muls.l r11, r11, r19
    divs.l r11, r11, r9
    add.l r11, r11, r24
    and.l r11, r11, #0xFF

    ; Pack RGBA into u32
    lsl.l r8, r8, #8
    or.l r7, r7, r8
    lsl.l r10, r10, #16
    or.l r7, r7, r10
    lsl.l r11, r11, #24
    or.l r7, r7, r11

    ; Fill column: write this color to dst[y*stride + x*4] for each row y
    ; Column pixel offset = dst + x * 4
    mulu.l r18, r19, #4
    add.l r18, r14, r18                   ; pixel ptr = dst + x*4
    move.l r10, r17                       ; row count = height

grad_fill_col:
    store.l r7, 0(r18)
    add.l r18, r18, r6                   ; next row (advance by stride)
    sub.l r10, r10, #1
    bne r10, r0, grad_fill_col

    add.l r19, r19, #1

    ; Reload startA since we clobbered r24 via r11 computation
    lsr.l r24, r12, #24
    bra grad_col_loop

grad_single_col:
    ; Single column: fill with start color
    move.l r18, r14
    move.l r10, r17
grad_single_col_fill:
    store.l r12, 0(r18)
    add.l r18, r18, r6
    sub.l r10, r10, #1
    bne r10, r0, grad_single_col_fill
    bra op_done_ok

; ── GLYPH_RENDER ───────────────────────────────────────────────────
; Batch render pre-rasterized glyph bitmaps to destination.
; Each glyph descriptor: [dstX:u16, dstY:u16, srcOffset:u32, width:u16, height:u16]
;   = 12 bytes per glyph
; reqPtr = glyph descriptor array, reqLen = glyph count
; respPtr = dst bitmap base, respCap = dstStride
; flags = src bitmap data (all glyph bitmaps concatenated, 1 byte per pixel alpha)
op_glyph_render:
    ; Source-over glyph batch rendering with per-glyph FG color.
    ; r12 = descriptor array, r13 = glyph count
    ; r14 = dst bitmap base, r15 = dstStride, r29 = src glyph data base
    ;
    ; 16-byte descriptor per glyph:
    ;   +0  dstX      (UWORD)
    ;   +2  dstY      (UWORD)
    ;   +4  srcOffset (ULONG, offset into glyph data)
    ;   +8  width     (UWORD)
    ;  +10  height    (UWORD)
    ;  +12  fgColor   (ULONG, RGBA32 packed)
    beq r13, r0, op_done_ok

    move.l r16, r12                       ; descriptor cursor
    move.l r17, r13                       ; glyph count

glyph_loop:
    beq r17, r0, op_done_ok

    ; Load glyph descriptor (16 bytes)
    load.w r21, 0(r16)                   ; dstX
    and.l r21, r21, #0xFFFF
    load.w r22, 2(r16)                   ; dstY
    and.l r22, r22, #0xFFFF
    load.l r23, 4(r16)                   ; srcOffset into glyph data
    load.w r24, 8(r16)                   ; glyph width
    and.l r24, r24, #0xFFFF
    load.w r25, 10(r16)                  ; glyph height
    and.l r25, r25, #0xFFFF
    load.l r28, 12(r16)                  ; fgColor (RGBA32)

    beq r24, r0, glyph_next
    beq r25, r0, glyph_next

    ; Extract FG channels from fgColor
    and.l r6, r28, #0xFF                 ; fg_R
    lsr.l r7, r28, #8
    and.l r9, r7, #0xFF                  ; fg_G
    lsr.l r7, r28, #16
    and.l r10, r7, #0xFF                 ; fg_B

    ; src = glyphData + srcOffset
    add.l r26, r29, r23

    ; dst start = dstBase + dstY * dstStride + dstX * 4 (RGBA32)
    mulu.l r7, r22, r15                  ; dstY * dstStride
    add.l r7, r14, r7                    ; + dstBase
    mulu.l r8, r21, #4                   ; dstX * 4
    add.l r27, r7, r8                    ; dst start

    ; Blit glyph with source-over compositing
    move.l r18, r25                       ; row count

glyph_row:
    move.l r19, r24                       ; col count
    move.l r21, r26                       ; src row (reuse r21)
    move.l r22, r27                       ; dst row (reuse r22)

glyph_col:
    load.b r7, 0(r21)                    ; alpha
    move.l r8, #255
    beq r7, r0, glyph_col_skip           ; alpha=0 → transparent, skip
    beq r7, r8, glyph_col_opaque         ; alpha=255 → fully opaque, replace

    ; Source-over: dst = fg * alpha/255 + dst * (255-alpha)/255
    sub.l r11, r8, r7                    ; inv_alpha = 255 - alpha

    ; R channel
    load.b r8, 0(r22)                    ; dst_R
    mulu.l r20, r6, r7                   ; fg_R * alpha
    mulu.l r8, r8, r11                   ; dst_R * inv_alpha
    add.l r20, r20, r8
    divu.l r20, r20, #255
    store.b r20, 0(r22)

    ; G channel
    load.b r8, 1(r22)                    ; dst_G
    mulu.l r20, r9, r7                   ; fg_G * alpha
    mulu.l r8, r8, r11                   ; dst_G * inv_alpha
    add.l r20, r20, r8
    divu.l r20, r20, #255
    store.b r20, 1(r22)

    ; B channel
    load.b r8, 2(r22)                    ; dst_B
    mulu.l r20, r10, r7                  ; fg_B * alpha
    mulu.l r8, r8, r11                   ; dst_B * inv_alpha
    add.l r20, r20, r8
    divu.l r20, r20, #255
    store.b r20, 2(r22)

    ; A = 255 (opaque)
    move.l r8, #255
    store.b r8, 3(r22)
    bra glyph_col_next

glyph_col_opaque:
    ; alpha=255 → replace with fg color
    store.b r6, 0(r22)                   ; fg_R
    store.b r9, 1(r22)                   ; fg_G
    store.b r10, 2(r22)                  ; fg_B
    move.l r8, #255
    store.b r8, 3(r22)                   ; A=255

glyph_col_next:
glyph_col_skip:
    add.l r21, r21, #1
    add.l r22, r22, #4
    sub.l r19, r19, #1
    bne r19, r0, glyph_col

    add.l r26, r26, r24                   ; src += glyphWidth (1 byte per pixel)
    add.l r27, r27, r15                   ; dst += dstStride
    sub.l r18, r18, #1
    bne r18, r0, glyph_row

glyph_next:
    add.l r16, r16, #16                  ; next descriptor (16 bytes)
    sub.l r17, r17, #1
    bra glyph_loop

; ── SCROLL ─────────────────────────────────────────────────────────
; Scroll a rectangular region of a bitmap.
; reqPtr = bitmap base, reqLen = width|(height<<16)
; respPtr = xMin|(yMin<<16), respCap = dx|(dy<<16)
; flags = stride (bytes per row)
;
; Copies the region shifted by (dx,dy), fills exposed area with 0.
op_scroll:
    and.l r16, r13, #0xFFFF              ; width
    lsr.l r17, r13, #16                  ; height
    beq r16, r0, op_done_ok
    beq r17, r0, op_done_ok

    ; Extract position
    and.l r18, r14, #0xFFFF              ; xMin
    lsr.l r19, r14, #16                  ; yMin

    ; Extract deltas (signed 16-bit packed into u32)
    and.l r21, r15, #0xFFFF              ; dx (as unsigned)
    lsr.l r22, r15, #16                  ; dy (as unsigned)
    ; Sign-extend dx/dy from 16-bit
    lsl.l r21, r21, #16
    asr.l r21, r21, #16
    lsl.l r22, r22, #16
    asr.l r22, r22, #16

    ; r29 = stride, r12 = bitmap base

    ; Compute the copy region (excluding exposed area)
    ; For a scroll by (dx, dy):
    ;   If dx > 0: copy columns [dx..width-1] to [0..width-dx-1]
    ;   If dy > 0: copy rows [dy..height-1] to [0..height-dy-1]
    ; This is equivalent to a BLIT_COPY from the shifted source to dest.

    ; For simplicity, use row-by-row copy in the correct order
    ; to handle the overlap (similar to memmove logic).

    ; Copy width/height after accounting for scroll delta
    ; Absolute dx/dy
    move.l r23, r21
    bge r23, r0, scroll_abs_dx_done
    sub.l r23, r0, r23                    ; abs(dx)
scroll_abs_dx_done:
    move.l r24, r22
    bge r24, r0, scroll_abs_dy_done
    sub.l r24, r0, r24                    ; abs(dy)
scroll_abs_dy_done:

    sub.l r25, r16, r23                  ; copyW = width - abs(dx)
    sub.l r26, r17, r24                  ; copyH = height - abs(dy)

    ; If no area to copy, just fill
    bge r25, r0, scroll_has_copy_w
    move.l r25, r0
scroll_has_copy_w:
    bge r26, r0, scroll_has_copy_h
    move.l r26, r0
scroll_has_copy_h:
    beq r25, r0, scroll_fill_all
    beq r26, r0, scroll_fill_all

    ; Compute src and dst offsets for the copy
    ; srcX = (dx > 0) ? dx : 0; dstX = (dx > 0) ? 0 : -dx
    ; srcY = (dy > 0) ? dy : 0; dstY = (dy > 0) ? 0 : -dy
    move.l r7, r0                         ; srcX = 0
    move.l r8, r0                         ; dstX = 0
    bge r21, r0, scroll_dx_pos
    sub.l r8, r0, r21                     ; dstX = -dx
    bra scroll_dx_done
scroll_dx_pos:
    move.l r7, r21                        ; srcX = dx
scroll_dx_done:

    move.l r9, r0                         ; srcY = 0
    move.l r10, r0                        ; dstY = 0
    bge r22, r0, scroll_dy_pos
    sub.l r10, r0, r22                    ; dstY = -dy
    bra scroll_dy_done
scroll_dy_pos:
    move.l r9, r22                        ; srcY = dy
scroll_dy_done:

    ; src addr = base + (yMin + srcY) * stride + (xMin + srcX)
    add.l r7, r7, r18                    ; xMin + srcX
    add.l r9, r9, r19                    ; yMin + srcY
    mulu.l r23, r9, r29                  ; (yMin+srcY) * stride
    add.l r23, r12, r23                  ; base + row offset
    add.l r23, r23, r7                   ; + col offset = src

    ; dst addr = base + (yMin + dstY) * stride + (xMin + dstX)
    add.l r8, r8, r18
    add.l r10, r10, r19
    mulu.l r24, r10, r29
    add.l r24, r12, r24
    add.l r24, r24, r8                   ; dst

    ; Row-by-row copy (copyH rows of copyW bytes)
    ; If dy > 0, copy top-to-bottom; if dy < 0, bottom-to-top
    blt r22, r0, scroll_copy_reverse

    ; Forward copy (top to bottom)
    move.l r27, r26                       ; row count
scroll_fwd_row:
    beq r27, r0, scroll_fill
    move.l r7, r25                        ; bytes to copy
    move.l r8, r23                        ; src row
    move.l r9, r24                        ; dst row
scroll_fwd_byte:
    load.b r10, 0(r8)
    store.b r10, 0(r9)
    add.l r8, r8, #1
    add.l r9, r9, #1
    sub.l r7, r7, #1
    bne r7, r0, scroll_fwd_byte
    add.l r23, r23, r29                   ; src += stride
    add.l r24, r24, r29                   ; dst += stride
    sub.l r27, r27, #1
    bra scroll_fwd_row

scroll_copy_reverse:
    ; Reverse copy (bottom to top)
    ; Start from last row
    sub.l r7, r26, #1                    ; lastRow = copyH - 1
    mulu.l r8, r7, r29                   ; lastRow * stride
    add.l r23, r23, r8                   ; src += lastRow * stride
    add.l r24, r24, r8                   ; dst += lastRow * stride
    move.l r27, r26
scroll_rev_row:
    beq r27, r0, scroll_fill
    move.l r7, r25
    move.l r8, r23
    move.l r9, r24
scroll_rev_byte:
    load.b r10, 0(r8)
    store.b r10, 0(r9)
    add.l r8, r8, #1
    add.l r9, r9, #1
    sub.l r7, r7, #1
    bne r7, r0, scroll_rev_byte
    sub.l r23, r23, r29                   ; src -= stride
    sub.l r24, r24, r29                   ; dst -= stride
    sub.l r27, r27, #1
    bra scroll_rev_row

scroll_fill_all:
    ; Fill entire region with 0
    mulu.l r7, r19, r29                  ; yMin * stride
    add.l r23, r12, r7                   ; base + yMin * stride
    add.l r23, r23, r18                  ; + xMin

    move.l r27, r17                       ; height rows
scroll_fill_all_row:
    beq r27, r0, op_done_ok
    move.l r7, r16
    move.l r8, r23
scroll_fill_all_byte:
    store.b r0, 0(r8)
    add.l r8, r8, #1
    sub.l r7, r7, #1
    bne r7, r0, scroll_fill_all_byte
    add.l r23, r23, r29
    sub.l r27, r27, #1
    bra scroll_fill_all_row

scroll_fill:
    ; Fill exposed region with 0
    ; Horizontal exposed strip (if dx != 0)
    beq r21, r0, scroll_fill_v

    blt r21, r0, scroll_fill_h_right
    ; dx > 0: fill left strip (xMin to xMin+dx-1)
    mulu.l r7, r19, r29                  ; yMin * stride
    add.l r23, r12, r7
    add.l r23, r23, r18                  ; + xMin
    move.l r27, r17                       ; all rows
scroll_fill_h_left_row:
    beq r27, r0, scroll_fill_v
    move.l r7, r21                        ; dx pixels
    move.l r8, r23
scroll_fill_h_left_byte:
    store.b r0, 0(r8)
    add.l r8, r8, #1
    sub.l r7, r7, #1
    bne r7, r0, scroll_fill_h_left_byte
    add.l r23, r23, r29
    sub.l r27, r27, #1
    bra scroll_fill_h_left_row

scroll_fill_h_right:
    ; dx < 0: fill right strip (xMin+width+dx to xMin+width-1)
    sub.l r7, r0, r21                    ; -dx = fill width
    add.l r8, r18, r16                   ; xMin + width
    sub.l r8, r8, r7                     ; - fillWidth = fill start X
    mulu.l r9, r19, r29
    add.l r23, r12, r9
    add.l r23, r23, r8
    move.l r27, r17
scroll_fill_h_right_row:
    beq r27, r0, scroll_fill_v
    move.l r9, r7
    move.l r10, r23
scroll_fill_h_right_byte:
    store.b r0, 0(r10)
    add.l r10, r10, #1
    sub.l r9, r9, #1
    bne r9, r0, scroll_fill_h_right_byte
    add.l r23, r23, r29
    sub.l r27, r27, #1
    bra scroll_fill_h_right_row

scroll_fill_v:
    ; Vertical exposed strip (if dy != 0)
    beq r22, r0, op_done_ok

    blt r22, r0, scroll_fill_v_bottom
    ; dy > 0: fill top strip (yMin to yMin+dy-1)
    mulu.l r7, r19, r29
    add.l r23, r12, r7
    add.l r23, r23, r18
    move.l r27, r22                       ; dy rows
scroll_fill_v_top_row:
    beq r27, r0, op_done_ok
    move.l r7, r16
    move.l r8, r23
scroll_fill_v_top_byte:
    store.b r0, 0(r8)
    add.l r8, r8, #1
    sub.l r7, r7, #1
    bne r7, r0, scroll_fill_v_top_byte
    add.l r23, r23, r29
    sub.l r27, r27, #1
    bra scroll_fill_v_top_row

scroll_fill_v_bottom:
    ; dy < 0: fill bottom strip (yMin+height+dy to yMin+height-1)
    sub.l r7, r0, r22                    ; -dy = fill height
    add.l r8, r19, r17                   ; yMin + height
    sub.l r8, r8, r7                     ; - fillHeight = fill start Y
    mulu.l r9, r8, r29
    add.l r23, r12, r9
    add.l r23, r23, r18
    move.l r27, r7                        ; -dy rows
scroll_fill_v_bot_row:
    beq r27, r0, op_done_ok
    move.l r7, r16
    move.l r8, r23
scroll_fill_v_bot_byte:
    store.b r0, 0(r8)
    add.l r8, r8, #1
    sub.l r7, r7, #1
    bne r7, r0, scroll_fill_v_bot_byte
    add.l r23, r23, r29
    sub.l r27, r27, #1
    bra scroll_fill_v_bot_row

; ============================================================================
; Completion handlers
; ============================================================================

op_done_ok:
    ; Write OK status
    move.l r7, #STATUS_OK
    store.l r7, RESP_STATUS(r20)
    move.l r7, #0
    store.l r7, RESP_RESULT(r20)
    store.l r7, RESP_LEN(r20)
    bra advance_tail

op_error:
    ; Write error status
    move.l r7, #STATUS_ERROR
    store.l r7, RESP_STATUS(r20)
    move.l r7, #1
    store.l r7, RESP_RESULT(r20)
    move.l r7, #0
    store.l r7, RESP_LEN(r20)

advance_tail:
    ; tail = (tail + 1) & 0x0F (mod 16)
    add.l r3, r3, #1
    and.l r3, r3, #0x0F
    store.b r3, RING_TAIL(r30)

    ; Back to poll loop
    bra poll_loop
