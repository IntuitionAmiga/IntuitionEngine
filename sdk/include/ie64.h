#ifndef IE64_H
#define IE64_H

/* IE64 bare-metal facilities.  These are raw ISA operations, not C11 atomics. */
#include <stdint.h>

enum ie64_control_register {
	IE64_CR_PTBR = 0, IE64_CR_FAULT_ADDR = 1, IE64_CR_FAULT_CAUSE = 2,
	IE64_CR_FAULT_PC = 3, IE64_CR_TRAP_VEC = 4, IE64_CR_MMU_CTRL = 5,
	IE64_CR_TP = 6, IE64_CR_INTR_VEC = 7, IE64_CR_KSP = 8,
	IE64_CR_TIMER_PERIOD = 9, IE64_CR_TIMER_COUNT = 10,
	IE64_CR_TIMER_CTRL = 11, IE64_CR_USP = 12, IE64_CR_PREV_MODE = 13,
	IE64_CR_SAVED_SUA = 14, IE64_CR_RAM_SIZE_BYTES = 15
};

uint64_t __builtin_ie64_mfcr(unsigned int control_register);
void __builtin_ie64_mtcr(unsigned int control_register, uint64_t value);
void __builtin_ie64_tlbinval(uint64_t address);
void __builtin_ie64_tlbflush(void);
void __builtin_ie64_suaen(void);
void __builtin_ie64_suadis(void);
_Noreturn void __builtin_ie64_eret(void);
_Noreturn void __builtin_ie64_rti(void);
_Noreturn void __builtin_ie64_halt(void);
void __builtin_ie64_nop(void);
void __builtin_ie64_sei(void);
void __builtin_ie64_cli(void);
void __builtin_ie64_wait(unsigned int microseconds);
uint64_t __builtin_ie64_syscall(unsigned int number);
uint64_t __builtin_ie64_smode(void);

uint64_t __builtin_ie64_cas(volatile uint64_t *address, uint64_t expected, uint64_t desired);
uint64_t __builtin_ie64_xchg(volatile uint64_t *address, uint64_t value);
uint64_t __builtin_ie64_faa(volatile uint64_t *address, uint64_t value);
uint64_t __builtin_ie64_fand(volatile uint64_t *address, uint64_t value);
uint64_t __builtin_ie64_for(volatile uint64_t *address, uint64_t value);
uint64_t __builtin_ie64_fxor(volatile uint64_t *address, uint64_t value);

/* The operation returns the old value. The architectural aperture, alignment,
 * fault and full-barrier rules apply exactly as they do to the instruction. */
static uint64_t ie64_atomic_compare_exchange(volatile uint64_t *address, uint64_t expected, uint64_t desired)
{
	return __builtin_ie64_cas(address, expected, desired);
}
static uint64_t ie64_atomic_exchange(volatile uint64_t *address, uint64_t value)
{
	return __builtin_ie64_xchg(address, value);
}
static uint64_t ie64_atomic_fetch_add(volatile uint64_t *address, uint64_t value)
{
	return __builtin_ie64_faa(address, value);
}
static uint64_t ie64_atomic_fetch_and(volatile uint64_t *address, uint64_t value)
{
	return __builtin_ie64_fand(address, value);
}
static uint64_t ie64_atomic_fetch_or(volatile uint64_t *address, uint64_t value)
{
	return __builtin_ie64_for(address, value);
}
static uint64_t ie64_atomic_fetch_xor(volatile uint64_t *address, uint64_t value)
{
	return __builtin_ie64_fxor(address, value);
}

/* FPU operations without an ordinary C expression. */
float __builtin_ie64_fmovecr(unsigned int constant);
double __builtin_ie64_dmovecr(unsigned int constant);
float __builtin_ie64_fmod(float, float);
double __builtin_ie64_dmod(double, double);
float __builtin_ie64_fabs(float);
double __builtin_ie64_dabs(double);
float __builtin_ie64_fint(float);
double __builtin_ie64_dint(double);
double __builtin_ie64_fcvtsd(float);
float __builtin_ie64_fcvtds(double);
float __builtin_ie64_fsin(float);
float __builtin_ie64_fcos(float);
float __builtin_ie64_ftan(float);
float __builtin_ie64_fatan(float);
float __builtin_ie64_flog(float);
float __builtin_ie64_fexp(float);
float __builtin_ie64_fpow(float, float);
float __builtin_ie64_fsqrt(float);
double __builtin_ie64_dsin(double);
double __builtin_ie64_dcos(double);
double __builtin_ie64_dtan(double);
double __builtin_ie64_datan(double);
double __builtin_ie64_dlog(double);
double __builtin_ie64_dexp(double);
double __builtin_ie64_dpow(double, double);
double __builtin_ie64_dsqrt(double);
uint64_t __builtin_ie64_fmovsr(void);
uint64_t __builtin_ie64_fmovcr(void);
void __builtin_ie64_fmovsc(unsigned int value);
void __builtin_ie64_fmovcc(unsigned int value);

static void ie64_nop(void) { __builtin_ie64_nop(); }
static void ie64_enable_interrupts(void) { __builtin_ie64_sei(); }
static void ie64_disable_interrupts(void) { __builtin_ie64_cli(); }
static _Noreturn void __ie64_assert_fail(void) { __builtin_ie64_halt(); }

#endif
