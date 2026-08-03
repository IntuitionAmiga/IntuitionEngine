#ifndef IE64_STDDEF_H
#define IE64_STDDEF_H

#define NULL ((void *)0)
#define offsetof(type, member) ((size_t)&(((type *)0)->member))
typedef unsigned long size_t;
typedef long ptrdiff_t;
typedef unsigned int wchar_t;
typedef unsigned int wint_t;
typedef struct {
	long long __integer;
	long double __floating;
} max_align_t;
#if __STDC_VERSION__ >= 202311L
typedef typeof(nullptr) nullptr_t;
#endif

#endif
