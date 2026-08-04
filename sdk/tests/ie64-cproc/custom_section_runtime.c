#include <stdint.h>

#define RESULT ((volatile uint64_t *)0x80000)

static uint64_t setting __attribute__((section(".settings"))) = 41;

static uint64_t hot_function(void) __attribute__((section(".fastcode")));

static uint64_t
hot_function(void)
{
	return setting + 1;
}

int
main(void)
{
	if (hot_function() != 42) {
		*RESULT = 1;
		return 1;
	}
	*RESULT = 0x53454354494f4e53UL;
	return 0;
}
