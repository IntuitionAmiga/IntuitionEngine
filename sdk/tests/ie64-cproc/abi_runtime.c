#include <stdarg.h>
#include <stdint.h>

#define RESULT ((volatile uint64_t *)0x80000)
#define FP_RECORD ((volatile uint32_t *)0x81000)

struct pair {
	int tag;
	double value;
};

struct wide {
	long first;
	char byte;
	short half;
};

struct bitfield_example {
	unsigned int low : 3;
	unsigned int middle : 5;
	unsigned int high : 8;
};

union aggregate_union {
	long integer;
	double real;
};

_Static_assert(sizeof(struct bitfield_example) == 4, "bit-field size");
_Static_assert(_Alignof(struct bitfield_example) == 4, "bit-field alignment");

static long
consume_wide(struct wide value)
{
	return value.first + value.byte + value.half;
}

static long
mutate_union(union aggregate_union value)
{
	value.integer = 99;
	return value.integer;
}

extern long abi_narrow(signed char, unsigned char, short, unsigned short, int,
	unsigned int, signed char, unsigned char, short, unsigned short, int,
	unsigned int, _Bool);
extern long abi_fp(float, float, float, float, float, float, float, double,
	float);
extern long abi_spilled_float(float, float, float, float, float, float, float,
	float, float);
extern long abi_call_c_spilled_float(void);
extern long abi_call_c_spilled_double(void);

long
abi_receive_spilled_float(float a, float b, float c, float d, float e,
	float f, float g, float h, float stacked)
{
	return a == 1.0f && b == 1.0f && c == 1.0f && d == 1.0f
		&& e == 1.0f && f == 1.0f && g == 1.0f && h == 1.0f
		&& stacked == 9.0f;
}

long
abi_receive_spilled_double(float a, float b, float c, float d, float e,
	float f, float g, double stacked, float final)
{
	return a == 1.0f && b == 1.0f && c == 1.0f && d == 1.0f
		&& e == 1.0f && f == 1.0f && g == 1.0f
		&& stacked == 7.0 && final == 8.0f;
}

static int
fail(unsigned int code)
{
	*RESULT = code;
	return (int)code;
}

static struct pair
make_pair(int tag, double value, long a, long b, long c, long d, long e,
	long f, long g)
{
	struct pair result = {tag + a + b + c + d + e + f + g, value};
	return result;
}

static long
mutate_pair(struct pair value)
{
	value.tag = 99;
	return value.tag;
}

static long
read_variadic(int tag, double named, struct pair fixed, ...)
{
	va_list ap;
	struct pair a;
	struct wide b;
	long first, last;

	va_start(ap, fixed);
	a = va_arg(ap, struct pair);
	first = va_arg(ap, long);
	b = va_arg(ap, struct wide);
	last = va_arg(ap, long);
	return tag + (named == 2.0) + fixed.tag + a.tag + first
		+ consume_wide(b) + last;
}

long
read_named_overflow(int tag, double named, struct pair fixed,
	long a, long b, long c, long d, long e, ...)
{
	va_list ap;
	long first, last;
	struct wide middle;

	va_start(ap, e);
	first = va_arg(ap, long);
	middle = va_arg(ap, struct wide);
	last = va_arg(ap, long);
	return tag + (named == 6.0) + fixed.tag + a + b + c + d + e
		+ first + middle.first + middle.byte + middle.half + last;
}

extern long abi_call_variadic_overflow(void);

int
main(void)
{
	struct pair original = {7, 8.0};
	struct pair made;
	struct wide wide = {11, 12, 13};
	struct bitfield_example bits = {5, 17, 0xa5};
	union aggregate_union aggregate_union = {.integer = 42};

	if (bits.low != 5 || bits.middle != 17 || bits.high != 0xa5)
		return fail(10);
	if (mutate_union(aggregate_union) != 99 || aggregate_union.integer != 42)
		return fail(11);

	if (!abi_narrow(-1, 255, -2, 65535, -3, 0xffffffffu,
		-4, 254, -5, 65534, -6, 0xfffffffeu, 1))
		return fail(1);
	made = make_pair(3, 4.0, 1, 2, 3, 4, 5, 6, 7);
	if (made.tag != 31 || made.value != 4.0)
		return fail(0x20000000u | (unsigned int)made.tag);
	if (mutate_pair(original) != 99 || original.tag != 7)
		return fail(3);
	if (read_variadic(1, 2.0, made, original, 5L, wide, 6L) != 87)
		return fail(4);
	if (read_named_overflow(1, 6.0, original, 2, 3, 4, 5, 6,
		7L, wide, 8L) != 80)
		return fail(7);
	if (abi_call_variadic_overflow() != 80)
		return fail(9);
	if (!abi_fp(0.0f, 1.0f, 2.0f, 3.0f, 4.0f, 5.0f, 6.0f,
		7.0, 8.0f))
		return fail(5);
	if (!abi_spilled_float(0.0f, 1.0f, 2.0f, 3.0f, 4.0f, 5.0f,
		6.0f, 7.0f, 9.0f) || !abi_call_c_spilled_float()
	|| !abi_call_c_spilled_double())
		return fail(8);
	if (FP_RECORD[0] != 0x00000000 || FP_RECORD[1] != 0x3f800000
	|| FP_RECORD[6] != 0x40c00000 || FP_RECORD[7] != 0x41000000
	|| FP_RECORD[16] != 0x00000000 || FP_RECORD[17] != 0x401c0000)
		return fail(6);
	*RESULT = 0x4142495041535345UL;
	return 0;
}
