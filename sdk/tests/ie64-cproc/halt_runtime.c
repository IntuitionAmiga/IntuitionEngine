#include <intuitionengine.h>
#include <stdint.h>

int
main(void)
{
	*(volatile uint64_t *)0x80000 = 0x48414c5450415353UL;
	__builtin_ie64_halt();
}
