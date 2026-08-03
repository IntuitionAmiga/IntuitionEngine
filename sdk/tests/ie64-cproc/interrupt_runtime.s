.section .interrupt_vector
.global interrupt_handler
interrupt_handler:
	li r1, #0x80000
	li r2, #0x494e545250415353
	store.q r2, (r1)
	halt
