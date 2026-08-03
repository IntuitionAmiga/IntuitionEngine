#include <stdatomic.h>
#include <stdint.h>

#define RESULT (*(volatile uint64_t *)(uintptr_t)0x80000)
#define PASS 0x41544f4d494e5446UL

static atomic_uint value;
static atomic_flag flag = ATOMIC_FLAG_INIT;

int
main(void)
{
	unsigned int expected;
	atomic_init(&value, 4);
	if (atomic_load(&value) != 4 || atomic_exchange(&value, 8) != 4)
		return 1;
	if (atomic_fetch_add(&value, 3) != 8 || atomic_load(&value) != 11)
		return 2;
	expected = 11;
	if (!atomic_compare_exchange_strong(&value, &expected, 19))
		return 3;
	expected = 7;
	if (atomic_compare_exchange_weak(&value, &expected, 23) || expected != 19)
		return 4;
	if (atomic_flag_test_and_set(&flag) || !atomic_flag_test_and_set(&flag))
		return 5;
	atomic_flag_clear(&flag);
	atomic_signal_fence(memory_order_seq_cst);
	atomic_thread_fence(memory_order_seq_cst);
	RESULT = PASS;
	return 0;
}
