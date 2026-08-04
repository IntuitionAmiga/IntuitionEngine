#ifndef _STDATOMIC_H
#define _STDATOMIC_H

#include <stddef.h>
#include <stdint.h>

uint64_t __builtin_ie64_xchg(volatile uint64_t *, uint64_t);

typedef enum {
	memory_order_relaxed,
	memory_order_consume,
	memory_order_acquire,
	memory_order_release,
	memory_order_acq_rel,
	memory_order_seq_cst
} memory_order;

typedef struct {
	alignas(8) volatile uint64_t __value;
} atomic_flag;

#define ATOMIC_FLAG_INIT { 0 }
#define ATOMIC_VAR_INIT(value) (value)

typedef _Atomic _Bool atomic_bool;
typedef _Atomic char atomic_char;
typedef _Atomic signed char atomic_schar;
typedef _Atomic unsigned char atomic_uchar;
typedef _Atomic short atomic_short;
typedef _Atomic unsigned short atomic_ushort;
typedef _Atomic int atomic_int;
typedef _Atomic unsigned int atomic_uint;
typedef _Atomic long atomic_long;
typedef _Atomic unsigned long atomic_ulong;
typedef _Atomic long long atomic_llong;
typedef _Atomic unsigned long long atomic_ullong;
typedef _Atomic uintptr_t atomic_uintptr_t;
typedef _Atomic size_t atomic_size_t;
typedef _Atomic ptrdiff_t atomic_ptrdiff_t;

void __ie64_atomic_load(const volatile void *, void *, size_t, uint64_t);
void __ie64_atomic_store(volatile void *, const void *, size_t, uint64_t);
void __ie64_atomic_exchange(volatile void *, const void *, void *, size_t,
	uint64_t);
int __ie64_atomic_compare_exchange(volatile void *, void *, const void *,
	size_t, uint64_t);
void __ie64_atomic_fetch(volatile void *, const void *, void *, size_t,
	uint64_t, uint64_t);
void __ie64_atomic_validate_orders(memory_order, memory_order);
void __ie64_atomic_signal_fence(void);
void __ie64_atomic_thread_fence(void);
int __ie64_atomic_is_lock_free(size_t, const volatile void *);

#define atomic_init(object, desired) atomic_store_explicit((object), (desired), memory_order_relaxed)
#define atomic_load(object) atomic_load_explicit((object), memory_order_seq_cst)
#define atomic_store(object, desired) atomic_store_explicit((object), (desired), memory_order_seq_cst)
#define atomic_exchange(object, desired) atomic_exchange_explicit((object), (desired), memory_order_seq_cst)
#define atomic_compare_exchange_strong(object, expected, desired) \
	atomic_compare_exchange_strong_explicit((object), (expected), (desired), memory_order_seq_cst, memory_order_seq_cst)
#define atomic_compare_exchange_weak atomic_compare_exchange_strong

#define atomic_load_explicit(object, order) \
	__builtin_ie64_atomic_load((object), (order))
#define atomic_store_explicit(object, desired, order) \
	__builtin_ie64_atomic_store((object), (desired), (order))
#define atomic_exchange_explicit(object, desired, order) \
	__builtin_ie64_atomic_exchange((object), (desired), (order))
#define atomic_compare_exchange_strong_explicit(object, expected, desired, success, failure) \
	__builtin_ie64_atomic_compare_exchange((object), (expected), (desired), (success), (failure))
#define atomic_compare_exchange_weak_explicit atomic_compare_exchange_strong_explicit

#define __IE64_ATOMIC_FETCH(object, operand, order, operation) \
	__builtin_ie64_atomic_fetch_##operation((object), (operand), (order))
#define atomic_fetch_add_explicit(object, operand, order) __IE64_ATOMIC_FETCH((object), (operand), (order), add)
#define atomic_fetch_sub_explicit(object, operand, order) __IE64_ATOMIC_FETCH((object), (operand), (order), sub)
#define atomic_fetch_or_explicit(object, operand, order)  __IE64_ATOMIC_FETCH((object), (operand), (order), or)
#define atomic_fetch_xor_explicit(object, operand, order) __IE64_ATOMIC_FETCH((object), (operand), (order), xor)
#define atomic_fetch_and_explicit(object, operand, order) __IE64_ATOMIC_FETCH((object), (operand), (order), and)
#define atomic_fetch_add(object, operand) atomic_fetch_add_explicit((object), (operand), memory_order_seq_cst)
#define atomic_fetch_sub(object, operand) atomic_fetch_sub_explicit((object), (operand), memory_order_seq_cst)
#define atomic_fetch_or(object, operand) atomic_fetch_or_explicit((object), (operand), memory_order_seq_cst)
#define atomic_fetch_xor(object, operand) atomic_fetch_xor_explicit((object), (operand), memory_order_seq_cst)
#define atomic_fetch_and(object, operand) atomic_fetch_and_explicit((object), (operand), memory_order_seq_cst)

#define atomic_flag_test_and_set_explicit(object, order) \
	(__ie64_atomic_validate_orders((order), memory_order_relaxed), \
	 __builtin_ie64_xchg(&(object)->__value, 1) != 0)
#define atomic_flag_clear_explicit(object, order) \
	do { __ie64_atomic_validate_orders((order), memory_order_relaxed); \
	     (void)__builtin_ie64_xchg(&(object)->__value, 0); \
	} while (0)
#define atomic_flag_test_and_set(object) atomic_flag_test_and_set_explicit((object), memory_order_seq_cst)
#define atomic_flag_clear(object) atomic_flag_clear_explicit((object), memory_order_seq_cst)

#define atomic_signal_fence(order) \
	(__ie64_atomic_validate_orders((order), memory_order_relaxed), __ie64_atomic_signal_fence())
#define atomic_thread_fence(order) \
	(__ie64_atomic_validate_orders((order), memory_order_relaxed), __ie64_atomic_thread_fence())
#define atomic_is_lock_free(object) __ie64_atomic_is_lock_free(sizeof *(object), (const volatile void *)(object))

#define ATOMIC_BOOL_LOCK_FREE 0
#define ATOMIC_CHAR_LOCK_FREE 0
#define ATOMIC_CHAR16_T_LOCK_FREE 0
#define ATOMIC_CHAR32_T_LOCK_FREE 0
#define ATOMIC_WCHAR_T_LOCK_FREE 0
#define ATOMIC_SHORT_LOCK_FREE 0
#define ATOMIC_INT_LOCK_FREE 0
#define ATOMIC_LONG_LOCK_FREE 0
#define ATOMIC_LLONG_LOCK_FREE 0
#define ATOMIC_POINTER_LOCK_FREE 0

#endif
