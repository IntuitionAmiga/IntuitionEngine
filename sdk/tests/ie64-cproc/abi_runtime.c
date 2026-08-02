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

extern long abi_narrow(signed char, unsigned char, short, unsigned short, int,
	unsigned int, signed char, unsigned char, short, unsigned short, int,
	unsigned int, _Bool);
extern long abi_fp(float, float, float, float, float, float, float, double,
	float);

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
	return tag + (named == 2.0) + fixed.tag + a.tag + first + b.first + last;
}

int
main(void)
{
	struct pair original = {7, 8.0};
	struct pair made;
	struct wide wide = {11, 12, 13};

	if (!abi_narrow(-1, 255, -2, 65535, -3, 0xffffffffu,
		-4, 254, -5, 65534, -6, 0xfffffffeu, 1))
		return fail(1);
	made = make_pair(3, 4.0, 1, 2, 3, 4, 5, 6, 7);
	if (made.tag != 31 || made.value != 4.0)
		return fail(0x20000000u | (unsigned int)made.tag);
	if (mutate_pair(original) != 99 || original.tag != 7)
		return fail(3);
	if (read_variadic(1, 2.0, made, original, 5L, wide, 6L) != 62)
		return fail(4);
	if (!abi_fp(0.0f, 1.0f, 2.0f, 3.0f, 4.0f, 5.0f, 6.0f,
		7.0, 8.0f))
		return fail(5);
	if (FP_RECORD[0] != 0x00000000 || FP_RECORD[1] != 0x3f800000
	|| FP_RECORD[6] != 0x40c00000 || FP_RECORD[7] != 0x41000000
	|| FP_RECORD[16] != 0x00000000 || FP_RECORD[17] != 0x401c0000)
		return fail(6);
	*RESULT = 0x4142495041535345UL;
	return 0;
}
