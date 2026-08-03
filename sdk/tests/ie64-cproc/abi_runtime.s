.global abi_narrow
.global abi_fp
.global abi_spilled_float
.global abi_call_c_spilled_float
.global abi_call_c_spilled_double
.global abi_call_variadic_overflow

abi_narrow:
	and.q r28, r31, #15
	li r29, #8
	bne r28, r29, abi_narrow_fail
	load.q r28, 0(r31)
	beq r28, r0, abi_narrow_fail
	li r29, #-1
	bne r1, r29, abi_narrow_fail
	li r29, #255
	bne r2, r29, abi_narrow_fail
	li r29, #-2
	bne r3, r29, abi_narrow_fail
	li r29, #65535
	bne r4, r29, abi_narrow_fail
	li r29, #-3
	bne r5, r29, abi_narrow_fail
	li r29, #0xffffffff
	bne r6, r29, abi_narrow_fail
	load.q r28, 8(r31)
	li r29, #-4
	bne r28, r29, abi_narrow_fail
	load.q r28, 16(r31)
	li r29, #254
	bne r28, r29, abi_narrow_fail
	load.q r28, 24(r31)
	li r29, #-5
	bne r28, r29, abi_narrow_fail
	load.q r28, 32(r31)
	li r29, #65534
	bne r28, r29, abi_narrow_fail
	load.q r28, 40(r31)
	li r29, #-6
	bne r28, r29, abi_narrow_fail
	load.q r28, 48(r31)
	li r29, #0xfffffffe
	bne r28, r29, abi_narrow_fail
	load.q r28, 56(r31)
	li r29, #1
	bne r28, r29, abi_narrow_fail
	li r1, #1
	rts
abi_narrow_fail:
	li r1, #0
	rts

abi_fp:
	move.q r29, #0x81000
	fstore f0, 0(r29)
	fstore f1, 4(r29)
	fstore f2, 8(r29)
	fstore f3, 12(r29)
	fstore f4, 16(r29)
	fstore f5, 20(r29)
	fstore f6, 24(r29)
	fstore f7, 28(r29)
	dload f8, 8(r31)
	dstore f8, 64(r29)
	li r1, #1
	rts

abi_spilled_float:
	load.l r28, 8(r31)
	li r29, #0x41100000
	bne r28, r29, abi_spilled_float_fail
	li r1, #1
	rts
abi_spilled_float_fail:
	li r1, #0
	rts

abi_call_c_spilled_float:
	sub.q r31, r31, #24
	li r28, #0x3f800000
	fmovi f0, r28
	fmovi f1, r28
	fmovi f2, r28
	fmovi f3, r28
	fmovi f4, r28
	fmovi f5, r28
	fmovi f6, r28
	fmovi f7, r28
	li r28, #0x41100000
	store.l r28, 0(r31)
	jsr abi_receive_spilled_float
	add.q r31, r31, #24
	rts

abi_call_c_spilled_double:
	sub.q r31, r31, #24
	li r28, #0x3f800000
	fmovi f0, r28
	fmovi f1, r28
	fmovi f2, r28
	fmovi f3, r28
	fmovi f4, r28
	fmovi f5, r28
	fmovi f6, r28
	li r28, #0x41000000
	fmovi f7, r28
	li r28, #0x401c000000000000
	store.q r28, 0(r31)
	jsr abi_receive_spilled_double
	add.q r31, r31, #24
	rts

abi_call_variadic_overflow:
	sub.q r31, r31, #40
	li r1, #1
	move.l r2, #lo32(abi_variadic_pair)
	movt r2, #hi32(abi_variadic_pair)
	li r3, #2
	li r4, #3
	li r5, #4
	li r6, #5
	li r28, #6
	store.q r28, 0(r31)
	li r28, #7
	store.q r28, 8(r31)
	move.l r28, #lo32(abi_variadic_wide)
	movt r28, #hi32(abi_variadic_wide)
	store.q r28, 16(r31)
	li r28, #8
	store.q r28, 24(r31)
	li r28, #0
	fmovi f0, r28
	li r28, #0x40180000
	fmovi f1, r28
	jsr read_named_overflow
	add.q r31, r31, #40
	rts

align 8
abi_variadic_pair:
	dc.l 7
	ds.b 4
	dc.q 0x4020000000000000
align 8
abi_variadic_wide:
	dc.q 11
	dc.b 12
	ds.b 1
	dc.w 13
	ds.b 4
