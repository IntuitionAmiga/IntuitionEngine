abi_narrow:
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
