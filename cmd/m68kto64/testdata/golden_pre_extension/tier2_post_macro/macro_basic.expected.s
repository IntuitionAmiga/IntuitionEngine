; Tier-2 fixture: macro/endm/rept/endr — Phase A→D byte-identity guard.
; Expected to drift in Phase E when macros are transpile-time-expanded.



start:
	sub.l r30, r30, #4
	store.l r15, (r30)
	sub.l r30, r30, #4
	store.l r14, (r30)
	sub.l r30, r30, #4
	store.l r13, (r30)
	sub.l r30, r30, #4
	store.l r12, (r30)
	sub.l r30, r30, #4
	store.l r11, (r30)
	sub.l r30, r30, #4
	store.l r10, (r30)
	sub.l r30, r30, #4
	store.l r9, (r30)
	sub.l r30, r30, #4
	store.l r8, (r30)
	sub.l r30, r30, #4
	store.l r7, (r30)
	sub.l r30, r30, #4
	store.l r6, (r30)
	sub.l r30, r30, #4
	store.l r5, (r30)
	sub.l r30, r30, #4
	store.l r4, (r30)
	sub.l r30, r30, #4
	store.l r3, (r30)
	sub.l r30, r30, #4
	store.l r2, (r30)
	sub.l r30, r30, #4
	store.l r1, (r30)
	; nop (m68k) — no IE64 output (transparent)
	; nop (m68k) — no IE64 output (transparent)
	; nop (m68k) — no IE64 output (transparent)
	load.l r1, (r30)
	add.l r30, r30, #4
	load.l r2, (r30)
	add.l r30, r30, #4
	load.l r3, (r30)
	add.l r30, r30, #4
	load.l r4, (r30)
	add.l r30, r30, #4
	load.l r5, (r30)
	add.l r30, r30, #4
	load.l r6, (r30)
	add.l r30, r30, #4
	load.l r7, (r30)
	add.l r30, r30, #4
	load.l r8, (r30)
	add.l r30, r30, #4
	load.l r9, (r30)
	add.l r30, r30, #4
	load.l r10, (r30)
	add.l r30, r30, #4
	load.l r11, (r30)
	add.l r30, r30, #4
	load.l r12, (r30)
	add.l r30, r30, #4
	load.l r13, (r30)
	add.l r30, r30, #4
	load.l r14, (r30)
	add.l r30, r30, #4
	load.l r15, (r30)
	add.l r30, r30, #4
	load.l r17, (r30)
	add.l r30, r30, #4
	jmp (r17)

