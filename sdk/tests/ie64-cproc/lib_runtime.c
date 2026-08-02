#include <ctype.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#define RESULT ((volatile uint64_t *)0x80000)

static int
long_compare(const void *a, const void *b)
{
	long x = *(const long *)a, y = *(const long *)b;
	return x < y ? -1 : x > y;
}

int
main(void)
{
	char a[32] = "abc", b[32];
	long values[5] = {4, 1, 5, 2, 3};
	long key = 4;
	char *end;
	unsigned char *p, *z, *grown;
	void *released, *after_release;
	unsigned int allocations = 0;

	memset(b, 'x', sizeof b);
	memcpy(b, a, 4);
	if (memcmp(a, b, 4) || memchr(b, 'b', 4) != b + 1)
		return 1;
	memmove(b + 1, b, 4);
	if (strcmp(b, "aabc"))
		return 2;
	if (strlen(b) != 4)
		return 17;
	if (strchr(b, 'b') != b + 2)
		return 18;
	strcpy(b, "ab"); strcat(b, "cd"); strncat(b, "efg", 2);
	if (strcmp(b, "abcdef") || strncmp(b, "abcxyz", 3))
		return 3;
	strncpy(b, "xy", 4);
	if (b[0] != 'x' || b[1] != 'y' || b[2] || b[3])
		return 4;
	if (!isalnum('9') || !isalpha('Z') || !isblank(' ') || !iscntrl('\n')
	|| !isdigit('5') || !isgraph('!') || !islower('a') || !isprint(' ')
	|| !ispunct('!') || !isspace('\t') || !isupper('A') || !isxdigit('f')
	|| tolower('Q') != 'q' || toupper('q') != 'Q')
		return 5;
	if (strtol(" -0x20z", &end, 0) != -32 || *end != 'z'
	|| strtoul("077!", &end, 0) != 63 || *end != '!'
	|| strtoll("123", 0, 10) != 123 || strtoull("ff", 0, 16) != 255)
		return 6;
	qsort(values, 5, sizeof values[0], long_compare);
	if (values[0] != 1 || values[1] != 2 || values[2] != 3
	|| values[3] != 4 || values[4] != 5) {
		*RESULT = (uint64_t)values[0] | (uint64_t)values[1] << 8
			| (uint64_t)values[2] << 16 | (uint64_t)values[3] << 24
			| (uint64_t)values[4] << 32;
		return 7;
	}
	if (*(long *)bsearch(&key, values, 5, sizeof values[0], long_compare) != 4)
		return 19;
	p = malloc(3); z = calloc(4, 2);
	if (!p || !z || ((uintptr_t)p & 7) || ((uintptr_t)z & 7))
		return 8;
	p[0] = 1; p[1] = 2; p[2] = 3;
	if (z[0] || z[7])
		return 9;
	grown = realloc(p, 16);
	if (!grown || grown[0] != 1 || grown[1] != 2 || grown[2] != 3)
		return 10;
	if (malloc(0) || calloc((size_t)-1, 2) || realloc(grown, 0))
		return 11;
	free(0); free(z); free(grown);
	if (abs(-3) != 3 || labs(-4) != 4 || llabs(-5) != 5)
		return 12;
	released = malloc(8);
	free(released);
	after_release = malloc(8);
	if (!released || !after_release || released == after_release)
		return 13;
	p = realloc(0, 8);
	if (!p)
		return 14;
	while (malloc(4096))
		allocations++;
	while (malloc(8))
		allocations++;
	if (!allocations || malloc(8))
		return 15;
	free(p);
	if (malloc(8))
		return 16;
	*RESULT = 0x4c49425041535345UL;
	return 0;
}
