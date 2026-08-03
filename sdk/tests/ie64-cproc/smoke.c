#include <stdint.h>

#define RESULT ((volatile uint64_t *)0x00080000)

static volatile unsigned int lifecycle_state;

void smoke_preinit(void)
{
	lifecycle_state = 1;
}

void smoke_init(void)
{
	lifecycle_state = lifecycle_state == 1 ? 2 : 0xff;
}

void smoke_fini(void)
{
	*RESULT = lifecycle_state == 3 ? 0x494536344350524fUL : 0x46494e494641494cUL;
}

int main(void)
{
	*RESULT = 0x494e49544641494cUL;
	if (lifecycle_state == 2)
		lifecycle_state = 3;
	return 0;
}
