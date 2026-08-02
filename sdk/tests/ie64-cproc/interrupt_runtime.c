#include <ie64.h>
#include <stdint.h>

#define RESULT ((volatile uint64_t *)0x80000)

static _Noreturn void
interrupt_handler(void)
{
	*RESULT = 0x494e545250415353UL;
	__builtin_ie64_halt();
}

int
main(void)
{
	__builtin_ie64_mtcr(IE64_CR_TRAP_VEC, (uintptr_t)interrupt_handler);
	*RESULT = 0x494e545250415353UL;
	ie64_enable_interrupts();
	ie64_disable_interrupts();
	return 0;
}
