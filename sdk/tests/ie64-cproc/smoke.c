#include <stdint.h>

int main(void)
{
	volatile uint64_t *result = (volatile uint64_t *)0x00080000;
	*result = 0x494536344350524fUL;
	return 0;
}
