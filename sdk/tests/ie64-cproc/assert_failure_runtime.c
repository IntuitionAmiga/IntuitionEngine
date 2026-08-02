#include <assert.h>
#include <stdint.h>

int
main(void)
{
	*(volatile uint64_t *)0x80000 = 0x4153534552544641UL;
	assert(0);
}
