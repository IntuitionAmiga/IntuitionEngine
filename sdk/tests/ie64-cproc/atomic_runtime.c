#include <stdint.h>
#include <intuitionengine.h>

#define RESULT (*(volatile uint64_t *)(uintptr_t)0x80000)
#define PASS 0x41544f4d50415353UL

static _Atomic unsigned int value;

int
main(void)
{
	__builtin_ie64_mtcr(IE64_CR_TIMER_CTRL, 0);
	value = 7;
	if (__builtin_ie64_mfcr(IE64_CR_TIMER_CTRL) != 0)
		return 3;
	__builtin_ie64_mtcr(IE64_CR_TIMER_CTRL, 2);
	if (value++ != 7 || value != 8)
		return 1;
	if (__builtin_ie64_mfcr(IE64_CR_TIMER_CTRL) != 2)
		return 4;
	__builtin_ie64_mtcr(IE64_CR_TIMER_CTRL, 0);
	value += 4;
	if (value != 12)
		return 2;
	RESULT = PASS;
	return 0;
}
