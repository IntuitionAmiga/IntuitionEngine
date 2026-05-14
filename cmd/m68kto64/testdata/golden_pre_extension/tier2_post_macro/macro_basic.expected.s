; Tier-2 fixture: macro/endm/rept/endr — Phase A→D byte-identity guard.
; Expected to drift in Phase E when macros are transpile-time-expanded.



start:
	sub.l r30, r30, #4
	bswap.l r20, r15
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r14
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r13
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r12
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r11
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r10
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r9
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r8
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r7
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r6
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r5
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r4
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r3
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r2
	store.l r20, (r30)
	sub.l r30, r30, #4
	bswap.l r20, r1
	store.l r20, (r30)
	; nop (m68k) — no IE64 output (transparent)
	; nop (m68k) — no IE64 output (transparent)
	; nop (m68k) — no IE64 output (transparent)
	load.l r1, (r30)
	bswap.l r1, r1
	add.l r30, r30, #4
	load.l r2, (r30)
	bswap.l r2, r2
	add.l r30, r30, #4
	load.l r3, (r30)
	bswap.l r3, r3
	add.l r30, r30, #4
	load.l r4, (r30)
	bswap.l r4, r4
	add.l r30, r30, #4
	load.l r5, (r30)
	bswap.l r5, r5
	add.l r30, r30, #4
	load.l r6, (r30)
	bswap.l r6, r6
	add.l r30, r30, #4
	load.l r7, (r30)
	bswap.l r7, r7
	add.l r30, r30, #4
	load.l r8, (r30)
	bswap.l r8, r8
	add.l r30, r30, #4
	load.l r9, (r30)
	bswap.l r9, r9
	add.l r30, r30, #4
	load.l r10, (r30)
	bswap.l r10, r10
	add.l r30, r30, #4
	load.l r11, (r30)
	bswap.l r11, r11
	add.l r30, r30, #4
	load.l r12, (r30)
	bswap.l r12, r12
	add.l r30, r30, #4
	load.l r13, (r30)
	bswap.l r13, r13
	add.l r30, r30, #4
	load.l r14, (r30)
	bswap.l r14, r14
	add.l r30, r30, #4
	load.l r15, (r30)
	bswap.l r15, r15
	add.l r30, r30, #4
	load.l r17, (r30)
	bswap.l r17, r17
	add.l r30, r30, #4
	jmp (r17)

