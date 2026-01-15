; scrolltext_demo.asm - Sine wave scrolltext demo with font lookup
; Font layout from cubeintro:
; Row 0: space ! " @ * £ ^ ' ( )
; Row 1: & ~ , - . + 0 1 2 3
; Row 2: 4 5 6 7 8 9 : ; = [
; Row 3: ] ? { A B C D E F G
; Row 4: H I J K L M N O P Q
; Row 5: R S T U V W X Y Z `

; ----------------------------------------------------------------------------
; VIDEO REGISTERS
; ----------------------------------------------------------------------------
.equ VIDEO_CTRL        0xF0000
.equ VIDEO_STATUS      0xF0008
.equ STATUS_VBLANK     0x02

.equ BLT_CTRL          0xF001C
.equ BLT_OP            0xF0020
.equ BLT_SRC           0xF0024
.equ BLT_DST           0xF0028
.equ BLT_WIDTH         0xF002C
.equ BLT_HEIGHT        0xF0030
.equ BLT_SRC_STRIDE    0xF0034
.equ BLT_DST_STRIDE    0xF0038
.equ BLT_COLOR         0xF003C
.equ BLT_STATUS        0xF0044

.equ VRAM_START        0x100000
.equ SCREEN_W          640
.equ SCREEN_H          480
.equ LINE_BYTES        2560
.equ BACKGROUND        0xFF200020

.equ BLT_OP_COPY       0
.equ BLT_OP_FILL       1
.equ BLT_OP_ALPHA      4

.equ CHAR_WIDTH        32
.equ CHAR_HEIGHT       32
.equ FONT_COLS         10
.equ FONT_STRIDE       1280
.equ FONT_ROW_BYTES    40960

.equ SCROLL_Y          224
.equ SCROLL_SPEED      3

.equ VAR_SCROLL_X      0x8800

; Data addresses (calculated after code)
; Code ends around 0x1800, pad to 0x2000
; Sin table: 0x2000 (1024 bytes)
; Char table: 0x2400 (512 bytes for 128 chars * 4)
; Font: 0x2600 (256000 bytes)
; Message: 0x2600 + 256000 = 0x40E00
.equ SIN_TABLE_ADDR    0x2000
.equ CHAR_TABLE_ADDR   0x2400
.equ FONT_ADDR         0x2600
.equ MESSAGE_ADDR      0x40E00

.org 0x1000

start:
    LDA #1
    STA @VIDEO_CTRL

    LDA #0
    STA @VAR_SCROLL_X

    ; Clear screen
    JSR wait_blit
    LDA #BLT_OP_FILL
    STA @BLT_OP
    LDA #VRAM_START
    STA @BLT_DST
    LDA #SCREEN_W
    STA @BLT_WIDTH
    LDA #SCREEN_H
    STA @BLT_HEIGHT
    LDA #LINE_BYTES
    STA @BLT_DST_STRIDE
    LDA #BACKGROUND
    STA @BLT_COLOR
    LDA #1
    STA @BLT_CTRL

main_loop:
    JSR wait_vblank

    ; Clear scroll area (covers 32px chars + ~50px sine wave range)
    ; Text center at SCROLL_Y=224, sine ±25px, char height 32px
    ; Clear from Y=180 to Y=280 (100px)
    JSR wait_blit
    LDA #BLT_OP_FILL
    STA @BLT_OP
    LDA #180
    MUL A, #LINE_BYTES
    ADD A, #VRAM_START
    STA @BLT_DST
    LDA #SCREEN_W
    STA @BLT_WIDTH
    LDA #100
    STA @BLT_HEIGHT
    LDA #LINE_BYTES
    STA @BLT_DST_STRIDE
    LDA #BACKGROUND
    STA @BLT_COLOR
    LDA #1
    STA @BLT_CTRL

    ; Draw scrolltext
    JSR draw_scrolltext

    ; Update scroll
    LDA @VAR_SCROLL_X
    ADD A, #SCROLL_SPEED
    STA @VAR_SCROLL_X

    JMP main_loop

; ----------------------------------------------------------------------------
wait_blit:
    LDA @BLT_STATUS
    AND A, #1
    JNZ A, wait_blit
    RTS

; ----------------------------------------------------------------------------
wait_vblank:
.w1:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JNZ A, .w1
.w2:
    LDA @VIDEO_STATUS
    AND A, #STATUS_VBLANK
    JZ A, .w2
    RTS

; ----------------------------------------------------------------------------
; draw_scrolltext
; Registers:
;   A - temp
;   B - scroll_x
;   C - char index in message
;   D - screen X position
;   E - char counter
;   F - temp
;   T - temp / character
;   U - Y position
; ----------------------------------------------------------------------------
draw_scrolltext:
    LDB @VAR_SCROLL_X

    ; C = starting char index = scroll_x / 32
    LDC B
    SHR C, #5

    ; D = starting X = -(scroll_x % 32)
    LDD B
    AND D, #0x1F
    LDF #0
    SUB F, D
    LDD F

    ; E = char counter
    LDE #0

.char_loop:
    ; Get character from message
    LDF #MESSAGE_ADDR
    ADD F, C
    LDT [F]
    AND T, #0xFF

    ; If null, wrap
    JZ T, .wrap_msg

    ; Skip if D < 0 (off left)
    LDA D
    AND A, #0x80000000
    JNZ A, .next_char

    ; Skip if D >= 608 (off right)
    LDA #608
    SUB A, D
    AND A, #0x80000000
    JNZ A, .done

    ; Look up character in table
    ; Table entry = (row * 10 + col) stored as word
    ; Font offset = row * FONT_ROW_BYTES + col * 128
    PUSH C
    PUSH D
    PUSH E

    ; Get table entry for this character
    LDF #CHAR_TABLE_ADDR
    LDA T
    SHL A, #2
    ADD F, A
    LDA [F]

    ; A now has the font offset for this character
    ; Add font base address
    ADD A, #FONT_ADDR
    LDF A

    ; Calculate sine offset for Y
    ; Use (screen_x + scroll) as phase
    LDA D
    ADD A, @VAR_SCROLL_X
    AND A, #0xFF
    SHL A, #2
    ADD A, #SIN_TABLE_ADDR
    LDU [A]
    ; U = 0-200, center at 100
    SUB U, #100
    ; Scale down: divide by 4
    LDA U
    AND A, #0x80000000
    JNZ A, .neg_y
    SHR U, #2
    JMP .y_done
.neg_y:
    ; Negate, shift, negate
    LDA #0
    SUB A, U
    SHR A, #2
    LDF #0
    SUB F, A
    LDU F
.y_done:
    ADD U, #SCROLL_Y

    ; Calculate dest address
    LDT U
    MUL T, #LINE_BYTES
    ADD T, #VRAM_START
    LDA D
    SHL A, #2
    ADD T, A

    ; Blit character with alpha transparency
    JSR wait_blit
    LDA #BLT_OP_ALPHA
    STA @BLT_OP
    LDA F
    STA @BLT_SRC
    LDA T
    STA @BLT_DST
    LDA #CHAR_WIDTH
    STA @BLT_WIDTH
    LDA #CHAR_HEIGHT
    STA @BLT_HEIGHT
    LDA #FONT_STRIDE
    STA @BLT_SRC_STRIDE
    LDA #LINE_BYTES
    STA @BLT_DST_STRIDE
    LDA #1
    STA @BLT_CTRL

    POP E
    POP D
    POP C

.next_char:
    ADD C, #1
    ADD D, #CHAR_WIDTH
    ADD E, #1
    LDA #22
    SUB A, E
    JNZ A, .char_loop

.done:
    RTS

.wrap_msg:
    LDC #0
    JMP .char_loop

; ----------------------------------------------------------------------------
; PAD TO DATA
; ----------------------------------------------------------------------------
.org 0x1FF8
    NOP

; ----------------------------------------------------------------------------
; SINE TABLE (256 entries, 0-200 range)
; ----------------------------------------------------------------------------
sin_table:
.word 100
.word 102
.word 105
.word 107
.word 110
.word 112
.word 115
.word 117
.word 120
.word 122
.word 124
.word 127
.word 129
.word 131
.word 134
.word 136
.word 138
.word 140
.word 142
.word 144
.word 146
.word 148
.word 150
.word 152
.word 154
.word 155
.word 157
.word 159
.word 160
.word 162
.word 163
.word 165
.word 166
.word 167
.word 169
.word 170
.word 171
.word 172
.word 173
.word 174
.word 175
.word 176
.word 177
.word 177
.word 178
.word 179
.word 179
.word 180
.word 180
.word 181
.word 181
.word 181
.word 182
.word 182
.word 182
.word 182
.word 182
.word 182
.word 182
.word 182
.word 182
.word 182
.word 182
.word 181
.word 181
.word 181
.word 180
.word 180
.word 179
.word 179
.word 178
.word 177
.word 177
.word 176
.word 175
.word 174
.word 173
.word 172
.word 171
.word 170
.word 169
.word 167
.word 166
.word 165
.word 163
.word 162
.word 160
.word 159
.word 157
.word 155
.word 154
.word 152
.word 150
.word 148
.word 146
.word 144
.word 142
.word 140
.word 138
.word 136
.word 134
.word 131
.word 129
.word 127
.word 124
.word 122
.word 120
.word 117
.word 115
.word 112
.word 110
.word 107
.word 105
.word 102
.word 100
.word 97
.word 95
.word 92
.word 90
.word 87
.word 85
.word 82
.word 80
.word 77
.word 75
.word 72
.word 70
.word 68
.word 65
.word 63
.word 61
.word 59
.word 57
.word 55
.word 53
.word 51
.word 49
.word 47
.word 45
.word 44
.word 42
.word 40
.word 39
.word 37
.word 36
.word 34
.word 33
.word 32
.word 30
.word 29
.word 28
.word 27
.word 26
.word 25
.word 24
.word 23
.word 22
.word 22
.word 21
.word 20
.word 20
.word 19
.word 19
.word 18
.word 18
.word 18
.word 17
.word 17
.word 17
.word 17
.word 17
.word 17
.word 17
.word 17
.word 17
.word 17
.word 17
.word 18
.word 18
.word 18
.word 19
.word 19
.word 20
.word 20
.word 21
.word 22
.word 22
.word 23
.word 24
.word 25
.word 26
.word 27
.word 28
.word 29
.word 30
.word 32
.word 33
.word 34
.word 36
.word 37
.word 39
.word 40
.word 42
.word 44
.word 45
.word 47
.word 49
.word 51
.word 53
.word 55
.word 57
.word 59
.word 61
.word 63
.word 65
.word 68
.word 70
.word 72
.word 75
.word 77
.word 80
.word 82
.word 85
.word 87
.word 90
.word 92
.word 95
.word 97
.word 100
.word 102
.word 105
.word 107
.word 110
.word 112
.word 115
.word 117
.word 120
.word 122
.word 124
.word 127
.word 129
.word 131
.word 134
.word 136
.word 138
.word 140
.word 142
.word 144
.word 146
.word 148
.word 150
.word 152
.word 154
.word 155
.word 157
.word 159

; ----------------------------------------------------------------------------
; CHARACTER LOOKUP TABLE
; For ASCII 0-127, stores font byte offset
; offset = row * 40960 + col * 128
; Unmapped characters point to space (0)
; ----------------------------------------------------------------------------
char_table:
; ASCII 0-31: control chars -> space
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
.word 0
; ASCII 32-47: space ! " # $ % & ' ( ) * + , - . /
; space=0,0 !=1,0 "=2,0 #->space $->space %->space &=0,1 '=7,0 (=8,0 )=9,0 *=4,0 +=5,1 ,=2,1 -=3,1 .=4,1 /->space
.word 0
.word 128
.word 256
.word 0
.word 0
.word 0
.word 40960
.word 896
.word 1024
.word 1152
.word 512
.word 41600
.word 41216
.word 41344
.word 41472
.word 0
; ASCII 48-63: 0 1 2 3 4 5 6 7 8 9 : ; < = > ?
; 0=6,1 1=7,1 2=8,1 3=9,1 4=0,2 5=1,2 6=2,2 7=3,2 8=4,2 9=5,2 :=6,2 ;=7,2 <->space ==8,2 >->space ?=1,3
.word 41728
.word 41856
.word 41984
.word 42112
.word 81920
.word 82048
.word 82176
.word 82304
.word 82432
.word 82560
.word 82688
.word 82816
.word 0
.word 82944
.word 0
.word 122880
; ASCII 64-79: @ A B C D E F G H I J K L M N O
; @=3,0 A=3,3 B=4,3 C=5,3 D=6,3 E=7,3 F=8,3 G=9,3 H=0,4 I=1,4 J=2,4 K=3,4 L=4,4 M=5,4 N=6,4 O=7,4
.word 384
.word 123264
.word 123392
.word 123520
.word 123648
.word 123776
.word 123904
.word 124032
.word 163840
.word 163968
.word 164096
.word 164224
.word 164352
.word 164480
.word 164608
.word 164736
; ASCII 80-95: P Q R S T U V W X Y Z [ \ ] ^ _
; P=8,4 Q=9,4 R=0,5 S=1,5 T=2,5 U=3,5 V=4,5 W=5,5 X=6,5 Y=7,5 Z=8,5 [=9,2 \->space ]=0,3 ^=6,0 _->space
.word 164864
.word 164992
.word 204800
.word 204928
.word 205056
.word 205184
.word 205312
.word 205440
.word 205568
.word 205696
.word 205824
.word 83072
.word 0
.word 122752
.word 768
.word 0
; ASCII 96-111: ` a b c d e f g h i j k l m n o (lowercase -> uppercase)
.word 205952
.word 123264
.word 123392
.word 123520
.word 123648
.word 123776
.word 123904
.word 124032
.word 163840
.word 163968
.word 164096
.word 164224
.word 164352
.word 164480
.word 164608
.word 164736
; ASCII 112-127: p q r s t u v w x y z { | } ~ DEL
.word 164864
.word 164992
.word 204800
.word 204928
.word 205056
.word 205184
.word 205312
.word 205440
.word 205568
.word 205696
.word 205824
.word 123008
.word 0
.word 0
.word 41088
.word 0

; ----------------------------------------------------------------------------
; FONT DATA
; ----------------------------------------------------------------------------
font_data:
.incbin "font_rgba.bin"

; ----------------------------------------------------------------------------
; SCROLL MESSAGE
; ----------------------------------------------------------------------------
scroll_message:
.ascii "    HELLO WORLD!   SINE SCROLLTEXT DEMO FOR INTUITION ENGINE...   GREETS TO ALL DEMOSCENERS!        "
.byte 0
