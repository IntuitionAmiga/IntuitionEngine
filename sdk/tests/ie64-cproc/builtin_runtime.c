#include <ie64.h>
#include <stdint.h>

#define RESULT ((volatile uint64_t *)0x80000)
#define ATOMIC_WORD ((volatile uint64_t *)0x82000)

static volatile uint64_t trap_record;

static _Noreturn void
trap_handler(void)
{
	uint64_t cause = __builtin_ie64_mfcr(IE64_CR_FAULT_CAUSE);
	trap_record = (cause << 32) | __builtin_ie64_mfcr(IE64_CR_FAULT_ADDR);
	__builtin_ie64_eret();
}

static int
fail(unsigned int code)
{
	*RESULT = code;
	return (int)code;
}

int
main(void)
{
	uint64_t old;
	float f;
	double d;

	*ATOMIC_WORD = 10;
	old = ie64_atomic_compare_exchange(ATOMIC_WORD, 10, 20);
	if (old != 10 || *ATOMIC_WORD != 20)
		return fail(1);
	old = ie64_atomic_compare_exchange(ATOMIC_WORD, 10, 30);
	if (old != 20 || *ATOMIC_WORD != 20)
		return fail(2);
	if (ie64_atomic_exchange(ATOMIC_WORD, 7) != 20 || *ATOMIC_WORD != 7)
		return fail(3);
	if (ie64_atomic_fetch_add(ATOMIC_WORD, 5) != 7 || *ATOMIC_WORD != 12)
		return fail(4);
	if (ie64_atomic_fetch_and(ATOMIC_WORD, 10) != 12 || *ATOMIC_WORD != 8)
		return fail(5);
	if (ie64_atomic_fetch_or(ATOMIC_WORD, 3) != 8 || *ATOMIC_WORD != 11)
		return fail(6);
	if (ie64_atomic_fetch_xor(ATOMIC_WORD, 15) != 11 || *ATOMIC_WORD != 4)
		return fail(7);

	__builtin_ie64_mtcr(IE64_CR_TP, 0x123456789abcdef0UL);
	if (__builtin_ie64_mfcr(IE64_CR_TP) != 0x123456789abcdef0UL)
		return fail(8);
	__builtin_ie64_tlbinval(0x4000);
	__builtin_ie64_tlbflush();
	__builtin_ie64_suaen();
	__builtin_ie64_suadis();
	__builtin_ie64_mtcr(IE64_CR_TIMER_CTRL, 0);
	__builtin_ie64_mtcr(IE64_CR_TRAP_VEC, (uintptr_t)trap_handler);
	ie64_disable_interrupts();
	ie64_nop();
	__builtin_ie64_wait(0);
	if (__builtin_ie64_smode() != 1)
		return fail(17);
	(void)__builtin_ie64_syscall(0x42);
	if (trap_record != (6UL << 32 | 0x42))
		return fail(18);

	f = __builtin_ie64_fmovecr(9);
	if (f != 2.0f || __builtin_ie64_dmovecr(13) != 0.5)
		return fail(9);
	if (__builtin_ie64_fmod(7.5f, 2.0f) != 1.5f
	|| __builtin_ie64_dmod(9.0, 4.0) != 1.0)
		return fail(10);
	if (__builtin_ie64_fint(2.75f) != 3.0f
	|| __builtin_ie64_dint(3.25) != 3.0)
		return fail(11);
	d = __builtin_ie64_fcvtsd(1.5f);
	f = __builtin_ie64_fcvtds(d);
	if (d != 1.5 || f != 1.5f)
		return fail(12);
	if (__builtin_ie64_fsqrt(9.0f) != 3.0f
	|| __builtin_ie64_dsqrt(16.0) != 4.0)
		return fail(13);
	if (__builtin_ie64_fsin(0.0f) != 0.0f
	|| __builtin_ie64_fcos(0.0f) != 1.0f
	|| __builtin_ie64_ftan(0.0f) != 0.0f
	|| __builtin_ie64_fatan(0.0f) != 0.0f
	|| __builtin_ie64_dsin(0.0) != 0.0
	|| __builtin_ie64_dcos(0.0) != 1.0
	|| __builtin_ie64_dtan(0.0) != 0.0
	|| __builtin_ie64_datan(0.0) != 0.0)
		return fail(14);

	__builtin_ie64_fmovsc(0);
	if (__builtin_ie64_fmovsr() != 0)
		return fail(15);
	__builtin_ie64_fmovcc(0);
	if (__builtin_ie64_fmovcr() != 0)
		return fail(16);
	*RESULT = 0x4255494c54494e53UL;
	return 0;
}
