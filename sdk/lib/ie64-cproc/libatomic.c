#include <stddef.h>
#include <stdint.h>
#include <intuitionengine.h>
#include "atomic_orders.h"

/* This table and hash are part of the IE64 bare-metal ABI V3. */
alignas(8) volatile uint64_t __ie64_atomic_locks[64];
alignas(8) static volatile uint64_t __ie64_atomic_fence_word;

static volatile uint64_t *atomic_lock_for(const volatile void *object)
{
	uintptr_t address = (uintptr_t)object;
	return &__ie64_atomic_locks[(address >> 3) & 63];
}

static uint64_t atomic_lock(volatile uint64_t *lock)
{
	uint64_t timer_control = __builtin_ie64_mfcr(IE64_CR_TIMER_CTRL);
	__builtin_ie64_cli();
	while (__builtin_ie64_xchg(lock, 1) != 0) {
	}
	return timer_control;
}

static void atomic_unlock(volatile uint64_t *lock, uint64_t timer_control)
{
	(void)__builtin_ie64_xchg(lock, 0);
	__builtin_ie64_mtcr(IE64_CR_TIMER_CTRL, timer_control);
}

static void atomic_copy_out(void *destination, const volatile void *source,
			    size_t size, uint64_t is_volatile)
{
	size_t i;
	unsigned char *out = destination;
	if (is_volatile) {
		const volatile unsigned char *in = source;
		for (i = 0; i < size; ++i)
			out[i] = in[i];
	} else {
		const unsigned char *in = (const unsigned char *)source;
		for (i = 0; i < size; ++i)
			out[i] = in[i];
	}
}

static void atomic_copy_in(volatile void *destination, const void *source,
			   size_t size, uint64_t is_volatile)
{
	size_t i;
	const unsigned char *in = source;
	if (is_volatile) {
		volatile unsigned char *out = destination;
		for (i = 0; i < size; ++i)
			out[i] = in[i];
	} else {
		unsigned char *out = (unsigned char *)destination;
		for (i = 0; i < size; ++i)
			out[i] = in[i];
	}
}

void __ie64_atomic_load(const volatile void *object, void *result, size_t size,
			uint64_t is_volatile)
{
	volatile uint64_t *lock = atomic_lock_for(object);
	uint64_t timer_control = atomic_lock(lock);
	atomic_copy_out(result, object, size, is_volatile);
	atomic_unlock(lock, timer_control);
}

void __ie64_atomic_store(volatile void *object, const void *value, size_t size,
			 uint64_t is_volatile)
{
	volatile uint64_t *lock = atomic_lock_for(object);
	uint64_t timer_control = atomic_lock(lock);
	atomic_copy_in(object, value, size, is_volatile);
	atomic_unlock(lock, timer_control);
}

void __ie64_atomic_exchange(volatile void *object, const void *desired,
			    void *old, size_t size, uint64_t is_volatile)
{
	volatile uint64_t *lock = atomic_lock_for(object);
	uint64_t timer_control = atomic_lock(lock);
	atomic_copy_out(old, object, size, is_volatile);
	atomic_copy_in(object, desired, size, is_volatile);
	atomic_unlock(lock, timer_control);
}

int __ie64_atomic_compare_exchange(volatile void *object, void *expected,
				   const void *desired, size_t size,
				   uint64_t is_volatile)
{
	volatile uint64_t *lock = atomic_lock_for(object);
	uint64_t timer_control;
	unsigned char actual;
	unsigned char wanted;
	size_t i;
	int equal = 1;
	timer_control = atomic_lock(lock);
	for (i = 0; i < size; ++i) {
		atomic_copy_out(&actual, (const volatile unsigned char *)object + i,
				1, is_volatile);
		wanted = ((const unsigned char *)expected)[i];
		if (actual != wanted)
			equal = 0;
	}
	if (equal)
		atomic_copy_in(object, desired, size, is_volatile);
	else
		atomic_copy_out(expected, object, size, is_volatile);
	atomic_unlock(lock, timer_control);
	return equal;
}

void __ie64_atomic_fetch(volatile void *object, const void *operand, void *old,
			 size_t size, uint64_t is_volatile, uint64_t operation)
{
	volatile uint64_t *lock = atomic_lock_for(object);
	uint64_t timer_control;
	uint64_t left = 0;
	uint64_t right = 0;
	uint64_t result;
	uint64_t mask;
	int64_t signed_left, signed_right;
	float left_float, right_float, result_float;
	double left_double, right_double, result_double;
	if (size == 0 || size > sizeof left)
		__ie64_terminate(1);
	timer_control = atomic_lock(lock);
	atomic_copy_out(&left, object, size, is_volatile);
	atomic_copy_out(old, object, size, is_volatile);
	atomic_copy_out(&right, operand, size, 0);
	if (operation == 5 || operation == 6 || operation == 15 || operation == 16) {
		if (size == sizeof(float)) {
			atomic_copy_out(&left_float, object, size, is_volatile);
			atomic_copy_out(&right_float, operand, size, 0);
			if (operation == 5) result_float = left_float + right_float;
			else if (operation == 6) result_float = left_float - right_float;
			else if (operation == 15) result_float = left_float * right_float;
			else result_float = left_float / right_float;
			atomic_copy_in(object, &result_float, size, is_volatile);
		} else if (size == sizeof(double)) {
			atomic_copy_out(&left_double, object, size, is_volatile);
			atomic_copy_out(&right_double, operand, size, 0);
			if (operation == 5) result_double = left_double + right_double;
			else if (operation == 6) result_double = left_double - right_double;
			else if (operation == 15) result_double = left_double * right_double;
			else result_double = left_double / right_double;
			atomic_copy_in(object, &result_double, size, is_volatile);
		} else {
			atomic_unlock(lock, timer_control);
			__ie64_terminate(1);
		}
		atomic_unlock(lock, timer_control);
		return;
	}
	switch (operation) {
	case 0: result = left + right; break;
	case 1: result = left - right; break;
	case 2: result = left | right; break;
	case 3: result = left ^ right; break;
	case 4: result = left & right; break;
	case 7: result = left * right; break;
	case 8: result = left / right; break;
	case 10: result = left % right; break;
	case 12: result = left << right; break;
	case 13: result = left >> right; break;
	case 9:
	case 11:
	case 14:
		mask = size == sizeof result ? UINT64_MAX : (1ull << (size * 8)) - 1;
		signed_left = (int64_t)(left << (64 - size * 8)) >> (64 - size * 8);
		signed_right = (int64_t)(right << (64 - size * 8)) >> (64 - size * 8);
		if (operation == 9) result = (uint64_t)(signed_left / signed_right);
		else if (operation == 11) result = (uint64_t)(signed_left % signed_right);
		else result = (uint64_t)(signed_left >> signed_right);
		break;
	default:
		atomic_unlock(lock, timer_control);
		__ie64_terminate(1);
	}
	mask = size == sizeof result ? UINT64_MAX : (1ull << (size * 8)) - 1;
	result &= mask;
	atomic_copy_in(object, &result, size, is_volatile);
	atomic_unlock(lock, timer_control);
}

void __ie64_atomic_validate_orders(uint64_t success, uint64_t failure)
{
	if (!ie64_atomic_compare_exchange_orders_valid(success, failure))
		__ie64_terminate(1);
}

void __ie64_atomic_signal_fence(void)
{
	/* The external call is the compiler barrier. */
}

void __ie64_atomic_thread_fence(void)
{
	(void)__builtin_ie64_faa(&__ie64_atomic_fence_word, 0);
}

int __ie64_atomic_is_lock_free(size_t size, const volatile void *object)
{
	(void)size;
	(void)object;
	return 0;
}
