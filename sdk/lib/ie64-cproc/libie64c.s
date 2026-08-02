align 16
runtime.ie64_align8:
	li r2, #-8
	bhi r1, r2, .Lruntime.2
.Lruntime.1:
	add.q r1, r1, #7
	and.q r1, r1, #-8
	move.q r1, r1
	rts
.Lruntime.2:
	li r1, #0
	rts
align 16
runtime.ie64_heap_first_byte:
	move.q r1, #__ie64_heap_start+7
	and.q r1, r1, #-8
	move.q r1, r1
	rts
align 16
malloc:
	sub.q r31, r31, #24
	store.q r18, 0(r31)
	store.q r19, 8(r31)
	move.q r18, r1
	move.q r1, r18
	jsr runtime.ie64_align8
	move.q r19, r1
	li r1, #0
	beq r18, r1, .Lruntime.14
.Lruntime.5:
	li r1, #0
	beq r19, r1, .Lruntime.14
.Lruntime.6:
	li r1, #-9
	bhi r19, r1, .Lruntime.14
.Lruntime.7:
	move.q r30, #runtime.ie64_heap_cursor+0
	load.q r1, (r30)
	li r2, #0
	bne r1, r2, .Lruntime.9
.Lruntime.8:
	jsr runtime.ie64_heap_first_byte
	move.q r30, #runtime.ie64_heap_cursor+0
	store.q r1, (r30)
.Lruntime.9:
	move.q r30, #runtime.ie64_heap_limit+0
	load.q r3, (r30)
	bls r3, r1, .Lruntime.13
.Lruntime.10:
	add.q r2, r19, #8
	sub.q r3, r3, r1
	divs.q r3, r3, #1
	bhi r2, r3, .Lruntime.12
.Lruntime.11:
	store.q r18, (r1)
	move.q r30, #runtime.ie64_heap_cursor+0
	load.q r3, (r30)
	add.q r2, r2, r3
	move.q r30, #runtime.ie64_heap_cursor+0
	store.q r2, (r30)
	add.q r1, r1, #8
	move.q r1, r1
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.Lruntime.12:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.Lruntime.13:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.Lruntime.14:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
align 16
free:
.Lruntime.16:
	rts
align 16
calloc:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	li r3, #0
	beq r1, r3, .Lruntime.23
.Lruntime.18:
	li r3, #0
	beq r2, r3, .Lruntime.23
.Lruntime.19:
	li r3, #-1
	divu.q r3, r3, r2
	bhi r1, r3, .Lruntime.23
.Lruntime.20:
	mulu.q r18, r1, r2
	move.q r1, r18
	jsr malloc
	move.q r3, r18
	move.q r18, r1
	li r1, #0
	beq r18, r1, .Lruntime.22
.Lruntime.21:
	move.q r1, r18
	li r2, #0
	jsr memset
.Lruntime.22:
	move.q r1, r18
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.Lruntime.23:
	li r1, #0
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
align 16
realloc:
	sub.q r31, r31, #24
	store.q r18, 0(r31)
	store.q r19, 8(r31)
	move.q r30, r1
	move.q r1, r2
	move.q r2, r30
	li r3, #0
	beq r2, r3, .Lruntime.32
.Lruntime.25:
	move.q r19, r2
	li r2, #0
	bne r1, r2, .Lruntime.27
.Lruntime.26:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.Lruntime.27:
	move.q r18, r1
	move.q r1, r1
	jsr malloc
	move.q r2, r19
	move.q r30, r1
	move.q r1, r18
	move.q r18, r30
	li r3, #0
	beq r18, r3, .Lruntime.31
.Lruntime.28:
	sub.q r3, r2, #8
	load.q r19, (r3)
	bls r1, r19, .Lruntime.30
.Lruntime.29:
	move.q r1, r19
.Lruntime.30:
	move.q r19, r1
	move.q r1, r18
	move.q r3, r19
	jsr memcpy
	move.q r1, r18
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.Lruntime.31:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.Lruntime.32:
	jsr malloc
	move.q r1, r1
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
align 16
memchr:
	and.q r2, r2, #0xff
.Lruntime.34:
	li r4, #0
	beq r3, r4, .Lruntime.38
.Lruntime.35:
	load.b r4, (r1)
	and.q r4, r4, #0xffffffff
	and.q r5, r2, #0xffffffff
	beq r4, r5, .Lruntime.37
.Lruntime.36:
	add.q r1, r1, #1
	sub.q r3, r3, #1
	bra .Lruntime.34
.Lruntime.37:
	move.q r1, r1
	rts
.Lruntime.38:
	li r1, #0
	rts
align 16
memcmp:
.Lruntime.40:
	li r4, #0
	beq r3, r4, .Lruntime.47
.Lruntime.41:
	load.b r4, (r1)
	load.b r5, (r2)
	and.q r6, r4, #0xffffffff
	and.q r7, r5, #0xffffffff
	bne r6, r7, .Lruntime.43
.Lruntime.42:
	add.q r1, r1, #1
	add.q r2, r2, #1
	sub.q r3, r3, #1
	bra .Lruntime.40
.Lruntime.43:
	sext.l r1, r4
	sext.l r2, r5
	blt r1, r2, .Lruntime.45
.Lruntime.44:
	li r1, #1
	bra .Lruntime.46
.Lruntime.45:
	li r1, #4294967295
.Lruntime.46:
	sext.l r1, r1
	move.q r1, r1
	rts
.Lruntime.47:
	li r1, #0
	rts
align 16
memcpy:
	move.q r4, r1
.Lruntime.49:
	move.q r1, r4
.Lruntime.50:
	li r5, #0
	beq r3, r5, .Lruntime.52
.Lruntime.51:
	move.q r5, r2
	add.q r2, r2, #1
	load.b r6, (r5)
	move.q r5, r1
	add.q r1, r1, #1
	store.b r6, (r5)
	sub.q r3, r3, #1
	bra .Lruntime.50
.Lruntime.52:
	move.q r1, r4
	rts
align 16
memmove:
	move.q r4, r1
	bls r2, r4, .Lruntime.57
.Lruntime.54:
	move.q r1, r4
.Lruntime.55:
	li r5, #0
	beq r3, r5, .Lruntime.60
.Lruntime.56:
	move.q r5, r2
	add.q r2, r2, #1
	load.b r6, (r5)
	move.q r5, r1
	add.q r1, r1, #1
	store.b r6, (r5)
	sub.q r3, r3, #1
	bra .Lruntime.55
.Lruntime.57:
	move.q r1, r2
	add.q r2, r4, r3
	add.q r1, r1, r3
.Lruntime.58:
	li r5, #0
	beq r3, r5, .Lruntime.60
.Lruntime.59:
	sub.q r1, r1, #1
	load.b r5, (r1)
	sub.q r2, r2, #1
	store.b r5, (r2)
	sub.q r3, r3, #1
	bra .Lruntime.58
.Lruntime.60:
	move.q r1, r4
	rts
align 16
memset:
	move.q r4, r1
.Lruntime.62:
	move.q r1, r4
.Lruntime.63:
	li r5, #0
	beq r3, r5, .Lruntime.65
.Lruntime.64:
	move.q r5, r1
	add.q r1, r1, #1
	store.b r2, (r5)
	sub.q r3, r3, #1
	bra .Lruntime.63
.Lruntime.65:
	move.q r1, r4
	rts
align 16
strlen:
	move.q r2, r1
.Lruntime.67:
	move.q r1, r2
.Lruntime.68:
	load.b r3, (r1)
	sext.b r3, r3
	beq r3, r0, .Lruntime.70
.Lruntime.69:
	add.q r1, r1, #1
	bra .Lruntime.68
.Lruntime.70:
	sub.q r1, r1, r2
	divs.q r1, r1, #1
	move.q r1, r1
	rts
align 16
strcpy:
	move.q r3, r1
.Lruntime.72:
	move.q r1, r3
.Lruntime.73:
	load.b r4, (r2)
	sext.b r4, r4
	store.b r4, (r1)
	beq r4, r0, .Lruntime.75
.Lruntime.74:
	add.q r2, r2, #1
	add.q r1, r1, #1
	bra .Lruntime.73
.Lruntime.75:
	move.q r1, r3
	rts
align 16
strncpy:
	move.q r4, r2
	move.q r2, r1
.Lruntime.77:
	move.q r1, r2
.Lruntime.78:
	li r5, #0
	beq r3, r5, .Lruntime.80
.Lruntime.79:
	load.b r6, (r4)
	sext.b r6, r6
	bne r6, r0, .Lruntime.83
.Lruntime.80:
	li r4, #0
	beq r3, r4, .Lruntime.82
.Lruntime.81:
	move.q r4, r1
	add.q r1, r1, #1
	li r5, #0
	store.b r5, (r4)
	sub.q r3, r3, #1
	bra .Lruntime.80
.Lruntime.82:
	move.q r1, r2
	rts
.Lruntime.83:
	add.q r4, r4, #1
	move.q r5, r1
	add.q r1, r1, #1
	store.b r6, (r5)
	sub.q r3, r3, #1
	bra .Lruntime.78
align 16
strcat:
	sub.q r31, r31, #24
	store.q r18, 0(r31)
	store.q r19, 8(r31)
	move.q r19, r2
	move.q r18, r1
	move.q r1, r1
	jsr strlen
	move.q r2, r19
	move.q r3, r1
	move.q r1, r18
	add.q r1, r1, r3
	jsr strcpy
	move.q r1, r1
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
align 16
strncat:
	sub.q r31, r31, #24
	store.q r18, 0(r31)
	store.q r19, 8(r31)
	store.q r20, 16(r31)
	move.q r19, r2
	move.q r20, r3
	move.q r18, r1
	move.q r1, r1
	jsr strlen
	move.q r3, r20
	move.q r2, r19
	move.q r4, r1
	move.q r1, r18
	add.q r4, r1, r4
.Lruntime.86:
	li r5, #0
	beq r3, r5, .Lruntime.89
.Lruntime.87:
	load.b r6, (r2)
	sext.b r6, r6
	beq r6, r0, .Lruntime.89
.Lruntime.88:
	add.q r2, r2, #1
	move.q r5, r4
	add.q r4, r4, #1
	store.b r6, (r5)
	sub.q r3, r3, #1
	bra .Lruntime.86
.Lruntime.89:
	li r2, #0
	store.b r2, (r4)
	move.q r1, r1
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	load.q r20, 16(r31)
	add.q r31, r31, #24
	rts
align 16
strcmp:
.Lruntime.91:
	load.b r3, (r1)
	sext.b r3, r3
	load.b r4, (r2)
	sext.b r4, r4
	and.q r5, r3, #0xffffffff
	and.q r6, r4, #0xffffffff
	bne r5, r6, .Lruntime.94
.Lruntime.92:
	beq r3, r0, .Lruntime.94
.Lruntime.93:
	add.q r1, r1, #1
	add.q r2, r2, #1
	bra .Lruntime.91
.Lruntime.94:
	and.q r1, r3, #0xff
	and.q r2, r4, #0xff
	sext.l r3, r1
	sext.l r4, r2
	blt r3, r4, .Lruntime.96
.Lruntime.95:
	sext.l r1, r1
	sext.l r2, r2
	li r30, #0
	ble r1, r2, .Lcmpruntime0
	li r30, #1
.Lcmpruntime0:
	move.q r1, r30
	bra .Lruntime.97
.Lruntime.96:
	li r1, #4294967295
.Lruntime.97:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
strncmp:
.Lruntime.99:
	li r4, #0
	beq r3, r4, .Lruntime.108
.Lruntime.100:
	load.b r4, (r1)
	sext.b r4, r4
	load.b r5, (r2)
	sext.b r5, r5
	and.q r6, r4, #0xffffffff
	and.q r7, r5, #0xffffffff
	bne r6, r7, .Lruntime.104
.Lruntime.101:
	beq r4, r0, .Lruntime.103
.Lruntime.102:
	add.q r2, r2, #1
	sub.q r3, r3, #1
	add.q r1, r1, #1
	bra .Lruntime.99
.Lruntime.103:
	li r1, #0
	rts
.Lruntime.104:
	and.q r1, r4, #0xff
	and.q r2, r5, #0xff
	sext.l r1, r1
	sext.l r2, r2
	blt r1, r2, .Lruntime.106
.Lruntime.105:
	li r1, #1
	bra .Lruntime.107
.Lruntime.106:
	li r1, #4294967295
.Lruntime.107:
	sext.l r1, r1
	move.q r1, r1
	rts
.Lruntime.108:
	li r1, #0
	rts
align 16
strchr:
	sext.b r2, r2
.Lruntime.110:
	load.b r3, (r1)
	sext.b r3, r3
	and.q r4, r3, #0xffffffff
	and.q r5, r2, #0xffffffff
	beq r4, r5, .Lruntime.114
.Lruntime.111:
	beq r3, r0, .Lruntime.113
.Lruntime.112:
	add.q r1, r1, #1
	bra .Lruntime.110
.Lruntime.113:
	li r1, #0
	rts
.Lruntime.114:
	move.q r1, r1
	rts
align 16
isalpha:
	sext.l r2, r1
	li r3, #65
	sext.l r3, r3
	blt r2, r3, .Lruntime.117
.Lruntime.116:
	sext.l r2, r1
	li r3, #90
	sext.l r3, r3
	ble r2, r3, .Lruntime.120
.Lruntime.117:
	sext.l r2, r1
	li r3, #97
	sext.l r3, r3
	blt r2, r3, .Lruntime.119
.Lruntime.118:
	sext.l r1, r1
	li r2, #122
	sext.l r2, r2
	ble r1, r2, .Lruntime.120
.Lruntime.119:
	li r1, #0
	bra .Lruntime.121
.Lruntime.120:
	li r1, #1
.Lruntime.121:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
isdigit:
	sext.l r2, r1
	li r3, #48
	sext.l r3, r3
	blt r2, r3, .Lruntime.124
.Lruntime.123:
	sext.l r1, r1
	li r2, #57
	sext.l r2, r2
	ble r1, r2, .Lruntime.125
.Lruntime.124:
	li r1, #0
	bra .Lruntime.126
.Lruntime.125:
	li r1, #1
.Lruntime.126:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
islower:
	sext.l r2, r1
	li r3, #97
	sext.l r3, r3
	blt r2, r3, .Lruntime.129
.Lruntime.128:
	sext.l r1, r1
	li r2, #122
	sext.l r2, r2
	ble r1, r2, .Lruntime.130
.Lruntime.129:
	li r1, #0
	bra .Lruntime.131
.Lruntime.130:
	li r1, #1
.Lruntime.131:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
isupper:
	sext.l r2, r1
	li r3, #65
	sext.l r3, r3
	blt r2, r3, .Lruntime.134
.Lruntime.133:
	sext.l r1, r1
	li r2, #90
	sext.l r2, r2
	ble r1, r2, .Lruntime.135
.Lruntime.134:
	li r1, #0
	bra .Lruntime.136
.Lruntime.135:
	li r1, #1
.Lruntime.136:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
isalnum:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	sext.l r1, r1
	move.q r18, r1
	move.q r1, r1
	jsr isalpha
	move.q r2, r1
	move.q r1, r18
	bne r2, r0, .Lruntime.140
.Lruntime.138:
	jsr isdigit
	bne r1, r0, .Lruntime.140
.Lruntime.139:
	li r1, #0
	bra .Lruntime.141
.Lruntime.140:
	li r1, #1
.Lruntime.141:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
align 16
isblank:
	and.q r2, r1, #0xffffffff
	li r3, #32
	and.q r3, r3, #0xffffffff
	beq r2, r3, .Lruntime.145
.Lruntime.143:
	and.q r1, r1, #0xffffffff
	li r2, #9
	and.q r2, r2, #0xffffffff
	beq r1, r2, .Lruntime.145
.Lruntime.144:
	li r1, #0
	bra .Lruntime.146
.Lruntime.145:
	li r1, #1
.Lruntime.146:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
iscntrl:
	sext.l r2, r1
	li r3, #32
	sext.l r3, r3
	blt r2, r3, .Lruntime.150
.Lruntime.148:
	and.q r1, r1, #0xffffffff
	li r2, #127
	and.q r2, r2, #0xffffffff
	beq r1, r2, .Lruntime.150
.Lruntime.149:
	li r1, #0
	bra .Lruntime.151
.Lruntime.150:
	li r1, #1
.Lruntime.151:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
isprint:
	sext.l r2, r1
	li r3, #32
	sext.l r3, r3
	blt r2, r3, .Lruntime.154
.Lruntime.153:
	sext.l r1, r1
	li r2, #127
	sext.l r2, r2
	blt r1, r2, .Lruntime.155
.Lruntime.154:
	li r1, #0
	bra .Lruntime.156
.Lruntime.155:
	li r1, #1
.Lruntime.156:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
isgraph:
	sext.l r2, r1
	li r3, #32
	sext.l r3, r3
	ble r2, r3, .Lruntime.159
.Lruntime.158:
	sext.l r1, r1
	li r2, #127
	sext.l r2, r2
	blt r1, r2, .Lruntime.160
.Lruntime.159:
	li r1, #0
	bra .Lruntime.161
.Lruntime.160:
	li r1, #1
.Lruntime.161:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
isspace:
	and.q r2, r1, #0xffffffff
	li r3, #32
	and.q r3, r3, #0xffffffff
	beq r2, r3, .Lruntime.166
.Lruntime.163:
	sext.l r2, r1
	li r3, #9
	sext.l r3, r3
	blt r2, r3, .Lruntime.165
.Lruntime.164:
	sext.l r1, r1
	li r2, #13
	sext.l r2, r2
	ble r1, r2, .Lruntime.166
.Lruntime.165:
	li r1, #0
	bra .Lruntime.167
.Lruntime.166:
	li r1, #1
.Lruntime.167:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
isxdigit:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	move.q r18, r1
	sext.l r1, r1
	jsr isdigit
	move.q r2, r1
	move.q r1, r18
	bne r2, r0, .Lruntime.174
.Lruntime.169:
	sext.l r2, r1
	li r3, #97
	sext.l r3, r3
	blt r2, r3, .Lruntime.171
.Lruntime.170:
	sext.l r2, r1
	li r3, #102
	sext.l r3, r3
	ble r2, r3, .Lruntime.174
.Lruntime.171:
	sext.l r2, r1
	li r3, #65
	sext.l r3, r3
	blt r2, r3, .Lruntime.173
.Lruntime.172:
	sext.l r1, r1
	li r2, #70
	sext.l r2, r2
	ble r1, r2, .Lruntime.174
.Lruntime.173:
	li r1, #0
	bra .Lruntime.175
.Lruntime.174:
	li r1, #1
.Lruntime.175:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
align 16
ispunct:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	sext.l r1, r1
	move.q r18, r1
	move.q r1, r1
	jsr isgraph
	move.q r2, r1
	move.q r1, r18
	beq r2, r0, .Lruntime.179
.Lruntime.177:
	jsr isalnum
	bne r1, r0, .Lruntime.179
.Lruntime.178:
	li r1, #1
	bra .Lruntime.180
.Lruntime.179:
	li r1, #0
.Lruntime.180:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
align 16
tolower:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	move.q r18, r1
	sext.l r1, r1
	jsr isupper
	move.q r2, r1
	move.q r1, r18
	beq r2, r0, .Lruntime.183
.Lruntime.182:
	add.l r1, r1, #32
.Lruntime.183:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
align 16
toupper:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	move.q r18, r1
	sext.l r1, r1
	jsr islower
	move.q r2, r1
	move.q r1, r18
	beq r2, r0, .Lruntime.186
.Lruntime.185:
	sub.l r1, r1, #32
.Lruntime.186:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
align 16
abs:
	sext.l r2, r1
	li r3, #0
	sext.l r3, r3
	bge r2, r3, .Lruntime.189
.Lruntime.188:
	neg.l r1, r1
.Lruntime.189:
	sext.l r1, r1
	move.q r1, r1
	rts
align 16
labs:
	li r2, #0
	bge r1, r2, .Lruntime.192
.Lruntime.191:
	neg.q r1, r1
.Lruntime.192:
	move.q r1, r1
	rts
align 16
llabs:
	li r2, #0
	bge r1, r2, .Lruntime.195
.Lruntime.194:
	neg.q r1, r1
.Lruntime.195:
	move.q r1, r1
	rts
align 16
runtime.ie64_digit:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	move.q r18, r1
	sext.l r1, r1
	jsr isdigit
	move.q r2, r1
	move.q r1, r18
	bne r2, r0, .Lruntime.204
.Lruntime.197:
	sext.l r2, r1
	li r3, #97
	sext.l r3, r3
	blt r2, r3, .Lruntime.199
.Lruntime.198:
	sext.l r2, r1
	li r3, #122
	sext.l r3, r3
	ble r2, r3, .Lruntime.203
.Lruntime.199:
	sext.l r2, r1
	li r3, #65
	sext.l r3, r3
	blt r2, r3, .Lruntime.201
.Lruntime.200:
	sext.l r2, r1
	li r3, #90
	sext.l r3, r3
	ble r2, r3, .Lruntime.202
.Lruntime.201:
	li r1, #-1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.Lruntime.202:
	sub.l r1, r1, #55
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.Lruntime.203:
	sub.l r1, r1, #87
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.Lruntime.204:
	sub.l r1, r1, #48
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
align 16
runtime.ie64_strtoull:
	sub.q r31, r31, #72
	store.q r18, 16(r31)
	store.q r19, 24(r31)
	store.q r20, 32(r31)
	store.q r21, 40(r31)
	store.q r22, 48(r31)
	store.q r23, 56(r31)
	store.q r24, 64(r31)
	move.q r18, r1
	move.q r20, r2
	move.q r21, r3
	move.q r23, r4
	move.q r24, r5
	lea r30, 0(r31)
	store.q r20, (r30)
.Lruntime.206:
	move.q r1, r18
.Lruntime.207:
	move.q r19, r1
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr isspace
	move.q r3, r21
	move.q r2, r20
	move.q r4, r1
	move.q r1, r19
	move.q r19, r1
	add.q r1, r1, #1
	beq r4, r0, .Lruntime.209
.Lruntime.208:
	move.q r21, r3
	move.q r20, r2
	bra .Lruntime.207
.Lruntime.209:
	move.q r1, r19
	move.q r21, r3
.Lruntime.210:
	add.q r19, r1, #2
	bne r21, r0, .Lruntime.219
.Lruntime.211:
	load.b r2, (r1)
	sext.b r2, r2
	and.q r2, r2, #0xffffffff
	li r3, #48
	and.q r3, r3, #0xffffffff
	beq r2, r3, .Lruntime.213
.Lruntime.212:
	li r22, #10
	bra .Lruntime.227
.Lruntime.213:
	add.q r2, r1, #1
	load.b r2, (r2)
	sext.b r2, r2
	and.q r2, r2, #0xffffffff
	li r3, #120
	and.q r3, r3, #0xffffffff
	beq r2, r3, .Lruntime.215
.Lruntime.214:
	add.q r2, r1, #1
	load.b r2, (r2)
	sext.b r2, r2
	and.q r2, r2, #0xffffffff
	li r3, #88
	and.q r3, r3, #0xffffffff
	bne r2, r3, .Lruntime.217
.Lruntime.215:
	move.q r20, r1
	add.q r1, r1, #2
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr runtime.ie64_digit
	move.q r2, r1
	move.q r1, r20
	sext.l r2, r2
	li r3, #0
	sext.l r3, r3
	blt r2, r3, .Lruntime.217
.Lruntime.216:
	move.q r20, r1
	add.q r1, r1, #2
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr runtime.ie64_digit
	move.q r2, r1
	move.q r1, r20
	sext.l r2, r2
	li r3, #16
	sext.l r3, r3
	blt r2, r3, .Lruntime.218
.Lruntime.217:
	li r22, #8
	bra .Lruntime.227
.Lruntime.218:
	li r22, #16
	move.q r1, r19
	bra .Lruntime.227
.Lruntime.219:
	and.q r2, r21, #0xffffffff
	li r3, #16
	and.q r3, r3, #0xffffffff
	bne r2, r3, .Lruntime.226
.Lruntime.220:
	load.b r2, (r1)
	sext.b r2, r2
	and.q r2, r2, #0xffffffff
	li r3, #48
	and.q r3, r3, #0xffffffff
	bne r2, r3, .Lruntime.226
.Lruntime.221:
	add.q r2, r1, #1
	load.b r2, (r2)
	sext.b r2, r2
	and.q r2, r2, #0xffffffff
	li r3, #120
	and.q r3, r3, #0xffffffff
	beq r2, r3, .Lruntime.223
.Lruntime.222:
	add.q r2, r1, #1
	load.b r2, (r2)
	sext.b r2, r2
	and.q r2, r2, #0xffffffff
	li r3, #88
	and.q r3, r3, #0xffffffff
	bne r2, r3, .Lruntime.226
.Lruntime.223:
	move.q r20, r1
	add.q r1, r1, #2
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr runtime.ie64_digit
	move.q r2, r1
	move.q r1, r20
	sext.l r2, r2
	li r3, #0
	sext.l r3, r3
	blt r2, r3, .Lruntime.226
.Lruntime.224:
	move.q r20, r1
	add.q r1, r1, #2
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr runtime.ie64_digit
	move.q r2, r1
	move.q r1, r20
	sext.l r2, r2
	li r3, #16
	sext.l r3, r3
	bge r2, r3, .Lruntime.226
.Lruntime.225:
	move.q r1, r19
.Lruntime.226:
	move.l r22, r21
.Lruntime.227:
	and.q r20, r22, #0xffffffff
.Lruntime.228:
	li r19, #0
.Lruntime.229:
	move.q r21, r1
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr runtime.ie64_digit
	move.q r5, r24
	move.q r4, r23
	move.l r3, r22
	move.q r2, r1
	move.q r1, r21
	sext.l r6, r2
	li r7, #0
	sext.l r7, r7
	blt r6, r7, .Lruntime.238
.Lruntime.230:
	sext.l r6, r2
	sext.l r7, r3
	bge r6, r7, .Lruntime.237
.Lruntime.231:
	and.q r2, r2, #0xffffffff
	sub.q r6, r4, r2
	divu.q r6, r6, r20
	bhi r19, r6, .Lruntime.234
.Lruntime.232:
	load.l r6, (r5)
	sext.l r6, r6
	bne r6, r0, .Lruntime.235
.Lruntime.233:
	mulu.q r6, r20, r19
	add.q r19, r2, r6
	bra .Lruntime.235
.Lruntime.234:
	li r2, #1
	store.l r2, (r5)
.Lruntime.235:
	add.q r1, r1, #1
.Lruntime.236:
	move.l r22, r3
	move.q r24, r5
	move.q r23, r4
	bra .Lruntime.229
.Lruntime.237:
	load.q r20, 0(r31)
	bra .Lruntime.239
.Lruntime.238:
	load.q r20, 0(r31)
.Lruntime.239:
	li r2, #0
	beq r20, r2, .Lruntime.243
.Lruntime.240:
	bne r18, r1, .Lruntime.242
.Lruntime.241:
	move.q r1, r18
.Lruntime.242:
	store.q r1, (r20)
.Lruntime.243:
	move.q r1, r19
	load.q r18, 16(r31)
	load.q r19, 24(r31)
	load.q r20, 32(r31)
	load.q r21, 40(r31)
	load.q r22, 48(r31)
	load.q r23, 56(r31)
	load.q r24, 64(r31)
	add.q r31, r31, #72
	rts
align 16
strtoull:
	sub.q r31, r31, #40
	store.q r18, 8(r31)
	store.q r19, 16(r31)
	store.q r20, 24(r31)
	store.q r21, 32(r31)
	move.q r21, r1
	move.q r20, r2
	lea r30, 0(r31)
	store.q r1, (r30)
	lea r2, 0(r31)
	li r1, #0
	store.l r1, (r2)
.Lruntime.245:
	move.q r18, r21
	move.q r1, r21
.Lruntime.246:
	move.q r21, r3
	move.q r19, r1
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr isspace
	move.q r3, r21
	move.q r2, r20
	move.q r4, r1
	move.q r1, r19
	move.q r19, r1
	add.q r1, r1, #1
	beq r4, r0, .Lruntime.248
.Lruntime.247:
	move.q r20, r2
	bra .Lruntime.246
.Lruntime.248:
	move.q r30, r19
	move.q r19, r1
	move.q r1, r30
	move.q r20, r2
	move.q r21, r18
.Lruntime.249:
	load.b r2, (r1)
	sext.b r2, r2
	and.q r4, r2, #0xffffffff
	li r5, #45
	and.q r5, r5, #0xffffffff
	beq r4, r5, .Lruntime.253
.Lruntime.250:
	and.q r2, r2, #0xffffffff
	li r4, #43
	and.q r4, r4, #0xffffffff
	beq r2, r4, .Lruntime.252
.Lruntime.251:
	move.q r19, r21
	li r18, #0
	bra .Lruntime.255
.Lruntime.252:
	move.q r1, r19
	bra .Lruntime.254
.Lruntime.253:
	move.q r1, r19
.Lruntime.254:
	move.q r19, r21
.Lruntime.255:
	move.q r21, r1
	sext.l r3, r3
	move.q r1, r21
	move.q r2, r20
	li r4, #-1
	lea r5, 0(r31)
	jsr runtime.ie64_strtoull
	li r2, #0
	beq r20, r2, .Lruntime.258
.Lruntime.256:
	load.q r2, (r20)
	bne r2, r21, .Lruntime.258
.Lruntime.257:
	store.q r19, (r20)
.Lruntime.258:
	beq r18, r0, .Lruntime.261
.Lruntime.259:
	lea r2, 0(r31)
	load.l r2, (r2)
	sext.l r2, r2
	bne r2, r0, .Lruntime.261
.Lruntime.260:
	li r2, #0
	sub.q r1, r2, r1
.Lruntime.261:
	lea r2, 0(r31)
	load.l r2, (r2)
	sext.l r2, r2
	beq r2, r0, .Lruntime.263
.Lruntime.262:
	li r1, #-1
.Lruntime.263:
	move.q r1, r1
	load.q r18, 8(r31)
	load.q r19, 16(r31)
	load.q r20, 24(r31)
	load.q r21, 32(r31)
	add.q r31, r31, #40
	rts
align 16
strtoll:
	sub.q r31, r31, #40
	store.q r18, 8(r31)
	store.q r19, 16(r31)
	store.q r20, 24(r31)
	store.q r21, 32(r31)
	move.q r21, r1
	move.q r20, r2
	lea r30, 0(r31)
	store.q r1, (r30)
	lea r2, 0(r31)
	li r1, #0
	store.l r1, (r2)
.Lruntime.265:
	move.q r18, r21
	move.q r1, r21
.Lruntime.266:
	move.q r21, r3
	move.q r19, r1
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr isspace
	move.q r3, r21
	move.q r2, r20
	move.q r4, r1
	move.q r1, r19
	move.q r19, r1
	add.q r1, r1, #1
	beq r4, r0, .Lruntime.268
.Lruntime.267:
	move.q r20, r2
	bra .Lruntime.266
.Lruntime.268:
	move.q r30, r19
	move.q r19, r1
	move.q r1, r30
	move.q r20, r2
	move.q r21, r18
.Lruntime.269:
	load.b r2, (r1)
	sext.b r2, r2
	and.q r4, r2, #0xffffffff
	li r5, #45
	and.q r5, r5, #0xffffffff
	beq r4, r5, .Lruntime.273
.Lruntime.270:
	and.q r2, r2, #0xffffffff
	li r4, #43
	and.q r4, r4, #0xffffffff
	beq r2, r4, .Lruntime.272
.Lruntime.271:
	move.q r19, r21
	li r18, #0
	bra .Lruntime.275
.Lruntime.272:
	move.q r1, r19
	bra .Lruntime.274
.Lruntime.273:
	move.q r1, r19
.Lruntime.274:
	move.q r19, r21
.Lruntime.275:
	move.q r21, r1
	bne r18, r0, .Lruntime.277
.Lruntime.276:
	li r4, #9223372036854775807
	bra .Lruntime.278
.Lruntime.277:
	li r4, #0x8000000000000000
.Lruntime.278:
	sext.l r3, r3
	move.q r1, r21
	move.q r2, r20
	lea r5, 0(r31)
	jsr runtime.ie64_strtoull
	li r2, #0
	beq r20, r2, .Lruntime.281
.Lruntime.279:
	load.q r2, (r20)
	bne r2, r21, .Lruntime.281
.Lruntime.280:
	store.q r19, (r20)
.Lruntime.281:
	lea r2, 0(r31)
	load.l r2, (r2)
	sext.l r2, r2
	bne r2, r0, .Lruntime.288
.Lruntime.282:
	bne r18, r0, .Lruntime.284
.Lruntime.283:
	move.q r1, r1
	load.q r18, 8(r31)
	load.q r19, 16(r31)
	load.q r20, 24(r31)
	load.q r21, 32(r31)
	add.q r31, r31, #40
	rts
.Lruntime.284:
	li r2, #0x8000000000000000
	beq r1, r2, .Lruntime.286
.Lruntime.285:
	neg.q r1, r1
	bra .Lruntime.287
.Lruntime.286:
	li r1, #0x8000000000000000
.Lruntime.287:
	move.q r1, r1
	load.q r18, 8(r31)
	load.q r19, 16(r31)
	load.q r20, 24(r31)
	load.q r21, 32(r31)
	add.q r31, r31, #40
	rts
.Lruntime.288:
	bne r18, r0, .Lruntime.290
.Lruntime.289:
	li r1, #9223372036854775807
	bra .Lruntime.291
.Lruntime.290:
	li r1, #0x8000000000000000
.Lruntime.291:
	move.q r1, r1
	load.q r18, 8(r31)
	load.q r19, 16(r31)
	load.q r20, 24(r31)
	load.q r21, 32(r31)
	add.q r31, r31, #40
	rts
align 16
strtoul:
	sub.q r31, r31, #8
	sext.l r3, r3
	jsr strtoull
	move.q r1, r1
	add.q r31, r31, #8
	rts
align 16
strtol:
	sub.q r31, r31, #8
	sext.l r3, r3
	jsr strtoll
	move.q r1, r1
	add.q r31, r31, #8
	rts
align 16
runtime.ie64_swap:
.Lruntime.295:
	li r4, #0
	beq r3, r4, .Lruntime.297
.Lruntime.296:
	load.b r5, (r1)
	load.b r6, (r2)
	move.q r4, r1
	add.q r1, r1, #1
	store.b r6, (r4)
	move.q r4, r2
	add.q r2, r2, #1
	store.b r5, (r4)
	sub.q r3, r3, #1
	bra .Lruntime.295
.Lruntime.297:
	rts
align 16
qsort:
	sub.q r31, r31, #72
	store.q r18, 16(r31)
	store.q r19, 24(r31)
	store.q r20, 32(r31)
	store.q r21, 40(r31)
	store.q r22, 48(r31)
	store.q r23, 56(r31)
	store.q r24, 64(r31)
	move.q r24, r3
	move.q r22, r4
	lea r30, 0(r31)
	store.q r2, (r30)
	li r3, #2
	bhi r3, r2, .Lruntime.310
.Lruntime.299:
	li r3, #0
	beq r24, r3, .Lruntime.310
.Lruntime.300:
	li r18, #1
.Lruntime.301:
	bls r2, r18, .Lruntime.310
.Lruntime.302:
	move.q r19, r18
.Lruntime.303:
	move.q r20, r1
	li r1, #0
	beq r18, r1, .Lruntime.308
.Lruntime.304:
	move.q r21, r18
	sub.q r18, r18, #1
	mulu.q r1, r24, r18
	add.q r1, r20, r1
	mulu.q r2, r24, r21
	add.q r2, r20, r2
	move.q r21, r1
	move.q r1, r1
	move.q r23, r2
	move.q r2, r2
	jsr (r22)
	move.q r3, r24
	move.q r2, r23
	move.q r4, r1
	move.q r1, r21
	sext.l r4, r4
	li r5, #0
	sext.l r5, r5
	ble r4, r5, .Lruntime.307
.Lruntime.305:
	move.q r21, r3
	move.q r3, r3
	jsr runtime.ie64_swap
	move.q r4, r22
	move.q r3, r21
	move.q r1, r20
.Lruntime.306:
	move.q r22, r4
	move.q r24, r3
	bra .Lruntime.303
.Lruntime.307:
	move.q r24, r3
	load.q r2, 0(r31)
	bra .Lruntime.309
.Lruntime.308:
	load.q r2, 0(r31)
.Lruntime.309:
	move.q r18, r19
	move.q r1, r20
	move.q r19, r18
	add.q r18, r18, #1
	bra .Lruntime.301
.Lruntime.310:
	load.q r18, 16(r31)
	load.q r19, 24(r31)
	load.q r20, 32(r31)
	load.q r21, 40(r31)
	load.q r22, 48(r31)
	load.q r23, 56(r31)
	load.q r24, 64(r31)
	add.q r31, r31, #72
	rts
align 16
bsearch:
	sub.q r31, r31, #56
	store.q r18, 0(r31)
	store.q r19, 8(r31)
	store.q r20, 16(r31)
	store.q r21, 24(r31)
	store.q r22, 32(r31)
	store.q r23, 40(r31)
	store.q r24, 48(r31)
	move.q r23, r4
	move.q r24, r5
.Lruntime.312:
	move.q r22, r3
	li r3, #0
	beq r22, r3, .Lruntime.319
.Lruntime.313:
	lsr.q r18, r22, #1
	mulu.q r19, r23, r18
	move.q r21, r2
	add.q r2, r19, r2
	move.q r20, r1
	move.q r1, r1
	jsr (r24)
	move.q r5, r24
	move.q r4, r23
	move.q r3, r22
	move.q r2, r21
	move.q r6, r1
	move.q r1, r20
	beq r6, r0, .Lruntime.318
.Lruntime.314:
	sext.l r6, r6
	li r7, #0
	sext.l r7, r7
	blt r6, r7, .Lruntime.316
.Lruntime.315:
	add.q r6, r18, #1
	mulu.q r7, r4, r6
	add.q r2, r7, r2
	sub.q r3, r3, r6
	bra .Lruntime.317
.Lruntime.316:
	move.q r3, r18
.Lruntime.317:
	move.q r24, r5
	move.q r23, r4
	bra .Lruntime.312
.Lruntime.318:
	add.q r1, r19, r2
	move.q r1, r1
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	load.q r20, 16(r31)
	load.q r21, 24(r31)
	load.q r22, 32(r31)
	load.q r23, 40(r31)
	load.q r24, 48(r31)
	add.q r31, r31, #56
	rts
.Lruntime.319:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	load.q r20, 16(r31)
	load.q r21, 24(r31)
	load.q r22, 32(r31)
	load.q r23, 40(r31)
	load.q r24, 48(r31)
	add.q r31, r31, #56
	rts
align 8
runtime.ie64_heap_limit:
	dc.q 585728
align 8
runtime.ie64_heap_cursor:
	ds.b 8
