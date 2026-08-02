#include <assert.h>

int
assert_enabled(void)
{
	int evaluations = 0;
	assert(++evaluations == 1);
	return evaluations;
}
