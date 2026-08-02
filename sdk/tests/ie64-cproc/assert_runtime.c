#include <stdint.h>

int assert_enabled(void);
int assert_disabled(void);

int
main(void)
{
	if (assert_enabled() != 1 || assert_disabled() != 0)
		return 1;
	*(volatile uint64_t *)0x80000 = 0x4153534552544f4bUL;
	return 0;
}
