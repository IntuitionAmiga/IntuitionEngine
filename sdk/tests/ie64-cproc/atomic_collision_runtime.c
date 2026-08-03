#include <stdatomic.h>
#include <stdint.h>

#define RESULT (*(volatile uint64_t *)(uintptr_t)0x80000)
#define PASS 0x41544f4d434f4c4cUL

static struct {
	atomic_ullong first;
	unsigned char gap[512 - sizeof(atomic_ullong)];
	atomic_ullong second;
} colliding;
static const _Atomic unsigned long long readonly_value = 37;

int
main(void)
{
	atomic_store(&colliding.first, 11);
	atomic_store(&colliding.second, 29);
	if (atomic_load(&colliding.first) != 11
	|| atomic_load(&colliding.second) != 29
	|| atomic_load(&readonly_value) != 37)
		return 1;
	RESULT = PASS;
	return 0;
}
