; rotozoomer_x86.asm - Mode7 Blitter Rotozoomer
;
; x86 (32-bit) rotozoomer using hardware Mode7 affine texture mapping.
; Proper sine tables, smooth 256-level zoom, fractional animation accumulators.
;
; Assemble: nasm -f bin -o assembler/rotozoomer_x86.ie86 assembler/rotozoomer_x86.asm
; Run:      ./bin/IntuitionEngine -x86 assembler/rotozoomer_x86.ie86

%include "ie86.inc"

TEXTURE_BASE    equ 0x500000
BACK_BUFFER     equ 0x600000
RENDER_W        equ 640
RENDER_H        equ 480
TEX_STRIDE      equ 1024

; Animation accumulator increments (8.8 fixed-point)
ANGLE_INC       equ 313
SCALE_INC       equ 104

bits 32
                org 0x0000

start:
                mov esp, STACK_TOP

                ; Enable VideoChip, mode 0
                mov dword [VIDEO_CTRL], 1
                mov dword [VIDEO_MODE], 0

                ; Generate 256x256 checkerboard texture via 4x BLIT FILL
                call generate_texture

                ; Init animation accumulators
                mov dword [angle_accum], 0
                mov dword [scale_accum], 0

                ; Start PSG music playback (looping)
                mov dword [PSG_PLUS_CTRL], 1
                mov dword [PSG_PLAY_PTR], psg_data
                mov dword [PSG_PLAY_LEN], psg_data_end - psg_data
                mov dword [PSG_PLAY_CTRL], 5

main_loop:
                call compute_frame
                call render_mode7
                call blit_to_front
                call wait_vsync
                call advance_animation
                jmp main_loop

; =============================================================================
; WAIT FOR VSYNC
; =============================================================================
wait_vsync:
.wait_end:      mov eax, [VIDEO_STATUS]
                test eax, STATUS_VBLANK
                jnz .wait_end
.wait_start:    mov eax, [VIDEO_STATUS]
                test eax, STATUS_VBLANK
                jz .wait_start
                ret

; =============================================================================
; GENERATE TEXTURE (256x256 checkerboard via 4x BLIT FILL)
; =============================================================================
generate_texture:
                ; Top-left 128x128 white
                mov dword [BLT_OP], BLT_OP_FILL
                mov dword [BLT_DST], TEXTURE_BASE
                mov dword [BLT_WIDTH], 128
                mov dword [BLT_HEIGHT], 128
                mov dword [BLT_COLOR], 0xFFFFFFFF
                mov dword [BLT_DST_STRIDE], TEX_STRIDE
                mov dword [BLT_CTRL], 1
.w1:            mov eax, [BLT_STATUS]
                test eax, 2
                jnz .w1

                ; Top-right 128x128 black
                mov dword [BLT_OP], BLT_OP_FILL
                mov dword [BLT_DST], TEXTURE_BASE+512
                mov dword [BLT_WIDTH], 128
                mov dword [BLT_HEIGHT], 128
                mov dword [BLT_COLOR], 0xFF000000
                mov dword [BLT_DST_STRIDE], TEX_STRIDE
                mov dword [BLT_CTRL], 1
.w2:            mov eax, [BLT_STATUS]
                test eax, 2
                jnz .w2

                ; Bottom-left 128x128 black
                mov dword [BLT_OP], BLT_OP_FILL
                mov dword [BLT_DST], TEXTURE_BASE+131072
                mov dword [BLT_WIDTH], 128
                mov dword [BLT_HEIGHT], 128
                mov dword [BLT_COLOR], 0xFF000000
                mov dword [BLT_DST_STRIDE], TEX_STRIDE
                mov dword [BLT_CTRL], 1
.w3:            mov eax, [BLT_STATUS]
                test eax, 2
                jnz .w3

                ; Bottom-right 128x128 white
                mov dword [BLT_OP], BLT_OP_FILL
                mov dword [BLT_DST], TEXTURE_BASE+131584
                mov dword [BLT_WIDTH], 128
                mov dword [BLT_HEIGHT], 128
                mov dword [BLT_COLOR], 0xFFFFFFFF
                mov dword [BLT_DST_STRIDE], TEX_STRIDE
                mov dword [BLT_CTRL], 1
.w4:            mov eax, [BLT_STATUS]
                test eax, 2
                jnz .w4

                ret

; =============================================================================
; COMPUTE FRAME - calculate Mode7 parameters from animation state
; =============================================================================
compute_frame:
                ; angle_idx = angle_accum >> 8
                mov eax, [angle_accum]
                shr eax, 8
                and eax, 255
                mov esi, eax                    ; esi = angle_idx

                ; scale_idx = scale_accum >> 8
                mov eax, [scale_accum]
                shr eax, 8
                and eax, 255
                mov edi, eax                    ; edi = scale_idx

                ; cos_val = sine_table[(angle_idx + 64) & 255]
                mov eax, esi
                add eax, 64
                and eax, 255
                movsx ecx, word [sine_table + eax*2]    ; ecx = cos_val (signed)

                ; sin_val = sine_table[angle_idx]
                movsx edx, word [sine_table + esi*2]    ; edx = sin_val (signed)

                ; recip = recip_table[scale_idx]
                movzx ebx, word [recip_table + edi*2]   ; ebx = recip (unsigned)

                ; CA = cos_val * recip (signed 32-bit result, 16.16 FP)
                mov eax, ecx
                imul eax, ebx                   ; eax = CA
                mov [var_ca], eax

                ; SA = sin_val * recip
                mov eax, edx
                imul eax, ebx                   ; eax = SA
                mov [var_sa], eax

                ; u0 = 8388608 - CA*320 + SA*240
                ; CA*320 = CA*256 + CA*64 = (CA<<8) + (CA<<6)
                mov eax, [var_ca]
                mov ecx, eax
                shl eax, 8                      ; CA * 256
                shl ecx, 6                      ; CA * 64
                add eax, ecx                    ; eax = CA * 320

                mov ecx, [var_sa]
                mov edx, ecx
                shl ecx, 8                      ; SA * 256
                shl edx, 4                      ; SA * 16
                sub ecx, edx                    ; ecx = SA * 240

                mov edx, 0x800000               ; 8388608
                sub edx, eax                    ; - CA*320
                add edx, ecx                    ; + SA*240
                mov [var_u0], edx

                ; v0 = 8388608 - SA*320 - CA*240
                mov eax, [var_sa]
                mov ecx, eax
                shl eax, 8                      ; SA * 256
                shl ecx, 6                      ; SA * 64
                add eax, ecx                    ; eax = SA * 320

                mov ecx, [var_ca]
                mov edx, ecx
                shl ecx, 8                      ; CA * 256
                shl edx, 4                      ; CA * 16
                sub ecx, edx                    ; ecx = CA * 240

                mov edx, 0x800000               ; 8388608
                sub edx, eax                    ; - SA*320
                sub edx, ecx                    ; - CA*240
                mov [var_v0], edx

                ret

; =============================================================================
; RENDER MODE7 - configure and trigger Mode7 blit
; =============================================================================
render_mode7:
                mov dword [BLT_OP], BLT_OP_MODE7
                mov dword [BLT_SRC], TEXTURE_BASE
                mov dword [BLT_DST], BACK_BUFFER
                mov dword [BLT_WIDTH], RENDER_W
                mov dword [BLT_HEIGHT], RENDER_H
                mov dword [BLT_SRC_STRIDE], TEX_STRIDE
                mov dword [BLT_DST_STRIDE], LINE_BYTES
                mov dword [BLT_MODE7_TEX_W], 255
                mov dword [BLT_MODE7_TEX_H], 255

                ; Mode7 parameters
                mov eax, [var_u0]
                mov [BLT_MODE7_U0], eax

                mov eax, [var_v0]
                mov [BLT_MODE7_V0], eax

                mov eax, [var_ca]
                mov [BLT_MODE7_DU_COL], eax     ; du_col = CA

                mov eax, [var_sa]
                mov [BLT_MODE7_DV_COL], eax     ; dv_col = SA

                neg eax                          ; -SA
                mov [BLT_MODE7_DU_ROW], eax     ; du_row = -SA

                mov eax, [var_ca]
                mov [BLT_MODE7_DV_ROW], eax     ; dv_row = CA

                ; Trigger blit
                mov dword [BLT_CTRL], 1

                ; Wait for completion
.wait:          mov eax, [BLT_STATUS]
                test eax, 2
                jnz .wait

                ret

; =============================================================================
; BLIT BACK BUFFER TO FRONT (VRAM)
; =============================================================================
blit_to_front:
                mov dword [BLT_OP], BLT_OP_COPY
                mov dword [BLT_SRC], BACK_BUFFER
                mov dword [BLT_DST], VRAM_START
                mov dword [BLT_WIDTH], RENDER_W
                mov dword [BLT_HEIGHT], RENDER_H
                mov dword [BLT_SRC_STRIDE], LINE_BYTES
                mov dword [BLT_DST_STRIDE], LINE_BYTES
                mov dword [BLT_CTRL], 1

.wait:          mov eax, [BLT_STATUS]
                test eax, 2
                jnz .wait

                ret

; =============================================================================
; ADVANCE ANIMATION
; =============================================================================
advance_animation:
                mov eax, [angle_accum]
                add eax, ANGLE_INC
                and eax, 0xFFFF
                mov [angle_accum], eax

                mov eax, [scale_accum]
                add eax, SCALE_INC
                and eax, 0xFFFF
                mov [scale_accum], eax

                ret

; =============================================================================
; VARIABLES
; =============================================================================
angle_accum:    dd 0
scale_accum:    dd 0
var_ca:         dd 0
var_sa:         dd 0
var_u0:         dd 0
var_v0:         dd 0

; =============================================================================
; SINE TABLE - 256 entries, signed 16-bit, round(sin(i*2pi/256)*256)
; =============================================================================
sine_table:
                dw 0,6,13,19,25,31,38,44,50,56,62,68,74,80,86,92
                dw 98,104,109,115,121,126,132,137,142,147,152,157,162,167,172,177
                dw 181,185,190,194,198,202,206,209,213,216,220,223,226,229,231,234
                dw 237,239,241,243,245,247,248,250,251,252,253,254,255,255,256,256
                dw 256,256,256,255,255,254,253,252,251,250,248,247,245,243,241,239
                dw 237,234,231,229,226,223,220,216,213,209,206,202,198,194,190,185
                dw 181,177,172,167,162,157,152,147,142,137,132,126,121,115,109,104
                dw 98,92,86,80,74,68,62,56,50,44,38,31,25,19,13,6
                dw 0,-6,-13,-19,-25,-31,-38,-44,-50,-56,-62,-68,-74,-80,-86,-92
                dw -98,-104,-109,-115,-121,-126,-132,-137,-142,-147,-152,-157,-162,-167,-172,-177
                dw -181,-185,-190,-194,-198,-202,-206,-209,-213,-216,-220,-223,-226,-229,-231,-234
                dw -237,-239,-241,-243,-245,-247,-248,-250,-251,-252,-253,-254,-255,-255,-256,-256
                dw -256,-256,-256,-255,-255,-254,-253,-252,-251,-250,-248,-247,-245,-243,-241,-239
                dw -237,-234,-231,-229,-226,-223,-220,-216,-213,-209,-206,-202,-198,-194,-190,-185
                dw -181,-177,-172,-167,-162,-157,-152,-147,-142,-137,-132,-126,-121,-115,-109,-104
                dw -98,-92,-86,-80,-74,-68,-62,-56,-50,-44,-38,-31,-25,-19,-13,-6

; =============================================================================
; RECIPROCAL TABLE - 256 entries, unsigned 16-bit, round(256/(0.5+sin(i*2pi/256)*0.3))
; =============================================================================
recip_table:
                dw 512,505,497,490,484,477,471,464,458,453,447,441,436,431,426,421
                dw 416,412,407,403,399,395,391,388,384,381,377,374,371,368,365,362
                dw 359,357,354,352,350,348,345,343,342,340,338,336,335,333,332,331
                dw 329,328,327,326,325,324,324,323,322,322,321,321,321,320,320,320
                dw 320,320,320,320,321,321,321,322,322,323,324,324,325,326,327,328
                dw 329,331,332,333,335,336,338,340,342,343,345,348,350,352,354,357
                dw 359,362,365,368,371,374,377,381,384,388,391,395,399,403,407,412
                dw 416,421,426,431,436,441,447,453,458,464,471,477,484,490,497,505
                dw 512,520,528,536,544,553,561,571,580,589,599,610,620,631,642,653
                dw 665,676,689,701,714,727,740,754,768,782,797,812,827,842,858,873
                dw 889,905,922,938,955,972,988,1005,1022,1038,1055,1071,1087,1103,1119,1134
                dw 1149,1163,1177,1190,1202,1214,1225,1235,1244,1252,1260,1266,1271,1275,1278,1279
                dw 1280,1279,1278,1275,1271,1266,1260,1252,1244,1235,1225,1214,1202,1190,1177,1163
                dw 1149,1134,1119,1103,1087,1071,1055,1038,1022,1005,988,972,955,938,922,905
                dw 889,873,858,842,827,812,797,782,768,754,740,727,714,701,689,676
                dw 665,653,642,631,620,610,599,589,580,571,561,553,544,536,528,520

; =============================================================================
; MUSIC DATA
; =============================================================================
psg_data:
                incbin "OverscanScreen.ym"
psg_data_end:
