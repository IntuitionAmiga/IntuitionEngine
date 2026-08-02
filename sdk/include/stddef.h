#ifndef IE64_STDDEF_H
#define IE64_STDDEF_H

#define NULL ((void *)0)
#define offsetof(type, member) ((size_t)&(((type *)0)->member))
typedef unsigned long size_t;
typedef long ptrdiff_t;
typedef unsigned int wchar_t;

#endif
