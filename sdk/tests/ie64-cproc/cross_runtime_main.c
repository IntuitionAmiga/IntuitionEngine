#include <stdint.h>

#define RESULT ((volatile uint64_t *)0x80000)

long cross_unit_a(void);
long cross_unit_b(void);
struct cross_pair { long integer; double real; };
struct cross_pair cross_make_pair(long, double);
long cross_read_pair(struct cross_pair);

int
main(void)
{
	long (*call_a)(void) = cross_unit_a;
	struct cross_pair pair = cross_make_pair(7, 4.5);
	if (call_a() != 208 || cross_unit_b() != 320
	|| cross_read_pair(pair) != 17 || pair.integer != 7) {
		*RESULT = 1;
		return 1;
	}
	*RESULT = 0x43524f5353504153UL;
	return 0;
}
