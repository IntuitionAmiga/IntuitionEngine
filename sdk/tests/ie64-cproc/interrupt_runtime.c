#include <ie64.h>

#define RESULT ((volatile uint64_t *)0x80000)

int
main(void)
{
	*RESULT = 0x494e54524641494cUL;
	ie64_disable_interrupts();
	__builtin_ie64_mtcr(IE64_CR_TIMER_PERIOD, 1);
	__builtin_ie64_mtcr(IE64_CR_TIMER_COUNT, 1);
	__builtin_ie64_mtcr(IE64_CR_TIMER_CTRL, 1);
	ie64_enable_interrupts();
	for (;;)
		ie64_nop();
}
