#define NDEBUG
#include <assert.h>

int
assert_disabled(void)
{
	int evaluations = 0;
	assert(++evaluations == 1);
	return evaluations;
}
