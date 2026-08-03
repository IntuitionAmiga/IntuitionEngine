#include <stdint.h>
#include <stdio.h>

#include "../../lib/ie64-cproc/atomic_orders.h"

int
main(void)
{
	static const unsigned char valid[6][6] = {
		{1, 0, 0, 0, 0, 0},
		{1, 1, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0},
		{1, 0, 0, 0, 0, 0},
		{1, 1, 1, 0, 0, 0},
		{1, 1, 1, 0, 0, 1},
	};
	uint64_t success, failure;

	for (success = 0; success < 7; ++success)
		for (failure = 0; failure < 7; ++failure)
			if (ie64_atomic_compare_exchange_orders_valid(success, failure)
			    != (success < 6 && failure < 6 && valid[success][failure])) {
				fprintf(stderr, "order matrix mismatch: success=%llu failure=%llu\n",
				    (unsigned long long)success, (unsigned long long)failure);
				return 1;
			}
	return 0;
}
