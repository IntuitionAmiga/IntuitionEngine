#ifndef IE64_ATOMIC_ORDERS_H
#define IE64_ATOMIC_ORDERS_H

#include <stdint.h>

static inline int
ie64_atomic_compare_exchange_orders_valid(uint64_t success, uint64_t failure)
{
	if (success > 5 || failure > 5 || failure == 3 || failure == 4)
		return 0;
	switch (success) {
	case 0:
		return failure == 0;
	case 1:
		return failure <= 1;
	case 2:
		return failure <= 2;
	case 3:
		return failure == 0;
	case 4:
		return failure <= 2;
	case 5:
		return failure <= 2 || failure == 5;
	}
	return 0;
}

#endif
