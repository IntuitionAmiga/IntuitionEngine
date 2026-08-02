#ifndef IE64_STDARG_H
#define IE64_STDARG_H

typedef unsigned char *va_list;
#define va_start(ap, last) __builtin_va_start(ap, last)
#define va_arg(ap, type) __builtin_va_arg(ap, type)
#define va_copy(dst, src) ((dst) = (src))
#define va_end(ap) ((void)0)

#endif
