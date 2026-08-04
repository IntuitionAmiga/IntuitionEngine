/* Host-side behavioural checks for architecture-independent libie64c logic. */
#include <stdint.h>

static unsigned char ie64_test_heap[128];

#define IE64_HEAP_START (ie64_test_heap + 1)
#define IE64_HEAP_LIMIT (ie64_test_heap + sizeof(ie64_test_heap))
#define IE_TARGET_X86 1
#define IE64_H
#define __builtin_ie64_halt() __builtin_trap()
#include "../../lib/ie64-cproc/libie64c.c"

static int check_end(const char *text, int base, unsigned long long want, unsigned long offset)
{
	char *end;
	if (strtoull(text, &end, base) != want)
		return 1;
	return end != text + offset;
}

int main(void)
{
	void *first = malloc(1);
	void *second = malloc(1);
	if (first == 0 || second == 0)
		return 1;
	if (((uintptr_t)first & 7) != 0 || ((uintptr_t)second & 7) != 0)
		return 2;
	ie64_heap_cursor = (unsigned char *)((uintptr_t)ie64_heap_limit + 8);
	if (malloc(1) != 0)
		return 3;
	if (check_end("0x", 0, 0, 1) || check_end("0xg", 0, 0, 1) ||
	    check_end("0x", 16, 0, 1) || check_end("0xg", 16, 0, 1) ||
	    check_end("+0x", 0, 0, 2) || check_end("+0x", 16, 0, 2))
		return 4;
	return 0;
}
