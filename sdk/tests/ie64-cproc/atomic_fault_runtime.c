#include <intuitionengine.h>
#include <stdint.h>

#ifndef FAULT_ADDRESS
#error FAULT_ADDRESS is required
#endif

#define RESULT ((volatile uint64_t *)0x80000)

static _Noreturn void
fault_handler(void)
{
	uint64_t cause = __builtin_ie64_mfcr(IE64_CR_FAULT_CAUSE);
	uint64_t address = __builtin_ie64_mfcr(IE64_CR_FAULT_ADDR);
	if (cause == 7 && address == FAULT_ADDRESS)
		*RESULT = 0x41544f4d4641554cUL;
	else
		*RESULT = (cause << 32) | address;
	__builtin_ie64_halt();
}

int
main(void)
{
	ie64_disable_interrupts();
	__builtin_ie64_mtcr(IE64_CR_TRAP_VEC, (uintptr_t)fault_handler);
	(void)__builtin_ie64_xchg((volatile uint64_t *)FAULT_ADDRESS, 1);
	return 1;
}
