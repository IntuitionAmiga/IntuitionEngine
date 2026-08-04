/* Freestanding support library for the IE64 bare-metal C ABI. */
#include <stddef.h>
#include <stdint.h>
#include <ctype.h>
#include <limits.h>
#include <stdlib.h>
#include <string.h>
#include <intuitionengine.h>

_Noreturn void __ie64_terminate(int status)
{
	(void)status;
	__builtin_ie64_halt();
	__builtin_unreachable();
}

typedef void (*ie64_array_function)(void);

extern ie64_array_function __preinit_array_start[];
extern ie64_array_function __preinit_array_end[];
extern ie64_array_function __init_array_start[];
extern ie64_array_function __init_array_end[];
extern ie64_array_function __fini_array_start[];
extern ie64_array_function __fini_array_end[];

void __libc_init_array(void)
{
	ie64_array_function *function;

	for (function = __preinit_array_start; function < __preinit_array_end; ++function)
		(*function)();
	for (function = __init_array_start; function < __init_array_end; ++function)
		(*function)();
}

void __libc_fini_array(void)
{
	ie64_array_function *function = __fini_array_end;

	while (function != __fini_array_start) {
		--function;
		(*function)();
	}
}

_Noreturn void exit(int status)
{
	__libc_fini_array();
	__ie64_terminate(status);
}

/* Host ctype headers may expose these as function-like macros. This library
 * supplies the functions, so their names must remain available for definitions. */
#ifdef isalnum
#undef isalnum
#endif
#ifdef isalpha
#undef isalpha
#endif
#ifdef isblank
#undef isblank
#endif
#ifdef iscntrl
#undef iscntrl
#endif
#ifdef isdigit
#undef isdigit
#endif
#ifdef isgraph
#undef isgraph
#endif
#ifdef islower
#undef islower
#endif
#ifdef isprint
#undef isprint
#endif
#ifdef ispunct
#undef ispunct
#endif
#ifdef isspace
#undef isspace
#endif
#ifdef isupper
#undef isupper
#endif
#ifdef isxdigit
#undef isxdigit
#endif
#ifdef tolower
#undef tolower
#endif
#ifdef toupper
#undef toupper
#endif

extern unsigned char __ie64_heap_start[];

#ifndef IE64_HEAP_START
#define IE64_HEAP_START __ie64_heap_start
#endif

#ifndef IE64_HEAP_LIMIT
#define IE64_HEAP_LIMIT ((unsigned char *)0x8F000)
#endif

typedef struct ie64_alloc_header {
	size_t size;
} ie64_alloc_header;

static unsigned char *ie64_heap_cursor;
static unsigned char *const ie64_heap_limit = IE64_HEAP_LIMIT;

static size_t ie64_align8(size_t n)
{
	if (n > (size_t)-1 - 7)
		return 0;
	return (n + 7) & ~(size_t)7;
}

static unsigned char *ie64_heap_first_byte(void)
{
	uintptr_t start = (uintptr_t)IE64_HEAP_START;
	start = (start + 7) & ~(uintptr_t)7;
	return (unsigned char *)start;
}

void *malloc(size_t size)
{
	size_t payload = ie64_align8(size);
	size_t total;
	ie64_alloc_header *header;
	if (size == 0 || payload == 0 || payload > (size_t)-1 - sizeof(*header))
		return NULL;
	if (ie64_heap_cursor == NULL)
		ie64_heap_cursor = ie64_heap_first_byte();
	if ((uintptr_t)ie64_heap_cursor >= (uintptr_t)ie64_heap_limit)
		return NULL;
	total = payload + sizeof(*header);
	if ((size_t)(ie64_heap_limit - ie64_heap_cursor) < total)
		return NULL;
	header = (ie64_alloc_header *)ie64_heap_cursor;
	header->size = size;
	ie64_heap_cursor += total;
	return header + 1;
}

void free(void *ptr)
{
	(void)ptr;
}

void *calloc(size_t count, size_t size)
{
	size_t total;
	void *ptr;
	if (count == 0 || size == 0 || count > (size_t)-1 / size)
		return NULL;
	total = count * size;
	ptr = malloc(total);
	if (ptr != NULL)
		memset(ptr, 0, total);
	return ptr;
}

void *realloc(void *ptr, size_t size)
{
	ie64_alloc_header *old;
	void *next;
	size_t copy;
	if (ptr == NULL)
		return malloc(size);
	if (size == 0)
		return NULL;
	old = (ie64_alloc_header *)ptr - 1;
	next = malloc(size);
	if (next == NULL)
		return NULL;
	copy = old->size < size ? old->size : size;
	memcpy(next, ptr, copy);
	return next;
}

void *memchr(const void *s, int c, size_t n)
{
	const unsigned char *p = s;
	unsigned char needle = (unsigned char)c;
	while (n-- != 0) {
		if (*p == needle)
			return (void *)p;
		++p;
	}
	return NULL;
}

int memcmp(const void *a, const void *b, size_t n)
{
	const unsigned char *p = a, *q = b;
	while (n-- != 0) {
		if (*p != *q)
			return *p < *q ? -1 : 1;
		++p;
		++q;
	}
	return 0;
}

void *memcpy(void *dst, const void *src, size_t n)
{
	unsigned char *d = dst;
	const unsigned char *s = src;
	while (n-- != 0)
		*d++ = *s++;
	return dst;
}

void *memmove(void *dst, const void *src, size_t n)
{
	unsigned char *d = dst;
	const unsigned char *s = src;
	if (d < s) {
		while (n-- != 0)
			*d++ = *s++;
	} else {
		d += n;
		s += n;
		while (n-- != 0)
			*--d = *--s;
	}
	return dst;
}

void *memset(void *dst, int c, size_t n)
{
	unsigned char *d = dst;
	while (n-- != 0)
		*d++ = (unsigned char)c;
	return dst;
}

size_t strlen(const char *s)
{
	const char *p = s;
	while (*p != '\0')
		++p;
	return (size_t)(p - s);
}

char *strcpy(char *dst, const char *src)
{
	char *out = dst;
	while ((*dst++ = *src++) != '\0')
		;
	return out;
}

char *strncpy(char *dst, const char *src, size_t n)
{
	char *out = dst;
	while (n != 0 && *src != '\0') {
		*dst++ = *src++;
		--n;
	}
	while (n-- != 0)
		*dst++ = '\0';
	return out;
}

char *strcat(char *dst, const char *src)
{
	return strcpy(dst + strlen(dst), src);
}

char *strncat(char *dst, const char *src, size_t n)
{
	char *out = dst + strlen(dst);
	while (n-- != 0 && *src != '\0')
		*out++ = *src++;
	*out = '\0';
	return dst;
}

int strcmp(const char *a, const char *b)
{
	while (*a == *b && *a != '\0') {
		++a;
		++b;
	}
	return (unsigned char)*a < (unsigned char)*b ? -1 :
		(unsigned char)*a > (unsigned char)*b;
}

int strncmp(const char *a, const char *b, size_t n)
{
	while (n-- != 0) {
		if (*a != *b)
			return (unsigned char)*a < (unsigned char)*b ? -1 : 1;
		if (*a++ == '\0')
			return 0;
		++b;
	}
	return 0;
}

char *strchr(const char *s, int c)
{
	char ch = (char)c;
	for (;; ++s) {
		if (*s == ch)
			return (char *)s;
		if (*s == '\0')
			return NULL;
	}
}

int isalpha(int c) { return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'); }
int isdigit(int c) { return c >= '0' && c <= '9'; }
int islower(int c) { return c >= 'a' && c <= 'z'; }
int isupper(int c) { return c >= 'A' && c <= 'Z'; }
int isalnum(int c) { return isalpha(c) || isdigit(c); }
int isblank(int c) { return c == ' ' || c == '\t'; }
int iscntrl(int c) { return c < 0x20 || c == 0x7f; }
int isprint(int c) { return c >= 0x20 && c < 0x7f; }
int isgraph(int c) { return c > 0x20 && c < 0x7f; }
int isspace(int c) { return c == ' ' || (c >= '\t' && c <= '\r'); }
int isxdigit(int c) { return isdigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'); }
int ispunct(int c) { return isgraph(c) && !isalnum(c); }
int tolower(int c) { return isupper(c) ? c + ('a' - 'A') : c; }
int toupper(int c) { return islower(c) ? c - ('a' - 'A') : c; }

int abs(int n) { return n < 0 ? -n : n; }
long labs(long n) { return n < 0 ? -n : n; }
long long llabs(long long n) { return n < 0 ? -n : n; }

static int ie64_digit(int c)
{
	if (isdigit(c)) return c - '0';
	if (c >= 'a' && c <= 'z') return c - 'a' + 10;
	if (c >= 'A' && c <= 'Z') return c - 'A' + 10;
	return -1;
}

static unsigned long long ie64_strtoull(const char *s, char **end, int base, unsigned long long limit, int *overflow)
{
	const char *p = s;
	unsigned long long value = 0;
	int digit;
	while (isspace((unsigned char)*p)) ++p;
	if (base == 0) {
		base = 10;
		if (p[0] == '0') {
			if ((p[1] == 'x' || p[1] == 'X') && ie64_digit((unsigned char)p[2]) >= 0 && ie64_digit((unsigned char)p[2]) < 16) {
				base = 16;
				p += 2;
			} else {
				base = 8;
			}
		}
	} else if (base == 16 && p[0] == '0' && (p[1] == 'x' || p[1] == 'X') && ie64_digit((unsigned char)p[2]) >= 0 && ie64_digit((unsigned char)p[2]) < 16) {
		p += 2;
	}
	while ((digit = ie64_digit((unsigned char)*p)) >= 0 && digit < base) {
		if (value > (limit - (unsigned)digit) / (unsigned)base)
			*overflow = 1;
		else if (!*overflow)
			value = value * (unsigned)base + (unsigned)digit;
		++p;
	}
	if (end != NULL) *end = (char *)(p == s ? s : p);
	return value;
}

unsigned long long strtoull(const char *s, char **end, int base)
{
	int overflow = 0;
	unsigned long long value;
	const char *start = s;
	int negative = 0;
	while (isspace((unsigned char)*s)) ++s;
	if (*s == '-' || *s == '+') negative = *s++ == '-';
	value = ie64_strtoull(s, end, base, ULLONG_MAX, &overflow);
	if (end != NULL && *end == s) *end = (char *)start;
	if (negative && !overflow) value = 0 - value;
	return overflow ? ULLONG_MAX : value;
}

long long strtoll(const char *s, char **end, int base)
{
	int negative = 0, overflow = 0;
	unsigned long long value;
	const char *start = s;
	while (isspace((unsigned char)*s)) ++s;
	if (*s == '-' || *s == '+') negative = *s++ == '-';
	value = ie64_strtoull(s, end, base, negative ? (unsigned long long)LLONG_MAX + 1 : LLONG_MAX, &overflow);
	if (end != NULL && *end == s) *end = (char *)start;
	if (overflow) return negative ? LLONG_MIN : LLONG_MAX;
	if (negative) return value == (unsigned long long)LLONG_MAX + 1 ? LLONG_MIN : -(long long)value;
	return (long long)value;
}

unsigned long strtoul(const char *s, char **end, int base) { return (unsigned long)strtoull(s, end, base); }
long strtol(const char *s, char **end, int base) { return (long)strtoll(s, end, base); }

static void ie64_swap(unsigned char *a, unsigned char *b, size_t size)
{
	while (size-- != 0) { unsigned char t = *a; *a++ = *b; *b++ = t; }
}

void qsort(void *base, size_t count, size_t size, int (*compare)(const void *, const void *))
{
	unsigned char *items = base;
	size_t i, j;
	if (count < 2 || size == 0) return;
	for (i = 1; i < count; ++i)
		for (j = i; j != 0 && compare(items + (j - 1) * size, items + j * size) > 0; --j)
			ie64_swap(items + (j - 1) * size, items + j * size, size);
}

void *bsearch(const void *key, const void *base, size_t count, size_t size, int (*compare)(const void *, const void *))
{
	const unsigned char *items = base;
	while (count != 0) {
		size_t half = count / 2;
		const void *item = items + half * size;
		int order = compare(key, item);
		if (order == 0) return (void *)item;
		if (order < 0) count = half;
		else { items += (half + 1) * size; count -= half + 1; }
	}
	return NULL;
}
int __ie64_clz32(unsigned int value)
{
	int count = 0;
	unsigned int bit = 1U << 31;

	while (bit && !(value & bit)) {
		++count;
		bit >>= 1;
	}
	return count;
}

int __ie64_clz64(unsigned long value)
{
	int count = 0;
	unsigned long bit = 1UL << 63;

	while (bit && !(value & bit)) {
		++count;
		bit >>= 1;
	}
	return count;
}
