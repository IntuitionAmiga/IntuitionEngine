.section .text,"ax"
.local ie64_atomic_compare_exchange
.type ie64_atomic_compare_exchange,@function
.align 16
ie64_atomic_compare_exchange:
	move.q r29, r3
	move.q r30, r1
	move.q r1, r2
	cas r1, (r30), r29
	move.q r1, r1
	rts
.size ie64_atomic_compare_exchange,.-ie64_atomic_compare_exchange
.section .text,"ax"
.local ie64_atomic_exchange
.type ie64_atomic_exchange,@function
.align 16
ie64_atomic_exchange:
	xchg r1, (r1), r2
	move.q r1, r1
	rts
.size ie64_atomic_exchange,.-ie64_atomic_exchange
.section .text,"ax"
.local ie64_atomic_fetch_add
.type ie64_atomic_fetch_add,@function
.align 16
ie64_atomic_fetch_add:
	faa r1, (r1), r2
	move.q r1, r1
	rts
.size ie64_atomic_fetch_add,.-ie64_atomic_fetch_add
.section .text,"ax"
.local ie64_atomic_fetch_and
.type ie64_atomic_fetch_and,@function
.align 16
ie64_atomic_fetch_and:
	fand r1, (r1), r2
	move.q r1, r1
	rts
.size ie64_atomic_fetch_and,.-ie64_atomic_fetch_and
.section .text,"ax"
.local ie64_atomic_fetch_or
.type ie64_atomic_fetch_or,@function
.align 16
ie64_atomic_fetch_or:
	for r1, (r1), r2
	move.q r1, r1
	rts
.size ie64_atomic_fetch_or,.-ie64_atomic_fetch_or
.section .text,"ax"
.local ie64_atomic_fetch_xor
.type ie64_atomic_fetch_xor,@function
.align 16
ie64_atomic_fetch_xor:
	fxor r1, (r1), r2
	move.q r1, r1
	rts
.size ie64_atomic_fetch_xor,.-ie64_atomic_fetch_xor
.section .text,"ax"
.local ie64_nop
.type ie64_nop,@function
.align 16
ie64_nop:
	nop
.L7:
	rts
.size ie64_nop,.-ie64_nop
.section .text,"ax"
.local ie64_enable_interrupts
.type ie64_enable_interrupts,@function
.align 16
ie64_enable_interrupts:
	sei
.L9:
	rts
.size ie64_enable_interrupts,.-ie64_enable_interrupts
.section .text,"ax"
.local ie64_disable_interrupts
.type ie64_disable_interrupts,@function
.align 16
ie64_disable_interrupts:
	cli
.L11:
	rts
.size ie64_disable_interrupts,.-ie64_disable_interrupts
.section .text,"ax"
.local __ie64_assert_fail
.type __ie64_assert_fail,@function
.align 16
__ie64_assert_fail:
	halt
	halt
.size __ie64_assert_fail,.-__ie64_assert_fail
.section .text,"ax"
.global __ie64_terminate
.type __ie64_terminate,@function
.align 16
__ie64_terminate:
	halt
	halt
.size __ie64_terminate,.-__ie64_terminate
.section .text,"ax"
.global __libc_init_array
.type __libc_init_array,@function
.align 16
__libc_init_array:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
.L15:
	move.l r18, #lo32(__preinit_array_start+0)
	movt r18, #hi32(__preinit_array_start+0)
.L16:
	move.l r1, #lo32(__preinit_array_end+0)
	movt r1, #hi32(__preinit_array_end+0)
	bls r1, r18, .L18
.L17:
	load.q r1, (r18)
	jsr (r1)
	add.q r18, r18, #8
	bra .L16
.L18:
	move.l r18, #lo32(__init_array_start+0)
	movt r18, #hi32(__init_array_start+0)
.L19:
	move.l r1, #lo32(__init_array_end+0)
	movt r1, #hi32(__init_array_end+0)
	bls r1, r18, .L21
.L20:
	load.q r1, (r18)
	jsr (r1)
	add.q r18, r18, #8
	bra .L19
.L21:
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.size __libc_init_array,.-__libc_init_array
.section .text,"ax"
.global __libc_fini_array
.type __libc_fini_array,@function
.align 16
__libc_fini_array:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
.L23:
	move.l r18, #lo32(__fini_array_end+0)
	movt r18, #hi32(__fini_array_end+0)
.L24:
	move.l r1, #lo32(__fini_array_start+0)
	movt r1, #hi32(__fini_array_start+0)
	beq r18, r1, .L26
.L25:
	sub.q r18, r18, #8
	load.q r1, (r18)
	jsr (r1)
	bra .L24
.L26:
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.size __libc_fini_array,.-__libc_fini_array
.section .text,"ax"
.global exit
.type exit,@function
.align 16
exit:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	move.q r18, r1
	jsr __libc_fini_array
	move.q r1, r18
	sext.l r1, r1
	jsr __ie64_terminate
	halt
.size exit,.-exit
.section .text,"ax"
.local ie64_align8
.type ie64_align8,@function
.align 16
ie64_align8:
	move.l r2, #0xfffffff8
	movt r2, #0xffffffff
	bhi r1, r2, .L30
.L29:
	add.q r1, r1, #7
	move.l r2, #0xfffffff8
	movt r2, #0xffffffff
	and.q r1, r1, r2
	move.q r1, r1
	rts
.L30:
	li r1, #0
	rts
.size ie64_align8,.-ie64_align8
.section .text,"ax"
.local ie64_heap_first_byte
.type ie64_heap_first_byte,@function
.align 16
ie64_heap_first_byte:
	move.l r2, #0xfffffff8
	movt r2, #0xffffffff
	move.l r1, #lo32(__ie64_heap_start+7)
	movt r1, #hi32(__ie64_heap_start+7)
	and.q r1, r1, r2
	move.q r1, r1
	rts
.size ie64_heap_first_byte,.-ie64_heap_first_byte
.section .text,"ax"
.global malloc
.type malloc,@function
.align 16
malloc:
	sub.q r31, r31, #24
	store.q r18, 0(r31)
	store.q r19, 8(r31)
	move.q r18, r1
	move.q r1, r18
	jsr ie64_align8
	move.q r19, r1
	li r1, #0
	beq r18, r1, .L42
.L33:
	li r1, #0
	beq r19, r1, .L42
.L34:
	move.l r1, #0xfffffff7
	movt r1, #0xffffffff
	bhi r19, r1, .L42
.L35:
	move.l r30, #lo32(ie64_heap_cursor+0)
	movt r30, #hi32(ie64_heap_cursor+0)
	load.q r1, (r30)
	li r2, #0
	bne r1, r2, .L37
.L36:
	jsr ie64_heap_first_byte
	move.l r30, #lo32(ie64_heap_cursor+0)
	movt r30, #hi32(ie64_heap_cursor+0)
	store.q r1, (r30)
.L37:
	move.l r30, #lo32(ie64_heap_limit+0)
	movt r30, #hi32(ie64_heap_limit+0)
	load.q r3, (r30)
	bls r3, r1, .L41
.L38:
	add.q r2, r19, #8
	sub.q r3, r3, r1
	divs.q r3, r3, #1
	bhi r2, r3, .L40
.L39:
	store.q r18, (r1)
	move.l r30, #lo32(ie64_heap_cursor+0)
	movt r30, #hi32(ie64_heap_cursor+0)
	load.q r3, (r30)
	add.q r2, r2, r3
	move.l r30, #lo32(ie64_heap_cursor+0)
	movt r30, #hi32(ie64_heap_cursor+0)
	store.q r2, (r30)
	add.q r1, r1, #8
	move.q r1, r1
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.L40:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.L41:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.L42:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.size malloc,.-malloc
.section .text,"ax"
.global free
.type free,@function
.align 16
free:
.L44:
	rts
.size free,.-free
.section .text,"ax"
.global calloc
.type calloc,@function
.align 16
calloc:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	li r3, #0
	beq r1, r3, .L51
.L46:
	li r3, #0
	beq r2, r3, .L51
.L47:
	move.l r3, #0xffffffff
	movt r3, #0xffffffff
	divu.q r3, r3, r2
	bhi r1, r3, .L51
.L48:
	mulu.q r18, r1, r2
	move.q r1, r18
	jsr malloc
	move.q r3, r18
	move.q r18, r1
	li r1, #0
	beq r18, r1, .L50
.L49:
	move.q r1, r18
	li r2, #0
	jsr memset
.L50:
	move.q r1, r18
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.L51:
	li r1, #0
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.size calloc,.-calloc
.section .text,"ax"
.global realloc
.type realloc,@function
.align 16
realloc:
	sub.q r31, r31, #24
	store.q r18, 0(r31)
	store.q r19, 8(r31)
	move.q r30, r1
	move.q r1, r2
	move.q r2, r30
	li r3, #0
	beq r2, r3, .L60
.L53:
	move.q r19, r2
	li r2, #0
	bne r1, r2, .L55
.L54:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.L55:
	move.q r18, r1
	move.q r1, r1
	jsr malloc
	move.q r2, r19
	move.q r30, r1
	move.q r1, r18
	move.q r18, r30
	li r3, #0
	beq r18, r3, .L59
.L56:
	sub.q r3, r2, #8
	load.q r19, (r3)
	bls r1, r19, .L58
.L57:
	move.q r1, r19
.L58:
	move.q r19, r1
	move.q r1, r18
	move.q r3, r19
	jsr memcpy
	move.q r1, r18
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.L59:
	li r1, #0
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.L60:
	jsr malloc
	move.q r1, r1
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	add.q r31, r31, #24
	rts
.size realloc,.-realloc
.section .text,"ax"
.global memchr
.type memchr,@function
.align 16
memchr:
	and.q r2, r2, #0xff
.L62:
	li r4, #0
	beq r3, r4, .L66
.L63:
	load.b r4, (r1)
	move.l r4, r4
	move.l r5, r2
	beq r4, r5, .L65
.L64:
	add.q r1, r1, #1
	sub.q r3, r3, #1
	bra .L62
.L65:
	move.q r1, r1
	rts
.L66:
	li r1, #0
	rts
.size memchr,.-memchr
.section .text,"ax"
.global memcmp
.type memcmp,@function
.align 16
memcmp:
.L68:
	li r4, #0
	beq r3, r4, .L75
.L69:
	load.b r4, (r1)
	load.b r5, (r2)
	move.l r6, r4
	move.l r7, r5
	bne r6, r7, .L71
.L70:
	add.q r1, r1, #1
	add.q r2, r2, #1
	sub.q r3, r3, #1
	bra .L68
.L71:
	sext.l r1, r4
	sext.l r2, r5
	blt r1, r2, .L73
.L72:
	li r1, #1
	bra .L74
.L73:
	li r1, #-1
.L74:
	sext.l r1, r1
	move.q r1, r1
	rts
.L75:
	li r1, #0
	rts
.size memcmp,.-memcmp
.section .text,"ax"
.global memcpy
.type memcpy,@function
.align 16
memcpy:
	move.q r4, r1
.L77:
	move.q r1, r4
.L78:
	li r5, #0
	beq r3, r5, .L80
.L79:
	move.q r5, r2
	add.q r2, r2, #1
	load.b r6, (r5)
	move.q r5, r1
	add.q r1, r1, #1
	store.b r6, (r5)
	sub.q r3, r3, #1
	bra .L78
.L80:
	move.q r1, r4
	rts
.size memcpy,.-memcpy
.section .text,"ax"
.global memmove
.type memmove,@function
.align 16
memmove:
	move.q r4, r1
	bls r2, r4, .L85
.L82:
	move.q r1, r4
.L83:
	li r5, #0
	beq r3, r5, .L88
.L84:
	move.q r5, r2
	add.q r2, r2, #1
	load.b r6, (r5)
	move.q r5, r1
	add.q r1, r1, #1
	store.b r6, (r5)
	sub.q r3, r3, #1
	bra .L83
.L85:
	move.q r1, r2
	add.q r2, r4, r3
	add.q r1, r1, r3
.L86:
	li r5, #0
	beq r3, r5, .L88
.L87:
	sub.q r1, r1, #1
	load.b r5, (r1)
	sub.q r2, r2, #1
	store.b r5, (r2)
	sub.q r3, r3, #1
	bra .L86
.L88:
	move.q r1, r4
	rts
.size memmove,.-memmove
.section .text,"ax"
.global memset
.type memset,@function
.align 16
memset:
	move.q r4, r1
.L90:
	move.q r1, r4
.L91:
	li r5, #0
	beq r3, r5, .L93
.L92:
	move.q r5, r1
	add.q r1, r1, #1
	store.b r2, (r5)
	sub.q r3, r3, #1
	bra .L91
.L93:
	move.q r1, r4
	rts
.size memset,.-memset
.section .text,"ax"
.global strlen
.type strlen,@function
.align 16
strlen:
	move.q r2, r1
.L95:
	move.q r1, r2
.L96:
	load.b r3, (r1)
	sext.b r3, r3
	beq r3, r0, .L98
.L97:
	add.q r1, r1, #1
	bra .L96
.L98:
	sub.q r1, r1, r2
	divs.q r1, r1, #1
	move.q r1, r1
	rts
.size strlen,.-strlen
.section .text,"ax"
.global strcpy
.type strcpy,@function
.align 16
strcpy:
	move.q r3, r1
.L100:
	move.q r1, r3
.L101:
	load.b r4, (r2)
	sext.b r4, r4
	store.b r4, (r1)
	beq r4, r0, .L103
.L102:
	add.q r2, r2, #1
	add.q r1, r1, #1
	bra .L101
.L103:
	move.q r1, r3
	rts
.size strcpy,.-strcpy
.section .text,"ax"
.global strncpy
.type strncpy,@function
.align 16
strncpy:
	move.q r4, r2
	move.q r2, r1
.L105:
	move.q r1, r2
.L106:
	li r5, #0
	beq r3, r5, .L108
.L107:
	load.b r6, (r4)
	sext.b r6, r6
	bne r6, r0, .L111
.L108:
	li r4, #0
	beq r3, r4, .L110
.L109:
	move.q r4, r1
	add.q r1, r1, #1
	li r5, #0
	store.b r5, (r4)
	sub.q r3, r3, #1
	bra .L108
.L110:
	move.q r1, r2
	rts
.L111:
	add.q r4, r4, #1
	move.q r5, r1
	add.q r1, r1, #1
	store.b r6, (r5)
	sub.q r3, r3, #1
	bra .L106
.size strncpy,.-strncpy
.section .text,"ax"
.global strcat
.type strcat,@function
.align 16
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
.size strcat,.-strcat
.section .text,"ax"
.global strncat
.type strncat,@function
.align 16
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
.L114:
	li r5, #0
	beq r3, r5, .L117
.L115:
	load.b r6, (r2)
	sext.b r6, r6
	beq r6, r0, .L117
.L116:
	add.q r2, r2, #1
	move.q r5, r4
	add.q r4, r4, #1
	store.b r6, (r5)
	sub.q r3, r3, #1
	bra .L114
.L117:
	li r2, #0
	store.b r2, (r4)
	move.q r1, r1
	load.q r18, 0(r31)
	load.q r19, 8(r31)
	load.q r20, 16(r31)
	add.q r31, r31, #24
	rts
.size strncat,.-strncat
.section .text,"ax"
.global strcmp
.type strcmp,@function
.align 16
strcmp:
.L119:
	load.b r3, (r1)
	sext.b r3, r3
	load.b r4, (r2)
	sext.b r4, r4
	move.l r5, r3
	move.l r6, r4
	bne r5, r6, .L122
.L120:
	beq r3, r0, .L122
.L121:
	add.q r1, r1, #1
	add.q r2, r2, #1
	bra .L119
.L122:
	and.q r1, r3, #0xff
	and.q r2, r4, #0xff
	sext.l r3, r1
	sext.l r4, r2
	blt r3, r4, .L124
.L123:
	sext.l r1, r1
	sext.l r2, r2
	li r30, #0
	ble r1, r2, .Lcmp0
	li r30, #1
.Lcmp0:
	move.q r1, r30
	bra .L125
.L124:
	li r1, #-1
.L125:
	sext.l r1, r1
	move.q r1, r1
	rts
.size strcmp,.-strcmp
.section .text,"ax"
.global strncmp
.type strncmp,@function
.align 16
strncmp:
.L127:
	li r4, #0
	beq r3, r4, .L136
.L128:
	load.b r4, (r1)
	sext.b r4, r4
	load.b r5, (r2)
	sext.b r5, r5
	move.l r6, r4
	move.l r7, r5
	bne r6, r7, .L132
.L129:
	beq r4, r0, .L131
.L130:
	add.q r2, r2, #1
	sub.q r3, r3, #1
	add.q r1, r1, #1
	bra .L127
.L131:
	li r1, #0
	rts
.L132:
	and.q r1, r4, #0xff
	and.q r2, r5, #0xff
	sext.l r1, r1
	sext.l r2, r2
	blt r1, r2, .L134
.L133:
	li r1, #1
	bra .L135
.L134:
	li r1, #-1
.L135:
	sext.l r1, r1
	move.q r1, r1
	rts
.L136:
	li r1, #0
	rts
.size strncmp,.-strncmp
.section .text,"ax"
.global strchr
.type strchr,@function
.align 16
strchr:
	sext.b r2, r2
.L138:
	load.b r3, (r1)
	sext.b r3, r3
	move.l r4, r3
	move.l r5, r2
	beq r4, r5, .L142
.L139:
	beq r3, r0, .L141
.L140:
	add.q r1, r1, #1
	bra .L138
.L141:
	li r1, #0
	rts
.L142:
	move.q r1, r1
	rts
.size strchr,.-strchr
.section .text,"ax"
.global isalpha
.type isalpha,@function
.align 16
isalpha:
	sext.l r2, r1
	li r3, #65
	sext.l r3, r3
	blt r2, r3, .L145
.L144:
	sext.l r2, r1
	li r3, #90
	sext.l r3, r3
	ble r2, r3, .L148
.L145:
	sext.l r2, r1
	li r3, #97
	sext.l r3, r3
	blt r2, r3, .L147
.L146:
	sext.l r1, r1
	li r2, #122
	sext.l r2, r2
	ble r1, r2, .L148
.L147:
	li r1, #0
	bra .L149
.L148:
	li r1, #1
.L149:
	sext.l r1, r1
	move.q r1, r1
	rts
.size isalpha,.-isalpha
.section .text,"ax"
.global isdigit
.type isdigit,@function
.align 16
isdigit:
	sext.l r2, r1
	li r3, #48
	sext.l r3, r3
	blt r2, r3, .L152
.L151:
	sext.l r1, r1
	li r2, #57
	sext.l r2, r2
	ble r1, r2, .L153
.L152:
	li r1, #0
	bra .L154
.L153:
	li r1, #1
.L154:
	sext.l r1, r1
	move.q r1, r1
	rts
.size isdigit,.-isdigit
.section .text,"ax"
.global islower
.type islower,@function
.align 16
islower:
	sext.l r2, r1
	li r3, #97
	sext.l r3, r3
	blt r2, r3, .L157
.L156:
	sext.l r1, r1
	li r2, #122
	sext.l r2, r2
	ble r1, r2, .L158
.L157:
	li r1, #0
	bra .L159
.L158:
	li r1, #1
.L159:
	sext.l r1, r1
	move.q r1, r1
	rts
.size islower,.-islower
.section .text,"ax"
.global isupper
.type isupper,@function
.align 16
isupper:
	sext.l r2, r1
	li r3, #65
	sext.l r3, r3
	blt r2, r3, .L162
.L161:
	sext.l r1, r1
	li r2, #90
	sext.l r2, r2
	ble r1, r2, .L163
.L162:
	li r1, #0
	bra .L164
.L163:
	li r1, #1
.L164:
	sext.l r1, r1
	move.q r1, r1
	rts
.size isupper,.-isupper
.section .text,"ax"
.global isalnum
.type isalnum,@function
.align 16
isalnum:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	sext.l r1, r1
	move.q r18, r1
	move.q r1, r1
	jsr isalpha
	move.q r2, r1
	move.q r1, r18
	bne r2, r0, .L168
.L166:
	jsr isdigit
	bne r1, r0, .L168
.L167:
	li r1, #0
	bra .L169
.L168:
	li r1, #1
.L169:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.size isalnum,.-isalnum
.section .text,"ax"
.global isblank
.type isblank,@function
.align 16
isblank:
	move.l r2, r1
	li r3, #32
	move.l r3, r3
	beq r2, r3, .L173
.L171:
	move.l r1, r1
	li r2, #9
	move.l r2, r2
	beq r1, r2, .L173
.L172:
	li r1, #0
	bra .L174
.L173:
	li r1, #1
.L174:
	sext.l r1, r1
	move.q r1, r1
	rts
.size isblank,.-isblank
.section .text,"ax"
.global iscntrl
.type iscntrl,@function
.align 16
iscntrl:
	sext.l r2, r1
	li r3, #32
	sext.l r3, r3
	blt r2, r3, .L178
.L176:
	move.l r1, r1
	li r2, #127
	move.l r2, r2
	beq r1, r2, .L178
.L177:
	li r1, #0
	bra .L179
.L178:
	li r1, #1
.L179:
	sext.l r1, r1
	move.q r1, r1
	rts
.size iscntrl,.-iscntrl
.section .text,"ax"
.global isprint
.type isprint,@function
.align 16
isprint:
	sext.l r2, r1
	li r3, #32
	sext.l r3, r3
	blt r2, r3, .L182
.L181:
	sext.l r1, r1
	li r2, #127
	sext.l r2, r2
	blt r1, r2, .L183
.L182:
	li r1, #0
	bra .L184
.L183:
	li r1, #1
.L184:
	sext.l r1, r1
	move.q r1, r1
	rts
.size isprint,.-isprint
.section .text,"ax"
.global isgraph
.type isgraph,@function
.align 16
isgraph:
	sext.l r2, r1
	li r3, #32
	sext.l r3, r3
	ble r2, r3, .L187
.L186:
	sext.l r1, r1
	li r2, #127
	sext.l r2, r2
	blt r1, r2, .L188
.L187:
	li r1, #0
	bra .L189
.L188:
	li r1, #1
.L189:
	sext.l r1, r1
	move.q r1, r1
	rts
.size isgraph,.-isgraph
.section .text,"ax"
.global isspace
.type isspace,@function
.align 16
isspace:
	move.l r2, r1
	li r3, #32
	move.l r3, r3
	beq r2, r3, .L194
.L191:
	sext.l r2, r1
	li r3, #9
	sext.l r3, r3
	blt r2, r3, .L193
.L192:
	sext.l r1, r1
	li r2, #13
	sext.l r2, r2
	ble r1, r2, .L194
.L193:
	li r1, #0
	bra .L195
.L194:
	li r1, #1
.L195:
	sext.l r1, r1
	move.q r1, r1
	rts
.size isspace,.-isspace
.section .text,"ax"
.global isxdigit
.type isxdigit,@function
.align 16
isxdigit:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	move.q r18, r1
	sext.l r1, r1
	jsr isdigit
	move.q r2, r1
	move.q r1, r18
	bne r2, r0, .L202
.L197:
	sext.l r2, r1
	li r3, #97
	sext.l r3, r3
	blt r2, r3, .L199
.L198:
	sext.l r2, r1
	li r3, #102
	sext.l r3, r3
	ble r2, r3, .L202
.L199:
	sext.l r2, r1
	li r3, #65
	sext.l r3, r3
	blt r2, r3, .L201
.L200:
	sext.l r1, r1
	li r2, #70
	sext.l r2, r2
	ble r1, r2, .L202
.L201:
	li r1, #0
	bra .L203
.L202:
	li r1, #1
.L203:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.size isxdigit,.-isxdigit
.section .text,"ax"
.global ispunct
.type ispunct,@function
.align 16
ispunct:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	sext.l r1, r1
	move.q r18, r1
	move.q r1, r1
	jsr isgraph
	move.q r2, r1
	move.q r1, r18
	beq r2, r0, .L207
.L205:
	jsr isalnum
	bne r1, r0, .L207
.L206:
	li r1, #1
	bra .L208
.L207:
	li r1, #0
.L208:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.size ispunct,.-ispunct
.section .text,"ax"
.global tolower
.type tolower,@function
.align 16
tolower:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	move.q r18, r1
	sext.l r1, r1
	jsr isupper
	move.q r2, r1
	move.q r1, r18
	beq r2, r0, .L211
.L210:
	add.l r1, r1, #32
.L211:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.size tolower,.-tolower
.section .text,"ax"
.global toupper
.type toupper,@function
.align 16
toupper:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	move.q r18, r1
	sext.l r1, r1
	jsr islower
	move.q r2, r1
	move.q r1, r18
	beq r2, r0, .L214
.L213:
	sub.l r1, r1, #32
.L214:
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.size toupper,.-toupper
.section .text,"ax"
.global abs
.type abs,@function
.align 16
abs:
	sext.l r2, r1
	li r3, #0
	sext.l r3, r3
	bge r2, r3, .L217
.L216:
	neg.l r1, r1
.L217:
	sext.l r1, r1
	move.q r1, r1
	rts
.size abs,.-abs
.section .text,"ax"
.global labs
.type labs,@function
.align 16
labs:
	li r2, #0
	bge r1, r2, .L220
.L219:
	neg.q r1, r1
.L220:
	move.q r1, r1
	rts
.size labs,.-labs
.section .text,"ax"
.global llabs
.type llabs,@function
.align 16
llabs:
	li r2, #0
	bge r1, r2, .L223
.L222:
	neg.q r1, r1
.L223:
	move.q r1, r1
	rts
.size llabs,.-llabs
.section .text,"ax"
.local ie64_digit
.type ie64_digit,@function
.align 16
ie64_digit:
	sub.q r31, r31, #8
	store.q r18, 0(r31)
	move.q r18, r1
	sext.l r1, r1
	jsr isdigit
	move.q r2, r1
	move.q r1, r18
	bne r2, r0, .L232
.L225:
	sext.l r2, r1
	li r3, #97
	sext.l r3, r3
	blt r2, r3, .L227
.L226:
	sext.l r2, r1
	li r3, #122
	sext.l r3, r3
	ble r2, r3, .L231
.L227:
	sext.l r2, r1
	li r3, #65
	sext.l r3, r3
	blt r2, r3, .L229
.L228:
	sext.l r2, r1
	li r3, #90
	sext.l r3, r3
	ble r2, r3, .L230
.L229:
	move.l r1, #0xffffffff
	movt r1, #0xffffffff
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.L230:
	sub.l r1, r1, #55
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.L231:
	sub.l r1, r1, #87
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.L232:
	sub.l r1, r1, #48
	sext.l r1, r1
	move.q r1, r1
	load.q r18, 0(r31)
	add.q r31, r31, #8
	rts
.size ie64_digit,.-ie64_digit
.section .text,"ax"
.local ie64_strtoull
.type ie64_strtoull,@function
.align 16
ie64_strtoull:
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
.L234:
	move.q r1, r18
.L235:
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
	beq r4, r0, .L237
.L236:
	move.q r21, r3
	move.q r20, r2
	bra .L235
.L237:
	move.q r1, r19
	move.q r21, r3
.L238:
	add.q r19, r1, #2
	bne r21, r0, .L247
.L239:
	load.b r2, (r1)
	sext.b r2, r2
	move.l r2, r2
	li r3, #48
	move.l r3, r3
	beq r2, r3, .L241
.L240:
	li r22, #10
	bra .L255
.L241:
	add.q r2, r1, #1
	load.b r2, (r2)
	sext.b r2, r2
	move.l r2, r2
	li r3, #120
	move.l r3, r3
	beq r2, r3, .L243
.L242:
	add.q r2, r1, #1
	load.b r2, (r2)
	sext.b r2, r2
	move.l r2, r2
	li r3, #88
	move.l r3, r3
	bne r2, r3, .L245
.L243:
	move.q r20, r1
	add.q r1, r1, #2
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr ie64_digit
	move.q r2, r1
	move.q r1, r20
	sext.l r2, r2
	li r3, #0
	sext.l r3, r3
	blt r2, r3, .L245
.L244:
	move.q r20, r1
	add.q r1, r1, #2
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr ie64_digit
	move.q r2, r1
	move.q r1, r20
	sext.l r2, r2
	li r3, #16
	sext.l r3, r3
	blt r2, r3, .L246
.L245:
	li r22, #8
	bra .L255
.L246:
	li r22, #16
	move.q r1, r19
	bra .L255
.L247:
	move.l r2, r21
	li r3, #16
	move.l r3, r3
	bne r2, r3, .L254
.L248:
	load.b r2, (r1)
	sext.b r2, r2
	move.l r2, r2
	li r3, #48
	move.l r3, r3
	bne r2, r3, .L254
.L249:
	add.q r2, r1, #1
	load.b r2, (r2)
	sext.b r2, r2
	move.l r2, r2
	li r3, #120
	move.l r3, r3
	beq r2, r3, .L251
.L250:
	add.q r2, r1, #1
	load.b r2, (r2)
	sext.b r2, r2
	move.l r2, r2
	li r3, #88
	move.l r3, r3
	bne r2, r3, .L254
.L251:
	move.q r20, r1
	add.q r1, r1, #2
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr ie64_digit
	move.q r2, r1
	move.q r1, r20
	sext.l r2, r2
	li r3, #0
	sext.l r3, r3
	blt r2, r3, .L254
.L252:
	move.q r20, r1
	add.q r1, r1, #2
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr ie64_digit
	move.q r2, r1
	move.q r1, r20
	sext.l r2, r2
	li r3, #16
	sext.l r3, r3
	bge r2, r3, .L254
.L253:
	move.q r1, r19
.L254:
	move.l r22, r21
.L255:
	move.l r20, r22
.L256:
	li r19, #0
.L257:
	move.q r21, r1
	load.b r1, (r1)
	sext.b r1, r1
	and.q r1, r1, #0xff
	sext.l r1, r1
	jsr ie64_digit
	move.q r5, r24
	move.q r4, r23
	move.l r3, r22
	move.q r2, r1
	move.q r1, r21
	sext.l r6, r2
	li r7, #0
	sext.l r7, r7
	blt r6, r7, .L266
.L258:
	sext.l r6, r2
	sext.l r7, r3
	bge r6, r7, .L265
.L259:
	move.l r2, r2
	sub.q r6, r4, r2
	divu.q r6, r6, r20
	bhi r19, r6, .L262
.L260:
	load.l r6, (r5)
	sext.l r6, r6
	bne r6, r0, .L263
.L261:
	mulu.q r6, r20, r19
	add.q r19, r2, r6
	bra .L263
.L262:
	li r2, #1
	store.l r2, (r5)
.L263:
	add.q r1, r1, #1
.L264:
	move.l r22, r3
	move.q r24, r5
	move.q r23, r4
	bra .L257
.L265:
	load.q r20, 0(r31)
	bra .L267
.L266:
	load.q r20, 0(r31)
.L267:
	li r2, #0
	beq r20, r2, .L271
.L268:
	bne r18, r1, .L270
.L269:
	move.q r1, r18
.L270:
	store.q r1, (r20)
.L271:
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
.size ie64_strtoull,.-ie64_strtoull
.section .text,"ax"
.global strtoull
.type strtoull,@function
.align 16
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
.L273:
	move.q r18, r21
	move.q r1, r21
.L274:
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
	beq r4, r0, .L276
.L275:
	move.q r20, r2
	bra .L274
.L276:
	move.q r30, r19
	move.q r19, r1
	move.q r1, r30
	move.q r20, r2
	move.q r21, r18
.L277:
	load.b r2, (r1)
	sext.b r2, r2
	move.l r4, r2
	li r5, #45
	move.l r5, r5
	beq r4, r5, .L281
.L278:
	move.l r2, r2
	li r4, #43
	move.l r4, r4
	beq r2, r4, .L280
.L279:
	move.q r19, r21
	li r18, #0
	bra .L283
.L280:
	move.q r1, r19
	bra .L282
.L281:
	move.q r1, r19
.L282:
	move.q r19, r21
.L283:
	move.q r21, r1
	sext.l r3, r3
	move.q r1, r21
	move.q r2, r20
	move.l r4, #0xffffffff
	movt r4, #0xffffffff
	lea r5, 0(r31)
	jsr ie64_strtoull
	li r2, #0
	beq r20, r2, .L286
.L284:
	load.q r2, (r20)
	bne r2, r21, .L286
.L285:
	store.q r19, (r20)
.L286:
	beq r18, r0, .L289
.L287:
	lea r2, 0(r31)
	load.l r2, (r2)
	sext.l r2, r2
	bne r2, r0, .L289
.L288:
	li r2, #0
	sub.q r1, r2, r1
.L289:
	lea r2, 0(r31)
	load.l r2, (r2)
	sext.l r2, r2
	beq r2, r0, .L291
.L290:
	move.l r1, #0xffffffff
	movt r1, #0xffffffff
.L291:
	move.q r1, r1
	load.q r18, 8(r31)
	load.q r19, 16(r31)
	load.q r20, 24(r31)
	load.q r21, 32(r31)
	add.q r31, r31, #40
	rts
.size strtoull,.-strtoull
.section .text,"ax"
.global strtoll
.type strtoll,@function
.align 16
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
.L293:
	move.q r18, r21
	move.q r1, r21
.L294:
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
	beq r4, r0, .L296
.L295:
	move.q r20, r2
	bra .L294
.L296:
	move.q r30, r19
	move.q r19, r1
	move.q r1, r30
	move.q r20, r2
	move.q r21, r18
.L297:
	load.b r2, (r1)
	sext.b r2, r2
	move.l r4, r2
	li r5, #45
	move.l r5, r5
	beq r4, r5, .L301
.L298:
	move.l r2, r2
	li r4, #43
	move.l r4, r4
	beq r2, r4, .L300
.L299:
	move.q r19, r21
	li r18, #0
	bra .L303
.L300:
	move.q r1, r19
	bra .L302
.L301:
	move.q r1, r19
.L302:
	move.q r19, r21
.L303:
	move.q r21, r1
	bne r18, r0, .L305
.L304:
	move.l r4, #0xffffffff
	movt r4, #0x7fffffff
	bra .L306
.L305:
	move.l r4, #0x00000000
	movt r4, #0x80000000
.L306:
	sext.l r3, r3
	move.q r1, r21
	move.q r2, r20
	lea r5, 0(r31)
	jsr ie64_strtoull
	li r2, #0
	beq r20, r2, .L309
.L307:
	load.q r2, (r20)
	bne r2, r21, .L309
.L308:
	store.q r19, (r20)
.L309:
	lea r2, 0(r31)
	load.l r2, (r2)
	sext.l r2, r2
	bne r2, r0, .L316
.L310:
	bne r18, r0, .L312
.L311:
	move.q r1, r1
	load.q r18, 8(r31)
	load.q r19, 16(r31)
	load.q r20, 24(r31)
	load.q r21, 32(r31)
	add.q r31, r31, #40
	rts
.L312:
	move.l r2, #0x00000000
	movt r2, #0x80000000
	beq r1, r2, .L314
.L313:
	neg.q r1, r1
	bra .L315
.L314:
	move.l r1, #0x00000000
	movt r1, #0x80000000
.L315:
	move.q r1, r1
	load.q r18, 8(r31)
	load.q r19, 16(r31)
	load.q r20, 24(r31)
	load.q r21, 32(r31)
	add.q r31, r31, #40
	rts
.L316:
	bne r18, r0, .L318
.L317:
	move.l r1, #0xffffffff
	movt r1, #0x7fffffff
	bra .L319
.L318:
	move.l r1, #0x00000000
	movt r1, #0x80000000
.L319:
	move.q r1, r1
	load.q r18, 8(r31)
	load.q r19, 16(r31)
	load.q r20, 24(r31)
	load.q r21, 32(r31)
	add.q r31, r31, #40
	rts
.size strtoll,.-strtoll
.section .text,"ax"
.global strtoul
.type strtoul,@function
.align 16
strtoul:
	sub.q r31, r31, #8
	sext.l r3, r3
	jsr strtoull
	move.q r1, r1
	add.q r31, r31, #8
	rts
.size strtoul,.-strtoul
.section .text,"ax"
.global strtol
.type strtol,@function
.align 16
strtol:
	sub.q r31, r31, #8
	sext.l r3, r3
	jsr strtoll
	move.q r1, r1
	add.q r31, r31, #8
	rts
.size strtol,.-strtol
.section .text,"ax"
.local ie64_swap
.type ie64_swap,@function
.align 16
ie64_swap:
.L323:
	li r4, #0
	beq r3, r4, .L325
.L324:
	load.b r5, (r1)
	load.b r6, (r2)
	move.q r4, r1
	add.q r1, r1, #1
	store.b r6, (r4)
	move.q r4, r2
	add.q r2, r2, #1
	store.b r5, (r4)
	sub.q r3, r3, #1
	bra .L323
.L325:
	rts
.size ie64_swap,.-ie64_swap
.section .text,"ax"
.global qsort
.type qsort,@function
.align 16
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
	bhi r3, r2, .L338
.L327:
	li r3, #0
	beq r24, r3, .L338
.L328:
	li r18, #1
.L329:
	bls r2, r18, .L338
.L330:
	move.q r19, r18
.L331:
	move.q r20, r1
	li r1, #0
	beq r18, r1, .L336
.L332:
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
	ble r4, r5, .L335
.L333:
	move.q r21, r3
	move.q r3, r3
	jsr ie64_swap
	move.q r4, r22
	move.q r3, r21
	move.q r1, r20
.L334:
	move.q r22, r4
	move.q r24, r3
	bra .L331
.L335:
	move.q r24, r3
	load.q r2, 0(r31)
	bra .L337
.L336:
	load.q r2, 0(r31)
.L337:
	move.q r18, r19
	move.q r1, r20
	move.q r19, r18
	add.q r18, r18, #1
	bra .L329
.L338:
	load.q r18, 16(r31)
	load.q r19, 24(r31)
	load.q r20, 32(r31)
	load.q r21, 40(r31)
	load.q r22, 48(r31)
	load.q r23, 56(r31)
	load.q r24, 64(r31)
	add.q r31, r31, #72
	rts
.size qsort,.-qsort
.section .text,"ax"
.global bsearch
.type bsearch,@function
.align 16
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
.L340:
	move.q r22, r3
	li r3, #0
	beq r22, r3, .L347
.L341:
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
	beq r6, r0, .L346
.L342:
	sext.l r6, r6
	li r7, #0
	sext.l r7, r7
	blt r6, r7, .L344
.L343:
	add.q r6, r18, #1
	mulu.q r7, r4, r6
	add.q r2, r7, r2
	sub.q r3, r3, r6
	bra .L345
.L344:
	move.q r3, r18
.L345:
	move.q r24, r5
	move.q r23, r4
	bra .L340
.L346:
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
.L347:
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
.size bsearch,.-bsearch
.section .text,"ax"
.global __ie64_clz32
.type __ie64_clz32,@function
.align 16
__ie64_clz32:
.L349:
	li r2, #0
	li r3, #-2147483648
.L350:
	beq r3, r0, .L353
.L351:
	and.l r4, r1, r3
	bne r4, r0, .L353
.L352:
	add.l r2, r2, #1
	lsr.l r3, r3, #1
	bra .L350
.L353:
	sext.l r1, r2
	move.q r1, r1
	rts
.size __ie64_clz32,.-__ie64_clz32
.section .text,"ax"
.global __ie64_clz64
.type __ie64_clz64,@function
.align 16
__ie64_clz64:
.L355:
	li r2, #0
	move.l r3, #0x00000000
	movt r3, #0x80000000
.L356:
	li r4, #0
	beq r3, r4, .L359
.L357:
	and.q r4, r1, r3
	li r5, #0
	bne r4, r5, .L359
.L358:
	add.l r2, r2, #1
	lsr.q r3, r3, #1
	bra .L356
.L359:
	sext.l r1, r2
	move.q r1, r1
	rts
.size __ie64_clz64,.-__ie64_clz64
.section .data,"aw"
.local ie64_heap_limit
.type ie64_heap_limit,@object
.align 8
ie64_heap_limit:
	dc.q 585728
.size ie64_heap_limit,8
.section .bss,"aw"
.local ie64_heap_cursor
.type ie64_heap_cursor,@object
.align 8
ie64_heap_cursor:
	ds.b 8
.size ie64_heap_cursor,8
